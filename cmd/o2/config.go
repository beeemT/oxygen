package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/beeemt/oxygen/internal/auth"
)

// configCmd is the "o2 config" command.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show effective configuration",
	Long:  "Print the resolved configuration (URL, org, format, timeout, and active auth context).",
	RunE:  runConfigShow,
}

func init() {
	rootCmd.AddCommand(configCmd)
}

type configShow struct {
	URL     string `json:"url"`
	Org     string `json:"org"`
	Format  string `json:"format"`
	Timeout string `json:"timeout"`
	NoColor bool   `json:"no_color"`
	Quiet   bool   `json:"quiet"`
	DryRun  bool   `json:"dry_run"`
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	show := configShow{
		URL:     cfg.URL,
		Org:     cfg.Org,
		Format:  string(cfg.Format),
		Timeout: cfg.Timeout.String(),
		NoColor: cfg.NoColor,
		Quiet:   cfg.Quiet,
		DryRun:  cfg.DryRun,
	}

	// Determine the effective token source.
	tokenSource := "keychain"
	if cfg.Token != "" {
		tokenSource = "flag"
	} else if os.Getenv("O2_TOKEN") != "" {
		tokenSource = "env"
	}

	// Try to resolve the active context from keychain for display.
	var activeUser, activeHost string
	resolver := auth.NewResolver(cfg, store)
	if resolved, err := resolver.Resolve(context.Background()); err == nil {
		activeUser = resolved.User
		activeHost = resolved.Host
		tokenSource = "keychain"
	}

	// Build extended info for pretty mode.
	if cfg.Format == "pretty" || cfg.Format == "table" {
		fmt.Fprintln(os.Stdout, "Effective configuration:")
		fmt.Fprintf(os.Stdout, "  URL:      %s\n", show.URL)
		fmt.Fprintf(os.Stdout, "  Org:      %s\n", show.Org)
		fmt.Fprintf(os.Stdout, "  Format:   %s\n", show.Format)
		fmt.Fprintf(os.Stdout, "  Timeout:  %s\n", show.Timeout)
		fmt.Fprintf(os.Stdout, "  NoColor:  %v\n", show.NoColor)
		fmt.Fprintf(os.Stdout, "  DryRun:   %v\n", show.DryRun)
		fmt.Fprintf(os.Stdout, "  Token:    %s\n", tokenSource)
		if activeUser != "" {
			fmt.Fprintf(os.Stdout, "  User:     %s\n", activeUser)
			fmt.Fprintf(os.Stdout, "  Host:     %s\n", activeHost)
		}
		return nil
	}

	// JSON mode: include token source in output.
	showJSON := struct {
		configShow
		TokenSource string `json:"token_source"`
		TokenValue  string `json:"-"` // never expose the actual token
	}{
		configShow:  show,
		TokenSource: tokenSource,
	}

	data, err := json.MarshalIndent(showJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	// Mask token if present in URL (for display purposes only in logs).
	masked := strings.ReplaceAll(string(data), cfg.Token, "<redacted>")
	fmt.Fprintln(os.Stdout, masked)

	return nil
}
