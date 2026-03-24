package db

import (
	"database/sql"
	"errors"
	"fmt"
)

// SetConfig inserts or updates a key/value pair in the config table.
func (db *Database) SetConfig(key, value string) error {
	const q = `
INSERT INTO config (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	if _, err := db.Exec(q, key, value); err != nil {
		return fmt.Errorf("SetConfig: %w", err)
	}
	return nil
}

// GetConfig returns the value for the given key, or "" if the key does not exist.
func (db *Database) GetConfig(key string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM config WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("GetConfig: %w", err)
	}
	return value, nil
}

// GetConfigByPrefix returns all config entries whose key starts with the given prefix.
func (db *Database) GetConfigByPrefix(prefix string) (map[string]string, error) {
	rows, err := db.Query(`SELECT key, value FROM config WHERE key LIKE ?`, prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("GetConfigByPrefix: %w", err)
	}
	defer rows.Close()
	result := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("GetConfigByPrefix: %w", err)
		}
		result[k] = v
	}
	return result, rows.Err()
}
