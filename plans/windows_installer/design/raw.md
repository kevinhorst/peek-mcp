# Windows install experience — Change Plan

## TLDR

- Ship a real Windows install wizard: an Inno Setup installer (`peek-mcp-setup.exe`) built and released by CI, with checkboxes for Claude Code / Codex configuration, control-server on/off (default on), PATH, and a finish-page "Open control server dashboard" option.
- The installer delegates all config writing to the binary via a new non-interactive `peek-mcp setup --claude --codex --control-server=<bool>` mode — one source of truth for config logic.
- The terminal wizard drops the Claude Desktop option (menu becomes Claude Code / Codex / All) and gains the control-server question; `setupClaudeDesktop` is deleted. Claude Desktop on macOS keeps its existing `.mcpb` route.
- Trusted install: SignPath Foundation chosen [USER]; the application is not instant, so signing is **out of this plan** — the installer ships unsigned with the SmartScreen note kept, and the SignPath CI step lands as a follow-up change once the account exists.

## Context

- Addendum to the shipped Windows MVP ([plans/windows_support/design/raw.md](plans/windows_support/design/raw.md), released in v1.0.8): `.exe` release assets, Windows CI leg, README docs exist; install is still manual (download, rename, PATH, terminal wizard) and SmartScreen-warned.
- The originating plan decided code signing and a GUI bundle **out of MVP** — explicitly as MVP scope cuts, not permanent decisions. This addendum picks them up; no `[USER]` decision is overridden.
- The control server already supports opt-out (`--control-port 0`, default 42442 with port walk, [cmd/listen.go:11](cmd/listen.go:11), [cmd/start.go:204](cmd/start.go:204)) — driver A3 is about surfacing that choice at setup time, not new server behavior.

## Drivers

| ID | Observed | Wanted | Impact | Origin |
|---|---|---|---|---|
| A1 | Downloaded `.exe` triggers SmartScreen "unknown publisher" warning | Install without trust warning, or at least with a real publisher identity | contract-touching (release pipeline, distribution) | Win 10 usage |
| A2 | Install is terminal-only: download, rename, place on PATH, run wizard in a console | A real GUI install wizard | behavioral | Win 10 usage |
| A3 | Control server is always on; no install-time choice | Setup option to enable/disable the control server, default on | behavioral | request |
| A4 | No way to reach the dashboard from the installer | Finish-page option "Open control server dashboard", opening it if running | behavioral | request |
| A5 | Wizard offers Claude Code / Claude Desktop / Codex / All | Only Claude Code + Codex — Claude Desktop is not necessary | behavioral | request |

## Scope

- **Opportunity menu** (ranked; all in unless cut):
  1. Non-interactive `setup` subcommand with flags — prerequisite for the installer, useful standalone
  2. Wizard: drop Claude Desktop, add control-server question (A3, A5)
  3. Inno Setup installer with tasks + open-dashboard finish option (A2, A4)
  4. CI job building and attaching `peek-mcp-setup.exe` to releases
  5. winget submission — complements A1 (winget installs carry no Mark-of-the-Web → no SmartScreen); **proposed as follow-up, not in this plan's Changes**
- **In:**
  - `setup` subcommand with `--claude` / `--codex` / `--control-server` flags
  - wizard menu and control-server prompt changes
  - `installer/windows/peek-mcp.iss`
  - release workflow installer job
  - README updates
- **Out:**
  - **signing (A1)** — SignPath Foundation chosen [USER]; CI signing step is a follow-up change once the application is approved
  - **winget manifest** — separate submission workflow, follow-up
  - **`.mcpb` win32 variant** — still out (originating concept decision)
  - **auto-start / daemon** — still out
- **Not changed:**
  - **control server behavior** — `--control-port` contract and port walk stay as-is
  - **`.mcpb` bundle** — remains the Claude Desktop (macOS) route
