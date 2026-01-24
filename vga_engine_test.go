// vga_engine_test.go - TDD tests for VGA chip emulation

package main

import (
	"testing"
)

// =============================================================================
// Phase 1: DAC/Palette Foundation Tests
// =============================================================================

func TestVGA_DAC_WriteIndex(t *testing.T) {
	vga := NewVGAEngine(nil)

	// Write to DAC write index
	vga.HandleWrite(VGA_DAC_WINDEX, 42)

	// Read back the write index
	if vga.dacWriteIndex != 42 {
		t.Errorf("DAC write index: got %d, want 42", vga.dacWriteIndex)
	}
}

func TestVGA_DAC_WritePalette(t *testing.T) {
	vga := NewVGAEngine(nil)

	// Set write index to palette entry 10
	vga.HandleWrite(VGA_DAC_WINDEX, 10)

	// Write R, G, B values (VGA uses 6-bit values 0-63)
	vga.HandleWrite(VGA_DAC_DATA, 63) // Red
	vga.HandleWrite(VGA_DAC_DATA, 32) // Green
	vga.HandleWrite(VGA_DAC_DATA, 0)  // Blue

	// Verify palette entry 10
	r, g, b := vga.GetPaletteEntry(10)
	if r != 63 || g != 32 || b != 0 {
		t.Errorf("Palette[10]: got (%d,%d,%d), want (63,32,0)", r, g, b)
	}

	// Verify auto-increment: next write should go to entry 11
	vga.HandleWrite(VGA_DAC_DATA, 10) // Red for entry 11
	vga.HandleWrite(VGA_DAC_DATA, 20) // Green
	vga.HandleWrite(VGA_DAC_DATA, 30) // Blue

	r, g, b = vga.GetPaletteEntry(11)
	if r != 10 || g != 20 || b != 30 {
		t.Errorf("Palette[11]: got (%d,%d,%d), want (10,20,30)", r, g, b)
	}
}

func TestVGA_DAC_ReadPalette(t *testing.T) {
	vga := NewVGAEngine(nil)

	// Set up palette entry 5
	vga.SetPaletteEntry(5, 63, 31, 15)

	// Set read index
	vga.HandleWrite(VGA_DAC_RINDEX, 5)

	// Read R, G, B values
	r := vga.HandleRead(VGA_DAC_DATA)
	g := vga.HandleRead(VGA_DAC_DATA)
	b := vga.HandleRead(VGA_DAC_DATA)

	if r != 63 || g != 31 || b != 15 {
		t.Errorf("Read Palette[5]: got (%d,%d,%d), want (63,31,15)", r, g, b)
	}

	// Verify auto-increment: should now be reading entry 6
	vga.SetPaletteEntry(6, 1, 2, 3)
	r = vga.HandleRead(VGA_DAC_DATA)
	g = vga.HandleRead(VGA_DAC_DATA)
	b = vga.HandleRead(VGA_DAC_DATA)

	if r != 1 || g != 2 || b != 3 {
		t.Errorf("Read Palette[6]: got (%d,%d,%d), want (1,2,3)", r, g, b)
	}
}

func TestVGA_DAC_Mask(t *testing.T) {
	vga := NewVGAEngine(nil)

	// Default mask should be 0xFF (all bits)
	mask := vga.HandleRead(VGA_DAC_MASK)
	if mask != 0xFF {
		t.Errorf("Default DAC mask: got 0x%02X, want 0xFF", mask)
	}

	// Set a custom mask
	vga.HandleWrite(VGA_DAC_MASK, 0x0F)
	mask = vga.HandleRead(VGA_DAC_MASK)
	if mask != 0x0F {
		t.Errorf("Custom DAC mask: got 0x%02X, want 0x0F", mask)
	}

	// Mask affects palette index lookup
	vga.SetPaletteEntry(0, 10, 20, 30)
	vga.SetPaletteEntry(16, 40, 50, 60)

	// With mask 0x0F, index 16 (0x10) should become index 0
	maskedIndex := vga.ApplyDACMask(16)
	if maskedIndex != 0 {
		t.Errorf("Masked index 16: got %d, want 0", maskedIndex)
	}
}

