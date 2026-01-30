// video_antic.go - ANTIC video chip emulation for Intuition Engine

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
video_antic.go - ANTIC Video Chip Emulation

This module implements the ANTIC (Alphanumeric Television Interface Controller)
from Atari 8-bit computers as a standalone video device for the Intuition Engine.

Features:
- Display list-driven graphics (14 modes)
- 320x192 active display, 384x240 with borders
- 128 colors (16 hues × 8 luminances)
- Horizontal/vertical fine scrolling
- WSYNC for raster synchronization
- Implements VideoSource interface for compositor integration
- Copper coprocessor compatible

Register Access:
- IE32/M68K/x86: Direct access at 0xF2100-0xF213F (4-byte aligned)
- 6502: Authentic Atari addresses at 0xD400-0xD40F

Signal Flow:
1. CPU configures display list pointer and DMACTL
2. ANTIC reads display list from memory via DMA
3. Display list specifies which modes to render on each line
4. ANTIC renders framebuffer based on display list
5. Compositor collects frame via GetFrame() and sends to display
*/

package main

import (
	"sync"
)

// ANTICEngine implements ANTIC video chip as a standalone device.
// Implements VideoSource interface for compositor integration.
type ANTICEngine struct {
	mutex sync.RWMutex
	bus   *SystemBus

	// DMA and control registers
	dmactl uint8 // DMA control (playfield width, DMA enables)
	chactl uint8 // Character control (inverse, reflect)

	// Display list pointer
	dlistl uint8 // Display list pointer low byte
	dlisth uint8 // Display list pointer high byte

	// Scroll registers
	hscrol uint8 // Horizontal fine scroll (0-15)
	vscrol uint8 // Vertical fine scroll (0-15)

	// Memory base addresses
	pmbase uint8 // Player-missile base address (high byte)
	chbase uint8 // Character set base address (high byte)

	// NMI control
	nmien uint8 // NMI enable register
	nmist uint8 // NMI status register

	// Read-only status
	vcount   uint8  // Vertical counter (scanline/2)
	scanline uint16 // Internal scanline counter

	// Light pen registers (read-only)
	penh uint8
	penv uint8

	// IE-specific extensions
	enabled      bool // Video output enabled
	vblankActive bool // VBlank flag

	// Color registers (from GTIA, but ANTIC uses for background)
	colbk uint8 // Background/border color

	// Pre-allocated frame buffer (384x240 RGBA)
	frameBuffer []byte
}

// NewANTICEngine creates a new ANTIC video engine instance
func NewANTICEngine(bus *SystemBus) *ANTICEngine {
	antic := &ANTICEngine{
		bus:         bus,
		enabled:     false, // Disabled by default
		frameBuffer: make([]byte, ANTIC_FRAME_WIDTH*ANTIC_FRAME_HEIGHT*4),
	}

	// Initialize to safe defaults
	antic.dmactl = 0
	antic.chactl = 0
	antic.hscrol = 0
	antic.vscrol = 0
	antic.nmien = 0
	antic.nmist = 0
	antic.vcount = 0
	antic.scanline = 0
	antic.colbk = 0

	return antic
}

// =============================================================================
// Register Access
// =============================================================================

// HandleRead handles register reads
func (a *ANTICEngine) HandleRead(addr uint32) uint32 {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	switch addr {
	case ANTIC_DMACTL:
		return uint32(a.dmactl)
	case ANTIC_CHACTL:
		return uint32(a.chactl)
	case ANTIC_DLISTL:
		return uint32(a.dlistl)
	case ANTIC_DLISTH:
		return uint32(a.dlisth)
	case ANTIC_HSCROL:
		return uint32(a.hscrol)
	case ANTIC_VSCROL:
		return uint32(a.vscrol)
	case ANTIC_PMBASE:
		return uint32(a.pmbase)
	case ANTIC_CHBASE:
		return uint32(a.chbase)
	case ANTIC_WSYNC:
		// Write-only register, read returns 0
		return 0
	case ANTIC_VCOUNT:
		// Returns scanline / 2
		return uint32(a.scanline / 2)
	case ANTIC_PENH:
		return uint32(a.penh)
	case ANTIC_PENV:
		return uint32(a.penv)
	case ANTIC_NMIEN:
		return uint32(a.nmien)
	case ANTIC_NMIST:
		return uint32(a.nmist)
	case ANTIC_ENABLE:
		if a.enabled {
			return ANTIC_ENABLE_VIDEO
		}
		return 0
	case ANTIC_STATUS:
		// Return VBlank status (read-only, doesn't clear)
		status := uint32(0)
		if a.vblankActive {
			status = ANTIC_STATUS_VBLANK
		}
		return status
	default:
		return 0
	}
}

// HandleWrite handles register writes
func (a *ANTICEngine) HandleWrite(addr uint32, value uint32) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	switch addr {
	case ANTIC_DMACTL:
		a.dmactl = uint8(value)
	case ANTIC_CHACTL:
		a.chactl = uint8(value)
	case ANTIC_DLISTL:
		a.dlistl = uint8(value)
	case ANTIC_DLISTH:
		a.dlisth = uint8(value)
	case ANTIC_HSCROL:
		// Mask to 4 bits (0-15)
		a.hscrol = uint8(value) & 0x0F
	case ANTIC_VSCROL:
		// Mask to 4 bits (0-15)
		a.vscrol = uint8(value) & 0x0F
	case ANTIC_PMBASE:
		a.pmbase = uint8(value)
	case ANTIC_CHBASE:
		a.chbase = uint8(value)
	case ANTIC_WSYNC:
		// Wait for horizontal sync - advance to next scanline
		// In real hardware this halts the CPU until HSYNC
		// For emulation, we just note it (actual sync handled elsewhere)
	case ANTIC_NMIEN:
		a.nmien = uint8(value)
	case ANTIC_NMIST:
		// Writing to NMIST (NMIRES) clears the status
		a.nmist = 0
	case ANTIC_ENABLE:
		a.enabled = (value & ANTIC_ENABLE_VIDEO) != 0
		// Note: VCOUNT, PENH, PENV are read-only
	}
}

