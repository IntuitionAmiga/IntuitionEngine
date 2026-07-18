//go:build linux && (amd64 || arm64)

// jit_ie64_cold_exit_test.go - Structural and behavioural tests for the
// outlined cold exit in native observed conditional regions.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "testing"

// Adjacent forward hot successor: the region must compile with the outlined
// layout, the cold outcome must exit with exact accounting and spills, and
// SMC coverage metadata must be unchanged.
func TestColdExitOutline_ColdOutcome(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_NOP64}, {opcode: OP_BEQ, rs: 3, rt: 4, pcOffset: 8, imm32: 0xf8}}, kind: ie64ObservedConditional, hotTarget: 0x200, coldTarget: 0x110},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(0x107)}}},
	}}
	rig.cpu.regs[3], rig.cpu.regs[4] = 1, 2 // BEQ false -> cold
	before := ie64ColdExitOutlines.Load()
	block := runObservedNative(t, rig, observed)
	if ie64ColdExitOutlines.Load() == before {
		t.Fatal("adjacent forward observed conditional was not compiled with an outlined cold exit")
	}
	if rig.ctx.RetPC != 0x110 || rig.ctx.RetCount != 2 || rig.cpu.regs[1] != 0 {
		t.Fatalf("cold exit PC=%#x count=%d R1=%d, want 0x110/2/0", rig.ctx.RetPC, rig.ctx.RetCount, rig.cpu.regs[1])
	}
	if block.instrCount != 4 || len(block.coveredRanges) != 2 {
		t.Fatalf("metadata=%+v", block)
	}
}

// Hot outcome: the fall-through path runs the loop natively until the budget
// is consumed, exactly as before.
func TestColdExitOutline_HotOutcomeLoopBudget(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_NOP64}, {opcode: OP_BEQ, rs: 3, rt: 4, pcOffset: 8, imm32: 0xf8}}, kind: ie64ObservedConditional, hotTarget: 0x200, coldTarget: 0x110},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(0x107)}}},
	}}
	rig.cpu.regs[2], rig.cpu.regs[3], rig.cpu.regs[4] = 1, 7, 7 // BEQ true -> hot
	before := ie64ColdExitOutlines.Load()
	runObservedNative(t, rig, observed)
	if ie64ColdExitOutlines.Load() == before {
		t.Fatal("hot-path region was not compiled with an outlined cold exit")
	}
	if rig.ctx.RetPC != 0x100 || rig.ctx.RetCount < jitBudget || rig.cpu.regs[1] == 0 {
		t.Fatalf("hot loop PC=%#x count=%d R1=%d", rig.ctx.RetPC, rig.ctx.RetCount, rig.cpu.regs[1])
	}
}

// Cold exit spills writes from earlier blocks even with the outlined layout.
func TestColdExitOutline_ColdSpillsEarlierWrites(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: 0xf8}}},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_BEQ, rs: 3, rt: 4, imm32: 0x100}}, kind: ie64ObservedConditional, hotTarget: 0x300, coldTarget: 0x208},
		{pc: 0x300, instrs: []JITInstr{{opcode: OP_BRA, imm32: ^uint32(0x1ff)}}},
	}}
	rig.cpu.regs[1], rig.cpu.regs[2] = 5, 7
	rig.cpu.regs[3], rig.cpu.regs[4] = 1, 2
	before := ie64ColdExitOutlines.Load()
	runObservedNative(t, rig, observed)
	if ie64ColdExitOutlines.Load() == before {
		t.Fatal("mid-region adjacent forward conditional was not outlined")
	}
	if rig.ctx.RetPC != 0x208 || rig.ctx.RetCount != 3 || rig.cpu.regs[1] != 12 {
		t.Fatalf("cold exit PC=%#x count=%d R1=%d, want 0x208/3/12", rig.ctx.RetPC, rig.ctx.RetCount, rig.cpu.regs[1])
	}
}

