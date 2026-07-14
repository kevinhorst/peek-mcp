# User Stories: Codex / Claude Feature Parity

---

## Consistent tooling across agents

**As a** Peek user, **I want** every MCP tool to behave the same for Claude Code and Codex sessions, **so that** I don't have to remember per-agent quirks when peeking at sessions.

**As a** Peek user, **I want** a documented parity matrix, **so that** I can tell whether a missing capability is a bug, a gap, or intentional.

---

## Codex plans

**As a** developer watching a Codex session, **I want** `session_plan` to return the plan-mode plan Codex produced (its latest `<proposed_plan>` block), **so that** I can read the plan without opening the rollout file.

**As a** developer refining a plan across several turns, **I want** the latest plan revision to win, **so that** `session_plan` always shows the current plan, not a superseded draft.

---

## Correct session metadata

**As a** developer listing Codex sessions, **I want** the git branch column to show the actual branch name, **so that** I can map sessions to the work they belong to.

**As a** developer analyzing forked Codex sessions, **I want** fork lineage and CLI metadata preserved, **so that** I can trace where a session originated and which client produced it.

**As a** developer listing sessions, **I want** Codex sub-agent sessions filtered out of `session_list`, **so that** spawned helper threads don't clutter the session overview — matching how Claude sidechains are hidden.

---

## Accurate usage

**As a** Peek user, **I want** Codex token totals to match what Codex itself reports, **so that** usage comparisons between agents are meaningful. *(Mechanism owned by the usage_reporting concept.)*
