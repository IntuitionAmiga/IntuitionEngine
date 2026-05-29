// jit_push_pop_helper_arm64_test.go — Phase 5 cycle 5.8 ARM64 PUSH/POP helper-exit tests.

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"testing"
)

func TestJIT_ARM64_PUSH_HighSP_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const highSP uint64 = 0x0000_0001_0000_8000
	const payload uint64 = 0xDEADBEEFCAFEBABE
	r.cpu.regs[3] = payload
	r.cpu.regs[31] = highSP
	r.ctx.NeedIOFallback = 0
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_PUSH64, 0, 0, 0, 3, 0, 0))

	if r.ctx.NeedHelper != HELPER_PUSH {
		t.Fatalf("NeedHelper = %d, want HELPER_PUSH (%d)", r.ctx.NeedHelper, HELPER_PUSH)
	}
	if r.ctx.NeedIOFallback != 0 {
		t.Fatalf("NeedIOFallback = %d, want 0", r.ctx.NeedIOFallback)
	}
	if r.ctx.HelperVal != payload {
		t.Fatalf("HelperVal = 0x%016X, want 0x%016X", r.ctx.HelperVal, payload)
	}
	if r.ctx.LiveSP != highSP {
		t.Fatalf("LiveSP = 0x%016X, want 0x%016X (pre-decrement SP)", r.ctx.LiveSP, highSP)
	}
	if r.ctx.HelperPC != PROG_START {
		t.Fatalf("HelperPC = 0x%016X, want 0x%016X", r.ctx.HelperPC, PROG_START)
	}
}

func TestJIT_ARM64_POP_HighSP_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const highSP uint64 = 0x0000_0001_0000_8000
	r.cpu.regs[31] = highSP
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_POP64, 5, 0, 0, 0, 0, 0))

	if r.ctx.NeedHelper != HELPER_POP {
		t.Fatalf("NeedHelper = %d, want HELPER_POP (%d)", r.ctx.NeedHelper, HELPER_POP)
	}
	if r.ctx.HelperRd != 5 {
		t.Fatalf("HelperRd = %d, want 5", r.ctx.HelperRd)
	}
	if r.ctx.LiveSP != highSP {
		t.Fatalf("LiveSP = 0x%016X, want 0x%016X", r.ctx.LiveSP, highSP)
	}
	if r.ctx.HelperPC != PROG_START {
		t.Fatalf("HelperPC = 0x%016X, want 0x%016X", r.ctx.HelperPC, PROG_START)
	}
}

func TestJIT_ARM64_PUSH_MMUEnabled_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const payload uint64 = 0x1122334455667788
	r.cpu.regs[3] = payload
	r.cpu.regs[31] = STACK_START
	r.ctx.NeedHelper = HELPER_NONE
	r.ctx.MMUEnabled = 1
	defer func() { r.ctx.MMUEnabled = 0 }()

	r.compileAndRun(t, ie64Instr(OP_PUSH64, 0, 0, 0, 3, 0, 0))

	if r.ctx.NeedHelper != HELPER_PUSH {
		t.Fatalf("NeedHelper = %d, want HELPER_PUSH (MMU on must helper-exit)", r.ctx.NeedHelper)
	}
	if r.ctx.HelperVal != payload {
		t.Fatalf("HelperVal = 0x%016X, want 0x%016X", r.ctx.HelperVal, payload)
	}
	if r.ctx.LiveSP != STACK_START {
		t.Fatalf("LiveSP = 0x%016X, want 0x%X", r.ctx.LiveSP, uint64(STACK_START))
	}
}

func TestJIT_ARM64_POP_MMUEnabled_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[31] = STACK_START
	r.ctx.NeedHelper = HELPER_NONE
	r.ctx.MMUEnabled = 1
	defer func() { r.ctx.MMUEnabled = 0 }()

	r.compileAndRun(t, ie64Instr(OP_POP64, 7, 0, 0, 0, 0, 0))

	if r.ctx.NeedHelper != HELPER_POP {
		t.Fatalf("NeedHelper = %d, want HELPER_POP (MMU on must helper-exit)", r.ctx.NeedHelper)
	}
	if r.ctx.HelperRd != 7 {
		t.Fatalf("HelperRd = %d, want 7", r.ctx.HelperRd)
	}
}

func TestJIT_ARM64_PUSH_LowSP_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	const payload uint64 = 0xC0FFEE0042424242
	r.cpu.regs[3] = payload
	r.cpu.regs[31] = STACK_START
	r.ctx.NeedHelper = 0xDEADBEEF

	r.compileAndRun(t, ie64Instr(OP_PUSH64, 0, 0, 0, 3, 0, 0))

	if r.cpu.regs[31] != STACK_START-8 {
		t.Fatalf("SP = 0x%X, want 0x%X (decremented)", r.cpu.regs[31], uint64(STACK_START-8))
	}
	sp := uint32(r.cpu.regs[31])
	got := binary.LittleEndian.Uint64(r.cpu.memory[sp:])
	if got != payload {
		t.Fatalf("memory[0x%X] = 0x%016X, want 0x%016X", sp, got, payload)
	}
	if r.ctx.NeedHelper != 0xDEADBEEF {
		t.Fatalf("NeedHelper = %d, want untouched poison", r.ctx.NeedHelper)
	}
}

