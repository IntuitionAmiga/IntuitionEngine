// jit_high_addr_bail_arm64_test.go - ARM64 high-address aliasing red tests.
//
// Mirrors jit_high_addr_bail_test.go (AMD64) for the ARM64 JIT. Proves that
// the ARM64 emitter must not silently truncate 64-bit effective addresses
// before the slow-path bail. See the AMD64 file for the rationale.

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"testing"
	"time"
)

const phase1HighAddr uint64 = 0x0000_0001_0000_8000
const phase1LowAlias uint32 = 0x8000

func TestJIT_ARM64_IE64Load_Above4GiB_MustNotAlias(t *testing.T) {
	r := newJITTestRig(t)

	const poison uint64 = 0xDEADDEADDEADDEAD
	binary.LittleEndian.PutUint64(r.cpu.memory[phase1LowAlias:], poison)

	const sentinel uint64 = 0x1111111111111111
	r.cpu.regs[1] = sentinel
	r.cpu.regs[2] = phase1HighAddr
	r.ctx.NeedIOFallback = 0

	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] == poison {
		t.Fatalf("R1 = 0x%016X, JIT aliased high addr 0x%016X to low 0x%X", r.cpu.regs[1], phase1HighAddr, phase1LowAlias)
	}
	if r.cpu.regs[1] != sentinel {
		t.Fatalf("R1 = 0x%016X, want sentinel 0x%016X", r.cpu.regs[1], sentinel)
	}
	// Phase 5 cycle 5.4: high-addr LOAD routes through HELPER_LOAD.
	if r.ctx.NeedHelper != HELPER_LOAD {
		t.Fatalf("NeedHelper = %d, want HELPER_LOAD", r.ctx.NeedHelper)
	}
}

func TestJIT_ARM64_IE64Store_Above4GiB_MustNotCorrupt(t *testing.T) {
	r := newJITTestRig(t)

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
		t.Fatalf("bus.memory[0x%X] = 0x%016X, want 0", phase1LowAlias, stored)
	}
	if r.ctx.NeedHelper != HELPER_STORE {
		t.Fatalf("NeedHelper = %d, want HELPER_STORE", r.ctx.NeedHelper)
	}
}

func TestJIT_ARM64_IE64FLoad_Above4GiB_MustNotAlias(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised on this rig")
	}

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
		t.Fatalf("F0 = 0x%08X, want sentinel 0x%08X", r.cpu.FPU.FPRegs[0], sentinel)
	}
	if r.ctx.NeedHelper != HELPER_FLOAD {
		t.Fatalf("NeedHelper = %d, want HELPER_FLOAD", r.ctx.NeedHelper)
	}
}

func TestJIT_ARM64_IE64FStore_Above4GiB_MustNotCorrupt(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised on this rig")
	}

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
		t.Fatalf("bus.memory[0x%X] = 0x%08X, want 0", phase1LowAlias, stored)
	}
	if r.ctx.NeedHelper != HELPER_FSTORE {
		t.Fatalf("NeedHelper = %d, want HELPER_FSTORE", r.ctx.NeedHelper)
	}
}

func TestJIT_ARM64_IE64Load_Above4GiB_SlowPathRange(t *testing.T) {
	r := newJITTestRig(t)

	r.cpu.regs[1] = 0x1111111111111111
	r.cpu.regs[2] = 0x0000_0001_000A_0001
	r.ctx.NeedIOFallback = 0

	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedHelper != HELPER_LOAD {
		t.Fatalf("NeedHelper = %d, want HELPER_LOAD (slow-path range high addr must helper-exit)", r.ctx.NeedHelper)
	}
}

