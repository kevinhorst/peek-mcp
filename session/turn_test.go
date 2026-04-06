package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func provideCompleteTurn() *Turn {
	return &Turn{
		Role:      "user",
		Text:      "What does this function do?",
		Timestamp: time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC),
		Model:     "claude-opus-4-6",
		Usage: &Usage{
			InputTokens:  100,
			OutputTokens: 50,
		},
	}
}

func TestTurn_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *Turn
	}

	tests := make([]*testCase, 0)

	// pass-all-ok
	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteTurn(),
	}
	tests = append(tests, test)

	// fail-nil-turn
	test = &testCase{
		_id:         "fail-nil-turn",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	// fail-invalid-role
	form := provideCompleteTurn()
	form.Role = "system"
	test = &testCase{
		_id:         "fail-invalid-role",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-empty-role
	form = provideCompleteTurn()
	form.Role = ""
	test = &testCase{
		_id:         "fail-empty-role",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-empty-text
	form = provideCompleteTurn()
	form.Text = ""
	test = &testCase{
		_id:         "fail-empty-text",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-zero-timestamp
	form = provideCompleteTurn()
	form.Timestamp = time.Time{}
	test = &testCase{
		_id:         "fail-zero-timestamp",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// pass-assistant-role
	form = provideCompleteTurn()
	form.Role = "assistant"
	test = &testCase{
		_id:         "pass-assistant-role",
		_shouldPass: true,
		form:        form,
	}
	tests = append(tests, test)

	// pass-nil-usage
	form = provideCompleteTurn()
	form.Usage = nil
	test = &testCase{
		_id:         "pass-nil-usage",
		_shouldPass: true,
		form:        form,
	}
	tests = append(tests, test)

	// fail-invalid-usage
	form = provideCompleteTurn()
	form.Usage = &Usage{InputTokens: -1}
	test = &testCase{
		_id:         "fail-invalid-usage",
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
