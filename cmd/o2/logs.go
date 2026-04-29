package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/beeemt/oxygen/internal/api"
	"github.com/beeemt/oxygen/internal/output"
)

// logsCmd is the parent for all logs subcommands.
var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Query and stream logs",
}

var logsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search logs with SQL or filter",
	RunE:  runLogsSearch,
}

var logsStreamCmd = &cobra.Command{
	Use:   "stream",
	Short: "Stream logs in real time",
	RunE:  runLogsStream,
}

var logsValuesCmd = &cobra.Command{
	Use:   "values",
	Short: "Get field value suggestions",
	RunE:  runLogsValues,
}

var logsHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Show recent query history",
	RunE:  runLogsHistory,
}

var logsViewsCmd = &cobra.Command{
	Use:   "views",
	Short: "Manage saved views",
}

var logsViewsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved views",
	RunE:  runLogsViewsList,
}

var logsViewsGetCmd = &cobra.Command{
	Use:   "get [view-id]",
	Short: "Get a saved view by ID",
	Args:  cobra.ExactArgs(1),
	RunE:  runLogsViewsGet,
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.AddCommand(logsSearchCmd, logsStreamCmd, logsValuesCmd, logsHistoryCmd, logsViewsCmd)
	logsViewsCmd.AddCommand(logsViewsListCmd, logsViewsGetCmd)

	fs := logsSearchCmd.Flags()
	fs.String("stream", "", "Stream name")
	fs.String("sql", "", "SQL query (e.g. \"SELECT * WHERE status = 'error'\")")
	fs.String("filter", "", "Full-text filter (alternative to --sql)")
	fs.Int64("from", 0, "Offset for pagination")
	fs.Int64("size", 100, "Number of results (max 10000)")
	fs.String("start", "", "Start time as Go duration (e.g. 1h, 24h, 7d)")
	fs.String("end", "", "End time as Go duration or 'now' (default: now)")
	fs.Bool("track-total", true, "Count total matching records")
	fs.Bool("quick-mode", false, "Enable quick mode for faster results")

	// Stream flags.
	sfs := logsStreamCmd.Flags()
	sfs.String("stream", "", "Stream name")
	sfs.String("sql", "", "SQL query")
	sfs.String("filter", "", "Full-text filter")
	sfs.String("start", "3h", "Start time as Go duration (e.g. 1h, 24h, 7d)")
	sfs.String("end", "", "End time as Go duration or 'now' (default: now)")
	sfs.Int64("size", 1000, "Results per page")
	sfs.Bool("follow", false, "Keep streaming continuously")

	// Values flags.
	vfs := logsValuesCmd.Flags()
	vfs.String("stream", "", "Stream name (required)")
	vfs.String("field", "", "Field name to get values for (required)")
	vfs.String("prefix", "", "Filter values by prefix")
	vfs.String("start", "24h", "Start time as Go duration")
	vfs.String("end", "now", "End time as Go duration or 'now'")

	// History flags.
	hfs := logsHistoryCmd.Flags()
	hfs.Int64("limit", 50, "Maximum number of history entries")
}

