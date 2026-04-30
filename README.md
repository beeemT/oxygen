# o2 — OpenObserve CLI

`o2` is a CLI for querying logs, metrics, and traces from OpenObserve, and for managing the platform. It targets both AI agents (structured JSON output) and humans (pretty terminal rendering).

## Installation

### Homebrew (macOS / Linux)

```bash
brew install beeemT/tap/oxygen
```

Fish and Zsh shell completions are installed automatically. Bash completions require manual setup (see [Shell completion](#shell-completion)).

### Binary releases

Download a pre-built binary from the [latest release](https://github.com/beeemT/oxygen/releases/latest) for your platform:

| Platform | Architectures |
|---|---|
| macOS | arm64, amd64 |
| Linux | amd64 |

Extract and place the `o2` binary somewhere in your `$PATH`.

### From source

```bash
go install github.com/beeemT/oxygen/cmd/o2@latest
```

## Configuration

`o2` resolves auth credentials in the following priority (highest first):

1. `--instance <name>` flag (selects a saved named instance)
2. `--url`, `--org`, `--token` flags
3. Current default instance (set via `o2 instance use <name>`)
4. `O2_URL`, `O2_ORG`, `O2_TOKEN` env vars
5. `O2_URL`, `O2_ORG` env vars + token from OS keychain

| Env var | Description |
|---|---|
| `O2_URL` | OpenObserve base URL (e.g. `https://cloud.openobserve.ai`) |
| `O2_ORG` | Organization ID |
| `O2_USER` | User email (used for keychain key construction) |
| `O2_TOKEN` | Basic auth credential (`Basic base64(email:password)`) |
| `O2_PASSWORD` | Password (used only for login, never stored) |
| `O2_FORMAT` | Default output format (`json`\|`pretty`\|`table`\|`log`\|`csv`) |

> **Security note**: `O2_PASSWORD` and `--token` are visible in process listings and shell history. Prefer `O2_TOKEN` or keychain storage for production use.

## Instances

Manage named OpenObserve instances. Each instance stores a URL, org, and user combination so you do not have to repeat `--url`, `--org`, `--user` on every command.

```bash
# Add an instance (creates without authenticating)
o2 instance add prod --url https://o2.example.com --org myorg --user admin@example.com

# Add and authenticate in one step
o2 instance add prod --url https://o2.example.com --org myorg --user admin@example.com --password mypass

# Set the default instance
o2 instance use prod

# List all instances (current marked with *)
o2 instance list

# Show the current default
no2 instance current

# Re-authenticate (password changed, session expired)
o2 instance login prod

# Remove an instance and its stored credential
o2 instance remove prod

# Use an instance on any command

o2 logs search --instance prod --stream mylogs --sql "SELECT 1"
```

On macOS and Linux with a desktop session, credentials are stored in the OS keychain. On headless Linux, an encrypted file at `~/.config/oxygen/credentials.json` is used with a warning.

## Logs

```bash
# SQL search (primary mode)
o2 logs search --stream mylogs --sql "SELECT * WHERE status = 'error'" --start=1h

# Human-readable log output
o2 logs search --stream mylogs --sql "SELECT *" --format log --start=5m

# Streaming (real-time)
o2 logs stream --stream mylogs --sql "SELECT * WHERE _timestamp > NOW() - 300" --follow

# Watch mode (polling)
o2 logs search --stream mylogs --sql "SELECT * WHERE status = 'error'" --start=1h --watch --interval=10s

# Field value suggestions
o2 logs values --stream mylogs --field service --prefix api

# Saved views
o2 logs views list
o2 logs views create --name "error hunt" --sql "SELECT * WHERE status = 'error'" --stream mylogs
o2 logs views get <view-id>
o2 logs views delete <view-id>

# Search history
o2 logs history --limit 20
```

### Relative time

All time flags accept Go duration strings: `--start=1h`, `--start=24h`, `--start=7d`. No leading dash.

## Metrics

```bash
# PromQL instant query
o2 metrics query --expr 'rate(http_requests_total[5m])' --start=1h

# PromQL range query
o2 metrics query --expr 'up' --start=1h --end=now --step=30s

# PromQL watch mode
o2 metrics query --expr 'up' --watch --interval=15s

# Series matching
o2 metrics series --match 'up{job="*"}'

# Label values
o2 metrics label-values --label job

# SQL query over metrics stream
o2 metrics search --stream prometheus --sql "SELECT avg(cpu) GROUP BY host" --start=1h
```

## Traces

```bash
# Latest traces from a stream
o2 traces latest --stream otel-traces --limit 20

# Trace DAG (flamegraph)
o2 traces dag --stream otel-traces --trace-id <id>

# Search traces via SQL
o2 traces search --stream otel-traces --sql "SELECT * WHERE service.name = 'checkout'"
```

## Streams

```bash
# List streams
o2 streams list [--type logs|metrics|traces]

# Show stream schema and stats
o2 streams describe <name>
```

## Dashboards

```bash
# List dashboards
o2 dashboards list [--folder <folder-id>]

# Get a dashboard
o2 dashboards get <dashboard-id>

# Create / update / delete
o2 dashboards create --file dashboard.json
o2 dashboards update <id> --file dashboard.json
o2 dashboards delete <id>
```

## Alerts

All alert commands use the `/v2/` API prefix.

```bash
# List alerts
o2 alerts list [--status firing|pending|resolved]

# Get / create / update / delete
o2 alerts get <alert-id>
o2 alerts create --file alert.json
o2 alerts update <id> --file alert.json
o2 alerts delete <alert-id>

# Trigger / history
o2 alerts trigger <alert-id>
o2 alerts history [--alert-id <id>] [--limit 50]

# Incidents and templates
o2 alerts incidents list
o2 alerts templates list
```

## Output formats

Use `--format` or `O2_FORMAT`:

| Format | Description |
|---|---|
| `json` | Compact JSON (default, for AI agents) |
| `pretty` | Indented JSON (human inspection) |
| `table` | ASCII table (tab-separated) |
| `log` | Human-readable log lines with color |
| `csv` | CSV export |

## Global flags

|| Flag | Default | Description |
||---|---|---|
|| `--instance NAME` | | Instance name (overrides current default) |
|| `--url URL` | | OpenObserve base URL |
|| `--org ORG` | | Organization ID |
|| `--token TOKEN` | | Basic auth credential |
|| `--format FORMAT` | `json` | Output format |
|| `--timeout DURATION` | `60s` | Request timeout |
|| `--no-color` | `false` | Disable color output |
|| `-q, --quiet` | `false` | Suppress informational output |
|| `--dry-run` | `false` | Print resolved request, no HTTP call |

## Dry-run mode

All query commands support `--dry-run`, which prints the resolved request without making an HTTP call. This is useful to verify timestamps and query construction:

```bash
o2 logs search --stream mylogs --sql "SELECT *" --start=1h --dry-run
```

## Exit codes

| Code | Meaning | Action |
|---|---|---|
| 0 | Success | none |
| 1 | Usage/syntax error | fix command arguments |
|| 2 | Auth failure (401) | run `o2 instance login <name>` |
| 3 | Forbidden (403) | check org/permissions |
| 4 | Not found (404) | check resource name |
| 5 | Invalid request (400/422) | fix query syntax |
| 6 | Rate limited (429) | wait and retry |
| 7 | Server error (500/503) | retry with backoff |

## Shell completion

Fish and Zsh completions are installed automatically with `brew install beeemT/tap/oxygen`. Bash completions require manual setup:

```bash
cp $(brew --prefix)/opt/oxygen/completions/o2.bash $(brew --prefix)/etc/bash_completion.d/
```

## License

MIT
