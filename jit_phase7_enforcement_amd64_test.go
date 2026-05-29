// jit_phase7_enforcement_amd64_test.go — Phase 7 comprehensive enforcement (AMD64).
//
// Phases 1-6 widened every JIT surface to full 64-bit and routed high /
// MMU memory, stack, FP, and control-flow ops through the JITContext
// helper protocol. The per-op suites already pin the emit-level helper
// shape and individual round-trips. Phase 7 fills the remaining
// end-to-end gaps that exercise full ExecuteJIT dispatch + helper
// re-entry across op boundaries:
//
//   - FLOAD / FSTORE high-backing round-trip (only emit-level before)
//   - JSR_IND to a >4 GiB target, returning via RTS (full 64-bit PC
//     round-trip across the call boundary)
//   - Two JIT blocks at >4 GiB PCs joined by a branch (64-bit chain
//     target installed and resolved end-to-end)

//go:build amd64 && linux

package main

import (
	"encoding/binary"
	"testing"
	"time"
)

// phase7RunOnce sets the PC and runs ExecuteJIT on an existing CPU until
// HALT or timeout, preserving the JIT cache across calls (cpu.jitPersist).
// ExecuteJIT does not self-arm cpu.running, so the caller does.
func phase7RunOnce(t *testing.T, cpu *CPU64, startPC uint64) {
	t.Helper()
	cpu.PC = startPC
	cpu.running.Store(true)
	done := make(chan struct{})
	go func() {
		cpu.ExecuteJIT()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("ExecuteJIT timed out")
	}
}

const (
	phase7HighData   uint64 = 0x0000_0001_0000_8000
	phase7RTSTarget  uint64 = 0x0000_0001_0030_0000
	phase7ChainBlock uint64 = 0x0000_0001_0040_0000
)

// TestJIT_AMD64_FLOAD_HighBacking_EndToEnd loads a 32-bit float from a
// >4 GiB backing slot into an FP register through full ExecuteJIT. A
// uint32 truncation would alias the high address into a low page and
// load the wrong value (or corrupt the low alias).
func TestJIT_AMD64_FLOAD_HighBacking_EndToEnd(t *testing.T) {
	const want uint32 = 0x40490FDB // float32(3.14159)
	const lowAlias uint32 = 0x8000

	cpu, _ := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			for i := uint64(0); i < 4; i++ {
				cpu.bus.backing.Write8(phase7HighData+i, byte(want>>(8*i)))
			}
			binary.LittleEndian.PutUint32(cpu.memory[lowAlias:], 0xAAAAAAAA)
			cpu.regs[2] = phase7HighData
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

// TestJIT_AMD64_FSTORE_HighBacking_EndToEnd writes an FP register to a
// >4 GiB backing slot and verifies the low alias stays clean.
func TestJIT_AMD64_FSTORE_HighBacking_EndToEnd(t *testing.T) {
	const want uint32 = 0xC2280000 // float32(-42.0)
	const lowAlias uint32 = 0x8000

	cpu, backing := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			if cpu.FPU != nil {
				cpu.FPU.FPRegs[3] = want
			}
			for i := uint32(0); i < 4; i++ {
				cpu.memory[lowAlias+i] = 0
			}
			cpu.regs[2] = phase7HighData
		},
		ie64Instr(OP_FSTORE, 3, IE64_SIZE_L, 0, 2, 0, 0),
	)

	if cpu.FPU == nil {
		t.Skip("FPU not initialised")
	}
	var got uint32
	for i := uint32(0); i < 4; i++ {
		got |= uint32(backing.Read8(phase7HighData+uint64(i))) << (8 * i)
	}
	if got != want {
		t.Fatalf("backing[0x%016X] = 0x%08X, want 0x%08X", phase7HighData, got, want)
	}
	if low := binary.LittleEndian.Uint32(cpu.memory[lowAlias:]); low != 0 {
		t.Fatalf("low alias 0x%X = 0x%08X, want 0 (must not be touched)", lowAlias, low)
	}
}

// TestJIT_AMD64_JSR_IND_HighTarget_RTS_RoundTrip calls a subroutine at a
// >4 GiB address via JSR_IND, runs RTS there, and verifies control
// returns to the low caller with the stack pointer restored. This pins
// the full 64-bit PC round-trip across the call/return boundary: the
// indirect target is widened on the way out and the popped return
// address is honoured on the way back.
func TestJIT_AMD64_JSR_IND_HighTarget_RTS_RoundTrip(t *testing.T) {
	cpu, _ := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			// Subroutine at the high target is a bare RTS.
			phase4PlantInstrAt(cpu.bus.backing.(*SparseBacking), phase7RTSTarget,
				ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0))
			cpu.regs[2] = phase7RTSTarget
			cpu.regs[31] = STACK_START // low, valid SP → fast-path push
		},
		ie64Instr(OP_JSR_IND, 0, 0, 0, 2, 0, 0), // JSR_IND R2 → call high target
	)

	// RTS must have returned to the instruction after the JSR_IND, which
	// is the HALT appended at PROG_START+8.
	if cpu.PC != PROG_START+IE64_INSTR_SIZE {
		t.Fatalf("cpu.PC = 0x%016X, want 0x%016X (RTS must return to caller+1)", cpu.PC, uint64(PROG_START+IE64_INSTR_SIZE))
	}
	// JSR_IND pushed (SP-8), RTS popped (SP+8) → net SP unchanged.
	if cpu.regs[31] != STACK_START {
		t.Fatalf("SP = 0x%016X, want 0x%016X (JSR_IND/RTS must balance)", cpu.regs[31], uint64(STACK_START))
	}
}

