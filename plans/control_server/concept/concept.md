# Concept: Peek Control Server

> **Status:** Draft
> **Author:** Kevin Horst
> **Date:** 2026-07-06

---

## Goals

- A human-facing local dashboard plus JSON API over the same in-memory `session.Store`, with live updates — usable while Peek runs in **either** transport (streamable HTTP or stdio/mcpb).
- Zero build toolchain: a single static HTML/JS dashboard embedded via `go:embed`; scriptable with `curl` + `jq`.
- Safe by default: opt-in enable, localhost-only bind, optional token, read-only.

---

## User Flows

### Watch sessions in the browser

**Goals:**
- See all sessions (agent, title, last activity, plan/diff availability) and drill into turns, plan, and diff — updating live as agents work.

**Options:**

**MVP**
- `peek start --control-port 4243` → `http://127.0.0.1:4243/` shows the session list, newest first, updating live via SSE.
- Clicking a session shows the last N turns, the plan (rendered markdown), and the diff (plain `<pre>`), refreshed on events.
- Works identically when Claude Desktop launched Peek over stdio — the control listener is independent of the MCP transport.

**Backlog**
- Diff syntax highlighting, usage charts (after the [usage_reporting](../usage_reporting/concept.md) Codex fix, otherwise Codex numbers are wrong), transcript search, desktop notifications ("Codex finished a task").

**Challenges:**
- `Store.TurnAdded` is a single-consumer `chan Id` (cap 16, drop-on-full) consumed only by the diff watcher — feeding SSE from it would starve one side.
- `UpdatePlanForPath` / `UpdateDiff` / `UpdateUncommittedDiff` mutate silently; live plan/diff updates need those paths to emit events.
- In stdio mode nothing listens on HTTP today; the control server must be its own `http.Server` goroutine.
- Diffs can be MBs; the API must truncate by default.

**Approach:**
- Introduce an `events.Broker` (pub/sub): the store publishes typed events; the diff watcher and each SSE client are subscribers. `Store.TurnAdded` is replaced by a broker subscription (repo-internal, no shim needed unless open question 1 says otherwise).

### Script against the API

**Goals:**
- `curl :4243/api/sessions | jq` and `curl -N :4243/api/events` work out of the box.

**Options:**

**MVP**
- Read-only JSON API mirroring the MCP tool surface (sessions, turns, plan, diff, uncommitted diff, usage) plus an SSE event feed.

**Backlog**
- `PUT /api/sessions/{id}/title` — user-assigned titles (closes the [codex_title_search](../codex_title_search/concept.md) backlog item), persisted in `~/.peek/titles.json`.
- Read-only remote binding (`--control-bind`) with mandatory token.

**Challenges:**
- Keeping API truncation behavior consistent with MCP tool `size` semantics.

**Approach:**
- Same defaults philosophy: bounded by default, `size=0` opt-in full output.

---

## Decisions / Open Questions

**Decisions:**
- **Dedicated `--control-port` (env `PEEK_CONTROL_PORT`), default `0` = disabled; docs suggest 4243.** Rejected: sharing the MCP mux on :4242 — it only exists in http transport, so stdio mode would need the standalone server anyway, and it mixes MCP request logging/auth with browser traffic. (Mounting the control handler on the MCP port under http transport is possible later as sugar.)
- **SSE, not WebSocket**: unidirectional server→browser fits exactly, zero dependencies, `curl -N`-friendly.
- `events.Broker` with per-subscriber buffered channels (cap 16, drop-on-full — identical semantics to today's `TurnAdded`: "dropped notifications are fine, the next turn re-triggers"). Event types: `turn_added`, `plan_updated`, `diff_updated`, `uncommitted_diff_updated`, `session_created`.
- Bind hardcoded to `127.0.0.1` in MVP (matching the MCP server); `--control-bind` is backlog and forces token auth.
- Dashboard = one embedded `index.html` + vanilla JS (`fetch` + `EventSource`); no framework, no npm.
- API is read-only in MVP.
- Events carry id-only payloads; clients re-fetch. Keeps SSE frames tiny and truncation logic in one place.

**Open Questions:**
1. Keep `Store.TurnAdded` as a deprecated shim for one release, or replace outright? (Repo-internal only → proposal: replace.)
2. Token delivery for the browser when auth is enabled: `?token=` on first load setting an HttpOnly SameSite=Strict cookie (proposal), or manual header only?
3. Should the mcpb bundle default `PEEK_CONTROL_PORT=4243` on, so Claude Desktop users get the dashboard for free? (Discoverability vs. surprise open port.)
4. Is later mounting on the MCP port under http transport (rejected Option A as sugar) worth it?
