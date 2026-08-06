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

// DefaultTimeout bounds an execution whose hook does not say otherwise.
const DefaultTimeout = 5 * time.Minute

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
	// Canceled distinguishes an operator stopping the run from the script
	// failing on its own, which the execution log has to be able to show.
	Canceled bool
}

func (e *Executor) Execute(hook *models.Hook, env map[string]string, args []string, opts ExecOptions) *ExecuteResult {
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
	return e.run(cmd, opts)
}

// ExecuteScript writes content to a temp file and runs it with the given
// interpreter. The interpreter binary must pass the command whitelist.
// workDir may be empty to inherit the current directory. A zero ExecOptions
// means output is only aggregated onto the result, uncapped.
func (e *Executor) ExecuteScript(interpreter, content string, args []string, env map[string]string, workDir string, opts ExecOptions) *ExecuteResult {
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
	return e.run(cmd, opts)
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
func (e *Executor) run(cmd *exec.Cmd, opts ExecOptions) *ExecuteResult {
	capture := newStreamCapture(opts)
	setProcessGroup(cmd)

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
	case <-timeoutChan(opts.Timeout):
		return e.abort(cmd, capture, timeoutMessage(opts.Timeout), false)
	case <-opts.Cancel:
		return e.abort(cmd, capture, "execution canceled", true)
	}
}

// abort kills the process group and returns whatever it produced first.
//
// It does not wait for the readers. They only see EOF once every descendant
// has closed the pipes, and a descendant that ignores the signal would
// otherwise keep a timeout or a cancellation pending indefinitely — which is
// exactly what neither is allowed to do. capture is mutex-guarded, so reading
// it while the orphaned readers may still be writing is safe; they exit on
// their own.
func (e *Executor) abort(cmd *exec.Cmd, capture *streamCapture, reason string, canceled bool) *ExecuteResult {
	killProcessTree(cmd)

	out, errOut := capture.result()
	return &ExecuteResult{
		Success:  false,
		Canceled: canceled,
		Output:   out,
		Error:    strings.TrimLeft(errOut+"\n"+reason, "\n"),
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
