# File-descriptor footprint (ENFILE saturation) — Implementation Plan

target: `fable` — design doc: the user's ad-hoc plan (pasted at invocation, binding; errata 2026-09-22 supersedes its D5: no new knob — see [D5](#d5))

## TLDR

- Not an unclosed-file leak: every transcript open is `defer Close()`d. The ~12,300 fds per instance are fsnotify's kqueue backend holding one fd per watched directory **and per file inside it**, by design, across the recursively-watched `~/.claude/projects` / cowork / codex trees.
- Fix: bound the watch set at directory granularity — only directories with activity inside the **existing ingest window** get `watcher.Add`; a periodic rescan re-walks the tree so cold directories that turn hot are picked up.
- **One window, one flag.** `--ingest-days` is replaced by `--watch-window-days` (default **14**, `0` = everything): the single window peek looks at — inside it, ingested and watched; outside it, neither. `state-retention-days` goes back to being a pure GC knob.
- Measured on the real tree: the old 90d ingest default would prune ~nothing (9,975 of 9,994 files are <90d); the 14d window prunes ~2–3x (~4–5k fds instead of ~9.4k for the projects tree) — enough that 17 instances stay far under `kern.maxfiles`.
- `walkAndWatch` also stops aborting the whole walk on the first `Add` error (today one EMFILE silently leaves the tree partially watched).
- Instance multiplication (17 concurrent stdio instances caused the machine-wide ENFILE on 2026-09-21) is a deployment-topology issue, deferred.

## Context

