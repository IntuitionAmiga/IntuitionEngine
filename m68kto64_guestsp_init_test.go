// m68kto64_guestsp_init_test.go
//
// Pins the boot invariant that m68kto64-transpiled programs need: r30
// (GuestSP / m68k a7) must be initialised to a valid stack top before
// the first `bsr` so the return address store doesn't wrap into the
// negative-address space.
//
// Failure mode without this guarantee: the first `bsr ...` decrements
// r30 from 0 to $FFFFFFFFFFFFFFFC, stores the return PC at that
// address (host memory wraps or traps), then a later `jmp (r17)`
// returns to garbage. Game logic appears to "run" but control flow
// silently corrupts and the game never reaches its render loop. This
// is what was masquerading as a perf problem in the AB3D64 port.

package main

import "testing"

// TestGuestSP_InitializedAtReset asserts r30 is non-zero after CPU64
// reset. The transpiler convention requires it to point at a sensible
// guest stack top so a bare `bsr` lowering can push a return PC
// without wrapping past zero.
func TestGuestSP_InitializedAtReset(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	if cpu.regs[30] == 0 {
		t.Fatalf("r30 (GuestSP) is zero after Reset; m68kto64-lowered bsr will wrap the first push to $FFFFFFFFFFFFFFFC")
	}
	// Reasonable bounds: must be inside the legacy bus.memory window so
	// the push lands somewhere the load path can read back.
	if uint64(cpu.regs[30]) > uint64(len(cpu.memory)) {
		t.Errorf("r30 = %#x is beyond cpu.memory size %#x", cpu.regs[30], len(cpu.memory))
	}
	if uint64(cpu.regs[30]) < uint64(STACK_START) {
		t.Errorf("r30 = %#x is below STACK_START %#x; first bsr will collide with program window",
			cpu.regs[30], STACK_START)
	}
}

// TestGuestSP_BsrPushDoesNotWrap exercises the actual m68kto64 bsr
// skeleton: sub.l r30, r30, #4; la r17, retaddr; store.l r17, (r30);
// bra target. After the push, r30 must remain inside cpu.memory and
// the stored long must be readable at that address.
func TestGuestSP_BsrPushDoesNotWrap(t *testing.T) {
	src := `
		org $1000
test_entry:
		; Standard m68kto64 bsr skeleton.
		sub.l r30, r30, #4
		la r17, retaddr
		store.l r17, (r30)
		; Verify by loading back.
		load.l r1, (r30)
		halt
retaddr:
		halt
	`
	bin := assembleIE64(t, src)

	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.LoadProgramBytes(bin)
	cpu.PC = PROG_START

	for i := 0; i < 200; i++ {
		if cpu.memory[cpu.PC] == OP_HALT64 {
			break
		}
		cpu.StepOne()
	}

	// r30 must be inside cpu.memory after the push (-4 from initial).
	if uint64(cpu.regs[30]) >= uint64(len(cpu.memory)) {
		t.Errorf("after bsr-style push: r30 = %#x outside cpu.memory (size %#x). GuestSP not initialised.",
			cpu.regs[30], len(cpu.memory))
	}

	// The reloaded value (r1) must equal the la'd return address.
	// Source layout: 5 instructions (sub, la, store, load, halt) before
	// retaddr — so retaddr = $1000 + 5*8 = $1028.
	retaddrPC := uint64(PROG_START + 5*8)
	if cpu.regs[1] != retaddrPC {
		t.Errorf("reloaded return address = %#x, want %#x (push/load round-trip broken)",
			cpu.regs[1], retaddrPC)
	}
}
