package sync

import (
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"lab/internal/db"
	"lab/internal/glab"
)

// newTestEngine creates a sync Engine with output silenced for tests.
func newTestEngine(database *db.Database, mock GlabClient) *Engine {
	e := New(database, mock)
	e.SetOutput(io.Discard)
	return e
}

// mockGlab implements GlabClient for testing.
type mockGlab struct {
	mrs          []glab.MRListItem
	discussions  map[int][]glab.Discussion
	pipelines    map[int]string
	fileContents map[string]string // key: "filePath@ref"
}

func (m *mockGlab) ListMRs(repoURL string) ([]glab.MRListItem, error) {
	return m.mrs, nil
}

func (m *mockGlab) ListDiscussions(repoURL string, projectID int64, mrIID int) ([]glab.Discussion, error) {
	return m.discussions[mrIID], nil
}

func (m *mockGlab) GetMRPipeline(repoURL string, projectID int64, mrIID int) (string, error) {
	return m.pipelines[mrIID], nil
}

func (m *mockGlab) GetFileContent(repoURL string, projectID int64, filePath, ref string) (string, error) {
	if m.fileContents != nil {
		if content, ok := m.fileContents[filePath+"@"+ref]; ok {
			return content, nil
		}
	}
	return "", fmt.Errorf("file not found")
}

