# Session usage & skills tracking — Change Plan

## TLDR

- Fix the usage-panel detail swap: one refresh root with a `?detail=` parameter replaces the `hx-preserve` + per-fragment self-refresh construction that loses clicks; the four detail fragment routes are deleted.
- Stop counting `/model` as a skill; track model changes as their own event (detected from per-turn model transitions) with a counter row and a clickable Timestamp/From/To detail table.
- Close a skill's tracking window when the agent's turn actually ends (`stop_reason != "tool_use"`), not at the next user prompt — kills the 27-minute package-commit artifact; prompt-boundary close no longer stamps the next prompt's timestamp over the last activity.
- Give the plan-revisions table meaningful columns: Phase (initial / planning / alteration) plus a line-delta, replacing the mostly-empty Alteration column (OPEN decision, recommendation below).
- Add a per-skill Cost column: single dollar value computed from the skill's token component breakdown via the existing `pricing` package; the model is captured per skill window at attribution time.

## Context

The control server's Usage section (built in [plans/control_server/design/telemetry_status_config_usage.md](plans/control_server/design/telemetry_status_config_usage.md), usage-stats addendum) shipped and was exercised in real sessions; five findings came back from usage. Originating plan checked — no `[USER]` decision conflicts: the detail-fragment routes and the Alteration column were implementation choices, not user decisions.

ACDSL: N/A — no `acdsl/registry.json` in this repo.

## Drivers

| ID | Observed | Wanted | Impact | Origin |
|----|----------|--------|--------|--------|
| F1 | Clicking a usage detail link (Skills invoked, Plan versions, …) does not swap the displayed table when another detail table is already shown | Every click swaps the detail area to the clicked table | behavioral | usage session, screenshots 2026-08-11 |
| F2 | `/model claude-fable-5` is counted as SKILL_INVOKED and appears in the skills table (with 0 tokens, 6s) | Model changes tracked as their own event kind, surfaced in usage stats; not counted as skills | behavioral | events timeline screenshot |
| F3 | Alteration column in the plan-revisions table is blank for all three revisions; meaning unclear | Column(s) that convey useful per-revision information | behavioral | usage screenshot, user question |
| F4 | package-commit shows 27m57s duration; skill windows run until the next skill/prompt, and the closing prompt's timestamp overwrites the last activity | Skill window ends when the agent's turn ends after the invocation (AgentStop semantics) | behavioral | usage screenshot |
| F5 | Skills table has Tokens but no cost | Per-skill cost estimate — one value, computed from the internal component breakdown | behavioral | usage feedback |

## Scope

- In
  - Usage-panel swap mechanism restructure (F1)
  - Model-change event kind, counter, detail table; `/model` excluded from skills (F2)
  - Plan-revisions table columns (F3)
  - Skill-window end boundary via `stop_reason` (F4)
  - Per-skill cost column (F5)
- Out
  - Any change to `/api/*` endpoints
  - Codex parser changes (Codex skill windows keep the prompt-boundary close; the boundary fix in `CloseSkillWindow` benefits both agents)
- Not changed
  - `pricing` rate table and `newCostData` component rows
  - Subagent usage attribution (subagent tokens stay out of skill windows, as today)
- Deferred findings
  - Other built-in slash commands (`/clear`, `/config`, …) still count as skills — only `model` is excluded; a general builtin filter needs a maintained list
  - `skillStatView` in the `session_events` MCP tool could also carry cost; would pull `pricing` into `tools`
  - Toggling a detail table closed by re-clicking its row (no toggle exists today either)
  - Full plan-delta presentation (per-revision diff view) — belongs to the plan display, not the usage table (per D3)

## Assumptions

| Assumption | Reality | Location |
|------------|---------|----------|
| Transcript assistant entries carry `stop_reason` | Verified on a live transcript: 142× `tool_use`, 2× `end_turn`, 3× `stop_sequence` | `~/.claude/projects/...jsonl`, checked 2026-08-11 |
| Only the final API message of an agent turn carries a non-`tool_use` stop reason | Matches observed counts; streamed duplicates share `requestId` and repeat the same message fields | [reference_claude-streaming-usage-dedup] memory |
| Assistant turns carry the model per turn | `message.model` parsed into `Turn.Meta.Model` already | [parser.go:176](claude/parser.go:176) |

