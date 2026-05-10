git worktree list --porcelain \
  | awk '/^worktree /{path=$2} /^branch .*claude/{print path} /^worktree .*\.codex\/worktrees/{print $2}' \
  | xargs -I{} git worktree remove --force {}