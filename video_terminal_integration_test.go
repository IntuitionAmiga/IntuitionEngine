package main

import (
	"testing"
	"time"
)

func TestVideoTerminal_CursorHidesDuringGraphics(t *testing.T) {
	vt, chip, term := newVideoTerminalForTest(t)

	// Simulate graphics activity under cursor cell.
	graphicsColor := uint32(0xFF112233)
	chip.RenderToFrontBuffer(func(fb []byte, stride int) {
		for y := 0; y < terminalGlyphHeight; y++ {
			for x := 0; x < terminalGlyphWidth; x++ {
				idx := y*stride + x*4
				writeColorLE(fb, idx, graphicsColor)
			}
		}
	})
	chip.MarkRectDirty(0, 0, terminalGlyphWidth, terminalGlyphHeight)

	// Draw cursor, then make TERM_STATUS stale and tick once.
	vt.mu.Lock()
	vt.cursorOn = true
	vt.renderCursorCellLocked(true)
	vt.mu.Unlock()
	term.lastStatusRead.Store(time.Now().Add(-time.Second).UnixNano())
	vt.cursorTick()

	stride := VideoModes[chip.currentMode].bytesPerRow
	got := pixelAt(chip.GetFrontBuffer(), stride, 0, 0)
	// Cursor hiding should restore the underlying cell content.
	if got != vt.bgColor {
		t.Fatalf("expected cursor hidden cell to restore underlying content path (bg), got 0x%08X", got)
	}
}

func TestVideoTerminal_MixedGraphicsAndPrint(t *testing.T) {
	vt, chip, _ := newVideoTerminalForTest(t)

	graphicsColor := uint32(0xFF335577)
	chip.RenderToFrontBuffer(func(fb []byte, _ int) {
		for i := 0; i < len(fb); i += 4 {
			writeColorLE(fb, i, graphicsColor)
		}
	})
	mode := VideoModes[chip.currentMode]
	chip.MarkRectDirty(0, 0, mode.width, mode.height)

	vt.processChar('R')
	vt.processChar('e')
	vt.processChar('a')
	vt.processChar('d')
	vt.processChar('y')

	stride := mode.bytesPerRow
	// First cell should no longer be raw graphics color.
	if got := pixelAt(chip.GetFrontBuffer(), stride, 0, 0); got == graphicsColor {
		t.Fatalf("expected printed glyph to modify top-left cell over graphics, still got 0x%08X", got)
	}
	// A far pixel should remain graphics-colored.
	if got := pixelAt(chip.GetFrontBuffer(), stride, mode.width-1, mode.height-1); got != graphicsColor {
		t.Fatalf("expected far graphics pixel unchanged, got 0x%08X want 0x%08X", got, graphicsColor)
	}
}
