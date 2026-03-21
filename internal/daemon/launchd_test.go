// internal/daemon/launchd_test.go
//go:build darwin

package daemon

import (
	"strings"
	"testing"
)

func TestGeneratePlist(t *testing.T) {
	binary := "/usr/local/bin/lab"
	interval := "10m"

	plist := GeneratePlist(binary, interval)

	if !strings.Contains(plist, binary) {
		t.Errorf("plist does not contain binary path %q", binary)
	}

	if !strings.Contains(plist, "com.lab.sync") {
		t.Error("plist does not contain label com.lab.sync")
	}

	if !strings.Contains(plist, interval) {
		t.Errorf("plist does not contain interval %q", interval)
	}

	// Verify it's valid XML structure
	if !strings.Contains(plist, "<?xml") {
		t.Error("plist does not start with XML declaration")
	}

	if !strings.Contains(plist, "<key>ProgramArguments</key>") {
		t.Error("plist does not contain ProgramArguments key")
	}
}
