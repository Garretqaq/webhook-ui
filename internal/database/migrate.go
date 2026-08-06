package database

import "fmt"

const schemaVersion = 11

func Migrate() error {
	return migrateTo(schemaVersion)
}

func migrateTo(target int) error {
	var version int
	err := DB.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	if version >= target {
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
		{ // 4 -> 5
			`ALTER TABLE hooks ADD COLUMN script_id TEXT DEFAULT ''`,
		},
		{ // 5 -> 6
			`ALTER TABLE scripts ADD COLUMN ssh_host_id TEXT DEFAULT ''`,
		},
		{ // 6 -> 7: execution location belongs to the hook, not the script
			`ALTER TABLE hooks ADD COLUMN ssh_host_id TEXT DEFAULT ''`,
			`UPDATE hooks SET ssh_host_id = COALESCE(
				(SELECT ssh_host_id FROM scripts WHERE scripts.id = hooks.script_id), '')
			 WHERE script_id != ''`,
			`ALTER TABLE scripts DROP COLUMN ssh_host_id`,
			`ALTER TABLE executions ADD COLUMN exec_target TEXT DEFAULT ''`,
		},
		{ // 7 -> 8: remote command syntax depends on the target's OS
			`ALTER TABLE ssh_hosts ADD COLUMN target_os TEXT NOT NULL DEFAULT 'linux'`,
		},
		{ // 8 -> 9: output is streamed in chunks so a running execution can be watched
			`CREATE TABLE IF NOT EXISTS execution_logs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				execution_id INTEGER NOT NULL,
				seq INTEGER NOT NULL,
				stream TEXT NOT NULL,
				chunk TEXT NOT NULL,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (execution_id) REFERENCES executions(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX IF NOT EXISTS idx_execution_logs_exec_seq ON execution_logs(execution_id, seq)`,
		},
		{ // 9 -> 10: hooks can run asynchronously, with their own time budget
			`ALTER TABLE hooks ADD COLUMN async INTEGER NOT NULL DEFAULT 0`,
			// 300 rather than 0 so existing hooks keep the timeout they have
			// always had; 0 is reserved to mean "no limit".
			`ALTER TABLE hooks ADD COLUMN timeout_seconds INTEGER NOT NULL DEFAULT 300`,
		},
		{ // 10 -> 11: a single key-value store for instance-wide settings
			`CREATE TABLE IF NOT EXISTS settings (
				key TEXT PRIMARY KEY,
				value TEXT NOT NULL DEFAULT ''
			)`,
		},
	}

	for v := version; v < target; v++ {
		for i, m := range migrations[v] {
			if _, err := DB.Exec(m); err != nil {
				return fmt.Errorf("migration to v%d, step %d: %w", v+1, i, err)
			}
		}
	}

	if _, err := DB.Exec(fmt.Sprintf("PRAGMA user_version = %d", target)); err != nil {
		return fmt.Errorf("set schema version: %w", err)
	}

	return nil
}
