//go:build !headless

// debug_overlay.go - Monitor overlay rendering for Ebiten

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"fmt"
	"image/color"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/intuitionamiga/IntuitionEngine/internal/clipboard"
)

const (
	glyphW = 8
	glyphH = 16
)

var (
	overlayWidth  = DefaultScreenWidth
	overlayHeight = DefaultScreenHeight
	overlayCols   = DefaultOverlayCols
	overlayRows   = DefaultOverlayRows
)

// MonitorOverlay handles rendering and input for the monitor's full-screen overlay.
var (
	monClipboardOnce sync.Once
	monClipboardOK   bool
)

type MonitorOverlay struct {
	monitor *MachineMonitor
	glyphs  [256][16]byte
	image   *ebiten.Image
	pixels  []byte // raw RGBA pixel buffer

	selActive    bool
	selAnchorCol int
	selAnchorRow int
	selEndCol    int
	selEndRow    int
	selText      string
}

// NewMonitorOverlay creates the overlay, loading the Topaz font.
func NewMonitorOverlay(monitor *MachineMonitor) *MonitorOverlay {
	return &MonitorOverlay{
		monitor: monitor,
		glyphs:  loadTopazFont(),
		pixels:  make([]byte, overlayWidth*overlayHeight*4),
	}
}

// colorFromPacked converts a packed 0xRRGGBBAA to RGBA bytes.
func colorFromPacked(c uint32) (byte, byte, byte, byte) {
	return byte(c >> 24), byte(c >> 16), byte(c >> 8), byte(c)
}

// drawGlyph renders a single character onto the pixel buffer.
func (o *MonitorOverlay) drawGlyph(ch byte, col, row int, fg, bg uint32) {
	x := col * glyphW
	y := row * glyphH
	if x+glyphW > overlayWidth || y+glyphH > overlayHeight {
		return
	}

	fgR, fgG, fgB, fgA := colorFromPacked(fg)
	bgR, bgG, bgB, bgA := colorFromPacked(bg)

	glyph := &o.glyphs[ch]
	for dy := range glyphH {
		rowBits := glyph[dy]
		pixY := (y + dy) * overlayWidth * 4
		for dx := range glyphW {
			pixIdx := pixY + (x+dx)*4
			if rowBits&(0x80>>dx) != 0 {
				o.pixels[pixIdx] = fgR
				o.pixels[pixIdx+1] = fgG
				o.pixels[pixIdx+2] = fgB
				o.pixels[pixIdx+3] = fgA
			} else {
				o.pixels[pixIdx] = bgR
				o.pixels[pixIdx+1] = bgG
				o.pixels[pixIdx+2] = bgB
				o.pixels[pixIdx+3] = bgA
			}
		}
	}
}

const (
	monitorKeyRepeatDelay    = 24
	monitorKeyRepeatInterval = 2
)

func monitorShouldRepeat(key ebiten.Key) bool {
	dur := inpututil.KeyPressDuration(key)
	if dur < monitorKeyRepeatDelay {
		return false
	}
	return (dur-monitorKeyRepeatDelay)%monitorKeyRepeatInterval == 0
}

// drawString renders a string at the given column/row with colors.
func (o *MonitorOverlay) drawString(s string, col, row int, fg uint32) {
	bg := uint32(0x0055AAFF) // deep blue background
	for i := 0; i < len(s) && col+i < overlayCols; i++ {
		o.drawGlyph(s[i], col+i, row, fg, bg)
	}
}

// fillRow fills an entire row with the background color.
func (o *MonitorOverlay) fillRow(row int) {
	bg := uint32(0x0055AAFF)
	for col := range overlayCols {
		o.drawGlyph(' ', col, row, colorWhite, bg)
	}
}

