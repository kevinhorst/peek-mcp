# Control Server HTTP API & Eventing

---

## Flows

### Startup

1. User runs `peek start --control-port 4243` (or sets `PEEK_CONTROL_PORT=4243`), any transport.
2. Backend
   1. `cmd/start.go`: after watchers start, if `controlPort > 0`, create `broker := events.NewBroker()`.
   2. Pass the broker into `session.NewStore` (store gains `publish(ev)`), `NewDiffWatcher` (subscribes instead of reading `TurnAdded`), and `control.NewServer(store, broker, token)`.
   3. `go controlServer.ListenAndServe("127.0.0.1:<controlPort>")` with ctx-driven graceful shutdown, mirroring the existing MCP `http.Server` pattern. Runs in both `stdio` and `http` transports.

### Live dashboard update

1. Browser loads `/` (embedded), fetches `/api/sessions`, opens `EventSource('/api/events')`.
2. A watcher parses a new turn.
3. Backend
   1. `Store.AddTurnBySessionId` → `broker.Publish(Event{Type: "turn_added", SessionId, Agent, Ts})`.
   2. Broker fans out: the diff watcher recomputes the diff, then `UpdateDiff` publishes `diff_updated`; each SSE subscriber's handler writes `event: turn_added\ndata: {json}\n\n` and flushes.
   3. Broker sends `: heartbeat\n\n` every 15 s; a write error marks the client dead → unsubscribe.
4. Dashboard patches the session row and re-fetches the open session's turns.

### Scripted event consumption

1. `curl -N http://127.0.0.1:4243/api/events?agent=codex`
2. Backend subscribes with the agent filter; only matching events are written to that client.

---

## APIs

| Method | Path | Params | Returns |
|---|---|---|---|
| GET | `/`, `/assets/*` | — | Embedded dashboard |
| GET | `/api/healthz` | — | `{"status":"ok","version":...}` |
| GET | `/api/sessions` | `agent` (claude/codex), `limit` (default 50, max 200), `title` (substring filter, reuses [codex_title_search](../codex_title_search/index_watcher.md) matching) | `{"sessions":[SessionSummary]}` |
| GET | `/api/sessions/{id}` | — | `SessionDetail` |
| GET | `/api/sessions/{id}/turns` | `n` (default 5, max = `--depth`) | `{"turns":[Turn]}` |
| GET | `/api/sessions/{id}/plan` | — | `{"plan_content":..., "plan_file_path":...}`; 404 if none |
| GET | `/api/sessions/{id}/diff` | `size` (default 262144, 0 = full) | `{"target":"main","diff":...,"truncated":bool}` |
| GET | `/api/sessions/{id}/uncommitted-diff` | `size` (same) | `{"diff":...,"truncated":bool}` |
| GET | `/api/sessions/{id}/usage` | — | Usage totals (shape from [usage_reporting](../usage_reporting/usage_tool.md)) |
| GET | `/api/events` | `agent` (optional filter) | SSE stream |

Title lookup is deliberately not a path segment (ids are links in the UI); `?title=` on `/api/sessions` filters instead.

---

## Models

### SessionSummary

**Public:**
- id, agent, title, title_source, last_active, cwd, git_branch, model, has_plan, has_diff, has_uncommitted_diff

### SessionDetail

**Public:**
- SessionSummary fields + total_usage, diff_target

### events.Event

**Public:**
- type: `turn_added` | `plan_updated` | `diff_updated` | `uncommitted_diff_updated` | `session_created` (string)
- session_id: affected session (string)
- agent: claude | codex (string)
- ts: event time (time.Time)

Id-only payloads — clients re-fetch content.

### events.Broker

**Internal / Not Exported:**
- Subscribe(filter) → (<-chan Event, cancel func); per-subscriber buffer 16, drop-on-full
- Publish(Event): non-blocking fan-out

---

## Security Considerations

- **Bind 127.0.0.1 only** (MVP hardcoded). Transcripts and diffs are highly sensitive — they may contain secrets pasted into prompts.
- **DNS-rebinding defense**: reject requests whose `Host` is not `localhost` / `127.0.0.1` / `[::1]` — a malicious website resolving to 127.0.0.1 could otherwise read the API cross-origin via simple GETs.
- **No CORS headers at all**: the same-origin dashboard needs none; their absence blocks cross-origin JS reads.
- Optional `--control-token` / `PEEK_CONTROL_TOKEN`: `Authorization: Bearer` or `?token=` → HttpOnly SameSite=Strict cookie (concept open question 2). Mandatory once `--control-bind` (backlog) is non-loopback.
- Read-only in MVP: no state mutation, no command-execution surface. The `{id}` path param is used only for map lookup — never filesystem or git.
- Request logging follows the existing `requestLogger` pattern but **without body logging** on control endpoints (avoids token leakage in debug logs).

---

## Limits

- SSE clients: max **16**, further connects get 429 (localhost dashboard tool; prevents goroutine/file-descriptor creep from stuck `curl`s).
- Event buffer: **16 per subscriber, drop-on-full** — identical to today's `TurnAdded` semantics; a dropped event is healed by the next one or a re-fetch.
- Diff truncation default: **256 KB** (browsers render fine; matches the `size` spirit of the MCP tools); `size=0` opt-in full.
- `turns` max = `--depth` (default 20) — the store keeps no more anyway.
- `limit` max 200 sessions — protects JSON size; a local machine rarely exceeds dozens.
- Heartbeat 15 s; write timeout 10 s per SSE flush.

---

## Infrastructure

- New flags `--control-port`, `--control-token` (+ env `PEEK_CONTROL_PORT`, `PEEK_CONTROL_TOKEN`) in `cmd/start.go`; surfaced in the mcpb manifest config UI (default off, pending concept open question 3).
- New packages: `control` (HTTP server + embedded assets) and `events` (broker).
- `watcher/diff_watcher.go` migrates from `store.TurnAdded` to a broker subscription; store publishes from `AddTurnBySessionId`, `UpdatePlanForPath`, `UpdateDiff`, `UpdateUncommittedDiff`, and `getOrCreate` (for `session_created` — needs a hook).

---

## Long-Tail Tasks

### Write operations

- `PUT /api/sessions/{id}/title` + `~/.peek/titles.json` persistence so user titles survive restarts.

### Serving

- Mount the control handler on the MCP port under http transport (sugar).
- Gzip static assets; ETag on `/api/sessions`.
- Graceful shutdown ordering: broker close vs `http.Server.Shutdown`.

### Dashboard

- Usage sparkline once the usage_reporting Codex fix lands.
- Distinct `session_created` handling (new row animation) vs first `turn_added`.

### Testing

- Broker: fan-out, drop-on-full, unsubscribe on dead client.
- Host-header check table test; token auth (header, query→cookie).
- SSE integration test with `httptest` + a fake store event.
