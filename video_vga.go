// video_vga.go - IBM VGA chip emulation for Intuition Engine

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
video_vga.go - IBM VGA Video Chip Emulation (Standalone)

This module implements IBM VGA compatible video modes as a standalone video device:
- Mode 13h: 320x200, 256 colors, linear memory
- Mode 12h: 640x480, 16 colors, planar memory
- Mode 03h: 80x25 text mode, 16 colors
- Mode X: 320x240, 256 colors, planar (unchained)

Features:
- Full DAC/palette support with 6-bit to 8-bit expansion
- Planar memory model with map mask and bit mask
- Text mode with embedded 8x16 VGA font
- Page flipping via CRTC start address
- VSync status for timing synchronization
- Implements VideoSource interface for compositor integration

Signal Flow:
1. CPU writes to VGA registers (mode, palette, etc.)
2. CPU writes to VRAM (linear or planar depending on mode)
3. VGA renders VRAM through palette to framebuffer
4. Compositor collects frame via GetFrame() and sends to display
*/

package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// VGA timing constants
const (
	VGA_REFRESH_RATE     = 60
	VGA_REFRESH_INTERVAL = time.Second / VGA_REFRESH_RATE
)

// VGA layer constant for compositor
const VGA_LAYER = 10 // VGA renders on top of VideoChip (layer 0)

// VGAEngine implements IBM VGA compatible video as a standalone device
// Implements VideoSource interface for compositor integration
type VGAEngine struct {
	mu    sync.Mutex
	bus   *MachineBus
	layer int // Z-order for compositor (higher = on top)

	// Current mode
	mode    uint8
	control uint8
	status  uint8

	// DAC state machine
	dacWriteIndex uint8
	dacReadIndex  uint8
	dacWritePhase uint8 // 0=R, 1=G, 2=B
	dacReadPhase  uint8
	dacMask       uint8

	// Palette (256 entries x 3 components, 6-bit values)
	palette [VGA_PALETTE_SIZE * 3]uint8

	// Sequencer registers
	seqIndex uint8
	seqRegs  [VGA_SEQ_REG_COUNT]uint8

	// CRTC registers
	crtcIndex uint8
	crtcRegs  [VGA_CRTC_REG_COUNT]uint8

	// Graphics Controller registers
	gcIndex uint8
	gcRegs  [VGA_GC_REG_COUNT]uint8

	// Attribute Controller registers
	attrIndex uint8
	attrRegs  [VGA_ATTR_REG_COUNT]uint8
	attrFlip  bool // Index/data flip-flop

	// VRAM (4 planes x 64KB for planar modes)
	vram [VGA_PLANE_COUNT][VGA_PLANE_SIZE]uint8

	// Text buffer (separate from graphics VRAM)
	textBuffer [VGA_TEXT_SIZE]uint8

	// Latches for planar reads
	latch [4]uint8

	// Pre-expanded RGBA palette cache (256 entries, 4 bytes each: R, G, B, A)
	// Invalidated on any palette write, rebuilt on first read after invalidation
	paletteRGBA  [256][4]uint8
	paletteU32   [256]uint32 // Packed LE uint32 for direct framebuffer stores
	paletteDirty bool        // True when palette cache needs rebuilding

	// Pre-allocated framebuffers for each mode to avoid per-frame allocation
	frameBuffer13h []uint8 // 320x200x4 = 256,000 bytes
	frameBuffer12h []uint8 // 640x480x4 = 1,228,800 bytes
	frameBufferX   []uint8 // 320x240x4 = 307,200 bytes
	frameBufferTxt []uint8 // 640x400x4 = 1,024,000 bytes

	// Optional render target — when non-nil, render functions write here
	// instead of into the mode-specific internal buffer.
	renderTarget []byte

	// VSync state
	vsync      atomic.Bool
	frameStart atomic.Int64 // Unix nano timestamp of frame start for time-based vsync

	// Lock-free enable flag
	enabled atomic.Bool

	// Per-scanline render buffer (used by ScanlineAware interface)
	scanlineFrame []byte

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

// NewVGAEngine creates a new VGA engine instance as a standalone video device
func NewVGAEngine(bus *MachineBus) *VGAEngine {
	vga := &VGAEngine{
		bus:          bus,
		layer:        VGA_LAYER, // VGA renders on top
		dacMask:      0xFF,      // Default: all bits enabled
		paletteDirty: true,      // Force initial cache build
	}

	// Initialize sequencer defaults
	vga.seqRegs[VGA_SEQ_MAPMASK_R] = 0x0F                 // All planes enabled
	vga.seqRegs[VGA_SEQ_MEMMODE] = VGA_SEQ_MEMMODE_CHAIN4 // Chain-4 for Mode 13h default

	// Initialize GC defaults
	vga.gcRegs[VGA_GC_BITMASK_R] = 0xFF // All bits enabled

	// Initialize default VGA palette (standard 16-color + grayscale)
	vga.initDefaultPalette()

	// Initialize triple buffers for lock-free GetFrame (largest mode: 640x480)
	bufSize := VGA_MODE12H_WIDTH * VGA_MODE12H_HEIGHT * 4
	for i := range vga.frameBufs {
		vga.frameBufs[i] = make([]byte, bufSize)
	}
	vga.writeIdx = 0
	vga.sharedIdx.Store(1)
	vga.readingIdx = 2

	return vga
}

// initDefaultPalette sets up the standard VGA 256-color palette
func (v *VGAEngine) initDefaultPalette() {
	// Standard VGA 16-color palette (indices 0-15)
	standardColors := [][3]uint8{
		{0, 0, 0},    // 0: Black
		{0, 0, 42},   // 1: Blue
		{0, 42, 0},   // 2: Green
		{0, 42, 42},  // 3: Cyan
		{42, 0, 0},   // 4: Red
		{42, 0, 42},  // 5: Magenta
		{42, 21, 0},  // 6: Brown
		{42, 42, 42}, // 7: Light Gray
		{21, 21, 21}, // 8: Dark Gray
		{21, 21, 63}, // 9: Light Blue
		{21, 63, 21}, // 10: Light Green
		{21, 63, 63}, // 11: Light Cyan
		{63, 21, 21}, // 12: Light Red
		{63, 21, 63}, // 13: Light Magenta
		{63, 63, 21}, // 14: Yellow
		{63, 63, 63}, // 15: White
	}

	for i, c := range standardColors {
		v.palette[i*3+0] = c[0]
		v.palette[i*3+1] = c[1]
		v.palette[i*3+2] = c[2]
	}

	// Colors 16-231: 6x6x6 color cube
	idx := 16
	for r := range 6 {
		for g := range 6 {
			for b := range 6 {
				v.palette[idx*3+0] = uint8(r * 63 / 5)
				v.palette[idx*3+1] = uint8(g * 63 / 5)
				v.palette[idx*3+2] = uint8(b * 63 / 5)
				idx++
			}
		}
	}

	// Colors 232-255: grayscale ramp
	for i := range 24 {
		gray := uint8(i * 63 / 23)
		v.palette[idx*3+0] = gray
		v.palette[idx*3+1] = gray
		v.palette[idx*3+2] = gray
		idx++
	}

	// Mark palette cache as dirty to force rebuild on first use
	v.paletteDirty = true
}

// HandleRead handles register reads
func (v *VGAEngine) HandleRead(addr uint32) uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()

	switch addr {
	case VGA_MODE:
		return uint32(v.mode)
	case VGA_STATUS:
		// Calculate vsync based on time elapsed since frame start
		// Self-resetting: automatically starts new frame when period elapses
		now := time.Now().UnixNano()
		frameStart := v.frameStart.Load()
		if frameStart == 0 {
			// Not initialized yet
			v.frameStart.Store(now)
			frameStart = now
		}
		elapsed := time.Duration(now - frameStart)
		// Auto-reset frame timer if we've passed a full frame
		if elapsed >= VGA_REFRESH_INTERVAL {
			v.frameStart.Store(now)
			elapsed = 0
		}
		// VSync active during last 10% of frame period (~1.6ms at 60Hz)
		inVSync := elapsed >= (VGA_REFRESH_INTERVAL * 9 / 10)
		status := v.status &^ (VGA_STATUS_VSYNC | VGA_STATUS_RETRACE)
		if inVSync {
			status |= VGA_STATUS_VSYNC | VGA_STATUS_RETRACE
		}
		return uint32(status)
	case VGA_CTRL:
		return uint32(v.control)

	// Sequencer
	case VGA_SEQ_INDEX:
		return uint32(v.seqIndex)
	case VGA_SEQ_DATA:
		if v.seqIndex < VGA_SEQ_REG_COUNT {
			return uint32(v.seqRegs[v.seqIndex])
		}
		return 0
	case VGA_SEQ_MAPMASK:
		return uint32(v.seqRegs[VGA_SEQ_MAPMASK_R])

	// CRTC
	case VGA_CRTC_INDEX:
		return uint32(v.crtcIndex)
	case VGA_CRTC_DATA:
		if v.crtcIndex < VGA_CRTC_REG_COUNT {
			return uint32(v.crtcRegs[v.crtcIndex])
		}
		return 0
	case VGA_CRTC_STARTHI:
		return uint32(v.crtcRegs[VGA_CRTC_START_HI])
	case VGA_CRTC_STARTLO:
		return uint32(v.crtcRegs[VGA_CRTC_START_LO])

	// Graphics Controller
	case VGA_GC_INDEX:
		return uint32(v.gcIndex)
	case VGA_GC_DATA:
		if v.gcIndex < VGA_GC_REG_COUNT {
			return uint32(v.gcRegs[v.gcIndex])
		}
		return 0
	case VGA_GC_READMAP:
		return uint32(v.gcRegs[VGA_GC_READ_MAP_R])
	case VGA_GC_BITMASK:
		return uint32(v.gcRegs[VGA_GC_BITMASK_R])

	// Attribute Controller
	case VGA_ATTR_INDEX:
		return uint32(v.attrIndex)
	case VGA_ATTR_DATA:
		if v.attrIndex < VGA_ATTR_REG_COUNT {
			return uint32(v.attrRegs[v.attrIndex])
		}
		return 0

	// DAC
	case VGA_DAC_MASK:
		return uint32(v.dacMask)
	case VGA_DAC_RINDEX:
		return uint32(v.dacReadIndex)
	case VGA_DAC_WINDEX:
		return uint32(v.dacWriteIndex)
	case VGA_DAC_DATA:
		return v.readDACData()
	}

	// Palette RAM direct access
	if addr >= VGA_PALETTE && addr <= VGA_PALETTE_END {
		offset := addr - VGA_PALETTE
		if offset < VGA_PALETTE_BYTES {
			return uint32(v.palette[offset])
		}
	}

	return 0
}

