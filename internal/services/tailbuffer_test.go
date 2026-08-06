package services

import "testing"

func TestTailBufferKeepsEverythingUnderLimit(t *testing.T) {
	b := newTailBuffer(100)
	b.Append("hello ")
	b.Append("world")
	if got := b.String(); got != "hello world" {
		t.Errorf("String() = %q, want %q", got, "hello world")
	}
	if b.Truncated() {
		t.Error("nothing was dropped, Truncated() must be false")
	}
}

func TestTailBufferDropsOldestBytes(t *testing.T) {
	b := newTailBuffer(10)
	b.Append("0123456789")
	b.Append("abcde")
	if got := b.String(); got != "56789abcde" {
		t.Errorf("String() = %q, want the last 10 bytes %q", got, "56789abcde")
	}
	if !b.Truncated() {
		t.Error("bytes were dropped, Truncated() must be true")
	}
}

func TestTailBufferHandlesWriteLargerThanLimit(t *testing.T) {
	b := newTailBuffer(4)
	b.Append("abcdefghij")
	if got := b.String(); got != "ghij" {
		t.Errorf("String() = %q, want %q", got, "ghij")
	}
	if !b.Truncated() {
		t.Error("Truncated() must be true")
	}
}

func TestTailBufferZeroLimitMeansUnbounded(t *testing.T) {
	b := newTailBuffer(0)
	b.Append("0123456789")
	b.Append("abcde")
	if got := b.String(); got != "0123456789abcde" {
		t.Errorf("limit 0 must retain everything, got %q", got)
	}
	if b.Truncated() {
		t.Error("Truncated() must be false when unbounded")
	}
}
