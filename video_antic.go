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
	"time"
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
	enabled        bool  // Video output enabled
	vblankActive   bool  // VBlank flag (legacy)
	lastFrameStart int64 // Timestamp of last SignalVSync (for time-based VBlank)

	// GTIA color registers
	colpf [4]uint8 // Playfield colors 0-3
	colbk uint8    // Background/border color
	colpm [4]uint8 // Player/missile colors 0-3

	// GTIA control registers
	prior  uint8 // Priority and GTIA modes
	gractl uint8 // Graphics control
	consol uint8 // Console switches (read-only, normally 0x07)

	// Player/Missile graphics registers
	hposp [4]uint8 // Player horizontal positions
	hposm [4]uint8 // Missile horizontal positions
	sizep [4]uint8 // Player sizes (0=normal, 1=double, 3=quad)
	sizem uint8    // Missile sizes (2 bits each)
	grafp [4]uint8 // Player graphics (8 pixels, directly written)
	grafm uint8    // Missile graphics (2 bits each)

	// Per-scanline player graphics and positions (for rendering) - DOUBLE BUFFERED
	// Each player can have different graphics/positions per scanline via writes during display
	// This is authentic Atari behavior - HPOSP/GRAFP changes take effect immediately
	playerGfx [2][4][ANTIC_DISPLAY_HEIGHT]uint8 // [buffer][player][scanline]
	playerPos [2][4][ANTIC_DISPLAY_HEIGHT]uint8 // [buffer][player][scanline]

	// Per-scanline color tracking for raster bar effects (double-buffered)
	scanlineColors [2][ANTIC_SCANLINES_NTSC]uint8
	writeBuffer    int  // Buffer being written to by CPU (0 or 1)
	frameReady     bool // Set when a full frame of scanlines has been written

	// Pre-allocated frame buffer (384x240 RGBA)
	frameBuffer []byte

	// Debug counters
	debugFrameCount int
	debugWriteCount int
	statusReads     int
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

	// Initialize GTIA registers
	antic.colbk = 0
	antic.prior = 0
	antic.gractl = 0
	antic.consol = 0x07 // All console buttons released

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
		// Debug: log status reads
		a.statusReads++
		// Calculate VBlank based on time within frame
		// Self-resetting: automatically starts new frame when period elapses
		// VBlank is active for the last 20% of each frame (~3.3ms of 16.67ms frame)
		now := time.Now().UnixNano()
		frameStart := a.lastFrameStart
		if frameStart == 0 {
			// Auto-initialize on first status read
			a.lastFrameStart = now
			frameStart = now
		}
		elapsed := time.Duration(now - frameStart)
		refreshInterval := time.Second / 60 // 60Hz

		// Auto-reset frame timer if we've passed a full frame
		if elapsed >= refreshInterval {
			a.lastFrameStart = now
			elapsed = 0
		}

		// VBlank is active during the last 20% of the frame
		inVBlank := elapsed >= (refreshInterval * 80 / 100)

		// Track VBlank transitions for WSYNC scanline reset
		wasInVBlank := a.vblankActive
		a.vblankActive = inVBlank

		// When transitioning from VBlank to active display, mark for scanline reset
		if wasInVBlank && !inVBlank {
			a.scanline = 0 // Reset scanline at start of active display
		}

		if inVBlank {
			return ANTIC_STATUS_VBLANK
		}
		return 0

	// GTIA color registers
	case GTIA_COLPF0:
		return uint32(a.colpf[0])
	case GTIA_COLPF1:
		return uint32(a.colpf[1])
	case GTIA_COLPF2:
		return uint32(a.colpf[2])
	case GTIA_COLPF3:
		return uint32(a.colpf[3])
	case GTIA_COLBK:
		return uint32(a.colbk)
	case GTIA_COLPM0:
		return uint32(a.colpm[0])
	case GTIA_COLPM1:
		return uint32(a.colpm[1])
	case GTIA_COLPM2:
		return uint32(a.colpm[2])
	case GTIA_COLPM3:
		return uint32(a.colpm[3])
	case GTIA_PRIOR:
		return uint32(a.prior)
	case GTIA_GRACTL:
		return uint32(a.gractl)
	case GTIA_CONSOL:
		return uint32(a.consol)

	// Player horizontal positions (read)
	case GTIA_HPOSP0:
		return uint32(a.hposp[0])
	case GTIA_HPOSP1:
		return uint32(a.hposp[1])
	case GTIA_HPOSP2:
		return uint32(a.hposp[2])
	case GTIA_HPOSP3:
		return uint32(a.hposp[3])

	// Missile horizontal positions (read)
	case GTIA_HPOSM0:
		return uint32(a.hposm[0])
	case GTIA_HPOSM1:
		return uint32(a.hposm[1])
	case GTIA_HPOSM2:
		return uint32(a.hposm[2])
	case GTIA_HPOSM3:
		return uint32(a.hposm[3])

	// Player sizes (read)
	case GTIA_SIZEP0:
		return uint32(a.sizep[0])
	case GTIA_SIZEP1:
		return uint32(a.sizep[1])
	case GTIA_SIZEP2:
		return uint32(a.sizep[2])
	case GTIA_SIZEP3:
		return uint32(a.sizep[3])
	case GTIA_SIZEM:
		return uint32(a.sizem)

	// Player graphics (read - returns last written value)
	case GTIA_GRAFP0:
		return uint32(a.grafp[0])
	case GTIA_GRAFP1:
		return uint32(a.grafp[1])
	case GTIA_GRAFP2:
		return uint32(a.grafp[2])
	case GTIA_GRAFP3:
		return uint32(a.grafp[3])
	case GTIA_GRAFM:
		return uint32(a.grafm)

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
		// Capture current state for per-scanline raster effects
		// Only write to visible scanlines (0-191)
		if a.scanline < ANTIC_DISPLAY_HEIGHT {
			a.scanlineColors[a.writeBuffer][a.scanline] = a.colbk
			// Capture player positions and graphics for this scanline
			// This enables authentic multiplexing - HPOSP/GRAFP changes mid-frame
			for p := 0; p < 4; p++ {
				a.playerGfx[a.writeBuffer][p][a.scanline] = a.grafp[p]
				a.playerPos[a.writeBuffer][p][a.scanline] = a.hposp[p]
			}
		}
		a.scanline++
		// Wrap at visible display height for clean raster bar loops
		if a.scanline >= ANTIC_DISPLAY_HEIGHT {
			// Frame complete - swap buffers
			a.writeBuffer = 1 - a.writeBuffer
			a.scanline = 0
		}
	case ANTIC_NMIEN:
		a.nmien = uint8(value)
	case ANTIC_NMIST:
		// Writing to NMIST (NMIRES) clears the status
		a.nmist = 0
	case ANTIC_ENABLE:
		wasEnabled := a.enabled
		a.enabled = (value & ANTIC_ENABLE_VIDEO) != 0
		// When first enabled, initialize frame timing so VBlank works immediately
		if !wasEnabled && a.enabled {
			a.lastFrameStart = time.Now().UnixNano()
		}
		// Note: VCOUNT, PENH, PENV are read-only

	// GTIA color registers
	case GTIA_COLPF0:
		a.colpf[0] = uint8(value)
	case GTIA_COLPF1:
		a.colpf[1] = uint8(value)
	case GTIA_COLPF2:
		a.colpf[2] = uint8(value)
	case GTIA_COLPF3:
		a.colpf[3] = uint8(value)
	case GTIA_COLBK:
		a.colbk = uint8(value)
	case GTIA_COLPM0:
		a.colpm[0] = uint8(value)
	case GTIA_COLPM1:
		a.colpm[1] = uint8(value)
	case GTIA_COLPM2:
		a.colpm[2] = uint8(value)
	case GTIA_COLPM3:
		a.colpm[3] = uint8(value)
	case GTIA_PRIOR:
		a.prior = uint8(value)
	case GTIA_GRACTL:
		a.gractl = uint8(value)
	// CONSOL is read-only

	// Player horizontal positions
	case GTIA_HPOSP0:
		a.hposp[0] = uint8(value)
	case GTIA_HPOSP1:
		a.hposp[1] = uint8(value)
	case GTIA_HPOSP2:
		a.hposp[2] = uint8(value)
	case GTIA_HPOSP3:
		a.hposp[3] = uint8(value)

	// Missile horizontal positions
	case GTIA_HPOSM0:
		a.hposm[0] = uint8(value)
	case GTIA_HPOSM1:
		a.hposm[1] = uint8(value)
	case GTIA_HPOSM2:
		a.hposm[2] = uint8(value)
	case GTIA_HPOSM3:
		a.hposm[3] = uint8(value)

	// Player sizes
	case GTIA_SIZEP0:
		a.sizep[0] = uint8(value) & 0x03
	case GTIA_SIZEP1:
		a.sizep[1] = uint8(value) & 0x03
	case GTIA_SIZEP2:
		a.sizep[2] = uint8(value) & 0x03
	case GTIA_SIZEP3:
		a.sizep[3] = uint8(value) & 0x03
	case GTIA_SIZEM:
		a.sizem = uint8(value)

	// Player graphics (direct write) - captures for current scanline (double-buffered)
	case GTIA_GRAFP0:
		a.grafp[0] = uint8(value)
		if a.scanline < ANTIC_DISPLAY_HEIGHT {
			a.playerGfx[a.writeBuffer][0][a.scanline] = uint8(value)
		}
	case GTIA_GRAFP1:
		a.grafp[1] = uint8(value)
		if a.scanline < ANTIC_DISPLAY_HEIGHT {
			a.playerGfx[a.writeBuffer][1][a.scanline] = uint8(value)
		}
	case GTIA_GRAFP2:
		a.grafp[2] = uint8(value)
		if a.scanline < ANTIC_DISPLAY_HEIGHT {
			a.playerGfx[a.writeBuffer][2][a.scanline] = uint8(value)
		}
	case GTIA_GRAFP3:
		a.grafp[3] = uint8(value)
		if a.scanline < ANTIC_DISPLAY_HEIGHT {
			a.playerGfx[a.writeBuffer][3][a.scanline] = uint8(value)
		}
	case GTIA_GRAFM:
		a.grafm = uint8(value)
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

