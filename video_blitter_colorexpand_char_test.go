package main

import (
	"encoding/binary"
	"testing"
)

// Characterisation of blitColorExpandLocked against an independent reference
// model of the colour-expand contract. Pins semantics before the fast-path /
// SIMD refactor: MSB-first template bit order, invertTmpl bit flip, JAM1 skip of
// clear bits, JAM2 background write, invert mode XOR, and the no-op cases. The
// model is deliberately re-derived here, not taken from the implementation.

type colorExpandCase struct {
	jam1, invertTmpl, invertMode bool
	bpp                          int
	maskSrcX                     int
	width, height                int
}

const ceMaskBase = 0x3000 // scratch mask region inside busMemory

// runColorExpand drives blitColorExpandLocked directly with the given state and
// returns the destination VRAM region as bytes. maskBits are laid out one row
// per maskMod stride from ceMaskBase.
func runColorExpand(t *testing.T, video *VideoChip, bus *MachineBus, c colorExpandCase, maskRows [][]byte, fg, bg uint32) []byte {
	t.Helper()
	bpp := c.bpp
	dstStride := uint32(c.width * bpp)
	maskMod := uint32(len(maskRows[0]))

	// Clear dst region.
	dstBytes := c.width * c.height * bpp
	for i := 0; i < dstBytes; i++ {
		bus.memory[VRAM_START+uint32(i)] = 0
	}
	// Lay out mask rows.
	for y, row := range maskRows {
		base := ceMaskBase + uint32(y)*maskMod
		copy(bus.memory[base:base+uint32(len(row))], row)
	}

	flags := uint32(0)
	if bpp == 1 {
		flags |= bltFlagsBPP_CLUT8
	}
	if c.jam1 {
		flags |= bltFlagsJAM1
	}
	if c.invertTmpl {
		flags |= bltFlagsInvertTmpl
	}
	if c.invertMode {
		flags |= bltFlagsInvertMode
	}

	video.mu.Lock()
	video.bltWidth = uint32(c.width)
	video.bltHeight = uint32(c.height)
	video.bltFlags = flags
	video.bltFG = fg
	video.bltBG = bg
	video.bltDst = VRAM_START
	video.bltDstStrideRun = dstStride
	video.bltMask = ceMaskBase
	video.bltMaskMod = maskMod
	video.bltMaskSrcX = uint32(c.maskSrcX)
	mode := VideoModes[video.currentMode]
	video.blitColorExpandLocked(mode)
	video.mu.Unlock()

	out := make([]byte, dstBytes)
	copy(out, bus.memory[VRAM_START:VRAM_START+uint32(dstBytes)])
	return out
}

// modelColorExpand is the independent reference: it produces the expected dst
// bytes for a zero-initialised destination.
func modelColorExpand(c colorExpandCase, maskRows [][]byte, fg, bg uint32) []byte {
	bpp := c.bpp
	dstStride := c.width * bpp
	out := make([]byte, c.width*c.height*bpp)
	writePixel := func(off int, v uint32) {
		if bpp == 1 {
			out[off] = byte(v)
		} else {
			binary.LittleEndian.PutUint32(out[off:off+4], v)
		}
	}
	readPixel := func(off int) uint32 {
		if bpp == 1 {
			return uint32(out[off])
		}
		return binary.LittleEndian.Uint32(out[off : off+4])
	}
	for y := 0; y < c.height; y++ {
		row := maskRows[y]
		for x := 0; x < c.width; x++ {
			bitX := c.maskSrcX + x
			b := (row[bitX/8] >> uint(7-(bitX%8))) & 1
			if c.invertTmpl {
				b ^= 1
			}
			off := y*dstStride + x*bpp
			switch {
			case c.invertMode:
				if b == 1 {
					if bpp == 1 {
						writePixel(off, readPixel(off)^0xFF)
					} else {
						writePixel(off, readPixel(off)^0xFFFFFFFF)
					}
				}
			case b == 1:
				writePixel(off, fg)
			case !c.jam1:
				writePixel(off, bg)
			}
		}
	}
	return out
}

func makeMaskRows(height, byteLen int, seed byte) [][]byte {
	rows := make([][]byte, height)
	for y := 0; y < height; y++ {
		r := make([]byte, byteLen)
		for i := range r {
			r[i] = seed ^ byte(y*31+i*97+0x5A)
		}
		rows[y] = r
	}
	return rows
}

// runColorExpandGeneric drives the generic per-bit loop directly (bypassing the
// fast-path branch) with the given state, returning the dst VRAM region.
func runColorExpandGeneric(t *testing.T, video *VideoChip, bus *MachineBus, c colorExpandCase, maskRows [][]byte, fg, bg uint32) []byte {
	t.Helper()
	bpp := c.bpp
	dstStride := uint32(c.width * bpp)
	maskMod := uint32(len(maskRows[0]))
	dstBytes := c.width * c.height * bpp
	for i := 0; i < dstBytes; i++ {
		bus.memory[VRAM_START+uint32(i)] = 0
	}
	for y, row := range maskRows {
		base := ceMaskBase + uint32(y)*maskMod
		copy(bus.memory[base:base+uint32(len(row))], row)
	}
	video.mu.Lock()
	mode := VideoModes[video.currentMode]
	video.blitColorExpandGenericLocked(mode, c.width, c.height, bpp, uint32(bpp), c.jam1, c.invertTmpl, c.invertMode, fg, bg, ceMaskBase, maskMod, c.maskSrcX, dstStride, VRAM_START)
	video.mu.Unlock()
	out := make([]byte, dstBytes)
	copy(out, bus.memory[VRAM_START:VRAM_START+uint32(dstBytes)])
	return out
}

