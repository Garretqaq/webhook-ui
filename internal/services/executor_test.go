package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not available")
	}
	return NewExecutor([]string{bashPath}, t.TempDir())
}

func TestExecuteScriptSuccess(t *testing.T) {
	e := newTestExecutor(t)
	result := e.ExecuteScript("bash", "echo hello $1; echo $MY_VAR", []string{"world"}, map[string]string{"MY_VAR": "42"}, "", OutputStream{})
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hello world") {
		t.Errorf("expected arg passed to script, got: %q", result.Output)
	}
	if !strings.Contains(result.Output, "42") {
		t.Errorf("expected env var passed to script, got: %q", result.Output)
	}
}

func TestExecuteScriptNonZeroExit(t *testing.T) {
	e := newTestExecutor(t)
	result := e.ExecuteScript("bash", "echo oops >&2; exit 1", nil, nil, "", OutputStream{})
	if result.Success {
		t.Fatal("expected failure for non-zero exit")
	}
	if !strings.Contains(result.Error, "oops") {
		t.Errorf("expected stderr captured, got: %q", result.Error)
	}
}

func TestExecuteScriptInterpreterNotAllowed(t *testing.T) {
	e := NewExecutor([]string{"/usr/bin/git"}, t.TempDir())
	result := e.ExecuteScript("bash", "echo hi", nil, nil, "", OutputStream{})
	if result.Success {
		t.Fatal("expected failure when interpreter not whitelisted")
	}
	if !strings.Contains(result.Error, "not allowed") {
		t.Errorf("expected whitelist error, got: %q", result.Error)
	}
}

func TestExecuteScriptWorkingDir(t *testing.T) {
	e := newTestExecutor(t)
	workDir := t.TempDir()
	result := e.ExecuteScript("bash", "pwd", nil, nil, workDir, OutputStream{})
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	// macOS /tmp symlinks to /private/tmp; compare resolved paths
	resolved, _ := filepath.EvalSymlinks(workDir)
	if !strings.Contains(result.Output, resolved) {
		t.Errorf("expected script to run in %s, got: %q", resolved, result.Output)
	}
}

func TestExecuteScriptCleansUpTempFile(t *testing.T) {
	e := newTestExecutor(t)
	result := e.ExecuteScript("bash", "echo done", nil, nil, "", OutputStream{})
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	entries, err := os.ReadDir(filepath.Join(e.tmpDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected temp dir cleaned, found %d entries", len(entries))
	}
}

func TestExecuteScriptRelativeTmpDirWithWorkDir(t *testing.T) {
	// DATA_DIR is relative by default; setting a hook working dir must not
	// break resolution of the temp script path.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	defer os.Chdir(cwd)

	e := NewExecutor([]string{"/bin", "/usr/bin"}, "./data")
	result := e.ExecuteScript("sh", "pwd", nil, nil, "/tmp", OutputStream{})
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "tmp") {
		t.Errorf("expected script to run in /tmp, got: %q", result.Output)
	}
}