// HandleWrite handles register writes
func (v *VGAEngine) HandleWrite(addr uint32, value uint32) {
	v.mu.Lock()
	defer v.mu.Unlock()

	switch addr {
	case VGA_MODE:
		v.setMode(uint8(value))
	case VGA_STATUS:
		// Status is read-only
	case VGA_CTRL:
		v.control = uint8(value)
		v.enabled.Store(v.control&VGA_CTRL_ENABLE != 0)

	// Sequencer
	case VGA_SEQ_INDEX:
		v.seqIndex = uint8(value)
	case VGA_SEQ_DATA:
		if v.seqIndex < VGA_SEQ_REG_COUNT {
			v.seqRegs[v.seqIndex] = uint8(value)
		}
	case VGA_SEQ_MAPMASK:
		v.seqRegs[VGA_SEQ_MAPMASK_R] = uint8(value)

	// CRTC
	case VGA_CRTC_INDEX:
		v.crtcIndex = uint8(value)
	case VGA_CRTC_DATA:
		if v.crtcIndex < VGA_CRTC_REG_COUNT {
			v.crtcRegs[v.crtcIndex] = uint8(value)
		}
	case VGA_CRTC_STARTHI:
		v.crtcRegs[VGA_CRTC_START_HI] = uint8(value)
	case VGA_CRTC_STARTLO:
		v.crtcRegs[VGA_CRTC_START_LO] = uint8(value)

	// Graphics Controller
	case VGA_GC_INDEX:
		v.gcIndex = uint8(value)
	case VGA_GC_DATA:
		if v.gcIndex < VGA_GC_REG_COUNT {
			v.gcRegs[v.gcIndex] = uint8(value)
		}
	case VGA_GC_READMAP:
		v.gcRegs[VGA_GC_READ_MAP_R] = uint8(value)
	case VGA_GC_BITMASK:
		v.gcRegs[VGA_GC_BITMASK_R] = uint8(value)

	// Attribute Controller
	case VGA_ATTR_INDEX:
		v.attrIndex = uint8(value)
	case VGA_ATTR_DATA:
		if v.attrIndex < VGA_ATTR_REG_COUNT {
			v.attrRegs[v.attrIndex] = uint8(value)
		}

	// DAC
	case VGA_DAC_MASK:
		v.dacMask = uint8(value)
	case VGA_DAC_RINDEX:
		v.dacReadIndex = uint8(value)
		v.dacReadPhase = 0
	case VGA_DAC_WINDEX:
		v.dacWriteIndex = uint8(value)
		v.dacWritePhase = 0
	case VGA_DAC_DATA:
		v.writeDACData(uint8(value))
	}

	// Palette RAM direct access
	if addr >= VGA_PALETTE && addr <= VGA_PALETTE_END {
		offset := addr - VGA_PALETTE
		if offset < VGA_PALETTE_BYTES {
			v.palette[offset] = uint8(value)
			v.paletteDirty = true // Invalidate cache on any palette write
		}
	}
}

// setMode configures VGA for the specified mode
func (v *VGAEngine) setMode(mode uint8) {
	v.mode = mode

	switch mode {
	case VGA_MODE_13H:
		// 320x200, 256 colors, linear (Chain-4)
		v.seqRegs[VGA_SEQ_MEMMODE] = VGA_SEQ_MEMMODE_CHAIN4 | VGA_SEQ_MEMMODE_EXT
		v.seqRegs[VGA_SEQ_MAPMASK_R] = 0x0F

	case VGA_MODE_12H:
		// 640x480, 16 colors, planar
		v.seqRegs[VGA_SEQ_MEMMODE] = VGA_SEQ_MEMMODE_EXT
		v.seqRegs[VGA_SEQ_MAPMASK_R] = 0x0F

	case VGA_MODE_TEXT:
		// 80x25 text mode
		v.seqRegs[VGA_SEQ_MEMMODE] = 0

	case VGA_MODE_X:
		// 320x240, 256 colors, planar (unchained)
		v.seqRegs[VGA_SEQ_MEMMODE] = VGA_SEQ_MEMMODE_EXT // No Chain-4
		v.seqRegs[VGA_SEQ_MAPMASK_R] = 0x0F
	}
}

// writeDACData writes a component to the DAC palette
func (v *VGAEngine) writeDACData(value uint8) {
	// Clamp to 6-bit value
	value &= 0x3F

	idx := int(v.dacWriteIndex)*3 + int(v.dacWritePhase)
	if idx < len(v.palette) {
		v.palette[idx] = value
		v.paletteDirty = true // Invalidate cache on any palette write
	}

	v.dacWritePhase++
	if v.dacWritePhase >= 3 {
		v.dacWritePhase = 0
		v.dacWriteIndex++
	}
}

// readDACData reads a component from the DAC palette
func (v *VGAEngine) readDACData() uint32 {
	idx := int(v.dacReadIndex)*3 + int(v.dacReadPhase)
	var value uint8
	if idx < len(v.palette) {
		value = v.palette[idx]
	}

	v.dacReadPhase++
	if v.dacReadPhase >= 3 {
		v.dacReadPhase = 0
		v.dacReadIndex++
	}

	return uint32(value)
}

// HandleVRAMRead handles reads from VRAM window
func (v *VGAEngine) HandleVRAMRead(addr uint32) uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()

	offset := addr - VGA_VRAM_WINDOW
	if offset >= VGA_VRAM_SIZE {
		return 0
	}

	if v.IsLinearMode() {
		// Chain-4 linear mode (Mode 13h)
		plane := offset & 3
		vramOffset := offset >> 2
		if vramOffset < VGA_PLANE_SIZE {
			return uint32(v.vram[plane][vramOffset])
		}
	} else {
		// Planar mode - use read map select
		plane := v.gcRegs[VGA_GC_READ_MAP_R] & 3

		// Load all latches (for potential read mode 1)
		for p := range 4 {
			if offset < VGA_PLANE_SIZE {
				v.latch[p] = v.vram[p][offset]
			}
		}

		if offset < VGA_PLANE_SIZE {
			return uint32(v.vram[plane][offset])
		}
	}

	return 0
}

// HandleVRAMWrite handles writes to VRAM window
func (v *VGAEngine) HandleVRAMWrite(addr uint32, value uint32) {
	v.mu.Lock()
	defer v.mu.Unlock()

	offset := addr - VGA_VRAM_WINDOW
	if offset >= VGA_VRAM_SIZE {
		return
	}

	if v.IsLinearMode() {
		// Chain-4 linear mode (Mode 13h)
		plane := offset & 3
		vramOffset := offset >> 2
		if vramOffset < VGA_PLANE_SIZE {
			v.vram[plane][vramOffset] = uint8(value)
		}
	} else {
		// Planar mode
		mapMask := v.seqRegs[VGA_SEQ_MAPMASK_R]
		bitMask := v.gcRegs[VGA_GC_BITMASK_R]

		for plane := range 4 {
			if mapMask&(1<<plane) != 0 && offset < VGA_PLANE_SIZE {
				if bitMask == 0xFF {
					// Full byte write
					v.vram[plane][offset] = uint8(value)
				} else {
					// Partial write with bit mask
					existing := v.vram[plane][offset]
					v.vram[plane][offset] = (existing & ^bitMask) | (uint8(value) & bitMask)
				}
			}
		}
	}
}

