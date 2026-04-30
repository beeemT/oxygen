# oxygen — Project Plan

> **Plan status**: reviewed and corrected by four agents (API, CLI/UX, Security, Project Structure).

---

## Context

OpenObserve is an open-source observability platform (logs, metrics, traces, RUM, dashboards, alerts, pipelines). It exposes a REST API at `/api/{org_id}/...` with HTTP Basic authentication (stored in an httpOnly cookie). The API has no official public OpenAPI doc at a stable URL; the definitive source is the Rust router source (`src/handler/http/router/mod.rs`).

The goal is a Go CLI for querying logs/metrics and managing the platform, targeting both AI agents (structured output, scripted usage) and humans (pretty terminal rendering). The project uses `mise` for toolchain management.

---

## 1. API Coverage

### 1.1 Endpoints — Tier Analysis

#### Tier 1: Core (must support, v1)

> **URL prefix**: All endpoints are relative to `<base_url>/api`. For example, `/{org_id}/_search` resolves to `https://o2.example.com/api/{org_id}/_search`. The auth endpoint `/auth/login` is at `https://o2.example.com/api/auth/login`.

These are the primary value of an observability CLI.

| Endpoint | Method | Purpose |
|---|---|---|
| `/auth/login` | POST | Authenticate with `{"name":"...", "password":"..."}` → Basic auth cookie |
| `/{org_id}/_search` | POST | Full log/trace search (SQL or full-text) |
| `/{org_id}/_search_stream` | POST | Streaming search results (HTTP/2, NDJSON) |
| `/{org_id}/_search_multi` | POST | Search across multiple streams |
| `/{org_id}/{stream_name}/_values` | GET | Field value suggestions |
| `/{org_id}/_values_stream` | POST | Streaming field value enumeration (HTTP/2) |
| `/{org_id}/{stream_name}/_around` | GET/POST | Search around a timestamp/record |
| `/{org_id}/_search_history` | POST | Query history |
| `/{org_id}/savedviews` | GET/POST | Saved views (list/create) |
| `/{org_id}/savedviews/{view_id}` | GET/PUT/DELETE | Manage saved view |
| `/{org_id}/prometheus/api/v1/query` | GET/POST | PromQL instant query |
| `/{org_id}/prometheus/api/v1/query_range` | GET/POST | PromQL range query |
| `/{org_id}/prometheus/api/v1/series` | GET/POST | Series cardinality |
| `/{org_id}/prometheus/api/v1/label/{name}/values` | GET | Label values |
| `/{org_id}/streams` | GET | List streams (logs/metrics/traces) |
| `/{org_id}/streams/{stream_name}/schema` | GET | Stream field schema |
| `/{org_id}/summary` | GET | Org summary (ingestion stats) |

#### Tier 2: Important (should support)

| Endpoint | Method | Purpose | Notes |
|---|---|---|---|
| `/organizations` | GET | List all orgs for current user | Root path, no `/{org_id}` prefix |
| `/{org_id}/dashboards` | GET/POST | List/create dashboards | |
| `/{org_id}/dashboards/{id}` | GET/PUT/DELETE | Dashboard CRUD | |
| `/v2/{org_id}/alerts` | GET/POST | List/create alerts | **Requires `/v2/` prefix** |
| `/v2/{org_id}/alerts/{alert_id}` | GET/PUT/DELETE | Alert CRUD | **Requires `/v2/` prefix** |
| `/v2/{org_id}/alerts/{alert_id}/trigger` | PATCH | Manual trigger | **Requires `/v2/` prefix** |
| `/v2/{org_id}/alerts/history` | GET | Alert firing history | **Requires `/v2/` prefix** |
| `/v2/{org_id}/alerts/incidents` | GET | List incidents | **Requires `/v2/` prefix** |
| `/v2/{org_id}/alerts/templates` | GET/POST | Alert templates | **Requires `/v2/` prefix** |
| `/{org_id}/functions` | GET/POST | List/create functions | |
| `/{org_id}/{stream_name}/traces/latest` | GET | Latest traces | `stream_name` is a **path** segment, not a query param |
| `/{org_id}/{stream_name}/traces/{trace_id}/dag` | GET | Trace DAG (flamegraph) | |
| `/{org_id}/prometheus/api/v1/query_exemplars` | GET/POST | Exemplar data | |

#### Tier 3: Nice to have (defer)

Dashboards with panels, traces session/user views, pipelines, RUM, enrichment tables, FGA/RBAC, reports, IAM, MCP, sourcemaps, and enterprise-only endpoints (anomaly detection, AI chat, search jobs, cipher keys, service accounts, etc.).

#### Not in scope

