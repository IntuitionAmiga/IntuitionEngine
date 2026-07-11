package main

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
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

// TestVideoRecorder_AudioStarvationDoesNotStallRecording pins the audio
// pump's grace window: a tap that stops delivering (headless hosts,
// suspended sinks) must only hold the audio stream for the grace period and
// then write silence at the real sample rate, and video writes must never
// depend on audio availability at all. The historical single-loop design
// gated video on the ring and later deadlocked against ffmpeg's muxer
// interleaving; every recording froze at exactly one 64 KiB audio pipe
// buffer (44 frames).
func TestVideoRecorder_AudioStarvationDoesNotStallRecording(t *testing.T) {
	comp := NewVideoCompositor(nil)
	rec := NewVideoRecorder(comp)

	// Video path: writes must flow with an empty, starved ring.
	rec.mu.Lock()
	rec.videoIn = &closeTrackingWriteCloser{}
	rec.ring = newSampleRing(recorderAudioRate * recorderAudioSecs)
	rec.sound = &SoundChip{}
	rec.width = 4
	rec.height = 4
	rec.fps = 60
	rec.mu.Unlock()
	rec.running.Store(true)

	pixels := make([]byte, 4*4*4)
	for i := 0; i < 120; i++ {
		rec.writeFrameData(pixels)
	}
	if got := rec.FrameCount(); got != 120 {
		t.Fatalf("video frames written with starved audio = %d, want 120", got)
	}

	// Audio pump: with an empty ring it must hold through the grace window,
	// then deliver silence at the sample rate rather than nothing.
	audioR, audioW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer audioR.Close()

	var consumed atomic.Int64
	go func() {
		buf := make([]byte, 65536)
		for {
			n, err := audioR.Read(buf)
			consumed.Add(int64(n))
			if err != nil {
				return
			}
		}
	}()

	stopCh := make(chan struct{})
	pumpDone := make(chan struct{})
	rec.mu.Lock()
	ring := rec.ring
	rec.mu.Unlock()
	go rec.audioPump(stopCh, audioW, ring, true, pumpDone)

	graceDeadline := time.Duration(recorderAudioGraceTicks+2) * recorderAudioPumpTick
	time.Sleep(graceDeadline / 2)
	if got := consumed.Load(); got != 0 {
		t.Fatalf("audio written inside grace window = %d bytes, want 0", got)
	}
	deadline := time.Now().Add(graceDeadline + 2*time.Second)
	for consumed.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if consumed.Load() == 0 {
		t.Fatal("audio pump never wrote silence after the grace window; recording would stall")
	}

	close(stopCh)
	<-pumpDone
	_ = audioW.Close()
}

func TestSampleRingDiscardDropsOldestSamples(t *testing.T) {
	ring := newSampleRing(8)
	for i := 0; i < 6; i++ {
		ring.push(float32(i))
	}

	ring.discard(4, 0)

	if got := ring.available(); got != 2 {
		t.Fatalf("available after discard = %d, want 2", got)
	}
	for _, want := range []float32{4, 5} {
		got, ok := ring.pop()
		if !ok || got != want {
			t.Fatalf("pop after discard = (%v, %v), want (%v, true)", got, ok, want)
		}
	}
}

func TestSampleRingDiscardPreservesNewestBatch(t *testing.T) {
	ring := newSampleRing(8)
	for i := 0; i < 8; i++ {
		ring.push(float32(i))
	}

	ring.discard(100, 3)

	if got := ring.available(); got != 3 {
		t.Fatalf("available after oversized discard = %d, want 3", got)
	}
	for _, want := range []float32{5, 6, 7} {
		got, ok := ring.pop()
		if !ok || got != want {
			t.Fatalf("pop after oversized discard = (%v, %v), want (%v, true)", got, ok, want)
		}
	}
}

// TestSampleRingCursorProtocolUnderContention pins the read-cursor CAS
// protocol: the producer's overflow drop, the consumer's pop and the
// consumer's discard all move the same cursor, and an unconditional store
// from a stale load can rewind it, resurrecting slots the producer is about
// to overwrite. The test hammers a full ring from both sides and checks the
// arithmetic invariants that a rewound cursor breaks; run with -race it
// also catches any non-atomic access.
func TestSampleRingCursorProtocolUnderContention(t *testing.T) {
	ring := newSampleRing(64)
	capacity := uint32(len(ring.buf))
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 200_000; i++ {
			ring.push(float32(i))
		}
	}()

	violations := 0
	for alive := true; alive; {
		select {
		case <-done:
			alive = false
		default:
		}
		ring.pop()
		ring.discard(3, 8)
		// Consistent snapshot: the invariant only holds for a write cursor
		// sampled while the read cursor is stable, since the producer moves
		// both. A cursor pair straddling a concurrent advance is not
		// evidence of a rewind.
		rd1 := ring.read.Load()
		w := ring.writ.Load()
		rd2 := ring.read.Load()
		if rd1 == rd2 {
			if avail := w - rd1; avail > capacity {
				violations++
			}
		}
	}
	if violations > 0 {
		t.Fatalf("read cursor rewound behind the producer %d time(s): available exceeded capacity", violations)
	}
}
