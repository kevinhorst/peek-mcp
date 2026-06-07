---
name: peek
description: >
  Show the latest Claude Code session. Call when user types /peek (with optional
  count), asks what Claude Code is doing, or wants recent session turns, plan, or diff.
---

## Routing

| Input | Tool | Notes |
|-------|------|-------|
| `/peek [n]`, "what is Claude doing", "show session" | `session_full` | n defaults to 5 |
| `/peek list` | `session_list` | shows all sessions with plan/diff flags |
| `/peek plan` | `session_plan` | current plan only |
| `/peek diff` | `session_diff` | git diff only |
| `/peek <id>` or `/peek <id> [n]` | `session_full` with id | specific session |

All tools need a required `agent` param (`"claude"` or `"codex"`). Pass it when the
user qualifies the command, e.g. `/peek codex`. If the user doesn't qualify, default to Claude.

## Pagination

`session_full` responses may be paginated. When the response contains `has_more: true` and a `request_id`, you MUST call `session_full` again with that `request_id` to get the next page. Keep calling until `has_more` is false or `request_id` is absent. Do NOT call `session_diff` or `session_plan` separately — all content (turns, plan, diff) arrives through the paginated `session_full` responses.

## Output format

Do NOT reproduce the tool result. The data is already in context for the LLM — formatting it again wastes time and tokens.

After calling the tool, respond with only a short confirmation line, e.g.:

> Peeked at session **Login simplification** (5 turns, has plan, has diff).

Include: session title or ID, turn count, and which sections are present (plan/diff). Nothing else.

For `/peek list`, show the session table as-is — that is already compact.