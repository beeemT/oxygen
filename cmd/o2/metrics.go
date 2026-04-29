package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/beeemt/oxygen/internal/api"
	"github.com/beeemt/oxygen/internal/output"
)

// metricsCmd is the parent for all metrics subcommands.
var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Query and explore Prometheus metrics",
}

var metricsQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query Prometheus with PromQL (instant or range)",
	RunE:  runMetricsQuery,
}

var metricsSeriesCmd = &cobra.Command{
	Use:   "series",
	Short: "List series matching a label selector",
	RunE:  runMetricsSeries,
}

var metricsLabelValuesCmd = &cobra.Command{
	Use:   "label-values",
	Short: "List all values for a label name",
	RunE:  runMetricsLabelValues,
}

var metricsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Query metrics stream with SQL",
	RunE:  runMetricsSearch,
}

var metricsStreamsCmd = &cobra.Command{
	Use:   "streams",
	Short: "List metrics streams",
}

var metricsStreamsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List metrics streams",
	RunE:  runMetricsStreamsList,
}

var metricsStreamsDescribeCmd = &cobra.Command{
	Use:   "describe [stream-name]",
	Short: "Show a metrics stream schema",
	Args:  cobra.ExactArgs(1),
	RunE:  runMetricsStreamsDescribe,
}

func init() {
	rootCmd.AddCommand(metricsCmd)
	metricsCmd.AddCommand(metricsQueryCmd, metricsSeriesCmd, metricsLabelValuesCmd, metricsSearchCmd, metricsStreamsCmd)
	metricsStreamsCmd.AddCommand(metricsStreamsListCmd, metricsStreamsDescribeCmd)

	// query flags.
	qf := metricsQueryCmd.Flags()
	qf.String("expr", "", "PromQL expression (required)")
	qf.String("start", "", "Start time: Go duration, unix timestamp, RFC3339, or 'now' (default: now)")
	qf.String("end", "", "End time: Go duration, unix timestamp, RFC3339, or 'now' (default: now)")
	qf.String("step", "", "Query resolution step (e.g. 15s, 1m, 5m)")

	// series flags.
	sf := metricsSeriesCmd.Flags()
	sf.String("match", "", "Series match selector (e.g. 'up{job=\"prometheus\"}')")
	sf.String("start", "", "Start time for series cardinality")
	sf.String("end", "", "End time for series cardinality")

	// label-values flags.
	lvf := metricsLabelValuesCmd.Flags()
	lvf.String("label", "", "Label name to list values for (required)")

	// search flags.
	srf := metricsSearchCmd.Flags()
	srf.String("stream", "", "Metrics stream name")
	srf.String("sql", "", "SQL query over metrics stream")
	srf.String("start", "", "Start time as Go duration (e.g. 1h, 24h, 7d)")
	srf.String("end", "", "End time as Go duration or 'now' (default: now)")
	srf.Int64("from", 0, "Offset for pagination")
	srf.Int64("size", 100, "Number of results")

	// streams list/describe reuse the parent streamsCmd — no extra flags needed.
}

// runMetricsQuery handles both instant and range PromQL queries.
func runMetricsQuery(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	expr := cmdFlagStr(cmd, "expr")
	if expr == "" {
		return fmt.Errorf("--expr is required")
	}

	startStr := cmdFlagStr(cmd, "start")
	endStr := cmdFlagStr(cmd, "end")
	step := cmdFlagStr(cmd, "step")

	// Resolve start/end times to unix timestamps.
	startTS, endTS, err := resolveMetricsTime(startStr, endStr)
	if err != nil {
		return err
	}

	// Range query: requires --step.
	if step != "" {
		return runMetricsQueryRange(cli, expr, startStr, endStr, step, startTS, endTS)
	}

	// Instant query: use end time as the query time.
	return runMetricsQueryInstant(cli, expr, endTS)
}

