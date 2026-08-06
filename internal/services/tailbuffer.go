package services

// tailBuffer accumulates output while retaining only the last limit bytes.
// A long-running hook can emit far more than anyone will read, and the whole
// aggregate ends up in a single database column, so the head is dropped rather
// than letting it grow without bound. limit <= 0 disables the cap.
type tailBuffer struct {
	limit     int
	buf       []byte
	truncated bool
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (t *tailBuffer) Append(s string) {
	t.buf = append(t.buf, s...)
	if t.limit <= 0 || len(t.buf) <= t.limit {
		return
	}
	// Copy the tail down rather than resliceing: a resliced buffer keeps the
	// dropped head alive in the backing array for the life of the execution.
	t.buf = append(t.buf[:0], t.buf[len(t.buf)-t.limit:]...)
	t.truncated = true
}

func (t *tailBuffer) String() string { return string(t.buf) }

// Truncated reports whether any bytes were dropped from the head.
func (t *tailBuffer) Truncated() bool { return t.truncated }
