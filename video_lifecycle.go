package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

func validateFrameSize(width, height int, data []byte) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid frame dimensions %dx%d", width, height)
	}
	want := width * height * 4
	if len(data) != want {
		return fmt.Errorf("frame buffer size mismatch: got %d bytes, want %d", len(data), want)
	}
	return nil
}

func closeVideoDoneOnce(done chan struct{}, once *sync.Once) {
	if done == nil || once == nil {
		return
	}
	once.Do(func() {
		close(done)
	})
}

func waitForFirstVideoFrame(vsync <-chan struct{}, done <-chan struct{}, timeout time.Duration) error {
	if timeout <= 0 {
		select {
		case <-vsync:
			return nil
		case <-done:
			return errors.New("video output aborted before first frame")
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-vsync:
		return nil
	case <-done:
		return errors.New("video output aborted before first frame")
	case <-timer.C:
		return errors.New("video output timeout waiting for first frame")
	}
}

func drainVSync(vsync <-chan struct{}) {
	for {
		select {
		case <-vsync:
		default:
			return
		}
	}
}
