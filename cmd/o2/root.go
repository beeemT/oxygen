// Package main is the o2 CLI entry point.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/beeemt/oxygen/internal/auth"
	"github.com/beeemt/oxygen/internal/config"
	"github.com/beeemt/oxygen/internal/output"
)

// Global config and writer — initialised in initConfig.
var (
	cfg       *config.Config
	store     auth.Store
	outWriter *output.Writer
)

// rootCmd is the top-level command.
var rootCmd = &cobra.Command{
	Use:   "o2",
	Short: "o2 — OpenObserve CLI",
	Long: `o2 is a CLI for querying logs, metrics, and traces from OpenObserve,
and for managing the platform. It targets both AI agents (structured JSON output)
and humans (pretty terminal rendering).

Auth:
  o2 auth login --url https://o2.example.com --org myorg --user admin@example.com

Search logs:
  o2 logs search --stream mylogs --sql "SELECT * WHERE status = 'error'" --start=1h

Query metrics:
  o2 metrics query --expr 'rate(http_requests_total[5m])' --start=1h
`,
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		return initConfig(cmd)
	},
}

func init() {
	fs := rootCmd.PersistentFlags()
	fs.String("url", "", "OpenObserve base URL")
	fs.String("org", "", "Organization ID")
	fs.String("token", "", "Basic auth credential (overrides keychain)")
	fs.String("format", "json", "Output format: json|pretty|table|log|csv")
	fs.Duration("timeout", 60*time.Second, "Request timeout")
	fs.Bool("no-color", false, "Disable color output")
	fs.BoolP("quiet", "q", false, "Suppress informational output")
	fs.Bool("dry-run", false, "Print resolved request without executing")

	rootCmd.AddCommand(authCmd)
}

// initConfig populates cfg from config file + env vars + CLI flags,
// and opens the credential store. Called via PersistentPreRunE.
func initConfig(cmd *cobra.Command) error {
	// Bind flags to viper so env vars O2_* override them.
	flags := cmd.Flags()
	viper.BindPFlag("url", flags.Lookup("url"))
	viper.BindPFlag("org", flags.Lookup("org"))
	viper.BindPFlag("token", flags.Lookup("token"))
	viper.BindPFlag("format", flags.Lookup("format"))
	viper.BindPFlag("timeout", flags.Lookup("timeout"))

	// Load config file + env vars into cfg.
	var err error
	cfg, err = config.Load("")
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Apply CLI flags (flags override env var defaults from viper binding).
	if flags.Changed("url") {
		cfg.URL = viper.GetString("url")
	}
	if flags.Changed("org") {
		cfg.Org = viper.GetString("org")
	}
	if flags.Changed("token") {
		cfg.Token = viper.GetString("token")
	}
	if flags.Changed("format") {
		cfg.Format = config.Format(viper.GetString("format"))
	}
	if flags.Changed("timeout") {
		cfg.Timeout = viper.GetDuration("timeout")
	}
	cfg.NoColor, _ = flags.GetBool("no-color")
	cfg.Quiet, _ = flags.GetBool("quiet")
	cfg.DryRun, _ = flags.GetBool("dry-run")

	// User may come from flag or env var.
	if cfg.User == "" {
		cfg.User = os.Getenv("O2_USER")
	}

	// Open credential store, falling back to encrypted file on headless Linux.
	store, err = auth.NewKeychain("openobserve-cli")
	if err != nil {
		home, _ := os.UserHomeDir()
		dir := ""
		if home != "" {
			dir = home + "/.config/openobserve-cli"
		}
		store, err = auth.NewFileStore(dir)
		if err != nil {
			return fmt.Errorf("initialising credential store: %w", err)
		}
		store = &warningStore{inner: store}
	}

	outWriter = output.NewWriter(output.Format(cfg.Format), cfg.Quiet)
	return nil
}

// warningStore wraps a Store and prints a one-time warning on first write.
type warningStore struct {
	inner auth.Store
	warned bool
}

func (s *warningStore) Store(key, token string) error {
	if !s.warned {
		fmt.Fprintln(os.Stderr, "WARNING: System keychain unavailable; credentials stored in ~/.config/openobserve-cli/credentials.json")
		s.warned = true
	}
	return s.inner.Store(key, token)
}

func (s *warningStore) Get(key string) (string, error) { return s.inner.Get(key) }
func (s *warningStore) Delete(key string) error       { return s.inner.Delete(key) }
func (s *warningStore) List() ([]string, error)       { return s.inner.List() }

// resolveContext returns the fully resolved auth context for API calls.
func resolveContext() (*auth.Context, error) {
	resolver := auth.NewResolver(cfg, store)
	return resolver.Resolve(context.Background())
}
