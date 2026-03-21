// cmd/add.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anttimattila/lab/internal/glab"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <local-path>",
	Short: "Register a local GitLab repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := glab.New()
		if err := client.CheckInstalled(); err != nil {
			return fmt.Errorf("glab is required: %w", err)
		}

		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}

		// Validate it's a git repo
		if _, err := os.Stat(filepath.Join(absPath, ".git")); os.IsNotExist(err) {
			return fmt.Errorf("%q is not a git repository (no .git directory found)", absPath)
		}

		gitlabURL, err := client.GetGitLabURL(absPath)
		if err != nil {
			return fmt.Errorf("get GitLab URL: %w", err)
		}

		database, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer database.Close()

		// Derive repo name from directory name
		name := filepath.Base(absPath)

		repo, err := database.AddRepo(absPath, gitlabURL, name)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") ||
				strings.Contains(err.Error(), "unique constraint") {
				return fmt.Errorf("repo %q is already registered", absPath)
			}
			return fmt.Errorf("add repo: %w", err)
		}

		fmt.Printf("Added repo %q (id=%d, url=%s)\n", repo.Name, repo.ID, repo.GitLabURL)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}
