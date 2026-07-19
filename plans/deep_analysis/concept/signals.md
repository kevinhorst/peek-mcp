# Signal Extraction & Plan History (~3–5d)

---

## Flows

### Skill invocation (Claude)

1. Assistant message contains a `Skill` tool_use block with input `{skill, args}`; slash commands additionally appear as `<command-name>`/`<command-message>` tags inside user message text.
2. Backend
   1. `claude/parser.go` inspects `tool_use` blocks by name (allowlist) instead of dropping all non-text blocks; a `Skill` block emits `SkillEvent{skill, args, timestamp}`.
   2. User text is scanned for `<command-name>` tags; a match emits the same event kind with `source: slash`.
3. Event lands in the session's event buffer and increments `skills_invoked`.

### Plan approval / rejection (Claude)

1. Assistant calls `ExitPlanMode`; the following user entry carries the tool_result for that `tool_use_id`.
2. Backend
   1. Parser remembers pending `ExitPlanMode` tool_use ids (per file, same stateful pattern as the Codex parser's `sessionId`).
   2. Matching tool_result with `is_error: true` → `PlanEvent{kind: rejected}`; increment `plan_rejections`.
   3. Matching tool_result starting "User has approved your plan" → `PlanEvent{kind: approved}`. Large results arrive as a `<persisted-output>` pointer into `<session-dir>/tool-results/<tool_use_id>.txt` — resolve the pointer and read the file (bounded, see Limits) to keep the approved-plan content.
3. Existing `plan_mode` / `plan_mode_exit` / `plan_mode_reentry` attachments (already parsed) become `PlanEvent{kind: mode_enter | mode_exit | mode_reenter}` instead of plan-signal-only turns.

### Plan revision history (both agents)

1. Claude: `PlanWatcher` already tails `~/.claude/plans/*.md` and sees every intermediate version before the next overwrite. Codex: each later `<proposed_plan>` block is a full replacement of the previous one.
2. Backend
   1. First observed version is stored whole; every subsequent content change stores a unified diff against the previous version plus a timestamp (`PlanEvent{kind: revised}`).
   2. Claude revisions persist to the state dir (`<state>/<agent>/<session-id>/plan/`), because overwritten plan-file versions are the one plan artifact disk forgets. Codex revisions need no persistence — all blocks remain in the rollout; the parser stops applying last-wins and emits one `revised` event per block after the first.
   3. `plan_alterations` counts `revised` events that occur after the first `mode_exit` (Claude) or after the first `<proposed_plan>` block (Codex) — edits before the plan was ever presented are drafting, not alteration.
3. Rejection→revision→approval sequences are now countable: rejections from `ExitPlanMode` errors, alterations from revision events, final state from the last approval event.

### Permission denial and user answers (Claude)

1. Any tool_use answered by a tool_result with `is_error: true` and content starting "The user doesn't want to proceed with this tool use." is an explicit user denial. `AskUserQuestion` results carry the user's chosen options and free-text notes.
2. Backend
   1. Parser tracks tool_use name+id pairs per assistant message (bounded map, cleared per turn) so a denial can name the denied tool; emits `PermissionEvent{tool, kind: denied}`; increments `permission_denials`.
   2. `AskUserQuestion` tool_use + result emit `UserAnswerEvent{questions, answers}` — the explicit prompt/answer pairs requested for analysis.
3. Approvals stay implicit (a normal tool_result follows) and are intentionally not evented — only user-visible decisions are signals.

### Permission events (Codex)

1. Codex emits approval requests as `event_msg` payloads; shape unverified — no rollout on disk contains one (concept Open Question 1).
2. Backend
   1. Once verified: map approval request + verdict to `PermissionEvent` with the same fields; until then Codex sessions report zero permission events, flagged as `unsupported` rather than `0` in `session_events` output.

---

## Security Considerations

- `<persisted-output>` pointers are attacker-influenceable text inside tool results: resolve only paths under the session's own `<project>/<session-id>/tool-results/` directory; anything else is ignored.
- Plan revision diffs persisted to the state dir may contain source code — same file permissions as the diff snapshots (see [diff_retention.md](diff_retention.md)).
- Extracted event payloads (skill args, answers, plan text) are display-only data, never interpolated into commands.

---

## Limits

- Event buffer: ring per session, cap 500 events (analysis sessions rarely exceed low hundreds; ring matches `TurnBuffer` pattern).
- Persisted-output resolution: read cap 256 KB per file (approved plans observed at ~64 KB).
- Plan revisions: 50 per session, diff cap 64 KB each (beyond that: store truncation marker, keep counting).
- Pending tool_use tracking: per-turn map, cleared when the matching results arrive or the turn finalizes.

---

## Models

### session.Event

**Public:**
- kind: plan_approved | plan_rejected | plan_revised | plan_mode_enter | plan_mode_exit | permission_denied | user_answer | skill_invoked | subagent_spawned | subagent_result (string)
- timestamp: event time from the transcript entry (time)
- actor: main | subagent id (string)
- payload: kind-specific fields as listed in the flows (object)

### session.Counters (derived, computed from the event buffer)

**Public:**
- plan_rejections, plan_alterations, permission_denials, skills_invoked, subagents_spawned (int)

### Plan revision record (state dir, Claude only)

**Internal / Not Exported:**
- initial version: full content (file `000.md`)
- revision N: unified diff vs N-1 + timestamp (file `NNN.diff`)

---

## Long-Tail Tasks

### Testing

- Fixtures from the verified real transcript: Skill invocation, ExitPlanMode approval (with persisted-output pointer) and rejection, permission denial, AskUserQuestion answer.
- Plan revision fixture: three plan-file versions → initial + two diffs, `plan_alterations` counts only post-exit revisions.
- Codex fixture: multi-`proposed_plan` rollout → one `revised` event per later block, latest content still wins for `session_plan`.

### Verification

- Codex approval `event_msg` shape (Open Question 1) — capture a real approval rollout before designing the Codex mapping.
