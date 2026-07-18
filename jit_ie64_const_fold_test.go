// jit_ie64_const_fold_test.go - Shared constant-only folding analysis tests.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "testing"

func foldIns(op, rd, rs, rt, size, x byte, imm int32, idx int) JITInstr {
	return JITInstr{opcode: op, rd: rd, rs: rs, rt: rt, size: size, xbit: x, imm32: uint32(imm), pcOffset: uint32(idx * IE64_INSTR_SIZE)}
}

func requireFold(t *testing.T, f []ie64FoldEntry, idx int, want uint64) {
	t.Helper()
	if f == nil {
		t.Fatalf("fold slice is nil, want entry %d folded to 0x%X", idx, want)
	}
	if !f[idx].folded {
		t.Fatalf("entry %d not folded, want 0x%X", idx, want)
	}
	if f[idx].value != want {
		t.Fatalf("entry %d folded to 0x%X, want 0x%X", idx, f[idx].value, want)
	}
}

func requireNoFold(t *testing.T, f []ie64FoldEntry, idx int) {
	t.Helper()
	if f != nil && f[idx].folded {
		t.Fatalf("entry %d folded to 0x%X, want not folded", idx, f[idx].value)
	}
}

func TestIE64ConstFoldChain(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 100, 0),
		foldIns(OP_ADD, 2, 1, 0, IE64_SIZE_Q, 1, 23, 1),
		foldIns(OP_SUB, 3, 2, 1, IE64_SIZE_Q, 0, 0, 2),
		foldIns(OP_AND64, 4, 3, 0, IE64_SIZE_Q, 1, 0xF0, 3),
		foldIns(OP_OR64, 5, 4, 0, IE64_SIZE_Q, 1, 0x0F, 4),
		foldIns(OP_EOR, 6, 5, 0, IE64_SIZE_Q, 1, 0xFF, 5),
	}
	f := ie64AnalyseConstFold(ins, 0x1000)
	requireFold(t, f, 0, 100)
	requireFold(t, f, 1, 123)
	requireFold(t, f, 2, 23) // 123 - 100
	requireFold(t, f, 3, 23&0xF0)
	requireFold(t, f, 4, 23&0xF0|0x0F)
	requireFold(t, f, 5, (23&0xF0|0x0F)^0xFF)
}

func TestIE64ConstFoldNilWhenNothingFolds(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_ADD, 2, 5, 6, IE64_SIZE_Q, 0, 0, 0),
		foldIns(OP_MULU, 3, 2, 0, IE64_SIZE_Q, 1, 7, 1),
	}
	if f := ie64AnalyseConstFold(ins, 0x1000); f != nil {
		t.Fatalf("fold slice = %#v, want nil", f)
	}
}

func TestIE64ConstFoldSizesAndTruncation(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, -1, 0), // imm32 zero-extended then masked: 0xFFFFFFFF
		foldIns(OP_MOVE, 2, 1, 0, IE64_SIZE_B, 0, 0, 1),  // 0xFF
		foldIns(OP_MOVE, 3, 1, 0, IE64_SIZE_W, 0, 0, 2),  // 0xFFFF
		foldIns(OP_MOVE, 4, 1, 0, IE64_SIZE_L, 0, 0, 3),  // 0xFFFFFFFF
		foldIns(OP_ADD, 5, 1, 0, IE64_SIZE_B, 1, 1, 4),   // (0xFFFFFFFF+1)&0xFF = 0
		foldIns(OP_ADD, 6, 1, 0, IE64_SIZE_W, 1, 1, 5),   // & 0xFFFF = 0
		foldIns(OP_ADD, 7, 1, 0, IE64_SIZE_L, 1, 1, 6),   // & 0xFFFFFFFF = 0
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 0, 0xFFFFFFFF)
	requireFold(t, f, 1, 0xFF)
	requireFold(t, f, 2, 0xFFFF)
	requireFold(t, f, 3, 0xFFFFFFFF)
	requireFold(t, f, 4, 0)
	requireFold(t, f, 5, 0)
	requireFold(t, f, 6, 0)
}

func TestIE64ConstFoldSignExtension(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVEQ, 1, 0, 0, 0, 0, -5, 0),         // 0xFFFFFFFFFFFFFFFB
		foldIns(OP_LEA, 2, 1, 0, 0, 0, -3, 1),           // -5 + -3 = -8
		foldIns(OP_LEA, 3, 0, 0, 0, 0, -16, 2),          // R0 + -16
		foldIns(OP_SUB, 4, 0, 1, IE64_SIZE_Q, 0, 0, 3),  // 0 - (-5) = 5
		foldIns(OP_MOVEQ, 5, 0, 0, 0, 0, 0x7FFFFFFF, 4), // positive stays
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 0, 0xFFFFFFFFFFFFFFFB)
	requireFold(t, f, 1, 0xFFFFFFFFFFFFFFF8)
	requireFold(t, f, 2, 0xFFFFFFFFFFFFFFF0)
	requireFold(t, f, 3, 5)
	requireFold(t, f, 4, 0x7FFFFFFF)
}

