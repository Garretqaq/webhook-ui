package services

import (
	"strings"
	"testing"
	"time"
)

func TestCancelStopsTheRunPromptly(t *testing.T) {
	e := newTestExecutor(t)
	cancel := make(chan struct{})

	done := make(chan *ExecuteResult, 1)
	go func() {
		done <- e.ExecuteScript("bash", "echo working; sleep 30", nil, nil, "",
			ExecOptions{Cancel: cancel})
	}()

	time.Sleep(300 * time.Millisecond)
	close(cancel)

	select {
	case result := <-done:
		if result.Success {
			t.Fatal("a canceled execution must not report success")
		}
		if !result.Canceled {
			t.Error("Canceled must be set, or the log cannot tell a stop from a crash")
		}
		if !strings.Contains(result.Output, "working") {
			t.Errorf("output produced before the cancel was lost: %q", result.Output)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not return promptly")
	}
}

func TestNilCancelChannelNeverFires(t *testing.T) {
	e := newTestExecutor(t)
	result := e.ExecuteScript("bash", "echo fine", nil, nil, "", ExecOptions{})
	if !result.Success {
		t.Fatalf("an execution with no cancel channel must run normally, got: %s", result.Error)
	}
	if result.Canceled {
		t.Error("Canceled must stay false when nothing cancelled it")
	}
}
