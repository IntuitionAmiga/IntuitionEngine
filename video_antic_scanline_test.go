package main

import (
	"bytes"
	"testing"
)

func TestANTIC_ScanlineAware_FrameMatchesRenderFrame(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.HandleWrite(ANTIC_ENABLE, ANTIC_ENABLE_VIDEO)
	antic.HandleWrite(GTIA_COLBK, 0x24)
	antic.HandleWrite(GTIA_COLPF1, 0x46)

	want := antic.RenderFrame(nil)
	antic.StartFrame()
	for y := range ANTIC_FRAME_HEIGHT {
		antic.ProcessScanline(y)
	}
	got := antic.FinishFrame()

	if !bytes.Equal(got, want) {
		t.Fatal("scanline-aware ANTIC frame differs from RenderFrame")
	}
}

func TestANTIC_ScanlineAware_DLICursorAdvances(t *testing.T) {
	bus := NewMachineBus()
	const dlist = 0x2000
	const screen = 0x3000
	bus.Write8(dlist, DL_MODE8|DL_LMS)
	bus.Write8(dlist+1, byte(screen&0xFF))
	bus.Write8(dlist+2, byte((screen>>8)&0xFF))
	bus.Write8(dlist+3, DL_DLI|DL_MODE8)
	bus.Write8(dlist+4, DL_JVB)
	bus.Write8(dlist+5, byte(dlist&0xFF))
	bus.Write8(dlist+6, byte((dlist>>8)&0xFF))
	for i := 0; i < 80; i++ {
		bus.Write8(screen+uint32(i), 0xFF)
	}

	antic := NewANTICEngine(bus)
	antic.HandleWrite(ANTIC_DMACTL, ANTIC_DMA_DL|ANTIC_DMA_NORMAL)
	antic.HandleWrite(ANTIC_DLISTL, dlist&0xFF)
	antic.HandleWrite(ANTIC_DLISTH, dlist>>8)
	antic.HandleWrite(ANTIC_NMIEN, ANTIC_NMIEN_DLI)
	antic.StartFrame()
	for y := ANTIC_BORDER_TOP; y < ANTIC_BORDER_TOP+8; y++ {
		antic.ProcessScanline(y)
	}
	if antic.nmist&ANTIC_NMIST_DLI != 0 {
		t.Fatal("DLI fired before DLI entry completed")
	}
	for y := ANTIC_BORDER_TOP + 8; y < ANTIC_BORDER_TOP+16; y++ {
		antic.ProcessScanline(y)
	}
	if antic.nmist&ANTIC_NMIST_DLI == 0 {
		t.Fatal("DLI did not fire when DLI entry completed")
	}
}

func TestANTIC_ScanlineAware_RendersOnlyProcessedRows(t *testing.T) {
	bus := NewMachineBus()
	const dlist = 0x2200
	const screen = 0x3200
	bus.Write8(dlist, DL_MODE8|DL_LMS)
	bus.Write8(dlist+1, byte(screen&0xFF))
	bus.Write8(dlist+2, byte((screen>>8)&0xFF))
	bus.Write8(dlist+3, DL_JVB)
	bus.Write8(dlist+4, byte(dlist&0xFF))
	bus.Write8(dlist+5, byte((dlist>>8)&0xFF))
	for i := 0; i < 40; i++ {
		bus.Write8(screen+uint32(i), 0xFF)
	}

	antic := NewANTICEngine(bus)
	antic.HandleWrite(ANTIC_DMACTL, ANTIC_DMA_DL|ANTIC_DMA_NORMAL)
	antic.HandleWrite(ANTIC_DLISTL, dlist&0xFF)
	antic.HandleWrite(ANTIC_DLISTH, dlist>>8)
	antic.HandleWrite(GTIA_COLBK, 0x00)
	antic.HandleWrite(GTIA_COLPF1, 0x0F)
	antic.StartFrame()

	before := anticTestPixel(antic.frameBufs[antic.scanlineWriteIdx], ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP)
	antic.ProcessScanline(ANTIC_BORDER_TOP)
	afterFirst := anticTestPixel(antic.frameBufs[antic.scanlineWriteIdx], ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP)
	nextRow := anticTestPixel(antic.frameBufs[antic.scanlineWriteIdx], ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP+1)

	if before == afterFirst {
		t.Fatal("processed display-list row did not render")
	}
	if nextRow == afterFirst {
		t.Fatal("unprocessed next row was rendered early")
	}
}

func TestANTIC_ScanlineAware_ClampsOutOfRangeY(t *testing.T) {
	antic := NewANTICEngine(nil)
	antic.StartFrame()
	antic.ProcessScanline(ANTIC_FRAME_HEIGHT + 10)
	_ = antic.FinishFrame()
}
