package ringbuffer

import (
	"testing"
	"time"
)

func TestMutex(t *testing.T) {
	var mu Mutex
	go func() {
		mu.Lock()
		time.Sleep(200 * time.Millisecond)
		mu.Unlock()
	}()

	time.Sleep(50 * time.Millisecond)

	ok := mu.TryLock()
	if ok {
		t.Fatalf("should not accquired the lock")
	}

	time.Sleep(200 * time.Millisecond)

	ok = mu.TryLock()
	if !ok {
		t.Fatalf("should accquired the lock")
	}
	mu.Unlock()
}
