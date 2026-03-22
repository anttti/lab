package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// ErrDirtyWorktree is returned when the repo has uncommitted changes.
var ErrDirtyWorktree = fmt.Errorf("repository has uncommitted changes")

// IsClean returns nil if the repo has no uncommitted changes, or
// ErrDirtyWorktree if it does.
func IsClean(repoPath string) error {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if len(strings.TrimSpace(string(out))) > 0 {
		return ErrDirtyWorktree
	}
	return nil
}

// CurrentBranch returns the current branch name of the repo.
func CurrentBranch(repoPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Checkout switches the repo to the given branch. It first fetches origin
// to ensure the branch is available locally.
func Checkout(repoPath, branch string) error {
	// Fetch to make sure the branch exists locally.
	fetch := exec.Command("git", "fetch", "origin", branch)
	fetch.Dir = repoPath
	_ = fetch.Run() // best-effort; branch may already exist locally

	cmd := exec.Command("git", "checkout", branch)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout %s: %s", branch, strings.TrimSpace(string(out)))
	}
	return nil
}
