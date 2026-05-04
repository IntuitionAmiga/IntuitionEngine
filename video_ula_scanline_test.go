package main

import (
	"bytes"
	"testing"
	"time"
)

func populateULATestPattern(u *ULAEngine) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.border = 2
	for y := range ULA_DISPLAY_HEIGHT {
		row := u.rowStartAddr[y]
		for x := range ULA_CELLS_X {
			u.vram[row+uint16(x)] = uint8(0x80 >> (x % 8))
		}
	}
	for i := range ULA_CELLS_X * ULA_CELLS_Y {
		u.vram[ULA_ATTR_OFFSET+uint16(i)] = uint8((i % 8) | (((i / 8) % 8) << 3) | 0x40)
	}
}

func cloneULAForRender(src *ULAEngine) *ULAEngine {
	dst := NewULAEngine(nil)
	src.mu.Lock()
	dst.mu.Lock()
	dst.vram = src.vram
	dst.border = src.border
	dst.control = src.control
	dst.mu.Unlock()
	src.mu.Unlock()
	dst.flashState.Store(src.flashState.Load())
	dst.flashCounter.Store(src.flashCounter.Load())
	dst.enabled.Store(src.enabled.Load())
	return dst
}

func TestULA_ScanlineAware_FrameMatchesRenderFrame(t *testing.T) {
	ula := NewULAEngine(nil)
	populateULATestPattern(ula)
	clone := cloneULAForRender(ula)

	want := clone.RenderFrame()
	ula.StartFrame()
	for y := range ULA_FRAME_HEIGHT {
		ula.ProcessScanline(y)
	}
	got := ula.FinishFrame()

	if !bytes.Equal(got, want) {
		t.Fatal("scanline-rendered ULA frame differs from RenderFrame")
	}
}

func TestULA_ScanlineAware_ClampsOutOfRangeY(t *testing.T) {
	ula := NewULAEngine(nil)
	populateULATestPattern(ula)
	ula.StartFrame()
	ula.ProcessScanline(ULA_FRAME_HEIGHT + 10)
	_ = ula.FinishFrame()
}

func TestULA_ScanlineAware_FlashStateSourcedFromTickFrame(t *testing.T) {
	ula := NewULAEngine(nil)
	ula.mu.Lock()
	ula.vram[0] = 0x80
	ula.vram[ULA_ATTR_OFFSET] = 0x82 // ink red, paper black, flash
	ula.mu.Unlock()
	ula.flashCounter.Store(ULA_FLASH_FRAMES - 1)
	ula.TickFrame()

	ula.StartFrame()
	ula.ProcessScanline(ULA_BORDER_TOP)
	_ = ula.FinishFrame()

	if got := ula.flashCounter.Load(); got != 0 {
		t.Fatalf("flash counter = %d, want 0 after exactly one TickFrame advance", got)
	}
	if !ula.flashState.Load() {
		t.Fatal("flash state did not advance on TickFrame")
	}
}

func TestULA_ScanlineAware_DoesNotHoldULALockDuringRender(t *testing.T) {
	ula := NewULAEngine(nil)
	populateULATestPattern(ula)
	ula.StartFrame()

	done := make(chan struct{})
	go func() {
		ula.ProcessScanline(ULA_BORDER_TOP)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("ProcessScanline appears blocked on ULA mutex")
	}
	_ = ula.FinishFrame()
}
