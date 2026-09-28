# Telemetry permission accounting — Implementation Plan

target: `frontier`

## TLDR

- peek's telemetry `permissions` block misreports denials: the eval run `0ba783f3` showed 11 while Claude Code's own result envelopes recorded 501.
- Three verified defects cause it:
  - a peek restart overwrites a session's persisted stats with a fresh zero;
  - the request list keeps only the first 200 decisions and fills with routine hook allows;
  - counters split by source only, so config-level denials (safety checks, working-dir fence, dontAsk mode) count as `auto_allowed` and hook denials hide inside `hook_decided`.
- Fix: count by decision (a `denied` total over every source plus per-source allow/deny counters), list only denials and prompted decisions with the list cap raised from 200 to 1000, and restore a session's persisted stats before folding new decisions.
- The transcript-side `counters.permission_denials` keeps its meaning (prompt rejections) and gets it documented.
- Result: `session_events.permissions.denied` equals Claude Code's own denial count, survives peek restarts, and ships in the pending v1.2.9 release.

## Context

- **Problem:** the claude-configs eval report printed "Permission denials (11)" for run `0ba783f3`; ground truth is 501 in the stream result envelopes and 440 hook denials in the transcripts.
- **Cause:** [telemetry/store.go:118-151](telemetry/store.go) folds by source and caps the list at the first 200 entries; [telemetry/store.go:90-101](telemetry/store.go) starts an unknown session at zero and [telemetry/store.go:178-193](telemetry/store.go) persists that over the file.
- **Design:** the fix shape agreed in-session after the evidence walk (restart wipe, cap, config-reject misclassification — all confirmed against live data and the Claude Code 2.1.282 binary).
- **Constraints:** peek only — the tool-policy gate's own ledger is out of scope (user: "fine as is"); the consumer in claude-configs is another repo.

## Drivers

N/A — new route

## Scope

- **In:**
  - **counting:** `denied` total over every source; `auto_allowed`/`config_denied` and `hook_allowed`/`hook_denied` split by decision.
  - **request list:** denials and prompted decisions only; config and hook allows are counted, not listed; cap raised from 200 to 1000 entries per session.
  - **restart continuity:** a session unknown to the in-memory store is seeded from its persisted `telemetry.json` before folding.
  - **view:** `permissionsView` serves the new counters.
  - **docs:** `docs/reference.md` telemetry paragraph and `docs/tools.md` `session_events` description, including the transcript `permission_denials` meaning.
  - **tests:** telemetry store, OTLP ingest, permissions view.
- **Out:**
  - **tool-policy ledger:** the gate's `denials.jsonl`, allows and APPLIED records stay as they are.
  - **transcript denial parsing:** no text matching of hook or native denial messages.
  - **claude-configs consumer:** `internal/peek/events.go` switching to `denied` is that repo's change.
  - **version bump:** rides the pending `cmd: release v1.2.9` commit.
- **Not changed:**
  - **OTLP ingest wiring:** `control/otlp.go`, event parsing in `telemetry/otlp.go` beyond one constant.
  - **metrics folding:** active seconds and cost semantics.
- **Deferred findings:**
  - **consumer counts samples:** claude-configs `internal/peek/events.go:150` `rejections()` counts `requests` entries decided `reject` — must read `denied` after this ships.
  - **legacy persisted files:** stats written before this change keep `auto_allowed` inflated by config denials and lose `hook_decided`; they cannot be re-split.

## Assumptions

| Assumption | Reality | Location |
| :--- | :--- | :--- |
| `docs/reference.md`: folded stats "survive peek restarts" | overwritten by the first decision after a restart: 7 of this session's 10 gate denials (21:49–22:16Z) are gone after the 22:23:46Z restart, the 3 later ones present | [docs/reference.md:82](docs/reference.md); ~/.peek/state/claude/41f8fb14-…/telemetry.json |
| `auto_allowed` = auto-allowed calls | also holds every `source: config` denial — Claude Code maps safetyCheck, workingDir, mode, classifier, sandboxOverride, asyncAgent to `config` | [telemetry/store.go:125-126](telemetry/store.go); claude 2.1.282 `B3n` |
| `requests` lists "each prompted/rejected request" | first 200 non-config decisions of any kind; run `0ba783f3`: 148 hook accepts, 41 prompted accepts, 11 hook rejects, window 00:58:45–01:24:19Z of a 3h run | [telemetry/store.go:139](telemetry/store.go); ~/.peek/state/claude/0ba783f3-…/telemetry.json |

