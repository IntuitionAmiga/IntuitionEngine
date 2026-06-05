// jit_helper_dispatch_test.go - Phase 5 cycle 5.2 tests for the Go-side
// helper-exit dispatcher.
//
// These tests bypass the emitter and drive cpu.handleJITHelper directly.
// They prove the dispatch routing is correct so subsequent cycles can
// wire each opcode emitter to set NeedHelper without re-deriving the
// expected post-conditions.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import (
	"math"
	"testing"
	"unsafe"
)

func dispatchTestCPU(t *testing.T) *CPU64 {
	t.Helper()
	bus, _ := phase4BusWithHighBacking(t)
	cpu := NewCPU64(bus)
	if cpu.jitCtx == nil {
		// Phase 5 dispatcher reads cpu.jitCtx directly; allocate it
		// even when the JIT loop has not been entered.
		cpu.jitCtx = newJITContext(cpu)
	}
	return cpu
}

func TestHandleJITHelper_NoRequest_IsNoOp(t *testing.T) {
	cpu := dispatchTestCPU(t)
	cpu.jitCtx.NeedHelper = HELPER_NONE
	cpu.PC = 0xDEAD_BEEF
	cpu.regs[31] = 0x1000
	retired, handled := cpu.handleJITHelper()
	if handled {
		t.Fatalf("handled = true, want false")
	}
	if retired != 0 {
		t.Fatalf("retired = %d, want 0", retired)
	}
	if cpu.PC != 0xDEAD_BEEF {
		t.Fatalf("PC clobbered: %#x", cpu.PC)
	}
	if cpu.regs[31] != 0x1000 {
		t.Fatalf("SP clobbered: %#x", cpu.regs[31])
	}
}

func TestHandleJITHelper_LOAD_HighAddr_LoadsAndAdvances(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const highAddr = phase4HighPC + 0x100
	const want uint64 = 0xCAFEBABEDEADBEEF
	cpu.bus.WritePhys64WithFault(highAddr, want)
	before := globalIE64TurboStats.helperExits[HELPER_LOAD].Load()

	cpu.jitCtx.NeedHelper = HELPER_LOAD
	cpu.jitCtx.HelperAddr = highAddr
	cpu.jitCtx.HelperSize = uint32(IE64_SIZE_Q)
	cpu.jitCtx.HelperRd = 5
	cpu.jitCtx.HelperPC = phase4HighPC + 0x200
	cpu.jitCtx.LiveSP = 0x4000

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if cpu.regs[5] != want {
		t.Fatalf("R5 = %#x, want %#x", cpu.regs[5], want)
	}
	if cpu.PC != phase4HighPC+0x200+IE64_INSTR_SIZE {
		t.Fatalf("PC = %#x, want HelperPC+8", cpu.PC)
	}
	if cpu.regs[31] != 0x4000 {
		t.Fatalf("SP = %#x, want LiveSP (0x4000)", cpu.regs[31])
	}
	if cpu.jitCtx.NeedHelper != HELPER_NONE {
		t.Fatalf("NeedHelper not cleared")
	}
	after := globalIE64TurboStats.helperExits[HELPER_LOAD].Load()
	if after-before != 1 {
		t.Fatalf("HELPER_LOAD counter delta = %d, want 1", after-before)
	}
}

func TestHandleJITHelper_STORE_HighAddr_WritesBacking(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const highAddr = phase4HighPC + 0x140
	const want uint64 = 0x0123_4567_89AB_CDEF

	cpu.jitCtx.NeedHelper = HELPER_STORE
	cpu.jitCtx.HelperAddr = highAddr
	cpu.jitCtx.HelperSize = uint32(IE64_SIZE_Q)
	cpu.jitCtx.HelperVal = want
	cpu.jitCtx.HelperPC = phase4HighPC + 0x208
	cpu.jitCtx.LiveSP = 0x4000

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	got, ok := cpu.bus.ReadPhys64WithFault(highAddr)
	if !ok || got != want {
		t.Fatalf("backing[%#x] = %#x ok=%v, want %#x", highAddr, got, ok, want)
	}
	if cpu.PC != phase4HighPC+0x208+IE64_INSTR_SIZE {
		t.Fatalf("PC = %#x", cpu.PC)
	}
}