// Handle6502GTIARead handles GTIA register reads from 6502-style addresses (0xD0xx)
func (a *ANTICEngine) Handle6502GTIARead(addr uint16) uint8 {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	reg := addr & 0x1F
	switch reg {
	case 0x00, 0x01, 0x02, 0x03: // HPOSP0-3 (reads return collision data, not position)
		return 0 // Collision detection not implemented
	case 0x04, 0x05, 0x06, 0x07: // HPOSM0-3
		return 0
	case 0x08, 0x09, 0x0A, 0x0B: // SIZEP0-3
		return a.sizep[reg-0x08]
	case 0x0C: // SIZEM
		return a.sizem
	case 0x0D, 0x0E, 0x0F, 0x10: // GRAFP0-3
		return a.grafp[reg-0x0D]
	case 0x11: // GRAFM
		return a.grafm
	case 0x12, 0x13, 0x14, 0x15: // COLPM0-3
		return a.colpm[reg-0x12]
	case 0x16, 0x17, 0x18, 0x19: // COLPF0-3
		return a.colpf[reg-0x16]
	case 0x1A: // COLBK
		return a.colbk
	case 0x1B: // PRIOR
		return a.prior
	case 0x1D: // GRACTL
		return a.gractl
	case 0x1F: // CONSOL
		return a.consol
	default:
		return 0xFF
	}
}

