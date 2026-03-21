# lab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI/TUI tool that manages GitLab merge requests, surfaces unresolved comments, and dispatches them to Claude Code for fixes.

**Architecture:** Single Go binary with Cobra subcommands. Bubbletea TUI with three views (MR list, MR detail, thread). SQLite persistence via modernc.org/sqlite. Sync engine shells out to `glab` CLI for MR listing and `glab api` for discussions. Background daemon via fork or launchd.

**Tech Stack:** Go 1.22+, Bubbletea, Lipgloss, Bubbles, Cobra, modernc.org/sqlite

**Spec:** `docs/superpowers/specs/2026-03-21-lab-design.md`

---

## File Structure

```
cmd/
  root.go           — Cobra root command, default launches TUI
  add.go            — `lab add <path>` command
  remove.go         — `lab remove <path>` command
  list.go           — `lab list` command
  sync.go           — `lab sync` command (one-shot and --loop)
  config.go         — `lab config` command
  daemon.go         — `lab daemon start/stop/status/install/uninstall`
internal/
  db/
    db.go           — DB open, migrations, WAL mode setup
    db_test.go
    repos.go        — repos CRUD operations
    repos_test.go
    mrs.go          — merge_requests + mr_labels CRUD
    mrs_test.go
    comments.go     — comments CRUD
    comments_test.go
    config.go       — config key-value store
    config_test.go
  glab/
    glab.go         — wrapper for shelling out to glab CLI
    glab_test.go
    types.go        — JSON response structs for glab output
  sync/
    sync.go         — sync engine: orchestrates glab calls + DB upserts
    sync_test.go
  tui/
    tui.go          — top-level Bubbletea program setup
    model.go        — root model, view routing, shared state
    mrlist.go       — MR list view model
    mrdetail.go     — MR detail view model (grouped threads)
    thread.go       — thread view model
    filter.go       — filter overlay model
    keys.go         — vim-style keybinding definitions
    styles.go       — Lipgloss style definitions
  claude/
    claude.go       — Claude Code prompt building + terminal launching
    claude_test.go
  daemon/
    daemon.go       — daemon start/stop/status, PID file management
    daemon_test.go
    launchd.go      — launchd plist generation, install/uninstall
    launchd_test.go
main.go             — entry point, calls cmd.Execute()
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `main.go`, `cmd/root.go`, `go.mod`, `go.sum`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/anttimattila/Projects/lab
go mod init github.com/anttimattila/lab
```

- [ ] **Step 2: Create main.go**

```go
// main.go
package main

import "github.com/anttimattila/lab/cmd"

func main() {
	cmd.Execute()
}
```

- [ ] **Step 3: Create cmd/root.go with Cobra root command**

```go
// cmd/root.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "lab",
	Short: "GitLab merge request TUI",
	Long:  "A TUI for managing GitLab merge requests and dispatching comments to Claude Code.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("TUI not yet implemented")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Install dependencies**

```bash
go get github.com/spf13/cobra
go mod tidy
```

- [ ] **Step 5: Verify it builds and runs**

```bash
go build -o lab . && ./lab
```

Expected: prints "TUI not yet implemented"

- [ ] **Step 6: Commit**

```bash
git add main.go cmd/ go.mod go.sum
git commit -m "feat: scaffold project with Cobra root command"
```

---

### Task 2: Database Layer — Schema & Migrations

**Files:**
- Create: `internal/db/db.go`, `internal/db/db_test.go`

- [ ] **Step 1: Install SQLite dependency**

```bash
go get modernc.org/sqlite
go get github.com/jmoiron/sqlx
```

We use `sqlx` on top of `database/sql` for convenient struct scanning.

- [ ] **Step 2: Write the failing test for DB initialization**

```go
// internal/db/db_test.go
package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	database, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer database.Close()

	dbPath := filepath.Join(dir, "lab.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("expected lab.db to be created")
	}
}

