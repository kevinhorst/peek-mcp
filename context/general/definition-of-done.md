# Definition of Done

## 1. Functionality & Spec
- [ ] Feature matches the agreed spec
- [ ] Inputs validated; auth checks enforced on new or changed endpoints
- [ ] Outputs (response format, status codes, error cases) are correct
- [ ] No behavior regressions — existing flows re-checked where touched (backward compatibility)

## 2. End-to-End Verification
- [ ] Happy path verified end-to-end in a running system (realistic use case)
- [ ] At least 1–2 relevant edge cases verified
- [ ] Error cases verified (e.g. invalid input, missing data)

## 3. Automated Tests & Build
- [ ] Unit or integration tests added for core logic
- [ ] Existing tests updated (if behavior changed)
- [ ] Tests pass
- [ ] Build, vet, and linters pass

## 4. Database & Migrations (if schema or data changed)
- [ ] Schema migrations implemented correctly (including indexes, defaults, constraints)
- [ ] Data migrations created for initial data population
- [ ] Migration tested (up + rollback considered)
- [ ] Existing data / edge cases accounted for (null values, legacy data, etc.)

## 5. Logging & Error Behavior
- [ ] Relevant errors are logged
- [ ] No debug logs left in code
- [ ] Errors carry enough context to locate the failure (no opaque errors)
- [ ] No secrets in code, config, or logs
- [ ] Debug/temporary logging redacts sensitive payloads — no payment nonces, no full provider request/response bodies

## 6. Deployment & Operations
- [ ] Required steps documented (e.g. migration, config, env vars)
- [ ] Changes are backward-compatible for clients

## 7. Code Quality
- [ ] Code follows project conventions (naming, structure)
- [ ] Code follows the defined style guides
- [ ] TODOs introduced by this change resolved or ticketed
- [ ] No unnecessary complexity / dead code
- [ ] No unnecessary code duplication

## 8. Documentation & Agent Guidance
- [ ] `AGENTS.md` updated if agent instructions or review expectations changed
- [ ] Relevant files in context directory updated if workflows, context, or operational knowledge changed
- [ ] Status/progress docs re-derived from git log/diff — never updated from memory

## Definition of "Done"
> Done means: every item above is PASS or N/A — and each N/A has a stated reason.
