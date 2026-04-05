package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func provideCompleteSessionMeta() *SessionMeta {
	return &SessionMeta{
		ID:         SessionID("sess-123"),
		Source:     SourceClaude,
		CWD:        "/home/user/project",
		GitBranch:  "main",
		Model:      "claude-opus-4-6",
		LastActive: time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC),
	}
}

func TestSessionMeta_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *SessionMeta
	}

	tests := make([]*testCase, 0)

	// pass-all-ok
	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteSessionMeta(),
	}
	tests = append(tests, test)

	// fail-nil-meta
	test = &testCase{
		_id:         "fail-nil-meta",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	// fail-empty-id
	form := provideCompleteSessionMeta()
	form.ID = ""
	test = &testCase{
		_id:         "fail-empty-id",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-invalid-source
	form = provideCompleteSessionMeta()
	form.Source = SessionSource("openai")
	test = &testCase{
		_id:         "fail-invalid-source",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-empty-source
	form = provideCompleteSessionMeta()
	form.Source = ""
	test = &testCase{
		_id:         "fail-empty-source",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-zero-last-active
	form = provideCompleteSessionMeta()
	form.LastActive = time.Time{}
	test = &testCase{
		_id:         "fail-zero-last-active",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// pass-codex-source
	form = provideCompleteSessionMeta()
	form.Source = SourceCodex
	test = &testCase{
		_id:         "pass-codex-source",
		_shouldPass: true,
		form:        form,
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			err := test.form.Validate()
			assert.Equalf(t, test._shouldPass, err == nil, "Err: %v", err)
		})
	}
}
