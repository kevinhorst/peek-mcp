package state

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

	// touch-advances-snapshot-mtime
	t.Run("touch-advances-snapshot-mtime", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffSnapshot("claude", "diff content", "s1"))
		old := time.Now().Add(-time.Hour)
		require.NoError(t, os.Chtimes(filepath.Join(root, "claude", "s1", diffSnapshotFile), old, old))

		require.NoError(t, dir.TouchDiffSnapshot("claude", "s1"))
		content, capturedAt, ok := dir.ReadDiffSnapshot("claude", "s1")
		require.True(t, ok)
		assert.Equal(t, "diff content", content)
		assert.True(t, capturedAt.After(old))
	})

	// touch-missing-snapshot-errors
	t.Run("touch-missing-snapshot-errors", func(t *testing.T) {
		dir := NewDir(t.TempDir())

		assert.Error(t, dir.TouchDiffSnapshot("claude", "s1"))
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

	// telemetry-roundtrip
	t.Run("telemetry-roundtrip", func(t *testing.T) {
		dir := NewDir(t.TempDir())
		require.NoError(t, dir.WriteTelemetry("claude", "s1", `{"active_seconds":1}`))

		content, err := dir.ReadTelemetry("claude", "s1")
		require.NoError(t, err)
		assert.Equal(t, `{"active_seconds":1}`, content)
	})

	// telemetry-missing-file-errors
	t.Run("telemetry-missing-file-errors", func(t *testing.T) {
		dir := NewDir(t.TempDir())

		_, err := dir.ReadTelemetry("claude", "s1")
		assert.Error(t, err)
	})

	// telemetry-sanitized-components
	t.Run("telemetry-sanitized-components", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteTelemetry("claude", "../escape", "{}"))

		content, err := dir.ReadTelemetry("claude", "../escape")
		require.NoError(t, err)
		assert.Equal(t, "{}", content)
		assert.NoFileExists(t, filepath.Join(filepath.Dir(root), "escape", "telemetry.json"))
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

	// write-leaves-no-temp
	t.Run("write-leaves-no-temp", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		initial := &PlanVersion{Content: "# initial", Index: 0}
		require.NoError(t, dir.WritePlanVersion("claude", "s1", initial))
		require.NoError(t, dir.WritePlanLatest("claude", "# initial", "s1"))

		entries, err := os.ReadDir(filepath.Join(root, "claude", "s1", planDir))
		require.NoError(t, err)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		assert.Equal(t, []string{initialFile, planLatestFile}, names)
	})

	// stray-temp-not-a-plan-version
	t.Run("stray-temp-not-a-plan-version", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		initial := &PlanVersion{Content: "# initial", Index: 0}
		require.NoError(t, dir.WritePlanVersion("claude", "s1", initial))
		strayPath := filepath.Join(root, "claude", "s1", planDir, ".000.md.123.tmp")
		require.NoError(t, os.WriteFile(strayPath, []byte("# torn"), 0o600))

		versions := dir.ReadPlanVersions("claude", "s1")
		require.Len(t, versions, 1)
		assert.Equal(t, "# initial", versions[0].Content)
	})

	// concurrent-writers-one-target
	t.Run("concurrent-writers-one-target", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		contents := []string{"# one", "# two", "# three", "# four", "# five", "# six", "# seven", "# eight"}

		errChan := make(chan error, len(contents))
		var waitGroup sync.WaitGroup
		for _, content := range contents {
			waitGroup.Go(func() {
				errChan <- dir.WritePlanLatest("claude", content, "s1")
			})
		}
		waitGroup.Wait()
		close(errChan)

		failed := 0
		for err := range errChan {
			if err == nil {
				continue
			}

			// Windows refuses a rename onto a target another writer is replacing at that moment.
			var linkErr *os.LinkError
			require.ErrorAs(t, err, &linkErr)
			require.Equal(t, "windows", runtime.GOOS, "err = %v", err)
			failed++
		}
		assert.Less(t, failed, len(contents))
		latest, ok := dir.ReadPlanLatest("claude", "s1")
		require.True(t, ok)
		assert.Contains(t, contents, latest)

		entries, err := os.ReadDir(filepath.Join(root, "claude", "s1", planDir))
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, planLatestFile, entries[0].Name())
	})
}

