// jit_ie64_const_fold_aggressive_test.go - Analysis tests for the aggressive
// constant-folding extensions: relaxed memory barriers and the full
// pure-integer opcode whitelist.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "testing"

// STORE reads registers and writes memory only; tracked constants survive.
func TestIE64ConstFold_StoreKeepsConstants(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 5, 0),
		foldIns(OP_STORE, 1, 2, 0, IE64_SIZE_Q, 0, 0, 1),
		foldIns(OP_ADD, 3, 1, 0, IE64_SIZE_Q, 1, 3, 2),
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 0, 5)
	requireFold(t, f, 2, 8)
}

// LOAD invalidates its destination only; unrelated constants survive.
func TestIE64ConstFold_LoadClearsOnlyRd(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 5, 0),
		foldIns(OP_LOAD, 4, 2, 0, IE64_SIZE_Q, 0, 0, 1),
		foldIns(OP_ADD, 3, 1, 0, IE64_SIZE_Q, 1, 2, 2), // R1 survives -> 7
		foldIns(OP_ADD, 5, 4, 0, IE64_SIZE_Q, 1, 1, 3), // R4 loaded -> unknown
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 0, 5)
	requireFold(t, f, 2, 7)
	requireNoFold(t, f, 3)
}

// LOAD into a register holding a tracked constant kills that constant.
func TestIE64ConstFold_LoadDestInvalidatesConstant(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 5, 0),
		foldIns(OP_LOAD, 1, 2, 0, IE64_SIZE_Q, 0, 0, 1),
		foldIns(OP_ADD, 3, 1, 0, IE64_SIZE_Q, 1, 1, 2),
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 0, 5)
	requireNoFold(t, f, 2)
}

// Extended whitelist: multiply family with size truncation and the exact
// interpreter divide-by-zero result (zero, no trap).
func TestIE64ConstFold_MulDivMod(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 100, 0),
		foldIns(OP_MULU, 2, 1, 0, IE64_SIZE_Q, 1, 7, 1),  // 700
		foldIns(OP_MULU, 3, 1, 0, IE64_SIZE_B, 1, 7, 2),  // 700&0xFF = 0xBC
		foldIns(OP_DIVU, 4, 1, 0, IE64_SIZE_Q, 1, 7, 3),  // 14
		foldIns(OP_DIVU, 5, 1, 0, IE64_SIZE_Q, 1, 0, 4),  // div by zero -> 0
		foldIns(OP_MOD64, 6, 1, 0, IE64_SIZE_Q, 1, 7, 5), // 2
		foldIns(OP_MOD64, 7, 1, 0, IE64_SIZE_Q, 1, 0, 6), // mod by zero -> 0
		foldIns(OP_MOVEQ, 8, 0, 0, 0, 0, -5, 7),
		foldIns(OP_MULS, 9, 8, 0, IE64_SIZE_Q, 1, 3, 8),   // -15
		foldIns(OP_DIVS, 10, 8, 0, IE64_SIZE_Q, 1, 0, 9),  // 0
		foldIns(OP_MODS, 11, 8, 0, IE64_SIZE_Q, 1, 3, 10), // -5 % 3 = -2
		foldIns(OP_DIVS, 12, 8, 0, IE64_SIZE_Q, 1, 2, 11), // -5 / 2 = -2 (trunc)
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 1, 700)
	requireFold(t, f, 2, 0xBC)
	requireFold(t, f, 3, 14)
	requireFold(t, f, 4, 0)
	requireFold(t, f, 5, 2)
	requireFold(t, f, 6, 0)
	requireFold(t, f, 8, ^uint64(14))
	requireFold(t, f, 9, 0)
	requireFold(t, f, 10, ^uint64(1))
	requireFold(t, f, 11, ^uint64(1))
}

// MULHU/MULHS write the full 64-bit high half, no size masking.
func TestIE64ConstFold_MulHigh(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVEQ, 1, 0, 0, 0, 0, -1, 0),               // 0xFFFF...F
		foldIns(OP_MULHU, 2, 1, 1, IE64_SIZE_B, 0, 0, 1),      // hi(-1 * -1) = 0xFFFF...E, size ignored
		foldIns(OP_MULHS, 3, 1, 1, IE64_SIZE_Q, 0, 0, 2),      // hi_signed(-1 * -1) = 0
		foldIns(OP_MOVE, 4, 0, 0, IE64_SIZE_Q, 1, 0, 3),       // 0
		foldIns(OP_MULHU, 5, 4, 0, IE64_SIZE_Q, 1, 0xFFFF, 4), // 0
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 1, 0xFFFFFFFFFFFFFFFE)
	requireFold(t, f, 2, 0)
	requireFold(t, f, 4, 0)
}

