# Desktop Projects (Cowork) Ingestion

---

## Flows

### Ingest a new Cowork session

1. User starts a session in a Desktop app project (Cowork).
2. Desktop creates `local_<id>/` under `…/<store>/<account-uuid>/<org-uuid>/` (store: `local-agent-mode-sessions`, being renamed to `claude-code-sessions`) with its own `.claude/projects/<slug>/<sessionId>.jsonl`.
3. Backend
   1. The Cowork watcher (recursive, rooted at the store) picks up the new directory chain and the transcript file.
   2. The transcript-path predicate accepts it (`**/.claude/projects/**/*.jsonl`), rejecting `audit.jsonl` and artifact files.
   3. The Claude parser processes entries as usual; the watcher stamps `Meta.Project = "cowork"` (the store carries no project identity — path segments are account/org UUIDs).
4. The session appears in `session_list` with `agent: "claude"` and `project: "cowork"`.

### Filter sessions by project

1. Client calls `session_list` with `project`.
2. Backend
   1. Handler filters the store's list by exact `Meta.Project` match, composing with the existing `agent` filter.
3. Response contains only that project's sessions.

---

## Security Considerations

- The Cowork store is `0700` and contains uploads/outputs; Peek reads only files matching the transcript predicate — no artifact or audit content is ingested.
- `audit.jsonl` is HMAC-protected Desktop-internal data; excluded at the watcher, never parsed.

---

## Limits

- Transcript predicate: path must contain `/.claude/projects/` and end in `.jsonl` (rationale: `.jsonl` alone matches audit logs and artifacts inside this store).
- Project filter: exact string match, no globbing (rationale: small closed label set, no partial-match use case).

---

## Models

### session.Meta (extension)

**Public:**
- Project: coarse label — `"cowork"` for Cowork-store transcripts, `<Name>` for legacy `~/Claude_<Name>` cwds, empty otherwise (string; omitted from JSON when empty). No UUIDs; upgrades in place when a real project-name source appears.

---

## APIs

### session_list (MCP tool, extended)

**Request fields:**
- project: optional exact-match filter on `meta.project` (string)

**Response fields:**
- sessions[].meta.project: as defined above (string, omitted when empty)

### GET /api/sessions (control server, extended)

**Request query params:**
- project: same semantics as the MCP param, beside the existing `agent` param

---

## Worker Tasks

- Cowork watcher goroutine in `cmd/start.go`
  - Same construction as the existing Claude watcher: `watcher.New(AgentClaude, coworkRoot, claude.NewParser, store)` plus the transcript-path predicate
  - Root: `~/Library/Application Support/Claude` store dirs by default (macOS), overridable via `--cowork-home`; both `local-agent-mode-sessions` and `claude-code-sessions` are probed and watched when present (store mid-rename, both can coexist)

---

## Infrastructure

- None — local watcher only; no config migration (new root is additive and skipped when the directory does not exist).

---

## Long-Tail Tasks

### Project names

- Attach Cowork sessions to their claude.ai Project once a local source for `claude_project_id`/names exists (Desktop export, API, or future on-disk mapping); today the association is server-side only and the store path carries just account/org UUIDs.

### Session titles

- Join Cowork sessions with their sibling `local_<id>.json` metadata (`title`, `lastActivityAt`) — Cowork transcripts carry no `custom-title` entries.

### Teleport labeling

- After the user-assisted teleport test (concept Open Question 1): if teleported transcripts persist a stable cloud-origin marker (candidate: the CLI's `isTeleported` flag), surface it as session metadata.
