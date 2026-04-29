package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/beeemt/oxygen/internal/api"
	"github.com/beeemt/oxygen/internal/output"
)

// tracesCmd is the parent for all traces subcommands.
var tracesCmd = &cobra.Command{
	Use:   "traces",
	Short: "Explore and search traces",
}

var tracesLatestCmd = &cobra.Command{
	Use:   "latest",
	Short: "Get latest trace IDs from a stream",
	RunE:  runTracesLatest,
}

var tracesDAGCmd = &cobra.Command{
	Use:   "dag",
	Short: "Get the full DAG (flamegraph/span tree) for a trace",
	RunE:  runTracesDAG,
}

var tracesSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search traces with SQL",
	RunE:  runTracesSearch,
}

func init() {
	rootCmd.AddCommand(tracesCmd)
	tracesCmd.AddCommand(tracesLatestCmd, tracesDAGCmd, tracesSearchCmd)

	// latest flags.
	lf := tracesLatestCmd.Flags()
	lf.String("stream", "", "Trace stream name (required)")
	lf.Int64("size", 20, "Number of trace IDs to return")
	lf.String("start", "1h", "Start time as Go duration")
	lf.String("end", "now", "End time as Go duration or 'now'")

	// dag flags.
	df := tracesDAGCmd.Flags()
	df.String("stream", "", "Trace stream name (required)")
	df.String("trace-id", "", "Trace ID (required)")

	// search flags — reuse logs search flags but add stream.
	sf := tracesSearchCmd.Flags()
	sf.String("stream", "", "Trace stream name")
	sf.String("sql", "", "SQL query over trace stream")
	sf.String("start", "", "Start time as Go duration (e.g. 1h, 24h, 7d)")
	sf.String("end", "", "End time as Go duration or 'now' (default: now)")
	sf.Int64("from", 0, "Offset for pagination")
	sf.Int64("size", 100, "Number of results")
}

func runTracesLatest(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	stream := cmdFlagStr(cmd, "stream")
	if stream == "" {
		return fmt.Errorf("--stream is required")
	}

	startUs, endUs, err := resolveSearchTime(cmd)
	if err != nil {
		return err
	}

	size, _ := cmd.Flags().GetInt64("size")

	if cfg.DryRun {
		dryRun := struct {
			Method string `json:"method"`
			URL    string `json:"url"`
			Stream string `json:"stream"`
			Start  int64  `json:"start_time_us"`
			End    int64  `json:"end_time_us"`
			Size   int64  `json:"size"`
		}{
			Method: "GET",
			URL:    cli.BaseURL + "/" + cli.Org + "/" + stream + "/traces/latest",
			Stream: stream,
			Start:  startUs,
			End:    endUs,
			Size:   size,
		}
		return outWriter.WriteJSON(dryRun)
	}

	resp, err := cli.TracesLatest(cmd.Context(), stream, startUs, endUs, size)
	if err != nil {
		return err
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "Trace ID", "Start Time", "End Time")
		for _, id := range resp.TraceIDs {
			startStr := formatTimestamp(resp.StartTime)
			endStr := formatTimestamp(resp.EndTime)
			tw.Row(id, startStr, endStr)
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(resp)
}

func runTracesDAG(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	stream := cmdFlagStr(cmd, "stream")
	if stream == "" {
		return fmt.Errorf("--stream is required")
	}

	traceID := cmdFlagStr(cmd, "trace-id")
	if traceID == "" {
		return fmt.Errorf("--trace-id is required")
	}

	if cfg.DryRun {
		dryRun := struct {
			Method  string `json:"method"`
			URL     string `json:"url"`
			Stream  string `json:"stream"`
			TraceID string `json:"trace_id"`
		}{
			Method:  "GET",
			URL:     cli.BaseURL + "/" + cli.Org + "/" + stream + "/traces/" + traceID + "/dag",
			Stream:  stream,
			TraceID: traceID,
		}
		return outWriter.WriteJSON(dryRun)
	}

	resp, err := cli.TracesDAG(cmd.Context(), stream, traceID)
	if err != nil {
		return err
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "Span ID", "Parent", "Operation", "Service", "Duration (ms)", "Status")
		for _, s := range resp.Spans {
			status := fmt.Sprintf("%d", s.StatusCode)
			duration := fmt.Sprintf("%.2f", s.DurationMs)
			parent := s.ParentSpanID
			if parent == "" {
				parent = "(root)"
			}
			tw.Row(s.SpanID, parent, s.OperationName, s.ServiceName, duration, status)
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(resp)
}

func runTracesSearch(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	startUs, endUs, err := resolveSearchTime(cmd)
	if err != nil {
		return err
	}

	sql := cmdFlagStr(cmd, "sql")
	stream := cmdFlagStr(cmd, "stream")
	from, _ := cmd.Flags().GetInt64("from")
	size, _ := cmd.Flags().GetInt64("size")

	reqBody := api.SearchRequest{
		Query: api.SearchQuery{
			SQL:       buildSQLWithStream(sql, stream),
			StartTime: startUs,
			EndTime:   endUs,
			From:      from,
			Size:      size,
			QueryType: "sql",
		},
		UseCache: true,
	}

	if cfg.DryRun {
		return printDryRun(cli, "POST", "_search", reqBody, startUs, endUs)
	}

	resp, err := cli.Search(cmd.Context(), reqBody)
	if err != nil {
		return err
	}

	switch cfg.Format {
	case "table":
		return renderHitsTable(resp)
	default:
		return outWriter.WriteSearchMeta(resp, startUs, endUs)
	}
}

// formatTimestamp formats a microsecond unix timestamp for table display.
func formatTimestamp(ts int64) string {
	if ts == 0 {
		return ""
	}
	// OpenObserve timestamps are in microseconds.
	if ts > 1e15 {
		ts = ts / 1000
	}
	return time.Unix(ts, 0).Format(time.RFC3339)
}
