//go:build darwin

package main

import "testing"

func TestDarwin_MmapBackingResetActuallyZeroes(t *testing.T) {
	b, err := NewMmapBacking(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatalf("NewMmapBacking: %v", err)
	}
	defer func() { _ = b.Close() }()

	b.Write32(0, 0xCAFEBABE)
	b.Reset()
	if got := b.Read32(0); got != 0 {
		t.Fatalf("after Reset Read32(0)=0x%08X, want 0", got)
	}
}

func TestDarwin_BusResetActuallyZeroesMmapMemory(t *testing.T) {
	bus, err := NewMachineBusSized(busMemMmapThreshold)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	bus.Write32(0x1000, 0x11223344)
	bus.Reset()
	if got := bus.Read32(0x1000); got != 0 {
		t.Fatalf("after Reset Read32(0x1000)=0x%08X, want 0", got)
	}
}
