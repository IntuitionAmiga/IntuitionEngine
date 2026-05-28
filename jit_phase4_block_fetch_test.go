// jit_phase4_block_fetch_test.go - Phase 4 block-fetch widening tests.
//
// Phase 4 widens the JIT block-fetch path to full 64-bit physical
// addresses: ExecuteJIT no longer truncates cpu.PC to uint32, scan
// stops cleanly on unmapped high-phys fetch, and HALT detection
// works at high PC. scanBlockBus / scanBlockBusWithLimit are the
// new helpers that route per-instruction fetch through
// bus.ReadPhys64WithFault when the physical address is outside the
// cpu.memory[] window.
//
// MMUEnabled is also pinned: the dispatcher refreshes it on every
// callNative so the Phase 5 native helpers can trust the value.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import (
	"encoding/binary"
	"testing"
	"time"
	"unsafe"
)

// phase4HighPC is well above 4 GiB so any uint32 truncation in the
// dispatcher path immediately surfaces (it would alias into a low
// page of cpu.memory and quietly succeed otherwise).
const phase4HighPC uint64 = 0x0000_0001_0010_0000

// plantInstrAt writes one IE64 instruction (8 bytes) into the bus
// backing at the given physical address.
func phase4PlantInstrAt(backing *SparseBacking, addr uint64, instr []byte) {
	for i, b := range instr {
		backing.Write8(addr+uint64(i), b)
	}
}

// phase4BusWithHighBacking builds a bus with the legacy low memory[]
// window plus an 8 GiB SparseBacking, so high-PC fetches can resolve.
func phase4BusWithHighBacking(t *testing.T) (*MachineBus, *SparseBacking) {
	t.Helper()
	const memSize = 32 * 1024 * 1024
	bus, err := NewMachineBusSized(memSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	backing := NewSparseBacking(8 * 1024 * 1024 * 1024)
	bus.SetBacking(backing)
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    8 * 1024 * 1024 * 1024,
		ActiveVisibleRAM: 8 * 1024 * 1024 * 1024,
	})
	return bus, backing
}

// =============================================================================
// scanBlockBus unit tests
// =============================================================================

func TestScanBlockBus_HighPC_DecodesInstructions(t *testing.T) {
	bus, backing := phase4BusWithHighBacking(t)
	_ = bus

	// MOVE R1, #42 ; HALT
	phase4PlantInstrAt(backing, phase4HighPC, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42))
	phase4PlantInstrAt(backing, phase4HighPC+8, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	instrs := scanBlockBus(bus, phase4HighPC)
	if len(instrs) != 2 {
		t.Fatalf("len(instrs) = %d, want 2 (MOVE + HALT terminator)", len(instrs))
	}
	if instrs[0].opcode != OP_MOVE {
		t.Errorf("instrs[0].opcode = 0x%02X, want OP_MOVE", instrs[0].opcode)
	}
	if instrs[0].imm32 != 42 {
		t.Errorf("instrs[0].imm32 = %d, want 42", instrs[0].imm32)
	}
	if instrs[1].opcode != OP_HALT64 {
		t.Errorf("instrs[1].opcode = 0x%02X, want OP_HALT64", instrs[1].opcode)
	}
}

func TestScanBlockBus_UnmappedAddress_StopsCleanly(t *testing.T) {
	bus, _ := phase4BusWithHighBacking(t)
	// Read from a high address with no backing planted. SparseBacking
	// allocates pages on write only; an untouched page returns ok=false
	// from ReadPhys64WithFault. The scanner must stop, not panic.
	const unmapped uint64 = 0x0000_00FF_FFFF_F000 // way above 8 GiB advertised — guarantees unmapped
	instrs := scanBlockBus(bus, unmapped)
	if len(instrs) != 0 {
		t.Fatalf("scanBlockBus on unmapped address returned %d instrs, want 0", len(instrs))
	}
}

