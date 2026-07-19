# Agents

## Implementing
- MUST follow golden paths and existing patterns
- MUST read the nearest sibling (service, model, test file) before writing new code — new code mirrors its architecture
- Every new concept (type, file, interface, goroutine, dependency, endpoint) MUST trace back to the request — otherwise remove it or stop and ask
- Non-trivial features start from an approved plan (feature-design skill); restructuring without behavior change starts from a refactor plan (feature-refactor skill)
- Implementations in classes listed in `context/general/hot-items.md` require an approved example implementation in the plan before coding
- Plans carry the stop conditions from `context/general/stop-conditions.md`

## Reviewing
- Check feature completeness against `context/general/definition-of-done.md` — every item PASS or N/A with a stated reason
- Review findings MUST explicitly flag violations of the style guides listed below
- Detect deviations from existing patterns; suggest improvements

## Testing & Verifying
- Tests exist and pass for core logic; add missing coverage as part of the change
- Verify changes in a running system, not only through unit tests

## Subagents
- Subagents do not inherit this file. Any subagent prompt MUST instruct the subagent to read `AGENTS.md` and `context/` first, or inline the constraints that matter for its task

# Rules

- Never scan the entire repository
- Always use `context/` for orientation
- Prefer existing implementations over new ones
- Follow Go code style: context/go/go.md
- Follow Go test code style: context/go/go-tests.md
- For ad-hoc JSON exploration (webhook dumps, DLQ files, API responses), use `jq` — not throwaway Python scripts
- Keep `AGENTS.md` and `context/` up to date as part of the change when instructions, workflows, or expectations change
