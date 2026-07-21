# Diff Target Inference — Implementation Plan

## TLDR

- `session_diff` stops needing a configured base branch: the base is inferred per session branch from the live checkout — reflog creation entry first, then `origin/HEAD`, then local `main`/`master`, terminal `HEAD`.
- `--diff-target` / `PEEK_DIFF_TARGET` / mcpb `diff_target` are deleted; `NewDiffWatcher` loses its target parameter.
- Diffs switch to merge-base semantics so an advanced base no longer pollutes session diffs with upstream commits — **via `git diff --merge-base <base>`, not the concept's three-dot form, which verification showed excludes the working tree** (see [Assumptions](#assumptions)).
- The resolved base is exposed as `diff_target` in `session_full` and `session_list`; `session_diff` stays pure diff text.
- Inference is cached per (cwd, branch) for the server lifetime; tests cover the full chain against real scratch git repos.

## Context

- Problem: `session_diff` diffs against a hardcoded `main` ([diff_watcher.go:73](watcher/diff_watcher.go:73), flag default [start.go:165](cmd/start.go:165)) — wrong for `master`/`develop` repos, worktree branches forked from feature branches, and remote-less repos.
- Design being implemented: [plans/diff_target_inference/concept/concept.md](plans/diff_target_inference/concept/concept.md) — binding, all questions resolved.
- One concept assumption was falsified during grounding (three-dot diff semantics, [F1!](#f1)); the replacement `--merge-base` preserves every stated concept intent and stays a one-process change ([D1](#d1)).
- Constraint: `git diff --merge-base` requires git ≥ 2.30 (Jan 2021); the dev machine runs 2.39.
- Constraint: zero code comments in new code (test grouping comments required by go-tests.md excepted).

## Scope

- **In:**
  - **inference chain:** reflog → `origin/HEAD` → local `main`/`master` → `HEAD`, in the watcher package.
  - **cache:** inferred base cached per (cwd, branch) on `DiffWatcher`.
  - **merge-base diff:** `refresh` diffs `--merge-base <base>` instead of two-dot `main`.
  - **config removal:** flag, env var, mcpb manifest arg + user_config, `NewDiffWatcher` target parameter.
  - **exposure:** `diff_target` JSON field in `session_full` and `session_list`; `Session.DiffTarget` tag change.
  - **docs:** tool descriptions in `tools.go`, README sections.
  - **tests:** new `watcher/diff_watcher_test.go` against real scratch repos.
- **Out:**
  - **uncommitted-diff pipeline:** `pollAll`/`pollRepo` (`git diff HEAD`) untouched.
  - **release mechanics:** version bumps, `.mcpb` publishing.
- **Not changed:**
  - **session_diff contract:** raw applyable diff text, no marker lines.
  - **store API:** `UpdateDiff(id, target, output)` already carries the target.
  - **pagination mechanics:** `PageBuilder` chunking unchanged.
- **Deferred findings:**
  - **sync.Map fields:** existing `running`/`polling`/`lastDiff` violate RULE-GR-007 — pre-existing, not touched.
  - **method order:** `Run` precedes private methods in `diff_watcher.go`, violating RULE-FILE-001 — pre-existing, not reshuffled.
  - **manifest version drift:** `mcpb/manifest.json` says 1.0.3 while v1.0.5 is released; `build-mcpb-only` copies it verbatim.

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| Concept: three-dot `git diff <base>...` gives merge-base semantics as "a one-argument change", HEAD terminal yields the uncommitted diff, base==current yields the working-tree diff | **False.** Three-dot with one arg diffs merge-base→HEAD, **excluding the working tree**: in a scratch repo the uncommitted line was absent, and `git diff HEAD...` is empty — contradicting two [USER] concept intents. `git diff --merge-base <base>` delivers exactly the intended semantics in one process ([F2!](#f2)) | verified in scratch repo (see [F1!](#f1)/[F2!](#f2)) |
| Concept: dropping `json:"-"` on `Session.DiffTarget` exposes it | `Session` is never marshaled whole — turns are marshaled at [tools.go:190](tools/tools.go:190), `session_list` uses explicit `sessionListItem` fields. Tag change applied per concept but inert; real exposure = new viewmodel fields | tools/tools.go:190, tools/viewmodels.go:32 |
| Concept: cache "keyed by session cwd — one checked-out branch per worktree" | True for worktrees; false for main-checkout sessions (Codex sessions in the primary repo switch branches). Cache key extended to (cwd, branch) — same rationale ("a branch's creation point never changes"), applied per branch ([D3](#d3)) | session/meta.go:6 |
| Concept: reflog oldest entry is `branch: Created from <ref>` | Confirmed on real data: this worktree's branch records `branch: Created from refs/remotes/origin/improvement/infer-target-repo` (fully qualified); plain `checkout -b` records `branch: Created from HEAD` | live repo + scratch repo |

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d1"></a>D1 | Concept's three-dot diff form is falsified — what implements merge-base semantics? | [F1!](#f1), [F2!](#f2) | `git diff --merge-base <base>` — flagged deviation from the concept's literal `<base>...` spelling | Preserves all three [USER]-stated outcomes (merge-base semantics, `HEAD` terminal → uncommitted diff, base==current → working-tree diff), still one git process; three-dot delivers none of the working-tree outcomes |
| <a id="d2"></a>D2 | Concept fallback 2 says strip `origin/` from the `origin/HEAD` result — but keep or strip? | [F2!](#f2), [F3!](#f3) | [USER] Keep the remote-qualified base (`origin/main`) — deviation from the concept's "strip `origin/`" | Consistent with the reflog signal, which already yields `origin/`-prefixed bases (this very branch: `origin/improvement/infer-target-repo`), and avoids stale-local-`main` noise: a lagging local `main` moves the merge-base backwards and re-imports upstream commits — exactly what the merge-base decision kills |
| <a id="d3"></a>D3 | Cache key: concept says cwd; main-checkout repos switch branches within a server lifetime | [F3!](#f3) | Key by (cwd, branch) via a `diffBaseKey` struct — flagged refinement of the concept's cwd-only key | Concept's own justification is per-branch ("a branch's creation point never changes"); cwd-only serves a stale base after a branch switch in a non-worktree checkout. Cost: one `git symbolic-ref` per refresh — noise next to the existing 2 git calls. Controllable and debuggable: the key states exactly what the cached value depends on |
| <a id="d4"></a>D4 | Cache container: siblings use `sync.Map`; RULE-GR-007 forbids it | [F4!](#f4) | `sync.Mutex` + `map[diffBaseKey]string`, mutex declared directly above the map | Style guide overrides sibling pattern ("fix violations rather than debating them"); existing `sync.Map` fields stay (out of scope) |
| <a id="d5"></a>D5 | Which branch feeds the reflog lookup — live checkout or `Meta.GitBranch`? | [F3!](#f3) | Live: `git symbolic-ref --quiet --short HEAD`; error (detached) → `"HEAD"` | Concept: "inference from the live repo, not session data"; transcript meta can be stale; symbolic-ref also resolves unborn branches where rev-parse fails |
| <a id="d6"></a>D6 | Where does the inference code live? | [F4!](#f4) | Package-level functions in `watcher/diff_watcher.go`; `diffBase` method owns the cache; `inferDiffBase` stays a separate function as the chain orchestrator | Single consumer, no new file/package concept; `inferDiffBase` separate from `diffBase` keeps cache and chain independently testable and respects RULE-NEST-001 |
| <a id="d7"></a>D7 | Where does `diff_target` go in paginated `session_full` responses? | [F5!](#f5) | Field on `sessionFullResult`, set on the first page only | Continuation pages are prebuilt without a session reference ([tools.go:149-166](tools/tools.go:149)); the first page is where clients read metadata; threading the target through `PageStore` adds state for no consumer benefit |
| <a id="d8"></a>D8 | `gitAbsoluteDir` duplicates the exec-output pattern the new code needs | [F6!](#f6) | Add `gitOutput` + `gitSucceeds` helpers; delete `gitAbsoluteDir` (its single caller inlines `gitOutput(ctx, cwd, "rev-parse", "--absolute-git-dir")`); `gitReady` uses `gitSucceeds` | Single source of truth for "run git, get stdout" / "run git, check exit"; `gitOutput` gains 4 callers, `gitSucceeds` 4 — both clear RULE-FUNC-002 |

- Inline decisions (recorded, not asked): errors from git during inference are treated as "signal absent" and fall through the chain — the chain's terminal `HEAD` guarantees a base; `gitOutput` returns the bare exec error (mirrors the deleted `gitAbsoluteDir`, `logDiffErr` unwraps stderr); reflog discard set is exactly empty/`HEAD`/`claude/*` per concept.

## Baseline (verified)

Base branch: `main` (worktree branch `claude/diff-target-inference-concept-1dbc4c`).

| ID | Fact | Needed for | Location |
|---|---|---|---|
| <a id="f1"></a>F1! | `git diff main...` in a scratch repo with an uncommitted edit omitted the uncommitted line; `git diff main` showed it (plus reversed upstream noise) | [D1](#d1), Assumptions | scratch repo verification |
| <a id="f2"></a>F2! | `git diff --merge-base main` showed committed+uncommitted branch changes and no upstream commits; `--merge-base HEAD` = uncommitted only; `--merge-base <current-branch>` = uncommitted only; works with `-- . :!pattern` pathspecs; git 2.39 local, flag exists since 2.30 | [D1](#d1), [D2](#d2), [§1](#c1) | scratch repo verification |
| <a id="f3"></a>F3! | Real reflog data: worktree branch records `branch: Created from refs/remotes/origin/improvement/infer-target-repo` (remote-prefixed base survives stripping as `origin/...`); plain `checkout -b` records `branch: Created from HEAD`; `git symbolic-ref refs/remotes/origin/HEAD` errors when unset | [D2](#d2), [D3](#d3), [D5](#d5), [§8](#c8) | live repo + scratch repo |
| <a id="f4"></a>F4! | `DiffWatcher` holds `target string` used only in `refresh`; three `sync.Map` fields exist; `NewDiffWatcher(store, target, interval, window)` has exactly one caller | [D4](#d4), [D6](#d6), [§1](#c1), [§2](#c2) | [diff_watcher.go:17-34](watcher/diff_watcher.go:17), [start.go:112](cmd/start.go:112) |
| <a id="f5"></a>F5! | `session_full` continuation pages are drained from a `PageStore` channel keyed by request id, with no session reference at continuation time | [D7](#d7), [§4](#c4) | [tools.go:149-166](tools/tools.go:149), [pages.go:40-50](tools/pages.go:40) |
| <a id="f6"></a>F6! | `gitAbsoluteDir` is byte-identical in shape to the needed `gitOutput` helper; single caller `pollRepo` | [D8](#d8), [§1](#c1) | [diff_watcher.go:148-156](watcher/diff_watcher.go:148) |
| F7 | `Store.UpdateDiff(id, target, output)` already persists the target per session | [§1](#c1) | [store.go:104-113](session/store.go:104) |
| F8 | Flag surface: definition, read, env-fallback map entry | [§2](#c2) | [start.go:42](cmd/start.go:42), [start.go:165](cmd/start.go:165), [start.go:202](cmd/start.go:202) |
| F9 | `Session.DiffTarget` is `json:"-"`; `sessionListItem` has explicit fields; `sessionFullResult` is `{turns, plan, diff}` | [§3](#c3), [§4](#c4) | [session.go:36](session/session.go:36), [viewmodels.go:9-41](tools/viewmodels.go:9) |
| F10 | mcpb manifest passes `--diff-target=${user_config.diff_target}` and declares the `diff_target` user_config; binary + manifest ship atomically (`server.type: binary`) | [§6](#c6) | [manifest.json:29](mcpb/manifest.json:29), [manifest.json:68-74](mcpb/manifest.json:68) |
| F11 | README mentions the flag/env/target in 8 places: git-diffs bullet, `session_full`/`session_list`/`session_diff` descriptions, flags table, env table, Codex toml example, limitations note | [§7](#c7) | README.md:37,41,58,81,161,177,210,263 |
| F12 | No test file exists in `tools/`; `watcher/watcher_test.go` is the package test exemplar (testify, `t.Helper` builders); go-tests.md defines the table style | Tests | [watcher_test.go:14-22](watcher/watcher_test.go:14) |
| F13 | Test store setup: `AddTurnBySessionId` with a `Turn{Meta: &Meta{SessionId, CWD}, Role: RoleUser, Text, Timestamp}` creates a session; `refresh` is directly callable | [§8](#c8) | [store.go:55](session/store.go:55), [turn.go:9-20](session/turn.go:9) |

## Exemplar & reuse

| Existing | Used for |
|---|---|
| `gitDiff` variadic args + pathspec excludes ([diff_watcher.go:201](watcher/diff_watcher.go:201)) | merge-base diff — call gains `"--merge-base", base` args, function unchanged |
| `Store.UpdateDiff` ([store.go:104](session/store.go:104)) | persisting the inferred base — no store change |
| `resolve_from()` in claude-configs `cmd/worktrees/print_agent_worktrees_status.sh` | reflog resolver logic mirrored in Go (`baseFromReflog`) |
| `logDiffErr` ([diff_watcher.go:214](watcher/diff_watcher.go:214)) | error logging for the merge-base diff failure path |

- Without exemplar: the inference chain itself (`inferDiffBase` + three `baseFrom*` functions) — no Go sibling exists; the shell `resolve_from()` covers only the reflog step. Risk contained by full code in this plan and repo-backed tests.

## Changes

### <a id="c1"></a>1. Diff base inference, cache, and merge-base diff (modified)

location: `watcher/diff_watcher.go`
mirrors: `resolve_from()` (claude-configs shell) for `baseFromReflog`; existing `gitDiff`/`gitReady` exec style for the helpers

Struct and constructor — `target` deleted, cache added (mutex directly above its map per RULE-GR-007):

```diff
+type diffBaseKey struct {
+	branch string
+	cwd    string
+}
+
 type DiffWatcher struct {
 	store    *session.Store
-	target   string
 	interval time.Duration
 	window   time.Duration
 	running  sync.Map // session.Id -> struct{}; one in-flight turn-diff per session
 	polling  sync.Map // cwd -> struct{}; one in-flight poll per repo
 	lastDiff sync.Map // gitDir -> string; last written uncommitted diff, to skip no-op writes
+
+	baseMu    sync.Mutex
+	baseByKey map[diffBaseKey]string
 }

-func NewDiffWatcher(store *session.Store, target string, interval, window time.Duration) *DiffWatcher {
+func NewDiffWatcher(store *session.Store, interval, window time.Duration) *DiffWatcher {
 	return &DiffWatcher{
 		store:    store,
-		target:   target,
 		interval: interval,
 		window:   window,
+		baseByKey: make(map[diffBaseKey]string),
 	}
 }
```

`refresh` — infer, diff with merge-base semantics:

```diff
 func (w *DiffWatcher) refresh(ctx context.Context, id session.Id, cwd string) {
 	defer w.running.Delete(id)

 	if !gitReady(ctx, cwd) {
 		return
 	}

-	output, err := gitDiff(ctx, cwd, w.target)
+	base := w.diffBase(ctx, cwd)
+	output, err := gitDiff(ctx, cwd, "--merge-base", base)
 	if err != nil {
 		logDiffErr(string(id), "git diff", err)
 		return
 	}
-	w.store.UpdateDiff(id, w.target, output)
-	slog.Debug("DiffWatcher: refreshed diff", "session", id, "target", w.target, "bytes", len(output))
+	w.store.UpdateDiff(id, base, output)
+	slog.Debug("DiffWatcher: refreshed diff", "session", id, "base", base, "bytes", len(output))
 }
```

New method `diffBase` — cache wrapper, lock never held across an exec:

```go
func (w *DiffWatcher) diffBase(ctx context.Context, cwd string) string {
	branch, err := gitOutput(ctx, cwd, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		branch = "HEAD"
	}

	key := diffBaseKey{branch: branch, cwd: cwd}
	w.baseMu.Lock()
	base, isCached := w.baseByKey[key]
	w.baseMu.Unlock()
	if isCached {
		return base
	}

	base = inferDiffBase(ctx, branch, cwd)
	w.baseMu.Lock()
	w.baseByKey[key] = base
	w.baseMu.Unlock()
	return base
}
```

New package-level chain (complete units):

```go
func inferDiffBase(ctx context.Context, branch, cwd string) string {
	if base := baseFromReflog(ctx, branch, cwd); base != "" {
		return base
	}
	if base := baseFromOriginHead(ctx, cwd); base != "" {
		return base
	}
	if base := baseFromLocalDefault(ctx, cwd); base != "" {
		return base
	}
	return "HEAD"
}
```

```go
func baseFromReflog(ctx context.Context, branch, cwd string) string {
	if branch == "HEAD" {
		return ""
	}

	output, err := gitOutput(ctx, cwd, "reflog", "show", "--format=%gs", branch)
	if err != nil || output == "" {
		return ""
	}

	lines := strings.Split(output, "\n")
	oldest := lines[len(lines)-1]
	created, isCreationEntry := strings.CutPrefix(oldest, "branch: Created from ")
	if !isCreationEntry {
		return ""
	}

	base := strings.TrimPrefix(created, "refs/remotes/")
	base = strings.TrimPrefix(base, "refs/heads/")
	if base == "" || base == "HEAD" {
		return ""
	}
	if strings.HasPrefix(base, "claude/") {
		return ""
	}

	for _, ref := range []string{"refs/heads/" + base, "refs/remotes/" + base} {
		if gitSucceeds(ctx, cwd, "show-ref", "--verify", "--quiet", ref) {
			return base
		}
	}
	return ""
}
```

`baseFromOriginHead` — keeps the remote-qualified name per [D2](#d2):

```go
func baseFromOriginHead(ctx context.Context, cwd string) string {
	name, err := gitOutput(ctx, cwd, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return ""
	}

	if !gitSucceeds(ctx, cwd, "rev-parse", "--verify", "--quiet", name) {
		return ""
	}
	return name
}
```

```go
func baseFromLocalDefault(ctx context.Context, cwd string) string {
	for _, name := range []string{"main", "master"} {
		if gitSucceeds(ctx, cwd, "rev-parse", "--verify", "--quiet", "refs/heads/"+name) {
			return name
		}
	}
	return ""
}
```

```go
func gitOutput(ctx context.Context, cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
```

```go
func gitSucceeds(ctx context.Context, cwd string, args ...string) bool {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	return cmd.Run() == nil
}
```

Consolidation per [D8](#d8) — `gitReady` delegates, `gitAbsoluteDir` deleted, its caller inlined:

```diff
 func gitReady(ctx context.Context, cwd string) bool {
 	if _, err := os.Stat(cwd); err != nil {
 		return false
 	}
-	check := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
-	check.Dir = cwd
-	return check.Run() == nil
+	return gitSucceeds(ctx, cwd, "rev-parse", "--git-dir")
 }

-func gitAbsoluteDir(ctx context.Context, cwd string) (string, error) {
-	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--absolute-git-dir")
-	cmd.Dir = cwd
-	out, err := cmd.Output()
-	if err != nil {
-		return "", err
-	}
-	return strings.TrimSpace(string(out)), nil
-}
```

```diff
 func (w *DiffWatcher) pollRepo(ctx context.Context, cwd string) {
 	// ...
-	gitDir, err := gitAbsoluteDir(ctx, cwd)
+	gitDir, err := gitOutput(ctx, cwd, "rev-parse", "--absolute-git-dir")
 	if err != nil {
 		return
 	}
```

### <a id="c2"></a>2. Delete the config surface (modified)

location: `cmd/start.go`

```diff
 	claudeHome, _ := flags.GetString("claude-home")
 	codexHome, _ := flags.GetString("codex-home")
-	diffTarget, _ := flags.GetString("diff-target")
 	pollInterval, _ := flags.GetDuration("poll-interval")
```

```diff
 	go func() {
-		err := watcher.NewDiffWatcher(store, diffTarget, pollInterval, pollWindow).Run(ctx)
+		err := watcher.NewDiffWatcher(store, pollInterval, pollWindow).Run(ctx)
 		if err != nil && !errors.Is(err, context.Canceled) {
```

```diff
 	flags.String("codex-home", defaultHome(".codex"), "Codex session root")
-	flags.String("diff-target", "main", "Branch to diff against for session_diff")
 	flags.Duration("poll-interval", time.Second*5, "How often to recompute the live uncommitted diff (git diff HEAD)")
```

```diff
 var envFallbacks = map[string]string{
 	// ...
 	"codex-home":    "PEEK_CODEX_HOME",
-	"diff-target":   "PEEK_DIFF_TARGET",
 	"poll-interval": "PEEK_POLL_INTERVAL",
```

### <a id="c3"></a>3. Session model tag (modified)

location: `session/session.go`

```diff
 	DiffOutput      string      `json:"-"`
-	DiffTarget      string      `json:"-"`
+	DiffTarget      string      `json:"diff_target,omitempty"`
 	UncommittedDiff string      `json:"-"` // git diff HEAD, refreshed by the poller
```

### <a id="c4"></a>4. Expose diff_target in the viewmodels (modified)

location: `tools/viewmodels.go`, `tools/tools.go`
mirrors: existing field style of `sessionFullResult` / `sessionListItem`

```diff
 type sessionFullResult struct {
 	Turns string `json:"turns,omitempty"`
 	Plan  string `json:"plan,omitempty"`
 	Diff  string `json:"diff,omitempty"`
+	DiffTarget string `json:"diff_target,omitempty"`
 }
```

```diff
 type sessionListItem struct {
 	// ...
 	HasPlan     bool                `json:"has_plan"`
 	HasDiff     bool                `json:"has_diff"`
+	DiffTarget  string              `json:"diff_target,omitempty"`
 	Meta        session.Meta        `json:"meta"`
 }
```

Handler wiring — first page only per [D7](#d7):

```diff
 func sessionFullHandler(s *session.Store, pageStore *PageStore) server.ToolHandlerFunc {
 	// ...
 	firstPage, nextPages := NewPageBuilder(maxResponseBytes(ctx)).build(turns, plan, diff)
+	firstPage.DiffTarget = sess.DiffTarget

 	resultPage := newSessionFullResultPage(firstPage)
```

```diff
 func sessionListHandler(s *session.Store) server.ToolHandlerFunc {
 	// ...
 		items[i] = sessionListItem{
 			Id:          sess.Meta.SessionId,
 			Agent:       sess.Agent,
 			Title:       sess.Title,
 			TitleSource: sess.TitleSource,
 			LastActive:  sess.LastActive,
 			HasPlan:     sess.PlanContent != "" || sess.PlanFilePath != "",
 			HasDiff:     sess.DiffOutput != "",
+			DiffTarget:  sess.DiffTarget,
 			Meta:        sess.Meta,
 		}
```

### <a id="c5"></a>5. Tool descriptions (modified)

location: `tools/tools.go`

```diff
 	sessionList :=
 		mcp.NewTool("session_list",
-			mcp.WithDescription("Lists all sessions. Returns session ID, agent, last activity timestamp, whether a plan or diff is available, and session metadata (cwd, git branch, model, origin)."),
+			mcp.WithDescription("Lists all sessions. Returns session ID, agent, last activity timestamp, whether a plan or diff is available, the inferred diff base branch (diff_target), and session metadata (cwd, git branch, model, origin)."),
```

```diff
 	sessionDiff :=
 		mcp.NewTool("session_diff",
-			mcp.WithDescription("Returns the pre-computed git diff for a session. The diff is run against the configured target branch (default: main) in the session's working directory, and refreshed automatically on each new turn. If id is omitted, uses the most recent session."),
+			mcp.WithDescription("Returns the pre-computed git diff for a session. The base branch is inferred from the session's live checkout (branch creation point from the reflog, falling back to origin/HEAD, then local main/master, then HEAD) and the diff uses merge-base semantics, refreshed automatically on each new turn. The resolved base is exposed as diff_target in session_full and session_list. If id is omitted, uses the most recent session."),
```

### <a id="c6"></a>6. mcpb manifest (modified)

location: `mcpb/manifest.json`

Final `args` and `user_config` content (diff_target removed from both; rest unchanged):

```json
"args": [
  "start",
  "--transport=stdio",
  "--depth=${user_config.depth}",
  "--claude-home=${user_config.claude_home}",
  "--codex-home=${user_config.codex_home}"
],
```

```json
"user_config": {
  "depth": {
    "type": "number",
    "title": "Ring buffer depth",
    "description": "Maximum turns kept per session.",
    "default": 50,
    "min": 1,
    "max": 500,
    "required": true
  },
  "claude_home": {
    "type": "directory",
    "title": "Claude Code home",
    "description": "Override the Claude Code session root.",
    "default": "${HOME}/.claude",
    "required": true
  },
  "codex_home": {
    "type": "directory",
    "title": "Codex CLI home",
    "description": "Override the Codex session root.",
    "default": "${HOME}/.codex",
    "required": true
  }
},
```

### <a id="c7"></a>7. README (modified)

location: `README.md`

```diff
-- **Git diffs** — After each new turn, peek-mcp runs `git diff <target-branch>` in the session's working directory and caches the result. `session_diff` and `session_full` expose this so a reviewer model can see exactly what changed without reading source files.
+- **Git diffs** — After each new turn, peek-mcp infers the session branch's base (reflog creation point, falling back to `origin/HEAD`, then local `main`/`master`, then `HEAD`) and runs `git diff --merge-base <base>` in the session's working directory. `session_diff` and `session_full` expose the result — no configuration needed; the resolved base is reported as `diff_target`.
```

```diff
-**`session_diff`** Returns the pre-computed git diff for a session, run against the configured target branch (default: `main`) and refreshed automatically on each new turn.
+**`session_diff`** Returns the pre-computed git diff for a session, run with merge-base semantics against the automatically inferred base branch and refreshed on each new turn.
```

- `session_list` description (line 58): append `diff_target` to the returned-fields enumeration, matching [§5](#c5).
- Flags table (line 161): delete the `--diff-target` row.
- Env table (line 177): delete the `PEEK_DIFF_TARGET` row.
- Codex toml example (line 210): drop `"--diff-target=main"` from the args array.

```diff
-- `session_diff` requires a local `git` binary in `PATH` and runs in the session's working directory. It will produce no output if the directory is not a git repository or the target branch does not exist.
+- `session_diff` requires a local `git` binary (≥ 2.30, for `git diff --merge-base`) in `PATH` and runs in the session's working directory. It produces no output if the directory is not a git repository.
```

### <a id="c8"></a>8. Tests (new)

location: `watcher/diff_watcher_test.go`
mirrors: `watcher/watcher_test.go` helper style ([watcher_test.go:14](watcher/watcher_test.go:14)); table shape per go-tests.md

Full code in [Tests](#tests) — helper, chain table test, cache test, refresh integration test.

## Hot items

| Class | Item | Example |
|---|---|---|
| Goroutines, channels, and locking | `baseMu` + `baseByKey` cache on `DiffWatcher`, read/written from concurrent `refresh` goroutines | full implementation in [§1](#c1): struct fields, `NewDiffWatcher` init, `diffBase` — lock scoped to map access only, never held across `exec`; a double inference race is idempotent (same repo state → same base) and the second write is a no-op overwrite |

- No SQL, no new interfaces or generics, no migrations, no weakened guards, no anonymous structs.

## Tests

| Location.Method | Cases | Comment |
|---|---|---|
| watcher/diff_watcher_test.go `TestInferDiffBase` | reflog-local-branch<br>reflog-remote-ref<br>reflog-created-from-head-falls-back<br>reflog-claude-branch-discarded<br>reflog-base-deleted<br>origin-head-fallback<br>local-probe-master<br>terminal-head<br>detached-head | table-driven; each append block builds its own scratch repo via `gitRun`; origin-head-fallback expects `origin/develop` per [D2](#d2) |
| watcher/diff_watcher_test.go `TestDiffWatcher_DiffBase` | cached-after-base-deleted<br>branch-switch-reinfers | cache behavior: deleting the base branch after first resolution keeps the cached value; switching the checked-out branch in the same cwd triggers fresh inference (key includes branch, [D3](#d3)) |
| watcher/diff_watcher_test.go `TestDiffWatcher_Refresh` | merge-base-excludes-upstream | integration: main advances after fork; branch has one commit + one uncommitted edit; `refresh` stores a diff containing both branch changes, not the upstream file, and `DiffTarget == "main"` |

Test skeleton (helper + table shape; the full case list follows the table above):

```go
package watcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func initRepo(t *testing.T, defaultBranch string) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", defaultBranch)
	gitRun(t, dir, "commit", "--allow-empty", "-m", "initial")
	return dir
}

func TestInferDiffBase(t *testing.T) {
	type testCase struct {
		_expected string
		_id       string
		branch    string
		cwd       string
	}

	tests := make([]*testCase, 0)

	// reflog-local-branch
	dir := initRepo(t, "main")
	gitRun(t, dir, "branch", "develop")
	gitRun(t, dir, "checkout", "-b", "feature", "develop")
	tests = append(tests, &testCase{
		_id:       "reflog-local-branch",
		_expected: "develop",
		branch:    "feature",
		cwd:       dir,
	})

	// reflog-remote-ref
	dir = initRepo(t, "main")
	gitRun(t, dir, "update-ref", "refs/remotes/origin/develop", "HEAD")
	gitRun(t, dir, "checkout", "-b", "feature", "origin/develop")
	tests = append(tests, &testCase{
		_id:       "reflog-remote-ref",
		_expected: "origin/develop",
		branch:    "feature",
		cwd:       dir,
	})

	// ... remaining cases per the Tests table, same construction pattern

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			assert.Equal(t, test._expected, inferDiffBase(context.Background(), test.branch, test.cwd))
		})
	}
}
```

`TestDiffWatcher_Refresh` setup (integration, no table):

```go
func TestDiffWatcher_Refresh(t *testing.T) {
	dir := initRepo(t, "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shared.txt"), []byte("shared\n"), 0o644))
	gitRun(t, dir, "add", "shared.txt")
	gitRun(t, dir, "commit", "-m", "shared")
	gitRun(t, dir, "checkout", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("committed\n"), 0o644))
	gitRun(t, dir, "add", "feature.txt")
	gitRun(t, dir, "commit", "-m", "feature work")
	gitRun(t, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "upstream.txt"), []byte("upstream\n"), 0o644))
	gitRun(t, dir, "add", "upstream.txt")
	gitRun(t, dir, "commit", "-m", "upstream advance")
	gitRun(t, dir, "checkout", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("committed\nuncommitted\n"), 0o644))

	store := session.NewStore(10, session.AgentClaude)
	turn := &session.Turn{
		Meta:      &session.Meta{SessionId: "sess-1", CWD: dir},
		Role:      session.RoleUser,
		Text:      "hello",
		Timestamp: time.Now(),
	}
	store.AddTurnBySessionId("sess-1", session.AgentClaude, turn)

	w := NewDiffWatcher(store, time.Second, 0)
	w.refresh(context.Background(), "sess-1", dir)

	sess, ok := store.GetById("sess-1")
	require.True(t, ok)
	assert.Equal(t, "main", sess.DiffTarget)
	assert.Contains(t, sess.DiffOutput, "+committed")
	assert.Contains(t, sess.DiffOutput, "+uncommitted")
	assert.NotContains(t, sess.DiffOutput, "upstream")
}
```

- Not tested: `tools/` handler field wiring — no test file exists in `tools/` (F12); covered by e2e verification.
- Not tested: `cmd/start.go` flag removal — compile plus e2e covers it.
- Not tested: README / manifest edits — covered by the sweep greps.

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| `--diff-target` flag / `PEEK_DIFF_TARGET` env (removed) | Go (`cmd/start.go`), README (flags table, env table, Codex toml example), mcpb manifest args | repo-wide grep for `diff-target` and `PEEK_DIFF_TARGET` → zero hits outside `plans/` |
| mcpb `user_config.diff_target` (removed) | `mcpb/manifest.json` only; installed bundles upgrade atomically (F10) | grep `diff_target` in `mcpb/` → zero hits |
| `diff_target` JSON field (added) | Go (`session/session.go` tag, `tools/viewmodels.go`), README tool descriptions, `tools.go` descriptions | grep `diff_target` → hits only in the four intended files + plans |
| `NewDiffWatcher` signature (narrowed) | Go only — single caller | compile |

## Verification

- [ ] Run `gofmt`/`goimports` on touched files and `go vet ./...` — clean.
- [ ] Run `make test` — all packages pass, including the three new watcher tests.
- [ ] Run `grep -rn "diff-target\|PEEK_DIFF_TARGET" --exclude-dir=vendor --exclude-dir=plans .` — zero hits.
- [ ] Run `make build-local`, then `make serve-http`, and from a Claude session in this worktree call `session_diff` — expect a diff against `origin/improvement/infer-target-repo` (this branch's real reflog base) containing only this branch's changes.
- [ ] Call `session_list` — the entry for this session carries `"diff_target": "origin/improvement/infer-target-repo"`.
- [ ] Call `session_full` — first page carries `diff_target`; paginated continuation pages still drain correctly.
- [ ] Degenerate: a session whose cwd is not a git repo yields no diff and no `diff_target` (field omitted).
- [ ] Degenerate: a scratch remote-less repo with only `master` and a `checkout -b` branch resolves `diff_target` to `master`.

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | Stop and report. Never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | Stop, research the actual cause, redesign. No third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | Run the producing step. If infrastructure is down, ask. Never skip validation, never start infrastructure yourself |
| S4 | Discovered work materially exceeds the approved scope | Ask before continuing |
| S5 | Same kind of bug found a second time: inside the diff → fix every instance now; pre-existing outside the diff | Report and ask before searching further |
| S6 | A structural obstacle (import cycle, package visibility) tempts a new abstraction | Stop and report. The fix is relocating the component, not indirection |
| S7 | A git version in a test/CI environment emits a reflog `%gs` format that breaks `baseFromReflog` parsing | Stop and report the observed format before changing the parser |
| S8 | `git diff --merge-base` behaves differently than verified (F2) in any real-repo verification step | Stop and report — do not fall back to two-dot or three-dot forms silently |

## Open questions

None.

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
| 2026-07-21 | Q: strip `origin/` in the `origin/HEAD` fallback? | D2 resolved to [USER] option b — keep the remote-qualified base; Tests table expectation fixed |