- Symptom (2026-09-21): 17 stale peek-mcp instances × ~12,300 fds saturated `kern.maxfiles` (368,627 / 368,640) → machine-wide ENFILE.
- Cause: [watcher.go:203](watcher/watcher.go#L203) `Add`s every directory in the tree; kqueue then opens one fd per directory **and per contained file** ([F2](#f2)).
- Live probe (2026-09-22, user-run, binding): healthy stdio instance pid 6888 holds 12,302 fds at steady state, matching tree size exactly — footprint, not leakage ([F4](#f4), [F5](#f5)).
- Design being implemented: the user's pasted ad-hoc plan (D1–D4, D6 verbatim) + errata: ONE window, one flag — `--watch-window-days 14` replaces `--ingest-days`; supersedes the pasted D5.
- Constraint: one lookback concept only — sessions inside the window are ingested and watched live; outside it, neither.

## Drivers

N/A — new route (the pasted plan is a design doc, not a change-route driver list).

## Scope

- **In:**
  - **watch-set bounding:** `walkAndWatch` prunes directories with no activity newer than the ingest window (`w.horizon` — already on the struct)
  - **rescan ticker:** `Watcher.Run` re-walks periodically so newly-hot directories get watched and their files ingested
  - **walk robustness:** `Add` errors logged per directory, walk continues
  - **flag replacement:** `--ingest-days` / `PEEK_INGEST_DAYS` deleted; `--watch-window-days` / `PEEK_WATCH_WINDOW_DAYS` (default 14, `0` = everything) is the single window; `state-retention-days` decoupled from ingest, GC only ([D9](#d9))
  - **tests:** watcher pruning + rescan + Add-failure coverage in [watcher_test.go](watcher/watcher_test.go)
  - **docs:** flag and env tables in [reference.md](docs/reference.md) updated (replacement + watch semantics + macOS fd rationale)
- **Out:**
  - **additional config surface:** no config-file key or control-dashboard row for the window — parity with the flag it replaces (`ingest-days` had flag+env only, [F15](#f15))
  - **instance multiplication:** per-session stdio topology (defaulting registrations to the shared http instance) — separate decision
  - **fsnotify replacement:** FSEvents backend or mtime-polling engine ([D2](#d2))
  - **plan/codex-index watchers:** `plan_watcher.go`, `codex_index_watcher.go` watch single small dirs — negligible footprint, untouched
- **Not changed:**
  - **`watcher.New` signature:** `horizon` already exists; no constructor change, no call-site churn
  - **readNewLines / offset bookkeeping:** unchanged; rescans are naturally idempotent through `w.files` offsets ([F9](#f9))
  - **state GC:** `state-retention-days` GC semantics untouched (only its double duty as ingest bound is removed)
- **Deferred findings:**
  - **instance multiplication:** 17 stale stdio instances were the actual saturation multiplier; `ServeStdio` exits on stdin EOF ([F8](#f8)), so stale = the client's pipe end stayed open. Candidate remedy: shared http instance as default registration.
  - **stale-instance forensics:** `ps -o ppid` was permission-denied during diagnosis; check parentage on the next stale instance manually.

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| "Leak from unclosed transcript files or watchers" (request premise) | Disproved — all open sites `defer Close()` or use `os.ReadFile`; one `fsnotify.Watcher` per tree with `defer watcher.Close()`; fd count matches current tree size exactly | [F1](#f1), [F4](#f4), [F5](#f5) |
| Stale instances ignore stdin EOF | Disproved — mcp-go `ServeStdio` returns on EOF; staleness means the clients' pipe ends stayed open | [F8](#f8) |
| Pasted plan: window pruning cuts fds "an order of magnitude" | Disproved by measurement — 90d prunes ~nothing; 14d prunes ~2–3x, which still resolves the machine-wide saturation (17 × ~4.5k ≈ 76k ≪ 368k) | [F14!](#f14) |
| Pasted plan: separate `--watch-window` knob needed | Superseded by invocation errata — one window; the existing `horizon` carries both ingest and watch | [D5](#d5) |

## Current state

N/A — new route.

## Target state

N/A — new route.

## Behavior contract

N/A — new route (live-event delivery for hot sessions is pinned via Verification instead).

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d1"></a>D1 | How to shrink the per-instance fd footprint | [F2!](#f2), [F3!](#f3), [F4!](#f4) | [USER] Prune the watch set at directory granularity by an activity window; kqueue then never opens fds for files in cold directories | kqueue offers no per-file opt-out under a watched dir, so directory granularity is the only lever fsnotify exposes |
| <a id="d2"></a>D2 | Why not FSEvents or polling | — | [USER] Rejected: FSEvents needs a darwin-only dependency and a second watch implementation; full polling replaces a working event pipeline | Fewest concepts: keep fsnotify, bound its input. Revisit only if the bounded footprint still bites ([S7](#stop-conditions)) |
| <a id="d3"></a>D3 | Cold directory turns hot (new session in an old project) — no event fires because the dir is unwatched | [F2!](#f2), [F9](#f9) | [USER] Periodic rescan: `Run` re-invokes `walkAndWatch` on a 5-min ticker.<br>Elaboration: ticker channel is nil when `horizon == 0` — a nil select case never fires, so the watch-everything mode costs nothing | Without it, pruning loses new sessions in cold projects; rescan is idempotent (`Add` on watched paths is a no-op, `w.files` offsets prevent re-ingest); ≤5 min staleness only for the first event of a cold project |
| <a id="d4"></a>D4 | Hotness criterion per directory | [F3!](#f3) | [USER] A directory is watched iff any file at-or-below it has ModTime within the window, computed bottom-up during the existing walk.<br>Elaborations: the dir's own ModTime also counts (a freshly created empty dir is hot before its first file lands); the tree root is always watched (create events for new project dirs must fire immediately) | Dir mtime alone misses deep subagent layouts; the walk already visits every entry, so newest-descendant mtime is free; propagation to ancestors keeps the path from root to every hot leaf watched |
| <a id="d5"></a>D5 | Window knob | [F14!](#f14), [F15](#f15) | [USER, errata] No parallel knob. The watch bound is the watcher's existing `horizon`: one window governs ingest **and** watch; `horizon == 0` means everything | Errata rejected a second concept; ingest and watch answer the same question ("how far back does peek look?"). Supersedes the pasted plan's `--watch-window` |
| <a id="d6"></a>D6 | Walk abort on `Add` failure | [F6](#f6) | [USER] Log and continue instead of returning the error from the WalkDir callback | Today one EMFILE aborts the whole walk, silently leaving the tree partially watched — worst behavior exactly under fd pressure |
| <a id="d7"></a>D7 | Restructured walk reads files before the `Add` pass — a write landing between read and `Add` raises no event | [F9](#f9) | Accept the gap: reads are offset-based, so the next write event on the same file recovers everything missed; a file whose *final* write lands in the gap is caught by the next rescan tick | The alternative (a second pre-pass walk that `Add`s first) doubles the walk for a gap the rescan already bounds at 5 min |
| <a id="d9"></a>D9 | Flag name and default: 90d prunes ~nothing ([F14](#f14)); `ingest-days` is a bad name and the state-retention coupling is a hidden second concept | [F14!](#f14), [F15](#f15) | [USER] Replace `--ingest-days` with `--watch-window-days`, default **14**, `0` = ingest and watch everything; drop the follow-`state-retention-days` fallback — `state-retention-days` becomes GC-only. Replace means gone: no alias, no `PEEK_INGEST_DAYS` compatibility read | "90 days is too much anyway. 14 days fits for all of this"; the direct default removes the fallback indirection, leaving exactly one source per concept (window → look-back, retention → GC). Trade-off accepted: `session_list` history shrinks to 14d unless overridden |

## Open questions

(empty — Q1 resolved into [D9](#d9))

## Baseline (verified)

Base branch: `main` (worktree branch `claude/file-descriptor-leak-48b041`).

| ID | Fact | Needed for | Location |
|---|---|---|---|
| <a id="f2"></a>F2! | kqueue backend keeps an open fd per watched entry: `internalWatch` → `unix.Open`; `Add` on a dir runs `watchDirectoryFiles`, which `internalWatch`es every file in it — unconditionally | [D1](#d1), [D3](#d3) | vendor/github.com/fsnotify/fsnotify/backend_kqueue.go:398, :430, :582–596 |
| <a id="f3"></a>F3! | `walkAndWatch` `Add`s every directory the walk visits; the ingest cutoff (`isBeforeCutoff`) only skips file reads, never `Add` | [D1](#d1), [D4](#d4), §1 | [watcher.go:203](watcher/watcher.go#L203), [watcher.go:209](watcher/watcher.go#L209) |
| <a id="f4"></a>F4! | Live probe (user-run, binding): pid 6888 holds 12,302 fds; 9,366 under `~/.claude/projects`, 50 under `~/.codex/sessions`, rest mostly the Cowork store | [D1](#d1), Verification baseline | `lsof -nP -p 6888`, 2026-09-22 |
| <a id="f14"></a>F14! | Window yield, measured on `~/.claude/projects` 2026-09-22: 9,994 files / 1,271 dirs total; 9,975 files <90d (90d prunes ~nothing); 363 dirs hold a <14d file, containing ~3,600–4,500 files → 14d ≈ 2–3x fd reduction; dominant fd mass is recent workflow-subagent dirs, untouchable by recency pruning | [D5](#d5), [D9](#d9), Assumptions | `find -mtime` counts, 2026-09-22 |
| <a id="f15"></a>F15 | `ingestHorizon` resolution: `state-retention-days` (default 90) unless `ingest-days` > 0; passed as `horizon` to all three tree watchers; `Watcher` already stores it | [D5](#d5), [D9](#d9), §2 | [start.go:72-75](cmd/start.go#L72), [watcher.go:94](watcher/watcher.go#L94) |
| <a id="f1"></a>F1 | All transcript-scan opens are closed: `defer file.Close()` in parser and watcher; other reads use `os.ReadFile` | Assumptions | [parser.go:644-649](claude/parser.go#L644), [watcher.go:252-256](watcher/watcher.go#L252) |
| <a id="f5"></a>F5 | `~/.claude/projects` at diagnosis: 840 dirs + 8,517 files ≈ the 9,366 project fds — footprint equals tree size, no accumulation | Assumptions | `find` counts, 2026-09-22 |
| <a id="f8"></a>F8 | mcp-go `ServeStdio` exits on stdin EOF | Assumptions, Deferred findings | vendor/github.com/mark3labs/mcp-go/server/stdio.go:451, :459 |
| <a id="f6"></a>F6 | The WalkDir callback returns the `watcher.Add` error, aborting the remainder of the walk | [D6](#d6) | [watcher.go:203-206](watcher/watcher.go#L203) |
| <a id="f9"></a>F9 | `w.files` offset map makes re-reads incremental; `readSubagentMeta` is once-per-path guarded | [D3](#d3), [D7](#d7) | [watcher.go:258-266](watcher/watcher.go#L258), [watcher.go:311](watcher/watcher.go#L311) |
| <a id="f7"></a>F7 | Three `watcher.New` construction sites (claude projects, cowork stores, codex sessions), each its own goroutine and `fsnotify.Watcher`; plan and codex-index watchers are separate types | §2, Scope | [start.go:121](cmd/start.go#L121), [start.go:146](cmd/start.go#L146), [start.go:161](cmd/start.go#L161) |
| <a id="f12"></a>F12 | `fsnotify.Watcher.WatchList()` returns the watched paths — usable for test assertions | §3 | vendor/github.com/fsnotify/fsnotify/backend_kqueue.go:337 |

Real data inspected: live fd table (`lsof`), diagnosis-time tree counts, and the 2026-09-22 `find -mtime` age distribution ([F14](#f14)); transcript fixtures in [watcher_test.go](watcher/watcher_test.go).

## Exemplar & reuse

| Existing | Used for |
|---|---|
| `isBeforeCutoff` ([watcher.go:60](watcher/watcher.go#L60)) | the cutoff computation — the `Add` gate reuses the same cutoff value the read gate already derives from `w.horizon` |
| `runStateGc` ticker loop ([start.go:349](cmd/start.go#L349)) | ticker lifecycle pattern (`NewTicker` + `defer Stop`) |
| `w.files` offsets ([F9](#f9)) | rescan idempotence — no new bookkeeping needed |
| `TestWalkAndWatch_Horizon` ([watcher_test.go:27](watcher/watcher_test.go#L27)) | test fixture shape (temp tree + `Chtimes` + store assertions) |

- Every change has an exemplar; none is novel machinery.

## Changes

| File | Kind | Entry |
|---|---|---|
| watcher/watcher.go | modified | §1 |
| cmd/start.go | modified | §2 |
| watcher/watcher_test.go | modified | §3 |
| docs/reference.md | modified | §4 |

### 1. Watch-set pruning and rescan (modified)

location: `watcher/watcher.go`

- **No struct or constructor change** — the gate reuses `w.horizon` ([D5](#d5)).
- **Rescan interval constant** next to `eventChannelBuffer`:

```go
const rescanInterval = 5 * time.Minute
```

- **`walkAndWatch` restructured** — one `WalkDir` collecting dirs and per-dir newest ModTime, file handling unchanged in place; then a bottom-up fold and a gated `Add` pass ([D1](#d1), [D4](#d4), [D6](#d6), [D7](#d7)). Complete final function (the `subagentPaths` tail loop stays byte-identical and is elided):

```go
func (w *Watcher) walkAndWatch(watcher *fsnotify.Watcher, root string) {
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		slog.Warn("walkAndWatch: not a directory", "path", root)
		return
	}

	var subagentPaths []string
	cutoff := time.Time{}
	if w.horizon > 0 {
		cutoff = time.Now().Add(-w.horizon)
	}

	var dirs []string
	newest := make(map[string]time.Time)

	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			if entry.IsDir() {
				dirs = append(dirs, path)
				newest[path] = info.ModTime()
			} else if info.ModTime().After(newest[filepath.Dir(path)]) {
				newest[filepath.Dir(path)] = info.ModTime()
			}
		}
		if entry.IsDir() {
			return nil
		}
		if isBeforeCutoff(entry, cutoff) {
			return nil
		}
		if isSubagentPath(path) {
			subagentPaths = append(subagentPaths, path)
			return nil
		}
		if w.isTranscriptPath(path) {
			err = w.readNewLines(path)
			if err != nil {
				slog.Warn("walkAndWatch: readNewLines", "err", err)
			}
		}
		return nil
	})
	if err != nil {
		slog.Error("walkAndWatch error", "err", err)
	}

	for i := len(dirs) - 1; i > 0; i-- {
		parent := filepath.Dir(dirs[i])
		if newest[dirs[i]].After(newest[parent]) {
			newest[parent] = newest[dirs[i]]
		}
	}

	for _, dir := range dirs {
		if dir != root && !cutoff.IsZero() && newest[dir].Before(cutoff) {
			continue
		}
		if err := watcher.Add(dir); err != nil {
			slog.Warn("walkAndWatch: watcher.Add", "path", dir, "err", err)
		}
	}

	// ... subagentPaths loop unchanged ...
}
```

- **One window, two gates:** `isBeforeCutoff` keeps gating file reads; the same `cutoff` now also gates `Add`. A dir with no in-window descendant has nothing to read *and* nothing to watch — the semantics collapse cleanly.
- **Fold correctness:** `WalkDir` is pre-order, so every parent precedes its children in `dirs`; the reverse loop folds each child's newest into its parent before the parent is read by *its* parent.
- **`Run` gains the rescan case** ([D3](#d3)) — diff (hot item, example in [Hot items](#hot-items)):

```diff
 func (w *Watcher) Run(ctx context.Context) error {
 	// ...
 	w.walkAndWatch(watcher, w.agentDir)
 	w.store.SeedDiffCache()
 
+	var rescan <-chan time.Time
+	if w.horizon > 0 {
+		ticker := time.NewTicker(rescanInterval)
+		defer ticker.Stop()
+		rescan = ticker.C
+	}
+
 	for {
 		select {
 		case <-ctx.Done():
 			return ctx.Err()
+		case <-rescan:
+			w.walkAndWatch(watcher, w.agentDir)
 		case event, ok := <-watcher.Events:
```

- **Create events for directories** keep the existing immediate `walkAndWatch(watcher, path)` in `processBatch` — a just-created dir is hot by definition (its own ModTime is fresh), so the gated `Add` pass watches it.

### 2. Flag replacement (modified)

location: `cmd/start.go`

- **Flag definition** — replace, don't alias ([D9](#d9)); `state-retention-days` help loses its ingest clause:

```diff
-	flags.Int("state-retention-days", 90, "Days to keep per-session state before GC removes it, and how far back startup ingests transcripts (0 disables both)")
-	flags.Int("ingest-days", 0, "How far back startup ingests transcripts (0 = follow state-retention-days)")
+	flags.Int("state-retention-days", 90, "Days to keep per-session state before GC removes it (0 disables)")
+	flags.Int("watch-window-days", 14, "How far back peek ingests transcripts and watches directories for live activity (0 = everything; macOS holds one fd per watched file)")
```

- **Horizon resolution** — the fallback indirection dies with the old flag:

```diff
-		ingestDays, _ := flags.GetInt("ingest-days")
-		ingestHorizon := time.Duration(stateRetentionDays) * 24 * time.Hour
-		if ingestDays > 0 {
-			ingestHorizon = time.Duration(ingestDays) * 24 * time.Hour
-		}
+		watchWindowDays, _ := flags.GetInt("watch-window-days")
+		watchWindow := time.Duration(watchWindowDays) * 24 * time.Hour
```

  (the three `watcher.New` sites at [start.go:121](cmd/start.go#L121), :146, :161 pass `watchWindow` where they passed `ingestHorizon` — same param position, no signature change)
- **Env fallback:**

```diff
-	"ingest-days":             "PEEK_INGEST_DAYS",
+	"watch-window-days":       "PEEK_WATCH_WINDOW_DAYS",
```

- **Post-replace check:** `grep -rn "ingest-days\|ingestDays\|ingestHorizon\|PEEK_INGEST_DAYS" --include="*.go"` → zero hits.

### 3. Tests (modified)

location: `watcher/watcher_test.go`

mirrors: `TestWalkAndWatch_Horizon` ([watcher_test.go:27](watcher/watcher_test.go#L27))

- New `TestWalkAndWatch_WatchPruning` using `fsWatcher.WatchList()` ([F12](#f12)) for watched/unwatched assertions; no signature churn — existing tests keep compiling unchanged.
- Details in [Tests](#tests).

### 4. Docs (modified)

location: `docs/reference.md`

- Flag table: `--ingest-days` row replaced by `--watch-window-days` (default `14`): bounds what is ingested **and** which directories are watched live (macOS: one fd per watched file); cold projects are picked up within 5 min of new activity; `0` = everything.
- `--state-retention-days` row loses its ingest clause (GC only).
- Env table: `PEEK_INGEST_DAYS` row replaced by `PEEK_WATCH_WINDOW_DAYS`.

## Hot items

Goroutines/channels/locking (baseline hot class 2): the `Run` select-loop change. Approved example — nil-channel gating so `horizon == 0` (watch everything) adds no ticker at all (a receive on a nil channel never fires):

```go
func (w *Watcher) Run(ctx context.Context) error {
	watcher, err := fsnotify.NewBufferedWatcher(eventChannelBuffer)
	if err != nil {
		return err
	}
	defer watcher.Close()

	w.walkAndWatch(watcher, w.agentDir)
	w.store.SeedDiffCache()

	var rescan <-chan time.Time
	if w.horizon > 0 {
		ticker := time.NewTicker(rescanInterval)
		defer ticker.Stop()
		rescan = ticker.C
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-rescan:
			w.walkAndWatch(watcher, w.agentDir)
		case event, ok := <-watcher.Events:
			if !ok {
				slog.Info("watcher closed")
				return nil
			}
			batch := newEventBatch()
			batch.add(event)
			batch.drain(watcher.Events)
			w.processBatch(watcher, batch)

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Error("watcher error", "err", err)
		}
	}
}
```

- No new goroutine, no new lock: the rescan runs on the existing `Run` goroutine, serialized with event processing by the select loop itself; `walkAndWatch`'s file reads take `w.mu` exactly as today.

## Tests

| Location.Method | Cases | Comment |
|---|---|---|
| watcher_test.go `TestWalkAndWatch_WatchPruning` | cold-dir-not-watched (file + dir `Chtimes` beyond horizon → dir absent from `WatchList()`)<br>cold-dir-not-ingested (same window — read and watch gates agree)<br>deep-hot-file-keeps-ancestor-chain-watched<br>root-always-watched-even-when-cold<br>new-file-in-cold-dir-then-rescan → dir watched and file ingested (direct `walkAndWatch` call as the forced rescan)<br>zero-horizon-watches-everything | pruning + [D3](#d3)/[D4](#d4) elaborations; `WatchList()` per [F12](#f12) |
| watcher_test.go `TestWalkAndWatch_AddFailure` | closed-watcher-add-fails-walk-still-ingests (call `walkAndWatch` with a `Close()`d fsnotify watcher; fresh transcript still lands in the store) | [D6](#d6) — no abort on `Add` error |
| watcher_test.go `TestWalkAndWatch_Horizon` (existing) | unchanged — pins the read gate | safety net; must stay green untouched |

- Not tested: the 5-min ticker firing in real time — the rescan path is exercised by calling `walkAndWatch` directly; ticker wiring is covered by Verification against the live tree.

## Test runbook

Scenario index (no request files — `runbook` arg absent):

- **fd-footprint-default** — `lsof -nP -p <pid> | wc -l` against a default-flags instance (14d window) on the real `~/.claude`; compare against [F14](#f14)'s prediction (~4–5k for the projects tree).
- **live-session-events** — `session_list` / `session_get` / `session_events` MCP tools against a hot session.
- **cold-project-pickup** — start a Claude session in a >14-day-cold project dir, `session_list` until it appears.
- **escape-hatch** — restart with `--watch-window-days 0`, repeat fd-footprint-default (expect ~12.3k, today's behavior).

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| one window, two gates (`w.horizon` drives read **and** watch) | `walkAndWatch` read gate / `Add` gate / `Run` ticker gate | `TestWalkAndWatch_WatchPruning` cold-dir-not-ingested + cold-dir-not-watched pin both gates to the same cutoff; `grep -n "cutoff" watcher/watcher.go` shows a single derivation site |
| `ingest-days` → `watch-window-days` replacement ([D9](#d9), replace = gone) | flag def / env map / local vars / docs tables / launchd or setup manifests that pass the old flag | `grep -rn "ingest-days\|ingestDays\|ingestHorizon\|PEEK_INGEST_DAYS" .` (excluding this plan) → zero hits; survivors justified per-hit or fixed |
| `state-retention-days` = GC only | `runStateGc` / flag help / docs row | `grep -rn "state-retention" --include="*.go" docs/` — no hit mentions ingest after the change |
| public API unchanged | `watcher.New` / all callers | `git diff main -- watcher/watcher.go` shows no exported-signature change; build is the gate |

## Verification

- [ ] Run `make test` — green, including untouched `TestWalkAndWatch_Horizon`.
- [ ] Run the replacement sweep — `grep -rn "ingest-days\|ingestDays\|ingestHorizon\|PEEK_INGEST_DAYS" --include="*.go" docs/` → zero hits.
- [ ] Run `make build-local`; start with defaults (14d window) against the real `~/.claude`.
- [ ] Run `lsof -nP -p <pid> | wc -l` — expect ~4–6k total ([F14](#f14) prediction); record the number in this plan.
- [ ] Exercise `session_list` / `session_get` / `session_events` repeatedly; re-run the lsof count — stays flat.
- [ ] Write to a transcript in a hot project — the new turn appears via `session_events` (live-event delivery intact).
- [ ] Start a Claude session in a >14-day-cold project — it appears in `session_list` within one rescan tick (≤5 min).
- [ ] Confirm `session_list` shows sessions up to 14 days back and none older (window semantics, [D9](#d9) trade-off visible).
- [ ] Restart with `--watch-window-days 0` — fd count returns to the ~12.3k baseline (everything mode works).

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | Stop and report; never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | Stop, research the actual cause, redesign; no third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | Run the producing step; if infrastructure is down, ask — never skip validation, never start infrastructure yourself |
| S4 | Discovered work materially exceeds the approved scope | Ask before continuing |
| S5 | Same kind of bug found a second time | Inside own diff: fix every instance now. Pre-existing outside the diff: report and ask before sweeping |
| S6 | A structural obstacle (import cycle, package visibility) tempts a new abstraction | Stop and report; the fix is relocating the component, not indirection |
| S7 | Pruning breaks live-event delivery for any currently-hot session (Verification hot-session or cold-pickup item fails twice) | Stop and reassess against [D2](#d2) alternatives rather than patching the walk further |

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| 2026-09-22 | initial | plan created from the user's pasted ad-hoc plan; facts re-anchored to worktree `claude/file-descriptor-leak-48b041` |
| 2026-09-22 | errata: no new knob | measured window yield ([F14](#f14)); `--watch-window` and all config plumbing deleted — watch bound reuses the existing ingest `horizon` ([D5](#d5) rewritten); planned files cut from 8 to 4; [D9](#d9) OPEN on the default window |
| 2026-09-22 | Q: default window & flag name | [D9](#d9) resolved [USER]: `--ingest-days` replaced by `--watch-window-days`, default 14, `0` = everything; `state-retention-days` decoupled to GC-only; §2 concretized, sweeps and verification updated |
