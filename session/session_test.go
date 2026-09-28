package session

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func provideCompleteSession() *Session {
	return &Session{
		Meta:          Meta{SessionId: Id("sess-123")},
		Agent:         AgentClaude,
		Events:        NewEventBuffer(EventBufferCapacity, EventBufferDenialBudget),
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

	// same-request-merges-tool-calls
	callChunk := &Turn{Role: RoleAssistant, RequestId: "req-5", Timestamp: timestamp, Meta: meta, ToolCalls: []*ToolCall{{Id: "tu1"}}}
	nextCallChunk := &Turn{Role: RoleAssistant, RequestId: "req-5", Timestamp: timestamp, Meta: meta, ToolCalls: []*ToolCall{{Id: "tu2"}}}
	active = appendTurn(callChunk, buffer, nextCallChunk)
	require.Len(t, active.ToolCalls, 2)
	assert.Equal(t, "tu1", active.ToolCalls[0].Id)
	assert.Equal(t, "tu2", active.ToolCalls[1].Id)
	assert.Equal(t, 2, buffer.Len())

	// tool-only-active-pushed
	appendTurn(active, buffer, &Turn{Role: RoleUser, Text: "u3", Timestamp: timestamp, Meta: meta})
	assert.Equal(t, 3, buffer.Len())
	assert.Equal(t, 2, buffer.Pushed(), "tool-only turn is buffered but not counted")
}

func TestSession_AddDenial(t *testing.T) {
	type testCase struct {
		_id                    string
		_expectedCancellations int
		_expectedDenials       int
		_expectedListed        int

		session *Session
	}

	denialEvent := func(kind, toolUseId string) *Event {
		payload := &PermissionPayload{Kind: kind, Tool: "Bash", ToolUseId: toolUseId}
		return &Event{Kind: EventKindPermissionDenied, Permission: payload}
	}

	tests := make([]*testCase, 0)

	// permission-rule-counted
	counted := provideCompleteSession()
	counted.AddEvent(denialEvent(DenialKindPermissionRule, "tu1"))
	tests = append(tests, &testCase{
		_id:                    "permission-rule-counted",
		_expectedCancellations: 0,
		_expectedDenials:       1,
		_expectedListed:        1,

		session: counted,
	})

	// cancelled-split
	cancelled := provideCompleteSession()
	cancelled.AddEvent(denialEvent(DenialKindCancelled, "tu1"))
	tests = append(tests, &testCase{
		_id:                    "cancelled-split",
		_expectedCancellations: 1,
		_expectedDenials:       0,
		_expectedListed:        1,

		session: cancelled,
	})

	// interrupted-split
	interrupted := provideCompleteSession()
	interrupted.AddEvent(denialEvent(DenialKindInterrupted, "tu1"))
	tests = append(tests, &testCase{
		_id:                    "interrupted-split",
		_expectedCancellations: 1,
		_expectedDenials:       0,
		_expectedListed:        1,

		session: interrupted,
	})

	// nil-payload-counted-as-denial
	nilPayload := provideCompleteSession()
	nilPayload.AddEvent(&Event{Kind: EventKindPermissionDenied})
	tests = append(tests, &testCase{
		_id:                    "nil-payload-counted-as-denial",
		_expectedCancellations: 0,
		_expectedDenials:       1,
		_expectedListed:        0,

		session: nilPayload,
	})

	// listed-up-to-budget
	upToBudget := provideCompleteSession()
	for index := range EventBufferDenialBudget {
		upToBudget.AddEvent(denialEvent(DenialKindPermissionRule, fmt.Sprintf("tu%d", index)))
	}
	tests = append(tests, &testCase{
		_id:                    "listed-up-to-budget",
		_expectedCancellations: 0,
		_expectedDenials:       EventBufferDenialBudget,
		_expectedListed:        EventBufferDenialBudget,

		session: upToBudget,
	})

	// list-capped-at-budget
	capped := provideCompleteSession()
	for index := range EventBufferDenialBudget + 1 {
		capped.AddEvent(denialEvent(DenialKindPermissionRule, fmt.Sprintf("tu%d", index)))
	}
	tests = append(tests, &testCase{
		_id:                    "list-capped-at-budget",
		_expectedCancellations: 0,
		_expectedDenials:       EventBufferDenialBudget + 1,
		_expectedListed:        EventBufferDenialBudget,

		session: capped,
	})

	// others-keep-denials
	othersFirst := provideCompleteSession()
	for range EventBufferCapacity - EventBufferDenialBudget {
		othersFirst.AddEvent(&Event{Kind: EventKindPlanApproved})
	}
	othersFirst.AddEvent(denialEvent(DenialKindPermissionRule, "tu1"))
	tests = append(tests, &testCase{
		_id:                    "others-keep-denials",
		_expectedCancellations: 0,
		_expectedDenials:       1,
		_expectedListed:        1,

		session: othersFirst,
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			assert.Equal(t, test._expectedDenials, test.session.Counters.PermissionDenials)
			assert.Equal(t, test._expectedCancellations, test.session.Counters.PermissionCancellations)
			assert.Len(t, test.session.Denials(), test._expectedListed)
		})
	}
}

