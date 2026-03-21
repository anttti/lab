package db

import (
	"testing"
)

func TestAddRepo(t *testing.T) {
	db := testDB(t)

	repo, err := db.AddRepo("/home/user/myrepo", "https://gitlab.com/user/myrepo", "myrepo")
	if err != nil {
		t.Fatalf("AddRepo: %v", err)
	}
	if repo.ID == 0 {
		t.Error("expected non-zero ID after insert")
	}
	if repo.Path != "/home/user/myrepo" {
		t.Errorf("Path = %q, want %q", repo.Path, "/home/user/myrepo")
	}
	if repo.GitLabURL != "https://gitlab.com/user/myrepo" {
		t.Errorf("GitLabURL = %q", repo.GitLabURL)
	}
	if repo.Name != "myrepo" {
		t.Errorf("Name = %q", repo.Name)
	}
}

func TestAddRepo_DuplicatePath(t *testing.T) {
	db := testDB(t)

	if _, err := db.AddRepo("/dup/path", "https://example.com", "repo"); err != nil {
		t.Fatalf("first AddRepo: %v", err)
	}
	_, err := db.AddRepo("/dup/path", "https://example.com", "repo")
	if err == nil {
		t.Error("expected error on duplicate path, got nil")
	}
}

func TestListRepos(t *testing.T) {
	db := testDB(t)

	paths := []string{"/z/repo", "/a/repo", "/m/repo"}
	names := []string{"z-repo", "a-repo", "m-repo"}
	for i, p := range paths {
		if _, err := db.AddRepo(p, "https://example.com", names[i]); err != nil {
			t.Fatalf("AddRepo %q: %v", p, err)
		}
	}

	repos, err := db.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(repos))
	}
	// Should be ordered by name: a-repo, m-repo, z-repo
	if repos[0].Name != "a-repo" || repos[1].Name != "m-repo" || repos[2].Name != "z-repo" {
		t.Errorf("expected alphabetical order, got %v", []string{repos[0].Name, repos[1].Name, repos[2].Name})
	}
}

func TestRemoveRepo(t *testing.T) {
	db := testDB(t)

	if _, err := db.AddRepo("/to/remove", "https://example.com", "repo"); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	if err := db.RemoveRepo("/to/remove"); err != nil {
		t.Fatalf("RemoveRepo: %v", err)
	}

	repos, err := db.ListRepos()
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos after remove, got %d", len(repos))
	}
}

func TestRemoveRepo_NotFound(t *testing.T) {
	db := testDB(t)

	err := db.RemoveRepo("/nonexistent")
	if err == nil {
		t.Error("expected error when removing non-existent repo, got nil")
	}
}
