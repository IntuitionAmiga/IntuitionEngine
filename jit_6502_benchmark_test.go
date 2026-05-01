// jit_6502_benchmark_test.go - 6502 JIT vs interpreter benchmark suite
//
// This file benchmarks the 6502 CPU core through both the Go interpreter and
// the JIT compiler (x86-64/ARM64), reporting ns/op and instructions/op for
// five workload categories. MIPS can be derived: MIPS = instructions/op / ns/op * 1000.
//
// Workload categories:
//
//   - ALU:    Register ALU operations (ADC, AND, ORA, EOR, ASL, LSR) in a counted loop
//   - Memory: Zero page load/store in a counted loop
//   - Call:   JSR/RTS subroutine calls measuring block-exit overhead
//   - Branch: BEQ/BNE with mixed taken/not-taken patterns
//   - Mixed:  Interleaved ALU, memory, stack, and branches
//
// Each workload has an _Interpreter and a _JIT variant. JIT benchmarks skip
// automatically on platforms without JIT support.
//
// Usage:
//
//	# Run all 6502 JIT benchmarks
//	go test -tags headless -run='^$' -bench 'Benchmark6502_' -benchtime 3s ./...
//
//	# Compare JIT vs interpreter side-by-side
//	go test -tags headless -run='^$' -bench 'Benchmark6502_(ALU|Memory|Call|Branch|Mixed)' -benchtime 3s ./...

//go:build (amd64 || arm64) && linux

package main

import (
	"math"
	"runtime"
	"runtime/debug"
	"testing"
)

// JAM ($02) halts the CPU (running.Store(false)).
const benchHalt = 0x02

// ===========================================================================
// Benchmark Helpers
// ===========================================================================

// bench6502GCQuiesce minimises GC interference with a 6502 benchmark run.
// Call this as the first statement in every Benchmark6502_* function so that
// the measured loop is not distorted by the Go runtime reclaiming
// setup-time allocations or triggering a mid-benchmark sweep.
//
// The tuning mirrors IntuitionSubtractor's real-time audio strategy:
//
//  1. Raise GOGC to 2000% — the heap must grow 20x before the GC triggers
//     an automatic collection. Mirrors main.go in IntuitionSubtractor
//     (debug.SetGCPercent(2000)).
//  2. Raise the soft memory limit to math.MaxInt64 so the GC will not fire
//     on the memory ceiling. Also matches IntuitionSubtractor's
//     debug.SetMemoryLimit invocation (there a 3.5 GiB cap; here we drop
//     the cap entirely because the 6502 benchmarks allocate essentially
//     nothing in their hot loop).
//  3. Sweep existing garbage with two back-to-back runtime.GC() calls so
//     the benchmark starts from a clean heap. The double-GC pattern
//     matches IntuitionSubtractor's dsp_memory_layout_scalar.go "Double GC
//     for thorough cleanup".
//  4. Register a cleanup that restores the previous knobs and does a final
//     sweep, so benchmarks that run back-to-back don't accumulate
//     distortion across the suite.
//
// Each benchmark drives the CPU via a `for b.Loop() { ... }` loop. The
// Loop method (Go 1.24+, inlining fix landed in Go 1.26) auto-starts the
// benchmark timer on first call, so no explicit b.ResetTimer() is needed
// — the quiesce and all other setup above the loop is excluded from the
// measured window automatically.
func bench6502GCQuiesce(b *testing.B) {
	b.Helper()

	// Defer every automatic collection until the heap has grown 20x.
	oldGCPercent := debug.SetGCPercent(2000)
	// Disable the soft memory ceiling for the duration of the benchmark.
	oldMemLimit := debug.SetMemoryLimit(math.MaxInt64)

	// Drain setup-time garbage before the measured window starts. Two
	// passes flush objects that only become unreachable after the first
	// sweep's finalizers run.
	runtime.GC()
	runtime.GC()

	b.Cleanup(func() {
		debug.SetGCPercent(oldGCPercent)
		debug.SetMemoryLimit(oldMemLimit)
		// Final sweep so accumulated benchmark-time garbage doesn't bleed
		// into the next benchmark's measured window.
		runtime.GC()
	})
}

