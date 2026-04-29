// Package main is the o2 CLI entry point.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/beeemt/oxygen/internal/api"
	"github.com/beeemt/oxygen/internal/auth"
	"github.com/beeemt/oxygen/internal/config"
	"github.com/beeemt/oxygen/internal/output"
)

// authCmd is the "o2 auth" parent command.
var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication contexts",
	Long:  "Login, logout, list, and show the active authentication context.",
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate and store credentials",
	RunE:  runLogin,
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored credentials",
	RunE:  runLogout,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all stored auth contexts",
	RunE:  runAuthList,
}

var currentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the active auth context",
	RunE:  runAuthCurrent,
}

func init() {
	authCmd.AddCommand(loginCmd, logoutCmd, listCmd, currentCmd)

	// o2 auth login flags
	lf := loginCmd.Flags()
	lf.String("url", "", "OpenObserve base URL (required)")
	lf.String("org", "", "Organization ID (required)")
	lf.String("user", "", "User email (required)")
	lf.Duration("timeout", 30*time.Second, "Login request timeout")
	loginCmd.MarkFlagRequired("url")
	loginCmd.MarkFlagRequired("org")
	loginCmd.MarkFlagRequired("user")

	// o2 auth logout flags
	vf := logoutCmd.Flags()
	vf.String("url", "", "OpenObserve base URL (required)")
	vf.String("org", "", "Organization ID (required)")
	vf.String("user", "", "User email")
}

func runLogin(cmd *cobra.Command, _ []string) error {
	url := viper.GetString("url")
	org := viper.GetString("org")
	user := viper.GetString("user")

	// Get password: flag > env var > interactive prompt.
	password := cfg.Password
	if password == "" {
		password = os.Getenv("O2_PASSWORD")
	}
	if password == "" {
		var err error
		password, err = output.PromptPassword("Password: ")
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}
	}

	// Build login URL.
	baseURL := strings.TrimRight(url, "/")
	loginURL := baseURL + "/api/auth/login"

	// Send login request.
	body, _ := json.Marshal(map[string]string{
		"name":     user,
		"password": password,
	})
	ctx, cancel := context.WithTimeout(context.Background(), viper.GetDuration("timeout"))
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	var loginResp struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return fmt.Errorf("decoding login response: %w", err)
	}

	if !loginResp.Status {
		msg := loginResp.Message
		if msg == "" {
			msg = "login failed"
		}
		outWriter.Error(5, msg)
		return fmt.Errorf("%s", msg)
	}

	// Extract Basic auth token from cookie.
	var basicToken string
	for _, c := range resp.Cookies() {
		if c.Name == "auth_tokens" {
			basicToken = extractBasicToken(c.Value)
			break
		}
	}
	if basicToken == "" {
		// Fallback: construct from user+password directly.
		basicToken = base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
		basicToken = "Basic " + basicToken
	}

	// Determine host for keychain key.
	host := extractHostFromURL(baseURL)
	key := auth.ContextKey(user, org, host)
	if err := store.Store(key, basicToken); err != nil {
		return fmt.Errorf("storing credential: %w", err)
	}

	outWriter.Info("Authenticated as %s for org %s on %s", user, org, host)
	return nil
}

// extractBasicToken decodes the auth_tokens cookie value, which is a
// base64-encoded JSON object containing {access_token: "Basic ..."}.
func extractBasicToken(cookieValue string) string {
	decoded, err := base64.StdEncoding.DecodeString(cookieValue)
	if err != nil {
		return ""
	}
	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(decoded, &tokens); err != nil {
		return ""
	}
	return tokens.AccessToken
}

func extractHostFromURL(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return u.Host
	}
	return strings.TrimPrefix(rawURL, "https://")
}

func runLogout(cmd *cobra.Command, _ []string) error {
	urlStr := viper.GetString("url")
	org := viper.GetString("org")
	user := viper.GetString("user")

	if urlStr == "" || org == "" {
		return fmt.Errorf("--url and --org are required")
	}
	host := extractHostFromURL(urlStr)
	if user == "" {
		user = os.Getenv("O2_USER")
	}
	if user == "" {
		return fmt.Errorf("--user or O2_USER is required")
	}

	key := auth.ContextKey(user, org, host)
	if err := store.Delete(key); err != nil {
		return fmt.Errorf("removing credential: %w", err)
	}

	outWriter.Info("Removed credential for %s@%s", user, host)
	return nil
}

func runAuthList(_ *cobra.Command, _ []string) error {
	summaries, err := auth.ListContexts(store)
	if err != nil {
		return fmt.Errorf("listing contexts: %w", err)
	}
	if len(summaries) == 0 {
		outWriter.Info("No stored auth contexts. Run 'o2 auth login' to add one.")
		return nil
	}

	type row struct {
		User string
		Org  string
		Host string
	}
	var rows []row
	for _, s := range summaries {
		rows = append(rows, row{User: s.User, Org: s.Org, Host: s.Host})
	}
	_ = outWriter.WriteJSON(rows)
	return nil
}

func detectActiveContext(cfg *config.Config) string {
	if cfg.URL != "" && cfg.Org != "" {
		return fmt.Sprintf("%s/%s", cfg.Org, extractHostFromURL(cfg.URL))
	}
	return ""
}

func runAuthCurrent(_ *cobra.Command, _ []string) error {
	ctx, err := resolveContext()
	if err != nil {
		code := apiExitCode(err)
		if errors.Is(err, auth.ErrNoAuthContext) {
			code = 2
		}
		outWriter.Error(code, err.Error())
		return err
	}
	return outWriter.WriteJSON(ctx)
}

func apiExitCode(err error) int {
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) {
		return api.ExitCode(httpErr.StatusCode)
	}
	return 1
}
