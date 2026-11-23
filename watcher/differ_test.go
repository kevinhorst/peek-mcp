package watcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kevinhorst/peek-mcp/events"
	"github.com/kevinhorst/peek-mcp/session"
	"github.com/kevinhorst/peek-mcp/state"
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

func TestDiffer_DiffBase(t *testing.T) {
	dir := initRepo(t, "main")
	gitRun(t, dir, "branch", "develop")
	gitRun(t, dir, "checkout", "-b", "feature", "develop")

	differ := NewDiffer(nil, session.NewStore(10, events.NewBroker(), session.AgentClaude), nil)
	ctx := context.Background()

	// cached-after-base-deleted
	assert.Equal(t, "develop", differ.diffBase(ctx, dir))
	gitRun(t, dir, "branch", "-D", "develop")
	assert.Equal(t, "develop", differ.diffBase(ctx, dir))

	// branch-switch-reinfers
	gitRun(t, dir, "checkout", "-b", "feature2", "main")
	assert.Equal(t, "main", differ.diffBase(ctx, dir))
}

func TestDiffer_SessionDiff(t *testing.T) {
	dir := buildFeatureRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("committed\nlocal\n"), 0o644))
	store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
	seedSession(t, store, "sess-1", dir)

	content, target, isFound := NewDiffer(nil, store, nil).SessionDiff(context.Background(), "sess-1")

	assert.True(t, isFound)
	assert.Equal(t, "main", target)
	assert.Contains(t, content, "+committed")
	assert.Contains(t, content, "+local")
	assert.NotContains(t, content, "upstream")
}

func TestDiffer_UncommittedDiff(t *testing.T) {
	ctx := context.Background()

	// uncommitted-in-tree-no-hook-file
	t.Run("uncommitted-in-tree-no-hook-file", func(t *testing.T) {
		dir := buildFeatureRepo(t)
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", dir)

		content, isFound := NewDiffer(nil, store, nil).UncommittedDiff(ctx, "s1")

		assert.True(t, isFound)
		assert.Contains(t, content, "+uncommitted")
		assert.NotContains(t, content, "feature.txt", "committed work is not uncommitted")
		assert.NoFileExists(t, filepath.Join(dir, ".git", "peek-diff"))
	})

	// not-a-repo-empty
	t.Run("not-a-repo-empty", func(t *testing.T) {
		store := session.NewStore(10, events.NewBroker(), session.AgentClaude)
		seedSession(t, store, "s1", t.TempDir())

		content, isFound := NewDiffer(nil, store, nil).UncommittedDiff(ctx, "s1")

		assert.True(t, isFound)
		assert.Empty(t, content)
	})

	// unknown-session-not-found
	t.Run("unknown-session-not-found", func(t *testing.T) {
		differ := NewDiffer(nil, session.NewStore(10, events.NewBroker(), session.AgentClaude), nil)

		_, isFound := differ.UncommittedDiff(ctx, "missing")
		assert.False(t, isFound)
	})
}

func TestGitDiff(t *testing.T) {
	ctx := context.Background()

	// oversized-output-capped
	t.Run("oversized-output-capped", func(t *testing.T) {
		dir := initRepo(t, "main")
		path := filepath.Join(dir, "big.txt")
		require.NoError(t, os.WriteFile(path, []byte("seed\n"), 0o644))
		gitRun(t, dir, "add", "big.txt")
		gitRun(t, dir, "commit", "-m", "big")
		line := strings.Repeat("x", 99) + "\n"
		require.NoError(t, os.WriteFile(path, []byte(strings.Repeat(line, 2*state.MaxSnapshotBytes/len(line))), 0o644))

		output, err := gitDiff(ctx, dir, "HEAD")

		require.NoError(t, err)
		assert.LessOrEqual(t, len(output), state.MaxSnapshotBytes+64)
		assert.Contains(t, output, "snapshot truncated at 5 MB")
	})

	// stat-dirty-index-untouched
	t.Run("stat-dirty-index-untouched", func(t *testing.T) {
		dir := initRepo(t, "main")
		path := filepath.Join(dir, "a.txt")
		require.NoError(t, os.WriteFile(path, []byte("a\n"), 0o644))
		gitRun(t, dir, "add", "a.txt")
		gitRun(t, dir, "commit", "-m", "a")
		future := time.Now().Add(time.Hour)
		require.NoError(t, os.Chtimes(path, future, future))
		indexPath := filepath.Join(dir, ".git", "index")
		before, err := os.Stat(indexPath)
		require.NoError(t, err)

		output, err := gitDiff(ctx, dir, "HEAD")

		require.NoError(t, err)
		assert.Empty(t, output)
		after, err := os.Stat(indexPath)
		require.NoError(t, err)
		assert.Equal(t, before.ModTime(), after.ModTime(), "git diff must not rewrite the index")
	})
}
