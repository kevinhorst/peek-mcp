package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/telemetry"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func provideToolStore() *session.Store {
	s := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude, session.AgentCodex)
	now := time.Now()

	s.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		Role:      session.RoleUser,
		Text:      "What does this do?",
		Timestamp: now.Add(-1 * time.Hour),
		Meta:      &session.Meta{SessionId: "s1", CWD: "/project", GitBranch: "main"},
	})
	s.AddTurnBySessionId("s2", session.AgentCodex, &session.Turn{
		Role:      session.RoleUser,
		Text:      "Refactor auth",
		Timestamp: now,
		Meta:      &session.Meta{SessionId: "s2", CWD: "/project", GitBranch: "feat"},
	})

	s1, _ := s.GetById("s1")
	s1.PlanContent = "# Plan"
	s1.DiffOutput = "diff-output"
	s1.DiffSource = session.DiffSourceLive
	s1.DiffTarget = "main"
	s1.UncommittedDiff = "uncommitted-output"

	return s
}

func providePageStore() *PageStore[*sessionGetResult] {
	return &PageStore[*sessionGetResult]{PagesByRequestId: make(map[string]<-chan *sessionGetResult)}
}

func requestWithArgs(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}

func decodeResult(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	require.False(t, result.IsError)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	payload := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &payload))
	return payload
}

func errorText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.True(t, result.IsError)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

func TestSessionList_ProjectFilter(t *testing.T) {
	s := provideToolStore()
	s.AddTurnBySessionId("s3", session.AgentClaude, &session.Turn{
		Role:      session.RoleUser,
		Text:      "Cowork task",
		Timestamp: time.Now(),
		Meta:      &session.Meta{SessionId: "s3", CWD: "/store/local_x/outputs", Project: "cowork"},
	})
	handler := sessionListHandler(s)

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"project": "cowork"}))
	require.NoError(t, err)
	payload, ok := result.StructuredContent.(map[string]any)
	require.True(t, ok)
	sessions := payload["sessions"].([]sessionListItem)
	require.Len(t, sessions, 1)
	assert.Equal(t, "cowork", sessions[0].Meta.Project)

	result, err = handler(context.Background(), requestWithArgs(map[string]any{}))
	require.NoError(t, err)
	payload, ok = result.StructuredContent.(map[string]any)
	require.True(t, ok)
	assert.Len(t, payload["sessions"].([]sessionListItem), 3)
}

func TestSessionGet_Defaults(t *testing.T) {
	store := provideToolStore()
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"agent": "claude"}))

	assert.NoError(t, err)
	payload := decodeResult(t, result)
	assert.Contains(t, payload, "turns")
	assert.Equal(t, "# Plan", payload["plan"])
	assert.Equal(t, "diff-output", payload["diff"])
	assert.Equal(t, "main", payload["diff_target"])
	assert.NotContains(t, payload, "uncommitted_diff")
	assert.Equal(t, false, payload["has_more"])
}

func TestSessionGet_PlanOnly(t *testing.T) {
	store := provideToolStore()
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{
		"agent": "claude",
		"turns": false,
		"diff":  false,
	}))

	assert.NoError(t, err)
	payload := decodeResult(t, result)
	assert.Equal(t, "# Plan", payload["plan"])
	assert.NotContains(t, payload, "turns")
	assert.NotContains(t, payload, "diff")
	assert.NotContains(t, payload, "diff_target")
}

func TestSessionGet_UncommittedDiff(t *testing.T) {
	store := provideToolStore()
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{
		"agent":            "claude",
		"turns":            false,
		"plan":             false,
		"diff":             false,
		"uncommitted_diff": true,
	}))

	assert.NoError(t, err)
	payload := decodeResult(t, result)
	assert.Equal(t, "uncommitted-output", payload["uncommitted_diff"])
	assert.NotContains(t, payload, "turns")
	assert.NotContains(t, payload, "plan")
	assert.NotContains(t, payload, "diff")
}

func TestSessionGet_ById(t *testing.T) {
	store := provideToolStore()
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1"}))
	assert.NoError(t, err)
	payload := decodeResult(t, result)
	assert.Equal(t, "# Plan", payload["plan"])

	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "nope"}))
	assert.NoError(t, err)
	assert.Contains(t, errorText(t, result), "not found")
}