func TestVGA_DAC_6BitTo8Bit(t *testing.T) {
	vga := NewVGAEngine(nil)

	// VGA DAC uses 6-bit values (0-63), but we need 8-bit output (0-255)
	// The conversion is: out = (in << 2) | (in >> 4)
	// This maps 0->0, 63->255, 32->130, etc.

	tests := []struct {
		in6bit  uint8
		out8bit uint8
	}{
		{0, 0},
		{63, 255},
		{32, 130}, // (32<<2) | (32>>4) = 128 | 2 = 130
		{1, 4},    // (1<<2) | (1>>4) = 4 | 0 = 4
		{16, 65},  // (16<<2) | (16>>4) = 64 | 1 = 65
	}

	for _, tc := range tests {
		result := vga.Expand6BitTo8Bit(tc.in6bit)
		if result != tc.out8bit {
			t.Errorf("6bit->8bit %d: got %d, want %d", tc.in6bit, result, tc.out8bit)
		}
	}
}

func TestVGA_DAC_GetRGBA(t *testing.T) {
	vga := NewVGAEngine(nil)

	// Set palette entry 100 with 6-bit values
	vga.SetPaletteEntry(100, 63, 0, 32) // Full red, no green, half blue

	// Get expanded 8-bit RGBA
	r, g, b, a := vga.GetPaletteRGBA(100)

	// Expected: R=255, G=0, B=130, A=255
	if r != 255 {
		t.Errorf("RGBA red: got %d, want 255", r)
	}
	if g != 0 {
		t.Errorf("RGBA green: got %d, want 0", g)
	}
	if b != 130 {
		t.Errorf("RGBA blue: got %d, want 130", b)
	}
	if a != 255 {
		t.Errorf("RGBA alpha: got %d, want 255", a)
	}
}

// =============================================================================
// Phase 2: Mode 13h (320x200x256) Tests
// =============================================================================

func TestVGA_Mode13h_SetMode(t *testing.T) {
	vga := NewVGAEngine(nil)

	// Set Mode 13h
	vga.HandleWrite(VGA_MODE, VGA_MODE_13H)

	// Verify mode is set
	mode := vga.HandleRead(VGA_MODE)
	if mode != VGA_MODE_13H {
		t.Errorf("Mode: got 0x%02X, want 0x%02X", mode, VGA_MODE_13H)
	}

	// Verify dimensions
	w, h := vga.GetModeDimensions()
	if w != VGA_MODE13H_WIDTH || h != VGA_MODE13H_HEIGHT {
		t.Errorf("Dimensions: got %dx%d, want %dx%d", w, h, VGA_MODE13H_WIDTH, VGA_MODE13H_HEIGHT)
	}

	// Verify linear mode (Chain-4 enabled)
	if !vga.IsLinearMode() {
		t.Error("Mode 13h should be linear mode")
	}
}

func TestVGA_Mode13h_WriteVRAM(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_13H)

	// Write to VRAM at offset 0
	vga.HandleVRAMWrite(VGA_VRAM_WINDOW, 42)

	// Read back
	val := vga.HandleVRAMRead(VGA_VRAM_WINDOW)
	if val != 42 {
		t.Errorf("VRAM[0]: got %d, want 42", val)
	}

	// Write at offset 1000
	vga.HandleVRAMWrite(VGA_VRAM_WINDOW+1000, 123)
	val = vga.HandleVRAMRead(VGA_VRAM_WINDOW + 1000)
	if val != 123 {
		t.Errorf("VRAM[1000]: got %d, want 123", val)
	}
}

func TestVGA_Mode13h_ReadVRAM(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_13H)

	// Fill a range of VRAM
	for i := uint32(0); i < 100; i++ {
		vga.HandleVRAMWrite(VGA_VRAM_WINDOW+i, uint32(i&0xFF))
	}

	// Read back and verify
	for i := uint32(0); i < 100; i++ {
		val := vga.HandleVRAMRead(VGA_VRAM_WINDOW + i)
		if val != i&0xFF {
			t.Errorf("VRAM[%d]: got %d, want %d", i, val, i&0xFF)
		}
	}
}

