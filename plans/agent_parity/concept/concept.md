# Concept: Codex / Claude Feature Parity

> **Status:** In Review
> **Author:** Kevin Horst
> **Date:** 2026-07-06

---

## Goals

- Every per-agent behavior difference in Peek is either intentional and documented, or a tracked gap with a concept/work item.
- Close the two user-visible gaps: title addressing for Codex sessions and `session_plan` returning nothing for Codex.
- Fix silent data-quality defects in the Codex parser (git branch bug, dropped usage, dropped metadata).
- Keep `session.Store` and `tools/` agent-agnostic: parity fixes live in the parsers, which emit the same signal-turn shapes for both agents.

---

## Parity Matrix

Verified against `claude/parser.go`, `codex/parser.go`, and sampled transcripts.

| Capability | Claude Code | Codex | Status / Action |
|---|---|---|---|
| Title (explicit) | `custom-title` entries → `Store.IdByTitle` | `session_index.jsonl` `thread_name` — **not read** | Gap → [codex_title_search](../codex_title_search/concept.md) |
| Title (AI/derived) | `ai-title` entries exist — not parsed | `thread_name` is AI-generated | Codex covered by title concept; Claude `ai-title` backlog |
| Plan | Plan attachments (4 types) + plan file watcher | `<proposed_plan>` block in assistant messages (plan mode) — **not extracted**. `update_plan` is a todo tool, errors in plan mode — intentionally not a plan | Gap → MVP here (see [codex_parser_fixes.md](codex_parser_fixes.md)) |
| Git branch | `entry.gitBranch` per entry | **Bug**: commit hash stored as `GitBranch` (codex/parser.go:69); payload `git.branch` never parsed | Fix → MVP here |
| Git repo URL / commit | Not captured | In `session_meta.git` — discarded by parser | MVP here (Meta extension) |
| Model | Per assistant message (`message.model`) | `turn_context.model` — captured | Parity OK |
| Token usage | Per-message deltas summed into `TotalUsage` | `token_count` events **silently dropped** by `Turn.Validate`; values are cumulative, summing would over-count | Gap → fix specified in [usage_reporting](../../../../../Documents/plans/usage_reporting/concept.md) |
| Cost | Absent | Absent | Gap for both → [usage_reporting](../../../../../Documents/plans/usage_reporting/concept.md) |
| CLI metadata | `version` field unparsed | `originator`, `cli_version`, `source` — discarded by parser | MVP here (Meta extension) |
| Session forks | Not applicable | `forked_from_id` — discarded by parser | MVP here (Meta extension) |
| Tool calls | Text blocks only; tool_use/tool_result filtered | `function_call`/`custom_tool_call`/`reasoning` dropped | Intentional, parity-neutral; optional `include_tools` backlog |
| Sidechains | Filtered via `isSidechain` | Sub-agent sessions are separate rollout files; `session_meta.source` = `{subagent: {thread_spawn: {...}}}` object instead of string — currently leak into `session_list` | Fix → MVP here (drop at parse) |
| Diffs (both diff tools) | Via `Meta.CWD` | Via `Meta.CWD` | Parity OK |
| Pagination | 800 KB response cap (client detection, `tools/forms.go`) | Unlimited | Intentional — keyed on client capability, not agent |

---

## User Flows

### Read a Codex plan

**Goals:**
- `session_plan(agent: "codex")` returns the plan-mode plan Codex produced, instead of "No plan found".

**Options:**

**MVP**
- Extract the latest `<proposed_plan>…</proposed_plan>` markdown block from assistant messages and store its content as the session's `PlanContent`. Codex plan mode writes no plan file — the block inside the final assistant message IS the plan (mandated by the plan-mode developer instructions embedded in `turn_context`).

**Backlog**
- Plan revision history for Codex (`<proposed_plan>` block sequence) feeding the [plan_history](../../../../../Documents/plans/plan_history/concept.md) revision buffer.