// Draw renders the monitor overlay onto the screen.
func (o *MonitorOverlay) Draw(screen *ebiten.Image) {
	m := o.monitor
	m.mu.Lock()
	defer m.mu.Unlock()

	if o.image == nil {
		o.image = ebiten.NewImage(overlayWidth, overlayHeight)
	}

	// Clear to background
	for row := range overlayRows {
		o.fillRow(row)
	}

	if m.state == MonitorHexEdit {
		o.drawHexEditor()
	} else {
		o.drawCommandMode()
	}

	o.image.WritePixels(o.pixels)
	screen.DrawImage(o.image, nil)
}

func (o *MonitorOverlay) drawStringSel(s string, col, row int, fg uint32) {
	bg := uint32(0x0055AAFF)
	for i := 0; i < len(s) && col+i < overlayCols; i++ {
		cfG, cBg := fg, bg
		if o.isInSelection(col+i, row) {
			cfG, cBg = cBg, cfG
		}
		o.drawGlyph(s[i], col+i, row, cfG, cBg)
	}
}

func (o *MonitorOverlay) drawCommandMode() {
	m := o.monitor

	// Header
	entry := m.cpus[m.focusedID]
	header := "MACHINE MONITOR"
	if entry != nil {
		header += "  [" + entry.Label + "]"
	}
	o.drawStringSel(header, 0, 0, colorCyan)

	// CPU tabs on the right
	tabCol := overlayCols - 30
	for _, e := range m.cpus {
		tabColor := uint32(colorDim)
		if e.ID == m.focusedID {
			tabColor = colorWhite
		}
		label := e.Label
		if len(label) > 6 {
			label = label[:6]
		}
		o.drawStringSel(label+" ", tabCol, 0, tabColor)
		tabCol += len(label) + 1
	}

	// Output scrollback
	outputStart := 1
	outputEnd := overlayRows - 2 // leave last row for input
	visibleLines := outputEnd - outputStart

	totalLines := len(m.outputLines)
	startIdx := max(totalLines-visibleLines-m.scrollOffset, 0)

	for row := outputStart; row < outputEnd; row++ {
		idx := startIdx + (row - outputStart)
		if idx >= 0 && idx < totalLines {
			line := m.outputLines[idx]
			o.drawStringSel(line.Text, 0, row, line.Color)
		}
	}

	// Input line
	inputRow := overlayRows - 1
	o.fillRow(inputRow)
	o.drawStringSel("> ", 0, inputRow, colorWhite)
	if len(m.inputLine) > 0 {
		o.drawStringSel(string(m.inputLine), 2, inputRow, colorWhite)
	}
	// Cursor (only show if not in selection at cursor position)
	cursorCol := 2 + m.cursorPos
	if cursorCol < overlayCols && !o.isInSelection(cursorCol, inputRow) {
		o.drawGlyph('_', cursorCol, inputRow, colorWhite, 0x0055AAFF)
	}
}

func (o *MonitorOverlay) drawHexEditor() {
	m := o.monitor
	bg := uint32(0x0055AAFF)

	entry := m.cpus[m.focusedID]
	if entry == nil {
		return
	}

	// Header
	header := fmt.Sprintf("HEX EDITOR - $%06X                     ESC=exit  ENTER=commit", m.hexEditAddr)
	o.drawString(header, 0, 0, colorCyan)

	// 16 rows of 16 bytes = 256 bytes visible
	hexRows := min(16, overlayRows-3)

	for row := range hexRows {
		rowAddr := m.hexEditAddr + uint64(row*16)
		data := entry.CPU.ReadMemory(rowAddr, 16)

		// Address
		addrStr := fmt.Sprintf("%06X:", rowAddr)
		o.drawString(addrStr, 0, row+1, colorDim)

		// Hex bytes
		for col := 0; col < 16 && col < len(data); col++ {
			byteAddr := rowAddr + uint64(col)
			byteOffset := row*16 + col
			val := data[col]

			// Check if this byte has been modified
			if dirtyVal, ok := m.hexEditDirty[byteAddr]; ok {
				val = dirtyVal
			}

			hexStr := fmt.Sprintf("%02X", val)
			xPos := 8 + col*3
			if col >= 8 {
				xPos++ // extra space between groups
			}

			fg := uint32(colorWhite)
			byteBg := bg
			if _, isDirty := m.hexEditDirty[byteAddr]; isDirty {
				fg = colorGreen
			}
			if byteOffset == m.hexEditCursor {
				// Invert for cursor
				fg, byteBg = bg, colorWhite
			}

			for i := range 2 {
				o.drawGlyph(hexStr[i], xPos+i, row+1, fg, byteBg)
			}
		}

		// ASCII column
		asciiStart := 8 + 16*3 + 2
		for col := 0; col < 16 && col < len(data); col++ {
			byteAddr := rowAddr + uint64(col)
			val := data[col]
			if dirtyVal, ok := m.hexEditDirty[byteAddr]; ok {
				val = dirtyVal
			}

			ch := byte('.')
			if val >= 0x20 && val < 0x7F {
				ch = val
			}

			fg := uint32(colorDim)
			if _, isDirty := m.hexEditDirty[byteAddr]; isDirty {
				fg = colorGreen
			}
			o.drawGlyph(ch, asciiStart+col, row+1, fg, bg)
		}
	}
}

