package main

import (
	"strings"
	"testing"
)

func TestScreenBuffer_New(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	x, y := sb.CursorPos()
	if x != 0 || y != 0 {
		t.Fatalf("expected cursor (0,0), got (%d,%d)", x, y)
	}
	for row := range 30 {
		for col := range 80 {
			if got := sb.GetCell(col, row); got != 0 {
				t.Fatalf("expected zero cell at (%d,%d), got %d", col, row, got)
			}
		}
	}
}

func TestScreenBuffer_SetGetCell(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.SetCell(1, 2, 'A')
	if got := sb.GetCell(1, 2); got != 'A' {
		t.Fatalf("expected 'A', got %q", got)
	}
	if got := sb.GetCell(-1, 0); got != 0 {
		t.Fatalf("expected OOB get to be 0, got %q", got)
	}
	if got := sb.GetCell(0, 3000); got != 0 {
		t.Fatalf("expected OOB get to be 0, got %q", got)
	}
}

func TestScreenBuffer_PutChar_Printable(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	scrolled := sb.PutChar('A')
	if scrolled {
		t.Fatal("expected no scroll")
	}
	if got := sb.GetCell(0, 0); got != 'A' {
		t.Fatalf("expected A at (0,0), got %q", got)
	}
	x, y := sb.CursorPos()
	if x != 1 || y != 0 {
		t.Fatalf("expected cursor (1,0), got (%d,%d)", x, y)
	}
}

func TestScreenBuffer_PutChar_Sequence(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	for _, ch := range "Hello" {
		sb.PutChar(byte(ch))
	}
	for i, ch := range []byte("Hello") {
		if got := sb.GetCell(i, 0); got != ch {
			t.Fatalf("expected %q at col %d, got %q", ch, i, got)
		}
	}
	x, y := sb.CursorPos()
	if x != 5 || y != 0 {
		t.Fatalf("expected cursor (5,0), got (%d,%d)", x, y)
	}
}

func TestScreenBuffer_PutChar_CR(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.PutChar('A')
	sb.PutChar('B')
	sb.PutChar('\r')
	x, y := sb.CursorPos()
	if x != 0 || y != 0 {
		t.Fatalf("expected cursor (0,0), got (%d,%d)", x, y)
	}
}

func TestScreenBuffer_PutChar_LF(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.PutChar('\n')
	_, y := sb.CursorPos()
	if y != 1 {
		t.Fatalf("expected y=1, got %d", y)
	}
}

func TestScreenBuffer_PutChar_LineWrap(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	for range 80 {
		sb.PutChar('X')
	}
	x, y := sb.CursorPos()
	if x != 0 || y != 1 {
		t.Fatalf("expected cursor (0,1), got (%d,%d)", x, y)
	}
}

func TestScreenBuffer_PutChar_BS_AtCol0(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.PutChar('\b')
	x, y := sb.CursorPos()
	if x != 0 || y != 0 {
		t.Fatalf("expected no-op BS, got (%d,%d)", x, y)
	}
}

func TestScreenBuffer_PutChar_BS(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.PutChar('A')
	sb.PutChar('B')
	sb.PutChar('\b')
	x, y := sb.CursorPos()
	if x != 1 || y != 0 {
		t.Fatalf("expected cursor (1,0), got (%d,%d)", x, y)
	}
	if got := sb.GetCell(1, 0); got != 'B' {
		t.Fatalf("expected non-destructive BS, got %q", got)
	}
}

func TestScreenBuffer_PutChar_TAB(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.PutChar('\t')
	x, _ := sb.CursorPos()
	if x != 8 {
		t.Fatalf("expected TAB to col 8, got %d", x)
	}
}

func TestScreenBuffer_PutChar_FF(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.PutChar('A')
	sb.PutChar('\f')
	x, y := sb.CursorPos()
	if x != 0 || y != 0 {
		t.Fatalf("expected cursor reset after FF, got (%d,%d)", x, y)
	}
	if got := sb.GetCell(0, 0); got != 0 {
		t.Fatalf("expected cleared cell, got %q", got)
	}
}

func TestScreenBuffer_PutChar_ScrollAtBottom(t *testing.T) {
	sb := NewScreenBuffer(80, 2, 1000)
	sb.cursorY = 1
	scrolled := sb.PutChar('\n')
	if !scrolled {
		t.Fatal("expected scroll on LF at bottom")
	}
	x, y := sb.CursorPos()
	if x != 0 || y != 2 {
		t.Fatalf("expected cursor at (0,2), got (%d,%d)", x, y)
	}
	if top := sb.ViewportTop(); top != 1 {
		t.Fatalf("expected viewportTop 1, got %d", top)
	}
}

func TestScreenBuffer_PutChar_NoScroll(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	if scrolled := sb.PutChar('Q'); scrolled {
		t.Fatal("expected no scroll on printable")
	}
}

