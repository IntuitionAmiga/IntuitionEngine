// jit_m68k_arm64_backend_test.go - differential tests for the minimal arm64
// M68020 JIT backend (parity plan milestone 3, slice 1).
//
// Every test compares native execution against the interpreter on the same
// program and initial state: data registers, CCR (SR low five bits), the
// resume PC, and the retired-instruction count must match exactly. The
// accounting differential runs from day one, per the parity plan.

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"math"
	"testing"
)

func TestM68KARM64_ProductionAvailable(t *testing.T) {
	if !m68kJitAvailable {
		t.Fatal("arm64 M68K JIT remains production-gated after real-hardware validation")
	}
}

func TestM68KARM64_JSRLeafFusionParity(t *testing.T) {
	t.Setenv("M68K_ARM64_CHAIN", "0")
	const leaf = uint32(0x1800)
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	for _, cpu := range []*M68KCPU{ref, got} {
		m68kARM64WriteWords(cpu, m68kARM64TestPC,
			0x4EB9, uint16(leaf>>16), uint16(leaf), 0x7207, 0x6002)
		m68kARM64WriteWords(cpu, leaf, 0x7005, 0x4E75)
		cpu.AddrRegs[7] = 0x00FF0000
		cpu.SR = 0x201F
		cpu.PC = m68kARM64TestPC
	}
	instrs := m68kFuseJSRLeafCalls(m68kScanBlock(got.memory, m68kARM64TestPC), m68kARM64TestPC, got.memory, got.ProfileTopOfRAM())
	if prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM()); prefix != len(instrs) {
		t.Fatalf("supported prefix = %d, want fused stream %d", prefix, len(instrs))
	}
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	block, err := m68kCompileBlockARM64(instrs, m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
	if err != nil {
		t.Fatalf("compile fused block: %v", err)
	}
	m68kARM64RunInterp(ref, len(instrs))
	ctx := newM68KJITContext(got, nil, nil, nil)
	callNative(block.execAddr, m68kJITContextPtr(ctx))
	got.PC = ctx.RetPC
	if ctx.RetCount != uint32(len(instrs)) {
		t.Fatalf("retired = %d, want %d", ctx.RetCount, len(instrs))
	}
	m68kARM64CompareState(t, "fused JSR leaf", ref, got)
}

func TestM68KARM64_FusedLeafRTSBailPreservesCommittedEffects(t *testing.T) {
	const (
		leaf  = uint32(0x1800)
		stack = uint32(0x8004)
	)
	cpu := m68kARM64NewCPU(t)
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x4EB9, uint16(leaf>>16), uint16(leaf), 0x7207, 0x6002)
	m68kARM64WriteWords(cpu, leaf, 0x7005, 0x4E75)
	cpu.stackLowerBound = 0
	cpu.stackUpperBound = stack - 4
	cpu.AddrRegs[7] = stack
	instrs := m68kFuseJSRLeafCalls(m68kScanBlock(cpu.memory, m68kARM64TestPC), m68kARM64TestPC, cpu.memory, cpu.ProfileTopOfRAM())
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	block, err := m68kCompileBlockARM64(instrs, m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
	if err != nil {
		t.Fatalf("compile fused block: %v", err)
	}
	ctx := newM68KJITContext(cpu, nil, nil, nil)
	callNative(block.execAddr, m68kJITContextPtr(ctx))
	if ctx.NeedIOFallback == 0 || ctx.RetPC != leaf+2 || ctx.RetCount != 2 {
		t.Fatalf("synthetic RTS bail: NeedIO=%d RetPC=%08X RetCount=%d", ctx.NeedIOFallback, ctx.RetPC, ctx.RetCount)
	}
	if cpu.DataRegs[0] != 5 || cpu.AddrRegs[7] != stack-4 {
		t.Fatalf("committed effects lost: D0=%08X A7=%08X", cpu.DataRegs[0], cpu.AddrRegs[7])
	}
}

func TestM68KARM64_ConstFoldShape(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7005, 0x5680, 0x6002)
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	before := m68kFoldedConstEmits.Load()
	if _, err := m68kCompileBlockARM64(instrs, m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM()); err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := m68kFoldedConstEmits.Load() - before; got != 2 {
		t.Fatalf("arm64 folded emits = %d, want 2", got)
	}
}

const m68kARM64TestPC = uint32(0x1000)

func m68kARM64WriteWords(cpu *M68KCPU, addr uint32, words ...uint16) uint32 {
	for _, w := range words {
		cpu.memory[addr] = byte(w >> 8)
		cpu.memory[addr+1] = byte(w)
		addr += 2
	}
	return addr
}

func m68kARM64NewCPU(t *testing.T) *M68KCPU {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	return cpu
}

// m68kARM64SeedRegs loads a deterministic register state.
func m68kARM64SeedRegs(cpu *M68KCPU, seed [8]uint32, ccr uint16) {
	for i, v := range seed {
		cpu.DataRegs[i] = v
	}
	cpu.SR = (cpu.SR &^ 0x1F) | (ccr & 0x1F)
}

// m68kARM64RunInterp executes n instructions on the interpreter.
func m68kARM64RunInterp(cpu *M68KCPU, n int) {
	for i := 0; i < n; i++ {
		cpu.StepOne()
	}
}

func m68kARM64CompareState(t *testing.T, name string, ref, got *M68KCPU) {
	t.Helper()
	for i := 0; i < 8; i++ {
		if ref.DataRegs[i] != got.DataRegs[i] {
			t.Errorf("%s: D%d interp=%08X native=%08X", name, i, ref.DataRegs[i], got.DataRegs[i])
		}
		if ref.AddrRegs[i] != got.AddrRegs[i] {
			t.Errorf("%s: A%d interp=%08X native=%08X", name, i, ref.AddrRegs[i], got.AddrRegs[i])
		}
	}
	if ref.SR&0x1F != got.SR&0x1F {
		t.Errorf("%s: CCR interp=%02X native=%02X", name, ref.SR&0x1F, got.SR&0x1F)
	}
	if ref.PC != got.PC {
		t.Errorf("%s: PC interp=%08X native=%08X", name, ref.PC, got.PC)
	}
}

// Operand grid covering sign, zero, carry and overflow boundaries.
var m68kARM64GridValues = []uint32{
	0x00000000, 0x00000001, 0x0000007F, 0x00000080, 0x0000FFFF,
	0x00010000, 0x7FFFFFFF, 0x80000000, 0x80000001, 0xFFFFFFFF,
	0xDEADBEEF, 0x12345678,
}

// TestM68KARM64_SupportedPrefix pins the admission behaviour: the supported
// prefix stops at the first unsupported instruction and never includes the
// block terminator.
func TestM68KARM64_SupportedPrefix(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	// moveq #1,d0 ; add.l d0,d1 ; jsr 8(a0,d0.w) [unsupported EA] ; rts
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7001, 0xD280, 0x4EB0, 0x0008, 0x4E75)
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	prefix := m68kARM64SupportedPrefix(instrs, cpu.memory, m68kARM64TestPC, cpu.ProfileTopOfRAM())
	if prefix != 2 {
		t.Fatalf("supported prefix = %d, want 2", prefix)
	}
}

// TestM68KARM64_DifferentialALUGrid runs every supported ALU shape over the
// operand grid and compares native and interpreter results exactly.
func TestM68KARM64_DifferentialALUGrid(t *testing.T) {
	type opCase struct {
		name  string
		words []uint16
		count int
	}
	cases := []opCase{
		{"ADD.L D0,D1", []uint16{0xD280}, 1},
		{"SUB.L D0,D1", []uint16{0x9280}, 1},
		{"CMP.L D0,D1", []uint16{0xB280}, 1},
		{"AND.L D0,D1", []uint16{0xC280}, 1},
		{"OR.L D0,D1", []uint16{0x8280}, 1},
		{"EOR.L D1,D0", []uint16{0xB380}, 1},
		{"MOVE.L D0,D2", []uint16{0x2400}, 1},
		{"TST.L D0", []uint16{0x4A80}, 1},
		{"CLR.L D3", []uint16{0x4283}, 1},
		{"MOVEQ #-1,D4", []uint16{0x78FF}, 1},
		{"MOVEQ #0,D4", []uint16{0x7800}, 1},
		{"MOVEQ #42,D4", []uint16{0x782A}, 1},
		{"ADDQ.L #8,D1", []uint16{0x5081}, 1},
		{"ADDQ.L #1,D1", []uint16{0x5281}, 1},
		{"SUBQ.L #1,D1", []uint16{0x5381}, 1},
		{"SUBQ.L #8,D1", []uint16{0x5181}, 1},
		{"NOP", []uint16{0x4E71}, 1},
		{"MOVE.L #imm,D5", []uint16{0x2A3C, 0x8000, 0x0001}, 1},
		{"chain ADD/CMP/MOVE", []uint16{0xD280, 0xB280, 0x2400, 0x5281}, 4},
	}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	for _, tc := range cases {
		tc := tc
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				end := m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
				m68kARM64WriteWords(cpu, end, 0x4E75) // rts terminator
			}
			// Compile once per case; reuse across the operand grid.
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix < tc.count {
				t.Fatalf("%s: supported prefix %d < want %d", tc.name, prefix, tc.count)
			}
			block, err := m68kCompileBlockARM64(instrs[:tc.count], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", tc.name, err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, a := range m68kARM64GridValues {
				for _, b := range m68kARM64GridValues {
					seed := [8]uint32{a, b, 0x11111111, 0x22222222, 0x33333333, 0x44444444, 0x55555555, 0x66666666}
					for _, ccrIn := range []uint16{0x00, 0x1F, 0x10} {
						for _, cpu := range []*M68KCPU{ref, got} {
							m68kARM64SeedRegs(cpu, seed, ccrIn)
							cpu.PC = m68kARM64TestPC
						}
						m68kARM64RunInterp(ref, tc.count)
						wantPC := m68kARM64TestPC + 2*uint32(len(tc.words))
						if ref.PC != wantPC {
							t.Fatalf("%s a=%08X b=%08X: interpreter PC=%08X want %08X (exception?)", tc.name, a, b, ref.PC, wantPC)
						}
						ctx.RetPC = 0
						ctx.RetCount = 0
						callNative(block.execAddr, m68kJITContextPtr(ctx))
						got.PC = ctx.RetPC
						retired := ctx.RetCount
						if retired != uint32(tc.count) {
							t.Fatalf("%s a=%08X b=%08X: retired=%d want %d", tc.name, a, b, retired, tc.count)
						}
						m68kARM64CompareState(t, tc.name, ref, got)
						if t.Failed() {
							t.Fatalf("%s: first divergence at a=%08X b=%08X ccrIn=%02X", tc.name, a, b, ccrIn)
						}
					}
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed, stopping grid", tc.name)
		}
	}
}

// TestM68KARM64_DispatcherDifferential runs a mixed program (supported and
// unsupported instructions) through the full arm64 JIT dispatcher and the
// interpreter, comparing final state and total retired instructions.
func TestM68KARM64_DispatcherDifferential(t *testing.T) {
	program := []uint16{
		0x7001,         // moveq #1,d0
		0x7202,         // moveq #2,d1
		0xD280,         // add.l d0,d1
		0x2401,         // move.l d1,d2
		0x4482,         // neg.l d2      (unsupported: interpreter fallback)
		0x5282,         // addq.l #1,d2
		0xB480,         // cmp.l d0,d2
		0x4A81,         // tst.l d1
		0x4E72, 0x2700, // stop #$2700
	}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	for _, cpu := range []*M68KCPU{ref, got} {
		m68kARM64WriteWords(cpu, m68kARM64TestPC, program...)
		cpu.PC = m68kARM64TestPC
	}

	refCount := 0
	for !ref.stopped.Load() && refCount < 100 {
		refCount += ref.StepOne()
	}

	got.m68kJitEnabled = true
	got.PerfEnabled = true
	got.StoppedIdleHook = func(c *M68KCPU) { c.running.Store(false) }
	got.running.Store(true)
	got.M68KExecuteJIT()

	m68kARM64CompareState(t, "dispatcher", ref, got)
	if got.InstructionCount != uint64(refCount) {
		t.Errorf("retired count: interp=%d jit=%d", refCount, got.InstructionCount)
	}
}

// TestM68KARM64_SMCStampMismatch pins the conservative SMC rule: once the
// guest bytes under a compiled block change, the stale native code must not
// execute.
func TestM68KARM64_SMCStampMismatch(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7001, 0xD280, 0x4E75)
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	m68kStampGuestBlockBytes(cpu.memory, block)
	if !m68kGuestBlockBytesStillMatch(cpu.memory, block) {
		t.Fatal("fresh block reported stale")
	}
	// Overwrite the second instruction.
	m68kARM64WriteWords(cpu, m68kARM64TestPC+2, 0x7203)
	if m68kGuestBlockBytesStillMatch(cpu.memory, block) {
		t.Fatal("modified guest bytes not detected")
	}
}

// TestM68KARM64_NativeSmoke: smallest possible native block (NOP;NOP) —
// pins the call/return path and the RetPC/RetCount publication.
func TestM68KARM64_NativeSmoke(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x4E71, 0x4E71, 0x4E75)
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	ctx := newM68KJITContext(cpu, nil, nil, nil)
	callNative(block.execAddr, m68kJITContextPtr(ctx))
	if ctx.RetPC != m68kARM64TestPC+4 || ctx.RetCount != 2 {
		t.Fatalf("RetPC=%08X RetCount=%d, want %08X/2", ctx.RetPC, ctx.RetCount, m68kARM64TestPC+4)
	}
}

// TestM68KARM64_CrossThreadInvalidationQueue pins the cross-thread SMC path:
// a bus write over compiled code while the dispatcher owns the cache must
// enqueue (bumping the generation) rather than mutate the cache off-thread,
// and the drain must evict the block. Writes outside the published code
// envelope must not bump the generation (livelock gate).
func TestM68KARM64_CrossThreadInvalidationQueue(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	defer cpu.freeM68KJIT()

	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7001, 0xD280, 0x4E75)
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, cpu.m68kGetJITExecMem(), cpu.memory, cpu.ProfileTopOfRAM())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	m68kStampGuestBlockBytes(cpu.memory, block)
	cpu.m68kARM64PublishCodeEnv(block)
	cpu.m68kJitCache.Put(block)

	cpu.m68kJitDispatchActive.Store(true)
	defer cpu.m68kJitDispatchActive.Store(false)

	genBefore := cpu.m68kJitInvalGen.Load()

	// Write far outside the envelope: must be gated out, no generation bump.
	if err := WriteGuestBytes(bus, 0x40000, 0, []byte{0xDE, 0xAD}); err != nil {
		t.Fatalf("WriteGuestBytes (outside): %v", err)
	}
	if got := cpu.m68kJitInvalGen.Load(); got != genBefore {
		t.Fatalf("write outside code envelope bumped generation %d -> %d", genBefore, got)
	}

	// Write over the compiled block: must enqueue and bump the generation,
	// leaving the cache untouched until the CPU thread drains.
	if err := WriteGuestBytes(bus, m68kARM64TestPC+2, 0, []byte{0x4E, 0x71}); err != nil {
		t.Fatalf("WriteGuestBytes (code): %v", err)
	}
	if got := cpu.m68kJitInvalGen.Load(); got == genBefore {
		t.Fatal("write over compiled code did not bump the invalidation generation")
	}
	if !cpu.m68kJitHasPendingInval.Load() {
		t.Fatal("write over compiled code did not set the pending-invalidation flag")
	}
	if cpu.m68kJitCache.Get(uint64(m68kARM64TestPC)) == nil {
		t.Fatal("cache was mutated off-thread before drain")
	}

	cpu.m68kDrainPendingJITInvalidations()
	if cpu.m68kJitCache.Get(uint64(m68kARM64TestPC)) != nil {
		t.Fatal("drain did not evict the invalidated block")
	}
}

