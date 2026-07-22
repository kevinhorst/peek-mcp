package tools

import (
	"strings"
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
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
