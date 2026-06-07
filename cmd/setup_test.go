package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteConfig_CreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "settings.json")
	err := writeConfig(path, map[string]any{"key": "val"})
	assert.NoError(t, err)

	data, _ := os.ReadFile(path)
	var m map[string]any
	assert.NoError(t, json.Unmarshal(data, &m))
	assert.Equal(t, "val", m["key"])
}

func TestWriteConfig_PreservesExistingKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	initial := map[string]any{
		"existingKey": "existingVal",
		"mcpServers": map[string]any{
			"other-server": map[string]any{"url": "http://other"},
		},
	}
	assert.NoError(t, writeConfig(path, initial))

	data, err := os.ReadFile(path)
	assert.NoError(t, err)

	var cfg map[string]any
	assert.NoError(t, json.Unmarshal(data, &cfg))

	servers := cfg["mcpServers"].(map[string]any)
	servers["peek-mcp"] = map[string]any{"type": "http", "url": "http://localhost:4242/mcp"}
	cfg["mcpServers"] = servers

	assert.NoError(t, writeConfig(path, cfg))

	data, _ = os.ReadFile(path)
	var result map[string]any
	assert.NoError(t, json.Unmarshal(data, &result))

	resultServers := result["mcpServers"].(map[string]any)
	assert.Contains(t, resultServers, "other-server")
	assert.Contains(t, resultServers, "peek-mcp")
	assert.Equal(t, "existingVal", result["existingKey"])
}

func TestReplaceTOMLSection_NotFound(t *testing.T) {
	content := "[other]\nkey = \"val\"\n"
	result := replaceTOMLSection(content, "[mcp_servers.peek-mcp]", "new block\n")
	assert.Equal(t, content, result)
}

func TestReplaceTOMLSection_AtEnd(t *testing.T) {
	content := "[other]\nkey = \"val\"\n\n[mcp_servers.peek-mcp]\ncommand = \"old\"\nargs = [\"old\"]\n"
	result := replaceTOMLSection(content, "[mcp_servers.peek-mcp]", "[mcp_servers.peek-mcp]\ncommand = \"new\"\n")
	assert.Equal(t, "[other]\nkey = \"val\"\n\n[mcp_servers.peek-mcp]\ncommand = \"new\"\n", result)
}

func TestReplaceTOMLSection_InMiddle(t *testing.T) {
	content := "[mcp_servers.peek-mcp]\ncommand = \"old\"\n\n[other]\nkey = \"val\"\n"
	result := replaceTOMLSection(content, "[mcp_servers.peek-mcp]", "[mcp_servers.peek-mcp]\ncommand = \"new\"\n")
	assert.Equal(t, "[mcp_servers.peek-mcp]\ncommand = \"new\"\n[other]\nkey = \"val\"\n", result)
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()

	assert.Equal(t, filepath.Join(home, ".claude"), expandHome("~/.claude"))
	assert.Equal(t, filepath.Join(home, ".codex"), expandHome("$HOME/.codex"))
	assert.Equal(t, home, expandHome("~"))
	assert.Equal(t, home, expandHome("$HOME"))
	assert.Equal(t, "/absolute/path", expandHome("/absolute/path"))
	assert.Equal(t, "relative/path", expandHome("relative/path"))
}
