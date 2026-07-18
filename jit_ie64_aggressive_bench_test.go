//go:build linux && (amd64 || arm64)

// jit_ie64_aggressive_bench_test.go - Focused benchmarks for the aggressive
// optimisation extensions: constants folded through memory traffic, the
// extended pure-integer fold whitelist, multi-instruction loop hoisting and
// multiple outlined cold exits. Each pair runs both variants in one binary
// under identical conditions; extract each into a separate benchstat input
// with the same benchmark name.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import (
	"runtime"
	"testing"
	"unsafe"
)

// buildFoldMemTrafficProgram interleaves a dependent constant chain with
// STOREs. With relaxed barriers the whole chain folds; with conservative
// barriers every STORE clears tracked constants, so only the first group
// folds.
func buildFoldMemTrafficProgram(mem []byte) {
	base := uint64(PROG_START)
	off := uint64(0)
	put := func(ins []byte) { copy(mem[base+off:], ins); off += 8 }
	put(ie64Instr(OP_LEA, 9, 0, 0, 0, 0, uint32(base+0x200000)))
	put(ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x12345))
	put(ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, 20000)) // counter
	head := off
	// The loop head is a branch target, so tracked constants restart here:
	// the immediate MOVE reseeds the chain every iteration.
	put(ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x12345))
	for i := 0; i < 12; i++ {
		put(ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 7))
		put(ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 9, 0, uint32(i*8)))
		put(ie64Instr(OP_EOR, 2, IE64_SIZE_Q, 1, 1, 0, 0x5A5A))
		put(ie64Instr(OP_LSL, 3, IE64_SIZE_Q, 1, 2, 0, 3))
	}
	put(ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1))
	back := int32(head) - int32(off)
	put(ie64Instr(OP_BNE, 0, 0, 0, 10, 0, uint32(back)))
	put(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

func benchFoldMemTraffic(b *testing.B, conservative bool) {
	if !jitAvailable {
		b.Skip("JIT not available")
	}
	prev := ie64FoldBarrierRelaxDisabled
	ie64FoldBarrierRelaxDisabled = conservative
	defer func() { ie64FoldBarrierRelaxDisabled = prev }()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	buildFoldMemTrafficProgram(cpu.memory)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[1] = 0
		cpu.running.Store(true)
		cpu.jitExecute()
	}
}

func BenchmarkIE64_FoldMemTraffic_Relaxed(b *testing.B)      { benchFoldMemTraffic(b, false) }
func BenchmarkIE64_FoldMemTraffic_Conservative(b *testing.B) { benchFoldMemTraffic(b, true) }

// buildFoldExtendedProgram is a dependent chain dominated by the extended
// whitelist: divides, signed modulo, multiplies and rotates on statically
// known values. The conservative whitelist folds none of these.
func buildFoldExtendedProgram(mem []byte) {
	base := uint64(PROG_START)
	off := uint64(0)
	put := func(ins []byte) { copy(mem[base+off:], ins); off += 8 }
	put(ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x7654321))
	for i := 0; i < 49; i++ {
		put(ie64Instr(OP_DIVU, 1, IE64_SIZE_Q, 1, 1, 0, 3))
		put(ie64Instr(OP_MULS, 1, IE64_SIZE_Q, 1, 1, 0, 5))
		put(ie64Instr(OP_MODS, 2, IE64_SIZE_Q, 1, 1, 0, 0x3FFFF))
		put(ie64Instr(OP_ROL, 1, IE64_SIZE_Q, 1, 1, 0, 7))
	}
	put(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

func benchFoldExtended(b *testing.B, disabled bool) {
	if !jitAvailable {
		b.Skip("JIT not available")
	}
	prev := ie64ConstFoldDisabled
	ie64ConstFoldDisabled = disabled
	defer func() { ie64ConstFoldDisabled = prev }()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	buildFoldExtendedProgram(cpu.memory)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[1] = 0
		cpu.running.Store(true)
		cpu.jitExecute()
	}
	b.ReportMetric(198, "instructions/op")
}

func BenchmarkIE64_FoldExtendedOps_Folded(b *testing.B)   { benchFoldExtended(b, false) }
func BenchmarkIE64_FoldExtendedOps_Unfolded(b *testing.B) { benchFoldExtended(b, true) }

