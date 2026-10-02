package session

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var snapshotFixtureStart = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

// decodedSnapshot encodes the store once and decodes it once, sessions sorted by id.
func decodedSnapshot(t *testing.T, store *Store) *StoreSnapshot {
	var buffer bytes.Buffer
	require.NoError(t, store.WriteSnapshot(nil, &buffer))

	snapshot, err := ReadStoreSnapshot(&buffer)
	require.NoError(t, err)

	slices.SortFunc(snapshot.Sessions, func(left, right *sessionSnapshot) int {
		return cmp.Compare(left.Session.Meta.SessionId, right.Session.Meta.SessionId)
	})
	return snapshot
}

func ingestTurns(store *Store, turns []*Turn) {
	for _, turn := range turns {
		store.AddTurnBySessionId(turn.Meta.SessionId, AgentClaude, turn)
	}
}

func provideSnapshotStore() *Store {
	store := NewStore(10, 25, events.NewBroker())
	ingestTurns(store, provideSnapshotTurns())

	store.AddTurnBySessionId("c", AgentCodex, &Turn{
		CustomTitle: "Codex fixture",
		Meta:        &Meta{SessionId: "c"},
		TitleSource: TitleSourceCustom,
	})
	store.AddTurnBySessionId("c", AgentCodex, &Turn{
		Meta:  &Meta{SessionId: "c"},
		Usage: &Usage{InputTokens: 50, TotalTokens: 50},
	})
	store.AddTurnBySessionId("c", AgentCodex, &Turn{
		Meta:      &Meta{SessionId: "c"},
		Role:      RoleUser,
		Text:      "codex prompt",
		Timestamp: snapshotFixtureStart.Add(15 * time.Minute),
	})
	return store
}

