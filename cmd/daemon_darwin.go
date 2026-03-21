// cmd/daemon_darwin.go
//go:build darwin

package cmd

import "github.com/anttimattila/lab/internal/daemon"

func daemonInstall(binary, interval string) error {
	return daemon.Install(binary, interval)
}

func daemonUninstall() error {
	return daemon.Uninstall()
}
