# Diff Retention (~2d)

---

## Flows

### Pin the base at first sight

1. A session becomes active in a git repo; `DiffWatcher` computes its first diff.
2. Backend
   1. Before the first compute: `git merge-base HEAD <target-branch>` → base sha; persist it to `<state>/<agent>/<session-id>/diff.base`.
   2. All subsequent `session_diff` computes run `git diff <base-sha>` instead of `git diff <target-branch>`.
3. The diff now shows the session's own work even after the target branch advances or the session branch is merged — the merge-collapse failure mode is gone while the repo exists.

### Snapshot on every non-empty compute

1. `DiffWatcher` recomputes on turn activity (existing trigger).
2. Backend
   1. Every successful non-empty compute atomically overwrites `<state>/<agent>/<session-id>/diff.snapshot` (+ capture timestamp).
   2. Empty results do not overwrite a non-empty snapshot — an empty live diff is served live, but the last real work is retained.
3. The snapshot is the durable copy of the session's final diff.

### Serve after cleanup (the cherry-pick scenario)

1. Real workflow (claude-configs worktree management): session commits are cherry-picked onto a feature branch — **new SHAs, the session's commits and base never appear in the target history** — then worktree and branch are removed. The session cwd no longer exists; `git diff` cannot run at all; even an intact repo no longer has the pinned sha reachable from any ref.
2. Backend
   1. `session_diff` tries the live compute first (pinned base). On failure (cwd gone, sha unresolvable, git error) it falls back to the snapshot.
   2. Fallback responses carry `source: "snapshot"` and `captured_at`; live responses carry `source: "live"`.
3. Analysis always gets the diff; staleness is explicit, never silent.

### Daemon restart

1. Peek restarts; in-memory session state is rebuilt from transcripts.
2. Backend
   1. On session re-discovery, `diff.base` and `diff.snapshot` are reloaded from the state dir — pin and snapshot survive restarts.

---

## Security Considerations

- Snapshots contain source code: state dir created `0700`, files `0600`.
- The state dir path never derives from transcript content — only from agent name + session id (sanitized as a path component).
- Atomic writes (temp file + rename), matching the existing `peek-diff` hook-file pattern.

---

## Limits

- Snapshot size: 5 MB per session (beyond that: truncate with marker — analysis of megadiff sessions doesn't need byte fidelity).
- Uncommitted diff stays live-only (decision): it has no meaning once the worktree is gone; the committed-work snapshot is the analysis artifact.
- Retention/GC of old sessions' state: Open Question 2 in [concept.md](concept.md).

---

## Models

### Diff state (state dir, per session)

**Internal / Not Exported:**
- diff.base: pinned merge-base sha + target branch name at pin time (small text file)
- diff.snapshot: last non-empty `git diff <base>` output + capture timestamp

### session_diff response (extended)

**Public:**
- source: live | snapshot (string)
- captured_at: snapshot capture time, only when source=snapshot (time)

---

## Long-Tail Tasks

### Testing

- Pin correctness: target branch advances after pin → diff unchanged; branch merged → diff still shows session work.
- Cherry-pick + cleanup end-to-end: remove worktree and branch → `session_diff` serves the snapshot, flagged.
- Empty-diff guard: revert-to-empty live diff is served live while the snapshot retains the last non-empty state.
- Restart: pin + snapshot reload from the state dir.
