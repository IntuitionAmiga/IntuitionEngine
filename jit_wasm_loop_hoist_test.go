//go:build !js

// jit_wasm_loop_hoist_test.go - Wasm backend tests for single-instruction
// loop hoisting: the invariant is emitted before the structured loop opens,
// the suppressed guest instruction stays in the retired count, and the exit
// state is exact.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "testing"

func TestWasmLoopHoist_StructuredLoopAndCounting(t *testing.T) {
	var p []byte
	add := func(ins []byte) { p = append(p, ins...) }
	add(ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 1000))
	add(ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 1, 0, 0, 0x123)) // invariant, loop head
	add(ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 0, 4, 3, 0))
	add(ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1))
	add(ie64Instr(OP_BNE, 0, 0, 0, 2, 0, ^uint32(0x17))) // -> instr 1
	before := ie64LoopHoistEmits.Load()
	res := runWasmDiffBlock(t, p, nil, nil)
	if ie64LoopHoistEmits.Load() == before {
		t.Fatal("wasm translation did not hoist the invariant")
	}
	// This is a bounded counter loop within the retire budget, so the
	// structured loop runs to completion in one call. The retired count
	// must include the suppressed invariant on every iteration:
	// 1 prefix + 1000 iterations x 4 body instructions = 4001.
	if res.retPC != PROG_START+40 || res.retCount != 4001 {
		t.Fatalf("retPC=%#x retCount=%d, want %#x/4001", res.retPC, res.retCount, uint64(PROG_START+40))
	}
	if res.regs[2] != 0 || res.regs[3] != 0x123 || res.regs[4] != 0x123*1000 {
		t.Fatalf("R2=%d R3=0x%X R4=0x%X, want 0/0x123/0x%X", res.regs[2], res.regs[3], res.regs[4], uint64(0x123*1000))
	}
}

// A dependent invariant chain hoists entirely under wasm; both suppressed
// guest instructions stay in the retired count on every iteration.
func TestWasmLoopHoist_ChainStructuredLoopAndCounting(t *testing.T) {
	var p []byte
	add := func(ins []byte) { p = append(p, ins...) }
	add(ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 800))
	add(ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 1, 0, 0, 0x123)) // invariant, loop head
	add(ie64Instr(OP_LSL, 6, IE64_SIZE_Q, 1, 3, 0, 4))      // invariant chain
	add(ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 0, 4, 6, 0))
	add(ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1))
	add(ie64Instr(OP_BNE, 0, 0, 0, 2, 0, ^uint32(0x1f))) // -> instr 1
	before := ie64LoopHoistEmits.Load()
	res := runWasmDiffBlock(t, p, nil, nil)
	if ie64LoopHoistEmits.Load() == before {
		t.Fatal("wasm translation did not hoist the invariant chain")
	}
	// Bounded counter loop within the retire budget: 1 prefix + 800
	// iterations x 5 body instructions = 4001, completing in one call.
	if res.retPC != PROG_START+48 || res.retCount != 4001 {
		t.Fatalf("retPC=%#x retCount=%d, want %#x/4001", res.retPC, res.retCount, uint64(PROG_START+48))
	}
	if res.regs[2] != 0 || res.regs[6] != 0x123<<4 || res.regs[4] != 800*(0x123<<4) {
		t.Fatalf("R2=%d R6=0x%X R4=0x%X", res.regs[2], res.regs[6], res.regs[4])
	}
}
