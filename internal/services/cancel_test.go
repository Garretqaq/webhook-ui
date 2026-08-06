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

func TestCancelIsNotLostWhenTheProcessExitsAtTheSameMoment(t *testing.T) {
	e := newTestExecutor(t)

	// Already closed: done and Cancel are both ready the instant the script
	// finishes, and select picks between ready cases at random. Reporting the
	// run as a plain success would erase the fact that it was stopped.
	cancel := make(chan struct{})
	close(cancel)

	for i := 0; i < 20; i++ {
		result := e.ExecuteScript("bash", "echo quick", nil, nil, "", ExecOptions{Cancel: cancel})
		if !result.Canceled {
			t.Fatalf("attempt %d reported a cancelled run as not cancelled: %+v", i, result)
		}
		if result.Success {
			t.Fatalf("attempt %d reported success for a cancelled run", i)
		}
	}
}

func TestSealedCaptureRefusesLateWrites(t *testing.T) {
	sink := newRecordingSink()
	c := newStreamCapture(ExecOptions{Sink: sink})
	c.write(StreamStdout, "before")
	c.result()

	// An aborted run returns while its readers may still be draining; anything
	// they produce now would land below the cancellation marker.
	c.write(StreamStdout, "after")

	if got := sink.textFor(StreamStdout); strings.Contains(got, "after") {
		t.Errorf("a sealed capture accepted a late write: %q", got)
	}
}