// TestBlitColorExpandFastPathMatchesGenericPath proves the fast path is
// byte-identical to the generic per-bit loop, including the all-zero JAM1 no-op
// row (asserted separately via hasContent side effect).
func TestBlitColorExpandFastPathMatchesGenericPath(t *testing.T) {
	video, bus := newDirectVRAMTestRig(t)
	const height = 4
	for _, bpp := range []int{1, 4} {
		for _, width := range []int{1, 8, 9, 31, 33, 257} {
			for _, maskSrcX := range []int{0, 3, 7, 8} {
				byteLen := (maskSrcX + width + 7) / 8
				maskRows := makeMaskRows(height, byteLen, 0x9E)
				for _, jam1 := range []bool{false, true} {
					for _, invT := range []bool{false, true} {
						for _, invM := range []bool{false, true} {
							c := colorExpandCase{jam1, invT, invM, bpp, maskSrcX, width, height}
							fast := runColorExpand(t, video, bus, c, maskRows, 0xFFAA55CC, 0xFF112233)
							generic := runColorExpandGeneric(t, video, bus, c, maskRows, 0xFFAA55CC, 0xFF112233)
							for i := range fast {
								if fast[i] != generic[i] {
									t.Fatalf("case %+v byte %d: fast %#02x generic %#02x", c, i, fast[i], generic[i])
								}
							}
						}
					}
				}
			}
		}
	}
}

// TestBlitColorExpandAllZeroJAM1RowIsNoOp pins the no-op contract: an all-clear
// JAM1 row must leave hasContent false (no writes, no invalidation).
func TestBlitColorExpandAllZeroJAM1RowIsNoOp(t *testing.T) {
	video, bus := newDirectVRAMTestRig(t)
	video.hasContent.Store(false)
	maskRows := [][]byte{{0x00}, {0x00}} // all clear
	c := colorExpandCase{jam1: true, bpp: 4, maskSrcX: 0, width: 8, height: 2}
	runColorExpand(t, video, bus, c, maskRows, 0xFFAA55CC, 0xFF112233)
	if video.hasContent.Load() {
		t.Fatal("all-clear JAM1 blit set hasContent; expected no-op")
	}
}

// TestColorExpandFastPathRejectsAddressWrap pins that a destination row that
// starts above VRAM, or whose end address would wrap uint32, is ineligible for
// the fast path (routed to the generic per-pixel/bus loop) rather than slicing
// busMemory with a wrapped range and panicking.
func TestColorExpandFastPathRejectsAddressWrap(t *testing.T) {
	video, _ := newDirectVRAMTestRig(t)
	video.mu.Lock()
	defer video.mu.Unlock()

	// Baseline: a normal in-VRAM row is eligible.
	if _, ok := video.colorExpandFastPathEligibleLocked(256, 1, 4, VRAM_START, 256*4, ceMaskBase, 0, 0); !ok {
		t.Fatal("in-VRAM row should be eligible")
	}
	for name, tc := range map[string]struct {
		dstRow, dstStride uint32
		width, height     int
	}{
		"row start above VRAM":     {0xFFFFF000, 256 * 4, 256, 1},
		"row end wraps uint32":     {0xFFFFFF00, 256 * 4, 256, 1},
		"stride pushes row past":   {VRAM_START, 0xFFFF0000, 256, 4},
		"y*stride wraps into VRAM": {VRAM_START, 0x40000000, 256, 6},
	} {
		if _, ok := video.colorExpandFastPathEligibleLocked(tc.width, tc.height, 4, tc.dstRow, tc.dstStride, ceMaskBase, 0, 0); ok {
			t.Fatalf("%s: must be ineligible (route generic)", name)
		}
	}
}

func TestBlitColorExpandCharacterisationMatrix(t *testing.T) {
	video, bus := newDirectVRAMTestRig(t)
	const height = 3
	widths := []int{1, 7, 8, 9, 31, 33, 640}
	const fg = 0xFF1234AB
	const bg = 0xFF775533
	for _, bpp := range []int{1, 4} {
		for _, width := range widths {
			for _, maskSrcX := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9} {
				byteLen := (maskSrcX + width + 7) / 8
				maskRows := makeMaskRows(height, byteLen, 0xC3)
				for _, jam1 := range []bool{false, true} {
					for _, invT := range []bool{false, true} {
						for _, invM := range []bool{false, true} {
							c := colorExpandCase{jam1, invT, invM, bpp, maskSrcX, width, height}
							got := runColorExpand(t, video, bus, c, maskRows, fg, bg)
							want := modelColorExpand(c, maskRows, fg, bg)
							for i := range want {
								if got[i] != want[i] {
									t.Fatalf("case %+v byte %d: got %#02x want %#02x", c, i, got[i], want[i])
								}
							}
						}
					}
				}
			}
		}
	}
}
