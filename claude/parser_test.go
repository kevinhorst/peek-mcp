package claude

import (
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
)

func TestClaude_UserPrompt(t *testing.T) {
	s := session.New(20)
	p := NewParser(s)

	p.ParseLine([]byte(`{
		"type": "user",
		"promptId": "abc-123",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:14.588Z",
		"cwd": "/home/user/project",
		"gitBranch": "main",
		"isSidechain": false,
		"message": {
			"role": "user",
			"content": "What does this function do?"
		}
	}`))
	p.Flush()

	sess, ok := s.Get("sess-1")
	assert.True(t, ok, "session not created")

	turns := sess.Turns.Last(10)
	assert.Len(t, turns, 1)
	assert.Equal(t, "user", turns[0].Role)
	assert.Equal(t, "What does this function do?", turns[0].Text)
	assert.Equal(t, "/home/user/project", sess.Info.CWD)
	assert.Equal(t, "main", sess.Info.GitBranch)
}

func TestClaude_ToolResultSkipped(t *testing.T) {
	s := session.New(20)
	p := NewParser(s)

	p.ParseLine([]byte(`{
		"type": "user",
		"promptId": "abc-123",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:14.588Z",
		"isSidechain": false,
		"message": {
			"role": "user",
			"content": [{"type": "tool_result", "tool_use_id": "toolu_123", "content": "file contents"}]
		}
	}`))
	p.Flush()

	sess, ok := s.Get("sess-1")
	if ok {
		assert.Equal(t, 0, sess.Turns.Len(), "tool result should not create a turn")
	}
}

func TestClaude_AssistantWithText(t *testing.T) {
	s := session.New(20)
	p := NewParser(s)

	p.ParseLine([]byte(`{
		"type": "assistant",
		"requestId": "req-1",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:18.628Z",
		"cwd": "/home/user/project",
		"gitBranch": "main",
		"isSidechain": false,
		"message": {
			"role": "assistant",
			"model": "claude-opus-4-6",
			"content": [
				{"type": "text", "text": "This function calculates the sum."}
			],
			"usage": {
				"input_tokens": 100,
				"output_tokens": 50,
				"cache_creation_input_tokens": 200,
				"cache_read_input_tokens": 300
			}
		}
	}`))
	p.Flush()

	sess, ok := s.Get("sess-1")
	assert.True(t, ok, "session not created")

	turns := sess.Turns.Last(10)
	assert.Len(t, turns, 1)
	assert.Equal(t, "assistant", turns[0].Role)
	assert.Equal(t, "This function calculates the sum.", turns[0].Text)
	assert.Equal(t, "claude-opus-4-6", turns[0].Model)
	assert.NotNil(t, turns[0].Usage)
	assert.Equal(t, 100, turns[0].Usage.InputTokens)
	assert.Equal(t, 50, turns[0].Usage.OutputTokens)
}

func TestClaude_AssistantThinkingOnlySkipped(t *testing.T) {
	s := session.New(20)
	p := NewParser(s)

	p.ParseLine([]byte(`{
		"type": "assistant",
		"requestId": "req-1",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:18.628Z",
		"isSidechain": false,
		"message": {
			"role": "assistant",
			"model": "claude-opus-4-6",
			"content": [
				{"type": "thinking", "thinking": "Let me analyze this..."}
			]
		}
	}`))
	p.Flush()

	sess, ok := s.Get("sess-1")
	if ok {
		assert.Equal(t, 0, sess.Turns.Len(), "thinking-only response should not create a turn")
	}
}

func TestClaude_SidechainSkipped(t *testing.T) {
	s := session.New(20)
	p := NewParser(s)

	p.ParseLine([]byte(`{
		"type": "user",
		"promptId": "abc-123",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:14.588Z",
		"isSidechain": true,
		"message": {
			"role": "user",
			"content": "sidechain message"
		}
	}`))
	p.Flush()

	_, ok := s.Get("sess-1")
	assert.False(t, ok, "sidechain entry should not create a session")
}

func TestClaude_QueueOperationSkipped(t *testing.T) {
	s := session.New(20)
	p := NewParser(s)

	p.ParseLine([]byte(`{
		"type": "queue-operation",
		"operation": "enqueue",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:14.575Z"
	}`))
	p.Flush()

	_, ok := s.Get("sess-1")
	assert.False(t, ok, "queue-operation should not create a session")
}

