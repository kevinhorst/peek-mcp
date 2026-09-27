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
		Meta:      &Meta{Model: "claude-opus-4-6"},
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

	// fail-missing-meta
	form = provideCompleteTurn()
	form.Meta = nil
	test = &testCase{
		_id:         "fail-missing-meta",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// pass-title-signal-with-source
	form = &Turn{
		CustomTitle: "Login simplification",
		Meta:        &Meta{SessionId: "s1"},
		TitleSource: TitleSourceCustom,
	}
	test = &testCase{
		_id:         "pass-title-signal-with-source",
		_shouldPass: true,
		form:        form,
	}
	tests = append(tests, test)

	// fail-title-signal-missing-source
	form = &Turn{
		CustomTitle: "Login simplification",
		Meta:        &Meta{SessionId: "s1"},
	}
	test = &testCase{
		_id:         "fail-title-signal-missing-source",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// event-signal-ok
	form = &Turn{
		Events: []*Event{{Kind: EventKindSkillInvoked}},
		Meta:   &Meta{SessionId: "s1"},
	}
	test = &testCase{
		_id:         "event-signal-ok",
		_shouldPass: true,
		form:        form,
	}
	tests = append(tests, test)

	// event-signal-missing-session
	form = &Turn{
		Events: []*Event{{Kind: EventKindSkillInvoked}},
		Meta:   &Meta{},
	}
	test = &testCase{
		_id:         "event-signal-missing-session",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// usage-signal-ok
	form = &Turn{
		Usage: &Usage{InputTokens: 10},
		Meta:  &Meta{SessionId: "s1"},
	}
	test = &testCase{
		_id:         "usage-signal-ok",
		_shouldPass: true,
		form:        form,
	}
	tests = append(tests, test)

	// usage-signal-invalid-usage
	form = &Turn{
		Usage: &Usage{InputTokens: -1},
		Meta:  &Meta{SessionId: "s1"},
	}
	test = &testCase{
		_id:         "usage-signal-invalid-usage",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// pass-tool-result-signal
	form = &Turn{
		Meta:        &Meta{SessionId: "s1"},
		ToolResults: []*ToolResult{{IsError: true, ToolUseId: "tu1"}},
	}
	test = &testCase{
		_id:         "pass-tool-result-signal",
		_shouldPass: true,
		form:        form,
	}
	tests = append(tests, test)

	// fail-tool-result-signal-without-session-id
	form = &Turn{
		Meta:        &Meta{},
		ToolResults: []*ToolResult{{IsError: true, ToolUseId: "tu1"}},
	}
	test = &testCase{
		_id:         "fail-tool-result-signal-without-session-id",
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

func TestTurn_IsToolOnly(t *testing.T) {
	type testCase struct {
		_id       string
		_expected bool
		turn      *Turn
	}

	calls := []*ToolCall{{Id: "tu1", Name: "Read"}}
	tests := make([]*testCase, 0)

	// calls-no-text
	tests = append(tests, &testCase{
		_id:       "calls-no-text",
		_expected: true,
		turn:      &Turn{ToolCalls: calls},
	})

	// calls-with-text
	tests = append(tests, &testCase{
		_id:       "calls-with-text",
		_expected: false,
		turn:      &Turn{Text: "reading", ToolCalls: calls},
	})

	// calls-with-thinking
	tests = append(tests, &testCase{
		_id:       "calls-with-thinking",
		_expected: false,
		turn:      &Turn{Thinking: "plan", ToolCalls: calls},
	})

	// no-calls
	tests = append(tests, &testCase{
		_id:       "no-calls",
		_expected: false,
		turn:      &Turn{},
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			assert.Equal(t, test._expected, test.turn.IsToolOnly())
		})
	}
}
