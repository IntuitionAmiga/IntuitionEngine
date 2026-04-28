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
	"testing"
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
