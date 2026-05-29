// jit_phase6_branch_target_arm64_test.go — Phase 6 branch-target widening tests (ARM64).
//
// ARM64 mirror of the AMD64 Phase 6 enforcement tests. Every branch/jump
// emitter (emitBRA/emitBcc/emitJMP/emitJSR/emitJSR_IND) computes targetPC as
// uint64 and routes it through emitPackedPCAndCount / emitLoadImm64; these
// tests pin that a >4 GiB target survives the full ExecuteJIT round-trip.
//
// Run via QEMU user-mode:
//   GOARCH=arm64 go test -tags headless -c -o ie_arm64.test .
//   ./ie_arm64.test -test.run 'TestJIT_ARM64_(JMP|JSR_IND|BRA).*(HighTarget|Boundary)'

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"testing"
)

const phase6HighTargetARM64 uint64 = 0x0000_0001_0020_0000

func TestJIT_ARM64_JMP_HighTarget_Above4GiB(t *testing.T) {
	cpu, _ := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			phase4PlantInstrAt(cpu.bus.backing.(*SparseBacking), phase6HighTargetARM64, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
			cpu.regs[2] = phase6HighTargetARM64
		},
		ie64Instr(OP_JMP, 0, 0, 0, 2, 0, 0), // JMP R2, +0 → PC = R2
	)

	if cpu.PC != phase6HighTargetARM64 {
		t.Fatalf("cpu.PC = 0x%016X, want 0x%016X (JMP target must be full 64-bit)", cpu.PC, phase6HighTargetARM64)
	}
}

func TestJIT_ARM64_JSR_IND_HighTarget_Above4GiB(t *testing.T) {
	cpu, _ := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			phase4PlantInstrAt(cpu.bus.backing.(*SparseBacking), phase6HighTargetARM64, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
			cpu.regs[2] = phase6HighTargetARM64
			cpu.regs[31] = STACK_START // low, valid SP → fast-path push
		},
		ie64Instr(OP_JSR_IND, 0, 0, 0, 2, 0, 0), // JSR_IND R2, +0 → target = R2
	)

	if cpu.PC != phase6HighTargetARM64 {
		t.Fatalf("cpu.PC = 0x%016X, want 0x%016X (JSR_IND target must be full 64-bit)", cpu.PC, phase6HighTargetARM64)
	}
	if cpu.regs[31] != STACK_START-8 {
		t.Fatalf("SP = 0x%016X, want 0x%016X (pre-decrement by 8)", cpu.regs[31], uint64(STACK_START-8))
	}
	got := binary.LittleEndian.Uint64(cpu.memory[STACK_START-8:])
	if got != PROG_START+IE64_INSTR_SIZE {
		t.Fatalf("stack[SP] = 0x%016X, want 0x%016X (return addr)", got, uint64(PROG_START+IE64_INSTR_SIZE))
	}
}

func TestJIT_ARM64_BRA_AcrossUint32Boundary(t *testing.T) {
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
