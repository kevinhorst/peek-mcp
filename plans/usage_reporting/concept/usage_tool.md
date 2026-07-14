# session_usage Tool & Pricing

---

## Flows

### Ingest — Claude

1. Claude Code appends an assistant entry with a `usage` object.
2. Backend
   1. `claude/parser.go` unchanged (already emits per-turn `Usage` + `Meta.Model`).
   2. `Session.AddTurn` retires a turn → `TotalUsage.Add(u)` (existing) **and** `UsageByModel[model].Add(u)` (new; empty model keyed `"unknown"`).

### Ingest — Codex

1. Codex appends an `event_msg` of type `token_count` with cumulative `total_token_usage`.
2. Backend
   1. `codex/parser.go` `handleEventMessage` additionally sets `UsageCumulative: true` and `Meta.Model = p.model` on the usage turn.
   2. `Turn.Validate` (session/turn.go): new branch — a usage-signal turn is valid with only `SessionId` + `Usage` (today it is dropped).
   3. `Store.AddTurnBySessionId`: usage-signal turns route to `session.ApplyCumulativeUsage(u, model)` instead of `AddTurn` — computes `delta = u − CumulativeUsage` clamped ≥ 0 per field, adds the delta to `UsageByModel[model]`, sets `CumulativeUsage = u`.
3. The session total for Codex is `CumulativeUsage`.

### session_usage call

1. Agent calls `session_usage` (id/title optional, defaults to most recent session).
2. Backend
   1. Resolve agent + session with the existing helpers.
   2. Effective totals: Claude → `TotalUsage + TurnActive.Usage`; Codex → `CumulativeUsage`.
   3. For each `UsageByModel` entry: `pricing.Match(model)` (longest prefix) → per-model cost via the agent formula; sum known-model costs; collect unknown models.
   4. If `include_turns`: serialize buffered turns carrying `Usage`, each with per-turn cost; set `turns_truncated` when the ring buffer is at capacity.
3. Caller receives totals, per-model split, cost estimate, provenance fields.

### Startup pricing load

1. `peek start` begins (cmd/start.go).
2. Backend
   1. Resolve `--pricing-file` / `PEEK_PRICING_FILE`; default `~/.config/peek/pricing.json` if it exists.
   2. Strict-parse (`DisallowUnknownFields`), validate (non-negative rates, size/entry caps), merge per-model over embedded defaults, log the merge summary.
   3. Malformed file → warning + fall back to embedded defaults; server still starts.

---

## Security Considerations

- Pricing file is local, user-owned input: strict JSON decode, 64 KiB / 200-entry caps, no network fetch (auto-updating price feeds are an explicitly deferred supply-chain surface; embedded defaults update via releases).
- Costs are labeled estimates (`pricing_as_of`, `cost_complete`) so downstream automation doesn't treat them as billing truth.
- No new data exposure: token counts already appear on serialized turns; the tool adds aggregation only. Transport remains loopback-bound.

---

## Limits

- Pricing override file: ≤ 64 KiB, ≤ 200 model entries (sanity bounds on a startup-parsed user file).
- `UsageByModel`: ≤ 16 entries per session; overflow keyed `"other"` (sessions realistically touch 2–4 models; 16 guards against garbage model strings).
- Per-turn breakdown: bounded by `--depth` (default 20); never exceeds the ring — reported via `turns_truncated`.
- Cost rounding: 6 decimal places (sub-cent precision matters for haiku-class turns).
- Codex delta clamping: any negative per-field delta (compaction/reset) clamps to 0, never subtracts.

---

## Models

### pricing.ModelPricing (new package `pricing`)

**Public:**
- input: USD per MTok input (float64)
- output: USD per MTok output (float64)
- cache_write: USD per MTok cache creation — Anthropic default input × 1.25 (float64)
- cache_read: USD per MTok cache read — Anthropic default input × 0.10 (float64)

### pricing.Table

**Public:**
- as_of: table date, e.g. "2026-07" (string)
- models: model-id **prefix** → ModelPricing (map)

**Internal / Not Exported:**
- source: `embedded` | `override` | `mixed` (string)
- Match(modelId): longest-prefix lookup

### session.Session (extended)

**Internal / Not Exported:**
- UsageByModel: ingest-time per-model accumulation (map[string]*Usage)
- CumulativeUsage: latest Codex snapshot (*Usage)

### session.Turn (extended)

**Internal / Not Exported:**
- UsageCumulative: signals Usage is a cumulative snapshot (bool, json:"-")

### tools.sessionListItem (extended)

**Public:**
- total_tokens: effective total for quick comparison — Claude: sum of in/out/cache fields; Codex: snapshot `total_tokens` (int)

---

## APIs

### Tool: session_usage

Report token usage and estimated cost for one session.

**Notes:**
- Defaults to the most recently active session when neither `id` nor `title` is given.
- Per-turn costs are Claude-only in MVP (Codex per-turn attribution is backlog).

**Request fields:**
- id: session ID (string, optional)
- title: exact session title, substring fallback (string, optional)
- agent: "claude" or "codex"; defaults to the sole enabled agent (string, required)
- include_turns: per-turn breakdown (boolean, default false)
- n: max turns in breakdown (number, default 20, capped at ring depth)

**Response fields:**
- session_id, agent, title, last_active
- totals: effective Usage totals
- by_model: array of { model, usage, estimated_cost_usd (float or null) }
- estimated_cost_usd: sum over known models (float or null)
- cost_complete: false when unknown models contributed tokens (boolean)
- unknown_models: model ids without pricing (array)
- pricing_source: embedded | override | mixed (string)
- pricing_as_of: embedded/override table date (string)
- turns: optional array of { timestamp, role, model, usage, estimated_cost_usd }
- turns_truncated: true when the ring buffer evicted older turns (boolean)

**Example (success):**

Request:
```json
{ "agent": "claude", "include_turns": false }
```

Response:
```json
{
  "session_id": "6ef1998c-2774-4680-b12d-8490b261bd51",
  "agent": "claude",
  "title": "Fix watcher offsets",
  "totals": { "input_tokens": 41200, "output_tokens": 9800, "cache_creation_input_tokens": 12000, "cache_read_input_tokens": 310000 },
  "by_model": [
    { "model": "claude-sonnet-5", "usage": { "input_tokens": 39000, "output_tokens": 9000, "cache_creation_input_tokens": 12000, "cache_read_input_tokens": 300000 }, "estimated_cost_usd": 0.4213 }
  ],
  "estimated_cost_usd": 0.4213,
  "cost_complete": true,
  "unknown_models": [],
  "pricing_source": "embedded",
  "pricing_as_of": "2026-07"
}
```

---

## Long-Tail Tasks

### Prerequisite fixes

- `Turn.Validate` usage-signal branch + regression test proving Codex `token_count` turns reach the store.
- Codex parser: stamp `UsageCumulative` + `Meta.Model` on usage turns.

### Testing

- Store/session tests: cumulative-delta math incl. clamp-on-reset; Claude active-turn inclusion in effective totals; `UsageByModel` overflow bucket.
- Verify Q1 (reasoning ⊂ output) against real rollout files before finalizing the Codex formula.
- Verify Q4 (RequestId-merge usage semantics) against a streamed Claude session.

### Pricing table

- Embedded defaults for current Anthropic (Claude 5 family, Opus/Sonnet/Haiku 4.x) and OpenAI Codex models; prefix keys verified against real `message.model` and `turn_context.model` values.

### Docs

- README: pricing override format, example file, "estimates only" disclaimer.
