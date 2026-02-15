// video_ted.go - MOS 7360/8360 TED video chip emulation for Intuition Engine

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
video_ted.go - MOS 7360/8360 TED Video Chip Emulation

This module implements the TED (Text Display) video capabilities from the
Commodore Plus/4 as a standalone video device for the Intuition Engine.

Features:
- 40x25 text mode (8x8 character cells)
- 320x200 pixel display with border (384x272 total frame)
- 121 colors (16 hues × 8 luminances)
- Hardware cursor with blink
- Implements VideoSource interface for compositor integration
- Copper coprocessor compatible

Memory Layout:
- Video matrix: 1KB (40x25 = 1000 bytes, rounded up)
- Color RAM: 1KB (one byte per character cell)
- Character set: 2KB (256 characters × 8 bytes)

Signal Flow:
1. CPU writes to VRAM (video matrix and color RAM)
2. CPU sets control registers (colors, enable, cursor)
3. TED renders VRAM through character set to framebuffer
4. Compositor collects frame via GetFrame() and sends to display
*/

package main

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// TEDVideoEngine implements TED video chip as a standalone device.
// Implements VideoSource interface for compositor integration.
type TEDVideoEngine struct {
	mu  sync.Mutex
	bus *MachineBus

	// Control registers
	ctrl1     uint8 // Control register 1 (DEN, BMM, ECM, RSEL, YSCROLL)
	ctrl2     uint8 // Control register 2 (MCM, CSEL, XSCROLL)
	charBase  uint8 // Character/bitmap base address
	videoBase uint8 // Video matrix base address

	// Color registers
	bgColor [4]uint8 // Background colors 0-3
	border  uint8    // Border color

	// Cursor registers
	cursorPos   uint16 // Cursor position (0-999)
	cursorColor uint8  // Cursor color

	// Lock-free flags
	enabled      atomic.Bool // Video output enabled
	vblankActive atomic.Bool // VBlank flag

	// Current raster line (for copper/raster effects)
	rasterLine uint16

	// Cursor state
	cursorVisible bool // Current cursor visibility
	cursorCounter int  // Counter for cursor blink

	// VRAM: video matrix + color RAM + character set
	vram [TED_V_VRAM_SIZE]uint8

	// Snapshot fields for lock-free rendering
	snapVram    [TED_V_VRAM_SIZE]uint8
	snapBgColor [4]uint8
	snapBorder  uint8

	// Pre-allocated frame buffer (384x272 RGBA)
	frameBuffer []byte

	// Triple-buffered frame output for lock-free GetFrame()
	frameBufs  [3][]byte
	writeIdx   int
	sharedIdx  atomic.Int32
	readingIdx int

	// Render goroutine lifecycle
	renderMu      sync.Mutex
	renderRunning atomic.Bool
	renderCancel  context.CancelFunc
	renderDone    chan struct{}

	// Set by compositor during scanline-aware rendering
	compositorManaged atomic.Bool
	rendering         atomic.Bool // True while renderLoop is inside RenderFrame
}

// NewTEDVideoEngine creates a new TED video engine instance
func NewTEDVideoEngine(bus *MachineBus) *TEDVideoEngine {
	ted := &TEDVideoEngine{
		bus:           bus,
		cursorVisible: true, // Cursor starts visible
		frameBuffer:   make([]byte, TED_V_FRAME_WIDTH*TED_V_FRAME_HEIGHT*4),
	}
	// enabled defaults to false (atomic.Bool zero value)

	// Initialize VRAM to zero
	for i := range ted.vram {
		ted.vram[i] = 0
	}

	// Copy default character set into VRAM
	charsetOffset := TED_V_MATRIX_SIZE + TED_V_COLOR_SIZE
	for i := 0; i < 256 && i < len(TEDDefaultCharset); i++ {
		for j := range 8 {
			ted.vram[charsetOffset+i*8+j] = TEDDefaultCharset[i][j]
		}
	}

	// Initialize triple buffers for lock-free GetFrame
	bufSize := TED_V_FRAME_WIDTH * TED_V_FRAME_HEIGHT * 4
	for i := range ted.frameBufs {
		ted.frameBufs[i] = make([]byte, bufSize)
	}
	ted.writeIdx = 0
	ted.sharedIdx.Store(1)
	ted.readingIdx = 2

	return ted
}

