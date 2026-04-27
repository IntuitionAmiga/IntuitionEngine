// x86_address_mask_test.go - PLAN_MAX_RAM.md slice 8 phase 4.
//
// Asserts the x86 CPU is a flat 32-bit guest. The legacy 25-bit
// x86AddressMask = 0x01FFFFFF aliased addresses above 32 MiB into low
// memory; the plan retires that mask so x86 sees its full 32-bit
// architectural visible range.

package main

import "testing"

func TestX86_AddressMask_NoAliasingAt32MiBBoundary(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)

	// Sentinel at low addr 0; TestX86Bus returns 0 for out-of-range reads.
	bus.memory[0] = 0xAB

	// With the legacy 25-bit mask, addr 0x02000000 wraps to 0 and returns
	// the sentinel byte. With the mask retired, the read goes out of
	// the 1 MiB test bus and returns 0.
	got := cpu.read8(0x02000000)
	if got == 0xAB {
		t.Fatalf("read8(0x02000000) returned sentinel 0xAB — 25-bit mask is still aliasing high addresses into low memory")
	}
	if got != 0 {
		t.Fatalf("read8(0x02000000) = 0x%02X, want 0 (out-of-bus)", got)
	}
}

func TestX86_AddressMask_NoAliasingNear4GiBTop(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	bus.memory[0] = 0xCD

	// Near top-of-32-bit. With the legacy mask, 0xFE000004 wraps to a
	// low-window addr and could return the sentinel; without it the
	// read goes to the empty bus.
	got := cpu.read8(0xFE000004)
	if got == 0xCD {
		t.Fatalf("read8(0xFE000004) returned sentinel — high-address aliasing")
	}
}

func TestX86_AddressMask_PreservesLowMemoryReads(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	bus.memory[0x100] = 0xEF

	if got := cpu.read8(0x100); got != 0xEF {
		t.Fatalf("read8(0x100) = 0x%02X, want 0xEF", got)
	}
}
