// video_compositor.go - Video Compositor for Intuition Engine

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine

License: GPLv3 or later
*/

/*
video_compositor.go - Video Compositor for Multiple Video Sources

This module implements a compositor that blends multiple video sources for display:
- Collects frames from registered VideoSource implementations
- Composites frames based on layer order (z-order)
- Outputs the final blended frame to the display

Signal Flow:
1. Video sources (VideoChip, VGA, future cards) register with compositor
2. Compositor runs at 60Hz refresh rate
3. Each frame, compositor collects frames from all enabled sources
4. Frames are blended in layer order (higher layer on top)
5. Final frame is sent to VideoOutput

Architecture:
                    ┌─────────────┐
  CPU → VGA VRAM → │   VideoVGA  │ ──┐
                    └─────────────┘   │     ┌─────────────┐     ┌─────────┐
                                      ├───→ │ Compositor  │ ──→ │ Display │
                    ┌─────────────┐   │     └─────────────┘     └─────────┘
  CPU → Chip VRAM → │  VideoChip  │ ──┘
                    └─────────────┘
*/

package main

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// Compositor constants
const (
	COMPOSITOR_REFRESH_RATE     = 60
	COMPOSITOR_REFRESH_INTERVAL = time.Second / COMPOSITOR_REFRESH_RATE
)

type compositorState int

const (
	compositorStopped compositorState = iota
	compositorRunning
	compositorStopping
	compositorClosed
)

type registeredSource struct {
	id     uint64
	source VideoSource
}

// VideoCompositor blends multiple video sources into a single output
type VideoCompositor struct {
	mu                sync.Mutex
	outputMu          sync.Mutex
	output            VideoOutput
	sources           []registeredSource
	nextSourceID      uint64
	finalFrame        []byte
	outputBuf         []byte
	onFrameComplete   func()
	done              chan struct{}
	frameWidth        int
	frameHeight       int
	pendingResolution atomic.Uint64
	lockedResolution  bool
	prevHasContent    bool
	frameCounter      uint64
	frameTimestamp    time.Time

	compositorRunning atomic.Bool
	state             compositorState
	stopRequested     bool
	loopDone          chan struct{}
}

// NewVideoCompositor creates a new video compositor
func NewVideoCompositor(output VideoOutput) *VideoCompositor {
	return &VideoCompositor{
		output:      output,
		sources:     make([]registeredSource, 0),
		done:        make(chan struct{}),
		frameWidth:  DefaultScreenWidth,
		frameHeight: DefaultScreenHeight,
	}
}

// RegisterSource adds a video source to the compositor
func (c *VideoCompositor) RegisterSource(source VideoSource) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.registerSourceLocked(source)
}

// RegisterSourceWithID adds a video source and returns its unregister handle.
func (c *VideoCompositor) RegisterSourceWithID(source VideoSource) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.registerSourceLocked(source)
}

func (c *VideoCompositor) registerSourceLocked(source VideoSource) uint64 {
	c.nextSourceID++
	id := c.nextSourceID
	c.sources = append(c.sources, registeredSource{id: id, source: source})
	c.sortSourcesByLayerLocked()
	return id
}

func (c *VideoCompositor) sortSourcesByLayerLocked() {
	sort.SliceStable(c.sources, func(i, j int) bool {
		return c.sources[i].source.GetLayer() < c.sources[j].source.GetLayer()
	})
}

// UnregisterSource removes a source by the id returned from RegisterSourceWithID.
func (c *VideoCompositor) UnregisterSource(id uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.sources {
		if c.sources[i].id == id {
			copy(c.sources[i:], c.sources[i+1:])
			c.sources[len(c.sources)-1] = registeredSource{}
			c.sources = c.sources[:len(c.sources)-1]
			return true
		}
	}
	return false
}

// SetDimensions sets the output frame dimensions
func (c *VideoCompositor) SetDimensions(width, height int) {
	c.mu.Lock()
	if c.lockedResolution {
		c.mu.Unlock()
		return
	}
	cfg, out, changed := c.prepareResolutionLocked(width, height)
	c.mu.Unlock()
	c.applyDisplayConfig(out, cfg, changed)
}