// HandleInput processes keyboard input when the monitor is active.
// Returns true if the monitor should deactivate.
func (o *MonitorOverlay) HandleInput() bool {
	m := o.monitor
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state == MonitorHexEdit {
		return o.handleHexEditInput()
	}

	ctrl := ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
	shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)

	// Shift+Arrow/Home/End: extend selection.
	// When Ctrl is also held, only Home/End trigger selection (arrows stay as Ctrl+Arrow word-move).
	shiftHandled := false
	if shift {
		inputRow := overlayRows - 1
		type selKey struct {
			key ebiten.Key
		}
		selKeys := []selKey{
			{ebiten.KeyArrowLeft},
			{ebiten.KeyArrowRight},
			{ebiten.KeyArrowUp},
			{ebiten.KeyArrowDown},
			{ebiten.KeyHome},
			{ebiten.KeyEnd},
		}
		for _, sk := range selKeys {
			// When Ctrl is also held, only handle Home/End for selection
			if ctrl && sk.key != ebiten.KeyHome && sk.key != ebiten.KeyEnd {
				continue
			}
			if inpututil.IsKeyJustPressed(sk.key) || monitorShouldRepeat(sk.key) {
				if !o.selActive {
					// Anchor at current cursor position
					o.selAnchorCol = 2 + m.cursorPos
					o.selAnchorRow = inputRow
					o.selEndCol = o.selAnchorCol
					o.selEndRow = o.selAnchorRow
					o.selActive = true
				}
				col, row := o.selEndCol, o.selEndRow
				switch sk.key {
				case ebiten.KeyArrowLeft:
					col--
				case ebiten.KeyArrowRight:
					col++
				case ebiten.KeyArrowUp:
					row--
				case ebiten.KeyArrowDown:
					row++
				case ebiten.KeyHome:
					col = 0
				case ebiten.KeyEnd:
					col = overlayCols - 1
				}
				o.selExtendTo(col, row)
				o.autoClipboardCopy(m)
				shiftHandled = true
			}
		}
	}

	// Ctrl+Shift+C/X/V
	if ctrl && shift {
		if inpututil.IsKeyJustPressed(ebiten.KeyC) {
			o.handleMonitorCopy(m)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyX) {
			o.handleMonitorCut(m)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyV) {
			o.selClear()
			o.handleMonitorPaste(m)
		}
	}

	// Middle mouse button paste
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle) {
		o.handleMonitorMiddlePaste(m)
	}

	if shiftHandled {
		return false
	}

	// Escape = exit
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		o.selClear()
		m.state = MonitorInactive
		for id, entry := range m.cpus {
			if m.wasRunning[id] {
				entry.CPU.Resume()
			}
		}
		return true
	}

	// PgUp/PgDn for scrolling
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		o.selClear()
		m.scrollOffset += 10
		maxScroll := len(m.outputLines) - (overlayRows - 3)
		if m.scrollOffset > maxScroll {
			m.scrollOffset = maxScroll
		}
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		o.selClear()
		m.scrollOffset -= 10
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
	}

	// Mouse wheel scroll
	_, wy := ebiten.Wheel()
	if wy != 0 {
		o.selClear()
		delta := int(wy)
		if delta == 0 && wy > 0 {
			delta = 1
		}
		if delta == 0 && wy < 0 {
			delta = -1
		}
		m.scrollOffset += delta
		maxScroll := len(m.outputLines) - (overlayRows - 3)
		if m.scrollOffset > maxScroll {
			m.scrollOffset = maxScroll
		}
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
	}

	// Up/Down for command history
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		o.selClear()
		if m.historyIdx > 0 {
			m.historyIdx--
			m.inputLine = []byte(m.history[m.historyIdx])
			m.cursorPos = len(m.inputLine)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		o.selClear()
		if m.historyIdx < len(m.history)-1 {
			m.historyIdx++
			m.inputLine = []byte(m.history[m.historyIdx])
			m.cursorPos = len(m.inputLine)
		} else {
			m.historyIdx = len(m.history)
			m.inputLine = nil
			m.cursorPos = 0
		}
	}

	// Enter = submit command
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		o.selClear()
		input := string(m.inputLine)
		m.appendOutput("> "+input, colorDim)
		m.inputLine = nil
		m.cursorPos = 0
		m.scrollOffset = 0

		if m.executeCommand(input) {
			// Command requested exit
			m.state = MonitorInactive
			for id, entry := range m.cpus {
				if m.wasRunning[id] {
					entry.CPU.Resume()
				}
			}
			return true
		}
		return false
	}

	// Backspace
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) || monitorShouldRepeat(ebiten.KeyBackspace) {
		o.selClear()
		if m.cursorPos > 0 && len(m.inputLine) > 0 {
			m.inputLine = append(m.inputLine[:m.cursorPos-1], m.inputLine[m.cursorPos:]...)
			m.cursorPos--
		}
	}

	// Delete
	if inpututil.IsKeyJustPressed(ebiten.KeyDelete) || monitorShouldRepeat(ebiten.KeyDelete) {
		o.selClear()
		if m.cursorPos < len(m.inputLine) {
			m.inputLine = append(m.inputLine[:m.cursorPos], m.inputLine[m.cursorPos+1:]...)
		}
	}

	// Home / End
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) || monitorShouldRepeat(ebiten.KeyHome) {
		o.selClear()
		m.cursorPos = 0
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) || monitorShouldRepeat(ebiten.KeyEnd) {
		o.selClear()
		m.cursorPos = len(m.inputLine)
	}

	if ctrl && !shift {
		o.selClear()
		// Ctrl+A = Home
		if inpututil.IsKeyJustPressed(ebiten.KeyA) {
			m.cursorPos = 0
		}
		// Ctrl+E = End
		if inpututil.IsKeyJustPressed(ebiten.KeyE) {
			m.cursorPos = len(m.inputLine)
		}
		// Ctrl+K = Kill to EOL
		if inpututil.IsKeyJustPressed(ebiten.KeyK) {
			m.inputLine = m.inputLine[:m.cursorPos]
		}
		// Ctrl+U = Kill to BOL
		if inpututil.IsKeyJustPressed(ebiten.KeyU) {
			m.inputLine = m.inputLine[m.cursorPos:]
			m.cursorPos = 0
		}
		// Ctrl+Left = Word left
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || monitorShouldRepeat(ebiten.KeyArrowLeft) {
			for m.cursorPos > 0 && m.inputLine[m.cursorPos-1] == ' ' {
				m.cursorPos--
			}
			for m.cursorPos > 0 && m.inputLine[m.cursorPos-1] != ' ' {
				m.cursorPos--
			}
		}
		// Ctrl+Right = Word right
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || monitorShouldRepeat(ebiten.KeyArrowRight) {
			for m.cursorPos < len(m.inputLine) && m.inputLine[m.cursorPos] != ' ' {
				m.cursorPos++
			}
			for m.cursorPos < len(m.inputLine) && m.inputLine[m.cursorPos] == ' ' {
				m.cursorPos++
			}
		}
		ebiten.AppendInputChars(nil) // drain to prevent ctrl+key inserting chars
	} else if !ctrl {
		// Plain Left/Right arrows
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || monitorShouldRepeat(ebiten.KeyArrowLeft) {
			o.selClear()
			if m.cursorPos > 0 {
				m.cursorPos--
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || monitorShouldRepeat(ebiten.KeyArrowRight) {
			o.selClear()
			if m.cursorPos < len(m.inputLine) {
				m.cursorPos++
			}
		}

		// Printable character input
		for _, r := range ebiten.AppendInputChars(nil) {
			o.selClear()
			if r >= 0x20 && r < 0x7F {
				ch := byte(r)
				if len(m.inputLine) < overlayCols-4 {
					m.inputLine = append(m.inputLine, 0)
					copy(m.inputLine[m.cursorPos+1:], m.inputLine[m.cursorPos:])
					m.inputLine[m.cursorPos] = ch
					m.cursorPos++
				}
			}
		}
	}

	return false
}

// handleMonitorPaste reads the system clipboard and inserts printable ASCII
// into the monitor input line at the cursor position. Caller must hold m.mu.
func (o *MonitorOverlay) handleMonitorPaste(m *MachineMonitor) {
	monClipboardOnce.Do(func() {
		monClipboardOK = clipboard.Init() == nil
	})
	if !monClipboardOK {
		return
	}
	data, _ := clipboard.ReadText()
	if len(data) == 0 {
		return
	}
	for _, b := range data {
		if b >= 0x20 && b < 0x7F {
			if len(m.inputLine) < overlayCols-4 {
				m.inputLine = append(m.inputLine, 0)
				copy(m.inputLine[m.cursorPos+1:], m.inputLine[m.cursorPos:])
				m.inputLine[m.cursorPos] = b
				m.cursorPos++
			}
		}
	}
}

// colorToRGBA converts packed RGBA to color.RGBA (unused but available).
func colorToRGBA(c uint32) color.RGBA {
	return color.RGBA{R: byte(c >> 24), G: byte(c >> 16), B: byte(c >> 8), A: byte(c)}
}

func (o *MonitorOverlay) isInSelection(col, row int) bool {
	if !o.selActive {
		return false
	}
	startCol, startRow := o.selAnchorCol, o.selAnchorRow
	endCol, endRow := o.selEndCol, o.selEndRow
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startCol, startRow, endCol, endRow = endCol, endRow, startCol, startRow
	}
	if row < startRow || row > endRow {
		return false
	}
	if startRow == endRow {
		return col >= startCol && col <= endCol
	}
	if row == startRow {
		return col >= startCol
	}
	if row == endRow {
		return col <= endCol
	}
	return true
}