// TestJIT_AMD64_ChainedBlocks_HighPC links two JIT blocks whose PCs are
// both above 4 GiB via a forward BRA, and verifies the patched native
// chain jump is actually installed and taken — not just that both blocks
// run via dispatcher fallback.
//
// Outbound chain slots are patched at the *requesting* block's compile
// time against already-cached targets (jit_exec.go). So block B is warmed
// (compiled + cached) first; when block A is then compiled, its BRA chain
// slot to B is patched immediately, and A's very first execution takes the
// patched native jump straight into B. A naive single-pass run from A
// would instead miss on B, fall back to the dispatcher, and still pass
// even if the recorded chain target were truncated — which this test
// guards against by asserting the slot's full 64-bit target and that its
// rel32 displacement resolves to B's chainEntry.
func TestJIT_AMD64_ChainedBlocks_HighPC(t *testing.T) {
	bus, backing := phase4BusWithHighBacking(t)

	const blockA = phase7ChainBlock
	const blockB = phase7ChainBlock + 24

	// Block A: MOVE R1,#0x111 ; BRA → block B.
	phase4PlantInstrAt(backing, blockA,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x111))
	phase4PlantInstrAt(backing, blockA+8,
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 16)) // target = (blockA+8) + 16 = blockB

	// Block B: MOVE R2,#0x222 ; HALT.
	phase4PlantInstrAt(backing, blockB,
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0x222))
	phase4PlantInstrAt(backing, blockB+8,
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	cpu.jitPersist = true // keep the JIT cache alive across runs

	// Warm B: compile + cache it so A's chain slot can be patched at A's
	// compile time.
	phase7RunOnce(t, cpu, blockB)
	blkB := cpu.jitCache.Get(blockB)
	if blkB == nil {
		t.Fatal("block B not cached after warm run")
	}
	if blkB.chainEntry == 0 {
		t.Fatal("block B has no chainEntry (cannot be a chain target)")
	}

	// Clear so re-execution of both blocks is observable.
	cpu.regs[1], cpu.regs[2] = 0, 0

	// Run from A: A compiles with B already cached → A's BRA chain slot is
	// patched to B and A's first execution takes the native chain jump.
	phase7RunOnce(t, cpu, blockA)

	if cpu.regs[1] != 0x111 {
		t.Fatalf("R1 = 0x%X, want 0x111 (block A must run)", cpu.regs[1])
	}
	if cpu.regs[2] != 0x222 {
		t.Fatalf("R2 = 0x%X, want 0x222 (block B must run via the chain)", cpu.regs[2])
	}
	if cpu.PC != blockB+8 {
		t.Fatalf("cpu.PC = 0x%016X, want 0x%016X (HALT addr)", cpu.PC, uint64(blockB+8))
	}

	// The chain slot must record the full 64-bit B target (no truncation).
	blkA := cpu.jitCache.Get(blockA)
	if blkA == nil {
		t.Fatal("block A not cached")
	}
	var slot *chainSlot
	for i := range blkA.chainSlots {
		if blkA.chainSlots[i].targetPC == blockB {
			slot = &blkA.chainSlots[i]
			break
		}
	}
	if slot == nil {
		t.Fatalf("block A has no chain slot targeting full 64-bit B PC 0x%016X; slots=%+v", uint64(blockB), blkA.chainSlots)
	}

	// The slot's rel32 displacement must resolve to B's chainEntry, proving
	// the high-PC chain slot was actually patched (not left as a fallback).
	// Read the displacement through lookupWritableBytes (the same writable
	// alias PatchRel32At writes), which keeps the unsafe pointer arithmetic
	// encapsulated and vet-clean.
	b, _, ok := lookupWritableBytes(slot.patchAddr, 4)
	if !ok {
		t.Fatalf("chain slot patchAddr 0x%X not in any ExecMem", slot.patchAddr)
	}
	disp := int32(binary.LittleEndian.Uint32(b))
	dest := uintptr(int64(slot.patchAddr) + 4 + int64(disp))
	if dest != blkB.chainEntry {
		t.Fatalf("chain slot patched to 0x%X, want B.chainEntry 0x%X", dest, blkB.chainEntry)
	}
}
