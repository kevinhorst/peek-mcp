package codex

import (
	"bytes"
	"os"
	"strings"
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
	assert.Equal(t, "develop", turn.Meta.GitBranch)

	assert.NotNil(t, turn.Meta.Origin)
	assert.Equal(t, "1.0.0", turn.Meta.Origin.CliVersion)
	assert.Equal(t, "abc123", turn.Meta.Origin.CommitHash)
	assert.Equal(t, "sess-codex-0", turn.Meta.Origin.ForkedFromId)
	assert.Equal(t, "Codex Desktop", turn.Meta.Origin.Originator)
	assert.Equal(t, "https://github.com/user/repo", turn.Meta.Origin.RepositoryUrl)
	assert.Equal(t, "vscode", turn.Meta.Origin.SourceKind)
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

func TestCodex_SubagentDropped(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/subagent_meta.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn := p.ParseLine(bytes.TrimSpace(data))

	assert.Nil(t, turn, "sub-agent session_meta is dropped")

	userData, err := os.ReadFile("fixtures/user_message.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	turn = p.ParseLine(bytes.TrimSpace(userData))

	assert.Nil(t, turn, "lines after a dropped session_meta are ignored")
}

func TestCodex_ProposedPlan(t *testing.T) {
	p := NewParser()

	data, err := os.ReadFile("fixtures/proposed_plan.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	var turns []*session.Turn
	for _, line := range splitLines(data) {
		if turn := p.ParseLine(line); turn != nil {
			turns = append(turns, turn)
		}
	}

	assert.Len(t, turns, 3, "session_meta + two assistant messages")

	// assistant message without a plan block carries no plan fields
	assert.Equal(t, "", turns[1].PlanFilePath)
	assert.Equal(t, "", turns[1].PlanContent)

	// last block wins; the message text stays a full chat turn
	planTurn := turns[2]
	assert.Equal(t, PlanFilePathProposedPlan, planTurn.PlanFilePath)
	assert.Equal(t, "# Harden recursive script cleanup v2\n- validate targets before deletion\n- keep rmdir for the temporary worktree", planTurn.PlanContent)
	assert.Equal(t, session.RoleAssistant, planTurn.Role)
	assert.Contains(t, planTurn.Text, "<proposed_plan>")
	assert.Contains(t, planTurn.Text, "Implemented.")
}

func TestExtractProposedPlan(t *testing.T) {
	type testCase struct {
		_expected string
		_id       string

		text string
	}

	tests := make([]*testCase, 0)

	// no-block
	test := &testCase{
		_id:       "no-block",
		_expected: "",
		text:      "just prose, no plan",
	}
	tests = append(tests, test)

	// unclosed-block
	test = &testCase{
		_id:       "unclosed-block",
		_expected: "",
		text:      "<proposed_plan>\n# Plan\n- step",
	}
	tests = append(tests, test)

	// single-block
	test = &testCase{
		_id:       "single-block",
		_expected: "# Plan\n- step",
		text:      "intro\n<proposed_plan>\n# Plan\n- step\n</proposed_plan>\noutro",
	}
	tests = append(tests, test)

	// two-blocks-last-wins
	test = &testCase{
		_id:       "two-blocks-last-wins",
		_expected: "# Plan v2",
		text:      "<proposed_plan>\n# Plan v1\n</proposed_plan>\nprose\n<proposed_plan>\n# Plan v2\n</proposed_plan>",
	}
	tests = append(tests, test)

	// oversized-truncated
	test = &testCase{
		_id:       "oversized-truncated",
		_expected: strings.Repeat("a", maxPlanBytes),
		text:      "<proposed_plan>\n" + strings.Repeat("a", maxPlanBytes+100) + "\n</proposed_plan>",
	}
	tests = append(tests, test)

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			assert.Equal(t, test._expected, extractProposedPlan(test.text))
		})
	}
}

func TestCodex_InvalidJSON(t *testing.T) {
	p := NewParser()

	assert.NotPanics(t, func() {
		assert.Nil(t, p.ParseLine([]byte(`not json`)))
		assert.Nil(t, p.ParseLine([]byte(`{}`)))
		assert.Nil(t, p.ParseLine([]byte(`{"type": "session_meta", "payload": "bad"}`)))
	})
}
