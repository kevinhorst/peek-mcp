package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsonString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func TestParseLine_SkillEvents(t *testing.T) {
	// skill-tool-use
	p := NewParser()
	line := []byte(`{"type":"assistant","sessionId":"s","agentId":"ag1","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Skill","input":{"skill":"feature-design","args":"raw"}}]}}`)
	turn := p.ParseLine(line)
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindSkillInvoked, turn.Events[0].Kind)
	assert.Equal(t, "feature-design", turn.Events[0].Skill.Skill)
	assert.Equal(t, "raw", turn.Events[0].Skill.Args)
	assert.Equal(t, session.SkillSourceTool, turn.Events[0].Skill.Source)

	// slash-command-tags
	p = NewParser()
	line = []byte(`{"type":"user","promptId":"p1","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"user","content":"<command-name>/peek</command-name><command-args>latest</command-args>"}}`)
	turn = p.ParseLine(line)
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindSkillInvoked, turn.Events[0].Kind)
	assert.Equal(t, "peek", turn.Events[0].Skill.Skill)
	assert.Equal(t, "latest", turn.Events[0].Skill.Args)
	assert.Equal(t, session.SkillSourceSlash, turn.Events[0].Skill.Source)

	// skill-inside-sidechain-actor-set
	p = NewParser()
	line = []byte(`{"type":"assistant","sessionId":"s","agentId":"sub-ag","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":true,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu2","name":"Skill","input":{"skill":"jq"}}]}}`)
	turn = p.ParseLine(line)
	require.NotNil(t, turn)
	require.True(t, turn.IsEventSignal())
	require.Len(t, turn.Events, 1)
	assert.Equal(t, "sub-ag", turn.Events[0].Actor)
}

func TestParseLine_PlanEvents(t *testing.T) {
	// exit-plan-approved-inline
	p := NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"ExitPlanMode","input":{}}]}}`))
	turn := p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","is_error":false,"content":"User has approved your plan. Here it is."}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindPlanApproved, turn.Events[0].Kind)
	assert.Contains(t, turn.Events[0].Plan.Content, "User has approved your plan")

	// exit-plan-approved-persisted-output
	p = NewParser()
	dir := t.TempDir()
	sessionId := "sess-persist"
	toolResultsPath := filepath.Join(dir, sessionId, "tool-results")
	require.NoError(t, os.MkdirAll(toolResultsPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(toolResultsPath, "tuA.txt"), []byte("User has approved your plan\nbig content"), 0o644))
	pointer := "<persisted-output>\nFull output saved to: " + filepath.Join(toolResultsPath, "tuA.txt") + "\n</persisted-output>"
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"` + sessionId + `","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tuA","name":"ExitPlanMode","input":{}}]}}`))
	resultLine := `{"type":"user","sessionId":"` + sessionId + `","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tuA","is_error":false,"content":` + jsonString(pointer) + `}]}}`
	turn = p.ParseLine([]byte(resultLine))
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindPlanApproved, turn.Events[0].Kind)
	assert.Contains(t, turn.Events[0].Plan.Content, "big content")

	// exit-plan-rejected
	p = NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu2","name":"ExitPlanMode","input":{}}]}}`))
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu2","is_error":true,"content":"The user doesn't want to proceed with this tool use. And gave feedback."}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindPlanRejected, turn.Events[0].Kind)

	// mode-enter-exit-reenter-attachments
	p = NewParser()
	enter := p.ParseLine([]byte(`{"type":"attachment","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"attachment":{"type":"plan_mode","planFilePath":"/p.md"}}`))
	require.NotNil(t, enter)
	assert.Equal(t, session.EventKindPlanModeEnter, enter.Events[0].Kind)
	exit := p.ParseLine([]byte(`{"type":"attachment","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"attachment":{"type":"plan_mode_exit","planFilePath":"/p.md"}}`))
	require.NotNil(t, exit)
	assert.Equal(t, session.EventKindPlanModeExit, exit.Events[0].Kind)
	reenter := p.ParseLine([]byte(`{"type":"attachment","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"attachment":{"type":"plan_mode_reentry","planFilePath":"/p.md"}}`))
	require.NotNil(t, reenter)
	assert.Equal(t, session.EventKindPlanModeReenter, reenter.Events[0].Kind)

	// plan-file-reference-no-event
	p = NewParser()
	ref := p.ParseLine([]byte(`{"type":"attachment","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"attachment":{"type":"plan_file_reference","planFilePath":"/p.md","planContent":"# Plan"}}`))
	require.NotNil(t, ref)
	assert.Equal(t, "/p.md", ref.PlanFilePath)
	assert.Empty(t, ref.Events)
}

