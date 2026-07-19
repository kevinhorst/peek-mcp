# User Stories: Deep Analysis

---

## Session analysis

**As a** session-analyze agent, **I want** one tool call returning all typed events and counters of a session, **so that** retrospectives don't reconstruct signals from prose turns.

**As a** session-analyze agent, **I want** plan rejection and alteration counts with the underlying revision diffs, **so that** I can measure how much plans changed before approval and why.

**As a** session-analyze agent, **I want** permission prompts and the user's answers exposed explicitly, **so that** denied actions and user corrections become mineable signals instead of disappearing into tool noise.

**As a** session-analyze agent, **I want** skill invocations listed — including those made inside subagents, **so that** skill usage and skill gaps are measurable per session.

---

## Subagents

**As a** session-analyze agent, **I want** the result each subagent returned to the main agent, plus its type and description, **so that** delegated work is visible without opening subagent transcripts.

**As a** Peek user, **I want** subagent sessions to stay out of `session_list`, **so that** helper threads don't clutter the overview while their content is still reachable through the parent.

---

## Live context

**As a** developer peeking at a running session, **I want** plan rejections, permission denials, and skill invocations inline in the turn stream by default, **so that** I see what actually happened, not only what was said.

**As a** developer resuming work, **I want** `session_get` with `remember: true` to return the project's memory files, **so that** recall doesn't require leaving the peek surface.

---

## Diff durability

**As a** session-analyze agent, **I want** `session_diff` to return the session's diff even after the branch was merged or cherry-picked away and the worktree removed, **so that** post-hoc analysis can always correlate conversation with code change.

**As a** Peek user, **I want** a served snapshot clearly flagged as a snapshot with its capture time, **so that** I never mistake stale state for a live diff.

---

## Usage

**As a** Peek user, **I want** Codex token totals to match what Codex itself reports, **so that** per-session usage numbers are trustworthy.

**As a** session-analyze agent, **I want** token totals included in the analysis output, **so that** session cost correlates with session outcomes. *(Token counts only — cost estimation stays outside peek.)*