func TestOpen_WALMode(t *testing.T) {
	dir := t.TempDir()
	database, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer database.Close()

	var mode string
	err = database.DB.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if err != nil {
		t.Fatalf("PRAGMA journal_mode error: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("expected WAL mode, got %s", mode)
	}
}

func TestOpen_CreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	database, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer database.Close()

	tables := []string{"repos", "merge_requests", "mr_labels", "comments", "config"}
	for _, table := range tables {
		var name string
		err := database.DB.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Fatalf("table %s not found: %v", table, err)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/db/ -v
```

Expected: FAIL — `Open` not defined.

- [ ] **Step 4: Implement db.go**

```go
// internal/db/db.go
package db

import (
	"fmt"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type Database struct {
	DB *sqlx.DB
}

func Open(dataDir string) (*Database, error) {
	dbPath := filepath.Join(dataDir, "lab.db")
	db, err := sqlx.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	d := &Database{DB: db}
	if err := d.migrate(); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return d, nil
}

func (d *Database) Close() error {
	return d.DB.Close()
}

func (d *Database) migrate() error {
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
	_, err := d.DB.Exec(schema)
	return err
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/db/ -v
```

Expected: all 3 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/db/db.go internal/db/db_test.go go.mod go.sum
git commit -m "feat: add SQLite database layer with schema and migrations"
```

---

### Task 3: Database Layer — Repos CRUD

**Files:**
- Create: `internal/db/repos.go`, `internal/db/repos_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/db/repos_test.go
package db

import (
	"testing"
)

func testDB(t *testing.T) *Database {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestAddRepo(t *testing.T) {
	db := testDB(t)

	repo, err := db.AddRepo("/home/user/project", "https://gitlab.com/user/project", "project")
	if err != nil {
		t.Fatalf("AddRepo() error: %v", err)
	}
	if repo.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if repo.Path != "/home/user/project" {
		t.Fatalf("expected path /home/user/project, got %s", repo.Path)
	}
}

func TestAddRepo_DuplicatePath(t *testing.T) {
	db := testDB(t)

	_, err := db.AddRepo("/home/user/project", "https://gitlab.com/user/project", "project")
	if err != nil {
		t.Fatalf("first AddRepo() error: %v", err)
	}

	_, err = db.AddRepo("/home/user/project", "https://gitlab.com/user/project", "project")
	if err == nil {
		t.Fatal("expected error for duplicate path")
	}
}

func TestListRepos(t *testing.T) {
	db := testDB(t)

	db.AddRepo("/home/user/a", "https://gitlab.com/user/a", "a")
	db.AddRepo("/home/user/b", "https://gitlab.com/user/b", "b")

	repos, err := db.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos() error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
}

func TestRemoveRepo(t *testing.T) {
	db := testDB(t)

	db.AddRepo("/home/user/project", "https://gitlab.com/user/project", "project")

	err := db.RemoveRepo("/home/user/project")
	if err != nil {
		t.Fatalf("RemoveRepo() error: %v", err)
	}

	repos, err := db.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos() error: %v", err)
	}
	if len(repos) != 0 {
		t.Fatalf("expected 0 repos after remove, got %d", len(repos))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/db/ -run TestAddRepo -v && go test ./internal/db/ -run TestListRepos -v && go test ./internal/db/ -run TestRemoveRepo -v
```

Expected: FAIL — methods not defined.

- [ ] **Step 3: Implement repos.go**

```go
// internal/db/repos.go
package db

import (
	"fmt"
	"time"
)

type Repo struct {
	ID           int64      `db:"id"`
	Path         string     `db:"path"`
	GitLabURL    string     `db:"gitlab_url"`
	ProjectID    int64      `db:"project_id"`
	Name         string     `db:"name"`
	AddedAt      time.Time  `db:"added_at"`
	LastSyncedAt *time.Time `db:"last_synced_at"`
}

func (d *Database) AddRepo(path, gitlabURL, name string) (*Repo, error) {
	result, err := d.DB.Exec(
		"INSERT INTO repos (path, gitlab_url, name) VALUES (?, ?, ?)",
		path, gitlabURL, name,
	)
	if err != nil {
		return nil, fmt.Errorf("insert repo: %w", err)
	}

	id, _ := result.LastInsertId()
	repo := &Repo{ID: id, Path: path, GitLabURL: gitlabURL, Name: name}
	return repo, nil
}

func (d *Database) ListRepos() ([]Repo, error) {
	var repos []Repo
	err := d.DB.Select(&repos, "SELECT * FROM repos ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}
	return repos, nil
}

func (d *Database) RemoveRepo(path string) error {
	result, err := d.DB.Exec("DELETE FROM repos WHERE path = ?", path)
	if err != nil {
		return fmt.Errorf("delete repo: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("repo not found: %s", path)
	}
	return nil
}

func (d *Database) GetRepo(id int64) (*Repo, error) {
	var repo Repo
	err := d.DB.Get(&repo, "SELECT * FROM repos WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}
	return &repo, nil
}

func (d *Database) UpdateRepoSyncTime(id int64) error {
	_, err := d.DB.Exec(
		"UPDATE repos SET last_synced_at = datetime('now') WHERE id = ?", id,
	)
	return err
}

func (d *Database) UpdateRepoProjectID(id int64, projectID int64) error {
	_, err := d.DB.Exec(
		"UPDATE repos SET project_id = ? WHERE id = ?", projectID, id,
	)
	return err
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/db/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/repos.go internal/db/repos_test.go
git commit -m "feat: add repos CRUD operations"
```

---

### Task 4: Database Layer — Merge Requests & Labels CRUD

**Files:**
- Create: `internal/db/mrs.go`, `internal/db/mrs_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/db/mrs_test.go
package db

import (
	"testing"
	"time"
)

func TestUpsertMR(t *testing.T) {
	db := testDB(t)
	repo, _ := db.AddRepo("/tmp/proj", "https://gitlab.com/u/p", "p")

	mr := &MergeRequest{
		RepoID:         repo.ID,
		IID:            42,
		Title:          "Fix bug",
		Author:         "alice",
		State:          "opened",
		SourceBranch:   "fix-bug",
		TargetBranch:   "main",
		WebURL:         "https://gitlab.com/u/p/-/merge_requests/42",
		PipelineStatus: strPtr("success"),
		UpdatedAt:      time.Now(),
	}

	err := db.UpsertMR(mr)
	if err != nil {
		t.Fatalf("UpsertMR() error: %v", err)
	}
	if mr.ID == 0 {
		t.Fatal("expected non-zero ID after upsert")
	}

	// Upsert again with updated title
	mr.Title = "Fix critical bug"
	err = db.UpsertMR(mr)
	if err != nil {
		t.Fatalf("UpsertMR() second call error: %v", err)
	}

	fetched, err := db.ListMRs(MRFilter{})
	if err != nil {
		t.Fatalf("ListMRs() error: %v", err)
	}
	if len(fetched) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(fetched))
	}
	if fetched[0].Title != "Fix critical bug" {
		t.Fatalf("expected updated title, got %s", fetched[0].Title)
	}
}

func TestUpsertMRLabels(t *testing.T) {
	db := testDB(t)
	repo, _ := db.AddRepo("/tmp/proj", "https://gitlab.com/u/p", "p")

	mr := &MergeRequest{
		RepoID: repo.ID, IID: 1, Title: "MR", Author: "a",
		State: "opened", UpdatedAt: time.Now(),
	}
	db.UpsertMR(mr)

	err := db.SetMRLabels(mr.ID, []string{"bug", "urgent"})
	if err != nil {
		t.Fatalf("SetMRLabels() error: %v", err)
	}

	labels, err := db.GetMRLabels(mr.ID)
	if err != nil {
		t.Fatalf("GetMRLabels() error: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
}

func TestListMRs_FilterByRepo(t *testing.T) {
	db := testDB(t)
	r1, _ := db.AddRepo("/tmp/a", "https://gitlab.com/u/a", "a")
	r2, _ := db.AddRepo("/tmp/b", "https://gitlab.com/u/b", "b")

	db.UpsertMR(&MergeRequest{RepoID: r1.ID, IID: 1, Title: "MR1", Author: "a", State: "opened", UpdatedAt: time.Now()})
	db.UpsertMR(&MergeRequest{RepoID: r2.ID, IID: 1, Title: "MR2", Author: "b", State: "opened", UpdatedAt: time.Now()})

	mrs, _ := db.ListMRs(MRFilter{RepoID: &r1.ID})
	if len(mrs) != 1 {
		t.Fatalf("expected 1 MR for repo filter, got %d", len(mrs))
	}
	if mrs[0].Title != "MR1" {
		t.Fatalf("expected MR1, got %s", mrs[0].Title)
	}
}

func TestListMRs_FilterByAuthor(t *testing.T) {
	db := testDB(t)
	repo, _ := db.AddRepo("/tmp/a", "https://gitlab.com/u/a", "a")

	db.UpsertMR(&MergeRequest{RepoID: repo.ID, IID: 1, Title: "MR1", Author: "alice", State: "opened", UpdatedAt: time.Now()})
	db.UpsertMR(&MergeRequest{RepoID: repo.ID, IID: 2, Title: "MR2", Author: "bob", State: "opened", UpdatedAt: time.Now()})

	author := "alice"
	mrs, _ := db.ListMRs(MRFilter{Author: &author})
	if len(mrs) != 1 {
		t.Fatalf("expected 1 MR for author filter, got %d", len(mrs))
	}
}

func TestListMRs_FilterByLabels(t *testing.T) {
	db := testDB(t)
	repo, _ := db.AddRepo("/tmp/a", "https://gitlab.com/u/a", "a")

	mr1 := &MergeRequest{RepoID: repo.ID, IID: 1, Title: "MR1", Author: "a", State: "opened", UpdatedAt: time.Now()}
	mr2 := &MergeRequest{RepoID: repo.ID, IID: 2, Title: "MR2", Author: "a", State: "opened", UpdatedAt: time.Now()}
	db.UpsertMR(mr1)
	db.UpsertMR(mr2)
	db.SetMRLabels(mr1.ID, []string{"bug"})
	db.SetMRLabels(mr2.ID, []string{"feature"})

	mrs, _ := db.ListMRs(MRFilter{Labels: []string{"bug"}})
	if len(mrs) != 1 {
		t.Fatalf("expected 1 MR for label filter, got %d", len(mrs))
	}
	if mrs[0].Title != "MR1" {
		t.Fatalf("expected MR1, got %s", mrs[0].Title)
	}
}

func TestAllLabels(t *testing.T) {
	db := testDB(t)
	repo, _ := db.AddRepo("/tmp/a", "https://gitlab.com/u/a", "a")

	mr1 := &MergeRequest{RepoID: repo.ID, IID: 1, Title: "MR1", Author: "a", State: "opened", UpdatedAt: time.Now()}
	mr2 := &MergeRequest{RepoID: repo.ID, IID: 2, Title: "MR2", Author: "a", State: "opened", UpdatedAt: time.Now()}
	db.UpsertMR(mr1)
	db.UpsertMR(mr2)
	db.SetMRLabels(mr1.ID, []string{"bug", "urgent"})
	db.SetMRLabels(mr2.ID, []string{"bug", "feature"})

	labels, err := db.AllLabels()
	if err != nil {
		t.Fatalf("AllLabels() error: %v", err)
	}
	if len(labels) != 3 {
		t.Fatalf("expected 3 distinct labels, got %d", len(labels))
	}
}

func TestDeleteStaleMRs(t *testing.T) {
	db := testDB(t)
	repo, _ := db.AddRepo("/tmp/a", "https://gitlab.com/u/a", "a")

	db.UpsertMR(&MergeRequest{RepoID: repo.ID, IID: 1, Title: "MR1", Author: "a", State: "opened", UpdatedAt: time.Now()})
	db.UpsertMR(&MergeRequest{RepoID: repo.ID, IID: 2, Title: "MR2", Author: "a", State: "opened", UpdatedAt: time.Now()})

	// Keep only IID 1
	err := db.DeleteStaleMRs(repo.ID, []int{1})
	if err != nil {
		t.Fatalf("DeleteStaleMRs() error: %v", err)
	}

	mrs, _ := db.ListMRs(MRFilter{})
	if len(mrs) != 1 {
		t.Fatalf("expected 1 MR after stale delete, got %d", len(mrs))
	}
}

func strPtr(s string) *string { return &s }
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/db/ -run "TestUpsertMR|TestListMRs|TestAllLabels|TestDeleteStaleMRs" -v
```

Expected: FAIL — types and methods not defined.

- [ ] **Step 3: Implement mrs.go**

```go
// internal/db/mrs.go
package db

import (
	"fmt"
	"strings"
	"time"
)

type MergeRequest struct {
	ID             int64      `db:"id"`
	RepoID         int64      `db:"repo_id"`
	IID            int        `db:"iid"`
	Title          string     `db:"title"`
	Author         string     `db:"author"`
	State          string     `db:"state"`
	SourceBranch   string     `db:"source_branch"`
	TargetBranch   string     `db:"target_branch"`
	WebURL         string     `db:"web_url"`
	PipelineStatus *string    `db:"pipeline_status"`
	UpdatedAt      time.Time  `db:"updated_at"`
	SyncedAt       *time.Time `db:"synced_at"`
}

type MRFilter struct {
	RepoID *int64
	Author *string
	Labels []string
}

func (d *Database) UpsertMR(mr *MergeRequest) error {
	result, err := d.DB.NamedExec(`
		INSERT INTO merge_requests (repo_id, iid, title, author, state, source_branch, target_branch, web_url, pipeline_status, updated_at, synced_at)
		VALUES (:repo_id, :iid, :title, :author, :state, :source_branch, :target_branch, :web_url, :pipeline_status, :updated_at, datetime('now'))
		ON CONFLICT (repo_id, iid) DO UPDATE SET
			title = excluded.title,
			author = excluded.author,
			state = excluded.state,
			source_branch = excluded.source_branch,
			target_branch = excluded.target_branch,
			web_url = excluded.web_url,
			pipeline_status = excluded.pipeline_status,
			updated_at = excluded.updated_at,
			synced_at = datetime('now')
	`, mr)
	if err != nil {
		return fmt.Errorf("upsert MR: %w", err)
	}
	if mr.ID == 0 {
		id, _ := result.LastInsertId()
		if id != 0 {
			mr.ID = id
		} else {
			// Was an update, fetch the ID
			err = d.DB.Get(&mr.ID, "SELECT id FROM merge_requests WHERE repo_id = ? AND iid = ?", mr.RepoID, mr.IID)
			if err != nil {
				return fmt.Errorf("fetch MR ID after upsert: %w", err)
			}
		}
	}
	return nil
}

func (d *Database) ListMRs(filter MRFilter) ([]MergeRequest, error) {
	query := "SELECT DISTINCT m.* FROM merge_requests m"
	var args []interface{}
	var conditions []string

	if len(filter.Labels) > 0 {
		query += " JOIN mr_labels ml ON ml.mr_id = m.id"
		placeholders := make([]string, len(filter.Labels))
		for i, l := range filter.Labels {
			placeholders[i] = "?"
			args = append(args, l)
		}
		conditions = append(conditions, "ml.label IN ("+strings.Join(placeholders, ",")+")")
	}

	if filter.RepoID != nil {
		conditions = append(conditions, "m.repo_id = ?")
		args = append(args, *filter.RepoID)
	}
	if filter.Author != nil {
		conditions = append(conditions, "m.author = ?")
		args = append(args, *filter.Author)
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY m.updated_at DESC"

	var mrs []MergeRequest
	err := d.DB.Select(&mrs, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list MRs: %w", err)
	}
	return mrs, nil
}

func (d *Database) SetMRLabels(mrID int64, labels []string) error {
	tx, err := d.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM mr_labels WHERE mr_id = ?", mrID)
	if err != nil {
		return fmt.Errorf("clear labels: %w", err)
	}

	for _, label := range labels {
		_, err = tx.Exec("INSERT INTO mr_labels (mr_id, label) VALUES (?, ?)", mrID, label)
		if err != nil {
			return fmt.Errorf("insert label %s: %w", label, err)
		}
	}

	return tx.Commit()
}

func (d *Database) GetMRLabels(mrID int64) ([]string, error) {
	var labels []string
	err := d.DB.Select(&labels, "SELECT label FROM mr_labels WHERE mr_id = ? ORDER BY label", mrID)
	return labels, err
}

func (d *Database) AllLabels() ([]string, error) {
	var labels []string
	err := d.DB.Select(&labels, "SELECT DISTINCT label FROM mr_labels ORDER BY label")
	return labels, err
}

func (d *Database) DeleteStaleMRs(repoID int64, keepIIDs []int) error {
	if len(keepIIDs) == 0 {
		_, err := d.DB.Exec("DELETE FROM merge_requests WHERE repo_id = ?", repoID)
		return err
	}

	placeholders := make([]string, len(keepIIDs))
	args := []interface{}{repoID}
	for i, iid := range keepIIDs {
		placeholders[i] = "?"
		args = append(args, iid)
	}

	_, err := d.DB.Exec(
		"DELETE FROM merge_requests WHERE repo_id = ? AND iid NOT IN ("+strings.Join(placeholders, ",")+")",
		args...,
	)
	return err
}

func (d *Database) GetMR(id int64) (*MergeRequest, error) {
	var mr MergeRequest
	err := d.DB.Get(&mr, "SELECT * FROM merge_requests WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("get MR: %w", err)
	}
	return &mr, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/db/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/mrs.go internal/db/mrs_test.go
git commit -m "feat: add merge requests and labels CRUD with filtering"
```

---

### Task 5: Database Layer — Comments CRUD

**Files:**
- Create: `internal/db/comments.go`, `internal/db/comments_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/db/comments_test.go
package db

import (
	"testing"
	"time"
)

func TestUpsertComment(t *testing.T) {
	db := testDB(t)
	repo, _ := db.AddRepo("/tmp/p", "https://gitlab.com/u/p", "p")
	mr := &MergeRequest{RepoID: repo.ID, IID: 1, Title: "MR", Author: "a", State: "opened", UpdatedAt: time.Now()}
	db.UpsertMR(mr)

	c := &Comment{
		MRID:         mr.ID,
		DiscussionID: "disc-1",
		NoteID:       100,
		Author:       "reviewer",
		Body:         "Fix this",
		FilePath:     strPtr("src/main.go"),
		NewLine:      intPtr(42),
		Resolved:     false,
		CreatedAt:    time.Now(),
	}

	err := db.UpsertComment(c)
	if err != nil {
		t.Fatalf("UpsertComment() error: %v", err)
	}

	comments, _ := db.ListComments(mr.ID)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if comments[0].Body != "Fix this" {
		t.Fatalf("unexpected body: %s", comments[0].Body)
	}
}

func TestListComments_GroupedByDiscussion(t *testing.T) {
	db := testDB(t)
	repo, _ := db.AddRepo("/tmp/p", "https://gitlab.com/u/p", "p")
	mr := &MergeRequest{RepoID: repo.ID, IID: 1, Title: "MR", Author: "a", State: "opened", UpdatedAt: time.Now()}
	db.UpsertMR(mr)

	db.UpsertComment(&Comment{MRID: mr.ID, DiscussionID: "disc-1", NoteID: 1, Author: "a", Body: "First", CreatedAt: time.Now()})
	db.UpsertComment(&Comment{MRID: mr.ID, DiscussionID: "disc-1", NoteID: 2, Author: "b", Body: "Reply", CreatedAt: time.Now()})
	db.UpsertComment(&Comment{MRID: mr.ID, DiscussionID: "disc-2", NoteID: 3, Author: "c", Body: "Other", CreatedAt: time.Now()})

	threads, err := db.ListThreads(mr.ID)
	if err != nil {
		t.Fatalf("ListThreads() error: %v", err)
	}
	if len(threads) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(threads))
	}
}

func TestUnresolvedCount(t *testing.T) {
	db := testDB(t)
	repo, _ := db.AddRepo("/tmp/p", "https://gitlab.com/u/p", "p")
	mr := &MergeRequest{RepoID: repo.ID, IID: 1, Title: "MR", Author: "a", State: "opened", UpdatedAt: time.Now()}
	db.UpsertMR(mr)

	db.UpsertComment(&Comment{MRID: mr.ID, DiscussionID: "d1", NoteID: 1, Author: "a", Body: "Fix", Resolved: false, CreatedAt: time.Now()})
	db.UpsertComment(&Comment{MRID: mr.ID, DiscussionID: "d2", NoteID: 2, Author: "b", Body: "Ok", Resolved: true, CreatedAt: time.Now()})
	db.UpsertComment(&Comment{MRID: mr.ID, DiscussionID: "d3", NoteID: 3, Author: "c", Body: "Fix2", Resolved: false, CreatedAt: time.Now()})

	count, err := db.UnresolvedCommentCount(mr.ID)
	if err != nil {
		t.Fatalf("UnresolvedCommentCount() error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 unresolved, got %d", count)
	}
}

func TestDeleteStaleComments(t *testing.T) {
	db := testDB(t)
	repo, _ := db.AddRepo("/tmp/p", "https://gitlab.com/u/p", "p")
	mr := &MergeRequest{RepoID: repo.ID, IID: 1, Title: "MR", Author: "a", State: "opened", UpdatedAt: time.Now()}
	db.UpsertMR(mr)

	db.UpsertComment(&Comment{MRID: mr.ID, DiscussionID: "d1", NoteID: 1, Author: "a", Body: "Keep", CreatedAt: time.Now()})
	db.UpsertComment(&Comment{MRID: mr.ID, DiscussionID: "d2", NoteID: 2, Author: "b", Body: "Remove", CreatedAt: time.Now()})

	err := db.DeleteStaleComments(mr.ID, []int{1})
	if err != nil {
		t.Fatalf("DeleteStaleComments() error: %v", err)
	}

	comments, _ := db.ListComments(mr.ID)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
}

func intPtr(i int) *int { return &i }
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/db/ -run "TestUpsertComment|TestListComments|TestUnresolvedCount|TestDeleteStaleComments" -v
```

Expected: FAIL.

- [ ] **Step 3: Implement comments.go**

```go
// internal/db/comments.go
package db

import (
	"fmt"
	"strings"
	"time"
)

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

type Thread struct {
	DiscussionID string
	FilePath     *string
	OldLine      *int
	NewLine      *int
	Resolved     bool
	Comments     []Comment
}

func (d *Database) UpsertComment(c *Comment) error {
	_, err := d.DB.NamedExec(`
		INSERT INTO comments (mr_id, discussion_id, note_id, author, body, file_path, old_line, new_line, resolved, created_at, synced_at)
		VALUES (:mr_id, :discussion_id, :note_id, :author, :body, :file_path, :old_line, :new_line, :resolved, :created_at, datetime('now'))
		ON CONFLICT (mr_id, note_id) DO UPDATE SET
			body = excluded.body,
			resolved = excluded.resolved,
			synced_at = datetime('now')
	`, c)
	if err != nil {
		return fmt.Errorf("upsert comment: %w", err)
	}
	return nil
}

func (d *Database) ListComments(mrID int64) ([]Comment, error) {
	var comments []Comment
	err := d.DB.Select(&comments,
		"SELECT * FROM comments WHERE mr_id = ? ORDER BY created_at", mrID)
	return comments, err
}

func (d *Database) ListThreads(mrID int64) ([]Thread, error) {
	comments, err := d.ListComments(mrID)
	if err != nil {
		return nil, err
	}

	threadMap := make(map[string]*Thread)
	var order []string

	for _, c := range comments {
		t, exists := threadMap[c.DiscussionID]
		if !exists {
			t = &Thread{
				DiscussionID: c.DiscussionID,
				FilePath:     c.FilePath,
				OldLine:      c.OldLine,
				NewLine:      c.NewLine,
				Resolved:     c.Resolved,
			}
			threadMap[c.DiscussionID] = t
			order = append(order, c.DiscussionID)
		}
		t.Comments = append(t.Comments, c)
	}

	threads := make([]Thread, 0, len(order))
	for _, id := range order {
		threads = append(threads, *threadMap[id])
	}
	return threads, nil
}

func (d *Database) UnresolvedCommentCount(mrID int64) (int, error) {
	var count int
	err := d.DB.Get(&count,
		"SELECT COUNT(*) FROM comments WHERE mr_id = ? AND resolved = 0", mrID)
	return count, err
}

func (d *Database) DeleteStaleComments(mrID int64, keepNoteIDs []int) error {
	if len(keepNoteIDs) == 0 {
		_, err := d.DB.Exec("DELETE FROM comments WHERE mr_id = ?", mrID)
		return err
	}

	placeholders := make([]string, len(keepNoteIDs))
	args := []interface{}{mrID}
	for i, id := range keepNoteIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}

	_, err := d.DB.Exec(
		"DELETE FROM comments WHERE mr_id = ? AND note_id NOT IN ("+strings.Join(placeholders, ",")+")",
		args...,
	)
	return err
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/db/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/comments.go internal/db/comments_test.go
git commit -m "feat: add comments CRUD with threads and unresolved counting"
```

---

### Task 6: Database Layer — Config Store

**Files:**
- Create: `internal/db/config.go`, `internal/db/config_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/db/config_test.go
package db

import "testing"

func TestConfigSetGet(t *testing.T) {
	db := testDB(t)

	err := db.SetConfig("username", "alice")
	if err != nil {
		t.Fatalf("SetConfig() error: %v", err)
	}

	val, err := db.GetConfig("username")
	if err != nil {
		t.Fatalf("GetConfig() error: %v", err)
	}
	if val != "alice" {
		t.Fatalf("expected alice, got %s", val)
	}
}

func TestConfigSetOverwrites(t *testing.T) {
	db := testDB(t)

	db.SetConfig("username", "alice")
	db.SetConfig("username", "bob")

	val, _ := db.GetConfig("username")
	if val != "bob" {
		t.Fatalf("expected bob after overwrite, got %s", val)
	}
}

func TestConfigGetMissing(t *testing.T) {
	db := testDB(t)

	val, err := db.GetConfig("nonexistent")
	if err != nil {
		t.Fatalf("GetConfig() error: %v", err)
	}
	if val != "" {
		t.Fatalf("expected empty string for missing key, got %s", val)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/db/ -run TestConfig -v
```

- [ ] **Step 3: Implement config.go**

```go
// internal/db/config.go
package db

import "database/sql"

func (d *Database) SetConfig(key, value string) error {
	_, err := d.DB.Exec(
		"INSERT INTO config (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

func (d *Database) GetConfig(key string) (string, error) {
	var value string
	err := d.DB.Get(&value, "SELECT value FROM config WHERE key = ?", key)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/db/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/config.go internal/db/config_test.go
git commit -m "feat: add config key-value store"
```

---

### Task 7: glab Wrapper — Types & Command Execution

**Files:**
- Create: `internal/glab/types.go`, `internal/glab/glab.go`, `internal/glab/glab_test.go`

- [ ] **Step 1: Define JSON response structs**

```go
// internal/glab/types.go
package glab

import "time"

type MRListItem struct {
	ID             int       `json:"id"`
	IID            int       `json:"iid"`
	ProjectID      int64     `json:"project_id"`
	Title          string    `json:"title"`
	State          string    `json:"state"`
	SourceBranch   string    `json:"source_branch"`
	TargetBranch   string    `json:"target_branch"`
	WebURL         string    `json:"web_url"`
	Draft          bool      `json:"draft"`
	UpdatedAt      time.Time `json:"updated_at"`
	Author         Author    `json:"author"`
	Labels         []string  `json:"labels"`
	HeadPipeline   *Pipeline `json:"head_pipeline"`
}

type Author struct {
	Username string `json:"username"`
}

type Pipeline struct {
	Status string `json:"status"`
}

type Discussion struct {
	ID             string `json:"id"`
	IndividualNote bool   `json:"individual_note"`
	Notes          []Note `json:"notes"`
}

type Note struct {
	ID        int       `json:"id"`
	Type      *string   `json:"type"`
	Body      string    `json:"body"`
	Author    Author    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	System    bool      `json:"system"`
	Resolvable bool    `json:"resolvable"`
	Resolved   bool    `json:"resolved"`
	Position  *Position `json:"position"`
}

type Position struct {
	OldPath  string `json:"old_path"`
	NewPath  string `json:"new_path"`
	OldLine  *int   `json:"old_line"`
	NewLine  *int   `json:"new_line"`
}
```

- [ ] **Step 2: Implement glab command wrapper**

```go
// internal/glab/glab.go
package glab

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Client struct{}

func New() *Client {
	return &Client{}
}

func (c *Client) CheckInstalled() error {
	_, err := exec.LookPath("glab")
	if err != nil {
		return fmt.Errorf("glab not found on PATH: %w", err)
	}
	return nil
}

func (c *Client) ListMRs(repoURL string) ([]MRListItem, error) {
	out, err := exec.Command("glab", "mr", "list",
		"-R", repoURL,
		"-F", "json",
		"--per-page", "100",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("glab mr list: %w", err)
	}

	var mrs []MRListItem
	if err := json.Unmarshal(out, &mrs); err != nil {
		return nil, fmt.Errorf("parse glab mr list output: %w", err)
	}
	return mrs, nil
}

func (c *Client) ListDiscussions(repoURL string, projectID int64, mrIID int) ([]Discussion, error) {
	endpoint := fmt.Sprintf("projects/%d/merge_requests/%d/discussions?per_page=100",
		projectID, mrIID)

	out, err := exec.Command("glab", "api", endpoint, "-R", repoURL).Output()
	if err != nil {
		return nil, fmt.Errorf("glab api discussions: %w", err)
	}

	var discussions []Discussion
	if err := json.Unmarshal(out, &discussions); err != nil {
		return nil, fmt.Errorf("parse discussions: %w", err)
	}
	return discussions, nil
}

func (c *Client) GetMRPipeline(repoURL string, projectID int64, mrIID int) (string, error) {
	endpoint := fmt.Sprintf("projects/%d/merge_requests/%d", projectID, mrIID)

	out, err := exec.Command("glab", "api", endpoint, "-R", repoURL).Output()
	if err != nil {
		return "", fmt.Errorf("glab api MR detail: %w", err)
	}

	var detail struct {
		HeadPipeline *Pipeline `json:"head_pipeline"`
	}
	if err := json.Unmarshal(out, &detail); err != nil {
		return "", fmt.Errorf("parse MR detail: %w", err)
	}

	if detail.HeadPipeline == nil {
		return "", nil
	}
	return detail.HeadPipeline.Status, nil
}

func (c *Client) GetGitLabURL(repoPath string) (string, error) {
	out, err := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", fmt.Errorf("get git remote URL: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
```

- [ ] **Step 3: Write test for CheckInstalled (only test we can reliably run without a real GitLab)**

```go
// internal/glab/glab_test.go
package glab

import "testing"

func TestCheckInstalled(t *testing.T) {
	c := New()
	err := c.CheckInstalled()
	if err != nil {
		t.Skip("glab not installed, skipping")
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/glab/ -v
```

Expected: PASS (or skip if glab not installed).

- [ ] **Step 5: Commit**

```bash
git add internal/glab/
git commit -m "feat: add glab CLI wrapper with types for MR and discussion JSON"
```

---

### Task 8: Sync Engine

**Files:**
- Create: `internal/sync/sync.go`, `internal/sync/sync_test.go`

- [ ] **Step 1: Write failing test with a mock glab client**

```go
// internal/sync/sync_test.go
package sync

import (
	"testing"
	"time"

	"github.com/anttimattila/lab/internal/db"
	"github.com/anttimattila/lab/internal/glab"
)

type mockGlab struct {
	mrs         []glab.MRListItem
	discussions map[int][]glab.Discussion
	pipelines   map[int]string
}

func (m *mockGlab) ListMRs(repoURL string) ([]glab.MRListItem, error) {
	return m.mrs, nil
}

func (m *mockGlab) ListDiscussions(repoURL string, projectID int64, mrIID int) ([]glab.Discussion, error) {
	return m.discussions[mrIID], nil
}

func (m *mockGlab) GetMRPipeline(repoURL string, projectID int64, mrIID int) (string, error) {
	return m.pipelines[mrIID], nil
}

func testSyncDB(t *testing.T) *db.Database {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(dir)
	if err != nil {
		t.Fatalf("db.Open() error: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestSyncRepo_CreatesMRs(t *testing.T) {
	database := testSyncDB(t)
	repo, _ := database.AddRepo("/tmp/p", "https://gitlab.com/u/p", "p")
	database.UpdateRepoProjectID(repo.ID, 123)

	mock := &mockGlab{
		mrs: []glab.MRListItem{
			{
				IID: 1, ProjectID: 123, Title: "MR One",
				State: "opened", SourceBranch: "feat", TargetBranch: "main",
				WebURL: "https://gitlab.com/u/p/-/merge_requests/1",
				Author: glab.Author{Username: "alice"}, Labels: []string{"bug"},
				UpdatedAt: time.Now(),
			},
		},
		discussions: map[int][]glab.Discussion{
			1: {
				{
					ID: "disc-1",
					Notes: []glab.Note{
						{
							ID: 100, Body: "Fix this", Author: glab.Author{Username: "bob"},
							CreatedAt: time.Now(), Resolvable: true, Resolved: false,
							Position: &glab.Position{NewPath: "src/main.go", NewLine: intPtr(42)},
						},
					},
				},
			},
		},
		pipelines: map[int]string{1: "success"},
	}

	engine := New(database, mock)
	err := engine.SyncRepo(repo)
	if err != nil {
		t.Fatalf("SyncRepo() error: %v", err)
	}

	mrs, _ := database.ListMRs(db.MRFilter{})
	if len(mrs) != 1 {
		t.Fatalf("expected 1 MR, got %d", len(mrs))
	}
	if mrs[0].Title != "MR One" {
		t.Fatalf("unexpected title: %s", mrs[0].Title)
	}

	comments, _ := database.ListComments(mrs[0].ID)
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}
	if *comments[0].FilePath != "src/main.go" {
		t.Fatalf("unexpected file_path: %s", *comments[0].FilePath)
	}
}

func TestSyncRepo_DeletesStaleMRs(t *testing.T) {
	database := testSyncDB(t)
	repo, _ := database.AddRepo("/tmp/p", "https://gitlab.com/u/p", "p")
	database.UpdateRepoProjectID(repo.ID, 123)

	// First sync: 2 MRs
	mock := &mockGlab{
		mrs: []glab.MRListItem{
			{IID: 1, ProjectID: 123, Title: "MR1", State: "opened", Author: glab.Author{Username: "a"}, UpdatedAt: time.Now()},
			{IID: 2, ProjectID: 123, Title: "MR2", State: "opened", Author: glab.Author{Username: "b"}, UpdatedAt: time.Now()},
		},
		discussions: map[int][]glab.Discussion{},
		pipelines:   map[int]string{},
	}
	engine := New(database, mock)
	engine.SyncRepo(repo)

	mrs, _ := database.ListMRs(db.MRFilter{})
	if len(mrs) != 2 {
		t.Fatalf("expected 2 MRs after first sync, got %d", len(mrs))
	}

	// Second sync: only MR 1 remains (MR 2 was closed/merged)
	mock.mrs = []glab.MRListItem{
		{IID: 1, ProjectID: 123, Title: "MR1", State: "opened", Author: glab.Author{Username: "a"}, UpdatedAt: time.Now()},
	}
	engine.SyncRepo(repo)

	mrs, _ = database.ListMRs(db.MRFilter{})
	if len(mrs) != 1 {
		t.Fatalf("expected 1 MR after second sync, got %d", len(mrs))
	}
	if mrs[0].IID != 1 {
		t.Fatalf("expected MR 1 to remain, got IID %d", mrs[0].IID)
	}
}

func intPtr(i int) *int { return &i }
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/sync/ -v
```

Expected: FAIL — `sync.New`, `GlabClient` interface not defined.

- [ ] **Step 3: Implement sync engine**

```go
// internal/sync/sync.go
package sync

import (
	"fmt"
	"log"
	"os"

	"github.com/anttimattila/lab/internal/db"
	"github.com/anttimattila/lab/internal/glab"
)

type GlabClient interface {
	ListMRs(repoURL string) ([]glab.MRListItem, error)
	ListDiscussions(repoURL string, projectID int64, mrIID int) ([]glab.Discussion, error)
	GetMRPipeline(repoURL string, projectID int64, mrIID int) (string, error)
}

type Engine struct {
	db   *db.Database
	glab GlabClient
}

func New(database *db.Database, client GlabClient) *Engine {
	return &Engine{db: database, glab: client}
}

func (e *Engine) SyncRepo(repo *db.Repo) error {
	// Verify repo path still exists
	if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
		log.Printf("WARNING: repo path %s no longer exists, skipping sync", repo.Path)
		return nil
	}

	mrs, err := e.glab.ListMRs(repo.GitLabURL)
	if err != nil {
		return fmt.Errorf("list MRs for %s: %w", repo.Name, err)
	}

	// Update project_id from first MR if not set
	if repo.ProjectID == 0 && len(mrs) > 0 {
		repo.ProjectID = mrs[0].ProjectID
		e.db.UpdateRepoProjectID(repo.ID, repo.ProjectID)
	}

	keepIIDs := make([]int, 0, len(mrs))
	for _, glabMR := range mrs {
		keepIIDs = append(keepIIDs, glabMR.IID)

		pipelineStatus, _ := e.glab.GetMRPipeline(repo.GitLabURL, repo.ProjectID, glabMR.IID)
		var ps *string
		if pipelineStatus != "" {
			ps = &pipelineStatus
		}

		mr := &db.MergeRequest{
			RepoID:         repo.ID,
			IID:            glabMR.IID,
			Title:          glabMR.Title,
			Author:         glabMR.Author.Username,
			State:          glabMR.State,
			SourceBranch:   glabMR.SourceBranch,
			TargetBranch:   glabMR.TargetBranch,
			WebURL:         glabMR.WebURL,
			PipelineStatus: ps,
			UpdatedAt:      glabMR.UpdatedAt,
		}

		if err := e.db.UpsertMR(mr); err != nil {
			log.Printf("upsert MR %d: %v", glabMR.IID, err)
			continue
		}

		if err := e.db.SetMRLabels(mr.ID, glabMR.Labels); err != nil {
			log.Printf("set labels for MR %d: %v", glabMR.IID, err)
		}

		// Only sync discussions if MR was updated since last sync
		needsSync := mr.SyncedAt == nil || glabMR.UpdatedAt.After(*mr.SyncedAt)
		if !needsSync {
			continue
		}
		if err := e.syncDiscussions(repo, mr, glabMR.IID); err != nil {
			log.Printf("sync discussions for MR %d: %v", glabMR.IID, err)
		}
	}

	if err := e.db.DeleteStaleMRs(repo.ID, keepIIDs); err != nil {
		log.Printf("delete stale MRs: %v", err)
	}

	return e.db.UpdateRepoSyncTime(repo.ID)
}

func (e *Engine) SyncMR(repo *db.Repo, mrIID int) error {
	return e.syncDiscussions(repo, nil, mrIID)
}

func (e *Engine) syncDiscussions(repo *db.Repo, mr *db.MergeRequest, mrIID int) error {
	discussions, err := e.glab.ListDiscussions(repo.GitLabURL, repo.ProjectID, mrIID)
	if err != nil {
		return err
	}

	// If mr not provided, look it up
	if mr == nil {
		mrs, err := e.db.ListMRs(db.MRFilter{RepoID: &repo.ID})
		if err != nil {
			return err
		}
		for i := range mrs {
			if mrs[i].IID == mrIID {
				mr = &mrs[i]
				break
			}
		}
		if mr == nil {
			return fmt.Errorf("MR %d not found in DB", mrIID)
		}
	}

	keepNoteIDs := make([]int, 0)
	for _, disc := range discussions {
		for _, note := range disc.Notes {
			if note.System {
				continue
			}

			keepNoteIDs = append(keepNoteIDs, note.ID)

			var filePath *string
			var oldLine, newLine *int
			if note.Position != nil {
				if note.Position.NewPath != "" {
					filePath = &note.Position.NewPath
				}
				oldLine = note.Position.OldLine
				newLine = note.Position.NewLine
			}

			comment := &db.Comment{
				MRID:         mr.ID,
				DiscussionID: disc.ID,
				NoteID:       note.ID,
				Author:       note.Author.Username,
				Body:         note.Body,
				FilePath:     filePath,
				OldLine:      oldLine,
				NewLine:      newLine,
				Resolved:     note.Resolved,
				CreatedAt:    note.CreatedAt,
			}

			if err := e.db.UpsertComment(comment); err != nil {
				log.Printf("upsert comment %d: %v", note.ID, err)
			}
		}
	}

	return e.db.DeleteStaleComments(mr.ID, keepNoteIDs)
}

func (e *Engine) SyncAll() error {
	repos, err := e.db.ListRepos()
	if err != nil {
		return err
	}
	for i := range repos {
		if err := e.SyncRepo(&repos[i]); err != nil {
			log.Printf("sync repo %s: %v", repos[i].Name, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/sync/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/sync/ internal/glab/
git commit -m "feat: add sync engine with glab client interface and mock tests"
```

---

### Task 9: CLI Commands — add, remove, list, config

**Files:**
- Create: `cmd/add.go`, `cmd/remove.go`, `cmd/list.go`, `cmd/config.go`
- Modify: `cmd/root.go` — add dataDir helper

- [ ] **Step 1: Add data directory helper to root.go**

Add to `cmd/root.go`:

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/anttimattila/lab/internal/db"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "lab",
	Short: "GitLab merge request TUI",
	Long:  "A TUI for managing GitLab merge requests and dispatching comments to Claude Code.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("TUI not yet implemented")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func dataDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "lab")
	os.MkdirAll(dir, 0755)
	return dir
}

func openDB() (*db.Database, error) {
	return db.Open(dataDir())
}
```

- [ ] **Step 2: Implement add command**

```go
// cmd/add.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anttimattila/lab/internal/glab"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
}

var addCmd = &cobra.Command{
	Use:   "add <local-path>",
	Short: "Register a local git repo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := glab.New()
		if err := client.CheckInstalled(); err != nil {
			return err
		}

		repoPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}

		info, err := os.Stat(filepath.Join(repoPath, ".git"))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("%s is not a git repository", repoPath)
		}

		gitlabURL, err := client.GetGitLabURL(repoPath)
		if err != nil {
			return err
		}

		name := filepath.Base(repoPath)

		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		repo, err := database.AddRepo(repoPath, gitlabURL, name)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return fmt.Errorf("repo already registered: %s", repoPath)
			}
			return err
		}

		fmt.Printf("Added %s (%s)\n", repo.Name, repo.GitLabURL)
		return nil
	},
}
```

- [ ] **Step 3: Implement remove command**

```go
// cmd/remove.go
package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(removeCmd)
}

