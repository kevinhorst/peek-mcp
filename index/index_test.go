package index

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/kevinhorst/peek-mcp/watcher"
)

const fixtureSessionId = "0e5e1d7a-0000-4000-8000-000000000001"

// transcriptFixture is one session tree on disk: a parent transcript invoking a skill, one subagent transcript and its meta.
type transcriptFixture struct {
	parentPath  string
	projectsDir string
	startedAt   time.Time
}

func provideTranscriptFixture(t *testing.T, startedAt time.Time) *transcriptFixture {
	projectsDir := t.TempDir()
	projectDir := filepath.Join(projectsDir, "-Users-me-project")
	subagentsDir := filepath.Join(projectDir, fixtureSessionId, "subagents")
	require.NoError(t, os.MkdirAll(subagentsDir, 0o700))

	at := func(minutes int) string {
		return startedAt.Add(time.Duration(minutes) * time.Minute).UTC().Format(time.RFC3339Nano)
	}
	parent := fmt.Sprintf(`{"type":"user","promptId":"p1","sessionId":%q,"timestamp":%q,"cwd":"/project","isSidechain":false,"message":{"role":"user","content":"<command-name>/railroad-review</command-name><command-args>service: intercity</command-args>"}}
{"type":"assistant","requestId":"r1","sessionId":%q,"timestamp":%q,"cwd":"/project","isSidechain":false,"message":{"role":"assistant","model":"claude-opus-5-5","stop_reason":"tool_use","content":[{"type":"text","text":"on it"}],"usage":{"input_tokens":10,"output_tokens":30,"cache_creation_input_tokens":0,"cache_read_input_tokens":2000}}}
`, fixtureSessionId, at(0), fixtureSessionId, at(1))
	subagent := fmt.Sprintf(`{"type":"assistant","agentId":"a1","requestId":"a1-r1","sessionId":%q,"timestamp":%q,"cwd":"/project","isSidechain":true,"message":{"role":"assistant","model":"claude-opus-5-5","stop_reason":"end_turn","content":[{"type":"text","text":"lane done"}],"usage":{"input_tokens":5,"output_tokens":40,"cache_creation_input_tokens":500,"cache_read_input_tokens":0}}}
`, fixtureSessionId, at(20))

	parentPath := filepath.Join(projectDir, fixtureSessionId+".jsonl")
	require.NoError(t, os.WriteFile(parentPath, []byte(parent), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(subagentsDir, "agent-a1.jsonl"), []byte(subagent), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(subagentsDir, "agent-a1.meta.json"), []byte(`{"agentType":"railroad-lane-tests"}`), 0o600))

	fixture := &transcriptFixture{
		parentPath:  parentPath,
		projectsDir: projectsDir,
		startedAt:   startedAt,
	}
	return fixture
}

func provideIndexer(projectsDir string, stateDir *state.Dir, store *session.Store) *Indexer {
	parserByAgent := map[session.Agent]func() watcher.Parser{
		session.AgentClaude: func() watcher.Parser { return claude.NewParser() },
	}
	return NewIndexer(watcher.NewTurnLoader(parserByAgent), projectsDir, stateDir, store)
}

func provideEmptyStore() *session.Store {
	return session.NewStore(10, events.NewBroker(), session.AgentClaude)
}

