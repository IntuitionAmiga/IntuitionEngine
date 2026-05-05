package main

import (
	"encoding/binary"
	"testing"
)

func TestMode320x200_RGBA32_Render(t *testing.T) {
	video, bus := newCLUT8TestRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	bus.Write32(VIDEO_MODE, MODE_320x200)
	bus.Write32(VIDEO_COLOR_MODE, 0)
	bus.Write32(VIDEO_FB_BASE, VRAM_START)

	mode := VideoModes[MODE_320x200]
	frameSize := mode.width * mode.height * BYTES_PER_PIXEL
	src := bus.memory[VRAM_START : VRAM_START+uint32(frameSize)]
	first := []byte{0x11, 0x22, 0x33, 0x44}
	midPixel := (mode.width*100 + 11) * BYTES_PER_PIXEL
	mid := []byte{0x55, 0x66, 0x77, 0x88}
	lastPixel := frameSize - BYTES_PER_PIXEL
	last := []byte{0x99, 0xAA, 0xBB, 0xCC}
	copy(src[0:4], first)
	copy(src[midPixel:midPixel+4], mid)
	copy(src[lastPixel:lastPixel+4], last)

	frame := video.FinishFrame()
	if len(frame) != frameSize {
		t.Fatalf("frame size: got %d, want %d", len(frame), frameSize)
	}
	assertBytesAt(t, frame, 0, first)
	assertBytesAt(t, frame, midPixel, mid)
	assertBytesAt(t, frame, lastPixel, last)
}

func TestMode320x200_CLUT8_Render(t *testing.T) {
	testLowResCLUT8(t, MODE_320x200)
}

func TestMode320x240_CLUT8_Render(t *testing.T) {
	testLowResCLUT8(t, MODE_320x240)
}

func TestVideoMode_Switching(t *testing.T) {
	video, bus := newCLUT8TestRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	for _, modeID := range []uint32{MODE_320x200, MODE_640x480, MODE_320x200} {
		bus.Write32(VIDEO_MODE, modeID)
		mode := VideoModes[modeID]
		if video.currentMode != modeID {
			t.Fatalf("currentMode: got %d, want %d", video.currentMode, modeID)
		}
		if len(video.frontBuffer) != mode.totalSize {
			t.Fatalf("frontBuffer size for mode %d: got %d, want %d", modeID, len(video.frontBuffer), mode.totalSize)
		}
	}
}

func testLowResCLUT8(t *testing.T, modeID uint32) {
	t.Helper()
	video, bus := newCLUT8TestRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	bus.Write32(VIDEO_MODE, modeID)
	bus.Write32(VIDEO_COLOR_MODE, 1)
	bus.Write32(VIDEO_FB_BASE, VRAM_START)

	bus.Write32(VIDEO_PAL_TABLE+1*4, 0x00112233)
	bus.Write32(VIDEO_PAL_TABLE+2*4, 0x00445566)
	bus.Write32(VIDEO_PAL_TABLE+3*4, 0x00778899)

	mode := VideoModes[modeID]
	pixelCount := mode.width * mode.height
	midPixel := mode.width*(mode.height/2) + 13
	bus.memory[VRAM_START] = 1
	bus.memory[VRAM_START+uint32(midPixel)] = 2
	bus.memory[VRAM_START+uint32(pixelCount-1)] = 3

	frame := video.FinishFrame()
	expectedSize := pixelCount * BYTES_PER_PIXEL
	if len(frame) != expectedSize {
		t.Fatalf("frame size: got %d, want %d", len(frame), expectedSize)
	}
	assertRGBA(t, frame, 0, 0x11, 0x22, 0x33, 0xFF)
	assertRGBA(t, frame, midPixel, 0x44, 0x55, 0x66, 0xFF)
	assertRGBA(t, frame, pixelCount-1, 0x77, 0x88, 0x99, 0xFF)
}

func assertRGBA(t *testing.T, frame []byte, pixel int, r, g, b, a byte) {
	t.Helper()
	off := pixel * BYTES_PER_PIXEL
	got := binary.LittleEndian.Uint32(frame[off : off+4])
	want := uint32(r) | uint32(g)<<8 | uint32(b)<<16 | uint32(a)<<24
	if got != want {
		t.Fatalf("pixel %d: got 0x%08X, want 0x%08X", pixel, got, want)
	}
}

func assertBytesAt(t *testing.T, frame []byte, offset int, want []byte) {
	t.Helper()
	for i, b := range want {
		if frame[offset+i] != b {
			t.Fatalf("byte at %d: got 0x%02X, want 0x%02X", offset+i, frame[offset+i], b)
		}
	}
}
