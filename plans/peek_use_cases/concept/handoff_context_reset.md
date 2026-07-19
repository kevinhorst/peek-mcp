# Use Case: Compaction Preventer (Context Full → Reset)

Replace auto-compaction with a hard reset plus peek-based state recovery. Claim level: **optimal** — the successor session starts from verbatim recent turns, the actual plan file, and the actual git diff instead of a model-written summary.

---

## Flows

### Reset instead of compact

1. A Claude Code session approaches the context limit.
2. User ends the session (no compaction) and starts a fresh one in the same project.
3. Fresh session calls `session_full(agent: "claude", id: "<old-session-id>")` (or by title).
4. Backend
   1. peek returns the dead session's last N verbatim turns, its plan file content, and the current `git diff main`.
   2. Large sessions page at 100 KB for Claude clients; the fresh session follows `request_id` until `has_more` is false.
5. Fresh session resumes the plan's next unchecked step against the diff's actual state — zero re-explanation from the user.

### Pre-reset snapshot discipline

1. Before resetting, the user has the dying session state anything non-obvious in its final turn (open decisions, gotchas).
2. That final turn is exactly what `session_full` serves first to the successor — the transcript itself is the snapshot medium; no separate handoff file.

---

## Tools Used

| Tool | Params in this flow | Contribution |
|---|---|---|
| `session_full` | `id` or `title`, `agent: "claude"`, `n`, `request_id` | The complete recovery payload |
| `session_list` | `agent: "claude"` | Find the dead session's ID when not noted before reset |

---

## Claims & Evidence

| ID | Type | Must show | Proves |
|---|---|---|---|
| CR-1 | Video | Full loop: session at high context usage → reset → fresh session → `session_full` → first working turn continuing the plan | The workflow is real-time viable with zero re-explanation |
| CR-2 | Screenshot (still from CR-1) | The returned payload: verbatim final turns + plan + diff side by side | What "verbatim state" concretely means |

Capture instructions: show the context-usage indicator before reset; keep the video under ~90 s (trim the tool-call wait); the continuing turn must pick up a concrete plan step, not restate a summary.

---

## Comparison vs Alternatives

| | Auto-compaction | Reset + peek |
|---|---|---|
| Full history | Kept as lossy model-written summary | Dropped beyond ring buffer |
| Recent turns | Summarized | Verbatim |
| Plan | Summarized (may drift) | The actual plan file |
| Code state | Summarized | The actual `git diff` |
| Token cost at switch | Summarization pass in the dying session | One tool call in the fresh session |

Honest framing: compaction preserves *breadth* (all of history, lossy); reset + peek preserves *fidelity* (recent history plus authoritative artifacts, exact). For continuing implementation work, fidelity of plan and diff is what matters — old conversational turns rarely are.

---

## Limits

- `--depth` ring buffer (default 20 turns) bounds recoverable conversation; size it up (e.g. `--depth 100`) if long-tail turns matter.
- The plan is the durable memory. Sessions run without plan mode recover only turns + diff — the workflow is strongest with plan-driven work.
- Decisions that live only in mid-session turns older than the buffer are lost; the pre-reset snapshot turn (flow 2) is the mitigation.
