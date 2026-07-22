package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirReadWrite(t *testing.T) {
	// diff-base-roundtrip
	t.Run("diff-base-roundtrip", func(t *testing.T) {
		dir := NewDir(t.TempDir())
		base := DiffBase{Sha: "abc1234", Target: "main"}
		require.NoError(t, dir.WriteDiffBase("claude", base, "s1"))

		read, ok := dir.ReadDiffBase("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, "abc1234", read.Sha)
		assert.Equal(t, "main", read.Target)
	})

	// malformed-diff-base-rejected
	t.Run("malformed-diff-base-rejected", func(t *testing.T) {
		dir := NewDir(t.TempDir())
		base := DiffBase{Sha: "--output=/tmp/pwned", Target: "main"}
		require.NoError(t, dir.WriteDiffBase("claude", base, "s1"))

		_, ok := dir.ReadDiffBase("claude", "s1")
		assert.False(t, ok, "non-sha diff.base content must not be served")
	})

	// snapshot-roundtrip-with-mtime
	t.Run("snapshot-roundtrip-with-mtime", func(t *testing.T) {
		dir := NewDir(t.TempDir())
		require.NoError(t, dir.WriteDiffSnapshot("claude", "diff content", "s1"))

		content, capturedAt, ok := dir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, "diff content", content)
		assert.False(t, capturedAt.IsZero())
	})

	// plan-versions-roundtrip-draft-vs-alteration
	t.Run("plan-versions-roundtrip-draft-vs-alteration", func(t *testing.T) {
		dir := NewDir(t.TempDir())
		initial := &PlanVersion{Content: "# initial", Index: 0}
		require.NoError(t, dir.WritePlanVersion("claude", "s1", initial))
		alteration := &PlanVersion{Content: "@@ alt @@", Index: 1, IsAlteration: true}
		require.NoError(t, dir.WritePlanVersion("claude", "s1", alteration))
		draft := &PlanVersion{Content: "@@ draft @@", Index: 2, IsAlteration: false}
		require.NoError(t, dir.WritePlanVersion("claude", "s1", draft))

		versions := dir.ReadPlanVersions("claude", "s1")
		require.Len(t, versions, 3)
		assert.Equal(t, 0, versions[0].Index)
		assert.Equal(t, "# initial", versions[0].Content)
		assert.Equal(t, 1, versions[1].Index)
		assert.True(t, versions[1].IsAlteration)
		assert.Equal(t, 2, versions[2].Index)
		assert.False(t, versions[2].IsAlteration)
	})

	// snapshot-5mb-truncation
	t.Run("snapshot-5mb-truncation", func(t *testing.T) {
		dir := NewDir(t.TempDir())
		big := strings.Repeat("z", MaxSnapshotBytes+1024)
		require.NoError(t, dir.WriteDiffSnapshot("claude", big, "s1"))

		content, _, ok := dir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Contains(t, content, "snapshot truncated at 5 MB")
		assert.Less(t, len(content), len(big))
	})

	// sanitized-path-components
	t.Run("sanitized-path-components", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		base := DiffBase{Sha: "abc1234", Target: "t"}
		require.NoError(t, dir.WriteDiffBase("claude", base, "../escape/id"))

		read, ok := dir.ReadDiffBase("claude", "../escape/id")
		require.True(t, ok)
		assert.Equal(t, "abc1234", read.Sha)

		// nothing was written outside the root
		escaped := filepath.Join(filepath.Dir(root), "escape")
		_, err := os.Stat(escaped)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestGc(t *testing.T) {
	// old-session-pruned
	t.Run("old-session-pruned", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffSnapshot("claude", "content", "old"))

		old := time.Now().Add(-48 * time.Hour)
		sessionPath := filepath.Join(root, "claude", "old")
		require.NoError(t, os.Chtimes(filepath.Join(sessionPath, diffSnapshotFile), old, old))

		dir.Gc(24 * time.Hour)
		_, err := os.Stat(sessionPath)
		assert.True(t, os.IsNotExist(err))
	})

	// fresh-session-kept
	t.Run("fresh-session-kept", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffSnapshot("claude", "content", "fresh"))

		dir.Gc(24 * time.Hour)
		_, err := os.Stat(filepath.Join(root, "claude", "fresh"))
		assert.NoError(t, err)
	})

	// zero-retention-noop
	t.Run("zero-retention-noop", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffSnapshot("claude", "content", "s1"))

		old := time.Now().Add(-1000 * time.Hour)
		require.NoError(t, os.Chtimes(filepath.Join(root, "claude", "s1", diffSnapshotFile), old, old))

		dir.Gc(0)
		_, err := os.Stat(filepath.Join(root, "claude", "s1"))
		assert.NoError(t, err)
	})
}
