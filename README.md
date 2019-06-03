# ringbuffer
A circular buffer (ring buffer) in Go, implement io.ReaderWriter interface


```go
	rb := NewRingBuffer(1024)
	rb.Write([]byte("abcd"))
	fmt.Println(rb.Length())
	fmt.Println(rb.Free())
	buf := make([]byte, 4)

	rb.Read(buf)
	fmt.Println(string(buf))
```