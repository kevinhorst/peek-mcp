package claude

import (
	"bytes"
	"os"
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, 200, turn.Usage.CacheCreationInputTokens)
	assert.Equal(t, 50, turn.Usage.CacheCreation5mInputTokens)
	assert.Equal(t, 150, turn.Usage.CacheCreation1hInputTokens)
	assert.Equal(t, "end_turn", turn.StopReason)
}

func TestClaude_AssistantUsageWithoutBreakdown(t *testing.T) {
	p := NewParser()

	line := `{"type":"assistant","requestId":"req-9","sessionId":"sess-1","timestamp":"2026-04-05T15:30:18.628Z","cwd":"/home/user/project","message":{"role":"assistant","model":"claude-opus-4-6","content":[{"type":"text","text":"legacy"}],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":200,"cache_read_input_tokens":300}}}`
	turn := p.ParseLine([]byte(line))

	assert.NotNil(t, turn)
	assert.NotNil(t, turn.Usage)
	assert.Equal(t, 200, turn.Usage.CacheCreationInputTokens)
	assert.Equal(t, 0, turn.Usage.CacheCreation5mInputTokens)
	assert.Equal(t, 0, turn.Usage.CacheCreation1hInputTokens)
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
	assert.Equal(t, "Let me analyze this...\n", turn.Thinking)
	assert.Equal(t, session.Id("sess-1"), turn.Meta.SessionId)
	assert.Equal(t, "claude-opus-4-6", turn.Meta.Model)
	assert.Equal(t, "", turn.StopReason)
}

func TestClaude_AssistantThinkingAndText(t *testing.T) {
	p := NewParser()
	line := []byte(`{"type":"assistant","requestId":"req-2","sessionId":"sess-1","timestamp":"2026-04-05T15:30:18.628Z","isSidechain":false,"message":{"role":"assistant","model":"claude-opus-4-6","content":[{"type":"thinking","thinking":"Checking the code...","signature":"sig"},{"type":"text","text":"Done."}]}}`)
	turn := p.ParseLine(line)

	assert.NotNil(t, turn)
	assert.Equal(t, "Done.\n", turn.Text)
	assert.Equal(t, "Checking the code...\n", turn.Thinking)
}

func TestClaude_SidechainTranscript(t *testing.T) {
	p := NewParser()

	userLine := []byte(`{"type":"user","sessionId":"sess-1","agentId":"ag1","timestamp":"2026-04-05T15:30:18.000Z","isSidechain":true,"message":{"role":"user","content":[{"type":"text","text":"Explore the repo."}]}}`)
	turn := p.ParseLine(userLine)
	assert.NotNil(t, turn)
	assert.Equal(t, session.RoleUser, turn.Role)
	assert.Equal(t, "Explore the repo.\n", turn.Text)
	assert.Equal(t, "ag1", turn.SubagentId)

	assistantLine := []byte(`{"type":"assistant","requestId":"req-3","sessionId":"sess-1","agentId":"ag1","timestamp":"2026-04-05T15:30:19.000Z","isSidechain":true,"message":{"role":"assistant","model":"claude-opus-4-6","content":[{"type":"thinking","thinking":"Reading files..."},{"type":"text","text":"Found it."}]}}`)
	turn = p.ParseLine(assistantLine)
	assert.NotNil(t, turn)
	assert.Equal(t, session.RoleAssistant, turn.Role)
	assert.Equal(t, "Found it.\n", turn.Text)
	assert.Equal(t, "Reading files...\n", turn.Thinking)
	assert.Equal(t, "ag1", turn.SubagentId)
	assert.Equal(t, "claude-opus-4-6", turn.Meta.Model)
}

