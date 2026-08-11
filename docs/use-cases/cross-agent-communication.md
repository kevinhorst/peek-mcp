[← peek-mcp](../../README.md) · Use cases

# Cross-agent communication

Let Claude Code and Codex CLI read each other's sessions on the same repo — one reviews or continues what the other did, with no copy-paste bridge between the two tools.

## When

- You use both Claude Code and Codex CLI and want them to hand work back and forth.
- One agent implemented something and you want the other to review or extend it.
- The two tools write to different session directories, so there is no shared context out of the box.

## Setup

Nothing beyond the [Quick start](../../README.md#quick-start). peek-mcp watches both `~/.claude` and `~/.codex`. Connect it to whichever agent is doing the reading; the `agent` parameter selects which side to read.

## Walkthrough

1. Codex implements a change on the repo. In Claude Code, read the Codex session:

   > Use `session_get` with `agent=codex` to see what Codex changed and review the diff.

2. `session_get` scoped to Codex returns one object — `turns` (a serialized array of the recent turns), the git `diff`, the resolved `diff_target`, and `plan` when the session has one:

   ```json
   {
     "turns": "[{\"role\":\"user\",\"text\":\"Thanks. ...\",\"timestamp\":\"2026-06-19T22:05:41Z\",\"meta\":{\"session_id\":\"019ee1d2-...\"}}]",
     "diff": "diff --git a/.github/workflows/integration-ci.yml b/.github/workflows/integration-ci.yml\nnew file mode 100644\n@@ ...",
     "diff_target": "origin/main",
     "has_more": true
   }
   ```

3. The reverse works the same way — from Codex, read a Claude session with `agent=claude`. Either tool sees the other's real work.

## What to expect

- **`agent` picks the side.** `codex` or `claude`; required when you omit both `id` and `title`.
- **Parity differs by source, and it matters here:**
  - Titles for Codex come from its session index (thread name), not custom titles.
  - Plans for Codex are the latest `proposed_plan` block; for Claude they are the plan-mode plan file.
  - Full details in the [parity table](../tools.md#agent-parity).
- **Codex token usage is not yet exposed** — cumulative snapshots are parsed, accurate totals are pending.
