package glab

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct{}

func New() *Client { return &Client{} }

func (c *Client) CheckInstalled() error {
	_, err := exec.LookPath("glab")
	if err != nil {
		return fmt.Errorf("glab not found on PATH: %w", err)
	}
	return nil
}

func (c *Client) ListMRs(repoURL string) ([]MRListItem, error) {
	cmd := exec.Command("glab", "mr", "list", "-R", repoURL, "-F", "json", "--per-page", "100")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("glab mr list: %s", cmdError(err))
	}
	var mrs []MRListItem
	if err := json.Unmarshal(out, &mrs); err != nil {
		return nil, fmt.Errorf("parse glab mr list output: %w", err)
	}
	return mrs, nil
}

func (c *Client) ListDiscussions(repoURL string, projectID int64, mrIID int) ([]Discussion, error) {
	endpoint := fmt.Sprintf("projects/%d/merge_requests/%d/discussions?per_page=100", projectID, mrIID)
	out, err := exec.Command("glab", "api", endpoint, "-R", repoURL).Output()
	if err != nil {
		return nil, fmt.Errorf("glab api discussions: %s", cmdError(err))
	}
	var discussions []Discussion
	if err := json.Unmarshal(out, &discussions); err != nil {
		return nil, fmt.Errorf("parse discussions: %w", err)
	}
	return discussions, nil
}

func (c *Client) GetMRPipeline(repoURL string, projectID int64, mrIID int) (string, error) {
	endpoint := fmt.Sprintf("projects/%d/merge_requests/%d", projectID, mrIID)
	out, err := exec.Command("glab", "api", endpoint, "-R", repoURL).Output()
	if err != nil {
		return "", fmt.Errorf("glab api MR detail: %s", cmdError(err))
	}
	var detail struct {
		HeadPipeline *Pipeline `json:"head_pipeline"`
	}
	if err := json.Unmarshal(out, &detail); err != nil {
		return "", fmt.Errorf("parse MR detail: %w", err)
	}
	if detail.HeadPipeline == nil {
		return "", nil
	}
	return detail.HeadPipeline.Status, nil
}

// cmdError extracts stderr from an exec.ExitError, falling back to err.Error().
func cmdError(err error) string {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
			return stderr
		}
	}
	return err.Error()
}

func (c *Client) GetGitLabURL(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("get git remote URL: %s", cmdError(err))
	}
	return strings.TrimSpace(string(out)), nil
}
