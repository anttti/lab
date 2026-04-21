package sync

import (
	"strings"
	"testing"
	"time"

	"lab/internal/config"
	"lab/internal/glab"
)

// allEnabled returns a Notifications config with every trigger on.
func allEnabled() config.Notifications {
	return config.Default().Notifications
}

// captureNotifier records every Notify call for test assertions.
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

// stubFetcher implements mrDetailFetcher with a fixed response.
type stubFetcher struct {
	detail glab.MRDetail
	err    error
}

func (s stubFetcher) GetMRDetail(string, int64, int) (glab.MRDetail, error) {
	return s.detail, s.err
}

// TestDiffSnapshots_NewComment reports a notification when a new note
// appears on an MR authored by the user.
func TestDiffSnapshots_NewComment(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Title: "Refactor", Author: "me", WebURL: "https://x/7", NoteIDs: map[int]bool{100: true}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Title: "Refactor", Author: "me", WebURL: "https://x/7", NoteIDs: map[int]bool{100: true, 101: true}},
	}

	updates := diffSnapshots(pre, post, "me", allEnabled(), nil)
	if len(updates) != 1 {
		t.Fatalf("want 1 update, got %d", len(updates))
	}
	if updates[0].Kind != "new_comment" {
		t.Errorf("Kind: want new_comment, got %q", updates[0].Kind)
	}
	if !strings.Contains(updates[0].Message, "1 new comment") {
		t.Errorf("Message %q should say 1 new comment", updates[0].Message)
	}
}

// TestDiffSnapshots_NewComment_Disabled respects the config toggle.
func TestDiffSnapshots_NewComment_Disabled(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", NoteIDs: map[int]bool{100: true}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", NoteIDs: map[int]bool{100: true, 101: true}},
	}

	cfg := allEnabled()
	cfg.NewComment = false

	if got := diffSnapshots(pre, post, "me", cfg, nil); len(got) != 0 {
		t.Fatalf("want 0 updates, got %d", len(got))
	}
}

// TestDiffSnapshots_PipelineFailed fires only on a transition to failed.
func TestDiffSnapshots_PipelineFailed(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", PipelineStatus: "running", NoteIDs: map[int]bool{}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", PipelineStatus: "failed", NoteIDs: map[int]bool{}},
	}

	updates := diffSnapshots(pre, post, "me", allEnabled(), nil)
	if len(updates) != 1 || updates[0].Kind != "pipeline_failed" {
		t.Fatalf("want 1 pipeline_failed update, got %+v", updates)
	}
}

// TestDiffSnapshots_PipelineSuccessIgnored does not fire on green transitions.
func TestDiffSnapshots_PipelineSuccessIgnored(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", PipelineStatus: "running", NoteIDs: map[int]bool{}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", PipelineStatus: "success", NoteIDs: map[int]bool{}},
	}

	if got := diffSnapshots(pre, post, "me", allEnabled(), nil); len(got) != 0 {
		t.Fatalf("want 0 updates, got %+v", got)
	}
}

// TestDiffSnapshots_Approved fires when an MR transitions to approved.
func TestDiffSnapshots_Approved(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", Approved: false, NoteIDs: map[int]bool{}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", Approved: true, NoteIDs: map[int]bool{}},
	}

	updates := diffSnapshots(pre, post, "me", allEnabled(), nil)
	if len(updates) != 1 || updates[0].Kind != "approved" {
		t.Fatalf("want 1 approved update, got %+v", updates)
	}
}

// TestDiffSnapshots_NoChange returns no updates when nothing changed.
func TestDiffSnapshots_NoChange(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", PipelineStatus: "success", NoteIDs: map[int]bool{100: true}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", PipelineStatus: "success", NoteIDs: map[int]bool{100: true}},
	}

	if got := diffSnapshots(pre, post, "me", allEnabled(), nil); len(got) != 0 {
		t.Fatalf("want 0 updates, got %d", len(got))
	}
}

// TestDiffSnapshots_NewMROwnedByUserIgnored does not fire new_comment for
// MRs the user just authored (they appear only in post).
func TestDiffSnapshots_NewMROwnedByUserIgnored(t *testing.T) {
	pre := map[int64]mrSnapshot{}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", NoteIDs: map[int]bool{101: true}},
	}

	if got := diffSnapshots(pre, post, "me", allEnabled(), nil); len(got) != 0 {
		t.Fatalf("want 0 updates, got %+v", got)
	}
}