// Backward hot edge (last block's conditional wraps to the region head):
// current layout retained.
func TestColdExitOutline_RejectsBackwardHotEdge(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: 0xf8}}},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_BNE, rs: 3, rt: 0, imm32: ^uint32(0xff)}}, kind: ie64ObservedConditional, hotTarget: 0x100, coldTarget: 0x208},
	}}
	rig.cpu.regs[2], rig.cpu.regs[3] = 1, 1 // BNE true -> hot backward loop
	before := ie64ColdExitOutlines.Load()
	runObservedNative(t, rig, observed)
	if ie64ColdExitOutlines.Load() != before {
		t.Fatal("backward hot edge must retain the current layout")
	}
	if rig.ctx.RetPC != 0x100 || rig.ctx.RetCount < jitBudget || rig.cpu.regs[1] == 0 {
		t.Fatalf("hot loop PC=%#x count=%d R1=%d", rig.ctx.RetPC, rig.ctx.RetCount, rig.cpu.regs[1])
	}
}

// More than one observed conditional: every adjacent-forward conditional is
// outlined, and each cold exit still behaves with exact accounting.
func TestColdExitOutline_MultipleConditionalsAllOutlined(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_BEQ, rs: 3, rt: 4, imm32: 0x100}}, kind: ie64ObservedConditional, hotTarget: 0x200, coldTarget: 0x108},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_BEQ, rs: 5, rt: 6, imm32: 0x100}}, kind: ie64ObservedConditional, hotTarget: 0x300, coldTarget: 0x208},
		{pc: 0x300, instrs: []JITInstr{{opcode: OP_BRA, imm32: ^uint32(0x1ff)}}},
	}}
	rig.cpu.regs[3], rig.cpu.regs[4] = 7, 7 // first hot
	rig.cpu.regs[5], rig.cpu.regs[6] = 1, 2 // second cold
	before := ie64ColdExitOutlines.Load()
	runObservedNative(t, rig, observed)
	if got := ie64ColdExitOutlines.Load() - before; got != 2 {
		t.Fatalf("outlined %d cold exits, want 2 (one per adjacent forward conditional)", got)
	}
	if rig.ctx.RetPC != 0x208 || rig.ctx.RetCount != 2 {
		t.Fatalf("second cold exit PC=%#x count=%d, want 0x208/2", rig.ctx.RetPC, rig.ctx.RetCount)
	}
}

// First-conditional cold outcome in a multi-conditional region: the first
// outlined stub must exit with its own captured accounting, not the second's.
func TestColdExitOutline_MultipleConditionalsFirstCold(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_BEQ, rs: 3, rt: 4, imm32: 0x100}}, kind: ie64ObservedConditional, hotTarget: 0x200, coldTarget: 0x108},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_BEQ, rs: 5, rt: 6, imm32: 0x100}}, kind: ie64ObservedConditional, hotTarget: 0x300, coldTarget: 0x208},
		{pc: 0x300, instrs: []JITInstr{{opcode: OP_BRA, imm32: ^uint32(0x1ff)}}},
	}}
	rig.cpu.regs[3], rig.cpu.regs[4] = 1, 2 // first cold
	rig.cpu.regs[5], rig.cpu.regs[6] = 7, 7
	runObservedNative(t, rig, observed)
	if rig.ctx.RetPC != 0x108 || rig.ctx.RetCount != 1 {
		t.Fatalf("first cold exit PC=%#x count=%d, want 0x108/1", rig.ctx.RetPC, rig.ctx.RetCount)
	}
}

// Mixed shapes: a backward hot edge keeps the current layout while an
// adjacent forward conditional in the same region is still outlined.
func TestColdExitOutline_MixedBackwardAndForward(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_BEQ, rs: 3, rt: 4, imm32: 0x100}}, kind: ie64ObservedConditional, hotTarget: 0x200, coldTarget: 0x108},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BNE, rs: 5, rt: 0, pcOffset: 8, imm32: ^uint32(0x107)}}, kind: ie64ObservedConditional, hotTarget: 0x100, coldTarget: 0x210},
	}}
	rig.cpu.regs[2] = 1
	rig.cpu.regs[3], rig.cpu.regs[4] = 7, 7 // forward conditional hot
	rig.cpu.regs[5] = 1                     // backward conditional hot -> loops
	before := ie64ColdExitOutlines.Load()
	runObservedNative(t, rig, observed)
	if got := ie64ColdExitOutlines.Load() - before; got != 1 {
		t.Fatalf("outlined %d cold exits, want 1 (backward hot edge keeps current layout)", got)
	}
	if rig.ctx.RetPC != 0x100 || rig.ctx.RetCount < jitBudget || rig.cpu.regs[1] == 0 {
		t.Fatalf("hot loop PC=%#x count=%d R1=%d", rig.ctx.RetPC, rig.ctx.RetCount, rig.cpu.regs[1])
	}
}
