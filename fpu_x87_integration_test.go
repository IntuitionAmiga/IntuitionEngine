package main

import (
	"math"
	"testing"
)

func writeCode(bus *TestX86Bus, addr uint32, code ...byte) {
	for i, b := range code {
		bus.memory[addr+uint32(i)] = b
	}
}

func write32le(bus *TestX86Bus, addr uint32, v uint32) {
	bus.memory[addr] = byte(v)
	bus.memory[addr+1] = byte(v >> 8)
	bus.memory[addr+2] = byte(v >> 16)
	bus.memory[addr+3] = byte(v >> 24)
}

func read16le(bus *TestX86Bus, addr uint32) uint16 {
	return uint16(bus.memory[addr]) | (uint16(bus.memory[addr+1]) << 8)
}

func TestX87_CPU_HasFPU(t *testing.T) {
	cpu := NewCPU_X86(NewTestX86Bus())
	if cpu.FPU == nil {
		t.Fatal("cpu.FPU should be initialized")
	}
}

func TestX87_CPU_ResetFPU(t *testing.T) {
	cpu := NewCPU_X86(NewTestX86Bus())
	cpu.FPU.push(3.14)
	cpu.FPU.FCW = 0
	cpu.Reset()
	if cpu.FPU.FCW != 0x037F || cpu.FPU.FTW != 0xFFFF || cpu.FPU.FSW != 0 {
		t.Fatalf("reset did not restore FPU defaults: FCW=%04X FTW=%04X FSW=%04X", cpu.FPU.FCW, cpu.FPU.FTW, cpu.FPU.FSW)
	}
}

func TestX87_Dispatch_D8_MemAndRegAndFNSTSWAX(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.EIP = 0

	cpu.FPU.push(1.0)

	dataAddr := uint32(0x200)
	cpu.FPU.storeFloat32(bus, dataAddr, 2.0)

	writeCode(bus, 0,
		0xD8, 0x05, byte(dataAddr), byte(dataAddr>>8), byte(dataAddr>>16), byte(dataAddr>>24), // FADD m32
		0xD8, 0xC1, // FADD ST(0),ST(1)
		0xDF, 0xE0, // FNSTSW AX
	)

	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 3.0) {
		t.Fatalf("after D8 mem ST0=%v want=3", cpu.FPU.ST(0))
	}
	if cpu.EIP != 6 {
		t.Fatalf("EIP after D8 mem = %d, want 6", cpu.EIP)
	}

	cpu.FPU.push(2.0) // ST0=2 ST1=3 for reg-form FADD ST0,ST1
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 5.0) {
		t.Fatalf("after D8 reg ST0=%v want=5", cpu.FPU.ST(0))
	}
	if cpu.EIP != 8 {
		t.Fatalf("EIP after D8 reg = %d, want 8", cpu.EIP)
	}

	cpu.FPU.FSW = 0x5A5A
	cpu.Step()
	if cpu.AX() != 0x5A5A {
		t.Fatalf("AX = 0x%04X want 0x5A5A", cpu.AX())
	}
}

func TestX87_FLD_FSTP_Integration(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	src := uint32(0x300)
	dst := uint32(0x320)
	cpu.FPU.storeFloat64(bus, src, math.Pi)

	writeCode(bus, 0,
		0xDD, 0x05, byte(src), byte(src>>8), byte(src>>16), byte(src>>24), // FLD m64
		0xDD, 0x1D, byte(dst), byte(dst>>8), byte(dst>>16), byte(dst>>24), // FSTP m64
	)
	cpu.Step()
	cpu.Step()
	got := cpu.FPU.loadFloat64(bus, dst)
	if !almostEq(got, math.Pi) {
		t.Fatalf("stored m64 got=%v want=%v", got, math.Pi)
	}
}

