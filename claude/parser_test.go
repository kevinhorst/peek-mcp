package claude

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

func TestClaude_UserPrompt(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/user_prompt.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(bytes.TrimSpace(data))

	assert.NotNil(t, turn)
	assert.Equal(t, session.RoleUser, turn.Role)
	assert.Equal(t, "What does this function do?\n", turn.Text)
	assert.Equal(t, session.Id("sess-1"), turn.Meta.SessionId)
	assert.Equal(t, "/home/user/project", turn.Meta.CWD)
	assert.Equal(t, "main", turn.Meta.GitBranch)
}

func TestClaude_AssistantWithText(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/assistant_messages.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(splitLines(data)[0]) // assistant with text + usage

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

	data, err := os.ReadFile("fixtures/assistant_messages.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(splitLines(data)[1]) // assistant thinking-only

	assert.NotNil(t, turn, "meta-only turn should not be nil")
	assert.Equal(t, "", turn.Text, "thinking-only should have empty text")
	assert.Equal(t, session.Id("sess-1"), turn.Meta.SessionId)
	assert.Equal(t, "claude-opus-4-6", turn.Meta.Model)
}

func TestClaude_Skipped(t *testing.T) {
	data, err := os.ReadFile("fixtures/skipped.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	for _, line := range splitLines(data) {
		p := NewParser()
		assert.Nil(t, p.ParseLine(line))
	}
}

func TestClaude_SameRequestIdMerged(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/streaming_chunks.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	lines := splitLines(data)
	turn1 := p.ParseLine(lines[0]) // thinking chunk
	turn2 := p.ParseLine(lines[1]) // text chunk, same requestId

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

	data, err := os.ReadFile("fixtures/full_conversation.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		if turn := p.ParseLine(line); turn != nil {
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

func TestClaude_PlanModeAttachment(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/attachments.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(splitLines(data)[0]) // plan_mode attachment

	assert.NotNil(t, turn)
	assert.Equal(t, "/Users/user/.claude/plans/some-plan.md", turn.PlanFilePath)
	assert.Equal(t, session.Id("sess-plan"), turn.Meta.SessionId)
	assert.Equal(t, "/Users/user/project", turn.Meta.CWD)
}

func TestClaude_PlanFileReferenceAttachment(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/attachments.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(splitLines(data)[1]) // plan_file_reference attachment

	assert.NotNil(t, turn)
	assert.Equal(t, "/Users/user/.claude/plans/some-plan.md", turn.PlanFilePath)
	assert.Equal(t, "# My Plan\n\nDo stuff.", turn.PlanContent)
	assert.Equal(t, session.Id("sess-plan"), turn.Meta.SessionId)
}

func TestClaude_PlanModeExitAttachment(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/attachments.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(splitLines(data)[3]) // plan_mode_exit attachment

	assert.NotNil(t, turn)
	assert.Equal(t, "/Users/user/.claude/plans/some-plan.md", turn.PlanFilePath)
	assert.Equal(t, session.Id("sess-plan"), turn.Meta.SessionId)
}

func TestClaude_PlanModeReentryAttachment(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/attachments.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(splitLines(data)[4]) // plan_mode_reentry attachment

	assert.NotNil(t, turn)
	assert.Equal(t, "/Users/user/.claude/plans/some-plan.md", turn.PlanFilePath)
	assert.Equal(t, session.Id("sess-plan"), turn.Meta.SessionId)
}

func TestClaude_NonPlanAttachmentSkipped(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/attachments.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(splitLines(data)[2]) // edited_text_file attachment

	assert.Nil(t, turn)
}

func TestClaude_AITitle(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/ai_title.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(bytes.TrimSpace(data))

	assert.NotNil(t, turn)
	assert.Equal(t, "Login simplification", turn.AITitle)
	assert.Equal(t, session.Id("sess-1"), turn.Meta.SessionId)
}

func TestClaude_AITitle_Empty(t *testing.T) {
	p := NewParser()

	line := []byte(`{"type":"ai-title","sessionId":"sess-1","aiTitle":""}`)
	turn := p.ParseLine(line)

	assert.Nil(t, turn)
}

func TestClaude_InvalidJSON(t *testing.T) {
	p := NewParser()

	assert.NotPanics(t, func() {
		assert.Nil(t, p.ParseLine([]byte(`not json`)))
		assert.Nil(t, p.ParseLine([]byte(`{}`)))
		assert.Nil(t, p.ParseLine([]byte(`{"type": "user"}`)))
	})
}
