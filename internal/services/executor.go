package services

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/songguangzhi/webhook-ui/internal/models"
)

type Executor struct {
	allowedCommands []string
	tmpDir          string
}

func NewExecutor(allowedCommands []string, dataDir string) *Executor {
	return &Executor{
		allowedCommands: allowedCommands,
		tmpDir:          filepath.Join(dataDir, "tmp"),
	}
}

func (e *Executor) isAllowed(cmd string) bool {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}
	baseCmd := parts[0]

	// Resolve to absolute path: if no slash, look up in PATH
	if !strings.Contains(baseCmd, "/") {
		resolved, err := exec.LookPath(baseCmd)
		if err != nil {
			return false
		}
		baseCmd = resolved
	}

	absPath, err := filepath.Abs(baseCmd)
	if err != nil {
		return false
	}

	for _, allowed := range e.allowedCommands {
		if absPath == allowed {
			return true
		}
		// Directory prefix match: allowed is a directory, command inside it
		if strings.HasPrefix(absPath, allowed+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

type ExecuteResult struct {
	Output  string
	Error   string
	Success bool
}

func (e *Executor) Execute(hook *models.Hook, env map[string]string, args []string) *ExecuteResult {
	if !e.isAllowed(hook.Command) {
		return &ExecuteResult{
			Success: false,
			Error:   fmt.Sprintf("command not allowed: %s", hook.Command),
		}
	}

	cmdParts := strings.Fields(hook.Command)
	cmdParts = append(cmdParts, args...)
	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)

	if hook.WorkingDir != "" {
		cmd.Dir = hook.WorkingDir
	}

	applyEnv(cmd, env)
	return runWithTimeout(cmd)
}

// ExecuteScript writes content to a temp file and runs it with the given
// interpreter. The interpreter binary must pass the command whitelist.
// workDir may be empty to inherit the current directory.
func (e *Executor) ExecuteScript(interpreter, content string, args []string, env map[string]string, workDir string) *ExecuteResult {
	if !e.isAllowed(interpreter) {
		return &ExecuteResult{
			Success: false,
			Error:   fmt.Sprintf("interpreter not allowed: %s", interpreter),
		}
	}

	binPath, err := exec.LookPath(interpreter)
	if err != nil {
		return &ExecuteResult{
			Success: false,
			Error:   fmt.Sprintf("interpreter not found: %s", interpreter),
		}
	}

	if err := os.MkdirAll(e.tmpDir, 0700); err != nil {
		return &ExecuteResult{Success: false, Error: err.Error()}
	}
	tmpFile, err := os.CreateTemp(e.tmpDir, "script-*")
	if err != nil {
		return &ExecuteResult{Success: false, Error: err.Error()}
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return &ExecuteResult{Success: false, Error: err.Error()}
	}
	if err := tmpFile.Close(); err != nil {
		return &ExecuteResult{Success: false, Error: err.Error()}
	}
	if err := os.Chmod(tmpFile.Name(), 0700); err != nil {
		return &ExecuteResult{Success: false, Error: err.Error()}
	}

	cmdArgs := append([]string{tmpFile.Name()}, args...)
	cmd := exec.Command(binPath, cmdArgs...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	applyEnv(cmd, env)
	return runWithTimeout(cmd)
}

func applyEnv(cmd *exec.Cmd, env map[string]string) {
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
}

func runWithTimeout(cmd *exec.Cmd) *ExecuteResult {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		result := &ExecuteResult{
			Output: stdout.String(),
			Error:  stderr.String(),
		}
		if err != nil {
			result.Success = false
			if result.Error == "" {
				result.Error = err.Error()
			}
		} else {
			result.Success = true
		}
		return result
	case <-time.After(5 * time.Minute):
		cmd.Process.Kill()
		return &ExecuteResult{
			Success: false,
			Error:   "execution timeout (5 minutes)",
		}
	}
}
