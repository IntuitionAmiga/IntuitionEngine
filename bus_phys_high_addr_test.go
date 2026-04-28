// bus_phys_high_addr_test.go - PLAN_MAX_RAM slice 10i.
//
// Pins the slice-3 phys-bus routing now that bus.memory can grow past
// 32 MiB: writes above len(bus.memory) must hit the bound Backing,
// writes below it must hit the legacy slice, and any access that
// straddles the seam must read as unmapped (zero) and write as no-op.

package main

import "testing"

func TestBusPhys_RoundTripAboveBusMemSeam(t *testing.T) {
	bus := NewMachineBus()
	seam := uint64(len(bus.GetMemory()))
	backing := NewSparseBacking(seam + 16*1024*1024)
	bus.SetBacking(backing)

	addr := seam + 1*1024*1024
	bus.WritePhys64(addr, 0xCAFEBABEDEADBEEF)
	if got := bus.ReadPhys64(addr); got != 0xCAFEBABEDEADBEEF {
		t.Fatalf("ReadPhys64 above seam = %#x, want 0xCAFEBABEDEADBEEF", got)
	}
	if got := backing.Read64(addr); got != 0xCAFEBABEDEADBEEF {
		t.Fatalf("backing.Read64 = %#x, want 0xCAFEBABEDEADBEEF", got)
	}
}

func TestBusPhys_BelowSeamRoutesToLegacy(t *testing.T) {
	bus := NewMachineBus()
	seam := uint64(len(bus.GetMemory()))
	backing := NewSparseBacking(seam + 16*1024*1024)
	bus.SetBacking(backing)

	addr := uint64(0x4000)
	bus.WritePhys32(addr, 0x12345678)
	if got := bus.Read32(uint32(addr)); got != 0x12345678 {
		t.Fatalf("legacy Read32 = %#x, want 0x12345678", got)
	}
	// Backing must NOT have observed the low-addr write.
	if got := backing.Read32(addr); got != 0 {
		t.Fatalf("backing.Read32 leaked low-addr write = %#x", got)
	}
}

func TestBusPhys_StraddleSeamUnmapped(t *testing.T) {
	bus := NewMachineBus()
	seam := uint64(len(bus.GetMemory()))
	backing := NewSparseBacking(seam + 16*1024*1024)
	bus.SetBacking(backing)

	// 8-byte access starting 4 bytes below seam straddles the boundary.
	addr := seam - 4
	if bus.PhysMapped(addr, 8) {
		t.Fatalf("PhysMapped reported straddling access as mapped")
	}
	// Pre-seed both sides with sentinel bytes to detect any leak.
	bus.Write32(uint32(addr), 0xAAAAAAAA)
	backing.Write32(seam, 0xBBBBBBBB)

	if _, ok := bus.ReadPhys64WithFault(addr); ok {
		t.Fatalf("ReadPhys64WithFault straddle reported ok, want fault")
	}
	if ok := bus.WritePhys64WithFault(addr, 0xDEADC0DEDEADC0DE); ok {
		t.Fatalf("WritePhys64WithFault straddle reported ok, want fault")
	}
	// Sentinels untouched.
	if got := bus.Read32(uint32(addr)); got != 0xAAAAAAAA {
		t.Fatalf("low sentinel clobbered = %#x", got)
	}
	if got := backing.Read32(seam); got != 0xBBBBBBBB {
		t.Fatalf("high sentinel clobbered = %#x", got)
	}
}
