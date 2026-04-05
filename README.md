# peek-mcp

A lightweight MCP server that reads Claude Code and Codex CLI sessions directly from disk and exposes them over HTTP, so a second model can evaluate what a primary agent produced without spending tokens on summarization.

## The problem

Opus finishes a task. I often are in the situation that I want to have a quick follow-up question or analysis on the output, which, when Opus or another bigger model is prompted, eats up valuable tokens and especially much more time than necessary. So I want to use Sonnet or GPT-5-mini to review it immediately, but copying the context by hand is cumbersome and time consuming. 

As of the 05.04.2026, there seems to be (could be wrong) no way to do cross-session communication with the tools provided by Anthropic or OpenAI and definetly not between both of them without any MCP magic. 

There seem to be some MCP servers that kinda, maybe do what I need, but they seemed a bit bloated, so I wrote my own, which is more tailored to my current workflow.

I wanted to avoid any interruption in said workflow, so an approach where the agent pushes to an MCP was ruled out. 
The session files are on disk, so I figured that should be a good starting point.

## The solution

peek-mcp watches the session files that Claude Code and Codex write to disk automatically, parses them passively, and serves the last N turns via MCP. Any connected client calls `session_latest` and quickly gets the context it needs.

```
Claude Code / Codex writes JSONL to disk (always, no configuration needed)
                    |
             fsnotify file watcher
                    |
          in-memory ring buffer per session
                    |
        MCP server over streamable HTTP
                    |
    Sonnet / GPT-5-mini calls session_latest(5)
```

## MCP Tools

**`session_latest(n?)`**
Returns the last N human/assistant turn pairs from the most recently active session. Defaults to 5. Tool calls and tool results are filtered out.

**`session_list()`**
Lists all known sessions with metadata: session ID, working directory, git branch, last activity timestamp, total token usage, and model.

**`session_get(id, n?)`**
Returns the last N turns from a specific session by ID.

## Supported agents

| Agent | Session path |
|---|---|
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
peek-mcp
```

Starts the MCP server on `http://localhost:4242/mcp` by default.

```bash
peek-mcp --port 4242 --depth 20
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--port` | `4242` | HTTP port |
| `--depth` | `20` | Ring buffer depth per session (max turns kept) |
| `--claude-home` | `~/.claude` | Override Claude Code session root |
| `--codex-home` | `~/.codex` | Override Codex session root |

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

## Example workflow

1. Start peek-mcp in a terminal tab. It runs silently and watches for sessions.
2. Run Claude Code with Opus on a task.
3. Open Claude Chat (Sonnet) and ask: "Use session_latest to review what was just built and flag any issues."
4. Sonnet calls the tool, reads the last 5 turns, responds. Done in under 30 seconds.

## Requirements

- Go 1.22+
- macOS or Linux
- Claude Code and/or Codex CLI installed (peek-mcp reads their output, it does not depend on them at runtime)

## License

MIT