func TestSessionGet_LatestFallback(t *testing.T) {
	store := provideToolStore()
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"agent": "claude"}))

	assert.NoError(t, err)
	payload := decodeResult(t, result)
	assert.Contains(t, payload["turns"], "What does this do?")
}

func TestSessionGet_AgentRequired(t *testing.T) {
	store := provideToolStore()
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{}))

	assert.NoError(t, err)
	assert.Contains(t, errorText(t, result), "agent parameter is required")
}

func TestSessionGet_NoSessions(t *testing.T) {
	store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{}))

	assert.NoError(t, err)
	text, ok := result.Content[0].(mcp.TextContent)
	assert.True(t, ok)
	assert.Equal(t, "no sessions found", text.Text)
}

func TestSessionGet_TurnCount(t *testing.T) {
	store := provideToolStore()
	now := time.Now()
	for i := 0; i < 3; i++ {
		store.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
			Role:      session.RoleUser,
			Text:      "extra turn",
			Timestamp: now,
			Meta:      &session.Meta{SessionId: "s1"},
		})
	}
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{
		"id":   "s1",
		"n":    float64(1),
		"plan": false,
		"diff": false,
	}))

	assert.NoError(t, err)
	payload := decodeResult(t, result)
	turns := []map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(payload["turns"].(string)), &turns))
	assert.Len(t, turns, 1)
}

func TestSessionGet_Pagination(t *testing.T) {
	store := provideToolStore()
	s1, _ := store.GetById("s1")
	s1.DiffOutput = strings.Repeat("d", MaxResponseBytesClaude*2)
	s1.DiffSource = session.DiffSourceLive
	pageStore := providePageStore()
	handler := sessionGetHandler(store, pageStore)

	result, err := handler(context.Background(), requestWithArgs(map[string]any{
		"id":    "s1",
		"turns": false,
		"plan":  false,
	}))

	assert.NoError(t, err)
	payload := decodeResult(t, result)
	assert.Equal(t, true, payload["has_more"])
	requestId, ok := payload["request_id"].(string)
	require.True(t, ok)

	result, err = handler(context.Background(), requestWithArgs(map[string]any{"request_id": requestId}))
	assert.NoError(t, err)
	payload = decodeResult(t, result)
	assert.NotEmpty(t, payload["diff"])

	result, err = handler(context.Background(), requestWithArgs(map[string]any{"request_id": "unknown"}))
	assert.NoError(t, err)
	assert.Contains(t, errorText(t, result), "not found or expired")
}

func TestSessionGet_JsonTypedUnpaginated(t *testing.T) {
	store := provideToolStore()
	s1, _ := store.GetById("s1")
	s1.DiffOutput = strings.Repeat("d", MaxResponseBytesClaude*2)
	s1.DiffSource = session.DiffSourceLive
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "json": true}))

	assert.NoError(t, err)
	require.False(t, result.IsError)
	payload, ok := result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	turns, ok := payload.Turns.([]*turnView)
	require.True(t, ok)
	assert.Equal(t, "What does this do?", turns[0].Text)
	assert.Equal(t, s1.DiffOutput, payload.Diff)
	assert.NotNil(t, payload.TurnsTotal)
}

func provideSubagentStore() *session.Store {
	s := provideToolStore()
	now := time.Now()
	meta := &session.Meta{SessionId: "s1"}

	s.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		SubagentId: "ag1",
		Events: []*session.Event{{
			Kind:      session.EventKindSubagentSpawned,
			Actor:     "ag1",
			Subagent:  &session.SubagentPayload{AgentId: "ag1", AgentType: "Explore", Description: "scan"},
			Timestamp: now,
		}},
		Timestamp: now,
		Meta:      meta,
	})
	s.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		SubagentId: "ag1", Role: session.RoleUser, Text: "sub prompt", Timestamp: now, Meta: meta,
	})
	s.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		SubagentId: "ag1", Role: session.RoleAssistant, Text: "sub answer", Thinking: "sub think", RequestId: "r-sub", Timestamp: now, Meta: meta,
	})
	s.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		Role: session.RoleAssistant, Text: "main answer", Thinking: "main think", RequestId: "r-main", Timestamp: now, Meta: meta,
	})
	return s
}

