package watcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@test",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func initRepo(t *testing.T, defaultBranch string) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", defaultBranch)
	gitRun(t, dir, "commit", "--allow-empty", "-m", "initial")
	return dir
}

func TestInferDiffBase(t *testing.T) {
	type testCase struct {
		_expected string
		_id       string
		branch    string
		cwd       string
	}

	tests := make([]*testCase, 0)

	// reflog-local-branch
	dir := initRepo(t, "main")
	gitRun(t, dir, "branch", "develop")
	gitRun(t, dir, "checkout", "-b", "feature", "develop")
	tests = append(tests, &testCase{
		_id:       "reflog-local-branch",
		_expected: "develop",
		branch:    "feature",
		cwd:       dir,
	})

	// reflog-remote-ref
	dir = initRepo(t, "main")
	gitRun(t, dir, "update-ref", "refs/remotes/origin/develop", "HEAD")
	gitRun(t, dir, "checkout", "-b", "feature", "origin/develop")
	tests = append(tests, &testCase{
		_id:       "reflog-remote-ref",
		_expected: "origin/develop",
		branch:    "feature",
		cwd:       dir,
	})

	// reflog-created-from-head-falls-back
	dir = initRepo(t, "main")
	gitRun(t, dir, "checkout", "-b", "feature")
	tests = append(tests, &testCase{
		_id:       "reflog-created-from-head-falls-back",
		_expected: "main",
		branch:    "feature",
		cwd:       dir,
	})

	// reflog-claude-branch-discarded
	dir = initRepo(t, "main")
	gitRun(t, dir, "branch", "claude/task")
	gitRun(t, dir, "checkout", "-b", "feature", "claude/task")
	tests = append(tests, &testCase{
		_id:       "reflog-claude-branch-discarded",
		_expected: "main",
		branch:    "feature",
		cwd:       dir,
	})

	// reflog-base-deleted
	dir = initRepo(t, "main")
	gitRun(t, dir, "branch", "tmp")
	gitRun(t, dir, "checkout", "-b", "feature", "tmp")
	gitRun(t, dir, "branch", "-D", "tmp")
	tests = append(tests, &testCase{
		_id:       "reflog-base-deleted",
		_expected: "main",
		branch:    "feature",
		cwd:       dir,
	})

	// origin-head-fallback
	dir = initRepo(t, "trunk")
	gitRun(t, dir, "update-ref", "refs/remotes/origin/develop", "HEAD")
	gitRun(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
	gitRun(t, dir, "checkout", "-b", "feature")
	tests = append(tests, &testCase{
		_id:       "origin-head-fallback",
		_expected: "origin/develop",
		branch:    "feature",
		cwd:       dir,
	})

	// local-probe-master
	dir = initRepo(t, "master")
	gitRun(t, dir, "checkout", "-b", "feature")
	tests = append(tests, &testCase{
		_id:       "local-probe-master",
		_expected: "master",
		branch:    "feature",
		cwd:       dir,
	})

	// terminal-head
	dir = initRepo(t, "trunk")
	tests = append(tests, &testCase{
		_id:       "terminal-head",
		_expected: "HEAD",
		branch:    "trunk",
		cwd:       dir,
	})

	// detached-head
	dir = initRepo(t, "main")
	tests = append(tests, &testCase{
		_id:       "detached-head",
		_expected: "main",
		branch:    "HEAD",
		cwd:       dir,
	})

	// Run tests
	for _, test := range tests {
		t.Run(test._id, func(t *testing.T) {
			assert.Equal(t, test._expected, inferDiffBase(context.Background(), test.branch, test.cwd))
		})
	}
}

func TestDiffWatcher_DiffBase(t *testing.T) {
	dir := initRepo(t, "main")
	gitRun(t, dir, "branch", "develop")
	gitRun(t, dir, "checkout", "-b", "feature", "develop")

	store := session.NewStore(10, session.AgentClaude)
	w := NewDiffWatcher(store, time.Second, 0)
	ctx := context.Background()

	// cached-after-base-deleted
	assert.Equal(t, "develop", w.diffBase(ctx, dir))
	gitRun(t, dir, "branch", "-D", "develop")
	assert.Equal(t, "develop", w.diffBase(ctx, dir))

	// branch-switch-reinfers
	gitRun(t, dir, "checkout", "-b", "feature2", "main")
	assert.Equal(t, "main", w.diffBase(ctx, dir))
}

func TestDiffWatcher_Refresh(t *testing.T) {
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("committed\nuncommitted\n"), 0o644))

	store := session.NewStore(10, session.AgentClaude)
	turn := &session.Turn{
		Meta:      &session.Meta{SessionId: "sess-1", CWD: dir},
		Role:      session.RoleUser,
		Text:      "hello",
		Timestamp: time.Now(),
	}
	store.AddTurnBySessionId("sess-1", session.AgentClaude, turn)

	w := NewDiffWatcher(store, time.Second, 0)
	w.refresh(context.Background(), "sess-1", dir)

	sess, ok := store.GetById("sess-1")
	require.True(t, ok)
	assert.Equal(t, "main", sess.DiffTarget)
	assert.Contains(t, sess.DiffOutput, "+committed")
	assert.Contains(t, sess.DiffOutput, "+uncommitted")
	assert.NotContains(t, sess.DiffOutput, "upstream")
}
