package main

import (
	"sync/atomic"
	"testing"
)

// =============================================================================
// CPU Benchmark Suite
// Measures instruction execution performance for 6502, Z80, M68K, and x86 CPUs
// Run with: go test -bench="Benchmark.*CPU" -benchmem -run="^$" ./...
// =============================================================================

// =============================================================================
// 6502 CPU Benchmarks
// =============================================================================

// setup6502BenchCPU creates a 6502 CPU for benchmarking
func setup6502BenchCPU() (*CPU_6502, *MachineBus) {
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)
	return cpu, bus
}

// Benchmark6502_LDA_Immediate measures LDA #imm throughput
func Benchmark6502_LDA_Immediate(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	// LDA #$42
	bus.Write8(0x1000, 0xA9)
	bus.Write8(0x1001, 0x42)
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
	}
}

// Benchmark6502_LDA_Absolute measures LDA addr throughput
func Benchmark6502_LDA_Absolute(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	// LDA $2000
	bus.Write8(0x1000, 0xAD)
	bus.Write8(0x1001, 0x00)
	bus.Write8(0x1002, 0x20)
	bus.Write8(0x2000, 0x42)
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
	}
}

// Benchmark6502_LDA_ZeroPage measures LDA zp throughput
func Benchmark6502_LDA_ZeroPage(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	// LDA $42
	bus.Write8(0x1000, 0xA5)
	bus.Write8(0x1001, 0x42)
	bus.Write8(0x0042, 0x55)
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
	}
}

// Benchmark6502_LDA_IndirectY measures LDA (zp),Y throughput
func Benchmark6502_LDA_IndirectY(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	cpu.Y = 0x01
	// LDA ($42),Y
	bus.Write8(0x1000, 0xB1)
	bus.Write8(0x1001, 0x42)
	// Pointer at $42 -> $2000
	bus.Write8(0x0042, 0x00)
	bus.Write8(0x0043, 0x20)
	// Value at $2001
	bus.Write8(0x2001, 0x55)
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
	}
}

func Benchmark6502_LDA_IndirectX(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	cpu.X = 0x04
	// LDA ($40,X)
	bus.Write8(0x1000, 0xA1)
	bus.Write8(0x1001, 0x40)
	// pointer at $44 -> $2000
	bus.Write8(0x0044, 0x00)
	bus.Write8(0x0045, 0x20)
	bus.Write8(0x2000, 0x66)
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
	}
}

// Benchmark6502_Memory_Read_Direct measures isolated adapter.Read() call
func Benchmark6502_Memory_Read_Direct(b *testing.B) {
	_, bus := setup6502BenchCPU()
	adapter := NewBus6502Adapter(bus)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adapter.Read(0x1000)
	}
}

func Benchmark6502_Execute_Mixed(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	// LDA #$01; STA $10; INX; JMP $1000
	bus.Write8(0x1000, 0xA9)
	bus.Write8(0x1001, 0x01)
	bus.Write8(0x1002, 0x85)
	bus.Write8(0x1003, 0x10)
	bus.Write8(0x1004, 0xE8)
	bus.Write8(0x1005, 0x4C)
	bus.Write8(0x1006, 0x00)
	bus.Write8(0x1007, 0x10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.X = 0
		cpu.Step()
		cpu.Step()
		cpu.Step()
		cpu.Step()
	}
}

func Benchmark6502_Execute_Tight_Loop(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	// NOP; JMP $1000
	bus.Write8(0x1000, 0xEA)
	bus.Write8(0x1001, 0x4C)
	bus.Write8(0x1002, 0x00)
	bus.Write8(0x1003, 0x10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
		cpu.Step()
	}
}

func Benchmark6502_UpdateNZ(b *testing.B) {
	cpu, _ := setup6502BenchCPU()
	var v byte

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v += byte(i)
		cpu.updateNZ(v)
	}
}

func Benchmark6502_Read_IO_PSG(b *testing.B) {
	_, bus := setup6502BenchCPU()
	adapter := NewBus6502Adapter(bus)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = adapter.Read(0xD400)
	}
}

func Benchmark6502_Write_IO_SID(b *testing.B) {
	_, bus := setup6502BenchCPU()
	adapter := NewBus6502Adapter(bus)
	var v byte

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v += byte(i)
		adapter.Write(0xD500, v)
	}
}

