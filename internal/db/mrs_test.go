package db

import (
	"testing"
	"time"
)

// strPtr is a helper to get a *string from a literal.
func strPtr(s string) *string { return &s }

// intPtr is a helper to get a *int from a literal.
func intPtr(i int) *int { return &i }

// insertTestRepo is a helper that adds a repo and fails the test on error.
func insertTestRepo(t *testing.T, db *Database) *Repo {
	t.Helper()
	r, err := db.AddRepo("/test/repo", "https://gitlab.com/test/repo", "test-repo")
	if err != nil {
		t.Fatalf("insertTestRepo: %v", err)
	}
	return r
}

// baseMR returns a minimal MergeRequest for a given repo.
func baseMR(repoID int64, iid int) *MergeRequest {
	return &MergeRequest{
		RepoID:       repoID,
		IID:          iid,
		Title:        "Test MR",
		Author:       "alice",
		State:        "opened",
		SourceBranch: "feature/x",
		TargetBranch: "main",
		WebURL:       "https://gitlab.com/test/repo/-/merge_requests/1",
		UpdatedAt:    time.Now().UTC().Truncate(time.Second),
	}
}

func TestUpsertMR(t *testing.T) {
	db := testDB(t)
	repo := insertTestRepo(t, db)

	mr := baseMR(repo.ID, 1)
	if err := db.UpsertMR(mr); err != nil {
		t.Fatalf("UpsertMR insert: %v", err)
	}
	if mr.ID == 0 {
		t.Error("expected mr.ID to be populated after insert")
	}

	// Update title and upsert again
	mr.Title = "Updated Title"
	if err := db.UpsertMR(mr); err != nil {
		t.Fatalf("UpsertMR update: %v", err)
	}

	got, err := db.GetMR(mr.ID)
	if err != nil {
		t.Fatalf("GetMR: %v", err)
	}
	if got.Title != "Updated Title" {
		t.Errorf("Title = %q, want %q", got.Title, "Updated Title")
	}
}

func TestUpsertMRLabels(t *testing.T) {
	db := testDB(t)
	repo := insertTestRepo(t, db)

	mr := baseMR(repo.ID, 1)
	if err := db.UpsertMR(mr); err != nil {
		t.Fatalf("UpsertMR: %v", err)
	}

	labels := []string{"bug", "enhancement"}
	if err := db.SetMRLabels(mr.ID, labels); err != nil {
		t.Fatalf("SetMRLabels: %v", err)
	}

	got, err := db.GetMRLabels(mr.ID)
	if err != nil {
		t.Fatalf("GetMRLabels: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 labels, got %d: %v", len(got), got)
	}

	// Replace labels
	if err := db.SetMRLabels(mr.ID, []string{"wip"}); err != nil {
		t.Fatalf("SetMRLabels replace: %v", err)
	}
	got, _ = db.GetMRLabels(mr.ID)
	if len(got) != 1 || got[0] != "wip" {
		t.Errorf("expected [wip], got %v", got)
	}
}

func TestListMRs_FilterByRepo(t *testing.T) {
	db := testDB(t)
	repo1 := insertTestRepo(t, db)
	repo2, err := db.AddRepo("/test/repo2", "https://gitlab.com/test/repo2", "repo2")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	mr1 := baseMR(repo1.ID, 1)
	mr2 := baseMR(repo2.ID, 1)
	_ = db.UpsertMR(mr1)
	_ = db.UpsertMR(mr2)

	mrs, err := db.ListMRs(MRFilter{RepoID: &repo1.ID})
	if err != nil {
		t.Fatalf("ListMRs: %v", err)
	}
	if len(mrs) != 1 || mrs[0].RepoID != repo1.ID {
		t.Errorf("expected 1 MR for repo1, got %d", len(mrs))
	}
}

func TestListMRs_FilterByAuthor(t *testing.T) {
	db := testDB(t)
	repo := insertTestRepo(t, db)

	mr1 := baseMR(repo.ID, 1)
	mr1.Author = "alice"
	mr2 := baseMR(repo.ID, 2)
	mr2.Author = "bob"
	_ = db.UpsertMR(mr1)
	_ = db.UpsertMR(mr2)

	author := "alice"
	mrs, err := db.ListMRs(MRFilter{Author: &author})
	if err != nil {
		t.Fatalf("ListMRs: %v", err)
	}
	if len(mrs) != 1 || mrs[0].Author != "alice" {
		t.Errorf("expected 1 MR by alice, got %d", len(mrs))
	}
}

func TestListMRs_FilterByLabels(t *testing.T) {
	db := testDB(t)
	repo := insertTestRepo(t, db)

	mr1 := baseMR(repo.ID, 1)
	mr2 := baseMR(repo.ID, 2)
	_ = db.UpsertMR(mr1)
	_ = db.UpsertMR(mr2)
	_ = db.SetMRLabels(mr1.ID, []string{"bug", "urgent"})
	_ = db.SetMRLabels(mr2.ID, []string{"feature"})

	mrs, err := db.ListMRs(MRFilter{Labels: []string{"bug"}})
	if err != nil {
		t.Fatalf("ListMRs: %v", err)
	}
	if len(mrs) != 1 || mrs[0].ID != mr1.ID {
		t.Errorf("expected 1 MR with label 'bug', got %d", len(mrs))
	}
}

func TestAllLabels(t *testing.T) {
	db := testDB(t)
	repo := insertTestRepo(t, db)

	mr1 := baseMR(repo.ID, 1)
	mr2 := baseMR(repo.ID, 2)
	_ = db.UpsertMR(mr1)
	_ = db.UpsertMR(mr2)
	_ = db.SetMRLabels(mr1.ID, []string{"bug", "urgent"})
	_ = db.SetMRLabels(mr2.ID, []string{"bug", "feature"}) // "bug" appears in both

	labels, err := db.AllLabels()
	if err != nil {
		t.Fatalf("AllLabels: %v", err)
	}
	// Should have 3 distinct labels: bug, feature, urgent
	if len(labels) != 3 {
		t.Errorf("expected 3 distinct labels, got %d: %v", len(labels), labels)
	}
}

func TestDeleteStaleMRs(t *testing.T) {
	db := testDB(t)
	repo := insertTestRepo(t, db)

	mr1 := baseMR(repo.ID, 1)
	mr2 := baseMR(repo.ID, 2)
	mr3 := baseMR(repo.ID, 3)
	_ = db.UpsertMR(mr1)
	_ = db.UpsertMR(mr2)
	_ = db.UpsertMR(mr3)

	// Keep IIDs 1 and 3; stale is IID 2
	if err := db.DeleteStaleMRs(repo.ID, []int{1, 3}); err != nil {
		t.Fatalf("DeleteStaleMRs: %v", err)
	}

	mrs, err := db.ListMRs(MRFilter{RepoID: &repo.ID})
	if err != nil {
		t.Fatalf("ListMRs: %v", err)
	}
	if len(mrs) != 2 {
		t.Errorf("expected 2 MRs after stale deletion, got %d", len(mrs))
	}
	for _, mr := range mrs {
		if mr.IID == 2 {
			t.Error("stale MR iid=2 was not deleted")
		}
	}
}
