# UI session metrics — Change Plan

On approval this plan is persisted to `plans/control_server/design/ui_session_metrics.md`.

## TLDR

- Four gathered-but-invisible data sets get surfaced in the control-server dashboard; no parsing or model changes.
- Memory peeking (`session_get` + `remember`) is **already done and documented** on the MCP side — the remaining gap is a Memory section on the session detail page plus two doc sentences.
- Subagent time/tokens/cost become a clickable `?detail=subagents` drill-down in the usage panel; touched files become a `?detail=files` drill-down — both mirror the existing skills detail.
- Session time / idle / active appear as three plain rows at the top of the usage table, computed from fields the session model already maintains.

## Context

- Drivers come from usage of the dashboard: data exists end-to-end through the MCP layer but the web UI never shows it.
- Originating plans checked: [usage_skills_tracking.md](plans/control_server/design/usage_skills_tracking.md) (usage panel, `?detail=` selection), [raw.md](plans/control_server/design/raw.md) (dashboard), [deep_analysis concept](plans/deep_analysis/concept/concept.md) (memory: `[USER]` arg name `remember` — untouched, MCP surface unchanged). No `[USER]` decision conflicts.
- All changes live in `control/` (handlers, templates) plus docs; the exemplar throughout is the usage panel detail pattern.

## Drivers

| ID | Observed | Wanted | Impact | Origin |
|----|----------|--------|--------|--------|
| DR1 | Memory peeking is implemented ([tools.go:389](tools/tools.go:389) `memoryBlock`, [memory.go:104](claude/memory.go:104) `ReadMemory`) and documented ([docs/tools.md:28](docs/tools.md), [README.md:95](README.md)); the dashboard shows nothing memory-related | Session detail page shows the project's auto-memory; docs mention the section | behavioral | request ("Is this done? If so, expose in UI, check documentation") |
| DR2 | Per-subagent duration and usage are tracked ([session.go:177](session/session.go:177) `SubagentStat`) and served via MCP; the dashboard shows only a static "Subagents spawned" counter ([_usage.html:17](control/templates/_usage.html:17)) — the one usage row without a click target | Clicking the subagents row opens a per-subagent table: type, description, started, duration, tokens, cost | behavioral | request |
| DR3 | Touched files are gathered ([session.go:71](session/session.go:71) `TouchedFiles`, parser at [parser.go:368](claude/parser.go:368)) and served via `session_events`; zero references in `control/` | Usage panel gains a "Touched files" row with a per-path reads/writes drill-down | behavioral | request |
| DR4 | `Session.StartedAt` / `LastActive` / `Idle` are maintained ([session.go:230](session/session.go:230)) and exposed via MCP as wall/idle/active ([viewmodels_events.go:99](tools/viewmodels_events.go:99)); the dashboard shows only a `LastActive` timestamp | Usage panel shows session time, idle time, and active time | behavioral | request |

- No driver is contract-touching: every change is additive UI surface; no existing route, type, or output changes shape.

## Scope

- Opportunity menu, ranked — items 1–5 in, 6 in (small, keeps the fragment↔API 1:1 pattern), 7+ deferred:
  1. **time-rows:** session/idle/active rows in the usage table (DR4)
  2. **subagents-detail:** `?detail=subagents` drill-down (DR2)
  3. **files-detail:** `?detail=files` drill-down + "Touched files" row (DR3)
  4. **memory-section:** Memory section on the session detail page (DR1)
  5. **docs:** dashboard section lists in reference.md / README.md (DR1)
  6. **memory-api:** `GET /api/sessions/{id}/memory` JSON mirror
- **In:**
  - the six menu items above
- **Out:**
  - MCP tool surface — `session_get`/`session_events` unchanged
  - session model and parsers — no new fields, no new tracking
- **Not changed:**
  - existing usage detail routes, templates, and their tests
  - the `remember` arg contract and its docs rows
