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
	"sync"
)

// TEDVideoEngine implements TED video chip as a standalone device.
// Implements VideoSource interface for compositor integration.
type TEDVideoEngine struct {
	mutex sync.RWMutex
	bus   *SystemBus

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

	// Enable/status
	enabled      bool // Video output enabled
	vblankActive bool // VBlank flag

	// Current raster line (for copper/raster effects)
	rasterLine uint16

	// Cursor state
	cursorVisible bool // Current cursor visibility
	cursorCounter int  // Counter for cursor blink

	// VRAM: video matrix + color RAM + character set
	vram [TED_V_VRAM_SIZE]uint8

	// Pre-allocated frame buffer (384x272 RGBA)
	frameBuffer []byte
}

// NewTEDVideoEngine creates a new TED video engine instance
func NewTEDVideoEngine(bus *SystemBus) *TEDVideoEngine {
	ted := &TEDVideoEngine{
		bus:           bus,
		enabled:       false, // Disabled by default
		cursorVisible: true,  // Cursor starts visible
		frameBuffer:   make([]byte, TED_V_FRAME_WIDTH*TED_V_FRAME_HEIGHT*4),
	}

	// Initialize VRAM to zero
	for i := range ted.vram {
		ted.vram[i] = 0
	}

	// Copy default character set into VRAM
	charsetOffset := TED_V_MATRIX_SIZE + TED_V_COLOR_SIZE
	for i := 0; i < 256 && i < len(TEDDefaultCharset); i++ {
		for j := 0; j < 8; j++ {
			ted.vram[charsetOffset+i*8+j] = TEDDefaultCharset[i][j]
		}
	}

	return ted
}

// =============================================================================
// Register Access
// =============================================================================

// HandleRead handles register reads
func (t *TEDVideoEngine) HandleRead(addr uint32) uint32 {
	t.mutex.Lock()
	defer t.mutex.Unlock()

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
		if t.enabled {
			return TED_V_ENABLE_VIDEO
		}
		return 0
	case TED_V_STATUS:
		// Return and clear vblank flag
		status := uint32(0)
		if t.vblankActive {
			status = TED_V_STATUS_VBLANK
			t.vblankActive = false
		}
		return status
	default:
		return 0
	}
}

// HandleWrite handles register writes
func (t *TEDVideoEngine) HandleWrite(addr uint32, value uint32) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

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
		t.enabled = (value & TED_V_ENABLE_VIDEO) != 0
		// Note: RASTER registers are read-only, writes are ignored
	}
}

// =============================================================================
// VRAM Access
// =============================================================================

// HandleVRAMRead reads from TED VRAM
func (t *TEDVideoEngine) HandleVRAMRead(offset uint16) uint8 {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	if int(offset) >= len(t.vram) {
		return 0
	}
	return t.vram[offset]
}

// HandleVRAMWrite writes to TED VRAM
func (t *TEDVideoEngine) HandleVRAMWrite(offset uint16, value uint8) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

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

// RenderFrame renders the complete display including border
func (t *TEDVideoEngine) RenderFrame() []byte {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	// Get border color
	borderR, borderG, borderB := GetTEDColor(t.border)

	// Fill entire frame with border color first
	for i := 0; i < len(t.frameBuffer); i += 4 {
		t.frameBuffer[i] = borderR
		t.frameBuffer[i+1] = borderG
		t.frameBuffer[i+2] = borderB
		t.frameBuffer[i+3] = 255 // Alpha
	}

	// Get background color
	bgR, bgG, bgB := GetTEDColor(t.bgColor[0])

	// Render the 320x200 display area (40x25 characters)
	for cellY := 0; cellY < TED_V_CELLS_Y; cellY++ {
		for cellX := 0; cellX < TED_V_CELLS_X; cellX++ {
			t.renderCharacter(cellX, cellY, bgR, bgG, bgB)
		}
	}

	// Render cursor if visible
	if t.cursorVisible && t.cursorPos < TED_V_CELLS_X*TED_V_CELLS_Y {
		t.renderCursor()
	}

	return t.frameBuffer
}

// renderCharacter renders a single character cell
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
	for row := 0; row < TED_V_CELL_HEIGHT; row++ {
		// Get bitmap row (if charset offset is valid)
		var bitmapByte uint8
		if charsetOffset+row < len(t.vram) {
			bitmapByte = t.vram[charsetOffset+row]
		}

		for col := 0; col < TED_V_CELL_WIDTH; col++ {
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

// renderCursor renders the hardware cursor
func (t *TEDVideoEngine) renderCursor() {
	// Calculate cursor cell position
	cellX := int(t.cursorPos) % TED_V_CELLS_X
	cellY := int(t.cursorPos) / TED_V_CELLS_X

	// Get cursor color
	cursorR, cursorG, cursorB := GetTEDColor(t.cursorColor)

	// Render cursor as underline (bottom row of character cell)
	screenY := cellY*TED_V_CELL_HEIGHT + (TED_V_CELL_HEIGHT - 1)
	frameY := TED_V_BORDER_TOP + screenY

	for col := 0; col < TED_V_CELL_WIDTH; col++ {
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

// GetFrame returns the current rendered frame (nil if disabled)
func (t *TEDVideoEngine) GetFrame() []byte {
	if !t.IsEnabled() {
		return nil
	}
	return t.RenderFrame()
}

// IsEnabled returns whether the TED video is active
func (t *TEDVideoEngine) IsEnabled() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.enabled
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
// Sets VBlank flag and handles cursor blink timing
func (t *TEDVideoEngine) SignalVSync() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Set VBlank flag - will be cleared when status register is read
	t.vblankActive = true

	// Handle cursor blink timing
	t.cursorCounter++
	if t.cursorCounter >= TED_V_CURSOR_FRAMES {
		t.cursorCounter = 0
		t.cursorVisible = !t.cursorVisible
	}

	// Reset raster line at VSync
	t.rasterLine = 0
}
