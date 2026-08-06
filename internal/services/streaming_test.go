package services

import (
	"strings"
	"sync"
	"testing"
	"time"
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
		done <- e.ExecuteScript("bash", "echo early; sleep 1; echo late", nil, nil, "", sink)
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

	result := e.ExecuteScript("bash", "echo out; echo err >&2", nil, nil, "", sink)
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
	result := e.ExecuteScript("bash", "echo hi", nil, nil, "", nil)
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hi") {
		t.Errorf("a nil sink must not break aggregation, got %q", result.Output)
	}
}

func TestExecuteScriptCapsAggregateAtTailLimit(t *testing.T) {
	e := newTestExecutor(t)
	e.logTailBytes = 16
	result := e.ExecuteScript("bash", "for i in $(seq 1 200); do echo LINE$i; done", nil, nil, "", nil)
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