func TestJIT_ARM64_IE64Load_NearEndOfMemory_Bails(t *testing.T) {
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
			r.cpu.regs[2] = memSize - c.bytes + 1
			r.ctx.NeedIOFallback = 0

			r.compileAndRun(t, ie64Instr(OP_LOAD, 1, c.size, 0, 2, 0, 0))

			if r.ctx.NeedHelper != HELPER_LOAD {
				t.Fatalf("size=%s addr=MemSize-%d+1: NeedHelper = %d, want HELPER_LOAD",
					c.name, c.bytes, r.ctx.NeedHelper)
			}
		})
	}
}

func TestJIT_ARM64_IE64FLoad_NearEndOfMemory_Bails(t *testing.T) {
	r := newJITTestRig(t)
	if r.cpu.FPU == nil {
		t.Skip("FPU not initialised on this rig")
	}
	memSize := uint64(len(r.cpu.memory))

	r.cpu.regs[2] = memSize - 3
	r.ctx.NeedIOFallback = 0

	r.compileAndRun(t, ie64Instr(OP_FLOAD, 0, IE64_SIZE_L, 0, 2, 0, 0))

	if r.ctx.NeedHelper != HELPER_FLOAD {
		t.Fatalf("FLOAD addr=MemSize-3: NeedHelper = %d, want HELPER_FLOAD", r.ctx.NeedHelper)
	}
}

func TestJIT_ARM64_IE64Store_NearEndOfMemory_Bails(t *testing.T) {
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

			if r.ctx.NeedHelper != HELPER_STORE {
				t.Fatalf("size=%s addr=MemSize-%d+1: NeedHelper = %d, want HELPER_STORE",
					c.name, c.bytes, r.ctx.NeedHelper)
			}
		})
	}
}

// =============================================================================
// ARM64 end-to-end SparseBacking tests
// =============================================================================

func runIE64HighBackingTest_ARM64(t *testing.T, setup func(cpu *CPU64), instrs ...[]byte) (*CPU64, *SparseBacking) {
	t.Helper()
	const memSize = 64 * 1024 * 1024
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

	cpu := NewCPU64(bus)
	cpu.jitEnabled = true

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

func TestJIT_ARM64_IE64Load_HighBacking_EndToEnd(t *testing.T) {
	const want uint64 = 0x1234567890ABCDEF
	const lowAlias uint32 = 0x8000

	cpu, _ := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			for i := uint64(0); i < 8; i++ {
				cpu.bus.backing.Write8(phase1HighAddr+i, byte(want>>(8*i)))
			}
			binary.LittleEndian.PutUint64(cpu.memory[lowAlias:], 0xAAAAAAAAAAAAAAAA)
			cpu.regs[2] = phase1HighAddr
		},
		ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0),
	)

	if cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%016X, want 0x%016X", cpu.regs[1], want)
	}
	gotLow := binary.LittleEndian.Uint64(cpu.memory[lowAlias:])
	if gotLow != 0xAAAAAAAAAAAAAAAA {
		t.Fatalf("bus.memory[0x%X] = 0x%016X, want untouched", lowAlias, gotLow)
	}
}

func TestJIT_ARM64_IE64Store_HighBacking_EndToEnd(t *testing.T) {
	const payload uint64 = 0xFEDCBA0987654321
	const lowAlias uint32 = 0x8000

	cpu, backing := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			for i := uint32(0); i < 8; i++ {
				cpu.memory[lowAlias+i] = 0
			}
			cpu.regs[1] = payload
			cpu.regs[2] = phase1HighAddr
		},
		ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0),
	)

	var got uint64
	for i := uint64(0); i < 8; i++ {
		got |= uint64(backing.Read8(phase1HighAddr+i)) << (8 * i)
	}
	if got != payload {
		t.Fatalf("backing[0x%016X] = 0x%016X, want 0x%016X", phase1HighAddr, got, payload)
	}
	gotLow := binary.LittleEndian.Uint64(cpu.memory[lowAlias:])
	if gotLow != 0 {
		t.Fatalf("bus.memory[0x%X] = 0x%016X, want 0", lowAlias, gotLow)
	}
}
