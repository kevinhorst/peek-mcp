package watcher

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func provideIndexWatcher(t *testing.T, indexContent string) (*CodexIndexWatcher, *session.Store) {
	codexHome := t.TempDir()
	if indexContent != "" {
		indexPath := filepath.Join(codexHome, "session_index.jsonl")
		require.NoError(t, os.WriteFile(indexPath, []byte(indexContent), 0644))
	}

	store := session.NewStore(10, session.AgentCodex)
	return NewCodexIndexWatcher(codexHome, store), store
}

func TestLoadIndex(t *testing.T) {
	type testCase struct {
		_expectedSessions int
		_expectedTitle    string
		_id               string
		_sessionId        session.Id

		indexContent string
	}

	tests := make([]*testCase, 0)

	// titles-land-in-store
	test := &testCase{
		_id:               "titles-land-in-store",
		_expectedSessions: 1,
		_expectedTitle:    "Set up pgroll migrations",
		_sessionId:        "s1",

		indexContent: `{"id":"s1","thread_name":"Set up pgroll migrations","updated_at":"2026-03-29T20:38:12.893516Z"}` + "\n",
	}
	tests = append(tests, test)

	// last-line-wins-duplicate-ids
	test = &testCase{
		_id:               "last-line-wins-duplicate-ids",
		_expectedSessions: 1,
		_expectedTitle:    "Renamed thread",
		_sessionId:        "s1",

		indexContent: `{"id":"s1","thread_name":"Old name","updated_at":"2026-03-29T20:38:12Z"}` + "\n" +
			`{"id":"s1","thread_name":"Renamed thread","updated_at":"2026-03-29T21:00:00Z"}` + "\n",
	}
	tests = append(tests, test)

	// skips-malformed-lines
	test = &testCase{
		_id:               "skips-malformed-lines",
		_expectedSessions: 1,
		_expectedTitle:    "Valid thread",
		_sessionId:        "s2",

		indexContent: "not json at all\n" +
			`{"id":"","thread_name":"missing id","updated_at":"2026-03-29T20:38:12Z"}` + "\n" +
			`{"id":"s2","thread_name":"Valid thread","updated_at":"2026-03-29T20:38:12Z"}` + "\n",
	}
	tests = append(tests, test)

	// missing-file-noop
	test = &testCase{
		_id:               "missing-file-noop",
		_expectedSessions: 0,

		indexContent: "",
	}
	tests = append(tests, test)

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			watcher, store := provideIndexWatcher(t, test.indexContent)
			watcher.loadIndex()

			sessions := store.List(session.AgentCodex)
			assert.Len(t, sessions, test._expectedSessions)
			if test._expectedSessions == 0 {
				return
			}

			sess, ok := store.GetById(test._sessionId)
			assert.True(t, ok, "session should exist")
			assert.Equal(t, test._expectedTitle, sess.Title)
			assert.Equal(t, session.TitleSourceIndex, sess.TitleSource)
		})
	}
}
