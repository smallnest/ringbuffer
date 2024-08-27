package ringbuffer

import (
	"fmt"
	"io"
	"log"
	"os"
)

func ExampleRingBuffer() {
	rb := New(1024)
	rb.Write([]byte("abcd"))
	fmt.Println(rb.Length())
	fmt.Println(rb.Free())
	buf := make([]byte, 4)

	rb.Read(buf)
	fmt.Println(string(buf))
	// Output: 4
	// 1020
	// abcd
}

func ExampleRingBuffer_Pipe() {
	// Create pipe from a 4KB ring buffer.
	r, w := New(4 << 10).Pipe()

	go func() {
		fmt.Fprint(w, "some io.Reader stream to be read\n")
		w.Close()
	}()

	if _, err := io.Copy(os.Stdout, r); err != nil {
		log.Fatal(err)
	}

	// Output:
	// some io.Reader stream to be read
}
