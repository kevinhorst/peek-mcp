---
name: per-package-commit
description: Build, test, and commit changed files grouped by package. Trigger whenever organizing local changes into logical per-package commits with validation. Use when multiple packages changed and need committing separately.
---

# Per-Package Commit

Commit changed files grouped by Go package. Validate first.

## 1. Validate (abort on any fail)
- `go build ./...` — fail: show error, stop.
- `go test ./...` — fail: show error, stop.
  No commit if either fails.

## 2. Identify changed files
- `git diff --name-only`
- `git diff --cached --name-only`
- Union both lists.

## 3. Group
- Go files: by package dir, dot-notation.
  `util/reporting/interactive/queries` → `util.reporting.interactive.queries`
- Non-Go files: by logical owner.
  - `sql/reporting/*` → `util.reporting`
  - `docs/runbooks/*` → `docs/runbooks`
  - root Makefile → `build`
- `*.gen.go`: skip (not version controlled).

## 4. Commit (per group, logical order)
- `git add <files>`
- Message: `<package.path>: <description>`
  Format: dot-notation path, colon, present-tense, concise, no trailing period, no rule IDs.

Examples:
  - docs.runbooks: added deployment.md

## 5. Show result
- `git log --oneline -10`

## Rules
- build/test fail → show error, no commit.
- Never combine unrelated packages.
- No push (local only).
- No "Co-authored by Claude".
- Skip `*.gen.go`.

