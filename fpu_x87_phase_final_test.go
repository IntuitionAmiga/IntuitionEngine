package main

import (
	"math"
	"testing"
)

func TestX87_FUCOM_FUCOMP_PopSemantics(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(2.0)
	cpu.FPU.push(1.0)
	writeCode(bus, 0, 0xDD, 0xE1) // FUCOM ST1
	cpu.Step()
	top1 := cpu.FPU.top()
	if top1 != 6 {
		t.Fatalf("FUCOM should not pop, TOP=%d", top1)
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(2.0)
	cpu.FPU.push(1.0)
	writeCode(bus, 0, 0xDD, 0xE9) // FUCOMP ST1
	cpu.Step()
	if cpu.FPU.top() != 7 {
		t.Fatalf("FUCOMP should pop once, TOP=%d", cpu.FPU.top())
	}
}

func TestX87_FILD_FIST_Variants(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	m16 := uint32(0xA00)
	m32 := uint32(0xA10)
	m64 := uint32(0xA20)
	out16 := uint32(0xA30)
	out32 := uint32(0xA40)
	out64 := uint32(0xA50)

	// +7, -9, +11
	bus.memory[m16] = 7
	bus.memory[m16+1] = 0
	neg9 := int32(-9)
	write32le(bus, m32, uint32(neg9))
	v64 := uint64(11)
	for i := range 8 {
		bus.memory[m64+uint32(i)] = byte(v64 >> (8 * i))
	}

	writeCode(bus, 0,
		0xDF, 0x05, byte(m16), byte(m16>>8), byte(m16>>16), byte(m16>>24), // FILD m16
		0xDB, 0x05, byte(m32), byte(m32>>8), byte(m32>>16), byte(m32>>24), // FILD m32
		0xDF, 0x2D, byte(m64), byte(m64>>8), byte(m64>>16), byte(m64>>24), // FILD m64
		0xDF, 0x15, byte(out16), byte(out16>>8), byte(out16>>16), byte(out16>>24), // FIST m16 (from 11)
		0xDB, 0x15, byte(out32), byte(out32>>8), byte(out32>>16), byte(out32>>24), // FIST m32 (from 11)
		0xDF, 0x3D, byte(out64), byte(out64>>8), byte(out64>>16), byte(out64>>24), // FISTP m64 (from 11)
	)
	for range 6 {
		cpu.Step()
	}
	if got := read16le(bus, out16); got != 11 {
		t.Fatalf("FIST m16 got=%d want=11", got)
	}
	if got := int32(read32u(bus, out32)); got != 11 {
		t.Fatalf("FIST m32 got=%d want=11", got)
	}
	got64 := uint64(0)
	for i := range 8 {
		got64 |= uint64(bus.memory[out64+uint32(i)]) << (8 * i)
	}
	if int64(got64) != 11 {
		t.Fatalf("FISTP m64 got=%d want=11", int64(got64))
	}
}

func TestX87_FPREM_QuotientBits(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(3.0)
	cpu.FPU.push(17.0) // q=trunc(17/3)=5 (101)
	writeCode(bus, 0, 0xD9, 0xF8)
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 2.0) {
		t.Fatalf("FPREM result got=%v want=2", cpu.FPU.ST(0))
	}
	if cpu.FPU.FSW&x87FSW_C0 == 0 || cpu.FPU.FSW&x87FSW_C1 == 0 || cpu.FPU.FSW&x87FSW_C3 != 0 {
		t.Fatalf("FPREM quotient bits mismatch FSW=0x%04X", cpu.FPU.FSW)
	}
}

func TestX87_FPREM1_QuotientBits(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(3.0)
	cpu.FPU.push(17.0) // q=round_even(17/3)=6 (110)
	writeCode(bus, 0, 0xD9, 0xF5)
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), -1.0) {
		t.Fatalf("FPREM1 result got=%v want=-1", cpu.FPU.ST(0))
	}
	if cpu.FPU.FSW&x87FSW_C0 == 0 || cpu.FPU.FSW&x87FSW_C3 == 0 || cpu.FPU.FSW&x87FSW_C1 != 0 {
		t.Fatalf("FPREM1 quotient bits mismatch FSW=0x%04X", cpu.FPU.FSW)
	}
}

func TestX87_EnvLayoutAndRestoreHelpers(t *testing.T) {
	bus := NewTestX86Bus()
	f := NewFPU_X87()
	f.FCW = 0x1234
	f.FSW = 0x5678
	f.FTW = 0x9ABC
	f.FIP = 0x11223344
	f.FCS = 0x3344
	f.FOP = 0x2AA
	f.FDP = 0x55667788
	f.FDS = 0x7788

	f.fnstenv32(bus, 0xB00)
	if read32u(bus, 0xB00+0) != 0x00001234 || read32u(bus, 0xB00+4) != 0x00005678 || read32u(bus, 0xB00+8) != 0x00009ABC {
		t.Fatalf("env scalar fields layout mismatch")
	}
	mix := read32u(bus, 0xB00+16)
	if uint16(mix) != 0x3344 || ((mix>>16)&0x7FF) != 0x2AA {
		t.Fatalf("env FCS/FOP mix mismatch 0x%08X", mix)
	}

	g := NewFPU_X87()
	g.fldenv32(bus, 0xB00)
	if g.FCW != 0x1234 || g.FSW != 0x5678 || g.FTW != 0x9ABC || g.FIP != 0x11223344 || g.FDP != 0x55667788 || g.FDS != 0x7788 || g.FCS != 0x3344 || g.FOP != 0x2AA {
		t.Fatalf("fldenv restore mismatch")
	}
}

func TestX87_FNSAVE_PhysicalRegisterOrder(t *testing.T) {
	bus := NewTestX86Bus()
	f := NewFPU_X87()
	for i := range 8 {
		f.regs[i] = float64(100 + i)
		f.setTag(i, x87TagValid)
	}
	f.setTop(3)

	f.fsave32(bus, 0xC00)
	if f.FCW != 0x037F || f.FTW != 0xFFFF {
		t.Fatalf("fsave should reset FPU")
	}

	for i := range 8 {
		got := NewFPU_X87().loadExtended80(bus, 0xC00+28+uint32(i*10))
		want := float64(100 + i)
		if !almostEq(got, want) {
			t.Fatalf("physical reg order mismatch at %d got=%v want=%v", i, got, want)
		}
	}
}

func TestX87_FXTRACT_SignificandRange(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(-6.5)
	writeCode(bus, 0, 0xD9, 0xF4)
	cpu.Step()
	sig := cpu.FPU.ST(1)
	if !(math.Abs(sig) >= 1.0 && math.Abs(sig) < 2.0) {
		t.Fatalf("FXTRACT significand range invalid: %v", sig)
	}
}

func TestX87_FYL2X_DomainErrors(t *testing.T) {
	bus := NewTestX86Bus()

	cpu := NewCPU_X86(bus)
	cpu.FPU.push(1.0)
	cpu.FPU.push(-1.0)
	writeCode(bus, 0, 0xD9, 0xF1)
	cpu.Step()
	if !math.IsNaN(cpu.FPU.ST(0)) || cpu.FPU.FSW&x87FSW_IE == 0 {
		t.Fatalf("FYL2X x<0 should be NaN + IE")
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(1.0)
	cpu.FPU.push(0.0)
	writeCode(bus, 0, 0xD9, 0xF1)
	cpu.Step()
	if !math.IsInf(cpu.FPU.ST(0), -1) {
		t.Fatalf("FYL2X x=0 should be -Inf")
	}
}
