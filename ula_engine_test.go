// ula_engine_test.go - ZX Spectrum ULA video chip test suite for Intuition Engine

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

// TestULA_NewEngine tests basic construction
func TestULA_NewEngine(t *testing.T) {
	ula := NewULAEngine(nil)
	if ula == nil {
		t.Fatal("NewULAEngine returned nil")
	}
}

// TestULA_DefaultState tests default state after construction
func TestULA_DefaultState(t *testing.T) {
	ula := NewULAEngine(nil)

	// Border should be 0 (black)
	if ula.border != 0 {
		t.Errorf("Expected border=0, got %d", ula.border)
	}

	// Should be enabled by default
	if !ula.IsEnabled() {
		t.Error("Expected ULA to be enabled by default")
	}

	// Flash state should be off
	if ula.flashState {
		t.Error("Expected flashState to be false initially")
	}

	// Flash counter should be 0
	if ula.flashCounter != 0 {
		t.Errorf("Expected flashCounter=0, got %d", ula.flashCounter)
	}
}

// TestULA_BorderColorWrite tests writing border color
func TestULA_BorderColorWrite(t *testing.T) {
	ula := NewULAEngine(nil)

	// Write border color (only bits 0-2 should be used)
	ula.HandleWrite(ULA_BORDER, 0xFF) // Should mask to 7
	if ula.border != 7 {
		t.Errorf("Expected border=7, got %d", ula.border)
	}

	ula.HandleWrite(ULA_BORDER, 3)
	if ula.border != 3 {
		t.Errorf("Expected border=3, got %d", ula.border)
	}

	// Read back
	val := ula.HandleRead(ULA_BORDER)
	if val != 3 {
		t.Errorf("Expected read border=3, got %d", val)
	}
}

// TestULA_ControlRegister tests enable/disable control
func TestULA_ControlRegister(t *testing.T) {
	ula := NewULAEngine(nil)

	// Disable ULA
	ula.HandleWrite(ULA_CTRL, 0)
	if ula.IsEnabled() {
		t.Error("Expected ULA to be disabled")
	}

	// Enable ULA
	ula.HandleWrite(ULA_CTRL, ULA_CTRL_ENABLE)
	if !ula.IsEnabled() {
		t.Error("Expected ULA to be enabled")
	}

	// Read back
	val := ula.HandleRead(ULA_CTRL)
	if val != ULA_CTRL_ENABLE {
		t.Errorf("Expected control=%d, got %d", ULA_CTRL_ENABLE, val)
	}
}

// TestULA_BitmapAddress_Formula tests non-linear bitmap addressing
func TestULA_BitmapAddress_Formula(t *testing.T) {
	ula := NewULAEngine(nil)

	// Test ZX Spectrum's peculiar screen addressing formula
	// Address = ((y & 0xC0) << 5) + ((y & 0x07) << 8) + ((y & 0x38) << 2) + (x >> 3)
	testCases := []struct {
		y, x         int
		expectedAddr uint16
	}{
		{0, 0, 0x0000},     // Top-left
		{0, 8, 0x0001},     // Second byte, first row
		{1, 0, 0x0100},     // y=1 shifts by 256
		{8, 0, 0x0020},     // y=8 shifts by 32
		{64, 0, 0x0800},    // y=64 shifts by 2048
		{191, 248, 0x17FF}, // Bottom-right
	}

	for _, tc := range testCases {
		addr := ula.GetBitmapAddress(tc.y, tc.x)
		if addr != tc.expectedAddr {
			t.Errorf("GetBitmapAddress(%d, %d) = 0x%04X, expected 0x%04X",
				tc.y, tc.x, addr, tc.expectedAddr)
		}
	}
}