// Handle6502GTIAWrite handles GTIA register writes from 6502-style addresses (0xD0xx)
func (a *ANTICEngine) Handle6502GTIAWrite(addr uint16, value uint8) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	reg := addr & 0x1F
	switch reg {
	case 0x00, 0x01, 0x02, 0x03: // HPOSP0-3
		a.hposp[reg] = value
	case 0x04, 0x05, 0x06, 0x07: // HPOSM0-3
		a.hposm[reg-0x04] = value
	case 0x08, 0x09, 0x0A, 0x0B: // SIZEP0-3
		a.sizep[reg-0x08] = value & 0x03
	case 0x0C: // SIZEM
		a.sizem = value
	case 0x0D, 0x0E, 0x0F, 0x10: // GRAFP0-3
		idx := reg - 0x0D
		a.grafp[idx] = value
		if a.scanline < ANTIC_DISPLAY_HEIGHT {
			a.playerGfx[a.writeBuffer][idx][a.scanline] = value
		}
	case 0x11: // GRAFM
		a.grafm = value
	case 0x12, 0x13, 0x14, 0x15: // COLPM0-3
		a.colpm[reg-0x12] = value
	case 0x16, 0x17, 0x18, 0x19: // COLPF0-3
		a.colpf[reg-0x16] = value
	case 0x1A: // COLBK
		a.colbk = value
	case 0x1B: // PRIOR
		a.prior = value
	case 0x1D: // GRACTL
		a.gractl = value
		// CONSOL is read-only
	}
}

