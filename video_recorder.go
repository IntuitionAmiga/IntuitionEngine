package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

const (
	recorderAudioRate   = SAMPLE_RATE
	recorderAudioSecs   = 2
	recorderSignalDepth = 1
)

type sampleRing struct {
	buf  []float32
	mask uint32
	read atomic.Uint32
	writ atomic.Uint32
}

func newSampleRing(capacity int) *sampleRing {
	n := 1
	for n < capacity {
		n <<= 1
	}
	return &sampleRing{buf: make([]float32, n), mask: uint32(n - 1)}
}

func (r *sampleRing) push(v float32) {
	w := r.writ.Load()
	rd := r.read.Load()
	next := w + 1
	if next-rd > uint32(len(r.buf)) {
		r.read.Store(rd + 1)
	}
	r.buf[w&r.mask] = v
	r.writ.Store(next)
}

func (r *sampleRing) pop() (float32, bool) {
	rd := r.read.Load()
	w := r.writ.Load()
	if rd == w {
		return 0, false
	}
	v := r.buf[rd&r.mask]
	r.read.Store(rd + 1)
	return v, true
}

func (r *sampleRing) available() uint32 {
	w := r.writ.Load()
	rd := r.read.Load()
	return w - rd
}

// VideoRecorder captures compositor frames and sound samples to FFmpeg.
type VideoRecorder struct {
	compositor *VideoCompositor
	sound      *SoundChip

	running    atomic.Bool
	frameCount atomic.Uint64

	mu      sync.Mutex
	lastErr error

	cmd       *exec.Cmd
	videoIn   io.WriteCloser
	audioW    *os.File
	audioR    *os.File
	stopCh    chan struct{}
	doneCh    chan struct{}
	waitDone  chan struct{}
	frameCh   chan struct{}
	sampleTap func(float32)
	ring      *sampleRing

	screenBufs     [3][]byte
	screenWriteIdx int
	screenReadIdx  int
	screenShared   atomic.Int32
	screenFrameCh  chan struct{}
	useScreen      atomic.Bool

	width  int
	height int
	fps    int
	accNum int
}

func NewVideoRecorder(compositor *VideoCompositor) *VideoRecorder {
	return &VideoRecorder{compositor: compositor}
}

func (r *VideoRecorder) SetSoundChip(sound *SoundChip) {
	r.mu.Lock()
	r.sound = sound
	r.mu.Unlock()
}

func (r *VideoRecorder) Start(path string) error {
	if path == "" {
		return fmt.Errorf("recording path is required")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found in PATH")
	}
	if r.compositor == nil {
		return fmt.Errorf("compositor unavailable")
	}
	if r.running.Load() {
		return fmt.Errorf("recording already running")
	}

	w, h := r.compositor.GetDimensions()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("invalid compositor dimensions")
	}
	fps := r.compositor.GetRefreshRate()
	if fps <= 0 {
		fps = COMPOSITOR_REFRESH_RATE
	}

	r.compositor.LockResolution(w, h)

	audioR, audioW, err := os.Pipe()
	if err != nil {
		r.compositor.UnlockResolution()
		return err
	}

	cmd := exec.Command(
		"ffmpeg",
		"-y",
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-s", fmt.Sprintf("%dx%d", w, h),
		"-r", fmt.Sprintf("%d", fps),
		"-i", "pipe:0",
		"-f", "s16le",
		"-ar", fmt.Sprintf("%d", recorderAudioRate),
		"-ac", "1",
		"-i", "pipe:3",
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-shortest",
		path,
	)
	cmd.ExtraFiles = []*os.File{audioR}

	videoIn, err := cmd.StdinPipe()
	if err != nil {
		_ = audioR.Close()
		_ = audioW.Close()
		r.compositor.UnlockResolution()
		return err
	}

	if err := cmd.Start(); err != nil {
		_ = videoIn.Close()
		_ = audioR.Close()
		_ = audioW.Close()
		r.compositor.UnlockResolution()
		return err
	}

	ring := newSampleRing(recorderAudioRate * recorderAudioSecs)
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	waitDone := make(chan struct{})
	frameCh := make(chan struct{}, recorderSignalDepth)
	screenFrameCh := make(chan struct{}, 1)

	// Pre-allocate screen capture buffers for triple-buffering
	bufSize := w * h * 4
	screenBufs := [3][]byte{make([]byte, bufSize), make([]byte, bufSize), make([]byte, bufSize)}

	r.mu.Lock()
	r.lastErr = nil
	r.cmd = cmd
	r.videoIn = videoIn
	r.audioR = audioR
	r.audioW = audioW
	r.stopCh = stopCh
	r.doneCh = doneCh
	r.waitDone = waitDone
	r.frameCh = frameCh
	r.screenFrameCh = screenFrameCh
	r.screenBufs = screenBufs
	r.screenWriteIdx = 0
	r.screenShared.Store(1)
	r.screenReadIdx = 2
	r.ring = ring
	r.width = w
	r.height = h
	r.fps = fps
	r.accNum = 0
	sound := r.sound
	tap := func(s float32) { ring.push(s) }
	r.sampleTap = tap
	r.mu.Unlock()

	if sound != nil {
		sound.SetSampleTap(tap)
	}

	r.frameCount.Store(0)
	r.running.Store(true)

	go r.waitProc(cmd, waitDone)
	go r.loop(stopCh, frameCh, screenFrameCh, doneCh)
	return nil
}

