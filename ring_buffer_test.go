package ringbuffer

import (
	"bytes"
	"strings"
	"testing"
)

func TestRingBuffer_Write(t *testing.T) {
	rb := New(64)

	// check empty or full
	if !rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is true but got false")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}
	if rb.Length() != 0 {
		t.Fatalf("expect len 0 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 64 {
		t.Fatalf("expect free 64 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}

	// write 4 * 4 = 16 bytes
	n, err := rb.Write([]byte(strings.Repeat("abcd", 4)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 16 {
		t.Fatalf("expect write 16 bytes but got %d", n)
	}
	if rb.Length() != 16 {
		t.Fatalf("expect len 16 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 48 {
		t.Fatalf("expect free 48 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if bytes.Compare(rb.Bytes(), []byte(strings.Repeat("abcd", 4))) != 0 {
		t.Fatalf("expect 4 abcd but got %s. r.w=%d, r.r=%d", rb.Bytes(), rb.w, rb.r)
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}

	// write 48 bytes, should full
	n, err = rb.Write([]byte(strings.Repeat("abcd", 12)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 48 {
		t.Fatalf("expect write 48 bytes but got %d", n)
	}
	if rb.Length() != 64 {
		t.Fatalf("expect len 64 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 0 {
		t.Fatalf("expect free 0 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if rb.w != 0 {
		t.Fatalf("expect r.w=0 but got %d. r.r=%d", rb.w, rb.r)
	}
	if bytes.Compare(rb.Bytes(), []byte(strings.Repeat("abcd", 16))) != 0 {
		t.Fatalf("expect 16 abcd but got %s. r.w=%d, r.r=%d", rb.Bytes(), rb.w, rb.r)
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}

	// write more 4 bytes, should reject
	n, err = rb.Write([]byte(strings.Repeat("abcd", 1)))
	if err == nil {
		t.Fatalf("expect an error but got nil. n=%d, r.w=%d, r.r=%d", n, rb.w, rb.r)
	}
	if err != ErrIsFull {
		t.Fatalf("expect ErrIsFull but got nil")
	}
	if n != 0 {
		t.Fatalf("expect write 0 bytes but got %d", n)
	}
	if rb.Length() != 64 {
		t.Fatalf("expect len 64 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 0 {
		t.Fatalf("expect free 0 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}

	// reset this ringbuffer and set a long slice
	rb.Reset()
	n, err = rb.Write([]byte(strings.Repeat("abcd", 20)))
	if err == nil {
		t.Fatalf("expect ErrTooManyDataToWrite but got nil")
	}
	if n != 64 {
		t.Fatalf("expect write 64 bytes but got %d", n)
	}
	if rb.Length() != 64 {
		t.Fatalf("expect len 64 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 0 {
		t.Fatalf("expect free 0 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if rb.w != 0 {
		t.Fatalf("expect r.w=0 but got %d. r.r=%d", rb.w, rb.r)
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}

	if bytes.Compare(rb.Bytes(), []byte(strings.Repeat("abcd", 16))) != 0 {
		t.Fatalf("expect 16 abcd but got %s. r.w=%d, r.r=%d", rb.Bytes(), rb.w, rb.r)
	}
}

func TestRingBuffer_Read(t *testing.T) {
	rb := New(64)

	// check empty or full
	if !rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is true but got false")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}
	if rb.Length() != 0 {
		t.Fatalf("expect len 0 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 64 {
		t.Fatalf("expect free 64 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}

	// read empty
	buf := make([]byte, 1024)
	n, err := rb.Read(buf)
	if err == nil {
		t.Fatalf("expect an error but got nil")
	}
	if err != ErrIsEmpty {
		t.Fatalf("expect ErrIsEmpty but got nil")
	}
	if n != 0 {
		t.Fatalf("expect read 0 bytes but got %d", n)
	}
	if rb.Length() != 0 {
		t.Fatalf("expect len 0 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 64 {
		t.Fatalf("expect free 64 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if rb.r != 0 {
		t.Fatalf("expect r.r=0 but got %d. r.w=%d", rb.r, rb.w)
	}

	// write 16 bytes to read
	rb.Write([]byte(strings.Repeat("abcd", 4)))
	n, err = rb.Read(buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if n != 16 {
		t.Fatalf("expect read 16 bytes but got %d", n)
	}
	if rb.Length() != 0 {
		t.Fatalf("expect len 0 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 64 {
		t.Fatalf("expect free 64 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if rb.r != 16 {
		t.Fatalf("expect r.r=16 but got %d. r.w=%d", rb.r, rb.w)
	}

	// write long slice to  read
	rb.Write([]byte(strings.Repeat("abcd", 20)))
	n, err = rb.Read(buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if n != 64 {
		t.Fatalf("expect read 64 bytes but got %d", n)
	}
	if rb.Length() != 0 {
		t.Fatalf("expect len 0 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 64 {
		t.Fatalf("expect free 64 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if rb.r != 16 {
		t.Fatalf("expect r.r=16 but got %d. r.w=%d", rb.r, rb.w)
	}

}
