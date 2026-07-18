// jit_ie64_loop_hoist_test.go - Shared analysis tests for loop-invariant
// instruction hoisting, including dependent invariant chains.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import (
	"reflect"
	"testing"
)

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

func requireHoists(t *testing.T, p *ie64LoopPlan, want []int) {
	t.Helper()
	if p == nil {
		if want != nil {
			t.Fatalf("no plan, want hoists %v", want)
		}
		return
	}
	if !reflect.DeepEqual(p.hoists, want) {
		t.Fatalf("hoists = %v, want %v", p.hoists, want)
	}
	for _, i := range p.hoists {
		if !p.hoistSet[i] {
			t.Fatalf("hoistSet missing %d", i)
		}
	}
}

func TestIE64LoopHoistSelectsInvariant(t *testing.T) {
	p := ie64AnalyseLoop(hoistLoop(), 0x100)
	if p == nil {
		t.Fatal("pure integer loop with an invariant produced no plan")
	}
	requireHoists(t, p, []int{1})
}

func TestIE64LoopHoistRejectsChangedOperands(t *testing.T) {
	ins := hoistLoop()
	// Make the candidate read a register the loop writes: MOVE R3, R4.
	ins[1] = loopIns(OP_MOVE, 3, 4, 0, IE64_SIZE_Q, 0, 0, 1)
	if p := ie64AnalyseLoop(ins, 0x100); p != nil && len(p.hoists) != 0 {
		t.Fatalf("hoists = %v, want none for loop-varying input", p.hoists)
	}
}

func TestIE64LoopHoistRejectsRepeatedDestinationWrites(t *testing.T) {
	ins := hoistLoop()
	// Second write to R3 inside the loop.
	ins[2] = loopIns(OP_ADD, 3, 3, 0, IE64_SIZE_Q, 1, 5, 2)
	if p := ie64AnalyseLoop(ins, 0x100); p != nil && len(p.hoists) != 0 {
		t.Fatalf("hoists = %v, want none for twice-written destination", p.hoists)
	}
}

// A dependent invariant chain in program order hoists entirely: the second
// instruction's only loop-varying input is the first hoisted instruction's
// destination, defined earlier in the body.
func TestIE64LoopHoistHoistsDependentChain(t *testing.T) {
	ins := []JITInstr{
		loopIns(OP_MOVE, 2, 0, 0, IE64_SIZE_Q, 1, 3000, 0),
		loopIns(OP_MOVE, 3, 0, 0, IE64_SIZE_Q, 1, 0x123, 1),
		loopIns(OP_ADD, 4, 3, 0, IE64_SIZE_Q, 1, 7, 2), // depends on hoisted R3
		loopIns(OP_SUB, 2, 2, 0, IE64_SIZE_Q, 1, 1, 3),
		loopIns(OP_BNE, 0, 2, 0, 0, 0, -24, 4),
	}
	p := ie64AnalyseLoop(ins, 0x100)
	if p == nil {
		t.Fatal("chain loop produced no plan")
	}
	requireHoists(t, p, []int{1, 2})
}

// Three-level chain, with a loop-varying consumer in between.
func TestIE64LoopHoistHoistsThreeLevelChain(t *testing.T) {
	ins := []JITInstr{
		loopIns(OP_MOVE, 2, 0, 0, IE64_SIZE_Q, 1, 3000, 0),
		loopIns(OP_MOVE, 3, 0, 0, IE64_SIZE_Q, 1, 0x123, 1),
		loopIns(OP_LSL, 4, 3, 0, IE64_SIZE_Q, 1, 2, 2),
		loopIns(OP_EOR, 5, 5, 4, IE64_SIZE_Q, 0, 0, 3), // varying consumer
		loopIns(OP_ADD, 6, 4, 0, IE64_SIZE_Q, 1, 9, 4),
		loopIns(OP_SUB, 2, 2, 0, IE64_SIZE_Q, 1, 1, 5),
		loopIns(OP_BNE, 0, 2, 0, 0, 0, -40, 6),
	}
	p := ie64AnalyseLoop(ins, 0x100)
	if p == nil {
		t.Fatal("three-level chain produced no plan")
	}
	requireHoists(t, p, []int{1, 2, 4})
}

// A reverse-order chain must hoist nothing: the consumer precedes its
// producer, so on iteration one it reads the pre-loop value.
func TestIE64LoopHoistRejectsReverseOrderChain(t *testing.T) {
	ins := []JITInstr{
		loopIns(OP_MOVE, 2, 0, 0, IE64_SIZE_Q, 1, 3000, 0),
		loopIns(OP_ADD, 4, 3, 0, IE64_SIZE_Q, 1, 7, 1), // reads R3 before its write
		loopIns(OP_MOVE, 3, 0, 0, IE64_SIZE_Q, 1, 0x123, 2),
		loopIns(OP_SUB, 2, 2, 0, IE64_SIZE_Q, 1, 1, 3),
		loopIns(OP_BNE, 0, 2, 0, 0, 0, -24, 4),
	}
	if p := ie64AnalyseLoop(ins, 0x100); p != nil && len(p.hoists) != 0 {
		t.Fatalf("hoists = %v, want none for a reverse-order chain", p.hoists)
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
	if p := ie64AnalyseLoop(ins, 0x100); p != nil && len(p.hoists) != 0 {
		t.Fatalf("hoists = %v, want none when the destination is read before the candidate on iteration one", p.hoists)
	}
}

func TestIE64LoopHoistRejectsMemoryAndFP(t *testing.T) {
	mem := hoistLoop()
	mem[2] = loopIns(OP_LOAD, 4, 5, 0, IE64_SIZE_Q, 0, 0, 2)
	if p := ie64AnalyseLoop(mem, 0x100); p != nil && len(p.hoists) != 0 {
		t.Fatalf("hoists = %v, want none for a loop with memory traffic", p.hoists)
	}
	fp := hoistLoop()
	fp[2] = loopIns(OP_FADD, 4, 5, 6, 0, 0, 0, 2)
	if p := ie64AnalyseLoop(fp, 0x100); p != nil && len(p.hoists) != 0 {
		t.Fatalf("hoists = %v, want none for a loop with FP", p.hoists)
	}
}

func TestIE64LoopHoistRegionPlansDisableHoisting(t *testing.T) {
	ins := hoistLoop()
	region := &ie64Region{entryPC: 0x100, blockPCs: []uint64{0x100}, blocks: [][]JITInstr{ins}}
	p, _ := ie64AnalyseRegionLoop(region)
	if p != nil && len(p.hoists) != 0 {
		t.Fatalf("region plan hoists = %v, want disabled", p.hoists)
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
	if len(p.hoists) != 0 {
		t.Fatalf("hoists = %v, want disabled for memory loops", p.hoists)
	}
}
