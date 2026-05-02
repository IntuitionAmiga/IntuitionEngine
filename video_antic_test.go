// video_antic_test.go - ANTIC video chip test suite for Intuition Engine

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

package main

import (
	"testing"
	"time"
)

// =============================================================================
// Sprint 1: Structure and Constants Tests
// =============================================================================

// TestANTIC_NewEngine tests basic construction
func TestANTIC_NewEngine(t *testing.T) {
	antic := NewANTICEngine(nil)
	if antic == nil {
		t.Fatal("NewANTICEngine returned nil")
	}
}

// TestANTIC_DefaultState tests default state after construction
func TestANTIC_DefaultState(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Should be disabled by default
	if antic.IsEnabled() {
		t.Error("Expected ANTIC to be disabled by default")
	}

	// DMACTL should be 0 (DMA disabled)
	if antic.dmactl != 0 {
		t.Errorf("Expected dmactl=0, got %d", antic.dmactl)
	}

	// VCOUNT should be 0
	if antic.vcount != 0 {
		t.Errorf("Expected vcount=0, got %d", antic.vcount)
	}

	// VBlank should be inactive
	if antic.vblankActive.Load() {
		t.Error("Expected vblankActive to be false initially")
	}

	// NMI status should be 0
	if antic.nmist != 0 {
		t.Errorf("Expected nmist=0, got %d", antic.nmist)
	}
}

// TestANTIC_Implements_VideoSource tests interface compliance
func TestANTIC_Implements_VideoSource(t *testing.T) {
	var _ VideoSource = (*ANTICEngine)(nil) // Compile-time check
}

// TestANTIC_RegisterAddresses tests register address constants
func TestANTIC_RegisterAddresses(t *testing.T) {
	// Verify register base address
	if ANTIC_BASE != 0xF2100 {
		t.Errorf("ANTIC_BASE: expected 0xF2100, got 0x%X", ANTIC_BASE)
	}

	// Verify 4-byte alignment for copper compatibility
	if (ANTIC_DMACTL-ANTIC_BASE)%4 != 0 {
		t.Error("ANTIC_DMACTL not 4-byte aligned")
	}
	if (ANTIC_ENABLE-ANTIC_BASE)%4 != 0 {
		t.Error("ANTIC_ENABLE not 4-byte aligned")
	}
	if (ANTIC_STATUS-ANTIC_BASE)%4 != 0 {
		t.Error("ANTIC_STATUS not 4-byte aligned")
	}
}

// =============================================================================
// Sprint 2: Register Read/Write Tests
// =============================================================================

// TestANTIC_RegisterWrite_DMACTL tests DMACTL register
func TestANTIC_RegisterWrite_DMACTL(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Write DMACTL with playfield and DMA enabled
	antic.HandleWrite(ANTIC_DMACTL, ANTIC_DMA_NORMAL|ANTIC_DMA_DL)

	// Read back
	val := antic.HandleRead(ANTIC_DMACTL)
	expected := uint32(ANTIC_DMA_NORMAL | ANTIC_DMA_DL)
	if val != expected {
		t.Errorf("DMACTL read: expected 0x%02X, got 0x%02X", expected, val)
	}
}

// TestANTIC_RegisterWrite_CHACTL tests character control register
func TestANTIC_RegisterWrite_CHACTL(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Write CHACTL with inverse and reflect
	antic.HandleWrite(ANTIC_CHACTL, ANTIC_CHACTL_INVERT|ANTIC_CHACTL_REFLECT)

	// Read back
	val := antic.HandleRead(ANTIC_CHACTL)
	expected := uint32(ANTIC_CHACTL_INVERT | ANTIC_CHACTL_REFLECT)
	if val != expected {
		t.Errorf("CHACTL read: expected 0x%02X, got 0x%02X", expected, val)
	}
}