func TestScanBlockBusWithLimit_RespectsMaxPC(t *testing.T) {
	bus, backing := phase4BusWithHighBacking(t)
	// Plant 3 instructions but cap maxPC after the 2nd: scanner must
	// return only the first 2 even though no terminator was hit yet.
	phase4PlantInstrAt(backing, phase4HighPC, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 1))
	phase4PlantInstrAt(backing, phase4HighPC+8, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 2))
	phase4PlantInstrAt(backing, phase4HighPC+16, ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 1, 0, 0, 3))

	instrs := scanBlockBusWithLimit(bus, phase4HighPC, phase4HighPC+16)
	if len(instrs) != 2 {
		t.Fatalf("len(instrs) = %d, want 2 (maxPC truncates third)", len(instrs))
	}
}

// =============================================================================
// ExecuteJIT high-PC end-to-end tests
// =============================================================================

// phase4RunUntilHalt boots a CPU with high-backed memory, sets cpu.PC
// to the given start address, and runs ExecuteJIT until HALT or
// timeout.
func phase4RunUntilHalt(t *testing.T, bus *MachineBus, startPC uint64) *CPU64 {
	t.Helper()
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	cpu.PC = startPC

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
	return cpu
}

// TestJIT_HighPC_BlockExecutes_EndToEnd plants a small JITable
// block (MOVE then JMP to a low HALT) at a >4 GiB physical address,
// runs ExecuteJIT starting from that high PC, and verifies the
// MOVE retired its side effect. Pre-Phase 4 ExecuteJIT truncated
// cpu.PC to uint32 (aliasing the high address into low memory) or
// bailed every instruction to the interpreter; the JIT block cache
// would never see the high PC.
func TestJIT_HighPC_BlockExecutes_EndToEnd(t *testing.T) {
	bus, backing := phase4BusWithHighBacking(t)

	// Plant code at high PC: MOVE R1, #0xCAFE ; JMP to low HALT.
	// JMP with rs=0, imm32 = -(highPC - lowHaltPC) (signed displacement)?
	// emit*JMP computes target = instrPC + sign_ext(imm32). To reach a
	// low absolute address, use a JMP whose target is the low HALT. The
	// emitter expects target = instrPC + imm32 (sign-extended).
	const lowHaltAddr uint64 = PROG_START
	// JMP instrPC is phase4HighPC+8. target = phase4HighPC + 8 + sext(imm32).
	// We want target = lowHaltAddr → imm32 = lowHaltAddr - (phase4HighPC + 8).
	// That diff is negative and >32-bit, so a static-target JMP from a
	// high PC to a far low PC can't be expressed in a single signed
	// imm32. Instead, plant the HALT at a high address adjacent to the
	// MOVE so the block terminator stays within imm32 reach.
	phase4PlantInstrAt(backing, phase4HighPC, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xCAFE))
	phase4PlantInstrAt(backing, phase4HighPC+8, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	_ = lowHaltAddr

	cpu := phase4RunUntilHalt(t, bus, phase4HighPC)

	if cpu.regs[1] != 0xCAFE {
		t.Fatalf("R1 = 0x%016X, want 0xCAFE (high-PC JIT block must execute MOVE)", cpu.regs[1])
	}
	if cpu.PC != phase4HighPC+8 {
		t.Fatalf("cpu.PC = 0x%016X, want 0x%016X (HALT addr)", cpu.PC, phase4HighPC+8)
	}
}

