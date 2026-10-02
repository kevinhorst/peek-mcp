package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
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
		store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		w := NewDiffWatcher(store, events.NewBroker(), time.Second, 0, state.NewDir(t.TempDir()))

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
		store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		w := NewDiffWatcher(store, events.NewBroker(), time.Second, 0, state.NewDir(t.TempDir()))

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
		store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		w := NewDiffWatcher(store, events.NewBroker(), time.Second, 0, state.NewDir(t.TempDir()))

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

		store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		stateDir := state.NewDir(t.TempDir())
		w := NewDiffWatcher(store, events.NewBroker(), time.Second, 0, stateDir)

		w.refresh(ctx, "s1", dir)
		snapshotBefore, _, ok := stateDir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Contains(t, snapshotBefore, "+extra")

		// revert the uncommitted change → clean working tree → empty diff; a new turn makes the snapshot stale
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("v1\n"), 0o644))
		seedSession(t, store, "s1", dir)
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
		store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		stateDir := state.NewDir(t.TempDir())
		w := NewDiffWatcher(store, events.NewBroker(), time.Second, 0, stateDir)

		w.refresh(ctx, "s1", dir)
		content, _, ok := stateDir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Contains(t, content, "+uncommitted")
	})

	// fresh-snapshot-adopted
	t.Run("fresh-snapshot-adopted", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		stateDir := state.NewDir(t.TempDir())
		require.NoError(t, stateDir.WriteDiffSnapshot("claude", "marker", "s1"))
		w := NewDiffWatcher(store, events.NewBroker(), time.Second, 0, stateDir)

		w.refresh(ctx, "s1", dir)
		sess, _ := store.GetById("s1")
		assert.Equal(t, "marker", sess.DiffOutput)
		assert.Equal(t, session.DiffSourceLive, sess.DiffSource)
		assert.True(t, sess.HasDiffSnapshot)
	})

	// stale-snapshot-recomputed
	t.Run("stale-snapshot-recomputed", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		root := t.TempDir()
		stateDir := state.NewDir(root)
		require.NoError(t, stateDir.WriteDiffSnapshot("claude", "marker", "s1"))
		old := time.Now().Add(-time.Hour)
		require.NoError(t, os.Chtimes(filepath.Join(root, "claude", "s1", "diff.snapshot"), old, old))
		w := NewDiffWatcher(store, events.NewBroker(), time.Second, 0, stateDir)

		w.refresh(ctx, "s1", dir)
		sess, _ := store.GetById("s1")
		assert.Contains(t, sess.DiffOutput, "+uncommitted")
		content, _, ok := stateDir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, sess.DiffOutput, content)
	})

	// unchanged-snapshot-touched
	t.Run("unchanged-snapshot-touched", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		root := t.TempDir()
		stateDir := state.NewDir(root)
		w := NewDiffWatcher(store, events.NewBroker(), time.Second, 0, stateDir)
		w.refresh(ctx, "s1", dir)
		snapshotPath := filepath.Join(root, "claude", "s1", "diff.snapshot")
		old := time.Now().Add(-time.Hour)
		require.NoError(t, os.Chtimes(snapshotPath, old, old))
		before := statPinned(t, snapshotPath)

		w.refresh(ctx, "s1", dir)
		after, err := os.Stat(snapshotPath)
		require.NoError(t, err)
		assert.True(t, after.ModTime().After(old))
		assert.True(t, os.SameFile(before, after), "timestamp advanced in place, no rewrite")
		sess, _ := store.GetById("s1")
		content, _, ok := stateDir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, sess.DiffOutput, content)
	})

	// base-read-from-disk
	t.Run("base-read-from-disk", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, 25, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		stateDir := state.NewDir(t.TempDir())
		head, err := gitOutput(ctx, dir, "rev-parse", "HEAD")
		require.NoError(t, err)
		written := state.DiffBase{Sha: head, Target: "develop"}
		require.NoError(t, stateDir.WriteDiffBase("claude", written, "s1"))
		w := NewDiffWatcher(store, events.NewBroker(), time.Second, 0, stateDir)

		w.refresh(ctx, "s1", dir)
		sess, _ := store.GetById("s1")
		assert.Equal(t, head, sess.DiffBase)
		assert.Equal(t, "develop", sess.DiffTarget)
		assert.Contains(t, sess.DiffOutput, "+uncommitted")
		assert.NotContains(t, sess.DiffOutput, "feature.txt", "committed work is behind the written base")
		persisted, ok := stateDir.ReadDiffBase("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, written, persisted)
	})
}