// TestM68KARM64_DispatcherCrossThreadSMC runs a native loop body, injects an
// enqueued code rewrite mid-run (at a dispatcher loop boundary, the earliest
// deterministic injection point for the cross-thread path), and requires the
// rewritten code to take effect through the drain/generation path.
func TestM68KARM64_DispatcherCrossThreadSMC(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	// 0x1000: moveq #1,d0 ; moveq #2,d1  (native block)
	// 0x1004: bra.s 0x1000               (interpreter, loops forever)
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7001, 0x7202, 0x60FA)
	cpu.PC = m68kARM64TestPC
	cpu.m68kJitEnabled = true

	patched := false
	cpu.InstructionCountHook = func(c *M68KCPU, count uint64) {
		if !patched && count >= 50 {
			patched = true
			// Rewrite the first instruction to moveq #7,d0 and enqueue the
			// range exactly as the cross-thread bus invalidator would.
			c.memory[m68kARM64TestPC] = 0x70
			c.memory[m68kARM64TestPC+1] = 0x07
			c.m68kEnqueueJITInvalidation(m68kARM64TestPC, 2)
		}
		// Stop well past the native chain budget (256) so the drain/recompile
		// is observed whether or not chaining is enabled.
		if count >= 2000 {
			c.running.Store(false)
		}
	}
	cpu.running.Store(true)
	cpu.M68KExecuteJIT()

	if !patched {
		t.Fatal("injection hook never fired")
	}
	if cpu.DataRegs[0] != 7 {
		t.Fatalf("D0=%08X after code rewrite, want 00000007 (stale native block executed)", cpu.DataRegs[0])
	}
	if cpu.m68kJitHasPendingInval.Load() {
		t.Fatal("pending invalidation was never drained")
	}
}

// ===========================================================================
// Milestone 3 slice 2: memory EA modes, big-endian paths, byte/word sizes.
// ===========================================================================

const m68kARM64BufBase = uint32(0x8000)

// Valid interpreter stack region (Push32 enforces a stack floor).
const m68kARM64StackTop = uint32(0x00FF0000)

// m68kARM64SeedMem fills the data buffer with a deterministic byte pattern
// and points the address registers into it.
func m68kARM64SeedMem(cpu *M68KCPU) {
	for i := uint32(0); i < 0x100; i++ {
		cpu.memory[m68kARM64BufBase+i] = byte(i*7 + 3)
	}
	cpu.AddrRegs[0] = m68kARM64BufBase + 0x10
	cpu.AddrRegs[1] = m68kARM64BufBase + 0x20
	cpu.AddrRegs[2] = m68kARM64BufBase + 0x30
	cpu.AddrRegs[3] = m68kARM64BufBase + 0x40
	cpu.AddrRegs[4] = m68kARM64BufBase + 0x50
	cpu.AddrRegs[5] = m68kARM64BufBase + 0x60
	cpu.AddrRegs[6] = m68kARM64BufBase + 0x70
	cpu.AddrRegs[7] = m68kARM64BufBase + 0x80
}

func m68kARM64CompareMem(t *testing.T, name string, ref, got *M68KCPU) {
	t.Helper()
	for i := uint32(0); i < 0x100; i++ {
		a := m68kARM64BufBase + i
		if ref.memory[a] != got.memory[a] {
			t.Errorf("%s: mem[%08X] interp=%02X native=%02X", name, a, ref.memory[a], got.memory[a])
			return
		}
	}
}

// TestM68KARM64_DifferentialMemoryEAGrid drives every slice-2 memory and
// sized shape over an operand grid, comparing registers, address registers,
// CCR, PC and the memory window against the interpreter.
func TestM68KARM64_DifferentialMemoryEAGrid(t *testing.T) {
	type opCase struct {
		name  string
		words []uint16
		count int
	}
	cases := []opCase{
		// MOVE loads
		{"MOVE.L (A0),D1", []uint16{0x2210}, 1},
		{"MOVE.W (A0),D1", []uint16{0x3210}, 1},
		{"MOVE.B (A0),D1", []uint16{0x1210}, 1},
		{"MOVE.L (A0)+,D1", []uint16{0x2218}, 1},
		{"MOVE.B (A0)+,D1", []uint16{0x1218}, 1},
		{"MOVE.L -(A0),D1", []uint16{0x2220}, 1},
		{"MOVE.B -(A7),D1", []uint16{0x1227}, 1},
		{"MOVE.B (A7)+,D1", []uint16{0x121F}, 1},
		{"MOVE.L 4(A0),D1", []uint16{0x2228, 0x0004}, 1},
		{"MOVE.L -8(A0),D1", []uint16{0x2228, 0xFFF8}, 1},
		{"MOVE.L abs.L,D2", []uint16{0x2439, 0x0000, 0x8020}, 1},
		// MOVE stores
		{"MOVE.L D1,(A0)", []uint16{0x2081}, 1},
		{"MOVE.B D1,(A0)+", []uint16{0x10C1}, 1},
		{"MOVE.W D1,-(A0)", []uint16{0x3101}, 1},
		{"MOVE.B D1,-(A7)", []uint16{0x1F01}, 1},
		{"MOVE.L D1,6(A0)", []uint16{0x2141, 0x0006}, 1},
		{"MOVE.W D2,abs.L", []uint16{0x33C2, 0x0000, 0x8060}, 1},
		{"MOVE.L #imm,(A0)", []uint16{0x20BC, 0xCAFE, 0xF00D}, 1},
		// MOVE memory to memory
		{"MOVE.L (A0),(A1)", []uint16{0x2290}, 1},
		{"MOVE.L (A0)+,(A1)+", []uint16{0x22D8}, 1},
		{"MOVE.B -(A0),(A1)+", []uint16{0x12E0}, 1},
		{"MOVE.L (A0)+,(A0)+", []uint16{0x20D8}, 1},
		{"MOVE.W (A0)+,-(A0)", []uint16{0x3118}, 1},
		// MOVEA
		{"MOVEA.L (A0),A1", []uint16{0x2250}, 1},
		{"MOVEA.W (A0),A1", []uint16{0x3250}, 1},
		{"MOVEA.W #imm,A1", []uint16{0x327C, 0x8000}, 1},
		{"MOVEA.L D1,A2", []uint16{0x2441}, 1},
		// TST / CLR
		{"TST.B (A0)", []uint16{0x4A10}, 1},
		{"TST.W (A0)+", []uint16{0x4A58}, 1},
		{"TST.L -(A0)", []uint16{0x4A20}, 1},
		{"TST.L 2(A0)", []uint16{0x4A28, 0x0002}, 1},
		{"TST.B D0", []uint16{0x4A00}, 1},
		{"TST.W D0", []uint16{0x4A40}, 1},
		{"CLR.B (A0)", []uint16{0x4210}, 1},
		{"CLR.W (A0)+", []uint16{0x4258}, 1},
		{"CLR.L -(A0)", []uint16{0x4220}, 1},
		{"CLR.L abs.L", []uint16{0x42B9, 0x0000, 0x8058}, 1},
		{"CLR.B D0", []uint16{0x4200}, 1},
		{"CLR.W D0", []uint16{0x4240}, 1},
		// ALU with memory source
		{"ADD.L (A0),D1", []uint16{0xD290}, 1},
		{"ADD.W (A0)+,D1", []uint16{0xD258}, 1},
		{"ADD.B (A0),D1", []uint16{0xD210}, 1},
		{"SUB.L (A0),D1", []uint16{0x9290}, 1},
		{"SUB.B -(A0),D1", []uint16{0x9220}, 1},
		{"CMP.L (A0),D1", []uint16{0xB290}, 1},
		{"CMP.B (A0),D1", []uint16{0xB210}, 1},
		{"CMP.W (A0)+,D1", []uint16{0xB258}, 1},
		{"AND.L (A0),D1", []uint16{0xC290}, 1},
		{"OR.W (A0),D1", []uint16{0x8250}, 1},
		// ALU with memory destination (read-modify-write)
		{"ADD.L D1,(A0)", []uint16{0xD190}, 1},
		{"ADD.B D1,(A0)", []uint16{0xD110}, 1},
		{"SUB.B D1,(A0)+", []uint16{0x9118}, 1},
		{"AND.B D1,(A0)", []uint16{0xC110}, 1},
		{"OR.L D1,(A0)", []uint16{0x8390}, 1},
		{"EOR.B D1,(A0)", []uint16{0xB310}, 1},
		{"EOR.W D1,(A0)+", []uint16{0xB358}, 1},
		// Sized register-register forms
		{"ADD.B D0,D1", []uint16{0xD200}, 1},
		{"ADD.W D0,D1", []uint16{0xD240}, 1},
		{"SUB.W D0,D1", []uint16{0x9240}, 1},
		{"CMP.B D0,D1", []uint16{0xB200}, 1},
		{"AND.W D0,D1", []uint16{0xC240}, 1},
		{"OR.B D0,D1", []uint16{0x8200}, 1},
		{"EOR.W D1,D0", []uint16{0xB340}, 1},
		{"MOVE.B D0,D1", []uint16{0x1200}, 1},
		{"MOVE.W D0,D1", []uint16{0x3200}, 1},
		// ADDQ / SUBQ sized and address-register forms
		{"ADDQ.B #4,D1", []uint16{0x5801}, 1},
		{"SUBQ.W #3,D1", []uint16{0x5741}, 1},
		{"ADDQ.W #1,A0", []uint16{0x5248}, 1},
		{"SUBQ.L #2,A0", []uint16{0x5588}, 1},
		{"ADDQ.W #2,(A0)", []uint16{0x5450}, 1},
		{"SUBQ.B #1,(A0)", []uint16{0x5310}, 1},
		{"ADDQ.L #8,-(A0)", []uint16{0x50A0}, 1},
		// LEA
		{"LEA (A0),A1", []uint16{0x43D0}, 1},
		{"LEA 8(A0),A2", []uint16{0x45E8, 0x0008}, 1},
		{"LEA -4(A7),A7", []uint16{0x4FEF, 0xFFFC}, 1},
		{"LEA abs.L,A1", []uint16{0x43F9, 0x0000, 0x8044}, 1},
		// Mixed block
		{"mixed mem chain", []uint16{0x2210, 0x5281, 0x2081, 0xB290, 0x1218}, 5},
	}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 22)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	for _, tc := range cases {
		tc := tc
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				end := m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
				m68kARM64WriteWords(cpu, end, 0x4E75) // rts terminator
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix < tc.count {
				t.Fatalf("%s: supported prefix %d < want %d", tc.name, prefix, tc.count)
			}
			block, err := m68kCompileBlockARM64(instrs[:tc.count], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", tc.name, err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, a := range m68kARM64GridValues {
				seed := [8]uint32{a, a ^ 0x5A5A5A5A, 0x11111111, 0x22222222, 0x33333333, 0x44444444, 0x55555555, 0x66666666}
				for _, ccrIn := range []uint16{0x00, 0x1F, 0x10} {
					for _, cpu := range []*M68KCPU{ref, got} {
						m68kARM64SeedRegs(cpu, seed, ccrIn)
						m68kARM64SeedMem(cpu)
						cpu.PC = m68kARM64TestPC
					}
					m68kARM64RunInterp(ref, tc.count)
					wantPC := m68kARM64TestPC + 2*uint32(len(tc.words))
					if ref.PC != wantPC {
						t.Fatalf("%s a=%08X: interpreter PC=%08X want %08X (exception?)", tc.name, a, ref.PC, wantPC)
					}
					ctx.RetPC = 0
					ctx.RetCount = 0
					ctx.NeedIOFallback = 0
					callNative(block.execAddr, m68kJITContextPtr(ctx))
					if ctx.NeedIOFallback != 0 {
						t.Fatalf("%s a=%08X: unexpected I/O bail", tc.name, a)
					}
					got.PC = ctx.RetPC
					if ctx.RetCount != uint32(tc.count) {
						t.Fatalf("%s a=%08X: retired=%d want %d", tc.name, a, ctx.RetCount, tc.count)
					}
					m68kARM64CompareState(t, tc.name, ref, got)
					m68kARM64CompareMem(t, tc.name, ref, got)
					if t.Failed() {
						t.Fatalf("%s: first divergence at a=%08X ccrIn=%02X", tc.name, a, ccrIn)
					}
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed, stopping grid", tc.name)
		}
	}
}

// TestM68KARM64_IOBailUnit pins the mid-block bail contract: a memory access
// hitting a marked I/O page must exit before any of the instruction's side
// effects, publishing the partial retired count, the faulting instruction PC
// and NeedIOFallback, with all earlier instructions committed.
func TestM68KARM64_IOBailUnit(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	// moveq #5,d0 ; move.l (a0),d1 ; addq.l #1,d1
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7005, 0x2210, 0x5281, 0x4E75)
	cpu.m68kJitIOPageBitmap = make([]bool, (uint32(len(cpu.memory))+255)>>8)
	ioAddr := uint32(0x9000)
	cpu.m68kJitIOPageBitmap[ioAddr>>8] = true

	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	block, err := m68kCompileBlockARM64(instrs[:3], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	ctx := newM68KJITContext(cpu, nil, nil, nil)

	cpu.AddrRegs[0] = ioAddr
	cpu.DataRegs[0] = 0
	cpu.DataRegs[1] = 0x12345678
	cpu.SR = cpu.SR &^ 0x1F
	callNative(block.execAddr, m68kJITContextPtr(ctx))

	if ctx.NeedIOFallback != 1 {
		t.Fatalf("NeedIOFallback=%d, want 1", ctx.NeedIOFallback)
	}
	if ctx.RetPC != m68kARM64TestPC+2 {
		t.Fatalf("bail RetPC=%08X, want %08X (faulting instruction)", ctx.RetPC, m68kARM64TestPC+2)
	}
	if ctx.RetCount != 1 {
		t.Fatalf("bail RetCount=%d, want 1", ctx.RetCount)
	}
	if cpu.DataRegs[0] != 5 {
		t.Fatalf("D0=%08X, want 5 (moveq before the bail must commit)", cpu.DataRegs[0])
	}
	if cpu.DataRegs[1] != 0x12345678 {
		t.Fatalf("D1=%08X changed by bailed instruction", cpu.DataRegs[1])
	}
	if cpu.AddrRegs[0] != ioAddr {
		t.Fatalf("A0=%08X changed by bailed instruction", cpu.AddrRegs[0])
	}
	if cpu.SR&0x1F != 0 {
		t.Fatalf("CCR=%02X after bail, want the pre-instruction CCR 00", cpu.SR&0x1F)
	}

	// Out-of-bounds address takes the same bail.
	cpu.AddrRegs[0] = uint32(len(cpu.memory))
	cpu.SR = cpu.SR &^ 0x1F
	ctx.NeedIOFallback = 0
	callNative(block.execAddr, m68kJITContextPtr(ctx))
	if ctx.NeedIOFallback != 1 || ctx.RetPC != m68kARM64TestPC+2 || ctx.RetCount != 1 {
		t.Fatalf("bounds bail: NeedIOFallback=%d RetPC=%08X RetCount=%d", ctx.NeedIOFallback, ctx.RetPC, ctx.RetCount)
	}
}

// TestM68KARM64_DispatcherIOFallback runs the full dispatcher over a block
// whose memory access hits a marked I/O page. The dispatcher must interpret
// the faulting instruction and continue, with exact retired accounting.
func TestM68KARM64_DispatcherIOFallback(t *testing.T) {
	bus := NewMachineBus()
	ref := NewM68KCPU(NewMachineBus())
	got := NewM68KCPU(bus)
	program := []uint16{
		0x7005,         // moveq #5,d0
		0x2210,         // move.l (a0),d1
		0x5281,         // addq.l #1,d1
		0x2081,         // move.l d1,(a0)
		0x4E72, 0x2700, // stop #$2700
	}
	ioAddr := uint32(0x9000)
	for _, cpu := range []*M68KCPU{ref, got} {
		m68kARM64WriteWords(cpu, m68kARM64TestPC, program...)
		cpu.PC = m68kARM64TestPC
		cpu.AddrRegs[0] = ioAddr
		// Deterministic data at the target so both sides read the same value.
		cpu.memory[ioAddr] = 0x01
		cpu.memory[ioAddr+1] = 0x02
		cpu.memory[ioAddr+2] = 0x03
		cpu.memory[ioAddr+3] = 0x04
	}

	refCount := 0
	for !ref.stopped.Load() && refCount < 100 {
		refCount += ref.StepOne()
	}

	// Mark the page as I/O for the JIT only: native code must bail to the
	// interpreter for the access, and the interpreter reads plain RAM, so
	// the final state must match the reference exactly.
	got.m68kJitEnabled = true
	got.PerfEnabled = true
	got.m68kJitForceNative = true
	got.StoppedIdleHook = func(c *M68KCPU) { c.running.Store(false) }
	if err := got.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	if got.m68kJitIOPageBitmap == nil {
		got.m68kJitIOPageBitmap = make([]bool, (uint32(len(got.memory))+255)>>8)
	}
	got.m68kJitIOPageBitmap[ioAddr>>8] = true
	got.m68kJitCtx = newM68KJITContext(got, nil, nil, nil)
	got.running.Store(true)
	got.M68KExecuteJIT()

	m68kARM64CompareState(t, "io-fallback", ref, got)
	for i := uint32(0); i < 4; i++ {
		if ref.memory[ioAddr+i] != got.memory[ioAddr+i] {
			t.Errorf("mem[%08X] interp=%02X jit=%02X", ioAddr+i, ref.memory[ioAddr+i], got.memory[ioAddr+i])
		}
	}
	if got.InstructionCount != uint64(refCount) {
		t.Errorf("retired count: interp=%d jit=%d", refCount, got.InstructionCount)
	}
}

