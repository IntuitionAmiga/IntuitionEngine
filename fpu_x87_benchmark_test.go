package main

import (
	"math"
	"testing"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func setupX87BenchCPU() (*CPU_X86, *x86BenchBus) {
	bus := &x86BenchBus{}
	cpu := NewCPU_X86(bus)
	return cpu, bus
}

func x87BenchWriteCode(bus *x86BenchBus, addr uint32, code ...byte) {
	for i, b := range code {
		bus.mem[addr+uint32(i)] = b
	}
}

func x87BenchWrite64(bus *x86BenchBus, addr uint32, v float64) {
	bits := math.Float64bits(v)
	for i := range 8 {
		bus.mem[addr+uint32(i)] = byte(bits >> (8 * i))
	}
}

func x87BenchWrite32f(bus *x86BenchBus, addr uint32, v float32) {
	bits := math.Float32bits(v)
	for i := range 4 {
		bus.mem[addr+uint32(i)] = byte(bits >> (8 * i))
	}
}

// ─── Isolated FPU operations ────────────────────────────────────────────────

func BenchmarkX87_Push(b *testing.B) {
	f := NewFPU_X87()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Reset()
		f.push(3.14)
	}
}

func BenchmarkX87_Pop(b *testing.B) {
	f := NewFPU_X87()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.Reset()
		f.push(3.14)
		f.pop()
	}
}

func BenchmarkX87_ST_Read(b *testing.B) {
	f := NewFPU_X87()
	f.push(1.0)
	f.push(2.0)
	f.push(3.0)
	var sink float64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = f.ST(0)
		sink = f.ST(1)
		sink = f.ST(2)
	}
	_ = sink
}

func BenchmarkX87_SetST(b *testing.B) {
	f := NewFPU_X87()
	f.push(1.0)
	f.push(2.0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.setST(0, 42.0)
	}
}

func BenchmarkX87_ClassifyTag_Normal(b *testing.B) {
	f := NewFPU_X87()
	var sink uint16
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = f.classifyTag(3.14)
	}
	_ = sink
}

func BenchmarkX87_ClassifyTag_Zero(b *testing.B) {
	f := NewFPU_X87()
	var sink uint16
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = f.classifyTag(0.0)
	}
	_ = sink
}

func BenchmarkX87_ClassifyTag_Special(b *testing.B) {
	f := NewFPU_X87()
	var sink uint16
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = f.classifyTag(math.NaN())
		sink = f.classifyTag(math.Inf(1))
	}
	_ = sink
}

func BenchmarkX87_DoCompare(b *testing.B) {
	f := NewFPU_X87()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f.doCompare(3.14, 2.72, true)
	}
}

// ─── CPU.Step binary ops ────────────────────────────────────────────────────

func BenchmarkX87_FADD_RegReg(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	cpu.FPU.push(2.0) // ST1
	cpu.FPU.push(3.0) // ST0
	// D8 C1 = FADD ST(0),ST(1)
	x87BenchWriteCode(bus, 0x1000, 0xD8, 0xC1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.regs[cpu.FPU.physReg(0)] = 3.0
		cpu.Step()
	}
}

func BenchmarkX87_FMUL_RegReg(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	cpu.FPU.push(2.0) // ST1
	cpu.FPU.push(3.0) // ST0
	// D8 C9 = FMUL ST(0),ST(1)
	x87BenchWriteCode(bus, 0x1000, 0xD8, 0xC9)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.regs[cpu.FPU.physReg(0)] = 3.0
		cpu.Step()
	}
}

func BenchmarkX87_FDIV_RegReg(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	cpu.FPU.push(2.0) // ST1
	cpu.FPU.push(6.0) // ST0
	// D8 F1 = FDIV ST(0),ST(1)
	x87BenchWriteCode(bus, 0x1000, 0xD8, 0xF1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.regs[cpu.FPU.physReg(0)] = 6.0
		cpu.Step()
	}
}

func BenchmarkX87_FADD_Mem32(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	cpu.FPU.push(3.0) // ST0
	// D8 05 00 0D 00 00 = FADD [0x0D00] m32real
	addr := uint32(0x0D00)
	x87BenchWrite32f(bus, addr, 2.0)
	x87BenchWriteCode(bus, 0x1000, 0xD8, 0x05,
		byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.regs[cpu.FPU.physReg(0)] = 3.0
		cpu.Step()
	}
}

func BenchmarkX87_FMUL_Mem64(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	cpu.FPU.push(3.0) // ST0
	// DC 0D 00 0D 00 00 = FMUL [0x0D00] m64real
	addr := uint32(0x0D00)
	x87BenchWrite64(bus, addr, 2.0)
	x87BenchWriteCode(bus, 0x1000, 0xDC, 0x0D,
		byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.regs[cpu.FPU.physReg(0)] = 3.0
		cpu.Step()
	}
}

// ─── CPU.Step D9 ops ────────────────────────────────────────────────────────

func BenchmarkX87_FCHS(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	cpu.FPU.push(3.14)
	// D9 E0 = FCHS
	x87BenchWriteCode(bus, 0x1000, 0xD9, 0xE0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.Step()
	}
}

