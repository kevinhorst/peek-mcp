# Desktop Projects (Cowork) Ingestion — Implementation Plan

## TLDR

- Peek misses all Claude Desktop Cowork sessions: each runs in a private claude-home under `~/Library/Application Support/Claude/<store>/…`, outside the watched `~/.claude/projects`.
- Add a Cowork watcher over that store (new `--cowork-home` flag, macOS default), reusing the existing Claude watcher/parser with a transcript-path filter that keeps `audit.jsonl` and session artifacts out.
- Add `Meta.Project` — `"cowork"` for store sessions, folder name for legacy `~/Claude_<Name>` cwds, empty otherwise — exposed automatically in `session_list` and filterable via a new `project` param (MCP + control API).
- Chat/Cloud were evaluated in the concept: not integrable, no build items here.
- Result: Cowork sessions appear in `session_list` like any repo session and can be filtered with `project: "cowork"`.

## Context

- Concept (clarified, binding): [plans/peek_projects_chat_cloud/concept/concept.md](plans/peek_projects_chat_cloud/concept/concept.md) + [desktop_projects.md](plans/peek_projects_chat_cloud/concept/desktop_projects.md) in the session worktree.
- Cowork transcripts live at `<store>/<account-uuid>/<org-uuid>/local_<id>/.claude/projects/<slug>/<sessionId>.jsonl`; store name is mid-rename (`local-agent-mode-sessions` → `claude-code-sessions`, both may coexist). Standard Claude entry schema.
- Peek watches one root per agent ([watcher/watcher.go:68](watcher/watcher.go)); every `.jsonl` under it counts as a transcript ([watcher/watcher.go:95,140](watcher/watcher.go)) — inside the Cowork store that predicate would also swallow `audit.jsonl` and artifacts.
- No local project identity exists (path segments are account/org UUIDs) — `project` is a coarse label by [USER] decision in the concept.
- Constraint: Cowork sessions stay `agent: "claude"` — no third agent value (concept decision; avoids ~15 hardcoded `claude|codex` sites).

## Drivers

N/A — new route.

## Scope

- **In:**
  - **cowork-watcher:** second Claude `watcher.Watcher` over the Cowork store(s), with transcript-path filter and project label.
  - **cowork-home-flag:** `--cowork-home` flag + `PEEK_COWORK_HOME` env fallback + `~` expansion; macOS default, empty default elsewhere.
  - **project-field:** `Meta.Project` incl. `Meta.Update` propagation and legacy `~/Claude_<Name>` derivation.
  - **project-filter:** `project` param on the `session_list` MCP tool and the control API sessions endpoint.
- **Out:**
  - **chat-cloud:** no chat/cloud integration — evaluated closed in the concept.
  - **teleport-labeling:** backlog, gated on concept Open Question 1 (user-side login test).
  - **project-names/titles/audit-events:** concept backlog items.
- **Not changed:**
  - **session_get/session_events:** no `project` param there ([USER] concept decision; field still visible inside `meta` where meta is rendered).
  - **control/sessions.go:95 handler:** keeps agent-only filtering (page/SSE surface, not the JSON list API).
  - **config file keys:** `cowork-home` is flag/env only, like `claude-home` (not in `config.EditableKeys`).
