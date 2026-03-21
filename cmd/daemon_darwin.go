// cmd/daemon_darwin.go
//go:build darwin

package cmd

import "lab/internal/daemon"

func daemonInstall(binary, interval string) error {
	return daemon.Install(binary, interval)
}

func daemonUninstall() error {
	return daemon.Uninstall()
}
