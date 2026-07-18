//go:build !js

// jit_wasm_const_fold_test.go - Wasm backend tests for constant-only folding:
// structural (folded emission occurs during translation) and differential
// parity against the interpreter under wazero.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "testing"

func wasmFoldProgram() []byte {
	var p []byte
	add := func(ins []byte) { p = append(p, ins...) }
	add(ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 100))
	add(ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 1, 0, 23))
	add(ie64Instr(OP_SUB, 3, IE64_SIZE_Q, 0, 2, 1, 0))
	add(ie64Instr(OP_LSL, 4, IE64_SIZE_Q, 1, 1, 0, 40))
	add(ie64Instr(OP_EOR, 5, IE64_SIZE_L, 0, 4, 3, 0))
	add(ie64Instr(OP_MOVEQ, 6, 0, 0, 0, 0, 0xFFFFFFFB))
	add(ie64Instr(OP_ASR, 7, IE64_SIZE_Q, 1, 6, 0, 2))
	add(ie64Instr(OP_AND64, 8, IE64_SIZE_W, 0, 6, 1, 0))
	return p
}

func TestWasmConstFold_Parity(t *testing.T) {
	program := wasmFoldProgram()
	before := ie64FoldedConstEmits.Load()
	res := runWasmDiffBlock(t, program, nil, nil)
	if ie64FoldedConstEmits.Load() == before {
		t.Fatal("wasm translation emitted no folded constants for a foldable chain")
	}
	interp := runInterpDiff(t, program, nil)
	for i := 0; i < 32; i++ {
		if res.regs[i] != interp.regs[i] {
			t.Fatalf("R%d mismatch: wasm 0x%X, interp 0x%X", i, res.regs[i], interp.regs[i])
		}
	}
}

// A barrier (STORE) mid-block plus an unsupported write must leave the
// downstream consumers on runtime values, and results must still match.
func TestWasmConstFold_BarrierAndInvalidationParity(t *testing.T) {
	var p []byte
	add := func(ins []byte) { p = append(p, ins...) }
	add(ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x1234))
	add(ie64Instr(OP_LEA, 2, 0, 0, 0, 0, uint32(PROG_START+0x200)))
	add(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))
	add(ie64Instr(OP_LOAD, 3, IE64_SIZE_Q, 0, 2, 0, 0))
	add(ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 0, 3, 1, 0))
	add(ie64Instr(OP_MULU, 1, IE64_SIZE_Q, 1, 1, 0, 2))
	add(ie64Instr(OP_ADD, 5, IE64_SIZE_Q, 1, 1, 0, 1))
	res := runWasmDiffBlock(t, p, nil, nil)
	interp := runInterpDiff(t, p, nil)
	for i := 0; i < 32; i++ {
		if res.regs[i] != interp.regs[i] {
			t.Fatalf("R%d mismatch: wasm 0x%X, interp 0x%X", i, res.regs[i], interp.regs[i])
		}
	}
	if res.regs[4] != 0x2468 || res.regs[5] != 0x2469 {
		t.Fatalf("R4=0x%X R5=0x%X, want 0x2468/0x2469", res.regs[4], res.regs[5])
	}
}
