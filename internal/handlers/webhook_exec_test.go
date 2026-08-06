package handlers

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/models"
	"github.com/songguangzhi/webhook-ui/internal/services"
	_ "modernc.org/sqlite"
)

// setupExecDB gives each test a migrated database of its own.
func setupExecDB(t *testing.T) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	database.DB = db
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
}

// unreachableHost is a configured host that will always refuse the dial, so
// a test can tell the SSH branch was taken without running a real server.
func unreachableHost(t *testing.T, id string) {
	t.Helper()
	_, err := database.DB.Exec(`
		INSERT INTO ssh_hosts (id, name, host, port, user, auth_type, credential)
		VALUES (?, 'prod', '127.0.0.1', 1, 'deploy', 'password', 'x')
	`, id)
	if err != nil {
		t.Fatal(err)
	}
}

func newExecTestHandler(t *testing.T) *WebhookHandler {
	t.Helper()
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	return NewWebhookHandler(services.NewExecutor([]string{shPath}, t.TempDir()), 0, NewRunner(4, 16))
}

func TestExecuteScriptHookRunsLocallyWhenNoHost(t *testing.T) {
	setupExecDB(t)
	h := newExecTestHandler(t)
	if _, err := database.DB.Exec(`
		INSERT INTO scripts (id, name, interpreter, content) VALUES ('s1', 'greet', 'sh', 'echo hi')
	`); err != nil {
		t.Fatal(err)
	}

	result := h.execute(&models.Hook{ID: "h1", ScriptID: "s1"}, nil, nil, services.ExecOptions{})
	if !result.Success {
		t.Fatalf("expected local success, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hi") {
		t.Errorf("unexpected output: %q", result.Output)
	}
}

func TestExecuteCommandHookRunsLocallyWhenNoHost(t *testing.T) {
	setupExecDB(t)
	h := newExecTestHandler(t)

	// The local runner splits on whitespace rather than going through a
	// shell, so the command points at a file instead of quoting an inline -c.
	script := filepath.Join(t.TempDir(), "hi.sh")
	if err := os.WriteFile(script, []byte("echo hi\n"), 0700); err != nil {
		t.Fatal(err)
	}

	result := h.execute(&models.Hook{ID: "h1", Command: "sh " + script}, nil, nil, services.ExecOptions{})
	if !result.Success {
		t.Fatalf("expected local success, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hi") {
		t.Errorf("unexpected output: %q", result.Output)
	}
}

// The hook's ssh_host_id — not the script — decides the location, so both
// hook kinds must reach the SSH branch. A refused dial proves they did:
// local execution would have succeeded instead.
func TestExecuteRoutesToSSHWhenHookHasHost(t *testing.T) {
	setupExecDB(t)
	h := newExecTestHandler(t)
	unreachableHost(t, "host1")
	if _, err := database.DB.Exec(`
		INSERT INTO scripts (id, name, interpreter, content) VALUES ('s1', 'greet', 'sh', 'echo hi')
	`); err != nil {
		t.Fatal(err)
	}

	cases := map[string]*models.Hook{
		"script":  {ID: "h1", ScriptID: "s1", SSHHostID: "host1"},
		"command": {ID: "h2", Command: "sh /bin/nonexistent", SSHHostID: "host1"},
	}
	for name, hook := range cases {
		t.Run(name, func(t *testing.T) {
			result := h.execute(hook, nil, nil, services.ExecOptions{})
			if result.Success {
				t.Fatal("expected the SSH dial to fail, but execution succeeded — it ran locally")
			}
			if !strings.Contains(result.Error, "dial") {
				t.Errorf("expected a dial error, got: %q", result.Error)
			}
		})
	}
}

func TestExecTargetSnapshot(t *testing.T) {
	setupExecDB(t)
	unreachableHost(t, "host1")

	if got := execTarget(""); got != "local" {
		t.Errorf("execTarget(\"\") = %q, want \"local\"", got)
	}
	if got := execTarget("host1"); got != "deploy@127.0.0.1:1" {
		t.Errorf("execTarget(\"host1\") = %q, want \"deploy@127.0.0.1:1\"", got)
	}
}