## Current state

N/A — new route

## Target state

N/A — new route

## Behavior contract

N/A — new route

## Decisions

| ID | Problem | Facts | Decision | Rejected | Why | Consequences |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| <a id="d1"></a>D1 | Counters are keyed by `source` only, so a config-level denial increments `auto_allowed` and a hook denial is indistinguishable from a hook allow inside `hook_decided`; no field holds the number of denied calls | [F1!](#f1), [F2!](#f2) | `denied` counts every `decision == "reject"` regardless of source<br>`config` splits into `auto_allowed` (accept) and `config_denied` (reject)<br>`hook_decided` is replaced by `hook_allowed` and `hook_denied`<br>`prompted_once`, `prompted_always`, `rejected`, `aborted` keep their meaning | keep `hook_decided` and add `hook_denied`: two overlapping counters, the mixed one stays misleading<br>derive the total in the view: a new Claude Code source would silently drop out of it | **reliable:** the total is keyed on the decision field, so any source — present or future — lands in it<br>**debuggable:** every counter now means one thing | the `hook_decided` JSON key disappears (no consumer reads it — [F6](#f6))<br>`auto_allowed` shrinks by the config denials |
| <a id="d2"></a>D2 | The 200-entry list fills with routine allows, so later denials never appear and a consumer counting it undercounts | [F3!](#f3), [F6](#f6), [F9](#f9) | list only decisions with `decision == "reject"` or a prompted source (`user_temporary`, `user_permanent`)<br>`maxPermissionRequests` 200 → **1000** [USER]; first-kept order stays<br>truncation is readable as `len(requests) < denied + prompted_once + prompted_always` | keep newest in a ring: still a sample, breaks the `pendingCommands` index enrichment relies on<br>a `requests_dropped` field: duplicates what the complete counters already say<br>2000 or unbounded: doubles the per-decision file rewrite for no observed run | **controllable:** the list shows what an operator acts on (denials, prompts); counts come from counters (D1)<br>1000 holds the largest real run (501 denials) twice over | a full list costs ~380 KB in memory and per `telemetry.json` rewrite (the file is rewritten on every decision)<br>a run with more than 1000 listed decisions lists the first 1000; the totals stay exact |
| <a id="d3"></a>D3 | A restart (or an eviction at 1000 sessions) makes `statsFor` create a zero entry, and the first fold persists it over the file | [F4!](#f4), [F5](#f5) | `statsFor` seeds an unknown session from `ReadPersisted` before folding (once per session per process, under the store lock) | load every persisted file at startup: reads all session dirs, unbounded<br>merge at read time only: the file is still overwritten | **reliable:** the persisted file becomes the continuation point, so peek downtime loses only the decisions exported while it was down | one small file read under the write lock on a session's first decision after start |
| <a id="d4"></a>D4 | `counters.permission_denials` (transcript side) only sees prompt rejections; hook and native denials have no structured transcript marker | [F7](#f7) | unchanged; documented as "prompt rejections recorded in the transcript"; telemetry `denied` is the complete count | match `PreToolUse:` and native denial texts: native messages are free text with no shared marker, the count stays incomplete | **reliable:** one exact source (telemetry) beats a partial text heuristic | analyses needing all denials require telemetry export |
| <a id="d5"></a>D5 | Files persisted before this change carry the old counters | [F4!](#f4) | load as-is: `hook_decided` is ignored, `auto_allowed` keeps its old (inflated) value, new counters start at zero and grow | a migration re-splitting old counts: the decision per event is not in the file | **debuggable:** old sessions read honestly as old data; no invented split | pre-change sessions report `denied` from post-change decisions only |
| <a id="d6"></a>D6 | The claude-configs eval report reads `requests` | [F6](#f6) | out of scope here; Contracts row names the switch to `denied` | edit claude-configs in this plan: other repo, other session | this plan stays peek-only | the eval report stays wrong until claude-configs switches |
| <a id="d7"></a>D7 | Release vehicle | — | no version bump; the change joins the uncommitted v1.2.9 release | 1.2.10 | v1.2.9 is not tagged yet | release notes gain one line |

## Open questions

None.

## Baseline (verified)

- **Base:** peek-mcp branch `claude/skill-adjustments-peek-a93a72` (session worktree), HEAD `a60f720`.

| ID | Fact | Needed for | Location |
| :--- | :--- | :--- | :--- |
| <a id="f1"></a>F1! | Claude Code emits `tool_decision` with `decision = allow ? "accept" : "reject"` and `source = B3n(reason)`; `B3n` returns `config` for mode, classifier, subcommandResults, asyncAgent, sandboxOverride, workingDir, safetyCheck and unknown reasons, `hook` for hooks, `user_*` for prompts | [D1](#d1) | /opt/homebrew/Caskroom/claude-code@latest/2.1.282/claude (`Ko("tool_decision",…)`, `function B3n`) |
| <a id="f2"></a>F2! | `foldDecision` increments by source only: `config` → `AutoAllowed`, `hook` → `HookDecided` | [D1](#d1) | [telemetry/store.go:124-137](telemetry/store.go) |
| <a id="f3"></a>F3! | run `0ba783f3`: `requests` holds 200 entries (148 hook accepts, 41 user_permanent accepts, 11 hook rejects), window 00:58:45–01:24:19Z; the 9 "jq … dangerous flags" native denials in that window are absent | [D2](#d2) | ~/.peek/state/claude/0ba783f3-b716-4bcb-ac54-e87feba798d9/telemetry.json |
| <a id="f4"></a>F4! | installed peek restarted 2026-09-28 00:23:46+02:00; this session's persisted `requests` start 22:29:46Z; the 7 gate denials before the restart are missing, the 3 after present | [D3](#d3), [D5](#d5) | ~/Library/Logs/smine-configserver.err.log line 589541; ~/.peek/state/claude/41f8fb14-…/telemetry.json |
| <a id="f5"></a>F5 | `ReadPersisted` returns `false` for a nil dir, a missing file, or invalid JSON | [D3](#d3) | [telemetry/store.go:195-210](telemetry/store.go) |
| <a id="f6"></a>F6 | the only consumer, claude-configs `internal/peek/events.go`, decodes `permissions.requests` and counts `decision == "reject"`; nothing reads `hook_decided` (a fixture key in its test only) | [D1](#d1), [D2](#d2), [D6](#d6) | /Users/kevinpersonal/GolandProjects/claude-configs/internal/peek/events.go:140-160 |
| <a id="f7"></a>F7 | transcript denials in run `0ba783f3`: 440 `PreToolUse:` hook errors plus free-text native denials ("jq command contains dangerous flags…" 23, "mkdir in '…'" 16, "Shell expansion syntax…" 7) | [D4](#d4) | transcripts under ~/.claude/projects/-Users-kevinpersonal--cache-claude-routine-worktrees-skilleval-2026-09-27-a34281-A/0ba783f3-… |
| <a id="f9"></a>F9 | a listed request entry averages 381 bytes JSON (max 533); run `0ba783f3` had 501 denials per its result envelopes; 2 persisted sessions sit at the old 200 cap | [D2](#d2) | ~/.peek/state/claude/*/telemetry.json; evals/railroad-review/2026-09-27-a34281/runs/A/stream.jsonl (claude-configs) |
| <a id="f8"></a>F8 | `claude` 2.1.282 accepts `--settings <file-or-json>` | [Verification](#verification) | claude binary option table |

## Exemplar & reuse

| Existing | Used for |
| :--- | :--- |
| `ReadPersisted` ([telemetry/store.go:195](telemetry/store.go)) | seeding a session in `statsFor` |
| `decisionRecord`, `logsPayload` ([telemetry/otlp_test.go:98-110](telemetry/otlp_test.go)) | new ingest test cases |
| `promptedDecisionLogs` ([tools/viewmodels_events_test.go:157](tools/viewmodels_events_test.go)) | view test payload shape |

- Every change modifies an existing unit; no new file, type or package.

## Permissions

| Rule | Needed for | Layer |
| :--- | :--- | :--- |
| `Bash(claude -p *)` | [Verification](#verification): real headless Claude Code run exporting OTLP to the new build | settings.local.json |

## Changes

| File | Kind | Entry |
| :--- | :--- | :--- |
| `telemetry/otlp.go` | modified | [§1](#1-decision-constant-modified) |
| `telemetry/store.go` | modified | [§2](#2-decision-keyed-counters-and-restore-modified) |
| `tools/viewmodels_events.go` | modified | [§3](#3-permissions-view-modified) |
| `docs/reference.md` | modified | [§4](#4-docs-modified) |
| `docs/tools.md` | modified | [§4](#4-docs-modified) |
| `telemetry/store_test.go` | modified | [Tests](#tests) |
| `telemetry/otlp_test.go` | modified | [Tests](#tests) |
| `tools/viewmodels_events_test.go` | modified | [Tests](#tests) |

### 1. Decision constant (modified)

location: `telemetry/otlp.go`

```diff
 	sourceUserReject    = "user_reject"
 	sourceUserAbort     = "user_abort"
+
+	decisionReject = "reject"
 )
```

### 2. Decision-keyed counters and restore (modified)

location: `telemetry/store.go`

- **Cap ([D2](#d2)):**

```diff
 const (
 	maxSessions           = 1000
-	maxPermissionRequests = 200
+	maxPermissionRequests = 1000
 	maxPendingCommands    = 256
```

- **PermissionStats:** fields sorted alphabetically (RULE-STRUCT-002) while the type changes; `HookDecided` removed.

```go
type PermissionStats struct {
	Aborted        int                  `json:"aborted,omitempty"`
	AutoAllowed    int                  `json:"auto_allowed,omitempty"`
	ConfigDenied   int                  `json:"config_denied,omitempty"`
	Denied         int                  `json:"denied,omitempty"`
	HookAllowed    int                  `json:"hook_allowed,omitempty"`
	HookDenied     int                  `json:"hook_denied,omitempty"`
	PromptedAlways int                  `json:"prompted_always,omitempty"`
	PromptedOnce   int                  `json:"prompted_once,omitempty"`
	Rejected       int                  `json:"rejected,omitempty"`
	Requests       []PermissionDecision `json:"requests,omitempty"`
}

// count tallies by decision first: Denied covers every source, so a config
// denial or a source Claude Code adds later is never read as an allow.
func (p *PermissionStats) count(decision *PermissionDecision) {
	isReject := decision.Decision == decisionReject
	if isReject {
		p.Denied++
	}

	switch decision.Source {
	case sourceConfig:
		if isReject {
			p.ConfigDenied++
			return
		}
		p.AutoAllowed++
	case sourceHook:
		if isReject {
			p.HookDenied++
			return
		}
		p.HookAllowed++
	case sourceUserTemporary:
		p.PromptedOnce++
	case sourceUserPermanent:
		p.PromptedAlways++
	case sourceUserReject:
		p.Rejected++
	case sourceUserAbort:
		p.Aborted++
	}
}

func (p *PermissionStats) IsZero() bool {
	hasNoAllows := p.AutoAllowed == 0 && p.HookAllowed == 0 && p.PromptedOnce == 0 && p.PromptedAlways == 0
	return hasNoAllows && p.Denied == 0 && len(p.Requests) == 0
}
```

- **statsFor:** seeds an unknown session from disk ([D3](#d3)); `restoredStats` follows `persist` in the private-method order of `Store`.

```diff
 func (s *Store) statsFor(sessionId string) *SessionStats {
 	stats, ok := s.sessions[sessionId]
 	if !ok {
 		if len(s.sessions) >= maxSessions {
 			s.evictOldest()
 		}
-		stats = &SessionStats{}
+		stats = s.restoredStats(sessionId)
 		s.sessions[sessionId] = stats
 	}
```

```go
// restoredStats continues a session peek holds no memory of (restart,
// eviction) from its persisted file instead of overwriting it with zero.
func (s *Store) restoredStats(sessionId string) *SessionStats {
	persisted, ok := ReadPersisted(s.StateDir, sessionId)
	if !ok {
		return &SessionStats{}
	}
	return &persisted
}
```

- **foldDecision:** counting moves to `count`; listing is decided by `isListedDecision` ([D2](#d2)).

```diff
 func (s *Store) foldDecision(sessionId string, decision PermissionDecision) {
 	s.mu.Lock()
 	defer s.mu.Unlock()
 
 	stats := s.statsFor(sessionId)
-
-	switch decision.Source {
-	case sourceConfig:
-		stats.Permissions.AutoAllowed++
-	// ... (hook, user_temporary, user_permanent, user_reject, user_abort cases)
-	}
-
-	if decision.Source != sourceConfig && len(stats.Permissions.Requests) < maxPermissionRequests {
+	stats.Permissions.count(&decision)
+
+	hasRoom := len(stats.Permissions.Requests) < maxPermissionRequests
+	if isListedDecision(&decision) && hasRoom {
 		stats.Permissions.Requests = append(stats.Permissions.Requests, decision)
```

```go
// isListedDecision keeps the list to what an operator acts on: every denial
// and every prompted decision; config and hook allows are counted only.
func isListedDecision(decision *PermissionDecision) bool {
	if decision.Decision == decisionReject {
		return true
	}
	return decision.Source == sourceUserTemporary || decision.Source == sourceUserPermanent
}
```

### 3. Permissions view (modified)

location: `tools/viewmodels_events.go`
mirrors: the existing `permissionsView` / `permissionsViewFromStats` pair

```go
type permissionsView struct {
	Aborted        int                            `json:"aborted,omitempty"`
	AutoAllowed    int                            `json:"auto_allowed"`
	ConfigDenied   int                            `json:"config_denied"`
	Denied         int                            `json:"denied"`
	Detail         string                         `json:"detail,omitempty"`
	HookAllowed    int                            `json:"hook_allowed"`
	HookDenied     int                            `json:"hook_denied"`
	PromptedAlways int                            `json:"prompted_always"`
	PromptedOnce   int                            `json:"prompted_once"`
	Rejected       int                            `json:"rejected"`
	Requests       []telemetry.PermissionDecision `json:"requests,omitempty"`
}

func permissionsViewFromStats(stats *telemetry.PermissionStats, detail string) *permissionsView {
	return &permissionsView{
		Aborted:        stats.Aborted,
		AutoAllowed:    stats.AutoAllowed,
		ConfigDenied:   stats.ConfigDenied,
		Denied:         stats.Denied,
		Detail:         detail,
		HookAllowed:    stats.HookAllowed,
		HookDenied:     stats.HookDenied,
		PromptedAlways: stats.PromptedAlways,
		PromptedOnce:   stats.PromptedOnce,
		Rejected:       stats.Rejected,
		Requests:       stats.Requests,
	}
}
```

### 4. Docs (modified)

location: `docs/reference.md`, `docs/tools.md`

```diff
-Logs: `claude_code.tool_decision` events are folded into per-session permission stats — auto-allowed (source `config`, which also covers `bypassPermissions` mode), prompted once/always, hook-decided, rejected, aborted — and every non-auto decision is listed with its tool; the matching `claude_code.tool_result` event supplies the command (requires `OTEL_LOG_TOOL_DETAILS=1`). Everything else is ignored. Folded stats are persisted per session to the state dir (`telemetry.json`), so they survive peek restarts.
+Logs: `claude_code.tool_decision` events are folded into per-session permission stats. `denied` counts every denied call regardless of who denied it; the per-source counters split it: `config_denied` (settings deny rules, safety checks, the working-directory fence, `dontAsk` mode, the auto-mode classifier), `hook_denied` (PreToolUse hooks), `rejected` (the user at a prompt), `aborted`. Allows split the same way: `auto_allowed` (settings, including `bypassPermissions`), `hook_allowed`, `prompted_once`, `prompted_always`. `requests` lists every denial and every prompted decision with its tool — up to 1000 per session, the first ones kept; the counters stay exact beyond that. The matching `claude_code.tool_result` event supplies the command (requires `OTEL_LOG_TOOL_DETAILS=1`). Everything else is ignored. Folded stats are persisted per session to the state dir (`telemetry.json`) and restored when a session's next decision arrives after a peek restart; decisions exported while peek was down are lost.
```

```diff
-… plus derived counters, a `permissions` block with telemetry-based permission decisions when [telemetry export](reference.md#telemetry) is enabled (auto-allowed vs. prompted vs. rejected counts, plus each prompted/rejected request with its tool and command — `detail: "persisted"` marks stats read back from the state dir), …
+… plus derived counters (`counters.permission_denials` counts only prompt rejections recorded in the transcript), a `permissions` block with telemetry-based permission decisions when [telemetry export](reference.md#telemetry) is enabled (`denied` is the complete count of denied calls from every source, split into `config_denied`, `hook_denied`, `rejected`, `aborted`; allows into `auto_allowed`, `hook_allowed`, `prompted_once`, `prompted_always`; `requests` lists each denial and prompted decision with its tool and command — `detail: "persisted"` marks stats read back from the state dir), …
```

## Hot items

- **H1 — locking (file read under the store write lock):** `restoredStats` runs inside `statsFor`, which every caller reaches holding `s.mu.Lock()` (`fold`, `foldDecision`). It reads one small JSON file, only on a session's first decision after start or after eviction; no goroutine, no second lock.

```go
func (s *Store) restoredStats(sessionId string) *SessionStats {
	persisted, ok := ReadPersisted(s.StateDir, sessionId)
	if !ok {
		return &SessionStats{}
	}
	return &persisted
}
```

- **H2 — guard (which decisions are listed):** replaces `decision.Source != sourceConfig` — hook accepts leave the list, config denials enter it.

```go
func isListedDecision(decision *PermissionDecision) bool {
	if decision.Decision == decisionReject {
		return true
	}
	return decision.Source == sourceUserTemporary || decision.Source == sourceUserPermanent
}
```

## Tests

| Location.Method | Cases | Comment |
| :--- | :--- | :--- |
| telemetry/store_test.go.TestStore_FoldDecision | `config-reject-denied-not-allowed`<br>`hook-split-by-decision`<br>`denied-counts-every-source`<br>`accepts-counted-not-listed`<br>`denial-listed-after-200-accepts`<br>`requests-capped-counters-keep-counting` (updated: `Denied` also 210) | sequential `t.Run` style of the existing function; `denial-listed-after-200-accepts`: 200 hook accepts, then one hook reject → `Requests` length 1, `HookAllowed` 200; the capped case loops `maxPermissionRequests + 10`, so it follows the new 1000 |
| telemetry/store_test.go.TestStore_Persist | `restart-continues-persisted-totals`<br>`eviction-reload-continues`<br>`legacy-file-keeps-old-counters` | restart: store A with `StateDir` folds a hook reject; fresh store B on the same dir folds another → `Denied` 2, `Requests` 2, file matches; legacy: file with `hook_decided` 5 and `auto_allowed` 3 → loads `AutoAllowed` 3, new counters 0, one fold → `Denied` 1 |
| telemetry/otlp_test.go.TestStore_IngestLogs | `config-reject-denied-and-listed`<br>`hook-accept-counted-not-listed`<br>`config-decision-counted-only` (unchanged) | OTLP payloads built with `decisionRecord` |
| tools/viewmodels_events_test.go.TestNewPermissionsView | `denied-fields-served` | live store with one config reject and one hook accept → `Denied` 1, `ConfigDenied` 1, `HookAllowed` 1, one request |

- **Existing tests pinning behavior:** `live-store-served`, `persisted-fallback`, control `TestHandleOtlpLogs/valid-payload-folded` (config accept stays `AutoAllowed` 1).
- **Not tested:** the headless `claude -p` run — it is verification against the real exporter, not a unit test.

## Test runbook

- **headless-denials:** `claude -p --settings <scratch>/otel.json` against the new build; one allowed call, one safety-check denial, one gate denial; `session_events` `permissions` compared with the run's result envelope `permission_denials`.
- **restart-continuity:** the same session resumed after restarting the build on the same state dir; `denied` continues.

## Contracts & sweeps

| Contract | Sides | Sweep |
| :--- | :--- | :--- |
| `session_events.permissions` JSON: `denied`, `config_denied`, `hook_allowed`, `hook_denied` added; `hook_decided` removed; `requests` holds denials and prompted decisions only | peek `tools/` ↔ claude-configs `internal/peek/events.go` | Grep `hook_decided\|HookDecided` over peek `*.go` and `docs/` → 0 hits; claude-configs switches `rejections()` to `denied` (their change, [D6](#d6)) |
| persisted `telemetry.json` shape | peek `telemetry/` writer ↔ reader | `legacy-file-keeps-old-counters` test |
| `counters.permission_denials` meaning | peek `session/` ↔ docs | documented in `docs/tools.md` |

## Verification

- [ ] Apply the Permissions row with one `toolpolicy allow 'Bash(claude -p *)'` call.
- [ ] Run `go vet ./...` — expect no output.
- [ ] Run `go test ./...` — expect every package `ok`.
- [ ] Run `gofmt -l telemetry tools` — expect no output.
- [ ] Grep `hook_decided` and `HookDecided` over `*.go` and `docs/` — expect 0 hits.
- [ ] Build `go build -o dist/peek-mcp .`, start `./dist/peek-mcp start --port 4293 --control-port 4393 --state-dir <scratch>/state` in the background — expect `control server listening` on 4393.
- [ ] Write `<scratch>/otel.json` with the Write tool: `env` holding `CLAUDE_CODE_ENABLE_TELEMETRY` `1`, `OTEL_LOGS_EXPORTER` `otlp`, `OTEL_EXPORTER_OTLP_PROTOCOL` `http/json`, `OTEL_EXPORTER_OTLP_ENDPOINT` `http://127.0.0.1:4393/otlp`, `OTEL_LOGS_EXPORT_INTERVAL` `1000`, `OTEL_LOG_TOOL_DETAILS` `1`.
- [ ] Run `claude -p --settings <scratch>/otel.json --output-format json` with a prompt that runs `ls`, then `jq -n --rawfile x /etc/hosts '$x'` (safety check), then `lsof -v` (gate: no allow rule) — expect the result envelope `permission_denials` length 2.
- [ ] Call `session_events` `{id: <run session id>, json: true}` on port 4293 — expect `permissions.denied` 2, `config_denied` 1, `hook_denied` 1, `requests` exactly those 2 tool_use_ids, no `accept` entry; `denied` equals the envelope count.
- [ ] Stop the build with TaskStop, start it again with the same state dir, run `claude -p --settings <scratch>/otel.json --resume <run session id>` with one more `lsof -v` — expect `denied` 3, `hook_denied` 2 (not 1).
- [ ] Degenerate: `session_events` for a session with no telemetry — expect no `permissions` block.
- [ ] Stop the build with TaskStop — expect ports 4293 and 4393 to refuse connections.
- [ ] Commit per package through /package-commit; the v1.2.9 release notes gain one line.

## Stop conditions

| ID | Condition | Action |
| :--- | :--- | :--- |
| ST1 | An approved signature or contract can't hold as planned | Stop and report. Never improvise architecture mid-edit |
| ST2 | Second failed fix on the same mechanism | Stop, research the actual cause, redesign. No third band-aid |
| ST3 | Missing prerequisite (generated code, running infra) | Run the producing step. If infrastructure is down, ask. Never skip validation, never start infrastructure yourself |
| ST4 | Discovered work materially exceeds the approved scope | Ask before continuing |
| ST5 | The same kind of bug found a second time | Inside own diff: fix every instance now. Pre-existing, outside the diff: report and ask before sweeping |
| ST6 | A structural obstacle (import cycle, package visibility) tempts a new abstraction | Stop and report. The fix is relocating the component, not indirection |
| ST7 | Live `denied` differs from the result envelope's `permission_denials` count | Stop and diagnose the fold or the exporter mapping; never adjust expectations |
| ST8 | `--settings` env does not reach the OTLP exporter (no decisions arrive on 4393) | Stop and report; never edit `~/.claude/settings.json` or point the installed peek elsewhere |
| ST9 | A change is needed outside the planned-files table (e.g. `control/`, `session/`) or in claude-configs | Stop and report |

## Changelog

| Date | Trigger | What changed |
| :--- | :--- | :--- |
| — | initial | plan created |
| 2026-09-28 | Q: can the 200-entry list cap be increased | D2: `maxPermissionRequests` 200 → 1000 [USER], sized by F9 (381 B per entry, largest run 501 denials); §2 diff, docs line, TLDR and Scope updated |