var removeCmd = &cobra.Command{
	Use:   "remove <local-path>",
	Short: "Unregister a repo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoPath, _ := filepath.Abs(args[0])

		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		if err := database.RemoveRepo(repoPath); err != nil {
			return err
		}

		fmt.Printf("Removed %s\n", repoPath)
		return nil
	},
}
```

- [ ] **Step 4: Implement list command**

```go
// cmd/list.go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered repos",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		repos, err := database.ListRepos()
		if err != nil {
			return err
		}

		if len(repos) == 0 {
			fmt.Println("No repos registered. Use 'lab add <path>' to add one.")
			return nil
		}

		for _, r := range repos {
			synced := "never"
			if r.LastSyncedAt != nil {
				synced = r.LastSyncedAt.Format("2006-01-02 15:04")
			}
			fmt.Printf("%-20s %-50s (last sync: %s)\n", r.Name, r.Path, synced)
		}
		return nil
	},
}
```

- [ ] **Step 5: Implement config command**

```go
// cmd/config.go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		if err := database.SetConfig(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("Set %s = %s\n", args[0], args[1])
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		val, err := database.GetConfig(args[0])
		if err != nil {
			return err
		}
		if val == "" {
			fmt.Printf("%s: (not set)\n", args[0])
		} else {
			fmt.Printf("%s: %s\n", args[0], val)
		}
		return nil
	},
}
```

- [ ] **Step 6: Verify build**

```bash
go build -o lab . && ./lab --help && ./lab add --help && ./lab config --help
```

Expected: help text shows all commands.

- [ ] **Step 7: Commit**

```bash
git add cmd/
git commit -m "feat: add CLI commands for add, remove, list, config"
```

---

### Task 10: CLI Command — sync

**Files:**
- Create: `cmd/sync.go`

- [ ] **Step 1: Implement sync command**

```go
// cmd/sync.go
package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/anttimattila/lab/internal/glab"
	gosync "github.com/anttimattila/lab/internal/sync"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().Bool("loop", false, "Run sync in a loop")
	syncCmd.Flags().Duration("interval", 5*time.Minute, "Sync interval (with --loop)")
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync merge requests from GitLab",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		client := glab.New()
		engine := gosync.New(database, client)

		loop, _ := cmd.Flags().GetBool("loop")
		interval, _ := cmd.Flags().GetDuration("interval")

		if !loop {
			fmt.Println("Syncing...")
			if err := engine.SyncAll(); err != nil {
				return err
			}
			fmt.Println("Done.")
			return nil
		}

		// Handle graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

		log.Printf("Starting sync loop (interval: %s)", interval)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Run once immediately
		if err := engine.SyncAll(); err != nil {
			log.Printf("Sync error: %v", err)
		}

		for {
			select {
			case <-ticker.C:
				if err := engine.SyncAll(); err != nil {
					log.Printf("Sync error: %v", err)
				} else {
					log.Printf("Sync complete")
				}
			case <-sigCh:
				log.Printf("Received shutdown signal, exiting")
				return nil
			}
		}
	},
}
```

- [ ] **Step 2: Verify build**

```bash
go build -o lab . && ./lab sync --help
```

Expected: shows --loop and --interval flags.

- [ ] **Step 3: Commit**

```bash
git add cmd/sync.go
git commit -m "feat: add sync command with one-shot and loop modes"
```

---

### Task 11: Daemon Management

**Files:**
- Create: `internal/daemon/daemon.go`, `internal/daemon/daemon_test.go`, `internal/daemon/launchd.go`, `internal/daemon/launchd_test.go`, `cmd/daemon.go`

- [ ] **Step 1: Write failing tests for PID management**

```go
// internal/daemon/daemon_test.go
package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadPID(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "daemon.pid")

	err := WritePID(pidFile, 12345)
	if err != nil {
		t.Fatalf("WritePID() error: %v", err)
	}

	pid, err := ReadPID(pidFile)
	if err != nil {
		t.Fatalf("ReadPID() error: %v", err)
	}
	if pid != 12345 {
		t.Fatalf("expected 12345, got %d", pid)
	}
}