func TestSessionGet_Subagent(t *testing.T) {
	store := provideSubagentStore()
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "subagent": "ag1"}))

	assert.NoError(t, err)
	payload := decodeResult(t, result)
	turns, ok := payload["turns"].(string)
	require.True(t, ok)
	assert.Contains(t, turns, "sub answer")
	assert.NotContains(t, turns, "main answer")
	assert.NotContains(t, payload, "plan")
	assert.NotContains(t, payload, "diff")
	events, ok := payload["events"].(string)
	require.True(t, ok)
	assert.Contains(t, events, "ag1")
	subagents, ok := payload["subagents"].([]any)
	require.True(t, ok)
	require.Len(t, subagents, 1)
	assert.Equal(t, "ag1", subagents[0].(map[string]any)["agent_id"])
	assert.Equal(t, "Explore", subagents[0].(map[string]any)["agent_type"])
}

func TestSessionGet_SubagentUnknown(t *testing.T) {
	store := provideSubagentStore()
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "subagent": "nope"}))

	assert.NoError(t, err)
	text := errorText(t, result)
	assert.Contains(t, text, "unknown subagent")
	assert.Contains(t, text, "ag1")
}

func TestSessionGet_SubagentListWithoutParam(t *testing.T) {
	store := provideSubagentStore()
	handler := sessionGetHandler(store, providePageStore())

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1"}))

	assert.NoError(t, err)
	payload := decodeResult(t, result)
	subagents, ok := payload["subagents"].([]any)
	require.True(t, ok)
	require.Len(t, subagents, 1)
	assert.Equal(t, "ag1", subagents[0].(map[string]any)["agent_id"])
}

func TestSessionGet_Thinking(t *testing.T) {
	store := provideSubagentStore()
	handler := sessionGetHandler(store, providePageStore())

	// default-strips-thinking
	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1"}))
	assert.NoError(t, err)
	payload := decodeResult(t, result)
	turns, ok := payload["turns"].(string)
	require.True(t, ok)
	assert.NotContains(t, turns, "main think")

	// thinking-true-includes
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "thinking": true}))
	assert.NoError(t, err)
	payload = decodeResult(t, result)
	turns, ok = payload["turns"].(string)
	require.True(t, ok)
	assert.Contains(t, turns, "main think")
}

func provideToolCallStore() *session.Store {
	s := provideSubagentStore()
	now := time.Now()
	meta := &session.Meta{SessionId: "s1"}

	s.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		Role: session.RoleAssistant, RequestId: "r-read", Timestamp: now, Meta: meta,
		ToolCalls: []*session.ToolCall{{Id: "tu-read", Input: json.RawMessage(`{"file_path":"/a.go"}`), Name: "Read", Timestamp: now}},
	})
	s.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		Role: session.RoleAssistant, Text: "running", RequestId: "r-bash", Timestamp: now, Meta: meta,
		ToolCalls: []*session.ToolCall{{Id: "tu-bash", Input: json.RawMessage(`{"command":"ls"}`), Name: "Bash", Timestamp: now}},
	})
	s.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		Meta: meta, ToolResults: []*session.ToolResult{{IsError: true, ToolUseId: "tu-bash"}},
	})
	s.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		SubagentId: "ag1", Role: session.RoleAssistant, RequestId: "r-sub-grep", Timestamp: now, Meta: meta,
		ToolCalls: []*session.ToolCall{{Id: "tu-sub", Input: json.RawMessage(`{}`), Name: "Grep", Timestamp: now}},
	})
	return s
}

func turnsFromJson(t *testing.T, payload map[string]any) []map[string]any {
	t.Helper()
	turns := []map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(payload["turns"].(string)), &turns))
	return turns
}

