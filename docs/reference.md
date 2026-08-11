[← peek-mcp](../README.md)

# Operations reference

Flags, environment variables, the control server, hot reload, the Claude Desktop bundle, and platform notes.

## Flags

```bash
peek-mcp start --port 4242 --depth 20
```

| Flag | Default | Description |
|------|---------|-------------|
| `--transport` | `http` | Transport: `http` or `stdio` |
| `--port` | `4242` | HTTP port (http transport only) |
| `--depth` | `20` | Ring buffer depth per session (max turns kept) |
| `--claude-home` | `~/.claude` | Override Claude Code session root |
| `--codex-home` | `~/.codex` | Override Codex session root |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `--poll-interval` | `1s` | How often to recompute the live uncommitted diff |
| `--poll-window` | `1h` | Only poll repos whose session was active within this window |
| `--state-dir` | `~/.peek/state` | State directory for diff pins/snapshots and plan revisions (empty disables persistence) |
| `--state-retention-days` | `90` | Days to keep per-session state before GC removes it (0 disables GC) |
| `--control-port` | `42442` | Control server start port; walks up to `42499` if taken (dashboard + JSON API + SSE); `0` disables |
| `--control-token` | — | Optional bearer token protecting the control server |

## Environment variables

Every flag has a corresponding environment variable that is used when the flag is not explicitly set. This is useful for the Claude Desktop `.mcpb` bundle where flags cannot be changed at runtime.

| Variable | Flag |
|----------|------|
| `PEEK_TRANSPORT` | `--transport` |
| `PEEK_PORT` | `--port` |
| `PEEK_DEPTH` | `--depth` |
| `PEEK_CLAUDE_HOME` | `--claude-home` |
| `PEEK_CODEX_HOME` | `--codex-home` |
| `PEEK_POLL_INTERVAL` | `--poll-interval` |
| `PEEK_POLL_WINDOW` | `--poll-window` |
| `PEEK_STATE_DIR` | `--state-dir` |
| `PEEK_STATE_RETENTION_DAYS` | `--state-retention-days` |
| `PEEK_CONTROL_PORT` | `--control-port` |
| `PEEK_CONTROL_TOKEN` | `--control-token` |
| `PEEK_LOG_LEVEL` | `--log-level` |

## Control server (dashboard + JSON API)

```bash
peek-mcp start --control-port 42442
```

Serves a live dashboard on `http://127.0.0.1:42442/` in both transports — session list, turns, plan, and diffs update as agents work. If the start port is taken (e.g. another harness already bound it), the server walks up to `42499` and binds the first free port, logging the chosen address; it fails only when the whole range is exhausted. The same data is scriptable as JSON:

```bash
curl -s http://127.0.0.1:42442/api/sessions | jq
curl -s "http://127.0.0.1:42442/api/sessions/<id>/diff?size=0" | jq -r .diff
curl -N http://127.0.0.1:42442/api/events
```

The server is read-only, binds to loopback only, rejects non-local `Host` headers, and sends no CORS headers. With `--control-token <t>`, requests need `Authorization: Bearer <t>` — or open `http://127.0.0.1:42442/?token=<t>` once in the browser to set a session cookie.

## Telemetry

peek-mcp can ingest Claude Code's OpenTelemetry export to enrich `session_events` with true active time and session cost. The control server exposes an OTLP receiver at `POST /otlp/v1/metrics` (protocol `http/json` only) and folds `claude_code.active_time.total` and `claude_code.cost.usage` per `session.id`; everything else is ignored, nothing is persisted.

`peek-mcp setup` (Claude Code choice) offers a telemetry step that writes the export config into `~/.claude/settings.json`:

```json
{
  "env": {
    "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
    "OTEL_METRICS_EXPORTER": "otlp",
    "OTEL_EXPORTER_OTLP_PROTOCOL": "http/json",
    "OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:42442/otlp"
  }
}
```

