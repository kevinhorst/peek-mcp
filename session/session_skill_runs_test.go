package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var skillRunsStart = time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC)

func provideSkillEvent(actor, name string, timestamp time.Time) *Event {
	return &Event{
		Actor:     actor,
		Kind:      EventKindSkillInvoked,
		Skill:     &SkillPayload{Args: "scope: branch", Skill: name, Source: SkillSourceSlash},
		Timestamp: timestamp,
	}
}

func provideRootTurn(requestId string, timestamp time.Time, usage *Usage) *Turn {
	return &Turn{
		Meta:      &Meta{Model: "claude-opus-5-5", SessionId: "sess-123"},
		RequestId: requestId,
		Role:      RoleAssistant,
		Timestamp: timestamp,
		Usage:     usage,
	}
}

func provideSubagentTurn(agentId, agentType, model, requestId string, timestamp time.Time, usage *Usage) *Turn {
	spawn := &Event{
		Actor:    agentId,
		Kind:     EventKindSubagentSpawned,
		Subagent: &SubagentPayload{AgentId: agentId, AgentType: agentType},
	}
	return &Turn{
		Events:     []*Event{spawn},
		Meta:       &Meta{Model: model, SessionId: "sess-123"},
		RequestId:  requestId,
		Role:       RoleAssistant,
		SubagentId: agentId,
		Timestamp:  timestamp,
		Usage:      usage,
	}
}

func TestSession_SkillRuns(t *testing.T) {
	at := func(minutes int) time.Time {
		return skillRunsStart.Add(time.Duration(minutes) * time.Minute)
	}

	// root-invocation-opens-run
	t.Run("root-invocation-opens-run", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(provideSkillEvent("", "railroad-review", at(0)))

		require.Len(t, s.SkillRuns, 1)
		assert.Equal(t, "railroad-review", s.SkillRuns[0].Skill)
		assert.Equal(t, "scope: branch", s.SkillRuns[0].Args)
		assert.Equal(t, SkillSourceSlash, s.SkillRuns[0].Source)
		assert.Equal(t, at(0), s.SkillRuns[0].StartedAt)
		assert.Same(t, s.SkillRuns[0], s.activeRun)
	})

	// subagent-invocation-opens-no-run
	t.Run("subagent-invocation-opens-no-run", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(provideSkillEvent("", "railroad-review", at(0)))
		s.AddEvent(provideSkillEvent("lane-1", "railroad-review", at(1)))

		require.Len(t, s.SkillRuns, 1)
		assert.Same(t, s.SkillRuns[0], s.activeRun)
	})

	// next-root-invocation-closes-run
	t.Run("next-root-invocation-closes-run", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(provideSkillEvent("", "fdesign", at(0)))
		s.AddEvent(provideSkillEvent("", "fimplement", at(30)))

		require.Len(t, s.SkillRuns, 2)
		assert.Equal(t, at(30), s.SkillRuns[0].EndedAt)
		assert.True(t, s.SkillRuns[1].EndedAt.IsZero())
		assert.Same(t, s.SkillRuns[1], s.activeRun)
	})

	// root-usage-deduplicated-across-chunks
	t.Run("root-usage-deduplicated-across-chunks", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(provideSkillEvent("", "fdesign", at(0)))
		s.AddTurn(provideRootTurn("r1", at(1), &Usage{CacheReadInputTokens: 900, InputTokens: 100, OutputTokens: 3}))
		s.AddTurn(provideRootTurn("r1", at(1), &Usage{CacheReadInputTokens: 900, InputTokens: 100, OutputTokens: 8}))
		s.AddTurn(provideRootTurn("r2", at(2), &Usage{CacheCreationInputTokens: 50, CacheReadInputTokens: 1000, InputTokens: 2, OutputTokens: 5}))

		run := s.SkillRuns[0]
		assert.Equal(t, 13, run.Usage.OutputTokens)
		assert.Equal(t, 1900, run.Usage.CacheReadInputTokens)
		assert.Equal(t, 2, run.Requests)
		assert.Equal(t, 1052, run.PeakContext)
		assert.Equal(t, at(2), run.LastActive)
		assert.Equal(t, "claude-opus-5-5", run.Model)
	})

	// run-outlives-the-turn-end
	t.Run("run-outlives-the-turn-end", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(provideSkillEvent("", "fdesign", at(0)))
		ended := provideRootTurn("r1", at(1), &Usage{OutputTokens: 7})
		ended.StopReason = "end_turn"
		s.AddTurn(ended)
		s.HandlePromptBoundary("p2", at(10))
		s.AddTurn(provideRootTurn("r2", at(11), &Usage{OutputTokens: 9}))

		assert.Equal(t, 16, s.SkillRuns[0].Usage.OutputTokens)
		assert.Equal(t, 7, s.Skills[0].Usage.OutputTokens, "the skill window still closes at the turn end")
	})

	// usage-before-any-invocation-belongs-to-no-run
	t.Run("usage-before-any-invocation-belongs-to-no-run", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddTurn(provideRootTurn("r1", at(0), &Usage{OutputTokens: 7}))
		s.AddEvent(provideSkillEvent("", "fdesign", at(1)))

		assert.Equal(t, 0, s.SkillRuns[0].Usage.OutputTokens)
		assert.Equal(t, 7, s.TotalUsage.OutputTokens)
	})

	// cap-closes-run-and-opens-none
	t.Run("cap-closes-run-and-opens-none", func(t *testing.T) {
		s := provideCompleteSession()
		for index := range maxSkillStats {
			s.AddEvent(provideSkillEvent("", "qq", at(index)))
		}
		s.AddEvent(provideSkillEvent("", "fdesign", at(maxSkillStats)))

		assert.Len(t, s.SkillRuns, maxSkillStats)
		assert.Nil(t, s.activeRun)
		assert.Equal(t, at(maxSkillStats), s.SkillRuns[maxSkillStats-1].EndedAt)
	})
}

