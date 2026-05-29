// jit_jsr_rts_helper_amd64_test.go — Phase 5 JSR/RTS/JSR_IND helper-exit tests (AMD64).

//go:build amd64 && linux

package main

import (
	"encoding/binary"
	"testing"
)

// ── JSR ──────────────────────────────────────────────────────────────────

func TestJIT_AMD64_JSR_HighSP_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const highSP uint64 = 0x0000_0001_0000_8000
	r.cpu.regs[31] = highSP
	r.ctx.NeedHelper = HELPER_NONE

	// JSR target = instrPC + 0x100.
	r.compileAndRun(t, ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 0x100))

	if r.ctx.NeedHelper != HELPER_JSR {
		t.Fatalf("NeedHelper = %d, want HELPER_JSR (%d)", r.ctx.NeedHelper, HELPER_JSR)
	}
	if r.ctx.HelperVal != PROG_START+IE64_INSTR_SIZE {
		t.Fatalf("HelperVal = 0x%016X, want 0x%016X (return addr)", r.ctx.HelperVal, uint64(PROG_START+IE64_INSTR_SIZE))
	}
	if r.ctx.HelperAddr != PROG_START+0x100 {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X (call target)", r.ctx.HelperAddr, uint64(PROG_START+0x100))
	}
	if r.ctx.LiveSP != highSP {
		t.Fatalf("LiveSP = 0x%016X, want 0x%016X (pre-decrement)", r.ctx.LiveSP, highSP)
	}
	if r.ctx.HelperPC != PROG_START {
		t.Fatalf("HelperPC = 0x%016X, want 0x%016X", r.ctx.HelperPC, uint64(PROG_START))
	}
}

func TestJIT_AMD64_JSR_MMUEnabled_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[31] = STACK_START
	r.ctx.NeedHelper = HELPER_NONE
	r.ctx.MMUEnabled = 1
	defer func() { r.ctx.MMUEnabled = 0 }()

	r.compileAndRun(t, ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 0x40))

	if r.ctx.NeedHelper != HELPER_JSR {
		t.Fatalf("NeedHelper = %d, want HELPER_JSR (MMU on must helper-exit)", r.ctx.NeedHelper)
	}
	if r.ctx.HelperAddr != PROG_START+0x40 {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X", r.ctx.HelperAddr, uint64(PROG_START+0x40))
	}
	if r.ctx.LiveSP != STACK_START {
		t.Fatalf("LiveSP = 0x%016X, want 0x%X", r.ctx.LiveSP, uint64(STACK_START))
	}
}

func TestJIT_AMD64_JSR_LowSP_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[31] = STACK_START
	r.ctx.NeedHelper = 0xDEADBEEF

	r.compileAndRun(t, ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 0x100))

	if r.cpu.regs[31] != STACK_START-8 {
		t.Fatalf("SP = 0x%X, want 0x%X (decremented)", r.cpu.regs[31], uint64(STACK_START-8))
	}
	sp := uint32(r.cpu.regs[31])
	got := binary.LittleEndian.Uint64(r.cpu.memory[sp:])
	if got != PROG_START+IE64_INSTR_SIZE {
		t.Fatalf("memory[0x%X] = 0x%016X, want 0x%016X (return addr)", sp, got, uint64(PROG_START+IE64_INSTR_SIZE))
	}
	if r.ctx.NeedHelper != 0xDEADBEEF {
		t.Fatalf("NeedHelper = %d, want untouched poison", r.ctx.NeedHelper)
	}
}

// ── RTS ──────────────────────────────────────────────────────────────────

func TestJIT_AMD64_RTS_HighSP_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const highSP uint64 = 0x0000_0001_0000_8000
	r.cpu.regs[31] = highSP
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0))

	if r.ctx.NeedHelper != HELPER_RTS {
		t.Fatalf("NeedHelper = %d, want HELPER_RTS (%d)", r.ctx.NeedHelper, HELPER_RTS)
	}
	if r.ctx.LiveSP != highSP {
		t.Fatalf("LiveSP = 0x%016X, want 0x%016X", r.ctx.LiveSP, highSP)
	}
	if r.ctx.HelperPC != PROG_START {
		t.Fatalf("HelperPC = 0x%016X, want 0x%016X", r.ctx.HelperPC, uint64(PROG_START))
	}
}