func TestReadPID_NoFile(t *testing.T) {
	_, err := ReadPID("/nonexistent/pid")
	if err == nil {
		t.Fatal("expected error for missing PID file")
	}
}

func TestIsRunning_NotRunning(t *testing.T) {
	// PID 99999999 should not exist
	if IsRunning(99999999) {
		t.Fatal("expected PID 99999999 to not be running")
	}
}
```

- [ ] **Step 2: Implement daemon.go**

```go
// internal/daemon/daemon.go
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func WritePID(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
}

func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func IsRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

func Start(labBinary string, dataDir string, interval string) (int, error) {
	pidFile := pidPath(dataDir)
	logFile := logPath(dataDir)

	// Check if already running
	if pid, err := ReadPID(pidFile); err == nil && IsRunning(pid) {
		return pid, fmt.Errorf("daemon already running (PID %d)", pid)
	}

	lf, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, fmt.Errorf("open log file: %w", err)
	}

	cmd := exec.Command(labBinary, "sync", "--loop", "--interval", interval)
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		lf.Close()
		return 0, fmt.Errorf("start daemon: %w", err)
	}
	lf.Close()

	pid := cmd.Process.Pid
	if err := WritePID(pidFile, pid); err != nil {
		return pid, fmt.Errorf("write PID file: %w", err)
	}

	return pid, nil
}

