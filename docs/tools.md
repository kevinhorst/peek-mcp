[← peek-mcp](../README.md)

# MCP tools

Full parameter reference for every tool peek-mcp exposes. For task-oriented walkthroughs, see the [use cases](../README.md#use-cases).

## Common semantics

- **Title matching** — a `title` is matched exact-first (case-insensitive), then falls back to substring match. When `agent` is provided, matching is scoped to that agent. For Codex, titles come from Codex's session index (the thread name).
- **Pagination** — responses that carry turns or diffs are paginated by the client's capability. When a response has `has_more: true`, call the same tool again with the returned `request_id` to fetch the next page.
- **Most-recent default** — tools that accept `id` use the most recently active session when `id` is omitted (an `agent` is then required so the lookup knows which side to read).

## `session_full`

Returns turns, plan, and git diff for a session in one call. Prefer this over calling `session_latest`, `session_plan`, and `session_diff` separately.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | no | Session ID (omit for most recent session) |
| `title` | string | no | Session title (see [title matching](#common-semantics)) |
| `n` | number | no | Number of turns to return (default 20) |
| `agent` | string | no | Agent: `claude` or `codex`. Required when id and title are omitted |
| `request_id` | string | no | Pagination request ID from a previous response |

## `session_latest`

Returns the last N human/assistant turn pairs from the most recently active session. Tool calls and tool results are filtered out.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `n` | number | no | Number of turns to return (default 20) |
| `agent` | string | yes | Agent: `claude` or `codex` |

## `session_list`

Lists all sessions. Returns session ID, agent, title, title source (`custom` \| `index` \| `derived`), last activity timestamp, whether a plan or diff is available, the inferred diff base (`diff_target`), and session metadata (cwd, git branch, model, origin).

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `agent` | string | no | Agent: `claude` or `codex`. Lists all sessions when omitted |

## `session_get`

Returns the last N turns from a specific session by ID or title.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | no | Session ID |
| `title` | string | no | Session title (see [title matching](#common-semantics)) |
| `agent` | string | no | Agent: `claude` or `codex`. Scopes title matching when provided |
| `n` | number | no | Number of turns to return (default 20) |

## `session_plan`

Returns the current plan for a session. For Claude sessions this is the plan-mode plan file; for Codex the latest `proposed_plan` block. Returns an empty response if the session has no plan.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | no | Session ID (omit for most recent session) |
| `title` | string | no | Session title (see [title matching](#common-semantics)) |
| `agent` | string | no | Agent: `claude` or `codex`. Required when id and title are omitted |

## `session_diff`

Returns the pre-computed git diff for a session, run with merge-base semantics against the automatically inferred base branch and refreshed on each new turn. The resolved base is exposed as `diff_target`.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | no | Session ID (omit for most recent session) |
| `title` | string | no | Session title (see [title matching](#common-semantics)) |
| `agent` | string | no | Agent: `claude` or `codex`. Required when id and title are omitted |

## `session_uncommitted_diff`

Returns the live uncommitted git diff (`git diff HEAD`) for a session, refreshed continuously as files are saved. Resolved in the session's own working tree, so it is correct inside linked git worktrees.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | no | Session ID (omit for most recent session) |
| `title` | string | no | Session title (see [title matching](#common-semantics)) |
| `agent` | string | no | Agent: `claude` or `codex`. Required when id and title are omitted |

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
| Token usage | summed per message | cumulative snapshots parsed; accurate totals pending |
| Tool calls | filtered out | filtered out |
| Sub-agent sessions | hidden (sidechains) | hidden (sub-agent rollouts) |
| Pagination | by client capability | by client capability |