func TestSize(t *testing.T) {
	dir := NewDir(filepath.Join(t.TempDir(), "missing"))
	assert.Zero(t, dir.Size())

	dir = NewDir(t.TempDir())
	require.NoError(t, dir.WriteDiffSnapshot("claude", "12345", "s1"))
	require.NoError(t, dir.WritePlanLatest("claude", "abc", "s1"))
	assert.Equal(t, int64(8), dir.Size())
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

		dir.Gc(24*time.Hour, 0)
		_, err := os.Stat(sessionPath)
		assert.True(t, os.IsNotExist(err))
	})

	// fresh-session-kept
	t.Run("fresh-session-kept", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffSnapshot("claude", "content", "fresh"))

		dir.Gc(24*time.Hour, 0)
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

		dir.Gc(0, 0)
		_, err := os.Stat(filepath.Join(root, "claude", "s1"))
		assert.NoError(t, err)
	})

	// old-snapshot-pruned-dir-kept
	t.Run("old-snapshot-pruned-dir-kept", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffSnapshot("claude", "content", "s1"))
		require.NoError(t, dir.WritePlanLatest("claude", "# plan", "s1"))

		old := time.Now().Add(-48 * time.Hour)
		snapshotPath := filepath.Join(root, "claude", "s1", diffSnapshotFile)
		require.NoError(t, os.Chtimes(snapshotPath, old, old))

		dir.Gc(0, 24*time.Hour)
		assert.NoFileExists(t, snapshotPath)
		assert.FileExists(t, filepath.Join(root, "claude", "s1", planDir, planLatestFile))
	})

	// fresh-snapshot-kept
	t.Run("fresh-snapshot-kept", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffSnapshot("claude", "content", "s1"))

		dir.Gc(0, 24*time.Hour)
		assert.FileExists(t, filepath.Join(root, "claude", "s1", diffSnapshotFile))
	})

	// zero-snapshot-retention-noop
	t.Run("zero-snapshot-retention-noop", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteDiffSnapshot("claude", "content", "s1"))

		old := time.Now().Add(-1000 * time.Hour)
		snapshotPath := filepath.Join(root, "claude", "s1", diffSnapshotFile)
		require.NoError(t, os.Chtimes(snapshotPath, old, old))
		now := time.Now()
		require.NoError(t, os.Chtimes(filepath.Join(root, "claude", "s1"), now, now))

		dir.Gc(0, 0)
		assert.FileExists(t, snapshotPath)
	})

	// stale-temp-removed
	t.Run("stale-temp-removed", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WritePlanLatest("claude", "# plan", "s1"))
		tempPath := filepath.Join(root, "claude", "s1", planDir, ".latest.md.123.tmp")
		require.NoError(t, os.WriteFile(tempPath, []byte("# torn"), 0o600))

		old := time.Now().Add(-2 * time.Hour)
		require.NoError(t, os.Chtimes(tempPath, old, old))

		dir.Gc(0, 0)
		assert.NoFileExists(t, tempPath)
		assert.FileExists(t, filepath.Join(root, "claude", "s1", planDir, planLatestFile))
	})

	// fresh-temp-kept
	t.Run("fresh-temp-kept", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		tempPath := filepath.Join(root, "claude", "s1", planDir, ".latest.md.123.tmp")
		require.NoError(t, os.MkdirAll(filepath.Dir(tempPath), 0o700))
		require.NoError(t, os.WriteFile(tempPath, []byte("# in flight"), 0o600))

		dir.Gc(0, 0)
		assert.FileExists(t, tempPath)
	})
}

func TestStatDiffSnapshot(t *testing.T) {
	dir := NewDir(t.TempDir())
	_, ok := dir.StatDiffSnapshot("claude", "s1")
	assert.False(t, ok)

	require.NoError(t, dir.WriteDiffSnapshot("claude", "content", "s1"))
	capturedAt, ok := dir.StatDiffSnapshot("claude", "s1")
	assert.True(t, ok)
	assert.False(t, capturedAt.IsZero())
}

