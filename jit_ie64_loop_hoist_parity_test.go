//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

// jit_ie64_loop_hoist_parity_test.go - Native backend tests for
// single-instruction loop hoisting: structural (the hoist is applied) and
// interpreter parity including exact retired counts for the suppressed
// guest instruction.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "testing"

// buildHoistLoopProgram: counted loop with one loop-invariant MOVE inside.
// R2 counts down from n; R3 is the invariant; R4 accumulates.
func buildHoistLoopProgram(n uint32) func(mem []byte) {
	return func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, n))
		put(0x08, ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 1, 0, 0, 0x123)) // invariant
		put(0x10, ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 0, 4, 3, 0))
		put(0x18, ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1))
		put(0x20, ie64Instr(OP_BNE, 0, 0, 0, 2, 0, ^uint32(0x17))) // -> 0x08
		put(0x28, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
}

func TestLoopHoist_ParityAndApplied(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	before := ie64LoopHoistEmits.Load()
	jit := assertFoldParity(t, "hoist/basic", buildHoistLoopProgram(1000))
	if ie64LoopHoistEmits.Load() == before {
		t.Fatal("invariant loop instruction was not hoisted")
	}
	if jit.regs[4] != 1000*0x123 {
		t.Fatalf("R4 = 0x%X, want 0x%X", jit.regs[4], uint64(1000*0x123))
	}
}

// Budget exit: the loop is long enough to cross the JIT loop budget, so the
// block exits mid-loop and re-enters. Retired counts must still match the
// interpreter exactly, including every suppressed guest instruction.
func TestLoopHoist_BudgetExitCounting(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	assertFoldParity(t, "hoist/budget", buildHoistLoopProgram(50000))
}

// Changed operands: the candidate reads a register the loop writes, so
// nothing is hoisted and results still match.
func TestLoopHoist_NonHoistableControl(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	build := func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 1000))
		put(0x08, ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 0, 4, 0, 0)) // reads loop-varying R4
		put(0x10, ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 1, 4, 0, 5))
		put(0x18, ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1))
		put(0x20, ie64Instr(OP_BNE, 0, 0, 0, 2, 0, ^uint32(0x17)))
		put(0x28, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	before := ie64LoopHoistEmits.Load()
	jit := assertFoldParity(t, "hoist/control", build)
	if ie64LoopHoistEmits.Load() != before {
		t.Fatal("loop-varying candidate must not be hoisted")
	}
	if jit.regs[3] != 5000-5 {
		t.Fatalf("R3 = %d, want %d", jit.regs[3], 5000-5)
	}
}

// The hoisted instruction's destination consumed after the loop.
func TestLoopHoist_DestLiveAfterLoop(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	build := func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 10))
		put(0x08, ie64Instr(OP_LEA, 3, 0, 0, 0, 0, ^uint32(15))) // R3 = -16, invariant
		put(0x10, ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 0, 4, 3, 0))
		put(0x18, ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1))
		put(0x20, ie64Instr(OP_BNE, 0, 0, 0, 2, 0, ^uint32(0x17)))
		put(0x28, ie64Instr(OP_ADD, 5, IE64_SIZE_Q, 0, 3, 3, 0)) // uses R3 after loop
		put(0x30, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	jit := assertFoldParity(t, "hoist/live-after", build)
	want := uint64(0xFFFFFFFFFFFFFFF0)
	if jit.regs[3] != want || jit.regs[5] != want+want {
		t.Fatalf("R3 = 0x%X R5 = 0x%X", jit.regs[3], jit.regs[5])
	}
}

// buildHoistChainProgram: counted loop containing a dependent invariant
// chain (MOVE then LSL of its result) consumed by a varying accumulator.
// R2 counts down from n; R3 and R6 are the chain; R4 accumulates.
func buildHoistChainProgram(n uint32) func(mem []byte) {
	return func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, n))
		put(0x08, ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 1, 0, 0, 0x123)) // invariant
		put(0x10, ie64Instr(OP_LSL, 6, IE64_SIZE_Q, 1, 3, 0, 4))      // invariant chain: R3<<4
		put(0x18, ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 0, 4, 6, 0))
		put(0x20, ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1))
		put(0x28, ie64Instr(OP_BNE, 0, 0, 0, 2, 0, ^uint32(0x1f))) // -> 0x08
		put(0x30, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
}

// A dependent invariant chain hoists entirely and matches the interpreter.
func TestLoopHoist_ChainParityAndApplied(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	before := ie64LoopHoistEmits.Load()
	jit := assertFoldParity(t, "hoist/chain", buildHoistChainProgram(1000))
	if ie64LoopHoistEmits.Load() == before {
		t.Fatal("invariant chain was not hoisted")
	}
	if jit.regs[4] != 1000*(0x123<<4) {
		t.Fatalf("R4 = 0x%X, want 0x%X", jit.regs[4], uint64(1000*(0x123<<4)))
	}
}

// Budget exit with two suppressed guest instructions per iteration: retired
// counts must match the interpreter exactly across mid-loop exits.
func TestLoopHoist_ChainBudgetExitCounting(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	assertFoldParity(t, "hoist/chain-budget", buildHoistChainProgram(50000))
}
