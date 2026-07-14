# Concept: Codex / Claude Feature Parity

> **Status:** Draft
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
| Plan | Plan attachments (4 types) + plan file watcher | `update_plan` function calls + `collaboration_mode.mode == "plan"` — **not parsed** | Gap → MVP here (see [codex_parser_fixes.md](codex_parser_fixes.md)) |
| Git branch | `entry.gitBranch` per entry | **Bug**: commit hash stored as `GitBranch` (codex/parser.go:69); payload `git.branch` never parsed | Fix → MVP here |
| Git repo URL / commit | Not captured | In `session_meta.git` — dropped | Backlog (Meta extension) |
| Model | Per assistant message (`message.model`) | `turn_context.model` — captured | Parity OK |
| Token usage | Per-message deltas summed into `TotalUsage` | `token_count` events **silently dropped** by `Turn.Validate`; values are cumulative, summing would over-count | Gap → fix specified in [usage_reporting](../../../../../Documents/plans/usage_reporting/concept.md) |
| Cost | Absent | Absent | Gap for both → [usage_reporting](../../../../../Documents/plans/usage_reporting/concept.md) |
| CLI metadata | `version` field unparsed | `originator`, `cli_version`, `source` — dropped | Backlog (Meta extension) |
| Session forks | Not applicable | `ForkedFromId` — dropped | Backlog |
| Tool calls | Text blocks only; tool_use/tool_result filtered | `function_call`/`custom_tool_call`/`reasoning` dropped | Intentional, parity-neutral; optional `include_tools` backlog |
| Sidechains | Filtered via `isSidechain` | Sub-agent marker unknown | Open question 2 |
| Diffs (both diff tools) | Via `Meta.CWD` | Via `Meta.CWD` | Parity OK |
| Pagination | 800 KB response cap (client detection, `tools/forms.go`) | Unlimited | Intentional — keyed on client capability, not agent |

---

## User Flows

### Read a Codex plan

**Goals:**
- `session_plan(agent: "codex")` returns the plan Codex itself maintains, instead of "No plan found".

**Options:**

**MVP**
- Parse `update_plan` function calls from Codex rollouts; render the latest call as a markdown checklist and store it as the session's `PlanContent`.
- Include the `explanation` field from the call as a lead paragraph.

**Backlog**
- Plan revision history for Codex (`update_plan` call sequence) feeding the [plan_history](../../../../../Documents/plans/plan_history/concept.md) revision buffer.
- Distinguish plan-mode plans (`collaboration_mode.mode == "plan"`) from ad-hoc todo lists in metadata.

**Challenges:**
- `update_plan` arrives as a `function_call` whose `arguments` is a JSON **string** — requires a double decode.
- Multiple `update_plan` calls per session; only the latest reflects current state.
- Codex plans have no backing file, but `Session.PlanFilePath` participates in store routing and `HasPlan` checks.

**Approach:**
- Last `update_plan` call wins (each call carries the full step list).
- Emit a plan-signal turn with sentinel `PlanFilePath: "codex:update_plan"` and non-empty `PlanContent`. The store returns early when `PlanContent != ""` before any `os.ReadFile`, so the sentinel never touches the filesystem — pin this with a test.

### Correct Codex session metadata

**Goals:**
- `session_list` / `session_get` for Codex sessions show a real branch name ("develop"), not a 40-char commit SHA.

**Options:**

**MVP**
- Parse `git.branch` from `session_meta` payload into a new `codex.GitInfo.Branch` field; use it for `Meta.GitBranch`.

**Backlog**
- Capture `repository_url` and `commit_hash` in a `Meta` extension.
- Capture `originator`, `cli_version`, `source`, `forked_from_id`.

**Challenges:**
- The current bug means existing consumers may have adapted to seeing a SHA; branch may also be empty for detached HEAD.

**Approach:**
- Straight field fix; empty branch stays empty (no SHA fallback in MVP).

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
- Parity fixes are implemented in `codex/parser.go` only, emitting Claude-shaped signal turns, so `session.Store` and `tools/` stay agent-agnostic.
- Codex plan = rendered latest `update_plan` payload; no plan watcher needed (no backing file).
- Git branch fix: add `Branch` to `codex.GitInfo`; commit hash is not stored in MVP.
- The Codex usage fix is owned by the usage_reporting concept (single source of truth); this concept does not re-design it parser-side.
- Tool-call filtering stays symmetric and intentional for both agents.
- The parity matrix above is the canonical inventory; the README gets a condensed version once MVP items land.

**Open Questions:**
1. `update_plan` is observed in sessions outside plan mode too. Should `HasPlan` for Codex mean "was in plan mode" or "has a todo list"? (Proposal: has a todo list — matches what `session_plan` returns.)
2. What marks Codex sub-agent/sidechain output, if anything? Claude's `isSidechain` filter has no verified Codex equivalent.
3. Claude streaming chunks merge by `RequestId` keeping the last entry's usage (session/session.go:49-54) — verify chunks repeat identical message-level usage rather than needing summation.
4. Should the dropped Codex metadata (originator, cli_version, source, forked_from_id) land as one `Meta` extension or per-field as needed?
