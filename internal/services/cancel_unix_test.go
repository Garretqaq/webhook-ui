//go:build !windows

package services

import (
	"fmt"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestCancelReapsGrandchildren is the point of the process group. A script's
// own children survive a plain kill of the shell, and keep the output pipes
// open — the trap ticket 01 hit from the timeout side.
func TestCancelReapsGrandchildren(t *testing.T) {
	e := newTestExecutor(t)
	cancel := make(chan struct{})
	sink := newRecordingSink()

	// The script reports its child's pid, so the check below targets that exact
	// process. Matching on a command line instead would pick up strays left by
	// an earlier run and fail for reasons that have nothing to do with the code.
	go e.ExecuteScript("bash", "sleep 300 & echo PID=$!; wait", nil, nil, "",
		ExecOptions{Cancel: cancel, Sink: sink, Timeout: 20 * time.Second})

	pid := 0
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && pid == 0 {
		if _, err := fmt.Sscanf(strings.TrimSpace(sink.textFor(StreamStdout)), "PID=%d", &pid); err != nil {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if pid == 0 {
		t.Fatal("the script never reported its child's pid; the test would otherwise prove nothing")
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("the grandchild %d was not alive to begin with: %v", pid, err)
	}

	close(cancel)

	// Signal 0 only probes for existence.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return // gone, as it must be
		}
		time.Sleep(50 * time.Millisecond)
	}
	syscall.Kill(pid, syscall.SIGKILL)
	t.Errorf("grandchild %d outlived the cancel; the process group was not signalled", pid)
}
