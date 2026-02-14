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
		for y := range terminalGlyphHeight {
			for x := range terminalGlyphWidth {
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
	if got := drainTermIn(term); got != "BC\n" {
		t.Fatalf("expected second line mode submit BC, got %q", got)
	}
}

func TestScreenEditor_E2E_EditAndReturn(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	// Type "10 PRIMT 42" (typo: M instead of N)
	for _, ch := range "10 PRIMT 42" {
		vt.HandleKeyInput(byte(ch))
	}
	// Navigate back to the 'M', delete it, type 'N'
	feedInput(vt, 0x1B, '[', 'D')      // left past '2'
	feedInput(vt, 0x1B, '[', 'D')      // left past '4'
	feedInput(vt, 0x1B, '[', 'D')      // left past ' '
	feedInput(vt, 0x1B, '[', 'D')      // left past 'T'
	feedInput(vt, 0x1B, '[', 'D')      // on 'M'
	feedInput(vt, 0x1B, '[', '3', '~') // delete 'M'
	vt.HandleKeyInput('N')             // insert 'N'
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

func TestScreenEditor_E2E_TypeaheadBeforeReadLine(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	// Default is line mode (simulates state before read_line runs).
	// Type keys before read_line sets TERM_CTRL — they should go to TERM_IN.
	for _, ch := range "10 PRINT 42" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')

	// Verify keys ended up in TERM_IN (where read_line reads), not TERM_KEY_IN.
	got := drainTermIn(term)
	if got != "10 PRINT 42\n" {
		t.Fatalf("expected typeahead in TERM_IN, got %q", got)
	}
	if term.HandleRead(TERM_KEY_STATUS)&1 != 0 {
		t.Fatal("expected TERM_KEY_IN empty for typeahead keys")
	}
}

func TestScreenEditor_E2E_ScrollbackNav(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	// Fill 35 rows of output (more than 30-row viewport)
	for i := range 35 {
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
	for range 10 {
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

func TestVideoTerminal_HandleScroll(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	// Fill 40 rows of output
	for i := range 40 {
		ch := byte('A' + byte(i%26))
		vt.processChar(ch)
		vt.processChar('\r')
		vt.processChar('\n')
	}
	topBefore := vt.screen.ViewportTop()
	vt.HandleScroll(-5)
	topAfter := vt.screen.ViewportTop()
	if topAfter != topBefore-5 {
		t.Fatalf("expected viewport scrolled up by 5, before=%d after=%d", topBefore, topAfter)
	}
	vt.HandleScroll(3)
	topAfter2 := vt.screen.ViewportTop()
	if topAfter2 != topAfter+3 {
		t.Fatalf("expected viewport scrolled down by 3, before=%d after=%d", topAfter, topAfter2)
	}
}

func TestHandleKeyInput_CtrlA(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	for _, ch := range "HELLO" {
		vt.processChar(byte(ch))
	}
	// Cursor is at col 5. Ctrl+A should go to col 0.
	vt.HandleKeyInput(0x01)
	cx, _ := vt.screen.CursorPos()
	if cx != 0 {
		t.Fatalf("expected Ctrl+A to go to col 0, got %d", cx)
	}
}

func TestHandleKeyInput_CtrlK_KillToEOL(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	// CPU output ">" then user types "HELLO WORLD"
	vt.processChar('>')
	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "HELLO WORLD" {
		vt.HandleKeyInput(byte(ch))
	}
	// Move back 5 chars to 'W', then Ctrl+K
	for range 5 {
		feedInput(vt, 0x1B, '[', 'D')
	}
	vt.HandleKeyInput(0x0B) // Ctrl+K
	// ReadLine trims trailing spaces, so ">HELLO " becomes ">HELLO"
	if got := vt.screen.ReadLine(0); got != ">HELLO" {
		t.Fatalf("expected '>HELLO' after Ctrl+K, got %q", got)
	}
	// Verify the cells after cursor are actually zeroed
	if got := vt.screen.GetCell(7, 0); got != 0 {
		t.Fatalf("expected cell 7 zeroed after Ctrl+K, got %q", got)
	}
}

func TestHandleKeyInput_CtrlU_KillToBOL(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	// CPU output ">" then user types "HELLO"
	vt.processChar('>')
	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "HELLO" {
		vt.HandleKeyInput(byte(ch))
	}
	// Ctrl+U should kill from cursor back to prompt
	vt.HandleKeyInput(0x15) // Ctrl+U
	if got := vt.screen.ReadLine(0); got != ">" {
		t.Fatalf("expected '>' after Ctrl+U, got %q", got)
	}
	cx, _ := vt.screen.CursorPos()
	if cx != 1 {
		t.Fatalf("expected cursor at col 1 (after prompt), got %d", cx)
	}
}

func TestHandleKeyInput_CtrlK_AnyRow(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	// Output on row 0
	for _, ch := range "OUTPUT" {
		vt.processChar(byte(ch))
	}
	vt.processChar('\r')
	vt.processChar('\n')
	// Move cursor back to row 0, col 3 (on 'P')
	vt.mu.Lock()
	vt.screen.MoveCursor(0, -1)
	vt.screen.cursorX = 3
	vt.mu.Unlock()
	// Ctrl+K clears from cursor to end of line
	vt.HandleKeyInput(0x0B)
	if got := vt.screen.ReadLine(0); got != "OUT" {
		t.Fatalf("expected 'OUT' after Ctrl+K from col 3, got %q", got)
	}
}

func TestVideoTerminal_HistoryRecall(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	// Type "A" and submit
	vt.HandleKeyInput('A')
	vt.HandleKeyInput('\n')
	drainTermIn(term)
	// Type "B" and submit
	vt.HandleKeyInput('B')
	vt.HandleKeyInput('\n')
	drainTermIn(term)
	// Now on row 2. Ctrl+Up should recall "B"
	feedInput(vt, 0x1B, '[', '1', ';', '5', 'A') // Ctrl+Up
	_, cy := vt.screen.CursorPos()
	if got := vt.screen.ReadLine(cy); got != "B" {
		t.Fatalf("expected history recall 'B', got %q", got)
	}
	// Ctrl+Up again should recall "A"
	feedInput(vt, 0x1B, '[', '1', ';', '5', 'A')
	if got := vt.screen.ReadLine(cy); got != "A" {
		t.Fatalf("expected history recall 'A', got %q", got)
	}
	// Ctrl+Down should go back to "B"
	feedInput(vt, 0x1B, '[', '1', ';', '5', 'B')
	if got := vt.screen.ReadLine(cy); got != "B" {
		t.Fatalf("expected history forward 'B', got %q", got)
	}
}

func TestVideoTerminal_HistoryOnNonInputRow(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	// Output some text
	for _, ch := range "OUTPUT" {
		vt.processChar(byte(ch))
	}
	vt.processChar('\r')
	vt.processChar('\n')
	// Move cursor back to output row
	vt.mu.Lock()
	vt.screen.MoveCursor(0, -1)
	vt.mu.Unlock()
	// Ctrl+Up should be a no-op
	feedInput(vt, 0x1B, '[', '1', ';', '5', 'A')
	if got := vt.screen.ReadLine(0); got != "OUTPUT" {
		t.Fatalf("expected OUTPUT unchanged, got %q", got)
	}
}

func TestVideoTerminal_HistoryPreservesPrompt(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	// CPU outputs ">"
	vt.processChar('>')
	term.HandleWrite(TERM_CTRL, 1)
	// User types "HELLO" and submits
	for _, ch := range "HELLO" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')
	drainTermIn(term)
	// CPU outputs ">" again on new line
	vt.processChar('>')
	// Ctrl+Up should recall "HELLO" after the ">"
	feedInput(vt, 0x1B, '[', '1', ';', '5', 'A')
	_, cy := vt.screen.CursorPos()
	if got := vt.screen.ReadLine(cy); got != ">HELLO" {
		t.Fatalf("expected '>HELLO' with prompt preserved, got %q", got)
	}
}

func TestVideoTerminal_ResetPreservesHistory(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	// Type "CMD1" and submit
	for _, ch := range "CMD1" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')
	drainTermIn(term)
	// Reset
	vt.Reset()
	// Output prompt to establish input region
	vt.processChar('>')
	// Ctrl+Up should still recall "CMD1"
	feedInput(vt, 0x1B, '[', '1', ';', '5', 'A')
	_, cy := vt.screen.CursorPos()
	if got := vt.screen.ReadLine(cy); got != ">CMD1" {
		t.Fatalf("expected history preserved after reset, got %q", got)
	}
}

func TestVideoTerminal_StopClearsHistory(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "CMD1" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')
	drainTermIn(term)
	vt.Stop()
	if vt.history != nil {
		t.Fatal("expected history cleared after Stop")
	}
}