func TestHandleJITHelper_FLOAD_SetsRegisterAndConditionCodes(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const highAddr = phase4HighPC + 0x180
	const f32bits uint32 = 0x40490FDB // pi
	cpu.bus.WritePhys64WithFault(highAddr, uint64(f32bits))

	cpu.jitCtx.NeedHelper = HELPER_FLOAD
	cpu.jitCtx.HelperAddr = highAddr
	cpu.jitCtx.HelperRd = 7
	cpu.jitCtx.HelperPC = phase4HighPC + 0x210

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if cpu.FPU.FPRegs[7] != f32bits {
		t.Fatalf("FPRegs[7] = %#x, want %#x", cpu.FPU.FPRegs[7], f32bits)
	}
}

func TestHandleJITHelper_FSTORE_WritesFromFPReg(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const highAddr = phase4HighPC + 0x1C0
	const f32bits uint32 = 0x42F60000 // 123.0
	cpu.FPU.FPRegs[3] = f32bits

	cpu.jitCtx.NeedHelper = HELPER_FSTORE
	cpu.jitCtx.HelperAddr = highAddr
	cpu.jitCtx.HelperRd = 3
	cpu.jitCtx.HelperPC = phase4HighPC + 0x218

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	got, _ := cpu.bus.ReadPhys64WithFault(highAddr)
	if uint32(got) != f32bits {
		t.Fatalf("low 32 = %#x, want %#x", uint32(got), f32bits)
	}
}

func TestHandleJITHelper_DLOAD_SetsFP64Pair(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const highAddr = phase4HighPC + 0x200
	want := math.Float64bits(2.718281828459045)
	cpu.bus.WritePhys64WithFault(highAddr, want)

	cpu.jitCtx.NeedHelper = HELPER_DLOAD
	cpu.jitCtx.HelperAddr = highAddr
	cpu.jitCtx.HelperRd = 4 // valid D pair
	cpu.jitCtx.HelperPC = phase4HighPC + 0x220

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if got := math.Float64bits(cpu.FPU.getDPair(4)); got != want {
		t.Fatalf("DPair(4) = %#x, want %#x", got, want)
	}
}

func TestHandleJITHelper_DSTORE_WritesFromFP64Pair(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const highAddr = phase4HighPC + 0x240
	cpu.FPU.setDPair(6, -42.5)
	want := math.Float64bits(-42.5)

	cpu.jitCtx.NeedHelper = HELPER_DSTORE
	cpu.jitCtx.HelperAddr = highAddr
	cpu.jitCtx.HelperRd = 6
	cpu.jitCtx.HelperPC = phase4HighPC + 0x228

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	got, _ := cpu.bus.ReadPhys64WithFault(highAddr)
	if got != want {
		t.Fatalf("dword = %#x, want %#x", got, want)
	}
}

func TestHandleJITHelper_DTRANS_ExecutesFP64AndAdvances(t *testing.T) {
	cpu := dispatchTestCPU(t)
	cpu.FPU.setDPair(2, -1)
	cpu.FPU.setDPair(4, 0.5)
	before := globalIE64TurboStats.helperExits[HELPER_DTRANS].Load()

	cpu.jitCtx.NeedHelper = HELPER_DTRANS
	cpu.jitCtx.HelperSize = uint32(OP_DPOW)
	cpu.jitCtx.HelperRd = 6
	cpu.jitCtx.HelperAddr = 2
	cpu.jitCtx.HelperVal = 4
	cpu.jitCtx.HelperPC = phase4HighPC + 0x280
	cpu.jitCtx.LiveSP = 0x5000

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if !math.IsNaN(cpu.FPU.getDPair(6)) {
		t.Fatalf("DPOW result = %v, want NaN", cpu.FPU.getDPair(6))
	}
	if (cpu.FPU.FPSR & IE64_FPU_EX_IO) == 0 {
		t.Fatalf("FPSR = 0x%08X, want invalid-operation sticky flag", cpu.FPU.FPSR)
	}
	if cpu.PC != phase4HighPC+0x280+IE64_INSTR_SIZE {
		t.Fatalf("PC = %#x, want HelperPC+8", cpu.PC)
	}
	if cpu.regs[31] != 0x5000 {
		t.Fatalf("SP = %#x, want LiveSP", cpu.regs[31])
	}
	if cpu.jitCtx.NeedHelper != HELPER_NONE {
		t.Fatalf("NeedHelper not cleared")
	}
	after := globalIE64TurboStats.helperExits[HELPER_DTRANS].Load()
	if after-before != 1 {
		t.Fatalf("HELPER_DTRANS counter delta = %d, want 1", after-before)
	}
}