**Challenges:**
- The plan block sits inside a normal assistant `message` item — the message must both remain visible as a chat turn and feed `PlanContent` (the store's plan branch currently returns early, skipping `AddTurn`; a fall-through when `turn.Text != ""` is agent-agnostic and backward compatible since Claude plan turns carry no text — mechanism finalized in feature-design).
- Multiple `<proposed_plan>` blocks per session; each new block is a complete replacement per the mode spec.
- Codex plans have no backing file, but `Session.PlanFilePath` participates in store routing and `HasPlan` checks.

**Approach:**
- Last `<proposed_plan>` block wins (spec-guaranteed complete replacement).
- Emit a plan-signal turn with sentinel `PlanFilePath: "codex:proposed_plan"` and non-empty `PlanContent`. The store returns early when `PlanContent != ""` before any `os.ReadFile`, so the sentinel never touches the filesystem — pin this with a test.
- `update_plan` checklists are intentionally not surfaced: they are a todo/progress tool, explicitly rejected by Codex during plan mode, not a plan.

### Correct Codex session metadata

**Goals:**
- `session_list` / `session_get` for Codex sessions show a real branch name ("develop"), not a 40-char commit SHA.

**Options:**

**MVP**
- Parse `git.branch` from `session_meta` payload into a new `codex.GitInfo.Branch` field; use it for `Meta.GitBranch`.
- Capture the full `session_meta` metadata set in one `Meta` extension: `originator`, `cli_version`, source kind, `forked_from_id`, `git.repository_url`, `git.commit_hash`, sub-agent lineage (`parent_thread_id`, `agent_nickname`).
- Drop sub-agent sessions at parse time: `session_meta.source` is a `{subagent: {thread_spawn: {...}}}` object (vs the string `"vscode"`); when detected, the parser never sets `sessionId`, so the whole rollout is ignored — the Codex analog of Claude's `isSidechain` filter.

**Backlog**
- Surface sub-agent lineage (list sub-agent sessions under their parent) instead of dropping them.

**Challenges:**
- The current bug means existing consumers may have adapted to seeing a SHA; branch may also be empty for detached HEAD.
- `source` is string-or-object — needs a tolerant decode (`json.RawMessage` / custom unmarshal), not a plain string field.

**Approach:**
- Straight field fix; empty branch stays empty (no SHA fallback in MVP).
- One extension struct, one work item — no per-field backlog drips.

### Accurate Codex token usage

**Goals:**
- `Session.TotalUsage` for Codex matches Codex's own final token count.

**Options:**

**MVP**
- Covered by [usage_reporting](../../../../../Documents/plans/usage_reporting/concept.md): validation fix + cumulative-snapshot semantics. This concept only tracks it as a parity item — the mechanism is specified exactly once, there.

**Challenges:**
- Two semantics behind one `TotalUsage` field (Claude additive, Codex cumulative).

**Approach:**
- Store-side per-session handling (`Turn.UsageCumulative` signal + `Session.ApplyCumulativeUsage`); see usage_reporting for design and rationale.

---

## Decisions / Open Questions

**Decisions:**
- Parity fixes are implemented in `codex/parser.go` only, emitting Claude-shaped signal turns, so `session.Store` and `tools/` stay agent-agnostic (the store's plan-branch fall-through for text-bearing plan turns is agent-agnostic and allowed).
- [USER] Codex plan = content of the latest `<proposed_plan>` block from assistant messages, nothing else; `HasPlan` for Codex = session produced a plan-mode plan. Rationale: Codex writes no plan files — the plan-mode developer instructions mandate the `<proposed_plan>` block as the plan artifact and explicitly reject `update_plan` during plan mode; verified in all 5 plan-mode rollouts on disk (e.g. `~/.codex/sessions/2026/07/11/rollout-2026-07-11T20-09-07-*.jsonl`). `update_plan` checklists are intentionally not surfaced.
- Later `<proposed_plan>` blocks fully replace earlier ones — last block wins (the mode spec requires each new block to be a complete replacement).
- Codex sub-agent sessions (marker: `session_meta.source` object containing `subagent.thread_spawn`) are dropped at parse time, mirroring Claude's `isSidechain` filter. Evidence: 3 sub-agent rollouts on disk, all carrying `parent_thread_id` / `agent_nickname`; today they leak into `session_list` as ordinary sessions. Lineage surfacing is backlog.
- Claude streaming usage merge (session/session.go:49-54, keep last entry) is correct as-is. Evidence: chunks sharing a `requestId` repeat byte-identical message-level usage (same `message.id`); zero divergent requestIds across the 5 most recent peek-mcp transcripts. Summation would over-count.
- [USER] All available `session_meta` metadata is captured in the MVP as one `Meta` extension: `originator`, `cli_version`, source kind, `forked_from_id`, `repository_url`, `commit_hash`, sub-agent lineage. Capture as much as possible now; per-field backlog drips are dropped as a notion.
- Git branch fix: add `Branch` to `codex.GitInfo`; commit hash moves to the `Meta` extension instead of masquerading as the branch.
- The Codex usage fix is owned by the usage_reporting concept (single source of truth); this concept does not re-design it parser-side.
- Tool-call filtering stays symmetric and intentional for both agents.
- The parity matrix above is the canonical inventory; the README gets a condensed version once MVP items land.

**Open Questions:**
_None._
