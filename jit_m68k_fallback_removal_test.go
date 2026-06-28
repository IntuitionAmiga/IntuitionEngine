//go:build amd64 && (linux || windows || darwin)

package main

// Tests for the M68K JIT fallback-removal conversion
// (M68K_JIT_FALLBACK_REMOVAL_PLAN.md): production fallback must execute exactly
// one interpreter instruction per unsupported/compile-failure event and return
// to JIT dispatch. Broad interpreter bursts are no longer a production path.

import "testing"

// TestM68KJIT_UnsupportedInstructionFallsBackOneInstruction verifies that a
// genuinely unsupported leading instruction (RESET, which has no native emitter)
// retires exactly one interpreter instruction and is classified as
// unsupported_one, while surrounding supported instructions still run native and
// final state matches the pure interpreter.
func TestM68KJIT_UnsupportedInstructionFallsBackOneInstruction(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	// RESET (0x4E70) is privileged, has no native emitter, and in supervisor
	// mode is a harmless cycle-only instruction in the interpreter. Three of
	// them force three consecutive unsupported_one fallbacks before a native
	// MOVEQ block. The helper appends STOP, which is itself unsupported_one.
	const startPC = 0x1000
	jit := runM68KJITStopProgramWithSetup(t, startPC, nil, false,
		0x4E70, // RESET
		0x4E70, // RESET
		0x4E70, // RESET
		0x7007, // MOVEQ #7,D0
	)

	if got := jit.DataRegs[0]; got != 7 {
		t.Fatalf("D0 = 0x%08X, want 0x7 (MOVEQ after unsupported instr did not run native)", got)
	}

	unsupportedOne := jit.m68kJitUnsupportedOneExits.Load()
	fallbackInstrs := jit.m68kJitFallbackInstructions.Load()
	nativeBlocks := jit.m68kJitNativeBlocksExecuted.Load()

	// 3 RESETs + STOP terminator = 4 unsupported_one fallbacks.
	if unsupportedOne != 4 {
		t.Fatalf("unsupported_one exits = %d, want 4", unsupportedOne)
	}
	// No bursts and no I/O bails: every fallback retired exactly one
	// instruction, so total fallback instructions equals unsupported_one count.
	if fallbackInstrs != unsupportedOne {
		t.Fatalf("fallback_instructions = %d, want == unsupported_one = %d (a burst or extra fallback ran)",
			fallbackInstrs, unsupportedOne)
	}
	if nativeBlocks == 0 {
		t.Fatal("native_blocks = 0, expected the MOVEQ block to execute natively")
	}

	// Interpreter-equivalence: same program through the pure interpreter must
	// reach the same architectural state.
	interp := newM68KTestProgramCPU(t, startPC)
	for i, op := range []uint16{0x4E70, 0x4E70, 0x4E70, 0x7007} {
		pc := uint32(startPC + i*2)
		interp.memory[pc] = byte(op >> 8)
		interp.memory[pc+1] = byte(op)
	}
	stopPC := uint32(startPC + 4*2)
	interp.memory[stopPC] = 0x4E
	interp.memory[stopPC+1] = 0x72
	interp.memory[stopPC+2] = 0x27
	interp.memory[stopPC+3] = 0x00
	interp.m68kJitEnabled = false
	runM68KInterpreterUntilStopped(t, interp)

	assertM68KCoreStateEqual(t, jit, interp)
}

// TestM68KJIT_NoProductionBurstFallback asserts that with default settings the
// dispatcher never retires more than one instruction per fallback event: the
// per-event unsupported_one counter must account for every fallback instruction.
// If the legacy burst path were active, fallback_instructions would exceed the
// sum of one-instruction exit counters.
func TestM68KJIT_NoProductionBurstFallback(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const startPC = 0x1000
	// A run of unsupported instructions with no native code between them. Under
	// burst fallback these would be retired in a single multi-instruction
	// sweep (unsupported_one == 0); under one-instruction fallback each is its
	// own exit.
	jit := runM68KJITStopProgramWithSetup(t, startPC, nil, false,
		0x4E70, 0x4E70, 0x4E70, 0x4E70, 0x4E70, // 5x RESET
	)

	unsupportedOne := jit.m68kJitUnsupportedOneExits.Load()
	mmioGuard := jit.m68kJitMMIOGuardExits.Load()
	compileFail := jit.m68kJitCompileFailureExits.Load()
	fallbackInstrs := jit.m68kJitFallbackInstructions.Load()

	if unsupportedOne == 0 {
		t.Fatal("unsupported_one = 0, expected one-instruction fallbacks (burst path active?)")
	}
	// Every fallback instruction must be accounted for by a single-instruction
	// exit category. No burst can inflate fallback_instructions beyond these.
	if fallbackInstrs != unsupportedOne+mmioGuard+compileFail {
		t.Fatalf("fallback_instructions = %d, want == unsupported_one(%d)+mmio_guard(%d)+compile_failure(%d) = %d",
			fallbackInstrs, unsupportedOne, mmioGuard, compileFail,
			unsupportedOne+mmioGuard+compileFail)
	}
}