func Stop(dataDir string) error {
	pidFile := pidPath(dataDir)

	pid, err := ReadPID(pidFile)
	if err != nil {
		return fmt.Errorf("daemon not running (no PID file)")
	}

	if !IsRunning(pid) {
		os.Remove(pidFile)
		return fmt.Errorf("daemon not running (stale PID file)")
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	os.Remove(pidFile)
	return nil
}

func Status(dataDir string) (bool, int, error) {
	pid, err := ReadPID(pidPath(dataDir))
	if err != nil {
		return false, 0, nil
	}
	return IsRunning(pid), pid, nil
}

func pidPath(dataDir string) string { return dataDir + "/daemon.pid" }
func logPath(dataDir string) string { return dataDir + "/daemon.log" }
```

- [ ] **Step 3: Write failing tests for launchd**

```go
// internal/daemon/launchd_test.go
package daemon

import (
	"strings"
	"testing"
)

func TestGeneratePlist(t *testing.T) {
	plist := GeneratePlist("/usr/local/bin/lab", "5m")

	if !strings.Contains(plist, "/usr/local/bin/lab") {
		t.Fatal("plist should contain binary path")
	}
	if !strings.Contains(plist, "com.lab.sync") {
		t.Fatal("plist should contain label")
	}
	if !strings.Contains(plist, "5m") {
		t.Fatal("plist should contain interval")
	}
}
```

- [ ] **Step 4: Implement launchd.go**

```go
// internal/daemon/launchd.go
package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.lab.sync</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.Binary}}</string>
        <string>sync</string>
        <string>--loop</string>
        <string>--interval</string>
        <string>{{.Interval}}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{.LogPath}}</string>
    <key>StandardErrorPath</key>
    <string>{{.LogPath}}</string>
</dict>
</plist>`

type plistData struct {
	Binary   string
	Interval string
	LogPath  string
}

func GeneratePlist(binary, interval string) string {
	home, _ := os.UserHomeDir()
	data := plistData{
		Binary:   binary,
		Interval: interval,
		LogPath:  filepath.Join(home, ".config", "lab", "daemon.log"),
	}

	var buf strings.Builder
	t := template.Must(template.New("plist").Parse(plistTemplate))
	t.Execute(&buf, data)
	return buf.String()
}

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.lab.sync.plist")
}

func Install(binary, interval string) error {
	plist := GeneratePlist(binary, interval)
	path := plistPath()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(plist), 0644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	if err := exec.Command("launchctl", "load", path).Run(); err != nil {
		return fmt.Errorf("launchctl load: %w", err)
	}

	return nil
}

func Uninstall() error {
	path := plistPath()
	exec.Command("launchctl", "unload", path).Run()
	return os.Remove(path)
}
```

Note: The `GeneratePlist` function uses `strings.Builder` — ensure `"strings"` is in the import block alongside `"fmt"`, `"os"`, `"os/exec"`, `"path/filepath"`, and `"text/template"`.

- [ ] **Step 5: Implement cmd/daemon.go**

```go
// cmd/daemon.go
package cmd

import (
	"fmt"
	"os"

	"github.com/anttimattila/lab/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(daemonCmd)
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonInstallCmd)
	daemonCmd.AddCommand(daemonUninstallCmd)
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage background sync daemon",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the background sync daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		binary, _ := os.Executable()

		// Read configured interval, default to 5m
		database, err := openDB()
		if err != nil {
			return err
		}
		interval, _ := database.GetConfig("sync_interval")
		database.Close()
		if interval == "" {
			interval = "5m"
		}

		pid, err := daemon.Start(binary, dataDir(), interval)
		if err != nil {
			return err
		}
		fmt.Printf("Daemon started (PID %d, interval %s)\n", pid, interval)
		return nil
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background sync daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := daemon.Stop(dataDir()); err != nil {
			return err
		}
		fmt.Println("Daemon stopped")
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		running, pid, _ := daemon.Status(dataDir())
		if running {
			fmt.Printf("Daemon is running (PID %d)\n", pid)
		} else {
			fmt.Println("Daemon is not running")
		}
		return nil
	},
}

var daemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install as a launchd service",
	RunE: func(cmd *cobra.Command, args []string) error {
		binary, _ := os.Executable()

		database, err := openDB()
		if err != nil {
			return err
		}
		interval, _ := database.GetConfig("sync_interval")
		database.Close()
		if interval == "" {
			interval = "5m"
		}

		if err := daemon.Install(binary, interval); err != nil {
			return err
		}
		fmt.Printf("Installed launchd service (com.lab.sync, interval %s)\n", interval)
		return nil
	},
}

var daemonUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall launchd service",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := daemon.Uninstall(); err != nil {
			return err
		}
		fmt.Println("Uninstalled launchd service")
		return nil
	},
}
```

- [ ] **Step 6: Run tests**

```bash
go test ./internal/daemon/ -v
```

Expected: all tests PASS.

- [ ] **Step 7: Verify build**

```bash
go build -o lab . && ./lab daemon --help
```

- [ ] **Step 8: Commit**

```bash
git add internal/daemon/ cmd/daemon.go
git commit -m "feat: add daemon management with PID tracking and launchd integration"
```

---

### Task 12: TUI Foundation — Keybindings, Styles, Root Model

**Files:**
- Create: `internal/tui/keys.go`, `internal/tui/styles.go`, `internal/tui/model.go`, `internal/tui/tui.go`

- [ ] **Step 1: Define vim-style keybindings**

```go
// internal/tui/keys.go
package tui

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Select   key.Binding
	Back     key.Binding
	Top      key.Binding
	Bottom   key.Binding
	Filter   key.Binding
	Sync     key.Binding
	Claude   key.Binding
	Quit     key.Binding
}

var Keys = KeyMap{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("k/↑", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j/↓", "down"),
	),
	Select: key.NewBinding(
		key.WithKeys("l", "enter", "right"),
		key.WithHelp("l/enter", "select"),
	),
	Back: key.NewBinding(
		key.WithKeys("h", "b", "left", "esc"),
		key.WithHelp("h/b", "back"),
	),
	Top: key.NewBinding(
		key.WithKeys("g", "home"),
		key.WithHelp("g", "top"),
	),
	Bottom: key.NewBinding(
		key.WithKeys("G", "end"),
		key.WithHelp("G", "bottom"),
	),
	Filter: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "filter"),
	),
	Sync: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "sync"),
	),
	Claude: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "claude"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}
```

- [ ] **Step 2: Define styles**

```go
// internal/tui/styles.go
package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("170")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	pipelineSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	pipelineFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	pipelineRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	unresolvedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	resolvedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)
```

- [ ] **Step 3: Create root model with view routing**

```go
// internal/tui/model.go
package tui

import (
	"github.com/anttimattila/lab/internal/db"
	gosync "github.com/anttimattila/lab/internal/sync"
	tea "github.com/charmbracelet/bubbletea"
)

type view int

const (
	viewMRList view = iota
	viewMRDetail
	viewThread
	viewFilter
)

type Model struct {
	db       *db.Database
	sync     *gosync.Engine
	current  view
	mrList   mrListModel
	mrDetail mrDetailModel
	thread   threadModel
	filter   filterModel
	width    int
	height   int
}

func NewModel(database *db.Database, engine *gosync.Engine) Model {
	return Model{
		db:      database,
		sync:    engine,
		current: viewMRList,
		mrList:  newMRListModel(database),
	}
}

func (m Model) Init() tea.Cmd {
	return m.mrList.loadMRs()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	switch m.current {
	case viewMRList:
		m.mrList, cmd = m.mrList.Update(msg, &m)
	case viewMRDetail:
		m.mrDetail, cmd = m.mrDetail.Update(msg, &m)
	case viewThread:
		m.thread, cmd = m.thread.Update(msg, &m)
	case viewFilter:
		m.filter, cmd = m.filter.Update(msg, &m)
	}
	return m, cmd
}

func (m Model) View() string {
	switch m.current {
	case viewMRList:
		return m.mrList.View(m.width, m.height)
	case viewMRDetail:
		return m.mrDetail.View(m.width, m.height)
	case viewThread:
		return m.thread.View(m.width, m.height)
	case viewFilter:
		return m.filter.View(m.width, m.height)
	}
	return ""
}
```

- [ ] **Step 4: Create TUI entry point**

```go
// internal/tui/tui.go
package tui

import (
	"fmt"

	"github.com/anttimattila/lab/internal/db"
	gosync "github.com/anttimattila/lab/internal/sync"
	tea "github.com/charmbracelet/bubbletea"
)

func Run(database *db.Database, engine *gosync.Engine) error {
	model := NewModel(database, engine)
	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Install Bubbletea dependencies**

```bash
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/bubbles
```

- [ ] **Step 6: Create stub files for sub-models so the package compiles**

Create minimal stub files that define just the types and method signatures. These will be replaced in Tasks 13-16:

```go
// internal/tui/mrlist_stub.go
package tui