func TestScreenBuffer_MoveCursor(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.MoveCursor(1, 0)
	x, y := sb.CursorPos()
	if x != 1 || y != 0 {
		t.Fatalf("expected (1,0), got (%d,%d)", x, y)
	}
	sb.cursorX = 79
	sb.MoveCursor(1, 0)
	x, y = sb.CursorPos()
	if x != 0 || y != 1 {
		t.Fatalf("expected wrap to (0,1), got (%d,%d)", x, y)
	}
	sb.MoveCursor(-1, 0)
	x, y = sb.CursorPos()
	if x != 79 || y != 0 {
		t.Fatalf("expected wrap left to (79,0), got (%d,%d)", x, y)
	}
}

func TestScreenBuffer_MoveCursorUpDown(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.MoveCursor(0, 1)
	_, y := sb.CursorPos()
	if y != 1 {
		t.Fatalf("expected y=1, got %d", y)
	}
	sb.MoveCursor(0, -1)
	_, y = sb.CursorPos()
	if y != 0 {
		t.Fatalf("expected y=0, got %d", y)
	}
	sb.MoveCursor(0, -1)
	_, y = sb.CursorPos()
	if y != 0 {
		t.Fatalf("expected top no-op, got %d", y)
	}
}

func TestScreenBuffer_HomeEnd(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	for _, ch := range "ABC" {
		sb.PutChar(byte(ch))
	}
	sb.Home()
	x, _ := sb.CursorPos()
	if x != 0 {
		t.Fatalf("expected home col 0, got %d", x)
	}
	sb.End()
	x, _ = sb.CursorPos()
	if x != 3 {
		t.Fatalf("expected end col 3, got %d", x)
	}
}

func TestScreenBuffer_InsertChar_MidLine(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	for _, ch := range "HLLO" {
		sb.PutChar(byte(ch))
	}
	sb.cursorX = 1 // between H and L
	sb.InsertChar('E')
	if got := sb.ReadLine(0); got != "HELLO" {
		t.Fatalf("expected HELLO, got %q", got)
	}
	x, _ := sb.CursorPos()
	if x != 2 {
		t.Fatalf("expected cursor at 2 after insert, got %d", x)
	}
}

func TestScreenBuffer_InsertChar_AtEnd(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	for _, ch := range "AB" {
		sb.PutChar(byte(ch))
	}
	sb.InsertChar('C')
	if got := sb.ReadLine(0); got != "ABC" {
		t.Fatalf("expected ABC, got %q", got)
	}
	x, _ := sb.CursorPos()
	if x != 3 {
		t.Fatalf("expected cursor at 3, got %d", x)
	}
}

func TestScreenBuffer_BackspaceDeleteChar(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	for _, ch := range "HELLO" {
		sb.PutChar(byte(ch))
	}
	sb.BackspaceChar()
	if got := sb.ReadLine(0); got != "HELL" {
		t.Fatalf("expected HELL, got %q", got)
	}
	sb.Home()
	sb.MoveCursor(1, 0) // on E
	sb.DeleteChar()
	if got := sb.ReadLine(0); got != "HLL" {
		t.Fatalf("expected HLL, got %q", got)
	}
}

func TestScreenBuffer_BackspaceChar_AtCol0(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.BackspaceChar()
	x, y := sb.CursorPos()
	if x != 0 || y != 0 {
		t.Fatalf("expected no-op at col0, got (%d,%d)", x, y)
	}
}

func TestScreenBuffer_ReadLine_Trimmed(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	for _, ch := range "HELLO   " {
		sb.PutChar(byte(ch))
	}
	if got := sb.ReadLine(0); got != "HELLO" {
		t.Fatalf("expected trimmed HELLO, got %q", got)
	}
}

func TestScreenBuffer_Scrollback_MaxLines(t *testing.T) {
	sb := NewScreenBuffer(5, 2, 4)
	for range 8 {
		sb.PutChar('\n')
	}
	if got := sb.TotalLines(); got != 4 {
		t.Fatalf("expected max 4 lines, got %d", got)
	}
}

func TestScreenBuffer_VisibleCell(t *testing.T) {
	sb := NewScreenBuffer(10, 2, 10)
	sb.SetCell(0, 0, 'A')
	sb.SetCell(0, 1, 'B')
	sb.SetCell(0, 2, 'C')
	sb.viewportTop = 1
	if got := sb.VisibleCell(0, 0); got != 'B' {
		t.Fatalf("expected B, got %q", got)
	}
	if got := sb.VisibleCell(0, 1); got != 'C' {
		t.Fatalf("expected C, got %q", got)
	}
}

func TestScreenBuffer_ReadLine_Empty(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	if got := sb.ReadLine(0); got != "" {
		t.Fatalf("expected empty line, got %q", got)
	}
}

