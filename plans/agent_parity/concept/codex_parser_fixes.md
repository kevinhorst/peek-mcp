# Codex Parser Fixes (Parity MVP)

---

## Flows

### Codex plan ingestion

1. Codex writes a `response_item` entry of type `function_call` with `name: "update_plan"` to the rollout file.
2. Watcher tails the file and passes the line to `codex/parser.go`.
3. Backend
   1. `handleResponseItem` accepts `item.Type == "function_call" && item.Name == "update_plan"` (today only `message` items pass).
   2. Double-decode: `item.Arguments` (JSON string) → `UpdatePlanArgs{Explanation, Plan []PlanStep}`.
   3. Render markdown: `explanation` as lead paragraph, then one line per step — `- [x] {step}` for `completed`, `- [ ] {step}` for `pending` / `in_progress` (annotate `(in_progress)`).
   4. Emit `session.Turn{PlanContent: md, PlanFilePath: "codex:update_plan", Meta: {SessionId}}`.
   5. `Store.AddTurnBySessionId` routes it through the existing plan branch; `PlanContent != ""` returns before any `os.ReadFile`, so the sentinel path is never treated as a file.
4. `session_plan(agent: "codex")` returns the rendered checklist; `session_list` shows `has_plan: true`.

### Git branch correction

1. Codex writes `session_meta` with `payload.git = {commit_hash, branch, repository_url}`.
2. Backend
   1. `codex.GitInfo` gains `Branch string` (json `branch`).
   2. `handleSessionMeta` sets `Meta.GitBranch = meta.Git.Branch` (today: `meta.Git.CommitHash`).
   3. Empty branch stays empty — no SHA fallback.

### Later `update_plan` calls replace earlier ones

1. Codex updates step statuses via a new `update_plan` call.
2. Backend
   1. Parser emits a fresh plan-signal turn; store overwrites `PlanContent` (existing behavior).
3. `session_plan` now reflects the latest checklist.

---

## Security Considerations

- `arguments` is model-generated JSON: decode with strict field types; ignore unknown fields; malformed arguments are skipped with a debug log, never partially rendered.
- The sentinel `PlanFilePath: "codex:update_plan"` must never reach `os.ReadFile` or any filesystem/git call. Only `session/store.go` reads `PlanFilePath`, guarded by the `PlanContent != ""` early return — add a test pinning that invariant.
- Rendered step text is treated as untrusted content (display only, never interpolated into commands).

---

## Limits

- Rendered Codex plan: 32 KB cap (plans are step lists; larger output indicates parse garbage — truncate and log).
- Steps rendered per plan: 100 (same rationale; Codex plans are typically < 10 steps).

---

## Models

### codex.UpdatePlanArgs

**Internal / Not Exported:**
- explanation: optional context for the plan update (string)
- plan: ordered step list (array)
- plan[].step: step description (string)
- plan[].status: `pending` | `in_progress` | `completed` (string)

### codex.GitInfo (extended)

**Internal / Not Exported:**
- branch: git branch name (string, new)
- commit_hash: existing field, no longer written to `Meta.GitBranch`

### codex.ResponseItem (extended)

**Internal / Not Exported:**
- name: function call name, e.g. `update_plan` (string, new)
- arguments: JSON-encoded arguments string (string, new)

---

## APIs

No tool signature changes. Description update only:

### session_plan

**Notes:**
- Description becomes: "For Claude: the plan-mode plan file. For Codex: the agent's latest update_plan checklist."

---

## Long-Tail Tasks

### Testing

- Fixture from a real rollout for `update_plan` (arguments double-decode, status rendering).
- Fixture asserting `git.branch` parsing and that `GitBranch` no longer carries the commit SHA.
- Store test pinning the sentinel-never-read-as-file invariant.

### Metadata enrichment (backlog)

- `Meta` extension: `originator`, `cli_version`, `source`, `forked_from_id`, `repository_url`, `commit_hash`.
- Decide Codex sidechain/sub-agent detection (open question 2 in concept.md).

### Docs

- README parity table (condensed matrix) once MVP lands.