- **Deferred findings:**
  - **wizard menu labels** print unix-style paths on Windows — cosmetic, carried over from the originating plan's deferred list

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| A GUI installer needs new tooling in CI | Inno Setup 6.7.1 is preinstalled on `windows-latest` runners (`iscc` on PATH) | actions/runner-images Windows2025 readme |
| Installer must detect arm64 | `IsArm64` is a built-in Inno Setup 6.3+ support function | Inno Setup docs |
| Second release upload overwrites the first | `softprops/action-gh-release` appends files to an existing tag's release | release.yml:58 (existing usage), action docs |
| Signing removes the SmartScreen warning | OV-level signing (SignPath, Azure) replaces "unknown publisher" with a named publisher and accelerates reputation, but per-file SmartScreen reputation still accrues — only established reputation/EV removes the prompt instantly | Microsoft SmartScreen reputation docs |
| Azure signing is open to individuals | Azure Artifact Signing (ex Trusted Signing) GA: businesses in US/CA/EU/UK; individuals US/CA only — for Kevin this means validating via the aqms business, not as an individual | Microsoft Artifact Signing docs (2026-04 GA) |

## Current state

| File | Lines | Responsibility |
|---|---|---|
| [cmd/setup.go](cmd/setup.go) | 299 | interactive wizard: 5-option menu ([cmd/setup.go:25](cmd/setup.go:25)), `setupClaudeCode`, `setupClaudeDesktop` (58 lines, to delete), `setupCodex`, helpers |
| [cmd/root.go](cmd/root.go) | 15 | bare `peek-mcp` runs the wizard |
| [cmd/prompt.go](cmd/prompt.go) | 59 | `prompter.Confirm` / `Choose`, TTY-driven |
| [cmd/start.go:257](cmd/start.go:257) | — | warn text already references `peek-mcp setup`, which does not exist as a subcommand yet |
| [.github/workflows/release.yml](.github/workflows/release.yml) | 68 | test matrix + macOS release job; windows exes cross-compiled there |
| [README.md:134](README.md:134) | — | Windows section: manual download/rename/PATH + SmartScreen note |

Duplication note: the `args` written for Claude Code ([cmd/setup.go:104](cmd/setup.go:104)) and Codex ([cmd/setup.go:198](cmd/setup.go:198)) are the same list in two syntaxes — the control-server option would make it three sites; extracted once as `mcpArgs`.

## Target state

```
peek-mcp-setup.exe (Inno Setup, per-user, no UAC)
  [Tasks] claude / codex / controlserver / addtopath
  [Files] arch-matched exe → %LOCALAPPDATA%\Programs\peek-mcp\peek-mcp.exe
  [Run]   peek-mcp.exe setup --claude --codex [--control-server=false]   (hidden, waits)
          "Open control server dashboard" → http://127.0.0.1:42442        (postinstall checkbox)

peek-mcp setup            → interactive wizard (same as bare peek-mcp)
peek-mcp setup --claude --codex --control-server=false
                          → non-interactive, overwrites existing entries
```

- **Principle: single source of truth** — the installer never writes agent configs itself; it shells out to `peek-mcp setup` with flags. Mechanism: Inno `[Run]` entry with a `{code:...}` parameter builder over `WizardIsTaskSelected`.
- **Principle: one prompt engine, two modes** — non-interactive mode is the existing `prompter` with an `auto` field answering defaults-to-yes, not a parallel code path. Mechanism: `auto bool` short-circuit in `Confirm`/`Choose`.

## Behavior contract