func Benchmark6502_Execute_WithContention(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	// NOP; JMP $1000
	bus.Write8(0x1000, 0xEA)
	bus.Write8(0x1001, 0x4C)
	bus.Write8(0x1002, 0x00)
	bus.Write8(0x1003, 0x10)

	stop := atomic.Bool{}
	go func() {
		for !stop.Load() {
			cpu.irqPending.Store(true)
			cpu.irqPending.Store(false)
		}
	}()
	defer stop.Store(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
		cpu.Step()
	}
}

// Benchmark6502_STA_Absolute measures STA addr throughput
func Benchmark6502_STA_Absolute(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	cpu.A = 0x42
	// STA $2000
	bus.Write8(0x1000, 0x8D)
	bus.Write8(0x1001, 0x00)
	bus.Write8(0x1002, 0x20)
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
	}
}

// Benchmark6502_ADC_Immediate measures ADC #imm throughput
func Benchmark6502_ADC_Immediate(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	cpu.A = 0x10
	// ADC #$20
	bus.Write8(0x1000, 0x69)
	bus.Write8(0x1001, 0x20)
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.A = 0x10
		cpu.Step()
	}
}

// Benchmark6502_Step_Dispatch measures dispatch overhead with NOP
func Benchmark6502_Step_Dispatch(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	// NOP
	bus.Write8(0x1000, 0xEA)
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
	}
}

// Benchmark6502_Branch_Taken measures branch (taken) throughput
func Benchmark6502_Branch_Taken(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	cpu.SR = ZERO_FLAG // Z=1 for BEQ to branch
	// BEQ +0 (branch to next instruction)
	bus.Write8(0x1000, 0xF0)
	bus.Write8(0x1001, 0x00)
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.SR = ZERO_FLAG
		cpu.Step()
	}
}

// Benchmark6502_JSR_RTS measures subroutine call overhead
func Benchmark6502_JSR_RTS(b *testing.B) {
	cpu, bus := setup6502BenchCPU()
	cpu.SP = 0xFF
	// JSR $2000
	bus.Write8(0x1000, 0x20)
	bus.Write8(0x1001, 0x00)
	bus.Write8(0x1002, 0x20)
	// RTS at $2000
	bus.Write8(0x2000, 0x60)
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.SP = 0xFF
		cpu.Step() // JSR
		cpu.Step() // RTS
	}
}

// =============================================================================
// Z80 CPU Benchmarks
// =============================================================================

// z80BenchBus implements Z80Bus for benchmarking
type z80BenchBus struct {
	mem [0x10000]byte
}

func (b *z80BenchBus) Read(addr uint16) byte       { return b.mem[addr] }
func (b *z80BenchBus) Write(addr uint16, val byte) { b.mem[addr] = val }
func (b *z80BenchBus) In(port uint16) byte         { return 0 }
func (b *z80BenchBus) Out(port uint16, val byte)   {}
func (b *z80BenchBus) Tick(cycles int)             {}

// setupZ80BenchCPU creates a Z80 CPU for benchmarking
func setupZ80BenchCPU() (*CPU_Z80, *z80BenchBus) {
	bus := &z80BenchBus{}
	cpu := NewCPU_Z80(bus)
	return cpu, bus
}

// BenchmarkZ80_LD_A_n measures LD A,n throughput
func BenchmarkZ80_LD_A_n(b *testing.B) {
	cpu, bus := setupZ80BenchCPU()
	// LD A,$42
	bus.mem[0x1000] = 0x3E
	bus.mem[0x1001] = 0x42
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
	}
}

// BenchmarkZ80_ADD_A_r measures ADD A,r throughput
func BenchmarkZ80_ADD_A_r(b *testing.B) {
	cpu, bus := setupZ80BenchCPU()
	cpu.A = 0x10
	cpu.B = 0x20
	// ADD A,B
	bus.mem[0x1000] = 0x80
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.A = 0x10
		cpu.Step()
	}
}

// BenchmarkZ80_LD_HL_nn measures LD HL,nn throughput
func BenchmarkZ80_LD_HL_nn(b *testing.B) {
	cpu, bus := setupZ80BenchCPU()
	// LD HL,$1234
	bus.mem[0x1000] = 0x21
	bus.mem[0x1001] = 0x34
	bus.mem[0x1002] = 0x12
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
	}
}