- **Deferred findings:**
  - memory type filtering (deep_analysis backlog item, [concept.md:95](plans/deep_analysis/concept/concept.md))
  - per-subagent model tracking (`SubagentStat` has no `Model`; cost estimates fall back to the session model)
  - subagent attribution of touched files (subagent touches fold into the parent unattributed, [store.go:178](session/store.go:178))
  - enriching `GET /api/sessions/{id}/events` with time/subagents/touched_files (JSON parity exists via MCP)

## Assumptions

| Assumption | Reality | Location |
|------------|---------|----------|
| "Subagent click" needs a per-subagent click target | No subagent element is clickable anywhere; the usage-panel `?detail=` row is the established click idiom, so the counter row becomes the click target | [_usage.html:11-17](control/templates/_usage.html:11) |
| Memory needs new plumbing | Reader, worktree fallback, 64 KB cap, and unsupported cases all exist in `claude.ReadMemory` — the UI only calls it | [memory.go:104](claude/memory.go:104) |

## Current state

- [control/usage.go](control/usage.go) — detail constants (15-21), param whitelist (23-29), one `new<X>Data` builder per detail (cost 78, plans 118, skills 184, models 218, denials 246)
- [control/sessions.go:121-168](control/sessions.go:121) — `usageData` + `handleUsageFragment` with the detail switch
- [control/templates/_usage.html](control/templates/_usage.html) — token rows, clickable `usage-row` rows, detail dispatch block
- [control/templates/session_detail.html](control/templates/session_detail.html) — six `<details>` sections, each an htmx fragment
- [control/server.go:124-139](control/server.go:124) — fragment routes with 1:1 `/api/` mirrors
- Duplication: none to consolidate — the change adds three more instances of the existing pattern

## Target state

- Usage table: three time rows on top, every counter row with drill-down data clickable (subagents and files join cost/denials/plans/skills/models)
- Session detail page: seventh section "Memory", load-once (no 1s refresh)
- **Principle:** single rendering pattern — every new surface is another instance of the fragment-handler + `usage-table`/`<details>` idiom; mechanism: `?detail=` whitelist constant + `new<X>Data` builder + sub-template, and for memory a fragment handler + section template. No new abstractions.

## Behavior contract

- Existing usage details, their routes, and empty states render byte-identically (pinned by [usage_test.go](control/usage_test.go)).
- Existing session-detail sections keep their triggers and order; Memory is appended last.
- MCP outputs unchanged.
- Intentional changes = the four drivers (additive display only).

## Decisions

| ID | Problem | Facts | Decision | Why |
|----|---------|-------|----------|-----|
| D1 | Where session time lives | usage table is the session-metrics surface; no drill-down data behind the numbers | Three plain (non-clickable) rows — Session time, wall = `LastActive − StartedAt`; Idle time = `Session.Idle`; Active time = wall − idle — rendered only when `StartedAt` is set | Matches MCP semantics ([viewmodels_events.go:99](tools/viewmodels_events.go:99)); a detail pane with nothing behind it would break the row idiom |
| D2 | Reuse `tools.newSessionTimeView` or compute in control | the view is unexported and returns JSON-int seconds; control needs formatted strings; the session fields are the single source both derive from | Compute inline in `handleUsageFragment` from `Session` fields, format `Round(time.Second).String()` like skill durations ([usage.go:189](control/usage.go:189)) | Two lines of arithmetic beat exporting a JSON viewmodel to reformat it |
| D3 | Subagent cost without a per-subagent model | `SubagentStat` has no `Model` field; skills fall back to `sess.Meta.Model` ([usage.go:191-193](control/usage.go:191)) | Same fallback; per-subagent model tracking deferred | Consistent estimate labeling; tracking the model is a parser change outside the drivers |
| D4 | Subagent row ordering | `Session.Subagents` is a map; MCP sorts by `FirstActive` ([viewmodels_events.go:160](tools/viewmodels_events.go:160)) | Sort by `StartedAt` ascending, mirroring the MCP view | Deterministic render; matches the JSON surface |
| D5 | Memory fragment refresh | memory reads disk on every render; other fragments re-render on `peek-refresh` every 1 s; no SSE signal exists for memory edits | `hx-trigger="load"` only (like the Usage section on page load); re-open the section or reload to refresh | 1 s disk scans for a file set that changes rarely is waste |
| D6 | Memory unavailable states | `memoryBlock` distinguishes codex / unknown path / read error ([tools.go:389-400](tools/tools.go:389)) | Mirror the same three strings as the fragment's empty state | One vocabulary across MCP and UI |
| D7 | Fact body rendering | fact files start with `---` frontmatter; markdown-rendering frontmatter produces garbage | Index rendered as markdown (`renderMarkdown`, like the plan fragment); each fact a `<details>` with name + type badge in the summary and the raw body in `<pre>` | Index is prose, facts are records; raw body keeps frontmatter legible |
| D8 | Store lock vs disk I/O | `WithSession` holds the store lock; `ReadMemory` does file I/O | Extract `Agent` + `FilePath` inside the callback, call `ReadMemory` after it returns | Never do I/O under the store lock |
| D9 | Detail param names | existing names are plain nouns (`cost`, `skills`) | `subagents` and `files` | Consistency |

