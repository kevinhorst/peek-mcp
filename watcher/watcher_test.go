package watcher

import (
	"bytes"
	"context"
	"log/slog"
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

func runUntilLoaded(t *testing.T, run func(context.Context) error, loaded <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	select {
	case <-loaded:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Loaded not signaled within 5s")
	}
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

func TestWalkAndWatch_WatchPruning(t *testing.T) {
	dir := t.TempDir()
	coldDir := filepath.Join(dir, "cold")
	require.NoError(t, os.Mkdir(coldDir, 0o755))
	coldFile := filepath.Join(coldDir, "rollout-cold.jsonl")
	appendLine(t, coldFile, `{"timestamp":"2026-07-11T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-cold","cwd":"/project"}}`)
	stale := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(coldFile, stale, stale))
	require.NoError(t, os.Chtimes(coldDir, stale, stale))

	hotDir := filepath.Join(dir, "hot")
	require.NoError(t, os.Mkdir(hotDir, 0o755))
	hotFile := filepath.Join(hotDir, "rollout-hot.jsonl")
	appendLine(t, hotFile, `{"timestamp":"2026-08-30T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-hot","cwd":"/project"}}`)

	deepDir := filepath.Join(dir, "deep", "subagents", "nested")
	require.NoError(t, os.MkdirAll(deepDir, 0o755))
	deepFile := filepath.Join(deepDir, "rollout-deep.jsonl")
	appendLine(t, deepFile, `{"timestamp":"2026-08-30T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-deep","cwd":"/project"}}`)

	store := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	newParser := func() Parser { return codex.NewParser() }
	w := New(session.AgentCodex, dir, 24*time.Hour, newParser, store)
	fsWatcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer fsWatcher.Close()

	w.walkAndWatch(fsWatcher, dir)
	watched := fsWatcher.WatchList()

	// cold-dir-not-watched
	assert.NotContains(t, watched, coldDir)

	// cold-dir-not-ingested
	_, ok := store.GetById("sess-cold")
	assert.False(t, ok)

	// deep-hot-file-keeps-ancestor-chain-watched
	assert.Contains(t, watched, filepath.Join(dir, "deep"))
	assert.Contains(t, watched, filepath.Join(dir, "deep", "subagents"))
	assert.Contains(t, watched, deepDir)

	// root-always-watched-even-when-cold
	assert.Contains(t, watched, dir)

	// new-file-in-cold-dir-then-rescan
	appendLine(t, coldFile, `{"timestamp":"2026-08-30T21:00:00.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"resumed"}]}}`)
	require.NoError(t, os.Chtimes(coldFile, time.Now(), time.Now()))
	w.walkAndWatch(fsWatcher, dir)
	assert.Contains(t, fsWatcher.WatchList(), coldDir)
	_, ok = store.GetById("sess-cold")
	assert.True(t, ok)

	// zero-horizon-watches-everything
	allStore := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	allWatcher := New(session.AgentCodex, dir, 0, newParser, allStore)
	allFsWatcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer allFsWatcher.Close()
	allWatcher.walkAndWatch(allFsWatcher, dir)
	assert.Contains(t, allFsWatcher.WatchList(), coldDir)
}

func TestWalkAndWatch_ToolResults(t *testing.T) {
	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "project", "sess-1")
	toolResultsDir := filepath.Join(sessionDir, "tool-results")
	require.NoError(t, os.MkdirAll(toolResultsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(toolResultsDir, "toolu_1.txt"), []byte("output"), 0o644))
	subagentsDir := filepath.Join(sessionDir, "subagents")
	require.NoError(t, os.MkdirAll(subagentsDir, 0o755))

	store := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	newParser := func() Parser { return codex.NewParser() }
	w := New(session.AgentCodex, dir, 0, newParser, store)
	fsWatcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer fsWatcher.Close()

	// tool-results-not-watched
	w.walkAndWatch(fsWatcher, dir)
	watched := fsWatcher.WatchList()
	assert.Contains(t, watched, sessionDir)
	assert.NotContains(t, watched, toolResultsDir)

	// tool-results-as-root-not-watched
	w.walkAndWatch(fsWatcher, toolResultsDir)
	assert.NotContains(t, fsWatcher.WatchList(), toolResultsDir)

	// subagents-sibling-still-watched
	assert.Contains(t, fsWatcher.WatchList(), subagentsDir)
}