// TestDiffSnapshots_NewReviewRequestOnExistingMR fires when the user is
// newly added as a reviewer on an existing MR they didn't author.
func TestDiffSnapshots_NewReviewRequestOnExistingMR(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Author: "alice", Reviewers: map[string]string{}, NoteIDs: map[int]bool{}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Author: "alice", Reviewers: map[string]string{"me": "unreviewed"}, NoteIDs: map[int]bool{}},
	}

	updates := diffSnapshots(pre, post, "me", allEnabled(), nil)
	if len(updates) != 1 || updates[0].Kind != "new_review_request" {
		t.Fatalf("want 1 new_review_request, got %+v", updates)
	}
}

// TestDiffSnapshots_NewReviewRequestOnNewMR fires when a brand new MR
// arrives with the user listed as a reviewer.
func TestDiffSnapshots_NewReviewRequestOnNewMR(t *testing.T) {
	pre := map[int64]mrSnapshot{}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Author: "alice", Reviewers: map[string]string{"me": "unreviewed"}},
	}

	updates := diffSnapshots(pre, post, "me", allEnabled(), nil)
	if len(updates) != 1 || updates[0].Kind != "new_review_request" {
		t.Fatalf("want 1 new_review_request, got %+v", updates)
	}
}

// TestDiffSnapshots_RereviewRequest fires when the user's reviewer state
// drops from reviewed back to unreviewed.
func TestDiffSnapshots_RereviewRequest(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Author: "alice", Reviewers: map[string]string{"me": "reviewed"}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Author: "alice", Reviewers: map[string]string{"me": "unreviewed"}},
	}

	updates := diffSnapshots(pre, post, "me", allEnabled(), nil)
	if len(updates) != 1 || updates[0].Kind != "rereview_request" {
		t.Fatalf("want 1 rereview_request, got %+v", updates)
	}
}

// TestDiffSnapshots_MRMerged fires when a user-authored MR disappears and
// the fetcher reports state=merged.
func TestDiffSnapshots_MRMerged(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", WebURL: "https://x/7", RepoURL: "r", ProjectID: 42},
	}
	post := map[int64]mrSnapshot{}

	fetcher := stubFetcher{detail: glab.MRDetail{State: "merged"}}
	updates := diffSnapshots(pre, post, "me", allEnabled(), fetcher)
	if len(updates) != 1 || updates[0].Kind != "mr_merged" {
		t.Fatalf("want 1 mr_merged, got %+v", updates)
	}
}

// TestDiffSnapshots_MRClosedIgnored does not fire mr_merged when the MR is
// closed rather than merged.
func TestDiffSnapshots_MRClosedIgnored(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Author: "me", RepoURL: "r", ProjectID: 42},
	}
	post := map[int64]mrSnapshot{}

	fetcher := stubFetcher{detail: glab.MRDetail{State: "closed"}}
	if got := diffSnapshots(pre, post, "me", allEnabled(), fetcher); len(got) != 0 {
		t.Fatalf("want 0 updates, got %+v", got)
	}
}

// TestDiffSnapshots_EmptyUsername produces no updates.
func TestDiffSnapshots_EmptyUsername(t *testing.T) {
	pre := map[int64]mrSnapshot{
		1: {IID: 7, Author: "alice", NoteIDs: map[int]bool{100: true}},
	}
	post := map[int64]mrSnapshot{
		1: {IID: 7, Author: "alice", NoteIDs: map[int]bool{100: true, 101: true}},
	}

	if got := diffSnapshots(pre, post, "", allEnabled(), nil); len(got) != 0 {
		t.Fatalf("want 0 updates, got %+v", got)
	}
}

// TestSyncAllWithNotifications_EndToEnd checks the full flow through the
// engine, including the use of config to gate triggers.
func TestSyncAllWithNotifications_EndToEnd(t *testing.T) {
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
	cfg := allEnabled()

	// First sync seeds the DB; no notifications expected.
	if err := engine.SyncAllWithNotifications("alice", cfg, cn); err != nil {
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

	if err := engine.SyncAllWithNotifications("alice", cfg, cn); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(cn.calls) != 1 {
		t.Fatalf("want 1 notification, got %d", len(cn.calls))
	}
	if cn.calls[0].url != "https://example.com/mr/1" {
		t.Errorf("url: want https://example.com/mr/1, got %q", cn.calls[0].url)
	}
}
