package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func provideCompleteSession() *Session {
	return &Session{
		Meta:          Meta{SessionId: Id("sess-123")},
		Agent:         AgentClaude,
		LastActive:    time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC),
		TurnsFinished: NewTurnBuffer(20),
	}
}

func TestSession_Validate(t *testing.T) {
	type testCase struct {
		_id         string
		_shouldPass bool

		form *Session
	}

	tests := make([]*testCase, 0)

	// pass-all-ok
	test := &testCase{
		_id:         "pass-all-ok",
		_shouldPass: true,
		form:        provideCompleteSession(),
	}
	tests = append(tests, test)

	// fail-nil-session
	test = &testCase{
		_id:         "fail-nil-session",
		_shouldPass: false,
		form:        nil,
	}
	tests = append(tests, test)

	// fail-empty-id
	form := provideCompleteSession()
	form.Meta.SessionId = ""
	test = &testCase{
		_id:         "fail-empty-id",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-invalid-source
	form = provideCompleteSession()
	form.Agent = Agent("openai")
	test = &testCase{
		_id:         "fail-invalid-source",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-empty-source
	form = provideCompleteSession()
	form.Agent = ""
	test = &testCase{
		_id:         "fail-empty-source",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// fail-zero-last-active
	form = provideCompleteSession()
	form.LastActive = time.Time{}
	test = &testCase{
		_id:         "fail-zero-last-active",
		_shouldPass: false,
		form:        form,
	}
	tests = append(tests, test)

	// pass-codex-source
	form = provideCompleteSession()
	form.Agent = AgentCodex
	test = &testCase{
		_id:         "pass-codex-source",
		_shouldPass: true,
		form:        form,
	}
	tests = append(tests, test)

	// fail-nil-turns
	form = provideCompleteSession()
	form.TurnsFinished = nil
	test = &testCase{
		_id:         "fail-nil-turns",
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

func provideUsageTurn(requestId string, outputTokens int) *Turn {
	return &Turn{
		Role:      RoleAssistant,
		Text:      "text",
		Timestamp: time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC),
		RequestId: requestId,
		Usage:     &Usage{InputTokens: 1, OutputTokens: outputTokens},
		Meta:      &Meta{SessionId: Id("sess-123")},
	}
}

func provideCumulativeUsageTurn(totalTokens int) *Turn {
	return &Turn{
		Role:            RoleAssistant,
		Timestamp:       time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC),
		Usage:           &Usage{TotalTokens: totalTokens},
		UsageCumulative: true,
		Meta:            &Meta{SessionId: Id("sess-123")},
	}
}

func TestSession_AddTurn_UsageDedupByRequestId(t *testing.T) {
	s := provideCompleteSession()

	s.AddTurn(provideUsageTurn("req-a", 10))
	s.AddTurn(provideUsageTurn("req-a", 10))
	s.AddTurn(provideUsageTurn("req-b", 20))
	s.AddTurn(provideUsageTurn("req-a", 10))
	s.AddTurn(provideUsageTurn("", 40))

	assert.Equal(t, 30, s.TotalUsage.OutputTokens)
	assert.Equal(t, 2, s.TotalUsage.InputTokens)
}

func TestSession_AddTurn_UsageCountsActiveTurn(t *testing.T) {
	s := provideCompleteSession()

	s.AddTurn(provideUsageTurn("req-a", 10))

	assert.Equal(t, 10, s.TotalUsage.OutputTokens)
	assert.NotNil(t, s.TurnActive)
}

func TestSession_AddTurn_CumulativeUsage(t *testing.T) {
	s := provideCompleteSession()

	s.AddTurn(provideCumulativeUsageTurn(100))
	s.AddTurn(provideCumulativeUsageTurn(250))

	assert.Equal(t, 250, s.TotalUsage.TotalTokens)
	assert.Nil(t, s.TurnActive)
}