func (c *VideoCompositor) NotifyResolutionChange(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	// A zero packed value is a safe "no pending resolution" sentinel because
	// this API rejects non-positive dimensions before packing.
	packed := (uint64(uint32(width)) << 32) | uint64(uint32(height))
	c.pendingResolution.Store(packed)
}

func (c *VideoCompositor) LockResolution(width, height int) {
	c.mu.Lock()
	c.lockedResolution = true
	cfg, out, changed := c.prepareResolutionLocked(width, height)
	c.mu.Unlock()
	c.applyDisplayConfig(out, cfg, changed)
}

func (c *VideoCompositor) UnlockResolution() {
	c.mu.Lock()
	c.lockedResolution = false
	c.mu.Unlock()
}

func (c *VideoCompositor) prepareResolutionLocked(width, height int) (DisplayConfig, VideoOutput, bool) {
	var cfg DisplayConfig
	if width <= 0 || height <= 0 {
		return cfg, nil, false
	}
	if width == c.frameWidth && height == c.frameHeight {
		return cfg, nil, false
	}
	c.frameWidth = width
	c.frameHeight = height
	c.finalFrame = make([]byte, width*height*BYTES_PER_PIXEL)
	c.outputBuf = make([]byte, width*height*BYTES_PER_PIXEL)

	if c.output != nil {
		cfg = c.output.GetDisplayConfig()
		cfg.Width = width
		cfg.Height = height
		return cfg, c.output, true
	}
	return cfg, nil, false
}

func (c *VideoCompositor) applyDisplayConfig(out VideoOutput, cfg DisplayConfig, changed bool) {
	if !changed || out == nil {
		return
	}
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	if err := out.SetDisplayConfig(cfg); err != nil {
		fmt.Printf("Compositor: Error applying display config: %v\n", err)
	}
}

// Start begins the compositor refresh loop
func (c *VideoCompositor) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == compositorClosed {
		return errors.New("video compositor is closed")
	}
	if c.state == compositorRunning || c.state == compositorStopping {
		return nil
	}
	if c.finalFrame == nil {
		c.finalFrame = make([]byte, c.frameWidth*c.frameHeight*BYTES_PER_PIXEL)
	}
	if len(c.outputBuf) != len(c.finalFrame) {
		c.outputBuf = make([]byte, len(c.finalFrame))
	}
	c.done = make(chan struct{})
	c.stopRequested = false
	loopDone := make(chan struct{})
	c.loopDone = loopDone
	c.compositorRunning.Store(true)
	c.state = compositorRunning
	go func() {
		defer func() {
			c.mu.Lock()
			if c.state == compositorStopping {
				c.state = compositorStopped
			}
			c.compositorRunning.Store(false)
			c.mu.Unlock()
			close(loopDone)
		}()
		c.refreshLoop()
	}()
	return nil
}

// Stop halts the compositor refresh loop and waits for it to exit.
func (c *VideoCompositor) Stop() {
	c.mu.Lock()
	if c.state == compositorStopping {
		loopDone := c.loopDone
		c.mu.Unlock()
		if loopDone != nil {
			<-loopDone
		}
		return
	}
	if c.state != compositorRunning {
		c.mu.Unlock()
		return
	}
	if !c.stopRequested {
		c.state = compositorStopping
		c.stopRequested = true
		close(c.done)
	}
	loopDone := c.loopDone
	c.mu.Unlock()
	<-loopDone
}

// Close stops the compositor and releases registered source references.
func (c *VideoCompositor) Close() error {
	c.Stop()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state = compositorClosed
	c.sources = nil
	return nil
}

// refreshLoop runs the compositor at 60Hz
func (c *VideoCompositor) refreshLoop() {
	ticker := time.NewTicker(COMPOSITOR_REFRESH_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.composite()
		}
	}
}

