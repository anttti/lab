package db

import (
	"fmt"
	"strings"
	"time"
)

// MergeRequest represents a GitLab MR record.
type MergeRequest struct {
	ID             int64      `db:"id"`
	RepoID         int64      `db:"repo_id"`
	IID            int        `db:"iid"`
	Title          string     `db:"title"`
	Author         string     `db:"author"`
	State          string     `db:"state"`
	Draft          bool       `db:"draft"`
	SourceBranch   string     `db:"source_branch"`
	TargetBranch   string     `db:"target_branch"`
	WebURL         string     `db:"web_url"`
	PipelineStatus *string    `db:"pipeline_status"`
	Approved       bool       `db:"approved"`
	UpdatedAt      time.Time  `db:"updated_at"`
	SyncedAt       *time.Time `db:"synced_at"`
}

// MRFilter holds optional filter criteria for ListMRs.
type MRFilter struct {
	RepoID       *int64
	Author       *string
	AuthorNegate bool // when true, exclude the named author instead of matching
	Labels       []string
	Draft        *bool // nil = all, true = drafts only, false = non-drafts only
	Approved     *bool // nil = all, true = approved only, false = not-approved only
}

// UpsertMR inserts or updates a MergeRequest. On conflict (repo_id, iid) it
// updates all fields and sets synced_at to now. mr.ID is populated on return.
func (db *Database) UpsertMR(mr *MergeRequest) error {
	const q = `
INSERT INTO merge_requests
    (repo_id, iid, title, author, state, draft, source_branch, target_branch,
     web_url, pipeline_status, approved, updated_at, synced_at)
VALUES
    (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
ON CONFLICT(repo_id, iid) DO UPDATE SET
    title           = excluded.title,
    author          = excluded.author,
    state           = excluded.state,
    draft           = excluded.draft,
    source_branch   = excluded.source_branch,
    target_branch   = excluded.target_branch,
    web_url         = excluded.web_url,
    pipeline_status = excluded.pipeline_status,
    approved        = excluded.approved,
    updated_at      = excluded.updated_at,
    synced_at       = datetime('now')
RETURNING id`

	row := db.QueryRowx(q,
		mr.RepoID, mr.IID, mr.Title, mr.Author, mr.State, mr.Draft,
		mr.SourceBranch, mr.TargetBranch, mr.WebURL,
		mr.PipelineStatus, mr.Approved, mr.UpdatedAt,
	)
	if err := row.Scan(&mr.ID); err != nil {
		return fmt.Errorf("UpsertMR: %w", err)
	}
	return nil
}

// GetMR returns the MR with the given primary-key id.
func (db *Database) GetMR(id int64) (*MergeRequest, error) {
	var mr MergeRequest
	if err := db.Get(&mr, `SELECT * FROM merge_requests WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("GetMR: %w", err)
	}
	return &mr, nil
}

// ListMRs returns merge requests matching the optional filter criteria.
func (db *Database) ListMRs(filter MRFilter) ([]MergeRequest, error) {
	args := []interface{}{}
	where := []string{}

	baseQuery := `SELECT DISTINCT mr.* FROM merge_requests mr`

	if len(filter.Labels) > 0 {
		baseQuery += ` JOIN mr_labels ml ON ml.mr_id = mr.id`
	}

	if filter.RepoID != nil {
		where = append(where, "mr.repo_id = ?")
		args = append(args, *filter.RepoID)
	}
	if filter.Author != nil {
		if filter.AuthorNegate {
			where = append(where, "mr.author != ?")
		} else {
			where = append(where, "mr.author = ?")
		}
		args = append(args, *filter.Author)
	}
	if filter.Draft != nil {
		where = append(where, "mr.draft = ?")
		args = append(args, *filter.Draft)
	}
	if filter.Approved != nil {
		where = append(where, "mr.approved = ?")
		args = append(args, *filter.Approved)
	}
	if len(filter.Labels) > 0 {
		placeholders := make([]string, len(filter.Labels))
		for i, l := range filter.Labels {
			placeholders[i] = "?"
			args = append(args, l)
		}
		where = append(where, "ml.label IN ("+strings.Join(placeholders, ",")+")")
	}

	query := baseQuery
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}

	var mrs []MergeRequest
	if err := db.Select(&mrs, query, args...); err != nil {
		return nil, fmt.Errorf("ListMRs: %w", err)
	}
	return mrs, nil
}

// SetMRLabels replaces all labels for mrID with the given set, in a transaction.
func (db *Database) SetMRLabels(mrID int64, labels []string) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("SetMRLabels begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM mr_labels WHERE mr_id = ?`, mrID); err != nil {
		return fmt.Errorf("SetMRLabels delete: %w", err)
	}
	for _, label := range labels {
		if _, err := tx.Exec(
			`INSERT INTO mr_labels (mr_id, label) VALUES (?, ?)`, mrID, label,
		); err != nil {
			return fmt.Errorf("SetMRLabels insert %q: %w", label, err)
		}
	}
	return tx.Commit()
}

// GetMRLabels returns all labels for the given MR id.
func (db *Database) GetMRLabels(mrID int64) ([]string, error) {
	var labels []string
	if err := db.Select(&labels, `SELECT label FROM mr_labels WHERE mr_id = ? ORDER BY label`, mrID); err != nil {
		return nil, fmt.Errorf("GetMRLabels: %w", err)
	}
	return labels, nil
}

// AllAuthors returns every distinct author across all MRs, sorted alphabetically.
func (db *Database) AllAuthors() ([]string, error) {
	var authors []string
	if err := db.Select(&authors, `SELECT DISTINCT author FROM merge_requests ORDER BY author`); err != nil {
		return nil, fmt.Errorf("AllAuthors: %w", err)
	}
	return authors, nil
}

// AllLabels returns every distinct label across all MRs.
func (db *Database) AllLabels() ([]string, error) {
	var labels []string
	if err := db.Select(&labels, `SELECT DISTINCT label FROM mr_labels ORDER BY label`); err != nil {
		return nil, fmt.Errorf("AllLabels: %w", err)
	}
	return labels, nil
}

// DeleteStaleMRs deletes MRs for repoID whose iid is not in keepIIDs.
func (db *Database) DeleteStaleMRs(repoID int64, keepIIDs []int) error {
	if len(keepIIDs) == 0 {
		_, err := db.Exec(`DELETE FROM merge_requests WHERE repo_id = ?`, repoID)
		return err
	}

	placeholders := make([]string, len(keepIIDs))
	args := []interface{}{repoID}
	for i, iid := range keepIIDs {
		placeholders[i] = "?"
		args = append(args, iid)
	}
	q := fmt.Sprintf(
		`DELETE FROM merge_requests WHERE repo_id = ? AND iid NOT IN (%s)`,
		strings.Join(placeholders, ","),
	)
	_, err := db.Exec(q, args...)
	return err
}
