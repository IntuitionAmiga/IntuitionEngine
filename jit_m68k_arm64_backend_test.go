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
	"testing"
)

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
	// moveq #1,d0 ; add.l d0,d1 ; jsr (a0) [unsupported] ; rts
	m68kARM64WriteWords(cpu, m68kARM64TestPC, 0x7001, 0xD280, 0x4E90, 0x4E75)
	instrs := m68kScanBlock(cpu.memory, m68kARM64TestPC)
	prefix := m68kARM64SupportedPrefix(instrs, cpu.memory, m68kARM64TestPC)
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
			prefix := m68kARM64SupportedPrefix(instrs, got.memory, m68kARM64TestPC)
			if prefix < tc.count {
				t.Fatalf("%s: supported prefix %d < want %d", tc.name, prefix, tc.count)
			}
			block, err := m68kCompileBlockARM64(instrs[:tc.count], m68kARM64TestPC, execMem, got.memory)
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
	block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory)
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
	block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, execMem, cpu.memory)
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
	block, err := m68kCompileBlockARM64(instrs[:2], m68kARM64TestPC, cpu.m68kGetJITExecMem(), cpu.memory)
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
		if count >= 200 {
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
