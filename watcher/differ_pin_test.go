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

func TestDiffer_SessionDiffPinAndSnapshot(t *testing.T) {
	ctx := context.Background()

	// pin-survives-target-advance
	t.Run("pin-survives-target-advance", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		differ := NewDiffer(nil, store, state.NewDir(t.TempDir()))

		content, target, _ := differ.SessionDiff(ctx, "s1")
		sess, _ := store.GetById("s1")
		pinned := sess.DiffBase
		require.Len(t, pinned, 40, "base is pinned as a SHA")
		assert.Equal(t, "main", target)
		assert.Contains(t, content, "+uncommitted")
		assert.NotContains(t, content, "upstream")

		// advance the target branch further
		gitRun(t, dir, "checkout", "main")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "upstream2.txt"), []byte("upstream2\n"), 0o644))
		gitRun(t, dir, "add", "upstream2.txt")
		gitRun(t, dir, "commit", "-m", "upstream advance 2")
		gitRun(t, dir, "checkout", "feature")

		content, _, _ = differ.SessionDiff(ctx, "s1")
		sess, _ = store.GetById("s1")
		assert.Equal(t, pinned, sess.DiffBase, "pin does not move when the target advances")
		assert.NotContains(t, content, "upstream2")
	})

	// pin-survives-branch-merge
	t.Run("pin-survives-branch-merge", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		differ := NewDiffer(nil, store, state.NewDir(t.TempDir()))

		differ.SessionDiff(ctx, "s1")
		sess, _ := store.GetById("s1")
		pinned := sess.DiffBase

		gitRun(t, dir, "checkout", "main")
		gitRun(t, dir, "merge", "--no-edit", "feature")
		gitRun(t, dir, "checkout", "feature")

		content, _, _ := differ.SessionDiff(ctx, "s1")
		sess, _ = store.GetById("s1")
		assert.Equal(t, pinned, sess.DiffBase)
		assert.Contains(t, content, "+uncommitted")
	})

	// gone-tree-serves-snapshot
	t.Run("gone-tree-serves-snapshot", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		differ := NewDiffer(nil, store, state.NewDir(t.TempDir()))

		differ.SessionDiff(ctx, "s1")
		require.NoError(t, os.RemoveAll(dir))

		content, target, isFound := differ.SessionDiff(ctx, "s1")
		assert.True(t, isFound)
		assert.Equal(t, "main", target)
		assert.Contains(t, content, "+uncommitted", "snapshot content is served")
	})

	// gone-tree-without-snapshot-empty
	t.Run("gone-tree-without-snapshot-empty", func(t *testing.T) {
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", filepath.Join(t.TempDir(), "removed"))
		differ := NewDiffer(nil, store, state.NewDir(t.TempDir()))

		content, _, isFound := differ.SessionDiff(ctx, "s1")
		assert.True(t, isFound)
		assert.Empty(t, content)
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

		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		stateDir := state.NewDir(t.TempDir())
		differ := NewDiffer(nil, store, stateDir)

		differ.SessionDiff(ctx, "s1")
		snapshotBefore, _, ok := stateDir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Contains(t, snapshotBefore, "+extra")

		require.NoError(t, os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("v1\n"), 0o644))
		content, _, _ := differ.SessionDiff(ctx, "s1")

		assert.Empty(t, content)
		snapshotAfter, _, ok := stateDir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, snapshotBefore, snapshotAfter, "empty live output never overwrites the snapshot")
	})

	// snapshot-written-and-marked
	t.Run("snapshot-written-and-marked", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		stateDir := state.NewDir(t.TempDir())
		differ := NewDiffer(nil, store, stateDir)

		content, _, _ := differ.SessionDiff(ctx, "s1")
		snapshot, _, ok := stateDir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, content, snapshot)
		sess, _ := store.GetById("s1")
		assert.True(t, sess.HasDiffSnapshot)
	})

	// base-read-from-disk
	t.Run("base-read-from-disk", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		stateDir := state.NewDir(t.TempDir())
		head, err := gitOutput(ctx, dir, "rev-parse", "HEAD")
		require.NoError(t, err)
		written := state.DiffBase{Sha: head, Target: "develop"}
		require.NoError(t, stateDir.WriteDiffBase("claude", written, "s1"))
		differ := NewDiffer(nil, store, stateDir)

		content, target, _ := differ.SessionDiff(ctx, "s1")
		sess, _ := store.GetById("s1")
		assert.Equal(t, head, sess.DiffBase)
		assert.Equal(t, "develop", target)
		assert.Contains(t, content, "+uncommitted")
		assert.NotContains(t, content, "feature.txt", "committed work is behind the written base")
		persisted, ok := stateDir.ReadDiffBase("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, written, persisted)
	})

	// unknown-session-not-found
	t.Run("unknown-session-not-found", func(t *testing.T) {
		differ := NewDiffer(nil, session.NewStore(10, events.NewBroker(), session.AgentClaude), nil)

		_, _, isFound := differ.SessionDiff(ctx, "missing")
		assert.False(t, isFound)
	})
}