// HandleTextRead handles reads from text buffer
func (v *VGAEngine) HandleTextRead(addr uint32) uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()

	offset := addr - VGA_TEXT_WINDOW
	if offset < VGA_TEXT_SIZE {
		return uint32(v.textBuffer[offset])
	}
	return 0
}

// HandleTextWrite handles writes to text buffer
func (v *VGAEngine) HandleTextWrite(addr uint32, value uint32) {
	v.mu.Lock()
	defer v.mu.Unlock()

	offset := addr - VGA_TEXT_WINDOW
	if offset < VGA_TEXT_SIZE {
		v.textBuffer[offset] = uint8(value)
	}
}

// IsLinearMode returns true if Chain-4 is enabled (Mode 13h)
func (v *VGAEngine) IsLinearMode() bool {
	return v.seqRegs[VGA_SEQ_MEMMODE]&VGA_SEQ_MEMMODE_CHAIN4 != 0
}

// GetModeDimensions returns width and height for current mode
func (v *VGAEngine) GetModeDimensions() (int, int) {
	switch v.mode {
	case VGA_MODE_13H:
		return VGA_MODE13H_WIDTH, VGA_MODE13H_HEIGHT
	case VGA_MODE_12H:
		return VGA_MODE12H_WIDTH, VGA_MODE12H_HEIGHT
	case VGA_MODE_X:
		return VGA_MODEX_WIDTH, VGA_MODEX_HEIGHT
	case VGA_MODE_TEXT:
		return VGA_TEXT_COLS * VGA_FONT_WIDTH, VGA_TEXT_ROWS * VGA_FONT_HEIGHT
	default:
		return VGA_MODE13H_WIDTH, VGA_MODE13H_HEIGHT
	}
}

// GetTextDimensions returns text mode dimensions in characters
func (v *VGAEngine) GetTextDimensions() (int, int) {
	return VGA_TEXT_COLS, VGA_TEXT_ROWS
}

// GetPaletteEntry returns RGB values for a palette entry (6-bit values)
func (v *VGAEngine) GetPaletteEntry(index uint8) (uint8, uint8, uint8) {
	v.mu.Lock()
	defer v.mu.Unlock()

	idx := int(index) * 3
	return v.palette[idx], v.palette[idx+1], v.palette[idx+2]
}

// SetPaletteEntry sets RGB values for a palette entry (6-bit values)
func (v *VGAEngine) SetPaletteEntry(index uint8, r, g, b uint8) {
	v.mu.Lock()
	defer v.mu.Unlock()

	idx := int(index) * 3
	v.palette[idx] = r & 0x3F
	v.palette[idx+1] = g & 0x3F
	v.palette[idx+2] = b & 0x3F
	v.paletteDirty = true // Invalidate cache
}

// ApplyDACMask applies the DAC pixel mask to an index
func (v *VGAEngine) ApplyDACMask(index uint8) uint8 {
	return index & v.dacMask
}

// Expand6BitTo8Bit converts a 6-bit VGA value to 8-bit
func (v *VGAEngine) Expand6BitTo8Bit(val uint8) uint8 {
	// Standard expansion: (val << 2) | (val >> 4)
	return (val << 2) | (val >> 4)
}

// GetPaletteRGBA returns expanded 8-bit RGBA for a palette entry
func (v *VGAEngine) GetPaletteRGBA(index uint8) (uint8, uint8, uint8, uint8) {
	r, g, b := v.GetPaletteEntry(index)
	return v.Expand6BitTo8Bit(r), v.Expand6BitTo8Bit(g), v.Expand6BitTo8Bit(b), 255
}

// ReadPlane reads a byte from a specific plane at an offset
func (v *VGAEngine) ReadPlane(offset uint32, plane int) uint8 {
	v.mu.Lock()
	defer v.mu.Unlock()

	if plane >= 0 && plane < 4 && offset < VGA_PLANE_SIZE {
		return v.vram[plane][offset]
	}
	return 0
}

// GetStartAddress returns the display start address from CRTC
func (v *VGAEngine) GetStartAddress() uint32 {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.getStartAddressInternal()
}

// getStartAddressInternal returns start address without locking (for internal use)
func (v *VGAEngine) getStartAddressInternal() uint32 {
	return uint32(v.crtcRegs[VGA_CRTC_START_HI])<<8 | uint32(v.crtcRegs[VGA_CRTC_START_LO])
}

// GetCursorPosition returns cursor column and row
func (v *VGAEngine) GetCursorPosition() (int, int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	offset := uint16(v.crtcRegs[VGA_CRTC_CURSOR_HI])<<8 | uint16(v.crtcRegs[VGA_CRTC_CURSOR_LO])
	col := int(offset % VGA_TEXT_COLS)
	row := int(offset / VGA_TEXT_COLS)
	return col, row
}

// SetVSync sets the vsync status flag
func (v *VGAEngine) SetVSync(active bool) {
	v.vsync.Store(active)

	v.mu.Lock()
	defer v.mu.Unlock()

	if active {
		v.status |= VGA_STATUS_VSYNC | VGA_STATUS_RETRACE
	} else {
		v.status &^= VGA_STATUS_VSYNC | VGA_STATUS_RETRACE
	}
}

// GetFontGlyph returns the font data for a character
func (v *VGAEngine) GetFontGlyph(char uint8) []uint8 {
	idx := int(char) * VGA_FONT_HEIGHT
	if idx+VGA_FONT_HEIGHT <= len(vgaFont8x16) {
		return vgaFont8x16[idx : idx+VGA_FONT_HEIGHT]
	}
	return make([]uint8, VGA_FONT_HEIGHT)
}

// RenderFrameTo renders the current mode directly into dst, avoiding a copy.
func (v *VGAEngine) RenderFrameTo(dst []byte) {
	v.renderTarget = dst
	v.RenderFrame()
	v.renderTarget = nil
}

// RenderFrame renders the current mode to a framebuffer
func (v *VGAEngine) RenderFrame() []uint8 {
	// Note: No lock acquired here - VRAM is fixed-size and minor racing is
	// acceptable for video rendering (like real VGA hardware behavior)

	switch v.mode {
	case VGA_MODE_13H:
		return v.renderMode13h()
	case VGA_MODE_12H:
		return v.renderMode12h()
	case VGA_MODE_TEXT:
		return v.renderTextMode()
	case VGA_MODE_X:
		return v.renderModeX()
	default:
		return v.renderMode13h()
	}
}

// renderMode13h renders 320x200x256 linear mode
func (v *VGAEngine) renderMode13h() []uint8 {
	width := VGA_MODE13H_WIDTH
	height := VGA_MODE13H_HEIGHT

	// Allocate framebuffer once, reuse on subsequent frames
	if v.frameBuffer13h == nil {
		v.frameBuffer13h = make([]uint8, width*height*4)
	}
	fb := v.frameBuffer13h
	if v.renderTarget != nil {
		fb = v.renderTarget
	}

	// Rebuild palette cache once at frame start if dirty
	if v.paletteDirty {
		v.rebuildPaletteCache()
	}

	startAddr := v.getStartAddressInternal()
	dacMask := v.dacMask

	for y := range height {
		rowLinear := uint32(y*width) + startAddr
		pixelBase := y * width * 4
		for x := range width {
			// Linear addressing with start address offset
			linearOffset := rowLinear + uint32(x)

			// Chain-4: address bits 0-1 select plane, bits 2+ are VRAM offset
			plane := linearOffset & 3
			vramOffset := linearOffset >> 2

			var colorIndex uint8
			if vramOffset < VGA_PLANE_SIZE {
				colorIndex = v.vram[plane][vramOffset]
			}

			// Apply DAC mask and get color from cache
			colorIndex &= dacMask

			pixelIdx := pixelBase + x*4
			*(*uint32)(unsafe.Pointer(&fb[pixelIdx])) = v.paletteU32[colorIndex]
		}
	}

	return fb
}

