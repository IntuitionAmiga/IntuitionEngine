package main

import (
	"testing"
	"time"
)

func newVideoTerminalForTest(t *testing.T) (*VideoTerminal, *VideoChip, *TerminalMMIO) {
	t.Helper()
	chip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip failed: %v", err)
	}
	term := NewTerminalMMIO()
	vt := NewVideoTerminal(chip, term)
	return vt, chip, term
}

func pixelAt(fb []byte, stride, x, y int) uint32 {
	idx := y*stride + x*4
	return uint32(fb[idx]) | uint32(fb[idx+1])<<8 | uint32(fb[idx+2])<<16 | uint32(fb[idx+3])<<24
}

func TestFontLoad_Size(t *testing.T) {
	if len(topazRawFont) != 4096 {
		t.Fatalf("expected 4096-byte raw font, got %d", len(topazRawFont))
	}
}

func TestFontLoad_GlyphA(t *testing.T) {
	glyphs := loadTopazFont()
	var nonZero int
	for _, b := range glyphs['A'] {
		if b != 0 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Fatal("expected glyph 'A' to contain lit pixels")
	}
}

func TestFontLoad_GlyphSpace(t *testing.T) {
	glyphs := loadTopazFont()
	for i, b := range glyphs[' '] {
		if b != 0 {
			t.Fatalf("expected space row %d to be zero, got 0x%02X", i, b)
		}
	}
}

func TestRenderGlyph_PixelPattern(t *testing.T) {
	vt, chip, _ := newVideoTerminalForTest(t)
	vt.processChar('A')
	fb := chip.GetFrontBuffer()
	stride := VideoModes[chip.currentMode].bytesPerRow
	glyph := vt.glyphs['A']
	for y := 0; y < terminalGlyphHeight; y++ {
		rowBits := glyph[y]
		for x := 0; x < terminalGlyphWidth; x++ {
			want := vt.bgColor
			if (rowBits & (0x80 >> x)) != 0 {
				want = vt.fgColor
			}
			got := pixelAt(fb, stride, x, y)
			if got != want {
				t.Fatalf("pixel mismatch at (%d,%d): got 0x%08X want 0x%08X", x, y, got, want)
			}
		}
	}
}

func TestRenderGlyph_Position(t *testing.T) {
	vt, chip, _ := newVideoTerminalForTest(t)
	vt.mu.Lock()
	vt.renderCellLocked(1, 1, 'A')
	vt.mu.Unlock()

	fb := chip.GetFrontBuffer()
	stride := VideoModes[chip.currentMode].bytesPerRow
	glyph := vt.glyphs['A']
	for y := 0; y < terminalGlyphHeight; y++ {
		rowBits := glyph[y]
		for x := 0; x < terminalGlyphWidth; x++ {
			want := vt.bgColor
			if (rowBits & (0x80 >> x)) != 0 {
				want = vt.fgColor
			}
			got := pixelAt(fb, stride, 8+x, 16+y)
			if got != want {
				t.Fatalf("pixel mismatch at positioned glyph (%d,%d): got 0x%08X want 0x%08X", x, y, got, want)
			}
		}
	}
}

func TestRenderGlyph_DynamicGrid(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	if vt.cols != 80 || vt.rows != 30 {
		t.Fatalf("expected 80x30 grid for 640x480, got %dx%d", vt.cols, vt.rows)
	}
}

func TestProcessChar_Printable(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('H')
	if vt.textBuf[0] != 'H' {
		t.Fatalf("expected first cell 'H', got %q", vt.textBuf[0])
	}
	if vt.cursorX != 1 || vt.cursorY != 0 {
		t.Fatalf("expected cursor (1,0), got (%d,%d)", vt.cursorX, vt.cursorY)
	}
}

func TestProcessChar_Sequence(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('H')
	vt.processChar('i')
	if vt.textBuf[0] != 'H' || vt.textBuf[1] != 'i' {
		t.Fatalf("expected 'Hi' in first two cells, got %q%q", vt.textBuf[0], vt.textBuf[1])
	}
	if vt.cursorX != 2 || vt.cursorY != 0 {
		t.Fatalf("expected cursor (2,0), got (%d,%d)", vt.cursorX, vt.cursorY)
	}
}

