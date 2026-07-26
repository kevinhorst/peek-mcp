# Concept: Usage Handling (Token & Cost per Session)

> **Status:** Draft
> **Author:** Kevin Horst
> **Date:** 2026-07-06

---

## Goals

- Report accurate token totals per session for both agents, and per turn where available, with a per-model split.
- Provide estimated USD cost from a pricing table that ships with sane defaults and is user-overridable.
- Fix two existing correctness defects in the Codex usage pipeline:
  1. **Usage is dropped entirely**: `codex/parser.go` emits usage-only turns for `token_count` events, but `Turn.Validate` (called from `watcher/watcher.go`) only whitelists plan/title signal turns — usage turns are silently discarded, so `Session.TotalUsage` is always zero for Codex.
  2. **Cumulative semantics**: Codex `token_count` events carry cumulative `total_token_usage`; summing them via `Session.AddTurn → TotalUsage.Add()` would over-count roughly quadratically. Correct Codex semantics is last-snapshot. (Claude per-turn usage is per-API-response; summing is correct, with a minor undercount because the active turn's usage folds in only when the next turn arrives.)

---

## User Flows

### Session cost check

**Goals:**
- "What has this session cost so far?" answered in one tool call.

**Options:**

**MVP**
- New `session_usage` tool: session totals, per-model split, `estimated_cost_usd`.
- `session_list` items gain `total_tokens` so the expensive session is spottable without N calls.

**Backlog**
- `estimated_cost_usd` column in `session_list`; usage block in `session_full`.
- Cross-session aggregate tool (`usage_summary`: totals by model across sessions, per day).

**Challenges:**
- One `Session.TotalUsage` field, two semantics (Claude additive, Codex cumulative).
- Schema mismatch: Claude `input_tokens` **excludes** cache tokens (`cache_creation`, `cache_read` separate); Codex `input_tokens` **includes** `cached_input_tokens`. Naive summation double- or under-counts.

**Approach:**
- Ingest-time accumulation with agent-appropriate semantics (see Decisions); effective totals computed at read time.

### Per-turn breakdown

**Goals:**
- See which turns were expensive.

**Options:**

**MVP**
- `session_usage(include_turns: true)`: last N turns' usage + per-turn cost (Claude), with `turns_truncated: true` when the ring buffer has evicted older turns.

**Backlog**
- Per-turn Codex usage via cumulative-delta attribution to the preceding assistant turn.

**Challenges:**
- Per-turn usage only exists for the last `--depth` (default 20) turns — the ring buffer evicts older ones, so per-model totals cannot be derived from the buffer.

**Approach:**
- Accumulate per-model totals at ingest (`Session.UsageByModel`), independent of the ring buffer; the per-turn view is explicitly bounded and labeled truncated.

### Custom pricing

**Goals:**
- Costs stay meaningful when prices change or new models appear.

**Options:**

**MVP**
- Embedded defaults table + user override file (`--pricing-file` / `PEEK_PRICING_FILE`, default `~/.config/peek/pricing.json` if present), merged per model at startup.
- Unknown models yield `estimated_cost_usd: null` contribution plus an `unknown_models` list and `cost_complete: false` — never silent zeros.

**Backlog**
- Hot-reload of the pricing file (mtime check); `peek pricing` subcommand printing the effective table.

**Challenges:**
- Any embedded table is stale the day a price changes; costs must be clearly labeled estimates.
- An auto-updating remote price feed is a supply-chain surface.

**Approach:**
- No network fetch — embedded defaults update via releases; responses carry `pricing_source` and `pricing_as_of` so consumers can judge freshness. Costs computed at read time (never stored), so a pricing correction retroactively fixes all reported costs.

---

## Decisions / Open Questions

**Decisions:**
- New dedicated `session_usage` tool; `session_list` gains only `total_tokens`; `session_full` untouched in MVP. (Rejected: extending `session_full` — usage questions are asked independently of transcript reading, and cost payloads would bloat every response.)
- `Turn.Validate` accepts usage-signal turns (`Usage != nil && Meta.SessionId != ""`), mirroring plan/title signals.
- **Codex semantics fix is store-side, per session** (this concept is the single source of truth; the [agent_parity](../agent_parity/concept.md) concept references it): the Codex parser stamps `UsageCumulative: true` + `Meta.Model` on usage turns; `Store.AddTurnBySessionId` routes them to `session.ApplyCumulativeUsage(u, model)`, which computes the per-field delta vs. the previous snapshot (clamped ≥ 0 to survive compaction/resets), adds the delta to `UsageByModel`, and keeps `CumulativeUsage = u` as the session total. Session-scoped state stays correct regardless of how parser instances map to files.
- Claude stays additive; effective total at read time = `TotalUsage + TurnActive.Usage` (fixes the active-turn undercount).
- Per-model accumulation happens in the ingest path into `Session.UsageByModel` (Claude keyed by `Meta.Model` per retired turn; Codex delta keyed by the parser's current model).
- Pricing model: four explicit per-MTok rates (`input`, `output`, `cache_write`, `cache_read`) rather than multipliers — Anthropic's 1.25×/0.1× become concrete numbers in the defaults, and non-Anthropic models don't have to fit that scheme. Longest-prefix model matching.
- Cost formulas — Claude: `in·Pin + cache_creation·Pcw + cache_read·Pcr + out·Pout`; Codex: `(in − cached)·Pin + cached·Pcr + out·Pout` (cached is a subset of input; reasoning tokens assumed included in `output_tokens`).
- Pricing load order: embedded defaults ← override file (per-model replace), once at startup; malformed override logs a warning and falls back (server still starts).

**Open Questions:**
1. Codex `reasoning_output_tokens` — confirm against real rollout files that it is included in `output_tokens` (the formula assumes yes; if not, add a `reasoning·Pout` term).
2. Codex per-model split when the model changes mid-session (turn_context between snapshots): is attributing the whole delta to the current model acceptable for MVP?
3. Unknown-model cost policy: report cost for known models with `cost_complete: false` and `unknown_models` list (proposed), or force session `estimated_cost_usd` to null?
4. Claude same-`RequestId` merge keeps only the last entry's usage — verify streaming chunks repeat identical message-level usage rather than needing summation.
5. Is `~/.config/peek/pricing.json` the right default override location on macOS, or `~/.peek/pricing.json`?
6. Anthropic 1-hour cache writes bill at 2×, not 1.25× — the transcript doesn't distinguish; accept the 1.25× assumption?