// =============================================================================
// Rendering
// =============================================================================

// RenderFrame renders the complete display including border
func (a *ANTICEngine) RenderFrame() []byte {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	// Read from the buffer that's NOT being written to (double-buffering)
	readBuffer := 1 - a.writeBuffer

	// Render per-scanline colors for raster bar effects
	// The frame is ANTIC_FRAME_WIDTH x ANTIC_FRAME_HEIGHT (384x240)
	// Extend plasma into borders by wrapping scanline indices
	for y := 0; y < ANTIC_FRAME_HEIGHT; y++ {
		// Map frame Y to a virtual scanline that wraps for border areas
		// This extends the plasma pattern seamlessly into the borders
		virtualScanline := y - ANTIC_BORDER_TOP

		// Wrap negative values (top border) and overflow (bottom border)
		// Use modulo to create seamless wrapping
		for virtualScanline < 0 {
			virtualScanline += ANTIC_DISPLAY_HEIGHT
		}
		virtualScanline = virtualScanline % ANTIC_DISPLAY_HEIGHT

		color := a.scanlineColors[readBuffer][virtualScanline]

		r, g, b := GetANTICColor(color)
		rowStart := y * ANTIC_FRAME_WIDTH * 4

		// Fill entire row with this color
		for x := 0; x < ANTIC_FRAME_WIDTH; x++ {
			offset := rowStart + x*4
			a.frameBuffer[offset] = r
			a.frameBuffer[offset+1] = g
			a.frameBuffer[offset+2] = b
			a.frameBuffer[offset+3] = 255 // Alpha
		}
	}

	// Render Player/Missile graphics on top of background
	// Only render in active display area
	if a.gractl&GTIA_GRACTL_PLAYER != 0 {
		for y := ANTIC_BORDER_TOP; y < ANTIC_FRAME_HEIGHT-ANTIC_BORDER_BOTTOM; y++ {
			scanline := y - ANTIC_BORDER_TOP
			if scanline >= ANTIC_DISPLAY_HEIGHT {
				continue
			}

			rowStart := y * ANTIC_FRAME_WIDTH * 4

			// Draw each player (0-3) - read from display buffer (opposite of write buffer)
			readBuffer := 1 - a.writeBuffer
			for p := 0; p < 4; p++ {
				gfx := a.playerGfx[readBuffer][p][scanline]
				if gfx == 0 {
					continue // No pixels set
				}

				// Use per-scanline position for authentic multiplexing
				hpos := int(a.playerPos[readBuffer][p][scanline])
				size := a.sizep[p]
				pr, pg, pb := GetANTICColor(a.colpm[p])

				// Width multiplier based on size
				widthMult := 1
				if size == 1 {
					widthMult = 2
				} else if size == 3 {
					widthMult = 4
				}

				// Draw 8 pixels (each bit in gfx)
				for bit := 0; bit < 8; bit++ {
					if gfx&(0x80>>bit) != 0 {
						// Calculate screen X position
						// HPOS is in color clocks, roughly maps to screen coords
						// Adjust for border offset
						baseX := hpos - 48 + ANTIC_BORDER_LEFT + bit*widthMult

						// Draw pixel(s) based on size
						for w := 0; w < widthMult; w++ {
							screenX := baseX + w
							if screenX >= 0 && screenX < ANTIC_FRAME_WIDTH {
								offset := rowStart + screenX*4
								a.frameBuffer[offset] = pr
								a.frameBuffer[offset+1] = pg
								a.frameBuffer[offset+2] = pb
								a.frameBuffer[offset+3] = 255
							}
						}
					}
				}
			}
		}
	}

	return a.frameBuffer
}

