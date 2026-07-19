# GO TEST CODE STYLE RULES (MINIMAL + AGENT-OPTIMIZED)

## SCOPE

This document defines required test patterns for Go tests in this repository.
It is the single source of truth for test case shape and execution style.

---

## BASELINE

All tests MUST use:

* `testing`
* `github.com/stretchr/testify/assert` for assertions
* `github.com/stretchr/testify/require` ONLY for test setup steps that MUST abort the test on failure (e.g. building requests, fixtures)

Keep tests short. Use table-driven tests for complex behavior or many combinations.

---

## TABLE-DRIVEN TESTS

RULE-TEST-001:

* Table-driven tests MUST use a struct named `testCase`
* Meta fields MUST be prefixed with `_`
* Input fields MUST NOT be prefixed with `_`

RULE-TEST-002:

Meta fields in `testCase` MUST come first and be sorted alphabetically.
Then input fields MUST come, also sorted alphabetically.

Example:

```go
type testCase struct {
	_id                string
	_expectedQuery     string
	_expectedQueryArgs []interface{}
	_shouldPass        bool
	filter             *GameLinksFilter
	query              string
}
```

RULE-TEST-003:

In each test case literal, `_id` MUST be the first field.

RULE-TEST-004:

Before the loop over test cases, add:

```go
// Run tests
```

RULE-TEST-005:

Test execution MUST use `t.Run(test._id, func(t *testing.T) { ... })`.

RULE-TEST-006:

Test case IDs MUST use kebab-case (e.g. `"nil-error"`, `"pq-error-with-different-code"`).

RULE-TEST-007:

This rule defines what "table-driven test" means concretely. It applies only when a table-driven test is warranted (complex behavior or many combinations — see baseline). For one or two simple cases, plain `assert.Equal` calls are preferred over the boilerplate below.

Test cases MUST be collected using `make([]*testCase, 0)` and appended individually with `append`. Do NOT use a slice literal.

Each append block MUST be preceded by a comment that repeats the `_id` value verbatim:

```go
tests := make([]*testCase, 0)

// nil-error
tests = append(tests, &testCase{
	_id:       "nil-error",
	_expected: false,
	err:       nil,
})

// unrelated-error
tests = append(tests, &testCase{
	_id:       "unrelated-error",
	_expected: false,
	err:       errors.New("something went wrong"),
})
```

RULE-TEST-008:

Test-case structs MUST carry fully-constructed dependencies (db, service, clients) as fields, initialized inside each case's append block. The `t.Run` loop body contains ONLY the call under test and assertions — no dependency construction, no field wiring from indirection fields.

```go
// fallback-match
db := models.NewMockDB()
db.TagRequests = provideTagRequests()
tests = append(tests, &testCase{
	_id:     "fallback-match",
	service: NewPrebidWebEventsServiceV1(&ServerContext{db: db}),
	event:   provideWebEvent(),
})
```

Not: `db.TagRequests = test.tagRequests` inside the run loop.

---

## ASSERTION STYLE

RULE-ASSERT-001:

* Use `assert.Equal`, `assert.NotNil`, `assert.Same`, etc.
* Prefer direct value assertions over custom `if`/`t.Fatalf` checks, unless assertion helpers are insufficient

RULE-ASSERT-002:

For `_shouldPass` patterns, assert `err == nil` against `_shouldPass`.

```go
assert.Equalf(t, test._shouldPass, err == nil, "err = %v", err)
```

---

## AGENT WORKFLOW

When creating or editing tests, AI agents MUST:

1. Reuse an existing test file in the same package as template first.
2. Keep naming and field order consistent with this document.
3. Run targeted tests for changed tests first (`go test ./path -run TestName`).
4. Run `gofmt` (or `goimports` if imports changed) before finishing.