func TestWalkAndWatch_Expiry(t *testing.T) {
	var logBuffer bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	dir := t.TempDir()
	stale := time.Now().Add(-48 * time.Hour)

	hotDir := filepath.Join(dir, "hot")
	require.NoError(t, os.Mkdir(hotDir, 0o755))
	appendLine(t, filepath.Join(hotDir, "rollout-hot.jsonl"), `{"timestamp":"2026-08-30T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-hot","cwd":"/project"}}`)

	agingDir := filepath.Join(dir, "aging")
	require.NoError(t, os.Mkdir(agingDir, 0o755))
	agingFile := filepath.Join(agingDir, "rollout-aging.jsonl")
	appendLine(t, agingFile, `{"timestamp":"2026-08-30T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-aging","cwd":"/project"}}`)
	appendLine(t, agingFile, `{"timestamp":"2026-08-30T20:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"first"}]}}`)

	neverDir := filepath.Join(dir, "never")
	require.NoError(t, os.Mkdir(neverDir, 0o755))
	neverFile := filepath.Join(neverDir, "rollout-never.jsonl")
	appendLine(t, neverFile, `{"timestamp":"2026-07-11T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-never","cwd":"/project"}}`)
	require.NoError(t, os.Chtimes(neverFile, stale, stale))
	require.NoError(t, os.Chtimes(neverDir, stale, stale))

	store := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	newParser := func() Parser { return codex.NewParser() }
	w := New(session.AgentCodex, dir, 24*time.Hour, newParser, store)
	fsWatcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer fsWatcher.Close()

	w.walkAndWatch(fsWatcher, dir)
	require.Contains(t, fsWatcher.WatchList(), agingDir)
	agingSession, ok := store.GetById("sess-aging")
	require.True(t, ok)
	require.Len(t, agingSession.Turns(10), 1)

	// aged-dir-unwatched-on-rescan
	require.NoError(t, os.Chtimes(agingFile, stale, stale))
	require.NoError(t, os.Chtimes(agingDir, stale, stale))
	w.walkAndWatch(fsWatcher, dir)
	watched := fsWatcher.WatchList()
	assert.NotContains(t, watched, agingDir)
	assert.NotContains(t, watched, neverDir)
	assert.Contains(t, watched, hotDir)

	// unwatched-dir-not-readded-as-root
	w.walkAndWatch(fsWatcher, agingDir)
	w.walkAndWatch(fsWatcher, neverDir)
	assert.NotContains(t, fsWatcher.WatchList(), agingDir)
	assert.NotContains(t, fsWatcher.WatchList(), neverDir)
	assert.NotContains(t, logBuffer.String(), "Failed to remove watch")

	// fresh-dir-as-root-watched
	freshDir := filepath.Join(dir, "fresh")
	require.NoError(t, os.Mkdir(freshDir, 0o755))
	appendLine(t, filepath.Join(freshDir, "rollout-fresh.jsonl"), `{"timestamp":"2026-08-30T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-fresh","cwd":"/project"}}`)
	w.walkAndWatch(fsWatcher, freshDir)
	assert.Contains(t, fsWatcher.WatchList(), freshDir)

	// reactivated-dir-resumes-at-offset
	appendLine(t, agingFile, `{"timestamp":"2026-08-30T21:00:00.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"second"}]}}`)
	require.NoError(t, os.Chtimes(agingFile, time.Now(), time.Now()))
	w.walkAndWatch(fsWatcher, dir)
	assert.Contains(t, fsWatcher.WatchList(), agingDir)
	agingSession, ok = store.GetById("sess-aging")
	require.True(t, ok)
	turns := agingSession.Turns(10)
	assert.Len(t, turns, 2)
	assert.Equal(t, "second", turns[len(turns)-1].Text)

	// agent-dir-always-watched
	coldAgentDir := t.TempDir()
	coldSubDir := filepath.Join(coldAgentDir, "cold")
	require.NoError(t, os.Mkdir(coldSubDir, 0o755))
	coldFile := filepath.Join(coldSubDir, "rollout-cold.jsonl")
	appendLine(t, coldFile, `{"timestamp":"2026-07-11T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-cold","cwd":"/project"}}`)
	require.NoError(t, os.Chtimes(coldFile, stale, stale))
	require.NoError(t, os.Chtimes(coldSubDir, stale, stale))
	require.NoError(t, os.Chtimes(coldAgentDir, stale, stale))
	coldStore := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	coldWatcher := New(session.AgentCodex, coldAgentDir, 24*time.Hour, newParser, coldStore)
	coldFsWatcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer coldFsWatcher.Close()
	coldWatcher.walkAndWatch(coldFsWatcher, coldAgentDir)
	coldWatcher.walkAndWatch(coldFsWatcher, coldAgentDir)
	assert.Contains(t, coldFsWatcher.WatchList(), coldAgentDir)
	assert.NotContains(t, coldFsWatcher.WatchList(), coldSubDir)

	// horizon-zero-never-expires
	allStore := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	allWatcher := New(session.AgentCodex, dir, 0, newParser, allStore)
	allFsWatcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	defer allFsWatcher.Close()
	allWatcher.walkAndWatch(allFsWatcher, dir)
	allWatcher.walkAndWatch(allFsWatcher, dir)
	assert.Contains(t, allFsWatcher.WatchList(), neverDir)
}