// =============================================================================
// Display List Processing
// =============================================================================

// Display List Instruction Constants
const (
	DL_BLANK1 = 0x00 // 1 blank scanline
	DL_BLANK2 = 0x10 // 2 blank scanlines
	DL_BLANK3 = 0x20 // 3 blank scanlines
	DL_BLANK4 = 0x30 // 4 blank scanlines
	DL_BLANK5 = 0x40 // 5 blank scanlines
	DL_BLANK6 = 0x50 // 6 blank scanlines
	DL_BLANK7 = 0x60 // 7 blank scanlines
	DL_BLANK8 = 0x70 // 8 blank scanlines

	DL_JMP = 0x01 // Jump to address (2 bytes follow)
	DL_JVB = 0x41 // Jump and wait for Vertical Blank

	// Graphics modes (low nibble)
	DL_MODE2  = 0x02 // 40 column text, 8 scanlines per row
	DL_MODE3  = 0x03 // 40 column text, 10 scanlines per row
	DL_MODE4  = 0x04 // 40 column text, 8 scanlines, multicolor
	DL_MODE5  = 0x05 // 40 column text, 16 scanlines, multicolor
	DL_MODE6  = 0x06 // 20 column text, 8 scanlines
	DL_MODE7  = 0x07 // 20 column text, 16 scanlines
	DL_MODE8  = 0x08 // 40 pixels wide, 8 scanlines per row
	DL_MODE9  = 0x09 // 80 pixels wide, 4 scanlines
	DL_MODE10 = 0x0A // 80 pixels wide, 2 scanlines
	DL_MODE11 = 0x0B // 160 pixels wide, 1 scanline
	DL_MODE12 = 0x0C // 160 pixels wide, 1 scanline (different colors)
	DL_MODE13 = 0x0D // 160 pixels wide, 2 scanlines
	DL_MODE14 = 0x0E // 160 pixels wide, 1 scanline, 4 colors
	DL_MODE15 = 0x0F // 320 pixels wide, 1 scanline, 2 colors (GTIA modes)

	// Modifiers (OR with mode)
	DL_LMS = 0x40 // Load Memory Scan (2 address bytes follow)
	DL_DLI = 0x80 // Display List Interrupt at end of this line

	// HSCROL/VSCROL enable
	DL_HSCROL = 0x10 // Horizontal scroll enable
	DL_VSCROL = 0x20 // Vertical scroll enable
)