func TestHandleJITHelper_PUSH_HighSP_DecrementsAndWrites(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const sp = phase4HighPC + 0x300
	const val uint64 = 0x1122_3344_5566_7788

	cpu.jitCtx.NeedHelper = HELPER_PUSH
	cpu.jitCtx.HelperVal = val
	cpu.jitCtx.HelperPC = phase4HighPC + 0x400
	cpu.jitCtx.LiveSP = sp

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if cpu.regs[31] != sp-8 {
		t.Fatalf("SP = %#x, want %#x", cpu.regs[31], sp-8)
	}
	got, _ := cpu.bus.ReadPhys64WithFault(sp - 8)
	if got != val {
		t.Fatalf("stack[%#x] = %#x, want %#x", sp-8, got, val)
	}
	if cpu.PC != phase4HighPC+0x400+IE64_INSTR_SIZE {
		t.Fatalf("PC = %#x", cpu.PC)
	}
}

func TestHandleJITHelper_POP_HighSP_ReadsAndIncrements(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const sp = phase4HighPC + 0x340
	const val uint64 = 0xA5A5_5A5A_DEAD_F00D
	cpu.bus.WritePhys64WithFault(sp, val)

	cpu.jitCtx.NeedHelper = HELPER_POP
	cpu.jitCtx.HelperRd = 12
	cpu.jitCtx.HelperPC = phase4HighPC + 0x408
	cpu.jitCtx.LiveSP = sp

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if cpu.regs[12] != val {
		t.Fatalf("R12 = %#x, want %#x", cpu.regs[12], val)
	}
	if cpu.regs[31] != sp+8 {
		t.Fatalf("SP = %#x, want %#x", cpu.regs[31], sp+8)
	}
}

func TestHandleJITHelper_JSR_HighSP_PushesAndJumps(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const sp = phase4HighPC + 0x380
	const target uint64 = phase4HighPC + 0x500
	const retAddr uint64 = phase4HighPC + 0x408

	cpu.jitCtx.NeedHelper = HELPER_JSR
	cpu.jitCtx.HelperAddr = target
	cpu.jitCtx.HelperVal = retAddr
	cpu.jitCtx.HelperPC = phase4HighPC + 0x400
	cpu.jitCtx.LiveSP = sp

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if cpu.PC != target {
		t.Fatalf("PC = %#x, want %#x", cpu.PC, target)
	}
	if cpu.regs[31] != sp-8 {
		t.Fatalf("SP = %#x, want %#x", cpu.regs[31], sp-8)
	}
	got, _ := cpu.bus.ReadPhys64WithFault(sp - 8)
	if got != retAddr {
		t.Fatalf("stack[%#x] = %#x, want %#x", sp-8, got, retAddr)
	}
}

func TestHandleJITHelper_RTS_HighSP_PopsAndJumps(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const sp = phase4HighPC + 0x3C0
	const ret uint64 = phase4HighPC + 0x900
	cpu.bus.WritePhys64WithFault(sp, ret)

	cpu.jitCtx.NeedHelper = HELPER_RTS
	cpu.jitCtx.HelperPC = phase4HighPC + 0x410
	cpu.jitCtx.LiveSP = sp

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if cpu.PC != ret {
		t.Fatalf("PC = %#x, want %#x", cpu.PC, ret)
	}
	if cpu.regs[31] != sp+8 {
		t.Fatalf("SP = %#x, want %#x", cpu.regs[31], sp+8)
	}
}

func TestHandleJITHelper_JSR_IND_HighSP_PushesAndJumps(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const sp = phase4HighPC + 0x400
	const target uint64 = phase4HighPC + 0xA00
	const retAddr uint64 = phase4HighPC + 0x418

	cpu.jitCtx.NeedHelper = HELPER_JSR_IND
	cpu.jitCtx.HelperAddr = target
	cpu.jitCtx.HelperVal = retAddr
	cpu.jitCtx.HelperPC = phase4HighPC + 0x410
	cpu.jitCtx.LiveSP = sp

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if cpu.PC != target {
		t.Fatalf("PC = %#x, want %#x", cpu.PC, target)
	}
	if cpu.regs[31] != sp-8 {
		t.Fatalf("SP = %#x, want %#x", cpu.regs[31], sp-8)
	}
}