import (
	"github.com/anttimattila/lab/internal/db"
	tea "github.com/charmbracelet/bubbletea"
)

type mrListModel struct{ db *db.Database }

func newMRListModel(database *db.Database) mrListModel { return mrListModel{db: database} }
func (m mrListModel) loadMRs() tea.Cmd                 { return nil }
func (m mrListModel) Update(msg tea.Msg, root *Model) (mrListModel, tea.Cmd) { return m, nil }
func (m mrListModel) View(width, height int) string     { return "MR list (stub)" }
```

```go
// internal/tui/mrdetail_stub.go
package tui

import tea "github.com/charmbracelet/bubbletea"

type mrDetailModel struct{}

func (m mrDetailModel) Update(msg tea.Msg, root *Model) (mrDetailModel, tea.Cmd) { return m, nil }
func (m mrDetailModel) View(width, height int) string { return "" }
```

```go
// internal/tui/thread_stub.go
package tui

import tea "github.com/charmbracelet/bubbletea"

type threadModel struct{}

func (m threadModel) Update(msg tea.Msg, root *Model) (threadModel, tea.Cmd) { return m, nil }
func (m threadModel) View(width, height int) string { return "" }
```

```go
// internal/tui/filter_stub.go
package tui

import tea "github.com/charmbracelet/bubbletea"

type filterModel struct{}

func (m filterModel) Update(msg tea.Msg, root *Model) (filterModel, tea.Cmd) { return m, nil }
func (m filterModel) View(width, height int) string { return "" }
```

- [ ] **Step 7: Verify the package compiles**

```bash
go build ./internal/tui/
```

Expected: compiles without errors.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/ go.mod go.sum
git commit -m "feat: add TUI foundation — keybindings, styles, root model with stubs"
```

**Note:** Tasks 13-16 will delete the `*_stub.go` files and replace them with full implementations.

---

### Task 13: TUI — MR List View

**Files:**
- Delete: `internal/tui/mrlist_stub.go`
- Create: `internal/tui/mrlist.go`

- [ ] **Step 1: Delete the stub file and implement MR list model**

```bash
rm internal/tui/mrlist_stub.go
```

```go
// internal/tui/mrlist.go
package tui

import (
	"fmt"
	"strings"

	"github.com/anttimattila/lab/internal/db"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type mrsLoadedMsg struct {
	mrs             []mrItem
	activeFilters   string
}

type mrItem struct {
	mr              db.MergeRequest
	repoName        string
	unresolvedCount int
}

type mrListModel struct {
	db            *db.Database
	items         []mrItem
	cursor        int
	activeFilters string
}

func newMRListModel(database *db.Database) mrListModel {
	return mrListModel{db: database}
}

func (m mrListModel) loadMRs() tea.Cmd {
	return func() tea.Msg {
		// Load filter state
		repoFilter, _ := m.db.GetConfig("active_repo_filter")
		userFilter, _ := m.db.GetConfig("active_user_filter")
		labelFilter, _ := m.db.GetConfig("active_label_filters")

		filter := db.MRFilter{}
		var filterParts []string

		if repoFilter != "" && repoFilter != "all" {
			var repoID int64
			fmt.Sscanf(repoFilter, "%d", &repoID)
			filter.RepoID = &repoID
			// Get repo name for display
			if repo, err := m.db.GetRepo(repoID); err == nil {
				filterParts = append(filterParts, repo.Name)
			}
		}

		if userFilter != "" && userFilter != "all" {
			filter.Author = &userFilter
			filterParts = append(filterParts, "Only me")
		}

		if labelFilter != "" {
			labels := strings.Split(labelFilter, ",")
			filter.Labels = labels
			filterParts = append(filterParts, strings.Join(labels, ", "))
		}

		mrs, _ := m.db.ListMRs(filter)

		// Get repo names and unresolved counts
		repoCache := make(map[int64]string)
		items := make([]mrItem, len(mrs))
		for i, mr := range mrs {
			name, ok := repoCache[mr.RepoID]
			if !ok {
				if repo, err := m.db.GetRepo(mr.RepoID); err == nil {
					name = repo.Name
				}
				repoCache[mr.RepoID] = name
			}
			count, _ := m.db.UnresolvedCommentCount(mr.ID)
			items[i] = mrItem{mr: mr, repoName: name, unresolvedCount: count}
		}

		activeStr := ""
		if len(filterParts) > 0 {
			activeStr = strings.Join(filterParts, " | ")
		}

		return mrsLoadedMsg{mrs: items, activeFilters: activeStr}
	}
}

func (m mrListModel) Update(msg tea.Msg, root *Model) (mrListModel, tea.Cmd) {
	switch msg := msg.(type) {
	case mrsLoadedMsg:
		m.items = msg.mrs
		m.activeFilters = msg.activeFilters
		if m.cursor >= len(m.items) {
			m.cursor = max(0, len(m.items)-1)
		}

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, Keys.Down):
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case key.Matches(msg, Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, Keys.Top):
			m.cursor = 0
		case key.Matches(msg, Keys.Bottom):
			m.cursor = max(0, len(m.items)-1)
		case key.Matches(msg, Keys.Select):
			if len(m.items) > 0 {
				selected := m.items[m.cursor]
				root.mrDetail = newMRDetailModel(root.db, root.sync, &selected.mr, selected.repoName)
				root.current = viewMRDetail
				return m, root.mrDetail.loadThreads()
			}
		case key.Matches(msg, Keys.Filter):
			root.filter = newFilterModel(root.db)
			root.current = viewFilter
			return m, root.filter.load()
		}
	}
	return m, nil
}

func (m mrListModel) View(width, height int) string {
	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render(" lab "))
	b.WriteString("\n")

	// Filter status
	if m.activeFilters != "" {
		b.WriteString(dimStyle.Render(" Filters: "+m.activeFilters))
	} else {
		b.WriteString(dimStyle.Render(" Filters: All repos | All users"))
	}
	b.WriteString("\n\n")

	// MR list
	if len(m.items) == 0 {
		b.WriteString(dimStyle.Render(" No merge requests found."))
		b.WriteString("\n")
	}

	for i, item := range m.items {
		cursor := "  "
		style := dimStyle
		if i == m.cursor {
			cursor = "▸ "
			style = selectedStyle
		}

		pipeline := pipelineIndicator(item.mr.PipelineStatus)
		unresolved := unresolvedStyle.Render(fmt.Sprintf("%d↩", item.unresolvedCount))

		line := fmt.Sprintf("%s%-15s !%-5d %-30s @%-12s %s %s",
			cursor,
			item.repoName,
			item.mr.IID,
			truncate(item.mr.Title, 30),
			item.mr.Author,
			unresolved,
			pipeline,
		)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	// Help
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(" j/k navigate  l/enter select  f filter  q quit"))

	return b.String()
}

func pipelineIndicator(status *string) string {
	if status == nil {
		return dimStyle.Render("—")
	}
	switch *status {
	case "success":
		return pipelineSuccess.Render("✓")
	case "failed":
		return pipelineFailed.Render("✗")
	case "running", "pending":
		return pipelineRunning.Render("⟳")
	default:
		return dimStyle.Render("—")
	}
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/mrlist.go
git commit -m "feat: add MR list view with filtering and pipeline indicators"
```

---

### Task 14: TUI — MR Detail View

**Files:**
- Delete: `internal/tui/mrdetail_stub.go`
- Create: `internal/tui/mrdetail.go`

- [ ] **Step 1: Delete stub and implement MR detail model**

```bash
rm internal/tui/mrdetail_stub.go
```

```go
// internal/tui/mrdetail.go
package tui

import (
	"fmt"
	"strings"

	"github.com/anttimattila/lab/internal/db"
	gosync "github.com/anttimattila/lab/internal/sync"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type threadsLoadedMsg struct {
	threads []db.Thread
}

type syncDoneMsg struct {
	err error
}

type mrDetailModel struct {
	db       *db.Database
	sync     *gosync.Engine
	mr       *db.MergeRequest
	repoName string
	threads  []db.Thread
	cursor   int
	syncing  bool
}

func newMRDetailModel(database *db.Database, engine *gosync.Engine, mr *db.MergeRequest, repoName string) mrDetailModel {
	return mrDetailModel{
		db:       database,
		sync:     engine,
		mr:       mr,
		repoName: repoName,
	}
}

func (m mrDetailModel) loadThreads() tea.Cmd {
	return func() tea.Msg {
		threads, _ := m.db.ListThreads(m.mr.ID)

		// Sort: unresolved first
		unresolved := make([]db.Thread, 0)
		resolved := make([]db.Thread, 0)
		for _, t := range threads {
			if t.Resolved {
				resolved = append(resolved, t)
			} else {
				unresolved = append(unresolved, t)
			}
		}
		sorted := append(unresolved, resolved...)

		return threadsLoadedMsg{threads: sorted}
	}
}

func (m mrDetailModel) syncMR() tea.Cmd {
	return func() tea.Msg {
		repo, err := m.db.GetRepo(m.mr.RepoID)
		if err != nil {
			return syncDoneMsg{err: err}
		}
		err = m.sync.SyncMR(repo, m.mr.IID)
		return syncDoneMsg{err: err}
	}
}

func (m mrDetailModel) Update(msg tea.Msg, root *Model) (mrDetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case threadsLoadedMsg:
		m.threads = msg.threads
		if m.cursor >= len(m.threads) {
			m.cursor = max(0, len(m.threads)-1)
		}

	case syncDoneMsg:
		m.syncing = false
		return m, m.loadThreads()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, Keys.Back):
			root.current = viewMRList
			return m, root.mrList.loadMRs()
		case key.Matches(msg, Keys.Down):
			if m.cursor < len(m.threads)-1 {
				m.cursor++
			}
		case key.Matches(msg, Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, Keys.Top):
			m.cursor = 0
		case key.Matches(msg, Keys.Bottom):
			m.cursor = max(0, len(m.threads)-1)
		case key.Matches(msg, Keys.Select):
			if len(m.threads) > 0 {
				selected := m.threads[m.cursor]
				repo, _ := m.db.GetRepo(m.mr.RepoID)
				root.thread = newThreadModel(m.db, &selected, m.mr, repo)
				root.current = viewThread
			}
		case key.Matches(msg, Keys.Sync):
			if !m.syncing {
				m.syncing = true
				return m, m.syncMR()
			}
		}
	}
	return m, nil
}

func (m mrDetailModel) View(width, height int) string {
	var b strings.Builder

	// Title
	title := fmt.Sprintf(" %s !%d — %s ", m.repoName, m.mr.IID, m.mr.Title)
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	if m.syncing {
		b.WriteString(dimStyle.Render(" Syncing..."))
		b.WriteString("\n\n")
	}

	if len(m.threads) == 0 {
		b.WriteString(dimStyle.Render(" No comments."))
		b.WriteString("\n")
	}

	for i, thread := range m.threads {
		cursor := "  "
		style := dimStyle
		if i == m.cursor {
			cursor = "▸ "
			style = selectedStyle
		}

		location := "General"
		if thread.FilePath != nil {
			location = *thread.FilePath
			if thread.NewLine != nil {
				location = fmt.Sprintf("%s:%d", location, *thread.NewLine)
			}
		}

		status := resolvedStyle.Render("resolved")
		if !thread.Resolved {
			status = unresolvedStyle.Render("unresolved")
		}

		noteCount := len(thread.Comments)
		header := fmt.Sprintf("%s%s (%d notes, %s)", cursor, location, noteCount, status)
		b.WriteString(style.Render(header))
		b.WriteString("\n")

		// First line of first comment
		if len(thread.Comments) > 0 {
			preview := truncate(strings.TrimSpace(thread.Comments[0].Body), 60)
			b.WriteString(dimStyle.Render(fmt.Sprintf("    \"%s\"", preview)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Help
	b.WriteString(helpStyle.Render(" j/k navigate  l/enter view thread  r sync  h/b back  q quit"))

	return b.String()
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/mrdetail.go
git commit -m "feat: add MR detail view with thread listing and manual sync"
```

