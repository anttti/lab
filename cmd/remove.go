// cmd/remove.go
package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <local-path>",
	Short: "Unregister a local GitLab repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}

		database, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer database.Close()

		if err := database.RemoveRepo(absPath); err != nil {
			return err
		}

		fmt.Printf("Removed repo %q\n", absPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(removeCmd)
}