## Current state

| File | Lines | Responsibility | Problem |
|------|-------|----------------|---------|
| [control/templates/_usage.html](control/templates/_usage.html) | 21 | usage panel, 4 clickable rows | `hx-preserve` detail div + self-refresh root |
| [control/templates/_usage_skills.html](control/templates/_usage_skills.html) (+ plans/denials/cost) | ~11 each | detail tables | each has its own `peek-refresh` `outerHTML` self-refresh wrapper racing the click swap |
| [control/usage.go](control/usage.go) | 196 | 4 detail fragment handlers + cost model | handlers duplicate `WithSession` ceremony 4× |
| [control/server.go:129-132](control/server.go:129) | 4 | detail fragment routes | become obsolete |
| [session/session.go:138-170](session/session.go:138) | 33 | skill windows | close only on next skill/prompt; close overwrites `EndedAt` with the closing prompt's timestamp |
| [claude/parser.go:604-623](claude/parser.go:604) | 20 | slash-command → skill event | every slash command is a skill, including `model` |
| [control/usage.go:100-127](control/usage.go:100) | 28 | plan revisions rows | `Alteration` bool, blank for initial + pre-exit revisions |

## Target state

```
session_detail.html ─ hx-trigger="load" ──▶ _usage.html (self-refreshing root, URL carries ?detail=…)
                                              ├─ counters table (rows hx-get usage?detail=X, target closest .usage-panel, outerHTML)
                                              └─ usage-detail div: {{template "_usage_<detail>.html"}} include, no own refresh
```

- **Principle: single source of truth / single refresh mechanism.** One self-refreshing element (the panel root) whose GET URL encodes the selected detail; the detail tables become passive includes. Mechanism: Go `html/template` sub-template includes + htmx `hx-get` query parameter, `hx-target="closest .usage-panel"`.
- **Principle: events as the one history mechanism.** Model changes ride the existing `Event`/`Counters` machinery (like permission denials) — no new store or parallel list. Mechanism: new `EventKind` + payload, detail table derived from `Events.All()`.
- **Principle: platform signal over heuristic.** Skill end = the transcript's own `stop_reason` turn-end signal, not inferred boundaries. Mechanism: `Message.StopReason` → `Turn.StopReason` → `CloseSkillWindow`.

## Behavior contract

- Must not change: `/api/*` responses (except the additive `model_changes` counter field), plan persistence format under state dir, skills/denials/cost table contents (columns are added, existing values unchanged), Total tokens / cache-hit computation, subagent stats.
- Intentional changes (per drivers): Skills-invoked counter no longer counts `/model` (F2); skill durations shorten to actual agent activity (F4); plans table columns change (F3); detail fragment URLs `usage/{cost,plans,skills,denials}` return 404 (F1, UI-internal contract).

## Decisions

| ID | Problem | Facts | Decision | Why |
|----|---------|-------|----------|-----|
| D1 | Click swap lost when a table is displayed | Current state rows 1–4: `hx-preserve` target + 4 self-refreshing fragments + panel self-refresh all on `peek-refresh from:body` | Single refresh root with `?detail=` param; delete `hx-preserve`, fragment wrappers, and the 4 detail routes | Removes the racing swap class entirely instead of patching one race |
| D2 | How to detect model changes | `/model` args may be empty (picker); per-turn `Meta.Model` is authoritative ([parser.go:176](claude/parser.go:176)) | Detect in `Store.AddTurnBySessionId` from `session.Meta.Model` → `turn.Meta.Model` transitions (both non-empty, different); parser drops `model` from slash-skill events | Catches picker and slash changes with the effective model id; one detection source, no dedup |
| D3 | What the plans table should display | `IsAlteration` = revision after plan-mode exit; initial revision never flagged ([store.go:196-208](session/store.go:196)) | [USER] Replace Alteration with `Phase` ∈ initial / planning / alteration, plus a `Δ` column showing a truncated line-count indicator only (counts capped at 999, shown as `999+`); full delta presentation belongs to plan display and is out of scope | Every row gets a value; the usage table is the wrong place for large deltas |
| D4 | Skill end boundary | `stop_reason` present in transcripts (Assumptions); `tool_use` continues the loop, anything else ends the agent's turn; AskUserQuestion pauses are `tool_use` so windows survive question round-trips | Close the active skill window on an assistant turn with `StopReason != "" && != "tool_use"` | Exactly the requested AgentStop semantics from data already in the transcript |
| D5 | 27m artifact also caused by close stamping | `CloseSkillWindow` overwrites `EndedAt` ([session.go:147-155](session/session.go:147)); attribution already advances `EndedAt` per turn | `CloseSkillWindow` sets `EndedAt` only when still zero | Idle time between last activity and the next prompt no longer counts |
| D6 | Which model prices a skill | Session stores only the latest model; skills can span a model switch | `SkillStat.Model` captured from the first attributed turn; fallback `sess.Meta.Model`; cost = `newCostData(...).Total` on the skill's usage | Reuses the one existing cost formula; accurate across `/model` switches |
| D7 | Disposal: detail routes/handlers | 4 routes + 4 handlers + 4 tmpl constants | Deleted; templates survive as includes; `usage_test.go` rewritten against `?detail=` | No parallel old/new path |
| D8 | Disposal: `hx-preserve` + fragment refresh wrappers | — | Deleted with the restructure | Dead once the panel is the only refresher |