func TestIndexer_Backfill(t *testing.T) {
	tenDaysAgo := time.Now().Add(-10 * 24 * time.Hour).Truncate(time.Second)

	// tree-recorded
	t.Run("tree-recorded", func(t *testing.T) {
		fixture := provideTranscriptFixture(t, tenDaysAgo)
		indexer := provideIndexer(fixture.projectsDir, state.NewDir(t.TempDir()), provideEmptyStore())

		indexer.Backfill(t.Context())

		records := indexer.Records()
		require.Len(t, records, 1)
		require.Len(t, records[0].Runs, 1)
		run := records[0].Runs[0]
		assert.Equal(t, "railroad-review", run.Skill)
		assert.Equal(t, "service: intercity", run.Args)
		assert.Equal(t, 30, run.Root.Usage.OutputTokens)
		assert.Equal(t, 2010, run.Root.PeakContext)
		require.Len(t, run.Agents, 1)
		assert.Equal(t, "railroad-lane-tests", run.Agents[0].AgentType)
		assert.Equal(t, 40, run.Agents[0].Usage.OutputTokens)
		assert.Equal(t, 500+5, run.Agents[0].PeakContext)
	})

	// current-record-kept-changed-transcript-rebuilt
	t.Run("current-record-kept-changed-transcript-rebuilt", func(t *testing.T) {
		fixture := provideTranscriptFixture(t, tenDaysAgo)
		stateDir := state.NewDir(t.TempDir())
		indexer := provideIndexer(fixture.projectsDir, stateDir, provideEmptyStore())
		indexer.Backfill(t.Context())

		marked := indexer.Records()[0]
		marked.Title = "marker"
		indexer.write(marked)
		indexer.Backfill(t.Context())
		assert.Equal(t, "marker", indexer.Records()[0].Title, "an unchanged transcript is never parsed again")

		file, err := os.OpenFile(fixture.parentPath, os.O_APPEND|os.O_WRONLY, 0o600)
		require.NoError(t, err)
		_, err = file.WriteString(fmt.Sprintf(`{"type":"custom-title","customTitle":"Renamed","sessionId":%q}`+"\n", fixtureSessionId))
		require.NoError(t, err)
		require.NoError(t, file.Close())
		indexer.Backfill(t.Context())
		assert.NotEqual(t, "marker", indexer.Records()[0].Title, "a grown transcript is rebuilt")
	})

	// outside-window-skipped
	t.Run("outside-window-skipped", func(t *testing.T) {
		twentyDaysAgo := time.Now().Add(-20 * 24 * time.Hour)
		fixture := provideTranscriptFixture(t, twentyDaysAgo)
		require.NoError(t, os.Chtimes(fixture.parentPath, twentyDaysAgo, twentyDaysAgo))
		indexer := provideIndexer(fixture.projectsDir, state.NewDir(t.TempDir()), provideEmptyStore())

		indexer.Backfill(t.Context())

		assert.Empty(t, indexer.Records())
	})

	// held-lock-skips
	t.Run("held-lock-skips", func(t *testing.T) {
		fixture := provideTranscriptFixture(t, tenDaysAgo)
		stateDir := state.NewDir(t.TempDir())
		release, isAcquired := stateDir.AcquireIndexLock(time.Hour)
		require.True(t, isAcquired)
		defer release()
		indexer := provideIndexer(fixture.projectsDir, stateDir, provideEmptyStore())

		indexer.Backfill(t.Context())

		assert.Empty(t, indexer.Records(), "another instance holds the backfill")
	})

	// live-session-left-to-persist
	t.Run("live-session-left-to-persist", func(t *testing.T) {
		fixture := provideTranscriptFixture(t, tenDaysAgo)
		store := provideEmptyStore()
		store.AddTurnBySessionId(fixtureSessionId, session.AgentClaude, &session.Turn{
			Meta:      &session.Meta{SessionId: fixtureSessionId},
			Role:      session.RoleUser,
			Text:      "live",
			Timestamp: time.Now(),
		})
		indexer := provideIndexer(fixture.projectsDir, state.NewDir(t.TempDir()), store)

		indexer.Backfill(t.Context())
		assert.Empty(t, indexer.Records(), "the backfill never parses a session the store holds")

		indexer.PersistLive()
		records := indexer.Records()
		require.Len(t, records, 1)
		assert.Empty(t, records[0].Runs, "the live store's view wins")
	})
}

func TestRunsOf(t *testing.T) {
	now := time.Now()
	run := func(skill string, age time.Duration) *Run {
		return &Run{Root: &Root{}, Skill: skill, StartedAt: now.Add(-age)}
	}
	records := []*Record{
		{SessionId: "a", Runs: []*Run{run("railroad-review", 2*time.Hour), run("qq", time.Hour)}},
		{SessionId: "b", Runs: []*Run{run("plugin:railroad-review", time.Hour)}},
		{SessionId: "c", Runs: []*Run{run("railroad-review", 15*24*time.Hour)}},
	}

	runs := RunsOf(records, "railroad-review", now)

	require.Len(t, runs, 2, "an older run and another skill are left out")
	assert.Equal(t, session.Id("b"), runs[0].Record.SessionId, "newest first, a namespaced invocation included")
	assert.Equal(t, session.Id("a"), runs[1].Record.SessionId)
}
