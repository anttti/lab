// cmd/sync.go
package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lab/internal/glab"
	"lab/internal/sync"
	"github.com/spf13/cobra"
)

var (
	syncLoop     bool
	syncInterval time.Duration
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync GitLab data for all registered repositories",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer database.Close()

		client := glab.New()
		engine := sync.New(database, client)

		runSync := func() error {
			if err := engine.SyncAll(); err != nil {
				return fmt.Errorf("sync: %w", err)
			}
			fmt.Println("Done.")
			return nil
		}

		if !syncLoop {
			return runSync()
		}

		// Loop mode: handle signals gracefully
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

		// Run immediately, then on each tick
		if err := runSync(); err != nil {
			log.Printf("sync error: %v", err)
		}

		ticker := time.NewTicker(syncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := runSync(); err != nil {
					log.Printf("sync error: %v", err)
				}
			case sig := <-sigs:
				log.Printf("Received shutdown signal (%s), exiting", sig)
				return nil
			}
		}
	},
}

func init() {
	syncCmd.Flags().BoolVar(&syncLoop, "loop", false, "Run sync in a loop")
	syncCmd.Flags().DurationVar(&syncInterval, "interval", 5*time.Minute, "Interval between syncs when using --loop")
	rootCmd.AddCommand(syncCmd)
}
