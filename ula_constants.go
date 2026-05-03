// ula_constants.go - ZX Spectrum ULA register addresses and constants for Intuition Engine

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
ula_constants.go - ZX Spectrum ULA Video Chip Constants

This file defines the register addresses and memory layout for the ULA video chip.
The ULA provides authentic ZX Spectrum video output with 256x192 display area,
attribute-based coloring, and the characteristic non-linear bitmap addressing.

CPU-Specific Mappings:
  IE32/IE64/M68K/x86: 0xF2000+ registers, 0xFA000+ VRAM aperture
  6502:               0xD800+ registers with paged VRAM data port
  Z80:                explicit ULA ports with paged VRAM data port

Display Specifications:
  - Resolution: 256x192 pixels (32x24 character cells of 8x8 pixels)
  - Border: 32 pixels on each side → 320x256 total frame
  - Colors: 15 unique colors (8 base + 8 bright, but black can't brighten)
  - VRAM: 6144 bytes bitmap + 768 bytes attributes = 6912 bytes total
  - Flash rate: ~1.6Hz (toggle every 32 frames at 50Hz)

Attribute Byte Format:
  Bit 7: FLASH (swap INK/PAPER when set, toggles at ~1.6Hz)
  Bit 6: BRIGHT (intensify both INK and PAPER)
  Bits 5-3: PAPER (background color, 0-7)
  Bits 2-0: INK (foreground color, 0-7)
*/

package main

// =============================================================================
// ULA Register Addresses (IE32/M68K direct access)
// =============================================================================

const (
	// ULA register base address
	ULA_BASE = 0xF2000

	// ULA_BORDER - Border color register (bits 0-2 only)
	// Writing sets the border color (0-7). Upper bits are ignored.
	ULA_BORDER = 0xF2000

	// ULA_CTRL - Control register
	// Bit 0: Enable (1=enabled, 0=disabled)
	ULA_CTRL = 0xF2004

	// ULA_STATUS - Status register (read-only)
	// Bit 0: VBlank active
	ULA_STATUS = 0xF2008

	// ULA_ADDR_LO/HI - 13-bit VRAM address latch for paged-port access.
	ULA_ADDR_LO = 0xF200C
	ULA_ADDR_HI = 0xF2010

	// ULA_DATA - byte data port at current latched VRAM address.
	ULA_DATA = 0xF2014

	// ULA register region end
	ULA_REG_END = 0xF2017
)

// =============================================================================
// ULA Control Register Bits
// =============================================================================

const (
	ULA_CTRL_ENABLE        = 1 << 0 // Bit 0: ULA enable
	ULA_CTRL_VBLANK_IRQ_EN = 1 << 1 // Bit 1: VBlank IRQ enable
	ULA_CTRL_AUTO_INC      = 1 << 2 // Bit 2: auto-increment ULA_DATA latch
)

// =============================================================================
// ULA Status Register Bits
// =============================================================================

const (
	ULA_STATUS_VBLANK = 1 << 0 // Bit 0: VBlank active (set during vertical blank)
)

const (
	// IRQ_VECTOR_VBLANK is the x86 interrupt vector used by ULA VBlank IRQs.
	IRQ_VECTOR_VBLANK = 0x20
)

// =============================================================================
// ULA VRAM Layout
// =============================================================================

const (
	// ULA_VRAM_AP_BASE is the canonical IE-native VRAM aperture.
	ULA_VRAM_AP_BASE = 0xFA000

	// Bitmap section: 6144 bytes (256x192 / 8 = 6144 pixels worth of bits)
	ULA_BITMAP_SIZE = 6144

	// Attribute section offset from VRAM base
	ULA_ATTR_OFFSET = 0x1800

	// Attribute section: 768 bytes (32x24 cells)
	ULA_ATTR_SIZE = 768

	// Total VRAM size
	ULA_VRAM_SIZE = ULA_BITMAP_SIZE + ULA_ATTR_SIZE // 6912 bytes

	// ULA_VRAM_AP_END is the inclusive end of the IE-native VRAM aperture.
	ULA_VRAM_AP_END = ULA_VRAM_AP_BASE + ULA_VRAM_SIZE - 1
)

// =============================================================================
// ULA Display Dimensions
// =============================================================================

const (
	// Main display area
	ULA_DISPLAY_WIDTH  = 256
	ULA_DISPLAY_HEIGHT = 192

	// Character cell dimensions
	ULA_CELL_WIDTH  = 8
	ULA_CELL_HEIGHT = 8
	ULA_CELLS_X     = 32 // 256 / 8
	ULA_CELLS_Y     = 24 // 192 / 8

	// Border size (pixels on each side)
	ULA_BORDER_LEFT   = 32
	ULA_BORDER_RIGHT  = 32
	ULA_BORDER_TOP    = 32
	ULA_BORDER_BOTTOM = 32

	// Total frame dimensions (display + border)
	ULA_FRAME_WIDTH  = ULA_DISPLAY_WIDTH + ULA_BORDER_LEFT + ULA_BORDER_RIGHT  // 320
	ULA_FRAME_HEIGHT = ULA_DISPLAY_HEIGHT + ULA_BORDER_TOP + ULA_BORDER_BOTTOM // 256
)

// =============================================================================
// ULA Timing Constants
// =============================================================================

const (
	// Flash toggle interval (in frames at 50Hz refresh)
	ULA_FLASH_FRAMES = 32

	// Compositor layer (above VGA which is at 10)
	ULA_LAYER = 15
)

// =============================================================================
// 6502 CPU Mappings
// =============================================================================

const (
	// 6502: ULA registers at 0xD800-0xD8FF
	C6502_ULA_BASE    = 0xD800
	C6502_ULA_BORDER  = 0xD800 // Border color
	C6502_ULA_CTRL    = 0xD804 // Control register
	C6502_ULA_STATUS  = 0xD808 // Status register
	C6502_ULA_ADDR_LO = 0xD80C // VRAM address latch low byte
	C6502_ULA_ADDR_HI = 0xD810 // VRAM address latch high bits
	C6502_ULA_DATA    = 0xD814 // VRAM data port
)

// =============================================================================
// Z80 CPU Mappings (Authentic ZX Spectrum)
// =============================================================================

const (
	// Z80: ULA via explicit port I/O.
	// Writing to port 0xFE: bits 0-2 = border color, bit 3 = MIC, bit 4 = EAR
	Z80_ULA_PORT_BORDER  = 0xFE
	Z80_ULA_PORT_CTRL    = 0xFD
	Z80_ULA_PORT_STATUS  = 0xBE
	Z80_ULA_PORT_ADDR_LO = 0xFA
	Z80_ULA_PORT_ADDR_HI = 0xFB
	Z80_ULA_PORT_DATA    = 0xFC

	// Backwards-compatible name for legacy border-port call sites.
	Z80_ULA_PORT = Z80_ULA_PORT_BORDER
)

// =============================================================================
// Color Palette
// =============================================================================

// Normal colors (RGB values when BRIGHT bit is 0)
var ULAColorNormal = [8][3]uint8{
	{0, 0, 0},       // 0: Black
	{0, 0, 205},     // 1: Blue
	{205, 0, 0},     // 2: Red
	{205, 0, 205},   // 3: Magenta
	{0, 205, 0},     // 4: Green
	{0, 205, 205},   // 5: Cyan
	{205, 205, 0},   // 6: Yellow
	{205, 205, 205}, // 7: White
}

// Bright colors (RGB values when BRIGHT bit is 1)
var ULAColorBright = [8][3]uint8{
	{0, 0, 0},       // 0: Black (same, can't brighten)
	{0, 0, 255},     // 1: Bright Blue
	{255, 0, 0},     // 2: Bright Red
	{255, 0, 255},   // 3: Bright Magenta
	{0, 255, 0},     // 4: Bright Green
	{0, 255, 255},   // 5: Bright Cyan
	{255, 255, 0},   // 6: Bright Yellow
	{255, 255, 255}, // 7: Bright White
}