Ingestion endpoints (write-only), debug/profiling, node management.

#### Known phantom endpoints (do not use)

- `/{org_id}/logs/latest` — does not exist. Implement as `/_search` with SQL `ORDER BY _timestamp DESC LIMIT N`.
- `/{org_id}/alerts` (non-v2) — does not exist. All alert endpoints are under `/v2/`.

### 1.2 Search Request Structure

The `/_search` body is:

```json
{
  "query": {
    "sql": "SELECT * FROM logs WHERE status = 'error' ORDER BY _timestamp DESC",
    "from": 0,
    "size": 100,
    "start_time": 1746000000000,
    "end_time": 1746086399000,
    "track_total_hits": true,
    "quick_mode": false,
    "query_type": "sql",
    "query_fn": null,
    "streaming_output": false,
    "histogram_interval": 0
  },
  "encoding": "",
  "regions": [],
  "clusters": [],
  "timeout": 0,
  "search_type": null,
  "use_cache": true,
  "clear_cache": false
}
```

Key fields in `query`:
- `sql` (string, required) — SQL query
- `start_time` / `end_time` (i64, required) — **microseconds** Unix epoch
- `from` / `size` (i64, default 0/10) — pagination
- `track_total_hits` (bool) — count total matches even if truncated
- `query_type` (string) — `sql`, `promql`, `userfields`, `matchall`
- `quick_mode` (bool) — enable quick mode for faster results
- `histogram_interval` (i64) — histogram bucket size in seconds
- `query_fn` (string|null) — VRL function for post-processing
- `streaming_output` (bool) — enable streaming output (used by `/_search_stream`)


Key fields at the top level of `Request`:
- `encoding` (string) — `"base64"` to send SQL as base64-encoded string
- `regions` / `clusters` (string[]) — for federated/multi-region queries
- `timeout` (i64) — query timeout in seconds
- `use_cache` / `clear_cache` (bool) — result cache control

**Relative time grammar** (AI agents must use this exactly):
- `--start=<duration>` — Go duration string, interpreted as "now minus duration". E.g., `--start=1h`, `--start=24h`, `--start=7d`.
- `--end=<duration|now>` — same grammar. Default: `now`.
- `--start=1h` means "from one hour ago until now". No leading dash.

PromQL queries use standard Prometheus URL params: `?query=up&time=<unix_timestamp>` or `?query=up&time=<rfc3339>`. Both formats are accepted.

All API endpoints are prefixed with `/api` in the base URL: e.g., `https://o2.example.com/api/{org_id}/_search`. The CLI must construct URLs with the `/api` prefix.

---

## 2. Project Layout

```
oxygen/
├── cmd/
│   └── o2/
│       ├── root.go          # root command + global flags
│       ├── instance.go      # instance add / remove / list / use / current / login
│       ├── logs.go          # search, stream, values, history, views
│       ├── metrics.go       # query (PromQL), search (SQL), streams
│       ├── traces.go        # latest, dag, search
│       ├── streams.go       # list, describe
│       ├── dashboards.go    # list, get, create, update, delete
│       ├── alerts.go         # list, get, create, update, delete, trigger, history
│       └── config.go        # config show
├── internal/
│   ├── api/
│   │   ├── client.go        # http.Client + auth middleware + retry
│   │   ├── auth.go          # login API
│   │   ├── search.go        # search / search_multi / search_stream
│   │   ├── streams.go       # streams list + schema
│   │   ├── promql.go        # promql query / query_range / series / labels
│   │   ├── dashboards.go
│   │   ├── alerts.go        # v2 alerts + incidents + templates
│   │   ├── traces.go         # traces latest + dag
│   │   └── views.go         # saved views
│   ├── auth/
│   │   ├── store.go         # keychain abstraction
│   │   └── context.go       # active auth context + resolution
│   ├── instances/
│   │   └── instances.go     # named instance manager (instances.yaml)
│   ├── config/
│   │   └── config.go        # config file + env var mapping
│   ├── output/
│   │   ├── json.go          # compact + indented JSON
│   │   ├── table.go         # tabwriter-based table renderer
│   │   ├── log.go           # human log renderer (color + alignment)
│   │   ├── csv.go           # CSV export
│   │   └── metrics.go       # metrics table renderer
│   └── models/              # request/response types (moved from pkg/)
│       ├── search.go
│       ├── stream.go
│       ├── alert.go
│       └── dashboard.go
├── tools.go                  # go.mod tooling directive
├── Makefile
├── mise.toml                 # mise toolchain definition
├── go.mod
└── README.md
```

**Layout decisions (from review):**

