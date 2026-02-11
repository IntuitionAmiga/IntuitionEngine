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
	for g := 0; g < len(glyphs); g++ {
		copy(glyphs[g][:], topazRawFont[offset:offset+terminalGlyphHeight])
		offset += terminalGlyphHeight
	}
	return glyphs
}

type VideoTerminal struct {
	video       *VideoChip
	term        *TerminalMMIO
	mu          sync.Mutex
	textBuf     []byte
	cols        int
	rows        int
	pixelWidth  int
	pixelHeight int
	cursorX     int
	cursorY     int
	cursorOn    bool
	fgColor     uint32
	bgColor     uint32
	glyphs      [256][16]byte

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
		textBuf:     make([]byte, cols*rows),
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

	switch ch {
	case '\r':
		vt.cursorX = 0
	case '\n':
		vt.newLineLocked()
	case '\b':
		if vt.cursorX > 0 {
			vt.cursorX--
			vt.setCellLocked(vt.cursorX, vt.cursorY, 0)
			vt.renderCellLocked(vt.cursorX, vt.cursorY, ' ')
		}
	case '\t':
		next := (vt.cursorX + tabWidth) &^ (tabWidth - 1)
		if next >= vt.cols {
			vt.cursorX = 0
			vt.newLineLocked()
		} else {
			vt.cursorX = next
		}
	case '\f':
		vt.clearScreenLocked()
	default:
		if ch < 0x20 {
			break
		}
		if vt.cursorX >= vt.cols {
			vt.cursorX = 0
			vt.newLineLocked()
		}
		vt.setCellLocked(vt.cursorX, vt.cursorY, ch)
		vt.renderCellLocked(vt.cursorX, vt.cursorY, ch)
		vt.cursorX++
		if vt.cursorX >= vt.cols {
			vt.cursorX = 0
			vt.newLineLocked()
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
	vt.clearScreenLocked()
}

func (vt *VideoTerminal) clearScreenLocked() {
	vt.video.RenderToFrontBuffer(func(fb []byte, _ int) {
		for i := 0; i < len(fb); i += 4 {
			writeColorLE(fb, i, vt.bgColor)
		}
	})
	for i := range vt.textBuf {
		vt.textBuf[i] = 0
	}
	vt.cursorX = 0
	vt.cursorY = 0
	vt.video.MarkRectDirty(0, 0, vt.pixelWidth, vt.pixelHeight)
}

func (vt *VideoTerminal) newLineLocked() {
	vt.cursorY++
	if vt.cursorY >= vt.rows {
		vt.scrollUpLocked()
		vt.cursorY = vt.rows - 1
	}
}

func (vt *VideoTerminal) scrollUpLocked() {
	if vt.rows <= 0 {
		return
	}

	if vt.rows > 1 {
		rowBytes := vt.cols
		copy(vt.textBuf[0:], vt.textBuf[rowBytes:])
	}
	lastRowStart := (vt.rows - 1) * vt.cols
	for i := lastRowStart; i < lastRowStart+vt.cols; i++ {
		vt.textBuf[i] = 0
	}

	scrollPx := terminalGlyphHeight
	vt.video.RenderToFrontBuffer(func(fb []byte, stride int) {
		if scrollPx >= vt.pixelHeight {
			for i := 0; i < len(fb); i += 4 {
				writeColorLE(fb, i, vt.bgColor)
			}
			return
		}

		moveBytes := (vt.pixelHeight - scrollPx) * stride
		copy(fb[0:moveBytes], fb[scrollPx*stride:scrollPx*stride+moveBytes])

		start := (vt.pixelHeight - scrollPx) * stride
		for i := start; i < len(fb); i += 4 {
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
		for gy := 0; gy < terminalGlyphHeight; gy++ {
			rowBits := glyph[gy]
			dst := (baseY+gy)*stride + baseX*4
			for gx := 0; gx < terminalGlyphWidth; gx++ {
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
	if vt.cursorX < 0 || vt.cursorX >= vt.cols || vt.cursorY < 0 || vt.cursorY >= vt.rows {
		return
	}
	if !visible {
		ch := vt.getCellLocked(vt.cursorX, vt.cursorY)
		if ch == 0 {
			ch = ' '
		}
		vt.renderCellLocked(vt.cursorX, vt.cursorY, ch)
		return
	}

	baseX := vt.cursorX * terminalGlyphWidth
	baseY := vt.cursorY * terminalGlyphHeight
	vt.video.RenderToFrontBuffer(func(fb []byte, stride int) {
		for gy := 0; gy < terminalGlyphHeight; gy++ {
			dst := (baseY+gy)*stride + baseX*4
			for gx := 0; gx < terminalGlyphWidth; gx++ {
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

func (vt *VideoTerminal) setCellLocked(col, row int, ch byte) {
	if col < 0 || col >= vt.cols || row < 0 || row >= vt.rows {
		return
	}
	vt.textBuf[row*vt.cols+col] = ch
}

func (vt *VideoTerminal) getCellLocked(col, row int) byte {
	if col < 0 || col >= vt.cols || row < 0 || row >= vt.rows {
		return 0
	}
	return vt.textBuf[row*vt.cols+col]
}

func writeColorLE(buf []byte, offset int, color uint32) {
	buf[offset] = byte(color)
	buf[offset+1] = byte(color >> 8)
	buf[offset+2] = byte(color >> 16)
	buf[offset+3] = byte(color >> 24)
}
