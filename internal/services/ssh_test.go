package services

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/songguangzhi/webhook-ui/internal/models"
	"golang.org/x/crypto/ssh"
)

// startTestServer runs an in-process SSH server accepting password
// "secret" for any user, and returns its listener address and host key.
// "echo ok" replies ok; any other command echoes its stdin back.
func startTestServer(t *testing.T) (addr string, hostKey ssh.PublicKey) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	hostKey = signer.PublicKey()

	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if string(pass) == "secret" {
				return nil, nil
			}
			return nil, ssh.ErrNoAuth
		},
	}
	config.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					if ch.ChannelType() != "session" {
						ch.Reject(ssh.UnknownChannelType, "only session")
						continue
					}
					channel, requests, err := ch.Accept()
					if err != nil {
						continue
					}
					go func() {
						for req := range requests {
							if req.Type == "exec" {
								req.Reply(true, nil)
								cmd := string(req.Payload[4:]) // skip uint32 length prefix
								if cmd == "echo ok" {
									channel.Write([]byte("ok\n"))
								} else if strings.Contains(cmd, "SLOWTEST") {
									// Emit in stages so a test can observe output
									// arriving before the session ends.
									channel.Write([]byte("first\n"))
									channel.Stderr().Write([]byte("err1\n"))
									time.Sleep(700 * time.Millisecond)
									channel.Write([]byte("second\n"))
								} else if strings.Contains(cmd, "BINARYTEST") {
									// Bytes that are not valid UTF-8, as a GBK
									// Windows host would produce.
									channel.Write([]byte{0xff, 0xfe, 'o', 'k', '\n'})
								} else {
									io.Copy(channel, channel) // echo stdin
								}
								channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
								channel.Close()
							} else {
								req.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()

	return ln.Addr().String(), hostKey
}

func testHost(addr string) *models.SSHHost {
	host, port := splitAddr(addr)
	return &models.SSHHost{
		Host:       host,
		Port:       port,
		User:       "tester",
		AuthType:   models.SSHAuthPassword,
		Credential: "secret",
	}
}

func splitAddr(addr string) (string, int) {
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	return host, port
}

func TestDialTOFULearnsHostKey(t *testing.T) {
	addr, serverKey := startTestServer(t)
	h := testHost(addr)

	result, err := DialSSH(h)
	if err != nil {
		t.Fatalf("first dial should succeed (TOFU learn), got: %v", err)
	}
	defer result.Client.Close()

	if result.LearnedHostKey == "" {
		t.Fatal("expected learned host key on first connect")
	}
	if !strings.Contains(result.LearnedHostKey, serverKey.Type()) {
		t.Errorf("learned key type mismatch: %q", result.LearnedHostKey)
	}
}

func TestDialPinnedKeyMatch(t *testing.T) {
	addr, serverKey := startTestServer(t)
	h := testHost(addr)
	h.HostKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(serverKey)))

	result, err := DialSSH(h)
	if err != nil {
		t.Fatalf("dial with matching pinned key should succeed, got: %v", err)
	}
	defer result.Client.Close()
	if result.LearnedHostKey != "" {
		t.Error("no new key should be learned when pinned key matches")
	}
}

func TestDialPinnedKeyMismatch(t *testing.T) {
	addr, _ := startTestServer(t)
	_, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	otherSigner, _ := ssh.NewSignerFromKey(otherPriv)

	h := testHost(addr)
	h.HostKey = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(otherSigner.PublicKey())))

	_, err := DialSSH(h)
	if err == nil {
		t.Fatal("dial with mismatched pinned key must fail")
	}
	if !strings.Contains(err.Error(), "host key") {
		t.Errorf("error should mention host key, got: %v", err)
	}
}

func TestDialBadPassword(t *testing.T) {
	addr, _ := startTestServer(t)
	h := testHost(addr)
	h.Credential = "wrong"

	_, err := DialSSH(h)
	if err == nil {
		t.Fatal("dial with wrong password must fail")
	}
}

func TestExecuteScriptSSHPipesContentViaStdin(t *testing.T) {
	addr, _ := startTestServer(t)
	h := testHost(addr)

	result, err := DialSSH(h)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Client.Close()

	content := "echo hello from script"
	execResult := ExecuteScriptSSH(result.Client, models.TargetOSLinux, "bash", content, nil, nil, "", ExecOptions{})
	if !execResult.Success {
		t.Fatalf("expected success, got: %s", execResult.Error)
	}
	if !strings.Contains(execResult.Output, content) {
		t.Errorf("expected script content piped via stdin, got: %q", execResult.Output)
	}
}

