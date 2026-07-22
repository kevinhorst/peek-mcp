package watcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kevinhorst/peek-mcp/codex"
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

func TestReadNewLines_PerFileParserState(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "rollout-a.jsonl")
	fileB := filepath.Join(dir, "rollout-b.jsonl")

	store := session.NewStore(10, session.AgentCodex)
	newParser := func() Parser { return codex.NewParser() }
	w := New(session.AgentCodex, dir, newParser, store)

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
	assert.Equal(t, "second message in A", turns[0].Text)

	// B holds only its own meta-only turn — no text turn from file A leaked in
	sessionB, ok := store.GetById("sess-b")
	assert.True(t, ok)
	for _, turn := range sessionB.Turns(10) {
		assert.Empty(t, turn.Text)
	}
}
