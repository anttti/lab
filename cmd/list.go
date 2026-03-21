// cmd/list.go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered repositories",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer database.Close()

		repos, err := database.ListRepos()
		if err != nil {
			return fmt.Errorf("list repos: %w", err)
		}

		if len(repos) == 0 {
			fmt.Println("No repos registered.")
			return nil
		}

		for _, r := range repos {
			lastSync := "never"
			if r.LastSyncedAt != nil {
				lastSync = r.LastSyncedAt.Format("2006-01-02 15:04:05")
			}
			fmt.Printf("%-30s  %s  (last sync: %s)\n", r.Name, r.Path, lastSync)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