- Must not change: config merge semantics (existing keys preserved, overwrite confirmation in interactive mode), `--control-port` contract, `.mcpb` bundle, macOS/Linux install paths, MCP server entry shape (`type`, `command`, `env.MAX_MCP_OUTPUT_TOKENS`).
- Intentional changes (map to drivers): wizard menu loses Claude Desktop (A5); wizard asks one new control-server question (A3); `setup` becomes a real subcommand (A2 prerequisite); release gains one asset (A2); config `args` may gain `--control-port=0` (A3).

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d1"></a>D1 | Signing route for A1 | Assumptions table | [USER] SignPath Foundation; application pending → signing is out of this plan, installer ships unsigned, SignPath CI step follows as its own change when the account exists | Free for OSS, removes "unknown publisher"; not instant, so it must not block the installer |
| D2 | Installer technology | Assumptions table | Inno Setup 6, single script `installer/windows/peek-mcp.iss` | Preinstalled on CI runners, per-user install without UAC, tasks/finish-page checkboxes are native; WiX/MSI rejected — heavier toolchain for no added requirement |
| D3 | Two architectures | Makefile:25-32 | One installer containing both exes; `IsArm64` check picks which is installed as `peek-mcp.exe` | One download link, one asset name; size cost (~2× binary) acceptable |
| D4 | Who writes configs at install time | Current state | Installer runs `peek-mcp.exe setup <flags>` post-copy | Config logic exists once, in Go, tested; the installer stays dumb |
| D5 | Where the flags live | cmd/start.go:257 | New `setup` subcommand; bare `peek-mcp` stays interactive | The warn text already tells users to run `peek-mcp setup`; flags on the root command would be odd (`peek-mcp --claude`) |
| D6 | Non-interactive prompt semantics | Behavior contract | `auto` prompter answers yes to everything → overwrite on rerun | Installer reruns must be idempotent, not silently skip |
| D7 | How "control server off" is encoded | cmd/start.go:204 | Append `--control-port=0` to the written `args` | Reuses the existing flag contract; no new mechanism, no new config key |
| D8 | Claude Desktop wizard path | A5, README:264 | Delete `setupClaudeDesktop` and its menu entry; disposal complete — no dead code kept | Desktop on macOS is served by `.mcpb`; on Windows it is unwanted |
| D9 | Open-dashboard mechanics | cmd/listen.go:11 | Postinstall checkbox, `shellexec` of `http://127.0.0.1:42442`, unchecked by default | The server only runs once an agent starts peek-mcp — usually not at install time; port-walk means a walked port isn't found, accepted (base port is the overwhelmingly common case) |
| D10 | How the installer job gets the exes | release.yml:53-56 | Release job uploads windows exes as a workflow artifact; installer job downloads them | `make` is not on windows runners; rebuilding inline would duplicate build flags |
| D11 | Uninstall scope | Behavior contract | Uninstaller removes `{app}` only; agent configs keep their peek-mcp entries | Editing user configs on uninstall risks destroying unrelated keys; documented in README |
| D12 | PATH handling | — | Optional task, HKCU `Environment\Path` append with a dedup check | Configs use the absolute binary path anyway; PATH is only for manual terminal use — hence optional, per-user, no admin |

## Changes

### Phase 1 — non-interactive setup + wizard changes (Go)

#### 1.1 `mcpArgs` helper and control-server parameter (modified)

location: [cmd/setup.go:62](cmd/setup.go:62)

```go
func mcpArgs(controlServer bool) []string {
	args := []string{"start", "--transport=stdio"}
	if !controlServer {
		args = append(args, "--control-port=0")
	}
	return args
}
```

```diff
-func setupClaudeCode(p *prompter) error {
+func setupClaudeCode(p *prompter, controlServer bool) error {
 	fmt.Println("Configuring peek-mcp for Claude Code...")
```

```diff
 	servers["peek-mcp"] = map[string]any{
 		"type":    "stdio",
 		"command": binPath,
-		"args":    []string{"start", "--transport=stdio"},
+		"args":    mcpArgs(controlServer),
 		"env": map[string]any{
 			"MAX_MCP_OUTPUT_TOKENS": "125000",
 		},
 	}
```

```diff
-func setupCodex(p *prompter) error {
+func setupCodex(p *prompter, controlServer bool) error {
 	fmt.Println("Configuring peek-mcp for Codex CLI...")
```

Codex block builds its args list from the same helper (enclosing: `setupCodex`, [cmd/setup.go:198](cmd/setup.go:198)):

