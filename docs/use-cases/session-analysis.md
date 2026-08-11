[← peek-mcp](../../README.md) · Use cases

# Session analysis

Mine past sessions for retrospectives — what was asked, what was built, where time and rework went — without opening a single JSONL file.

## When

- You run retrospectives over recent work and want the raw material without hand-parsing transcripts.
- You are looking for patterns: rejected approaches, repeated corrections, plans that drifted from their result.
- The session files are on disk in a format that is tedious to read directly.

## Setup

Nothing beyond the [Quick start](../../README.md#quick-start). A larger `--depth` keeps more turns per session available for analysis. Connect peek-mcp to the analyzing model.

## Walkthrough

1. Pick candidate sessions with `session_list` — scan titles, activity, and which ones carry a plan or diff:

   ```json
   {
     "sessions": [
       { "id": "9d46e048-...", "title": "[CC, Fdesign, F.l] ACDSL improvements",
         "last_active": "2026-08-10T18:30:31Z", "has_plan": true, "has_diff": true },
       { "id": "a05b4ef4-...", "title": "[CC, Fchange, Opus48] skill defects",
         "last_active": "2026-08-10T13:15:56Z", "has_plan": true, "has_diff": true }
     ]
   }
   ```

2. For each candidate, pull the full picture with `session_full` — turns, plan, and the final diff:

   > Use `session_full` for id 9d46e048 and summarize what was asked, what the plan committed to, and how the final diff compares.

3. The analyzing model extracts the patterns: where the plan and the diff diverged, which approaches were tried and rejected in the turns, which corrections repeated. Feed several sessions through the same loop for a batch retrospective.

## What to expect

- **Tool calls are filtered out** — turns are the human/assistant exchange, not the tool-call noise, so analysis stays on intent and outcome.
- **Sub-agent sessions are hidden** — you analyze real sessions, not the sidechains they spawn.
- **`--depth` bounds history** — analysis reaches only as far back as the ring buffer held; raise it for deeper retrospectives.
