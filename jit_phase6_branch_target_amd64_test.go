// jit_phase6_branch_target_amd64_test.go — Phase 6 branch-target widening tests (AMD64).
//
// Phase 6 proves every branch/jump target flows as a full uint64 with no
// uint32 truncation:
//   - JMP rs+imm  → target = regs[rs] + sext(imm32)
//   - JSR_IND rs+imm → call target = regs[rs] + sext(imm32)
//   - BRA instrPC+imm → target = instrPC + sext(imm32), crossing the 4 GiB line
//
// The widening itself landed as a byproduct of Phases 1-5 (every emitter
// already computes targetPC as uint64 and routes it through
// emitPackedPCAndCount / emitLoadImm64AMD64). These tests are the
// enforcement layer: a regression that re-narrows any target to uint32
// would alias the high target into a low page and fail here.

//go:build amd64 && linux

package main

import (
	"encoding/binary"
	"testing"
)

// phase6HighTarget is well above 4 GiB. A uint32 truncation of any branch
// target would alias it into a low page and the planted HALT would never
// be reached.
const phase6HighTarget uint64 = 0x0000_0001_0020_0000

// TestJIT_AMD64_JMP_HighTarget_Above4GiB drives JMP rs,#0 with rs holding a
// >4 GiB address. The dispatcher must re-enter at the full 64-bit target and
// fetch the HALT planted there. Pre-widening, the PC was truncated to uint32
// and aliased into low memory.
func TestJIT_AMD64_JMP_HighTarget_Above4GiB(t *testing.T) {
	cpu, _ := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			phase4PlantInstrAt(cpu.bus.backing.(*SparseBacking), phase6HighTarget, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
			cpu.regs[2] = phase6HighTarget
		},
		ie64Instr(OP_JMP, 0, 0, 0, 2, 0, 0), // JMP R2, +0 → PC = R2
	)

	if cpu.PC != phase6HighTarget {
		t.Fatalf("cpu.PC = 0x%016X, want 0x%016X (JMP target must be full 64-bit)", cpu.PC, phase6HighTarget)
	}
}

// TestJIT_AMD64_JSR_IND_HighTarget_Above4GiB drives JSR_IND rs,#0 with rs
// holding a >4 GiB call target while SP stays in the low window (fast-path
// push). The return address must land on the low stack and PC must re-enter
// at the full 64-bit target.
func TestJIT_AMD64_JSR_IND_HighTarget_Above4GiB(t *testing.T) {
	cpu, _ := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			phase4PlantInstrAt(cpu.bus.backing.(*SparseBacking), phase6HighTarget, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
			cpu.regs[2] = phase6HighTarget
			cpu.regs[31] = STACK_START // low, valid SP → fast-path push
		},
		ie64Instr(OP_JSR_IND, 0, 0, 0, 2, 0, 0), // JSR_IND R2, +0 → target = R2
	)

	if cpu.PC != phase6HighTarget {
		t.Fatalf("cpu.PC = 0x%016X, want 0x%016X (JSR_IND target must be full 64-bit)", cpu.PC, phase6HighTarget)
	}
	if cpu.regs[31] != STACK_START-8 {
		t.Fatalf("SP = 0x%016X, want 0x%016X (pre-decrement by 8)", cpu.regs[31], uint64(STACK_START-8))
	}
	got := binary.LittleEndian.Uint64(cpu.memory[STACK_START-8:])
	if got != PROG_START+IE64_INSTR_SIZE {
		t.Fatalf("stack[SP] = 0x%016X, want 0x%016X (return addr)", got, uint64(PROG_START+IE64_INSTR_SIZE))
	}
}

// TestJIT_AMD64_BRA_AcrossUint32Boundary plants a single BRA block just below
// the 4 GiB line with a forward displacement that lands just above it. The
// target = instrPC + sext(imm32) crosses 0x1_0000_0000. A uint32 truncation
// would compute 0x10 and dispatch into low memory instead of the planted HALT.
func TestJIT_AMD64_BRA_AcrossUint32Boundary(t *testing.T) {
	bus, backing := phase4BusWithHighBacking(t)

	const braPC uint64 = 0x0000_0000_FFFF_FFF0  // 16 bytes below 4 GiB
	const target uint64 = 0x0000_0001_0000_0010 // 16 bytes above 4 GiB
	const disp = uint32(target - braPC)         // +0x20, fits signed imm32

	phase4PlantInstrAt(backing, braPC, ie64Instr(OP_BRA, 0, 0, 0, 0, 0, disp))
	phase4PlantInstrAt(backing, target, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu := phase4RunUntilHalt(t, bus, braPC)

	if cpu.PC != target {
		t.Fatalf("cpu.PC = 0x%016X, want 0x%016X (BRA target must not truncate across 4 GiB)", cpu.PC, target)
	}
}
