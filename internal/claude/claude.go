package claude

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"lab/internal/db"
)

// BuildPrompt constructs a prompt string from a thread and the repository path.
func BuildPrompt(thread *db.Thread, repoPath string) string {
	var sb strings.Builder

	if thread.FilePath != nil {
		line := 0
		if thread.NewLine != nil {
			line = *thread.NewLine
		} else if thread.OldLine != nil {
			line = *thread.OldLine
		}
		sb.WriteString(fmt.Sprintf("File: %s (line %d)\n", *thread.FilePath, line))
		sb.WriteString(fmt.Sprintf("Full path: %s\n", filepath.Join(repoPath, *thread.FilePath)))
	}

	sb.WriteString("--- Comment thread ---\n")
	for _, c := range thread.Comments {
		sb.WriteString(fmt.Sprintf("@%s:\n%s\n", c.Author, c.Body))
	}
	sb.WriteString("--- End thread ---\n")
	sb.WriteString("Verify this issue exists and then fix it.\n")

	return sb.String()
}

// WritePromptToTempFile creates a temp file named "lab-prompt-*.md", writes
// the prompt to it, and returns the file path.
func WritePromptToTempFile(prompt string) (string, error) {
	f, err := os.CreateTemp("", "lab-prompt-*.md")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(prompt); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}

	return f.Name(), nil
}

// LaunchInNewTerminal writes the prompt to a temp file and opens a new
// terminal window running claude with that prompt file.
func LaunchInNewTerminal(prompt, repoPath string) error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude not found on PATH: %w", err)
	}

	tmpFile, err := WritePromptToTempFile(prompt)
	if err != nil {
		return err
	}

	shellCmd := fmt.Sprintf("cd %s && claude --prompt-file %s; rm -f %s",
		repoPath, tmpFile, tmpFile)

	return openTerminalWindow(shellCmd)
}

// openTerminalWindow opens a new terminal window running shellCmd.
// It supports iTerm.app and falls back to Terminal.app via AppleScript.
func openTerminalWindow(shellCmd string) error {
	termProgram := os.Getenv("TERM_PROGRAM")

	var script string
	if termProgram == "iTerm.app" {
		script = fmt.Sprintf(`tell application "iTerm"
    create window with default profile command "bash -c %q"
end tell`, shellCmd)
	} else {
		script = fmt.Sprintf(`tell application "Terminal"
    do script "bash -c %q"
    activate
end tell`, shellCmd)
	}

	return exec.Command("osascript", "-e", script).Start()
}