func TestHandleJITHelper_ClearsNeedHelperEvenOnFault(t *testing.T) {
	cpu := dispatchTestCPU(t)
	// Drive a LOAD that takes an MMU trap. MMU is off in
	// dispatchTestCPU, so use a phys address inside the backing — the
	// op completes cleanly. This test is only about the NeedHelper
	// reset, so use the success path; the halt-on-non-trapping path
	// is covered by TestHandleJITHelper_Stack*NonTrapping_Halts.
	cpu.bus.WritePhys64WithFault(phase4HighPC+0x700, 0xAB)
	cpu.jitCtx.NeedHelper = HELPER_LOAD
	cpu.jitCtx.HelperAddr = phase4HighPC + 0x700
	cpu.jitCtx.HelperSize = uint32(IE64_SIZE_Q)
	cpu.jitCtx.HelperRd = 1
	cpu.jitCtx.HelperPC = phase4HighPC + 0x500
	cpu.jitCtx.LiveSP = phase4HighPC + 0x600

	_, handled := cpu.handleJITHelper()
	if !handled {
		t.Fatalf("handled = false, want true")
	}
	if cpu.jitCtx.NeedHelper != HELPER_NONE {
		t.Fatalf("NeedHelper not cleared after handler")
	}
}

// Stack helpers (PUSH/POP/JSR/RTS/JSR_IND) that the bus cannot
// service (non-trapping !ok) must halt the CPU, matching the
// interpreter (cpu_ie64.go:1943, 1958, 1975, 1988, 2009). Without
// the halt the JIT loop would suppress the I/O fallback for a
// handled helper and re-enter the same block forever.

const dispatchUnmappedAddr uint64 = 0x0000_00FF_FFFF_FFF0

func TestHandleJITHelper_PUSH_NonTrappingFailure_Halts(t *testing.T) {
	cpu := dispatchTestCPU(t)
	cpu.running.Store(true)
	cpu.jitCtx.NeedHelper = HELPER_PUSH
	cpu.jitCtx.HelperVal = 0xDEAD
	cpu.jitCtx.HelperPC = phase4HighPC + 0x500
	cpu.jitCtx.LiveSP = dispatchUnmappedAddr

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 0 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if cpu.running.Load() {
		t.Fatalf("running still true after non-trapping PUSH failure")
	}
	if cpu.trapped {
		t.Fatalf("trapped set unexpectedly")
	}
	// Interpreter (cpu_ie64.go:1966-1978) pre-decrements SP and only
	// rolls back on trapping faults. For a non-trapping halt the
	// architectural SP of the halted CPU must match the interpreter.
	if cpu.regs[31] != dispatchUnmappedAddr-8 {
		t.Fatalf("SP = %#x, want %#x (pre-decrement preserved)",
			cpu.regs[31], dispatchUnmappedAddr-8)
	}
}

// Low-window stack helper must hit the direct memBase fast path
// (cpu_ie64.go:296-298) rather than the bus phys helper, so RAM
// stores land in cpu.memory[] and MMIO callbacks on low-window
// pages are not spuriously fired.
func TestHandleJITHelper_PUSH_LowSP_UsesDirectFastPath(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const sp uint64 = 0x0010_0000
	const val uint64 = 0xFEED_FACE_5555_AAAA
	cpu.running.Store(true)
	cpu.jitCtx.NeedHelper = HELPER_PUSH
	cpu.jitCtx.HelperVal = val
	cpu.jitCtx.HelperPC = 0x4000
	cpu.jitCtx.LiveSP = sp

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if cpu.regs[31] != sp-8 {
		t.Fatalf("SP = %#x, want %#x", cpu.regs[31], sp-8)
	}
	// Direct fast path writes the bytes into cpu.memory[] — read
	// them back without going through the bus to prove no bus
	// helper was involved.
	got := *(*uint64)(unsafe.Pointer(&cpu.memory[sp-8]))
	if got != val {
		t.Fatalf("cpu.memory[%#x] = %#x, want %#x", sp-8, got, val)
	}
}