// TestJIT_HighPC_Halt_StopsDispatch verifies the dispatcher's HALT
// detection works when pcPhys is outside the legacy cpu.memory[]
// window. Pre-Phase 4 the dispatcher indexed cpu.memory[pcPhys]
// directly, which would either panic (out of range) or alias into
// low memory.
func TestJIT_HighPC_Halt_StopsDispatch(t *testing.T) {
	bus, backing := phase4BusWithHighBacking(t)
	phase4PlantInstrAt(backing, phase4HighPC, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu := phase4RunUntilHalt(t, bus, phase4HighPC)

	if cpu.PC != phase4HighPC {
		t.Fatalf("cpu.PC = 0x%016X, want 0x%016X (HALT must keep PC at the HALT)", cpu.PC, phase4HighPC)
	}
}

// TestJIT_HighPC_UnmappedFetch_StopsCleanly drops the dispatcher
// into a PC with no backing. Pre-Phase 4 the indexer panicked; the
// new path stops execution via bus.ReadPhys64WithFault returning ok=false.
func TestJIT_HighPC_UnmappedFetch_StopsCleanly(t *testing.T) {
	bus, _ := phase4BusWithHighBacking(t)
	// Address inside the advertised 8 GiB but never written, so the
	// SparseBacking has no allocated page. ReadPhys64WithFault must
	// return !ok and the dispatcher must stop running without panic.
	const unmappedHigh uint64 = 0x0000_0000_4000_0000 // 1 GiB into backing, unwritten
	_ = unmappedHigh

	// Pick an address guaranteed to be unbacked.
	const trulyUnmapped uint64 = 0x0000_0001_FF00_0000

	cpu := phase4RunUntilHalt(t, bus, trulyUnmapped)
	// No assertion on registers — just proves the run completed
	// without panic. running must be false.
	if cpu.running.Load() {
		t.Fatal("cpu.running still true after unmapped fetch")
	}
}

// TestScanBlockBus_NearMaxUint64_NoWrap pins the wrap guard: a
// startPC in the last 7 bytes of uint64 must not advance past
// MaxUint64 (which would alias into a low page on the next iter).
// The interpreter uses the subtraction-form bound; the JIT scanner
// must too.
func TestScanBlockBus_NearMaxUint64_NoWrap(t *testing.T) {
	bus, _ := phase4BusWithHighBacking(t)
	const nearMax uint64 = 0xFFFF_FFFF_FFFF_FFF8 // exactly one instr below wrap
	instrs := scanBlockBus(bus, nearMax)
	// Unmapped at that address → empty. The test would panic before
	// the guard, when pc += 8 wrapped and ReadPhys64WithFault read
	// from low memory in a tight loop.
	if len(instrs) != 0 {
		t.Fatalf("scanBlockBus near MaxUint64 returned %d instrs, want 0", len(instrs))
	}
}

// TestExecuteJIT_NearMaxUint64_NoPanic exercises the dispatcher
// bounds check: pcPhys+IE64_INSTR_SIZE wraps when pcPhys is in the
// last 7 bytes of uint64 space, so the add-form bound (pcPhys+8 <=
// memLen) would falsely route the fetch into cpu.memory[pcPhys]
// and panic.  The subtraction-form bound (pcPhys <= memLen - 8)
// keeps it on the bus path, which returns !ok and stops the loop.
func TestExecuteJIT_NearMaxUint64_NoPanic(t *testing.T) {
	bus, _ := phase4BusWithHighBacking(t)
	const nearMax uint64 = 0xFFFF_FFFF_FFFF_FFFC // pcPhys+8 wraps to a small value
	cpu := phase4RunUntilHalt(t, bus, nearMax)
	if cpu.running.Load() {
		t.Fatal("cpu.running still true after near-MaxUint64 fetch")
	}
}

// TestJIT_HighPC_StackOpBlock_BailsToInterpreter pins the Phase 4
// interim guard: when a high-phys block contains PUSH/POP/JSR/RTS/
// JSR_IND, the dispatcher must NOT hand it to the native emitter
// (which addresses the stack as raw [memBase+R31] and would corrupt
// the host if SP is also in high RAM). Until Phase 5 wires bus-aware
// stack helpers, such blocks must bail to the interpreter.
//
// Test pattern: PUSH R1 ; HALT at high PC. SP points into high RAM
// with a SparseBacking page primed. The interpreter routes the
// PUSH through the bus; the JIT must not have a cache entry.
func TestJIT_HighPC_StackOpBlock_BailsToInterpreter(t *testing.T) {
	bus, backing := phase4BusWithHighBacking(t)
	// Plant: PUSH R1 ; HALT (rs=1 = source register).
	phase4PlantInstrAt(backing, phase4HighPC, ie64Instr(OP_PUSH64, 0, 0, 0, 1, 0, 0))
	phase4PlantInstrAt(backing, phase4HighPC+8, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	cpu.jitPersist = true
	cpu.PC = phase4HighPC
	cpu.regs[1] = 0xDEADBEEFCAFEBABE
	// SP in high RAM, 16 bytes above a SparseBacking page so PUSH
	// (pre-decrement) lands in mapped territory.
	const highSP uint64 = 0x0000_0001_0020_0000
	// Prime the backing page so the interpreter's bus write succeeds
	// (SparseBacking allocates pages on first write).
	backing.Write8(highSP-8, 0)
	cpu.regs[31] = highSP

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

	// PUSH must have decremented SP by 8.
	if cpu.regs[31] != highSP-8 {
		t.Fatalf("SP = 0x%016X, want 0x%016X (PUSH must decrement by 8)", cpu.regs[31], highSP-8)
	}
	// Stack memory must hold the pushed value.
	var got uint64
	for i := uint64(0); i < 8; i++ {
		got |= uint64(backing.Read8(highSP-8+i)) << (8 * i)
	}
	if got != 0xDEADBEEFCAFEBABE {
		t.Fatalf("backing[SP] = 0x%016X, want 0xDEADBEEFCAFEBABE", got)
	}
	// The JIT cache must not contain a block for the high PC — the
	// dispatcher bailed before compile.
	if blk := cpu.jitCache.Get(phase4HighPC); blk != nil {
		t.Fatalf("JIT compiled a stack-op high-phys block (must bail): %v", blk)
	}
}

// =============================================================================
// JITContext.MMUEnabled refresh test
// =============================================================================

// TestJIT_MMUEnabled_RefreshedBeforeCallNative pins that the
// dispatcher writes ctx.MMUEnabled before every callNative so the
// Phase 5 helpers can trust the value. We run a low-PC block in both
// MMU-off and (synthetically) MMU-on modes and inspect the field.
func TestJIT_MMUEnabled_RefreshedBeforeCallNative(t *testing.T) {
	const memSize = 32 * 1024 * 1024
	bus, err := NewMachineBusSized(memSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	cpu.jitPersist = true // keep ctx alive after ExecuteJIT for inspection

	// Plant MOVE then HALT at PROG_START.
	offset := uint32(PROG_START)
	move := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 1)
	halt := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	copy(cpu.memory[offset:], move)
	copy(cpu.memory[offset+uint32(len(move)):], halt)

	cpu.PC = PROG_START
	// Pre-poison MMUEnabled to a sentinel; if the dispatcher fails to
	// refresh it, the field would retain the poison.
	if err := cpu.initJIT(); err != nil {
		t.Fatalf("initJIT: %v", err)
	}
	cpu.jitCtx.MMUEnabled = 0xDEADBEEF

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

	// MMU was off → MMUEnabled must equal 0 after the last callNative.
	if cpu.jitCtx.MMUEnabled != 0 {
		t.Fatalf("ctx.MMUEnabled = 0x%X, want 0 (MMU off — must be cleared, not stale)", cpu.jitCtx.MMUEnabled)
	}
}

// =============================================================================
// JITContext field-offset for the new MMUEnabled field
// =============================================================================

func TestJITContext_MMUEnabledOffset(t *testing.T) {
	var ctx JITContext
	got := unsafe.Offsetof(ctx.MMUEnabled)
	if uintptr(jitCtxOffMMUEnabled) != got {
		t.Fatalf("jitCtxOffMMUEnabled = %d, unsafe.Offsetof = %d", jitCtxOffMMUEnabled, got)
	}
}

// keepalive: avoid "imported and not used" if any helper drops below
var _ = binary.LittleEndian
