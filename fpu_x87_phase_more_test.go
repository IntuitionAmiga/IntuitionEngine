package main

import (
	"math"
	"testing"
)

func read64f(bus *TestX86Bus, addr uint32) float64 {
	bits := uint64(read32u(bus, addr)) | (uint64(read32u(bus, addr+4)) << 32)
	return math.Float64frombits(bits)
}

func TestX87_Control_FNINIT_FLDCW_FNSTCW_FNCLEX_FNSTSWm16(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cwAddr := uint32(0x500)
	swAddr := uint32(0x510)
	bus.memory[cwAddr] = 0x7F
	bus.memory[cwAddr+1] = 0x0B // RC=10 (up)

	cpu.FPU.FCW = 0
	cpu.FPU.FSW = 0xFFFF
	cpu.FPU.FTW = 0
	writeCode(bus, 0,
		0xD9, 0x2D, byte(cwAddr), byte(cwAddr>>8), byte(cwAddr>>16), byte(cwAddr>>24), // FLDCW
		0xD9, 0x3D, byte(cwAddr+2), byte((cwAddr+2)>>8), byte((cwAddr+2)>>16), byte((cwAddr+2)>>24), // FNSTCW
		0xDB, 0xE2, // FNCLEX
		0xDD, 0x3D, byte(swAddr), byte(swAddr>>8), byte(swAddr>>16), byte(swAddr>>24), // FNSTSW m16
		0xDB, 0xE3, // FNINIT
	)
	cpu.Step()
	if cpu.FPU.FCW != 0x0B7F {
		t.Fatalf("FLDCW FCW=0x%04X want 0x0B7F", cpu.FPU.FCW)
	}
	cpu.Step()
	if got := read16le(bus, cwAddr+2); got != 0x0B7F {
		t.Fatalf("FNSTCW mem=0x%04X want 0x0B7F", got)
	}

	cpu.FPU.FSW = 0x80FF
	cpu.Step() // FNCLEX
	if cpu.FPU.FSW&0x80FF != 0 {
		t.Fatalf("FNCLEX did not clear exception/status bits FSW=0x%04X", cpu.FPU.FSW)
	}

	cpu.FPU.FSW = 0x1357
	cpu.Step() // FNSTSW m16
	if got := read16le(bus, swAddr); got != 0x1357 {
		t.Fatalf("FNSTSW m16=0x%04X want 0x1357", got)
	}

	cpu.Step() // FNINIT
	if cpu.FPU.FCW != 0x037F || cpu.FPU.FSW != 0 || cpu.FPU.FTW != 0xFFFF {
		t.Fatalf("FNINIT reset mismatch FCW=%04X FSW=%04X FTW=%04X", cpu.FPU.FCW, cpu.FPU.FSW, cpu.FPU.FTW)
	}
}

func TestX87_FCOM_vs_FUCOM_NaN_IEBehavior(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(1.0)
	cpu.FPU.push(math.NaN()) // ST0=NaN ST1=1

	writeCode(bus, 0,
		0xD8, 0xD1, // FCOM ST1 (signals IE on NaN)
		0xDD, 0xE1, // FUCOM ST1 (no IE on NaN)
	)
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_IE == 0 {
		t.Fatalf("FCOM NaN should set IE")
	}

	cpu.FPU.FSW &= x87FSW_TOPMask
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_IE != 0 {
		t.Fatalf("FUCOM NaN should not set IE")
	}
	if (cpu.FPU.FSW & (x87FSW_C0 | x87FSW_C2 | x87FSW_C3)) != (x87FSW_C0 | x87FSW_C2 | x87FSW_C3) {
		t.Fatalf("FUCOM NaN condition bits mismatch FSW=0x%04X", cpu.FPU.FSW)
	}
}

