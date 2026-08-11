# Telemetry detection status, editable global config, interactive usage stats — Change Plan

## TLDR

- peek learns whether Claude Code's telemetry exporter is actually configured: a `telemetry.Detector` inspects `~/.claude/settings.json` against the control server's **actually bound** port and yields `receiving | configured | misconfigured | not_configured` — surfaced in `session_events` (instead of silent omission), on `/stats`, and in one startup log line.
- `peek-mcp setup`'s telemetry step shrinks to one question ("Enable telemetry export to peek? [Y/n]") — no port prompt, no token prompt, headers key deleted, `OTEL_METRIC_EXPORT_INTERVAL=10000` written by default; when the control server is disabled it just says telemetry stays disabled.
- The `/stats` config block becomes editable: a new global `~/.peek/config.json` (new root `config` package, precedence flag > env > file > default) shared by all peek instances; five safe keys editable via claude-configs-style htmx rows with restart-required/overridden badges; ripple = shared file + existing restart button.
- The session Usage block becomes an interactive two-column layout: Total tokens fixed at render time (Claude summed), cache-hit percentage, structurally-zero rows hidden, and four clickable rows loading detail tables — cost estimate (new minimal `pricing` package), plan versions (renamed, with timestamps), skills (name/duration/tokens), permission denials (with the actual denied command — Claude parser gains command capture).
- No existing JSON contract changes; all ingest semantics stay pinned.

## Context

- The telemetry receiver shipped from [telemetry_extension.md](plans/deep_analysis/design/telemetry_extension.md); its silent-omission gap and setup UX surfaced in first real use.
- The /stats page shipped from [usage_stats_addendum.md](plans/control_server/design/usage_stats_addendum.md); its D6 `[USER]` decision scoped editable config out ("no config-file mechanism exists — a separate feature") — the user now explicitly requests that feature (stop condition 9 conflict surfaced; the newer request supersedes).
- Style reference for the config UI: the claude-configs repo (`internal/server/`) — peek's `control/assets/style.css` is already a port of the same brandbook CSS, so styling is template restructuring only.
- Plan persists to `plans/control_server/design/telemetry_status_config_usage.md`; runbook files to `plans/control_server/runbooks/`.

## Drivers

| ID | Observed | Wanted | Impact | Origin |
|---|---|---|---|---|
| <a id="dr1"></a>DR1 | `controlPort > 0` gates peek's receiver, not Claude's exporter; a missing `telemetry` block in `session_events` is indistinguishable from "enabled but no data yet"; the bound port (walk 42442–42499) is never stored; setup never writes `OTEL_METRIC_EXPORT_INTERVAL` | Best-effort three-state detection (vs the bound port) in `session_events`, `/stats`, and a startup log line; setup writes the export interval | behavioral | request pts 1–4 |
| <a id="dr1b"></a>DR1b | Setup telemetry step prompts for control port (already decided elsewhere in the flow) and for a token (no auth in use); asks even when the control server is disabled | One Y/n question; port derived; no token prompt; disabled control server → informational line only | behavioral | user feedback mid-planning |
| <a id="dr2"></a>DR2 | /stats config block is display-only, unstyled bare `<table>` (cramped "Control port42442"); config is load-once flags, no file | Styled per claude-configs; safe keys alterable from the page; alterations global, rippling to all peek instances | contract-touching | Screenshot1 |
| <a id="dr3"></a>DR3 | Usage block: Total tokens renders 0 for Claude (stored field never computed); flat key/value list, dead space; denials show only a count; plan/skill detail exists in memory but is not surfaced | Total fixed + cache percentage; left column table-like; click loads detail: cost estimate, plan versions w/ timestamps, skills w/ time+tokens, denial commands | behavioral | Screenshot2 |

## Scope

