package services

import (
	"strings"
	"sync"
)

// Stream names for a chunk's origin.
const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
)

// LogSink receives output chunks as the process produces them, so callers can
// persist a running execution's log before it finishes. Implementations must
// be safe for concurrent use; streamCapture serializes calls, but nothing
// stops a sink from being shared.
type LogSink interface {
	WriteChunk(stream, chunk string)
}

// OutputStream bundles where an execution's output goes with how much of it is
// retained, so the two stop travelling as a pair of loose parameters through
// every execution entry point.
type OutputStream struct {
	Sink LogSink
	// TailBytes caps each aggregated stream; 0 means unbounded.
	TailBytes int
}

// streamCapture fans a process's two output streams into the per-stream
// aggregates that end up on ExecuteResult and, when set, into a sink.
//
// Every write holds the mutex across the sink call. That is what makes the
// sink's arrival order match the order the reader goroutines observed —
// without it two chunks could be appended to the aggregate in one order and
// reach the sink in another, and a sink that assigns sequence numbers would
// hand out an order the aggregate disagrees with.
type streamCapture struct {
	mu     sync.Mutex
	sink   LogSink
	stdout *tailBuffer
	stderr *tailBuffer
}

func newStreamCapture(out OutputStream) *streamCapture {
	return &streamCapture{
		sink:   out.Sink,
		stdout: newTailBuffer(out.TailBytes),
		stderr: newTailBuffer(out.TailBytes),
	}
}

func (c *streamCapture) write(stream, chunk string) {
	if chunk == "" {
		return
	}
	// A remote host can emit output in any encoding — GBK is routine on Chinese
	// Windows. Invalid UTF-8 would survive into the database and only turn into
	// replacement characters at the JSON boundary, so it is replaced here, where
	// the aggregate and the persisted chunks still agree on the bytes.
	chunk = strings.ToValidUTF8(chunk, "\uFFFD")

	c.mu.Lock()
	defer c.mu.Unlock()

	if stream == StreamStderr {
		c.stderr.Append(chunk)
	} else {
		c.stdout.Append(chunk)
	}
	if c.sink != nil {
		c.sink.WriteChunk(stream, chunk)
	}
}

func (c *streamCapture) result() (stdout, stderr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stdout.String(), c.stderr.String()
}