// testSyncDB opens an in-memory SQLite database suitable for testing.
func testSyncDB(t *testing.T) *db.Database {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(dir)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// testRepo inserts a repo into the DB and also creates a temp directory for
// its path so the disk-existence check in SyncRepo passes.
func testRepo(t *testing.T, database *db.Database) *db.Repo {
	t.Helper()
	dir := t.TempDir()
	repo, err := database.AddRepo(dir, "https://gitlab.example.com/owner/repo", "testrepo")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	// Set a non-zero project_id so API endpoints can be constructed.
	if err := database.UpdateRepoProjectID(repo.ID, 42); err != nil {
		t.Fatalf("UpdateRepoProjectID: %v", err)
	}
	repo.ProjectID = 42
	return repo
}

// TestSyncRepo_CreatesMRs verifies that a sync creates MRs and their comments.
func TestSyncRepo_CreatesMRs(t *testing.T) {
	database := testSyncDB(t)
	repo := testRepo(t, database)

	noteType := "DiffNote"
	newLine := 10

	mock := &mockGlab{
		mrs: []glab.MRListItem{
			{
				ID:           1,
				IID:          1,
				ProjectID:    42,
				Title:        "My first MR",
				State:        "opened",
				SourceBranch: "feature/x",
				TargetBranch: "main",
				WebURL:       "https://gitlab.example.com/owner/repo/-/merge_requests/1",
				UpdatedAt:    time.Now().Add(-time.Hour),
				Author:       glab.Author{Username: "alice"},
				Labels:       []string{"bug", "review"},
			},
		},
		discussions: map[int][]glab.Discussion{
			1: {
				{
					ID:             "abc123",
					IndividualNote: false,
					Notes: []glab.Note{
						{
							ID:        101,
							Type:      &noteType,
							Body:      "Please fix this",
							Author:    glab.Author{Username: "bob"},
							CreatedAt: time.Now().Add(-30 * time.Minute),
							System:    false,
							Resolvable: true,
							Resolved:  false,
							Position: &glab.Position{
								NewPath: "main.go",
								NewLine: &newLine,
								HeadSHA: "abc123def",
							},
						},
					},
				},
			},
		},
		pipelines: map[int]string{1: "success"},
		fileContents: map[string]string{
			"main.go@abc123def": "package main\n\nimport \"fmt\"\n\nfunc init() {\n}\n\nfunc hello() {\n}\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n",
		},
	}

	engine := newTestEngine(database, mock)
	if err := engine.SyncRepo(repo); err != nil {
		t.Fatalf("SyncRepo: %v", err)
	}

	// Verify MR was created.
	mrs, err := database.ListMRs(db.MRFilter{RepoID: &repo.ID})
	if err != nil {
		t.Fatalf("ListMRs: %v", err)
	}
	if len(mrs) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(mrs))
	}
	mr := mrs[0]
	if mr.IID != 1 {
		t.Errorf("IID: want 1, got %d", mr.IID)
	}
	if mr.Title != "My first MR" {
		t.Errorf("Title: want %q, got %q", "My first MR", mr.Title)
	}
	if mr.Author != "alice" {
		t.Errorf("Author: want %q, got %q", "alice", mr.Author)
	}
	if mr.PipelineStatus == nil || *mr.PipelineStatus != "success" {
		t.Errorf("PipelineStatus: want %q, got %v", "success", mr.PipelineStatus)
	}

	// Verify labels.
	labels, err := database.GetMRLabels(mr.ID)
	if err != nil {
		t.Fatalf("GetMRLabels: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}

	// Verify comment was created.
	comments, err := database.ListComments(mr.ID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	c := comments[0]
	if c.NoteID != 101 {
		t.Errorf("NoteID: want 101, got %d", c.NoteID)
	}
	if c.Author != "bob" {
		t.Errorf("Comment Author: want %q, got %q", "bob", c.Author)
	}
	if c.Body != "Please fix this" {
		t.Errorf("Body: want %q, got %q", "Please fix this", c.Body)
	}
	if c.FilePath == nil || *c.FilePath != "main.go" {
		t.Errorf("FilePath: want %q, got %v", "main.go", c.FilePath)
	}
	if c.NewLine == nil || *c.NewLine != 10 {
		t.Errorf("NewLine: want 10, got %v", c.NewLine)
	}
	if c.Resolved {
		t.Error("Resolved: want false, got true")
	}
	if c.DiffHunk == "" {
		t.Error("DiffHunk: expected non-empty diff hunk")
	}
}

// TestSyncRepo_DeletesStaleMRs verifies that MRs no longer returned by glab
// are removed from the database on subsequent sync.
func TestSyncRepo_DeletesStaleMRs(t *testing.T) {
	database := testSyncDB(t)
	repo := testRepo(t, database)

	now := time.Now()

	mr1 := glab.MRListItem{
		ID: 1, IID: 1, ProjectID: 42,
		Title: "MR one", State: "opened",
		UpdatedAt: now.Add(-2 * time.Hour),
		Author:    glab.Author{Username: "alice"},
	}
	mr2 := glab.MRListItem{
		ID: 2, IID: 2, ProjectID: 42,
		Title: "MR two", State: "opened",
		UpdatedAt: now.Add(-2 * time.Hour),
		Author:    glab.Author{Username: "bob"},
	}

	mock := &mockGlab{
		mrs:         []glab.MRListItem{mr1, mr2},
		discussions: map[int][]glab.Discussion{},
		pipelines:   map[int]string{},
	}

	engine := newTestEngine(database, mock)

	// First sync: both MRs present.
	if err := engine.SyncRepo(repo); err != nil {
		t.Fatalf("first SyncRepo: %v", err)
	}

	mrs, err := database.ListMRs(db.MRFilter{RepoID: &repo.ID})
	if err != nil {
		t.Fatalf("ListMRs after first sync: %v", err)
	}
	if len(mrs) != 2 {
		t.Fatalf("expected 2 MRs after first sync, got %d", len(mrs))
	}

	// Second sync: only MR 1 remains.
	mock.mrs = []glab.MRListItem{mr1}
	if err := engine.SyncRepo(repo); err != nil {
		t.Fatalf("second SyncRepo: %v", err)
	}

	mrs, err = database.ListMRs(db.MRFilter{RepoID: &repo.ID})
	if err != nil {
		t.Fatalf("ListMRs after second sync: %v", err)
	}
	if len(mrs) != 1 {
		t.Fatalf("expected 1 MR after second sync, got %d", len(mrs))
	}
	if mrs[0].IID != 1 {
		t.Errorf("expected surviving MR to have IID=1, got %d", mrs[0].IID)
	}
}

// TestSyncRepo_SkipsMissingPath verifies that SyncRepo skips repos whose
// path does not exist on disk, without returning an error.
func TestSyncRepo_SkipsMissingPath(t *testing.T) {
	database := testSyncDB(t)
	repo, err := database.AddRepo("/nonexistent/path/that/does/not/exist", "https://gitlab.example.com/x/y", "ghost")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	mock := &mockGlab{
		mrs:         []glab.MRListItem{},
		discussions: map[int][]glab.Discussion{},
		pipelines:   map[int]string{},
	}

	engine := newTestEngine(database, mock)
	if err := engine.SyncRepo(repo); err != nil {
		t.Errorf("SyncRepo with missing path should not error, got: %v", err)
	}
}

// TestSyncRepo_SystemNotesFiltered verifies that system notes are not stored
// as comments.
func TestSyncRepo_SystemNotesFiltered(t *testing.T) {
	database := testSyncDB(t)
	repo := testRepo(t, database)

	mock := &mockGlab{
		mrs: []glab.MRListItem{
			{
				ID: 1, IID: 1, ProjectID: 42,
				Title:     "MR with system notes",
				State:     "opened",
				UpdatedAt: time.Now().Add(-time.Hour),
				Author:    glab.Author{Username: "alice"},
			},
		},
		discussions: map[int][]glab.Discussion{
			1: {
				{
					ID: "sys1",
					Notes: []glab.Note{
						{
							ID:        200,
							Body:      "assigned to @alice",
							Author:    glab.Author{Username: "gitlab-bot"},
							CreatedAt: time.Now(),
							System:    true, // should be filtered
						},
						{
							ID:        201,
							Body:      "Real comment",
							Author:    glab.Author{Username: "carol"},
							CreatedAt: time.Now(),
							System:    false,
						},
					},
				},
			},
		},
		pipelines: map[int]string{},
	}

	engine := newTestEngine(database, mock)
	if err := engine.SyncRepo(repo); err != nil {
		t.Fatalf("SyncRepo: %v", err)
	}

	mrs, _ := database.ListMRs(db.MRFilter{RepoID: &repo.ID})
	comments, err := database.ListComments(mrs[0].ID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 non-system comment, got %d", len(comments))
	}
	if comments[0].NoteID != 201 {
		t.Errorf("expected note 201, got %d", comments[0].NoteID)
	}
}

// TestSyncRepo_PathExists verifies disk path check uses os.Stat correctly.
func TestSyncRepo_PathExists(t *testing.T) {
	// os.Stat on a temp dir created by the test harness should always succeed.
	dir := t.TempDir()
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("os.Stat on TempDir: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected TempDir to be a directory")
	}
}
