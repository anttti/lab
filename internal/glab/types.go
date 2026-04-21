package glab

import "time"

type MRListItem struct {
	ID           int         `json:"id"`
	IID          int         `json:"iid"`
	ProjectID    int64       `json:"project_id"`
	Title        string      `json:"title"`
	State        string      `json:"state"`
	SourceBranch string      `json:"source_branch"`
	TargetBranch string      `json:"target_branch"`
	WebURL       string      `json:"web_url"`
	Draft        bool        `json:"draft"`
	UpdatedAt    time.Time   `json:"updated_at"`
	Author       Author      `json:"author"`
	Labels       []string    `json:"labels"`
	Reviewers    []Reviewer  `json:"reviewers"`
	HeadPipeline *Pipeline   `json:"head_pipeline"`
}

type Author struct {
	Username string `json:"username"`
}

// Reviewer captures the minimal reviewer info returned by GitLab's MR
// endpoints. ReviewState is a GitLab 15+ field ("unreviewed", "reviewed",
// "requested_changes", "approved", "unapproved"); it may be empty on
// older instances.
type Reviewer struct {
	Username    string `json:"username"`
	ReviewState string `json:"reviewer_state"`
}

type Pipeline struct {
	Status string `json:"status"`
}

type Discussion struct {
	ID             string `json:"id"`
	IndividualNote bool   `json:"individual_note"`
	Notes          []Note `json:"notes"`
}

type Note struct {
	ID         int       `json:"id"`
	Type       *string   `json:"type"`
	Body       string    `json:"body"`
	Author     Author    `json:"author"`
	CreatedAt  time.Time `json:"created_at"`
	System     bool      `json:"system"`
	Resolvable bool      `json:"resolvable"`
	Resolved   bool      `json:"resolved"`
	Position   *Position `json:"position"`
}

type Position struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
	OldLine *int   `json:"old_line"`
	NewLine *int   `json:"new_line"`
	HeadSHA string `json:"head_sha"`
}
