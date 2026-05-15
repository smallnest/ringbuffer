// Copyright 2019 smallnest. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package ringbuffer

import (
	"context"
	"errors"
	"io"
	"iter"
	"sync"
	"time"
)

var (
	// ErrTooMuchDataToWrite is returned when the data to write is more than the buffer size.
	ErrTooMuchDataToWrite = errors.New("too much data to write")

	// ErrIsFull is returned when the buffer is full and not blocking.
	ErrIsFull = errors.New("ringbuffer is full")

	// ErrIsEmpty is returned when the buffer is empty and not blocking.
	ErrIsEmpty = errors.New("ringbuffer is empty")

	// ErrIsNotEmpty is returned when the buffer is not empty and not blocking.
	ErrIsNotEmpty = errors.New("ringbuffer is not empty")

	// ErrAcquireLock is returned when the lock is not acquired on Try operations.
	ErrAcquireLock = errors.New("unable to acquire lock")

	// ErrWriteOnClosed is returned when write on a closed ringbuffer.
	ErrWriteOnClosed = errors.New("write on closed ringbuffer")

	// ErrReaderClosed is returned when a ReadClosed closed the ringbuffer.
	ErrReaderClosed = errors.New("reader closed")

	// ErrReset is returned when Reset() is called, causing pending operations to abort.
	ErrReset = errors.New("reset called")
)

// Ring is a generic circular buffer.
// It operates like a buffered pipe, where data is written to a Ring
// and can be read back from another goroutine.
// It is safe to concurrently read and write Ring.
type Ring[T any] struct {
	buf        []T
	size       int
	r          int // next position to read
	w          int // next position to write
	isFull     bool
	err        error
	block      bool
	overwrite  bool          // when true, overwrite old data when buffer is full
	rTimeout   time.Duration // Applies to writes (waits for the read condition)
	wTimeout   time.Duration // Applies to read (wait for the write condition)
	mu         sync.Mutex
	wg         sync.WaitGroup
	readCond   *sync.Cond // Signaled when data has been read.
	writeCond  *sync.Cond // Signaled when data has been written.
	generation int64      // Incremented on Reset() to invalidate current waiters
	done       chan struct{}
	closeOnce  sync.Once
}

// NewRing returns a new Ring whose buffer has the given size.
func NewRing[T any](size int) *Ring[T] {
	if size < 0 {
		size = 0
	}
	return &Ring[T]{
		buf:  make([]T, size),
		size: size,
		done: make(chan struct{}),
	}
}

// NewRingFromSlice returns a new Ring whose buffer is provided.
func NewRingFromSlice[T any](b []T) *Ring[T] {
	return &Ring[T]{
		buf:  b,
		size: len(b),
		done: make(chan struct{}),
	}
}

// SetBlocking sets the blocking mode of the ring buffer.
// If block is true, Read and Write will block when there is no data to read or no space to write.
// If block is false, Read and Write will return ErrIsEmpty or ErrIsFull immediately.
// By default, the ring buffer is not blocking.
// This setting should be called before any Read or Write operation or after a Reset.
func (r *Ring[T]) SetBlocking(block bool) *Ring[T] {
	r.block = block
	if block {
		r.readCond = sync.NewCond(&r.mu)
		r.writeCond = sync.NewCond(&r.mu)
	}
	return r
}

// SetOverwrite sets the overwrite mode of the ring buffer.
// If overwrite is true, Write operations will overwrite the oldest data when the buffer is full,
// similar to a traditional circular buffer. The read pointer will advance to skip overwritten data.
// If overwrite is false (default), Write will return ErrIsFull or block (if blocking mode is enabled).
func (r *Ring[T]) SetOverwrite(overwrite bool) *Ring[T] {
	r.overwrite = overwrite
	return r
}

// WithCancel sets a context to cancel the ring buffer.
// When the context is canceled, the ring buffer will be closed with the context error.
// The goroutine will exit cleanly when the ring buffer is closed.
func (r *Ring[T]) WithCancel(ctx context.Context) *Ring[T] {
	go func() {
		select {
		case <-ctx.Done():
			r.CloseWithError(ctx.Err())
		case <-r.done:
		}
	}()
	return r
}

