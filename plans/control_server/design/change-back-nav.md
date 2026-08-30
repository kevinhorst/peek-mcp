# Peek back-navigation to the smine config server — Change Plan

route: `change`

## TLDR

- The smine config server (claude-configs, `127.0.0.1:6001`) links to the peek dashboard; peek links nowhere back.
- Add an optional `back-link` value to peek — full three-tier config (flag `--back-link`, env `PEEK_BACK_LINK`, editable key in `~/.peek/config.json`) — rendered as one extra nav anchor when set.
- The config server passes `--back-link http://<its own -addr>/` when it spawns peek, so the round trip works with zero manual setup; externally started peeks can set the key once in the dashboard config editor.
- Two phases, each shippable: peek-mcp first, claude-configs second.

## Context

- The config server's nav links to peek (`internal/server/templates/layout.html:41` in claude-configs) and deep-links sessions; peek's nav ([layout.html:29-34](control/templates/layout.html:29)) has only Sessions/Stats — no way back.
- Terminology trap: in peek-mcp, "control server" means peek's own dashboard (`control/` package). The thing we link back to is the **smine config server** (`cmd/configserver` in claude-configs, launchd label `com.smine.configserver`). No new peek identifier may use the bare word "control" for the external target.
- The config server's port is configurable (`-addr`, default `127.0.0.1:6001`, install-time `CONFIGSERVER_PORT`), so peek must not hardcode it.
- Origin: smine proposal "Add Peek control server navigation back".

## Drivers

| ID | Observed | Wanted | Impact | Origin |
|----|----------|--------|--------|--------|
| DR1 | Config server nav links to peek; peek dashboard has no link back | Peek nav shows a link to the config server whenever peek knows its URL | behavioral (additive) | smine proposal / this request |

## Scope

- **In:**
  - **peek config value:** new `back-link` editable config key with flag + env fallback, threaded to the dashboard.
  - **peek nav:** conditional anchor in the shared nav template.
  - **peek config UI:** editable row on the /stats config editor.
  - **spawn wiring:** claude-configs `startPeek` passes `--back-link` derived from its own `-addr`.
  - **docs:** peek `docs/reference.md` flag/env/config tables; claude-configs README integration prose.
