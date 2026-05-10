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

## Output format

**Turns** — label each turn `**Human**` / `**Assistant**`, separated by a blank line. Omit tool calls.

**Plan** — if non-empty, show under `## Plan` as-is (it is already markdown).

**Diff** — if non-empty, show under `## Diff` in a `diff` code block.

Omit any section that is empty.