package db

import "testing"

func TestConfigSetGet(t *testing.T) {
	db := testDB(t)

	if err := db.SetConfig("gitlab.token", "secret123"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	val, err := db.GetConfig("gitlab.token")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "secret123" {
		t.Errorf("GetConfig = %q, want %q", val, "secret123")
	}
}

func TestConfigSetOverwrites(t *testing.T) {
	db := testDB(t)

	_ = db.SetConfig("key", "first")
	if err := db.SetConfig("key", "second"); err != nil {
		t.Fatalf("SetConfig overwrite: %v", err)
	}
	val, _ := db.GetConfig("key")
	if val != "second" {
		t.Errorf("GetConfig = %q, want %q", val, "second")
	}
}

func TestConfigGetMissing(t *testing.T) {
	db := testDB(t)

	val, err := db.GetConfig("nonexistent.key")
	if err != nil {
		t.Fatalf("GetConfig missing key returned error: %v", err)
	}
	if val != "" {
		t.Errorf("GetConfig missing = %q, want empty string", val)
	}
}