func TestVGA_Mode13h_Render(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_13H)

	// Set up a simple palette: entry 1 = red, entry 2 = green
	vga.SetPaletteEntry(1, 63, 0, 0) // Red
	vga.SetPaletteEntry(2, 0, 63, 0) // Green

	// Write pattern to VRAM
	vga.HandleVRAMWrite(VGA_VRAM_WINDOW, 1)   // Pixel 0 = color 1 (red)
	vga.HandleVRAMWrite(VGA_VRAM_WINDOW+1, 2) // Pixel 1 = color 2 (green)

	// Render to framebuffer
	fb := vga.RenderFrame()

	// Check pixel 0 (should be red: 255, 0, 0, 255)
	if fb[0] != 255 || fb[1] != 0 || fb[2] != 0 || fb[3] != 255 {
		t.Errorf("Pixel 0: got (%d,%d,%d,%d), want (255,0,0,255)",
			fb[0], fb[1], fb[2], fb[3])
	}

	// Check pixel 1 (should be green: 0, 255, 0, 255)
	if fb[4] != 0 || fb[5] != 255 || fb[6] != 0 || fb[7] != 255 {
		t.Errorf("Pixel 1: got (%d,%d,%d,%d), want (0,255,0,255)",
			fb[4], fb[5], fb[6], fb[7])
	}
}

func TestVGA_Mode13h_PaletteCycle(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_13H)

	// Set initial palette
	vga.SetPaletteEntry(1, 63, 0, 0) // Red

	// Write pixel
	vga.HandleVRAMWrite(VGA_VRAM_WINDOW, 1)

	// Render first frame
	fb1 := vga.RenderFrame()
	r1 := fb1[0]

	// Cycle palette (change entry 1 to blue)
	vga.SetPaletteEntry(1, 0, 0, 63)

	// Render second frame (same VRAM, different palette)
	fb2 := vga.RenderFrame()
	r2, g2, b2 := fb2[0], fb2[1], fb2[2]

	// First frame was red, second should be blue
	if r1 != 255 {
		t.Errorf("Frame 1 red: got %d, want 255", r1)
	}
	if r2 != 0 || g2 != 0 || b2 != 255 {
		t.Errorf("Frame 2: got (%d,%d,%d), want (0,0,255)", r2, g2, b2)
	}
}

// =============================================================================
// Phase 3: Mode 12h (640x480x16) Tests
// =============================================================================

func TestVGA_Mode12h_SetMode(t *testing.T) {
	vga := NewVGAEngine(nil)

	// Set Mode 12h
	vga.HandleWrite(VGA_MODE, VGA_MODE_12H)

	mode := vga.HandleRead(VGA_MODE)
	if mode != VGA_MODE_12H {
		t.Errorf("Mode: got 0x%02X, want 0x%02X", mode, VGA_MODE_12H)
	}

	w, h := vga.GetModeDimensions()
	if w != VGA_MODE12H_WIDTH || h != VGA_MODE12H_HEIGHT {
		t.Errorf("Dimensions: got %dx%d, want %dx%d", w, h, VGA_MODE12H_WIDTH, VGA_MODE12H_HEIGHT)
	}

	// Mode 12h is planar
	if vga.IsLinearMode() {
		t.Error("Mode 12h should be planar, not linear")
	}
}

func TestVGA_Mode12h_MapMask(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_12H)

	// Set map mask to plane 0 only
	vga.HandleWrite(VGA_SEQ_MAPMASK, 0x01)

	// Write to VRAM - should only affect plane 0
	vga.HandleVRAMWrite(VGA_VRAM_WINDOW, 0xFF)

	// Verify only plane 0 was written
	for plane := 0; plane < 4; plane++ {
		val := vga.ReadPlane(0, plane)
		if plane == 0 {
			if val != 0xFF {
				t.Errorf("Plane 0: got 0x%02X, want 0xFF", val)
			}
		} else {
			if val != 0 {
				t.Errorf("Plane %d: got 0x%02X, want 0x00", plane, val)
			}
		}
	}

	// Set map mask to all planes
	vga.HandleWrite(VGA_SEQ_MAPMASK, 0x0F)
	vga.HandleVRAMWrite(VGA_VRAM_WINDOW+1, 0xAA)

	// All planes should have the value
	for plane := 0; plane < 4; plane++ {
		val := vga.ReadPlane(1, plane)
		if val != 0xAA {
			t.Errorf("Plane %d at offset 1: got 0x%02X, want 0xAA", plane, val)
		}
	}
}

func TestVGA_Mode12h_ReadMap(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_12H)

	// Write different values to each plane
	for plane := 0; plane < 4; plane++ {
		vga.HandleWrite(VGA_SEQ_MAPMASK, uint32(1<<plane))
		vga.HandleVRAMWrite(VGA_VRAM_WINDOW, uint32(0x10+plane))
	}

	// Read from each plane using read map select
	for plane := 0; plane < 4; plane++ {
		vga.HandleWrite(VGA_GC_READMAP, uint32(plane))
		val := vga.HandleVRAMRead(VGA_VRAM_WINDOW)
		expected := uint32(0x10 + plane)
		if val != expected {
			t.Errorf("Read plane %d: got 0x%02X, want 0x%02X", plane, val, expected)
		}
	}
}