// renderMode12h renders 640x480x16 planar mode
func (v *VGAEngine) renderMode12h() []uint8 {
	width := VGA_MODE12H_WIDTH
	height := VGA_MODE12H_HEIGHT

	// Allocate framebuffer once, reuse on subsequent frames
	if v.frameBuffer12h == nil {
		v.frameBuffer12h = make([]uint8, width*height*4)
	}
	fb := v.frameBuffer12h
	if v.renderTarget != nil {
		fb = v.renderTarget
	}

	// Rebuild palette cache once at frame start if dirty
	if v.paletteDirty {
		v.rebuildPaletteCache()
	}

	bytesPerLine := width / 8
	dacMask := v.dacMask

	for y := range height {
		rowBase := y * width * 4
		for byteX := range bytesPerLine {
			offset := y*bytesPerLine + byteX

			// Get all 4 planes for this byte
			var planes [4]uint8
			if offset < int(VGA_PLANE_SIZE) {
				planes[0] = v.vram[0][offset]
				planes[1] = v.vram[1][offset]
				planes[2] = v.vram[2][offset]
				planes[3] = v.vram[3][offset]
			}

			// Extract 8 pixels from the planes (branchless)
			pixelBase := rowBase + byteX*8*4
			for bit := 7; bit >= 0; bit-- {
				ubit := uint(bit)
				colorIndex := ((planes[0] >> ubit) & 1) |
					(((planes[1] >> ubit) & 1) << 1) |
					(((planes[2] >> ubit) & 1) << 2) |
					(((planes[3] >> ubit) & 1) << 3)

				colorIndex &= dacMask
				pixelIdx := pixelBase + (7-bit)*4
				*(*uint32)(unsafe.Pointer(&fb[pixelIdx])) = v.paletteU32[colorIndex]
			}
		}
	}

	return fb
}

// renderModeX renders 320x240x256 planar (unchained) mode
func (v *VGAEngine) renderModeX() []uint8 {
	width := VGA_MODEX_WIDTH
	height := VGA_MODEX_HEIGHT

	// Allocate framebuffer once, reuse on subsequent frames
	if v.frameBufferX == nil {
		v.frameBufferX = make([]uint8, width*height*4)
	}
	fb := v.frameBufferX
	if v.renderTarget != nil {
		fb = v.renderTarget
	}

	// Rebuild palette cache once at frame start if dirty
	if v.paletteDirty {
		v.rebuildPaletteCache()
	}

	startAddr := v.getStartAddressInternal()
	dacMask := v.dacMask
	widthDiv4 := width / 4

	for y := range height {
		rowStart := y * width
		yOffset := uint32(y * widthDiv4)
		for x := range width {
			// Unchained: pixel X determines plane, Y*width/4 + X/4 is offset
			plane := x & 3
			offset := yOffset + uint32(x/4) + startAddr

			var colorIndex uint8
			if offset < VGA_PLANE_SIZE {
				colorIndex = v.vram[plane][offset]
			}

			colorIndex &= dacMask
			pixelIdx := (rowStart + x) * 4
			*(*uint32)(unsafe.Pointer(&fb[pixelIdx])) = v.paletteU32[colorIndex]
		}
	}

	return fb
}

// renderTextMode renders 80x25 text mode
func (v *VGAEngine) renderTextMode() []uint8 {
	charWidth := VGA_FONT_WIDTH
	charHeight := VGA_FONT_HEIGHT
	width := VGA_TEXT_COLS * charWidth
	height := VGA_TEXT_ROWS * charHeight

	// Allocate framebuffer once, reuse on subsequent frames
	if v.frameBufferTxt == nil {
		v.frameBufferTxt = make([]uint8, width*height*4)
	}
	fb := v.frameBufferTxt
	if v.renderTarget != nil {
		fb = v.renderTarget
	}

	// Rebuild palette cache once at frame start if dirty
	if v.paletteDirty {
		v.rebuildPaletteCache()
	}

	for row := range VGA_TEXT_ROWS {
		for col := range VGA_TEXT_COLS {
			// Get character and attribute from text buffer
			bufOffset := (row*VGA_TEXT_COLS + col) * 2
			char := v.textBuffer[bufOffset]
			attr := v.textBuffer[bufOffset+1]

			// Extract foreground/background from attribute
			fg := attr & 0x0F
			bg := (attr >> 4) & 0x0F

			// Get font glyph
			glyphOffset := int(char) * charHeight
			charBaseX := col * charWidth
			charBaseY := row * charHeight

			// Render character
			for cy := range charHeight {
				fontRow := vgaFont8x16[glyphOffset+cy]
				pixelY := charBaseY + cy
				rowBase := pixelY * width * 4

				fgU32 := v.paletteU32[fg]
				bgU32 := v.paletteU32[bg]
				for cx := range charWidth {
					pixelX := charBaseX + cx
					pixelIdx := rowBase + pixelX*4

					if fontRow&(0x80>>cx) != 0 {
						*(*uint32)(unsafe.Pointer(&fb[pixelIdx])) = fgU32
					} else {
						*(*uint32)(unsafe.Pointer(&fb[pixelIdx])) = bgU32
					}
				}
			}
		}
	}

	return fb
}

// rebuildPaletteCache rebuilds the pre-expanded RGBA palette cache
func (v *VGAEngine) rebuildPaletteCache() {
	for i := range 256 {
		idx := i * 3
		r := v.Expand6BitTo8Bit(v.palette[idx])
		g := v.Expand6BitTo8Bit(v.palette[idx+1])
		b := v.Expand6BitTo8Bit(v.palette[idx+2])
		v.paletteRGBA[i][0] = r
		v.paletteRGBA[i][1] = g
		v.paletteRGBA[i][2] = b
		v.paletteRGBA[i][3] = 255
		v.paletteU32[i] = uint32(r) | uint32(g)<<8 | uint32(b)<<16 | 0xFF000000
	}
	v.paletteDirty = false
}

// getPaletteRGBAInternal returns expanded 8-bit RGBA (internal, no lock)
// Uses pre-computed cache for fast palette lookups during rendering
func (v *VGAEngine) getPaletteRGBAInternal(index uint8) (uint8, uint8, uint8, uint8) {
	if v.paletteDirty {
		v.rebuildPaletteCache()
	}
	c := v.paletteRGBA[index]
	return c[0], c[1], c[2], c[3]
}

var vgaFrameCount int
var vgaDebugFile *os.File

func vgaDebugLog(format string, args ...any) {
	if vgaDebugFile == nil {
		var err error
		vgaDebugFile, err = os.Create("/tmp/vga_debug.log")
		if err != nil {
			return
		}
	}
	fmt.Fprintf(vgaDebugFile, format, args...)
	vgaDebugFile.Sync()
}

// -----------------------------------------------------------------------------
// VideoSource Interface Implementation
// -----------------------------------------------------------------------------

// GetFrame implements VideoSource - lock-free triple-buffer swap
// Called by compositor each frame to collect video output
func (v *VGAEngine) GetFrame() []byte {
	if !v.enabled.Load() {
		return nil
	}
	newRead := int(v.sharedIdx.Swap(int32(v.readingIdx)))
	v.readingIdx = newRead
	return v.frameBufs[v.readingIdx]
}

// IsEnabled implements VideoSource - returns whether VGA is enabled (lock-free)
func (v *VGAEngine) IsEnabled() bool {
	return v.enabled.Load()
}

// GetLayer implements VideoSource - returns Z-order for compositing
func (v *VGAEngine) GetLayer() int {
	return v.layer
}

// GetDimensions implements VideoSource - returns frame dimensions
func (v *VGAEngine) GetDimensions() (int, int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.GetModeDimensions()
}

// SignalVSync implements VideoSource - called by compositor after frame sent (lock-free)
func (v *VGAEngine) SignalVSync() {
	v.vsync.Store(true)
	v.frameStart.Store(time.Now().UnixNano())
}

// GetCurrentFramebuffer returns the current VGA framebuffer for testing
func (v *VGAEngine) GetCurrentFramebuffer() []uint8 {
	return v.RenderFrame()
}

// -----------------------------------------------------------------------------
// ScanlineAware Interface Implementation
// -----------------------------------------------------------------------------

// StartFrame prepares for per-scanline rendering
func (v *VGAEngine) StartFrame() {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Allocate scanline buffer based on current mode
	w, h := v.GetModeDimensions()
	if len(v.scanlineFrame) != w*h*4 {
		v.scanlineFrame = make([]byte, w*h*4)
	}
}

// ProcessScanline renders a single scanline using current palette state
// This allows copper-driven palette changes to affect specific scanlines
func (v *VGAEngine) ProcessScanline(y int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	switch v.mode {
	case VGA_MODE_13H:
		v.renderScanlineMode13h(y)
	case VGA_MODE_12H:
		v.renderScanlineMode12h(y)
	case VGA_MODE_TEXT:
		v.renderScanlineText(y)
	case VGA_MODE_X:
		v.renderScanlineModeX(y)
	default:
		v.renderScanlineMode13h(y)
	}
}

// FinishFrame completes the frame and returns the rendered result
func (v *VGAEngine) FinishFrame() []byte {
	// Return the scanline-rendered buffer
	return v.scanlineFrame
}

