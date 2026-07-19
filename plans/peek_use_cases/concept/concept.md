# Concept: Peek Use-Case Documentation

> **Status:** Draft
> **Author:** Kevin Horst
> **Date:** 2026-07-19

---

## Goals

- Public GitHub readers can see, per use case, exactly how peek is used, what it returns, and proof that it works — screenshots or video for every claim.
- Each use case is a reproducible walkthrough grounded in the actual tool surface (params, defaults, limits), not marketing prose.
- Claims are calibrated: "optimal" only where defensible (session analysis, compaction prevention), "usable" elsewhere. Overclaiming to a skeptical developer audience destroys the whole document.
- The final artifact is `docs/use-cases.md` (with media in `docs/assets/`) plus a short README "Use cases" section linking to it. This concept specifies content and the evidence shot-list; writing the docs page is the implementation step.

---

## Use-Case Matrix

| Use case | Claim level | Primary peek surface | Evidence | Detail page |
|---|---|---|---|---|
| Model-switch handoff (Fable → GPT) | Usable, faster than any manual alternative | `session_full` cross-agent | 1 screenshot sequence | [handoff_model_switch](handoff_model_switch.md) |
| Compaction preventer (context full → reset) | Optimal — verbatim state vs lossy summary | `session_full` + plan + diff | 1 video | [handoff_context_reset](handoff_context_reset.md) |
| Orchestration helper | Usable — zero-integration fleet view | `session_list` + `session_get` + `session_diff` | 2 screenshots | [orchestration_helper](orchestration_helper.md) |
| Cross-agent communication | Usable — shared-state observation, honestly scoped | `session_plan` / `session_get` both directions | 2 screenshots | [cross_agent_communication](cross_agent_communication.md) |
| Session analysis | Optimal — everything from one tool surface | `session_get` by title + `session_full` | 1 screenshot + 1 video | [session_analysis](session_analysis.md) |

---

## User Flows

### Model-switch handoff (Fable → GPT)

**Goals:**
- A reader sees a fresh Codex/GPT session continue a Claude session's task without any manual context transfer.

**Options:**

**MVP**
- Walkthrough: Claude session in progress → new Codex session prompts "continue via peek" → `session_full(agent: "claude")` returns turns + plan + diff in one call → Codex continues correctly. (~0.5d)
- Comparison paragraph vs copy-paste and vs re-prompting from scratch. (~0.25d)

**Backlog**
- Reverse direction (Codex → Claude) walkthrough. (~0.25d)

**Challenges:**
- The demo must show the receiving model *acting correctly* on the handoff, not just receiving text — otherwise the proof is hollow.

**Approach:**
- Evidence captures the receiving session's first working turn after the tool call, showing it references the plan and diff content.

**Block total:** ~1d

### Compaction preventer (context window full → reset)

**Goals:**
- A reader understands the reset-instead-of-compact workflow and why verbatim turns + real plan file + real diff beat a model-written summary.

**Options:**

**MVP**
- Walkthrough: session near context limit → hard reset → fresh session calls `session_full` on the dead session → continues with the actual plan and diff as ground truth. (~0.5d)
- Honest limits: `--depth` ring buffer bounds recoverable turns; plan and diff carry the durable state. (~0.25d)

**Backlog**
- Recommended flag/config preset for this workflow (`--depth` sizing guidance). (~0.25d)

**Challenges:**
- Auto-compaction is the built-in default; the doc must argue against it without overreaching — compaction keeps *summarized* full history, peek keeps *verbatim* recent history plus authoritative artifacts.

**Approach:**
- Frame as complementary trade-off table (what each preserves, what each loses), then show the reset flow winning on the artifacts that matter for continuing work.

**Block total:** ~1d

### Orchestration helper

**Goals:**
- A reader sees one supervisor session observing several worker sessions — inventory, progress, output review — with zero agent-side integration.

**Options:**

**MVP**
- Walkthrough: `session_list` fleet inventory (agent, title, branch, model, last-active, HasPlan/HasDiff) → `session_get` drill-down on one worker → `session_diff` to review its output. (~0.5d)
- Passive-architecture note: fsnotify on transcript files, workers need no MCP config at all. (~0.25d)

