// jit_dload_dstore_helper_amd64_test.go — Phase 5 cycle 5.7 AMD64 DLOAD/DSTORE helper-exit tests.
//
// DLOAD/DSTORE helper-exit tests pin the remaining slow paths: high addresses
// and MMU-on accesses exit to the Go dispatcher, while low dense RAM stays
// native.

//go:build amd64 && linux

package main

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestJIT_AMD64_DLOAD_HighAddr_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	const highAddr uint64 = 0x0000_0001_0000_8000
	r.cpu.regs[2] = highAddr
	r.cpu.regs[31] = 0xC0DEFEED
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_DLOAD, 4, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedHelper != HELPER_DLOAD {
		t.Fatalf("NeedHelper = %d, want HELPER_DLOAD (%d)", r.ctx.NeedHelper, HELPER_DLOAD)
	}
	if r.ctx.NeedIOFallback != 0 {
		t.Fatalf("NeedIOFallback = %d, want 0", r.ctx.NeedIOFallback)
	}
	if r.ctx.HelperAddr != highAddr {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X", r.ctx.HelperAddr, highAddr)
	}
	if r.ctx.HelperSize != uint32(IE64_SIZE_Q) {
		t.Fatalf("HelperSize = %d, want IE64_SIZE_Q", r.ctx.HelperSize)
	}
	if r.ctx.HelperRd != 4 {
		t.Fatalf("HelperRd = %d, want 4", r.ctx.HelperRd)
	}
	if r.ctx.HelperPC != PROG_START {
		t.Fatalf("HelperPC = 0x%016X, want 0x%016X", r.ctx.HelperPC, PROG_START)
	}
	if r.ctx.LiveSP != 0xC0DEFEED {
		t.Fatalf("LiveSP = 0x%016X, want 0xC0DEFEED", r.ctx.LiveSP)
	}
}

func TestJIT_AMD64_DLOAD_LowAddr_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	const addr uint32 = 0x4000
	const want = -2.5
	binary.LittleEndian.PutUint64(r.cpu.memory[addr:], math.Float64bits(want))
	r.cpu.regs[2] = uint64(addr)
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_DLOAD, 4, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedHelper != HELPER_NONE {
		t.Fatalf("NeedHelper = %d, want HELPER_NONE (low DLOAD should be native)", r.ctx.NeedHelper)
	}
	if got := r.cpu.FPU.getDPair(4); got != want {
		t.Fatalf("D4 = %v, want %v", got, want)
	}
}

func TestJIT_AMD64_DSTORE_HighAddr_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	const highAddr uint64 = 0x0000_0001_0000_8000
	r.cpu.regs[2] = highAddr
	r.cpu.regs[31] = 0xBEEFFEED
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_DSTORE, 6, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedHelper != HELPER_DSTORE {
		t.Fatalf("NeedHelper = %d, want HELPER_DSTORE (%d)", r.ctx.NeedHelper, HELPER_DSTORE)
	}
	if r.ctx.HelperAddr != highAddr {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X", r.ctx.HelperAddr, highAddr)
	}
	if r.ctx.HelperSize != uint32(IE64_SIZE_Q) {
		t.Fatalf("HelperSize = %d, want IE64_SIZE_Q", r.ctx.HelperSize)
	}
	if r.ctx.HelperRd != 6 {
		t.Fatalf("HelperRd = %d, want 6", r.ctx.HelperRd)
	}
	if r.ctx.LiveSP != 0xBEEFFEED {
		t.Fatalf("LiveSP = 0x%016X, want 0xBEEFFEED", r.ctx.LiveSP)
	}
}

func TestJIT_AMD64_DSTORE_LowAddr_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	const addr uint32 = 0x4000
	const want = 6.25
	r.cpu.FPU.setDPair(6, want)
	r.cpu.regs[2] = uint64(addr)
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_DSTORE, 6, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedHelper != HELPER_NONE {
		t.Fatalf("NeedHelper = %d, want HELPER_NONE (low DSTORE should be native)", r.ctx.NeedHelper)
	}
	if gotBits := binary.LittleEndian.Uint64(r.cpu.memory[addr:]); gotBits != math.Float64bits(want) {
		t.Fatalf("mem[0x%X] bits = 0x%016X, want 0x%016X", addr, gotBits, math.Float64bits(want))
	}
}

func TestJIT_AMD64_DLOAD_MMUEnabled_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	r.cpu.regs[2] = 0x4000
	r.ctx.NeedHelper = HELPER_NONE
	r.ctx.MMUEnabled = 1
	defer func() { r.ctx.MMUEnabled = 0 }()

	r.compileAndRun(t, ie64Instr(OP_DLOAD, 4, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedHelper != HELPER_DLOAD {
		t.Fatalf("NeedHelper = %d, want HELPER_DLOAD", r.ctx.NeedHelper)
	}
}

// End-to-end: DLOAD reads an FP64 value from high backing into a D-pair
// register; DSTORE writes a D-pair register back to high backing. Both go
// through the helper dispatcher with full ExecuteJIT.
func TestJIT_AMD64_DLOAD_HighBacking_EndToEnd(t *testing.T) {
	const highAddr uint64 = 0x0000_0001_0000_8000
	const want float64 = 3.14159265358979

	cpu, _ := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			bits := math.Float64bits(want)
			for i := uint64(0); i < 8; i++ {
				cpu.bus.backing.Write8(highAddr+i, byte(bits>>(8*i)))
			}
			cpu.regs[2] = highAddr
		},
		ie64Instr(OP_DLOAD, 4, IE64_SIZE_Q, 0, 2, 0, 0),
	)

	if cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	got := cpu.FPU.getDPair(4)
	if got != want {
		t.Fatalf("D4 = %v, want %v", got, want)
	}
}

func TestJIT_AMD64_DSTORE_HighBacking_EndToEnd(t *testing.T) {
	const highAddr uint64 = 0x0000_0001_0000_8000
	const want float64 = 2.718281828459045

	cpu, backing := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			if cpu.FPU != nil {
				cpu.FPU.setDPair(6, want)
			}
			cpu.regs[2] = highAddr
		},
		ie64Instr(OP_DSTORE, 6, IE64_SIZE_Q, 0, 2, 0, 0),
	)

	if cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	var bits uint64
	for i := uint64(0); i < 8; i++ {
		bits |= uint64(backing.Read8(highAddr+i)) << (8 * i)
	}
	if got := math.Float64frombits(bits); got != want {
		t.Fatalf("backing[0x%016X] = %v, want %v", highAddr, got, want)
	}
}