func TestClaude_NoPromptIDSkipped(t *testing.T) {
	s := session.New(20)
	p := NewParser(s)

	p.ParseLine([]byte(`{
		"type": "user",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:14.588Z",
		"isSidechain": false,
		"message": {
			"role": "user",
			"content": "no promptId"
		}
	}`))
	p.Flush()

	sess, ok := s.Get("sess-1")
	if ok {
		assert.Equal(t, 0, sess.Turns.Len(), "entry without promptId should not create a turn")
	}
}

func TestClaude_SameRequestIDMerged(t *testing.T) {
	s := session.New(20)
	p := NewParser(s)

	// First chunk: thinking only (no text)
	p.ParseLine([]byte(`{
		"type": "assistant",
		"requestId": "req-1",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:18.000Z",
		"isSidechain": false,
		"message": {
			"role": "assistant",
			"model": "claude-opus-4-6",
			"content": [
				{"type": "thinking", "thinking": "analyzing..."}
			]
		}
	}`))

	// Second chunk: text content, same requestId
	p.ParseLine([]byte(`{
		"type": "assistant",
		"requestId": "req-1",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:19.000Z",
		"isSidechain": false,
		"message": {
			"role": "assistant",
			"model": "claude-opus-4-6",
			"content": [
				{"type": "text", "text": "Here is the answer."}
			],
			"usage": {
				"input_tokens": 100,
				"output_tokens": 25
			}
		}
	}`))
	p.Flush()

	sess, ok := s.Get("sess-1")
	assert.True(t, ok, "session not created")

	turns := sess.Turns.Last(10)
	assert.Len(t, turns, 1, "expected 1 merged turn")
	assert.Equal(t, "Here is the answer.", turns[0].Text)
}

func TestClaude_FullConversation(t *testing.T) {
	s := session.New(20)
	p := NewParser(s)

	// User prompt
	p.ParseLine([]byte(`{
		"type": "user",
		"promptId": "p1",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:00:00.000Z",
		"cwd": "/project",
		"gitBranch": "feature",
		"isSidechain": false,
		"message": {"role": "user", "content": "Explain this code"}
	}`))

	// Assistant response
	p.ParseLine([]byte(`{
		"type": "assistant",
		"requestId": "req-1",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:00:05.000Z",
		"cwd": "/project",
		"gitBranch": "feature",
		"isSidechain": false,
		"message": {
			"role": "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": [{"type": "text", "text": "This code does X."}],
			"usage": {"input_tokens": 50, "output_tokens": 20}
		}
	}`))

	// Tool use (assistant) — should be skipped (no text)
	p.ParseLine([]byte(`{
		"type": "assistant",
		"requestId": "req-2",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:00:06.000Z",
		"isSidechain": false,
		"message": {
			"role": "assistant",
			"model": "claude-sonnet-4-20250514",
			"content": [{"type": "tool_use", "id": "toolu_1", "name": "Read", "input": {}}]
		}
	}`))

	// Tool result (user) — should be skipped
	p.ParseLine([]byte(`{
		"type": "user",
		"promptId": "p1",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:00:07.000Z",
		"isSidechain": false,
		"message": {
			"role": "user",
			"content": [{"type": "tool_result", "tool_use_id": "toolu_1", "content": "file data"}]
		}
	}`))

	// Second user prompt
	p.ParseLine([]byte(`{
		"type": "user",
		"promptId": "p2",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:01:00.000Z",
		"cwd": "/project",
		"gitBranch": "feature",
		"isSidechain": false,
		"message": {"role": "user", "content": "Now fix the bug"}
	}`))

	p.Flush()

	sess, ok := s.Get("sess-1")
	assert.True(t, ok, "session not created")

	turns := sess.Turns.Last(10)
	assert.Len(t, turns, 3, "expected 3 turns (user, assistant, user)")
	assert.Equal(t, "user", turns[0].Role)
	assert.Equal(t, "Explain this code", turns[0].Text)
	assert.Equal(t, "assistant", turns[1].Role)
	assert.Equal(t, "This code does X.", turns[1].Text)
	assert.Equal(t, "user", turns[2].Role)
	assert.Equal(t, "Now fix the bug", turns[2].Text)

	assert.Equal(t, "claude-sonnet-4-20250514", sess.Info.Model)
	assert.Equal(t, 50, sess.Info.TotalUsage.InputTokens)
}

func TestClaude_InvalidJSON(t *testing.T) {
	s := session.New(20)
	p := NewParser(s)

	// Should not panic
	assert.NotPanics(t, func() {
		p.ParseLine([]byte(`not json`))
		p.ParseLine([]byte(`{}`))
		p.ParseLine([]byte(`{"type": "user"}`))
		p.Flush()
	})
}
