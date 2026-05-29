// jit_store_helper_arm64_test.go — Phase 5 cycle 5.5: ARM64 STORE helper-exit.

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"testing"
)

func assertSTOREHelperFields(t *testing.T, ctx *JITContext, wantAddr uint64, wantVal uint64, wantSize uint32, wantPC uint64, wantSP uint64) {
	t.Helper()
	if ctx.NeedHelper != HELPER_STORE {
		t.Fatalf("NeedHelper = %d, want HELPER_STORE (%d)", ctx.NeedHelper, HELPER_STORE)
	}
	if ctx.NeedIOFallback != 0 {
		t.Fatalf("NeedIOFallback = %d, want 0", ctx.NeedIOFallback)
	}
	if ctx.HelperAddr != wantAddr {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X", ctx.HelperAddr, wantAddr)
	}
	if ctx.HelperVal != wantVal {
		t.Fatalf("HelperVal = 0x%016X, want 0x%016X", ctx.HelperVal, wantVal)
	}
	if ctx.HelperSize != wantSize {
		t.Fatalf("HelperSize = %d, want %d", ctx.HelperSize, wantSize)
	}
	if ctx.HelperPC != wantPC {
		t.Fatalf("HelperPC = 0x%016X, want 0x%016X", ctx.HelperPC, wantPC)
	}
	if ctx.LiveSP != wantSP {
		t.Fatalf("LiveSP = 0x%016X, want 0x%016X", ctx.LiveSP, wantSP)
	}
}

func TestJIT_ARM64_STORE_HighAddr_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const highAddr uint64 = 0x0000_0001_0000_8000
	const payload uint64 = 0xDEADBEEFCAFEBABE
	const sentinelSP uint64 = 0xCAFE_BABE_F00D_F00D
	r.cpu.regs[1] = payload
	r.cpu.regs[2] = highAddr
	r.cpu.regs[31] = sentinelSP
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	assertSTOREHelperFields(t, r.ctx, highAddr, payload, uint32(IE64_SIZE_Q), PROG_START, sentinelSP)
}

func TestJIT_ARM64_STORE_MMUEnabled_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const lowAddr uint64 = 0x4000
	const payload uint64 = 0x1122334455667788
	r.cpu.regs[1] = payload
	r.cpu.regs[2] = lowAddr
	r.cpu.regs[31] = 0x2000
	r.ctx.NeedHelper = HELPER_NONE
	r.ctx.MMUEnabled = 1
	defer func() { r.ctx.MMUEnabled = 0 }()

	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	assertSTOREHelperFields(t, r.ctx, lowAddr, payload, uint32(IE64_SIZE_Q), PROG_START, 0x2000)
}

func TestJIT_ARM64_STORE_LowAddr_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	const addr uint32 = 0x4000
	const payload uint64 = 0xC0FFEE0042424242
	r.cpu.regs[1] = payload
	r.cpu.regs[2] = uint64(addr)
	r.ctx.NeedHelper = 0xDEADBEEF

	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	got := binary.LittleEndian.Uint64(r.cpu.memory[addr:])
	if got != payload {
		t.Fatalf("memory[0x%X] = 0x%016X, want 0x%016X", addr, got, payload)
	}
	if r.ctx.NeedHelper != 0xDEADBEEF {
		t.Fatalf("NeedHelper = %d, want untouched poison", r.ctx.NeedHelper)
	}
}

func TestJIT_ARM64_STORE_HighAddr_HelperEndToEnd(t *testing.T) {
	const payload uint64 = 0xFEDCBA0987654321
	const highAddr uint64 = 0x0000_0001_0000_8000

	cpu, backing := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			cpu.regs[1] = payload
			cpu.regs[2] = highAddr
		},
		ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0),
	)

	var got uint64
	for i := uint64(0); i < 8; i++ {
		got |= uint64(backing.Read8(highAddr+i)) << (8 * i)
	}
	if got != payload {
		t.Fatalf("backing[0x%016X] = 0x%016X, want 0x%016X", highAddr, got, payload)
	}
	if cpu.regs[2] != highAddr {
		t.Fatalf("R2 clobbered = 0x%016X, want 0x%016X", cpu.regs[2], highAddr)
	}
}