- **In:**
  - **detector:** `telemetry/detect.go` + wiring into `cmd/start.go`, `tools`, `control`
  - **bound port:** listen-before-construct reorder; `Config.ControlPort` = bound port
  - **setup:** single-question telemetry step + export interval
  - **config file:** new root `config` package, `~/.peek/config.json`, fallback pass in `cmd/start.go`
  - **config UI:** own fragment, editable rows, badges, `POST /api/config/{key}`
  - **stats styling:** runtime tables → `evidence-table`; config block → card rows
  - **usage fixes:** render-time total, cache percentage, zero-row hiding
  - **usage details:** four htmx detail fragments + routes + `.usage-grid`/`.usage-table` CSS
  - **pricing:** minimal embedded rate table package ([Q1](#d-p1))
  - **parser:** Claude denied-command capture
  - **docs:** README config-file section
- **Out:**
  - **full pricing concept:** `UsageByModel` ingest split, override file, `session_usage` tool ([usage_reporting concept](plans/usage_reporting/concept/concept.md) stays a separate feature)
  - **live-apply config:** no fsnotify watch, no runtime re-plumbing — restart applies
  - **risky config keys:** transport, port, control-port, control-token, homes, state-dir stay read-only ([Q2](#d-c2))
  - **OTLP logs/traces:** unchanged from telemetry_extension scope
- **Not changed:**
  - **JSON APIs:** `/api/sessions/{id}/usage`, `/api/stats` field shapes (only additive `telemetry_export`)
  - **ingest semantics:** `Usage.Add`, Codex keep-last, `TotalUsage` accumulation
  - **restart mechanism:** re-exec closure unchanged
  - **MCP tool surface:** only the additive `time.telemetry.status/detail` fields
- **Deferred findings:**
  - **Claude subagent spawn counter:** `SubagentsSpawned` is structurally 0 for Claude (parser emits only `subagent_result`) — same class of defect as Total tokens, not in any driver
  - **`/api/sessions/{id}/usage` TotalUsage vs CurrentUsage inconsistency:** pre-existing, documented in usage_stats_addendum

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| Request: skills breakdown "depends on telemetry enabled" | False — `Session.Skills []*SkillStat` (Skill, Args, StartedAt, EndedAt, Usage) is transcript-derived, always available | [session/session.go:130](session/session.go) |
| Request: settings.json is reliable ground truth | Best-effort only — shell-env exports are invisible; wording says so ("may still be enabled via shell env") | [cmd/setup.go:180](cmd/setup.go) |
| peek's stylesheet needs porting to claude-configs | Already a port of the same brandbook (`.card-config`, `.label-key`, `.inline-form`, badges present); cramping = bare `<table>` with no CSS rule | control/assets/style.css:183-361 |
| Setup knows whether the control server is enabled | Confirmed — `setupTelemetry(p, controlServer bool)` already receives and ignores the bool | [cmd/setup.go:173](cmd/setup.go) |
| `PlanRevision` carries version + timestamp | Confirmed: `Index`, `Timestamp`, `IsAlteration` | [session/plan_revision.go:5](session/plan_revision.go) |
| Denied command exists but is dropped (Claude) | Confirmed: `pendingToolUse.input` holds it; `permissionDeniedEvent` sets only `Tool` | [claude/parser.go:471](claude/parser.go) |

## Current state

| File | Lines (slice) | Responsibility |
|---|---|---|
| [cmd/start.go](cmd/start.go) | 80-83, 148-210, 245-261, 307-356 | telemetry-store gate, control wiring (construct-then-listen), flags + `PEEK_*` env fallbacks; bound port only logged (:204) |
| [cmd/listen.go](cmd/listen.go) | 11-32 | port walk 42442–42499 |
| [cmd/setup.go](cmd/setup.go) | 173-230, 315 | telemetry step (port+token prompts, no interval), `writeConfig`; only reader of `~/.claude/settings.json` |
| [telemetry/](telemetry/store.go) | store.go, otlp.go | side-store keyed by session.id; no detection code |
| [tools/tools.go](tools/tools.go) | 36, 281, 349-355, 427 | `Register`, `session_events` handler; telemetry block set only on store hit; Codex unsupported (post-merge line numbers, content unchanged) |
| [tools/viewmodels_events.go](tools/viewmodels_events.go) | 27-38 | `telemetryTimeView`, omitempty pointer field |
| [control/server.go](control/server.go) | 19-23, 35-36, 70-133 | embeds, Options, routes |
| [control/stats.go](control/stats.go) | 14-71 | stats builder, page/fragment handlers, restart |
| [control/templates/_stats.html](control/templates/_stats.html) | 1-32 | 10s self-poll, bare tables, config block, restart button |
| [control/templates/_usage.html](control/templates/_usage.html) | 1-16 | flat usage/counters list |
| [control/sessions.go](control/sessions.go) | 14-23, 121-160 | template constants, `usageData`, usage/events fragment handlers |
| [claude/parser.go](claude/parser.go) | 156-163, 242-249, 345-348, 471-479, 607, 657-666 | usage without TotalTokens; `pendingToolUse{input,name}`; denial events tool-only |
| [session/](session/session.go) | usage.go, session.go:130, plan_revision.go, event.go, event_buffer.go | Usage fields, SkillStat, PlanRevision, PermissionPayload{Command,Justification,Tool}, buffer cap 500 |
| — | — | no `pricing` code, no config file mechanism anywhere in the repo |

## Target state

```
~/.claude/settings.json ──read per call──▶ telemetry.Detector ──▶ session_events time.telemetry.status
                                               ▲ boundPort            /stats telemetry_export row
cmd/start.go: listen FIRST ─────────────────────┘                     startup slog line

~/.peek/config.json ◀──atomic save── POST /api/config/{key} (any instance's dashboard)
        │ load at startup (flag > env > file > default)
        ▼
every peek instance ──restart (existing button / client)──▶ new values live

session detail Usage:  [left table: rows, 4 clickable] ──hx-get──▶ [detail pane, hx-preserve]
                                                                    cost | plans | skills | denials
```

- **Detection (DR1).** Principle: stateless best-effort inspection at read time — no cache, no watcher, no goroutine. Mechanism: plain struct `telemetry.Detector` re-reading a ~1KB file per call; store hit short-circuits to `receiving`.
- **Global config (DR2).** Principle: single source of truth per key with explicit precedence; the file is the cross-instance rendezvous (like `~/.peek/state`). Mechanism: typed `config.File` with pointer fields (nil = unset), second `Changed(flag)`-guarded fallback pass, atomic temp+rename writes, restart-to-apply uniformly (no per-key live paths).
- **Usage interactivity (DR3).** Principle: reuse the existing fragment pipeline — every detail view is just another self-refreshing fragment. Mechanism: htmx `hx-get` rows, `hx-preserve` detail pane surviving the 1s outerHTML self-swap, render-time computation in `control` (never ingest).

## Behavior contract

- Every existing MCP tool field and control JSON field keeps shape and semantics; additions are new optional fields (`time.telemetry.status/detail`, `telemetry_export` on `/api/stats`).
- Ingest pinned: `Usage.Add` additive ([session/usage_test.go](session/usage_test.go)), Codex keep-last ([session/store_events_test.go:96](session/store_events_test.go)), `TotalUsage` main-loop-only.
- `/api/sessions/{id}/usage` byte-identical ([control/api_test.go:168](control/api_test.go)).
- Fragment th/td one-line adjacency kept ([control/pages_test.go:46-57](control/pages_test.go)).
- OTLP receiver behavior unchanged ([control/otlp_test.go:35](control/otlp_test.go)).
- Restart re-exec unchanged; `restart_available` in `/api/stats` unchanged.
- Intentional changes (the drivers): telemetry status field always present for Claude sessions when control server on; `/stats` Control port row now shows the bound port; config block markup replaced; `_usage.html` markup replaced; setup prompts change; Claude denial events gain `Command`.

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d-t1"></a>T1 | Where detection lives | telemetry pkg imported by cmd/tools/control | `telemetry.Detector{boundPort, settingsPath}` in new `telemetry/detect.go`; `Status()` re-reads per call — no cache, no watcher | Fewest concepts, one type reused by all three surfaces; file read trivially cheap at the call rates (session_events calls, 10s stats poll) |
| <a id="d-t2"></a>T2 | Receiving vs configured | store.Get is per-session evidence | Store hit short-circuits — settings.json never read when data exists | Data is strictly stronger evidence than config |
| <a id="d-t3"></a>T3 | Bound port before control.New | `control.New` and `listenLoopback` are independent ([cmd/start.go:148-192](cmd/start.go)) | Reorder: listen first inside the gate, `boundPort` from `controlLn.Addr().(*net.TCPAddr).Port`; `Config.ControlPort` = bound port; `tools.Register` moves below the block | Verified feasible; fixes the /stats port row for free |
| <a id="d-t4"></a>T4 | Strictness of "configured" | setup writes http/json + `http://127.0.0.1:<port>/otlp` | Configured iff enable truthy ∧ exporter contains `otlp` ∧ protocol == `http/json` ∧ endpoint http + loopback host + port == bound + path `/otlp` (trailing slash ok); per-signal `OTEL_EXPORTER_OTLP_METRICS_*` overrides ignored | Matches exactly what setup writes; anything else → misconfigured with want-vs-got detail |
| <a id="d-t5"></a>T5 | Where the /stats row goes | DR2 dismantles the config table into fragments | Telemetry export row goes in the **runtime** stats table (next to SSE clients), not the config block | It is runtime status, not config; avoids DR1/DR2 template collision |
| <a id="d-t6"></a>T6 | Setup prompts | `setupTelemetry` already receives the controlServer bool and ignores it | **[USER]** One prompt "Enable telemetry export to peek? [Y/n]"; port = `controlPortBase` (no prompt — rebind divergence covered by misconfigured detection); token prompt dropped, `OTEL_EXPORTER_OTLP_HEADERS` deleted; controlServer false → print "stays disabled", return; overwrite-guard and write-confirm prompts dropped | User feedback [DR1b](#dr1b); no new plumbing needed |
| <a id="d-t7"></a>T7 | Export interval | OTEL default 60000ms = minute-late data | Write `OTEL_METRIC_EXPORT_INTERVAL="10000"` unconditionally, no prompt | Fewer questions; 10s freshness/overhead tradeoff |
| <a id="d-c1"></a>C1 | Config file location/format | `~/.peek` is the existing rendezvous | `~/.peek/config.json`, JSON, tool-owned typed struct; only editable keys serialized; unknown keys dropped on rewrite | Sibling of state dir; typed Load/Save, no raw-map juggling |
| <a id="d-c2"></a>C2 | Editable key set | transport/port/control-port break connected clients; token = lockout trap; state-dir orphans state; homes change watch roots | **[USER]** `depth`, `poll-interval`, `poll-window`, `state-retention-days`, `log-level` — the 5 safe keys; everything else read-only | Behavior-only tuning, safe on any instance; homes/paths/ports excluded (wrong path silently empties the dashboard, ports break clients) |
| <a id="d-c3"></a>C3 | Precedence | `applyEnvFallbacks` marks env-set flags Changed via `flags.Set` | flag > env > file > default, as a second Changed-guarded fallback pass after `applyEnvFallbacks` | Existing pattern gives the precedence for free |
| <a id="d-c4"></a>C4 | Ripple mechanism | only log-level is trivially live; depth/poll/retention are baked into constructions; re-exec re-runs startup incl. file load | Startup-read only + per-row "restart required" badge + existing restart button; every config render re-reads the file (cross-instance drift visible on next fragment load); no fsnotify, no live-apply | Uniform restart semantics; live-applying one key on one instance while siblings stay stale is asymmetric state |
| <a id="d-c5"></a>C5 | Edit UI shape | stats fragment outerHTML-swaps every 10s — would clobber in-progress edits | Config in its **own fragment** `GET /fragments/config`, triggered `load, config-op from:body`; `POST /api/config/{key}` re-renders the single row + fires `HX-Trigger: config-op`; restart button moves into it | Mirrors claude-configs exactly; no timed poll over form inputs |
| <a id="d-c6"></a>C6 | Concurrent writes, no lock | writers are humans clicking Save | Atomic temp+rename per `state.Dir.writeFile`; read-modify-write; last-writer-wins, no flock | Rename kills the only real hazard (torn file); collisions rare and re-editable |
| <a id="d-c7"></a>C7 | Loader placement | `control` and `cmd` both need it | New root package `config`; precedence application stays in `cmd/start.go` beside `applyEnvFallbacks`; no interfaces | Avoids import cycle, keeps cobra out of the file model |
| <a id="d-c8"></a>C8 | Override visibility | a `PEEK_DEPTH` env var silently defeats a file edit after restart | Keys pinned by flag/env get an "overridden" badge; cmd computes the set (Changed after env pass, before file pass) and passes it down | claude-configs' exact pattern; removes a trap |
| <a id="d-u1"></a>U1 | Claude total tokens | parser never sets `TotalTokens`; ingest pinned | Render-time in control: `TotalTokens` if > 0, else `input+output+cache_creation+cache_read` | No ingest risk; retroactively fixes already-ingested sessions |
| <a id="d-u2"></a>U2 | Cache percentage | Claude input excludes cache tokens; Codex cached ⊂ input | Claude: `cache_read/(input+cache_creation+cache_read)`; Codex: `cached_input/input`; base 0 → row hidden | Matches each agent's token schema |
| <a id="d-u3"></a>U3 | Zero rows | Cached input / Reasoning output structurally 0 for Claude; Cache creation/read 0 for Codex | Hide token rows with value 0; counter rows always render | Zero token rows are schema artifacts; zero counters are real information (pinned) |
| <a id="d-u4"></a>U4 | Click mechanism | usage fragment outerHTML-swaps every 1s | htmx `hx-get` on `<tr>` targeting `#usage-detail-{{.Id}}` carrying `hx-preserve`; each loaded detail self-refreshes; no persistent selection highlight (hover only) | Mirrors every existing fragment, zero JS; selection state would die each swap anyway |
| <a id="d-p1"></a>P1 | Pricing scope | no pricing code exists; full concept designed in [usage_reporting](plans/usage_reporting/concept/concept.md) | **[USER]** minimal embedded slice: per-MTok rate map in new `pricing` package, longest-prefix match on `Meta.Model` (last model wins), no override file, "estimate" labeled | ~120 LOC, zero ingest risk, upgradeable to the full concept later; full concept stays a separate feature |
| <a id="d-u6"></a>U6 | Claude denial detail | command in `pending.input`, dropped; `PermissionPayload.Command` exists (Codex fills it) | `permissionDeniedEvent` gains a command param; named `deniedToolInput{Command,FilePath,NotebookPath}` extractor: command → file_path → notebook_path; 3 call sites swept | Reuses the existing payload field; no schema change |
| <a id="d-u7"></a>U7 | "Plan versions" value | `Counters.PlanAlterations` counts alterations, not versions | Renamed row shows `len(sess.PlanRevisions)`; alteration flag becomes a detail-table column; "Plan rejections" row kept | Label says versions, count versions |
| <a id="d-u8"></a>U8 | Denials data source | only per-denial source is the EventBuffer (cap 500 FIFO) | Detail reads `Events.All()` filtered to `permission_denied`, newest first; cap noted in the fragment footer | Only source that exists; accepted limitation |
| <a id="d-x1"></a>X1 | Template define convention | repo fragments carry no `{{define}}`; ParseFS names templates by filename | New `_config.html`/`_config_row.html` written WITHOUT define wrappers; `_config.html` includes rows via `{{template "_config_row.html" .}}` | Matches every existing fragment |

## Changes

Phasing: each phase independently shippable; the app works after every phase.

---

### Phase 1 — telemetry detection status + setup ([DR1](#dr1), [DR1b](#dr1b))

**1.1 Export detector** (new)
location: `telemetry/detect.go`
mirrors: [telemetry/store.go](telemetry/store.go) (package sibling; plain concrete struct)

```go
package telemetry

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	envEnableTelemetry = "CLAUDE_CODE_ENABLE_TELEMETRY"
	envMetricsExporter = "OTEL_METRICS_EXPORTER"
	envOtlpEndpoint    = "OTEL_EXPORTER_OTLP_ENDPOINT"
	envOtlpProtocol    = "OTEL_EXPORTER_OTLP_PROTOCOL"

	requiredProtocol = "http/json"
)

type ExportState string

const (
	ExportConfigured    ExportState = "configured"
	ExportMisconfigured ExportState = "misconfigured"
	ExportNotConfigured ExportState = "not_configured"
	ExportReceiving     ExportState = "receiving"
)

type ExportStatus struct {
	Detail string      `json:"detail,omitempty"`
	State  ExportState `json:"status"`
}

type claudeSettings struct {
	Env map[string]any `json:"env"`
}

type Detector struct {
	boundPort    int
	settingsPath string
}

func NewDetector(boundPort int, settingsPath string) *Detector {
	return &Detector{boundPort: boundPort, settingsPath: settingsPath}
}

func (d *Detector) Status() ExportStatus {
	settingsData, err := os.ReadFile(d.settingsPath)
	if err != nil {
		return notConfiguredStatus("no telemetry env in " + d.settingsPath + " (may still be enabled via shell env)")
	}

	var settings claudeSettings
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		return notConfiguredStatus("cannot parse " + d.settingsPath)
	}

	enabled := envString(settings.Env, envEnableTelemetry)
	if enabled == "" {
		return notConfiguredStatus("no telemetry env in " + d.settingsPath + " (may still be enabled via shell env)")
	}
	isEnabled := enabled == "1" || strings.EqualFold(enabled, "true")
	if !isEnabled {
		return notConfiguredStatus(envEnableTelemetry + "=" + enabled)
	}

	problems := make([]string, 0)
	protocol := envString(settings.Env, envOtlpProtocol)
	if protocol != requiredProtocol {
		problems = append(problems, fmt.Sprintf("protocol %q (want %s)", protocol, requiredProtocol))
	}

	exporter := envString(settings.Env, envMetricsExporter)
	if !strings.Contains(exporter, "otlp") {
		problems = append(problems, fmt.Sprintf("metrics exporter %q (want otlp)", exporter))
	}

	if problem := d.endpointProblem(envString(settings.Env, envOtlpEndpoint)); problem != "" {
		problems = append(problems, problem)
	}

	if len(problems) > 0 {
		return ExportStatus{Detail: strings.Join(problems, "; "), State: ExportMisconfigured}
	}
	return ExportStatus{State: ExportConfigured}
}

func (d *Detector) endpointProblem(endpoint string) string {
	expected := fmt.Sprintf("http://127.0.0.1:%d/otlp", d.boundPort)
	if endpoint == "" {
		return "no " + envOtlpEndpoint + " (want " + expected + ")"
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Sprintf("endpoint %q is not a valid URL (want %s)", endpoint, expected)
	}

	mismatch := fmt.Sprintf("endpoint %q (want %s)", endpoint, expected)
	if parsed.Scheme != "http" {
		return mismatch
	}
	hostname := parsed.Hostname()
	isLoopback := hostname == "127.0.0.1" || hostname == "localhost"
	if !isLoopback {
		return mismatch
	}
	if parsed.Port() != strconv.Itoa(d.boundPort) {
		return mismatch
	}
	if strings.TrimSuffix(parsed.Path, "/") != "/otlp" {
		return mismatch
	}
	return ""
}

func notConfiguredStatus(detail string) ExportStatus {
	return ExportStatus{Detail: detail, State: ExportNotConfigured}
}

func envString(env map[string]any, key string) string {
	value, ok := env[key]
	if !ok {
		return ""
	}

	text, ok := value.(string)
	if ok {
		return text
	}
	return fmt.Sprint(value)
}
```

**1.2 Bind first, detector wiring, startup log** (modified)
location: [cmd/start.go](cmd/start.go) (adds import `"net"`)

```diff
 		invocations := tools.NewInvocationCounter()
-		tools.Register(srv, store, invocations, telemetryStore)

+		var detector *telemetry.Detector
 		if controlPort > 0 {
+			controlLn, err := listenLoopback(controlPort, controlPort+controlPortSpan-1)
+			if err != nil {
+				slog.Error("control server error", "err", err)
+				os.Exit(1)
+			}
+
+			boundPort := controlLn.Addr().(*net.TCPAddr).Port
+			if claudeHome != "" {
+				detector = telemetry.NewDetector(boundPort, filepath.Join(claudeHome, "settings.json"))
+				exportStatus := detector.Status()
+				slog.Info("telemetry export", "status", exportStatus.State, "detail", exportStatus.Detail)
+			}
+
 			controlOpts := &control.Options{
 				Store:       store,
 				Broker:      broker,
 				Telemetry:   telemetryStore,
+				Detector:    detector,
 				Token:       controlToken,
 				// ...
 				Config: control.Config{
 					// ...
 					StateRetentionDays: stateRetentionDays,
-					ControlPort:        controlPort,
+					ControlPort:        boundPort,
 					TokenSet:           controlToken != "",
```

```diff
 			controlServer, err := control.New(controlOpts)
 			if err != nil {
 				slog.Error("control server init error", "err", err)
 				os.Exit(1)
 			}

-			controlLn, err := listenLoopback(controlPort, controlPort+controlPortSpan-1)
-			if err != nil {
-				slog.Error("control server error", "err", err)
-				os.Exit(1)
-			}
-
 			controlHTTP := &http.Server{Handler: controlServer.Handler()}
 			// ...
 		}
+
+		tools.Register(srv, store, invocations, telemetryStore, detector)

 		switch transport {
```

**1.3 session_events status field** (modified)
location: [tools/viewmodels_events.go](tools/viewmodels_events.go)
mirrors: `newSessionTimeView` (same file, nil-returning view builder)

```diff
 type telemetryTimeView struct {
-	ActiveSeconds int     `json:"active_seconds"`
-	CostUSD       float64 `json:"cost_usd,omitempty"`
+	ActiveSeconds int                   `json:"active_seconds,omitempty"`
+	CostUSD       float64               `json:"cost_usd,omitempty"`
+	Detail        string                `json:"detail,omitempty"`
+	Status        telemetry.ExportState `json:"status"`
 }
```

```go
func newTelemetryTimeView(currentSession *session.Session, detector *telemetry.Detector, telemetryStore *telemetry.Store) *telemetryTimeView {
	if currentSession.Agent != session.AgentClaude {
		return nil
	}
	if telemetryStore == nil {
		return nil
	}

	if stats, ok := telemetryStore.Get(string(currentSession.Meta.SessionId)); ok {
		view := &telemetryTimeView{
			ActiveSeconds: int(stats.ActiveSeconds),
			CostUSD:       stats.CostUSD,
			Status:        telemetry.ExportReceiving,
		}
		return view
	}

	if detector == nil {
		return nil
	}
	status := detector.Status()
	view := &telemetryTimeView{Detail: status.Detail, Status: status.State}
	return view
}
```

location: [tools/tools.go](tools/tools.go)

```diff
-func Register(server *server.MCPServer, store *session.Store, counter *InvocationCounter, telemetryStore *telemetry.Store) {
+func Register(server *server.MCPServer, store *session.Store, counter *InvocationCounter, telemetryStore *telemetry.Store, detector *telemetry.Detector) {
```

```diff
-	server.AddTool(sessionEvents, counted(counter, "session_events", sessionEventsHandler(store, eventsPageStore, telemetryStore)))
+	server.AddTool(sessionEvents, counted(counter, "session_events", sessionEventsHandler(detector, store, eventsPageStore, telemetryStore)))
```

```diff
-func sessionEventsHandler(s *session.Store, pageStore *PageStore[*sessionEventsResult], telemetryStore *telemetry.Store) server.ToolHandlerFunc {
+func sessionEventsHandler(detector *telemetry.Detector, s *session.Store, pageStore *PageStore[*sessionEventsResult], telemetryStore *telemetry.Store) server.ToolHandlerFunc {
 	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
 		// ...
 		firstPage.Time = newSessionTimeView(currentSession)
-		if telemetryStore != nil && firstPage.Time != nil {
-			if stats, ok := telemetryStore.Get(string(currentSession.Meta.SessionId)); ok {
-				firstPage.Time.Telemetry = &telemetryTimeView{
-					ActiveSeconds: int(stats.ActiveSeconds),
-					CostUSD:       stats.CostUSD,
-				}
-			}
-		}
+		if firstPage.Time != nil {
+			firstPage.Time.Telemetry = newTelemetryTimeView(currentSession, detector, telemetryStore)
+		}
 		firstPage.TouchedFiles = newTouchedFileViews(currentSession)
```

**1.4 /stats telemetry export row** (modified)
location: [control/server.go](control/server.go), [control/viewmodels.go](control/viewmodels.go), [control/stats.go](control/stats.go), [control/templates/_stats.html](control/templates/_stats.html)

```diff
 type Options struct {
 	Store       *session.Store
 	Broker      *events.Broker
 	Telemetry   *telemetry.Store
+	Detector    *telemetry.Detector
 	Token       string
```

(same one-line additions to `Server` struct and the `New` constructor return)

```diff
 type statsResponse struct {
 	// ...
 	SSEClients       int64          `json:"sse_clients"`
 	Config           Config         `json:"config"`
+	TelemetryExport  *telemetry.ExportStatus `json:"telemetry_export,omitempty"`
 	RestartAvailable bool           `json:"restart_available"`
 }
```

```diff
 	if s.invocations != nil {
 		resp.Invocations = s.invocations.Snapshot()
 	}
+	if s.detector != nil {
+		exportStatus := s.detector.Status()
+		resp.TelemetryExport = &exportStatus
+	}
 	return resp
```

Template — row goes in the **runtime** table ([T5](#d-t5)):

```diff
   <tr><th>Sessions</th><td>{{.Sessions.Claude}} claude · {{.Sessions.Codex}} codex · {{.Sessions.Total}} total</td></tr>
   <tr><th>SSE clients</th><td>{{.SSEClients}}</td></tr>
+  {{if .TelemetryExport}}<tr><th>Telemetry export</th><td>{{.TelemetryExport.State}}{{if .TelemetryExport.Detail}} · {{.TelemetryExport.Detail}}{{end}}</td></tr>{{end}}
 </table>
```

**1.5 Single-question setup** (modified)
location: [cmd/setup.go](cmd/setup.go)

```diff
+const defaultMetricExportIntervalMs = "10000"
+
-func setupTelemetry(p *prompter, _ bool) error {
+func setupTelemetry(p *prompter, controlServer bool) error {
 	fmt.Println("Enabling Claude Code telemetry export to peek-mcp...")

+	if !controlServer {
+		fmt.Println("  Telemetry export stays disabled because the control server is disabled.")
+		return nil
+	}
+
 	home, err := os.UserHomeDir()
```

```diff
 	env, _ := cfg["env"].(map[string]any)
 	if env == nil {
 		env = map[string]any{}
 	}
-	if _, exists := env["CLAUDE_CODE_ENABLE_TELEMETRY"]; exists {
-		if !p.Confirm("  Telemetry is already configured. Overwrite?", false) {
-			fmt.Println("  Skipped.")
-			return nil
-		}
-	}
-
-	port := p.Ask("  Control server port", strconv.Itoa(controlPortBase))
-	token := p.Ask("  Control server token (empty for none)", "")
+	if !p.Confirm("  Enable telemetry export to peek?", true) {
+		fmt.Println("  Skipped.")
+		return nil
+	}

 	env["CLAUDE_CODE_ENABLE_TELEMETRY"] = "1"
 	env["OTEL_METRICS_EXPORTER"] = "otlp"
 	env["OTEL_EXPORTER_OTLP_PROTOCOL"] = "http/json"
-	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = fmt.Sprintf("http://127.0.0.1:%s/otlp", port)
-	if token != "" {
-		env["OTEL_EXPORTER_OTLP_HEADERS"] = "Authorization=Bearer " + token
-	} else {
-		delete(env, "OTEL_EXPORTER_OTLP_HEADERS")
-	}
+	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = fmt.Sprintf("http://127.0.0.1:%d/otlp", controlPortBase)
+	env["OTEL_METRIC_EXPORT_INTERVAL"] = defaultMetricExportIntervalMs
+	delete(env, "OTEL_EXPORTER_OTLP_HEADERS")
 	cfg["env"] = env

-	if !p.Confirm("  Write telemetry config?", true) {
-		fmt.Println("  Skipped.")
-		return nil
-	}
 	if err := writeConfig(path, cfg); err != nil {
 		return err
 	}
```

- `strconv` stays imported (`setupCodex` uses `strconv.Quote`).

---

### Phase 2 — global editable config ([DR2](#dr2))

**2.1 Config file model, load, save, validation** (new)
location: `config/file.go`
mirrors: [state/dir.go:66-80](state/dir.go) (atomic write); claude-configs `internal/server/config.go` (per-key validation)

```go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

const (
	KeyDepth              = "depth"
	KeyLogLevel           = "log-level"
	KeyPollInterval       = "poll-interval"
	KeyPollWindow         = "poll-window"
	KeyStateRetentionDays = "state-retention-days"

	dirPerm  = 0o700
	filePerm = 0o600

	maxDepth         = 1000
	maxRetentionDays = 3650
	minDepth         = 1
	minPollInterval  = time.Second
	minPollWindow    = time.Minute
)

var EditableKeys = []string{KeyDepth, KeyPollInterval, KeyPollWindow, KeyStateRetentionDays, KeyLogLevel}

var logLevels = []string{"debug", "info", "warn", "error"}

type File struct {
	Depth              *int    `json:"depth,omitempty"`
	LogLevel           *string `json:"log_level,omitempty"`
	PollInterval       *string `json:"poll_interval,omitempty"`
	PollWindow         *string `json:"poll_window,omitempty"`
	StateRetentionDays *int    `json:"state_retention_days,omitempty"`
}

func (f *File) Set(key, value string) error {
	switch key {
	case KeyDepth:
		return f.setDepth(value)
	case KeyLogLevel:
		return f.setLogLevel(value)
	case KeyPollInterval:
		return f.setPollInterval(value)
	case KeyPollWindow:
		return f.setPollWindow(value)
	case KeyStateRetentionDays:
		return f.setStateRetentionDays(value)
	}
	return errors.Errorf("File.Set: Unknown or non-editable key: %s", key)
}

func (f *File) setDepth(value string) error {
	depth, err := strconv.Atoi(value)
	if err != nil {
		return errors.Errorf("File.setDepth: Invalid field depth: %s (want an integer)", value)
	}
	if depth < minDepth || depth > maxDepth {
		return errors.Errorf("File.setDepth: Invalid field depth: %d (want %d-%d)", depth, minDepth, maxDepth)
	}

	f.Depth = &depth
	return nil
}

func (f *File) setLogLevel(value string) error {
	if !slices.Contains(logLevels, value) {
		return errors.Errorf("File.setLogLevel: Invalid field log-level: %s (want debug|info|warn|error)", value)
	}

	f.LogLevel = &value
	return nil
}

func (f *File) setPollInterval(value string) error {
	interval, err := time.ParseDuration(value)
	if err != nil {
		return errors.Errorf("File.setPollInterval: Invalid field poll-interval: %s (want a Go duration, e.g. 5s)", value)
	}
	if interval < minPollInterval {
		return errors.Errorf("File.setPollInterval: Invalid field poll-interval: %s (want >= 1s)", value)
	}

	normalized := interval.String()
	f.PollInterval = &normalized
	return nil
}

func (f *File) setPollWindow(value string) error {
	window, err := time.ParseDuration(value)
	if err != nil {
		return errors.Errorf("File.setPollWindow: Invalid field poll-window: %s (want a Go duration, e.g. 1h)", value)
	}
	if window < minPollWindow {
		return errors.Errorf("File.setPollWindow: Invalid field poll-window: %s (want >= 1m)", value)
	}

	normalized := window.String()
	f.PollWindow = &normalized
	return nil
}

func (f *File) setStateRetentionDays(value string) error {
	days, err := strconv.Atoi(value)
	if err != nil {
		return errors.Errorf("File.setStateRetentionDays: Invalid field state-retention-days: %s (want an integer)", value)
	}
	if days < 0 || days > maxRetentionDays {
		return errors.Errorf("File.setStateRetentionDays: Invalid field state-retention-days: %d (want 0-%d, 0 disables GC)", days, maxRetentionDays)
	}

	f.StateRetentionDays = &days
	return nil
}

func (f *File) FlagValues() map[string]string {
	values := make(map[string]string)
	if f.Depth != nil {
		values[KeyDepth] = strconv.Itoa(*f.Depth)
	}
	if f.LogLevel != nil {
		values[KeyLogLevel] = *f.LogLevel
	}
	if f.PollInterval != nil {
		values[KeyPollInterval] = *f.PollInterval
	}
	if f.PollWindow != nil {
		values[KeyPollWindow] = *f.PollWindow
	}
	if f.StateRetentionDays != nil {
		values[KeyStateRetentionDays] = strconv.Itoa(*f.StateRetentionDays)
	}
	return values
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("~", ".peek", "config.json")
	}
	return filepath.Join(home, ".peek", "config.json")
}

func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &File{}, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "Load: Failed to read config file")
	}

	file := &File{}
	if err := json.Unmarshal(data, file); err != nil {
		return nil, errors.Wrapf(err, "Load: Invalid config file %s", path)
	}
	return file, nil
}

func Save(path string, file *File) error {
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return errors.Wrap(err, "Save: Failed to marshal config")
	}

	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return errors.Wrap(err, "Save: Failed to create config directory")
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, filePerm); err != nil {
		return errors.Wrap(err, "Save: Failed to write temp file")
	}

	if err := os.Rename(tmp, path); err != nil {
		return errors.Wrap(err, "Save: Failed to rename temp file")
	}
	return nil
}
```

On-disk shape:

```json
{
  "depth": 50,
  "log_level": "debug",
  "poll_interval": "10s"
}
```

**2.2 Fallback pass + wiring** (modified)
location: [cmd/start.go](cmd/start.go)
mirrors: `applyEnvFallbacks` ([cmd/start.go:341](cmd/start.go))

```diff
 	Run: func(cmd *cobra.Command, args []string) {
 		startedAt := time.Now()
 		applyEnvFallbacks(cmd)
+		overriddenKeys := changedConfigKeys(cmd)
+		configPath := config.DefaultPath()
+		configFile, err := config.Load(configPath)
+		if err != nil {
+			slog.Warn("start: Ignoring invalid config file", "path", configPath, "err", err)
+			configFile = &config.File{}
+		}
+		applyConfigFileFallbacks(cmd, configFile)
 		warnMaxOutputTokens()
 		flags := cmd.Flags()
```

```diff
 			controlOpts := &control.Options{
 				// ...
 				Invocations: invocations,
+				ConfigPath:     configPath,
+				OverriddenKeys: overriddenKeys,
 				Config: control.Config{
```

```go
func applyConfigFileFallbacks(cmd *cobra.Command, file *config.File) {
	for flag, value := range file.FlagValues() {
		if !cmd.Flags().Changed(flag) {
			cmd.Flags().Set(flag, value)
		}
	}
}

func changedConfigKeys(cmd *cobra.Command) map[string]bool {
	changed := make(map[string]bool)
	for _, key := range config.EditableKeys {
		changed[key] = cmd.Flags().Changed(key)
	}
	return changed
}
```

- Ordering is load-bearing: `changedConfigKeys` after `applyEnvFallbacks` (env marks Changed), before the file pass — the map is exactly "pinned by flag or env" ([C8](#d-c8)).
- Invalid file is non-fatal: warn and continue on flags/env/defaults.
- Note: `slog` level is configured after this point in `Run`; the warn line uses the default logger — acceptable.

**2.3 Options and routes** (modified)
location: [control/server.go](control/server.go)

```diff
 type Options struct {
 	// ... (incl. Detector from Phase 1)
 	Invocations *tools.InvocationCounter
 	Config      Config
+	ConfigPath     string
+	OverriddenKeys map[string]bool
 	Restart     func()
 }
```

(same additions to `Server` struct and `New`)

```diff
 	mux.HandleFunc("GET /fragments/stats", s.handleStatsFragment)
+	mux.HandleFunc("GET /fragments/config", s.handleConfigFragment)
 	// ...
 	mux.HandleFunc("POST /api/restart", s.handleRestart)
+	mux.HandleFunc("POST /api/config/{key}", s.handleConfigSet)
```

- The new POST inherits token auth + loopback host check + SameSite-strict cookie — no new middleware.

**2.4 Config fragment handlers** (new)
location: `control/config.go`
mirrors: [control/sessions.go](control/sessions.go) fragment handlers; claude-configs `handleConfigSet`/`renderConfigRow`

```go
package control

import (
	"net/http"
	"strconv"

	"github.com/kevinhorst/peek-mcp/config"
)

const (
	tmplConfig    = "_config.html"
	tmplConfigRow = "_config_row.html"
)

type configData struct {
	RestartAvailable bool
	Rows             []configRow
}

type configRow struct {
	Editable      bool
	Explanation   string
	Key           string
	Overridden    bool
	RestartNeeded bool
	SavedValue    string
	Type          string
	Value         string
	Values        []string
}

func (s *Server) configRows() ([]configRow, error) {
	file, err := config.Load(s.configPath)
	if err != nil {
		return nil, err
	}

	saved := file.FlagValues()
	rows := make([]configRow, 0)
	rows = append(rows, readOnlyRow("MCP transport, fixed at launch", "transport", s.config.Transport))
	rows = append(rows, readOnlyRow("MCP HTTP port, fixed at launch", "port", strconv.Itoa(s.config.Port)))
	rows = append(rows, s.editableRow("turns kept per session (ring buffer)", config.KeyDepth, "number", strconv.Itoa(s.config.Depth), saved))
	rows = append(rows, readOnlyRow("Claude Code session root", "claude-home", s.config.ClaudeHome))
	rows = append(rows, readOnlyRow("Codex session root", "codex-home", s.config.CodexHome))
	rows = append(rows, s.editableRow("uncommitted-diff poll cadence", config.KeyPollInterval, "text", s.config.PollInterval, saved))
	rows = append(rows, s.editableRow("only poll repos active within this window", config.KeyPollWindow, "text", s.config.PollWindow, saved))
	rows = append(rows, readOnlyRow("diff/plan persistence root", "state-dir", s.config.StateDir))
	rows = append(rows, s.editableRow("days before idle session state is GCed (0 disables)", config.KeyStateRetentionDays, "number", strconv.Itoa(s.config.StateRetentionDays), saved))
	rows = append(rows, readOnlyRow("dashboard port, fixed at launch", "control-port", strconv.Itoa(s.config.ControlPort)))
	rows = append(rows, readOnlyRow("bearer token protecting this dashboard", "control-token", tokenDisplay(s.config.TokenSet)))
	rows = append(rows, s.editableRow("slog level", config.KeyLogLevel, "enum", s.config.LogLevel, saved, "debug", "info", "warn", "error"))
	return rows, nil
}

func (s *Server) editableRow(explanation, key, rowType, runningValue string, savedValues map[string]string, values ...string) configRow {
	saved := savedValues[key]
	row := configRow{
		Editable:      true,
		Explanation:   explanation,
		Key:           key,
		Overridden:    s.overriddenKeys[key],
		RestartNeeded: saved != "" && saved != runningValue,
		SavedValue:    saved,
		Type:          rowType,
		Value:         runningValue,
		Values:        values,
	}
	if saved != "" {
		row.Value = saved
	}
	return row
}

func (s *Server) handleConfigFragment(w http.ResponseWriter, r *http.Request) {
	rows, err := s.configRows()
	if err != nil {
		respondInternalServerError(err, w)
		return
	}

	data := configData{RestartAvailable: s.restart != nil, Rows: rows}
	s.renderFragment(w, tmplConfig, data)
}

func (s *Server) handleConfigSet(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	file, err := config.Load(s.configPath)
	if err != nil {
		respondInternalServerError(err, w)
		return
	}

	if err := file.Set(key, r.FormValue("value")); err != nil {
		respondBadRequest(err.Error(), w)
		return
	}

	if err := config.Save(s.configPath, file); err != nil {
		respondInternalServerError(err, w)
		return
	}

	w.Header().Set("HX-Trigger", "config-op")
	s.renderConfigRow(key, w)
}

func (s *Server) renderConfigRow(key string, w http.ResponseWriter) {
	rows, err := s.configRows()
	if err != nil {
		respondInternalServerError(err, w)
		return
	}

	for _, row := range rows {
		if row.Key == key {
			s.renderFragment(w, tmplConfigRow, row)
			return
		}
	}

	respondNotFound("unknown config key", w)
}

func readOnlyRow(explanation, key, value string) configRow {
	return configRow{Explanation: explanation, Key: key, Value: value}
}

func tokenDisplay(isSet bool) string {
	if isSet {
		return "set"
	}
	return "not set"
}
```

- Validation gate: unknown/non-editable keys rejected by `File.Set` → 400 → error banner; the template's key list is not a security boundary.
- Fresh `config.Load` per render → cross-instance edits visible on next fragment load ([C4](#d-c4)).

**2.5 Config templates** (new)
location: `control/templates/_config.html`, `control/templates/_config_row.html`
mirrors: claude-configs `templates/_config_row.html`; no `{{define}}` wrapper ([X1](#d-x1))

`_config.html`:

```html
{{range .Rows}}{{template "_config_row.html" .}}{{end}}
{{if .RestartAvailable}}<button hx-post="/api/restart" hx-confirm="Restart peek-mcp?">Restart</button>{{end}}
```

`_config_row.html`:

```html
<div class="card card-config" id="config-{{.Key}}">
  <span class="label label-key">
    <strong>{{.Key}}</strong>
    <br><span class="meta explanation">{{.Explanation}}</span>
  </span>
  {{if .RestartNeeded}}<span class="badge badge-action" title="saved value {{.SavedValue}} takes effect after restart">restart required</span>{{end}}
  {{if .Overridden}}<span class="badge badge-dim" title="a flag or PEEK_* env var pins this key; the config file value is ignored at startup">overridden</span>{{end}}
  {{if .Editable}}
    {{if eq .Type "enum"}}
    <select name="value"
            hx-post="/api/config/{{.Key}}"
            hx-target="#config-{{.Key}}" hx-swap="outerHTML">
      {{range .Values}}<option {{if eq . $.Value}}selected{{end}}>{{.}}</option>{{end}}
    </select>
    {{else}}
    <form class="inline-form" hx-post="/api/config/{{.Key}}"
          hx-target="#config-{{.Key}}" hx-swap="outerHTML">
      <input type="{{if eq .Type "number"}}number{{else}}text{{end}}" name="value" value="{{.Value}}">
      <button type="submit" class="small">Save</button>
    </form>
    {{end}}
  {{else}}
  <span class="meta">{{.Value}}</span>
  {{end}}
</div>
```

- All styling pre-exists in peek's stylesheet — zero CSS edits in this phase.

**2.6 Stats page restructure** (modified)
location: [control/templates/_stats.html](control/templates/_stats.html), [control/templates/stats.html](control/templates/stats.html)

```diff
 <div hx-get="/fragments/stats" hx-trigger="every 10s" hx-swap="outerHTML">
-<table>
+<table class="evidence-table">
   <tr><th>PID</th><td>{{.PID}}</td></tr>
   <!-- ... runtime rows incl. the Phase-1 telemetry export row ... -->
 </table>
 {{if .Invocations}}
 <h2>Tool invocations</h2>
-<table>
+<table class="evidence-table">
   {{range $tool, $count := .Invocations}}<tr><th>{{$tool}}</th><td>{{$count}}</td></tr>{{end}}
 </table>
 {{end}}
-<h2>Config</h2>
-<table>
-  <tr><th>Transport</th><td>{{.Config.Transport}}</td></tr>
-  <tr><th>Port</th><td>{{.Config.Port}}</td></tr>
-  <tr><th>Depth</th><td>{{.Config.Depth}}</td></tr>
-  <tr><th>Claude home</th><td>{{.Config.ClaudeHome}}</td></tr>
-  <tr><th>Codex home</th><td>{{.Config.CodexHome}}</td></tr>
-  <tr><th>Poll interval</th><td>{{.Config.PollInterval}}</td></tr>
-  <tr><th>Poll window</th><td>{{.Config.PollWindow}}</td></tr>
-  <tr><th>State retention</th><td>{{.Config.StateRetentionDays}} days</td></tr>
-  <tr><th>Control port</th><td>{{.Config.ControlPort}}</td></tr>
-  <tr><th>Token</th><td>{{if .Config.TokenSet}}set{{else}}not set{{end}}</td></tr>
-  <tr><th>Log level</th><td>{{.Config.LogLevel}}</td></tr>
-</table>
-{{if .RestartAvailable}}<button hx-post="/api/restart" hx-confirm="Restart peek-mcp?">Restart</button>{{end}}
 </div>
```

```diff
 {{template "head" .}}
 {{template "nav" .}}
 <h1>Peek</h1>
 <div hx-get="/fragments/stats" hx-trigger="load, every 10s" hx-swap="outerHTML">
   <div class="empty">Loading…</div>
 </div>
+<h2>Config</h2>
+<div hx-get="/fragments/config" hx-trigger="load, config-op from:body" hx-swap="innerHTML">
+  <div class="empty">Loading…</div>
+</div>
 {{template "foot" .}}
```

- Why the split: the 10s outerHTML swap would clobber in-progress edits ([C5](#d-c5)); config refreshes only on load and after any write.
- Restart button moves into the config fragment; `GET /api/stats` JSON keeps `restart_available`.

**2.7 README config-file section** (modified)
location: [README.md](README.md)

- One short section under the flags docs: path `~/.peek/config.json`, key list, precedence `flag > PEEK_* env > config file > default`, edits apply on restart, shared by all instances. (One-line doc edit class.)

---

### Phase 3 — interactive usage stats ([DR3](#dr3))

**3.1 Claude parser captures denied command** (modified)
location: [claude/parser.go](claude/parser.go)
mirrors: [codex/parser.go:264-268](codex/parser.go) (PermissionPayload.Command fill)

```go
// deniedToolInput covers the primary-input fields of tools whose denial
// detail we surface: Bash-style commands and file tools.
type deniedToolInput struct {
	Command      string `json:"command"`
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
}

func commandFromInput(pending *pendingToolUse) string {
	var input deniedToolInput
	if err := json.Unmarshal(pending.input, &input); err != nil {
		return ""
	}
	if input.Command != "" {
		return input.Command
	}
	if input.FilePath != "" {
		return input.FilePath
	}
	return input.NotebookPath
}
```

Note: per the repo's no-comment rule the `deniedToolInput` doc comment is dropped at implementation unless Kevin wants it.

```diff
-func permissionDeniedEvent(entry *Entry, tool string) *session.Event {
-	payload := &session.PermissionPayload{Tool: tool}
+func permissionDeniedEvent(entry *Entry, tool string, command string) *session.Event {
+	payload := &session.PermissionPayload{Command: command, Tool: tool}
 	return &session.Event{
 		Actor:      entry.AgentId,
 		Kind:       session.EventKindPermissionDenied,
```

Call-site sweep (3 sites):

```diff
 func subagentResultEvent(block *ContentBlock, entry *Entry, isDenied bool, text string) *session.Event {
 	if isDenied {
-		return permissionDeniedEvent(entry, toolNameAgent)
+		return permissionDeniedEvent(entry, toolNameAgent, "")
 	}
```

```diff
 	default:
 		if !isDenied {
 			return nil
 		}
-		return permissionDeniedEvent(entry, pending.name)
+		return permissionDeniedEvent(entry, pending.name, commandFromInput(pending))
```

```diff
 func userAnswerEvent(entry *Entry, isDenied bool, pending *pendingToolUse, text string) *session.Event {
 	if isDenied {
-		return permissionDeniedEvent(entry, toolNameAskUserQuestion)
+		return permissionDeniedEvent(entry, toolNameAskUserQuestion, "")
 	}
```

**3.2 Pricing package** (new — [P1](#d-p1) option a)
location: `pricing/pricing.go`
mirrors: none (first pricing code — full unit inlined for approval)

```go
package pricing

import "strings"

type Rates struct {
	InputPerMTok      float64
	OutputPerMTok     float64
	CacheWritePerMTok float64
	CacheReadPerMTok  float64
}

var rateTable = map[string]Rates{
	"claude-fable-5":  {InputPerMTok: 5, OutputPerMTok: 25, CacheWritePerMTok: 6.25, CacheReadPerMTok: 0.50},
	"claude-opus-4":   {InputPerMTok: 15, OutputPerMTok: 75, CacheWritePerMTok: 18.75, CacheReadPerMTok: 1.50},
	"claude-sonnet-4": {InputPerMTok: 3, OutputPerMTok: 15, CacheWritePerMTok: 3.75, CacheReadPerMTok: 0.30},
	"claude-haiku-4":  {InputPerMTok: 1, OutputPerMTok: 5, CacheWritePerMTok: 1.25, CacheReadPerMTok: 0.10},
	"gpt-5.1":         {InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
	"gpt-5-codex":     {InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
	"gpt-5-mini":      {InputPerMTok: 0.25, OutputPerMTok: 2, CacheReadPerMTok: 0.025},
	"gpt-5-nano":      {InputPerMTok: 0.05, OutputPerMTok: 0.40, CacheReadPerMTok: 0.005},
	"gpt-5":           {InputPerMTok: 1.25, OutputPerMTok: 10, CacheReadPerMTok: 0.125},
}

func Lookup(model string) (Rates, bool) {
	var bestRates Rates
	bestLength := -1
	for prefix, rates := range rateTable {
		if !strings.HasPrefix(model, prefix) {
			continue
		}
		if len(prefix) > bestLength {
			bestLength = len(prefix)
			bestRates = rates
		}
	}
	return bestRates, bestLength >= 0
}

func Cost(tokens int, ratePerMTok float64) float64 {
	return float64(tokens) / 1e6 * ratePerMTok
}
```

- Rate values are best-effort as of planning date — a verification item covers confirming them against vendor pricing pages before merge.
- OpenAI rows carry `CacheWritePerMTok: 0` — the Codex formula never uses the term.

**3.3 Usage view model + render-time totals** (modified)
location: [control/sessions.go](control/sessions.go)

```diff
 type usageData struct {
-	Id       session.Id
-	Counters session.Counters
-	Usage    session.Usage
+	Id           session.Id
+	Counters     session.Counters
+	Usage        session.Usage
+	TotalTokens  int
+	CachePercent string
+	PlanVersions int
 }
```

```diff
 func (s *Server) handleUsageFragment(w http.ResponseWriter, r *http.Request) {
 	id := session.Id(r.PathValue("id"))
 	data := usageData{Id: id}
 	if !s.store.WithSession(id, func(sess *session.Session) {
 		data.Counters = sess.Counters
 		data.Usage = *sess.CurrentUsage()
+		data.TotalTokens = displayTotalTokens(&data.Usage)
+		data.CachePercent = cachePercent(sess.Agent, &data.Usage)
+		data.PlanVersions = len(sess.PlanRevisions)
 	}) {
 		respondNotFound("unknown session", w)
 		return
 	}
 	s.renderFragment(w, tmplUsage, data)
 }
```

```diff
 	tmplUsage         = "_usage.html"
+	tmplUsageCost     = "_usage_cost.html"
+	tmplUsageDenials  = "_usage_denials.html"
+	tmplUsagePlans    = "_usage_plans.html"
+	tmplUsageSkills   = "_usage_skills.html"
 	tmplEvents        = "_events.html"
```

**3.4 Detail fragment handlers + helpers** (new)
location: `control/usage.go`
mirrors: [control/sessions.go](control/sessions.go) `handleUsageFragment`/`handleEventsFragment`

```go
package control

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/kevinhorst/peek-mcp/pricing"
	"github.com/kevinhorst/peek-mcp/session"
)

func displayTotalTokens(usage *session.Usage) int {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	return usage.InputTokens + usage.OutputTokens +
		usage.CacheCreationInputTokens + usage.CacheReadInputTokens
}

func cachePercent(agent session.Agent, usage *session.Usage) string {
	hit := usage.CacheReadInputTokens
	base := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	if agent == session.AgentCodex {
		hit = usage.CachedInputTokens
		base = usage.InputTokens
	}
	if base == 0 {
		return ""
	}
	return fmt.Sprintf("%.0f%%", float64(hit)/float64(base)*100)
}

type costRow struct {
	Component string
	Tokens    int
	Rate      string
	Cost      string
}

type costData struct {
	Id    session.Id
	Model string
	Known bool
	Rows  []costRow
	Total string
}

func newCostRow(component string, tokens int, ratePerMTok float64, total *float64) costRow {
	cost := pricing.Cost(tokens, ratePerMTok)
	*total += cost
	return costRow{
		Component: component,
		Tokens:    tokens,
		Rate:      fmt.Sprintf("$%.2f", ratePerMTok),
		Cost:      fmt.Sprintf("$%.4f", cost),
	}
}

func newCostData(id session.Id, agent session.Agent, model string, usage *session.Usage) costData {
	data := costData{Id: id, Model: model}
	rates, known := pricing.Lookup(model)
	if !known {
		return data
	}
	data.Known = true

	var total float64
	if agent == session.AgentCodex {
		uncached := max(0, usage.InputTokens-usage.CachedInputTokens)
		data.Rows = []costRow{
			newCostRow("Input (uncached)", uncached, rates.InputPerMTok, &total),
			newCostRow("Cached input", usage.CachedInputTokens, rates.CacheReadPerMTok, &total),
			newCostRow("Output", usage.OutputTokens, rates.OutputPerMTok, &total),
		}
	} else {
		data.Rows = []costRow{
			newCostRow("Input", usage.InputTokens, rates.InputPerMTok, &total),
			newCostRow("Cache write", usage.CacheCreationInputTokens, rates.CacheWritePerMTok, &total),
			newCostRow("Cache read", usage.CacheReadInputTokens, rates.CacheReadPerMTok, &total),
			newCostRow("Output", usage.OutputTokens, rates.OutputPerMTok, &total),
		}
	}
	data.Total = fmt.Sprintf("$%.4f", total)
	return data
}

func (s *Server) handleUsageCostFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	var data costData
	if !s.store.WithSession(id, func(sess *session.Session) {
		data = newCostData(id, sess.Agent, sess.Meta.Model, sess.CurrentUsage())
	}) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplUsageCost, data)
}

type planVersionRow struct {
	Index      int
	Timestamp  time.Time
	Alteration bool
}

type planVersionsData struct {
	Id       session.Id
	Versions []planVersionRow
}

func (s *Server) handleUsagePlansFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	data := planVersionsData{Id: id}
	if !s.store.WithSession(id, func(sess *session.Session) {
		for _, revision := range sess.PlanRevisions {
			data.Versions = append(data.Versions, planVersionRow{
				Index:      revision.Index,
				Timestamp:  revision.Timestamp,
				Alteration: revision.IsAlteration,
			})
		}
	}) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplUsagePlans, data)
}

type skillRow struct {
	Skill     string
	StartedAt time.Time
	Duration  string
	Tokens    int
}

type skillsData struct {
	Id     session.Id
	Skills []skillRow
}

func (s *Server) handleUsageSkillsFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	data := skillsData{Id: id}
	if !s.store.WithSession(id, func(sess *session.Session) {
		for _, skill := range sess.Skills {
			duration := "running"
			if !skill.EndedAt.IsZero() {
				duration = skill.EndedAt.Sub(skill.StartedAt).Round(time.Second).String()
			}
			data.Skills = append(data.Skills, skillRow{
				Skill:     skill.Skill,
				StartedAt: skill.StartedAt,
				Duration:  duration,
				Tokens:    displayTotalTokens(&skill.Usage),
			})
		}
	}) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplUsageSkills, data)
}

type denialRow struct {
	Tool      string
	Command   string
	Timestamp time.Time
}

type denialsData struct {
	Id      session.Id
	Denials []denialRow
}

func (s *Server) handleUsageDenialsFragment(w http.ResponseWriter, r *http.Request) {
	id := session.Id(r.PathValue("id"))
	data := denialsData{Id: id}
	if !s.store.WithSession(id, func(sess *session.Session) {
		all := sess.Events.All()
		slices.Reverse(all)
		for _, event := range all {
			if event.Kind != session.EventKindPermissionDenied || event.Permission == nil {
				continue
			}
			data.Denials = append(data.Denials, denialRow{
				Tool:      event.Permission.Tool,
				Command:   event.Permission.Command,
				Timestamp: event.Timestamp,
			})
		}
	}) {
		respondNotFound("unknown session", w)
		return
	}
	s.renderFragment(w, tmplUsageDenials, data)
}
```

**3.5 Routes** (modified)
location: [control/server.go](control/server.go)

```diff
 	mux.HandleFunc("GET /fragments/sessions/{id}/usage", s.handleUsageFragment)
+	mux.HandleFunc("GET /fragments/sessions/{id}/usage/cost", s.handleUsageCostFragment)
+	mux.HandleFunc("GET /fragments/sessions/{id}/usage/plans", s.handleUsagePlansFragment)
+	mux.HandleFunc("GET /fragments/sessions/{id}/usage/skills", s.handleUsageSkillsFragment)
+	mux.HandleFunc("GET /fragments/sessions/{id}/usage/denials", s.handleUsageDenialsFragment)
 	mux.HandleFunc("GET /fragments/sessions/{id}/events", s.handleEventsFragment)
```

**3.6 Usage template rewrite** (modified)
location: [control/templates/_usage.html](control/templates/_usage.html)
mirrors: existing self-refresh wrapper; th/td one-line adjacency (pinned)

```html
<div hx-get="/fragments/sessions/{{.Id}}/usage" hx-trigger="peek-refresh from:body throttle:1s" hx-swap="outerHTML">
<div class="usage-grid">
<table class="usage-table">
  <tr><th>Input tokens</th><td>{{.Usage.InputTokens}}</td></tr>
  <tr><th>Output tokens</th><td>{{.Usage.OutputTokens}}</td></tr>
  {{if .Usage.CacheCreationInputTokens}}<tr><th>Cache creation</th><td>{{.Usage.CacheCreationInputTokens}}</td></tr>{{end}}
  {{if .Usage.CacheReadInputTokens}}<tr><th>Cache read</th><td>{{.Usage.CacheReadInputTokens}}</td></tr>{{end}}
  {{if .Usage.CachedInputTokens}}<tr><th>Cached input</th><td>{{.Usage.CachedInputTokens}}</td></tr>{{end}}
  {{if .Usage.ReasoningOutputTokens}}<tr><th>Reasoning output</th><td>{{.Usage.ReasoningOutputTokens}}</td></tr>{{end}}
  {{if .CachePercent}}<tr><th>Cache hit</th><td>{{.CachePercent}}</td></tr>{{end}}
  <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage/cost" hx-target="#usage-detail-{{.Id}}"><th>Total tokens</th><td>{{.TotalTokens}}</td></tr>
  <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage/denials" hx-target="#usage-detail-{{.Id}}"><th>Permission denials</th><td>{{.Counters.PermissionDenials}}</td></tr>
  <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage/plans" hx-target="#usage-detail-{{.Id}}"><th>Plan versions</th><td>{{.PlanVersions}}</td></tr>
  <tr><th>Plan rejections</th><td>{{.Counters.PlanRejections}}</td></tr>
  <tr class="usage-row" hx-get="/fragments/sessions/{{.Id}}/usage/skills" hx-target="#usage-detail-{{.Id}}"><th>Skills invoked</th><td>{{.Counters.SkillsInvoked}}</td></tr>
  <tr><th>Subagents spawned</th><td>{{.Counters.SubagentsSpawned}}</td></tr>
</table>
<div id="usage-detail-{{.Id}}" class="usage-detail" hx-preserve></div>
</div>
</div>
```

**3.7 Detail templates** (new)
location: `control/templates/_usage_cost.html`, `_usage_plans.html`, `_usage_skills.html`, `_usage_denials.html`
mirrors: [control/templates/_events.html](control/templates/_events.html) (self-refresh wrapper, `.empty` fallback)

`_usage_cost.html`:

```html
<div hx-get="/fragments/sessions/{{.Id}}/usage/cost" hx-trigger="peek-refresh from:body throttle:1s" hx-swap="outerHTML">
{{if .Known}}
<table class="usage-table">
  <tr><th>Component</th><th>Tokens</th><th>Rate/MTok</th><th>Cost</th></tr>
  {{range .Rows}}<tr><th>{{.Component}}</th><td>{{.Tokens}}</td><td>{{.Rate}}</td><td>{{.Cost}}</td></tr>
  {{end}}<tr><th>Total</th><td></td><td></td><td>{{.Total}}</td></tr>
</table>
<div class="meta">Estimate from embedded rates for {{.Model}}.</div>
{{else}}
<div class="empty">No pricing for model {{if .Model}}{{.Model}}{{else}}(unknown){{end}}.</div>
{{end}}
</div>
```

`_usage_plans.html`:

```html
<div hx-get="/fragments/sessions/{{.Id}}/usage/plans" hx-trigger="peek-refresh from:body throttle:1s" hx-swap="outerHTML">
{{if .Versions}}
<table class="usage-table">
  <tr><th>Revision</th><th>Timestamp</th><th>Alteration</th></tr>
  {{range .Versions}}<tr><th>{{.Index}}</th><td>{{ts .Timestamp}}</td><td>{{if .Alteration}}yes{{end}}</td></tr>
  {{end}}
</table>
{{else}}
<div class="empty">No plan versions yet.</div>
{{end}}
</div>
```

`_usage_skills.html`:

```html
<div hx-get="/fragments/sessions/{{.Id}}/usage/skills" hx-trigger="peek-refresh from:body throttle:1s" hx-swap="outerHTML">
{{if .Skills}}
<table class="usage-table">
  <tr><th>Skill</th><th>Started</th><th>Duration</th><th>Tokens</th></tr>
  {{range .Skills}}<tr><th>{{.Skill}}</th><td>{{ts .StartedAt}}</td><td>{{.Duration}}</td><td>{{.Tokens}}</td></tr>
  {{end}}
</table>
{{else}}
<div class="empty">No skills invoked yet.</div>
{{end}}
</div>
```

`_usage_denials.html`:

```html
<div hx-get="/fragments/sessions/{{.Id}}/usage/denials" hx-trigger="peek-refresh from:body throttle:1s" hx-swap="outerHTML">
{{if .Denials}}
<table class="usage-table">
  <tr><th>Tool</th><th>Command</th><th>Timestamp</th></tr>
  {{range .Denials}}<tr><th>{{.Tool}}</th><td>{{.Command}}</td><td>{{ts .Timestamp}}</td></tr>
  {{end}}
</table>
<div class="meta">From the event buffer (capped).</div>
{{else}}
<div class="empty">No permission denials.</div>
{{end}}
</div>
```

**3.8 Styles** (modified)
location: [control/assets/style.css](control/assets/style.css)
mirrors: `.md-body table` values

```css
.usage-grid { display: flex; gap: 16px; align-items: flex-start; }
.usage-grid > .usage-detail { flex: 1; min-width: 0; }
.usage-table { border-collapse: collapse; font-size: 0.9375rem; }
.usage-table th, .usage-table td { border: 1px solid var(--line); padding: 4px 10px; text-align: left; }
.usage-table th { color: var(--text-2); font-weight: 600; }
tr.usage-row { cursor: pointer; }
tr.usage-row:hover th, tr.usage-row:hover td { color: var(--neon); }
```

## Hot items

- **H1 — validation logic (hot-items #5):** every `config.File.set*` method is new guard logic gating the shared config file; `handleConfigSet` relies on it exclusively. Complete bodies in [§2.1](#phase-2--global-editable-config-dr2) for approval; durations normalized via `Duration.String()` so saved-vs-running comparison is stable. No existing validation weakened.
- **H2 — cross-process writes without locking (hot-items #2):** `config.Save` is read-modify-write on a shared file, deliberately lock-free ([C6](#d-c6)); complete implementation in §2.1, byte-for-byte the `state.Dir.writeFile` mechanism. Readers never see a torn file; concurrent saves race whole-file, last-writer-wins. No new goroutines, channels, or mutexes anywhere in this plan.
- **Not triggered:** no SQL/CTEs, no new interfaces or generics, no migrations, no anonymous structs (all view/row types named; examples in §3.1/§3.4).

## Tests

| Location.Method | Cases | Comment |
|---|---|---|
| telemetry/detect_test.go TestDetector_Status | missing-file-not-configured<br>invalid-json-not-configured<br>no-env-block-not-configured<br>enabled-zero-not-configured<br>exact-setup-output-configured<br>localhost-endpoint-configured<br>trailing-slash-endpoint-configured<br>grpc-protocol-misconfigured<br>wrong-port-misconfigured<br>missing-endpoint-misconfigured<br>numeric-env-value-tolerated | table-driven; settings.json into t.TempDir(); detail assertions on misconfigured |
| tools/viewmodels_time_test.go TestNewTelemetryTimeView | receiving-overrides-configured<br>configured-without-data<br>misconfigured-detail-passthrough<br>codex-session-nil<br>nil-store-nil<br>nil-detector-nil | pins the previously untested session_events emission |
| cmd/setup_test.go TestSetupTelemetry | writes-all-env-keys-with-interval<br>derives-port-from-control-port-base<br>deletes-stale-headers-key<br>control-server-disabled-writes-nothing<br>decline-writes-nothing | pins the previously untested setupTelemetry keys; scripted prompter, t.Setenv HOME |
| control/api_test.go TestHandleStats_TelemetryExport | detector-set-includes-status<br>detector-nil-omits-field | mirrors newTestServer usage |
| config/file_test.go TestLoad | missing-file-returns-empty<br>invalid-json-errors<br>valid-file-roundtrip | t.TempDir |
| config/file_test.go TestFile_Set | depth-valid<br>depth-not-a-number<br>depth-out-of-range<br>poll-interval-valid-normalized<br>poll-interval-below-minimum<br>poll-window-valid<br>log-level-valid<br>log-level-unknown<br>state-retention-zero-valid<br>state-retention-negative<br>unknown-key<br>non-editable-key-transport | table-driven |
| config/file_test.go TestSave | roundtrip-load-equals-saved<br>creates-parent-dir | assert perms 0600 |
| cmd/start_test.go TestApplyConfigFileFallbacks | flag-beats-file<br>env-marked-changed-beats-file<br>file-beats-default<br>empty-file-keeps-defaults | throwaway cobra.Command with the 5 flags |
| control/config_test.go TestConfigFragment | renders-card-config-rows<br>editable-input-for-depth<br>read-only-transport-no-form<br>restart-required-badge-on-drift<br>overridden-badge<br>no-restart-button-without-closure | ConfigPath in t.TempDir |
| control/config_test.go TestConfigSet | valid-depth-writes-file-and-triggers<br>invalid-value-400<br>unknown-key-400<br>non-editable-key-400 | new `post` helper mirroring `get` |
| pricing/pricing_test.go TestLookup | exact-match<br>longest-prefix-wins<br>dated-model-suffix<br>unknown-model | |
| pricing/pricing_test.go TestCost | zero-tokens<br>one-mtok-equals-rate | |
| control/usage_test.go TestDisplayTotalTokens | codex-total-preferred<br>claude-summed | |
| control/usage_test.go TestCachePercent | claude-read-share<br>codex-cached-share<br>empty-base-blank | |
| control/pages_test.go TestUsageFragment (extend) | total-tokens-computed (s1 expect 15)<br>plan-versions-label<br>zero-rows-hidden (no Cached input)<br>existing assertions kept | |
| control/usage_test.go TestUsageCostFragment | known-model-total<br>unknown-model-no-pricing<br>not-found-404 | fixture model "opus" gives the unknown path free |
| control/usage_test.go TestUsagePlansFragment | rows-rendered<br>empty-state | s1 has a plan revision |
| control/usage_test.go TestUsageSkillsFragment | rows-rendered<br>empty-state | |
| control/usage_test.go TestUsageDenialsFragment | rows-with-command<br>empty-state | |
| claude/parser_events_test.go TestParseLine_PermissionDenied (extend) | bash-denied-command-captured<br>file-tool-denied-path-captured<br>agent-denied-command-empty | mirrors existing denial cases |
| control/pages_test.go TestStatsFragment/TestStatsPage (adjust) | config-table-absent<br>evidence-table-class-present<br>config-fragment-link-present | old `<th>Transport</th>` assertion moves to the config fragment test |

## Test runbook

Tool: curl JSON-RPC against `make serve-http` (`/mcp`) for MCP surfaces, plain curl for the dashboard — same format as [plans/deep_analysis/runbooks/](plans/deep_analysis/runbooks/). Files persist to `plans/control_server/runbooks/` at implementation.

- **telemetry_status.sh** — location: `plans/control_server/runbooks/telemetry_status.sh`

```bash
#!/bin/sh
# session_events telemetry status on a Claude session (replace SESSION_ID)
curl -s http://127.0.0.1:4242/mcp -H 'Content-Type: application/json' -d '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "session_events",
    "arguments": {
      "id": "'"${SESSION_ID}"'",
      "json": true
    }
  }
}' | jq '.result.structuredContent.time.telemetry'
```

- **stats_telemetry.sh** — location: `plans/control_server/runbooks/stats_telemetry.sh`

```bash
#!/bin/sh
# /api/stats telemetry_export + bound control port
curl -s "http://127.0.0.1:${CONTROL_PORT:-42442}/api/stats" | jq '{telemetry_export, control_port: .config.control_port}'
```

- **config_set.sh** — location: `plans/control_server/runbooks/config_set.sh`

```bash
#!/bin/sh
# set depth via the config API, then show the file and the fragment badge
curl -s -X POST "http://127.0.0.1:${CONTROL_PORT:-42442}/api/config/depth" -d 'value=50' -o /dev/null -w '%{http_code}\n'
jq . ~/.peek/config.json
curl -s "http://127.0.0.1:${CONTROL_PORT:-42442}/fragments/config" | grep -o 'restart required' | head -1
```

- **usage_details.sh** — location: `plans/control_server/runbooks/usage_details.sh`

```bash
#!/bin/sh
# usage fragment + all four detail fragments for one session (replace SESSION_ID)
base="http://127.0.0.1:${CONTROL_PORT:-42442}/fragments/sessions/${SESSION_ID}/usage"
curl -s "$base" | grep -E 'Total tokens|Cache hit|Plan versions'
for detail in cost plans skills denials; do
  echo "== $detail =="
  curl -s "$base/$detail"
done
```

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| `tools.Register` signature (+detector) | cmd/start.go ↔ tools/tools.go ↔ tools tests | compiler-driven; grep `tools.Register(` to zero stale arity |
| `sessionEventsHandler` signature | tools/tools.go internal + tests | compiler-driven |
| `control.Options` (+Detector, +ConfigPath, +OverriddenKeys) | cmd/start.go ↔ control ↔ control tests (`newTestServer`) | compiler-driven; grep `control.Options{` |
| `permissionDeniedEvent` signature | claude/parser.go 3 call sites | grep `permissionDeniedEvent(` to zero 2-arg calls |
| flag names = config keys = `PEEK_*` env names | config.EditableKeys ↔ cmd/start.go flags ↔ envFallbacks | grep `envFallbacks` keys vs `Key*` consts — each editable key present in both |
| `GET /api/stats` JSON (additive `telemetry_export` only; `config`/`restart_available` unchanged) | control ↔ docs/reference.md | grep docs for `/api/stats` fields |
| `/api/sessions/{id}/usage` unchanged | control/api.go ↔ control/api_test.go:168 | test passes untouched |
| stats fragment markup (config rows removed, telemetry row added) | templates ↔ control/pages_test.go | grep tests for `<th>Transport`, `Restart` assertions |
| `session_events` JSON: `time.telemetry` gains status/detail, existing fields keep meaning when receiving | tools ↔ README tool docs | grep README/docs for `telemetry` field description; update |
| settings.json env keys (setup writer ↔ detector reader) | cmd/setup.go ↔ telemetry/detect.go | shared expectation pinned by exact-setup-output-configured test case |

## Verification

Phase 1:
- [ ] Run `go build ./... && go vet ./... && go test ./...` — all green.
- [ ] Run `peek-mcp setup` with control server accepted — exactly one telemetry question; settings.json env gains the 5 keys incl. `OTEL_METRIC_EXPORT_INTERVAL=10000`, no `OTEL_EXPORTER_OTLP_HEADERS`.
- [ ] Run setup declining the control server — no telemetry question, only the "stays disabled" line, settings.json unchanged.
- [ ] Start peek — one startup line `telemetry export status=configured`; /stats runtime table shows "Telemetry export: configured"; Control port row shows the bound port.
- [ ] Occupy 42442 (`nc -l 42442`), start peek — bound port 42443 in log and /stats; export shows `misconfigured` with want-vs-got naming 42443.
- [ ] Edit settings.json to `grpc` protocol while peek runs, reload /stats — `misconfigured` without restart (lazy re-read).
- [ ] Remove telemetry env keys — `not_configured` with the shell-env wording.
- [ ] `session_events` on a Claude session before metrics arrive — `time.telemetry.status` present (no omission); within ~10s of activity → `receiving` with `active_seconds`.
- [ ] `session_events` on a Codex session — no `time.telemetry`, `telemetry` still under `unsupported`.
- [ ] Start with `--control-port=0` — no telemetry line, `time` block without telemetry field (degenerate).

Phase 2:
- [ ] Run `go test ./config ./cmd ./control` — all pass.
- [ ] Instance A `/stats`: styled card rows, padded runtime tables (no "Control port42442"), no badges without a config file (degenerate).
- [ ] Edit depth to 50 — row re-renders with "restart required"; `jq . ~/.peek/config.json` shows `"depth": 50`.
- [ ] Start instance B (port walks to 42443) — its config fragment shows depth 50 + "restart required" (ripple via shared file).
- [ ] Restart A — depth 50 running, badge gone.
- [ ] Enter `depth=abc` — error banner with validation message; file unchanged.
- [ ] `PEEK_DEPTH=30 peek-mcp start` — "overridden" badge; file edit + restart keeps 30 (env wins).
- [ ] log-level select → debug — save + restart-required; after restart debug logs appear.
- [ ] Corrupt config.json, start — startup warning, server runs on defaults (degenerate).
- [ ] stdio instance with control port — rows editable, no restart button.

Phase 3:
- [ ] Verify embedded rates in `pricing/pricing.go` against current Anthropic/OpenAI pricing pages before merge — update values, not structure.
- [ ] Live Claude session detail — Total tokens > 0, Cache hit percentage shown; Cached input/Reasoning output rows absent.
- [ ] Codex session detail — parser Total kept; Cache creation/read rows absent; Cached input shown.
- [ ] Click Total tokens — cost table with components + Total + "Estimate" note; unknown model → "No pricing for model …".
- [ ] Click Plan versions / Skills invoked / Permission denials — each detail table renders; with SSE refreshes firing, the detail pane survives the 1s swap (hx-preserve) and its content updates.
- [ ] Deny a Bash call in a live Claude session — denial row shows the actual shell command.
- [ ] Degenerate: session with no skills/plans/denials — `.empty` messages, not empty tables.

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| 1 | An approved signature/contract can't hold as planned | stop and report — never improvise architecture mid-edit |
| 2 | Second failed fix on the same mechanism | stop, research the actual cause, redesign — no third band-aid |
| 3 | Missing prerequisite (generated code, running infra) | run the producing step; if infrastructure is down, ask |
| 4 | Discovered work materially exceeds approved scope | ask before continuing |
| 5 | Same kind of bug twice: in own diff → fix all in diff; pre-existing outside → report and ask | per stop-conditions.md #5 |
| 6 | Structural obstacle tempts a new abstraction (interface, DTO, wrapper) | stop and report; relocate instead |
| 7 | Mechanical transform (template rewrite, signature sweep) | diff result element-by-element vs source; fidelity loss → stop |
| 8 | Old and new structure would coexist beyond phasing (e.g. config rows both in _stats.html and the fragment) | stop and report; never leave a half-migration |
| 9 | A driver contradicts a `[USER]` decision in an originating plan | surface the conflict (D6 editable-config conflict surfaced and superseded in this plan's Context) |
| 10 | `hx-preserve` does not survive the outerHTML self-swap in the vendored htmx | stop and report — fallback (detail pane outside the fragment) needs a decision |
| 11 | `config.Save` fails on the shared file (permissions, read-only home) | surface the 500; never fall back to per-instance config files |
| 12 | A pinned test (`control/api_test.go` usage shape, `session/store_events_test.go`) needs modification to pass | stop — the plan promises they stay untouched |

## Open questions

- None — all decisions resolved.

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
| 2026-08-11 | intake feedback | setup step: one Y/n question, no port/token prompts, disabled-control notice ([T6](#d-t6)) |
| 2026-08-11 | Q: pricing scope | [P1](#d-p1) → `[USER]` minimal embedded slice |
| 2026-08-11 | Q: editable keys | [C2](#d-c2) → `[USER]` the 5 safe keys |
| 2026-08-11 | local: merged main (57720f5) | re-verified anchors: tools/session line shifts only; `CurrentUsage` no longer re-adds the active turn (contract for §3.3 unchanged); runbook `session_events` call gains `"json": true` (structuredContent is now opt-in) |
