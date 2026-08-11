---
name: peek
description: >
  Show the latest Claude Code session. Call when user types /peek (with optional
  count), asks what Claude Code is doing, or wants recent session turns, plan, or diff.
---

## Routing

| Input | Tool | Notes |
|-------|------|-------|
| `/peek [n]`, "what is Claude doing", "show session" | `session_get` | n defaults to 20 |
| `/peek list` | `session_list` | shows all sessions with plan/diff flags |
| `/peek plan` | `session_get` with `turns: false, diff: false` | current plan only |
| `/peek diff` | `session_get` with `turns: false, plan: false` | git diff only |
| `/peek <id>` or `/peek <id> [n]` | `session_get` with `id` param | specific session by ID |
| `/peek <title>` or `/peek <title> [n]` | `session_get` with `title` param | exact title match (case-insensitive) |

When the input looks like a session ID (UUID-style), pass it as `id`. Otherwise pass it as `title`.
`id` takes precedence over `title` when both are provided.
Title matching is exact (case-insensitive) — substrings will not match.

The `agent` param (`"claude"` or `"codex"`) is only needed when no `id` or `title` is
provided and both agents are enabled. Pass it when the user qualifies the command
(e.g. `/peek codex`). If the user doesn't qualify, default to `"claude"`.

## Pagination

`session_get` responses may be paginated. When the response contains `has_more: true` and a `request_id`, you MUST call `session_get` again with that `request_id` to get the next page. Keep calling until `has_more` is false or `request_id` is absent. All requested sections (turns, plan, diff, uncommitted diff) arrive through the paginated `session_get` responses.

## Output format

Do NOT reproduce the tool result. The data is already in context for the LLM — formatting it again wastes time and tokens.

After calling the tool, respond with only a short confirmation line, e.g.:

> Peeked at session **Login simplification** (20 turns, has plan, has diff).

Include: session title or ID, turn count, and which sections are present (plan/diff). Nothing else.

For `/peek list`, show the session table as-is — that is already compact.
