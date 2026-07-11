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

	// recorderAudioGraceTicks is how many consecutive pump ticks may pass
	// with the sample ring short before the audio stream is treated as
	// absent rather than lagging, and silence is written in its place.
	// Half a second at the pump cadence.
	recorderAudioGraceTicks = 25

	// recorderAudioPumpTick is the audio pump cadence.
	recorderAudioPumpTick = 20 * time.Millisecond
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

	running      atomic.Bool
	frameCount   atomic.Uint64
	frameSignals atomic.Uint64

	mu      sync.Mutex
	lastErr error

	cmd       *exec.Cmd
	videoIn   io.WriteCloser
	audioW    *os.File
	audioR    *os.File
	stopCh    chan struct{}
	doneCh    chan struct{}
	pumpDone  chan struct{}
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
		"-preset", "ultrafast",
		"-crf", "20",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
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
	pumpDone := make(chan struct{})
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
	r.pumpDone = pumpDone
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
	sound := r.sound
	tap := func(s float32) { ring.push(s) }
	r.sampleTap = tap
	r.mu.Unlock()

	if sound != nil {
		sound.SetSampleTap(tap)
	}

	r.frameCount.Store(0)
	r.frameSignals.Store(0)
	r.running.Store(true)
	r.writeFrameData(make([]byte, w*h*4))
	r.writeFrameData(make([]byte, w*h*4))
	if !r.running.Load() {
		return r.cleanupStartupFailure(cmd, videoIn, audioW, audioR)
	}

	go r.waitProc(cmd, waitDone)
	go r.loop(stopCh, frameCh, screenFrameCh, doneCh)
	go r.audioPump(stopCh, audioW, ring, sound != nil, pumpDone)
	return nil
}

func (r *VideoRecorder) cleanupStartupFailure(cmd *exec.Cmd, videoIn io.WriteCloser, audioW, audioR *os.File) error {
	r.running.Store(false)

	r.mu.Lock()
	err := r.lastErr
	sound := r.sound
	r.cmd = nil
	r.videoIn = nil
	r.audioR = nil
	r.audioW = nil
	r.stopCh = nil
	r.doneCh = nil
	r.pumpDone = nil
	r.waitDone = nil
	r.frameCh = nil
	r.screenFrameCh = nil
	r.sampleTap = nil
	r.ring = nil
	r.mu.Unlock()

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
	if cmd != nil && cmd.Process != nil {
		waitDone := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(waitDone)
		}()
		select {
		case <-waitDone:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
			<-waitDone
		}
	}
	r.compositor.UnlockResolution()
	if err != nil {
		return err
	}
	return fmt.Errorf("recorder failed during startup frame write")
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

// loop paces video writes by wall clock at the recording frame rate. New
// frames arrive via the frame signals; when none has arrived by the next
// tick, the current frame is written again. A frozen machine (debugger
// stops, unchanged composites) therefore records as a held shot rather
// than a gap: gaps stall ffmpeg's muxer interleaving against the audio
// stream and -shortest then truncates the file at the pause.
func (r *VideoRecorder) loop(stopCh <-chan struct{}, frameCh <-chan struct{}, screenFrameCh <-chan struct{}, doneCh chan struct{}) {
	defer close(doneCh)
	r.mu.Lock()
	fps := r.fps
	r.mu.Unlock()
	if fps <= 0 {
		fps = COMPOSITOR_REFRESH_RATE
	}
	ticker := time.NewTicker(time.Second / time.Duration(fps))
	defer ticker.Stop()
	start := time.Now()
	var written int64
	useScreen := r.useScreen.Load()
	for {
		select {
		case <-stopCh:
			return
		case <-screenFrameCh:
			// A fresh screen frame was pushed; take it. The write itself
			// waits for the ticker so pacing stays wall-clock.
			r.screenReadIdx = int(r.screenShared.Swap(int32(r.screenReadIdx)))
			continue
		case <-frameCh:
			// Compositor frames are pulled at write time; the signal only
			// wakes the loop early so a stop is noticed promptly.
			continue
		case <-ticker.C:
		}
		if !r.running.Load() {
			return
		}
		owed := int64(time.Since(start).Seconds()*float64(fps)) - written
		if owed <= 0 {
			continue
		}
		// After a long stall (blocked encoder), catch up at most one
		// second per tick rather than bursting unboundedly.
		if owed > int64(fps) {
			owed = int64(fps)
		}
		for i := int64(0); i < owed; i++ {
			if useScreen {
				r.writeFrameData(r.screenBufs[r.screenReadIdx])
			} else {
				r.writeFrame()
			}
			if !r.running.Load() {
				return
			}
		}
		written += owed
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

// writeFrameData writes one video frame to ffmpeg. Used by both compositor
// mode (writeFrame) and screen-capture mode (loop). Audio travels through
// audioPump on its own goroutine: the two ffmpeg input pipes are
// interdependent through the muxer's interleaving, so a single loop writing
// both in sequence deadlocks the moment the encoder falls behind and ffmpeg
// throttles one of them (historically the audio pipe filled at exactly one
// 64 KiB pipe buffer, 44 frames, and every recording froze there).
func (r *VideoRecorder) writeFrameData(pixels []byte) {
	r.mu.Lock()
	videoIn := r.videoIn
	r.mu.Unlock()

	if videoIn == nil {
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

	r.frameCount.Add(1)
}

// audioPump feeds the audio pipe at the real sample rate on its own
// goroutine, paced by wall clock. Samples come from the sound tap's ring;
// a ring that stays short past the grace window is treated as an absent
// audio stream (headless hosts, suspended sinks) and silence is written in
// its place rather than stalling the recording. Exits on stop or on a pipe
// error; Stop closes the pipes after signalling stop, so a pump blocked in
// Write is always released.
func (r *VideoRecorder) audioPump(stopCh <-chan struct{}, audioW *os.File, ring *sampleRing, haveSound bool, pumpDone chan struct{}) {
	defer close(pumpDone)
	ticker := time.NewTicker(recorderAudioPumpTick)
	defer ticker.Stop()
	start := time.Now()
	var written int64
	shortTicks := 0
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
		}
		if !r.running.Load() {
			return
		}
		owed := int64(time.Since(start).Seconds()*recorderAudioRate) - written
		if owed <= 0 {
			continue
		}
		if haveSound && ring.available() < uint32(owed) {
			if shortTicks < recorderAudioGraceTicks {
				shortTicks++
				continue
			}
		} else {
			shortTicks = 0
		}
		buf := make([]byte, owed*2)
		for i := int64(0); i < owed; i++ {
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
			return
		}
		written += owed
	}
}

func (r *VideoRecorder) Stop() error {
	r.useScreen.Store(false)

	r.mu.Lock()
	if r.cmd == nil {
		err := r.lastErr
		r.mu.Unlock()
		return err
	}
	stopCh := r.stopCh
	doneCh := r.doneCh
	pumpDone := r.pumpDone
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
	r.pumpDone = nil
	r.frameCh = nil
	r.screenFrameCh = nil
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
	r.running.Store(false)
	r.mu.Lock()
	r.videoIn = nil
	r.audioW = nil
	r.audioR = nil
	r.ring = nil
	r.mu.Unlock()
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
	if pumpDone != nil {
		<-pumpDone
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
		r.frameSignals.Add(1)
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
