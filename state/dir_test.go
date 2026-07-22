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
		require.NoError(t, dir.WriteDiffBase("claude", "s1", DiffBase{Sha: "abc123", Target: "main"}))

		base, ok := dir.ReadDiffBase("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, "abc123", base.Sha)
		assert.Equal(t, "main", base.Target)
	})

	// snapshot-roundtrip-with-mtime
	t.Run("snapshot-roundtrip-with-mtime", func(t *testing.T) {
		dir := NewDir(t.TempDir())
		require.NoError(t, dir.WriteDiffSnapshot("claude", "s1", "diff content"))

		content, capturedAt, ok := dir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, "diff content", content)
		assert.False(t, capturedAt.IsZero())
	})

	// plan-versions-roundtrip-draft-vs-alteration
	t.Run("plan-versions-roundtrip-draft-vs-alteration", func(t *testing.T) {
		dir := NewDir(t.TempDir())
		require.NoError(t, dir.WritePlanVersion("claude", "s1", &PlanVersion{Index: 0, Content: "# initial"}))
		require.NoError(t, dir.WritePlanVersion("claude", "s1", &PlanVersion{Index: 1, Content: "@@ alt @@", IsAlteration: true}))
		require.NoError(t, dir.WritePlanVersion("claude", "s1", &PlanVersion{Index: 2, Content: "@@ draft @@", IsAlteration: false}))

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
		require.NoError(t, dir.WriteDiffSnapshot("claude", "s1", big))

		content, _, ok := dir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Contains(t, content, "snapshot truncated at 5 MB")
		assert.Less(t, len(content), len(big))
	})

	// sanitized-path-components
	t.Run("sanitized-path-components", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffBase("claude", "../escape/id", DiffBase{Sha: "sha", Target: "t"}))

		base, ok := dir.ReadDiffBase("claude", "../escape/id")
		require.True(t, ok)
		assert.Equal(t, "sha", base.Sha)

		// nothing was written outside the root
		escaped := filepath.Join(filepath.Dir(root), "escape")
		_, err := os.Stat(escaped)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestGC(t *testing.T) {
	// old-session-pruned
	t.Run("old-session-pruned", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffSnapshot("claude", "old", "content"))

		old := time.Now().Add(-48 * time.Hour)
		sessionPath := filepath.Join(root, "claude", "old")
		require.NoError(t, os.Chtimes(filepath.Join(sessionPath, diffSnapshotFile), old, old))

		dir.GC(24 * time.Hour)
		_, err := os.Stat(sessionPath)
		assert.True(t, os.IsNotExist(err))
	})

	// fresh-session-kept
	t.Run("fresh-session-kept", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffSnapshot("claude", "fresh", "content"))

		dir.GC(24 * time.Hour)
		_, err := os.Stat(filepath.Join(root, "claude", "fresh"))
		assert.NoError(t, err)
	})

	// zero-retention-noop
	t.Run("zero-retention-noop", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffSnapshot("claude", "s1", "content"))

		old := time.Now().Add(-1000 * time.Hour)
		require.NoError(t, os.Chtimes(filepath.Join(root, "claude", "s1", diffSnapshotFile), old, old))

		dir.GC(0)
		_, err := os.Stat(filepath.Join(root, "claude", "s1"))
		assert.NoError(t, err)
	})
}