---

### Task 15: TUI — Thread View

**Files:**
- Delete: `internal/tui/thread_stub.go`
- Create: `internal/tui/thread.go`

- [ ] **Step 1: Delete stub and implement thread view model**

```bash
rm internal/tui/thread_stub.go
```

```go
// internal/tui/thread.go
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/anttimattila/lab/internal/db"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type threadModel struct {
	db     *db.Database
	thread *db.Thread
	mr     *db.MergeRequest
	repo   *db.Repo
	scroll int
}

func newThreadModel(database *db.Database, thread *db.Thread, mr *db.MergeRequest, repo *db.Repo) threadModel {
	return threadModel{
		db:     database,
		thread: thread,
		mr:     mr,
		repo:   repo,
	}
}

func (m threadModel) Update(msg tea.Msg, root *Model) (threadModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, Keys.Back):
			root.current = viewMRDetail
		case key.Matches(msg, Keys.Down):
			m.scroll++
		case key.Matches(msg, Keys.Up):
			if m.scroll > 0 {
				m.scroll--
			}
		case key.Matches(msg, Keys.Claude):
			return m, m.launchClaude(root)
		}
	}
	return m, nil
}

func (m threadModel) launchClaude(root *Model) tea.Cmd {
	return func() tea.Msg {
		// This will be implemented in the Claude integration task
		return nil
	}
}

func (m threadModel) View(width, height int) string {
	var b strings.Builder

	// Title
	location := "General"
	if m.thread.FilePath != nil {
		location = *m.thread.FilePath
		if m.thread.NewLine != nil {
			location = fmt.Sprintf("%s:%d", location, *m.thread.NewLine)
		}
	}
	b.WriteString(titleStyle.Render(fmt.Sprintf(" %s ", location)))
	b.WriteString("\n\n")

	for _, comment := range m.thread.Comments {
		age := timeAgo(comment.CreatedAt)
		b.WriteString(selectedStyle.Render(fmt.Sprintf(" @%s ", comment.Author)))
		b.WriteString(dimStyle.Render(fmt.Sprintf("(%s)", age)))
		b.WriteString("\n")

		// Indent comment body
		lines := strings.Split(strings.TrimSpace(comment.Body), "\n")
		for _, line := range lines {
			b.WriteString(fmt.Sprintf(" %s\n", line))
		}
		b.WriteString("\n")
	}

	// Help
	b.WriteString(helpStyle.Render(" c launch Claude Code  j/k scroll  h/b back  q quit"))

	return b.String()
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/thread.go
git commit -m "feat: add thread view with comment display"
```

---

### Task 16: TUI — Filter Overlay

**Files:**
- Delete: `internal/tui/filter_stub.go`
- Create: `internal/tui/filter.go`

First: `rm internal/tui/filter_stub.go`

- [ ] **Step 1: Implement filter overlay model**

```go
// internal/tui/filter.go
package tui

import (
	"fmt"
	"strings"

	"github.com/anttimattila/lab/internal/db"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type filterGroup int

const (
	filterGroupRepo filterGroup = iota
	filterGroupUser
	filterGroupLabels
)

type filterDataMsg struct {
	repos  []db.Repo
	labels []string
}

type filterModel struct {
	db            *db.Database
	group         filterGroup
	repos         []db.Repo
	labels        []string
	cursor        int
	selectedRepo  string // repo ID or "all"
	selectedUser  string // "all" or username
	activeLabels  map[string]bool
}

func newFilterModel(database *db.Database) filterModel {
	return filterModel{
		db:           database,
		activeLabels: make(map[string]bool),
	}
}

func (m filterModel) load() tea.Cmd {
	return func() tea.Msg {
		repos, _ := m.db.ListRepos()
		labels, _ := m.db.AllLabels()
		return filterDataMsg{repos: repos, labels: labels}
	}
}

func (m filterModel) Update(msg tea.Msg, root *Model) (filterModel, tea.Cmd) {
	switch msg := msg.(type) {
	case filterDataMsg:
		m.repos = msg.repos
		m.labels = msg.labels

		// Load current filter state
		m.selectedRepo, _ = m.db.GetConfig("active_repo_filter")
		if m.selectedRepo == "" {
			m.selectedRepo = "all"
		}
		m.selectedUser, _ = m.db.GetConfig("active_user_filter")
		if m.selectedUser == "" {
			m.selectedUser = "all"
		}
		labelStr, _ := m.db.GetConfig("active_label_filters")
		m.activeLabels = make(map[string]bool)
		if labelStr != "" {
			for _, l := range strings.Split(labelStr, ",") {
				m.activeLabels[l] = true
			}
		}

	case tea.KeyMsg:
		switch {
		case msg.String() == "esc":
			// Save filters and go back
			m.saveFilters()
			root.current = viewMRList
			return m, root.mrList.loadMRs()

		case msg.String() == "tab":
			m.group = (m.group + 1) % 3
			m.cursor = 0

		case msg.String() == "shift+tab":
			if m.group == 0 {
				m.group = 2
			} else {
				m.group--
			}
			m.cursor = 0

		case key.Matches(msg, Keys.Down):
			maxItems := m.maxCursor()
			if m.cursor < maxItems {
				m.cursor++
			}

		case key.Matches(msg, Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case msg.String() == " ", msg.String() == "enter":
			m.toggleCurrent()
		}
	}
	return m, nil
}

func (m *filterModel) maxCursor() int {
	switch m.group {
	case filterGroupRepo:
		return len(m.repos) // +1 for "All repos" minus 1 for 0-indexed
	case filterGroupUser:
		return 1 // "All" and "Only me"
	case filterGroupLabels:
		return len(m.labels) // +1 for "No filter" minus 1
	}
	return 0
}

func (m *filterModel) toggleCurrent() {
	switch m.group {
	case filterGroupRepo:
		if m.cursor == 0 {
			m.selectedRepo = "all"
		} else {
			m.selectedRepo = fmt.Sprintf("%d", m.repos[m.cursor-1].ID)
		}
	case filterGroupUser:
		if m.cursor == 0 {
			m.selectedUser = "all"
		} else {
			username, _ := m.db.GetConfig("username")
			m.selectedUser = username
		}
	case filterGroupLabels:
		if m.cursor == 0 {
			m.activeLabels = make(map[string]bool)
		} else {
			label := m.labels[m.cursor-1]
			if m.activeLabels[label] {
				delete(m.activeLabels, label)
			} else {
				m.activeLabels[label] = true
			}
		}
	}
}

func (m *filterModel) saveFilters() {
	m.db.SetConfig("active_repo_filter", m.selectedRepo)
	m.db.SetConfig("active_user_filter", m.selectedUser)

	var labels []string
	for l := range m.activeLabels {
		labels = append(labels, l)
	}
	m.db.SetConfig("active_label_filters", strings.Join(labels, ","))
}

func (m filterModel) View(width, height int) string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" Filters "))
	b.WriteString("\n\n")

	groups := []struct {
		name  string
		group filterGroup
	}{
		{"Repo", filterGroupRepo},
		{"User", filterGroupUser},
		{"Labels", filterGroupLabels},
	}

	for _, g := range groups {
		active := ""
		if g.group == m.group {
			active = " ◂"
		}
		b.WriteString(titleStyle.Render(fmt.Sprintf(" %s%s", g.name, active)))
		b.WriteString("\n")

		switch g.group {
		case filterGroupRepo:
			m.renderRepoGroup(&b)
		case filterGroupUser:
			m.renderUserGroup(&b)
		case filterGroupLabels:
			m.renderLabelsGroup(&b)
		}
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render(" tab next group  j/k navigate  space/enter toggle  esc apply & close"))

	return b.String()
}

func (m filterModel) renderRepoGroup(b *strings.Builder) {
	isActive := m.group == filterGroupRepo
	items := []string{"All repos"}
	selected := []bool{m.selectedRepo == "all"}
	for _, r := range m.repos {
		items = append(items, r.Name)
		selected = append(selected, m.selectedRepo == fmt.Sprintf("%d", r.ID))
	}
	renderList(b, items, selected, m.cursor, isActive)
}

func (m filterModel) renderUserGroup(b *strings.Builder) {
	isActive := m.group == filterGroupUser
	items := []string{"All", "Only me"}
	selected := []bool{m.selectedUser == "all", m.selectedUser != "all"}
	renderList(b, items, selected, m.cursor, isActive)
}

func (m filterModel) renderLabelsGroup(b *strings.Builder) {
	isActive := m.group == filterGroupLabels
	items := []string{"No filter"}
	selected := []bool{len(m.activeLabels) == 0}
	for _, l := range m.labels {
		items = append(items, l)
		selected = append(selected, m.activeLabels[l])
	}
	renderList(b, items, selected, m.cursor, isActive)
}

func renderList(b *strings.Builder, items []string, selected []bool, cursor int, isActive bool) {
	for i, item := range items {
		check := "  "
		if selected[i] {
			check = "● "
		}

		prefix := "   "
		style := dimStyle
		if isActive && i == cursor {
			prefix = " ▸ "
			style = selectedStyle
		}

		b.WriteString(style.Render(fmt.Sprintf("%s%s%s", prefix, check, item)))
		b.WriteString("\n")
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/filter.go
git commit -m "feat: add filter overlay with repo, user, and label filtering"
```

---

### Task 17: Claude Code Integration

**Files:**
- Create: `internal/claude/claude.go`, `internal/claude/claude_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/claude/claude_test.go
package claude

import (
	"strings"
	"testing"

	"github.com/anttimattila/lab/internal/db"
)

func TestBuildPrompt(t *testing.T) {
	thread := &db.Thread{
		FilePath: strPtr("src/auth/handler.go"),
		NewLine:  intPtr(45),
		Comments: []db.Comment{
			{Author: "reviewer", Body: "This timeout should be configurable"},
			{Author: "author", Body: "Good point, will fix"},
		},
	}

	prompt := BuildPrompt(thread, "/home/user/project")

	if !strings.Contains(prompt, "src/auth/handler.go") {
		t.Fatal("prompt should contain file path")
	}
	if !strings.Contains(prompt, "line 45") {
		t.Fatal("prompt should contain line number")
	}
	if !strings.Contains(prompt, "/home/user/project/src/auth/handler.go") {
		t.Fatal("prompt should contain full path")
	}
	if !strings.Contains(prompt, "@reviewer") {
		t.Fatal("prompt should contain author")
	}
	if !strings.Contains(prompt, "Verify this issue exists and then fix it") {
		t.Fatal("prompt should contain instruction")
	}
}

func TestBuildPrompt_GeneralComment(t *testing.T) {
	thread := &db.Thread{
		Comments: []db.Comment{
			{Author: "reviewer", Body: "Please add tests"},
		},
	}

	prompt := BuildPrompt(thread, "/home/user/project")

	if strings.Contains(prompt, "File:") {
		t.Fatal("general comment should not have File: line")
	}
	if !strings.Contains(prompt, "@reviewer") {
		t.Fatal("prompt should contain author")
	}
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/claude/ -v
```

- [ ] **Step 3: Implement claude.go**

