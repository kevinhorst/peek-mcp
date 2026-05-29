# peek-mcp

A lightweight MCP server that reads Claude Code and Codex CLI sessions directly from disk and exposes them over HTTP or stdio, so a second model can evaluate what a primary agent produced without spending tokens on summarization.

## The problem

Opus finishes a task. I am quite often in the situation that I want to have a quick follow-up question or analysis on the output. If I then prompt Opus or another bigger model, it eats up valuable tokens and especially much more time than necessary. So I want to use Sonnet or GPT5-mini to review it quickly, but copying the context by hand is cumbersome.

As of 05.04.2026 (update: 10.05.2026 - still nothing), there seems to be no other way to do cross-session communication between different integrations than to either copy or prompt the model to read the respective session directory directly, which works, but is also slow.

There seem to be some MCP servers that kinda, maybe do what I need, but they did not quite fit my case, so I wrote my own, which is more tailored to my current workflow.

I wanted to avoid any interruption in said workflow, so an approach where the agent pushes to an MCP was ruled out. The session files are on disk, so I figured that should be a good starting point and took it from there. It is also an experiment for a codebase with heavy use of agentic development (but not vibe coding).

## The solution

peek-mcp watches the session files that Claude Code and Codex write to disk automatically, parses them passively, and serves the last N turns via MCP. Any connected client calls `session_latest` or `session_full` and quickly gets the context it needs.

```
Claude Code / Codex writes JSONL to disk (always, no configuration needed)
                    |
             fsnotify file watcher
                    |
          in-memory buffer per session
                    |
        MCP server over streamable HTTP or stdio
                    |
    Sonnet / GPT-5-mini calls session_full(n)
```

In addition to turns, peek-mcp passively watches two more sources:

- **Plans** — Claude Code writes a plan file to `~/.claude/plans/` at the start of each task. peek-mcp reads and stores it alongside the session so `session_plan` and `session_full` can surface it without any extra prompting.
- **Git diffs** — After each new turn, peek-mcp runs `git diff <target-branch>` in the session's working directory and caches the result. `session_diff` and `session_full` expose this so a reviewer model can see exactly what changed without reading source files.

## MCP Tools

**`session_full(id?, n?, diff_size?)`** Returns turns, plan, and git diff for a session in one call. Prefer this over calling `session_latest`, `session_plan`, and `session_diff` separately. Omit `id` to use the most recently active session.

**`session_latest(n?)`** Returns the last N human/assistant turn pairs from the most recently active session. Defaults to 5. Tool calls and tool results are filtered out.

**`session_list()`** Lists all known sessions with metadata: session ID, last activity timestamp, and whether a plan or diff is available.

**`session_get(id, n?)`** Returns the last N turns from a specific session by ID.

**`session_plan(id?)`** Returns the current plan for a session. Returns an empty response if the session has no plan. Omit `id` to use the most recently active session.

**`session_diff(id?, size?)`** Returns the pre-computed git diff for a session, run against the configured target branch and refreshed automatically on each new turn. `size` limits the response to N bytes (0 = no limit). Omit `id` to use the most recently active session.

**`session_uncommitted_diff(id?, size?)`** Returns the live uncommitted git diff (`git diff HEAD`, i.e. staged + unstaged changes) for a session, refreshed continuously as files are saved — not only on new turns. Resolved in the session's own working tree, so it is correct inside linked git worktrees. `size` limits the response to N bytes (0 = no limit). Omit `id` to use the most recently active session. See [Hot reload](#hot-reload-live-diff) for injecting this into Claude Code automatically.

## Supported agents

| Agent | Session path |
|-------|-------------|
| Claude Code | `~/.claude/projects/<encoded-cwd>/*.jsonl` |
| Codex CLI | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` |

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
| `--diff-target` | `main` | Branch to diff against for `session_diff` |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |

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

A separate observer session can read the same diff via the `session_uncommitted_diff` tool.

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
3. Open Claude Chat (Sonnet) and ask: "Use session_full to review what was just built and flag any issues."
4. Sonnet calls the tool, reads the last 5 turns, the current plan, and the git diff against `main`. Done in under 30 seconds.

## Limitations

- `session_diff` requires a local `git` binary in `PATH` and runs in the session's working directory. It will produce no output if the directory is not a git repository or the target branch does not exist.
- Codex CLI sessions do not currently expose token usage metadata.
- The stdio transport is intended for Claude Desktop use via `.mcpb`. Running it manually requires the client to manage the process lifecycle.

## Requirements

- Go 1.26+
- macOS or Linux
- Claude Code and/or Codex CLI installed (peek-mcp reads their output; it does not depend on them at runtime)

## License

MIT