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

func TestScreenEditor_E2E_TypeAndReturn(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "10 PRINT 42" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')
	if got := drainTermIn(term); got != "10 PRINT 42\n" {
		t.Fatalf("expected line submit, got %q", got)
	}
}

func TestScreenEditor_E2E_RawKeyForGet(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 0)
	vt.HandleKeyInput('A')
	if got := string(drainRawKeys(term)); got != "A" {
		t.Fatalf("expected raw key A, got %q", got)
	}
}

func TestScreenEditor_E2E_ReturnInCharVsLineMode(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)

	term.HandleWrite(TERM_CTRL, 0)
	vt.HandleKeyInput('\n')
	if got := string(drainRawKeys(term)); got != "\n" {
		t.Fatalf("expected raw newline in char mode, got %q", got)
	}
	if term.HandleRead(TERM_STATUS)&1 != 0 {
		t.Fatal("expected TERM_IN empty in char mode")
	}

	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "RUN" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')
	if got := drainTermIn(term); got != "RUN\n" {
		t.Fatalf("expected TERM_IN line in line mode, got %q", got)
	}
	if term.HandleRead(TERM_KEY_STATUS)&1 != 0 {
		t.Fatal("expected TERM_KEY empty in line mode")
	}
}

func TestScreenEditor_E2E_ConsoleModeRouting(t *testing.T) {
	term := NewTerminalMMIO()
	term.HandleWrite(TERM_CTRL, 1)
	term.RouteHostKey('L')
	if got := term.HandleRead(TERM_IN); got != 'L' {
		t.Fatalf("expected line mode route to TERM_IN, got 0x%X", got)
	}
	term.HandleWrite(TERM_CTRL, 0)
	term.RouteHostKey('K')
	if got := term.HandleRead(TERM_KEY_IN); got != 'K' {
		t.Fatalf("expected char mode route to TERM_KEY_IN, got 0x%X", got)
	}
}

func TestScreenEditor_E2E_EchoSuppressed(t *testing.T) {
	term := NewTerminalMMIO()
	term.HandleWrite(TERM_ECHO, 1)
	term.SetForceEchoOff(true)
	if got := term.HandleRead(TERM_ECHO); got != 0 {
		t.Fatalf("expected forced echo off, got %d", got)
	}
}

func TestScreenEditor_E2E_ModeTransition(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	vt.HandleKeyInput('A')
	vt.HandleKeyInput('\n')
	if got := drainTermIn(term); got != "A\n" {
		t.Fatalf("expected line mode submit A, got %q", got)
	}

	term.HandleWrite(TERM_CTRL, 0)
	vt.HandleKeyInput('B')
	if got := string(drainRawKeys(term)); got != "B" {
		t.Fatalf("expected char mode raw B, got %q", got)
	}

	term.HandleWrite(TERM_CTRL, 1)
	vt.HandleKeyInput('C')
	vt.HandleKeyInput('\n')
	if got := drainTermIn(term); got != "C\n" {
		t.Fatalf("expected second line mode submit C, got %q", got)
	}
}

func TestScreenEditor_E2E_EditAndReturn(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	// Type "10 PRIMT 42" (typo: M instead of N)
	for _, ch := range "10 PRIMT 42" {
		vt.HandleKeyInput(byte(ch))
	}
	// Navigate back to the 'M' and overwrite with 'N'
	feedInput(vt, 0x1B, '[', 'D') // left past '2'
	feedInput(vt, 0x1B, '[', 'D') // left past '4'
	feedInput(vt, 0x1B, '[', 'D') // left past ' '
	feedInput(vt, 0x1B, '[', 'D') // left past 'T'
	feedInput(vt, 0x1B, '[', 'D') // on 'M'
	vt.HandleKeyInput('N')        // overwrite 'M' with 'N'
	// Submit
	vt.HandleKeyInput('\n')
	got := drainTermIn(term)
	if got != "10 PRINT 42\n" {
		t.Fatalf("expected corrected line, got %q", got)
	}
}

func TestScreenEditor_E2E_OutputThenInput(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	// CPU output via processChar
	for _, ch := range "Ready.\r\n" {
		vt.processChar(byte(ch))
	}
	// Verify output rendered
	if got := vt.screen.ReadLine(0); got != "Ready." {
		t.Fatalf("expected output 'Ready.' on row 0, got %q", got)
	}
	// Now user types in line mode
	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "RUN" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')
	if got := drainTermIn(term); got != "RUN\n" {
		t.Fatalf("expected typed line after output, got %q", got)
	}
	// Verify screen shows both output and typed text
	if got := vt.screen.ReadLine(0); got != "Ready." {
		t.Fatalf("expected output preserved on row 0, got %q", got)
	}
	if got := vt.screen.ReadLine(1); got != "RUN" {
		t.Fatalf("expected typed text on row 1, got %q", got)
	}
}

func TestScreenEditor_E2E_ScrollbackNav(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	// Fill 35 rows of output (more than 30-row viewport)
	for i := 0; i < 35; i++ {
		line := byte('A' + byte(i%26))
		vt.processChar(line)
		vt.processChar('\r')
		vt.processChar('\n')
	}
	// Cursor should be past viewport's original position
	_, cy := vt.screen.CursorPos()
	if cy < 35 {
		t.Fatalf("expected cursor at or past row 35, got %d", cy)
	}
	// Navigate up past the viewport
	for i := 0; i < 10; i++ {
		vt.mu.Lock()
		vt.screen.MoveCursor(0, -1)
		vt.mu.Unlock()
	}
	// Verify viewport adjusted
	top := vt.screen.ViewportTop()
	_, curY := vt.screen.CursorPos()
	if curY < top || curY >= top+vt.rows {
		t.Fatalf("expected cursor within viewport after navigation, cursorY=%d viewportTop=%d rows=%d", curY, top, vt.rows)
	}
	// Verify content from scrollback is accessible
	firstLineContent := vt.screen.ReadLine(0)
	if firstLineContent != "A" {
		t.Fatalf("expected scrollback row 0 to be 'A', got %q", firstLineContent)
	}
}