func TestSession_TurnsWithToolCalls(t *testing.T) {
	timestamp := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)
	meta := &Meta{SessionId: Id("sess-123")}

	// finished-tool-only-hidden-from-turns
	s := provideCompleteSession()
	s.AddTurn(&Turn{Role: RoleUser, Text: "go", RequestId: "r1", Timestamp: timestamp, Meta: meta})
	s.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r2", Timestamp: timestamp, Meta: meta, ToolCalls: []*ToolCall{{Id: "tu1"}}})
	s.AddTurn(&Turn{Role: RoleAssistant, Text: "done", RequestId: "r3", Timestamp: timestamp, Meta: meta})
	turns := s.Turns(AllTurns)
	require.Len(t, turns, 2)
	assert.Equal(t, "go", turns[0].Text)
	assert.Equal(t, "done", turns[1].Text)

	// active-tool-only-kept-in-turns
	s = provideCompleteSession()
	s.AddTurn(&Turn{Role: RoleUser, Text: "go", RequestId: "r1", Timestamp: timestamp, Meta: meta})
	s.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r2", Timestamp: timestamp, Meta: meta, ToolCalls: []*ToolCall{{Id: "tu1"}}})
	turns = s.Turns(AllTurns)
	require.Len(t, turns, 2)
	assert.Equal(t, "r2", turns[1].RequestId, "in-progress tool-only turn stays, as in v1.2.8")

	// with-tool-calls-returns-all
	s = provideCompleteSession()
	s.AddTurn(&Turn{Role: RoleUser, Text: "go", RequestId: "r1", Timestamp: timestamp, Meta: meta})
	s.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r2", Timestamp: timestamp, Meta: meta, ToolCalls: []*ToolCall{{Id: "tu1"}}})
	s.AddTurn(&Turn{Role: RoleAssistant, Text: "done", RequestId: "r3", Timestamp: timestamp, Meta: meta})
	turns = s.TurnsWithToolCalls(AllTurns)
	require.Len(t, turns, 3)
	assert.Equal(t, "tu1", turns[1].ToolCalls[0].Id)

	// number-counts-tool-only-turns
	turns = s.TurnsWithToolCalls(2)
	require.Len(t, turns, 2)
	assert.Equal(t, "r2", turns[0].RequestId)
	assert.Equal(t, "r3", turns[1].RequestId)
}

func TestSession_TotalTurns_ToolOnlyNotCounted(t *testing.T) {
	s := provideCompleteSession()
	timestamp := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)
	meta := &Meta{SessionId: Id("sess-123")}

	s.AddTurn(&Turn{Role: RoleUser, Text: "go", RequestId: "r1", Timestamp: timestamp, Meta: meta})
	s.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r2", Timestamp: timestamp, Meta: meta, ToolCalls: []*ToolCall{{Id: "tu1"}}})
	s.AddTurn(&Turn{Role: RoleAssistant, Text: "done", RequestId: "r3", Timestamp: timestamp, Meta: meta})

	assert.Equal(t, 2, s.TotalTurns())
}

