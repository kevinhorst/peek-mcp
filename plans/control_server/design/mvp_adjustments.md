# Peek Control Server MVP — Change Plan

## TLDR

- Dashboard turn default goes 5 → 20 by deleting `control.defaultTurns` and reusing `tools.DefaultReturnedTurns`; every stale "default 5" doc string is swept to 20.
- Session-detail sections (Turns, Plan, Diff, Uncommitted diff) become `<details class="section">`, collapsed by default; content still preloads and live-refreshes.
- `/api/sessions` gains `offset` and a `total` response field; the HTML sessions fragment gains `agent` + `offset` params — all sessions reachable, 50 per page.
- The index page becomes two collapsible groups (Claude, Codex — open by default) with a last-activity timestamp in the group header; each group renders a table: truncated Id (link) | Title | Branch (branch only) | Timestamp | Status badges. Per-row agent badge and `cwd @` prefix are removed.

## Context

Post-implementation adjustments to the control-server dashboard shipped from [plans/control_server/design/raw.md](plans/control_server/design/raw.md), surfaced by first real usage. The originating plan's D14 set the turns default to 5; usage shows 5 is useless. The flat card list does not scale beyond 50 sessions and wastes space on redundant agent/cwd text.

Persistence on approval: `plans/control_server/design/mvp_adjustments.md`; a changelog row is appended to `raw.md` (repo convention, cf. commit 71fa8bb).

## Drivers

| ID | Observed | Wanted | Impact | Origin |
|---|---|---|---|---|
| DR1 | Dashboard shows 5 turns (`defaultTurns = 5`, [api.go:18](control/api.go:18)); docs/tool descriptions still claim "default 5" while MCP tools already default to `DefaultReturnedTurns = 20` ([tools.go:15](tools/tools.go:15)) | Default 20 everywhere; every "5" default swept from the codebase | behavioral | usage |
| DR2 | Detail page renders Turns/Plan/Diff/Uncommitted diff fully expanded under `<h2>` headings | Each section collapsible, collapsed by default | behavioral | usage |
| DR3 | Session list hard-caps at 50 ([sessions.go:80](control/sessions.go:80)); no offset anywhere — sessions beyond 50 unreachable | Pagination; all sessions available | contract-touching | usage |
| DR4 | Flat card list; per-row agent badge; branch shown as `baseName(cwd) @ branch`; no grouping | Two top-level collapsible widgets (Claude / Codex) with last timestamp in header; per-group table Id (truncated) \| Title \| Branch (branch only) \| Timestamp \| Status badges; agent removed from rows | behavioral | usage |

