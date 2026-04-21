// cmd/root.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"lab/internal/config"
	"lab/internal/db"
	"lab/internal/glab"
	gosync "lab/internal/sync"
	"lab/internal/tui"
	"github.com/spf13/cobra"
)

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

// runInstall ensures lab.json exists with defaults (creating it if missing)
// and installs the background sync daemon as a launchd agent using the
// interval from that file.
func runInstall() error {
	database, err := openDB()
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	dir := dataDir()
	cfg, err := config.Load(dir)
	if err != nil {
		return fmt.Errorf("load lab.json: %w", err)
	}

	if _, err := os.Stat(config.Path(dir)); os.IsNotExist(err) {
		// Seed lab.json from any legacy DB config values so the user has
		// a well-documented file to edit.
		if cfg.Username == "" {
			if v, err := database.GetConfig("username"); err == nil && v != "" {
				cfg.Username = v
			}
		}
		if v, err := database.GetConfig("sync_interval"); err == nil && v != "" {
			cfg.SyncInterval = v
		}
		if err := config.Save(dir, cfg); err != nil {
			return fmt.Errorf("write lab.json: %w", err)
		}
		fmt.Printf("Created %s\n", config.Path(dir))
	}

	binary, err := labBinaryPath()
	if err != nil {
		return fmt.Errorf("find lab binary: %w", err)
	}

	if err := daemonInstall(binary, cfg.SyncInterval); err != nil {
		return err
	}

	fmt.Printf("Sync interval set to %s.\n", cfg.SyncInterval)
	fmt.Println("Install terminal-notifier (brew install terminal-notifier) to receive desktop notifications.")
	fmt.Printf("Edit %s to customise which changes trigger notifications.\n", config.Path(dir))
	return nil
}

// loadEffectiveConfig returns the lab.json config with per-user values
// (username, sync_interval) falling back to legacy SQLite config when the
// JSON file doesn't set them.
func loadEffectiveConfig(database *db.Database) (config.Config, error) {
	cfg, err := config.Load(dataDir())
	if err != nil {
		return cfg, fmt.Errorf("load lab.json: %w", err)
	}
	if cfg.Username == "" {
		if v, err := database.GetConfig("username"); err == nil {
			cfg.Username = v
		}
	}
	if cfg.SyncInterval == "" {
		if v, err := database.GetConfig("sync_interval"); err == nil && v != "" {
			cfg.SyncInterval = v
		}
	}
	return cfg, nil
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
