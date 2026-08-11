[← peek-mcp](../../README.md) · Use cases

# Compaction preventer

When a session's context window fills up, start a fresh session and rehydrate it from the old one — full-fidelity turns and plan instead of a lossy compaction summary.

## When

- A long session is about to compact, and you would rather not lose detail to a summary.
- You want the *actual* recent turns and the *actual* plan carried into the next session, verbatim.
- The work is not done, so continuity matters more than a clean slate.

## Setup

Nothing beyond the [Quick start](../../README.md#quick-start). Give the old session a recognizable title (Claude Code custom titles, or Codex's thread name) so you can address it by name. A larger `--depth` keeps more turns available to rehydrate from.

## Walkthrough

1. The old session approaches its context limit. Instead of compacting, note its title — e.g. `Session tools consolidation` — and open a new session in the same repo.

2. First prompt in the fresh session:

   > Use `session_get` with title "Session tools consolidation" to load the last 30 turns and the plan, then continue the task.

3. `session_get` resolves the title (exact match first, then substring) and returns the last N turns from that session as an array, each carrying the session's `meta` (cwd, branch, model):

   ```json
   [
     { "role": "assistant", "text": "Now pages.go — rename plus the fourth stream: ...", "timestamp": "2026-08-11T14:00:24Z",
       "meta": { "session_id": "1cfa5868-d178-4c48-bb6d-a851daba01ce", "git_branch": "claude/consolidate-session-tools-c165e5", "model": "claude-fable-5" } },
     { "role": "assistant", "text": "Now tools.go — the consolidated Register and handler: ...", "timestamp": "2026-08-11T14:00:59Z",
       "meta": { "session_id": "1cfa5868-d178-4c48-bb6d-a851daba01ce", "git_branch": "claude/consolidate-session-tools-c165e5", "model": "claude-fable-5" } }
   ]
   ```

4. Pair it with `session_plan` (or use `session_full`) to pull the plan across too. The new session continues with real context — not a summary of it.

## What to expect

- **`--depth` bounds the reach.** The ring buffer keeps the last N turns per session (default 20); rehydration can only reach as far back as the buffer held. Raise `--depth` for longer histories.
- **Plan and diff arrive uncompacted** — the plan file and the git diff are read from disk as-is, not summarized.
- **Same-repo is not required** — the new session can live in any working directory; it addresses the old one by title or id.
