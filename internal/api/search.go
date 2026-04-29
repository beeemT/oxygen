package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SearchRequest is the request body for POST /{org}/_search.
type SearchRequest struct {
	Query      SearchQuery `json:"query"`
	Encoding   string      `json:"encoding,omitempty"`
	Regions    []string    `json:"regions,omitempty"`
	Clusters   []string    `json:"clusters,omitempty"`
	Timeout    int64       `json:"timeout,omitempty"`
	UseCache   bool        `json:"use_cache,omitempty"`
	ClearCache bool        `json:"clear_cache,omitempty"`
}

// SearchQuery contains the core query parameters.
type SearchQuery struct {
	SQL               string `json:"sql"`
	From              int64  `json:"from,omitempty"`
	Size              int64  `json:"size,omitempty"`
	StartTime         int64  `json:"start_time"` // microseconds Unix epoch
	EndTime           int64  `json:"end_time"`   // microseconds Unix epoch
	TrackTotalHits    bool   `json:"track_total_hits,omitempty"`
	QuickMode         bool   `json:"quick_mode,omitempty"`
	QueryType         string `json:"query_type,omitempty"`
	QueryFn           string `json:"query_fn,omitempty"`
	StreamingOutput   bool   `json:"streaming_output,omitempty"`
	HistogramInterval int64  `json:"histogram_interval,omitempty"`
}

// SearchResponse is the JSON response from POST /{org}/_search.
type SearchResponse struct {
	Took         int               `json:"took"`
	TookDetail   TookDetail        `json:"took_detail"`
	Columns      []string          `json:"columns"`
	Hits         []json.RawMessage `json:"hits"`
	Total        uint64            `json:"total"`
	From         int64             `json:"from"`
	Size         int64             `json:"size"`
	ScanSize     uint64            `json:"scan_size"`
	CachedRatio  uint64            `json:"cached_ratio"`
	IsPartial    bool              `json:"is_partial"`
	TraceID      string            `json:"trace_id"`
	FnErrors     []string          `json:"function_error"`
	NewStartTime *int64            `json:"new_start_time,omitempty"`
	NewEndTime   *int64            `json:"new_end_time,omitempty"`
}

// TookDetail breaks down query timing stages.
type TookDetail struct {
	Total        int `json:"total"`
	CacheTook    int `json:"cache_took"`
	FileListTook int `json:"file_list_took"`
	WaitInQueue  int `json:"wait_in_queue"`
	IdxTook      int `json:"idx_took"`
	SearchTook   int `json:"search_took"`
}

// SearchMeta wraps the API response with resolved time range.
type SearchMeta struct {
	Hits              []json.RawMessage `json:"hits"`
	Total             uint64            `json:"total"`
	TookMs            int               `json:"took_ms"`
	ResolvedStart     time.Time         `json:"resolved_start"`
	ResolvedEnd       time.Time         `json:"resolved_end"`
	ResolvedStartTime int64             `json:"resolved_start_time"`
	ResolvedEndTime   int64             `json:"resolved_end_time"`
}

// SearchMultiRequest is the request body for POST /{org}/_search_multi.
type SearchMultiRequest struct {
	Streams  []string    `json:"streams"`
	Query    SearchQuery `json:"query"`
	Encoding string      `json:"encoding,omitempty"`
	Timeout  int64       `json:"timeout,omitempty"`
	UseCache bool        `json:"use_cache,omitempty"`
}

// SearchMultiResponse is the response from POST /{org}/_search_multi.
type SearchMultiResponse struct {
	Results []SearchResponse `json:"results"`
}

// SearchStreamRequest is the request body for POST /{org}/_search_stream.
type SearchStreamRequest struct {
	Query    SearchQuery `json:"query"`
	Encoding string      `json:"encoding,omitempty"`
	Timeout  int64       `json:"timeout,omitempty"`
}