```diff
-	block := fmt.Sprintf("tool_output_token_limit = 125000\n[mcp_servers.peek-mcp]\ncommand = %q\nargs = [\"start\", \"--transport=stdio\"]\n",
-		binPath)
+	quoted := make([]string, 0, 3)
+	for _, a := range mcpArgs(controlServer) {
+		quoted = append(quoted, strconv.Quote(a))
+	}
+	block := fmt.Sprintf("tool_output_token_limit = 125000\n[mcp_servers.peek-mcp]\ncommand = %q\nargs = [%s]\n",
+		binPath, strings.Join(quoted, ", "))
```

#### 1.2 Wizard: drop Claude Desktop, ask control-server (modified)

location: [cmd/setup.go:15](cmd/setup.go:15) — enclosing `runSetup`

```diff
 	p := newPrompter()
 	choice := p.Choose("Which environment do you want to configure?", []string{
 		"Claude Code     (~/.claude.json)",
-		"Claude Desktop  (claude_desktop_config.json)",
 		"Codex CLI       (~/.codex/config.toml)",
 		"All",
 		"Exit",
 	}, 0)

-	type setupFn func(*prompter) error
+	type setupFn func(*prompter, bool) error
 	var steps []setupFn

 	switch choice {
 	case 0:
 		steps = []setupFn{setupClaudeCode}
 	case 1:
-		steps = []setupFn{setupClaudeDesktop}
-	case 2:
 		steps = []setupFn{setupCodex}
-	case 3:
-		steps = []setupFn{setupClaudeCode, setupClaudeDesktop, setupCodex}
+	case 2:
+		steps = []setupFn{setupClaudeCode, setupCodex}
 	default:
 		return
 	}

+	controlServer := p.Confirm("Enable the control server dashboard (http://127.0.0.1:42442)?", true)
+
 	for i, fn := range steps {
 		if i > 0 {
 			fmt.Println()
 		}
-		if err := fn(p); err != nil {
+		if err := fn(p, controlServer); err != nil {
```

**Disposal:** `setupClaudeDesktop` ([cmd/setup.go:122-179](cmd/setup.go:122)) deleted entirely.

#### 1.3 Auto prompter (modified)

location: [cmd/prompt.go:12](cmd/prompt.go:12)

```diff
 type prompter struct {
 	scanner *bufio.Scanner
 	out     io.Writer
+	auto    bool
 }

 func newPrompter() *prompter {
 	return &prompter{scanner: bufio.NewScanner(os.Stdin), out: os.Stdout}
 }

+func autoPrompter() *prompter {
+	return &prompter{auto: true, out: os.Stdout}
+}

 func (p *prompter) Confirm(question string, defaultYes bool) bool {
+	if p.auto {
+		return true
+	}
 	hint := "y/N"
```

```diff
 func (p *prompter) Choose(question string, options []string, defaultIdx int) int {
+	if p.auto {
+		return defaultIdx
+	}
 	fmt.Fprintln(p.out, question)
```

#### 1.4 `setup` subcommand (new)

location: [cmd/setup.go:15](cmd/setup.go:15), above `runSetup`
mirrors: `startCmd` registration pattern ([cmd/start.go:29](cmd/start.go:29), init at [cmd/start.go:195](cmd/start.go:195))

```go
var setupCmd = &cobra.Command{
	Use:               "setup",
	Short:             "Configure agents to use peek-mcp",
	Long:              `Write peek-mcp MCP server entries into agent configs. Interactive without flags; --claude/--codex select targets non-interactively.`,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		claude, _ := flags.GetBool("claude")
		codex, _ := flags.GetBool("codex")
		controlServer, _ := flags.GetBool("control-server")

		if !claude && !codex {
			runSetup(cmd, args)
			return
		}

		p := autoPrompter()
		var steps []func(*prompter, bool) error
		if claude {
			steps = append(steps, setupClaudeCode)
		}
		if codex {
			steps = append(steps, setupCodex)
		}
		for _, fn := range steps {
			if err := fn(p, controlServer); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

func init() {
	flags := setupCmd.Flags()
	flags.Bool("claude", false, "Configure Claude Code non-interactively")
	flags.Bool("codex", false, "Configure Codex CLI non-interactively")
	flags.Bool("control-server", true, "Enable the control server dashboard in the written config")

	rootCmd.AddCommand(setupCmd)
}
```

