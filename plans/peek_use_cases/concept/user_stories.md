# User Stories: Peek Use-Case Documentation

---

## Model-switch handoff

**As a** developer running an expensive frontier model, **I want** a cheaper model to pick up my session's turns, plan, and diff in one tool call, **so that** follow-up work doesn't burn frontier-model tokens or my time re-explaining context.

**As a** reader evaluating peek, **I want** to see the receiving model act correctly on the handed-over context, **so that** I believe the handoff transfers working state, not just text.

---

## Compaction preventer

**As a** developer near the context limit, **I want** to reset the session and have the successor pull verbatim recent turns plus the actual plan file and git diff, **so that** I never depend on a lossy auto-compaction summary.

**As a** reader evaluating peek, **I want** an honest account of what the ring buffer does and does not preserve, **so that** I can judge the workflow's limits before adopting it.

---

## Orchestration helper

**As a** developer running several agent sessions in parallel, **I want** one supervisor session to list all sessions with branch, model, and last activity, **so that** I can see fleet state without switching terminals.

**As a** supervisor-session user, **I want** to drill into any worker's turns and diff by ID or title, **so that** I can review a worker's output without interrupting it.

**As a** reader evaluating peek, **I want** the observation-vs-dispatch boundary stated up front, **so that** I don't expect a task scheduler and get disappointed.

---

## Cross-agent communication

**As a** Claude Code user, **I want** to read the plan a Codex session proposed, **so that** two agents can work against the same plan without copy-paste.

**As a** Codex user, **I want** to read a Claude session's recent turns and diff, **so that** I can review or continue Claude's work from the other CLI.

**As a** security-conscious reader, **I want** the docs to say plainly that connected clients can read everything in my transcripts, **so that** I can decide what to connect.

---

## Session analysis

**As a** developer running retrospectives, **I want** to fetch a finished session by its human-readable title and get turns, plan, and diff from one tool surface, **so that** analysis needs no raw JSONL handling.

**As a** developer mining many sessions, **I want** `session_list` metadata (agent, title, branch, model, activity) as the entry point for batch analysis, **so that** I can select sessions programmatically instead of globbing transcript directories.

**As a** reader evaluating peek, **I want** a before/after against raw JSONL, **so that** the "optimal" claim is demonstrated, not asserted.
