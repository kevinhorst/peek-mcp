package claude

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

const (
	memoryDirName        = "memory"
	memoryIndexFile      = "MEMORY.md"
	worktreeDirSeparator = "--claude-worktrees-"

	frontmatterFence = "---"
	typeFieldPrefix  = "type:"

	markdownSuffix = ".md"
	maxMemoryBytes = 64 * 1024
)

type Memory struct {
	Facts       []*MemoryFact `json:"facts,omitempty"`
	Index       string        `json:"index,omitempty"`
	IsTruncated bool          `json:"truncated,omitempty"`
}

type MemoryFact struct {
	Body string `json:"body"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

func factType(body string) string {
	if !strings.HasPrefix(body, frontmatterFence) {
		return ""
	}

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, typeFieldPrefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, typeFieldPrefix))
		}
	}
	return ""
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.IsDir()
}

func isFactFile(entry os.DirEntry) bool {
	if entry.IsDir() {
		return false
	}

	if entry.Name() == memoryIndexFile {
		return false
	}

	return strings.HasSuffix(entry.Name(), markdownSuffix)
}

func readFact(memoryDir, name string) *MemoryFact {
	data, err := os.ReadFile(filepath.Join(memoryDir, name))
	if err != nil {
		return nil
	}

	body := string(data)
	fact := &MemoryFact{
		Body: body,
		Name: strings.TrimSuffix(name, markdownSuffix),
		Type: factType(body),
	}
	return fact
}

func resolveMemoryDir(projectDir string) string {
	direct := filepath.Join(projectDir, memoryDirName)
	if isDir(direct) {
		return direct
	}

	base := filepath.Base(projectDir)
	stripped, _, found := strings.Cut(base, worktreeDirSeparator)
	if !found {
		return ""
	}

	fallback := filepath.Join(filepath.Dir(projectDir), stripped, memoryDirName)
	if isDir(fallback) {
		return fallback
	}
	return ""
}

func ReadMemory(transcriptPath string) (*Memory, error) {
	memoryDir := resolveMemoryDir(filepath.Dir(transcriptPath))
	if memoryDir == "" {
		return nil, errors.New("ReadMemory: No memory directory found")
	}

	memory := &Memory{}
	budget := maxMemoryBytes

	if index, err := os.ReadFile(filepath.Join(memoryDir, memoryIndexFile)); err == nil {
		memory.Index = string(index)
		budget -= len(memory.Index)
	}

	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return nil, errors.Wrap(err, "ReadMemory: Failed to read memory directory")
	}

	for _, entry := range entries {
		if !isFactFile(entry) {
			continue
		}

		if budget <= 0 {
			memory.IsTruncated = true
			break
		}

		fact := readFact(memoryDir, entry.Name())
		if fact == nil {
			continue
		}

		budget -= len(fact.Body)
		memory.Facts = append(memory.Facts, fact)
	}

	return memory, nil
}