## Changes

### Phase 1 — Skill window boundary (F4)

**1. Parse `stop_reason`** — [claude/message.go:10](claude/message.go:10)

```diff
 type Message struct {
 	Role    session.Role    `json:"role"`
 	Content json.RawMessage `json:"content"`
 	Model   string          `json:"model"` // optional
+	StopReason string       `json:"stop_reason"` // optional
 	Usage   *Usage          `json:"usage"` // optional
 }
```

**2. Carry it on the turn** — [session/turn.go:14](session/turn.go:14), mirrors: `PromptId` signal-field style

```diff
 	PromptId     string       `json:"-"`                    // prompt submission id, not serialized
+	StopReason   string       `json:"-"`                    // assistant turn-end signal, not serialized
 	SubagentId   string       `json:"-"`                    // subagent signal: routes fold to per-agent stats
```

**3. Set it in the parser** — [claude/parser.go:165](claude/parser.go:165), inside `handleAssistant`

```diff
 	turn := &session.Turn{
 		Events:    events,
 		Role:      session.RoleAssistant,
 		Text:      text,
 		Timestamp: entry.Timestamp,
 		RequestId: entry.RequestId,
+		StopReason: message.StopReason,
 		Usage:     usage,
```

**4. Close on turn end, keep last-activity timestamp** — [session/session.go:147](session/session.go:147) and [session/session.go:235](session/session.go:235)

```diff
+const StopReasonToolUse = "tool_use"
+
 func (s *Session) CloseSkillWindow(timestamp time.Time) {
 	if s.activeSkill == nil {
 		return
 	}
-	if !timestamp.IsZero() {
+	if s.activeSkill.EndedAt.IsZero() && !timestamp.IsZero() {
 		s.activeSkill.EndedAt = timestamp
 	}
 	s.activeSkill = nil
 }
```

```diff
 		if _, counted := s.usageRequestIds[nextTurn.RequestId]; !counted {
 			s.usageRequestIds[nextTurn.RequestId] = struct{}{}
 			s.TotalUsage.Add(nextTurn.Usage)
 			if s.activeSkill != nil {
 				s.activeSkill.Usage.Add(nextTurn.Usage)
 				s.activeSkill.EndedAt = nextTurn.Timestamp
+				if s.activeSkill.Model == "" {
+					s.activeSkill.Model = nextTurn.Meta.Model
+				}
 			}
 		}
 	}
+
+	if nextTurn.StopReason != "" && nextTurn.StopReason != StopReasonToolUse {
+		s.CloseSkillWindow(nextTurn.Timestamp)
+	}
```

(`SkillStat.Model` addition is in Phase 3 change 10; it lands together with this phase at implementation — declared once there.)

### Phase 2 — Model-change tracking (F2)

**5. Event kind, payload, counter** — [session/event.go](session/event.go), mirrors: `PermissionPayload` rows

