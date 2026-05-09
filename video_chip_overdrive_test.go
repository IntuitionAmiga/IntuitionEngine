package main

import "testing"

const (
	overdriveCLUTFront = 0x02000000
	overdriveRGBAFront = 0x03000000
	overdriveBusSize   = 0x04000000
)

func newOverdriveVideoTestRig(t *testing.T) (*VideoChip, *MachineBus) {
	t.Helper()
	bus, err := NewMachineBusSized(overdriveBusSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip: %v", err)
	}
	video.AttachBus(bus)
	video.SetBigEndianMode(true)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	return video, bus
}

func TestVideoMode1920x1080_Metadata(t *testing.T) {
	mode, ok := VideoModes[MODE_1920x1080]
	if !ok {
		t.Fatal("MODE_1920x1080 missing from VideoModes")
	}
	if mode.width != 1920 || mode.height != 1080 {
		t.Fatalf("dimensions got %dx%d, want 1920x1080", mode.width, mode.height)
	}
	if mode.bytesPerRow != 7680 {
		t.Fatalf("bytesPerRow got %d, want 7680", mode.bytesPerRow)
	}
	if mode.totalSize != 8294400 {
		t.Fatalf("totalSize got %d, want 8294400", mode.totalSize)
	}
	if DEFAULT_VIDEO_MODE != MODE_800x600 {
		t.Fatalf("DEFAULT_VIDEO_MODE changed: got 0x%X want MODE_800x600", DEFAULT_VIDEO_MODE)
	}
	if VRAM_START != 0x100000 || VRAM_SIZE != 0x500000 {
		t.Fatalf("legacy VRAM aperture changed: start=0x%X size=0x%X", VRAM_START, VRAM_SIZE)
	}
}

func TestVideoMode1920x1080_ModeWriteNotifiesAndAllocates(t *testing.T) {
	video, bus := newOverdriveVideoTestRig(t)
	var gotW, gotH int
	video.SetResolutionChangeCallback(func(w, h int) {
		gotW, gotH = w, h
	})

	bus.Write32(VIDEO_MODE, MODE_1920x1080)

	mode := VideoModes[MODE_1920x1080]
	if video.currentMode != MODE_1920x1080 {
		t.Fatalf("currentMode got 0x%X, want 0x%X", video.currentMode, MODE_1920x1080)
	}
	if gotW != 1920 || gotH != 1080 {
		t.Fatalf("callback got %dx%d, want 1920x1080", gotW, gotH)
	}
	if len(video.frontBuffer) != mode.totalSize || len(video.backBuffer) != mode.totalSize {
		t.Fatalf("buffer sizes got front=%d back=%d want %d", len(video.frontBuffer), len(video.backBuffer), mode.totalSize)
	}
}

func TestVideoMode1920x1080_NilCallbackUpdatesOutputConfig(t *testing.T) {
	out := newMockVideoOutput()
	video := newTestVideoChip(out)

	video.HandleWrite(VIDEO_MODE, MODE_1920x1080)

	cfg := out.GetDisplayConfig()
	if cfg.Width != 1920 || cfg.Height != 1080 {
		t.Fatalf("output config got %dx%d, want 1920x1080", cfg.Width, cfg.Height)
	}
}

func TestVideoMode1920x1080_WithCompositorEndToEnd(t *testing.T) {
	out := newMockVideoOutput()
	video := newTestVideoChip(out)
	comp := NewVideoCompositor(out)
	video.SetResolutionChangeCallback(func(w, h int) {
		comp.NotifyResolutionChange(w, h)
	})

	video.HandleWrite(VIDEO_MODE, MODE_1920x1080)
	comp.composite()

	if comp.frameWidth != 1920 || comp.frameHeight != 1080 {
		t.Fatalf("compositor dimensions got %dx%d, want 1920x1080", comp.frameWidth, comp.frameHeight)
	}
	if len(comp.finalFrame) != 1920*1080*BYTES_PER_PIXEL {
		t.Fatalf("compositor frame len got %d, want %d", len(comp.finalFrame), 1920*1080*BYTES_PER_PIXEL)
	}
}

func TestVideoMode1920x1080_CLUT8HighFBBase(t *testing.T) {
	video, bus := newOverdriveVideoTestRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	bus.Write32(VIDEO_MODE, MODE_1920x1080)
	bus.Write32(VIDEO_COLOR_MODE, 1)
	bus.Write32(VIDEO_FB_BASE, overdriveCLUTFront)
	bus.Write32(VIDEO_PAL_TABLE+7*4, 0x00112233)
	bus.Write32(VIDEO_PAL_TABLE+9*4, 0x00445566)

	mode := VideoModes[MODE_1920x1080]
	pixelCount := mode.width * mode.height
	bus.memory[overdriveCLUTFront] = 7
	bus.memory[overdriveCLUTFront+uint32(pixelCount-1)] = 9

	frame := video.FinishFrame()
	if len(frame) != mode.totalSize {
		t.Fatalf("frame size got %d, want %d", len(frame), mode.totalSize)
	}
	assertRGBA(t, frame, 0, 0x11, 0x22, 0x33, 0xFF)
	assertRGBA(t, frame, pixelCount-1, 0x44, 0x55, 0x66, 0xFF)
}