func TestSessionGet_Tools(t *testing.T) {
	store := provideToolCallStore()
	handler := sessionGetHandler(store, providePageStore())

	// default-omits-tool-calls
	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1"}))
	assert.NoError(t, err)
	payload := decodeResult(t, result)
	assert.NotContains(t, payload["turns"].(string), `"tool_calls"`)

	// default-hides-tool-only-turn
	turns := turnsFromJson(t, payload)
	require.Len(t, turns, 3)
	assert.Equal(t, "main answer", turns[1]["text"])
	assert.Equal(t, "running", turns[2]["text"])

	// tools-true-carries-calls
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "tools": true}))
	assert.NoError(t, err)
	payload = decodeResult(t, result)
	turns = turnsFromJson(t, payload)
	require.Len(t, turns, 4)
	bashCalls := turns[3]["tool_calls"].([]any)
	require.Len(t, bashCalls, 1)
	bashCall := bashCalls[0].(map[string]any)
	assert.Equal(t, "tu-bash", bashCall["id"])
	assert.Equal(t, "Bash", bashCall["name"])
	assert.Equal(t, true, bashCall["is_error"])
	assert.Equal(t, map[string]any{"command": "ls"}, bashCall["input"])
	assert.NotEmpty(t, bashCall["timestamp"])

	// tools-true-text-turn-empty-list
	assert.Equal(t, []any{}, turns[0]["tool_calls"])

	// tools-true-includes-tool-only-turn
	readCalls := turns[2]["tool_calls"].([]any)
	require.Len(t, readCalls, 1)
	assert.Equal(t, "Read", readCalls[0].(map[string]any)["name"])
	assert.Equal(t, false, readCalls[0].(map[string]any)["is_error"])

	// subagent-scope-calls
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "subagent": "ag1", "tools": true}))
	assert.NoError(t, err)
	payload = decodeResult(t, result)
	turns = turnsFromJson(t, payload)
	require.Len(t, turns, 3)
	subCalls := turns[2]["tool_calls"].([]any)
	require.Len(t, subCalls, 1)
	assert.Equal(t, "tu-sub", subCalls[0].(map[string]any)["id"])
	assert.NotContains(t, payload["turns"].(string), "tu-bash")

	// json-typed-view
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "tools": true, "json": true}))
	assert.NoError(t, err)
	typedPayload, ok := result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	views, ok := typedPayload.Turns.([]*turnView)
	require.True(t, ok)
	require.Len(t, views, 4)
	assert.Equal(t, "tu-bash", views[3].ToolCalls[0].Id)
	assert.Nil(t, views[3].Turn.ToolCalls, "the embedded turn never carries live calls")
}

func TestSessionGet_TurnsTotal(t *testing.T) {
	store := provideToolCallStore()
	handler := sessionGetHandler(store, providePageStore())
	s1, _ := store.GetById("s1")
	defaultTotal := s1.TotalTurns()

	// json-root-default
	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "json": true}))
	assert.NoError(t, err)
	payload, ok := result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	require.NotNil(t, payload.TurnsTotal)
	assert.Equal(t, defaultTotal, *payload.TurnsTotal)

	// json-root-tools-counts-tool-only
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "json": true, "tools": true}))
	assert.NoError(t, err)
	payload, ok = result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	require.NotNil(t, payload.TurnsTotal)
	assert.Equal(t, defaultTotal+1, *payload.TurnsTotal)

	// json-subagent-scope
	subagentHandler := sessionGetHandler(provideSubagentStore(), providePageStore())
	result, err = subagentHandler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "json": true, "subagent": "ag1"}))
	assert.NoError(t, err)
	payload, ok = result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	require.NotNil(t, payload.TurnsTotal)
	assert.Equal(t, 2, *payload.TurnsTotal)

	// n-below-total
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "json": true, "n": float64(1)}))
	assert.NoError(t, err)
	payload, ok = result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	assert.Len(t, payload.Turns.([]*turnView), 1)
	assert.Equal(t, defaultTotal, *payload.TurnsTotal)

	// paginated-first-page-carries-total
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1"}))
	assert.NoError(t, err)
	pagePayload := decodeResult(t, result)
	assert.Equal(t, float64(defaultTotal), pagePayload["turns_total"])

	// turns-false-still-present
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "turns": false}))
	assert.NoError(t, err)
	pagePayload = decodeResult(t, result)
	assert.NotContains(t, pagePayload, "turns")
	assert.Equal(t, float64(defaultTotal), pagePayload["turns_total"])
}