// WithTimeout will set a blocking read/write timeout.
// If no reads or writes occur within the timeout,
// the ringbuffer will be closed and context.DeadlineExceeded will be returned.
// A timeout of 0 or less will disable timeouts (default).
func (r *Ring[T]) WithTimeout(d time.Duration) *Ring[T] {
	r.mu.Lock()
	r.rTimeout = d
	r.wTimeout = d
	r.mu.Unlock()
	return r
}

// WithReadTimeout will set a blocking read timeout.
// Reads refers to any call that reads data from the buffer.
// If no writes occur within the timeout,
// the ringbuffer will be closed and context.DeadlineExceeded will be returned.
// A timeout of 0 or less will disable timeouts (default).
func (r *Ring[T]) WithReadTimeout(d time.Duration) *Ring[T] {
	r.mu.Lock()
	// Read operations wait for writes to complete,
	// therefore we set the wTimeout.
	r.wTimeout = d
	r.mu.Unlock()
	return r
}

// WithWriteTimeout will set a blocking write timeout.
// Write refers to any call that writes data into the buffer.
// If no reads occur within the timeout,
// the ringbuffer will be closed and context.DeadlineExceeded will be returned.
// A timeout of 0 or less will disable timeouts (default).
func (r *Ring[T]) WithWriteTimeout(d time.Duration) *Ring[T] {
	r.mu.Lock()
	// Write operations wait for reads to complete,
	// therefore we set the rTimeout.
	r.rTimeout = d
	r.mu.Unlock()
	return r
}

func (r *Ring[T]) setErr(err error, locked bool) error {
	if !locked {
		r.mu.Lock()
		defer r.mu.Unlock()
	}
	if r.err != nil && r.err != io.EOF {
		return r.err
	}

	switch err {
	// Internal errors are transient
	case nil, ErrIsEmpty, ErrIsFull, ErrAcquireLock, ErrTooMuchDataToWrite, ErrIsNotEmpty:
		return err
	default:
		r.err = err
		if r.block {
			r.readCond.Broadcast()
			r.writeCond.Broadcast()
		}
	}
	return err
}

func (r *Ring[T]) readErr(locked bool) error {
	if !locked {
		r.mu.Lock()
		defer r.mu.Unlock()
	}
	if r.err != nil {
		if r.err == io.EOF {
			if r.w == r.r && !r.isFull {
				return io.EOF
			}
			return nil
		}
		return r.err
	}
	return nil
}

// Read reads up to len(p) items into p. It returns the number of items read (0 <= n <= len(p)) and any error encountered.
// Even if Read returns n < len(p), it may use all of p as scratch space during the call.
// If some data is available but not len(p) items, Read conventionally returns what is available instead of waiting for more.
// When Read encounters an error or end-of-file condition after successfully reading n > 0 items, it returns the number of items read.
// It may return the (non-nil) error from the same call or return the error (and n == 0) from a subsequent call.
// Callers should always process the n > 0 items returned before considering the error err.
// Doing so correctly handles I/O errors that happen after reading some items and also both of the allowed EOF behaviors.
func (r *Ring[T]) Read(p []T) (n int, err error) {
	if len(p) == 0 {
		return 0, r.readErr(false)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.readErr(true); err != nil {
		return 0, err
	}

	r.wg.Add(1)
	defer r.wg.Done()
	n, err = r.read(p)
	for err == ErrIsEmpty && r.block {
		// Record the generation before waiting. If Reset() increments the generation,
		// we will detect it after waking up and continue waiting (as if nothing happened).
		myGen := r.generation
		if !r.waitWrite() {
			return 0, context.DeadlineExceeded
		}
		// If generation changed, Reset() happened while we were waiting.
		// Go back to waiting - the buffer was reset and we should continue as normal.
		if myGen != r.generation {
			continue
		}
		if err = r.readErr(true); err != nil {
			break
		}
		n, err = r.read(p)
	}
	if r.block && n > 0 {
		r.readCond.Broadcast()
	}
	return n, err
}

// TryRead read up to len(p) items into p like Read, but it is never blocking.
// If it does not succeed to acquire the lock, it returns ErrAcquireLock.
func (r *Ring[T]) TryRead(p []T) (n int, err error) {
	ok := r.mu.TryLock()
	if !ok {
		return 0, ErrAcquireLock
	}
	defer r.mu.Unlock()
	if err := r.readErr(true); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, r.readErr(true)
	}

	n, err = r.read(p)
	if r.block && n > 0 {
		r.readCond.Broadcast()
	}
	return n, err
}

