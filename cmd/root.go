package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "peek-mcp",
	Short: "MCP server for peeking at Claude Code sessions",
	Long:  `peek-mcp watches Claude Code and Codex session files and exposes them via the MCP protocol.`,
	Run:   runSetup,
}

func Execute() error {
	return rootCmd.Execute()
}