func TestInstances(t *testing.T) {
	// newest-first-with-limit
	t.Run("newest-first-with-limit", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteInstance("100-1", `{"pid":1}`))
		require.NoError(t, dir.WriteInstance("200-2", `{"pid":2}`))

		old := time.Now().Add(-time.Hour)
		require.NoError(t, os.Chtimes(filepath.Join(root, "instances", "100-1.json"), old, old))

		assert.Equal(t, []string{`{"pid":2}`, `{"pid":1}`}, dir.ReadInstances(10))
		assert.Equal(t, []string{`{"pid":2}`}, dir.ReadInstances(1))
	})

	// missing-dir-nil
	t.Run("missing-dir-nil", func(t *testing.T) {
		dir := NewDir(t.TempDir())
		assert.Nil(t, dir.ReadInstances(10))
	})

	// old-instance-pruned-by-gc
	t.Run("old-instance-pruned-by-gc", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteInstance("100-1", `{"pid":1}`))
		require.NoError(t, dir.WriteInstance("200-2", `{"pid":2}`))

		old := time.Now().Add(-48 * time.Hour)
		require.NoError(t, os.Chtimes(filepath.Join(root, "instances", "100-1.json"), old, old))

		dir.Gc(24*time.Hour, 0)
		assert.Equal(t, []string{`{"pid":2}`}, dir.ReadInstances(10))
	})
}

func TestPruneInstances(t *testing.T) {
	// old-removed-fresh-kept
	t.Run("old-removed-fresh-kept", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteInstance("100-1", `{"pid":1}`))
		require.NoError(t, dir.WriteInstance("200-2", `{"pid":2}`))
		require.NoError(t, os.WriteFile(filepath.Join(root, "instances", "notes.txt"), []byte("keep"), 0o644))

		old := time.Now().Add(-49 * time.Hour)
		require.NoError(t, os.Chtimes(filepath.Join(root, "instances", "100-1.json"), old, old))
		require.NoError(t, os.Chtimes(filepath.Join(root, "instances", "notes.txt"), old, old))

		dir.PruneInstances(48 * time.Hour)

		assert.Equal(t, []string{`{"pid":2}`}, dir.ReadInstances(10))
		_, err := os.Stat(filepath.Join(root, "instances", "notes.txt"))
		assert.NoError(t, err, "non-json files are ignored")
	})

	// missing-dir-noop
	t.Run("missing-dir-noop", func(t *testing.T) {
		dir := NewDir(t.TempDir())
		dir.PruneInstances(48 * time.Hour)
	})
}

func TestInstanceStore(t *testing.T) {
	writeStore := func(writer io.Writer) error {
		_, err := io.WriteString(writer, "snapshot")
		return err
	}

	// write-open-remove
	t.Run("write-open-remove", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteInstanceStore("100-1", writeStore))

		file, err := dir.OpenInstanceStore("100-1")
		require.NoError(t, err)
		content, err := io.ReadAll(file)
		require.NoError(t, file.Close())
		require.NoError(t, err)
		assert.Equal(t, "snapshot", string(content))

		dir.RemoveInstanceStore("100-1")
		_, err = dir.OpenInstanceStore("100-1")
		assert.True(t, os.IsNotExist(err))
	})

	// prune-removes-dead-process-store
	t.Run("prune-removes-dead-process-store", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteInstanceStore("100-1", writeStore))
		require.NoError(t, dir.WriteInstanceStore("200-2", writeStore))

		dir.PruneInstanceStores(func(pid int) bool { return pid != 1 })

		assert.NoFileExists(t, filepath.Join(root, "instances", "100-1.store"))
		assert.FileExists(t, filepath.Join(root, "instances", "200-2.store"))
	})

	// prune-keeps-live-process-store
	t.Run("prune-keeps-live-process-store", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteInstanceStore("100-1", writeStore))

		dir.PruneInstanceStores(func(pid int) bool { return pid == 1 })

		assert.FileExists(t, filepath.Join(root, "instances", "100-1.store"))
	})

	// prune-ignores-records
	t.Run("prune-ignores-records", func(t *testing.T) {
		root := t.TempDir()
		dir := NewDir(root)
		require.NoError(t, dir.WriteInstance("100-1", `{"pid":1}`))
		require.NoError(t, dir.WriteInstanceStore("100-1", writeStore))
		assert.Equal(t, []string{`{"pid":1}`}, dir.ReadInstances(10))

		dir.PruneInstanceStores(func(pid int) bool { return false })

		assert.NoFileExists(t, filepath.Join(root, "instances", "100-1.store"))
		assert.Equal(t, []string{`{"pid":1}`}, dir.ReadInstances(10))
	})
}
