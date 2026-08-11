# peek-mcp

A lightweight MCP server that reads Claude Code and Codex CLI sessions directly from disk and serves them over MCP — so any other model or agent can see what an agent did, live, without spending tokens on summarization or copying context by hand.

Agents already write their sessions to disk as JSONL. peek-mcp watches those files, parses them passively, and exposes the last N turns, the plan, and the git diff to any connected client. There is nothing to push and no workflow to interrupt — a second model just calls a tool and gets the context it needs.

## Use cases

| Use case | You want to… |
|---|---|
| [Model handoff](docs/use-cases/model-handoff.md) | have a cheap model review what an expensive one built |
| [Compaction preventer](docs/use-cases/compaction-preventer.md) | continue in a fresh session instead of compacting a full one |
| [Agent orchestration](docs/use-cases/agent-orchestration.md) | supervise several worker sessions from one place |
| [Cross-agent communication](docs/use-cases/cross-agent-communication.md) | let Claude Code and Codex read each other's work |
| [Session analysis](docs/use-cases/session-analysis.md) | mine past sessions for retrospectives |

## How it works

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

Alongside turns, peek-mcp passively watches two more sources:

- **Plans** — Claude Code writes a plan file per task; peek-mcp stores it with the session so `session_plan` and `session_full` surface it. For Codex, the plan is the latest `proposed_plan` block.
- **Git diffs** — after each new turn, peek-mcp infers the session branch's base and runs `git diff --merge-base <base>` in the session's working directory. `session_diff` and `session_full` expose the result; the resolved base is reported as `diff_target`.

## Quick start

Install:

```bash
go install github.com/kevinhorst/peek-mcp@latest
```

Or grab a prebuilt binary from the [latest release](https://github.com/kevinhorst/peek-mcp/releases/latest) (Windows setup: see the [reference](docs/reference.md#windows)).

Run the interactive setup wizard — it detects your environment and writes the correct config for Claude Code, Codex CLI, or both, merging without destroying other keys:

```bash
peek-mcp
```

Or start the server directly:

```bash
peek-mcp start
```

This serves the MCP endpoint on `http://localhost:4242/mcp` by default.

Connect it to a client:

```bash
# Claude Chat
claude mcp add peek-mcp http://localhost:4242/mcp --transport http
```

```json
// Claude Code — .claude/settings.json
{
  "mcpServers": {
    "peek-mcp": { "type": "http", "url": "http://localhost:4242/mcp" }
  }
}
```

```toml
# Codex — ~/codex/config.toml
[mcp_servers.peek-mcp]
command = "/absolute/path/to/peek-mcp"
args = ["start", "--transport=stdio"]
```

## Tools

One call — `session_full` — usually does the job; it bundles turns, plan, and diff. The rest are there when you want one piece.

| Tool | Returns |
|---|---|
| `session_full` | turns + plan + diff for a session, in one call |
| `session_latest` | last N turns from the most recently active session |
| `session_list` | all sessions with metadata (branch, model, activity, diff base) |
| `session_get` | last N turns from a session by id or title |
| `session_plan` | the current plan for a session |
| `session_diff` | the precomputed git diff against the inferred base branch |
| `session_uncommitted_diff` | the live `git diff HEAD`, refreshed as files are saved |

Full parameters, title-matching and pagination rules, supported agents, and the Claude/Codex parity table: [docs/tools.md](docs/tools.md).

## Dashboard

Run with `--control-port` and peek-mcp serves a live, read-only dashboard on loopback — session list, turns, plan, and diffs, updating as agents work. It doubles as a scriptable JSON API. See [docs/reference.md](docs/reference.md#control-server-dashboard--json-api).

![peek-mcp dashboard — session list](docs/assets/dashboard-sessions.png)

## More

- [Tool reference](docs/tools.md) — every parameter, supported agents, agent parity
- [Operations reference](docs/reference.md) — flags, env vars, control server, hot reload, `.mcpb`, Windows
- [Contributing](CONTRIBUTING.md)

## License

MIT
