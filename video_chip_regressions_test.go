package main

import (
	"encoding/binary"
	"runtime"
	"testing"
	"time"
)

func newMappedVideoRig(t *testing.T) (*VideoChip, *MachineBus) {
	t.Helper()
	bus := NewMachineBus()
	video, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("failed to create video chip: %v", err)
	}
	video.AttachBus(bus)
	bus.MapIO(VIDEO_CTRL, VIDEO_REG_END, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VIDEO_CTRL, VIDEO_REG_END, video.HandleWrite8)
	bus.MapIO(VRAM_START, VRAM_START+VRAM_SIZE-1, video.HandleRead, video.HandleWrite)
	bus.MapIOByte(VRAM_START, VRAM_START+VRAM_SIZE-1, video.HandleWrite8)
	return video, bus
}

func TestPaletteWrite_M68K_ByteSequence(t *testing.T) {
	video, bus := newMappedVideoRig(t)
	video.SetBigEndianMode(true)

	addr := uint32(VIDEO_PAL_TABLE + 9*4)
	bus.Write8(addr+0, 0x00)
	bus.Write8(addr+1, 0x12)
	bus.Write8(addr+2, 0x34)
	bus.Write8(addr+3, 0x56)

	if got := video.HandleRead(addr); got != 0x00123456 {
		t.Fatalf("big-endian palette byte sequence = 0x%08X, want 0x00123456", got)
	}
}

func TestCLUT8_MappedVRAM_GetFrame(t *testing.T) {
	video, bus := newMappedVideoRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	video.hasContent.Store(true)
	bus.Write32(VIDEO_COLOR_MODE, 1)
	bus.Write32(VIDEO_FB_BASE, VRAM_START)
	bus.Write32(VIDEO_PAL_TABLE+2*4, 0x00AABBCC)
	bus.Write8(VRAM_START, 2)

	frame := video.GetFrame()
	if frame == nil {
		t.Fatal("GetFrame returned nil")
	}
	clut8CheckPixel(t, frame, 0, 0xAA, 0xBB, 0xCC, 0xFF, "mapped CLUT8 pixel")
}

func TestRasterBand_DirectVRAM_Visible(t *testing.T) {
	video, bus := newCLUT8TestRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	video.hasContent.Store(true)
	color := uint32(0x11223344)

	bus.Write32(VIDEO_RASTER_Y, 1)
	bus.Write32(VIDEO_RASTER_HEIGHT, 1)
	bus.Write32(VIDEO_RASTER_COLOR, color)
	bus.Write32(VIDEO_RASTER_CTRL, rasterCtrlStart)

	if got := binary.LittleEndian.Uint32(bus.memory[VRAM_START:]); got != 0 {
		t.Fatalf("row 0 unexpectedly changed: 0x%08X", got)
	}
	row1 := VRAM_START + uint32(VideoModes[video.currentMode].bytesPerRow)
	if got := binary.LittleEndian.Uint32(bus.memory[row1:]); got != color {
		t.Fatalf("direct VRAM raster pixel = 0x%08X, want 0x%08X", got, color)
	}
}

func TestRasterBand_CLUT8_Visible(t *testing.T) {
	video, bus := newMappedVideoRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	video.hasContent.Store(true)
	bus.Write32(VIDEO_COLOR_MODE, 1)
	bus.Write32(VIDEO_FB_BASE, VRAM_START)
	bus.Write32(VIDEO_PAL_TABLE+7*4, 0x00010203)

	bus.Write32(VIDEO_RASTER_Y, 0)
	bus.Write32(VIDEO_RASTER_HEIGHT, 1)
	bus.Write32(VIDEO_RASTER_COLOR, 7)
	bus.Write32(VIDEO_RASTER_CTRL, rasterCtrlStart)

	frame := video.GetFrame()
	clut8CheckPixel(t, frame, 0, 0x01, 0x02, 0x03, 0xFF, "CLUT8 raster pixel")
}

func TestBlitter_AlphaBlend_HalfOver(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]
	src := uint32(0x2000)
	dst := vramAddr(mode, 0, 0)
	binary.LittleEndian.PutUint32(bus.memory[src:src+4], 0x80808080)
	video.HandleWrite(dst, 0x000000FF)

	bus.Write32(BLT_OP, bltOpAlphaCopy)
	bus.Write32(BLT_SRC, src)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	if got := video.HandleRead(dst); got != 0x804040BF {
		t.Fatalf("alpha blend half-over = 0x%08X, want 0x804040BF", got)
	}
}