// TestM68KARM64_DifferentialShiftImmGrid covers slice 3: immediate ALU
// (ADDI/SUBI/CMPI/ANDI/ORI/EORI), NEG, NOT, SWAP, EXT, PEA and the
// immediate-count shift and rotate family, differentially against the
// interpreter over the operand grid.
func TestM68KARM64_DifferentialShiftImmGrid(t *testing.T) {
	type opCase struct {
		name  string
		words []uint16
		count int
	}
	cases := []opCase{
		// Immediate ALU to Dn
		{"ADDI.L #imm,D1", []uint16{0x0681, 0x0001, 0x8000}, 1},
		{"ADDI.B #imm,D1", []uint16{0x0601, 0x007F}, 1},
		{"SUBI.W #imm,D1", []uint16{0x0441, 0x8001}, 1},
		{"CMPI.L #imm,D1", []uint16{0x0C81, 0xDEAD, 0xBEEF}, 1},
		{"CMPI.B #imm,D1", []uint16{0x0C01, 0x0080}, 1},
		{"ANDI.W #imm,D1", []uint16{0x0241, 0x00FF}, 1},
		{"ORI.B #imm,D1", []uint16{0x0001, 0x0081}, 1},
		{"EORI.L #imm,D1", []uint16{0x0A81, 0xFFFF, 0x0000}, 1},
		// Immediate ALU to memory
		{"ADDI.W #imm,(A0)", []uint16{0x0650, 0x1234}, 1},
		{"SUBI.B #imm,(A0)+", []uint16{0x0418, 0x0001}, 1},
		{"ANDI.L #imm,-(A0)", []uint16{0x02A0, 0x0F0F, 0xF0F0}, 1},
		{"CMPI.W #imm,4(A0)", []uint16{0x0C68, 0x0100, 0x0004}, 1},
		{"EORI.B #imm,(A0)", []uint16{0x0A10, 0x00FF}, 1},
		// NEG / NOT
		{"NEG.L D1", []uint16{0x4481}, 1},
		{"NEG.B D1", []uint16{0x4401}, 1},
		{"NEG.W (A0)", []uint16{0x4450}, 1},
		{"NOT.L D1", []uint16{0x4681}, 1},
		{"NOT.B (A0)+", []uint16{0x4618}, 1},
		// SWAP / EXT
		{"SWAP D1", []uint16{0x4841}, 1},
		{"EXT.W D1", []uint16{0x4881}, 1},
		{"EXT.L D1", []uint16{0x48C1}, 1},
		{"EXTB.L D1", []uint16{0x49C1}, 1},
		// PEA
		{"PEA (A0)", []uint16{0x4850}, 1},
		{"PEA 8(A0)", []uint16{0x4868, 0x0008}, 1},
		{"PEA abs.L", []uint16{0x4879, 0x0000, 0x8030}, 1},
		// Shifts and rotates, immediate count, all sizes
		{"LSL.L #1,D1", []uint16{0xE389}, 1},
		{"LSL.B #8,D1", []uint16{0xE109}, 1},
		{"LSL.W #4,D1", []uint16{0xE949}, 1},
		{"LSR.L #1,D1", []uint16{0xE289}, 1},
		{"LSR.B #8,D1", []uint16{0xE009}, 1},
		{"LSR.W #7,D1", []uint16{0xEE49}, 1},
		{"ASL.L #2,D1", []uint16{0xE581}, 1},
		{"ASL.B #8,D1", []uint16{0xE101}, 1},
		{"ASL.W #1,D1", []uint16{0xE341}, 1},
		{"ASR.L #3,D1", []uint16{0xE681}, 1},
		{"ASR.B #8,D1", []uint16{0xE001}, 1},
		{"ASR.W #1,D1", []uint16{0xE241}, 1},
		{"ROL.L #1,D1", []uint16{0xE399}, 1},
		{"ROL.B #8,D1", []uint16{0xE119}, 1},
		{"ROL.W #4,D1", []uint16{0xE959}, 1},
		{"ROR.L #5,D1", []uint16{0xEA99}, 1},
		{"ROR.B #8,D1", []uint16{0xE019}, 1},
		{"ROR.W #1,D1", []uint16{0xE259}, 1},
		{"ROXL.L #1,D1", []uint16{0xE391}, 1},
		{"ROXL.B #8,D1", []uint16{0xE111}, 1},
		{"ROXL.W #4,D1", []uint16{0xE951}, 1},
		{"ROXR.L #1,D1", []uint16{0xE291}, 1},
		{"ROXR.B #8,D1", []uint16{0xE011}, 1},
		{"ROXR.W #7,D1", []uint16{0xEE51}, 1},
		// Mixed
		{"shift chain", []uint16{0xE389, 0xE289, 0x4481, 0x4841}, 4},
	}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 22)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	for _, tc := range cases {
		tc := tc
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				end := m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
				m68kARM64WriteWords(cpu, end, 0x4E75)
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix < tc.count {
				t.Fatalf("%s: supported prefix %d < want %d", tc.name, prefix, tc.count)
			}
			block, err := m68kCompileBlockARM64(instrs[:tc.count], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", tc.name, err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, a := range m68kARM64GridValues {
				seed := [8]uint32{a, a ^ 0x5A5A5A5A, 0x11111111, 0x22222222, 0x33333333, 0x44444444, 0x55555555, 0x66666666}
				for _, ccrIn := range []uint16{0x00, 0x1F, 0x10} {
					for _, cpu := range []*M68KCPU{ref, got} {
						m68kARM64SeedRegs(cpu, seed, ccrIn)
						m68kARM64SeedMem(cpu)
						// PEA pushes through the interpreter's Push32, which
						// enforces the stack floor; keep A7 in the valid
						// stack region for this grid.
						cpu.AddrRegs[7] = m68kARM64StackTop
						for i := uint32(1); i <= 16; i++ {
							cpu.memory[m68kARM64StackTop-i] = byte(i)
						}
						cpu.PC = m68kARM64TestPC
					}
					m68kARM64RunInterp(ref, tc.count)
					wantPC := m68kARM64TestPC + 2*uint32(len(tc.words))
					if ref.PC != wantPC {
						t.Fatalf("%s a=%08X: interpreter PC=%08X want %08X (exception?)", tc.name, a, ref.PC, wantPC)
					}
					ctx.RetPC = 0
					ctx.RetCount = 0
					ctx.NeedIOFallback = 0
					callNative(block.execAddr, m68kJITContextPtr(ctx))
					if ctx.NeedIOFallback != 0 {
						t.Fatalf("%s a=%08X: unexpected I/O bail", tc.name, a)
					}
					got.PC = ctx.RetPC
					if ctx.RetCount != uint32(tc.count) {
						t.Fatalf("%s a=%08X: retired=%d want %d", tc.name, a, ctx.RetCount, tc.count)
					}
					m68kARM64CompareState(t, tc.name, ref, got)
					m68kARM64CompareMem(t, tc.name, ref, got)
					for i := uint32(1); i <= 16; i++ {
						sa := m68kARM64StackTop - i
						if ref.memory[sa] != got.memory[sa] {
							t.Errorf("%s: stack[%08X] interp=%02X native=%02X", tc.name, sa, ref.memory[sa], got.memory[sa])
							break
						}
					}
					if t.Failed() {
						t.Fatalf("%s: first divergence at a=%08X ccrIn=%02X", tc.name, a, ccrIn)
					}
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed, stopping grid", tc.name)
		}
	}
}

// ===========================================================================
// Milestone 3 slice 4: branches — BRA, Bcc (all conditions) and DBcc as
// block-ending exits with dynamic resume PC.
// ===========================================================================

// TestM68KARM64_BranchPrefixAdmission pins the admission rule: a supported
// branch ends the native block and is included as its final instruction.
func TestM68KARM64_BranchPrefixAdmission(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	// moveq #1,d0 ; moveq #2,d1 ; beq.s +4 ; moveq #3,d2 ; rts
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7001, 0x7202, 0x6704, 0x7403, 0x4E75)
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	prefix := m68kARM64SupportedPrefix(instrs, cpu.memory, m68kARM64TestPC, cpu.ProfileTopOfRAM())
	if prefix != 3 {
		t.Fatalf("supported prefix = %d, want 3 (branch included as final instruction)", prefix)
	}
	// BRA is also included and ends the block.
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7001, 0x6006)
	instrs = m68kScanBlock(cpu.memory, m68kARM64TestPC)
	prefix = m68kARM64SupportedPrefix(instrs, cpu.memory, m68kARM64TestPC, cpu.ProfileTopOfRAM())
	if prefix != 2 {
		t.Fatalf("BRA prefix = %d, want 2", prefix)
	}
}

// TestM68KARM64_DifferentialBranchGrid drives BRA, every Bcc condition and a
// set of DBcc conditions over the operand grid. The final PC (taken or not),
// registers, CCR and retired count must match the interpreter exactly.
func TestM68KARM64_DifferentialBranchGrid(t *testing.T) {
	type opCase struct {
		name  string
		words []uint16
		count int
	}
	cases := []opCase{
		// Unconditional
		{"BRA.S +8", []uint16{0x7001, 0x6008}, 2},
		{"BRA.S -16", []uint16{0x7001, 0x60F0}, 2},
		{"BRA.W +256", []uint16{0x7001, 0x6000, 0x0100}, 2},
		{"BRA.L +1024", []uint16{0x7001, 0x60FF, 0x0000, 0x0400}, 2},
		// Bcc byte displacement, every condition, flags from CMP.L D0,D1
		{"BHI.S", []uint16{0xB280, 0x6208}, 2},
		{"BLS.S", []uint16{0xB280, 0x6308}, 2},
		{"BCC.S", []uint16{0xB280, 0x6408}, 2},
		{"BCS.S", []uint16{0xB280, 0x6508}, 2},
		{"BNE.S", []uint16{0xB280, 0x6608}, 2},
		{"BEQ.S", []uint16{0xB280, 0x6708}, 2},
		{"BVC.S", []uint16{0xB280, 0x6808}, 2},
		{"BVS.S", []uint16{0xB280, 0x6908}, 2},
		{"BPL.S", []uint16{0xB280, 0x6A08}, 2},
		{"BMI.S", []uint16{0xB280, 0x6B08}, 2},
		{"BGE.S", []uint16{0xB280, 0x6C08}, 2},
		{"BLT.S", []uint16{0xB280, 0x6D08}, 2},
		{"BGT.S", []uint16{0xB280, 0x6E08}, 2},
		{"BLE.S", []uint16{0xB280, 0x6F08}, 2},
		// Bcc word displacement and backward targets
		{"BNE.W +512", []uint16{0xB280, 0x6600, 0x0200}, 2},
		{"BEQ.W -256", []uint16{0xB280, 0x6700, 0xFF00}, 2},
		{"BLT.S -8", []uint16{0xB280, 0x6DF8}, 2},
		// DBcc: counter and condition interplay (flags from CMP.L D0,D1)
		{"DBRA D2", []uint16{0xB280, 0x51CA, 0xFFFC}, 2},
		{"DBT D2", []uint16{0xB280, 0x50CA, 0xFFFC}, 2},
		{"DBEQ D2", []uint16{0xB280, 0x57CA, 0xFFFC}, 2},
		{"DBNE D2", []uint16{0xB280, 0x56CA, 0xFFFC}, 2},
		{"DBMI D2", []uint16{0xB280, 0x5BCA, 0xFFFC}, 2},
		{"DBLT D2", []uint16{0xB280, 0x5DCA, 0xFFFC}, 2},
		{"DBGE D2 fwd", []uint16{0xB280, 0x5CCA, 0x0040}, 2},
	}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 22)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	// DBcc counter values: zero (expires), small, low-word wrap, upper-word
	// preservation.
	d2Values := []uint32{0x00000000, 0x00000001, 0x00000002, 0x0000FFFF,
		0x00010000, 0xABCD0001, 0xABCD0000}
	for _, tc := range cases {
		tc := tc
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix < tc.count {
				t.Fatalf("%s: supported prefix %d < want %d", tc.name, prefix, tc.count)
			}
			block, err := m68kCompileBlockARM64(instrs[:tc.count], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", tc.name, err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, a := range m68kARM64GridValues {
				for _, b := range m68kARM64GridValues {
					for _, d2 := range d2Values {
						seed := [8]uint32{a, b, d2, 0x22222222, 0x33333333, 0x44444444, 0x55555555, 0x66666666}
						for _, ccrIn := range []uint16{0x00, 0x1F, 0x10, 0x0A} {
							for _, cpu := range []*M68KCPU{ref, got} {
								m68kARM64SeedRegs(cpu, seed, ccrIn)
								cpu.PC = m68kARM64TestPC
							}
							ref.running.Store(true)
							m68kARM64RunInterp(ref, tc.count)
							if !ref.running.Load() {
								t.Fatalf("%s a=%08X b=%08X: interpreter halted (branch target out of profile RAM)", tc.name, a, b)
							}
							ctx.RetPC = 0
							ctx.RetCount = 0
							ctx.NeedIOFallback = 0
							callNative(block.execAddr, m68kJITContextPtr(ctx))
							if ctx.NeedIOFallback != 0 {
								t.Fatalf("%s a=%08X b=%08X: unexpected I/O bail", tc.name, a, b)
							}
							got.PC = ctx.RetPC
							if ctx.RetCount != uint32(tc.count) {
								t.Fatalf("%s a=%08X b=%08X: retired=%d want %d", tc.name, a, b, ctx.RetCount, tc.count)
							}
							m68kARM64CompareState(t, tc.name, ref, got)
							if t.Failed() {
								t.Fatalf("%s: first divergence at a=%08X b=%08X d2=%08X ccrIn=%02X", tc.name, a, b, d2, ccrIn)
							}
						}
					}
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed, stopping grid", tc.name)
		}
	}
}

// TestM68KARM64_DispatcherLoopDBRA runs a real DBRA loop through the full
// dispatcher: the loop body plus its DBRA must execute as one native block
// per iteration, with exact retired accounting against the interpreter.
func TestM68KARM64_DispatcherLoopDBRA(t *testing.T) {
	program := []uint16{
		0x7009,         // moveq #9,d0
		0x7200,         // moveq #0,d1
		0x5281,         // loop: addq.l #1,d1
		0xD481,         // add.l d1,d2
		0x51C8, 0xFFFA, // dbra d0,loop
		0x4E72, 0x2700, // stop #$2700
	}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	for _, cpu := range []*M68KCPU{ref, got} {
		m68kARM64WriteWords(cpu, m68kARM64TestPC, program...)
		cpu.PC = m68kARM64TestPC
	}

	refCount := 0
	for !ref.stopped.Load() && refCount < 1000 {
		refCount += ref.StepOne()
	}

	got.m68kJitEnabled = true
	got.PerfEnabled = true
	got.m68kJitForceNative = true
	got.StoppedIdleHook = func(c *M68KCPU) { c.running.Store(false) }
	got.running.Store(true)
	got.M68KExecuteJIT()

	m68kARM64CompareState(t, "dbra-loop", ref, got)
	if got.DataRegs[1] != 10 {
		t.Errorf("D1=%d, want 10 loop iterations", got.DataRegs[1])
	}
	if got.InstructionCount != uint64(refCount) {
		t.Errorf("retired count: interp=%d jit=%d", refCount, got.InstructionCount)
	}
}

// TestM68KARM64_BranchTargetOutOfProfileRAM pins the admission quirk: the
// interpreter halts the machine when a taken BRA/Bcc target lands beyond
// ProfileTopOfRAM-2, so the native backend must refuse such branches and
// leave them to the interpreter.
func TestM68KARM64_BranchTargetOutOfProfileRAM(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	top := cpu.ProfileTopOfRAM()
	// moveq #1,d0 ; bra.w to a target beyond top of RAM (via a tiny topOfRAM
	// override passed straight to the admission check).
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7001, 0x6000, 0x0100)
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	if got := m68kARM64SupportedPrefix(instrs, cpu.memory, m68kARM64TestPC, top); got != 2 {
		t.Fatalf("in-range BRA prefix = %d, want 2", got)
	}
	// With a top of RAM below the branch target, the branch must be rejected
	// and the prefix must stop before it.
	if got := m68kARM64SupportedPrefix(instrs, cpu.memory, m68kARM64TestPC, m68kARM64TestPC+0x40); got != 1 {
		t.Fatalf("out-of-range BRA prefix = %d, want 1 (branch rejected)", got)
	}
}

// ===========================================================================
// Milestone 3 slice 5: subroutine flow — BSR, JSR, JMP and RTS as
// block-ending exits with native stack push/pop.
// ===========================================================================

