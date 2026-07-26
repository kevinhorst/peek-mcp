# User Stories: Usage Handling (Token & Cost per Session)

---

## Cost visibility

**As a** developer running agent sessions, **I want** to ask "what has this session cost so far?" and get a dollar estimate, **so that** I can decide whether to keep iterating or change approach.

**As a** developer, **I want** costs labeled with the pricing table's date and source, **so that** I treat them as estimates, not billing truth.

**As a** developer using a model Peek doesn't know, **I want** an explicit `unknown_models` list instead of a silent zero, **so that** I don't mistake incomplete costs for cheap sessions.

---

## Token breakdowns

**As a** developer, **I want** per-session token totals split by model, **so that** I can see how much went to the main model vs. cheap subagent models.

**As a** developer, **I want** a per-turn usage breakdown for recent turns, **so that** I can find the turn that blew the budget.

**As a** Peek user comparing sessions, **I want** `total_tokens` in `session_list`, **so that** I can spot the expensive session without one call per session.

---

## Correctness

**As a** Codex user, **I want** Peek's token totals to match what Codex itself reports, **so that** cross-agent comparisons are meaningful. *(Today Codex usage is silently dropped.)*

**As a** Claude Code user, **I want** the in-flight turn's tokens included in the session total, **so that** the number doesn't lag one turn behind.

---

## Pricing control

**As a** user, **I want** to override or add model prices in a local JSON file, **so that** price changes or new models don't require a Peek release.

**As a** security-conscious user, **I want** Peek to never fetch pricing from the network, **so that** cost reporting introduces no supply-chain surface.