func TestSession_SkillRunAgents(t *testing.T) {
	at := func(minutes int) time.Time {
		return skillRunsStart.Add(time.Duration(minutes) * time.Minute)
	}

	// grouped-by-agent-type-and-model
	t.Run("grouped-by-agent-type-and-model", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(provideSkillEvent("", "railroad-review", at(0)))
		s.AddSubagentTurn(provideSubagentTurn("a1", "railroad-lane-tests", "claude-opus-5-5", "a1-r1", at(1), &Usage{CacheReadInputTokens: 400, InputTokens: 10, OutputTokens: 4}))
		s.AddSubagentTurn(provideSubagentTurn("a1", "railroad-lane-tests", "claude-opus-5-5", "a1-r2", at(3), &Usage{CacheReadInputTokens: 700, InputTokens: 10, OutputTokens: 6}))
		s.AddSubagentTurn(provideSubagentTurn("a2", "railroad-lane-tests", "claude-opus-5-5", "a2-r1", at(2), &Usage{CacheReadInputTokens: 900, InputTokens: 10, OutputTokens: 1}))
		s.AddSubagentTurn(provideSubagentTurn("a3", "railroad-grouper", "claude-opus-5-5", "a3-r1", at(4), &Usage{InputTokens: 50, OutputTokens: 2}))
		s.AddSubagentTurn(provideSubagentTurn("a4", "railroad-lane-tests", "claude-sonnet-5-5", "a4-r1", at(5), &Usage{InputTokens: 20, OutputTokens: 3}))

		agents := s.SkillRunAgents(s.SkillRuns[0])

		require.Len(t, agents, 3)
		assert.Equal(t, "railroad-grouper", agents[0].AgentType)
		assert.Equal(t, 1, agents[0].Count)

		lanes := agents[1]
		assert.Equal(t, "railroad-lane-tests", lanes.AgentType)
		assert.Equal(t, "claude-opus-5-5", lanes.Model)
		assert.Equal(t, 2, lanes.Count)
		assert.Equal(t, 3, lanes.Requests)
		assert.Equal(t, 910, lanes.PeakContext)
		assert.Equal(t, 11, lanes.Usage.OutputTokens)
		assert.Equal(t, 120, lanes.Seconds)
		assert.Equal(t, at(3), lanes.LastActive)

		assert.Equal(t, "claude-sonnet-5-5", agents[2].Model)
	})

	// subagents-outside-the-run-excluded
	t.Run("subagents-outside-the-run-excluded", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddSubagentTurn(provideSubagentTurn("before", "Explore", "claude-opus-5-5", "b-r1", at(0), &Usage{OutputTokens: 1}))
		s.AddEvent(provideSkillEvent("", "fdesign", at(1)))
		s.AddSubagentTurn(provideSubagentTurn("inside", "Explore", "claude-opus-5-5", "i-r1", at(2), &Usage{OutputTokens: 2}))
		s.AddEvent(provideSkillEvent("", "fimplement", at(3)))
		s.AddSubagentTurn(provideSubagentTurn("after", "Explore", "claude-opus-5-5", "a-r1", at(4), &Usage{OutputTokens: 4}))

		first := s.SkillRunAgents(s.SkillRuns[0])
		require.Len(t, first, 1)
		assert.Equal(t, 1, first[0].Count)
		assert.Equal(t, 2, first[0].Usage.OutputTokens)

		open := s.SkillRunAgents(s.SkillRuns[1])
		require.Len(t, open, 1)
		assert.Equal(t, 4, open[0].Usage.OutputTokens, "the open run takes every later subagent")
	})

	// no-subagents-no-rows
	t.Run("no-subagents-no-rows", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddEvent(provideSkillEvent("", "qq", at(0)))

		assert.Empty(t, s.SkillRunAgents(s.SkillRuns[0]))
	})
}

func TestSession_SubagentContextLoad(t *testing.T) {
	// requests-counted-once-per-request-id
	t.Run("requests-counted-once-per-request-id", func(t *testing.T) {
		s := provideCompleteSession()
		s.AddSubagentTurn(&Turn{SubagentId: "sub-1", RequestId: "r1", Usage: &Usage{CacheReadInputTokens: 300, InputTokens: 10}, Meta: &Meta{SessionId: "sess-123"}})
		s.AddSubagentTurn(&Turn{SubagentId: "sub-1", RequestId: "r1", Usage: &Usage{CacheReadInputTokens: 300, InputTokens: 10}, Meta: &Meta{SessionId: "sess-123"}})
		s.AddSubagentTurn(&Turn{SubagentId: "sub-1", RequestId: "r2", Usage: &Usage{CacheCreationInputTokens: 20, CacheReadInputTokens: 500, InputTokens: 5}, Meta: &Meta{SessionId: "sess-123"}})

		assert.Equal(t, 2, s.Subagents["sub-1"].Requests)
		assert.Equal(t, 525, s.Subagents["sub-1"].PeakContext)
	})
}