func TestBlitter_AlphaBlend_OpaqueTransparent(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]
	src := uint32(0x2000)
	dst := vramAddr(mode, 0, 0)

	for _, tc := range []struct {
		name string
		src  uint32
		dst  uint32
		want uint32
	}{
		{name: "opaque", src: 0xFF010203, dst: 0x100A0B0C, want: 0xFF010203},
		{name: "transparent", src: 0x00010203, dst: 0x100A0B0C, want: 0x100A0B0C},
	} {
		binary.LittleEndian.PutUint32(bus.memory[src:src+4], tc.src)
		video.HandleWrite(dst, tc.dst)
		bus.Write32(BLT_OP, bltOpAlphaCopy)
		bus.Write32(BLT_SRC, src)
		bus.Write32(BLT_DST, dst)
		bus.Write32(BLT_WIDTH, 1)
		bus.Write32(BLT_HEIGHT, 1)
		bus.Write32(BLT_CTRL, bltCtrlStart)
		if got := video.HandleRead(dst); got != tc.want {
			t.Fatalf("%s alpha blend = 0x%08X, want 0x%08X", tc.name, got, tc.want)
		}
	}
}

func TestBlitter_IRQ_PulseAckAndDisabled(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	sink := &recordingInterruptSink{}
	video.SetInterruptSink(sink)
	mode := VideoModes[video.currentMode]
	dst := vramAddr(mode, 0, 0)

	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_CTRL, bltCtrlStart|bltCtrlIRQEnable)
	if len(sink.pulses) != 1 || sink.pulses[0] != IntMaskBlitter {
		t.Fatalf("IRQ pulses = %v, want one blitter pulse", sink.pulses)
	}
	if got := video.HandleRead(BLT_STATUS); got&(bltStatusDone|bltStatusIRQPending) != bltStatusDone|bltStatusIRQPending {
		t.Fatalf("BLT_STATUS after IRQ blit = 0x%X", got)
	}

	bus.Write32(BLT_STATUS, bltStatusIRQPending)
	if got := video.HandleRead(BLT_STATUS); got&bltStatusIRQPending != 0 {
		t.Fatalf("IRQ pending survived ack: 0x%X", got)
	}

	bus.Write32(BLT_CTRL, bltCtrlStart)
	if len(sink.pulses) != 1 {
		t.Fatalf("disabled IRQ produced extra pulse: %v", sink.pulses)
	}
	if got := video.HandleRead(BLT_STATUS); got&bltStatusIRQPending != 0 || got&bltStatusDone == 0 {
		t.Fatalf("disabled IRQ status = 0x%X", got)
	}
}

func TestWireVideoInterruptSinks_WiresVideoAndANTIC(t *testing.T) {
	video, _ := newBlitterTestRig(t)
	antic := NewANTICEngine(nil)
	sink := &recordingInterruptSink{}

	wireVideoInterruptSinks(video, antic, sink)

	if video.intSink != sink {
		t.Fatal("video interrupt sink was not wired")
	}
	if antic.sink != sink {
		t.Fatal("ANTIC interrupt sink was not wired")
	}
}

func TestVideoChip_ResetClearsBlitterStickyStatus(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	sink := &recordingInterruptSink{}
	video.SetInterruptSink(sink)
	mode := VideoModes[video.currentMode]

	bus.Write32(BLT_OP, bltOpFill)
	bus.Write32(BLT_DST, vramAddr(mode, 0, 0))
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_CTRL, bltCtrlStart|bltCtrlIRQEnable)
	if got := video.HandleRead(BLT_STATUS); got&(bltStatusDone|bltStatusIRQPending) == 0 {
		t.Fatalf("test setup did not produce sticky status: 0x%X", got)
	}

	video.Reset()
	if got := video.HandleRead(BLT_STATUS); got != 0 {
		t.Fatalf("BLT_STATUS after Reset = 0x%X, want 0", got)
	}
}

func TestBlitter_InvalidOpcodeSetsError(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]
	dst := vramAddr(mode, 0, 0)
	video.HandleWrite(dst, 0x01020304)

	bus.Write32(BLT_OP, 99)
	bus.Write32(BLT_DST, dst)
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_COLOR, 0xDEADBEEF)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	if got := video.HandleRead(BLT_STATUS); got&bltStatusErr == 0 {
		t.Fatalf("invalid blitter op did not set error: 0x%X", got)
	}
	if got := video.HandleRead(dst); got != 0x01020304 {
		t.Fatalf("invalid blitter op mutated dst: 0x%08X", got)
	}
}