func TestClaude_SyntheticModelDropped(t *testing.T) {
	p := NewParser()
	line := []byte(`{"type":"assistant","requestId":"req-9","sessionId":"sess-1","timestamp":"2026-04-05T15:30:18.628Z","isSidechain":false,"message":{"role":"assistant","model":"<synthetic>","content":[{"type":"text","text":"error placeholder"}]}}`)
	turn := p.ParseLine(line)

	assert.NotNil(t, turn)
	assert.Equal(t, "", turn.Meta.Model)
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
	assert.Equal(t, "analyzing...\n", turn1.Thinking)
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

	// user, assistant (text), assistant (tool_use), tool_result signal, user = 5 non-nil turns
	require.Len(t, turns, 5)

	assert.Equal(t, session.RoleUser, turns[0].Role)
	assert.Equal(t, "Explain this code", turns[0].Text)

	assert.Equal(t, session.RoleAssistant, turns[1].Role)
	assert.Equal(t, "This code does X.\n", turns[1].Text)
	assert.Equal(t, "claude-sonnet-4-20250514", turns[1].Meta.Model)

	assert.Equal(t, session.RoleAssistant, turns[2].Role)
	assert.Equal(t, "", turns[2].Text, "tool_use-only is meta-only")
	require.Len(t, turns[2].ToolCalls, 1)
	assert.Equal(t, "Read", turns[2].ToolCalls[0].Name)

	assert.Equal(t, session.Role(""), turns[3].Role)
	require.Len(t, turns[3].ToolResults, 1)
	assert.Equal(t, "toolu_1", turns[3].ToolResults[0].ToolUseId)

	assert.Equal(t, session.RoleUser, turns[4].Role)
	assert.Equal(t, "Now fix the bug", turns[4].Text)
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

func TestClaude_CustomTitle(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/custom_title.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(bytes.TrimSpace(data))

	assert.NotNil(t, turn)
	assert.Equal(t, "Login simplification", turn.CustomTitle)
	assert.Equal(t, session.Id("sess-1"), turn.Meta.SessionId)
}

func TestClaude_CustomTitle_Empty(t *testing.T) {
	p := NewParser()

	line := []byte(`{"type":"custom-title","sessionId":"sess-1","customTitle":""}`)
	turn := p.ParseLine(line)

	assert.Nil(t, turn)
}

func TestClaude_VersionOrigin(t *testing.T) {
	p := NewParser()

	// entry without version → no Origin
	data, err := os.ReadFile("fixtures/user_prompt.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(data)

	assert.NotNil(t, turn)
	assert.Nil(t, turn.Meta.Origin)

	// entry with version → Origin.CliVersion
	line := []byte(`{"type":"user","promptId":"abc-124","sessionId":"sess-1","timestamp":"2026-04-05T15:31:00.000Z","version":"2.1.0","isSidechain":false,"message":{"role":"user","content":"hello"}}`)
	turn = p.ParseLine(line)

	assert.NotNil(t, turn)
	assert.NotNil(t, turn.Meta.Origin)
	assert.Equal(t, "2.1.0", turn.Meta.Origin.CliVersion)
}

func TestClaude_InvalidJSON(t *testing.T) {
	p := NewParser()

	assert.NotPanics(t, func() {
		assert.Nil(t, p.ParseLine([]byte(`not json`)))
		assert.Nil(t, p.ParseLine([]byte(`{}`)))
		assert.Nil(t, p.ParseLine([]byte(`{"type": "user"}`)))
	})
}

func TestParser_StateRoundTrip(t *testing.T) {
	type testCase struct {
		_id string

		lines    [][]byte
		original *Parser
		restored *Parser
	}

	toolUseLine := []byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-bash","name":"Bash","input":{"command":"rm -rf /tmp/x"}}]}}`)
	denialLine := []byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"toolDenialKind":"user-rejected","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-bash","is_error":true,"content":"The user doesn't want to proceed with this tool use."}]}}`)

	tests := make([]*testCase, 0)

	// pending-tool-survives
	original := NewParser()
	original.ParseLine(toolUseLine)
	tests = append(tests, &testCase{
		_id:      "pending-tool-survives",
		lines:    [][]byte{denialLine},
		original: original,
		restored: NewParser(),
	})

	// permission-mode-survives
	original = NewParser()
	original.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"permissionMode":"acceptEdits","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"go"}]}}`))
	tests = append(tests, &testCase{
		_id:      "permission-mode-survives",
		lines:    [][]byte{[]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:01:00.000Z","isSidechain":false,"permissionMode":"acceptEdits","promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"next"}]}}`)},
		original: original,
		restored: NewParser(),
	})

	// empty-state
	tests = append(tests, &testCase{
		_id:      "empty-state",
		lines:    [][]byte{toolUseLine, denialLine},
		original: NewParser(),
		restored: NewParser(),
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			state, err := test.original.State()
			require.NoError(t, err)
			require.NoError(t, test.restored.Restore(state))

			for _, line := range test.lines {
				expected := test.original.ParseLine(line)
				require.NotNil(t, expected)
				assert.Equal(t, expected, test.restored.ParseLine(line))
			}
		})
	}
}
