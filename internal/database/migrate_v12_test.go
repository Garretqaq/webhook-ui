package database

import "testing"

// TestMigrateV12ClearsTriggerTokenFromMixedAuthHooks covers the migration
// that resolves hooks configured with both an HMAC secret and a fixed
// trigger token: HMAC wins, the token is dropped.
func TestMigrateV12ClearsTriggerTokenFromMixedAuthHooks(t *testing.T) {
	openTestDB(t)
	if err := migrateTo(11); err != nil {
		t.Fatalf("migrate to v11: %v", err)
	}

	mustExec(t, `INSERT INTO hooks (id, name, command, hmac_secret, trigger_token)
	             VALUES ('hk-both', 'a', 'echo hi', 'sec', 'tok')`)
	mustExec(t, `INSERT INTO hooks (id, name, command, hmac_secret, trigger_token)
	             VALUES ('hk-token', 'b', 'echo hi', '', 'tok')`)
	mustExec(t, `INSERT INTO hooks (id, name, command, hmac_secret, trigger_token)
	             VALUES ('hk-hmac', 'c', 'echo hi', 'sec', '')`)
	mustExec(t, `INSERT INTO hooks (id, name, command, hmac_secret, trigger_token)
	             VALUES ('hk-none', 'd', 'echo hi', '', '')`)

	if err := Migrate(); err != nil {
		t.Fatalf("migrate to latest: %v", err)
	}

	want := map[string][2]string{
		"hk-both":  {"sec", ""}, // mixed hook: HMAC kept, token dropped
		"hk-token": {"", "tok"},
		"hk-hmac":  {"sec", ""},
		"hk-none":  {"", ""},
	}
	for id, w := range want {
		var secret, token string
		if err := DB.QueryRow(
			"SELECT hmac_secret, trigger_token FROM hooks WHERE id = ?", id,
		).Scan(&secret, &token); err != nil {
			t.Fatal(err)
		}
		if secret != w[0] || token != w[1] {
			t.Errorf("hook %s = (%q, %q), want (%q, %q)", id, secret, token, w[0], w[1])
		}
	}

	// Re-running the migration must be a no-op, not an error.
	if err := Migrate(); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}
