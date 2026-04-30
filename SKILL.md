# SKILL.md — o2 (OpenObserve CLI)

## Tool identity

`o2` is the CLI binary at the repo root. It queries OpenObserve (logs, metrics, traces) and manages the platform.

## Quick reference

```bash
# Authenticate
O2_PASSWORD=... o2 auth login --url https://o2.example.com --org myorg --user admin@example.com

# Query logs
o2 logs search --stream mylogs --sql "SELECT * WHERE status = 'error'" --start=1h
o2 logs stream --stream mylogs --sql "SELECT * WHERE _timestamp > NOW() - 300" --follow

# Query metrics (PromQL)
o2 metrics query --expr 'rate(http_requests_total{service="api"}[5m])' --start=1h
o2 metrics query --expr 'up' --start=1h --end=now --step=30s

# Explore
o2 streams list
o2 streams describe <name>
o2 logs values --stream mylogs --field service --prefix api

# Watch mode (polling)
o2 logs search --stream mylogs --sql "SELECT * WHERE status = 'error'" --start=1h --watch --interval=10s
o2 metrics query --expr 'up' --watch --interval=15s

# Dry-run
o2 logs search --stream mylogs --sql "SELECT *" --start=1h --dry-run
```

## Auth

**Token format**: `Basic base64("email:password")` — the raw credential, not a derived token.

**Priority**: flags > env vars (`O2_URL`, `O2_ORG`, `O2_TOKEN`) > keychain.

**Keychain key**: `oxygen/{user}/{org}@{host}`. Credentials are stored on login and retrieved automatically on subsequent calls.

**Headless Linux**: falls back to `~/.config/oxygen/credentials.json` with a warning. The encrypted file is acceptable for CI.

## Time syntax

- Go duration strings: `--start=1h`, `--start=24h`, `--start=7d`, `--start=1h30m`
- No leading dash
- Default start: `now - 1h` for most commands
- API expects **microseconds** Unix epoch (the CLI converts automatically)

## Output formats

| Format | Use case |
|---|---|
| `json` (default) | AI agents, scripting |
| `pretty` | Human inspection of JSON |
| `table` | Streams, alerts, dashboards |
| `log` | Human log output with color |
| `csv` | Export |

## Search API facts

- `_search` body uses `start_time` / `end_time` in **microseconds**
- `query_type: "sql"` for SQL queries
- `track_total_hits: true` to get total match count
- `quick_mode: true` for faster, approximate results
- `streaming_output: true` for `/_search_stream` (NDJSON)
- Multi-stream: comma-separated names → `/_search_multi` internally

## PromQL API facts

- Instant query: `?query=<expr>&time=<unix>` (seconds)
- Range query: `?query=<expr>&start=<unix>&end=<unix>&step=<duration>`
- Labels: `?match[]=<selector>`

## Error handling

Exit codes map to HTTP status classes:

| Exit | HTTP |
|---|---|
| 2 | 401 |
| 3 | 403 |
| 4 | 404 |
| 5 | 400/422 |
| 6 | 429 |
| 7 | 500/503 |

JSON error shape: `{"error": "...", "code": <http_status>, "message": "...", "exit_code": <exit>}`

## Shell completion

Fish and Zsh completions are installed automatically with `brew install beeemT/tap/oxygen`.

For Bash, source the completion file from the tarball or copy it manually:

```bash
# After installing the tarball (not via brew)
# Copy bash completion to your bash_completions dir, e.g.:
cp completions/o2.bash ~/.bash_completion.d/
```

## Key gotchas

- **Microseconds**: search `start_time`/`end_time` are microseconds. Dry-run shows resolved values to verify.
- **No `--time` alias**: use `--start` for PromQL instant query time (not `--time`).
- **Alert endpoints**: all use `/v2/` prefix — the CLI handles this automatically.
- **Watch vs stream**: `--follow` uses HTTP/2 streaming (`/_search_stream`); `--watch` polls `/_search` at intervals.
- **Keychain on macOS**: first run may prompt the OS credential access dialog.
