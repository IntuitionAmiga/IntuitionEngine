// jit_m68k_bounded_loop_test.go - Milestone 7 bounded counter-loop budget
// removal. Analysis unit tests for each proof obligation, a shape test
// (bounded loop compiles without the budget-exit machinery), and parity
// with dynamic retired counts.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"unsafe"
)

// MOVEQ #N,D1 ; ADDQ.L #1,D0 ; DBRA D1,-4 ; then scanner-appended tail.
func m68kBoundedLoopWords(n uint16) []uint16 {
	return []uint16{0x7200 | n, 0x5280, 0x51C9, 0xFFFC, 0x6002}
}

func TestM68KJIT_BoundedLoopAnalysis(t *testing.T) {
	scan := func(words ...uint16) ([]M68KJITInstr, []byte, uint32) {
		t.Helper()
		mem := make([]byte, 1<<16)
		pc := uint32(0x1000)
		for i, w := range words {
			mem[pc+uint32(i*2)] = byte(w >> 8)
			mem[pc+uint32(i*2)+1] = byte(w)
		}
		return m68kScanBlock(mem, pc), mem, pc
	}

	// Proven: MOVEQ seed, pure body, small count.
	instrs, mem, pc := scan(m68kBoundedLoopWords(10)...)
	if len(instrs) < 3 {
		t.Fatalf("scan: %d instrs", len(instrs))
	}
	if !m68kBoundedCounterDBccLoop(instrs, 2, 1, mem, pc, m68kJitBudget) {
		t.Fatal("bounded loop not proven")
	}

	// Refuted: no seed (loop head at block start).
	instrs, mem, pc = scan(0x5280, 0x51C9, 0xFFFC, 0x6002)
	if m68kBoundedCounterDBccLoop(instrs, 1, 0, mem, pc, m68kJitBudget) {
		t.Fatal("seedless loop proven bounded")
	}

	// Refuted: body rewrites the counter.
	instrs, mem, pc = scan(0x7205, 0x5281, 0x51C9, 0xFFFC, 0x6002) // ADDQ.L #1,D1
	if m68kBoundedCounterDBccLoop(instrs, 2, 1, mem, pc, m68kJitBudget) {
		t.Fatal("counter-rewriting loop proven bounded")
	}

	// Refuted: seed too large for the budget (MOVE.W #4095,D1 with body 2).
	instrs, mem, pc = scan(0x323C, 0x0FFF, 0x5280, 0x51C9, 0xFFFC, 0x6002)
	if m68kBoundedCounterDBccLoop(instrs, 2, 1, mem, pc, m68kJitBudget) {
		t.Fatal("budget-exceeding loop proven bounded")
	}

	// Proven: MOVE.W seed within budget.
	instrs, mem, pc = scan(0x323C, 0x0400, 0x5280, 0x51C9, 0xFFFC, 0x6002)
	if !m68kBoundedCounterDBccLoop(instrs, 2, 1, mem, pc, m68kJitBudget) {
		t.Fatal("MOVE.W-seeded loop not proven")
	}

	// Refuted: branch inside the body.
	instrs, mem, pc = scan(0x7205, 0x6600, 0x0002, 0x5280, 0x51C9, 0xFFFA, 0x6002)
	for i := range instrs {
		if instrs[i].opcode&0xF0F8 == 0x50C8 {
			if m68kBoundedCounterDBccLoop(instrs, i, 1, mem, pc, m68kJitBudget) {
				t.Fatal("branch-in-body loop proven bounded")
			}
		}
	}
}

// Shape: with the proof active the DBcc back edge is an unconditional
// jump; the compiled block must be smaller than with the kill switch on.
func TestM68KJIT_BoundedLoopShape(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	words := m68kBoundedLoopWords(10)
	compileLen := func(disable bool) int {
		old := m68kJITBoundedLoopDisabled
		m68kJITBoundedLoopDisabled = disable
		defer func() { m68kJITBoundedLoopDisabled = old }()
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
		block, err := m68kCompileBlockWithMem(instrs, pc, em, mem)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		return block.execSize
	}
	lenOn := compileLen(false)
	lenOff := compileLen(true)
	if lenOn >= lenOff {
		t.Fatalf("bounded-loop proof did not shrink the block: on=%d off=%d", lenOn, lenOff)
	}
}