// =============================================================================
// Register Access
// =============================================================================

// HandleRead handles register reads
func (t *TEDVideoEngine) HandleRead(addr uint32) uint32 {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch addr {
	case TED_V_CTRL1:
		return uint32(t.ctrl1)
	case TED_V_CTRL2:
		return uint32(t.ctrl2)
	case TED_V_CHAR_BASE:
		return uint32(t.charBase)
	case TED_V_VIDEO_BASE:
		return uint32(t.videoBase)
	case TED_V_BG_COLOR0:
		return uint32(t.bgColor[0])
	case TED_V_BG_COLOR1:
		return uint32(t.bgColor[1])
	case TED_V_BG_COLOR2:
		return uint32(t.bgColor[2])
	case TED_V_BG_COLOR3:
		return uint32(t.bgColor[3])
	case TED_V_BORDER:
		return uint32(t.border)
	case TED_V_CURSOR_HI:
		return uint32((t.cursorPos >> 8) & 0xFF)
	case TED_V_CURSOR_LO:
		return uint32(t.cursorPos & 0xFF)
	case TED_V_CURSOR_CLR:
		return uint32(t.cursorColor)
	case TED_V_RASTER_LO:
		return uint32(t.rasterLine & 0xFF)
	case TED_V_RASTER_HI:
		return uint32((t.rasterLine >> 8) & 0x01)
	case TED_V_ENABLE:
		if t.enabled.Load() {
			return TED_V_ENABLE_VIDEO
		}
		return 0
	case TED_V_STATUS:
		// Return and clear vblank flag - atomic swap
		if t.vblankActive.Swap(false) {
			return TED_V_STATUS_VBLANK
		}
		return 0
	default:
		return 0
	}
}

// HandleWrite handles register writes
func (t *TEDVideoEngine) HandleWrite(addr uint32, value uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch addr {
	case TED_V_CTRL1:
		t.ctrl1 = uint8(value)
	case TED_V_CTRL2:
		t.ctrl2 = uint8(value)
	case TED_V_CHAR_BASE:
		t.charBase = uint8(value)
	case TED_V_VIDEO_BASE:
		t.videoBase = uint8(value)
	case TED_V_BG_COLOR0:
		t.bgColor[0] = uint8(value)
	case TED_V_BG_COLOR1:
		t.bgColor[1] = uint8(value)
	case TED_V_BG_COLOR2:
		t.bgColor[2] = uint8(value)
	case TED_V_BG_COLOR3:
		t.bgColor[3] = uint8(value)
	case TED_V_BORDER:
		t.border = uint8(value)
	case TED_V_CURSOR_HI:
		t.cursorPos = (t.cursorPos & 0x00FF) | (uint16(value&0x03) << 8)
	case TED_V_CURSOR_LO:
		t.cursorPos = (t.cursorPos & 0xFF00) | uint16(value&0xFF)
	case TED_V_CURSOR_CLR:
		t.cursorColor = uint8(value)
	case TED_V_ENABLE:
		t.enabled.Store((value & TED_V_ENABLE_VIDEO) != 0)
		// Note: RASTER registers are read-only, writes are ignored
	}
}

// =============================================================================
// VRAM Access
// =============================================================================

// HandleVRAMRead reads from TED VRAM
func (t *TEDVideoEngine) HandleVRAMRead(offset uint16) uint8 {
	t.mu.Lock()
	defer t.mu.Unlock()

	if int(offset) >= len(t.vram) {
		return 0
	}
	return t.vram[offset]
}

// HandleVRAMWrite writes to TED VRAM
func (t *TEDVideoEngine) HandleVRAMWrite(offset uint16, value uint8) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if int(offset) >= len(t.vram) {
		return
	}
	t.vram[offset] = value
}

