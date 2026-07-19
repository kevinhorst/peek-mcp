# Use Case: Cross-Agent Communication

Claude Code and Codex read each other's state — both directions — because peek watches both transcript trees. Claim level: **usable**, honestly scoped: this is shared-state observation, not message passing. There is no push channel and no inbox; "communication" means both agents observing the same durable artifacts (turns, plans, diffs).

---

## Flows

### Claude reads a Codex plan

1. A Codex session produces a plan in plan mode (its latest `<proposed_plan>` block — Codex writes no plan file; peek extracts the block).
2. A Claude Code session calls `session_plan(agent: "codex")`.
3. Backend
   1. peek resolves the most recent Codex session and returns the extracted plan content; later blocks fully replace earlier ones, so the current plan wins.
4. Claude works against the same plan — no copy-paste, no drift between two plan copies.

### Codex reviews Claude's work

1. A Claude session builds a feature (the README "Example workflow", promoted here).
2. A Codex session calls `session_get(agent: "claude", n: 10)` for the turns and `session_diff(agent: "claude")` for the changes vs `main`.
3. Codex reviews and reports findings in its own session; the user relays actionable items back to Claude (or lets Claude read Codex's turns the same way).

### Addressing across agents

1. When several sessions are live, either side targets the counterpart by title: exact case-insensitive match first, substring fallback, scoped by `agent`. Claude titles come from explicit custom titles; Codex titles from its session index (`thread_name`).

---

## Security Considerations

- Transcripts contain whatever was pasted into sessions — including secrets. Every client connected to peek can read every watched session. Mitigation is deployment-side: peek binds to localhost by default and reads only local files the user already owns; don't expose the HTTP port beyond the machine.
- No write path exists: peek cannot inject content into any session, which also bounds the blast radius of a misbehaving connected client to reads.

---

## Tools Used

| Tool | Params in this flow | Contribution |
|---|---|---|
| `session_plan` | `agent: "codex"` / `"claude"`, optional `title` | Read the counterpart's current plan |
| `session_get` | `agent`, `title`/`id`, `n` | Read the counterpart's turns |
| `session_diff` | `agent`, `title`/`id` | Read the counterpart's changes vs target branch |

---

## Claims & Evidence

| ID | Type | Must show | Proves |
|---|---|---|---|
| CA-1 | Screenshot | Claude session displaying a Codex `<proposed_plan>` retrieved via `session_plan(agent: "codex")` | Direction Codex → Claude works, including plan extraction |
| CA-2 | Screenshot | Codex session displaying Claude turns + diff via `session_get`/`session_diff` | Direction Claude → Codex works |

Capture instructions: use two genuinely different tasks so the screenshots are visibly cross-session; include the tool-call line and the payload in frame; CA-1 must show recognizable `<proposed_plan>` content rendered as plan output.

---

## Comparison vs Alternatives

| Alternative | What it costs |
|---|---|
| Copy-paste between terminals | Manual, lossy, skips diffs; drifts the moment either side continues |
| Shared scratch files both agents are prompted to maintain | Cooperation-dependent; agents forget to update; another artifact to manage |
| One agent reads the other's transcript directory raw | Works but slow and token-heavy; both JSONL formats differ and change |

---

## Limits

- Read-only, pull-based: an agent sees counterpart updates only when it calls a tool again; there is no notification mechanism.
- Turn history bounded by the `--depth` ring buffer; tool calls and results are filtered from turns by design.