func TestHandleJITHelper_POP_LowSP_UsesDirectFastPath(t *testing.T) {
	cpu := dispatchTestCPU(t)
	const sp uint64 = 0x0010_1000
	const want uint64 = 0xCAFEBABE_DEADBEEF
	*(*uint64)(unsafe.Pointer(&cpu.memory[sp])) = want
	cpu.running.Store(true)
	cpu.jitCtx.NeedHelper = HELPER_POP
	cpu.jitCtx.HelperRd = 9
	cpu.jitCtx.HelperPC = 0x4008
	cpu.jitCtx.LiveSP = sp

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if cpu.regs[9] != want {
		t.Fatalf("R9 = %#x, want %#x", cpu.regs[9], want)
	}
	if cpu.regs[31] != sp+8 {
		t.Fatalf("SP = %#x, want %#x", cpu.regs[31], sp+8)
	}
}

func TestHandleJITHelper_JSR_NonTrappingFailure_PreservesPredecrement(t *testing.T) {
	cpu := dispatchTestCPU(t)
	cpu.running.Store(true)
	cpu.jitCtx.NeedHelper = HELPER_JSR
	cpu.jitCtx.HelperAddr = phase4HighPC + 0xC00
	cpu.jitCtx.HelperVal = phase4HighPC + 0x510
	cpu.jitCtx.HelperPC = phase4HighPC + 0x508
	cpu.jitCtx.LiveSP = dispatchUnmappedAddr

	if _, handled := cpu.handleJITHelper(); !handled {
		t.Fatalf("handled = false")
	}
	if cpu.running.Load() {
		t.Fatalf("running still true")
	}
	if cpu.regs[31] != dispatchUnmappedAddr-8 {
		t.Fatalf("SP = %#x, want %#x", cpu.regs[31], dispatchUnmappedAddr-8)
	}
}

func TestHandleJITHelper_JSR_IND_NonTrappingFailure_PreservesPredecrement(t *testing.T) {
	cpu := dispatchTestCPU(t)
	cpu.running.Store(true)
	cpu.jitCtx.NeedHelper = HELPER_JSR_IND
	cpu.jitCtx.HelperAddr = phase4HighPC + 0xD00
	cpu.jitCtx.HelperVal = phase4HighPC + 0x520
	cpu.jitCtx.HelperPC = phase4HighPC + 0x518
	cpu.jitCtx.LiveSP = dispatchUnmappedAddr

	if _, handled := cpu.handleJITHelper(); !handled {
		t.Fatalf("handled = false")
	}
	if cpu.running.Load() {
		t.Fatalf("running still true")
	}
	if cpu.regs[31] != dispatchUnmappedAddr-8 {
		t.Fatalf("SP = %#x, want %#x", cpu.regs[31], dispatchUnmappedAddr-8)
	}
}

func TestHandleJITHelper_POP_NonTrappingFailure_Halts(t *testing.T) {
	cpu := dispatchTestCPU(t)
	cpu.running.Store(true)
	cpu.jitCtx.NeedHelper = HELPER_POP
	cpu.jitCtx.HelperRd = 5
	cpu.jitCtx.HelperPC = phase4HighPC + 0x508
	cpu.jitCtx.LiveSP = dispatchUnmappedAddr

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 0 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	if cpu.running.Load() {
		t.Fatalf("running still true after non-trapping POP failure")
	}
}

func TestHandleJITHelper_JSR_NonTrappingFailure_Halts(t *testing.T) {
	cpu := dispatchTestCPU(t)
	cpu.running.Store(true)
	cpu.jitCtx.NeedHelper = HELPER_JSR
	cpu.jitCtx.HelperAddr = phase4HighPC + 0xC00
	cpu.jitCtx.HelperVal = phase4HighPC + 0x510
	cpu.jitCtx.HelperPC = phase4HighPC + 0x508
	cpu.jitCtx.LiveSP = dispatchUnmappedAddr

	if _, handled := cpu.handleJITHelper(); !handled {
		t.Fatalf("handled = false")
	}
	if cpu.running.Load() {
		t.Fatalf("running still true after non-trapping JSR failure")
	}
}

// FPU fault path (FPU=nil or invalid pair register) must halt to
// match the interpreter's fpu_missing / invalid_freg labels.
func TestHandleJITHelper_FLOAD_NoFPU_Halts(t *testing.T) {
	cpu := dispatchTestCPU(t)
	cpu.running.Store(true)
	cpu.FPU = nil
	cpu.jitCtx.NeedHelper = HELPER_FLOAD
	cpu.jitCtx.HelperAddr = phase4HighPC + 0x800
	cpu.jitCtx.HelperRd = 2
	cpu.jitCtx.HelperPC = phase4HighPC + 0x510

	if _, handled := cpu.handleJITHelper(); !handled {
		t.Fatalf("handled = false")
	}
	if cpu.running.Load() {
		t.Fatalf("running still true after FPU=nil FLOAD")
	}
}

