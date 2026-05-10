package codex

import (
	"bytes"
	"os"
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
)

func TestCodex_SessionMeta(t *testing.T) {
	p := NewParser()

	turn := p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {
			"id": "sess-codex-1",
			"cwd": "/home/user/project",
			"cli_version": "1.0.0",
			"git": {
				"commit_hash": "abc123",
				"repository_url": "https://github.com/user/repo"
			}
		}
	}`))

	assert.NotNil(t, turn)
	assert.Equal(t, "", turn.Text, "session_meta is meta-only")
	assert.Equal(t, session.Id("sess-codex-1"), turn.Meta.SessionId)
	assert.Equal(t, "/home/user/project", turn.Meta.CWD)
	assert.Equal(t, "abc123", turn.Meta.GitBranch)
}

func TestCodex_TurnContext(t *testing.T) {
	p := NewParser()

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))

	turn := p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:38.123Z",
		"type": "turn_context",
		"payload": {
			"turn_id": "turn-1",
			"model": "gpt-5.4",
			"cwd": "/project"
		}
	}`))

	assert.Nil(t, turn, "turn_context returns nil")
	assert.Equal(t, "gpt-5.4", p.model)
}

func TestCodex_UserMessage(t *testing.T) {
	p := NewParser()

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))

	turn := p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:38.234Z",
		"type": "response_item",
		"payload": {
			"type": "message",
			"role": "user",
			"content": [{"type": "input_text", "text": "Fix the bug in auth"}]
		}
	}`))

	assert.NotNil(t, turn)
	assert.Equal(t, session.RoleUser, turn.Role)
	assert.Equal(t, "Fix the bug in auth", turn.Text)
	assert.Equal(t, session.Id("sess-codex-1"), turn.Meta.SessionId)
}

func TestCodex_AssistantMessage(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/assistant_turn.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	var turn *session.Turn
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		turn = p.ParseLine(line)
	}

	assert.NotNil(t, turn)
	assert.Equal(t, session.RoleAssistant, turn.Role)
	assert.Equal(t, "gpt-5.4", turn.Meta.Model)
	assert.Equal(t, "I fixed the auth bug by updating the token validation.", turn.Text)
}

func TestCodex_FunctionCallSkipped(t *testing.T) {
	p := NewParser()

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))
	turn := p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "response_item",
		"payload": {
			"type": "function_call",
			"name": "exec_command",
			"arguments": "{\"cmd\":\"pwd\"}",
			"call_id": "call_123"
		}
	}`))

	assert.Nil(t, turn)
}

func TestCodex_FunctionCallOutputSkipped(t *testing.T) {
	p := NewParser()

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))
	turn := p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "response_item",
		"payload": {
			"type": "function_call_output",
			"call_id": "call_123",
			"output": "/home/user/project"
		}
	}`))

	assert.Nil(t, turn)
}

func TestCodex_ReasoningSkipped(t *testing.T) {
	p := NewParser()

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))
	turn := p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "response_item",
		"payload": {
			"type": "reasoning",
			"summary": [],
			"content": null
		}
	}`))

	assert.Nil(t, turn)
}

func TestCodex_NoSessionMetaSkipped(t *testing.T) {
	p := NewParser()

	turn := p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "response_item",
		"payload": {
			"type": "message",
			"role": "user",
			"content": [{"type": "input_text", "text": "hello"}]
		}
	}`))

	assert.Nil(t, turn)
}

func TestCodex_EventMsgSkipped(t *testing.T) {
	p := NewParser()

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))
	turn := p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "event_msg",
		"payload": {"type": "token_count", "info": null}
	}`))

	assert.Nil(t, turn, "event_msg without usage info returns nil")
}

func TestCodex_TokenCountEvent(t *testing.T) {
	p := NewParser()

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))
	turn := p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "event_msg",
		"payload": {
			"type": "token_count",
			"info": {
				"total_token_usage": {
					"input_tokens": 100,
					"cached_input_tokens": 60,
					"output_tokens": 20,
					"reasoning_output_tokens": 5,
					"total_tokens": 125
				}
			}
		}
	}`))

	assert.NotNil(t, turn)
	assert.Equal(t, "", turn.Text, "token_count is meta-only")
	assert.Equal(t, session.Id("sess-codex-1"), turn.Meta.SessionId)
	assert.NotNil(t, turn.Usage)
	assert.Equal(t, 100, turn.Usage.InputTokens)
	assert.Equal(t, 60, turn.Usage.CachedInputTokens)
	assert.Equal(t, 20, turn.Usage.OutputTokens)
	assert.Equal(t, 5, turn.Usage.ReasoningOutputTokens)
	assert.Equal(t, 125, turn.Usage.TotalTokens)
}

func TestCodex_InvalidJSON(t *testing.T) {
	p := NewParser()

	assert.NotPanics(t, func() {
		assert.Nil(t, p.ParseLine([]byte(`not json`)))
		assert.Nil(t, p.ParseLine([]byte(`{}`)))
		assert.Nil(t, p.ParseLine([]byte(`{"type": "session_meta", "payload": "bad"}`)))
	})
}
