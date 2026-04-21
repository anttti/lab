// cmd/root.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"lab/internal/db"
	"lab/internal/glab"
	gosync "lab/internal/sync"
	"lab/internal/tui"
	"github.com/spf13/cobra"
)

// installInterval is the sync interval set by `lab --install`.
const installInterval = "10m"

var installFlag bool

var rootCmd = &cobra.Command{
	Use:   "lab",
	Short: "GitLab merge request TUI",
	Long:  "A TUI for managing GitLab merge requests and dispatching comments to Claude Code.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if installFlag {
			return runInstall()
		}

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

// runInstall persists a 10-minute sync_interval and installs the background
// sync daemon as a launchd agent. The running daemon picks up the interval
// from the plist that is written here.
func runInstall() error {
	database, err := openDB()
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	if err := database.SetConfig("sync_interval", installInterval); err != nil {
		return fmt.Errorf("set sync_interval: %w", err)
	}

	binary, err := labBinaryPath()
	if err != nil {
		return fmt.Errorf("find lab binary: %w", err)
	}

	if err := daemonInstall(binary, installInterval); err != nil {
		return err
	}

	fmt.Printf("Sync interval set to %s.\n", installInterval)
	fmt.Println("Install terminal-notifier (brew install terminal-notifier) to receive desktop notifications when your MRs update.")
	return nil
}

func init() {
	rootCmd.Flags().BoolVar(&installFlag, "install", false, "Install the background sync daemon (syncs every 10 minutes)")
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
