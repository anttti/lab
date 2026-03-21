package db

import (
	"fmt"
	"time"
)

// Repo represents a tracked local repository.
type Repo struct {
	ID           int64      `db:"id"`
	Path         string     `db:"path"`
	GitLabURL    string     `db:"gitlab_url"`
	ProjectID    int64      `db:"project_id"`
	Name         string     `db:"name"`
	AddedAt      time.Time  `db:"added_at"`
	LastSyncedAt *time.Time `db:"last_synced_at"`
}

// AddRepo inserts a new repo record and returns the populated Repo.
func (db *Database) AddRepo(path, gitlabURL, name string) (*Repo, error) {
	const q = `
INSERT INTO repos (path, gitlab_url, name)
VALUES (?, ?, ?)
RETURNING id, path, gitlab_url, project_id, name, added_at, last_synced_at`

	var r Repo
	if err := db.QueryRowx(q, path, gitlabURL, name).StructScan(&r); err != nil {
		return nil, fmt.Errorf("AddRepo: %w", err)
	}
	return &r, nil
}

// ListRepos returns all repos ordered by name.
func (db *Database) ListRepos() ([]Repo, error) {
	var repos []Repo
	if err := db.Select(&repos, `SELECT * FROM repos ORDER BY name`); err != nil {
		return nil, fmt.Errorf("ListRepos: %w", err)
	}
	return repos, nil
}

// RemoveRepo deletes the repo with the given path. Returns an error if not found.
func (db *Database) RemoveRepo(path string) error {
	res, err := db.Exec(`DELETE FROM repos WHERE path = ?`, path)
	if err != nil {
		return fmt.Errorf("RemoveRepo: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("RemoveRepo rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("RemoveRepo: repo %q not found", path)
	}
	return nil
}

// GetRepo returns the repo with the given id.
func (db *Database) GetRepo(id int64) (*Repo, error) {
	var r Repo
	if err := db.Get(&r, `SELECT * FROM repos WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("GetRepo: %w", err)
	}
	return &r, nil
}

// UpdateRepoSyncTime sets last_synced_at to now for the given repo.
func (db *Database) UpdateRepoSyncTime(id int64) error {
	_, err := db.Exec(
		`UPDATE repos SET last_synced_at = datetime('now') WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("UpdateRepoSyncTime: %w", err)
	}
	return nil
}

// UpdateRepoProjectID sets the project_id for the given repo.
func (db *Database) UpdateRepoProjectID(id int64, projectID int64) error {
	_, err := db.Exec(
		`UPDATE repos SET project_id = ? WHERE id = ?`, projectID, id,
	)
	if err != nil {
		return fmt.Errorf("UpdateRepoProjectID: %w", err)
	}
	return nil
}
