package main

import (
	"sync"
	"testing"
)

func TestVideoLifecycle_StartAbortSelect(t *testing.T) {
	done := make(chan struct{})
	close(done)
	vsync := make(chan struct{}, 1)
	if err := waitForFirstVideoFrame(vsync, done, 0); err == nil {
		t.Fatal("expected abort error when done is already closed")
	}
}

func TestVideoLifecycle_DoneCloseOnce(t *testing.T) {
	done := make(chan struct{})
	var once sync.Once
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			closeVideoDoneOnce(done, &once)
		}()
	}
	wg.Wait()
	select {
	case <-done:
	default:
		t.Fatal("done channel was not closed")
	}
}

func TestVideoLifecycle_ValidateFrameSize(t *testing.T) {
	if err := validateFrameSize(320, 200, make([]byte, 320*200*4)); err != nil {
		t.Fatalf("valid frame rejected: %v", err)
	}
	if err := validateFrameSize(320, 200, make([]byte, 320*200*4-1)); err == nil {
		t.Fatal("short frame was accepted")
	}
	if err := validateFrameSize(320, 200, make([]byte, 320*200*4+1)); err == nil {
		t.Fatal("long frame was accepted")
	}
}
