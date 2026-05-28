// jit_load_helper_amd64_test.go — Phase 5 cycle 5.3: AMD64 LOAD helper-exit.
//
// Pins the rewrite of emitLOAD_AMD64 to set the JITContext HELPER_LOAD
// protocol fields when the load cannot complete locally:
//   - MMU enabled (any address)
//   - Effective address above MemSize - accessBytes (high addr)
//   - Effective address inside an I/O page (low MMIO hole)
//
// Each case must exit the block via the helper protocol (NeedHelper =
// HELPER_LOAD with HelperAddr / HelperSize / HelperRd / HelperPC /
// LiveSP all populated) and NOT via the legacy NeedIOFallback channel.

//go:build amd64 && linux

package main

import (
	"encoding/binary"
	"testing"
	"time"
)

var _ = time.Second

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

// TestJIT_AMD64_LOAD_HighAddr_SetsHelper — high address routes to
// HELPER_LOAD, not NeedIOFallback.
func TestJIT_AMD64_LOAD_HighAddr_SetsHelper(t *testing.T) {
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

// TestJIT_AMD64_LOAD_MMUEnabled_SetsHelper — MMU on routes any LOAD
// through HELPER_LOAD regardless of address.
func TestJIT_AMD64_LOAD_MMUEnabled_SetsHelper(t *testing.T) {
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

// TestJIT_AMD64_LOAD_LowAddr_NoHelper — fast-path load (below IOStart,
// MMU off) must not touch the helper protocol.
func TestJIT_AMD64_LOAD_LowAddr_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	const want uint64 = 0xDEADBEEFCAFEBABE
	const addr uint32 = 0x4000
	binary.LittleEndian.PutUint64(r.cpu.memory[addr:], want)
	r.cpu.regs[2] = uint64(addr)
	r.ctx.NeedHelper = 0xDEADBEEF // poison to verify untouched

	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%016X, want 0x%016X", r.cpu.regs[1], want)
	}
	if r.ctx.NeedHelper != 0xDEADBEEF {
		t.Fatalf("NeedHelper = %d, want untouched poison (fast path must not write helper)", r.ctx.NeedHelper)
	}
}

// TestJIT_AMD64_LOAD_HighAddr_HelperEndToEnd — dispatcher services
// HELPER_LOAD against SparseBacking via cpu.loadMem and advances PC.
// Verifies the emitter + dispatcher round trip.
func TestJIT_AMD64_LOAD_HighAddr_HelperEndToEnd(t *testing.T) {
	const want uint64 = 0xFEEDFACECAFEBEEF
	const highAddr uint64 = 0x0000_0001_0000_8000

	cpu, _ := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			for i := uint64(0); i < 8; i++ {
				cpu.bus.backing.Write8(highAddr+i, byte(want>>(8*i)))
			}
			cpu.regs[2] = highAddr
		},
		ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0),
	)
	_ = time.Second

	if cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%016X, want 0x%016X (dispatcher must read from backing)", cpu.regs[1], want)
	}
}