// MOVT is a read-modify-write of the destination: folds only when the
// destination is already known.
func TestIE64ConstFold_MOVT(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 0x12345678, 0),
		foldIns(OP_MOVT, 1, 0, 0, 0, 0, -559087615, 1),
		foldIns(OP_MOVT, 9, 0, 0, 0, 0, 0x1, 2), // R9 unknown -> no fold, invalidated
		foldIns(OP_ADD, 2, 9, 0, IE64_SIZE_Q, 1, 1, 3),
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 1, 0xDEAD000112345678)
	requireNoFold(t, f, 2)
	requireNoFold(t, f, 3)
}

// Unary and bit-manipulation set: NEG, NOT64, CLZ/CTZ/POPCNT/BSWAP (32-bit
// semantics), SEXT and rotates.
func TestIE64ConstFold_UnaryAndBitOps(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 0x00F0, 0),
		foldIns(OP_NEG, 2, 1, 0, IE64_SIZE_Q, 0, 0, 1),   // -0xF0
		foldIns(OP_NEG, 3, 1, 0, IE64_SIZE_B, 0, 0, 2),   // (-0xF0)&0xFF = 0x10
		foldIns(OP_NOT64, 4, 1, 0, IE64_SIZE_W, 0, 0, 3), // ^0xF0 & 0xFFFF = 0xFF0F
		foldIns(OP_CLZ, 5, 1, 0, 0, 0, 0, 4),             // LeadingZeros32(0xF0) = 24
		foldIns(OP_CTZ, 6, 1, 0, 0, 0, 0, 5),             // TrailingZeros32(0xF0) = 4
		foldIns(OP_POPCNT, 7, 1, 0, 0, 0, 0, 6),          // 4
		foldIns(OP_BSWAP, 8, 1, 0, 0, 0, 0, 7),           // ReverseBytes32(0xF0) = 0xF0000000
		foldIns(OP_MOVE, 9, 0, 0, IE64_SIZE_Q, 1, 0x80, 8),
		foldIns(OP_SEXT, 10, 9, 0, IE64_SIZE_B, 0, 0, 9),  // 0xFFFFFFFFFFFFFF80
		foldIns(OP_SEXT, 11, 9, 0, IE64_SIZE_Q, 0, 0, 10), // pass-through
		foldIns(OP_ROL, 12, 9, 0, IE64_SIZE_B, 1, 1, 11),  // rot8(0x80,1) = 0x01
		foldIns(OP_ROR, 13, 9, 0, IE64_SIZE_B, 1, 1, 12),  // ror8(0x80,1) = 0x40
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 1, ^uint64(0xEF))
	requireFold(t, f, 2, 0x10)
	requireFold(t, f, 3, 0xFF0F)
	requireFold(t, f, 4, 24)
	requireFold(t, f, 5, 4)
	requireFold(t, f, 6, 4)
	requireFold(t, f, 7, 0xF0000000)
	requireFold(t, f, 9, 0xFFFFFFFFFFFFFF80)
	requireFold(t, f, 10, 0x80)
	requireFold(t, f, 11, 0x01)
	requireFold(t, f, 12, 0x40)
}

// Constants survive a whole LOAD/STORE sequence: the common real-code shape
// the conservative barrier rules could never fold through.
func TestIE64ConstFold_MemoryHeavySequence(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 0x10, 0),
		foldIns(OP_LOAD, 4, 2, 0, IE64_SIZE_L, 0, 0x100, 1),
		foldIns(OP_STORE, 4, 2, 0, IE64_SIZE_L, 0, 0x200, 2),
		foldIns(OP_LOAD, 5, 2, 0, IE64_SIZE_L, 0, 0x104, 3),
		foldIns(OP_STORE, 5, 2, 0, IE64_SIZE_L, 0, 0x204, 4),
		foldIns(OP_LSL, 6, 1, 0, IE64_SIZE_Q, 1, 2, 5), // 0x10<<2 = 0x40
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 0, 0x10)
	requireFold(t, f, 5, 0x40)
}