func runMetricsQueryInstant(cli *api.Client, expr string, endTS int64) error {
	// Use end time as the query time for instant queries.
	timeStr := strconv.FormatInt(endTS, 10)

	if cfg.DryRun {
		// Dry-run: print the resolved request.
		dryRun := struct {
			Method string `json:"method"`
			URL    string `json:"url"`
			Query  string `json:"query"`
			Time   string `json:"time"`
		}{
			Method: "GET",
			URL:    cli.BaseURL + "/" + cli.Org + "/prometheus/api/v1/query",
			Query:  expr,
			Time:   timeStr,
		}
		return outWriter.WriteJSON(dryRun)
	}

	resp, err := cli.PromQLQuery(context.Background(), expr, timeStr)
	if err != nil {
		return err
	}

	if resp.Status != "success" {
		return fmt.Errorf("promql query failed: %s - %s", resp.ErrorType, resp.Error)
	}

	results, err := api.ParsePromQLInstant(resp)
	if err != nil {
		return err
	}

	return outWriter.WriteMetrics(results, expr, endTS)
}

func runMetricsQueryRange(cli *api.Client, expr string, startStr string, endStr string, step string, startTS int64, endTS int64) error {
	// Convert start/end to unix timestamps for the API.
	startUnix := strconv.FormatInt(startTS, 10)
	endUnix := strconv.FormatInt(endTS, 10)

	if cfg.DryRun {
		dryRun := struct {
			Method string `json:"method"`
			URL    string `json:"url"`
			Query  string `json:"query"`
			Start  string `json:"start"`
			End    string `json:"end"`
			Step   string `json:"step"`
		}{
			Method: "GET",
			URL:    cli.BaseURL + "/" + cli.Org + "/prometheus/api/v1/query_range",
			Query:  expr,
			Start:  startUnix,
			End:    endUnix,
			Step:   step,
		}
		return outWriter.WriteJSON(dryRun)
	}

	resp, err := cli.PromQLQueryRange(context.Background(), expr, startUnix, endUnix, step)
	if err != nil {
		return err
	}

	if resp.Status != "success" {
		return fmt.Errorf("promql query_range failed: %s - %s", resp.ErrorType, resp.Error)
	}

	results, err := api.ParsePromQLRange(resp)
	if err != nil {
		return err
	}

	// Render range results using the metrics renderer.
	if cfg.Format == "json" || cfg.Format == "pretty" {
		return outWriter.WriteJSON(results)
	}
	r := output.NewMetricsRenderer(outWriter.Stdout(), cfg.NoColor || cfg.Format == "csv")
	return r.RenderRange(results, expr)
}

// resolveMetricsTime resolves start and end duration strings to unix timestamps.
func resolveMetricsTime(startStr string, endStr string) (startTS int64, endTS int64, err error) {
	_, endTS, err = api.ResolvePromQLTime(endStr)
	if err != nil {
		return 0, 0, fmt.Errorf("--end: %w", err)
	}
	if startStr != "" {
		_, startTS, err = api.ResolvePromQLTime(startStr)
		if err != nil {
			return 0, 0, fmt.Errorf("--start: %w", err)
		}
	} else {
		// Default: start = end - 1 hour
		startTS = endTS - 3600
	}
	return startTS, endTS, nil
}

