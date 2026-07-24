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
}

func NewExecutor(allowedCommands []string) *Executor {
	return &Executor{
		allowedCommands: allowedCommands,
	}
}

func (e *Executor) isAllowed(cmd string) bool {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}
	baseCmd := parts[0]

	absPath, err := filepath.Abs(baseCmd)
	if err != nil {
		absPath = baseCmd
	}

	for _, allowed := range e.allowedCommands {
		if absPath == allowed || strings.HasPrefix(absPath, allowed+string(os.PathSeparator)) {
			return true
		}
		if strings.HasPrefix(absPath, allowed) {
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

	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

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