func BenchmarkX87_FLD_STi(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	// D9 C1 = FLD ST(1)
	x87BenchWriteCode(bus, 0x1000, 0xD9, 0xC1)
	// Snapshot a clean 2-entry stack to restore each iteration
	cpu.FPU.push(1.0) // ST1
	cpu.FPU.push(2.0) // ST0
	saveFSW := cpu.FPU.FSW
	saveFTW := cpu.FPU.FTW
	saveRegs := cpu.FPU.regs
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.FSW = saveFSW
		cpu.FPU.FTW = saveFTW
		cpu.FPU.regs = saveRegs
		cpu.Step()
	}
}

func BenchmarkX87_FLDPI(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	// D9 EB = FLDPI
	x87BenchWriteCode(bus, 0x1000, 0xD9, 0xEB)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.Reset()
		cpu.Step()
	}
}

func BenchmarkX87_FSIN(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	cpu.FPU.push(1.0)
	// D9 FE = FSIN
	x87BenchWriteCode(bus, 0x1000, 0xD9, 0xFE)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.regs[cpu.FPU.physReg(0)] = 1.0
		cpu.Step()
	}
}

func BenchmarkX87_FSQRT(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	cpu.FPU.push(9.0)
	// D9 FA = FSQRT
	x87BenchWriteCode(bus, 0x1000, 0xD9, 0xFA)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.regs[cpu.FPU.physReg(0)] = 9.0
		cpu.Step()
	}
}

// ─── CPU.Step load/store ────────────────────────────────────────────────────

func BenchmarkX87_FLD_m64(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	addr := uint32(0x0D00)
	x87BenchWrite64(bus, addr, math.Pi)
	// DD 05 00 0D 00 00 = FLD [0x0D00] m64real
	x87BenchWriteCode(bus, 0x1000, 0xDD, 0x05,
		byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.Reset()
		cpu.Step()
	}
}

func BenchmarkX87_FSTP_m64(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	cpu.FPU.push(math.Pi)
	addr := uint32(0x0D00)
	// DD 1D 00 0D 00 00 = FSTP [0x0D00] m64real
	x87BenchWriteCode(bus, 0x1000, 0xDD, 0x1D,
		byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.Reset()
		cpu.FPU.push(math.Pi)
		cpu.Step()
	}
}

// ─── CPU.Step compare ───────────────────────────────────────────────────────

func BenchmarkX87_FCOM_RegReg(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	cpu.FPU.push(2.0) // ST1
	cpu.FPU.push(3.0) // ST0
	// D8 D1 = FCOM ST(1)
	x87BenchWriteCode(bus, 0x1000, 0xD8, 0xD1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.Step()
	}
}

func BenchmarkX87_FTST(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	cpu.FPU.push(3.14)
	// D9 E4 = FTST
	x87BenchWriteCode(bus, 0x1000, 0xD9, 0xE4)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.Step()
	}
}

// ─── Pipeline benchmark ────────────────────────────────────────────────────

func BenchmarkX87_MulAdd_Pipeline(b *testing.B) {
	cpu, bus := setupX87BenchCPU()
	addr1 := uint32(0x0D00) // a = 3.0
	addr2 := uint32(0x0D08) // b = 4.0
	addr3 := uint32(0x0D10) // result
	x87BenchWrite64(bus, addr1, 3.0)
	x87BenchWrite64(bus, addr2, 4.0)

	// FLD [addr1]         DD 05 ...
	// FMUL [addr1]        DC 0D ...  (ST0 * a = a*a)
	// FLD [addr2]         DD 05 ...
	// FMUL [addr2]        DC 0D ...  (ST0 * b = b*b)
	// FADDP               DE C1      (ST1 = a*a + b*b, pop)
	// FSTP [addr3]        DD 1D ...
	off := uint32(0x1000)
	// FLD m64 [addr1]
	x87BenchWriteCode(bus, off, 0xDD, 0x05,
		byte(addr1), byte(addr1>>8), byte(addr1>>16), byte(addr1>>24))
	off += 6
	// FMUL m64 [addr1]
	x87BenchWriteCode(bus, off, 0xDC, 0x0D,
		byte(addr1), byte(addr1>>8), byte(addr1>>16), byte(addr1>>24))
	off += 6
	// FLD m64 [addr2]
	x87BenchWriteCode(bus, off, 0xDD, 0x05,
		byte(addr2), byte(addr2>>8), byte(addr2>>16), byte(addr2>>24))
	off += 6
	// FMUL m64 [addr2]
	x87BenchWriteCode(bus, off, 0xDC, 0x0D,
		byte(addr2), byte(addr2>>8), byte(addr2>>16), byte(addr2>>24))
	off += 6
	// FADDP ST(1),ST(0)
	x87BenchWriteCode(bus, off, 0xDE, 0xC1)
	off += 2
	// FSTP m64 [addr3]
	x87BenchWriteCode(bus, off, 0xDD, 0x1D,
		byte(addr3), byte(addr3>>8), byte(addr3>>16), byte(addr3>>24))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cpu.EIP = 0x1000
		cpu.FPU.Reset()
		for range 6 {
			cpu.Step()
		}
	}
}
