package session

import (
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddTurnBySessionId_Events(t *testing.T) {
	now := time.Now()

	// events-appended
	t.Run("events-appended", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		turn := &Turn{
			Events:    []*Event{{Kind: EventKindSkillInvoked, Skill: &SkillPayload{Skill: "jq"}}},
			Role:      RoleUser,
			Text:      "do it",
			Timestamp: now,
			Meta:      &Meta{SessionId: "s1"},
		}
		s.AddTurnBySessionId("s1", AgentClaude, turn)

		sess, ok := s.GetById("s1")
		require.True(t, ok)
		assert.Len(t, sess.Events.All(), 1)
		assert.Len(t, sess.Turns(10), 1)
	})

	// event-only-turn-no-chat-turn
	t.Run("event-only-turn-no-chat-turn", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		turn := &Turn{
			Events: []*Event{{Kind: EventKindPlanModeEnter}},
			Meta:   &Meta{SessionId: "s1"},
		}
		s.AddTurnBySessionId("s1", AgentClaude, turn)

		sess, ok := s.GetById("s1")
		require.True(t, ok)
		assert.Len(t, sess.Events.All(), 1)
		assert.Empty(t, sess.Turns(10))
	})

	// subagent-events-drop-unknown-parent
	t.Run("subagent-events-drop-unknown-parent", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		turn := &Turn{
			Events:     []*Event{{Actor: "sub-1", Kind: EventKindSubagentSpawned, Subagent: &SubagentPayload{AgentId: "sub-1"}}},
			SubagentId: "sub-1",
			Meta:       &Meta{SessionId: "unknown-parent"},
		}
		s.AddTurnBySessionId("unknown-parent", AgentClaude, turn)

		_, ok := s.GetById("unknown-parent")
		assert.False(t, ok, "subagent events never create a session")
	})

	// subagent-result-resolves-agent-id
	t.Run("subagent-result-resolves-agent-id", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		// Parent session exists via a chat turn
		chatTurn := &Turn{
			Role:      RoleUser,
			Text:      "start",
			Timestamp: now,
			Meta:      &Meta{SessionId: "p"},
		}
		s.AddTurnBySessionId("p", AgentClaude, chatTurn)
		// Spawned event (subagent signal, actor set)
		spawnedTurn := &Turn{
			Events:     []*Event{{Actor: "sub-9", Kind: EventKindSubagentSpawned, Subagent: &SubagentPayload{AgentId: "sub-9", ToolUseId: "tu"}}},
			SubagentId: "sub-9",
			Meta:       &Meta{SessionId: "p"},
		}
		s.AddTurnBySessionId("p", AgentClaude, spawnedTurn)
		// Result event on the parent, agent id unknown at parse time
		resultTurn := &Turn{
			Events: []*Event{{Kind: EventKindSubagentResult, Subagent: &SubagentPayload{ToolUseId: "tu"}}},
			Meta:   &Meta{SessionId: "p"},
		}
		s.AddTurnBySessionId("p", AgentClaude, resultTurn)

		sess, ok := s.GetById("p")
		require.True(t, ok)
		events := sess.Events.All()
		result := events[len(events)-1]
		assert.Equal(t, EventKindSubagentResult, result.Kind)
		assert.Equal(t, "sub-9", result.Subagent.AgentId)
	})

	// subagent-spawn-backfills-earlier-result
	t.Run("subagent-spawn-backfills-earlier-result", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		// Parent session exists via a chat turn
		chatTurn := &Turn{
			Role:      RoleUser,
			Text:      "start",
			Timestamp: now,
			Meta:      &Meta{SessionId: "p"},
		}
		s.AddTurnBySessionId("p", AgentClaude, chatTurn)
		// Restart replay order: result from the parent transcript arrives first
		resultTurn := &Turn{
			Events: []*Event{{Kind: EventKindSubagentResult, Subagent: &SubagentPayload{ToolUseId: "tu"}}},
			Meta:   &Meta{SessionId: "p"},
		}
		s.AddTurnBySessionId("p", AgentClaude, resultTurn)
		// Spawned event from the deferred subagent meta pass arrives second
		spawnedTurn := &Turn{
			Events:     []*Event{{Actor: "sub-9", Kind: EventKindSubagentSpawned, Subagent: &SubagentPayload{AgentId: "sub-9", ToolUseId: "tu"}}},
			SubagentId: "sub-9",
			Meta:       &Meta{SessionId: "p"},
		}
		s.AddTurnBySessionId("p", AgentClaude, spawnedTurn)

		sess, ok := s.GetById("p")
		require.True(t, ok)
		var result *Event
		for _, event := range sess.Events.All() {
			if event.Kind == EventKindSubagentResult {
				result = event
			}
		}
		require.NotNil(t, result)
		assert.Equal(t, "sub-9", result.Subagent.AgentId)
	})

	// usage-signal-keep-last
	t.Run("usage-signal-keep-last", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		firstSnapshot := &Turn{
			Usage: &Usage{InputTokens: 100, TotalTokens: 100},
			Meta:  &Meta{SessionId: "c"},
		}
		s.AddTurnBySessionId("c", AgentCodex, firstSnapshot)
		secondSnapshot := &Turn{
			Usage: &Usage{InputTokens: 250, TotalTokens: 250},
			Meta:  &Meta{SessionId: "c"},
		}
		s.AddTurnBySessionId("c", AgentCodex, secondSnapshot)

		sess, ok := s.GetById("c")
		require.True(t, ok)
		assert.Equal(t, 250, sess.TotalUsage.TotalTokens, "cumulative snapshots are kept-last, not summed")
	})
}

