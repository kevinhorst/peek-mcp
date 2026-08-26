[← peek-mcp](../README.md)

# MCP tools

Full parameter reference for every tool peek-mcp exposes. For task-oriented walkthroughs, see the [use cases](../README.md#use-cases).

## Common semantics

- **Title matching** — a `title` is matched exact-first (case-insensitive), then falls back to substring match. When `agent` is provided, matching is scoped to that agent. For Codex, titles come from Codex's session index (the thread name).
- **Pagination** — responses that carry turns or diffs are paginated by the client's capability. When a response has `has_more: true`, call the same tool again with the returned `request_id` to fetch the next page. Chunked payload fields (`turns`, `events`, `revisions`) are JSON-encoded strings split at page boundaries — concatenate the chunks across pages, then parse the result. `json: true` responses are never paginated.
- **Most-recent default** — `session_get` uses the most recently active session when `id` and `title` are omitted (an `agent` is then required when more than one agent is enabled, so the lookup knows which side to read).

## `session_get`

Returns session data (turns, events, plan, git diff, uncommitted diff, auto-memory) for a session in one call. Sections are selected with flat boolean flags. Turns are returned as the last N human/assistant turn pairs; tool calls and tool results are filtered out.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | no | Session ID (omit for most recent session) |
| `title` | string | no | Session title (see [title matching](#common-semantics)) |
| `agent` | string | no | Agent: `claude` or `codex`. Required when `id` and `title` are omitted and more than one agent is enabled |
| `n` | number | no | Number of turns to return (default 20). Only applies to the turns section |
| `turns` | boolean | no | Return the session turns (default `true`) |
| `events` | boolean | no | Return the compact one-line event entries (default `true`) — interleave them with turns by timestamp; the full typed event stream lives in `session_events` |
| `plan` | boolean | no | Return the session plan (default `true`). For Claude sessions this is the plan-mode plan file; for Codex the latest `proposed_plan` block |
| `diff` | boolean | no | Return the pre-computed merge-base diff against the inferred base branch, reported as `diff_target` (default `true`) |
| `uncommitted_diff` | boolean | no | Return the live `git diff HEAD` in the session's own working tree, refreshed as files are saved (default `false`) |
| `remember` | boolean | no | Include the project's auto-memory (`MEMORY.md` + fact files). Claude sessions only (default `false`) |
| `request_id` | string | no | Pagination request ID from a previous response |
| `json` | boolean | no | Return the full typed response as structuredContent, unpaginated — sections are real JSON objects instead of chunked strings (default `false`: paginated JSON text block) |

The first page also carries `total_usage`, the running token total (including the in-flight turn).

## `session_events`

Returns the typed event stream of a session (plan lifecycle, permission denials/grants, permission-mode changes, skill invocations, subagent spawns/results, user answers) plus derived counters, a `permissions` block with telemetry-based permission decisions when [telemetry export](reference.md#telemetry) is enabled (auto-allowed vs. prompted vs. rejected counts, plus each prompted/rejected request with its tool and command — `detail: "persisted"` marks stats read back from the state dir), token usage totals, session time (`time` block: `started_at`, `last_active`, wall/idle/active seconds — idle is the sum of gaps ≥ 5 minutes between transcript timestamps; a `telemetry` sub-block with true active seconds and cost appears when [telemetry export](reference.md#telemetry) is enabled), touched files (`touched_files`: per-path read/write counts from Read/Write/Edit tool results, subagent touches included), plan revision history, and diff availability (`live` \| `snapshot` \| `none`). Turns are not included — use `session_get` for those. The `unsupported` array lists signals not detectable for the session's agent.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | no | Session ID (omit for most recent session) |
| `title` | string | no | Session title (see [title matching](#common-semantics)) |
| `agent` | string | no | Agent: `claude` or `codex`. Required when `id` and `title` are omitted |
| `revisions` | boolean | no | Include plan revision diffs (default `false`; they dominate response size) |
| `breakdown` | boolean | no | Include per-skill and per-subagent time and token usage (default `false`; Claude sessions only). A skill window spans from its invocation to the next user prompt or next skill; skill usage counts main-loop tokens only — subagent tokens are listed per subagent and never enter the session's `usage` totals |
| `request_id` | string | no | Pagination request ID from a previous response |
| `json` | boolean | no | Return the full typed response as structuredContent, unpaginated — sections are real JSON objects instead of chunked strings (default `false`: paginated JSON text block) |

## `session_list`

Lists all sessions. Returns session ID, agent, title, title source (`custom` \| `index` \| `derived`), last activity timestamp, whether a plan or diff is available, the inferred diff base (`diff_target`), and session metadata (cwd, git branch, model, origin).

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `agent` | string | no | Agent: `claude` or `codex`. Lists all sessions when omitted |

## Supported agents

| Agent | Session path |
|-------|-------------|
| Claude Code | `~/.claude/projects/<encoded-cwd>/*.jsonl` |
| Codex CLI | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` |

On Windows the session roots resolve to `%USERPROFILE%\.claude` and `%USERPROFILE%\.codex`.

## Agent parity

| Capability | Claude Code | Codex |
|---|---|---|
| Title | explicit custom titles | session index thread names |
| Plan | plan-mode plan file (watched live) | latest `proposed_plan` block |
| Git metadata | branch per entry | branch, commit hash, repo URL from `session_meta` |
| Client metadata | CLI version | originator, CLI version, source, fork lineage |
| Model | per assistant message | per turn context |
| Token usage | summed per message | cumulative snapshots, kept-last; accurate totals (incl. in-flight turn) |
| Tool calls | filtered out | filtered out |
| Sub-agent sessions | hidden (sidechains) | hidden (sub-agent rollouts) |
| Session time | wall/idle/active from transcript timestamps | wall/idle/active from transcript timestamps |
| Touched files | per-path read/write counts from file-tool results | not available |
| Skill/subagent usage | per-skill and per-subagent time + tokens via `breakdown` | not available |
| Telemetry | OTLP receiver enriches `time` with active seconds + cost | not available |
| Pagination | by client capability | by client capability |
