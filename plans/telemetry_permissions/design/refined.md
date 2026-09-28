# Telemetry permission accounting — Implementation Plan

target: `frontier` — route: `refine` of [raw.md](raw.md)

## TLDR

- peek misreports permission denials: the eval run `0ba783f3` showed 11 while Claude Code recorded 501. The telemetry fixes from the original plan are built, but its premise that telemetry can count every denial is false.
- Claude Code's telemetry emits a decision event only when the permission step reaches a verdict. A headless session that cannot answer an approval prompt denies the call without any event, and file-path deny rules are enforced before the permission step. Measured: about 10% of all denials, on real runs and in the binary.
- The transcript carries the complete record instead: every denied tool result is tagged with `toolDenialKind`, on the main chain, in Agent-tool subagents and in workflow agents. It matched Claude Code's own denial list 501 = 501 on the eval run and 868 = 868 across 37 headless sessions, and it covers 66 subagent denials that list omits.
- Design: the transcript is the denial ledger; telemetry attributes each denial to hook, settings or prompt by joining on the tool call id; the reconciliation is visible as numbers (`denied`, `denied_by_source`, `unattributed`, `telemetry_only`) so a gap never disappears.
- Plan rejections are tagged too and stay `plan_rejected`, never counted as denials. Pre-2.1.202 transcripts lose their untagged denials (16 in the current window).
- The telemetry restart restore, decision-keyed allow counters and the 1000-entry list stay; the telemetry `denied` total goes, so there is one denial count.

## Context

- **Problem:** `session_events` reported 11 denials for `0ba783f3` against 501 in Claude Code's result envelopes; the denial number is the input of the skill-eval report.
- **Cause, telemetry:** [telemetry/store.go](telemetry/store.go) folded by source and capped the list; the deeper cause is Claude Code 2.1.282 itself: `tool_decision` is emitted only for a non-`ask` verdict, and file-tool path deny rules run in input validation.
- **Cause, transcript:** [claude/parser.go:821](claude/parser.go) recognises a denial only by the prompt-rejection text, so hook and native denials never become events.
- **Design being implemented:** the user's decision of this session: transcript and telemetry, reconciled per `tool_use_id`.
- **Constraints:** peek only; the tool-policy gate ledger and the claude-configs consumer stay out; no new OTLP receiver (traces).

## Drivers

N/A — refine route (drivers recorded in the Changelog)

## Scope

- **In:**
  - **denial ledger:** `permission_denied` from the transcript's `toolDenialKind`, with kind, tool, command and `tool_use_id`, per agent.
  - **counters:** `counters.permission_denials` counts kinds `permission-rule`, `user-rejected`, `automode-blocked`; `counters.permission_cancellations` counts `cancelled` and `interrupted`.
  - **denial index:** a bounded per-session map `tool_use_id` → kind, so the reconciliation survives the 500-event ring.
  - **reconciliation view:** `permissions.denied`, `denied_by_kind`, `denied_by_source`, `unattributed`, `telemetry_only`; the block exists whenever the transcript has denials or telemetry exists.
  - **telemetry (kept from raw.md, implemented):** restart restore, decision-keyed allow counters, `config_denied`/`hook_denied`, 1000-entry list of denials and prompts.
  - **telemetry (changed):** the `denied` total is removed.
  - **docs:** `docs/tools.md`, `docs/reference.md`.
- **Out:**
  - **traces receiver:** `blocked_on_user` spans would add nothing the transcript lacks (D8).
  - **path deny rules on file tools:** untagged in the transcript, absent from telemetry; documented, not counted (D9).
  - **tool-policy ledger:** unchanged (user).
  - **claude-configs consumer:** reads `denied` after this ships; its change.
  - **dashboard columns:** the denials table gains rows, not a kind column.
- **Not changed:**
  - **Codex parser:** its permission events are unchanged.
  - **`plan_rejected`:** ExitPlanMode results keep today's meaning.
  - **per-subagent stats:** counters stay session-wide.
- **Deferred findings:**
  - **event ring:** `EventBufferCapacity` 500 truncates `events` for the eval run (about 750 events); counters and the denial index are exact, the listed events are not.
  - **`pendingTools` cap:** 64 outstanding tool uses reset the map (`claude/parser.go:380`); measured 0 stale uses in the eval run, so no denial lost its tool name there.

## Assumptions

| Assumption | Reality | Location |
| :--- | :--- | :--- |
| raw.md D1/D8: telemetry `denied` equals Claude Code's denial count | false: unresolved asks emit no `tool_decision` (61 of 501 in the eval run, 79 of 868 corpus-wide); file-path deny rules emit nothing; measured with a console exporter on sessions `b72e62e1`, `01631406` | claude 2.1.282: `tool_decision` guarded by `kn.behavior!=="ask"`; denial path calls only `fQt` (Perfetto annotation) |
| the transcript has no structured denial marker (raw.md D4) | false: `toolDenialKind` on every denied result's entry since CLI 2.1.202 | corpus: 5563 tagged entries, each with exactly one result block |
| `plan_rejected` and denials are disjoint | ExitPlanMode rejections carry a kind too (301 `permission-rule`, 57 `user-rejected`, all "The user doesn't want to proceed", plus 3 abort errors) | corpus join by tool name |

## Current state

N/A — refine route

## Target state

N/A — refine route

## Behavior contract

N/A — refine route

## Decisions