func TestX87_DC_DE_SwapEncodings_SubDiv(t *testing.T) {
	mk := func(op1, op2 byte) (float64, float64) {
		bus := NewTestX86Bus()
		cpu := NewCPU_X86(bus)
		cpu.FPU.push(10.0) // ST0
		cpu.FPU.push(2.0)  // ST0=2 ST1=10
		writeCode(bus, 0, 0xDC, op1, 0xDE, op2)
		cpu.Step() // DC reg-form
		a := cpu.FPU.ST(1)
		cpu.Step() // DE reg-form pop-form
		b := cpu.FPU.ST(0)
		return a, b
	}

	// DC E9: FSUB ST1,ST0 -> 10-2=8 ; DE E9: FSUBP ST1,ST0 -> 8-2=6 then pop
	a, b := mk(0xE9, 0xE9)
	if !almostEq(a, 8.0) || !almostEq(b, 6.0) {
		t.Fatalf("FSUB swapped encoding mismatch a=%v b=%v", a, b)
	}

	// DC E1: FSUBR ST1,ST0 -> 2-10=-8 ; DE E1: FSUBRP ST1,ST0 -> 2-(-8)=10 then pop
	a, b = mk(0xE1, 0xE1)
	if !almostEq(a, -8.0) || !almostEq(b, 10.0) {
		t.Fatalf("FSUBR swapped encoding mismatch a=%v b=%v", a, b)
	}

	mk2 := func(op byte) float64 {
		bus := NewTestX86Bus()
		cpu := NewCPU_X86(bus)
		cpu.FPU.push(10.0)
		cpu.FPU.push(2.0) // ST0=2 ST1=10
		writeCode(bus, 0, 0xDC, op)
		cpu.Step()
		return cpu.FPU.ST(1)
	}
	if got := mk2(0xF9); !almostEq(got, 5.0) { // FDIV ST1,ST0
		t.Fatalf("FDIV swapped encoding got=%v want=5", got)
	}
	if got := mk2(0xF1); !almostEq(got, 0.2) { // FDIVR ST1,ST0
		t.Fatalf("FDIVR swapped encoding got=%v want=0.2", got)
	}
}

func TestX87_Transcendentals_FSINCOS_FPREM_FPREM1(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(math.Pi / 2)
	writeCode(bus, 0, 0xD9, 0xFB) // FSINCOS
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), math.Cos(math.Pi/2)) || !almostEq(cpu.FPU.ST(1), math.Sin(math.Pi/2)) {
		t.Fatalf("FSINCOS ordering mismatch ST0=%v ST1=%v", cpu.FPU.ST(0), cpu.FPU.ST(1))
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(3.0)
	cpu.FPU.push(17.0)                        // ST0=17 ST1=3
	writeCode(bus, 0, 0xD9, 0xF8, 0xD9, 0xF5) // FPREM; FPREM1
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 2.0) {
		t.Fatalf("FPREM result=%v want=2", cpu.FPU.ST(0))
	}
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), -1.0) {
		t.Fatalf("FPREM1 result=%v want=-1", cpu.FPU.ST(0))
	}
}

func TestX87_FBLD_FBSTP_RoundTrip(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	in := uint32(0x580)
	out := uint32(0x590)
	cpu.FPU.storeBCD(bus, in, -123456789012345678)
	writeCode(bus, 0,
		0xDF, 0x25, byte(in), byte(in>>8), byte(in>>16), byte(in>>24), // FBLD m80bcd
		0xDF, 0x35, byte(out), byte(out>>8), byte(out>>16), byte(out>>24), // FBSTP m80bcd
	)
	cpu.Step()
	cpu.Step()
	for i := range 10 {
		if bus.memory[in+uint32(i)] != bus.memory[out+uint32(i)] {
			t.Fatalf("BCD byte %d mismatch in=%02X out=%02X", i, bus.memory[in+uint32(i)], bus.memory[out+uint32(i)])
		}
	}
}

func TestX87_Integration_SqrtPi(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	out := uint32(0x5C0)
	writeCode(bus, 0,
		0xD9, 0xEB, // FLDPI
		0xD9, 0xFA, // FSQRT
		0xDD, 0x1D, byte(out), byte(out>>8), byte(out>>16), byte(out>>24), // FSTP qword [out]
	)
	cpu.Step()
	cpu.Step()
	cpu.Step()
	if got := read64f(bus, out); !almostEq(got, math.Sqrt(math.Pi)) {
		t.Fatalf("sqrt(pi) got=%v want=%v", got, math.Sqrt(math.Pi))
	}
}

