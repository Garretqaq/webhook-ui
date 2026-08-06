package services

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// timeoutMessage is derived from the duration itself so no copy can drift.
func timeoutMessage(d time.Duration) string {
	return fmt.Sprintf("execution timeout (%s)", d)
}

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

// ExecOptions carries the per-call settings an execution needs, so they stop
// travelling as a growing list of loose parameters through every entry point.
type ExecOptions struct {
	Sink LogSink
	// TailBytes caps each aggregated stream; 0 means unbounded.
	TailBytes int
	// Timeout bounds the execution; 0 means it may run indefinitely, which is
	// the point of an asynchronous hook — nothing is holding a request open.
	Timeout time.Duration
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
	// pending holds the trailing bytes of a rune that a read cut in half, per
	// stream, until the rest of it arrives.
	pending map[string][]byte
}

func newStreamCapture(opts ExecOptions) *streamCapture {
	return &streamCapture{
		sink:    opts.Sink,
		stdout:  newTailBuffer(opts.TailBytes),
		stderr:  newTailBuffer(opts.TailBytes),
		pending: map[string][]byte{},
	}
}

func (c *streamCapture) write(stream, chunk string) {
	if chunk == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// A read boundary lands wherever the pipe happened to fill, so a multi-byte
	// rune is routinely cut in half. Sanitising each read on its own would turn
	// both halves into replacement characters — worst for exactly the CJK output
	// this is meant to protect — so the split tail waits for its other half.
	buf := append(c.pending[stream], chunk...)
	keep := incompleteTailLen(buf)
	c.pending[stream] = append([]byte(nil), buf[len(buf)-keep:]...)

	c.emit(stream, string(buf[:len(buf)-keep]))
}

// emit sanitises and records text. The caller holds the lock.
func (c *streamCapture) emit(stream, text string) {
	if text == "" {
		return
	}
	// Whatever is left that is still not valid UTF-8 is genuinely foreign
	// encoding, not a split rune. It is replaced here rather than at the JSON
	// boundary so the aggregate and the persisted chunks agree on the bytes.
	text = strings.ToValidUTF8(text, "\uFFFD")

	if stream == StreamStderr {
		c.stderr.Append(text)
	} else {
		c.stdout.Append(text)
	}
	if c.sink != nil {
		c.sink.WriteChunk(stream, text)
	}
}

func (c *streamCapture) result() (stdout, stderr string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// A stream that ended mid-rune is truncated output, not a pending one; give
	// up waiting and let the leftovers become replacement characters.
	for stream, leftover := range c.pending {
		if len(leftover) > 0 {
			c.pending[stream] = nil
			c.emit(stream, string(leftover))
		}
	}
	return c.stdout.String(), c.stderr.String()
}

// incompleteTailLen reports how many bytes at the end of b begin a rune whose
// remaining bytes have not arrived yet.
func incompleteTailLen(b []byte) int {
	for back := 1; back <= utf8.UTFMax && back <= len(b); back++ {
		lead := b[len(b)-back]
		if !utf8.RuneStart(lead) {
			continue // continuation byte; keep walking back to the lead
		}
		if runeLenFor(lead) > back {
			return back
		}
		return 0
	}
	return 0
}

// runeLenFor returns how many bytes the rune starting with lead occupies. A
// byte that leads nothing valid counts as one, so foreign-encoding garbage is
// emitted immediately instead of being held back waiting for a rune that will
// never complete.
func runeLenFor(lead byte) int {
	switch {
	case lead < 0x80:
		return 1
	case lead&0xE0 == 0xC0:
		return 2
	case lead&0xF0 == 0xE0:
		return 3
	case lead&0xF8 == 0xF0:
		return 4
	}
	return 1
}

// timeoutChan returns a channel that fires after d, or one that never fires
// when d is not positive. A nil channel blocks forever in a select, which is
// exactly what "no timeout" has to mean.
func timeoutChan(d time.Duration) <-chan time.Time {
	if d <= 0 {
		return nil
	}
	return time.After(d)
}