// LOAD into R0 must be a no-op exactly like the interpreter
// (cpu_ie64.go:1682-1690). The dispatcher must NOT call loadMem at
// all, so MMIO read side effects and faults at the target address
// are suppressed when the destination is R0.
// LOAD fault must clobber Rd to 0 like the interpreter
// (cpu_ie64.go:1683-1689): cpu.regs[rd] is written from loadMem's
// zero return BEFORE the trapped check, so the trap handler
// observes the destination cleared. Helper must match.
func TestHandleJITHelper_LOAD_Fault_ClobbersRdToZero(t *testing.T) {
	cpu := dispatchTestCPU(t)
	cpu.running.Store(true)
	cpu.regs[7] = 0xCAFEBABE_DEADBEEF // sentinel that MUST be wiped
	// Force MMU translation fault: enable MMU with PTBR=0, so the
	// first page-table read returns zero (PTE_P clear) →
	// translateAddr returns FAULT_NOT_PRESENT and trapFault fires
	// while loadMem returns 0.
	cpu.mmuEnabled = true
	cpu.ptbr = 0

	cpu.jitCtx.NeedHelper = HELPER_LOAD
	cpu.jitCtx.HelperAddr = 0x1000
	cpu.jitCtx.HelperSize = uint32(IE64_SIZE_Q)
	cpu.jitCtx.HelperRd = 7
	cpu.jitCtx.HelperPC = 0x4000

	retired, handled := cpu.handleJITHelper()
	if !handled {
		t.Fatalf("handled = false")
	}
	if retired != 0 {
		t.Fatalf("retired = %d, want 0 (faulted instruction not counted)", retired)
	}
	if cpu.trapped {
		t.Fatalf("trapped still set after dispatcher")
	}
	if cpu.regs[7] != 0 {
		t.Fatalf("R7 = %#x, want 0 (interpreter clobbers Rd before trap check)",
			cpu.regs[7])
	}
}

func TestHandleJITHelper_LOAD_R0_DoesNotCallLoadMem(t *testing.T) {
	cpu := dispatchTestCPU(t)
	// Use an address well outside the 8 GiB SparseBacking so any
	// actual loadMem call would either fault via MMU or return a
	// zero from an unmapped bus dispatch — either way distinguishable
	// from "not called".
	const unmapped uint64 = 0x0000_00FF_FFFF_FFF0
	cpu.regs[0] = 0xCAFEBABE // sentinel that must survive
	cpu.running.Store(true)

	cpu.jitCtx.NeedHelper = HELPER_LOAD
	cpu.jitCtx.HelperAddr = unmapped
	cpu.jitCtx.HelperSize = uint32(IE64_SIZE_Q)
	cpu.jitCtx.HelperRd = 0
	cpu.jitCtx.HelperPC = phase4HighPC + 0x600

	retired, handled := cpu.handleJITHelper()
	if !handled || retired != 1 {
		t.Fatalf("handled=%v retired=%d", handled, retired)
	}
	// R0 is hard-wired zero on read in IE64; setReg(0, ...) is a
	// no-op. The sentinel survives because setReg was never invoked
	// — but more importantly, no fault was raised and PC advanced.
	if cpu.trapped {
		t.Fatalf("trapped set — loadMem must not have been called")
	}
	if !cpu.running.Load() {
		t.Fatalf("running cleared — loadMem must not have been called")
	}
	if cpu.PC != phase4HighPC+0x600+IE64_INSTR_SIZE {
		t.Fatalf("PC = %#x, want HelperPC+8", cpu.PC)
	}
}

func TestHandleJITHelper_DLOAD_InvalidPair_Halts(t *testing.T) {
	cpu := dispatchTestCPU(t)
	cpu.running.Store(true)
	cpu.jitCtx.NeedHelper = HELPER_DLOAD
	cpu.jitCtx.HelperAddr = phase4HighPC + 0x880
	cpu.jitCtx.HelperRd = 5 // odd / not a valid D pair index
	cpu.jitCtx.HelperPC = phase4HighPC + 0x518

	if _, handled := cpu.handleJITHelper(); !handled {
		t.Fatalf("handled = false")
	}
	if cpu.running.Load() {
		t.Fatalf("running still true after invalid D pair DLOAD")
	}
}
