package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_MissingFileReturnsDefaults returns Default() when no file exists.
func TestLoad_MissingFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := Default()
	if cfg != want {
		t.Errorf("Load: want %+v, got %+v", want, cfg)
	}
}

// TestSaveLoadRoundTrip writes and reads the config back unchanged.
func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		SyncInterval: "15m",
		Username:     "alice",
		Notifications: Notifications{
			NewComment:       true,
			PipelineFailed:   false,
			Approved:         true,
			MRMerged:         false,
			NewReviewRequest: true,
			RereviewRequest:  false,
		},
	}
	if err := Save(dir, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != cfg {
		t.Errorf("round trip: want %+v, got %+v", cfg, got)
	}
}

// TestLoad_MissingFieldsInheritDefaults checks partial JSON.
func TestLoad_MissingFieldsInheritDefaults(t *testing.T) {
	dir := t.TempDir()
	// JSON with no notifications section and no sync_interval.
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte(`{"username": "bob"}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Username != "bob" {
		t.Errorf("Username: want bob, got %q", cfg.Username)
	}
	if cfg.SyncInterval != Default().SyncInterval {
		t.Errorf("SyncInterval: want default %q, got %q", Default().SyncInterval, cfg.SyncInterval)
	}
	// Notifications block was absent; the loader keeps the defaults
	// rather than zeroing the user out of all notifications.
	if cfg.Notifications != Default().Notifications {
		t.Errorf("Notifications: want defaults, got %+v", cfg.Notifications)
	}
}

// TestLoad_InvalidJSON returns Default() alongside an error so callers can
// log/notify and keep running with sane defaults instead of failing hard.
func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, Filename), []byte(`{not json`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg, err := Load(dir)
	if err == nil {
		t.Fatal("Load should report a parse error")
	}
	if cfg != Default() {
		t.Errorf("Load: want Default() on parse error, got %+v", cfg)
	}
}
