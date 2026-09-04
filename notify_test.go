package ringbuffer

import (
	"io"
	"testing"
	"time"
)

// waitNotify reports whether a signal arrives within d.
func waitNotify(rb *RingBuffer, d time.Duration) bool {
	select {
	case <-rb.Notify():
		return true
	case <-time.After(d):
		return false
	}
}

// TestNotifyOnWrite is the basic contract: a consumer parked on Notify wakes
// when data lands.
func TestNotifyOnWrite(t *testing.T) {
	rb := New(64)

	if waitNotify(rb, 50*time.Millisecond) {
		t.Fatal("signalled before anything was written")
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		rb.Write([]byte("hello"))
	}()

	if !waitNotify(rb, 2*time.Second) {
		t.Fatal("no signal after a write")
	}
}

// TestNotifyWorksInNonBlockingMode — the existing wakeups are all guarded by
// r.block, so signalling only alongside them would leave Notify silent for a
// non-blocking buffer. It is signalled from the write path instead.
func TestNotifyWorksInNonBlockingMode(t *testing.T) {
	rb := New(64) // default: non-blocking
	rb.Write([]byte("x"))
	if !waitNotify(rb, time.Second) {
		t.Fatal("no signal in non-blocking mode")
	}
}

// TestNotifyCoalesces — capacity 1, so a burst of writes leaves exactly one
// pending signal. The contract is "something changed", not a count, and a woken
// consumer re-inspects the buffer.
func TestNotifyCoalesces(t *testing.T) {
	rb := New(64)
	for i := 0; i < 10; i++ {
		rb.Write([]byte("x"))
	}

	if !waitNotify(rb, time.Second) {
		t.Fatal("no signal after writes")
	}
	if waitNotify(rb, 50*time.Millisecond) {
		t.Error("a second signal was queued; signals must coalesce")
	}
}

// TestNotifyOnClose — a consumer waiting for data must also be woken when there
// will never be any, or it parks forever on a closed buffer.
func TestNotifyOnClose(t *testing.T) {
	rb := New(64).SetBlocking(true)
	drain(t, rb)

	go func() {
		time.Sleep(20 * time.Millisecond)
		rb.CloseWriter()
	}()

	if !waitNotify(rb, 2*time.Second) {
		t.Fatal("no signal on CloseWriter; a waiting consumer would hang")
	}
}

// TestNotifyOnCloseWithError — same, for the failure path.
func TestNotifyOnCloseWithError(t *testing.T) {
	rb := New(64).SetBlocking(true)
	drain(t, rb)

	go func() {
		time.Sleep(20 * time.Millisecond)
		rb.CloseWithError(io.ErrUnexpectedEOF)
	}()

	if !waitNotify(rb, 2*time.Second) {
		t.Fatal("no signal on CloseWithError; a waiting consumer would hang")
	}
}

// TestNotifyDoesNotConsume — Notify must not disturb the buffer. A consumer
// woken by it still has to Peek to see the data, and the data must be intact.
func TestNotifyDoesNotConsume(t *testing.T) {
	rb := New(64)
	rb.Write([]byte("payload"))
	<-rb.Notify()

	if got := rb.Length(); got != 7 {
		t.Errorf("Length after Notify = %d, want 7", got)
	}
	p := make([]byte, 7)
	n, err := rb.Peek(p)
	if err != nil || string(p[:n]) != "payload" {
		t.Errorf("Peek after Notify = %q err=%v, want %q", p[:n], err, "payload")
	}
}

// TestPeekOnlyNeverSeesEOF documents a trap for consumers that only peek.
//
// readErr reports io.EOF only once the buffer is empty, so data always drains
// before end-of-stream is announced. A consumer that never consumes therefore
// never learns the writer is finished, and will wait forever.
//
// This is correct behaviour, not a defect — but a peek-driven consumer has to
// consume something eventually, and must not treat "no EOF yet" as "the writer
// is still alive".
func TestPeekOnlyNeverSeesEOF(t *testing.T) {
	rb := New(64).SetBlocking(true)
	rb.Write([]byte("data"))
	rb.CloseWriter()

	scratch := make([]byte, 64)
	for i := 0; i < 3; i++ {
		n, err := rb.Peek(scratch)
		if err == io.EOF {
			t.Fatal("Peek reported EOF while data was still buffered")
		}
		if n != 4 {
			t.Fatalf("Peek returned %d bytes, want 4", n)
		}
	}

	// Consuming is what surfaces it.
	rb.Read(make([]byte, 4))
	if _, err := rb.Peek(scratch); err != io.EOF {
		t.Errorf("after draining, Peek err = %v, want io.EOF", err)
	}
}

// TestNotifyDrivesPeekConsumeLoop is the pattern the API exists for: wait on a
// signal, look without consuming, send, and consume only what has been
// confirmed. Consuming on confirmation is also what eventually surfaces EOF.
func TestNotifyDrivesPeekConsumeLoop(t *testing.T) {
	rb := New(1024).SetBlocking(true)

	go func() {
		for _, s := range []string{"alpha", "beta", "gamma"} {
			rb.Write([]byte(s))
			time.Sleep(10 * time.Millisecond)
		}
		rb.CloseWriter()
	}()

	var seen string
	sent := 0
	scratch := make([]byte, 1024)
	discard := make([]byte, 1024)

	deadline := time.After(5 * time.Second)
	for {
		n, err := rb.Peek(scratch)
		if n > sent {
			seen += string(scratch[sent:n]) // only what has not gone out yet
			sent = n
		}
		if err == io.EOF {
			break
		}

		// The client confirms everything sent, so consume it. Until this
		// happens the buffer never empties and EOF never arrives.
		if sent > 0 {
			rb.Read(discard[:sent])
			sent = 0
			continue
		}

		select {
		case <-rb.Notify():
		case <-deadline:
			t.Fatal("loop stalled waiting for a signal")
		}
	}

	if seen != "alphabetagamma" {
		t.Errorf("got %q, want %q", seen, "alphabetagamma")
	}
	if got := rb.Length(); got != 0 {
		t.Errorf("Length = %d, want 0 after everything was confirmed", got)
	}
}

// drain keeps a blocking buffer from wedging a test when nothing reads it.
func drain(t *testing.T, rb *RingBuffer) {
	t.Helper()
	t.Cleanup(func() { rb.CloseWithError(io.ErrClosedPipe) })
}
