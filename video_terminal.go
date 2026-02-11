package main

import (
	_ "embed"
	"sync"
	"time"
)

const (
	terminalGlyphWidth  = 8
	terminalGlyphHeight = 16
	tabWidth            = 8
	cursorPollWindow    = 200 * time.Millisecond
	cursorBlinkPeriod   = 500 * time.Millisecond
)

//go:embed TopazPlus_a1200_v1.0.raw
var topazRawFont []byte

func loadTopazFont() [256][16]byte {
	var glyphs [256][16]byte
	if len(topazRawFont) < len(glyphs)*len(glyphs[0]) {
		return glyphs
	}
	offset := 0
	for g := range len(glyphs) {
		copy(glyphs[g][:], topazRawFont[offset:offset+terminalGlyphHeight])
		offset += terminalGlyphHeight
	}
	return glyphs
}

type VideoTerminal struct {
	video       *VideoChip
	term        *TerminalMMIO
	mu          sync.Mutex
	screen      *ScreenBuffer
	cols        int
	rows        int
	pixelWidth  int
	pixelHeight int
	cursorOn    bool
	fgColor     uint32
	bgColor     uint32
	glyphs      [256][16]byte
	escState    int
	escParam    byte

	inputEscState int
	inputEscParam byte

	done     chan struct{}
	stopOnce sync.Once
}

func NewVideoTerminal(video *VideoChip, term *TerminalMMIO) *VideoTerminal {
	mode := VideoModes[video.currentMode]
	cols := mode.width / terminalGlyphWidth
	rows := mode.height / terminalGlyphHeight
	if cols <= 0 {
		cols = 1
	}
	if rows <= 0 {
		rows = 1
	}

	vt := &VideoTerminal{
		video:       video,
		term:        term,
		screen:      NewScreenBuffer(cols, rows, 1000),
		cols:        cols,
		rows:        rows,
		pixelWidth:  mode.width,
		pixelHeight: mode.height,
		cursorOn:    true,
		fgColor:     0xFFFFFFFF,
		bgColor:     0xFFAA5500,
		glyphs:      loadTopazFont(),
		done:        make(chan struct{}),
	}

	vt.clearScreen()
	term.SetCharOutputCallback(vt.processChar)
	return vt
}

func (vt *VideoTerminal) Start() {
	go func() {
		ticker := time.NewTicker(cursorBlinkPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-vt.done:
				return
			case <-ticker.C:
				vt.cursorTick()
			}
		}
	}()
}

func (vt *VideoTerminal) Stop() {
	vt.stopOnce.Do(func() {
		close(vt.done)
		vt.term.SetCharOutputCallback(nil)
	})
}

func (vt *VideoTerminal) processChar(ch byte) {
	vt.mu.Lock()
	defer vt.mu.Unlock()

	if vt.cursorOn {
		vt.renderCursorCellLocked(false)
	}

	if vt.handleOutputEscapeLocked(ch) {
		vt.cursorOn = true
		if vt.shouldShowCursorLocked() {
			vt.renderCursorCellLocked(true)
		}
		return
	}

	switch ch {
	case '\b':
		vt.screen.PutChar('\b')
		cx, cy := vt.screen.CursorPos()
		vt.screen.SetCell(cx, cy, 0)
		vrow := cy - vt.screen.ViewportTop()
		vt.renderRowFromLocked(vrow, cx)
	case '\f':
		vt.screen.Clear()
		vt.clearScreenLocked()
	default:
		beforeTop := vt.screen.ViewportTop()
		beforeX, beforeY := vt.screen.CursorPos()
		scrolled := vt.screen.PutChar(ch)
		if scrolled || vt.screen.ViewportTop() != beforeTop {
			vt.renderViewportLocked()
		} else if ch >= 0x20 {
			vrow := beforeY - vt.screen.ViewportTop()
			if vrow >= 0 && vrow < vt.rows {
				vt.renderCellLocked(beforeX, vrow, ch)
			}
		}
	}

	vt.cursorOn = true
	if vt.shouldShowCursorLocked() {
		vt.renderCursorCellLocked(true)
	}
}

func (vt *VideoTerminal) clearScreen() {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	vt.screen.Clear()
	vt.clearScreenLocked()
}

func (vt *VideoTerminal) clearScreenLocked() {
	vt.video.RenderToFrontBuffer(func(fb []byte, _ int) {
		for i := 0; i < len(fb); i += 4 {
			writeColorLE(fb, i, vt.bgColor)
		}
	})
	vt.video.MarkRectDirty(0, 0, vt.pixelWidth, vt.pixelHeight)
}

