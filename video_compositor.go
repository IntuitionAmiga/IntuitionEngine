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
	"time"
)

// Compositor constants
const (
	COMPOSITOR_REFRESH_RATE     = 60
	COMPOSITOR_REFRESH_INTERVAL = time.Second / COMPOSITOR_REFRESH_RATE
)

// VideoCompositor blends multiple video sources into a single output
type VideoCompositor struct {
	mutex       sync.RWMutex
	output      VideoOutput
	sources     []VideoSource
	finalFrame  []byte
	done        chan struct{}
	frameWidth  int
	frameHeight int
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
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.sources = append(c.sources, source)
}

// SetDimensions sets the output frame dimensions
func (c *VideoCompositor) SetDimensions(width, height int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.frameWidth = width
	c.frameHeight = height
	c.finalFrame = make([]byte, width*height*BYTES_PER_PIXEL)
}

// Start begins the compositor refresh loop
func (c *VideoCompositor) Start() error {
	// Initialize final frame buffer
	c.mutex.Lock()
	if c.finalFrame == nil {
		c.finalFrame = make([]byte, c.frameWidth*c.frameHeight*BYTES_PER_PIXEL)
	}
	c.mutex.Unlock()

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
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Clear final frame
	for i := range c.finalFrame {
		c.finalFrame[i] = 0
	}

	// Track if any source provided content
	hasContent := false

	// Collect frames from enabled sources sorted by layer
	// Lower layer numbers are rendered first (background)
	// Higher layer numbers are rendered on top
	for _, source := range c.sources {
		if !source.IsEnabled() {
			continue
		}

		frame := source.GetFrame()
		if frame == nil {
			continue
		}

		hasContent = true
		srcW, srcH := source.GetDimensions()

		// Blend source frame into final frame
		c.blendFrame(frame, srcW, srcH)

		// Signal VSync to source
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

	// Scale source to destination dimensions
	for dstY := 0; dstY < dstH; dstY++ {
		srcY := dstY * srcH / dstH
		for dstX := 0; dstX < dstW; dstX++ {
			srcX := dstX * srcW / dstW

			srcIdx := (srcY*srcW + srcX) * BYTES_PER_PIXEL
			dstIdx := (dstY*dstW + dstX) * BYTES_PER_PIXEL

			if srcIdx+3 < len(srcFrame) && dstIdx+3 < len(c.finalFrame) {
				// Simple copy (opaque blend) - source replaces destination
				// Alpha blending can be added here for transparency support
				srcAlpha := srcFrame[srcIdx+3]
				if srcAlpha > 0 {
					c.finalFrame[dstIdx+0] = srcFrame[srcIdx+0] // R
					c.finalFrame[dstIdx+1] = srcFrame[srcIdx+1] // G
					c.finalFrame[dstIdx+2] = srcFrame[srcIdx+2] // B
					c.finalFrame[dstIdx+3] = srcFrame[srcIdx+3] // A
				}
			}
		}
	}
}
