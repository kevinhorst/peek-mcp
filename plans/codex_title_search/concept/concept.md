# Concept: Peek Codex Search by Title

> **Status:** Draft
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
- Index write semantics unknown: append-only (duplicate ids on rename) vs rewritten in place.
- Duplicate titles are real (observed: "Propagate Supabase schema" four times with distinct ids).
- Index rows may reference sessions whose rollouts were never parsed (archived/pruned).
- Ordering: an index entry may appear before or after the rollout's first turns.

**Approach:**
- Re-read the whole index file on every fsnotify write (39 lines observed; trivially cheap), last line per id wins — correct under both append and rewrite semantics. Do **not** reuse the offset-tailing session `Watcher`; a rewrite would break offsets.
- Exact match on duplicates: most-recently-active session wins. Ambiguity is surfaced only in substring mode (candidate list).
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

**Open Questions:**
1. Does Codex rewrite `session_index.jsonl` on rename/update or append a new row with the same id? (Design tolerates both; verify with a live rename.)
2. Is `thread_name` present from session start or only after the first assistant turn completes? Determines how long the derived-title fallback is visible.
3. Should substring matching search across both agents when the caller is a third client (e.g. Claude Desktop), or only within the resolved agent? (Proposal: only within the resolved agent.)
4. Are `~/.codex/archived_sessions/` rollouts still listed in the index, and should Peek watch that directory too?
5. Is 5 the right cap for the ambiguity candidate list?