## Changes

### Phase 1 — session time rows (DR4)

**usageData gains formatted time fields (modified)**
location: [control/sessions.go:121](control/sessions.go:121)

```diff
 type usageData struct {
 	Id           session.Id
 	Counters     session.Counters
 	Usage        session.Usage
 	TotalTokens  int
 	CachePercent string
 	PlanVersions int
+	SessionTime  string
+	IdleTime     string
+	ActiveTime   string
 	Detail       string
 	Cost         *costData
 	Denials      *denialsData
 	Models       *modelsData
 	Plans        *planVersionsData
 	Skills       *skillsData
 }
```

**handleUsageFragment computes the times (modified)**
location: [control/sessions.go:141](control/sessions.go:141)

```diff
 func (s *Server) handleUsageFragment(w http.ResponseWriter, r *http.Request) {
 	id := session.Id(r.PathValue("id"))
 	data := usageData{Id: id, Detail: usageDetailParam(r)}
 	if !s.store.WithSession(id, func(sess *session.Session) {
 		data.Counters = sess.Counters
 		data.Usage = *sess.CurrentUsage()
 		data.TotalTokens = displayTotalTokens(&data.Usage)
 		data.CachePercent = cachePercent(sess.Agent, &data.Usage)
 		data.PlanVersions = len(sess.PlanRevisions)
+		if !sess.StartedAt.IsZero() {
+			wall := sess.LastActive.Sub(sess.StartedAt)
+			data.SessionTime = wall.Round(time.Second).String()
+			data.IdleTime = sess.Idle.Round(time.Second).String()
+			data.ActiveTime = (wall - sess.Idle).Round(time.Second).String()
+		}
 		switch data.Detail {
```

**time rows in the usage table (modified)**
location: [control/templates/_usage.html:3](control/templates/_usage.html:3)

```diff
 <table class="usage-table">
+  {{if .SessionTime}}<tr><th>Session time</th><td>{{.SessionTime}}</td></tr>
+  <tr><th>Idle time</th><td>{{.IdleTime}}</td></tr>
+  <tr><th>Active time</th><td>{{.ActiveTime}}</td></tr>{{end}}
   <tr><th>Input tokens</th><td>{{.Usage.InputTokens}}</td></tr>
```

### Phase 2 — subagents detail (DR2)

**detail constant + whitelist (modified)**
location: [control/usage.go:15](control/usage.go:15)

