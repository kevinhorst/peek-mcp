# Contributing to peek-mcp

## Architecture overview

Understanding the data flow is a prerequisite for meaningful contributions.

```
JSONL on disk
    │
    ▼
watcher.Watcher          — fsnotify loop, reads new lines from session files
    │
    ▼
claude.Parser            — parses Claude Code JSONL into Turn / metadata
codex.Parser             — parses Codex CLI JSONL into Turn / metadata
    │
    ▼
session.Store            — in-memory ring buffer, keyed by session ID
    │        ▲
    │        │  plan_watcher.go — watches ~/.claude/plans/, writes Plan to store
    │        │  diff_watcher.go — runs git diff after each turn, writes Diff to store
    ▼
tools.Register           — exposes session_full / session_latest / session_get /
                           session_list / session_plan / session_diff via mcp-go
    │
    ▼
HTTP (streamable) or stdio transport
```

The server is intentionally passive. It never writes to the directories it watches and never communicates with Claude Code or Codex directly.

## Package map

| Package | Responsibility |
|---------|---------------|
| `cmd/` | Cobra command, flag parsing, watcher goroutine wiring, HTTP server |
| `session/` | `Store` — thread-safe ring buffer; `Turn`, `Meta`, `Plan`, `Diff` types |
| `claude/` | Parser for `~/.claude/projects/<encoded-cwd>/*.jsonl` |
| `codex/` | Parser for `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` |
| `watcher/` | File watcher, plan watcher, diff watcher |
| `tools/` | MCP tool registration and handlers |
| `mcpb/` | Claude Desktop bundle manifest and build assets |

## Prerequisites

- Go 1.26+
- macOS or Linux
- A working installation of Claude Code or Codex CLI to generate session files against

## Building

```bash
make build
```

For distribution builds:

| Target | Output |
|--------|--------|
| `make build-darwin-universal` | `dist/peek-mcp` — fat binary (arm64 + amd64) via `lipo` |
| `make build-linux-amd64` | `dist/peek-mcp-linux-amd64` |
| `make build-linux-arm64` | `dist/peek-mcp-linux-arm64` |
| `make mcpb` | `dist/peek-mcp.mcpb` — Claude Desktop bundle (macOS only) |

`make clean-dist` removes the `dist/` directory.

## Running locally

```bash
make serve-http
```

Builds and starts the HTTP server on `http://localhost:4242/mcp` with debug logging enabled. Open a Claude Code session in another terminal to generate traffic; `session_latest` will reflect it within seconds.

To test the stdio transport:

```bash
make serve-stdio
```

Input is MCP-framed JSON on stdin; output is MCP-framed JSON on stdout.

## Tests

```bash
make test
```

New parser logic and store behaviour must ship with tests. The parsers (`claude/`, `codex/`) and the store (`session/`) are the areas most likely to need coverage for a given change.

## Code style

Standard Go conventions apply — `gofmt`, `go vet`, no `init()` side effects beyond flag registration. Log via `slog` at the appropriate level:

- `slog.Debug` — parse skips, per-turn store updates, diff refresh confirmations
- `slog.Info` — server startup, HTTP requests
- `slog.Warn` — recoverable errors (watch failures, unreadable files)
- `slog.Error` — fatal conditions, always followed by `os.Exit(1)`

Use structured key-value pairs, not format strings: `slog.Warn("watcher.Add", "err", err)`.

## Adding a new agent

peek-mcp currently supports Claude Code and Codex CLI. To add another agent:

1. Create a new package (e.g. `myagent/`) with a `Parser` that implements the interface consumed by `watcher.New`.
2. Add the watched directory path and a `watcher.New(...)` goroutine in `cmd/start.go`.
3. Add a flag for the agent's home directory if the path is not fixed.
4. Document the session path in the README's supported agents table.

## Releasing

Releases are tagged from `main`. To cut a release:

```bash
make git-release VERSION=x.y.z
```

This updates the version string in `cmd/version.go` and `mcpb/manifest.json`, commits with the message `cmd: release vx.y.z`, and creates the tag. Push both the commit and the tag:

```bash
git push origin main --tags
```

## Updating dependencies

```bash
make update-go-deps
```

This updates all non-indirect dependencies, runs `go mod tidy`, and vendors if a `vendor/` directory exists.

## Submitting changes

Open a pull request against `main`. Keep the diff focused — one logical change per PR. The PR description should explain what the change does and why, not just what files were modified.

Commit messages should be short and direct. Do not include co-author attributions for AI tooling.