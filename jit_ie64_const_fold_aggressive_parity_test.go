//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

// jit_ie64_const_fold_aggressive_parity_test.go - Native backend parity tests
// for the aggressive folding extensions: constants surviving memory traffic
// and the extended pure-integer opcode whitelist.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "testing"

// Constants must survive a STORE/LOAD pair and still be folded afterwards:
// the LSL of the constant register after memory traffic is statically known.
func TestConstFold_SurvivesMemoryTraffic(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	build := func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x21))
		put(0x08, ie64Instr(OP_LEA, 2, 0, 0, 0, 0, uint32(base+0x200)))
		put(0x10, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))
		put(0x18, ie64Instr(OP_LOAD, 3, IE64_SIZE_Q, 0, 2, 0, 0))
		put(0x20, ie64Instr(OP_LSL, 4, IE64_SIZE_Q, 1, 1, 0, 4)) // 0x21<<4, folds
		put(0x28, ie64Instr(OP_ADD, 5, IE64_SIZE_Q, 0, 3, 4, 0)) // runtime + folded
		put(0x30, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	before := ie64FoldedConstEmits.Load()
	jit := assertFoldParity(t, "fold/memory-traffic", build)
	if jit.regs[4] != 0x210 || jit.regs[5] != 0x231 {
		t.Fatalf("R4=0x%X R5=0x%X, want 0x210/0x231", jit.regs[4], jit.regs[5])
	}
	if ie64FoldedConstEmits.Load() == before {
		t.Fatal("no folded constants emitted after memory traffic")
	}
}

// Extended whitelist parity: MOVT composition, multiply/divide family with
// the architectural divide-by-zero result, and the 32-bit bit-manipulation
// set, all fully folded, against the interpreter.
func TestConstFold_ExtendedOpsParity(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	build := func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x12345678))
		put(0x08, ie64Instr(OP_MOVT, 1, 0, 0, 0, 0, 0xDEAD0001))
		put(0x10, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 100))
		put(0x18, ie64Instr(OP_MULU, 3, IE64_SIZE_Q, 1, 2, 0, 7))
		put(0x20, ie64Instr(OP_DIVU, 4, IE64_SIZE_Q, 1, 2, 0, 0)) // div by zero -> 0
		put(0x28, ie64Instr(OP_MOVEQ, 5, 0, 0, 0, 0, 0xFFFFFFFB)) // -5
		put(0x30, ie64Instr(OP_DIVS, 6, IE64_SIZE_Q, 1, 5, 0, 2)) // -2
		put(0x38, ie64Instr(OP_MODS, 7, IE64_SIZE_Q, 1, 5, 0, 3)) // -2
		put(0x40, ie64Instr(OP_MULHS, 8, IE64_SIZE_Q, 0, 5, 5, 0))
		put(0x48, ie64Instr(OP_NEG, 9, IE64_SIZE_Q, 0, 5, 0, 0)) // 5
		put(0x50, ie64Instr(OP_NOT64, 10, IE64_SIZE_W, 0, 2, 0, 0))
		put(0x58, ie64Instr(OP_CLZ, 11, 0, 0, 2, 0, 0))
		put(0x60, ie64Instr(OP_CTZ, 12, 0, 0, 2, 0, 0))
		put(0x68, ie64Instr(OP_POPCNT, 13, 0, 0, 2, 0, 0))
		put(0x70, ie64Instr(OP_BSWAP, 14, 0, 0, 2, 0, 0))
		put(0x78, ie64Instr(OP_SEXT, 15, IE64_SIZE_B, 0, 2, 0, 0))
		put(0x80, ie64Instr(OP_ROL, 16, IE64_SIZE_B, 1, 2, 0, 3))
		put(0x88, ie64Instr(OP_ROR, 17, IE64_SIZE_B, 1, 2, 0, 3))
		put(0x90, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	before := ie64FoldedConstEmits.Load()
	jit := assertFoldParity(t, "fold/extended", build)
	if jit.regs[1] != 0xDEAD000112345678 || jit.regs[3] != 700 || jit.regs[4] != 0 || jit.regs[9] != 5 {
		t.Fatalf("R1=0x%X R3=%d R4=%d R9=%d", jit.regs[1], jit.regs[3], jit.regs[4], jit.regs[9])
	}
	if ie64FoldedConstEmits.Load() == before {
		t.Fatal("extended whitelist ops were not folded")
	}
}

// A LOAD into a register holding a tracked constant invalidates only that
// register; the dependent ADD must read the loaded runtime value.
func TestConstFold_LoadDestParity(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	build := func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x55))
		put(0x08, ie64Instr(OP_LEA, 2, 0, 0, 0, 0, uint32(base+0x200)))
		put(0x10, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))
		put(0x18, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 8)) // overwrites R1 with mem[+8]
		put(0x20, ie64Instr(OP_ADD, 3, IE64_SIZE_Q, 1, 1, 0, 1))  // must not fold
		put(0x28, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	jit := assertFoldParity(t, "fold/load-dest", build)
	if jit.regs[3] != 1 { // mem[+8] is zero
		t.Fatalf("R3 = %d, want 1", jit.regs[3])
	}
}