func TestAddTurnBySessionId_SkillWindowOrdering(t *testing.T) {
	now := time.Now()

	// slash-command-prompt-close-then-open
	t.Run("slash-command-prompt-close-then-open", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		first := &Turn{
			Events:    []*Event{{Kind: EventKindSkillInvoked, Skill: &SkillPayload{Skill: "first", Source: SkillSourceSlash}, Timestamp: now}},
			Role:      RoleUser,
			Text:      "<command-name>/first</command-name>",
			Timestamp: now,
			Meta:      &Meta{SessionId: "s1"},
		}
		s.AddTurnBySessionId("s1", AgentClaude, first)
		second := &Turn{
			Events:    []*Event{{Kind: EventKindSkillInvoked, Skill: &SkillPayload{Skill: "second", Source: SkillSourceSlash}, Timestamp: now.Add(time.Minute)}},
			Role:      RoleUser,
			Text:      "<command-name>/second</command-name>",
			Timestamp: now.Add(time.Minute),
			Meta:      &Meta{SessionId: "s1"},
		}
		s.AddTurnBySessionId("s1", AgentClaude, second)

		sess, ok := s.GetById("s1")
		require.True(t, ok)
		require.Len(t, sess.Skills, 2)
		assert.Equal(t, "first", sess.Skills[0].Skill)
		assert.Equal(t, now.Add(time.Minute), sess.Skills[0].EndedAt)
		assert.True(t, sess.Skills[1].EndedAt.IsZero(), "second window stays open after its own prompt")
	})

	// subagent-turn-folds-touches-and-stats
	t.Run("subagent-turn-folds-touches-and-stats", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		chatTurn := &Turn{Role: RoleUser, Text: "start", Timestamp: now, Meta: &Meta{SessionId: "p"}}
		s.AddTurnBySessionId("p", AgentClaude, chatTurn)
		subTurn := &Turn{
			FileTouches: []*FileTouch{{Path: "/a.go", Write: true}},
			RequestId:   "r1",
			SubagentId:  "sub-1",
			Timestamp:   now,
			Usage:       &Usage{InputTokens: 3},
			Meta:        &Meta{SessionId: "p"},
		}
		s.AddTurnBySessionId("p", AgentClaude, subTurn)

		sess, ok := s.GetById("p")
		require.True(t, ok)
		assert.Equal(t, 1, sess.TouchedFiles["/a.go"].Writes)
		assert.Equal(t, 3, sess.Subagents["sub-1"].Usage.InputTokens)
		assert.Equal(t, 0, sess.TotalUsage.InputTokens)
	})
}

