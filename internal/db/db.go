package db

import (
	"fmt"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// Database wraps sqlx.DB with migration and helper methods.
type Database struct {
	*sqlx.DB
}

// Open opens (or creates) lab.db in dataDir, enables WAL mode and foreign
// keys via the connection-string pragmas, and runs all schema migrations.
func Open(dataDir string) (*Database, error) {
	dbPath := filepath.Join(dataDir, "lab.db")
	dsn := dbPath + "?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)"

	sqlxDB, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Verify connection
	if err := sqlxDB.Ping(); err != nil {
		_ = sqlxDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	db := &Database{sqlxDB}
	if err := db.migrate(); err != nil {
		_ = sqlxDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

// Close closes the underlying database connection.
func (db *Database) Close() error {
	return db.DB.Close()
}

// migrate creates all application tables if they don't already exist.
func (db *Database) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS repos (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    path           TEXT UNIQUE NOT NULL,
    gitlab_url     TEXT NOT NULL DEFAULT '',
    project_id     INTEGER NOT NULL DEFAULT 0,
    name           TEXT NOT NULL DEFAULT '',
    added_at       DATETIME NOT NULL DEFAULT (datetime('now')),
    last_synced_at DATETIME
);

CREATE TABLE IF NOT EXISTS merge_requests (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    repo_id         INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    iid             INTEGER NOT NULL,
    title           TEXT NOT NULL DEFAULT '',
    author          TEXT NOT NULL DEFAULT '',
    state           TEXT NOT NULL DEFAULT 'opened',
    source_branch   TEXT NOT NULL DEFAULT '',
    target_branch   TEXT NOT NULL DEFAULT '',
    web_url         TEXT NOT NULL DEFAULT '',
    pipeline_status TEXT,
    updated_at      DATETIME,
    synced_at       DATETIME,
    UNIQUE(repo_id, iid)
);

CREATE TABLE IF NOT EXISTS mr_labels (
    mr_id  INTEGER NOT NULL REFERENCES merge_requests(id) ON DELETE CASCADE,
    label  TEXT NOT NULL,
    PRIMARY KEY (mr_id, label)
);

CREATE TABLE IF NOT EXISTS comments (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    mr_id         INTEGER NOT NULL REFERENCES merge_requests(id) ON DELETE CASCADE,
    discussion_id TEXT NOT NULL DEFAULT '',
    note_id       INTEGER NOT NULL,
    author        TEXT NOT NULL DEFAULT '',
    body          TEXT NOT NULL DEFAULT '',
    file_path     TEXT,
    old_line      INTEGER,
    new_line      INTEGER,
    resolved      BOOLEAN NOT NULL DEFAULT 0,
    created_at    DATETIME,
    synced_at     DATETIME,
    UNIQUE(mr_id, note_id)
);

CREATE TABLE IF NOT EXISTS config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);
`
	_, err := db.Exec(schema)
	return err
}
