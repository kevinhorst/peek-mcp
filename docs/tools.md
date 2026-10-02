[← peek-mcp](../README.md)

# MCP tools

Full parameter reference for every tool peek-mcp exposes. For task-oriented walkthroughs, see the [use cases](../README.md#use-cases).

## Common semantics

- **Title matching** — a `title` is matched exact-first (case-insensitive), then falls back to substring match. When `agent` is provided, matching is scoped to that agent. For Codex, titles come from Codex's session index (the thread name).
- **Pagination** — responses that carry turns or diffs are paginated by the client's capability. When a response has `has_more: true`, call the same tool again with the returned `request_id` to fetch the next page. Chunked payload fields (`turns`, `events`, `revisions`) are JSON-encoded strings split at page boundaries — concatenate the chunks across pages, then parse the result. `json: true` responses are never paginated.
- **Startup wait** — right after peek starts, tool calls wait until the initial session load (all transcripts inside the watch window) is complete, so the first answer is never a partial session list. If the load takes longer than 2 minutes, the call returns `initial session load still in progress; retry shortly`.
- **Most-recent default** — `session_get` uses the most recently active session when `id` and `title` are omitted (an `agent` is then required when more than one agent is enabled, so the lookup knows which side to read).

## `session_get`

Returns session data (turns, events, plan, git diff, uncommitted diff, auto-memory) for a session in one call. Sections are selected with flat boolean flags. Turns are returned as the last N human/assistant turn pairs; tool calls are omitted unless `tools` is set. Assistant thinking is captured but omitted unless `thinking` is set; `subagent` scopes the whole response to one subagent's transcript.

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
| `subagent` | string | no | Subagent id: scope the response to that agent's transcript and actor-tagged events (plan/diff/memory sections are omitted). Valid ids are listed in every response's `subagents` field |
| `thinking` | boolean | no | Include assistant thinking text on turns (default `false`) |
| `tools` | boolean | no | Include each turn's tool calls as `tool_calls` (default `false`). Turns with tool calls but no text are returned too and count toward `n`. Claude sessions only; Codex turns carry an empty list |
| `request_id` | string | no | Pagination request ID from a previous response |
| `json` | boolean | no | Return the full typed response as structuredContent, unpaginated — sections are real JSON objects instead of chunked strings (default `false`: paginated JSON text block) |

The first page also carries `total_usage`, the main chain's running token total (including the in-flight turn; subagent tokens never enter it), `last_active`, the main chain's latest transcript timestamp, `plan_revisions` (`count` and `timestamps`, the same block `session_events` returns; absent when the session has no plan), `subagents` — every spawned subagent's `agent_id`, `agent_type`, `description`, `model`, `last_active` and, for Claude sessions, `usage` (its own token total), always present so a follow-up call can scope to one of them — and `turns_total`, the number of turns in the addressed scope (the session, or the `subagent`) including the in-flight turn and turns the ring no longer holds. Without `tools` it counts the turns `turns` can return, with `tools` it also counts tool-only turns. A scope returned fewer turns than `turns_total` was cut by `n` or by the ring.

With every section flag off, `session_get` returns only these fields: the cheap call for a monitoring loop. Subagent work shows as movement in `subagents[].usage` and `subagents[].last_active` while `total_usage` and `last_active` stay flat; the latest activity anywhere is the max of `last_active` and every `subagents[].last_active`.

With `tools`, every turn carries `tool_calls`, in call order and empty when the turn made none. Each entry has `id` (the tool-use id), `name`, `input` (the tool's raw input object, verbatim from the transcript), `is_error` (from the matching tool result; `false` while the call is still running) and `timestamp` (when the call was emitted). Scoped with `subagent`, the same holds for Agent-tool subagents and workflow agents. The session keeps at most `--depth` turns and each subagent at least 200 ([reference](reference.md)); raise `--depth` to keep long runs complete.

## `session_events`

Returns the typed event stream of a session (plan lifecycle, permission denials/grants, permission-mode changes, skill invocations, subagent spawns/results, background task completions, user answers) plus derived counters (`counters.permission_denials` counts every denied tool call the transcript records — hook, settings, safety check, unanswered approval, auto-mode — on the main chain and in every subagent; `counters.permission_cancellations` counts calls the user cancelled or interrupted), a `permissions` block that reconciles that ledger with [telemetry](reference.md#telemetry): `denied` (the ledger), `denied_by_kind`, `denied_by_source` (hook, config, prompt, aborted — from the telemetry decision with the same tool use id), `unattributed` (denials with no telemetry decision: mostly approval prompts a headless session could not answer, which Claude Code exports no event for), `telemetry_only` (rejects with no transcript denial); the allow counters and `requests` come from telemetry alone, and `detail` says `persisted` or `transcript-only` when live telemetry is missing, token usage totals, session time (`time` block: `started_at`, `last_active`, wall/idle/active seconds — idle is the sum of gaps ≥ 5 minutes between transcript timestamps; a `telemetry` sub-block with true active seconds and cost appears when [telemetry export](reference.md#telemetry) is enabled), touched files (`touched_files`: per-path read/write counts from Read/Write/Edit tool results, subagent touches included), plan revision history, and diff availability (`live` \| `snapshot` \| `none`). Turns are not included — use `session_get` for those. The `unsupported` array lists signals not detectable for the session's agent.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | no | Session ID (omit for most recent session) |
| `title` | string | no | Session title (see [title matching](#common-semantics)) |
| `agent` | string | no | Agent: `claude` or `codex`. Required when `id` and `title` are omitted |
| `revisions` | boolean | no | Include plan revision diffs (default `false`; they dominate response size) |
| `breakdown` | boolean | no | Include per-skill and per-subagent time and token usage (default `false`; Claude sessions only). A skill window spans from its invocation to the next user prompt or next skill; skill usage counts main-loop tokens only — subagent tokens are listed per subagent and never enter the session's `usage` totals |
| `subagent` | string | no | Subagent id: scope events (by actor) and the `breakdown` stats to that agent. Valid ids are listed in every response's `subagent_ids` field |
| `request_id` | string | no | Pagination request ID from a previous response |
| `json` | boolean | no | Return the full typed response as structuredContent, unpaginated — sections are real JSON objects instead of chunked strings (default `false`: paginated JSON text block) |

`permission_denied` comes from the transcript's per-result `toolDenialKind` and carries `permission.kind` (`permission-rule`, `user-rejected`, `automode-blocked`, `cancelled`, `interrupted`), the tool, the command and `tool_use_id`. A rejected ExitPlanMode call is `plan_rejected`, never a denial. Claude Code does not tag file-path deny rules on Read/Edit/Write, so those are not counted; transcripts from CLI versions before 2.1.202 carry no kind and report no denials.

The event list keeps a session's first 2000 `permission_denied` events and its first 1000 events of all other kinds; the `counters` stay exact beyond that, so a counter higher than the listed events means the budget ran out.

A background Agent-tool subagent's `subagent_result` is its launch acknowledgement (content starts `Async agent launched`), not its completion. Its completion is a `task_completed` event with `task` `{task_id, tool_use_id, status, summary}`, where `task_id` is the agent id and `status` is terminal (`completed`, `failed`, `killed`, `stopped`, …). Background shell commands and workflow runs emit `task_completed` too, with their own task ids. Workflow agents report their completion as `subagent_result`.

## `session_list`

Lists all sessions. Returns session ID, agent, title, title source (`custom` \| `index` \| `derived`), last activity timestamp, whether a plan or diff is available, the inferred diff base (`diff_target`), and session metadata (cwd, git branch, model, origin).

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `agent` | string | no | Agent: `claude` or `codex`. Lists all sessions when omitted |
| `project` | string | no | Exact project label filter. Lists all sessions when omitted |
| `window_days` | number | no | Widen this instance's watch window to this many days before listing; at least 1. The wider window stays until the instance exits, and the call waits while the added days load. A stdio instance starts with 3 days |

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
| Tool calls | per turn via `tools`, root and every subagent | not available |
| Thinking | captured, returned via `thinking` | not available |
| Sub-agent sessions | folded into the parent; per-agent transcript via `subagent` | folded into the parent; per-agent transcript via `subagent` |
| Session time | wall/idle/active from transcript timestamps | wall/idle/active from transcript timestamps |
| Touched files | per-path read/write counts from file-tool results | not available |
| Skill/subagent usage | per-skill and per-subagent time + tokens via `breakdown` | not available |
| Telemetry | OTLP receiver enriches `time` with active seconds + cost | not available |
| Pagination | by client capability | by client capability |