// copyFromBuffer copies data from the ring buffer to dst without modifying the read pointer.
// Returns the number of items copied. Does not check for errors.
func (r *Ring[T]) copyFromBuffer(dst []T) int {
	if r.w == r.r && !r.isFull {
		return 0
	}

	var n int
	if r.w > r.r {
		n = min(r.w-r.r, len(dst))
		copy(dst, r.buf[r.r:r.r+n])
		return n
	}

	n = min(r.size-r.r+r.w, len(dst))

	if r.r+n <= r.size {
		copy(dst, r.buf[r.r:r.r+n])
	} else {
		c1 := r.size - r.r
		copy(dst, r.buf[r.r:r.size])
		c2 := n - c1
		copy(dst[c1:], r.buf[0:c2])
	}
	return n
}

func (r *Ring[T]) read(p []T) (n int, err error) {
	if r.w == r.r && !r.isFull {
		return 0, ErrIsEmpty
	}

	n = r.copyFromBuffer(p)
	if n == 0 {
		return 0, ErrIsEmpty
	}

	r.r = (r.r + n) % r.size
	r.isFull = false

	return n, r.readErr(true)
}

// Returns true if a read may have happened.
// Returns false if waited longer than rTimeout.
// Must be called when locked and returns locked.
func (r *Ring[T]) waitRead() (ok bool) {
	if r.rTimeout <= 0 {
		r.readCond.Wait()
		return true
	}
	start := time.Now()
	defer time.AfterFunc(r.rTimeout, r.readCond.Broadcast).Stop()

	r.readCond.Wait()
	if time.Since(start) >= r.rTimeout {
		r.setErr(context.DeadlineExceeded, true) //nolint errcheck
		return false
	}
	return true
}

func (r *Ring[T]) readOne() (b T, err error) {
	if err = r.readErr(true); err != nil {
		return b, err
	}
	for r.w == r.r && !r.isFull {
		if r.block {
			// Record the generation before waiting. If Reset() increments the generation,
			// we will detect it after waking up and continue waiting.
			myGen := r.generation
			if !r.waitWrite() {
				return b, context.DeadlineExceeded
			}
			// If generation changed, Reset() happened while we were waiting.
			// Go back to waiting - the buffer was reset.
			if myGen != r.generation {
				continue
			}
			err = r.readErr(true)
			if err != nil {
				return b, err
			}
			continue
		}
		return b, ErrIsEmpty
	}
	b = r.buf[r.r]
	r.r++
	if r.r == r.size {
		r.r = 0
	}

	r.isFull = false
	return b, r.readErr(true)
}

// ReadOne reads and returns the next item from the input or ErrIsEmpty.
func (r *Ring[T]) ReadOne() (b T, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.readOne()
}

// TryReadOne reads and returns the next item without blocking.
// If it does not succeed to acquire the lock, it returns ErrAcquireLock.
func (r *Ring[T]) TryReadOne() (T, error) {
	var zero T
	ok := r.mu.TryLock()
	if !ok {
		return zero, ErrAcquireLock
	}
	defer r.mu.Unlock()
	if err := r.readErr(true); err != nil {
		return zero, err
	}
	return r.readOne()
}

// checkWriteErr checks if the buffer has an error that should prevent writes.
// Returns the appropriate error to return (nil if write can proceed).
// Must be called with mutex held.
func (r *Ring[T]) checkWriteErr() error {
	if r.err == nil {
		return nil
	}
	if r.err == io.EOF {
		return ErrWriteOnClosed
	}
	return r.err
}