- All types live in `internal/models/` — no separate `pkg/` layer needed for a single binary.
- `internal/output/table.go` uses `text/tabwriter` (stdlib) — avoids the unmaintained `rodaine/table` library.

---

## 3. CLI Command & Argument Pattern

Pattern: **noun-first, verb-object, consistent flags**. Designed for AI agent shell command construction.

```
o2 [global flags] <noun> [subcommand] [flags] [--] [positional args]

Global flags:
  --instance NAME     Instance name (overrides current default)
  --url URL           OpenObserve instance base URL (fallback when no instance set)
  --org ORG           Organization ID (fallback when no instance set)
  --token TOKEN       Basic auth credential `Basic <base64>` (overrides keychain/env) [WARNING: visible in process list]
  --format FORMAT     Output format: json | pretty | table | log | csv  (default: json)
  --no-color          Disable color output
  --timeout DURATION  Request timeout (default: 60s)
  --dry-run           Print resolved request without executing (AI agents)
  -q, --quiet         Suppress non-result output to stderr
```

> **Security note**: `--token` exposes credentials to command-line history and process listings.
> Prefer `O2_TOKEN` env var or keychain storage for production use.

### 3.1 Instance Commands

Instances store a URL + org + user combination under a unique name. Once added, they can be selected with `--instance` on any command, or set as the default with `o2 instance use`.

```bash
# Add an instance (authenticate immediately if --password is given)
o2 instance add prod --url https://o2.example.com --org myorg --user admin@example.com --password mypass

# Add an instance without authenticating (run 'o2 instance login' later)
o2 instance add staging --url https://staging.o2.example.com --org staging --user admin@example.com

# Authenticate (or re-authenticate) an existing instance
O2_PASSWORD=... o2 instance login prod

# List all instances (* marks the current default)
o2 instance list

# Set the default instance
o2 instance use prod

# Show the current default instance
o2 instance current

# Remove an instance and its stored credential
no2 instance remove prod
```

**Instance data** is stored in `~/.config/oxygen/instances.yaml`:
```yaml
instances:
  - name: prod
    url: https://o2.example.com
    org: myorg
    user: admin@example.com
  - name: staging
    url: https://staging.o2.example.com
    org: staging
    user: admin@example.com
current: prod
```

**Auth resolution priority (highest first):**

1. `--instance <name>` flag → look up instance → use its URL/org/user → keychain lookup
2. Current default instance → same as above
3. `--url` + `--org` + `--token` flags
4. `O2_URL` + `O2_ORG` + `O2_TOKEN` env vars
5. `O2_URL` + `O2_ORG` env vars → reads token from keychain

**Keychain key format**: `oxygen/{user}/{org}@{host}` (unchanged). Credentials are keyed by the instance's user/org/host tuple.

> **Keychain security**: On Linux systems without `dbus` or a desktop session (headless servers, CI), the `keyring` library falls back to an encrypted-on-disk JSON store. The CLI detects this and prints a warning: `WARNING: System keychain unavailable; credentials stored in ~/.config/oxygen/credentials.json.`

> **Basic auth note**: The stored token is a Basic auth credential (`Basic email:password`). This is the credential OpenObserve uses — it is not a JWT. The token remains valid until the user's password is changed.

### 3.2 Log Commands

```bash
# SQL search (primary mode for AI agents)
o2 logs search \
  --stream myapp-logs \
  --sql "SELECT * WHERE status = 'error' AND service = 'api'" \
  --from 0 --size 100 \
  --start=1h \
  --format json | jq '.hits'

# Full-text filter mode (simpler, no SQL)
o2 logs search \
  --stream myapp-logs \
  --filter 'status:error service:api' \
  --start=1h --size 50

# Human-readable log output
o2 logs search --stream nginx --sql "SELECT *" --format log --start=5m
  # Output:
  # [2025-04-29 10:23:01.123] ERROR  [service=api] [host=prod-1] Request timeout after 30s
  # [2025-04-29 10:23:02.456] WARN   [service=api] [host=prod-1] Retrying (attempt 1/3)

# Streaming search (long-running tail with --follow)
o2 logs stream --stream myapp-logs --sql "SELECT * WHERE _timestamp > NOW() - 300" --follow

# Field value suggestions (autocomplete)
o2 logs values --stream myapp-logs --field service --prefix api

# Saved views
o2 logs views list
o2 logs views create --name "error hunt" --sql "SELECT * WHERE status = 'error'" --stream myapp-logs
o2 logs views get <view-id>

# Search history
o2 logs history --limit 20
```

**Relative time** always uses `--start=<duration>` and `--end=<duration|now>`, where `<duration>` is a Go duration string (`1h`, `24h`, `7d`, `1h30m`). No leading dash.

