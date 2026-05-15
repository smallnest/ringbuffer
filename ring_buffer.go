// Copyright 2019 smallnest. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package ringbuffer

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
	"unsafe"
)

// RingBuffer is a circular buffer for byte slices.
// It wraps *Ring[byte] and provides all standard library io interfaces
// as well as convenience methods for byte-stream operations.
type RingBuffer struct {
	*Ring[byte]
}

// New returns a new RingBuffer whose buffer has the given size.
func New(size int) *RingBuffer {
	return &RingBuffer{Ring: NewRing[byte](size)}
}

// NewBuffer returns a new RingBuffer whose buffer is provided.
func NewBuffer(b []byte) *RingBuffer {
	return &RingBuffer{Ring: NewRingFromSlice(b)}
}

// SetBlocking sets the blocking mode of the ring buffer.
func (r *RingBuffer) SetBlocking(block bool) *RingBuffer {
	r.Ring.SetBlocking(block)
	return r
}

// SetOverwrite sets the overwrite mode of the ring buffer.
func (r *RingBuffer) SetOverwrite(overwrite bool) *RingBuffer {
	r.Ring.SetOverwrite(overwrite)
	return r
}

// WithCancel sets a context to cancel the ring buffer.
func (r *RingBuffer) WithCancel(ctx context.Context) *RingBuffer {
	r.Ring.WithCancel(ctx)
	return r
}

// WithTimeout will set a blocking read/write timeout.
func (r *RingBuffer) WithTimeout(d time.Duration) *RingBuffer {
	r.Ring.WithTimeout(d)
	return r
}

// WithReadTimeout will set a blocking read timeout.
func (r *RingBuffer) WithReadTimeout(d time.Duration) *RingBuffer {
	r.Ring.WithReadTimeout(d)
	return r
}

// WithWriteTimeout will set a blocking write timeout.
func (r *RingBuffer) WithWriteTimeout(d time.Duration) *RingBuffer {
	r.Ring.WithWriteTimeout(d)
	return r
}

// ReadByte reads and returns the next byte from the input or ErrIsEmpty.
func (r *RingBuffer) ReadByte() (byte, error) {
	return r.Ring.ReadOne()
}

// TryReadByte reads and returns the next byte without blocking.
// If it does not succeed to acquire the lock, it returns ErrAcquireLock.
func (r *RingBuffer) TryReadByte() (byte, error) {
	return r.Ring.TryReadOne()
}

// WriteByte writes one byte into buffer, and returns ErrIsFull if the buffer is full.
func (r *RingBuffer) WriteByte(c byte) error {
	return r.Ring.WriteOne(c)
}

// TryWriteByte writes one byte into buffer without blocking.
// If it does not succeed to acquire the lock, it returns ErrAcquireLock.
func (r *RingBuffer) TryWriteByte(c byte) error {
	return r.Ring.TryWriteOne(c)
}

// WriteString writes the contents of the string s to buffer.
func (r *RingBuffer) WriteString(s string) (n int, err error) {
	if len(s) == 0 {
		return 0, r.setErr(nil, false)
	}
	buf := unsafe.Slice(unsafe.StringData(s), len(s))
	return r.Write(buf)
}

// ReadFrom will fulfill the write side of the ringbuffer.
// This will do writes directly into the buffer,
// therefore avoiding a mem-copy when using the Write.
//
// ReadFrom will not automatically close the buffer even after returning.
// For that call CloseWriter().
//
// ReadFrom reads data from rd until EOF or error.
// The return value n is the number of bytes read.
// Any error except EOF encountered during the read is also returned,
// and the error will cause the Read side to fail as well.
// ReadFrom only available in blocking mode.
func (r *RingBuffer) ReadFrom(rd io.Reader) (n int64, err error) {
	if !r.block {
		return 0, errors.New("RingBuffer: ReadFrom only available in blocking mode")
	}
	zeroReads := 0
	r.mu.Lock()
	defer r.mu.Unlock()
	for {
		if err = r.readErr(true); err != nil {
			return n, err
		}
		if r.isFull {
			// Wait for a read
			if !r.waitRead() {
				return 0, context.DeadlineExceeded
			}
			continue
		}

		// Calculate available space to read into
		var toRead []byte
		if r.w >= r.r {
			// After reader, read until end of buffer
			toRead = r.buf[r.w:]
		} else {
			// Before reader, read until reader.
			if r.w >= r.size || r.r > r.size {
				// Pointers are corrupted, return error to prevent panic
				return n, errors.New("RingBuffer: internal state corrupted")
			}
			toRead = r.buf[r.w:r.r]
		}

		nr, rerr := rd.Read(toRead)
		if rerr != nil && rerr != io.EOF {
			err = r.setErr(rerr, true)
			break
		}
		if nr == 0 && rerr == nil {
			zeroReads++
			if zeroReads >= 100 {
				err = r.setErr(io.ErrNoProgress, true)
			}
			continue
		}
		zeroReads = 0

		// Update write pointer with proper wrap-around using modulo
		r.w = (r.w + nr) % r.size
		r.isFull = r.r == r.w && nr > 0
		n += int64(nr)
		r.writeCond.Broadcast()
		if rerr == io.EOF {
			// We do not close.
			break
		}
	}
	return n, err
}

