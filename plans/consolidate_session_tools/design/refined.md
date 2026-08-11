# Consolidate session tools — Implementation Plan

## TLDR

- Collapse the five per-session data tools (`session_full`, `session_latest`, `session_get`, `session_plan`, `session_diff`, `session_uncommitted_diff` — six names, five behaviors) into one tool: `session_get`.
- No `id`/`title` → the most recently active session (agent resolved via the existing single-agent default).
- Section selection via flat boolean params — `turns`, `plan`, `diff` (default true), `uncommitted_diff` (default false) — the default equals the current `session_full` shape.
- The uncommitted diff joins the paginated result as a fourth section; pagination (`request_id` / `has_more`) carries over unchanged.
- `session_list` stays as the discovery tool. Clean break: the five old tool names are gone, docs/manifest/skill swept.
- New handler tests in `tools/` (currently untested) plus stdio runbooks.

## Context

- Six MCP tools overlap heavily: four handlers duplicate the identical latest-session fallback verbatim ([tools/tools.go:169-183](tools/tools.go#L169), :295-309, :321-335, :343-357), and all read the same `id`/`title`/`agent`/`n` selector set.
- The user's request: one `session_get` for all per-session data, latest by default, params for everything else — a breaking tool-surface change, explicitly wanted.
- `session_full` is already the documented "prefer this" aggregator with pagination; the consolidated `session_get` is `session_full` renamed, extended by the section flags and the uncommitted diff.
- Constraint: `respond()` client shaping (Claude text / others structured, [tools/respond.go:16](tools/respond.go#L16)) and the `PageStore` mechanics stay untouched.

## Scope

- **In:**
  - **session_get:** consolidated tool with `id`, `title`, `agent`, `n`, section flags (`turns`, `plan`, `diff`, `uncommitted_diff`), `request_id`
  - **removals:** `session_full`, `session_latest`, `session_plan`, `session_diff`, `session_uncommitted_diff` — registrations, handlers, descriptions
  - **pagination:** `uncommitted_diff` as fourth priority stream in `PageBuilder`
  - **renames:** `sessionFullResult*` → `sessionGetResult*` viewmodels
  - **docs sweep:** README, CONTRIBUTING, `mcpb/manifest.json`, `skills/peek/SKILL.md`
  - **tests:** new `tools/tools_test.go` and `tools/pages_test.go`
  - **runbooks:** stdio smoke scripts under `plans/consolidate_session_tools/runbooks/`
- **Out:**
  - **session_list:** unchanged (see [D2](#d2))
  - **control server:** HTTP API and dashboard untouched
  - **release:** manifest `version` / Makefile `VERSION` bump happens in Kevin's release commit, not here
  - **pagination redesign:** `PageStore`/`request_id` mechanics kept as-is
- **Not changed:**
  - **resolveSession:** id > title precedence, title matching semantics
  - **ResolveAgent:** single-agent default / multi-agent required error
  - **respond:** per-client text vs structured shaping, 100KB/0 page sizes
- **Deferred findings:**
  - **out-of-repo consumers:** `~/.claude/skills/peek` (user-global) and the `anthropic-skills:peek` copy route to `session_full`/`session_plan`/`session_diff` — they live outside this repo and need a separate sync
  - **historical plans:** `plans/*/` design docs mention old tool names — historical records, not swept

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| Request premise: the data tools are parameter-compatible enough to merge | Verified — all five share the `id`/`title`/`agent` selector and four already share the identical latest-fallback idiom | [tools/tools.go:169-183](tools/tools.go#L169) |
| Request premise: "no id or title → latest" is new behavior | Partially — four tools already fall back to latest; only today's `session_get` errors instead ([F3](#f3)) | [tools/tools.go:273-276](tools/tools.go#L273) |

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d1"></a>D1 | Which tools merge | [F1!](#f1), [F2!](#f2) | [USER] One `session_get` replaces `session_full`, `session_latest`, `session_plan`, `session_diff`, `session_uncommitted_diff`. Clean break, no aliases, no deprecation shims | Stated request; "replace" means the old names are gone |
| <a id="d2"></a>D2 | Does `session_list` merge too | — | Keep `session_list` as a separate tool | It returns the multi-session roster, not one session's data; merging would fork `session_get` into two output shapes — two concepts in one tool instead of one |
| <a id="d3"></a>D3 | Agent requirement on the latest path | [F4!](#f4) | Reuse `resolveAgentFromRequest` → `ResolveAgent`: `agent` required only when no `id`/`title` AND more than one agent is enabled | Existing semantics of the four fallback handlers; single-agent installs stay zero-config |
| <a id="d4"></a>D4 ⟲ | Section selection mechanism | [F5!](#f5) | [USER] Flat boolean params `turns`, `plan`, `diff` (default true) and `uncommitted_diff` (default false) — no `include` array | User rejected the array ("Include is bad ... Flat"); defaults keep today's `session_full` shape; `uncommitted_diff` stays opt-in |
| <a id="d5"></a>D5 ⟲ | Unknown `include` value | — | Dropped with the `include` array — boolean params need no value validation; unknown args are ignored per MCP convention | Follows from revised D4 |
| <a id="d6"></a>D6 | Where `uncommitted_diff` fits in pagination | [F6!](#f6) | Fourth, lowest-priority drain stream in `PageBuilder.build` | Single pagination mechanism for all sections; priority order turns → plan → diff → uncommitted_diff mirrors the existing priority comment |
| <a id="d7"></a>D7 | Viewmodel naming | [F1!](#f1) | Rename `sessionFullResult`/`sessionFullResultPage` → `sessionGetResult`/`sessionGetResultPage`; `PageStore` map type follows | The type is named after the tool that returns it; keeping "full" after the tool dies leaves a dangling concept |
| <a id="d8"></a>D8 | Empty-section behavior | [F1!](#f1) | Sections absent from the session are omitted via `omitempty` — no "No plan found" / "No turns found" sentinel texts; only the no-session case keeps the `no sessions found` text | `session_full` parity: JSON key absence is the machine-readable signal; per-tool sentinel strings were an artifact of single-purpose tools |
| <a id="d9"></a>D9 | Manifest handling | [F8](#f8) | Update `tools` array to the two remaining tools and fix `long_description`; leave `version` at 1.0.8 | Tools array must match the binary in the same commit; version bumps live in release commits (repo pattern: "cmd: release v1.0.8") |

## Baseline (verified)

Base branch: `main` (worktree branch `claude/consolidate-session-tools-c165e5`).

| ID | Fact | Needed for | Location |
|---|---|---|---|
| <a id="f1"></a>F1! | `session_full` is already the aggregator: id/title/n/agent selector, latest fallback, pagination via `PageStore`, `omitempty` result fields | [D1](#d1), [D7](#d7), [D8](#d8), §4 | [tools/tools.go:144-213](tools/tools.go#L144) |
| <a id="f2"></a>F2! | The latest-fallback idiom (`errSessionSelectorMissing` → `resolveAgentFromRequest` → `s.Last`) is duplicated verbatim in 4 handlers | [D1](#d1) | [tools/tools.go:169-183](tools/tools.go#L169) |
| <a id="f3"></a>F3 | Today's `session_get` returns the resolver error when id and title are omitted — no fallback | Assumptions | [tools/tools.go:273-276](tools/tools.go#L273) |
| <a id="f4"></a>F4! | `Store.ResolveAgent`: empty agent → the sole enabled agent when exactly one is enabled, else error "agent parameter is required" | [D3](#d3) | [session/store.go:54](session/store.go#L54) |
| <a id="f5"></a>F5! | mcp-go v0.52.0 has `WithArray` and `WithStringEnumItems` for array-of-enum params | [D4](#d4) | mcp-go@v0.52.0/mcp/tools.go:1234, :1385 |
| <a id="f6"></a>F6! | `PageBuilder.build(turns, plan, diff)` drains sections into fixed-size pages by priority; last stream has no trailing size bookkeeping | [D6](#d6), §3 | [tools/pages.go:67-111](tools/pages.go#L67) |
| <a id="f7"></a>F7! | `respond` shapes per client (Claude → text JSON, others → structured); `maxResponseBytes`: Claude 100KB, Codex 0 (no pagination) | §4, Tests | [tools/respond.go:16-49](tools/respond.go#L16) |
| <a id="f8"></a>F8 | Manifest `tools` array omits `session_uncommitted_diff` today; `long_description` names 3 tools | [D9](#d9) | [mcpb/manifest.json:7,37-42](mcpb/manifest.json#L37) |
| <a id="f9"></a>F9! | `tools/` package has zero test files; resolution logic is tested in `session/store_test.go` (testify `assert`, fixture builder `provideCompleteStore`) | Tests | session/store_test.go:15-48 |
| <a id="f10"></a>F10 | Session exposes `PlanContent`, `DiffOutput`, `UncommittedDiff`, `DiffTarget` as exported fields | §4, Tests | [tools/tools.go:196-200](tools/tools.go#L196), :359 |
| <a id="f11"></a>F11 | Runbook convention: bash scripts under `plans/<slug>/runbooks/`, env-var base URL/binary, jq assertions | Test runbook | plans/control_server/runbooks/turns_default.sh |
| <a id="f12"></a>F12 | `make test` and `make build-local` targets exist | Verification | Makefile:35, :84 |

## Exemplar & reuse

| Existing | Used for |
|---|---|
| `resolveSession` / `resolveAgentFromRequest` / `resolveAgentFilter` ([tools/tools.go:365-401](tools/tools.go#L365)) | Selector resolution — unchanged, reused |
| `PageStore` / `PageBuilder` / `UTF8SafeSlice` ([tools/pages.go](tools/pages.go)) | Pagination — extended by one stream, mechanics reused |
| `respond` / `respondWithText` / `respondWithStructured` / `maxResponseBytes` ([tools/respond.go](tools/respond.go)) | Client-shaped responses — unchanged |
| `intArgFromRequest` ([tools/forms.go:12](tools/forms.go#L12)) | `n` parsing — unchanged |
| `withMaxResultSize` ([tools/tools.go:21](tools/tools.go#L21)) | Tool meta — unchanged |

- Without exemplar: none — `boolArgFromRequest` mirrors `intArgFromRequest` in `tools/forms.go`.

## Changes

### 1. Result viewmodels renamed, uncommitted diff field added (modified)

location: [tools/viewmodels.go](tools/viewmodels.go)

```diff
-type sessionFullResult struct {
-	Turns      string `json:"turns,omitempty"`
-	Plan       string `json:"plan,omitempty"`
-	Diff       string `json:"diff,omitempty"`
-	DiffTarget string `json:"diff_target,omitempty"`
+type sessionGetResult struct {
+	Turns           string `json:"turns,omitempty"`
+	Plan            string `json:"plan,omitempty"`
+	Diff            string `json:"diff,omitempty"`
+	DiffTarget      string `json:"diff_target,omitempty"`
+	UncommittedDiff string `json:"uncommitted_diff,omitempty"`
 }

-type sessionFullResultPage struct {
-	*sessionFullResult
+type sessionGetResultPage struct {
+	*sessionGetResult
 	RequestId string `json:"request_id,omitempty"`
 	HasMore   bool   `json:"has_more"`
 }

-func newSessionFullResultPage(result *sessionFullResult) *sessionFullResultPage {
-	return &sessionFullResultPage{
-		sessionFullResult: result,
+func newSessionGetResultPage(result *sessionGetResult) *sessionGetResultPage {
+	return &sessionGetResultPage{
+		sessionGetResult: result,
 	}
 }

-func (p *sessionFullResultPage) WithRequestId(id string) {
+func (p *sessionGetResultPage) WithRequestId(id string) {
 	p.HasMore = true
 	p.RequestId = id
 }
```

- `sessionListItem` stays unchanged.

### 2. PageStore type rename (modified)

location: [tools/pages.go:10-57](tools/pages.go#L10)

- Mechanical rename following [D7](#d7): every `*sessionFullResult` in `PageStore`, `add`, `next` becomes `*sessionGetResult`. Locking and channel mechanics untouched.

```diff
 type PageStore struct {
 	mu               sync.Mutex
-	PagesByRequestId map[string]<-chan *sessionFullResult
+	PagesByRequestId map[string]<-chan *sessionGetResult
 }
```

### 3. PageBuilder gains the uncommitted-diff stream (modified)

location: [tools/pages.go:67-111](tools/pages.go#L67)

```diff
-func (b *PageBuilder) build(turns, plan, diff string) (first *sessionFullResult, next []*sessionFullResult) {
-	// Check if everything fits in a single page
-	contentSize := len(turns) + len(plan) + len(diff)
+func (b *PageBuilder) build(turns, plan, diff, uncommittedDiff string) (first *sessionGetResult, next []*sessionGetResult) {
+	contentSize := len(turns) + len(plan) + len(diff) + len(uncommittedDiff)
 	if b.Size <= 0 || contentSize <= b.Size {
 		slog.Info("PageBuilder.build: fits in a single page", "size", contentSize, "page_size", b.Size)
-		first = &sessionFullResult{
-			Turns: turns,
-			Plan:  plan,
-			Diff:  diff,
+		first = &sessionGetResult{
+			Turns:           turns,
+			Plan:            plan,
+			Diff:            diff,
+			UncommittedDiff: uncommittedDiff,
 		}
 		return first, nil
 	}

-	// Check how many pages we need to build, round up
 	pageCount := math.Ceil(float64(contentSize) / float64(b.Size))
-	pages := make([]*sessionFullResult, int(pageCount))
+	pages := make([]*sessionGetResult, int(pageCount))
 	slog.Info("PageBuilder.build: building", "pageCount", pageCount, "size", b.Size)

 	for i := 0; i < int(pageCount); i++ {
-		pages[i] = &sessionFullResult{}
+		pages[i] = &sessionGetResult{}
 		size := b.Size

-		// drain turns, plan and diff into pages by priority
 		turnChunk := UTF8SafeSlice(turns, size)
 		pages[i].Turns = turnChunk
 		turns = turns[len(turnChunk):]
 		if len(turnChunk) == size {
 			continue
 		}
 		size = size - len(turnChunk)

 		planChunk := UTF8SafeSlice(plan, size)
 		pages[i].Plan = planChunk
 		plan = plan[len(planChunk):]
 		if len(planChunk) == size {
 			continue
 		}
 		size = size - len(planChunk)

 		diffChunk := UTF8SafeSlice(diff, size)
 		pages[i].Diff = diffChunk
 		diff = diff[len(diffChunk):]
+		if len(diffChunk) == size {
+			continue
+		}
+		size = size - len(diffChunk)
+
+		uncommittedChunk := UTF8SafeSlice(uncommittedDiff, size)
+		pages[i].UncommittedDiff = uncommittedChunk
+		uncommittedDiff = uncommittedDiff[len(uncommittedChunk):]
 	}

 	return pages[0], pages[1:]
 }
```

### 4. Consolidated tool registration and handler (modified)

location: [tools/tools.go](tools/tools.go)
mirrors: `sessionFullHandler` ([tools/tools.go:144-213](tools/tools.go#L144)) — the consolidated handler is its extension

- **Deleted:** registrations and handlers for `session_full`, `session_latest`, `session_plan`, `session_diff`, `session_uncommitted_diff`; the old `sessionGetHandler` body.
- **Kept:** `errSessionSelectorMissing`, `DefaultReturnedTurns`, `withMaxResultSize`, `resolveSession`, `resolveAgentFilter`, `resolveAgentFromRequest`, `sessionListHandler` — all unchanged.
- New `Register` (complete final function):

```go
func Register(server *server.MCPServer, store *session.Store) {
	pageStore := &PageStore{
		PagesByRequestId: make(map[string]<-chan *sessionGetResult),
	}

	sessionGet := mcp.NewTool("session_get",
		mcp.WithDescription("Returns session data (turns, plan, git diff, uncommitted diff) for a session. Defaults to the most recently active session when id and title are omitted. Select sections with the turns/plan/diff/uncommitted_diff flags. Responses are paginated: if has_more is true, call again with the returned request_id to get the next page."),
		mcp.WithString("id",
			mcp.Description("Session ID (omit for most recent session)"),
		),
		mcp.WithString("title",
			mcp.Description("Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to agent when provided. For Codex, titles come from Codex's session index (thread name)."),
		),
		mcp.WithString("agent",
			mcp.Description("Agent: \"claude\" or \"codex\". Required when id and title are omitted and more than one agent is enabled."),
		),
		mcp.WithNumber("n",
			mcp.Description("Number of turns to return (default 20). Only applies to the turns section."),
		),
		mcp.WithBoolean("turns",
			mcp.Description("Return the session turns (default true)."),
		),
		mcp.WithBoolean("plan",
			mcp.Description("Return the session plan (default true)."),
		),
		mcp.WithBoolean("diff",
			mcp.Description("Return the pre-computed merge-base git diff against the inferred base branch, reported as diff_target (default true)."),
		),
		mcp.WithBoolean("uncommitted_diff",
			mcp.Description("Return the live uncommitted git diff (`git diff HEAD`) in the session's own working tree (default false)."),
		),
		mcp.WithString("request_id",
			mcp.Description("Pagination request ID from a previous response. Pass this to get the next page."),
		),
	)
	sessionGet.Meta = withMaxResultSize()
	server.AddTool(sessionGet, sessionGetHandler(store, pageStore))

	sessionList := mcp.NewTool("session_list",
		mcp.WithDescription("Lists all sessions. Returns session ID, agent, last activity timestamp, whether a plan or diff is available, the inferred diff base branch (diff_target), and session metadata (cwd, git branch, model, origin)."),
		mcp.WithString("agent",
			mcp.Description("Agent: \"claude\" or \"codex\". Lists all sessions when omitted."),
		),
	)
	sessionList.Meta = withMaxResultSize()
	server.AddTool(sessionList, sessionListHandler(store))
}
```

- New consolidated handler (complete final function, replaces `sessionFullHandler` and the old `sessionGetHandler`):

```go
func sessionGetHandler(s *session.Store, pageStore *PageStore) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		if reqId, ok := args["request_id"].(string); ok && reqId != "" {
			next, ok := pageStore.next(reqId)
			if !ok {
				return mcp.NewToolResultError(fmt.Sprintf("request_id %q not found or expired", reqId)), nil
			}

			if !pageStore.hasNext(reqId) {
				pageStore.remove(reqId)
				reqId = ""
			}

			result := &sessionGetResultPage{
				sessionGetResult: next,
				RequestId:        reqId,
				HasMore:          pageStore.hasNext(reqId),
			}
			return respond(ctx, result)
		}

		sess, err := resolveSession(s, request)
		if err != nil {
			if !errors.Is(err, errSessionSelectorMissing) {
				return mcp.NewToolResultError(err.Error()), nil
			}
			agent, agentErr := resolveAgentFromRequest(s, request)
			if agentErr != nil {
				return mcp.NewToolResultError(agentErr.Error()), nil
			}
			found, ok := s.Last(agent)
			if !ok {
				return mcp.NewToolResultText("no sessions found"), nil
			}
			sess = found
		}

		var turns, plan, diff, uncommitted string
		if boolArgFromRequest(request, "turns", true) {
			n := intArgFromRequest(request, "n")
			if n <= 0 {
				n = DefaultReturnedTurns
			}
			data, err := json.Marshal(sess.Turns(n))
			if err != nil {
				return nil, fmt.Errorf("marshaling turns: %w", err)
			}
			turns = string(data)
		}
		withDiff := boolArgFromRequest(request, "diff", true)
		if boolArgFromRequest(request, "plan", true) {
			plan = sess.PlanContent
		}
		if withDiff {
			diff = sess.DiffOutput
		}
		if boolArgFromRequest(request, "uncommitted_diff", false) {
			uncommitted = sess.UncommittedDiff
		}

		firstPage, nextPages := NewPageBuilder(maxResponseBytes(ctx)).build(turns, plan, diff, uncommitted)
		if withDiff {
			firstPage.DiffTarget = sess.DiffTarget
		}

		resultPage := newSessionGetResultPage(firstPage)
		if len(nextPages) == 0 {
			return respond(ctx, resultPage)
		}

		requestId := uuid.NewString()
		pageStore.add(requestId, nextPages)

		resultPage.WithRequestId(requestId)
		return respond(ctx, resultPage)
	}
}
```

- New bool-arg helper (complete final unit, in `tools/forms.go` next to `intArgFromRequest`):

```go
func boolArgFromRequest(request mcp.CallToolRequest, name string, fallback bool) bool {
	value, ok := request.GetArguments()[name].(bool)
	if !ok {
		return fallback
	}

	return value
}
```

### 5. README rewritten for the two-tool surface (modified)

location: [README.md](README.md)

- Line 20 (solution paragraph):

```diff
-peek-mcp watches the session files that Claude Code and Codex write to disk automatically, parses them passively, and serves the last N turns via MCP. Any connected client calls `session_latest` or `session_full` and quickly gets the context it needs.
+peek-mcp watches the session files that Claude Code and Codex write to disk automatically, parses them passively, and serves the last N turns via MCP. Any connected client calls `session_get` and quickly gets the context it needs.
```

- Line 31 (ASCII diagram): `session_full(n)` → `session_get(n)`.
- Lines 36–37 (plans/diffs bullets): `session_plan` and `session_full` → `session_get`; `session_diff` and `session_full` → `session_get` (diff section).
- "## MCP Tools" section (lines 41–95) replaced by:

```markdown
**`session_get`** Returns session data (turns, plan, git diff, uncommitted diff) for a session in one call. Defaults to the most recently active session when `id` and `title` are omitted. Responses are paginated: if `has_more` is true, call again with the returned `request_id` to get the next page.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | no | Session ID (omit for most recent session) |
| `title` | string | no | Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to `agent` when provided. For Codex, titles come from Codex's session index (thread name) |
| `agent` | string | no | Agent: `claude` or `codex`. Required when `id` and `title` are omitted and more than one agent is enabled |
| `n` | number | no | Number of turns to return (default 20). Only applies to the turns section |
| `turns` | boolean | no | Return the session turns (default `true`) |
| `plan` | boolean | no | Return the session plan (default `true`) |
| `diff` | boolean | no | Return the pre-computed merge-base diff against the inferred base branch, reported as `diff_target` (default `true`) |
| `uncommitted_diff` | boolean | no | Return the live `git diff HEAD` in the session's own working tree (default `false`) |
| `request_id` | string | no | Pagination request ID from a previous response |

**`session_list`** Lists all sessions. Returns session ID, agent, title, title source (`custom` | `index` | `derived`), last activity timestamp, whether a plan or diff is available, the inferred diff base (`diff_target`), and session metadata (cwd, git branch, model, origin).

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `agent` | string | no | Agent: `claude` or `codex`. Lists all sessions when omitted |
```

- Line 293 (example workflow): "Use session_full to review…" → "Use session_get to review…".
- Line 298 (limitations): `session_diff` → "the `diff` and `uncommitted_diff` sections require a local `git` binary…".

### 6. CONTRIBUTING tool roster (modified)

location: [CONTRIBUTING.md:23-24](CONTRIBUTING.md#L23), [CONTRIBUTING.md:73](CONTRIBUTING.md#L73)

```diff
-tools.Register           — exposes session_full / session_latest / session_get /
-                           session_list / session_plan / session_diff via mcp-go
+tools.Register           — exposes session_get / session_list via mcp-go
```

```diff
-... Open a Claude Code session in another terminal to generate traffic; `session_latest` will reflect it within seconds.
+... Open a Claude Code session in another terminal to generate traffic; `session_get` will reflect it within seconds.
```

### 7. Manifest tool roster (modified)

location: [mcpb/manifest.json:7,36-43](mcpb/manifest.json#L36)

- `long_description` value becomes:

```json
"peek-mcp watches local Claude Code (~/.claude/projects) and Codex CLI (~/.codex/sessions) JSONL session files and exposes session_get and session_list tools so a second model can review what a primary agent produced without re-summarisation."
```

- `tools` array becomes:

```json
"tools": [
  {
    "name": "session_get",
    "description": "Turns, plan, git diff, and uncommitted diff for a session in one call; defaults to the most recent session. Executes local git binary in the session's working directory."
  },
  {
    "name": "session_list",
    "description": "All known sessions with metadata."
  }
]
```

### 8. Peek skill routing (modified)

location: [skills/peek/SKILL.md](skills/peek/SKILL.md)

- Routing table becomes:

```markdown
| Input | Tool | Notes |
|-------|------|-------|
| `/peek [n]`, "what is Claude doing", "show session" | `session_get` | n defaults to 20 |
| `/peek list` | `session_list` | shows all sessions with plan/diff flags |
| `/peek plan` | `session_get` with `turns: false, diff: false` | current plan only |
| `/peek diff` | `session_get` with `turns: false, plan: false` | git diff only |
| `/peek <id>` or `/peek <id> [n]` | `session_get` with `id` param | specific session by ID |
| `/peek <title>` or `/peek <title> [n]` | `session_get` with `title` param | exact title match (case-insensitive) |
```

- The `session_latest` agent-param note becomes: `agent` is only needed when no `id`/`title` is provided and both agents are enabled; pass it when the user qualifies the command (e.g. `/peek codex`), default `"claude"`.
- Pagination paragraph: `session_full` → `session_get`; drop "Do NOT call session_diff or session_plan separately" in favor of "all requested sections arrive through the paginated session_get responses".

### 9. Handler tests (new)

location: `tools/tools_test.go`
mirrors: [session/store_test.go](session/store_test.go) — testify `assert`, package-local fixture builder in the style of `provideCompleteStore`

- Fixture: `provideToolStore()` builds a `session.Store` via `session.NewStore(10, events.NewBroker())` + `AddTurnBySessionId` (one Claude session "s1" with turns, one Codex "s2"), then sets `PlanContent`, `DiffOutput`, `DiffTarget`, `UncommittedDiff` on the fetched sessions directly.
- Requests built as `mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: map[string]any{…}}}`; results decoded from `result.Content[0].(mcp.TextContent).Text` via `json.Unmarshal` into a map (background `context.Background()` has no client session → text shaping, 100KB pages, [F7](#f7)).
- Representative test (full code; remaining cases mirror this skeleton):

```go
func TestSessionGet_Defaults(t *testing.T) {
	store := provideToolStore()
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"agent": "claude"}))

	assert.NoError(t, err)
	payload := decodeResult(t, result)
	assert.Contains(t, payload, "turns")
	assert.Equal(t, "# Plan", payload["plan"])
	assert.Equal(t, "diff-output", payload["diff"])
	assert.Equal(t, "main", payload["diff_target"])
	assert.NotContains(t, payload, "uncommitted_diff")
	assert.Equal(t, false, payload["has_more"])
}
```

### 10. PageBuilder tests (new)

location: `tools/pages_test.go`
mirrors: [session/store_test.go](session/store_test.go) test style

- Cases in the Tests table below; direct calls to `NewPageBuilder(size).build(...)`, asserting per-page section contents and drain order.

### 11. Smoke runbooks (new)

location: `plans/consolidate_session_tools/runbooks/`
mirrors: [plans/control_server/runbooks/turns_default.sh](plans/control_server/runbooks/turns_default.sh)

- Scripts drive the stdio transport directly (JSON-RPC lines piped into the binary), since the MCP surface — not the control HTTP API — is what changed. Full contents in [Test runbook](#test-runbook).

## Hot items

- **Channels/locking (class 2, context/general/hot-items.md):** `PageStore` (mutex + channel map) is touched only by the mechanical value-type rename in [§2](#2-pagestore-type-rename-modified); no goroutine, channel, or locking logic changes. The full `PageBuilder.build` change (pure function) is written out in [§3](#3-pagebuilder-gains-the-uncommitted-diff-stream-modified) for approval.
- No SQL, interfaces, generics, migrations, anonymous structs, or weakened guards. The removal of `session_get`'s id-required error is a planned behavior change ([D3](#d3)), not a guard removal.

## Tests

| Location.Method | Cases | Comment |
|---|---|---|
| tools_test.TestSessionGet_Defaults | no flags → turns+plan+diff+diff_target, no uncommitted_diff | full code in §9 |
| tools_test.TestSessionGet_PlanOnly | `turns: false, diff: false` → only plan key<br>no turns/diff/diff_target keys | |
| tools_test.TestSessionGet_UncommittedDiff | all three defaults false, `uncommitted_diff: true` → only uncommitted_diff | |
| tools_test.TestSessionGet_ById | known id → that session's data<br>unknown id → IsError "not found" | |
| tools_test.TestSessionGet_LatestFallback | no id/title, `agent: "claude"` → most recent Claude session | [D3](#d3) |
| tools_test.TestSessionGet_AgentRequired | two agents enabled, no selector, no agent → IsError "agent parameter is required" | [F4](#f4) |
| tools_test.TestSessionGet_NoSessions | empty store with agent → text "no sessions found" | |
| tools_test.TestSessionGet_TurnCount | `n: 1` → one turn in turns JSON<br>n absent → default 20 cap | |
| tools_test.TestSessionGet_Pagination | >100KB diff → has_more true + request_id<br>continuation call returns next page<br>unknown request_id → IsError | uses `strings.Repeat` fixture |
| pages_test.TestPageBuilder_Build_SinglePage | all four sections fit → one page, all populated, next empty | |
| pages_test.TestPageBuilder_Build_DrainOrder | size forces 3 pages → turns drain before plan before diff before uncommitted_diff<br>page boundaries UTF-8 safe | |

- Integration: none needed — no infrastructure beyond the in-memory store; the live path is covered by the runbooks.
- Not tested: `respond`/client-shaping branches (unchanged code); `session_list` (unchanged); title-matching semantics (covered by `session/store_test.go:TestGetByTitle`).

## Test runbook

Env: `PEEK_MCP_BIN` (default `./dist/peek-mcp`), built via `make build-local`. Scripts speak JSON-RPC over the stdio transport; clientInfo name "runbook" routes to structured content (no pagination, [F7](#f7)).

- location: `plans/consolidate_session_tools/runbooks/tool_surface.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
BIN="${PEEK_MCP_BIN:-./dist/peek-mcp}"
{
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"runbook","version":"0.0.1"}}}'
  echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  echo '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
} | "$BIN" start --transport=stdio 2>/dev/null \
  | jq -c 'select(.id == 2) | [.result.tools[].name] | sort'
echo 'expect: ["session_get","session_list"]'
```

- location: `plans/consolidate_session_tools/runbooks/session_get_default.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
BIN="${PEEK_MCP_BIN:-./dist/peek-mcp}"
{
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"runbook","version":"0.0.1"}}}'
  echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  sleep "${PEEK_MCP_SCAN_WAIT:-3}"
  echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"session_get","arguments":{"agent":"claude"}}}'
} | "$BIN" start --transport=stdio 2>/dev/null \
  | jq -c 'select(.id == 2) | .result.structuredContent // .result.content[0].text | if type == "object" then {sections: (keys - ["has_more","request_id"]), has_more} else . end'
echo 'expect: sections ⊆ ["diff","diff_target","plan","turns"], never uncommitted_diff'
```

- location: `plans/consolidate_session_tools/runbooks/session_get_plan_only.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
BIN="${PEEK_MCP_BIN:-./dist/peek-mcp}"
{
  echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"runbook","version":"0.0.1"}}}'
  echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  sleep "${PEEK_MCP_SCAN_WAIT:-3}"
  echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"session_get","arguments":{"agent":"claude","turns":false,"diff":false}}}'
} | "$BIN" start --transport=stdio 2>/dev/null \
  | jq -c 'select(.id == 2) | .result.structuredContent // .result.content[0].text | if type == "object" then keys - ["has_more"] else . end'
echo 'expect: ["plan"] (or [] when the latest session has no plan)'
```

Run:

```bash
make build-local && for f in plans/consolidate_session_tools/runbooks/*.sh; do bash "$f"; done
```

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| Tool names `session_full`, `session_latest`, `session_plan`, `session_diff`, `session_uncommitted_diff` (removed) | Go server<br>README<br>CONTRIBUTING<br>mcpb/manifest.json<br>skills/peek/SKILL.md | repo-wide grep for each name must hit only `plans/*/` history and this plan; out-of-repo consumers (user-global peek skill, other repos' MCP allowlists) listed as deferred finding |
| `session_get` input schema (section flags added, latest-fallback added) | Go server<br>README param table<br>skills/peek routing<br>manifest description | grep `session_get` across README/CONTRIBUTING/manifest/skills after edit — every hit reflects the new schema |
| Result JSON keys (`uncommitted_diff` added; `turns`/`plan`/`diff`/`diff_target`/`request_id`/`has_more` unchanged) | Go viewmodels<br>skills/peek pagination notes<br>README | grep `has_more`, `request_id`, `diff_target` — docs consistent |

## Verification

- [ ] Run `make test` — all packages pass, including new `tools` tests.
- [ ] Run `make build-local` — binary builds.
- [ ] Run `bash plans/consolidate_session_tools/runbooks/tool_surface.sh` — exactly `["session_get","session_list"]`.
- [ ] Run `bash plans/consolidate_session_tools/runbooks/session_get_default.sh` against a live Claude session — turns/plan/diff sections present, no `uncommitted_diff`.
- [ ] Run `bash plans/consolidate_session_tools/runbooks/session_get_plan_only.sh` — only `plan` returned.
- [ ] Run `grep -rn "session_full\|session_latest\|session_plan\|session_diff\|session_uncommitted_diff" --exclude-dir=plans .` — zero hits outside historical plans.
- [ ] Degenerate cases via tests: empty store, unknown id, missing agent with two agents enabled — each returns its planned error/text.

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | Stop and report. Never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | Stop, research the actual cause, redesign. No third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | Run the producing step. If infrastructure is down, ask. Never skip validation, never start infrastructure yourself |
| S4 | Discovered work materially exceeds the approved scope | Ask before continuing |
| S5 | Same kind of bug found a second time: in own diff → fix all in diff; pre-existing outside diff | Report and ask before sweeping |
| S6 | A structural obstacle tempts a new abstraction (interface, DTO, wrapper) | Stop and report. The fix is relocating the component, not indirection |
| S8 | The stdio runbook handshake fails structurally (transport rejects the initialize sequence) | Stop and report the observed frames; don't reshape the runbook convention silently |

## Open questions

None — all decisions resolved above.

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
| 2026-08-11 | refine: driver 1 | D4/D5 revised — `include` array replaced by flat boolean params turns/plan/diff/uncommitted_diff; §4/§5/§8/§9/§11, Tests, Stop conditions updated |
