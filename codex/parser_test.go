package codex

import (
	"bytes"
	"os"
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
)

func splitLines(data []byte) [][]byte {
	var out [][]byte
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 {
			out = append(out, line)
		}
	}
	return out
}

func seededParser(t *testing.T) *Parser {
	t.Helper()
	p := NewParser()
	p.ParseLine([]byte(`{"timestamp":"2026-03-29T23:45:22.019Z","type":"session_meta","payload":{"id":"sess-codex-1","cwd":"/project"}}`))
	return p
}

func TestCodex_SessionMeta(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/session_meta_full.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(bytes.TrimSpace(data))

	assert.NotNil(t, turn)
	assert.Equal(t, "", turn.Text, "session_meta is meta-only")
	assert.Equal(t, session.Id("sess-codex-1"), turn.Meta.SessionId)
	assert.Equal(t, "/home/user/project", turn.Meta.CWD)
	assert.Equal(t, "abc123", turn.Meta.GitBranch)
}

func TestCodex_TurnContext(t *testing.T) {
	p := seededParser(t)

	data, err := os.ReadFile("fixtures/turn_context.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(bytes.TrimSpace(data))

	assert.Nil(t, turn, "turn_context returns nil")
	assert.Equal(t, "gpt-5.4", p.model)
}

func TestCodex_UserMessage(t *testing.T) {
	p := seededParser(t)

	data, err := os.ReadFile("fixtures/user_message.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(bytes.TrimSpace(data))

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
	for _, line := range splitLines(data) {
		turn = p.ParseLine(line)
	}

	assert.NotNil(t, turn)
	assert.Equal(t, session.RoleAssistant, turn.Role)
	assert.Equal(t, "gpt-5.4", turn.Meta.Model)
	assert.Equal(t, "I fixed the auth bug by updating the token validation.", turn.Text)
}

func TestCodex_Skipped(t *testing.T) {
	data, err := os.ReadFile("fixtures/skipped_events.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	for _, line := range splitLines(data) {
		p := seededParser(t)
		assert.Nil(t, p.ParseLine(line))
	}
}

func TestCodex_NoSessionMetaSkipped(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/no_session_message.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(bytes.TrimSpace(data))

	assert.Nil(t, turn)
}

func TestCodex_TokenCountEvent(t *testing.T) {
	p := seededParser(t)

	data, err := os.ReadFile("fixtures/token_count_event.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(bytes.TrimSpace(data))

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
