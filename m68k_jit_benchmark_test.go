// m68k_jit_benchmark_test.go - M68020 JIT vs interpreter benchmark suite
//
// Benchmarks the M68K CPU through both the Go interpreter and the JIT compiler,
// reporting ns/op. MIPS can be derived: MIPS = instructions/op / ns/op * 1000.
//
// Workload categories:
//
//   - ALU:     Register-to-register integer arithmetic (MOVEQ, ADD, SUB, AND, OR, LSL)
//   - MemCopy: Memory copy loop using MOVE.L (A0)+,(A1)+
//   - Call:    Subroutine call/return (JSR + RTS) measuring block-exit cost
//   - Branch:  Conditional branches with mixed taken/not-taken patterns
//   - Mixed:   Interleaved ALU, memory, and branches
//
// Usage:
//
//	go test -tags headless -run='^$' -bench 'BenchmarkM68K_' -benchtime 3s ./...

//go:build amd64 && linux

package main

import (
	"testing"
)

import "runtime"

const (
	m68kBenchIterations = 10000
	m68kBenchDataAddr   = 0x5000
)

// runM68KBenchJIT runs the JIT until the program hits STOP, then halts.
func runM68KBenchJIT(cpu *M68KCPU, startPC uint32) {
	cpu.PC = startPC
	cpu.running.Store(true)
	cpu.stopped.Store(false)

	done := make(chan struct{})
	go func() {
		cpu.M68KExecuteJIT()
		close(done)
	}()

	// Poll for STOP state (the program ends with STOP)
	for !cpu.stopped.Load() {
		runtime.Gosched()
	}
	cpu.running.Store(false)
	<-done
}

// setupM68KJITBenchCPU creates a CPU for benchmarking.
func setupM68KJITBenchCPU() *M68KCPU {
	bus := NewMachineBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, 0x00010000) // SSP
	bus.Write32(4, 0x00001000) // PC
	cpu := NewM68KCPU(bus)
	cpu.SR = M68K_SR_S
	return cpu
}

// writeM68KProgram writes big-endian opcodes to memory at startPC.
func writeM68KProgram(cpu *M68KCPU, startPC uint32, opcodes ...uint16) uint32 {
	pc := startPC
	for _, op := range opcodes {
		cpu.memory[pc] = byte(op >> 8)
		cpu.memory[pc+1] = byte(op)
		pc += 2
	}
	return pc
}

// ===========================================================================
// ALU Benchmark: Register integer arithmetic
// ===========================================================================

func buildM68KALUProgram(cpu *M68KCPU) (startPC uint32, instrPerIter int) {
	startPC = 0x1000
	pc := startPC

	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	// D7 = iteration counter
	// MOVEQ #0,D0; MOVEQ #1,D1; MOVE.W #iterations-1,D7
	w(0x7000)                                // MOVEQ #0,D0
	w(0x7201)                                // MOVEQ #1,D1
	w(0x3E3C, uint16(m68kBenchIterations-1)) // MOVE.W #iter-1,D7

	// Loop body (at loop_top):
	loopTop := pc
	w(0xD081) // ADD.L D1,D0    (2)
	w(0x9081) // SUB.L D1,D0    (2)
	w(0xC081) // AND.L D1,D0    (2)
	w(0x8081) // OR.L D1,D0     (2)
	w(0xE188) // LSL.L #8,D0    (2) -- 1110 000 1 10 0 01 000 = nope
	// LSL.L #1,D0: 1110 001 1 10 0 01 000 = 0xE388
	// Actually let me use ADDQ #1,D0 instead for simplicity
	w(0x5280) // ADDQ.L #1,D0   (2)
	w(0x4840) // SWAP D0        (2)
	w(0x4840) // SWAP D0        (2) -- restore original order

	// DBRA D7,loop_top
	disp := int16(int32(loopTop) - int32(pc) - 2)
	w(0x51CF, uint16(disp)) // DBRA D7,loop_top (4)

	// STOP
	w(0x4E72, 0x2700)

	return startPC, 9 // 9 instructions per loop iteration (8 body + 1 DBRA)
}

// runM68KBenchInterpreter runs the interpreter until STOP, then forces halt.
func runM68KBenchInterpreter(cpu *M68KCPU, startPC uint32) {
	cpu.PC = startPC
	cpu.running.Store(true)
	cpu.stopped.Store(false)
	// Run instruction-by-instruction until stopped (STOP instruction)
	for cpu.running.Load() {
		if cpu.stopped.Load() {
			break
		}
		cpu.StepOne()
	}
	cpu.running.Store(false)
	cpu.stopped.Store(false)
}