func TestJIT_ARM64_POP_LowSP_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	const payload uint64 = 0x9988776655443322
	r.cpu.regs[31] = STACK_START
	binary.LittleEndian.PutUint64(r.cpu.memory[STACK_START:], payload)
	r.ctx.NeedHelper = 0xDEADBEEF

	r.compileAndRun(t, ie64Instr(OP_POP64, 4, 0, 0, 0, 0, 0))

	if r.cpu.regs[4] != payload {
		t.Fatalf("R4 = 0x%016X, want 0x%016X", r.cpu.regs[4], payload)
	}
	if r.cpu.regs[31] != STACK_START+8 {
		t.Fatalf("SP = 0x%X, want 0x%X (incremented)", r.cpu.regs[31], uint64(STACK_START+8))
	}
	if r.ctx.NeedHelper != 0xDEADBEEF {
		t.Fatalf("NeedHelper = %d, want untouched poison", r.ctx.NeedHelper)
	}
}

func TestJIT_ARM64_PUSH_R31_HighSP_StoresDecrementedSP(t *testing.T) {
	r := newJITTestRig(t)
	const highSP uint64 = 0x0000_0001_0000_8000
	r.cpu.regs[31] = highSP
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_PUSH64, 0, 0, 0, 31, 0, 0))

	if r.ctx.NeedHelper != HELPER_PUSH {
		t.Fatalf("NeedHelper = %d, want HELPER_PUSH", r.ctx.NeedHelper)
	}
	if r.ctx.HelperVal != highSP-8 {
		t.Fatalf("HelperVal = 0x%016X, want 0x%016X (SP-8)", r.ctx.HelperVal, highSP-8)
	}
	if r.ctx.LiveSP != highSP {
		t.Fatalf("LiveSP = 0x%016X, want 0x%016X (pre-decrement)", r.ctx.LiveSP, highSP)
	}
}

func TestJIT_ARM64_PUSH_R31_HighSP_EndToEnd(t *testing.T) {
	const highAddr uint64 = 0x0000_0001_0000_8000

	_, backing := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			cpu.regs[31] = highAddr + 8 // SP-8 lands at highAddr
		},
		ie64Instr(OP_PUSH64, 0, 0, 0, 31, 0, 0),
	)

	var got uint64
	for i := uint64(0); i < 8; i++ {
		got |= uint64(backing.Read8(highAddr+i)) << (8 * i)
	}
	if got != highAddr {
		t.Fatalf("backing[0x%016X] = 0x%016X, want 0x%016X (SP-8)", highAddr, got, highAddr)
	}
}

func TestJIT_ARM64_PUSH_HighSP_HelperEndToEnd(t *testing.T) {
	const highAddr uint64 = 0x0000_0001_0000_8000
	const payload uint64 = 0xFEEDFACEDEADC0DE

	cpu, backing := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			cpu.regs[3] = payload
			cpu.regs[31] = highAddr + 8 // SP-8 lands at highAddr
		},
		ie64Instr(OP_PUSH64, 0, 0, 0, 3, 0, 0),
	)

	var got uint64
	for i := uint64(0); i < 8; i++ {
		got |= uint64(backing.Read8(highAddr+i)) << (8 * i)
	}
	if got != payload {
		t.Fatalf("backing[0x%016X] = 0x%016X, want 0x%016X", highAddr, got, payload)
	}
	if cpu.regs[31] != highAddr {
		t.Fatalf("SP = 0x%016X, want 0x%016X (decremented by 8)", cpu.regs[31], highAddr)
	}
}

func TestJIT_ARM64_POP_HighSP_HelperEndToEnd(t *testing.T) {
	const highAddr uint64 = 0x0000_0001_0000_8000
	const payload uint64 = 0x0123456789ABCDEF

	cpu, _ := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			for i := uint64(0); i < 8; i++ {
				cpu.bus.backing.Write8(highAddr+i, byte(payload>>(8*i)))
			}
			cpu.regs[31] = highAddr
		},
		ie64Instr(OP_POP64, 6, 0, 0, 0, 0, 0),
	)

	if cpu.regs[6] != payload {
		t.Fatalf("R6 = 0x%016X, want 0x%016X", cpu.regs[6], payload)
	}
	if cpu.regs[31] != highAddr+8 {
		t.Fatalf("SP = 0x%016X, want 0x%016X (incremented by 8)", cpu.regs[31], highAddr+8)
	}
}