// SetCompositorManaged implements CompositorManageable — signals the render
// goroutine to skip rendering while the compositor owns scanline rendering.
func (v *VGAEngine) SetCompositorManaged(managed bool) {
	v.compositorManaged.Store(managed)
}

// WaitRenderIdle implements CompositorManageable — spins until any in-flight
// render tick has finished, so the compositor can safely begin scanline rendering.
func (v *VGAEngine) WaitRenderIdle() {
	for v.rendering.Load() {
		runtime.Gosched()
	}
}

// -----------------------------------------------------------------------------
// Independent Render Goroutine
// -----------------------------------------------------------------------------

// StartRenderLoop spawns a 60Hz render goroutine that renders frames into
// the triple buffer. GetFrame() becomes a lock-free atomic swap.
func (v *VGAEngine) StartRenderLoop() {
	v.renderMu.Lock()
	defer v.renderMu.Unlock()
	if v.renderRunning.Load() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.renderCancel = cancel
	done := make(chan struct{})
	v.renderDone = done
	v.renderRunning.Store(true)
	go v.renderLoop(ctx, done)
}

// StopRenderLoop stops the render goroutine and waits for it to exit.
func (v *VGAEngine) StopRenderLoop() {
	v.renderMu.Lock()
	if !v.renderRunning.Swap(false) {
		v.renderMu.Unlock()
		return
	}
	cancel := v.renderCancel
	done := v.renderDone
	v.renderMu.Unlock()
	cancel()
	<-done
}

// renderLoop runs at 60Hz, rendering frames into the triple buffer.
// done is goroutine-local to avoid close-of-wrong-channel on restart.
func (v *VGAEngine) renderLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(VGA_REFRESH_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !v.enabled.Load() || v.compositorManaged.Load() {
				continue
			}
			// Double-check barrier: signal rendering, then re-check managed.
			// Compositor sets managed=true then calls WaitRenderIdle().
			v.rendering.Store(true)
			if v.compositorManaged.Load() {
				v.rendering.Store(false)
				continue
			}
			v.RenderFrameTo(v.frameBufs[v.writeIdx])
			v.rendering.Store(false)
			v.writeIdx = int(v.sharedIdx.Swap(int32(v.writeIdx)))
		}
	}
}

// renderScanlineMode13h renders one scanline in Mode 13h (320x200x256)
func (v *VGAEngine) renderScanlineMode13h(y int) {
	width := VGA_MODE13H_WIDTH
	height := VGA_MODE13H_HEIGHT

	if y < 0 || y >= height {
		return
	}

	startAddr := v.getStartAddressInternal()

	for x := range width {
		// Linear addressing with start address offset
		linearOffset := uint32(y*width+x) + startAddr

		// Chain-4: address bits 0-1 select plane, bits 2+ are VRAM offset
		plane := linearOffset & 3
		vramOffset := linearOffset >> 2

		var colorIndex uint8
		if vramOffset < VGA_PLANE_SIZE {
			colorIndex = v.vram[plane][vramOffset]
		}

		// Apply DAC mask
		colorIndex &= v.dacMask

		// Get expanded color using current palette state
		r, g, b, a := v.getPaletteRGBAInternal(colorIndex)

		pixelIdx := (y*width + x) * 4
		if pixelIdx+3 < len(v.scanlineFrame) {
			v.scanlineFrame[pixelIdx+0] = r
			v.scanlineFrame[pixelIdx+1] = g
			v.scanlineFrame[pixelIdx+2] = b
			v.scanlineFrame[pixelIdx+3] = a
		}
	}
}

// renderScanlineMode12h renders one scanline in Mode 12h (640x480x16)
func (v *VGAEngine) renderScanlineMode12h(y int) {
	width := VGA_MODE12H_WIDTH
	height := VGA_MODE12H_HEIGHT

	if y < 0 || y >= height {
		return
	}

	bytesPerLine := width / 8

	for byteX := range bytesPerLine {
		offset := y*bytesPerLine + byteX

		// Get all 4 planes for this byte
		var planes [4]uint8
		for p := range 4 {
			if offset < int(VGA_PLANE_SIZE) {
				planes[p] = v.vram[p][offset]
			}
		}

		// Extract 8 pixels from the planes
		for bit := 7; bit >= 0; bit-- {
			// Combine bits from all planes to get color index
			colorIndex := uint8(0)
			for p := range 4 {
				if planes[p]&(1<<bit) != 0 {
					colorIndex |= 1 << p
				}
			}

			colorIndex &= v.dacMask
			r, g, b, a := v.getPaletteRGBAInternal(colorIndex)

			pixelX := byteX*8 + (7 - bit)
			pixelIdx := (y*width + pixelX) * 4
			if pixelIdx+3 < len(v.scanlineFrame) {
				v.scanlineFrame[pixelIdx+0] = r
				v.scanlineFrame[pixelIdx+1] = g
				v.scanlineFrame[pixelIdx+2] = b
				v.scanlineFrame[pixelIdx+3] = a
			}
		}
	}
}

// renderScanlineModeX renders one scanline in Mode X (320x240x256)
func (v *VGAEngine) renderScanlineModeX(y int) {
	width := VGA_MODEX_WIDTH
	height := VGA_MODEX_HEIGHT

	if y < 0 || y >= height {
		return
	}

	startAddr := v.getStartAddressInternal()

	for x := range width {
		// Unchained: pixel X determines plane, Y*width/4 + X/4 is offset
		plane := x & 3
		offset := uint32(y*(width/4)+x/4) + startAddr

		var colorIndex uint8
		if offset < VGA_PLANE_SIZE {
			colorIndex = v.vram[plane][offset]
		}

		colorIndex &= v.dacMask
		r, g, b, a := v.getPaletteRGBAInternal(colorIndex)

		pixelIdx := (y*width + x) * 4
		if pixelIdx+3 < len(v.scanlineFrame) {
			v.scanlineFrame[pixelIdx+0] = r
			v.scanlineFrame[pixelIdx+1] = g
			v.scanlineFrame[pixelIdx+2] = b
			v.scanlineFrame[pixelIdx+3] = a
		}
	}
}

// renderScanlineText renders one scanline in text mode (80x25)
func (v *VGAEngine) renderScanlineText(y int) {
	charWidth := VGA_FONT_WIDTH
	charHeight := VGA_FONT_HEIGHT
	width := VGA_TEXT_COLS * charWidth
	totalHeight := VGA_TEXT_ROWS * charHeight

	if y < 0 || y >= totalHeight {
		return
	}

	// Determine which character row and which line within the character
	charRow := y / charHeight
	charLine := y % charHeight

	for col := range VGA_TEXT_COLS {
		// Get character and attribute from text buffer
		bufOffset := (charRow*VGA_TEXT_COLS + col) * 2
		char := v.textBuffer[bufOffset]
		attr := v.textBuffer[bufOffset+1]

		// Extract foreground/background from attribute
		fg := attr & 0x0F
		bg := (attr >> 4) & 0x0F

		// Get font glyph row
		fontRow := vgaFont8x16[int(char)*charHeight+charLine]

		// Render 8 pixels for this character
		for cx := range charWidth {
			pixelX := col*charWidth + cx

			var colorIndex uint8
			if fontRow&(0x80>>cx) != 0 {
				colorIndex = fg
			} else {
				colorIndex = bg
			}

			r, g, b, a := v.getPaletteRGBAInternal(colorIndex)

			pixelIdx := (y*width + pixelX) * 4
			if pixelIdx+3 < len(v.scanlineFrame) {
				v.scanlineFrame[pixelIdx+0] = r
				v.scanlineFrame[pixelIdx+1] = g
				v.scanlineFrame[pixelIdx+2] = b
				v.scanlineFrame[pixelIdx+3] = a
			}
		}
	}
}