The endpoint base must match the control server port (`--control-port`, default 42442); Claude Code appends `/v1/metrics` per the OTLP spec. When the control server runs with `--control-token`, add `"OTEL_EXPORTER_OTLP_HEADERS": "Authorization=Bearer <token>"` — the setup step prompts for both. Metrics arrive at the default 60-second export interval, so the `telemetry` block appears in `session_events` about a minute after a session starts.

## Hot reload (live diff)

To keep Claude Code grounded in your current work as you edit — a "hot reload" — peek-mcp keeps an up-to-date `git diff HEAD` for each active repo and writes it to `<gitDir>/peek-diff` (inside `.git/`, so it is never committed and resolves correctly inside linked worktrees). A `UserPromptSubmit` hook then injects that diff into context on every prompt. The hook needs only `git` and `cat` — no peek binary on `PATH`, no server call — so it works under both the HTTP and `.mcpb` deployments.

Merge `hooks/settings.snippet.json` into your project `.claude/settings.json`:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      { "hooks": [ { "type": "command", "command": "cat \"$(git rev-parse --git-path peek-diff)\" 2>/dev/null" } ] }
    ]
  }
}
```

On Windows the hook works unchanged: Claude Code on Windows requires Git for Windows and runs hooks through its bash.

## Installing in Claude Desktop (.mcpb)

For one-click install on macOS — useful for distributing peek-mcp inside an organisation — peek-mcp ships as an [MCP Bundle](https://github.com/modelcontextprotocol/mcpb). The bundle is a self-contained `.mcpb` file with a universal (arm64 + amd64) macOS binary inside.

Build the bundle (requires macOS, since it uses `lipo` to fuse architectures):

```bash
make build-mcpb
# → dist/peek-mcp.mcpb
```

Install:

1. Open Claude Desktop → **Settings → Extensions**.
2. Click **Advanced settings**, find the **Extension Developer** section, click **Install Extension…**.
3. Pick `dist/peek-mcp.mcpb` and follow the prompts. The configuration UI exposes ring-buffer depth, the Claude / Codex session roots, and the diff target branch.

When launched this way, Claude Desktop runs `peek-mcp start --transport=stdio` directly — no HTTP server, no port to manage.

If macOS Gatekeeper quarantines the unsigned binary on first run:

```bash
xattr -dr com.apple.quarantine ~/Library/Application\ Support/Claude/Extensions/peek-mcp
```

## Windows

Download and run [peek-mcp-setup.exe](https://github.com/kevinhorst/peek-mcp/releases/latest)
— a wizard that installs the binary, configures Claude Code and/or Codex CLI, lets you
enable or disable the control server dashboard (default on), and optionally adds
peek-mcp to your PATH. The finish page starts peek-mcp (a console window stays open)
and opens the dashboard. Uninstalling removes the binary but leaves your agent
configs untouched.

For a manual install, download `peek-mcp-windows-amd64.exe` (or `-arm64.exe`) from the
[latest release](https://github.com/kevinhorst/peek-mcp/releases/latest),
rename it to `peek-mcp.exe`, and place it on your `PATH`.

The binary is unsigned; on first run SmartScreen may warn. Choose
**More info → Run anyway**, or unblock it in PowerShell:

```powershell
Unblock-File peek-mcp.exe
```

## Limitations

- The `diff` and `uncommitted_diff` sections of `session_get` require a local `git` binary (≥ 2.30, for `git diff --merge-base`) in `PATH` and run in the session's working directory. They produce no output if the directory is not a git repository.
- Codex CLI sessions do not currently expose token usage metadata.
- The stdio transport is intended for Claude Desktop use via `.mcpb`. Running it manually requires the client to manage the process lifecycle.

## Requirements

- Go 1.26+
- macOS, Linux, or Windows
- Claude Code and/or Codex CLI installed (peek-mcp reads their output; it does not depend on them at runtime)