### 3.3 Metrics Commands

```bash
# PromQL instant query (--start is the only time flag; no --time alias)
o2 metrics query \
  --expr 'rate(http_requests_total{service="api"}[5m])' \
  --start=now

# PromQL range query
o2 metrics query \
  --expr 'up{job="prometheus"}' \
  --start=1h --end=now --step=30s

# PromQL series cardinality
o2 metrics series --match='up{job="*"}'

# PromQL label values
o2 metrics label-values --label job

# SQL-style metrics query (queries metrics stream directly with SQL)
o2 metrics search \
  --stream prometheus \
  --sql "SELECT avg(cpu_usage) FROM prometheus WHERE host = 'prod-1' GROUP BY time(1m)" \
  --start=1h

# List available metric streams
o2 metrics streams list
o2 metrics streams describe <name>  # shows schema + stats
```

### 3.4 Traces Commands

```bash
# Latest traces from a stream (stream_name is a path segment in the API)
o2 traces latest --stream otel-traces --limit 20

# Trace DAG (full flamegraph/span tree)
o2 traces dag --stream otel-traces --trace-id <id>

# Search traces via SQL (uses _search endpoint with trace stream)
o2 traces search \
  --stream otel-traces \
  --sql "SELECT * WHERE service.name = 'checkout' AND duration_ms > 1000"
```

### 3.5 Streams Commands

```bash
o2 streams list [--type logs|metrics|traces]
o2 streams describe <name>  # schema + settings + stats
```

### 3.6 Dashboards Commands

```bash
o2 dashboards list [--folder <folder-id>]
o2 dashboards get <dashboard-id>
o2 dashboards create [--file dashboard.json]
o2 dashboards update <id> [--file dashboard.json]
o2 dashboards delete <id>
```

### 3.7 Alerts Commands

> All alerts endpoints use the `/v2/` prefix.

```bash
o2 alerts list [--status firing|pending|resolved]
o2 alerts get <alert-id>
o2 alerts create [--file alert.json]
o2 alerts update <id> [--file alert.json]
o2 alerts delete <id>
o2 alerts trigger <alert-id>  # manual trigger
o2 alerts history [--alert-id <id>] [--limit 50]
o2 alerts incidents list [--limit 50]
o2 alerts templates list
```

### 3.8 Config Command

```bash
o2 config show   # prints effective config (url, org, format, timeout)
```

---

## 4. Authentication Design

### 4.1 Login Flow

**Critical**: OpenObserve uses HTTP Basic Auth encoded in a cookie, NOT a JWT bearer token.

1. `POST /auth/login` with `{"name":"...", "password":"..."}` → `{"status": true|false, "message": "..."}` + `Set-Cookie: auth_tokens=<base64-encoded tokens>`
2. The `auth_tokens` cookie contains: `{"access_token": "Basic <base64(email:password)>", "refresh_token": ""}`, base64-encoded again
3. **Store the `access_token` value** (the `Basic email:password` string) in the OS keychain under `oxygen/{user}/{org}@{host}`
4. The token is valid until the user's password is changed
5. On subsequent requests, send as `Authorization: Basic <token>` header

**Request body**: `{"name": "user@example.com", "password": "..."}` — note the field is `name`, not `email`.

**Response body** (confirmed from `src/handler/http/request/users/mod.rs`):
```json
{"status": true, "message": "Login successful"}
```
On failure: `{"status": false, "message": "Invalid credentials"}`.

**Stored token format**: `Basic base64("user@example.com:password")`. The CLI must send this as `Authorization: Basic <token>` on every request.

**On token expiry/failure**: print `Authentication failed (401). Run 'o2 instance login <name>' to refresh.` with exit code 2. Do NOT attempt automatic refresh with a stored password — that creates a persistent credential that survives password rotation.

### 4.2 Keychain Abstraction

```go
type Store interface {
    Store(key, token string) error
    Get(key string) (string, error)
    Delete(key string) error
    List() ([]string, error)
}
```

- **macOS**: `security add-generic-password` / `security find-generic-password`
- **Linux**: `dbus-secret-service` via `keyring`; falls back to encrypted file with warning
- **Windows**: Windows Credential Manager via `keyring`

The key is: `oxygen/{user}/{org}@{host}` — includes username to prevent collision between users sharing a host.

### 4.3 Environment Variable Fallback

```
O2_URL      Base URL (e.g. https://cloud.openobserve.ai)
O2_ORG      Organization ID
O2_USER     User email (used for keychain key construction)
O2_TOKEN    Basic auth credential (`Basic base64(email:password)`) — bypasses keychain lookup entirely
O2_PASSWORD Password — used only for login request, never stored
O2_FORMAT   Default output format (json|pretty|table|log|csv)
```

