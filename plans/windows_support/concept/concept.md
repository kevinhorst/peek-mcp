# Concept: Windows Support

> **Status:** Approved
> **Author:** Kevin Horst
> **Date:** 2026-08-10

---

## Goals

- peek-mcp runs on Windows against real Claude Code and Codex session directories (`%USERPROFILE%\.claude`, `%USERPROFILE%\.codex`).
- Primary driver: smine headless session mining runs against a Windows machine's transcripts.
- Windows binaries (`.exe`, amd64 and arm64) are published with every release.
- The setup wizard registers the server correctly on Windows for Claude Code, Claude Desktop, and Codex.

---

## User Flows

### Install and run on Windows

**Goals:**
- A Windows developer downloads a release binary, runs `peek-mcp setup`, and gets live session tailing without building from source.

**Options:**

**MVP** (~4–6d total)
- Cross-compiled `peek-mcp-windows-amd64.exe` / `peek-mcp-windows-arm64.exe` release assets (~0.5d)
- Makefile Windows build targets; portable release version-bump (replace BSD `sed -i ''`) (~0.5d)
- `runtime.GOOS` branch for the Claude Desktop config path (`%APPDATA%\Claude\claude_desktop_config.json`) (~0.5d)
- Windows CI test leg (`windows-latest`) (~0.5d)
- README: Windows install section, path documentation, PowerShell hot-reload hook variant, requirements update (~0.5–1d)
- Windows validation pass: fsnotify tail behavior, diff-watcher atomic rename, real Claude Code/Codex session dirs (~1–2d)

**Backlog**
- Windows `.mcpb` bundle (`platforms: ["win32"]`, `.exe` entry point) (~1d)
- `%USERPROFILE%` / `%APPDATA%` expansion in `expandHome` (~0.5d)
- Code signing for Windows binaries via SignPath OSS tier or Azure Trusted Signing (~1–2d, needs signup/identity validation first)

**Challenges:**
- fsnotify on Windows (ReadDirectoryChangesW) fires different events for appends and new subdirectories than kqueue/inotify; the JSONL tail and the Create-driven watch registration for new date dirs (`sessions/YYYY/MM/DD/`) must be validated on a real Windows machine.
- `os.Rename` over an existing open file can fail on Windows (`watcher/diff_watcher.go:333-338` `writeFileAtomic`).
- Unsigned `.exe` binaries trigger SmartScreen warnings.

**Approach:**
- Code is already portable in intent: `filepath.Join` throughout, CRLF-safe line splitting (`watcher/watcher.go:171`), `os.UserHomeDir()` defaults, `os.Interrupt` only, no syscall or build tags. The work is concentrated in tooling, packaging, one hardcoded macOS path, and behavioral validation.
- Cross-compile from the existing macOS release runner (`CGO_ENABLED=0` makes this trivial); add a `windows-latest` test job to catch path/rename/exec regressions.
- Retry-on-failure or delete-then-rename fallback for `writeFileAtomic` if validation shows rename failures.

---

## Decisions / Open Questions

**Decisions:**
- No daemon/service work — none exists on any platform; the server runs foreground or is spawned by the MCP client.
- No path-handling rewrite — production code already uses `path/filepath` correctly; the only `"path"` imports operate on URLs.
- The encoded-cwd project directory names under `~/.claude/projects/` are not a portability hazard: peek-mcp never decodes them, it reads the real `cwd` from each JSONL entry.
- [USER] No Windows `.mcpb` bundle in MVP — a bare `.exe` is enough. The MVP use case is smine headless mining, which runs peek-mcp as a stdio/HTTP server, not as a Claude Desktop extension. Bundle stays backlog.
- [USER] Validation runs on a real Windows 10 machine AND in CI — the Win 10 box covers behavioral items (fsnotify semantics, rename-over-open-file, real session dirs), the `windows-latest` CI leg guards against regressions.
- [USER] MVP ships unsigned; code signing stays backlog conditional on a low-hassle path. Evidence: every signing route (SignPath OSS free tier, Azure Trusted Signing, OV cert) requires signup/identity validation up front — none is hassle-free today. The SmartScreen warning is documented in the README instead.
- [USER] Windows binaries are cross-compiled from the existing `macos-latest` release job (`CGO_ENABLED=0` makes this a two-line addition) — no per-OS release jobs.

**Open Questions:**
None.