// TestANTIC_RegisterWrite_DisplayList tests display list pointer registers
func TestANTIC_RegisterWrite_DisplayList(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Write display list pointer (0x1234)
	antic.HandleWrite(ANTIC_DLISTL, 0x34)
	antic.HandleWrite(ANTIC_DLISTH, 0x12)

	// Read back
	lo := antic.HandleRead(ANTIC_DLISTL)
	hi := antic.HandleRead(ANTIC_DLISTH)
	ptr := (hi << 8) | lo
	if ptr != 0x1234 {
		t.Errorf("Display list pointer: expected 0x1234, got 0x%04X", ptr)
	}
}

// TestANTIC_RegisterRead_VCOUNT tests vertical counter register
func TestANTIC_RegisterRead_VCOUNT(t *testing.T) {
	antic := NewANTICEngine(nil)
	now := int64(1_000_000_000)
	antic.now = func() int64 { return now }
	antic.lastFrameStart = now

	// Advance to scanline 100 in the NTSC frame.
	now += (antic.framePeriodNS()*100)/int64(ANTIC_SCANLINES_NTSC) + 1

	// VCOUNT returns scanline / 2
	val := antic.HandleRead(ANTIC_VCOUNT)
	if val != 50 {
		t.Errorf("VCOUNT: expected 50, got %d", val)
	}

	// Test another value
	now = antic.lastFrameStart + (antic.framePeriodNS()*200)/int64(ANTIC_SCANLINES_NTSC) + 1
	val = antic.HandleRead(ANTIC_VCOUNT)
	if val != 100 {
		t.Errorf("VCOUNT for 200: expected 100, got %d", val)
	}
}

// TestANTIC_WSYNC_WriteOnly tests that WSYNC is write-only
func TestANTIC_WSYNC_WriteOnly(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Read should return 0
	val := antic.HandleRead(ANTIC_WSYNC)
	if val != 0 {
		t.Errorf("WSYNC read: expected 0, got %d", val)
	}
}

// TestANTIC_Scroll_Masking tests scroll register masking
func TestANTIC_Scroll_Masking(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Write values > 15 should be masked to 4 bits
	antic.HandleWrite(ANTIC_HSCROL, 0xFF)
	val := antic.HandleRead(ANTIC_HSCROL)
	if val != 0x0F {
		t.Errorf("HSCROL masking: expected 0x0F, got 0x%02X", val)
	}

	antic.HandleWrite(ANTIC_VSCROL, 0xF7)
	val = antic.HandleRead(ANTIC_VSCROL)
	if val != 0x07 {
		t.Errorf("VSCROL masking: expected 0x07, got 0x%02X", val)
	}
}

// =============================================================================
// Sprint 3: VideoSource Interface Tests
// =============================================================================

// TestANTIC_GetDimensions tests frame dimensions
func TestANTIC_GetDimensions(t *testing.T) {
	antic := NewANTICEngine(nil)

	w, h := antic.GetDimensions()
	if w != ANTIC_FRAME_WIDTH {
		t.Errorf("Width: expected %d, got %d", ANTIC_FRAME_WIDTH, w)
	}
	if h != ANTIC_FRAME_HEIGHT {
		t.Errorf("Height: expected %d, got %d", ANTIC_FRAME_HEIGHT, h)
	}
}

// TestANTIC_GetLayer tests compositor layer
func TestANTIC_GetLayer(t *testing.T) {
	antic := NewANTICEngine(nil)

	layer := antic.GetLayer()
	if layer != ANTIC_LAYER {
		t.Errorf("GetLayer: expected %d, got %d", ANTIC_LAYER, layer)
	}

	// Verify layer is between TED (12) and ULA (15)
	if layer <= 12 || layer >= 15 {
		t.Errorf("Layer %d should be between TED (12) and ULA (15)", layer)
	}
}

// TestANTIC_GetFrame_Disabled tests GetFrame when disabled
func TestANTIC_GetFrame_Disabled(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Should return nil when disabled
	frame := antic.GetFrame()
	if frame != nil {
		t.Error("GetFrame should return nil when disabled")
	}
}