func provideSubagentUsageStore() *session.Store {
	s := provideSubagentStore()
	now := time.Now()

	s.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
		SubagentId: "ag1",
		Role:       session.RoleAssistant,
		Text:       "sub usage",
		RequestId:  "r-sub-usage",
		Timestamp:  now,
		Usage:      &session.Usage{InputTokens: 7, OutputTokens: 11},
		Meta:       &session.Meta{SessionId: "s1", Model: "claude-sonnet-5"},
	})
	s.AddTurnBySessionId("s2", session.AgentCodex, &session.Turn{
		SubagentId: "cx1",
		Role:       session.RoleAssistant,
		Text:       "codex sub answer",
		Timestamp:  now,
		Meta:       &session.Meta{SessionId: "s2", Model: "gpt-5.5"},
	})

	s1, _ := s.GetById("s1")
	s1.PlanRevisions = []*session.PlanRevision{
		{Index: 0, Timestamp: now.Add(-time.Minute)},
		{Index: 1, Timestamp: now},
	}
	return s
}

func TestSessionGet_WatchHeader(t *testing.T) {
	store := provideSubagentUsageStore()
	handler := sessionGetHandler(store, providePageStore())
	s1, _ := store.GetById("s1")

	// json-subagent-model-last-active-usage
	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "json": true, "turns": false, "events": false, "plan": false, "diff": false}))
	assert.NoError(t, err)
	payload, ok := result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	require.Len(t, payload.Subagents, 1)
	assert.Equal(t, "claude-sonnet-5", payload.Subagents[0].Model)
	assert.Equal(t, s1.Subagents["ag1"].LastActive, payload.Subagents[0].LastActive)
	require.NotNil(t, payload.Subagents[0].Usage)
	assert.Equal(t, 7, payload.Subagents[0].Usage.InputTokens)
	assert.Equal(t, 11, payload.Subagents[0].Usage.OutputTokens)

	// json-plan-revisions-and-last-active
	require.NotNil(t, payload.PlanRevisions)
	assert.Equal(t, 2, payload.PlanRevisions.Count)
	assert.Len(t, payload.PlanRevisions.Timestamps, 2)
	assert.Equal(t, s1.LastActive, payload.LastActive)

	// paginated-first-page-carries-header
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "turns": false, "events": false, "plan": false, "diff": false}))
	assert.NoError(t, err)
	pagePayload := decodeResult(t, result)
	subagents, ok := pagePayload["subagents"].([]any)
	require.True(t, ok)
	subagent := subagents[0].(map[string]any)
	assert.Equal(t, "claude-sonnet-5", subagent["model"])
	assert.Contains(t, subagent, "last_active")
	assert.Equal(t, float64(11), subagent["usage"].(map[string]any)["output_tokens"])
	assert.Equal(t, float64(2), pagePayload["plan_revisions"].(map[string]any)["count"])
	assert.Contains(t, pagePayload, "last_active")

	// subagent-scope-keeps-header
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "json": true, "subagent": "ag1"}))
	assert.NoError(t, err)
	payload, ok = result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	assert.NotNil(t, payload.PlanRevisions)
	assert.Len(t, payload.Subagents, 1)

	// codex-subagent-omits-usage-and-no-revisions
	result, err = handler(context.Background(), requestWithArgs(map[string]any{"id": "s2", "json": true, "turns": false, "events": false, "plan": false, "diff": false}))
	assert.NoError(t, err)
	payload, ok = result.StructuredContent.(*sessionGetResult)
	require.True(t, ok)
	require.Len(t, payload.Subagents, 1)
	assert.Equal(t, "gpt-5.5", payload.Subagents[0].Model)
	assert.Nil(t, payload.Subagents[0].Usage)
	assert.Nil(t, payload.PlanRevisions)
}

func TestSessionEvents_Subagent(t *testing.T) {
	store := provideSubagentStore()
	s1, _ := store.GetById("s1")
	s1.AddEvent(&session.Event{Kind: session.EventKindSkillInvoked, Skill: &session.SkillPayload{Skill: "jq"}})
	pageStore := &PageStore[*sessionEventsResult]{PagesByRequestId: make(map[string]<-chan *sessionEventsResult)}
	handler := sessionEventsHandler(nil, store, pageStore, nil)

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "subagent": "ag1", "json": true, "breakdown": true}))

	assert.NoError(t, err)
	require.False(t, result.IsError)
	payload, ok := result.StructuredContent.(*sessionEventsResult)
	require.True(t, ok)
	events, ok := payload.Events.([]*session.Event)
	require.True(t, ok)
	require.Len(t, events, 1)
	assert.Equal(t, "ag1", events[0].Actor)
	require.Len(t, payload.Subagents, 1)
	assert.Equal(t, "ag1", payload.Subagents[0].AgentId)
	assert.Empty(t, payload.Skills)
	assert.Equal(t, []string{"ag1"}, payload.SubagentIds)
}

