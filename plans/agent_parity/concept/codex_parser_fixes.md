# Codex Parser Fixes (Parity MVP)

---

## Flows

### Codex plan ingestion

1. In plan mode, Codex writes its final plan as a `<proposed_plan>…</proposed_plan>` markdown block inside an assistant `message` response item (no plan file exists; the mode spec mandates the block, tags on their own lines, and rejects `update_plan` during plan mode).
2. Watcher tails the file and passes the line to `codex/parser.go`.
3. Backend
   1. `handleAssistantMessage` scans the extracted text for a `<proposed_plan>` block (line-anchored tags per spec).
   2. If present: extract the block content and emit `session.Turn{Text: fullMessage, PlanContent: block, PlanFilePath: "codex:proposed_plan", Meta: {SessionId, Model}}` — the message stays a chat turn AND feeds the plan.
   3. `Store.AddTurnBySessionId` routes it through the plan branch; `PlanContent != ""` is stored before any `os.ReadFile`, so the sentinel path is never treated as a file; when `turn.Text != ""` the turn additionally falls through to `AddTurn` (agent-agnostic, backward compatible — Claude plan turns carry no text; mechanism finalized in feature-design).
4. `session_plan(agent: "codex")` returns the plan markdown; `session_list` shows `has_plan: true`.
5. `update_plan` function calls remain unparsed — they are a todo/progress tool, intentionally not a plan.

### Sub-agent session dropping

1. Codex sub-agents write separate rollout files whose `session_meta.payload.source` is an object `{"subagent": {"thread_spawn": {"parent_thread_id", "depth", "agent_path", "agent_nickname", "agent_role"}}}` instead of the string `"vscode"`.
2. Backend
   1. `SessionMeta.Source` decodes tolerantly (string-or-object, e.g. via `json.RawMessage` / custom unmarshal).
   2. When the source is a subagent object, `handleSessionMeta` returns nil without setting `p.sessionId` — every subsequent line of the rollout is ignored by the existing `sessionId == ""` guards.
3. `session_list` no longer shows sub-agent sessions (parity with Claude's `isSidechain` filter). Lineage surfacing is backlog.

### Git branch correction

1. Codex writes `session_meta` with `payload.git = {commit_hash, branch, repository_url}` plus `originator`, `cli_version`, `source`, `forked_from_id` (and, for sub-agents, `parent_thread_id`, `agent_nickname`).
2. Backend
   1. `codex.GitInfo` gains `Branch string` (json `branch`).
   2. `handleSessionMeta` sets `Meta.GitBranch = meta.Git.Branch` (today: `meta.Git.CommitHash`).
   3. Empty branch stays empty — no SHA fallback.
   4. The remaining fields are parsed into the extended `codex.SessionMeta` and mapped into the `session.Meta` extension (one struct, one work item).

### Later `proposed_plan` blocks replace earlier ones

1. Codex revises the plan with a new `<proposed_plan>` block in a later assistant message (the mode spec requires each new block to be a complete replacement).
2. Backend
   1. Parser emits a fresh plan-signal turn; store overwrites `PlanContent` (existing behavior).
3. `session_plan` now reflects the latest plan revision.

---

## Security Considerations

- The sentinel `PlanFilePath: "codex:proposed_plan"` must never reach `os.ReadFile` or any filesystem/git call. Only `session/store.go` reads `PlanFilePath`, guarded by the `PlanContent != ""` check — add a test pinning that invariant.
- Extracted plan text is treated as untrusted content (display only, never interpolated into commands).
- `source` is model-external but polymorphic (string-or-object): a tolerant decode must not fail the whole `session_meta` unmarshal; malformed values degrade to "unknown source", never to a dropped session for normal rollouts.

---

## Limits

- Extracted Codex plan: 32 KB cap (larger output indicates parse garbage — truncate and log).

---

## Models

### codex.GitInfo (extended)

**Internal / Not Exported:**
- branch: git branch name (string, new)
- commit_hash: existing field, no longer written to `Meta.GitBranch` — moves to the `Meta` extension

### codex.SessionMeta (extended)

**Internal / Not Exported:**
- originator: launching client, e.g. `vscode` (string, new)
- cli_version: Codex CLI version (string, already parsed — now mapped into `session.Meta`)
- source: string-or-object; object form carries `subagent.thread_spawn` lineage (tolerant decode, new)
- forked_from_id: origin session for forks (string, new)
- parent_thread_id: sub-agent parent session (string, new)
- agent_nickname: sub-agent display name (string, new)

### session.Meta (extension)

**Exported (session_list / session_get):**
- one extension struct carrying originator, cli_version, source kind, forked_from_id, repository_url, commit_hash, sub-agent lineage — populated by the Codex parser; Claude fills what it has (`version`)

---

## APIs

No tool signature changes. Description update only:

### session_plan

**Notes:**
- Description becomes: "For Claude: the plan-mode plan file. For Codex: the latest proposed_plan block."

---

## Long-Tail Tasks

### Testing

- Fixture from a real plan-mode rollout: `<proposed_plan>` extraction, last-block-wins, message text preserved as chat turn.
- Fixture from a real sub-agent rollout: whole session dropped; string `source` sessions unaffected.
- Fixture asserting `git.branch` parsing, that `GitBranch` no longer carries the commit SHA, and that the `Meta` extension fields are populated.
- Store test pinning the sentinel-never-read-as-file invariant.

### Docs

- README parity table (condensed matrix) once MVP lands.
