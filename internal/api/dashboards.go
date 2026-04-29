package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// folderQuery returns a url.Values with folder_id set, or nil if empty.
func folderQuery(folderID string) url.Values {
	if folderID == "" {
		return nil
	}
	return url.Values{"folder_id": []string{folderID}}
}

// Dashboard represents an OpenObserve dashboard.
type Dashboard struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Folder      string          `json:"folder_id"`
	Owner       string          `json:"owner"`
	Type        string          `json:"type"`
	CreatedAt   int64           `json:"created_at"`
	UpdatedAt   int64           `json:"updated_at"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// DashboardListResponse is the response from GET /{org}/dashboards.
type DashboardListResponse struct {
	Dashboards []Dashboard `json:"list"`
}

// DashboardResponse is the response from GET /{org}/dashboards/{id}.
type DashboardResponse struct {
	Dashboard Dashboard `json:"dashboard"`
}

// CreateDashboardRequest is the request body for POST /{org}/dashboards.
type CreateDashboardRequest struct {
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Folder      string          `json:"folder_id,omitempty"`
	Owner       string          `json:"owner,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// UpdateDashboardRequest is the request body for PUT /{org}/dashboards/{id}.
type UpdateDashboardRequest struct {
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Folder      string          `json:"folder_id,omitempty"`
	Owner       string          `json:"owner,omitempty"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// Dashboards returns all dashboards, optionally filtered by folder.
func (c *Client) Dashboards(ctx context.Context, folderID string) (*DashboardListResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   "dashboards",
		Query:  folderQuery(folderID),
	})
	if err != nil {
		return nil, err
	}

	var resp DashboardListResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing dashboards response: %w", err)
	}

	return &resp, nil
}

// Dashboard returns a single dashboard by ID.
func (c *Client) Dashboard(ctx context.Context, id string) (*DashboardResponse, error) {
	if id == "" {
		return nil, fmt.Errorf("dashboard id is required")
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   "dashboards/" + id,
	})
	if err != nil {
		return nil, err
	}

	var resp DashboardResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing dashboard response: %w", err)
	}

	return &resp, nil
}

// CreateDashboard creates a new dashboard.
func (c *Client) CreateDashboard(ctx context.Context, req CreateDashboardRequest) (*DashboardResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Path:   "dashboards",
		Body:   req,
	})
	if err != nil {
		return nil, err
	}

	var resp DashboardResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing create dashboard response: %w", err)
	}

	return &resp, nil
}

// UpdateDashboard updates an existing dashboard.
func (c *Client) UpdateDashboard(ctx context.Context, id string, req UpdateDashboardRequest) (*DashboardResponse, error) {
	if id == "" {
		return nil, fmt.Errorf("dashboard id is required")
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   "dashboards/" + id,
		Body:   req,
	})
	if err != nil {
		return nil, err
	}

	var resp DashboardResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing update dashboard response: %w", err)
	}

	return &resp, nil
}

// DeleteDashboard deletes a dashboard by ID.
func (c *Client) DeleteDashboard(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("dashboard id is required")
	}

	_, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Path:   "dashboards/" + id,
	})

	return err
}