func TestProcessChar_CRLF(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('A')
	vt.processChar('B')
	vt.processChar('\r')
	if vt.cursorX != 0 || vt.cursorY != 0 {
		t.Fatalf("after CR expected cursor (0,0), got (%d,%d)", vt.cursorX, vt.cursorY)
	}
	vt.processChar('\n')
	if vt.cursorX != 0 || vt.cursorY != 1 {
		t.Fatalf("after CRLF expected cursor (0,1), got (%d,%d)", vt.cursorX, vt.cursorY)
	}
}

func TestProcessChar_CR(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('A')
	vt.processChar('B')
	vt.processChar('\r')
	if vt.cursorX != 0 || vt.cursorY != 0 {
		t.Fatalf("after CR expected cursor (0,0), got (%d,%d)", vt.cursorX, vt.cursorY)
	}
}

func TestProcessChar_LF(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('\n')
	if vt.cursorY != 1 {
		t.Fatalf("after LF expected cursorY 1, got %d", vt.cursorY)
	}
}

func TestProcessChar_BS(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('A')
	vt.processChar('B')
	vt.processChar('\b')
	if vt.cursorX != 1 || vt.cursorY != 0 {
		t.Fatalf("after BS expected cursor (1,0), got (%d,%d)", vt.cursorX, vt.cursorY)
	}
	if vt.textBuf[1] != 0 {
		t.Fatalf("expected cleared cell at index 1, got %q", vt.textBuf[1])
	}
}

func TestProcessChar_BS_AtCol0(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('\b')
	if vt.cursorX != 0 || vt.cursorY != 0 {
		t.Fatalf("expected BS at col 0 to no-op, got (%d,%d)", vt.cursorX, vt.cursorY)
	}
}

func TestProcessChar_TAB(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('\t')
	if vt.cursorX != 8 {
		t.Fatalf("expected cursorX 8 after TAB from col 0, got %d", vt.cursorX)
	}
	vt.cursorX = 8
	vt.processChar('\t')
	if vt.cursorX != 16 {
		t.Fatalf("expected cursorX 16 after TAB from col 8, got %d", vt.cursorX)
	}
}

func TestProcessChar_TAB_Aligned(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.cursorX = 8
	vt.processChar('\t')
	if vt.cursorX != 16 {
		t.Fatalf("expected aligned TAB to advance to 16, got %d", vt.cursorX)
	}
}

func TestProcessChar_LineWrap(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	for i := 0; i < vt.cols; i++ {
		vt.processChar('X')
	}
	if vt.cursorX != 0 || vt.cursorY != 1 {
		t.Fatalf("expected wrap to (0,1), got (%d,%d)", vt.cursorX, vt.cursorY)
	}
}

func TestProcessChar_LineWrap_AtBottom(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.cursorY = vt.rows - 1
	vt.cursorX = vt.cols - 1
	vt.processChar('X')
	if vt.cursorY != vt.rows-1 || vt.cursorX != 0 {
		t.Fatalf("expected wrap at bottom to keep cursor on last row col0, got (%d,%d)", vt.cursorX, vt.cursorY)
	}
}

func TestScrollUp_TextBuf(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.mu.Lock()
	vt.setCellLocked(0, 0, 'A')
	vt.setCellLocked(0, 1, 'B')
	vt.scrollUpLocked()
	vt.mu.Unlock()

	if vt.textBuf[0] != 'B' {
		t.Fatalf("expected old row 1 at top after scroll, got %q", vt.textBuf[0])
	}
	last := (vt.rows - 1) * vt.cols
	if vt.textBuf[last] != 0 {
		t.Fatalf("expected cleared last row, got %q", vt.textBuf[last])
	}
}

func TestScrollUp_Pixels(t *testing.T) {
	vt, chip, _ := newVideoTerminalForTest(t)
	stride := VideoModes[chip.currentMode].bytesPerRow
	vt.mu.Lock()
	vt.renderCellLocked(0, 0, 'A')
	vt.renderCellLocked(0, 1, 'B')
	before := pixelAt(chip.GetFrontBuffer(), stride, 0, terminalGlyphHeight)
	vt.scrollUpLocked()
	vt.mu.Unlock()
	after := pixelAt(chip.GetFrontBuffer(), stride, 0, 0)
	if after != before {
		t.Fatalf("expected row 1 pixel to move to row 0 after scroll, got 0x%08X want 0x%08X", after, before)
	}
}

