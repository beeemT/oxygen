package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/beeemt/oxygen/internal/auth"
)

// Client is the shared HTTP client used by all API calls.
type Client struct {
	httpClient *http.Client
	baseURL    string // e.g. "https://o2.example.com/api"
	org        string
	token      string // Basic auth credential
}

// NewClient creates an API client for the given auth context.
func NewClient(ctx *auth.Context, timeout time.Duration) *Client {
	baseURL := ctx.URL + "/api"
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // don't follow redirects with POST bodies
			},
		},
		baseURL: baseURL,
		org:     ctx.Org,
		token:   ctx.Token,
	}
}

// Request wraps an HTTP request with auth headers and optional JSON body.
// Org is the organization segment; leave empty for root-level endpoints
// (/auth/login, /organizations).
type Request struct {
	Method  string
	Org     string // overrides client.org if non-empty
	Path    string // relative to /api/{org}/
	Query   url.Values
	Body    any
	Token   string // overrides client token if non-empty
	Timeout time.Duration
}

// Do executes the request and returns the raw response body.
// It handles retry for safe (idempotent) HTTP methods and never retries POST.
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	body, err := c.doWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}
	return &Response{Body: body}, nil
}

func (c *Client) doWithRetry(ctx context.Context, req Request) ([]byte, error) {
	token := req.Token
	if token == "" {
		token = c.token
	}

	org := req.Org
	if org == "" {
		org = c.org
	}

	// Build URL.
	path := orgPath(org, req.Path)
	u := c.baseURL + "/" + path
	if req.Query != nil {
		u += "?" + req.Query.Encode()
	}

	// Encode body.
	var bodyReader io.Reader
	contentType := ""
	if req.Body != nil {
		data, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
		contentType = "application/json"
	}

	timeout := req.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// Determine if we should retry.
	safe := req.Method == http.MethodGet ||
		req.Method == http.MethodHead ||
		req.Method == http.MethodDelete

	if safe {
		bo := backoff.WithContext(backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 3), ctx)

		type opResult struct {
			body       []byte
			statusCode int
			err        error
		}

		op := func() (opResult, error) {
			httpReq, err := http.NewRequestWithContext(ctx, req.Method, u, bodyReader)
			if err != nil {
				return opResult{}, fmt.Errorf("building request: %w", err)
			}
			if contentType != "" {
				httpReq.Header.Set("Content-Type", contentType)
			}
			httpReq.Header.Set("Authorization", "Basic "+token)
			httpReq.Header.Set("Accept", "application/json")

			resp, err := c.httpClient.Do(httpReq)
			if err != nil {
				return opResult{}, err
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			return opResult{body: body, statusCode: resp.StatusCode}, nil
		}

		r, err := backoff.RetryWithData[opResult](op, bo)
		if err != nil {
			return nil, wrapError(err, -1)
		}
		if r.statusCode >= 400 {
			return nil, wrapError(newHTTPError(r.statusCode, r.body), r.statusCode)
		}
		return r.body, nil
	}

	// No retry for POST/PUT/PATCH.
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, u, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	httpReq.Header.Set("Authorization", "Basic "+token)
	httpReq.Header.Set("Accept", "application/json")

	_ = timeout // TODO: respect per-request timeout via context deadline.

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, wrapError(err, -1)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, wrapError(newHTTPError(resp.StatusCode, body), resp.StatusCode)
	}
	return body, nil
}

// orgPath returns the path segment with org, unless the path is org-less.
func orgPath(org, path string) string {
	// Endpoints that have no org segment.
	switch path {
	case "auth/login", "auth/logout", "organizations", "":
		return path
	}
	if org == "" {
		return path
	}
	return org + "/" + path
}

// Response is the raw JSON response body.
type Response struct {
	Body []byte
}

// Parse unmarshals the response body into v.
func (r *Response) Parse(v any) error {
	if len(r.Body) == 0 {
		return nil
	}
	return json.Unmarshal(r.Body, v)
}

// HTTPError represents an API error with its status code.
type HTTPError struct {
	StatusCode int
	Message    string
	Body       []byte
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("API error (%d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("API error (%d)", e.StatusCode)
}

func newHTTPError(status int, body []byte) *HTTPError {
	var msg string
	if len(body) > 0 {
		var parsed struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &parsed) == nil && parsed.Message != "" {
			msg = parsed.Message
		}
	}
	return &HTTPError{StatusCode: status, Message: msg, Body: body}
}

func wrapError(err error, status int) error {
	if err == nil {
		return nil
	}
	if httpErr, ok := err.(*HTTPError); ok {
		return httpErr
	}
	return fmt.Errorf("request failed: %w", err)
}

// ExitCode maps an HTTP status code to the CLI exit code.
func ExitCode(status int) int {
	switch {
	case status >= 200 && status < 300:
		return 0
	case status == 400 || status == 422:
		return 5
	case status == 401:
		return 2
	case status == 403:
		return 3
	case status == 404:
		return 4
	case status == 429:
		return 6
	case status >= 500:
		return 7
	default:
		return 1
	}
}

// BasicAuth builds a Basic auth credential string from email and password.
func BasicAuth(email, password string) string {
	creds := base64.StdEncoding.EncodeToString([]byte(email + ":" + password))
	return creds
}

// LoginRequest is the JSON payload for POST /auth/login.
type LoginRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// LoginResponse is the JSON response from POST /auth/login.
type LoginResponse struct {
	Status  bool   `json:"status"`
	Message string `json:"message"`
}
