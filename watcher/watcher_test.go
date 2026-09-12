package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kevinhorst/peek-mcp/codex"
	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	defer file.Close()

	_, err = file.WriteString(line + "\n")
	require.NoError(t, err)
}

func TestWalkAndWatch_Horizon(t *testing.T) {
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "rollout-old.jsonl")
	freshFile := filepath.Join(dir, "rollout-fresh.jsonl")
	appendLine(t, oldFile, `{"timestamp":"2026-07-11T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-old","cwd":"/project"}}`)
	appendLine(t, freshFile, `{"timestamp":"2026-08-30T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-fresh","cwd":"/project"}}`)
	stale := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(oldFile, stale, stale))

	store := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	newParser := func() Parser { return codex.NewParser() }
	w := New(session.AgentCodex, dir, 24*time.Hour, newParser, store)
	fsWatcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer fsWatcher.Close()

	// old-file-skipped-at-startup
	w.walkAndWatch(fsWatcher, dir)
	_, ok := store.GetById("sess-old")
	assert.False(t, ok)
	_, ok = store.GetById("sess-fresh")
	assert.True(t, ok)

	// write-event-ingests-skipped-file-fully
	appendLine(t, oldFile, `{"timestamp":"2026-08-30T21:00:00.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"resumed"}]}}`)
	require.NoError(t, w.readNewLines(oldFile))
	resumed, ok := store.GetById("sess-old")
	assert.True(t, ok)
	assert.Len(t, resumed.Turns(10), 1)

	// zero-horizon-ingests-everything
	allStore := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	allWatcher := New(session.AgentCodex, dir, 0, newParser, allStore)
	allWatcher.walkAndWatch(fsWatcher, dir)
	_, ok = allStore.GetById("sess-old")
	assert.True(t, ok)
}

func TestEventBatch_Coalesce(t *testing.T) {
	batch := newEventBatch()

	// five-writes-same-path-one-dirty-entry
	for range 5 {
		batch.add(fsnotify.Event{Name: "/home/a.jsonl", Op: fsnotify.Write})
	}
	assert.Len(t, batch.dirty, 1)

	// writes-on-two-paths-two-entries
	batch.add(fsnotify.Event{Name: "/home/b.jsonl", Op: fsnotify.Write})
	assert.Len(t, batch.dirty, 2)

	// create-goes-to-created-not-dirty
	batch.add(fsnotify.Event{Name: "/home/newdir", Op: fsnotify.Create})
	assert.Equal(t, []string{"/home/newdir"}, batch.created)
	assert.Len(t, batch.dirty, 2)

	// create-and-write-on-one-event-created-only
	batch.add(fsnotify.Event{Name: "/home/c.jsonl", Op: fsnotify.Create | fsnotify.Write})
	assert.Equal(t, []string{"/home/newdir", "/home/c.jsonl"}, batch.created)
	assert.Len(t, batch.dirty, 2)

	// chmod-only-event-dropped
	batch.add(fsnotify.Event{Name: "/home/d.jsonl", Op: fsnotify.Chmod})
	assert.Len(t, batch.created, 2)
	assert.Len(t, batch.dirty, 2)

	// drain-consumes-pending-events
	events := make(chan fsnotify.Event, 4)
	events <- fsnotify.Event{Name: "/home/a.jsonl", Op: fsnotify.Write}
	events <- fsnotify.Event{Name: "/home/e.jsonl", Op: fsnotify.Write}
	batch.drain(events)
	assert.Len(t, batch.dirty, 3)
	assert.Empty(t, events)
}

