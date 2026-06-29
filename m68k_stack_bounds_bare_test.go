package main

import "testing"

// A bare .ie68 program builds its own stack at runtime (move/lea into A7) and
// has no initial SSP in the reset vector. The legacy behaviour pinned the
// stack-corruption window to the load-time SP (M68K_STACK_START + 64 KiB =
// 0x01000000), so a guest that later relocated its stack high faulted on every
// RTS/Pop (underflow) and on deep pushes (overflow). Reset now spans the full
// guest RAM for bare loads, matching what the AROS/EmuTOS loaders already do.

func TestM68K_Reset_BareLoad_StackBoundsSpanFullRAM(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)

	cpu.Write32(0, 0) // bare image: no initial SSP in the reset vector
	cpu.Reset()

	top := cpu.ProfileTopOfRAM()
	if cpu.stackUpperBound != top {
		t.Fatalf("stackUpperBound = 0x%08X, want full RAM top 0x%08X", cpu.stackUpperBound, top)
	}
	if cpu.stackLowerBound != 0 {
		t.Fatalf("stackLowerBound = 0x%08X, want 0", cpu.stackLowerBound)
	}
	// Guard against the legacy load-time window pin re-appearing.
	if cpu.stackUpperBound == 0x01000000 && top != 0x01000000 {
		t.Fatalf("stackUpperBound still pinned to load-time window 0x01000000")
	}
}

// The real bare-M68K launch path (program_executor.go EXEC_TYPE_M68K) creates a
// CPU and calls LoadProgramBytes WITHOUT Reset(), then runs. The stack bounds
// must span full RAM on this path too, not just when Reset() is called.
func TestM68K_BareLoad_ViaLoadProgramBytes_StackBoundsSpanFullRAM(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)

	cpu.LoadProgramBytes([]byte{0x4E, 0x75}) // RTS — minimal bare program, no Reset()

	top := cpu.ProfileTopOfRAM()
	if cpu.stackUpperBound != top {
		t.Fatalf("stackUpperBound = 0x%08X, want full RAM top 0x%08X", cpu.stackUpperBound, top)
	}
	if cpu.stackLowerBound != 0 {
		t.Fatalf("stackLowerBound = 0x%08X, want 0", cpu.stackLowerBound)
	}
}

func TestM68K_BareLoad_ViaLoadProgramBytes_HighStackPopNoUnderflow(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)

	cpu.LoadProgramBytes([]byte{0x4E, 0x75}) // no Reset()

	top := cpu.ProfileTopOfRAM()
	if top <= 0x01000000+0x2000 {
		t.Skipf("bus RAM 0x%08X too small to exercise a high stack", top)
	}
	sp := (top - 0x10) &^ 1
	cpu.AddrRegs[7] = sp
	cpu.Write32(sp, 0xCAFEF00D)
	if got := cpu.Pop32(); got != 0xCAFEF00D {
		t.Fatalf("Pop32 at high SP 0x%08X returned 0x%08X (underflow fault?), want 0xCAFEF00D", sp, got)
	}
}

func TestM68K_BareLoad_HighRuntimeStack_PopDoesNotUnderflow(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)

	cpu.Write32(0, 0)
	cpu.Reset()

	top := cpu.ProfileTopOfRAM()
	// A stack placed above the legacy 0x01000000 window but inside real RAM.
	if top <= 0x01000000+0x2000 {
		t.Skipf("bus RAM 0x%08X too small to exercise a high stack", top)
	}
	sp := (top - 0x10) &^ 1
	cpu.AddrRegs[7] = sp
	cpu.Write32(sp, 0xCAFEF00D)

	got := cpu.Pop32()
	if got != 0xCAFEF00D {
		t.Fatalf("Pop32 at high SP 0x%08X returned 0x%08X (underflow fault?), want 0xCAFEF00D", sp, got)
	}
	if cpu.AddrRegs[7] != sp+4 {
		t.Fatalf("SP after Pop32 = 0x%08X, want 0x%08X (pop did not advance — faulted)", cpu.AddrRegs[7], sp+4)
	}
}

func TestM68K_BareLoad_HighRuntimeStack_DeepPushDoesNotOverflow(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)

	cpu.Write32(0, 0)
	cpu.Reset()

	top := cpu.ProfileTopOfRAM()
	if top <= 0x01000000+0x40000 {
		t.Skipf("bus RAM 0x%08X too small to exercise a deep high stack", top)
	}
	// Start just above the legacy window upper and push more than the old
	// 64 KiB window depth, which the legacy lower bound would have rejected.
	sp := uint32(0x01040000)
	cpu.AddrRegs[7] = sp
	for i := 0; i < 0x20000/4; i++ { // push 128 KiB of longs
		cpu.Push32(0x11220000 | uint32(i))
	}
	wantSP := sp - 0x20000
	if cpu.AddrRegs[7] != wantSP {
		t.Fatalf("SP after deep push = 0x%08X, want 0x%08X (push faulted on lower bound)", cpu.AddrRegs[7], wantSP)
	}
	if got := cpu.Read32(wantSP); got != (0x11220000 | uint32(0x20000/4-1)) {
		t.Fatalf("top-of-stack long = 0x%08X, want 0x%08X", got, 0x11220000|uint32(0x20000/4-1))
	}
}
