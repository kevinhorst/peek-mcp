package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/kevinhorst/peek-mcp/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	warmTestSessionId = "sess-warm"
	warmTestWindow    = 24 * time.Hour
)

func appendTranscriptLine(t *testing.T, line, path string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	defer file.Close()

	_, err = file.WriteString(line + "\n")
	require.NoError(t, err)
}

func awaitWarm(t *testing.T, store *session.Store) {
	t.Helper()
	select {
	case <-store.Ready():
	case <-time.After(10 * time.Second):
		require.FailNow(t, "Store not ready within 10s")
	}
}

func hasInstanceStore(deps *warmDeps) bool {
	file, err := deps.stateDir.OpenInstanceStore(deps.invocations.Id())
	if err != nil {
		return false
	}
	file.Close()
	return true
}

// newTestWarmDeps builds the warm dependencies over a temp Claude home holding one transcript
// with two user turns, and empty Codex and Cowork homes. It returns the transcript path.
func newTestWarmDeps(t *testing.T, isStateDirActive bool) (*warmDeps, string) {
	t.Helper()
	claudeHome := t.TempDir()
	projectDir := filepath.Join(claudeHome, claude.ProjectsDir, "-Users-a-project")
	require.NoError(t, os.MkdirAll(projectDir, 0o755))

	transcriptPath := filepath.Join(projectDir, warmTestSessionId+".jsonl")
	appendTranscriptLine(t, `{"type":"user","sessionId":"sess-warm","timestamp":"2026-10-02T15:00:00.000Z","isSidechain":false,"promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"first prompt"}]}}`, transcriptPath)
	appendTranscriptLine(t, `{"type":"user","sessionId":"sess-warm","timestamp":"2026-10-02T15:01:00.000Z","isSidechain":false,"promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"second prompt"}]}}`, transcriptPath)

	var stateDir *state.Dir
	if isStateDirActive {
		stateDir = state.NewDir(t.TempDir())
	}

	info := tools.InstanceInfo{
		PID:       os.Getpid(),
		StartedAt: time.Now(),
		Transport: "stdio",
	}
	broker := events.NewBroker()
	deps := &warmDeps{
		broker:       broker,
		claudeHome:   claudeHome,
		codexHome:    t.TempDir(),
		coworkHome:   t.TempDir(),
		invocations:  tools.NewInvocationCounter(info, stateDir),
		pollInterval: time.Hour,
		pollWindow:   time.Hour,
		stateDir:     stateDir,
		store:        session.NewStore(200, 25, broker, session.AgentClaude, session.AgentCodex),
	}
	return deps, transcriptPath
}

func sessionTurnCount(store *session.Store) int {
	count := 0
	store.WithSession(warmTestSessionId, func(current *session.Session) {
		count = len(current.Turns(100))
	})
	return count
}

func startWarm(t *testing.T, deps *warmDeps) *warmSet {
	t.Helper()
	set := startWarmSet(context.Background(), deps, warmTestWindow)
	awaitWarm(t, deps.store)
	return set
}

func TestWarmSet(t *testing.T) {
	// stop-writes-store-and-empties
	t.Run("stop-writes-store-and-empties", func(t *testing.T) {
		deps, _ := newTestWarmDeps(t, true)
		set := startWarm(t, deps)
		require.Positive(t, sessionTurnCount(deps.store))

		set.stop(deps)

		assert.Empty(t, deps.store.List())
		assert.False(t, deps.store.IsReady())
		assert.True(t, hasInstanceStore(deps))
	})

	// restart-resumes-without-double-ingest
	t.Run("restart-resumes-without-double-ingest", func(t *testing.T) {
		deps, _ := newTestWarmDeps(t, true)
		set := startWarm(t, deps)
		turnsBefore := sessionTurnCount(deps.store)
		set.stop(deps)

		set = startWarm(t, deps)
		defer set.stop(deps)

		assert.Equal(t, turnsBefore, sessionTurnCount(deps.store))
		assert.False(t, hasInstanceStore(deps))
	})

	// lines-added-while-cold-are-read
	t.Run("lines-added-while-cold-are-read", func(t *testing.T) {
		deps, transcriptPath := newTestWarmDeps(t, true)
		set := startWarm(t, deps)
		turnsBefore := sessionTurnCount(deps.store)
		set.stop(deps)

		appendTranscriptLine(t, `{"type":"user","sessionId":"sess-warm","timestamp":"2026-10-02T15:02:00.000Z","isSidechain":false,"promptId":"p3","message":{"role":"user","content":[{"type":"text","text":"written while cold"}]}}`, transcriptPath)
		set = startWarm(t, deps)
		defer set.stop(deps)

		assert.Equal(t, turnsBefore+1, sessionTurnCount(deps.store))
	})

	// corrupt-snapshot-falls-back
	t.Run("corrupt-snapshot-falls-back", func(t *testing.T) {
		deps, _ := newTestWarmDeps(t, true)
		err := deps.stateDir.WriteInstanceStore(deps.invocations.Id(), func(writer io.Writer) error {
			_, err := io.WriteString(writer, "not a gob stream")
			return err
		})
		require.NoError(t, err)

		set := startWarm(t, deps)
		defer set.stop(deps)

		assert.Equal(t, 2, sessionTurnCount(deps.store))
		assert.False(t, hasInstanceStore(deps))
	})

	// no-state-dir-full-ingest
	t.Run("no-state-dir-full-ingest", func(t *testing.T) {
		deps, _ := newTestWarmDeps(t, false)
		set := startWarm(t, deps)
		turnsBefore := sessionTurnCount(deps.store)
		set.stop(deps)

		set = startWarm(t, deps)
		defer set.stop(deps)

		assert.Equal(t, 2, turnsBefore)
		assert.Equal(t, turnsBefore, sessionTurnCount(deps.store))
	})
}