```diff
 const (
-	usageDetailCost    = "cost"
-	usageDetailDenials = "denials"
-	usageDetailModels  = "models"
-	usageDetailPlans   = "plans"
-	usageDetailSkills  = "skills"
+	usageDetailCost      = "cost"
+	usageDetailDenials   = "denials"
+	usageDetailModels    = "models"
+	usageDetailPlans     = "plans"
+	usageDetailSkills    = "skills"
+	usageDetailSubagents = "subagents"
 )

 func usageDetailParam(r *http.Request) string {
 	switch detail := r.URL.Query().Get("detail"); detail {
-	case usageDetailCost, usageDetailDenials, usageDetailModels, usageDetailPlans, usageDetailSkills:
+	case usageDetailCost, usageDetailDenials, usageDetailModels, usageDetailPlans,
+		usageDetailSkills, usageDetailSubagents:
 		return detail
 	}
 	return ""
 }
```

**subagents data builder (new)**
location: [control/usage.go:205](control/usage.go:205) (after `newSkillsData`)
mirrors: `newSkillsData` at [control/usage.go:184](control/usage.go:184)

```go
type subagentRow struct {
	Agent       string
	Description string
	StartedAt   time.Time
	Duration    string
	Tokens      int
	Cost        string
}

type subagentsData struct {
	Id        session.Id
	Subagents []subagentRow
}

func newSubagentsData(id session.Id, sess *session.Session) *subagentsData {
	data := &subagentsData{Id: id}
	for _, stat := range sess.Subagents {
		cost := newCostData(id, sess.Agent, sess.Meta.Model, &stat.Usage)
		data.Subagents = append(data.Subagents, subagentRow{
			Agent:       stat.AgentType,
			Description: stat.Description,
			StartedAt:   stat.FirstActive,
			Duration:    stat.LastActive.Sub(stat.FirstActive).Round(time.Second).String(),
			Tokens:      displayTotalTokens(&stat.Usage),
			Cost:        cost.Total,
		})
	}
	slices.SortFunc(data.Subagents, func(a, b subagentRow) int { return a.StartedAt.Compare(b.StartedAt) })
	return data
}
```

**wire into usageData and the detail switch (modified)**
location: [control/sessions.go:133](control/sessions.go:133), [control/sessions.go:160](control/sessions.go:160)

```diff
 	Skills       *skillsData
+	Subagents    *subagentsData
```

```diff
 		case usageDetailSkills:
 			data.Skills = newSkillsData(id, sess)
+		case usageDetailSubagents:
+			data.Subagents = newSubagentsData(id, sess)
 		}
```

**subagents sub-template (new)**
location: `control/templates/_usage_subagents.html`
mirrors: [_usage_skills.html](control/templates/_usage_skills.html)

```html
{{if .Subagents}}
<table class="usage-table">
  <tr><th>Agent</th><th>Description</th><th>Started</th><th>Duration</th><th>Tokens</th><th>Cost</th></tr>
  {{range .Subagents}}<tr><th>{{.Agent}}</th><td>{{.Description}}</td><td>{{ts .StartedAt}}</td><td>{{.Duration}}</td><td>{{.Tokens}}</td><td>{{.Cost}}</td></tr>
  {{end}}
</table>
{{else}}
<div class="empty">No subagents spawned yet.</div>
{{end}}
```

**counter row becomes clickable + dispatch (modified)**
location: [control/templates/_usage.html:17](control/templates/_usage.html:17), [_usage.html:24](control/templates/_usage.html:24)

```diff
-  <tr><th>Subagents spawned</th><td>{{.Counters.SubagentsSpawned}}</td></tr>
+  <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage?detail=subagents" hx-target="closest .usage-panel" hx-swap="outerHTML"><th>Subagents spawned</th><td>{{.Counters.SubagentsSpawned}}</td></tr>
```

```diff
 {{if eq .Detail "skills"}}{{template "_usage_skills.html" .Skills}}{{end}}
+{{if eq .Detail "subagents"}}{{template "_usage_subagents.html" .Subagents}}{{end}}
```

### Phase 3 — touched files detail (DR3)

**detail constant + whitelist (modified)**
location: [control/usage.go:15](control/usage.go:15)

```diff
 	usageDetailSubagents = "subagents"
+	usageDetailFiles     = "files"
```

```diff
 	case usageDetailCost, usageDetailDenials, usageDetailModels, usageDetailPlans,
-		usageDetailSkills, usageDetailSubagents:
+		usageDetailSkills, usageDetailSubagents, usageDetailFiles:
```

