package services

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strings"
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
// the interpreter's stdin (bash -s / sh -s / python3 -). Nothing is written
// to the remote filesystem. Execution is bounded by the same 5 minute
// timeout as local execution. workDir may be empty to run in the login
// directory; otherwise the remote shell cds into it first and the whole
// execution fails if the directory does not exist.
func ExecuteScriptSSH(client *ssh.Client, interpreter, content string, args []string, env map[string]string, workDir string) *ExecuteResult {
	if !models.IsValidInterpreter(interpreter) {
		return &ExecuteResult{Success: false, Error: fmt.Sprintf("invalid interpreter: %s", interpreter)}
	}
	return runSSHSession(client, sshScriptCommand(interpreter, args, env, workDir), strings.NewReader(content))
}

// ExecuteCommandSSH runs a free-form command on the remote host. Unlike
// local execution there is no ALLOWED_COMMANDS whitelist — the whitelist
// describes binaries on the webhook server, not on the remote machine.
func ExecuteCommandSSH(client *ssh.Client, command string, args []string, env map[string]string, workDir string) *ExecuteResult {
	if strings.TrimSpace(command) == "" {
		return &ExecuteResult{Success: false, Error: "command is empty"}
	}
	return runSSHSession(client, sshCommandLine(command, args, env, workDir), nil)
}

func runSSHSession(client *ssh.Client, remoteCmd string, stdin io.Reader) *ExecuteResult {
	session, err := client.NewSession()
	if err != nil {
		return &ExecuteResult{Success: false, Error: err.Error()}
	}

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	session.Stdin = stdin

	done := make(chan error, 1)
	go func() {
		done <- session.Run(remoteCmd)
	}()

	select {
	case err := <-done:
		session.Close()
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
		session.Close()
		return &ExecuteResult{
			Success: false,
			Error:   "execution timeout (5 minutes)",
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
	keys := make([]string, 0, len(env))
	for k := range env {
		if envKeyPattern.MatchString(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(shellEscape(env[k]))
		b.WriteByte(' ')
	}
	return b.String()
}

func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
