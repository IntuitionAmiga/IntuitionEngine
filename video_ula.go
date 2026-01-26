// video_ula.go - ZX Spectrum ULA video chip emulation for Intuition Engine

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
video_ula.go - ZX Spectrum ULA Video Chip Emulation

This module implements the ZX Spectrum ULA (Uncommitted Logic Array) video chip
as a standalone video device for the Intuition Engine. The ULA provides the
characteristic display of the ZX Spectrum with its unique memory addressing
and attribute-based color system.

Features:
- 256x192 pixel display with 32-pixel border on each side (320x256 total)
- Non-linear bitmap addressing (the famous Spectrum screen layout quirk)
- Attribute-based coloring: 8x8 pixel cells share foreground/background colors
- 15 colors: 8 base + 8 bright (black can't brighten = 15 unique)
- FLASH attribute: swaps INK/PAPER at ~1.6Hz
- BRIGHT attribute: intensifies both INK and PAPER colors
- Implements VideoSource interface for compositor integration

Memory Layout:
- Bitmap: 6144 bytes at 0x4000-0x57FF (non-linear Y addressing)
- Attributes: 768 bytes at 0x5800-0x5AFF (32x24 cells, linear)

Signal Flow:
1. CPU writes to VRAM (bitmap and attributes)
2. CPU optionally sets border color via ULA register
3. ULA renders VRAM through attribute colors to framebuffer
4. Compositor collects frame via GetFrame() and sends to display
*/

package main

import (
	"sync"
)

// ULAEngine implements ZX Spectrum ULA video as a standalone device.
// Implements VideoSource interface for compositor integration.
type ULAEngine struct {
	mutex sync.RWMutex
	bus   *SystemBus

	// Border color (0-7)
	border uint8

	// Control register
	control uint8

	// Status register - VBlank flag
	vblankActive bool

	// VRAM (6144 bitmap + 768 attributes = 6912 bytes)
	vram [ULA_VRAM_SIZE]uint8

	// Flash state for FLASH attribute
	flashState   bool
	flashCounter int

	// Pre-allocated frame buffer (320x256 RGBA)
	frameBuffer []byte
}

// NewULAEngine creates a new ULA engine instance
func NewULAEngine(bus *SystemBus) *ULAEngine {
	ula := &ULAEngine{
		bus:         bus,
		border:      0,               // Default: black border
		control:     ULA_CTRL_ENABLE, // Enabled by default
		frameBuffer: make([]byte, ULA_FRAME_WIDTH*ULA_FRAME_HEIGHT*4),
	}

	// Initialize VRAM to zero
	for i := range ula.vram {
		ula.vram[i] = 0
	}

	return ula
}

// HandleRead handles register reads
func (u *ULAEngine) HandleRead(addr uint32) uint32 {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	switch addr {
	case ULA_BORDER:
		return uint32(u.border)
	case ULA_CTRL:
		return uint32(u.control)
	case ULA_STATUS:
		// Return vblank status and clear it (acknowledge)
		status := uint32(0)
		if u.vblankActive {
			status = ULA_STATUS_VBLANK
			u.vblankActive = false // Clear on read
		}
		return status
	default:
		return 0
	}
}

// HandleWrite handles register writes
func (u *ULAEngine) HandleWrite(addr uint32, value uint32) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	switch addr {
	case ULA_BORDER:
		// Border color: only bits 0-2 are used
		u.border = uint8(value & 0x07)
	case ULA_CTRL:
		u.control = uint8(value)
	}
}

// HandleVRAMRead reads from ULA VRAM
func (u *ULAEngine) HandleVRAMRead(offset uint16) uint8 {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	if int(offset) >= len(u.vram) {
		return 0
	}
	return u.vram[offset]
}

// HandleVRAMWrite writes to ULA VRAM
func (u *ULAEngine) HandleVRAMWrite(offset uint16, value uint8) {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	if int(offset) >= len(u.vram) {
		return
	}
	u.vram[offset] = value
}

// GetBitmapAddress calculates the VRAM address for a pixel coordinate.
// The ZX Spectrum uses a peculiar non-linear addressing scheme:
// Address = ((y & 0xC0) << 5) + ((y & 0x07) << 8) + ((y & 0x38) << 2) + (x >> 3)
func (u *ULAEngine) GetBitmapAddress(y, x int) uint16 {
	// Decompose Y coordinate into its three parts
	highY := (y & 0xC0) << 5 // Top 2 bits of Y * 32
	lowY := (y & 0x07) << 8  // Bottom 3 bits of Y * 256
	midY := (y & 0x38) << 2  // Middle 3 bits of Y * 4

	// X coordinate gives the byte offset within the row
	xByte := x >> 3

	return uint16(highY + lowY + midY + xByte)
}

// GetAttributeAddress calculates the attribute address for a character cell.
// Attributes are stored linearly: row * 32 + column, starting at offset 0x1800.
func (u *ULAEngine) GetAttributeAddress(cellY, cellX int) uint16 {
	return uint16(ULA_ATTR_OFFSET + cellY*ULA_CELLS_X + cellX)
}

