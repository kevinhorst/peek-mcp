# Use Case: Session Analysis

Retrieve everything a finished session produced — turns, plan, diff, metadata — from one tool surface, addressed by human-readable title. Claim level: **optimal**. This is the flagship claim: after a deep-analysis session completes, one title query returns the complete package; nothing else on the market reads Claude Code and Codex transcripts this way.

---

## Flows

### Retrieve a finished session by title

1. A session titled `[Peek, Concept, F-H] Deep Analysis` has completed.
2. An analysis session (any connected client) calls `session_get(title: "[Peek, Concept, F-H] Deep Analysis")`.
3. Backend
   1. Title resolution: exact case-insensitive match first, substring fallback ("Deep Analysis" alone finds it), scoped by `agent` when given.
   2. Returns the last N turns — human/assistant pairs only, tool calls and results already filtered, sidechains already hidden.
4. Follow-ups as needed: `session_full` for turns + plan + diff in one call, `session_plan` / `session_diff` individually.

### Batch analysis entry point

1. Analysis session calls `session_list` — every known session with ID, agent, title, title source, last-active, `HasPlan`/`HasDiff`, cwd, branch, model, origin.
2. It selects sessions programmatically (by branch, by title pattern, by recency) and drills into each — the foundation for retrospective and session-mining workflows over many sessions.

### The counterfactual (shown in the doc as before/after)

1. Without peek: glob `~/.claude/projects/<encoded-cwd>/*.jsonl`, parse an undocumented line format, strip tool-use/tool-result noise, detect and drop sidechain entries, track format changes across CLI releases — per agent, twice for Codex's different rollout format.
2. With peek: one tool call. The parser churn is peek's problem, permanently.

---

## Tools Used

| Tool | Params in this flow | Contribution |
|---|---|---|
| `session_get` | `title`, `n`, optional `agent` | Title-addressed transcript retrieval |
| `session_full` | `title`/`id`, `n`, `request_id` | Complete package: turns + plan + diff |
| `session_list` | optional `agent` | Metadata inventory; batch-selection entry point |
| `session_diff` | `title`/`id` | What the session actually changed vs `main` |

---

## Claims & Evidence

| ID | Type | Must show | Proves |
|---|---|---|---|
| SA-1 | Screenshot | `session_get(title: "[Peek, Concept, F-H] Deep Analysis")` call and the returned clean turns | Title addressing works on a real, messy-titled session; output is analysis-ready |
| SA-2 | Video | An analysis pass: `session_list` → pick session → `session_full` → the analyzing model producing findings from the payload | End-to-end analysis needs nothing outside peek |
| SA-3 | Screenshot | Split view: raw JSONL lines of the same session next to the peek response | The "optimal vs raw files" claim, demonstrated not asserted |

Capture instructions: SA-1 uses the real deep-analysis session (its bracket-heavy title is itself a demo of substring matching — capture a second query with just "Deep Analysis"); SA-3's raw side should include at least one tool_use line and one sidechain line to show what peek filters.

---

## Comparison vs Alternatives

| Alternative | What it costs |
|---|---|
| Reading raw JSONL | Undocumented format ×2 agents, tool noise, sidechains, format churn per CLI release |
| Built-in session resume/memory features | Bound to the same vendor's client; no cross-agent access, no structured metadata, no diff |
| Log/observability tooling around agents | Requires instrumenting the agents; peek needs zero agent-side changes |

---

## Limits

- Turn depth bounded by the `--depth` ring buffer — run peek with a larger depth (e.g. `--depth 100`) when sessions targeted for analysis are long.
- Sessions are known from transcripts parsed since peek started; point `--claude-home`/`--codex-home` at the right roots.
- Token-usage metadata for Codex sessions is not yet exposed (tracked in the usage_reporting concept) — usage-based analysis is Claude-only today.
