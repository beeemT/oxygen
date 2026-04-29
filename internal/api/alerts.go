package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// queryVal returns url.Values with k=v set, or nil if v is empty.
func queryVal(k, v string) url.Values {
	if v == "" {
		return nil
	}
	return url.Values{k: []string{v}}
}

// Alert represents an OpenObserve v2 alert.
type Alert struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	StreamName  string          `json:"stream_name"`
	Query       string          `json:"query"`
	Condition   AlertCondition  `json:"condition"`
	Duration    int             `json:"duration"`
	Threshold   json.RawMessage `json:"threshold,omitempty"`
	Status      string          `json:"status"`
	IsEnabled   bool            `json:"is_enabled"`
	Owner       string          `json:"owner,omitempty"`
	Description string          `json:"description,omitempty"`
	CreatedAt   int64           `json:"created_at"`
	UpdatedAt   int64           `json:"updated_at"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// AlertCondition defines the trigger condition for an alert.
type AlertCondition struct {
	Column   string `json:"column,omitempty"`
	Operator string `json:"operator,omitempty"`
	Value    any    `json:"value,omitempty"`
}

// AlertListResponse is the response from GET /v2/{org}/alerts.
type AlertListResponse struct {
	Alerts []Alert `json:"list"`
}

// AlertResponse is the response from GET /v2/{org}/alerts/{id}.
type AlertResponse struct {
	Alert Alert `json:"alert"`
}

// CreateAlertRequest is the request body for POST /v2/{org}/alerts.
type CreateAlertRequest struct {
	Name        string          `json:"name"`
	StreamName  string          `json:"stream_name"`
	Query       string          `json:"query"`
	Condition   AlertCondition  `json:"condition"`
	Duration    int             `json:"duration"`
	Threshold   json.RawMessage `json:"threshold,omitempty"`
	IsEnabled   bool            `json:"is_enabled"`
	Owner       string          `json:"owner,omitempty"`
	Description string          `json:"description,omitempty"`
}

// UpdateAlertRequest is the request body for PUT /v2/{org}/alerts/{id}.
type UpdateAlertRequest struct {
	Name        string          `json:"name,omitempty"`
	StreamName  string          `json:"stream_name,omitempty"`
	Query       string          `json:"query,omitempty"`
	Condition   AlertCondition  `json:"condition,omitempty"`
	Duration    int             `json:"duration,omitempty"`
	Threshold   json.RawMessage `json:"threshold,omitempty"`
	IsEnabled   *bool           `json:"is_enabled,omitempty"`
	Owner       string          `json:"owner,omitempty"`
	Description string          `json:"description,omitempty"`
}

// AlertHistoryEntry is a single entry in the alert firing history.
type AlertHistoryEntry struct {
	AlertID    string `json:"alert_id"`
	AlertName  string `json:"alert_name"`
	Stream     string `json:"stream"`
	StartTime  int64  `json:"start_time"`
	EndTime    int64  `json:"end_time"`
	Status     string `json:"status"`
	Severity   string `json:"severity"`
	FiredAt    int64  `json:"fired_at"`
	ResolvedAt int64  `json:"resolved_at,omitempty"`
}

// AlertHistoryResponse is the response from GET /v2/{org}/alerts/history.
type AlertHistoryResponse struct {
	Alerts []AlertHistoryEntry `json:"list"`
}

// Incident represents a firing alert incident.
type Incident struct {
	ID             string `json:"id"`
	AlertID        string `json:"alert_id"`
	AlertName      string `json:"alert_name"`
	Status         string `json:"status"`
	FiredAt        int64  `json:"fired_at"`
	ResolvedAt     int64  `json:"resolved_at,omitempty"`
	AcknowledgedAt int64  `json:"acknowledged_at,omitempty"`
}

// IncidentListResponse is the response from GET /v2/{org}/alerts/incidents.
type IncidentListResponse struct {
	Incidents []Incident `json:"list"`
}

// AlertTemplate represents an alert template.
type AlertTemplate struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Payload     json.RawMessage `json:"payload,omitempty"`
}

// AlertTemplateListResponse is the response from GET /v2/{org}/alerts/templates.
type AlertTemplateListResponse struct {
	Templates []AlertTemplate `json:"list"`
}

// Alerts returns all v2 alerts, optionally filtered by status.
func (c *Client) Alerts(ctx context.Context, status string) (*AlertListResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Org:    "v2/" + c.Org,
		Path:   "alerts",
		Query:  queryVal("status", status),
	})
	if err != nil {
		return nil, err
	}

	var resp AlertListResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing alerts response: %w", err)
	}

	return &resp, nil
}

// Alert returns a single v2 alert by ID.
func (c *Client) Alert(ctx context.Context, id string) (*AlertResponse, error) {
	if id == "" {
		return nil, fmt.Errorf("alert id is required")
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Org:    "v2/" + c.Org,
		Path:   "alerts/" + id,
	})
	if err != nil {
		return nil, err
	}

	var resp AlertResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing alert response: %w", err)
	}

	return &resp, nil
}

// CreateAlert creates a new v2 alert.
func (c *Client) CreateAlert(ctx context.Context, req CreateAlertRequest) (*AlertResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodPost,
		Org:    "v2/" + c.Org,
		Path:   "alerts",
		Body:   req,
	})
	if err != nil {
		return nil, err
	}

	var resp AlertResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing create alert response: %w", err)
	}

	return &resp, nil
}

// UpdateAlert updates an existing v2 alert.
func (c *Client) UpdateAlert(ctx context.Context, id string, req UpdateAlertRequest) (*AlertResponse, error) {
	if id == "" {
		return nil, fmt.Errorf("alert id is required")
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodPut,
		Org:    "v2/" + c.Org,
		Path:   "alerts/" + id,
		Body:   req,
	})
	if err != nil {
		return nil, err
	}

	var resp AlertResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing update alert response: %w", err)
	}

	return &resp, nil
}

// DeleteAlert deletes a v2 alert by ID.
func (c *Client) DeleteAlert(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("alert id is required")
	}

	_, err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		Org:    "v2/" + c.Org,
		Path:   "alerts/" + id,
	})

	return err
}

// TriggerAlert manually triggers a v2 alert.
func (c *Client) TriggerAlert(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("alert id is required")
	}

	_, err := c.Do(ctx, Request{
		Method: http.MethodPatch,
		Org:    "v2/" + c.Org,
		Path:   "alerts/" + id + "/trigger",
	})

	return err
}

// AlertHistory returns the firing history for v2 alerts.
func (c *Client) AlertHistory(ctx context.Context, alertID string, limit int64) (*AlertHistoryResponse, error) {
	q := url.Values{}
	if alertID != "" {
		q.Set("alert_id", alertID)
	}
	if limit > 0 {
		q.Set("limit", strconv.FormatInt(limit, 10))
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Org:    "v2/" + c.Org,
		Path:   "alerts/history",
		Query:  q,
	})
	if err != nil {
		return nil, err
	}

	var resp AlertHistoryResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing alert history response: %w", err)
	}

	return &resp, nil
}

// AlertIncidents returns the list of firing incidents for v2 alerts.
func (c *Client) AlertIncidents(ctx context.Context, limit int64) (*IncidentListResponse, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.FormatInt(limit, 10))
	}

	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Org:    "v2/" + c.Org,
		Path:   "alerts/incidents",
		Query:  q,
	})
	if err != nil {
		return nil, err
	}

	var resp IncidentListResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing alert incidents response: %w", err)
	}

	return &resp, nil
}

// AlertTemplates returns all alert templates.
func (c *Client) AlertTemplates(ctx context.Context) (*AlertTemplateListResponse, error) {
	apiResp, err := c.Do(ctx, Request{
		Method: http.MethodGet,
		Org:    "v2/" + c.Org,
		Path:   "alerts/templates",
	})
	if err != nil {
		return nil, err
	}

	var resp AlertTemplateListResponse
	if err := apiResp.Parse(&resp); err != nil {
		return nil, fmt.Errorf("parsing alert templates response: %w", err)
	}

	return &resp, nil
}
