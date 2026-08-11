# peek-mcp use-case documentation + README refinement — Implementation Plan

## TLDR

- Create a `docs/` tree with five use-case walkthroughs (session handoff to a smaller model, compaction preventer, agent orchestration, cross-agent communication, session analysis), each with a real example captured from live tool calls and dashboard screenshots where applicable.
- Rewrite README.md to be short and to the point: one-liner, use-case index up front, quick start, compact tool table — reference material (full tool params, flags, control server API, hot reload, .mcpb) moves to `docs/`.
- All example outputs are captured from real sessions via the connected peek MCP tools — nothing invented.
- Screenshots ship in this change; GIFs are deferred — Kevin records real interactions later (D6).

## Context

- peek-mcp's README is the only documentation: 311 lines, seven per-tool parameter tables, a dated problem narrative ([README.md:5](README.md)), and no use-case guidance — a new user cannot see *how* to use it.
- The request names five concrete use cases to document with examples and visuals, and asks for a simpler, briefer README.
- No `docs/` directory or image assets exist anywhere in the repo (F2).
- The control-server dashboard (F3) is the only screenshot-able surface; terminal interactions have no recording tooling installed (F5).
- Doc style is bound by the user's standing preference: brief, use-case-first, navigable, minimal inline code, English.

## Scope

- **In:**
  - **use-case docs:** five walkthrough pages under `docs/use-cases/`
  - **tool reference:** `docs/tools.md` holding the full per-tool parameter tables
  - **operations reference:** `docs/reference.md` holding flags, env vars, control server, hot reload, .mcpb, Windows notes
  - **README rewrite:** compact, use-case-first, links into `docs/`
  - **screenshots:** dashboard captures under `docs/assets/`
  - **example capture:** real tool outputs via the connected peek MCP tools
- **Out:**
  - **code changes:** no Go code, no tool behavior, no dashboard changes
  - **videos:** ruled out — GitHub README cannot embed video without external hosting; GIFs cover the animated case (D6)
- **Not changed:**
  - **CONTRIBUTING.md content:** only its README pointer is updated if the supported-agents table moves (see Contracts & sweeps)
  - **plans/ historical docs:** stale README references in past plans stay as-is
