package main

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

type closeTrackingWriteCloser struct {
	closed bool
}

func (w *closeTrackingWriteCloser) Write(p []byte) (int, error) {
	return len(p), nil
}

func (w *closeTrackingWriteCloser) Close() error {
	w.closed = true
	return nil
}

type blockingWriteCloser struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func newBlockingWriteCloser() *blockingWriteCloser {
	return &blockingWriteCloser{
		started: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (w *blockingWriteCloser) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.closed
	return 0, errors.New("closed")
}

func (w *blockingWriteCloser) Close() error {
	select {
	case <-w.closed:
	default:
		close(w.closed)
	}
	return nil
}

func TestVideoRecorder_StopPreservesErrorAfterFramesWritten(t *testing.T) {
	comp := NewVideoCompositor(nil)
	rec := NewVideoRecorder(comp)
	wantErr := errors.New("encoder failed after frames")
	doneCh := make(chan struct{})
	close(doneCh)
	waitDone := make(chan struct{})
	close(waitDone)

	rec.mu.Lock()
	rec.cmd = &exec.Cmd{}
	rec.stopCh = make(chan struct{})
	rec.doneCh = doneCh
	rec.waitDone = waitDone
	rec.lastErr = wantErr
	rec.width = 1
	rec.height = 1
	rec.fps = 60
	rec.mu.Unlock()
	rec.frameCount.Store(1)

	if gotErr := rec.Stop(); !errors.Is(gotErr, wantErr) {
		t.Fatalf("Stop error = %v, want %v", gotErr, wantErr)
	}
}

func TestVideoRecorder_StartupFailureCleanupAllowsStop(t *testing.T) {
	comp := NewVideoCompositor(nil)
	rec := NewVideoRecorder(comp)
	wantErr := errors.New("startup write failed")
	videoIn := &closeTrackingWriteCloser{}
	audioR, audioW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	doneCh := make(chan struct{})
	waitDone := make(chan struct{})

	comp.LockResolution(2, 2)
	rec.running.Store(false)
	rec.mu.Lock()
	rec.cmd = &exec.Cmd{}
	rec.videoIn = videoIn
	rec.audioR = audioR
	rec.audioW = audioW
	rec.stopCh = make(chan struct{})
	rec.doneCh = doneCh
	rec.waitDone = waitDone
	rec.frameCh = make(chan struct{}, 1)
	rec.screenFrameCh = make(chan struct{}, 1)
	rec.sampleTap = func(float32) {}
	rec.ring = newSampleRing(8)
	rec.lastErr = wantErr
	rec.mu.Unlock()

	if gotErr := rec.cleanupStartupFailure(rec.cmd, videoIn, audioW, audioR); !errors.Is(gotErr, wantErr) {
		t.Fatalf("cleanupStartupFailure error = %v, want %v", gotErr, wantErr)
	}
	if !videoIn.closed {
		t.Fatal("startup failure cleanup did not close video pipe")
	}

	stopped := make(chan error, 1)
	go func() {
		stopped <- rec.Stop()
	}()
	select {
	case gotErr := <-stopped:
		if !errors.Is(gotErr, wantErr) {
			t.Fatalf("Stop error = %v, want %v", gotErr, wantErr)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Stop blocked after startup failure cleanup")
	}
}

func TestVideoRecorder_StopClosesPipesBeforeWaitingForLoop(t *testing.T) {
	comp := NewVideoCompositor(nil)
	rec := NewVideoRecorder(comp)
	videoIn := newBlockingWriteCloser()
	audioR, audioW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stopCh := make(chan struct{})
	frameCh := make(chan struct{}, 1)
	screenFrameCh := make(chan struct{}, 1)
	doneCh := make(chan struct{})
	waitDone := make(chan struct{})
	close(waitDone)

	comp.LockResolution(2, 2)
	rec.running.Store(true)
	rec.mu.Lock()
	rec.cmd = &exec.Cmd{}
	rec.videoIn = videoIn
	rec.audioR = audioR
	rec.audioW = audioW
	rec.stopCh = stopCh
	rec.doneCh = doneCh
	rec.waitDone = waitDone
	rec.frameCh = frameCh
	rec.screenFrameCh = screenFrameCh
	rec.ring = newSampleRing(8)
	rec.width = 2
	rec.height = 2
	rec.fps = 60
	rec.mu.Unlock()

	go rec.loop(stopCh, frameCh, screenFrameCh, doneCh)
	frameCh <- struct{}{}
	select {
	case <-videoIn.started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("recorder loop did not enter blocking video write")
	}

	stopped := make(chan error, 1)
	go func() {
		stopped <- rec.Stop()
	}()
	select {
	case gotErr := <-stopped:
		if gotErr != nil {
			t.Fatalf("Stop error = %v, want nil", gotErr)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Stop blocked waiting for loop before closing encoder pipes")
	}
}
