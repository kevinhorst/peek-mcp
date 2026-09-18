# Subagent parity sweep — Change Plan

route: `change`

## TLDR

- Parity sweep across all subagent ingestion paths (Claude flat, Claude workflow, Codex, cowork, restart) found the earlier fixes complete for usage, turns, and file touches — workflow file touches were dropped by the same unknown-parent bug and are already resolved.
- Four gaps remain in scope: workflow agents have no description (the run manifest's `label` is unparsed), no result events (`journal.jsonl` result records are unparsed), a flat-path bug hides result events from subagent-scoped views (`event.Actor` never backfilled), and file touches are only rolled up at parent level.
- Changes: manifest label lookup in `readSubagentMeta`, a journal reader in the watcher emitting `EventKindSubagentResult`, `Actor` backfill in `resolveSubagentActor`, and per-subagent `TouchedFiles` on `SubagentStat` surfaced in `subagentStatView`.
- Out by user cut: Codex subagent usage/turns un-suppression; topmost-ancestor hardening and `parentAgentId` chain stay deferred findings.

## Context

- Driver source: user asked whether workflow subagent file touches are resolved and for a full parity sweep across environments.
- Sweep verdict (verified, current branch): workflow agents now have usage, turns, parent-level touches, spawn events, agent_type, counters, and skill windows — parity with flat agents except description and result events. Codex subagents are deliberately suppressed to empty stat rows ([codex/parser.go:196-198](codex/parser.go:196), [:289-291](codex/parser.go:289)) — out of scope per user. Cowork admits subagent files; state persistence stores nothing subagent-derived (rebuilt by re-ingest, only the ingest horizon is lost).
- Data facts: journal `result` records are `{type:"result", key, agentId, result:<JSON>}`, no timestamp; the run manifest `<sess>/workflows/wf_<runId>.json` has `workflowProgress[]` entries `{agentId, label, agentType, model, ...}`; nested spawns (spawnDepth 2–3, 148 real metas) all write flat into `<sess>/subagents/` — no nested `subagents` dirs exist anywhere.
- Flat-path bug: [claude/parser.go:717](claude/parser.go:717) sets result `Actor` from the main-chain entry's empty `AgentId`; [resolveSubagentActor](session/store.go:610) backfills only `event.Subagent.AgentId`, while [filterEventsByActor](tools/tools.go:519) matches `event.Actor` — so a subagent-scoped view omits that agent's own result event.

## Drivers

| ID | Observed | Wanted | Impact | Origin |
| :- | :- | :- | :- | :- |
| DR1 | Workflow subagents render nameless — `Description` empty in stat views and the control usage table | Manifest `workflowProgress[].label` (e.g. `code-style-acdsl-core-l1`) as the description | behavioral | parity sweep + user scope selection |
| DR2 | Workflow agents emit no `subagent_result` event; their returned output never surfaces | Result events synthesized from journal `result` records, like `Agent`-tool results | behavioral | parity sweep + user scope selection |
| DR3 | Scoping `session_get`/`session_events` to a flat subagent omits its own result event | Result event visible under its subagent's actor scope | behavioral | parity sweep (code evidence above) |
| DR4 | File touches only aggregate on the parent session; a subagent's reads/writes are indistinguishable | Per-subagent touch counts on `SubagentStat`, parent rollup unchanged | behavioral | user scope selection |

## Scope

- **In**
  - **Manifest label enrichment**: `readSubagentMeta` reads the sibling run manifest for workflow metas; `Description` = matching `workflowProgress[].label`.
  - **Journal reader**: watcher-level reader for `subagents/workflows/wf_*/journal.jsonl` emitting `EventKindSubagentResult` turns; journal files excluded from the no-op claude-parser ingestion.
  - **Actor backfill**: `resolveSubagentActor` sets `event.Actor` alongside `Subagent.AgentId`, both directions.
  - **Per-subagent touches**: `SubagentStat.TouchedFiles` populated in `AddSubagentTurn` via a helper shared with `Session.AddFileTouch`; exposed as `touched_files` on `subagentStatView`.
- **Out (explicit non-goals)**
  - **Codex subagent usage/turns**: suppression stays ([codex/parser.go:196,289]) — user cut; they keep rendering as empty stat rows.
  - **Live manifest re-watching**: labels are read once at meta time; a label that lands in the manifest after the meta file is written stays empty for that live session (complete after any restart/backfill).
  - **Manifest rollups**: `totalTokens`/`status`/phase data unparsed.
- **Not changed**
  - **Parent-level touch rollup**, spawn-event pipeline, keep-last usage dedup, discovery predicates from the previous plan.
- **Deferred findings**
  - **Topmost vs nearest `subagents` ancestor**: identical on all real data (no nested `subagents` dirs); flip to topmost only if the layout ever nests.
  - **`parentAgentId` in meta files** (93/400 sampled): agent-to-agent chain not surfaced.
  - **Codex subagents appear as $0 rows** in the control usage table — consequence of the suppression left in place.

## Assumptions

| Assumption | Reality | Location |
| :- | :- | :- |
| Journal result records carry `agentId` and structured `result`, no timestamp | Verified via jq on real journals | `.../f00b23fa-.../subagents/workflows/wf_01a1e666-d15/journal.jsonl` |
| Manifest `workflowProgress[].agentId` matches the `agent-<id>` filename id | Verified (join key per earlier survey) | `.../f00b23fa-.../workflows/wf_01a1e666-d15.json` |
| Manifest lives at `<sess>/workflows/<wfDirBase>.json` where `wfDirBase` is the run dir name | Verified both sessions surveyed | same |
| `Event.Validate` requires only `Kind` | zero/coarse timestamps on synthesized events are legal | [session/event.go:59-69](session/event.go:59) |
| `filterEventsByActor` is exact-match on `Actor`, applied only when a `subagent` arg is given | setting `Actor` moves result events into that scope, unscoped views unchanged | [tools/tools.go:519-527](tools/tools.go:519), [:377-432](tools/tools.go:377) |

## Current state

- [watcher/watcher.go:253-311](watcher/watcher.go:253) `readSubagentMeta`: parses `subagentMeta{agentType, description, spawnDepth, toolUseId}`; workflow metas have no description/toolUseId.
- [watcher/watcher.go:181-191](watcher/watcher.go:181) deferred subagent pass: journal.jsonl currently goes through `readNewLines` + claude parser → every line fails `Turn.Validate` (no-op).
- [session/store.go:610-644](session/store.go:610) `resolveSubagentActor`: backfills `Subagent.AgentId` by `ToolUseId` pairing; never touches `Actor`.
- [session/session.go:125-144](session/session.go:125) `AddFileTouch` on session-level `TouchedFiles` with `maxTouchedFiles` cap; [store.go:180-182](session/store.go:180) forwards subagent touches there; `SubagentStat` has no touches field.
- [tools/viewmodels_events.go:185-217](tools/viewmodels_events.go:185) `subagentStatView`; [:63-80] `newTouchedFileViews` builds from the session map.
- [claude/parser.go:701-722](claude/parser.go:701) `subagentResultEvent`: payload `{Content (32KB cap), IsError, ToolUseId}`, `Actor` from `entry.AgentId`.

## Target state

- Workflow agents reach full signal parity with flat agents minus `ToolUseId` pairing (which has no data source): named, result-evented, actor-scopable, touch-attributed. Principle: one ingestion contract per signal regardless of spawn mechanism — the watcher owns Claude's on-disk layout knowledge (meta, journal, manifest all resolved via `subagentsRootDir`), the session model stays layout-agnostic.

## Behavior contract

- Unscoped `session_events`/`session_get` output: unchanged except new result events appearing in the stream and descriptions filling in.
- Parent-level `TouchedFiles` totals: byte-identical (per-subagent counts are additional, not moved).
- Flat-agent behavior: unchanged except DR3 — result events gain a non-empty `Actor` and become visible under their subagent's scope.
- Journal `started` records and non-result types: ignored.
- Manifest missing or unparsable at meta time: description stays empty, no error beyond a debug log — never blocks the spawn event.

## Decisions

| ID | Problem | Facts | Decision | Why |
| :- | :- | :- | :- | :- |
| D1 | Where to get workflow labels | manifest has per-agent `label`; meta does not; manifest is rewritten live | One-shot manifest read inside `readSubagentMeta` (path derived: meta dir base `wf_X` → `<root>/workflows/wf_X.json`), no manifest watching | Controllable and minimal: no new watch surface, complete on backfill (the dominant use); live gap accepted in Scope |
| D2 | Result-event source for workflow agents | journal has one `result` per agent with `agentId` + structured return; no per-agent parent tool_use | Dedicated `readJournal` in the watcher, offset-tracked via the existing `w.files` map, emitting `EventKindSubagentResult` with `Actor`/`AgentId` = agentId, `Content` = raw result JSON (32KB cap), empty `ToolUseId` | Reuses the proven line-reader pattern; setting AgentId directly makes `resolveSubagentActor` a no-op for these events |
| D3 | Timestamp for journal events | records carry none; `readSubagentMeta` precedent uses file ModTime | Journal file ModTime at read time | Debuggable ordering, matches the meta-event precedent; per-record accuracy has no data source |
| D4 | Journal parsing placement | watcher already owns Claude layout knowledge (meta files); parser routes by `Meta.SessionId` which journal records lack | Watcher-local reader + watcher-local 32KB cap constant mirroring `claude.maxSubagentResultBytes` | Parser cannot route without a session id; importing claude into watcher couples the generic watcher to one agent |
| D5 | DR3 fix shape | filter matches `Actor`; resolver already pairs by `ToolUseId` | In `resolveSubagentActor`, set `Actor` wherever `Subagent.AgentId` is set/backfilled (both branches) | Single resolver stays the one place actor identity is established |
| D6 | Per-subagent touch storage | session-level map + cap logic exists | `SubagentStat.TouchedFiles map[string]*FileTouchCounts`, filled in `AddSubagentTurn`; extract shared `addFileTouch(files, touch)` helper honoring `maxTouchedFiles`; parent rollup untouched | Single touch-accounting implementation; same cap avoids a second constant |
| D7 | Touch surface | `subagentStatView` is the per-agent telemetry view; `newTouchedFileViews` builds from a session | Generalize the view builder to take the map; add `touched_files` to `subagentStatView`; control UI unchanged | Tools surface is where per-agent telemetry is consumed; UI columns are a separate concern nobody asked for |

## Open questions

None — scope cut confirmed by user (labels + result events + per-subagent touches + DR3; Codex out).

## Baseline (verified)

N/A — change route; grounding lives in Current state / Assumptions.

## Exemplar & reuse

N/A — new-route section. Reuses: `readNewLines` offset pattern and `w.files` registry (journal reader), `readSubagentMeta` ModTime precedent, `AddFileTouch` cap logic, `newTouchedFileViews`/`touchedFileView`, `SubagentPayload` result shape.

## Changes

| File | Kind | Entry |
| :- | :- | :- |
| watcher/watcher.go | modified | Phase 1, 2 |
| session/store.go | modified | Phase 3 |
| session/session.go | modified | Phase 4 |
| tools/viewmodels_events.go | modified | Phase 4 |
| watcher/watcher_events_test.go | modified | Tests |
| session/store_events_test.go | modified | Tests |
| session/session_test.go | modified | Tests |
| tools/tools_test.go | modified | Tests (only if a view assertion exists to extend) |

### Phase 1 — manifest label enrichment (DR1, shippable alone)

`watcher/watcher.go`, inside `readSubagentMeta` after parsing `meta`, before building the payload:

```go
	description := meta.Description
	if description == "" {
		description = workflowAgentLabel(path, agentId, root)
	}
```

New helpers (mirror `readSubagentMeta`'s tolerant style — every failure returns ""):

```go
type workflowManifest struct {
	WorkflowProgress []struct {
		AgentId string `json:"agentId"`
		Label   string `json:"label"`
	} `json:"workflowProgress"`
}

func workflowAgentLabel(metaPath, agentId, root string) string {
	runDir := filepath.Base(filepath.Dir(metaPath))
	if !strings.HasPrefix(runDir, "wf_") {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(root, "workflows", runDir+".json"))
	if err != nil {
		return ""
	}
	var manifest workflowManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ""
	}
	for _, agent := range manifest.WorkflowProgress {
		if agent.AgentId == agentId {
			return agent.Label
		}
	}
	return ""
}
```

(`root` is the already-derived `subagentsRootDir` result; payload uses `Description: description`.)

### Phase 2 — journal reader (DR2, shippable alone)

`watcher/watcher.go`:
- Constant `journalFileName = "journal.jsonl"`, `maxJournalResultBytes = 32 * 1024` (mirrors `claude.maxSubagentResultBytes`).
- Predicate `isWorkflowJournalPath(path)`: basename == `journalFileName` && `subagentsRootDir(path) != ""`.
- Routing: in the deferred loop (`walkAndWatch`) and the live loop (`Run`), check `isWorkflowJournalPath` before the transcript branch and call `readJournal(path)` instead of `readNewLines`.
- `readJournal`: same lock/open/seek/complete-line loop as `readNewLines` (offset in `w.files`, nil parser), per line:

```go
type journalRecord struct {
	Type    string          `json:"type"`
	AgentId string          `json:"agentId"`
	Result  json.RawMessage `json:"result"`
}
```

For `Type == "result"` with non-empty `AgentId`: content = `string(record.Result)` truncated at the cap, then

```go
	event := &session.Event{
		Actor: record.AgentId,
		Kind:  session.EventKindSubagentResult,
		Subagent: &session.SubagentPayload{
			AgentId: record.AgentId,
			Content: content,
		},
		Timestamp: modTime,
	}
	turn := &session.Turn{
		Events:     []*session.Event{event},
		SubagentId: record.AgentId,
		Meta:       &session.Meta{SessionId: sessionId},
	}
```

routed via `w.store.AddTurnBySessionId` (sessionId from `subagentsRootDir`, modTime from `os.Stat` at read time per D3). Other record types skipped.

### Phase 3 — actor backfill (DR3, shippable alone)

`session/store.go` `resolveSubagentActor`:
- Result branch: on match, set both `event.Subagent.AgentId` and `event.Actor` to `seen.Subagent.AgentId`; also change the early return to backfill `event.Actor` from `event.Subagent.AgentId` when the payload already carries one but `Actor` is empty.
- Spawn branch: when backfilling `seen.Subagent.AgentId`, also set `seen.Actor`.

### Phase 4 — per-subagent touches (DR4, shippable alone)

`session/session.go`:
- `SubagentStat` gains `TouchedFiles map[string]*FileTouchCounts json:"-"`.
- Extract the body of `AddFileTouch` into `addFileTouch(files map[string]*FileTouchCounts, touch *FileTouch)` (cap `maxTouchedFiles` inside); `Session.AddFileTouch` delegates.
- `AddSubagentTurn`: loop `turn.FileTouches` into `stat.TouchedFiles` (lazily created).

`tools/viewmodels_events.go`:
- Generalize `newTouchedFileViews` to accept the map (session call site passes `currentSession.TouchedFiles`).
- `subagentStatView` gains `TouchedFiles []*touchedFileView json:"touched_files,omitempty"`, filled in `newSubagentStatViews`.

## Hot items

N/A — no hot-class implementations; no persistence, concurrency primitives (journal reader runs under the existing `w.mu` like its siblings), or migrations.

## Tests

| Location.Method | Cases | Comment |
| :- | :- | :- |
| watcher/watcher_events_test.go `TestReadSubagentMeta` (extend workflow subtest) | manifest present → Description = label; manifest absent → empty; meta-provided description wins | fixture writes `<sess>/workflows/wf_x.json` next to the existing workflow-meta helper |
| watcher/watcher_events_test.go `TestReadJournal` | result record → `EventKindSubagentResult` on parent with Actor/AgentId/Content; `started` record ignored; incremental second read emits only new lines | mirrors readNewLines-style tests |
| watcher/watcher_events_test.go `TestWalkAndWatch_ColdBackfillWorkflowSubagents` (extend) | journal file present → result event exists after backfill | pins routing exclusion from readNewLines |
| session/store_events_test.go `resolveSubagentActor` cases (extend) | result-after-spawn: event.Actor set; spawn-after-result: seen result's Actor set; payload-carried AgentId with empty Actor: Actor backfilled | DR3 regression |
| session/session_test.go `TestSession_AddSubagentTurn_TouchedFiles` | touches land on stat map with read/write counts; parent rollup unaffected assertion lives in store test | DR4 |
| tools tests | `subagentStatView.touched_files` populated — extend existing stat-view assertion if one exists, else covered by struct wiring | keep minimal |

Existing safety net: all watcher/store/session/tools/control suites green (behavior contract).

Not tested: live fsnotify journal delivery (same dir-watch mechanics as agent files, covered by routing test + manual verification).

## Test runbook

Scenario index:
- **workflow-labels** — restart peek-mcp; control usage subagents table for session `f00b23fa-…`: Description column shows lane labels (`setup:sonnet-5-medium-r2`, `code-style-…`).
- **workflow-results** — `session_events` for the same session with `breakdown=subagent_results`: journal-derived results present; scoping to one workflow agent id returns its result event.
- **flat-actor-scope** — `session_get` scoped to a flat Agent subagent of any recent session: its `subagent_result` event now included.
- **touches** — `session_events` subagents view: `touched_files` per agent non-empty for agents that edited files; parent `touched_files` totals unchanged vs pre-change.

## Contracts & sweeps

| Contract | Sides | Sweep |
| :- | :- | :- |
| `EventKindSubagentResult` payload shape | claude parser, journal reader, resolver, viewmodels | journal events carry AgentId but no ToolUseId — grep `ToolUseId` consumers for empty-tolerance (resolver early-returns on non-empty AgentId; summaries fall back to Content) |
| `touchedFileView` builder signature | tools viewmodels call sites | grep `newTouchedFileViews` → all call sites updated |
| `Actor` semantics on result events | filterEventsByActor, control event rendering | flat result events move from empty-actor to agent-actor scope — confirm no surface renders "main-chain results" by empty Actor (grep `EventKindSubagentResult` in tools/ and control/) |

## Verification

- [ ] `go build ./...` and full `go test ./...` green including new cases
- [ ] Run against real `~/.claude`: control usage table for `f00b23fa-…` shows labels in the Description column
- [ ] `session_events` for `f00b23fa-…`: result events present for workflow agents; count matches `jq 'select(.type=="result")' | wc -l` over that session's journals
- [ ] Flat session: subagent-scoped `session_get` includes the result event (DR3)
- [ ] Per-agent `touched_files` non-empty for a workflow agent that edited files; parent totals unchanged vs pre-change output

## Stop conditions

| ID | Condition | Action |
| :- | :- | :- |
| S1 | A planned edit requires touching a file outside the Changes table | stop, ask |
| S2 | An assumption row proves false against repo/data | stop, report |
| S3 | A test can only pass by weakening an existing assertion | stop, ask |
| S4 | The change grows a new concept not in this plan | stop, ask |
| S5 | Flat-agent regression tests fail | stop, diagnose before touching tests |
| S6 | The Actor change surfaces a consumer that relied on empty-actor result events | stop, report before adapting |

## Changelog

| Date | Trigger | What changed |
| :- | :- | :- |
| 2026-09-14 | initial | plan created (supersedes the implemented workflow-token plan in this file) |
| 2026-09-18 | persistence | plan persisted to plans/agent_parity/design/; all four phases found already implemented and tested on main (v1.2.6: workflowAgentLabel, readJournal, resolveSubagentActor Actor backfill, SubagentStat.TouchedFiles + SubagentStatView.touched_files) |
