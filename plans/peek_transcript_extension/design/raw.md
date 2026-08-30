# Peek Transcript Extension — Implementation Plan

## TLDR

- Two additions to peek-mcp's transcript surface:
  - **Subagent transcripts**: a `subagent` id param on `session_get` / `session_events` returns only that agent's turns, events, and telemetry; every response always carries the list of all subagent ids.
  - **Thinking transcripts**: assistant thinking blocks are captured at parse time; an optional `thinking` param on `session_get` includes them in the output.
- Today both are dropped at parse time — subagent messages are folded to stats-only signals, thinking blocks are skipped entirely. Capture is added at ingestion; params only gate output.
- Control web UI: the Turns section gets a tab strip (main agent + one tab per subagent, one visible at a time) and renders thinking as a dimmed block above the normal text, Claude-Code-style.
- Codex subagent transcripts come along for free (their turns already carry role/text); Codex thinking has no parsed source and stays out.

## Context

- **Problem**: subagent transcripts and thinking are invisible — `handleSidechain` builds signal-only turns without role/text ([claude/parser.go:263](claude/parser.go:263)), and `extractTextBlocks` keeps only `type=="text"` blocks ([claude/parser.go:431](claude/parser.go:431)).
- **Request**: user spec (this invocation) — subagent-scoped retrieval with an always-returned id list, one-agent-at-a-time tabbed UI; optional thinking output rendered like Claude Code.
- **Prior decision**: [plans/deep_analysis/concept/subagents.md](plans/deep_analysis/concept/subagents.md) deliberately kept subagent turns out of the **parent** turn buffer; this plan keeps that — subagent turns go to per-agent buffers, never the parent's.
- **Constraint**: everything is loaded once at ingestion (watcher already tails `subagents/agent-<id>.jsonl`); no lazy disk reads at query time.

## Drivers

N/A — new route.

## Scope

- **In:**
  - **thinking capture:** parse `thinking` content blocks into `Turn.Thinking` (Claude assistant turns, main + sidechain).
  - **subagent transcript capture:** sidechain turns gain role/text/thinking; per-agent turn buffers on `SubagentStat`.
  - **tool params:** `subagent` on `session_get` and `session_events`; `thinking` on `session_get`; subagent id list always in both results.
  - **UI:** tab strip on the Turns section (main + per-subagent, one at a time); thinking rendered as a dimmed block in turn cards.
- **Out:**
  - **Codex thinking:** no reasoning-item parsing exists in `codex/`; not added.
  - **subagent plan/diff/memory:** subagent scope returns turns + events only — subagents have no plan or diff of their own.
  - **nested subagents:** `spawnDepth > 1` agents are listed flat like any other; no tree UI.
- **Not changed:**
  - **parent turn buffer:** subagent turns still never enter `TurnsFinished`.
  - **per-agent usage stats and spawn/result events:** existing folding stays as is.
- **Deferred findings:**
  - **Codex reasoning blocks:** `codex/content.go` has no thinking analog; adding one is its own feature.
  - **`unsupportedSignals`** ([tools/tools.go:423](tools/tools.go:423)) still lists Codex subagent signals as unsupported although subagent transcripts will now work for Codex — revisit wording when Codex parity is next touched.

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| Spec: "whatever we have" per subagent | We have: turns (after this change), actor-tagged events, usage/type/description stats. No per-subagent plan/diff/skills windows | [session/session.go:181](session/session.go:181) |
| Spec: "Display in UI similar to claude code, normal, thinking" | No thinking rendering exists anywhere today; net-new style | [control/templates/_turns.html:9](control/templates/_turns.html:9) |

## Current state

N/A — change-route section.

## Target state

N/A — change-route section.

## Behavior contract