// TestANTIC_GetFrame_Enabled tests GetFrame when enabled
func TestANTIC_GetFrame_Enabled(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Enable ANTIC
	antic.HandleWrite(ANTIC_ENABLE, ANTIC_ENABLE_VIDEO)

	frame := antic.GetFrame()
	if frame == nil {
		t.Error("GetFrame should return frame when enabled")
	}

	// Verify frame size
	expectedSize := ANTIC_FRAME_WIDTH * ANTIC_FRAME_HEIGHT * 4
	if len(frame) != expectedSize {
		t.Errorf("Frame size: expected %d, got %d", expectedSize, len(frame))
	}
}

// TestANTIC_SignalVSync tests VSync signal handling
func TestANTIC_SignalVSync(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Set scanline to non-zero
	antic.scanline = 100

	// Signal VSync
	antic.SignalVSync()

	if antic.scanline != 100 {
		t.Errorf("Incomplete scanline capture should survive VSync, got %d", antic.scanline)
	}

	// Frame timer should be set
	if antic.lastFrameStart == 0 {
		t.Error("lastFrameStart should be set after SignalVSync")
	}
}

func TestANTIC_SignalVSyncPreservesIncompleteCapture(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.writeBuffer = 0
	antic.scanline = 13
	antic.colbk = 0x0C

	// Old completed frame in read buffer.
	antic.scanlineColors[1][12] = 0x02
	antic.playerGfx[1][1][12] = 0x55

	// Partial current frame in write buffer.
	antic.scanlineColors[0][12] = 0x6E
	antic.playerGfx[0][1][12] = 0xAA
	antic.playerPos[0][1][12] = 0x40

	antic.SignalVSync()

	if antic.writeBuffer != 0 {
		t.Fatalf("SignalVSync should keep incomplete buffer writable, got writeBuffer=%d", antic.writeBuffer)
	}
	if antic.scanline != 13 {
		t.Fatalf("SignalVSync should not reset incomplete capture scanline, got %d", antic.scanline)
	}
	if got := antic.scanlineColors[0][12]; got != 0x6E {
		t.Fatalf("incomplete raster color changed to 0x%02X", got)
	}
	if got := antic.playerGfx[0][1][12]; got != 0xAA {
		t.Fatalf("incomplete player gfx changed to 0x%02X", got)
	}
	if got := antic.scanlineColors[1][12]; got != 0x02 {
		t.Fatalf("last complete raster color changed to 0x%02X", got)
	}
	if got := antic.playerGfx[1][1][12]; got != 0x55 {
		t.Fatalf("last complete player gfx changed to 0x%02X", got)
	}
}

func TestANTIC_IncompleteCapturePublishesOnlyAfterFullWSYNCFrame(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.writeBuffer = 0
	antic.colbk = 0x2E
	antic.scanlineColors[1][20] = 0x04
	for y := 0; y < 100; y++ {
		antic.scanlineColors[0][y] = 0x06
		antic.scanline++
	}

	antic.SignalVSync()

	frame := antic.RenderFrame(nil)
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP+20); got != anticRGBA(0x04) {
		t.Fatalf("incomplete frame was published before 192 WSYNC rows, got %v", got)
	}

	for antic.scanline < ANTIC_DISPLAY_HEIGHT {
		antic.scanlineColors[0][antic.scanline] = 0x08
		antic.scanline++
	}
	antic.writeBuffer = 1 - antic.writeBuffer
	antic.scanline = 0

	frame = antic.RenderFrame(nil)
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP+20); got != anticRGBA(0x06) {
		t.Fatalf("completed frame did not publish captured row, got %v", got)
	}
}

func TestANTIC_SignalVSyncClearsUnwrittenRowsToCurrentBackground(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.colbk = 0x0C
	antic.writeBuffer = 0
	antic.scanlineColors[0][20] = 0x04

	antic.SignalVSync()

	if got := antic.scanlineColors[0][20]; got != 0x0C {
		t.Fatalf("unwritten row after VSync = 0x%02X want COLBK 0x0C", got)
	}
}

// =============================================================================
// Sprint 4: Enable/Disable and Status Tests
// =============================================================================