func runMetricsSeries(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	match := cmdFlagStr(cmd, "match")
	if match == "" {
		return fmt.Errorf("--match is required")
	}
	startStr := cmdFlagStr(cmd, "start")
	endStr := cmdFlagStr(cmd, "end")

	// Resolve optional time range params.
	var startTS, endTS int64
	if startStr != "" {
		_, startTS, err = api.ResolvePromQLTime(startStr)
		if err != nil {
			return fmt.Errorf("--start: %w", err)
		}
	}
	if endStr != "" {
		_, endTS, err = api.ResolvePromQLTime(endStr)
		if err != nil {
			return fmt.Errorf("--end: %w", err)
		}
	}

	if cfg.DryRun {
		dryRun := struct {
			Method string `json:"method"`
			URL    string `json:"url"`
			Match  string `json:"match[]"`
			Start  string `json:"start"`
			End    string `json:"end"`
		}{
			Method: "GET",
			URL:    cli.BaseURL + "/" + cli.Org + "/prometheus/api/v1/series",
			Match:  match,
		}
		if startTS > 0 {
			dryRun.Start = strconv.FormatInt(startTS, 10)
		}
		if endTS > 0 {
			dryRun.End = strconv.FormatInt(endTS, 10)
		}
		return outWriter.WriteJSON(dryRun)
	}

	var startArg, endArg string
	if startTS > 0 {
		startArg = strconv.FormatInt(startTS, 10)
	}
	if endTS > 0 {
		endArg = strconv.FormatInt(endTS, 10)
	}

	resp, err := cli.PromQLSeries(context.Background(), match, startArg, endArg)
	if err != nil {
		return err
	}

	if resp.Status != "success" {
		return fmt.Errorf("promql series failed: %s - %s", resp.ErrorType, resp.Error)
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "Series")
		for _, series := range resp.Data {
			// Format as "key=value,key=value".
			var parts []string
			for k, v := range series {
				parts = append(parts, k+"="+v)
			}
			tw.Row(joinLabelParts(parts))
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(resp.Data)
}

func joinLabelParts(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}

func runMetricsLabelValues(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	label := cmdFlagStr(cmd, "label")
	if label == "" {
		return fmt.Errorf("--label is required")
	}

	if cfg.DryRun {
		dryRun := struct {
			Method string `json:"method"`
			URL    string `json:"url"`
		}{
			Method: "GET",
			URL:    cli.BaseURL + "/" + cli.Org + "/prometheus/api/v1/label/" + label + "/values",
		}
		return outWriter.WriteJSON(dryRun)
	}

	resp, err := cli.PromQLLabelValues(context.Background(), label)
	if err != nil {
		return err
	}

	if resp.Status != "success" {
		return fmt.Errorf("promql label values failed: %s - %s", resp.ErrorType, resp.Error)
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "Value")
		for _, v := range resp.Data {
			tw.Row(v)
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(resp.Data)
}

// runMetricsSearch queries a metrics stream using SQL.
func runMetricsSearch(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	startStr := cmdFlagStr(cmd, "start")
	endStr := cmdFlagStr(cmd, "end")
	startTS, endTS, err := api.ResolveTime(startStr, endStr)
	if err != nil {
		return err
	}

	sql := cmdFlagStr(cmd, "sql")
	stream := cmdFlagStr(cmd, "stream")
	from, _ := cmd.Flags().GetInt64("from")
	size, _ := cmd.Flags().GetInt64("size")

	reqBody := buildMetricsSearchRequest(sql, stream, startTS, endTS, from, size)

	if cfg.DryRun {
		return printDryRun(cli, "POST", "_search", reqBody, startTS, endTS)
	}

	resp, err := cli.Search(context.Background(), reqBody)
	if err != nil {
		return err
	}

	switch cfg.Format {
	case "table":
		return renderHitsTable(resp)
	default:
		return outWriter.WriteSearchMeta(resp, startTS, endTS)
	}
}

// buildMetricsSearchRequest constructs a SearchRequest for metrics stream queries.
func buildMetricsSearchRequest(sql string, stream string, startUs int64, endUs int64, from int64, size int64) api.SearchRequest {
	q := buildSQLWithStream(sql, stream)

	return api.SearchRequest{
		Query: api.SearchQuery{
			SQL:            q,
			StartTime:      startUs,
			EndTime:        endUs,
			From:           from,
			Size:           size,
			TrackTotalHits: true,
			QueryType:      "sql",
		},
		UseCache: true,
	}
}

func runMetricsStreamsList(cmd *cobra.Command, _ []string) error {
	// Reuse the streams list logic but filter for metrics type.
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	resp, err := cli.Streams(context.Background())
	if err != nil {
		return err
	}

	filtered := filterStreamsByType(resp.Streams, "metrics")
	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "Name", "Docs", "Size (MB)")
		for _, s := range filtered {
			sizeMB := float64(s.StorageMB) / (1024 * 1024)
			tw.Row(s.Name, fmt.Sprintf("%d", s.DocCount), fmt.Sprintf("%.2f", sizeMB))
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(filtered)
}

func runMetricsStreamsDescribe(cmd *cobra.Command, args []string) error {
	// Reuse the streams describe logic.
	name := args[0]
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	resp, err := cli.StreamSchema(context.Background(), name)
	if err != nil {
		return err
	}

	// Validate it's a metrics stream.
	if resp.StreamType != "metrics" {
		outWriter.Warning("stream %q has type %q, expected 'metrics'", name, resp.StreamType)
	}

	return outWriter.WriteJSON(resp)
}

func filterStreamsByType(streams []api.StreamInfo, streamType string) []api.StreamInfo {
	var result []api.StreamInfo
	for _, s := range streams {
		if s.StreamType == streamType {
			result = append(result, s)
		}
	}
	return result
}