func TestVGA_Mode12h_BitMask(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_12H)

	// Enable all planes
	vga.HandleWrite(VGA_SEQ_MAPMASK, 0x0F)

	// First write: fill with 0xFF
	vga.HandleWrite(VGA_GC_BITMASK, 0xFF)
	vga.HandleVRAMWrite(VGA_VRAM_WINDOW, 0xFF)

	// Second write with bit mask 0x0F: only lower 4 bits affected
	vga.HandleWrite(VGA_GC_BITMASK, 0x0F)
	vga.HandleVRAMWrite(VGA_VRAM_WINDOW, 0x00)

	// Result should be 0xF0 (upper 4 bits preserved, lower 4 cleared)
	vga.HandleWrite(VGA_GC_READMAP, 0)
	val := vga.HandleVRAMRead(VGA_VRAM_WINDOW)
	if val != 0xF0 {
		t.Errorf("Bit mask result: got 0x%02X, want 0xF0", val)
	}
}

func TestVGA_Mode12h_Render(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_12H)

	// Default EGA/VGA 16-color palette should be set
	// Color 1 = blue (0, 0, 170), Color 2 = green (0, 170, 0)

	// Write pixel data: first byte = 8 pixels
	// Set all planes to create color index
	vga.HandleWrite(VGA_SEQ_MAPMASK, 0x0F)

	// We need to write the bit pattern for color 1 in first pixel position
	// For planar mode, we set bit 7 (leftmost pixel) in the planes we want
	// Color 1 = 0001 binary, so only plane 0 gets bit 7 set
	vga.HandleWrite(VGA_SEQ_MAPMASK, 0x01)     // Plane 0
	vga.HandleVRAMWrite(VGA_VRAM_WINDOW, 0x80) // Bit 7 set

	vga.HandleWrite(VGA_SEQ_MAPMASK, 0x0E)     // Planes 1,2,3
	vga.HandleVRAMWrite(VGA_VRAM_WINDOW, 0x00) // Bit 7 clear

	fb := vga.RenderFrame()

	// First pixel should be color 1 (blue in standard VGA palette)
	// Standard VGA: color 1 = 0, 0, 170 -> expanded to 0, 0, 170
	// Check blue component is non-zero
	if fb[2] == 0 {
		t.Error("Pixel 0 blue component should be non-zero for color 1")
	}
}

// =============================================================================
// Phase 4: Text Mode (80x25) Tests
// =============================================================================

func TestVGA_Text_SetMode(t *testing.T) {
	vga := NewVGAEngine(nil)

	vga.HandleWrite(VGA_MODE, VGA_MODE_TEXT)

	mode := vga.HandleRead(VGA_MODE)
	if mode != VGA_MODE_TEXT {
		t.Errorf("Mode: got 0x%02X, want 0x%02X", mode, VGA_MODE_TEXT)
	}

	// Text mode is 80x25 characters
	cols, rows := vga.GetTextDimensions()
	if cols != VGA_TEXT_COLS || rows != VGA_TEXT_ROWS {
		t.Errorf("Text dimensions: got %dx%d, want %dx%d", cols, rows, VGA_TEXT_COLS, VGA_TEXT_ROWS)
	}
}

func TestVGA_Text_WriteChar(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_TEXT)

	// Write 'A' (0x41) with attribute 0x07 (white on black) at position 0
	// Text buffer format: [char][attr][char][attr]...
	vga.HandleTextWrite(VGA_TEXT_WINDOW, 0x41)   // Character
	vga.HandleTextWrite(VGA_TEXT_WINDOW+1, 0x07) // Attribute

	// Read back
	ch := vga.HandleTextRead(VGA_TEXT_WINDOW)
	attr := vga.HandleTextRead(VGA_TEXT_WINDOW + 1)

	if ch != 0x41 {
		t.Errorf("Character: got 0x%02X, want 0x41", ch)
	}
	if attr != 0x07 {
		t.Errorf("Attribute: got 0x%02X, want 0x07", attr)
	}
}

