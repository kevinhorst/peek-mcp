# Concept: Peek Projects, Chat and Cloud

> **Status:** Draft
> **Author:** Kevin Horst
> **Date:** 2026-08-30

---

## Goals

- Claude Desktop project (Cowork) sessions are ingested and listed — today their transcripts are invisible to Peek because each session runs in a private claude-home outside `~/.claude`.
- Sessions expose a `project` field, and `session_list` accepts a `project` filter param, uniformly across agents (empty for repo sessions).
- Chat (claude.ai / Desktop chat) and Cloud (claude.ai/code) integration is evaluated and the verdict documented with evidence — a decision record, not an open gap.

---

## Findings

Verified on disk and against the running Peek instance (2026-08-30).

### Where desktop project sessions live

Two generations of Desktop session storage exist:

| Generation | Working dir | Transcript location | Peek coverage today |
|---|---|---|---|
| Legacy desktop session (dead generation — single sample, v2.1.205) | `~/Claude_Unassigned` | `~/.claude/projects/-Users-…-Claude-Unassigned/<id>.jsonl` | Covered (appears in `session_list`) |
| Cowork / local agent mode (current) | `…/local_<id>/outputs` inside the store | `~/Library/Application Support/Claude/local-agent-mode-sessions/<account-uuid>/<org-uuid>/local_<id>/.claude/projects/<slug>/<sessionId>.jsonl` | **Not covered — outside the watch root** |

The store directory is being renamed by Anthropic from `local-agent-mode-sessions` to `claude-code-sessions`; both names can coexist on one machine (they do here — the latter currently holds only metadata for Desktop-dispatched repo sessions). The path segments are the user's `accountUuid` and `organizationUuid` (verified against `~/.claude.json` `oauthAccount`) — **there is no project segment and no project field in any of the 481 session metadata files on disk**. Project grouping (`claude_project_id`) exists only server-side. The legacy layout is the only generation where a project name (`Claude_<Name>` folder) ever reached disk.

The Cowork store gives every session its own claude-home. Layout per session dir `local_<id>/`:

- `.claude/projects/<slug>/<sessionId>.jsonl` — standard Claude Code transcript (same entry schema as repo sessions; `queue-operation` entries at the top, `gitBranch: "HEAD"`, non-git cwd).
- `audit.jsonl` — HMAC-signed audit log (own schema: `_audit_hmac`, `message`, `session_id`, …). Not a transcript; must not be ingested as one.
- `outputs/`, `uploads/` — session artifacts.
- Sibling `local_<id>.json` — session metadata: `title`, `cliSessionId` (= transcript filename), `cwd`, `createdAt`, `lastActivityAt`, model/MCP config. No message content.

A parallel store `~/Library/Application Support/Claude/claude-code-sessions/` holds metadata for repo sessions dispatched through Desktop — those transcripts land in `~/.claude/projects` and are already covered.

### Peek's current discovery

The watcher (`watcher/watcher.go`) recursively watches one root per agent (`~/.claude/projects`, `~/.codex/sessions`) and treats every `.jsonl` as a transcript. There is no exclusion logic — Cowork sessions are missed purely because their claude-homes are elsewhere. `session.Meta` has no project notion; `session_list` filters only by `agent`.

---

## User Flows

### Peek at desktop project sessions

**Goals:**
- A Cowork session shows up in `session_list` like any repo session, carrying `project`.
- `session_list(project: "cowork")` narrows to desktop sessions.

**Options:**

**MVP** (~3–4d total)
- Cowork watch roots: watch the store recursively with the existing Claude parser, restricted to files matching `**/.claude/projects/**/*.jsonl` — excludes `audit.jsonl` and any `.jsonl` in `outputs/`/`uploads/`. Wired in `cmd/start.go` as a second Claude watcher. Default root is the macOS path, overridable via a `--cowork-home` flag (mirrors `--claude-home`; also covers the Windows `%AppData%\Claude\…` layout); both store names (`local-agent-mode-sessions`, `claude-code-sessions`) are checked and whichever exists is watched. (~1.5d)
- `Meta.Project` as a coarse label at parse/watch time: `"cowork"` for transcripts under the Cowork store; the folder name (e.g. `"Unassigned"`) for legacy cwds matching `~/Claude_<Name>`; empty and omitted from JSON for repo sessions. Real per-project identity is not derivable locally (see Findings); the label upgrades in place if a name source appears. (~0.5d)
- Expose `project` in `sessionListItem.meta` and add an optional `project` filter param to `session_list` (`tools/tools.go`) with a mirroring `projectParam` in the control API (`control/api.go`). Exact-match filter; `session_get`/`session_events` stay unchanged. (~0.5–1d)
- Verify graceful handling of the transcript deltas: `queue-operation` entries ignored, non-git cwd yields `has_diff: false` without log noise. (~0.5d)

**Backlog**
- Real project names: attach sessions to their claude.ai Project once any local source for `claude_project_id`/names exists (Desktop export, API, or future on-disk mapping). (~1–2d, blocked on source)
- Session titles from the sibling `local_<id>.json` metadata files (Cowork sessions have no `custom-title` transcript entries). (~1d)
- Surface `audit.jsonl` events as session events. (~2d)

**Challenges:**
- The Cowork store is deeply nested and grows a new claude-home per session; fsnotify must pick up newly created `local_<id>/.claude/projects/...` chains, not just new files in known dirs.
- `.jsonl` is no longer a sufficient transcript predicate inside this store (`audit.jsonl`, arbitrary session artifacts).
- Store layout is an undocumented Desktop internal and is mid-rename (`local-agent-mode-sessions` → `claude-code-sessions`, both may coexist); the paths are contract-by-observation.
- No local project identity exists for current Cowork sessions — only the coarse cowork/repo distinction is honest today.