// BenchmarkZ80_INC_r measures INC r throughput
func BenchmarkZ80_INC_r(b *testing.B) {
	cpu, bus := setupZ80BenchCPU()
	cpu.B = 0
	// INC B
	bus.mem[0x1000] = 0x04
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.B = 0
		cpu.Step()
	}
}

// BenchmarkZ80_Step_Dispatch measures dispatch overhead with NOP
func BenchmarkZ80_Step_Dispatch(b *testing.B) {
	cpu, bus := setupZ80BenchCPU()
	// NOP
	bus.mem[0x1000] = 0x00
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
	}
}

// BenchmarkZ80_JP_nn measures JP nn throughput
func BenchmarkZ80_JP_nn(b *testing.B) {
	cpu, bus := setupZ80BenchCPU()
	// JP $1000 (jump to self for benchmark)
	bus.mem[0x1000] = 0xC3
	bus.mem[0x1001] = 0x00
	bus.mem[0x1002] = 0x10
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.Step()
	}
}

// BenchmarkZ80_CALL_RET measures subroutine call overhead
func BenchmarkZ80_CALL_RET(b *testing.B) {
	cpu, bus := setupZ80BenchCPU()
	cpu.SP = 0xFFFE
	// CALL $2000
	bus.mem[0x1000] = 0xCD
	bus.mem[0x1001] = 0x00
	bus.mem[0x1002] = 0x20
	// RET at $2000
	bus.mem[0x2000] = 0xC9
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.SP = 0xFFFE
		cpu.Step() // CALL
		cpu.Step() // RET
	}
}

// BenchmarkZ80_LDIR measures LDIR throughput (1 iteration)
func BenchmarkZ80_LDIR(b *testing.B) {
	cpu, bus := setupZ80BenchCPU()
	// Source data
	bus.mem[0x2000] = 0xAA
	// LDIR (ED B0)
	bus.mem[0x1000] = 0xED
	bus.mem[0x1001] = 0xB0
	cpu.PC = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.H = 0x20
		cpu.L = 0x00 // HL = $2000 (source)
		cpu.D = 0x30
		cpu.E = 0x00 // DE = $3000 (dest)
		cpu.B = 0x00
		cpu.C = 0x01 // BC = 1 (count)
		cpu.Step()
	}
}

// =============================================================================
// x86 CPU Benchmarks
// =============================================================================

// x86BenchBus implements X86Bus for benchmarking
type x86BenchBus struct {
	mem [0x100000]byte
}

func (b *x86BenchBus) Read(addr uint32) byte       { return b.mem[addr&0xFFFFF] }
func (b *x86BenchBus) Write(addr uint32, val byte) { b.mem[addr&0xFFFFF] = val }
func (b *x86BenchBus) In(port uint16) byte         { return 0 }
func (b *x86BenchBus) Out(port uint16, val byte)   {}
func (b *x86BenchBus) Tick(cycles int)             {}

// setupX86BenchCPU creates an x86 CPU for benchmarking
func setupX86BenchCPU() (*CPU_X86, *x86BenchBus) {
	bus := &x86BenchBus{}
	cpu := NewCPU_X86(bus)
	return cpu, bus
}

// BenchmarkX86_MOV_r32_imm measures MOV r32,imm32 throughput
func BenchmarkX86_MOV_r32_imm(b *testing.B) {
	cpu, bus := setupX86BenchCPU()
	// MOV EAX,$12345678 (B8 78 56 34 12)
	bus.mem[0x1000] = 0xB8
	bus.mem[0x1001] = 0x78
	bus.mem[0x1002] = 0x56
	bus.mem[0x1003] = 0x34
	bus.mem[0x1004] = 0x12
	cpu.EIP = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.Step()
	}
}

// BenchmarkX86_MOV_r32_r32 measures MOV r32,r32 throughput
func BenchmarkX86_MOV_r32_r32(b *testing.B) {
	cpu, bus := setupX86BenchCPU()
	cpu.EBX = 0x12345678
	// MOV EAX,EBX (89 D8)
	bus.mem[0x1000] = 0x89
	bus.mem[0x1001] = 0xD8
	cpu.EIP = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.Step()
	}
}

// BenchmarkX86_ADD_r32_r32 measures ADD r32,r32 throughput
func BenchmarkX86_ADD_r32_r32(b *testing.B) {
	cpu, bus := setupX86BenchCPU()
	cpu.EAX = 100
	cpu.EBX = 200
	// ADD EAX,EBX (01 D8)
	bus.mem[0x1000] = 0x01
	bus.mem[0x1001] = 0xD8
	cpu.EIP = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.EAX = 100
		cpu.Step()
	}
}

