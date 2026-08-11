package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSession_AddSubagentTurn(t *testing.T) {
	base := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)

	// usage-summed-with-dedup
	t.Run("usage-summed-with-dedup", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddSubagentTurn(&Turn{SubagentId: "sub-1", RequestId: "r1", Usage: &Usage{InputTokens: 10}, Meta: &Meta{SessionId: "sess-123"}})
		s.AddSubagentTurn(&Turn{SubagentId: "sub-1", RequestId: "r1", Usage: &Usage{InputTokens: 10}, Meta: &Meta{SessionId: "sess-123"}})
		s.AddSubagentTurn(&Turn{SubagentId: "sub-1", RequestId: "r2", Usage: &Usage{InputTokens: 5}, Meta: &Meta{SessionId: "sess-123"}})

		assert.Equal(t, 15, s.Subagents["sub-1"].Usage.InputTokens)
		assert.Equal(t, 0, s.TotalUsage.InputTokens, "subagent usage never enters TotalUsage")
	})

	// first-last-active-span
	t.Run("first-last-active-span", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddSubagentTurn(&Turn{SubagentId: "sub-1", Timestamp: base, Meta: &Meta{SessionId: "sess-123"}})
		s.AddSubagentTurn(&Turn{SubagentId: "sub-1", Timestamp: base.Add(time.Minute), Meta: &Meta{SessionId: "sess-123"}})

		assert.Equal(t, base, s.Subagents["sub-1"].FirstActive)
		assert.Equal(t, base.Add(time.Minute), s.Subagents["sub-1"].LastActive)
	})

	// spawn-event-fills-type-and-description
	t.Run("spawn-event-fills-type-and-description", func(t *testing.T) {
		s := provideCompleteSession()
		spawn := &Event{
			Actor:    "sub-1",
			Kind:     EventKindSubagentSpawned,
			Subagent: &SubagentPayload{AgentId: "sub-1", AgentType: "Explore", Description: "scan repo"},
		}
		s.AddSubagentTurn(&Turn{SubagentId: "sub-1", Events: []*Event{spawn}, Meta: &Meta{SessionId: "sess-123"}})

		assert.Equal(t, "Explore", s.Subagents["sub-1"].AgentType)
		assert.Equal(t, "scan repo", s.Subagents["sub-1"].Description)
	})
}

func TestSession_SkillWindows(t *testing.T) {
	base := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)

	skillEvent := func(name string, ts time.Time) *Event {
		return &Event{Kind: EventKindSkillInvoked, Skill: &SkillPayload{Skill: name, Source: SkillSourceTool}, Timestamp: ts}
	}

	// open-then-close-on-prompt
	t.Run("open-then-close-on-prompt", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(skillEvent("fchange", base))
		s.CloseSkillWindow(base.Add(time.Minute))

		require.Len(t, s.Skills, 1)
		assert.Equal(t, base, s.Skills[0].StartedAt)
		assert.Equal(t, base.Add(time.Minute), s.Skills[0].EndedAt)
	})

	// usage-attributed-with-dedup
	t.Run("usage-attributed-with-dedup", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(skillEvent("fchange", base))
		turn := &Turn{Role: RoleAssistant, RequestId: "r1", Usage: &Usage{OutputTokens: 7}, Timestamp: base.Add(time.Second), Meta: &Meta{SessionId: "sess-123"}}
		s.AddTurn(turn)
		s.AddTurn(turn)

		assert.Equal(t, 7, s.Skills[0].Usage.OutputTokens)
		assert.Equal(t, base.Add(time.Second), s.Skills[0].EndedAt)
	})

	// second-skill-closes-first
	t.Run("second-skill-closes-first", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(skillEvent("first", base))
		s.AddEvent(skillEvent("second", base.Add(time.Minute)))

		require.Len(t, s.Skills, 2)
		assert.Equal(t, base.Add(time.Minute), s.Skills[0].EndedAt)
		assert.True(t, s.Skills[1].EndedAt.IsZero())
	})

	// same-prompt-id-keeps-window-open
	t.Run("same-prompt-id-keeps-window-open", func(t *testing.T) {
		s := provideCompleteSession()
		s.HandlePromptBoundary("p1", base)
		s.AddEvent(skillEvent("fchange", base))
		s.HandlePromptBoundary("p1", base.Add(time.Second))
		s.HandlePromptBoundary("p2", base.Add(time.Minute))

		require.Len(t, s.Skills, 1)
		assert.Equal(t, base.Add(time.Minute), s.Skills[0].EndedAt)
	})

	// empty-prompt-id-always-closes
	t.Run("empty-prompt-id-always-closes", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(skillEvent("fchange", base))
		s.HandlePromptBoundary("", base.Add(time.Second))

		require.Len(t, s.Skills, 1)
		assert.Equal(t, base.Add(time.Second), s.Skills[0].EndedAt)
	})

	// close-without-window-noop
	t.Run("close-without-window-noop", func(t *testing.T) {
		s := provideCompleteSession()
		s.CloseSkillWindow(base)
		assert.Empty(t, s.Skills)
	})

	// stop-reason-closes-window
	t.Run("stop-reason-closes-window", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(skillEvent("fchange", base))
		s.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r1", StopReason: "end_turn", Usage: &Usage{OutputTokens: 7}, Timestamp: base.Add(time.Second), Meta: &Meta{SessionId: "sess-123"}})
		s.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r2", Usage: &Usage{OutputTokens: 9}, Timestamp: base.Add(time.Minute), Meta: &Meta{SessionId: "sess-123"}})

		assert.Equal(t, base.Add(time.Second), s.Skills[0].EndedAt)
		assert.Equal(t, 7, s.Skills[0].Usage.OutputTokens, "turns after the stop no longer attribute")
	})

	// tool-use-keeps-window-open
	t.Run("tool-use-keeps-window-open", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(skillEvent("fchange", base))
		s.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r1", StopReason: StopReasonToolUse, Usage: &Usage{OutputTokens: 7}, Timestamp: base.Add(time.Second), Meta: &Meta{SessionId: "sess-123"}})
		s.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r2", Usage: &Usage{OutputTokens: 9}, Timestamp: base.Add(time.Minute), Meta: &Meta{SessionId: "sess-123"}})

		assert.Equal(t, 16, s.Skills[0].Usage.OutputTokens)
		assert.Equal(t, base.Add(time.Minute), s.Skills[0].EndedAt)
	})

	// prompt-close-keeps-last-activity
	t.Run("prompt-close-keeps-last-activity", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(skillEvent("fchange", base))
		s.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r1", Usage: &Usage{OutputTokens: 7}, Timestamp: base.Add(time.Second), Meta: &Meta{SessionId: "sess-123"}})
		s.HandlePromptBoundary("p2", base.Add(30*time.Minute))

		assert.Equal(t, base.Add(time.Second), s.Skills[0].EndedAt, "idle gap before the next prompt never counts")
	})

	// model-captured-from-first-attributed-turn
	t.Run("model-captured-from-first-attributed-turn", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(skillEvent("fchange", base))
		s.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r1", Usage: &Usage{OutputTokens: 7}, Timestamp: base.Add(time.Second), Meta: &Meta{SessionId: "sess-123", Model: "claude-fable-5"}})
		s.AddTurn(&Turn{Role: RoleAssistant, RequestId: "r2", Usage: &Usage{OutputTokens: 9}, Timestamp: base.Add(time.Minute), Meta: &Meta{SessionId: "sess-123", Model: "claude-opus-4"}})

		assert.Equal(t, "claude-fable-5", s.Skills[0].Model)
	})
}