func (vt *VideoTerminal) renderCellLocked(col, row int, ch byte) {
	if col < 0 || col >= vt.cols || row < 0 || row >= vt.rows {
		return
	}

	baseX := col * terminalGlyphWidth
	baseY := row * terminalGlyphHeight
	glyph := vt.glyphs[ch]
	vt.video.RenderToFrontBuffer(func(fb []byte, stride int) {
		for gy := range terminalGlyphHeight {
			rowBits := glyph[gy]
			dst := (baseY+gy)*stride + baseX*4
			for gx := range terminalGlyphWidth {
				color := vt.bgColor
				if (rowBits & (0x80 >> gx)) != 0 {
					color = vt.fgColor
				}
				writeColorLE(fb, dst+gx*4, color)
			}
		}
	})
	vt.video.MarkRectDirty(baseX, baseY, terminalGlyphWidth, terminalGlyphHeight)
}

func (vt *VideoTerminal) renderCursorCellLocked(visible bool) {
	cursorX, cursorY := vt.screen.CursorViewportPos()
	if cursorX < 0 || cursorX >= vt.cols || cursorY < 0 || cursorY >= vt.rows {
		return
	}
	if !visible {
		ch := vt.screen.VisibleCell(cursorX, cursorY)
		if ch == 0 {
			ch = ' '
		}
		vt.renderCellLocked(cursorX, cursorY, ch)
		return
	}

	baseX := cursorX * terminalGlyphWidth
	baseY := cursorY * terminalGlyphHeight
	vt.video.RenderToFrontBuffer(func(fb []byte, stride int) {
		for gy := range terminalGlyphHeight {
			dst := (baseY+gy)*stride + baseX*4
			for gx := range terminalGlyphWidth {
				writeColorLE(fb, dst+gx*4, vt.fgColor)
			}
		}
	})
	vt.video.MarkRectDirty(baseX, baseY, terminalGlyphWidth, terminalGlyphHeight)
}

func (vt *VideoTerminal) cursorTick() {
	vt.mu.Lock()
	defer vt.mu.Unlock()

	if !vt.shouldShowCursorLocked() {
		if vt.cursorOn {
			vt.cursorOn = false
			vt.renderCursorCellLocked(false)
		}
		return
	}

	vt.cursorOn = !vt.cursorOn
	vt.renderCursorCellLocked(vt.cursorOn)
}

func (vt *VideoTerminal) shouldShowCursorLocked() bool {
	last := vt.term.LastStatusReadTime()
	if last.IsZero() {
		return false
	}
	return time.Since(last) <= cursorPollWindow
}

func (vt *VideoTerminal) renderViewportLocked() {
	for row := 0; row < vt.rows; row++ {
		for col := 0; col < vt.cols; col++ {
			ch := vt.screen.VisibleCell(col, row)
			if ch == 0 {
				ch = ' '
			}
			vt.renderCellLocked(col, row, ch)
		}
	}
}

// renderRowFromLocked re-renders visible row vrow from column startCol to end.
func (vt *VideoTerminal) renderRowFromLocked(vrow, startCol int) {
	if vrow < 0 || vrow >= vt.rows {
		return
	}
	for col := startCol; col < vt.cols; col++ {
		ch := vt.screen.VisibleCell(col, vrow)
		if ch == 0 {
			ch = ' '
		}
		vt.renderCellLocked(col, vrow, ch)
	}
}

// renderVisibleCellLocked renders a single cell using its ScreenBuffer content.
func (vt *VideoTerminal) renderVisibleCellLocked(col, vrow int) {
	ch := vt.screen.VisibleCell(col, vrow)
	if ch == 0 {
		ch = ' '
	}
	vt.renderCellLocked(col, vrow, ch)
}

func (vt *VideoTerminal) setCellLocked(col, row int, ch byte) {
	vt.screen.SetCell(col, vt.screen.ViewportTop()+row, ch)
}

func (vt *VideoTerminal) getCellLocked(col, row int) byte {
	return vt.screen.GetCell(col, vt.screen.ViewportTop()+row)
}

func (vt *VideoTerminal) scrollUpLocked() {
	vt.screen.ScrollUp()
	vt.renderViewportLocked()
}

func (vt *VideoTerminal) HandleKeyInput(b byte) {
	vt.mu.Lock()
	defer vt.mu.Unlock()

	if vt.cursorOn {
		vt.renderCursorCellLocked(false)
	}

	lineMode := vt.term.RouteGraphicalKey(b)

	redrawAll := vt.handleInputByteLocked(b, lineMode)
	if redrawAll {
		vt.renderViewportLocked()
	}

	vt.cursorOn = true
	if vt.shouldShowCursorLocked() {
		vt.renderCursorCellLocked(true)
	}
}

