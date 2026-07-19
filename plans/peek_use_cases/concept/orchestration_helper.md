# Use Case: Agent Orchestration Helper

One supervisor session observes a fleet of worker sessions — inventory, progress, output review — with zero worker-side integration. Claim level: **usable**. Boundary stated up front: peek observes; it does not dispatch, schedule, or control workers. It is the observation layer under whatever dispatch mechanism is already in use (worktrees, terminal tabs, task runners).

---

## Flows

### Fleet inventory

1. Several Claude Code / Codex sessions run in parallel (e.g. in separate git worktrees).
2. A supervisor session calls `session_list` (no params — all agents).
3. Backend
   1. peek returns every known session: ID, agent, title, title source, last-active timestamp, `HasPlan`/`HasDiff`, and metadata (cwd, git branch, model, origin).
   2. Sub-agent sessions (Claude sidechains, Codex sub-agent rollouts) are filtered out — the list shows top-level work only.
4. Supervisor sees at a glance which workers are active, on which branch, with which model.

### Progress check on one worker

1. Supervisor calls `session_get(id: "<worker>", n: 5)` for the worker's recent turns.
2. `session_plan(id: "<worker>")` shows how far the plan has progressed.
3. Neither call interrupts the worker — reads come from peek's in-memory buffer, not from the agent.

### Output review

1. Worker reports done (or goes quiet).
2. Supervisor calls `session_diff(id: "<worker>")` — the pre-computed diff vs `main`, refreshed on each turn, resolved in the worker's own working dir (worktree-correct).
3. Supervisor reviews, then feeds verdicts back through its own channel (a follow-up prompt to the worker, a PR comment — outside peek's scope).

---

## Tools Used

| Tool | Params in this flow | Contribution |
|---|---|---|
| `session_list` | none / `agent` | Fleet inventory with metadata |
| `session_get` | `id`, `n` | Worker progress drill-down |
| `session_plan` | `id` | Plan progress without reading turns |
| `session_diff` | `id` | Worker output review vs target branch |
| `session_uncommitted_diff` | `id` | Backlog variant: live view of a worker mid-edit (`git diff HEAD`, polled) |

---

## Claims & Evidence

| ID | Type | Must show | Proves |
|---|---|---|---|
| OH-1 | Screenshot | `session_list` output with 3+ concurrently active sessions on different branches/models | Fleet visibility is real, not a single-session toy |
| OH-2 | Screenshot | Drill-down on one worker: `session_get` turns plus its `session_diff` | Supervisor can review output without touching the worker's terminal |

Capture instructions: run at least three real sessions (mix of Claude and Codex if possible) in separate worktrees so branches differ visibly; OH-1 and OH-2 from the same fleet state.

---

## Comparison vs Alternatives

| Alternative | What it costs |
|---|---|
| Switching terminal tabs / tmux panes | Human polling; no structured metadata; doesn't scale past a few workers |
| Workers push status to a shared file/channel | Requires per-worker prompt engineering and cooperation; workers forget |
| Orchestration frameworks with built-in telemetry | Buy into a whole dispatch framework; peek works with plain parallel CLI sessions |

The zero-integration property is the point: workers need no MCP config, no hooks, no prompt additions — they are observable because they write transcripts anyway.

---

## Limits

- Observation only — no dispatch, no lifecycle control, no messaging to workers.
- `session_list` shows sessions peek has seen since its own start (transcripts parsed from disk); freshness of turn data is bounded by fsnotify latency (effectively immediate on save).
- Per-worker turn history bounded by the `--depth` ring buffer.
