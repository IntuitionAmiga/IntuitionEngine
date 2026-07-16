//go:build linux && amd64

package main

import (
	"os"
	"testing"
	"unsafe"
)

func newObservedBenchRig(b *testing.B) *jitTestRig {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	em, err := AllocExecMem(1 << 20)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { em.Free() })
	return &jitTestRig{bus: bus, cpu: cpu, execMem: em, ctx: newJITContext(cpu)}
}

func BenchmarkIE64ObservedConditionalLoop(b *testing.B) {
	rig := newObservedBenchRig(b)
	a := []JITInstr{{opcode: OP_NOP64}, {opcode: OP_BEQ, rs: 3, rt: 4, pcOffset: 8, imm32: 0xf8}}
	z := []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(0x107)}}
	baseA, _ := compileBlock(a, 0x100, rig.execMem)
	baseB, _ := compileBlock(z, 0x200, rig.execMem)
	observed, _ := ie64CompileRegion(ie64NativeObservedRegion(&ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: a, kind: ie64ObservedConditional, hotTarget: 0x200, coldTarget: 0x110}, {pc: 0x200, instrs: z},
	}}), rig.execMem, rig.cpu.memory)
	rig.cpu.regs[2], rig.cpu.regs[3], rig.cpu.regs[4] = 1, 7, 7
	rig.ctx.RegsPtr = uintptr(unsafe.Pointer(&rig.cpu.regs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&rig.cpu.memory[0]))
	if os.Getenv("IE64_OBSERVED_BENCH") == "baseline" {
		for i := 0; i < b.N; i++ {
			for n := 0; n < jitBudget; n++ {
				callNative(baseA.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
				callNative(baseB.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
			}
		}
	} else {
		for i := 0; i < b.N; i++ {
			callNative(observed.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
		}
	}
}

func BenchmarkIE64ObservedIndirectLoop(b *testing.B) {
	rig := newObservedBenchRig(b)
	a := []JITInstr{{opcode: OP_JMP, rs: 3, imm32: ^uint32(7)}}
	z := []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(0x107)}}
	baseA, _ := compileBlock(a, 0x100, rig.execMem)
	baseB, _ := compileBlock(z, 0x200, rig.execMem)
	observed, _ := ie64CompileRegion(ie64NativeObservedRegion(&ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: a, kind: ie64ObservedIndirectJMP, hotTarget: 0x200, predictedTarget: 0x200}, {pc: 0x200, instrs: z},
	}}), rig.execMem, rig.cpu.memory)
	rig.cpu.regs[2], rig.cpu.regs[3] = 1, 0x208
	rig.ctx.RegsPtr = uintptr(unsafe.Pointer(&rig.cpu.regs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&rig.cpu.memory[0]))
	if os.Getenv("IE64_OBSERVED_BENCH") == "baseline" {
		for i := 0; i < b.N; i++ {
			for n := 0; n < jitBudget; n++ {
				callNative(baseA.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
				callNative(baseB.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
			}
		}
	} else {
		for i := 0; i < b.N; i++ {
			callNative(observed.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
		}
	}
}
