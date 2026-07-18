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