func TestSession_TurnsTotal(t *testing.T) {
	type testCase struct {
		_id       string
		_expected int
		total     func() int
	}

	timestamp := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)
	meta := &Meta{SessionId: Id("sess-123")}

	mixed := provideCompleteSession()
	mixed.AddTurn(&Turn{Role: RoleUser, Text: "go", RequestId: "r1", Timestamp: timestamp, Meta: meta})
	mixed.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r2", Timestamp: timestamp, Meta: meta, ToolCalls: []*ToolCall{{Id: "tu1"}}})
	mixed.AddTurn(&Turn{Role: RoleAssistant, Text: "done", RequestId: "r3", Timestamp: timestamp, Meta: meta})
	mixed.AddSubagentTurn(&Turn{SubagentId: "ag1", Role: RoleUser, Text: "prompt", Timestamp: timestamp, Meta: meta})
	mixed.AddSubagentTurn(&Turn{SubagentId: "ag1", Role: RoleAssistant, RequestId: "s1", Timestamp: timestamp, Meta: meta, ToolCalls: []*ToolCall{{Id: "tu2"}}})
	mixed.AddSubagentTurn(&Turn{SubagentId: "ag1", Role: RoleAssistant, Text: "answer", RequestId: "s2", Timestamp: timestamp, Meta: meta})

	evicted := provideCompleteSession()
	for index := range 221 {
		requestId := fmt.Sprintf("s%d", index)
		evicted.AddSubagentTurn(&Turn{SubagentId: "ag1", Role: RoleAssistant, RequestId: requestId, Timestamp: timestamp, Meta: meta, ToolCalls: []*ToolCall{{Id: requestId}}})
	}
	evictedTurns, _ := evicted.SubagentTurnsWithToolCalls("ag1", AllTurns)

	tests := make([]*testCase, 0)

	// root-default-conversational-plus-active
	tests = append(tests, &testCase{
		_id:       "root-default-conversational-plus-active",
		_expected: 2,
		total:     mixed.TotalTurns,
	})

	// root-tools-includes-tool-only
	tests = append(tests, &testCase{
		_id:       "root-tools-includes-tool-only",
		_expected: 3,
		total:     mixed.TotalTurnsWithToolCalls,
	})

	// subagent-default
	tests = append(tests, &testCase{
		_id:       "subagent-default",
		_expected: 2,
		total:     func() int { return mixed.SubagentTotalTurns("ag1") },
	})

	// subagent-tools-includes-tool-only
	tests = append(tests, &testCase{
		_id:       "subagent-tools-includes-tool-only",
		_expected: 3,
		total:     func() int { return mixed.SubagentTotalTurnsWithToolCalls("ag1") },
	})

	// unknown-subagent-zero
	tests = append(tests, &testCase{
		_id:       "unknown-subagent-zero",
		_expected: 0,
		total:     func() int { return mixed.SubagentTotalTurnsWithToolCalls("nope") },
	})

	// evicted-subagent-turns-still-counted
	tests = append(tests, &testCase{
		_id:       "evicted-subagent-turns-still-counted",
		_expected: 221,
		total:     func() int { return evicted.SubagentTotalTurnsWithToolCalls("ag1") },
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			assert.Equal(t, test._expected, test.total())
		})
	}

	assert.Len(t, evictedTurns, 201, "ring floor 200 plus the active turn")
}

func TestSession_AddSubagentTurn_ToolCalls(t *testing.T) {
	timestamp := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)
	meta := &Meta{SessionId: Id("sess-123")}

	// calls-buffered-per-agent
	s := provideCompleteSession()
	s.AddSubagentTurn(&Turn{SubagentId: "ag1", Role: RoleAssistant, RequestId: "s1", Timestamp: timestamp, Meta: meta, ToolCalls: []*ToolCall{{Id: "tu1"}}})
	s.AddSubagentTurn(&Turn{SubagentId: "ag1", Role: RoleAssistant, RequestId: "s2", Timestamp: timestamp, Meta: meta, ToolCalls: []*ToolCall{{Id: "tu2"}}})
	turns, ok := s.SubagentTurnsWithToolCalls("ag1", 10)
	require.True(t, ok)
	require.Len(t, turns, 2)
	assert.Equal(t, "tu1", turns[0].ToolCalls[0].Id)
	assert.Equal(t, "tu2", turns[1].ToolCalls[0].Id)

	// result-sets-is-error-on-active
	s.AddSubagentTurn(&Turn{SubagentId: "ag1", Meta: meta, ToolResults: []*ToolResult{{IsError: true, ToolUseId: "tu2"}}})
	assert.True(t, s.Subagents["ag1"].TurnActive.ToolCalls[0].IsError)

	// result-sets-is-error-on-finished
	s.AddSubagentTurn(&Turn{SubagentId: "ag1", Meta: meta, ToolResults: []*ToolResult{{IsError: true, ToolUseId: "tu1"}}})
	assert.True(t, s.Subagents["ag1"].Turns.items[0].ToolCalls[0].IsError)

	// unknown-result-ignored
	s.AddSubagentTurn(&Turn{SubagentId: "ag1", Meta: meta, ToolResults: []*ToolResult{{IsError: true, ToolUseId: "missing"}}})
	turns, _ = s.SubagentTurnsWithToolCalls("ag1", 10)
	assert.Len(t, turns, 2)

	// ring-follows-depth-floor-200
	assert.Equal(t, minSubagentTurnDepth, s.Subagents["ag1"].Turns.capacity)
	deep := provideCompleteSession()
	deep.TurnsFinished = NewTurnBuffer(500)
	deep.AddSubagentTurn(&Turn{SubagentId: "ag1", Role: RoleUser, Text: "prompt", Timestamp: timestamp, Meta: meta})
	assert.Equal(t, 500, deep.Subagents["ag1"].Turns.capacity)
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
	assert.Equal(t, "prompt", turns[0].Text)
	assert.Equal(t, "answer", turns[1].Text, "active turn appended last, same ordering as Session.Turns")
	assert.Equal(t, "why", turns[1].Thinking)

	// unknown-agent
	_, ok = s.SubagentTurns("nope", 10)
	assert.False(t, ok)

	// sorted-ids
	s.AddSubagentTurn(&Turn{SubagentId: "ag0", Role: RoleUser, Text: "x", Timestamp: timestamp, Meta: meta})
	assert.Equal(t, []string{"ag0", "ag1"}, s.SubagentIds())

	// model-captured-from-turn-meta
	s.AddSubagentTurn(&Turn{SubagentId: "ag1", Role: RoleAssistant, Text: "y", RequestId: "req-s2", Timestamp: timestamp, Meta: &Meta{SessionId: Id("sess-123"), Model: "claude-haiku-4-5-20251001"}})
	assert.Equal(t, "claude-haiku-4-5-20251001", s.Subagents["ag1"].Model)
}

