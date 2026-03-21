package db

import (
	"os"
	"path/filepath"
	"testing"
)

// testDB creates a temp dir, opens a DB, and registers cleanup.
// Available to all test files in this package.
func testDB(t *testing.T) *Database {
	t.Helper()
	dir := t.TempDir()
	database, err := Open(dir)
	if err != nil {
		t.Fatalf("testDB: Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	return database
}

func TestOpen_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer db.Close()

	dbPath := filepath.Join(dir, "lab.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("expected lab.db to exist at %s", dbPath)
	}
}

func TestOpen_WALMode(t *testing.T) {
	db := testDB(t)

	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("expected journal_mode=wal, got %q", mode)
	}
}

func TestOpen_CreatesAllTables(t *testing.T) {
	db := testDB(t)

	want := []string{"repos", "merge_requests", "mr_labels", "comments", "config"}
	for _, table := range want {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}
