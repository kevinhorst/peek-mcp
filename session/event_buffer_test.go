package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventBuffer_PushAndAll(t *testing.T) {
	// under-capacity
	t.Run("under-capacity", func(t *testing.T) {
		buffer := NewEventBuffer(5)
		buffer.Push(&Event{Kind: EventKindSkillInvoked})
		buffer.Push(&Event{Kind: EventKindPlanApproved})

		all := buffer.All()
		require.Len(t, all, 2)
		assert.Equal(t, EventKindSkillInvoked, all[0].Kind)
		assert.Equal(t, EventKindPlanApproved, all[1].Kind)
	})

	// overflow-drops-oldest
	t.Run("overflow-drops-oldest", func(t *testing.T) {
		buffer := NewEventBuffer(2)
		buffer.Push(&Event{Kind: EventKindSkillInvoked})
		buffer.Push(&Event{Kind: EventKindPlanApproved})
		buffer.Push(&Event{Kind: EventKindPlanRejected})

		all := buffer.All()
		require.Len(t, all, 2)
		assert.Equal(t, EventKindPlanApproved, all[0].Kind)
		assert.Equal(t, EventKindPlanRejected, all[1].Kind)
	})
}

func TestEventBuffer_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *EventBuffer
	}

	tests := make([]*testCase, 0)

	// pass-valid-buffer
	tests = append(tests, &testCase{
		_id:         "pass-valid-buffer",
		_shouldPass: true,
		form:        NewEventBuffer(10),
	})

	// fail-nil-buffer
	tests = append(tests, &testCase{
		_id:         "fail-nil-buffer",
		_shouldPass: false,
		form:        nil,
	})

	// fail-zero-capacity
	tests = append(tests, &testCase{
		_id:         "fail-zero-capacity",
		_shouldPass: false,
		form:        &EventBuffer{},
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			err := test.form.Validate()
			assert.Equalf(t, test._shouldPass, err == nil, "Err: %v", err)
		})
	}
}
