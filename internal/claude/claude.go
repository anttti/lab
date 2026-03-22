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

// ClaudeCmd returns an exec.Cmd that runs claude with the given prompt
// in the specified repo directory.
func ClaudeCmd(prompt, repoPath string) (*exec.Cmd, error) {
	if _, err := exec.LookPath("claude"); err != nil {
		return nil, fmt.Errorf("claude not found on PATH: %w", err)
	}

	cmd := exec.Command("claude", prompt)
	cmd.Dir = repoPath
	return cmd, nil
}
