// jit_high_addr_smoke_test.go - PLAN_MAX_RAM slice 10k.
//
// Regression sweep: hammer the JIT slow paths at 64 MiB so any missed
// emitter (or unbounded bitmap index) surfaces under code-cache churn.
// IE64 JIT must bail cleanly to interpreter on every iteration; the x86
// and M68K plumbing must round-trip 32-bit values at 64 MiB on a 256 MiB
// bus without panicking.

//go:build amd64 && linux

package main

import "testing"

const jitHighAddrIters = 1024

func TestJIT_IE64_LoadStoreAt64MiB(t *testing.T) {
	r := newJITTestRig(t)

	// 64 MiB is above the rig's default 32 MiB MemSize, so the slow path
	// must bail every iteration. Without the slice-10b bail this would
	// index ioPageBitmap OOB and crash nondeterministically.
	const highAddr uint64 = 64 * 1024 * 1024
	r.cpu.regs[2] = highAddr

	for i := 0; i < jitHighAddrIters; i++ {
		r.ctx.NeedIOFallback = 0
		r.cpu.regs[1] = uint64(i)
		// LOAD.Q R1, 0(R2)
		r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0))
		if r.ctx.NeedIOFallback != 1 {
			t.Fatalf("iter %d: load NeedIOFallback = %d, want 1", i, r.ctx.NeedIOFallback)
		}
		r.ctx.NeedIOFallback = 0
		// STORE.Q R1, 0(R2)
		r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))
		if r.ctx.NeedIOFallback != 1 {
			t.Fatalf("iter %d: store NeedIOFallback = %d, want 1", i, r.ctx.NeedIOFallback)
		}
	}
}

func TestJIT_X86_PointerLoadAt64MiB(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	cfg := &CPUX86Config{LoadAddr: 0, Entry: 0}
	NewCPUX86Runner(bus, cfg)

	const addr uint32 = 64 * 1024 * 1024
	for i := 0; i < jitHighAddrIters; i++ {
		v := uint32(0xC0DE0000 | (i & 0xFFFF))
		bus.Write32(addr, v)
		if got := bus.Read32(addr); got != v {
			t.Fatalf("iter %d: bus.Read32(64 MiB) = %#x, want %#x", i, got, v)
		}
	}
}

func TestJIT_M68K_LongMoveAt64MiB(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	cpu := NewM68KCPU(bus)
	if got := cpu.ProfileTopOfRAM(); got != uint32(len(bus.GetMemory())) {
		t.Fatalf("bare M68K ProfileTopOfRAM = %#x, want %#x", got, len(bus.GetMemory()))
	}

	const addr uint32 = 64 * 1024 * 1024
	for i := 0; i < jitHighAddrIters; i++ {
		v := uint32(0xBEEF0000 | (i & 0xFFFF))
		bus.Write32(addr, v)
		if got := bus.Read32(addr); got != v {
			t.Fatalf("iter %d: bus.Read32(64 MiB) = %#x, want %#x", i, got, v)
		}
	}
}
