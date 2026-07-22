package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSession_AddEventCounters(t *testing.T) {
	// denial-increments
	t.Run("denial-increments", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(&Event{Kind: EventKindPermissionDenied})
		assert.Equal(t, 1, s.Counters.PermissionDenials)
	})

	// rejection-increments
	t.Run("rejection-increments", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(&Event{Kind: EventKindPlanRejected})
		assert.Equal(t, 1, s.Counters.PlanRejections)
	})

	// skill-increments
	t.Run("skill-increments", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(&Event{Kind: EventKindSkillInvoked})
		assert.Equal(t, 1, s.Counters.SkillsInvoked)
	})

	// spawn-increments
	t.Run("spawn-increments", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(&Event{Kind: EventKindSubagentSpawned})
		assert.Equal(t, 1, s.Counters.SubagentsSpawned)
	})

	// mode-exit-sets-phase
	t.Run("mode-exit-sets-phase", func(t *testing.T) {
		s := provideCompleteSession()
		assert.False(t, s.isAlterationPhase())
		s.AddEvent(&Event{Kind: EventKindPlanModeExit})
		assert.True(t, s.isAlterationPhase())
	})
}

func TestSession_CurrentUsage(t *testing.T) {
	// totals-plus-active-turn
	t.Run("totals-plus-active-turn", func(t *testing.T) {
		s := provideCompleteSession()
		s.TotalUsage = Usage{InputTokens: 100, OutputTokens: 40}
		s.TurnActive = &Turn{Usage: &Usage{InputTokens: 10, OutputTokens: 5}}

		usage := s.CurrentUsage()
		assert.Equal(t, 110, usage.InputTokens)
		assert.Equal(t, 45, usage.OutputTokens)
		assert.Equal(t, 100, s.TotalUsage.InputTokens, "TotalUsage must not be mutated")
	})

	// no-active-turn
	t.Run("no-active-turn", func(t *testing.T) {
		s := provideCompleteSession()
		s.TotalUsage = Usage{InputTokens: 100}

		usage := s.CurrentUsage()
		assert.Equal(t, 100, usage.InputTokens)
	})
}
