// io_page_bitmap_test.go - PLAN_MAX_RAM.md slice 8 phase 6.
//
// Locks the bitmap-sizing invariant the lock-free Read32/Write32 fast
// path depends on: ioPageBitmap must cover every byte of bus.memory at
// PAGE_SIZE granularity. If a future slice widens bus.memory above
// DEFAULT_MEMORY_SIZE the bitmap must grow with it; otherwise the
// fast-path index `addr>>8` could read past the bitmap and the bus
// would lose MMIO dispatch above the bitmap's coverage.

package main

import "testing"

func TestMachineBus_IOPageBitmapMatchesMemoryWindow(t *testing.T) {
	bus := NewMachineBus()
	wantLen := len(bus.memory) / int(PAGE_SIZE)
	if got := len(bus.ioPageBitmap); got != wantLen {
		t.Fatalf("len(ioPageBitmap) = %d, want len(memory)/PAGE_SIZE = %d (fast-path bound invariant)",
			got, wantLen)
	}
}

func TestMachineBus_IOPageBitmapBoundsCheckGuardsFastPath(t *testing.T) {
	// The Read32/Write32 fast path bounds-checks addr+4 against len(memory)
	// before indexing the bitmap. With both sized off DEFAULT_MEMORY_SIZE
	// the highest bitmap index used is (len(memory)-4)>>8, which must be
	// strictly less than len(ioPageBitmap).
	bus := NewMachineBus()
	highestAddr := uint32(len(bus.memory)) - 4
	highestIdx := highestAddr >> 8
	if int(highestIdx) >= len(bus.ioPageBitmap) {
		t.Fatalf("highest bitmap index %d would index past len=%d", highestIdx, len(bus.ioPageBitmap))
	}
}
