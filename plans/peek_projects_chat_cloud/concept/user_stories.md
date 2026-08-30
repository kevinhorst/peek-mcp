# User Stories: Peek Projects, Chat and Cloud

---

## Project visibility

**As a** Peek user running Cowork sessions in the Desktop app, **I want** those sessions to appear in `session_list`, **so that** desktop project work is peekable like any repo session.

**As a** Peek user, **I want** every session to expose a `project` field, **so that** I can tell desktop project sessions apart from repo sessions at a glance.

**As a** Peek user, **I want** `session_list(project: "cowork")` to filter to desktop Cowork sessions, **so that** I can follow desktop work without scanning repo sessions. *(Per-project names are backlog — no local name source exists yet.)*

---

## Consistent metadata

**As a** Peek user, **I want** repo sessions to stay unchanged (no fake project values), **so that** existing consumers of `session_list` keep working.

**As a** Peek user, **I want** Cowork transcript quirks (queue-operation entries, non-git working dirs, audit logs) handled silently, **so that** ingesting the desktop store adds no noise or phantom sessions.

---

## Coverage transparency

**As a** Peek user, **I want** the chat/cloud verdict documented in the concept, **so that** their absence from Peek reads as an intentional decision with evidence, not a bug.

**As a** Peek user who teleports a cloud session locally, **I want** the resulting session to show up in Peek automatically, **so that** the teleport bridge is the supported way to peek at cloud work.
