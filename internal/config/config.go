// Package config reads and writes the lab user-configuration file.
//
// The canonical location is ~/.config/lab/lab.json. Fields that aren't
// present in the file fall back to the hard-coded defaults from Default().
// Callers that need per-user values (username, sync_interval) can still
// fall back to the SQLite config table via LoadWithDBFallback.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Filename is the JSON file lab reads and writes in the data directory.
const Filename = "lab.json"

// Config is the user-facing configuration stored in lab.json.
type Config struct {
	SyncInterval  string        `json:"sync_interval"`
	Username      string        `json:"username"`
	Notifications Notifications `json:"notifications"`
}

// Notifications toggles which MR changes produce a desktop notification
// when the daemon runs in loop mode.
type Notifications struct {
	NewComment       bool `json:"new_comment"`
	PipelineFailed   bool `json:"pipeline_failed"`
	Approved         bool `json:"approved"`
	MRMerged         bool `json:"mr_merged"`
	NewReviewRequest bool `json:"new_review_request"`
	RereviewRequest  bool `json:"rereview_request"`
}

// Default returns the default configuration: 10 minute sync interval and
// every notification trigger enabled.
func Default() Config {
	return Config{
		SyncInterval: "10m",
		Notifications: Notifications{
			NewComment:       true,
			PipelineFailed:   true,
			Approved:         true,
			MRMerged:         true,
			NewReviewRequest: true,
			RereviewRequest:  true,
		},
	}
}

// Path returns the JSON file path inside dataDir.
func Path(dataDir string) string {
	return filepath.Join(dataDir, Filename)
}

// Load reads lab.json from dataDir. If the file does not exist, Default() is
// returned with no error. Missing individual fields inherit defaults.
func Load(dataDir string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(Path(dataDir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read %s: %w", Path(dataDir), err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", Path(dataDir), err)
	}
	if cfg.SyncInterval == "" {
		cfg.SyncInterval = Default().SyncInterval
	}
	return cfg, nil
}

// Save writes cfg to lab.json in dataDir, creating the file if necessary.
// Existing content is overwritten atomically via a temp file rename.
func Save(dataDir string, cfg Config) error {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dataDir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	target := Path(dataDir)
	tmp, err := os.CreateTemp(dataDir, ".lab.json.*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}