// composite collects and blends frames from all enabled sources
func (c *VideoCompositor) composite() {
	c.mu.Lock()

	if !c.lockedResolution {
		packed := c.pendingResolution.Swap(0)
		if packed != 0 {
			width := int(uint32(packed >> 32))
			height := int(uint32(packed))
			cfg, out, changed := c.prepareResolutionLocked(width, height)
			c.mu.Unlock()
			c.applyDisplayConfig(out, cfg, changed)
			c.mu.Lock()
		}
	}
	if c.finalFrame == nil {
		c.finalFrame = make([]byte, c.frameWidth*c.frameHeight*BYTES_PER_PIXEL)
	}
	if len(c.outputBuf) != len(c.finalFrame) {
		c.outputBuf = make([]byte, len(c.finalFrame))
	}

	// Clear final frame (Go compiler optimizes this to memset)
	for i := range c.finalFrame {
		c.finalFrame[i] = 0
	}

	for _, entry := range c.sources {
		source := entry.source
		if ticker, ok := source.(FrameTicker); ok {
			safeCall("TickFrame", ticker.TickFrame)
		}
	}

	hasContent, usedScanline := c.compositeScanlineAware()
	if !usedScanline {
		hasContent = c.compositeFullFrame()
	}

	var outputFrame []byte
	if hasContent {
		c.prevHasContent = true
		copy(c.outputBuf, c.finalFrame)
		outputFrame = c.outputBuf
	} else if c.prevHasContent {
		c.prevHasContent = false
		copy(c.outputBuf, c.finalFrame)
		outputFrame = c.outputBuf
	}

	c.frameCounter++
	c.frameTimestamp = time.Now()
	out := c.output
	cb := c.onFrameComplete
	c.mu.Unlock()

	c.updateOutput(out, outputFrame)
	if cb != nil {
		cb()
	}
}

// scanlineSourceEntry pairs a VideoSource with its ScanlineAware implementation
type scanlineSourceEntry struct {
	id     uint64
	source VideoSource
	sa     ScanlineAware
	layer  int
	height int
}

// compositeScanlineAware performs per-scanline rendering for copper-style effects
// Returns whether content was produced and whether the scanline path was used.
func (c *VideoCompositor) compositeScanlineAware() (bool, bool) {
	// Collect enabled scanline sources. Opaque sources are still blended later
	// in their sorted layer slots.
	var entries []scanlineSourceEntry
	maxSourceHeight := 0

	for _, registered := range c.sources {
		source := registered.source
		if !source.IsEnabled() {
			continue
		}

		sa, ok := source.(ScanlineAware)
		if !ok {
			continue
		}

		_, srcH := source.GetDimensions()
		if srcH > maxSourceHeight {
			maxSourceHeight = srcH
		}

		entries = append(entries, scanlineSourceEntry{
			id:     registered.id,
			source: source,
			sa:     sa,
			layer:  source.GetLayer(),
			height: srcH,
		})
	}

	if len(entries) == 0 {
		return false, false
	}

	// Signal render goroutines to yield, then wait for any in-flight
	// render tick to finish before entering scanline-aware compositing.
	for _, e := range entries {
		if cm, ok := e.source.(CompositorManageable); ok {
			cm.SetCompositorManaged(true)
			defer cm.SetCompositorManaged(false)
		}
	}
	for _, e := range entries {
		if cm, ok := e.source.(CompositorManageable); ok {
			cm.WaitRenderIdle()
		}
	}

	// Start frame on all sources
	for _, e := range entries {
		safeCall("StartFrame", e.sa.StartFrame)
	}

	// Process each scanline
	// Lower layer sources (VideoChip with copper) process first to update state,
	// then higher layer sources (VGA) render using the updated palette
	for y := 0; y < maxSourceHeight; y++ {
		for _, e := range entries {
			sourceY := y
			if e.height > 0 && sourceY >= e.height {
				sourceY = e.height - 1
			}
			safeCallY("ProcessScanline", sourceY, e.sa.ProcessScanline)
		}
	}

	// Finish frame and collect results
	scanlineFrames := make(map[uint64][]byte, len(entries))
	for _, e := range entries {
		if frame, ok := safeCallR("FinishFrame", e.sa.FinishFrame); ok {
			scanlineFrames[e.id] = frame
		}
	}

	hasContent := false
	for _, registered := range c.sources {
		source := registered.source
		if !source.IsEnabled() {
			continue
		}
		frame, isScanline := scanlineFrames[registered.id]
		if !isScanline {
			frame, _ = safeCallR("GetFrame", source.GetFrame)
		}
		safeCall("SignalVSync", source.SignalVSync)

		if frame != nil {
			hasContent = true
			srcW, srcH := source.GetDimensions()
			c.blendFrame(frame, srcW, srcH)
		}
	}

	return hasContent, true
}