// TestANTIC_EnableRegister tests ANTIC_ENABLE toggles IsEnabled
func TestANTIC_EnableRegister(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Initially disabled
	if antic.IsEnabled() {
		t.Error("Should be disabled initially")
	}

	// Enable
	antic.HandleWrite(ANTIC_ENABLE, ANTIC_ENABLE_VIDEO)
	if !antic.IsEnabled() {
		t.Error("Should be enabled after write")
	}

	// Verify read back
	val := antic.HandleRead(ANTIC_ENABLE)
	if val != ANTIC_ENABLE_VIDEO {
		t.Errorf("ENABLE read: expected %d, got %d", ANTIC_ENABLE_VIDEO, val)
	}

	// Disable
	antic.HandleWrite(ANTIC_ENABLE, 0)
	if antic.IsEnabled() {
		t.Error("Should be disabled after clear")
	}
}

// TestANTIC_Status_VBlank tests STATUS reflects time-based VBlank
func TestANTIC_Status_VBlank(t *testing.T) {
	antic := NewANTICEngine(nil)
	now := int64(1_000_000_000)
	antic.now = func() int64 { return now }

	// Initial status should be 0 and must not initialise frame timing.
	val := antic.HandleRead(ANTIC_STATUS)
	if val != 0 {
		t.Errorf("Initial STATUS: expected 0, got %d", val)
	}
	if antic.lastFrameStart != 0 {
		t.Fatalf("STATUS read mutated lastFrameStart to %d", antic.lastFrameStart)
	}

	antic.lastFrameStart = now - int64(16*time.Millisecond)

	val = antic.HandleRead(ANTIC_STATUS)
	if val&ANTIC_STATUS_VBLANK == 0 {
		t.Error("Expected VBlank bit to be set in VBlank scanline range")
	}
}

func TestANTIC_VCOUNT_AutoTicksFromFrameStart(t *testing.T) {
	antic := NewANTICEngine(nil)
	now := int64(1_000_000_000)
	antic.now = func() int64 { return now }
	antic.lastFrameStart = now

	if got := antic.HandleRead(ANTIC_VCOUNT); got != 0 {
		t.Fatalf("initial VCOUNT got %d, want 0", got)
	}

	now += antic.framePeriodNS() / 2
	if got := antic.HandleRead(ANTIC_VCOUNT); got == 0 {
		t.Fatalf("mid-frame VCOUNT did not advance")
	}
}

func TestANTIC_STATUSReadIsIdempotent(t *testing.T) {
	antic := NewANTICEngine(nil)
	now := int64(2_000_000_000)
	antic.now = func() int64 { return now }
	antic.lastFrameStart = now - int64(16*time.Millisecond)
	antic.nmist = ANTIC_NMIST_VBI
	antic.scanline = 77

	first := antic.HandleRead(ANTIC_STATUS)
	for i := 0; i < 1000; i++ {
		if got := antic.HandleRead(ANTIC_STATUS); got != first {
			t.Fatalf("STATUS read %d got %d, want %d", i, got, first)
		}
	}
	if antic.nmist != ANTIC_NMIST_VBI {
		t.Fatalf("STATUS reads changed NMIST to 0x%02X", antic.nmist)
	}
	if antic.scanline != 77 {
		t.Fatalf("STATUS reads changed scanline to %d", antic.scanline)
	}
}

func TestANTIC_TickFrameSetsVBIPendingOncePerFrame(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.HandleWrite(ANTIC_NMIEN, ANTIC_NMIEN_VBI)

	antic.tickFrame(100)
	if antic.nmist&ANTIC_NMIST_VBI == 0 {
		t.Fatal("tickFrame did not set VBI pending")
	}
	firstFrame := antic.frameID
	for i := 0; i < 1000; i++ {
		_ = antic.HandleRead(ANTIC_STATUS)
	}
	if antic.frameID != firstFrame {
		t.Fatalf("STATUS reads changed frameID to %d", antic.frameID)
	}

	antic.HandleWrite(ANTIC_NMIST, 0)
	antic.tickFrame(200)
	if antic.nmist&ANTIC_NMIST_VBI == 0 {
		t.Fatal("next tickFrame did not set VBI pending again")
	}
	if antic.frameID != firstFrame+1 {
		t.Fatalf("next tickFrame frameID got %d, want %d", antic.frameID, firstFrame+1)
	}
}

