# Concept: Diff Target Inference

> **Status:** Draft
> **Author:** Kevin Horst
> **Date:** 2026-07-19

---

## Goals

- `session_diff` produces a correct diff without the user configuring a base branch — `--diff-target` / `PEEK_DIFF_TARGET` disappear from the config surface.
- The base branch is inferred per repository from the live git checkout at diff time; repos using `master`, `develop`, or anything else just work.
- Clients can see which base each diff was computed against.

---

## User Flows

### Automatic diff base per session

**Goals:**
- A session in any repo gets its diff against that repo's actual default branch, with zero configuration.

**Options:**

**MVP** (~1.5–2d total)
- Inference helper in the watcher package, run in the session's `cwd` (~0.5–1d):
  1. `git symbolic-ref --short refs/remotes/origin/HEAD` → strip `origin/` → base branch.
  2. Fallback when no remote / unset origin/HEAD: probe local branches `main`, then `master` via `git rev-parse --verify`.
  3. If the session's current branch (`Meta.GitBranch`) *is* the inferred base, the diff is base-vs-working-tree — same as today, no special case.
  4. Inference fails entirely (no repo, detached mid-rebase, bare) → skip the turn diff for that refresh, as `gitReady` failures do today.
- Per-repo cache of the inferred base keyed by `gitAbsoluteDir` — the default branch of a repo effectively never changes within a server lifetime (~0.25d).
- Wire into `refresh` replacing `w.target`; delete the flag, env var, mcpb `diff_target`, and the `NewDiffWatcher` target parameter — no override mode kept alive alongside inference (~0.5d).
- Surface the resolved base: drop `json:"-"` on `Session.DiffTarget`, include it in `session_diff` / `session_full` output; update tool descriptions and README (~0.5d).

**Backlog**
- Merge-base semantics: `git diff $(git merge-base <base> HEAD)` (or three-dot) so a base that advanced past the fork point doesn't pollute the session diff with unrelated upstream commits (~0.5–1d).
- Record the inferred base back into session-adjacent state so `session_list` can show it per session (~0.5d).

**Challenges:**
- `origin/HEAD` is unset in some clones (`git remote set-head origin -a` never ran) and absent in remote-less repos.
- Claude worktrees share the main checkout's remotes, so `origin/HEAD` resolves there too — but the *current* branch (`claude/<slug>`) must never be mistaken for the base.

**Approach:**
- The symbolic-ref → local-probe fallback chain covers unset `origin/HEAD` and remote-less repos; `Meta.GitBranch` is only used as a guard/input, never as the base itself.

---

## Decisions / Open Questions

**Decisions:**
- Inference from the live repo, not session data — the base branch is provably absent from Claude and Codex transcripts; only the current branch is recorded.
- `--diff-target` is deleted, not demoted to an override — keeping both would leave two parallel mechanisms alive.
- Inference runs in the watcher at diff time (per repo, cached), not at tool-call time — diffs are precomputed by the watcher today and that stays.

**Open Questions:**
1. Fallback-chain miss (no origin/HEAD, no local `main`/`master`): skip the diff silently like `gitReady` failures, or store an explanatory marker the tools surface ("no diff base inferable")?
2. MVP diff semantics stay two-dot `git diff <base>` (status quo) with merge-base as backlog — or is fork-point correctness required for MVP because worktree sessions routinely outlive base advancement?
3. mcpb bundle: removing `diff_target` from `user_config` changes the manifest contract — any released-bundle compatibility concern, or free to drop in the next release?
