// cmd/root.go
package cmd

import (
	"os"
	"path/filepath"

	"github.com/anttimattila/lab/internal/db"
	"github.com/anttimattila/lab/internal/glab"
	gosync "github.com/anttimattila/lab/internal/sync"
	"github.com/anttimattila/lab/internal/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "lab",
	Short: "GitLab merge request TUI",
	Long:  "A TUI for managing GitLab merge requests and dispatching comments to Claude Code.",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		client := glab.New()
		engine := gosync.New(database, client)

		return tui.Run(database, engine)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func dataDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "lab")
	os.MkdirAll(dir, 0755)
	return dir
}

func openDB() (*db.Database, error) {
	return db.Open(dataDir())
}
