# Deep-analysis extension — session time, telemetry, attribution, touched files — Change Plan

## TLDR

- `session_events` (the deep-analysis surface) gains a `time` block: session wall time, idle time, active time — computed from transcript timestamps for both agents.
- peek ingests Claude Code OpenTelemetry: the control server gains a `POST /otlp/v1/metrics` receiver (OTLP http/json), `peek-mcp setup` gains a step that enables telemetry export in `~/.claude/settings.json` pointing at it. True active time and session cost enrich the `time` block.
- A new `breakdown` flag on `session_events` returns per-skill token usage + execution time and per-subagent time + token usage (Claude only; transcript-derived).
- `session_events` always lists the session's touched files with read/write counts, extracted from file-tool results the parser currently discards.
- All additions are additive — no existing field of any tool changes.

## Context

- Drivers extend the deep-analysis feature shipped from [plans/deep_analysis/design/raw.md](plans/deep_analysis/design/raw.md); its surface is the `session_events` MCP tool ([tools/tools.go:172](tools/tools.go)).
- No duration/idle computation exists anywhere; sessions track only `LastActive` ([session/session.go:46](session/session.go)).
- Sidechain (subagent) usage and timestamps are parsed but discarded ([claude/parser.go:216-235](claude/parser.go)); file paths in tool inputs are discarded after result matching ([claude/parser.go:576-597](claude/parser.go)).
- Claude Code telemetry ([docs](https://code.claude.com/docs/en/monitoring-usage)) exports `claude_code.active_time.total` and `claude_code.cost.usage` with a `session.id` attribute over OTLP — peek's control server ([control/server.go:70](control/server.go)) is the natural receiver.
- Originating-plan conflict surfaced per stop condition 9: raw.md lists "general tool-call mirroring" as a non-goal. Driver 4 grazes it; [D4](#d4) resolves it as targeted, aggregated file-path extraction — no tool-call mirror, no new event kinds.

## Drivers

| ID | Observed | Wanted | Impact | Origin |
|---|---|---|---|---|
| <a id="dr1"></a>DR1 | `session_events` reports no time metrics; only per-event timestamps exist | Session wall time and idle time reported | behavioral | request |
| <a id="dr2"></a>DR2 | peek has no access to Claude Code telemetry (true active time, cost) | peek receives OTel export (OTLP receiver) and `setup` enables it | contract-touching | request + [monitoring docs](https://code.claude.com/docs/en/monitoring-usage) |
| <a id="dr3"></a>DR3 | Skills/subagents appear only as events; sidechain usage/timestamps are discarded at parse time | Per-skill token usage + execution time, per-subagent time + token usage, gated behind a flag | behavioral | request |
| <a id="dr4"></a>DR4 | File paths in Read/Write/Edit tool inputs are discarded after result matching | `session_events` lists touched files of the session | behavioral | request |

## Scope

- **In:**
  - **time block:** `started_at`, wall/idle/active seconds on `session_events` (Claude + Codex)
  - **OTLP receiver:** `POST /otlp/v1/metrics` (http/json) on the control server, new `telemetry` package
  - **setup step:** telemetry env block written to `~/.claude/settings.json`
  - **breakdown flag:** per-skill and per-subagent attribution arrays on `session_events` (Claude only)
  - **touched files:** aggregated per-path read/write counts on `session_events` (Claude only)
  - **docs:** README tool rows, parity notes, telemetry section
- **Out:**
  - **OTLP logs/traces:** events endpoint (`/v1/logs`), tracing beta — metrics only ([D7](#d7))
  - **Codex telemetry:** Codex has no OTel export
  - **cost computation from transcripts:** the `plans/usage_reporting` concept (pricing package) stays untouched
- **Not changed:**
  - **TotalUsage semantics:** main-loop only for Claude, keep-last for Codex
  - **event stream:** no new event kinds, buffer content unchanged
  - **session_full / session_latest / session_get:** turn JSON and existing blocks untouched
- **Deferred findings:**
  - **control dashboard:** could render the new time/attribution data (`control/templates`) — not planned
  - **OTLP logs ingestion:** per-tool durations (`claude_code.tool_result`) could later refine skill execution time

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| Request implied telemetry could attribute skill tokens | OTel metrics carry no per-skill token attribution; skill attribution stays transcript-derived per the answered window question | [monitoring docs](https://code.claude.com/docs/en/monitoring-usage) |
| "Deep analysis" is a distinct feature | It shipped as the `session_events` tool plus inline event entries | [tools/tools.go:172](tools/tools.go) |
| Skill execution has an end marker | A `Skill` tool_use only loads instructions; the window is defined by [D2](#d2) | [claude/parser.go:500](claude/parser.go) |

## Current state

| File | Lines | Responsibility (relevant slice) |
|---|---|---|
| [session/session.go](session/session.go) | 197 | `LastActive`, `TotalUsage` fold with `usageRequestIds` dedup (AddTurn:86-123); no start time, no idle |
| [session/turn.go](session/turn.go) | 105 | signal-turn taxonomy: `IsEventSignal`, `IsSubagentSignal`, `IsUsageSignal` |
| [session/store.go](session/store.go) | ~560 | fold order in `AddTurnBySessionId`:71-137 — subagent-signal route, events loop, plan, usage-signal, `AddTurn` |
| [claude/parser.go](claude/parser.go) | 622 | `handleSidechain`:216 drops usage/timestamps; `toolResultEvent`:576 default branch drops successful file-tool results; `pendingTools` holds `{input, name}` |
| [tools/tools.go](tools/tools.go) | ~700 | `Register(server, store)`:29; `sessionEventsHandler`:457; `unsupportedSignals`:571 |
| [tools/viewmodels_events.go](tools/viewmodels_events.go) | 152 | `sessionEventsResult`:26 — Counters, Diff, Events, PlanRevisions, Unsupported, Usage |
| [control/server.go](control/server.go) | 91 | `Options`/`Server`, mux in `Handler()`:70, middleware chain `logRequests(checkHost(auth(mux)))` |
| [cmd/start.go](cmd/start.go) | 320 | wiring: store, watchers, `tools.Register`:137, control server:139-165, flags + `envFallbacks` |
| [cmd/setup.go](cmd/setup.go) | 299 | interactive steps `setupClaudeCode`/`setupClaudeDesktop`/`setupCodex`; `writeConfig` JSON merge helper |
| [watcher/watcher.go](watcher/watcher.go) | 287 | `readSubagentMeta`:223 emits `subagent_spawned` via event-only turn |

## Target state

```
transcript JSONL ──claude/codex parser──▶ session.Turn ──store fold──▶ session.Session
                     + FileTouches                                       + StartedAt / Idle
                     + SubagentId/Usage                                  + TouchedFiles map
                                                                         + Skills []SkillStat
                                                                         + Subagents map[string]SubagentStat
Claude Code ──OTLP http/json──▶ control POST /otlp/v1/metrics ──▶ telemetry.Store ─┐
                                                                                    ▼
                                                     tools.sessionEventsHandler ──▶ time / touched_files / breakdown
```

- **Session aggregates (Phases 1–4).** Principle: single source of truth — all metrics fold incrementally into `session.Session` during the existing line-by-line ingestion; no lazy recomputation at read time. Mechanism: new fields on `Session` mutated under the store lock, exactly like `TotalUsage` today.
- **Telemetry side-store (Phase 5).** Principle: separate bounded cache for an independent data source keyed by session id — telemetry arrives on HTTP goroutines, transcripts on watcher goroutines; merging happens only at render time in `tools`. Mechanism: `telemetry.Store` with its own `sync.RWMutex` (hot item below), consulted read-only by the handler.
- **Parser stays allowlist-based.** Principle: targeted signal extraction, not tool-call mirroring. Mechanism: the existing `pendingTools` result-matching loop gains one aggregated output (file touches) and the sidechain path stops discarding fields it already parses.

## Behavior contract

- Every existing `session_events`, `session_full`, `session_latest`, `session_get` field keeps its shape and semantics; all additions are new optional JSON fields.
- `TotalUsage` never includes subagent tokens (unchanged — sidechain turns carry no usage into `AddTurn` today, and the new route bypasses it).
- Codex keep-last usage fold (`IsUsageSignal` → replace) unchanged.
- Watcher-emitted `subagent_spawned` events still appear in the event stream with identical payloads.
- Pinned by: [session/session_events_test.go](session/session_events_test.go), [session/store_events_test.go](session/store_events_test.go), [session/usage_test.go](session/usage_test.go), [claude/parser_events_test.go](claude/parser_events_test.go), [codex/parser_events_test.go](codex/parser_events_test.go), [tools/viewmodels_events_test.go](tools/viewmodels_events_test.go), [control/server_test.go](control/server_test.go), [control/api_test.go](control/api_test.go).
- Intentional changes: only the additive fields and the new `/otlp/v1/metrics` route ([DR1](#dr1)–[DR4](#dr4)).

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d1"></a>D1 | Idle has no transcript marker | only per-entry timestamps exist ([Current state](#current-state)) | Idle = sum of gaps ≥ 5 min between consecutive main-timeline turn timestamps; active = wall − idle; `idleThreshold` const | Gaps under 5 min are normal tool/processing latency; above is a human away. Subagent activity during a main-loop wait counts as idle by this rule — acceptable, telemetry active time corrects it |
| <a id="d2"></a>D2 | Skill has no end marker | `Skill` tool_use loads instructions only | **[USER]** window = invocation → next user prompt or next skill invocation, whichever first; open window closes at `LastActive` at render time | Answered at intake. Skill usage counts main-loop tokens only; subagent tokens stay in the subagent rows |
| <a id="d3"></a>D3 | How telemetry reaches peek | control server exists with bearer auth ([control/middleware.go:55](control/middleware.go)) | **[USER]** peek runs an OTLP receiver; http/json protocol only; setup enables export | Answered at intake. http/json is decodable with stdlib JSON — no OTel SDK dependency |
| <a id="d4"></a>D4 | Touched-files vs. raw.md non-goal "general tool-call mirroring" | file_path is present in `pendingTools` input and dropped ([claude/parser.go:299](claude/parser.go)) | Extract file paths of successful Read/Write/Edit/NotebookEdit/MultiEdit results, aggregate per path as `{reads, writes}` counts on the session; no events, no per-call records | Aggregation is not a mirror; the event buffer (cap 500) is never touched. Sidechain touches included — subagents touch real files |
| <a id="d5"></a>D5 | Where subagent usage lives | `TotalUsage` is main-loop-only today | Subagent usage folds into `Session.Subagents` per agent id, never into `TotalUsage` | Preserves the behavior contract; the breakdown makes the split explicit |
| <a id="d6"></a>D6 | Two subagent routes would coexist (event-only signal + new usage turns) | `IsSubagentSignal` ([session/turn.go:28](session/turn.go)), `addSubagentEvents` ([session/store.go:139](session/store.go)) | Dispose: watcher meta turns set `SubagentId` too; store routes on `SubagentId`; `IsSubagentSignal` and `addSubagentEvents` are deleted | Single routing mechanism; no parallel old/new form survives |
| <a id="d7"></a>D7 | Telemetry scope | drivers need active time + cost; skill/subagent attribution is transcript-derived ([D2](#d2)) | Ingest only `claude_code.active_time.total` and `claude_code.cost.usage`; ignore everything else; no logs endpoint | Smallest receiver that serves the drivers; logs are a deferred finding |
| <a id="d8"></a>D8 | Unbounded growth of new aggregates | store holds all sessions in memory | Caps: 500 touched-file paths, 100 skill windows, 200 subagent stats per session; 1000 telemetry sessions (evict oldest `UpdatedAt`) | Same defensive posture as `maxPlanRevisions`/`EventBufferCapacity` |
| <a id="d9"></a>D9 | Where setup writes telemetry config | Claude Code reads env from `~/.claude/settings.json` `env` block | New interactive step writes `CLAUDE_CODE_ENABLE_TELEMETRY`, `OTEL_METRICS_EXPORTER`, `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_EXPORTER_OTLP_ENDPOINT` (+ `OTEL_EXPORTER_OTLP_HEADERS` when a control token is entered); merge-preserving like `writeConfig` | Native Claude Code mechanism; per-user, reversible |
| <a id="d10"></a>D10 | No smoke-test tool discoverable (no runbook dir, no smoke Makefile target) | `make serve-http` exposes JSON-RPC at /mcp | Runbook = curl JSON-RPC request files under `plans/deep_analysis/runbooks/` | Only callable surface the repo ships; requests usable out of the box |
| <a id="d11"></a>D11 | Flag name for attribution output | `session_events` args are flat booleans | `breakdown` (bool, default false) adds `skills` and `subagents` arrays; aggregation always computed (cheap, streaming), only output is gated | Mirrors the existing `revisions` gate pattern |

## Changes

### Phase 1 — session time and idle time (DR1)

**1.1 Session start/idle fields and fold** (modified)
location: [session/session.go](session/session.go)

```diff
 const EventBufferCapacity = 500
+
+const idleThreshold = 5 * time.Minute
```

```diff
 type Session struct {
 	planExitSeen bool
 
 	Agent           Agent           `json:"agent"`
 	Counters        Counters        `json:"-"`
 	// ...
 	LastActive      time.Time       `json:"last_active"`
+	Idle            time.Duration   `json:"-"`
 	Meta            Meta            `json:"meta"`
 	// ...
+	StartedAt       time.Time       `json:"-"`
 	Title           string          `json:"title,omitempty"`
```

```diff
 func (s *Session) AddTurn(nextTurn *Turn) {
 	// always update meta info
 	s.Meta.Update(nextTurn.Meta)
 
 	if !nextTurn.Timestamp.IsZero() {
+		if s.StartedAt.IsZero() {
+			s.StartedAt = nextTurn.Timestamp
+		}
+		if gap := nextTurn.Timestamp.Sub(s.LastActive); !s.LastActive.IsZero() && gap >= idleThreshold {
+			s.Idle += gap
+		}
 		s.LastActive = nextTurn.Timestamp
 	}
```

**1.2 Time view on session_events** (modified)
location: [tools/viewmodels_events.go](tools/viewmodels_events.go)
mirrors: `planRevisionsView` ([tools/viewmodels_events.go:21](tools/viewmodels_events.go))

```go
type sessionTimeView struct {
	StartedAt     time.Time          `json:"started_at"`
	LastActive    time.Time          `json:"last_active"`
	WallSeconds   int                `json:"wall_seconds"`
	IdleSeconds   int                `json:"idle_seconds"`
	ActiveSeconds int                `json:"active_seconds"`
	Telemetry     *telemetryTimeView `json:"telemetry,omitempty"`
}

func newSessionTimeView(currentSession *session.Session) *sessionTimeView {
	if currentSession.StartedAt.IsZero() {
		return nil
	}

	wall := currentSession.LastActive.Sub(currentSession.StartedAt)
	idle := currentSession.Idle
	return &sessionTimeView{
		StartedAt:     currentSession.StartedAt,
		LastActive:    currentSession.LastActive,
		WallSeconds:   int(wall.Seconds()),
		IdleSeconds:   int(idle.Seconds()),
		ActiveSeconds: int((wall - idle).Seconds()),
	}
}
```

- `telemetryTimeView` is defined in Phase 5; until then the field is declared with the phase-5 type stubbed out — to keep Phase 1 independently shippable, add the `Telemetry` field only in Phase 5 (the diff there includes it); Phase 1 ships the struct without it.

```diff
 type sessionEventsResult struct {
 	Counters      *session.Counters  `json:"counters,omitempty"`
 	Diff          string             `json:"diff,omitempty"`
 	Events        json.RawMessage    `json:"events,omitempty"`
 	PlanRevisions *planRevisionsView `json:"plan_revisions,omitempty"`
 	Revisions     json.RawMessage    `json:"revisions,omitempty"`
+	Time          *sessionTimeView   `json:"time,omitempty"`
+	TouchedFiles  []*touchedFileView `json:"touched_files,omitempty"`
 	Unsupported   []string           `json:"unsupported,omitempty"`
 	Usage         *session.Usage     `json:"usage,omitempty"`
 }
```

- `TouchedFiles` lands in Phase 2; listed here once so the struct diff appears a single time.

**1.3 Handler wiring** (modified)
location: [tools/tools.go](tools/tools.go)

```diff
 func sessionEventsHandler(s *session.Store, pageStore *PageStore[*sessionEventsResult]) server.ToolHandlerFunc {
 	// ...
 		counters := currentSession.Counters
 		firstPage.Counters = &counters
 		firstPage.Diff = diffAvailability(currentSession)
 		firstPage.PlanRevisions = newPlanRevisionsView(currentSession)
+		firstPage.Time = newSessionTimeView(currentSession)
 		firstPage.Unsupported = unsupportedSignals(currentSession.Agent)
 		firstPage.Usage = currentSession.CurrentUsage()
```

- Tool description ([tools/tools.go:173](tools/tools.go)) gains: "…, session time (wall/idle/active seconds), touched files, …".

### Phase 2 — touched files (DR4)

**2.1 FileTouch model** (new)
location: [session/turn.go](session/turn.go)
mirrors: signal-field pattern on `Turn` (`PlanFilePath`, `CustomTitle`)

```go
type FileTouch struct {
	Path  string `json:"path"`
	Write bool   `json:"write"`
}
```

```diff
 type Turn struct {
 	Role         Role        `json:"role"`
 	// ...
 	Events       []*Event    `json:"-"`                    // signal payload, not serialized
+	FileTouches  []*FileTouch `json:"-"`                   // touched-file signal, not serialized
 	FilePath     string      `json:"-"`                    // transcript path, set by the watcher
```

```diff
 func (t *Turn) IsEventSignal() bool {
-	return len(t.Events) > 0 && t.Role == "" && t.PlanFilePath == "" && t.Usage == nil
+	return (len(t.Events) > 0 || len(t.FileTouches) > 0) && t.Role == "" && t.PlanFilePath == "" && t.Usage == nil
 }
```

**2.2 Parser extraction** (modified)
location: [claude/parser.go](claude/parser.go)

```diff
 const (
 	toolNameAgent           = "Agent"
 	toolNameAskUserQuestion = "AskUserQuestion"
 	toolNameExitPlanMode    = "ExitPlanMode"
+	toolNameEdit            = "Edit"
+	toolNameMultiEdit       = "MultiEdit"
+	toolNameNotebookEdit    = "NotebookEdit"
+	toolNameRead            = "Read"
 	toolNameSkill           = "Skill"
+	toolNameWrite           = "Write"
 )
```

```go
type fileToolInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
}

func fileTouchFromResult(block *ContentBlock, pending *pendingToolUse) *session.FileTouch {
	if block.IsError {
		return nil
	}

	var isWrite bool
	switch pending.name {
	case toolNameEdit, toolNameMultiEdit, toolNameNotebookEdit, toolNameWrite:
		isWrite = true
	case toolNameRead:
	default:
		return nil
	}

	var input fileToolInput
	if err := json.Unmarshal(pending.input, &input); err != nil {
		return nil
	}

	path := input.FilePath
	if path == "" {
		path = input.NotebookPath
	}
	if path == "" {
		return nil
	}

	return &session.FileTouch{Path: path, Write: isWrite}
}
```

```diff
-func (p *Parser) eventsFromUserContent(entry *Entry, message *Message) []*session.Event {
+func (p *Parser) eventsFromUserContent(entry *Entry, message *Message) ([]*session.Event, []*session.FileTouch) {
 	blocks := contentBlocks(message.Content)
 
 	events := make([]*session.Event, 0)
+	touches := make([]*session.FileTouch, 0)
 	for index := range blocks {
 		block := &blocks[index]
 		if block.Type != contentTypeToolResult {
 			continue
 		}
 
 		pending, ok := p.pendingTools[block.ToolUseId]
 		if !ok {
 			continue
 		}
 
 		delete(p.pendingTools, block.ToolUseId)
 
+		if touch := fileTouchFromResult(block, pending); touch != nil {
+			touches = append(touches, touch)
+		}
+
 		event := toolResultEvent(block, entry, pending)
 		if event != nil {
 			events = append(events, event)
 		}
 	}
 
-	if len(events) == 0 {
-		return nil
-	}
-
-	return events
+	return events, touches
 }
```

- Callers updated: `handleUser` ([claude/parser.go:101](claude/parser.go)) and `handleSidechain` ([claude/parser.go:229](claude/parser.go)) receive `(events, touches)`; both attach `touches` to the turn they build; `eventTurn` gains a `touches` parameter and returns non-nil when either slice is non-empty:

```diff
-func eventTurn(entry *Entry, events []*session.Event) *session.Turn {
-	if len(events) == 0 {
+func eventTurn(entry *Entry, events []*session.Event, touches []*session.FileTouch) *session.Turn {
+	if len(events) == 0 && len(touches) == 0 {
 		return nil
 	}
 
 	turn := &session.Turn{
 		Events: events,
+		FileTouches: touches,
 		Meta: &session.Meta{
 			SessionId: entry.SessionId,
 			CWD:       entry.CurrentWorkingDir,
 		},
 	}
 	return turn
 }
```

**2.3 Session aggregation** (modified)
location: [session/session.go](session/session.go)
mirrors: `AddEvent` counter fold ([session/session.go:67](session/session.go))

```go
const maxTouchedFiles = 500

type FileTouchCounts struct {
	Reads  int `json:"reads"`
	Writes int `json:"writes"`
}

func (s *Session) AddFileTouch(touch *FileTouch) {
	if s.TouchedFiles == nil {
		s.TouchedFiles = make(map[string]*FileTouchCounts)
	}

	counts, ok := s.TouchedFiles[touch.Path]
	if !ok {
		if len(s.TouchedFiles) >= maxTouchedFiles {
			return
		}
		counts = &FileTouchCounts{}
		s.TouchedFiles[touch.Path] = counts
	}

	if touch.Write {
		counts.Writes++
		return
	}
	counts.Reads++
}
```

```diff
 type Session struct {
 	// ...
 	Title           string          `json:"title,omitempty"`
 	TitleSource     TitleSource     `json:"title_source,omitempty"`
 	TotalUsage      Usage           `json:"total_usage"`
+	TouchedFiles    map[string]*FileTouchCounts `json:"-"`
 	TurnActive      *Turn           `json:"-"`
```

**2.4 Store fold** (modified)
location: [session/store.go](session/store.go)

```diff
 	for _, event := range turn.Events {
 		s.appendEvent(session, event)
 	}
 
+	for _, touch := range turn.FileTouches {
+		session.AddFileTouch(touch)
+	}
+
 	// update only plan content
 	if turn.PlanFilePath != "" {
```

**2.5 Touched-files view** (new)
location: [tools/viewmodels_events.go](tools/viewmodels_events.go)
mirrors: `newPlanRevisionsView` ([tools/tools.go:559](tools/tools.go))

```go
type touchedFileView struct {
	Path   string `json:"path"`
	Reads  int    `json:"reads,omitempty"`
	Writes int    `json:"writes,omitempty"`
}

func newTouchedFileViews(currentSession *session.Session) []*touchedFileView {
	if len(currentSession.TouchedFiles) == 0 {
		return nil
	}

	views := make([]*touchedFileView, 0, len(currentSession.TouchedFiles))
	for path, counts := range currentSession.TouchedFiles {
		views = append(views, &touchedFileView{Path: path, Reads: counts.Reads, Writes: counts.Writes})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Path < views[j].Path })
	return views
}
```

- Handler sets `firstPage.TouchedFiles = newTouchedFileViews(currentSession)` next to the Phase-1 line.
- `unsupportedSignals` ([tools/tools.go:571](tools/tools.go)) gains `"touched_files"` for Codex.

### Phase 3 — subagent time and token usage (DR3)

**3.1 SubagentId on Turn, sidechain stops discarding** (modified)
location: [session/turn.go](session/turn.go), [claude/parser.go](claude/parser.go)

```diff
 type Turn struct {
 	// ...
 	CustomTitle  string      `json:"-"`                    // title signal only, not serialized
+	SubagentId   string      `json:"-"`                    // subagent signal: routes fold to per-agent stats
 	TitleSource  TitleSource `json:"-"`
 }
```

- `Turn.Validate` gains a branch (before the event-signal branch): `SubagentId != ""` requires `Meta.SessionId`, validates `Usage` when present, and returns — mirrors the usage-signal branch ([session/turn.go:81](session/turn.go)).
- `IsSubagentSignal` and its event-only contract are deleted ([D6](#d6)).

```diff
 func (p *Parser) handleSidechain(entry *Entry) *session.Turn {
 	if entry.AgentId == "" {
 		return nil
 	}
 
 	var message Message
 	if err := json.Unmarshal(entry.Message, &message); err != nil {
 		return nil
 	}
 
 	var events []*session.Event
+	var touches []*session.FileTouch
+	var usage *session.Usage
 	switch entry.Type {
 	case EntryTypeUser:
-		events = p.eventsFromUserContent(entry, &message)
+		events, touches = p.eventsFromUserContent(entry, &message)
 	case EntryTypeAssistant:
 		events = p.eventsFromAssistantContent(entry, &message)
+		if message.Usage != nil {
+			usage = &session.Usage{
+				InputTokens:              message.Usage.InputTokens,
+				OutputTokens:             message.Usage.OutputTokens,
+				CacheCreationInputTokens: message.Usage.CacheCreationInputTokens,
+				CacheReadInputTokens:     message.Usage.CacheReadInputTokens,
+			}
+		}
 	}
 
-	return eventTurn(entry, events)
+	return &session.Turn{
+		Events:      events,
+		FileTouches: touches,
+		RequestId:   entry.RequestId,
+		SubagentId:  entry.AgentId,
+		Timestamp:   entry.Timestamp,
+		Usage:       usage,
+		Meta: &session.Meta{
+			SessionId: entry.SessionId,
+			CWD:       entry.CurrentWorkingDir,
+		},
+	}
 }
```

**3.2 Watcher meta turn joins the route** (modified)
location: [watcher/watcher.go](watcher/watcher.go)

```diff
 	turn := &session.Turn{
 		Events: []*session.Event{event},
+		SubagentId: agentId,
 		Meta:   &session.Meta{SessionId: sessionId},
 	}
```

**3.3 Session per-agent stats** (new)
location: [session/session.go](session/session.go)
mirrors: `AddTurn` usage-dedup branch ([session/session.go:94](session/session.go))

```go
const maxSubagentStats = 200

type SubagentStat struct {
	AgentType   string    `json:"agent_type,omitempty"`
	Description string    `json:"description,omitempty"`
	FirstActive time.Time `json:"first_active"`
	LastActive  time.Time `json:"last_active"`
	Usage       Usage     `json:"usage"`
}

func (s *Session) AddSubagentTurn(turn *Turn) {
	if s.Subagents == nil {
		s.Subagents = make(map[string]*SubagentStat)
	}

	stat, ok := s.Subagents[turn.SubagentId]
	if !ok {
		if len(s.Subagents) >= maxSubagentStats {
			return
		}
		stat = &SubagentStat{}
		s.Subagents[turn.SubagentId] = stat
	}

	if !turn.Timestamp.IsZero() {
		if stat.FirstActive.IsZero() {
			stat.FirstActive = turn.Timestamp
		}
		stat.LastActive = turn.Timestamp
	}

	for _, event := range turn.Events {
		if event.Kind == EventKindSubagentSpawned && event.Subagent != nil {
			stat.AgentType = event.Subagent.AgentType
			stat.Description = event.Subagent.Description
		}
	}

	if turn.Usage == nil || turn.RequestId == "" {
		return
	}
	if s.usageRequestIds == nil {
		s.usageRequestIds = make(map[string]struct{})
	}
	if _, counted := s.usageRequestIds[turn.RequestId]; counted {
		return
	}
	s.usageRequestIds[turn.RequestId] = struct{}{}
	stat.Usage.Add(turn.Usage)
}
```

```diff
 type Session struct {
 	// ...
 	StartedAt       time.Time       `json:"-"`
+	Subagents       map[string]*SubagentStat `json:"-"`
 	Title           string          `json:"title,omitempty"`
```

**3.4 Store routing, old route disposed** (modified)
location: [session/store.go](session/store.go)

```diff
 func (s *Store) AddTurnBySessionId(id Id, agent Agent, turn *Turn) {
-	if turn.IsSubagentSignal() {
-		s.addSubagentEvents(id, turn)
+	if turn.SubagentId != "" {
+		s.addSubagentTurn(id, turn)
 		return
 	}
```

```diff
-func (s *Store) addSubagentEvents(id Id, turn *Turn) {
+func (s *Store) addSubagentTurn(id Id, turn *Turn) {
 	s.mu.Lock()
 	defer s.mu.Unlock()
 
 	session, ok := s.sessions[id]
 	if !ok {
-		slog.Debug("Store.addSubagentEvents: Unknown parent session, dropping events", "session", id)
+		slog.Debug("Store.addSubagentTurn: Unknown parent session, dropping turn", "session", id)
 		return
 	}
 
 	for _, event := range turn.Events {
 		s.appendEvent(session, event)
 	}
+
+	for _, touch := range turn.FileTouches {
+		session.AddFileTouch(touch)
+	}
+
+	session.AddSubagentTurn(turn)
 }
```

- Disposal: `IsSubagentSignal` deleted from [session/turn.go:28](session/turn.go); no other production caller (sweep below).

**3.5 Breakdown flag and subagent view** (modified)
location: [tools/tools.go](tools/tools.go), [tools/viewmodels_events.go](tools/viewmodels_events.go)
mirrors: `revisions` flag gate ([tools/tools.go:498](tools/tools.go))

```diff
 		mcp.WithBoolean("revisions",
 			mcp.Description("Include plan revision diffs (default false; they dominate response size)"),
 		),
+		mcp.WithBoolean("breakdown",
+			mcp.Description("Include per-skill and per-subagent time and token usage (default false; Claude sessions only)"),
+		),
```

```go
type subagentStatView struct {
	AgentId     string         `json:"agent_id"`
	AgentType   string         `json:"agent_type,omitempty"`
	Description string         `json:"description,omitempty"`
	StartedAt   time.Time      `json:"started_at"`
	LastActive  time.Time      `json:"last_active"`
	Seconds     int            `json:"seconds"`
	Usage       *session.Usage `json:"usage,omitempty"`
}

func newSubagentStatViews(currentSession *session.Session) []*subagentStatView {
	if len(currentSession.Subagents) == 0 {
		return nil
	}

	views := make([]*subagentStatView, 0, len(currentSession.Subagents))
	for agentId, stat := range currentSession.Subagents {
		usage := stat.Usage
		views = append(views, &subagentStatView{
			AgentId:     agentId,
			AgentType:   stat.AgentType,
			Description: stat.Description,
			StartedAt:   stat.FirstActive,
			LastActive:  stat.LastActive,
			Seconds:     int(stat.LastActive.Sub(stat.FirstActive).Seconds()),
			Usage:       &usage,
		})
	}
	sort.Slice(views, func(i, j int) bool { return views[i].StartedAt.Before(views[j].StartedAt) })
	return views
}
```

```diff
 type sessionEventsResult struct {
 	// ...
+	Skills        []*skillStatView    `json:"skills,omitempty"`
+	Subagents     []*subagentStatView `json:"subagents,omitempty"`
 	Time          *sessionTimeView   `json:"time,omitempty"`
```

```diff
 		firstPage.Time = newSessionTimeView(currentSession)
+		if boolArgFromRequest("breakdown", request) {
+			firstPage.Skills = newSkillStatViews(currentSession)
+			firstPage.Subagents = newSubagentStatViews(currentSession)
+		}
```

- `unsupportedSignals` gains `"skill_usage"`, `"subagent_usage"` for Codex.

### Phase 4 — skill token usage and execution time (DR3, [D2](#d2))

**4.1 Skill windows on Session** (new)
location: [session/session.go](session/session.go)
mirrors: `AddEvent` switch ([session/session.go:70](session/session.go))

```go
const maxSkillStats = 100

type SkillStat struct {
	Skill     string    `json:"skill"`
	Args      string    `json:"args,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Usage     Usage     `json:"usage"`
}

func (s *Session) CloseSkillWindow(timestamp time.Time) {
	if s.activeSkill == nil {
		return
	}
	if !timestamp.IsZero() {
		s.activeSkill.EndedAt = timestamp
	}
	s.activeSkill = nil
}

func (s *Session) openSkillWindow(event *Event) {
	s.CloseSkillWindow(event.Timestamp)
	if len(s.Skills) >= maxSkillStats {
		return
	}

	stat := &SkillStat{
		Skill:     event.Skill.Skill,
		Args:      event.Skill.Args,
		StartedAt: event.Timestamp,
	}
	s.Skills = append(s.Skills, stat)
	s.activeSkill = stat
}
```

```diff
 type Session struct {
 	planExitSeen bool
+	activeSkill  *SkillStat
 
 	Agent           Agent           `json:"agent"`
 	// ...
+	Skills          []*SkillStat    `json:"-"`
 	StartedAt       time.Time       `json:"-"`
```

```diff
 	case EventKindSkillInvoked:
 		s.Counters.SkillsInvoked++
+		if event.Skill != nil {
+			s.openSkillWindow(event)
+		}
```

```diff
 		if _, counted := s.usageRequestIds[nextTurn.RequestId]; !counted {
 			s.usageRequestIds[nextTurn.RequestId] = struct{}{}
 			s.TotalUsage.Add(nextTurn.Usage)
+			if s.activeSkill != nil {
+				s.activeSkill.Usage.Add(nextTurn.Usage)
+				s.activeSkill.EndedAt = nextTurn.Timestamp
+			}
 		}
```

**4.2 Close on user prompt, before same-turn events open a new window** (modified)
location: [session/store.go](session/store.go)

```diff
 	if turn.FilePath != "" && session.FilePath == "" {
 		session.FilePath = turn.FilePath
 	}
 
+	if turn.Role == RoleUser && strings.TrimSpace(turn.Text) != "" {
+		session.CloseSkillWindow(turn.Timestamp)
+	}
+
 	for _, event := range turn.Events {
 		s.appendEvent(session, event)
 	}
```

- Ordering rationale: a slash-command prompt carries both the closing user text and the opening `skill_invoked` event in one turn; closing first, then folding events, yields the [D2](#d2) window.

**4.3 Skill view** (new)
location: [tools/viewmodels_events.go](tools/viewmodels_events.go)
mirrors: `newSubagentStatViews` (3.5)

```go
type skillStatView struct {
	Skill     string         `json:"skill"`
	Args      string         `json:"args,omitempty"`
	StartedAt time.Time      `json:"started_at"`
	EndedAt   time.Time      `json:"ended_at"`
	Seconds   int            `json:"seconds"`
	Usage     *session.Usage `json:"usage,omitempty"`
}

func newSkillStatViews(currentSession *session.Session) []*skillStatView {
	if len(currentSession.Skills) == 0 {
		return nil
	}

	views := make([]*skillStatView, 0, len(currentSession.Skills))
	for _, stat := range currentSession.Skills {
		ended := stat.EndedAt
		if ended.IsZero() {
			ended = currentSession.LastActive
		}
		usage := stat.Usage
		views = append(views, &skillStatView{
			Skill:     stat.Skill,
			Args:      stat.Args,
			StartedAt: stat.StartedAt,
			EndedAt:   ended,
			Seconds:   int(ended.Sub(stat.StartedAt).Seconds()),
			Usage:     &usage,
		})
	}
	return views
}
```

### Phase 5 — telemetry ingestion and setup (DR2)

**5.1 telemetry package** (new)
location: `telemetry/store.go`, `telemetry/otlp.go`, new package
mirrors: package layout of [events/broker.go](events/broker.go) (small single-purpose package)

- `store.go` — full unit in [Hot items](#hot-items) (locking class).
- `otlp.go` — OTLP/JSON decode, named types only:

```go
package telemetry

import "encoding/json"

const (
	metricActiveTime = "claude_code.active_time.total"
	metricCostUsage  = "claude_code.cost.usage"

	temporalityDelta = 1
)

type exportMetricsRequest struct {
	ResourceMetrics []resourceMetrics `json:"resourceMetrics"`
}

type resourceMetrics struct {
	ScopeMetrics []scopeMetrics `json:"scopeMetrics"`
}

type scopeMetrics struct {
	Metrics []metric `json:"metrics"`
}

type metric struct {
	Name string `json:"name"`
	Sum  *sum   `json:"sum"`
}

type sum struct {
	AggregationTemporality int         `json:"aggregationTemporality"`
	DataPoints             []dataPoint `json:"dataPoints"`
}

type dataPoint struct {
	Attributes []keyValue `json:"attributes"`
	AsDouble   *float64   `json:"asDouble"`
	AsInt      string     `json:"asInt"`
}

type keyValue struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

type anyValue struct {
	StringValue string `json:"stringValue"`
}

func (s *Store) IngestMetrics(body []byte) error {
	var request exportMetricsRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return err
	}

	for _, resource := range request.ResourceMetrics {
		for _, scope := range resource.ScopeMetrics {
			for _, m := range scope.Metrics {
				s.ingestMetric(&m)
			}
		}
	}
	return nil
}

func (s *Store) ingestMetric(m *metric) {
	if m.Sum == nil {
		return
	}
	if m.Name != metricActiveTime && m.Name != metricCostUsage {
		return
	}

	isDelta := m.Sum.AggregationTemporality == temporalityDelta
	for _, point := range m.Sum.DataPoints {
		sessionId := attributeValue(point.Attributes, "session.id")
		if sessionId == "" {
			continue
		}
		s.fold(sessionId, m.Name, pointValue(&point), isDelta)
	}
}
```

- `attributeValue` and `pointValue` (asDouble preferred, asInt parsed via `strconv.ParseInt`) are small helpers in the same file; OTLP JSON encodes int64 as a string.
- `fold` (on `Store`, see Hot items) adds deltas and keep-maxes cumulative values per session.

**5.2 Control route** (modified)
location: [control/server.go](control/server.go), handler in new `control/otlp.go`
mirrors: `respond.go` handler style

```diff
 type Options struct {
 	Store   *session.Store
 	Broker  *events.Broker
+	Telemetry *telemetry.Store
 	Token   string
 	Version string
 	Depth   int
 }
```

- `Server` gains the matching `telemetry` field, set in `New`.

```diff
 	mux.HandleFunc("GET /api/events", s.handleEvents)
+	mux.HandleFunc("POST /otlp/v1/metrics", s.handleOtlpMetrics)
 	return s.logRequests(s.checkHost(s.auth(mux)))
```

```go
const maxOtlpBodyBytes = 4 << 20

func (s *Server) handleOtlpMetrics(w http.ResponseWriter, r *http.Request) {
	if s.telemetry == nil {
		http.Error(w, "telemetry disabled", http.StatusNotFound)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "only application/json (OTLP http/json) is supported", http.StatusUnsupportedMediaType)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxOtlpBodyBytes))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}

	if err := s.telemetry.IngestMetrics(body); err != nil {
		http.Error(w, "invalid OTLP payload", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{}"))
}
```

- Auth: existing bearer middleware covers the route; exporters authenticate via `OTEL_EXPORTER_OTLP_HEADERS`.

**5.3 Wiring** (modified)
location: [cmd/start.go](cmd/start.go), [tools/tools.go](tools/tools.go)

```diff
 		broker := events.NewBroker()
 		store := session.NewStore(depth, broker, agents...)
+
+		var telemetryStore *telemetry.Store
+		if controlPort > 0 {
+			telemetryStore = telemetry.NewStore()
+		}
```

```diff
-		tools.Register(srv, store)
+		tools.Register(srv, store, telemetryStore)
```

```diff
 			controlServer, err := control.New(&control.Options{
 				Store:   store,
 				Broker:  broker,
+				Telemetry: telemetryStore,
 				Token:   controlToken,
```

```diff
-func Register(server *server.MCPServer, store *session.Store) {
+func Register(server *server.MCPServer, store *session.Store, telemetryStore *telemetry.Store) {
```

- `sessionEventsHandler` gains the store parameter and fills the time view:

```go
type telemetryTimeView struct {
	ActiveSeconds int     `json:"active_seconds"`
	CostUSD       float64 `json:"cost_usd,omitempty"`
}
```

```diff
 		firstPage.Time = newSessionTimeView(currentSession)
+		if telemetryStore != nil && firstPage.Time != nil {
+			if stats, ok := telemetryStore.Get(string(currentSession.Meta.SessionId)); ok {
+				firstPage.Time.Telemetry = &telemetryTimeView{
+					ActiveSeconds: int(stats.ActiveSeconds),
+					CostUSD:       stats.CostUSD,
+				}
+			}
+		}
```

- `unsupportedSignals` gains `"telemetry"` for Codex.

**5.4 Setup step** (new)
location: [cmd/setup.go](cmd/setup.go)
mirrors: `setupClaudeCode` ([cmd/setup.go:62](cmd/setup.go))

```go
func setupTelemetry(p *prompter) error {
	fmt.Println("Enabling Claude Code telemetry export to peek-mcp...")

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	path := filepath.Join(home, ".claude", "settings.json")
	fmt.Printf("  Config: %s\n", path)

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	cfg := map[string]any{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("%s contains invalid JSON: %w", path, err)
		}
	}

	env, _ := cfg["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	if _, exists := env["CLAUDE_CODE_ENABLE_TELEMETRY"]; exists {
		if !p.Confirm("  Telemetry is already configured. Overwrite?", false) {
			fmt.Println("  Skipped.")
			return nil
		}
	}

	port := p.Ask("  Control server port", "42422")
	token := p.Ask("  Control server token (empty for none)", "")

	env["CLAUDE_CODE_ENABLE_TELEMETRY"] = "1"
	env["OTEL_METRICS_EXPORTER"] = "otlp"
	env["OTEL_EXPORTER_OTLP_PROTOCOL"] = "http/json"
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = fmt.Sprintf("http://127.0.0.1:%s/otlp", port)
	if token != "" {
		env["OTEL_EXPORTER_OTLP_HEADERS"] = "Authorization=Bearer " + token
	} else {
		delete(env, "OTEL_EXPORTER_OTLP_HEADERS")
	}
	cfg["env"] = env

	if !p.Confirm("  Write telemetry config?", true) {
		fmt.Println("  Skipped.")
		return nil
	}
	if err := writeConfig(path, cfg); err != nil {
		return err
	}
	fmt.Println("  ✓ Wrote telemetry config.")
	return nil
}
```

- Registered in `runSetup`: the Claude Code choice becomes `[]setupFn{setupClaudeCode, setupTelemetry}` and "All" appends it after `setupClaudeCode`.
- If `prompter` has no `Ask(label, default string) string` helper (check [cmd/prompt.go](cmd/prompt.go)), add one mirroring `Confirm`; OTLP endpoint spec appends `/v1/metrics` to the configured base, matching the route in 5.2.

### Phase 6 — docs

**6.1 README** (modified)
location: [README.md](README.md)

- `session_events` row (README.md:100): mention time block, touched files, `breakdown` flag.
- Parity note (README.md:312): Codex additionally omits touched files, skill/subagent usage, telemetry.
- New "Telemetry" section: what the receiver ingests, the setup step, the env block written, manual configuration for non-default ports/tokens.

## Hot items

**telemetry.Store — locking (hot-items.md class 2)** — complete example implementation for approval:

```go
package telemetry

import (
	"sync"
	"time"
)

const maxSessions = 1000

type SessionStats struct {
	ActiveSeconds float64
	CostUSD       float64
	UpdatedAt     time.Time
}

type Store struct {
	mu       sync.RWMutex
	now      func() time.Time
	sessions map[string]*SessionStats
}

func NewStore() *Store {
	return &Store{
		now:      time.Now,
		sessions: make(map[string]*SessionStats),
	}
}

func (s *Store) Get(sessionId string) (SessionStats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats, ok := s.sessions[sessionId]
	if !ok {
		return SessionStats{}, false
	}
	return *stats, true
}

func (s *Store) fold(sessionId, metricName string, value float64, isDelta bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats, ok := s.sessions[sessionId]
	if !ok {
		if len(s.sessions) >= maxSessions {
			s.evictOldest()
		}
		stats = &SessionStats{}
		s.sessions[sessionId] = stats
	}
	stats.UpdatedAt = s.now()

	switch metricName {
	case metricActiveTime:
		stats.ActiveSeconds = foldValue(stats.ActiveSeconds, value, isDelta)
	case metricCostUsage:
		stats.CostUSD = foldValue(stats.CostUSD, value, isDelta)
	}
}

func (s *Store) evictOldest() {
	var oldestId string
	var oldest time.Time
	for id, stats := range s.sessions {
		if oldestId == "" || stats.UpdatedAt.Before(oldest) {
			oldestId = id
			oldest = stats.UpdatedAt
		}
	}
	delete(s.sessions, oldestId)
}

func foldValue(current, incoming float64, isDelta bool) float64 {
	if isDelta {
		return current + incoming
	}
	return max(current, incoming)
}
```

- No other hot classes touched: no new goroutines (HTTP handlers run on the server's), no new interfaces/generics, no anonymous structs, no weakened guard logic. Session aggregates mutate under the existing store lock, same as `TotalUsage`.

## Tests

| Location.Method | Cases | Comment |
|---|---|---|
| session/session_time_test.go `Session.AddTurn` | StartedAt set once from first timestamped turn<br>gap ≥ 5 min counted as idle<br>gap < 5 min not counted<br>zero-timestamp turns ignored | new file, mirrors session_events_test.go style |
| session/session_events_test.go `Session.AddFileTouch` | read and write counts per path<br>cap: 501st distinct path dropped, existing paths still counted | extend existing file |
| session/session_events_test.go `Session skill windows` | skill event opens window, user prompt closes<br>usage dedup attributes each requestId once<br>second skill closes first<br>open window keeps EndedAt zero | |
| session/session_events_test.go `Session.AddSubagentTurn` | usage summed with requestId dedup<br>FirstActive/LastActive span<br>AgentType/Description captured from spawn event<br>usage never enters TotalUsage | |
| session/store_events_test.go `Store.AddTurnBySessionId` | SubagentId turn routed, unknown parent dropped<br>slash-command prompt: close-then-open ordering<br>file touches folded | update: `IsSubagentSignal` cases rewritten against `SubagentId` |
| claude/parser_events_test.go `Parser.ParseLine` | Write/Edit/Read results yield touches with correct Write flag<br>notebook_path fallback<br>denied result yields no touch<br>sidechain assistant turn carries SubagentId/Usage/RequestId/Timestamp | fixtures inline JSONL as today |
| telemetry/otlp_test.go `Store.IngestMetrics` | active_time delta summed<br>cumulative keep-max<br>cost folded<br>missing session.id skipped<br>unknown metric ignored<br>asInt string parsed<br>malformed JSON errors | new package tests |
| telemetry/store_test.go `Store.Get / fold` | miss returns false<br>eviction at cap removes oldest | |
| control/api_test.go `handleOtlpMetrics` | 200 with `{}` on valid payload<br>401 without token when token set<br>415 on protobuf content type<br>404 when telemetry nil | extend existing file |
| tools/viewmodels_events_test.go `newSessionTimeView / newTouchedFileViews / newSkillStatViews / newSubagentStatViews` | wall/idle/active math<br>nil when no StartedAt<br>touched files sorted by path<br>open skill window ends at LastActive | extend existing file |

- Not tested: `setupTelemetry` interactive flow beyond the config-merge helper (prompter is terminal-bound; merge logic covered via `writeConfig`-level assertions if a seam exists, else N/A like the existing setup steps, which have no tests).

## Test runbook

Per [D10](#d10): curl JSON-RPC against `make serve-http` (port 4242). Files under `plans/deep_analysis/runbooks/`.

**Scenario 1 — time block + touched files**
location: `plans/deep_analysis/runbooks/events_time.sh`

```bash
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

**Scenario 2 — breakdown flag**
location: `plans/deep_analysis/runbooks/events_breakdown.sh`

```bash
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
      "breakdown": true,
      "json": true
    }
  }
}'
```

**Scenario 3 — OTLP receiver**
location: `plans/deep_analysis/runbooks/otlp_metrics.sh`

```bash
curl -s -X POST http://127.0.0.1:42422/otlp/v1/metrics \
  -H 'Content-Type: application/json' \
  -d '{
  "resourceMetrics": [
    {
      "scopeMetrics": [
        {
          "metrics": [
            {
              "name": "claude_code.active_time.total",
              "sum": {
                "aggregationTemporality": 1,
                "dataPoints": [
                  {
                    "attributes": [
                      {
                        "key": "session.id",
                        "value": {
                          "stringValue": "SESSION_UUID_FROM_SCENARIO_1"
                        }
                      }
                    ],
                    "asDouble": 42.5
                  }
                ]
              }
            }
          ]
        }
      ]
    }
  ]
}'
```

- The session UUID placeholder is the runbook's only env-style substitution: take the id from scenario 1's response, then re-run scenario 1 and expect `time.telemetry.active_seconds` = 42 (85 after a second post — delta temporality).

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| `tools.Register(server, store, telemetryStore)` | [cmd/start.go:137](cmd/start.go), any test constructing it | `grep -rn "tools.Register\|Register(srv" --include="*.go" . \| grep -v vendor` → all call sites pass 3 args |
| `control.Options` + `/otlp/v1/metrics` route | [cmd/start.go:140](cmd/start.go), control tests, OTLP exporter (external) | `grep -rn "control.New\|control.Options" --include="*.go" .` → all updated; route path matches endpoint written by setup (`<base>/otlp` + `/v1/metrics`) |
| `Turn.IsSubagentSignal` removal | [session/turn.go](session/turn.go), [session/store.go:72](session/store.go), tests | `grep -rn "IsSubagentSignal\|addSubagentEvents" --include="*.go" .` → zero hits after Phase 3 |
| `eventsFromUserContent` / `eventTurn` signatures | claude parser internal callers | `grep -n "eventsFromUserContent\|eventTurn(" claude/*.go` → all two-value / three-arg |
| `session_events` JSON shape (additive) | MCP clients, README | `grep -n "session_events" README.md` → rows mention time/touched_files/breakdown; tool description updated |
| `~/.claude/settings.json` env block | Claude Code (external), setup step | manual: setup on a copy, diff shows only the five env keys; existing keys preserved |

- Per-survivor justification: `session_full`/`session_latest`/`session_get` intentionally do not expose the new blocks — deep analysis stays on `session_events` (concept decision, unchanged).

## Verification

Phase 1
- [ ] Run `make test` — all green.
- [ ] Run `make serve-http` against real `~/.claude`; scenario 1 — expect `time.started_at` before `time.last_active`, `wall_seconds` = their difference, `idle_seconds` > 0 on a session with a lunch-break gap, `active_seconds` = wall − idle.
- [ ] Call `session_events` for a Codex session — time block present.

Phase 2
- [ ] Scenario 1 on this very session — expect `touched_files` to list the files this session read/edited (e.g. `session/session.go` with reads > 0), sorted by path.
- [ ] Codex session — `touched_files` absent, `unsupported` contains `touched_files`.

Phase 3
- [ ] Scenario 2 on a session that spawned subagents — expect `subagents` rows with agent_type, seconds > 0, usage token sums; `usage` (TotalUsage) unchanged versus before the flag.
- [ ] Without `breakdown` — no `skills`/`subagents` keys.

Phase 4
- [ ] Scenario 2 on a session that invoked a skill (this session invoked `fchange`) — expect a `skills` row with started_at at the invocation, non-zero seconds and usage.
- [ ] Slash-command session: window starts at the command turn, closes at the next user prompt.

Phase 5
- [ ] Scenario 3 — expect `{}` response; scenario 1 then shows `time.telemetry.active_seconds` = 42; repeat post → 85.
- [ ] POST with protobuf content type — expect 415; with token configured and no header — expect 401.
- [ ] Run `peek-mcp setup` on a scratch HOME — settings.json gains exactly the env block; re-run declines overwrite.
- [ ] Real end-to-end: enable telemetry, start a Claude Code session, wait one export interval (60 s), scenario 1 shows telemetry block.

Phase 6
- [ ] README renders; all three `session_events` mentions consistent.

Degenerate cases
- [ ] Session with a single entry — wall 0, idle 0, no crash.
- [ ] Empty OTLP payload `{}` — 200, no state change.
- [ ] `breakdown: true` on a session with no skills/subagents — keys absent.

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| 1 | An approved signature/contract can't hold as planned | stop and report; never improvise architecture mid-edit |
| 2 | Second failed fix on the same mechanism | stop, research the actual cause, redesign; no third band-aid |
| 3 | Missing prerequisite (generated code, running infra) | run the producing step; if infrastructure is down, ask |
| 4 | Discovered work materially exceeds approved scope | ask before continuing |
| 5 | Same kind of bug found twice: in own diff → fix all in diff; pre-existing outside → report and ask | per stop-conditions.md |
| 6 | Structural obstacle tempts a new abstraction | stop and report; relocate the component instead |
| 7 | Mechanical transform (fixture regeneration, config merge) | diff result element-by-element against source before presenting; fidelity loss → stop |
| 8 | Old and new structure would coexist beyond phasing (e.g. `IsSubagentSignal` survives Phase 3) | stop and report; never leave a half-migration |
| 9 | A driver contradicts a `[USER]` decision in the originating plan | surface the conflict (raw.md non-goal already surfaced and resolved as [D4](#d4)) |
| 10 | OTLP JSON from a real Claude Code export doesn't match the decode structs (field naming/temporality) | stop, capture a real payload, adjust structs against it — never guess the wire format |

## Open questions

None — [D2](#d2)/[D3](#d3) were answered at intake; all other decisions are recorded above.

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
| 2026-08-11 | adjust: driver 3 | skill windows close on prompt boundary via `Turn.PromptId`, not on every user turn — Claude injects skill bodies/hook outputs as user turns sharing the invoking prompt's id, which closed windows after 0–2 s; `Session.HandlePromptBoundary` closes only when the prompt id changes (empty id always closes, Codex unaffected) |
