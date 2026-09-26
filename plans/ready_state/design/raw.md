# Peek ready state — Implementation Plan

target: `opus` — design: the user's request (binding): a "ready" gate so peek only answers once the initial session load is done, exposed externally, shipped as a peek-mcp release

## TLDR

- Fresh peek instances accept MCP tool calls before the startup walk has ingested the transcript tree, so an early `session_list` / `session_get` returns a partial or empty session list.
- Every transcript watcher (claude, cowork, codex) and the codex index watcher signal "loaded" once their first pass is done; `start` waits for all of them and marks the session store ready.
- All three MCP tools wait for that ready signal before running — the MCP handshake is not delayed, the call just returns once the data is complete. A hung load returns an explicit "still loading" tool error after 2 minutes instead of hanging forever.
- Readiness is exposed as `ready: true|false` on the MCP port's `GET /healthz` and on the control server's `GET /api/healthz`, plus one info log line with session count and load duration.
- No client changes needed: the miners' own per-session stdio peeks get the gate for free. Shipped as v1.2.8.

## Context

- Problem: tool handlers read the store directly ([tools.go:297](tools/tools.go#L297)); nothing orders them after the startup walk.
- Cause: every watcher runs its initial `walkAndWatch` in a detached goroutine ([start.go:114](cmd/start.go#L114)) while the MCP server starts serving immediately ([start.go:286](cmd/start.go#L286)).
- Scale: the 14-day window currently holds 1,352 Claude transcripts / 1.5 GB ([F1](#f1)) — the window where a fresh agent's first call races the load is real, not theoretical.
- Consumers: smine's configserver probes `/healthz` for identity and requires HTTP 200 ([F6](#f6)); per-session miner peeks are stdio instances with the control server off.
- Constraint: the gate must not delay the MCP `initialize` handshake (client MCP startup timeouts).

## Drivers

N/A — new route

## Scope

- **In:**
  - **loaded signal:** `watcher.Watcher` and `watcher.CodexIndexWatcher` each expose a `Loaded()` channel closed after their first load pass
  - **store readiness:** `session.Store` owns one ready channel (`MarkReady` / `Ready` / `IsReady`)
  - **aggregation:** `cmd/start.go` collects every watcher's `Loaded()` and marks the store ready when all are closed, logging count and duration
  - **tool gate:** `session_get`, `session_list`, `session_events` wait for readiness, bounded by a 2-minute timeout
  - **exposure:** `ready` field on `GET /healthz` (MCP port) and `GET /api/healthz` (control server)
  - **tests:** store, both watchers, gate wrapper, aggregation, both healthz handlers
  - **docs:** `ready` on `/healthz` in [reference.md](docs/reference.md); startup-wait semantics in [tools.md](docs/tools.md)
  - **release:** version bump to v1.2.8 via `make git-release`
- **Out:**
  - **dashboard gating:** control-server HTML/JSON pages keep serving live, partial data during load — they update via SSE, and the request targets agent calls
  - **smine-side consumption:** configserver polling `ready` before spawning miners — separate smine change; not needed for correctness since the tool gate already blocks
  - **publishing:** pushing the tag, building/notarizing the mcpb bundle — user's call after merge
- **Not changed:**
  - **plan watcher:** no initial load pass (reacts to write events only; plan content arrives via transcript turns) — not a readiness participant
  - **diff watcher:** live diffs are computed on its poll cycle; `session_list.has_diff` from snapshots is covered because `SeedDiffCache` runs before the transcript watcher signals loaded
  - **rescan ticker:** later `walkAndWatch` passes do not touch readiness
- **Deferred findings:**
  - **reference.md port drift:** documents `--port` default `4242`; code default is `4244` ([start.go:326](cmd/start.go#L326))
  - **Desktop "Peek MCP" early exits:** `~/Library/Logs/Claude/mcp-server-Peek MCP.log` shows repeated "Server transport closed unexpectedly" within ~100 ms of start on 2026-09-25 — cause unknown, unrelated to this plan

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| "The initial load" is one thing | Four independent loaders: claude, 0–2 cowork stores, codex transcripts, codex index — each finishes separately | [start.go:114](cmd/start.go#L114) |
| The initial load always finishes | Codex is enabled by default (`--codex-home ~/.codex`); the index watcher blocks in `waitForDir` indefinitely when `~/.codex` is absent ([F4](#f4)) | [codex_index_watcher.go:84](watcher/codex_index_watcher.go#L84) |

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d1"></a>D1 | How "only answers once loaded" behaves for an early call | [F2](#f2), [F3](#f3) | Tool handlers **block** until ready (not: error-and-retry, not: delaying `ServeStdio`/`ListenAndServe`) | **controllable/reliable:** deterministic for every client with zero client change, incl. the miners' stdio peeks. Delaying serve risks the client's MCP startup timeout on a 1.5 GB load; an immediate "not ready" error pushes retry logic into every agent prompt. Blocking is safe: stdio tool calls run on a worker pool, so `initialize`/`ping` keep flowing ([F2](#f2)) |
| <a id="d2"></a>D2 | Where the per-loader "done" signal comes from | [F5](#f5) | Each watcher owns a `loaded chan struct{}` created in its constructor, closed once in `Run` after its first pass, exposed via `Loaded()` | **debuggable:** each loader's completion is local to the code that does the loading; no shared WaitGroup threaded through constructors. Mirrors the existing channel-returning accessor style (`<-chan`) |
| <a id="d3"></a>D3 | Who owns readiness | [F3](#f3) | `session.Store` holds the ready channel; `cmd/start.go` aggregates loaders and calls `store.MarkReady()` | **single source of truth:** the store is already passed to tools, control server, and healthz — no new parameter anywhere. Readiness is a property of the store's data. `start.go` already owns watcher composition, so it is the only place that knows the loader set |
| <a id="d4"></a>D4 | Waiting forever if a loader hangs | — | Gate waits for ready, client cancellation, or `readyTimeout = 2 * time.Minute`; timeout returns tool error `initial session load still in progress; retry shortly` | **reliable:** degrades to a locatable, retryable error instead of a silently hung agent. 2 min is ~10x the expected load; the new log line's `took` measures real load time for tuning |
| <a id="d5"></a>D5 | Codex index watcher with `~/.codex` missing | [F4](#f4) | If the first `Add` on codex home fails, close `loaded` immediately (nothing to load), then fall into `waitForDir` as today | **reliable:** otherwise every non-codex machine would never become ready. Keeps today's late-appearance behavior (load after dir shows up) |
| <a id="d6"></a>D6 | How readiness is exposed | [F6](#f6) | `ready` bool on MCP `/healthz` and control `/api/healthz`, HTTP 200 in both states; info log `awaitInitialLoad: Initial session load complete` with `sessions` and `took` | **controllable:** supervisors can poll it; 200 keeps smine's identity probe working during load (a 503 would make configserver reject the instance). stdio instances have no HTTP surface — the tool gate is their exposure |
| <a id="d7"></a>D7 | Release mechanics | [F7](#f7) | `make git-release VERSION=1.2.8` as the final commit (bumps Makefile, `cmd/version.go`, `mcpb/manifest.json`, commits, tags `v1.2.8` locally) | Existing release path; no hand-edited version strings. Push/publish stays with the user |
| <a id="d8"></a>D8 | Timeout testability | — | `awaitReady` takes the timeout as a parameter; `Register` passes the `readyTimeout` constant | Tests run the timeout path in milliseconds without a mutable package var |
| <a id="d9"></a>D9 | Tool-facing error wording | — | Package-level `errInitialLoadPending` next to `errSessionSelectorMissing`, lowercase | Mirrors the file's existing agent-facing error style ([tools.go:17](tools/tools.go#L17)) |

## Open questions

None.

## Baseline (verified)

Base branch: `main` at `24d8cc5` (cmd: release v1.2.7).

| ID | Fact | Needed for | Location |
|---|---|---|---|
| <a id="f2"></a>F2! | mcp-go v0.52.0 stdio server dispatches `tools/call` to a worker pool (default 5); other messages are handled on the read loop — a blocked tool handler does not block `initialize`/`ping` | [D1](#d1) | [stdio.go:403](vendor/github.com/mark3labs/mcp-go/server/stdio.go#L403), [stdio.go:547](vendor/github.com/mark3labs/mcp-go/server/stdio.go#L547), [stdio.go:607](vendor/github.com/mark3labs/mcp-go/server/stdio.go#L607) |
| <a id="f3"></a>F3! | `tools.Register`, `control.Options`, and `start` all already hold the same `*session.Store` | [D1](#d1), [D3](#d3) | [tools.go:48](tools/tools.go#L48), [server.go:27](control/server.go#L27), [start.go:97](cmd/start.go#L97) |
| <a id="f4"></a>F4! | `--codex-home` defaults to `~/.codex`, so codex is an enabled agent even when not installed; `CodexIndexWatcher.Run` calls `waitForDir`, which polls every 5 s until the dir exists | [D5](#d5) | [start.go:329](cmd/start.go#L329), [plan_watcher.go:70](watcher/plan_watcher.go#L70) |
| <a id="f5"></a>F5! | `Watcher.Run` does the first `walkAndWatch` + `SeedDiffCache` synchronously before its event loop; a missing root makes `walkAndWatch` return immediately | [D2](#d2), [Watcher change](#watcher-loaded-signal-modified) | [watcher.go:141](watcher/watcher.go#L141), [watcher.go:212](watcher/watcher.go#L212) |
| <a id="f6"></a>F6! | smine configserver reuses a peek listener only when `/healthz` returns 200 with matching homes | [D6](#d6) | smine `cmd/configserver/main.go:225-241` |
| <a id="f7"></a>F7 | `make git-release` seds VERSION into three files, commits `cmd: release v$(VERSION)`, tags | [D7](#d7) | [Makefile:55](Makefile#L55) |
| <a id="f1"></a>F1 | 14-day window today: 1,352 Claude `.jsonl`, 1.5 GB; codex 1 file; both cowork store dirs present | [Context](#context), [Verification](#verification) | `find ~/.claude/projects -name '*.jsonl' -mtime -14` (2026-09-26) |

## Exemplar & reuse

| Existing | Used for |
|---|---|
| `counted` handler wrapper ([tools.go:29](tools/tools.go#L29)) | Shape of `awaitReady` (wraps `server.ToolHandlerFunc`) |
| `runStateGc` ([start.go:346](cmd/start.go#L346)) | Shape of `awaitInitialLoad` (package func in `start.go`, launched with `go`, exits on `ctx.Done()`) |
| `errSessionSelectorMissing` ([tools.go:17](tools/tools.go#L17)) | Shape of `errInitialLoadPending` |
| `make git-release` | Version bump + tag |

- Without exemplar: the `loaded` channel on the watchers — no watcher exposes a lifecycle signal today. Covered as hot item [H1](#h1).

## Permissions

N/A — `go build`/`go test`/`make`/`curl`/`jq` and git commit/tag are covered by the deployed layers.

## Changes

| File | Kind | Entry |
|---|---|---|
| `session/store.go` | modified | [Store readiness](#store-readiness-modified) |
| `session/store_test.go` | modified | [Tests](#tests) |
| `watcher/watcher.go` | modified | [Watcher loaded signal](#watcher-loaded-signal-modified) |
| `watcher/watcher_test.go` | modified | [Tests](#tests) |
| `watcher/codex_index_watcher.go` | modified | [Codex index loaded signal](#codex-index-loaded-signal-modified) |
| `watcher/codex_index_watcher_test.go` | modified | [Tests](#tests) |
| `cmd/start.go` | modified | [Start aggregation and healthz](#start-aggregation-and-healthz-modified) |
| `cmd/start_test.go` | modified | [Tests](#tests) |
| `tools/tools.go` | modified | [Tool gate](#tool-gate-modified) |
| `tools/tools_test.go` | modified | [Tests](#tests) |
| `control/viewmodels.go` | modified | [Control healthz](#control-healthz-modified) |
| `control/api.go` | modified | [Control healthz](#control-healthz-modified) |
| `control/api_test.go` | modified | [Tests](#tests) |
| `docs/reference.md` | modified | [Docs](#docs-modified) |
| `docs/tools.md` | modified | [Docs](#docs-modified) |
| `Makefile`, `cmd/version.go`, `mcpb/manifest.json` | modified | [Release](#release-modified) |
| `plans/ready_state/design/raw.md` | created | this plan, persisted on approval |

### Store readiness (modified)

location: `session/store.go`

- Field `ready chan struct{}` added to `Store`, between `plainTitleById` and `sessions` (alphabetical), initialized in `NewStore`.
- Methods: see [H2](#h2).

```diff
 type Store struct {
 	mu sync.RWMutex
 
 	StateDir       *state.Dir
 	broker         *events.Broker
 	depth          int
 	enabledAgents  []Agent
 	plainTitleById map[Id]string
+	ready          chan struct{}
 	sessions       map[Id]*Session
 	snapshots      *snapshotCache
 }
 
 func NewStore(depth, diffCacheSessions int, broker *events.Broker, agents ...Agent) *Store {
 	return &Store{
 		sessions:       make(map[Id]*Session),
 		plainTitleById: make(map[Id]string),
 		depth:          depth,
 		enabledAgents:  agents,
 		broker:         broker,
+		ready:          make(chan struct{}),
 		snapshots:      newSnapshotCache(diffCacheSessions),
 	}
 }
```

### Watcher loaded signal (modified)

location: `watcher/watcher.go`

- Code: [H1](#h1).

### Codex index loaded signal (modified)

location: `watcher/codex_index_watcher.go`

- Code: [H3](#h3).

### Start aggregation and healthz (modified)

location: `cmd/start.go`
mirrors: `runStateGc` for `awaitInitialLoad`

- Watcher construction hoisted out of the `go func()` bodies so `Loaded()` can be collected into `loads` before the goroutine starts; `Run` error handling unchanged.
- Aggregation goroutine and function: [H4](#h4).
- `healthzHandler` gains the store parameter and the `ready` field.

```diff
 		broker := events.NewBroker()
 		store := session.NewStore(depth, diffCacheSessions, broker, agents...)
+		var loads []<-chan struct{}
 
 		...
 
 		if claudeHome != "" {
+			watchedDir := filepath.Join(claudeHome, claude.ProjectsDir)
+			newParser := func() watcher.Parser { return claude.NewParser() }
+			claudeWatcher := watcher.New(session.AgentClaude, watchedDir, watchWindow, newParser, store)
+			loads = append(loads, claudeWatcher.Loaded())
 			go func() {
-				watchedDir := filepath.Join(claudeHome, claude.ProjectsDir)
-				newParser := func() watcher.Parser { return claude.NewParser() }
-				err := watcher.New(session.AgentClaude, watchedDir, watchWindow, newParser, store).Run(ctx)
+				err := claudeWatcher.Run(ctx)
 				if err != nil && !errors.Is(err, context.Canceled) {
 					slog.Error("claude watcher error", "err", err)
 					os.Exit(1)
 				}
 			}()
 
 		...
 
 		if coworkHome != "" {
 			for _, name := range coworkStoreNames {
 				storeDir := filepath.Join(coworkHome, name)
 				if info, err := os.Stat(storeDir); err != nil || !info.IsDir() {
 					continue
 				}
+				newParser := func() watcher.Parser { return claude.NewParser() }
+				coworkWatcher := watcher.New(session.AgentClaude, storeDir, watchWindow, newParser, store)
+				coworkWatcher.TranscriptPathOk = isCoworkTranscriptPath
+				coworkWatcher.Project = "cowork"
+				loads = append(loads, coworkWatcher.Loaded())
 				go func() {
-					newParser := func() watcher.Parser { return claude.NewParser() }
-					w := watcher.New(session.AgentClaude, storeDir, watchWindow, newParser, store)
-					w.TranscriptPathOk = isCoworkTranscriptPath
-					w.Project = "cowork"
-					if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
+					if err := coworkWatcher.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
 						slog.Error("cowork watcher error", "err", err)
 						os.Exit(1)
 					}
 				}()
 			}
 		}
 
 		if codexHome != "" {
+			watchedDir := filepath.Join(codexHome, codex.SessionDir)
+			newParser := func() watcher.Parser { return codex.NewParser() }
+			codexWatcher := watcher.New(session.AgentCodex, watchedDir, watchWindow, newParser, store)
+			loads = append(loads, codexWatcher.Loaded())
 			go func() {
-				watchedDir := filepath.Join(codexHome, codex.SessionDir)
-				newParser := func() watcher.Parser { return codex.NewParser() }
-				err := watcher.New(session.AgentCodex, watchedDir, watchWindow, newParser, store).Run(ctx)
+				err := codexWatcher.Run(ctx)
 				if err != nil && !errors.Is(err, context.Canceled) {
 					slog.Error("codex watcher error", "err", err)
 					os.Exit(1)
 				}
 			}()
 
+			indexWatcher := watcher.NewCodexIndexWatcher(codexHome, store)
+			loads = append(loads, indexWatcher.Loaded())
 			go func() {
-				err := watcher.NewCodexIndexWatcher(codexHome, store).Run(ctx)
+				err := indexWatcher.Run(ctx)
 				if err != nil && !errors.Is(err, context.Canceled) {
 					slog.Error("codex index watcher error", "err", err)
 					os.Exit(1)
 				}
 			}()
 		}
+
+		go awaitInitialLoad(ctx, store, loads, startedAt)
 
 		go func() {
 			err := watcher.NewDiffWatcher(store, broker, pollInterval, pollWindow, stateDir).Run(ctx)
 
 		...
 
-			mux.HandleFunc("GET /healthz", healthzHandler(claudeHome, codexHome, boundControlPort))
+			mux.HandleFunc("GET /healthz", healthzHandler(claudeHome, codexHome, boundControlPort, store))
```

```diff
-func healthzHandler(claudeHome, codexHome string, controlPort int) http.HandlerFunc {
+func healthzHandler(claudeHome, codexHome string, controlPort int, store *session.Store) http.HandlerFunc {
 	return func(w http.ResponseWriter, r *http.Request) {
 		w.Header().Set("Content-Type", "application/json")
 		json.NewEncoder(w).Encode(map[string]any{
 			"version":     Version(),
 			"claudeHome":  claudeHome,
 			"codexHome":   codexHome,
 			"controlPort": controlPort,
+			"ready":       store.IsReady(),
 		})
 	}
 }
```

### Tool gate (modified)

location: `tools/tools.go`
mirrors: `counted`

- Gate function: [H5](#h5).
- Wiring: `awaitReady` sits inside `counted`, so timed-out calls are still counted.
- Imports: `time` added.

```diff
-var errSessionSelectorMissing = errors.New("id or title parameter is required")
+var (
+	errInitialLoadPending     = errors.New("initial session load still in progress; retry shortly")
+	errSessionSelectorMissing = errors.New("id or title parameter is required")
+)
 
 const (
 	DefaultReturnedTurns = 20
+	readyTimeout         = 2 * time.Minute
 )
```

```diff
 func Register(server *server.MCPServer, store *session.Store, counter *InvocationCounter, telemetryStore *telemetry.Store, detector *telemetry.Detector) {
 	// ...
-	server.AddTool(sessionGet, counted(counter, "session_get", sessionGetHandler(store, pageStore)))
+	server.AddTool(sessionGet, counted(counter, "session_get", awaitReady(store, readyTimeout, sessionGetHandler(store, pageStore))))
 	// ...
-	server.AddTool(sessionList, counted(counter, "session_list", sessionListHandler(store)))
+	server.AddTool(sessionList, counted(counter, "session_list", awaitReady(store, readyTimeout, sessionListHandler(store))))
 	// ...
-	server.AddTool(sessionEvents, counted(counter, "session_events", sessionEventsHandler(detector, store, eventsPageStore, telemetryStore)))
+	server.AddTool(sessionEvents, counted(counter, "session_events", awaitReady(store, readyTimeout, sessionEventsHandler(detector, store, eventsPageStore, telemetryStore))))
 }
```

### Control healthz (modified)

location: `control/viewmodels.go`, `control/api.go`

```diff
 type healthzResponse struct {
+	Ready   bool   `json:"ready"`
 	Status  string `json:"status"`
 	Version string `json:"version"`
 }
```

```diff
 func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
-	writeJSON(w, healthzResponse{Status: "ok", Version: s.version})
+	writeJSON(w, healthzResponse{Ready: s.store.IsReady(), Status: "ok", Version: s.version})
 }
```

### Docs (modified)

location: `docs/reference.md`, `docs/tools.md`

- `reference.md` `--port` row: identity-probe field list becomes `version`, `claudeHome`, `codexHome`, `controlPort`, `ready`; append "`ready` turns true once the initial session load completes".
- `tools.md` Common semantics gains one bullet:

```markdown
- **Startup wait** — right after peek starts, tool calls wait until the initial session load (all transcripts inside the watch window) is complete, so the first answer is never a partial session list. If the load takes longer than 2 minutes, the call returns `initial session load still in progress; retry shortly`.
```

### Release (modified)

location: `Makefile`, `cmd/version.go`, `mcpb/manifest.json`

- Final step after all feature commits: `make git-release VERSION=1.2.8` — produces commit `cmd: release v1.2.8` and local tag `v1.2.8`. No push.

## Hot items

### <a id="h1"></a>H1 — Watcher loaded channel (goroutines/channels)

location: `watcher/watcher.go`

- Risk: closed exactly once; a second close panics. Closed in `Run` directly after the first walk — the rescan ticker calls `walkAndWatch`, never this line.
- `loaded` sits between `horizon` and `mu` (alphabetical private group).

```diff
 type Watcher struct {
 	agent     session.Agent
 	agentDir  string
 	files     map[string]*watchedFile
 	horizon   time.Duration
+	loaded    chan struct{}
 	mu        sync.Mutex
 	newParser func() Parser
 	store     *session.Store
 
 func New(agent session.Agent, agentDir string, horizon time.Duration, newParser func() Parser, store *session.Store) *Watcher {
 	return &Watcher{
 		agent:     agent,
 		agentDir:  agentDir,
 		files:     make(map[string]*watchedFile),
 		horizon:   horizon,
+		loaded:    make(chan struct{}),
 		newParser: newParser,
 		store:     store,
 	}
 }
 
+func (w *Watcher) Loaded() <-chan struct{} {
+	return w.loaded
+}
+
 func (w *Watcher) Run(ctx context.Context) error {
 	// ...
 	// Add root directories and backfill existing files
 	w.walkAndWatch(watcher, w.agentDir)
 	w.store.SeedDiffCache()
+	close(w.loaded)
```

### <a id="h2"></a>H2 — Store ready channel (goroutines/channels)

location: `session/store.go`

- Risk: `MarkReady` has exactly one production caller (`awaitInitialLoad`); a second call panics — deliberate, a double-mark is a wiring bug.
- `IsReady` is a non-blocking probe for healthz; `Ready` is the blocking handle for the gate.

```go
func (s *Store) MarkReady() {
	close(s.ready)
}

func (s *Store) Ready() <-chan struct{} {
	return s.ready
}

func (s *Store) IsReady() bool {
	select {
	case <-s.ready:
		return true
	default:
		return false
	}
}
```

### <a id="h3"></a>H3 — Codex index loaded, missing-home branch (goroutines/channels + guard)

location: `watcher/codex_index_watcher.go`

- Risk: must close `loaded` on both paths exactly once — home present (after first `loadIndex`) and home absent (immediately, before `waitForDir` blocks) ([D5](#d5)).
- The absent path keeps today's semantics: after `waitForDir` succeeds, `loadIndex` runs once.

```diff
 type CodexIndexWatcher struct {
 	codexHome string
+	loaded    chan struct{}
 	store     *session.Store
 }
 
 func NewCodexIndexWatcher(codexHome string, store *session.Store) *CodexIndexWatcher {
 	return &CodexIndexWatcher{
 		codexHome: codexHome,
+		loaded:    make(chan struct{}),
 		store:     store,
 	}
 }
 
+func (w *CodexIndexWatcher) Loaded() <-chan struct{} {
+	return w.loaded
+}
+
 func (w *CodexIndexWatcher) Run(ctx context.Context) error {
 	watcher, err := fsnotify.NewWatcher()
 	if err != nil {
 		return err
 	}
 	defer watcher.Close()
 
-	if err := waitForDir(ctx, watcher, w.codexHome); err != nil {
-		return err
-	}
-
-	w.loadIndex()
+	if err := watcher.Add(w.codexHome); err != nil {
+		close(w.loaded)
+		if err := waitForDir(ctx, watcher, w.codexHome); err != nil {
+			return err
+		}
+		w.loadIndex()
+	} else {
+		w.loadIndex()
+		close(w.loaded)
+	}
 
 	debounce := time.NewTimer(indexDebounce)
```

### <a id="h4"></a>H4 — Initial-load aggregation (goroutines/channels)

location: `cmd/start.go`, placed after `runStateGc`

- Data flow: `loads` is fully built before the goroutine starts (all watchers constructed synchronously above it) — no concurrent append.
- Waits sequentially: total wait = slowest loader; order is irrelevant because every channel only ever closes.
- Empty `loads` (no homes configured) marks ready immediately.
- On shutdown before load completes, returns without marking — process is exiting.

```go
func awaitInitialLoad(ctx context.Context, store *session.Store, loads []<-chan struct{}, startedAt time.Time) {
	for _, loaded := range loads {
		select {
		case <-ctx.Done():
			return
		case <-loaded:
		}
	}

	store.MarkReady()
	slog.Info("awaitInitialLoad: Initial session load complete", "sessions", len(store.List()), "took", time.Since(startedAt).Round(time.Millisecond))
}
```

### <a id="h5"></a>H5 — Tool gate (guard logic)

location: `tools/tools.go`, placed after `counted`

- Ready → delegate unchanged. Client cancellation → return the context error (mcp-go maps it to a JSON-RPC error; the client has already gone). Timeout → tool error result, retryable ([D4](#d4)).
- Pagination follow-ups (`request_id`) pass the gate instantly — readiness never reverts.

```go
func awaitReady(store *session.Store, timeout time.Duration, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case <-store.Ready():
			return handler(ctx, request)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return mcp.NewToolResultError(errInitialLoadPending.Error()), nil
		}
	}
}
```

## Tests

Test skeletons mirror the nearest sibling in each file (table-driven `testCase` with `_id` / `_expected*` fields per [go-tests.md](context/go/go-tests.md)).

| Location.Method | Cases | Comment |
|---|---|---|
| `session/store_test.go` `TestStore_Ready` | fresh-store-not-ready<br>mark-ready-closes-channel-and-reports-ready | `Ready()` checked non-blocking via `select` |
| `watcher/watcher_test.go` `TestRun_Loaded` | loaded-after-transcripts-ingested<br>missing-root-still-signals-loaded | `Run` in a goroutine on a `t.TempDir()` with one codex transcript (fixture line from `TestWalkAndWatch_Horizon`); wait on `Loaded()` with a 5 s `time.After` guard; at signal time `store.GetById` must find the session; cancel ctx |
| `watcher/codex_index_watcher_test.go` `TestCodexIndexWatcher_Loaded` | home-present-titles-loaded-before-signal<br>home-missing-signals-immediately | uses `provideIndexWatcher`; missing case points `codexHome` at a non-existent child of `t.TempDir()` |
| `tools/tools_test.go` `TestAwaitReady` | ready-store-delegates-to-handler<br>pending-store-times-out-with-tool-error<br>cancelled-context-returns-context-error | 10 ms timeout; stub handler returns a sentinel result; timeout case asserts `errorText` equals the `errInitialLoadPending` message |
| `cmd/start_test.go` `TestAwaitInitialLoad` | all-loaders-closed-marks-ready<br>no-loaders-marks-ready<br>cancelled-context-leaves-not-ready | calls `awaitInitialLoad` synchronously with pre-closed / open channels |
| `cmd/start_test.go` `TestHealthzHandler` | existing assertions<br>ready-false-before-mark<br>ready-true-after-mark | updated for the new `store` parameter |
| `control/api_test.go` `TestHealthz` | existing assertions<br>ready-reflects-store | `newTestServer`'s store: false, then `MarkReady`, then true |

- Existing handler tests (`TestSessionGet_*`, `TestSessionList_*`, `TestSessionEvents_*`) call handlers directly, not through `Register` — unaffected. `TestRegister_ReadOnlyHint` only lists tools — unaffected.
- Not tested: `start` wiring of each watcher into `loads` — no unit seam around the cobra `Run`; covered by the running-system checks in Verification.

## Test runbook

- **fresh-instance-healthz:** `GET 127.0.0.1:4299/healthz` polled from process start — `ready` flips false → true; source: real `~/.claude`, `~/.codex`, cowork stores.
- **early-session_list:** MCP `tools/call session_list` sent immediately after `initialize` on a fresh http instance — response arrives after the ready log line and carries the full session count.
- **control-healthz:** `GET 127.0.0.1:<control port>/api/healthz` — carries `ready`.

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| MCP `/healthz` JSON gains `ready` | peek `healthzHandler` → smine configserver identity probe | additive field, status stays 200; `git grep -n healthz` in smine: only `cmd/configserver/main.go` + test read it, decoding named fields — no change needed |
| Control `/api/healthz` gains `ready` | peek control → dashboard / external probes | additive; `control/middleware.go:60` token-exemption by path unchanged |
| Tool calls block until ready | peek tools → every MCP client (Claude Code, Desktop, smine miners, `/peek` skill) | behavioral, documented in `docs/tools.md`; no response-shape change |
| `healthzHandler` signature | `cmd/start.go` ↔ `cmd/start_test.go` | `git grep -n "healthzHandler("` → exactly 2 call sites, both updated |

## Verification

- [ ] Run `make test` — expect all packages pass
- [ ] Run `go vet ./...` — expect no findings
- [ ] Run `make build-local` — expect `dist/peek-mcp` built
- [ ] Start a fresh instance: `./dist/peek-mcp start --port 4299 --control-port 42499 --log-level info` — expect log `awaitInitialLoad: Initial session load complete` with `sessions` > 0 and a `took` value; record `took` (must be well under 2 min, else S7)
- [ ] Within the first second after start, run `curl -s 127.0.0.1:4299/healthz | jq .ready` — expect `false` (1.5 GB load, [F1](#f1)); after the log line, rerun — expect `true`
- [ ] Run `curl -s 127.0.0.1:42499/api/healthz | jq .ready` after the log line — expect `true`
- [ ] Restart the instance; immediately send MCP `initialize` → `notifications/initialized` → `tools/call` `session_list` over `http://127.0.0.1:4299/mcp` (curl, `Mcp-Session-Id` from the initialize response header) — expect `initialize` to answer instantly, `session_list` to return only after the ready log line, and `jq '.result.structuredContent.sessions | length'` to equal the logged `sessions` count
- [ ] Degenerate: start with `--codex-home /tmp/peek-no-codex --cowork-home ""` — expect the ready log line (index watcher's missing-home path) and `/healthz` `ready: true`
- [ ] Degenerate: start with `--claude-home "" --codex-home "" --cowork-home ""` — expect ready immediately with `sessions=0`
- [ ] Stdio: `./dist/peek-mcp start --transport stdio --control-port 0` driven by a fresh Claude Code session calling `session_list` as its first action — expect the full list, no empty first answer
- [ ] Run `make git-release VERSION=1.2.8` — expect commit `cmd: release v1.2.8`, tag `v1.2.8`, and `./dist/peek-mcp version` after rebuild printing `1.2.8`

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | Stop and report. Never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | Stop, research the actual cause, redesign. No third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | Run the producing step. If infrastructure is down, ask. Never skip validation, never start infrastructure yourself |
| S4 | Discovered work materially exceeds the approved scope | Ask before continuing |
| S5 | Same kind of bug found a second time | Inside the diff: fix every instance now. Pre-existing, outside the diff: report and ask before sweeping |
| S6 | A structural obstacle (import cycle, package visibility) tempts a new abstraction (interface, DTO, wrapper) | Stop and report. The fix is relocating the component, not indirection |
| S7 | Measured `took` on the real tree exceeds ~60 s | Stop and report — the 2-minute timeout ([D4](#d4)) needs re-deciding |
| S8 | A blocked `tools/call` delays `initialize`, `ping`, or `tools/list` on either transport | Stop — [D1](#d1)'s premise ([F2](#f2)) is false |
| S9 | Any loader path found that never closes `loaded` (beyond [D5](#d5)'s missing codex home) | Stop and report — the instance would never become ready |

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