// TestM68KARM64_CallReturnPrefixAdmission pins the admission rules for the
// call and return terminators: supported shapes end the block and are
// included as its final instruction; unsupported EA forms are excluded.
func TestM68KARM64_CallReturnPrefixAdmission(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	top := cpu.ProfileTopOfRAM()
	cases := []struct {
		name  string
		words []uint16
		top   uint32
		want  int
	}{
		{"JSR (A0)", []uint16{0x7001, 0x4E90}, top, 2},
		{"JSR 16(A0)", []uint16{0x7001, 0x4EA8, 0x0010}, top, 2},
		{"JSR abs.L", []uint16{0x7001, 0x4EB9, 0x0000, 0x2000}, top, 2},
		{"JMP (A0)", []uint16{0x7001, 0x4ED0}, top, 2},
		{"RTS", []uint16{0x7001, 0x4E75}, top, 2},
		{"BSR.S", []uint16{0x7001, 0x6108}, top, 2},
		// Indexed EA is not lowered; the prefix stops before the JSR.
		{"JSR 8(A0,D0.W)", []uint16{0x7001, 0x4EB0, 0x0008}, top, 1},
		// The interpreter halts on a taken BSR target beyond top of RAM
		// (after pushing); such a BSR must stay on the interpreter.
		{"BSR.W out of RAM", []uint16{0x7001, 0x6100, 0x0100}, m68kARM64TestPC + 0x40, 1},
	}
	for _, tc := range cases {
		m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
		instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
		if got := m68kARM64SupportedPrefix(instrs, cpu.memory, m68kARM64TestPC, tc.top); got != tc.want {
			t.Errorf("%s: prefix = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestM68KARM64_DifferentialCallReturnGrid drives every lowered call and
// return shape against the interpreter: PC, all registers (A7 in
// particular), CCR, the retired count and the stack memory window must match
// exactly.
func TestM68KARM64_DifferentialCallReturnGrid(t *testing.T) {
	type opCase struct {
		name  string
		words []uint16
		count int
	}
	cases := []opCase{
		{"BSR.S +8", []uint16{0x7001, 0x6108}, 2},
		{"BSR.S -16", []uint16{0x7001, 0x61F0}, 2},
		{"BSR.W +256", []uint16{0x7001, 0x6100, 0x0100}, 2},
		{"BSR.L +1024", []uint16{0x7001, 0x61FF, 0x0000, 0x0400}, 2},
		{"JSR (A0)", []uint16{0x7001, 0x4E90}, 2},
		{"JSR 16(A0)", []uint16{0x7001, 0x4EA8, 0x0010}, 2},
		{"JSR -8(A1)", []uint16{0x7001, 0x4EA9, 0xFFF8}, 2},
		{"JSR abs.L", []uint16{0x7001, 0x4EB9, 0x0000, 0x2000}, 2},
		{"JMP (A0)", []uint16{0x7001, 0x4ED0}, 2},
		{"JMP 16(A0)", []uint16{0x7001, 0x4EE8, 0x0010}, 2},
		{"JMP -8(A1)", []uint16{0x7001, 0x4EE9, 0xFFF8}, 2},
		{"JMP abs.L", []uint16{0x7001, 0x4EF9, 0x0000, 0x2000}, 2},
		{"RTS", []uint16{0x7001, 0x4E75}, 2},
	}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 21)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	// Stack pointer seeds inside the interpreter's valid stack region
	// (Push32 enforces a stack floor well above the data buffer).
	spSeeds := []uint32{m68kARM64StackTop, m68kARM64StackTop + 0x40}
	for _, tc := range cases {
		tc := tc
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix != tc.count {
				t.Fatalf("%s: supported prefix %d, want %d", tc.name, prefix, tc.count)
			}
			block, err := m68kCompileBlockARM64(instrs[:tc.count], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", tc.name, err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, sp := range spSeeds {
				for _, ccrIn := range []uint16{0x00, 0x1F, 0x0A} {
					for _, cpu := range []*M68KCPU{ref, got} {
						m68kARM64SeedRegs(cpu, [8]uint32{1, 2, 3, 4, 5, 6, 7, 8}, ccrIn)
						m68kARM64SeedMem(cpu)
						cpu.AddrRegs[7] = sp
						// Return address for RTS: an even in-RAM target.
						cpu.memory[sp] = 0x00
						cpu.memory[sp+1] = 0x00
						cpu.memory[sp+2] = 0x20
						cpu.memory[sp+3] = 0x40
						cpu.PC = m68kARM64TestPC
					}
					ref.running.Store(true)
					m68kARM64RunInterp(ref, tc.count)
					if !ref.running.Load() {
						t.Fatalf("%s sp=%08X: interpreter halted unexpectedly", tc.name, sp)
					}
					ctx.RetPC = 0
					ctx.RetCount = 0
					ctx.NeedIOFallback = 0
					callNative(block.execAddr, m68kJITContextPtr(ctx))
					if ctx.NeedIOFallback != 0 {
						t.Fatalf("%s sp=%08X: unexpected I/O bail", tc.name, sp)
					}
					got.PC = ctx.RetPC
					if ctx.RetCount != uint32(tc.count) {
						t.Fatalf("%s sp=%08X: retired=%d want %d", tc.name, sp, ctx.RetCount, tc.count)
					}
					m68kARM64CompareState(t, tc.name, ref, got)
					m68kARM64CompareMem(t, tc.name, ref, got)
					for i := uint32(0); i < 16; i++ {
						a := sp - 8 + i
						if ref.memory[a] != got.memory[a] {
							t.Errorf("%s: stack[%08X] interp=%02X native=%02X", tc.name, a, ref.memory[a], got.memory[a])
							break
						}
					}
					if t.Failed() {
						t.Fatalf("%s: first divergence at sp=%08X ccrIn=%02X", tc.name, sp, ccrIn)
					}
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed, stopping grid", tc.name)
		}
	}
}

// TestM68KARM64_JSRStackBailUnit pins the pre-commit bail contract for the
// native stack push: a JSR whose push lands on an I/O page must exit with
// NeedIOFallback before any side effect (no A7 change, no store, no jump).
func TestM68KARM64_JSRStackBailUnit(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	// moveq #5,d0 ; jsr $2000.l
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7005, 0x4EB9, 0x0000, 0x2000)
	cpu.m68kJitIOPageBitmap = make([]bool, (uint32(len(cpu.memory))+255)>>8)
	ioPage := uint32(0x9000)
	cpu.m68kJitIOPageBitmap[ioPage>>8] = true

	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	ctx := newM68KJITContext(cpu, nil, nil, nil)

	// SP such that the push (SP-4) lands inside the I/O page.
	cpu.AddrRegs[7] = ioPage + 8
	cpu.DataRegs[0] = 0
	callNative(block.execAddr, m68kJITContextPtr(ctx))

	if ctx.NeedIOFallback != 1 {
		t.Fatalf("NeedIOFallback=%d, want 1", ctx.NeedIOFallback)
	}
	if ctx.RetPC != m68kARM64TestPC+2 {
		t.Fatalf("bail RetPC=%08X, want %08X (the JSR)", ctx.RetPC, m68kARM64TestPC+2)
	}
	if ctx.RetCount != 1 {
		t.Fatalf("bail RetCount=%d, want 1", ctx.RetCount)
	}
	if cpu.AddrRegs[7] != ioPage+8 {
		t.Fatalf("A7=%08X changed by bailed JSR", cpu.AddrRegs[7])
	}
	if cpu.DataRegs[0] != 5 {
		t.Fatalf("D0=%08X, want 5 (moveq before the bail must commit)", cpu.DataRegs[0])
	}
}

// TestM68KARM64_DispatcherCallReturn runs a real call/return pair through the
// full dispatcher: the caller block ends in JSR, the callee block ends in
// RTS, and the whole flow must match the interpreter exactly, including the
// retired-instruction count.
func TestM68KARM64_DispatcherCallReturn(t *testing.T) {
	program := []uint16{
		0x7005,                 // 1000: moveq #5,d0
		0x4EB9, 0x0000, 0x1010, // 1002: jsr $1010.l
		0x5281,         // 1008: addq.l #1,d1
		0x4E72, 0x2700, // 100A: stop #$2700
		0x4E71, // 100E: nop (pad)
		0x5480, // 1010: addq.l #2,d0
		0x7403, // 1012: moveq #3,d2
		0x4E75, // 1014: rts
	}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	for _, cpu := range []*M68KCPU{ref, got} {
		m68kARM64WriteWords(cpu, m68kARM64TestPC, program...)
		cpu.PC = m68kARM64TestPC
		cpu.AddrRegs[7] = m68kARM64StackTop
	}

	refCount := 0
	for !ref.stopped.Load() && refCount < 100 {
		refCount += ref.StepOne()
	}

	got.m68kJitEnabled = true
	got.PerfEnabled = true
	got.m68kJitForceNative = true
	got.StoppedIdleHook = func(c *M68KCPU) { c.running.Store(false) }
	got.running.Store(true)
	got.M68KExecuteJIT()

	m68kARM64CompareState(t, "call-return", ref, got)
	if got.DataRegs[0] != 7 || got.DataRegs[1] != 1 || got.DataRegs[2] != 3 {
		t.Errorf("D0=%d D1=%d D2=%d, want 7 1 3", got.DataRegs[0], got.DataRegs[1], got.DataRegs[2])
	}
	if got.InstructionCount != uint64(refCount) {
		t.Errorf("retired count: interp=%d jit=%d", refCount, got.InstructionCount)
	}
}

// TestM68KARM64_CrossPageIOGuard pins the two-page guard policy: a multi-byte
// guest access that starts on a plain RAM page but whose final byte lands on
// an I/O page must bail to the interpreter, not write (or read) through the
// backing array. Covers the guarded stack push (JSR), the guarded stack pop
// (RTS) and a plain data store, since they all share emitGuard.
func TestM68KARM64_CrossPageIOGuard(t *testing.T) {
	type shape struct {
		name  string
		words []uint16
		count int
		setup func(cpu *M68KCPU, ioPage uint32)
	}
	shapes := []shape{
		{
			// Push starts at ioPage-2, bytes ioPage-2..ioPage+1.
			name:  "jsr-push-cross",
			words: []uint16{0x7005, 0x4EB9, 0x0000, 0x2000}, // moveq #5,d0 ; jsr $2000.l
			count: 2,
			setup: func(cpu *M68KCPU, ioPage uint32) { cpu.AddrRegs[7] = ioPage + 2 },
		},
		{
			// Pop reads ioPage-2..ioPage+1.
			name:  "rts-pop-cross",
			words: []uint16{0x7005, 0x4E75}, // moveq #5,d0 ; rts
			count: 2,
			setup: func(cpu *M68KCPU, ioPage uint32) { cpu.AddrRegs[7] = ioPage - 2 },
		},
		{
			// Data store crossing: move.l d0,(a0) with a0 = ioPage-2.
			name:  "move-store-cross",
			words: []uint16{0x7005, 0x2080}, // moveq #5,d0 ; move.l d0,(a0)
			count: 2,
			setup: func(cpu *M68KCPU, ioPage uint32) { cpu.AddrRegs[0] = ioPage - 2 },
		},
	}
	for _, sh := range shapes {
		t.Run(sh.name, func(t *testing.T) {
			cpu := m68kARM64NewCPU(t)
			m68kARM64WriteWords(cpu, m68kARM64TestPC, sh.words...)
			cpu.m68kJitIOPageBitmap = make([]bool, (uint32(len(cpu.memory))+255)>>8)
			ioPage := uint32(0x9000)
			cpu.m68kJitIOPageBitmap[ioPage>>8] = true
			sh.setup(cpu, ioPage)
			a7Before := cpu.AddrRegs[7]

			instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
			execMem, err := AllocExecMem(1 << 20)
			if err != nil {
				t.Fatalf("AllocExecMem: %v", err)
			}
			defer execMem.Free()
			block, err := m68kCompileBlockARM64(instrs[:sh.count], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			ctx := newM68KJITContext(cpu, nil, nil, nil)
			callNative(block.execAddr, m68kJITContextPtr(ctx))

			if ctx.NeedIOFallback != 1 {
				t.Fatalf("NeedIOFallback=%d, want 1 (cross-page access must bail)", ctx.NeedIOFallback)
			}
			if ctx.RetPC != m68kARM64TestPC+2 {
				t.Fatalf("bail RetPC=%08X, want %08X", ctx.RetPC, m68kARM64TestPC+2)
			}
			if ctx.RetCount != 1 {
				t.Fatalf("bail RetCount=%d, want 1", ctx.RetCount)
			}
			if cpu.AddrRegs[7] != a7Before {
				t.Fatalf("A7=%08X changed by bailed instruction", cpu.AddrRegs[7])
			}
			if cpu.DataRegs[0] != 5 {
				t.Fatalf("D0=%08X, want 5", cpu.DataRegs[0])
			}
			// The I/O page bytes must be untouched.
			for off := uint32(0); off < 4; off++ {
				if cpu.memory[ioPage+off] != 0 {
					t.Fatalf("I/O page byte %08X = %02X, want untouched", ioPage+off, cpu.memory[ioPage+off])
				}
			}
		})
	}
}

// TestM68KARM64_StackBoundBail pins the interpreter's configured stack-bound
// exceptions on the native call/return paths: Push32 raises a bus error when
// the decremented A7 falls below cpu.stackLowerBound, Pop32 when A7 is at or
// above cpu.stackUpperBound, even though both addresses are valid guest RAM.
// The native BSR/JSR/RTS paths must bail (instruction unexecuted) so the
// interpreter fallback delivers the exact exception.
func TestM68KARM64_StackBoundBail(t *testing.T) {
	type shape struct {
		name  string
		words []uint16
		setup func(cpu *M68KCPU)
	}
	shapes := []shape{
		{
			// Push at A7-4 = 0x7FFC, below the 0x8000 floor but valid RAM.
			name:  "bsr-below-floor",
			words: []uint16{0x7005, 0x6100, 0x0010}, // moveq #5,d0 ; bsr.w +16
			setup: func(cpu *M68KCPU) {
				cpu.stackLowerBound = 0x8000
				cpu.AddrRegs[7] = 0x8000
			},
		},
		{
			// JSR shares emitPushRet with BSR.
			name:  "jsr-below-floor",
			words: []uint16{0x7005, 0x4EB9, 0x0000, 0x2000}, // moveq #5,d0 ; jsr $2000.l
			setup: func(cpu *M68KCPU) {
				cpu.stackLowerBound = 0x8000
				cpu.AddrRegs[7] = 0x8000
			},
		},
		{
			// PEA pushes through its own path, not emitPushRet.
			name:  "pea-below-floor",
			words: []uint16{0x7005, 0x4850}, // moveq #5,d0 ; pea (a0)
			setup: func(cpu *M68KCPU) {
				cpu.stackLowerBound = 0x8000
				cpu.AddrRegs[7] = 0x8000
				cpu.AddrRegs[0] = 0x4000
			},
		},
		{
			// Pop with A7 at the ceiling: valid RAM, but Pop32 bus-errors.
			name:  "rts-at-ceiling",
			words: []uint16{0x7005, 0x4E75}, // moveq #5,d0 ; rts
			setup: func(cpu *M68KCPU) {
				cpu.stackUpperBound = 0x8000
				cpu.AddrRegs[7] = 0x8000
			},
		},
	}
	for _, sh := range shapes {
		t.Run(sh.name, func(t *testing.T) {
			cpu := m68kARM64NewCPU(t)
			m68kARM64WriteWords(cpu, m68kARM64TestPC, sh.words...)
			sh.setup(cpu)
			a7Before := cpu.AddrRegs[7]

			instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
			execMem, err := AllocExecMem(1 << 20)
			if err != nil {
				t.Fatalf("AllocExecMem: %v", err)
			}
			defer execMem.Free()
			block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			ctx := newM68KJITContext(cpu, nil, nil, nil)
			callNative(block.execAddr, m68kJITContextPtr(ctx))

			if ctx.NeedIOFallback != 1 {
				t.Fatalf("NeedIOFallback=%d, want 1 (stack-bound violation must bail)", ctx.NeedIOFallback)
			}
			if ctx.RetPC != m68kARM64TestPC+2 {
				t.Fatalf("bail RetPC=%08X, want %08X", ctx.RetPC, m68kARM64TestPC+2)
			}
			if ctx.RetCount != 1 {
				t.Fatalf("bail RetCount=%d, want 1", ctx.RetCount)
			}
			if cpu.AddrRegs[7] != a7Before {
				t.Fatalf("A7=%08X changed by bailed instruction", cpu.AddrRegs[7])
			}
			if cpu.DataRegs[0] != 5 {
				t.Fatalf("D0=%08X, want 5", cpu.DataRegs[0])
			}
		})
	}
}

// TestM68KARM64_DifferentialExtendedEAGrid exercises the slice-6 effective
// address formats: absolute short, (d16,PC), and the brief-format index modes
// (d8,An,Xn) and (d8,PC,Xn) with word/long index size and scale, comparing
// native execution against the interpreter over an index-value grid.
func TestM68KARM64_DifferentialExtendedEAGrid(t *testing.T) {
	const absW = uint32(0x0400) // small positive absolute-short scratch
	seed := func(cpu *M68KCPU) {
		for i := uint32(0); i < 0x100; i++ {
			cpu.memory[m68kARM64BufBase+i] = byte(i*7 + 3)
			cpu.memory[absW+i] = byte(i*5 + 9)
		}
		cpu.AddrRegs[0] = m68kARM64BufBase + 0x40
		cpu.AddrRegs[1] = m68kARM64BufBase + 0x60
		cpu.DataRegs[0] = 0x0BADF00D
		cpu.DataRegs[1] = 0x1234ABCD
	}
	cmpMem := func(t *testing.T, name string, ref, got *M68KCPU) {
		t.Helper()
		for _, base := range []uint32{absW, m68kARM64BufBase} {
			for i := uint32(0); i < 0x100; i++ {
				if ref.memory[base+i] != got.memory[base+i] {
					t.Errorf("%s: mem[%08X] interp=%02X native=%02X", name, base+i,
						ref.memory[base+i], got.memory[base+i])
					return
				}
			}
		}
	}
	type opCase struct {
		name   string
		words  []uint16
		idxVal uint32 // seeded into D2 (the index register)
	}
	cases := []opCase{
		{"MOVE.L (0,A0,D2.W),D1", []uint16{0x2230, 0x2000}, 0x0000},
		{"MOVE.L (8,A0,D2.W),D1", []uint16{0x2230, 0x2008}, 0x0004},
		{"MOVE.L (0,A0,D2.W)-neg,D1", []uint16{0x2230, 0x2000}, 0xFFFFFFFC},
		{"MOVE.L (0,A0,D2.W)-hibits,D1", []uint16{0x2230, 0x2000}, 0x00010004},
		{"MOVE.L (0,A0,D2.L),D1", []uint16{0x2230, 0x2800}, 0x00000008},
		{"MOVE.L (0,A0,D2.L*4),D1", []uint16{0x2230, 0x2C00}, 0x00000004},
		{"MOVE.W (0,A0,D2.W),D1", []uint16{0x3230, 0x2000}, 0x0006},
		{"MOVE.B (0,A0,D2.W),D1", []uint16{0x1230, 0x2000}, 0x0003},
		{"MOVE.L (0x0400).W,D1", []uint16{0x2238, 0x0400}, 0},
		{"MOVE.W (0x0400).W,D1", []uint16{0x3238, 0x0400}, 0},
		{"MOVE.W D1,(0x0400).W", []uint16{0x31C1, 0x0400}, 0},
		{"MOVE.L (0,PC),D1", []uint16{0x223A, 0x0000}, 0},
		{"MOVE.L (0,PC,D2.W),D1", []uint16{0x223B, 0x2000}, 0x0000},
		{"LEA (8,A0,D2.W),A3", []uint16{0x47F0, 0x2008}, 0x0004},
		{"LEA (0,PC,D2.W),A3", []uint16{0x47FB, 0x2000}, 0x0000},
		{"ADD.L (0,A0,D2.W),D1", []uint16{0xD2B0, 0x2000}, 0x0008},
		{"TST.L (0,A0,D2.W)", []uint16{0x4AB0, 0x2000}, 0x0004},
	}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 22)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	for _, tc := range cases {
		tc := tc
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				end := m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
				m68kARM64WriteWords(cpu, end, 0x4E75)
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix < 1 {
				t.Fatalf("%s: supported prefix %d < 1 (mode not admitted)", tc.name, prefix)
			}
			block, err := m68kCompileBlockARM64(instrs[:1], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", tc.name, err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, ccrIn := range []uint16{0x00, 0x1F, 0x10} {
				for _, cpu := range []*M68KCPU{ref, got} {
					seed(cpu)
					cpu.DataRegs[2] = tc.idxVal
					cpu.SR = (cpu.SR &^ 0x1F) | ccrIn
					cpu.PC = m68kARM64TestPC
				}
				m68kARM64RunInterp(ref, 1)
				wantPC := m68kARM64TestPC + 2*uint32(len(tc.words))
				if ref.PC != wantPC {
					t.Fatalf("%s: interpreter PC=%08X want %08X (exception?)", tc.name, ref.PC, wantPC)
				}
				ctx.RetPC = 0
				ctx.RetCount = 0
				ctx.NeedIOFallback = 0
				callNative(block.execAddr, m68kJITContextPtr(ctx))
				if ctx.NeedIOFallback != 0 {
					t.Fatalf("%s: unexpected I/O bail (idx=%08X)", tc.name, tc.idxVal)
				}
				got.PC = ctx.RetPC
				if ctx.RetCount != 1 {
					t.Fatalf("%s: retired=%d want 1", tc.name, ctx.RetCount)
				}
				m68kARM64CompareState(t, tc.name, ref, got)
				cmpMem(t, tc.name, ref, got)
				if t.Failed() {
					t.Fatalf("%s: divergence at ccrIn=%02X idx=%08X", tc.name, ccrIn, tc.idxVal)
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed, stopping grid", tc.name)
		}
	}
}

// TestM68KARM64_DifferentialMemShiftGrid exercises the slice-6 memory
// shift/rotate-by-one forms across every operation, comparing native and
// interpreter results including the C/X/V flag behaviour.
func TestM68KARM64_DifferentialMemShiftGrid(t *testing.T) {
	type opCase struct {
		name  string
		words []uint16
	}
	cases := []opCase{
		{"ASR.W (A0)", []uint16{0xE0D0}},
		{"ASL.W (A0)", []uint16{0xE1D0}},
		{"LSR.W (A0)", []uint16{0xE2D0}},
		{"LSL.W (A0)", []uint16{0xE3D0}},
		{"ROXR.W (A0)", []uint16{0xE4D0}},
		{"ROXL.W (A0)", []uint16{0xE5D0}},
		{"ROR.W (A0)", []uint16{0xE6D0}},
		{"ROL.W (A0)", []uint16{0xE7D0}},
		{"ASL.W (A0)+", []uint16{0xE1D8}},
		{"LSR.W 4(A0)", []uint16{0xE2E8, 0x0004}},
		{"ROXL.W (0x0400).W", []uint16{0xE5F8, 0x0400}},
	}
	words := []uint16{0x0000, 0x0001, 0x8000, 0xFFFF, 0x1234, 0x8001, 0x7FFF, 0xAAAA}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	for _, tc := range cases {
		tc := tc
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				end := m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
				m68kARM64WriteWords(cpu, end, 0x4E75)
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix < 1 {
				t.Fatalf("%s: not admitted (prefix %d)", tc.name, prefix)
			}
			block, err := m68kCompileBlockARM64(instrs[:1], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", tc.name, err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, w := range words {
				for _, ccrIn := range []uint16{0x00, 0x1F, 0x10, 0x00} {
					for _, cpu := range []*M68KCPU{ref, got} {
						m68kARM64SeedMem(cpu)
						cpu.memory[m68kARM64BufBase+0x10] = byte(w >> 8)
						cpu.memory[m68kARM64BufBase+0x11] = byte(w)
						cpu.memory[m68kARM64BufBase+0x14] = byte(w >> 8)
						cpu.memory[m68kARM64BufBase+0x15] = byte(w)
						cpu.memory[0x0400] = byte(w >> 8)
						cpu.memory[0x0401] = byte(w)
						cpu.SR = (cpu.SR &^ 0x1F) | ccrIn
						cpu.PC = m68kARM64TestPC
					}
					m68kARM64RunInterp(ref, 1)
					ctx.RetPC = 0
					ctx.RetCount = 0
					ctx.NeedIOFallback = 0
					callNative(block.execAddr, m68kJITContextPtr(ctx))
					if ctx.NeedIOFallback != 0 {
						t.Fatalf("%s: unexpected bail (w=%04X)", tc.name, w)
					}
					got.PC = ctx.RetPC
					m68kARM64CompareState(t, tc.name, ref, got)
					for _, a := range []uint32{m68kARM64BufBase + 0x10, m68kARM64BufBase + 0x14, 0x0400} {
						if ref.memory[a] != got.memory[a] || ref.memory[a+1] != got.memory[a+1] {
							t.Errorf("%s: mem[%08X] interp=%02X%02X native=%02X%02X", tc.name, a,
								ref.memory[a], ref.memory[a+1], got.memory[a], got.memory[a+1])
						}
					}
					if t.Failed() {
						t.Fatalf("%s: divergence at w=%04X ccrIn=%02X", tc.name, w, ccrIn)
					}
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed, stopping grid", tc.name)
		}
	}
}

// TestM68KARM64_DifferentialShiftRegGrid drives every register-count shift and
// rotate (all eight operations, all three sizes, both directions) over a grid
// of destination values, count-register values and incoming CCR. The count
// grid deliberately spans zero (whole-CCR preservation), sub-width, the
// exact-width clamp boundary, super-width, and the &0x3F wrap so the runtime
// clamp/modulo paths in emitShiftReg are all exercised against ExecShiftRotate.
func TestM68KARM64_DifferentialShiftRegGrid(t *testing.T) {
	type opCase struct {
		name string
		tt   uint16 // bits 4-3: 00=AS 01=LS 10=ROX 11=RO
		dir  uint16 // bit 8: 1=left 0=right
	}
	ops := []opCase{
		{"ASR", 0, 0}, {"ASL", 0, 1},
		{"LSR", 1, 0}, {"LSL", 1, 1},
		{"ROXR", 2, 0}, {"ROXL", 2, 1},
		{"ROR", 3, 0}, {"ROL", 3, 1},
	}
	sizes := []struct {
		name string
		bits uint16
	}{{"B", 0}, {"W", 1}, {"L", 2}}
	values := []uint32{
		0x00000000, 0x00000001, 0x00000080, 0x0000FFFF, 0x00008000,
		0x7FFFFFFF, 0x80000000, 0xFFFFFFFF, 0x12345678, 0xAAAAAAAA,
	}
	counts := []uint32{0, 1, 2, 7, 8, 9, 15, 16, 17, 31, 32, 33, 40, 63}
	ccrs := []uint16{0x00, 0x1F, 0x10}

	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	// Count register D7, destination register D0.
	for _, op := range ops {
		for _, sz := range sizes {
			word := uint16(0xE000) | (7 << 9) | (op.dir << 8) | (sz.bits << 6) | 0x20 | (op.tt << 3)
			name := op.name + "." + sz.name + " D7,D0"
			ok := t.Run(name, func(t *testing.T) {
				for _, cpu := range []*M68KCPU{ref, got} {
					end := m68kARM64WriteWords(cpu, m68kARM64TestPC, word)
					m68kARM64WriteWords(cpu, end, 0x4E75)
				}
				instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
				prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
				if prefix < 1 {
					t.Fatalf("%s: not admitted (prefix %d)", name, prefix)
				}
				block, err := m68kCompileBlockARM64(instrs[:1], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
				if err != nil {
					t.Fatalf("%s: compile: %v", name, err)
				}
				ctx := newM68KJITContext(got, nil, nil, nil)
				for _, v := range values {
					for _, c := range counts {
						for _, ccrIn := range ccrs {
							for _, cpu := range []*M68KCPU{ref, got} {
								cpu.DataRegs[0] = v
								cpu.DataRegs[7] = c
								cpu.SR = (cpu.SR &^ 0x1F) | ccrIn
								cpu.PC = m68kARM64TestPC
							}
							m68kARM64RunInterp(ref, 1)
							ctx.RetPC = 0
							ctx.RetCount = 0
							ctx.NeedIOFallback = 0
							callNative(block.execAddr, m68kJITContextPtr(ctx))
							if ctx.NeedIOFallback != 0 {
								t.Fatalf("%s: unexpected bail v=%08X c=%d", name, v, c)
							}
							got.PC = ctx.RetPC
							m68kARM64CompareState(t, name, ref, got)
							if t.Failed() {
								t.Fatalf("%s: divergence v=%08X c=%d ccrIn=%02X", name, v, c, ccrIn)
							}
						}
					}
				}
			})
			if !ok {
				t.Fatalf("%s: subtest failed, stopping grid", name)
			}
		}
	}
}

// TestM68KARM64_DifferentialMulDivGrid drives MULU/MULS/DIVU/DIVS word forms
// over an operand grid (register, immediate and memory sources), comparing
// native and interpreter results. Zero divisors are covered separately by the
// bail test; overflow cases arise naturally from the grid.
func TestM68KARM64_DifferentialMulDivGrid(t *testing.T) {
	type opCase struct {
		name   string
		words  []uint16
		isDiv  bool
		memSrc bool
	}
	cases := []opCase{
		{"MULU.W D0,D1", []uint16{0xC2C0}, false, false},
		{"MULS.W D0,D1", []uint16{0xC3C0}, false, false},
		{"MULU.W #$1234,D1", []uint16{0xC2FC, 0x1234}, false, false},
		{"MULS.W #$FFFE,D1", []uint16{0xC3FC, 0xFFFE}, false, false},
		{"MULU.W (A0),D1", []uint16{0xC2D0}, false, true},
		{"DIVU.W D0,D1", []uint16{0x82C0}, true, false},
		{"DIVS.W D0,D1", []uint16{0x83C0}, true, false},
		{"DIVU.W #$0007,D1", []uint16{0x82FC, 0x0007}, true, false},
		{"DIVS.W #$FFF9,D1", []uint16{0x83FC, 0xFFF9}, true, false},
		{"DIVU.W (A0),D1", []uint16{0x82D0}, true, true},
	}
	vals := []uint32{
		0x00000000, 0x00000001, 0x00000007, 0x0000FFFF, 0x00008000,
		0x00010000, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF, 0x0003ABCD,
		0x00000003, 0x12345678, 0xFFFF0001,
	}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	for _, tc := range cases {
		tc := tc
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				end := m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
				m68kARM64WriteWords(cpu, end, 0x4E75)
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix < 1 {
				t.Fatalf("%s: not admitted (prefix %d)", tc.name, prefix)
			}
			block, err := m68kCompileBlockARM64(instrs[:1], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", tc.name, err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, a := range vals {
				for _, b := range vals {
					// Skip zero divisor for DIV register-source cases.
					if tc.isDiv && !tc.memSrc && (a&0xFFFF) == 0 {
						continue
					}
					for _, ccrIn := range []uint16{0x00, 0x1F, 0x10} {
						for _, cpu := range []*M68KCPU{ref, got} {
							cpu.DataRegs[0] = a
							cpu.DataRegs[1] = b
							if tc.memSrc {
								cpu.AddrRegs[0] = m68kARM64BufBase + 0x10
								div := uint16(a)
								if tc.isDiv && div == 0 {
									div = 1
								}
								cpu.memory[m68kARM64BufBase+0x10] = byte(div >> 8)
								cpu.memory[m68kARM64BufBase+0x11] = byte(div)
							}
							cpu.SR = (cpu.SR &^ 0x1F) | ccrIn
							cpu.PC = m68kARM64TestPC
						}
						m68kARM64RunInterp(ref, 1)
						wantPC := m68kARM64TestPC + 2*uint32(len(tc.words))
						if ref.PC != wantPC {
							t.Fatalf("%s: interp PC=%08X want %08X (a=%08X b=%08X)", tc.name, ref.PC, wantPC, a, b)
						}
						ctx.RetPC = 0
						ctx.RetCount = 0
						ctx.NeedIOFallback = 0
						callNative(block.execAddr, m68kJITContextPtr(ctx))
						if ctx.NeedIOFallback != 0 {
							t.Fatalf("%s: unexpected bail (a=%08X b=%08X)", tc.name, a, b)
						}
						got.PC = ctx.RetPC
						m68kARM64CompareState(t, tc.name, ref, got)
						if t.Failed() {
							t.Fatalf("%s: divergence a=%08X b=%08X ccrIn=%02X", tc.name, a, b, ccrIn)
						}
					}
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed, stopping grid", tc.name)
		}
	}
}

// TestM68KARM64_DifferentialMulDivLGrid drives the 68020 long multiply and
// divide (MULU.L/MULS.L and DIVU.L/DIVS.L, 32-bit and 64-bit forms, register,
// immediate and memory sources) over an operand grid. Overflow (both the
// quotient-out-of-range and INT_MIN/-1 cases) and the 32-bit single-register
// DIVL encoding arise naturally; zero divisors are covered by the bail test.
func TestM68KARM64_DifferentialMulDivLGrid(t *testing.T) {
	type opCase struct {
		name   string
		words  []uint16
		srcReg int // register source Dn, or -1 for immediate/memory
		dstLo  int // Dl (mul) or Dq (div)
		dstHi  int // Dh (mul) or Dr (div); -1 if the 32-bit form uses only dstLo
		isDiv  bool
		wide   bool
		memSrc bool
	}
	cases := []opCase{
		{"MULU.L D2,D3", []uint16{0x4C02, 0x3000}, 2, 3, -1, false, false, false},
		{"MULS.L D2,D3", []uint16{0x4C02, 0x3800}, 2, 3, -1, false, false, false},
		{"MULU.L D2,D5:D4", []uint16{0x4C02, 0x4405}, 2, 4, 5, false, true, false},
		{"MULS.L D2,D5:D4", []uint16{0x4C02, 0x4C05}, 2, 4, 5, false, true, false},
		{"MULU.L #$00010001,D3", []uint16{0x4C3C, 0x3000, 0x0001, 0x0001}, -1, 3, -1, false, false, false},
		{"MULU.L (A0),D3", []uint16{0x4C10, 0x3000}, -1, 3, -1, false, false, true},
		{"DIVU.L D2,D3", []uint16{0x4C42, 0x3004}, 2, 3, 4, true, false, false},
		{"DIVS.L D2,D3", []uint16{0x4C42, 0x3804}, 2, 3, 4, true, false, false},
		{"DIVU.L D2,D3 (Dr==Dq)", []uint16{0x4C42, 0x3003}, 2, 3, 3, true, false, false},
		{"DIVU.L D2,D5:D4", []uint16{0x4C42, 0x4405}, 2, 4, 5, true, true, false},
		{"DIVS.L D2,D5:D4", []uint16{0x4C42, 0x4C05}, 2, 4, 5, true, true, false},
		{"DIVU.L #$0007,D3", []uint16{0x4C7C, 0x3004, 0x0000, 0x0007}, -1, 3, 4, true, false, false},
		{"DIVS.L (A0),D3", []uint16{0x4C50, 0x3804}, -1, 3, 4, true, false, true},
	}
	vals := []uint32{
		0x00000000, 0x00000001, 0x00000007, 0x0000FFFF, 0x00008000,
		0x00010000, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF, 0x0003ABCD,
		0x00000003, 0x12345678,
	}
	his := []uint32{0x00000000, 0x00000001, 0x7FFFFFFF, 0x80000000, 0xFFFFFFFF}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	for _, tc := range cases {
		tc := tc
		hiGrid := []uint32{0}
		if tc.isDiv && tc.wide {
			hiGrid = his
		}
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				end := m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
				m68kARM64WriteWords(cpu, end, 0x4E75)
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix < 1 {
				t.Fatalf("%s: not admitted (prefix %d)", tc.name, prefix)
			}
			block, err := m68kCompileBlockARM64(instrs[:1], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", tc.name, err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, a := range vals {
				for _, b := range vals {
					// Zero divisor bails; covered separately.
					if tc.isDiv && !tc.memSrc && tc.srcReg >= 0 && b == 0 {
						continue
					}
					for _, h := range hiGrid {
						for _, cpu := range []*M68KCPU{ref, got} {
							cpu.DataRegs[tc.dstLo] = a
							if tc.dstHi >= 0 {
								cpu.DataRegs[tc.dstHi] = h
							}
							if tc.srcReg >= 0 {
								cpu.DataRegs[tc.srcReg] = b
							}
							if tc.memSrc {
								cpu.AddrRegs[0] = m68kARM64BufBase + 0x10
								src := b
								if tc.isDiv && src == 0 {
									src = 1
								}
								cpu.memory[m68kARM64BufBase+0x10] = byte(src >> 24)
								cpu.memory[m68kARM64BufBase+0x11] = byte(src >> 16)
								cpu.memory[m68kARM64BufBase+0x12] = byte(src >> 8)
								cpu.memory[m68kARM64BufBase+0x13] = byte(src)
							}
							cpu.SR = (cpu.SR &^ 0x1F) | 0x10
							cpu.PC = m68kARM64TestPC
						}
						m68kARM64RunInterp(ref, 1)
						wantPC := m68kARM64TestPC + 2*uint32(len(tc.words))
						if ref.PC != wantPC {
							t.Fatalf("%s: interp PC=%08X want %08X (a=%08X b=%08X h=%08X)", tc.name, ref.PC, wantPC, a, b, h)
						}
						ctx.RetPC = 0
						ctx.RetCount = 0
						ctx.NeedIOFallback = 0
						callNative(block.execAddr, m68kJITContextPtr(ctx))
						if ctx.NeedIOFallback != 0 {
							t.Fatalf("%s: unexpected bail (a=%08X b=%08X h=%08X)", tc.name, a, b, h)
						}
						got.PC = ctx.RetPC
						m68kARM64CompareState(t, tc.name, ref, got)
						if t.Failed() {
							t.Fatalf("%s: divergence a=%08X b=%08X h=%08X", tc.name, a, b, h)
						}
					}
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed, stopping grid", tc.name)
		}
	}
}

// TestM68KARM64_DivZeroBail pins the zero-divide contract: a DIV with a zero
// divisor must bail before touching Dn so the interpreter raises the trap.
func TestM68KARM64_DivZeroBail(t *testing.T) {
	for _, tc := range []struct {
		name  string
		words []uint16
	}{
		{"DIVU.W D0,D1 zero", []uint16{0x7405, 0x82C0}}, // moveq #5,d0 (clobbered by div src? no) ...
		{"DIVS.W D0,D1 zero", []uint16{0x7405, 0x83C0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cpu := m68kARM64NewCPU(t)
			m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
			m68kARM64WriteWords(cpu, m68kARM64TestPC+uint32(len(tc.words))*2, 0x4E75)
			cpu.DataRegs[0] = 0x00000000 // zero divisor (low word)
			cpu.DataRegs[1] = 0x12345678
			d1Before := cpu.DataRegs[1]

			instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
			execMem, err := AllocExecMem(1 << 20)
			if err != nil {
				t.Fatalf("AllocExecMem: %v", err)
			}
			defer execMem.Free()
			block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			ctx := newM68KJITContext(cpu, nil, nil, nil)
			callNative(block.execAddr, m68kJITContextPtr(ctx))
			if ctx.NeedIOFallback != 1 {
				t.Fatalf("NeedIOFallback=%d, want 1 (zero divide must bail)", ctx.NeedIOFallback)
			}
			if ctx.RetPC != m68kARM64TestPC+2 {
				t.Fatalf("bail RetPC=%08X, want %08X (the DIV)", ctx.RetPC, m68kARM64TestPC+2)
			}
			if ctx.RetCount != 1 {
				t.Fatalf("bail RetCount=%d, want 1 (moveq retired)", ctx.RetCount)
			}
			if cpu.DataRegs[1] != d1Before {
				t.Fatalf("D1=%08X changed by bailed DIV", cpu.DataRegs[1])
			}
		})
	}
}

// TestM68KARM64_DifferentialFPUGrid drives the native 68881 register-to-register
// subset (FMOVE/FADD/FSUB/FMUL/FDIV/FABS/FNEG/FSQRT/FCMP/FTST and the single
// FSGLMUL/FSGLDIV) over a grid of double operands and incoming FPSR, comparing
// FP registers (bit-exact), FPSR, FPIAR and the integer state against the
// interpreter. FP0..FP7 must all match; FPIAR must be the instruction PC.
func TestM68KARM64_DifferentialFPUGrid(t *testing.T) {
	cmd := func(src, dst, opmode uint16) uint16 { return src<<10 | dst<<7 | opmode }
	const srcReg, dstReg = 1, 2
	type opCase struct {
		name   string
		opmode uint16
	}
	ops := []opCase{
		{"FMOVE", 0x00}, {"FSQRT", 0x04}, {"FABS", 0x18}, {"FNEG", 0x1A},
		{"FDIV", 0x20}, {"FADD", 0x22}, {"FMUL", 0x23}, {"FSGLDIV", 0x24},
		{"FSGLMUL", 0x27}, {"FSUB", 0x28}, {"FCMP", 0x38}, {"FTST", 0x3A},
		// Precision-qualified forms (single = base|0x40, double = base|0x44)
		// exercise the result-precision round-trip.
		{"FSMOVE", 0x40}, {"FSADD", 0x62}, {"FSMUL", 0x63}, {"FSDIV", 0x60},
		{"FSNEG", 0x5A}, {"FDADD", 0x66}, {"FDMOVE", 0x44},
	}
	vals := []float64{
		0.0, math.Copysign(0, -1), 1.0, -1.0, 2.5, -3.75, 1e300, -1e-300,
		math.Pi, 123456.789, math.Inf(1), math.Inf(-1), math.NaN(),
		math.MaxFloat64, math.SmallestNonzeroFloat64,
	}
	fpsrs := []uint32{0x00000000, 0x0F000000, 0x00FF00FF}
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	cmpFP := func(t *testing.T, name string, ref, got *M68KCPU) {
		t.Helper()
		for i := 0; i < 8; i++ {
			rb, gb := math.Float64bits(ref.FPU.fp[i]), math.Float64bits(got.FPU.fp[i])
			if rb != gb {
				t.Errorf("%s: FP%d interp=%016X native=%016X", name, i, rb, gb)
			}
		}
		if ref.FPU.FPSR != got.FPU.FPSR {
			t.Errorf("%s: FPSR interp=%08X native=%08X", name, ref.FPU.FPSR, got.FPU.FPSR)
		}
		if ref.FPU.FPIAR != got.FPU.FPIAR {
			t.Errorf("%s: FPIAR interp=%08X native=%08X", name, ref.FPU.FPIAR, got.FPU.FPIAR)
		}
	}

	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	for _, oc := range ops {
		oc := oc
		ok := t.Run(oc.name, func(t *testing.T) {
			words := []uint16{0xF200, cmd(srcReg, dstReg, oc.opmode)}
			for _, cpu := range []*M68KCPU{ref, got} {
				end := m68kARM64WriteWords(cpu, m68kARM64TestPC, words...)
				m68kARM64WriteWords(cpu, end, 0x4E75)
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix < 1 {
				t.Fatalf("%s: not admitted (prefix %d)", oc.name, prefix)
			}
			block, err := m68kCompileBlockARM64(instrs[:1], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", oc.name, err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, a := range vals {
				for _, b := range vals {
					for _, fpsr := range fpsrs {
						for _, cpu := range []*M68KCPU{ref, got} {
							for i := 0; i < 8; i++ {
								cpu.FPU.fp[i] = 0
							}
							cpu.FPU.fp[dstReg] = a
							cpu.FPU.fp[srcReg] = b
							cpu.FPU.FPSR = fpsr
							cpu.FPU.FPIAR = 0
							cpu.SR = (cpu.SR &^ 0x1F) | 0x10
							cpu.PC = m68kARM64TestPC
						}
						m68kARM64RunInterp(ref, 1)
						ctx.RetPC = 0
						ctx.RetCount = 0
						ctx.NeedIOFallback = 0
						callNative(block.execAddr, m68kJITContextPtr(ctx))
						if ctx.NeedIOFallback != 0 {
							t.Fatalf("%s: unexpected bail a=%v b=%v", oc.name, a, b)
						}
						got.PC = ctx.RetPC
						m68kARM64CompareState(t, oc.name, ref, got)
						cmpFP(t, oc.name, ref, got)
						if t.Failed() {
							t.Fatalf("%s: divergence a=%016X b=%016X fpsr=%08X",
								oc.name, math.Float64bits(a), math.Float64bits(b), fpsr)
						}
					}
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed, stopping grid", oc.name)
		}
	}
}

// TestM68KARM64_PinPlan pins the register-residency plan: used data registers
// pin first, then address registers (never A7), each to a distinct callee-saved
// host, with the dirty flag set exactly for written registers.
func TestM68KARM64_PinPlan(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	// add.l d0,d1 (reads d0,d1; writes d1) ; move.l d2,(a0) (reads d2,a0)
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0xD280, 0x2082)
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	e := &m68kA64Emitter{cb: NewCodeBuffer(64)}
	e.buildPinPlan(instrs)

	if e.pinD[0] == 0 || e.pinD[1] == 0 || e.pinD[2] == 0 || e.pinA[0] == 0 {
		t.Fatalf("expected D0,D1,D2,A0 pinned: pinD=%v pinA=%v", e.pinD, e.pinA)
	}
	if e.pinA[7] != 0 {
		t.Fatal("A7 must never be pinned")
	}
	// Distinct hosts, all callee-saved (>= X19).
	seen := map[byte]bool{}
	for _, p := range e.pins {
		if p.host < m68kA64PinFirstHost {
			t.Errorf("host %d is not callee-saved", p.host)
		}
		if seen[p.host] {
			t.Errorf("host %d assigned twice", p.host)
		}
		seen[p.host] = true
	}
	// D1 is written (dirty); D0/D2/A0 are read-only here.
	for _, p := range e.pins {
		wantDirty := !p.isAddr && p.guest == 1
		if p.dirty != wantDirty {
			t.Errorf("pin D%d/A dirty=%v, want %v", p.guest, p.dirty, wantDirty)
		}
	}
}

// TestM68KARM64_ChainingEntryPoint pins that native block chaining emits a
// chain entry point (past the prologue) exactly when chaining is enabled, and
// that a chained-loop program still produces interpreter-identical state and
// retired-instruction counts (the DBRA/CallReturn dispatcher tests run under
// M68K_ARM64_CHAIN=1 exercise the run-time edge; this fixes the compile-time
// contract).
func TestM68KARM64_ChainingEntryPoint(t *testing.T) {
	if m68kARM64PinEnabled() {
		t.Skip("pinning disables chaining for register-using blocks")
	}
	cpu := m68kARM64NewCPU(t)
	// moveq #1,d0 ; moveq #2,d1 ; bra.s back-to-start (a self-chaining loop body)
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7001, 0x7202, 0x60FC)
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	prefix := m68kARM64SupportedPrefix(instrs, cpu.memory, m68kARM64TestPC, cpu.ProfileTopOfRAM())
	block, err := m68kCompileBlockARM64(instrs[:prefix], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if m68kARM64ChainEnabled() {
		if block.chainEntry == 0 {
			t.Fatal("chaining enabled but no chain entry emitted")
		}
		if block.chainEntry <= block.execAddr {
			t.Fatalf("chain entry %x must be past the prologue (execAddr %x)", block.chainEntry, block.execAddr)
		}
	} else if block.chainEntry != 0 {
		t.Fatalf("chaining disabled but chain entry %x emitted", block.chainEntry)
	}
}

// TestM68KARM64_CCRLivenessAnalysis pins the within-block CCR liveness the
// arm64 backend consumes: in a run of register-only ALU ops, a producer whose
// condition codes are fully overwritten before any consumer is dead, while the
// last producer before the block exit stays live (the boundary observes CCR).
func TestM68KARM64_CCRLivenessAnalysis(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	// add.l d0,d1 ; add.l d0,d1 ; rts
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0xD280, 0xD280, 0x4E75)
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	live := m68kCCRLiveness(instrs)
	if live[0] {
		t.Errorf("first ADD.L CCR should be dead (overwritten by the second)")
	}
	if !live[1] {
		t.Errorf("second ADD.L CCR should be live (block exit observes it)")
	}
}

// TestM68KARM64_CCRLivenessSkipsWork proves the emitter actually elides the
// condition-code materialisation when the current instruction's CCR is dead:
// the same instruction emits strictly fewer bytes with ccrDead set.
func TestM68KARM64_CCRLivenessSkipsWork(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0xD280) // add.l d0,d1
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	ji := &instrs[0]
	dec, ok := m68kA64Decode(ji, cpu.memory, m68kARM64TestPC)
	if !ok {
		t.Fatalf("decode failed")
	}
	emitLen := func(dead bool) int {
		e := &m68kA64Emitter{cb: NewCodeBuffer(256), ccrDead: dead}
		if err := e.emitInstr(&dec, ji, m68kARM64TestPC); err != nil {
			t.Fatalf("emit: %v", err)
		}
		return e.cb.Len()
	}
	full := emitLen(false)
	dead := emitLen(true)
	if dead >= full {
		t.Fatalf("dead-CCR emit not shorter: dead=%d full=%d", dead, full)
	}
}

// TestM68KARM64_SMCExactRange pins the native-exit exact-range SMC contract: a
// native store onto a page marked as containing compiled code sets NeedInval
// with the precise written range; a store onto an unmarked page does not.
// TestM68KARM64_SMCCrossPage pins that a store straddling a 4 KiB boundary into
// a compiled page is detected even when the starting page is unmarked: the SMC
// check must probe the last touched page too, or native chaining could run a
// stale block that this store modified.
func TestM68KARM64_SMCCrossPage(t *testing.T) {
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	cpu := m68kARM64NewCPU(t)
	// moveq #1,d0 ; move.l d0,(a0) with a0 = 0x0FFE (bytes 0x0FFE..0x1001 span
	// pages 0 and 1).
	const storeAddr = 0x0FFE
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7001, 0x2080)
	cpu.AddrRegs[0] = storeAddr

	pageCount := (uint32(len(cpu.memory)) + 4095) >> 12
	bitmap := make([]byte, pageCount)
	bitmap[1] = 1 // only the SECOND page (0x1000) is marked; the start page is not
	ctx := newM68KJITContext(cpu, bitmap, nil, nil)

	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	ctx.NeedInval, ctx.InvalAddr, ctx.InvalSize = 0, 0, 0
	callNative(block.execAddr, m68kJITContextPtr(ctx))

	if ctx.NeedInval != 1 {
		t.Fatalf("NeedInval=%d, want 1 (store straddles into a marked page)", ctx.NeedInval)
	}
	if ctx.InvalAddr != storeAddr || ctx.InvalSize != 4 {
		t.Fatalf("range = [%08X,+%d), want [%08X,+4)", ctx.InvalAddr, ctx.InvalSize, uint32(storeAddr))
	}
}

func TestM68KARM64_SMCExactRange(t *testing.T) {
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	// moveq #1,d0 ; move.l d0,(a0)
	words := []uint16{0x7001, 0x2080}
	const storeAddr = m68kARM64BufBase + 0x10

	for _, tc := range []struct {
		name     string
		markPage bool
		wantInv  uint32
	}{
		{"marked page flags inval", true, 1},
		{"unmarked page no inval", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cpu := m68kARM64NewCPU(t)
			m68kARM64WriteWords(cpu, m68kARM64TestPC, words...)
			cpu.AddrRegs[0] = storeAddr

			pageCount := (uint32(len(cpu.memory)) + 4095) >> 12
			bitmap := make([]byte, pageCount)
			if tc.markPage {
				bitmap[storeAddr>>12] = 1
			}
			ctx := newM68KJITContext(cpu, bitmap, nil, nil)

			instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
			block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			ctx.NeedInval = 0
			ctx.InvalAddr = 0
			ctx.InvalSize = 0
			callNative(block.execAddr, m68kJITContextPtr(ctx))

			if ctx.NeedInval != tc.wantInv {
				t.Fatalf("NeedInval=%d, want %d", ctx.NeedInval, tc.wantInv)
			}
			if tc.wantInv != 0 {
				if ctx.InvalAddr != storeAddr {
					t.Fatalf("InvalAddr=%08X, want %08X", ctx.InvalAddr, uint32(storeAddr))
				}
				if ctx.InvalSize != 4 {
					t.Fatalf("InvalSize=%d, want 4", ctx.InvalSize)
				}
			}
			// The store must have taken effect regardless (D0=1, big-endian).
			got := uint32(cpu.memory[storeAddr])<<24 | uint32(cpu.memory[storeAddr+1])<<16 |
				uint32(cpu.memory[storeAddr+2])<<8 | uint32(cpu.memory[storeAddr+3])
			if got != 1 {
				t.Fatalf("store result=%08X, want 00000001", got)
			}
		})
	}
}

// TestM68KARM64_SMCRejectsMutableGap pins the distinction between a page that
// merely contains compiled code and the exact bytes compiled in
// it. A data write in the gap must not force a native exit, while a write to a
// marked segment remains covered by TestM68KARM64_SMCExactRange.
func TestM68KARM64_SMCRejectsMutableGap(t *testing.T) {
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	cpu := m68kARM64NewCPU(t)
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7001, 0x2080) // moveq #1,d0 ; move.l d0,(a0)
	const storeAddr = m68kARM64BufBase + 0x180
	cpu.AddrRegs[0] = storeAddr

	pageCount := (uint32(len(cpu.memory)) + 4095) >> 12
	bitmap := make([]byte, pageCount)
	bitmap[storeAddr>>12] = 1 // old page-granular guard would falsely hit.
	cpu.m68kJitCodePageMap = make([]*[128]uint32, pageCount)
	compiledAddr := m68kARM64BufBase + 0x20
	compiledPage := compiledAddr >> 12
	cpu.m68kJitCodePageMap[compiledPage] = new([128]uint32)
	compiledOff := compiledAddr & 0xFFF
	cpu.m68kJitCodePageMap[compiledPage][compiledOff>>5] |= 1 << (compiledOff & 31)
	ctx := newM68KJITContext(cpu, bitmap, nil, nil)

	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	callNative(block.execAddr, m68kJITContextPtr(ctx))
	if ctx.NeedInval != 0 {
		t.Fatalf("NeedInval=%d, want 0 for mutable data gap at %08X", ctx.NeedInval, uint32(storeAddr))
	}
}

func TestM68KARM64_InvalidationClearsRemovedBlockAcrossEveryPage(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	pageCount := (uint32(len(cpu.memory)) + 4095) >> 12
	cpu.m68kJitCache = NewCodeCache()
	cpu.m68kJitCodeBitmap = make([]byte, pageCount)
	cpu.m68kJitCodePageMap = make([]*[128]uint32, pageCount)

	const blockLo = uint64(0x1FF0)
	const blockHi = uint64(0x2010)
	block := &JITBlock{startPC: blockLo, endPC: blockHi}
	survivor := &JITBlock{startPC: 0x3000, endPC: 0x3010}
	cpu.m68kJitCache.Put(block)
	cpu.m68kJitCache.Put(survivor)
	cpu.m68kARM64MarkCodePages(block)
	cpu.m68kARM64MarkCodePages(survivor)
	if cpu.m68kJitCodePageMap[1] == nil || cpu.m68kJitCodePageMap[2] == nil {
		t.Fatal("test block did not populate both occupancy pages")
	}

	// A write on the first page removes the whole block. Exact occupancy on
	// the block's second page must disappear as well.
	cpu.invalidateM68KJITForGuestWrite(uint32(blockLo), 1)
	if cpu.m68kJitCache.Len() != 1 {
		t.Fatalf("cache length = %d, want unrelated survivor only", cpu.m68kJitCache.Len())
	}
	for page := 1; page <= 2; page++ {
		if cpu.m68kJitCodeBitmap[page] != 0 {
			t.Fatalf("page bitmap[%d] remained marked after whole-block eviction", page)
		}
		if cpu.m68kJitCodePageMap[page] != nil {
			t.Fatalf("exact occupancy page %d remained allocated after whole-block eviction", page)
		}
	}
	if cpu.m68kJitCodeBitmap[3] == 0 || cpu.m68kJitCodePageMap[3] == nil {
		t.Fatal("unrelated surviving block lost occupancy during metadata rebuild")
	}
}

// TestM68KARM64_DivLZeroBail pins the zero-divide contract for the 68020 long
// divide: a DIVL with a zero divisor must bail before touching Dq/Dr so the
// interpreter raises the trap.
func TestM68KARM64_DivLZeroBail(t *testing.T) {
	for _, tc := range []struct {
		name  string
		words []uint16
	}{
		{"DIVU.L D2,D3 zero", []uint16{0x7400, 0x4C42, 0x3004}}, // moveq #0,d2 ; divu.l d2,d3
		{"DIVS.L D2,D5:D4 zero", []uint16{0x7400, 0x4C42, 0x4C05}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cpu := m68kARM64NewCPU(t)
			m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
			m68kARM64WriteWords(cpu, m68kARM64TestPC+uint32(len(tc.words))*2, 0x4E75)
			cpu.DataRegs[3] = 0x12345678
			cpu.DataRegs[4] = 0x9ABCDEF0
			cpu.DataRegs[5] = 0x0F1E2D3C
			before := [3]uint32{cpu.DataRegs[3], cpu.DataRegs[4], cpu.DataRegs[5]}

			instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
			execMem, err := AllocExecMem(1 << 20)
			if err != nil {
				t.Fatalf("AllocExecMem: %v", err)
			}
			defer execMem.Free()
			block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			ctx := newM68KJITContext(cpu, nil, nil, nil)
			callNative(block.execAddr, m68kJITContextPtr(ctx))
			if ctx.NeedIOFallback != 1 {
				t.Fatalf("NeedIOFallback=%d, want 1 (zero divide must bail)", ctx.NeedIOFallback)
			}
			if ctx.RetPC != m68kARM64TestPC+2 {
				t.Fatalf("bail RetPC=%08X, want %08X (the DIVL)", ctx.RetPC, m68kARM64TestPC+2)
			}
			if ctx.RetCount != 1 {
				t.Fatalf("bail RetCount=%d, want 1 (moveq retired)", ctx.RetCount)
			}
			if cpu.DataRegs[3] != before[0] || cpu.DataRegs[4] != before[1] || cpu.DataRegs[5] != before[2] {
				t.Fatalf("registers changed by bailed DIVL: %08X %08X %08X", cpu.DataRegs[3], cpu.DataRegs[4], cpu.DataRegs[5])
			}
		})
	}
}

// TestM68KARM64_DifferentialBitOpGrid drives BTST/BCHG/BCLR/BSET with dynamic
// (Dn) and immediate bit sources over register (long) and memory (byte)
// destinations, comparing native and interpreter results including the Z-only
// flag behaviour.
func TestM68KARM64_DifferentialBitOpGrid(t *testing.T) {
	type opCase struct {
		name    string
		words   []uint16
		dynamic bool // bit number sourced from D0
		memDst  bool
	}
	cases := []opCase{
		{"BTST D0,D1", []uint16{0x0101}, true, false},
		{"BCHG D0,D1", []uint16{0x0141}, true, false},
		{"BCLR D0,D1", []uint16{0x0181}, true, false},
		{"BSET D0,D1", []uint16{0x01C1}, true, false},
		{"BTST D0,(A0)", []uint16{0x0110}, true, true},
		{"BCHG D0,(A0)", []uint16{0x0150}, true, true},
		{"BCLR D0,(A0)", []uint16{0x0190}, true, true},
		{"BSET D0,(A0)", []uint16{0x01D0}, true, true},
		{"BTST #3,D1", []uint16{0x0801, 0x0003}, false, false},
		{"BTST #17,D1", []uint16{0x0801, 0x0011}, false, false},
		{"BSET #31,D1", []uint16{0x08C1, 0x001F}, false, false},
		{"BCLR #0,D1", []uint16{0x0881, 0x0000}, false, false},
		{"BCHG #5,(A0)", []uint16{0x0850, 0x0005}, false, true},
		{"BSET #7,(A0)", []uint16{0x08D0, 0x0007}, false, true},
		{"BTST #4,(A0)", []uint16{0x0810, 0x0004}, false, true},
	}
	bits := []uint32{0, 1, 7, 8, 15, 16, 31, 32, 0xFF, 0x1234}
	vals := []uint32{0x00000000, 0x00000001, 0x00008000, 0xFFFFFFFF, 0x12345678, 0x80000001, 0x0000FF00, 0xA5A5A5A5}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	for _, tc := range cases {
		tc := tc
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				end := m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
				m68kARM64WriteWords(cpu, end, 0x4E75)
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix < 1 {
				t.Fatalf("%s: not admitted (prefix %d)", tc.name, prefix)
			}
			block, err := m68kCompileBlockARM64(instrs[:1], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", tc.name, err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, bit := range bits {
				for _, v := range vals {
					for _, ccrIn := range []uint16{0x00, 0x1F, 0x0A} {
						for _, cpu := range []*M68KCPU{ref, got} {
							cpu.DataRegs[0] = bit
							cpu.DataRegs[1] = v
							if tc.memDst {
								cpu.AddrRegs[0] = m68kARM64BufBase + 0x10
								cpu.memory[m68kARM64BufBase+0x10] = byte(v)
							}
							cpu.SR = (cpu.SR &^ 0x1F) | ccrIn
							cpu.PC = m68kARM64TestPC
						}
						m68kARM64RunInterp(ref, 1)
						ctx.RetPC = 0
						ctx.RetCount = 0
						ctx.NeedIOFallback = 0
						callNative(block.execAddr, m68kJITContextPtr(ctx))
						if ctx.NeedIOFallback != 0 {
							t.Fatalf("%s: unexpected bail (bit=%d v=%08X)", tc.name, bit, v)
						}
						got.PC = ctx.RetPC
						m68kARM64CompareState(t, tc.name, ref, got)
						if tc.memDst && ref.memory[m68kARM64BufBase+0x10] != got.memory[m68kARM64BufBase+0x10] {
							t.Errorf("%s: mem interp=%02X native=%02X", tc.name,
								ref.memory[m68kARM64BufBase+0x10], got.memory[m68kARM64BufBase+0x10])
						}
						if t.Failed() {
							t.Fatalf("%s: divergence bit=%d v=%08X ccrIn=%02X", tc.name, bit, v, ccrIn)
						}
					}
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed, stopping grid", tc.name)
		}
	}
}

// TestM68KARM64_DifferentialSccGrid drives all 16 Scc conditions on register
// and memory byte destinations across every CCR input, comparing native and
// interpreter results (Scc leaves the CCR unchanged).
func TestM68KARM64_DifferentialSccGrid(t *testing.T) {
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	type dstShape struct {
		name string
		base uint16 // opcode with cond=0
		mem  bool
	}
	for _, shape := range []dstShape{
		{"Dn", 0x50C1, false},  // Scc D1
		{"(A0)", 0x50D0, true}, // Scc (A0)
	} {
		for cond := 0; cond < 16; cond++ {
			word := shape.base | uint16(cond)<<8
			name := fmt.Sprintf("Scond%d_%s", cond, shape.name)
			ok := t.Run(name, func(t *testing.T) {
				for _, cpu := range []*M68KCPU{ref, got} {
					m68kARM64WriteWords(cpu, m68kARM64TestPC, word, 0x4E75)
				}
				instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
				if p := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM()); p < 1 {
					t.Fatalf("%s: not admitted", name)
				}
				block, err := m68kCompileBlockARM64(instrs[:1], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
				if err != nil {
					t.Fatalf("%s: compile: %v", name, err)
				}
				ctx := newM68KJITContext(got, nil, nil, nil)
				for ccrIn := uint16(0); ccrIn < 32; ccrIn++ {
					for _, cpu := range []*M68KCPU{ref, got} {
						cpu.DataRegs[1] = 0x1122AA55
						cpu.AddrRegs[0] = m68kARM64BufBase + 0x10
						cpu.memory[m68kARM64BufBase+0x10] = 0x3C
						cpu.SR = (cpu.SR &^ 0x1F) | ccrIn
						cpu.PC = m68kARM64TestPC
					}
					m68kARM64RunInterp(ref, 1)
					ctx.RetPC = 0
					ctx.RetCount = 0
					ctx.NeedIOFallback = 0
					callNative(block.execAddr, m68kJITContextPtr(ctx))
					if ctx.NeedIOFallback != 0 {
						t.Fatalf("%s: unexpected bail", name)
					}
					got.PC = ctx.RetPC
					m68kARM64CompareState(t, name, ref, got)
					if shape.mem && ref.memory[m68kARM64BufBase+0x10] != got.memory[m68kARM64BufBase+0x10] {
						t.Errorf("%s: mem interp=%02X native=%02X", name,
							ref.memory[m68kARM64BufBase+0x10], got.memory[m68kARM64BufBase+0x10])
					}
					if t.Failed() {
						t.Fatalf("%s: divergence at ccrIn=%02X", name, ccrIn)
					}
				}
			})
			if !ok {
				t.Fatalf("%s failed", name)
			}
		}
	}
}

// TestM68KARM64_DifferentialTAS drives TAS on register and memory byte
// destinations, comparing native and interpreter results.
func TestM68KARM64_DifferentialTAS(t *testing.T) {
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	bytes := []uint8{0x00, 0x01, 0x7F, 0x80, 0xFF, 0x55, 0x80, 0x3C}
	for _, tc := range []struct {
		name string
		word uint16
		mem  bool
	}{
		{"TAS D1", 0x4AC1, false},
		{"TAS (A0)", 0x4AD0, true},
	} {
		tc := tc
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.word, 0x4E75)
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			block, err := m68kCompileBlockARM64(instrs[:1], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			ctx := newM68KJITContext(got, nil, nil, nil)
			for _, b := range bytes {
				for _, ccrIn := range []uint16{0x00, 0x1F, 0x10} {
					for _, cpu := range []*M68KCPU{ref, got} {
						cpu.DataRegs[1] = 0x11223300 | uint32(b)
						cpu.AddrRegs[0] = m68kARM64BufBase + 0x10
						cpu.memory[m68kARM64BufBase+0x10] = b
						cpu.SR = (cpu.SR &^ 0x1F) | ccrIn
						cpu.PC = m68kARM64TestPC
					}
					m68kARM64RunInterp(ref, 1)
					ctx.RetPC = 0
					ctx.RetCount = 0
					ctx.NeedIOFallback = 0
					callNative(block.execAddr, m68kJITContextPtr(ctx))
					if ctx.NeedIOFallback != 0 {
						t.Fatalf("%s: unexpected bail", tc.name)
					}
					got.PC = ctx.RetPC
					m68kARM64CompareState(t, tc.name, ref, got)
					if tc.mem && ref.memory[m68kARM64BufBase+0x10] != got.memory[m68kARM64BufBase+0x10] {
						t.Errorf("%s: mem interp=%02X native=%02X", tc.name,
							ref.memory[m68kARM64BufBase+0x10], got.memory[m68kARM64BufBase+0x10])
					}
					if t.Failed() {
						t.Fatalf("%s: divergence b=%02X ccrIn=%02X", tc.name, b, ccrIn)
					}
				}
			}
		})
		if !ok {
			t.Fatalf("%s failed", tc.name)
		}
	}
}

// TestM68KARM64_DifferentialMOVEM drives MOVEM in both directions across
// word/long sizes, predecrement and postincrement, and the base-register-in-
// list edge cases, comparing registers, the memory window, CCR and PC against
// the interpreter.
func TestM68KARM64_DifferentialMOVEM(t *testing.T) {
	type opCase struct {
		name  string
		words []uint16
		setup func(cpu *M68KCPU)
	}
	seedRegs := func(cpu *M68KCPU) {
		for i := 0; i < 8; i++ {
			cpu.DataRegs[i] = 0xD0000000 | uint32(i*0x11111111)
		}
		for i := 0; i < 7; i++ {
			cpu.AddrRegs[i] = 0xA0000000 | uint32(i)<<8
		}
	}
	cases := []opCase{
		{
			name:  "MOVEM.L D0-D3/A0-A1,-(A7)",
			words: []uint16{0x48E7, 0xF0C0},
			setup: func(cpu *M68KCPU) { seedRegs(cpu); cpu.AddrRegs[7] = m68kARM64BufBase + 0x80 },
		},
		{
			name:  "MOVEM.L (A7)+,D0-D3/A0-A1",
			words: []uint16{0x4CDF, 0x030F},
			setup: func(cpu *M68KCPU) { seedRegs(cpu); cpu.AddrRegs[7] = m68kARM64BufBase + 0x10 },
		},
		{
			name:  "MOVEM.W D0-D2,(A0)",
			words: []uint16{0x4890, 0x0007},
			setup: func(cpu *M68KCPU) { seedRegs(cpu); cpu.AddrRegs[0] = m68kARM64BufBase + 0x20 },
		},
		{
			name:  "MOVEM.W (A0),D4-D6",
			words: []uint16{0x4C90, 0x0070},
			setup: func(cpu *M68KCPU) { seedRegs(cpu); cpu.AddrRegs[0] = m68kARM64BufBase + 0x20 },
		},
		{
			name:  "MOVEM.L D0-D7/A0-A7,(A0)",
			words: []uint16{0x48D0, 0xFFFF},
			setup: func(cpu *M68KCPU) { seedRegs(cpu); cpu.AddrRegs[0] = m68kARM64BufBase + 0x10 },
		},
		{
			name:  "MOVEM.L (A0)+,D0-D7/A0-A7",
			words: []uint16{0x4CD8, 0xFFFF},
			setup: func(cpu *M68KCPU) { seedRegs(cpu); cpu.AddrRegs[0] = m68kARM64BufBase + 0x10 },
		},
		{
			name:  "MOVEM.L 8(A0),D0-D2",
			words: []uint16{0x4CE8, 0x0008, 0x0007},
			setup: func(cpu *M68KCPU) { seedRegs(cpu); cpu.AddrRegs[0] = m68kARM64BufBase + 0x20 },
		},
		{
			name:  "MOVEM.L A7,-(A7) (base in list)",
			words: []uint16{0x48E7, 0x0001},
			setup: func(cpu *M68KCPU) { seedRegs(cpu); cpu.AddrRegs[7] = m68kARM64BufBase + 0x40 },
		},
		{
			name:  "MOVEM.W A0-A3,-(A7) (word predec)",
			words: []uint16{0x48A7, 0x00F0},
			setup: func(cpu *M68KCPU) { seedRegs(cpu); cpu.AddrRegs[7] = m68kARM64BufBase + 0x40 },
		},
	}
	ref := m68kARM64NewCPU(t)
	got := m68kARM64NewCPU(t)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	for _, tc := range cases {
		tc := tc
		ok := t.Run(tc.name, func(t *testing.T) {
			for _, cpu := range []*M68KCPU{ref, got} {
				end := m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
				m68kARM64WriteWords(cpu, end, 0x4E75)
				for i := uint32(0); i < 0x100; i++ {
					cpu.memory[m68kARM64BufBase+i] = byte(i*3 + 1)
				}
				tc.setup(cpu)
				cpu.PC = m68kARM64TestPC
			}
			instrs := m68kScanBlock(got.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC, got.ProfileTopOfRAM())
			if prefix < 1 {
				t.Fatalf("%s: not admitted (prefix %d)", tc.name, prefix)
			}
			block, err := m68kCompileBlockARM64(instrs[:1], m68kARM64TestPC, execMem, got.memory, got.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("%s: compile: %v", tc.name, err)
			}
			m68kARM64RunInterp(ref, 1)
			ctx := newM68KJITContext(got, nil, nil, nil)
			callNative(block.execAddr, m68kJITContextPtr(ctx))
			if ctx.NeedIOFallback != 0 {
				t.Fatalf("%s: unexpected bail", tc.name)
			}
			got.PC = ctx.RetPC
			if ctx.RetCount != 1 {
				t.Fatalf("%s: retired=%d want 1", tc.name, ctx.RetCount)
			}
			m68kARM64CompareState(t, tc.name, ref, got)
			for i := uint32(0); i < 0x100; i++ {
				if ref.memory[m68kARM64BufBase+i] != got.memory[m68kARM64BufBase+i] {
					t.Fatalf("%s: mem[%08X] interp=%02X native=%02X", tc.name, m68kARM64BufBase+i,
						ref.memory[m68kARM64BufBase+i], got.memory[m68kARM64BufBase+i])
				}
			}
		})
		if !ok {
			t.Fatalf("%s: subtest failed", tc.name)
		}
	}
}

// TestM68KARM64_MOVEMIOBail pins the MOVEM whole-span guard: a MOVEM whose
// access range hits an I/O page must bail before any transfer, leaving every
// register and the memory window untouched so the interpreter replays it.
func TestM68KARM64_MOVEMIOBail(t *testing.T) {
	cpu := m68kARM64NewCPU(t)
	// moveq #5,d2 ; movem.l d0-d3,(a0)
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7405, 0x48D0, 0x000F, 0x4E75)
	cpu.m68kJitIOPageBitmap = make([]bool, (uint32(len(cpu.memory))+255)>>8)
	ioAddr := uint32(0x9000)
	cpu.m68kJitIOPageBitmap[ioAddr>>8] = true

	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	ctx := newM68KJITContext(cpu, nil, nil, nil)
	for i := 0; i < 4; i++ {
		cpu.DataRegs[i] = 0x11111111 * uint32(i+1)
	}
	cpu.AddrRegs[0] = ioAddr + 4 // span [ioAddr+4, ioAddr+20) lies inside the I/O page
	d0, d1, d3 := cpu.DataRegs[0], cpu.DataRegs[1], cpu.DataRegs[3]
	memBefore := cpu.memory[ioAddr]
	callNative(block.execAddr, m68kJITContextPtr(ctx))

	if ctx.NeedIOFallback != 1 {
		t.Fatalf("NeedIOFallback=%d, want 1 (MOVEM into I/O page must bail)", ctx.NeedIOFallback)
	}
	if ctx.RetPC != m68kARM64TestPC+2 {
		t.Fatalf("bail RetPC=%08X, want %08X (the MOVEM)", ctx.RetPC, m68kARM64TestPC+2)
	}
	if ctx.RetCount != 1 {
		t.Fatalf("bail RetCount=%d, want 1", ctx.RetCount)
	}
	if cpu.DataRegs[2] != 5 {
		t.Fatalf("D2=%08X, want 5 (moveq before the bail must commit)", cpu.DataRegs[2])
	}
	if cpu.DataRegs[0] != d0 || cpu.DataRegs[1] != d1 || cpu.DataRegs[3] != d3 {
		t.Fatalf("MOVEM source registers changed by bailed MOVEM")
	}
	if cpu.AddrRegs[0] != ioAddr+4 {
		t.Fatalf("A0=%08X changed by bailed MOVEM", cpu.AddrRegs[0])
	}
	if cpu.memory[ioAddr] != memBefore {
		t.Fatalf("memory written by bailed MOVEM")
	}
}

// TestM68KARM64_BoundarySRPublication pins the interrupt-boundary invariant:
// a block's epilogue must publish the live CCR into cpu.SR (preserving the
// supervisor and interrupt-mask bits) so a successor block or a boundary
// interrupt observes the predecessor's flags exactly. This is the correctness
// guarantee that stands in for native chaining in milestone 3.
func TestM68KARM64_BoundarySRPublication(t *testing.T) {
	cases := []struct {
		name    string
		words   []uint16
		srIn    uint16
		wantCCR uint16
	}{
		// moveq #-1,d0 sets N, clears Z/V/C, preserves X.
		{"moveq neg sets N", []uint16{0x70FF, 0x4E71}, 0x2700, 0x08},
		// moveq #0,d0 sets Z, preserves X (X=1 in from 0x2710).
		{"moveq zero sets Z keeps X", []uint16{0x7000, 0x4E71}, 0x2710, 0x14},
		// add.l with a carry-producing pair set below; here just check publish.
		{"moveq pos clears NZ", []uint16{0x7001, 0x4E71}, 0x271F, 0x10},
	}
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer execMem.Free()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cpu := m68kARM64NewCPU(t)
			m68kARM64WriteWords(cpu, m68kARM64TestPC, tc.words...)
			instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
			block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory, cpu.ProfileTopOfRAM())
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			cpu.SR = tc.srIn
			ctx := newM68KJITContext(cpu, nil, nil, nil)
			callNative(block.execAddr, m68kJITContextPtr(ctx))
			if cpu.SR&0x1F != tc.wantCCR {
				t.Fatalf("published CCR=%02X, want %02X", cpu.SR&0x1F, tc.wantCCR)
			}
			if cpu.SR&0xFF00 != tc.srIn&0xFF00 {
				t.Fatalf("SR high bits=%04X changed, want %04X preserved", cpu.SR&0xFF00, tc.srIn&0xFF00)
			}
		})
	}
}

