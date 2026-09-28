package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventBuffer_PushAndAll(t *testing.T) {
	// under-capacity
	t.Run("under-capacity", func(t *testing.T) {
		buffer := NewEventBuffer(5, 2)
		buffer.Push(&Event{Kind: EventKindSkillInvoked})
		buffer.Push(&Event{Kind: EventKindPlanApproved})

		all := buffer.All()
		require.Len(t, all, 2)
		assert.Equal(t, EventKindSkillInvoked, all[0].Kind)
		assert.Equal(t, EventKindPlanApproved, all[1].Kind)
	})

	// denial-budget-keeps-first
	t.Run("denial-budget-keeps-first", func(t *testing.T) {
		buffer := NewEventBuffer(5, 2)
		for _, toolUseId := range []string{"tu1", "tu2", "tu3"} {
			buffer.Push(&Event{Kind: EventKindPermissionDenied, Permission: &PermissionPayload{ToolUseId: toolUseId}})
		}

		all := buffer.All()
		require.Len(t, all, 2)
		assert.Equal(t, "tu1", all[0].Permission.ToolUseId)
		assert.Equal(t, "tu2", all[1].Permission.ToolUseId)
	})

	// other-budget-keeps-first
	t.Run("other-budget-keeps-first", func(t *testing.T) {
		buffer := NewEventBuffer(3, 1)
		buffer.Push(&Event{Kind: EventKindSkillInvoked})
		buffer.Push(&Event{Kind: EventKindPlanApproved})
		buffer.Push(&Event{Kind: EventKindPlanRejected})

		all := buffer.All()
		require.Len(t, all, 2)
		assert.Equal(t, EventKindSkillInvoked, all[0].Kind)
		assert.Equal(t, EventKindPlanApproved, all[1].Kind)
	})

	// denials-never-evict-others
	t.Run("denials-never-evict-others", func(t *testing.T) {
		buffer := NewEventBuffer(3, 1)
		buffer.Push(&Event{Kind: EventKindSkillInvoked})
		buffer.Push(&Event{Kind: EventKindPlanApproved})
		buffer.Push(&Event{Kind: EventKindPermissionDenied})
		buffer.Push(&Event{Kind: EventKindPermissionDenied})

		all := buffer.All()
		require.Len(t, all, 3)
		assert.Equal(t, EventKindSkillInvoked, all[0].Kind)
		assert.Equal(t, EventKindPlanApproved, all[1].Kind)
		assert.Equal(t, EventKindPermissionDenied, all[2].Kind)
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
		form:        NewEventBuffer(10, 5),
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

	// fail-denial-budget-above-capacity
	tests = append(tests, &testCase{
		_id:         "fail-denial-budget-above-capacity",
		_shouldPass: false,
		form:        NewEventBuffer(10, 11),
	})

	// fail-negative-denial-budget
	tests = append(tests, &testCase{
		_id:         "fail-negative-denial-budget",
		_shouldPass: false,
		form:        NewEventBuffer(10, -1),
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			err := test.form.Validate()
			assert.Equalf(t, test._shouldPass, err == nil, "Err: %v", err)
		})
	}
}