func TestWalkAndWatch_AddFailure(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sessions")
	require.NoError(t, os.Mkdir(subDir, 0o755))
	transcript := filepath.Join(subDir, "rollout-fresh.jsonl")
	appendLine(t, transcript, `{"timestamp":"2026-08-30T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-fresh","cwd":"/project"}}`)

	store := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	newParser := func() Parser { return codex.NewParser() }
	w := New(session.AgentCodex, dir, 0, newParser, store)
	fsWatcher, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	require.NoError(t, fsWatcher.Close())

	// closed-watcher-add-fails-walk-still-ingests
	w.walkAndWatch(fsWatcher, dir)
	_, ok := store.GetById("sess-fresh")
	assert.True(t, ok)
}

func TestRun_Loaded(t *testing.T) {
	type testCase struct {
		_expectedSession bool
		_id              string

		store   *session.Store
		watcher *Watcher
	}

	newParser := func() Parser { return codex.NewParser() }
	tests := make([]*testCase, 0)

	// loaded-after-transcripts-ingested
	dir := t.TempDir()
	appendLine(t, filepath.Join(dir, "rollout-fresh.jsonl"), `{"timestamp":"2026-08-30T20:00:00.000Z","type":"session_meta","payload":{"id":"sess-fresh","cwd":"/project"}}`)
	store := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	tests = append(tests, &testCase{
		_id:              "loaded-after-transcripts-ingested",
		_expectedSession: true,

		store:   store,
		watcher: New(session.AgentCodex, dir, 0, newParser, store),
	})

	// missing-root-still-signals-loaded
	missingStore := session.NewStore(10, 25, events.NewBroker(), session.AgentCodex)
	tests = append(tests, &testCase{
		_id:              "missing-root-still-signals-loaded",
		_expectedSession: false,

		store:   missingStore,
		watcher: New(session.AgentCodex, filepath.Join(t.TempDir(), "missing"), 0, newParser, missingStore),
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			runUntilLoaded(t, test.watcher.Run, test.watcher.Loaded())

			_, ok := test.store.GetById("sess-fresh")
			assert.Equal(t, test._expectedSession, ok)
		})
	}
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

func findDeniedEvent(t *testing.T, store *session.Store, sessionId session.Id) *session.Event {
	t.Helper()
	sess, ok := store.GetById(sessionId)
	require.True(t, ok)

	for _, event := range sess.Events.All() {
		if event.Kind == session.EventKindPermissionDenied {
			return event
		}
	}
	require.FailNow(t, "No permission denied event", "session %s", sessionId)
	return nil
}

