// jit_ie64_loop_hoist_test.go - Shared analysis tests for single-instruction
// loop hoisting.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "testing"

// Loop shape: seed counter, head at 1.
// 1: MOVE R3, #0x123   <- invariant candidate (immediate input)
// 2: ADD  R4, R4, R3   <- consumes it, not invariant (R4 written in loop)
// 3: SUB  R2, R2, #1
// 4: BNE  R2, R0, -> 1
func hoistLoop() []JITInstr {
	return []JITInstr{
		loopIns(OP_MOVE, 2, 0, 0, IE64_SIZE_Q, 1, 3000, 0),
		loopIns(OP_MOVE, 3, 0, 0, IE64_SIZE_Q, 1, 0x123, 1),
		loopIns(OP_ADD, 4, 4, 3, IE64_SIZE_Q, 0, 0, 2),
		loopIns(OP_SUB, 2, 2, 0, IE64_SIZE_Q, 1, 1, 3),
		loopIns(OP_BNE, 0, 2, 0, 0, 0, -24, 4),
	}
}

func TestIE64LoopHoistSelectsInvariant(t *testing.T) {
	p := ie64AnalyseLoop(hoistLoop(), 0x100)
	if p == nil {
		t.Fatal("pure integer loop with an invariant produced no plan")
	}
	if p.hoist != 1 {
		t.Fatalf("hoist = %d, want 1 (the invariant MOVE)", p.hoist)
	}
}

func TestIE64LoopHoistRejectsChangedOperands(t *testing.T) {
	ins := hoistLoop()
	// Make the candidate read a register the loop writes: MOVE R3, R4.
	ins[1] = loopIns(OP_MOVE, 3, 4, 0, IE64_SIZE_Q, 0, 0, 1)
	if p := ie64AnalyseLoop(ins, 0x100); p != nil && p.hoist >= 0 {
		t.Fatalf("hoist = %d, want none for loop-varying input", p.hoist)
	}
}

func TestIE64LoopHoistRejectsRepeatedDestinationWrites(t *testing.T) {
	ins := hoistLoop()
	// Second write to R3 inside the loop.
	ins[2] = loopIns(OP_ADD, 3, 3, 0, IE64_SIZE_Q, 1, 5, 2)
	if p := ie64AnalyseLoop(ins, 0x100); p != nil && p.hoist >= 0 {
		t.Fatalf("hoist = %d, want none for twice-written destination", p.hoist)
	}
}

func TestIE64LoopHoistRejectsDependentInvariantChain(t *testing.T) {
	ins := []JITInstr{
		loopIns(OP_MOVE, 2, 0, 0, IE64_SIZE_Q, 1, 3000, 0),
		loopIns(OP_MOVE, 3, 0, 0, IE64_SIZE_Q, 1, 0x123, 1),
		loopIns(OP_ADD, 4, 3, 0, IE64_SIZE_Q, 1, 7, 2), // depends on R3, written in loop
		loopIns(OP_SUB, 2, 2, 0, IE64_SIZE_Q, 1, 1, 3),
		loopIns(OP_BNE, 0, 2, 0, 0, 0, -24, 4),
	}
	p := ie64AnalyseLoop(ins, 0x100)
	if p == nil || p.hoist != 1 {
		t.Fatalf("plan=%+v, want only the first invariant hoisted", p)
	}
}

func TestIE64LoopHoistRejectsDestReadBeforeCandidate(t *testing.T) {
	ins := []JITInstr{
		loopIns(OP_MOVE, 2, 0, 0, IE64_SIZE_Q, 1, 3000, 0),
		loopIns(OP_ADD, 4, 4, 3, IE64_SIZE_Q, 0, 0, 1), // reads R3 before its write
		loopIns(OP_MOVE, 3, 0, 0, IE64_SIZE_Q, 1, 0x123, 2),
		loopIns(OP_SUB, 2, 2, 0, IE64_SIZE_Q, 1, 1, 3),
		loopIns(OP_BNE, 0, 2, 0, 0, 0, -24, 4),
	}
	if p := ie64AnalyseLoop(ins, 0x100); p != nil && p.hoist >= 0 {
		t.Fatalf("hoist = %d, want none when the destination is read before the candidate on iteration one", p.hoist)
	}
}

func TestIE64LoopHoistRejectsMemoryAndFP(t *testing.T) {
	mem := hoistLoop()
	mem[2] = loopIns(OP_LOAD, 4, 5, 0, IE64_SIZE_Q, 0, 0, 2)
	if p := ie64AnalyseLoop(mem, 0x100); p != nil && p.hoist >= 0 {
		t.Fatalf("hoist = %d, want none for a loop with memory traffic", p.hoist)
	}
	fp := hoistLoop()
	fp[2] = loopIns(OP_FADD, 4, 5, 6, 0, 0, 0, 2)
	if p := ie64AnalyseLoop(fp, 0x100); p != nil && p.hoist >= 0 {
		t.Fatalf("hoist = %d, want none for a loop with FP", p.hoist)
	}
}

func TestIE64LoopHoistRegionPlansDisableHoisting(t *testing.T) {
	ins := hoistLoop()
	region := &ie64Region{entryPC: 0x100, blockPCs: []uint64{0x100}, blocks: [][]JITInstr{ins}}
	p, _ := ie64AnalyseRegionLoop(region)
	if p != nil && p.hoist >= 0 {
		t.Fatalf("region plan hoist = %d, want disabled", p.hoist)
	}
}

func TestIE64LoopHoistMemoryLoopPlanKeepsHoistDisabled(t *testing.T) {
	ins := []JITInstr{
		loopIns(OP_MOVE, 2, 0, 0, IE64_SIZE_Q, 1, 3, 0),
		loopIns(OP_LOAD, 3, 5, 0, IE64_SIZE_Q, 0, 16, 1),
		loopIns(OP_STORE, 3, 5, 0, IE64_SIZE_Q, 0, 16, 2),
		loopIns(OP_SUB, 2, 2, 0, IE64_SIZE_Q, 1, 1, 3),
		loopIns(OP_BNE, 0, 2, 0, 0, 0, -24, 4),
	}
	p := ie64AnalyseLoop(ins, 0x100)
	if p == nil {
		t.Fatal("memory loop plan expected")
	}
	if p.hoist >= 0 {
		t.Fatalf("hoist = %d, want disabled for memory loops", p.hoist)
	}
}