// ParseAttribute extracts INK, PAPER, BRIGHT, and FLASH from an attribute byte.
func ParseAttribute(attr uint8) (ink, paper uint8, bright, flash bool) {
	ink = attr & 0x07           // Bits 0-2
	paper = (attr >> 3) & 0x07  // Bits 3-5
	bright = (attr & 0x40) != 0 // Bit 6
	flash = (attr & 0x80) != 0  // Bit 7
	return
}

// GetColor returns the RGB values for a color index with brightness.
func (u *ULAEngine) GetColor(colorIndex uint8, bright bool) (r, g, b uint8) {
	index := colorIndex & 0x07
	if bright {
		return ULAColorBright[index][0], ULAColorBright[index][1], ULAColorBright[index][2]
	}
	return ULAColorNormal[index][0], ULAColorNormal[index][1], ULAColorNormal[index][2]
}

// RenderFrame renders the complete display including border.
func (u *ULAEngine) RenderFrame() []byte {
	u.mutex.RLock()
	defer u.mutex.RUnlock()

	// Get border color
	borderR, borderG, borderB := u.GetColor(u.border, false)

	// Fill entire frame with border color first
	for i := 0; i < len(u.frameBuffer); i += 4 {
		u.frameBuffer[i] = borderR
		u.frameBuffer[i+1] = borderG
		u.frameBuffer[i+2] = borderB
		u.frameBuffer[i+3] = 255 // Alpha
	}

	// Render the 256x192 display area
	for screenY := 0; screenY < ULA_DISPLAY_HEIGHT; screenY++ {
		for screenX := 0; screenX < ULA_DISPLAY_WIDTH; screenX++ {
			// Get bitmap byte and bit within it
			bitmapAddr := u.GetBitmapAddress(screenY, screenX)
			bitmapByte := u.vram[bitmapAddr]
			bitPosition := 7 - (screenX & 0x07) // MSB is leftmost pixel
			pixelSet := (bitmapByte >> bitPosition) & 1

			// Get attribute for this character cell
			cellX := screenX >> 3 // screenX / 8
			cellY := screenY >> 3 // screenY / 8
			attrAddr := u.GetAttributeAddress(cellY, cellX)
			attr := u.vram[attrAddr]

			// Parse attribute
			ink, paper, bright, flash := ParseAttribute(attr)

			// Determine actual foreground/background based on FLASH state
			fgColor := ink
			bgColor := paper
			if flash && u.flashState {
				// Swap ink and paper when flashing
				fgColor, bgColor = bgColor, fgColor
			}

			// Choose color based on whether pixel is set
			var r, g, b uint8
			if pixelSet != 0 {
				r, g, b = u.GetColor(fgColor, bright)
			} else {
				r, g, b = u.GetColor(bgColor, bright)
			}

			// Calculate frame buffer position (add border offset)
			frameX := ULA_BORDER_LEFT + screenX
			frameY := ULA_BORDER_TOP + screenY
			offset := (frameY*ULA_FRAME_WIDTH + frameX) * 4

			u.frameBuffer[offset] = r
			u.frameBuffer[offset+1] = g
			u.frameBuffer[offset+2] = b
			u.frameBuffer[offset+3] = 255 // Alpha
		}
	}

	return u.frameBuffer
}

// =============================================================================
// VideoSource Interface Implementation
// =============================================================================

// GetFrame returns the current rendered frame (nil if disabled).
func (u *ULAEngine) GetFrame() []byte {
	if !u.IsEnabled() {
		return nil
	}
	return u.RenderFrame()
}

// IsEnabled returns whether the ULA is active.
func (u *ULAEngine) IsEnabled() bool {
	u.mutex.RLock()
	defer u.mutex.RUnlock()
	return (u.control & ULA_CTRL_ENABLE) != 0
}

// GetLayer returns the Z-order for compositing (higher = on top).
func (u *ULAEngine) GetLayer() int {
	return ULA_LAYER
}

// GetDimensions returns the frame dimensions.
func (u *ULAEngine) GetDimensions() (w, h int) {
	return ULA_FRAME_WIDTH, ULA_FRAME_HEIGHT
}

// SignalVSync is called by compositor after frame sent.
// Sets VBlank flag and handles flash timing (toggle every 32 frames).
func (u *ULAEngine) SignalVSync() {
	u.mutex.Lock()
	defer u.mutex.Unlock()

	// Set VBlank flag - will be cleared when status register is read
	u.vblankActive = true

	// Handle flash timing
	u.flashCounter++
	if u.flashCounter >= ULA_FLASH_FRAMES {
		u.flashCounter = 0
		u.flashState = !u.flashState
	}
}

// =============================================================================
// SystemBus-Compatible VRAM Handlers
// =============================================================================

// HandleBusVRAMRead handles VRAM reads from the system bus (uint32 addresses)
func (u *ULAEngine) HandleBusVRAMRead(addr uint32) uint32 {
	offset := uint16(addr - ULA_VRAM_BASE)
	return uint32(u.HandleVRAMRead(offset))
}

// HandleBusVRAMWrite handles VRAM writes from the system bus (uint32 addresses)
func (u *ULAEngine) HandleBusVRAMWrite(addr uint32, value uint32) {
	offset := uint16(addr - ULA_VRAM_BASE)
	u.HandleVRAMWrite(offset, uint8(value))
}
