package services

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/songguangzhi/webhook-ui/internal/models"
	"golang.org/x/crypto/ssh"
)

const sshDialTimeout = 10 * time.Second

type DialResult struct {
	Client *ssh.Client
	// LearnedHostKey is set when no host key was pinned and the server's
	// key was trusted on first use (TOFU). Caller should persist it.
	LearnedHostKey string
}

// DialSSH connects to the host. If HostKey is set, it is strictly verified;
// otherwise the server's key is trusted on first use and returned for
// persistence.
func DialSSH(h *models.SSHHost) (*DialResult, error) {
	authMethod, err := sshAuthMethod(h)
	if err != nil {
		return nil, err
	}

	result := &DialResult{}
	hostKeyCallback, err := hostKeyCallback(h, result)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User:            h.User,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: hostKeyCallback,
		Timeout:         sshDialTimeout,
	}

	addr := net.JoinHostPort(h.Host, fmt.Sprintf("%d", h.Port))
	conn, err := net.DialTimeout("tcp", addr, sshDialTimeout)
	if err != nil {
		return nil, err
	}
	// Bound the handshake too; ClientConfig.Timeout only covers the TCP dial.
	conn.SetDeadline(time.Now().Add(sshDialTimeout))
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		if h.HostKey != "" && strings.Contains(err.Error(), "host key") {
			return nil, fmt.Errorf("服务器 Host Key 与已固定的不匹配（可能被劫持或服务器重装）: %w", err)
		}
		return nil, err
	}
	conn.SetDeadline(time.Time{})

	result.Client = ssh.NewClient(c, chans, reqs)
	return result, nil
}

func sshAuthMethod(h *models.SSHHost) (ssh.AuthMethod, error) {
	switch h.AuthType {
	case models.SSHAuthPassword:
		return ssh.Password(h.Credential), nil
	case models.SSHAuthKey:
		signer, err := ssh.ParsePrivateKey([]byte(h.Credential))
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return ssh.PublicKeys(signer), nil
	default:
		return nil, fmt.Errorf("unsupported auth_type: %s", h.AuthType)
	}
}

func hostKeyCallback(h *models.SSHHost, result *DialResult) (ssh.HostKeyCallback, error) {
	if h.HostKey != "" {
		pubKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(h.HostKey))
		if err != nil {
			return nil, fmt.Errorf("parse pinned host key: %w", err)
		}
		return ssh.FixedHostKey(pubKey), nil
	}
	// TOFU: accept any key, record it for the caller to persist.
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		result.LearnedHostKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
		return nil
	}, nil
}

// RunCommand runs a command over an existing connection and returns stdout;
// stderr is attached to the error on failure.
func RunCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if err := session.Run(command); err != nil {
		return stdout.String(), fmt.Errorf("%w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

// ExecuteScriptSSH runs script content on the remote host by piping it to
// the interpreter's stdin (bash -s / sh -s / python3 - on Linux, powershell
// -Command - on Windows). Nothing is written to the remote filesystem.
// Execution is bounded by opts.Timeout, as local execution is.
// workDir may be empty to run in the login directory; otherwise the remote
// shell cds into it first and the whole execution fails if the directory
// does not exist.
func ExecuteScriptSSH(client *ssh.Client, targetOS, interpreter, content string, args []string, env map[string]string, workDir string, opts ExecOptions) *ExecuteResult {
	if !models.IsInterpreterForOS(interpreter, targetOS) {
		return &ExecuteResult{Success: false, Error: fmt.Sprintf("interpreter %q cannot run on a %s target", interpreter, targetOS)}
	}
	if targetOS == models.TargetOSWindows {
		stdin := strings.NewReader(powershellPreamble(args, env, workDir) + content)
		return runSSHSession(client, windowsScriptCommand, stdin, opts)
	}
	return runSSHSession(client, sshScriptCommand(interpreter, args, env, workDir), strings.NewReader(content), opts)
}

// ExecuteCommandSSH runs a free-form command on the remote host. Unlike
// local execution there is no ALLOWED_COMMANDS whitelist — the whitelist
// describes binaries on the webhook server, not on the remote machine.
func ExecuteCommandSSH(client *ssh.Client, targetOS, command string, args []string, env map[string]string, workDir string, opts ExecOptions) *ExecuteResult {
	if strings.TrimSpace(command) == "" {
		return &ExecuteResult{Success: false, Error: "command is empty"}
	}
	if targetOS == models.TargetOSWindows {
		line, err := windowsCommandLine(command, args, env, workDir)
		if err != nil {
			return &ExecuteResult{Success: false, Error: err.Error()}
		}
		return runSSHSession(client, line, nil, opts)
	}
	return runSSHSession(client, sshCommandLine(command, args, env, workDir), nil, opts)
}

// runSSHSession runs remoteCmd and streams the remote host's output into out
// as it arrives, so a remote execution can be watched live exactly like a
// local one.
func runSSHSession(client *ssh.Client, remoteCmd string, stdin io.Reader, opts ExecOptions) *ExecuteResult {
	session, err := client.NewSession()
	if err != nil {
		return &ExecuteResult{Success: false, Error: err.Error()}
	}

	streams := newStreamCapture(opts)
	session.Stdin = stdin

	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return &ExecuteResult{Success: false, Error: err.Error()}
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		return &ExecuteResult{Success: false, Error: err.Error()}
	}

	if err := session.Start(remoteCmd); err != nil {
		session.Close()
		return &ExecuteResult{Success: false, Error: err.Error()}
	}

	var readers sync.WaitGroup
	readers.Add(2)
	go pumpStream(&readers, streams, StreamStdout, stdout)
	go pumpStream(&readers, streams, StreamStderr, stderr)

	done := make(chan error, 1)
	go func() {
		readers.Wait()
		done <- session.Wait()
	}()

	select {
	case err := <-done:
		session.Close()
		stdoutText, stderrText := streams.result()
		result := &ExecuteResult{Output: stdoutText, Error: stderrText}
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
		// Close only sends a channel-close message; the readers unblock when the
		// remote answers it, so a wedged host would keep `done` pending forever.
		// The timeout has to return regardless, exactly as the local branch does.
		session.Close()
		stdoutText, stderrText := streams.result()
		return &ExecuteResult{
			Success: false,
			Output:  stdoutText,
			Error:   strings.TrimLeft(stderrText+"\n"+timeoutMessage(opts.Timeout), "\n"),
		}
	}
}

// sshScriptCommand builds the remote command: env assignments prefix the
// interpreter call, args follow after the stdin-script flag.
func sshScriptCommand(interpreter string, args []string, env map[string]string, workDir string) string {
	var b strings.Builder
	b.WriteString(cdPrefix(workDir))
	b.WriteString(envPrefix(env))

	b.WriteString(interpreter)
	if interpreter == "python3" {
		b.WriteString(" -")
	} else {
		b.WriteString(" -s --")
	}
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(shellEscape(a))
	}
	return b.String()
}

