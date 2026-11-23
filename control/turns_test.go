package control

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/watcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDeepServer serves a session of 5 prompts whose store keeps only 2 finished turns.
func newDeepServer(t *testing.T) *Server {
	t.Helper()
	path := filepath.Join(t.TempDir(), "deep.jsonl")
	var transcript strings.Builder
	for index := range 5 {
		fmt.Fprintf(&transcript, `{"type":"user","promptId":"p%d","sessionId":"deep","timestamp":"2026-04-05T15:%02d:00.000Z","isSidechain":false,"message":{"role":"user","content":"prompt %d"}}`+"\n", index, index, index)
	}
	require.NoError(t, os.WriteFile(path, []byte(transcript.String()), 0o644))

	broker := events.NewBroker()
	store := session.NewStore(2, broker, session.AgentClaude)
	start := time.Date(2026, 4, 5, 15, 0, 0, 0, time.UTC)
	for index := range 5 {
		store.AddTurnBySessionId("deep", session.AgentClaude, &session.Turn{
			FilePath:  path,
			Meta:      &session.Meta{SessionId: "deep"},
			PromptId:  fmt.Sprintf("p%d", index),
			Role:      session.RoleUser,
			Text:      fmt.Sprintf("prompt %d", index),
			Timestamp: start.Add(time.Duration(index) * time.Minute),
		})
	}

	loader := watcher.NewTurnLoader(map[session.Agent]func() watcher.Parser{
		session.AgentClaude: func() watcher.Parser { return claude.NewParser() },
	})
	server, err := New(&Options{Store: store, Broker: broker, Loader: loader, Version: "test", Depth: 2})
	require.NoError(t, err)
	return server
}

func TestTurnsFragment_OlderTurns(t *testing.T) {
	server := newDeepServer(t)

	// buffer-shown-with-load-link
	t.Run("buffer-shown-with-load-link", func(t *testing.T) {
		response := get(server, "/fragments/sessions/deep/turns")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<pre>prompt 4</pre>")
		assert.NotContains(t, body, "<pre>prompt 0</pre>")
		assert.Contains(t, body, "load older turns")
		assert.Contains(t, body, "all=on")
		assert.Contains(t, body, "peek-refresh")
	})

	// all-turns-read-from-transcript
	t.Run("all-turns-read-from-transcript", func(t *testing.T) {
		response := get(server, "/fragments/sessions/deep/turns?all=on")
		require.Equal(t, http.StatusOK, response.Code)
		body := response.Body.String()
		assert.Contains(t, body, "<pre>prompt 0</pre>")
		assert.Contains(t, body, "<pre>prompt 4</pre>")
		assert.NotContains(t, body, "load older turns")
		assert.NotContains(t, body, "peek-refresh", "live refresh pauses while every turn is shown")
	})
}

func TestTurns_BeyondBuffer(t *testing.T) {
	server := newDeepServer(t)

	turns := decode[turnsResponse](t, get(server, "/api/sessions/deep/turns?n=5"))
	require.Len(t, turns.Turns, 5)
	assert.Equal(t, "prompt 0", turns.Turns[0].Text)

	turns = decode[turnsResponse](t, get(server, "/api/sessions/deep/turns?n=2"))
	require.Len(t, turns.Turns, 2)
	assert.Equal(t, "prompt 3", turns.Turns[0].Text)
}
