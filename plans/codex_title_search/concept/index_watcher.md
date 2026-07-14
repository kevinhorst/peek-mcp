# Codex Index Watcher & Title Matching

---

## Flows

### Index ingestion

1. Codex writes or updates `~/.codex/session_index.jsonl`.
2. Backend
   1. `cmd/start.go`: when `codexHome != ""`, start `watcher.NewCodexIndexWatcher(filepath.Join(codexHome, "session_index.jsonl"), store)` alongside the session watcher.
   2. On startup and on each fsnotify Write/Create for that path (debounced 250 ms): read the entire file, JSON-decode line-wise into `codex.IndexEntry`; skip malformed lines with `slog.Debug`.
   3. For each entry, last occurrence per id wins.
   4. Emit `store.AddTurnBySessionId(entry.Id, session.AgentCodex, &session.Turn{CustomTitle: entry.ThreadName, Meta: &session.Meta{SessionId: entry.Id}})` — reuses the existing title-signal path (store updates `Session.Title` and `IdByTitle`, deleting the old hash first).
   5. Store records `TitleSource = "index"` and registers the plain normalized title in `plainTitleById`.

### Derived-title fallback

1. A user turn arrives for a session with `Title == ""`.
2. Backend
   1. In the store's turn path: set `Title = firstLine(turn.Text)` truncated to 80 chars, `TitleSource = "derived"`, register in title maps.
   2. When an index or custom title later arrives, `HasNewTitle` replaces it and re-registers (old hash deleted — existing behavior at store.go:60-63). Derived titles never overwrite index/custom titles.

### Lookup with substring fallback

1. Tool call passes `title` (any agent).
2. Backend (`Store.GetByTitle`, extended; `resolveSession` in tools/tools.go unchanged)
   1. Hash-exact lookup in `IdByTitle` → hit → return session.
   2. Miss → scan `plainTitleById` for case-insensitive substring; collect matches sorted by `LastActive` desc.
   3. 1 match → return; >1 → error listing up to 5 candidates as `(title, id, last_active)`; 0 → current "no session matching title" error.

---

## Security Considerations

- `session_index.jsonl` is user-local data inside the already-trusted `codex-home`; no new trust boundary.
- `thread_name` is model-generated text: treat as untrusted display content, never interpolate into shell/git commands (no current path does).
- Substring matching exposes no new data — `session_list` already returns all titles.
- Full-file re-read is bounded (see Limits) so a pathologically large index cannot stall the watcher goroutine indefinitely.

---

## Limits

- Derived title: 80 chars, first line only (keeps `session_list` rows compact and stable).
- Index re-read debounce: 250 ms (Codex may write several rows in a burst).
- Index file size: full re-read acceptable to ~10k rows / ~1 MB — far above the observed 39 rows; log a warning beyond that.
- Ambiguity candidate list: 5 entries (enough to disambiguate, small enough for a tool error string).

---

## Models

### codex.IndexEntry

**Internal / Not Exported:**
- id: session id, matches rollout `session_meta` id (session.Id)
- thread_name: Codex-maintained session title (string)
- updated_at: last update timestamp (time.Time)

### session.Session (extended)

**Public:**
- title_source: `custom` | `index` | `derived` (string, omitempty)

### session.Store (extended)

**Internal / Not Exported:**
- plainTitleById: normalized plain titles for substring search (map[Id]string), maintained alongside `IdByTitle`

### tools.sessionListItem (extended)

**Public:**
- title_source: provenance of the title (string)

---

## APIs

No new tools. Parameter description update on the five `title`-accepting tools:

> title: Session title. Exact match first (case-insensitive); falls back to substring match. For Codex, titles come from Codex's session index (thread name).

---

## Long-Tail Tasks

### Watcher

- Verify index rewrite-vs-append semantics with a live session rename (concept open question 1).
- Consider watching `~/.codex/archived_sessions/` (open question 4).

### Title sources

- Parse Claude `ai-title` entries as `derived`-tier source (claude/entry.go currently only knows `custom-title`).
- Startup log line counting title sources (custom/index/derived) for debuggability.

### Testing

- Index watcher: full re-read on rewrite, last-line-wins on duplicate ids, debounce, malformed-line skip.
- Store: derived-vs-index precedence, substring match ordering, ambiguity error format, title-only session creation.
