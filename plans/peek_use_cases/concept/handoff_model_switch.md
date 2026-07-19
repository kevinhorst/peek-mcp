# Use Case: Model-Switch Handoff (Fable → GPT)

Continue a Claude Code session in a Codex/GPT session without manually transferring context. Claim level: **usable** — faster and more faithful than any manual alternative.

---

## Flows

### Hand a running Claude task to GPT

1. Claude Code (frontier model) works on a task; peek-mcp watches its transcript passively.
2. User opens a fresh Codex session and prompts: "Continue the current Claude session. Use peek's `session_full` to load it."
3. Codex calls `session_full(agent: "claude")` — no `id`/`title` needed, the most recently active Claude session resolves automatically.
4. Backend
   1. peek returns the last N turns (default 20), the plan-mode plan file content, and the pre-computed `git diff main` in one response.
   2. Codex clients get the full response unpaginated (`MaxResponseBytesCodex = 0`); Claude-family clients would get 100 KB pages with `has_more`/`request_id` continuation.
5. Codex continues the task, referencing the plan's remaining steps and the diff's current state.

### Targeted handoff by title

1. Same as above, but the source session is addressed explicitly: `session_full(title: "…", agent: "claude")` — exact case-insensitive match first, substring fallback.

---

## Tools Used

| Tool | Params in this flow | Contribution |
|---|---|---|
| `session_full` | `agent: "claude"`, optional `title`, `n`, `request_id` | Turns + plan + diff in one call — the entire handoff |
| `session_list` | `agent: "claude"` | Optional: pick the right source session when several are active |

---

## Claims & Evidence

| ID | Type | Must show | Proves |
|---|---|---|---|
| MS-1 | Screenshot | Claude Code session mid-task (plan visible, some turns done) | There is real prior state to hand over |
| MS-2 | Screenshot | Codex session: the `session_full` call and the returned turns/plan/diff payload | One call transfers everything |
| MS-3 | Screenshot | Codex's next working turn referencing a plan step and a file from the diff by name | The receiving model *acts* on the state, doesn't just receive text |

Capture instructions: MS-1→MS-3 come from one continuous run; the task should be small but non-trivial (a real repo, a real plan) so MS-3 can show concrete plan/diff references. Crop terminal to relevant output; no synthetic/staged payloads.

---

## Comparison vs Alternatives

| Alternative | What it costs |
|---|---|
| Copy-paste context by hand | Minutes of curation per handoff; diff and plan usually skipped |
| Re-prompt GPT from scratch | Loses the plan and all decisions; the new model re-derives (or contradicts) prior work |
| Prompting GPT to read `~/.claude/projects/.../*.jsonl` directly | Works, but slow and token-heavy: raw JSONL with tool noise and sidechains, format subject to change |

---

## Limits

- `n` default 20 turns; older turns beyond the `--depth` ring buffer (default 20) are gone — plan and diff carry the durable state.
- Both CLIs must have peek-mcp connected; the source agent needs no configuration (transcripts are watched passively).
- `session_diff` needs the session's working dir to be a git repo with the target branch present.