func TestMode7_StrideOverflowSetsError(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	mode := VideoModes[video.currentMode]

	bus.Write32(BLT_OP, bltOpMode7)
	bus.Write32(BLT_SRC, 0x1000)
	bus.Write32(BLT_DST, vramAddr(mode, 0, 0))
	bus.Write32(BLT_WIDTH, 1)
	bus.Write32(BLT_HEIGHT, 1)
	bus.Write32(BLT_SRC_STRIDE, 0xFFFFFFFF)
	bus.Write32(BLT_MODE7_U0, 0)
	bus.Write32(BLT_MODE7_V0, 1<<16)
	bus.Write32(BLT_MODE7_DU_COL, 0)
	bus.Write32(BLT_MODE7_DV_COL, 0)
	bus.Write32(BLT_MODE7_TEX_W, 0xFFFF)
	bus.Write32(BLT_MODE7_TEX_H, 0xFFFF)
	bus.Write32(BLT_CTRL, bltCtrlStart)

	if got := video.HandleRead(BLT_STATUS); got&bltStatusErr == 0 {
		t.Fatalf("Mode7 stride overflow did not set error: 0x%X", got)
	}
}

func TestGetFrame_PaletteRace(t *testing.T) {
	video, bus := newMappedVideoRig(t)
	bus.Write32(VIDEO_CTRL, 1)
	video.hasContent.Store(true)
	bus.Write32(VIDEO_COLOR_MODE, 1)
	bus.Write32(VIDEO_FB_BASE, VRAM_START)
	bus.Write8(VRAM_START, 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 1000 {
			bus.Write32(VIDEO_PAL_TABLE+4, uint32(i)&0x00FFFFFF)
		}
	}()
	for range 1000 {
		_ = video.GetFrame()
	}
	<-done
}

func TestRasterCtrlReadback(t *testing.T) {
	video, bus := newBlitterTestRig(t)
	bus.Write32(VIDEO_RASTER_CTRL, rasterCtrlStart)
	if got := video.HandleRead(VIDEO_RASTER_CTRL); got != rasterCtrlStart {
		t.Fatalf("VIDEO_RASTER_CTRL readback = 0x%X, want 0x%X", got, rasterCtrlStart)
	}
}

func TestHandleWrite8_VRAMOffsetUsesBufferOffset(t *testing.T) {
	video, bus := newMappedVideoRig(t)
	mode := VideoModes[video.currentMode]
	addr := vramAddr(mode, 0, 0)

	bus.Write8(addr, 0xAB)
	if got := video.frontBuffer[0]; got != 0xAB {
		t.Fatalf("frontBuffer[0] after byte write = 0x%02X, want 0xAB", got)
	}
}

func TestVBlankStatus_AfterSignalVSync_LevelAndFrameClear(t *testing.T) {
	video, _ := newBlitterTestRig(t)
	video.SignalVSync()
	for range 3 {
		if got := video.HandleRead(VIDEO_STATUS); got&2 == 0 {
			t.Fatalf("VIDEO_STATUS lost signaled VBlank: 0x%X", got)
		}
	}
	video.GetFrame()
	if got := video.HandleRead(VIDEO_STATUS); got&2 != 0 {
		t.Fatalf("VIDEO_STATUS VBlank not cleared by GetFrame: 0x%X", got)
	}
}

func TestVBlankStatus_TimerFallback(t *testing.T) {
	video, _ := newBlitterTestRig(t)
	video.lastFrameStart.Store(time.Now().Add(-REFRESH_INTERVAL).UnixNano())
	if got := video.HandleRead(VIDEO_STATUS); got&2 == 0 {
		t.Fatalf("timer fallback did not report VBlank: 0x%X", got)
	}
}

func TestVideoChip_DoubleStop_NoPanic(t *testing.T) {
	video, _ := newBlitterTestRig(t)
	if err := video.Stop(); err != nil {
		t.Fatalf("first Stop failed: %v", err)
	}
	if err := video.Stop(); err != nil {
		t.Fatalf("second Stop failed: %v", err)
	}
}

func TestVideoChip_StartAfterStop_Inert(t *testing.T) {
	video, _ := newBlitterTestRig(t)
	if err := video.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if err := video.Start(); err != nil {
		t.Fatalf("Start after Stop failed: %v", err)
	}
	if video.enabled.Load() {
		t.Fatal("Start after Stop re-enabled stopped chip")
	}
}

func TestVideoChip_StopExitsRefreshLoop(t *testing.T) {
	before := runtime.NumGoroutine()
	video, _ := newBlitterTestRig(t)
	if err := video.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		runtime.Gosched()
		if runtime.NumGoroutine() <= before {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("refreshLoop appears to still be running; goroutines before=%d after=%d", before, runtime.NumGoroutine())
}

func TestGetFrame_ReturnsImmutableSnapshot(t *testing.T) {
	video, _ := newBlitterTestRig(t)
	video.enabled.Store(true)
	video.hasContent.Store(true)
	video.frontBuffer[0] = 1
	frame := video.GetFrame()
	video.frontBuffer[0] = 2
	if frame[0] != 1 {
		t.Fatalf("GetFrame returned mutable backing slice, frame[0]=%d", frame[0])
	}
}
