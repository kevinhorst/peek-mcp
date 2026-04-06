package codex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func provideCompleteTurnContext() *TurnContext {
	return &TurnContext{
		TurnId: "turn-123",
		Model:  "gpt-5.4",
		CWD:    "/project",
	}
}

func TestTurnContext_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *TurnContext
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteTurnContext(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-context",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteTurnContext()
	form.TurnId = ""
	form.Model = ""
	form.CWD = ""
	test = &testCase{
		_id:         "fail-empty-context",
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
