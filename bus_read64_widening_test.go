// bus_read64_widening_test.go - PLAN_MAX_RAM slice 10d.
//
// Pins that 64-bit data / 32-bit address bus operations widen automatically
// with bus.memory growth: their bounds checks read len(bus.memory) directly
// (no hardcoded DEFAULT_MEMORY_SIZE constant on the non-aliased fast path).
// Sign-extended aliasing paths intentionally retain DEFAULT_MEMORY_SIZE
// gating so M68K's pre-decrement-from-zero idiom keeps producing wrap-
// behaviour identical to a 24-bit 68000.

package main

import "testing"

func TestBus_Read64WriteAt64MiB_HitsGrownMemory(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	const addr uint32 = 64 * 1024 * 1024
	const want uint64 = 0xDEADBEEFCAFEBABE
	bus.Write64(addr, want)
	if got := bus.Read64(addr); got != want {
		t.Fatalf("Read64(64 MiB) = 0x%X, want 0x%X", got, want)
	}
}

func TestBus_Read64WriteWithFaultAt64MiB_HitsGrownMemory(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	const addr uint32 = 64 * 1024 * 1024
	const want uint64 = 0x1122334455667788
	if !bus.Write64WithFault(addr, want) {
		t.Fatalf("Write64WithFault(64 MiB) returned false; expected success after bus.memory growth")
	}
	got, ok := bus.Read64WithFault(addr)
	if !ok {
		t.Fatalf("Read64WithFault(64 MiB) returned ok=false; expected success")
	}
	if got != want {
		t.Fatalf("Read64WithFault = 0x%X, want 0x%X", got, want)
	}
}

// TestBus_Read64WriteSignExtendedAliasing_StillBounded keeps the M68K
// 24-bit-wrap aliasing semantics intact: 0xFFFF0000 + n maps to (n &
// 0xFFFF), and the bounds check on that aliased address remains tied to
// DEFAULT_MEMORY_SIZE because the low-16-bit aliased space is the
// historical 24-bit ABI surface.
func TestBus_Read64WriteSignExtendedAliasing_StillBounded(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	// 0xFFFFFFE0 sign-extends to mapped = 0xFFE0 — a valid low-16-bit RAM
	// address. Round-trip should still work even after bus.memory growth
	// because the aliased path drops to the low window.
	const aliased uint32 = 0xFFFFFFE0
	const want uint32 = 0xCAFEBABE
	bus.Write32(aliased, want)
	if got := bus.Read32(aliased); got != want {
		t.Fatalf("aliased Read32 = 0x%X, want 0x%X (sign-extended path must keep working)", got, want)
	}
}
