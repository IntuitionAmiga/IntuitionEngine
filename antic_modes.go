package main

func bytesForDLMode(mode uint8) int {
	switch mode {
	case 6, 7:
		return 20
	case 8, 15:
		return 40
	case 9, 10:
		return 20
	case 11, 12, 13, 14:
		return 40
	default:
		return 40
	}
}

func bytesConsumedByDLMode(mode uint8, scanlines int) int {
	if mode >= 2 && mode <= 7 {
		return bytesForDLMode(mode)
	}
	return bytesForDLMode(mode) * scanlines
}

func (a *ANTICEngine) renderDLMode(dst []byte, pfMask []uint8, y int, screenAddr uint16, entry DisplayListEntry) {
	if entry.Mode >= 2 && entry.Mode <= 7 {
		cols := 40
		cellWidth := 8
		if entry.Mode == 6 || entry.Mode == 7 {
			cols = 20
			cellWidth = 16
		}
		a.renderTextMode(dst, pfMask, y, screenAddr, entry, cols, cellWidth)
		return
	}
	a.renderBitmapMode(dst, pfMask, y, screenAddr, entry)
}

func (a *ANTICEngine) renderDLModeScanline(dst []byte, pfMask []uint8, y int, screenAddr uint16, entry DisplayListEntry, row int) {
	if row < 0 || row >= entry.ScanLines {
		return
	}
	if entry.Mode >= 2 && entry.Mode <= 7 {
		cols := 40
		cellWidth := 8
		if entry.Mode == 6 || entry.Mode == 7 {
			cols = 20
			cellWidth = 16
		}
		a.renderTextModeScanline(dst, pfMask, y, screenAddr, entry, row, cols, cellWidth)
		return
	}
	a.renderBitmapModeScanline(dst, pfMask, y, screenAddr, entry, row)
}

func (a *ANTICEngine) renderTextMode(dst []byte, pfMask []uint8, y int, screenAddr uint16, entry DisplayListEntry, cols, cellWidth int) {
	charBase := uint32(a.chbase) << 8
	hscroll := 0
	if entry.HasHScrol {
		hscroll = int(a.hscrol)
	}
	vscroll := 0
	if entry.HasVScrol {
		vscroll = int(a.vscrol)
	}
	for row := 0; row < entry.ScanLines; row++ {
		glyphRow := (row + vscroll) & 7
		if a.chactl&ANTIC_CHACTL_REFLECT != 0 {
			glyphRow = 7 - glyphRow
		}
		for col := 0; col < cols; col++ {
			ch := a.bus.Read8(uint32(screenAddr) + uint32(col))
			glyph := uint8(0)
			if a.chactl&ANTIC_CHACTL_BLANK == 0 {
				glyph = a.bus.Read8(charBase + uint32(ch)*8 + uint32(glyphRow))
				if a.chactl&ANTIC_CHACTL_INVERT != 0 {
					glyph ^= 0xFF
				}
			}
			for sx := 0; sx < cellWidth; sx++ {
				bit := (sx * 8) / cellWidth
				color := a.colpf[0]
				mask := uint8(1 << 0)
				if glyph&(0x80>>bit) != 0 {
					color = a.colpf[1]
					mask = 1 << 1
				}
				a.putFramePixel(dst, pfMask, ANTIC_BORDER_LEFT+col*cellWidth+sx-hscroll, y+row, color, mask)
			}
		}
	}
}

func (a *ANTICEngine) renderTextModeScanline(dst []byte, pfMask []uint8, y int, screenAddr uint16, entry DisplayListEntry, row, cols, cellWidth int) {
	charBase := uint32(a.chbase) << 8
	hscroll := 0
	if entry.HasHScrol {
		hscroll = int(a.hscrol)
	}
	vscroll := 0
	if entry.HasVScrol {
		vscroll = int(a.vscrol)
	}
	glyphRow := (row + vscroll) & 7
	if a.chactl&ANTIC_CHACTL_REFLECT != 0 {
		glyphRow = 7 - glyphRow
	}
	for col := 0; col < cols; col++ {
		ch := a.bus.Read8(uint32(screenAddr) + uint32(col))
		glyph := uint8(0)
		if a.chactl&ANTIC_CHACTL_BLANK == 0 {
			glyph = a.bus.Read8(charBase + uint32(ch)*8 + uint32(glyphRow))
			if a.chactl&ANTIC_CHACTL_INVERT != 0 {
				glyph ^= 0xFF
			}
		}
		for sx := 0; sx < cellWidth; sx++ {
			bit := (sx * 8) / cellWidth
			color := a.colpf[0]
			mask := uint8(1 << 0)
			if glyph&(0x80>>bit) != 0 {
				color = a.colpf[1]
				mask = 1 << 1
			}
			a.putFramePixel(dst, pfMask, ANTIC_BORDER_LEFT+col*cellWidth+sx-hscroll, y, color, mask)
		}
	}
}

