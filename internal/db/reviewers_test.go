package db

import "testing"

func TestSetAndGetMRReviewers(t *testing.T) {
	database := testDB(t)

	repo, err := database.AddRepo("/tmp/r", "https://gitlab.example.com/x/r", "r")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	mr := &MergeRequest{RepoID: repo.ID, IID: 1, Title: "t"}
	if err := database.UpsertMR(mr); err != nil {
		t.Fatalf("UpsertMR: %v", err)
	}

	want := []Reviewer{
		{Username: "alice", State: "reviewed"},
		{Username: "bob", State: "unreviewed"},
	}
	if err := database.SetMRReviewers(mr.ID, want); err != nil {
		t.Fatalf("SetMRReviewers: %v", err)
	}

	got, err := database.GetMRReviewers(mr.ID)
	if err != nil {
		t.Fatalf("GetMRReviewers: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("want %d reviewers, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d]: want %+v, got %+v", i, want[i], got[i])
		}
	}
}

// TestSetMRReviewers_Replaces overwrites previously stored reviewers.
func TestSetMRReviewers_Replaces(t *testing.T) {
	database := testDB(t)

	repo, _ := database.AddRepo("/tmp/r", "https://gitlab.example.com/x/r", "r")
	mr := &MergeRequest{RepoID: repo.ID, IID: 1}
	_ = database.UpsertMR(mr)

	_ = database.SetMRReviewers(mr.ID, []Reviewer{
		{Username: "alice"},
		{Username: "bob"},
	})
	if err := database.SetMRReviewers(mr.ID, []Reviewer{{Username: "carol"}}); err != nil {
		t.Fatalf("SetMRReviewers: %v", err)
	}

	got, _ := database.GetMRReviewers(mr.ID)
	if len(got) != 1 || got[0].Username != "carol" {
		t.Fatalf("want [carol], got %+v", got)
	}
}
