# Skill map

Single source of truth for skill ordering and hand-offs. Each skill's own `## When to use` section carries only its one-line position and links here.

## Planning chain

Ordering doctrine — violating it has invalidated approved plans:

```
concept → concept-clarify → feature-explore → feature-design → design-refine → feature-implement → per-package-commit
```

| Step | Skill | Artifact out | Feeds the next step as |
| :--- | :--- | :--- | :--- |
| 1 | `concept` | `plans/{slug}/concept/concept.md`, `user_stories.md`, design pages | The concept the questions are drained from |
| 2 | `concept-clarify` | Same files, Open Questions drained into Decisions | A stable concept — clarifying after plan approval invalidates decisions. `concept` chains straight into it in the same session when Open Questions remain |
| 3 | `feature-explore` *(optional — solution space open/contested)* | `plans/{slug}/design/exploration.md` | Chosen option, recorded in the plan as a `[USER]` decision |
| 4 | `feature-design` | `plans/{slug}/design/raw.md` — the implementation plan | The binding contract implementation follows |
| 5 | `design-refine` *(optional — drivers against the plan, pre-implementation)* | `plans/{slug}/design/refined.md`, gate re-passed | Same contract, updated (refined supersedes raw) |
| 6 | `feature-implement` | Working, committed code | The approved plan run to completion as a binding contract |
| 7 | `per-package-commit` | Per-package validated commits | — |

`feature-refactor` is the sibling of `feature-design` for behavior-preserving restructuring: same rigor, same downstream (`feature-implement` → per-package-commit — feature-implement consumes refactor plans too), no concept/explore stages upstream. `reformat-plan` is a side-branch off steps 4/5: format migration plus familiarity-mode up-conversion, no content change. `reformat-concept` is the same side-branch off step 1: audience renderings, no content change.

`feature-refine` is the post-implementation loop: testing or usage surfaces minor behavioral adjustments after step 6 — implementation → verification → `feature-refine` (brief → gate → implement, same session) → `per-package-commit`. Contracts stay untouched and no new architecture appears; either one routes the adjustment back to `feature-design`.

## Analyze chain

```
session-analyze → batch report → analyze (fan-out) → dimension skills → proposals / memory / JSON
```

| Step | Skill | Artifact out |
| :--- | :--- | :--- |
| 1 | `session-analyze` | `sessions/<scope>/*batch-NN.md` + ledger |
| 2 | `analyze` | Fans one batch to all six dimensions via the `analyze` workflow |
| 3a | `analyze-memory` | Applied auto-memory updates + consolidation |
| 3b | `analyze-skill` | `sessions/proposals/skills.md` |
| 3c | `analyze-workflow` | `sessions/proposals/workflows.md` |
| 3d | `analyze-routines` | `sessions/proposals/routines.md` |
| 3e | `analyze-rules` | `sessions/proposals/rules.md` |
| 3f | `session-summarize` | `sessions/<scope>/json/<batch>.json` |

Dimension skills run standalone only when a single dimension is wanted; `/analyze` is the default route. Each keeps its own `analyzed-*.txt` ledger.

## Standalone skills

No fixed chain position; invoked on demand: `diagnose-debug` (diagnosis before any fix), `contract-drift-audit` (sweep every consumer of a changed shared contract + grep-to-zero close; /diagnose-debug may hand off here), `spec-drift` (read-only drift report diffing a doc set against current code; hands off to a doc-fix session or contract-drift-audit), `railroad-review` (solo or multi-agent railroad — see the skill's Modes), `couchskill-create` (repo skill authoring), `couchroutine-create` (launchd routine scaffolding + bootstrap; often downstream of couchskill-create or analyze-routines), `feature-impact` (per-axis change evaluation), `decision-support` (read-only adjudication of an external position against a closed verdict question), `coverage-increase` (hands off to per-package-commit), `parallelize` (matrix bake-off of one skill invocation across model/effort/arg-variant/replica cells; fronts the parallelize workflow), `couchskill-eval` (score skill runs against a rubric derived from the skill's own SKILL.md; matrix mode fronts the couchskill-parallel-eval pipe workflow, which nests the parallelize workflow), `investigation` (fan out N independent investigations of one open question, re-verify load-bearing claims against primary sources, merge into one baseline plus a refuted-hypotheses register; fronts the investigation workflow), `dev-stack`, `peek`, `jq`, `xlsx`, `caveman` (style modifier the planning skills delegate to).

## Composition (workflow piping)

How skills and workflows compose — three primitives, all harness-native:

1. **Skill fronts workflow** — the skill's SKILL.md resolves intake, the main session calls the Workflow tool with a `scriptPath` inside the skill's directory (`parallelize`, `investigation`, `couchskill-eval` matrix mode).
2. **Workflow nests workflow** — `workflow({scriptPath}, args)` inside a script runs another workflow inline, deterministically, sharing budget and concurrency. One nesting level only. This is the pipe primitive.
3. **Workflow runs skill** — an `agent()` stage (`agentType: 'general-purpose'`) whose prompt invokes the Skill tool and follows it unattended (parallelize cells run their target skill this way; the couchskill-parallel-eval pipe runs couchskill-eval this way).

**Pipe doctrine:** piping skill A's output into skill B is codified as a thin workflow script — nest A's workflow via `workflow()`, adapt its structured return to B's input contract in pure JS, run B via an agent stage. The pipe lives in the **consumer** skill's `workflows/` dir (the adapter produces the consumer's input contract; the consumer's SKILL.md is the trigger surface; sync deploys the script with the skill for free). Paths a pipe needs (sibling scripts, SKILL.md locations) always travel in `args` — workflow scripts have no filesystem or env access. Because nesting is one level deep, pipes are never piped: a longer chain is one pipe script calling its children sequentially.

First instance: `couchskill-parallel-eval` (`skills/couchskill/couchskill-eval/workflows/parallel-eval.js`) — nests the parallelize workflow for a sandboxed multi-model fan-out, copies surviving artifacts out of the ephemeral cell worktrees, writes the eval manifest, and scores every run with couchskill-eval.
