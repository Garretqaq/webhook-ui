package database

import "fmt"

const schemaVersion = 1

func Migrate() error {
	var version int
	err := DB.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	if version >= schemaVersion {
		return nil
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS hooks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			command TEXT NOT NULL,
			working_dir TEXT DEFAULT '',
			response_message TEXT DEFAULT 'OK',
			hmac_secret TEXT DEFAULT '',
			hmac_algorithm TEXT DEFAULT 'sha256',
			pass_arguments TEXT DEFAULT '',
			pass_headers TEXT DEFAULT '',
			pass_payload_to TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS executions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			hook_id TEXT NOT NULL,
			trigger_source TEXT DEFAULT '',
			status TEXT NOT NULL,
			output TEXT DEFAULT '',
			error TEXT DEFAULT '',
			started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			finished_at DATETIME,
			FOREIGN KEY (hook_id) REFERENCES hooks(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_hook_id ON executions(hook_id)`,
		`CREATE INDEX IF NOT EXISTS idx_executions_started_at ON executions(started_at DESC)`,
	}

	for i, m := range migrations {
		if _, err := DB.Exec(m); err != nil {
			return fmt.Errorf("migration %d: %w", i, err)
		}
	}

	if _, err := DB.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return nil
}
