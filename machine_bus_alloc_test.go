package main

import "testing"

// TestBusAccess_ZeroAllocsSteadyState pins the non-I/O RAM access path at zero
// allocations. Guest execution drives millions of these per frame, so any
// allocation here is a per-instruction cost, not a one-off. The fast path is a
// bounds check and a slice index; this gate keeps it that way.
func TestBusAccess_ZeroAllocsSteadyState(t *testing.T) {
	bus := NewMachineBus()
	bus.Write32(0x1000, 0x12345678)

	read := testing.AllocsPerRun(1000, func() {
		_ = bus.Read32(0x1000)
		_ = bus.Read8(0x1000)
	})
	if read != 0 {
		t.Fatalf("RAM read path allocates %.0f times per run, want 0", read)
	}

	write := testing.AllocsPerRun(1000, func() {
		bus.Write32(0x1000, 0xCAFEBABE)
		bus.Write8(0x1004, 0x5A)
	})
	if write != 0 {
		t.Fatalf("RAM write path allocates %.0f times per run, want 0", write)
	}
}