**Backlog**
- `session_uncommitted_diff` live-monitoring variant (watching a worker mid-edit). (~0.25d)

**Challenges:**
- "Orchestration" overpromises — peek observes, it does not dispatch. The page must define the boundary in the first paragraph.

**Approach:**
- Title the capability "orchestration helper": the observation layer under whatever dispatch mechanism the reader already uses.

**Block total:** ~1d

### Cross-agent communication

**Goals:**
- A reader sees Claude and Codex reading each other's state — both directions — and understands this is shared-state observation, not message passing.

**Options:**

**MVP**
- Direction 1: Claude reads a Codex session's `<proposed_plan>` via `session_plan(agent: "codex")`. (~0.25d)
- Direction 2: Codex reads a Claude session's turns via `session_get(agent: "claude")` — the README reviewer workflow, promoted from "Example workflow" into this page. (~0.25d)
- Read-only caveat stated plainly: no push channel, no inbox; "communication" = both agents observing the same durable artifacts. (~0.25d)

**Backlog**
- Three-party variant (Claude Chat as reviewer over both CLI agents). (~0.25d)

**Challenges:**
- Transcripts can contain secrets pasted into sessions; anything connected to peek can read them.

**Approach:**
- Security note on the detail page: peek is bound to localhost by default and reads only local files the user already owns — but connected clients see everything the transcripts contain.

**Block total:** ~1d

### Session analysis

**Goals:**
- A reader sees that after a session ends, peek retrieves *everything* a retrospective needs — turns, plan, diff, metadata — from one tool surface, addressed by human-readable title.

**Options:**

**MVP**
- Walkthrough anchored on a real session: `session_get(title: "[Peek, Concept, F-H] Deep Analysis")` — exact-then-substring title match — then `session_full` for turns + plan + diff. (~0.5d)
- Comparison vs reading raw JSONL (format churn, sidechain filtering, tool-noise stripping all handled by peek). (~0.25d)
- Position as the foundation for retrospective/session-mining workflows (batch analysis over `session_list`). (~0.25d)

**Backlog**
- Worked example of a multi-session analysis sweep. (~0.5d)

**Challenges:**
- The "optimal" claim needs a visible counterfactual — what raw-JSONL consumption looks like — without turning the page into a rant.

**Approach:**
- One compact before/after: a raw JSONL excerpt (unreadable) next to the peek response (readable), then the title-addressed retrieval demo.

**Block total:** ~1.5d

---

## Decisions / Open Questions

**Decisions:**
- [USER] Target artifact: `docs/use-cases.md` + README "Use cases" link section; media in `docs/assets/`. Keeps README lean, gives each use case a full walkthrough.
- [USER] Audience: public GitHub readers. Tone matches the existing README — technical, first-person, candid about limitations.
- [USER] Every use case ships with screenshot or video proof; Kevin captures the media, this concept defines the shot-list (Claims & Evidence section per detail page).
- Claim calibration: "optimal" for session analysis and compaction prevention, "usable" for the rest — decided here so detail pages and the final doc never drift into uniform superlatives.
- Codex title addressing is documented as shipped (session-index thread names, `TitleSourceIndex`); the README parity table row saying "not yet" is stale and is fixed independently of this concept.
- All five use cases stay in one `docs/use-cases.md` page with anchors, matching the single-page README style — revisit only if the page exceeds roughly 400 lines with media.

**Open Questions:**
1. Media format for GitHub embedding: GIF (autoplays inline, large files), MP4 (small, but GitHub renders it as a player only in issues/PRs, not README/docs markdown — needs verification), or asciinema links (crisp for terminal content, external dependency)? Decision changes the capture instructions in every shot-list.
2. Do orchestration helper and cross-agent communication share one combined demo recording (one fleet, two agents) or get separate captures? Combined is less capture work but couples the two pages' evidence.
3. Should the compaction-preventer page reference concrete token numbers (e.g. "reset at 150–180k of 200k") as a recommended threshold, or stay qualitative? Numbers date quickly as context windows grow.
