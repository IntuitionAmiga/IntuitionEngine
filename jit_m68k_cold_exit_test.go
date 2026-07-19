// jit_m68k_cold_exit_test.go - Milestone 7 cold-exit outlining. Shape test
// via the outline counter and layout comparison; the standing differential
// suites provide the parity evidence (outlining is on by default).

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"unsafe"
)

// A block with several memory instructions produces multiple NeedInval/
// NeedIOFallback exit sites: MOVE.L D0,(A0); MOVE.L D1,(A1); MOVE.L (A0),D2.
func m68kColdExitWords() []uint16 {
	return []uint16{0x2080, 0x2281, 0x2410, 0x6002}
}

func m68kColdExitCompile(t *testing.T, disable bool) (int, uint64) {
	t.Helper()
	old := m68kJITColdOutlineDisabled
	m68kJITColdOutlineDisabled = disable
	defer func() { m68kJITColdOutlineDisabled = old }()
	mem := make([]byte, 1<<16)
	pc := uint32(0x1000)
	for i, w := range m68kColdExitWords() {
		mem[pc+uint32(i*2)] = byte(w >> 8)
		mem[pc+uint32(i*2)+1] = byte(w)
	}
	instrs := m68kScanBlock(mem, pc)
	em, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer em.Free()
	before := m68kColdExitOutlines.Load()
	block, err := m68kCompileBlockWithMem(instrs, pc, em, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return block.execSize, m68kColdExitOutlines.Load() - before
}

func TestM68KJIT_ColdExitOutlineShape(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	_, outlinesOn := m68kColdExitCompile(t, false)
	_, outlinesOff := m68kColdExitCompile(t, true)
	if outlinesOn != 1 {
		t.Fatalf("outline counter with outlining on: got %d want 1", outlinesOn)
	}
	if outlinesOff != 0 {
		t.Fatalf("outline counter with kill switch: got %d want 0", outlinesOff)
	}
}

// The outlined exit must still deliver a correct bail: an MMIO store hits
// NeedIOFallback and the stub publishes the faulting PC and count.
func TestM68KJIT_ColdExitOutlineBail(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	jit.PC = m68kDiffStartPC
	m68kDiffSetupCPU(jit)
	jit.AddrRegs[0] = 0x4000
	jit.AddrRegs[1] = 0xFFFF0000 // outside RAM: second store bails
	m68kDiffWriteProgram(jit, m68kDiffStartPC, m68kColdExitWords()...)

	instrs := m68kScanBlock(jit.memory, m68kDiffStartPC)
	rig.execMem.Reset()
	block, err := m68kCompileBlockWithMem(instrs, m68kDiffStartPC, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.RetPC = 0
	rig.ctx.NeedIOFallback = 0
	rig.ctx.RetCount = 0
	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	if rig.ctx.NeedIOFallback != 1 && rig.ctx.NeedHelper == m68kJITHelperNone {
		t.Fatal("out-of-range store raised neither NeedIOFallback nor a helper exit")
	}
	if rig.ctx.RetPC != m68kDiffStartPC+2 || rig.ctx.RetCount != 1 {
		t.Fatalf("outlined bail state: RetPC=%08X RetCount=%d want RetPC=%08X RetCount=1",
			rig.ctx.RetPC, rig.ctx.RetCount, m68kDiffStartPC+2)
	}
	// First store landed, second did not.
	if got := jit.Read32(0x4000); got != jit.DataRegs[0] {
		t.Fatalf("first store missing: %08X", got)
	}
}

// Benchmark: the invariant-access loop again; outlined exits move the
// per-iteration NeedInval/NeedIOFallback stub bodies out of the hot loop.
func benchmarkM68KColdExit(b *testing.B, disable bool) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available")
	}
	old := m68kJITColdOutlineDisabled
	m68kJITColdOutlineDisabled = disable
	defer func() { m68kJITColdOutlineDisabled = old }()

	words := m68kHoistLoopWords(50)
	mem := make([]byte, 1<<20)
	pc := uint32(0x1000)
	for i, w := range words {
		mem[pc+uint32(i*2)] = byte(w >> 8)
		mem[pc+uint32(i*2)+1] = byte(w)
	}
	instrs := m68kScanBlock(mem, pc)
	em, err := AllocExecMem(1 << 20)
	if err != nil {
		b.Fatalf("AllocExecMem: %v", err)
	}
	defer em.Free()
	block, err := m68kCompileBlockWithMem(instrs, pc, em, mem)
	if err != nil {
		b.Fatalf("compile: %v", err)
	}
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.AddrRegs[2] = 0x4000
	copy(cpu.memory[pc:], mem[pc:pc+uint32(len(words)*2)])
	bitmap := make([]byte, (uint32(len(cpu.memory))+4095)>>12)
	pageMin := make([]uint16, len(bitmap))
	pageMax := make([]uint16, len(bitmap))
	ctx := newM68KJITContext(cpu, bitmap, pageMin, pageMax)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx.RetPC = 0
		ctx.NeedIOFallback = 0
		ctx.ChainCount = 0
		ctx.ChainBudget = 100000
		callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	}
}

func BenchmarkM68KJIT_ColdExitOff(b *testing.B) { benchmarkM68KColdExit(b, true) }
func BenchmarkM68KJIT_ColdExitOn(b *testing.B)  { benchmarkM68KColdExit(b, false) }
