# User Stories: Windows Support

---

## Installation

**As a Windows developer using Claude Code**, I want to install peek-mcp from a release `.exe`, so that I can inspect my sessions without building from source.

**As a Windows developer**, I want `peek-mcp setup` to register the server in the Claude Code, Claude Desktop, and Codex configs at their Windows locations, so that setup matches the macOS experience.

---

## Runtime

**As a Windows Codex user**, I want live session tailing to work on `%USERPROFILE%\.codex\sessions`, so that new turns appear without restarting the server.

**As a Windows Claude Code user**, I want the git diff watcher and hot-reload hook to work under PowerShell, so that the diff context feature is not macOS-only.

---

## Maintenance

**As a maintainer**, I want Windows covered in CI, so that path, rename, and exec regressions are caught before release.

**As a maintainer**, I want the release process to produce Windows artifacts without manual steps, so that every release ships all platforms.
