# peek-mcp

A lightweight MCP server that reads Claude Code and Codex CLI sessions directly from disk and exposes them over HTTP or stdio, so a second model can evaluate what a primary agent produced without spending tokens on summarization.

## The problem

Opus finishes a task. I am quite often in the situation that I want to have a quick follow-up question or analysis on the output. If I then prompt Opus or another bigger model, it eats up valuable tokens and especially much more time than necessary. So I want to use Sonnet or GPT5-mini to review it quickly, but copying the context by hand is cumbersome.

As of 05.04.2026 , there seems to be no other way to do cross-session communication between different integrations than to either copy or prompt the model to read the respective session directory directly, which works, but is also slow.
- update: 10.05.2026 - still nothing
- update 07.06.2026 - Claude Code now has "Memory" (not to be confused with MEMORY.md), so it can reference previous sessions. Helpful, but not enough

There seem to be some MCP servers that kinda, maybe do what I need, but they did not quite fit my case, so I wrote my own, which is more tailored to my current workflow.
Examples: TBD

I wanted to avoid any interruption in said workflow, so an approach where the agent pushes to an MCP was ruled out. The session files are on disk, so I figured that should be a good starting point and took it from there. It is also an experiment for a codebase with heavy use of agentic development (but not vibe coding).

## The solution

peek-mcp watches the session files that Claude Code and Codex write to disk automatically, parses them passively, and serves the last N turns via MCP. Any connected client calls `session_get` and quickly gets the context it needs.

```
Claude Code / Codex writes JSONL to disk (always, no configuration needed)
                    |
             fsnotify file watcher
                    |
          in-memory buffer per session
                    |
        MCP server over streamable HTTP or stdio
                    |
    Sonnet / GPT-5-mini calls session_get(n)
```

In addition to turns, peek-mcp passively watches two more sources:

- **Plans** — Claude Code writes a plan file to `~/.claude/plans/` at the start of each task. peek-mcp reads and stores it alongside the session so `session_get` can surface it without any extra prompting.
- **Git diffs** — After each new turn, peek-mcp infers the session branch's base (reflog creation point, falling back to `origin/HEAD`, then local `main`/`master`, then `HEAD`) and runs `git diff --merge-base <base>` in the session's working directory. `session_get` exposes the result via its `diff` section — no configuration needed; the resolved base is reported as `diff_target`.

## MCP Tools

**`session_get`** Returns session data (turns, plan, git diff, uncommitted diff) for a session in one call. Defaults to the most recently active session when `id` and `title` are omitted. Sections are selected with flat boolean flags. Responses are paginated: if `has_more` is true, call again with the returned `request_id` to get the next page.

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | no | Session ID (omit for most recent session) |
| `title` | string | no | Session title. Exact match first (case-insensitive); falls back to substring match. Scoped to `agent` when provided. For Codex, titles come from Codex's session index (thread name) |
| `agent` | string | no | Agent: `claude` or `codex`. Required when `id` and `title` are omitted and more than one agent is enabled |
| `n` | number | no | Number of turns to return (default 20). Only applies to the turns section |
| `turns` | boolean | no | Return the session turns (default `true`) |
| `plan` | boolean | no | Return the session plan (default `true`) |
| `diff` | boolean | no | Return the pre-computed merge-base diff against the inferred base branch, reported as `diff_target` (default `true`) |
| `uncommitted_diff` | boolean | no | Return the live `git diff HEAD` in the session's own working tree (default `false`) |
| `request_id` | string | no | Pagination request ID from a previous response |

**`session_list`** Lists all sessions. Returns session ID, agent, title, title source (`custom` | `index` | `derived`), last activity timestamp, whether a plan or diff is available, the inferred diff base (`diff_target`), and session metadata (cwd, git branch, model, origin).

| Param | Type | Required | Description |
|-------|------|----------|-------------|
| `agent` | string | no | Agent: `claude` or `codex`. Lists all sessions when omitted |

## Supported agents

