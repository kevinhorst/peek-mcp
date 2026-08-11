# Windows support — Implementation Plan

## TLDR

- Ship peek-mcp on Windows: `.exe` release binaries (amd64 + arm64), a working setup wizard, and Windows CI coverage.
- Driver: smine headless mining runs against a Windows 10 machine's Claude Code / Codex transcripts.
- The Go code is already portable; the changes are one config-path fix in the setup wizard, Makefile build targets, release-workflow additions, a portable version-bump, and README documentation.
- No `.mcpb` Windows bundle, no code signing, no daemon work (all decided out of MVP in the concept).

## Context

- Concept: [plans/windows_support/concept/concept.md](plans/windows_support/concept/concept.md) — clarified, status In Review, Open Questions empty. Binding input.
- The only Windows-breaking code path is the Claude Desktop config location hardcoded to `~/Library/Application Support/...` at [cmd/setup.go:137](cmd/setup.go:137).
- Build/release tooling is darwin/linux-only: no `GOOS=windows` Makefile target, no `.exe` release asset, no Windows CI leg, BSD-only `sed -i ''` in `git-release`.
- Everything else (session watching, git diff subsystem, JSONL parsing) is already portable and gets validated on the Win 10 machine, not rewritten.

## Scope

- **In:**
  - **setup wizard:** Claude Desktop config path resolved per-OS via `os.UserConfigDir()`
  - **Makefile:** `build-windows-amd64` / `build-windows-arm64` targets; portable `sed -i.bak` in `git-release`
  - **CI:** `windows-latest` test leg; Windows cross-compile + `.exe` release assets
  - **README:** Windows install/paths/requirements/hook documentation, SmartScreen note
  - **runbook:** Windows smoke script for the Win 10 validation pass
- **Out:**
  - **mcpb bundle:** no `win32` variant — decided out ([concept.md](plans/windows_support/concept/concept.md) Decisions)
  - **code signing:** MVP ships unsigned — decided out
  - **expandHome:** no `%USERPROFILE%` expansion — backlog
- **Not changed:**
  - **watcher/diff_watcher.go:** `writeFileAtomic` stays as-is; `os.Rename` replaces existing targets on Windows via `MOVEFILE_REPLACE_EXISTING`, failure is already logged non-fatally — becomes a validation item, code change only if validation shows flakiness (concept Approach)
  - **hooks/settings.snippet.json:** the bash hook works under Git Bash, which Claude Code on Windows requires anyway — documented, not forked
