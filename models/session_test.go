package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func provideCompleteSession() *Session {
	return &Session{
		Meta: &SessionMeta{
			ID:         SessionID("sess-123"),
			Source:     SourceClaude,
			LastActive: time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC),
		},
		Turns: NewTurnBuffer(20),
	}
}

func TestSession_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *Session
	}

	tests := make([]*testCase, 0)

	// pass-all-ok
	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteSession(),
	}
	tests = append(tests, test)

	// fail-nil-session
	test = &testCase{
		_id:         "fail-nil-session",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	// fail-nil-meta
	form := provideCompleteSession()
	form.Meta = nil
	test = &testCase{
		_id:         "fail-nil-meta",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-invalid-meta
	form = provideCompleteSession()
	form.Meta.ID = ""
	test = &testCase{
		_id:         "fail-invalid-meta",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-nil-turns
	form = provideCompleteSession()
	form.Turns = nil
	test = &testCase{
		_id:         "fail-nil-turns",
		_shouldPass: false,
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