// Write writes len(p) items from p to the underlying buf.
// It returns the number of items written from p (0 <= n <= len(p))
// and any error encountered that caused the write to stop early.
// If blocking n < len(p) will be returned only if an error occurred.
// Write returns a non-nil error if it returns n < len(p).
// Write will not modify the slice data, even temporarily.
func (r *Ring[T]) Write(p []T) (n int, err error) {
	if len(p) == 0 {
		return 0, r.setErr(nil, false)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkWriteErr(); err != nil {
		return 0, err
	}
	wrote := 0
	for len(p) > 0 {
		n, err = r.write(p)
		wrote += n
		if !r.block || err == nil {
			break
		}
		err = r.setErr(err, true)
		if r.block && (err == ErrIsFull || err == ErrTooMuchDataToWrite) {
			// Record the generation before waiting. If Reset() increments the generation,
			// we will detect it after waking up and continue waiting.
			myGen := r.generation
			r.writeCond.Broadcast()
			r.waitRead()
			// If generation changed, Reset() happened while we were waiting.
			// Go back to waiting - the buffer was reset.
			if myGen != r.generation {
				p = p[n:]
				err = nil
				continue
			}
			p = p[n:]
			err = nil
			continue
		}
		break
	}
	if r.block && wrote > 0 {
		r.writeCond.Broadcast()
	}

	return wrote, r.setErr(err, true)
}

// waitWrite will wait for a write event.
// Returns true if a write may have happened.
// Returns false if waited longer than wTimeout.
// Must be called when locked and returns locked.
func (r *Ring[T]) waitWrite() (ok bool) {
	if r.wTimeout <= 0 {
		r.writeCond.Wait()
		return true
	}

	start := time.Now()
	defer time.AfterFunc(r.wTimeout, r.writeCond.Broadcast).Stop()

	r.writeCond.Wait()
	if time.Since(start) >= r.wTimeout {
		r.setErr(context.DeadlineExceeded, true) //nolint errcheck
		return false
	}
	return true
}

// TryWrite writes len(p) items from p to the underlying buf like Write, but it is not blocking.
// If it does not succeed to acquire the lock, it returns ErrAcquireLock.
func (r *Ring[T]) TryWrite(p []T) (n int, err error) {
	if len(p) == 0 {
		return 0, r.setErr(nil, false)
	}
	ok := r.mu.TryLock()
	if !ok {
		return 0, ErrAcquireLock
	}
	defer r.mu.Unlock()
	if err := r.checkWriteErr(); err != nil {
		return 0, err
	}

	n, err = r.write(p)
	if r.block && n > 0 {
		r.writeCond.Broadcast()
	}
	return n, r.setErr(err, true)
}

func (r *Ring[T]) write(p []T) (n int, err error) {
	// In overwrite mode, we always allow writing by discarding old data
	if r.overwrite && len(p) > 0 {
		var avail int
		if r.w == r.r && r.isFull {
			// Buffer is full, no space available
			avail = 0
		} else if r.w >= r.r {
			avail = r.size - r.w + r.r
		} else {
			avail = r.r - r.w
		}

		// If we need more space than available, discard old data
		if len(p) > avail {
			// Advance read pointer to make room
			needed := len(p) - avail
			r.r = (r.r + needed) % r.size
			// If buffer was full, it's no longer full after advancing read pointer
			r.isFull = false
		}
	}

	if r.isFull {
		return 0, ErrIsFull
	}

	var avail int
	if r.w >= r.r {
		avail = r.size - r.w + r.r
	} else {
		avail = r.r - r.w
	}

	if len(p) > avail {
		err = ErrTooMuchDataToWrite
		p = p[:avail]
	}
	n = len(p)

	if r.w >= r.r {
		c1 := r.size - r.w
		if c1 >= n {
			copy(r.buf[r.w:], p)
			r.w += n
		} else {
			copy(r.buf[r.w:], p[:c1])
			c2 := n - c1
			copy(r.buf[0:], p[c1:])
			r.w = c2
		}
	} else {
		copy(r.buf[r.w:], p)
		r.w += n
	}

	if r.w == r.size {
		r.w = 0
	}
	if r.w == r.r {
		r.isFull = true
	}

	return n, err
}

// WriteOne writes one item into buffer, and returns ErrIsFull if the buffer is full.
func (r *Ring[T]) WriteOne(c T) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.checkWriteErr(); err != nil {
		return err
	}
	err := r.writeByte(c)
	for err == ErrIsFull && r.block {
		// Record the generation before waiting. If Reset() increments the generation,
		// we will detect it after waking up and continue waiting.
		myGen := r.generation
		if !r.waitRead() {
			return context.DeadlineExceeded
		}
		// If generation changed, Reset() happened while we were waiting.
		// Go back to waiting - the buffer was reset.
		if myGen != r.generation {
			err = r.writeByte(c)
			continue
		}
		err = r.setErr(r.writeByte(c), true)
	}
	if r.block && err == nil {
		r.writeCond.Broadcast()
	}
	return err
}