| ID | Problem | Facts | Decision | Rejected | Why | Consequences |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| <a id="d1"></a>D1 | Telemetry cannot see two denial classes at all, and the transcript sees every denial but not who denied it; each source alone gives a wrong picture | [F1!](#f1), [F2!](#f2), [F3!](#f3), [F4!](#f4) | **[USER]** transcript `toolDenialKind` is the denial ledger; telemetry `tool_decision` attributes each denial by `tool_use_id`; the join is exposed as numbers, never hidden | telemetry only: misses ~10% structurally<br>transcript only: no hook/settings/prompt attribution<br>traces: same gaps as logs for path deny rules, `source: unknown` for unresolved asks, needs a new receiver | **reliable:** the ledger comes from a field Claude Code writes for its own denial list; **debuggable:** `unattributed` and `telemetry_only` make any disagreement a visible count | one denial count (`counters.permission_denials`); telemetry's `denied` total is removed |
| <a id="d2"></a>D2 | Which kinds are denials: `cancelled` (Esc while a tool waits) and `interrupted` (request interrupted) are not refusals | [F5!](#f5) | `permission-rule`, `user-rejected`, `automode-blocked` count as denials; `cancelled`, `interrupted` count in `permission_cancellations`; all five emit `permission_denied` with `kind` | a second event kind for cancellations: consumers filter by `kind` anyway<br>counting all five as denials: inflates by interactive Esc presses | **controllable:** the split is one field; **reliable:** matches Claude Code's own list, which excludes cancellations | two counters instead of one |
| <a id="d3"></a>D3 | ExitPlanMode rejections carry a kind and would double as `plan_rejected` and `permission_denied` | [F6!](#f6) | an ExitPlanMode result never yields `permission_denied`; it keeps the plan verdict path (`plan_rejected` on error) | count both: 361 plan rejections would inflate denials<br>drop `plan_rejected`: breaks the plan lifecycle consumers | **reliable:** 361 of 361 ExitPlanMode kind entries are prompt rejections of a plan, none is a hook denial | a hook that denies ExitPlanMode would be counted as a plan rejection (measured 0 cases) |
| <a id="d4"></a>D4 | The text match on "The user doesn't want to proceed" and the kind field would both fire on the same result | [F2!](#f2), [F7](#f7) | the kind replaces the text match: `denialPrefix` and the `isDenied` branches in `toolResultEvent`, `subagentResultEvent`, `userAnswerEvent` are removed | keep the text match as a fallback for pre-2.1.202 transcripts: a second mechanism for 16 results in the window | replace semantics; **reliable:** the kind is present on every tagged result, the text on a subset | transcripts from CLI < 2.1.202 report no denials; documented |
| <a id="d5"></a>D5 | The reconciliation joins denials to telemetry by `tool_use_id`, but the event ring holds 500 events and the eval run has 501 denials | [F8!](#f8) | `Session.DeniedToolUses map[string]string` (`tool_use_id` → kind), filled in `AddEvent`, capped at `maxDeniedToolUses` 5000, mirroring `TouchedFiles` (map, `maxTouchedFiles`) | join over `Events.All()`: wrong once the ring wraps<br>raise the ring: 500 is a response-size choice | **reliable:** the index is exact up to 5000 denials (10× the largest observed run); the counter stays exact beyond | ~70 bytes per denial in memory; beyond 5000 `unattributed` overstates |
| <a id="d6"></a>D6 | Attribution needs the telemetry side per `tool_use_id`; the counters are exact but the list is capped at 1000 | [F9](#f9) | the join reads `Requests` (every denial is listed, D2 of raw.md); a session with more than 1000 listed decisions attributes the first 1000 | a second telemetry index: the list already is one | **debuggable:** the cap is documented and visible as `unattributed` growth | none for observed runs (max 501) |
| <a id="d7"></a>D7 | Tool name for a denial: the result block carries no tool name | [F10](#f10) | tool and command come from the parser's `pendingTools` entry; when absent, the event carries the kind and `tool_use_id` with empty tool and command | look up the store's `ToolCall` ring: parser events would depend on store state | **reliable:** parsers are per file and a tool use precedes its result in the same file; measured 0 stale uses | a result after a `pendingTools` reset yields a tool-less denial, still counted |
| <a id="d8"></a>D8 | Traces (`CLAUDE_CODE_ENHANCED_TELEMETRY_BETA`) see unresolved asks | [F11](#f11) | out of scope: no traces receiver | build one: `source: unknown` on the spans that matter, no span for path deny rules, new endpoint and store | adds nothing over D1 | — |
| <a id="d9"></a>D9 | File-path deny rules on Read/Edit/Write produce an untagged error result | [F12](#f12) | not counted; documented with the three fixed messages | match the messages: version-tied strings for 1 occurrence in the corpus (own test) | **reliable:** no guessing in the ledger | a session denied by path rules undercounts by those calls |
| <a id="d10"></a>D10 | When the `permissions` block appears | [F3!](#f3) | Claude sessions: present when the session has denials or telemetry stats; `detail` is `""` (live telemetry), `persisted`, or `transcript-only` | keep telemetry-gated: the ledger would vanish without export | **controllable:** the block reports what it has and says which source | `no-data-nil` test case changes meaning |
| <a id="d11"></a>D11 | `subagent`-scoped `session_events` | — | the `permissions` block stays session-wide, like `counters` | per-agent reconciliation: telemetry has no agent id | consistent with counters | documented |

## Open questions

None.

## Baseline (verified)

- **Base:** branch `claude/skill-adjustments-peek-a93a72`, HEAD `a60f720`, plus the uncommitted raw.md §1–§4 implementation (`telemetry/`, `tools/viewmodels_events.go`, docs, tests; `go test ./...` green).

| ID | Fact | Needed for | Location |
| :--- | :--- | :--- | :--- |
| <a id="f1"></a>F1! | `tool_decision` fires only inside `if(kn.behavior!=="ask"&&!s.toolDecisions.has(n))`; the denial path emits `tengu_tool_use_can_use_tool_rejected` and `fQt(...)`, a Perfetto span annotation, nothing to OTEL; file-tool path deny rules return `deniedByPermissionRule` from input validation (`sLt`, `gHr`, `hHr` messages) | [D1](#d1), [D9](#d9) | /opt/homebrew/Caskroom/claude-code@latest/2.1.282/claude |
| <a id="f2"></a>F2! | `toolDenialKind` is a top-level field of the user entry holding the denied result; values `permission-rule` (5027), `user-rejected` (598), `cancelled` (39), `interrupted` (18), `automode-blocked` (7); present from CLI 2.1.202; every tagged entry holds exactly one `tool_result` block (5563 of 5563); 3509 tagged entries are sidechain lines | [D1](#d1), [D2](#d2), [D4](#d4) | ~/.claude/projects corpus, jq over all transcripts |
| <a id="f3"></a>F3! | Eval run `0ba783f3`: 501 envelope denials = 501 tagged results, same ids (440 `permission-rule`, 61 `user-rejected`); all 37 headless sessions: 868 envelope ids all tagged; 68 tagged denials are missing from envelopes, 66 of them in Agent-tool or workflow-agent transcripts | [D1](#d1), [D10](#d10), [Verification](#verification) | claude-configs `evals/**/stream.jsonl`, `~/.cache/claude-routine/steer/**/stream.jsonl`, transcripts |
| <a id="f4"></a>F4! | Raw log export for session `b72e62e1` (four calls): `tool_decision` for `ls` (hook/accept) and `lsof` (hook/reject) only; nothing for `mkdir` outside cwd (transcript `user-rejected`, a `PermissionRequest` hook round ran) or `Read(./go.mod)` deny rule; telemetry attributes subagent calls to the parent `session.id` (eval run: 57 agent-tool, 85 workflow-agent of 200 entries) | [D1](#d1), [D11](#d11) | scratchpad `console-capture2.txt`; ~/.peek/state/claude/0ba783f3-…/telemetry.json |
| <a id="f5"></a>F5! | `cancelled` results read "The user doesn't want to take this action right now"; `interrupted` reads "[Request interrupted by user for tool use]"; 10 of the 18 interrupted are Agent calls | [D2](#d2) | corpus tally by kind and result text |
| <a id="f6"></a>F6! | ExitPlanMode results with a kind: 301 `permission-rule` + 57 `user-rejected`, text "The user doesn't want to proceed with this tool use", + 3 "Tool permission request failed: AbortError"; headless envelopes list no ExitPlanMode | [D3](#d3) | corpus join tool_use → result |
| <a id="f7"></a>F7 | untagged "doesn't want to proceed" results exist only on CLI 2.1.128–2.1.197; 16 of them in transcripts modified in the last 14 days | [D4](#d4) | corpus tally by version |
| <a id="f8"></a>F8! | `EventBufferCapacity` = 500; `TouchedFiles` is the existing bounded per-session map (`maxTouchedFiles` 2000) | [D5](#d5) | [session/session.go:28,32](session/session.go) |
| <a id="f9"></a>F9 | telemetry `Requests` lists every reject and every prompted decision, capped at 1000 (raw.md D2, implemented) | [D6](#d6) | [telemetry/store.go](telemetry/store.go) `isListedDecision` |
| <a id="f10"></a>F10 | one `Parser` per watched file; `pendingTools` resets at 64 outstanding uses; eval run: max 0 tool uses without a result in any file | [D7](#d7) | [watcher/watcher.go:104,345](watcher/watcher.go); [claude/parser.go:380](claude/parser.go) |
| <a id="f11"></a>F11 | traces export (session `01631406`): `claude_code.tool` spans for `ls`, `mkdir`, `lsof` with `tool_use_id`; `tool.blocked_on_user` child spans with `decision: reject`, `source: unknown`; no span for the Read deny rule | [D8](#d8) | scratchpad `traces-capture.txt` |
| <a id="f12"></a>F12 | path deny rule result: `<tool_use_error>File is in a directory that is denied by your permission settings.</tool_use_error>`, `is_error` true, no kind; 1 occurrence corpus-wide (own test) | [D9](#d9) | session `464aaf9e` transcript |
| <a id="f13"></a>F13 | `permissionDeniedEvent(block, command, entry, tool)` is called from `subagentResultEvent`, `toolResultEvent`, `userAnswerEvent`; `denialPrefix` decides `isDenied`; the dashboard lists denial events (`control/usage.go:595`) and shows `Counters.PermissionDenials` | [§2](#2-kind-based-denial-events-modified) | [claude/parser.go:565-590,815-836](claude/parser.go) |

## Exemplar & reuse

| Existing | Used for |
| :--- | :--- |
| `TouchedFiles` + `addFileTouch` cap ([session/session.go:125-149](session/session.go)) | the bounded `DeniedToolUses` index |
| `permissionDeniedEvent`, `commandFromInput` ([claude/parser.go:565,564](claude/parser.go)) | the kind-based event |
| `newPermissionsView` / `permissionsViewFromStats` ([tools/viewmodels_events.go:93-122](tools/viewmodels_events.go)) | the reconciliation view |
| `decisionRecord`, `logsPayload` ([telemetry/otlp_test.go](telemetry/otlp_test.go)); `promptedDecisionLogs` ([tools/viewmodels_events_test.go](tools/viewmodels_events_test.go)) | tests |

- No new file; every change modifies an existing unit.

## Permissions

N/A — `Bash(claude -p *)` was applied for raw.md and covers the live checks.

## Changes

| File | Kind | Entry |
| :--- | :--- | :--- |
| `claude/entry.go` | modified | [§1](#1-denial-kind-on-the-entry-modified) |
| `claude/parser.go` | modified | [§2](#2-kind-based-denial-events-modified) |
| `session/event.go` | modified | [§3](#3-payload-kind-counters-and-the-denial-index-modified) |
| `session/session.go` | modified | [§3](#3-payload-kind-counters-and-the-denial-index-modified) |
| `telemetry/store.go` | modified | [§4](#4-telemetry-without-a-denied-total-modified) |
| `tools/viewmodels_events.go` | modified | [§5](#5-reconciliation-view-modified) |
| `docs/tools.md` | modified | [§6](#6-docs-modified) |
| `docs/reference.md` | modified | [§6](#6-docs-modified) |
| `claude/parser_events_test.go` | modified | [Tests](#tests) |
| `claude/parser_touches_test.go` | modified | [Tests](#tests) |
| `session/session_test.go` | modified | [Tests](#tests) |
| `telemetry/store_test.go` | modified | [Tests](#tests) |
| `tools/viewmodels_events_test.go` | modified | [Tests](#tests) |

### 1. Denial kind on the entry (modified)

location: `claude/entry.go`

```diff
 	Content           string          `json:"content"`   // queue-operation payload
 	Operation         string          `json:"operation"` // queue-operation: enqueue, dequeue, remove
+	ToolDenialKind    string          `json:"toolDenialKind"` // set on the entry holding a denied tool_result
 }
```

### 2. Kind-based denial events (modified)

location: `claude/parser.go`

- **Constants:** `denialPrefix` is removed; the kind values live in `session` (§3).
- **eventsFromUserContent:** the kind decides a denial before any tool-specific handling; ExitPlanMode keeps its verdict path ([D3](#d3)); a missing pending entry no longer skips the block ([D7](#d7)).

```diff
 func (p *Parser) eventsFromUserContent(entry *Entry, message *Message) ([]*session.Event, []*session.FileTouch) {
 	blocks := contentBlocks(message.Content)
 
 	events := make([]*session.Event, 0)
 	touches := make([]*session.FileTouch, 0)
 	for index := range blocks {
 		block := &blocks[index]
 		if block.Type != contentTypeToolResult {
 			continue
 		}
 
 		pending, ok := p.pendingTools[block.ToolUseId]
-		if !ok {
-			continue
-		}
-
-		delete(p.pendingTools, block.ToolUseId)
+		delete(p.pendingTools, block.ToolUseId)
+
+		isPlanVerdict := ok && pending.name == toolNameExitPlanMode
+		if entry.ToolDenialKind != "" && !isPlanVerdict {
+			events = append(events, deniedToolEvent(block, entry, pending))
+			continue
+		}
+		if !ok {
+			continue
+		}
 
 		if touch := fileTouchFromResult(block, pending); touch != nil {
```

- **New free function** `deniedToolEvent`, placed before `eventTurn` (alphabetical among free functions); `pending` may be nil.

```go
// deniedToolEvent turns a denied tool result into the ledger event; the tool
// name and command come from the matching tool use when the parser saw it.
func deniedToolEvent(block *ContentBlock, entry *Entry, pending *pendingToolUse) *session.Event {
	if pending == nil {
		return permissionDeniedEvent(block, "", entry, entry.ToolDenialKind, "")
	}
	return permissionDeniedEvent(block, commandFromInput(pending), entry, entry.ToolDenialKind, pending.name)
}
```

- **permissionDeniedEvent:** gains `kind` (alphabetical parameter order).

```diff
-func permissionDeniedEvent(block *ContentBlock, command string, entry *Entry, tool string) *session.Event {
-	payload := &session.PermissionPayload{Command: command, Tool: tool, ToolUseId: block.ToolUseId}
+func permissionDeniedEvent(block *ContentBlock, command string, entry *Entry, kind string, tool string) *session.Event {
+	payload := &session.PermissionPayload{Command: command, Kind: kind, Tool: tool, ToolUseId: block.ToolUseId}
 	return &session.Event{
```

- **toolResultEvent and callees:** the text-based denial branches go ([D4](#d4)).

```diff
 func toolResultEvent(block *ContentBlock, entry *Entry, pending *pendingToolUse) *session.Event {
 	var text string
 	if err := json.Unmarshal(block.Content, &text); err != nil {
 		text = extractTextBlocks(block.Content)
 	}
 
-	isDenied := block.IsError && strings.HasPrefix(text, denialPrefix)
-
 	switch pending.name {
 	case toolNameExitPlanMode:
 		return planVerdictEvent(block, entry, text)
 	case toolNameAgent:
-		return subagentResultEvent(block, entry, isDenied, text)
+		return subagentResultEvent(block, entry, text)
 	case toolNameAskUserQuestion:
-		return userAnswerEvent(block, entry, isDenied, pending, text)
+		return userAnswerEvent(block, entry, pending, text)
 	default:
-		if !isDenied {
-			return nil
-		}
-		return permissionDeniedEvent(block, commandFromInput(pending), entry, pending.name)
+		return nil
 	}
 }
```

```diff
-func subagentResultEvent(block *ContentBlock, entry *Entry, isDenied bool, text string) *session.Event {
-	if isDenied {
-		return permissionDeniedEvent(block, "", entry, toolNameAgent)
-	}
-
+func subagentResultEvent(block *ContentBlock, entry *Entry, text string) *session.Event {
 	content := resolvePersistedOutput(entry.SessionId, text, block.ToolUseId)
```

```diff
-func userAnswerEvent(block *ContentBlock, entry *Entry, isDenied bool, pending *pendingToolUse, text string) *session.Event {
-	if isDenied {
-		return permissionDeniedEvent(block, "", entry, toolNameAskUserQuestion)
-	}
-
+func userAnswerEvent(block *ContentBlock, entry *Entry, pending *pendingToolUse, text string) *session.Event {
 	var input askUserQuestionInput
```

- **Sidechain:** `handleSidechain` already calls `eventsFromUserContent`, so subagent denials carry `Actor: entry.AgentId` unchanged.
- **Unchanged:** `permissionModeEvent`, plan verdicts, skill events, `toolResultsFromContent` (§ tool calls of this release).

### 3. Payload kind, counters and the denial index (modified)

location: `session/event.go`, `session/session.go`
mirrors: `TouchedFiles` / `addFileTouch` for the index

```diff
 const (
 	EventKindModelChanged          EventKind = "model_changed"
 	// ...
 )
+
+const (
+	DenialKindAutomodeBlocked = "automode-blocked"
+	DenialKindCancelled       = "cancelled"
+	DenialKindInterrupted     = "interrupted"
+	DenialKindPermissionRule  = "permission-rule"
+	DenialKindUserRejected    = "user-rejected"
+)
```

```diff
 type Counters struct {
 	ModelChanges          int `json:"model_changes"`
+	PermissionCancellations int `json:"permission_cancellations"`
 	PermissionDenials     int `json:"permission_denials"`
```

```diff
 type PermissionPayload struct {
 	Command       string `json:"command,omitempty"`
 	Justification string `json:"justification,omitempty"`
+	Kind          string `json:"kind,omitempty"` // transcript toolDenialKind; empty on Codex and granted events
 	Tool          string `json:"tool"`
 	ToolUseId     string `json:"tool_use_id,omitempty"`
 }
```

```diff
 const maxTouchedFiles = 2000
+
+const maxDeniedToolUses = 5000
```

```diff
 type Session struct {
 	// ...
+	DeniedToolUses  map[string]string           `json:"-"` // tool_use_id → denial kind, survives the event ring
 	DiffBase        string                      `json:"-"`
```

```diff
 func (s *Session) AddEvent(event *Event) {
 	s.Events.Push(event)
 
 	switch event.Kind {
 	case EventKindModelChanged:
 		s.Counters.ModelChanges++
 	case EventKindPermissionDenied:
-		s.Counters.PermissionDenials++
+		s.addDenial(event.Permission)
```

- **New method** `addDenial`, placed with the private methods of `Session` (before `isAlterationPhase`):

```go
// addDenial counts by kind and indexes the tool use, so the telemetry join
// stays exact after the event ring has dropped the event.
func (s *Session) addDenial(payload *PermissionPayload) {
	isCancellation := payload != nil && (payload.Kind == DenialKindCancelled || payload.Kind == DenialKindInterrupted)
	if isCancellation {
		s.Counters.PermissionCancellations++
	} else {
		s.Counters.PermissionDenials++
	}

	if payload == nil || payload.ToolUseId == "" {
		return
	}
	if s.DeniedToolUses == nil {
		s.DeniedToolUses = make(map[string]string)
	}
	if len(s.DeniedToolUses) >= maxDeniedToolUses {
		return
	}
	s.DeniedToolUses[payload.ToolUseId] = payload.Kind
}
```

- **Codex:** its `permission_denied` events carry no `Kind`, so they count as denials (unchanged behaviour) and index by `ToolUseId` (the call id, set this release).

### 4. Telemetry without a denied total (modified)

location: `telemetry/store.go`

- Everything from raw.md §1–§2 stays as implemented; only the `Denied` total goes ([D1](#d1)).

```diff
 type PermissionStats struct {
 	Aborted        int                  `json:"aborted,omitempty"`
 	AutoAllowed    int                  `json:"auto_allowed,omitempty"`
 	ConfigDenied   int                  `json:"config_denied,omitempty"`
-	Denied         int                  `json:"denied,omitempty"`
 	HookAllowed    int                  `json:"hook_allowed,omitempty"`
```

```diff
 func (p *PermissionStats) count(decision *PermissionDecision) {
 	isReject := decision.Decision == decisionReject
-	if isReject {
-		p.Denied++
-	}
-
 	switch decision.Source {
```

```diff
 func (p *PermissionStats) IsZero() bool {
 	hasNoAllows := p.AutoAllowed == 0 && p.HookAllowed == 0 && p.PromptedOnce == 0 && p.PromptedAlways == 0
-	return hasNoAllows && p.Denied == 0 && len(p.Requests) == 0
+	hasNoDenials := p.ConfigDenied == 0 && p.HookDenied == 0 && p.Rejected == 0 && p.Aborted == 0
+	return hasNoAllows && hasNoDenials && len(p.Requests) == 0
 }
```

### 5. Reconciliation view (modified)

location: `tools/viewmodels_events.go`
mirrors: the existing `permissionsView` / `permissionsViewFromStats` pair

- **Shape:** the transcript numbers first, then the join, then the telemetry counters and list.

```go
type deniedByKindView struct {
	AutomodeBlocked int `json:"automode_blocked"`
	PermissionRule  int `json:"permission_rule"`
	UserRejected    int `json:"user_rejected"`
}

type deniedBySourceView struct {
	Aborted int `json:"aborted"`
	Config  int `json:"config"`
	Hook    int `json:"hook"`
	Prompt  int `json:"prompt"`
}

type permissionsView struct {
	Aborted        int                            `json:"aborted,omitempty"`
	AutoAllowed    int                            `json:"auto_allowed"`
	ConfigDenied   int                            `json:"config_denied"`
	Denied         int                            `json:"denied"`
	DeniedByKind   *deniedByKindView              `json:"denied_by_kind"`
	DeniedBySource *deniedBySourceView            `json:"denied_by_source"`
	Detail         string                         `json:"detail,omitempty"`
	HookAllowed    int                            `json:"hook_allowed"`
	HookDenied     int                            `json:"hook_denied"`
	PromptedAlways int                            `json:"prompted_always"`
	PromptedOnce   int                            `json:"prompted_once"`
	Rejected       int                            `json:"rejected"`
	Requests       []telemetry.PermissionDecision `json:"requests,omitempty"`
	TelemetryOnly  int                            `json:"telemetry_only"`
	Unattributed   int                            `json:"unattributed"`
}
```

```go
const detailTranscriptOnly = "transcript-only"

func newPermissionsView(currentSession *session.Session, telemetryStore *telemetry.Store, stateDir *state.Dir) *permissionsView {
	if currentSession.Agent != session.AgentClaude {
		return nil
	}

	stats, detail := permissionStatsFor(currentSession, telemetryStore, stateDir)
	hasDenials := currentSession.Counters.PermissionDenials > 0
	if stats == nil && !hasDenials {
		return nil
	}

	view := permissionsViewFromStats(stats, detail)
	view.Denied = currentSession.Counters.PermissionDenials
	view.DeniedByKind = deniedByKind(currentSession.DeniedToolUses)
	view.DeniedBySource, view.Unattributed, view.TelemetryOnly = reconcileDenials(currentSession.DeniedToolUses, stats)
	return view
}

// permissionStatsFor returns the telemetry stats and their provenance: live,
// persisted, or none (transcript-only).
func permissionStatsFor(currentSession *session.Session, telemetryStore *telemetry.Store, stateDir *state.Dir) (*telemetry.PermissionStats, string) {
	sessionId := string(currentSession.Meta.SessionId)
	if telemetryStore != nil {
		if stats, ok := telemetryStore.Get(sessionId); ok && !stats.Permissions.IsZero() {
			return &stats.Permissions, ""
		}
	}

	if stats, ok := telemetry.ReadPersisted(stateDir, sessionId); ok && !stats.Permissions.IsZero() {
		return &stats.Permissions, "persisted"
	}
	return nil, detailTranscriptOnly
}

func permissionsViewFromStats(stats *telemetry.PermissionStats, detail string) *permissionsView {
	view := &permissionsView{Detail: detail}
	if stats == nil {
		return view
	}

	view.Aborted = stats.Aborted
	view.AutoAllowed = stats.AutoAllowed
	view.ConfigDenied = stats.ConfigDenied
	view.HookAllowed = stats.HookAllowed
	view.HookDenied = stats.HookDenied
	view.PromptedAlways = stats.PromptedAlways
	view.PromptedOnce = stats.PromptedOnce
	view.Rejected = stats.Rejected
	view.Requests = stats.Requests
	return view
}

func deniedByKind(denied map[string]string) *deniedByKindView {
	view := &deniedByKindView{}
	for _, kind := range denied {
		switch kind {
		case session.DenialKindAutomodeBlocked:
			view.AutomodeBlocked++
		case session.DenialKindPermissionRule:
			view.PermissionRule++
		case session.DenialKindUserRejected:
			view.UserRejected++
		}
	}
	return view
}

// reconcileDenials joins the transcript ledger to telemetry's listed rejects
// by tool use id: attributed per source, unattributed (no decision event, the
// unresolved asks), and telemetry-only (a reject with no transcript denial).
func reconcileDenials(denied map[string]string, stats *telemetry.PermissionStats) (*deniedBySourceView, int, int) {
	bySource := &deniedBySourceView{}
	attributed := make(map[string]bool)
	telemetryOnly := 0
	if stats != nil {
		for index := range stats.Requests {
			request := &stats.Requests[index]
			if request.Decision != "reject" {
				continue
			}
			if !isCountedDenial(denied, request.ToolUseId) {
				telemetryOnly++
				continue
			}
			attributed[request.ToolUseId] = true
			countSource(bySource, request.Source)
		}
	}

	unattributed := 0
	for toolUseId, kind := range denied {
		if isCancellationKind(kind) || attributed[toolUseId] {
			continue
		}
		unattributed++
	}
	return bySource, unattributed, telemetryOnly
}
```

- **Helpers** `isCountedDenial(denied, toolUseId)` (present and not a cancellation kind), `isCancellationKind(kind)`, `countSource(view, source)` (`config` → Config, `hook` → Hook, `user_reject` → Prompt, `user_abort` → Aborted) — small free functions after `reconcileDenials`, alphabetical among the file's free functions.
- **`permissionSummary`:** prefixes the kind when set (`permission-rule Bash: lsof -v`), so the compact event line shows who denied.

### 6. Docs (modified)

location: `docs/tools.md`, `docs/reference.md`

```diff
-… plus derived counters (`counters.permission_denials` counts only prompt rejections recorded in the transcript), a `permissions` block with telemetry-based permission decisions when [telemetry export](reference.md#telemetry) is enabled (`denied` is the complete count of denied calls from every source, split into `config_denied`, `hook_denied`, `rejected`, `aborted`; allows into `auto_allowed`, `hook_allowed`, `prompted_once`, `prompted_always`; `requests` lists each denial and prompted decision with its tool and command — `detail: "persisted"` marks stats read back from the state dir), …
+… plus derived counters (`counters.permission_denials` counts every denied tool call the transcript records — hook, settings, safety check, unanswered approval, auto-mode — on the main chain and in every subagent; `counters.permission_cancellations` counts calls the user cancelled or interrupted), a `permissions` block that reconciles that ledger with [telemetry](reference.md#telemetry): `denied` (the ledger), `denied_by_kind`, `denied_by_source` (hook, config, prompt, aborted — from the telemetry decision with the same tool use id), `unattributed` (denials with no telemetry decision: mostly approval prompts a headless session could not answer, which Claude Code exports no event for), `telemetry_only` (rejects with no transcript denial); the allow counters and `requests` come from telemetry alone, and `detail` says `persisted` or `transcript-only` when live telemetry is missing, …
```

- **Event kinds paragraph (tools.md):** `permission_denied` carries `permission.kind` (`permission-rule`, `user-rejected`, `automode-blocked`, `cancelled`, `interrupted`) from the transcript; ExitPlanMode rejections are `plan_rejected`, never denials; file-path deny rules on Read/Edit/Write are not tagged by Claude Code and are not counted; transcripts from CLI before 2.1.202 carry no kind and report no denials.
- **reference.md telemetry paragraph:** replace "`denied` counts every denied call regardless of who denied it" with: telemetry supplies attribution only; the transcript is the denial ledger (see tools.md); Claude Code emits no decision event for an approval prompt that is never answered, nor for file-path deny rules, so those appear as `unattributed` or are absent.

## Hot items

- **H1 — guard (denial detection moves from text to kind):** replaces `isDenied := block.IsError && strings.HasPrefix(text, denialPrefix)`. The kind is checked before tool-specific handling, ExitPlanMode is exempt, and a missing pending entry still yields the event.

```go
pending, ok := p.pendingTools[block.ToolUseId]
delete(p.pendingTools, block.ToolUseId)

isPlanVerdict := ok && pending.name == toolNameExitPlanMode
if entry.ToolDenialKind != "" && !isPlanVerdict {
	events = append(events, deniedToolEvent(block, entry, pending))
	continue
}
if !ok {
	continue
}
```

- **H2 — guard (counter split by kind):** `addDenial` above; a nil payload or unknown kind counts as a denial, never silently dropped.
- **H3 — locking (unchanged from raw.md):** `restoredStats` runs under the store write lock; `newPermissionsView` reads `currentSession` inside the handler's existing session access and the telemetry store under its own read lock; the `DeniedToolUses` map is written only in `AddEvent` under the store lock and read by the tool handler as every other session field is today.

## Tests

| Location.Method | Cases | Comment |
| :--- | :--- | :--- |
| claude/parser_events_test.go.TestParseLine_PermissionAndAnswers | `denied-edit-tool`, `bash-denied-command-captured`, `file-tool-denied-path-captured`, `ask-user-question-denied` gain `"toolDenialKind":"permission-rule"` and assert `Permission.Kind`<br>`unknown-tool-result-ignored` → renamed `untagged-error-result-no-denial`: is_error text without kind yields no event<br>`hook-denial-by-kind`: `PreToolUse:Bash hook error` text, kind `permission-rule` → denial with tool Bash, command<br>`unresolved-ask-by-kind`: mkdir "needs approval" text, kind `user-rejected`<br>`cancelled-kind-carried`: kind `cancelled` → event with `Kind` cancelled<br>`denial-without-pending`: kind set, no prior tool_use → event with empty tool, `ToolUseId` set<br>`plan-rejection-not-denial`: ExitPlanMode is_error with kind `permission-rule` → `plan_rejected` only | sequential `t.Run` style of the file |
| claude/parser_events_test.go.TestParseLine_SubagentResult | `agent-denied-is-permission-event` gains the kind; asserts `Kind` and no `subagent_result`<br>`sidechain-denial-carries-actor`: sidechain line with kind → `Actor` = agent id | |
| claude/parser_touches_test.go.TestParseLine_FileTouches | `error-result-no-touch` gains `"toolDenialKind":"permission-rule"` | keeps its one-event assertion |
| session/session_test.go.TestSession_AddDenial | `permission-rule-counted`<br>`cancelled-split`<br>`interrupted-split`<br>`nil-payload-counted-as-denial`<br>`index-capped-at-5000` | table-driven over `AddEvent`; the cap case asserts counter 5001, index 5000 |
| telemetry/store_test.go.TestStore_FoldDecision | cases from raw.md updated: no `Denied` assertions; `denied-counts-every-source` → `rejects-split-by-source` (config 1, hook 1, rejected 1, aborted 1, unknown source counted nowhere but listed) | |
| tools/viewmodels_events_test.go.TestNewPermissionsView | `attributed-hook-denial`: transcript denial + telemetry hook/reject same id → `Denied` 1, `DeniedBySource.Hook` 1, `Unattributed` 0<br>`unresolved-ask-unattributed`: transcript `user-rejected`, no telemetry → `Unattributed` 1, `Detail` transcript-only<br>`telemetry-only-reject`: telemetry reject id with no transcript denial → `TelemetryOnly` 1<br>`cancellation-not-denied`: kind cancelled → `Denied` 0, `PermissionCancellations` 1<br>`ring-overflow-still-attributed`: 501 denials pushed, telemetry reject for the first id → attributed via the index<br>`no-denials-no-telemetry-nil` (replaces `no-data-nil`)<br>`codex-session-nil` unchanged | builds sessions with `AddEvent` directly |

- **Existing tests pinning behaviour:** `live-store-served`, `persisted-fallback` (now also assert `Denied` 0), control `TestHandleOtlpLogs`, `TestUsageDenials` in control (rows from events, unchanged).
- **Not tested:** the headless `claude -p` runs and the corpus tallies — they are verification against real transcripts.

## Test runbook

- **eval-run-ledger:** new build on the watch window containing `0ba783f3`; `session_events` `{json: true}` → `counters.permission_denials` 501, `permission_cancellations` 0, `permissions.denied` 501, `unattributed` ≥ 61 (installed peek's telemetry file has only the first 200 decisions; the scratch state dir has none → `transcript-only`, `unattributed` 501).
- **subagent-heavy:** session `de6b5e87` → `permission_denials` 51 (Claude Code's own list says 4).
- **headless-four-calls:** session `b72e62e1` → `denied` 2 (`mkdir` user-rejected, `lsof` permission-rule), Read deny rule not counted.
- **live-attribution:** a fresh `claude -p` run against the new build's OTLP port: `lsof -v` → `denied_by_source.hook` 1; `mkdir /tmp/...` → `unattributed` 1; `telemetry_only` 0.

## Contracts & sweeps

| Contract | Sides | Sweep |
| :--- | :--- | :--- |
| `session_events.permissions`: `denied` is the transcript ledger; `denied_by_kind`, `denied_by_source`, `unattributed`, `telemetry_only` added; telemetry `denied` never shipped | peek `tools/` ↔ claude-configs `internal/peek/events.go` (`rejections()` must read `denied`) | Grep `Denied ` over `telemetry/` → 0 hits; docs updated |
| `permission_denied` event: `permission.kind` added; emitted for every tagged result except ExitPlanMode; no longer emitted from the prompt-rejection text | peek `claude/` ↔ dashboard `control/usage.go:595` ↔ claude-configs `internal/peek` | Grep `denialPrefix`, `isDenied` over `claude/` → 0 hits; control tests green |
| `counters.permission_denials` meaning changes from prompt rejections to all denials; `permission_cancellations` added | peek `session/` ↔ dashboard `_usage.html` ↔ docs | docs updated; dashboard label unchanged |
| persisted `telemetry.json`: `denied` key written by the uncommitted build in the scratch state dir only | scratch only | delete `<scratchpad>/state` before verification |

## Verification

- [ ] Run `go vet ./...` — expect no output.
- [ ] Run `go test ./...` — expect every package `ok`.
- [ ] Run `gofmt -l claude session telemetry tools` — expect no output.
- [ ] Grep `denialPrefix`, `isDenied` over `claude/*.go` and `Denied ` over `telemetry/*.go` — expect 0 hits.
- [ ] Build `go build -o dist/peek-mcp .`; remove `<scratchpad>/state`; start `./dist/peek-mcp start --port 4293 --control-port 4393 --state-dir <scratchpad>/state --depth 5000` in the background — expect `peek-mcp listening`.
- [ ] Call `session_events` `{id: 0ba783f3-b716-4bcb-ac54-e87feba798d9, json: true}` on 4293 — expect `counters.permission_denials` 501, `counters.permission_cancellations` 0, `permissions.denied` 501, `permissions.detail` `transcript-only`, `unattributed` 501, `denied_by_kind` {permission_rule 440, user_rejected 61, automode_blocked 0}.
- [ ] Cross-check with jq over the run's transcripts: count entries with `toolDenialKind` in `permission-rule`/`user-rejected`/`automode-blocked` whose result is not an ExitPlanMode call — expect 501.
- [ ] Call `session_events` for `de6b5e87-76f6-4503-9533-47443c64e7bb` — expect `permission_denials` 51; scoped with `subagent` of one workflow agent, expect its `permission_denied` events listed with that actor.
- [ ] Call `session_events` for `b72e62e1-34d1-4b89-9e94-e21d5c33ef61` — expect `denied` 2 and `events` holding two `permission_denied` with kinds `user-rejected` (mkdir) and `permission-rule` (lsof), none for the Read deny rule.
- [ ] Run `claude -p "…ls, mkdir -p /tmp/peek-measure-d, lsof -v…" --settings <scratchpad>/otel.json --output-format json` (OTLP endpoint 4393) — expect the envelope `permission_denials` length 2; then `session_events` for that session — expect `denied` 2, `denied_by_source.hook` 1, `unattributed` 1, `telemetry_only` 0, `hook_allowed` ≥ 1.
- [ ] Restart the build on the same state dir, resume that session with one more `lsof -v` — expect `denied` 3, `denied_by_source.hook` 2 (telemetry restore) and `unattributed` still 1.
- [ ] Degenerate: a Claude session with no denials and no telemetry — expect no `permissions` block; a Codex session — expect none.
- [ ] Open the dashboard session page for `0ba783f3` on 4393 — expect the Usage table's "Permission denials" row to read 501 and the denials detail to list rows with tool and command.
- [ ] Stop the build with TaskStop — expect 4293 and 4393 to refuse connections.
- [ ] Commit per package through /package-commit; the v1.2.9 release notes replace the earlier permissions line.

## Stop conditions

| ID | Condition | Action |
| :--- | :--- | :--- |
| ST1 | An approved signature or contract can't hold as planned | Stop and report. Never improvise architecture mid-edit |
| ST2 | Second failed fix on the same mechanism | Stop, research the actual cause, redesign. No third band-aid |
| ST3 | Missing prerequisite (generated code, running infra) | Run the producing step. If infrastructure is down, ask. Never skip validation, never start infrastructure yourself |
| ST4 | Discovered work materially exceeds the approved scope | Ask before continuing |
| ST5 | The same kind of bug found a second time | Inside own diff: fix every instance now. Pre-existing, outside the diff: report and ask before sweeping |
| ST6 | A structural obstacle (import cycle, package visibility) tempts a new abstraction | Stop and report. The fix is relocating the component, not indirection |
| ST7 | `counters.permission_denials` for `0ba783f3` differs from 501, or for `de6b5e87` from 51, or the jq cross-check disagrees | Stop and diagnose the parser or the counter; never adjust expectations |
| ST8 | A live `telemetry_only` count above 0 in the four-call run | Stop: a telemetry reject without a transcript denial means the join key or the ledger is wrong |
| ST9 | A change is needed outside the planned-files table, in `codex/`, `control/`, `watcher/`, or in claude-configs | Stop and report |
| ST10 | An ExitPlanMode result with a hook-denial text appears in the verification sessions | Stop and report: D3's exemption would hide a real denial |

## Changelog

| Date | Trigger | What changed |
| :--- | :--- | :--- |
| — | initial | plan created (raw.md) |
| 2026-09-28 | Q: can the 200-entry list cap be increased | D2 (raw): `maxPermissionRequests` 200 → 1000 [USER] |
| 2026-09-28 | refine: driver 1 — telemetry structurally incomplete (unresolved asks, path deny rules; binary + console-exporter capture) | D1 rewritten: telemetry is attribution, not the ledger; telemetry `denied` removed (§4); Assumptions rows 1–2 |
| 2026-09-28 | refine: driver 2 — transcript `toolDenialKind` complete (501 = 501, 868 = 868, subagents included) | §1–§3: kind-based `permission_denied`, counters by kind, denial index (D2, D4, D5, D7); F2, F3, F5, F7, F8, F10 |
| 2026-09-28 | refine: driver 3 — [USER] reconcile transcript and telemetry per `tool_use_id` | §5 reconciliation view (D1, D6, D10, D11); §6 docs; Tests, Runbook, Verification, ST7–ST8 |
| 2026-09-28 | refine: measurement — ExitPlanMode rejections carry a kind (361) | D3 exemption, F6, ST10 |
| 2026-09-28 | refine: drivers 5–6 — traces and path deny rules | D8, D9 out of scope with facts F11, F12 |
