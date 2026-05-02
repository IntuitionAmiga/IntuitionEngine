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
- 320x192 active display, 384x240 with borders
- 128 colors (16 hues × 8 luminances)
- Horizontal/vertical fine scrolling
- WSYNC for raster synchronization
- Implements VideoSource interface for compositor integration
- Copper coprocessor compatible

Register Access:
- IE32/M68K/x86: Direct access at 0xF2100-0xF213F (4-byte aligned)
- Z80/x86: Select/data port access at 0xD4-0xD7
- 6502: Not exposed; the 16-bit address space already uses $D400-$D40F for PSG

Signal Flow:
1. CPU configures ANTIC/GTIA registers
2. ANTIC renders a frame into a compositor-provided buffer
3. Compositor collects frame via GetFrame() and sends to display
*/

package main

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ANTICEngine implements ANTIC video chip as a standalone device.
// Implements VideoSource interface for compositor integration.
type ANTICEngine struct {
	mu  sync.Mutex
	bus *MachineBus

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
	sink  InterruptSink

	// Read-only status
	vcount   uint8  // Vertical counter (scanline/2)
	scanline uint16 // Internal scanline counter

	// Light pen registers (read-only)
	penh uint8
	penv uint8

	// IE-specific extensions
	enabled        atomic.Bool // Video output enabled (lock-free)
	palMode        atomic.Bool // PAL timing selected (lock-free)
	vblankActive   atomic.Bool // VBlank flag (lock-free)
	lastFrameStart int64       // Timestamp of last SignalVSync (for time-based VBlank)
	frameID        uint64
	now            func() int64

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
	playerGfx  [2][4][ANTIC_DISPLAY_HEIGHT]uint8 // [buffer][player][scanline]
	playerPos  [2][4][ANTIC_DISPLAY_HEIGHT]uint8 // [buffer][player][scanline]
	missileGfx [2][4][ANTIC_DISPLAY_HEIGHT]uint8 // [buffer][missile][scanline]
	missilePos [2][4][ANTIC_DISPLAY_HEIGHT]uint8 // [buffer][missile][scanline]

	// Collision latches. Low four bits correspond to PF0..PF3 or P/M0..P/M3.
	missilePF [4]uint8
	playerPF  [4]uint8
	missilePL [4]uint8
	playerPL  [4]uint8

	// Per-scanline color tracking for raster bar effects (double-buffered)
	scanlineColors [2][ANTIC_SCANLINES_NTSC]uint8
	writeBuffer    int  // Buffer being written to by CPU (0 or 1)
	frameReady     bool // Set when a full frame of scanlines has been written

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

// NewANTICEngine creates a new ANTIC video engine instance
func NewANTICEngine(bus *MachineBus) *ANTICEngine {
	antic := &ANTICEngine{
		bus: bus,
		now: func() int64 {
			return time.Now().UnixNano()
		},
	}
	// enabled defaults to false (atomic.Bool zero value)

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

	// Initialize triple buffers for lock-free GetFrame
	bufSize := ANTIC_FRAME_WIDTH * ANTIC_FRAME_HEIGHT * 4
	for i := range antic.frameBufs {
		antic.frameBufs[i] = make([]byte, bufSize)
	}
	antic.writeIdx = 0
	antic.sharedIdx.Store(1)
	antic.readingIdx = 2

	return antic
}

// =============================================================================
// Register Access
// =============================================================================

// HandleRead handles register reads
func (a *ANTICEngine) HandleRead(addr uint32) uint32 {
	a.mu.Lock()
	defer a.mu.Unlock()

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
		return uint32(a.queryScanlineAt(a.now()) / 2)
	case ANTIC_PENH:
		return uint32(a.penh)
	case ANTIC_PENV:
		return uint32(a.penv)
	case ANTIC_NMIEN:
		return uint32(a.nmien)
	case ANTIC_NMIST:
		return uint32(a.nmist)
	case ANTIC_ENABLE:
		value := uint32(0)
		if a.enabled.Load() {
			value |= ANTIC_ENABLE_VIDEO
		}
		if a.palMode.Load() {
			value |= ANTIC_ENABLE_PAL
		}
		return value
	case ANTIC_STATUS:
		if a.inVBlankAt(a.now()) {
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
	case GTIA_M0PF, GTIA_M1PF, GTIA_M2PF, GTIA_M3PF:
		return uint32(a.missilePF[(addr-GTIA_M0PF)/4])
	case GTIA_P0PF, GTIA_P1PF, GTIA_P2PF, GTIA_P3PF:
		return uint32(a.playerPF[(addr-GTIA_P0PF)/4])
	case GTIA_M0PL, GTIA_M1PL, GTIA_M2PL, GTIA_M3PL:
		return uint32(a.missilePL[(addr-GTIA_M0PL)/4])
	case GTIA_P0PL, GTIA_P1PL, GTIA_P2PL, GTIA_P3PL:
		return uint32(a.playerPL[(addr-GTIA_P0PL)/4])
	case GTIA_HITCLR:
		return 0

	default:
		return 0
	}
}

// HandleWrite handles register writes
func (a *ANTICEngine) HandleWrite(addr uint32, value uint32) {
	a.mu.Lock()
	defer a.mu.Unlock()

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
			for p := range 4 {
				a.playerGfx[a.writeBuffer][p][a.scanline] = a.grafp[p]
				a.playerPos[a.writeBuffer][p][a.scanline] = a.hposp[p]
				if a.grafm&(1<<p) != 0 {
					a.missileGfx[a.writeBuffer][p][a.scanline] = 1
				} else {
					a.missileGfx[a.writeBuffer][p][a.scanline] = 0
				}
				a.missilePos[a.writeBuffer][p][a.scanline] = a.hposm[p]
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
		wasEnabled := a.enabled.Load()
		a.enabled.Store((value & ANTIC_ENABLE_VIDEO) != 0)
		a.palMode.Store((value & ANTIC_ENABLE_PAL) != 0)
		// When first enabled, initialize frame timing so VBlank works immediately
		if !wasEnabled && a.enabled.Load() {
			a.lastFrameStart = a.now()
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
		a.capturePlayerPos(0, uint8(value))
	case GTIA_HPOSP1:
		a.hposp[1] = uint8(value)
		a.capturePlayerPos(1, uint8(value))
	case GTIA_HPOSP2:
		a.hposp[2] = uint8(value)
		a.capturePlayerPos(2, uint8(value))
	case GTIA_HPOSP3:
		a.hposp[3] = uint8(value)
		a.capturePlayerPos(3, uint8(value))

	// Missile horizontal positions
	case GTIA_HPOSM0:
		a.hposm[0] = uint8(value)
		a.captureMissilePos(0, uint8(value))
	case GTIA_HPOSM1:
		a.hposm[1] = uint8(value)
		a.captureMissilePos(1, uint8(value))
	case GTIA_HPOSM2:
		a.hposm[2] = uint8(value)
		a.captureMissilePos(2, uint8(value))
	case GTIA_HPOSM3:
		a.hposm[3] = uint8(value)
		a.captureMissilePos(3, uint8(value))

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
		if a.scanline < ANTIC_DISPLAY_HEIGHT {
			for m := range 4 {
				if a.grafm&(1<<m) != 0 {
					a.missileGfx[a.writeBuffer][m][a.scanline] = 1
				} else {
					a.missileGfx[a.writeBuffer][m][a.scanline] = 0
				}
			}
		}
	case GTIA_HITCLR:
		a.clearCollisions()
	}
}

func (a *ANTICEngine) capturePlayerPos(player int, pos uint8) {
	if a.scanline < ANTIC_DISPLAY_HEIGHT {
		a.playerPos[a.writeBuffer][player][a.scanline] = pos
	}
}

func (a *ANTICEngine) captureMissilePos(missile int, pos uint8) {
	if a.scanline < ANTIC_DISPLAY_HEIGHT {
		a.missilePos[a.writeBuffer][missile][a.scanline] = pos
	}
}

func (a *ANTICEngine) clearCollisions() {
	for i := range 4 {
		a.missilePF[i] = 0
		a.playerPF[i] = 0
		a.missilePL[i] = 0
		a.playerPL[i] = 0
	}
}

func (a *ANTICEngine) clearRasterCaptureBuffer(buffer int) {
	for y := range ANTIC_DISPLAY_HEIGHT {
		a.scanlineColors[buffer][y] = a.colbk
		for i := range 4 {
			a.playerGfx[buffer][i][y] = 0
			a.playerPos[buffer][i][y] = 0
			a.missileGfx[buffer][i][y] = 0
			a.missilePos[buffer][i][y] = 0
		}
	}
}

func (a *ANTICEngine) preserveUnwrittenRasterRows(dstBuffer, srcBuffer int, fromScanline uint16) {
	start := int(fromScanline)
	if start < 0 {
		start = 0
	}
	if start > ANTIC_DISPLAY_HEIGHT {
		start = ANTIC_DISPLAY_HEIGHT
	}
	for y := start; y < ANTIC_DISPLAY_HEIGHT; y++ {
		a.scanlineColors[dstBuffer][y] = a.scanlineColors[srcBuffer][y]
		for i := range 4 {
			a.playerGfx[dstBuffer][i][y] = a.playerGfx[srcBuffer][i][y]
			a.playerPos[dstBuffer][i][y] = a.playerPos[srcBuffer][i][y]
			a.missileGfx[dstBuffer][i][y] = a.missileGfx[srcBuffer][i][y]
			a.missilePos[dstBuffer][i][y] = a.missilePos[srcBuffer][i][y]
		}
	}
}

func (a *ANTICEngine) totalScanlines() uint16 {
	if a.palMode.Load() {
		return ANTIC_SCANLINES_PAL
	}
	return ANTIC_SCANLINES_NTSC
}

func (a *ANTICEngine) framePeriodNS() int64 {
	if a.palMode.Load() {
		return int64(time.Second / 50)
	}
	return int64(time.Second / 60)
}

func (a *ANTICEngine) queryScanlineAt(now int64) uint16 {
	frameStart := a.lastFrameStart
	if frameStart == 0 || now <= frameStart {
		return 0
	}
	period := a.framePeriodNS()
	elapsed := (now - frameStart) % period
	return uint16((elapsed * int64(a.totalScanlines())) / period)
}

func (a *ANTICEngine) inVBlankAt(now int64) bool {
	return a.queryScanlineAt(now) >= ANTIC_FRAME_HEIGHT
}

func (a *ANTICEngine) tickFrame(now int64) {
	a.lastFrameStart = now
	a.frameID++
	a.vblankActive.Store(false)

	if a.nmien&ANTIC_NMIEN_VBI != 0 {
		a.nmist |= ANTIC_NMIST_VBI
		if a.sink != nil {
			a.sink.Pulse(IntMaskVBI)
		}
	}
}

func (a *ANTICEngine) SetInterruptSink(sink InterruptSink) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sink = sink
}

// =============================================================================
// Rendering
// =============================================================================

// RenderFrame renders the complete display including border into dst.
func (a *ANTICEngine) RenderFrame(dst []byte) []byte {
	if len(dst) < ANTIC_FRAME_WIDTH*ANTIC_FRAME_HEIGHT*4 {
		dst = make([]byte, ANTIC_FRAME_WIDTH*ANTIC_FRAME_HEIGHT*4)
	}

	// Snapshot state under lock, then render lock-free
	a.mu.Lock()
	readBuffer := 1 - a.writeBuffer
	snapScanlineColors := a.scanlineColors[readBuffer]
	snapPlayerGfx := a.playerGfx[readBuffer]
	snapPlayerPos := a.playerPos[readBuffer]
	snapMissileGfx := a.missileGfx[readBuffer]
	snapMissilePos := a.missilePos[readBuffer]
	snapGractl := a.gractl
	snapSizep := a.sizep
	snapSizem := a.sizem
	snapColpm := a.colpm
	snapColpf := a.colpf
	snapColbk := a.colbk
	snapPrior := a.prior
	a.mu.Unlock()

	// Render per-scanline colors for raster bar effects
	// The frame is ANTIC_FRAME_WIDTH x ANTIC_FRAME_HEIGHT (384x240)
	for y := range ANTIC_FRAME_HEIGHT {
		var color byte
		if y < ANTIC_BORDER_TOP || y >= ANTIC_FRAME_HEIGHT-ANTIC_BORDER_BOTTOM {
			// Border rows use background color
			color = snapColbk
		} else {
			// Active display: use per-scanline color
			virtualScanline := y - ANTIC_BORDER_TOP
			color = snapScanlineColors[virtualScanline]
		}
		rowStart := y * ANTIC_FRAME_WIDTH * 4

		// Fill entire row with this color using pre-packed RGBA
		colorRGBA := ANTICPaletteRGBA[color][:]
		for x := range ANTIC_FRAME_WIDTH {
			offset := rowStart + x*4
			copy(dst[offset:offset+4], colorRGBA)
		}
	}

	pfMask := make([]uint8, ANTIC_FRAME_WIDTH*ANTIC_FRAME_HEIGHT)
	a.renderDisplayList(dst, pfMask)

	a.renderPMG(dst, pmgSnapshot{
		gractl:     snapGractl,
		prior:      snapPrior,
		sizep:      snapSizep,
		sizem:      snapSizem,
		colpm:      snapColpm,
		colpf:      snapColpf,
		playerGfx:  snapPlayerGfx,
		playerPos:  snapPlayerPos,
		missileGfx: snapMissileGfx,
		missilePos: snapMissilePos,
		pfMask:     pfMask,
	})

	return dst
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

// GetFrame returns the current rendered frame via lock-free triple-buffer swap.
func (a *ANTICEngine) GetFrame() []byte {
	if !a.IsEnabled() {
		return nil
	}
	newRead := int(a.sharedIdx.Swap(int32(a.readingIdx)))
	a.readingIdx = newRead
	return a.frameBufs[a.readingIdx]
}

// IsEnabled returns whether ANTIC video is active (lock-free)
func (a *ANTICEngine) IsEnabled() bool {
	return a.enabled.Load()
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
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.scanline > 0 {
		a.tickFrame(a.now())
		return
	}
	a.scanline = 0
	a.clearRasterCaptureBuffer(a.writeBuffer)
	a.tickFrame(a.now())
}

// =============================================================================
// Independent Render Goroutine
// =============================================================================

// SetCompositorManaged implements CompositorManageable.
func (a *ANTICEngine) SetCompositorManaged(managed bool) {
	a.compositorManaged.Store(managed)
}

// WaitRenderIdle implements CompositorManageable.
func (a *ANTICEngine) WaitRenderIdle() {
	for a.rendering.Load() {
		runtime.Gosched()
	}
}

// StartRenderLoop spawns a 60Hz render goroutine for lock-free GetFrame.
func (a *ANTICEngine) StartRenderLoop() {
	a.renderMu.Lock()
	defer a.renderMu.Unlock()
	if a.renderRunning.Load() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.renderCancel = cancel
	done := make(chan struct{})
	a.renderDone = done
	a.renderRunning.Store(true)
	go a.renderLoop(ctx, done)
}

// StopRenderLoop stops the render goroutine and waits for it to exit.
func (a *ANTICEngine) StopRenderLoop() {
	a.renderMu.Lock()
	if !a.renderRunning.Swap(false) {
		a.renderMu.Unlock()
		return
	}
	cancel := a.renderCancel
	done := a.renderDone
	a.renderMu.Unlock()
	cancel()
	<-done
}

// renderLoop runs at 60Hz, rendering frames into the triple buffer.
// done is goroutine-local to avoid close-of-wrong-channel on restart.
func (a *ANTICEngine) renderLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(COMPOSITOR_REFRESH_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !a.enabled.Load() || a.compositorManaged.Load() {
				continue
			}
			a.rendering.Store(true)
			if a.compositorManaged.Load() {
				a.rendering.Store(false)
				continue
			}
			a.RenderFrame(a.frameBufs[a.writeIdx])
			a.rendering.Store(false)
			a.writeIdx = int(a.sharedIdx.Swap(int32(a.writeIdx)))
		}
	}
}