N/A — change-route section.

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d1"></a>D1 | When to capture thinking | [F1!](#f1) | Always capture at ingestion into `Turn.Thinking`; the `thinking` tool param (default false) only gates output; the UI always shows it when present | Repo loads once at ingestion; gating capture would force lazy re-parsing of transcripts at query time |
| <a id="d2"></a>D2 | Where subagent turns live | [F2!](#f2), [F3!](#f3), [F12](#f12) | `SubagentStat` gains `TurnActive *Turn` + `Turns *TurnBuffer` (ring, `subagentTurnDepth = 200`); parent buffer untouched | Mirrors the existing per-agent stat container and cap pattern; ring bounds memory; keeps the concept's parent-buffer decision intact |
| <a id="d3"></a>D3 | Streaming-chunk merge for subagent turns duplicates `AddTurn` logic | [F4!](#f4) | Extract the merge/push tail of `Session.AddTurn` into one shared `appendTurn` helper used by both paths | Single source of truth for merge semantics; a copied 15-line block would drift |
| <a id="d4"></a>D4 | Finished-turn push guard is `Text != ""` — thinking-only turns would vanish | [F4!](#f4) | Guard becomes `Text != "" \|\| Thinking != ""` | Assistant turns that are pure reasoning must survive; flagged: main-path behavior change (a thinking-only turn now counts as a finished turn) |
| <a id="d5"></a>D5 | Which tools take `subagent` | [F6!](#f6), [F7](#f7) | Both: `session_get` (scoped turns + actor-filtered events) and `session_events` (actor-filtered events + only that agent's breakdown row). Exact id match; unknown id errors and names the valid ids | Spec asks for transcript *and* events/telemetry; events/telemetry live on `session_events`. Exact match is unambiguous since the id list is always returned |
| <a id="d6"></a>D6 | How "list of all subagent ids always returned" lands in the results | [F7](#f7) | `session_get` result gains `subagents` (`[]subagentRef{agent_id, agent_type, description}`), always set. `session_events` result gains `subagent_ids []string`, always set — its `subagents` JSON key is already taken by the breakdown stat views | Additive; renaming the existing breakdown field would break the tool contract |
| <a id="d7"></a>D7 | UI mechanism for one-agent-at-a-time | [F10](#f10) | Tab strip using the dormant `.tabs` CSS; each tab is an htmx `hx-get` of the turns fragment with `?subagent=<id>`, `hx-swap="outerHTML"` — the working `_usage.html` `?detail=` pattern | Reuses the in-repo one-panel-at-a-time exemplar; no JS, no new mechanism |
| <a id="d8"></a>D8 | UI thinking display: toggle vs always-on | [F1!](#f1) | Always render thinking (dimmed block above the answer text) when present; no toggle | Matches "similar to claude code, normal, thinking"; a toggle adds state for no asked-for benefit |
| <a id="d9"></a>D9 | Codex subagents | [F11](#f11) | No Codex parser change; the store-side buffer change alone surfaces Codex subagent transcripts (their turns already carry role/text + `SubagentId`) | Zero-cost parity; Codex thinking stays out (no parsed source) |
| <a id="d10"></a>D10 | String param helper | [F8](#f8) | Read `subagent` inline via `args["subagent"].(string)` — no new forms helper | Repo pattern: string args are read inline (`resolveSession`); a helper for one caller is speculative |

## Open questions

None — all decisions closed.

## Baseline (verified)

Base branch: `main` (worktree branch `claude/peek-transcript-extension-4d81d2`, clean).

| ID | Fact | Needed for | Location |
|---|---|---|---|
| <a id="f1"></a>F1! | Thinking blocks exist on disk as `{"type":"thinking","thinking":"…","signature":"…"}` (verified in a real session JSONL); `ContentBlock` has no `Thinking` field and `extractTextBlocks` keeps only `type=="text"` — thinking is dropped at the single choke point | [D1](#d1), [D8](#d8), §1, §2 | [claude/content.go:8](claude/content.go:8), [claude/parser.go:426](claude/parser.go:426) |
| <a id="f2"></a>F2! | `handleSidechain` builds a signal-only `Turn` — events, touches, usage, `SubagentId` — never `Role`/`Text` | [D2](#d2), §2 | [claude/parser.go:235](claude/parser.go:235) |
| <a id="f3"></a>F3! | Store routes `SubagentId` turns to `addSubagentTurn` → per-agent stats only; they never reach `TurnsFinished`; `Turn.Validate` already accepts subagent turns leniently | [D2](#d2), §4 | [session/store.go:71](session/store.go:71), [session/store.go:161](session/store.go:161), [session/turn.go:51](session/turn.go:51) |
| <a id="f4"></a>F4! | `Session.AddTurn` merges streaming chunks by `RequestId` (concatenating `Text`) and pushes the finished turn only when `Text != ""` | [D3](#d3), [D4](#d4), §4 | [session/session.go:266](session/session.go:266) |
| <a id="f6"></a>F6! | Every Claude sidechain event and every Codex subagent event already carries `Actor` = agent id; `resolveSubagentActor` backfills result/spawn actors | [D5](#d5), §6 | [claude/parser.go:521](claude/parser.go:521), [codex/parser.go:148](codex/parser.go:148), [session/store.go:533](session/store.go:533) |
| <a id="f11"></a>F11 | Codex subagent turns are normal role/text turns with `SubagentId = agent_nickname` set — only the store routing discards their text | [D9](#d9) | [codex/parser.go:156](codex/parser.go:156), [codex/parser.go:279](codex/parser.go:279) |
| <a id="f5"></a>F5 | Watcher already tails `<session>/subagents/agent-<id>.jsonl` + `.meta.json`; meta carries `agentType`/`description`; lines flow through `ParseLine` → `handleSidechain` | §2, §4 | [watcher/watcher.go:115](watcher/watcher.go:115), [watcher/watcher.go:223](watcher/watcher.go:223) |
| <a id="f7"></a>F7 | `Session.Subagents` is `map[string]*SubagentStat{AgentType, Description, FirstActive, LastActive, Usage}` — the id list is its keys | [D5](#d5), [D6](#d6), §4, §5 | [session/session.go:67](session/session.go:67), [session/session.go:181](session/session.go:181) |
| <a id="f8"></a>F8 | Tool params are declared with `mcp.WithString/WithBoolean` + read via `boolArgFromRequest` / inline `args["x"].(string)`; no string helper exists | [D10](#d10), §6 | [tools/tools.go:44](tools/tools.go:44), [tools/forms.go:27](tools/forms.go:27), [tools/tools.go:481](tools/tools.go:481) |
| <a id="f9"></a>F9 | `session_get` marshals `sess.Turns(n)` on both the structured and paginated paths; result struct is `sessionGetResult` | §5, §6 | [tools/tools.go:174](tools/tools.go:174), [tools/viewmodels.go:9](tools/viewmodels.go:9) |
| <a id="f10"></a>F10 | UI: `handleTurnsFragment` → `_turns.html` cards; `.tabs` CSS exists unused; `_usage.html` `?detail=` htmx swap is the working one-panel-at-a-time pattern | [D7](#d7), §7, §8 | [control/sessions.go:206](control/sessions.go:206), [control/assets/style.css:127](control/assets/style.css:127), [control/templates/_usage.html:14](control/templates/_usage.html:14) |
| <a id="f12"></a>F12 | Cap-constant pattern: `maxSubagentStats = 200`, `EventBufferCapacity = 500`; `TurnBuffer` is a ring | [D2](#d2), §4 | [session/session.go:25](session/session.go:25), [session/turn_buffer.go:6](session/turn_buffer.go:6) |

## Exemplar & reuse

| Existing | Used for |
|---|---|
| `boolArgFromRequest` / inline string-arg reads ([tools/forms.go:27](tools/forms.go:27)) | Reading `subagent` / `thinking` params |
| `breakdown` param wiring ([tools/tools.go:110](tools/tools.go:110), [tools/tools.go:354](tools/tools.go:354)) | Param-gated response assembly pattern |
| `TurnBuffer` ring ([session/turn_buffer.go](session/turn_buffer.go)) | Per-subagent turn storage |
| `_usage.html` `?detail=` htmx swap + dormant `.tabs` CSS | Subagent tab strip |
| `newSubagentStatViews` ([tools/viewmodels_events.go:184](tools/viewmodels_events.go:184)) | Shape reference for `subagentRef` |

- Without exemplar: thinking rendering in `_turns.html` — no thinking display exists anywhere; the dimmed-block style is net-new (risk signal, small).

## Changes

Dependency order. All paths relative to the repo root.

### §1 Thinking block parsing (modified)

location: `claude/content.go`, `claude/parser.go`

```diff
 type ContentBlock struct {
 	Id        string          `json:"id"`
 	Content   json.RawMessage `json:"content"`
 	Input     json.RawMessage `json:"input"`
 	IsError   bool            `json:"is_error"`
 	Name      string          `json:"name"`
 	Text      string          `json:"text"`
+	Thinking  string          `json:"thinking"`
 	ToolUseId string          `json:"tool_use_id"`
 	Type      string          `json:"type"`
 }
```

New extractor next to `extractTextBlocks` ([claude/parser.go:426](claude/parser.go:426)):

```go
func extractThinkingBlocks(raw json.RawMessage) string {
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}

	var builder strings.Builder
	for _, block := range blocks {
		if block.Type != "thinking" || block.Thinking == "" {
			continue
		}

		builder.WriteString(block.Thinking + "\n")
	}
	return builder.String()
}
```

`handleAssistant` sets the new field:

```diff
 func (p *Parser) handleAssistant(entry *Entry) *session.Turn {
 	// ...
 	text := extractTextBlocks(message.Content)
+	thinking := extractThinkingBlocks(message.Content)
 	events := p.eventsFromAssistantContent(entry, &message)
 	// ...
 	turn := &session.Turn{
 		Events:     events,
 		Role:       session.RoleAssistant,
 		Text:       text,
+		Thinking:   thinking,
 		Timestamp:  entry.Timestamp,
```

### §2 Sidechain turns carry role, text, thinking (modified)

location: `claude/parser.go`
mirrors: `handleAssistant` / `handleUser` text extraction in the same file

```diff
 func (p *Parser) handleSidechain(entry *Entry) *session.Turn {
 	// ...
 	var events []*session.Event
 	var touches []*session.FileTouch
 	var usage *session.Usage
+	var role session.Role
+	var text, thinking string
 	switch entry.Type {
 	case EntryTypeUser:
 		events, touches = p.eventsFromUserContent(entry, &message)
+		role = session.RoleUser
+		text = extractTextBlocks(message.Content)
 	case EntryTypeAssistant:
 		events = p.eventsFromAssistantContent(entry, &message)
+		role = session.RoleAssistant
+		text = extractTextBlocks(message.Content)
+		thinking = extractThinkingBlocks(message.Content)
 		if message.Usage != nil {
 			// ...
 		}
 	}

 	return &session.Turn{
 		Events:      events,
 		FileTouches: touches,
 		RequestId:   entry.RequestId,
+		Role:        role,
+		Text:        text,
+		Thinking:    thinking,
 		SubagentId:  entry.AgentId,
 		Timestamp:   entry.Timestamp,
```

- `Turn.Validate` already returns early for `SubagentId != ""` turns ([session/turn.go:51](session/turn.go:51)) — the added fields need no validation change.

### §3 Turn model (modified)

location: `session/turn.go`

```diff
 type Turn struct {
 	Role         Role         `json:"role"`
 	Text         string       `json:"text"` // may be empty
+	Thinking     string       `json:"thinking,omitempty"` // assistant reasoning, may be empty
 	Timestamp    time.Time    `json:"timestamp"`
```

### §4 Per-subagent turn buffers + shared merge helper (modified)

location: `session/session.go`

- New cap constant next to the existing ones:

```diff
 const maxSubagentStats = 200

+const subagentTurnDepth = 200
+
 const maxSkillStats = 100
```

- `SubagentStat` gains the transcript buffer:

```diff
 type SubagentStat struct {
 	AgentType   string    `json:"agent_type,omitempty"`
 	Description string    `json:"description,omitempty"`
 	FirstActive time.Time `json:"first_active"`
 	LastActive  time.Time `json:"last_active"`
+	TurnActive  *Turn     `json:"-"`
+	Turns       *TurnBuffer `json:"-"`
 	Usage       Usage     `json:"usage"`
 }
```

- Shared merge/push helper — the extracted tail of `AddTurn` (hot item, full code in [Hot items](#hot-items)).
- `Session.AddTurn` tail is replaced by the helper call:

```diff
 	if nextTurn.StopReason != "" && nextTurn.StopReason != StopReasonToolUse {
 		s.CloseSkillWindow(nextTurn.Timestamp)
 	}

-	// first turn
-	if s.TurnActive == nil {
-		s.TurnActive = nextTurn
-		return
-	}
-
-	// same turn, append text, no-op for empty text
-	if nextTurn.RequestId != "" && s.TurnActive.RequestId == nextTurn.RequestId {
-		merged := *nextTurn
-		merged.Text = s.TurnActive.Text + nextTurn.Text
-		s.TurnActive = &merged
-		return
-	}
-
-	if s.TurnActive.Text != "" {
-		s.TurnsFinished.Push(s.TurnActive)
-	}
-
-	s.TurnActive = nextTurn
+	s.TurnActive = appendTurn(s.TurnActive, s.TurnsFinished, nextTurn)
 }
```

- `AddSubagentTurn` buffers transcript turns (signal-only turns — `Role == ""` — keep flowing to stats untouched):

```diff
 func (s *Session) AddSubagentTurn(turn *Turn) {
 	// ...
 	stat, ok := s.Subagents[turn.SubagentId]
 	if !ok {
 		if len(s.Subagents) >= maxSubagentStats {
 			return
 		}
-		stat = &SubagentStat{}
+		stat = &SubagentStat{Turns: NewTurnBuffer(subagentTurnDepth)}
 		s.Subagents[turn.SubagentId] = stat
 	}
+
+	if turn.Role != "" {
+		stat.TurnActive = appendTurn(stat.TurnActive, stat.Turns, turn)
+	}

 	if !turn.Timestamp.IsZero() {
```

- New accessors mirroring `Session.Turns` ([session/session.go:303](session/session.go:303)):

```go
func (s *Session) SubagentTurns(agentId string, number int) ([]*Turn, bool) {
	stat, ok := s.Subagents[agentId]
	if !ok {
		return nil, false
	}

	if stat.TurnActive == nil {
		return stat.Turns.Last(number), true
	}

	buffer := &TurnBuffer{
		capacity: stat.Turns.capacity,
		items:    append([]*Turn{stat.TurnActive}, stat.Turns.items...),
	}

	return buffer.Last(number), true
}

func (s *Session) SubagentIds() []string {
	ids := slices.Collect(maps.Keys(s.Subagents))
	slices.Sort(ids)
	return ids
}
```

- No `session/store.go` change: `addSubagentTurn` already delegates to `AddSubagentTurn`.

### §5 Result view models (modified)

location: `tools/viewmodels.go`, `tools/viewmodels_events.go`

```diff
 type sessionGetResult struct {
 	Diff            string         `json:"diff,omitempty"`
 	DiffTarget      string         `json:"diff_target,omitempty"`
 	Events          any            `json:"events,omitempty"`
 	Memory          any            `json:"memory,omitempty"`
 	Plan            string         `json:"plan,omitempty"`
+	Subagents       []subagentRef  `json:"subagents,omitempty"`
 	TotalUsage      *session.Usage `json:"total_usage,omitempty"`
 	Turns           any            `json:"turns,omitempty"`
 	UncommittedDiff string         `json:"uncommitted_diff,omitempty"`
 }
```

New ref type + builder (in `tools/viewmodels.go`, shape mirrors `subagentStatView`):

```go
type subagentRef struct {
	AgentId     string `json:"agent_id"`
	AgentType   string `json:"agent_type,omitempty"`
	Description string `json:"description,omitempty"`
}

func newSubagentRefs(sess *session.Session) []subagentRef {
	refs := make([]subagentRef, 0, len(sess.Subagents))
	for _, id := range sess.SubagentIds() {
		stat := sess.Subagents[id]
		refs = append(refs, subagentRef{AgentId: id, AgentType: stat.AgentType, Description: stat.Description})
	}
	return refs
}
```

- `sessionEventsResult` gains `SubagentIds []string json:"subagent_ids,omitempty"` — always populated ([D6](#d6); the `subagents` key stays with the breakdown stat views).

### §6 Tool params and handlers (modified)

location: `tools/tools.go`
mirrors: `breakdown` wiring ([tools/tools.go:110](tools/tools.go:110), [tools/tools.go:354](tools/tools.go:354))

- Param declarations on `session_get`:

```go
mcp.WithString("subagent",
	mcp.Description("Subagent id: scope the response to that agent's transcript and events (plan/diff/memory are omitted). Valid ids are listed in every response's subagents field."),
),
mcp.WithBoolean("thinking",
	mcp.Description("Include assistant thinking text on turns (default false)."),
),
```

- `session_events` gains the same `subagent` param (description: scope events and breakdown to that agent).
- `sessionGetHandler` logic after section flags:

```go
subagentId, _ := args["subagent"].(string)
withThinking := boolArgFromRequest(request, "thinking", false)
```

  - When `subagentId != ""`: resolve via `sess.SubagentTurns(subagentId, n)`; unknown id → `mcp.NewToolResultError` naming the valid ids (from `sess.SubagentIds()`). Turns = the subagent's; events = `filterEventsByActor(sess.Events.All(), subagentId)`; plan/diff/uncommitted/memory forced empty.
  - Both output paths (structured + paginated) marshal `turnsForOutput(turns, withThinking)`; `result.Subagents = newSubagentRefs(sess)` always.
- New helpers in `tools/`:

```go
func turnsForOutput(turns []*session.Turn, withThinking bool) []*session.Turn {
	if withThinking {
		return turns
	}

	stripped := make([]*session.Turn, len(turns))
	for i, turn := range turns {
		copied := *turn
		copied.Thinking = ""
		stripped[i] = &copied
	}
	return stripped
}

func filterEventsByActor(all []*session.Event, actor string) []*session.Event {
	filtered := make([]*session.Event, 0)
	for _, event := range all {
		if event.Actor == actor {
			filtered = append(filtered, event)
		}
	}
	return filtered
}
```

- `sessionEventsHandler`: when `subagent` set, events are actor-filtered on both paths; `breakdown` keeps only that agent's stat view; `firstPage.SubagentIds = currentSession.SubagentIds()` always.

### §7 Control UI — turns fragment with subagent tabs (modified)

location: `control/sessions.go`
mirrors: `handleUsageFragment` detail switching ([control/sessions.go:149](control/sessions.go:149))

```diff
 type turnsData struct {
-	Id    session.Id
-	Turns []*session.Turn
+	Id       session.Id
+	Turns    []*session.Turn
+	Subagent string
+	Tabs     []subagentTab
 }
+
+type subagentTab struct {
+	Id    string
+	Label string
+}
```

```diff
 func (s *Server) handleTurnsFragment(w http.ResponseWriter, r *http.Request) {
 	id := session.Id(r.PathValue("id"))
-	data := turnsData{Id: id}
-	if !s.store.WithSession(id, func(sess *session.Session) { data.Turns = sess.Turns(tools.DefaultReturnedTurns) }) {
+	data := turnsData{Id: id, Subagent: r.URL.Query().Get("subagent")}
+	if !s.store.WithSession(id, func(sess *session.Session) {
+		for _, agentId := range sess.SubagentIds() {
+			data.Tabs = append(data.Tabs, subagentTab{Id: agentId, Label: subagentTabLabel(agentId, sess.Subagents[agentId])})
+		}
+		if data.Subagent != "" {
+			if turns, ok := sess.SubagentTurns(data.Subagent, tools.DefaultReturnedTurns); ok {
+				data.Turns = turns
+			}
+			return
+		}
+		data.Turns = sess.Turns(tools.DefaultReturnedTurns)
+	}) {
 		respondNotFound("unknown session", w)
 		return
 	}
 	s.renderFragment(w, tmplTurns, data)
 }
```

- `subagentTabLabel`: `AgentType` when set, else the first 8 runes of the id.
- An unknown `subagent` query value renders the empty state — same lenient behavior as an empty buffer (UI links only ever carry known ids).

### §8 Control UI — template and style (modified)

location: `control/templates/_turns.html`, `control/assets/style.css`

Full replacement of `_turns.html` (small file):

```html
<div hx-get="/fragments/sessions/{{.Id}}/turns{{if .Subagent}}?subagent={{.Subagent}}{{end}}" hx-trigger="peek-refresh from:body throttle:1s" hx-swap="outerHTML">
{{if .Tabs}}
<div class="tabs subtabs">
  <a hx-get="/fragments/sessions/{{.Id}}/turns" hx-target="closest div.tabs" hx-swap="none" {{if not .Subagent}}class="active"{{end}} hx-select-oob="true">main</a>
  {{$root := .}}
  {{range .Tabs}}
  <a hx-get="/fragments/sessions/{{$root.Id}}/turns?subagent={{.Id}}" {{if eq $root.Subagent .Id}}class="active"{{end}}>{{.Label}}</a>
  {{end}}
</div>
{{end}}
{{if .Turns}}
{{range .Turns}}
<div class="card card-column">
  <div class="card-row">
    <span class="badge {{if eq (printf "%s" .Role) "user"}}badge-action{{else}}badge-ok{{end}}">{{.Role}}</span>
    <span class="meta">{{ts .Timestamp}}</span>
  </div>
  {{if .Thinking}}<div class="snippet thinking"><pre>{{.Thinking}}</pre></div>{{end}}
  <div class="snippet"><pre>{{.Text}}</pre></div>
</div>
{{end}}
{{else}}
<div class="empty">No finished turns yet.</div>
{{end}}
</div>
```

- Tab links target the enclosing fragment wrapper with `hx-swap="outerHTML"` on the wrapper (exact htmx attributes finalized against the `_usage.html` pattern: `hx-target` the fragment root, swap `outerHTML` — one agent visible at a time, refresh keeps the selected tab via the query param baked into the wrapper's `hx-get`).

```css
.snippet.thinking pre { color: var(--text-2); font-style: italic; }
```

## Hot items

<a id="hot-items"></a>

- **Guard-logic change (hot class 5)** — the finished-turn push guard moves from `Session.AddTurn` into the shared helper and is widened by `Thinking` ([D3](#d3), [D4](#d4)). Example implementation for approval (`session/session.go`):

```go
// appendTurn merges streaming chunks of the same request into the active turn
// and pushes the finished turn to the buffer when a new request begins.
func appendTurn(active *Turn, buffer *TurnBuffer, next *Turn) *Turn {
	if active == nil {
		return next
	}

	if next.RequestId != "" && active.RequestId == next.RequestId {
		merged := *next
		merged.Text = active.Text + next.Text
		merged.Thinking = active.Thinking + next.Thinking
		return &merged
	}

	if active.Text != "" || active.Thinking != "" {
		buffer.Push(active)
	}

	return next
}
```

- No goroutines, no new interfaces, no SQL, no anonymous structs. Store locking is untouched — all new state lives inside `Session`, mutated under the existing `Store.mu`.
- **UI screenshot**: the tabbed Turns section and thinking block cannot be screenshotted pre-implementation (the rendering does not exist and plan mode cannot run the server). Deviation: the screenshot is captured during implementation verification and stored under `plans/peek_transcript_extension/design/ui/` before the change is considered done.

## Tests

| Location.Method | Cases | Comment |
|---|---|---|
| claude/parser_test.go `TestParseLine` (extend) | assistant entry with thinking+text blocks → `Turn.Thinking` and `Turn.Text` both set<br>thinking-only assistant entry → `Thinking` set, `Text` empty | fixture JSONL lines mirror the real block shape from F1 |
| claude/parser_test.go sidechain cases (extend) | sidechain assistant entry → `Role`, `Text`, `Thinking`, `SubagentId` all set<br>sidechain user entry → `RoleUser` + `Text` | existing sidechain tests updated for new fields |
| session/session_test.go `TestAppendTurn` (new) | first turn → becomes active<br>same `RequestId` → text and thinking concatenated<br>new request, active has text → pushed<br>thinking-only active → pushed<br>empty active → dropped | pins the widened guard ([D4](#d4)) |
| session/session_test.go `TestAddSubagentTurn` (extend) | role-carrying turn lands in `stat.Turns` via `SubagentTurns`<br>signal-only turn (no role) buffers nothing, stats still fold<br>`SubagentIds` returns sorted ids | |
| tools/tools_test.go `TestSessionGet` (extend) | `subagent` id → only that agent's turns, actor-filtered events, no plan/diff<br>unknown `subagent` → error naming valid ids<br>`subagents` list present without the param<br>`thinking` default → no thinking field in output<br>`thinking:true` → thinking present | covers structured and paginated paths |
| tools/tools_test.go `TestSessionEvents` (extend) | `subagent` → actor-filtered events, breakdown reduced to that agent<br>`subagent_ids` always present | |
| control/pages_test.go `TestTurnsFragment` (extend) | tab strip renders when subagents exist, absent otherwise<br>`?subagent=<id>` renders only that agent's turns with `class="active"` on its tab<br>thinking block markup (`snippet thinking`) present when turn has thinking | substring assertions, existing pattern |

- Not tested: htmx swap behavior in a live browser — covered by manual verification below; Go template output is asserted in `pages_test.go`.

## Test runbook

- **subagent transcript via MCP**: `session_get` with `subagent: <id>` on a real session that spawned agents (data source: this very session's transcript — it spawned two Explore agents).
- **subagent id list**: `session_get` without `subagent` — response carries `subagents` with both Explore agents.
- **thinking via MCP**: `session_get` with `thinking: true` on a session run with a thinking-capable model.
- **events scoping**: `session_events` with `subagent: <id>` — only actor-tagged events.
- **UI tabs**: control app session detail → Turns section → click a subagent tab, then back to main.

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| `session_get` result: new `subagents` field, `thinking` on turns | tool ↔ MCP callers (peek skill, docs) | grep `docs/` + `skills/peek/SKILL.md` + `README.md` for `session_get` param/field lists; update where enumerated |
| `session_events` result: new `subagent_ids` field | same | same sweep |
| `Turn` JSON: new `thinking,omitempty` | session ↔ tools ↔ control templates | additive, omitempty — no consumer breaks; sweep `tools/` + `control/` for exhaustive `Turn` field handling |
| Turns fragment URL: new `subagent` query param | control server ↔ `_turns.html` | self-contained pair, both sides in this plan |

## Verification

- [ ] Run `make test` — all packages pass.
- [ ] Run `make build-local` — builds clean.
- [ ] Start the server (`make serve-http`), run a Claude session with subagents against it; call `session_get` — expect `subagents` listing every spawned agent id.
- [ ] Call `session_get` with `subagent: <id>` — expect only that agent's turns and actor-filtered events; no plan/diff sections.
- [ ] Call `session_get` with an unknown `subagent` id — expect an error naming the valid ids.
- [ ] Call `session_get` with `thinking: true` on a thinking-model session — expect `thinking` text on assistant turns; without the flag — expect none.
- [ ] Open the control app session detail — expect a tab strip on Turns (main + per-subagent); clicking a tab shows exactly one agent's transcript; the active tab survives the SSE refresh.
- [ ] Expect thinking rendered as a dimmed italic block above the answer text in turn cards.
- [ ] Degenerate: session with zero subagents — no tab strip, `subagents` empty/omitted, everything else unchanged.
- [ ] Degenerate: subagent with only signal turns (no transcript yet) — tab renders the "No finished turns yet." empty state.
- [ ] Capture the UI screenshot into `plans/peek_transcript_extension/design/ui/` (deferred hot-item evidence).

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | Stop and report. Never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | Stop, research the actual cause, redesign. No third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | Run the producing step. If infrastructure is down, ask. Never skip validation, never start infrastructure yourself |
| S4 | Discovered work materially exceeds the approved scope | Ask before continuing |
| S5 | Same kind of bug a second time: in own diff → fix all instances now; pre-existing outside the diff | Report and ask before sweeping |
| S6 | A structural obstacle tempts a new abstraction (interface, DTO, wrapper) | Stop and report. Relocate the component, don't add indirection |
| S7 | The `appendTurn` extraction changes observable main-path turn ordering in existing tests beyond the flagged thinking-only case | Stop — the guard widening ([D4](#d4)) was approved, other behavior drift was not |
| S8 | htmx tab swap can't preserve the selected tab across SSE refresh with the planned attributes | Stop and report options — don't invent client-side JS state |

## Open questions

None.

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
