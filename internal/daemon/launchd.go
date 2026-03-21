// internal/daemon/launchd.go
//go:build darwin

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.lab.sync</string>

    <key>ProgramArguments</key>
    <array>
        <string>{{.Binary}}</string>
        <string>sync</string>
        <string>--loop</string>
        <string>--interval</string>
        <string>{{.Interval}}</string>
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>{{.LogDir}}/daemon.log</string>

    <key>StandardErrorPath</key>
    <string>{{.LogDir}}/daemon.log</string>
</dict>
</plist>
`

type plistData struct {
	Binary   string
	Interval string
	LogDir   string
}

// plistPath returns the path to the launchd plist file.
func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.lab.sync.plist")
}

// GeneratePlist renders the launchd plist XML for the given binary and interval.
func GeneratePlist(binary, interval string) string {
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".config", "lab")

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		// Template is a compile-time constant; this should never fail.
		panic(fmt.Sprintf("parse plist template: %v", err))
	}

	var sb strings.Builder
	if err := tmpl.Execute(&sb, plistData{
		Binary:   binary,
		Interval: interval,
		LogDir:   logDir,
	}); err != nil {
		panic(fmt.Sprintf("execute plist template: %v", err))
	}

	return sb.String()
}

// Install writes the launchd plist and loads it with launchctl.
func Install(binary, interval string) error {
	pp := plistPath()

	// Ensure the LaunchAgents directory exists.
	if err := os.MkdirAll(filepath.Dir(pp), 0755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}

	plist := GeneratePlist(binary, interval)
	if err := os.WriteFile(pp, []byte(plist), 0644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	out, err := exec.Command("launchctl", "load", pp).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl load: %w\n%s", err, string(out))
	}

	fmt.Printf("Installed and loaded launchd agent: %s\n", pp)
	return nil
}

// Uninstall unloads the launchd agent and removes the plist file.
func Uninstall() error {
	pp := plistPath()

	out, err := exec.Command("launchctl", "unload", pp).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl unload: %w\n%s", err, string(out))
	}

	if err := os.Remove(pp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plist: %w", err)
	}

	fmt.Printf("Uninstalled launchd agent: %s\n", pp)
	return nil
}