func TestX87_ConstantLoads_All(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	writeCode(bus, 0,
		0xD9, 0xE8, // FLD1
		0xD9, 0xE9, // FLDL2T
		0xD9, 0xEA, // FLDL2E
		0xD9, 0xEB, // FLDPI
		0xD9, 0xEC, // FLDLG2
		0xD9, 0xED, // FLDLN2
		0xD9, 0xEE, // FLDZ
	)
	for range 7 {
		cpu.Step()
	}
	want := []float64{0.0, math.Ln2, math.Log10(2), math.Pi, math.Log2E, math.Log2(10), 1.0}
	for i, w := range want {
		if !almostEq(cpu.FPU.ST(i), w) {
			t.Fatalf("constant ST(%d) got=%v want=%v", i, cpu.FPU.ST(i), w)
		}
	}
	if cpu.FPU.getTag(cpu.FPU.physReg(0)) != x87TagZero {
		t.Fatalf("FLDZ should produce zero tag")
	}
}

func TestX87_UnaryOps_FCHS_FABS_FSQRT_FRNDINTModes(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(0.0)
	writeCode(bus, 0, 0xD9, 0xE0, 0xD9, 0xE1) // FCHS; FABS
	cpu.Step()
	if !math.Signbit(cpu.FPU.ST(0)) {
		t.Fatalf("FCHS +0 should become -0")
	}
	cpu.Step()
	if math.Signbit(cpu.FPU.ST(0)) {
		t.Fatalf("FABS should clear sign")
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(4.0)
	writeCode(bus, 0, 0xD9, 0xFA) // FSQRT
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 2.0) {
		t.Fatalf("FSQRT(4) got=%v", cpu.FPU.ST(0))
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(-1.0)
	writeCode(bus, 0, 0xD9, 0xFA)
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_IE == 0 {
		t.Fatalf("FSQRT(-1) should set IE")
	}

	testRound := func(rc uint16, in float64, want float64) {
		cpu := NewCPU_X86(bus)
		cpu.FPU.FCW = (cpu.FPU.FCW &^ x87FCW_RCMask) | (rc << x87FCW_RCShift)
		cpu.FPU.push(in)
		writeCode(bus, 0, 0xD9, 0xFC) // FRNDINT
		cpu.Step()
		if !almostEq(cpu.FPU.ST(0), want) {
			t.Fatalf("FRNDINT rc=%d in=%v got=%v want=%v", rc, in, cpu.FPU.ST(0), want)
		}
	}
	testRound(x87FCW_RCNearest, 2.5, 2.0)
	testRound(x87FCW_RCDown, 2.5, 2.0)
	testRound(x87FCW_RCUp, 2.5, 3.0)
	testRound(x87FCW_RCChop, -2.5, -2.0)
}

func TestX87_FIST_OverflowStoresIndefinite(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	addr := uint32(0x640)
	cpu.FPU.push(100000.0)
	writeCode(bus, 0,
		0xDF, 0x15, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24), // FIST m16
	)
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_IE == 0 {
		t.Fatalf("FIST overflow should set IE")
	}
	if got := read16le(bus, addr); got != 0x8000 {
		t.Fatalf("FIST overflow indefinite got=0x%04X want=0x8000", got)
	}
}

func TestX87_FNSTENV_MasksExceptions_And_CapturesPointers(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.CS = 0x1234
	opAddr := uint32(0x680)
	envAddr := uint32(0x6C0)
	write32f(bus, opAddr, 2.0)
	cpu.FPU.push(1.0)
	cpu.FPU.FCW = 0

	writeCode(bus, 0,
		0xD8, 0x05, byte(opAddr), byte(opAddr>>8), byte(opAddr>>16), byte(opAddr>>24), // FADD m32
		0xD9, 0x35, byte(envAddr), byte(envAddr>>8), byte(envAddr>>16), byte(envAddr>>24), // FNSTENV
	)
	cpu.Step()
	cpu.Step()

	if cpu.FPU.FCW&0x003F != 0x003F {
		t.Fatalf("FNSTENV should mask exceptions in FCW")
	}
	if got := read32u(bus, envAddr+12); got != 6 {
		t.Fatalf("saved FIP got=0x%08X want 0x00000006", got)
	}
	mix := read32u(bus, envAddr+16)
	if uint16(mix) != cpu.CS {
		t.Fatalf("saved FCS got=0x%04X want=0x%04X", uint16(mix), cpu.CS)
	}
	if (mix>>16)&0x7FF != uint32(0x135) { // FNSTENV itself: D9 /6 with modrm=0x35
		t.Fatalf("saved FOP got=0x%03X want=0x135", (mix>>16)&0x7FF)
	}
	if got := read32u(bus, envAddr+20); got != opAddr {
		t.Fatalf("saved FDP got=0x%08X want=0x%08X", got, opAddr)
	}
}

