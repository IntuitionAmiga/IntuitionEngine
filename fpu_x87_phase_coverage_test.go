package main

import (
	"math"
	"testing"
)

func write64f(bus *TestX86Bus, addr uint32, v float64) {
	bits := math.Float64bits(v)
	for i := range 8 {
		bus.memory[addr+uint32(i)] = byte(bits >> (8 * i))
	}
}

func write32f(bus *TestX86Bus, addr uint32, v float32) {
	bits := math.Float32bits(v)
	for i := range 4 {
		bus.memory[addr+uint32(i)] = byte(bits >> (8 * i))
	}
}

func read32u(bus *TestX86Bus, addr uint32) uint32 {
	return uint32(bus.memory[addr]) |
		(uint32(bus.memory[addr+1]) << 8) |
		(uint32(bus.memory[addr+2]) << 16) |
		(uint32(bus.memory[addr+3]) << 24)
}

func TestX87_FSTP_STi_RegisterForm(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(1.0)
	cpu.FPU.push(2.0)
	writeCode(bus, 0, 0xDD, 0xD9) // FSTP ST(1)
	cpu.Step()
	if !almostEq(cpu.FPU.ST(0), 2.0) {
		t.Fatalf("ST0=%v want 2.0", cpu.FPU.ST(0))
	}
}

func TestX87_FXAM_EmptyClassBits(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	writeCode(bus, 0, 0xD9, 0xE5) // FXAM
	cpu.Step()
	c0 := (cpu.FPU.FSW & x87FSW_C0) != 0
	c2 := (cpu.FPU.FSW & x87FSW_C2) != 0
	c3 := (cpu.FPU.FSW & x87FSW_C3) != 0
	if !(c3 && !c2 && c0) {
		t.Fatalf("empty FXAM class mismatch C3=%v C2=%v C0=%v", c3, c2, c0)
	}
}

func TestX87_FNSTSW_SAHF_EqualLessGreaterNaN(t *testing.T) {
	mk := func(a, b float64) *CPU_X86 {
		bus := NewTestX86Bus()
		cpu := NewCPU_X86(bus)
		cpu.FPU.push(b)
		cpu.FPU.push(a)
		writeCode(bus, 0,
			0xDE, 0xD9, // FCOMPP
			0xDF, 0xE0, // FNSTSW AX
			0x9E, // SAHF
		)
		cpu.Step()
		cpu.Step()
		cpu.Step()
		return cpu
	}

	cpu := mk(1, 1)
	if !cpu.ZF() || cpu.CF() {
		t.Fatalf("equal: ZF=%v CF=%v", cpu.ZF(), cpu.CF())
	}

	cpu = mk(1, 2)
	if !cpu.CF() || cpu.ZF() {
		t.Fatalf("less: ZF=%v CF=%v", cpu.ZF(), cpu.CF())
	}

	cpu = mk(2, 1)
	if cpu.CF() || cpu.ZF() {
		t.Fatalf("greater: ZF=%v CF=%v", cpu.ZF(), cpu.CF())
	}

	cpu = mk(math.NaN(), 1)
	if !(cpu.CF() && cpu.ZF() && cpu.PF()) {
		t.Fatalf("nan unordered: CF=%v ZF=%v PF=%v", cpu.CF(), cpu.ZF(), cpu.PF())
	}
}

func TestX87_FDS_RegForm_NoUpdate(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.DS = 0x44
	mem := uint32(0x180)
	env := uint32(0x1C0)
	write32f(bus, mem, 2)
	cpu.FPU.push(1)
	cpu.FPU.push(3)

	writeCode(bus, 0,
		0xD8, 0x05, byte(mem), byte(mem>>8), byte(mem>>16), byte(mem>>24), // FADD m32 (captures FDP/FDS)
		0xD8, 0xC1, // FADD ST0,ST1 (must not overwrite FDP/FDS)
		0xD9, 0x35, byte(env), byte(env>>8), byte(env>>16), byte(env>>24), // FNSTENV
	)
	cpu.Step()
	cpu.Step()
	cpu.Step()
	if got := read32u(bus, env+20); got != mem {
		t.Fatalf("FDP changed by reg-form op: got=0x%08X want=0x%08X", got, mem)
	}
	if got := read16le(bus, env+24); got != cpu.DS {
		t.Fatalf("FDS changed by reg-form op: got=0x%04X want=0x%04X", got, cpu.DS)
	}
}

