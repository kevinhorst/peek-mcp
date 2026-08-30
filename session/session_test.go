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
		Events:        NewEventBuffer(EventBufferCapacity),
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

func TestSession_AppendTurn(t *testing.T) {
	timestamp := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)
	meta := &Meta{SessionId: Id("sess-123")}

	// first-turn-becomes-active
	buffer := NewTurnBuffer(10)
	first := &Turn{Role: RoleAssistant, Text: "a", RequestId: "req-1", Timestamp: timestamp, Meta: meta}
	active := appendTurn(nil, buffer, first)
	assert.Same(t, first, active)
	assert.Equal(t, 0, buffer.Len())

	// same-request-merges-text-and-thinking
	chunk := &Turn{Role: RoleAssistant, Text: "b", Thinking: "t2", RequestId: "req-1", Timestamp: timestamp, Meta: meta}
	active.Thinking = "t1"
	active = appendTurn(active, buffer, chunk)
	assert.Equal(t, "ab", active.Text)
	assert.Equal(t, "t1t2", active.Thinking)
	assert.Equal(t, 0, buffer.Len())

	// new-request-pushes-active
	next := &Turn{Role: RoleAssistant, Text: "c", RequestId: "req-2", Timestamp: timestamp, Meta: meta}
	active = appendTurn(active, buffer, next)
	assert.Same(t, next, active)
	assert.Equal(t, 1, buffer.Len())

	// thinking-only-active-pushed
	thinkingOnly := &Turn{Role: RoleAssistant, Thinking: "reasoning", RequestId: "req-3", Timestamp: timestamp, Meta: meta}
	active = appendTurn(thinkingOnly, buffer, &Turn{Role: RoleUser, Text: "u", Timestamp: timestamp, Meta: meta})
	assert.Equal(t, 2, buffer.Len())

	// empty-active-dropped
	empty := &Turn{Role: RoleAssistant, RequestId: "req-4", Timestamp: timestamp, Meta: meta}
	appendTurn(empty, buffer, &Turn{Role: RoleUser, Text: "u2", Timestamp: timestamp, Meta: meta})
	assert.Equal(t, 2, buffer.Len())
	_ = active
}

func TestSession_AddSubagentTurn_Transcript(t *testing.T) {
	s := provideCompleteSession()
	timestamp := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)
	meta := &Meta{SessionId: Id("sess-123")}

	// signal-only-turn-buffers-nothing
	s.AddSubagentTurn(&Turn{SubagentId: "ag1", Timestamp: timestamp, Meta: meta})
	turns, ok := s.SubagentTurns("ag1", 10)
	assert.True(t, ok)
	assert.Empty(t, turns)

	// transcript-turns-buffered-per-agent
	s.AddSubagentTurn(&Turn{SubagentId: "ag1", Role: RoleUser, Text: "prompt", Timestamp: timestamp, Meta: meta})
	s.AddSubagentTurn(&Turn{SubagentId: "ag1", Role: RoleAssistant, Text: "answer", Thinking: "why", RequestId: "req-s1", Timestamp: timestamp, Meta: meta})
	turns, ok = s.SubagentTurns("ag1", 10)
	assert.True(t, ok)
	assert.Len(t, turns, 2)
	assert.Equal(t, "answer", turns[0].Text, "active turn prepended, same ordering as Session.Turns")
	assert.Equal(t, "why", turns[0].Thinking)
	assert.Equal(t, "prompt", turns[1].Text)

	// unknown-agent
	_, ok = s.SubagentTurns("nope", 10)
	assert.False(t, ok)

	// sorted-ids
	s.AddSubagentTurn(&Turn{SubagentId: "ag0", Role: RoleUser, Text: "x", Timestamp: timestamp, Meta: meta})
	assert.Equal(t, []string{"ag0", "ag1"}, s.SubagentIds())
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