// WriteTo writes data to w until there's no more data to write or
// when an error occurs. The return value n is the number of bytes
// written. Any error encountered during the write is also returned.
//
// If a non-nil error is returned the write side will also see the error.
func (r *RingBuffer) WriteTo(w io.Writer) (n int64, err error) {
	if !r.block {
		return 0, errors.New("RingBuffer: WriteTo only available in blocking mode")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	// Don't write more than half, to unblock reads earlier.
	maxWrite := len(r.buf) / 2
	// But write at least 8K if possible
	if maxWrite < 8<<10 {
		maxWrite = len(r.buf)
	}
	for {
		if err = r.readErr(true); err != nil {
			break
		}
		if r.r == r.w && !r.isFull {
			// Wait for a write to make space
			if !r.waitWrite() {
				return 0, context.DeadlineExceeded
			}
			continue
		}

		var toWrite []byte
		if r.r >= r.w {
			// After writer, we can write until end of buffer
			toWrite = r.buf[r.r:]
		} else {
			// Before reader, we can read until writer.
			toWrite = r.buf[r.r:r.w]
		}
		toWrite = toWrite[:min(len(toWrite), maxWrite)]
		// Unlock while reading
		r.mu.Unlock()
		nr, werr := w.Write(toWrite)
		r.mu.Lock()
		if werr != nil {
			n += int64(nr)
			err = r.setErr(werr, true)
			break
		}
		if nr != len(toWrite) {
			err = r.setErr(io.ErrShortWrite, true)
			break
		}
		r.r += nr
		if r.r == r.size {
			r.r = 0
		}
		r.isFull = false
		n += int64(nr)
		r.readCond.Broadcast()
	}
	if err == io.EOF {
		err = nil
	}
	return n, err
}

// Copy will pipe all data from the reader to the writer through the ringbuffer.
// The ringbuffer will switch to blocking mode.
// Reads and writes will be done async.
// No internal mem-copies are used for the transfer.
//
// Calling CloseWithError will cancel the transfer and make the function return when
// any ongoing reads or writes have finished.
//
// Calling Read or Write functions concurrently with running this will lead to unpredictable results.
func (r *RingBuffer) Copy(dst io.Writer, src io.Reader) (written int64, err error) {
	r.SetBlocking(true)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = r.ReadFrom(src) //nolint errcheck
		r.CloseWriter()
	}()
	defer wg.Wait()
	return r.WriteTo(dst)
}

// Pipe creates an asynchronous in-memory pipe from this RingBuffer.
// The buffer is switched to blocking mode.
func (r *RingBuffer) Pipe() (*PipeReader, *PipeWriter) {
	return PipeFrom(r.Ring)
}

// WriteCloser returns an io.WriteCloser that writes to the ring buffer.
// When the returned WriteCloser is closed, it will wait for all data to be read before returning.
func (r *RingBuffer) WriteCloser() io.WriteCloser {
	return r.Ring.WriteCloser()
}

// ReadCloser returns an io.ReadCloser that reads from the ring buffer.
// When the returned ReadCloser is closed, ErrReaderClosed will be returned on any writes done afterwards.
func (r *RingBuffer) ReadCloser() io.ReadCloser {
	return r.Ring.ReadCloser()
}
