// internal/daemon/daemon_test.go
package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadPID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")

	if err := WritePID(path, 12345); err != nil {
		t.Fatalf("WritePID: %v", err)
	}

	pid, err := ReadPID(path)
	if err != nil {
		t.Fatalf("ReadPID: %v", err)
	}
	if pid != 12345 {
		t.Errorf("expected PID 12345, got %d", pid)
	}
}

func TestReadPID_NoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.pid")

	_, err := ReadPID(path)
	if err == nil {
		t.Fatal("expected error when reading non-existent PID file, got nil")
	}
}

func TestIsRunning_NotRunning(t *testing.T) {
	// PID 0 is never a valid user process; on most Unix systems signaling it
	// is either an error or affects the process group — either way the process
	// itself is not "running" from our perspective.
	// Use a very large PID that is extremely unlikely to exist.
	if IsRunning(999999999) {
		t.Error("expected IsRunning(999999999) to return false")
	}
}

func TestIsRunning_CurrentProcess(t *testing.T) {
	pid := os.Getpid()
	if !IsRunning(pid) {
		t.Errorf("expected IsRunning(%d) (current process) to return true", pid)
	}
}