// compositeFullFrame performs full-frame compositing with sequential frame collection
func (c *VideoCompositor) compositeFullFrame() bool {
	// Collect enabled sources and fetch frames sequentially
	// (GetFrame is a single atomic swap - goroutine overhead far exceeds the work)
	hasContent := false
	for _, registered := range c.sources {
		source := registered.source
		if !source.IsEnabled() {
			continue
		}
		frame, _ := safeCallR("GetFrame", source.GetFrame)
		safeCall("SignalVSync", source.SignalVSync)
		if frame != nil {
			w, h := source.GetDimensions()
			hasContent = true
			c.blendFrame(frame, w, h)
		}
	}
	return hasContent
}

func (c *VideoCompositor) updateOutput(out VideoOutput, frame []byte) {
	if frame == nil || out == nil {
		return
	}
	c.outputMu.Lock()
	defer c.outputMu.Unlock()
	if out.IsStarted() {
		if err := out.UpdateFrame(frame); err != nil {
			fmt.Printf("Compositor: Error updating frame: %v\n", err)
		}
	}
}

// blendFrame blends a source frame into the final frame with scaling.
// Alpha is a binary mask: any nonzero alpha overwrites the destination.
func (c *VideoCompositor) blendFrame(srcFrame []byte, srcW, srcH int) {
	dstW := c.frameWidth
	dstH := c.frameHeight

	// Early bounds check
	if srcW <= 0 || srcH <= 0 || len(srcFrame) < srcW*srcH*BYTES_PER_PIXEL {
		return
	}
	if dstW <= 0 || dstH <= 0 || len(c.finalFrame) < dstW*dstH*BYTES_PER_PIXEL {
		return
	}

	// Fast path: 1:1 scaling (most common case)
	if srcW == dstW && srcH == dstH {
		c.blendFrame1to1(srcFrame, srcW, srcH)
		return
	}

	// Scaled path using Bresenham-style integer arithmetic
	c.blendFrameScaled(srcFrame, srcW, srcH)
}

// blendFrame1to1 is the optimized fast path for same-size source and destination.
// For large frames, it splits into horizontal strips blended in parallel.
func (c *VideoCompositor) blendFrame1to1(srcFrame []byte, width, height int) {
	const stripHeight = 60
	if height <= stripHeight {
		c.blendStrip(srcFrame, width, 0, height)
		return
	}

	var wg sync.WaitGroup
	for y0 := 0; y0 < height; y0 += stripHeight {
		y1 := min(y0+stripHeight, height)
		wg.Add(1)
		go func(startY, endY int) {
			defer wg.Done()
			c.blendStrip(srcFrame, width, startY, endY)
		}(y0, y1)
	}
	wg.Wait()
}

// blendStrip blends rows [startY, endY) from srcFrame into finalFrame.
// Alpha is tested as a mask; partial alpha is intentionally treated opaque.
func (c *VideoCompositor) blendStrip(srcFrame []byte, width, startY, endY int) {
	rowBytes := width * BYTES_PER_PIXEL
	srcOffset := startY * rowBytes
	dstOffset := startY * rowBytes

	for y := startY; y < endY; y++ {
		for x := 0; x < rowBytes; x += BYTES_PER_PIXEL {
			srcIdx := srcOffset + x
			dstIdx := dstOffset + x
			srcPixel := *(*uint32)(unsafe.Pointer(&srcFrame[srcIdx]))
			if srcPixel&0xFF000000 != 0 {
				*(*uint32)(unsafe.Pointer(&c.finalFrame[dstIdx])) = srcPixel
			}
		}
		srcOffset += rowBytes
		dstOffset += rowBytes
	}
}

