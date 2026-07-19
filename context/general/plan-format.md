# Plan Format

Presentation rules for every plan the feature-design and feature-refactor skills produce. A plan the user cannot skim is worthless — every section must be checkable at reading speed.

## Language

- All plan and doc artifacts are written in English, regardless of the chat or source-document language. German requirements in, English plan out.

## Section order

- Feature plans: TLDR → Context → Scope → Assumptions → Decisions → Baseline (verified) → Exemplar & reuse → Changes → Hot items → Tests → Contracts & sweeps → Verification → Stop conditions → Open questions → Changelog.
- Refactor plans: TLDR → Context → Scope → Assumptions → Current state → Target state → Behavior contract → Decisions → Changes → Hot items → Tests → Contracts & sweeps → Verification → Stop conditions → Open questions → Changelog.
- Rationale: question → answer → evidence. Scope and decisions frame the plan before the evidence tables that back them.

## TLDR

- First section of every plan: half a page max, bullets only — what is being done, why, what the result is. Nothing more.
- No tables, no diagrams, no F/D references — it must read standalone, before any other section.
- design-refine keeps it current: a refinement that changes what/why/result updates the TLDR in the same pass.

## Args line

- A plan built with non-default intake args records them directly under the title: mode: `owned`, style: `caveman`.
- design-refine keeps the line current — an override at invocation updates it.

## Assumptions

- Own section directly after Scope: what the source doc, sketch, or user premise assumed vs. repo reality. Table: `Assumption | Reality | Location`.
- `N/A — <reason>` when the plan rests on no external assumptions.

## Text

- Bullets, not paragraphs. No prose block over 3 lines — break it into bullets with a bold lead word. This applies inside Changes entries too, not just top-level sections.
- Explanatory prose only where a hot item or an OPEN question needs it. Everything else is a bullet, a table row, or code.
- One idea per bullet; detail goes into sub-bullets.
- No semicolon-chained clauses in bullets: each clause is its own (sub-)bullet. A bullet with 3+ semicolons is a list pretending to be a sentence.
- Per-file package descriptions: file name as bullet, responsibilities as sub-bullets — never one run-on line per file.
- Scope bullets start with a bold label naming the item (e.g. **labeling-stage:**, **filter:**).
- Scope groups (**In:**, **Out:**, **Not changed:**, **Deferred findings:**) are parent bullets; every item is its own sub-bullet — never a semicolon-joined enumeration on the group line.

## Links

- Every reference to a fact, decision, or section (F20, D6, §11) is an internal markdown link to an anchor; IDs carry an `<a id="f20"></a>`-style anchor in their table cell.
- Every referenced document (design doc, style guide) is a markdown link with its path — never "doc §5" without a target.
- Locations are markdown links to path:line where possible so the viewer can open the file; plain text fallback if the renderer doesn't linkify.

## Headlines

- No inline code or formatting in headings — plain-language title plus (new | modified).
- The first line under a Changes heading is `location:` with each path formatted as inline code.
- A Changes entry with an exemplar adds a `mirrors:` line under `location:` naming the sibling it copies.

## Code

- Code appears only in fenced blocks with a language tag — never woven into sentences.
- An inline code span holds exactly one identifier or path. A sentence needing 3+ spans becomes bullets or a code block.
- Code containing backticks (Go struct tags) never goes in an inline span — fenced block only. Inline backtick nesting breaks the plan renderer and mangles the surrounding paragraph.
- **Modified existing code = `diff` fenced block**: `+`/`-` lines with 2–3 unchanged context lines — renders green/red. Every diff inside a function includes the enclosing function signature line and a `// ...` marker for elided code; a hunk must be attributable to its method at a glance.
- **New standalone units = language-tagged block with the complete final unit** (file / type / function). Never a floating fragment.
- Config, TOML, SQL, and JSON changes are shown as the final block content — never described in prose.
- JSON/TOML examples are always pretty-printed, one key per line — never multiple keys per line.
- **Code is mode-invariant.** Every added function/method appears as its full code block and every modification as its diff block — in every familiarity mode and style. Only complete boilerplate covered by a named exemplar may stay a descriptive bullet; exhaustively: mirrored test skeletons, one-line doc/config edits. Modes compress prose, never code.

## Tables

- Cells hold paths, identifiers, or short phrases. Explanations go to bullets under the table, referencing the row.
- Multi-clause cells stack their clauses top-to-bottom with `<br>` — never semicolon chains.
- A stacked clause leads with its code identifier(s) plus a colon, prose after; **bold** the discriminating words. Example cell:

```markdown
`auctionId` + `bidderRequestId` (1:1): shared by **all offers** of one request<br>`bidId`/`transactionId`: **per offer**
```