func (o *MonitorOverlay) selClear() {
	if !o.selActive {
		return
	}
	o.selActive = false
	o.selText = ""
}

func (o *MonitorOverlay) selExtendTo(col, row int) {
	if col < 0 {
		col = 0
	}
	if col >= overlayCols {
		col = overlayCols - 1
	}
	if row < 0 {
		row = 0
	}
	if row >= overlayRows {
		row = overlayRows - 1
	}
	o.selEndCol = col
	o.selEndRow = row
	o.selActive = true
}

func (o *MonitorOverlay) monitorExtractText(m *MachineMonitor) string {
	if !o.selActive {
		return ""
	}
	startCol, startRow := o.selAnchorCol, o.selAnchorRow
	endCol, endRow := o.selEndCol, o.selEndRow
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startCol, startRow, endCol, endRow = endCol, endRow, startCol, startRow
	}
	if startRow < 0 {
		startRow = 0
	}
	if endRow >= overlayRows {
		endRow = overlayRows - 1
	}

	getRowText := func(row int) string {
		if row == 0 {
			entry := m.cpus[m.focusedID]
			header := "MACHINE MONITOR"
			if entry != nil {
				header += "  [" + entry.Label + "]"
			}
			for len(header) < overlayCols {
				header += " "
			}
			return header
		}
		inputRow := overlayRows - 1
		if row == inputRow {
			line := "> " + string(m.inputLine)
			for len(line) < overlayCols {
				line += " "
			}
			return line
		}
		// Output rows: 1 to overlayRows-2
		outputStart := 1
		outputEnd := overlayRows - 2
		visibleLines := outputEnd - outputStart
		totalLines := len(m.outputLines)
		startIdx := max(totalLines-visibleLines-m.scrollOffset, 0)
		idx := startIdx + (row - 1)
		if idx >= 0 && idx < totalLines {
			line := m.outputLines[idx].Text
			for len(line) < overlayCols {
				line += " "
			}
			return line
		}
		return ""
	}

	trimRight := func(s string) string {
		end := len(s)
		for end > 0 && (s[end-1] == ' ' || s[end-1] == 0) {
			end--
		}
		return s[:end]
	}

	if startRow == endRow {
		line := getRowText(startRow)
		ec := min(endCol+1, len(line))
		sc := min(startCol, len(line))
		if sc > ec {
			return ""
		}
		return trimRight(line[sc:ec])
	}

	var parts []string
	for r := startRow; r <= endRow; r++ {
		line := getRowText(r)
		sc := 0
		ec := len(line)
		if r == startRow {
			sc = startCol
		}
		if r == endRow {
			ec = endCol + 1
		}
		if sc > len(line) {
			sc = len(line)
		}
		if ec > len(line) {
			ec = len(line)
		}
		if sc > ec {
			parts = append(parts, "")
		} else {
			parts = append(parts, trimRight(line[sc:ec]))
		}
	}
	var result strings.Builder
	result.WriteString(parts[0])
	for i := 1; i < len(parts); i++ {
		result.WriteString("\n" + parts[i])
	}
	return result.String()
}