func (a *ANTICEngine) renderBitmapMode(dst []byte, pfMask []uint8, y int, screenAddr uint16, entry DisplayListEntry) {
	hscroll := 0
	if entry.HasHScrol {
		hscroll = int(a.hscrol)
	}
	bytesPerLine := bytesForDLMode(entry.Mode)
	pixelsPerByte := 8
	if entry.Mode >= 9 && entry.Mode <= 14 {
		pixelsPerByte = 4
	}
	for row := 0; row < entry.ScanLines; row++ {
		srcRow := row
		if entry.HasVScrol && entry.ScanLines > 1 {
			srcRow = (row + int(a.vscrol)) % entry.ScanLines
		}
		for col := 0; col < bytesPerLine; col++ {
			value := a.bus.Read8(uint32(screenAddr) + uint32(srcRow*bytesPerLine+col))
			if entry.Mode == DL_MODE8 {
				color := a.colpf[0]
				mask := uint8(1 << 0)
				if value != 0 {
					color = a.applyGTIAModeColor(a.colpf[1], value&0x0F, value)
					mask = 1 << 1
				}
				for bit := 0; bit < 8; bit++ {
					a.putFramePixel(dst, pfMask, ANTIC_BORDER_LEFT+col*8+bit-hscroll, y+row, color, mask)
				}
				continue
			}
			if pixelsPerByte == 8 {
				for bit := 0; bit < 8; bit++ {
					color := a.colpf[0]
					mask := uint8(1 << 0)
					if value&(0x80>>bit) != 0 {
						color = a.applyGTIAModeColor(a.colpf[1], uint8(bit), value)
						mask = 1 << 1
					}
					a.putFramePixel(dst, pfMask, ANTIC_BORDER_LEFT+col*8+bit-hscroll, y+row, color, mask)
				}
				continue
			}
			for pair := 0; pair < 4; pair++ {
				idx := (value >> (6 - pair*2)) & 0x03
				color := a.applyGTIAModeColor(a.colpf[idx], idx, value)
				mask := uint8(1 << idx)
				for sx := 0; sx < 2; sx++ {
					a.putFramePixel(dst, pfMask, ANTIC_BORDER_LEFT+col*8+pair*2+sx-hscroll, y+row, color, mask)
				}
			}
		}
	}
}

func (a *ANTICEngine) renderBitmapModeScanline(dst []byte, pfMask []uint8, y int, screenAddr uint16, entry DisplayListEntry, row int) {
	hscroll := 0
	if entry.HasHScrol {
		hscroll = int(a.hscrol)
	}
	bytesPerLine := bytesForDLMode(entry.Mode)
	pixelsPerByte := 8
	if entry.Mode >= 9 && entry.Mode <= 14 {
		pixelsPerByte = 4
	}
	srcRow := row
	if entry.HasVScrol && entry.ScanLines > 1 {
		srcRow = (row + int(a.vscrol)) % entry.ScanLines
	}
	for col := 0; col < bytesPerLine; col++ {
		value := a.bus.Read8(uint32(screenAddr) + uint32(srcRow*bytesPerLine+col))
		if entry.Mode == DL_MODE8 {
			color := a.colpf[0]
			mask := uint8(1 << 0)
			if value != 0 {
				color = a.applyGTIAModeColor(a.colpf[1], value&0x0F, value)
				mask = 1 << 1
			}
			for bit := 0; bit < 8; bit++ {
				a.putFramePixel(dst, pfMask, ANTIC_BORDER_LEFT+col*8+bit-hscroll, y, color, mask)
			}
			continue
		}
		if pixelsPerByte == 8 {
			for bit := 0; bit < 8; bit++ {
				color := a.colpf[0]
				mask := uint8(1 << 0)
				if value&(0x80>>bit) != 0 {
					color = a.applyGTIAModeColor(a.colpf[1], uint8(bit), value)
					mask = 1 << 1
				}
				a.putFramePixel(dst, pfMask, ANTIC_BORDER_LEFT+col*8+bit-hscroll, y, color, mask)
			}
			continue
		}
		for pair := 0; pair < 4; pair++ {
			idx := (value >> (6 - pair*2)) & 0x03
			color := a.applyGTIAModeColor(a.colpf[idx], idx, value)
			mask := uint8(1 << idx)
			for sx := 0; sx < 2; sx++ {
				a.putFramePixel(dst, pfMask, ANTIC_BORDER_LEFT+col*8+pair*2+sx-hscroll, y, color, mask)
			}
		}
	}
}

func (a *ANTICEngine) applyGTIAModeColor(base uint8, idx uint8, value uint8) uint8 {
	mode := a.prior & (GTIA_PRIOR_GTIA1 | GTIA_PRIOR_GTIA2)
	switch mode {
	case GTIA_PRIOR_GTIA1:
		return (a.colpf[1] & 0xF0) | (value & 0x0F)
	case GTIA_PRIOR_GTIA2:
		return a.colpf[int(idx)%4]
	case GTIA_PRIOR_GTIA1 | GTIA_PRIOR_GTIA2:
		return (value & 0xF0) | (a.colbk & 0x0F)
	default:
		return base
	}
}
