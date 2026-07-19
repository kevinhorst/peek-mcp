# Concept: Deep Analysis

> **Status:** Draft
> **Author:** Kevin Horst
> **Date:** 2026-07-19

---

## Goals

- Peek becomes the data source for deep session analysis (session-analyze retrospectives, skill/memory/workflow mining): every analysis-relevant signal in a session is reachable through MCP tools, for both agents, without touching raw transcript files.
- A typed event stream plus derived counters (plan rejections, plan alterations, permission denials, skill invocations, subagent spawns) — surfaced inline by default and via a dedicated analysis tool.
- The session diff is never lost: it survives target-branch merges, cherry-pick + worktree cleanup, and daemon restarts.
- Plan alteration history, memory files, and per-session token usage are exposed alongside the existing turn stream.
- Peek stays one-thing-well: it extracts signals it already sees on disk. No cost estimation, no ccusage coupling — usage orchestration across agents belongs to the config server.

Persistence principle: **persist only what disk doesn't already remember.** Events, subagent content, memory, and usage are re-derivable from transcripts at any time and stay in-memory. Only live-computed git diffs and overwritten plan-file versions get a peek state directory.

---

## Signal Matrix

Verified against raw transcripts (`~/.claude/projects/...-control-server-feature-design-f8329e/a50eb4fd-*.jsonl` and sampled Codex rollouts, 2026-07-19).

| Signal | Claude Code source | Codex source | Status / Action |
|---|---|---|---|
| Skill invocation | `Skill` tool_use (`{skill, args}`); slash commands as `<command-name>`/`<command-message>` tags in user text | No skill concept | New → MVP ([signals.md](signals.md)), Claude-only |
| Plan lifecycle | `plan_mode` / `plan_mode_exit` / `plan_mode_reentry` attachments + `ExitPlanMode` tool_use with result | Plan-mode `<proposed_plan>` blocks | Attachments partially parsed today → MVP extends |
| Plan approval / rejection | `ExitPlanMode` tool_result: approval = "User has approved your plan" (+ approved plan, possibly behind a `<persisted-output>` pointer); rejection = `is_error: true` | No approval event — a follow-up user message after a `<proposed_plan>` is the implicit verdict | New → MVP for Claude; Codex approvals not detectable (documented gap) |
| Plan revisions | Plan file overwrites (PlanWatcher sees each version live) | `<proposed_plan>` block sequence (today last-wins) | New → MVP ([signals.md](signals.md)) |
| Permission denial | tool_result `is_error` + "The user doesn't want to proceed with this tool use." on any tool_use | Approval `event_msg` payloads exist; no local sample — verify (Open Question 1) | New → MVP |
| Explicit user answers | `AskUserQuestion` tool_use + result (chosen options, notes) | No equivalent observed | New → MVP, Claude-only |
| Subagent content | `<project>/<sessionId>/subagents/agent-<id>.jsonl` + `agent-<id>.meta.json`; result = tool_result of the spawning `Agent` tool_use in the parent | Separate rollouts, `session_meta.source.subagent.thread_spawn` with `parent_thread_id` (today dropped entirely) | New → MVP ([subagents.md](subagents.md)) |
| Memory files | `~/.claude/projects/<encoded-cwd>/memory/` (MEMORY.md + fact files) | No memory concept | New → MVP ([surface.md](surface.md)), Claude-only |
| Token usage | Per-message usage, requestId keep-last merge (correct) | `token_count` events are cumulative; summing over-counts (known bug) | Fix + expose → MVP ([surface.md](surface.md)) |
| Diff durability | Live `git diff <target-branch>` — collapses after merge, dies with worktree cleanup | Same mechanism | New → MVP ([diff_retention.md](diff_retention.md)) |

---

## User Flows

### Analyze a finished session

**Goals:**
- An analysis agent (session-analyze) pulls the full signal picture of a past session in one or two tool calls — including sessions whose worktree and branch were already cleaned up.

**Options:**

**MVP**
- `session_events` tool: typed event stream (plan, permission, skill, subagent, user-answer events) + derived counters + usage totals per session (~1d, on top of extraction below).
- Event extraction from both agents' transcripts, including inside subagent transcripts ([signals.md](signals.md), [subagents.md](subagents.md)).
- Plan revision history: initial version + diff per alteration, rejection/alteration counters ([signals.md](signals.md)).
- Diff pinning + snapshot so `session_diff` still answers after merge/cleanup ([diff_retention.md](diff_retention.md)).

**Backlog**
- Full subagent turn browsing (internal thinking) (~2d).
- Cross-session aggregation (counters over a batch of sessions) (~2d) — likely config-server territory, revisit.

**Challenges:**
- Analysis happens days later; peek's in-memory state is gone after restart, and git state is gone after cleanup.

