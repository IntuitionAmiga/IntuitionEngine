package main

type pmgSnapshot struct {
	gractl     uint8
	prior      uint8
	sizep      [4]uint8
	sizem      uint8
	colpm      [4]uint8
	colpf      [4]uint8
	playerGfx  [4][ANTIC_DISPLAY_HEIGHT]uint8
	playerPos  [4][ANTIC_DISPLAY_HEIGHT]uint8
	missileGfx [4][ANTIC_DISPLAY_HEIGHT]uint8
	missilePos [4][ANTIC_DISPLAY_HEIGHT]uint8
	pfMask     []uint8
}

func (a *ANTICEngine) renderPMG(dst []byte, snap pmgSnapshot) {
	if snap.gractl&(GTIA_GRACTL_PLAYER|GTIA_GRACTL_MISSILE) == 0 {
		return
	}
	for y := ANTIC_BORDER_TOP; y < ANTIC_FRAME_HEIGHT-ANTIC_BORDER_BOTTOM; y++ {
		scanline := y - ANTIC_BORDER_TOP
		if scanline >= ANTIC_DISPLAY_HEIGHT {
			continue
		}
		pfMaskByX := playfieldMaskRow(snap.pfMask, y)

		if snap.gractl&GTIA_GRACTL_MISSILE != 0 {
			var playerMaskByX [ANTIC_FRAME_WIDTH]uint8
			var missileMaskByX [ANTIC_FRAME_WIDTH]uint8

			if snap.gractl&GTIA_GRACTL_PLAYER != 0 {
				for p := range 4 {
					gfx := snap.playerGfx[p][scanline]
					if gfx == 0 {
						continue
					}
					widthMult := playerWidth(snap.sizep[p])
					hpos := int(snap.playerPos[p][scanline])
					for bit := range 8 {
						if gfx&(0x80>>bit) == 0 {
							continue
						}
						baseX := hpos - 48 + ANTIC_BORDER_LEFT + bit*widthMult
						for w := 0; w < widthMult; w++ {
							x := baseX + w
							if x >= 0 && x < ANTIC_FRAME_WIDTH {
								playerMaskByX[x] |= 1 << p
							}
						}
					}
				}
			}

			for m := range 4 {
				if snap.missileGfx[m][scanline] == 0 {
					continue
				}
				width := missileWidth(snap.sizem, m)
				x0 := int(snap.missilePos[m][scanline]) - 48 + ANTIC_BORDER_LEFT
				for x := x0; x < x0+width; x++ {
					if x >= 0 && x < ANTIC_FRAME_WIDTH {
						a.noteMissilePlayerCollisions(m, playerMaskByX[x])
						missileMaskByX[x] |= 1 << m
					}
					a.composePMGPixel(dst, x, y, snap.colpm[m], m, true, snap.prior, pfMaskByX)
				}
			}

			if snap.gractl&GTIA_GRACTL_PLAYER != 0 {
				var drawnPlayerMaskByX [ANTIC_FRAME_WIDTH]uint8
				for p := range 4 {
					gfx := snap.playerGfx[p][scanline]
					if gfx == 0 {
						continue
					}
					widthMult := playerWidth(snap.sizep[p])
					hpos := int(snap.playerPos[p][scanline])
					for bit := range 8 {
						if gfx&(0x80>>bit) == 0 {
							continue
						}
						baseX := hpos - 48 + ANTIC_BORDER_LEFT + bit*widthMult
						for w := 0; w < widthMult; w++ {
							x := baseX + w
							if x >= 0 && x < ANTIC_FRAME_WIDTH {
								a.notePlayerCollisions(p, drawnPlayerMaskByX[x], missileMaskByX[x])
								drawnPlayerMaskByX[x] |= 1 << p
							}
							a.composePMGPixel(dst, x, y, snap.colpm[p], p, false, snap.prior, pfMaskByX)
						}
					}
				}
			}
			continue
		}

		if snap.gractl&GTIA_GRACTL_PLAYER != 0 {
			var drawnPlayerMaskByX [ANTIC_FRAME_WIDTH]uint8
			for p := range 4 {
				gfx := snap.playerGfx[p][scanline]
				if gfx == 0 {
					continue
				}
				widthMult := playerWidth(snap.sizep[p])
				hpos := int(snap.playerPos[p][scanline])
				for bit := range 8 {
					if gfx&(0x80>>bit) == 0 {
						continue
					}
					baseX := hpos - 48 + ANTIC_BORDER_LEFT + bit*widthMult
					for w := 0; w < widthMult; w++ {
						x := baseX + w
						if x >= 0 && x < ANTIC_FRAME_WIDTH {
							a.notePlayerCollisions(p, drawnPlayerMaskByX[x], 0)
							drawnPlayerMaskByX[x] |= 1 << p
						}
						a.composePMGPixel(dst, x, y, snap.colpm[p], p, false, snap.prior, pfMaskByX)
					}
				}
			}
		}
	}
}