- **Deferred findings:**
  - **wizard menu labels:** the Choose menu at [cmd/setup.go:26-28](cmd/setup.go:26) prints unix-style paths (`~/.claude.json`) on all platforms — cosmetic, left as-is

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| Concept: Claude Desktop Windows config lives at `%APPDATA%\Claude\claude_desktop_config.json` | `os.UserConfigDir()` returns exactly `%AppData%` on Windows and `$HOME/Library/Application Support` on darwin — one stdlib call covers both, no `runtime.GOOS` branch needed | `go doc os.UserConfigDir` |
| Concept: PowerShell hook variant needed | Claude Code on Windows requires Git for Windows and runs hooks through it; the existing bash one-liner is expected to work unchanged — verified on the Win 10 box, not forked preemptively | README.md:241 |
| Concept: version-bump portability requires dropping sed | `sed -i.bak` + `rm` is BSD/GNU-portable; sed itself stays | Makefile:46-48 |

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d1"></a>D1 | Claude Desktop config path per OS | [F1!](#f1) | Replace the hardcoded darwin path with `filepath.Join(os.UserConfigDir(), "Claude", "claude_desktop_config.json")`, inlined in `setupClaudeDesktop` | One stdlib call yields the correct path on darwin, Windows, and Linux — fewer concepts than a `runtime.GOOS` switch, and single-caller so no helper |
| <a id="d2"></a>D2 | Portable `git-release` version bump | [F2!](#f2) | `sed -i.bak` on all three files, then `rm -f *.bak` | Two-character change per line, works on BSD and GNU sed; ldflags injection was rejected — it would leave plain `go build` binaries with a stale default version, adding a second source of truth |
| <a id="d3"></a>D3 | Where Windows binaries are built | [F3!](#f3) | Cross-compile in the existing `macos-latest` release job via new Makefile targets | [USER] — concept decision; `CGO_ENABLED=0` makes cross-compilation exact |
| <a id="d4"></a>D4 | Windows test coverage shape | [F4!](#f4) | Convert the CI test job to an OS matrix `[ubuntu-latest, windows-latest]` | Matrix reuses the existing job verbatim; `needs: test` in the release job waits on all matrix legs automatically |
| <a id="d5"></a>D5 | `writeFileAtomic` Windows behavior | [F5!](#f5) | No code change; runbook validates it, retry logic only if validation fails | [USER] — concept Approach binds "retry … if validation shows rename failures"; Go's `os.Rename` already replaces existing files on Windows, and failure is logged, not fatal |

## Baseline (verified)

Base branch: `claude/vigorous-agnesi-aad15e` (current worktree, clean at session start).

| ID | Fact | Needed for | Location |
|---|---|---|---|
| <a id="f1"></a>F1! | `os.UserConfigDir()` returns `%AppData%` on Windows, `$HOME/Library/Application Support` on darwin, XDG on Unix | [D1](#d1) | `go doc os.UserConfigDir` (Go 1.26.2 stdlib) |
| <a id="f2"></a>F2! | `git-release` uses BSD-only `sed -i ''` on Makefile, cmd/version.go, mcpb/manifest.json | [D2](#d2) | Makefile:45-50 |
| <a id="f3"></a>F3! | Release job runs on `macos-latest`, builds via `make` targets, uploads via softprops/action-gh-release file list | [D3](#d3), [§3](#c3) | .github/workflows/release.yml:21-57 |
| <a id="f4"></a>F4! | Test job is a single `ubuntu-latest` job running `go test ./...`; all tests use `t.TempDir()`, no /tmp, no symlinks, no chmod assertions | [D4](#d4) | .github/workflows/release.yml:10-19, cmd/setup_test.go:13 |
| <a id="f5"></a>F5! | `writeFileAtomic` = temp write + `os.Rename`; caller logs failure as Warn and continues | [D5](#d5) | watcher/diff_watcher.go:333-339, 165-167 |
| <a id="f6"></a>F6 | Existing runbooks are bash scripts with env-based host config (`PEEK_CONTROL_URL`) | [§6](#c6) | plans/control_server/runbooks/sessions_pagination.sh |
| <a id="f7"></a>F7 | Linux build targets are the two-line mkdir + env-prefixed `go build` pattern | [§2](#c2) | Makefile:15-22 |

## Exemplar & reuse

| Existing | Used for |
|---|---|
| `build-linux-amd64` / `build-linux-arm64` Makefile targets | Windows build targets mirror them exactly |
| Release workflow "Build linux binaries" step + asset list | Windows build step + `.exe` assets |
| plans/control_server/runbooks/*.sh | Windows smoke runbook format |

- Without exemplar: none — every change mirrors an existing sibling.

## Changes

### 1. Claude Desktop config path per OS (modified)

location: `cmd/setup.go`

```diff
 func setupClaudeDesktop(p *prompter) error {
 	// ...
-	home, err := os.UserHomeDir()
+	cfgDir, err := os.UserConfigDir()
 	if err != nil {
-		return fmt.Errorf("cannot determine home directory: %w", err)
+		return fmt.Errorf("cannot determine user config directory: %w", err)
 	}
 
-	path := filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
+	path := filepath.Join(cfgDir, "Claude", "claude_desktop_config.json")
 	fmt.Printf("  Config: %s\n", path)
```

### <a id="c2"></a>2. Windows build targets and portable version bump (modified)

location: `Makefile`
mirrors: `build-linux-amd64` / `build-linux-arm64` (Makefile:15-22)

```makefile
build-windows-amd64:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o $(DIST)/peek-mcp-windows-amd64.exe .


build-windows-arm64:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o $(DIST)/peek-mcp-windows-arm64.exe .
```

```diff
 git-release:
-	sed -i '' 's/^VERSION = .*/VERSION = $(VERSION)/' Makefile
-	sed -i '' 's/^var version = ".*"/var version = "$(VERSION)"/' cmd/version.go
-	sed -i '' 's/^  "version": ".*",/  "version": "$(VERSION)",/' mcpb/manifest.json
+	sed -i.bak 's/^VERSION = .*/VERSION = $(VERSION)/' Makefile
+	sed -i.bak 's/^var version = ".*"/var version = "$(VERSION)"/' cmd/version.go
+	sed -i.bak 's/^  "version": ".*",/  "version": "$(VERSION)",/' mcpb/manifest.json
+	rm -f Makefile.bak cmd/version.go.bak mcpb/manifest.json.bak
 	git commit -am "cmd: release v$(VERSION)"
 	git tag v$(VERSION)
```

### <a id="c3"></a>3. Windows CI leg and release assets (modified)

location: `.github/workflows/release.yml`
mirrors: existing test job and "Build linux binaries" step (release.yml:10-19, 45-49)

```diff
   test:
     name: Test
-    runs-on: ubuntu-latest
+    strategy:
+      matrix:
+        os: [ubuntu-latest, windows-latest]
+    runs-on: ${{ matrix.os }}
     steps:
       - uses: actions/checkout@v4
```

```diff
       - name: Build linux binaries
         run: |
           make build-linux-amd64
           make build-linux-arm64
 
+      - name: Build windows binaries
+        run: |
+          make build-windows-amd64
+          make build-windows-arm64
+
       - uses: softprops/action-gh-release@v2
         with:
           files: |
             dist/peek-mcp.mcpb
             dist/peek-mcp
             dist/peek-mcp-linux-amd64
             dist/peek-mcp-linux-arm64
+            dist/peek-mcp-windows-amd64.exe
+            dist/peek-mcp-windows-arm64.exe
           generate_release_notes: true
```

### 4. README Windows documentation (modified)

location: `README.md`

- **Requirements** (line 288): `macOS or Linux` → `macOS, Linux, or Windows`.
- **Supported agents table** (lines 99-102): add one sentence under the table — on Windows the roots resolve to `%USERPROFILE%\.claude` and `%USERPROFILE%\.codex`.
- **Installation** (after line 130): new Windows subsection:

```markdown
### Windows

Download `peek-mcp-windows-amd64.exe` (or `-arm64.exe`) from the
[latest release](https://github.com/kevinhorst/peek-mcp/releases/latest),
rename it to `peek-mcp.exe`, and place it on your `PATH`.

The binary is unsigned; on first run SmartScreen may warn. Choose
**More info → Run anyway**, or unblock it in PowerShell:

​```powershell
Unblock-File peek-mcp.exe
​```
```

- **Hot reload** (after line 245): one note — on Windows the hook works unchanged under Git Bash, which Claude Code on Windows requires.

### 5. Concept status bump (modified)

location: `plans/windows_support/concept/concept.md`

- Status `In Review` → `Approved` on plan approval (concept lifecycle).

### <a id="c6"></a>6. Windows smoke runbook (new)

location: `plans/windows_support/runbooks/windows_smoke.sh`
mirrors: plans/control_server/runbooks/sessions_pagination.sh

```bash
#!/usr/bin/env bash
set -euo pipefail
BASE="${PEEK_CONTROL_URL:-http://127.0.0.1:4243}"
./peek-mcp.exe version
curl -s "$BASE/api/sessions?agent=claude" | jq '{total, first: .sessions[0].id}'
ID=$(curl -s "$BASE/api/sessions?agent=claude" | jq -r '.sessions[0].id')
curl -s "$BASE/api/sessions/$ID/turns" | jq '.turns | length'
GITDIR=$(git rev-parse --absolute-git-dir)
test -f "$GITDIR/peek-diff" && echo "peek-diff written: OK"
```

## Hot items

N/A — no SQL, goroutines, interfaces, generated formats, validation changes, or anonymous structs; the one Go change swaps a stdlib call inside an existing function.

## Tests

| Location.Method | Cases | Comment |
|---|---|---|
| — | — | No new unit tests |

- **Not tested: the config-path expression** — a single stdlib call plus `filepath.Join`, no branching; a test would only re-assert Go's own documentation.
- **Existing suite on Windows:** the `windows-latest` CI leg runs the full `go test ./...` — this is the new coverage, exercising every `t.TempDir()`-based test (watcher, store, setup) under Windows path and rename semantics.
- Integration setup: the Windows CI leg needs nothing extra — git is preinstalled on `windows-latest` runners.

## Test runbook

- Scenario: end-to-end smoke on the Windows 10 machine — `plans/windows_support/runbooks/windows_smoke.sh` ([§6](#c6)), run under Git Bash in a git repo with active Claude Code sessions, server started with `peek-mcp.exe start --control-port 4243`.
- Host/auth via the existing `PEEK_CONTROL_URL` env mechanism (F6).
- Run line:

```bash
bash plans/windows_support/runbooks/windows_smoke.sh
```

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| Release asset names `peek-mcp-windows-{amd64,arm64}.exe` | release.yml producer<br>README install docs consumer | grep `peek-mcp-windows` across repo after change — README and workflow must agree |
| Error message `cannot determine home directory` (changed in setupClaudeDesktop) | none — not asserted anywhere | grep confirms no test or doc references it |

## Verification

- [ ] Run `make build-windows-amd64 build-windows-arm64` — expect `dist/peek-mcp-windows-amd64.exe` and `dist/peek-mcp-windows-arm64.exe` produced.
- [ ] Run `make test` — expect pass locally (no behavior change on darwin).
- [ ] Run `go build ./...` and `go vet ./...` — expect clean.
- [ ] Push branch → CI: expect both matrix legs (`Test (ubuntu-latest)`, `Test (windows-latest)`) green.
- [ ] Dry-run the sed change: `make git-release VERSION=1.0.7` on a scratch branch — expect identical file content, no `.bak` files left, then discard.
- [ ] On the Win 10 machine (post-merge, next release): download the `.exe`, run the smoke runbook — expect version printed, sessions listed, turns returned, `peek-diff` present.
- [ ] Win 10 degenerate checks: run `peek-mcp.exe start` with no Claude/Codex dirs present — expect clean start, empty session list, no crash.
- [ ] Win 10 validation items from the concept: append to an active session JSONL — expect the new turn via `/api/sessions/<id>/turns` within seconds (fsnotify); edit a tracked file — expect `peek-diff` to update (rename-over-existing works).

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | Stop and report — never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | Stop, research the actual cause, redesign — no third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | Run the producing step; if infrastructure is down, ask — never skip validation, never start infrastructure yourself |
| S4 | Discovered work materially exceeds approved scope | Ask before continuing |
| S5 | Same kind of bug found twice — in own diff: fix all in diff; pre-existing outside diff | Report and ask before sweeping |
| S6 | Structural obstacle tempts a new abstraction | Stop and report — relocate, don't indirect |
| S7 | `windows-latest` CI leg fails on pre-existing tests (not this diff) | Stop and report the failing tests — fixing Windows-only test bugs is scope the user approves, not silent expansion |
| S8 | Win 10 validation shows `writeFileAtomic` rename failures | Stop and report — the retry design is a follow-up decision per D5, not an inline improvisation |

## Open questions

None.

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
