package main

import (
	"sync"
	"testing"
)

// newCLUT8TestRig creates a VideoChip+MachineBus configured for CLUT8 testing.
func newCLUT8TestRig(t *testing.T) (*VideoChip, *MachineBus) {
	t.Helper()
	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(true)
	bus.UnmapIO(VRAM_START, VRAM_START+VRAM_SIZE-1)
	video.SetDirectVRAM(bus.memory[VRAM_START : VRAM_START+VRAM_SIZE])
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	return video, bus
}

func TestVideoChip_CLUT8_PaletteWrite(t *testing.T) {
	video, bus := newCLUT8TestRig(t)

	// Write palette entries via PAL_INDEX + PAL_DATA (auto-increment)
	bus.Write32(VIDEO_PAL_INDEX, 0)
	bus.Write32(VIDEO_PAL_DATA, 0x00FF0000) // Entry 0: Red
	bus.Write32(VIDEO_PAL_DATA, 0x0000FF00) // Entry 1: Green
	bus.Write32(VIDEO_PAL_DATA, 0x000000FF) // Entry 2: Blue

	// Verify auto-increment: index should now be 3
	gotIdx := video.HandleRead(VIDEO_PAL_INDEX)
	if gotIdx != 3 {
		t.Fatalf("PAL_INDEX after 3 writes: got %d, want 3", gotIdx)
	}

	// Verify palette entries via HW readback
	got0 := video.HandleRead(VIDEO_PAL_TABLE + 0*4)
	got1 := video.HandleRead(VIDEO_PAL_TABLE + 1*4)
	got2 := video.HandleRead(VIDEO_PAL_TABLE + 2*4)
	if got0 != 0x00FF0000 {
		t.Fatalf("PAL_TABLE[0] HW: got 0x%08X, want 0x00FF0000", got0)
	}
	if got1 != 0x0000FF00 {
		t.Fatalf("PAL_TABLE[1] HW: got 0x%08X, want 0x0000FF00", got1)
	}
	if got2 != 0x000000FF {
		t.Fatalf("PAL_TABLE[2] HW: got 0x%08X, want 0x000000FF", got2)
	}

	// Verify pre-packed LE RGBA values
	// Entry 0 (0x00FF0000 = R=0xFF,G=0x00,B=0x00): LE RGBA = 0xFF0000FF
	if video.clutPalette[0] != 0xFF0000FF {
		t.Fatalf("clutPalette[0]: got 0x%08X, want 0xFF0000FF", video.clutPalette[0])
	}
	// Entry 1 (0x0000FF00 = R=0x00,G=0xFF,B=0x00): LE RGBA = 0xFF00FF00
	if video.clutPalette[1] != 0xFF00FF00 {
		t.Fatalf("clutPalette[1]: got 0x%08X, want 0xFF00FF00", video.clutPalette[1])
	}
	// Entry 2 (0x000000FF = R=0x00,G=0x00,B=0xFF): LE RGBA = 0xFFFF0000
	if video.clutPalette[2] != 0xFFFF0000 {
		t.Fatalf("clutPalette[2]: got 0x%08X, want 0xFFFF0000", video.clutPalette[2])
	}

	// Test direct palette table write
	bus.Write32(VIDEO_PAL_TABLE+10*4, 0x00AABBCC)
	got10 := video.HandleRead(VIDEO_PAL_TABLE + 10*4)
	if got10 != 0x00AABBCC {
		t.Fatalf("PAL_TABLE[10] after direct write: got 0x%08X, want 0x00AABBCC", got10)
	}

	// Test index wrapping at 256
	bus.Write32(VIDEO_PAL_INDEX, 255)
	bus.Write32(VIDEO_PAL_DATA, 0x00112233) // Entry 255
	gotIdx = video.HandleRead(VIDEO_PAL_INDEX)
	if gotIdx != 0 {
		t.Fatalf("PAL_INDEX after wrap: got %d, want 0", gotIdx)
	}
}