> **O2_PASSWORD security**: Accepting the password as an env var is necessary for scripting and CI, but it is visible to other processes via `/proc/PID/environ` and inherited by child processes. Document this clearly. For interactive use, prefer the interactive prompt.

### 4.4 Token on the Command Line

The `--token` flag is provided for scripting convenience but:

- The token is visible in `ps aux`, process listings, and shell history
- For production scripts, prefer `O2_TOKEN` env var or keychain storage

---

## 5. Output Design

### 5.1 Format Modes

| Mode | Use case | Implementation |
|---|---|---|
| `json` | AI agents, scripting, piping to jq | `encoding/json` compact |
| `pretty` | Human inspection with whitespace | `encoding/json` indented |
| `table` | Tabular data (streams list, alert list) | `text/tabwriter` + custom header |
| `log` | Log entries (human) | Colorized ANSI, timestamped, field-aligned |
| `csv` | Export | `encoding/csv` writer |

Default: `json`. Override with `--format` or `O2_FORMAT` env var.

### 5.2 Log Renderer (`--format log`)

```
[2025-04-29 10:23:01.123] ERROR  [service=api] [host=prod-1] Request timeout after 30s
[2025-04-29 10:23:02.456] WARN   [service=api] [host=prod-1] Retrying (attempt 1/3)
[2025-04-29 10:23:03.789] INFO   [service=api] [host=prod-1] Request completed: 200 OK (234ms)
```

Color: ERROR=red, WARN=yellow, INFO=green, DEBUG=dim, timestamp=gray.

The log renderer auto-detects the severity field (looks for `level`, `severity`, `log_level` fields) and applies color. Falls back to monocolor if no severity field is present.

### 5.3 Metrics Renderer (`--format table`)

```
Query: rate(http_requests_total{service="api"}[5m]) at 2025-04-29T10:23:00Z

Labels                                      Value
service=api, method=GET, status=200       1423.50
service=api, method=POST, status=200        512.75
service=api, method=GET, status=500          3.12
```

### 5.4 Streaming Output

`o2 logs stream --follow` opens an HTTP/2 connection to `/_search_stream`, receives NDJSON, and prints results line-by-line as they arrive.

**Machine-friendly by default**: streaming writes raw NDJSON lines to stdout — no ANSI cursor control, no progress indicator, no TTY assumption. Output is safe to pipe (`|`), redirect (`>`), and capture into variables. This is critical for AI agent use.

**TTY-aware enrichment**: when stdout is a terminal (`isatty`), a live indicator `● streaming...` is printed to stderr. When stdout is a pipe or file, the indicator is suppressed automatically.

**Cancellation**: uses `context.WithCancel`; `SIGINT` (Ctrl+C) sends context cancellation, the HTTP stream is aborted cleanly, goroutines are joined before exit.

**Token limit**: `bufio.Scanner` default 64KB token limit is insufficient for large log entries. Set `bufio.Scanner.Buffer(nil, 1024*1024)` (1MB buffer) to handle oversized entries without silent truncation.

---

## 6. Technical Decisions

### 6.1 Dependencies

| Purpose | Library | Reason |
|---|---|---|
| CLI framework | `github.com/spf13/cobra` | standard, testable |
| HTTP client | stdlib `net/http` | no extra dep; wraps with retry |
| HTTP retry | `github.com/cenkalti/backoff/v4` | exponential backoff for GET/HEAD only; POST requests are not automatically retried to avoid duplicate side-effects |
| JSON | stdlib `encoding/json` | sufficient; avoid compatibility risk |
| Keychain | `github.com/99designs/keyring` | cross-platform; documented fallback |
| Tables | stdlib `text/tabwriter` | no dep; avoids unmaintained library |
| CSV | stdlib `encoding/csv` | no dep |
| Time parsing | `github.com/araddon/dateparse` | flexible relative time parsing |
| ANSI colors | `github.com/mattn/go-isatty` + stdlib | minimal dep |
| Shell completion | cobra built-in | no extra dep |

No external spinner, TUI, or animation dependencies unless phase 5 explicitly requires them.

### 6.2 Error Handling & Exit Codes

Errors print to stderr with exit codes differentiated by class so AI agents can branch on them programmatically:

| Exit code | Meaning | Agent action |
|---|---|---|
| 0 | Success | none |
| 1 | Usage/syntax error | fix command arguments |
| 2 | Auth failure (401) | run `o2 instance login <name>` |
| 3 | Forbidden (403) | check org/permissions |
| 4 | Not found (404) | check stream/resource name |
| 5 | Invalid request (400/422) | fix query syntax, see error message |
| 6 | Rate limited (429) | wait and retry |
| 7 | Server error (500/503) | retry with backoff |