func (r *VideoRecorder) waitProc(cmd *exec.Cmd, waitDone chan struct{}) {
	err := cmd.Wait()
	r.running.Store(false)
	r.mu.Lock()
	if err != nil && r.lastErr == nil {
		r.lastErr = err
	}
	r.mu.Unlock()
	close(waitDone)
}

func (r *VideoRecorder) loop(stopCh <-chan struct{}, frameCh <-chan struct{}, screenFrameCh <-chan struct{}, doneCh chan struct{}) {
	defer close(doneCh)
	// Branch on mode at start: screen-capture or compositor
	if r.useScreen.Load() {
		// Screen-capture mode: only screenFrameCh drives writes
		for {
			select {
			case <-stopCh:
				return
			case <-screenFrameCh:
				if !r.running.Load() {
					return
				}
				// Swap readIdx with shared to get latest frame
				r.screenReadIdx = int(r.screenShared.Swap(int32(r.screenReadIdx)))
				r.writeFrameData(r.screenBufs[r.screenReadIdx])
			}
		}
	}
	// Compositor mode: only frameCh drives writes
	for {
		select {
		case <-stopCh:
			return
		case <-frameCh:
			if !r.running.Load() {
				return
			}
			r.writeFrame()
		}
	}
}

func (r *VideoRecorder) writeFrame() {
	frame := r.compositor.GetCurrentFrame()
	r.mu.Lock()
	w, h := r.width, r.height
	r.mu.Unlock()
	if len(frame) < w*h*4 {
		frame = make([]byte, w*h*4)
	}
	r.writeFrameData(frame[:w*h*4])
}

// writeFrameData writes one video frame and its corresponding audio samples to ffmpeg.
// Used by both compositor mode (writeFrame) and screen-capture mode (loop).
func (r *VideoRecorder) writeFrameData(pixels []byte) {
	r.mu.Lock()
	videoIn := r.videoIn
	audioW := r.audioW
	ring := r.ring
	fps := r.fps
	sound := r.sound
	r.accNum += recorderAudioRate
	targetSamples := r.accNum / fps
	r.accNum -= targetSamples * fps
	r.mu.Unlock()

	if videoIn == nil || audioW == nil || ring == nil || targetSamples < 0 {
		return
	}
	if sound != nil && ring.available() < uint32(targetSamples) {
		return
	}

	if _, err := videoIn.Write(pixels); err != nil {
		if r.running.Load() {
			r.mu.Lock()
			if r.lastErr == nil {
				r.lastErr = err
			}
			r.mu.Unlock()
		}
		r.running.Store(false)
		return
	}

	if targetSamples == 0 {
		r.frameCount.Add(1)
		return
	}

	buf := make([]byte, targetSamples*2)
	for i := range targetSamples {
		s, ok := ring.pop()
		if !ok {
			s = 0
		}
		if s > 1 {
			s = 1
		} else if s < -1 {
			s = -1
		}
		iv := int16(math.Round(float64(s) * 32767))
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(iv))
	}
	if _, err := audioW.Write(buf); err != nil {
		if r.running.Load() {
			r.mu.Lock()
			if r.lastErr == nil {
				r.lastErr = err
			}
			r.mu.Unlock()
		}
		r.running.Store(false)
		return
	}

	r.frameCount.Add(1)
}

