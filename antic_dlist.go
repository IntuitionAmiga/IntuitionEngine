package main

const anticDisplayListMaxEntries = 1024

func (a *ANTICEngine) renderDisplayList(dst []byte, pfMask []uint8) {
	if a.bus == nil || a.dmactl&ANTIC_DMA_DL == 0 {
		return
	}

	pc := a.getDisplayListAddress()
	screenAddr := uint16(0)
	y := ANTIC_BORDER_TOP
	for entries := 0; entries < anticDisplayListMaxEntries && y < ANTIC_BORDER_TOP+ANTIC_DISPLAY_HEIGHT; entries++ {
		entry, next := a.decodeDisplayListInstruction(pc)
		pc = next

		if entry.IsJump {
			pc = entry.JumpAddr
			if entry.IsJVB {
				break
			}
			continue
		}
		if entry.HasLMS {
			screenAddr = entry.MemoryAddr
		}
		if entry.IsBlank {
			y += entry.ScanLines
			continue
		}

		a.renderDLMode(dst, pfMask, y, screenAddr, entry)
		screenAddr += uint16(bytesConsumedByDLMode(entry.Mode, entry.ScanLines))
		y += entry.ScanLines

		if entry.HasDLI && a.nmien&ANTIC_NMIEN_DLI != 0 {
			a.nmist |= ANTIC_NMIST_DLI
			if a.sink != nil {
				a.sink.Pulse(IntMaskDLI)
			}
		}
	}
}

func (a *ANTICEngine) processANTICScanline(y int) {
	pass := &a.scanlinePass
	if pass.stopped || len(pass.target) < ANTIC_FRAME_WIDTH*ANTIC_FRAME_HEIGHT*4 {
		return
	}
	if y < ANTIC_BORDER_TOP || y >= ANTIC_BORDER_TOP+ANTIC_DISPLAY_HEIGHT {
		return
	}
	if a.bus == nil || a.dmactl&ANTIC_DMA_DL == 0 {
		return
	}

	for pass.entries < anticDisplayListMaxEntries && !pass.stopped {
		if !pass.entryValid {
			entry, next := a.decodeDisplayListInstruction(pass.pc)
			pass.pc = next
			pass.entries++

			if entry.IsJump {
				pass.pc = entry.JumpAddr
				if entry.IsJVB {
					pass.stopped = true
					return
				}
				continue
			}
			if entry.HasLMS {
				pass.screenAddr = entry.MemoryAddr
			}
			if entry.IsBlank {
				pass.displayY += entry.ScanLines
				if y < pass.displayY {
					return
				}
				continue
			}

			pass.entry = entry
			pass.entryLine = 0
			pass.entryValid = true
		}

		if y < pass.displayY {
			return
		}
		if y >= pass.displayY+pass.entry.ScanLines {
			a.finishANTICScanlineEntry(pass)
			continue
		}

		for pass.entryValid && pass.displayY+pass.entryLine <= y {
			rowY := pass.displayY + pass.entryLine
			a.renderDLModeScanline(pass.target, pass.pfMask, rowY, pass.screenAddr, pass.entry, pass.entryLine)
			pass.entryLine++
			if pass.entryLine >= pass.entry.ScanLines {
				a.finishANTICScanlineEntry(pass)
			}
		}
		return
	}
}

func (a *ANTICEngine) finishANTICScanlineEntry(pass *anticScanlinePass) {
	entry := pass.entry
	pass.screenAddr += uint16(bytesConsumedByDLMode(entry.Mode, entry.ScanLines))
	pass.displayY += entry.ScanLines
	pass.entryValid = false
	pass.entryLine = 0

	if entry.HasDLI && a.nmien&ANTIC_NMIEN_DLI != 0 {
		a.nmist |= ANTIC_NMIST_DLI
		if a.sink != nil {
			a.sink.Pulse(IntMaskDLI)
		}
	}
}

func (a *ANTICEngine) putFramePixel(dst []byte, pfMask []uint8, x, y int, color uint8, playfieldMask uint8) {
	if x < 0 || x >= ANTIC_FRAME_WIDTH || y < 0 || y >= ANTIC_FRAME_HEIGHT {
		return
	}
	pixel := y*ANTIC_FRAME_WIDTH + x
	offset := pixel * 4
	copy(dst[offset:offset+4], ANTICPaletteRGBA[color][:])
	if pfMask != nil {
		pfMask[pixel] = playfieldMask
	}
}
