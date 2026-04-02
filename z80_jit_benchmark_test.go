// z80_jit_benchmark_test.go - Z80 JIT vs interpreter benchmark suite
//
// Benchmarks the Z80 CPU core through both the Go interpreter and the JIT
// compiler, reporting ns/op and instrs/op. MIPS = instrs/op / ns/op * 1000.
//
// Workloads:
//   - ALU:    Register ALU operations in a DJNZ loop (256 iterations)
//   - Memory: Load/store loop via LD A,(HL); LD (DE),A; INC HL; INC DE
//   - Mixed:  Interleaved ALU + memory + stack (PUSH/POP, LD (HL), ADD)
//   - Call:   CALL/RET subroutine overhead (256 iterations)
//
// Reference results (i5-8365U, benchtime=30s, Phases A-D optimizations):
//   ALU:    Interp 43.8us (53 MIPS), JIT  5.3us (433 MIPS) → 8.2x
//   Memory: Interp 24.2us (64 MIPS), JIT  4.6us (333 MIPS) → 5.3x
//   Mixed:  Interp 49.5us (41 MIPS), JIT  9.0us (228 MIPS) → 5.5x
//   Call:   Interp 28.2us (36 MIPS), JIT 11.0us  (93 MIPS) → 2.6x
//
// Cross-architecture comparison (same machine):
//   6502 JIT: ALU 1405 MIPS, Memory 1106 MIPS, Mixed 1389 MIPS, Call 362 MIPS
//   M68K JIT: ALU 1612 MIPS, MemCopy 335 MIPS, LazyCCR 1610 MIPS
//   Z80 is ~3x behind 6502 on ALU due to packed register extraction overhead.
//   Z80 Memory (333) and Call (93) are comparable to M68K (335/95).
//
// Usage:
//   go test -tags headless -run='^$' -bench 'BenchmarkZ80_' -benchtime 30s ./...

//go:build (amd64 || arm64) && linux

package main

import (
	"testing"
)

// ===========================================================================
// Benchmark Helpers
// ===========================================================================

func setupZ80BenchInterp(program []byte, startPC uint16) (*CPU_Z80, *MachineBus, *Z80BusAdapter) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.SP = 0x1FFE
	for i, b := range program {
		bus.Write8(uint32(startPC)+uint32(i), b)
	}
	return cpu, bus, adapter
}

func setupZ80BenchJIT(b *testing.B, program []byte, startPC uint16, resetState func(*CPU_Z80)) *CPU_Z80 {
	b.Helper()
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.SP = 0x1FFE
	for i, byt := range program {
		bus.Write8(uint32(startPC)+uint32(i), byt)
	}

	cpu.jitEnabled = true
	cpu.jitPersist = true

	// Warm-up: compile JIT blocks by running once.
	// ExecuteJITZ80 now returns on HALT.
	resetState(cpu)
	cpu.ExecuteJITZ80()

	b.Cleanup(func() {
		cpu.jitPersist = false
		cpu.freeZ80JIT()
	})

	return cpu
}

// ===========================================================================
// ALU Benchmark — 256 iterations of register ALU operations
// ===========================================================================
//
// LD B, 0          ; 256 iterations (0 wraps to 256)
// loop:
//   ADD A, B        ; ALU
//   XOR A, C        ; ALU
//   AND A, D        ; ALU
//   OR  A, E        ; ALU
//   SUB A, H        ; ALU
//   INC A           ; ALU
//   DEC A           ; ALU
//   DJNZ loop       ; decrement B, loop if not zero
//   HALT

func buildZ80ALUProgram() ([]byte, uint16) {
	return []byte{
		0x06, 0x00, // LD B, 0 (256 iterations)
		0x80,       // ADD A, B
		0xA9,       // XOR A, C
		0xA2,       // AND A, D
		0xB3,       // OR A, E
		0x94,       // SUB A, H
		0x3C,       // INC A
		0x3D,       // DEC A
		0x10, 0xF6, // DJNZ -10 (back to ADD A,B)
		0x76, // HALT
	}, 0x0100
}

func BenchmarkZ80_ALU_Interpreter(b *testing.B) {
	prog, startPC := buildZ80ALUProgram()
	cpu, _, _ := setupZ80BenchInterp(prog, startPC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = startPC
		cpu.A = 0
		cpu.B = 0
		cpu.Halted = false
		cpu.SetRunning(true)
		// Run until halted
		for cpu.running.Load() && !cpu.Halted {
			cpu.Step()
		}
		cpu.SetRunning(false)
	}
	b.ReportMetric(float64(2307), "instrs/op")
}

