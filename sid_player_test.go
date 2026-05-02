package main

import (
	"runtime"
	"testing"
	"time"
)

func TestSIDPlayerAsyncStartUsesSingleBoundedWorker(t *testing.T) {
	player := NewSIDPlayer(NewSIDEngine(nil, 44100))
	baseline := runtime.NumGoroutine()

	for i := 0; i < 1000; i++ {
		player.enqueueStart(sidAsyncStartRequest{
			gen:  uint64(i + 1),
			data: []byte("bad sid data"),
		})
	}

	time.Sleep(50 * time.Millisecond)
	got := runtime.NumGoroutine()
	if got > baseline+4 {
		t.Fatalf("goroutines grew from %d to %d; want bounded worker behavior", baseline, got)
	}
}
