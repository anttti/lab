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

// runInstall ensures lab.json exists with defaults (creating it if missing,
// or overwriting it if it's malformed) and installs the background sync
// daemon as a launchd agent using the interval from that file.
func runInstall() error {
	database, err := openDB()
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	dir := dataDir()
	path := config.Path(dir)
	cfg, loadErr := config.Load(dir)

	_, statErr := os.Stat(path)
	fileMissing := os.IsNotExist(statErr)

	if fileMissing || loadErr != nil {
		// Either no file yet, or it's malformed. Seed a fresh one from
		// any legacy DB config so the user has a well-documented file.
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
		if fileMissing {
			fmt.Printf("Created %s\n", path)
		} else {
			fmt.Printf("Overwrote malformed %s with defaults (%v)\n", path, loadErr)
		}
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
	fmt.Printf("Edit %s to customise which changes trigger notifications.\n", path)
	return nil
}

// loadEffectiveConfig returns the lab.json config with per-user values
// (username, sync_interval) falling back to legacy SQLite config when the
// JSON file doesn't set them. A non-nil error means lab.json was present
// but unreadable or malformed; the returned Config is still usable
// (defaults + DB fallbacks) so callers can log/notify and keep running.
func loadEffectiveConfig(database *db.Database) (config.Config, error) {
	cfg, loadErr := config.Load(dataDir())
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
	return cfg, loadErr
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