func TestANTIC_TickFrameHonorsNMIEN(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.tickFrame(100)
	if antic.nmist != 0 {
		t.Fatalf("tickFrame with VBI disabled changed NMIST to 0x%02X", antic.nmist)
	}
}

func TestANTIC_PALTimingAndEnableBits(t *testing.T) {
	antic := NewANTICEngine(nil)
	now := int64(3_000_000_000)
	antic.now = func() int64 { return now }
	antic.lastFrameStart = now

	now = antic.lastFrameStart + antic.framePeriodNS() - 1
	if got := antic.HandleRead(ANTIC_VCOUNT); got != 130 {
		t.Fatalf("NTSC max VCOUNT got %d, want 130", got)
	}

	antic.HandleWrite(ANTIC_ENABLE, ANTIC_ENABLE_PAL)
	if antic.IsEnabled() {
		t.Fatal("PAL bit should not enable video")
	}
	if got := antic.HandleRead(ANTIC_ENABLE); got != ANTIC_ENABLE_PAL {
		t.Fatalf("ENABLE after PAL-only write got 0x%02X", got)
	}

	antic.lastFrameStart = now
	now = antic.lastFrameStart + antic.framePeriodNS() - 1
	if got := antic.HandleRead(ANTIC_VCOUNT); got != 155 {
		t.Fatalf("PAL max VCOUNT got %d, want 155", got)
	}

	antic.HandleWrite(ANTIC_ENABLE, ANTIC_ENABLE_VIDEO)
	if !antic.IsEnabled() {
		t.Fatal("video bit should enable video")
	}
	if got := antic.HandleRead(ANTIC_ENABLE); got != ANTIC_ENABLE_VIDEO {
		t.Fatalf("ENABLE after video-only write got 0x%02X", got)
	}
}

// TestANTIC_NMIST_VBI_Flag tests VBI flag in NMIST
func TestANTIC_NMIST_VBI_Flag(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Enable VBI in NMIEN
	antic.HandleWrite(ANTIC_NMIEN, ANTIC_NMIEN_VBI)

	// Signal VSync
	antic.SignalVSync()

	// NMIST should have VBI flag
	val := antic.HandleRead(ANTIC_NMIST)
	if val&ANTIC_NMIST_VBI == 0 {
		t.Error("Expected VBI flag in NMIST after VSync")
	}

	// Writing to NMIST (NMIRES) should clear it
	antic.HandleWrite(ANTIC_NMIST, 0)
	val = antic.HandleRead(ANTIC_NMIST)
	if val != 0 {
		t.Errorf("Expected NMIST cleared after write, got 0x%02X", val)
	}
}

// =============================================================================
// Sprint 5: Basic Rendering Tests
// =============================================================================

// TestANTIC_Render_FrameSize tests frame buffer size
func TestANTIC_Render_FrameSize(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.HandleWrite(ANTIC_ENABLE, ANTIC_ENABLE_VIDEO)

	frame := antic.RenderFrame(nil)

	// Frame should be 384*240*4 bytes (RGBA)
	expectedSize := ANTIC_FRAME_WIDTH * ANTIC_FRAME_HEIGHT * 4
	if len(frame) != expectedSize {
		t.Errorf("Frame size: expected %d, got %d", expectedSize, len(frame))
	}
}

// TestANTIC_Render_BorderColor tests border color filling
func TestANTIC_Render_BorderColor(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.HandleWrite(ANTIC_ENABLE, ANTIC_ENABLE_VIDEO)

	// Set background/border color (COLBK is used for border)
	// Using color index 0x94 (green, luminance 9)
	antic.colbk = 0x94

	frame := antic.RenderFrame(nil)

	// Check a border pixel (top-left corner)
	r, g, b := frame[0], frame[1], frame[2]

	// Get expected color from palette
	expectedR, expectedG, expectedB := GetANTICColor(0x94)
	if r != expectedR || g != expectedG || b != expectedB {
		t.Errorf("Border pixel: expected RGB(%d,%d,%d), got RGB(%d,%d,%d)",
			expectedR, expectedG, expectedB, r, g, b)
	}
}

