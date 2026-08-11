# Railroad findings re-verification & fix — Change Plan

## TLDR

- All 7 findings from the 2026-07-26 railroad review were re-verified against the current tree (`4a7eaae`, ~30 commits past the reviewed head `e4f69fb`): **all 7 are still valid** — none were fixed by the intervening merges.
- correctness-01 (BLOCKER) got worse: the double-counting `CurrentUsage` gained two new consumers via the control server (stats API + sessions page), so the doubled figure now also renders on the dashboard.
- One review reference went stale: `session_full` was consolidated into `session_get` (commit `50cf1e4`), so code-style-03's sibling target is now `sessionGetResultPage`, and intentional-deviation 1's comparison baseline is `session_get`'s string-typed `Turns`.
- This plan fixes all 7 in four independently shippable phases: usage fix → subagent backfill → uniform events shape (+docs) → hygiene (untrack sentinel, field order, error prefixes).

## Context

The deep_analysis branch review (railroad, 3 tracks) produced 3 correctness findings and 4 style findings, recommendation REJECT. Since then the branch absorbed the control server, telemetry (OTLP), session_get flat-flag consolidation, and subagent/skill/time analytics. The user asked to re-check every finding against the current tree and produce an updated plan. Originating plan: `plans/deep_analysis/design/raw.md`; the review record arrived as pasted notes (no `[USER]` decision conflicts — the review's three "intentional deviations" are canonicalized, not fixed, per its own disposition).

### Re-verification result (2026-08-11, head `4a7eaae`)

| Finding | Status | Evidence in current tree |
|---------|--------|--------------------------|
| correctness-01 BLOCKER | **VALID, blast radius widened** | `CurrentUsage` ([session/session.go:270](session/session.go:270)) still re-adds `TurnActive.Usage` on top of the RequestId-deduped fold in `AddTurn` ([session/session.go:235](session/session.go:235)). Test still hand-builds state ([session/session_events_test.go:52](session/session_events_test.go:52)). New consumers: [control/sessions.go:137](control/sessions.go:137), [control/api.go:207](control/api.go:207), plus [tools/tools.go:203](tools/tools.go:203), [tools/tools.go:315](tools/tools.go:315). Codex is unaffected: its usage arrives only as usage-signal turns (`Role==""`), assigned via `session.TotalUsage = *turn.Usage` ([session/store.go:128](session/store.go:128)) and returned before `AddTurn`, so a Codex `TurnActive` never carries usage. |
| correctness-02 WARNING | **VALID, unchanged** | `resolveSubagentActor` (moved to [session/store.go:523](session/store.go:523)) still joins only at result-append time. `walkAndWatch` still defers subagent paths to a second pass ([watcher/watcher.go:152](watcher/watcher.go:152)); `readSubagentMeta` is the sole Claude producer of `SubagentSpawned` ([watcher/watcher.go:262](watcher/watcher.go:262)). On restart the parent transcript's `subagent_result` appends first → `agent_id` stays empty in the serialized event stream (`marshalEvents` serializes raw `session.Event` incl. `Subagent.AgentId`). The new `breakdown` subagent view is independent (keyed by `SubagentId`) and unaffected. |
| correctness-03 WARNING | **VALID, unchanged** | `buildEvents` ([tools/pages.go:144](tools/pages.go:144)): single page → `rawJsonSegment` keeps the valid array; multi-page byte-slices fail `json.Valid` → JSON-quoted strings. `surface.md` still promises "ordered event list … (array)" ([plans/deep_analysis/concept/surface.md:66](plans/deep_analysis/concept/surface.md:66)). |
| code-style-01 WARNING | **VALID** | `.claude-worktree` still tracked (`git ls-files` hit), still hardcodes an absolute path + the unrelated branch `claude/merge-conflict-replay-a55b3a`. `.gitignore`'s `.claude/**` does not match the root-level file. |
| code-style-02 NIT | **VALID** | `usageRequestIds` trails all exported fields ([session/session.go:72](session/session.go:72)); the private group leads at lines 43–45 (`activeSkill`, `currentPromptId`, `planExitSeen`). |
| code-style-03 NIT | **VALID, sibling renamed** | `sessionEventsResultPage` lists `RequestId` before `HasMore` ([tools/viewmodels_events.go:92](tools/viewmodels_events.go:92)). The review's sibling `sessionFullResultPage` no longer exists — `session_full` was consolidated into `session_get`; the same order lives in `sessionGetResultPage` ([tools/viewmodels.go:20](tools/viewmodels.go:20)). |
| code-style-04 NIT | **VALID** | All six `Session.Validate` errors lowercase, unprefixed ([session/session.go:303](session/session.go:303)); sibling `Turn.Validate` uses the `Turn.Validate:` prefix throughout. |

Intentional deviations 2 and 3 remain canonical (broker in place, param orders unchanged). Deviation 1 (RawMessage page type) is superseded by D3's decision below — the string type resolves both the deviation and the shape bug with one mechanism.

## Drivers

| ID | Observed | Wanted | Impact | Origin |
|----|----------|--------|--------|--------|
| D1 | `total_usage`/`usage` double-count the active Claude turn's request in `session_get`, `session_events`, control stats API, and dashboard | Each request counted once, per the design's "correct per-session token totals" | behavioral | correctness-01 (railroad review) |
| D2 | `subagent_result` events replayed on restart carry empty `agent_id` (spawn meta arrives in the deferred second walk pass) | `agent_id` resolved regardless of append order | behavioral | correctness-02 (railroad review) |
| D3 | `session_events` `events`/`revisions` are a JSON array on single-page responses but a JSON-quoted string chunk on multi-page responses | One uniform shape, documented, matching the `session_get` pagination contract | contract-touching | correctness-03 (railroad review) |
| D4 | Machine-generated `.claude-worktree` sentinel tracked in HEAD | Untracked and ignored | behavior-preserving | code-style-01 (railroad review) |
| D5 | `usageRequestIds` trails exported struct fields | Grouped with the leading private fields | behavior-preserving | code-style-02 (railroad review) |
| D6 | `RequestId` before `HasMore` in both page structs (non-alphabetical) | Alphabetical order in `sessionEventsResultPage` and `sessionGetResultPage` | behavior-preserving | code-style-03 (railroad review) |
| D7 | `Session.Validate` errors lowercase without component prefix (6 sites) | `Session.Validate:` prefix, capitalized, matching sibling `Turn.Validate` | behavior-preserving | code-style-04 (railroad review) |

## Scope

- In
  - The 7 drivers above, each fixed at its named location plus the tests and docs that pin it.
- Out
  - The two "context, not findings" security notes (unauthenticated localhost surface, prompt-injection via transcripts) — explicitly de-scoped by the review.
  - Intentional deviations 2–3 (broker, param orders) — canonical, no change.
- Not changed
  - `AddTurn`'s eager RequestId-deduped fold — it stays the single usage mechanism; only the `CurrentUsage` re-add is removed.
  - The Codex usage path (`IsUsageSignal` assignment at [session/store.go:128](session/store.go:128)).
  - `EventEntry`/compact event summaries on `session_get`.
- Deferred findings
  - The review's design-doc reconciliation for deviation 2 (broker vs. planned channel in `raw.md`) — documentation-only, not blocking, can ride along with any later doc pass.

## Assumptions

| Assumption | Reality | Location |
|------------|---------|----------|
| Review: fix targets `session/session.go:125` etc. | Lines moved (~30 commits of merges); all re-anchored in the re-verification table above | Context section |
| Review: mirror `session_full`'s string-typed `Events` | `session_full` is gone (commit `50cf1e4`); the string-typed sibling contract is `session_get`'s `Turns`/`Events` string fields | [tools/viewmodels.go](tools/viewmodels.go) |
| Review: Claude usage always carries a RequestId | Confirmed: only `claude/parser.go` sets `RequestId`; Codex usage never reaches `AddTurn` | [claude/parser.go:170](claude/parser.go:170), [session/store.go:128](session/store.go:128) |

## Current state

| File | Lines | Responsibility | Defect |
|------|-------|----------------|--------|
| [session/session.go](session/session.go) | 342 | Session aggregate: turns, usage, events, analytics | `CurrentUsage` re-add (270–276); field order (72); Validate messages (303–329) |
| [session/store.go](session/store.go) | ~560 | Store: turn routing, event append, subagent join | one-directional `resolveSubagentActor` (523–545) |
| [watcher/watcher.go](watcher/watcher.go) | ~300 | FS walk + watch, subagent meta | deferred second pass (152–162) makes result-before-spawn the *normal* restart order |
| [tools/pages.go](tools/pages.go) | ~210 | Response pagination | `buildEvents` shape flip (144–177), `rawJsonSegment` (179–192) |
| [tools/viewmodels_events.go](tools/viewmodels_events.go) | 271 | session_events view models | `json.RawMessage` fields (44, 46); page field order (90–94) |
| [.claude-worktree](.claude-worktree) | 4 | harness sentinel | tracked in HEAD |

## Target state

`N/A — no structural change; every fix is in-place at the files above.` Principle per fix: **single source of truth** for usage (the `AddTurn` fold is the only counting site; `CurrentUsage` becomes a copy-returning accessor); **order-independent join** for the subagent actor (the join runs from whichever side appends second); **one serialization contract** for paged payloads (string segments, same as `session_get`).

## Behavior contract

- Must not change
  - Codex usage totals (assignment path untouched).
  - Claude `TotalUsage` accumulation and RequestId dedup (`TestSession_AddTurn_UsageDedupByRequestId` pins it).
  - Live-tail subagent resolution (spawn-before-result order keeps working).
  - Single-page `session_events` consumers that `json.Unmarshal` the whole response into loose types — but see D3: the `events` field's *type* changes from array to string by decision.
- Intentional changes (map to drivers)
  - D1: `usage`/`total_usage` values drop by exactly the active turn's request on affected Claude sessions.
  - D2: `agent_id` now populated on replayed `subagent_result` events.
  - D3: `events`/`revisions` are always JSON strings (breaking for array-parsing consumers on the single-page path; previously breaking on the multi-page path — the contract is now honest and uniform).
  - D7: `Session.Validate` error strings change (message-matching callers would break; none exist — errors are only surfaced, never matched).

## Decisions

| ID | Problem | Facts | Decision | Why |
|----|---------|-------|----------|-----|
| DEC-1 | Empty-`RequestId` Claude turn with usage: `AddTurn` skips the fold, and after D1 `CurrentUsage` no longer compensates | `AddTurn` guard at [session/session.go:235](session/session.go:235); Claude parser always sets `RequestId` from the entry; dedup test's `provideUsageTurn("", 40)` case expects the skip | Such turns stay uncounted — the skip is the deliberate contract, consistent in both methods | A request without an id cannot be deduped; counting it re-opens double-count risk on retries. Anomalous by design per the review. |
| DEC-2 | Where to resolve the subagent actor order-independently | Producers: parser (result, inline) + watcher meta (spawn, deferred). Buffer capped at 500 events | Bidirectional join in `resolveSubagentActor`: result-append joins backward (existing), spawn-append backfills forward over buffered results | Runs at append time only (no read-time O(n²)), no store/watcher coupling, no new concept; symmetric with the existing mechanism. |
| DEC-3 | Uniform `events`/`revisions` shape: entry-boundary pagination vs. always-string segments | A single plan-revision diff can exceed a page budget, so entry-boundary paging cannot guarantee fit; `session_get` already ships string-typed chunked fields (`Turns`, `Events`) | `Events`/`Revisions` become `string`; pages carry reassemblable string segments; `rawJsonSegment` is deleted | One contract for all paged payloads in the codebase; the impossible-fit edge disappears. Supersedes intentional-deviation 1 (RawMessage) — the review kept RawMessage only for lack of a uniform alternative. |
| DEC-4 | Disposal of `rawJsonSegment` | Only caller is `buildEvents` | Deleted in phase 3 | Dead after DEC-3; no parallel mechanisms survive. |
| DEC-5 | D7 scope: new error only vs. whole method | Review proposed fix says "across the whole method"; sibling `Turn.Validate` is fully prefixed | Prefix + capitalize all six `Session.Validate` errors | Half-converted method would violate the same finding again. |

## Changes

### Phase 1 — D1: single-count usage

`CurrentUsage` becomes a copy accessor ([session/session.go:270](session/session.go:270)):

```diff
 func (s *Session) CurrentUsage() *Usage {
 	total := s.TotalUsage
-	if s.TurnActive != nil {
-		total.Add(s.TurnActive.Usage)
-	}
 	return &total
 }
```

Rewrite the test to drive state through `AddTurn` ([session/session_events_test.go:52](session/session_events_test.go:52)), mirrors: `TestSession_AddTurn_UsageDedupByRequestId` ([session/session_test.go:125](session/session_test.go:125)):

```diff
 func TestSession_CurrentUsage(t *testing.T) {
-	// totals-plus-active-turn
-	t.Run("totals-plus-active-turn", func(t *testing.T) {
+	// active-turn-counted-once
+	t.Run("active-turn-counted-once", func(t *testing.T) {
 		s := provideCompleteSession()
-		s.TotalUsage = Usage{InputTokens: 100, OutputTokens: 40}
-		s.TurnActive = &Turn{Usage: &Usage{InputTokens: 10, OutputTokens: 5}}
+		s.AddTurn(provideUsageTurn("req-a", 10))
+		s.AddTurn(provideUsageTurn("req-b", 20))
 
 		usage := s.CurrentUsage()
-		assert.Equal(t, 110, usage.InputTokens)
-		assert.Equal(t, 45, usage.OutputTokens)
+		assert.NotNil(t, s.TurnActive)
+		assert.Equal(t, 30, usage.OutputTokens)
 		assert.Equal(t, 100, s.TotalUsage.InputTokens, "TotalUsage must not be mutated")
 	})
```

(Adjust the `InputTokens` assertion to whatever `provideUsageTurn` sets — the dedup test shows 1 per unique request; the mutation-guard assertion stays, re-based on the driven value.)

### Phase 2 — D2: order-independent subagent join

Bidirectional `resolveSubagentActor` ([session/store.go:523](session/store.go:523)):

```diff
 func resolveSubagentActor(event *Event, session *Session) {
-	if event.Kind != EventKindSubagentResult {
+	if event.Subagent == nil {
 		return
 	}
-	if event.Subagent == nil || event.Subagent.AgentId != "" {
-		return
-	}
 
-	for _, seen := range session.Events.All() {
-		if seen.Kind != EventKindSubagentSpawned {
-			continue
-		}
-		if seen.Subagent == nil {
-			continue
-		}
-		if seen.Subagent.ToolUseId != event.Subagent.ToolUseId {
-			continue
-		}
+	switch event.Kind {
+	case EventKindSubagentResult:
+		if event.Subagent.AgentId != "" {
+			return
+		}
+		for _, seen := range session.Events.All() {
+			if seen.Kind != EventKindSubagentSpawned || seen.Subagent == nil {
+				continue
+			}
+			if seen.Subagent.ToolUseId != event.Subagent.ToolUseId {
+				continue
+			}
+			event.Subagent.AgentId = seen.Subagent.AgentId
+			return
+		}
+	case EventKindSubagentSpawned:
+		if event.Subagent.AgentId == "" {
+			return
+		}
+		for _, seen := range session.Events.All() {
+			if seen.Kind != EventKindSubagentResult || seen.Subagent == nil {
+				continue
+			}
+			if seen.Subagent.AgentId != "" || seen.Subagent.ToolUseId != event.Subagent.ToolUseId {
+				continue
+			}
+			seen.Subagent.AgentId = event.Subagent.AgentId
+		}
+	}
-
-		event.Subagent.AgentId = seen.Subagent.AgentId
-		return
-	}
 }
```

Add a result-before-spawn ordering test next to the existing resolution test in [session/store_test.go](session/store_test.go) (mirrors: the existing spawn-before-result case): append a `subagent_result` with `ToolUseId: "tu-1"` and empty `AgentId`, then a `subagent_spawned` with the same `ToolUseId` and `AgentId: "agent-1"`, assert the buffered result event now carries `agent_id`.

### Phase 3 — D3: uniform string-typed events pages

`sessionEventsResult` fields become strings ([tools/viewmodels_events.go:44](tools/viewmodels_events.go:44)):

```diff
 type sessionEventsResult struct {
 	Counters      *session.Counters   `json:"counters,omitempty"`
 	Diff          string              `json:"diff,omitempty"`
-	Events        json.RawMessage     `json:"events,omitempty"`
+	Events        string              `json:"events,omitempty"`
 	PlanRevisions *planRevisionsView  `json:"plan_revisions,omitempty"`
-	Revisions     json.RawMessage     `json:"revisions,omitempty"`
+	Revisions     string              `json:"revisions,omitempty"`
```

`buildEvents` assigns chunks directly; `rawJsonSegment` deleted ([tools/pages.go:144](tools/pages.go:144)):

```diff
 	if b.Size <= 0 || contentSize <= b.Size {
 		first = &sessionEventsResult{
-			Events:    rawJsonSegment(events),
-			Revisions: rawJsonSegment(revisions),
+			Events:    events,
+			Revisions: revisions,
 		}
 		return first, nil
 	}
```

```diff
 		eventChunk := UTF8SafeSlice(events, size)
-		page.Events = rawJsonSegment(eventChunk)
+		page.Events = eventChunk
 		events = events[len(eventChunk):]
```

```diff
 		revisionChunk := UTF8SafeSlice(revisions, size)
-		page.Revisions = rawJsonSegment(revisionChunk)
+		page.Revisions = revisionChunk
 		revisions = revisions[len(revisionChunk):]
```

```diff
-func rawJsonSegment(segment string) json.RawMessage {
-	if segment == "" {
-		return nil
-	}
-	if json.Valid([]byte(segment)) {
-		return json.RawMessage(segment)
-	}
-
-	quoted, err := json.Marshal(segment)
-	if err != nil {
-		return nil
-	}
-	return quoted
-}
```

Drop the now-unused `encoding/json` import from `pages.go` if nothing else uses it. Update the `buildEvents` cases in [tools/pages_test.go:94](tools/pages_test.go:94) to assert string fields and full reassembly across pages. Docs: state in the `session_events` section of [docs/tools.md:34](docs/tools.md:34) that `events`/`revisions` are JSON-encoded strings, chunked across pages, concatenate then parse; amend [plans/deep_analysis/concept/surface.md:66](plans/deep_analysis/concept/surface.md:66) from "(array)" to the string contract.

### Phase 4 — D4–D7: hygiene

- D4: `git rm --cached .claude-worktree`; append `.claude-worktree` to [.gitignore](.gitignore).
- D5 ([session/session.go:72](session/session.go:72)):

```diff
 	activeSkill     *SkillStat
 	currentPromptId string
 	planExitSeen    bool
+	usageRequestIds map[string]struct{}
 
 	Agent           Agent                       `json:"agent"`
```

```diff
 	TurnsFinished   *TurnBuffer
 	UncommittedDiff string `json:"-"`
-	usageRequestIds map[string]struct{}
 }
```

- D6: swap `RequestId`/`HasMore` in `sessionEventsResultPage` ([tools/viewmodels_events.go:90](tools/viewmodels_events.go:90)) and `sessionGetResultPage` ([tools/viewmodels.go:20](tools/viewmodels.go:20)); update the composite-literal construction at [tools/tools.go:262](tools/tools.go:262) only if it uses positional fields (it uses named fields — no change expected).
- D7 ([session/session.go:303](session/session.go:303)), mirrors: `Turn.Validate` ([session/turn.go:40](session/turn.go:40)):

```diff
 func (s *Session) Validate() error {
 	if s == nil {
-		return errors.New("session is nil")
+		return errors.New("Session.Validate: called on nil")
 	}
 
 	if s.Meta.SessionId == "" {
-		return errors.New("id must not be empty")
+		return errors.New("Session.Validate: id must not be empty")
 	}
 
 	if s.Agent != AgentClaude && s.Agent != AgentCodex {
-		return errors.New("agent must be \"claude\" or \"codex\"")
+		return errors.New("Session.Validate: agent must be \"claude\" or \"codex\"")
 	}
 
 	if s.LastActive.IsZero() {
-		return errors.New("last_active must not be zero")
+		return errors.New("Session.Validate: last_active must not be zero")
 	}
 
 	if s.TurnsFinished == nil {
-		return errors.New("turns must not be nil")
+		return errors.New("Session.Validate: turns must not be nil")
 	}
 
 	if s.Events == nil {
-		return errors.New("events must not be nil")
+		return errors.New("Session.Validate: events must not be nil")
 	}
 
 	return nil
 }
```

Update any test asserting these exact messages (grep in phase 4 sweep).

## Hot items

`N/A — no SQL/CTE, no new goroutine/channel/lock, no new interface or generic, no migration, no anonymous struct.` D7 touches validation *messages* only (hot-items entry 5 covers weakening/removing guard logic — no guard changes); the full diffs are in the plan regardless.

## Tests

| Location.Method | Cases | Comment |
|-----------------|-------|---------|
| session/session_events_test.go `TestSession_CurrentUsage` | active turn counted once (driven via AddTurn)<br>no active turn<br>TotalUsage not mutated | rewrite — replaces hand-built state |
| session/store_test.go `TestStore_ResolveSubagentActor_ResultBeforeSpawn` (new) | result appended first, spawn second → buffered result gains agent_id | pins the restart replay order |
| tools/pages_test.go `buildEvents` cases | single page: full string<br>multi page: chunks concatenate to original<br>empty inputs | update — assert string type + reassembly |
| session/session_test.go `TestSession_Validate*` | existing cases | update expected messages if asserted |

Not tested: live OTLP/telemetry paths — untouched by this plan.

## Test runbook

Behavioral drivers D1/D3 re-run and extend the existing deep_analysis runbooks (tool discovered: `sh` scripts POSTing MCP `tools/call` to `127.0.0.1:4242/mcp`, see [plans/deep_analysis/runbooks/events_time.sh](plans/deep_analysis/runbooks/events_time.sh)).

- location: plans/deep_analysis/runbooks/events_usage.sh (new)

```sh
#!/bin/sh
curl -s http://127.0.0.1:4242/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "session_events",
    "arguments": {
      "agent": "claude",
      "json": true
    }
  }
}'
```

- location: plans/deep_analysis/runbooks/events_revisions_paged.sh (new — forces the multi-page path via revisions)

```sh
#!/bin/sh
curl -s http://127.0.0.1:4242/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "session_events",
    "arguments": {
      "agent": "claude",
      "revisions": true,
      "json": true
    }
  }
}'
```

D2 restart replay: restart the running peek-mcp binary against existing transcripts containing subagent activity, then run `events_usage.sh` and check `subagent_result` entries for non-empty `agent_id`. D4–D7 need no runbook (behavior-preserving).

## Contracts & sweeps

| Contract | Sides | Sweep |
|----------|-------|-------|
| `session_events.events`/`revisions` shape (D3) | producer: `buildEvents`; consumers: docs/tools.md, surface.md, pages_test.go, runbooks under plans/*/runbooks (session_events.sh, events_*.sh), README examples | `grep -rn "rawJsonSegment"` → 0 hits; `grep -rn "RawMessage" tools/` → only pre-existing non-events uses; docs grep for "array" in the session_events sections → reworded |
| `CurrentUsage` semantics (D1) | producer: session.go; consumers: tools.go:203, tools.go:315, control/sessions.go:137, control/api.go:207 | all 4 call sites unchanged in code, re-verified by live runbook values |
| `Session.Validate` messages (D7) | producer: session.go; consumers: any `assert.*Error*` on the literals | `grep -rn "must not be nil\|must not be empty\|must not be zero" --include="*_test.go" session/` → each hit updated or confirmed pre-prefixed |
| `.claude-worktree` (D4) | git index, .gitignore, worktree-sessionstart.sh (writer, untracked by design) | `git ls-files .claude-worktree` → empty; sentinel file itself remains on disk |

## Verification

Phase 1:
- [ ] Run `go test ./session/...` — all pass, including the rewritten CurrentUsage test.
- [ ] Rebuild, restart peek-mcp, run `events_usage.sh` against a live Claude session mid-assistant-turn — `usage.output_tokens` no longer exceeds the transcript's deduped sum (compare against `jq` over the transcript's usage entries).

Phase 2:
- [ ] Run `go test ./session/...` — new ordering test passes.
- [ ] Restart the binary over transcripts with subagent activity; `subagent_result` events show non-empty `agent_id`.

Phase 3:
- [ ] Run `go test ./tools/...` — updated pagination tests pass.
- [ ] Run `events_revisions_paged.sh` on a session with large revisions — every page's `events`/`revisions` is a JSON string; concatenated chunks parse as the original arrays.
- [ ] Confirm docs/tools.md and surface.md state the string contract.

Phase 4:
- [ ] Run `go build ./... && go vet ./... && gofmt -l .` — clean.
- [ ] Run `git ls-files .claude-worktree` — empty; `git status` shows the sentinel as ignored.
- [ ] Run full `go test ./...` — green.

## Stop conditions

| ID | Condition | Action |
|----|-----------|--------|
| 1 | An approved signature/contract can't hold as planned | stop and report; never improvise architecture mid-edit |
| 2 | Second failed fix on the same mechanism | stop, research the actual cause, redesign; no third band-aid |
| 3 | Missing prerequisite (generated code, running infra) | run the producing step; if infrastructure is down, ask |
| 4 | Discovered work materially exceeds approved scope | ask before continuing |
| 5 | Same kind of bug found twice: in-diff → fix all in-diff; pre-existing outside → report and ask | sweeps are the user's call |
| 6 | Structural obstacle tempts a new abstraction | stop and report; relocate, don't indirect |
| 7 | Mechanical transform (test rewrite, message prefixing) loses fidelity vs. source | diff element-by-element before presenting; any loss → stop |
| 8 | Old and new structure would coexist beyond phasing (e.g. `rawJsonSegment` survives phase 3) | stop and report |
| 9 | A driver contradicts a `[USER]` decision in the originating plan | surface the conflict |
| 10 | Live verification shows Codex usage totals changed after phase 1 | stop — the Codex path must be untouched |

## Open questions

`N/A — empty; DEC-1…DEC-5 are decided above and open to override at approval.`

## Changelog

| Date | Trigger | What changed |
|------|---------|--------------|
| — | initial | plan created |
