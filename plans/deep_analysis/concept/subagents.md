# Subagent Content (~2–3d)

---

## Flows

### Discover and scan Claude subagents

1. When a session spawns a subagent, Claude Code writes `<project>/<sessionId>/subagents/agent-<id>.jsonl` plus `agent-<id>.meta.json` (`{agentType, description, toolUseId, spawnDepth}`); entries inside carry `isSidechain: true`, `agentId`, and the parent `sessionId`.
2. Backend
   1. The session watcher additionally watches `<project>/<sessionId>/subagents/` (directories appear mid-session — watch is registered when the parent session is first seen).
   2. `agent-<id>.meta.json` emits `SubagentEvent{kind: spawned, agentId, agentType, description, spawnDepth}` on the parent session.
   3. Subagent transcript lines run through the same signal extraction as the parent ([signals.md](signals.md)) — skill invocations and permission denials inside subagents emit events on the parent session with `actor: <agentId>`. Subagent conversational turns are NOT added to the parent turn buffer.
3. `session_events` shows subagent lifecycle and their signals interleaved with main-session events.

### Capture the subagent result (Claude)

1. The parent transcript contains the spawning `Agent` tool_use and later its tool_result — the exact content the main agent received back.
2. Backend
   1. Parser tracks pending `Agent` tool_use ids (same mechanism as `ExitPlanMode` tracking); the matching tool_result emits `SubagentEvent{kind: result, agentId (via meta toolUseId match), content}`.
   2. Persisted-output pointers resolve as in [signals.md](signals.md).
3. Analysis reads what was delegated (description) and what came back (result) without opening the subagent transcript.

### Codex subagent rollouts

1. Codex subagent rollouts are separate files whose `session_meta.source` carries `subagent.thread_spawn` with `parent_thread_id` — today the parser drops the whole rollout.
2. Backend
   1. Keep them out of `session_list` (parity decision stands), but instead of discarding, parse them in subagent mode: signal extraction only, events attached to the parent session resolved via `parent_thread_id`.
   2. Spawn meta (`agent_nickname`, depth) maps to the same `SubagentEvent{kind: spawned}` shape.
3. Parent Codex sessions gain subagent visibility equivalent to Claude's.

---

## Security Considerations

- Subagent transcripts are the same trust level as parent transcripts (local files written by the agent CLI) — no new exposure, but result content can be large: cap and truncate, never reject.
- `parent_thread_id` resolution must not create sessions: events for an unknown parent are buffered briefly, then dropped (a subagent whose parent is outside the ring buffer is not analysis-relevant).

---

## Limits

- Subagent result content: 32 KB stored per result (matches Codex plan cap; full text remains in the transcript).
- Subagent event scan shares the parent's 500-event ring — a runaway subagent cannot starve main-session events beyond ring semantics.
- Spawn depth > 1 (subagents of subagents): scanned identically; `spawnDepth` recorded, no special handling.

---

## Models

### SubagentEvent payloads (on session.Event)

**Public:**
- spawned: agentId, agentType/nickname, description, spawnDepth (object)
- result: agentId, content (truncated), is_error (object)

---

## Long-Tail Tasks

### Backlog

- Full subagent turn browsing (dedicated tool arg to page through a subagent's own turns, including internal thinking) (~2d).

### Testing

- Fixture from the real `subagents/` directory: spawn meta + transcript scan + parent-side result matching via `toolUseId`.
- Codex fixture: subagent rollout attaches events to parent, still absent from `session_list`.
- Mid-session directory creation: watcher picks up `subagents/` appearing after the parent session started.
