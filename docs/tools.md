[← peek-mcp](../README.md)

# MCP tools

Full parameter reference for every tool peek-mcp exposes. For task-oriented walkthroughs, see the [use cases](../README.md#use-cases).

## Common semantics

- **Title matching** — a `title` is matched exact-first (case-insensitive), then falls back to substring match. When `agent` is provided, matching is scoped to that agent. For Codex, titles come from Codex's session index (the thread name).
- **Pagination** — responses that carry turns or diffs are paginated by the client's capability. When a response has `has_more: true`, call the same tool again with the returned `request_id` to fetch the next page.
- **Most-recent default** — `session_get` uses the most recently active session when `id` and `title` are omitted (an `agent` is then required when more than one agent is enabled, so the lookup knows which side to read).

## `session_get`

Returns session data (turns, plan, git diff, uncommitted diff) for a session in one call. Sections are selected with flat boolean flags. Turns are returned as the last N human/assistant turn pairs; tool calls and tool results are filtered out.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | no | Session ID (omit for most recent session) |
| `title` | string | no | Session title (see [title matching](#common-semantics)) |
| `agent` | string | no | Agent: `claude` or `codex`. Required when `id` and `title` are omitted and more than one agent is enabled |
| `n` | number | no | Number of turns to return (default 20). Only applies to the turns section |
| `turns` | boolean | no | Return the session turns (default `true`) |
| `plan` | boolean | no | Return the session plan (default `true`). For Claude sessions this is the plan-mode plan file; for Codex the latest `proposed_plan` block |
| `diff` | boolean | no | Return the pre-computed merge-base diff against the inferred base branch, reported as `diff_target` (default `true`) |
| `uncommitted_diff` | boolean | no | Return the live `git diff HEAD` in the session's own working tree, refreshed as files are saved (default `false`) |
| `request_id` | string | no | Pagination request ID from a previous response |

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
| Token usage | summed per message | cumulative snapshots parsed; accurate totals pending |
| Tool calls | filtered out | filtered out |
| Sub-agent sessions | hidden (sidechains) | hidden (sub-agent rollouts) |
| Pagination | by client capability | by client capability |
