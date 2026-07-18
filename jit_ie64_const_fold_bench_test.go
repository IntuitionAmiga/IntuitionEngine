//go:build (amd64 || arm64) && (linux || windows || darwin)

// jit_ie64_const_fold_bench_test.go - Focused benchmark for constant-only
// folding: repeated constant integer arithmetic, straight-line and inside a
// counted loop. The folded and unfolded variants run in one binary under
// identical conditions; extract each into a separate benchstat input with
// the same benchmark name.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "testing"

// buildConstFoldBenchProgram lays down a long dependent chain of constant
// integer arithmetic: every instruction's inputs are statically known, so
// with folding each becomes an independent immediate load.
func buildConstFoldBenchProgram(mem []byte) {
	base := uint64(PROG_START)
	off := uint64(0)
	put := func(ins []byte) { copy(mem[base+off:], ins); off += 8 }
	put(ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x12345))
	for i := 0; i < 199; i++ {
		switch i % 4 {
		case 0:
			put(ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 7))
		case 1:
			put(ie64Instr(OP_EOR, 1, IE64_SIZE_Q, 1, 1, 0, 0x5A5A))
		case 2:
			put(ie64Instr(OP_LSL, 1, IE64_SIZE_Q, 1, 1, 0, 3))
		case 3:
			put(ie64Instr(OP_SUB, 1, IE64_SIZE_L, 1, 1, 0, 13))
		}
	}
	put(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

func benchConstFold(b *testing.B, disabled bool) {
	if !jitAvailable {
		b.Skip("JIT not available")
	}
	prev := ie64ConstFoldDisabled
	ie64ConstFoldDisabled = disabled
	defer func() { ie64ConstFoldDisabled = prev }()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	buildConstFoldBenchProgram(cpu.memory)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[1] = 0
		cpu.running.Store(true)
		cpu.jitExecute()
	}
	b.ReportMetric(201, "instructions/op")
}

func BenchmarkIE64_ConstFoldChain_Folded(b *testing.B)   { benchConstFold(b, false) }
func BenchmarkIE64_ConstFoldChain_Unfolded(b *testing.B) { benchConstFold(b, true) }

// buildConstFoldLoopProgram wraps a constant chain in a counted loop so the
// chain re-executes natively without dispatcher transitions. The loop counter
// is not foldable (its head is a branch target), the body chain is.
func buildConstFoldLoopProgram(mem []byte) {
	base := uint64(PROG_START)
	off := uint64(0)
	put := func(ins []byte) { copy(mem[base+off:], ins); off += 8 }
	put(ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 100000)) // counter
	head := off
	for i := 0; i < 25; i++ {
		put(ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x12345))
		put(ie64Instr(OP_ADD, 3, IE64_SIZE_Q, 1, 1, 0, 7))
		put(ie64Instr(OP_EOR, 4, IE64_SIZE_Q, 1, 3, 0, 0x5A5A))
		put(ie64Instr(OP_LSL, 5, IE64_SIZE_Q, 1, 4, 0, 3))
	}
	put(ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1))
	back := int32(head) - int32(off)
	put(ie64Instr(OP_BNE, 0, 0, 0, 2, 0, uint32(back)))
	put(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

func benchConstFoldLoop(b *testing.B, disabled bool) {
	if !jitAvailable {
		b.Skip("JIT not available")
	}
	prev := ie64ConstFoldDisabled
	ie64ConstFoldDisabled = disabled
	defer func() { ie64ConstFoldDisabled = prev }()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	buildConstFoldLoopProgram(cpu.memory)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[2] = 0
		cpu.running.Store(true)
		cpu.jitExecute()
	}
}

func BenchmarkIE64_ConstFoldLoop_Folded(b *testing.B)   { benchConstFoldLoop(b, false) }
func BenchmarkIE64_ConstFoldLoop_Unfolded(b *testing.B) { benchConstFoldLoop(b, true) }