func (r *VideoRecorder) Stop() error {
	r.running.Store(false)
	r.useScreen.Store(false)

	r.mu.Lock()
	if r.cmd == nil {
		err := r.lastErr
		r.mu.Unlock()
		return err
	}
	stopCh := r.stopCh
	doneCh := r.doneCh
	frameCh := r.frameCh
	screenFrameCh := r.screenFrameCh
	videoIn := r.videoIn
	audioW := r.audioW
	audioR := r.audioR
	waitDone := r.waitDone
	cmd := r.cmd
	sound := r.sound
	r.cmd = nil
	r.stopCh = nil
	r.doneCh = nil
	r.frameCh = nil
	r.screenFrameCh = nil
	r.videoIn = nil
	r.audioW = nil
	r.audioR = nil
	r.ring = nil
	r.sampleTap = nil
	r.mu.Unlock()

	_ = screenFrameCh // nilled on struct; loop exits via stopCh

	if stopCh != nil {
		close(stopCh)
	}
	_ = frameCh

	if sound != nil {
		sound.ClearSampleTap()
	}
	if videoIn != nil {
		_ = videoIn.Close()
	}
	if audioW != nil {
		_ = audioW.Close()
	}
	if audioR != nil {
		_ = audioR.Close()
	}
	if doneCh != nil {
		<-doneCh
	}
	if waitDone != nil {
		select {
		case <-waitDone:
		case <-time.After(2 * time.Second):
			if cmd != nil && cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			select {
			case <-waitDone:
			case <-time.After(2 * time.Second):
				r.mu.Lock()
				if r.lastErr == nil {
					r.lastErr = fmt.Errorf("ffmpeg did not exit after stop timeout")
				}
				r.mu.Unlock()
			}
		}
	}

	r.compositor.UnlockResolution()

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastErr
}

func (r *VideoRecorder) IsRecording() bool {
	return r.running.Load()
}

func (r *VideoRecorder) FrameCount() uint64 {
	return r.frameCount.Load()
}

func (r *VideoRecorder) OnFrame() {
	if !r.running.Load() {
		return
	}
	r.mu.Lock()
	frameCh := r.frameCh
	r.mu.Unlock()
	if frameCh == nil {
		return
	}
	select {
	case frameCh <- struct{}{}:
	default:
	}
}

func (r *VideoRecorder) PushScreenFrame(pixels []byte) {
	if !r.running.Load() || !r.useScreen.Load() {
		return
	}
	copy(r.screenBufs[r.screenWriteIdx], pixels)
	// Swap write buffer with shared slot (give completed frame, get recycled buffer)
	r.screenWriteIdx = int(r.screenShared.Swap(int32(r.screenWriteIdx)))
	r.mu.Lock()
	ch := r.screenFrameCh
	r.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default: // signal already pending; consumer will get the latest frame on wake
	}
}

func (r *VideoRecorder) IsRecordingScreen() bool {
	return r.running.Load() && r.useScreen.Load()
}

func (r *VideoRecorder) StartScreen(path string) error {
	if path == "" {
		return fmt.Errorf("recording path is required")
	}
	if r.running.Load() {
		return fmt.Errorf("recording already running")
	}
	r.useScreen.Store(true)
	if err := r.Start(path); err != nil {
		r.useScreen.Store(false)
		return err
	}
	return nil
}
