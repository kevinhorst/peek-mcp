package watcher

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kevinhorst/peek-mcp/claude"
	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadNewLines_ProjectLabel(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cowork.jsonl")

	store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
	newParser := func() Parser { return claude.NewParser() }
	w := New(session.AgentClaude, dir, 0, newParser, store)
	w.Project = "cowork"

	appendLine(t, file, `{"type":"user","promptId":"p1","sessionId":"sess-cowork","timestamp":"2026-08-30T10:00:00.000Z","isSidechain":false,"cwd":"/store/local_x/outputs","message":{"role":"user","content":"hi"}}`)
	require.NoError(t, w.readNewLines(file))

	sess, ok := store.GetById("sess-cowork")
	require.True(t, ok)
	assert.Equal(t, "cowork", sess.Meta.Project)
}

func TestReadNewLines_LegacyProjectFromCwd(t *testing.T) {
	origHome := userHome
	userHome = "/Users/tester"
	t.Cleanup(func() { userHome = origHome })

	dir := t.TempDir()
	fileLegacy := filepath.Join(dir, "legacy.jsonl")
	fileRepo := filepath.Join(dir, "repo.jsonl")

	store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
	newParser := func() Parser { return claude.NewParser() }
	w := New(session.AgentClaude, dir, 0, newParser, store)

	appendLine(t, fileLegacy, `{"type":"user","promptId":"p1","sessionId":"sess-legacy","timestamp":"2026-08-30T10:00:00.000Z","isSidechain":false,"cwd":"/Users/tester/Claude_Unassigned","message":{"role":"user","content":"hi"}}`)
	require.NoError(t, w.readNewLines(fileLegacy))

	appendLine(t, fileRepo, `{"type":"user","promptId":"p1","sessionId":"sess-repo","timestamp":"2026-08-30T10:00:00.000Z","isSidechain":false,"cwd":"/Users/tester/GolandProjects/foo","message":{"role":"user","content":"hi"}}`)
	require.NoError(t, w.readNewLines(fileRepo))

	legacy, ok := store.GetById("sess-legacy")
	require.True(t, ok)
	assert.Equal(t, "Unassigned", legacy.Meta.Project)

	repo, ok := store.GetById("sess-repo")
	require.True(t, ok)
	assert.Empty(t, repo.Meta.Project)
}

func TestLegacyProjectFromCwd(t *testing.T) {
	origHome := userHome
	userHome = "/Users/tester"
	t.Cleanup(func() { userHome = origHome })

	cases := []struct {
		name string
		cwd  string
		want string
	}{
		{"exact dir", "/Users/tester/Claude_Unassigned", "Unassigned"},
		{"subdir cwd", "/Users/tester/Claude_Research/notes", "Research"},
		{"non-matching cwd", "/Users/tester/GolandProjects/foo", ""},
		{"empty cwd", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, legacyProjectFromCwd(tc.cwd))
		})
	}
}

func TestIsTranscriptPath_Filter(t *testing.T) {
	filtered := &Watcher{TranscriptPathOk: func(path string) bool {
		return strings.Contains(filepath.ToSlash(path), "/.claude/projects/")
	}}
	unfiltered := &Watcher{}

	cases := []struct {
		name    string
		watcher *Watcher
		path    string
		want    bool
	}{
		{"cowork transcript", filtered, "/store/acc/org/local_x/.claude/projects/slug/id.jsonl", true},
		{"audit log", filtered, "/store/acc/org/local_x/audit.jsonl", false},
		{"outputs artifact", filtered, "/store/acc/org/local_x/outputs/data.jsonl", false},
		{"non-jsonl", filtered, "/store/acc/org/local_x/.claude/projects/slug/id.json", false},
		{"nil filter accepts any jsonl", unfiltered, "/anywhere/file.jsonl", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.watcher.isTranscriptPath(tc.path))
		})
	}
}
