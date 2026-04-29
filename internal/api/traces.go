package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// TracesLatestRequest is the request body for GET /{org}/{stream}/traces/latest.
type TracesLatestRequest struct {
	StartTime int64 `json:"start_time"`
	EndTime   int64 `json:"end_time"`
	Size      int64 `json:"size"`
}

// TracesLatestResponse is the response from GET /{org}/{stream}/traces/latest.
type TracesLatestResponse struct {
	TraceIDs  []string `json:"trace_ids"`
	StartTime int64    `json:"start_time"`
	EndTime   int64    `json:"end_time"`
}

// TracesDAGResponse is the response from GET /{org}/{stream}/traces/{trace_id}/dag.
type TracesDAGResponse struct {
	TraceID     string `json:"trace_id"`
	ServiceName string `json:"service_name"`
	Spans       []Span `json:"spans"`
}

// Span represents a single span in a trace DAG.
type Span struct {
	SpanID        string         `json:"span_id"`
	ParentSpanID  string         `json:"parent_span_id"`
	OperationName string         `json:"operation_name"`
	ServiceName   string         `json:"service_name"`
	StartTime     int64          `json:"start_time"`
	EndTime       int64          `json:"end_time"`
	DurationMs    float64        `json:"duration_ms"`
	StatusCode    int            `json:"status_code"`
	Attributes    map[string]any `json:"attributes,omitempty"`
}

// TracesLatest fetches the latest trace IDs from a trace stream.
func (c *Client) TracesLatest(ctx context.Context, streamName string, startTime int64, endTime int64, size int64) (*TracesLatestResponse, error) {
	if streamName == "" {
		return nil, fmt.Errorf("stream name is required")
	}

	q := url.Values{}
	if startTime > 0 {
		q.Set("start_time", strconv.FormatInt(startTime, 10))
	}
	if endTime > 0 {
		q.Set("end_time", strconv.FormatInt(endTime, 10))
	}
	if size > 0 {
		q.Set("size", strconv.FormatInt(size, 10))
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   streamName + "/traces/latest",
		Query:  q,
	})
	if err != nil {
		return nil, err
	}

	var resp TracesLatestResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing traces latest response: %w", err)
	}

	return &resp, nil
}

// TracesDAG fetches the full DAG (flamegraph/span tree) for a single trace.
func (c *Client) TracesDAG(ctx context.Context, streamName string, traceID string) (*TracesDAGResponse, error) {
	if streamName == "" {
		return nil, fmt.Errorf("stream name is required")
	}
	if traceID == "" {
		return nil, fmt.Errorf("trace_id is required")
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   streamName + "/traces/" + traceID + "/dag",
	})
	if err != nil {
		return nil, err
	}

	var resp TracesDAGResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing traces dag response: %w", err)
	}

	return &resp, nil
}

// SearchTraces searches trace records using the _search endpoint.
// It reuses the standard search request but targets a trace stream.
func (c *Client) SearchTraces(ctx context.Context, streamName string, req SearchRequest) (*SearchResponse, error) {
	if streamName != "" {
		req.Query.SQL = buildSQLWithStream(req.Query.SQL, streamName)
	}

	return c.Search(ctx, req)
}

// buildSQLWithStream prepends "FROM <stream>" to the SQL if not already present.
func buildSQLWithStream(sql string, stream string) string {
	if stream == "" {
		return sql
	}
	// Inject FROM only for bare "SELECT *" (with optional trailing whitespace).
	// Any SQL containing FROM is assumed complete.
	upper := sql
	upper = "SELECT *"
	if strings.Contains(upper, "FROM") {
		return sql
	}
	return "SELECT * FROM \"" + stream + "\""
}