**Approach:**
- Reuse the recursive-watch mechanism (it already handles nested dir creation) and add a per-watcher path filter predicate instead of the bare `.jsonl` suffix check — the only watcher-level change; parser and store stay untouched.
- Treat the project label as a pure function of the transcript path/cwd, computed watcher-side where the path is known, carried on the signal turns like other `Meta` fields.
- Default the store root for macOS, `--cowork-home` for everything else; probe both store names under it.

### Chat & Cloud — evaluation (decision record, no build items)

**Chat (claude.ai conversations, Desktop chat):** not integrable.
- No local transcripts exist — Desktop persists only opaque browser state (Cookies, IndexedDB, Local Storage).
- No public per-user API. The only programmatic access is the Compliance API (`/v1/compliance/apps/chats`), Enterprise-only and admin-scoped — out of Peek's scope as a personal local tool.

**Cloud (claude.ai/code sessions):** not directly integrable.
- Transcripts are server-side only; nothing syncs to disk. No cloud/remote origin markers exist in any local transcript.
- No public API: the Compliance API remote-session endpoints explicitly exclude Claude Code on the web; the CLI's own session-list fetch (see teleport below) is a private, OAuth-gated endpoint.
- The sanctioned bridge is `claude --teleport`: it pulls a cloud session into a normal local session, whose transcript then lands under `~/.claude/projects/` — automatically covered by Peek with zero work.

**Teleport evaluation (tested 2026-08-30, CLI 2.1.247):**
- `--teleport [session]` exists in the desktop-bundled binary (`~/Library/Application Support/Claude/claude-code/2.1.247/…/claude`); the Homebrew CLI (2.1.153) predates it. Interactive/TTY-only — silent no-op without a TTY, refuses `--print`.
- Driven under a pseudo-TTY it reaches "Fetching your Claude Code sessions…", then fails on this machine: "Teleport requires a Claude account — run /login and select 'Claude account with subscription'". `claude auth status` reports `loggedIn: false` — all local usage authenticates through the Desktop app's host auth, which the standalone CLI does not share.
- `--cloud <description>` (create) is interactive-only; `claude -p "msg" --cloud <session-id>` can message an existing cloud session headlessly.
- Binary analysis (2.1.247 strings): teleport pulls the conversation as paged events (`api_teleport_events_fetch`, `teleport_events_page_cap`) and retires the cloud conversation on pull (`retireConversationForTeleportPull`); the CLI tracks an `isTeleported` state flag. Whether that flag persists into the transcript JSONL is the one thing still unverified.
- End-to-end verification (on-disk marker, landing project dir) is blocked on a one-time user-side `claude auth login` — recorded as Open Question 1.

**Revisit triggers:** local transcript sync for cloud sessions appears; a public per-user sessions API appears; the teleport test reveals a stable cloud-origin marker worth labeling.

---

## Decisions / Open Questions

**Decisions:**
- [USER] "Projects" means Claude Desktop app projects (Cowork); goal is first-class exposure with a `project` field, like normal sessions.
- [USER] Chat and Cloud are evaluate-and-document only — no build items; the evaluation above is the deliverable.
- [USER] Teleport is part of the evaluation and was tested to the auth boundary; the remaining end-to-end run is user-assisted.
- Cowork sessions stay `agent: "claude"` — they are Claude sessions with a project attribute, not a third source. Avoids widening the ~15 hardcoded `claude|codex` validation sites; the watcher+parser seam carries the new root.
- The Cowork store carries no project identity: path segments are `accountUuid/organizationUuid` (verified against `~/.claude.json`), and none of the 481 session metadata files hold a project field — `claude_project_id` is server-side only. Sources: on-disk inspection 2026-08-30; store rename and layout corroborated by anthropics/claude-code#69663 and brycewatson.com/blog/09-cowork-session-schema.
- [USER] `Meta.Project` is a coarse label: `"cowork"` for store sessions, the folder name for legacy `~/Claude_<Name>` cwds, empty otherwise. No UUIDs are exposed; the label upgrades in place when a real name source appears.
- The legacy `~/Claude_<Name>` layout is a dead generation (single sample, v2.1.205, last active 2026-07-11; current Cowork sessions use `…/local_<id>/outputs` cwds). The cwd-pattern derivation is kept only as a cheap fallback for existing transcripts, which the current watcher already covers.
- [USER] `project` is exposed on `session_list` only; `session_get`/`session_events` output stays unchanged.
- [USER] Watch-root packaging: macOS store path is the built-in default; a `--cowork-home` flag (like `--claude-home`) overrides it for Windows/Linux/custom setups. Both store names (`local-agent-mode-sessions`, `claude-code-sessions`) are probed under the root.
- Teleport mechanics (evidence: binary strings 2.1.247): paged event fetch, cloud conversation retired on pull, CLI-side `isTeleported` flag. Peek needs no work for coverage — teleported sessions land under `~/.claude/projects`. Labeling them is backlog, gated on Open Question 1.
- `audit.jsonl` is not a transcript and is filtered out at the watcher.
- Raw-UUID exposure (former question 5) is obsolete: the coarse-label decision means no UUIDs reach the API.

**Open Questions:**
1. Teleport end-to-end: after a user-side `claude auth login` (subscription OAuth), does a teleported session's transcript persist a cloud-origin marker (candidate: `isTeleported`), and which project dir does it land in? — *Status: blocked on user-side login; owner [USER]; everything researchable without it is recorded above. Decides only the backlog item "label teleported sessions".*
