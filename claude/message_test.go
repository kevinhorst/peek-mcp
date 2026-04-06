package claude

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func provideCompleteMessage() *Message {
	return &Message{
		Role:    "assistant",
		Content: json.RawMessage(`[]`),
		Usage: &Usage{
			InputTokens:  1,
			OutputTokens: 2,
		},
	}
}

func TestMessage_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *Message
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteMessage(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-message",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteMessage()
	form.Role = "system"
	test = &testCase{
		_id:         "fail-invalid-role",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteMessage()
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
