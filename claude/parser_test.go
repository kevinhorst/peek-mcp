package claude

import (
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
)

func TestClaude_UserPrompt(t *testing.T) {
	p := NewParser()

	turn := p.ParseLine([]byte(`{
		"type": "user",
		"promptId": "abc-123",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:14.588Z",
		"cwd": "/home/user/project",
		"gitBranch": "main",
		"isSidechain": false,
	  "message": {
		"role": "user",
		"content": [
		  {
			"tool_use_id": "toolu_01XfZ7iUmYn6oXKSrbnGGQbR",
			"type": "text",
			"text": "What does this function do?",
			"is_error": false
		  }
		]
  }
	}`))

	assert.NotNil(t, turn)
	assert.Equal(t, session.RoleUser, turn.Role)
	assert.Equal(t, "What does this function do?\n", turn.Text)
	assert.Equal(t, session.Id("sess-1"), turn.Meta.SessionId)
	assert.Equal(t, "/home/user/project", turn.Meta.CWD)
	assert.Equal(t, "main", turn.Meta.GitBranch)
}

func TestClaude_ToolResultSkipped(t *testing.T) {
	p := NewParser()

	turn := p.ParseLine([]byte(`{
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

	assert.Nil(t, turn)
}

func TestClaude_AssistantWithText(t *testing.T) {
	p := NewParser()

	turn := p.ParseLine([]byte(`{
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

	assert.NotNil(t, turn)
	assert.Equal(t, session.RoleAssistant, turn.Role)
	assert.Equal(t, "This function calculates the sum.\n", turn.Text)
	assert.Equal(t, "claude-opus-4-6", turn.Meta.Model)
	assert.Equal(t, session.Id("sess-1"), turn.Meta.SessionId)
	assert.NotNil(t, turn.Usage)
	assert.Equal(t, 100, turn.Usage.InputTokens)
	assert.Equal(t, 50, turn.Usage.OutputTokens)
}

func TestClaude_AssistantThinkingOnly(t *testing.T) {
	p := NewParser()

	turn := p.ParseLine([]byte(`{
		"type": "assistant",
		"requestId": "req-1",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:18.628Z",
		"cwd": "/home/user/project",
		"isSidechain": false,
		"message": {
			"role": "assistant",
			"model": "claude-opus-4-6",
			"content": [
				{"type": "thinking", "thinking": "Let me analyze this..."}
			]
		}
	}`))

	assert.NotNil(t, turn, "meta-only turn should not be nil")
	assert.Equal(t, "", turn.Text, "thinking-only should have empty text")
	assert.Equal(t, session.Id("sess-1"), turn.Meta.SessionId)
	assert.Equal(t, "claude-opus-4-6", turn.Meta.Model)
}

func TestClaude_SidechainSkipped(t *testing.T) {
	p := NewParser()

	turn := p.ParseLine([]byte(`{
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

	assert.Nil(t, turn)
}

func TestClaude_QueueOperationSkipped(t *testing.T) {
	p := NewParser()

	turn := p.ParseLine([]byte(`{
		"type": "queue-operation",
		"operation": "enqueue",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:14.575Z"
	}`))

	assert.Nil(t, turn)
}

func TestClaude_NoPromptIdSkipped(t *testing.T) {
	p := NewParser()

	turn := p.ParseLine([]byte(`{
		"type": "user",
		"sessionId": "sess-1",
		"timestamp": "2026-04-05T15:30:14.588Z",
		"isSidechain": false,
		"message": {
			"role": "user",
			"content": "no promptId"
		}
	}`))

	assert.Nil(t, turn)
}

func TestClaude_SameRequestIdMerged(t *testing.T) {
	p := NewParser()

	// First chunk: thinking only (no text)
	turn1 := p.ParseLine([]byte(`{
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
	turn2 := p.ParseLine([]byte(`{
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

	assert.NotNil(t, turn1, "thinking-only returns meta-only turn")
	assert.Equal(t, "", turn1.Text)
	assert.Equal(t, "req-1", turn1.RequestId)

	assert.NotNil(t, turn2)
	assert.Equal(t, "Here is the answer.\n", turn2.Text)
	assert.Equal(t, "req-1", turn2.RequestId)
}

func TestClaude_FullConversation(t *testing.T) {
	p := NewParser()

	var turns []*session.Turn

	lines := []string{
		// User prompt
		`{"type":"user","promptId":"p1","sessionId":"sess-1","timestamp":"2026-04-05T15:00:00.000Z","cwd":"/project","gitBranch":"feature","isSidechain":false,"message":{"role":"user","content":"Explain this code"}}`,
		// Assistant response
		`{"type":"assistant","requestId":"req-1","sessionId":"sess-1","timestamp":"2026-04-05T15:00:05.000Z","cwd":"/project","gitBranch":"feature","isSidechain":false,"message":{"role":"assistant","model":"claude-sonnet-4-20250514","content":[{"type":"text","text":"This code does X."}],"usage":{"input_tokens":50,"output_tokens":20}}}`,
		// Tool use (assistant) — meta-only (no text)
		`{"type":"assistant","requestId":"req-2","sessionId":"sess-1","timestamp":"2026-04-05T15:00:06.000Z","isSidechain":false,"message":{"role":"assistant","model":"claude-sonnet-4-20250514","content":[{"type":"tool_use","id":"toolu_1","name":"Read","input":{}}]}}`,
		// Tool result (user) — skipped (content is array, not string)
		`{"type":"user","promptId":"p1","sessionId":"sess-1","timestamp":"2026-04-05T15:00:07.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"file data"}]}}`,
		// Second user prompt
		`{"type":"user","promptId":"p2","sessionId":"sess-1","timestamp":"2026-04-05T15:01:00.000Z","cwd":"/project","gitBranch":"feature","isSidechain":false,"message":{"role":"user","content":"Now fix the bug"}}`,
	}

	for _, line := range lines {
		turn := p.ParseLine([]byte(line))
		if turn != nil {
			turns = append(turns, turn)
		}
	}

	// user, assistant (text), assistant (meta-only tool_use), user = 4 non-nil turns
	// tool_result is nil (content is array, unmarshal to string fails)
	assert.Len(t, turns, 4)

	assert.Equal(t, session.RoleUser, turns[0].Role)
	assert.Equal(t, "Explain this code", turns[0].Text)

	assert.Equal(t, session.RoleAssistant, turns[1].Role)
	assert.Equal(t, "This code does X.\n", turns[1].Text)
	assert.Equal(t, "claude-sonnet-4-20250514", turns[1].Meta.Model)

	assert.Equal(t, session.RoleAssistant, turns[2].Role)
	assert.Equal(t, "", turns[2].Text, "tool_use-only is meta-only")

	assert.Equal(t, session.RoleUser, turns[3].Role)
	assert.Equal(t, "Now fix the bug", turns[3].Text)
}

func TestClaude_InvalidJSON(t *testing.T) {
	p := NewParser()

	assert.NotPanics(t, func() {
		assert.Nil(t, p.ParseLine([]byte(`not json`)))
		assert.Nil(t, p.ParseLine([]byte(`{}`)))
		assert.Nil(t, p.ParseLine([]byte(`{"type": "user"}`)))
	})
}
