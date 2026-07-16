//go:build amd64 && linux

package main

import (
	"testing"
	"unsafe"
)

func TestAMD64HoistedLoopRejectsRuntimeMMUTransition(t *testing.T) {
	r := newJITTestRig(t)
	back := int32(-16)
	program := [][]byte{
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 2),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_Q, 0, 5, 0, 0),
		ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1),
		ie64Instr(OP_BNE, 0, 0, 0, 2, 0, uint32(back)),
	}
	for i, ins := range program {
		copy(r.cpu.memory[PROG_START+uint64(i)*IE64_INSTR_SIZE:], ins)
	}
	copy(r.cpu.memory[PROG_START+uint64(len(program))*IE64_INSTR_SIZE:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	block, err := compileBlock(scanBlock(r.cpu.memory, PROG_START), PROG_START, r.execMem)
	if err != nil {
		t.Fatal(err)
	}
	r.cpu.regs[5] = 0x3000
	r.cpu.regs[3] = 0xfeedface
	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&r.cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	r.ctx.MMUEnabled = 1
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))
	if r.ctx.NeedIOFallback != jitFallbackLoopPrecheck {
		t.Fatalf("fallback = %d, want loop precheck", r.ctx.NeedIOFallback)
	}
	if r.ctx.RetPC != PROG_START+IE64_INSTR_SIZE || r.ctx.RetCount != 1 {
		t.Fatalf("return = {pc:%#x count:%d}, want loop head and prefix count 1", r.ctx.RetPC, r.ctx.RetCount)
	}
	if r.cpu.regs[3] != 0xfeedface {
		t.Fatalf("hoisted load executed with MMU enabled: R3=%#x", r.cpu.regs[3])
	}
}