- **Deferred findings:**
  - **README "Examples: TBD" line** ([README.md:14](README.md)) — resolved by this change (the use-case docs are those examples), noted for completeness
  - **GIF recordings:** deferred per [D6](#d6) — Kevin records real interactions in a later change

## Assumptions

| Assumption | Reality | Location |
|---|---|---|
| "screenshots, maybe videos, gifs" implies recording tooling exists | No vhs/asciinema/agg installed; only the dashboard is screenshot-able without new tooling | F5 |
| Examples can be produced during implementation | peek MCP tools are connected in this session and real sessions exist on this machine | F6 |

## Decisions

| ID | Problem | Facts | Decision | Why |
|---|---|---|---|---|
| <a id="d1"></a>D1 | Where use-case docs live | [F1!](#f1), [F2!](#f2) | Compact "Use cases" index in README + one page per use case under `docs/use-cases/` | Navigable beats monolithic: README stays brief (the explicit ask) while each use case gets room for a full walkthrough; matches the standing doc-style preference (use-case-first, navigable) |
| <a id="d2"></a>D2 | README carries 7 param tables + flags + env + control-server + hot-reload + mcpb detail — incompatible with "brevity" | [F1!](#f1) | Move full tool params to `docs/tools.md`, operational reference to `docs/reference.md`; README keeps a one-row-per-tool summary table and quick start only | Single source of truth per topic — README links, never duplicates; reference material is looked up, not read linearly, so it doesn't pay rent on the front page |
| <a id="d3"></a>D3 | Dated problem narrative with update log (05.04 / 10.05 / 07.06) reads as a journal, not docs | [F1!](#f1) | Replace with a 3-sentence problem statement; delete the dated updates and "Examples: TBD" | The history serves the author, not the reader; the use-case docs supersede "Examples: TBD" |
| <a id="d4"></a>D4 | Example content could be written from memory and drift from actual tool behavior | [F6!](#f6) | Every example prompt/output in the docs is captured from a live tool call against a real session during implementation, then trimmed | A field name is a hypothesis until seen — invented JSON in docs is the classic drift source |
| <a id="d5"></a>D5 | No exemplar exists for doc pages (first docs/ in repo) | [F2!](#f2) | Write `docs/use-cases/model-handoff.md` first to the template in [§3](#c3); the remaining four mirror it | The first page becomes the in-repo exemplar; template fixed in this plan so all five stay parallel |
| <a id="d6"></a>D6 | Visual media beyond dashboard screenshots | [F3!](#f3), [F5!](#f5) | [USER] Screenshots + fenced real transcript excerpts now; GIFs deferred to a later change (recorded by Kevin) | No recording tooling installed and honest GIFs need real model interactions; docs are written so GIF slots can be added later without restructuring |

## Baseline (verified)

Base branch: `main` (worktree branch `claude/peek-documentation-usecases-8587da`, clean).

| ID | Fact | Needed for | Location |
|---|---|---|---|
| <a id="f1"></a>F1! | README is 311 lines: 7 per-tool param tables, flags/env tables, control-server/hot-reload/mcpb sections, dated problem log, "Examples: TBD" | [D1](#d1), [D2](#d2), [D3](#d3) | [README.md](README.md) |
| <a id="f2"></a>F2! | No `docs/` directory and no image assets exist in the repo | [D1](#d1), [D5](#d5) | repo root listing |
| <a id="f3"></a>F3! | Control-server dashboard exists and serves live session list/turns/plan/diff on loopback | [D6](#d6), [§9](#c9) | [control/server.go](control/server.go), [README.md:199](README.md) |
| <a id="f5"></a>F5! | vhs, asciinema, agg all absent from PATH | [D6](#d6) | `which` check |
| <a id="f6"></a>F6! | peek MCP tools (session_list, session_full, session_get, session_plan, session_diff, session_uncommitted_diff, session_latest) are connected in this session against real local sessions | [D4](#d4) | session tool roster |
| <a id="f7"></a>F7 | CONTRIBUTING.md step 4 points at "the README's supported agents table" | Contracts & sweeps | [CONTRIBUTING.md:109](CONTRIBUTING.md) |
| <a id="f8"></a>F8 | Hot-reload hook snippet ships at hooks/settings.snippet.json and is embedded in README | [§8](#c8) | [README.md:248](README.md) |

## Exemplar & reuse

| Existing | Used for |
|---|---|
| README tool/flag/env tables ([README.md:43-197](README.md)) | Moved verbatim (content-identical) into docs/tools.md and docs/reference.md — not rewritten |
| README control-server / hot-reload / mcpb sections | Moved into docs/reference.md with only heading-level adjustments |
| Existing "Example workflow" ([README.md:289](README.md)) | Seed for the model-handoff use case; superseded and deleted from README |

- No doc-page exemplar exists in the repo — [D5](#d5) makes `docs/use-cases/model-handoff.md` the exemplar the other four pages mirror; this is the plan's main consistency risk.

## Changes

Dependency order: capture assets → use-case pages → reference pages → README rewrite.

### 1. Capture example material (new, working assets)
<a id="c1"></a>
location: `docs/assets/*.png`, scratch captures

- **Dashboard screenshots** (per [F3](#f3)): start `peek-mcp start` locally, open the dashboard in the in-app browser, capture:
  - session list view → `docs/assets/dashboard-sessions.png`
  - session detail with turns + diff → `docs/assets/dashboard-session-detail.png`
- **Tool-output captures** (per [D4](#d4)): one real call per documented tool; trim to ≤ 25 lines per excerpt, redact nothing but long diff bodies (`…` marker).
- Stop condition S7 applies if no suitable real session exists for a use case.

### 2. Use-case page template (new)
<a id="c2"></a>
location: fixed by this plan, instantiated per page

Every page follows exactly this skeleton (headings verbatim):

```markdown
# <Use case title>

<One-sentence promise: what you get, in whose situation.>

## When

<2–4 bullets: the situation and pain, no product pitch.>

## Setup

<Only what this use case needs beyond the Quick start; link README quick start instead of repeating it.>

## Walkthrough

<Numbered steps. Each tool interaction shows:
the prompt given to the model (verbatim),
the tool call it makes,
a trimmed real output excerpt in a fenced block.>

## What to expect

<2–3 bullets: limits, timing, gotchas for this flow.>
```

### 3. Model handoff use case (new)
<a id="c3"></a>
location: `docs/use-cases/model-handoff.md`
mirrors: template in [§2](#c2) — this page is the exemplar for §4–§7

- **Scenario:** Opus/Fable finishes a task in Claude Code; a cheaper model (Sonnet in Claude Chat, GPT-5-mini in Codex) reviews it via `session_full` without re-prompting the big model.
- **Walkthrough:** start server → big-model session ends → in the cheap client: "Use session_full to review what was just built and flag issues" → tool returns turns + plan + diff → real trimmed output.
- Absorbs and replaces the README "Example workflow" section.
- Includes `docs/assets/dashboard-session-detail.png` showing the same session the walkthrough reads.

### 4. Compaction preventer use case (new)
<a id="c4"></a>
location: `docs/use-cases/compaction-preventer.md`
mirrors: §3

- **Scenario:** context window near full; instead of compacting, start a fresh session and rehydrate from the old one.
- **Walkthrough:** old session hits ~limit → new session's first prompt: "Use session_get with title '<old title>' to load the last 30 turns and the plan, then continue the task" → fresh session continues with full-fidelity context instead of a lossy compaction summary.
- **What to expect** covers: ring-buffer depth bounds how far back rehydration reaches (`--depth`), and the plan/diff arrive uncompacted.

### 5. Agent orchestration use case (new)
<a id="c5"></a>
location: `docs/use-cases/agent-orchestration.md`
mirrors: §3

- **Scenario:** one orchestrator session supervises several worker sessions (worktrees/terminals) without pasting anything between them.
- **Walkthrough:** `session_list` to enumerate workers with branch + activity → per worker `session_get` (progress) and `session_diff` (produced code) → orchestrator decides who continues/stops.
- Includes `docs/assets/dashboard-sessions.png` as the human-eye equivalent of `session_list`.

### 6. Cross-agent communication use case (new)
<a id="c6"></a>
location: `docs/use-cases/cross-agent-communication.md`
mirrors: §3

- **Scenario:** Claude Code and Codex CLI working the same repo read each other's sessions via the `agent` parameter — no copy-paste bridge.
- **Walkthrough:** Codex implements → Claude Code: "Use session_full with agent=codex to see what Codex changed and review the diff" → and the reverse direction with `agent=claude`.
- **What to expect** covers the parity differences that matter here (titles from Codex's index, plans as `proposed_plan` blocks) with a link to the parity table in `docs/tools.md`.

### 7. Session analysis use case (new)
<a id="c7"></a>
location: `docs/use-cases/session-analysis.md`
mirrors: §3

- **Scenario:** retrospectives/mining over past sessions — what was asked, what was built, where time went — without opening JSONL files.
- **Walkthrough:** `session_list` to pick candidates → `session_full` per session for turns + plan + final diff → the analyzing model extracts patterns (rejected approaches, repeated corrections).
- **What to expect** covers: sub-agent sessions are hidden, tool calls are filtered out, depth bounds history.

### 8. Reference pages (new)
<a id="c8"></a>
location: `docs/tools.md`, `docs/reference.md`

- **docs/tools.md** (per [D2](#d2)):
  - the seven full per-tool parameter tables, moved content-identical from README
  - the supported-agents table and the agent-parity table
  - pagination and title-matching semantics (the prose currently inline in the tool descriptions)
- **docs/reference.md**:
  - flags table, env-var table
  - control server section (dashboard, JSON API, curl examples, token auth)
  - hot reload hook (full snippet, per [F8](#f8))
  - .mcpb build + install, Windows install/SmartScreen notes, limitations, requirements
- Both pages open with a one-line breadcrumb link back to the README.

### 9. README rewrite (modified)
<a id="c9"></a>
location: `README.md`

Target structure (~120 lines, down from 311):

```markdown
# peek-mcp

<one-liner: reads Claude Code / Codex sessions from disk, serves them over MCP —
so any other model or agent can see what an agent did, live, without copy-paste.>

<3-sentence problem statement (per D3) — no dates, no journal>

## Use cases

| Use case | You want to… |
|---|---|
| [Model handoff](docs/use-cases/model-handoff.md) | have a cheap model review what an expensive one built |
| [Compaction preventer](docs/use-cases/compaction-preventer.md) | continue in a fresh session instead of compacting |
| [Agent orchestration](docs/use-cases/agent-orchestration.md) | supervise several worker sessions from one place |
| [Cross-agent communication](docs/use-cases/cross-agent-communication.md) | let Claude Code and Codex read each other's work |
| [Session analysis](docs/use-cases/session-analysis.md) | mine past sessions for retrospectives |

## How it works

<existing ASCII pipeline diagram, kept; plans + diffs paragraph compressed to 2 bullets>

## Quick start

<install (go install + release link), wizard, start, connect snippets for
Claude Chat / Claude Code / Codex — kept, this is the golden path>

## Tools

<one-row-per-tool summary table: tool | what it returns; link to docs/tools.md>

## Dashboard

<2 sentences + docs/assets/dashboard-sessions.png + link to docs/reference.md>

## More

<links: docs/tools.md, docs/reference.md, CONTRIBUTING.md>

## License

MIT
```

- Deleted from README (moved, per [D2](#d2)): per-tool param tables, flags/env tables, control-server detail, hot reload, .mcpb section, Windows install detail, limitations, agent-parity table, example workflow.
- Final README text is written at implementation following this structure; the use-case table above is the final content.

### 10. CONTRIBUTING pointer (modified)
<a id="c10"></a>
location: `CONTRIBUTING.md:109`

```diff
-4. Document the session path in the README's supported agents table.
+4. Document the session path in the supported agents table in docs/tools.md.
```

## Hot items

N/A — documentation-only change, no code in hot classes.

## Tests

N/A — no code changes. Doc correctness is covered by the Verification checklist (live tool calls compared against documented behavior, link check).

## Test runbook

N/A — no callable surface changes; the use-case walkthroughs themselves are the executable scenarios and are verified live in Verification.

## Contracts & sweeps

| Contract | Sides | Sweep |
|---|---|---|
| README section anchors (tools, flags, supported agents) | README, CONTRIBUTING.md, external links unknown | `grep -rn "README" --include=*.md --include=*.go --include=*.json` excluding vendor/ and plans/ — only [F7](#f7) hit; fix per [§10](#c10) |
| Tool names/params in docs | docs/tools.md, docs/use-cases/*, actual MCP schema | every documented param verified against a live call ([D4](#d4)) |
| Image paths | README, use-case pages, docs/assets/ | all `docs/assets/` references resolve to committed files |

## Verification

- [ ] Run `peek-mcp start` locally — dashboard reachable, screenshots captured from the live instance, not mockups.
- [ ] For each of the five walkthroughs, execute the documented tool call verbatim — the real response matches the documented excerpt shape (field names, structure).
- [ ] Call `session_get` with a title and `session_full` with `agent=codex` exactly as written in the cross-agent and compaction pages — expect non-error responses.
- [ ] Run a markdown link check over README.md and docs/ (relative links + image paths) — zero broken links.
- [ ] Confirm README is ≤ ~130 lines and contains no parameter table, flag table, or hook snippet.
- [ ] Confirm no content was lost in the move: every README section deleted in [§9](#c9) exists in docs/tools.md or docs/reference.md.
- [ ] `grep -rn "Examples: TBD"` — zero hits.
- [ ] Degenerate case: a use-case page read standalone (direct link) still orients the reader — breadcrumb to README present on every docs page.

## Stop conditions

| ID | Condition | Action |
|---|---|---|
| S1 | An approved signature/contract can't hold as planned | Stop and report — never improvise mid-edit |
| S2 | Second failed fix on the same mechanism | Stop, research actual cause, redesign — no third band-aid |
| S3 | Missing prerequisite (generated code, running infra) | Run the producing step; if infra is down, ask — never skip validation or start infra unasked |
| S4 | Discovered work materially exceeds approved scope | Ask before continuing |
| S5 | Same kind of bug found twice: in own diff → fix all in diff; pre-existing outside → report and ask | Per condition |
| S6 | Structural obstacle tempts a new abstraction | Stop and report — relocate, don't indirect |
| S7 | No real session exists that can produce the example a walkthrough needs (e.g. no Codex session for cross-agent) | Stop and report — never fabricate tool output for docs |
| S8 | A live tool call contradicts what the current README claims about that tool | Fix the doc to match reality and report the discrepancy — never paper over it |

## Open questions

Empty — all decisions resolved.

## Changelog

| Date | Trigger | What changed |
|---|---|---|
| — | initial | plan created |
| 2026-08-11 | Q: GIF/media scope | D6 answered [USER]: screenshots now, GIFs later; GIF recording added to Deferred findings |
