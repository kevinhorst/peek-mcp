package cmd

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMcpArgs(t *testing.T) {
	assert.Equal(t, []string{"start", "--transport=stdio"}, mcpArgs(true))
	assert.Equal(t, []string{"start", "--transport=stdio", "--control-port=0"}, mcpArgs(false))
}

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

func scriptedPrompter(input string) *prompter {
	return &prompter{scanner: bufio.NewScanner(strings.NewReader(input)), out: io.Discard}
}

func setTestHome(t *testing.T, home string) {
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
		return
	}
	t.Setenv("HOME", home)
}

func readTelemetryEnv(t *testing.T, home string) map[string]any {
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	require.NoError(t, err)
	cfg := map[string]any{}
	require.NoError(t, json.Unmarshal(data, &cfg))
	env, _ := cfg["env"].(map[string]any)
	return env
}

func TestSetupTelemetry(t *testing.T) {
	// writes-all-env-keys-with-interval
	t.Run("writes-all-env-keys-with-interval", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)

		require.NoError(t, setupTelemetry(scriptedPrompter("\n"), true))

		env := readTelemetryEnv(t, home)
		assert.Equal(t, "1", env["CLAUDE_CODE_ENABLE_TELEMETRY"])
		assert.Equal(t, "otlp", env["OTEL_METRICS_EXPORTER"])
		assert.Equal(t, "http/json", env["OTEL_EXPORTER_OTLP_PROTOCOL"])
		assert.Equal(t, "http://127.0.0.1:42442/otlp", env["OTEL_EXPORTER_OTLP_ENDPOINT"])
		assert.Equal(t, "10000", env["OTEL_METRIC_EXPORT_INTERVAL"])
		assert.NotContains(t, env, "OTEL_EXPORTER_OTLP_HEADERS")
	})

	// custom-control-port-in-endpoint
	t.Run("custom-control-port-in-endpoint", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)
		setupControlPort = 42542
		t.Cleanup(func() { setupControlPort = controlPortBase })

		require.NoError(t, setupTelemetry(scriptedPrompter("\n"), true))

		env := readTelemetryEnv(t, home)
		assert.Equal(t, "http://127.0.0.1:42542/otlp", env["OTEL_EXPORTER_OTLP_ENDPOINT"])
	})

	// deletes-stale-headers-key
	t.Run("deletes-stale-headers-key", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)
		existing := `{"env": {"OTEL_EXPORTER_OTLP_HEADERS": "Authorization=Bearer old", "OTHER": "kept"}}`
		require.NoError(t, os.MkdirAll(filepath.Join(home, ".claude"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(existing), 0o644))

		require.NoError(t, setupTelemetry(scriptedPrompter("y\n"), true))

		env := readTelemetryEnv(t, home)
		assert.NotContains(t, env, "OTEL_EXPORTER_OTLP_HEADERS")
		assert.Equal(t, "kept", env["OTHER"])
	})

	// control-server-disabled-writes-nothing
	t.Run("control-server-disabled-writes-nothing", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)

		require.NoError(t, setupTelemetry(scriptedPrompter(""), false))

		_, err := os.Stat(filepath.Join(home, ".claude", "settings.json"))
		assert.True(t, os.IsNotExist(err))
	})

	// decline-writes-nothing
	t.Run("decline-writes-nothing", func(t *testing.T) {
		home := t.TempDir()
		setTestHome(t, home)

		require.NoError(t, setupTelemetry(scriptedPrompter("n\n"), true))

		_, err := os.Stat(filepath.Join(home, ".claude", "settings.json"))
		assert.True(t, os.IsNotExist(err))
	})
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()

	assert.Equal(t, filepath.Join(home, ".claude"), expandHome("~/.claude"))
	assert.Equal(t, filepath.Join(home, ".codex"), expandHome("$HOME/.codex"))
	assert.Equal(t, filepath.Join(home, ".claude"), expandHome("${HOME}/.claude"))
	assert.Equal(t, home, expandHome("~"))
	assert.Equal(t, home, expandHome("$HOME"))
	assert.Equal(t, home, expandHome("${HOME}"))
	assert.Equal(t, "/absolute/path", expandHome("/absolute/path"))
	assert.Equal(t, "relative/path", expandHome("relative/path"))
}
