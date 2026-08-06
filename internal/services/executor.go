package services

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/songguangzhi/webhook-ui/internal/models"
)

const executionTimeout = 5 * time.Minute

type Executor struct {
	allowedCommands []string
	tmpDir          string
	// logTailBytes caps each aggregated stream; 0 means unbounded.
	logTailBytes int
}

func NewExecutor(allowedCommands []string, dataDir string, logTailBytes int) *Executor {
	return &Executor{
		allowedCommands: allowedCommands,
		tmpDir:          filepath.Join(dataDir, "tmp"),
		logTailBytes:    logTailBytes,
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

func (e *Executor) Execute(hook *models.Hook, env map[string]string, args []string, sink LogSink) *ExecuteResult {
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
	return e.run(cmd, sink)
}

// ExecuteScript writes content to a temp file and runs it with the given
// interpreter. The interpreter binary must pass the command whitelist.
// workDir may be empty to inherit the current directory. sink may be nil,
// in which case output is only aggregated onto the result.
func (e *Executor) ExecuteScript(interpreter, content string, args []string, env map[string]string, workDir string, sink LogSink) *ExecuteResult {
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

	// Absolute: workDir changes the process cwd, so a relative tmp path
	// (DATA_DIR defaults to "./data") would no longer resolve.
	scriptPath, err := filepath.Abs(tmpFile.Name())
	if err != nil {
		return &ExecuteResult{Success: false, Error: err.Error()}
	}

	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.Command(binPath, cmdArgs...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	applyEnv(cmd, env)
	return e.run(cmd, sink)
}

func applyEnv(cmd *exec.Cmd, env map[string]string) {
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
}

// run starts cmd and streams both of its output streams into capture until it
// exits. Output reaches the sink while the process is still running, which is
// what lets a long execution be watched live instead of only after it ends.
func (e *Executor) run(cmd *exec.Cmd, sink LogSink) *ExecuteResult {
	capture := newStreamCapture(sink, e.logTailBytes)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return &ExecuteResult{Success: false, Error: err.Error()}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return &ExecuteResult{Success: false, Error: err.Error()}
	}

	if err := cmd.Start(); err != nil {
		return &ExecuteResult{Success: false, Error: err.Error()}
	}

	var readers sync.WaitGroup
	readers.Add(2)
	go pumpStream(&readers, capture, StreamStdout, stdout)
	go pumpStream(&readers, capture, StreamStderr, stderr)

	// Both pipes reach EOF when the process dies — including when it is killed
	// on timeout — so waiting on the readers first cannot outlive the process.
	done := make(chan error, 1)
	go func() {
		readers.Wait()
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		out, errOut := capture.result()
		result := &ExecuteResult{Output: out, Error: errOut}
		if err != nil {
			result.Success = false
			if result.Error == "" {
				result.Error = err.Error()
			}
		} else {
			result.Success = true
		}
		return result
	case <-time.After(executionTimeout):
		cmd.Process.Kill()
		<-done
		out, _ := capture.result()
		return &ExecuteResult{
			Success: false,
			Output:  out,
			Error:   "execution timeout (5 minutes)",
		}
	}
}

// pumpStream forwards everything r produces into capture, chunk by chunk.
func pumpStream(wg *sync.WaitGroup, capture *streamCapture, stream string, r io.Reader) {
	defer wg.Done()
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			capture.write(stream, string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}