func TestSession_AddSubagentTurn_TouchedFiles(t *testing.T) {
	s := provideCompleteSession()
	timestamp := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)
	meta := &Meta{SessionId: Id("sess-123")}

	s.AddSubagentTurn(&Turn{
		SubagentId: "ag1",
		Timestamp:  timestamp,
		Meta:       meta,
		FileTouches: []*FileTouch{
			{Path: "/a.go"},
			{Path: "/a.go", Write: true},
			{Path: "/b.go", Write: true},
		},
	})

	stat := s.Subagents["ag1"]
	require.NotNil(t, stat)
	require.Len(t, stat.TouchedFiles, 2)
	assert.Equal(t, 1, stat.TouchedFiles["/a.go"].Reads)
	assert.Equal(t, 1, stat.TouchedFiles["/a.go"].Writes)
	assert.Equal(t, 1, stat.TouchedFiles["/b.go"].Writes)
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

func TestSession_AddTurn_UsageKeepsLastChunkPerRequestId(t *testing.T) {
	s := provideCompleteSession()

	s.AddTurn(provideUsageTurn("req-a", 10))
	s.AddTurn(provideUsageTurn("req-a", 15))
	s.AddTurn(provideUsageTurn("req-b", 20))

	assert.Equal(t, 35, s.TotalUsage.OutputTokens)
	assert.Equal(t, 2, s.TotalUsage.InputTokens)
}

func TestSession_AddSubagentTurn_UsageKeepsLastChunkPerRequestId(t *testing.T) {
	s := provideCompleteSession()
	subagentTurn := func(requestId string, outputTokens int) *Turn {
		turn := provideUsageTurn(requestId, outputTokens)
		turn.SubagentId = "ag1"
		return turn
	}

	s.AddSubagentTurn(subagentTurn("req-a", 10))
	s.AddSubagentTurn(subagentTurn("req-a", 15))
	s.AddSubagentTurn(subagentTurn("req-a", 15))
	s.AddSubagentTurn(subagentTurn("req-b", 20))

	assert.Equal(t, 35, s.Subagents["ag1"].Usage.OutputTokens)
	assert.Equal(t, 2, s.Subagents["ag1"].Usage.InputTokens)
}

func TestSession_AddTurn_UsageCountsActiveTurn(t *testing.T) {
	s := provideCompleteSession()

	s.AddTurn(provideUsageTurn("req-a", 10))

	assert.Equal(t, 10, s.TotalUsage.OutputTokens)
	assert.NotNil(t, s.TurnActive)
}

func TestSession_AddFileTouch_Cap(t *testing.T) {
	s := &Session{TouchedFiles: make(map[string]*FileTouchCounts)}
	for i := 0; i < maxTouchedFiles; i++ {
		s.AddFileTouch(&FileTouch{Path: fmt.Sprintf("/f/%d.go", i)})
	}
	assert.Len(t, s.TouchedFiles, maxTouchedFiles)

	s.AddFileTouch(&FileTouch{Path: "/f/overflow.go"})
	assert.Len(t, s.TouchedFiles, maxTouchedFiles, "path beyond the cap is dropped")
	_, ok := s.TouchedFiles["/f/overflow.go"]
	assert.False(t, ok)
}

