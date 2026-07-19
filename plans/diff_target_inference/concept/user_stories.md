# User Stories: Diff Target Inference

---

## Zero configuration

**As a peek-mcp user**, I want session diffs to work against my repo's actual default branch without setting `--diff-target`, so that repos using `master` or `develop` need no per-repo setup.

**As a peek-mcp user**, I want to remove `diff_target` from my server config entirely, so that one global server instance serves repos with different default branches correctly.

---

## Transparency

**As an agent consuming `session_diff`**, I want the response to state which base branch the diff was computed against, so that I can judge whether the diff means what I think it means.
