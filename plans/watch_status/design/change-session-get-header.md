# session_get watch header — Change Plan

route: `change` — target: `frontier`

## TLDR

- Extend the cheap `session_get` call (every section flag off) so a monitoring loop can see subagent work without calling `session_events`.
- Each `subagents` entry gains `model`, `last_active` and, for Claude sessions, `usage` (the subagent's own token total). Peek already tracks all three.
- The first page gains `last_active` (main chain) and `plan_revisions` (`count` + `timestamps`, the same shape `session_events` returns).
- Why: `total_usage` counts only main-chain tokens, so a watch that polls it reads a 10-minute subagent run as idle. Today the only way to get per-subagent tokens is `session_events` with `breakdown`, which always carries the full event stream, every permission request and every touched file.
- Result: one call of about 200 B plus about 280 B per subagent gives the watch tick everything it needs from peek. A release (v1.2.11) ships it. The watch skill rewrite and heartbeat liveness stay in the claude-configs watch session.

## Context

- **Problem:** `session_get`'s `subagents` entries carry only the id, type and description ([viewmodels.go:39](tools/viewmodels.go:39)). Per-subagent usage exists only in `session_events` with `breakdown` ([tools.go:455](tools/tools.go:455)).
- **Cause:** subagent tokens go only to `SubagentStat.Usage` and never reach `Session.TotalUsage` ([session.go:287](session/session.go:287) vs [session.go:333](session/session.go:333)).
- **Design:** the watch session's proposal, part 1 ("in the `session_get` response, extend each `subagents` entry with the `model`, `last_active` and `usage` it already tracks, and add a `plan_revisions` count and `last_active` at the top"). The user confirmed it: "lets align peek so this is lightweight".
- **Constraints:** additive only. Existing fields, pagination and section flags stay unchanged. Codex parity stays honest: no zero `usage` that would read as "no work".

## Drivers

| ID | Origin | Observed | Wanted | Impact |
| :-- | :-- | :-- | :-- | :-- |
| <a id="r1"></a>R1 | "extend each `subagents` entry with the `model`, `last_active` and `usage` it already tracks" | `subagentRef` = `agent_id`, `agent_type`, `description` | + `model`, `last_active`, `usage` | contract-touching |
| <a id="r2"></a>R2 | "add a `plan_revisions` count and `last_active` at the top" | only `session_events` carries them | `session_get` first page carries them | contract-touching |
| <a id="r3"></a>R3 | "lets align peek so this is lightweight" | per-subagent tokens need `session_events` + `breakdown` (events, up to 2000 permission requests, touched files) | the all-sections-off `session_get` is enough | behavioral |
| <a id="r4"></a>R4 | "That needs a peek-mcp release" | latest tag v1.2.10 | v1.2.11 carrying R1–R3 | behavior-preserving |

## Scope

- **In:**
  - **subagent-ref:** `model`, `last_active` and `usage` (Claude only) on every `session_get` `subagents` entry
  - **header:** top-level `last_active` and `plan_revisions` on the `session_get` first page and in the `json` result
  - **lock:** build the whole header (`total_usage`, `subagents`, `plan_revisions`, `last_active`, `turns_total`) inside the existing `WithSession` read lock
  - **docs:** the `session_get` paragraph in [docs/tools.md](docs/tools.md)
  - **release:** v1.2.11 via `make git-release`, run by the user
- **Out:**
  - **watch-skill:** the tick rewrite (heartbeat probe, idle rule, alerts) belongs to the claude-configs session `watch-skill-second-attempt-bfcb3a`
  - **version-pin:** the `PEEK_MCP_VERSION` bump to 1.2.11 in claude-configs belongs to the same session
  - **session_events:** no change. `breakdown` keeps its fuller per-subagent view (`started_at`, `seconds`, `touched_files`)
- **Not changed:**
  - **total_usage semantics:** still main chain only. The watch sums subagent `usage` itself
  - **pagination:** the header rides the first page, as `total_usage` and `turns_total` do today
- **Deferred findings:**
  - **session_get unlocked reads:** `sess.SubagentIds()`, `sess.Events.All()`, `sess.PlanContent`, `sess.UncommittedDiff` are read outside the store lock ([tools.go:244](tools/tools.go:244), [tools.go:247](tools/tools.go:247), [tools.go:266](tools/tools.go:266))
  - **session_events unlocked reads:** the handler reads `currentSession` fields outside the lock throughout ([tools.go:415](tools/tools.go:415)–[tools.go:471](tools/tools.go:471))
  - **"always present" claim:** docs say `subagents` is always present, but `omitempty` drops an empty list ([viewmodels.go:15](tools/viewmodels.go:15))

## Assumptions

| Assumption | Reality | Location |
| :-- | :-- | :-- |
| `PEEK_MCP_VERSION` pins 1.2.8 while the repo is at 1.2.10 | claude-configs `main` pins **1.2.10**. The missing `turns_total` points to an installed or running binary older than the pin, not to the pin itself | claude-configs `PEEK_MCP_VERSION` |
| `total_usage` counts only the main session's tokens | confirmed: `AddTurn` targets `s.TotalUsage`, `AddSubagentTurn` targets `stat.Usage` only | [session.go:333](session/session.go:333), [session.go:287](session/session.go:287) |
| peek already tracks model, last_active and usage per subagent | confirmed: `SubagentStat` fields `Model`, `LastActive`, `Usage` | [session.go:229](session/session.go:229) |
| a top-level `last_active` reflects subagent work | **false**: `Session.LastActive` moves only on main-chain turns. Subagent turns move only `stat.LastActive`, so the watch must take the max itself | [session.go:329](session/session.go:329), [session.go:270](session/session.go:270) |
| about 150 B per subagent | about 190 B added per Claude subagent (`last_active` about 45, `model` about 25, `usage` about 120). A full entry is about 280 B | estimate from the `Usage` JSON tags, [usage.go:5](session/usage.go:5) |
| per-subagent usage works for Codex | **false**: Codex subagent turns carry no `RequestId`, so `stat.Usage` stays zero | [session.go:284](session/session.go:284), [tools.go:537](tools/tools.go:537) |

## Current state

N/A — additive fields on one view model plus one read moved under an existing lock; no structural change.

## Target state

N/A — see Current state.

## Behavior contract

- **Must not change:**
  - every existing `session_get` key, its value and its presence rules
  - pagination and chunking of `turns`, `events`, `plan`, `diff`, `uncommitted_diff`, `memory`
  - `subagent` scoping and the unknown-subagent error
  - `session_events` output, byte for byte
- **Intentional changes:**
  - new keys `subagents[].model`, `subagents[].last_active`, `subagents[].usage` ([R1](#r1))
  - new top-level keys `last_active`, `plan_revisions` ([R2](#r2))
  - header values are read under the store's read lock, so they come from one consistent snapshot. The output shape does not change

## Decisions

| ID | Problem | Facts | Decision | Rejected | Why | Consequences |
| :-- | :-- | :-- | :-- | :-- | :-- | :-- |
| <a id="d1"></a>D1 | The `session_get` subagent entry needs per-subagent activity and tokens. Two view types exist: the light `subagentRef` and the full `SubagentStatView` from `breakdown`. The full view carries a `touched_files` list per subagent, which is unbounded | [F1](#f1), [F2](#f2) | extend `subagentRef` with `last_active`, `model`, `usage` | reuse `SubagentStatView`: `touched_files` is unbounded, which defeats "lightweight"<br>a new opt-in flag: a second concept for data the watch always needs | reliable: size stays bounded per subagent | two subagent views remain, one light and one full. They are split by purpose, not duplicated state: both project the same `SubagentStat` |
| <a id="d2"></a>D2 | Codex subagents never accumulate usage. A zero `usage` would read to a watch as "no tokens moved" and trigger a false idle | [F3](#f3) | `usage` only on Claude sessions, omitted for Codex | always emit `usage`: misleading zeros<br>omit when zero: a Claude subagent before its first response would lose the key, so presence would carry no meaning | debuggable: a missing key means "not supported", as the `unsupportedSignals` `subagent_usage` entry already says | Codex watch targets rely on `last_active` only |
| <a id="d3"></a>D3 | The proposal asks for a `plan_revisions` count. `session_events` already publishes `plan_revisions` as an object `{count, timestamps}`. Giving the same key a bare int in `session_get` would give one key two types across the tools | [F4](#f4) | reuse `planRevisionsView` via `newPlanRevisionsView` under the same key | bare int under `plan_revisions`: type clash between the tools<br>new key `plan_revision_count`: a second name for the same fact | reliable: one key, one shape, one producer. Size is bounded by `maxPlanRevisions` = 50 timestamps (about 1.6 KB at worst, usually a few) | **deviation from the proposal's "count":** a superset. The watch reads `.plan_revisions.count`. Absent means the session has no plan |
| <a id="d4"></a>D4 | Top-level `last_active` could mean main chain only (`Session.LastActive`) or the latest activity anywhere | [F5](#f5) | `Session.LastActive`: main chain, the same value `session_list` and `session_events.time` report | max over subagents: the same key would mean something different in `session_get` than in the other two tools | reliable: one meaning per key across tools | the watch computes `max(last_active, subagents[].last_active)`. The docs state it |
| <a id="d5"></a>D5 | The new fields read `SubagentStat.Usage` and `LastActive` while ingestion mutates them under the store lock. The handler already runs one `WithSession` read-locked closure for the turns | [F6](#f6) | build the whole header (`total_usage`, `subagents`, `plan_revisions`, `last_active`, `turns_total`) inside that closure | read outside the lock as today: a torn `Usage` struct under concurrent ingestion<br>a new accessor on `Store`: a new concept for one call site | reliable: one consistent snapshot per response | `total_usage` and `subagents` move into the lock too. The other unlocked reads stay a deferred finding |
| <a id="d6"></a>D6 | Should a `subagent`-scoped call keep the session-level header? | [F7](#f7) | yes: `plan_revisions` and `last_active` stay, as `total_usage` and `subagents` do today | drop them when scoped: breaks the existing rule that header fields are session-level | controllable: one presence rule for every header key | none |

## Open questions

None.

## Baseline (verified)

Base branch: `main` at `75f0e55` (release v1.2.10); the worktree is clean.

| ID | Fact | Needed for | Location |
| :-- | :-- | :-- | :-- |
| <a id="f1"></a>F1! | `SubagentStat` already holds `Model`, `LastActive`, `Usage` per subagent | [D1](#d1), [subagent-ref](#subagent-ref-and-header-fields-modified) | [session.go:229](session/session.go:229) |
| <a id="f2"></a>F2! | `SubagentStatView` carries `TouchedFiles` per subagent, which is unbounded | [D1](#d1) | [viewmodels_events.go:312](tools/viewmodels_events.go:312) |
| <a id="f3"></a>F3! | Codex subagent usage never accumulates: no `RequestId` means an early return. `unsupportedSignals` lists `subagent_usage` for Codex | [D2](#d2) | [session.go:284](session/session.go:284), [tools.go:537](tools/tools.go:537) |
| <a id="f4"></a>F4! | `session_events` emits `plan_revisions` as `planRevisionsView{count, timestamps}`, which is nil when there are no revisions. Revisions are capped at 50 | [D3](#d3) | [tools.go:523](tools/tools.go:523), [viewmodels_events.go:23](tools/viewmodels_events.go:23), [store.go:23](session/store.go:23) |
| <a id="f5"></a>F5! | `Session.LastActive` moves on main-chain turns only. `session_list` and `session_events.time` report it | [D4](#d4) | [session.go:329](session/session.go:329), [tools.go:360](tools/tools.go:360) |
| <a id="f6"></a>F6! | `WithSession` holds `s.mu.RLock` for the closure. Ingestion (`addSubagentTurn`) writes under `s.mu.Lock` | [D5](#d5), [header](#session-get-handler-header-modified) | [store.go:487](session/store.go:487), [store.go:167](session/store.go:167) |
| <a id="f7"></a>F7 | `total_usage` and `subagents` are set on every response, scoped or not | [D6](#d6) | [tools.go:254](tools/tools.go:254), [tools.go:317](tools/tools.go:317) |
| <a id="f8"></a>F8 | the repo targets Go 1.26 and already uses `omitzero` on view models | [subagent-ref](#subagent-ref-and-header-fields-modified) | go.mod, [viewmodels.go:68](tools/viewmodels.go:68) |

## Exemplar & reuse

| Existing | Used for |
| :-- | :-- |
| `newPlanRevisionsView` ([tools.go:523](tools/tools.go:523)) | the top-level `plan_revisions` |
| `planRevisionsView` ([viewmodels_events.go:23](tools/viewmodels_events.go:23)) | the shape shared with `session_events` |
| `Session.CurrentUsage` ([session.go:385](session/session.go:385)) | copy-by-value usage, the same pattern used for the subagent `usage` |
| `provideSubagentStore` ([tools_test.go:276](tools/tools_test.go:276)) | base fixture for the new test fixture |

- Every change has an exemplar.

## Permissions

N/A — `go test`, `make test` and `gofmt` are covered by the deployed layers. The release (`make git-release VERSION=…`, push, tag push, asset upload) is user-run.

## Changes

| File | Kind | Entry |
| :-- | :-- | :-- |
| `tools/viewmodels.go` | modified | [Subagent ref and header fields](#subagent-ref-and-header-fields-modified) |
| `tools/tools.go` | modified | [session_get handler header](#session-get-handler-header-modified) |
| `tools/tools_test.go` | modified | [Header test](#header-test-modified) |
| `docs/tools.md` | modified | [Tool docs](#tool-docs-modified) |
| `plans/watch_status/design/change-session-get-header.md` | created | this plan, persisted on approval |
| `Makefile`, `cmd/version.go`, `mcpb/manifest.json` | modified | [Release](#phase-2--release-user-run), by `make git-release` |

### Phase 1 — Header fields (shippable alone)

#### Subagent ref and header fields (modified)

location: `tools/viewmodels.go`
mirrors: `SubagentStatView` field set ([viewmodels_events.go:312](tools/viewmodels_events.go:312)), `Session.CurrentUsage` copy pattern

```diff
 type sessionGetResult struct {
-	Diff            string         `json:"diff,omitempty"`
-	DiffTarget      string         `json:"diff_target,omitempty"`
-	Events          any            `json:"events,omitempty"`
-	Memory          any            `json:"memory,omitempty"`
-	Plan            string         `json:"plan,omitempty"`
-	Subagents       []subagentRef  `json:"subagents,omitempty"`
-	TotalUsage      *session.Usage `json:"total_usage,omitempty"`
-	Turns           any            `json:"turns,omitempty"`
-	TurnsTotal      *int           `json:"turns_total,omitempty"`
-	UncommittedDiff string         `json:"uncommitted_diff,omitempty"`
+	Diff            string             `json:"diff,omitempty"`
+	DiffTarget      string             `json:"diff_target,omitempty"`
+	Events          any                `json:"events,omitempty"`
+	LastActive      time.Time          `json:"last_active,omitzero"`
+	Memory          any                `json:"memory,omitempty"`
+	Plan            string             `json:"plan,omitempty"`
+	PlanRevisions   *planRevisionsView `json:"plan_revisions,omitempty"`
+	Subagents       []subagentRef      `json:"subagents,omitempty"`
+	TotalUsage      *session.Usage     `json:"total_usage,omitempty"`
+	Turns           any                `json:"turns,omitempty"`
+	TurnsTotal      *int               `json:"turns_total,omitempty"`
+	UncommittedDiff string             `json:"uncommitted_diff,omitempty"`
 }
```

```diff
 type subagentRef struct {
-	AgentId     string `json:"agent_id"`
-	AgentType   string `json:"agent_type,omitempty"`
-	Description string `json:"description,omitempty"`
+	AgentId     string         `json:"agent_id"`
+	AgentType   string         `json:"agent_type,omitempty"`
+	Description string         `json:"description,omitempty"`
+	LastActive  time.Time      `json:"last_active,omitzero"`
+	Model       string         `json:"model,omitempty"`
+	Usage       *session.Usage `json:"usage,omitempty"`
 }
 
 func newSubagentRefs(sess *session.Session) []subagentRef {
 	refs := make([]subagentRef, 0, len(sess.Subagents))
 	for _, id := range sess.SubagentIds() {
 		stat := sess.Subagents[id]
-		refs = append(refs, subagentRef{AgentId: id, AgentType: stat.AgentType, Description: stat.Description})
+		ref := subagentRef{AgentId: id, AgentType: stat.AgentType, Description: stat.Description, LastActive: stat.LastActive, Model: stat.Model}
+		if sess.Agent == session.AgentClaude {
+			usage := stat.Usage
+			ref.Usage = &usage
+		}
+		refs = append(refs, ref)
 	}
 	return refs
 }
```

- **usage copy:** `usage := stat.Usage` copies the value, so the response never aliases the live struct ([D5](#d5)).
- **Claude gate:** implements [D2](#d2).

#### session_get handler header (modified)

location: `tools/tools.go`
mirrors: the existing `turnsTotal` capture inside the same closure ([tools.go:233](tools/tools.go:233))

```diff
 func sessionGetHandler(s *session.Store, pageStore *PageStore[*sessionGetResult]) server.ToolHandlerFunc {
 	// ...
 		var scopedTurns []*turnView
-		var turnsTotal int
+		var header *sessionGetResult
 		isKnownSubagent := true
 		isFound := s.WithSession(sess.Meta.SessionId, func(lockedSession *session.Session) {
 			turns, ok := scopeTurns(lockedSession, n, subagentId, withTools)
 			isKnownSubagent = ok
 			scopedTurns = newTurnViews(turns, withThinking, withTools)
-			turnsTotal = scopeTurnsTotal(lockedSession, subagentId, withTools)
+			turnsTotal := scopeTurnsTotal(lockedSession, subagentId, withTools)
+			header = &sessionGetResult{
+				LastActive:    lockedSession.LastActive,
+				PlanRevisions: newPlanRevisionsView(lockedSession),
+				Subagents:     newSubagentRefs(lockedSession),
+				TotalUsage:    lockedSession.CurrentUsage(),
+				TurnsTotal:    &turnsTotal,
+			}
 		})
 	// ...
 		if boolArgFromRequest(request, "json", false) {
-			result := &sessionGetResult{Subagents: newSubagentRefs(sess), TotalUsage: sess.CurrentUsage(), TurnsTotal: &turnsTotal}
+			result := header
 			if withTurns {
 	// ...
 		if withDiff {
 			firstPage.DiffTarget = sess.DiffTarget
 		}
-		firstPage.TotalUsage = sess.CurrentUsage()
-		firstPage.Subagents = newSubagentRefs(sess)
-		firstPage.TurnsTotal = &turnsTotal
+		firstPage.LastActive = header.LastActive
+		firstPage.PlanRevisions = header.PlanRevisions
+		firstPage.Subagents = header.Subagents
+		firstPage.TotalUsage = header.TotalUsage
+		firstPage.TurnsTotal = header.TurnsTotal
 
 		resultPage := newSessionGetResultPage(firstPage)
```

- **nil safety:** `header` is set whenever `isFound` is true, and the handler returns early on `!isFound` ([tools.go:239](tools/tools.go:239)).
- **scoping:** the header is session-level in both paths ([D6](#d6)).

#### Header test (modified)

location: `tools/tools_test.go`
mirrors: `TestSessionGet_TurnsTotal` ([tools_test.go:468](tools/tools_test.go:468)) — sequential comment-labeled cases. `provideSubagentStore` ([tools_test.go:276](tools/tools_test.go:276)) — fixture shape

```go
func provideSubagentUsageStore() *session.Store {
	s := provideSubagentStore()
	now := time.Now()

	s.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		SubagentId: "ag1",
		Role:       session.RoleAssistant,
		Text:       "sub usage",
		RequestId:  "r-sub-usage",
		Timestamp:  now,
		Usage:      &session.Usage{InputTokens: 7, OutputTokens: 11},
		Meta:       &session.Meta{SessionId: "s1", Model: "claude-sonnet-5"},
	})
	s.AddTurnBySessionId("s2", session.AgentCodex, &session.Turn{
		SubagentId: "cx1",
		Role:       session.RoleAssistant,
		Text:       "codex sub answer",
		Timestamp:  now,
		Meta:       &session.Meta{SessionId: "s2", Model: "gpt-5.5"},
	})

	s1, _ := s.GetById("s1")
	s1.PlanRevisions = []*session.PlanRevision{
		{Index: 0, Timestamp: now.Add(-time.Minute)},
		{Index: 1, Timestamp: now},
	}
	return s
}

func TestSessionGet_WatchHeader(t *testing.T) {
	store := provideSubagentUsageStore()
	handler := sessionGetHandler(store, providePageStore())
	s1, _ := store.GetById("s1")

	// json-subagent-model-last-active-usage
	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "json": true, "turns": false, "events": false, "plan": false, "diff": false}))
	assert.NoError(t, err)
	payload, ok := result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	require.Len(t, payload.Subagents, 1)
	assert.Equal(t, "claude-sonnet-5", payload.Subagents[0].Model)
	assert.Equal(t, s1.Subagents["ag1"].LastActive, payload.Subagents[0].LastActive)
	require.NotNil(t, payload.Subagents[0].Usage)
	assert.Equal(t, 7, payload.Subagents[0].Usage.InputTokens)
	assert.Equal(t, 11, payload.Subagents[0].Usage.OutputTokens)

	// json-plan-revisions-and-last-active
	require.NotNil(t, payload.PlanRevisions)
	assert.Equal(t, 2, payload.PlanRevisions.Count)
	assert.Len(t, payload.PlanRevisions.Timestamps, 2)
	assert.Equal(t, s1.LastActive, payload.LastActive)

	// paginated-first-page-carries-header
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "turns": false, "events": false, "plan": false, "diff": false}))
	assert.NoError(t, err)
	pagePayload := decodeResult(t, result)
	subagents, ok := pagePayload["subagents"].([]any)
	require.True(t, ok)
	subagent := subagents[0].(map[string]any)
	assert.Equal(t, "claude-sonnet-5", subagent["model"])
	assert.Contains(t, subagent, "last_active")
	assert.Equal(t, float64(11), subagent["usage"].(map[string]any)["output_tokens"])
	assert.Equal(t, float64(2), pagePayload["plan_revisions"].(map[string]any)["count"])
	assert.Contains(t, pagePayload, "last_active")

	// subagent-scope-keeps-header
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "json": true, "subagent": "ag1"}))
	assert.NoError(t, err)
	payload, ok = result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	assert.NotNil(t, payload.PlanRevisions)
	assert.Len(t, payload.Subagents, 1)

	// codex-subagent-omits-usage-and-no-revisions
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s2", "json": true, "turns": false, "events": false, "plan": false, "diff": false}))
	assert.NoError(t, err)
	payload, ok = result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	require.Len(t, payload.Subagents, 1)
	assert.Equal(t, "gpt-5.5", payload.Subagents[0].Model)
	assert.Nil(t, payload.Subagents[0].Usage)
	assert.Nil(t, payload.PlanRevisions)
}
```

#### Tool docs (modified)

location: `docs/tools.md`

```diff
-The first page also carries `total_usage`, the running token total (including the in-flight turn), `subagents` — every spawned subagent's `agent_id`, `agent_type`, and `description`, always present so a follow-up call can scope to one of them — and `turns_total`, …
+The first page also carries `total_usage`, the main chain's running token total (including the in-flight turn; subagent tokens never enter it), `last_active`, the main chain's latest transcript timestamp, `plan_revisions` (`count` and `timestamps`, the same block `session_events` returns; absent when the session has no plan), `subagents` — every spawned subagent's `agent_id`, `agent_type`, `description`, `model`, `last_active` and, for Claude sessions, `usage` (its own token total), always present so a follow-up call can scope to one of them — and `turns_total`, …
+
+With every section flag off, `session_get` returns only these fields: the cheap call for a monitoring loop. Subagent work shows as movement in `subagents[].usage` and `subagents[].last_active` while `total_usage` and `last_active` stay flat; the latest activity anywhere is the max of `last_active` and every `subagents[].last_active`.
```

- The `…` stands for the unchanged `turns_total` sentence, kept verbatim.

### Phase 2 — Release (user-run)

location: `Makefile`, `cmd/version.go`, `mcpb/manifest.json`

- The user runs `make git-release VERSION=1.2.11` (bumps the three files, commits `cmd: release v1.2.11`, tags `v1.2.11`), then pushes and publishes the release asset the usual way.
- Hand the claude-configs `PEEK_MCP_VERSION` bump to 1.2.11 to the watch session.

## Hot items

- **Locking (class 2):** [D5](#d5) moves the header reads into the existing `WithSession` read lock. The example implementation is the handler diff in [session_get handler header](#session-get-handler-header-modified). No new lock, goroutine or channel. The closure calls `CurrentUsage`, `newSubagentRefs` and `newPlanRevisionsView`, and none of them locks the store, so there is no re-entrant `RLock`.

## Tests

| Location.Method | Cases | Comment |
| :-- | :-- | :-- |
| `tools_test.go.TestSessionGet_WatchHeader` | json-subagent-model-last-active-usage<br>json-plan-revisions-and-last-active<br>paginated-first-page-carries-header<br>subagent-scope-keeps-header<br>codex-subagent-omits-usage-and-no-revisions | new; covers [R1](#r1), [R2](#r2), [D2](#d2), [D3](#d3), [D6](#d6) |

- **Safety net, unchanged:** `TestSessionGet_TurnsTotal`, `TestSessionGet_Subagent`, `TestSessionGet_SubagentListWithoutParam`, `TestSessionGet_JsonTypedUnpaginated`, `TestSessionGet_Pagination` pin the existing header and paging. They must pass unedited.
- **session_events untouched:** `TestSessionEvents_*` and `TestNewSubagentStatViews` pin the `breakdown` view.
- **Not tested:** the lock placement under real concurrent ingestion, because no race test harness exists for handlers. `go test -race ./tools/...` runs the suite with the detector instead.

## Test runbook

- **session_get_watch_header** — stdio `tools/call` of `session_get` with `turns`, `events`, `plan`, `diff` false against the latest Claude session that has subagents. Mirrors [session_get_plan_only.sh](plans/consolidate_session_tools/runbooks/session_get_plan_only.sh), keys expected: `last_active`, `plan_revisions`, `subagents`, `total_usage`, `turns_total`. User-run, because a `bash <script>` run is denied by policy.
- **Behavior contract:** the existing runbooks `session_get_default.sh` and `session_get_plan_only.sh` re-verify the unchanged sections.

## Contracts & sweeps

| Contract | Sides | Sweep |
| :-- | :-- | :-- |
| `session_get` first-page keys (additive) | peek-mcp `tools/`<br>claude-configs watch skill (consumer, out of scope)<br>claude-configs peek skill (reads `session_get`, ignores unknown keys) | `grep -rn "subagents\|plan_revisions\|last_active" docs/ skills/ README.md` — update only [docs/tools.md](docs/tools.md) |
| `plan_revisions` shape shared with `session_events` | `newPlanRevisionsView`, the single producer | no second producer allowed: `grep -n "planRevisionsView{" tools/` → exactly one hit |

## Verification

- [ ] Run `go test -race ./tools/...` — expect PASS, including `TestSessionGet_WatchHeader` and every existing `TestSessionGet_*` unedited
- [ ] Run `make test` — expect PASS across all packages
- [ ] Run `go vet ./...` and `gofmt -l tools` — expect no output
- [ ] Run `git diff --stat main` — expect only the four Phase 1 files plus the persisted plan
- [ ] After the user rebuilds or installs and restarts peek: call `mcp__peek-mcp__session_get` with `turns`, `events`, `plan`, `diff` false on a live Claude session that has spawned subagents — expect `subagents[].model`, `subagents[].last_active`, `subagents[].usage`, plus top-level `last_active`, `plan_revisions`, `turns_total`
- [ ] Same call while one of that session's subagents is running, twice a minute apart — expect that subagent's `usage` and `last_active` to move while `total_usage` stays flat
- [ ] Same call on a Codex session with subagents — expect `model` and `last_active`, no `usage`
- [ ] Same call on a session without a plan — expect no `plan_revisions` key
- [ ] Measure the response size with every flag off on the largest live subagent session — expect about 200 B plus about 280 B per subagent (Stop S8 fires above 400 B per subagent)

## Stop conditions

| ID | Condition | Action |
| :-- | :-- | :-- |
| S1 | An approved signature or contract can't hold as planned | stop and report; never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | stop, research the actual cause, redesign; no third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | run the producing step; if infrastructure is down, ask; never skip validation or start infrastructure yourself |
| S4 | Discovered work materially exceeds the approved scope | ask before continuing |
| S5 | Same kind of bug found a second time | inside the diff: fix every instance now; pre-existing, outside the diff: report and ask before searching further |
| S6 | A structural obstacle tempts a new abstraction | stop and report; relocate instead of adding indirection |
| S7 | An existing `TestSessionGet_*` or `TestSessionEvents_*` needs editing to pass | stop — the Behavior contract is broken |
| S8 | A live per-subagent entry exceeds 400 B | stop and report before release; the lightweight premise fails |
| S9 | Codex subagents turn out to carry non-zero usage | stop and report; [D2](#d2) rests on [F3](#f3) |

## Changelog

| Date | Trigger | What changed |
| :-- | :-- | :-- |
| — | initial | plan created |
| 2026-09-28 | local: verification path | ran the live checks through `make serve-http` on the free port 4244 instead of a second instance with `--state-dir ""` (a direct binary run and `go run` are denied); the Codex live check was replaced by a parser check (`RequestId` is never set under `codex/`), because no Codex session inside the watch window has subagents |
