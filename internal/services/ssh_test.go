package services

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/songguangzhi/webhook-ui/internal/models"
	"golang.org/x/crypto/ssh"
)

// startTestServer runs an in-process SSH server accepting password
// "secret" for any user, and returns its listener address and host key.
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
								channel.Write([]byte("ok\n"))
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
