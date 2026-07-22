# Tool Surface (~2–3d)

---

## Flows

### Analysis pull

1. An analysis agent calls `session_events` for a session (by id, title, or latest-per-agent — same resolution as existing tools).
2. Backend
   1. Serializes the event ring, derived counters, usage totals, plan revision summary (revision count + timestamps, diffs on request), and diff availability (`live` / `snapshot` / `none`).
3. One response carries the complete signal picture; turns are explicitly not included (that's `session_full`).

### Inline events in the turn stream

1. `session_full` / `session_latest` / `session_get` are called as today.
2. Backend
   1. Events interleave with turns in timestamp order as compact single-line entries (kind + one-line payload summary), **on by default** (decision).
   2. Pagination priority becomes turns → events → plan → diff; events are small and must never displace turns.
3. `/peek` shows rejections, denials, and skill calls where they happened in the conversation.

### Memory recall

1. `session_get` or `session_full` is called with `remember: true`.
2. Backend
   1. Resolves the memory dir from the transcript's project directory (`<project>/memory/`), reads `MEMORY.md` plus fact files.
   2. Codex sessions: `remember` returns `unsupported` (no memory concept).
3. Response gains a `memory` block: index content + fact files (name, frontmatter type, body).

### Usage exposure

1. Any consumer reads `session_get` / `session_list` / `session_events`.
2. Backend
   1. Correctness first: Codex cumulative `token_count` applies keep-last semantics (per parity/usage design); Claude requestId keep-last merge is already correct.
   2. `session_get` and `session_events` include the totals (input, output, cache create/read, reasoning for Codex); `session_list` keeps its existing meta.
3. Token counts only — no cost fields anywhere (decision: cost/ccusage stays outside peek).

---

## Security Considerations

- `remember` reads only from the project directory peek already watches — no path input from the caller.
- Memory content is user-authored local data; served verbatim as display content.

---

## Limits

- Inline event entry: one line, payload summary capped 200 chars (full payload in `session_events`).
- `session_events` response: same 100 KB Claude cap / pagination machinery as `session_full`; plan revision diffs only included when `revisions: true` (they dominate size otherwise).
- Memory block: 64 KB cap (index first, then facts until cap; truncation marker).

---

## APIs

### session_events (new)

Typed event stream + counters + usage for one session.

**Request fields:**
- id / title / agent: session resolution, same semantics as existing tools (string)
- revisions: include plan revision diffs, default false (bool)

**Response fields:**
- events: ordered event list ([signals.md](signals.md) model) (array)
- counters: plan_rejections, plan_alterations, permission_denials, skills_invoked, subagents_spawned (object)
- usage: token totals (object)
- plan_revisions: count + timestamps, diffs when requested (object)
- diff: source availability — live | snapshot | none (string)
- unsupported: signals not available for this agent, e.g. Codex skills/memory/user-answers (array)

### session_full / session_latest / session_get (extended)

**Notes:**
- Events inline by default, interleaved by timestamp; backlog arg to suppress.
- `remember` (bool, default false) adds the memory block; `session_get` and `session_full` only.
- `session_get` response gains usage totals.

### session_diff (extended)

**Notes:**
- Response gains `source` / `captured_at` ([diff_retention.md](diff_retention.md)).

---

## Long-Tail Tasks

### Testing

- `session_events` fixture session covering every event kind + counters + unsupported list for Codex.
- Pagination: event-heavy session never displaces turns; `revisions: true` pages correctly.
- `remember` on a worktree session resolves the correct project memory dir; Codex returns unsupported.

### Docs

- README tool table + parity matrix rows for events, remember, diff source flags once MVP lands.
