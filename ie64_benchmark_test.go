// ie64_benchmark_test.go - IE64 CPU benchmark suite
//
// This file benchmarks the IE64 CPU core through both the Go interpreter and
// the JIT compiler (ARM64/x86-64), reporting ns/op and instructions/op for
// five workload categories. MIPS can be derived: MIPS = instructions/op / ns/op * 1000.
//
// Workload categories:
//
//   - ALU:    Register-to-register integer arithmetic (ADD, SUB, MULU, AND, OR, LSL)
//   - FPU:    Floating-point arithmetic via FPU registers (FADD, FSUB, FMUL, FDIV)
//   - Memory: LOAD/STORE in a sequential-access loop below IO_REGION_START
//   - Mixed:  Interleaved ALU, FPU, and memory operations
//   - Call:   Subroutine call/return (JSR + RTS) measuring JIT block-exit cost
//
// Each workload has an _Interpreter and a _JIT variant. JIT benchmarks skip
// automatically on platforms without JIT support.
//
// Usage:
//
//	# Run all IE64 benchmarks (skip normal tests)
//	go test -tags headless -run='^$' -bench BenchmarkIE64_ -benchtime 3s ./...
//
//	# Compare JIT vs interpreter side-by-side
//	go test -tags headless -run='^$' -bench 'BenchmarkIE64_(ALU|FPU|Memory|Mixed|Call)' -benchtime 3s ./...
//
//	# JIT only
//	go test -tags headless -run='^$' -bench 'BenchmarkIE64_.*_JIT' -benchtime 3s ./...

package main

import (
	"testing"
)

// ===========================================================================
// Benchmark Constants
// ===========================================================================

const (
	// benchIterations is the default loop count for benchmark programs.
	// Chosen to produce ~100K instructions per run (~5-50ms depending on backend).
	benchIterations = 10000

	// benchDataAddr is the base address for memory benchmarks, well below
	// IO_REGION_START so all accesses take the direct-memory fast path.
	// With 10,000 iterations at 8 bytes each, the last address is ~0x18A00,
	// well below IO_REGION_START (0xA0000).
	benchDataAddr = 0x5000
)

// neg16 is -16 encoded as uint32, used for backward branches targeting
// two instructions back (2 * IE64_INSTR_SIZE = 16 bytes).
var neg16 = uint32(0xFFFFFFF0)

// ===========================================================================
// Program Builders
// ===========================================================================

// buildALUProgram constructs an IE64 program that executes a tight integer
// ALU loop for the given number of iterations. Each iteration performs 8
// register-to-register operations using R1-R8:
//
//	+0:   ADD.Q  R3, R1, R2      ; 64-bit addition
//	+8:   SUB.Q  R4, R3, R1      ; 64-bit subtraction
//	+16:  MULU.Q R5, R3, R4      ; unsigned multiply
//	+24:  AND.Q  R6, R5, R2      ; bitwise AND
//	+32:  OR.Q   R7, R6, R1      ; bitwise OR
//	+40:  LSL.Q  R8, R7, #3      ; logical shift left by 3
//	+48:  ADD.Q  R1, R1, R8      ; feed back into R1
//	+56:  SUB.Q  R10, R10, #1    ; decrement loop counter
//	+64:  BNE    R10, R0, -64    ; loop back to +0
//	+72:  HALT
//
// Total instructions per run: iterations * 9 + 3 (MOVE R10 + MOVE R1 + MOVE R2).
// The backward branch (BNE) forms a single JIT block with a native loop
// and 4095-iteration budget counter.
func buildALUProgram(iterations uint32) (instrs [][]byte, totalInstrs int) {
	neg64 := uint32(0xFFFFFFC0) // -64
	instrs = [][]byte{
		// Setup (3 instructions, outside the loop)
		ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, iterations), // R10 = iterations
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 7),           // R1 = 7
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 3),           // R2 = 3
		// Loop body (8 ALU + 1 SUB counter + 1 BNE = 10 per iteration, but the
		// last iteration's BNE falls through so effectively 9 loop instrs + BNE)
		ie64Instr(OP_ADD, 3, IE64_SIZE_Q, 0, 1, 2, 0),   // R3 = R1 + R2
		ie64Instr(OP_SUB, 4, IE64_SIZE_Q, 0, 3, 1, 0),   // R4 = R3 - R1
		ie64Instr(OP_MULU, 5, IE64_SIZE_Q, 0, 3, 4, 0),  // R5 = R3 * R4
		ie64Instr(OP_AND64, 6, IE64_SIZE_Q, 0, 5, 2, 0), // R6 = R5 & R2
		ie64Instr(OP_OR64, 7, IE64_SIZE_Q, 0, 6, 1, 0),  // R7 = R6 | R1
		ie64Instr(OP_LSL, 8, IE64_SIZE_Q, 1, 7, 0, 3),   // R8 = R7 << 3
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 0, 1, 8, 0),   // R1 = R1 + R8
		ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1), // R10 -= 1
		ie64Instr(OP_BNE, 0, 0, 0, 10, 0, neg64),        // BNE R10, R0, -64
	}
	// 3 setup + iterations * 9 (the loop body) + 1 HALT (appended by harness)
	totalInstrs = 3 + int(iterations)*9 + 1
	return
}