// HandleBusVRAMRead handles VRAM reads from the system bus (uint32 addresses)
func (t *TEDVideoEngine) HandleBusVRAMRead(addr uint32) uint32 {
	offset := uint16(addr - TED_V_VRAM_BASE)
	return uint32(t.HandleVRAMRead(offset))
}

// HandleBusVRAMWrite handles VRAM writes from the system bus (uint32 addresses)
func (t *TEDVideoEngine) HandleBusVRAMWrite(addr uint32, value uint32) {
	offset := uint16(addr - TED_V_VRAM_BASE)
	t.HandleVRAMWrite(offset, uint8(value))
}

// =============================================================================
// Address Calculation
// =============================================================================

// GetVideoMatrixAddress returns the VRAM offset for a character position
func (t *TEDVideoEngine) GetVideoMatrixAddress(x, y int) int {
	return y*TED_V_CELLS_X + x
}

// GetColorRAMAddress returns the color RAM offset for a character position
func (t *TEDVideoEngine) GetColorRAMAddress(x, y int) int {
	return TED_V_MATRIX_SIZE + y*TED_V_CELLS_X + x
}

// GetCharsetAddress returns the offset into the character set for a character
func (t *TEDVideoEngine) GetCharsetAddress(charCode uint8) int {
	return TED_V_MATRIX_SIZE + TED_V_COLOR_SIZE + int(charCode)*8
}

// =============================================================================
// Rendering
// =============================================================================

// RenderFrameTo renders the complete display directly into dst, avoiding a copy.
func (t *TEDVideoEngine) RenderFrameTo(dst []byte) {
	saved := t.frameBuffer
	t.frameBuffer = dst
	t.RenderFrame()
	t.frameBuffer = saved
}

// RenderFrame renders the complete display including border
func (t *TEDVideoEngine) RenderFrame() []byte {
	// Snapshot VRAM and registers under lock, then render lock-free
	t.mu.Lock()
	t.snapVram = t.vram
	t.snapBorder = t.border
	t.snapBgColor = t.bgColor
	snapCursorVisible := t.cursorVisible
	snapCursorPos := t.cursorPos
	snapCursorColor := t.cursorColor
	t.mu.Unlock()

	// Pack border color as uint32
	borderIdx := t.snapBorder & 0x7F
	borderC := TEDPalette[borderIdx]
	borderU32 := uint32(borderC[0]) | uint32(borderC[1])<<8 | uint32(borderC[2])<<16 | 0xFF000000

	// Fill entire frame with border color using uint32 writes
	for i := 0; i < len(t.frameBuffer); i += 4 {
		*(*uint32)(unsafe.Pointer(&t.frameBuffer[i])) = borderU32
	}

	// Pack background color as uint32
	bgIdx := t.snapBgColor[0] & 0x7F
	bgC := TEDPalette[bgIdx]
	bgU32 := uint32(bgC[0]) | uint32(bgC[1])<<8 | uint32(bgC[2])<<16 | 0xFF000000

	// Pre-compute address offsets for character rendering
	charsetBase := TED_V_MATRIX_SIZE + TED_V_COLOR_SIZE

	// Render the 320x200 display area (40x25 characters)
	for cellY := range TED_V_CELLS_Y {
		// Pre-compute row base addresses
		matrixRowBase := cellY * TED_V_CELLS_X
		colorRowBase := TED_V_MATRIX_SIZE + cellY*TED_V_CELLS_X

		// Pre-compute frame buffer base for this character row
		screenYBase := cellY * TED_V_CELL_HEIGHT
		frameYBase := (TED_V_BORDER_TOP + screenYBase) * TED_V_FRAME_WIDTH

		for cellX := range TED_V_CELLS_X {
			// Get character code from video matrix
			charCode := t.snapVram[matrixRowBase+cellX]

			// Get foreground color from color RAM - pack as uint32
			fgColorByte := t.snapVram[colorRowBase+cellX]
			fgIdx := fgColorByte & 0x7F
			fgC := TEDPalette[fgIdx]
			fgU32 := uint32(fgC[0]) | uint32(fgC[1])<<8 | uint32(fgC[2])<<16 | 0xFF000000

			// Get character bitmap offset
			charsetOffset := charsetBase + int(charCode)*8

			// Pre-compute X base for frame buffer
			frameXBase := TED_V_BORDER_LEFT + cellX*TED_V_CELL_WIDTH

			// Render 8x8 pixel character
			for row := range TED_V_CELL_HEIGHT {
				// Get bitmap row (if charset offset is valid)
				var bitmapByte uint8
				if charsetOffset+row < len(t.snapVram) {
					bitmapByte = t.snapVram[charsetOffset+row]
				}

				// Frame buffer row offset
				frameRowOffset := (frameYBase + row*TED_V_FRAME_WIDTH + frameXBase) * 4

				// Render 8 pixels with uint32 writes
				for col := range TED_V_CELL_WIDTH {
					offset := frameRowOffset + col*4
					if (bitmapByte>>(7-col))&1 != 0 {
						*(*uint32)(unsafe.Pointer(&t.frameBuffer[offset])) = fgU32
					} else {
						*(*uint32)(unsafe.Pointer(&t.frameBuffer[offset])) = bgU32
					}
				}
			}
		}
	}

	// Render cursor if visible (using snapshot)
	if snapCursorVisible && snapCursorPos < TED_V_CELLS_X*TED_V_CELLS_Y {
		t.renderCursorSnapshot(snapCursorPos, snapCursorColor)
	}

	return t.frameBuffer
}

