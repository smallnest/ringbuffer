package ringbuffer

import (
	"bytes"
	"testing"
)

// PeekAt exists for a consumer that runs ahead of what it can release: bytes
// stay buffered until something confirms them, so the read pointer sits where
// confirmation reached and the consumer reads from somewhere past it.

func TestPeekAtReadsFromTheOffset(t *testing.T) {
	rb := New(64)
	rb.Write([]byte("0123456789"))

	p := make([]byte, 4)
	for _, tc := range []struct {
		off  int
		want string
	}{
		{0, "0123"},
		{3, "3456"},
		{6, "6789"},
	} {
		n, err := rb.PeekAt(tc.off, p)
		if err != nil {
			t.Fatalf("PeekAt(%d): %v", tc.off, err)
		}
		if got := string(p[:n]); got != tc.want {
			t.Errorf("PeekAt(%d) = %q, want %q", tc.off, got, tc.want)
		}
	}

	if rb.Length() != 10 {
		t.Errorf("Length = %d, want 10 — PeekAt must not consume", rb.Length())
	}
}

func TestPeekAtShortAtTheEnd(t *testing.T) {
	rb := New(64)
	rb.Write([]byte("0123456789"))

	p := make([]byte, 8)
	n, err := rb.PeekAt(7, p)
	if err != nil {
		t.Fatalf("PeekAt: %v", err)
	}
	if got := string(p[:n]); got != "789" {
		t.Errorf("got %q, want %q", got, "789")
	}
}

// An offset at or past the end is the ordinary "nothing new yet" case for a
// consumer that has taken everything, and must not look like an empty buffer.
func TestPeekAtPastTheEndIsNotAnError(t *testing.T) {
	rb := New(64)
	rb.Write([]byte("0123456789"))

	p := make([]byte, 8)
	for _, off := range []int{10, 11, 1000} {
		n, err := rb.PeekAt(off, p)
		if n != 0 || err != nil {
			t.Errorf("PeekAt(%d) = %d, %v; want 0, nil", off, n, err)
		}
	}
	if rb.Length() != 10 {
		t.Errorf("Length = %d, want 10", rb.Length())
	}
}

// The offset has to be applied in ring coordinates, not slice coordinates: the
// window it names can begin before the wrap and end after it.
func TestPeekAtAcrossTheWrap(t *testing.T) {
	rb := New(10)
	rb.Write([]byte("0123456789")) // full
	rb.Read(make([]byte, 6))       // read pointer now at 6
	rb.Write([]byte("abcdef"))     // wraps: buffered is "6789abcdef"

	if rb.Length() != 10 {
		t.Fatalf("Length = %d, want 10", rb.Length())
	}

	p := make([]byte, 10)
	n, err := rb.PeekAt(0, p)
	if err != nil || string(p[:n]) != "6789abcdef" {
		t.Fatalf("PeekAt(0) = %q, %v", p[:n], err)
	}

	// Starting before the wrap and running past it.
	n, _ = rb.PeekAt(2, p)
	if got := string(p[:n]); got != "89abcdef" {
		t.Errorf("PeekAt(2) = %q, want %q", got, "89abcdef")
	}
	// Starting after the wrap.
	n, _ = rb.PeekAt(6, p)
	if got := string(p[:n]); got != "cdef" {
		t.Errorf("PeekAt(6) = %q, want %q", got, "cdef")
	}
}

func TestPeekAtOnAFullBuffer(t *testing.T) {
	rb := New(8)
	rb.Write([]byte("abcdefgh")) // exactly full: w == r and isFull

	p := make([]byte, 8)
	n, _ := rb.PeekAt(0, p)
	if got := string(p[:n]); got != "abcdefgh" {
		t.Errorf("PeekAt(0) on a full buffer = %q", got)
	}
	n, _ = rb.PeekAt(5, p)
	if got := string(p[:n]); got != "fgh" {
		t.Errorf("PeekAt(5) on a full buffer = %q, want %q", got, "fgh")
	}
}

func TestPeekAtEmptyAndDegenerate(t *testing.T) {
	rb := New(8)

	p := make([]byte, 4)
	if n, err := rb.PeekAt(0, p); n != 0 || err != ErrIsEmpty {
		t.Errorf("PeekAt on empty = %d, %v; want 0, ErrIsEmpty", n, err)
	}

	rb.Write([]byte("abcd"))
	if n, _ := rb.PeekAt(0, nil); n != 0 {
		t.Errorf("PeekAt with no destination returned %d", n)
	}
	if n, _ := rb.PeekAt(-1, p); n != 0 {
		t.Errorf("PeekAt with a negative offset returned %d", n)
	}
}

// PeekAt must agree with Peek where they overlap, or a consumer switching from
// one to the other would silently shift the stream.
func TestPeekAtAgreesWithPeek(t *testing.T) {
	rb := New(32)
	rb.Write([]byte("the quick brown fox jumps"))

	a := make([]byte, 32)
	b := make([]byte, 32)
	na, _ := rb.Peek(a)
	nb, _ := rb.PeekAt(0, b)
	if na != nb || !bytes.Equal(a[:na], b[:nb]) {
		t.Errorf("Peek = %q, PeekAt(0) = %q", a[:na], b[:nb])
	}
}
