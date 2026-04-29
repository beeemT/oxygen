package api

import (
	"context"
	"net/http"
)

// StreamListResponse is the response from GET /{org}/streams.
type StreamListResponse struct {
	Streams []StreamInfo `json:"list"`
}

// StreamInfo describes a single stream.
type StreamInfo struct {
	StreamType string `json:"stream_type"`
	Name       string `json:"name"`
	DocCount   uint64 `json:"doc_num"`
	StorageMB  uint64 `json:"storage_bytes"`
}

// StreamSchemaResponse is the response from GET /{org}/streams/{name}/schema.
type StreamSchemaResponse struct {
	Name           string        `json:"stream_name"`
	StreamType     string        `json:"stream_type"`
	Fields         []FieldSchema `json:"schema"`
	TotalStorageMB uint64        `json:"total_storage_bytes"`
}

// FieldSchema describes a field within a stream.
type FieldSchema struct {
	Name      string `json:"name"`
	DataType  string `json:"data_type"`
	IndexType string `json:"index_type"`
}

// Streams returns the list of streams in the org.
func (c *Client) Streams(ctx context.Context) (*StreamListResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   "streams",
	})
	if err != nil {
		return nil, err
	}

	var resp StreamListResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// StreamSchema returns the field schema for a given stream.
func (c *Client) StreamSchema(ctx context.Context, streamName string) (*StreamSchemaResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   "streams/" + streamName + "/schema",
	})
	if err != nil {
		return nil, err
	}

	var resp StreamSchemaResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
