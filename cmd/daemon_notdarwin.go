// cmd/daemon_notdarwin.go
//go:build !darwin

package cmd

import "fmt"

func daemonInstall(binary, interval string) error {
	return fmt.Errorf("launchd install is only supported on macOS")
}

func daemonUninstall() error {
	return fmt.Errorf("launchd uninstall is only supported on macOS")
}