func TestProcessBatch(t *testing.T) {
	dir := t.TempDir()
	store := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	newParser := func() Parser { return codex.NewParser() }
	w := New(session.AgentCodex, dir, 0, newParser, store)
	fsWatcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer fsWatcher.Close()

	// created-dir-with-existing-transcript-is-walked
	subDir := filepath.Join(dir, "sessions")
	require.NoError(t, os.Mkdir(subDir, 0o755))
	walked := filepath.Join(subDir, "rollout-walked.jsonl")
	appendLine(t, walked, `{"timestamp":"2026-07-11T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-walked","cwd":"/project"}}`)
	batch := newEventBatch()
	batch.add(fsnotify.Event{Name: subDir, Op: fsnotify.Create})
	w.processBatch(fsWatcher, batch)
	_, ok := store.GetById("sess-walked")
	assert.True(t, ok)

	// created-file-plus-queued-writes-ingested-once
	created := filepath.Join(dir, "rollout-created.jsonl")
	appendLine(t, created, `{"timestamp":"2026-07-11T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-created","cwd":"/project"}}`)
	appendLine(t, created, `{"timestamp":"2026-07-11T20:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"only once"}]}}`)
	batch = newEventBatch()
	batch.add(fsnotify.Event{Name: created, Op: fsnotify.Create})
	batch.add(fsnotify.Event{Name: created, Op: fsnotify.Write})
	batch.add(fsnotify.Event{Name: created, Op: fsnotify.Write})
	w.processBatch(fsWatcher, batch)
	sess, ok := store.GetById("sess-created")
	require.True(t, ok)
	assert.Len(t, sess.Turns(10), 1)

	// meta-path-in-dirty-set-one-spawned-event
	claudeStore := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
	parent := &session.Turn{
		Role:      session.RoleUser,
		Text:      "start",
		Timestamp: time.Now(),
		Meta:      &session.Meta{SessionId: "parent-sess"},
	}
	claudeStore.AddTurnBySessionId("parent-sess", session.AgentClaude, parent)
	cw := claudeWatcher(dir, claudeStore)
	metaPath := writeSubagentMeta(t, dir, "parent-sess", "sub1",
		`{"agentType":"explore","description":"survey","toolUseId":"tu1","spawnDepth":1}`)
	batch = newEventBatch()
	batch.add(fsnotify.Event{Name: metaPath, Op: fsnotify.Write})
	batch.add(fsnotify.Event{Name: metaPath, Op: fsnotify.Write})
	cw.processBatch(fsWatcher, batch)
	parentSess, ok := claudeStore.GetById("parent-sess")
	require.True(t, ok)
	spawned := parentSess.Events.All()
	require.Len(t, spawned, 1)
	assert.Equal(t, session.EventKindSubagentSpawned, spawned[0].Kind)
}

func TestReadNewLines_PerFileParserState(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "rollout-a.jsonl")
	fileB := filepath.Join(dir, "rollout-b.jsonl")

	store := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	newParser := func() Parser { return codex.NewParser() }
	w := New(session.AgentCodex, dir, 0, newParser, store)

	// session A starts and produces a turn
	appendLine(t, fileA, `{"timestamp":"2026-07-11T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-a","cwd":"/project"}}`)
	appendLine(t, fileA, `{"timestamp":"2026-07-11T20:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"first message in A"}]}}`)
	require.NoError(t, w.readNewLines(fileA))

	// session B starts before A is finished
	appendLine(t, fileB, `{"timestamp":"2026-07-11T20:00:02.000Z","type":"session_meta","payload":{"id":"sess-b","cwd":"/project"}}`)
	require.NoError(t, w.readNewLines(fileB))

	// A continues — its turns must stay on session A
	appendLine(t, fileA, `{"timestamp":"2026-07-11T20:00:03.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"second message in A"}]}}`)
	require.NoError(t, w.readNewLines(fileA))

	sessionA, ok := store.GetById("sess-a")
	assert.True(t, ok)
	turns := sessionA.Turns(10)
	assert.Len(t, turns, 2)
	assert.Equal(t, "second message in A", turns[1].Text)

	// B holds only its own meta-only turn — no text turn from file A leaked in
	sessionB, ok := store.GetById("sess-b")
	assert.True(t, ok)
	for _, turn := range sessionB.Turns(10) {
		assert.Empty(t, turn.Text)
	}
}