// Parity: the bounded loop retires the exact trip count and matches the
// interpreter's registers, CCR and PC.
func TestM68KJIT_BoundedLoopParity(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	for _, n := range []uint16{0, 1, 10, 100} {
		n := n
		t.Run("count_"+string(rune('0'+n%10))+"_"+string(rune('0'+n/10%10)), func(t *testing.T) {
			tc := m68kDiffCase{
				name:  "bounded_dbra",
				words: m68kBoundedLoopWords(n),
				setup: func(cpu *M68KCPU) {
					cpu.DataRegs[0] = 0
				},
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

// Benchmark: the same invariant loop with the per-iteration budget checks
// versus the proven-bounded unconditional back edge.
func benchmarkM68KBoundedLoop(b *testing.B, disable bool) {
	if !m68kJitAvailable {
		b.Skip("M68K JIT not available")
	}
	old := m68kJITBoundedLoopDisabled
	m68kJITBoundedLoopDisabled = disable
	defer func() { m68kJITBoundedLoopDisabled = old }()

	words := []uint16{0x7232, 0x5280, 0x51C9, 0xFFFC, 0x6002} // MOVEQ #50,D1; ADDQ; DBRA
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

func BenchmarkM68KJIT_BoundedLoopOff(b *testing.B) { benchmarkM68KBoundedLoop(b, true) }
func BenchmarkM68KJIT_BoundedLoopOn(b *testing.B)  { benchmarkM68KBoundedLoop(b, false) }

// A proven-bounded loop must still honour the live ChainBudget: the
// dispatcher seeds it dynamically (IRQ-sampling remainder, 0 in verify
// mode) and chained predecessors arrive with it already drawn down, so
// the compile-time proof against m68kJitBudget never bounds it. Entering
// with a small budget must chain-exit at the back edge, not run the loop
// to exhaustion.
func TestM68KJIT_BoundedLoopHonoursReducedChainBudget(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	words := m68kBoundedLoopWords(50) // 51 trips, body ADDQ.L #1,D0 + DBRA
	mem := make([]byte, 1<<20)
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
	block, err := m68kCompileBlockWithMem(instrs, pc, em, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	copy(cpu.memory[pc:], mem[pc:pc+uint32(len(words)*2)])
	bitmap := make([]byte, (uint32(len(cpu.memory))+4095)>>12)
	pageMin := make([]uint16, len(bitmap))
	pageMax := make([]uint16, len(bitmap))
	ctx := newM68KJITContext(cpu, bitmap, pageMin, pageMax)
	const budget = 5
	ctx.RetPC = 0
	ctx.NeedIOFallback = 0
	ctx.ChainCount = 0
	ctx.ChainBudget = budget
	callNative(block.execAddr, uintptr(unsafe.Pointer(ctx)))
	if cpu.DataRegs[0] >= 51 {
		t.Fatalf("bounded loop ignored reduced ChainBudget: ran all %d trips", cpu.DataRegs[0])
	}
	// The exit overshoots by at most one loop body plus the chain exit's
	// partial-block retirement — never by the loop's remaining trips.
	slack := uint32(2 + len(instrs))
	if ctx.ChainCount > budget+slack {
		t.Fatalf("ChainCount %d exceeds seeded budget %d (+slack %d)", ctx.ChainCount, budget, slack)
	}
	loopHead := pc + 2 // ADDQ.L #1,D0
	if ctx.RetPC != loopHead {
		t.Fatalf("expected chain exit to loop head %08X, got RetPC=%08X", loopHead, ctx.RetPC)
	}
}