func TestVideoMode1920x1080_RGBA32HighFBBase(t *testing.T) {
	video, bus := newOverdriveVideoTestRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	bus.Write32(VIDEO_MODE, MODE_1920x1080)
	bus.Write32(VIDEO_COLOR_MODE, 0)
	bus.Write32(VIDEO_FB_BASE, overdriveRGBAFront)
	if got := bus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr != 0 {
		t.Fatalf("VIDEO_STATUS framebuffer error set immediately for high RGBA fbBase: 0x%X", got)
	}

	mode := VideoModes[MODE_1920x1080]
	first := []byte{0x10, 0x20, 0x30, 0x40}
	last := []byte{0x50, 0x60, 0x70, 0x80}
	lastOffset := mode.totalSize - BYTES_PER_PIXEL
	copy(bus.memory[overdriveRGBAFront:overdriveRGBAFront+4], first)
	copy(bus.memory[overdriveRGBAFront+uint32(lastOffset):overdriveRGBAFront+uint32(lastOffset)+4], last)

	frame := video.FinishFrame()
	if len(frame) != mode.totalSize {
		t.Fatalf("frame size got %d, want %d", len(frame), mode.totalSize)
	}
	assertBytesAt(t, frame, 0, first)
	assertBytesAt(t, frame, lastOffset, last)
	if got := bus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr != 0 {
		t.Fatalf("VIDEO_STATUS framebuffer error set for high RGBA fbBase: 0x%X", got)
	}
}

func TestVideoMode1920x1080_RGBA32RejectsImplicitLegacyFramebuffer(t *testing.T) {
	video, bus := newOverdriveVideoTestRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	bus.Write32(VIDEO_MODE, MODE_1920x1080)
	bus.Write32(VIDEO_COLOR_MODE, 0)
	if got := bus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr == 0 {
		t.Fatalf("VIDEO_STATUS missing immediate framebuffer error for implicit legacy 1080p RGBA path: 0x%X", got)
	}

	frame := video.FinishFrame()
	if frame != nil {
		t.Fatalf("FinishFrame returned %d bytes from implicit legacy framebuffer, want nil", len(frame))
	}
	if got := bus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr == 0 {
		t.Fatalf("VIDEO_STATUS missing framebuffer error for implicit legacy 1080p RGBA path: 0x%X", got)
	}
	if VRAM_SIZE != 0x500000 {
		t.Fatalf("legacy VRAM aperture changed: 0x%X", VRAM_SIZE)
	}
}

func TestVideoMode1920x1080_RGBA32RejectsLegacyVRAMFBBase(t *testing.T) {
	video, bus := newOverdriveVideoTestRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	bus.Write32(VIDEO_MODE, MODE_1920x1080)
	bus.Write32(VIDEO_COLOR_MODE, 0)
	bus.Write32(VIDEO_FB_BASE, VRAM_START)
	if got := bus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr == 0 {
		t.Fatalf("VIDEO_STATUS missing immediate framebuffer error for legacy VRAM 1080p RGBA path: 0x%X", got)
	}

	frame := video.FinishFrame()
	if frame != nil {
		t.Fatalf("FinishFrame returned %d bytes from legacy VRAM fbBase, want nil", len(frame))
	}
	if got := bus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr == 0 {
		t.Fatalf("VIDEO_STATUS missing framebuffer error for legacy VRAM 1080p RGBA path: 0x%X", got)
	}

	bus.Write32(VIDEO_FB_BASE, overdriveRGBAFront)
	if got := bus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr != 0 {
		t.Fatalf("VIDEO_STATUS framebuffer error did not clear immediately after high RGBA fbBase: 0x%X", got)
	}
	first := []byte{0x01, 0x02, 0x03, 0x04}
	copy(bus.memory[overdriveRGBAFront:overdriveRGBAFront+4], first)
	frame = video.FinishFrame()
	if frame == nil {
		t.Fatal("FinishFrame returned nil after switching to high RGBA fbBase")
	}
	assertBytesAt(t, frame, 0, first)
	if got := bus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr != 0 {
		t.Fatalf("VIDEO_STATUS framebuffer error did not clear after high RGBA fbBase: 0x%X", got)
	}
}

func TestVideoFramebufferStatusRejectsInvalidFBBaseForSmallerMode(t *testing.T) {
	video, bus := newOverdriveVideoTestRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	bus.Write32(VIDEO_MODE, MODE_640x480)
	bus.Write32(VIDEO_COLOR_MODE, 0)
	bus.Write32(VIDEO_FB_BASE, overdriveBusSize-4)
	if got := bus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr == 0 {
		t.Fatalf("VIDEO_STATUS missing immediate framebuffer error for out-of-range 640x480 fbBase: 0x%X", got)
	}
	if frame := video.FinishFrame(); frame != nil {
		t.Fatalf("FinishFrame returned %d bytes from out-of-range 640x480 fbBase, want nil", len(frame))
	}

	bus.Write32(VIDEO_FB_BASE, VRAM_START)
	if got := bus.Read32(VIDEO_STATUS); got&videoStatusFramebufferErr != 0 {
		t.Fatalf("VIDEO_STATUS framebuffer error did not clear for valid 640x480 legacy VRAM fbBase: 0x%X", got)
	}
	frame := video.FinishFrame()
	if len(frame) != VideoModes[MODE_640x480].totalSize {
		t.Fatalf("FinishFrame size got %d want %d", len(frame), VideoModes[MODE_640x480].totalSize)
	}
}
