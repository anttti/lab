package db

import "fmt"

// Reviewer is a per-MR reviewer record.
type Reviewer struct {
	Username string `db:"username"`
	State    string `db:"state"`
}

// SetMRReviewers replaces the reviewer list for mrID with the given set.
func (db *Database) SetMRReviewers(mrID int64, reviewers []Reviewer) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("SetMRReviewers begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM mr_reviewers WHERE mr_id = ?`, mrID); err != nil {
		return fmt.Errorf("SetMRReviewers delete: %w", err)
	}
	for _, r := range reviewers {
		if _, err := tx.Exec(
			`INSERT INTO mr_reviewers (mr_id, username, state) VALUES (?, ?, ?)`,
			mrID, r.Username, r.State,
		); err != nil {
			return fmt.Errorf("SetMRReviewers insert %q: %w", r.Username, err)
		}
	}
	return tx.Commit()
}

// GetMRReviewers returns all reviewers for mrID, sorted by username.
func (db *Database) GetMRReviewers(mrID int64) ([]Reviewer, error) {
	var rs []Reviewer
	if err := db.Select(&rs,
		`SELECT username, state FROM mr_reviewers WHERE mr_id = ? ORDER BY username`,
		mrID,
	); err != nil {
		return nil, fmt.Errorf("GetMRReviewers: %w", err)
	}
	return rs, nil
}