func TestParseLine_PermissionAndAnswers(t *testing.T) {
	// denied-edit-tool
	p := NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Edit","input":{}}]}}`))
	turn := p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","is_error":true,"content":"The user doesn't want to proceed with this tool use."}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindPermissionDenied, turn.Events[0].Kind)
	assert.Equal(t, "Edit", turn.Events[0].Permission.Tool)

	// ask-user-question-answered
	p = NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu2","name":"AskUserQuestion","input":{"questions":[{"question":"Proceed?"}]}}]}}`))
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu2","is_error":false,"content":"Your questions have been answered: \"Proceed?\"=\"Yes\""}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindUserAnswer, turn.Events[0].Kind)
	assert.Equal(t, []string{"Proceed?"}, turn.Events[0].UserAnswer.Questions)
	assert.Contains(t, turn.Events[0].UserAnswer.Answers, "Yes")

	// ask-user-question-denied
	p = NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu3","name":"AskUserQuestion","input":{"questions":[{"question":"Q"}]}}]}}`))
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu3","is_error":true,"content":"The user doesn't want to proceed with this tool use."}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindPermissionDenied, turn.Events[0].Kind)
	assert.Equal(t, "AskUserQuestion", turn.Events[0].Permission.Tool)

	// unknown-tool-result-ignored
	p = NewParser()
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"never-seen","is_error":true,"content":"The user doesn't want to proceed with this tool use."}]}}`))
	assert.Nil(t, turn)
}

func TestParseLine_SubagentResult(t *testing.T) {
	// agent-result-captured
	p := NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu1","name":"Agent","input":{}}]}}`))
	turn := p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","is_error":false,"content":"subagent findings"}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindSubagentResult, turn.Events[0].Kind)
	assert.Equal(t, "subagent findings", turn.Events[0].Subagent.Content)
	assert.Equal(t, "tu1", turn.Events[0].Subagent.ToolUseId)

	// agent-result-truncated-32k
	p = NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu2","name":"Agent","input":{}}]}}`))
	big := strings.Repeat("x", 40*1024)
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu2","is_error":false,"content":` + jsonString(big) + `}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Contains(t, turn.Events[0].Subagent.Content, "truncated at 32 KB")

	// agent-denied-is-permission-event
	p = NewParser()
	p.ParseLine([]byte(`{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu3","name":"Agent","input":{}}]}}`))
	turn = p.ParseLine([]byte(`{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu3","is_error":true,"content":"The user doesn't want to proceed with this tool use."}]}}`))
	require.NotNil(t, turn)
	require.Len(t, turn.Events, 1)
	assert.Equal(t, session.EventKindPermissionDenied, turn.Events[0].Kind)
	assert.Equal(t, "Agent", turn.Events[0].Permission.Tool)
}

func TestResolvePersistedOutput(t *testing.T) {
	type testCase struct {
		_id       string
		_expected string
		build     func(dir string) (text string, sessionId session.Id, toolUseId string)
	}

	tests := make([]*testCase, 0)

	// valid-session-path
	tests = append(tests, &testCase{
		_id:       "valid-session-path",
		_expected: "resolved body",
		build: func(dir string) (string, session.Id, string) {
			sessionId := session.Id("sess-x")
			toolUseId := "tuV"
			resultsDir := filepath.Join(dir, string(sessionId), "tool-results")
			require.NoError(t, os.MkdirAll(resultsDir, 0o755))
			path := filepath.Join(resultsDir, toolUseId+".txt")
			require.NoError(t, os.WriteFile(path, []byte("resolved body"), 0o644))
			return "<persisted-output>\nFull output saved to: " + path + "\n", sessionId, toolUseId
		},
	})

	// foreign-path-rejected
	tests = append(tests, &testCase{
		_id: "foreign-path-rejected",
		build: func(dir string) (string, session.Id, string) {
			foreign := filepath.Join(dir, "elsewhere", "tuV.txt")
			require.NoError(t, os.MkdirAll(filepath.Dir(foreign), 0o755))
			require.NoError(t, os.WriteFile(foreign, []byte("secret"), 0o644))
			text := "<persisted-output>\nFull output saved to: " + foreign + "\n"
			return text, session.Id("sess-x"), "tuV"
		},
	})

	// wrong-tool-use-id-rejected
	tests = append(tests, &testCase{
		_id: "wrong-tool-use-id-rejected",
		build: func(dir string) (string, session.Id, string) {
			sessionId := session.Id("sess-x")
			resultsDir := filepath.Join(dir, string(sessionId), "tool-results")
			require.NoError(t, os.MkdirAll(resultsDir, 0o755))
			path := filepath.Join(resultsDir, "other.txt")
			require.NoError(t, os.WriteFile(path, []byte("body"), 0o644))
			text := "<persisted-output>\nFull output saved to: " + path + "\n"
			return text, sessionId, "tuV"
		},
	})

	// missing-file-falls-back
	tests = append(tests, &testCase{
		_id: "missing-file-falls-back",
		build: func(dir string) (string, session.Id, string) {
			sessionId := session.Id("sess-x")
			path := filepath.Join(dir, string(sessionId), "tool-results", "tuV.txt")
			text := "<persisted-output>\nFull output saved to: " + path + "\n"
			return text, sessionId, "tuV"
		},
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			dir := t.TempDir()
			text, sessionId, toolUseId := test.build(dir)
			got := resolvePersistedOutput(text, sessionId, toolUseId)
			if test._expected != "" {
				assert.Equal(t, test._expected, got)
				return
			}
			assert.Equal(t, text, got, "rejected path returns original text")
		})
	}
}