**Approach:**
- Events re-derive from transcripts on disk at watch/parse time — restart-safe by construction. Diffs and plan revisions persist in the peek state dir (the only two artifacts disk doesn't remember).

### Peek at a live session with signal context

**Goals:**
- `/peek` output shows what actually happened, not only prose: plan rejections, permission denials, and skill invocations appear in the turn stream by default.

**Options:**

**MVP**
- Compact inline event entries in `session_full` / `session_latest` / `session_get` output, on by default ([surface.md](surface.md)).

**Backlog**
- Arg to suppress events for minimal-context consumers (~0.5d).

**Challenges:**
- Context budget: events must stay compact (single line each), never dominate the 100 KB Claude response cap.

**Approach:**
- Events serialize as short typed entries; pagination priority keeps turns first ([surface.md](surface.md)).

### Recall memory

**Goals:**
- `session_get` / `session_full` with `remember: true` returns the project's auto-memory (MEMORY.md + fact files) alongside the session.

**Options:**

**MVP**
- `remember` arg, Claude-only, resolved from the session's project directory ([surface.md](surface.md)).

**Backlog**
- Filtering by memory type (~0.5d).

**Challenges:**
- Memory belongs to the project, not the session — worktree sessions must resolve to the right encoded project dir.

**Approach:**
- Derive the memory dir from the transcript's own location (the project dir peek already watches), not from cwd re-encoding.

### See what a session cost

**Goals:**
- Correct per-session token totals for both agents, exposed where sessions are listed and analyzed.

**Options:**

**MVP**
- Fix Codex cumulative `token_count` handling (keep-last, not sum); keep Claude requestId keep-last merge; expose totals in `session_get` / `session_events` ([surface.md](surface.md)).

**Backlog**
- Nothing. Cost estimation and cross-agent usage reporting stay out of peek (decision below).

**Challenges:**
- Two semantics behind one totals field (Claude additive deltas, Codex cumulative snapshots).

**Approach:**
- Store-side per-agent handling as already sketched in the parity concept; this concept only adds the exposure surface.

---

## Implementation Blocks

| Block | Content | Estimate |
|---|---|---|
| [signals.md](signals.md) | Event model, per-agent extraction, plan revision history, counters | ~3–5d |
| [subagents.md](subagents.md) | Subagent discovery, result capture, event scan inside subagents | ~2–3d |
| [diff_retention.md](diff_retention.md) | Base pinning, snapshot persistence, state dir | ~2d |
| [surface.md](surface.md) | `session_events` tool, inline events, `remember` arg, usage exposure | ~2–3d |
| **Total** | | **~9–13d** |

---

## Decisions / Open Questions

**Decisions:**
- [USER] No ccusage integration in peek. ccusage (reads `~/.claude` and `~/.codex`, JSON output, `@ccusage/mcp`) is the right wheel for usage/cost reporting, but peek is the wrong place to mount it — peek does one thing well; if anything, the config server orchestrates ccusage. Peek ships only the low-hanging fruit on usage data it already extracts (correctness fixes + exposure); the impact of even that gets a separate evaluation before implementation.
- [USER] Exposure is both inline and dedicated: events appear in existing tool output **by default**, and `session_events` serves the analysis use-case with the full stream + counters.
- [USER] Subagent MVP = result returned to the parent + meta + event scan of the subagent transcript, with skill invocations inside subagents explicitly included. Internal thinking/full turns are backlog.
- [USER] Memory exposure arg is named `remember`.
- [USER] Plan revisions are stored as initial version + diff-to-previous per alteration; the final full content already lives in the plan file on disk.
- [USER] Diff durability = pin merge-base at first sight **and** persist the last non-empty diff as a snapshot. Pin alone dies in the real workflow: session commits get cherry-picked onto a feature branch (new SHAs), then worktree + branch are removed — cwd gone, `git diff` impossible. Verified against the claude-configs worktree-management workflow.
- Signal extraction is allowlist-based: the parsers recognize specific signal tools (`Skill`, `ExitPlanMode`, `Agent`, `AskUserQuestion`) and the generic denial pattern — they do not become a general tool-call mirror. General tool_use/tool_result surfacing stays intentionally out (parity concept decision unchanged).
- Events live in memory only; the peek state dir persists exactly two artifact kinds: diff pins+snapshots and plan revision diffs.
- Codex plan approval is not detectable (no approval event exists around `<proposed_plan>`); Codex rejection/alteration counting is approximated by counting successive `<proposed_plan>` blocks as alterations. Documented as an intentional parity gap in the signal matrix.

**Open Questions:**
1. Codex approval/permission `event_msg` payload shapes — verify against a rollout that actually contains approval requests (none on disk at drafting time), then finalize the Codex PermissionEvent mapping in [signals.md](signals.md).
2. State dir location (`~/.peek/state/...` vs XDG state dir) and snapshot retention/GC policy (per-session size cap exists; when do old sessions' snapshots get pruned?).