func TestExecuteScriptSSHBuildsCommand(t *testing.T) {
	// bash/sh get "-s", python3 gets "-"
	if got := sshScriptCommand("bash", nil, nil, ""); !strings.HasPrefix(got, "bash -s --") {
		t.Errorf("bash command prefix wrong: %q", got)
	}
	if got := sshScriptCommand("python3", nil, nil, ""); !strings.HasPrefix(got, "python3 -") {
		t.Errorf("python3 command prefix wrong: %q", got)
	}
}

func TestSSHScriptCommandWorkDir(t *testing.T) {
	got := sshScriptCommand("bash", nil, map[string]string{"A": "1"}, "/srv/app")
	if !strings.HasPrefix(got, "cd '/srv/app' && ") {
		t.Errorf("workDir should prefix the command: %q", got)
	}
	if !strings.Contains(got, "A='1' bash -s --") {
		t.Errorf("env should still precede interpreter: %q", got)
	}

	// a workDir containing a quote must not break out of the cd
	got = sshScriptCommand("bash", nil, nil, "/tmp/it's")
	if !strings.HasPrefix(got, `cd '/tmp/it'\''s' && `) {
		t.Errorf("workDir not escaped: %q", got)
	}
}

func TestSSHCommandLine(t *testing.T) {
	got := sshCommandLine("deploy.sh", []string{"a b"}, map[string]string{"A": "1"}, "/srv/app")
	want := "cd '/srv/app' && A='1' deploy.sh 'a b'"
	if got != want {
		t.Errorf("sshCommandLine = %q, want %q", got, want)
	}

	if got := sshCommandLine("echo ok", nil, nil, ""); got != "echo ok" {
		t.Errorf("bare command should pass through, got %q", got)
	}
}

func TestExecuteCommandSSH(t *testing.T) {
	addr, _ := startTestServer(t)
	h := testHost(addr)

	result, err := DialSSH(h)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Client.Close()

	execResult := ExecuteCommandSSH(result.Client, models.TargetOSLinux, "echo ok", nil, nil, "", ExecOptions{})
	if !execResult.Success {
		t.Fatalf("expected success, got: %s", execResult.Error)
	}
	if !strings.Contains(execResult.Output, "ok") {
		t.Errorf("expected command output, got: %q", execResult.Output)
	}
}

func TestExecuteCommandSSHRejectsEmptyCommand(t *testing.T) {
	if r := ExecuteCommandSSH(nil, models.TargetOSLinux, "   ", nil, nil, "", ExecOptions{}); r.Success {
		t.Fatal("empty command must be rejected before dialing a session")
	}
}

func TestShellEscape(t *testing.T) {
	cases := map[string]string{
		"simple":     "'simple'",
		"with space": "'with space'",
		"it's":       "'it'\\''s'",
		"":           "''",
	}
	for in, want := range cases {
		if got := shellEscape(in); got != want {
			t.Errorf("shellEscape(%q) = %q, want %q", in, got, want)
		}
	}

	// env and args are escaped into the command
	got := sshScriptCommand("bash", []string{"a b"}, map[string]string{"MY_VAR": "x'y"}, "")
	if !strings.Contains(got, "MY_VAR='x'\\''y'") {
		t.Errorf("env not escaped: %q", got)
	}
	if !strings.Contains(got, "-- 'a b'") {
		t.Errorf("args not escaped: %q", got)
	}
}

func TestExecuteScriptSSHRejectsInvalidInterpreter(t *testing.T) {
	addr, _ := startTestServer(t)
	h := testHost(addr)

	result, err := DialSSH(h)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Client.Close()

	execResult := ExecuteScriptSSH(result.Client, models.TargetOSLinux, "bash; touch /tmp/pwn", "echo hi", nil, nil, "", ExecOptions{})
	if execResult.Success {
		t.Fatal("expected rejection of non-enum interpreter")
	}
}

func TestSSHScriptCommandSkipsInvalidEnvKeys(t *testing.T) {
	got := sshScriptCommand("bash", nil, map[string]string{
		"GOOD_KEY":       "1",
		"BAD KEY":        "2",
		"X=touch /tmp":   "3",
		"9LEADING_DIGIT": "4",
	}, "")
	if !strings.Contains(got, "GOOD_KEY='1'") {
		t.Errorf("valid key missing: %q", got)
	}
	for _, bad := range []string{"BAD KEY", "touch", "9LEADING_DIGIT"} {
		if strings.Contains(got, bad) {
			t.Errorf("invalid key %q leaked into command: %q", bad, got)
		}
	}
}