func TestX87_FNSTENV_And_FDSCapture(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.DS = 0x23
	cpu.SS = 0x2B
	cpu.ES = 0x33
	cpu.EBP = 0x500

	memAddr := uint32(0x600)
	envAddr := uint32(0x700)
	cpu.FPU.push(1)
	cpu.FPU.storeFloat32(bus, memAddr, 2)
	cpu.FPU.storeFloat32(bus, cpu.EBP+8, 3)

	// D8 05 [disp32]  (default DS),  D9 /6 [env]
	writeCode(bus, 0,
		0xD8, 0x05, byte(memAddr), byte(memAddr>>8), byte(memAddr>>16), byte(memAddr>>24),
		0xD9, 0x35, byte(envAddr), byte(envAddr>>8), byte(envAddr>>16), byte(envAddr>>24),
	)
	cpu.Step()
	cpu.Step()
	if got := read16le(bus, envAddr+24); got != cpu.DS {
		t.Fatalf("FDS after DS-based mem op = 0x%04X want 0x%04X", got, cpu.DS)
	}

	// D8 45 08 (EBP-based -> SS), FNSTENV
	writeCode(bus, 0,
		0xD8, 0x45, 0x08,
		0xD9, 0x35, byte(envAddr), byte(envAddr>>8), byte(envAddr>>16), byte(envAddr>>24),
	)
	cpu.EIP = 0
	cpu.Step()
	cpu.Step()
	if got := read16le(bus, envAddr+24); got != cpu.SS {
		t.Fatalf("FDS after EBP mem op = 0x%04X want 0x%04X", got, cpu.SS)
	}

	// ES override: 26 D8 05 [disp32], FNSTENV
	writeCode(bus, 0,
		0x26, 0xD8, 0x05, byte(memAddr), byte(memAddr>>8), byte(memAddr>>16), byte(memAddr>>24),
		0xD9, 0x35, byte(envAddr), byte(envAddr>>8), byte(envAddr>>16), byte(envAddr>>24),
	)
	cpu.EIP = 0
	cpu.Step()
	cpu.Step()
	if got := read16le(bus, envAddr+24); got != cpu.ES {
		t.Fatalf("FDS after ES override = 0x%04X want 0x%04X", got, cpu.ES)
	}
}

func TestX87_FNSTENV_OperandSizePrefixStill32bit(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	envAddr := uint32(0x900)

	for i := range 32 {
		bus.memory[envAddr+uint32(i)] = 0xAA
	}

	writeCode(bus, 0,
		0x66, 0xD9, 0x35, byte(envAddr), byte(envAddr>>8), byte(envAddr>>16), byte(envAddr>>24),
	)
	cpu.Step()
	// If 32-bit env was written, bytes beyond 14 should change
	unchanged := true
	for i := 14; i < 28; i++ {
		if bus.memory[envAddr+uint32(i)] != 0xAA {
			unchanged = false
			break
		}
	}
	if unchanged {
		t.Fatalf("expected 32-bit (28-byte) env write even with 0x66 prefix")
	}
}

func TestX87_FNSAVE_FRSTOR_Roundtrip(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	buf := uint32(0xA00)

	cpu.FPU.push(1.5)
	cpu.FPU.push(2.5)
	cpu.FPU.push(3.5)

	writeCode(bus, 0,
		0xDD, 0x35, byte(buf), byte(buf>>8), byte(buf>>16), byte(buf>>24), // FNSAVE
		0xDD, 0x25, byte(buf), byte(buf>>8), byte(buf>>16), byte(buf>>24), // FRSTOR
	)
	cpu.Step()
	if cpu.FPU.FTW != 0xFFFF {
		t.Fatalf("FNSAVE should reset FPU")
	}
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 3.5) || !almostEq(cpu.FPU.ST(1), 2.5) || !almostEq(cpu.FPU.ST(2), 1.5) {
		t.Fatalf("FRSTOR roundtrip mismatch ST0=%v ST1=%v ST2=%v", cpu.FPU.ST(0), cpu.FPU.ST(1), cpu.FPU.ST(2))
	}
}