// DisplayListEntry represents a decoded display list instruction
type DisplayListEntry struct {
	Opcode     uint8
	Mode       uint8  // Graphics mode (0-15)
	ScanLines  int    // Number of scanlines this instruction covers
	HasLMS     bool   // Load Memory Scan modifier
	HasDLI     bool   // Display List Interrupt modifier
	HasHScrol  bool   // Horizontal scroll enabled
	HasVScrol  bool   // Vertical scroll enabled
	MemoryAddr uint16 // Screen memory address (if LMS)
	JumpAddr   uint16 // Jump address (if JMP/JVB)
	IsBlank    bool   // Blank line instruction
	IsJump     bool   // Jump instruction
	IsJVB      bool   // Jump and wait for VBlank
}

// decodeDisplayListInstruction decodes a single display list instruction
func (a *ANTICEngine) decodeDisplayListInstruction(addr uint16) (DisplayListEntry, uint16) {
	entry := DisplayListEntry{}
	opcode := a.bus.Read8(uint32(addr))
	entry.Opcode = opcode
	nextAddr := addr + 1

	// Check for blank lines (mode 0 with scan count in upper nibble)
	mode := opcode & 0x0F
	if mode == 0 && opcode != DL_JMP {
		entry.IsBlank = true
		entry.ScanLines = int((opcode>>4)&0x07) + 1
		return entry, nextAddr
	}

	// Check for JMP/JVB
	if mode == 1 {
		entry.IsJump = true
		entry.IsJVB = (opcode & 0x40) != 0
		// Read 2-byte address
		lo := a.bus.Read8(uint32(nextAddr))
		hi := a.bus.Read8(uint32(nextAddr + 1))
		entry.JumpAddr = uint16(hi)<<8 | uint16(lo)
		nextAddr += 2
		return entry, nextAddr
	}

	// Graphics mode instruction
	entry.Mode = mode
	entry.HasDLI = (opcode & DL_DLI) != 0
	entry.HasLMS = (opcode & DL_LMS) != 0
	entry.HasHScrol = (opcode & DL_HSCROL) != 0
	entry.HasVScrol = (opcode & DL_VSCROL) != 0

	// Set scanlines per mode
	switch mode {
	case 2, 4, 6, 8:
		entry.ScanLines = 8
	case 3:
		entry.ScanLines = 10
	case 5, 7:
		entry.ScanLines = 16
	case 9:
		entry.ScanLines = 4
	case 10:
		entry.ScanLines = 2
	case 11, 12, 14, 15:
		entry.ScanLines = 1
	case 13:
		entry.ScanLines = 2
	default:
		entry.ScanLines = 1
	}

	// Read LMS address if present
	if entry.HasLMS {
		lo := a.bus.Read8(uint32(nextAddr))
		hi := a.bus.Read8(uint32(nextAddr + 1))
		entry.MemoryAddr = uint16(hi)<<8 | uint16(lo)
		nextAddr += 2
	}

	return entry, nextAddr
}

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
	frame := a.RenderFrame()
	a.debugFrameCount++
	return frame
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

	// Record frame start time for time-based VBlank calculation
	a.lastFrameStart = time.Now().UnixNano()
	a.vblankActive = true // Legacy support

	// Set VBI flag in NMIST if VBI is enabled in NMIEN
	if a.nmien&ANTIC_NMIEN_VBI != 0 {
		a.nmist |= ANTIC_NMIST_VBI
	}

	// Reset scanline counter
	a.scanline = 0
	a.vcount = 0

	// Clear the write buffer for next frame (start with current background)
	for i := 0; i < ANTIC_DISPLAY_HEIGHT; i++ {
		a.scanlineColors[a.writeBuffer][i] = a.colbk
	}

	// Clear player graphics and positions for next frame (write buffer)
	for p := 0; p < 4; p++ {
		for i := 0; i < ANTIC_DISPLAY_HEIGHT; i++ {
			a.playerGfx[a.writeBuffer][p][i] = 0
			a.playerPos[a.writeBuffer][p][i] = 0
		}
	}
}
