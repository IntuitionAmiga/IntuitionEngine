// jit_m68k_loop_hoist_test.go - Milestone 7 invariant memory-check
// hoisting. Analysis tests for the invariance proof, a shape test on the
// hoist counter and code size, and parity with dynamic retired counts.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"unsafe"
)

// MOVEQ #N,D1 ; loop: MOVE.W 8(A2),D0 ; MOVE.W D0,12(A2) ; DBRA D1 ; tail.
func m68kHoistLoopWords(n uint16) []uint16 {
	return []uint16{0x7200 | n, 0x302A, 0x0008, 0x3540, 0x000C, 0x51C9, 0xFFF6, 0x6002}
}

func m68kHoistScan(t *testing.T, words ...uint16) ([]M68KJITInstr, []byte, uint32) {
	t.Helper()
	mem := make([]byte, 1<<16)
	pc := uint32(0x1000)
	for i, w := range words {
		mem[pc+uint32(i*2)] = byte(w >> 8)
		mem[pc+uint32(i*2)+1] = byte(w)
	}
	instrs := m68kScanBlock(mem, pc)
	if len(instrs) == 0 {
		t.Fatal("scan produced no instructions")
	}
	return instrs, mem, pc
}

func TestM68KJIT_LoopHoistAnalysis(t *testing.T) {
	// Invariant A2 base: both accesses hoist, both body MOVEs elide.
	instrs, mem, pc := m68kHoistScan(t, m68kHoistLoopWords(10)...)
	plan := m68kAnalyseLoopInvariantGuards(instrs, pc, mem)
	if plan == nil {
		t.Fatal("no hoist plan for invariant loop")
	}
	if len(plan.accesses) != 2 {
		t.Fatalf("accesses: got %d want 2: %+v", len(plan.accesses), plan.accesses)
	}
	if !plan.elide[1] || !plan.elide[2] {
		t.Fatalf("body MOVEs not elided: %+v", plan.elide)
	}

	// Base written inside the loop ((A2)+ in body): no hoist for A2.
	instrs, mem, pc = m68kHoistScan(t,
		0x7200|10, 0x302A, 0x0008, 0x30DA, 0x51C9, 0xFFF8, 0x6002)
	// MOVE.W (A2)+,(A0)+ writes A2 → 8(A2) not invariant
	if plan := m68kAnalyseLoopInvariantGuards(instrs, pc, mem); plan != nil {
		t.Fatalf("hoisted a written base: %+v", plan)
	}

	// MOVEA to the base in the prefix: not invariant either.
	instrs, mem, pc = m68kHoistScan(t,
		0x2448, // MOVEA.L A0,A2
		0x7200|10, 0x302A, 0x0008, 0x51C9, 0xFFFA, 0x6002)
	if plan := m68kAnalyseLoopInvariantGuards(instrs, pc, mem); plan != nil {
		t.Fatalf("hoisted a prefix-written base: %+v", plan)
	}
}

func TestM68KJIT_LoopHoistShape(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	words := m68kHoistLoopWords(10)
	compile := func(disable bool) (int, uint64) {
		old := m68kJITLoopHoistDisabled
		m68kJITLoopHoistDisabled = disable
		defer func() { m68kJITLoopHoistDisabled = old }()
		mem := make([]byte, 1<<16)
		pc := uint32(0x1000)
		for i, w := range words {
			mem[pc+uint32(i*2)] = byte(w >> 8)
			mem[pc+uint32(i*2)+1] = byte(w)
		}
		instrs := m68kScanBlock(mem, pc)
		em, err := AllocExecMem(1 << 20)
		if err != nil {
			t.Fatalf("AllocExecMem: %v", err)
		}
		defer em.Free()
		before := m68kLoopHoistEmits.Load()
		block, err := m68kCompileBlockWithMem(instrs, pc, em, mem)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		return block.execSize, m68kLoopHoistEmits.Load() - before
	}
	_, emitsOn := compile(false)
	_, emitsOff := compile(true)
	if emitsOn != 1 {
		t.Fatalf("hoist counter with hoisting on: got %d want 1", emitsOn)
	}
	if emitsOff != 0 {
		t.Fatalf("hoist counter with kill switch: got %d want 0", emitsOff)
	}
}

// Parity: hoisted loop matches the interpreter; out-of-range base bails to
// the interpreter with nothing retired.
func TestM68KJIT_LoopHoistParity(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	for _, n := range []uint16{0, 5, 50} {
		n := n
		t.Run("count", func(t *testing.T) {
			tc := m68kDiffCase{
				name:  "hoist_loop",
				words: m68kHoistLoopWords(n),
				setup: func(cpu *M68KCPU) {
					cpu.AddrRegs[2] = 0x4000
					cpu.Write16(0x4008, 0xBEEF)
				},
				watch: []m68kDiffMemWatch{{addr: 0x400C, size: M68K_SIZE_WORD}},
			}
			mem := make([]byte, 0x2000)
			for i, w := range tc.words {
				mem[0x1000+i*2] = byte(w >> 8)
				mem[0x1000+i*2+1] = byte(w)
			}
			instrs := m68kScanBlock(mem, 0x1000)
			runM68KJITDifferentialBlockDynamicRetire(t, tc, len(instrs))
		})
	}
}

// Precheck failure path: base pointing past RAM must bail with zero
// retirement and untouched state so the interpreter takes over.
func TestM68KJIT_LoopHoistPrecheckBail(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	jit.PC = m68kDiffStartPC
	m68kDiffSetupCPU(jit)
	jit.AddrRegs[2] = 0xFFFF0000 // far outside RAM
	m68kDiffWriteProgram(jit, m68kDiffStartPC, m68kHoistLoopWords(5)...)

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
	rig.ctx.ChainCount = 0
	d1Before := jit.DataRegs[1]
	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	if rig.ctx.NeedIOFallback != 1 {
		t.Fatal("precheck did not bail for an out-of-range base")
	}
	if rig.ctx.RetPC != m68kDiffStartPC || rig.ctx.RetCount != 0 {
		t.Fatalf("precheck bail retired state: RetPC=%08X RetCount=%d", rig.ctx.RetPC, rig.ctx.RetCount)
	}
	if jit.DataRegs[1] != d1Before {
		t.Fatalf("precheck bail mutated D1: %08X want %08X", jit.DataRegs[1], d1Before)
	}
}

// Benchmark: 51-iteration invariant-access loop with per-iteration guards
// versus the hoisted precheck.
func benchmarkM68KLoopHoist(b *testing.B, disable bool) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available")
	}
	old := m68kJITLoopHoistDisabled
	m68kJITLoopHoistDisabled = disable
	defer func() { m68kJITLoopHoistDisabled = old }()

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

func BenchmarkM68KJIT_LoopHoistOff(b *testing.B) { benchmarkM68KLoopHoist(b, true) }
func BenchmarkM68KJIT_LoopHoistOn(b *testing.B)  { benchmarkM68KLoopHoist(b, false) }
