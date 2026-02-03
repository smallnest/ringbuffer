package ringbuffer

import (
	"errors"
	"testing"
)

// TestIssue23 tests the fix for https://github.com/smallnest/ringbuffer/issues/23
// Overwrite should work for unaligned data even when buffer is empty
func TestIssue23(t *testing.T) {
	overwrite := func(good, fill bool) (b *RingBuffer, full bool, err error) {
		b = New(32)
		b.SetOverwrite(true)
		var v []byte
		if good {
			v = []byte("0123456789abcdef") // 16 bytes
		} else {
			v = []byte("0123456789") // 10 bytes
		}
		if fill {
			for i := 0; i < b.Capacity(); i++ {
				if writeErr := b.WriteByte(' '); writeErr != nil {
					err = writeErr
					return
				}
			}
			full = b.IsFull()
			if !full {
				err = errors.New("Buffer is not full")
				return
			}
		}

		var n int
		for i := 0; i < 1000; i++ {
			n, err = b.Write(v)
			full = b.IsFull()
			if err != nil {
				return
			}
			if n != len(v) {
				err = errors.New("Did not write all bytes")
				return
			}
		}
		full = b.IsFull()
		return
	}

	// Test 16 bytes (good case - aligned)
	b, full, err := overwrite(true, false)
	if err != nil {
		t.Errorf("Good bytes not fill failed: err=%v", err)
	}
	if !full {
		t.Errorf("Good bytes not fill: buffer should be full")
	}
	if b.Length() != 32 {
		t.Errorf("Good bytes not fill: expected length 32, got %d", b.Length())
	}

	// Test 16 bytes with filled buffer (also good)
	_, full, err = overwrite(true, true)
	if err != nil {
		t.Errorf("Good bytes fill failed: err=%v", err)
	}
	if !full {
		t.Errorf("Good bytes fill: buffer should be full")
	}

	// Test 10 bytes with empty buffer (the bug case - should now work)
	b, full, err = overwrite(false, false)
	if err != nil {
		t.Errorf("Bad bytes not fill failed: err=%v (this is the bug)", err)
	}
	if !full {
		t.Errorf("Bad bytes not fill: buffer should be full")
	}
	if b.Length() != 32 {
		t.Errorf("Bad bytes not fill: expected length 32, got %d", b.Length())
	}

	// Test 10 bytes with filled buffer (workaround case - should still work)
	_, full, err = overwrite(false, true)
	if err != nil {
		t.Errorf("Bad bytes fill failed: err=%v", err)
	}
	if !full {
		t.Errorf("Bad bytes fill: buffer should be full")
	}
}

// TestOverwriteVariousSizes tests overwrite mode with various buffer sizes and data sizes
func TestOverwriteVariousSizes(t *testing.T) {
	testCases := []struct {
		capacity  int
		writeSize int
		numWrites int
	}{
		{32, 10, 100},  // original bug case
		{32, 7, 100},   // prime number size
		{64, 13, 200},  // larger buffer
		{100, 17, 500}, // non-power-of-2 buffer
		{32, 1, 1000},  // single byte
		{32, 31, 100},  // almost full size
		{1024, 100, 1000},
	}

	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			b := New(tc.capacity)
			b.SetOverwrite(true)

			data := make([]byte, tc.writeSize)
			for i := range data {
				data[i] = byte(i % 256)
			}

			for i := 0; i < tc.numWrites; i++ {
				n, err := b.Write(data)
				if err != nil {
					t.Errorf("Write failed at iteration %d: capacity=%d, writeSize=%d, err=%v",
						i, tc.capacity, tc.writeSize, err)
					return
				}
				if n != tc.writeSize {
					t.Errorf("Did not write all bytes at iteration %d: expected %d, got %d",
						i, tc.writeSize, n)
					return
				}
			}

			// Buffer should be full
			if !b.IsFull() {
				t.Errorf("Buffer should be full after %d writes of %d bytes into capacity %d",
					tc.numWrites, tc.writeSize, tc.capacity)
			}

			// Length should be capacity
			if b.Length() != tc.capacity {
				t.Errorf("Expected length %d, got %d", tc.capacity, b.Length())
			}
		})
	}
}
