# User Stories: Peek Control Server

---

## Live dashboard

**As a** developer running several agent sessions, **I want** a browser dashboard listing all sessions with live updates, **so that** I can monitor agents without polling MCP tools from another agent.

**As a** developer, **I want** to click into a session and see its recent turns, plan, and diff side by side, **so that** I can review what an agent is doing at a glance.

**As a** Claude Desktop user (stdio transport), **I want** the same dashboard to work, **so that** the feature doesn't depend on how Peek was launched.

---

## Scriptability

**As a** power user, **I want** a plain JSON API (`curl ... | jq`), **so that** I can build my own tooling on top of Peek's session store.

**As a** power user, **I want** an SSE event stream (`curl -N /api/events`), **so that** my scripts can react to new turns, plan changes, and diff updates without polling.

---

## Safety

**As a** security-conscious user, **I want** the control server off by default and bound to localhost only, **so that** enabling Peek never silently exposes my transcripts.

**As a** security-conscious user, **I want** protection against DNS rebinding and no CORS access, **so that** websites I visit cannot read my session data through the local API.

**As a** cautious operator, **I want** an optional access token, **so that** other local processes can't read transcripts without it.

---

## Session management (backlog)

**As a** Peek user, **I want** to rename sessions from the dashboard, **so that** title-based lookup works with my own naming scheme.
