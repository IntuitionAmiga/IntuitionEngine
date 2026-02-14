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

	o.image.WritePixels(o.pixels)
	screen.DrawImage(o.image, nil)
}

// HandleInput processes keyboard input when the monitor is active.
// Returns true if the monitor should deactivate.
func (o *MonitorOverlay) HandleInput() bool {
	m := o.monitor
	m.mu.Lock()
	defer m.mu.Unlock()

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
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		if m.cursorPos > 0 && len(m.inputLine) > 0 {
			m.inputLine = append(m.inputLine[:m.cursorPos-1], m.inputLine[m.cursorPos:]...)
			m.cursorPos--
		}
	}

	// Left/Right arrows
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) {
		if m.cursorPos > 0 {
			m.cursorPos--
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) {
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

	return false
}

// colorToRGBA converts packed RGBA to color.RGBA (unused but available).
func colorToRGBA(c uint32) color.RGBA {
	return color.RGBA{R: byte(c >> 24), G: byte(c >> 16), B: byte(c >> 8), A: byte(c)}
}