// SearchStreamResponse is a single NDJSON line from the streaming endpoint.
type SearchStreamResponse struct {
	Columns []string        `json:"columns"`
	Hit     json.RawMessage `json:"record"`
	Total   uint64          `json:"total"`
	IsLast  bool            `json:"is_last"`
	TookMs  int             `json:"took_ms"`
}

// ValuesRequest is the request body for POST /{org}/_values_stream.
type ValuesRequest struct {
	StreamName string `json:"stream_name"`
	FieldName  string `json:"field_name"`
	StartTime  int64  `json:"start_time"` // microseconds
	EndTime    int64  `json:"end_time"`   // microseconds
	Filter     string `json:"filter,omitempty"`
	Size       int64  `json:"size,omitempty"`
}

// ValuesResponse is the response from GET /{org}/{stream}/_values.
type ValuesResponse struct {
	FieldName string   `json:"field_name"`
	Values    []string `json:"values"`
}

// ValuesStreamResponse is a single NDJSON line from the values stream.
type ValuesStreamResponse struct {
	Value  string `json:"value"`
	IsLast bool   `json:"is_last"`
}

// HistoryRequest is the request body for POST /{org}/_search_history.
type HistoryRequest struct {
	Limit  int64 `json:"limit,omitempty"`
	Offset int64 `json:"offset,omitempty"`
}

// HistoryResponse is the response from POST /{org}/_search_history.
type HistoryResponse struct {
	Hits []HistoryEntry `json:"hits"`
}

// HistoryEntry is a single query history entry.
type HistoryEntry struct {
	SQL        string `json:"sql"`
	StreamType string `json:"stream_type"`
	StreamName string `json:"stream"`
	CreatedAt  int64  `json:"created_at"` // Unix seconds
}

// StreamValuesResponse is the response from GET /{org}/{stream}/_values.
type StreamValuesResponse struct {
	FieldName string   `json:"field_name"`
	Values    []string `json:"values"`
}

// Search builds a SearchRequest from user-provided flags.
// sql may be empty if filter is provided.
func (c *Client) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	if req.Query.StartTime == 0 || req.Query.EndTime == 0 {
		return nil, fmt.Errorf("start_time and end_time are required")
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   "_search",
		Body:   req,
	})
	if err != nil {
		return nil, err
	}

	var resp SearchResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing search response: %w", err)
	}

	return &resp, nil
}

// SearchMulti searches across multiple streams.
func (c *Client) SearchMulti(ctx context.Context, req SearchMultiRequest) (*SearchMultiResponse, error) {
	if req.Query.StartTime == 0 || req.Query.EndTime == 0 {
		return nil, fmt.Errorf("start_time and end_time are required")
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   "_search_multi",
		Body:   req,
	})
	if err != nil {
		return nil, err
	}

	var resp SearchMultiResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing search_multi response: %w", err)
	}

	return &resp, nil
}

// SearchStream performs a streaming search and calls onHit for each NDJSON record.
// It returns when the stream is exhausted or ctx is cancelled.
func (c *Client) SearchStream(ctx context.Context, req SearchStreamRequest, onHit func(ValuesStreamResponse) error) error {
	if req.Query.StartTime == 0 || req.Query.EndTime == 0 {
		return fmt.Errorf("start_time and end_time are required")
	}

	token := c.Token
	req.Query.StreamingOutput = true

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encoding request body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/"+orgPath(c.Org, "_search_stream"), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Basic "+token)
	httpReq.Header.Set("Accept", "application/x-ndjson")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("streaming request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return newHTTPError(resp.StatusCode, body)
	}

	dec := json.NewDecoder(resp.Body)
	for dec.More() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var hit ValuesStreamResponse
		if err := dec.Decode(&hit); err != nil {
			return fmt.Errorf("decoding stream record: %w", err)
		}
		if err := onHit(hit); err != nil {
			return err
		}
		if hit.IsLast {
			break
		}
	}

	return nil
}