func BenchmarkZ80_ALU_JIT(b *testing.B) {
	if !z80JitAvailable {
		b.Skip("Z80 JIT not available on this platform")
	}
	prog, startPC := buildZ80ALUProgram()
	cpu := setupZ80BenchJIT(b, prog, startPC, func(c *CPU_Z80) {
		c.PC = startPC
		c.A = 0
		c.B = 0
		c.Halted = false
		c.SetRunning(true)
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = startPC
		cpu.A = 0
		cpu.B = 0
		cpu.Halted = false
		cpu.SetRunning(true)
		cpu.ExecuteJITZ80() // returns on HALT
	}
	b.ReportMetric(float64(2307), "instrs/op")
}

// ===========================================================================
// Memory Benchmark — 256-byte memory copy loop
// ===========================================================================
//
// LD HL, $0500     ; source
// LD DE, $0600     ; dest
// LD B, 0          ; 256 iterations
// loop:
//   LD A, (HL)     ; load from memory
//   LD (DE), A     ; store to memory (uses interpreter fallback for now)
//   INC HL
//   INC DE
//   DJNZ loop
//   HALT

func buildZ80MemoryProgram() ([]byte, uint16) {
	return []byte{
		0x21, 0x00, 0x05, // LD HL, $0500
		0x11, 0x00, 0x06, // LD DE, $0600
		0x06, 0x00, // LD B, 0
		0x7E,       // LD A, (HL)
		0x12,       // LD (DE), A
		0x23,       // INC HL
		0x13,       // INC DE
		0x10, 0xFA, // DJNZ -6
		0x76, // HALT
	}, 0x0100
}

func BenchmarkZ80_Memory_Interpreter(b *testing.B) {
	prog, startPC := buildZ80MemoryProgram()
	cpu, _, _ := setupZ80BenchInterp(prog, startPC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = startPC
		cpu.Halted = false
		cpu.SetRunning(true)
		for cpu.running.Load() && !cpu.Halted {
			cpu.Step()
		}
		cpu.SetRunning(false)
	}
	b.ReportMetric(float64(1539), "instrs/op")
}

func BenchmarkZ80_Memory_JIT(b *testing.B) {
	if !z80JitAvailable {
		b.Skip("Z80 JIT not available on this platform")
	}
	prog, startPC := buildZ80MemoryProgram()
	cpu := setupZ80BenchJIT(b, prog, startPC, func(c *CPU_Z80) {
		c.PC = startPC
		c.Halted = false
		c.SetRunning(true)
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = startPC
		cpu.Halted = false
		cpu.SetRunning(true)
		cpu.ExecuteJITZ80()
	}
	b.ReportMetric(float64(1539), "instrs/op")
}

// ===========================================================================
// Mixed Benchmark — ALU + memory + stack
// ===========================================================================

func buildZ80MixedProgram() ([]byte, uint16) {
	return []byte{
		0x06, 0x00, // LD B, 0 (256 iterations)
		0x21, 0x00, 0x05, // LD HL, $0500
		0x7E,       // LD A, (HL)
		0x80,       // ADD A, B
		0x77,       // LD (HL), A
		0x23,       // INC HL
		0xC5,       // PUSH BC
		0xC1,       // POP BC
		0x10, 0xF8, // DJNZ -8
		0x76, // HALT
	}, 0x0100
}

func BenchmarkZ80_Mixed_Interpreter(b *testing.B) {
	prog, startPC := buildZ80MixedProgram()
	cpu, _, _ := setupZ80BenchInterp(prog, startPC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = startPC
		cpu.SP = 0x1FFE
		cpu.Halted = false
		cpu.SetRunning(true)
		for cpu.running.Load() && !cpu.Halted {
			cpu.Step()
		}
		cpu.SetRunning(false)
	}
	b.ReportMetric(float64(2051), "instrs/op")
}

func BenchmarkZ80_Mixed_JIT(b *testing.B) {
	if !z80JitAvailable {
		b.Skip("Z80 JIT not available on this platform")
	}
	prog, startPC := buildZ80MixedProgram()
	cpu := setupZ80BenchJIT(b, prog, startPC, func(c *CPU_Z80) {
		c.PC = startPC
		c.SP = 0x1FFE
		c.Halted = false
		c.SetRunning(true)
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = startPC
		cpu.SP = 0x1FFE
		cpu.Halted = false
		cpu.SetRunning(true)
		cpu.ExecuteJITZ80()
	}
	b.ReportMetric(float64(2051), "instrs/op")
}

// ===========================================================================
// Call Benchmark — subroutine call overhead
// ===========================================================================

func buildZ80CallProgram() ([]byte, uint16) {
	// Main at $0100: LD B,0; loop: CALL $0200; DJNZ loop; HALT
	// Sub at $0200: INC A; RET
	prog := make([]byte, 0x200)
	// Main
	prog[0x00] = 0x06 // LD B, 0
	prog[0x01] = 0x00
	prog[0x02] = 0xCD // CALL $0200
	prog[0x03] = 0x00
	prog[0x04] = 0x02
	prog[0x05] = 0x10 // DJNZ -5
	prog[0x06] = 0xFB
	prog[0x07] = 0x76 // HALT
	// Sub at offset $100 (addr $0200)
	prog[0x100] = 0x3C // INC A
	prog[0x101] = 0xC9 // RET
	return prog, 0x0100
}

func BenchmarkZ80_Call_Interpreter(b *testing.B) {
	prog, startPC := buildZ80CallProgram()
	cpu, _, _ := setupZ80BenchInterp(prog, startPC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = startPC
		cpu.SP = 0x1FFE
		cpu.A = 0
		cpu.Halted = false
		cpu.SetRunning(true)
		for cpu.running.Load() && !cpu.Halted {
			cpu.Step()
		}
		cpu.SetRunning(false)
	}
	b.ReportMetric(float64(1027), "instrs/op")
}

func BenchmarkZ80_Call_JIT(b *testing.B) {
	if !z80JitAvailable {
		b.Skip("Z80 JIT not available on this platform")
	}
	prog, startPC := buildZ80CallProgram()
	cpu := setupZ80BenchJIT(b, prog, startPC, func(c *CPU_Z80) {
		c.PC = startPC
		c.SP = 0x1FFE
		c.A = 0
		c.Halted = false
		c.SetRunning(true)
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = startPC
		cpu.SP = 0x1FFE
		cpu.A = 0
		cpu.Halted = false
		cpu.SetRunning(true)
		cpu.ExecuteJITZ80()
	}
	b.ReportMetric(float64(1027), "instrs/op")
}
