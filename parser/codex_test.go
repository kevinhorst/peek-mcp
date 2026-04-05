package parser

import (
	"testing"

	"github.com/kevinhorst/peek-mcp/models"
	"github.com/kevinhorst/peek-mcp/store"
	"github.com/stretchr/testify/assert"
)

func TestCodex_SessionMeta(t *testing.T) {
	s := store.New(20)
	p := NewCodexParser(s)

	p.ParseLine([]byte(`{
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

	sess, ok := s.Get("sess-codex-1")
	assert.True(t, ok, "session not created")
	assert.Equal(t, models.SourceCodex, sess.Meta.Source)
	assert.Equal(t, "/home/user/project", sess.Meta.CWD)
	assert.Equal(t, "abc123", sess.Meta.GitBranch)
}

func TestCodex_TurnContext(t *testing.T) {
	s := store.New(20)
	p := NewCodexParser(s)

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:38.123Z",
		"type": "turn_context",
		"payload": {
			"turn_id": "turn-1",
			"model": "gpt-5.4",
			"cwd": "/project"
		}
	}`))

	assert.Equal(t, "gpt-5.4", p.model)
}

func TestCodex_UserMessage(t *testing.T) {
	s := store.New(20)
	p := NewCodexParser(s)

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:38.234Z",
		"type": "response_item",
		"payload": {
			"type": "message",
			"role": "user",
			"content": [{"type": "input_text", "text": "Fix the bug in auth"}]
		}
	}`))

	sess, _ := s.Get("sess-codex-1")
	turns := sess.Turns.Last(10)
	assert.Len(t, turns, 1)
	assert.Equal(t, "user", turns[0].Role)
	assert.Equal(t, "Fix the bug in auth", turns[0].Text)
}

func TestCodex_AssistantMessage(t *testing.T) {
	s := store.New(20)
	p := NewCodexParser(s)

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))
	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:38.000Z",
		"type": "turn_context",
		"payload": {"turn_id": "t1", "model": "gpt-5.4"}
	}`))
	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "response_item",
		"payload": {
			"type": "message",
			"role": "assistant",
			"content": [{"type": "output_text", "text": "I fixed the auth bug by updating the token validation."}]
		}
	}`))

	sess, _ := s.Get("sess-codex-1")
	turns := sess.Turns.Last(10)
	assert.Len(t, turns, 1)
	assert.Equal(t, "assistant", turns[0].Role)
	assert.Equal(t, "gpt-5.4", turns[0].Model)
	assert.Equal(t, "I fixed the auth bug by updating the token validation.", turns[0].Text)
}

func TestCodex_FunctionCallSkipped(t *testing.T) {
	s := store.New(20)
	p := NewCodexParser(s)

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))
	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "response_item",
		"payload": {
			"type": "function_call",
			"name": "exec_command",
			"arguments": "{\"cmd\":\"pwd\"}",
			"call_id": "call_123"
		}
	}`))

	sess, _ := s.Get("sess-codex-1")
	assert.Equal(t, 0, sess.Turns.Len(), "function_call should not create a turn")
}

func TestCodex_FunctionCallOutputSkipped(t *testing.T) {
	s := store.New(20)
	p := NewCodexParser(s)

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))
	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "response_item",
		"payload": {
			"type": "function_call_output",
			"call_id": "call_123",
			"output": "/home/user/project"
		}
	}`))

	sess, _ := s.Get("sess-codex-1")
	assert.Equal(t, 0, sess.Turns.Len(), "function_call_output should not create a turn")
}

func TestCodex_ReasoningSkipped(t *testing.T) {
	s := store.New(20)
	p := NewCodexParser(s)

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))
	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "response_item",
		"payload": {
			"type": "reasoning",
			"summary": [],
			"content": null
		}
	}`))

	sess, _ := s.Get("sess-codex-1")
	assert.Equal(t, 0, sess.Turns.Len(), "reasoning should not create a turn")
}

func TestCodex_NoSessionMetaSkipped(t *testing.T) {
	s := store.New(20)
	p := NewCodexParser(s)

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "response_item",
		"payload": {
			"type": "message",
			"role": "user",
			"content": [{"type": "input_text", "text": "hello"}]
		}
	}`))

	assert.Empty(t, s.List(), "response_item without session_meta should not create a session")
}

func TestCodex_EventMsgSkipped(t *testing.T) {
	s := store.New(20)
	p := NewCodexParser(s)

	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:45:22.019Z",
		"type": "session_meta",
		"payload": {"id": "sess-codex-1", "cwd": "/project"}
	}`))
	p.ParseLine([]byte(`{
		"timestamp": "2026-03-29T23:47:40.000Z",
		"type": "event_msg",
		"payload": {"type": "token_count", "info": null}
	}`))

	sess, _ := s.Get("sess-codex-1")
	assert.Equal(t, 0, sess.Turns.Len(), "event_msg should not create a turn")
}

func TestCodex_InvalidJSON(t *testing.T) {
	s := store.New(20)
	p := NewCodexParser(s)

	assert.NotPanics(t, func() {
		p.ParseLine([]byte(`not json`))
		p.ParseLine([]byte(`{}`))
		p.ParseLine([]byte(`{"type": "session_meta", "payload": "bad"}`))
	})
}
