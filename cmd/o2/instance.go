// Package main is the o2 CLI entry point.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/beeemt/oxygen/internal/auth"
	"github.com/beeemt/oxygen/internal/instances"
	"github.com/beeemt/oxygen/internal/output"
)

// instanceCmd is the parent for all instance subcommands.
var instanceCmd = &cobra.Command{
	Use:   "instance",
	Short: "Manage named OpenObserve instances",
	Long: `Add, remove, list, and switch between named OpenObserve instances.
Each instance stores a URL, org, and user combination. Once added, use
--instance <name> on any command to select it, or run 'o2 instance use <name>'
to set a default.`,
}

// Declare command vars so init() can reference them before run* functions are defined.
var (
	instanceAddCmd    *cobra.Command
	instanceRemoveCmd = &cobra.Command{
		Use:   "remove",
		Short: "Remove an instance",
		Long:  `Remove a named instance and its stored credential from the keychain.`,
		Args:  cobra.ExactArgs(1),
		RunE:  runInstanceRemove,
	}
	instanceListCmd = &cobra.Command{
		Use:   "list",
		Short: "List all instances",
		RunE:  runInstanceList,
	}
	instanceUseCmd = &cobra.Command{
		Use:   "use",
		Short: "Set the default instance",
		Long: `Set the default instance. Subsequent commands without --instance will
use this instance as the target.`,
		Args: cobra.ExactArgs(1),
		RunE: runInstanceUse,
	}
	instanceCurrentCmd = &cobra.Command{
		Use:   "current",
		Short: "Show the current default instance",
		RunE:  runInstanceCurrent,
	}
	instanceLoginCmd *cobra.Command
)

func init() {
	// Create the commands that need flag access in init() before referencing Flags().
	instanceAddCmd = &cobra.Command{
		Use:   "add",
		Short: "Add a new instance",
		Long: `Add a new named OpenObserve instance. If --password is given or O2_PASSWORD
is set, the command authenticates immediately and stores the credential.
Otherwise the instance is created unauthenticated; run 'o2 instance login <name>'
to authenticate later.`,
		RunE: runInstanceAdd,
	}
	instanceLoginCmd = &cobra.Command{
		Use:   "login",
		Short: "Authenticate an existing instance",
		Long: `Authenticate an existing instance. Use this when the stored credential
has expired or when the password has changed. Prompts for password interactively
if --password is not given and O2_PASSWORD is not set.`,
		Args: cobra.ExactArgs(1),
		RunE: runInstanceLogin,
	}

	rootCmd.AddCommand(instanceCmd)
	instanceCmd.AddCommand(instanceAddCmd, instanceRemoveCmd, instanceListCmd, instanceUseCmd, instanceCurrentCmd, instanceLoginCmd)

	af := instanceAddCmd.Flags()
	af.String("url", "", "OpenObserve base URL (required)")
	af.String("org", "", "Organization ID (required)")
	af.String("user", "", "User email (required)")
	_ = instanceAddCmd.MarkFlagRequired("url")
	_ = instanceAddCmd.MarkFlagRequired("org")
	_ = instanceAddCmd.MarkFlagRequired("user")
	_ = viper.BindPFlag("url", af.Lookup("url"))
	_ = viper.BindPFlag("org", af.Lookup("org"))
	_ = viper.BindPFlag("user", af.Lookup("user"))

	pf := instanceLoginCmd.Flags()
	pf.String("password", "", "Password (optional; falls back to O2_PASSWORD or interactive prompt)")
}

// getInstanceManager returns the shared instance manager (initialised by root.go initConfig).
func getInstanceManager() *instances.Manager {
	return instanceMgr
}

// runInstanceAdd creates a new instance, optionally authenticating immediately.
func runInstanceAdd(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("name argument required")
	}
	name := args[0]

	url := viper.GetString("url")
	org := viper.GetString("org")
	user := viper.GetString("user")
	password := cfg.Password
	if password == "" {
		password = os.Getenv("O2_PASSWORD")
	}

	m := getInstanceManager()
	if err := m.Add(name, url, org, user); err != nil {
		return err
	}

	outWriter.Info("Added instance %q", name)

	// Authenticate immediately if a password is available.
	if password != "" {
		token, err := performLogin(url, org, user, password)
		if err != nil {
			return fmt.Errorf("instance created, but login failed: %w", err)
		}
		if storeErr := storeCredential(user, org, url, token); storeErr != nil {
			return fmt.Errorf("instance created, but storing credential failed: %w", storeErr)
		}
		outWriter.Info("Authenticated as %s for org %s on %s", user, org, extractHostFromURL(url))
	}

	return nil
}