func (vt *VideoTerminal) handleOutputEscapeLocked(ch byte) bool {
	beforeTop := vt.screen.ViewportTop()
	switch vt.escState {
	case 0:
		if ch == 0x1B {
			vt.escState = 1
			return true
		}
		return false
	case 1:
		if ch == '[' {
			vt.escState = 2
			return true
		}
		vt.escState = 0
		return false
	case 2:
		vt.escState = 0
		switch ch {
		case 'A':
			vt.screen.MoveCursor(0, -1)
		case 'B':
			vt.screen.MoveCursor(0, 1)
		case 'C':
			vt.screen.MoveCursor(1, 0)
		case 'D':
			vt.screen.MoveCursor(-1, 0)
		case 'H':
			vt.screen.Home()
		case 'F':
			vt.screen.End()
		default:
			if ch >= '0' && ch <= '9' {
				vt.escParam = ch
				vt.escState = 3
			}
		}
		if vt.screen.ViewportTop() != beforeTop {
			vt.renderViewportLocked()
		}
		return true
	case 3:
		vt.escState = 0
		if ch == '~' && vt.escParam == '3' {
			cx, cy := vt.screen.CursorPos()
			vt.screen.DeleteChar()
			vrow := cy - vt.screen.ViewportTop()
			vt.renderRowFromLocked(vrow, cx)
		}
		return true
	default:
		vt.escState = 0
		return true
	}
}

func (vt *VideoTerminal) handleInputByteLocked(b byte, lineMode bool) bool {
	if vt.handleInputEscapeLocked(b) {
		return false // escape handler does its own targeted rendering
	}

	if b == 0x1B {
		vt.inputEscState = 1
		return false
	}

	switch b {
	case '\r', '\n':
		if lineMode {
			_, absRow := vt.screen.CursorPos()
			line := vt.screen.ReadLine(absRow)
			for i := 0; i < len(line); i++ {
				vt.term.EnqueueByte(line[i])
			}
			vt.term.EnqueueByte('\n')
		}
		beforeTop := vt.screen.ViewportTop()
		vt.screen.PutChar('\r')
		vt.screen.PutChar('\n')
		return vt.screen.ViewportTop() != beforeTop // full redraw only on scroll
	case '\b':
		cx, cy := vt.screen.CursorPos()
		vt.screen.BackspaceChar()
		vrow := cy - vt.screen.ViewportTop()
		if cx > 0 {
			vt.renderRowFromLocked(vrow, cx-1)
		}
		return false
	case '\t':
		beforeTop := vt.screen.ViewportTop()
		vt.screen.PutChar('\t')
		return vt.screen.ViewportTop() != beforeTop
	default:
		if b < 0x20 {
			return false
		}
		beforeTop := vt.screen.ViewportTop()
		beforeX, beforeY := vt.screen.CursorPos()
		scrolled := vt.screen.PutChar(b)
		if scrolled || vt.screen.ViewportTop() != beforeTop {
			return true // full redraw on scroll
		}
		vrow := beforeY - vt.screen.ViewportTop()
		if vrow >= 0 && vrow < vt.rows {
			vt.renderCellLocked(beforeX, vrow, b)
		}
		return false
	}
}

func (vt *VideoTerminal) handleInputEscapeLocked(b byte) bool {
	switch vt.inputEscState {
	case 0:
		return false
	case 1:
		if b == '[' {
			vt.inputEscState = 2
			return true
		}
		vt.inputEscState = 0
		return false
	case 2:
		vt.inputEscState = 0
		beforeTop := vt.screen.ViewportTop()
		switch b {
		case 'A':
			vt.screen.MoveCursor(0, -1)
		case 'B':
			vt.screen.MoveCursor(0, 1)
		case 'C':
			vt.screen.MoveCursor(1, 0)
		case 'D':
			vt.screen.MoveCursor(-1, 0)
		case 'H':
			vt.screen.Home()
		case 'F':
			vt.screen.End()
		default:
			if b >= '0' && b <= '9' {
				vt.inputEscParam = b
				vt.inputEscState = 3
			}
		}
		if vt.screen.ViewportTop() != beforeTop {
			vt.renderViewportLocked()
		}
		return true
	case 3:
		vt.inputEscState = 0
		if b == '~' && vt.inputEscParam == '3' {
			cx, cy := vt.screen.CursorPos()
			vt.screen.DeleteChar()
			vrow := cy - vt.screen.ViewportTop()
			vt.renderRowFromLocked(vrow, cx)
		}
		return true
	default:
		vt.inputEscState = 0
		return true
	}
}

func writeColorLE(buf []byte, offset int, color uint32) {
	buf[offset] = byte(color)
	buf[offset+1] = byte(color >> 8)
	buf[offset+2] = byte(color >> 16)
	buf[offset+3] = byte(color >> 24)
}