func TestDiffer_PinFirstTurn(t *testing.T) {
	ctx := context.Background()

	// landed-branch-keeps-committed-work
	t.Run("landed-branch-keeps-committed-work", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		differ := NewDiffer(nil, store, state.NewDir(t.TempDir()))
		differ.pinFirstTurn(ctx, "s1")

		gitRun(t, dir, "checkout", "main")
		gitRun(t, dir, "merge", "--no-edit", "feature")
		gitRun(t, dir, "checkout", "feature")

		content, target, _ := differ.SessionDiff(ctx, "s1")
		assert.Equal(t, "main", target)
		assert.Contains(t, content, "feature.txt", "committed work stays in the diff after the branch landed")
		assert.Contains(t, content, "+uncommitted")
	})

	// stale-session-not-pinned
	t.Run("stale-session-not-pinned", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		store.AddTurnBySessionId("s1", session.AgentClaude, &session.Turn{
			Meta:      &session.Meta{SessionId: "s1", CWD: dir},
			Role:      session.RoleUser,
			Text:      "hello",
			Timestamp: time.Now().Add(-2 * pinWindow),
		})
		differ := NewDiffer(nil, store, nil)

		differ.pinFirstTurn(ctx, "s1")

		sess, _ := store.GetById("s1")
		assert.Empty(t, sess.DiffBase)
		assert.NotContains(t, differ.pinAttempted, session.Id("s1"))
	})

	// already-pinned-untouched
	t.Run("already-pinned-untouched", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)
		store.PinDiffBase("s1", "abc1234", "develop")
		differ := NewDiffer(nil, store, nil)

		differ.pinFirstTurn(ctx, "s1")

		sess, _ := store.GetById("s1")
		assert.Equal(t, "abc1234", sess.DiffBase)
		assert.Equal(t, "develop", sess.DiffTarget)
	})

	// not-a-repo-attempted-once
	t.Run("not-a-repo-attempted-once", func(t *testing.T) {
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", t.TempDir())
		differ := NewDiffer(nil, store, nil)

		differ.pinFirstTurn(ctx, "s1")

		sess, _ := store.GetById("s1")
		assert.Empty(t, sess.DiffBase)
		assert.Contains(t, differ.pinAttempted, session.Id("s1"), "a failed pin is not retried on every turn")
	})
}

func TestDiffer_Run(t *testing.T) {
	dir := buildFeatureRepo(t)
	broker := events.NewBroker()
	store := session.NewStore(10, broker, session.AgentClaude)
	differ := NewDiffer(broker, store, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- differ.Run(ctx) }()

	require.Eventually(t, func() bool {
		seedSession(t, store, "s1", dir)
		var base string
		store.WithSession("s1", func(sess *session.Session) { base = sess.DiffBase })
		return base != ""
	}, 5*time.Second, 50*time.Millisecond, "a turn of a live session pins its base")

	cancel()
	assert.ErrorIs(t, <-done, context.Canceled)
}
