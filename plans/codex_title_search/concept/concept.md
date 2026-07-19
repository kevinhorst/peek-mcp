# Concept: Peek Codex Search by Title

> **Status:** Approved
> **Author:** Kevin Horst
> **Date:** 2026-07-06

---

## Goals

- Codex sessions become addressable by human-readable title in every `title`-accepting MCP tool (`session_full`, `session_get`, `session_plan`, `session_diff`, `session_uncommitted_diff`) with zero new user workflow.
- Titles come from Codex's own data: `~/.codex/session_index.jsonl` maintains `{"id", "thread_name", "updated_at"}` per session, with ids matching rollout `session_meta` ids.
- Title matching becomes forgiving for **both** agents: exact normalized match stays primary; case-insensitive substring match is added as fallback, so "supabase schema" finds "Propagate Supabase schema".
- `session_list` shows titles for Codex sessions, making ids discoverable.

---

## User Flows

### Look up a Codex session by title

**Goals:**
- "Peek at my Codex session 'Fix no-signals report case'" resolves without knowing the session id.

**Options:**

**MVP**
- New `IndexWatcher` reads `session_index.jsonl` on startup and on change, feeding titles through the existing title-signal pipeline (`Turn{CustomTitle}` → `Store.AddTurnBySessionId`), so all tools work with no tool-side changes.
- Substring fallback when the exact-hash lookup misses; ambiguous matches return a candidate list instead of guessing.
- Derived-title fallback for sessions not (yet) in the index: first line of the first user message, truncated to 80 chars, flagged as derived.

**Backlog**
- User-assigned titles via a write mechanism (`PUT /api/sessions/{id}/title` on the [control_server](../control_server/concept.md); MCP tools stay read-only).
- Fuzzy matching (edit distance) beyond substring.
- Parse Claude `ai-title` entries as a derived-tier title source when no `custom-title` exists.
- Watch `~/.codex/archived_sessions/` rollouts.

**Challenges:**
- Index write semantics (verified in Codex source): name updates are appended (duplicate ids on rename, newest wins); thread deletion rewrites the whole file atomically via temp file + rename — the file inode gets replaced, so byte offsets break.
- Duplicate titles are real (observed: "Propagate Supabase schema" four times with distinct ids).
- Index rows may reference sessions whose rollouts were never parsed (archived/pruned).
- Ordering: an index entry may appear before or after the rollout's first turns.

**Approach:**
- Re-read the whole index file on every fsnotify Write/Create (39 lines observed; trivially cheap), last line per id wins — correct under both append and rewrite semantics. Do **not** reuse the offset-tailing session `Watcher`; the atomic rewrite replaces the inode. Watch the **parent directory** with a filename filter (the `PlanWatcher` pattern) so the watch survives the temp-file + rename replacement.
- Exact match on duplicates: most-recently-active session wins. Ambiguity is surfaced only in substring mode (candidate list).
- When the caller passes `agent`, both exact and substring matches are filtered to that agent; without it, search spans all agents.
- `GetByTitle` already tolerates `IdByTitle` pointing at a missing session (returns not-found); keep that.
- Both arrival orders converge because `Store.AddTurnBySessionId` handles title-signal turns idempotently via `HasNewTitle`, and `getOrCreate` accepts title-only sessions.

---

## Decisions / Open Questions

**Decisions:**
- Primary title source is `session_index.jsonl` `thread_name`, not text derived from prompts. Derived first-prompt titles are a fallback only and never overwrite index/custom titles (precedence: explicit/index > derived), tracked via a new `Session.TitleSource` field (`custom` | `index` | `derived`).
- Full-file re-read with 250 ms debounce; malformed lines skipped with a debug log.
- Matching order: exact normalized (lowercase/trim) hash — unchanged; then case-insensitive substring over plain titles. 0 matches → error (current message); 1 → session; >1 → error listing up to 5 candidates (title, id, last_active), sorted by last_active desc.
- Sessions created purely from an index row (title known, no turns yet) are allowed: `session_list` shows them; turn-returning tools report "No turns found".
- No new MCP tools; only `title` parameter descriptions change.
- Index write semantics: name updates append a new row per id (newest wins); thread deletion rewrites the file atomically via temp file + rename. Watcher watches the parent directory (Write/Create, filename filter) so the watch survives the inode replacement. (Evidence: `codex-rs/rollout/src/session_index.rs` — `append_thread_name` "Name updates are append-only; the most recent entry wins", `remove_thread_name_entries` temp+rename.)
- `thread_name` is never present at session start: Codex writes it after the first turn (auto-named) or on manual rename; some sessions never get one. The derived-title fallback covers the first-turn gap and permanently-unnamed sessions. (Evidence: local index — one entry per id, written 3 s–3 min after rollout start; 6 of 48 rollouts have no entry.)
- [USER] Agent scoping: when the caller passes `agent`, exact and substring matches are filtered to that agent; without it, search spans all agents. (Today's exact lookup ignores `agent` — this is a refinement; honoring an explicit arg is least surprising.)
- Archived rollouts stay listed in the index (evidence: archived id `019f2410-…` present in the live index); their titles resolve to title-only sessions ("No turns found"). Watching `~/.codex/archived_sessions/` stays Backlog.
- [USER] Ambiguity candidate cap stays 5 — covers the observed worst case (4 identical titles) and keeps the error string tool-friendly.
- [USER] Title-only sessions seed `LastActive` from the index row's `updated_at`; turn timestamps take over once turns arrive. (Otherwise zero `LastActive` breaks most-recently-active duplicate resolution and candidate sorting.)
- [USER] Index-row removal (thread deletion) is not reconciled: the watcher only adds/updates titles, so a deleted thread's title stays in memory until restart. Consistent with the session watcher, which never removes sessions; a title pointing at a missing session already returns not-found.

**Open Questions:**

*(none — all resolved 2026-07-15, see Decisions)*
