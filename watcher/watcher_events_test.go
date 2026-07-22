package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func claudeWatcher(dir string, store *session.Store) *Watcher {
	newParser := func() Parser { return claude.NewParser() }
	return New(session.AgentClaude, dir, newParser, store)
}

func writeSubagentMeta(t *testing.T, projectDir, parentId, agentId, body string) string {
	t.Helper()
	subagentsDir := filepath.Join(projectDir, parentId, subagentsDirName)
	require.NoError(t, os.MkdirAll(subagentsDir, 0o755))
	path := filepath.Join(subagentsDir, agentFilePrefix+agentId+metaJsonSuffix)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestReadSubagentMeta(t *testing.T) {
	// spawned-event-on-parent
	t.Run("spawned-event-on-parent", func(t *testing.T) {
		projectDir := t.TempDir()
		store := session.NewStore(10, session.AgentClaude)
		turn := &session.Turn{
			Role:      session.RoleUser,
			Text:      "start",
			Timestamp: time.Now(),
			Meta:      &session.Meta{SessionId: "parent-sess"},
		}
		store.AddTurnBySessionId("parent-sess", session.AgentClaude, turn)
		w := claudeWatcher(projectDir, store)

		path := writeSubagentMeta(t, projectDir, "parent-sess", "sub1",
			`{"agentType":"explore","description":"survey","toolUseId":"tu1","spawnDepth":1}`)
		w.readSubagentMeta(path)

		sess, ok := store.GetById("parent-sess")
		require.True(t, ok)
		events := sess.Events.All()
		require.Len(t, events, 1)
		assert.Equal(t, session.EventKindSubagentSpawned, events[0].Kind)
		assert.Equal(t, "sub1", events[0].Subagent.AgentId)
		assert.Equal(t, "explore", events[0].Subagent.AgentType)
		assert.Equal(t, "survey", events[0].Subagent.Description)
	})

	// duplicate-read-no-second-event
	t.Run("duplicate-read-no-second-event", func(t *testing.T) {
		projectDir := t.TempDir()
		store := session.NewStore(10, session.AgentClaude)
		turn := &session.Turn{
			Role:      session.RoleUser,
			Text:      "start",
			Timestamp: time.Now(),
			Meta:      &session.Meta{SessionId: "parent-sess"},
		}
		store.AddTurnBySessionId("parent-sess", session.AgentClaude, turn)
		w := claudeWatcher(projectDir, store)

		path := writeSubagentMeta(t, projectDir, "parent-sess", "sub1", `{"agentType":"explore"}`)
		w.readSubagentMeta(path)
		w.readSubagentMeta(path)

		sess, _ := store.GetById("parent-sess")
		assert.Len(t, sess.Events.All(), 1)
	})

	// non-meta-json-ignored
	t.Run("non-meta-json-ignored", func(t *testing.T) {
		assert.False(t, isSubagentMetaPath("/x/y/rollout.jsonl"))
		assert.False(t, isSubagentMetaPath("/x/y/agent-1.meta.json"), "not under a subagents dir")
		assert.True(t, isSubagentMetaPath("/x/sess/subagents/agent-1.meta.json"))
	})
}

func TestWalkAndWatch_ColdBackfillSubagents(t *testing.T) {
	projectDir := t.TempDir()
	store := session.NewStore(10, session.AgentClaude)
	w := claudeWatcher(projectDir, store)

	// On-disk layout as left behind by a finished session: the subagent dir
	// sorts lexically before the parent transcript file
	transcript := filepath.Join(projectDir, "parent-sess.jsonl")
	line := `{"type":"user","promptId":"p1","sessionId":"parent-sess","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"user","content":"hello"}}` + "\n"
	require.NoError(t, os.WriteFile(transcript, []byte(line), 0o644))
	writeSubagentMeta(t, projectDir, "parent-sess", "sub1",
		`{"agentType":"explore","description":"survey","toolUseId":"tu1","spawnDepth":1}`)

	fsWatcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer fsWatcher.Close()

	w.walkAndWatch(fsWatcher, projectDir)

	sess, ok := store.GetById("parent-sess")
	require.True(t, ok, "parent transcript must be backfilled")
	events := sess.Events.All()
	require.Len(t, events, 1, "spawned event must land on the parent, not be dropped")
	assert.Equal(t, session.EventKindSubagentSpawned, events[0].Kind)
	assert.Equal(t, "sub1", events[0].Subagent.AgentId)
}

func TestWalkAndWatch_NewDirBackfill(t *testing.T) {
	root := t.TempDir()
	store := session.NewStore(10, session.AgentClaude)
	w := claudeWatcher(root, store)

	// A dir already populated with a transcript before we start watching it
	sessionDir := filepath.Join(root, "sess-x")
	require.NoError(t, os.MkdirAll(sessionDir, 0o755))
	transcript := filepath.Join(sessionDir, "chat.jsonl")
	require.NoError(t, os.WriteFile(transcript, []byte(`{"type":"user","promptId":"p1","sessionId":"sess-x","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"user","content":"hello"}}`+"\n"), 0o644))

	fsWatcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer fsWatcher.Close()

	w.walkAndWatch(fsWatcher, sessionDir)

	sess, ok := store.GetById("sess-x")
	require.True(t, ok, "files present at dir creation must be backfilled")
	assert.Len(t, sess.Turns(10), 1)
}
