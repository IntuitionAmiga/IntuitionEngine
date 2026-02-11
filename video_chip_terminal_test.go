package main

import (
	"math/bits"
	"testing"
)

func TestRenderToFrontBuffer(t *testing.T) {
	chip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip failed: %v", err)
	}

	color := uint32(0x11223344)
	chip.RenderToFrontBuffer(func(fb []byte, _ int) {
		fb[0] = byte(color)
		fb[1] = byte(color >> 8)
		fb[2] = byte(color >> 16)
		fb[3] = byte(color >> 24)
	})

	fb := chip.GetFrontBuffer()
	got := uint32(fb[0]) | uint32(fb[1])<<8 | uint32(fb[2])<<16 | uint32(fb[3])<<24
	if got != color {
		t.Fatalf("expected 0x%08X at pixel 0, got 0x%08X", color, got)
	}
}

func TestMarkRectDirty_SingleTile(t *testing.T) {
	chip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip failed: %v", err)
	}

	for i := range chip.dirtyBitmap {
		chip.dirtyBitmap[i].Store(0)
	}
	chip.MarkRectDirty(4, 4, 8, 16)
	dirty := chip.clearDirtyBitmap()

	count := 0
	for _, w := range dirty {
		count += bits.OnesCount64(w)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 dirty tile, got %d", count)
	}
}

func TestMarkRectDirty_CrossTile(t *testing.T) {
	chip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip failed: %v", err)
	}

	tileW := int(chip.tileWidth)
	if tileW <= 0 {
		t.Fatal("expected positive tile width")
	}

	for i := range chip.dirtyBitmap {
		chip.dirtyBitmap[i].Store(0)
	}
	chip.MarkRectDirty(tileW-4, 0, 8, 16)
	dirty := chip.clearDirtyBitmap()

	count := 0
	for _, w := range dirty {
		count += bits.OnesCount64(w)
	}
	if count != 2 {
		t.Fatalf("expected 2 dirty tiles for boundary crossing rect, got %d", count)
	}
}

func TestMarkRectDirty_FullScreen(t *testing.T) {
	chip, err := NewVideoChip(VIDEO_BACKEND_EBITEN)
	if err != nil {
		t.Fatalf("NewVideoChip failed: %v", err)
	}

	mode := VideoModes[chip.currentMode]
	for i := range chip.dirtyBitmap {
		chip.dirtyBitmap[i].Store(0)
	}
	chip.MarkRectDirty(0, 0, mode.width, mode.height)
	dirty := chip.clearDirtyBitmap()

	count := 0
	for _, w := range dirty {
		count += bits.OnesCount64(w)
	}
	if count != DIRTY_GRID_COLS*DIRTY_GRID_ROWS {
		t.Fatalf("expected all tiles dirty (%d), got %d", DIRTY_GRID_COLS*DIRTY_GRID_ROWS, count)
	}
}