// buildFPUProgram constructs an IE64 program that exercises floating-point
// arithmetic in a tight loop. Each iteration performs 5 FP operations:
//
//	+0:   FADD F4, F1, F2    ; F4 = F1 + F2
//	+8:   FSUB F5, F4, F3    ; F5 = F4 - F3
//	+16:  FMUL F6, F5, F1    ; F6 = F5 * F1
//	+24:  FDIV F7, F6, F2    ; F7 = F6 / F2
//	+32:  FADD F1, F7, F3    ; F1 = F7 + F3 (feedback)
//	+40:  SUB.Q R10, R10, #1 ; decrement counter
//	+48:  BNE R10, R0, -48   ; loop
//	+56:  HALT
//
// FP registers are pre-loaded via FMOVECR (ROM constant table) before the
// loop. On JIT platforms, FADD/FSUB/FMUL/FDIV compile to native SSE (x86-64)
// or ARM64 FP instructions. FDIV is the most expensive single operation.
//
// Total instructions per run: 4 (setup) + iterations * 7 + 1 (HALT).
func buildFPUProgram(iterations uint32) (instrs [][]byte, totalInstrs int) {
	neg48 := uint32(0xFFFFFFD0) // -48
	instrs = [][]byte{
		// Setup: load FP constants from ROM table
		ie64Instr(OP_FMOVECR, 1, 0, 0, 0, 0, 8),                  // F1 = 1.0
		ie64Instr(OP_FMOVECR, 2, 0, 0, 0, 0, 9),                  // F2 = 2.0
		ie64Instr(OP_FMOVECR, 3, 0, 0, 0, 0, 0),                  // F3 = pi
		ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, iterations), // R10 = iterations
		// Loop body (5 FP + 1 SUB + 1 BNE = 7 per iteration)
		ie64Instr(OP_FADD, 4, 0, 0, 1, 2, 0),            // F4 = F1 + F2
		ie64Instr(OP_FSUB, 5, 0, 0, 4, 3, 0),            // F5 = F4 - F3
		ie64Instr(OP_FMUL, 6, 0, 0, 5, 1, 0),            // F6 = F5 * F1
		ie64Instr(OP_FDIV, 7, 0, 0, 6, 2, 0),            // F7 = F6 / F2
		ie64Instr(OP_FADD, 1, 0, 0, 7, 3, 0),            // F1 = F7 + F3
		ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1), // R10 -= 1
		ie64Instr(OP_BNE, 0, 0, 0, 10, 0, neg48),        // loop
	}
	totalInstrs = 4 + int(iterations)*7 + 1
	return
}

