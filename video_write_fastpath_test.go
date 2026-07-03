package main

import (
	"encoding/binary"
	"testing"
)

func videoDirtySnapshot(chip *VideoChip) [DIRTY_GRID_SIZE]uint64 {
	var out [DIRTY_GRID_SIZE]uint64
	for i := range chip.dirtyBitmap {
		out[i] = chip.dirtyBitmap[i].Load()
	}
	return out
}

func TestHandleWrite_VRAMFastPathWritesFrontBuffer(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	chip := &VideoChip{
		currentMode:    MODE_640x480,
		frontBuffer:    make([]byte, mode.totalSize),
		blitterEnabled: true,
	}
	chip.initialiseDirtyGrid(mode)

	addr := vramAddr(mode, 3, 4)
	chip.handleWriteLocked(addr, 0x11223344)

	offset := addr - BUFFER_OFFSET
	if got := binary.LittleEndian.Uint32(chip.frontBuffer[offset:]); got != 0x11223344 {
		t.Fatalf("frontBuffer pixel = 0x%08X, want 0x11223344", got)
	}
	if !chip.hasDirtyTiles() {
		t.Fatal("VRAM write did not mark dirty")
	}
}

func TestBlitFillRasterOp32FrontBufferMatchesReference(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	chip := &VideoChip{
		currentMode:     MODE_640x480,
		frontBuffer:     make([]byte, mode.totalSize),
		blitterEnabled:  true,
		bltDst:          vramAddr(mode, 5, 7),
		bltWidth:        6,
		bltHeight:       4,
		bltDstStrideRun: uint32(mode.bytesPerRow),
		bltColor:        0x0F0F00FF,
		bltFlags:        uint32(0x06 << bltFlagsDrawModeShift),
	}
	chip.initialiseDirtyGrid(mode)
	reference := make([]byte, len(chip.frontBuffer))
	for i := range chip.frontBuffer {
		value := byte((i * 17) & 0xFF)
		chip.frontBuffer[i] = value
		reference[i] = value
	}

	for y := 7; y < 11; y++ {
		for x := 5; x < 11; x++ {
			off := uint32((y*mode.width + x) * BYTES_PER_PIXEL)
			dst := binary.LittleEndian.Uint32(reference[off : off+BYTES_PER_PIXEL])
			binary.LittleEndian.PutUint32(reference[off:], applyDrawMode(chip.bltColor, dst, 0x06))
		}
	}

	chip.blitFillLocked(mode)
	if chip.bltErr {
		t.Fatal("blitFillLocked set bltErr")
	}
	for i := range reference {
		if chip.frontBuffer[i] != reference[i] {
			t.Fatalf("frontBuffer[%d] = 0x%02X, want 0x%02X", i, chip.frontBuffer[i], reference[i])
		}
	}

	expectedDirty := &VideoChip{currentMode: MODE_640x480}
	expectedDirty.initialiseDirtyGrid(mode)
	expectedDirty.markRectDirty(5, 7, 6, 4)
	if got, want := videoDirtySnapshot(chip), videoDirtySnapshot(expectedDirty); got != want {
		t.Fatalf("dirty bitmap = %#v, want %#v", got, want)
	}
}

func TestRasterBand_BusInvalidatesOncePerRow(t *testing.T) {
	mode := VideoModes[MODE_640x480]
	chip := &VideoChip{
		currentMode:  MODE_640x480,
		busMemory:    make([]byte, int(VRAM_START)+mode.totalSize),
		fbBase:       VRAM_START,
		rasterY:      10,
		rasterHeight: 3,
		rasterColor:  0xAABBCCDD,
	}
	chip.initialiseDirtyGrid(mode)

	type invalidation struct {
		addr uint32
		size uint32
	}
	var invalidations []invalidation
	chip.invalidateBusMemoryWriteHook = func(addr, size uint32) {
		invalidations = append(invalidations, invalidation{addr: addr, size: size})
	}

	chip.drawRasterBandLocked()

	if len(invalidations) != 3 {
		t.Fatalf("invalidations = %d, want 3", len(invalidations))
	}
	for i, inv := range invalidations {
		wantAddr := VRAM_START + uint32((10+i)*mode.bytesPerRow)
		if inv.addr != wantAddr || inv.size != uint32(mode.bytesPerRow) {
			t.Fatalf("invalidation %d = {0x%X,%d}, want {0x%X,%d}", i, inv.addr, inv.size, wantAddr, mode.bytesPerRow)
		}
	}
}