// runInstanceRemove deletes an instance and its stored credential.
func runInstanceRemove(_ *cobra.Command, args []string) error {
	name := args[0]

	m := getInstanceManager()
	inst, err := m.Get(name)
	if err != nil {
		return err
	}

	if err := m.Remove(name); err != nil {
		return err
	}
	outWriter.Info("Removed instance %q", name)

	// Also remove the stored credential.
	host := extractHostFromURL(inst.URL)
	key := auth.ContextKey(inst.User, inst.Org, host)
	if err := store.Delete(key); err != nil {
		outWriter.Info("(credential for %s already absent)", key)
	}

	return nil
}

// runInstanceList prints all instances.
func runInstanceList(_ *cobra.Command, _ []string) error {
	m := getInstanceManager()
	current, hasCurrent, _ := m.Current()
	instances := m.List()

	type row struct {
		Name    string `json:"name"`
		URL     string `json:"url"`
		Org     string `json:"org"`
		User    string `json:"user"`
		Current bool   `json:"current"`
	}
	var rows []row
	for _, inst := range instances {
		rows = append(rows, row{
			Name:    inst.Name,
			URL:     inst.URL,
			Org:     inst.Org,
			User:    inst.User,
			Current: hasCurrent && inst.Name == current.Name,
		})
	}

	return outWriter.WriteJSON(rows)
}

// runInstanceUse sets the default instance.
func runInstanceUse(_ *cobra.Command, args []string) error {
	name := args[0]

	m := getInstanceManager()
	if err := m.SetCurrent(name); err != nil {
		return err
	}
	outWriter.Info("%q is now the default instance", name)

	return nil
}

// runInstanceCurrent prints the current default instance.
func runInstanceCurrent(_ *cobra.Command, _ []string) error {
	m := getInstanceManager()
	inst, hasCurrent, err := m.Current()
	if err != nil {
		return err
	}
	if !hasCurrent {
		outWriter.Info("No default instance set. Use 'o2 instance use <name>' to set one.")

		return nil
	}

	type row struct {
		Name string `json:"name"`
		URL  string `json:"url"`
		Org  string `json:"org"`
		User string `json:"user"`
	}
	return outWriter.WriteJSON(row{Name: inst.Name, URL: inst.URL, Org: inst.Org, User: inst.User})
}

// runInstanceLogin re-authenticates an existing instance.
func runInstanceLogin(_ *cobra.Command, args []string) error {
	name := args[0]

	m := getInstanceManager()
	inst, err := m.Get(name)
	if err != nil {
		return fmt.Errorf("%w; run 'o2 instance add %s ...' first", err, name)
	}

	password := viper.GetString("password")
	if password == "" {
		password = os.Getenv("O2_PASSWORD")
	}
	if password == "" {
		var promptErr error
		password, promptErr = output.PromptPassword("Password: ")
		if promptErr != nil {
			return fmt.Errorf("reading password: %w", promptErr)
		}
	}

	token, err := performLogin(inst.URL, inst.Org, inst.User, password)
	if err != nil {
		return err
	}
	if err := storeCredential(inst.User, inst.Org, inst.URL, token); err != nil {
		return fmt.Errorf("storing credential: %w", err)
	}

	outWriter.Info("Authenticated as %s for org %s on %s", inst.User, inst.Org, extractHostFromURL(inst.URL))

	return nil
}

// performLogin sends a login request and returns the auth token.
// Callers must ensure password is non-empty; this function does not prompt.
func performLogin(rawURL, _ string, user, password string) (string, error) {
	baseURL := strings.TrimRight(rawURL, "/")
	loginURL := baseURL + "/api/auth/login"

	body, err := json.Marshal(map[string]string{
		"name":     user,
		"password": password,
	})
	if err != nil {
		return "", fmt.Errorf("encoding request body: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending login request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var loginResp struct {
		Status  bool   `json:"status"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return "", fmt.Errorf("decoding login response: %w", err)
	}

	if !loginResp.Status {
		msg := loginResp.Message
		if msg == "" {
			msg = "login failed"
		}

		return "", fmt.Errorf("%s", msg)
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
		basicToken = "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password))
	}

	return basicToken, nil
}

// storeCredential stores the given token under the appropriate keychain key.
func storeCredential(user, org, rawURL, token string) error {
	host := extractHostFromURL(rawURL)
	key := auth.ContextKey(user, org, host)
	if err := store.Store(key, token); err != nil {
		return fmt.Errorf("storing credential: %w", err)
	}

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