```diff
 const (
+	EventKindModelChanged     EventKind = "model_changed"
 	EventKindPermissionDenied EventKind = "permission_denied"
```

```diff
 type Counters struct {
+	ModelChanges      int `json:"model_changes"`
 	PermissionDenials int `json:"permission_denials"`
```

```diff
 type Event struct {
 	Actor      string             `json:"actor,omitempty"`
 	Kind       EventKind          `json:"kind"`
+	Model      *ModelPayload      `json:"model,omitempty"`
 	Permission *PermissionPayload `json:"permission,omitempty"`
```

```go
type ModelPayload struct {
	From string `json:"from"`
	To   string `json:"to"`
}
```

**6. Count it** — [session/session.go:85](session/session.go:85) in `AddEvent`

```diff
 	switch event.Kind {
+	case EventKindModelChanged:
+		s.Counters.ModelChanges++
 	case EventKindPermissionDenied:
 		s.Counters.PermissionDenials++
```

**7. Detect transitions in the store** — [session/store.go:145](session/store.go:145), before `session.AddTurn(turn)`

```diff
+	if turn.Meta.Model != "" && session.Meta.Model != "" && turn.Meta.Model != session.Meta.Model {
+		event := &Event{
+			Kind:      EventKindModelChanged,
+			Model:     &ModelPayload{From: session.Meta.Model, To: turn.Meta.Model},
+			Timestamp: turn.Timestamp,
+		}
+		s.appendEvent(session, event)
+		s.publish(events.TypeEventAdded, id, agent)
+	}
+
 	// update user or assistent turn
 	session.AddTurn(turn)
```

**8. Drop `model` from slash-skill events** — [claude/parser.go:604](claude/parser.go:604)

```diff
+const builtinCommandModel = "model"
+
 func slashCommandEvent(entry *Entry, text string) *session.Event {
 	name := textBetween(commandNameCloseTag, commandNameOpenTag, text)
 	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
-	if name == "" {
+	if name == "" || name == builtinCommandModel {
 		return nil
 	}
```

**9. Event summary in the tools view** — [tools/viewmodels_events.go:260](tools/viewmodels_events.go:260), mirrors: `permissionSummary`

```diff
 	switch event.Kind {
+	case session.EventKindModelChanged:
+		summary = modelSummary(event.Model)
 	case session.EventKindPermissionDenied:
 		summary = permissionSummary(event.Permission)
```

```go
func modelSummary(payload *session.ModelPayload) string {
	if payload == nil {
		return ""
	}

	return payload.From + " -> " + payload.To
}
```

### Phase 3 — Usage panel restructure + detail content (F1, F2-UI, F3, F5)

**10. `SkillStat.Model`** — [session/session.go:130](session/session.go:130)

```diff
 type SkillStat struct {
 	Skill     string    `json:"skill"`
 	Args      string    `json:"args,omitempty"`
+	Model     string    `json:"model,omitempty"`
 	StartedAt time.Time `json:"started_at"`
```

**11. Panel template** — [control/templates/_usage.html](control/templates/_usage.html), full replacement