func TestJIT_AMD64_RTS_MMUEnabled_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[31] = STACK_START
	r.ctx.NeedHelper = HELPER_NONE
	r.ctx.MMUEnabled = 1
	defer func() { r.ctx.MMUEnabled = 0 }()

	r.compileAndRun(t, ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0))

	if r.ctx.NeedHelper != HELPER_RTS {
		t.Fatalf("NeedHelper = %d, want HELPER_RTS (MMU on must helper-exit)", r.ctx.NeedHelper)
	}
}

func TestJIT_AMD64_RTS_LowSP_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	const retAddr uint64 = PROG_START + 0x200
	r.cpu.regs[31] = STACK_START
	binary.LittleEndian.PutUint64(r.cpu.memory[STACK_START:], retAddr)
	r.ctx.NeedHelper = 0xDEADBEEF

	r.compileAndRun(t, ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0))

	if r.cpu.PC != retAddr {
		t.Fatalf("PC = 0x%016X, want 0x%016X (popped return addr)", r.cpu.PC, retAddr)
	}
	if r.cpu.regs[31] != STACK_START+8 {
		t.Fatalf("SP = 0x%X, want 0x%X (incremented)", r.cpu.regs[31], uint64(STACK_START+8))
	}
	if r.ctx.NeedHelper != 0xDEADBEEF {
		t.Fatalf("NeedHelper = %d, want untouched poison", r.ctx.NeedHelper)
	}
}

// ── JSR_IND ──────────────────────────────────────────────────────────────

func TestJIT_AMD64_JSR_IND_HighSP_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const highSP uint64 = 0x0000_0001_0000_8000
	const base uint64 = 0x0000_0000_1234_0000
	r.cpu.regs[31] = highSP
	r.cpu.regs[5] = base
	r.ctx.NeedHelper = HELPER_NONE

	// target = R5 + 0x10.
	r.compileAndRun(t, ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, 0x10))

	if r.ctx.NeedHelper != HELPER_JSR_IND {
		t.Fatalf("NeedHelper = %d, want HELPER_JSR_IND (%d)", r.ctx.NeedHelper, HELPER_JSR_IND)
	}
	if r.ctx.HelperAddr != base+0x10 {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X (rs + imm32)", r.ctx.HelperAddr, base+0x10)
	}
	if r.ctx.HelperVal != PROG_START+IE64_INSTR_SIZE {
		t.Fatalf("HelperVal = 0x%016X, want 0x%016X (return addr)", r.ctx.HelperVal, uint64(PROG_START+IE64_INSTR_SIZE))
	}
	if r.ctx.LiveSP != highSP {
		t.Fatalf("LiveSP = 0x%016X, want 0x%016X", r.ctx.LiveSP, highSP)
	}
}

// TestJIT_AMD64_JSR_IND_R31_HighSP_UsesDecrementedSP pins the SP-relative
// indirect target. The interpreter decrements SP before resolving rs+imm
// (cpu_ie64.go:1998-2010), so for rs==R31 the helper must base the target on
// SP-8, not the pre-decrement SP.
func TestJIT_AMD64_JSR_IND_R31_HighSP_UsesDecrementedSP(t *testing.T) {
	r := newJITTestRig(t)
	const highSP uint64 = 0x0000_0001_0000_8000
	r.cpu.regs[31] = highSP
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_JSR_IND, 0, 0, 0, 31, 0, 0x10))

	if r.ctx.NeedHelper != HELPER_JSR_IND {
		t.Fatalf("NeedHelper = %d, want HELPER_JSR_IND", r.ctx.NeedHelper)
	}
	if r.ctx.HelperAddr != highSP-8+0x10 {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X ((SP-8)+imm)", r.ctx.HelperAddr, highSP-8+0x10)
	}
	if r.ctx.LiveSP != highSP {
		t.Fatalf("LiveSP = 0x%016X, want 0x%016X (pre-decrement)", r.ctx.LiveSP, highSP)
	}
}

