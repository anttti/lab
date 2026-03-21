// internal/daemon/daemon.go
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// pidPath returns the path to the daemon PID file.
func pidPath(dataDir string) string {
	return filepath.Join(dataDir, "daemon.pid")
}

// logPath returns the path to the daemon log file.
func logPath(dataDir string) string {
	return filepath.Join(dataDir, "daemon.log")
}

// WritePID writes the given PID to path.
func WritePID(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
}

// ReadPID reads the PID from path.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read PID file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse PID: %w", err)
	}
	return pid, nil
}

// IsRunning returns true if the process with the given PID is running.
func IsRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// Start checks if a daemon is already running, then launches `lab sync --loop
// --interval <interval>` as a background process with a new session. It writes
// the PID file and returns the new process's PID.
func Start(labBinary, dataDir, interval string) (int, error) {
	pp := pidPath(dataDir)

	// Check if already running
	if pid, err := ReadPID(pp); err == nil {
		if IsRunning(pid) {
			return 0, fmt.Errorf("daemon is already running (pid %d)", pid)
		}
	}

	lp := logPath(dataDir)
	logFile, err := os.OpenFile(lp, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(labBinary, "sync", "--loop", "--interval", interval)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start daemon: %w", err)
	}

	pid := cmd.Process.Pid
	if err := WritePID(pp, pid); err != nil {
		// Try to kill the process since we can't track it
		_ = cmd.Process.Kill()
		return 0, fmt.Errorf("write PID: %w", err)
	}

	return pid, nil
}

// Stop reads the PID file, verifies the process is running, sends SIGTERM,
// and removes the PID file.
func Stop(dataDir string) error {
	pp := pidPath(dataDir)

	pid, err := ReadPID(pp)
	if err != nil {
		return fmt.Errorf("daemon is not running (no PID file)")
	}

	if !IsRunning(pid) {
		_ = os.Remove(pp)
		return fmt.Errorf("daemon is not running (stale PID file cleaned up)")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM to pid %d: %w", pid, err)
	}

	_ = os.Remove(pp)
	return nil
}

// Status returns whether the daemon is running and its PID.
func Status(dataDir string) (bool, int, error) {
	pp := pidPath(dataDir)

	pid, err := ReadPID(pp)
	if err != nil {
		return false, 0, nil
	}

	running := IsRunning(pid)
	return running, pid, nil
}
