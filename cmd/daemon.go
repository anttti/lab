// cmd/daemon.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anttimattila/lab/internal/daemon"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the lab background sync daemon",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the background sync daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer database.Close()

		interval, err := database.GetConfig("sync_interval")
		if err != nil {
			return fmt.Errorf("get sync_interval: %w", err)
		}
		if interval == "" {
			interval = "5m"
		}

		labBinary, err := labBinaryPath()
		if err != nil {
			return fmt.Errorf("find lab binary: %w", err)
		}

		pid, err := daemon.Start(labBinary, dataDir(), interval)
		if err != nil {
			return err
		}
		fmt.Printf("Daemon started (pid %d)\n", pid)
		return nil
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background sync daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := daemon.Stop(dataDir()); err != nil {
			return err
		}
		fmt.Println("Daemon stopped.")
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the status of the background sync daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		running, pid, err := daemon.Status(dataDir())
		if err != nil {
			return err
		}
		if running {
			fmt.Printf("Daemon is running (pid %d)\n", pid)
		} else {
			fmt.Println("Daemon is not running.")
		}
		return nil
	},
}

var daemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the lab sync launchd agent (macOS only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer database.Close()

		interval, err := database.GetConfig("sync_interval")
		if err != nil {
			return fmt.Errorf("get sync_interval: %w", err)
		}
		if interval == "" {
			interval = "5m"
		}

		labBinary, err := labBinaryPath()
		if err != nil {
			return fmt.Errorf("find lab binary: %w", err)
		}

		return daemonInstall(labBinary, interval)
	},
}

var daemonUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the lab sync launchd agent (macOS only)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemonUninstall()
	},
}

// labBinaryPath returns the absolute path of the running lab executable.
func labBinaryPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func init() {
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonInstallCmd)
	daemonCmd.AddCommand(daemonUninstallCmd)
	rootCmd.AddCommand(daemonCmd)
}
