package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/watcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// provideBufferedSession loads 30 prompts and a 30-answer subagent into a session keeping depth turns, as a live store would.
func provideBufferedSession(t *testing.T, depth int) (*session.Session, *watcher.TurnLoader) {
	t.Helper()
	dir := t.TempDir()

	var parent strings.Builder
	for index := range 30 {
		fmt.Fprintf(&parent, `{"type":"user","promptId":"p%d","sessionId":"sess-1","timestamp":"2026-04-05T15:%02d:00.000Z","isSidechain":false,"message":{"role":"user","content":"prompt %d"}}`+"\n", index, index, index)
	}
	parentPath := filepath.Join(dir, "sess-1.jsonl")
	require.NoError(t, os.WriteFile(parentPath, []byte(parent.String()), 0o644))

	var sub strings.Builder
	for index := range 30 {
		fmt.Fprintf(&sub, `{"type":"assistant","agentId":"sub1","sessionId":"sess-1","requestId":"req_%d","timestamp":"2026-04-05T16:%02d:00.000Z","isSidechain":true,"message":{"role":"assistant","content":[{"type":"text","text":"answer %d"}]}}`+"\n", index, index, index)
	}
	subPath := filepath.Join(dir, "sess-1", "subagents", "agent-sub1.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(subPath), 0o755))
	require.NoError(t, os.WriteFile(subPath, []byte(sub.String()), 0o644))

	loader := watcher.NewTurnLoader(map[session.Agent]func() watcher.Parser{
		session.AgentClaude: func() watcher.Parser { return claude.NewParser() },
	})
	sess, err := loader.Load(session.AgentClaude, depth, "sess-1", parentPath, subPath)
	require.NoError(t, err)
	return sess, loader
}

func TestTurnSource_Turns(t *testing.T) {
	sess, loader := provideBufferedSession(t, 5)

	// buffer-answers-without-loading
	t.Run("buffer-answers-without-loading", func(t *testing.T) {
		buffered := sess.Turns(3)

		turns := NewTurnSource(sess, "", false).Turns(loader, buffered, 3)
		assert.Equal(t, buffered, turns)
	})

	// deeper-read-from-transcript
	t.Run("deeper-read-from-transcript", func(t *testing.T) {
		turns := NewTurnSource(sess, "", false).Turns(loader, sess.Turns(25), 25)

		require.Len(t, turns, 25)
		assert.Equal(t, "prompt 5", turns[0].Text)
		assert.Equal(t, "prompt 29", turns[24].Text)
	})

	// all-turns-read-from-transcript
	t.Run("all-turns-read-from-transcript", func(t *testing.T) {
		turns := NewTurnSource(sess, "", false).Turns(loader, sess.Turns(session.AllTurns), session.AllTurns)
		assert.Len(t, turns, 30)
	})

	// subagent-deeper-from-own-transcript
	t.Run("subagent-deeper-from-own-transcript", func(t *testing.T) {
		buffered, ok := sess.SubagentTurns("sub1", 25)
		require.True(t, ok)
		require.Len(t, buffered, 6)

		turns := NewTurnSource(sess, "sub1", false).Turns(loader, buffered, 25)
		require.Len(t, turns, 25)
		assert.Equal(t, "answer 5\n", turns[0].Text)
	})

	// nil-loader-keeps-buffer
	t.Run("nil-loader-keeps-buffer", func(t *testing.T) {
		buffered := sess.Turns(25)

		turns := NewTurnSource(sess, "", false).Turns(nil, buffered, 25)
		assert.Len(t, turns, len(buffered))
	})

	// transcript-gone-keeps-buffer
	t.Run("transcript-gone-keeps-buffer", func(t *testing.T) {
		gone, goneLoader := provideBufferedSession(t, 5)
		gone.FilePaths = []string{filepath.Join(t.TempDir(), "gone.jsonl")}
		buffered := gone.Turns(25)

		turns := NewTurnSource(gone, "", false).Turns(goneLoader, buffered, 25)
		assert.Len(t, turns, len(buffered))
	})
}

func TestTurnSource_MultipleTranscripts(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for file := range 2 {
		var transcript strings.Builder
		for index := file * 10; index < file*10+10; index++ {
			fmt.Fprintf(&transcript, `{"type":"user","promptId":"p%d","sessionId":"sess-1","timestamp":"2026-04-05T15:%02d:00.000Z","isSidechain":false,"message":{"role":"user","content":"prompt %d"}}`+"\n", index, index, index)
		}
		path := filepath.Join(dir, fmt.Sprintf("part-%d.jsonl", file))
		require.NoError(t, os.WriteFile(path, []byte(transcript.String()), 0o644))
		paths = append(paths, path)
	}

	loader := watcher.NewTurnLoader(map[session.Agent]func() watcher.Parser{
		session.AgentClaude: func() watcher.Parser { return claude.NewParser() },
	})
	sess, err := loader.Load(session.AgentClaude, 3, "sess-1", paths...)
	require.NoError(t, err)
	require.Equal(t, paths, sess.FilePaths, "every feeding transcript recorded in first-seen order")

	turns := NewTurnSource(sess, "", false).Turns(loader, sess.Turns(20), 20)

	require.Len(t, turns, 20)
	assert.Equal(t, "prompt 0", turns[0].Text)
	assert.Equal(t, "prompt 19", turns[19].Text)
}

func TestTurnSource_HasOlder(t *testing.T) {
	sess, _ := provideBufferedSession(t, 5)
	source := NewTurnSource(sess, "", false)

	// buffer-short-of-total
	assert.True(t, source.HasOlder(len(sess.Turns(session.AllTurns))))

	// everything-shown
	assert.False(t, source.HasOlder(30))
}
