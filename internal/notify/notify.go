// Package notify sends desktop notifications via the terminal-notifier CLI
// on macOS. If terminal-notifier is not installed, notifications are silently
// dropped.
package notify

import (
	"os/exec"
)

// Notifier sends a desktop notification.
type Notifier interface {
	Notify(title, message, openURL string) error
}

// New returns a Notifier that uses terminal-notifier if available, or a no-op
// notifier otherwise.
func New() Notifier {
	path, err := exec.LookPath("terminal-notifier")
	if err != nil {
		return noop{}
	}
	return &terminalNotifier{bin: path}
}

type terminalNotifier struct {
	bin string
}

func (t *terminalNotifier) Notify(title, message, openURL string) error {
	args := []string{"-title", title, "-message", message, "-group", "com.lab.sync"}
	if openURL != "" {
		args = append(args, "-open", openURL)
	}
	return exec.Command(t.bin, args...).Run()
}

type noop struct{}

func (noop) Notify(string, string, string) error { return nil }