// StreamValues performs a streaming field value enumeration and calls onValue for each NDJSON record.
func (c *Client) StreamValues(ctx context.Context, req ValuesRequest, onValue func(ValuesStreamResponse) error) error {
	token := c.Token

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encoding request body: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/"+orgPath(c.Org, "_values_stream"), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Basic "+token)
	httpReq.Header.Set("Accept", "application/x-ndjson")

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("values stream request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return newHTTPError(resp.StatusCode, body)
	}

	dec := json.NewDecoder(resp.Body)
	for dec.More() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var v ValuesStreamResponse
		if err := dec.Decode(&v); err != nil {
			return fmt.Errorf("decoding value record: %w", err)
		}
		if err := onValue(v); err != nil {
			return err
		}
		if v.IsLast {
			break
		}
	}

	return nil
}

// FieldValues performs a GET request to /{org}/{stream}/_values.
func (c *Client) FieldValues(ctx context.Context, streamName string, fieldName string, startUs int64, endUs int64) (*StreamValuesResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   streamName + "/_values",
		Query: map[string][]string{
			"field_name": {fieldName},
			"start_time": {fmt.Sprintf("%d", startUs)},
			"end_time":   {fmt.Sprintf("%d", endUs)},
		},
	})
	if err != nil {
		return nil, err
	}

	var resp StreamValuesResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing field values response: %w", err)
	}

	return &resp, nil
}

// SearchHistory returns the query history for the org.
func (c *Client) SearchHistory(ctx context.Context, limit int64) (*HistoryResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   "_search_history",
		Body:   HistoryRequest{Limit: limit},
	})
	if err != nil {
		return nil, err
	}

	var resp HistoryResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing history response: %w", err)
	}

	return &resp, nil
}

// ResolveTime converts a Go duration string to microseconds relative to now.
// "now" or empty returns now. The returned values are start (earlier) and end (later).
func ResolveTime(startDur string, endDur string) (startUs int64, endUs int64, err error) {
	now := time.Now()

	// Resolve end time.
	if endDur == "" || endDur == "now" {
		endUs = now.UnixMicro()
	} else {
		d, err := time.ParseDuration(endDur)
		if err != nil {
			return 0, 0, fmt.Errorf("parsing --end duration %q: %w", endDur, err)
		}
		endUs = now.Add(d).UnixMicro()
	}

	// Resolve start time.
	if startDur == "" || startDur == "now" {
		startUs = endUs // caller must provide --start; degenerate case
	} else {
		d, err := time.ParseDuration(startDur)
		if err != nil {
			return 0, 0, fmt.Errorf("parsing --start duration %q: %w", startDur, err)
		}
		startUs = now.Add(-d).UnixMicro() // subtract: --start=1h means "1h ago"
	}

	return startUs, endUs, nil
}

// BuildSearchRequest constructs a SearchRequest from component parts.
// It does not inject a FROM clause — callers must embed the stream
// name in the SQL themselves.
func BuildSearchRequest(sql string, streamName string, startUs int64, endUs int64, from int64, size int64, queryType string) SearchRequest {
	return SearchRequest{
		Query: SearchQuery{
			SQL:            sql,
			StartTime:      startUs,
			EndTime:        endUs,
			From:           from,
			Size:           size,
			TrackTotalHits: true,
			QueryType:      queryType,
		},
		UseCache: true,
	}
}

// BuildSearchMeta enriches a SearchResponse with resolved time info.
func BuildSearchMeta(resp *SearchResponse, startUs int64, endUs int64) SearchMeta {
	start := time.UnixMicro(startUs)
	end := time.UnixMicro(endUs)
	if resp.NewStartTime != nil {
		start = time.UnixMicro(*resp.NewStartTime)
	}
	if resp.NewEndTime != nil {
		end = time.UnixMicro(*resp.NewEndTime)
	}

	return SearchMeta{
		Hits:              resp.Hits,
		Total:             resp.Total,
		TookMs:            resp.Took,
		ResolvedStart:     start,
		ResolvedEnd:       end,
		ResolvedStartTime: start.UnixMicro(),
		ResolvedEndTime:   end.UnixMicro(),
	}
}