// buildMemoryProgram constructs an IE64 program that exercises LOAD and STORE
// in a sequential-access loop. All addresses are below IO_REGION_START, so
// they take the direct-memory fast path after the range check (CMP + JAE).
// The bitmap lookup in the JIT slow path is never reached.
//
// Addresses advance linearly from benchDataAddr (0x5000) by 8 bytes per
// iteration. With 10,000 iterations the last address is ~0x18A00, well below
// IO_REGION_START (0xA0000).
//
//	+0:   STORE.Q R10, 0(R1)     ; write counter to memory
//	+8:   LOAD.Q  R3, 0(R1)     ; read it back
//	+16:  ADD.Q   R2, R2, R3    ; accumulate
//	+24:  ADD.Q   R1, R1, #8    ; advance address
//	+32:  SUB.Q   R10, R10, #1  ; decrement counter
//	+40:  BNE     R10, R0, -40  ; loop
//	+48:  HALT
//
// Total instructions per run: 3 (setup) + iterations * 6 + 1 (HALT).
func buildMemoryProgram(iterations uint32) (instrs [][]byte, totalInstrs int) {
	neg40 := uint32(0xFFFFFFD8) // -40
	instrs = [][]byte{
		ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, iterations),   // R10 = iterations
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, benchDataAddr), // R1 = base addr
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0),             // R2 = accumulator
		// Loop body
		ie64Instr(OP_STORE, 10, IE64_SIZE_Q, 0, 1, 0, 0), // mem[R1] = R10
		ie64Instr(OP_LOAD, 3, IE64_SIZE_Q, 0, 1, 0, 0),   // R3 = mem[R1]
		ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 0, 2, 3, 0),    // R2 += R3
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 8),    // R1 += 8
		ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1),  // R10 -= 1
		ie64Instr(OP_BNE, 0, 0, 0, 10, 0, neg40),         // loop
	}
	totalInstrs = 3 + int(iterations)*6 + 1
	return
}

// buildMixedProgram constructs an IE64 program that interleaves integer ALU,
// floating-point, and memory operations in each loop iteration. This is
// representative of real program behavior where different functional units
// are exercised in close succession.
//
// Addresses advance linearly from benchDataAddr, same as buildMemoryProgram.
//
//	+0:   LOAD.Q  R3, 0(R1)      ; load from memory
//	+8:   ADD.Q   R3, R3, #1     ; increment
//	+16:  STORE.Q R3, 0(R1)      ; store back
//	+24:  FADD    F3, F1, F2     ; FP add
//	+32:  FMUL    F1, F3, F2     ; FP multiply (feedback)
//	+40:  ADD.Q   R1, R1, #8     ; advance address
//	+48:  SUB.Q   R10, R10, #1   ; decrement counter
//	+56:  BNE     R10, R0, -56   ; loop
//	+64:  HALT
//
// Total instructions per run: 5 (setup) + iterations * 8 + 1 (HALT).
func buildMixedProgram(iterations uint32) (instrs [][]byte, totalInstrs int) {
	neg56 := uint32(0xFFFFFFC8) // -56
	instrs = [][]byte{
		ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, iterations),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, benchDataAddr),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0),
		ie64Instr(OP_FMOVECR, 1, 0, 0, 0, 0, 8), // F1 = 1.0
		ie64Instr(OP_FMOVECR, 2, 0, 0, 0, 0, 9), // F2 = 2.0
		// Loop body
		ie64Instr(OP_LOAD, 3, IE64_SIZE_Q, 0, 1, 0, 0),
		ie64Instr(OP_ADD, 3, IE64_SIZE_Q, 1, 3, 0, 1),
		ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 0, 1, 0, 0),
		ie64Instr(OP_FADD, 3, 0, 0, 1, 2, 0),
		ie64Instr(OP_FMUL, 1, 0, 0, 3, 2, 0),
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 8),
		ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1),
		ie64Instr(OP_BNE, 0, 0, 0, 10, 0, neg56),
	}
	totalInstrs = 5 + int(iterations)*8 + 1
	return
}