```html
<div class="usage-panel" hx-get="/fragments/sessions/{{.Id}}/usage{{if .Detail}}?detail={{.Detail}}{{end}}" hx-trigger="peek-refresh from:body throttle:1s" hx-swap="outerHTML">
<div class="usage-grid">
<table class="usage-table">
  <tr><th>Input tokens</th><td>{{.Usage.InputTokens}}</td></tr>
  <tr><th>Output tokens</th><td>{{.Usage.OutputTokens}}</td></tr>
  {{if .Usage.CacheCreationInputTokens}}<tr><th>Cache creation</th><td>{{.Usage.CacheCreationInputTokens}}</td></tr>{{end}}
  {{if .Usage.CacheReadInputTokens}}<tr><th>Cache read</th><td>{{.Usage.CacheReadInputTokens}}</td></tr>{{end}}
  {{if .Usage.CachedInputTokens}}<tr><th>Cached input</th><td>{{.Usage.CachedInputTokens}}</td></tr>{{end}}
  {{if .Usage.ReasoningOutputTokens}}<tr><th>Reasoning output</th><td>{{.Usage.ReasoningOutputTokens}}</td></tr>{{end}}
  {{if .CachePercent}}<tr><th>Cache hit</th><td>{{.CachePercent}}</td></tr>{{end}}
  <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage?detail=cost" hx-target="closest .usage-panel" hx-swap="outerHTML"><th>Total tokens</th><td>{{.TotalTokens}}</td></tr>
  <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage?detail=denials" hx-target="closest .usage-panel" hx-swap="outerHTML"><th>Permission denials</th><td>{{.Counters.PermissionDenials}}</td></tr>
  <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage?detail=plans" hx-target="closest .usage-panel" hx-swap="outerHTML"><th>Plan versions</th><td>{{.PlanVersions}}</td></tr>
  <tr><th>Plan rejections</th><td>{{.Counters.PlanRejections}}</td></tr>
  <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage?detail=skills" hx-target="closest .usage-panel" hx-swap="outerHTML"><th>Skills invoked</th><td>{{.Counters.SkillsInvoked}}</td></tr>
  <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage?detail=models" hx-target="closest .usage-panel" hx-swap="outerHTML"><th>Model changes</th><td>{{.Counters.ModelChanges}}</td></tr>
  <tr><th>Subagents spawned</th><td>{{.Counters.SubagentsSpawned}}</td></tr>
</table>
<div class="usage-detail">
{{if eq .Detail "cost"}}{{template "_usage_cost.html" .Cost}}{{end}}
{{if eq .Detail "denials"}}{{template "_usage_denials.html" .Denials}}{{end}}
{{if eq .Detail "models"}}{{template "_usage_models.html" .Models}}{{end}}
{{if eq .Detail "plans"}}{{template "_usage_plans.html" .Plans}}{{end}}
{{if eq .Detail "skills"}}{{template "_usage_skills.html" .Skills}}{{end}}
</div>
</div>
</div>
```

**12. One-shot outer wrapper** — [control/templates/session_detail.html:12](control/templates/session_detail.html:12)

```diff
-  <div hx-get="/fragments/sessions/{{.Summary.Id}}/usage" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
+  <div hx-get="/fragments/sessions/{{.Summary.Id}}/usage" hx-trigger="load" hx-swap="outerHTML">
```

**13. Detail templates lose their refresh wrappers.** Pattern (same for `_usage_plans.html`, `_usage_denials.html`, `_usage_cost.html`); `_usage_skills.html` in full, with the new Cost column:

```html
{{if .Skills}}
<table class="usage-table">
  <tr><th>Skill</th><th>Started</th><th>Duration</th><th>Tokens</th><th>Cost</th></tr>
  {{range .Skills}}<tr><th>{{.Skill}}</th><td>{{ts .StartedAt}}</td><td>{{.Duration}}</td><td>{{.Tokens}}</td><td>{{.Cost}}</td></tr>
  {{end}}
</table>
{{else}}
<div class="empty">No skills invoked yet.</div>
{{end}}
```

**14. New `_usage_models.html`** — mirrors: `_usage_denials.html` structure

```html
{{if .Models}}
<table class="usage-table">
  <tr><th>Timestamp</th><th>From</th><th>To</th></tr>
  {{range .Models}}<tr><td>{{ts .Timestamp}}</td><td>{{.From}}</td><th>{{.To}}</th></tr>
  {{end}}
</table>
{{else}}
<div class="empty">No model changes.</div>
{{end}}
```

**15. Plans template — Phase + Δ (per D3)** — [control/templates/_usage_plans.html](control/templates/_usage_plans.html)

```html
{{if .Versions}}
<table class="usage-table">
  <tr><th>Revision</th><th>Timestamp</th><th>Phase</th><th>Δ</th></tr>
  {{range .Versions}}<tr><th>{{.Index}}</th><td>{{ts .Timestamp}}</td><td>{{.Phase}}</td><td>{{.Delta}}</td></tr>
  {{end}}
</table>
{{else}}
<div class="empty">No plan versions yet.</div>
{{end}}
```

**16. Usage handler takes `?detail=`** — [control/sessions.go:125-153](control/sessions.go:125)

