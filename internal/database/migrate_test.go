package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// openTestDB opens a fresh file-backed database and points the package
// global at it for the duration of the test.
func openTestDB(t *testing.T) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	DB = db
}

func TestMigrateToLatest(t *testing.T) {
	openTestDB(t)
	if err := Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var version int
	if err := DB.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != schemaVersion {
		t.Errorf("user_version = %d, want %d", version, schemaVersion)
	}

	// scripts no longer carry an execution location
	if _, err := DB.Exec("SELECT ssh_host_id FROM scripts"); err == nil {
		t.Error("scripts.ssh_host_id should have been dropped")
	}
}

func TestMigrateV7BackfillsHookExecLocation(t *testing.T) {
	openTestDB(t)
	if err := migrateTo(6); err != nil {
		t.Fatalf("migrate to v6: %v", err)
	}

	mustExec(t, `INSERT INTO ssh_hosts (id, name, host, port, user, auth_type, credential)
	             VALUES ('h1', 'prod', '10.0.0.1', 22, 'deploy', 'password', 'x')`)
	mustExec(t, `INSERT INTO scripts (id, name, interpreter, content, ssh_host_id)
	             VALUES ('s-remote', 'deploy', 'bash', 'echo hi', 'h1')`)
	mustExec(t, `INSERT INTO scripts (id, name, interpreter, content, ssh_host_id)
	             VALUES ('s-local', 'cleanup', 'bash', 'echo hi', '')`)
	mustExec(t, `INSERT INTO hooks (id, name, command, script_id) VALUES ('hk-remote', 'a', '', 's-remote')`)
	mustExec(t, `INSERT INTO hooks (id, name, command, script_id) VALUES ('hk-remote2', 'b', '', 's-remote')`)
	mustExec(t, `INSERT INTO hooks (id, name, command, script_id) VALUES ('hk-local', 'c', '', 's-local')`)
	mustExec(t, `INSERT INTO hooks (id, name, command, script_id) VALUES ('hk-cmd', 'd', 'echo hi', '')`)

	if err := Migrate(); err != nil {
		t.Fatalf("migrate to v7: %v", err)
	}

	want := map[string]string{
		"hk-remote":  "h1",
		"hk-remote2": "h1", // one script, many hooks — each inherits the host
		"hk-local":   "",
		"hk-cmd":     "",
	}
	for id, wantHost := range want {
		var got string
		if err := DB.QueryRow("SELECT ssh_host_id FROM hooks WHERE id = ?", id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != wantHost {
			t.Errorf("hook %s ssh_host_id = %q, want %q", id, got, wantHost)
		}
	}
}

func TestMigrateV8DefaultsExistingHostsToLinux(t *testing.T) {
	openTestDB(t)
	if err := migrateTo(7); err != nil {
		t.Fatalf("migrate to v7: %v", err)
	}
	mustExec(t, `INSERT INTO ssh_hosts (id, name, host, port, user, auth_type, credential)
	             VALUES ('h1', 'prod', '10.0.0.1', 22, 'deploy', 'password', 'x')`)

	if err := Migrate(); err != nil {
		t.Fatalf("migrate to v8: %v", err)
	}

	var targetOS string
	if err := DB.QueryRow("SELECT target_os FROM ssh_hosts WHERE id = 'h1'").Scan(&targetOS); err != nil {
		t.Fatal(err)
	}
	if targetOS != "linux" {
		t.Errorf("pre-existing host target_os = %q, want linux", targetOS)
	}
}

func mustExec(t *testing.T, query string) {
	t.Helper()
	if _, err := DB.Exec(query); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}
