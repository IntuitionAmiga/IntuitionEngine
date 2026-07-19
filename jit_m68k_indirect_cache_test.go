// jit_m68k_indirect_cache_test.go - Milestone 7 indirect-target
// specialisation: dynamic JMP/JSR targets probe the target-keyed MRU and
// chain natively into the cached successor.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"unsafe"
)

// TestM68KJIT_IndirectTargetChain: JMP (A0) chains into the cached block
// without returning to the dispatcher.
func TestM68KJIT_IndirectTargetChain(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	t.Setenv("IE_M68K_JIT_ENABLE_RTS_CACHE", "1")
	if !m68kIndirectCacheEnabled() {
		t.Skip("indirect cache disabled")
	}
	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	m68kDiffSetupCPU(jit)

	const jmpPC = uint32(0x1000)
	const targetPC = uint32(0x2000)
	m68kDiffWriteProgram(jit, jmpPC, 0x4ED0)            // JMP (A0)
	m68kDiffWriteProgram(jit, targetPC, 0x7007, 0x6002) // MOVEQ #7,D0 ; BRA.S
	jit.AddrRegs[0] = targetPC

	rig.execMem.Reset()
	instrsB := m68kScanBlock(jit.memory, targetPC)
	blockB, err := m68kCompileBlockWithMem(instrsB, targetPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("compile B: %v", err)
	}
	if blockB.chainEntry == 0 {
		t.Fatal("target block has no chain entry")
	}
	instrsA := m68kScanBlock(jit.memory, jmpPC)
	before := m68kIndirectCacheEmits.Load()
	blockA, err := m68kCompileBlockWithMem(instrsA, jmpPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("compile A: %v", err)
	}
	if m68kIndirectCacheEmits.Load() == before {
		t.Fatal("JMP (An) block compiled without the target-cache probe")
	}

	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.RTSCache0PC = targetPC
	rig.ctx.RTSCache0Addr = blockB.chainEntry
	rig.ctx.ChainBudget = 1000
	rig.ctx.ChainCount = 0
	rig.ctx.RetPC = 0
	rig.ctx.NeedIOFallback = 0
	callNative(blockA.execAddr, uintptr(unsafe.Pointer(rig.ctx)))

	if jit.DataRegs[0] != 7 {
		t.Fatalf("chained target did not execute: D0=%08X", jit.DataRegs[0])
	}
	if rig.ctx.ChainCount == 0 {
		t.Fatal("no chained transfer accounted")
	}
	// Retired total: JMP (1, in ChainCount) + MOVEQ+BRA (blockB): dispatcher
	// contract is ChainCount + RetCount.
	total := uint64(rig.ctx.ChainCount) + uint64(rig.ctx.RetCount)
	if total != 3 {
		t.Fatalf("retired total: got %d want 3 (ChainCount=%d RetCount=%d)",
			total, rig.ctx.ChainCount, rig.ctx.RetCount)
	}
}

// Miss path: an uncached target exits unchained with the committed target
// PC and full count.
func TestM68KJIT_IndirectTargetMiss(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	t.Setenv("IE_M68K_JIT_ENABLE_RTS_CACHE", "1")
	if !m68kIndirectCacheEnabled() {
		t.Skip("indirect cache disabled")
	}
	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	m68kDiffSetupCPU(jit)
	const jmpPC = uint32(0x1000)
	m68kDiffWriteProgram(jit, jmpPC, 0x4ED0) // JMP (A0)
	jit.AddrRegs[0] = 0x3000

	rig.execMem.Reset()
	instrs := m68kScanBlock(jit.memory, jmpPC)
	block, err := m68kCompileBlockWithMem(instrs, jmpPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.ChainBudget = 1000
	rig.ctx.RetPC = 0
	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	if rig.ctx.RetPC != 0x3000 || rig.ctx.RetCount != 1 {
		t.Fatalf("miss exit: RetPC=%08X RetCount=%d want 3000/1", rig.ctx.RetPC, rig.ctx.RetCount)
	}
}

// Odd target: must not probe; unchained exit hands the odd PC to the
// dispatcher for the architectural address-error path.
func TestM68KJIT_IndirectTargetOdd(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	m68kDiffSetupCPU(jit)
	const jmpPC = uint32(0x1000)
	m68kDiffWriteProgram(jit, jmpPC, 0x4ED0)
	jit.AddrRegs[0] = 0x3001

	rig.execMem.Reset()
	instrs := m68kScanBlock(jit.memory, jmpPC)
	block, err := m68kCompileBlockWithMem(instrs, jmpPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.ChainBudget = 1000
	rig.ctx.RetPC = 0
	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	if rig.ctx.RetPC != 0x3001 || rig.ctx.RetCount != 1 {
		t.Fatalf("odd-target exit: RetPC=%08X RetCount=%d want 3001/1", rig.ctx.RetPC, rig.ctx.RetCount)
	}
}
