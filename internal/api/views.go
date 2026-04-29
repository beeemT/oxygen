package api

import (
	"context"
	"net/http"
)

// SavedView represents a saved view.
type SavedView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Stream    string `json:"stream"`
	SQL       string `json:"sql"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// SavedViewsResponse is the response from GET /{org}/savedviews.
type SavedViewsResponse struct {
	Views []SavedView `json:"list"`
}

// SavedViewResponse wraps a single saved view.
type SavedViewResponse struct {
	View SavedView `json:"savedview"`
}

// CreateSavedViewRequest is the request body for POST /{org}/savedviews.
type CreateSavedViewRequest struct {
	Name   string `json:"name"`
	Stream string `json:"stream"`
	SQL    string `json:"sql"`
}

// Views returns all saved views.
func (c *Client) Views(ctx context.Context) (*SavedViewsResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   "savedviews",
	})
	if err != nil {
		return nil, err
	}

	var resp SavedViewsResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// View returns a single saved view by ID.
func (c *Client) View(ctx context.Context, viewID string) (*SavedViewResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   "savedviews/" + viewID,
	})
	if err != nil {
		return nil, err
	}

	var resp SavedViewResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