```diff
 type usageData struct {
 	Id           session.Id
 	Counters     session.Counters
 	Usage        session.Usage
 	TotalTokens  int
 	CachePercent string
 	PlanVersions int
+	Detail       string
+	Cost         *costData
+	Denials      *denialsData
+	Models       *modelsData
+	Plans        *planVersionsData
+	Skills       *skillsData
 }
```

```diff
 func (s *Server) handleUsageFragment(w http.ResponseWriter, r *http.Request) {
 	id := session.Id(r.PathValue("id"))
-	data := usageData{Id: id}
+	data := usageData{Id: id, Detail: usageDetailParam(r)}
 	if !s.store.WithSession(id, func(sess *session.Session) {
 		data.Counters = sess.Counters
 		data.Usage = *sess.CurrentUsage()
 		data.TotalTokens = displayTotalTokens(&data.Usage)
 		data.CachePercent = cachePercent(sess.Agent, &data.Usage)
 		data.PlanVersions = len(sess.PlanRevisions)
+		switch data.Detail {
+		case usageDetailCost:
+			cost := newCostData(id, sess.Agent, sess.Meta.Model, sess.CurrentUsage())
+			data.Cost = &cost
+		case usageDetailDenials:
+			data.Denials = newDenialsData(sess)
+		case usageDetailModels:
+			data.Models = newModelsData(sess)
+		case usageDetailPlans:
+			data.Plans = newPlanVersionsData(sess)
+		case usageDetailSkills:
+			data.Skills = newSkillsData(id, sess)
+		}
 	}) {
 		respondNotFound("unknown session", w)
 		return
 	}
 	s.renderFragment(w, tmplUsage, data)
 }
```

**17. Detail handlers become builders** — [control/usage.go](control/usage.go): delete `handleUsageCostFragment`, `handleUsagePlansFragment`, `handleUsageSkillsFragment`, `handleUsageDenialsFragment`; add param constants + builders. New units in full:

```go
const (
	usageDetailCost    = "cost"
	usageDetailDenials = "denials"
	usageDetailModels  = "models"
	usageDetailPlans   = "plans"
	usageDetailSkills  = "skills"
)

func usageDetailParam(r *http.Request) string {
	switch detail := r.URL.Query().Get("detail"); detail {
	case usageDetailCost, usageDetailDenials, usageDetailModels, usageDetailPlans, usageDetailSkills:
		return detail
	}
	return ""
}
```

```go
func newSkillsData(id session.Id, sess *session.Session) *skillsData {
	data := &skillsData{Id: id}
	for _, skill := range sess.Skills {
		duration := "running"
		if !skill.EndedAt.IsZero() {
			duration = skill.EndedAt.Sub(skill.StartedAt).Round(time.Second).String()
		}
		model := skill.Model
		if model == "" {
			model = sess.Meta.Model
		}
		cost := newCostData(id, sess.Agent, model, &skill.Usage)
		data.Skills = append(data.Skills, skillRow{
			Skill:     skill.Skill,
			StartedAt: skill.StartedAt,
			Duration:  duration,
			Tokens:    displayTotalTokens(&skill.Usage),
			Cost:      cost.Total,
		})
	}
	return data
}
```

```go
func newPlanVersionsData(sess *session.Session) *planVersionsData {
	data := &planVersionsData{Id: sess.Meta.SessionId}
	for _, revision := range sess.PlanRevisions {
		data.Versions = append(data.Versions, planVersionRow{
			Index:     revision.Index,
			Timestamp: revision.Timestamp,
			Phase:     revisionPhase(revision),
			Delta:     revisionDelta(revision),
		})
	}
	return data
}

func revisionPhase(revision *session.PlanRevision) string {
	if revision.Index == 0 {
		return "initial"
	}
	if revision.IsAlteration {
		return "alteration"
	}
	return "planning"
}

const maxRevisionDeltaLines = 999

func revisionDelta(revision *session.PlanRevision) string {
	if revision.Index == 0 {
		return "+" + truncatedLineCount(strings.Count(revision.Content, "\n")+1)
	}

	var added, removed int
	for line := range strings.Lines(revision.Diff) {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		case strings.HasPrefix(line, "+"):
			added++
		case strings.HasPrefix(line, "-"):
			removed++
		}
	}
	if added == 0 && removed == 0 {
		return ""
	}
	return "+" + truncatedLineCount(added) + " −" + truncatedLineCount(removed)
}

func truncatedLineCount(count int) string {
	if count > maxRevisionDeltaLines {
		return "999+"
	}
	return strconv.Itoa(count)
}
```