- Cases cells list one test case per line (`<br>`-separated) — never a semicolon chain.
- Never restate in prose what a table row already says.
- Inline code is allowed on identifiers in Fact/Decision/Cases cells; Location cells stay plain or are links — never backticked.
- Canonical structures:
  - Baseline facts: `ID | Fact | Needed for | Location` — "Needed for" links to the Changes entry or decision that depends on the fact. Rows are ordered by their Needed-for target in document order — decision-facts first (by D number), then change-facts (by § number), then hot-item facts — so the table can be checked side-by-side while scrolling the plan once. The primary target comes first in the cell; IDs stay stable, only row order follows the targets.
  - Assumptions: `Assumption | Reality | Location` (own section after Scope — see Assumptions)
  - Decisions: `ID | Problem | Facts | Decision | Why` — problem first, so each row reads question → answer; "Facts" cites the `F<n>` IDs the decision rests on; "Why" gives the reasoning. User-made decisions get the marker `[USER]` in the Decision cell.
  - Unit tests: `Location.Method | Cases | Comment` — location and method merged, receiver included when known.
  - Contracts & sweeps: `Contract | Sides | Sweep`
  - Stop conditions: `ID | Condition | Action`
  - Exemplar reuses: `Existing | Used for`
  - Changelog: `Date | Trigger | What changed` (see Changelog section)

## Facts relevance (EXPERIMENT — evaluate in next retrospective)

- **Pivotal marker**: a fact ID gets `!` (e.g. `F3!`) when the fact (a) **falsifies** an input assumption (memo/concept said otherwise), (b) **decides** — is cited in a Decision's Facts column, or (c) **constrains** — binds a hot item or rules out a mechanism.
- Discriminator: what would change if the fact were false? "A decision dies" = pivotal. "Nothing, it just locates code" = anchor.
- Pivotal rows sort first within their target group; cross-checking reads only pivotal rows.
- Anchor-only facts that exist solely to locate a Changes entry move INTO that Changes entry (its `location:` line) and leave Baseline entirely.

## Open questions & in-plan Q&A

- Design questions live INSIDE the plan at the point they bind — never in AskUserQuestion popups. The user needs the plan context to answer; the question must sit where the context is.
- Form: an `OPEN` row in the Decisions table — `D<n> | problem | facts | OPEN — options: a) … b) … | why it matters` — or an `> OPEN(Q<n>):` block inside the affected Changes entry.
- The `Open questions` section is an index of pointers only (`Q1 → D7`), not a separate list.
- An answer converts the row to a `[USER]` decision and appends a Changelog row.

## Changelog

- Last section of every plan. Table: `Date | Trigger | What changed`.
- One row per event: Q&A resolution (`Q: <question>`), design-refine pass (`refine: driver <n>`), post-implementation adjustment via feature-refine (`adjust: driver <n>`), local refinement without the full skill (`local: <ask>`).
- Any post-approval plan edit appends a row. The plan body is updated in place; history lives only here.
- Created empty at plan creation: `| — | initial | plan created |`.

## Caveman mode

- Planning skills accept a `caveman` intake arg: plan prose is compressed in the style of the `caveman` skill (technical content byte-perfect).
- The style is delegated to the installed skill at `~/.claude/skills/caveman` — a skill invoked with `caveman` stops with "caveman requested but skill not installed" if it is missing; it never approximates the style itself.
- Compression removes words **inside** a bullet or cell, never structure: bullets, sub-bullets, and stacked cells are invariant. Merging bullets into semicolon chains is a compression bug, not a feature.

## Exemplar placement

- Mirrors live on the Changes entries (`mirrors:` line), not in a standalone mapping — reviewable where they matter.
- The Exemplar & reuse section holds only the Reuses table (cross-cutting infrastructure) plus one bullet naming any change WITHOUT an exemplar — that is the risk signal.

## Checklists

- Verification is a `- [ ]` checklist, not a numbered list. Every item is a checkable action point: verb-first, with the observable pass condition in the item ("run X — expect Y"), so ticking it means something.
- Tool/CLI deliverables include a first-run recipe as checklist items: the exact commands with the user's real sample values, never placeholders.
- Degenerate cases (empty input, zero rows, missing config) are explicit verification items, not implied by the happy path.
- When the repo has no local runtime, verification is CI-first: items drive a CI run and name the expected job result — never local commands that cannot run.

## Diagrams

- Sequential/spatial mechanisms (windowing, overlap, pipelines, state machines) get displayed using Mermaid; bullets only annotate the diagrams — never explain such a mechanism in prose alone.

## Length

- Context: max 5 bullets — problem, cause (one path:line), the design being implemented, constraints.
- Baseline: one fact per row; only facts the plan depends on.