func TestVideoChip_CLUT8_GetFrame(t *testing.T) {
	video, bus := newCLUT8TestRig(t)

	// Enable video
	bus.Write32(VIDEO_CTRL, 1)
	video.hasContent.Store(true)

	// Set up palette: index 0=black, 1=red, 2=green, 3=blue
	bus.Write32(VIDEO_PAL_INDEX, 0)
	bus.Write32(VIDEO_PAL_DATA, 0x00000000) // Black
	bus.Write32(VIDEO_PAL_DATA, 0x00FF0000) // Red
	bus.Write32(VIDEO_PAL_DATA, 0x0000FF00) // Green
	bus.Write32(VIDEO_PAL_DATA, 0x000000FF) // Blue

	// Set COLOR_MODE=1 (CLUT8) and FB_BASE
	bus.Write32(VIDEO_COLOR_MODE, 1)
	bus.Write32(VIDEO_FB_BASE, VRAM_START)

	// Verify readback
	gotMode := video.HandleRead(VIDEO_COLOR_MODE)
	if gotMode != 1 {
		t.Fatalf("COLOR_MODE readback: got %d, want 1", gotMode)
	}
	gotFB := video.HandleRead(VIDEO_FB_BASE)
	if gotFB != VRAM_START {
		t.Fatalf("FB_BASE readback: got 0x%X, want 0x%X", gotFB, VRAM_START)
	}

	// Write indexed pixels to bus memory at VRAM_START
	bus.memory[VRAM_START+0] = 0 // Black
	bus.memory[VRAM_START+1] = 1 // Red
	bus.memory[VRAM_START+2] = 2 // Green
	bus.memory[VRAM_START+3] = 3 // Blue

	// GetFrame() should return RGBA32 conversion
	frame := video.GetFrame()
	if frame == nil {
		t.Fatal("GetFrame() returned nil")
	}

	mode := VideoModes[video.currentMode]
	expectedSize := mode.width * mode.height * BYTES_PER_PIXEL
	if len(frame) != expectedSize {
		t.Fatalf("GetFrame() size: got %d, want %d", len(frame), expectedSize)
	}

	// Check first 4 pixels (each 4 bytes in RGBA order)
	// Pixel 0 (index 0 = 0x00000000): R=0,G=0,B=0,A=0xFF
	clut8CheckPixel(t, frame, 0, 0x00, 0x00, 0x00, 0xFF, "pixel 0 (black)")
	// Pixel 1 (index 1 = 0x00FF0000): R=0xFF,G=0,B=0,A=0xFF
	clut8CheckPixel(t, frame, 1, 0xFF, 0x00, 0x00, 0xFF, "pixel 1 (red)")
	// Pixel 2 (index 2 = 0x0000FF00): R=0,G=0xFF,B=0,A=0xFF
	clut8CheckPixel(t, frame, 2, 0x00, 0xFF, 0x00, 0xFF, "pixel 2 (green)")
	// Pixel 3 (index 3 = 0x000000FF): R=0,G=0,B=0xFF,A=0xFF
	clut8CheckPixel(t, frame, 3, 0x00, 0x00, 0xFF, 0xFF, "pixel 3 (blue)")
}

func TestVideoChip_CLUT8_FinishFrame(t *testing.T) {
	video, bus := newCLUT8TestRig(t)

	bus.Write32(VIDEO_CTRL, 1)
	video.hasContent.Store(true)

	// Set palette entry 5 = white
	bus.Write32(VIDEO_PAL_TABLE+5*4, 0x00FFFFFF)

	// Enable CLUT8
	bus.Write32(VIDEO_COLOR_MODE, 1)
	bus.Write32(VIDEO_FB_BASE, VRAM_START)

	// Write pixel index 5 at first position
	bus.memory[VRAM_START] = 5

	// FinishFrame (scanline-aware path) should also do CLUT conversion
	frame := video.FinishFrame()
	if frame == nil {
		t.Fatal("FinishFrame() returned nil")
	}

	mode := VideoModes[video.currentMode]
	expectedSize := mode.width * mode.height * BYTES_PER_PIXEL
	if len(frame) != expectedSize {
		t.Fatalf("FinishFrame() size: got %d, want %d", len(frame), expectedSize)
	}

	// Pixel 0 should be white (R=0xFF,G=0xFF,B=0xFF,A=0xFF)
	clut8CheckPixel(t, frame, 0, 0xFF, 0xFF, 0xFF, 0xFF, "pixel 0 (white via FinishFrame)")
}

func TestVideoChip_CLUT8_FBBase_Bounds(t *testing.T) {
	video, bus := newCLUT8TestRig(t)

	bus.Write32(VIDEO_CTRL, 1)
	video.hasContent.Store(true)

	// Set CLUT8 mode with an out-of-range fbBase
	bus.Write32(VIDEO_COLOR_MODE, 1)
	bus.Write32(VIDEO_FB_BASE, 0x01FFFFFF) // Way past end of bus memory

	// GetFrame should return a zeroed frame, not panic
	frame := video.GetFrame()
	if frame == nil {
		t.Fatal("GetFrame() returned nil for OOB fbBase")
	}

	// Frame should be all zeros (black)
	nonZero := 0
	for _, b := range frame {
		if b != 0 {
			nonZero++
		}
	}
	if nonZero > 0 {
		t.Fatalf("GetFrame() with OOB fbBase: expected all-zero frame, got %d non-zero bytes", nonZero)
	}

	// Test edge case: fbBase + pixelCount == len(busMemory) — should work
	mode := VideoModes[video.currentMode]
	pixelCount := uint32(mode.width * mode.height)
	validFB := uint32(len(bus.memory)) - pixelCount
	bus.Write32(VIDEO_FB_BASE, validFB)

	frame2 := video.GetFrame()
	if frame2 == nil {
		t.Fatal("GetFrame() returned nil for valid edge-case fbBase")
	}

	// Test: fbBase + pixelCount > len(busMemory) — should return zeroed frame
	bus.Write32(VIDEO_FB_BASE, validFB+1)
	// Reset the sync.Once so we can trigger the warning path again
	video.clutWarnOnce = sync.Once{}

	frame3 := video.GetFrame()
	if frame3 == nil {
		t.Fatal("GetFrame() returned nil for slightly-OOB fbBase")
	}
	_ = frame3
}