// sshCommandLine builds the remote command for a free-form hook command.
// The command itself is passed through verbatim (it is operator-authored
// shell); only the args and env values are escaped.
func sshCommandLine(command string, args []string, env map[string]string, workDir string) string {
	var b strings.Builder
	b.WriteString(cdPrefix(workDir))
	b.WriteString(envPrefix(env))
	b.WriteString(command)
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(shellEscape(a))
	}
	return b.String()
}

func cdPrefix(workDir string) string {
	if workDir == "" {
		return ""
	}
	return "cd " + shellEscape(workDir) + " && "
}

// envPrefix renders env assignments, sorted for deterministic output. Keys
// that are not valid shell identifiers are skipped — query-derived keys come
// from the external caller and must never reach the shell raw.
var envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func envPrefix(env map[string]string) string {
	var b strings.Builder
	for _, k := range sortedEnvKeys(env) {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(shellEscape(env[k]))
		b.WriteByte(' ')
	}
	return b.String()
}

func sortedEnvKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		if envKeyPattern.MatchString(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// windowsScriptCommand is the remote command for a Windows target. Per
// about_PowerShell_exe, `-Command -` reads the script from stdin and runs it
// one statement at a time as though typed at the prompt: no param() block is
// honoured and no $args is bound, so workDir, env and args cannot ride on the
// command line — powershellPreamble injects them into the piped content.
const windowsScriptCommand = "powershell -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command -"

// powershellPreamble renders the statements prepended to the piped script.
func powershellPreamble(args []string, env map[string]string, workDir string) string {
	var b strings.Builder
	if workDir != "" {
		// Statements from stdin keep executing after a non-terminating error,
		// so a failed Set-Location has to abort explicitly — otherwise the
		// script body would run in the login directory instead.
		b.WriteString("Set-Location -LiteralPath " + psEscape(workDir) + "\n")
		b.WriteString("if (-not $?) { exit 1 }\n")
	}
	for _, k := range sortedEnvKeys(env) {
		b.WriteString("$env:" + k + " = " + psEscape(env[k]) + "\n")
	}
	b.WriteString("$args = @(")
	for i, a := range args {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(psEscape(a))
	}
	b.WriteString(")\n")
	return b.String()
}

// psEscape renders s as a PowerShell single-quoted string. The quote is the
// only metacharacter inside one — no $ or backtick expansion happens — so
// doubling it is a total escape.
func psEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// windowsCommandLine builds the remote command for a free-form hook command on
// a Windows target, whose default OpenSSH shell is cmd.exe.
func windowsCommandLine(command string, args []string, env map[string]string, workDir string) (string, error) {
	var b strings.Builder
	if workDir != "" {
		q, err := cmdQuote(workDir)
		if err != nil {
			return "", fmt.Errorf("working dir: %w", err)
		}
		b.WriteString("cd /d " + q + " && ")
	}
	for _, k := range sortedEnvKeys(env) {
		q, err := cmdQuote(k + "=" + env[k])
		if err != nil {
			return "", fmt.Errorf("env %s: %w", k, err)
		}
		b.WriteString("set " + q + " && ")
	}
	b.WriteString(command)
	for _, a := range args {
		q, err := cmdQuote(a)
		if err != nil {
			return "", fmt.Errorf("argument: %w", err)
		}
		b.WriteByte(' ')
		b.WriteString(q)
	}
	return b.String(), nil
}

// cmdQuote renders s as a cmd.exe double-quoted string. cmd.exe offers no
// escape for a double quote inside one, still expands %VAR% there, and treats
// a newline as the end of the command line — a payload-controlled value
// carrying any of those could break out of the quoting, so reject it rather
// than mangle it into something that silently runs.
func cmdQuote(s string) (string, error) {
	if strings.ContainsAny(s, "\"%\r\n") {
		return "", fmt.Errorf(`cmd.exe cannot safely quote a value containing " %% CR or LF: %q`, s)
	}
	return `"` + s + `"`, nil
}