func TestJIT_AMD64_JSR_IND_MMUEnabled_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const base uint64 = 0x0000_0000_0042_0000
	r.cpu.regs[31] = STACK_START
	r.cpu.regs[6] = base
	r.ctx.NeedHelper = HELPER_NONE
	r.ctx.MMUEnabled = 1
	defer func() { r.ctx.MMUEnabled = 0 }()

	r.compileAndRun(t, ie64Instr(OP_JSR_IND, 0, 0, 0, 6, 0, 0x8))

	if r.ctx.NeedHelper != HELPER_JSR_IND {
		t.Fatalf("NeedHelper = %d, want HELPER_JSR_IND (MMU on must helper-exit)", r.ctx.NeedHelper)
	}
	if r.ctx.HelperAddr != base+0x8 {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X", r.ctx.HelperAddr, base+0x8)
	}
}

// ── End-to-end through the full ExecuteJIT dispatch + helper round-trip ──

func TestJIT_AMD64_JSR_HighSP_HelperEndToEnd(t *testing.T) {
	const highAddr uint64 = 0x0000_0001_0000_8000

	// JSR target = PROG_START + 8 (the appended HALT). After the call the
	// return address is pushed to backing[highAddr] and PC lands on HALT.
	cpu, backing := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			cpu.regs[31] = highAddr + 8 // SP-8 lands at highAddr
		},
		ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, IE64_INSTR_SIZE),
	)

	var got uint64
	for i := uint64(0); i < 8; i++ {
		got |= uint64(backing.Read8(highAddr+i)) << (8 * i)
	}
	if got != PROG_START+IE64_INSTR_SIZE {
		t.Fatalf("backing[0x%016X] = 0x%016X, want 0x%016X (return addr)", highAddr, got, uint64(PROG_START+IE64_INSTR_SIZE))
	}
	if cpu.regs[31] != highAddr {
		t.Fatalf("SP = 0x%016X, want 0x%016X (decremented by 8)", cpu.regs[31], highAddr)
	}
}

func TestJIT_AMD64_RTS_HighSP_HelperEndToEnd(t *testing.T) {
	const highAddr uint64 = 0x0000_0001_0000_8000
	const retAddr uint64 = PROG_START + IE64_INSTR_SIZE // the appended HALT

	cpu, _ := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			for i := uint64(0); i < 8; i++ {
				cpu.bus.backing.Write8(highAddr+i, byte(retAddr>>(8*i)))
			}
			cpu.regs[31] = highAddr
		},
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),
	)

	if cpu.regs[31] != highAddr+8 {
		t.Fatalf("SP = 0x%016X, want 0x%016X (incremented by 8)", cpu.regs[31], highAddr+8)
	}
}

func TestJIT_AMD64_JSR_IND_HighSP_HelperEndToEnd(t *testing.T) {
	const highAddr uint64 = 0x0000_0001_0000_8000

	// target = R5 + 8 = PROG_START + 8 (the appended HALT).
	cpu, backing := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			cpu.regs[5] = PROG_START
			cpu.regs[31] = highAddr + 8
		},
		ie64Instr(OP_JSR_IND, 0, 0, 0, 5, 0, IE64_INSTR_SIZE),
	)

	var got uint64
	for i := uint64(0); i < 8; i++ {
		got |= uint64(backing.Read8(highAddr+i)) << (8 * i)
	}
	if got != PROG_START+IE64_INSTR_SIZE {
		t.Fatalf("backing[0x%016X] = 0x%016X, want 0x%016X (return addr)", highAddr, got, uint64(PROG_START+IE64_INSTR_SIZE))
	}
	if cpu.regs[31] != highAddr {
		t.Fatalf("SP = 0x%016X, want 0x%016X (decremented by 8)", cpu.regs[31], highAddr)
	}
}
