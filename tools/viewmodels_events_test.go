package tools

import (
	"strings"
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/kevinhorst/peek-mcp/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarizeEvent(t *testing.T) {
	type testCase struct {
		_id       string
		_expected string
		event     *session.Event
	}

	tests := make([]*testCase, 0)

	// permission-denied
	tests = append(tests, &testCase{
		_id:       "permission-denied",
		_expected: "exec_command: rm -rf",
		event:     &session.Event{Kind: session.EventKindPermissionDenied, Permission: &session.PermissionPayload{Tool: "exec_command", Command: "rm -rf"}},
	})

	// permission-denied-no-command
	tests = append(tests, &testCase{
		_id:       "permission-denied-no-command",
		_expected: "Edit",
		event:     &session.Event{Kind: session.EventKindPermissionDenied, Permission: &session.PermissionPayload{Tool: "Edit"}},
	})

	// permission-granted
	tests = append(tests, &testCase{
		_id:       "permission-granted",
		_expected: "exec_command: make build",
		event:     &session.Event{Kind: session.EventKindPermissionGranted, Permission: &session.PermissionPayload{Tool: "exec_command", Command: "make build"}},
	})

	// permission-mode-changed
	tests = append(tests, &testCase{
		_id:       "permission-mode-changed",
		_expected: "default -> bypassPermissions",
		event:     &session.Event{Kind: session.EventKindPermissionModeChanged, PermissionMode: &session.PermissionModePayload{From: "default", To: "bypassPermissions"}},
	})

	// permission-mode-initial
	tests = append(tests, &testCase{
		_id:       "permission-mode-initial",
		_expected: "bypassPermissions",
		event:     &session.Event{Kind: session.EventKindPermissionModeChanged, PermissionMode: &session.PermissionModePayload{To: "bypassPermissions"}},
	})

	// model-changed
	tests = append(tests, &testCase{
		_id:       "model-changed",
		_expected: "claude-opus-4-6 -> claude-fable-5",
		event:     &session.Event{Kind: session.EventKindModelChanged, Model: &session.ModelPayload{From: "claude-opus-4-6", To: "claude-fable-5"}},
	})

	// plan-approved-empty
	tests = append(tests, &testCase{
		_id:       "plan-approved-empty",
		_expected: "",
		event:     &session.Event{Kind: session.EventKindPlanApproved},
	})

	// plan-mode-enter-empty
	tests = append(tests, &testCase{
		_id:       "plan-mode-enter-empty",
		_expected: "",
		event:     &session.Event{Kind: session.EventKindPlanModeEnter},
	})

	// plan-mode-exit-empty
	tests = append(tests, &testCase{
		_id:       "plan-mode-exit-empty",
		_expected: "",
		event:     &session.Event{Kind: session.EventKindPlanModeExit},
	})

	// plan-mode-reenter-empty
	tests = append(tests, &testCase{
		_id:       "plan-mode-reenter-empty",
		_expected: "",
		event:     &session.Event{Kind: session.EventKindPlanModeReenter},
	})

	// plan-rejected-empty
	tests = append(tests, &testCase{
		_id:       "plan-rejected-empty",
		_expected: "",
		event:     &session.Event{Kind: session.EventKindPlanRejected},
	})

	// plan-revised
	tests = append(tests, &testCase{
		_id:       "plan-revised",
		_expected: "revision 3",
		event:     &session.Event{Kind: session.EventKindPlanRevised, Plan: &session.PlanPayload{Revision: 3}},
	})

	// skill-invoked
	tests = append(tests, &testCase{
		_id:       "skill-invoked",
		_expected: "feature-design raw",
		event:     &session.Event{Kind: session.EventKindSkillInvoked, Skill: &session.SkillPayload{Skill: "feature-design", Args: "raw"}},
	})

	// subagent-spawned
	tests = append(tests, &testCase{
		_id:       "subagent-spawned",
		_expected: "explore: survey the code",
		event:     &session.Event{Kind: session.EventKindSubagentSpawned, Subagent: &session.SubagentPayload{AgentType: "explore", Description: "survey the code"}},
	})

	// subagent-result
	tests = append(tests, &testCase{
		_id:       "subagent-result",
		_expected: "first line",
		event:     &session.Event{Kind: session.EventKindSubagentResult, Subagent: &session.SubagentPayload{Content: "first line\nsecond line"}},
	})

	// user-answer
	tests = append(tests, &testCase{
		_id:       "user-answer",
		_expected: "yes proceed",
		event:     &session.Event{Kind: session.EventKindUserAnswer, UserAnswer: &session.UserAnswerPayload{Answers: "yes proceed\ndetails"}},
	})

	// 200-char-truncation
	tests = append(tests, &testCase{
		_id:       "200-char-truncation",
		_expected: "skill " + strings.Repeat("x", maxEventSummaryChars-len("skill ")),
		event:     &session.Event{Kind: session.EventKindSkillInvoked, Skill: &session.SkillPayload{Skill: "skill", Args: strings.Repeat("x", 400)}},
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			assert.Equal(t, test._expected, summarizeEvent(test.event))
		})
	}
}

const promptedDecisionLogs = `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":"1787749325706000000","body":{"stringValue":"claude_code.tool_decision"},"attributes":[{"key":"session.id","value":{"stringValue":"s1"}},{"key":"decision","value":{"stringValue":"accept"}},{"key":"source","value":{"stringValue":"user_temporary"}},{"key":"tool_name","value":{"stringValue":"Bash"}},{"key":"tool_use_id","value":{"stringValue":"tu1"}}]}]}]}]}`

func TestNewPermissionsView(t *testing.T) {
	claudeSession := func() *session.Session {
		return &session.Session{Agent: session.AgentClaude, Meta: session.Meta{SessionId: "s1"}}
	}

	// live-store-served
	t.Run("live-store-served", func(t *testing.T) {
		store := telemetry.NewStore()
		require.NoError(t, store.IngestLogs([]byte(promptedDecisionLogs)))

		view := newPermissionsView(claudeSession(), store, nil)

		require.NotNil(t, view)
		assert.Equal(t, 1, view.PromptedOnce)
		require.Len(t, view.Requests, 1)
		assert.Equal(t, "Bash", view.Requests[0].Tool)
		assert.Empty(t, view.Detail)
	})

	// persisted-fallback
	t.Run("persisted-fallback", func(t *testing.T) {
		dir := state.NewDir(t.TempDir())
		writer := telemetry.NewStore()
		writer.StateDir = dir
		require.NoError(t, writer.IngestLogs([]byte(promptedDecisionLogs)))

		view := newPermissionsView(claudeSession(), telemetry.NewStore(), dir)

		require.NotNil(t, view)
		assert.Equal(t, 1, view.PromptedOnce)
		assert.Equal(t, "persisted", view.Detail)
	})

	// no-data-nil
	t.Run("no-data-nil", func(t *testing.T) {
		assert.Nil(t, newPermissionsView(claudeSession(), telemetry.NewStore(), nil))
	})

	// codex-session-nil
	t.Run("codex-session-nil", func(t *testing.T) {
		store := telemetry.NewStore()
		require.NoError(t, store.IngestLogs([]byte(promptedDecisionLogs)))
		codexSession := &session.Session{Agent: session.AgentCodex, Meta: session.Meta{SessionId: "s1"}}

		assert.Nil(t, newPermissionsView(codexSession, store, nil))
	})
}