- **Out:**
  - **per-session deep links back:** the config server has no per-session detail pages to target.
  - **configurable link label:** single fixed label (see [D4](#decisions)).
  - **install-script changes:** no plist/task changes — the value is derived at runtime from `-addr`.
- **Not changed:**
  - **existing nav entries, pages, JSON API, OTLP:** untouched.
- **Deferred findings:** none.

## Assumptions

| Assumption | Reality | Location |
|------------|---------|----------|
| Peek is normally spawned by the config server, so a spawn-time flag reaches it | True for the launchd path (`-peek-start` default true); an externally started peek keeps running and gets no flag — covered by the editable key tier | claude-configs [main.go:119-121](/Users/kevinpersonal/GolandProjects/claude-configs/cmd/configserver/main.go), [main.go:205-215](/Users/kevinpersonal/GolandProjects/claude-configs/cmd/configserver/main.go) |
| The config server knows its own reachable URL | It knows only its listen `-addr`; `http://<addr>/` is correct for the loopback default. Empty-host binds (require `-allow-remote`) yield a degraded link — accepted | claude-configs [main.go:28](/Users/kevinpersonal/GolandProjects/claude-configs/cmd/configserver/main.go), [main.go:79-85](/Users/kevinpersonal/GolandProjects/claude-configs/cmd/configserver/main.go) |

## Current state

| ID | Fact | Location |
|----|------|----------|
| C1! | Shared nav template renders Sessions/Stats from the page struct (`.Page`); all three pages invoke it | [layout.html:29-34](control/templates/layout.html:29) |
| C2! | Nav data comes from page structs `indexPage{Page, Title}` / `detailPage{Page, Title, Summary}` built in three handlers | [sessions.go:30-39](control/sessions.go:30), [sessions.go:75](control/sessions.go:75), [sessions.go:91](control/sessions.go:91), [stats.go:58](control/stats.go:58) |
| C3! | Editable-config mechanism: key const + `File` field + `set*` validator + `EditableKeys` + `FlagValues` in [config/file.go](config/file.go); precedence flag > env > file via `applyEnvFallbacks`/`applyConfigFileFallbacks`; UI row via `s.editableRow` | [file.go:14-57](config/file.go:14), [start.go:366-397](cmd/start.go:366), [config.go:42-51](control/config.go:42) |
| C4 | Every flag has a `PEEK_*` env fallback in `envFallbacks`; flag registration in `init()` | [start.go:332-345](cmd/start.go:332), [start.go:270-286](cmd/start.go:270) |
| C5 | Launch values reach the dashboard via `control.Config` (built only when `controlPort > 0`) | [start.go:188-201](cmd/start.go:188), [viewmodels.go:66-79](control/viewmodels.go:66) |
| C6! | `startPeek` spawns `peek-mcp start --transport http --port N [--control-port M]`; skips spawn when the port already serves | claude-configs [main.go:208-235](/Users/kevinpersonal/GolandProjects/claude-configs/cmd/configserver/main.go) |
| C7 | Flag/env/config docs live in three tables plus a config-file paragraph | [reference.md:14-49](docs/reference.md:14) |
| C8 | Nav anchors inherit existing styling (`nav a`, `.active`) — no CSS change needed | control/assets/style.css:120-125 |

## Target state

N/A — single additive value threaded through the existing config path; no structural change.

## Behavior contract

- With `back-link` unset (the default everywhere today): rendered pages are byte-identical to current output — no empty anchor, no placeholder.
- With `back-link` set: exactly one additional nav anchor on all three pages; nothing else changes.
- Config editor: invalid URL values are rejected with 400 and never persisted; flag/env-pinned values show the existing "overridden" badge.
- Config server spawn behavior unchanged apart from the extra argument; an already-serving peek is still left alone.

## Decisions

| ID | Problem | Facts | Decision | Why |
|----|---------|-------|----------|-----|
| <a id="d1"></a>D1 | How peek learns the config server URL | [C3](#current-state), [C6](#current-state) | Full editable config key `back-link` (flag `--back-link`, env `PEEK_BACK_LINK`, file field `back_link`) | One existing three-tier mechanism covers both launch paths: spawn-time flag (automatic, controllable) and dashboard-editable file value for externally started peeks. A flag-only value dies on the external-start path; a hardcoded 6001 breaks configurable installs |
| <a id="d2"></a>D2 | How the config server supplies the value | [C6](#current-state) | `startPeek` gains a `backLink` param; caller passes `"http://" + *addr + "/"`; appended as `--back-link` only when `controlPort != 0` | Derived from the flag the server already owns — single source of truth, no new configserver flag, no install-script change |
| <a id="d3"></a>D3 | How the value reaches the nav template | [C1](#current-state), [C2](#current-state), [C5](#current-state) | `BackLink` field on `indexPage` and `detailPage`, populated from `s.config.BackLink` in the three page handlers; conditional anchor in the `nav` define | Follows the in-repo pattern (nav consumes the page struct); peek's template funcs are pure formatters, config-reading funcs would be a new concept |
| <a id="d4"></a>D4 | Nav link label | — | Fixed text `Hub` | Peek is generic — `smine` would couple it to one consumer; `Home` reads as peek's own root. One-word change if vetoed at review |
| <a id="d5"></a>D5 | Validation of the value | [C3](#current-state) | Empty string clears the key (field set to nil); otherwise require absolute `http`/`https` URL with non-empty host | Mirrors sibling `set*` validators; empty-clears gives the editor a way to remove the link; scheme check keeps `href` sane (html/template additionally neutralizes unsafe URLs) |

## Open questions

None.

## Baseline (verified)

N/A — change route; facts live in Current state.

## Exemplar & reuse

| Existing | Used for |
|----------|----------|
| Three-tier config mechanism ([C3](#current-state)) | The whole `back-link` value path — no new plumbing invented |
| `setPollInterval` / `setLogLevel` validators | `setBackLink` shape |
| `editableRow` + config-row tests | Config UI surface |
| Config server's own nav link (`layout.html:41`, claude-configs) | Cross-repo sibling for the conditional nav anchor |

- Every change has an exemplar.

## Changes

### Phase 1 — peek-mcp (shippable alone: key exists, link renders when set)

#### 1. Config key (modified)

location: [config/file.go](config/file.go)

```diff
 const (
+	KeyBackLink           = "back-link"
 	KeyDepth              = "depth"
 	KeyLogLevel           = "log-level"
```

```diff
-var EditableKeys = []string{KeyDepth, KeyPollInterval, KeyPollWindow, KeyStateRetentionDays, KeyLogLevel}
+var EditableKeys = []string{KeyBackLink, KeyDepth, KeyPollInterval, KeyPollWindow, KeyStateRetentionDays, KeyLogLevel}
```

```diff
 type File struct {
+	BackLink           *string `json:"back_link,omitempty"`
 	Depth              *int    `json:"depth,omitempty"`
```

```diff
 func (f *File) Set(key, value string) error {
 	switch key {
+	case KeyBackLink:
+		return f.setBackLink(value)
 	case KeyDepth:
```

New validator (hot item — full code in [Hot items](#hot-items)); `FlagValues` gains:

```diff
 func (f *File) FlagValues() map[string]string {
 	values := make(map[string]string)
+	if f.BackLink != nil {
+		values[KeyBackLink] = *f.BackLink
+	}
 	if f.Depth != nil {
```

- Adds `net/url` import.

#### 2. Flag, env fallback, plumbing (modified)

location: [cmd/start.go](cmd/start.go)

```diff
 func init() {
 	flags := startCmd.Flags()
 	// ...
 	flags.String("control-token", "", "Optional bearer token protecting the control server")
+	flags.String("back-link", "", "URL of an external dashboard the control server nav links back to (empty hides the link)")
 	flags.String("log-level", "info", "Log level: debug, info, warn, error")
```

```diff
 var envFallbacks = map[string]string{
 	// ...
 	"control-token":        "PEEK_CONTROL_TOKEN",
+	"back-link":            "PEEK_BACK_LINK",
 	"log-level":            "PEEK_LOG_LEVEL",
 }
```

```diff
 		controlPort, _ := flags.GetInt("control-port")
 		controlToken, _ := flags.GetString("control-token")
+		backLink, _ := flags.GetString("back-link")
```

```diff
 				Config: control.Config{
 					// ...
 					ControlPort:        boundPort,
 					TokenSet:           controlToken != "",
 					LogLevel:           logLevel,
+					BackLink:           backLink,
 				},
```

#### 3. Dashboard config struct (modified)

location: [control/viewmodels.go:66](control/viewmodels.go:66)

```diff
 type Config struct {
 	// ...
 	TokenSet           bool   `json:"token_set"`
 	LogLevel           string `json:"log_level"`
+	BackLink           string `json:"back_link,omitempty"`
 }
```

#### 4. Config editor row (modified)

location: [control/config.go:50](control/config.go:50)

```diff
 	rows = append(rows, readOnlyRow("bearer token protecting this dashboard", "control-token", tokenDisplay(s.config.TokenSet)))
+	rows = append(rows, s.editableRow("URL the nav's Hub link points to (empty hides it)", config.KeyBackLink, "text", s.config.BackLink, saved))
 	rows = append(rows, s.editableRow("slog level", config.KeyLogLevel, "enum", s.config.LogLevel, saved, "debug", "info", "warn", "error"))
```

#### 5. Page structs and handlers (modified)

location: [control/sessions.go:30](control/sessions.go:30), [control/stats.go:58](control/stats.go:58)

```diff
 type indexPage struct {
 	Page  string
 	Title string
+	BackLink string
 }

 type detailPage struct {
 	Page    string
 	Title   string
 	Summary *sessionSummary
+	BackLink string
 }
```

```diff
 func (s *Server) handleSessionsPage(w http.ResponseWriter, r *http.Request) {
-	s.renderFragment(w, tmplSessionsIndex, indexPage{Page: pageSessions, Title: "Peek"})
+	s.renderFragment(w, tmplSessionsIndex, indexPage{Page: pageSessions, Title: "Peek", BackLink: s.config.BackLink})
 }
```

- Same one-line addition in `handleSessionDetailPage` ([sessions.go:91](control/sessions.go:91)) and `handleStatsPage` ([stats.go:58](control/stats.go:58)).

#### 6. Nav template (modified)

location: [control/templates/layout.html:29](control/templates/layout.html:29)
mirrors: claude-configs `internal/server/templates/layout.html:41` (the reverse link)

```diff
 {{define "nav"}}
 <nav>
   <a href="/" {{if eq .Page "sessions"}}class="active"{{end}}>Sessions</a>
   <a href="/stats" {{if eq .Page "stats"}}class="active"{{end}}>Stats</a>
+  {{if .BackLink}}<a href="{{.BackLink}}">Hub</a>{{end}}
 </nav>
 {{end}}
```

#### 7. Docs (modified)

location: [docs/reference.md:14](docs/reference.md:14)

- Flag table: `--back-link` row — "— | URL of an external dashboard the nav links back to (empty hides the link)".
- Env table: `PEEK_BACK_LINK` → `--back-link` row.
- Config-file paragraph: add `back_link` to the persistable-keys list.

### Phase 2 — claude-configs (shippable alone: passes the flag Phase 1 introduced)

#### 8. Spawn wiring (modified)

location: `/Users/kevinpersonal/GolandProjects/claude-configs/cmd/configserver/main.go:119`, `main.go:208-220`

```diff
 		if *peekStart {
-			startPeek(ctx, *peekBin, *peekPort, *peekControlPort)
+			startPeek(ctx, *peekBin, *peekPort, *peekControlPort, "http://"+*addr+"/")
 		}
```

```diff
-func startPeek(ctx context.Context, bin string, port, controlPort int) {
+func startPeek(ctx context.Context, bin string, port, controlPort int, backLink string) {
 	// ...
 	args := []string{"start", "--transport", "http", "--port", strconv.Itoa(port)}
 	if controlPort != 0 {
 		args = append(args, "--control-port", strconv.Itoa(controlPort))
+		args = append(args, "--back-link", backLink)
 	}
```

#### 9. Docs (modified)

location: `/Users/kevinpersonal/GolandProjects/claude-configs/README.md:357`

- One sentence in the peek integration prose: the spawn passes `--back-link` with the config server's own address so peek's nav links back.

### Persistence

- On approval, this plan is copied to `plans/control_server/design/change-back-nav.md` (change route; `raw.md`/`refined.md` untouched) as the first implementation step.

## Hot items

- **Validation logic (hot class 5)** — `setBackLink` in [config/file.go](config/file.go), full implementation:

```go
func (f *File) setBackLink(value string) error {
	if value == "" {
		f.BackLink = nil
		return nil
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.Errorf("File.setBackLink: Invalid field back-link: %s (want an absolute http(s) URL, empty hides the link)", value)
	}

	f.BackLink = &value
	return nil
}
```

- **UI screenshot** — deviation: plan mode forbids starting the server, so the screenshot of the nav with the Hub link is captured at implementation and stored under `plans/control_server/design/ui/` before the change is declared done.

## Tests

| Location.Method | Cases | Comment |
|-----------------|-------|---------|
| config/file_test.go `TestFile_Set` | back-link-valid (`http://127.0.0.1:6001/`)<br>back-link-empty-clears (pre-set field → nil, no error)<br>back-link-not-a-url (`::bad`)<br>back-link-relative (`/x`, no host)<br>back-link-bad-scheme (`ftp://h/`) | extends the existing table |
| config/file_test.go `TestSave`/`TestLoad` | round-trips `back_link` | one added assertion each |
| control/pages_test.go `TestSessionsPage` | `Config{BackLink: "http://127.0.0.1:6001/"}` → body contains `>Hub<` anchor<br>default server → body has no `Hub` | detail + stats pages covered by the same nav define; one page test suffices plus the negative |
| control/config_test.go `TestConfigFragment` | `back-link` row rendered editable | existing pattern |
| cmd/start_test.go `TestApplyConfigFileFallbacks` / `TestChangedConfigKeys` | `back-link` file value applied when flag unset; key reported changed when flag set | extends existing tables |

- Not tested: `startPeek` argument change — claude-configs `cmd/configserver/main.go` has no test file; verified in the running system instead.

## Test runbook

- **nav-back-link:** GET `/` on a peek started with `--back-link` — anchor present (curl, no runbook file; existing runbooks under `plans/control_server/runbooks/` re-verify unchanged pages).
- **config-set:** `POST /api/config/back-link` valid and invalid values (existing config runbook pattern).

## Contracts & sweeps

| Contract | Sides | Sweep |
|----------|-------|-------|
| `--back-link` flag name | peek `cmd/start.go` ↔ claude-configs `startPeek` | grep both repos for `back-link`/`PEEK_BACK_LINK`; docs tables match code |
| `back_link` JSON key | `config/file.go` ↔ `~/.peek/config.json` ↔ `control.Config` JSON | grep `back_link`; reference.md paragraph lists it |
| Spawn arg order/shape | `startPeek` ↔ peek cobra parsing | manual spawn check in Verification |

## Verification

- [ ] Run `make test` in the peek worktree — all packages pass.
- [ ] Run `go build ./...` and `go test ./...` in claude-configs — pass.
- [ ] Start peek: `./dist/peek-mcp start --transport http --port 4243 --control-port 42443 --back-link http://127.0.0.1:6001/` — nav on `/`, `/stats`, and a session page shows Hub linking to the config server.
- [ ] Start peek without `--back-link` — no Hub anchor anywhere (degenerate case).
- [ ] Set `back-link` on `/stats` config editor to an invalid value (`not-a-url`) — 400, file unchanged; set a valid one — persists to `~/.peek/config.json`, restart applies it.
- [ ] Kill the running peek, restart the config server (`launchctl kickstart -k gui/$UID/com.smine.configserver`) — spawned peek's dashboard shows Hub → `http://127.0.0.1:6001/`; clicking it lands on the config server, whose nav links back to peek (round trip).
- [ ] Confirm existing pages with `back-link` unset render no empty anchor (view source of nav block).

## Stop conditions

| ID | Condition | Action |
|----|-----------|--------|
| S1 | An approved signature/contract can't hold as planned | stop and report; never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | stop, research the actual cause, redesign |
| S3 | Missing prerequisite (generated code, running infra) | run the producing step; if infrastructure is down, ask |
| S4 | Discovered work materially exceeds approved scope | ask before continuing |
| S5 | Same kind of bug twice: in own diff → fix all in diff; pre-existing outside → report and ask | no unapproved sweeps |
| S6 | Structural obstacle tempts a new abstraction | stop and report; relocate, don't indirect |
| S7 | Any new identifier would need the bare word "control" for the external target | stop — naming collision with peek's own control server |
| S8 | The behavior contract (byte-identical default output) can't hold | stop and report |

## Changelog

| Date | Trigger | What changed |
|------|---------|--------------|
| — | initial | plan created |