func TestScreenBuffer_PutChar_CRLF(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.PutChar('A')
	sb.PutChar('\r')
	sb.PutChar('\n')
	x, y := sb.CursorPos()
	if x != 0 || y != 1 {
		t.Fatalf("expected (0,1), got (%d,%d)", x, y)
	}
}

func TestScreenBuffer_OverwriteChar(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	sb.PutChar('A')
	sb.MoveCursor(-1, 0)
	sb.PutChar('B')
	if got := sb.GetCell(0, 0); got != 'B' {
		t.Fatalf("expected overwrite with B, got %q", got)
	}
}

func TestScreenBuffer_ScrollUp(t *testing.T) {
	sb := NewScreenBuffer(4, 2, 10)
	sb.SetCell(0, 0, 'A')
	sb.SetCell(0, 1, 'B')
	sb.ScrollUp()
	if got := sb.VisibleCell(0, 0); got != 'B' {
		t.Fatalf("expected top line to be old row1, got %q", got)
	}
	if got := sb.VisibleCell(0, 1); got != 0 {
		t.Fatalf("expected bottom line cleared, got %q", got)
	}
}

func TestScreenBuffer_Scrollback_Preserved(t *testing.T) {
	sb := NewScreenBuffer(4, 2, 10)
	copy(sb.lines[0], []byte("ABCD"))
	copy(sb.lines[1], []byte("EFGH"))
	sb.ScrollUp()
	if got := strings.TrimRight(sb.ReadLine(0), " "); got != "ABCD" {
		t.Fatalf("expected row 0 preserved in history, got %q", got)
	}
}

func TestScreenBuffer_Scrollback_CursorUpDown(t *testing.T) {
	sb := NewScreenBuffer(10, 2, 100)
	for range 5 {
		sb.PutChar('\n')
	}
	_, y := sb.CursorPos()
	if y != 5 {
		t.Fatalf("expected y=5, got %d", y)
	}
	if top := sb.ViewportTop(); top != 4 {
		t.Fatalf("expected viewportTop 4, got %d", top)
	}

	sb.MoveCursor(0, -3)
	if top := sb.ViewportTop(); top != 2 {
		t.Fatalf("expected viewportTop move up to 2, got %d", top)
	}
	sb.MoveCursor(0, 2)
	if top := sb.ViewportTop(); top != 3 {
		t.Fatalf("expected viewportTop move down to 3, got %d", top)
	}
}

func TestScreenBuffer_ScrollViewport(t *testing.T) {
	sb := NewScreenBuffer(10, 2, 100)
	for range 10 {
		sb.PutChar('\n')
	}
	// cursor at 10, viewportTop at 9
	sb.ScrollViewport(-3)
	if top := sb.ViewportTop(); top != 6 {
		t.Fatalf("expected viewport at 6 after scroll up 3, got %d", top)
	}
	sb.ScrollViewport(2)
	if top := sb.ViewportTop(); top != 8 {
		t.Fatalf("expected viewport at 8 after scroll down 2, got %d", top)
	}
}

func TestScreenBuffer_ScrollViewport_Clamping(t *testing.T) {
	sb := NewScreenBuffer(10, 2, 100)
	for range 5 {
		sb.PutChar('\n')
	}
	// viewport at 4, scroll way past top
	sb.ScrollViewport(-100)
	if top := sb.ViewportTop(); top != 0 {
		t.Fatalf("expected viewport clamped to 0, got %d", top)
	}
	// scroll way past bottom
	sb.ScrollViewport(100)
	if top := sb.ViewportTop(); top != 4 {
		t.Fatalf("expected viewport clamped to max, got %d", top)
	}
}

func TestScreenBuffer_WordLeft(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	for _, ch := range "HELLO WORLD" {
		sb.PutChar(byte(ch))
	}
	// cursor at 11 (end)
	sb.WordLeft()
	x, _ := sb.CursorPos()
	if x != 6 {
		t.Fatalf("expected WordLeft to go to 6 (start of WORLD), got %d", x)
	}
	sb.WordLeft()
	x, _ = sb.CursorPos()
	if x != 0 {
		t.Fatalf("expected WordLeft to go to 0 (start of HELLO), got %d", x)
	}
	sb.WordLeft()
	x, _ = sb.CursorPos()
	if x != 0 {
		t.Fatalf("expected WordLeft at start to stay at 0, got %d", x)
	}
}

func TestScreenBuffer_WordRight(t *testing.T) {
	sb := NewScreenBuffer(80, 30, 1000)
	for _, ch := range "HELLO WORLD" {
		sb.PutChar(byte(ch))
	}
	sb.Home()
	sb.WordRight()
	x, _ := sb.CursorPos()
	if x != 6 {
		t.Fatalf("expected WordRight to go to 6 (start of WORLD), got %d", x)
	}
	sb.WordRight()
	x, _ = sb.CursorPos()
	if x != 11 {
		t.Fatalf("expected WordRight to go to 11 (end), got %d", x)
	}
}
