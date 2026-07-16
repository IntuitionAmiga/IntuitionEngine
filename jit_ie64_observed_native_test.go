//go:build linux && (amd64 || arm64)

package main

import (
	"runtime"
	"testing"
	"unsafe"
)

func runObservedNative(t *testing.T, rig *jitTestRig, observed *ie64ObservedRegion) *JITBlock {
	t.Helper()
	block, err := ie64CompileRegion(ie64NativeObservedRegion(observed), rig.execMem, rig.cpu.memory)
	if err != nil {
		t.Fatal(err)
	}
	rig.ctx.RegsPtr = uintptr(unsafe.Pointer(&rig.cpu.regs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&rig.cpu.memory[0]))
	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	runtime.KeepAlive(rig.ctx)
	return block
}

func TestNativeObservedConditionalColdExitSkipsTail(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_NOP64}, {opcode: OP_BEQ, rs: 3, rt: 4, pcOffset: 8, imm32: 0xf8}}, kind: ie64ObservedConditional, hotTarget: 0x200, coldTarget: 0x110},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(0x107)}}},
	}}
	rig.cpu.regs[3], rig.cpu.regs[4] = 1, 2
	block := runObservedNative(t, rig, observed)
	if rig.ctx.RetPC != 0x110 || rig.ctx.RetCount != 2 || rig.cpu.regs[1] != 0 {
		t.Fatalf("cold exit PC=%#x count=%d R1=%d", rig.ctx.RetPC, rig.ctx.RetCount, rig.cpu.regs[1])
	}
	if block.instrCount != 4 || len(block.coveredRanges) != 2 {
		t.Fatalf("metadata=%+v", block)
	}
}

func TestNativeObservedConditionalColdExitSpillsEarlierBlockWrites(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: 0xf8}}},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_BEQ, rs: 3, rt: 4, imm32: 0x100}}, kind: ie64ObservedConditional, hotTarget: 0x300, coldTarget: 0x208},
		{pc: 0x300, instrs: []JITInstr{{opcode: OP_BRA, imm32: ^uint32(0x1ff)}}},
	}}
	rig.cpu.regs[1], rig.cpu.regs[2] = 5, 7
	rig.cpu.regs[3], rig.cpu.regs[4] = 1, 2
	runObservedNative(t, rig, observed)
	if rig.ctx.RetPC != 0x208 || rig.ctx.RetCount != 3 || rig.cpu.regs[1] != 12 {
		t.Fatalf("cold exit PC=%#x count=%d R1=%d, want 0x208/3/12", rig.ctx.RetPC, rig.ctx.RetCount, rig.cpu.regs[1])
	}
}

func TestNativeObservedConditionalTakenClosesLoop(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_NOP64}, {opcode: OP_BEQ, rs: 3, rt: 4, pcOffset: 8, imm32: 0xf8}}, kind: ie64ObservedConditional, hotTarget: 0x200, coldTarget: 0x110},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(0x107)}}},
	}}
	rig.cpu.regs[2], rig.cpu.regs[3], rig.cpu.regs[4] = 1, 7, 7
	runObservedNative(t, rig, observed)
	if rig.ctx.RetPC != 0x100 || rig.ctx.RetCount < jitBudget || rig.cpu.regs[1] == 0 {
		t.Fatalf("hot loop PC=%#x count=%d R1=%d", rig.ctx.RetPC, rig.ctx.RetCount, rig.cpu.regs[1])
	}
}

func TestNativeObservedIndirectMismatchUsesDynamicExit(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_JMP, rs: 3, imm32: ^uint32(7)}}, kind: ie64ObservedIndirectJMP, hotTarget: 0x200, predictedTarget: 0x200},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(0x107)}}},
	}}
	rig.cpu.regs[3] = 0x1_0000_0010
	rig.cpu.regs[1], rig.cpu.regs[2] = 3, 4
	runObservedNative(t, rig, observed)
	if rig.ctx.RetPC != 0x1_0000_0008 || rig.ctx.RetCount != 1 || rig.cpu.regs[1] != 3 {
		t.Fatalf("mismatch PC=%#x count=%d R1=%d", rig.ctx.RetPC, rig.ctx.RetCount, rig.cpu.regs[1])
	}
}

func TestNativeObservedIndirectHitClosesLoop(t *testing.T) {
	rig := newJITTestRig(t)
	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_JMP, rs: 3, imm32: ^uint32(7)}}, kind: ie64ObservedIndirectJMP, hotTarget: 0x200, predictedTarget: 0x200},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(0x107)}}},
	}}
	rig.cpu.regs[2], rig.cpu.regs[3] = 1, 0x208
	runObservedNative(t, rig, observed)
	if rig.ctx.RetPC != 0x100 || rig.ctx.RetCount < jitBudget || rig.cpu.regs[1] == 0 {
		t.Fatalf("hit loop PC=%#x count=%d R1=%d", rig.ctx.RetPC, rig.ctx.RetCount, rig.cpu.regs[1])
	}
}
