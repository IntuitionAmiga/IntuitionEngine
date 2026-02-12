package main

import (
	"math"
	"testing"
)

func TestX87_FLD_StackOverflowFlags(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	for i := range 8 {
		cpu.FPU.push(float64(i + 1))
	}
	writeCode(bus, 0, 0xD9, 0xC0) // FLD ST0 (requires push)
	cpu.Step()
	if cpu.FPU.FSW&(x87FSW_IE|x87FSW_SF|x87FSW_C1) != (x87FSW_IE | x87FSW_SF | x87FSW_C1) {
		t.Fatalf("FLD overflow flags FSW=0x%04X", cpu.FPU.FSW)
	}
}

func TestX87_FST_EmptyUnderflowFlags(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	addr := uint32(0xD00)
	bus.memory[addr] = 0xAA
	writeCode(bus, 0,
		0xD9, 0x15, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24), // FST m32
	)
	cpu.Step()
	if cpu.FPU.FSW&(x87FSW_IE|x87FSW_SF) != (x87FSW_IE | x87FSW_SF) {
		t.Fatalf("FST empty underflow flags FSW=0x%04X", cpu.FPU.FSW)
	}
	if bus.memory[addr] != 0xAA {
		t.Fatalf("memory should remain unchanged on underflow")
	}
}

func TestX87_FST_m32_PrecisionLossSetsPE(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	addr := uint32(0xD20)
	cpu.FPU.push(math.Pi)
	writeCode(bus, 0,
		0xD9, 0x15, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24), // FST m32
	)
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_PE == 0 {
		t.Fatalf("precision loss should set PE")
	}
}

func TestX87_DA_DB_RegisterForms_ReservedNoCrash(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(1)
	top := cpu.FPU.top()
	ftw := cpu.FPU.FTW
	writeCode(bus, 0,
		0xDA, 0xC0, // reserved on 387
		0xDB, 0xC0, // reserved on 387
	)
	cpu.Step()
	cpu.Step()
	if cpu.FPU.top() != top || cpu.FPU.FTW != ftw || !almostEq(cpu.FPU.ST(0), 1) {
		t.Fatalf("reserved reg-form ops should not mutate core stack state")
	}
}

func TestX87_FNCLEX_ClearsBBit(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.FSW = x87FSW_B | x87FSW_ES | x87FSW_IE | x87FSW_PE
	writeCode(bus, 0, 0xDB, 0xE2) // FNCLEX
	cpu.Step()
	if cpu.FPU.FSW&(x87FSW_B|x87FSW_ES|x87FSW_IE|x87FSW_PE) != 0 {
		t.Fatalf("FNCLEX should clear B/ES/exceptions, FSW=0x%04X", cpu.FPU.FSW)
	}
}

func TestX87_FYL2X_ZeroSetsZE(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(1.0)
	cpu.FPU.push(0.0)
	writeCode(bus, 0, 0xD9, 0xF1) // FYL2X
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_ZE == 0 {
		t.Fatalf("FYL2X x==0 should set ZE")
	}
}

func TestX87_FYL2XP1_DomainIE(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(1.0)
	cpu.FPU.push(-1.0)
	writeCode(bus, 0, 0xD9, 0xF9) // FYL2XP1 domain edge
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_IE == 0 {
		t.Fatalf("FYL2XP1 x<=-1 should set IE")
	}
}

func TestX87_FSINCOS_StackOverflowFlags(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	for i := range 8 {
		cpu.FPU.push(float64(i + 1))
	}
	writeCode(bus, 0, 0xD9, 0xFB) // FSINCOS (pushes one)
	cpu.Step()
	if cpu.FPU.FSW&(x87FSW_IE|x87FSW_SF|x87FSW_C1) != (x87FSW_IE | x87FSW_SF | x87FSW_C1) {
		t.Fatalf("FSINCOS overflow flags FSW=0x%04X", cpu.FPU.FSW)
	}
}

func TestX87_FXAM_ClassAndSignCases(t *testing.T) {
	bus := NewTestX86Bus()

	check := func(v float64, wantC3, wantC2, wantC0, wantC1 bool) {
		cpu := NewCPU_X86(bus)
		cpu.FPU.push(v)
		writeCode(bus, 0, 0xD9, 0xE5) // FXAM
		cpu.Step()
		c0 := (cpu.FPU.FSW & x87FSW_C0) != 0
		c1 := (cpu.FPU.FSW & x87FSW_C1) != 0
		c2 := (cpu.FPU.FSW & x87FSW_C2) != 0
		c3 := (cpu.FPU.FSW & x87FSW_C3) != 0
		if c3 != wantC3 || c2 != wantC2 || c0 != wantC0 || c1 != wantC1 {
			t.Fatalf("FXAM(%v) C3C2C0C1 got %v%v%v%v want %v%v%v%v", v, b2i(c3), b2i(c2), b2i(c0), b2i(c1), b2i(wantC3), b2i(wantC2), b2i(wantC0), b2i(wantC1))
		}
	}

	check(math.NaN(), false, false, true, false)                           // 001
	check(math.Inf(1), false, true, true, false)                           // 011
	check(0.0, true, false, false, false)                                  // 100
	check(-1.25, false, true, false, true)                                 // 010 + sign
	check(math.Copysign(x87SmallestNormal/2, -1), true, true, false, true) // 110 + sign
}

func b2i(v bool) int {
	if v {
		return 1
	}
	return 0
}

func TestX87_FCOS_FPTAN_LargeArgDeviationC2Clear(t *testing.T) {
	bus := NewTestX86Bus()
	large := math.Pow(2, 63) * 2

	cpu := NewCPU_X86(bus)
	cpu.FPU.push(large)
	writeCode(bus, 0, 0xD9, 0xFF) // FCOS
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_C2 != 0 {
		t.Fatalf("FCOS large arg deviation should keep C2=0")
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(large)
	writeCode(bus, 0, 0xD9, 0xF2) // FPTAN
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_C2 != 0 {
		t.Fatalf("FPTAN large arg deviation should keep C2=0")
	}
}

func TestX87_FISTP_m16_m32_PopBehavior(t *testing.T) {
	bus := NewTestX86Bus()

	cpu := NewCPU_X86(bus)
	m16 := uint32(0xDA0)
	cpu.FPU.push(42.0)
	writeCode(bus, 0, 0xDF, 0x1D, byte(m16), byte(m16>>8), byte(m16>>16), byte(m16>>24)) // FISTP m16
	cpu.Step()
	if read16le(bus, m16) != 42 || cpu.FPU.FTW != 0xFFFF {
		t.Fatalf("FISTP m16 should store+pop")
	}

	cpu = NewCPU_X86(bus)
	m32 := uint32(0xDB0)
	cpu.FPU.push(1234.0)
	writeCode(bus, 0, 0xDB, 0x1D, byte(m32), byte(m32>>8), byte(m32>>16), byte(m32>>24)) // FISTP m32
	cpu.Step()
	if int32(read32u(bus, m32)) != 1234 || cpu.FPU.FTW != 0xFFFF {
		t.Fatalf("FISTP m32 should store+pop")
	}
}
