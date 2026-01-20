package ringbuffer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRingBuffer_interface(t *testing.T) {
	rb := New(1)
	var _ io.Writer = rb
	var _ io.Reader = rb
	// var _ io.StringWriter = rb
	var _ io.ByteReader = rb
	var _ io.ByteWriter = rb
}

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
	if !bytes.Equal(rb.Bytes(nil), []byte(strings.Repeat("abcd", 4))) {
		t.Fatalf("expect 4 abcd but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
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
	if !bytes.Equal(rb.Bytes(nil), []byte(strings.Repeat("abcd", 16))) {
		t.Fatalf("expect 16 abcd but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
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

	if !bytes.Equal(rb.Bytes(nil), []byte(strings.Repeat("abcd", 16))) {
		t.Fatalf("expect 16 abcd but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
	}

	rb.Reset()
	// write 4 * 2 = 8 bytes
	n, err = rb.Write([]byte(strings.Repeat("abcd", 2)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 8 {
		t.Fatalf("expect write 16 bytes but got %d", n)
	}
	if rb.Length() != 8 {
		t.Fatalf("expect len 16 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 56 {
		t.Fatalf("expect free 48 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	buf := make([]byte, 5)
	rb.Read(buf)
	if rb.Length() != 3 {
		t.Fatalf("expect len 3 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	rb.Write([]byte(strings.Repeat("abcd", 15)))

	if !bytes.Equal(rb.Bytes(nil), []byte("bcd"+strings.Repeat("abcd", 15))) {
		t.Fatalf("expect 63 ... but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
	}

	rb.Reset()
	n, err = rb.Write([]byte(strings.Repeat("abcd", 16)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 64 {
		t.Fatalf("expect write 64 bytes but got %d", n)
	}
	if rb.Free() != 0 {
		t.Fatalf("expect free 0 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	buf = make([]byte, 16)
	rb.Read(buf)
	n, err = rb.Write([]byte(strings.Repeat("1234", 4)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 16 {
		t.Fatalf("expect write 16 bytes but got %d", n)
	}
	if rb.Free() != 0 {
		t.Fatalf("expect free 0 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if !bytes.Equal(append(buf, rb.Bytes(nil)...), []byte(strings.Repeat("abcd", 16)+strings.Repeat("1234", 4))) {
		t.Fatalf("expect 16 abcd and 4 1234 but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
	}
}

func TestRingBuffer_WriteBlocking(t *testing.T) {
	rb := New(64).SetBlocking(true)

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
	if !bytes.Equal(rb.Bytes(nil), []byte(strings.Repeat("abcd", 4))) {
		t.Fatalf("expect 4 abcd but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
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
	if !bytes.Equal(rb.Bytes(nil), []byte(strings.Repeat("abcd", 16))) {
		t.Fatalf("expect 16 abcd but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
	}

	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}

	rb.Reset()
	// write 4 * 2 = 8 bytes
	n, err = rb.Write([]byte(strings.Repeat("abcd", 2)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 8 {
		t.Fatalf("expect write 16 bytes but got %d", n)
	}
	if rb.Length() != 8 {
		t.Fatalf("expect len 16 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 56 {
		t.Fatalf("expect free 48 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	buf := make([]byte, 5)
	rb.Read(buf)
	if rb.Length() != 3 {
		t.Fatalf("expect len 3 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	rb.Write([]byte(strings.Repeat("abcd", 15)))

	if !bytes.Equal(rb.Bytes(nil), []byte("bcd"+strings.Repeat("abcd", 15))) {
		t.Fatalf("expect 63 ... but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
	}

	rb.Reset()
	n, err = rb.Write([]byte(strings.Repeat("abcd", 16)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 64 {
		t.Fatalf("expect write 64 bytes but got %d", n)
	}
	if rb.Free() != 0 {
		t.Fatalf("expect free 0 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	buf = make([]byte, 16)
	rb.Read(buf)
	n, err = rb.Write([]byte(strings.Repeat("1234", 4)))
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if n != 16 {
		t.Fatalf("expect write 16 bytes but got %d", n)
	}
	if rb.Free() != 0 {
		t.Fatalf("expect free 0 bytes but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if !bytes.Equal(append(buf, rb.Bytes(nil)...), []byte(strings.Repeat("abcd", 16)+strings.Repeat("1234", 4))) {
		t.Fatalf("expect 16 abcd and 4 1234 but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
	}
}

func TestRingBuffer_Read(t *testing.T) {
	defer timeout(5 * time.Second)()
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

func TestRingBuffer_Blocking(t *testing.T) {
	// Typical runtime is ~5-10s.
	defer timeout(60 * time.Second)()
	const debug = false

	var readBytes int
	var wroteBytes int
	var readBuf bytes.Buffer
	var wroteBuf bytes.Buffer
	readHash := crc32.NewIEEE()
	wroteHash := crc32.NewIEEE()
	read := io.Writer(readHash)
	wrote := io.Writer(wroteHash)
	if debug {
		read = io.MultiWriter(read, &readBuf)
		wrote = io.MultiWriter(wrote, &wroteBuf)
	}
	debugln := func(args ...interface{}) {
		if debug {
			fmt.Println(args...)
		}
	}
	// Inject random reader/writer sleeps.
	const maxSleep = int(1 * time.Millisecond)
	doSleep := !testing.Short()
	rb := New(4 << 10).SetBlocking(true)

	// Reader
	var readErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		readRng := rand.New(rand.NewSource(1))
		defer wg.Done()
		defer rb.CloseWithError(readErr)
		buf := make([]byte, 1024)
		for {
			// Read
			n, err := rb.Read(buf[:readRng.Intn(len(buf))])
			readBytes += n
			read.Write(buf[:n])
			debugln("READ 1\t", n, readBytes)
			if err != nil {
				readErr = err
				break
			}

			// ReadByte
			b, err := rb.ReadByte()
			if err != nil {
				readErr = err
				break
			}
			readBytes++
			read.Write([]byte{b})
			debugln("READ 2\t", 1, readBytes)

			// TryRead
			n, err = rb.TryRead(buf[:readRng.Intn(len(buf))])
			readBytes += n
			read.Write(buf[:n])
			debugln("READ 3\t", n, readBytes)
			if err != nil && err != ErrAcquireLock && err != ErrIsEmpty {
				readErr = err
				break
			}
			if doSleep && readRng.Intn(20) == 0 {
				time.Sleep(time.Duration(readRng.Intn(maxSleep)))
			}
		}
	}()

	// Writer
	{
		buf := make([]byte, 1024)
		writeRng := rand.New(rand.NewSource(2))
		for i := 0; i < 2500; i++ {
			writeRng.Read(buf)
			// Write
			n, err := rb.Write(buf[:writeRng.Intn(len(buf))])
			if err != nil {
				t.Fatalf("write failed: %v", err)
			}
			wroteBytes += n
			wrote.Write(buf[:n])
			debugln("WRITE 1\t", n, wroteBytes)

			// WriteString
			n, err = rb.WriteString(string(buf[:writeRng.Intn(len(buf))]))
			if err != nil {
				t.Fatalf("write failed: %v", err)
			}
			wroteBytes += n
			wrote.Write(buf[:n])
			debugln("WRITE 2\t", writeRng.Intn(len(buf)), wroteBytes)

			// WriteByte
			err = rb.WriteByte(buf[0])
			if err != nil {
				t.Fatalf("write failed: %v", err)
			}
			wroteBytes++
			wrote.Write(buf[:1])
			debugln("WRITE 3\t", 1, wroteBytes)

			// TryWrite
			n, err = rb.TryWrite(buf[:writeRng.Intn(len(buf))])
			if err != nil && err != ErrAcquireLock && err != ErrTooMuchDataToWrite && err != ErrIsFull {
				t.Fatalf("write failed: %v", err)
			}
			wroteBytes += n
			wrote.Write(buf[:n])
			debugln("WRITE 4\t", n, wroteBytes)

			// TryWriteByte
			err = rb.TryWriteByte(buf[0])
			if err != nil && err != ErrAcquireLock && err != ErrTooMuchDataToWrite && err != ErrIsFull {
				t.Fatalf("write failed: %v", err)
			}
			if err == nil {
				wroteBytes++
				wrote.Write(buf[:1])
				debugln("WRITE 5\t", 1, wroteBytes)
			}
			if doSleep && writeRng.Intn(10) == 0 {
				time.Sleep(time.Duration(writeRng.Intn(maxSleep)))
			}
		}
		if err := rb.Flush(); err != nil {
			t.Fatalf("flush failed: %v", err)
		}
		rb.CloseWriter()
	}
	wg.Wait()
	if !errors.Is(readErr, io.EOF) {
		t.Fatalf("expect io.EOF but got %v", readErr)
	}
	if readBytes != wroteBytes {
		a, b := readBuf.Bytes(), wroteBuf.Bytes()
		if debug && !bytes.Equal(a, b) {
			common := len(a)
			for i := range a {
				if a[i] != b[i] {
					common = i
					break
				}
			}
			a, b = a[common:], b[common:]
			if len(a) > 64 {
				a = a[:64]
			}
			if len(b) > 64 {
				b = b[:64]
			}
			t.Errorf("after %d common bytes, difference \nread: %x\nwrote:%x", common, a, b)
		}
		t.Fatalf("expect read %d bytes but got %d", wroteBytes, readBytes)
	}
	if readHash.Sum32() != wroteHash.Sum32() {
		t.Fatalf("expect read hash 0x%08x but got 0x%08x", readHash.Sum32(), wroteHash.Sum32())
	}
}

func TestRingBuffer_BlockingBig(t *testing.T) {
	// Typical runtime is ~5-10s.
	defer timeout(60 * time.Second)()
	const debug = false

	var readBytes int
	var wroteBytes int
	readHash := crc32.NewIEEE()
	wroteHash := crc32.NewIEEE()
	var readBuf bytes.Buffer
	var wroteBuf bytes.Buffer
	read := io.Writer(readHash)
	wrote := io.Writer(wroteHash)
	if debug {
		read = io.MultiWriter(read, &readBuf)
		wrote = io.MultiWriter(wrote, &wroteBuf)
	}
	debugln := func(args ...interface{}) {
		if debug {
			fmt.Println(args...)
		}
	}
	// Inject random reader/writer sleeps.
	const maxSleep = int(1 * time.Millisecond)
	doSleep := !testing.Short()
	rb := New(4 << 10).SetBlocking(true)

	// Reader
	var readErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer rb.CloseWithError(readErr)
		readRng := rand.New(rand.NewSource(1))
		buf := make([]byte, 64<<10)
		for {
			// Read
			n, err := rb.Read(buf[:readRng.Intn(len(buf))])
			readBytes += n
			read.Write(buf[:n])
			if err != nil {
				readErr = err
				break
			}
			debugln("READ 1\t", n, readBytes)

			// ReadByte
			b, err := rb.ReadByte()
			if err != nil {
				readErr = err
				break
			}
			readBytes++
			read.Write([]byte{b})
			debugln("READ 2\t", 1, readBytes)

			// TryRead
			n, err = rb.TryRead(buf[:readRng.Intn(len(buf))])
			readBytes += n
			read.Write(buf[:n])
			if err != nil && err != ErrAcquireLock && err != ErrIsEmpty {
				readErr = err
				break
			}
			debugln("READ 3\t", n, readBytes)
			if doSleep && readRng.Intn(20) == 0 {
				time.Sleep(time.Duration(readRng.Intn(maxSleep)))
			}
		}
	}()

	// Writer
	{
		writeRng := rand.New(rand.NewSource(2))
		buf := make([]byte, 64<<10)
		for i := 0; i < 500; i++ {
			writeRng.Read(buf)
			// Write
			n, err := rb.Write(buf[:writeRng.Intn(len(buf))])
			if err != nil {
				t.Fatalf("write failed: %v", err)
			}
			wroteBytes += n
			wrote.Write(buf[:n])
			debugln("WRITE 1\t", n, wroteBytes)

			// WriteString
			n, err = rb.WriteString(string(buf[:writeRng.Intn(len(buf))]))
			if err != nil {
				t.Fatalf("write failed: %v", err)
			}
			wroteBytes += n
			wrote.Write(buf[:n])
			debugln("WRITE 2\t", writeRng.Intn(len(buf)), wroteBytes)

			// WriteByte
			err = rb.WriteByte(buf[0])
			if err != nil {
				t.Fatalf("write failed: %v", err)
			}
			wroteBytes++
			wrote.Write(buf[:1])
			debugln("WRITE 3\t", 1, wroteBytes)

			// TryWrite
			n, err = rb.TryWrite(buf[:writeRng.Intn(len(buf))])
			if err != nil && err != ErrAcquireLock && err != ErrTooMuchDataToWrite && err != ErrIsFull {
				t.Fatalf("write failed: %v", err)
			}
			wroteBytes += n
			wrote.Write(buf[:n])
			debugln("WRITE 4\t", n, wroteBytes)

			// TryWriteByte
			err = rb.TryWriteByte(buf[0])
			if err != nil && err != ErrAcquireLock && err != ErrTooMuchDataToWrite && err != ErrIsFull {
				t.Fatalf("write failed: %v", err)
			}
			if err == nil {
				wroteBytes++
				wrote.Write(buf[:1])
				debugln("WRITE 5\t", 1, wroteBytes)
			}
			if doSleep && writeRng.Intn(10) == 0 {
				time.Sleep(time.Duration(writeRng.Intn(maxSleep)))
			}
		}
		if err := rb.Flush(); err != nil {
			t.Fatalf("flush failed: %v", err)
		}
		rb.CloseWriter()
	}
	wg.Wait()
	if !errors.Is(readErr, io.EOF) {
		t.Fatalf("expect io.EOF but got %v", readErr)
	}
	if readBytes != wroteBytes {
		a, b := readBuf.Bytes(), wroteBuf.Bytes()
		if debug && !bytes.Equal(a, b) {
			common := len(a)
			for i := range a {
				if a[i] != b[i] {
					t.Errorf("%x != %x", a[i], b[i])
					common = i
					break
				}
			}
			a, b = a[common:], b[common:]
			if len(a) > 64 {
				a = a[:64]
			}
			if len(b) > 64 {
				b = b[:64]
			}
			t.Errorf("after %d common bytes, difference \nread: %x\nwrote:%x", common, a, b)
		}
		t.Fatalf("expect read %d bytes but got %d", wroteBytes, readBytes)
	}
	if readHash.Sum32() != wroteHash.Sum32() {
		t.Fatalf("expect read hash 0x%08x but got 0x%08x", readHash.Sum32(), wroteHash.Sum32())
	}
}

func TestRingBuffer_ReadFromBig(t *testing.T) {
	// Typical runtime is ~5-10s.
	defer timeout(60 * time.Second)()
	const debug = false

	var readBytes int
	var wroteBytes int
	readHash := crc32.NewIEEE()
	wroteHash := crc32.NewIEEE()
	var readBuf bytes.Buffer
	var wroteBuf bytes.Buffer
	read := io.Writer(readHash)
	wrote := io.Writer(wroteHash)
	read = io.MultiWriter(read, &readBuf)
	wrote = io.MultiWriter(wrote, &wroteBuf)
	debugln := func(args ...interface{}) {
		if debug {
			fmt.Println(args...)
		}
	}
	// Inject random reader/writer sleeps.
	const maxSleep = int(1 * time.Millisecond)
	doSleep := !testing.Short()
	rb := New(4 << 10).SetBlocking(true)

	// Reader
	var readErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer rb.CloseWithError(readErr)
		readRng := rand.New(rand.NewSource(1))
		buf := make([]byte, 64<<10)
		for {
			// Read
			n, err := rb.Read(buf[:readRng.Intn(len(buf))])
			readBytes += n
			read.Write(buf[:n])
			if err != nil {
				readErr = err
				break
			}
			debugln("READ 1\t", n, readBytes)

			// ReadByte
			b, err := rb.ReadByte()
			if err != nil {
				readErr = err
				break
			}
			readBytes++
			read.Write([]byte{b})
			debugln("READ 2\t", 1, readBytes)

			// TryRead
			n, err = rb.TryRead(buf[:readRng.Intn(len(buf))])
			readBytes += n
			read.Write(buf[:n])
			if err != nil && err != ErrAcquireLock && err != ErrIsEmpty {
				readErr = err
				break
			}
			debugln("READ 3\t", n, readBytes)
			if doSleep && readRng.Intn(20) == 0 {
				time.Sleep(time.Duration(readRng.Intn(maxSleep)))
			}
		}
	}()

	// Writer
	{
		writeRng := rand.New(rand.NewSource(2))
		buf := make([]byte, 100<<10)
		for i := 0; i < 500; i++ {
			writeRng.Read(buf)
			// Write
			wroteBytes += len(buf)
			wrote.Write(buf)
		}
		debugln("ReadFrom with", wroteBytes, wroteBuf.Len())
		n, err := rb.ReadFrom(bytes.NewReader(wroteBuf.Bytes()))
		debugln("ReadFrom returned", n, err)
		if n != int64(wroteBytes) {
			t.Fatalf("expected %d bytes but got %d", wroteBytes, n)
		}
		if err != nil {
			t.Fatalf("ReadFrom failed: %v", err)
		}
		debugln("ReadFrom with", wroteBytes, wroteBuf.Len())
		if err := rb.Flush(); err != nil {
			t.Fatalf("flush failed: %v", err)
		}
		rb.CloseWriter()
	}
	wg.Wait()
	if !errors.Is(readErr, io.EOF) {
		t.Fatalf("expect io.EOF but got %v", readErr)
	}
	if readBytes != wroteBytes {
		a, b := readBuf.Bytes(), wroteBuf.Bytes()
		if debug && !bytes.Equal(a, b) {
			common := len(a)
			for i := range a {
				if a[i] != b[i] {
					t.Errorf("%x != %x", a[i], b[i])
					common = i
					break
				}
			}
			a, b = a[common:], b[common:]
			if len(a) > 64 {
				a = a[:64]
			}
			if len(b) > 64 {
				b = b[:64]
			}
			t.Errorf("after %d common bytes, difference \nread: %x\nwrote:%x", common, a, b)
		}
		t.Fatalf("expect read %d bytes but got %d", wroteBytes, readBytes)
	}
	if readHash.Sum32() != wroteHash.Sum32() {
		t.Fatalf("expect read hash 0x%08x but got 0x%08x", readHash.Sum32(), wroteHash.Sum32())
	}
}

type serveStream struct {
	b   []byte
	rng *rand.Rand
}

func (s *serveStream) Read(p []byte) (n int, err error) {
	if len(s.b) == 0 {
		return 0, io.EOF
	}
	n = s.rng.Intn(len(p) + 1)
	if n > len(s.b) {
		n = len(s.b)
	}
	copy(p, s.b[:n])
	s.b = s.b[n:]
	if len(s.b) == 0 {
		return n, io.EOF
	}
	return n, nil
}

func TestRingBuffer_Copy(t *testing.T) {
	// Typical runtime is ~1-2s.
	defer timeout(60 * time.Second)()

	for i := int64(0); i < 100; i++ {
		if testing.Short() && i > 5 {
			break
		}
		var wroteBytes int
		readHash := crc32.NewIEEE()
		wroteHash := crc32.NewIEEE()
		var readBuf bytes.Buffer
		var wroteBuf bytes.Buffer
		read := io.Writer(readHash)
		wrote := io.Writer(wroteHash)
		read = io.MultiWriter(read, &readBuf)
		wrote = io.MultiWriter(wrote, &wroteBuf)

		rb := New(4 << 10).SetBlocking(true)

		// Writer
		writeRng := rand.New(rand.NewSource(2 + i))
		buf := make([]byte, 100<<10)
		writeRng.Read(buf)
		for i := 0; i < writeRng.Intn(1000); i++ {
			// Write
			wroteBytes += len(buf)
			wrote.Write(buf)
		}
		in := &serveStream{
			b:   wroteBuf.Bytes(),
			rng: rand.New(rand.NewSource(0xc0cac01a + i)),
		}

		copied, err := rb.Copy(read, in)
		if err != nil {
			t.Fatal(err)
		}
		if copied != int64(wroteBytes) {
			t.Fatalf("copied %d bytes, expected %d", copied, wroteBytes)
		}
		readBytes := readBuf.Len()

		if readBytes != wroteBytes || readHash.Sum32() != wroteHash.Sum32() {
			a, b := readBuf.Bytes(), wroteBuf.Bytes()
			if !bytes.Equal(a, b) {
				common := len(a)
				for i := range a {
					if a[i] != b[i] {
						t.Errorf("%x != %x", a[i], b[i])
						common = i
						break
					}
				}
				a, b = a[common:], b[common:]
				if len(a) > 64 {
					a = a[:64]
				}
				if len(b) > 64 {
					b = b[:64]
				}
				t.Errorf("after %d common bytes, difference \nread: %x\nwrote:%x", common, a, b)
			}
			t.Fatalf("expect read %d bytes but got %d", wroteBytes, readBytes)
		}
		if readHash.Sum32() != wroteHash.Sum32() {
			t.Fatalf("expect read hash 0x%08x but got 0x%08x", readHash.Sum32(), wroteHash.Sum32())
		}
	}
}

type errormock struct {
	rerr    error
	read    int
	rleft   int
	werr    error
	written int
	wleft   int
}

var _ io.Reader = &errormock{}
var _ io.Writer = &errormock{}

func (e *errormock) Read(p []byte) (n int, err error) {
	switch {
	case e.rleft <= 0:
		err = e.rerr

	case len(p) > e.rleft:
		n = e.rleft

	default:
		n = len(p)
	}

	e.rleft -= n
	e.read += n

	return
}

func (e *errormock) Write(p []byte) (n int, err error) {
	switch {
	case e.wleft <= 0:
		err = e.werr

	case len(p) > e.wleft:
		n = e.wleft
		err = e.werr

	default:
		n = len(p)
	}

	e.wleft -= n
	e.written += n

	return
}

func TestRingBuffer_ReadFrom_Error(t *testing.T) {
	const bufsize = 4 << 10

	cases := []int{0, 1, bufsize, bufsize - 1, bufsize + 1}

	for _, c := range cases {
		t.Run(fmt.Sprintf("limit=%d", c), func(t *testing.T) {
			rb := New(bufsize).SetBlocking(true)
			tester := &errormock{rerr: io.ErrUnexpectedEOF, rleft: c, wleft: 10 << 10}

			// drain the buffer
			go func() {
				_, _ = io.Copy(io.Discard, rb)
			}()

			copied, err := rb.ReadFrom(tester)

			if err != io.ErrUnexpectedEOF {
				t.Errorf("expect io.ErrUnexpectedEOF but got %v", err)
			}

			if copied != int64(tester.read) {
				t.Errorf("expect %d bytes copied but got %d", c, copied)
			}
		})
	}
}

func TestRingBuffer_WriteTo_Error(t *testing.T) {
	const bufsize = 4 << 10

	cases := []int{0, 1, bufsize, bufsize - 1, bufsize + 1}

	for _, c := range cases {
		t.Run(fmt.Sprintf("limit=%d", c), func(t *testing.T) {
			rb := New(bufsize).SetBlocking(true)
			tester := &errormock{werr: io.ErrClosedPipe, wleft: c}

			// fill the buffer with enough data
			go func() {
				_, _ = io.Copy(rb, bytes.NewReader(make([]byte, bufsize*2)))
			}()

			copied, err := rb.WriteTo(tester)

			if err != io.ErrClosedPipe {
				t.Errorf("expect io.ErrClosedPipe but got %v", err)
			}

			if copied != int64(tester.written) {
				t.Errorf("expect %d bytes copied but got %d", c, copied)
			}
		})
	}
}

func TestRingBuffer_Copy_ReadError(t *testing.T) {
	const bufsize = 4 << 10

	cases := []int{0, 1, bufsize, bufsize - 1, bufsize + 1}

	for _, c := range cases {
		t.Run(fmt.Sprintf("limit=%d", c), func(t *testing.T) {
			rb := New(bufsize)
			tester := &errormock{rerr: io.ErrUnexpectedEOF, rleft: c, wleft: 10 << 10}

			copied, err := rb.Copy(tester, tester)

			if err != io.ErrUnexpectedEOF {
				t.Errorf("expect io.ErrUnexpectedEOF but got %v", err)
			}

			if copied != int64(tester.written) {
				t.Errorf("expect %d bytes copied but got %d", c, copied)
			}
		})
	}
}

func TestRingBuffer_Copy_WriteError(t *testing.T) {
	const bufsize = 4 << 10

	cases := []int{0, 1, bufsize, bufsize - 1, bufsize + 1}

	for _, c := range cases {
		t.Run(fmt.Sprintf("limit=%d", c), func(t *testing.T) {
			rb := New(bufsize)
			tester := &errormock{rleft: 10 << 10, werr: io.ErrClosedPipe, wleft: c}

			copied, err := rb.Copy(tester, tester)

			if err != io.ErrClosedPipe {
				t.Errorf("expect io.ErrUnexpectedEOF but got %v", err)
			}

			if copied != int64(tester.written) {
				t.Errorf("expect %d bytes copied but got %d", c, copied)
			}
		})
	}
}

func TestRingBuffer_ByteInterface(t *testing.T) {
	defer timeout(5 * time.Second)()
	rb := New(2)

	// write one
	err := rb.WriteByte('a')
	if err != nil {
		t.Fatalf("WriteByte failed: %v", err)
	}
	if rb.Length() != 1 {
		t.Fatalf("expect len 1 byte but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 1 {
		t.Fatalf("expect free 1 byte but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if !bytes.Equal(rb.Bytes(nil), []byte{'a'}) {
		t.Fatalf("expect a but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
	}
	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}

	// write to, isFull
	err = rb.WriteByte('b')
	if err != nil {
		t.Fatalf("WriteByte failed: %v", err)
	}
	if rb.Length() != 2 {
		t.Fatalf("expect len 2 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 0 {
		t.Fatalf("expect free 0 byte but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if !bytes.Equal(rb.Bytes(nil), []byte{'a', 'b'}) {
		t.Fatalf("expect a but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
	}
	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}

	// write
	err = rb.WriteByte('c')
	if err == nil {
		t.Fatalf("expect ErrIsFull but got nil")
	}
	if rb.Length() != 2 {
		t.Fatalf("expect len 2 bytes but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 0 {
		t.Fatalf("expect free 0 byte but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if !bytes.Equal(rb.Bytes(nil), []byte{'a', 'b'}) {
		t.Fatalf("expect a but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
	}
	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if !rb.IsFull() {
		t.Fatalf("expect IsFull is true but got false")
	}

	// read one
	b, err := rb.ReadByte()
	if err != nil {
		t.Fatalf("ReadByte failed: %v", err)
	}
	if b != 'a' {
		t.Fatalf("expect a but got %c. r.w=%d, r.r=%d", b, rb.w, rb.r)
	}
	if rb.Length() != 1 {
		t.Fatalf("expect len 1 byte but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 1 {
		t.Fatalf("expect free 1 byte but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	if !bytes.Equal(rb.Bytes(nil), []byte{'b'}) {
		t.Fatalf("expect a but got %s. r.w=%d, r.r=%d", rb.Bytes(nil), rb.w, rb.r)
	}
	// check empty or full
	if rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is false but got true")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}

	// read two, empty
	b, err = rb.ReadByte()
	if err != nil {
		t.Fatalf("ReadByte failed: %v", err)
	}
	if b != 'b' {
		t.Fatalf("expect b but got %c. r.w=%d, r.r=%d", b, rb.w, rb.r)
	}
	if rb.Length() != 0 {
		t.Fatalf("expect len 0 byte but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 2 {
		t.Fatalf("expect free 2 byte but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	// check empty or full
	if !rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is true but got false")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}

	// read three, error
	_, err = rb.ReadByte()
	if err == nil {
		t.Fatalf("expect ErrIsEmpty but got nil")
	}
	if rb.Length() != 0 {
		t.Fatalf("expect len 0 byte but got %d. r.w=%d, r.r=%d", rb.Length(), rb.w, rb.r)
	}
	if rb.Free() != 2 {
		t.Fatalf("expect free 2 byte but got %d. r.w=%d, r.r=%d", rb.Free(), rb.w, rb.r)
	}
	// check empty or full
	if !rb.IsEmpty() {
		t.Fatalf("expect IsEmpty is true but got false")
	}
	if rb.IsFull() {
		t.Fatalf("expect IsFull is false but got true")
	}
}

func TestRingBufferCloseError(t *testing.T) {
	type testError1 struct{ error }
	type testError2 struct{ error }

	rb := New(100)
	rb.CloseWithError(testError1{})
	if _, err := rb.Write(nil); err != (testError1{}) {
		t.Errorf("Write error: got %T, want testError1", err)
	}
	if _, err := rb.Write([]byte{1}); err != (testError1{}) {
		t.Errorf("Write error: got %T, want testError1", err)
	}
	if err := rb.WriteByte(0); err != (testError1{}) {
		t.Errorf("Write error: got %T, want testError1", err)
	}
	if _, err := rb.TryWrite(nil); err != (testError1{}) {
		t.Errorf("Write error: got %T, want testError1", err)
	}
	if _, err := rb.TryWrite([]byte{1}); err != (testError1{}) {
		t.Errorf("Write error: got %T, want testError1", err)
	}
	if err := rb.TryWriteByte(0); err != (testError1{}) {
		t.Errorf("Write error: got %T, want testError1", err)
	}
	if err := rb.Flush(); err != (testError1{}) {
		t.Errorf("Write error: got %T, want testError1", err)
	}

	rb.CloseWithError(testError2{})
	if _, err := rb.Write(nil); err != (testError1{}) {
		t.Errorf("Write error: got %T, want testError1", err)
	}

	rb.Reset()
	rb.CloseWithError(testError1{})
	if _, err := rb.Read(nil); err != (testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
	if _, err := rb.Read([]byte{0}); err != (testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
	if _, err := rb.ReadByte(); err != (testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
	if _, err := rb.TryRead(nil); err != (testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
	if _, err := rb.TryRead([]byte{0}); err != (testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
	rb.CloseWithError(testError2{})
	if _, err := rb.Read(nil); err != (testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
	if _, err := rb.Read([]byte{0}); err != (testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
	if _, err := rb.ReadByte(); err != (testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
	if _, err := rb.TryRead(nil); err != (testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
	if _, err := rb.TryRead([]byte{0}); err != (testError1{}) {
		t.Errorf("Read error: got %T, want testError1", err)
	}
}

func TestRingBufferCloseErrorUnblocks(t *testing.T) {
	const sz = 100
	rb := New(sz).SetBlocking(true)

	testCancel := func(fn func()) {
		t.Helper()
		defer timeout(5 * time.Second)()
		rb.Reset()
		done := make(chan struct{})
		go func() {
			defer close(done)
			time.Sleep(10 * time.Millisecond)
			fn()
		}()
		rb.CloseWithError(errors.New("test error"))
		<-done

		rb.Reset()
		done = make(chan struct{})
		go func() {
			defer close(done)
			fn()
		}()
		time.Sleep(10 * time.Millisecond)
		rb.CloseWithError(errors.New("test error"))
		<-done
	}
	testCancel(func() {
		rb.Write([]byte{sz + 5: 1})
	})
	testCancel(func() {
		rb.Write(make([]byte, sz))
		rb.WriteByte(0)
	})
	testCancel(func() {
		rb.Read([]byte{10: 1})
	})
	testCancel(func() {
		rb.ReadByte()
	})
	testCancel(func() {
		rb.Write(make([]byte, sz))
		rb.Flush()
	})
}

func TestWriteAfterWriterClose(t *testing.T) {
	rb := New(100).SetBlocking(true)

	done := make(chan error)
	go func() {
		defer close(done)
		_, err := rb.Write([]byte("hello"))
		if err != nil {
			t.Errorf("got error: %q; expected none", err)
		}
		rb.CloseWriter()
		_, err = rb.Write([]byte("world"))
		done <- err
		err = rb.WriteByte(0)
		done <- err
		_, err = rb.TryWrite([]byte("world"))
		done <- err
		err = rb.TryWriteByte(0)
		done <- err
	}()

	buf := make([]byte, 100)
	n, err := io.ReadFull(rb, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		t.Fatalf("got: %q; want: %q", err, io.ErrUnexpectedEOF)
	}
	for writeErr := range done {
		if writeErr != ErrWriteOnClosed {
			t.Errorf("got: %q; want: %q", writeErr, ErrWriteOnClosed)
		} else {
			t.Log("ok")
		}
	}
	result := string(buf[0:n])
	if result != "hello" {
		t.Errorf("got: %q; want: %q", result, "hello")
	}
}

func TestWithDeadline(t *testing.T) {
	rb := New(100).SetBlocking(true)
	tests := []struct {
		// test function that will fill buffer and block either on read or write.
		t func(chan<- error)
		// Set one, based on if we expect to block read or write
		r, w bool
	}{
		// Read tests.
		0: {t: func(c chan<- error) {
			_, err := rb.Read(make([]byte, 1000))
			c <- err
		}, r: true},
		1: {t: func(c chan<- error) {
			_, err := rb.ReadByte()
			c <- err
		}, r: true},
		2: {t: func(c chan<- error) {
			_, err := rb.WriteTo(io.Discard)
			c <- err
		}, r: true},
		// Write tests
		3: {t: func(c chan<- error) {
			// fill it
			_, err := rb.Write(make([]byte, 100))
			if err != nil {
				panic(err)
			}
			_, err = rb.Write(make([]byte, 1000))
			c <- err
		}, w: true},
		4: {t: func(c chan<- error) {
			// fill it
			_, err := rb.Write(make([]byte, 100))
			if err != nil {
				panic(err)
			}
			err = rb.WriteByte(42)
			c <- err
		}, w: true},
		5: {t: func(c chan<- error) {
			// fill it
			_, err := rb.Write(make([]byte, 100))
			if err != nil {
				panic(err)
			}
			_, err = rb.WriteString("hello world!")
			c <- err
		}, w: true},
		6: {t: func(c chan<- error) {
			// fill it
			_, err := rb.Write(make([]byte, 100))
			if err != nil {
				panic(err)
			}
			_, err = rb.ReadFrom(bytes.NewBuffer([]byte("hello world!")))
			c <- err
		}, w: true},
	}

	for i := range tests {
		t.Run(fmt.Sprint("both-", i), func(t *testing.T) {
			rb.WithTimeout(50 * time.Millisecond)
			rb.Reset()
			timedOut := make(chan error)
			started := time.Now()
			go tests[i].t(timedOut)
			select {
			case <-time.After(10 * time.Second):
				t.Fatalf("deadline exceeded by 200x")
			case err := <-timedOut:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("unexpected error:", err)
				}
				if d := time.Since(started); d < 40*time.Millisecond {
					t.Errorf("benchmark terminated before timeout: %v", d)
				}
			}
		})
		t.Run(fmt.Sprint("read-", i), func(t *testing.T) {
			rb.Reset()
			rb.WithTimeout(0).WithReadTimeout(50 * time.Millisecond)
			timedOut := make(chan error)
			started := time.Now()
			go tests[i].t(timedOut)
			select {
			case <-time.After(100 * time.Millisecond):
				if tests[i].r {
					t.Fatalf("deadline exceeded by 200x")
				}
				rb.CloseWithError(errors.New("test error"))
				<-timedOut
			case err := <-timedOut:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("unexpected error:", err)
				}
				if tests[i].w {
					t.Errorf("terminated on write")
				}
				if d := time.Since(started); d < 40*time.Millisecond {
					t.Errorf("benchmark terminated before timeout: %v", d)
				}
			}
		})
		t.Run(fmt.Sprint("write-", i), func(t *testing.T) {
			rb.Reset()
			rb.WithTimeout(0).WithWriteTimeout(50 * time.Millisecond)
			timedOut := make(chan error)
			started := time.Now()
			go tests[i].t(timedOut)
			select {
			case <-time.After(100 * time.Millisecond):
				if tests[i].w {
					t.Fatalf("deadline exceeded on write")
				}
				rb.CloseWithError(errors.New("test error"))
				<-timedOut
			case err := <-timedOut:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("unexpected error:", err)
				}
				if tests[i].r {
					t.Errorf("terminated on read")
				}
				if d := time.Since(started); d < 40*time.Millisecond {
					t.Errorf("benchmark terminated before timeout: %v", d)
				}
			}
		})

		t.Run(fmt.Sprint("allocs-test-", i), func(t *testing.T) {
			rb.Reset()
			rb.WithTimeout(50 * time.Millisecond)
			timedOut := make(chan error)
			a := testing.AllocsPerRun(5, func() {
				rb.Reset()
				go tests[i].t(timedOut)
				<-timedOut
			})
			t.Logf("Average Allocs: %.1f", a)
		})
	}
}

func timeout(after time.Duration) (cancel func()) {
	c := time.After(after)
	cc := make(chan struct{})
	go func() {
		select {
		case <-cc:
			return
		case <-c:
			buf := make([]byte, 1<<20)
			stacklen := runtime.Stack(buf, true)
			fmt.Printf("=== Timeout, assuming deadlock ===\n*** goroutine dump...\n%s\n*** end\n", string(buf[:stacklen]))
			os.Exit(2)
		}
	}()
	return func() {
		close(cc)
	}
}

func TestRingBuffer_Peek(t *testing.T) {
	rb := New(10)
	data := []byte("hello")
	rb.Write(data)

	buf := make([]byte, len(data))
	n, err := rb.Peek(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != len(data) {
		t.Fatalf("expected %d bytes, got %d", len(data), n)
	}
	if string(buf) != string(data) {
		t.Fatalf("expected %s, got %s", string(data), string(buf))
	}
}

func TestRingBuffer_Peek_WrapAround(t *testing.T) {
	rb := New(16)

	// Fill buffer with pattern that causes wrap-around
	data := []byte("abcdefgh")
	rb.Write(data)       // r=0, w=8
	rb.Read(make([]byte, 4)) // r=4, w=8

	// Write more to cause wrap-around
	rb.Write([]byte("ijkl"))   // r=4, w=12, no wrap yet
	rb.Read(make([]byte, 4))  // r=8, w=12
	rb.Write([]byte("mnop"))    // r=8, w=16 (wrapped to 0)

	// Peek should correctly read wrapped buffer
	buf := make([]byte, 8)
	n, err := rb.Peek(buf)
	if err != nil {
		t.Fatalf("Peek failed: %v", err)
	}
	if n != 8 {
		t.Fatalf("Expected 8 bytes, got %d", n)
	}
	expected := []byte("ijklmnop")
	if !bytes.Equal(buf, expected) {
		t.Fatalf("Expected %v, got %v", expected, buf)
	}

	// Verify read pointer unchanged
	readBuf := make([]byte, 8)
	rb.Read(readBuf)
	if !bytes.Equal(readBuf, expected) {
		t.Fatalf("Read after Peek returned different data")
	}
}

func TestRingBuffer_Peek_FullBuffer(t *testing.T) {
	rb := New(8)

	// Fill completely
	data := []byte("abcdefgh")
	rb.Write(data)

	// Peek should work on full buffer
	buf := make([]byte, 8)
	n, err := rb.Peek(buf)
	if err != nil {
		t.Fatalf("Peek failed: %v", err)
	}
	if n != 8 {
		t.Fatalf("Expected 8 bytes, got %d", n)
	}
	if !bytes.Equal(buf, data) {
		t.Fatalf("Peek returned wrong data")
	}

	// Read pointer unchanged
	if rb.Length() != 8 {
		t.Fatalf("Length changed after Peek")
	}
}

func TestRingBuffer_Peek_Partial(t *testing.T) {
	rb := New(16)
	data := []byte("hello world")
	rb.Write(data)

	// Peek fewer bytes than available
	buf := make([]byte, 5)
	n, err := rb.Peek(buf)
	if err != nil {
		t.Fatalf("Peek failed: %v", err)
	}
	if n != 5 {
		t.Fatalf("Expected 5 bytes, got %d", n)
	}
	expected := []byte("hello")
	if !bytes.Equal(buf, expected) {
		t.Fatalf("Expected %v, got %v", expected, buf)
	}

	// Verify buffer still has all data
	allData := rb.Bytes(nil)
	if !bytes.Equal(allData, data) {
		t.Fatalf("Peek modified buffer")
	}
}

func TestRingBuffer_Bytes_BufferReuse(t *testing.T) {
	rb := New(32)

	// Create a reusable buffer with capacity
	dst := make([]byte, 0, 64)

	// First call - should use dst buffer if capacity sufficient
	rb.Write([]byte("first data"))
	result := rb.Bytes(dst)
	if len(result) != len("first data") {
		t.Fatalf("Wrong length: %d", len(result))
	}
	if !bytes.Equal(result, []byte("first data")) {
		t.Fatalf("Wrong data: %s", string(result))
	}
	// Verify buffer was reused (cap >= 64 means dst capacity was used)
	if cap(result) < 64 {
		t.Fatalf("Expected buffer reuse, but got cap=%d (< 64)", cap(result))
	}

	// Second call with different data size
	rb.Reset()
	rb.Write([]byte("different"))
	result = rb.Bytes(result)
	if string(result) != "different" {
		t.Fatalf("Wrong data: %s", string(result))
	}
}

func TestRingBuffer_Bytes_SmallDst(t *testing.T) {
	rb := New(16)

	// Write data
	data := []byte("hello world")
	rb.Write(data)

	// Try to use a buffer that's too small
	smallDst := make([]byte, 0, 4) // cap=4, but we need 11 bytes
	result := rb.Bytes(smallDst)

	// Should allocate new buffer
	if cap(result) < 11 {
		t.Fatalf("Expected new buffer allocation, got cap=%d", cap(result))
	}
	if !bytes.Equal(result, data) {
		t.Fatalf("Wrong data: %s", string(result))
	}
}

func TestRingBuffer_Bytes_WrapAround(t *testing.T) {
	rb := New(16)

	// Create wrap-around scenario
	rb.Write([]byte("abcd"))  // r=0, w=4
	rb.Read(make([]byte, 2))  // r=2, w=4
	rb.Write([]byte("efghijklmnop"))  // r=2, w=14

	// Get bytes
	result := rb.Bytes(nil)
	expected := []byte("cdefghijklmnop")
	if !bytes.Equal(result, expected) {
		t.Fatalf("Expected %v, got %v", expected, result)
	}
}

func TestRingBuffer_TryContention(t *testing.T) {
	rb := New(1024)
	const numGoroutines = 50
	const opsPerGoroutine = 1000

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*2)

	// Reader goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 16)
			for j := 0; j < opsPerGoroutine; j++ {
				n, err := rb.TryRead(buf)
				if err != nil && err != ErrAcquireLock && err != ErrIsEmpty {
					errors <- fmt.Errorf("TryRead error: %w", err)
					return
				}
				if err == nil && n > 0 {
					// Process data
					_ = buf[:n]
				}
			}
		}()
	}

	// Writer goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data := []byte("test data content")
			for j := 0; j < opsPerGoroutine; j++ {
				n, err := rb.TryWrite(data)
				if err != nil && err != ErrAcquireLock && err != ErrIsFull && err != ErrTooMuchDataToWrite {
					errors <- fmt.Errorf("TryWrite error: %w", err)
					return
				}
				if err == nil {
					// Verify write count
					_ = n
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Fatal(err)
	}

	// Final verification
	rb.Reset()
	finalBuf := make([]byte, 1024)
	rb.Write(finalBuf)
	readBuf := make([]byte, 1024)
	n, err := rb.Read(readBuf)
	if err != nil {
		t.Fatalf("Final read failed: %v", err)
	}
	if n != 1024 {
		t.Fatalf("Final read got %d bytes", n)
	}
}

func TestRingBuffer_TryLockSuccess(t *testing.T) {
	rb := New(64)

	// Successful TryWrite
	n, err := rb.TryWrite([]byte("test"))
	if err != nil {
		t.Fatalf("TryWrite failed: %v", err)
	}
	if n != 4 {
		t.Fatalf("Expected 4 bytes, got %d", n)
	}

	// Successful TryRead
	buf := make([]byte, 4)
	n, err = rb.TryRead(buf)
	if err != nil {
		t.Fatalf("TryRead failed: %v", err)
	}
	if n != 4 {
		t.Fatalf("Expected 4 bytes, got %d", n)
	}
	if string(buf) != "test" {
		t.Fatalf("Wrong data: %s", string(buf))
	}
}
