# Codex Index Watcher & Title Matching

---

## Flows

### Index ingestion

1. Codex writes or updates `~/.codex/session_index.jsonl`.
2. Backend
   1. `cmd/start.go`: when `codexHome != ""`, start `watcher.NewCodexIndexWatcher(codexHome, store)` alongside the session watcher.
   2. Watch the `codexHome` **directory** (not the file path) for fsnotify Write/Create events whose name is `session_index.jsonl` — the `PlanWatcher` pattern. Codex's deletion path replaces the file atomically (temp file + rename), so a watch on the file path would die with the old inode.
   3. On startup and on each matching event (debounced 250 ms): read the entire file, JSON-decode line-wise into `codex.IndexEntry`; skip malformed lines with `slog.Debug`.
   4. For each entry, last occurrence per id wins.
   5. Emit `store.AddTurnBySessionId(entry.Id, session.AgentCodex, &session.Turn{CustomTitle: entry.ThreadName, Meta: &session.Meta{SessionId: entry.Id}})` — reuses the existing title-signal path (store updates `Session.Title` and `IdByTitle`, deleting the old hash first).
   6. Store records `TitleSource = "index"` and registers the plain normalized title in `plainTitleById`. For sessions without turns, `LastActive` is seeded from the entry's `updated_at`; turn timestamps take over once turns arrive.
   7. Rows removed by Codex (thread deletion) are not reconciled: the watcher only adds/updates; a stale title pointing at a missing session returns not-found.

### Derived-title fallback

1. A user turn arrives for a session with `Title == ""`.
2. Backend
   1. In the store's turn path: set `Title = firstLine(turn.Text)` truncated to 80 chars, `TitleSource = "derived"`, register in title maps.
   2. When an index or custom title later arrives, `HasNewTitle` replaces it and re-registers (old hash deleted — existing behavior at store.go:60-63). Derived titles never overwrite index/custom titles.

### Lookup with substring fallback

1. Tool call passes `title`, optionally with `agent`.
2. Backend (`Store.GetByTitle`, extended; `resolveSession` in tools/tools.go passes the parsed `agent` through)
   1. Hash-exact lookup in `IdByTitle` → hit whose agent matches the filter (or no filter) → return session.
   2. Miss → scan `plainTitleById` for case-insensitive substring, restricted to the `agent` filter when present; collect matches sorted by `LastActive` desc.
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
- updated_at: last update timestamp (time.Time); seeds `LastActive` for sessions without turns

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

> title: Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to `agent` when provided. For Codex, titles come from Codex's session index (thread name).

---

## Long-Tail Tasks

### Watcher

- Consider watching `~/.codex/archived_sessions/` (backlog; archived rollouts stay listed in the index, so their titles resolve to title-only sessions meanwhile).

### Title sources

- Parse Claude `ai-title` entries as `derived`-tier source (claude/entry.go currently only knows `custom-title`).
- Startup log line counting title sources (custom/index/derived) for debuggability.

### Testing

- Index watcher: full re-read on rewrite, last-line-wins on duplicate ids, debounce, malformed-line skip, watch survives atomic file replacement (temp+rename).
- Store: derived-vs-index precedence, substring match ordering, ambiguity error format, title-only session creation, agent-filtered lookup, `LastActive` seeding from `updated_at`.