func BenchmarkM68K_ALU_Interpreter(b *testing.B) {
	cpu := setupM68KJITBenchCPU()
	startPC, instrPerIter := buildM68KALUProgram(cpu)
	totalInstrs := (m68kBenchIterations) * instrPerIter

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runM68KBenchInterpreter(cpu, startPC)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

func BenchmarkM68K_ALU_JIT(b *testing.B) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available on this platform")
	}
	cpu := setupM68KJITBenchCPU()
	startPC, instrPerIter := buildM68KALUProgram(cpu)
	totalInstrs := (m68kBenchIterations) * instrPerIter

	cpu.m68kJitEnabled = true
	cpu.m68kJitPersist = true

	// Warm-up
	runM68KBenchJIT(cpu, startPC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runM68KBenchJIT(cpu, startPC)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// MemCopy Benchmark: Memory throughput with endian handling
// ===========================================================================

func buildM68KMemCopyProgram(cpu *M68KCPU) (startPC uint32, instrPerIter int) {
	startPC = 0x1000
	pc := startPC

	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	// Fill source data
	for i := uint32(0); i < uint32(m68kBenchIterations)*4; i += 4 {
		addr := m68kBenchDataAddr + i
		cpu.memory[addr] = byte(i >> 24)
		cpu.memory[addr+1] = byte(i >> 16)
		cpu.memory[addr+2] = byte(i >> 8)
		cpu.memory[addr+3] = byte(i)
	}

	// LEA source,A0; LEA dest,A1; MOVE.W #iter-1,D7
	w(0x41F9, uint16(m68kBenchDataAddr>>16), uint16(m68kBenchDataAddr&0xFFFF)) // LEA src,A0
	destAddr := m68kBenchDataAddr + uint32(m68kBenchIterations)*4
	w(0x43F9, uint16(destAddr>>16), uint16(destAddr&0xFFFF)) // LEA dest,A1
	w(0x3E3C, uint16(m68kBenchIterations-1))                 // MOVE.W #iter-1,D7

	// Loop: MOVE.L (A0)+,(A1)+; DBRA D7,loop
	loopTop := pc
	w(0x22D8) // MOVE.L (A0)+,(A1)+

	disp := int16(int32(loopTop) - int32(pc) - 2)
	w(0x51CF, uint16(disp)) // DBRA D7,loop

	w(0x4E72, 0x2700) // STOP

	return startPC, 2 // 2 instructions per iteration
}

func BenchmarkM68K_MemCopy_Interpreter(b *testing.B) {
	cpu := setupM68KJITBenchCPU()
	startPC, instrPerIter := buildM68KMemCopyProgram(cpu)
	totalInstrs := m68kBenchIterations * instrPerIter

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runM68KBenchInterpreter(cpu, startPC)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

func BenchmarkM68K_MemCopy_JIT(b *testing.B) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available on this platform")
	}
	cpu := setupM68KJITBenchCPU()
	startPC, instrPerIter := buildM68KMemCopyProgram(cpu)
	totalInstrs := m68kBenchIterations * instrPerIter

	cpu.m68kJitEnabled = true
	cpu.m68kJitPersist = true
	runM68KBenchJIT(cpu, startPC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runM68KBenchJIT(cpu, startPC)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// Call Benchmark: Subroutine call/return overhead
// ===========================================================================

func buildM68KCallProgram(cpu *M68KCPU) (startPC uint32, instrPerIter int) {
	startPC = 0x1000
	pc := startPC

	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	cpu.AddrRegs[7] = 0x10000 // stack pointer

	w(0x3E3C, uint16(m68kBenchIterations-1)) // MOVE.W #iter-1,D7

	// Loop: JSR sub; DBRA D7,loop
	loopTop := pc
	subAddr := uint32(0x2000)
	w(0x4EB9, uint16(subAddr>>16), uint16(subAddr&0xFFFF)) // JSR sub

	disp := int16(int32(loopTop) - int32(pc) - 2)
	w(0x51CF, uint16(disp)) // DBRA D7,loop

	w(0x4E72, 0x2700) // STOP

	// Subroutine at 0x2000: MOVEQ #1,D0; RTS
	pc = 0x2000
	w(0x7001) // MOVEQ #1,D0
	w(0x4E75) // RTS

	return startPC, 4 // JSR + MOVEQ + RTS + DBRA per iteration
}

func BenchmarkM68K_Call_Interpreter(b *testing.B) {
	cpu := setupM68KJITBenchCPU()
	startPC, instrPerIter := buildM68KCallProgram(cpu)
	totalInstrs := m68kBenchIterations * instrPerIter

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.AddrRegs[7] = 0x10000
		runM68KBenchInterpreter(cpu, startPC)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

func BenchmarkM68K_Call_JIT(b *testing.B) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available on this platform")
	}
	cpu := setupM68KJITBenchCPU()
	startPC, instrPerIter := buildM68KCallProgram(cpu)
	totalInstrs := m68kBenchIterations * instrPerIter

	cpu.m68kJitEnabled = true
	cpu.m68kJitPersist = true
	cpu.AddrRegs[7] = 0x10000
	runM68KBenchJIT(cpu, startPC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.AddrRegs[7] = 0x10000
		runM68KBenchJIT(cpu, startPC)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// Chain-Specific Benchmarks
// ===========================================================================

// BenchmarkM68K_Chain_BRA benchmarks two blocks BRA-ing to each other,
// measuring pure block chaining overhead.
func buildM68KChainBRAProgram(cpu *M68KCPU) (startPC uint32, instrPerIter int) {
	startPC = 0x1000

	w := func(pc uint32, ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	// Block A at 0x1000: SUBQ #1,D7; BEQ STOP; BRA 0x2000
	// Block B at 0x2000: SUBQ #1,D7; BEQ STOP; BRA 0x1000
	// STOP at 0x3000

	// Block A at 0x1000: SUBQ(2B) + BEQ.W(4B) + BRA.W(4B) = 10 bytes
	// BEQ.W at 0x1002: instrPC+2=0x1004, target 0x3000, disp=0x1FFC
	// BRA.W at 0x1006: instrPC+2=0x1008, target 0x2000, disp=0x0FF8
	w(0x1000, 0x5387, 0x6700, 0x1FFC, 0x6000, 0x0FF8)

	// Block B at 0x2000: SUBQ(2B) + BEQ.W(4B) + BRA.W(4B) = 10 bytes
	// BEQ.W at 0x2002: instrPC+2=0x2004, target 0x3000, disp=0x0FFC
	// BRA.W at 0x2006: instrPC+2=0x2008, target 0x1000, disp=0xEFF8
	w(0x2000, 0x5387, 0x6700, 0x0FFC, 0x6000, 0xEFF8)

	// STOP at 0x3000
	w(0x3000, 0x4E72, 0x2700)

	cpu.DataRegs[7] = uint32(m68kBenchIterations)
	return startPC, 3 // SUBQ + BEQ + BRA per iteration (2 iterations per pair)
}

func BenchmarkM68K_Chain_BRA_JIT(b *testing.B) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available on this platform")
	}
	cpu := setupM68KJITBenchCPU()
	startPC, instrPerIter := buildM68KChainBRAProgram(cpu)
	totalInstrs := m68kBenchIterations * instrPerIter

	cpu.m68kJitEnabled = true
	cpu.m68kJitPersist = true
	runM68KBenchJIT(cpu, startPC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.DataRegs[7] = uint32(m68kBenchIterations)
		runM68KBenchJIT(cpu, startPC)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

func BenchmarkM68K_Chain_BRA_Interpreter(b *testing.B) {
	cpu := setupM68KJITBenchCPU()
	startPC, instrPerIter := buildM68KChainBRAProgram(cpu)
	totalInstrs := m68kBenchIterations * instrPerIter

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.DataRegs[7] = uint32(m68kBenchIterations)
		runM68KBenchInterpreter(cpu, startPC)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// BenchmarkM68K_LazyCCR_CMP_Bcc benchmarks CMP+BEQ in a tight loop,
// measuring the benefit of lazy CCR (direct Jcc from host EFLAGS).
func buildM68KLazyCCRProgram(cpu *M68KCPU) (startPC uint32, instrPerIter int) {
	startPC = 0x1000
	pc := startPC

	w := func(ops ...uint16) {
		for _, op := range ops {
			cpu.memory[pc] = byte(op >> 8)
			cpu.memory[pc+1] = byte(op)
			pc += 2
		}
	}

	// D0 counts up, D1 = target (never reached), D7 = loop counter
	cpu.DataRegs[0] = 0
	cpu.DataRegs[1] = 0xFFFFFFFF // never equal

	w(0x3E3C, uint16(m68kBenchIterations-1)) // MOVE.W #iter-1,D7

	loopTop := pc
	w(0x5280) // ADDQ.L #1,D0
	w(0xB081) // CMP.L D1,D0
	w(0x6702) // BEQ.B +2 (never taken)
	w(0x4E71) // NOP (fall-through target)

	disp := int16(int32(loopTop) - int32(pc) - 2)
	w(0x51CF, uint16(disp)) // DBRA D7,loop

	w(0x4E72, 0x2700) // STOP

	return startPC, 5 // ADDQ + CMP + BEQ + NOP + DBRA per iteration
}

func BenchmarkM68K_LazyCCR_CMP_Bcc_JIT(b *testing.B) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available on this platform")
	}
	cpu := setupM68KJITBenchCPU()
	startPC, instrPerIter := buildM68KLazyCCRProgram(cpu)
	totalInstrs := m68kBenchIterations * instrPerIter

	cpu.m68kJitEnabled = true
	cpu.m68kJitPersist = true
	runM68KBenchJIT(cpu, startPC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.DataRegs[0] = 0
		cpu.DataRegs[1] = 0xFFFFFFFF
		runM68KBenchJIT(cpu, startPC)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

func BenchmarkM68K_LazyCCR_CMP_Bcc_Interpreter(b *testing.B) {
	cpu := setupM68KJITBenchCPU()
	startPC, instrPerIter := buildM68KLazyCCRProgram(cpu)
	totalInstrs := m68kBenchIterations * instrPerIter

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.DataRegs[0] = 0
		cpu.DataRegs[1] = 0xFFFFFFFF
		runM68KBenchInterpreter(cpu, startPC)
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}