The `setupFn` type moves to package level so both `runSetup` and the subcommand share it (replacing the local `type setupFn` in 1.2's diff).

### Phase 2 — Inno Setup installer

#### 2.1 Installer script (new)

location: `installer/windows/peek-mcp.iss`
mirrors: none in repo — new artifact class; structure follows Inno Setup canonical layout

```innosetup
#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

[Setup]
AppId={{7E1F7C34-9D3B-4A46-B4F2-2A7C0E6D5A11}
AppName=peek-mcp
AppVersion={#AppVersion}
AppPublisher=Kevin Horst
AppPublisherURL=https://github.com/kevinhorst/peek-mcp
DefaultDirName={localappdata}\Programs\peek-mcp
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
ArchitecturesInstallIn64BitMode=x64compatible and arm64
ChangesEnvironment=yes
OutputDir=..\..\dist
OutputBaseFilename=peek-mcp-setup
SolidCompression=yes

[Tasks]
Name: "claude"; Description: "Configure Claude Code (%USERPROFILE%\.claude.json)"
Name: "codex"; Description: "Configure Codex CLI (%USERPROFILE%\.codex\config.toml)"
Name: "controlserver"; Description: "Enable the control server dashboard (http://127.0.0.1:42442)"
Name: "addtopath"; Description: "Add peek-mcp to PATH"

[Files]
Source: "..\..\dist\peek-mcp-windows-amd64.exe"; DestDir: "{app}"; DestName: "peek-mcp.exe"; Check: not IsArm64
Source: "..\..\dist\peek-mcp-windows-arm64.exe"; DestDir: "{app}"; DestName: "peek-mcp.exe"; Check: IsArm64

[Registry]
Root: HKCU; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; ValueData: "{olddata};{app}"; Tasks: addtopath; Check: NeedsAddPath(ExpandConstant('{app}'))

[Run]
Filename: "{app}\peek-mcp.exe"; Parameters: "{code:SetupParams}"; StatusMsg: "Writing agent configuration..."; Flags: runhidden waituntilterminated; Check: WizardIsTaskSelected('claude') or WizardIsTaskSelected('codex')
Filename: "http://127.0.0.1:42442"; Description: "Open the control server dashboard (needs a running agent session)"; Flags: shellexec postinstall skipifsilent unchecked

[Code]
function SetupParams(Param: string): string;
begin
  Result := 'setup';
  if WizardIsTaskSelected('claude') then
    Result := Result + ' --claude';
  if WizardIsTaskSelected('codex') then
    Result := Result + ' --codex';
  if not WizardIsTaskSelected('controlserver') then
    Result := Result + ' --control-server=false';
end;

function NeedsAddPath(Param: string): boolean;
var
  OrigPath: string;
begin
  if not RegQueryStringValue(HKCU, 'Environment', 'Path', OrigPath) then
  begin
    Result := True;
    exit;
  end;
  Result := Pos(';' + Uppercase(Param) + ';', ';' + Uppercase(OrigPath) + ';') = 0;
end;
```

#### 2.2 Release workflow: artifact hand-off + installer job (modified)

location: [.github/workflows/release.yml:53](.github/workflows/release.yml:53)
mirrors: existing release job structure

```diff
       - name: Build windows binaries
         run: |
           make build-windows-amd64
           make build-windows-arm64

+      - uses: actions/upload-artifact@v4
+        with:
+          name: windows-binaries
+          path: |
+            dist/peek-mcp-windows-amd64.exe
+            dist/peek-mcp-windows-arm64.exe
+
       - uses: softprops/action-gh-release@v2
```

Appended after the release job:

```yaml
  installer:
    name: Windows installer
    runs-on: windows-latest
    needs: release
    if: startsWith(github.ref, 'refs/tags/v')
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4

      - uses: actions/download-artifact@v4
        with:
          name: windows-binaries
          path: dist

      - name: Build installer
        shell: pwsh
        run: |
          $version = $env:GITHUB_REF_NAME.TrimStart('v')
          iscc /DAppVersion=$version installer\windows\peek-mcp.iss

      - uses: softprops/action-gh-release@v2
        with:
          files: dist/peek-mcp-setup.exe
```

### Phase 3 — README

location: [README.md:134](README.md:134) — Windows section replaced:

```markdown
### Windows

Download and run [peek-mcp-setup.exe](https://github.com/kevinhorst/peek-mcp/releases/latest)
— a wizard that installs the binary, configures Claude Code and/or Codex CLI, lets you
enable or disable the control server dashboard (default on), and optionally adds
peek-mcp to your PATH. Uninstalling removes the binary but leaves your agent configs
untouched.

For a manual install, download `peek-mcp-windows-amd64.exe` (or `-arm64.exe`),
rename it to `peek-mcp.exe`, and place it on your `PATH`. If SmartScreen warns,
choose **More info → Run anyway**, or unblock it in PowerShell:

​```powershell
Unblock-File peek-mcp.exe
​```
```

location: [README.md:147](README.md:147) — Quick setup section gains the non-interactive form:

```markdown
Non-interactive (used by the Windows installer, works everywhere):

​```bash
peek-mcp setup --claude --codex --control-server=false
​```
```

The SmartScreen note stays until the SignPath follow-up lands (signing step: `signpath/github-action-submit-signing-request` between `iscc` and the release upload — written in that follow-up change, since project slug/policy IDs are account data).

## Hot items

N/A — no SQL, goroutines, new interfaces, generated formats, or validation changes. The Inno `[Code]` Pascal section is new territory but not a hot class; it is fully spelled out in [§2.1](#changes) anyway.

## Tests

| Location.Method | Cases | Comment |
|---|---|---|
| cmd/setup_test.go `TestMcpArgs` | control server on → no extra flag<br>off → trailing `--control-port=0` | pins D7's encoding |
| cmd/prompt_test.go `TestConfirm_Auto` | auto prompter returns true regardless of default | pins D6 overwrite semantics |
| cmd/prompt_test.go `TestChoose_Auto` | auto prompter returns defaultIdx | — |

- Existing `TestWriteConfig_*` / `TestReplaceTOMLSection_*` pin the merge behavior that must not change.
- Not tested: the `.iss` script and workflow — no test harness exists for either; covered by the runbook and a tag dry-run.
- Not tested: `setupClaudeCode`/`setupCodex` end-to-end — they read `os.UserHomeDir` directly (not injectable); pre-existing gap, unchanged by this plan.

## Test runbook

Tool: bash scripts under Git Bash, per the existing Windows smoke runbook ([plans/windows_support/runbooks/windows_smoke.sh](plans/windows_support/runbooks/windows_smoke.sh)).

Scenario 1 — silent full install (A2, A3-on, A5): location: `plans/windows_installer/runbooks/installer_smoke.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
SETUP_EXE="${1:?usage: installer_smoke.sh <path-to-peek-mcp-setup.exe>}"
"$SETUP_EXE" //VERYSILENT //SUPPRESSMSGBOXES //TASKS="claude,codex,controlserver,addtopath"
BIN="$LOCALAPPDATA/Programs/peek-mcp/peek-mcp.exe"
"$BIN" version
grep -q '"peek-mcp"' "$USERPROFILE/.claude.json" && echo "claude config: OK"
grep -q 'mcp_servers.peek-mcp' "$USERPROFILE/.codex/config.toml" && echo "codex config: OK"
! grep -q 'control-port=0' "$USERPROFILE/.claude.json" && echo "control server on: OK"
```

Scenario 2 — control server off (A3-off): location: `plans/windows_installer/runbooks/installer_no_control.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail
SETUP_EXE="${1:?usage: installer_no_control.sh <path-to-peek-mcp-setup.exe>}"
"$SETUP_EXE" //VERYSILENT //SUPPRESSMSGBOXES //TASKS="claude"
grep -q -- '--control-port=0' "$USERPROFILE/.claude.json" && echo "control server off: OK"
```

- A4 (open dashboard) and the wizard pages are GUI-only — verified manually per the Verification checklist, not scriptable.

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| `setup --claude --codex --control-server` CLI | cmd/setup.go producer<br>installer `SetupParams` consumer<br>README consumer<br>runbooks consumer | grep `control-server` repo-wide — all four sides agree on flag names |
| Asset name `peek-mcp-setup.exe` | release.yml uploader<br>README download link<br>iss `OutputBaseFilename` | grep `peek-mcp-setup` — three hits, consistent |
| Dashboard URL `http://127.0.0.1:42442` | listen.go const (authoritative)<br>iss `[Run]` + task text<br>setup.go prompt text<br>README | grep `42442` — survivors justified: the iss and prompt strings cannot read the Go const; a comment-free literal duplication accepted |
| Artifact name `windows-binaries` | release job upload<br>installer job download | grep in release.yml — both sides in one file |
| Removed: wizard "Claude Desktop" option | — | grep `setupClaudeDesktop` → zero; grep `Claude Desktop` → survivors only in `.mcpb` docs (README:264, CONTRIBUTING:42,63) and plans/ history — justified |

## Verification

Phase 1:
- [ ] Run `make test` — new and existing tests pass.
- [ ] Run `go build ./... && go vet ./...` — clean.
- [ ] Run bare `./dist/peek-mcp` — menu shows Claude Code / Codex CLI / All / Exit; control-server question appears with default Y.
- [ ] Run `./dist/peek-mcp setup --claude --control-server=false` on a scratch `$HOME` — `.claude.json` entry contains `--control-port=0`, no prompts appeared, rerun overwrites silently.

Phase 2:
- [ ] Push branch — both CI test legs green.
- [ ] Tag a pre-release (e.g. `v1.0.9-rc1`) — release has `peek-mcp-setup.exe` attached; installer job green.
- [ ] On the Win 10 machine: run installer GUI — tasks visible, control-server checked by default, finish page offers "Open the control server dashboard" unchecked.
- [ ] Run both runbook scenarios — all OK lines printed.
- [ ] With an agent session active (server running): tick the finish-page open option — browser lands on the dashboard.
- [ ] `where peek-mcp` in a NEW terminal after PATH task — resolves to `%LOCALAPPDATA%\Programs\peek-mcp`.
- [ ] Uninstall via Apps — binary gone, `.claude.json` peek-mcp entry still present.

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | Stop and report — never improvise architecture mid-edit |
| S2 | Second failed fix on the same mechanism | Stop, research the actual cause, redesign — no third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | Run the producing step; if infrastructure is down, ask — never start it yourself |
| S4 | Discovered work materially exceeds approved scope | Ask before continuing |
| S5 | Same kind of bug found twice — in own diff: fix all in diff; pre-existing outside diff | Report and ask before sweeping |
| S6 | Structural obstacle tempts a new abstraction | Stop and report — relocate, don't indirect |
| S7 | A mechanical transform loses fidelity vs its source | Diff element-by-element before presenting; any loss → stop |
| S8 | Old and new structure would coexist beyond the phasing | Stop and report — never leave a half-migration as done |
| S9 | A driver contradicts a `[USER]` decision in the originating plan | Surface the conflict, never silently override |
| S10 | `iscc` missing or failing on the CI runner | Stop and report — do not switch installer technology mid-implementation |

## Open questions

None.

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
| 2026-08-11 | approval feedback | D1 resolved [USER]: SignPath Foundation, application pending — signing phase removed from scope, ships unsigned |
