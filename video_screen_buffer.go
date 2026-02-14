package main

type ScreenBuffer struct {
	cols, visibleRows, maxLines int
	lines                       [][]byte
	viewportTop                 int
	cursorX                     int
	cursorY                     int
}

func NewScreenBuffer(cols, visibleRows, maxLines int) *ScreenBuffer {
	if cols <= 0 {
		cols = 1
	}
	if visibleRows <= 0 {
		visibleRows = 1
	}
	if maxLines < visibleRows {
		maxLines = visibleRows
	}
	sb := &ScreenBuffer{
		cols:        cols,
		visibleRows: visibleRows,
		maxLines:    maxLines,
		lines:       make([][]byte, visibleRows),
	}
	for i := range sb.lines {
		sb.lines[i] = make([]byte, cols)
	}
	return sb
}

func (sb *ScreenBuffer) PutChar(ch byte) (scrolled bool) {
	switch ch {
	case '\r':
		sb.cursorX = 0
		return false
	case '\n':
		beforeTop := sb.viewportTop
		sb.cursorY++
		sb.ensureLine(sb.cursorY)
		sb.EnsureCursorVisible()
		return sb.viewportTop != beforeTop
	case '\b':
		if sb.cursorX > 0 {
			sb.cursorX--
		}
		return false
	case '\t':
		next := (sb.cursorX + tabWidth) &^ (tabWidth - 1)
		if next >= sb.cols {
			sb.cursorX = 0
			sb.cursorY++
			beforeTop := sb.viewportTop
			sb.ensureLine(sb.cursorY)
			sb.EnsureCursorVisible()
			return sb.viewportTop != beforeTop
		}
		sb.cursorX = next
		return false
	case '\f':
		sb.Clear()
		return true
	default:
		if ch < 0x20 {
			return false
		}
		sb.ensureLine(sb.cursorY)
		sb.lines[sb.cursorY][sb.cursorX] = ch
		sb.cursorX++
		if sb.cursorX >= sb.cols {
			sb.cursorX = 0
			sb.cursorY++
			beforeTop := sb.viewportTop
			sb.ensureLine(sb.cursorY)
			sb.EnsureCursorVisible()
			return sb.viewportTop != beforeTop
		}
		return false
	}
}

func (sb *ScreenBuffer) MoveCursor(dx, dy int) {
	for dx > 0 {
		if sb.cursorX < sb.cols-1 {
			sb.cursorX++
		} else if sb.cursorY < len(sb.lines)-1 {
			sb.cursorX = 0
			sb.cursorY++
		}
		dx--
	}
	for dx < 0 {
		if sb.cursorX > 0 {
			sb.cursorX--
		} else if sb.cursorY > 0 {
			sb.cursorY--
			sb.cursorX = sb.cols - 1
		}
		dx++
	}
	for dy > 0 {
		if sb.cursorY < len(sb.lines)-1 {
			sb.cursorY++
		}
		dy--
	}
	for dy < 0 {
		if sb.cursorY > 0 {
			sb.cursorY--
		}
		dy++
	}
	sb.EnsureCursorVisible()
}

func (sb *ScreenBuffer) Home() {
	sb.cursorX = 0
}

func (sb *ScreenBuffer) End() {
	sb.ensureLine(sb.cursorY)
	last := -1
	line := sb.lines[sb.cursorY]
	for i := len(line) - 1; i >= 0; i-- {
		if line[i] != 0 {
			last = i
			break
		}
	}
	if last < 0 {
		sb.cursorX = 0
		return
	}
	if last+1 >= sb.cols {
		sb.cursorX = sb.cols - 1
		return
	}
	sb.cursorX = last + 1
}

func (sb *ScreenBuffer) BackspaceChar() {
	if sb.cursorX == 0 {
		return
	}
	sb.ensureLine(sb.cursorY)
	line := sb.lines[sb.cursorY]
	del := sb.cursorX - 1
	copy(line[del:], line[del+1:])
	line[sb.cols-1] = 0
	sb.cursorX--
}

func (sb *ScreenBuffer) InsertChar(ch byte) bool {
	if ch < 0x20 {
		return sb.PutChar(ch)
	}
	sb.ensureLine(sb.cursorY)
	line := sb.lines[sb.cursorY]
	copy(line[sb.cursorX+1:], line[sb.cursorX:sb.cols-1])
	line[sb.cursorX] = ch
	sb.cursorX++
	if sb.cursorX >= sb.cols {
		sb.cursorX = 0
		sb.cursorY++
		beforeTop := sb.viewportTop
		sb.ensureLine(sb.cursorY)
		sb.EnsureCursorVisible()
		return sb.viewportTop != beforeTop
	}
	return false
}

func (sb *ScreenBuffer) DeleteChar() {
	if sb.cursorX < 0 || sb.cursorX >= sb.cols {
		return
	}
	sb.ensureLine(sb.cursorY)
	line := sb.lines[sb.cursorY]
	copy(line[sb.cursorX:], line[sb.cursorX+1:])
	line[sb.cols-1] = 0
}

func (sb *ScreenBuffer) WordLeft() {
	sb.ensureLine(sb.cursorY)
	line := sb.lines[sb.cursorY]
	x := sb.cursorX
	// Skip spaces left
	for x > 0 && (line[x-1] == ' ' || line[x-1] == 0) {
		x--
	}
	// Skip word chars left
	for x > 0 && line[x-1] != ' ' && line[x-1] != 0 {
		x--
	}
	sb.cursorX = x
}

