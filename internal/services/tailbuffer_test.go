package services

import (
	"testing"
	"unicode/utf8"
)

func TestTailBufferKeepsEverythingUnderLimit(t *testing.T) {
	b := newTailBuffer(100)
	b.Append("hello ")
	b.Append("world")
	if got := b.String(); got != "hello world" {
		t.Errorf("String() = %q, want %q", got, "hello world")
	}
}

func TestTailBufferDropsOldestBytes(t *testing.T) {
	b := newTailBuffer(10)
	b.Append("0123456789")
	b.Append("abcde")
	if got := b.String(); got != "56789abcde" {
		t.Errorf("String() = %q, want the last 10 bytes %q", got, "56789abcde")
	}
}

func TestTailBufferHandlesWriteLargerThanLimit(t *testing.T) {
	b := newTailBuffer(4)
	b.Append("abcdefghij")
	if got := b.String(); got != "ghij" {
		t.Errorf("String() = %q, want %q", got, "ghij")
	}
}

func TestTailBufferZeroLimitMeansUnbounded(t *testing.T) {
	b := newTailBuffer(0)
	b.Append("0123456789")
	b.Append("abcde")
	if got := b.String(); got != "0123456789abcde" {
		t.Errorf("limit 0 must retain everything, got %q", got)
	}
}

func TestTailBufferCutsOnARuneBoundary(t *testing.T) {
	// Nine bytes of CJK; a 4 byte cap lands mid-rune if the cut is naive.
	b := newTailBuffer(4)
	b.Append("中文字")
	if !utf8.ValidString(b.String()) {
		t.Errorf("truncation left invalid UTF-8: %q", b.String())
	}
	if got := b.String(); got != "字" {
		t.Errorf("String() = %q, want the last whole rune %q", got, "字")
	}
}