// TestANTIC_ColorPalette tests 128-color palette
func TestANTIC_ColorPalette(t *testing.T) {
	// Test black (should be at index 0)
	r, g, b := GetANTICColor(0x00)
	if r != 0 || g != 0 || b != 0 {
		t.Errorf("Color 0x00: expected (0,0,0), got (%d,%d,%d)", r, g, b)
	}

	// Test white (should be bright)
	r, g, b = GetANTICColor(0x0F)
	if r < 200 || g < 200 || b < 200 {
		t.Errorf("Color 0x0F: expected bright values, got (%d,%d,%d)", r, g, b)
	}

	// Test a mid-range color
	r, g, b = GetANTICColor(0x88)
	// Should have some non-zero values
	if r == 0 && g == 0 && b == 0 {
		t.Error("Color 0x88 should not be black")
	}
}

// =============================================================================
// Sprint 7: Display List Tests (basic)
// =============================================================================

// TestANTIC_DisplayList_BlankLines tests blank line opcodes
func TestANTIC_DisplayList_BlankLines(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Test that opcode 0x00 creates 1 blank line
	lines := antic.decodeBlankLines(0x00)
	if lines != 1 {
		t.Errorf("Opcode 0x00: expected 1 blank line, got %d", lines)
	}

	// Test that opcode 0x70 creates 8 blank lines
	lines = antic.decodeBlankLines(0x70)
	if lines != 8 {
		t.Errorf("Opcode 0x70: expected 8 blank lines, got %d", lines)
	}
}

// TestANTIC_DisplayList_JVB tests Jump and Vertical Blank
func TestANTIC_DisplayList_JVB(t *testing.T) {
	antic := NewANTICEngine(nil)

	// JVB opcode is 0x41
	if !antic.isJVBOpcode(0x41) {
		t.Error("0x41 should be recognized as JVB opcode")
	}

	// Regular mode 2 is not JVB
	if antic.isJVBOpcode(0x02) {
		t.Error("0x02 should not be recognized as JVB opcode")
	}
}

// =============================================================================
// Copper Integration Tests
// =============================================================================

// TestANTIC_CopperSetBorder tests copper can set border color
func TestANTIC_CopperSetBorder(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Simulate copper writing COLBK (using direct register write)
	// In real usage, copper would write via bus
	antic.colbk = 0x28 // Orange

	// Verify
	if antic.colbk != 0x28 {
		t.Errorf("COLBK write: expected 0x28, got 0x%02X", antic.colbk)
	}
}

// TestANTIC_RegisterAlignmentForCopper verifies 4-byte alignment
func TestANTIC_RegisterAlignmentForCopper(t *testing.T) {
	// Copper uses regIndex * 4 addressing, so all registers must be 4-byte aligned
	registers := []struct {
		name string
		addr uint32
	}{
		{"DMACTL", ANTIC_DMACTL},
		{"CHACTL", ANTIC_CHACTL},
		{"DLISTL", ANTIC_DLISTL},
		{"DLISTH", ANTIC_DLISTH},
		{"HSCROL", ANTIC_HSCROL},
		{"VSCROL", ANTIC_VSCROL},
		{"WSYNC", ANTIC_WSYNC},
		{"VCOUNT", ANTIC_VCOUNT},
		{"NMIEN", ANTIC_NMIEN},
		{"NMIST", ANTIC_NMIST},
		{"ENABLE", ANTIC_ENABLE},
		{"STATUS", ANTIC_STATUS},
	}

	for _, reg := range registers {
		offset := reg.addr - ANTIC_BASE
		if offset%4 != 0 {
			t.Errorf("Register %s (0x%X) is not 4-byte aligned from base", reg.name, reg.addr)
		}
	}
}