func TestVGA_Text_FontROM(t *testing.T) {
	vga := NewVGAEngine(nil)

	// Get font data for character 'A' (0x41)
	fontData := vga.GetFontGlyph(0x41)

	// Font should be 16 bytes (8x16 font)
	if len(fontData) != VGA_FONT_HEIGHT {
		t.Errorf("Font glyph size: got %d, want %d", len(fontData), VGA_FONT_HEIGHT)
	}

	// Check that 'A' has some non-zero data (it's not a blank character)
	hasData := false
	for _, b := range fontData {
		if b != 0 {
			hasData = true
			break
		}
	}
	if !hasData {
		t.Error("Font glyph for 'A' should have non-zero data")
	}
}

func TestVGA_Text_CursorPos(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_TEXT)

	// Set cursor to position (10, 5) = offset 5*80+10 = 410
	offset := uint32(5*80 + 10)
	vga.HandleWrite(VGA_CRTC_INDEX, VGA_CRTC_CURSOR_HI)
	vga.HandleWrite(VGA_CRTC_DATA, uint32(offset>>8))
	vga.HandleWrite(VGA_CRTC_INDEX, VGA_CRTC_CURSOR_LO)
	vga.HandleWrite(VGA_CRTC_DATA, uint32(offset&0xFF))

	// Read back cursor position
	col, row := vga.GetCursorPosition()
	if col != 10 || row != 5 {
		t.Errorf("Cursor position: got (%d,%d), want (10,5)", col, row)
	}
}

func TestVGA_Text_Render(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_TEXT)

	// Write 'X' at position 0 with white on black attribute
	vga.HandleTextWrite(VGA_TEXT_WINDOW, 'X')
	vga.HandleTextWrite(VGA_TEXT_WINDOW+1, 0x0F) // Bright white on black

	fb := vga.RenderFrame()

	// Text mode renders at 640x400 (or 720x400)
	// Check that there's some white pixels in the first character cell
	hasWhite := false
	// Character cell is 8 pixels wide, 16 tall
	for y := 0; y < VGA_FONT_HEIGHT; y++ {
		for x := 0; x < VGA_FONT_WIDTH; x++ {
			idx := (y*640 + x) * 4 // Assuming 640 width for text mode
			if idx+3 < len(fb) {
				if fb[idx] > 0 || fb[idx+1] > 0 || fb[idx+2] > 0 {
					hasWhite = true
					break
				}
			}
		}
		if hasWhite {
			break
		}
	}

	if !hasWhite {
		t.Error("Rendered 'X' should have some non-black pixels")
	}
}

// =============================================================================
// Phase 5: Mode-X (320x240 Planar) Tests
// =============================================================================

func TestVGA_ModeX_SetMode(t *testing.T) {
	vga := NewVGAEngine(nil)

	vga.HandleWrite(VGA_MODE, VGA_MODE_X)

	mode := vga.HandleRead(VGA_MODE)
	if mode != VGA_MODE_X {
		t.Errorf("Mode: got 0x%02X, want 0x%02X", mode, VGA_MODE_X)
	}

	w, h := vga.GetModeDimensions()
	if w != VGA_MODEX_WIDTH || h != VGA_MODEX_HEIGHT {
		t.Errorf("Dimensions: got %dx%d, want %dx%d", w, h, VGA_MODEX_WIDTH, VGA_MODEX_HEIGHT)
	}
}

func TestVGA_ModeX_Unchained(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_X)

	// Mode X is planar (unchained)
	if vga.IsLinearMode() {
		t.Error("Mode X should be planar (unchained), not linear")
	}

	// Verify Chain-4 is disabled in sequencer memory mode
	vga.HandleWrite(VGA_SEQ_INDEX, VGA_SEQ_MEMMODE)
	memMode := vga.HandleRead(VGA_SEQ_DATA)
	if memMode&VGA_SEQ_MEMMODE_CHAIN4 != 0 {
		t.Error("Mode X should have Chain-4 disabled")
	}
}

func TestVGA_ModeX_PageFlip(t *testing.T) {
	vga := NewVGAEngine(nil)
	vga.HandleWrite(VGA_MODE, VGA_MODE_X)

	// Page size in Mode X: 320*240/4 = 19200 bytes per plane
	// Set start address for page 1
	pageOffset := uint32(19200)
	vga.HandleWrite(VGA_CRTC_INDEX, VGA_CRTC_START_HI)
	vga.HandleWrite(VGA_CRTC_DATA, pageOffset>>8)
	vga.HandleWrite(VGA_CRTC_INDEX, VGA_CRTC_START_LO)
	vga.HandleWrite(VGA_CRTC_DATA, pageOffset&0xFF)

	// Verify start address
	startAddr := vga.GetStartAddress()
	if startAddr != pageOffset {
		t.Errorf("Start address: got %d, want %d", startAddr, pageOffset)
	}
}