// TryWriteOne writes one item into buffer without blocking.
// If it does not succeed to acquire the lock, it returns ErrAcquireLock.
func (r *Ring[T]) TryWriteOne(c T) error {
	ok := r.mu.TryLock()
	if !ok {
		return ErrAcquireLock
	}
	defer r.mu.Unlock()
	if err := r.checkWriteErr(); err != nil {
		return err
	}

	err := r.writeByte(c)
	if err == nil && r.block {
		r.writeCond.Broadcast()
	}
	return err
}

func (r *Ring[T]) writeByte(c T) error {
	if r.err != nil {
		return r.err
	}
	if r.w == r.r && r.isFull {
		// In overwrite mode, discard the oldest item and write the new one
		if r.overwrite {
			r.r++
			if r.r == r.size {
				r.r = 0
			}
		} else {
			return ErrIsFull
		}
	}
	r.buf[r.w] = c
	r.w++

	if r.w == r.size {
		r.w = 0
	}
	if r.w == r.r {
		r.isFull = true
	}

	return nil
}

// Length returns the number of items that can be read without blocking.
func (r *Ring[T]) Length() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.w == r.r {
		if r.isFull {
			return r.size
		}
		return 0
	}

	if r.w > r.r {
		return r.w - r.r
	}

	return r.size - r.r + r.w
}

// Capacity returns the size of the underlying buffer.
func (r *Ring[T]) Capacity() int {
	return r.size
}

// Free returns the number of items that can be written without blocking.
func (r *Ring[T]) Free() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.w == r.r {
		if r.isFull {
			return 0
		}
		return r.size
	}

	if r.w < r.r {
		return r.r - r.w
	}

	return r.size - r.w + r.r
}

// IsFull returns true when the ringbuffer is full.
func (r *Ring[T]) IsFull() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.isFull
}

// IsEmpty returns true when the ringbuffer is empty.
func (r *Ring[T]) IsEmpty() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return !r.isFull && r.w == r.r
}

// CloseWithError closes the writer; reads will return
// no items and the error err, or EOF if err is nil.
//
// CloseWithError never overwrites the previous error if it exists
// and always returns nil.
func (r *Ring[T]) CloseWithError(err error) {
	if err == nil {
		err = io.EOF
	}
	r.setErr(err, false) //nolint errcheck
	r.closeOnce.Do(func() { close(r.done) })
}

// CloseWriter closes the writer.
// Reads will return any remaining items and io.EOF.
func (r *Ring[T]) CloseWriter() {
	r.setErr(io.EOF, false) //nolint errcheck
	r.closeOnce.Do(func() { close(r.done) })
}

// Flush waits for the buffer to be empty and fully read.
// If not blocking ErrIsNotEmpty will be returned if the buffer still contains data.
func (r *Ring[T]) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for r.w != r.r || r.isFull {
		err := r.readErr(true)
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}
		if !r.block {
			return ErrIsNotEmpty
		}
		if !r.waitRead() {
			return context.DeadlineExceeded
		}
	}

	err := r.readErr(true)
	if err == io.EOF {
		return nil
	}
	return err
}

