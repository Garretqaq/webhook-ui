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
	result := e.ExecuteScript("bash", "echo hello $1; echo $MY_VAR", []string{"world"}, map[string]string{"MY_VAR": "42"})
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
	result := e.ExecuteScript("bash", "echo oops >&2; exit 1", nil, nil)
	if result.Success {
		t.Fatal("expected failure for non-zero exit")
	}
	if !strings.Contains(result.Error, "oops") {
		t.Errorf("expected stderr captured, got: %q", result.Error)
	}
}

func TestExecuteScriptInterpreterNotAllowed(t *testing.T) {
	e := NewExecutor([]string{"/usr/bin/git"}, t.TempDir())
	result := e.ExecuteScript("bash", "echo hi", nil, nil)
	if result.Success {
		t.Fatal("expected failure when interpreter not whitelisted")
	}
	if !strings.Contains(result.Error, "not allowed") {
		t.Errorf("expected whitelist error, got: %q", result.Error)
	}
}

func TestExecuteScriptCleansUpTempFile(t *testing.T) {
	e := newTestExecutor(t)
	result := e.ExecuteScript("bash", "echo done", nil, nil)
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
