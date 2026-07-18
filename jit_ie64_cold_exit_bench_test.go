//go:build linux && (amd64 || arm64)

// jit_ie64_cold_exit_bench_test.go - Focused benchmark for the outlined cold
// exit: a strongly biased forward observed conditional whose hot successor is
// the next emitted block, followed by an existing back edge that closes the
// loop. Both layouts run in one binary under identical conditions; extract
// each into a separate benchstat input with the same benchmark name.
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

func benchColdExit(b *testing.B, disabled bool) {
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
		{pc: 0x200, instrs: []JITInstr{{opcode: OP_ADD, rd: 1, rs: 1, rt: 2, size: IE64_SIZE_Q}, {opcode: OP_BRA, pcOffset: 8, imm32: ^uint32(0x107)}}},
	}}
	block, err := ie64CompileRegion(ie64NativeObservedRegion(observed), em, cpu.memory)
	if err != nil {
		b.Fatal(err)
	}
	cpu.regs[2], cpu.regs[3], cpu.regs[4] = 1, 7, 7 // BEQ always true: hot loop
	ctx.RegsPtr = uintptr(unsafe.Pointer(&cpu.regs[0]))
	ctx.MemPtr = uintptr(unsafe.Pointer(&cpu.memory[0]))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	}
	runtime.KeepAlive(ctx)
	b.ReportMetric(float64(jitBudget), "instructions/op")
}

func BenchmarkIE64_ColdExitHotLoop_Outlined(b *testing.B) { benchColdExit(b, false) }
func BenchmarkIE64_ColdExitHotLoop_Inline(b *testing.B)   { benchColdExit(b, true) }
