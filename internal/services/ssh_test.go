package services

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"

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
	execResult := ExecuteScriptSSH(result.Client, "bash", content, nil, nil)
	if !execResult.Success {
		t.Fatalf("expected success, got: %s", execResult.Error)
	}
	if !strings.Contains(execResult.Output, content) {
		t.Errorf("expected script content piped via stdin, got: %q", execResult.Output)
	}
}

func TestExecuteScriptSSHBuildsCommand(t *testing.T) {
	// bash/sh get "-s", python3 gets "-"
	if got := sshScriptCommand("bash", nil, nil); !strings.HasPrefix(got, "bash -s --") {
		t.Errorf("bash command prefix wrong: %q", got)
	}
	if got := sshScriptCommand("python3", nil, nil); !strings.HasPrefix(got, "python3 -") {
		t.Errorf("python3 command prefix wrong: %q", got)
	}
}

func TestShellEscape(t *testing.T) {
	cases := map[string]string{
		"simple":    "'simple'",
		"with space": "'with space'",
		"it's":      "'it'\\''s'",
		"":          "''",
	}
	for in, want := range cases {
		if got := shellEscape(in); got != want {
			t.Errorf("shellEscape(%q) = %q, want %q", in, got, want)
		}
	}

	// env and args are escaped into the command
	got := sshScriptCommand("bash", []string{"a b"}, map[string]string{"MY_VAR": "x'y"})
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

	execResult := ExecuteScriptSSH(result.Client, "bash; touch /tmp/pwn", "echo hi", nil, nil)
	if execResult.Success {
		t.Fatal("expected rejection of non-enum interpreter")
	}
}

func TestSSHScriptCommandSkipsInvalidEnvKeys(t *testing.T) {
	got := sshScriptCommand("bash", nil, map[string]string{
		"GOOD_KEY":      "1",
		"BAD KEY":       "2",
		"X=touch /tmp":  "3",
		"9LEADING_DIGIT": "4",
	})
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
