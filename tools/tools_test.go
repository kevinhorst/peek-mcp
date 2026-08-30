package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/mark3labs/mcp-go/mcp"
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
	turns, ok := payload.Turns.([]*session.Turn)
	require.True(t, ok)
	assert.Equal(t, "What does this do?", turns[0].Text)
	assert.Equal(t, s1.DiffOutput, payload.Diff)
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
