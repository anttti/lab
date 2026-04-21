package glab

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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

// MRDetail holds pipeline, approval, reviewer and state info from the MR
// detail endpoint.
type MRDetail struct {
	PipelineStatus string
	Approved       bool
	State          string
	Reviewers      []Reviewer
}

func (c *Client) GetMRDetail(repoURL string, projectID int64, mrIID int) (MRDetail, error) {
	endpoint := fmt.Sprintf("projects/%d/merge_requests/%d", projectID, mrIID)
	out, err := exec.Command("glab", "api", endpoint, "-R", repoURL).Output()
	if err != nil {
		return MRDetail{}, fmt.Errorf("glab api MR detail: %s", cmdError(err))
	}
	var detail struct {
		HeadPipeline *Pipeline  `json:"head_pipeline"`
		ApprovedBy   []approver `json:"approved_by"`
		State        string     `json:"state"`
		Reviewers    []Reviewer `json:"reviewers"`
	}
	if err := json.Unmarshal(out, &detail); err != nil {
		return MRDetail{}, fmt.Errorf("parse MR detail: %w", err)
	}
	var result MRDetail
	if detail.HeadPipeline != nil {
		result.PipelineStatus = detail.HeadPipeline.Status
	}
	result.Approved = len(detail.ApprovedBy) > 0
	result.State = detail.State
	result.Reviewers = detail.Reviewers
	return result, nil
}

// approver is a minimal struct for the approved_by array in the MR detail response.
type approver struct {
	User Author `json:"user"`
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

// GetFileContent fetches the raw content of a file at a specific ref from GitLab.
func (c *Client) GetFileContent(repoURL string, projectID int64, filePath, ref string) (string, error) {
	encoded := url.PathEscape(filePath)
	endpoint := fmt.Sprintf("projects/%d/repository/files/%s/raw?ref=%s", projectID, encoded, ref)
	out, err := exec.Command("glab", "api", endpoint, "-R", repoURL).Output()
	if err != nil {
		return "", fmt.Errorf("glab api file content: %s", cmdError(err))
	}
	return string(out), nil
}

// ExtractSnippet extracts a few lines of context around a target line from file content.
// Returns the snippet with up to contextLines above and below the target line.
func ExtractSnippet(content string, targetLine, contextLines int) string {
	lines := strings.Split(content, "\n")
	if targetLine < 1 || targetLine > len(lines) {
		return ""
	}

	start := targetLine - contextLines - 1
	if start < 0 {
		start = 0
	}
	end := targetLine + contextLines
	if end > len(lines) {
		end = len(lines)
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		sb.WriteString(fmt.Sprintf("%d\t%s\n", i+1, lines[i]))
	}
	return sb.String()
}

func (c *Client) GetGitLabURL(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("get git remote URL: %s", cmdError(err))
	}
	return strings.TrimSpace(string(out)), nil
}
