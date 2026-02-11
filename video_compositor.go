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
	"fmt"
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

// VideoCompositor blends multiple video sources into a single output
type VideoCompositor struct {
	mu                sync.Mutex
	output            VideoOutput
	sources           []VideoSource
	finalFrame        []byte
	done              chan struct{}
	frameWidth        int
	frameHeight       int
	pendingResolution atomic.Uint64
	lockedResolution  bool
}

// NewVideoCompositor creates a new video compositor
func NewVideoCompositor(output VideoOutput) *VideoCompositor {
	return &VideoCompositor{
		output:      output,
		sources:     make([]VideoSource, 0),
		done:        make(chan struct{}),
		frameWidth:  RESOLUTION_640x480_WIDTH,
		frameHeight: RESOLUTION_640x480_HEIGHT,
	}
}

// RegisterSource adds a video source to the compositor
func (c *VideoCompositor) RegisterSource(source VideoSource) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sources = append(c.sources, source)
}

// SetDimensions sets the output frame dimensions
func (c *VideoCompositor) SetDimensions(width, height int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.applyResolution(width, height)
}

func (c *VideoCompositor) NotifyResolutionChange(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	packed := (uint64(uint32(width)) << 32) | uint64(uint32(height))
	c.pendingResolution.Store(packed)
}

func (c *VideoCompositor) LockResolution(width, height int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lockedResolution = true
	c.applyResolution(width, height)
}

func (c *VideoCompositor) applyResolution(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	if width == c.frameWidth && height == c.frameHeight {
		return
	}
	c.frameWidth = width
	c.frameHeight = height
	c.finalFrame = make([]byte, width*height*BYTES_PER_PIXEL)

	if c.output != nil {
		config := c.output.GetDisplayConfig()
		config.Width = width
		config.Height = height
		if err := c.output.SetDisplayConfig(config); err != nil {
			fmt.Printf("Compositor: Error applying display config: %v\n", err)
		}
	}
}

// Start begins the compositor refresh loop
func (c *VideoCompositor) Start() error {
	// Initialize final frame buffer
	c.mu.Lock()
	if c.finalFrame == nil {
		c.finalFrame = make([]byte, c.frameWidth*c.frameHeight*BYTES_PER_PIXEL)
	}
	c.mu.Unlock()

	// Start refresh loop
	go c.refreshLoop()

	return nil
}

// Stop halts the compositor refresh loop
func (c *VideoCompositor) Stop() {
	close(c.done)
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
	defer c.mu.Unlock()

	if !c.lockedResolution {
		packed := c.pendingResolution.Swap(0)
		if packed != 0 {
			width := int(uint32(packed >> 32))
			height := int(uint32(packed))
			c.applyResolution(width, height)
		}
	}

	// Clear final frame (Go compiler optimizes this to memset)
	for i := range c.finalFrame {
		c.finalFrame[i] = 0
	}

	// Check if we can use per-scanline rendering for copper effects
	// This requires all enabled sources to implement ScanlineAware
	if c.compositeScanlineAware() {
		return
	}

	// Fallback: full-frame compositing (original behavior)
	c.compositeFullFrame()
}

// scanlineSourceEntry pairs a VideoSource with its ScanlineAware implementation
type scanlineSourceEntry struct {
	source VideoSource
	sa     ScanlineAware
	layer  int
}

// compositeScanlineAware performs per-scanline rendering for copper-style effects
// Returns true if successful, false if sources don't support it
func (c *VideoCompositor) compositeScanlineAware() bool {
	// Collect enabled sources that implement ScanlineAware
	var entries []scanlineSourceEntry
	maxSourceHeight := 0

	for _, source := range c.sources {
		if !source.IsEnabled() {
			continue
		}

		sa, ok := source.(ScanlineAware)
		if !ok {
			// Not all sources support scanline rendering, fall back to full frame
			return false
		}

		_, srcH := source.GetDimensions()
		if srcH > maxSourceHeight {
			maxSourceHeight = srcH
		}

		entries = append(entries, scanlineSourceEntry{
			source: source,
			sa:     sa,
			layer:  source.GetLayer(),
		})
	}

	// If no sources, nothing to do
	if len(entries) == 0 {
		return false
	}

	// Sort by layer (lower layers first - VideoChip layer 0 before VGA layer 10)
	// This ensures copper runs before VGA renders each scanline
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].layer < entries[i].layer {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Signal render goroutines to yield, then wait for any in-flight
	// render tick to finish before entering scanline-aware compositing.
	for _, e := range entries {
		if cm, ok := e.source.(CompositorManageable); ok {
			cm.SetCompositorManaged(true)
		}
	}
	for _, e := range entries {
		if cm, ok := e.source.(CompositorManageable); ok {
			cm.WaitRenderIdle()
		}
	}

	// Start frame on all sources
	for _, e := range entries {
		e.sa.StartFrame()
	}

	// Process each scanline
	// Lower layer sources (VideoChip with copper) process first to update state,
	// then higher layer sources (VGA) render using the updated palette
	for y := 0; y < maxSourceHeight; y++ {
		for _, e := range entries {
			e.sa.ProcessScanline(y)
		}
	}

	// Finish frame and collect results
	hasContent := false
	for _, e := range entries {
		frame := e.sa.FinishFrame()
		if frame == nil {
			continue
		}

		hasContent = true
		srcW, srcH := e.source.GetDimensions()

		// Blend source frame into final frame
		c.blendFrame(frame, srcW, srcH)

		// Signal VSync to source
		e.source.SignalVSync()
	}

	// Release render goroutines
	for _, e := range entries {
		if cm, ok := e.source.(CompositorManageable); ok {
			cm.SetCompositorManaged(false)
		}
	}

	// Send final frame to output if we have content
	if hasContent && c.output != nil && c.output.IsStarted() {
		if err := c.output.UpdateFrame(c.finalFrame); err != nil {
			fmt.Printf("Compositor: Error updating frame: %v\n", err)
		}
	}

	return true
}

// compositeFullFrame performs full-frame compositing with sequential frame collection
func (c *VideoCompositor) compositeFullFrame() {
	// Collect enabled sources and fetch frames sequentially
	// (GetFrame is a single atomic swap — goroutine overhead far exceeds the work)
	hasContent := false
	for _, source := range c.sources {
		if !source.IsEnabled() {
			continue
		}
		frame := source.GetFrame()
		if frame == nil {
			continue
		}
		w, h := source.GetDimensions()
		hasContent = true
		c.blendFrame(frame, w, h)
		source.SignalVSync()
	}

	// Send final frame to output if we have content
	if hasContent && c.output != nil && c.output.IsStarted() {
		if err := c.output.UpdateFrame(c.finalFrame); err != nil {
			fmt.Printf("Compositor: Error updating frame: %v\n", err)
		}
	}
}

// blendFrame blends a source frame into the final frame with scaling
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