// buildCallProgram constructs an IE64 program that repeatedly calls a short
// subroutine via JSR/RTS. This measures JIT block-exit overhead: every JSR
// and RTS terminates the current JIT block and returns to the dispatcher,
// so this workload is dominated by prologue/epilogue and dispatch cost rather
// than ALU throughput.
//
//	+0:   MOVE.Q  R10, #iterations ; loop counter
//	+8:   MOVE.Q  R31, #STACK_START ; initialize SP
//	+16:  MOVE.Q  R1, #0            ; accumulator
//	--- loop ---
//	+24:  JSR     +16               ; call subroutine at +40
//	+32:  SUB.Q   R10, R10, #1
//	+40:  BNE     R10, R0, -16      ; loop back to +24
//	+48:  BRA     +16               ; skip subroutine, jump to HALT at +64
//	--- subroutine (at +56) ---
//	+56:  ADD.Q   R1, R1, #1        ; increment accumulator
//	+64:  RTS                        ; return
//	--- exit ---
//	+72:  HALT
//
// Total instructions per run: 3 (setup) + iterations * 5 (JSR+ADD+RTS+SUB+BNE) + 2 (BRA+HALT).
func buildCallProgram(iterations uint32) (instrs [][]byte, totalInstrs int) {
	// Layout (byte offsets from PROG_START):
	//   0:  MOVE R10, #iterations
	//   8:  MOVE R31, #STACK_START
	//   16: MOVE R1, #0
	//   24: JSR +32               → target = 24+32 = 56 (subroutine)
	//   32: SUB R10, R10, #1
	//   40: BNE R10, R0, -16     → target = 40-16 = 24 (back to JSR)
	//   48: BRA +24               → target = 48+24 = 72 (HALT, appended by harness)
	//   56: ADD R1, R1, #1        (subroutine body)
	//   64: RTS                   (return to 32)
	//   72: HALT                  (appended by loadBenchProgram)
	instrs = [][]byte{
		ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, iterations),
		ie64Instr(OP_MOVE, 31, IE64_SIZE_Q, 1, 0, 0, STACK_START),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0),
		ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 32),          // JSR +32 → offset 56
		ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1), // R10 -= 1
		ie64Instr(OP_BNE, 0, 0, 0, 10, 0, neg16),        // BNE → offset 24
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 24),            // BRA +24 → offset 72 (HALT)
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 1),   // R1 += 1
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),           // return
	}
	// 3 setup + iterations*(JSR+ADD+RTS+SUB+BNE=5) + BRA + HALT = 3 + iter*5 + 2
	totalInstrs = 3 + int(iterations)*5 + 2
	return
}

// ===========================================================================
// Benchmark Harness Helpers
// ===========================================================================

