# User Stories: Codex / Claude Feature Parity

---

## Consistent tooling across agents

**As a** Peek user, **I want** every MCP tool to behave the same for Claude Code and Codex sessions, **so that** I don't have to remember per-agent quirks when peeking at sessions.

**As a** Peek user, **I want** a documented parity matrix, **so that** I can tell whether a missing capability is a bug, a gap, or intentional.

---

## Codex plans

**As a** developer watching a Codex session, **I want** `session_plan` to return Codex's current `update_plan` checklist, **so that** I can see what the agent intends to do without opening the rollout file.

**As a** developer, **I want** the Codex plan rendered as a markdown checklist with step status, **so that** completed and pending steps are distinguishable at a glance.

---

## Correct session metadata

**As a** developer listing Codex sessions, **I want** the git branch column to show the actual branch name, **so that** I can map sessions to the work they belong to.

**As a** developer analyzing forked Codex sessions, **I want** fork lineage preserved, **so that** I can trace where a session originated. *(Backlog)*

---

## Accurate usage

**As a** Peek user, **I want** Codex token totals to match what Codex itself reports, **so that** usage comparisons between agents are meaningful. *(Mechanism owned by the usage_reporting concept.)*