```go
// internal/claude/claude.go
package claude

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anttimattila/lab/internal/db"
)

func BuildPrompt(thread *db.Thread, repoPath string) string {
	var b strings.Builder

	if thread.FilePath != nil {
		lineInfo := ""
		if thread.NewLine != nil {
			lineInfo = fmt.Sprintf(" (line %d)", *thread.NewLine)
		}
		b.WriteString(fmt.Sprintf("File: %s%s\n", *thread.FilePath, lineInfo))
		fullPath := filepath.Join(repoPath, *thread.FilePath)
		b.WriteString(fmt.Sprintf("Full path: %s\n", fullPath))
		b.WriteString("\n")
	}

	b.WriteString("--- Comment thread ---\n")
	for _, c := range thread.Comments {
		b.WriteString(fmt.Sprintf("@%s:\n", c.Author))
		b.WriteString(c.Body)
		b.WriteString("\n\n")
	}
	b.WriteString("--- End thread ---\n\n")
	b.WriteString("Verify this issue exists and then fix it.\n")

	return b.String()
}

func LaunchInNewTerminal(prompt, repoPath string) error {
	// Check claude is available
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude not found on PATH: %w", err)
	}

	// Write prompt to temp file
	tmpFile, err := os.CreateTemp("", "lab-claude-*.md")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	if _, err := tmpFile.WriteString(prompt); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write prompt: %w", err)
	}
	tmpFile.Close()

	shellCmd := fmt.Sprintf("cd %q && claude --prompt-file %q; rm -f %q",
		repoPath, tmpFile.Name(), tmpFile.Name())

	return openTerminalWindow(shellCmd)
}

func openTerminalWindow(shellCmd string) error {
	termProgram := os.Getenv("TERM_PROGRAM")

	switch termProgram {
	case "iTerm.app":
		script := fmt.Sprintf(`tell application "iTerm"
			create window with default profile command "/bin/bash -c %q"
		end tell`, shellCmd)
		return exec.Command("osascript", "-e", script).Start()

	default:
		// Terminal.app fallback
		script := fmt.Sprintf(`tell application "Terminal"
			do script "%s"
			activate
		end tell`, strings.ReplaceAll(shellCmd, `"`, `\"`))
		return exec.Command("osascript", "-e", script).Start()
	}
}

func WritePromptToTempFile(prompt string) (string, error) {
	tmpFile, err := os.CreateTemp("", "lab-prompt-*.md")
	if err != nil {
		return "", err
	}
	if _, err := tmpFile.WriteString(prompt); err != nil {
		tmpFile.Close()
		return "", err
	}
	tmpFile.Close()
	return tmpFile.Name(), nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/claude/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/claude/
git commit -m "feat: add Claude Code integration — prompt building and terminal launching"
```

---

### Task 18: Wire Claude Code Into Thread View

**Files:**
- Modify: `internal/tui/thread.go` — replace stub `launchClaude` with real implementation

- [ ] **Step 1: Update thread.go to use claude package**

Add import for `"github.com/anttimattila/lab/internal/claude"` and replace the `launchClaude` method:

```go
type claudeChoiceMsg struct{}
type claudeLaunchedMsg struct{ err error }

func (m threadModel) launchClaude(root *Model) tea.Cmd {
	// This returns a message asking the user to choose
	return func() tea.Msg {
		return claudeChoiceMsg{}
	}
}
```

Then add handling in `Update` for the Claude flow. The thread model needs a `claudeState` field to track the augment/send-as-is choice:

Update thread.go with a `state` field:

```go
type threadState int

const (
	threadViewing threadState = iota
	threadClaudeChoice
)
```

Add to `threadModel`:
```go
	state threadState
```

Update the `Update` method to handle the choice:
```go
case key.Matches(msg, Keys.Claude):
	if m.state == threadViewing {
		m.state = threadClaudeChoice
	}

case msg.String() == "s":
	if m.state == threadClaudeChoice {
		m.state = threadViewing
		prompt := claude.BuildPrompt(m.thread, m.repo.Path)
		return m, func() tea.Msg {
			err := claude.LaunchInNewTerminal(prompt, m.repo.Path)
			return claudeLaunchedMsg{err: err}
		}
	}

case msg.String() == "a":
	if m.state == threadClaudeChoice {
		m.state = threadViewing
		prompt := claude.BuildPrompt(m.thread, m.repo.Path)
		// Write prompt to temp file for the editor
		tmpFile, err := claude.WritePromptToTempFile(prompt)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		repoPath := m.repo.Path
		// tea.ExecProcess suspends the TUI properly while the editor runs
		c := exec.Command(editor, tmpFile)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			if err != nil {
				return claudeLaunchedMsg{err: err}
			}
			edited, readErr := os.ReadFile(tmpFile)
			os.Remove(tmpFile)
			if readErr != nil {
				return claudeLaunchedMsg{err: readErr}
			}
			launchErr := claude.LaunchInNewTerminal(string(edited), repoPath)
			return claudeLaunchedMsg{err: launchErr}
		})
	}
```

Add to `View` when `m.state == threadClaudeChoice`:
```go
if m.state == threadClaudeChoice {
	b.WriteString("\n")
	b.WriteString(titleStyle.Render(" Send as-is (s) or augment in editor (a)? "))
}
```

This is complex — implement the full updated `thread.go` as a single file rewrite rather than incremental edits. The full implementation:

```go
// internal/tui/thread.go
package tui

import (
	"fmt"
	"strings"
	"time"

	"os"
	"os/exec"

	"github.com/anttimattila/lab/internal/claude"
	"github.com/anttimattila/lab/internal/db"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type threadState int

const (
	threadViewing threadState = iota
	threadClaudeChoice
)

type claudeLaunchedMsg struct{ err error }

type threadModel struct {
	db     *db.Database
	thread *db.Thread
	mr     *db.MergeRequest
	repo   *db.Repo
	scroll int
	state  threadState
	err    string
}

func newThreadModel(database *db.Database, thread *db.Thread, mr *db.MergeRequest, repo *db.Repo) threadModel {
	return threadModel{
		db:     database,
		thread: thread,
		mr:     mr,
		repo:   repo,
		state:  threadViewing,
	}
}

func (m threadModel) Update(msg tea.Msg, root *Model) (threadModel, tea.Cmd) {
	switch msg := msg.(type) {
	case claudeLaunchedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
		}

	case tea.KeyMsg:
		// Handle Claude choice state
		if m.state == threadClaudeChoice {
			switch msg.String() {
			case "s":
				m.state = threadViewing
				prompt := claude.BuildPrompt(m.thread, m.repo.Path)
				return m, func() tea.Msg {
					err := claude.LaunchInNewTerminal(prompt, m.repo.Path)
					return claudeLaunchedMsg{err: err}
				}
			case "a":
				m.state = threadViewing
				prompt := claude.BuildPrompt(m.thread, m.repo.Path)
				tmpFile, err := claude.WritePromptToTempFile(prompt)
				if err != nil {
					m.err = err.Error()
					return m, nil
				}
				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vi"
				}
				repoPath := m.repo.Path
				c := exec.Command(editor, tmpFile)
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					if err != nil {
						return claudeLaunchedMsg{err: err}
					}
					edited, readErr := os.ReadFile(tmpFile)
					os.Remove(tmpFile)
					if readErr != nil {
						return claudeLaunchedMsg{err: readErr}
					}
					launchErr := claude.LaunchInNewTerminal(string(edited), repoPath)
					return claudeLaunchedMsg{err: launchErr}
				})
			case "esc":
				m.state = threadViewing
			}
			return m, nil
		}

		// Normal viewing state
		switch {
		case key.Matches(msg, Keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, Keys.Back):
			root.current = viewMRDetail
		case key.Matches(msg, Keys.Down):
			m.scroll++
		case key.Matches(msg, Keys.Up):
			if m.scroll > 0 {
				m.scroll--
			}
		case key.Matches(msg, Keys.Claude):
			m.state = threadClaudeChoice
			m.err = ""
		}
	}
	return m, nil
}

func (m threadModel) View(width, height int) string {
	var b strings.Builder

	// Title
	location := "General"
	if m.thread.FilePath != nil {
		location = *m.thread.FilePath
		if m.thread.NewLine != nil {
			location = fmt.Sprintf("%s:%d", location, *m.thread.NewLine)
		}
	}
	b.WriteString(titleStyle.Render(fmt.Sprintf(" %s ", location)))
	b.WriteString("\n\n")

	for _, comment := range m.thread.Comments {
		age := timeAgo(comment.CreatedAt)
		b.WriteString(selectedStyle.Render(fmt.Sprintf(" @%s ", comment.Author)))
		b.WriteString(dimStyle.Render(fmt.Sprintf("(%s)", age)))
		b.WriteString("\n")

		lines := strings.Split(strings.TrimSpace(comment.Body), "\n")
		for _, line := range lines {
			b.WriteString(fmt.Sprintf(" %s\n", line))
		}
		b.WriteString("\n")
	}

	// Error display
	if m.err != "" {
		b.WriteString(pipelineFailed.Render(fmt.Sprintf(" Error: %s", m.err)))
		b.WriteString("\n\n")
	}

	// Claude choice overlay
	if m.state == threadClaudeChoice {
		b.WriteString(titleStyle.Render(" Send as-is (s) or augment in editor (a)? (esc to cancel) "))
		b.WriteString("\n")
	}

	// Help
	b.WriteString(helpStyle.Render(" c launch Claude Code  j/k scroll  h/b back  q quit"))

	return b.String()
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
```

Note: The `$EDITOR` flow suspends the TUI. The Bubbletea `tea.ExecProcess` function handles this, but for simplicity we'll run it in a goroutine and send a message back. If the editor approach needs refinement, it can be adjusted during implementation.

- [ ] **Step 2: Verify build compiles**

```bash
go build -o lab .
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/thread.go
git commit -m "feat: wire Claude Code launch into thread view with augment/as-is choice"
```

---

### Task 19: Wire TUI Into Root Command

**Files:**
- Modify: `cmd/root.go` — replace placeholder with TUI launch

- [ ] **Step 1: Update root command to launch TUI**

Replace the `Run` function in `cmd/root.go`:

```go
var rootCmd = &cobra.Command{
	Use:   "lab",
	Short: "GitLab merge request TUI",
	Long:  "A TUI for managing GitLab merge requests and dispatching comments to Claude Code.",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		client := glab.New()
		engine := gosync.New(database, client)

		return tui.Run(database, engine)
	},
}
```

Add imports:
```go
import (
	"os"
	"path/filepath"

	"github.com/anttimattila/lab/internal/db"
	"github.com/anttimattila/lab/internal/glab"
	gosync "github.com/anttimattila/lab/internal/sync"
	"github.com/anttimattila/lab/internal/tui"
	"github.com/spf13/cobra"
)
```

- [ ] **Step 2: Verify full build and run**

```bash
go build -o lab . && ./lab --help
```

- [ ] **Step 3: Commit**

```bash
git add cmd/root.go
git commit -m "feat: wire TUI into root command"
```

---

### Task 20: Background Sync in TUI

**Files:**
- Modify: `internal/tui/model.go` — add periodic background sync

- [ ] **Step 1: Add sync tick to model**

Add a tick command that fires every 5 minutes and triggers a sync:

```go
type syncTickMsg struct{}

func syncTick() tea.Cmd {
	return tea.Tick(5*time.Minute, func(time.Time) tea.Msg {
		return syncTickMsg{}
	})
}
```

In `Init()`:
```go
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.mrList.loadMRs(), syncTick())
}
```

Add a message type for sync completion:
```go
type bgSyncDoneMsg struct{}
```

In `Update()`, handle both messages:
```go
case syncTickMsg:
	return m, tea.Batch(
		func() tea.Msg {
			m.sync.SyncAll()
			return bgSyncDoneMsg{}
		},
		syncTick(),
	)
case bgSyncDoneMsg:
	// Refresh current view after background sync completes
	if m.current == viewMRList {
		return m, m.mrList.loadMRs()
	}
```

The sync runs in a Bubbletea command (not a bare goroutine), so the `bgSyncDoneMsg` triggers a UI refresh when complete.

- [ ] **Step 2: Verify build**

```bash
go build -o lab .
```

- [ ] **Step 3: Commit**

```bash
git add internal/tui/model.go
git commit -m "feat: add periodic background sync in TUI"
```

---

### Task 21: End-to-End Smoke Test

**Files:**
- Create: none (manual verification)

- [ ] **Step 1: Build the binary**

```bash
go build -o lab .
```

- [ ] **Step 2: Run help**

```bash
./lab --help
```

Expected: shows all subcommands (add, remove, list, sync, config, daemon).

- [ ] **Step 3: Run config**

```bash
./lab config set username testuser
./lab config get username
```

Expected: prints `username: testuser`

- [ ] **Step 4: Run list (empty)**

```bash
./lab list
```

Expected: "No repos registered."

- [ ] **Step 5: Run all tests**

```bash
go test ./... -v
```

Expected: all tests pass.

- [ ] **Step 6: Run vet and build check**

```bash
go vet ./...
```

Expected: no issues.

- [ ] **Step 7: Commit any fixes**

If any issues found, fix and commit.

```bash
git add -A
git commit -m "fix: address smoke test findings"
```