// renderCharacter renders a single character cell (legacy function for compatibility)
func (t *TEDVideoEngine) renderCharacter(cellX, cellY int, bgR, bgG, bgB uint8) {
	// Get character code from video matrix
	matrixOffset := t.GetVideoMatrixAddress(cellX, cellY)
	charCode := t.vram[matrixOffset]

	// Get foreground color from color RAM
	colorOffset := t.GetColorRAMAddress(cellX, cellY)
	fgColorByte := t.vram[colorOffset]
	fgR, fgG, fgB := GetTEDColor(fgColorByte)

	// Get character bitmap
	charsetOffset := t.GetCharsetAddress(charCode)

	// Render 8x8 pixel character
	for row := range TED_V_CELL_HEIGHT {
		// Get bitmap row (if charset offset is valid)
		var bitmapByte uint8
		if charsetOffset+row < len(t.vram) {
			bitmapByte = t.vram[charsetOffset+row]
		}

		for col := range TED_V_CELL_WIDTH {
			// Check if pixel is set (MSB = leftmost)
			bitPos := 7 - col
			pixelSet := (bitmapByte >> bitPos) & 1

			// Calculate frame buffer position
			screenX := cellX*TED_V_CELL_WIDTH + col
			screenY := cellY*TED_V_CELL_HEIGHT + row
			frameX := TED_V_BORDER_LEFT + screenX
			frameY := TED_V_BORDER_TOP + screenY
			offset := (frameY*TED_V_FRAME_WIDTH + frameX) * 4

			// Set pixel color
			if pixelSet != 0 {
				t.frameBuffer[offset] = fgR
				t.frameBuffer[offset+1] = fgG
				t.frameBuffer[offset+2] = fgB
			} else {
				t.frameBuffer[offset] = bgR
				t.frameBuffer[offset+1] = bgG
				t.frameBuffer[offset+2] = bgB
			}
			t.frameBuffer[offset+3] = 255 // Alpha
		}
	}
}

// renderCursor renders the hardware cursor (uses live state, requires lock held)
func (t *TEDVideoEngine) renderCursor() {
	t.renderCursorSnapshot(t.cursorPos, t.cursorColor)
}

