package ringbuffer

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"
)

// idleReader delivers a fixed set of chunks, then blocks in Read until gate is
// closed — modelling a network source that has gone quiet with nothing to send.
type idleReader struct {
	chunks [][]byte
	i      int
	gate   chan struct{}
}

func (rd *idleReader) Read(p []byte) (int, error) {
	if rd.i < len(rd.chunks) {
		n := copy(p, rd.chunks[rd.i])
		rd.i++
		return n, nil
	}
	<-rd.gate // idle: block here without any lock held by ReadFrom
	return 0, io.EOF
}

// mustReturn runs fn in a goroutine and fails if it does not complete within d.
// Before the fix, an inspection call made while ReadFrom is blocked in rd.Read
// never returns because ReadFrom holds r.mu across the blocked Read.
func mustReturn(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { fn(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s blocked for more than %v while ReadFrom was idle in rd.Read", what, d)
	}
}

// TestReadFromIdleSourceDoesNotBlockInspectors is the regression test for #28:
// while ReadFrom is parked in a blocked rd.Read, the read/inspection side of the
// buffer must stay responsive.
func TestReadFromIdleSourceDoesNotBlockInspectors(t *testing.T) {
	rb := New(64).SetBlocking(true)
	rd := &idleReader{chunks: [][]byte{[]byte("hello")}, gate: make(chan struct{})}

	rfDone := make(chan struct{})
	go func() { rb.ReadFrom(rd); close(rfDone) }()

	// Wait until the first chunk has been committed. After committing it,
	// ReadFrom loops, calls rd.Read again, and blocks on the gate — now with
	// r.mu released.
	deadline := time.After(2 * time.Second)
	for rb.Length() != 5 {
		select {
		case <-deadline:
			t.Fatalf("data never landed; Length = %d", rb.Length())
		default:
			time.Sleep(time.Millisecond)
		}
	}

	mustReturn(t, 2*time.Second, "Length", func() { rb.Length() })
	mustReturn(t, 2*time.Second, "Free", func() { rb.Free() })
	mustReturn(t, 2*time.Second, "IsEmpty", func() { rb.IsEmpty() })
	mustReturn(t, 2*time.Second, "Peek", func() {
		p := make([]byte, 5)
		if n, _ := rb.Peek(p); string(p[:n]) != "hello" {
			t.Errorf("Peek = %q, want %q", p[:n], "hello")
		}
	})
	mustReturn(t, 2*time.Second, "Read", func() {
		p := make([]byte, 5)
		if n, _ := rb.Read(p); string(p[:n]) != "hello" {
			t.Errorf("Read = %q, want %q", p[:n], "hello")
		}
	})

	close(rd.gate) // let the source finish
	select {
	case <-rfDone:
	case <-time.After(2 * time.Second):
		t.Fatal("ReadFrom did not return after the source closed")
	}
}

// slowReader hands back one byte at a time with a small pause, so ReadFrom
// spends most of its life blocked in Read with the lock released.
type slowReader struct {
	data []byte
	i    int
}

func (rd *slowReader) Read(p []byte) (int, error) {
	if rd.i >= len(rd.data) {
		return 0, io.EOF
	}
	time.Sleep(200 * time.Microsecond)
	p[0] = rd.data[rd.i]
	rd.i++
	return 1, nil
}

// TestReadFromConcurrentWithInspectorsAndDrain exercises ReadFrom against a slow
// source while other goroutines inspect and drain, to catch races in the
// reserve/commit handoff under -race.
func TestReadFromConcurrentWithInspectorsAndDrain(t *testing.T) {
	const payload = "the quick brown fox jumps over the lazy dog, twice over"
	rb := New(8).SetBlocking(true) // small buffer forces many wrap/full cycles

	var wg sync.WaitGroup

	// Writer.
	wg.Go(func() {
		rb.ReadFrom(&slowReader{data: []byte(payload)})
		rb.CloseWriter()
	})

	// Inspectors: must never observe corruption and must always return.
	stop := make(chan struct{})
	wg.Go(func() {
		p := make([]byte, 8)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = rb.Length()
			_ = rb.Free()
			_, _ = rb.Peek(p)
		}
	})

	// Drainer.
	var got bytes.Buffer
	wg.Go(func() {
		defer close(stop)
		p := make([]byte, 3)
		for {
			n, err := rb.Read(p)
			got.Write(p[:n])
			if err == io.EOF {
				return
			}
			if err != nil {
				t.Errorf("Read error: %v", err)
				return
			}
		}
	})

	wg.Wait()
	if got.String() != payload {
		t.Errorf("drained %q, want %q", got.String(), payload)
	}
}

// TestReadFromConcurrentWithReset hammers Reset while ReadFrom is running, to
// exercise the generation snapshot/compare path (a Reset landing while we are
// blocked in rd.Read must be detected and the read discarded, not committed
// against a stale write pointer). Safety/termination only — content after a
// mid-copy Reset is intentionally undefined.
func TestReadFromConcurrentWithReset(t *testing.T) {
	rb := New(8).SetBlocking(true)

	stop := make(chan struct{})
	var rf sync.WaitGroup // the ReadFrom writer
	var churn sync.WaitGroup

	rf.Go(func() {
		// A reader that keeps producing until told to stop, then EOFs.
		rb.ReadFrom(readerFunc(func(p []byte) (int, error) {
			select {
			case <-stop:
				return 0, io.EOF
			default:
				time.Sleep(100 * time.Microsecond)
				p[0] = 'x'
				return 1, nil
			}
		}))
	})

	// Reset and drain concurrently for a bounded number of rounds.
	churn.Go(func() {
		p := make([]byte, 4)
		for range 500 {
			rb.Reset()
			_, _ = rb.Peek(p)
			if l := rb.Length(); l < 0 || l > 8 {
				t.Errorf("Length out of range: %d", l)
				return
			}
			time.Sleep(50 * time.Microsecond)
		}
	})

	churn.Go(func() {
		p := make([]byte, 4)
		for range 500 {
			_, _ = rb.Read(p)
			time.Sleep(50 * time.Microsecond)
		}
	})

	// Shut down only after the churn goroutines are done, so no late Reset can
	// clear the EOF and strand the winddown.
	churn.Wait()
	close(stop)
	rb.CloseWriter() // unblock a ReadFrom parked in waitRead on a full buffer

	done := make(chan struct{})
	go func() { rf.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ReadFrom did not settle; possible deadlock in the reserve/commit path")
	}
}

// readerFunc adapts a function to io.Reader.
type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }
