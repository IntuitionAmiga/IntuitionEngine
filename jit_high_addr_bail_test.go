// jit_high_addr_bail_test.go - PLAN_MAX_RAM slice 10b TDD coverage.
//
// Pins the IE64 JIT slow-path bail when an address exceeds bus.memory: the
// existing slow path indexes ioPageBitmap[addr>>8] without bounds checking,
// so above-MemSize addresses index past the end of the bitmap and either
// crash or behave nondeterministically. The bail check at the top of the
// slow path translates `addr >= MemSize` into a clean bail-to-interpreter.

//go:build amd64 && linux

package main

import (
	"encoding/binary"
	"testing"
	"time"
)

// TestJIT_AMD64_IE64Load_AboveMemSize_BailsToInterpreter pins the bail.
// Without the fix the slow path would either segfault or read random
// memory; with the fix NeedIOFallback is set cleanly.
func TestJIT_AMD64_IE64Load_AboveMemSize_BailsToInterpreter(t *testing.T) {
	r := newJITTestRig(t)

	// MemSize for default rig is DEFAULT_MEMORY_SIZE = 32 MiB.
	// 0x80000000 is far above MemSize and far above IO_REGION_START, so it
	// takes the slow path. Before the fix the slow path would index
	// ioPageBitmap[0x80000000>>8] = ioPageBitmap[0x800000], which is OOB
	// for a bitmap sized 32 MiB / 256 = 0x20000 entries.
	r.cpu.regs[2] = 0x80000000
	r.ctx.NeedIOFallback = 0

	// LOAD.Q R1, 0(R2)
	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedIOFallback != 1 {
		t.Fatalf("NeedIOFallback = %d, want 1 (clean bail above MemSize)", r.ctx.NeedIOFallback)
	}
}

func TestJIT_AMD64_IE64Store_AboveMemSize_BailsToInterpreter(t *testing.T) {
	r := newJITTestRig(t)

	r.cpu.regs[1] = 0xDEADBEEFCAFEBABE
	r.cpu.regs[2] = 0x80000000
	r.ctx.NeedIOFallback = 0

	// STORE.Q R1, 0(R2)
	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedIOFallback != 1 {
		t.Fatalf("NeedIOFallback = %d, want 1 (clean bail above MemSize)", r.ctx.NeedIOFallback)
	}
}

// TestJIT_AMD64_IE64Load_AtMemSize_BailsToInterpreter pins the boundary:
// addr == MemSize is exactly out-of-range (last valid byte is MemSize-1).
func TestJIT_AMD64_IE64Load_AtMemSize_BailsToInterpreter(t *testing.T) {
	r := newJITTestRig(t)

	r.cpu.regs[2] = uint64(len(r.cpu.memory))
	r.ctx.NeedIOFallback = 0

	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedIOFallback != 1 {
		t.Fatalf("NeedIOFallback = %d, want 1 (boundary bail at addr==MemSize)", r.ctx.NeedIOFallback)
	}
}

// =============================================================================
// Phase 1 — Address aliasing red tests
//
// These tests prove that JIT-emitted native code must not silently truncate a
// 64-bit effective address to 32 bits before the slow-path bail. Without the
// fix, R2 = 0x0000000100001000 becomes 0x1000 in EAX, which passes the
// `< IO_REGION_START` fast-path check and reads/writes bus.memory[0x1000],
// silently aliasing high addresses into low memory.
// =============================================================================

const phase1HighAddr uint64 = 0x0000_0001_0000_8000
const phase1LowAlias uint32 = 0x8000 // low 32 bits of phase1HighAddr (away from PROG_START)

func TestJIT_AMD64_IE64Load_Above4GiB_MustNotAlias(t *testing.T) {
	r := newJITTestRig(t)

	// Plant a poisoned value at the low-alias address. If the JIT
	// truncates, it will read this value instead of bailing.
	const poison uint64 = 0xDEADDEADDEADDEAD
	binary.LittleEndian.PutUint64(r.cpu.memory[phase1LowAlias:], poison)

	// Sentinel in R1 — must remain untouched if bail is correct.
	const sentinel uint64 = 0x1111111111111111
	r.cpu.regs[1] = sentinel
	r.cpu.regs[2] = phase1HighAddr
	r.ctx.NeedIOFallback = 0

	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] == poison {
		t.Fatalf("R1 = 0x%016X, JIT aliased high addr 0x%016X to low 0x%X", r.cpu.regs[1], phase1HighAddr, phase1LowAlias)
	}
	if r.cpu.regs[1] != sentinel {
		t.Fatalf("R1 = 0x%016X, want sentinel 0x%016X (bail must leave dest untouched)", r.cpu.regs[1], sentinel)
	}
	if r.ctx.NeedIOFallback != 1 {
		t.Fatalf("NeedIOFallback = %d, want 1 (high addr must bail)", r.ctx.NeedIOFallback)
	}
}

func TestJIT_AMD64_IE64Store_Above4GiB_MustNotCorrupt(t *testing.T) {
	r := newJITTestRig(t)

	// Zero the low-alias window.
	for i := uint32(0); i < 8; i++ {
		r.cpu.memory[phase1LowAlias+i] = 0
	}

	const payload uint64 = 0xCAFEBABECAFEBABE
	r.cpu.regs[1] = payload
	r.cpu.regs[2] = phase1HighAddr
	r.ctx.NeedIOFallback = 0

	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	stored := binary.LittleEndian.Uint64(r.cpu.memory[phase1LowAlias:])
	if stored != 0 {
		t.Fatalf("bus.memory[0x%X] = 0x%016X, want 0 (JIT must not alias high store into low memory)", phase1LowAlias, stored)
	}
	if r.ctx.NeedIOFallback != 1 {
		t.Fatalf("NeedIOFallback = %d, want 1", r.ctx.NeedIOFallback)
	}
}

func TestJIT_AMD64_IE64FLoad_Above4GiB_MustNotAlias(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised on this rig")
	}

	// Poison the low-alias address.
	const poison uint32 = 0xDEADBEEF
	binary.LittleEndian.PutUint32(r.cpu.memory[phase1LowAlias:], poison)

	const sentinel uint32 = 0x11111111
	r.cpu.FPU.FPRegs[0] = sentinel
	r.cpu.regs[2] = phase1HighAddr
	r.ctx.NeedIOFallback = 0

	r.compileAndRun(t, ie64Instr(OP_FLOAD, 0, IE64_SIZE_L, 0, 2, 0, 0))

	if r.cpu.FPU.FPRegs[0] == poison {
		t.Fatalf("F0 = 0x%08X, JIT aliased high addr to low 0x%X", r.cpu.FPU.FPRegs[0], phase1LowAlias)
	}
	if r.cpu.FPU.FPRegs[0] != sentinel {
		t.Fatalf("F0 = 0x%08X, want sentinel 0x%08X (bail must leave FP dest untouched)", r.cpu.FPU.FPRegs[0], sentinel)
	}
	if r.ctx.NeedIOFallback != 1 {
		t.Fatalf("NeedIOFallback = %d, want 1", r.ctx.NeedIOFallback)
	}
}

func TestJIT_AMD64_IE64FStore_Above4GiB_MustNotCorrupt(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised on this rig")
	}

	// Zero the low-alias window.
	for i := uint32(0); i < 4; i++ {
		r.cpu.memory[phase1LowAlias+i] = 0
	}

	const payload uint32 = 0xCAFEBABE
	r.cpu.FPU.FPRegs[0] = payload
	r.cpu.regs[2] = phase1HighAddr
	r.ctx.NeedIOFallback = 0

	r.compileAndRun(t, ie64Instr(OP_FSTORE, 0, IE64_SIZE_L, 0, 2, 0, 0))

	stored := binary.LittleEndian.Uint32(r.cpu.memory[phase1LowAlias:])
	if stored != 0 {
		t.Fatalf("bus.memory[0x%X] = 0x%08X, want 0 (JIT must not alias high FP store)", phase1LowAlias, stored)
	}
	if r.ctx.NeedIOFallback != 1 {
		t.Fatalf("NeedIOFallback = %d, want 1", r.ctx.NeedIOFallback)
	}
}

// TestJIT_AMD64_IE64Load_Above4GiB_SlowPathRange exercises the widened bail
// check: the low 32 bits are >= IO_REGION_START (so truncation routes to the
// slow path) AND < MemSize (so the legacy 32-bit MemSize check would pass
// and proceed to the ioPageBitmap probe with an OOB index). The 64-bit bail
// must catch this.
func TestJIT_AMD64_IE64Load_Above4GiB_SlowPathRange(t *testing.T) {
	r := newJITTestRig(t)

	// Low 32 bits = 0x000A0001: >= IO_REGION_START (0xA0000), < MemSize (32 MiB).
	r.cpu.regs[1] = 0x1111111111111111
	r.cpu.regs[2] = 0x0000_0001_000A_0001
	r.ctx.NeedIOFallback = 0

	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedIOFallback != 1 {
		t.Fatalf("NeedIOFallback = %d, want 1 (slow-path range high addr must bail)", r.ctx.NeedIOFallback)
	}
}

// TestJIT_AMD64_IE64Load_NearEndOfMemory_Bails exercises the size-aware bail.
// For each access size, addr = MemSize - accessSize + 1 is the first address
// where the access escapes bus.memory (last byte at MemSize, which is OOB).
func TestJIT_AMD64_IE64Load_NearEndOfMemory_Bails(t *testing.T) {
	memSize := uint64(len(newJITTestRig(t).cpu.memory))

	cases := []struct {
		name  string
		size  byte
		bytes uint64
	}{
		{"B", IE64_SIZE_B, 1},
		{"W", IE64_SIZE_W, 2},
		{"L", IE64_SIZE_L, 4},
		{"Q", IE64_SIZE_Q, 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newJITTestRig(t)
			// First OOB start: last byte lands at MemSize (one past valid).
			r.cpu.regs[2] = memSize - c.bytes + 1
			r.ctx.NeedIOFallback = 0

			r.compileAndRun(t, ie64Instr(OP_LOAD, 1, c.size, 0, 2, 0, 0))

			if r.ctx.NeedIOFallback != 1 {
				t.Fatalf("size=%s addr=MemSize-%d+1: NeedIOFallback = %d, want 1",
					c.name, c.bytes, r.ctx.NeedIOFallback)
			}
		})
	}
}

func TestJIT_AMD64_IE64FLoad_NearEndOfMemory_Bails(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised on this rig")
	}
	memSize := uint64(len(r.cpu.memory))

	// FLOAD is L-sized (4 bytes). First OOB start is MemSize-3.
	r.cpu.regs[2] = memSize - 3
	r.ctx.NeedIOFallback = 0

	r.compileAndRun(t, ie64Instr(OP_FLOAD, 0, IE64_SIZE_L, 0, 2, 0, 0))

	if r.ctx.NeedIOFallback != 1 {
		t.Fatalf("FLOAD addr=MemSize-3: NeedIOFallback = %d, want 1 (size-aware bail)", r.ctx.NeedIOFallback)
	}
}

// =============================================================================
// Phase 1.3 — End-to-end SparseBacking tests
//
// These run via ExecuteJIT, exercising the full dispatch loop:
//   JIT compile → native bail (NeedIOFallback=1) → interpretOne →
//   cpu.loadMem/storeMem → bus.ReadPhys64/WritePhys64 → SparseBacking.
// They prove that a high address read/written via the JIT path lands in the
// backing, not aliased into bus.memory.
// =============================================================================

func runIE64HighBackingTest(t *testing.T, setup func(cpu *CPU64), instrs ...[]byte) (*CPU64, *SparseBacking) {
	t.Helper()
	const memSize = 64 * 1024 * 1024 // 64 MiB low window
	bus, err := NewMachineBusSized(memSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	// 8 GiB advertised backing.
	backing := NewSparseBacking(8 * 1024 * 1024 * 1024)
	bus.SetBacking(backing)
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    8 * 1024 * 1024 * 1024,
		ActiveVisibleRAM: 8 * 1024 * 1024 * 1024,
	})

	cpu := NewCPU64(bus)
	cpu.jitEnabled = true

	// Load instructions at PROG_START.
	offset := uint32(PROG_START)
	for _, ins := range instrs {
		copy(cpu.memory[offset:], ins)
		offset += uint32(len(ins))
	}
	copy(cpu.memory[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	if setup != nil {
		setup(cpu)
	}

	done := make(chan struct{})
	go func() {
		cpu.ExecuteJIT()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cpu.running.Store(false)
		waitDoneWithGuard(t, done)
		t.Fatal("ExecuteJIT timed out")
	}
	return cpu, backing
}

func TestJIT_AMD64_IE64Load_HighBacking_EndToEnd(t *testing.T) {
	const want uint64 = 0x1234567890ABCDEF
	const lowAlias uint32 = 0x8000

	cpu, _ := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			// Plant target value in backing at the high address.
			for i := uint64(0); i < 8; i++ {
				cpu.bus.backing.Write8(phase1HighAddr+i, byte(want>>(8*i)))
			}
			// Plant a different value at the low alias to detect aliasing.
			binary.LittleEndian.PutUint64(cpu.memory[lowAlias:], 0xAAAAAAAAAAAAAAAA)
			cpu.regs[2] = phase1HighAddr
		},
		ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0),
	)

	if cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%016X, want 0x%016X (backing value, not alias)", cpu.regs[1], want)
	}
	gotLow := binary.LittleEndian.Uint64(cpu.memory[lowAlias:])
	if gotLow != 0xAAAAAAAAAAAAAAAA {
		t.Fatalf("bus.memory[0x%X] = 0x%016X, want 0xAAAA... (low memory must not be touched)", lowAlias, gotLow)
	}
}

func TestJIT_AMD64_IE64Store_HighBacking_EndToEnd(t *testing.T) {
	const payload uint64 = 0xFEDCBA0987654321
	const lowAlias uint32 = 0x8000

	cpu, backing := runIE64HighBackingTest(t,
		func(cpu *CPU64) {
			// Zero the low alias — any aliased write would corrupt it.
			for i := uint32(0); i < 8; i++ {
				cpu.memory[lowAlias+i] = 0
			}
			cpu.regs[1] = payload
			cpu.regs[2] = phase1HighAddr
		},
		ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0),
	)

	// Read back from backing.
	var got uint64
	for i := uint64(0); i < 8; i++ {
		got |= uint64(backing.Read8(phase1HighAddr+i)) << (8 * i)
	}
	if got != payload {
		t.Fatalf("backing[0x%016X] = 0x%016X, want 0x%016X", phase1HighAddr, got, payload)
	}
	gotLow := binary.LittleEndian.Uint64(cpu.memory[lowAlias:])
	if gotLow != 0 {
		t.Fatalf("bus.memory[0x%X] = 0x%016X, want 0 (high store must not alias low memory)", lowAlias, gotLow)
	}
}

// TestJIT_AMD64_IE64Store_NearEndOfMemory_Bails — mirror of LOAD size-aware test.
func TestJIT_AMD64_IE64Store_NearEndOfMemory_Bails(t *testing.T) {
	memSize := uint64(len(newJITTestRig(t).cpu.memory))

	cases := []struct {
		name  string
		size  byte
		bytes uint64
	}{
		{"B", IE64_SIZE_B, 1},
		{"W", IE64_SIZE_W, 2},
		{"L", IE64_SIZE_L, 4},
		{"Q", IE64_SIZE_Q, 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newJITTestRig(t)
			r.cpu.regs[1] = 0xCAFEBABECAFEBABE
			r.cpu.regs[2] = memSize - c.bytes + 1
			r.ctx.NeedIOFallback = 0

			r.compileAndRun(t, ie64Instr(OP_STORE, 1, c.size, 0, 2, 0, 0))

			if r.ctx.NeedIOFallback != 1 {
				t.Fatalf("size=%s addr=MemSize-%d+1: NeedIOFallback = %d, want 1",
					c.name, c.bytes, r.ctx.NeedIOFallback)
			}
		})
	}
}