```go
type modelRow struct {
	Timestamp time.Time
	From      string
	To        string
}

type modelsData struct {
	Id     session.Id
	Models []modelRow
}

func newModelsData(sess *session.Session) *modelsData {
	data := &modelsData{Id: sess.Meta.SessionId}
	all := sess.Events.All()
	slices.Reverse(all)
	for _, event := range all {
		if event.Kind != session.EventKindModelChanged || event.Model == nil {
			continue
		}
		data.Models = append(data.Models, modelRow{
			Timestamp: event.Timestamp,
			From:      event.Model.From,
			To:        event.Model.To,
		})
	}
	return data
}
```

`newDenialsData(sess)` is the body of today's `handleUsageDenialsFragment` loop ([control/usage.go:178-190](control/usage.go:178)) returning `*denialsData`; `skillRow` gains `Cost string`; `planVersionRow` replaces `Alteration bool` with `Phase string` and `Delta string`.

**18. Routes and constants removed** — [control/server.go:129-132](control/server.go:129)

```diff
 	mux.HandleFunc("GET /fragments/sessions/{id}/usage", s.handleUsageFragment)
-	mux.HandleFunc("GET /fragments/sessions/{id}/usage/cost", s.handleUsageCostFragment)
-	mux.HandleFunc("GET /fragments/sessions/{id}/usage/plans", s.handleUsagePlansFragment)
-	mux.HandleFunc("GET /fragments/sessions/{id}/usage/skills", s.handleUsageSkillsFragment)
-	mux.HandleFunc("GET /fragments/sessions/{id}/usage/denials", s.handleUsageDenialsFragment)
 	mux.HandleFunc("GET /fragments/sessions/{id}/events", s.handleEventsFragment)
```

[control/sessions.go:23-26](control/sessions.go:23): `tmplUsageCost`, `tmplUsageDenials`, `tmplUsagePlans`, `tmplUsageSkills` constants deleted (templates are referenced by filename in `{{template}}` includes).

## Hot items

N/A — no SQL, goroutines, new interfaces/generics, migrations, guard-logic weakening, or anonymous structs (hot-items.md 1–6).

## Tests

| Location.Method | Cases | Comment |
|-----------------|-------|---------|
| `claude/parser_test.go` (assistant turn) | stop_reason parsed onto `Turn.StopReason`<br>missing stop_reason → empty | extend existing assistant fixture cases |
| `claude/parser_events_test.go` (slash commands) | `/model x` → no skill event<br>`/fchange notes` → skill event unchanged | |
| `session/session_test.go` (skill windows) | turn with `stop_reason: end_turn` closes window at that turn's timestamp<br>`tool_use` keeps window open<br>prompt boundary after attribution keeps last-activity `EndedAt`<br>skill with no attributed turns closed by prompt boundary gets boundary timestamp<br>`SkillStat.Model` set from first attributed turn | core F4/D5/D6 coverage |
| `session/store_test.go` (model change) | first model set → no event<br>transition A→B → one `model_changed` event + counter<br>same model repeated → no event | |
| `tools/viewmodels_events_test.go` | `model_changed` summary "from -> to" | |
| `control/usage_test.go` | `usage?detail=cost/skills/plans/models/denials` render their tables<br>invalid detail → plain panel<br>old `/usage/cost` route → 404<br>skills table shows Cost column (known model) and empty cost (unknown model)<br>plans table shows Phase initial/planning/alteration + Δ<br>Δ counts above 999 render as `999+` | rewrite of the 9 existing detail-route assertions |
| not tested: the htmx click race itself | — | browser behavior; covered by manual verification below |

## Test runbook

Tool: curl + jq shell scripts (existing convention under `plans/control_server/runbooks/`).

location: [plans/control_server/runbooks/usage_details.sh](plans/control_server/runbooks/usage_details.sh)

