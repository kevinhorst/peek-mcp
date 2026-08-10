# Windows Port

---

## Flows

### Install from release

1. User downloads `peek-mcp-windows-amd64.exe` from the GitHub release.
2. User places it on `PATH` (or anywhere) and runs `peek-mcp setup`.
3. Backend
   1. `resolveBinaryPath` resolves via `os.Executable()` (already portable; `exec.LookPath` fallback finds `.exe` automatically on Windows).
   2. Setup writes MCP entries into `%USERPROFILE%\.claude.json`, `%APPDATA%\Claude\claude_desktop_config.json` (new `runtime.GOOS` branch), and `%USERPROFILE%\.codex\config.toml`.
4. User restarts the MCP client; peek-mcp is available.

### Release pipeline

1. Maintainer runs `make git-release` (version bump made portable — no BSD `sed -i ''`).
2. Tag push triggers `.github/workflows/release.yml`.
3. CI
   1. Test job runs on `ubuntu-latest` and `windows-latest`.
   2. Release job cross-compiles `GOOS=windows GOARCH=amd64|arm64` with `CGO_ENABLED=0` alongside existing darwin/linux targets.
   3. `.exe` assets are attached to the release.

### Session tailing on Windows

1. Claude Code / Codex writes JSONL under `%USERPROFILE%\.claude\projects\...` / `%USERPROFILE%\.codex\sessions\YYYY\MM\DD\`.
2. Backend
   1. `os.UserHomeDir()` resolves the roots (already portable).
   2. fsnotify (ReadDirectoryChangesW) delivers Write/Create events; the tail reader trims `\r` (already handled at `watcher/watcher.go:171`).
   3. Create events on new date/project subdirectories register new watches (existing mechanism, needs Windows validation).

---

## Security Considerations

- MVP `.exe` is unsigned; the SmartScreen warning is documented in the README. Signing (SignPath OSS tier or Azure Trusted Signing) is backlog, conditional on a low-hassle path.
- No elevation required; all writes go to per-user config locations.

---

## Limits

- Windows arm64: built and published, best-effort tested (no arm64 CI runner) — same policy as linux-arm64 today.

---

## Infrastructure

- `Makefile`: add `build-windows-amd64` / `build-windows-arm64` targets producing `.exe`; replace `sed -i ''` in `git-release` with a portable mechanism.
- `.github/workflows/release.yml`: add `windows-latest` to the test matrix; add two cross-compile steps and `.exe` release assets to the release job, which stays on `macos-latest` (decided: cross-compile, no per-OS release jobs).
- `mcpb/manifest.json`: untouched in MVP; a `win32` variant is backlog (decided: bare `.exe` suffices for the smine headless-mining use case).

---

## Long-Tail Tasks

### Windows validation

Runs on the available Windows 10 machine (decided); CI covers regressions only.

- fsnotify event semantics: verify append-driven Write events keep the JSONL tail live, and Create events on new subdirectories trigger watch registration.
- `writeFileAtomic` (`watcher/diff_watcher.go:333-338`): verify `os.Rename` over an existing `peek-diff` file while a hook reads it; add retry or remove-then-rename fallback if it fails.
- Verify Claude Code and Codex on Windows actually write to `%USERPROFILE%\.claude` / `%USERPROFILE%\.codex`, and that encoded-cwd project dir names with drive letters parse (expected fine — peek reads `cwd` from entries).
- Git for Windows: confirm the diff watcher's git invocations behave identically (flags already portable).

### Documentation

- README: Windows install section, Windows path table, requirements update (currently "macOS or Linux").
- Hot-reload hook: PowerShell equivalent of the bash `cat "$(git rev-parse --git-path peek-diff)"` one-liner.