func TestX87_Transcendentals_DeviationsAndCoreOps(t *testing.T) {
	bus := NewTestX86Bus()

	cpu := NewCPU_X86(bus)
	cpu.FPU.push(math.Pow(2, 63) * 2)
	writeCode(bus, 0, 0xD9, 0xFE) // FSIN large arg
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_C2 != 0 {
		t.Fatalf("FSIN large arg deviation: expected C2=0")
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(0.5)
	writeCode(bus, 0, 0xD9, 0xF2) // FPTAN
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 1.0) || !almostEq(cpu.FPU.ST(1), math.Tan(0.5)) {
		t.Fatalf("FPTAN stack/result mismatch ST0=%v ST1=%v", cpu.FPU.ST(0), cpu.FPU.ST(1))
	}
	if cpu.FPU.FSW&x87FSW_C2 != 0 {
		t.Fatalf("FPTAN expected C2=0")
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(8.0)
	writeCode(bus, 0, 0xD9, 0xF4) // FXTRACT
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 3.0) || !almostEq(cpu.FPU.ST(1), 1.0) {
		t.Fatalf("FXTRACT mismatch ST0=%v ST1=%v", cpu.FPU.ST(0), cpu.FPU.ST(1))
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(0.5)
	writeCode(bus, 0, 0xD9, 0xF0) // F2XM1
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), math.Exp2(0.5)-1.0) {
		t.Fatalf("F2XM1 mismatch")
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(1.0)             // y
	cpu.FPU.push(8.0)             // x
	writeCode(bus, 0, 0xD9, 0xF1) // FYL2X
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 3.0) {
		t.Fatalf("FYL2X mismatch got=%v", cpu.FPU.ST(0))
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(2.0)             // y
	cpu.FPU.push(0.5)             // x
	writeCode(bus, 0, 0xD9, 0xF9) // FYL2XP1 => y*log2(1.5)
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 2.0*math.Log2(1.5)) {
		t.Fatalf("FYL2XP1 mismatch got=%v", cpu.FPU.ST(0))
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(3.0)             // scale in ST1 after next push
	cpu.FPU.push(1.5)             // ST0
	writeCode(bus, 0, 0xD9, 0xFD) // FSCALE
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 12.0) {
		t.Fatalf("FSCALE mismatch got=%v", cpu.FPU.ST(0))
	}
}

