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
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// ULAEngine implements ZX Spectrum ULA video as a standalone device.
// Implements VideoSource interface for compositor integration.
type ULAEngine struct {
	mu  sync.Mutex
	bus *MachineBus

	irqSinkMu   sync.RWMutex
	irqSink     ulaIRQSink
	irqAsserted atomic.Bool

	// Border color (0-7)
	border uint8

	// Control register
	control uint8

	// Lock-free flags
	enabled      atomic.Bool // Set by HandleWrite when control changes
	vblankActive atomic.Bool // Set by SignalVSync, cleared by HandleRead(ULA_STATUS)

	// VRAM (6144 bitmap + 768 attributes = 6912 bytes)
	vram      [ULA_VRAM_SIZE]uint8
	addrLatch uint16

	// Flash state for FLASH attribute
	flashState   atomic.Bool
	flashCounter atomic.Int32

	// Pre-computed row start addresses for the non-linear ZX Spectrum addressing
	// Computed once at init, indexed by Y coordinate (0-191)
	rowStartAddr [ULA_DISPLAY_HEIGHT]uint16

	// Pre-built uint32 color lookup: [0..7] = normal, [8..15] = bright
	colorU32 [16]uint32

	// Pre-allocated frame buffer (320x256 RGBA)
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

// NewULAEngine creates a new ULA engine instance
func NewULAEngine(bus *MachineBus) *ULAEngine {
	ula := &ULAEngine{
		bus:         bus,
		border:      0, // Default: black border
		control:     0, // Disabled by default - programs must enable explicitly
		frameBuffer: make([]byte, ULA_FRAME_WIDTH*ULA_FRAME_HEIGHT*4),
	}

	// Initialize VRAM to zero
	for i := range ula.vram {
		ula.vram[i] = 0
	}

	// Pre-build uint32 color lookup: [0..7] = normal, [8..15] = bright
	for i := range 8 {
		c := ULAColorNormal[i]
		ula.colorU32[i] = uint32(c[0]) | uint32(c[1])<<8 | uint32(c[2])<<16 | 0xFF000000
		c = ULAColorBright[i]
		ula.colorU32[8+i] = uint32(c[0]) | uint32(c[1])<<8 | uint32(c[2])<<16 | 0xFF000000
	}

	// Pre-compute row start addresses for the non-linear ZX Spectrum addressing
	// This avoids recalculating the complex formula on every pixel
	for y := range ULA_DISPLAY_HEIGHT {
		highY := (y & 0xC0) << 5 // Top 2 bits of Y * 32
		lowY := (y & 0x07) << 8  // Bottom 3 bits of Y * 256
		midY := (y & 0x38) << 2  // Middle 3 bits of Y * 4
		ula.rowStartAddr[y] = uint16(highY + lowY + midY)
	}

	// Initialize triple buffers for lock-free GetFrame
	bufSize := ULA_FRAME_WIDTH * ULA_FRAME_HEIGHT * 4
	for i := range ula.frameBufs {
		ula.frameBufs[i] = make([]byte, bufSize)
	}
	ula.writeIdx = 0
	ula.sharedIdx.Store(1)
	ula.readingIdx = 2

	return ula
}

// HandleRead handles register reads
func (u *ULAEngine) HandleRead(addr uint32) uint32 {
	u.mu.Lock()
	defer u.mu.Unlock()

	switch addr {
	case ULA_BORDER:
		return uint32(u.border)
	case ULA_CTRL:
		return uint32(u.control)
	case ULA_STATUS:
		// Return vblank status and clear it (acknowledge) - atomic swap
		if u.vblankActive.Swap(false) {
			u.deassertVBlankIRQ()
			return ULA_STATUS_VBLANK
		}
		u.deassertVBlankIRQ()
		return 0
	case ULA_ADDR_LO:
		return uint32(u.addrLatch & 0x00FF)
	case ULA_ADDR_HI:
		return uint32((u.addrLatch >> 8) & 0x1F)
	case ULA_DATA:
		return uint32(u.readLatchedVRAMLocked())
	default:
		return 0
	}
}

// HandleWrite handles register writes
func (u *ULAEngine) HandleWrite(addr uint32, value uint32) {
	u.mu.Lock()
	defer u.mu.Unlock()

	switch addr {
	case ULA_BORDER:
		// Border color: only bits 0-2 are used
		u.border = uint8(value & 0x07)
	case ULA_CTRL:
		u.control = uint8(value)
		u.enabled.Store(u.control&ULA_CTRL_ENABLE != 0)
		if u.control&(ULA_CTRL_ENABLE|ULA_CTRL_VBLANK_IRQ_EN) != ULA_CTRL_ENABLE|ULA_CTRL_VBLANK_IRQ_EN {
			u.deassertVBlankIRQ()
		}
	case ULA_ADDR_LO:
		u.addrLatch = (u.addrLatch & 0x1F00) | uint16(value&0xFF)
	case ULA_ADDR_HI:
		u.addrLatch = (u.addrLatch & 0x00FF) | (uint16(value&0x1F) << 8)
	case ULA_DATA:
		u.writeLatchedVRAMLocked(uint8(value))
	}
}

func (u *ULAEngine) readLatchedVRAMLocked() uint8 {
	if int(u.addrLatch) >= len(u.vram) {
		return 0
	}
	value := u.vram[u.addrLatch]
	u.advanceLatchLocked()
	return value
}

func (u *ULAEngine) writeLatchedVRAMLocked(value uint8) {
	if int(u.addrLatch) < len(u.vram) {
		u.vram[u.addrLatch] = value
	}
	u.advanceLatchLocked()
}

func (u *ULAEngine) advanceLatchLocked() {
	if u.control&ULA_CTRL_AUTO_INC == 0 {
		return
	}
	u.addrLatch++
	if u.addrLatch >= ULA_VRAM_SIZE {
		u.addrLatch = 0
	}
}

// SetIRQSink binds ULA VBlank IRQ delivery to the active CPU.
func (u *ULAEngine) SetIRQSink(sink ulaIRQSink) {
	if sink == nil {
		sink = noopULAIRQAdapter{}
	}
	u.irqSinkMu.Lock()
	old := u.irqSink
	if old != nil {
		old.DeassertVBlankIRQ()
	}
	u.irqSink = sink
	u.irqAsserted.Store(false)
	u.irqSinkMu.Unlock()
}

func (u *ULAEngine) assertVBlankIRQ() {
	if !u.irqAsserted.CompareAndSwap(false, true) {
		u.irqSinkMu.RLock()
		sink := u.irqSink
		u.irqSinkMu.RUnlock()
		if sink != nil {
			sink.AssertVBlankIRQ()
		}
		return
	}
	u.irqSinkMu.RLock()
	sink := u.irqSink
	u.irqSinkMu.RUnlock()
	if sink != nil {
		sink.AssertVBlankIRQ()
	}
}

func (u *ULAEngine) deassertVBlankIRQ() {
	if !u.irqAsserted.Swap(false) {
		return
	}
	u.irqSinkMu.RLock()
	sink := u.irqSink
	u.irqSinkMu.RUnlock()
	if sink != nil {
		sink.DeassertVBlankIRQ()
	}
}

// HandleVRAMRead reads from ULA VRAM
func (u *ULAEngine) HandleVRAMRead(offset uint16) uint8 {
	u.mu.Lock()
	defer u.mu.Unlock()

	if int(offset) >= len(u.vram) {
		return 0
	}
	return u.vram[offset]
}

// HandleVRAMWrite writes to ULA VRAM
func (u *ULAEngine) HandleVRAMWrite(offset uint16, value uint8) {
	u.mu.Lock()
	defer u.mu.Unlock()

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

// RenderFrameTo renders the complete display directly into dst, avoiding a copy.
func (u *ULAEngine) RenderFrameTo(dst []byte) {
	u.renderInto(dst)
}

// RenderFrame renders the complete display including border.
func (u *ULAEngine) RenderFrame() []byte {
	u.mu.Lock()
	frameBuffer := u.frameBuffer
	u.mu.Unlock()
	return u.renderInto(frameBuffer)
}

func (u *ULAEngine) renderInto(frameBuffer []byte) []byte {
	// Snapshot VRAM and registers under lock, then render lock-free
	var snapVram [ULA_VRAM_SIZE]uint8
	var snapBorder uint8
	u.mu.Lock()
	snapVram = u.vram
	snapBorder = u.border
	u.mu.Unlock()
	snapFlashState := u.flashState.Load()

	// Get border color as packed uint32
	borderU32 := u.colorU32[snapBorder&0x07]

	// Fill entire frame with border color using uint32 writes
	for i := 0; i < len(frameBuffer); i += 4 {
		*(*uint32)(unsafe.Pointer(&frameBuffer[i])) = borderU32
	}

	// Render the 256x192 display area (cell-based: 32 cells wide x 192 scanlines)
	for screenY := range ULA_DISPLAY_HEIGHT {
		// Use pre-computed row start address
		rowAddr := u.rowStartAddr[screenY]

		// Pre-compute attribute row address base
		cellY := screenY >> 3
		attrRowBase := uint16(ULA_ATTR_OFFSET + cellY*ULA_CELLS_X)

		// Frame buffer offset for this row
		frameY := ULA_BORDER_TOP + screenY
		frameRowBase := frameY * ULA_FRAME_WIDTH * 4

		// Iterate by 8-pixel cell (32 cells per row)
		for cellX := range ULA_CELLS_X {
			// Read bitmap byte once per cell
			bitmapAddr := rowAddr + uint16(cellX)
			bitmapByte := snapVram[bitmapAddr]

			// Read attribute once per cell
			attr := snapVram[attrRowBase+uint16(cellX)]

			// Parse attribute
			ink := attr & 0x07
			paper := (attr >> 3) & 0x07
			bright := (attr & 0x40) != 0
			flash := (attr & 0x80) != 0

			// Determine fg/bg based on FLASH state
			fgColor := ink
			bgColor := paper
			if flash && snapFlashState {
				fgColor, bgColor = bgColor, fgColor
			}

			// Resolve to uint32 colors once per cell
			var brightOff uint8
			if bright {
				brightOff = 8
			}
			fgU32 := u.colorU32[brightOff+fgColor]
			bgU32 := u.colorU32[brightOff+bgColor]

			// Write 8 pixels for this cell
			frameX := ULA_BORDER_LEFT + cellX*8
			pixelBase := frameRowBase + frameX*4
			for bit := 7; bit >= 0; bit-- {
				pixelIdx := pixelBase + (7-bit)*4
				if (bitmapByte>>bit)&1 != 0 {
					*(*uint32)(unsafe.Pointer(&frameBuffer[pixelIdx])) = fgU32
				} else {
					*(*uint32)(unsafe.Pointer(&frameBuffer[pixelIdx])) = bgU32
				}
			}
		}
	}

	return frameBuffer
}

// =============================================================================
// VideoSource Interface Implementation
// =============================================================================

// GetFrame returns the current rendered frame via lock-free triple-buffer swap.
func (u *ULAEngine) GetFrame() []byte {
	if !u.IsEnabled() {
		return nil
	}
	newRead := int(u.sharedIdx.Swap(int32(u.readingIdx)))
	u.readingIdx = newRead
	return u.frameBufs[u.readingIdx]
}

// IsEnabled returns whether the ULA is active (lock-free).
func (u *ULAEngine) IsEnabled() bool {
	return u.enabled.Load()
}

// GetLayer returns the Z-order for compositing (higher = on top).
func (u *ULAEngine) GetLayer() int {
	return ULA_LAYER
}

// GetDimensions returns the frame dimensions.
func (u *ULAEngine) GetDimensions() (w, h int) {
	return ULA_FRAME_WIDTH, ULA_FRAME_HEIGHT
}

// SignalVSync is called by compositor after visible output is sent.
// ULA chip-clock state advances in TickFrame so disabled sources still tick.
func (u *ULAEngine) SignalVSync() {
}

// TickFrame advances ULA chip-clock state regardless of compositor visibility.
func (u *ULAEngine) TickFrame() {
	// Set VBlank flag - lock-free
	u.vblankActive.Store(true)
	if u.enabled.Load() {
		u.mu.Lock()
		irqEnabled := u.control&ULA_CTRL_VBLANK_IRQ_EN != 0
		u.mu.Unlock()
		if irqEnabled {
			u.assertVBlankIRQ()
		}
	}

	if u.flashCounter.Add(1) >= ULA_FLASH_FRAMES {
		u.flashCounter.Store(0)
		u.flashState.Store(!u.flashState.Load())
	}
}

// =============================================================================
// Independent Render Goroutine
// =============================================================================

// SetCompositorManaged implements CompositorManageable.
func (u *ULAEngine) SetCompositorManaged(managed bool) {
	u.compositorManaged.Store(managed)
}

// WaitRenderIdle implements CompositorManageable.
func (u *ULAEngine) WaitRenderIdle() {
	for u.rendering.Load() {
		runtime.Gosched()
	}
}

// StartRenderLoop spawns a 60Hz render goroutine for lock-free GetFrame.
func (u *ULAEngine) StartRenderLoop() {
	u.renderMu.Lock()
	defer u.renderMu.Unlock()
	if u.renderRunning.Load() {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	u.renderCancel = cancel
	done := make(chan struct{})
	u.renderDone = done
	u.renderRunning.Store(true)
	go u.renderLoop(ctx, done)
}

// StopRenderLoop stops the render goroutine and waits for it to exit.
func (u *ULAEngine) StopRenderLoop() {
	u.renderMu.Lock()
	if !u.renderRunning.Swap(false) {
		u.renderMu.Unlock()
		return
	}
	cancel := u.renderCancel
	done := u.renderDone
	u.renderMu.Unlock()
	cancel()
	<-done
}

// renderLoop runs at 60Hz, rendering frames into the triple buffer.
// done is goroutine-local to avoid close-of-wrong-channel on restart.
func (u *ULAEngine) renderLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(COMPOSITOR_REFRESH_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !u.enabled.Load() || u.compositorManaged.Load() {
				continue
			}
			u.rendering.Store(true)
			if u.compositorManaged.Load() {
				u.rendering.Store(false)
				continue
			}
			u.RenderFrameTo(u.frameBufs[u.writeIdx])
			u.rendering.Store(false)
			u.writeIdx = int(u.sharedIdx.Swap(int32(u.writeIdx)))
		}
	}
}

// =============================================================================
// MachineBus-Compatible VRAM Handlers
// =============================================================================

// HandleBusVRAMRead handles VRAM reads from the system bus (uint32 addresses)
func (u *ULAEngine) HandleBusVRAMRead(addr uint32) uint32 {
	if addr < ULA_VRAM_AP_BASE || addr+3 > ULA_VRAM_AP_END {
		return 0
	}
	offset := uint16(addr - ULA_VRAM_AP_BASE)
	u.mu.Lock()
	defer u.mu.Unlock()
	return uint32(u.vram[offset]) |
		uint32(u.vram[offset+1])<<8 |
		uint32(u.vram[offset+2])<<16 |
		uint32(u.vram[offset+3])<<24
}

// HandleBusVRAMWrite handles VRAM writes from the system bus (uint32 addresses)
func (u *ULAEngine) HandleBusVRAMWrite(addr uint32, value uint32) {
	if addr < ULA_VRAM_AP_BASE || addr+3 > ULA_VRAM_AP_END {
		return
	}
	u.HandleWrite8(addr, uint8(value))
	u.HandleWrite8(addr+1, uint8(value>>8))
	u.HandleWrite8(addr+2, uint8(value>>16))
	u.HandleWrite8(addr+3, uint8(value>>24))
}

// HandleRead8 handles byte reads from the ULA register block or VRAM aperture.
func (u *ULAEngine) HandleRead8(addr uint32) uint8 {
	if addr >= ULA_VRAM_AP_BASE && addr <= ULA_VRAM_AP_END {
		return u.HandleVRAMRead(uint16(addr - ULA_VRAM_AP_BASE))
	}
	return uint8(u.HandleRead(addr))
}

// HandleWrite8 handles byte writes from the ULA register block or VRAM aperture.
func (u *ULAEngine) HandleWrite8(addr uint32, value uint8) {
	if addr >= ULA_VRAM_AP_BASE && addr <= ULA_VRAM_AP_END {
		u.HandleVRAMWrite(uint16(addr-ULA_VRAM_AP_BASE), value)
		return
	}
	u.HandleWrite(addr, uint32(value))
}

// HandleRead64 returns eight aperture bytes packed little-endian.
func (u *ULAEngine) HandleRead64(addr uint32) uint64 {
	if addr < ULA_VRAM_AP_BASE || addr+7 > ULA_VRAM_AP_END {
		return 0
	}
	lo := uint64(u.HandleBusVRAMRead(addr))
	hi := uint64(u.HandleBusVRAMRead(addr + 4))
	return lo | hi<<32
}

// HandleWrite64 stores all eight aperture bytes little-endian.
func (u *ULAEngine) HandleWrite64(addr uint32, value uint64) {
	if addr < ULA_VRAM_AP_BASE || addr+7 > ULA_VRAM_AP_END {
		return
	}
	u.HandleBusVRAMWrite(addr, uint32(value))
	u.HandleBusVRAMWrite(addr+4, uint32(value>>32))
}