func TestRunCommand(t *testing.T) {
	addr, _ := startTestServer(t)
	h := testHost(addr)

	result, err := DialSSH(h)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Client.Close()

	out, err := RunCommand(result.Client, "echo ok")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("expected command output, got: %q", out)
	}
}

func TestExecuteScriptSSHWindowsPipesPreambleAndContent(t *testing.T) {
	addr, _ := startTestServer(t)
	h := testHost(addr)

	result, err := DialSSH(h)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Client.Close()

	execResult := ExecuteScriptSSH(result.Client, models.TargetOSWindows, "powershell",
		"Write-Output $args[0]", []string{"v1"}, map[string]string{"MY_VAR": "x"}, `D:\app`, ExecOptions{})
	if !execResult.Success {
		t.Fatalf("expected success, got: %s", execResult.Error)
	}
	for _, want := range []string{
		`Set-Location -LiteralPath 'D:\app'`,
		"$env:MY_VAR = 'x'",
		"$args = @('v1')",
		"Write-Output $args[0]",
	} {
		if !strings.Contains(execResult.Output, want) {
			t.Errorf("piped stdin missing %q, got: %q", want, execResult.Output)
		}
	}
}

// -Command - leaves the process exit code to PowerShell's session state,
// which reports non-zero after harmless things like a native command's
// stderr crossing a 2>&1 redirect. The piped stdin must end with an explicit
// exit so the code a run is judged by is the last native command's.
func TestExecuteScriptSSHWindowsAppendsExitEpilogue(t *testing.T) {
	addr, _ := startTestServer(t)
	h := testHost(addr)

	result, err := DialSSH(h)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Client.Close()

	// Content without a trailing newline: the epilogue has to carry its own
	// leading one or it would glue onto the last statement.
	execResult := ExecuteScriptSSH(result.Client, models.TargetOSWindows, "powershell",
		"Write-Output done", nil, nil, "", ExecOptions{})
	if !execResult.Success {
		t.Fatalf("expected success, got: %s", execResult.Error)
	}
	if !strings.HasSuffix(execResult.Output, "Write-Output done\nexit $LASTEXITCODE\n") {
		t.Errorf("piped stdin must end with the exit epilogue, got: %q", execResult.Output)
	}
}

func TestExecuteScriptSSHRejectsInterpreterForWrongOS(t *testing.T) {
	cases := []struct{ targetOS, interpreter string }{
		{models.TargetOSWindows, "bash"},
		{models.TargetOSLinux, "powershell"},
	}
	for _, c := range cases {
		if r := ExecuteScriptSSH(nil, c.targetOS, c.interpreter, "echo hi", nil, nil, "", ExecOptions{}); r.Success {
			t.Errorf("%s on a %s target must be rejected before dialing", c.interpreter, c.targetOS)
		}
	}
}

func TestPowershellPreambleEscapesQuotes(t *testing.T) {
	got := powershellPreamble([]string{"a'b"}, map[string]string{"MY_VAR": "x'y"}, "")
	if !strings.Contains(got, "$args = @('a''b')") {
		t.Errorf("arg quote not doubled: %q", got)
	}
	if !strings.Contains(got, "$env:MY_VAR = 'x''y'") {
		t.Errorf("env quote not doubled: %q", got)
	}
}

func TestPowershellPreambleSkipsInvalidEnvKeys(t *testing.T) {
	got := powershellPreamble(nil, map[string]string{
		"GOOD_KEY":            "1",
		"BAD KEY":             "2",
		"X'; rm -rf /; $null": "3",
	}, "")
	if !strings.Contains(got, "$env:GOOD_KEY = '1'") {
		t.Errorf("valid key dropped: %q", got)
	}
	if strings.Contains(got, "BAD KEY") || strings.Contains(got, "rm -rf") {
		t.Errorf("invalid keys must be skipped, got: %q", got)
	}
}

func TestPowershellPreambleAbortsOnUnusableWorkDir(t *testing.T) {
	got := powershellPreamble(nil, nil, `D:\missing`)
	if !strings.Contains(got, "if (-not $?) { exit 1 }") {
		t.Errorf("a failed Set-Location must abort the run, got: %q", got)
	}
	if powershellPreamble(nil, nil, "") != "$args = @()\n" {
		t.Error("empty workDir must not emit a Set-Location")
	}
}