**files data builder (new)**
location: [control/usage.go](control/usage.go) (after `newSubagentsData`)
mirrors: `newSubagentsData` (sorting), `newTouchedFileViews` semantics at [viewmodels_events.go:63](tools/viewmodels_events.go:63)

```go
type fileRow struct {
	Path   string
	Reads  int
	Writes int
}

type filesData struct {
	Id    session.Id
	Files []fileRow
}

func newFilesData(sess *session.Session) *filesData {
	data := &filesData{Id: sess.Meta.SessionId}
	for path, counts := range sess.TouchedFiles {
		data.Files = append(data.Files, fileRow{Path: path, Reads: counts.Reads, Writes: counts.Writes})
	}
	slices.SortFunc(data.Files, func(a, b fileRow) int { return strings.Compare(a.Path, b.Path) })
	return data
}
```

**wire into usageData and the detail switch (modified)**
location: [control/sessions.go](control/sessions.go)

```diff
 	Subagents    *subagentsData
+	TouchedFiles int
+	Files        *filesData
```

```diff
 		data.PlanVersions = len(sess.PlanRevisions)
+		data.TouchedFiles = len(sess.TouchedFiles)
```

```diff
 		case usageDetailSubagents:
 			data.Subagents = newSubagentsData(id, sess)
+		case usageDetailFiles:
+			data.Files = newFilesData(sess)
 		}
```

**files sub-template (new)**
location: `control/templates/_usage_files.html`
mirrors: [_usage_models.html](control/templates/_usage_models.html)

```html
{{if .Files}}
<table class="usage-table">
  <tr><th>Path</th><th>Reads</th><th>Writes</th></tr>
  {{range .Files}}<tr><th>{{.Path}}</th><td>{{.Reads}}</td><td>{{.Writes}}</td></tr>
  {{end}}
</table>
{{else}}
<div class="empty">No touched files.</div>
{{end}}
```

**new counter row + dispatch (modified)**
location: [control/templates/_usage.html:17](control/templates/_usage.html:17), dispatch block

```diff
   <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage?detail=subagents" hx-target="closest .usage-panel" hx-swap="outerHTML"><th>Subagents spawned</th><td>{{.Counters.SubagentsSpawned}}</td></tr>
+  <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage?detail=files" hx-target="closest .usage-panel" hx-swap="outerHTML"><th>Touched files</th><td>{{.TouchedFiles}}</td></tr>
```

```diff
 {{if eq .Detail "subagents"}}{{template "_usage_subagents.html" .Subagents}}{{end}}
+{{if eq .Detail "files"}}{{template "_usage_files.html" .Files}}{{end}}
```

### Phase 4 — memory section + docs (DR1)

**memory fragment handler (new)**
location: [control/sessions.go](control/sessions.go) (after `handlePlanFragment`; template constant `tmplMemory = "_memory.html"` joins the block at [sessions.go:12](control/sessions.go:12))
mirrors: `handlePlanFragment` at [control/sessions.go:197](control/sessions.go:197); unavailable strings mirror `memoryBlock` at [tools.go:389](tools/tools.go:389)

```go
type memoryFact struct {
	Name string
	Type string
	Body string
}

type memoryData struct {
	Id          session.Id
	IndexHTML   any
	Facts       []memoryFact
	Truncated   bool
	Unavailable string
}

func (s *Server) handleMemoryFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	var agent session.Agent
	var transcriptPath string
	if !s.store.WithSession(id, func(sess *session.Session) {
		agent = sess.Agent
		transcriptPath = sess.FilePath
	}) {
		respondNotFound("unknown session", w)
		return
	}

	data := memoryData{Id: id}
	switch {
	case agent != session.AgentClaude:
		data.Unavailable = "memory is not available for codex sessions"
	case transcriptPath == "":
		data.Unavailable = "transcript path unknown"
	default:
		memory, err := claude.ReadMemory(transcriptPath)
		if err != nil {
			data.Unavailable = err.Error()
			break
		}
		data.Truncated = memory.IsTruncated
		if memory.Index != "" {
			html, err := renderMarkdown([]byte(memory.Index))
			if err != nil {
				respondInternalServerError(err, w)
				return
			}
			data.IndexHTML = html
		}
		for _, fact := range memory.Facts {
			data.Facts = append(data.Facts, memoryFact{Name: fact.Name, Type: fact.Type, Body: fact.Body})
		}
	}
	s.renderFragment(w, tmplMemory, data)
}
```