| Agent | Session path |
|-------|-------------|
| Claude Code | `~/.claude/projects/<encoded-cwd>/*.jsonl` |
| Codex CLI | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` |

On Windows the session roots resolve to `%USERPROFILE%\.claude` and `%USERPROFILE%\.codex`.

### Agent parity

| Capability | Claude Code | Codex |
|---|---|---|
| Title | explicit custom titles | session index thread names |
| Plan | plan-mode plan file (watched live) | latest `proposed_plan` block |
| Git metadata | branch per entry | branch, commit hash, repo URL from `session_meta` |
| Client metadata | CLI version | originator, CLI version, source, fork lineage |
| Model | per assistant message | per turn context |
| Token usage | summed per message | cumulative snapshots parsed; accurate totals pending (usage_reporting concept) |
| Tool calls | filtered out | filtered out |
| Sub-agent sessions | hidden (sidechains) | hidden (sub-agent rollouts) |
| Pagination | by client capability | by client capability |

## Installation

```bash
go install github.com/kevinhorst/peek-mcp@latest
```

Or build from source:

```bash
git clone https://github.com/kevinhorst/peek-mcp
cd peek-mcp
go build -o peek-mcp .
```

### Windows

Download and run [peek-mcp-setup.exe](https://github.com/kevinhorst/peek-mcp/releases/latest)
— a wizard that installs the binary, configures Claude Code and/or Codex CLI, lets you
enable or disable the control server dashboard (default on), and optionally adds
peek-mcp to your PATH. The finish page starts peek-mcp (a console window stays open)
and opens the dashboard. Uninstalling removes the binary but leaves your agent
configs untouched.

For a manual install, download `peek-mcp-windows-amd64.exe` (or `-arm64.exe`),
rename it to `peek-mcp.exe`, and place it on your `PATH`. If SmartScreen warns,
choose **More info → Run anyway**, or unblock it in PowerShell:

```powershell
Unblock-File peek-mcp.exe
```

## Quick setup

```bash
peek-mcp
```

Running `peek-mcp` with no arguments launches an interactive wizard that writes the correct config for your environment (Claude Code, Codex CLI, or both). It detects existing configs and merges without destroying other keys.

Non-interactive (used by the Windows installer, works everywhere):

```bash
peek-mcp setup --claude --codex --control-server=false
```

## Usage

```bash
peek-mcp start
```

Starts the MCP server on `http://localhost:4242/mcp` by default.

```bash
peek-mcp start --port 4242 --depth 20
```

### Flags

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
| `--control-port` | `42442` | Control server start port; walks up to `42499` if taken (dashboard + JSON API + SSE); `0` disables |
| `--control-token` | — | Optional bearer token protecting the control server |

### Environment variables

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

## Connecting to Claude Chat

```bash
claude mcp add peek-mcp http://localhost:4242/mcp --transport http
```

## Connecting to Claude Code

Add to `.claude/settings.json` in your project:

```json
{
  "mcpServers": {
    "peek-mcp": {
      "type": "http",
      "url": "http://localhost:4242/mcp"
    }
  }
}
```

## Connecting to Codex

Add to `~/codex/config.toml`:

```toml
[mcp_servers.peek-mcp]
command = "/Users/kevinpersonal/GolandProjects/peek-mcp/dist/peek-mcp"
args = ["start", "--transport=stdio", "--depth=100", "--claude-home=/Users/kevinpersonal/.claude", "--codex-home=/Users/kevinpersonal/.codex"]
```

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

## Example workflow

1. Start peek-mcp in a terminal tab. It runs silently and watches for sessions.
2. Run Claude Code with Opus on a task.
3. Open Claude Chat (Sonnet) and ask: "Use session_get to review what was just built and flag any issues."
4. Sonnet calls the tool, reads the last 20 turns, the current plan, and the git diff against `main`. Done in under 30 seconds.

## Limitations

- The `diff` and `uncommitted_diff` sections of `session_get` require a local `git` binary (≥ 2.30, for `git diff --merge-base`) in `PATH` and runs in the session's working directory. It produces no output if the directory is not a git repository.
- Codex CLI sessions do not currently expose token usage metadata.
- The stdio transport is intended for Claude Desktop use via `.mcpb`. Running it manually requires the client to manage the process lifecycle.

## Requirements

- Go 1.26+
- macOS, Linux, or Windows
- Claude Code and/or Codex CLI installed (peek-mcp reads their output; it does not depend on them at runtime)

## License

MIT