Interpretation fixed in [D3](#decisions): "Remove Claude from display" = drop the per-row agent badge (the group header carries the agent); the group headers themselves read "Claude" / "Codex".

## Scope

- In
  - `control/api.go`, `control/sessions.go`, `control/viewmodels.go` handler/viewmodel changes
  - Templates `sessions_index.html`, `_session_list.html`, `session_detail.html`
  - Doc sweep: `tools/tools.go` descriptions, `README.md`, `skills/peek/SKILL.md`, `.claude/peek/SKILL.md`, `plans/control_server/concept/http_api.md`
  - Tests in `control/`
- Out
  - MCP tool behavior (already defaults to 20)
  - SSE, auth, middleware, configserver side
- Not changed
  - Detail-page description line (agent badge, `cwd @ branch`, model) — DR4 targets the list
  - Turn/plan/diff fragment rendering internals; 256 KB diff truncation
  - `--depth` flag and its mcpb drift (default 50 vs CLI 20)
- Deferred findings
  - mcpb `user_config.depth` default 50 vs CLI `--depth` 20 (already noted in raw.md:44)
  - `skills/peek/SKILL.md` and `.claude/peek/SKILL.md` have drifted on the `agent` param text (only the "5" lines are touched here)
  - `Session.Turns(n)` may drop the active turn (raw.md:41)

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| Request assumes list shows the word "Claude" per row | It renders the raw agent value as a badge (`claude`/`codex`) | [_session_list.html:5](control/templates/_session_list.html:5) |
| Request assumes 5 is a repo-wide default | Runtime 5 exists only in `control`; MCP tools already use 20 — remaining "5"s are doc drift | [api.go:18](control/api.go:18), [tools.go:15](tools/tools.go:15) |

## Current state

| File | Lines | Responsibility |
|---|---|---|
| [control/api.go](control/api.go) | 177 | JSON API; consts incl. `defaultTurns = 5`, `defaultSessionLimit = 50`; agent-switch inline in `handleSessions` |
| [control/sessions.go](control/sessions.go) | 144 | HTML pages + fragments; `handleSessionsFragment` hardcodes 50-cap, no params; `handleTurnsFragment` hardcodes `defaultTurns` |
| [control/viewmodels.go](control/viewmodels.go) | 70 | `sessionSummary`, `sessionsResponse{Sessions}` (no total) |
| [control/templates/*.html](control/templates) | 7 files | Card idiom; no `<details>`, no `<table>` anywhere |
| [control/assets/style.css](control/assets/style.css) | 433 | Ships unused `details.section` (:363-368), `.evidence-table` (:235-239), `button.small`, `.section-controls` idioms |

## Target state

```
sessions_index.html
├─ <details class="section" open> Claude  <span id=last-active-claude>(OOB)</span>
│   └─ #sessions-claude  ← /fragments/sessions?agent=claude&offset=N  (self-refreshing)
│       └─ _session_list.html: table + prev/next controls + OOB timestamp span
└─ <details class="section" open> Codex   (same, agent=codex)

session_detail.html
├─ <details class="section"> Turns            └─ htmx container (loads collapsed)
├─ <details class="section"> Plan             └─ …
├─ <details class="section"> Diff             └─ …
└─ <details class="section"> Uncommitted diff └─ …
```

**Principle — single source of truth:** the one turn-count default is `tools.DefaultReturnedTurns`; `control` imports it (it already imports `tools` for `UTF8SafeSlice`), `control.defaultTurns` is deleted.
**Principle — state lives in the URL:** each group's htmx root carries its current `agent`+`offset` in its own `hx-get`, so the 1 s `peek-refresh` re-fetch preserves the page; collapse state survives because the `<details>` elements sit in the static page shell and only their inner containers swap. Header timestamps update via `hx-swap-oob` spans appended after the fragment root.
**Principle — mirror existing idiom:** tables use the shipped `.evidence-table` CSS, collapsibles the shipped `details.section` CSS, paging buttons `button.small` inside `.section-controls` (all already in `style.css`, copied from the configserver per raw.md D12).

## Behavior contract

- Must not change: session ordering (last-active desc, [store.go:209](session/store.go:209)); JSON shapes of existing fields; fragment self-refresh via `peek-refresh` throttle 1 s; diff truncation at 256 KB; auth/host checks; empty states rendered per section.
- Intentional changes (map to drivers): turns fragment/API default 5 → 20 (DR1); detail sections collapsed by default (DR2); `/api/sessions` response gains `total`, accepts `offset`; `/fragments/sessions` requires `agent`, accepts `offset` (DR3); index layout card list → grouped tables, per-row agent badge and `cwd @` prefix removed (DR4).

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| D1 | Where the 20 comes from | `tools.DefaultReturnedTurns = 20` exists; `control` already imports `tools` | Delete `defaultTurns`; use `tools.DefaultReturnedTurns` in both call sites | Single source of truth; a second `= 20` const re-creates the drift DR1 kills |
| D2 | Sweep vs historical plan docs | raw.md contains "default 5" in D14 and planned code blocks; repo convention appends changelog rows (commit 71fa8bb) | `plans/control_server/design/raw.md` is not rewritten — it gets a changelog row; `concept/http_api.md` is a living API doc and is updated | Historical plans record what was decided then; the API doc must match the API |
| D3 | "Remove Claude from display" ambiguity | Group headers must be distinguishable | Per-row agent badge removed; group summaries read "Claude" / "Codex" | The header carries the agent; a per-row badge inside an agent group is redundant |
| D4 | Index group default state | DR2 says detail sections collapsed; DR4 only says "collapsable" | Index groups **open** by default; detail sections **collapsed** | A dashboard landing page that shows nothing is useless; detail sections are opt-in depth |
| D5 | Page size | `defaultSessionLimit = 50` exists | Fragment page size = `defaultSessionLimit` (50); API keeps `limit` (default 50, max 200) plus new `offset` | Table rows are compact; no reason for a second constant |
| D6 | Fragment `agent` param | Index always renders exactly two groups | `agent` is required on `/fragments/sessions`; missing/invalid → 400 | An ungrouped fragment has no remaining consumer; a lenient default hides wiring bugs |
| D7 | Group-header timestamp transport | Summary sits outside the swapped container | `hx-swap-oob` span emitted after the fragment root, targeting `#last-active-{agent}` in the static summary | Keeps collapse state static-side; OOB is stock htmx, no JS added |
| D8 | Disposal of old list markup | Card list in `_session_list.html` | Card markup in `_session_list.html` is deleted, replaced by the table; the `.card` CSS stays (used by turns) | No parallel old/new list rendering |
| D9 | `total` semantics | `title` filter exists on the JSON API | `total` = count after agent+title filtering, before offset/limit | It is the paging denominator; a pre-filter total can't drive prev/next |

## Changes

### Phase 1 — turns default 20 + doc sweep (DR1)

Delete the const, [api.go:14-19](control/api.go:14):

```diff
 const (
 	defaultSessionLimit = 50
 	maxSessionLimit     = 200
 	defaultDiffSize     = 256 * 1024
-	defaultTurns        = 5
 )
```

`handleTurns`, [api.go:96](control/api.go:96):

```diff
 func (s *Server) handleTurns(w http.ResponseWriter, r *http.Request) {
-	n, ok := intParam(r, "n", defaultTurns)
+	n, ok := intParam(r, "n", tools.DefaultReturnedTurns)
 	if !ok {
```

`handleTurnsFragment`, [sessions.go:91](control/sessions.go:91):

```diff
 	data := turnsData{Id: id}
-	if !s.store.WithSession(id, func(sess *session.Session) { data.Turns = sess.Turns(defaultTurns) }) {
+	if !s.store.WithSession(id, func(sess *session.Session) { data.Turns = sess.Turns(tools.DefaultReturnedTurns) }) {
 		respondNotFound("unknown session", w)
```

Doc sweep (same one-line substitution at each site):

- [tools/tools.go:39](tools/tools.go:39), [:54](tools/tools.go:54), [:83](tools/tools.go:83) — `"Number of turns to return (default 5)"` → `(default 20)`
- [README.md:47](README.md:47), [:55](README.md:55), [:70](README.md:70) — `(default 5)` → `(default 20)`
- [README.md:264](README.md:264) — `reads the last 5 turns` → `reads the last 20 turns`
- [skills/peek/SKILL.md:12](skills/peek/SKILL.md:12) + [.claude/peek/SKILL.md:12](.claude/peek/SKILL.md:12) — `n defaults to 5` → `n defaults to 20`
- [skills/peek/SKILL.md:38](skills/peek/SKILL.md:38) + [.claude/peek/SKILL.md:36](.claude/peek/SKILL.md:36) — `(5 turns, has plan, has diff)` → `(20 turns, has plan, has diff)`
- [plans/control_server/concept/http_api.md:40](plans/control_server/concept/http_api.md:40) — `` `n` (default 5, max = `--depth`) `` → `default 20`
- [plans/control_server/design/raw.md](plans/control_server/design/raw.md) — changelog row appended (per [D2](#decisions)), no other edits

### Phase 2 — collapsible detail sections (DR2)

Full replacement of the section block, [session_detail.html:10-25](control/templates/session_detail.html:10):

```diff
-<h2>Turns</h2>
-<div hx-get="/fragments/sessions/{{.Summary.Id}}/turns" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
-  <div class="empty">Loading…</div>
-</div>
-<h2>Plan</h2>
-<div hx-get="/fragments/sessions/{{.Summary.Id}}/plan" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
-  <div class="empty">Loading…</div>
-</div>
-<h2>Diff</h2>
-<div hx-get="/fragments/sessions/{{.Summary.Id}}/diff" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
-  <div class="empty">Loading…</div>
-</div>
-<h2>Uncommitted diff</h2>
-<div hx-get="/fragments/sessions/{{.Summary.Id}}/uncommitted-diff" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
-  <div class="empty">Loading…</div>
-</div>
+<details class="section">
+  <summary>Turns</summary>
+  <div hx-get="/fragments/sessions/{{.Summary.Id}}/turns" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
+    <div class="empty">Loading…</div>
+  </div>
+</details>
+<details class="section">
+  <summary>Plan</summary>
+  <div hx-get="/fragments/sessions/{{.Summary.Id}}/plan" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
+    <div class="empty">Loading…</div>
+  </div>
+</details>
+<details class="section">
+  <summary>Diff</summary>
+  <div hx-get="/fragments/sessions/{{.Summary.Id}}/diff" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
+    <div class="empty">Loading…</div>
+  </div>
+</details>
+<details class="section">
+  <summary>Uncommitted diff</summary>
+  <div hx-get="/fragments/sessions/{{.Summary.Id}}/uncommitted-diff" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
+    <div class="empty">Loading…</div>
+  </div>
+</details>
```

mirrors: `details.section` styling contract at [style.css:363-368](control/assets/style.css:363). Content keeps preloading (`load` trigger) so opening a section is instant and refresh keeps working while collapsed.

### Phase 3 — JSON API pagination (DR3, contract-touching)

New helper + rewritten `handleSessions`, replacing [api.go:44-77](control/api.go:44) (helper placed above it; `min` is the Go builtin):

```go
func agentParam(r *http.Request) ([]session.Agent, bool) {
	switch agent := r.URL.Query().Get("agent"); agent {
	case "":
		return nil, true
	case string(session.AgentClaude), string(session.AgentCodex):
		return []session.Agent{session.Agent(agent)}, true
	default:
		return nil, false
	}
}

func pageSlice(sessions []*session.Session, offset, limit int) []*session.Session {
	if offset >= len(sessions) {
		return nil
	}
	return sessions[offset:min(offset+limit, len(sessions))]
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	agents, ok := agentParam(r)
	if !ok {
		respondBadRequest("agent must be \"claude\" or \"codex\"", w)
		return
	}
	limit, ok := intParam(r, "limit", defaultSessionLimit)
	if !ok {
		respondBadRequest("limit must be a non-negative integer", w)
		return
	}
	if limit == 0 || limit > maxSessionLimit {
		limit = maxSessionLimit
	}
	offset, ok := intParam(r, "offset", 0)
	if !ok {
		respondBadRequest("offset must be a non-negative integer", w)
		return
	}
	title := strings.ToLower(r.URL.Query().Get("title"))

	resp := sessionsResponse{Sessions: make([]sessionSummary, 0)}
	s.store.WithSessions(agents, func(sessions []*session.Session) {
		filtered := sessions
		if title != "" {
			filtered = make([]*session.Session, 0, len(sessions))
			for _, sess := range sessions {
				if strings.Contains(strings.ToLower(sess.Title), title) {
					filtered = append(filtered, sess)
				}
			}
		}
		resp.Total = len(filtered)
		for _, sess := range pageSlice(filtered, offset, limit) {
			resp.Sessions = append(resp.Sessions, newSessionSummary(sess))
		}
	})
	writeJSON(w, resp)
}
```

Response gains `total`, [viewmodels.go:28-30](control/viewmodels.go:28):

```diff
 type sessionsResponse struct {
 	Sessions []sessionSummary `json:"sessions"`
+	Total    int              `json:"total"`
 }
```

### Phase 4 — grouped tables + fragment pagination (DR3+DR4)

Viewmodel, replacing [sessions.go:31-33](control/sessions.go:31):

```go
type sessionListData struct {
	Agent      session.Agent
	Sessions   []sessionSummary
	LastActive time.Time
	Total      int
	Offset     int
	PrevOffset int
	NextOffset int
	HasPrev    bool
	HasNext    bool
	RangeEnd   int
}
```

Handler, replacing [sessions.go:75-86](control/sessions.go:75) (`time` joins the imports; `max` is the Go builtin):

```go
func (s *Server) handleSessionsFragment(w http.ResponseWriter, r *http.Request) {
	agents, ok := agentParam(r)
	if !ok || agents == nil {
		respondBadRequest("agent must be \"claude\" or \"codex\"", w)
		return
	}
	offset, ok := intParam(r, "offset", 0)
	if !ok {
		respondBadRequest("offset must be a non-negative integer", w)
		return
	}
	data := sessionListData{Agent: agents[0], Sessions: make([]sessionSummary, 0), Offset: offset}
	s.store.WithSessions(agents, func(sessions []*session.Session) {
		data.Total = len(sessions)
		if len(sessions) > 0 {
			data.LastActive = sessions[0].LastActive
		}
		for _, sess := range pageSlice(sessions, offset, defaultSessionLimit) {
			data.Sessions = append(data.Sessions, newSessionSummary(sess))
		}
	})
	data.HasPrev = offset > 0
	data.PrevOffset = max(0, offset-defaultSessionLimit)
	data.NextOffset = offset + defaultSessionLimit
	data.HasNext = data.NextOffset < data.Total
	data.RangeEnd = offset + len(data.Sessions)
	s.renderFragment(w, tmplSessionList, data)
}
```

Index page, full replacement of [sessions_index.html](control/templates/sessions_index.html):

```html
{{template "head" .}}
{{template "nav" .}}
<h1>Sessions</h1>
<details class="section" open>
  <summary>Claude <span class="meta" id="last-active-claude"></span></summary>
  <div id="sessions-claude" hx-get="/fragments/sessions?agent=claude&amp;offset=0" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
    <div class="empty">Loading…</div>
  </div>
</details>
<details class="section" open>
  <summary>Codex <span class="meta" id="last-active-codex"></span></summary>
  <div id="sessions-codex" hx-get="/fragments/sessions?agent=codex&amp;offset=0" hx-trigger="load, peek-refresh from:body throttle:1s" hx-swap="outerHTML">
    <div class="empty">Loading…</div>
  </div>
</details>
{{template "foot" .}}
```

List fragment, full replacement of [_session_list.html](control/templates/_session_list.html) — OOB span sits after the root div (htmx only honors `hx-swap-oob` on top-level response nodes); root carries its current offset so `peek-refresh` re-fetches the same page:

```html
<div id="sessions-{{.Agent}}" hx-get="/fragments/sessions?agent={{.Agent}}&amp;offset={{.Offset}}" hx-trigger="peek-refresh from:body throttle:1s" hx-swap="outerHTML">
{{if .Sessions}}
<table class="evidence-table">
  <thead>
    <tr><th>Id</th><th>Title</th><th>Branch</th><th>Timestamp</th><th>Status</th></tr>
  </thead>
  <tbody>
  {{range .Sessions}}
  <tr>
    <td><a href="/sessions/{{.Id}}">{{printf "%.8s" .Id}}</a></td>
    <td>{{.Title}}</td>
    <td>{{.GitBranch}}</td>
    <td class="meta">{{ts .LastActive}}</td>
    <td>
      {{if .HasPlan}}<span class="badge badge-ok">plan</span>{{end}}
      {{if .HasDiff}}<span class="badge badge-ok">diff</span>{{end}}
      {{if .HasUncommittedDiff}}<span class="badge badge-action">uncommitted</span>{{end}}
    </td>
  </tr>
  {{end}}
  </tbody>
</table>
{{if or .HasPrev .HasNext}}
<div class="section-controls">
  {{if .HasPrev}}<button class="small" hx-get="/fragments/sessions?agent={{.Agent}}&amp;offset={{.PrevOffset}}" hx-target="#sessions-{{.Agent}}" hx-swap="outerHTML">prev</button>{{end}}
  <span class="meta">{{if .Sessions}}{{add .Offset 1}}–{{.RangeEnd}} of {{.Total}}{{end}}</span>
  {{if .HasNext}}<button class="small" hx-get="/fragments/sessions?agent={{.Agent}}&amp;offset={{.NextOffset}}" hx-target="#sessions-{{.Agent}}" hx-swap="outerHTML">next</button>{{end}}
</div>
{{end}}
{{else}}
<div class="empty">No {{.Agent}} sessions yet.</div>
{{end}}
</div>
<span id="last-active-{{.Agent}}" class="meta" hx-swap-oob="true">{{if not .LastActive.IsZero}}{{ts .LastActive}}{{end}}</span>
```

FuncMap gains `add` next to `baseName`/`ts`, [server.go:38-56](control/server.go:38):

```diff
 	funcs := template.FuncMap{
 		"baseName": filepath.Base,
+		"add":      func(a, b int) int { return a + b },
 		"ts": func(t time.Time) string {
```

mirrors: table = `.evidence-table` ([style.css:235](control/assets/style.css:235)); paging = `button.small` in `.section-controls` ([style.css:350,369](control/assets/style.css:350)); groups = `details.section`.

## Hot items

N/A — no SQL, goroutines/channels, new interfaces, migrations, or validation/transaction guards; changes are handlers, viewmodels, and templates (baseline classes used; `context/general/hot-items.md` not present in this repo).

## Tests

| Location.Method | Cases | Comment |
|---|---|---|
| control/api_test.go `TestSessions` | response contains `"total"` matching seeded count | extend existing |
| control/api_test.go `TestSessions_Offset` (new) | `offset=1` skips newest<br>`offset` ≥ total → empty list, total intact<br>`offset=-1`/non-numeric → 400 | mirrors `TestSessions_Limit` |
| control/api_test.go `TestTurns` | unchanged (fixture has 2 turns; passes for any default ≥ 2) | no numeric-default assertion possible with 2-turn fixture |
| control/pages_test.go `TestSessionsFragment` | `?agent=claude` → table headers `Id`/`Title`/`Branch`, truncated id link `/sessions/s1`, branch cell without `@`, no per-row agent badge, OOB span `last-active-claude` | rewrite for table markup |
| control/pages_test.go `TestSessionsFragment_MissingAgent` (new) | no `agent` → 400<br>`agent=bogus` → 400 | [D6](#decisions) |
| control/pages_test.go `TestSessionsFragment_Pagination` (new) | seed 51 claude sessions via the `newTestStore` fixture loop → page 1 has 50 rows + `next`; `offset=50` → 1 row + `prev` | |
| control/pages_test.go `TestSessionsFragment_Empty` | update: `?agent=codex` on empty store → "No codex sessions yet." | |
| control/pages_test.go `TestSessionDetailPage` | extend: body contains four `<details class="section">` without `open` | DR2 |

Not tested: htmx runtime behavior (OOB swap, collapse persistence, refresh throttling) — browser-side, covered by the runbook.

## Test runbook

Format: curl+jq shell scripts, the convention of the originating plan's verification section. Persisted at implementation under `plans/control_server/runbooks/`.

location: `plans/control_server/runbooks/turns_default.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
ID=$(curl -s "$BASE/api/sessions?agent=claude" | jq -r '.sessions[0].id')
COUNT=$(curl -s "$BASE/api/sessions/$ID/turns" | jq '.turns | length')
echo "turns returned without n: $COUNT (expect min(20, --depth, available))"
curl -s "$BASE/fragments/sessions/$ID/turns" | grep -c 'card card-column'
```

location: `plans/control_server/runbooks/sessions_pagination.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
curl -s "$BASE/api/sessions?agent=claude&limit=2" | jq '{total, page: (.sessions | length), ids: [.sessions[].id]}'
curl -s "$BASE/api/sessions?agent=claude&limit=2&offset=2" | jq '{total, ids: [.sessions[].id]}'
curl -s "$BASE/fragments/sessions?agent=claude&offset=0" | grep -o 'evidence-table\|last-active-claude\|offset=50' | sort -u
curl -s -o /dev/null -w '%{http_code}\n' "$BASE/fragments/sessions"   # expect 400
```

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| `control.defaultTurns` | api.go, sessions.go (only consumers) | `grep -rn defaultTurns` → zero hits |
| "default 5" / "defaults to 5" / "5 turns" doc strings | tools.go ×3, README ×4, both SKILL.md ×2 each, http_api.md | `grep -rn 'default 5\|defaults to 5\|last 5 turns\|(5 turns'` → zero outside `plans/control_server/design/raw.md` (historical, justified per [D2](#decisions), changelog row appended) |
| `GET /api/sessions` response | dashboard-adjacent consumers: none in repo besides tests; documented in README control section + http_api.md | additive `total` + `offset`; update [http_api.md:38](plans/control_server/concept/http_api.md:38) and the README control-server API table if it lists params |
| `GET /fragments/sessions` params | sole consumers: `sessions_index.html`, `_session_list.html` self-refresh, tests | `grep -rn 'fragments/sessions"'` — every hit carries `agent=`; missing-agent 400 pinned by test |
| `sessionListData` shape | `_session_list.html`, `sessions.go` | template executes in `pages_test` — compile+render is the sweep |

## Verification

Phase 1

- [ ] Run `make test` — green.
- [ ] Run `grep -rn defaultTurns` — zero hits.
- [ ] Run the sweep grep from Contracts & sweeps — only raw.md survivors remain.
- [ ] Start `peek-mcp start --control-port 4243`; `curl /api/sessions/{id}/turns | jq '.turns|length'` on this session returns up to 20, not 5.

Phase 2

- [ ] Open a session detail page — four collapsed sections; opening Turns shows content instantly (preloaded); sending a prompt updates an open section within ~1 s and does not re-collapse it.

Phase 3

- [ ] `curl '/api/sessions?agent=claude&limit=2'` then `&offset=2` — disjoint ids, same `total`.
- [ ] `curl '/api/sessions?offset=-1'` → 400.

Phase 4

- [ ] Index shows Claude and Codex groups, open, each header with last-activity timestamp; rows show truncated id (link), title, branch only (no `cwd @`), timestamp, badges; no per-row agent badge; the word appears only in group headers.
- [ ] With > 50 sessions in a group: prev/next paging works; while on page 2, trigger a live turn — the page stays on page 2 and the header timestamp updates.
- [ ] Collapse a group, trigger a refresh — it stays collapsed.
- [ ] Run `make test` and `make build-local` — green.

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | Stop and report. Never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | Stop, research the actual cause, redesign. No third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | Run the producing step. If infrastructure is down, ask. Never skip validation, never start infrastructure yourself |
| S4 | Discovered work materially exceeds the approved scope | Ask before continuing |
| S5 | Same kind of bug found a second time: inside the diff → fix every instance now; pre-existing outside the diff | Report and ask before searching further |
| S6 | A structural obstacle tempts a new abstraction (interface, DTO, wrapper) | Stop and report. The fix is relocating the component, not indirection |
| S7 | A mechanical transform (sweep, template rewrite) loses fidelity vs the source element-by-element | Stop |
| S8 | Old and new structure would have to coexist beyond the phasing (e.g. card list kept alongside tables) | Stop and report |
| S9 | A driver contradicts a `[USER]` decision in raw.md | Surface the conflict (checked: D11/D13 untouched; D14's "default 5" is exactly what DR1 changes — driver wins, changelog row records it) |
| S10 | htmx OOB swap for the header timestamp does not work as planned | Stop and report — no inline-JS workaround without a decision |

## Open questions

None — D3, D4, D5 resolve the request's ambiguities and are recorded as decisions.

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
| 2026-07-26 | implemented ad hoc | all four phases landed; `make test` + `make build-local` green; live dashboard verified (grouped tables, pagination to 312 sessions, collapsed detail sections, 20-turn default) |