- New import in `control/sessions.go`:

```go
"github.com/kevinhorst/peek-mcp/claude"
```

**memory fragment template (new)**
location: `control/templates/_memory.html`
mirrors: [_plan.html](control/templates/_plan.html) (index part); facts per [D7](#decisions)

```html
<div>
{{if .Unavailable}}
<div class="empty">{{.Unavailable}}</div>
{{else}}
{{if .IndexHTML}}<div class="md-body">{{.IndexHTML}}</div>{{end}}
{{range .Facts}}
<details class="section">
  <summary>{{.Name}}{{if .Type}} <span class="badge">{{.Type}}</span>{{end}}</summary>
  <pre>{{.Body}}</pre>
</details>
{{end}}
{{if .Truncated}}<div class="meta">Truncated at 64 KiB.</div>{{end}}
{{end}}
</div>
```

- No self-refreshing `hx-get` root ([D5](#decisions)): the fragment renders once per section open.

**memory section on the detail page (modified)**
location: [control/templates/session_detail.html:45](control/templates/session_detail.html:45) (after Uncommitted diff)

```diff
 <details class="section">
   <summary>Uncommitted diff</summary>
   <div hx-get="/fragments/sessions/{{.Summary.Id}}/uncommitted-diff" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
     <div class="empty">Loading…</div>
   </div>
 </details>
+<details class="section">
+  <summary>Memory</summary>
+  <div hx-get="/fragments/sessions/{{.Summary.Id}}/memory" hx-trigger="load" hx-swap="outerHTML">
+    <div class="empty">Loading…</div>
+  </div>
+</details>
 {{template "foot" .}}
```

**routes (modified)**
location: [control/server.go:129](control/server.go:129), [server.go:139](control/server.go:139)

```diff
 	mux.HandleFunc("GET /fragments/sessions/{id}/events", s.handleEventsFragment)
+	mux.HandleFunc("GET /fragments/sessions/{id}/memory", s.handleMemoryFragment)
```

```diff
 	mux.HandleFunc("GET /api/sessions/{id}/events", s.handleSessionEvents)
+	mux.HandleFunc("GET /api/sessions/{id}/memory", s.handleMemory)
```

**memory JSON API (new)**
location: [control/api.go](control/api.go) (after `handleSessionEvents`)
mirrors: `handlePlan` at [control/api.go:138](control/api.go:138); payload mirrors `memoryBlock` at [tools.go:389](tools/tools.go:389)

```go
func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	var agent session.Agent
	var transcriptPath string
	found := s.store.WithSession(id, func(sess *session.Session) {
		agent = sess.Agent
		transcriptPath = sess.FilePath
	})
	if !found {
		respondNotFound("unknown session", w)
		return
	}
	if agent != session.AgentClaude {
		respondNotFound("memory is not available for codex sessions", w)
		return
	}
	if transcriptPath == "" {
		respondNotFound("transcript path unknown", w)
		return
	}
	memory, err := claude.ReadMemory(transcriptPath)
	if err != nil {
		respondNotFound(err.Error(), w)
		return
	}
	writeJSON(w, memory)
}
```

**docs (modified)**
location: [docs/reference.md:57](docs/reference.md:57), [README.md:103](README.md:103)

```diff
-Serves a live dashboard on `http://127.0.0.1:42442/` in both transports — session list, turns, plan, diffs, per-session usage and events update as agents work.
+Serves a live dashboard on `http://127.0.0.1:42442/` in both transports — session list, turns, plan, diffs, per-session usage (tokens, cost, session/idle time, skills, subagents, touched files) and events update as agents work; Claude sessions also show the project's auto-memory.
```

```diff
-Run with `--control-port` and peek-mcp serves a live, read-only dashboard on loopback — session list, turns, plan, and diffs, updating as agents work.
+Run with `--control-port` and peek-mcp serves a live, read-only dashboard on loopback — session list, turns, plan, diffs, usage metrics, and auto-memory, updating as agents work.
```

## Hot items

N/A — no SQL, goroutines, interfaces, generics, migrations, guard logic, or anonymous structs; all new types are named structs mirroring existing rows.

## Tests

- Safety net: [control/usage_test.go](control/usage_test.go) pins all existing details and empty states; [session/session_time_test.go](session/session_time_test.go) and [tools/viewmodels_time_test.go](tools/viewmodels_time_test.go) pin the time math; [claude/memory_test.go](claude/memory_test.go) pins the reader.
- Updated: none — all additions.

| Location.Method | Cases | Comment |
|---|---|---|
| control/usage_test.go `TestUsageTimeRows` | rows-rendered (s1 has two timestamped turns 1 min apart → `Session time` + `1m0s`)<br>no-started-at-hidden (session without timestamps → no `Session time` row) | mirrors `TestUsageSkillsDetail` setup style |
| control/usage_test.go `TestUsageSubagentsDetail` | rows-rendered-with-cost (seed `sess.Subagents` entry with type/description/usage + `Meta.Model`)<br>sorted-by-start (two entries, earlier first)<br>empty-state (`No subagents spawned yet.`)<br>row-clickable (`?detail=subagents` in panel HTML) | seeds the map directly, like skills tests seed `sess.Skills` |
| control/usage_test.go `TestUsageFilesDetail` | rows-rendered (seed `TouchedFiles` via `sess.AddFileTouch`)<br>path-sorted<br>empty-state (`No touched files.`)<br>counter-row-count (`Touched files` row shows map length) | |
| control/pages_test.go `TestMemoryFragment` | codex-unavailable (s2 → `memory is not available for codex sessions`)<br>no-path-unavailable (s1 has no `FilePath` → `transcript path unknown`)<br>facts-rendered (temp project dir with `memory/MEMORY.md` + one fact file, `FilePath` set into it → index HTML + fact name + type badge)<br>not-found-404 | temp dir via `t.TempDir()`, transcript path = `<dir>/x.jsonl` |
| control/api_test.go `TestMemoryAPI` | claude-with-memory-200 (JSON has `facts`, `index`)<br>codex-404 | mirrors existing api_test handlers |
- Not tested: htmx trigger behavior (`load` vs `peek-refresh`) — template attribute, no test seam; covered by runbook.

## Test runbook

- Tool: bash + curl + jq scripts in [plans/control_server/runbooks/](plans/control_server/runbooks/) (exemplar: [usage_details.sh](plans/control_server/runbooks/usage_details.sh)); run against a live `make serve-http` instance.
- New scenario — location: `plans/control_server/runbooks/ui_session_metrics.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
ID=$(curl -s "$BASE/api/sessions?agent=claude" | jq -r '.sessions[0].id')
curl -s "$BASE/fragments/sessions/$ID/usage" | grep -cE '<th>Session time</th>'
curl -s "$BASE/fragments/sessions/$ID/usage" | grep -cE '<th>Idle time</th>'
curl -s "$BASE/fragments/sessions/$ID/usage" | grep -cE '<th>Touched files</th>'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=subagents" | grep -cE '<th>Agent</th>|No subagents spawned'
curl -s "$BASE/fragments/sessions/$ID/usage?detail=files" | grep -cE '<th>Path</th>|No touched files'
curl -s "$BASE/fragments/sessions/$ID/memory" | grep -cE 'md-body|<details|memory is not available|transcript path unknown|No memory directory'
curl -s "$BASE/api/sessions/$ID/memory" | jq -e 'has("index") or has("facts") or has("error")' >/dev/null && echo 1
CODEX=$(curl -s "$BASE/api/sessions?agent=codex" | jq -r '.sessions[0].id // empty')
[ -z "$CODEX" ] || curl -s "$BASE/fragments/sessions/$CODEX/memory" | grep -c 'memory is not available for codex sessions'
```

- The existing [usage_details.sh](plans/control_server/runbooks/usage_details.sh) re-runs unchanged to verify the behavior contract on the untouched details.

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| `?detail=` whitelist | `usageDetailParam` ↔ `_usage.html` rows ↔ dispatch block ↔ [usage_details.sh](plans/control_server/runbooks/usage_details.sh) | every new constant appears in all three code sides; grep `detail=subagents` and `detail=files` hits param switch, row, dispatch — 3 each |
| Fragment↔API route parity | [server.go:124-139](control/server.go:124) | after Phase 4, every `/fragments/sessions/{id}/<x>` has an `/api/sessions/{id}/<x>` mirror — memory included |
| Unavailable-string vocabulary | `memoryBlock` ([tools.go:389](tools/tools.go:389)) ↔ `handleMemoryFragment`/`handleMemory` | strings byte-identical; grep "not available for codex sessions" → 3 hits (tools, fragment, api) plus tests |
- No renames, moves, or signature changes — no grep-to-zero sweeps.

## Verification

- Build/test per phase: `make test` (runs vet + unit tests), `make build-local`.

Phase 1
- [ ] Run `make test` — control package green including `TestUsageTimeRows`
- [ ] `make serve-http`, open a real session's detail page — Session/Idle/Active rows show; values match `session_events` MCP output (`wall_seconds` etc.) for the same session
- [ ] Open a session with a single turn — time rows show `0s`, not garbage; a session with no timestamps shows no time rows

Phase 2
- [ ] Click "Subagents spawned" on a session that spawned agents — table shows type, description, start, duration, tokens, cost; order oldest-first
- [ ] Click on a session without subagents — "No subagents spawned yet."
- [ ] Detail selection survives a `peek-refresh` (panel root carries `?detail=subagents`)

Phase 3
- [ ] "Touched files" row count equals `session_events` `touched_files` length for the same session
- [ ] Click it — paths sorted, reads/writes match MCP output; codex session shows 0 / empty state

Phase 4
- [ ] Open Memory on a Claude session of this repo — MEMORY.md renders as markdown, fact files listed as collapsible entries with type badges
- [ ] Codex session → "memory is not available for codex sessions"
- [ ] `curl /api/sessions/<id>/memory` returns the same JSON shape as `session_get remember:true`'s memory block content
- [ ] Run `plans/control_server/runbooks/ui_session_metrics.sh` — all greps ≥ 1
- [ ] Run `plans/control_server/runbooks/usage_details.sh` — unchanged results (behavior contract)

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| 1 | An approved signature/contract can't hold as planned | stop and report; never improvise architecture mid-edit |
| 2 | Second failed fix on the same mechanism | stop, research the actual cause, redesign; no third band-aid |
| 3 | Missing prerequisite (generated code, running infra) | run the producing step; if infrastructure is down, ask |
| 4 | Discovered work materially exceeds approved scope | ask before continuing |
| 5 | Same kind of bug found a second time: in own diff → fix all in diff; pre-existing outside → report and ask | |
| 6 | Structural obstacle tempts a new abstraction (interface, DTO, wrapper) | stop and report; relocate, don't indirect |
| 7 | Mechanical transform loses fidelity vs source | diff element-by-element before presenting; any loss → stop |
| 8 | Old and new structure would coexist beyond the phasing | stop and report |
| 9 | A driver contradicts a `[USER]` decision in the originating plan | surface the conflict, never override |
| 10 | `control` → `claude` import creates a cycle or the build rejects it | stop and report (expected clean: `tools` already imports `claude`) |

## Open questions

None.

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
