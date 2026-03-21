package claude

import (
	"strings"
	"testing"

	"github.com/anttimattila/lab/internal/db"
)

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestBuildPrompt(t *testing.T) {
	filePath := "pkg/foo/bar.go"
	newLine := 42

	thread := &db.Thread{
		DiscussionID: "abc123",
		FilePath:     strPtr(filePath),
		NewLine:      intPtr(newLine),
		Comments: []db.Comment{
			{Author: "alice", Body: "This is a bug"},
			{Author: "bob", Body: "Agreed, needs fixing"},
		},
	}

	repoPath := "/home/user/myrepo"
	prompt := BuildPrompt(thread, repoPath)

	if !strings.Contains(prompt, "File: pkg/foo/bar.go (line 42)") {
		t.Errorf("expected file path with line, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Full path: /home/user/myrepo/pkg/foo/bar.go") {
		t.Errorf("expected full path, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "@alice:") {
		t.Errorf("expected alice comment, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "@bob:") {
		t.Errorf("expected bob comment, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "This is a bug") {
		t.Errorf("expected comment body, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Verify this issue exists and then fix it.") {
		t.Errorf("expected instruction, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "--- Comment thread ---") {
		t.Errorf("expected thread delimiter, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "--- End thread ---") {
		t.Errorf("expected end delimiter, got:\n%s", prompt)
	}
}

func TestBuildPrompt_GeneralComment(t *testing.T) {
	thread := &db.Thread{
		DiscussionID: "xyz789",
		FilePath:     nil,
		Comments: []db.Comment{
			{Author: "carol", Body: "Please update the docs"},
		},
	}

	repoPath := "/home/user/myrepo"
	prompt := BuildPrompt(thread, repoPath)

	if strings.Contains(prompt, "File:") {
		t.Errorf("expected no File: line for general comment, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Full path:") {
		t.Errorf("expected no Full path: line for general comment, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "@carol:") {
		t.Errorf("expected carol comment, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Verify this issue exists and then fix it.") {
		t.Errorf("expected instruction, got:\n%s", prompt)
	}
}