// cmdFlagStr reads a string flag value from a cobra command.
func cmdFlagStr(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

// cmdFlagBool reads a bool flag value from a cobra command.
func cmdFlagBool(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

// cmdFlagInt64 reads an int64 flag value from a cobra command.
func cmdFlagInt64(cmd *cobra.Command, name string) int64 {
	v, _ := cmd.Flags().GetInt64(name)
	return v
}

// resolveSearchTime resolves --start and --end durations from cmd flags to microseconds.
func resolveSearchTime(cmd *cobra.Command) (startUs int64, endUs int64, err error) {
	return api.ResolveTime(cmdFlagStr(cmd, "start"), cmdFlagStr(cmd, "end"))
}

// resolveClient resolves the auth context and creates an API client.
func resolveClient() (*api.Client, error) {
	ctx, err := resolveContext()
	if err != nil {
		return nil, err
	}

	return api.NewClient(ctx, cfg.Timeout)
}

// buildSearchQuery builds a SQL query string from cmd flags.
func buildSearchQuery(cmd *cobra.Command) string {
	sql := cmdFlagStr(cmd, "sql")
	filter := cmdFlagStr(cmd, "filter")

	if sql != "" {
		return sql
	}
	if filter != "" {
		return filter
	}
	return "SELECT *"
}

// buildSearchRequest constructs a SearchRequest from user flags.
func buildSearchRequest(sql string, stream string, startUs int64, endUs int64, from int64, size int64, trackTotal bool, quickMode bool) api.SearchRequest {
	q := buildSQLWithStream(sql, stream)

	return api.SearchRequest{
		Query: api.SearchQuery{
			SQL:            q,
			StartTime:      startUs,
			EndTime:        endUs,
			From:           from,
			Size:           size,
			TrackTotalHits: trackTotal,
			QuickMode:      quickMode,
			QueryType:      "sql",
		},
		UseCache: true,
	}
}

// printDryRun prints the resolved request without executing.
func printDryRun(cli *api.Client, method string, path string, body any, startUs int64, endUs int64) error {
	data, _ := json.Marshal(body)
	req := output.DryRunRequest{
		Method: method,
		URL:    cli.BaseURL + "/" + cli.Org + "/" + path,
		Headers: map[string]string{
			"Authorization": "Basic <token>",
			"Content-Type":  "application/json",
		},
		Body: json.RawMessage(data),
	}
	return output.WriteDryRun(outWriter.Stdout(), req, startUs, endUs)
}

func runLogsSearch(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	startUs, endUs, err := resolveSearchTime(cmd)
	if err != nil {
		return err
	}

	sql := buildSearchQuery(cmd)
	stream := cmdFlagStr(cmd, "stream")
	from, _ := cmd.Flags().GetInt64("from")
	size, _ := cmd.Flags().GetInt64("size")
	trackTotal, _ := cmd.Flags().GetBool("track-total")
	quickMode, _ := cmd.Flags().GetBool("quick-mode")

	// Build the request.
	reqBody := buildSearchRequest(sql, stream, startUs, endUs, from, size, trackTotal, quickMode)

	if cfg.DryRun {
		return printDryRun(cli, "POST", "_search", reqBody, startUs, endUs)
	}

	resp, err := cli.Search(cmd.Context(), reqBody)
	if err != nil {
		return err
	}

	// Route output based on --format.
	switch cfg.Format {
	case "table":
		// Table output: render hits with columns from response.
		return renderHitsTable(resp)
	case "log":
		// Human-readable log lines.
		return outWriter.WriteLogs(resp.Hits)
	default:
		return outWriter.WriteSearchMeta(resp, startUs, endUs)
	}
}

// renderHitsTable renders search hits as a table using the response columns.
func renderHitsTable(resp *api.SearchResponse) error {
	if len(resp.Hits) == 0 {
		fmt.Fprintln(outWriter.Stdout(), "(no results)")
		return nil
	}
	tw := output.NewTable(outWriter.Stdout(), resp.Columns...)
	for _, raw := range resp.Hits {
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		row := make([]string, len(resp.Columns))
		for i, col := range resp.Columns {
			if v, ok := obj[col]; ok {
				row[i] = formatJSONField(v)
			}
		}
		tw.Row(row...)
	}
	return tw.Flush()
}

// formatJSONField formats a JSON value for table display.
func formatJSONField(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%v", x)
	case bool:
		return fmt.Sprintf("%v", x)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", x)
	}
}

func runLogsStream(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	startUs, endUs, err := resolveSearchTime(cmd)
	if err != nil {
		return err
	}

	sql := buildSearchQuery(cmd)
	stream := cmdFlagStr(cmd, "stream")
	follow := cmdFlagBool(cmd, "follow")

	// Build streaming request.
	streamReq := func(start int64, end int64) api.SearchStreamRequest {
		return api.SearchStreamRequest{
			Query: api.SearchQuery{
				SQL:             buildSQLWithStream(sql, stream),
				StartTime:       start,
				EndTime:         end,
				QuickMode:       cmdFlagBool(cmd, "quick-mode"),
				StreamingOutput: true,
			},
		}
	}(startUs, endUs)

	if cfg.DryRun {
		body, _ := json.Marshal(streamReq)
		return output.WriteDryRun(cmd.OutOrStdout(), output.DryRunRequest{
			Method: "POST",
			URL:    cli.BaseURL + "/" + cli.Org + "/_search_stream",
			Headers: map[string]string{
				"Authorization": "Basic <token>",
				"Content-Type":  "application/json",
				"Accept":        "application/x-ndjson",
			},
			Body: json.RawMessage(body),
		}, startUs, endUs)
	}

	ctx := cmd.Context()
	followCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Print TTY indicator on stderr.
	isTTY := outWriter.IsTerminal()
	if isTTY && !cfg.Quiet {
		fmt.Fprintln(os.Stderr, "● streaming...")
	}

	// Create a pipe for the scanner goroutine.
	r, w, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("creating pipe: %w", err)
	}

	// Scanner goroutine: reads NDJSON from pipe, writes to stdout.
	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(r)
		scanner.Buffer(nil, 1024*1024) // 1MB buffer for large log entries
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) > 0 {
				os.Stdout.Write(line)
				os.Stdout.Write([]byte("\n"))
			}
		}
		r.Close()
	}()

	pw := &pipeWriter{w: w}

	if follow {
		err = runFollowMode(cli, followCtx, streamReq, pw)
	} else {
		err = streamWithWriter(cli, followCtx, streamReq, pw)
	}

	// Always close the pipe writer so the scanner goroutine unblocks.
	// Ignore close error — we only care that the write end is signalled closed.
	_ = pw.Close()
	w.Close()
	<-done

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}

	return nil
}