// TestULA_AttributeAddress tests linear attribute addressing
func TestULA_AttributeAddress(t *testing.T) {
	ula := NewULAEngine(nil)

	// Attributes are stored linearly: 32 bytes per row, 24 rows
	testCases := []struct {
		y, x         int
		expectedAddr uint16
	}{
		{0, 0, 0x1800},   // First attribute
		{0, 31, 0x181F},  // End of first row
		{1, 0, 0x1820},   // Start of second row
		{23, 31, 0x1AFF}, // Last attribute
	}

	for _, tc := range testCases {
		addr := ula.GetAttributeAddress(tc.y, tc.x)
		if addr != tc.expectedAddr {
			t.Errorf("GetAttributeAddress(%d, %d) = 0x%04X, expected 0x%04X",
				tc.y, tc.x, addr, tc.expectedAddr)
		}
	}
}

// TestULA_AttributeParsing tests INK/PAPER/BRIGHT/FLASH extraction
func TestULA_AttributeParsing(t *testing.T) {
	testCases := []struct {
		attr          uint8
		ink, paper    uint8
		bright, flash bool
	}{
		{0x00, 0, 0, false, false}, // All black, no effects
		{0x07, 7, 0, false, false}, // White ink, black paper
		{0x38, 0, 7, false, false}, // Black ink, white paper
		{0x3F, 7, 7, false, false}, // White on white
		{0x47, 7, 0, true, false},  // Bright white ink
		{0x87, 7, 0, false, true},  // Flash white ink
		{0xC7, 7, 0, true, true},   // Bright + flash white ink
		{0xFF, 7, 7, true, true},   // Everything on
	}

	for _, tc := range testCases {
		ink, paper, bright, flash := ParseAttribute(tc.attr)
		if ink != tc.ink {
			t.Errorf("ParseAttribute(0x%02X): ink=%d, expected %d", tc.attr, ink, tc.ink)
		}
		if paper != tc.paper {
			t.Errorf("ParseAttribute(0x%02X): paper=%d, expected %d", tc.attr, paper, tc.paper)
		}
		if bright != tc.bright {
			t.Errorf("ParseAttribute(0x%02X): bright=%v, expected %v", tc.attr, bright, tc.bright)
		}
		if flash != tc.flash {
			t.Errorf("ParseAttribute(0x%02X): flash=%v, expected %v", tc.attr, flash, tc.flash)
		}
	}
}

// TestULA_Palette_Normal tests the 8 base colors
func TestULA_Palette_Normal(t *testing.T) {
	ula := NewULAEngine(nil)

	// Normal palette (non-bright)
	expectedColors := [][3]uint8{
		{0, 0, 0},       // 0: Black
		{0, 0, 205},     // 1: Blue
		{205, 0, 0},     // 2: Red
		{205, 0, 205},   // 3: Magenta
		{0, 205, 0},     // 4: Green
		{0, 205, 205},   // 5: Cyan
		{205, 205, 0},   // 6: Yellow
		{205, 205, 205}, // 7: White
	}

	for i, expected := range expectedColors {
		r, g, b := ula.GetColor(uint8(i), false)
		if r != expected[0] || g != expected[1] || b != expected[2] {
			t.Errorf("Normal color %d: got (%d,%d,%d), expected (%d,%d,%d)",
				i, r, g, b, expected[0], expected[1], expected[2])
		}
	}
}

// TestULA_Palette_Bright tests the 8 bright colors
func TestULA_Palette_Bright(t *testing.T) {
	ula := NewULAEngine(nil)

	// Bright palette
	expectedColors := [][3]uint8{
		{0, 0, 0},       // 0: Black (same)
		{0, 0, 255},     // 1: Bright Blue
		{255, 0, 0},     // 2: Bright Red
		{255, 0, 255},   // 3: Bright Magenta
		{0, 255, 0},     // 4: Bright Green
		{0, 255, 255},   // 5: Bright Cyan
		{255, 255, 0},   // 6: Bright Yellow
		{255, 255, 255}, // 7: Bright White
	}

	for i, expected := range expectedColors {
		r, g, b := ula.GetColor(uint8(i), true)
		if r != expected[0] || g != expected[1] || b != expected[2] {
			t.Errorf("Bright color %d: got (%d,%d,%d), expected (%d,%d,%d)",
				i, r, g, b, expected[0], expected[1], expected[2])
		}
	}
}

