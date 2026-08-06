package services

import "unicode/utf8"

// tailBuffer accumulates output while retaining only the last limit bytes.
// A long-running hook can emit far more than anyone will read, and the whole
// aggregate ends up in a single database column, so the head is dropped rather
// than letting it grow without bound. limit <= 0 disables the cap.
type tailBuffer struct {
	limit int
	buf   []byte
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (t *tailBuffer) Append(s string) {
	t.buf = append(t.buf, s...)
	if t.limit <= 0 || len(t.buf) <= t.limit {
		return
	}
	// Cutting on a byte offset can land inside a rune and leave a broken one at
	// the head, so advance to the next boundary — dropping at most three more
	// bytes is cheaper than emitting invalid UTF-8.
	cut := len(t.buf) - t.limit
	for cut < len(t.buf) && !utf8.RuneStart(t.buf[cut]) {
		cut++
	}
	// Copy the tail down rather than resliceing: a resliced buffer keeps the
	// dropped head alive in the backing array for the life of the execution.
	t.buf = append(t.buf[:0], t.buf[cut:]...)
}

func (t *tailBuffer) String() string { return string(t.buf) }