// Standard VGA 8x16 font (256 characters)
var vgaFont8x16 = []uint8{
	// Character 0 (null)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 1 (smiley)
	0x00, 0x00, 0x7E, 0x81, 0xA5, 0x81, 0x81, 0xBD,
	0x99, 0x81, 0x81, 0x7E, 0x00, 0x00, 0x00, 0x00,
	// Character 2 (inverse smiley)
	0x00, 0x00, 0x7E, 0xFF, 0xDB, 0xFF, 0xFF, 0xC3,
	0xE7, 0xFF, 0xFF, 0x7E, 0x00, 0x00, 0x00, 0x00,
	// Character 3 (heart)
	0x00, 0x00, 0x00, 0x00, 0x6C, 0xFE, 0xFE, 0xFE,
	0xFE, 0x7C, 0x38, 0x10, 0x00, 0x00, 0x00, 0x00,
	// Character 4 (diamond)
	0x00, 0x00, 0x00, 0x00, 0x10, 0x38, 0x7C, 0xFE,
	0x7C, 0x38, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 5 (club)
	0x00, 0x00, 0x00, 0x18, 0x3C, 0x3C, 0xE7, 0xE7,
	0xE7, 0x18, 0x18, 0x3C, 0x00, 0x00, 0x00, 0x00,
	// Character 6 (spade)
	0x00, 0x00, 0x00, 0x18, 0x3C, 0x7E, 0xFF, 0xFF,
	0x7E, 0x18, 0x18, 0x3C, 0x00, 0x00, 0x00, 0x00,
	// Character 7 (bullet)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x3C,
	0x3C, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 8 (inverse bullet)
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xE7, 0xC3,
	0xC3, 0xE7, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	// Character 9 (ring)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x3C, 0x66, 0x42,
	0x42, 0x66, 0x3C, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 10 (inverse ring)
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xC3, 0x99, 0xBD,
	0xBD, 0x99, 0xC3, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	// Character 11 (male)
	0x00, 0x00, 0x1E, 0x0E, 0x1A, 0x32, 0x78, 0xCC,
	0xCC, 0xCC, 0xCC, 0x78, 0x00, 0x00, 0x00, 0x00,
	// Character 12 (female)
	0x00, 0x00, 0x3C, 0x66, 0x66, 0x66, 0x66, 0x3C,
	0x18, 0x7E, 0x18, 0x18, 0x00, 0x00, 0x00, 0x00,
	// Character 13 (note)
	0x00, 0x00, 0x3F, 0x33, 0x3F, 0x30, 0x30, 0x30,
	0x30, 0x70, 0xF0, 0xE0, 0x00, 0x00, 0x00, 0x00,
	// Character 14 (double note)
	0x00, 0x00, 0x7F, 0x63, 0x7F, 0x63, 0x63, 0x63,
	0x63, 0x67, 0xE7, 0xE6, 0xC0, 0x00, 0x00, 0x00,
	// Character 15 (sun)
	0x00, 0x00, 0x00, 0x18, 0x18, 0xDB, 0x3C, 0xE7,
	0x3C, 0xDB, 0x18, 0x18, 0x00, 0x00, 0x00, 0x00,
	// Character 16 (right triangle)
	0x00, 0x80, 0xC0, 0xE0, 0xF0, 0xF8, 0xFE, 0xF8,
	0xF0, 0xE0, 0xC0, 0x80, 0x00, 0x00, 0x00, 0x00,
	// Character 17 (left triangle)
	0x00, 0x02, 0x06, 0x0E, 0x1E, 0x3E, 0xFE, 0x3E,
	0x1E, 0x0E, 0x06, 0x02, 0x00, 0x00, 0x00, 0x00,
	// Character 18 (up/down arrow)
	0x00, 0x00, 0x18, 0x3C, 0x7E, 0x18, 0x18, 0x18,
	0x7E, 0x3C, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 19 (double exclaim)
	0x00, 0x00, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66,
	0x66, 0x00, 0x66, 0x66, 0x00, 0x00, 0x00, 0x00,
	// Character 20 (paragraph)
	0x00, 0x00, 0x7F, 0xDB, 0xDB, 0xDB, 0x7B, 0x1B,
	0x1B, 0x1B, 0x1B, 0x1B, 0x00, 0x00, 0x00, 0x00,
	// Character 21 (section)
	0x00, 0x7C, 0xC6, 0x60, 0x38, 0x6C, 0xC6, 0xC6,
	0x6C, 0x38, 0x0C, 0xC6, 0x7C, 0x00, 0x00, 0x00,
	// Character 22 (thick underline)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0xFE, 0xFE, 0xFE, 0xFE, 0x00, 0x00, 0x00, 0x00,
	// Character 23 (up/down arrow underline)
	0x00, 0x00, 0x18, 0x3C, 0x7E, 0x18, 0x18, 0x18,
	0x7E, 0x3C, 0x18, 0x7E, 0x00, 0x00, 0x00, 0x00,
	// Character 24 (up arrow)
	0x00, 0x00, 0x18, 0x3C, 0x7E, 0x18, 0x18, 0x18,
	0x18, 0x18, 0x18, 0x18, 0x00, 0x00, 0x00, 0x00,
	// Character 25 (down arrow)
	0x00, 0x00, 0x18, 0x18, 0x18, 0x18, 0x18, 0x18,
	0x18, 0x7E, 0x3C, 0x18, 0x00, 0x00, 0x00, 0x00,
	// Character 26 (right arrow)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x0C, 0xFE,
	0x0C, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 27 (left arrow)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x30, 0x60, 0xFE,
	0x60, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 28 (right angle)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xC0, 0xC0,
	0xC0, 0xFE, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 29 (left-right arrow)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x24, 0x66, 0xFF,
	0x66, 0x24, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 30 (up triangle)
	0x00, 0x00, 0x00, 0x00, 0x10, 0x38, 0x38, 0x7C,
	0x7C, 0xFE, 0xFE, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 31 (down triangle)
	0x00, 0x00, 0x00, 0x00, 0xFE, 0xFE, 0x7C, 0x7C,
	0x38, 0x38, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 32 (space)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 33 (!)
	0x00, 0x00, 0x18, 0x3C, 0x3C, 0x3C, 0x18, 0x18,
	0x18, 0x00, 0x18, 0x18, 0x00, 0x00, 0x00, 0x00,
	// Character 34 (")
	0x00, 0x66, 0x66, 0x66, 0x24, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 35 (#)
	0x00, 0x00, 0x00, 0x6C, 0x6C, 0xFE, 0x6C, 0x6C,
	0x6C, 0xFE, 0x6C, 0x6C, 0x00, 0x00, 0x00, 0x00,
	// Character 36 ($)
	0x18, 0x18, 0x7C, 0xC6, 0xC2, 0xC0, 0x7C, 0x06,
	0x06, 0x86, 0xC6, 0x7C, 0x18, 0x18, 0x00, 0x00,
	// Character 37 (%)
	0x00, 0x00, 0x00, 0x00, 0xC2, 0xC6, 0x0C, 0x18,
	0x30, 0x60, 0xC6, 0x86, 0x00, 0x00, 0x00, 0x00,
	// Character 38 (&)
	0x00, 0x00, 0x38, 0x6C, 0x6C, 0x38, 0x76, 0xDC,
	0xCC, 0xCC, 0xCC, 0x76, 0x00, 0x00, 0x00, 0x00,
	// Character 39 (')
	0x00, 0x30, 0x30, 0x30, 0x60, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 40 (()
	0x00, 0x00, 0x0C, 0x18, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x18, 0x0C, 0x00, 0x00, 0x00, 0x00,
	// Character 41 ())
	0x00, 0x00, 0x30, 0x18, 0x0C, 0x0C, 0x0C, 0x0C,
	0x0C, 0x0C, 0x18, 0x30, 0x00, 0x00, 0x00, 0x00,
	// Character 42 (*)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x66, 0x3C, 0xFF,
	0x3C, 0x66, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 43 (+)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x18, 0x7E,
	0x18, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 44 (,)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x18, 0x18, 0x18, 0x30, 0x00, 0x00, 0x00,
	// Character 45 (-)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFE,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 46 (.)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x18, 0x18, 0x00, 0x00, 0x00, 0x00,
	// Character 47 (/)
	0x00, 0x00, 0x00, 0x00, 0x02, 0x06, 0x0C, 0x18,
	0x30, 0x60, 0xC0, 0x80, 0x00, 0x00, 0x00, 0x00,
	// Character 48 (0)
	0x00, 0x00, 0x3C, 0x66, 0xC3, 0xC3, 0xDB, 0xDB,
	0xC3, 0xC3, 0x66, 0x3C, 0x00, 0x00, 0x00, 0x00,
	// Character 49 (1)
	0x00, 0x00, 0x18, 0x38, 0x78, 0x18, 0x18, 0x18,
	0x18, 0x18, 0x18, 0x7E, 0x00, 0x00, 0x00, 0x00,
	// Character 50 (2)
	0x00, 0x00, 0x7C, 0xC6, 0x06, 0x0C, 0x18, 0x30,
	0x60, 0xC0, 0xC6, 0xFE, 0x00, 0x00, 0x00, 0x00,
	// Character 51 (3)
	0x00, 0x00, 0x7C, 0xC6, 0x06, 0x06, 0x3C, 0x06,
	0x06, 0x06, 0xC6, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 52 (4)
	0x00, 0x00, 0x0C, 0x1C, 0x3C, 0x6C, 0xCC, 0xFE,
	0x0C, 0x0C, 0x0C, 0x1E, 0x00, 0x00, 0x00, 0x00,
	// Character 53 (5)
	0x00, 0x00, 0xFE, 0xC0, 0xC0, 0xC0, 0xFC, 0x06,
	0x06, 0x06, 0xC6, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 54 (6)
	0x00, 0x00, 0x38, 0x60, 0xC0, 0xC0, 0xFC, 0xC6,
	0xC6, 0xC6, 0xC6, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 55 (7)
	0x00, 0x00, 0xFE, 0xC6, 0x06, 0x06, 0x0C, 0x18,
	0x30, 0x30, 0x30, 0x30, 0x00, 0x00, 0x00, 0x00,
	// Character 56 (8)
	0x00, 0x00, 0x7C, 0xC6, 0xC6, 0xC6, 0x7C, 0xC6,
	0xC6, 0xC6, 0xC6, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 57 (9)
	0x00, 0x00, 0x7C, 0xC6, 0xC6, 0xC6, 0x7E, 0x06,
	0x06, 0x06, 0x0C, 0x78, 0x00, 0x00, 0x00, 0x00,
	// Character 58 (:)
	0x00, 0x00, 0x00, 0x00, 0x18, 0x18, 0x00, 0x00,
	0x00, 0x18, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 59 (;)
	0x00, 0x00, 0x00, 0x00, 0x18, 0x18, 0x00, 0x00,
	0x00, 0x18, 0x18, 0x30, 0x00, 0x00, 0x00, 0x00,
	// Character 60 (<)
	0x00, 0x00, 0x00, 0x06, 0x0C, 0x18, 0x30, 0x60,
	0x30, 0x18, 0x0C, 0x06, 0x00, 0x00, 0x00, 0x00,
	// Character 61 (=)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x7E, 0x00, 0x00,
	0x7E, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 62 (>)
	0x00, 0x00, 0x00, 0x60, 0x30, 0x18, 0x0C, 0x06,
	0x0C, 0x18, 0x30, 0x60, 0x00, 0x00, 0x00, 0x00,
	// Character 63 (?)
	0x00, 0x00, 0x7C, 0xC6, 0xC6, 0x0C, 0x18, 0x18,
	0x18, 0x00, 0x18, 0x18, 0x00, 0x00, 0x00, 0x00,
	// Character 64 (@)
	0x00, 0x00, 0x00, 0x7C, 0xC6, 0xC6, 0xDE, 0xDE,
	0xDE, 0xDC, 0xC0, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 65 (A)
	0x00, 0x00, 0x10, 0x38, 0x6C, 0xC6, 0xC6, 0xFE,
	0xC6, 0xC6, 0xC6, 0xC6, 0x00, 0x00, 0x00, 0x00,
	// Character 66 (B)
	0x00, 0x00, 0xFC, 0x66, 0x66, 0x66, 0x7C, 0x66,
	0x66, 0x66, 0x66, 0xFC, 0x00, 0x00, 0x00, 0x00,
	// Character 67 (C)
	0x00, 0x00, 0x3C, 0x66, 0xC2, 0xC0, 0xC0, 0xC0,
	0xC0, 0xC2, 0x66, 0x3C, 0x00, 0x00, 0x00, 0x00,
	// Character 68 (D)
	0x00, 0x00, 0xF8, 0x6C, 0x66, 0x66, 0x66, 0x66,
	0x66, 0x66, 0x6C, 0xF8, 0x00, 0x00, 0x00, 0x00,
	// Character 69 (E)
	0x00, 0x00, 0xFE, 0x66, 0x62, 0x68, 0x78, 0x68,
	0x60, 0x62, 0x66, 0xFE, 0x00, 0x00, 0x00, 0x00,
	// Character 70 (F)
	0x00, 0x00, 0xFE, 0x66, 0x62, 0x68, 0x78, 0x68,
	0x60, 0x60, 0x60, 0xF0, 0x00, 0x00, 0x00, 0x00,
	// Character 71 (G)
	0x00, 0x00, 0x3C, 0x66, 0xC2, 0xC0, 0xC0, 0xDE,
	0xC6, 0xC6, 0x66, 0x3A, 0x00, 0x00, 0x00, 0x00,
	// Character 72 (H)
	0x00, 0x00, 0xC6, 0xC6, 0xC6, 0xC6, 0xFE, 0xC6,
	0xC6, 0xC6, 0xC6, 0xC6, 0x00, 0x00, 0x00, 0x00,
	// Character 73 (I)
	0x00, 0x00, 0x3C, 0x18, 0x18, 0x18, 0x18, 0x18,
	0x18, 0x18, 0x18, 0x3C, 0x00, 0x00, 0x00, 0x00,
	// Character 74 (J)
	0x00, 0x00, 0x1E, 0x0C, 0x0C, 0x0C, 0x0C, 0x0C,
	0xCC, 0xCC, 0xCC, 0x78, 0x00, 0x00, 0x00, 0x00,
	// Character 75 (K)
	0x00, 0x00, 0xE6, 0x66, 0x66, 0x6C, 0x78, 0x78,
	0x6C, 0x66, 0x66, 0xE6, 0x00, 0x00, 0x00, 0x00,
	// Character 76 (L)
	0x00, 0x00, 0xF0, 0x60, 0x60, 0x60, 0x60, 0x60,
	0x60, 0x62, 0x66, 0xFE, 0x00, 0x00, 0x00, 0x00,
	// Character 77 (M)
	0x00, 0x00, 0xC3, 0xE7, 0xFF, 0xFF, 0xDB, 0xC3,
	0xC3, 0xC3, 0xC3, 0xC3, 0x00, 0x00, 0x00, 0x00,
	// Character 78 (N)
	0x00, 0x00, 0xC6, 0xE6, 0xF6, 0xFE, 0xDE, 0xCE,
	0xC6, 0xC6, 0xC6, 0xC6, 0x00, 0x00, 0x00, 0x00,
	// Character 79 (O)
	0x00, 0x00, 0x7C, 0xC6, 0xC6, 0xC6, 0xC6, 0xC6,
	0xC6, 0xC6, 0xC6, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 80 (P)
	0x00, 0x00, 0xFC, 0x66, 0x66, 0x66, 0x7C, 0x60,
	0x60, 0x60, 0x60, 0xF0, 0x00, 0x00, 0x00, 0x00,
	// Character 81 (Q)
	0x00, 0x00, 0x7C, 0xC6, 0xC6, 0xC6, 0xC6, 0xC6,
	0xC6, 0xD6, 0xDE, 0x7C, 0x0C, 0x0E, 0x00, 0x00,
	// Character 82 (R)
	0x00, 0x00, 0xFC, 0x66, 0x66, 0x66, 0x7C, 0x6C,
	0x66, 0x66, 0x66, 0xE6, 0x00, 0x00, 0x00, 0x00,
	// Character 83 (S)
	0x00, 0x00, 0x7C, 0xC6, 0xC6, 0x60, 0x38, 0x0C,
	0x06, 0xC6, 0xC6, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 84 (T)
	0x00, 0x00, 0xFF, 0xDB, 0x99, 0x18, 0x18, 0x18,
	0x18, 0x18, 0x18, 0x3C, 0x00, 0x00, 0x00, 0x00,
	// Character 85 (U)
	0x00, 0x00, 0xC6, 0xC6, 0xC6, 0xC6, 0xC6, 0xC6,
	0xC6, 0xC6, 0xC6, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 86 (V)
	0x00, 0x00, 0xC3, 0xC3, 0xC3, 0xC3, 0xC3, 0xC3,
	0xC3, 0x66, 0x3C, 0x18, 0x00, 0x00, 0x00, 0x00,
	// Character 87 (W)
	0x00, 0x00, 0xC3, 0xC3, 0xC3, 0xC3, 0xC3, 0xDB,
	0xDB, 0xFF, 0x66, 0x66, 0x00, 0x00, 0x00, 0x00,
	// Character 88 (X)
	0x00, 0x00, 0xC3, 0xC3, 0x66, 0x3C, 0x18, 0x18,
	0x3C, 0x66, 0xC3, 0xC3, 0x00, 0x00, 0x00, 0x00,
	// Character 89 (Y)
	0x00, 0x00, 0xC3, 0xC3, 0xC3, 0x66, 0x3C, 0x18,
	0x18, 0x18, 0x18, 0x3C, 0x00, 0x00, 0x00, 0x00,
	// Character 90 (Z)
	0x00, 0x00, 0xFF, 0xC3, 0x86, 0x0C, 0x18, 0x30,
	0x60, 0xC1, 0xC3, 0xFF, 0x00, 0x00, 0x00, 0x00,
	// Character 91 ([)
	0x00, 0x00, 0x3C, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x3C, 0x00, 0x00, 0x00, 0x00,
	// Character 92 (\)
	0x00, 0x00, 0x00, 0x80, 0xC0, 0xE0, 0x70, 0x38,
	0x1C, 0x0E, 0x06, 0x02, 0x00, 0x00, 0x00, 0x00,
	// Character 93 (])
	0x00, 0x00, 0x3C, 0x0C, 0x0C, 0x0C, 0x0C, 0x0C,
	0x0C, 0x0C, 0x0C, 0x3C, 0x00, 0x00, 0x00, 0x00,
	// Character 94 (^)
	0x10, 0x38, 0x6C, 0xC6, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 95 (_)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0xFF, 0x00, 0x00,
	// Character 96 (`)
	0x30, 0x30, 0x18, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 97 (a)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x78, 0x0C, 0x7C,
	0xCC, 0xCC, 0xCC, 0x76, 0x00, 0x00, 0x00, 0x00,
	// Character 98 (b)
	0x00, 0x00, 0xE0, 0x60, 0x60, 0x78, 0x6C, 0x66,
	0x66, 0x66, 0x66, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 99 (c)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x7C, 0xC6, 0xC0,
	0xC0, 0xC0, 0xC6, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 100 (d)
	0x00, 0x00, 0x1C, 0x0C, 0x0C, 0x3C, 0x6C, 0xCC,
	0xCC, 0xCC, 0xCC, 0x76, 0x00, 0x00, 0x00, 0x00,
	// Character 101 (e)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x7C, 0xC6, 0xFE,
	0xC0, 0xC0, 0xC6, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 102 (f)
	0x00, 0x00, 0x38, 0x6C, 0x64, 0x60, 0xF0, 0x60,
	0x60, 0x60, 0x60, 0xF0, 0x00, 0x00, 0x00, 0x00,
	// Character 103 (g)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x76, 0xCC, 0xCC,
	0xCC, 0xCC, 0xCC, 0x7C, 0x0C, 0xCC, 0x78, 0x00,
	// Character 104 (h)
	0x00, 0x00, 0xE0, 0x60, 0x60, 0x6C, 0x76, 0x66,
	0x66, 0x66, 0x66, 0xE6, 0x00, 0x00, 0x00, 0x00,
	// Character 105 (i)
	0x00, 0x00, 0x18, 0x18, 0x00, 0x38, 0x18, 0x18,
	0x18, 0x18, 0x18, 0x3C, 0x00, 0x00, 0x00, 0x00,
	// Character 106 (j)
	0x00, 0x00, 0x06, 0x06, 0x00, 0x0E, 0x06, 0x06,
	0x06, 0x06, 0x06, 0x06, 0x66, 0x66, 0x3C, 0x00,
	// Character 107 (k)
	0x00, 0x00, 0xE0, 0x60, 0x60, 0x66, 0x6C, 0x78,
	0x78, 0x6C, 0x66, 0xE6, 0x00, 0x00, 0x00, 0x00,
	// Character 108 (l)
	0x00, 0x00, 0x38, 0x18, 0x18, 0x18, 0x18, 0x18,
	0x18, 0x18, 0x18, 0x3C, 0x00, 0x00, 0x00, 0x00,
	// Character 109 (m)
	0x00, 0x00, 0x00, 0x00, 0x00, 0xE6, 0xFF, 0xDB,
	0xDB, 0xDB, 0xDB, 0xDB, 0x00, 0x00, 0x00, 0x00,
	// Character 110 (n)
	0x00, 0x00, 0x00, 0x00, 0x00, 0xDC, 0x66, 0x66,
	0x66, 0x66, 0x66, 0x66, 0x00, 0x00, 0x00, 0x00,
	// Character 111 (o)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x7C, 0xC6, 0xC6,
	0xC6, 0xC6, 0xC6, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 112 (p)
	0x00, 0x00, 0x00, 0x00, 0x00, 0xDC, 0x66, 0x66,
	0x66, 0x66, 0x66, 0x7C, 0x60, 0x60, 0xF0, 0x00,
	// Character 113 (q)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x76, 0xCC, 0xCC,
	0xCC, 0xCC, 0xCC, 0x7C, 0x0C, 0x0C, 0x1E, 0x00,
	// Character 114 (r)
	0x00, 0x00, 0x00, 0x00, 0x00, 0xDC, 0x76, 0x66,
	0x60, 0x60, 0x60, 0xF0, 0x00, 0x00, 0x00, 0x00,
	// Character 115 (s)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x7C, 0xC6, 0x60,
	0x38, 0x0C, 0xC6, 0x7C, 0x00, 0x00, 0x00, 0x00,
	// Character 116 (t)
	0x00, 0x00, 0x10, 0x30, 0x30, 0xFC, 0x30, 0x30,
	0x30, 0x30, 0x36, 0x1C, 0x00, 0x00, 0x00, 0x00,
	// Character 117 (u)
	0x00, 0x00, 0x00, 0x00, 0x00, 0xCC, 0xCC, 0xCC,
	0xCC, 0xCC, 0xCC, 0x76, 0x00, 0x00, 0x00, 0x00,
	// Character 118 (v)
	0x00, 0x00, 0x00, 0x00, 0x00, 0xC3, 0xC3, 0xC3,
	0xC3, 0x66, 0x3C, 0x18, 0x00, 0x00, 0x00, 0x00,
	// Character 119 (w)
	0x00, 0x00, 0x00, 0x00, 0x00, 0xC3, 0xC3, 0xC3,
	0xDB, 0xDB, 0xFF, 0x66, 0x00, 0x00, 0x00, 0x00,
	// Character 120 (x)
	0x00, 0x00, 0x00, 0x00, 0x00, 0xC3, 0x66, 0x3C,
	0x18, 0x3C, 0x66, 0xC3, 0x00, 0x00, 0x00, 0x00,
	// Character 121 (y)
	0x00, 0x00, 0x00, 0x00, 0x00, 0xC6, 0xC6, 0xC6,
	0xC6, 0xC6, 0xC6, 0x7E, 0x06, 0x0C, 0xF8, 0x00,
	// Character 122 (z)
	0x00, 0x00, 0x00, 0x00, 0x00, 0xFE, 0xCC, 0x18,
	0x30, 0x60, 0xC6, 0xFE, 0x00, 0x00, 0x00, 0x00,
	// Character 123 ({)
	0x00, 0x00, 0x0E, 0x18, 0x18, 0x18, 0x70, 0x18,
	0x18, 0x18, 0x18, 0x0E, 0x00, 0x00, 0x00, 0x00,
	// Character 124 (|)
	0x00, 0x00, 0x18, 0x18, 0x18, 0x18, 0x00, 0x18,
	0x18, 0x18, 0x18, 0x18, 0x00, 0x00, 0x00, 0x00,
	// Character 125 (})
	0x00, 0x00, 0x70, 0x18, 0x18, 0x18, 0x0E, 0x18,
	0x18, 0x18, 0x18, 0x70, 0x00, 0x00, 0x00, 0x00,
	// Character 126 (~)
	0x00, 0x00, 0x76, 0xDC, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Character 127 (DEL - block)
	0x00, 0x00, 0x00, 0x00, 0x10, 0x38, 0x6C, 0xC6,
	0xC6, 0xC6, 0xFE, 0x00, 0x00, 0x00, 0x00, 0x00,
	// Characters 128-255 are filled with block characters for now
}

func init() {
	// Extend font to 256 characters if needed
	if len(vgaFont8x16) < 256*16 {
		extended := make([]uint8, 256*16)
		copy(extended, vgaFont8x16)

		// Fill remaining with placeholder first
		for i := len(vgaFont8x16); i < 256*16; i++ {
			extended[i] = 0x00
		}

		// Character 176 (░) - light shade (25% density)
		copy(extended[176*16:], []uint8{
			0x22, 0x88, 0x22, 0x88, 0x22, 0x88, 0x22, 0x88,
			0x22, 0x88, 0x22, 0x88, 0x22, 0x88, 0x22, 0x88,
		})

		// Character 177 (▒) - medium shade (50% density)
		copy(extended[177*16:], []uint8{
			0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55,
			0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55,
		})

		// Character 178 (▓) - dark shade (75% density)
		copy(extended[178*16:], []uint8{
			0xDD, 0x77, 0xDD, 0x77, 0xDD, 0x77, 0xDD, 0x77,
			0xDD, 0x77, 0xDD, 0x77, 0xDD, 0x77, 0xDD, 0x77,
		})

		// Character 219 (█) - full block
		copy(extended[219*16:], []uint8{
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		})

		// Character 220 (▄) - lower half block
		copy(extended[220*16:], []uint8{
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		})

		// Character 221 (▌) - left half block
		copy(extended[221*16:], []uint8{
			0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0,
			0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0, 0xF0,
		})

		// Character 222 (▐) - right half block
		copy(extended[222*16:], []uint8{
			0x0F, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F,
			0x0F, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F, 0x0F,
		})

		// Character 223 (▀) - upper half block
		copy(extended[223*16:], []uint8{
			0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		})

		vgaFont8x16 = extended
	}
}
