package db

import (
	"fmt"
	"strings"
	"time"
)

// Comment represents a single note/comment on a GitLab MR.
type Comment struct {
	ID           int64      `db:"id"`
	MRID         int64      `db:"mr_id"`
	DiscussionID string     `db:"discussion_id"`
	NoteID       int        `db:"note_id"`
	Author       string     `db:"author"`
	Body         string     `db:"body"`
	FilePath     *string    `db:"file_path"`
	OldLine      *int       `db:"old_line"`
	NewLine      *int       `db:"new_line"`
	Resolved     bool       `db:"resolved"`
	CreatedAt    time.Time  `db:"created_at"`
	SyncedAt     *time.Time `db:"synced_at"`
}

// Thread groups a discussion's comments together.
type Thread struct {
	DiscussionID string
	FilePath     *string
	OldLine      *int
	NewLine      *int
	Resolved     bool
	Comments     []Comment
}

// UpsertComment inserts or updates a comment. On conflict (mr_id, note_id)
// it updates body, resolved, and synced_at. c.ID is populated on return.
func (db *Database) UpsertComment(c *Comment) error {
	const q = `
INSERT INTO comments
    (mr_id, discussion_id, note_id, author, body, file_path, old_line, new_line,
     resolved, created_at, synced_at)
VALUES
    (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT(mr_id, note_id) DO UPDATE SET
    body      = excluded.body,
    resolved  = excluded.resolved,
    synced_at = datetime('now')
RETURNING id`

	row := db.QueryRowx(q,
		c.MRID, c.DiscussionID, c.NoteID, c.Author, c.Body,
		c.FilePath, c.OldLine, c.NewLine, c.Resolved, c.CreatedAt,
	)
	if err := row.Scan(&c.ID); err != nil {
		return fmt.Errorf("UpsertComment: %w", err)
	}
	return nil
}

// ListComments returns all comments for a given MR, ordered by created_at.
func (db *Database) ListComments(mrID int64) ([]Comment, error) {
	var comments []Comment
	if err := db.Select(&comments,
		`SELECT * FROM comments WHERE mr_id = ? ORDER BY created_at`, mrID,
	); err != nil {
		return nil, fmt.Errorf("ListComments: %w", err)
	}
	return comments, nil
}

// ListThreads groups comments for the given MR into Thread structs, preserving
// insertion order of the first comment in each discussion.
func (db *Database) ListThreads(mrID int64) ([]Thread, error) {
	comments, err := db.ListComments(mrID)
	if err != nil {
		return nil, err
	}

	// Preserve insertion order of first occurrence of each discussion.
	seen := map[string]int{} // discussion_id -> index in threads slice
	var threads []Thread

	for _, c := range comments {
		idx, ok := seen[c.DiscussionID]
		if !ok {
			th := Thread{
				DiscussionID: c.DiscussionID,
				FilePath:     c.FilePath,
				OldLine:      c.OldLine,
				NewLine:      c.NewLine,
				Resolved:     c.Resolved,
			}
			threads = append(threads, th)
			idx = len(threads) - 1
			seen[c.DiscussionID] = idx
		}
		threads[idx].Comments = append(threads[idx].Comments, c)
	}

	return threads, nil
}

// UnresolvedCommentCount returns the number of unresolved comments for an MR.
func (db *Database) UnresolvedCommentCount(mrID int64) (int, error) {
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM comments WHERE mr_id = ? AND resolved = 0`, mrID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("UnresolvedCommentCount: %w", err)
	}
	return n, nil
}

// DeleteStaleComments deletes comments for mrID whose note_id is not in keepNoteIDs.
func (db *Database) DeleteStaleComments(mrID int64, keepNoteIDs []int) error {
	if len(keepNoteIDs) == 0 {
		_, err := db.Exec(`DELETE FROM comments WHERE mr_id = ?`, mrID)
		return err
	}

	placeholders := make([]string, len(keepNoteIDs))
	args := []interface{}{mrID}
	for i, id := range keepNoteIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(
		`DELETE FROM comments WHERE mr_id = ? AND note_id NOT IN (%s)`,
		strings.Join(placeholders, ","),
	)
	_, err := db.Exec(q, args...)
	return err
}
