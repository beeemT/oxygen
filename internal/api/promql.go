package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// PromQLInstantResponse is the response from GET /{org}/prometheus/api/v1/query.
type PromQLInstantResponse struct {
	Status    string     `json:"status"`
	Data      PromQLData `json:"data"`
	ErrorType string     `json:"errorType,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// PromQLData is the data section of a PromQL response.
type PromQLData struct {
	ResultType string              `json:"resultType"`
	Result     []PromQLVectorEntry `json:"result"`
}

// PromQLVectorEntry is a single result in a vector (instant query) result.
type PromQLVectorEntry struct {
	Metric map[string]string `json:"metric"`
	// Vector: value [timestamp, value]
	Value [2]any `json:"value,omitempty"`
}

// PromQLRangeResponse is the response from GET /{org}/prometheus/api/v1/query_range.
type PromQLRangeResponse struct {
	Status string          `json:"status"`
	Data   PromQLRangeData `json:"data"`
}

// PromQLRangeData is the data section of a range query response.
type PromQLRangeData struct {
	ResultType string              `json:"resultType"`
	Result     []PromQLMatrixEntry `json:"result"`
}

// PromQLMatrixEntry is a single series in a matrix (range) result.
type PromQLMatrixEntry struct {
	Metric map[string]string `json:"metric"`
	Values []PromQLSample    `json:"values"`
}

// PromQLSample is a single timestamped value in a range result.
type PromQLSample struct {
	Timestamp float64 `json:"timestamp"`
	Value     float64 `json:"value"`
}

// PromQLSeriesResponse is the response from GET /{org}/prometheus/api/v1/series.
type PromQLSeriesResponse struct {
	Status string              `json:"status"`
	Data   []map[string]string `json:"data"`
}

// PromQLLabelValuesResponse is the response from GET /{org}/prometheus/api/v1/label/{name}/values.
type PromQLLabelValuesResponse struct {
	Status string   `json:"status"`
	Data   []string `json:"data"`
}

// PromQLQuery sends a PromQL instant query.
func (c *Client) PromQLQuery(ctx context.Context, query string, timeStr string) (*PromQLInstantResponse, error) {
	q := url.Values{"query": []string{query}}
	if timeStr != "" {
		q.Set("time", timeStr)
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   "prometheus/api/v1/query",
		Query:  q,
	})
	if err != nil {
		return nil, err
	}

	var resp PromQLInstantResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing promql query response: %w", err)
	}

	return &resp, nil
}

// PromQLQueryRange sends a PromQL range query.
func (c *Client) PromQLQueryRange(ctx context.Context, query string, start string, end string, step string) (*PromQLRangeResponse, error) {
	q := url.Values{
		"query": []string{query},
		"start": []string{start},
		"end":   []string{end},
		"step":  []string{step},
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   "prometheus/api/v1/query_range",
		Query:  q,
	})
	if err != nil {
		return nil, err
	}

	var resp PromQLRangeResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing promql query_range response: %w", err)
	}

	return &resp, nil
}

// PromQLSeries returns series matching the given match selector.
func (c *Client) PromQLSeries(ctx context.Context, match string, start string, end string) (*PromQLSeriesResponse, error) {
	q := url.Values{}
	if match != "" {
		q.Set("match[]", match)
	}
	if start != "" {
		q.Set("start", start)
	}
	if end != "" {
		q.Set("end", end)
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   "prometheus/api/v1/series",
		Query:  q,
	})
	if err != nil {
		return nil, err
	}

	var resp PromQLSeriesResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing promql series response: %w", err)
	}

	return &resp, nil
}

// PromQLLabelValues returns all values for a given label name.
func (c *Client) PromQLLabelValues(ctx context.Context, label string) (*PromQLLabelValuesResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   "prometheus/api/v1/label/" + label + "/values",
	})
	if err != nil {
		return nil, err
	}

	var resp PromQLLabelValuesResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing promql label values response: %w", err)
	}

	return &resp, nil
}

// InstantResult is a single PromQL instant query result.
type InstantResult struct {
	Labels map[string]string
	Value  float64
}

// RangeResult is a single PromQL range query series.
type RangeResult struct {
	Labels  map[string]string
	Samples []Sample
}

// Sample is a timestamped value.
type Sample struct {
	Timestamp int64
	Value     float64
}

// ParsePromQLInstant converts a PromQL instant query response into InstantResult.
func ParsePromQLInstant(resp *PromQLInstantResponse) ([]InstantResult, error) {
	if resp.Status != "success" {
		return nil, fmt.Errorf("promql query failed: %s - %s", resp.ErrorType, resp.Error)
	}

	var results []InstantResult
	for _, entry := range resp.Data.Result {
		if len(entry.Value) < 2 {
			continue
		}
		val, err := parsePromQLValue(entry.Value[1])
		if err != nil {
			continue
		}
		results = append(results, InstantResult{
			Labels: entry.Metric,
			Value:  val,
		})
	}

	return results, nil
}

// ParsePromQLRange converts a PromQL range query response into RangeResult.
func ParsePromQLRange(resp *PromQLRangeResponse) ([]RangeResult, error) {
	if resp.Status != "success" {
		return nil, fmt.Errorf("promql query_range failed")
	}

	var results []RangeResult
	for _, entry := range resp.Data.Result {
		samples := make([]Sample, len(entry.Values))
		for i, v := range entry.Values {
			samples[i] = Sample{
				Timestamp: int64(v.Timestamp),
				Value:     v.Value,
			}
		}
		results = append(results, RangeResult{
			Labels:  entry.Metric,
			Samples: samples,
		})
	}

	return results, nil
}

// parsePromQLValue converts a JSON value (float64, string, or nil) to float64.
func parsePromQLValue(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case string:
		return strconv.ParseFloat(x, 64)
	case nil:
		return 0, fmt.Errorf("null value")
	default:
		return 0, fmt.Errorf("unexpected type %T", v)
	}
}

// ResolvePromQLTime converts a time string to a Unix timestamp string and int64.
// If timeStr is empty or "now", returns now. Accepts unix timestamps, Go duration
// strings, or RFC3339.
func ResolvePromQLTime(timeStr string) (string, int64, error) {
	now := time.Now()
	if timeStr == "" || timeStr == "now" {
		return strconv.FormatInt(now.Unix(), 10), now.Unix(), nil
	}

	// Try parsing as unix timestamp first.
	if ts, err := strconv.ParseInt(timeStr, 10, 64); err == nil {
		return timeStr, ts, nil
	}

	// Try parsing as Go duration string.
	if d, err := time.ParseDuration(timeStr); err == nil {
		t := now.Add(-d)
		return strconv.FormatInt(t.Unix(), 10), t.Unix(), nil
	}

	// Try parsing as RFC3339.
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return strconv.FormatInt(t.Unix(), 10), t.Unix(), nil
	}

	return "", 0, fmt.Errorf("invalid time format: %s (expected unix timestamp, duration like 1h, or RFC3339)", timeStr)
}
