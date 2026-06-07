package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func runSetup(_ *cobra.Command, _ []string) {
	fi, _ := os.Stdin.Stat()
	if fi.Mode()&os.ModeCharDevice == 0 {
		fmt.Fprintln(os.Stderr, "stdin is not a terminal — run 'peek-mcp start' instead")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stdout, "peek-mcp %s — interactive setup\n\n", Version())

	p := newPrompter()
	choice := p.Choose("Which environment do you want to configure?", []string{
		"Claude Code     (~/.claude/settings.json)",
		"Claude Desktop  (claude_desktop_config.json)",
		"Codex CLI       (~/.codex/config.toml)",
		"All",
		"Exit",
	}, 0)

	type setupFn func(*prompter) error
	var steps []setupFn

	switch choice {
	case 0:
		steps = []setupFn{setupClaudeCode}
	case 1:
		steps = []setupFn{setupClaudeDesktop}
	case 2:
		steps = []setupFn{setupCodex}
	case 3:
		steps = []setupFn{setupClaudeCode, setupClaudeDesktop, setupCodex}
	default:
		return
	}

	for i, fn := range steps {
		if i > 0 {
			fmt.Println()
		}
		if err := fn(p); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("\nDone. Start the server with: peek-mcp start")
}

func setupClaudeCode(p *prompter) error {
	fmt.Println("Configuring peek-mcp for Claude Code...")

	binPath, err := resolveBinaryPath()
	if err != nil {
		return fmt.Errorf("cannot determine peek-mcp binary path: %w", err)
	}
	fmt.Printf("  Binary: %s\n", binPath)

	path := filepath.Join(defaultHome(".claude"), "settings.json")
	fmt.Printf("  Config: %s\n", path)

	// Read existing config or start fresh.
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	cfg := map[string]any{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("%s contains invalid JSON: %w", path, err)
		}
	}

	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if _, exists := servers["peek-mcp"]; exists {
		if !p.Confirm("  peek-mcp is already configured. Overwrite?", false) {
			fmt.Println("  Skipped.")
			return nil
		}
	}

	servers["peek-mcp"] = map[string]any{
		"command": binPath,
		"args":    []string{"start", "--transport=stdio"},
	}
	cfg["mcpServers"] = servers

	if !p.Confirm("  Write MCP server config?", true) {
		fmt.Println("  Skipped.")
		return nil
	}
	if err := writeConfig(path, cfg); err != nil {
		return err
	}
	fmt.Println("  ✓ Wrote MCP server config.")

	if p.Confirm("  Install hot-reload hook? (injects live git diff into every prompt)", true) {
		if err := installHotReloadHook(path, cfg); err != nil {
			return err
		}
	}

	return nil
}

func installHotReloadHook(path string, cfg map[string]any) error {
	raw, _ := json.Marshal(cfg)
	if strings.Contains(string(raw), "peek-diff") {
		fmt.Println("  ✓ Hook already present.")
		return nil
	}

	hookEntry := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": `cat "$(git rev-parse --git-path peek-diff)" 2>/dev/null`,
			},
		},
	}

	hooks, _ := cfg["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	existing, _ := hooks["UserPromptSubmit"].([]any)
	hooks["UserPromptSubmit"] = append(existing, hookEntry)
	cfg["hooks"] = hooks

	if err := writeConfig(path, cfg); err != nil {
		return err
	}
	fmt.Println("  ✓ Installed UserPromptSubmit hook.")
	return nil
}

func setupClaudeDesktop(p *prompter) error {
	fmt.Println("Configuring peek-mcp for Claude Desktop...")

	binPath, err := resolveBinaryPath()
	if err != nil {
		return fmt.Errorf("cannot determine peek-mcp binary path: %w", err)
	}
	fmt.Printf("  Binary: %s\n", binPath)

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	path := filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	fmt.Printf("  Config: %s\n", path)

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	cfg := map[string]any{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("%s contains invalid JSON: %w", path, err)
		}
	}

	servers, _ := cfg["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if _, exists := servers["peek-mcp"]; exists {
		if !p.Confirm("  peek-mcp is already configured. Overwrite?", false) {
			fmt.Println("  Skipped.")
			return nil
		}
	}

	servers["peek-mcp"] = map[string]any{
		"command": binPath,
		"args":    []string{"start", "--transport=stdio"},
	}
	cfg["mcpServers"] = servers

	if !p.Confirm("  Write config?", true) {
		fmt.Println("  Skipped.")
		return nil
	}
	if err := writeConfig(path, cfg); err != nil {
		return err
	}
	fmt.Println("  ✓ Wrote Claude Desktop config.")
	return nil
}

func setupCodex(p *prompter) error {
	fmt.Println("Configuring peek-mcp for Codex CLI...")

	binPath, err := resolveBinaryPath()
	if err != nil {
		return fmt.Errorf("cannot determine peek-mcp binary path: %w", err)
	}
	fmt.Printf("  Binary: %s\n", binPath)

	path := filepath.Join(defaultHome(".codex"), "config.toml")
	fmt.Printf("  Config: %s\n", path)

	content, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	block := fmt.Sprintf("[mcp_servers.peek-mcp]\ncommand = %q\nargs = [\"start\", \"--transport=stdio\"]\n",
		binPath)

	text := string(content)
	if strings.Contains(text, "[mcp_servers.peek-mcp]") {
		if !p.Confirm("  peek-mcp is already configured. Overwrite?", false) {
			fmt.Println("  Skipped.")
			return nil
		}
		text = replaceTOMLSection(text, "[mcp_servers.peek-mcp]", block)
	} else {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		if text != "" {
			text += "\n"
		}
		text += block
	}

	if !p.Confirm("  Write config?", true) {
		fmt.Println("  Skipped.")
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return err
	}
	fmt.Println("  ✓ Wrote Codex config.")
	return nil
}

// replaceTOMLSection replaces a [section] block (up to the next [header] or EOF).
func replaceTOMLSection(content, header, replacement string) string {
	idx := strings.Index(content, header)
	if idx < 0 {
		return content
	}
	rest := content[idx+len(header):]
	end := strings.Index(rest, "\n[")
	if end < 0 {
		return content[:idx] + replacement
	}
	return content[:idx] + replacement + rest[end+1:]
}

func resolveBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
		if err == nil && filepath.IsAbs(exe) {
			return exe, nil
		}
	}
	return exec.LookPath("peek-mcp")
}

// writeConfig marshals a map as indented JSON and writes it to path,
// creating parent directories as needed.
func writeConfig(path string, m map[string]any) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// expandHome replaces a leading ~ or $HOME with the actual home directory.
func expandHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	if strings.HasPrefix(path, "$HOME/") {
		return filepath.Join(home, path[6:])
	}
	if path == "$HOME" {
		return home
	}
	return path
}
