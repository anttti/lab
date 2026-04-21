package sync

import (
	"strings"
	"testing"
	"time"

	"lab/internal/glab"
)

// TestSnapshotUserMRs_EmptyUsername returns empty map without hitting the DB.
func TestSnapshotUserMRs_EmptyUsername(t *testing.T) {
	database := testSyncDB(t)
	snap, err := snapshotUserMRs(database, "")
	if err != nil {
		t.Fatalf("snapshotUserMRs: %v", err)
	}
	if len(snap) != 0 {
		t.Errorf("want empty snapshot, got %d entries", len(snap))
	}
}

// TestSyncAllWithNotifications_NotifiesOnNewComment exercises the full flow
// end-to-end with a captureNotifier and a mock glab client.
func TestSyncAllWithNotifications_NotifiesOnNewComment(t *testing.T) {
	database := testSyncDB(t)
	testRepo(t, database)

	mr := glab.MRListItem{
		ID: 1, IID: 1, ProjectID: 42,
		Title: "My MR", State: "opened",
		WebURL:    "https://example.com/mr/1",
		UpdatedAt: time.Now().Add(-time.Hour),
		Author:    glab.Author{Username: "alice"},
	}

	mock := &mockGlab{
		mrs:         []glab.MRListItem{mr},
		discussions: map[int][]glab.Discussion{},
		pipelines:   map[int]string{},
	}

	engine := newTestEngine(database, mock)
	cn := &captureNotifier{}

	// First sync seeds the DB; no notifications expected (MR is new to pre).
	if err := engine.SyncAllWithNotifications("alice", cn); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if len(cn.calls) != 0 {
		t.Fatalf("first sync should not notify, got %d", len(cn.calls))
	}

	// Add a new comment authored by someone else.
	mock.discussions = map[int][]glab.Discussion{
		1: {
			{
				ID: "d1",
				Notes: []glab.Note{
					{
						ID: 101, Body: "please fix",
						Author:    glab.Author{Username: "bob"},
						CreatedAt: time.Now(),
					},
				},
			},
		},
	}

	// Second sync: should detect the new comment and notify.
	if err := engine.SyncAllWithNotifications("alice", cn); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(cn.calls) != 1 {
		t.Fatalf("want 1 notification, got %d", len(cn.calls))
	}
	if cn.calls[0].url != "https://example.com/mr/1" {
		t.Errorf("url: want https://example.com/mr/1, got %q", cn.calls[0].url)
	}
	if !strings.Contains(cn.calls[0].message, "new comment") {
		t.Errorf("message should mention new comment, got %q", cn.calls[0].message)
	}
}

type captureNotifier struct {
	calls []notifyCall
}

type notifyCall struct {
	title, message, url string
}

func (c *captureNotifier) Notify(title, message, url string) error {
	c.calls = append(c.calls, notifyCall{title, message, url})
	return nil
}

// TestDiffSnapshots_NewComment reports a notification when a new note
// appears on a tracked MR.
func TestDiffSnapshots_NewComment(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Title: "Refactor", WebURL: "https://x/7", NoteIDs: map[int]bool{100: true}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Title: "Refactor", WebURL: "https://x/7", NoteIDs: map[int]bool{100: true, 101: true}},
	}

	updates := diffSnapshots(pre, post)
	if len(updates) != 1 {
		t.Fatalf("want 1 update, got %d", len(updates))
	}
	if updates[0].WebURL != "https://x/7" {
		t.Errorf("WebURL: want https://x/7, got %q", updates[0].WebURL)
	}
	if updates[0].Message == "" {
		t.Error("Message should not be empty")
	}
}

// TestDiffSnapshots_MultipleNewComments reports a pluralised message.
func TestDiffSnapshots_MultipleNewComments(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, NoteIDs: map[int]bool{}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, NoteIDs: map[int]bool{101: true, 102: true, 103: true}},
	}

	updates := diffSnapshots(pre, post)
	if len(updates) != 1 {
		t.Fatalf("want 1 update, got %d", len(updates))
	}
	if got := updates[0].Message; !strings.Contains(got, "3 new comments") {
		t.Errorf("Message %q should mention 3 new comments", got)
	}
}

// TestDiffSnapshots_PipelineChanged reports pipeline status transitions.
func TestDiffSnapshots_PipelineChanged(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, PipelineStatus: "running", NoteIDs: map[int]bool{}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, PipelineStatus: "failed", NoteIDs: map[int]bool{}},
	}

	updates := diffSnapshots(pre, post)
	if len(updates) != 1 {
		t.Fatalf("want 1 update, got %d", len(updates))
	}
	if !strings.Contains(updates[0].Message, "failed") {
		t.Errorf("Message %q should mention failed pipeline", updates[0].Message)
	}
}

// TestDiffSnapshots_NoChange returns no updates when nothing changed.
func TestDiffSnapshots_NoChange(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, PipelineStatus: "success", NoteIDs: map[int]bool{100: true}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, PipelineStatus: "success", NoteIDs: map[int]bool{100: true}},
	}

	updates := diffSnapshots(pre, post)
	if len(updates) != 0 {
		t.Fatalf("want 0 updates, got %d", len(updates))
	}
}

// TestDiffSnapshots_NewMRIgnored does not notify for MRs that appear in post
// but not pre — they are likely just created by the user.
func TestDiffSnapshots_NewMRIgnored(t *testing.T) {
	pre := map[int64]mrSnapshot{}
	post := map[int64]mrSnapshot{
		1: {IID: 7, NoteIDs: map[int]bool{101: true}},
	}

	updates := diffSnapshots(pre, post)
	if len(updates) != 0 {
		t.Fatalf("want 0 updates, got %d", len(updates))
	}
}

// TestDiffSnapshots_Approved fires when an MR transitions to approved.
func TestDiffSnapshots_Approved(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Approved: false, NoteIDs: map[int]bool{}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Approved: true, NoteIDs: map[int]bool{}},
	}

	updates := diffSnapshots(pre, post)
	if len(updates) != 1 {
		t.Fatalf("want 1 update, got %d", len(updates))
	}
	if !strings.Contains(updates[0].Message, "approved") {
		t.Errorf("Message %q should mention approved", updates[0].Message)
	}
}