func TestSessionEvents_SubagentIdsAlwaysPresent(t *testing.T) {
	store := provideSubagentStore()
	pageStore := &PageStore[*sessionEventsResult]{PagesByRequestId: make(map[string]<-chan *sessionEventsResult)}
	handler := sessionEventsHandler(nil, store, pageStore, nil)

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "json": true}))

	assert.NoError(t, err)
	require.False(t, result.IsError)
	payload, ok := result.StructuredContent.(*sessionEventsResult)
	require.True(t, ok)
	assert.Equal(t, []string{"ag1"}, payload.SubagentIds)
}

func TestSessionEvents_JsonTypedUnpaginated(t *testing.T) {
	store := provideToolStore()
	s1, _ := store.GetById("s1")
	s1.AddEvent(&session.Event{Kind: session.EventKindSkillInvoked, Skill: &session.SkillPayload{Skill: "jq"}})
	pageStore := &PageStore[*sessionEventsResult]{PagesByRequestId: make(map[string]<-chan *sessionEventsResult)}
	handler := sessionEventsHandler(nil, store, pageStore, nil)

	result, err := handler(context.Background(), requestWithArgs(map[string]any{"id": "s1", "json": true}))

	assert.NoError(t, err)
	require.False(t, result.IsError)
	payload, ok := result.StructuredContent.(*sessionEventsResult)
	require.True(t, ok)
	events, ok := payload.Events.([]*session.Event)
	require.True(t, ok)
	require.Len(t, events, 1)
	assert.Equal(t, session.EventKindSkillInvoked, events[0].Kind)
	assert.NotNil(t, payload.Counters)
}

func TestResultBytes(t *testing.T) {
	assert.Zero(t, resultBytes(nil))
	assert.Greater(t, resultBytes(mcp.NewToolResultText("hello")), int64(0))
}

func TestAwaitReady(t *testing.T) {
	type testCase struct {
		_expectedErr    error
		_expectedResult *mcp.CallToolResult
		_id             string

		ctx   context.Context
		store *session.Store
	}

	delegated := mcp.NewToolResultText("delegated")
	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return delegated, nil
	}
	tests := make([]*testCase, 0)

	// ready-store-delegates-to-handler
	readyStore := provideToolStore()
	readyStore.MarkReady()
	tests = append(tests, &testCase{
		_id:             "ready-store-delegates-to-handler",
		_expectedResult: delegated,

		ctx:   context.Background(),
		store: readyStore,
	})

	// pending-store-times-out-with-tool-error
	tests = append(tests, &testCase{
		_id:             "pending-store-times-out-with-tool-error",
		_expectedResult: mcp.NewToolResultError(errInitialLoadPending.Error()),

		ctx:   context.Background(),
		store: provideToolStore(),
	})

	// cancelled-context-returns-context-error
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	tests = append(tests, &testCase{
		_id:          "cancelled-context-returns-context-error",
		_expectedErr: context.Canceled,

		ctx:   cancelledCtx,
		store: provideToolStore(),
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			result, err := awaitReady(test.store, 10*time.Millisecond, handler)(test.ctx, requestWithArgs(nil))

			assert.ErrorIs(t, err, test._expectedErr)
			assert.Equal(t, test._expectedResult, result)
		})
	}
}

func TestRegister_ReadOnlyHint(t *testing.T) {
	srv := server.NewMCPServer("peek-mcp", "test")
	Register(srv, provideToolStore(), NewInvocationCounter(InstanceInfo{}, nil), telemetry.NewStore(), telemetry.NewDetector(0, ""))

	registered := srv.ListTools()
	require.NotEmpty(t, registered)
	for name, tool := range registered {
		require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint, "%s missing readOnlyHint", name)
		assert.True(t, *tool.Tool.Annotations.ReadOnlyHint, "%s readOnlyHint should be true", name)
	}
}
