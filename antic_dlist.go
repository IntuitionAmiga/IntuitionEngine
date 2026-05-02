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