func (sb *ScreenBuffer) WordRight() {
	sb.ensureLine(sb.cursorY)
	line := sb.lines[sb.cursorY]
	x := sb.cursorX
	// Skip word chars right
	for x < sb.cols && line[x] != ' ' && line[x] != 0 {
		x++
	}
	// Skip spaces right
	for x < sb.cols && (line[x] == ' ') {
		x++
	}
	sb.cursorX = x
}

func (sb *ScreenBuffer) ReplaceLine(absRow, fromCol int, text string) {
	if absRow < 0 || absRow >= len(sb.lines) {
		return
	}
	line := sb.lines[absRow]
	for i := fromCol; i < sb.cols; i++ {
		line[i] = 0
	}
	for i, ch := range []byte(text) {
		pos := fromCol + i
		if pos >= sb.cols {
			break
		}
		line[pos] = ch
	}
	sb.cursorX = fromCol + len(text)
	if sb.cursorX >= sb.cols {
		sb.cursorX = sb.cols - 1
	}
}

func (sb *ScreenBuffer) ClearLine(absRow, fromCol int) {
	if absRow < 0 || absRow >= len(sb.lines) {
		return
	}
	line := sb.lines[absRow]
	for i := fromCol; i < sb.cols; i++ {
		line[i] = 0
	}
}

func (sb *ScreenBuffer) KillToStart(absRow, fromCol int) {
	if absRow < 0 || absRow >= len(sb.lines) {
		return
	}
	line := sb.lines[absRow]
	n := copy(line[fromCol:], line[sb.cursorX:])
	for i := fromCol + n; i < sb.cols; i++ {
		line[i] = 0
	}
	sb.cursorX = fromCol
}

func (sb *ScreenBuffer) ScrollUp() {
	sb.lines = append(sb.lines, make([]byte, sb.cols))
	sb.viewportTop++
	sb.trimToMaxLines()
	maxTop := sb.maxViewportTop()
	if sb.viewportTop > maxTop {
		sb.viewportTop = maxTop
	}
	if sb.cursorY < sb.viewportTop {
		sb.cursorY = sb.viewportTop
	}
}

func (sb *ScreenBuffer) ReadLine(absRow int) string {
	if absRow < 0 || absRow >= len(sb.lines) {
		return ""
	}
	line := sb.lines[absRow]
	end := len(line)
	for end > 0 {
		ch := line[end-1]
		if ch != 0 && ch != ' ' {
			break
		}
		end--
	}
	return string(line[:end])
}

func (sb *ScreenBuffer) GetCell(col, absRow int) byte {
	if col < 0 || col >= sb.cols || absRow < 0 || absRow >= len(sb.lines) {
		return 0
	}
	return sb.lines[absRow][col]
}

func (sb *ScreenBuffer) SetCell(col, absRow int, ch byte) {
	if col < 0 || col >= sb.cols || absRow < 0 {
		return
	}
	sb.ensureLine(absRow)
	sb.lines[absRow][col] = ch
}

func (sb *ScreenBuffer) VisibleCell(col, vrow int) byte {
	absRow := sb.viewportTop + vrow
	return sb.GetCell(col, absRow)
}

func (sb *ScreenBuffer) CursorPos() (col, absRow int) {
	return sb.cursorX, sb.cursorY
}

func (sb *ScreenBuffer) CursorViewportPos() (col, vrow int) {
	return sb.cursorX, sb.cursorY - sb.viewportTop
}

func (sb *ScreenBuffer) Clear() {
	sb.lines = make([][]byte, sb.visibleRows)
	for i := range sb.lines {
		sb.lines[i] = make([]byte, sb.cols)
	}
	sb.viewportTop = 0
	sb.cursorX = 0
	sb.cursorY = 0
}

func (sb *ScreenBuffer) EnsureCursorVisible() {
	if sb.cursorY < sb.viewportTop {
		sb.viewportTop = sb.cursorY
	}
	if sb.cursorY >= sb.viewportTop+sb.visibleRows {
		sb.viewportTop = sb.cursorY - sb.visibleRows + 1
	}
	if sb.viewportTop < 0 {
		sb.viewportTop = 0
	}
	maxTop := sb.maxViewportTop()
	if sb.viewportTop > maxTop {
		sb.viewportTop = maxTop
	}
}

func (sb *ScreenBuffer) ScrollViewport(delta int) {
	sb.viewportTop += delta
	if sb.viewportTop < 0 {
		sb.viewportTop = 0
	}
	maxTop := sb.maxViewportTop()
	if sb.viewportTop > maxTop {
		sb.viewportTop = maxTop
	}
}

func (sb *ScreenBuffer) ViewportTop() int {
	return sb.viewportTop
}

func (sb *ScreenBuffer) TotalLines() int {
	return len(sb.lines)
}

func (sb *ScreenBuffer) ensureLine(absRow int) {
	for len(sb.lines) <= absRow {
		before := len(sb.lines)
		sb.lines = append(sb.lines, make([]byte, sb.cols))
		sb.trimToMaxLines()
		trimmed := before + 1 - len(sb.lines)
		absRow -= trimmed
	}
}

func (sb *ScreenBuffer) trimToMaxLines() {
	if len(sb.lines) <= sb.maxLines {
		return
	}
	trim := len(sb.lines) - sb.maxLines
	sb.lines = sb.lines[trim:]
	sb.cursorY -= trim
	if sb.cursorY < 0 {
		sb.cursorY = 0
	}
	sb.viewportTop -= trim
	if sb.viewportTop < 0 {
		sb.viewportTop = 0
	}
}

func (sb *ScreenBuffer) maxViewportTop() int {
	if len(sb.lines) <= sb.visibleRows {
		return 0
	}
	return len(sb.lines) - sb.visibleRows
}