// TestULA_VRAM_WriteRead tests memory access
func TestULA_VRAM_WriteRead(t *testing.T) {
	ula := NewULAEngine(nil)

	// Write to bitmap area
	ula.HandleVRAMWrite(0x0000, 0xAA)
	val := ula.HandleVRAMRead(0x0000)
	if val != 0xAA {
		t.Errorf("VRAM bitmap read: got 0x%02X, expected 0xAA", val)
	}

	// Write to attribute area
	ula.HandleVRAMWrite(0x1800, 0x47) // Bright white ink
	val = ula.HandleVRAMRead(0x1800)
	if val != 0x47 {
		t.Errorf("VRAM attribute read: got 0x%02X, expected 0x47", val)
	}

	// Test bounds checking - should not panic
	ula.HandleVRAMWrite(ULA_VRAM_SIZE, 0xFF) // Out of bounds
	val = ula.HandleVRAMRead(ULA_VRAM_SIZE)
	if val != 0 {
		t.Errorf("Out of bounds read: got 0x%02X, expected 0x00", val)
	}
}

// TestULA_Render_Dimensions tests output frame dimensions
func TestULA_Render_Dimensions(t *testing.T) {
	ula := NewULAEngine(nil)

	w, h := ula.GetDimensions()

	// 320x256 total (256x192 display + 32px border on each side)
	if w != ULA_FRAME_WIDTH {
		t.Errorf("Width: got %d, expected %d", w, ULA_FRAME_WIDTH)
	}
	if h != ULA_FRAME_HEIGHT {
		t.Errorf("Height: got %d, expected %d", h, ULA_FRAME_HEIGHT)
	}
}

// TestULA_Render_Border tests border color fill
func TestULA_Render_Border(t *testing.T) {
	ula := NewULAEngine(nil)

	// Set border to blue (1)
	ula.HandleWrite(ULA_BORDER, 1)

	// Render frame
	frame := ula.RenderFrame()
	if frame == nil {
		t.Fatal("RenderFrame returned nil")
	}

	// Check a border pixel (top-left corner, should be blue)
	// RGBA format, 4 bytes per pixel
	offset := 0 // First pixel
	r, g, b, a := frame[offset], frame[offset+1], frame[offset+2], frame[offset+3]

	// Blue color (non-bright): 0, 0, 205
	if r != 0 || g != 0 || b != 205 || a != 255 {
		t.Errorf("Border pixel at (0,0): got RGBA(%d,%d,%d,%d), expected (0,0,205,255)",
			r, g, b, a)
	}
}

// TestULA_Render_SinglePixel tests bitmap + attribute rendering
func TestULA_Render_SinglePixel(t *testing.T) {
	ula := NewULAEngine(nil)

	// Set first bitmap byte to 0x80 (pixel at x=0 is set)
	ula.HandleVRAMWrite(0x0000, 0x80)

	// Set first attribute to bright white ink (7) on black paper (0)
	// Attribute byte: FBPPPIII = 0_1_000_111 = 0x47
	ula.HandleVRAMWrite(0x1800, 0x47)

	// Render frame
	frame := ula.RenderFrame()

	// The display starts at x=32, y=32 (border offset)
	// First display pixel is at (32, 32) in frame coordinates
	offset := (ULA_BORDER_TOP*ULA_FRAME_WIDTH + ULA_BORDER_LEFT) * 4
	r, g, b := frame[offset], frame[offset+1], frame[offset+2]

	// Should be bright white (255, 255, 255) for ink where bit is set
	if r != 255 || g != 255 || b != 255 {
		t.Errorf("Ink pixel: got RGB(%d,%d,%d), expected (255,255,255)", r, g, b)
	}

	// Second pixel (x=1) should be paper (black)
	offset2 := (ULA_BORDER_TOP*ULA_FRAME_WIDTH + ULA_BORDER_LEFT + 1) * 4
	r2, g2, b2 := frame[offset2], frame[offset2+1], frame[offset2+2]

	if r2 != 0 || g2 != 0 || b2 != 0 {
		t.Errorf("Paper pixel: got RGB(%d,%d,%d), expected (0,0,0)", r2, g2, b2)
	}
}

