package codex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func provideCompleteSessionMeta() *SessionMeta {
	return &SessionMeta{
		Id:         "sess-123",
		CWD:        "/project",
		CLIVersion: "1.0.0",
	}
}

func TestSessionMeta_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *SessionMeta
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteSessionMeta(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-meta",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteSessionMeta()
	form.Id = ""
	test = &testCase{
		_id:         "fail-empty-id",
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
