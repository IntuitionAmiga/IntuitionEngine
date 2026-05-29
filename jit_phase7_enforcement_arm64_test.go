// jit_phase7_enforcement_arm64_test.go — Phase 7 comprehensive enforcement (ARM64).
//
// ARM64 mirror of the AMD64 Phase 7 end-to-end gaps: FLOAD/FSTORE
// high-backing round-trip, JSR_IND→high-target→RTS full 64-bit PC
// round-trip, and two >4 GiB JIT blocks joined by a branch. Run via
// QEMU user-mode:
//   GOARCH=arm64 go test -tags headless -c -o ie_arm64.test .
//   ./ie_arm64.test -test.run 'TestJIT_ARM64_(FLOAD|FSTORE)_HighBacking_EndToEnd|TestJIT_ARM64_JSR_IND_HighTarget_RTS_RoundTrip|TestJIT_ARM64_MultiBlock_HighPC'

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"testing"
)

const (
	phase7HighDataARM64   uint64 = 0x0000_0001_0000_8000
	phase7RTSTargetARM64  uint64 = 0x0000_0001_0030_0000
	phase7ChainBlockARM64 uint64 = 0x0000_0001_0040_0000
)

func TestJIT_ARM64_FLOAD_HighBacking_EndToEnd(t *testing.T) {
	const want uint32 = 0x40490FDB // float32(3.14159)
	const lowAlias uint32 = 0x8000

	cpu, _ := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			for i := uint64(0); i < 4; i++ {
				cpu.bus.backing.Write8(phase7HighDataARM64+i, byte(want>>(8*i)))
			}
			binary.LittleEndian.PutUint32(cpu.memory[lowAlias:], 0xAAAAAAAA)
			cpu.regs[2] = phase7HighDataARM64
		},
		ie64Instr(OP_FLOAD, 5, IE64_SIZE_L, 0, 2, 0, 0),
	)

	if cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	if cpu.FPU.FPRegs[5] != want {
		t.Fatalf("F5 = 0x%08X, want 0x%08X (backing value, not alias)", cpu.FPU.FPRegs[5], want)
	}
	if got := binary.LittleEndian.Uint32(cpu.memory[lowAlias:]); got != 0xAAAAAAAA {
		t.Fatalf("low alias 0x%X = 0x%08X, want 0xAAAAAAAA (must not be touched)", lowAlias, got)
	}
}

func TestJIT_ARM64_FSTORE_HighBacking_EndToEnd(t *testing.T) {
	const want uint32 = 0xC2280000 // float32(-42.0)
	const lowAlias uint32 = 0x8000

	cpu, backing := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			if cpu.FPU != nil {
				cpu.FPU.FPRegs[3] = want
			}
			for i := uint32(0); i < 4; i++ {
				cpu.memory[lowAlias+i] = 0
			}
			cpu.regs[2] = phase7HighDataARM64
		},
		ie64Instr(OP_FSTORE, 3, IE64_SIZE_L, 0, 2, 0, 0),
	)

	if cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	var got uint32
	for i := uint32(0); i < 4; i++ {
		got |= uint32(backing.Read8(phase7HighDataARM64+uint64(i))) << (8 * i)
	}
	if got != want {
		t.Fatalf("backing[0x%016X] = 0x%08X, want 0x%08X", phase7HighDataARM64, got, want)
	}
	if low := binary.LittleEndian.Uint32(cpu.memory[lowAlias:]); low != 0 {
		t.Fatalf("low alias 0x%X = 0x%08X, want 0 (must not be touched)", lowAlias, low)
	}
}

func TestJIT_ARM64_JSR_IND_HighTarget_RTS_RoundTrip(t *testing.T) {
	cpu, _ := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			phase4PlantInstrAt(cpu.bus.backing.(*SparseBacking), phase7RTSTargetARM64,
				ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0))
			cpu.regs[2] = phase7RTSTargetARM64
			cpu.regs[31] = STACK_START
		},
		ie64Instr(OP_JSR_IND, 0, 0, 0, 2, 0, 0),
	)

	if cpu.PC != PROG_START+IE64_INSTR_SIZE {
		t.Fatalf("cpu.PC = 0x%016X, want 0x%016X (RTS must return to caller+1)", cpu.PC, uint64(PROG_START+IE64_INSTR_SIZE))
	}
	if cpu.regs[31] != STACK_START {
		t.Fatalf("SP = 0x%016X, want 0x%016X (JSR_IND/RTS must balance)", cpu.regs[31], uint64(STACK_START))
	}
}

// TestJIT_ARM64_MultiBlock_HighPC runs two JIT blocks whose PCs are both
// above 4 GiB, joined by a forward BRA. Unlike AMD64, the ARM64 emitter
// does not install chain slots for BRA (emitBRA exits to the dispatcher),
// so this is not a patched-chain test — it pins that the dispatcher
// resolves a >4 GiB branch target and dispatches the successor block at a
// high PC. Block A sets R1 and branches to block B; block B sets R2 and
// halts.
func TestJIT_ARM64_MultiBlock_HighPC(t *testing.T) {
	bus, backing := phase4BusWithHighBacking(t)

	phase4PlantInstrAt(backing, phase7ChainBlockARM64,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x111))
	phase4PlantInstrAt(backing, phase7ChainBlockARM64+8,
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 16)) // target = (highPC+8) + 16 = highPC+24

	phase4PlantInstrAt(backing, phase7ChainBlockARM64+24,
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0x222))
	phase4PlantInstrAt(backing, phase7ChainBlockARM64+32,
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu := phase4RunUntilHalt(t, bus, phase7ChainBlockARM64)

	if cpu.regs[1] != 0x111 {
		t.Fatalf("R1 = 0x%X, want 0x111 (block A must run)", cpu.regs[1])
	}
	if cpu.regs[2] != 0x222 {
		t.Fatalf("R2 = 0x%X, want 0x222 (successor block B must run at high PC)", cpu.regs[2])
	}
	if cpu.PC != phase7ChainBlockARM64+32 {
		t.Fatalf("cpu.PC = 0x%016X, want 0x%016X (HALT addr)", cpu.PC, phase7ChainBlockARM64+32)
	}
}