// runFollowMode polls the search endpoint every 5 seconds, advancing the
// start timestamp to the last seen record so new entries are captured.
func runFollowMode(cli *api.Client, ctx context.Context, firstReq api.SearchStreamRequest, w streamWriter) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// lastEnd tracks the upper bound of the next query window.
	// Initialise to the first request's end so the first poll query is a
	// small window from lastEnd to now.
	lastEnd := firstReq.Query.EndTime

	// Run the initial request immediately.
	if err := streamWithWriter(cli, ctx, firstReq, w); err != nil {
		return err
	}

	for {
		select {
		case <-ticker.C:
			// no-op: select exists only to receive ctx.Done or tick
		case <-ctx.Done():
			return ctx.Err()
		}

		now := time.Now().UnixMicro()
		req := api.SearchStreamRequest{
			Query: api.SearchQuery{
				SQL:             firstReq.Query.SQL,
				StartTime:       lastEnd,
				EndTime:         now,
				Size:            firstReq.Query.Size,
				QuickMode:       firstReq.Query.QuickMode,
				StreamingOutput: true,
			},
		}

		if err := streamWithWriter(cli, ctx, req, w); err != nil {
			return err
		}

		// Advance lastEnd to now so subsequent queries don't re-emit the
		// same window. Even if no new records arrived, querying from 'now'
		// to 'future now' ensures we capture whatever arrives next.
		lastEnd = time.Now().UnixMicro()
	}
}

// buildSQLWithStream prepends "FROM <stream>" to the SQL if not already present.
func buildSQLWithStream(sql string, stream string) string {
	if stream == "" {
		return sql
	}
	// Inject FROM only for bare "SELECT *" (with optional trailing whitespace).
	// Any SQL containing FROM is assumed complete.
	upper := strings.ToUpper(strings.TrimSpace(sql))
	upper = strings.TrimSuffix(upper, ";") // strip trailing semicolon
	if strings.HasPrefix(upper, "SELECT *") && !strings.Contains(upper, "FROM") {
		return "SELECT * FROM \"" + stream + "\""
	}
	return sql
}