// TestM68KARM64_ExceptionDelivery drives programs that raise processor
// exceptions (TRAP, CHK, illegal instruction) through both the pure
// interpreter and the JIT dispatcher, and requires identical final state.
// Exception-generating instructions are never admitted into a native block,
// so they fall back to the interpreter with the exact resume PC; this test
// pins that per-instruction resume-PC contract end to end.
func TestM68KARM64_ExceptionDelivery(t *testing.T) {
	type prog struct {
		name    string
		main    []uint16 // written at 0x1000
		vector  uint32   // exception vector address to point at the handler
		handler []uint16 // written at 0x2000, must STOP
		setup   func(cpu *M68KCPU)
	}
	handlerAddr := uint32(0x2000)
	stopHandler := []uint16{0x747F, 0x4E72, 0x2700} // moveq #$7f,d2 ; stop #$2700
	progs := []prog{
		{
			name:    "TRAP #0",
			main:    []uint16{0x7005, 0x4E40, 0x7209, 0x4E72, 0x2700}, // moveq #5,d0 ; trap #0 ; moveq #9,d1 ; stop
			vector:  0x80,                                             // TRAP #0 => vector 32
			handler: stopHandler,
		},
		{
			name:    "CHK out of range",
			main:    []uint16{0x203C, 0x00, 0x0064, 0x4186, 0x4E72, 0x2700}, // move.l #100,d0 ; chk.w d6,d0 ; stop
			vector:  0x18,                                                   // CHK => vector 6
			handler: stopHandler,
			setup:   func(cpu *M68KCPU) { cpu.DataRegs[6] = 0x0000000A }, // bound 10 < 100 => trap
		},
		{
			name:    "illegal instruction",
			main:    []uint16{0x4AFC, 0x7209, 0x4E72, 0x2700}, // illegal ; moveq #9,d1 ; stop
			vector:  0x10,                                     // illegal => vector 4
			handler: stopHandler,
		},
	}
	for _, p := range progs {
		p := p
		t.Run(p.name, func(t *testing.T) {
			ref := m68kARM64NewCPU(t)
			got := m68kARM64NewCPU(t)
			for _, cpu := range []*M68KCPU{ref, got} {
				m68kARM64WriteWords(cpu, m68kARM64TestPC, p.main...)
				m68kARM64WriteWords(cpu, handlerAddr, p.handler...)
				// Vector points at the handler.
				cpu.memory[p.vector] = byte(handlerAddr >> 24)
				cpu.memory[p.vector+1] = byte(handlerAddr >> 16)
				cpu.memory[p.vector+2] = byte(handlerAddr >> 8)
				cpu.memory[p.vector+3] = byte(handlerAddr)
				cpu.SR = 0x2700 // supervisor, IPL 7
				cpu.AddrRegs[7] = 0x7000
				cpu.stackLowerBound = 0 // this test uses a low supervisor stack
				cpu.PC = m68kARM64TestPC
				if p.setup != nil {
					p.setup(cpu)
				}
			}

			refCount := 0
			for !ref.stopped.Load() && refCount < 200 {
				refCount += ref.StepOne()
			}

			got.m68kJitEnabled = true
			got.StoppedIdleHook = func(c *M68KCPU) { c.running.Store(false) }
			got.running.Store(true)
			got.M68KExecuteJIT()

			m68kARM64CompareState(t, p.name, ref, got)
			// Both must have entered the handler and stopped there.
			if got.DataRegs[2] != 0x7F {
				t.Fatalf("%s: handler did not run (D2=%08X)", p.name, got.DataRegs[2])
			}
			// Supervisor stack frames must match too.
			for a := uint32(0x6FE0); a < 0x7000; a++ {
				if ref.memory[a] != got.memory[a] {
					t.Fatalf("%s: exception frame mem[%08X] interp=%02X jit=%02X", p.name, a,
						ref.memory[a], got.memory[a])
				}
			}
		})
	}
}

