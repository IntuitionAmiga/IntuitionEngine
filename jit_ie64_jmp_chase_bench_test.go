//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import "testing"

// BenchmarkIE64_TrampolineColdDispatch_JIT is the focused benchmark for the
// static-JMP chase (Technique 1). Each op runs, from a cold code cache, a chain
// of static unconditional jumps (BRA/JMP R0) that forwards to a HALT. Without
// the chase the dispatcher compiles and dispatches one native block per
// trampoline hop; with the chase all hops collapse in Go and no trampoline
// block is compiled. The cold cache (no jitPersist) makes the compile-and-
// dispatch cost the dominant term the chase removes.
func BenchmarkIE64_TrampolineColdDispatch_JIT(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available on this platform")
	}
	const hops = 60 // < ie64StaticJumpChaseCap so a single chase collapses all
	base := uint64(PROG_START)
	build := func(cpu *CPU64) {
		// Trampolines spaced 0x40 apart, each jumping to the next.
		for i := uint64(0); i < hops; i++ {
			at := base + i*0x40
			nextAbs := base + (i+1)*0x40
			// Alternate BRA (PC-relative) and JMP R0 (absolute) so both static
			// forms are exercised.
			if i&1 == 0 {
				copy(cpu.memory[at:], ie64Instr(OP_JMP, 0, 0, 0, 0, 0, uint32(nextAbs)))
			} else {
				copy(cpu.memory[at:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(0x40)))
			}
		}
		copy(cpu.memory[base+hops*0x40:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}

	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	build(cpu)

	// jitPersist stays false so jitExecute's freeJIT releases the code cache
	// and exec memory after each op: every iteration dispatches from cold.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = base
		cpu.running.Store(true)
		cpu.jitExecute()
	}
	b.StopTimer()
	b.ReportMetric(float64(hops), "hops/op")
}

// BenchmarkIE64_ConstLoad_JIT is the focused benchmark for constant-address
// LOAD/STORE elision (Technique 1). A tight backward loop performs a run of
// LOAD rd, K(R0) with compile-time constant addresses below IO_REGION_START,
// so each load's native path omits the I/O-region compare, window bound and
// I/O-page bitmap probe. The loop compiles into a single native block.
func BenchmarkIE64_ConstLoad_JIT(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available on this platform")
	}
	const nLoads = 16
	var instrs [][]byte
	for i := 0; i < nLoads; i++ {
		instrs = append(instrs, ie64Instr(OP_LOAD, byte(1+i%8), IE64_SIZE_Q, 0, 0, 0, uint32(0x100+i*8)))
	}
	// SUB.Q R10, R10, #1 (immediate: xbit=1, rs=R10).
	instrs = append(instrs, ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1))
	// BNE R10, R0, loop : backward branch to PROG_START.
	branchIdx := len(instrs)
	back := uint32(int32(-int64(branchIdx) * int64(IE64_INSTR_SIZE)))
	instrs = append(instrs, ie64Instr(OP_BNE, 0, 0, 0, 10, 0, back))
	totalInstrs := int(benchIterations) * len(instrs)

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
