package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type setupFn func(*prompter, bool) error

var setupCmd = &cobra.Command{
	Use:               "setup",
	Short:             "Configure agents to use peek-mcp",
	Long:              `Write peek-mcp MCP server entries into agent configs. Interactive without flags; --claude/--codex select targets non-interactively.`,
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		claude, _ := flags.GetBool("claude")
		codex, _ := flags.GetBool("codex")
		controlServer, _ := flags.GetBool("control-server")

		if !claude && !codex {
			runSetup(cmd, args)
			return
		}

		p := autoPrompter()
		var steps []setupFn
		if claude {
			steps = append(steps, setupClaudeCode)
		}
		if codex {
			steps = append(steps, setupCodex)
		}
		for _, fn := range steps {
			if err := fn(p, controlServer); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

func init() {
	flags := setupCmd.Flags()
	flags.Bool("claude", false, "Configure Claude Code non-interactively")
	flags.Bool("codex", false, "Configure Codex CLI non-interactively")
	flags.Bool("control-server", true, "Enable the control server dashboard in the written config")

	rootCmd.AddCommand(setupCmd)
}

func runSetup(_ *cobra.Command, _ []string) {
	fi, _ := os.Stdin.Stat()
	if fi.Mode()&os.ModeCharDevice == 0 {
		fmt.Fprintln(os.Stderr, "stdin is not a terminal — run 'peek-mcp start' instead")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stdout, "peek-mcp %s — interactive setup\n\n", Version())

	p := newPrompter()
	choice := p.Choose("Which environment do you want to configure?", []string{
		"Claude Code     (~/.claude.json)",
		"Codex CLI       (~/.codex/config.toml)",
		"All",
		"Exit",
	}, 0)

	var steps []setupFn

	switch choice {
	case 0:
		steps = []setupFn{setupClaudeCode, setupTelemetry}
	case 1:
		steps = []setupFn{setupCodex}
	case 2:
		steps = []setupFn{setupClaudeCode, setupTelemetry, setupCodex}
	default:
		return
	}

	controlServer := p.Confirm("Enable the control server dashboard (http://127.0.0.1:42442)?", true)

	for i, fn := range steps {
		if i > 0 {
			fmt.Println()
		}
		if err := fn(p, controlServer); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("\nDone. Start the server with: peek-mcp start")
}

func mcpArgs(controlServer bool) []string {
	args := []string{"start", "--transport=stdio"}
	if !controlServer {
		args = append(args, "--control-port=0")
	}
	return args
}

func setupClaudeCode(p *prompter, controlServer bool) error {
	fmt.Println("Configuring peek-mcp for Claude Code...")

	binPath, err := resolveBinaryPath()
	if err != nil {
		return fmt.Errorf("cannot determine peek-mcp binary path: %w", err)
	}
	fmt.Printf("  Binary: %s\n", binPath)

	home, err2 := os.UserHomeDir()
	if err2 != nil {
		return fmt.Errorf("cannot determine home directory: %w", err2)
	}
	path := filepath.Join(home, ".claude.json")
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
		"type":    "stdio",
		"command": binPath,
		"args":    mcpArgs(controlServer),
		"env": map[string]any{
			"MAX_MCP_OUTPUT_TOKENS": "125000",
		},
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
	return nil
}

const (
	defaultMetricExportIntervalMs = "10000"
	defaultLogsExportIntervalMs   = "5000"
)

func setupTelemetry(p *prompter, controlServer bool) error {
	fmt.Println("Enabling Claude Code telemetry export to peek-mcp...")

	if !controlServer {
		fmt.Println("  Telemetry export stays disabled because the control server is disabled.")
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	path := filepath.Join(home, ".claude", "settings.json")
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

	env, _ := cfg["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	if !p.Confirm("  Enable telemetry export to peek?", true) {
		fmt.Println("  Skipped.")
		return nil
	}

	env["CLAUDE_CODE_ENABLE_TELEMETRY"] = "1"
	env["OTEL_METRICS_EXPORTER"] = "otlp"
	env["OTEL_LOGS_EXPORTER"] = "otlp"
	env["OTEL_LOG_TOOL_DETAILS"] = "1"
	env["OTEL_EXPORTER_OTLP_PROTOCOL"] = "http/json"
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = fmt.Sprintf("http://127.0.0.1:%d/otlp", controlPortBase)
	env["OTEL_METRIC_EXPORT_INTERVAL"] = defaultMetricExportIntervalMs
	env["OTEL_LOGS_EXPORT_INTERVAL"] = defaultLogsExportIntervalMs
	delete(env, "OTEL_EXPORTER_OTLP_HEADERS")
	cfg["env"] = env

	if err := writeConfig(path, cfg); err != nil {
		return err
	}
	fmt.Println("  ✓ Wrote telemetry config.")
	return nil
}

func setupCodex(p *prompter, controlServer bool) error {
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

	quoted := make([]string, 0, 3)
	for _, a := range mcpArgs(controlServer) {
		quoted = append(quoted, strconv.Quote(a))
	}
	block := fmt.Sprintf("tool_output_token_limit = 125000\n[mcp_servers.peek-mcp]\ncommand = %q\nargs = [%s]\n",
		binPath, strings.Join(quoted, ", "))

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
	for _, prefix := range []string{"$HOME/", "${HOME}/"} {
		if strings.HasPrefix(path, prefix) {
			return filepath.Join(home, path[len(prefix):])
		}
	}

	if path == "$HOME" || path == "${HOME}" {
		return home
	}
	return path
}