// provideSnapshotTurns returns a fresh Claude turn sequence over sessions "p" and "q": tool calls and
// results, main and subagent skill windows, repeated request ids, a denial, plan revisions and a model change.
func provideSnapshotTurns() []*Turn {
	at := func(minutes int) time.Time {
		return snapshotFixtureStart.Add(time.Duration(minutes) * time.Minute)
	}

	turns := make([]*Turn, 0)
	turns = append(turns, &Turn{
		CustomTitle: "Snapshot fixture",
		Meta:        &Meta{SessionId: "p"},
		TitleSource: TitleSourceCustom,
	})
	turns = append(turns, &Turn{
		Meta:      &Meta{CWD: "/project", SessionId: "p"},
		PromptId:  "prompt-1",
		Role:      RoleUser,
		Text:      "start",
		Timestamp: at(0),
	})
	turns = append(turns, &Turn{
		Meta:       &Meta{Model: "claude-opus", SessionId: "p"},
		RequestId:  "r1",
		Role:       RoleAssistant,
		StopReason: StopReasonToolUse,
		Text:       "on it",
		Timestamp:  at(1),
		ToolCalls:  []*ToolCall{{Id: "tu1", Input: []byte(`{"command":"ls"}`), Name: "Bash", Timestamp: at(1)}},
		Usage:      &Usage{InputTokens: 10, OutputTokens: 5},
	})
	turns = append(turns, &Turn{
		FileTouches: []*FileTouch{{Path: "/project/a.go"}},
		Meta:        &Meta{SessionId: "p"},
		ToolResults: []*ToolResult{{IsError: true, ToolUseId: "tu1"}},
	})
	turns = append(turns, &Turn{
		Events:    []*Event{{Kind: EventKindSkillInvoked, Skill: &SkillPayload{Skill: "jq", Source: SkillSourceSlash}, Timestamp: at(2)}},
		Meta:      &Meta{SessionId: "p"},
		PromptId:  "prompt-2",
		Role:      RoleUser,
		Text:      "/jq",
		Timestamp: at(2),
	})
	turns = append(turns, &Turn{
		Meta:      &Meta{Model: "claude-opus", SessionId: "p"},
		RequestId: "r2",
		Role:      RoleAssistant,
		Text:      "jq chunk",
		Timestamp: at(3),
		Usage:     &Usage{InputTokens: 20, OutputTokens: 1},
	})
	turns = append(turns, &Turn{
		Meta:      &Meta{Model: "claude-opus", SessionId: "p"},
		RequestId: "r2",
		Role:      RoleAssistant,
		Text:      " continued",
		Timestamp: at(3),
		Usage:     &Usage{InputTokens: 20, OutputTokens: 7},
	})
	turns = append(turns, &Turn{
		Events:     []*Event{{Actor: "sub-1", Kind: EventKindSubagentSpawned, Subagent: &SubagentPayload{AgentId: "sub-1", AgentType: "Explore", ToolUseId: "tu2"}, Timestamp: at(4)}},
		Meta:       &Meta{SessionId: "p"},
		SubagentId: "sub-1",
		Timestamp:  at(4),
	})
	turns = append(turns, &Turn{
		Events:     []*Event{{Actor: "sub-1", Kind: EventKindSkillInvoked, Skill: &SkillPayload{Skill: "lint", Source: SkillSourceTool}, Timestamp: at(5)}},
		Meta:       &Meta{Model: "claude-haiku", SessionId: "p"},
		RequestId:  "sr1",
		Role:       RoleAssistant,
		SubagentId: "sub-1",
		Text:       "lint",
		Timestamp:  at(5),
		ToolCalls:  []*ToolCall{{Id: "tu3", Name: "Read", Timestamp: at(5)}},
		Usage:      &Usage{InputTokens: 4, OutputTokens: 2},
	})
	turns = append(turns, &Turn{
		FileTouches: []*FileTouch{{Path: "/project/b.go", Write: true}},
		Meta:        &Meta{Model: "claude-haiku", SessionId: "p"},
		RequestId:   "sr1",
		Role:        RoleAssistant,
		SubagentId:  "sub-1",
		Text:        " more",
		Timestamp:   at(5),
		Usage:       &Usage{InputTokens: 4, OutputTokens: 6},
	})
	turns = append(turns, &Turn{
		Meta:        &Meta{SessionId: "p"},
		Role:        RoleUser,
		SubagentId:  "sub-1",
		Timestamp:   at(6),
		ToolResults: []*ToolResult{{ToolUseId: "tu3"}},
	})
	turns = append(turns, &Turn{
		Events: []*Event{{Kind: EventKindSubagentResult, Subagent: &SubagentPayload{Content: "done", ToolUseId: "tu2"}, Timestamp: at(7)}},
		Meta:   &Meta{SessionId: "p"},
	})
	turns = append(turns, &Turn{
		Events: []*Event{{Kind: EventKindPermissionDenied, Permission: &PermissionPayload{Kind: DenialKindUserRejected, Tool: "Bash", ToolUseId: "tu4"}, Timestamp: at(7)}},
		Meta:   &Meta{SessionId: "p"},
	})
	turns = append(turns, &Turn{
		Meta:         &Meta{SessionId: "p"},
		PlanContent:  "# Plan v1",
		PlanFilePath: "/plans/p.md",
		Timestamp:    at(8),
	})
	turns = append(turns, &Turn{
		Events: []*Event{{Kind: EventKindPlanModeExit, Timestamp: at(9)}},
		Meta:   &Meta{SessionId: "p"},
	})
	turns = append(turns, &Turn{
		Meta:         &Meta{SessionId: "p"},
		PlanContent:  "# Plan v2",
		PlanFilePath: "/plans/p.md",
		Timestamp:    at(10),
	})
	turns = append(turns, &Turn{
		Meta:       &Meta{Model: "claude-fable", SessionId: "p"},
		RequestId:  "r3",
		Role:       RoleAssistant,
		StopReason: "end_turn",
		Text:       "switched",
		Timestamp:  at(11),
		Usage:      &Usage{InputTokens: 3, OutputTokens: 3},
	})
	turns = append(turns, &Turn{
		Meta:      &Meta{SessionId: "q"},
		PromptId:  "q-1",
		Role:      RoleUser,
		Text:      "other session",
		Timestamp: at(12),
	})
	turns = append(turns, &Turn{
		Events:    []*Event{{Kind: EventKindSkillInvoked, Skill: &SkillPayload{Skill: "review", Source: SkillSourceSlash}, Timestamp: at(13)}},
		Meta:      &Meta{SessionId: "p"},
		PromptId:  "prompt-3",
		Role:      RoleUser,
		Text:      "/review",
		Timestamp: at(13),
	})
	turns = append(turns, &Turn{
		Meta:      &Meta{Model: "claude-fable", SessionId: "p"},
		RequestId: "r4",
		Role:      RoleAssistant,
		Text:      "reviewing",
		Timestamp: at(14),
		Usage:     &Usage{InputTokens: 8, OutputTokens: 4},
	})
	return turns
}