func playerWidth(size uint8) int {
	switch size & 0x03 {
	case 1:
		return 2
	case 3:
		return 4
	default:
		return 1
	}
}

func missileWidth(sizem uint8, missile int) int {
	return playerWidth((sizem >> (missile * 2)) & 0x03)
}

func (a *ANTICEngine) composePMGPixel(dst []byte, x, y int, color uint8, obj int, missile bool, prior uint8, pfMaskByX [ANTIC_FRAME_WIDTH]uint8) {
	if x < 0 || x >= ANTIC_FRAME_WIDTH || y < 0 || y >= ANTIC_FRAME_HEIGHT {
		return
	}
	offset := (y*ANTIC_FRAME_WIDTH + x) * 4
	pfMask := pfMaskByX[x]
	if pfMask != 0 {
		if missile {
			a.missilePF[obj] |= pfMask
		} else {
			a.playerPF[obj] |= pfMask
		}
	}

	if !pmgOverPlayfield(obj, prior, pfMask) {
		return
	}
	copy(dst[offset:offset+4], ANTICPaletteRGBA[color][:])
}

func playfieldMaskRow(pfMask []uint8, y int) [ANTIC_FRAME_WIDTH]uint8 {
	var masks [ANTIC_FRAME_WIDTH]uint8
	if len(pfMask) < ANTIC_FRAME_WIDTH*ANTIC_FRAME_HEIGHT {
		return masks
	}
	rowStart := y * ANTIC_FRAME_WIDTH
	for x := range ANTIC_FRAME_WIDTH {
		masks[x] = pfMask[rowStart+x]
	}
	return masks
}

func (a *ANTICEngine) notePlayerPlayerCollision(aObj, bObj int) {
	if aObj == bObj {
		return
	}
	a.playerPL[aObj] |= 1 << bObj
	a.playerPL[bObj] |= 1 << aObj
}

func (a *ANTICEngine) noteMissilePlayerCollisions(missile int, playerMask uint8) {
	a.missilePL[missile] |= playerMask
}

func (a *ANTICEngine) notePlayerCollisions(player int, playerMask, missileMask uint8) {
	for p := range 4 {
		if playerMask&(1<<p) != 0 {
			a.notePlayerPlayerCollision(player, p)
		}
		if missileMask&(1<<p) != 0 {
			a.noteMissilePlayerCollisions(p, 1<<player)
		}
	}
}

func pmgOverPlayfield(obj int, prior uint8, pfMask uint8) bool {
	if pfMask == 0 {
		return true
	}
	switch {
	case prior&GTIA_PRIOR_P03 != 0:
		return true
	case prior&GTIA_PRIOR_P01 != 0:
		return obj < 2
	case prior&GTIA_PRIOR_P23 != 0:
		return obj >= 2
	case prior&0x08 != 0:
		return pfMask&(1<<2|1<<3) != 0
	default:
		return true
	}
}