func TestX87_ComparePopForms_FCOMP_FCOMPP(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(1.0)
	cpu.FPU.push(2.0)             // ST0=2 ST1=1
	writeCode(bus, 0, 0xD8, 0xD9) // FCOM ST1
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_C0 != 0 || cpu.FPU.FSW&x87FSW_C3 != 0 {
		t.Fatalf("FCOM greater should clear C0/C3 FSW=0x%04X", cpu.FPU.FSW)
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(2.0)
	cpu.FPU.push(1.0)             // ST0=1 ST1=2
	writeCode(bus, 0, 0xD8, 0xD9) // FCOM ST1, less
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_C0 == 0 {
		t.Fatalf("FCOM less should set C0")
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(1.0)
	cpu.FPU.push(1.0)
	writeCode(bus, 0, 0xDE, 0xD9) // FCOMPP
	cpu.Step()
	if cpu.FPU.FTW != 0xFFFF {
		t.Fatalf("FCOMPP should pop two values FTW=0x%04X", cpu.FPU.FTW)
	}
}

func TestX87_Integration_MulAdd(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	a := uint32(0x700)
	b := uint32(0x708)
	cc := uint32(0x710)
	d := uint32(0x718)
	out := uint32(0x720)
	write64f(bus, a, 2)
	write64f(bus, b, 3)
	write64f(bus, cc, 4)
	write64f(bus, d, 5)

	writeCode(bus, 0,
		0xDD, 0x05, byte(a), byte(a>>8), byte(a>>16), byte(a>>24), // FLD [a]
		0xDC, 0x0D, byte(b), byte(b>>8), byte(b>>16), byte(b>>24), // FMUL [b]
		0xDD, 0x05, byte(cc), byte(cc>>8), byte(cc>>16), byte(cc>>24), // FLD [c]
		0xDC, 0x0D, byte(d), byte(d>>8), byte(d>>16), byte(d>>24), // FMUL [d]
		0xDE, 0xC1, // FADDP ST1,ST0
		0xDD, 0x1D, byte(out), byte(out>>8), byte(out>>16), byte(out>>24), // FSTP [out]
	)
	for range 6 {
		cpu.Step()
	}
	if got := read64f(bus, out); !almostEq(got, 26.0) {
		t.Fatalf("muladd got=%v want=26", got)
	}
}

func TestX87_Integration_SaveRestore_Sequence(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	buf := uint32(0x780)
	out := uint32(0x800)
	cpu.FPU.push(7.0)
	cpu.FPU.push(9.0)

	writeCode(bus, 0,
		0xDD, 0x35, byte(buf), byte(buf>>8), byte(buf>>16), byte(buf>>24), // FNSAVE
		0xD9, 0xE8, // FLD1 (mutate)
		0xDD, 0x25, byte(buf), byte(buf>>8), byte(buf>>16), byte(buf>>24), // FRSTOR
		0xDD, 0x1D, byte(out), byte(out>>8), byte(out>>16), byte(out>>24), // FSTP [out] => expect 9
	)
	for range 4 {
		cpu.Step()
	}
	if got := read64f(bus, out); !almostEq(got, 9.0) {
		t.Fatalf("restore sequence top got=%v want=9", got)
	}
}

func TestX87_FDIVR_DivideByZeroFlags(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(1.0)             // ST1
	cpu.FPU.push(0.0)             // ST0 denominator for FDIVR ST0,ST1
	writeCode(bus, 0, 0xD8, 0xF9) // FDIVR ST0,ST1 => ST0=ST1/ST0
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_ZE == 0 {
		t.Fatalf("FDIVR divide-by-zero should set ZE, FSW=0x%04X", cpu.FPU.FSW)
	}
}

func TestX87_IntegerCompareAndArithmeticMemoryForms(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	m16 := uint32(0x840)
	m32 := uint32(0x850)

	// m16 = 3, m32 = 5
	bus.memory[m16] = 3
	bus.memory[m16+1] = 0
	write32le(bus, m32, 5)

	cpu.FPU.push(10.0)
	writeCode(bus, 0,
		0xDE, 0x05, byte(m16), byte(m16>>8), byte(m16>>16), byte(m16>>24), // FIADD m16 => 13
		0xDA, 0x25, byte(m32), byte(m32>>8), byte(m32>>16), byte(m32>>24), // FISUB m32 => 8
	)
	cpu.Step()
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 8.0) {
		t.Fatalf("integer mem arithmetic result got=%v want=8", cpu.FPU.ST(0))
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(3.0)
	writeCode(bus, 0,
		0xDE, 0x15, byte(m16), byte(m16>>8), byte(m16>>16), byte(m16>>24), // FICOM m16 (eq)
		0xDA, 0x1D, byte(m32), byte(m32>>8), byte(m32>>16), byte(m32>>24), // FICOMP m32 (3<5, pop)
	)
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_C3 == 0 {
		t.Fatalf("FICOM equal should set C3")
	}
	beforeFTW := cpu.FPU.FTW
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_C0 == 0 {
		t.Fatalf("FICOMP less should set C0")
	}
	if cpu.FPU.FTW == beforeFTW {
		t.Fatalf("FICOMP should pop")
	}
}

func TestX87_M80_LoadStore_Opcodes(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	in := uint32(0x880)
	out := uint32(0x8A0)
	cpu.FPU.storeExtended80(bus, in, math.Pi)

	writeCode(bus, 0,
		0xDB, 0x2D, byte(in), byte(in>>8), byte(in>>16), byte(in>>24), // FLD m80
		0xDB, 0x3D, byte(out), byte(out>>8), byte(out>>16), byte(out>>24), // FSTP m80
	)
	cpu.Step()
	cpu.Step()
	if got := cpu.FPU.loadExtended80(bus, out); !almostEq(got, math.Pi) {
		t.Fatalf("m80 roundtrip got=%v want=%v", got, math.Pi)
	}
}

func TestX87_FISTP_m64_Path(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	addr := uint32(0x8C0)
	cpu.FPU.push(123456789.0)
	writeCode(bus, 0,
		0xDF, 0x3D, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24), // FISTP m64
	)
	cpu.Step()
	got := uint64(bus.memory[addr]) |
		(uint64(bus.memory[addr+1]) << 8) |
		(uint64(bus.memory[addr+2]) << 16) |
		(uint64(bus.memory[addr+3]) << 24) |
		(uint64(bus.memory[addr+4]) << 32) |
		(uint64(bus.memory[addr+5]) << 40) |
		(uint64(bus.memory[addr+6]) << 48) |
		(uint64(bus.memory[addr+7]) << 56)
	if int64(got) != 123456789 {
		t.Fatalf("FISTP m64 got=%d want=123456789", int64(got))
	}
	if cpu.FPU.FTW != 0xFFFF {
		t.Fatalf("FISTP should pop stack")
	}
}

func TestX87_FNSAVE_FRSTOR_PhysicalOrderWithNonZeroTOP(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	buf := uint32(0x900)

	// Build non-zero TOP state with distinct values in multiple physical regs.
	cpu.FPU.push(11)
	cpu.FPU.push(22)
	cpu.FPU.push(33)
	cpu.FPU.push(44)
	origST0 := cpu.FPU.ST(0)
	origST1 := cpu.FPU.ST(1)
	origST2 := cpu.FPU.ST(2)
	origTop := cpu.FPU.top()
	origFTW := cpu.FPU.FTW

	writeCode(bus, 0,
		0xDD, 0x35, byte(buf), byte(buf>>8), byte(buf>>16), byte(buf>>24), // FNSAVE
		0xD9, 0xE8, // FLD1 (clobber)
		0xDD, 0x25, byte(buf), byte(buf>>8), byte(buf>>16), byte(buf>>24), // FRSTOR
	)
	cpu.Step()
	cpu.Step()
	cpu.Step()

	if cpu.FPU.top() != origTop || cpu.FPU.FTW != origFTW {
		t.Fatalf("FRSTOR did not restore TOP/FTW top=%d/%d ftw=%04X/%04X", cpu.FPU.top(), origTop, cpu.FPU.FTW, origFTW)
	}
	if !almostEq(cpu.FPU.ST(0), origST0) || !almostEq(cpu.FPU.ST(1), origST1) || !almostEq(cpu.FPU.ST(2), origST2) {
		t.Fatalf("FRSTOR values mismatch ST0=%v ST1=%v ST2=%v", cpu.FPU.ST(0), cpu.FPU.ST(1), cpu.FPU.ST(2))
	}
}

func TestX87_ArithmeticSpecialValues(t *testing.T) {
	bus := NewTestX86Bus()

	// Inf + (-Inf) => NaN + IE
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(math.Inf(-1))
	cpu.FPU.push(math.Inf(1))
	writeCode(bus, 0, 0xD8, 0xC1) // FADD ST0,ST1
	cpu.Step()
	if !math.IsNaN(cpu.FPU.ST(0)) || cpu.FPU.FSW&x87FSW_IE == 0 {
		t.Fatalf("Inf+(-Inf) should be NaN + IE, ST0=%v FSW=0x%04X", cpu.FPU.ST(0), cpu.FPU.FSW)
	}

	// 1/0 => Inf + ZE
	cpu = NewCPU_X86(bus)
	cpu.FPU.push(0.0)
	cpu.FPU.push(1.0)
	writeCode(bus, 0, 0xD8, 0xF1) // FDIV ST0,ST1 => 1/0
	cpu.Step()
	if !math.IsInf(cpu.FPU.ST(0), 1) || cpu.FPU.FSW&x87FSW_ZE == 0 {
		t.Fatalf("1/0 should set ZE and yield +Inf, ST0=%v FSW=0x%04X", cpu.FPU.ST(0), cpu.FPU.FSW)
	}

	// Inf * 0 => NaN + IE
	cpu = NewCPU_X86(bus)
	cpu.FPU.push(0.0)
	cpu.FPU.push(math.Inf(1))
	writeCode(bus, 0, 0xD8, 0xC9) // FMUL ST0,ST1
	cpu.Step()
	if !math.IsNaN(cpu.FPU.ST(0)) || cpu.FPU.FSW&x87FSW_IE == 0 {
		t.Fatalf("Inf*0 should be NaN + IE, ST0=%v FSW=0x%04X", cpu.FPU.ST(0), cpu.FPU.FSW)
	}

	// 0/0 => NaN + IE + ZE
	cpu = NewCPU_X86(bus)
	cpu.FPU.push(0.0)
	cpu.FPU.push(0.0)
	writeCode(bus, 0, 0xD8, 0xF1) // FDIV ST0,ST1
	cpu.Step()
	if !math.IsNaN(cpu.FPU.ST(0)) || cpu.FPU.FSW&x87FSW_IE == 0 || cpu.FPU.FSW&x87FSW_ZE == 0 {
		t.Fatalf("0/0 should set IE+ZE and yield NaN, ST0=%v FSW=0x%04X", cpu.FPU.ST(0), cpu.FPU.FSW)
	}
}

func TestX87_FCOM_FCOMP_MemoryForms(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	m32 := uint32(0x980)
	m64 := uint32(0x990)
	write32f(bus, m32, 2.0)
	write64f(bus, m64, 3.0)

	cpu.FPU.push(2.0)
	writeCode(bus, 0,
		0xD8, 0x15, byte(m32), byte(m32>>8), byte(m32>>16), byte(m32>>24), // FCOM m32 (equal)
		0xDC, 0x1D, byte(m64), byte(m64>>8), byte(m64>>16), byte(m64>>24), // FCOMP m64 (2<3, pop)
	)
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_C3 == 0 {
		t.Fatalf("FCOM m32 equal should set C3")
	}
	ftwBefore := cpu.FPU.FTW
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_C0 == 0 {
		t.Fatalf("FCOMP m64 less should set C0")
	}
	if cpu.FPU.FTW == ftwBefore {
		t.Fatalf("FCOMP m64 should pop")
	}
}

func TestX87_FCOMP_ST_PopBehavior(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(5.0)
	cpu.FPU.push(5.0)
	writeCode(bus, 0, 0xD8, 0xD1) // FCOM ST1 (reg=/2)
	cpu.Step()
	if cpu.FPU.FTW == 0xFFFF {
		t.Fatalf("FCOM should not pop")
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(5.0)
	cpu.FPU.push(5.0)
	topBefore := cpu.FPU.top()
	writeCode(bus, 0, 0xD8, 0xD9) // FCOMP ST1 (reg=/3)
	cpu.Step()
	if cpu.FPU.FTW == 0xFFFF {
		t.Fatalf("FCOMP ST1 pops one entry, stack should not be empty")
	}
	if cpu.FPU.top() != ((topBefore + 1) & 7) {
		t.Fatalf("FCOMP ST1 should pop ST0 and increment TOP")
	}
}

func TestX87_RemainingUnaryAndStackOps(t *testing.T) {
	bus := NewTestX86Bus()

	cpu := NewCPU_X86(bus)
	cpu.FPU.push(math.Pi)
	writeCode(bus, 0, 0xD9, 0xFF) // FCOS
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), math.Cos(math.Pi)) {
		t.Fatalf("FCOS mismatch got=%v", cpu.FPU.ST(0))
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(1.0)             // y in ST1 after push x
	cpu.FPU.push(1.0)             // x in ST0
	writeCode(bus, 0, 0xD9, 0xF3) // FPATAN => atan2(y,x)=pi/4, pop
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), math.Pi/4) {
		t.Fatalf("FPATAN mismatch got=%v want=%v", cpu.FPU.ST(0), math.Pi/4)
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(-1.0)
	writeCode(bus, 0, 0xD9, 0xE4) // FTST
	cpu.Step()
	if cpu.FPU.FSW&x87FSW_C0 == 0 {
		t.Fatalf("FTST with negative value should set C0")
	}

	cpu = NewCPU_X86(bus)
	cpu.FPU.push(2.0)
	cpu.FPU.push(1.0)             // ST0=1 ST1=2
	writeCode(bus, 0, 0xD9, 0xC9) // FXCH ST1
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 2.0) || !almostEq(cpu.FPU.ST(1), 1.0) {
		t.Fatalf("FXCH ST1 mismatch ST0=%v ST1=%v", cpu.FPU.ST(0), cpu.FPU.ST(1))
	}
}
