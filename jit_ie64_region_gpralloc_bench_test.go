//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// BenchmarkIE64_RegionHighRegLoop_JIT is the focused benchmark for Technique 2
// (region GPR allocation). A hot three-block loop does all its work in high-
// numbered guest registers (R5/R7/R9/R10) that the fixed Tier-1 mapping leaves
// spilled to memory. Once the loop promotes to the region tier, the planner
// binds those hot registers to the callee-saved hosts, so the per-iteration
// arithmetic runs register-to-register instead of through memory. Compare
// against a build with ie64BuildRegionRegMap forced to nil to measure the win.
func BenchmarkIE64_RegionHighRegLoop_JIT(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available on this platform")
	}
	b.Setenv("IE64_JIT_REGIONS", "1")

	// Contiguous 3-block loop (A->B->C) joined by BRAs with a conditional
	// back-edge to the loop head. R10 (trip count) is seeded by resetState;
	// there is deliberately no in-program seed instruction, so the back-edge
	// re-entering the loop head never clobbers the counter.
	var instrs [][]byte
	// Block A (loop head, i0): R5 += R10 ; BRA B
	instrs = append(instrs, ie64Instr(OP_ADD, 5, IE64_SIZE_Q, 0, 5, 10, 0)) // i0
	instrs = append(instrs, ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 8))            // i1 -> i2
	// Block B (i2): R7 += R5 ; R1 += R5 ; R9 ^= R7 ; BRA C
	instrs = append(instrs, ie64Instr(OP_ADD, 7, IE64_SIZE_Q, 0, 7, 5, 0)) // i2
	instrs = append(instrs, ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 0, 1, 5, 0)) // i3
	instrs = append(instrs, ie64Instr(OP_EOR, 9, IE64_SIZE_Q, 0, 9, 7, 0)) // i4
	instrs = append(instrs, ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 8))           // i5 -> i6
	// Block C (i6): R10 -= 1 ; BNE R10,R0 -> loop head (i0)
	instrs = append(instrs, ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1)) // i6
	negBack := int32(-56)
	back := uint32(negBack) // i7 -> i0
	instrs = append(instrs, ie64Instr(OP_BNE, 0, 0, 0, 10, 0, back))         // i7
	loopBodyInstrs := 8                                                       // A(2)+B(4)+C(2) retired per iteration
	totalInstrs := int(benchIterations) * loopBodyInstrs

	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	resetState := func() {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
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