func (o *MonitorOverlay) autoClipboardCopy(m *MachineMonitor) {
	o.selText = o.monitorExtractText(m)
	monClipboardOnce.Do(func() { monClipboardOK = clipboard.Init() == nil })
	if monClipboardOK && o.selText != "" {
		_ = clipboard.WriteText([]byte(o.selText))
	}
}

func (o *MonitorOverlay) handleMonitorCopy(m *MachineMonitor) {
	if !o.selActive {
		return
	}
	o.selText = o.monitorExtractText(m)
	monClipboardOnce.Do(func() { monClipboardOK = clipboard.Init() == nil })
	if monClipboardOK && o.selText != "" {
		_ = clipboard.WriteText([]byte(o.selText))
	}
}

func (o *MonitorOverlay) handleMonitorCut(m *MachineMonitor) {
	if !o.selActive {
		return
	}
	o.handleMonitorCopy(m)

	startCol, startRow := o.selAnchorCol, o.selAnchorRow
	endCol, endRow := o.selEndCol, o.selEndRow
	if startRow > endRow || (startRow == endRow && startCol > endCol) {
		startCol, startRow, endCol, endRow = endCol, endRow, startCol, startRow
	}

	inputRow := overlayRows - 1
	outputStart := 1
	outputEnd := overlayRows - 2
	visibleLines := outputEnd - outputStart
	totalLines := len(m.outputLines)
	baseIdx := max(totalLines-visibleLines-m.scrollOffset, 0)

	for row := startRow; row <= endRow; row++ {
		delStart := 0
		if row == startRow {
			delStart = startCol
		}
		delEnd := overlayCols - 1
		if row == endRow {
			delEnd = endCol
		}

		if row == 0 {
			// Header row — skip
			continue
		} else if row >= outputStart && row < outputEnd {
			// Output row — modify outputLines text
			idx := baseIdx + (row - outputStart)
			if idx >= 0 && idx < totalLines {
				line := m.outputLines[idx].Text
				for len(line) < overlayCols {
					line += " "
				}
				if delStart >= 0 && delEnd < len(line) && delStart <= delEnd {
					line = line[:delStart] + line[delEnd+1:]
				}
				// Trim trailing spaces
				end := len(line)
				for end > 0 && line[end-1] == ' ' {
					end--
				}
				m.outputLines[idx].Text = line[:end]
			}
		} else if row == inputRow {
			// Input line: protect "> " prompt prefix
			if delStart < 2 {
				delStart = 2
			}
			idxStart := delStart - 2
			idxEnd := delEnd - 2
			if idxEnd >= len(m.inputLine) {
				idxEnd = len(m.inputLine) - 1
			}
			if idxStart <= idxEnd && idxStart < len(m.inputLine) {
				m.inputLine = append(m.inputLine[:idxStart], m.inputLine[idxEnd+1:]...)
				m.cursorPos = idxStart
			}
		}
	}

	o.selClear()
}

