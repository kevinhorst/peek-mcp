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

func TestTurnBuffer_Pushed(t *testing.T) {
	type testCase struct {
		_id                          string
		_expectedLen                 int
		_expectedPushed              int
		_expectedPushedWithToolCalls int
		buffer                       *TurnBuffer
	}

	toolOnly := &Turn{ToolCalls: []*ToolCall{{Id: "tu1"}}}
	conversational := &Turn{Text: "hello"}
	tests := make([]*testCase, 0)

	// tool-only-not-counted
	buffer := NewTurnBuffer(10)
	buffer.Push(toolOnly)
	tests = append(tests, &testCase{
		_id:                          "tool-only-not-counted",
		_expectedLen:                 1,
		_expectedPushed:              0,
		_expectedPushedWithToolCalls: 1,
		buffer:                       buffer,
	})

	// conversational-counted
	buffer = NewTurnBuffer(10)
	buffer.Push(conversational)
	tests = append(tests, &testCase{
		_id:                          "conversational-counted",
		_expectedLen:                 1,
		_expectedPushed:              1,
		_expectedPushedWithToolCalls: 1,
		buffer:                       buffer,
	})

	// with-tool-calls-counts-both
	buffer = NewTurnBuffer(10)
	buffer.Push(conversational)
	buffer.Push(toolOnly)
	buffer.Push(conversational)
	tests = append(tests, &testCase{
		_id:                          "with-tool-calls-counts-both",
		_expectedLen:                 3,
		_expectedPushed:              2,
		_expectedPushedWithToolCalls: 3,
		buffer:                       buffer,
	})

	// eviction-keeps-counts
	buffer = NewTurnBuffer(2)
	buffer.Push(conversational)
	buffer.Push(toolOnly)
	buffer.Push(conversational)
	tests = append(tests, &testCase{
		_id:                          "eviction-keeps-counts",
		_expectedLen:                 2,
		_expectedPushed:              2,
		_expectedPushedWithToolCalls: 3,
		buffer:                       buffer,
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			assert.Equal(t, test._expectedLen, test.buffer.Len())
			assert.Equal(t, test._expectedPushed, test.buffer.Pushed())
			assert.Equal(t, test._expectedPushedWithToolCalls, test.buffer.PushedWithToolCalls())
		})
	}
}
