package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/beeemt/oxygen/internal/config"
)

// Context holds the fully resolved authentication context for a request.
type Context struct {
	URL   string // e.g. "https://o2.example.com"
	Org   string // e.g. "myorg"
	User  string // e.g. "admin@example.com"
	Token string // e.g. "Basic base64..."
	Host  string // e.g. "o2.example.com" (extracted from URL)
}

// Resolver resolves CLI flags + env vars + keychain into a [Context].
type Resolver struct {
	cfg   *config.Config
	store Store
}

// NewResolver returns a Resolver that reads from the given config and store.
func NewResolver(cfg *config.Config, store Store) *Resolver {
	return &Resolver{cfg: cfg, store: store}
}

// Resolve returns the effective auth context following priority order:
//
//	1. --url + --org + --token flags
//	2. O2_URL + O2_ORG + O2_TOKEN env vars
//	3. O2_URL + O2_ORG → keychain lookup
func (r *Resolver) Resolve(ctx context.Context) (*Context, error) {
	cfg := r.cfg

	// Priority 1: explicit CLI flags (Token is non-empty only when --token was set).
	if cfg.URL != "" && cfg.Org != "" && cfg.Token != "" {
		return &Context{
			URL:   stripSlash(cfg.URL),
			Org:   cfg.Org,
			User:  cfg.User,
			Token: cfg.Token,
			Host:  extractHost(cfg.URL),
		}, nil
	}

	// Priority 2: env vars (O2_URL, O2_ORG, O2_TOKEN, O2_USER).
	url := os.Getenv("O2_URL")
	org := os.Getenv("O2_ORG")
	token := os.Getenv("O2_TOKEN")
	user := os.Getenv("O2_USER")

	if url == "" {
		url = cfg.URL
	}
	if org == "" {
		org = cfg.Org
	}
	if token == "" {
		token = cfg.Token
	}
	if user == "" {
		user = cfg.User
	}

	if url != "" && org != "" && token != "" {
		return &Context{
			URL:   stripSlash(url),
			Org:   org,
			User:  user,
			Token: token,
			Host:  extractHost(url),
		}, nil
	}

	// Priority 3: URL + ORG → keychain lookup.
	if url == "" || org == "" {
		return nil, ErrNoAuthContext
	}

	if user == "" {
		return nil, fmt.Errorf("O2_USER is required for keychain lookup (set it or pass --user): %w", ErrNoAuthContext)
	}

	host := extractHost(url)
	stored, err := r.store.Get(ContextKey(user, org, host))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("no stored credential for %s@%s; run 'o2 auth login' first: %w", user, host, ErrNoAuthContext)
		}
		return nil, fmt.Errorf("reading keychain: %w", err)
	}

	return &Context{
		URL:   stripSlash(url),
		Org:   org,
		User:  user,
		Token: stored,
		Host:  host,
	}, nil
}

// ListContexts returns all stored credential keys, parsed into user/org/host.
func ListContexts(store Store) ([]ContextSummary, error) {
	keys, err := store.List()
	if err != nil {
		return nil, err
	}
	var summaries []ContextSummary
	for _, key := range keys {
		// Format: openobserve-cli/{user}/{org}@{host}
		parts := strings.Split(key, "/")
		if len(parts) != 3 {
			continue
		}
		userOrgHost := strings.SplitN(parts[2], "@", 2)
		if len(userOrgHost) != 2 {
			continue
		}
		summaries = append(summaries, ContextSummary{
			Key:  key,
			User: parts[1],
			Org:  userOrgHost[0],
			Host: userOrgHost[1],
		})
	}
	return summaries, nil
}

// ContextSummary describes a stored credential without exposing the token.
type ContextSummary struct {
	Key  string
	User string
	Org  string
	Host string
}

// ErrNoAuthContext is returned when no auth context can be resolved.
var ErrNoAuthContext = fmt.Errorf("no authentication context")

func extractHost(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		return u.Host
	}
	return strings.TrimPrefix(rawURL, "https://")
}

func stripSlash(s string) string {
	return strings.TrimRight(s, "/")
}