func TestX87_FDS_16bitAddr_BP_DefaultSS(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.SS = 0x88
	cpu.SetBP(0x0120)
	mem := uint32(0x0128)
	env := uint32(0x220)
	write32f(bus, mem, 1)
	cpu.FPU.push(1)

	writeCode(bus, 0,
		0x67, 0xD8, 0x46, 0x08, // FADD m32 [BP+8] using 16-bit addressing
		0xD9, 0x35, byte(env), byte(env>>8), byte(env>>16), byte(env>>24),
	)
	cpu.Step()
	cpu.Step()
	if got := read16le(bus, env+24); got != cpu.SS {
		t.Fatalf("FDS for 16-bit BP mode got=0x%04X want=0x%04X", got, cpu.SS)
	}
}

func TestX87_Integration_AddDoubles(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	a := uint32(0x300)
	b := uint32(0x320)
	out := uint32(0x340)
	write64f(bus, a, 1.25)
	write64f(bus, b, 2.75)

	writeCode(bus, 0,
		0xDD, 0x05, byte(a), byte(a>>8), byte(a>>16), byte(a>>24), // FLD qword [a]
		0xDC, 0x05, byte(b), byte(b>>8), byte(b>>16), byte(b>>24), // FADD qword [b]
		0xDD, 0x1D, byte(out), byte(out>>8), byte(out>>16), byte(out>>24), // FSTP qword [out]
	)
	cpu.Step()
	cpu.Step()
	cpu.Step()
	got := math.Float64frombits(uint64(read32u(bus, out)) | (uint64(read32u(bus, out+4)) << 32))
	if !almostEq(got, 4.0) {
		t.Fatalf("sum got=%v want=4", got)
	}
}

func TestX87_Integration_IntToFloat_And_FloatToInt(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	i := uint32(0x380)
	f := uint32(0x390)
	o := uint32(0x3A0)
	write32le(bus, i, 42)
	write64f(bus, f, 12.75)

	writeCode(bus, 0,
		0xDB, 0x05, byte(i), byte(i>>8), byte(i>>16), byte(i>>24), // FILD dword [i]
		0xDD, 0x1D, byte(f), byte(f>>8), byte(f>>16), byte(f>>24), // FSTP qword [f]
		0xDD, 0x05, byte(f), byte(f>>8), byte(f>>16), byte(f>>24), // FLD qword [f]
		0xDB, 0x1D, byte(o), byte(o>>8), byte(o>>16), byte(o>>24), // FISTP dword [o]
	)
	for range 4 {
		cpu.Step()
	}
	if got := int32(read32u(bus, o)); got != 42 {
		t.Fatalf("roundtrip int got=%d want=42", got)
	}
}

func TestX87_Integration_CompareAndBranch(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	a := uint32(0x3C0)
	b := uint32(0x3C8)
	write64f(bus, a, 1.0)
	write64f(bus, b, 2.0)

	writeCode(bus, 0,
		0xDD, 0x05, byte(a), byte(a>>8), byte(a>>16), byte(a>>24), // FLD [a]
		0xDD, 0x05, byte(b), byte(b>>8), byte(b>>16), byte(b>>24), // FLD [b]
		0xDE, 0xD9, // FCOMPP
		0xDF, 0xE0, // FNSTSW AX
		0x9E,       // SAHF
		0x77, 0x05, // JA +5
		0xB8, 0x11, 0x11, 0x00, 0x00, // MOV EAX,0x1111
		0xB8, 0x22, 0x22, 0x00, 0x00, // MOV EAX,0x2222
	)
	for range 8 {
		cpu.Step()
	}
	if cpu.EAX != 0x00002222 {
		t.Fatalf("JA path failed, EAX=0x%08X", cpu.EAX)
	}
}

func TestX87_ControlOps_DecIncFreeNop(t *testing.T) {
	bus := NewTestX86Bus()
	cpu := NewCPU_X86(bus)
	cpu.FPU.push(1)
	cpu.FPU.push(2)
	startTop := cpu.FPU.top()

	writeCode(bus, 0,
		0xD9, 0xF6, // FDECSTP
		0xD9, 0xF7, // FINCSTP
		0xDD, 0xC1, // FFREE ST(1)
		0xD9, 0xD0, // FNOP
	)
	cpu.Step()
	if cpu.FPU.top() != ((startTop - 1) & 7) {
		t.Fatalf("FDECSTP top=%d", cpu.FPU.top())
	}
	cpu.Step()
	if cpu.FPU.top() != startTop {
		t.Fatalf("FINCSTP top=%d want %d", cpu.FPU.top(), startTop)
	}
	cpu.Step()
	if cpu.FPU.getTag(cpu.FPU.physReg(1)) != x87TagEmpty {
		t.Fatalf("FFREE did not clear tag")
	}
	before := cpu.FPU.FSW
	cpu.Step()
	if cpu.FPU.FSW != before {
		t.Fatalf("FNOP should not mutate FSW")
	}
}