func TestWindowsCommandLine(t *testing.T) {
	got, err := windowsCommandLine("npm run start", []string{"a b"}, map[string]string{"MY_VAR": "x"}, `D:\app`)
	if err != nil {
		t.Fatal(err)
	}
	want := `cd /d "D:\app" && set "MY_VAR=x" && npm run start "a b"`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCmdQuoteRejectsUnquotableValues(t *testing.T) {
	for _, bad := range []string{`a"b`, "a%PATH%b", "a\nb", "a\rb"} {
		if _, err := cmdQuote(bad); err == nil {
			t.Errorf("cmdQuote(%q) must fail: cmd.exe cannot quote it safely", bad)
		}
	}
	got, err := cmdQuote("plain value")
	if err != nil || got != `"plain value"` {
		t.Errorf(`cmdQuote("plain value") = %q, %v`, got, err)
	}
}

func TestExecuteCommandSSHWindowsRejectsUnquotableArg(t *testing.T) {
	r := ExecuteCommandSSH(nil, models.TargetOSWindows, "echo", []string{`x" & calc.exe`}, nil, "", ExecOptions{})
	if r.Success {
		t.Fatal("an arg that breaks cmd.exe quoting must be rejected before dialing")
	}
}

func TestExecuteCommandSSHStreamsBeforeSessionEnds(t *testing.T) {
	addr, _ := startTestServer(t)
	h := testHost(addr)
	result, err := DialSSH(h)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Client.Close()

	sink := newRecordingSink()
	done := make(chan *ExecuteResult, 1)
	go func() {
		done <- ExecuteCommandSSH(result.Client, models.TargetOSLinux, "SLOWTEST", nil, nil, "",
			ExecOptions{Sink: sink})
	}()

	select {
	case <-sink.first:
	case <-time.After(5 * time.Second):
		t.Fatal("no chunk reached the sink before the timeout — remote output is still buffered until the session ends")
	}
	select {
	case <-done:
		t.Fatal("the session finished before the first chunk was observed; the server's pause should have kept it open")
	default:
	}

	execResult := <-done
	if !execResult.Success {
		t.Fatalf("expected success, got: %s", execResult.Error)
	}
	if got := sink.textFor(StreamStdout); !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Errorf("sink missing stdout, got %q", got)
	}
	if got := sink.textFor(StreamStderr); !strings.Contains(got, "err1") {
		t.Errorf("remote stderr was not labelled as stderr, got %q", got)
	}
	if !strings.Contains(execResult.Output, "second") || !strings.Contains(execResult.Error, "err1") {
		t.Errorf("aggregates wrong: out=%q err=%q", execResult.Output, execResult.Error)
	}
}

func TestExecuteCommandSSHSanitisesNonUTF8Output(t *testing.T) {
	addr, _ := startTestServer(t)
	h := testHost(addr)
	result, err := DialSSH(h)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Client.Close()

	sink := newRecordingSink()
	execResult := ExecuteCommandSSH(result.Client, models.TargetOSLinux, "BINARYTEST", nil, nil, "",
		ExecOptions{Sink: sink})

	if !utf8.ValidString(execResult.Output) {
		t.Errorf("aggregate holds invalid UTF-8: %q", execResult.Output)
	}
	if got := sink.textFor(StreamStdout); !utf8.ValidString(got) {
		t.Errorf("persisted chunk holds invalid UTF-8: %q", got)
	}
	if !strings.Contains(execResult.Output, "ok") {
		t.Errorf("valid bytes must survive sanitising, got %q", execResult.Output)
	}
}

func TestExecuteScriptSSHTailLimitAppliesToRemoteOutput(t *testing.T) {
	addr, _ := startTestServer(t)
	h := testHost(addr)
	result, err := DialSSH(h)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Client.Close()

	// The test server echoes stdin, so the script content comes straight back.
	content := strings.Repeat("0123456789", 20)
	execResult := ExecuteScriptSSH(result.Client, models.TargetOSLinux, "bash", content, nil, nil, "",
		ExecOptions{TailBytes: 16})
	if !execResult.Success {
		t.Fatalf("expected success, got: %s", execResult.Error)
	}
	if len(execResult.Output) > 16 {
		t.Errorf("remote aggregate ignored the tail limit: %d bytes", len(execResult.Output))
	}
}
