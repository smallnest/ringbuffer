package ringbuffer

import (
	"context"
	"io"
	"strings"
	"testing"
)

func BenchmarkRingBuffer_Sync(b *testing.B) {
	rb := New(1024)
	data := []byte(strings.Repeat("a", 512))
	buf := make([]byte, 512)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rb.Write(data) //nolint errcheck
		_, _ = rb.Read(buf)   //nolint errcheck
	}
}

func BenchmarkRingBuffer_AsyncRead(b *testing.B) {
	// Pretty useless benchmark, but it's here for completeness.
	rb := New(1024)
	data := []byte(strings.Repeat("a", 512))
	buf := make([]byte, 512)

	go func() {
		for {
			_, _ = rb.Read(buf) //nolint errcheck
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rb.Write(data) //nolint errcheck
	}
}

func BenchmarkRingBuffer_AsyncReadBlocking(b *testing.B) {
	const sz = 512
	const buffers = 10
	rb := New(sz * buffers)
	rb.SetBlocking(true)
	data := []byte(strings.Repeat("a", sz))
	buf := make([]byte, sz)

	go func() {
		for {
			_, _ = rb.Read(buf) //nolint errcheck
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rb.Write(data) //nolint errcheck
	}
}

func BenchmarkRingBuffer_AsyncWrite(b *testing.B) {
	rb := New(1024)
	data := []byte(strings.Repeat("a", 512))
	buf := make([]byte, 512)

	go func() {
		for {
			_, _ = rb.Write(data) //nolint errcheck
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rb.Read(buf) //nolint errcheck
	}
}

func BenchmarkRingBuffer_AsyncWriteBlocking(b *testing.B) {
	const sz = 512
	const buffers = 10
	rb := New(sz * buffers)
	rb.SetBlocking(true)
	data := []byte(strings.Repeat("a", sz))
	buf := make([]byte, sz)

	go func() {
		for {
			_, _ = rb.Write(data) //nolint errcheck
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rb.Read(buf) //nolint errcheck
	}
}

type repeatReader struct {
	b      []byte
	doCopy bool // Actually copy data...
}

func (r repeatReader) Read(b []byte) (n int, err error) {
	n = len(b)
	for r.doCopy && len(b) > 0 {
		n2 := copy(b, r.b)
		b = b[n2:]
	}
	return n, nil
}

func BenchmarkRingBuffer_ReadFrom(b *testing.B) {
	const sz = 512
	const buffers = 10
	rb := New(sz * buffers)
	rb.SetBlocking(true)
	data := []byte(strings.Repeat("a", sz))
	buf := make([]byte, sz)

	go func() {
		_, _ = rb.ReadFrom(repeatReader{b: data}) //nolint errcheck
	}()

	b.ResetTimer()
	b.SetBytes(sz)
	for i := 0; i < b.N; i++ {
		_, _ = io.ReadFull(rb, buf) //nolint errcheck
	}
	rb.CloseWithError(context.Canceled) //nolint errcheck
}

func BenchmarkRingBuffer_WriteTo(b *testing.B) {
	const sz = 512
	const buffers = 10
	rb := New(sz * buffers)
	rb.SetBlocking(true)
	data := []byte(strings.Repeat("a", sz))

	go func() {
		_, _ = rb.WriteTo(io.Discard) //nolint errcheck
	}()

	b.ResetTimer()
	b.SetBytes(sz)
	for i := 0; i < b.N; i++ {
		_, err := rb.Write(data)
		if err != nil {
			b.Fatal(err)
		}
	}
	rb.CloseWithError(context.Canceled) //nolint errcheck
}

func BenchmarkIoPipeReader(b *testing.B) {
	pr, pw := io.Pipe()
	data := []byte(strings.Repeat("a", 512))
	buf := make([]byte, 512)

	go func() {
		for {
			_, _ = pw.Write(data) //nolint errcheck
		}
	}()

	b.ResetTimer()
	b.SetBytes(int64(len(data)))
	for i := 0; i < b.N; i++ {
		_, _ = pr.Read(buf) //nolint errcheck
	}
}