func restoredStore(t *testing.T, store *Store) *Store {
	var buffer bytes.Buffer
	require.NoError(t, store.WriteSnapshot(nil, &buffer))

	snapshot, err := ReadStoreSnapshot(&buffer)
	require.NoError(t, err)

	restored := NewStore(10, 25, events.NewBroker())
	restored.Restore(snapshot)
	return restored
}

func TestStoreSnapshot_RoundTrip(t *testing.T) {
	// sessions-equal-after-round-trip
	t.Run("sessions-equal-after-round-trip", func(t *testing.T) {
		original := provideSnapshotStore()
		expected := decodedSnapshot(t, original)
		require.Len(t, expected.Sessions, 3)

		restored := restoredStore(t, original)
		assert.Equal(t, expected, decodedSnapshot(t, restored))

		session, err := restored.GetByTitle("snapshot fixture", AgentClaude)
		require.NoError(t, err)
		assert.Equal(t, Id("p"), session.Meta.SessionId)

		files := map[string]FileState{"/project/p.jsonl": {Offset: 42, Parser: []byte("pending")}}
		var buffer bytes.Buffer
		require.NoError(t, original.WriteSnapshot(files, &buffer))
		snapshot, err := ReadStoreSnapshot(&buffer)
		require.NoError(t, err)
		assert.Equal(t, files, snapshot.Files)
	})

	// active-skill-relinked
	t.Run("active-skill-relinked", func(t *testing.T) {
		restored := restoredStore(t, provideSnapshotStore())

		session, ok := restored.GetById("p")
		require.True(t, ok)
		require.Len(t, session.Skills, 3)
		assert.Same(t, session.Skills[2], session.activeSkills[""])
		assert.Same(t, session.Skills[1], session.activeSkills["sub-1"])
		assert.Len(t, session.activeSkills, 2)
	})

	// request-usage-relinked
	t.Run("request-usage-relinked", func(t *testing.T) {
		repeatedChunk := func() *Turn {
			return &Turn{
				Meta:      &Meta{Model: "claude-fable", SessionId: "p"},
				RequestId: "r4",
				Role:      RoleAssistant,
				Text:      " more",
				Timestamp: snapshotFixtureStart.Add(14 * time.Minute),
				Usage:     &Usage{InputTokens: 8, OutputTokens: 9},
			}
		}
		original := provideSnapshotStore()
		restored := restoredStore(t, original)
		original.AddTurnBySessionId("p", AgentClaude, repeatedChunk())
		restored.AddTurnBySessionId("p", AgentClaude, repeatedChunk())

		originalSession, ok := original.GetById("p")
		require.True(t, ok)
		restoredSession, ok := restored.GetById("p")
		require.True(t, ok)
		assert.Equal(t, originalSession.TotalUsage, restoredSession.TotalUsage)
		assert.Equal(t, originalSession.Skills[2].Usage, restoredSession.Skills[2].Usage)
		assert.Equal(t, 9, restoredSession.Skills[2].Usage.OutputTokens)
	})

	// subagent-usage-relinked
	t.Run("subagent-usage-relinked", func(t *testing.T) {
		repeatedChunk := func() *Turn {
			return &Turn{
				Meta:       &Meta{Model: "claude-haiku", SessionId: "p"},
				RequestId:  "sr1",
				Role:       RoleAssistant,
				SubagentId: "sub-1",
				Text:       " again",
				Timestamp:  snapshotFixtureStart.Add(6 * time.Minute),
				Usage:      &Usage{InputTokens: 4, OutputTokens: 11},
			}
		}
		original := provideSnapshotStore()
		restored := restoredStore(t, original)
		original.AddTurnBySessionId("p", AgentClaude, repeatedChunk())
		restored.AddTurnBySessionId("p", AgentClaude, repeatedChunk())

		originalSession, ok := original.GetById("p")
		require.True(t, ok)
		restoredSession, ok := restored.GetById("p")
		require.True(t, ok)
		assert.Equal(t, originalSession.Subagents["sub-1"].Usage, restoredSession.Subagents["sub-1"].Usage)
		assert.Equal(t, originalSession.Skills[1].Usage, restoredSession.Skills[1].Usage)
		assert.Equal(t, 11, restoredSession.Subagents["sub-1"].Usage.OutputTokens)
	})

	// buffers-keep-capacity-and-counters
	t.Run("buffers-keep-capacity-and-counters", func(t *testing.T) {
		original := provideSnapshotStore()
		restored := restoredStore(t, original)

		originalSession, ok := original.GetById("p")
		require.True(t, ok)
		restoredSession, ok := restored.GetById("p")
		require.True(t, ok)

		assert.Equal(t, 10, restoredSession.TurnsFinished.capacity)
		assert.Equal(t, originalSession.TurnsFinished.pushed, restoredSession.TurnsFinished.pushed)
		assert.Equal(t, originalSession.TurnsFinished.pushedToolOnly, restoredSession.TurnsFinished.pushedToolOnly)
		assert.Equal(t, originalSession.TurnsFinished.Len(), restoredSession.TurnsFinished.Len())

		assert.Equal(t, EventBufferCapacity, restoredSession.Events.capacity)
		assert.Equal(t, EventBufferDenialBudget, restoredSession.Events.denialBudget)
		assert.Equal(t, 1, restoredSession.Events.denials)
		assert.Equal(t, originalSession.Events.Len(), restoredSession.Events.Len())

		originalTurns := originalSession.Subagents["sub-1"].Turns
		restoredTurns := restoredSession.Subagents["sub-1"].Turns
		assert.Equal(t, minSubagentTurnDepth, restoredTurns.capacity)
		assert.Equal(t, originalTurns.PushedWithToolCalls(), restoredTurns.PushedWithToolCalls())
		assert.Equal(t, originalSession.TotalTurns(), restoredSession.TotalTurns())
	})

	// zero-payload-event-restored
	t.Run("zero-payload-event-restored", func(t *testing.T) {
		original := NewStore(10, 25, events.NewBroker())
		original.AddTurnBySessionId("z", AgentClaude, &Turn{
			Events: []*Event{{Kind: EventKindModelChanged, Model: &ModelPayload{}, Timestamp: snapshotFixtureStart}},
			Meta:   &Meta{SessionId: "z"},
		})
		restored := restoredStore(t, original)

		session, ok := restored.GetById("z")
		require.True(t, ok)
		all := session.Events.All()
		require.Len(t, all, 1)
		assert.Equal(t, EventKindModelChanged, all[0].Kind)
		assert.Equal(t, snapshotFixtureStart, all[0].Timestamp)
		assert.Equal(t, &ModelPayload{}, all[0].Model)
		assert.Equal(t, 1, session.Counters.ModelChanges)
	})
}

func TestStoreSnapshot_SplitIngest(t *testing.T) {
	type testCase struct {
		_id       string
		_expected *StoreSnapshot
		store     *Store
	}

	full := NewStore(10, 25, events.NewBroker())
	ingestTurns(full, provideSnapshotTurns())
	expected := decodedSnapshot(t, full)

	tests := make([]*testCase, 0)

	// split-at-<k>: ingest k turns, snapshot, restore into a fresh store, ingest the rest
	turnCount := len(provideSnapshotTurns())
	for splitIndex := 0; splitIndex <= turnCount; splitIndex++ {
		turns := provideSnapshotTurns()
		before := NewStore(10, 25, events.NewBroker())
		ingestTurns(before, turns[:splitIndex])
		after := restoredStore(t, before)
		ingestTurns(after, turns[splitIndex:])

		tests = append(tests, &testCase{
			_id:       fmt.Sprintf("split-at-%d", splitIndex),
			_expected: expected,
			store:     after,
		})
	}

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			assert.Equal(t, test._expected, decodedSnapshot(t, test.store))
		})
	}
}