- **Deferred findings:**
  - **claude-code-sessions store today:** holds only `local_*.json` metadata on this machine — watcher over it is future-proofing for the rename; ingests nothing until transcripts appear there.

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| Concept: watcher can filter paths | Watcher has no filter hook today — bare `.jsonl` suffix checks at two sites | [watcher/watcher.go:95,140](watcher/watcher.go) |
| Concept: label stamped watcher-side | Watcher already post-stamps turns (`turn.FilePath`) after parse — same seam works for the label | [watcher/watcher.go:212](watcher/watcher.go) |
| Concept: `project` in `session_list` output is a new exposure | `sessionListItem.Meta` embeds `session.Meta` — a new `Project` field with `omitempty` is exposed automatically | [tools/viewmodels.go:37-47](tools/viewmodels.go) |

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d1"></a>D1 | How the watcher learns filter + label | [F1](#f1), [F2](#f2) | Two optional exported fields on `Watcher`: `TranscriptPathOk func(string) bool`, `Project string` — set between `New()` and `Run()`; nil/empty = today's behavior | Smallest seam; no new constructor or interface (stop-condition 6 style); fields are set before the goroutine starts, so no locking concern |
| <a id="d2"></a>D2 | Where the project label is computed | [F2](#f2), [F5](#f5) | In `readNewLines` after `Validate`: `w.Project` wins; else legacy `~/Claude_<Name>` derivation from `turn.Meta.CWD` via package-level helper | The watcher is the only place that knows both the store (label) and the parsed CWD; parsers stay agent-agnostic and untouched |
| <a id="d3"></a>D3 | Transcript predicate for the Cowork root | [F6](#f6) | `strings.Contains(filepath.ToSlash(path), "/.claude/projects/")`, composed with the existing `.jsonl` suffix check | Excludes `audit.jsonl` (store root of each session dir) and `outputs/`/`uploads/` artifacts; `ToSlash` keeps it Windows-safe |
| <a id="d4"></a>D4 | Which roots to watch | [F7](#f7) | Per store name (`local-agent-mode-sessions`, `claude-code-sessions`) under cowork-home: spawn a watcher iff the dir exists at startup | Store is mid-rename, both can coexist ([USER] concept decision); non-existent dir = one warn log, same parity as claude/codex homes |
| <a id="d5"></a>D5 | Default cowork-home | [F7](#f7) | `runtime.GOOS == "darwin"` → `~/Library/Application Support/Claude`; else `""` (disabled) | [USER]: macOS default + `--cowork-home` override for everything else |
| <a id="d6"></a>D6 | Filter semantics in handlers | [F3](#f3), [F4](#f4) | Exact string match on `Meta.Project`; empty param = no filter; MCP tool + `handleSessions` only | [USER]: `session_list` only; label set is small and closed, no globbing use case |
| <a id="d7"></a>D7 | Legacy derivation match rule | [F5](#f5) | CWD equal to or under `<home>/Claude_<Name>` → `<Name>`; computed against `os.UserHomeDir()` once at package init | Single sample on disk is `/Users/kevinpersonal/Claude_Unassigned` with cwd = the dir itself; prefix rule also covers subdir cwds |

## Open questions

None — all decisions closed (concept Q1/teleport gates only a backlog item outside this plan).

## Baseline (verified)

Base branch: `claude/peek-projects-chat-cloud-6940d7` (current worktree, clean).

| ID | Fact | Needed for | Location |
|---|---|---|---|
| <a id="f1"></a>F1! | `watcher.New(agent, agentDir, newParser, store)` builds the struct; `Run` walks + watches; both call sites in start.go pass no options | [D1](#d1) | [watcher/watcher.go:50-58](watcher/watcher.go), [cmd/start.go:109,130](cmd/start.go) |
| <a id="f2"></a>F2! | `readNewLines` post-stamps each validated turn (`turn.FilePath = path`) before `AddTurnBySessionId` — the seam for label stamping | [D1](#d1), [D2](#d2) | [watcher/watcher.go:207-214](watcher/watcher.go) |
| <a id="f3"></a>F3! | `sessionListHandler` builds `sessionListItem` with `Meta: sess.Meta`; agent arg read via `resolveAgentFromRequest` | [D6](#d6), [§tools](#changes) | [tools/tools.go:252-278](tools/tools.go) |
| <a id="f4"></a>F4! | `handleSessions` validates `agent` via `agentParam(r)` then lists/pages | [D6](#d6), [§control](#changes) | [control/api.go:44-63](control/api.go) |
| <a id="f5"></a>F5! | Claude parser fills `Meta.CWD` from entries; legacy sample cwd `/Users/kevinpersonal/Claude_Unassigned`, transcript already ingested today | [D2](#d2), [D7](#d7) | claude/parser.go (cwd handling); `~/.claude/projects/-Users-kevinpersonal-Claude-Unassigned/31009732….jsonl` |
| <a id="f6"></a>F6! | Real Cowork session dir holds `audit.jsonl` (own schema, HMAC) next to `.claude/projects/<slug>/<id>.jsonl` — suffix-only predicate would ingest it | [D3](#d3) | `…/local_313a721a…/audit.jsonl` (inspected 2026-08-30) |
| <a id="f7"></a>F7! | Flag plumbing: `flags.String("claude-home", defaultHome(...))` + `envFallbacks` map + `pathFlags` set for `~` expansion | [D4](#d4), [D5](#d5), [§start](#changes) | [cmd/start.go:270-351](cmd/start.go) |
| <a id="f8"></a>F8 | `Meta.Update` copies field-by-field on non-empty — new fields must be added explicitly | [§meta](#changes) | [session/meta.go:24-44](session/meta.go) |
| <a id="f9"></a>F9 | Watcher events also hit the suffix check at Run loop (`event.Name`), not only walkAndWatch | [§watcher](#changes) | [watcher/watcher.go:95](watcher/watcher.go) |

## Exemplar & reuse

| Existing | Used for |
|---|---|
| Claude watcher goroutine block ([cmd/start.go:105-114](cmd/start.go)) | Cowork watcher goroutine — copied verbatim, different root + fields |
| `defaultHome` helper ([cmd/start.go:399-405](cmd/start.go)) | Building the darwin default cowork-home |
| `envFallbacks` / `pathFlags` maps | `PEEK_COWORK_HOME` + `~` expansion |
| `resolveAgentFromRequest` arg-reading pattern | Reading the `project` string arg in `session_list` |

- Without exemplar: none — every change mirrors an existing pattern in the same file.

## Changes

Dependency order.

### 1. session/meta.go (modified) <a id="changes"></a>

location: `session/meta.go`

```diff
 type Meta struct {
 	SessionId Id      `json:"session_id,omitempty"`
 	CWD       string  `json:"cwd,omitempty"`
 	GitBranch string  `json:"git_branch,omitempty"`
 	Model     string  `json:"model,omitempty"`
+	Project   string  `json:"project,omitempty"`
 	Origin    *Origin `json:"origin,omitempty"`
 }
```

```diff
 func (m *Meta) Update(other *Meta) {
 	// ...
 	if other.Model != "" {
 		m.Model = other.Model
 	}
 
+	if other.Project != "" {
+		m.Project = other.Project
+	}
+
 	if other.Origin != nil {
 		m.Origin = other.Origin
 	}
 }
```

### 2. watcher/watcher.go (modified) <a id="watcher"></a>

location: `watcher/watcher.go`
mirrors: existing post-parse stamping in `readNewLines` (F2)

Struct fields (D1) — full final struct:

```go
type Watcher struct {
	agent     session.Agent
	agentDir  string
	files     map[string]*watchedFile
	mu        sync.Mutex
	newParser func() Parser
	store     *session.Store

	// Optional, set between New and Run.
	// TranscriptPathOk restricts which .jsonl files count as transcripts (nil = all).
	// Project labels every ingested turn's Meta.Project (empty = derive from CWD).
	TranscriptPathOk func(path string) bool
	Project          string
}
```

Predicate applied at both `.jsonl` sites (F9):

```diff
 			// new or changed file
-			if strings.HasSuffix(path, jsonlSuffix) {
+			if w.isTranscriptPath(path) {
 				err = w.readNewLines(path)
```

```diff
 		if isSubagentPath(path) {
 			subagentPaths = append(subagentPaths, path)
 			return nil
 		}
-		if strings.HasSuffix(path, jsonlSuffix) {
+		if w.isTranscriptPath(path) {
 			err = w.readNewLines(path)
```

New helpers + label stamping — complete units:

```go
func (w *Watcher) isTranscriptPath(path string) bool {
	if !strings.HasSuffix(path, jsonlSuffix) {
		return false
	}
	if w.TranscriptPathOk != nil {
		return w.TranscriptPathOk(path)
	}
	return true
}
```

```diff
 			turn := watched.parser.ParseLine(line)
 			err = turn.Validate()
 			if err != nil {
 				continue
 			}
 			turn.FilePath = path
+			w.stampProject(turn)
 
 			w.store.AddTurnBySessionId(turn.Meta.SessionId, w.agent, turn)
```

```go
func (w *Watcher) stampProject(turn *session.Turn) {
	if turn.Meta == nil {
		return
	}
	if w.Project != "" {
		turn.Meta.Project = w.Project
		return
	}
	turn.Meta.Project = legacyProjectFromCwd(turn.Meta.CWD)
}

var userHome, _ = os.UserHomeDir()

// legacyProjectFromCwd maps the dead-generation desktop layout
// (cwd at or under ~/Claude_<Name>) to <Name>; everything else is "".
func legacyProjectFromCwd(cwd string) string {
	if userHome == "" || cwd == "" {
		return ""
	}
	prefix := filepath.Join(userHome, "Claude_")
	if !strings.HasPrefix(cwd, prefix) {
		return ""
	}
	rest := cwd[len(prefix):]
	if i := strings.IndexByte(rest, filepath.Separator); i >= 0 {
		rest = rest[:i]
	}
	return rest
}
```

- The subagent fold-in path (`readSubagentMeta`) builds meta-only turns with just `SessionId` — `Meta.Update` ignores their empty `Project`, so labels from transcript turns survive.

### 3. cmd/start.go (modified) <a id="start"></a>

location: `cmd/start.go`
mirrors: Claude watcher goroutine block (cmd/start.go:105-114), flag/env plumbing (F7)

Flag + env + default:

```diff
 	flags.String("claude-home", defaultHome(".claude"), "Claude Code session root")
 	flags.String("codex-home", defaultHome(".codex"), "Codex session root")
+	flags.String("cowork-home", defaultCoworkHome(), "Claude Desktop Cowork data root (empty disables; macOS default set automatically)")
```

```diff
 var envFallbacks = map[string]string{
 	// ...
 	"codex-home":           "PEEK_CODEX_HOME",
+	"cowork-home":          "PEEK_COWORK_HOME",
```

```diff
 var pathFlags = map[string]bool{
 	"claude-home": true,
 	"codex-home":  true,
+	"cowork-home": true,
 	"state-dir":   true,
 }
```

```go
// coworkStoreNames: the Cowork session store is mid-rename; both names can coexist.
var coworkStoreNames = []string{"local-agent-mode-sessions", "claude-code-sessions"}

func defaultCoworkHome() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "Claude")
}

func isCoworkTranscriptPath(path string) bool {
	return strings.Contains(filepath.ToSlash(path), "/.claude/projects/")
}
```

Wiring (after the codex block, reading the flag next to the others):

```go
coworkHome, _ := flags.GetString("cowork-home")
```

```go
if coworkHome != "" {
	for _, name := range coworkStoreNames {
		storeDir := filepath.Join(coworkHome, name)
		if info, err := os.Stat(storeDir); err != nil || !info.IsDir() {
			continue
		}
		go func() {
			newParser := func() watcher.Parser { return claude.NewParser() }
			w := watcher.New(session.AgentClaude, storeDir, newParser, store)
			w.TranscriptPathOk = isCoworkTranscriptPath
			w.Project = "cowork"
			if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("cowork watcher error", "err", err)
				os.Exit(1)
			}
		}()
	}
}
```

- `agents` list: no change — Cowork requires `claudeHome` semantics only for `AgentClaude` registration, which is keyed on `claudeHome != ""`; when a user runs cowork-only (claude-home explicitly emptied), the store must still know the agent:

```diff
 	var agents []session.Agent
-	if claudeHome != "" {
+	if claudeHome != "" || coworkHome != "" {
 		agents = append(agents, session.AgentClaude)
 	}
```

- Control server `Config`: add `CoworkHome` next to `ClaudeHome`/`CodexHome` (one field in `control.Config` + one assignment) so the dashboard config view stays truthful.

### 4. tools/tools.go (modified) <a id="tools"></a>

location: `tools/tools.go`
mirrors: `agent` param declaration + `resolveAgentFromRequest` read pattern (F3)

```diff
 		mcp.WithString("agent",
 			mcp.Description("Agent: \"claude\" or \"codex\". Lists all sessions when omitted."),
 		),
+		mcp.WithString("project",
+			mcp.Description("Exact project label filter (e.g. \"cowork\"). Lists all sessions when omitted."),
+		),
```

```diff
 func sessionListHandler(s *session.Store) server.ToolHandlerFunc {
 	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
 		agent, err := resolveAgentFromRequest(s, request)
 
 		var sessions []*session.Session
 		if err != nil {
 			sessions = s.List()
 		} else {
 			sessions = s.List(agent)
 		}
+		if project := request.GetString("project", ""); project != "" {
+			filtered := sessions[:0:0]
+			for _, sess := range sessions {
+				if sess.Meta.Project == project {
+					filtered = append(filtered, sess)
+				}
+			}
+			sessions = filtered
+		}
 		items := make([]sessionListItem, len(sessions))
```

- `sessionListItem` needs no change — `Meta` embeds `session.Meta` (Assumptions row 3).
- Tool description: append "project label" to the metadata enumeration in `session_list`'s description string.

### 5. control/api.go (modified) <a id="control"></a>

location: `control/api.go`
mirrors: `agentParam` (F4)

```go
func projectParam(r *http.Request) string {
	return r.URL.Query().Get("project")
}
```

```diff
 func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
 	agents, ok := agentParam(r)
 	if !ok {
 		respondBadRequest("agent must be \"claude\" or \"codex\"", w)
 		return
 	}
+	project := projectParam(r)
 	// ... after sessions are listed, before paging:
+	if project != "" {
+		filtered := sessions[:0:0]
+		for _, sess := range sessions {
+			if sess.Meta.Project == project {
+				filtered = append(filtered, sess)
+			}
+		}
+		sessions = filtered
+	}
```

- Exact insertion point: directly after the store list call that produces `sessions` in `handleSessions`, before `pageSlice` — so paging counts filtered rows.

## Hot items

- **Goroutine (hot class 2): Cowork watcher goroutine** — example implementation written out in full in [Changes §3](#start); it is a verbatim copy of the approved Claude-watcher goroutine pattern at cmd/start.go:105-114 (same lifecycle: ctx cancellation via `Run`, `os.Exit(1)` on non-cancel error). No channels, no new locking — `TranscriptPathOk`/`Project` are set before `Run` starts and only read afterwards.
- No SQL, no new interfaces/generics (func field + string field on an existing struct), no migrations, no validation weakened, no anonymous structs.

## Tests

| Location.Method | Cases | Comment |
|---|---|---|
| watcher/watcher_test.go `TestReadNewLines_ProjectLabel` | watcher with `Project: "cowork"` stamps `Meta.Project` on ingested turn<br>watcher without label + cwd `<home>/Claude_Unassigned` derives `"Unassigned"`<br>plain repo cwd → `Project` empty | mirrors `TestReadNewLines_PerFileParserState` skeleton (tmp dir + `appendLine`); home override via the `userHome` package var |
| watcher/watcher_test.go `TestIsTranscriptPath_CoworkFilter` | `…/local_x/.claude/projects/slug/id.jsonl` → true<br>`…/local_x/audit.jsonl` → false<br>`…/outputs/data.jsonl` → false<br>nil filter → any `.jsonl` true | table-driven per repo test style |
| watcher/watcher_test.go `TestLegacyProjectFromCwd` | exact dir, subdir cwd, non-matching cwd, empty cwd | pure function table test |
| tools (existing session_list test file if present, else new `tools/tools_list_test.go`) `TestSessionList_ProjectFilter` | project set → only matching sessions<br>project omitted → all | store seeded via `AddTurnBySessionId` |

- Not tested: fsnotify runtime behavior on the deep store tree — covered by the live verification below, because the existing watcher tests also drive `readNewLines` directly rather than through fsnotify.

## Test runbook

- **cowork-visible:** `session_list` via MCP (running peek) — Cowork session `5f3f7331-1789-4bd7-a621-6cc0fa3946ec` present with `"project":"cowork"`.
- **legacy-label:** `session_list` — legacy session `31009732-3ef5-4a5e-8c0e-6d2fde43d9df` carries `"project":"Unassigned"`.
- **filter:** `session_list {project:"cowork"}` and `GET /api/sessions?project=cowork` — only Cowork rows.
- **no-noise:** peek log after startup — no parse warnings from `audit.jsonl` paths.

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| `session_list` response shape (`meta.project` added, omitempty) | tools/viewmodels.go ↔ MCP clients, docs | grep docs/ + README for `session_list` field lists; update tool description |
| Control API `/api/sessions` query params | control/api.go ↔ dashboard JS | grep control/ static assets for `agent=` usage; `project=` is additive, no consumer breaks |
| `control.Config` JSON (new `CoworkHome`) | control/viewmodels ↔ dashboard config view | grep dashboard template for config keys; additive |
| Flag/env surface | cmd/start.go ↔ setup docs (`cmd/setup.go`, README, docs/) | grep for `PEEK_CODEX_HOME` to find every enumeration that must gain `PEEK_COWORK_HOME` |

## Verification

- [ ] `make test` (or `go test ./...` if no target) — all green, incl. the four new tests.
- [ ] `go build ./...` — clean.
- [ ] Run `./peek-mcp start` locally against real data — log shows no `audit.jsonl` parse warnings.
- [ ] Call `session_list` — expect `5f3f7331-…` (Cowork, `project:"cowork"`) and `31009732-…` (`project:"Unassigned"`) both present.
- [ ] Call `session_list` with `project:"cowork"` — expect only the Cowork session(s).
- [ ] `curl "http://127.0.0.1:<control-port>/api/sessions?project=cowork"` — same filtering; `?project=nope` — empty list, HTTP 200.
- [ ] Degenerate: start with `--cowork-home ""` — no cowork watcher, no behavior change vs. main.
- [ ] Degenerate: `--cowork-home /nonexistent` — single warn-free skip (dirs don't exist), server runs.

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | Stop and report — never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | Stop, research actual cause, redesign — no third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | Run the producing step; if infra is down, ask |
| S4 | Discovered work materially exceeds approved scope | Ask before continuing |
| S5 | Same kind of bug found twice: in-diff → fix all in diff; pre-existing outside → report and ask | Sweeps are the user's call |
| S6 | Structural obstacle tempts a new abstraction | Stop and report; relocate, don't indirect |
| S7 | Cowork store layout on disk deviates from the concept's observed layout during verification | Stop; re-verify layout before adapting the predicate |
| S8 | `queue-operation` or other Cowork-specific entries produce parse errors that pollute logs | Stop and report — parser changes are outside this plan's scope |

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