Structured error response in JSON mode:

```json
// O2_FORMAT=json
{"error": "search failed", "code": 400, "message": "invalid SQL: near 'WHER'", "exit_code": 5}
```

In non-JSON mode: `Error: search failed (400): invalid SQL: near 'WHER' [exit 5]`

### 6.3 Dry-Run Mode

`--dry-run` is available on all query commands (`logs search`, `logs stream`, `metrics query`, etc.). It prints the resolved request without executing it:

```json
{
  "method": "POST",
  "url": "https://o2.example.com/api/default/_search",
  "headers": { "Authorization": "Basic <token>", "Content-Type": "application/json" },
  "body": {
    "query": {
      "sql": "SELECT * WHERE status = 'error' ORDER BY _timestamp DESC",
      "from": 0,
      "size": 100,
      "start_time": 1746038400000000,
      "end_time": 1746042000000000,
      "track_total_hits": true,
      "query_type": "sql"
    },
    "encoding": "",
    "regions": [],
    "timeout": 0,
    "use_cache": true
  },
  "resolved_time": {
    "start": "2025-04-30T18:00:00Z",
    "end": "2025-04-30T19:00:00Z",
    "start_us": 1746038400000000,
    "end_us": 1746042000000000
  }
}
```

The search response includes a `meta` field with the resolved time range:
```json
{
  "took": 155,
  "took_detail": {
    "total": 155,
    "cache_took": 0,
    "file_list_took": 10,
    "wait_in_queue": 0,
    "idx_took": 50,
    "search_took": 95
  },
  "columns": ["_timestamp", "log", "stream"],
  "hits": [
    {
      "_timestamp": 1674213225158000,
      "log": "[2023-01-20T11:13:45Z INFO  actix_web] 10.2.80.192 POST /api/demo",
      "stream": "stderr"
    }
  ],
  "total": 27179431,
  "from": 0,
  "size": 100,
  "scan_size": 28943,
  "cached_ratio": 0,
  "is_partial": false,
  "trace_id": "abc123",
  "function_error": []
}
```

Key response fields:
- `took` (usize) — total query time in **milliseconds**
- `took_detail` (object) — breakdown of timing stages
- `columns` (string[]) — ordered list of field names in hit objects
- `hits` (json.Value[]) — array of log records, each field accessible by name
- `total` (usize) — total matching records (respects `track_total_hits`)
- `from` / `size` (i64) — pagination echo
- `scan_size` (usize) — bytes scanned
- `cached_ratio` (usize) — percentage of results from cache
- `is_partial` (bool) — true if results are partial (capped at limit)
- `trace_id` (string) — server-assigned trace ID for debugging
- `function_error` (string[]) — VRL function errors, if any
- `new_start_time` / `new_end_time` (i64|null) — adjusted time range if server modified it (e.g., query range restriction)

The CLI adds `meta` to the output as a convenience wrapper that includes both the server-provided fields and the resolved time range:
```json
{
  "hits": [...],
  "meta": {
    "total": 27179431,
    "took_ms": 155,
    "resolved_start": "2025-04-30T18:00:00Z",
    "resolved_end": "2025-04-30T19:00:00Z",
    "resolved_start_us": 1746038400000000,
    "resolved_end_us": 1746042000000000
  }
}
```
This `meta` wrapper is added by the CLI, not returned by the API. The API returns the flat response shown above. The CLI's `--dry-run` output uses the same shape as the actual API request body.

AI agents use `--dry-run` to:
- Verify resolved microsecond timestamps before executing
- Reproduce exact queries from a previous run
- Debug query construction without burning API quota

### 6.4 Request Batching

**Retry safety**: only safe (idempotent) HTTP methods — GET, HEAD, DELETE — are automatically retried on transient failures (5xx, network timeout). POST requests (search, ingestion, write operations) are never retried automatically — if the response is lost, re-sending could produce duplicate results. The agent must decide to retry POST requests explicitly.

### 6.5 Request Batching

For multi-stream exploration, commands accept comma-separated names:

```bash
o2 logs search --stream "log1,log2,log3" --sql "SELECT * WHERE error"
```

The CLI translates this to a single `/_search_multi` request internally.

---

## 7. Implementation Phases

### Phase 1: Foundation

- Project scaffold (go.mod, Makefile, mise.toml, tools.go)
- `internal/config/config.go` — env vars + config file
- `internal/auth/store.go` — keychain abstraction + Linux fallback warning
- `internal/auth/context.go` — resolution + env var priority
- `internal/api/client.go` — http.Client + auth middleware + backoff retry
- `cmd/o2/root.go` — global flags + config wiring
- `o2 instance add/remove/list/use/current/login` (Phase 6)

