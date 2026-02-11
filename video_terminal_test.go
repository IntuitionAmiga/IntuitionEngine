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

func feedSeq(vt *VideoTerminal, seq ...byte) {
	for _, b := range seq {
		vt.processChar(b)
	}
}

func feedInput(vt *VideoTerminal, seq ...byte) {
	for _, b := range seq {
		vt.HandleKeyInput(b)
	}
}

func drainTermIn(term *TerminalMMIO) string {
	var out []byte
	for term.HandleRead(TERM_STATUS)&1 != 0 {
		out = append(out, byte(term.HandleRead(TERM_IN)))
	}
	return string(out)
}

func drainRawKeys(term *TerminalMMIO) []byte {
	var out []byte
	for term.HandleRead(TERM_KEY_STATUS)&1 != 0 {
		out = append(out, byte(term.HandleRead(TERM_KEY_IN)))
	}
	return out
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
	for y := range terminalGlyphHeight {
		rowBits := glyph[y]
		for x := range terminalGlyphWidth {
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
	for y := range terminalGlyphHeight {
		rowBits := glyph[y]
		for x := range terminalGlyphWidth {
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
	if got := vt.screen.VisibleCell(0, 0); got != 'H' {
		t.Fatalf("expected first cell 'H', got %q", got)
	}
	x, y := vt.screen.CursorPos()
	if x != 1 || y != 0 {
		t.Fatalf("expected cursor (1,0), got (%d,%d)", x, y)
	}
}

func TestProcessChar_Sequence(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('H')
	vt.processChar('i')
	if got0, got1 := vt.screen.VisibleCell(0, 0), vt.screen.VisibleCell(1, 0); got0 != 'H' || got1 != 'i' {
		t.Fatalf("expected 'Hi' in first two cells, got %q%q", got0, got1)
	}
	x, y := vt.screen.CursorPos()
	if x != 2 || y != 0 {
		t.Fatalf("expected cursor (2,0), got (%d,%d)", x, y)
	}
}

func TestProcessChar_CRLF(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('A')
	vt.processChar('B')
	vt.processChar('\r')
	x, y := vt.screen.CursorPos()
	if x != 0 || y != 0 {
		t.Fatalf("after CR expected cursor (0,0), got (%d,%d)", x, y)
	}
	vt.processChar('\n')
	x, y = vt.screen.CursorPos()
	if x != 0 || y != 1 {
		t.Fatalf("after CRLF expected cursor (0,1), got (%d,%d)", x, y)
	}
}

func TestProcessChar_CR(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('A')
	vt.processChar('B')
	vt.processChar('\r')
	x, y := vt.screen.CursorPos()
	if x != 0 || y != 0 {
		t.Fatalf("after CR expected cursor (0,0), got (%d,%d)", x, y)
	}
}

func TestProcessChar_LF(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('\n')
	_, y := vt.screen.CursorPos()
	if y != 1 {
		t.Fatalf("after LF expected cursorY 1, got %d", y)
	}
}

func TestProcessChar_BS(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('A')
	vt.processChar('B')
	vt.processChar('\b')
	x, y := vt.screen.CursorPos()
	if x != 1 || y != 0 {
		t.Fatalf("after BS expected cursor (1,0), got (%d,%d)", x, y)
	}
	if got := vt.screen.VisibleCell(1, 0); got != 0 {
		t.Fatalf("expected cleared cell at index 1, got %q", got)
	}
}

func TestProcessChar_BS_AtCol0(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('\b')
	x, y := vt.screen.CursorPos()
	if x != 0 || y != 0 {
		t.Fatalf("expected BS at col 0 to no-op, got (%d,%d)", x, y)
	}
}

func TestProcessChar_TAB(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('\t')
	x, _ := vt.screen.CursorPos()
	if x != 8 {
		t.Fatalf("expected cursorX 8 after TAB from col 0, got %d", x)
	}
	vt.screen.cursorX = 8
	vt.processChar('\t')
	x, _ = vt.screen.CursorPos()
	if x != 16 {
		t.Fatalf("expected cursorX 16 after TAB from col 8, got %d", x)
	}
}

func TestProcessChar_TAB_Aligned(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.screen.cursorX = 8
	vt.processChar('\t')
	x, _ := vt.screen.CursorPos()
	if x != 16 {
		t.Fatalf("expected aligned TAB to advance to 16, got %d", x)
	}
}

func TestProcessChar_LineWrap(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	for i := 0; i < vt.cols; i++ {
		vt.processChar('X')
	}
	x, y := vt.screen.CursorPos()
	if x != 0 || y != 1 {
		t.Fatalf("expected wrap to (0,1), got (%d,%d)", x, y)
	}
}

func TestProcessChar_LineWrap_AtBottom(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.screen.cursorY = vt.rows - 1
	vt.screen.cursorX = vt.cols - 1
	vt.processChar('X')
	x, y := vt.screen.CursorPos()
	if y != vt.rows || x != 0 {
		t.Fatalf("expected wrap to new absolute line with col0, got (%d,%d)", x, y)
	}
}

func TestScrollUp_TextBuf(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.mu.Lock()
	vt.setCellLocked(0, 0, 'A')
	vt.setCellLocked(0, 1, 'B')
	vt.scrollUpLocked()
	vt.mu.Unlock()

	if got := vt.screen.VisibleCell(0, 0); got != 'B' {
		t.Fatalf("expected old row 1 at top after scroll, got %q", got)
	}
	if got := vt.screen.VisibleCell(0, vt.rows-1); got != 0 {
		t.Fatalf("expected cleared last row, got %q", got)
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

	for row := 0; row < vt.rows; row++ {
		for col := 0; col < vt.cols; col++ {
			if b := vt.screen.VisibleCell(col, row); b != 0 {
				t.Fatalf("expected text buffer cleared at (%d,%d)", col, row)
			}
		}
	}
	x, y := vt.screen.CursorPos()
	if x != 0 || y != 0 {
		t.Fatalf("expected cursor reset to (0,0), got (%d,%d)", x, y)
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

func TestProcessChar_EscSeq_CursorRight(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('A')
	vt.processChar('B')
	vt.processChar(0x1B)
	vt.processChar('[')
	vt.processChar('C')

	x, y := vt.screen.CursorPos()
	if x != 3 || y != 0 {
		t.Fatalf("expected cursor (3,0), got (%d,%d)", x, y)
	}
	if got := vt.screen.VisibleCell(2, 0); got != 0 {
		t.Fatalf("expected no literal escape bytes rendered, got %q", got)
	}
}

func TestProcessChar_EscSeq_Unknown(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar('A')
	vt.processChar(0x1B)
	vt.processChar('[')
	vt.processChar('Z')
	if got := vt.screen.ReadLine(0); got != "A" {
		t.Fatalf("expected unknown CSI to be ignored, got %q", got)
	}
}

func TestProcessChar_EscSeq_CursorLeftUpDown(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.screen.cursorX = 5
	vt.screen.cursorY = 5
	feedSeq(vt, 0x1B, '[', 'A')
	x, y := vt.screen.CursorPos()
	if x != 5 || y != 4 {
		t.Fatalf("expected up to (5,4), got (%d,%d)", x, y)
	}
	feedSeq(vt, 0x1B, '[', 'B')
	feedSeq(vt, 0x1B, '[', 'D')
	x, y = vt.screen.CursorPos()
	if x != 4 || y != 5 {
		t.Fatalf("expected down+left to (4,5), got (%d,%d)", x, y)
	}
}

func TestProcessChar_EscSeq_IncompleteAndMultiple(t *testing.T) {
	vt, _, _ := newVideoTerminalForTest(t)
	vt.processChar(0x1B)
	vt.processChar('X')
	if got := vt.screen.VisibleCell(0, 0); got != 'X' {
		t.Fatalf("expected X rendered after incomplete ESC, got %q", got)
	}
	feedSeq(vt, 0x1B, '[', 'C', 0x1B, '[', 'C')
	x, y := vt.screen.CursorPos()
	if x != 3 || y != 0 {
		t.Fatalf("expected two cursor-right moves to x=3, got (%d,%d)", x, y)
	}
}

func TestHandleKeyInput_LineMode_Printable(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	vt.HandleKeyInput('A')

	if got := vt.screen.VisibleCell(0, 0); got != 'A' {
		t.Fatalf("expected 'A' rendered, got %q", got)
	}
	if got := term.HandleRead(TERM_STATUS); got&1 != 0 {
		t.Fatalf("expected TERM_IN empty in line-mode typing, got status 0x%X", got)
	}
	if got := term.HandleRead(TERM_KEY_STATUS); got != 0 {
		t.Fatalf("expected TERM_KEY empty in line mode, got 0x%X", got)
	}
}

func TestHandleKeyInput_LineMode_ArrowAndHomeEndDelete(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "HELLO" {
		vt.HandleKeyInput(byte(ch))
	}
	feedInput(vt, 0x1B, '[', 'D', 0x1B, '[', 'D') // cursor before second L
	feedInput(vt, 0x1B, '[', '3', '~')            // delete second L
	if got := vt.screen.ReadLine(0); got != "HELO" {
		t.Fatalf("expected HELO after delete, got %q", got)
	}
	feedInput(vt, 0x1B, '[', 'H')
	x, _ := vt.screen.CursorPos()
	if x != 0 {
		t.Fatalf("expected home x=0, got %d", x)
	}
	feedInput(vt, 0x1B, '[', 'F')
	x, _ = vt.screen.CursorPos()
	if x != 4 {
		t.Fatalf("expected end x=4, got %d", x)
	}
}

func TestHandleKeyInput_LineMode_BackspaceDestructive(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "HELLO" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\b')
	if got := vt.screen.ReadLine(0); got != "HELL" {
		t.Fatalf("expected HELL after backspace, got %q", got)
	}
}

func TestHandleKeyInput_LineMode_ControlIgnored(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	vt.HandleKeyInput(0x01)
	if got := vt.screen.ReadLine(0); got != "" {
		t.Fatalf("expected ignored control, got %q", got)
	}
	if term.HandleRead(TERM_STATUS)&1 != 0 || term.HandleRead(TERM_KEY_STATUS)&1 != 0 {
		t.Fatal("expected no channel enqueue for ignored control")
	}
}

func TestHandleKeyInput_LineMode_ReturnSubmitsLine(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "PRINT 42" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')

	var got []byte
	for term.HandleRead(TERM_STATUS)&1 != 0 {
		got = append(got, byte(term.HandleRead(TERM_IN)))
	}
	if string(got) != "PRINT 42\n" {
		t.Fatalf("expected submitted line, got %q", string(got))
	}
	if term.HandleRead(TERM_KEY_STATUS) != 0 {
		t.Fatal("expected no raw keys queued in line mode")
	}
}

func TestHandleKeyInput_LineMode_ReturnTrimmedAndEmpty(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "HELLO   " {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')
	vt.HandleKeyInput('\n')
	if got := drainTermIn(term); got != "HELLO\n\n" {
		t.Fatalf("expected trimmed HELLO and empty line, got %q", got)
	}
}

func TestHandleKeyInput_LineMode_ReturnMultipleLines(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	for _, line := range []string{"A", "B", "C"} {
		for i := 0; i < len(line); i++ {
			vt.HandleKeyInput(line[i])
		}
		vt.HandleKeyInput('\n')
	}
	if got := drainTermIn(term); got != "A\nB\nC\n" {
		t.Fatalf("expected three queued lines, got %q", got)
	}
}

func TestHandleKeyInput_CharMode_RawAndDisplay(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 0)
	vt.HandleKeyInput('A')

	if got := vt.screen.VisibleCell(0, 0); got != 'A' {
		t.Fatalf("expected visual echo in char mode, got %q", got)
	}
	if term.HandleRead(TERM_KEY_STATUS) != 1 {
		t.Fatal("expected raw key available")
	}
	if got := term.HandleRead(TERM_KEY_IN); got != 'A' {
		t.Fatalf("expected raw key 'A', got 0x%X", got)
	}
	if term.HandleRead(TERM_STATUS)&1 != 0 {
		t.Fatal("expected TERM_IN empty in char mode")
	}
}

func TestHandleKeyInput_CharMode_ArrowToRawAndMove(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 0)
	feedInput(vt, 0x1B, '[', 'C')
	if got := string(drainRawKeys(term)); got != "\x1b[C" {
		t.Fatalf("expected raw ESC[C sequence, got %q", got)
	}
	x, y := vt.screen.CursorPos()
	if x != 1 || y != 0 {
		t.Fatalf("expected cursor moved right in char mode, got (%d,%d)", x, y)
	}
}

func TestHandleKeyInput_CharMode_ReturnNoTermIn(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 0)
	vt.HandleKeyInput('\n')
	if term.HandleRead(TERM_STATUS)&1 != 0 {
		t.Fatal("expected TERM_IN empty for char-mode return")
	}
	if term.HandleRead(TERM_KEY_STATUS) != 1 {
		t.Fatal("expected raw return queued")
	}
	if got := term.HandleRead(TERM_KEY_IN); got != '\n' {
		t.Fatalf("expected raw '\\n', got 0x%X", got)
	}
}

func TestHandleKeyInput_LineMode_Sequence(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "Hello" {
		vt.HandleKeyInput(byte(ch))
	}
	for i, ch := range []byte("Hello") {
		if got := vt.screen.VisibleCell(i, 0); got != ch {
			t.Fatalf("expected %q at col %d, got %q", ch, i, got)
		}
	}
	x, y := vt.screen.CursorPos()
	if x != 5 || y != 0 {
		t.Fatalf("expected cursor (5,0), got (%d,%d)", x, y)
	}
}

func TestHandleKeyInput_LineMode_LineWrap(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	for i := 0; i < vt.cols; i++ {
		vt.HandleKeyInput('X')
	}
	x, y := vt.screen.CursorPos()
	if x != 0 || y != 1 {
		t.Fatalf("expected wrap to (0,1), got (%d,%d)", x, y)
	}
}

func TestHandleKeyInput_LineMode_RenderUpdate(t *testing.T) {
	vt, chip, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	vt.HandleKeyInput('A')
	fb := chip.GetFrontBuffer()
	stride := VideoModes[chip.currentMode].bytesPerRow
	glyph := vt.glyphs['A']
	// Check that at least one foreground pixel was rendered for glyph 'A'
	found := false
	for y := range terminalGlyphHeight {
		rowBits := glyph[y]
		for x := range terminalGlyphWidth {
			if (rowBits & (0x80 >> x)) != 0 {
				if pixelAt(fb, stride, x, y) == vt.fgColor {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("expected glyph pixel rendered after typing in line mode")
	}
}

func TestHandleKeyInput_LineMode_CursorRender(t *testing.T) {
	vt, chip, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	// Make cursor visible by setting recent status read
	term.lastStatusRead.Store(time.Now().UnixNano())
	vt.HandleKeyInput('A')
	// Cursor should be at (1,0) and rendered as a solid block
	fb := chip.GetFrontBuffer()
	stride := VideoModes[chip.currentMode].bytesPerRow
	// Check top-left pixel of cursor cell (1,0)
	px := 1 * terminalGlyphWidth
	if got := pixelAt(fb, stride, px, 0); got != vt.fgColor {
		t.Fatalf("expected cursor block pixel at new position, got 0x%08X want 0x%08X", got, vt.fgColor)
	}
}

func TestHandleKeyInput_Return_CursorNewLine(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	vt.HandleKeyInput('A')
	vt.HandleKeyInput('\n')
	x, y := vt.screen.CursorPos()
	if x != 0 {
		t.Fatalf("expected cursor col 0 after RETURN, got %d", x)
	}
	if y != 1 {
		t.Fatalf("expected cursor row 1 after RETURN, got %d", y)
	}
}

func TestHandleKeyInput_Return_MidScreen(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	// Type on row 0
	for _, ch := range "LINE0" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')
	_ = drainTermIn(term) // consume row 0 submit
	// Type on row 1
	for _, ch := range "LINE1" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')
	_ = drainTermIn(term) // consume row 1 submit
	// Type on row 2
	for _, ch := range "EDIT" {
		vt.HandleKeyInput(byte(ch))
	}
	// Navigate up to row 1 and RETURN → should read row 1 content
	feedInput(vt, 0x1B, '[', 'A') // cursor up
	vt.HandleKeyInput('\n')
	got := drainTermIn(term)
	if got != "LINE1\n" {
		t.Fatalf("expected mid-screen RETURN to read row 1, got %q", got)
	}
}

func TestHandleKeyInput_Return_LineStatus(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	for _, ch := range "TEST" {
		vt.HandleKeyInput(byte(ch))
	}
	vt.HandleKeyInput('\n')
	if got := term.HandleRead(TERM_LINE_STATUS); got&1 != 1 {
		t.Fatalf("expected TERM_LINE_STATUS=1 after RETURN, got 0x%X", got)
	}
}

func TestHandleKeyInput_Return_ScrollAtBottom(t *testing.T) {
	vt, _, term := newVideoTerminalForTest(t)
	term.HandleWrite(TERM_CTRL, 1)
	// Fill to last visible row
	for i := 0; i < vt.rows-1; i++ {
		vt.HandleKeyInput('A' + byte(i%26))
		vt.HandleKeyInput('\n')
		_ = drainTermIn(term)
	}
	// Now on last row — type and RETURN should scroll
	vt.HandleKeyInput('Z')
	beforeTop := vt.screen.ViewportTop()
	vt.HandleKeyInput('\n')
	_ = drainTermIn(term)
	_, cy := vt.screen.CursorViewportPos()
	if cy < 0 || cy >= vt.rows {
		t.Fatalf("expected cursor within viewport after scroll, viewport row=%d", cy)
	}
	afterTop := vt.screen.ViewportTop()
	if afterTop <= beforeTop {
		t.Fatalf("expected viewport to scroll, beforeTop=%d afterTop=%d", beforeTop, afterTop)
	}
}
