package codex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func provideCompleteEventMessage() *EventMessage {
	return &EventMessage{
		Type: EventTypeTokenCount,
		Info: &EventInfo{
			TotalTokenUsage: &TokenUsage{
				InputTokens:           100,
				CachedInputTokens:     60,
				OutputTokens:          20,
				ReasoningOutputTokens: 5,
				TotalTokens:           125,
			},
		},
	}
}

func TestEventMessage_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *EventMessage
	}

	tests := make([]*testCase, 0)

	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteEventMessage(),
	}
	tests = append(tests, test)

	test = &testCase{
		_id:         "fail-nil-event-message",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	form := provideCompleteEventMessage()
	form.Type = ""
	test = &testCase{
		_id:         "fail-empty-type",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	form = provideCompleteEventMessage()
	form.Info.TotalTokenUsage.TotalTokens = -1
	test = &testCase{
		_id:         "fail-invalid-token-usage",
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
