package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildFeatureRepo creates a repo where feature branched from main, main
// advanced with an unrelated commit, and feature has an uncommitted change.
func buildFeatureRepo(t *testing.T) string {
	t.Helper()
	dir := initRepo(t, "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shared.txt"), []byte("shared\n"), 0o644))
	gitRun(t, dir, "add", "shared.txt")
	gitRun(t, dir, "commit", "-m", "shared")
	gitRun(t, dir, "checkout", "-b", "feature", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("committed\n"), 0o644))
	gitRun(t, dir, "add", "feature.txt")
	gitRun(t, dir, "commit", "-m", "feature work")
	gitRun(t, dir, "checkout", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "upstream.txt"), []byte("upstream\n"), 0o644))
	gitRun(t, dir, "add", "upstream.txt")
	gitRun(t, dir, "commit", "-m", "upstream advance")
	gitRun(t, dir, "checkout", "feature")
	// Uncommitted edit lives on shared.txt (identical on both branches) so
	// branch switches in the tests carry it across without conflict.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shared.txt"), []byte("shared\nuncommitted\n"), 0o644))
	return dir
}

func seedSession(t *testing.T, store *session.Store, id session.Id, cwd string) {
	t.Helper()
	store.AddTurnBySessionId(id, session.AgentClaude, &session.Turn{
		Meta:      &session.Meta{SessionId: id, CWD: cwd},
		Role:      session.RoleUser,
		Text:      "hello",
		Timestamp: time.Now(),
	})
}

func TestRefresh_PinAndSnapshot(t *testing.T) {
	ctx := context.Background()

	// pin-survives-target-advance
	t.Run("pin-survives-target-advance", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, session.AgentClaude)
		seedSession(t, store, "s1", dir)
		w := NewDiffWatcher(store, time.Second, 0, state.NewDir(t.TempDir()))

		w.refresh(ctx, "s1", dir)
		sess, _ := store.GetById("s1")
		pinned := sess.DiffBase
		require.Len(t, pinned, 40, "base is pinned as a SHA")
		assert.Equal(t, "main", sess.DiffTarget)
		assert.Contains(t, sess.DiffOutput, "+uncommitted")
		assert.NotContains(t, sess.DiffOutput, "upstream")

		// advance the target branch further
		gitRun(t, dir, "checkout", "main")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "upstream2.txt"), []byte("upstream2\n"), 0o644))
		gitRun(t, dir, "add", "upstream2.txt")
		gitRun(t, dir, "commit", "-m", "upstream advance 2")
		gitRun(t, dir, "checkout", "feature")

		w.refresh(ctx, "s1", dir)
		sess, _ = store.GetById("s1")
		assert.Equal(t, pinned, sess.DiffBase, "pin does not move when the target advances")
		assert.NotContains(t, sess.DiffOutput, "upstream2")
	})

	// pin-survives-branch-merge
	t.Run("pin-survives-branch-merge", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, session.AgentClaude)
		seedSession(t, store, "s1", dir)
		w := NewDiffWatcher(store, time.Second, 0, state.NewDir(t.TempDir()))

		w.refresh(ctx, "s1", dir)
		sess, _ := store.GetById("s1")
		pinned := sess.DiffBase

		// merge feature into main and delete feature is not possible while checked
		// out; instead advance+merge main into a throwaway, then delete develop-like
		gitRun(t, dir, "checkout", "main")
		gitRun(t, dir, "merge", "--no-edit", "feature")
		gitRun(t, dir, "checkout", "feature")

		w.refresh(ctx, "s1", dir)
		sess, _ = store.GetById("s1")
		assert.Equal(t, pinned, sess.DiffBase)
		assert.Contains(t, sess.DiffOutput, "+uncommitted")
	})

	// failure-flips-to-snapshot
	t.Run("failure-flips-to-snapshot", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, session.AgentClaude)
		seedSession(t, store, "s1", dir)
		w := NewDiffWatcher(store, time.Second, 0, state.NewDir(t.TempDir()))

		w.refresh(ctx, "s1", dir)
		require.NoError(t, os.RemoveAll(dir))

		w.refresh(ctx, "s1", dir)
		sess, _ := store.GetById("s1")
		assert.Equal(t, session.DiffSourceSnapshot, sess.DiffSource)
		assert.Contains(t, sess.DiffOutput, "+uncommitted", "snapshot content is retained")
	})

	// empty-live-keeps-snapshot
	t.Run("empty-live-keeps-snapshot", func(t *testing.T) {
		// No branch divergence: the base pins to HEAD, so reverting the sole
		// uncommitted edit yields a genuinely empty live diff.
		dir := initRepo(t, "main")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("v1\n"), 0o644))
		gitRun(t, dir, "add", "tracked.txt")
		gitRun(t, dir, "commit", "-m", "tracked")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("v1\nextra\n"), 0o644))

		store := session.NewStore(10, session.AgentClaude)
		seedSession(t, store, "s1", dir)
		stateDir := state.NewDir(t.TempDir())
		w := NewDiffWatcher(store, time.Second, 0, stateDir)

		w.refresh(ctx, "s1", dir)
		snapshotBefore, _, ok := stateDir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Contains(t, snapshotBefore, "+extra")

		// revert the uncommitted change → clean working tree → empty diff
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("v1\n"), 0o644))
		w.refresh(ctx, "s1", dir)

		sess, _ := store.GetById("s1")
		assert.Empty(t, sess.DiffOutput)
		snapshotAfter, _, ok := stateDir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, snapshotBefore, snapshotAfter, "empty live output never overwrites the snapshot")
	})

	// snapshot-written-on-change-only
	t.Run("snapshot-written-on-change-only", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, session.AgentClaude)
		seedSession(t, store, "s1", dir)
		stateDir := state.NewDir(t.TempDir())
		w := NewDiffWatcher(store, time.Second, 0, stateDir)

		w.refresh(ctx, "s1", dir)
		content, _, ok := stateDir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Contains(t, content, "+uncommitted")
	})
}
