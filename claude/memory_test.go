package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeMemoryDir(t *testing.T, memoryDir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(memoryDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "MEMORY.md"), []byte("- index line"), 0o644))
	fact := "---\ntype: user\n---\nbody"
	require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "user_thing.md"), []byte(fact), 0o644))
}

func TestReadMemory(t *testing.T) {
	// direct-memory-dir
	t.Run("direct-memory-dir", func(t *testing.T) {
		projectDir := t.TempDir()
		writeMemoryDir(t, filepath.Join(projectDir, "memory"))

		memory, err := ReadMemory(filepath.Join(projectDir, "transcript.jsonl"))
		require.NoError(t, err)
		assert.Equal(t, "- index line", memory.Index)
		require.Len(t, memory.Facts, 1)
		assert.Equal(t, "user_thing", memory.Facts[0].Name)
		assert.Equal(t, "user", memory.Facts[0].Type)
	})

	// worktree-suffix-fallback
	t.Run("worktree-suffix-fallback", func(t *testing.T) {
		root := t.TempDir()
		worktreeProject := filepath.Join(root, "proj--claude-worktrees-feature-x")
		require.NoError(t, os.MkdirAll(worktreeProject, 0o755))
		writeMemoryDir(t, filepath.Join(root, "proj", "memory"))

		memory, err := ReadMemory(filepath.Join(worktreeProject, "transcript.jsonl"))
		require.NoError(t, err)
		require.Len(t, memory.Facts, 1)
		assert.Equal(t, "user_thing", memory.Facts[0].Name)
	})

	// no-memory-dir-errors
	t.Run("no-memory-dir-errors", func(t *testing.T) {
		projectDir := t.TempDir()
		_, err := ReadMemory(filepath.Join(projectDir, "transcript.jsonl"))
		assert.Error(t, err)
	})

	// budget-truncation
	t.Run("budget-truncation", func(t *testing.T) {
		projectDir := t.TempDir()
		memoryDir := filepath.Join(projectDir, "memory")
		require.NoError(t, os.MkdirAll(memoryDir, 0o755))
		big := strings.Repeat("y", 40*1024)
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "aaa.md"), []byte(big), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "bbb.md"), []byte(big), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(memoryDir, "ccc.md"), []byte(big), 0o644))

		memory, err := ReadMemory(filepath.Join(projectDir, "transcript.jsonl"))
		require.NoError(t, err)
		assert.True(t, memory.Truncated)
	})
}
