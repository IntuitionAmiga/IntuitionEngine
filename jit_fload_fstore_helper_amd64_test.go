// jit_fload_fstore_helper_amd64_test.go — Phase 5 cycle 5.6 AMD64 FLOAD/FSTORE helper-exit tests.

//go:build amd64 && linux

package main

import (
	"encoding/binary"
	"testing"
)

func TestJIT_AMD64_FLOAD_HighAddr_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	const highAddr uint64 = 0x0000_0001_0000_8000
	r.cpu.regs[2] = highAddr
	r.cpu.regs[31] = 0xC0DEFEED
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_FLOAD, 5, IE64_SIZE_L, 0, 2, 0, 0))

	if r.ctx.NeedHelper != HELPER_FLOAD {
		t.Fatalf("NeedHelper = %d, want HELPER_FLOAD (%d)", r.ctx.NeedHelper, HELPER_FLOAD)
	}
	if r.ctx.NeedIOFallback != 0 {
		t.Fatalf("NeedIOFallback = %d, want 0", r.ctx.NeedIOFallback)
	}
	if r.ctx.HelperAddr != highAddr {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X", r.ctx.HelperAddr, highAddr)
	}
	if r.ctx.HelperSize != uint32(IE64_SIZE_L) {
		t.Fatalf("HelperSize = %d, want IE64_SIZE_L", r.ctx.HelperSize)
	}
	if r.ctx.HelperRd != 5 {
		t.Fatalf("HelperRd = %d, want 5", r.ctx.HelperRd)
	}
	if r.ctx.HelperPC != PROG_START {
		t.Fatalf("HelperPC = 0x%016X, want 0x%016X", r.ctx.HelperPC, PROG_START)
	}
	if r.ctx.LiveSP != 0xC0DEFEED {
		t.Fatalf("LiveSP = 0x%016X, want 0xC0DEFEED", r.ctx.LiveSP)
	}
}

func TestJIT_AMD64_FLOAD_MMUEnabled_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	r.cpu.regs[2] = 0x4000
	r.ctx.NeedHelper = HELPER_NONE
	r.ctx.MMUEnabled = 1
	defer func() { r.ctx.MMUEnabled = 0 }()

	r.compileAndRun(t, ie64Instr(OP_FLOAD, 2, IE64_SIZE_L, 0, 2, 0, 0))

	if r.ctx.NeedHelper != HELPER_FLOAD {
		t.Fatalf("NeedHelper = %d, want HELPER_FLOAD", r.ctx.NeedHelper)
	}
}

func TestJIT_AMD64_FLOAD_LowAddr_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	const addr uint32 = 0x4000
	const want uint32 = 0x4048F5C3
	binary.LittleEndian.PutUint32(r.cpu.memory[addr:], want)
	r.cpu.regs[2] = uint64(addr)
	r.ctx.NeedHelper = 0xDEADBEEF

	r.compileAndRun(t, ie64Instr(OP_FLOAD, 0, IE64_SIZE_L, 0, 2, 0, 0))

	if r.cpu.FPU.FPRegs[0] != want {
		t.Fatalf("F0 = 0x%08X, want 0x%08X", r.cpu.FPU.FPRegs[0], want)
	}
	if r.ctx.NeedHelper != 0xDEADBEEF {
		t.Fatalf("NeedHelper = %d, want untouched poison", r.ctx.NeedHelper)
	}
}

func TestJIT_AMD64_FSTORE_HighAddr_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	const highAddr uint64 = 0x0000_0001_0000_8000
	r.cpu.FPU.FPRegs[3] = 0xC0FFEE01
	r.cpu.regs[2] = highAddr
	r.cpu.regs[31] = 0xBEEFFEED
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_FSTORE, 3, IE64_SIZE_L, 0, 2, 0, 0))

	if r.ctx.NeedHelper != HELPER_FSTORE {
		t.Fatalf("NeedHelper = %d, want HELPER_FSTORE (%d)", r.ctx.NeedHelper, HELPER_FSTORE)
	}
	if r.ctx.HelperAddr != highAddr {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X", r.ctx.HelperAddr, highAddr)
	}
	if r.ctx.HelperRd != 3 {
		t.Fatalf("HelperRd = %d, want 3", r.ctx.HelperRd)
	}
	if r.ctx.LiveSP != 0xBEEFFEED {
		t.Fatalf("LiveSP = 0x%016X, want 0xBEEFFEED", r.ctx.LiveSP)
	}
}

func TestJIT_AMD64_FSTORE_LowAddr_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	const addr uint32 = 0x4000
	const payload uint32 = 0xCAFEBEEF
	r.cpu.FPU.FPRegs[0] = payload
	r.cpu.regs[2] = uint64(addr)
	r.ctx.NeedHelper = 0xDEADBEEF

	r.compileAndRun(t, ie64Instr(OP_FSTORE, 0, IE64_SIZE_L, 0, 2, 0, 0))

	got := binary.LittleEndian.Uint32(r.cpu.memory[addr:])
	if got != payload {
		t.Fatalf("memory[0x%X] = 0x%08X, want 0x%08X", addr, got, payload)
	}
	if r.ctx.NeedHelper != 0xDEADBEEF {
		t.Fatalf("NeedHelper = %d, want untouched poison", r.ctx.NeedHelper)
	}
}