func setup6502BenchInterp(program []byte, startPC uint16) (*CPU_6502, *MachineBus) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)
	for i, b := range program {
		bus.Write8(uint32(startPC)+uint32(i), b)
	}
	return cpu, bus
}

func setup6502BenchJIT(b *testing.B, program []byte, startPC uint16, resetState func(*CPU_6502)) *CPU_6502 {
	b.Helper()
	// The 6502 JIT trampoline dispatches through runtime.asmcgocall (see
	// jit_call.go). asmcgocall is the raw stack-switch primitive and works
	// in both CGO_ENABLED=1 and CGO_ENABLED=0 builds, so the only remaining
	// gating condition is the platform build tag (amd64/arm64 + linux) via
	// jit6502Available.
	if !jit6502Available {
		b.Skip("6502 JIT is not available on this platform")
	}
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)
	for i, byt := range program {
		bus.Write8(uint32(startPC)+uint32(i), byt)
	}

	cpu.jitEnabled = true
	cpu.jitPersist = true

	// Warm-up: compile JIT blocks
	resetState(cpu)
	cpu.ExecuteJIT6502()

	b.Cleanup(func() {
		cpu.jitPersist = false
		cpu.freeJIT6502()
	})

	return cpu
}

// ===========================================================================
// ALU Benchmark
// ===========================================================================
//
// Tight loop of register ALU operations. 256 iterations (counted by X).
//
//   $0600: LDA #$07       ; 2 bytes - seed A
//   $0602: LDX #$00       ; 2 bytes - counter (256 iterations via wrap)
//   loop:
//   $0604: ADC #$03       ; 2 bytes
//   $0606: AND #$7F       ; 2 bytes
//   $0608: ORA #$01       ; 2 bytes
//   $060A: EOR #$55       ; 2 bytes
//   $060C: ASL A          ; 1 byte
//   $060D: LSR A          ; 1 byte
//   $060E: ROL A          ; 1 byte
//   $060F: DEX            ; 1 byte
//   $0610: BNE loop       ; 2 bytes (offset = $0604 - $0612 = -14 = $F2)
//   $0612: JAM            ; 1 byte

var bench6502ALUProgram = []byte{
	0xA9, 0x07, // LDA #$07
	0xA2, 0x00, // LDX #$00 (256 iterations)
	0x69, 0x03, // ADC #$03
	0x29, 0x7F, // AND #$7F
	0x09, 0x01, // ORA #$01
	0x49, 0x55, // EOR #$55
	0x0A,       // ASL A
	0x4A,       // LSR A
	0x2A,       // ROL A
	0xCA,       // DEX
	0xD0, 0xF2, // BNE -14 (back to ADC)
	benchHalt,
}

// 2 setup + 256 * 9 (loop body) = 2306 instructions per run
const bench6502ALUInstrs = 2 + 256*9