### Phase 2: Core Querying

- `internal/api/search.go` — search / search_multi / search_stream types
- `internal/api/auth.go` — login API
- `internal/output/table.go` + `log.go` + `csv.go` + `metrics.go`
- `o2 logs search` (SQL + filter modes, pagination, `--dry-run`)
- `o2 logs stream --follow` (streaming: raw NDJSON to stdout, isatty-aware indicator)
- `o2 logs values` (field autocomplete)
- `o2 logs history`
- `o2 logs views list/get`
- `o2 streams describe <name>` — stream schema as JSON (critical for AI agents to introspect fields before writing queries)

### Phase 3: Metrics

- `internal/api/promql.go` — promql query / query_range / series / label-values
- `o2 metrics query` (PromQL instant + range, unified `--start`/`--end`/`--step`)
- `o2 metrics series`
- `o2 metrics label-values`
- `o2 metrics search` (SQL over metrics stream)
- `o2 metrics streams list/describe`

### Phase 4: Supporting Commands

- `o2 streams list` (describe was moved to Phase 2)
- `o2 traces latest/dag/search`
- `o2 dashboards list/get`
- `o2 alerts list/get/history/trigger` (all `/v2/` prefixed endpoints)
- `o2 alerts incidents list`
- `o2 alerts templates list`
- `cmd/o2/config.go` — `o2 config show`

### Phase 5: Polish

- Shell completion (bash/zsh/fish)
  - Fish and Zsh completions are **automatically installed** via Homebrew when installing via `brew install beeemT/tap/oxygen`.
  - Bash completions are included in the tarball but require manual installation (`source` in `.bashrc`).
- `o2 dashboards create/update/delete`
- `o2 alerts create/update/delete`
- `o2 logs views create/delete`
- `--watch` flag for polling-based continuous output
- README + man page generation
- SKILL.md file
- Binary release workflow (macOS arm64/amd64 + Linux amd64)
- **Homebrew tap integration**: on every version tag, the release workflow generates `Formula/oxygen.rb` and pushes it to `beeemT/homebrew-tap` (the same pattern used by `beeemT/git-work`). Users install with `brew install beeemT/tap/oxygen`; the installed binary is named `o2`. Fish and Zsh shell completions are installed automatically; Bash requires manual setup.

### Phase 6: Multi-Instance Support

**Goal**: Support managing and switching between multiple named OpenObserve instances.

#### 6.1 `internal/instances/instances.go`

```go
type Instance struct {
    Name string
    URL  string
    Org  string
    User string
}

type Manager struct {
    instances   map[string]Instance
    currentName string
    configPath  string
}

func NewManager(configPath string) (*Manager, error)
func (m *Manager) Add(name, url, org, user string) error
func (m *Manager) Remove(name string) error
func (m *Manager) Get(name string) (Instance, error)
func (m *Manager) SetCurrent(name string) error
func (m *Manager) Current() (Instance, bool, error)
func (m *Manager) List() []Instance
```

- Load/save from YAML in the config directory (`~/.config/oxygen/instances.yaml`)
- Validate unique names on add
- `SetCurrent` errors if instance does not exist
- `Current` returns `(instance, hasCurrent, error)` where `hasCurrent=false` means no default set
- `Remove` also deletes the stored keychain credential for that instance

#### 6.2 `cmd/o2/instance.go` — instance commands

```
o2 instance add <name> --url <url> --org <org> --user <user> [--password <pw>]
o2 instance remove <name>
o2 instance list
no2 instance use <name>     # set current default
no2 instance current        # show current default
no2 instance login <name>  # (re-)authenticate an existing instance
```

**`o2 instance add`**:
- `--url`, `--org`, `--user` are required flags
- If `--password` is given or `O2_PASSWORD` is set, performs login immediately and stores the credential
- If no password is provided, the instance is created unauthenticated; the user must run `o2 instance login <name>` later

**`o2 instance remove`**:
- Removes the instance from `instances.yaml`
- Also removes the stored keychain credential for that instance

**`o2 instance login`**:
- Re-authenticates an existing instance (password changed, session expired, etc.)
- Prompts for password interactively if not given via `--password` or `O2_PASSWORD`
- Errors if the instance does not exist

**`o2 instance list`**:
- Lists all instances, marking the current one with `*`

**`o2 instance use`**:
- Sets the current default instance (stored in `instances.yaml`)
- Errors if the instance does not exist

**`o2 instance current`**:
- Prints the current default instance name, or nothing if none is set