// =============================================================================
// 6502 Register Access (Atari authentic addresses)
// =============================================================================

// Handle6502Read handles register reads from 6502-style addresses
func (a *ANTICEngine) Handle6502Read(addr uint16) uint8 {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	reg := addr & 0x0F
	switch reg {
	case 0x00: // DMACTL
		return a.dmactl
	case 0x01: // CHACTL
		return a.chactl
	case 0x02: // DLISTL
		return a.dlistl
	case 0x03: // DLISTH
		return a.dlisth
	case 0x04: // HSCROL
		return a.hscrol
	case 0x05: // VSCROL
		return a.vscrol
	case 0x07: // PMBASE
		return a.pmbase
	case 0x09: // CHBASE
		return a.chbase
	case 0x0A: // WSYNC (write-only)
		return 0
	case 0x0B: // VCOUNT
		return uint8(a.scanline / 2)
	case 0x0C: // PENH
		return a.penh
	case 0x0D: // PENV
		return a.penv
	case 0x0E: // NMIEN
		return a.nmien
	case 0x0F: // NMIST
		return a.nmist
	default:
		return 0xFF
	}
}

// Handle6502Write handles register writes from 6502-style addresses
func (a *ANTICEngine) Handle6502Write(addr uint16, value uint8) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	reg := addr & 0x0F
	switch reg {
	case 0x00: // DMACTL
		a.dmactl = value
	case 0x01: // CHACTL
		a.chactl = value
	case 0x02: // DLISTL
		a.dlistl = value
	case 0x03: // DLISTH
		a.dlisth = value
	case 0x04: // HSCROL
		a.hscrol = value & 0x0F
	case 0x05: // VSCROL
		a.vscrol = value & 0x0F
	case 0x07: // PMBASE
		a.pmbase = value
	case 0x09: // CHBASE
		a.chbase = value
	case 0x0A: // WSYNC
		// Wait for horizontal sync
		a.advanceToNextScanline()
	case 0x0E: // NMIEN
		a.nmien = value
	case 0x0F: // NMIRES (writing clears NMIST)
		a.nmist = 0
	}
}

// advanceToNextScanline advances to the next scanline boundary
func (a *ANTICEngine) advanceToNextScanline() {
	a.scanline++
	if a.scanline >= ANTIC_SCANLINES_NTSC {
		a.scanline = 0
	}
}

// =============================================================================
// Rendering
// =============================================================================

// RenderFrame renders the complete display including border
func (a *ANTICEngine) RenderFrame() []byte {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	// Get border/background color
	borderR, borderG, borderB := GetANTICColor(a.colbk)

	// Fill entire frame with border color first
	for i := 0; i < len(a.frameBuffer); i += 4 {
		a.frameBuffer[i] = borderR
		a.frameBuffer[i+1] = borderG
		a.frameBuffer[i+2] = borderB
		a.frameBuffer[i+3] = 255 // Alpha
	}

	// For now, basic rendering - future implementation will process display list
	// The display list would specify what to render on each line

	return a.frameBuffer
}

// =============================================================================
// Display List Processing
// =============================================================================

// decodeBlankLines returns the number of blank lines for a blank line opcode
func (a *ANTICEngine) decodeBlankLines(opcode uint8) int {
	// Opcodes 0x00-0x70 (top 4 bits) specify 1-8 blank lines
	// 0x00 = 1 blank line
	// 0x10 = 2 blank lines
	// ...
	// 0x70 = 8 blank lines
	return int((opcode>>4)&0x07) + 1
}

// isJVBOpcode returns true if the opcode is Jump and Vertical Blank
func (a *ANTICEngine) isJVBOpcode(opcode uint8) bool {
	// JVB opcode is 0x41 (Jump and wait for Vertical Blank)
	return opcode == 0x41
}

// getDisplayListAddress returns the full 16-bit display list address
func (a *ANTICEngine) getDisplayListAddress() uint16 {
	return (uint16(a.dlisth) << 8) | uint16(a.dlistl)
}

// =============================================================================
// VideoSource Interface Implementation
// =============================================================================

// GetFrame returns the current rendered frame (nil if disabled)
func (a *ANTICEngine) GetFrame() []byte {
	if !a.IsEnabled() {
		return nil
	}
	return a.RenderFrame()
}

// IsEnabled returns whether ANTIC video is active
func (a *ANTICEngine) IsEnabled() bool {
	a.mutex.RLock()
	defer a.mutex.RUnlock()
	return a.enabled
}

// GetLayer returns the Z-order for compositing (higher = on top)
func (a *ANTICEngine) GetLayer() int {
	return ANTIC_LAYER
}

// GetDimensions returns the frame dimensions
func (a *ANTICEngine) GetDimensions() (w, h int) {
	return ANTIC_FRAME_WIDTH, ANTIC_FRAME_HEIGHT
}

// SignalVSync is called by compositor after frame sent
// Sets VBlank flag and handles NMI timing
func (a *ANTICEngine) SignalVSync() {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	// Set VBlank flag
	a.vblankActive = true

	// Set VBI flag in NMIST if VBI is enabled in NMIEN
	if a.nmien&ANTIC_NMIEN_VBI != 0 {
		a.nmist |= ANTIC_NMIST_VBI
	}

	// Reset scanline counter
	a.scanline = 0
	a.vcount = 0
}
