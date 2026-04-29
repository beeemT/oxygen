// Package main is the o2 CLI entry point.
package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
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
	// Activate environment variable watching for all keys bound via BindEnv.
	// This must be called before flag parsing so env vars are available when
	// subcommand flags are accessed via viper.GetString after ParseFlags.
	// BindEnv must have been called already (done in subcommand init()).
	viper.AutomaticEnv()

	// Bind root-level flags to viper so env vars O2_* override them.
	flags := cmd.Flags()
	_ = viper.BindPFlag("url", flags.Lookup("url"))
	_ = viper.BindPFlag("org", flags.Lookup("org"))
	_ = viper.BindPFlag("token", flags.Lookup("token"))
	_ = viper.BindPFlag("format", flags.Lookup("format"))
	_ = viper.BindPFlag("timeout", flags.Lookup("timeout"))
	_ = viper.BindEnv("url", "O2_URL")
	_ = viper.BindEnv("org", "O2_ORG")
	_ = viper.BindEnv("token", "O2_TOKEN")
	_ = viper.BindEnv("format", "O2_FORMAT")
	_ = viper.BindEnv("timeout", "O2_TIMEOUT")

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
	if noColor, err := flags.GetBool("no-color"); err == nil {
		cfg.NoColor = noColor
	}
	if quiet, err := flags.GetBool("quiet"); err == nil {
		cfg.Quiet = quiet
	}
	if dryRun, err := flags.GetBool("dry-run"); err == nil {
		cfg.DryRun = dryRun
	}

	// User may come from flag or env var.
	if cfg.User == "" {
		cfg.User = os.Getenv("O2_USER")
	}

	// Open credential store, falling back to encrypted file on headless Linux.
	store, err = auth.NewKeychain("oxygen")
	if err != nil {
		home, homeErr := os.UserHomeDir()
		dir := ""
		if homeErr == nil {
			dir = home + "/.config/oxygen"
		}
		store, err = auth.NewFileStore(dir)
		if err != nil {
			return fmt.Errorf("initialising credential store: %w", err)
		}
		store = &warningStore{inner: store}
	}

	outWriter = output.NewWriter(output.Format(cfg.Format), cfg.Quiet, cfg.NoColor)

	return nil
}

// warningStore wraps a Store and prints a one-time warning on first write.
type warningStore struct {
	inner  auth.Store
	warned bool
}

func (s *warningStore) Store(key string, token string) error {
	if !s.warned {
		fmt.Fprintln(os.Stderr, "WARNING: System keychain unavailable; credentials stored in ~/.config/oxygen/credentials.json")
		s.warned = true
	}

	return s.inner.Store(key, token)
}

func (s *warningStore) Get(key string) (string, error) { return s.inner.Get(key) }
func (s *warningStore) Delete(key string) error        { return s.inner.Delete(key) }
func (s *warningStore) List() ([]string, error)        { return s.inner.List() }

// resolveContext returns the fully resolved auth context for API calls.
// When --dry-run is set and --url/--org are provided but no token is available,
// it returns a minimal context so dry-run output can be printed without auth.
func resolveContext() (*auth.Context, error) {
	// Dry-run mode: accept URL + ORG even without a token.
	if cfg.DryRun && cfg.URL != "" && cfg.Org != "" {
		token := cfg.Token
		if token == "" {
			token = "<dry-run-token>"
		}
		return &auth.Context{
			URL:   strings.TrimRight(cfg.URL, "/"),
			Org:   cfg.Org,
			User:  cfg.User,
			Token: token,
			Host:  extractHost(cfg.URL),
		}, nil
	}

	resolver := auth.NewResolver(cfg, store)

	return resolver.Resolve(context.Background())
}

func extractHost(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return u.Host
	}

	return strings.TrimPrefix(rawURL, "https://")
}