```bash
#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
ID=$(curl -s "$BASE/api/sessions?agent=claude" | jq -r '.sessions[0].id')
curl -s "$BASE/fragments/sessions/$ID/usage" | grep -c '<th>Model changes</th>'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=skills" | grep -cE '<th>Cost</th>|No skills invoked'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=plans" | grep -cE '<th>Phase</th>|No plan versions'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=models" | grep -cE '<th>From</th>|No model changes'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=cost" | grep -cE 'Estimate from embedded rates|No pricing'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=denials" | grep -cE '<th>Tool</th>|No permission denials'
curl -s -o /dev/null -w '%{http_code}\n' "$BASE/fragments/sessions/$ID/usage/skills"
```

Last line expects `404`. Behavior-preserving re-runs: existing [session_events.sh](plans/control_server/runbooks/session_events.sh) still passes (usage fragment grep + `/api` counters).

## Contracts & sweeps

| Contract | Sides | Sweep |
|----------|-------|-------|
| Fragment URLs `usage/{cost,plans,skills,denials}` | templates, `server.go` routes, `usage_test.go` | `grep -rn "usage/cost\|usage/plans\|usage/skills\|usage/denials" --include='*.go' --include='*.html'` → zero outside `plans/` design docs (historical, survive) |
| `tmplUsageCost/Denials/Plans/Skills` constants | `sessions.go`, `usage.go` | `grep -rn "tmplUsage[CDPS]"` → zero |
| `planVersionRow.Alteration` | `usage.go`, `_usage_plans.html`, tests | `grep -rn "Alteration" control/` → zero (session-package `IsAlteration` and `Counters.PlanAlterations` stay) |
| `Counters` JSON (additive `model_changes`) | `/api/sessions/{id}/events`, `session_events` MCP tool, docs | `grep -rn "plan_rejections" docs/` — update any counters listing found |
| `hx-preserve` / `#usage-detail-` | templates, CSS | `grep -rn "usage-detail\|hx-preserve" control/` → only the class selector in CSS/templates, no id, no preserve |

## Verification

Phase 1
- [ ] Run `go test ./claude/... ./session/...` — new boundary cases pass
- [ ] Restart `peek-mcp` against a live session, invoke a skill, let the turn finish, wait >2 min, submit a new prompt — skills table duration matches last agent activity, not the idle gap

Phase 2
- [ ] With a live session, run `/model` and switch models — events timeline shows MODEL_CHANGED with "old -> new", no SKILL_INVOKED `model` card; Skills-invoked counter unchanged
- [ ] `session_events` MCP output contains `model_changes` counter

Phase 3
- [ ] In the browser: click Skills invoked (table shows), then click Plan versions — table swaps immediately; repeat across all four detail rows in both orders while auto-refresh is running
- [ ] Detail table survives a `peek-refresh` cycle (stays selected and updates values)
- [ ] Skills table shows a `$` cost per skill on this session (fable model priced); plans table shows Phase initial/planning/alteration and Δ per revision with the real revisions from Phase-1 observation (revisions 0/1/2 at 18:36:17 / 18:37:55 / 18:53:38)
- [ ] Run [usage_details.sh](plans/control_server/runbooks/usage_details.sh) — all greps ≥1, last line 404
- [ ] `go build ./... && go test ./...` green

## Stop conditions

| ID | Condition | Action |
|----|-----------|--------|
| 1–6 | Generic `stop-conditions.md` entries 1–6 (contract can't hold; second failed fix; missing prerequisite; scope exceeded; repeated bug class; abstraction temptation) | as written there |
| 7 | Mechanical transform (template restructure) loses fidelity vs. source | diff element-by-element, stop on loss |
| 8 | Old and new detail-route mechanisms would have to coexist beyond Phase 3 | stop and report |
| 9 | A driver contradicts a `[USER]` decision in the originating plan | surface, never override |
| P1 | Transcript entries in the wild carry a stop_reason value that is neither `tool_use` nor turn-ending (e.g. streaming partials with unexpected values) breaking window tests | stop, re-verify against real transcripts before adjusting the predicate |

## Open questions

Empty — D3 answered by the user during review.

## Changelog

| Date | Trigger | What changed |
|------|---------|--------------|
| — | initial | plan created |
| 2026-08-11 | review feedback | D3 [USER]: Δ shown as truncated indicator only (999+ cap); full delta presentation deferred to plan display, out of scope |
