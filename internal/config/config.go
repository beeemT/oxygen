package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Format represents the CLI output format.
type Format string

const (
	FormatJSON   Format = "json"
	FormatPretty Format = "pretty"
	FormatTable  Format = "table"
	FormatLog    Format = "log"
	FormatCSV    Format = "csv"
)

const (
	DefaultTimeout = 60 * time.Second
	DefaultFormat  = FormatJSON
	EnvPrefix      = "O2"
)

// Config holds all CLI configuration. Fields are populated by config file,
// environment variables (with O2_ prefix), and CLI flags.
type Config struct {
	URL       string
	Org       string
	User      string
	Token     string
	Password  string
	Format    Format
	NoColor   bool
	Timeout   time.Duration
	Quiet     bool
	DryRun    bool
}

// Load reads configuration from a Viper-backed config file and environment
// variables. It is idempotent and safe to call before flag parsing.
func Load(configFile string) (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		xdgConfigHome := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfigHome == "" {
			if home, err := os.UserHomeDir(); err == nil {
				xdgConfigHome = filepath.Join(home, ".config")
			}
		}
		if xdgConfigHome != "" {
			v.AddConfigPath(filepath.Join(xdgConfigHome, "openobserve-cli"))
		}
		v.AddConfigPath(".")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	v.SetDefault("timeout", DefaultTimeout.String())
	v.SetDefault("format", string(DefaultFormat))

	if configFile != "" {
		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("reading config file: %w", err)
			}
		}
	}

	cfg := &Config{
		URL:      v.GetString("url"),
		Org:      v.GetString("org"),
		User:     v.GetString("user"),
		Token:    v.GetString("token"),
		Password: v.GetString("password"),
		NoColor:  v.GetBool("no_color"),
		Quiet:    v.GetBool("quiet"),
		DryRun:   v.GetBool("dry_run"),
		Format:   Format(v.GetString("format")),
	}
	if timeout := v.GetDuration("timeout"); timeout > 0 {
		cfg.Timeout = timeout
	}

	switch cfg.Format {
	case FormatJSON, FormatPretty, FormatTable, FormatLog, FormatCSV:
		// valid
	default:
		cfg.Format = DefaultFormat
	}

	return cfg, nil
}