// BenchmarkX86_ADD_r32_imm measures ADD r32,imm32 throughput
func BenchmarkX86_ADD_r32_imm(b *testing.B) {
	cpu, bus := setupX86BenchCPU()
	cpu.EAX = 100
	// ADD EAX,$64 (05 64 00 00 00)
	bus.mem[0x1000] = 0x05
	bus.mem[0x1001] = 0x64
	bus.mem[0x1002] = 0x00
	bus.mem[0x1003] = 0x00
	bus.mem[0x1004] = 0x00
	cpu.EIP = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.EAX = 100
		cpu.Step()
	}
}

// BenchmarkX86_Step_Dispatch measures dispatch overhead with NOP
func BenchmarkX86_Step_Dispatch(b *testing.B) {
	cpu, bus := setupX86BenchCPU()
	// NOP (90)
	bus.mem[0x1000] = 0x90
	cpu.EIP = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.Step()
	}
}

// BenchmarkX86_JMP_rel measures JMP rel8 throughput
func BenchmarkX86_JMP_rel(b *testing.B) {
	cpu, bus := setupX86BenchCPU()
	// JMP $+0 (EB FE - jump to self)
	bus.mem[0x1000] = 0xEB
	bus.mem[0x1001] = 0xFE
	cpu.EIP = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.Step()
	}
}

// BenchmarkX86_PUSH_POP measures PUSH/POP throughput
func BenchmarkX86_PUSH_POP(b *testing.B) {
	cpu, bus := setupX86BenchCPU()
	cpu.EAX = 0x12345678
	cpu.ESP = 0x10000
	// PUSH EAX (50)
	bus.mem[0x1000] = 0x50
	// POP EBX (5B)
	bus.mem[0x1001] = 0x5B
	cpu.EIP = 0x1000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.ESP = 0x10000
		cpu.Step() // PUSH
		cpu.Step() // POP
	}
}

// =============================================================================
// M68K CPU Benchmarks
// Note: M68K already has m68k_benchmark_test.go with comprehensive benchmarks
// These are additional benchmarks for the unified test suite
// =============================================================================

// setupM68KBenchCPU creates a M68K CPU for benchmarking
func setupM68KBenchCPU() *M68KCPU {
	bus := NewMachineBus()
	bus.Write32(0, M68K_STACK_START)
	bus.Write32(M68K_RESET_VECTOR, M68K_ENTRY_POINT)
	return NewM68KCPU(bus)
}

// BenchmarkM68K_MOVE_Long measures MOVE.L Dn,Dm throughput
func BenchmarkM68K_MOVE_Long(b *testing.B) {
	cpu := setupM68KBenchCPU()
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

// BenchmarkM68K_ADD_Long measures ADD.L Dn,Dm throughput
func BenchmarkM68K_ADD_Long(b *testing.B) {
	cpu := setupM68KBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	cpu.DataRegs[0] = 100
	cpu.DataRegs[1] = 200
	// ADD.L D0,D1: 0xD280
	cpu.Write16(0x1000, 0xD280)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.DataRegs[1] = 200
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkM68K_Step_Dispatch measures dispatch overhead with NOP
func BenchmarkM68K_Step_Dispatch(b *testing.B) {
	cpu := setupM68KBenchCPU()
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

// BenchmarkM68K_MOVEQ measures MOVEQ throughput (fast immediate move)
func BenchmarkM68K_MOVEQ(b *testing.B) {
	cpu := setupM68KBenchCPU()
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

// BenchmarkM68K_BRA measures BRA throughput
func BenchmarkM68K_BRA(b *testing.B) {
	cpu := setupM68KBenchCPU()
	cpu.PC = 0x1000
	cpu.SR = M68K_SR_S
	// BRA.S +0: 0x6000 with word displacement
	cpu.Write16(0x1000, 0x6000)
	cpu.Write16(0x1002, 0x0002)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.PC = 0x1000
		cpu.currentIR = cpu.Fetch16()
		cpu.FetchAndDecodeInstruction()
	}
}

// BenchmarkM68K_DBcc measures DBRA throughput
func BenchmarkM68K_DBcc(b *testing.B) {
	cpu := setupM68KBenchCPU()
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
