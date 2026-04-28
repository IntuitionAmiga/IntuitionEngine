// x86_address_space_retired_test.go - PLAN_MAX_RAM slice 10g.
//
// Pins that x86 reaches addresses inside the bus.memory window — including
// 64 MiB on a 256 MiB-grown bus — and that the legacy 32 MiB x86Address-
// Space cap is no longer in force.

package main

import "testing"

// TestX86_LoadProgramDataRespectsBusMemory checks that LoadProgramData
// uses the bus-driven address space cap (which equals len(bus.memory))
// rather than the retired 32 MiB constant.
func TestX86_LoadProgramDataRespectsBusMemory(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	cfg := &CPUX86Config{LoadAddr: 0, Entry: 0}
	runner := NewCPUX86Runner(bus, cfg)
	// Allocate a buffer larger than the retired 32 MiB cap but smaller
	// than the grown bus.memory. Without slice-10g this would error
	// with "program too large".
	data := make([]byte, 64*1024*1024)
	if err := runner.LoadProgramData(data); err != nil {
		t.Fatalf("LoadProgramData(64 MiB): %v (slice 10g should accept up to bus.memory)", err)
	}
}

// TestX86_AccessAt64MiB_RoundTripsAfterCapRetired uses the bus directly
// (the x86 interpreter routes through the same MachineBus) to confirm a
// 64 MiB byte address round-trips on a grown bus.
func TestX86_AccessAt64MiB_RoundTripsAfterCapRetired(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	cfg := &CPUX86Config{LoadAddr: 0, Entry: 0}
	NewCPUX86Runner(bus, cfg) // wire adapter
	const addr uint32 = 64 * 1024 * 1024
	bus.Write32(addr, 0xCAFEBABE)
	if got := bus.Read32(addr); got != 0xCAFEBABE {
		t.Fatalf("bus.Read32(64 MiB) = 0x%X, want 0xCAFEBABE", got)
	}
}
