package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeLoaderTranscripts writes a parent transcript of count prompts and a subagent transcript of count answers.
func writeLoaderTranscripts(t *testing.T, count int) (string, string) {
	t.Helper()
	dir := t.TempDir()

	var parent strings.Builder
	for index := range count {
		fmt.Fprintf(&parent, `{"type":"user","promptId":"p%d","sessionId":"sess-1","timestamp":"2026-04-05T15:%02d:00.000Z","isSidechain":false,"message":{"role":"user","content":"prompt %d"}}`+"\n", index, index, index)
	}
	parentPath := filepath.Join(dir, "sess-1.jsonl")
	require.NoError(t, os.WriteFile(parentPath, []byte(parent.String()), 0o644))

	var sub strings.Builder
	for index := range count {
		fmt.Fprintf(&sub, `{"type":"assistant","agentId":"sub1","sessionId":"sess-1","requestId":"req_%d","timestamp":"2026-04-05T16:%02d:00.000Z","isSidechain":true,"message":{"role":"assistant","content":[{"type":"text","text":"answer %d"}]}}`+"\n", index, index, index)
	}
	subPath := filepath.Join(dir, "sess-1", "subagents", "agent-sub1.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(subPath), 0o755))
	require.NoError(t, os.WriteFile(subPath, []byte(sub.String()), 0o644))
	return parentPath, subPath
}

func TestTurnLoader_Load(t *testing.T) {
	parentPath, subPath := writeLoaderTranscripts(t, 30)
	loader := NewTurnLoader(map[session.Agent]func() Parser{
		session.AgentClaude: func() Parser { return claude.NewParser() },
	})

	// main-turns-beyond-buffer
	t.Run("main-turns-beyond-buffer", func(t *testing.T) {
		loaded, err := loader.Load(session.AgentClaude, 31, "sess-1", parentPath)

		require.NoError(t, err)
		turns := loaded.Turns(session.AllTurns)
		require.Len(t, turns, 30)
		assert.Equal(t, "prompt 0", turns[0].Text)
		assert.Equal(t, "prompt 29", turns[29].Text)
	})

	// subagent-own-transcript
	t.Run("subagent-own-transcript", func(t *testing.T) {
		loaded, err := loader.Load(session.AgentClaude, 31, "sess-1", parentPath, subPath)

		require.NoError(t, err)
		turns, ok := loaded.SubagentTurns("sub1", session.AllTurns)
		require.True(t, ok)
		require.Len(t, turns, 30)
		assert.Equal(t, "answer 0\n", turns[0].Text)
		assert.Equal(t, subPath, loaded.Subagents["sub1"].FilePath)
	})

	// depth-bounds-the-loaded-buffer
	t.Run("depth-bounds-the-loaded-buffer", func(t *testing.T) {
		loaded, err := loader.Load(session.AgentClaude, 4, "sess-1", parentPath)

		require.NoError(t, err)
		assert.Len(t, loaded.Turns(session.AllTurns), 5, "4 finished plus the active turn")
		assert.Equal(t, 30, loaded.TotalTurns())
	})

	// unknown-agent-error
	t.Run("unknown-agent-error", func(t *testing.T) {
		_, err := loader.Load(session.AgentCodex, 31, "sess-1", parentPath)
		assert.Error(t, err)
	})

	// session-absent-error
	t.Run("session-absent-error", func(t *testing.T) {
		_, err := loader.Load(session.AgentClaude, 31, "other", parentPath)
		assert.Error(t, err)
	})

	// missing-file-error
	t.Run("missing-file-error", func(t *testing.T) {
		_, err := loader.Load(session.AgentClaude, 31, "sess-1", filepath.Join(t.TempDir(), "gone.jsonl"))
		assert.Error(t, err)
	})
}
