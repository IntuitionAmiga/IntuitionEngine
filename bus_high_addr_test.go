// bus_high_addr_test.go - PLAN_MAX_RAM Bus32 high-backing roundtrip.
//
// Pins that bus.memory allocated up to busMemMaxBytes (~4 GiB-page) on
// mmap-capable hosts is reachable end-to-end via the legacy uint32 Read/
// Write paths used by IE32/x86/bare-M68K. Page is demand-committed by
// the kernel so RSS impact is bounded by what the test touches.
//
// Build tag scopes the test to platforms where busMemBootClamp ==
// busMemMaxBytes (Linux/darwin); other platforms clamp the boot allocation
// at 256 MiB and cannot exercise the >256 MiB regime.

//go:build linux || darwin

package main

import "testing"

func TestBus32_RoundTripAt3GiB_MmapDemandPaged(t *testing.T) {
	bus, err := NewMachineBusSized(busMemMaxBytes)
	if err != nil {
		t.Fatalf("NewMachineBusSized(busMemMaxBytes): %v", err)
	}

	const addr uint32 = 3 * 1024 * 1024 * 1024 // 3 GiB
	if uint64(addr)+4 > busMemMaxBytes {
		t.Fatalf("test offset above bus cap: addr=%#x cap=%#x", addr, busMemMaxBytes)
	}

	bus.Write32(addr, 0xDEADBEEF)
	if got := bus.Read32(addr); got != 0xDEADBEEF {
		t.Fatalf("Read32(3 GiB) = %#x, want 0xDEADBEEF (mmap demand-paged commit failed)", got)
	}

	const lowAddr uint32 = 0x1000
	if got := bus.Read32(lowAddr); got != 0 {
		t.Fatalf("Read32(0x1000) = %#x, want 0 (high-addr write leaked into low memory)", got)
	}
}

func TestBus32_RoundTripNearTopOfBusMem_MmapDemandPaged(t *testing.T) {
	bus, err := NewMachineBusSized(busMemMaxBytes)
	if err != nil {
		t.Fatalf("NewMachineBusSized(busMemMaxBytes): %v", err)
	}

	// Last page below the cap. busMemMaxBytes = 0xFFFF0000; last 4-byte
	// slot in-bounds is 0xFFFEFFFC.
	addr := uint32(busMemMaxBytes - 4)
	bus.Write32(addr, 0xCAFEBABE)
	if got := bus.Read32(addr); got != 0xCAFEBABE {
		t.Fatalf("Read32(top-of-bus) = %#x, want 0xCAFEBABE", got)
	}
}
