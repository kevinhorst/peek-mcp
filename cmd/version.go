package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "1.0.5"

func Version() string {
	return version
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of peek-mcp",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(Version())
	},
}