#### 6.3 `cmd/o2/root.go` — global `--instance` flag

Add global `--instance` flag:
```go
fs.String("instance", "", "Instance name (overrides current default)")
```

Instance resolution happens in `initConfig` via `PersistentPreRunE`. The manager is lazily initialized with `sync.Once` to avoid initialization order issues. When `--instance` is set, the named instance's URL/org/user override the merged config values. When no flag is set, the current default from `instanceMgr.Current()` is used if available.

```go
// Lazily initialise instance manager once.
var initInstanceMgrOnce sync.Once
initInstanceMgrOnce.Do(func() {
    configDir := configFileDir()
    path := filepath.Join(configDir, "instances.yaml")
    instanceMgr, err = instances.NewManager(path)
    if err != nil {
        fmt.Fprintf(os.Stderr, "WARNING: failed to load instances: %v\n", err)
    }
})

// Resolve instance from --instance flag or current default.
instanceName, _ := cmd.Flags().GetString("instance")
var inst instances.Instance
hasInstance := false
if instanceName != "" {
    inst, err = instanceMgr.Get(instanceName)
    if err != nil {
        return fmt.Errorf("instance %q not found. ...", instanceName, instanceName)
    }
    hasInstance = true
} else {
    inst, hasInstance, err = instanceMgr.Current()
    if err != nil {
        return fmt.Errorf("loading current instance: %w", err)
    }
}
if hasInstance {
    if inst.URL != ""  { cfg.URL = inst.URL }
    if inst.Org != ""  { cfg.Org = inst.Org }
    if inst.User != "" { cfg.User = inst.User }
}
```

The `instanceMgr` package variable is exposed so command files can call `instanceMgr.Current()` (e.g., in `o2 config show`).

#### 6.4 `internal/auth/context.go`

`Resolver.Resolve` is unchanged — it accepts only a `context.Context`. Instance selection happens in `initConfig` before any command runs: the resolved instance values are written directly into `cfg.URL/Org/User`, so `Resolve` sees the correct values without needing an instance name parameter.

#### 6.5 `cmd/o2/root.go` — remove `auth` commands

`cmd/o2/auth.go` does not exist. Authentication is handled exclusively through `o2 instance add` (with `--password`) and `o2 instance login`.

#### 6.6 Command propagation

Child commands (`logs`, `metrics`, `traces`, `streams`, `dashboards`, `alerts`) call `resolveClient()` which reads from the already-merged `cfg` global. Since `initConfig` runs via `PersistentPreRunE` before every command, the cfg values already reflect the instance override — no explicit `--instance` flag propagation is needed. Cobra's flag inheritance means `viper.GetString("instance")` is available in any subcommand if needed.

---

## 8. Acceptance Criteria

1. `O2_PASSWORD=... o2 instance add prod --url $O2_URL --org $O2_ORG --user $O2_USER` stores token in keychain under `oxygen/{user}/{org}@{host}`; `o2 instance list` shows the new instance.
3. `o2 logs search --stream mylogs --sql "SELECT *" --format log --start=5m` renders human-readable log lines with colored severity and aligned fields.
4. `o2 metrics query --expr 'up' --start=now` returns PromQL results in the default JSON format.
5. `o2 logs stream --stream mylogs --sql "SELECT *" --follow` streams NDJSON results; `Ctrl+C` exits cleanly without goroutine leak.
6. `o2 streams list` shows a formatted table with stream name, type, doc count, and storage size.
7. `o2 alerts list` uses the correct `/v2/{org_id}/alerts` endpoint (verified by `curl -v` against a test instance).
8. On a Linux machine without dbus, `o2 instance add` prints `WARNING: System keychain unavailable; credentials stored in ~/.config/oxygen/credentials.json.`
9. Build produces a single static binary for macOS (arm64) with no external runtime dependency.
10. `make lint test` passes on each commit.
11. `o2 logs search --stream mylogs --sql "SELECT *" --dry-run` outputs the resolved request body with microsecond timestamps, no HTTP call is made.
12. `o2 streams describe mylogs --format json` returns the stream schema as JSON (fields, types, indexed status) in Phase 2.
13. `o2 logs stream --stream mylogs --sql "SELECT *" --follow > out.ndjson` redirects raw NDJSON lines to a file with no TTY artifacts.
14. A 401 response from any command exits with code 2; a 404 exits with code 4; a 429 exits with code 6; a 500 exits with code 7.
15. Search response JSON includes a `meta` field with `resolved_start`, `resolved_end`, and `took_ms`.
16. After `brew install beeemT/tap/oxygen`, the `o2` binary is available and `o2 --help` succeeds without external dependencies.