// pipeWriter adapts an os.File to the streamWriter interface.
type pipeWriter struct {
	w *os.File
}

func (p *pipeWriter) WriteHit(h api.SearchStreamResponse) error {
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	_, err = p.w.Write(data)
	return err
}

func (p *pipeWriter) Close() error { return p.w.Close() }

// streamWriter receives streaming results.
type streamWriter interface {
	WriteHit(api.SearchStreamResponse) error
	io.Closer
}

func streamWithWriter(cli *api.Client, ctx context.Context, req api.SearchStreamRequest, w streamWriter) error {
	req.Query.StreamingOutput = true

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cli.BaseURL+"/"+cli.Org+"/_search_stream", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Basic "+cli.Token)
	httpReq.Header.Set("Accept", "application/x-ndjson")

	resp, err := cli.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("streaming request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return api.NewHTTPError(resp.StatusCode, body)
	}

	dec := json.NewDecoder(resp.Body)
	for dec.More() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var hit api.SearchStreamResponse
		if err := dec.Decode(&hit); err != nil {
			return fmt.Errorf("decoding stream record: %w", err)
		}
		if err := w.WriteHit(hit); err != nil {
			return err
		}
		if hit.IsLast {
			break
		}
	}
	return nil
}

func runLogsValues(cmd *cobra.Command, _ []string) error {
	stream := cmdFlagStr(cmd, "stream")
	field := cmdFlagStr(cmd, "field")
	if stream == "" || field == "" {
		return fmt.Errorf("--stream and --field are required")
	}

	cli, err := resolveClient()
	if err != nil {
		return err
	}

	prefix := cmdFlagStr(cmd, "prefix")

	startUs, endUs, err := resolveSearchTime(cmd)
	if err != nil {
		return err
	}

	// Use GET endpoint for simple field values.
	resp, err := cli.FieldValues(cmd.Context(), stream, field, startUs, endUs)
	if err != nil {
		return err
	}

	// Filter by prefix.
	vals := resp.Values
	if prefix != "" {
		var filtered []string
		for _, v := range vals {
			if strings.HasPrefix(v, prefix) {
				filtered = append(filtered, v)
			}
		}
		vals = filtered
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "Value")
		for _, v := range vals {
			tw.Row(v)
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(vals)
}

func runLogsHistory(cmd *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	limit := cmdFlagInt64(cmd, "limit")
	resp, err := cli.SearchHistory(cmd.Context(), limit)
	if err != nil {
		return err
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "Stream", "Type", "SQL", "Created At")
		for _, h := range resp.Hits {
			tw.Row(h.StreamName, h.StreamType, h.SQL, time.Unix(h.CreatedAt, 0).Format(time.RFC3339))
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(resp.Hits)
}

func runLogsViewsList(_ *cobra.Command, _ []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	resp, err := cli.Views(context.Background())
	if err != nil {
		return err
	}

	if cfg.Format == "table" {
		tw := output.NewTable(outWriter.Stdout(), "ID", "Name", "Stream", "SQL")
		for _, v := range resp.Views {
			sql := v.SQL
			if len(sql) > 60 {
				sql = sql[:57] + "..."
			}
			tw.Row(v.ID, v.Name, v.Stream, sql)
		}
		return tw.Flush()
	}

	return outWriter.WriteJSON(resp.Views)
}

func runLogsViewsGet(cmd *cobra.Command, args []string) error {
	cli, err := resolveClient()
	if err != nil {
		return err
	}

	resp, err := cli.View(context.Background(), args[0])
	if err != nil {
		return err
	}

	return outWriter.WriteJSON(resp.View)
}
