package database

import "fmt"

const schemaVersion = 4

func Migrate() error {
	var version int
	err := DB.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	if version >= schemaVersion {
		return nil
	}

	// migrations[i] upgrades schema from version i to version i+1
	migrations := [][]string{
		{ // 0 -> 1
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
			`CREATE INDEX IF NOT EXISTS idx_executions_hook_id ON executions(hook_id)`,
			`CREATE INDEX IF NOT EXISTS idx_executions_started_at ON executions(started_at DESC)`,
		},
		{ // 1 -> 2
			`ALTER TABLE hooks ADD COLUMN trigger_token TEXT DEFAULT ''`,
		},
		{ // 2 -> 3
			`CREATE TABLE IF NOT EXISTS scripts (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				interpreter TEXT NOT NULL DEFAULT 'bash',
				content TEXT NOT NULL,
				description TEXT DEFAULT '',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
		},
		{ // 3 -> 4
			`CREATE TABLE IF NOT EXISTS ssh_hosts (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				host TEXT NOT NULL,
				port INTEGER NOT NULL DEFAULT 22,
				user TEXT NOT NULL,
				auth_type TEXT NOT NULL DEFAULT 'key',
				credential TEXT NOT NULL,
				host_key TEXT DEFAULT '',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)`,
		},
	}

	for v := version; v < schemaVersion; v++ {
		for i, m := range migrations[v] {
			if _, err := DB.Exec(m); err != nil {
				return fmt.Errorf("migration to v%d, step %d: %w", v+1, i, err)
			}
		}
	}

	if _, err := DB.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return nil
}