func TestPlanRevisions(t *testing.T) {
	// initial-version-recorded
	t.Run("initial-version-recorded", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		sess := s.getOrCreate("s1", AgentClaude)
		s.setPlanContent("# Plan v1", sess, time.Time{})

		require.Len(t, sess.PlanRevisions, 1)
		assert.Equal(t, "# Plan v1", sess.PlanRevisions[0].Content)
		assert.Empty(t, sess.Events.All())
	})

	// change-appends-diff-and-event
	t.Run("change-appends-diff-and-event", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		sess := s.getOrCreate("s1", AgentClaude)
		sess.planExitSeen = true
		s.setPlanContent("# Plan v1\n", sess, time.Time{})
		s.setPlanContent("# Plan v2\n", sess, time.Time{})

		require.Len(t, sess.PlanRevisions, 2)
		assert.NotEmpty(t, sess.PlanRevisions[1].Diff)
		assert.Equal(t, 1, sess.Counters.PlanAlterations)
		events := sess.Events.All()
		require.Len(t, events, 1)
		assert.Equal(t, EventKindPlanRevised, events[0].Kind)
	})

	// revision-timestamp-from-transcript-entry
	t.Run("revision-timestamp-from-transcript-entry", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		sess := s.getOrCreate("s1", AgentClaude)
		entryTime := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)
		s.setPlanContent("# Plan v1", sess, entryTime)

		require.Len(t, sess.PlanRevisions, 1)
		assert.Equal(t, entryTime, sess.PlanRevisions[0].Timestamp)
	})

	// identical-content-no-revision
	t.Run("identical-content-no-revision", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		sess := s.getOrCreate("s1", AgentClaude)
		s.setPlanContent("# Plan", sess, time.Time{})
		s.setPlanContent("# Plan", sess, time.Time{})

		assert.Len(t, sess.PlanRevisions, 1)
	})

	// codex-second-block-is-alteration
	t.Run("codex-second-block-is-alteration", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		sess := s.getOrCreate("s1", AgentCodex)
		s.setPlanContent("# Plan v1\n", sess, time.Time{})
		s.setPlanContent("# Plan v2\n", sess, time.Time{})

		require.Len(t, sess.PlanRevisions, 2)
		assert.True(t, sess.PlanRevisions[1].IsAlteration)
	})

	// claude-pre-exit-is-draft
	t.Run("claude-pre-exit-is-draft", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		sess := s.getOrCreate("s1", AgentClaude)
		s.setPlanContent("# Plan v1\n", sess, time.Time{})
		s.setPlanContent("# Plan v2\n", sess, time.Time{})

		require.Len(t, sess.PlanRevisions, 2)
		assert.False(t, sess.PlanRevisions[1].IsAlteration)
		assert.Equal(t, 0, sess.Counters.PlanAlterations)
	})

	// cap-50-keeps-counting
	t.Run("cap-50-keeps-counting", func(t *testing.T) {
		s := NewStore(10, events.NewBroker())
		sess := s.getOrCreate("s1", AgentCodex)
		for index := 0; index < 60; index++ {
			content := "# Plan v" + string(rune('A'+index%26)) + string(rune('a'+index%23)) + "\n"
			s.setPlanContent(content, sess, time.Time{})
		}

		assert.LessOrEqual(t, len(sess.PlanRevisions), maxPlanRevisions)
		assert.Greater(t, sess.Counters.PlanAlterations, maxPlanRevisions)
	})
}

func TestHydrateFromState(t *testing.T) {
	// pin-and-snapshot-restored
	t.Run("pin-and-snapshot-restored", func(t *testing.T) {
		dir := state.NewDir(t.TempDir())
		base := state.DiffBase{Sha: "abc1234", Target: "main"}
		require.NoError(t, dir.WriteDiffBase("claude", base, "s1"))
		require.NoError(t, dir.WriteDiffSnapshot("claude", "diff body", "s1"))

		s := NewStore(10, events.NewBroker())
		s.StateDir = dir
		sess := s.getOrCreate("s1", AgentClaude)

		assert.Equal(t, "abc1234", sess.DiffBase)
		assert.Equal(t, "main", sess.DiffTarget)
		assert.Equal(t, "diff body", sess.DiffOutput)
		assert.Equal(t, DiffSourceSnapshot, sess.DiffSource)
		assert.False(t, sess.DiffCapturedAt.IsZero())
	})

	// plan-revisions-restored-with-alteration-count
	t.Run("plan-revisions-restored-with-alteration-count", func(t *testing.T) {
		dir := state.NewDir(t.TempDir())
		initial := &state.PlanVersion{Content: "# initial", Index: 0}
		require.NoError(t, dir.WritePlanVersion("claude", "s1", initial))
		alteration := &state.PlanVersion{Content: "@@ diff @@", Index: 1, IsAlteration: true}
		require.NoError(t, dir.WritePlanVersion("claude", "s1", alteration))
		draft := &state.PlanVersion{Content: "@@ draft @@", Index: 2, IsAlteration: false}
		require.NoError(t, dir.WritePlanVersion("claude", "s1", draft))
		require.NoError(t, dir.WritePlanLatest("claude", "# latest", "s1"))

		s := NewStore(10, events.NewBroker())
		s.StateDir = dir
		sess := s.getOrCreate("s1", AgentClaude)

		require.Len(t, sess.PlanRevisions, 3)
		assert.Equal(t, "# initial", sess.PlanRevisions[0].Content)
		assert.Equal(t, "@@ diff @@", sess.PlanRevisions[1].Diff)
		assert.True(t, sess.PlanRevisions[1].IsAlteration)
		assert.False(t, sess.PlanRevisions[2].IsAlteration)
		assert.Equal(t, 1, sess.Counters.PlanAlterations)
		assert.Equal(t, "# latest", sess.PlanContent)
	})

	// replayed-equal-content-no-phantom-revision
	t.Run("replayed-equal-content-no-phantom-revision", func(t *testing.T) {
		dir := state.NewDir(t.TempDir())
		version := &state.PlanVersion{Content: "# X", Index: 0}
		require.NoError(t, dir.WritePlanVersion("claude", "s1", version))
		require.NoError(t, dir.WritePlanLatest("claude", "# X", "s1"))

		s := NewStore(10, events.NewBroker())
		s.StateDir = dir
		sess := s.getOrCreate("s1", AgentClaude)
		s.setPlanContent("# X", sess, time.Time{})

		assert.Len(t, sess.PlanRevisions, 1, "replayed identical content produces no phantom revision")
	})
}
