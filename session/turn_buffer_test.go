package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTurnBuffer_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *TurnBuffer
	}

	tests := make([]*testCase, 0)

	// pass-valid-buffer
	test := &testCase{
		_id:         "pass-valid-buffer",
		_shouldPass: true,
		form:        NewTurnBuffer(10),
	}
	tests = append(tests, test)

	// fail-nil-buffer
	test = &testCase{
		_id:         "fail-nil-buffer",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			err := test.form.Validate()
			assert.Equalf(t, test._shouldPass, err == nil, "Err: %v", err)
		})
	}
}