// buildMultiHoistProgram: counted loop whose body carries a three-instruction
// dependent invariant chain feeding two varying accumulators.
func buildMultiHoistProgram(mem []byte) {
	base := uint64(PROG_START)
	put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
	put(0x00, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 100000))
	put(0x08, ie64Instr(OP_LSL, 3, IE64_SIZE_Q, 1, 6, 0, 4))  // invariant: R6<<4
	put(0x10, ie64Instr(OP_ADD, 7, IE64_SIZE_Q, 1, 3, 0, 9))  // invariant chain
	put(0x18, ie64Instr(OP_MULU, 8, IE64_SIZE_Q, 1, 3, 0, 5)) // invariant chain
	put(0x20, ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 0, 4, 7, 0))  // varying
	put(0x28, ie64Instr(OP_EOR, 5, IE64_SIZE_Q, 0, 5, 8, 0))  // varying
	put(0x30, ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1))
	put(0x38, ie64Instr(OP_BNE, 0, 0, 0, 2, 0, ^uint32(0x2f))) // -> 0x08
	put(0x40, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

func benchMultiHoist(b *testing.B, disabled bool) {
	if !jitAvailable {
		b.Skip("JIT not available")
	}
	prev := ie64LoopHoistDisabled
	ie64LoopHoistDisabled = disabled
	defer func() { ie64LoopHoistDisabled = prev }()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	buildMultiHoistProgram(cpu.memory)
	cpu.regs[6] = 7
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[2], cpu.regs[4], cpu.regs[5] = 0, 0, 0
		cpu.running.Store(true)
		cpu.jitExecute()
	}
}

func BenchmarkIE64_MultiHoist_Hoisted(b *testing.B)  { benchMultiHoist(b, false) }
func BenchmarkIE64_MultiHoist_Baseline(b *testing.B) { benchMultiHoist(b, true) }

// benchMultiColdExit: an observed region with two strongly biased forward
// conditionals; the conservative rule (exactly one conditional) would not
// outline either.
func benchMultiColdExit(b *testing.B, disabled bool) {
	prev := ie64ColdExitOutlineDisabled
	ie64ColdExitOutlineDisabled = disabled
	defer func() { ie64ColdExitOutlineDisabled = prev }()

	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	em, err := AllocExecMem(1 << 20)
	if err != nil {
		b.Fatalf("AllocExecMem: %v", err)
	}
	defer em.Free()
	ctx := newJITContext(cpu)

	observed := &ie64ObservedRegion{entryPC: 0x100, blocks: []ie64ObservedBlock{
		{pc: 0x100, instrs: []JITInstr{{opcode: OP_NOP64}, {opcode: OP_BEQ, rs: 3, rt: 4, pcOffset: 8, imm32: 0xf8}}, kind: ie64ObservedConditional, hotTarget: 0x200, coldTarget: 0x110},
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BNE, rs: 5, rt: 0, pcOffset: 8, imm32: 0xf8}}, kind: ie64ObservedConditional, hotTarget: 0x300, coldTarget: 0x210},
		{pc: 0x300, instrs: []JITInstr{{opcode: OP_ADD, rd: 6, rs: 6, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(0x207)}}},
	}}
	block, err := ie64CompileRegion(ie64NativeObservedRegion(observed), em, cpu.memory)
	if err != nil {
		b.Fatal(err)
	}
	cpu.regs[2], cpu.regs[3], cpu.regs[4], cpu.regs[5] = 1, 7, 7, 1 // both hot
	ctx.RegsPtr = uintptr(unsafe.Pointer(&cpu.regs[0]))
	ctx.MemPtr = uintptr(unsafe.Pointer(&cpu.memory[0]))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	}
	runtime.KeepAlive(ctx)
	b.ReportMetric(float64(jitBudget), "instructions/op")
}

func BenchmarkIE64_MultiColdExit_Outlined(b *testing.B) { benchMultiColdExit(b, false) }
func BenchmarkIE64_MultiColdExit_Inline(b *testing.B)   { benchMultiColdExit(b, true) }