func TestIE64ConstFoldShifts(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 1, 0),
		foldIns(OP_LSL, 2, 1, 0, IE64_SIZE_Q, 1, 63, 1), // 1<<63
		foldIns(OP_LSR, 3, 2, 0, IE64_SIZE_Q, 1, 63, 2), // back to 1
		foldIns(OP_MOVEQ, 4, 0, 0, 0, 0, -1, 3),         // all ones
		foldIns(OP_ASR, 5, 4, 0, IE64_SIZE_Q, 1, 40, 4), // stays all ones
		foldIns(OP_ASR, 6, 4, 0, IE64_SIZE_B, 1, 4, 5),  // int8(0xFF)>>4 = -1, mask B = 0xFF
		foldIns(OP_LSL, 7, 1, 0, IE64_SIZE_Q, 1, 64, 6), // shift 64&63 = 0 -> 1
		foldIns(OP_LSL, 8, 1, 0, IE64_SIZE_B, 1, 8, 7),  // (1<<8)&0xFF = 0
		foldIns(OP_LSR, 9, 4, 0, IE64_SIZE_B, 1, 4, 8),  // full-width shift then mask: 0xFF
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 1, 1<<63)
	requireFold(t, f, 2, 1)
	requireFold(t, f, 4, 0xFFFFFFFFFFFFFFFF)
	requireFold(t, f, 5, 0xFF)
	requireFold(t, f, 6, 1)
	requireFold(t, f, 7, 0)
	requireFold(t, f, 8, 0xFF)
}

func TestIE64ConstFoldR0(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_ADD, 1, 0, 0, IE64_SIZE_Q, 0, 0, 0),   // R0+R0 = 0
		foldIns(OP_MOVE, 0, 0, 0, IE64_SIZE_Q, 1, 55, 1), // write to R0 ignored, not folded
		foldIns(OP_ADD, 2, 0, 0, IE64_SIZE_Q, 1, 9, 2),   // R0 still 0 -> 9
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 0, 0)
	requireNoFold(t, f, 1)
	requireFold(t, f, 2, 9)
}

func TestIE64ConstFoldUnsupportedWriteInvalidates(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 7, 0),
		foldIns(OP_MULU, 1, 5, 6, IE64_SIZE_Q, 0, 0, 1), // unsupported write to R1
		foldIns(OP_ADD, 2, 1, 0, IE64_SIZE_Q, 1, 1, 2),  // R1 unknown -> no fold
		foldIns(OP_MOVE, 3, 0, 0, IE64_SIZE_Q, 1, 4, 3), // still folds after
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 0, 7)
	requireNoFold(t, f, 1)
	requireNoFold(t, f, 2)
	requireFold(t, f, 3, 4)
}

func TestIE64ConstFoldBarriers(t *testing.T) {
	mk := func(op JITInstr) []JITInstr {
		barrier := op
		barrier.pcOffset = uint32(IE64_INSTR_SIZE)
		return []JITInstr{
			foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 7, 0),
			barrier,
			foldIns(OP_ADD, 2, 1, 0, IE64_SIZE_Q, 1, 1, 2), // R1 cleared -> no fold
			foldIns(OP_ADD, 3, 0, 0, IE64_SIZE_Q, 1, 2, 3), // R0 restored -> folds
		}
	}
	cases := map[string]JITInstr{
		"memory-load":  foldIns(OP_LOAD, 4, 5, 0, IE64_SIZE_Q, 0, 0, 1),
		"memory-store": foldIns(OP_STORE, 4, 5, 0, IE64_SIZE_Q, 0, 0, 1),
		"fp":           foldIns(OP_FADD, 4, 5, 6, 0, 0, 0, 1),
		"fp64":         foldIns(OP_DADD, 4, 5, 6, 0, 0, 0, 1),
		"branch":       foldIns(OP_BEQ, 0, 5, 6, 0, 0, 512, 1),
		"push":         foldIns(OP_PUSH64, 0, 4, 0, 0, 0, 0, 1),
	}
	for name, barrier := range cases {
		t.Run(name, func(t *testing.T) {
			f := ie64AnalyseConstFold(mk(barrier), 0)
			requireFold(t, f, 0, 7)
			requireNoFold(t, f, 2)
			requireFold(t, f, 3, 2)
		})
	}
}

// A forward branch INTO the middle of the block must clear constants at the
// branch target: the target is reachable on a path that skips the setters.
func TestIE64ConstFoldBranchTargetClears(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_BEQ, 0, 5, 6, 0, 0, 3*IE64_INSTR_SIZE, 0), // target = instr 3
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 7, 1),
		foldIns(OP_ADD, 2, 1, 0, IE64_SIZE_Q, 1, 1, 2), // folds: only fall-through reaches it
		foldIns(OP_ADD, 3, 1, 0, IE64_SIZE_Q, 1, 2, 3), // branch target: R1 must be unknown
	}
	f := ie64AnalyseConstFold(ins, 0x2000)
	requireFold(t, f, 1, 7)
	requireFold(t, f, 2, 8)
	requireNoFold(t, f, 3)
}

func TestIE64ConstFoldFusedFlagBarrier(t *testing.T) {
	fused := foldIns(OP_NOP64, 0, 0, 0, 0, 0, 0, 1)
	fused.fusedFlag = ie64FusedJSRLeafCall
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 7, 0),
		fused,
		foldIns(OP_ADD, 2, 1, 0, IE64_SIZE_Q, 1, 1, 2),
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 0, 7)
	requireNoFold(t, f, 2)
}

func TestIE64ConstFoldRegOperandUnknownRt(t *testing.T) {
	ins := []JITInstr{
		foldIns(OP_MOVE, 1, 0, 0, IE64_SIZE_Q, 1, 7, 0),
		foldIns(OP_ADD, 2, 1, 9, IE64_SIZE_Q, 0, 0, 1), // rt=R9 unknown -> no fold, R2 invalidated
		foldIns(OP_ADD, 3, 2, 0, IE64_SIZE_Q, 1, 1, 2),
	}
	f := ie64AnalyseConstFold(ins, 0)
	requireFold(t, f, 0, 7)
	requireNoFold(t, f, 1)
	requireNoFold(t, f, 2)
}