func TestWatcher_FileStates(t *testing.T) {
	toolUseLine := `{"type":"assistant","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"tu-bash","name":"Bash","input":{"command":"rm -rf /tmp/x"}}]}}`
	denialLine := `{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:01.000Z","isSidechain":false,"toolDenialKind":"user-rejected","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu-bash","is_error":true,"content":"The user doesn't want to proceed with this tool use."}]}}`

	// states-cover-transcript-meta-journal
	t.Run("states-cover-transcript-meta-journal", func(t *testing.T) {
		dir := t.TempDir()
		w := claudeWatcher(dir, session.NewStore(10, 25, events.NewBroker(), session.AgentClaude))

		transcriptPath := filepath.Join(dir, "s.jsonl")
		appendLine(t, transcriptPath, toolUseLine)
		require.NoError(t, w.readNewLines(transcriptPath))
		metaPath := writeSubagentMeta(t, dir, "s", "sub1", `{"agentType":"explore"}`)
		w.readSubagentMeta(metaPath)
		journalPath := writeWorkflowJournal(t, dir, "s", "wf_1",
			`{"type":"result","key":"k1","agentId":"sub1","result":1}`+"\n")
		require.NoError(t, w.readJournal(journalPath))

		states, err := w.FileStates()
		require.NoError(t, err)
		assert.Len(t, states, 3)
		for _, path := range []string{transcriptPath, metaPath, journalPath} {
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, info.Size(), states[path].Offset, path)
		}
		assert.NotEmpty(t, states[transcriptPath].Parser)
		assert.Empty(t, states[metaPath].Parser)
		assert.Empty(t, states[journalPath].Parser)
	})

	// restore-resumes-at-offset
	t.Run("restore-resumes-at-offset", func(t *testing.T) {
		dir := t.TempDir()
		transcriptPath := filepath.Join(dir, "s.jsonl")
		appendLine(t, transcriptPath, `{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:00:00.000Z","isSidechain":false,"promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"before snapshot"}]}}`)
		original := claudeWatcher(dir, session.NewStore(10, 25, events.NewBroker(), session.AgentClaude))
		require.NoError(t, original.readNewLines(transcriptPath))
		states, err := original.FileStates()
		require.NoError(t, err)

		restoredStore := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		restored := claudeWatcher(dir, restoredStore)
		require.NoError(t, restored.RestoreFiles(states))
		appendLine(t, transcriptPath, `{"type":"user","sessionId":"s","timestamp":"2026-04-05T15:01:00.000Z","isSidechain":false,"promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"after snapshot"}]}}`)
		require.NoError(t, restored.readNewLines(transcriptPath))

		sess, ok := restoredStore.GetById("s")
		require.True(t, ok)
		turns := sess.Turns(10)
		require.Len(t, turns, 1)
		assert.Contains(t, turns[0].Text, "after snapshot")
	})

	// restore-keeps-parser-state
	t.Run("restore-keeps-parser-state", func(t *testing.T) {
		dir := t.TempDir()
		transcriptPath := filepath.Join(dir, "s.jsonl")
		appendLine(t, transcriptPath, toolUseLine)
		uninterruptedStore := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		uninterrupted := claudeWatcher(dir, uninterruptedStore)
		require.NoError(t, uninterrupted.readNewLines(transcriptPath))
		states, err := uninterrupted.FileStates()
		require.NoError(t, err)

		restoredStore := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		restored := claudeWatcher(dir, restoredStore)
		require.NoError(t, restored.RestoreFiles(states))
		appendLine(t, transcriptPath, denialLine)
		require.NoError(t, uninterrupted.readNewLines(transcriptPath))
		require.NoError(t, restored.readNewLines(transcriptPath))

		expected := findDeniedEvent(t, uninterruptedStore, "s")
		actual := findDeniedEvent(t, restoredStore, "s")
		assert.Equal(t, expected, actual)
		assert.Equal(t, "Bash", actual.Permission.Tool)
		assert.Equal(t, "rm -rf /tmp/x", actual.Permission.Command)
	})

	// foreign-paths-ignored
	t.Run("foreign-paths-ignored", func(t *testing.T) {
		dir := t.TempDir()
		w := claudeWatcher(dir, session.NewStore(10, 25, events.NewBroker(), session.AgentClaude))
		states := map[string]session.FileState{
			filepath.Join(t.TempDir(), "other.jsonl"):    {Offset: 10},
			filepath.Join(dir+"-sibling", "other.jsonl"): {Offset: 20},
		}

		require.NoError(t, w.RestoreFiles(states))
		restored, err := w.FileStates()
		require.NoError(t, err)
		assert.Empty(t, restored)
	})

	// corrupt-parser-state-errors
	t.Run("corrupt-parser-state-errors", func(t *testing.T) {
		dir := t.TempDir()
		w := claudeWatcher(dir, session.NewStore(10, 25, events.NewBroker(), session.AgentClaude))
		states := map[string]session.FileState{
			filepath.Join(dir, "s.jsonl"): {Offset: 10, Parser: []byte("garbage")},
		}

		assert.Error(t, w.RestoreFiles(states))
	})
}