func (o *MonitorOverlay) handleMonitorMiddlePaste(m *MachineMonitor) {
	if o.selActive && o.selText != "" {
		// Paste from internal selection buffer
		for _, b := range []byte(o.selText) {
			if b >= 0x20 && b < 0x7F {
				if len(m.inputLine) < overlayCols-4 {
					m.inputLine = append(m.inputLine, 0)
					copy(m.inputLine[m.cursorPos+1:], m.inputLine[m.cursorPos:])
					m.inputLine[m.cursorPos] = b
					m.cursorPos++
				}
			}
		}
		return
	}
	// Try X11 PRIMARY selection first (text highlighted by mouse in other apps)
	if primary := readPrimarySelection(); len(primary) > 0 {
		for _, b := range primary {
			if b >= 0x20 && b < 0x7F {
				if len(m.inputLine) < overlayCols-4 {
					m.inputLine = append(m.inputLine, 0)
					copy(m.inputLine[m.cursorPos+1:], m.inputLine[m.cursorPos:])
					m.inputLine[m.cursorPos] = b
					m.cursorPos++
				}
			}
		}
		return
	}
	// Fall back to OS CLIPBOARD
	o.handleMonitorPaste(m)
}

func (o *MonitorOverlay) handleHexEditInput() bool {
	m := o.monitor

	// Escape = discard and return to command mode
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		m.HexEditDiscard()
		return false
	}

	// Enter = commit and return to command mode
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		m.HexEditCommit()
		return false
	}

	// Arrow keys
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || monitorShouldRepeat(ebiten.KeyArrowLeft) {
		if m.hexEditCursor > 0 {
			m.hexEditCursor--
			m.hexEditNibble = 0
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || monitorShouldRepeat(ebiten.KeyArrowRight) {
		if m.hexEditCursor < 255 {
			m.hexEditCursor++
			m.hexEditNibble = 0
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) || monitorShouldRepeat(ebiten.KeyArrowUp) {
		if m.hexEditCursor >= 16 {
			m.hexEditCursor -= 16
			m.hexEditNibble = 0
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) || monitorShouldRepeat(ebiten.KeyArrowDown) {
		if m.hexEditCursor < 240 {
			m.hexEditCursor += 16
			m.hexEditNibble = 0
		}
	}

	// PgUp/PgDn scroll by 256 bytes
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		if m.hexEditAddr >= 256 {
			m.hexEditAddr -= 256
		} else {
			m.hexEditAddr = 0
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		m.hexEditAddr += 256
	}

	// Hex digit input (0-9, A-F)
	for _, r := range ebiten.AppendInputChars(nil) {
		var nibbleVal byte
		var valid bool
		switch {
		case r >= '0' && r <= '9':
			nibbleVal = byte(r - '0')
			valid = true
		case r >= 'a' && r <= 'f':
			nibbleVal = byte(r-'a') + 10
			valid = true
		case r >= 'A' && r <= 'F':
			nibbleVal = byte(r-'A') + 10
			valid = true
		}
		if valid {
			m.HexEditSetNibble(nibbleVal)
		}
	}

	return false
}