// loadBenchProgram loads IE64 instructions into CPU memory at PROG_START
// and appends a HALT terminator.
func loadBenchProgram(cpu *CPU64, instrs [][]byte) {
	offset := uint32(PROG_START)
	for _, instr := range instrs {
		copy(cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}
	copy(cpu.memory[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

// setupJITBench prepares a CPU for JIT benchmarking: enables JIT, loads the
// program, runs one warm-up iteration to populate the code cache, then sets
// jitPersist so the cache and exec memory survive across b.N iterations.
// Returns a cleanup function to call via b.Cleanup.
func setupJITBench(b *testing.B, cpu *CPU64, instrs [][]byte, resetState func()) {
	b.Helper()
	cpu.jitEnabled = true
	cpu.jitPersist = true // prevent freeJIT from releasing state between runs
	loadBenchProgram(cpu, instrs)

	// Warm-up: run once to compile all JIT blocks into the code cache.
	// jitPersist is already set, so the defer freeJIT() inside ExecuteJIT
	// is a no-op and the compiled blocks survive for the timed iterations.
	resetState()
	cpu.jitExecute()

	b.Cleanup(func() {
		cpu.jitPersist = false
		cpu.freeJIT()
	})
}

// reportMIPS computes and reports MIPS from known instruction count and elapsed time.
func reportMIPS(b *testing.B, totalInstrs int) {
	b.Helper()
	b.ReportMetric(float64(totalInstrs), "instructions/op")
	// MIPS = (totalInstrs * N) / elapsed_seconds / 1e6
	// Go's benchmark framework provides ns/op; MIPS = instructions/op / (ns/op) * 1000
	// We report instructions/op and let the user derive MIPS, or we can approximate
	// using the total elapsed time from the benchmark.
}

// ===========================================================================
// ALU Benchmarks
// ===========================================================================

// BenchmarkIE64_ALU_Interpreter measures integer ALU throughput through the
// Go interpreter loop. The workload is a tight loop of 8 register-to-register
// operations (ADD, SUB, MULU, AND, OR, LSL) repeated 10,000 times.
// This isolates the interpreter's per-instruction dispatch overhead without
// memory or I/O pressure.
//
// Compare with BenchmarkIE64_ALU_JIT to measure JIT speedup.
func BenchmarkIE64_ALU_Interpreter(b *testing.B) {
	instrs, totalInstrs := buildALUProgram(benchIterations)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	loadBenchProgram(cpu, instrs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.regs[1] = 7
		cpu.regs[2] = 3
		cpu.running.Store(true)
		cpu.Execute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// BenchmarkIE64_ALU_JIT measures the same integer ALU workload compiled to
// native code via the JIT compiler. The 10,000-iteration backward loop
// compiles into a single JIT block with a native backward branch and budget
// counter, so this primarily measures native ALU throughput plus the
// prologue/epilogue overhead amortized over the loop body.
//
// Compare with BenchmarkIE64_ALU_Interpreter to measure JIT speedup.
func BenchmarkIE64_ALU_JIT(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available on this platform")
	}
	instrs, totalInstrs := buildALUProgram(benchIterations)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	resetState := func() {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.regs[1] = 7
		cpu.regs[2] = 3
		cpu.running.Store(true)
	}
	setupJITBench(b, cpu, instrs, resetState)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetState()
		cpu.jitExecute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// FPU Benchmarks
// ===========================================================================

// BenchmarkIE64_FPU_Interpreter measures floating-point throughput through
// the interpreter. Each iteration performs FADD, FSUB, FMUL, FDIV — the four
// core FP operations. FDIV is the most expensive single operation and
// dominates the per-iteration cost.
//
// Compare with BenchmarkIE64_FPU_JIT to measure native SSE/ARM64 FP speedup.
func BenchmarkIE64_FPU_Interpreter(b *testing.B) {
	instrs, totalInstrs := buildFPUProgram(benchIterations)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	loadBenchProgram(cpu, instrs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.FPU.FPRegs = [16]uint32{} // reset FP state
		cpu.running.Store(true)
		cpu.Execute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// BenchmarkIE64_FPU_JIT measures floating-point throughput compiled to native
// SSE (x86-64) or ARM64 FP instructions. FADD/FSUB/FMUL/FDIV map directly
// to ADDSS/SUBSS/MULSS/DIVSS on x86-64 and to native ARM64 S-register
// instructions. The JIT loads FP register values from the FPU struct into
// XMM/S registers, operates, and stores back.
func BenchmarkIE64_FPU_JIT(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available on this platform")
	}
	instrs, totalInstrs := buildFPUProgram(benchIterations)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	resetState := func() {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.FPU.FPRegs = [16]uint32{}
		cpu.running.Store(true)
	}
	setupJITBench(b, cpu, instrs, resetState)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetState()
		cpu.jitExecute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// Memory Benchmarks
// ===========================================================================

// BenchmarkIE64_Memory_Interpreter measures LOAD/STORE throughput through the
// interpreter. Addresses are below IO_REGION_START, taking the direct-memory
// fast path. Each iteration writes a value, reads it back, and accumulates.
func BenchmarkIE64_Memory_Interpreter(b *testing.B) {
	instrs, totalInstrs := buildMemoryProgram(benchIterations)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	loadBenchProgram(cpu, instrs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.regs[1] = benchDataAddr
		cpu.regs[2] = 0
		cpu.running.Store(true)
		cpu.Execute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// BenchmarkIE64_Memory_JIT measures LOAD/STORE throughput compiled to native
// code. The JIT emits a range check (CMP addr, IO_REGION_START) before each
// memory access. For addresses below IO_REGION_START, the fast path uses
// direct base+index addressing (MOV via [RSI+RAX] on x86-64, LDR/STR via
// [X9, Xn] on ARM64). The bitmap lookup is only reached for addresses in
// the slow-path I/O region.
func BenchmarkIE64_Memory_JIT(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available on this platform")
	}
	instrs, totalInstrs := buildMemoryProgram(benchIterations)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	resetState := func() {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.regs[1] = benchDataAddr
		cpu.regs[2] = 0
		cpu.running.Store(true)
	}
	setupJITBench(b, cpu, instrs, resetState)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetState()
		cpu.jitExecute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// Mixed Benchmarks
// ===========================================================================

// BenchmarkIE64_Mixed_Interpreter measures a mixed workload (ALU + FP + memory)
// through the interpreter. This is representative of real IE64 programs that
// interleave computation types — e.g., a mandelbrot renderer doing FP math,
// integer iteration counting, and framebuffer writes in the same loop.
func BenchmarkIE64_Mixed_Interpreter(b *testing.B) {
	instrs, totalInstrs := buildMixedProgram(benchIterations)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	loadBenchProgram(cpu, instrs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.regs[1] = benchDataAddr
		cpu.regs[2] = 0
		cpu.FPU.FPRegs = [16]uint32{}
		cpu.running.Store(true)
		cpu.Execute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// BenchmarkIE64_Mixed_JIT measures the same mixed workload through the JIT.
// This exercises all major JIT code paths in a single block: register ALU,
// SSE FP, and dual-path memory access with range check.
func BenchmarkIE64_Mixed_JIT(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available on this platform")
	}
	instrs, totalInstrs := buildMixedProgram(benchIterations)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	resetState := func() {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.regs[1] = benchDataAddr
		cpu.regs[2] = 0
		cpu.FPU.FPRegs = [16]uint32{}
		cpu.running.Store(true)
	}
	setupJITBench(b, cpu, instrs, resetState)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetState()
		cpu.jitExecute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// ===========================================================================
// Call/Return Benchmarks
// ===========================================================================

// BenchmarkIE64_Call_Interpreter measures subroutine call/return overhead
// through the interpreter. Each iteration performs JSR (push return address,
// jump) + ADD (subroutine body) + RTS (pop return address, jump back).
// This tests the interpreter's stack manipulation and PC update paths.
func BenchmarkIE64_Call_Interpreter(b *testing.B) {
	instrs, totalInstrs := buildCallProgram(benchIterations)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	loadBenchProgram(cpu, instrs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.regs[31] = STACK_START
		cpu.regs[1] = 0
		cpu.running.Store(true)
		cpu.Execute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}

// BenchmarkIE64_Call_JIT measures subroutine call/return overhead through the
// JIT. Unlike ALU/FPU/memory benchmarks where the entire loop compiles into a
// single native block, JSR and RTS are block terminators — each one exits the
// JIT block and returns to the Go dispatcher. This means every call/return
// pair pays: native epilogue → dispatcher unpack → cache lookup → native
// prologue. This benchmark isolates that overhead, which is the dominant cost
// in programs with many small subroutines.
//
// Expected: lower JIT speedup than ALU/FPU (2-4x vs 5-10x) due to the
// per-block dispatch cost on every call/return.
func BenchmarkIE64_Call_JIT(b *testing.B) {
	if !jitAvailable {
		b.Skip("JIT not available on this platform")
	}
	instrs, totalInstrs := buildCallProgram(benchIterations)
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	resetState := func() {
		cpu.PC = PROG_START
		cpu.regs[10] = benchIterations
		cpu.regs[31] = STACK_START
		cpu.regs[1] = 0
		cpu.running.Store(true)
	}
	setupJITBench(b, cpu, instrs, resetState)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetState()
		cpu.jitExecute()
	}
	b.ReportMetric(float64(totalInstrs), "instructions/op")
}
