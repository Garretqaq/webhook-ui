package handlers

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/songguangzhi/webhook-ui/internal/database"
	"github.com/songguangzhi/webhook-ui/internal/services"
	"golang.org/x/crypto/ssh"
)

// startStagedSSHServer runs an in-process SSH server that answers every exec
// request with output in two stages. The pause is what makes it possible to
// assert that chunks reach the database before the session ends.
func startStagedSSHServer(t *testing.T, pause time.Duration) string {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	config := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil },
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
							if req.Type != "exec" {
								req.Reply(false, nil)
								continue
							}
							req.Reply(true, nil)
							go io.Copy(io.Discard, channel) // drain the piped script
							channel.Write([]byte("stage-one\n"))
							time.Sleep(pause)
							channel.Write([]byte("stage-two\n"))
							channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
							channel.Close()
						}
					}()
				}
			}()
		}
	}()

	return ln.Addr().String()
}

// TestRemoteScriptStreamsIntoTheLogTable covers the wiring the unit tests
// cannot: a remote execution's chunks have to travel through the SSH branch of
// runScript into the real database-backed sink, and be readable while the
// session is still open.
func TestRemoteScriptStreamsIntoTheLogTable(t *testing.T) {
	setupExecDB(t)
	addr := startStagedSSHServer(t, 1500*time.Millisecond)
	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	if _, err := database.DB.Exec(`
		INSERT INTO ssh_hosts (id, name, host, port, user, auth_type, credential, target_os)
		VALUES ('h-stage', 'staged', ?, ?, 'tester', 'password', 'x', 'linux')
	`, host, port); err != nil {
		t.Fatal(err)
	}

	execID := startedExecution(t)
	executor := services.NewExecutor(nil, t.TempDir(), 0)
	out := services.OutputStream{Sink: sinkFor(execID, 0)}

	done := make(chan *services.ExecuteResult, 1)
	go func() {
		done <- runScript(executor, "bash", "echo hi", "h-stage", nil, nil, "", out)
	}()

	// Poll the table the way the UI polls the endpoint.
	deadline := time.After(5 * time.Second)
	for {
		if logRowCount(t, execID) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no chunk reached execution_logs before the timeout")
		case r := <-done:
			t.Fatalf("the session ended before any chunk was stored: %+v", r)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	result := <-done
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}

	var stored string
	rows, err := database.DB.Query(
		"SELECT chunk FROM execution_logs WHERE execution_id = ? ORDER BY seq", execID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var chunk string
		if err := rows.Scan(&chunk); err != nil {
			t.Fatal(err)
		}
		stored += chunk
	}
	if !strings.Contains(stored, "stage-one") || !strings.Contains(stored, "stage-two") {
		t.Errorf("both stages should be stored, got %q", stored)
	}
}
