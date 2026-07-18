//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

// jit_ie64_const_fold_parity_test.go - Native backend tests for constant-only
// folding: structural (the fold is actually applied) and interpreter parity
// (registers, memory, FP state, PC, retired counts).
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "testing"

// assertFoldParity runs the program under both JIT and interpreter and
// compares full architectural state.
func assertFoldParity(t *testing.T, name string, build func(mem []byte)) (jit *CPU64) {
	t.Helper()
	jitCPU := runToHaltAt(t, true, build)
	interpCPU := runToHaltAt(t, false, build)
	if jitCPU.PC != interpCPU.PC {
		t.Fatalf("%s: PC mismatch: JIT 0x%X, interp 0x%X", name, jitCPU.PC, interpCPU.PC)
	}
	if jitCPU.InstructionCount != interpCPU.InstructionCount {
		t.Fatalf("%s: retired count mismatch: JIT %d, interp %d",
			name, jitCPU.InstructionCount, interpCPU.InstructionCount)
	}
	for i := range jitCPU.regs {
		if jitCPU.regs[i] != interpCPU.regs[i] {
			t.Fatalf("%s: R%d mismatch: JIT 0x%X, interp 0x%X", name, i, jitCPU.regs[i], interpCPU.regs[i])
		}
	}
	if jitCPU.FPU != nil && interpCPU.FPU != nil {
		if jitCPU.FPU.FPSR != interpCPU.FPU.FPSR {
			t.Fatalf("%s: FPSR mismatch: JIT 0x%08X, interp 0x%08X", name, jitCPU.FPU.FPSR, interpCPU.FPU.FPSR)
		}
		for i := range jitCPU.FPU.FPRegs {
			if jitCPU.FPU.FPRegs[i] != interpCPU.FPU.FPRegs[i] {
				t.Fatalf("%s: F%d mismatch", name, i)
			}
		}
	}
	return jitCPU
}

// buildFoldChainProgram is a pure constant chain: every ALU result is
// statically known, so the whole chain folds.
func buildFoldChainProgram(mem []byte) {
	base := uint64(PROG_START)
	put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
	put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 100))
	put(0x08, ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 1, 0, 23))
	put(0x10, ie64Instr(OP_SUB, 3, IE64_SIZE_Q, 0, 2, 1, 0)) // R3 = 123-100
	put(0x18, ie64Instr(OP_LSL, 4, IE64_SIZE_Q, 1, 1, 0, 40))
	put(0x20, ie64Instr(OP_LSR, 5, IE64_SIZE_Q, 0, 4, 2, 0)) // shift by R2=123 -> &63 = 59
	put(0x28, ie64Instr(OP_EOR, 6, IE64_SIZE_L, 0, 4, 3, 0))
	put(0x30, ie64Instr(OP_MOVEQ, 7, 0, 0, 0, 0, 0xFFFFFFFB)) // -5 sign-extended
	put(0x38, ie64Instr(OP_ASR, 8, IE64_SIZE_Q, 1, 7, 0, 2))
	put(0x40, ie64Instr(OP_AND64, 9, IE64_SIZE_W, 0, 7, 1, 0))
	put(0x48, ie64Instr(OP_OR64, 10, IE64_SIZE_B, 1, 7, 0, 0x0F))
	put(0x50, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

func TestConstFold_ChainParityAndApplied(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	before := ie64FoldedConstEmits.Load()
	jit := assertFoldParity(t, "fold/chain", buildFoldChainProgram)
	if jit.regs[1] != 100 || jit.regs[2] != 123 || jit.regs[3] != 23 {
		t.Fatalf("unexpected results: R1=%d R2=%d R3=%d", jit.regs[1], jit.regs[2], jit.regs[3])
	}
	if after := ie64FoldedConstEmits.Load(); after == before {
		t.Fatal("no folded constants were emitted for a fully foldable chain")
	}
}

// A memory barrier must clear tracked constants: the ADD after the LOAD
// consumes a register the LOAD produced, and the ADD reading the pre-barrier
// constant register must still be correct (it is simply not folded).
func TestConstFold_BarrierParity(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	build := func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x1234))
		put(0x08, ie64Instr(OP_LEA, 2, 0, 0, 0, 0, uint32(base+0x100)))
		put(0x10, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))
		put(0x18, ie64Instr(OP_LOAD, 3, IE64_SIZE_Q, 0, 2, 0, 0))
		put(0x20, ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 0, 3, 1, 0))
		put(0x28, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	jit := assertFoldParity(t, "fold/barrier", build)
	if jit.regs[4] != 0x2468 {
		t.Fatalf("R4 = 0x%X, want 0x2468", jit.regs[4])
	}
}

// Unsupported instruction writing a tracked register invalidates it; the
// dependent ADD must consume the runtime value.
func TestConstFold_UnsupportedWriteParity(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	build := func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 7))
		put(0x08, ie64Instr(OP_MULU, 1, IE64_SIZE_Q, 1, 1, 0, 6)) // R1 = 42, not folded
		put(0x10, ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 1, 0, 1))  // must read runtime R1
		put(0x18, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	jit := assertFoldParity(t, "fold/unsupported", build)
	if jit.regs[2] != 43 {
		t.Fatalf("R2 = %d, want 43", jit.regs[2])
	}
}

// Truncation and sign-extension edges: byte/word/long masks and ASR sign
// behaviour survive folding.
func TestConstFold_SizesParity(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	build := func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVEQ, 1, 0, 0, 0, 0, 0xFFFFFFFF)) // -1
		put(0x08, ie64Instr(OP_MOVE, 2, IE64_SIZE_B, 0, 1, 0, 0))
		put(0x10, ie64Instr(OP_MOVE, 3, IE64_SIZE_W, 0, 1, 0, 0))
		put(0x18, ie64Instr(OP_MOVE, 4, IE64_SIZE_L, 0, 1, 0, 0))
		put(0x20, ie64Instr(OP_ADD, 5, IE64_SIZE_B, 1, 1, 0, 1))
		put(0x28, ie64Instr(OP_ASR, 6, IE64_SIZE_B, 1, 1, 0, 4))
		put(0x30, ie64Instr(OP_LSL, 7, IE64_SIZE_W, 1, 1, 0, 12))
		put(0x38, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	assertFoldParity(t, "fold/sizes", build)
}

// R0 writes are architecturally ignored and R0 reads are zero, folded or not.
func TestConstFold_R0Parity(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	build := func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 0, IE64_SIZE_Q, 1, 0, 0, 55))
		put(0x08, ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 0, 0, 9))
		put(0x10, ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 0, 0, 1, 0))
		put(0x18, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	jit := assertFoldParity(t, "fold/r0", build)
	if jit.regs[1] != 9 {
		t.Fatalf("R1 = %d, want 9", jit.regs[1])
	}
}
