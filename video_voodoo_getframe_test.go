package main

import (
	"testing"
	"time"
)

// GetFrame must never block on an in-flight swap job: the compositor
// calls it while holding the compositor lock, which the windowed UI's
// input update also needs. A long-running software flush (no-clear
// composite frames replay whole triangle batches per flush) would
// otherwise freeze input and present a black window for the duration.
// Scanout semantics: a busy swap means the previous published frame is
// returned, stale but immediate.
func TestVoodooGetFrame_NonBlockingWhileSwapInFlight(t *testing.T) {
	v, err := NewVoodooEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	v.enabled.Store(true)

	// Simulate a long-running swap job.
	v.mu.Lock()
	v.jobsInFlight = 1
	v.mu.Unlock()
	v.swapInFlight.Store(true)

	done := make(chan []byte, 1)
	go func() {
		done <- v.GetFrame()
	}()

	select {
	case frame := <-done:
		if frame == nil {
			t.Fatal("GetFrame returned nil for an enabled engine")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GetFrame blocked on an in-flight swap job")
	}
}