func TestVideoChip_CLUT8_ByteWrite(t *testing.T) {
	video, _ := newCLUT8TestRig(t)

	// Write a palette entry via 4 individual byte writes (simulating M68K big-endian)
	// Target: PAL_TABLE[7] = 0x00AABB42
	video.SetBigEndianMode(true)
	base := uint32(VIDEO_PAL_TABLE + 7*4)

	// Big-endian byte order: byte 0 = MSB
	video.HandleWrite8(base+0, 0x00) // bits 31:24
	video.HandleWrite8(base+1, 0xAA) // bits 23:16
	video.HandleWrite8(base+2, 0xBB) // bits 15:8
	video.HandleWrite8(base+3, 0x42) // bits 7:0

	got := video.HandleRead(VIDEO_PAL_TABLE + 7*4)
	if got != 0x00AABB42 {
		t.Fatalf("PAL_TABLE[7] after byte writes: got 0x%08X, want 0x00AABB42", got)
	}

	// Verify the pre-packed RGBA: R=0xAA, G=0xBB, B=0x42, A=0xFF
	expected := uint32(0xAA) | uint32(0xBB)<<8 | uint32(0x42)<<16 | 0xFF000000
	if video.clutPalette[7] != expected {
		t.Fatalf("clutPalette[7]: got 0x%08X, want 0x%08X", video.clutPalette[7], expected)
	}
}

func TestClipboardBridge_Headless(t *testing.T) {
	bus := NewMachineBus()
	cb := NewClipboardBridge(bus)

	// Read command should return CLIP_STATUS_EMPTY
	cb.HandleWrite(CLIP_DATA_PTR, 0x1000)
	cb.HandleWrite(CLIP_DATA_LEN, 256)
	cb.HandleWrite(CLIP_CTRL, CLIP_CMD_READ)

	status := cb.HandleRead(CLIP_STATUS)
	if status != CLIP_STATUS_EMPTY {
		t.Fatalf("Clipboard read status: got %d, want %d (EMPTY)", status, CLIP_STATUS_EMPTY)
	}

	resultLen := cb.HandleRead(CLIP_RESULT_LEN)
	if resultLen != 0 {
		t.Fatalf("Clipboard read result_len: got %d, want 0", resultLen)
	}

	// Write command should succeed silently
	cb.HandleWrite(CLIP_DATA_PTR, 0x2000)
	cb.HandleWrite(CLIP_DATA_LEN, 128)
	cb.HandleWrite(CLIP_CTRL, CLIP_CMD_WRITE)

	status = cb.HandleRead(CLIP_STATUS)
	if status != CLIP_STATUS_READY {
		t.Fatalf("Clipboard write status: got %d, want %d (READY)", status, CLIP_STATUS_READY)
	}

	resultLen = cb.HandleRead(CLIP_RESULT_LEN)
	if resultLen != 128 {
		t.Fatalf("Clipboard write result_len: got %d, want 128", resultLen)
	}
}

// clut8CheckPixel verifies a pixel in an RGBA32 frame buffer
func clut8CheckPixel(t *testing.T, frame []byte, pixelIdx int, r, g, b, a byte, desc string) {
	t.Helper()
	off := pixelIdx * BYTES_PER_PIXEL
	if off+4 > len(frame) {
		t.Fatalf("%s: pixel offset %d out of range (frame size %d)", desc, off, len(frame))
	}
	gotR, gotG, gotB, gotA := frame[off], frame[off+1], frame[off+2], frame[off+3]
	if gotR != r || gotG != g || gotB != b || gotA != a {
		t.Fatalf("%s: got RGBA [%02X,%02X,%02X,%02X], want [%02X,%02X,%02X,%02X]",
			desc, gotR, gotG, gotB, gotA, r, g, b, a)
	}
}
