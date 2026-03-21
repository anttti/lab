package db

import (
	"testing"
	"time"
)

// insertTestMR inserts a minimal MR for use in comment tests.
func insertTestMR(t *testing.T, db *Database) *MergeRequest {
	t.Helper()
	repo := insertTestRepo(t, db)
	mr := baseMR(repo.ID, 1)
	if err := db.UpsertMR(mr); err != nil {
		t.Fatalf("insertTestMR: %v", err)
	}
	return mr
}

func baseComment(mrID int64, noteID int, discussionID string) *Comment {
	return &Comment{
		MRID:         mrID,
		DiscussionID: discussionID,
		NoteID:       noteID,
		Author:       "alice",
		Body:         "looks good",
		Resolved:     false,
		CreatedAt:    time.Now().UTC().Truncate(time.Second),
	}
}

func TestUpsertComment(t *testing.T) {
	db := testDB(t)
	mr := insertTestMR(t, db)

	c := baseComment(mr.ID, 100, "disc-1")
	if err := db.UpsertComment(c); err != nil {
		t.Fatalf("UpsertComment insert: %v", err)
	}
	if c.ID == 0 {
		t.Error("expected c.ID to be populated after insert")
	}

	// Update body and upsert
	c.Body = "updated body"
	if err := db.UpsertComment(c); err != nil {
		t.Fatalf("UpsertComment update: %v", err)
	}

	comments, err := db.ListComments(mr.ID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Body != "updated body" {
		t.Errorf("Body = %q, want %q", comments[0].Body, "updated body")
	}
}

func TestListComments_GroupedByDiscussion(t *testing.T) {
	db := testDB(t)
	mr := insertTestMR(t, db)

	// Two discussions, two comments each
	_ = db.UpsertComment(baseComment(mr.ID, 1, "disc-A"))
	_ = db.UpsertComment(baseComment(mr.ID, 2, "disc-A"))
	_ = db.UpsertComment(baseComment(mr.ID, 3, "disc-B"))
	_ = db.UpsertComment(baseComment(mr.ID, 4, "disc-B"))

	threads, err := db.ListThreads(mr.ID)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(threads))
	}
	for _, th := range threads {
		if len(th.Comments) != 2 {
			t.Errorf("thread %q: expected 2 comments, got %d", th.DiscussionID, len(th.Comments))
		}
	}
}

func TestUnresolvedCount(t *testing.T) {
	db := testDB(t)
	mr := insertTestMR(t, db)

	c1 := baseComment(mr.ID, 1, "disc-1")
	c1.Resolved = false
	c2 := baseComment(mr.ID, 2, "disc-2")
	c2.Resolved = true
	c3 := baseComment(mr.ID, 3, "disc-3")
	c3.Resolved = false

	_ = db.UpsertComment(c1)
	_ = db.UpsertComment(c2)
	_ = db.UpsertComment(c3)

	n, err := db.UnresolvedCommentCount(mr.ID)
	if err != nil {
		t.Fatalf("UnresolvedCommentCount: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 unresolved, got %d", n)
	}
}

func TestDeleteStaleComments(t *testing.T) {
	db := testDB(t)
	mr := insertTestMR(t, db)

	_ = db.UpsertComment(baseComment(mr.ID, 10, "disc-1"))
	_ = db.UpsertComment(baseComment(mr.ID, 20, "disc-2"))
	_ = db.UpsertComment(baseComment(mr.ID, 30, "disc-3"))

	if err := db.DeleteStaleComments(mr.ID, []int{10, 30}); err != nil {
		t.Fatalf("DeleteStaleComments: %v", err)
	}

	comments, err := db.ListComments(mr.ID)
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments after stale delete, got %d", len(comments))
	}
	for _, c := range comments {
		if c.NoteID == 20 {
			t.Error("stale comment note_id=20 was not deleted")
		}
	}
}