func Benchmark6502_ALU_Interpreter(b *testing.B) {
	bench6502GCQuiesce(b)
	cpu, _ := setup6502BenchInterp(bench6502ALUProgram, 0x0600)

	for b.Loop() {
		cpu.PC = 0x0600
		cpu.A = 0
		cpu.X = 0
		cpu.SR = 0
		cpu.Cycles = 0
		cpu.SetRunning(true)
		cpu.Execute()
	}
	b.ReportMetric(float64(bench6502ALUInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, bench6502ALUInstrs)
}

func Benchmark6502_ALU_JIT(b *testing.B) {
	if !jit6502Available {
		b.Skip("6502 JIT not available on this platform")
	}
	bench6502GCQuiesce(b)
	resetState := func(cpu *CPU_6502) {
		cpu.PC = 0x0600
		cpu.A = 0
		cpu.X = 0
		cpu.SR = 0
		cpu.Cycles = 0
		cpu.SetRunning(true)
	}
	cpu := setup6502BenchJIT(b, bench6502ALUProgram, 0x0600, resetState)

	for b.Loop() {
		resetState(cpu)
		cpu.ExecuteJIT6502()
	}
	b.ReportMetric(float64(bench6502ALUInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, bench6502ALUInstrs)
}

// ===========================================================================
// Memory Benchmark
// ===========================================================================
//
// Zero page load/store loop. 256 iterations.
//
//   $0600: LDX #$00       ; counter (256 via wrap)
//   loop:
//   $0602: LDA $10,X      ; load from ZP
//   $0604: STA $80,X      ; store to ZP
//   $0606: INX            ; next
//   $0607: BNE loop       ; loop
//   $0609: JAM

// Layout:
//
//	+0: A2 00  LDX #$00
//	+2: B5 10  LDA $10,X     ← loop start (target)
//	+4: 95 80  STA $80,X
//	+6: E8     INX
//	+7: D0 F9  BNE (PC+2=$0609, target=$0602, offset=$0602-$0609=-7=0xF9)
//	+9: 02     JAM
var bench6502MemProgram = []byte{
	0xA2, 0x00, // LDX #$00
	0xB5, 0x10, // LDA $10,X
	0x95, 0x80, // STA $80,X
	0xE8,       // INX
	0xD0, 0xF9, // BNE -7 (back to LDA $10,X)
	benchHalt,
}

const bench6502MemInstrs = 1 + 256*4

func Benchmark6502_Memory_Interpreter(b *testing.B) {
	bench6502GCQuiesce(b)
	cpu, _ := setup6502BenchInterp(bench6502MemProgram, 0x0600)

	for b.Loop() {
		cpu.PC = 0x0600
		cpu.X = 0
		cpu.Cycles = 0
		cpu.SetRunning(true)
		cpu.Execute()
	}
	b.ReportMetric(float64(bench6502MemInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, bench6502MemInstrs)
}

func Benchmark6502_Memory_JIT(b *testing.B) {
	if !jit6502Available {
		b.Skip("6502 JIT not available on this platform")
	}
	bench6502GCQuiesce(b)
	resetState := func(cpu *CPU_6502) {
		cpu.PC = 0x0600
		cpu.X = 0
		cpu.Cycles = 0
		cpu.SetRunning(true)
	}
	cpu := setup6502BenchJIT(b, bench6502MemProgram, 0x0600, resetState)

	for b.Loop() {
		resetState(cpu)
		cpu.ExecuteJIT6502()
	}
	b.ReportMetric(float64(bench6502MemInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, bench6502MemInstrs)
}

// ===========================================================================
// Call Benchmark
// ===========================================================================
//
// JSR/RTS loop measuring block-exit overhead.
//
//   $0600: LDX #$00       ; counter (256 via wrap)
//   loop:
//   $0602: JSR $0610      ; call subroutine
//   $0605: DEX            ; decrement
//   $0606: BNE loop       ; loop
//   $0608: JAM
//
//   subroutine:
//   $0610: INY            ; do something
//   $0611: RTS

var bench6502CallProgram []byte

func init() {
	main := []byte{
		0xA2, 0x00, // LDX #$00
		0x20, 0x10, 0x06, // JSR $0610
		0xCA,       // DEX
		0xD0, 0xFA, // BNE -6 (back to JSR)
		benchHalt, // JAM
	}
	// Pad to $0610
	prog := make([]byte, 0x12)
	copy(prog, main)
	// Subroutine at offset $10 (= $0610 - $0600)
	prog[0x10] = 0xC8 // INY
	prog[0x11] = 0x60 // RTS
	bench6502CallProgram = prog
}

// 1 setup + 256 * 5 (JSR + INY + RTS + DEX + BNE) = 1281
const bench6502CallInstrs = 1 + 256*5

func Benchmark6502_Call_Interpreter(b *testing.B) {
	bench6502GCQuiesce(b)
	cpu, _ := setup6502BenchInterp(bench6502CallProgram, 0x0600)

	for b.Loop() {
		cpu.PC = 0x0600
		cpu.X = 0
		cpu.Y = 0
		cpu.SP = 0xFF
		cpu.Cycles = 0
		cpu.SetRunning(true)
		cpu.Execute()
	}
	b.ReportMetric(float64(bench6502CallInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, bench6502CallInstrs)
}

func Benchmark6502_Call_JIT(b *testing.B) {
	if !jit6502Available {
		b.Skip("6502 JIT not available on this platform")
	}
	bench6502GCQuiesce(b)
	resetState := func(cpu *CPU_6502) {
		cpu.PC = 0x0600
		cpu.X = 0
		cpu.Y = 0
		cpu.SP = 0xFF
		cpu.Cycles = 0
		cpu.SetRunning(true)
	}
	cpu := setup6502BenchJIT(b, bench6502CallProgram, 0x0600, resetState)

	for b.Loop() {
		resetState(cpu)
		cpu.ExecuteJIT6502()
	}
	b.ReportMetric(float64(bench6502CallInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, bench6502CallInstrs)
}

// ===========================================================================
// Branch Benchmark
// ===========================================================================
//
// Alternating taken/not-taken branches to stress branch compilation.
//
//   $0600: LDX #$00       ; counter
//   loop:
//   $0602: INX            ; X++
//   $0603: TXA            ; A = X
//   $0604: AND #$01       ; A = X & 1 (alternates 0/1)
//   $0606: BEQ skip       ; skip next if even
//   $0608: NOP            ; odd path
//   skip:
//   $0609: DEY            ; Y-- (always)
//   $060A: BNE loop       ; loop back (Y wraps from 0 → FF, counts 256)
//   $060C: JAM

// Layout:
//
//	+0:  A2 00  LDX #$00
//	+2:  E8     INX           ← loop start (target)
//	+3:  8A     TXA
//	+4:  29 01  AND #$01
//	+6:  F0 01  BEQ +1 (skip NOP, target = +8 + 1 = +9 = DEY)
//	+8:  EA     NOP (odd path)
//	+9:  88     DEY
//	+A:  D0 F6  BNE (PC+2=$060C, target=$0602, offset=$0602-$060C=-10=0xF6)
//	+C:  02     JAM
var bench6502BranchProgram = []byte{
	0xA2, 0x00, // LDX #$00
	0xE8,       // INX
	0x8A,       // TXA
	0x29, 0x01, // AND #$01
	0xF0, 0x01, // BEQ +1 (skip NOP)
	0xEA,       // NOP
	0x88,       // DEY
	0xD0, 0xF6, // BNE -10 (back to INX)
	benchHalt,
}

// 1 setup + 256 iterations * ~6 avg instructions = ~1537
// Exact: 1 (LDX) + 256*(INX + TXA + AND + BEQ + [NOP half the time] + DEY + BNE)
// = 1 + 256*(5 + 0.5*1) = 1 + 256*5.5 ≈ 1409. Let's just report 256*6.
const bench6502BranchInstrs = 1 + 256*6

func Benchmark6502_Branch_Interpreter(b *testing.B) {
	bench6502GCQuiesce(b)
	cpu, _ := setup6502BenchInterp(bench6502BranchProgram, 0x0600)

	for b.Loop() {
		cpu.PC = 0x0600
		cpu.X = 0
		cpu.Y = 0
		cpu.SR = 0
		cpu.Cycles = 0
		cpu.SetRunning(true)
		cpu.Execute()
	}
	b.ReportMetric(float64(bench6502BranchInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, bench6502BranchInstrs)
}

func Benchmark6502_Branch_JIT(b *testing.B) {
	if !jit6502Available {
		b.Skip("6502 JIT not available on this platform")
	}
	bench6502GCQuiesce(b)
	resetState := func(cpu *CPU_6502) {
		cpu.PC = 0x0600
		cpu.X = 0
		cpu.Y = 0
		cpu.SR = 0
		cpu.Cycles = 0
		cpu.SetRunning(true)
	}
	cpu := setup6502BenchJIT(b, bench6502BranchProgram, 0x0600, resetState)

	for b.Loop() {
		resetState(cpu)
		cpu.ExecuteJIT6502()
	}
	b.ReportMetric(float64(bench6502BranchInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, bench6502BranchInstrs)
}

// ===========================================================================
// Mixed Benchmark
// ===========================================================================
//
// Interleaved ALU + memory + stack + branches.
//
//   $0600: LDX #$00       ; counter (256 iterations)
//   $0602: LDA #$00       ; accumulator seed
//   loop:
//   $0604: ADC $10,X      ; load from ZP + add
//   $0606: PHA            ; push to stack
//   $0607: EOR #$AA       ; XOR
//   $0609: STA $80,X      ; store to ZP
//   $060B: PLA            ; pull from stack
//   $060C: INX            ; next
//   $060D: BNE loop       ; loop
//   $060F: JAM

// Layout:
//
//	+0:  A2 00  LDX #$00
//	+2:  A9 00  LDA #$00
//	+4:  75 10  ADC $10,X    ← loop start (target)
//	+6:  48     PHA
//	+7:  49 AA  EOR #$AA
//	+9:  95 80  STA $80,X
//	+B:  68     PLA
//	+C:  E8     INX
//	+D:  D0 F5  BNE (PC+2=$060F, target=$0604, offset=$0604-$060F=-11=0xF5)
//	+F:  02     JAM
var bench6502MixedProgram = []byte{
	0xA2, 0x00, // LDX #$00
	0xA9, 0x00, // LDA #$00
	0x75, 0x10, // ADC $10,X
	0x48,       // PHA
	0x49, 0xAA, // EOR #$AA
	0x95, 0x80, // STA $80,X
	0x68,       // PLA
	0xE8,       // INX
	0xD0, 0xF5, // BNE -11 (back to ADC)
	benchHalt,
}

// 2 setup + 256 * 8 (loop body) = 2050
const bench6502MixedInstrs = 2 + 256*8

func Benchmark6502_Mixed_Interpreter(b *testing.B) {
	bench6502GCQuiesce(b)
	cpu, _ := setup6502BenchInterp(bench6502MixedProgram, 0x0600)

	for b.Loop() {
		cpu.PC = 0x0600
		cpu.A = 0
		cpu.X = 0
		cpu.SP = 0xFF
		cpu.SR = 0
		cpu.Cycles = 0
		cpu.SetRunning(true)
		cpu.Execute()
	}
	b.ReportMetric(float64(bench6502MixedInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, bench6502MixedInstrs)
}

func Benchmark6502_Mixed_JIT(b *testing.B) {
	if !jit6502Available {
		b.Skip("6502 JIT not available on this platform")
	}
	bench6502GCQuiesce(b)
	resetState := func(cpu *CPU_6502) {
		cpu.PC = 0x0600
		cpu.A = 0
		cpu.X = 0
		cpu.SP = 0xFF
		cpu.SR = 0
		cpu.Cycles = 0
		cpu.SetRunning(true)
	}
	cpu := setup6502BenchJIT(b, bench6502MixedProgram, 0x0600, resetState)

	for b.Loop() {
		resetState(cpu)
		cpu.ExecuteJIT6502()
	}
	b.ReportMetric(float64(bench6502MixedInstrs), "instructions/op")
	ReportMIPSHostNormalized(b, bench6502MixedInstrs)
}