// TestM68KARM64_FallbackAdmission pins that exception-generating and 68881
// floating-point instructions are never lowered natively: each must terminate
// the supported prefix so the dispatcher interprets it. This is the contract
// behind the staged FPU fallback and the per-instruction exception resume PC.
func TestM68KARM64_FallbackAdmission(t *testing.T) {
	cases := []struct {
		name  string
		words []uint16
	}{
		{"TRAP #0", []uint16{0x4E40}},
		{"TRAPV", []uint16{0x4E76}},
		{"CHK.W D0,D1", []uint16{0x4380}},
		{"illegal", []uint16{0x4AFC}},
		{"Line-A", []uint16{0xA000}},
		// FPU forms outside the native subset must stay on the interpreter.
		{"FINT", []uint16{0xF200, 0x0001}}, // round-to-integer (FPCR mode)
		{"FSIN transcendental", []uint16{0xF200, 0x000E}},
		{"FPU EA source FADD", []uint16{0xF210, 0x4022}}, // R/M=1: (A0) source
		{"FPU control move", []uint16{0xF200, 0x8000}},   // cmdWord bit15: FMOVEM/control
		{"RTE", []uint16{0x4E73}},
		{"STOP", []uint16{0x4E72, 0x2700}},
	}
	cpu := m68kARM64NewCPU(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A supported instruction first, then the probe: the prefix must
			// cover only the leading supported instruction, never the probe.
			words := append([]uint16{0x7001}, tc.words...) // moveq #1,d0 ; <probe>
			m68kARM64WriteWords(cpu, m68kARM64TestPC, words...)
			instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
			prefix := m68kARM64SupportedPrefix(instrs, cpu.memory, m68kARM64TestPC, cpu.ProfileTopOfRAM())
			if prefix != 1 {
				t.Fatalf("%s: prefix=%d, want 1 (probe must not be admitted)", tc.name, prefix)
			}
		})
	}
}
