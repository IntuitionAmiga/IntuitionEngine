//go:build (amd64 || arm64) && (linux || windows || darwin)

// jit_ie64_loop_hoist_bench_test.go - Focused benchmark for loop hoisting: a
// loop repeating one invariant integer calculation consumed by later integer
// operations, plus a non-hoistable control shape. Both variants run in one
// binary under identical conditions; extract each into a separate benchstat
// input with the same benchmark name.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "testing"

func buildHoistBenchProgram(hoistable bool) func(mem []byte) {
	return func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 100000))
		if hoistable {
			put(0x08, ie64Instr(OP_LSL, 3, IE64_SIZE_Q, 1, 6, 0, 4)) // invariant: R6<<4
		} else {
			put(0x08, ie64Instr(OP_LSL, 3, IE64_SIZE_Q, 1, 4, 0, 4)) // reads loop-varying R4
		}
		put(0x10, ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 0, 4, 3, 0))
		put(0x18, ie64Instr(OP_EOR, 5, IE64_SIZE_Q, 0, 5, 4, 0))
		put(0x20, ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1))
		put(0x28, ie64Instr(OP_BNE, 0, 0, 0, 2, 0, ^uint32(0x1f))) // -> 0x08
		put(0x30, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
}

func benchLoopHoist(b *testing.B, disabled bool) {
	if !jitAvailable {
		b.Skip("JIT not available")
	}
	prev := ie64LoopHoistDisabled
	ie64LoopHoistDisabled = disabled
	defer func() { ie64LoopHoistDisabled = prev }()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	buildHoistBenchProgram(true)(cpu.memory)
	cpu.regs[6] = 7
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[2], cpu.regs[4], cpu.regs[5] = 0, 0, 0
		cpu.running.Store(true)
		cpu.jitExecute()
	}
}

func BenchmarkIE64_LoopHoist_Hoisted(b *testing.B)  { benchLoopHoist(b, false) }
func BenchmarkIE64_LoopHoist_Baseline(b *testing.B) { benchLoopHoist(b, true) }

// Non-hoistable control: identical shape, loop-varying input. Both toggle
// states must produce the same code, so this pins the absence of a hidden
// cost for rejected loops.
func BenchmarkIE64_LoopHoistControl(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available")
	}
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	buildHoistBenchProgram(false)(cpu.memory)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[2], cpu.regs[4], cpu.regs[5] = 0, 0, 0
		cpu.running.Store(true)
		cpu.jitExecute()
	}
}