// =============================================================================
// Phase 6: Integration Tests
// =============================================================================

func TestVGA_Integration_VideoChip(t *testing.T) {
	// Create VGA engine without video chip for testing
	vga := NewVGAEngine(nil)

	// Set Mode 13h
	vga.HandleWrite(VGA_MODE, VGA_MODE_13H)
	vga.HandleWrite(VGA_CTRL, VGA_CTRL_ENABLE)

	// Draw a pattern
	vga.SetPaletteEntry(1, 63, 0, 0) // Red
	for i := uint32(0); i < 100; i++ {
		vga.HandleVRAMWrite(VGA_VRAM_WINDOW+i, 1)
	}

	// Render to framebuffer
	fb := vga.RenderFrame()

	// Check that some pixels are non-black
	hasColor := false
	for i := 0; i < len(fb); i += 4 {
		if fb[i] > 0 { // Red channel
			hasColor = true
			break
		}
	}

	if !hasColor {
		t.Error("Framebuffer should have colored pixels after VGA render")
	}
}

func TestVGA_Status_VSync(t *testing.T) {
	vga := NewVGAEngine(nil)

	// VSync flag should toggle based on timing
	// Simulate waiting for vsync
	vga.SetVSync(true)
	status := vga.HandleRead(VGA_STATUS)
	if status&VGA_STATUS_VSYNC == 0 {
		t.Error("VSync flag should be set")
	}

	vga.SetVSync(false)
	status = vga.HandleRead(VGA_STATUS)
	if status&VGA_STATUS_VSYNC != 0 {
		t.Error("VSync flag should be clear")
	}
}

func TestVGA_Sequencer_Registers(t *testing.T) {
	vga := NewVGAEngine(nil)

	// Test indexed access to sequencer registers
	vga.HandleWrite(VGA_SEQ_INDEX, VGA_SEQ_MAPMASK_R)
	vga.HandleWrite(VGA_SEQ_DATA, 0x0F)

	vga.HandleWrite(VGA_SEQ_INDEX, VGA_SEQ_MAPMASK_R)
	val := vga.HandleRead(VGA_SEQ_DATA)
	if val != 0x0F {
		t.Errorf("Sequencer map mask: got 0x%02X, want 0x0F", val)
	}
}

func TestVGA_GraphicsController_Registers(t *testing.T) {
	vga := NewVGAEngine(nil)

	// Test indexed access to GC registers
	vga.HandleWrite(VGA_GC_INDEX, VGA_GC_BITMASK_R)
	vga.HandleWrite(VGA_GC_DATA, 0xAA)

	vga.HandleWrite(VGA_GC_INDEX, VGA_GC_BITMASK_R)
	val := vga.HandleRead(VGA_GC_DATA)
	if val != 0xAA {
		t.Errorf("GC bit mask: got 0x%02X, want 0xAA", val)
	}
}

func TestVGA_CRTC_Registers(t *testing.T) {
	vga := NewVGAEngine(nil)

	// Test indexed access to CRTC registers
	vga.HandleWrite(VGA_CRTC_INDEX, VGA_CRTC_OFFSET)
	vga.HandleWrite(VGA_CRTC_DATA, 40) // 80 bytes per line / 2

	vga.HandleWrite(VGA_CRTC_INDEX, VGA_CRTC_OFFSET)
	val := vga.HandleRead(VGA_CRTC_DATA)
	if val != 40 {
		t.Errorf("CRTC offset: got %d, want 40", val)
	}
}

func TestVGA_DefaultPalette(t *testing.T) {
	vga := NewVGAEngine(nil)

	// VGA should have standard 16-color palette set by default
	// Color 0 = black, Color 1 = blue, Color 2 = green, etc.

	// Check color 0 (black)
	r, g, b := vga.GetPaletteEntry(0)
	if r != 0 || g != 0 || b != 0 {
		t.Errorf("Palette[0] (black): got (%d,%d,%d), want (0,0,0)", r, g, b)
	}

	// Check color 15 (white/bright white)
	r, g, b = vga.GetPaletteEntry(15)
	if r != 63 || g != 63 || b != 63 {
		t.Errorf("Palette[15] (white): got (%d,%d,%d), want (63,63,63)", r, g, b)
	}
}
