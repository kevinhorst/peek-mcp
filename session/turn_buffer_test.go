package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTurnBuffer_ConversationalRing(t *testing.T) {
	first := &Turn{Text: "first"}
	second := &Turn{Text: "second"}
	buffer := NewTurnBuffer(2)
	buffer.Push(first)
	for range 5 {
		buffer.Push(&Turn{ToolCalls: []*ToolCall{{Id: "tu"}}})
	}
	buffer.Push(second)

	// tool-only-flood-keeps-conversational
	assert.Equal(t, []*Turn{first, second}, buffer.conversational)
	assert.Len(t, buffer.items, 2)
	assert.Same(t, second, buffer.items[1])

	// backing-array-never-grows
	assert.Equal(t, 2, cap(buffer.items))
	assert.Equal(t, 2, cap(buffer.conversational))

	// session-turns-read-conversational-ring
	sess := &Session{TurnsFinished: buffer}
	assert.Equal(t, []*Turn{first, second}, sess.Turns(AllTurns))
	assert.Len(t, sess.TurnsWithToolCalls(AllTurns), 2)
}

func TestTurnBuffer_Gob(t *testing.T) {
	first := &Turn{Text: "first"}
	buffer := NewTurnBuffer(2)
	buffer.Push(first)
	buffer.Push(&Turn{ToolCalls: []*ToolCall{{Id: "tu1"}}})
	buffer.Push(&Turn{Text: "second"})

	data, err := buffer.GobEncode()
	require.NoError(t, err)
	decoded := &TurnBuffer{}
	require.NoError(t, decoded.GobDecode(data))

	// rings-restored
	require.Len(t, decoded.conversational, 2)
	assert.Equal(t, "first", decoded.conversational[0].Text)
	assert.Equal(t, "second", decoded.conversational[1].Text)
	require.Len(t, decoded.items, 2)

	// shared-turn-stays-shared
	assert.Same(t, decoded.items[1], decoded.conversational[1])

	// counts-restored
	assert.Equal(t, 2, decoded.Pushed())
	assert.Equal(t, 3, decoded.PushedWithToolCalls())
}

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