func TestSession_Turns_ActiveLast(t *testing.T) {
	s := &Session{TurnsFinished: NewTurnBuffer(100)}
	ts := time.Now()
	s.AddTurn(&Turn{Role: RoleUser, Text: "first", Timestamp: ts, Meta: &Meta{SessionId: "s"}})
	s.AddTurn(&Turn{Role: RoleAssistant, Text: "active", RequestId: "r1", Timestamp: ts, Meta: &Meta{SessionId: "s"}})

	turns := s.Turns(10)
	require.Len(t, turns, 2)
	assert.Equal(t, "first", turns[0].Text)
	assert.Equal(t, "active", turns[1].Text, "in-progress turn is last, never dropped")
}

func TestSession_TotalTurns_BeyondRingCap(t *testing.T) {
	s := &Session{TurnsFinished: NewTurnBuffer(2)}
	ts := time.Now()
	for index := range 5 {
		s.AddTurn(&Turn{Role: RoleUser, Text: "t", RequestId: fmt.Sprintf("r%d", index), Timestamp: ts, Meta: &Meta{SessionId: "s"}})
	}

	assert.Equal(t, 5, s.TotalTurns(), "parse-time count survives the ring cap")
	assert.Equal(t, 2, s.TurnsFinished.Len())
}

func TestSession_Turns_ActiveKeptOverOldest(t *testing.T) {
	s := &Session{TurnsFinished: NewTurnBuffer(2)}
	ts := time.Now()
	s.AddTurn(&Turn{Role: RoleUser, Text: "t0", RequestId: "a", Timestamp: ts, Meta: &Meta{SessionId: "s"}})
	s.AddTurn(&Turn{Role: RoleUser, Text: "t1", RequestId: "b", Timestamp: ts, Meta: &Meta{SessionId: "s"}})
	s.AddTurn(&Turn{Role: RoleAssistant, Text: "active", RequestId: "c", Timestamp: ts, Meta: &Meta{SessionId: "s"}})

	turns := s.Turns(2)
	require.Len(t, turns, 2)
	assert.Equal(t, "active", turns[len(turns)-1].Text, "active turn survives the cap")
}

func TestSession_SkillWindowsPerActor(t *testing.T) {
	s := provideCompleteSession()
	now := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)

	s.AddEvent(&Event{Kind: EventKindSkillInvoked, Skill: &SkillPayload{Skill: "main-skill"}, Timestamp: now})
	s.AddEvent(&Event{Kind: EventKindSkillInvoked, Actor: "ag1", Skill: &SkillPayload{Skill: "sub-skill"}, Timestamp: now})

	require.Len(t, s.Skills, 2)
	assert.Equal(t, "", s.Skills[0].AgentId, "main window keeps an empty actor")
	assert.Equal(t, "ag1", s.Skills[1].AgentId)

	s.AddTurn(provideUsageTurn("req-main", 10))
	assert.Equal(t, 10, s.Skills[0].Usage.OutputTokens, "main turn usage lands on main's window")
	assert.Equal(t, 0, s.Skills[1].Usage.OutputTokens, "subagent window untouched by main turns")

	s.AddSubagentTurn(&Turn{
		SubagentId: "ag1",
		Role:       RoleAssistant,
		RequestId:  "req-sub",
		Timestamp:  now.Add(time.Minute),
		Usage:      &Usage{OutputTokens: 7},
		Meta:       &Meta{SessionId: Id("sess-123"), Model: "haiku"},
	})
	assert.Equal(t, 10, s.Skills[0].Usage.OutputTokens, "main window untouched by subagent turns")
	assert.Equal(t, 7, s.Skills[1].Usage.OutputTokens, "subagent turn usage lands on its actor's window")
	assert.Equal(t, "haiku", s.Skills[1].Model)

	s.CloseSkillWindow(now.Add(2 * time.Minute))
	s.AddSubagentTurn(&Turn{
		SubagentId: "ag1",
		Role:       RoleAssistant,
		RequestId:  "req-sub-2",
		Timestamp:  now.Add(3 * time.Minute),
		Usage:      &Usage{OutputTokens: 5},
		Meta:       &Meta{SessionId: Id("sess-123")},
	})
	assert.Equal(t, 12, s.Skills[1].Usage.OutputTokens, "prompt boundary closes only main's window")

	s.AddEvent(&Event{Kind: EventKindSkillInvoked, Actor: "ag1", Skill: &SkillPayload{Skill: "sub-skill-2"}, Timestamp: now.Add(4 * time.Minute)})
	require.Len(t, s.Skills, 3)
	assert.Equal(t, now.Add(3*time.Minute), s.Skills[1].EndedAt, "next skill of the same actor closes its predecessor")
}
