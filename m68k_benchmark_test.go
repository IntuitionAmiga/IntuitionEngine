package main

import (
	"testing"
)

// =============================================================================
// M68K CPU Benchmark Suite
// Measures instruction execution performance for various operation types
// Run with: go test -bench=. -benchmem -count=3
// =============================================================================

// setupBenchCPU creates a CPU instance for benchmarking (without banner)
func setupBenchCPU() *M68KCPU {
	// Skip boilerPlateTest() for faster benchmark setup
	bus := NewSystemBus()
	termOut := NewTerminalOutput()
	bus.MapIO(TERM_OUT, TERM_OUT, nil, termOut.HandleWrite)
	bus.Write32(0, M68K_STACK_START)
	bus.Write32(M68K_RESET_VECTOR, M68K_ENTRY_POINT)
	return NewM68KCPU(bus)
}

// =============================================================================
// Data Movement Benchmarks
// =============================================================================

// BenchmarkMoveRegToReg measures MOVE Dn,Dm throughput
func BenchmarkMoveRegToReg(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 0x12345678

	// MOVE.L D0,D1: 0x2200
	cpu.Write16(0x1000, 0x2200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkMoveq measures MOVEQ throughput (fast immediate move)
func BenchmarkMoveq(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S

	// MOVEQ #42,D0: 0x702A
	cpu.Write16(0x1000, 0x702A)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkMoveMemToReg measures memory-to-register MOVE throughput
func BenchmarkMoveMemToReg(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[0] = 0x2000
	cpu.Write32(0x2000, 0xDEADBEEF)

	// MOVE.L (A0),D0: 0x2010
	cpu.Write16(0x1000, 0x2010)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkMoveRegToMem measures register-to-memory MOVE throughput
func BenchmarkMoveRegToMem(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[0] = 0x2000
	cpu.DataRegs[0] = 0x12345678

	// MOVE.L D0,(A0): 0x2080
	cpu.Write16(0x1000, 0x2080)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// =============================================================================
// Arithmetic Benchmarks
// =============================================================================

// BenchmarkAddLong measures ADD.L throughput
func BenchmarkAddLong(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 100
	cpu.DataRegs[1] = 200

	// ADD.L D0,D1: 0xD280
	cpu.Write16(0x1000, 0xD280)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[1] = 200 // Reset to avoid overflow
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkSubLong measures SUB.L throughput
func BenchmarkSubLong(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 100
	cpu.DataRegs[1] = 500

	// SUB.L D0,D1: 0x9280
	cpu.Write16(0x1000, 0x9280)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[1] = 500
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkMultiplySigned measures MULS throughput (signed multiply)
func BenchmarkMultiplySigned(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 100
	cpu.DataRegs[1] = 50

	// MULS D1,D0: 0xC1C1
	cpu.Write16(0x1000, 0xC1C1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[0] = 100
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkMultiplyUnsigned measures MULU throughput (unsigned multiply)
func BenchmarkMultiplyUnsigned(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 100
	cpu.DataRegs[1] = 50

	// MULU D1,D0: 0xC0C1
	cpu.Write16(0x1000, 0xC0C1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[0] = 100
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkDivideSigned measures DIVS throughput (signed divide)
func BenchmarkDivideSigned(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 10000
	cpu.DataRegs[1] = 100

	// DIVS D1,D0: 0x81C1
	cpu.Write16(0x1000, 0x81C1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[0] = 10000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkDivideUnsigned measures DIVU throughput (unsigned divide)
func BenchmarkDivideUnsigned(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 10000
	cpu.DataRegs[1] = 100

	// DIVU D1,D0: 0x80C1
	cpu.Write16(0x1000, 0x80C1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[0] = 10000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkCmpLong measures CMP.L throughput
func BenchmarkCmpLong(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 100
	cpu.DataRegs[1] = 100

	// CMP.L D0,D1: 0xB280
	cpu.Write16(0x1000, 0xB280)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// =============================================================================
// Logical Operation Benchmarks
// =============================================================================

// BenchmarkAndLong measures AND.L throughput
func BenchmarkAndLong(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 0xFF00FF00
	cpu.DataRegs[1] = 0x0FF00FF0

	// AND.L D0,D1: 0xC280
	cpu.Write16(0x1000, 0xC280)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[1] = 0x0FF00FF0
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkOrLong measures OR.L throughput
func BenchmarkOrLong(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 0xFF00FF00
	cpu.DataRegs[1] = 0x00FF00FF

	// OR.L D0,D1: 0x8280
	cpu.Write16(0x1000, 0x8280)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[1] = 0x00FF00FF
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkEorLong measures EOR.L throughput
func BenchmarkEorLong(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 0xAAAAAAAA
	cpu.DataRegs[1] = 0x55555555

	// EOR.L D0,D1: 0xB180
	cpu.Write16(0x1000, 0xB180)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[1] = 0x55555555
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkNotLong measures NOT.L throughput
func BenchmarkNotLong(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 0xAAAAAAAA

	// NOT.L D0: 0x4680
	cpu.Write16(0x1000, 0x4680)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// =============================================================================
// Shift/Rotate Benchmarks
// =============================================================================

// BenchmarkLslImmediate measures LSL with immediate count
func BenchmarkLslImmediate(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 0x00000001

	// LSL.L #8,D0: 0xE188
	cpu.Write16(0x1000, 0xE188)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[0] = 0x00000001
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkLsrImmediate measures LSR with immediate count
func BenchmarkLsrImmediate(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 0x80000000

	// LSR.L #8,D0: 0xE088
	cpu.Write16(0x1000, 0xE088)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[0] = 0x80000000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkRolImmediate measures ROL with immediate count
func BenchmarkRolImmediate(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 0x12345678

	// ROL.L #4,D0: 0xE998
	cpu.Write16(0x1000, 0xE998)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[0] = 0x12345678
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkRorImmediate measures ROR with immediate count
func BenchmarkRorImmediate(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 0x12345678

	// ROR.L #4,D0: 0xE898
	cpu.Write16(0x1000, 0xE898)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[0] = 0x12345678
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// =============================================================================
// Branch Benchmarks
// =============================================================================

// BenchmarkBraTaken measures BRA throughput (always taken)
func BenchmarkBraTaken(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S

	// BRA.S +0 (branch to next instruction): 0x6000
	// Use word displacement to avoid looping
	cpu.Write16(0x1000, 0x6000)
	cpu.Write16(0x1002, 0x0002) // displacement +2

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkBeqTaken measures BEQ throughput (branch taken)
func BenchmarkBeqTaken(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S | M68K_SR_Z // Z=1 so BEQ will branch

	// BEQ.S +8: 0x6708
	cpu.Write16(0x1000, 0x6708)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S | M68K_SR_Z
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkBeqNotTaken measures BEQ throughput (branch not taken)
func BenchmarkBeqNotTaken(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S // Z=0 so BEQ won't branch

	// BEQ.S +8: 0x6708
	cpu.Write16(0x1000, 0x6708)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.SR = M68K_SR_S
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkDbra measures DBRA throughput (decrement and branch)
func BenchmarkDbra(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 1000

	// DBRA D0,-4: 0x51C8 FFFC
	cpu.Write16(0x1000, 0x51C8)
	cpu.Write16(0x1002, 0xFFFC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[0] = 1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// =============================================================================
// Bit Field Benchmarks (68020+)
// =============================================================================

// BenchmarkBfextract measures BFEXTU throughput
func BenchmarkBfextract(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 0x12345678

	// BFEXTU D0{8:8},D1: 0xE9C0 0841
	cpu.Write16(0x1000, 0xE9C0)
	cpu.Write16(0x1002, 0x0841) // offset=8, width=8, dest=D1

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkBfinsert measures BFINS throughput
func BenchmarkBfinsert(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 0x00000000
	cpu.DataRegs[1] = 0xFF

	// BFINS D1,D0{8:8}: 0xEFC0 0841
	cpu.Write16(0x1000, 0xEFC0)
	cpu.Write16(0x1002, 0x0841)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[0] = 0x00000000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// =============================================================================
// Complex Instruction Sequence Benchmarks
// =============================================================================

// BenchmarkArithmeticSequence measures a typical arithmetic sequence
func BenchmarkArithmeticSequence(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.SR = M68K_SR_S

	// Set up instruction sequence:
	// MOVEQ #10,D0
	// MOVEQ #20,D1
	// ADD.L D0,D1
	// MULU D0,D1
	cpu.Write16(0x1000, 0x700A) // MOVEQ #10,D0
	cpu.Write16(0x1002, 0x7214) // MOVEQ #20,D1
	cpu.Write16(0x1004, 0xD280) // ADD.L D0,D1
	cpu.Write16(0x1006, 0xC2C0) // MULU D0,D1

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		// Execute 4 instructions
		for j := 0; j < 4; j++ {
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}
	}
}

// BenchmarkMemoryCopySequence measures memory-to-memory operations
func BenchmarkMemoryCopySequence(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[0] = 0x2000 // Source
	cpu.AddrRegs[1] = 0x3000 // Dest

	// Pre-fill source memory
	for i := uint32(0); i < 16; i++ {
		cpu.Write32(0x2000+i*4, 0x12345678+i)
	}

	// MOVE.L (A0)+,(A1)+: 0x22D8
	cpu.Write16(0x1000, 0x22D8)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.AddrRegs[0] = 0x2000
		cpu.AddrRegs[1] = 0x3000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkLoopConstruct measures a typical counting loop
func BenchmarkLoopConstruct(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.SR = M68K_SR_S

	// Loop: ADDQ #1,D1 / DBRA D0,loop
	// 0x1000: ADDQ.L #1,D1 (5281)
	// 0x1002: DBRA D0,loop (51C8 FFFC)
	cpu.Write16(0x1000, 0x5281) // ADDQ.L #1,D1
	cpu.Write16(0x1002, 0x51C8) // DBRA D0
	cpu.Write16(0x1004, 0xFFFC) // displacement -4

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[0] = 10 // 10 iterations
		cpu.DataRegs[1] = 0
		// Execute complete loop (11 iterations: 10 branches + exit)
		// DBRA decrements and branches if result != -1
		// Loop exits when D0 becomes 0xFFFF (word) which is -1
		for j := 0; j < 22; j++ { // 11 iterations * 2 instructions
			cpu.currentIR = cpu.Fetch16()
			cpu.FetchAndDecodeInstruction()
		}
	}
}

// BenchmarkSubroutineCall measures JSR/RTS overhead
func BenchmarkSubroutineCall(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.SR = M68K_SR_S
	cpu.AddrRegs[7] = 0x10000 // Stack pointer well above stackLowerBound

	// 0x1000: JSR 0x2000 (4EB9 00002000)
	// 0x2000: RTS (4E75)
	cpu.Write16(0x1000, 0x4EB9)
	cpu.Write32(0x1002, 0x2000)
	cpu.Write16(0x2000, 0x4E75) // RTS

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.AddrRegs[7] = 0x10000
		// JSR
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
		// RTS
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkNop measures NOP throughput (baseline instruction overhead)
func BenchmarkNop(b *testing.B) {
	cpu := setupBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S

	// NOP: 0x4E71
	cpu.Write16(0x1000, 0x4E71)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}
