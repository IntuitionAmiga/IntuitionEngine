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

	inputEscState  int
	inputEscParam  byte
	inputEscParam2 byte

	inputStartCol int
	inputStartRow int
	inputActive   bool

	history    []string
	historyIdx int
	savedInput string

	selActive    bool
	selAnchorCol int
	selAnchorRow int
	selEndCol    int
	selEndRow    int
	selText      string

	clipboardWrite func([]byte)
	clipboardRead  func() []byte

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
		vt.history = nil
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

	cx, cy := vt.screen.CursorPos()
	vt.inputStartCol = cx
	vt.inputStartRow = cy
	vt.inputActive = true

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

	fg, bg := vt.fgColor, vt.bgColor
	if vt.selActive {
		absRow := vt.screen.ViewportTop() + row
		if vt.isInSelectionLocked(col, absRow) {
			fg, bg = bg, fg
		}
	}

	baseX := col * terminalGlyphWidth
	baseY := row * terminalGlyphHeight
	glyph := vt.glyphs[ch]
	vt.video.RenderToFrontBuffer(func(fb []byte, stride int) {
		for gy := range terminalGlyphHeight {
			rowBits := glyph[gy]
			dst := (baseY+gy)*stride + baseX*4
			for gx := range terminalGlyphWidth {
				color := bg
				if (rowBits & (0x80 >> gx)) != 0 {
					color = fg
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

func (vt *VideoTerminal) HandleScroll(delta int) {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	vt.selClearLocked()
	vt.screen.ScrollViewport(delta)
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
		vt.selClearLocked()
		if lineMode {
			_, absRow := vt.screen.CursorPos()
			line := vt.screen.ReadLine(absRow)
			// Strip prompt: only enqueue the user-typed portion (after inputStartCol)
			startCol := 0
			if vt.inputActive && absRow == vt.inputStartRow {
				startCol = vt.inputStartCol
			}
			if startCol < len(line) {
				line = line[startCol:]
			}
			for i := 0; i < len(line); i++ {
				vt.term.EnqueueByte(line[i])
			}
			vt.term.EnqueueByte('\n')
		}
		// Append to history before moving cursor
		{
			_, absRow := vt.screen.CursorPos()
			startCol := 0
			if vt.inputActive && absRow == vt.inputStartRow {
				startCol = vt.inputStartCol
			}
			line := vt.screen.ReadLine(absRow)
			if len(line) > startCol {
				entry := line[startCol:]
				if entry != "" {
					vt.history = append(vt.history, entry)
				}
			}
		}
		beforeTop := vt.screen.ViewportTop()
		vt.screen.PutChar('\r')
		vt.screen.PutChar('\n')
		cx, cy := vt.screen.CursorPos()
		vt.inputStartCol = cx
		vt.inputStartRow = cy
		vt.inputActive = true
		vt.historyIdx = len(vt.history)
		vt.savedInput = ""
		return vt.screen.ViewportTop() != beforeTop // full redraw only on scroll
	case '\b':
		vt.selClearLocked()
		cx, cy := vt.screen.CursorPos()
		vt.screen.BackspaceChar()
		vrow := cy - vt.screen.ViewportTop()
		if cx > 0 {
			vt.renderRowFromLocked(vrow, cx-1)
		}
		return false
	case '\t':
		vt.selClearLocked()
		beforeTop := vt.screen.ViewportTop()
		vt.screen.PutChar('\t')
		return vt.screen.ViewportTop() != beforeTop
	case 0x01: // Ctrl+A - Home
		vt.selClearLocked()
		vt.screen.Home()
		return false
	case 0x05: // Ctrl+E - End
		vt.selClearLocked()
		vt.screen.End()
		return false
	case 0x0B: // Ctrl+K - Kill to EOL
		vt.selClearLocked()
		cx, cy := vt.screen.CursorPos()
		vt.screen.ClearLine(cy, cx)
		vrow := cy - vt.screen.ViewportTop()
		vt.renderRowFromLocked(vrow, cx)
		return false
	case 0x0C: // Ctrl+L - Clear screen
		vt.selClearLocked()
		vt.screen.Clear()
		vt.clearScreenLocked()
		vt.inputActive = false
		return false
	case 0x15: // Ctrl+U - Kill to BOL
		vt.selClearLocked()
		cx, cy := vt.screen.CursorPos()
		startCol := 0
		if vt.inputActive && cy == vt.inputStartRow {
			startCol = vt.inputStartCol
		}
		if cx > startCol {
			vt.screen.KillToStart(cy, startCol)
			vrow := cy - vt.screen.ViewportTop()
			vt.renderRowFromLocked(vrow, startCol)
		}
		return false
	default:
		if b < 0x20 {
			return false
		}
		vt.selClearLocked()
		beforeTop := vt.screen.ViewportTop()
		beforeX, beforeY := vt.screen.CursorPos()
		scrolled := vt.screen.InsertChar(b)
		if scrolled || vt.screen.ViewportTop() != beforeTop {
			return true // full redraw on scroll
		}
		vrow := beforeY - vt.screen.ViewportTop()
		if vrow >= 0 && vrow < vt.rows {
			vt.renderRowFromLocked(vrow, beforeX)
		}
		return false
	}
}

func (vt *VideoTerminal) historyPrevLocked() {
	if !vt.inputActive || len(vt.history) == 0 {
		return
	}
	_, cy := vt.screen.CursorPos()
	if cy != vt.inputStartRow {
		return
	}
	if vt.historyIdx == len(vt.history) {
		// Save current input
		line := vt.screen.ReadLine(vt.inputStartRow)
		if len(line) >= vt.inputStartCol {
			vt.savedInput = line[vt.inputStartCol:]
		} else {
			vt.savedInput = ""
		}
	}
	if vt.historyIdx <= 0 {
		return
	}
	vt.historyIdx--
	vt.screen.ReplaceLine(vt.inputStartRow, vt.inputStartCol, vt.history[vt.historyIdx])
	vrow := vt.inputStartRow - vt.screen.ViewportTop()
	vt.renderRowFromLocked(vrow, vt.inputStartCol)
}

func (vt *VideoTerminal) historyNextLocked() {
	if !vt.inputActive || len(vt.history) == 0 {
		return
	}
	_, cy := vt.screen.CursorPos()
	if cy != vt.inputStartRow {
		return
	}
	if vt.historyIdx >= len(vt.history) {
		return
	}
	vt.historyIdx++
	var text string
	if vt.historyIdx >= len(vt.history) {
		text = vt.savedInput
	} else {
		text = vt.history[vt.historyIdx]
	}
	vt.screen.ReplaceLine(vt.inputStartRow, vt.inputStartCol, text)
	vrow := vt.inputStartRow - vt.screen.ViewportTop()
	vt.renderRowFromLocked(vrow, vt.inputStartCol)
}

func (vt *VideoTerminal) SetClipboardHandlers(write func([]byte), read func() []byte) {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	vt.clipboardWrite = write
	vt.clipboardRead = read
}

func (vt *VideoTerminal) isInSelectionLocked(col, absRow int) bool {
	if !vt.selActive {
		return false
	}
	startCol, startRow := vt.selAnchorCol, vt.selAnchorRow
	endCol, endRow := vt.selEndCol, vt.selEndRow
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startCol, startRow, endCol, endRow = endCol, endRow, startCol, startRow
	}
	if absRow < startRow || absRow > endRow {
		return false
	}
	if startRow == endRow {
		return col >= startCol && col <= endCol
	}
	if absRow == startRow {
		return col >= startCol
	}
	if absRow == endRow {
		return col <= endCol
	}
	return true
}

func (vt *VideoTerminal) selExtendToLocked(col, absRow int) {
	vt.selEndCol = col
	vt.selEndRow = absRow
	vt.selActive = true
	// Auto-copy to internal buffer and OS clipboard
	vt.selText = vt.screen.ExtractText(vt.selAnchorCol, vt.selAnchorRow, vt.selEndCol, vt.selEndRow)
	if vt.clipboardWrite != nil && vt.selText != "" {
		vt.clipboardWrite([]byte(vt.selText))
	}
	vt.renderViewportLocked()
}

func (vt *VideoTerminal) selClearLocked() {
	if !vt.selActive {
		return
	}
	vt.selActive = false
	vt.selText = ""
	vt.renderViewportLocked()
}

func (vt *VideoTerminal) CopySelection() {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	if !vt.selActive {
		return
	}
	vt.selText = vt.screen.ExtractText(vt.selAnchorCol, vt.selAnchorRow, vt.selEndCol, vt.selEndRow)
	if vt.clipboardWrite != nil && vt.selText != "" {
		vt.clipboardWrite([]byte(vt.selText))
	}
}

func (vt *VideoTerminal) CutSelection() {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	if !vt.selActive {
		return
	}
	// Copy first
	vt.selText = vt.screen.ExtractText(vt.selAnchorCol, vt.selAnchorRow, vt.selEndCol, vt.selEndRow)
	if vt.clipboardWrite != nil && vt.selText != "" {
		vt.clipboardWrite([]byte(vt.selText))
	}

	// Normalize selection
	startCol, startRow := vt.selAnchorCol, vt.selAnchorRow
	endCol, endRow := vt.selEndCol, vt.selEndRow
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startCol, startRow, endCol, endRow = endCol, endRow, startCol, startRow
	}

	// Delete selected text from each row in the selection range
	for row := startRow; row <= endRow; row++ {
		if row < 0 || row >= len(vt.screen.lines) {
			continue
		}
		line := vt.screen.lines[row]

		// Compute deletion range for this row
		delStart := 0
		if row == startRow {
			delStart = startCol
		}
		delEnd := vt.cols - 1
		if row == endRow {
			delEnd = endCol
		}

		// For the input row, protect the prompt prefix
		if vt.inputActive && row == vt.inputStartRow && delStart < vt.inputStartCol {
			delStart = vt.inputStartCol
		}

		// Clamp delEnd to actual content (don't shift null padding)
		contentEnd := -1
		for i := vt.cols - 1; i >= 0; i-- {
			if line[i] != 0 {
				contentEnd = i
				break
			}
		}
		if delEnd > contentEnd {
			delEnd = contentEnd
		}
		if delStart > delEnd || delStart < 0 {
			continue
		}

		// Delete range by shifting left
		count := delEnd - delStart + 1
		copy(line[delStart:], line[delEnd+1:])
		for i := vt.cols - count; i < vt.cols; i++ {
			line[i] = 0
		}

		// Update cursor if this is the input row
		if vt.inputActive && row == vt.inputStartRow {
			vt.screen.cursorX = delStart
		}
	}

	vt.selClearLocked()
}

func (vt *VideoTerminal) MiddleMousePaste() {
	vt.mu.Lock()
	var data []byte
	if vt.selActive && vt.selText != "" {
		data = []byte(vt.selText)
	} else if vt.clipboardRead != nil {
		data = vt.clipboardRead()
	}
	vt.mu.Unlock()

	if len(data) == 0 {
		return
	}
	data = normalizePasteText(data)
	data = capPasteText(data, 4096)
	// Route through the keyboard input path so characters appear on screen
	// (same path as Ctrl+Shift+V paste via emitByte → HandleKeyInput).
	for _, b := range data {
		vt.HandleKeyInput(b)
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
			vt.selClearLocked()
			vt.screen.MoveCursor(0, -1)
		case 'B':
			vt.selClearLocked()
			vt.screen.MoveCursor(0, 1)
		case 'C':
			vt.selClearLocked()
			vt.screen.MoveCursor(1, 0)
		case 'D':
			vt.selClearLocked()
			vt.screen.MoveCursor(-1, 0)
		case 'H':
			vt.selClearLocked()
			vt.screen.Home()
		case 'F':
			vt.selClearLocked()
			vt.screen.End()
		default:
			// Digit starts a parameterized sequence (e.g. ESC[1;2D for Shift+Arrow).
			// Do NOT clear selection here — this is an intermediate parse step.
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
		if b == '~' {
			vt.selClearLocked()
			switch vt.inputEscParam {
			case '3':
				cx, cy := vt.screen.CursorPos()
				vt.screen.DeleteChar()
				vrow := cy - vt.screen.ViewportTop()
				vt.renderRowFromLocked(vrow, cx)
			case '5':
				vt.screen.ScrollViewport(-vt.rows)
				vt.renderViewportLocked()
			case '6':
				vt.screen.ScrollViewport(vt.rows)
				vt.renderViewportLocked()
			}
		} else if b == ';' && vt.inputEscParam == '1' {
			// Semicolon continues to modifier parameter (e.g. ESC[1;2D).
			// Do NOT clear selection — this is an intermediate parse step.
			vt.inputEscState = 4
		}
		return true
	case 4:
		vt.inputEscState = 5
		vt.inputEscParam2 = b
		return true
	case 5:
		vt.inputEscState = 0
		if vt.inputEscParam2 == '2' { // Shift modifier — extend selection
			// Set anchor at cursor position BEFORE the move (only on first press)
			if !vt.selActive {
				cx, cy := vt.screen.CursorPos()
				vt.selAnchorCol = cx
				vt.selAnchorRow = cy
			}
			switch b {
			case 'C': // Shift+Right
				vt.screen.MoveCursor(1, 0)
				cx, cy := vt.screen.CursorPos()
				vt.selExtendToLocked(cx, cy)
			case 'D': // Shift+Left
				vt.screen.MoveCursor(-1, 0)
				cx, cy := vt.screen.CursorPos()
				vt.selExtendToLocked(cx, cy)
			case 'A': // Shift+Up
				vt.screen.MoveCursor(0, -1)
				cx, cy := vt.screen.CursorPos()
				vt.selExtendToLocked(cx, cy)
			case 'B': // Shift+Down
				vt.screen.MoveCursor(0, 1)
				cx, cy := vt.screen.CursorPos()
				vt.selExtendToLocked(cx, cy)
			case 'H': // Shift+Home
				vt.screen.Home()
				cx, cy := vt.screen.CursorPos()
				vt.selExtendToLocked(cx, cy)
			case 'F': // Shift+End
				vt.screen.End()
				cx, cy := vt.screen.CursorPos()
				vt.selExtendToLocked(cx, cy)
			}
		} else if vt.inputEscParam2 == '5' { // Ctrl modifier
			vt.selClearLocked()
			switch b {
			case 'C': // Ctrl+Right - word right
				vt.screen.WordRight()
			case 'D': // Ctrl+Left - word left
				vt.screen.WordLeft()
			case 'A': // Ctrl+Up - history previous
				vt.historyPrevLocked()
			case 'B': // Ctrl+Down - history next
				vt.historyNextLocked()
			}
		} else {
			vt.selClearLocked()
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

func normalizePasteText(raw []byte) []byte {
	norm := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\r' {
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
			norm = append(norm, '\n')
			continue
		}
		norm = append(norm, raw[i])
	}
	return norm
}

func capPasteText(raw []byte, max int) []byte {
	if len(raw) <= max {
		return raw
	}
	return raw[:max]
}