// Reset the read pointer and writer pointer to zero.
func (r *Ring[T]) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Signal any WithCancel goroutines to exit
	r.closeOnce.Do(func() { close(r.done) })

	if r.block {
		// In blocking mode, increment generation to invalidate current waiters.
		// They will wake up, see the generation changed, and go back to waiting.
		// This makes Reset() transparent to waiters - they continue waiting as if nothing happened.
		//
		// Note: We don't wait for wg.Wait() in blocking mode since waiters may
		// block indefinitely. The generation mechanism ensures they handle the reset correctly.
		r.generation++
		r.readCond.Broadcast()
		r.writeCond.Broadcast()
		r.r = 0
		r.w = 0
		r.err = nil
		r.isFull = false
		clear(r.buf)
		r.done = make(chan struct{})
		r.closeOnce = sync.Once{}
		return
	}

	// In non-blocking mode, set error to return immediately to any readers/writers.
	r.setErr(ErrReset, true) //nolint errcheck

	// Unlock the mutex so readers/writers can finish.
	r.mu.Unlock()
	r.wg.Wait()
	r.mu.Lock()
	r.r = 0
	r.w = 0
	r.err = nil
	r.isFull = false
	clear(r.buf)
	r.done = make(chan struct{})
	r.closeOnce = sync.Once{}
}

// WriteCloser returns a WriteCloser that writes to the ring buffer.
// When the returned WriteCloser is closed, it will wait for all data to be read before returning.
func (r *Ring[T]) WriteCloser() *writeCloser[T] {
	return &writeCloser[T]{r}
}

type writeCloser[T any] struct {
	*Ring[T]
}

// Close provides a close method for the WriteCloser.
func (wc *writeCloser[T]) Close() error {
	wc.CloseWriter()
	return wc.Flush()
}

// ReadCloser returns a ReadCloser that reads to the ring buffer.
// When the returned ReadCloser is closed, ErrReaderClosed will be returned on any writes done afterwards.
func (r *Ring[T]) ReadCloser() *readCloser[T] {
	return &readCloser[T]{r}
}

type readCloser[T any] struct {
	*Ring[T]
}

// Close provides a close method for the ReadCloser.
func (rc *readCloser[T]) Close() error {
	rc.CloseWithError(ErrReaderClosed)
	err := rc.readErr(false)
	if err == ErrReaderClosed {
		err = nil
	}
	return err
}

// Peek reads up to len(p) items into p without moving the read pointer.
func (r *Ring[T]) Peek(p []T) (n int, err error) {
	if len(p) == 0 {
		return 0, r.readErr(false)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.readErr(true); err != nil {
		return 0, err
	}

	return r.peek(p)
}

func (r *Ring[T]) peek(p []T) (n int, err error) {
	n = r.copyFromBuffer(p)
	if n == 0 {
		return 0, ErrIsEmpty
	}
	return n, r.readErr(true)
}

// Bytes returns all available read items.
// It does not move the read pointer and only copies the available data.
// If the dst is big enough, it will be used as destination,
// otherwise a new buffer will be allocated.
func (r *Ring[T]) Bytes(dst []T) []T {
	r.mu.Lock()
	defer r.mu.Unlock()
	getDst := func(n int) []T {
		if cap(dst) < n {
			return make([]T, n)
		}
		return dst[:n]
	}

	if r.w == r.r {
		if r.isFull {
			buf := getDst(r.size)
			copy(buf, r.buf[r.r:])
			copy(buf[r.size-r.r:], r.buf[:r.w])
			return buf
		}
		return nil
	}

	if r.w > r.r {
		buf := getDst(r.w - r.r)
		copy(buf, r.buf[r.r:r.w])
		return buf
	}

	n := r.size - r.r + r.w
	buf := getDst(n)

	if r.r+n < r.size {
		copy(buf, r.buf[r.r:r.r+n])
	} else {
		c1 := r.size - r.r
		copy(buf, r.buf[r.r:r.size])
		c2 := n - c1
		copy(buf[c1:], r.buf[0:c2])
	}

	return buf
}

// All returns an iterator over the unread items in the buffer.
// It does not modify the read pointer.
func (r *Ring[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		r.mu.Lock()
		defer r.mu.Unlock()

		if r.w == r.r && !r.isFull {
			return
		}

		if r.w > r.r {
			for i := r.r; i < r.w; i++ {
				if !yield(r.buf[i]) {
					return
				}
			}
			return
		}

		// Handle wrap-around
		for i := r.r; i < r.size; i++ {
			if !yield(r.buf[i]) {
				return
			}
		}
		for i := 0; i < r.w; i++ {
			if !yield(r.buf[i]) {
				return
			}
		}
	}
}
