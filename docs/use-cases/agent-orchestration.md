[← peek-mcp](../../README.md) · Use cases

# Agent orchestration

Supervise several worker sessions from one orchestrator session — see who is progressing and what code each produced, without pasting anything between terminals.

## When

- You run multiple agents in parallel (worktrees, terminal tabs, background tasks) and need a single vantage point.
- You want the orchestrator to decide who continues and who stops, based on real progress — not guesses.
- Copy-pasting each worker's state into the orchestrator by hand does not scale past two agents.

## Setup

Nothing beyond the [Quick start](../../README.md#quick-start). Every worker writes its session to disk automatically; peek-mcp watches them all. Connect peek-mcp to the orchestrator session.

## Walkthrough

1. From the orchestrator, enumerate the workers with `session_list`. Each row carries branch, activity time, and whether a plan or diff exists:

   ```json
   {
     "sessions": [
       { "id": "397567c6-...", "title": "Peek-windows installer improvements", "last_active": "2026-08-11T13:54:44Z",
         "has_plan": true, "has_diff": false, "diff_target": "improvement/windows-support",
         "meta": { "git_branch": "claude/peek-windows-installer-977c97", "model": "claude-fable-5" } },
       { "id": "6d041d57-...", "title": "Control Server usage stats", "last_active": "2026-08-11T13:53:10Z",
         "has_plan": true, "has_diff": false,
         "meta": { "git_branch": "claude/control-server-usage-stats-5d5ec9", "model": "claude-fable-5" } }
     ]
   }
   ```

2. For any worker, pull progress with `session_get` — recent turns plus the produced code as the `diff` section (the change against its base branch), all in one call.

3. The orchestrator decides per worker — let it run, redirect it, or stop it — from live state, then repeats.

The same fleet view a human would scan:

![peek-mcp dashboard — session list across worker sessions](../assets/dashboard-sessions.png)

## What to expect

- **`session_list` is the roster.** Filter by `agent` (`claude` or `codex`) to scope it; omit to list everything.
- **`last_active` and `has_diff`** are the cheap progress signals — poll `session_list`, drill in with `session_get` only where something changed.
- **Sub-agent sessions are hidden** — the list shows real worker sessions, not the sidechains they spawn.
