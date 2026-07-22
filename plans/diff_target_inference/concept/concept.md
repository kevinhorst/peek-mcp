# Concept: Diff Target Inference

> **Status:** In Review
> **Author:** Kevin Horst
> **Date:** 2026-07-19

---

## Goals

- `session_diff` produces a correct diff without the user configuring a base branch — `--diff-target` / `PEEK_DIFF_TARGET` disappear from the config surface.
- The base branch is inferred per session branch from the live git checkout at diff time; repos using `master`, `develop`, or anything else just work — with or without a remote.
- Clients can see which base each diff was computed against.

---

## User Flows

### Automatic diff base per session

**Goals:**
- A session in any repo gets its diff against that repo's actual default branch, with zero configuration.

**Options:**

**MVP** (~1.5–2d total)
- Inference helper in the watcher package, run in the session's `cwd` (~0.5–1d):
  1. Branch reflog of the checked-out branch: the oldest entry records `branch: Created from <ref>` — strip `refs/remotes/` / `refs/heads/`, discard empty/`HEAD`/`claude/*` results, verify the ref still exists (mirrors `resolve_from()` in claude-configs `cmd/worktrees/print_agent_worktrees_status.sh`). Per-branch, purely local, no remote or naming assumptions.
  2. Fallback when the reflog records no resolvable origin: `git symbolic-ref --short refs/remotes/origin/HEAD` → strip `origin/`.
  3. Fallback: probe local branches `main`, then `master` via `git rev-parse --verify`.
  4. Terminal fallback: `HEAD` — `diff_target` is never empty; a `HEAD` base yields the uncommitted diff.
  5. If the session's current branch (`Meta.GitBranch`) *is* the inferred base, the diff is base-vs-working-tree — same as today, no special case.
  6. Not a git repo / bare (`gitReady` false) → no diff at all, exactly as today.
- Cache of the inferred base keyed by session `cwd` — one checked-out branch per worktree, and a branch's creation point never changes within a server lifetime (~0.25d).
- Wire into `refresh` replacing `w.target`, diffing with merge-base semantics: `git diff <base>...` (three-dot), so a base that advanced past the fork point doesn't pollute the session diff with unrelated upstream commits; delete the flag, env var, mcpb `diff_target`, and the `NewDiffWatcher` target parameter — no override mode kept alive alongside inference (~0.5d).
- Surface the resolved base as JSON fields: drop `json:"-"` on `Session.DiffTarget`, expose `diff_target` in `session_full` output and `session_list` items; `session_diff` stays a pure raw-text diff. Update tool descriptions and README (~0.5d).

**Backlog**
- (empty — merge-base semantics and `session_list` base exposure moved into MVP)

**Challenges:**
- Reflog entries expire (`gc.reflogExpire`, default 90 days) — old branches may have lost their creation entry; the fallback chain covers that.
- `origin/HEAD` is unset in some clones (`git remote set-head origin -a` never ran) and absent in remote-less repos.
- The *current* branch (`claude/<slug>`) must never be mistaken for the base — the reflog resolver discards `claude/*` and `HEAD` creation records.

**Approach:**
- The reflog → symbolic-ref → local-probe → `HEAD` chain always resolves a base in any real repo, remote or not; `Meta.GitBranch` is only used as a guard/input, never as the base itself.

---

## Decisions / Open Questions

**Decisions:**
- Inference from the live repo, not session data — the base branch is provably absent from Claude and Codex transcripts; only the current branch is recorded.
- `--diff-target` is deleted, not demoted to an override — keeping both would leave two parallel mechanisms alive.
- Inference runs in the watcher at diff time (per repo, cached), not at tool-call time — diffs are precomputed by the watcher today and that stays.
- [USER] The primary inference signal is the branch reflog's `Created from <ref>` entry — per-branch, purely local, records the actual creation point instead of guessing the repo default; proven in claude-configs `cmd/worktrees/print_agent_worktrees_status.sh` (`resolve_from()`). `origin/HEAD` and the `main`/`master` probe are demoted to fallbacks for expired/absent reflogs.
- [USER] `diff_target` is never empty: the chain terminates in `HEAD` (base = `HEAD` yields the uncommitted diff). No silent-skip-with-empty-field state exists; the only diff-less state is "not a git repo", where `gitReady` already skips today.
- [USER] Merge-base semantics land in MVP via three-dot `git diff <base>...` — a one-argument change to the existing `gitDiff` call, no extra git process; worktree sessions that outlive base advancement stop showing upstream commits as reversed noise. The backlog merge-base item is gone.
- mcpb `diff_target` is free to drop in the next release: the manifest and binary ship together in one `.mcpb` (`mcpb/manifest.json` args + `server.type: binary`), so installed bundles keep old manifest + old binary and upgrades replace both atomically — no skew possible; stored user-config values for removed keys are dropped on upgrade.
- [USER] The resolved base is surfaced as JSON fields only — `diff_target` in `session_full` and `session_list` — because `session_diff` returns raw diff text (`respondWithText`, `tools/tools.go`) and a marker line inside plain diff text would be ambiguous with diff content; `session_diff` output stays a pure applyable diff.

**Open Questions:**

None.
