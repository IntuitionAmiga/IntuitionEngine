//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// BenchmarkIE64_FPSRDeadCCLoop_JIT is the focused benchmark for Technique 3's
// FPSR condition-code liveness elision. A hot single-block loop chains three FP
// CC writers (FABS, FNEG, FABS) per iteration; only the last is observed (by the
// terminating BNE / block end), so the first two CC updates are dead and elided.
// Compare against a build with ie64MarkFPSRCCDead forced to a no-op to measure
// the win.
func BenchmarkIE64_FPSRDeadCCLoop_JIT(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available on this platform")
	}

	// Single-block loop: FABS F1 ; FNEG F1 ; FABS F1 ; SUB R10 ; BNE -> head.
	// F1 is seeded by resetState; R10 is the trip counter.
	var instrs [][]byte
	instrs = append(instrs, ie64Instr(OP_FABS, 1, 0, 0, 1, 0, 0))            // i0 CC dead
	instrs = append(instrs, ie64Instr(OP_FNEG, 1, 0, 0, 1, 0, 0))            // i1 CC dead
	instrs = append(instrs, ie64Instr(OP_FABS, 1, 0, 0, 1, 0, 0))            // i2 CC live (block end)
	instrs = append(instrs, ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1)) // i3
	negBack := int32(-0x20)
	instrs = append(instrs, ie64Instr(OP_BNE, 0, 0, 0, 10, 0, uint32(negBack))) // i4 -> i0
	loopBodyInstrs := 5
	totalInstrs := int(benchIterations) * loopBodyInstrs

	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	resetState := func() {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.FPU.FPRegs[1] = 0xC0000000 // -2.0
		cpu.running.Store(true)
	}
	setupJITBench(b, cpu, instrs, resetState)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetState()
		cpu.jitExecute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, totalInstrs)
}

// BenchmarkIE64_FPSRDeadCC64Loop_JIT is the FP64 analogue: a hot single-block
// loop where a DADD's CC update (emitSetFPCondCodes64AMD64, the largest CC
// sequence) is overwritten by a trailing FP32 FABS before the block-end observer,
// so it is elided every iteration. Compare against ie64MarkFPSRCCDead as a no-op.
func BenchmarkIE64_FPSRDeadCC64Loop_JIT(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available on this platform")
	}

	// Loop: DADD D4,D0,D2 ; FABS F6 (kills CC) ; SUB R10 ; BNE -> head.
	// D0/D2 seeded by resetState; R10 is the trip counter.
	var instrs [][]byte
	instrs = append(instrs, ie64Instr(OP_DADD, 4, IE64_SIZE_L, 0, 0, 2, 0))     // i0 CC64 dead
	instrs = append(instrs, ie64Instr(OP_FABS, 6, 0, 0, 6, 0, 0))               // i1 CC live (kills i0)
	instrs = append(instrs, ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1))    // i2
	negBack := int32(-0x18)                                                     // BNE at 0x18 -> i0 at 0x00
	instrs = append(instrs, ie64Instr(OP_BNE, 0, 0, 0, 10, 0, uint32(negBack))) // i3 -> i0
	instrs = append(instrs, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))             // i4 clean fall-through exit
	loopBodyInstrs := 4
	totalInstrs := int(benchIterations) * loopBodyInstrs

	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	resetState := func() {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.FPU.setDPair(0, 3.0)
		cpu.FPU.setDPair(2, 5.0)
		cpu.running.Store(true)
	}
	setupJITBench(b, cpu, instrs, resetState)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetState()
		cpu.jitExecute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, totalInstrs)
}
