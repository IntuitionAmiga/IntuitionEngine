// jit_load_helper_arm64_test.go — Phase 5 cycle 5.4: ARM64 LOAD helper-exit.
//
// Mirror of jit_load_helper_amd64_test.go for the ARM64 emitter. Pins the
// HELPER_LOAD protocol fields on MMU-on / high-addr / I/O-page bails and
// proves the dispatcher round-trips a high-address load through
// SparseBacking end-to-end.

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"testing"
)

func assertLOADHelperFields(t *testing.T, ctx *JITContext, wantAddr uint64, wantSize uint32, wantRd uint32, wantPC uint64, wantSP uint64) {
	t.Helper()
	if ctx.NeedHelper != HELPER_LOAD {
		t.Fatalf("NeedHelper = %d, want HELPER_LOAD (%d)", ctx.NeedHelper, HELPER_LOAD)
	}
	if ctx.NeedIOFallback != 0 {
		t.Fatalf("NeedIOFallback = %d, want 0 (helper path must not set IO fallback)", ctx.NeedIOFallback)
	}
	if ctx.HelperAddr != wantAddr {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X", ctx.HelperAddr, wantAddr)
	}
	if ctx.HelperSize != wantSize {
		t.Fatalf("HelperSize = %d, want %d", ctx.HelperSize, wantSize)
	}
	if ctx.HelperRd != wantRd {
		t.Fatalf("HelperRd = %d, want %d", ctx.HelperRd, wantRd)
	}
	if ctx.HelperPC != wantPC {
		t.Fatalf("HelperPC = 0x%016X, want 0x%016X", ctx.HelperPC, wantPC)
	}
	if ctx.LiveSP != wantSP {
		t.Fatalf("LiveSP = 0x%016X, want 0x%016X", ctx.LiveSP, wantSP)
	}
}

func TestJIT_ARM64_LOAD_HighAddr_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const highAddr uint64 = 0x0000_0001_0000_8000
	const sentinelSP uint64 = 0xCAFE_BABE_F00D_F00D
	r.cpu.regs[2] = highAddr
	r.cpu.regs[31] = sentinelSP
	r.ctx.NeedIOFallback = 0
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	assertLOADHelperFields(t, r.ctx, highAddr, uint32(IE64_SIZE_Q), 1, PROG_START, sentinelSP)
}

func TestJIT_ARM64_LOAD_MMUEnabled_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const lowAddr uint64 = 0x4000
	r.cpu.regs[2] = lowAddr
	r.cpu.regs[31] = 0x1000
	r.ctx.NeedIOFallback = 0
	r.ctx.NeedHelper = HELPER_NONE
	r.ctx.MMUEnabled = 1
	defer func() { r.ctx.MMUEnabled = 0 }()

	r.compileAndRun(t, ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 0, 2, 0, 0))

	assertLOADHelperFields(t, r.ctx, lowAddr, uint32(IE64_SIZE_L), 3, PROG_START, 0x1000)
}

func TestJIT_ARM64_LOAD_LowAddr_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	const want uint64 = 0xDEADBEEFCAFEBABE
	const addr uint32 = 0x4000
	binary.LittleEndian.PutUint64(r.cpu.memory[addr:], want)
	r.cpu.regs[2] = uint64(addr)
	r.ctx.NeedHelper = 0xDEADBEEF

	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%016X, want 0x%016X", r.cpu.regs[1], want)
	}
	if r.ctx.NeedHelper != 0xDEADBEEF {
		t.Fatalf("NeedHelper = %d, want untouched poison (fast path must not write helper)", r.ctx.NeedHelper)
	}
}

// TestJIT_ARM64_LOAD_HelperExit_FutureSPWriter_PreservesSP — covers the
// case where the block has a later MOVEQ to R31 (writes SP, no read) and
// the bailing LOAD comes before it. br.used&(1<<31)!=0 because the
// writer sets the written bit, but br.read&(1<<31)==0 so the conditional
// prologue load would have skipped X27. The helper must still capture
// the architectural SP from regs[31], not the saved host X27.
func TestJIT_ARM64_LOAD_HelperExit_FutureSPWriter_PreservesSP(t *testing.T) {
	r := newJITTestRig(t)
	const highAddr uint64 = 0x0000_0001_0000_8000
	const archSP uint64 = 0xABCD_EF01_2345_6789
	r.cpu.regs[2] = highAddr
	r.cpu.regs[31] = archSP
	r.ctx.NeedIOFallback = 0
	r.ctx.NeedHelper = HELPER_NONE

	// LOAD bails (high addr) → helper exit. Subsequent MOVEQ R31 in same
	// scanned block forces br.written|=1<<31 while br.read stays 0.
	r.compileAndRun(t,
		ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0),
		ie64Instr(OP_MOVEQ, 31, IE64_SIZE_Q, 1, 0, 0, 0x1234),
	)

	if r.ctx.NeedHelper != HELPER_LOAD {
		t.Fatalf("NeedHelper = %d, want HELPER_LOAD", r.ctx.NeedHelper)
	}
	if r.ctx.LiveSP != archSP {
		t.Fatalf("LiveSP = 0x%016X, want 0x%016X (helper must capture arch SP, not host garbage)", r.ctx.LiveSP, archSP)
	}
}

func TestJIT_ARM64_LOAD_HighAddr_HelperEndToEnd(t *testing.T) {
	const want uint64 = 0xFEEDFACECAFEBEEF
	const highAddr uint64 = 0x0000_0001_0000_8000

	cpu, _ := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			for i := uint64(0); i < 8; i++ {
				cpu.bus.backing.Write8(highAddr+i, byte(want>>(8*i)))
			}
			cpu.regs[2] = highAddr
		},
		ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0),
	)

	if cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%016X, want 0x%016X", cpu.regs[1], want)
	}
}
