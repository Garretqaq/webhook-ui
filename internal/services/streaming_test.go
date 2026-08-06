package services

import (
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// recordingSink captures chunks and signals the first one, so a test can tell
// that output was observable before the process exited.
type recordingSink struct {
	mu     sync.Mutex
	chunks []loggedChunk
	first  chan struct{}
	once   sync.Once
}

type loggedChunk struct {
	stream string
	text   string
}

func newRecordingSink() *recordingSink {
	return &recordingSink{first: make(chan struct{})}
}

func (s *recordingSink) WriteChunk(stream, chunk string) {
	s.mu.Lock()
	s.chunks = append(s.chunks, loggedChunk{stream, chunk})
	s.mu.Unlock()
	s.once.Do(func() { close(s.first) })
}

func (s *recordingSink) textFor(stream string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for _, c := range s.chunks {
		if c.stream == stream {
			b.WriteString(c.text)
		}
	}
	return b.String()
}

func TestExecuteScriptStreamsBeforeProcessExits(t *testing.T) {
	e := newTestExecutor(t)
	sink := newRecordingSink()

	done := make(chan *ExecuteResult, 1)
	go func() {
		done <- e.ExecuteScript("bash", "echo early; sleep 1; echo late", nil, nil, "", ExecOptions{Sink: sink})
	}()

	select {
	case <-sink.first:
	case <-time.After(5 * time.Second):
		t.Fatal("no chunk reached the sink before the timeout — output is still buffered until exit")
	}

	select {
	case <-done:
		t.Fatal("script finished before the first chunk was observed; the sleep should have kept it running")
	default:
	}

	result := <-done
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if got := sink.textFor(StreamStdout); !strings.Contains(got, "early") || !strings.Contains(got, "late") {
		t.Errorf("sink missing output, got %q", got)
	}
	if !strings.Contains(result.Output, "early") || !strings.Contains(result.Output, "late") {
		t.Errorf("aggregate Output incomplete: %q", result.Output)
	}
}

func TestExecuteScriptLabelsStderrChunks(t *testing.T) {
	e := newTestExecutor(t)
	sink := newRecordingSink()

	result := e.ExecuteScript("bash", "echo out; echo err >&2", nil, nil, "", ExecOptions{Sink: sink})
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if got := sink.textFor(StreamStdout); !strings.Contains(got, "out") {
		t.Errorf("stdout chunk missing: %q", got)
	}
	if got := sink.textFor(StreamStderr); !strings.Contains(got, "err") {
		t.Errorf("stderr chunk missing: %q", got)
	}
	if strings.Contains(sink.textFor(StreamStdout), "err") {
		t.Error("stderr text leaked into the stdout stream")
	}
	if !strings.Contains(result.Output, "out") || !strings.Contains(result.Error, "err") {
		t.Errorf("aggregates wrong: out=%q err=%q", result.Output, result.Error)
	}
}

func TestExecuteScriptWithoutSinkStillAggregates(t *testing.T) {
	e := newTestExecutor(t)
	result := e.ExecuteScript("bash", "echo hi", nil, nil, "", ExecOptions{})
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hi") {
		t.Errorf("a nil sink must not break aggregation, got %q", result.Output)
	}
}

func TestExecuteScriptCapsAggregateAtTailLimit(t *testing.T) {
	e := newTestExecutor(t)

	result := e.ExecuteScript("bash", "for i in $(seq 1 200); do echo LINE$i; done", nil, nil, "",
		ExecOptions{TailBytes: 16})
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if len(result.Output) > 16 {
		t.Errorf("aggregate exceeded the tail limit: %d bytes", len(result.Output))
	}
	if !strings.Contains(result.Output, "LINE200") {
		t.Errorf("the tail must be what survives, got %q", result.Output)
	}
}

func TestRunKeepsStderrWhenKilledOnTimeout(t *testing.T) {
	e := newTestExecutor(t)

	result := e.ExecuteScript("bash", "echo out; echo diagnostic >&2; sleep 30", nil, nil, "",
		ExecOptions{Timeout: 300 * time.Millisecond})
	if result.Success {
		t.Fatal("a timed-out process must not report success")
	}
	if !strings.Contains(result.Error, "execution timeout") {
		t.Errorf("the timeout notice is missing: %q", result.Error)
	}
	if !strings.Contains(result.Output, "out") {
		t.Errorf("stdout captured before the kill was lost: %q", result.Output)
	}
	if !strings.Contains(result.Error, "diagnostic") {
		t.Errorf("stderr captured before the kill was lost: %q", result.Error)
	}
}

func TestRunTimeoutIsNotHeldUpByASurvivingGrandchild(t *testing.T) {
	e := newTestExecutor(t)

	// `sleep` inherits the pipes and outlives the shell it was spawned from, so
	// waiting for EOF here would stretch the 300ms timeout out to 10 seconds.
	start := time.Now()
	result := e.ExecuteScript("bash", "echo out; sleep 10", nil, nil, "",
		ExecOptions{Timeout: 300 * time.Millisecond})
	elapsed := time.Since(start)

	if result.Success {
		t.Fatal("a timed-out execution must not report success")
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout took %s; a surviving grandchild must not hold the call open", elapsed)
	}
}

func TestStreamCaptureReassemblesRunesSplitAcrossReads(t *testing.T) {
	sink := newRecordingSink()
	c := newStreamCapture(ExecOptions{Sink: sink})

	// A pipe read boundary can fall anywhere, including inside a rune.
	const text = "中文输出"
	for i := 0; i < len(text); i++ {
		c.write(StreamStdout, text[i:i+1])
	}

	out, _ := c.result()
	if out != text {
		t.Errorf("aggregate = %q, want %q — a split rune was corrupted", out, text)
	}
	if got := sink.textFor(StreamStdout); got != text {
		t.Errorf("sink text = %q, want %q", got, text)
	}
}

func TestStreamCaptureReplacesGenuinelyForeignBytes(t *testing.T) {
	c := newStreamCapture(ExecOptions{})
	c.write(StreamStdout, string([]byte{0xff, 0xfe})+"ok")

	out, _ := c.result()
	if !utf8.ValidString(out) {
		t.Errorf("invalid bytes survived: %q", out)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("valid text must survive alongside them, got %q", out)
	}
}

func TestStreamCaptureFlushesAStreamEndingMidRune(t *testing.T) {
	c := newStreamCapture(ExecOptions{})
	// Only the first two bytes of a three byte rune ever arrive.
	c.write(StreamStdout, "中"[:2])

	out, _ := c.result()
	if !utf8.ValidString(out) {
		t.Errorf("a stream cut mid-rune must not leave invalid UTF-8: %q", out)
	}
	if out == "" {
		t.Error("the dangling bytes should surface as replacement characters, not vanish")
	}
}
