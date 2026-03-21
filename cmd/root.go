// cmd/root.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "lab",
	Short: "GitLab merge request TUI",
	Long:  "A TUI for managing GitLab merge requests and dispatching comments to Claude Code.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("TUI not yet implemented")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