// TestULA_Render_Flash tests INK/PAPER swap during flash
func TestULA_Render_Flash(t *testing.T) {
	ula := NewULAEngine(nil)

	// Set first bitmap byte to 0x80 (pixel at x=0 is set)
	ula.HandleVRAMWrite(0x0000, 0x80)

	// Set attribute with FLASH bit: white ink on black paper with flash
	// Attribute byte: FBPPPIII = 1_0_000_111 = 0x87
	ula.HandleVRAMWrite(0x1800, 0x87)

	// Render with flash off - ink should be white
	ula.flashState = false
	frame1 := ula.RenderFrame()
	offset := (ULA_BORDER_TOP*ULA_FRAME_WIDTH + ULA_BORDER_LEFT) * 4
	r1, g1, b1 := frame1[offset], frame1[offset+1], frame1[offset+2]

	// Flash off: ink color (white) where bit is set
	if r1 != 205 || g1 != 205 || b1 != 205 { // Normal white (not bright)
		t.Errorf("Flash off ink: got RGB(%d,%d,%d), expected (205,205,205)", r1, g1, b1)
	}

	// Render with flash on - colors should swap
	ula.flashState = true
	frame2 := ula.RenderFrame()
	r2, g2, b2 := frame2[offset], frame2[offset+1], frame2[offset+2]

	// Flash on: paper color (black) where bit is set (swapped)
	if r2 != 0 || g2 != 0 || b2 != 0 {
		t.Errorf("Flash on ink->paper: got RGB(%d,%d,%d), expected (0,0,0)", r2, g2, b2)
	}
}

// TestULA_Implements_VideoSource tests interface compliance
func TestULA_Implements_VideoSource(t *testing.T) {
	var _ VideoSource = (*ULAEngine)(nil) // Compile-time check
}

// TestULA_FlashTiming tests 32 frames = toggle
func TestULA_FlashTiming(t *testing.T) {
	ula := NewULAEngine(nil)

	// Initial state
	if ula.flashState {
		t.Error("Initial flashState should be false")
	}

	// Signal 31 vsyncs - should not toggle yet
	for i := 0; i < 31; i++ {
		ula.SignalVSync()
	}
	if ula.flashState {
		t.Error("flashState should still be false after 31 frames")
	}

	// 32nd vsync should toggle
	ula.SignalVSync()
	if !ula.flashState {
		t.Error("flashState should be true after 32 frames")
	}

	// Another 32 frames should toggle back
	for i := 0; i < 32; i++ {
		ula.SignalVSync()
	}
	if ula.flashState {
		t.Error("flashState should be false after 64 frames")
	}
}

// TestULA_GetFrame tests VideoSource interface method
func TestULA_GetFrame(t *testing.T) {
	ula := NewULAEngine(nil)

	// Enabled - should return frame
	frame := ula.GetFrame()
	if frame == nil {
		t.Error("GetFrame returned nil when enabled")
	}

	expectedLen := ULA_FRAME_WIDTH * ULA_FRAME_HEIGHT * 4 // RGBA
	if len(frame) != expectedLen {
		t.Errorf("Frame length: got %d, expected %d", len(frame), expectedLen)
	}

	// Disabled - should return nil
	ula.HandleWrite(ULA_CTRL, 0)
	frame = ula.GetFrame()
	if frame != nil {
		t.Error("GetFrame should return nil when disabled")
	}
}

// TestULA_GetLayer tests layer ordering
func TestULA_GetLayer(t *testing.T) {
	ula := NewULAEngine(nil)

	layer := ula.GetLayer()
	if layer != ULA_LAYER {
		t.Errorf("GetLayer: got %d, expected %d", layer, ULA_LAYER)
	}
}