func TestScrollUp_BottomCleared(t *testing.T) {
	vt, chip, _ := newVideoTerminalForTest(t)
	vt.mu.Lock()
	vt.renderCellLocked(0, vt.rows-1, 'A')
	vt.scrollUpLocked()
	vt.mu.Unlock()

	stride := VideoModes[chip.currentMode].bytesPerRow
	y := vt.pixelHeight - 1
	if got := pixelAt(chip.GetFrontBuffer(), stride, 0, y); got != vt.bgColor {
		t.Fatalf("expected bottom row cleared to bg color, got 0x%08X", got)
	}
}

func TestClearScreen(t *testing.T) {
	vt, chip, _ := newVideoTerminalForTest(t)
	vt.processChar('Z')
	vt.clearScreen()

	for i, b := range vt.textBuf {
		if b != 0 {
			t.Fatalf("expected text buffer cleared at %d", i)
		}
	}
	if vt.cursorX != 0 || vt.cursorY != 0 {
		t.Fatalf("expected cursor reset to (0,0), got (%d,%d)", vt.cursorX, vt.cursorY)
	}

	fb := chip.GetFrontBuffer()
	for i := 0; i < len(fb); i += 4 {
		got := uint32(fb[i]) | uint32(fb[i+1])<<8 | uint32(fb[i+2])<<16 | uint32(fb[i+3])<<24
		if got != vt.bgColor {
			t.Fatalf("expected cleared bg color at pixel byte %d, got 0x%08X", i, got)
		}
	}
}

func TestCursorReset_OnOutput(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.cursorOn = false
	vt.processChar('A')
	if !vt.cursorOn {
		t.Fatal("expected cursorOn reset to true after output")
	}
}

func TestCursorRender_Visible(t *testing.T) {
	vt, chip, _ := newVideoTerminalForTest(t)
	vt.mu.Lock()
	vt.renderCursorCellLocked(true)
	vt.mu.Unlock()
	stride := VideoModes[chip.currentMode].bytesPerRow
	if got := pixelAt(chip.GetFrontBuffer(), stride, 0, 0); got != vt.fgColor {
		t.Fatalf("expected visible cursor pixel to be fgColor, got 0x%08X", got)
	}
}

func TestCursorRender_Hidden(t *testing.T) {
	vt, chip, _ := newVideoTerminalForTest(t)
	vt.mu.Lock()
	vt.renderCursorCellLocked(true)
	vt.renderCursorCellLocked(false)
	vt.mu.Unlock()
	stride := VideoModes[chip.currentMode].bytesPerRow
	if got := pixelAt(chip.GetFrontBuffer(), stride, 0, 0); got != vt.bgColor {
		t.Fatalf("expected hidden cursor cell to restore bg, got 0x%08X", got)
	}
}

func TestCursorAutoHide_NoStatusReads(t *testing.T) {
	vt, chip, term := newVideoTerminalForTest(t)
	term.lastStatusRead.Store(time.Now().Add(-time.Second).UnixNano())
	vt.mu.Lock()
	vt.cursorOn = true
	vt.renderCursorCellLocked(true)
	vt.mu.Unlock()

	vt.cursorTick()
	if vt.cursorOn {
		t.Fatal("expected cursor to hide when status reads are stale")
	}

	fb := chip.GetFrontBuffer()
	stride := VideoModes[chip.currentMode].bytesPerRow
	if got := pixelAt(fb, stride, 0, 0); got != vt.bgColor {
		t.Fatalf("expected cursor cell restored to background, got 0x%08X", got)
	}
}

func TestCursorAutoHide_WithStatusReads(t *testing.T) {
	vt, chip, term := newVideoTerminalForTest(t)
	term.lastStatusRead.Store(time.Now().UnixNano())
	vt.cursorOn = false

	vt.cursorTick()
	if !vt.cursorOn {
		t.Fatal("expected cursor to toggle on when status reads are recent")
	}

	fb := chip.GetFrontBuffer()
	stride := VideoModes[chip.currentMode].bytesPerRow
	if got := pixelAt(fb, stride, 0, 0); got != vt.fgColor {
		t.Fatalf("expected visible cursor fg pixel, got 0x%08X", got)
	}
}