// renderCursorSnapshot renders the hardware cursor using snapshot values (lock-free)
func (t *TEDVideoEngine) renderCursorSnapshot(cursorPos uint16, cursorColor uint8) {
	// Calculate cursor cell position
	cellX := int(cursorPos) % TED_V_CELLS_X
	cellY := int(cursorPos) / TED_V_CELLS_X

	// Get cursor color
	cursorR, cursorG, cursorB := GetTEDColor(cursorColor)

	// Render cursor as underline (bottom row of character cell)
	screenY := cellY*TED_V_CELL_HEIGHT + (TED_V_CELL_HEIGHT - 1)
	frameY := TED_V_BORDER_TOP + screenY

	for col := range TED_V_CELL_WIDTH {
		screenX := cellX*TED_V_CELL_WIDTH + col
		frameX := TED_V_BORDER_LEFT + screenX
		offset := (frameY*TED_V_FRAME_WIDTH + frameX) * 4

		t.frameBuffer[offset] = cursorR
		t.frameBuffer[offset+1] = cursorG
		t.frameBuffer[offset+2] = cursorB
		t.frameBuffer[offset+3] = 255
	}
}

// =============================================================================
// VideoSource Interface Implementation
// =============================================================================

// GetFrame returns the current rendered frame via lock-free triple-buffer swap.
func (t *TEDVideoEngine) GetFrame() []byte {
	if !t.IsEnabled() {
		return nil
	}
	newRead := int(t.sharedIdx.Swap(int32(t.readingIdx)))
	t.readingIdx = newRead
	return t.frameBufs[t.readingIdx]
}

// IsEnabled returns whether the TED video is active (lock-free)
func (t *TEDVideoEngine) IsEnabled() bool {
	return t.enabled.Load()
}

// GetLayer returns the Z-order for compositing (higher = on top)
func (t *TEDVideoEngine) GetLayer() int {
	return TED_V_LAYER
}

// GetDimensions returns the frame dimensions
func (t *TEDVideoEngine) GetDimensions() (w, h int) {
	return TED_V_FRAME_WIDTH, TED_V_FRAME_HEIGHT
}

// SignalVSync is called by compositor after frame sent
// Sets VBlank flag (lock-free) and handles cursor blink timing
func (t *TEDVideoEngine) SignalVSync() {
	// Set VBlank flag - lock-free
	t.vblankActive.Store(true)

	// Cursor blink and raster line are compositor-only state
	t.cursorCounter++
	if t.cursorCounter >= TED_V_CURSOR_FRAMES {
		t.cursorCounter = 0
		t.cursorVisible = !t.cursorVisible
	}

	// Reset raster line at VSync
	t.rasterLine = 0
}

// =============================================================================
// Independent Render Goroutine
// =============================================================================

// SetCompositorManaged implements CompositorManageable.
func (t *TEDVideoEngine) SetCompositorManaged(managed bool) {
	t.compositorManaged.Store(managed)
}

// WaitRenderIdle implements CompositorManageable.
func (t *TEDVideoEngine) WaitRenderIdle() {
	for t.rendering.Load() {
		runtime.Gosched()
	}
}

// StartRenderLoop spawns a 60Hz render goroutine for lock-free GetFrame.
func (t *TEDVideoEngine) StartRenderLoop() {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	if t.renderRunning.Load() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.renderCancel = cancel
	done := make(chan struct{})
	t.renderDone = done
	t.renderRunning.Store(true)
	go t.renderLoop(ctx, done)
}

// StopRenderLoop stops the render goroutine and waits for it to exit.
func (t *TEDVideoEngine) StopRenderLoop() {
	t.renderMu.Lock()
	if !t.renderRunning.Swap(false) {
		t.renderMu.Unlock()
		return
	}
	cancel := t.renderCancel
	done := t.renderDone
	t.renderMu.Unlock()
	cancel()
	<-done
}

// renderLoop runs at 60Hz, rendering frames into the triple buffer.
// done is goroutine-local to avoid close-of-wrong-channel on restart.
func (t *TEDVideoEngine) renderLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(COMPOSITOR_REFRESH_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !t.enabled.Load() || t.compositorManaged.Load() {
				continue
			}
			t.rendering.Store(true)
			if t.compositorManaged.Load() {
				t.rendering.Store(false)
				continue
			}
			t.RenderFrameTo(t.frameBufs[t.writeIdx])
			t.rendering.Store(false)
			t.writeIdx = int(t.sharedIdx.Swap(int32(t.writeIdx)))
		}
	}
}
