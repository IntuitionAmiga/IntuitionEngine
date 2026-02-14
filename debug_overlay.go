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

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	overlayWidth  = 640
	overlayHeight = 480
	overlayCols   = 80
	overlayRows   = 30
	glyphW        = 8
	glyphH        = 16
)

// MonitorOverlay handles rendering and input for the monitor's full-screen overlay.
type MonitorOverlay struct {
	monitor *MachineMonitor
	glyphs  [256][16]byte
	image   *ebiten.Image
	pixels  []byte // raw RGBA pixel buffer
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

func (o *MonitorOverlay) drawCommandMode() {
	m := o.monitor

	// Header
	entry := m.cpus[m.focusedID]
	header := "MACHINE MONITOR"
	if entry != nil {
		header += "  [" + entry.Label + "]"
	}
	o.drawString(header, 0, 0, colorCyan)

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
		o.drawString(label+" ", tabCol, 0, tabColor)
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
			o.drawString(line.Text, 0, row, line.Color)
		}
	}

	// Input line
	inputRow := overlayRows - 1
	o.fillRow(inputRow)
	o.drawString("> ", 0, inputRow, colorWhite)
	if len(m.inputLine) > 0 {
		o.drawString(string(m.inputLine), 2, inputRow, colorWhite)
	}
	// Cursor
	cursorCol := 2 + m.cursorPos
	if cursorCol < overlayCols {
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

	for row := 0; row < hexRows; row++ {
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

	// Escape = exit
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
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
		m.scrollOffset -= 10
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
	}

	// Mouse wheel scroll
	_, wy := ebiten.Wheel()
	if wy != 0 {
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
		if m.historyIdx > 0 {
			m.historyIdx--
			m.inputLine = []byte(m.history[m.historyIdx])
			m.cursorPos = len(m.inputLine)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
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
		input := string(m.inputLine)
		m.appendOutput("> "+input, colorDim)
		m.inputLine = nil
		m.cursorPos = 0
		m.scrollOffset = 0

		if m.ExecuteCommand(input) {
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
		if m.cursorPos > 0 && len(m.inputLine) > 0 {
			m.inputLine = append(m.inputLine[:m.cursorPos-1], m.inputLine[m.cursorPos:]...)
			m.cursorPos--
		}
	}

	// Delete
	if inpututil.IsKeyJustPressed(ebiten.KeyDelete) || monitorShouldRepeat(ebiten.KeyDelete) {
		if m.cursorPos < len(m.inputLine) {
			m.inputLine = append(m.inputLine[:m.cursorPos], m.inputLine[m.cursorPos+1:]...)
		}
	}

	// Home / End
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) || monitorShouldRepeat(ebiten.KeyHome) {
		m.cursorPos = 0
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) || monitorShouldRepeat(ebiten.KeyEnd) {
		m.cursorPos = len(m.inputLine)
	}

	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl)
	if ctrl {
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
	} else {
		// Plain Left/Right arrows
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || monitorShouldRepeat(ebiten.KeyArrowLeft) {
			if m.cursorPos > 0 {
				m.cursorPos--
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || monitorShouldRepeat(ebiten.KeyArrowRight) {
			if m.cursorPos < len(m.inputLine) {
				m.cursorPos++
			}
		}

		// Printable character input
		for _, r := range ebiten.AppendInputChars(nil) {
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

// colorToRGBA converts packed RGBA to color.RGBA (unused but available).
func colorToRGBA(c uint32) color.RGBA {
	return color.RGBA{R: byte(c >> 24), G: byte(c >> 16), B: byte(c >> 8), A: byte(c)}
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
