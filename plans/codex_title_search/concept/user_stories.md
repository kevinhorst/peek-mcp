# User Stories: Peek Codex Search by Title

---

## Title-based lookup

**As a** developer running multiple Codex sessions, **I want** to reference a session by its thread name, **so that** I don't have to copy UUIDs out of `session_list`.

**As a** developer with a vague memory of a session name, **I want** partial-title matching, **so that** "no-signals report" finds "Fix no-signals report case".

**As a** developer whose partial title matches several sessions, **I want** a short candidate list (title, id, last activity), **so that** I can pick the right one instead of getting a wrong guess.

---

## Discoverability

**As a** Peek user, **I want** `session_list(agent: "codex")` to show a title for every session, **so that** the list is readable without opening sessions one by one.

**As a** Peek user, **I want** to see whether a title is explicit, from the Codex index, or derived from the first prompt, **so that** I know how much to trust it.

---

## Fallbacks

**As a** developer whose Codex session is not yet in the index, **I want** a derived title from my first message, **so that** the session is still addressable.

**As a** Claude Code user without a custom title, **I want** Peek to use Claude's auto-generated title, **so that** both agents behave alike. *(Backlog)*

---

## Naming sessions

**As a** Peek user, **I want** to assign my own title to any session, **so that** I can organize sessions by my own naming scheme. *(Backlog — via control server, MCP tools stay read-only)*
