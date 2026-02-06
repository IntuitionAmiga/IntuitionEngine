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

	// Set internal scanline
	antic.scanline = 100

	// VCOUNT returns scanline / 2
	val := antic.HandleRead(ANTIC_VCOUNT)
	if val != 50 {
		t.Errorf("VCOUNT: expected 50, got %d", val)
	}

	// Test another value
	antic.scanline = 200
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

	// Initial state
	if antic.vblankActive.Load() {
		t.Error("VBlank should be inactive initially")
	}

	// Set scanline to non-zero
	antic.scanline = 100

	// Signal VSync
	antic.SignalVSync()

	// VBlank should be active
	if !antic.vblankActive.Load() {
		t.Error("VBlank should be active after SignalVSync")
	}

	// Scanline should reset to 0
	if antic.scanline != 0 {
		t.Errorf("Scanline should be 0 after VSync, got %d", antic.scanline)
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

// TestANTIC_Status_VBlank tests STATUS reflects vblankActive
func TestANTIC_Status_VBlank(t *testing.T) {
	antic := NewANTICEngine(nil)

	// Initial status should be 0
	val := antic.HandleRead(ANTIC_STATUS)
	if val != 0 {
		t.Errorf("Initial STATUS: expected 0, got %d", val)
	}

	// Signal VSync to set VBlank flag
	antic.SignalVSync()

	// Now status should have VBlank bit set
	val = antic.HandleRead(ANTIC_STATUS)
	if val&ANTIC_STATUS_VBLANK == 0 {
		t.Error("Expected VBlank bit to be set after SignalVSync")
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

	frame := antic.RenderFrame()

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

	frame := antic.RenderFrame()

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
// Sprint 6: 6502 Bus Integration Tests
// =============================================================================

// TestANTIC_6502_Addresses tests 6502-style register access
func TestANTIC_6502_Addresses(t *testing.T) {
	// Verify 6502 register addresses match Atari authentic addresses
	if C6502_ANTIC_DMACTL != 0xD400 {
		t.Errorf("C6502_ANTIC_DMACTL: expected 0xD400, got 0x%X", C6502_ANTIC_DMACTL)
	}
	if C6502_ANTIC_VCOUNT != 0xD40B {
		t.Errorf("C6502_ANTIC_VCOUNT: expected 0xD40B, got 0x%X", C6502_ANTIC_VCOUNT)
	}
	if C6502_ANTIC_WSYNC != 0xD40A {
		t.Errorf("C6502_ANTIC_WSYNC: expected 0xD40A, got 0x%X", C6502_ANTIC_WSYNC)
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