// blendFrameScaled handles scaling using optimized integer arithmetic
// This matches the original dstX * srcW / dstW calculation exactly
func (c *VideoCompositor) blendFrameScaled(srcFrame []byte, srcW, srcH int) {
	dstW := c.frameWidth
	dstH := c.frameHeight

	srcRowBytes := srcW * BYTES_PER_PIXEL
	dstRowBytes := dstW * BYTES_PER_PIXEL

	dstOffset := 0

	for dstY := range dstH {
		// Calculate srcY once per row (matches original: dstY * srcH / dstH)
		srcY := dstY * srcH / dstH
		srcRowOffset := srcY * srcRowBytes

		for dstX := range dstW {
			srcX := dstX * srcW / dstW
			srcIdx := srcRowOffset + srcX*BYTES_PER_PIXEL
			dstIdx := dstOffset + dstX*BYTES_PER_PIXEL

			// Read uint32 directly using unsafe pointer
			srcPixel := *(*uint32)(unsafe.Pointer(&srcFrame[srcIdx]))
			// Check alpha (high byte in little-endian RGBA)
			if srcPixel&0xFF000000 != 0 {
				// Write uint32 directly
				*(*uint32)(unsafe.Pointer(&c.finalFrame[dstIdx])) = srcPixel
			}
		}

		dstOffset += dstRowBytes
	}
}

// SetFrameCallback installs a callback invoked after each composite() pass.
func (c *VideoCompositor) SetFrameCallback(cb func()) {
	c.mu.Lock()
	c.onFrameComplete = cb
	c.mu.Unlock()
}

// GetCurrentFrame returns a copy of the compositor's latest frame buffer.
func (c *VideoCompositor) GetCurrentFrame() []byte {
	buf, _, _ := c.GetFrameSnapshot()
	return buf
}

// GetFrameSnapshot returns a copy of the latest compositor frame with metadata.
func (c *VideoCompositor) GetFrameSnapshot() ([]byte, uint64, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.finalFrame) == 0 {
		return nil, c.frameCounter, c.frameTimestamp
	}
	out := make([]byte, len(c.finalFrame))
	copy(out, c.finalFrame)
	return out, c.frameCounter, c.frameTimestamp
}

// GetDimensions returns the compositor's current output dimensions.
func (c *VideoCompositor) GetDimensions() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.frameWidth, c.frameHeight
}

// GetNativeSourceDimensions returns the first enabled video source's native
// resolution. This may differ from the compositor output when upscaling
// (e.g. VideoChip 640x480 → compositor 800x600). Falls back to compositor
// dimensions if no source is enabled.
func (c *VideoCompositor) GetNativeSourceDimensions() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.sources {
		if s.source.IsEnabled() {
			return s.source.GetDimensions()
		}
	}
	return c.frameWidth, c.frameHeight
}

// GetTickRate returns the compositor's fixed scheduling tick in Hz.
func (c *VideoCompositor) GetTickRate() int {
	return COMPOSITOR_REFRESH_RATE
}

// GetRefreshRate returns the output device refresh rate in Hz, falling back to
// the compositor tick when no backend reports a usable value.
func (c *VideoCompositor) GetRefreshRate() int {
	c.mu.Lock()
	out := c.output
	c.mu.Unlock()
	if out == nil {
		return COMPOSITOR_REFRESH_RATE
	}
	rate := out.GetRefreshRate()
	if rate <= 0 {
		return COMPOSITOR_REFRESH_RATE
	}
	return rate
}

func safeCall(name string, fn func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Compositor: recovered panic in %s: %v\n", name, r)
			ok = false
		}
	}()
	fn()
	return true
}

func safeCallR[T any](name string, fn func() T) (out T, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Compositor: recovered panic in %s: %v\n", name, r)
			var zero T
			out = zero
			ok = false
		}
	}()
	return fn(), true
}

func safeCallY(name string, y int, fn func(int)) (ok bool) {
	return safeCall(name, func() { fn(y) })
}
