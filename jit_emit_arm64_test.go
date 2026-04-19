// jit_emit_arm64_test.go - ARM64 emitter tests

//go:build arm64 && (linux || windows)

package main

import (
	"encoding/binary"
	"math"
	"runtime"
	"testing"
	"unsafe"
)

// negU64 converts a signed int64 to uint64, used for setting negative values
// in registers without triggering Go 1.26 constant overflow checks.
func negU64(v int64) uint64 {
	return uint64(v)
}

// ===========================================================================
// JIT Test Rig
// ===========================================================================

type jitTestRig struct {
	bus     *MachineBus
	cpu     *CPU64
	execMem *ExecMem
	ctx     *JITContext
}

func newJITTestRig(t *testing.T) *jitTestRig {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	em, err := AllocExecMem(1 << 20) // 1MB
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	ctx := newJITContext(cpu)
	t.Cleanup(func() { em.Free() })
	return &jitTestRig{bus: bus, cpu: cpu, execMem: em, ctx: ctx}
}

// compileAndRun compiles the given IE64 instructions to native code and executes them.
func (r *jitTestRig) compileAndRun(t *testing.T, instructions ...[]byte) {
	t.Helper()

	// Load instructions into memory
	offset := uint32(PROG_START)
	for _, instr := range instructions {
		copy(r.cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}
	// Add HALT as terminator
	copy(r.cpu.memory[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Scan and compile
	instrs := scanBlock(r.cpu.memory, PROG_START)
	if len(instrs) == 0 {
		t.Fatal("scanBlock returned 0 instructions")
	}

	// Remove HALT from compilation (it's a fallback instruction)
	compilableInstrs := instrs
	if instrs[len(instrs)-1].opcode == OP_HALT64 {
		compilableInstrs = instrs[:len(instrs)-1]
	}

	if len(compilableInstrs) == 0 {
		return
	}

	r.execMem.Reset()
	block, err := compileBlock(compilableInstrs, PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlock: %v", err)
	}

	// Update JITContext with latest pointers
	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&r.cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	if r.cpu.FPU != nil {
		r.ctx.FPUPtr = uintptr(unsafe.Pointer(r.cpu.FPU))
	}

	// Execute native code
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	// The epilogue stores X28 to regs[0]. X28 contains:
	//   lower 32 bits = target PC, upper 32 bits = instruction count.
	// Extract PC (lower 32 bits) and restore R0 = 0.
	combined := r.cpu.regs[0]
	r.cpu.PC = uint64(uint32(combined))
	r.cpu.regs[0] = 0

	runtime.KeepAlive(r.ctx)
	runtime.KeepAlive(r.execMem)
}

// ===========================================================================
// Data Movement Tests
// ===========================================================================

func TestARM64_MOVE_Imm(t *testing.T) {
	r := newJITTestRig(t)

	// MOVE.Q R1, #42
	r.compileAndRun(t, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42))

	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
}

func TestARM64_MOVE_Reg(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xDEADBEEF

	// MOVE.Q R1, R2
	r.compileAndRun(t, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] != 0xDEADBEEF {
		t.Fatalf("R1 = 0x%X, want 0xDEADBEEF", r.cpu.regs[1])
	}
}

func TestARM64_MOVE_AllSizes(t *testing.T) {
	r := newJITTestRig(t)

	tests := []struct {
		size byte
		imm  uint32
		want uint64
	}{
		{IE64_SIZE_B, 0xFF01, 0x01},
		{IE64_SIZE_W, 0xFFFF0102, 0x0102},
		{IE64_SIZE_L, 0xDEADBEEF, 0xDEADBEEF},
		{IE64_SIZE_Q, 0x12345678, 0x12345678},
	}

	for _, tt := range tests {
		r.cpu.regs[1] = 0 // reset
		r.compileAndRun(t, ie64Instr(OP_MOVE, 1, tt.size, 1, 0, 0, tt.imm))
		if r.cpu.regs[1] != tt.want {
			t.Errorf("MOVE.%d #0x%X: R1 = 0x%X, want 0x%X", tt.size, tt.imm, r.cpu.regs[1], tt.want)
		}
	}
}

func TestARM64_MOVT(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[1] = 0x00000000AABBCCDD

	// MOVT R1, #0x12345678 → upper 32 bits
	r.compileAndRun(t, ie64Instr(OP_MOVT, 1, 0, 1, 0, 0, 0x12345678))

	want := uint64(0x12345678AABBCCDD)
	if r.cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%016X, want 0x%016X", r.cpu.regs[1], want)
	}
}

func TestARM64_MOVEQ(t *testing.T) {
	r := newJITTestRig(t)

	// MOVEQ R1, #0xFFFFFFFF (-1 as int32, sign-extends to 0xFFFFFFFFFFFFFFFF)
	r.compileAndRun(t, ie64Instr(OP_MOVEQ, 1, 0, 1, 0, 0, 0xFFFFFFFF))

	if r.cpu.regs[1] != 0xFFFFFFFFFFFFFFFF {
		t.Fatalf("R1 = 0x%016X, want 0xFFFFFFFFFFFFFFFF", r.cpu.regs[1])
	}
}

func TestARM64_MOVEQ_Positive(t *testing.T) {
	r := newJITTestRig(t)

	r.compileAndRun(t, ie64Instr(OP_MOVEQ, 1, 0, 1, 0, 0, 100))

	if r.cpu.regs[1] != 100 {
		t.Fatalf("R1 = %d, want 100", r.cpu.regs[1])
	}
}

func TestARM64_LEA(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0x1000

	r.compileAndRun(t, ie64Instr(OP_LEA, 1, 0, 1, 2, 0, 0x100))

	if r.cpu.regs[1] != 0x1100 {
		t.Fatalf("R1 = 0x%X, want 0x1100", r.cpu.regs[1])
	}
}

// ===========================================================================
// ALU Tests
// ===========================================================================

func TestARM64_ADD_Reg(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 42

	r.compileAndRun(t, ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 142 {
		t.Fatalf("R1 = %d, want 142", r.cpu.regs[1])
	}
}

func TestARM64_ADD_Imm(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 100

	r.compileAndRun(t, ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 2, 0, 50))

	if r.cpu.regs[1] != 150 {
		t.Fatalf("R1 = %d, want 150", r.cpu.regs[1])
	}
}

func TestARM64_ADD_AllSizes(t *testing.T) {
	r := newJITTestRig(t)

	tests := []struct {
		size byte
		a, b uint64
		want uint64
	}{
		{IE64_SIZE_B, 0x80, 0x90, 0x10},         // wraps at 8 bits
		{IE64_SIZE_W, 0xFF00, 0x0200, 0x0100},   // wraps at 16 bits
		{IE64_SIZE_L, 0xFFFFFF00, 0x200, 0x100}, // wraps at 32 bits
		{IE64_SIZE_Q, 100, 200, 300},
	}

	for _, tt := range tests {
		r.cpu.regs[2] = tt.a
		r.cpu.regs[3] = tt.b
		r.compileAndRun(t, ie64Instr(OP_ADD, 1, tt.size, 0, 2, 3, 0))
		if r.cpu.regs[1] != tt.want {
			t.Errorf("ADD.%d 0x%X + 0x%X: R1 = 0x%X, want 0x%X", tt.size, tt.a, tt.b, r.cpu.regs[1], tt.want)
		}
	}
}

func TestARM64_SUB(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 42

	r.compileAndRun(t, ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 58 {
		t.Fatalf("R1 = %d, want 58", r.cpu.regs[1])
	}
}

func TestARM64_MULU(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 7
	r.cpu.regs[3] = 6

	r.compileAndRun(t, ie64Instr(OP_MULU, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
}

func TestARM64_MULS(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = negU64(-7)
	r.cpu.regs[3] = 6

	r.compileAndRun(t, ie64Instr(OP_MULS, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	want := negU64(-42)
	if r.cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%X, want 0x%X", r.cpu.regs[1], want)
	}
}

func TestARM64_DIVU(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 6

	r.compileAndRun(t, ie64Instr(OP_DIVU, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 7 {
		t.Fatalf("R1 = %d, want 7", r.cpu.regs[1])
	}
}

func TestARM64_DIVS(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = negU64(-42)
	r.cpu.regs[3] = 6

	r.compileAndRun(t, ie64Instr(OP_DIVS, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	want := negU64(-7)
	if r.cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%X, want 0x%X", r.cpu.regs[1], want)
	}
}

func TestARM64_DIVU_ByZero(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 0

	r.compileAndRun(t, ie64Instr(OP_DIVU, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0 (div by zero)", r.cpu.regs[1])
	}
}

func TestARM64_MOD(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 5

	r.compileAndRun(t, ie64Instr(OP_MOD64, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 2 {
		t.Fatalf("R1 = %d, want 2 (42 %% 5)", r.cpu.regs[1])
	}
}

func TestARM64_NEG(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42

	r.compileAndRun(t, ie64Instr(OP_NEG, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	want := negU64(-42)
	if r.cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%X, want 0x%X", r.cpu.regs[1], want)
	}
}

func TestARM64_NOT(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xFF

	r.compileAndRun(t, ie64Instr(OP_NOT64, 1, IE64_SIZE_B, 0, 2, 0, 0))

	if r.cpu.regs[1] != 0x00 {
		t.Fatalf("R1 = 0x%X, want 0x00 (NOT 0xFF masked to byte)", r.cpu.regs[1])
	}
}

func TestARM64_AND(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xFF
	r.cpu.regs[3] = 0x0F

	r.compileAndRun(t, ie64Instr(OP_AND64, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0x0F {
		t.Fatalf("R1 = 0x%X, want 0x0F", r.cpu.regs[1])
	}
}

func TestARM64_OR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xF0
	r.cpu.regs[3] = 0x0F

	r.compileAndRun(t, ie64Instr(OP_OR64, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0xFF {
		t.Fatalf("R1 = 0x%X, want 0xFF", r.cpu.regs[1])
	}
}

func TestARM64_EOR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xFF
	r.cpu.regs[3] = 0x0F

	r.compileAndRun(t, ie64Instr(OP_EOR, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0xF0 {
		t.Fatalf("R1 = 0x%X, want 0xF0", r.cpu.regs[1])
	}
}

func TestARM64_LSL(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 1

	r.compileAndRun(t, ie64Instr(OP_LSL, 1, IE64_SIZE_Q, 1, 2, 0, 4))

	if r.cpu.regs[1] != 16 {
		t.Fatalf("R1 = %d, want 16", r.cpu.regs[1])
	}
}

func TestARM64_LSR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 256

	r.compileAndRun(t, ie64Instr(OP_LSR, 1, IE64_SIZE_Q, 1, 2, 0, 4))

	if r.cpu.regs[1] != 16 {
		t.Fatalf("R1 = %d, want 16", r.cpu.regs[1])
	}
}

func TestARM64_ASR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = negU64(-256) // 0xFFFFFFFFFFFFFF00

	r.compileAndRun(t, ie64Instr(OP_ASR, 1, IE64_SIZE_Q, 1, 2, 0, 4))

	want := negU64(-16)
	if r.cpu.regs[1] != want {
		t.Fatalf("R1 = 0x%X, want 0x%X", r.cpu.regs[1], want)
	}
}

func TestARM64_CLZ(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0x00010000 // 1 << 16

	r.compileAndRun(t, ie64Instr(OP_CLZ, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	// CLZ operates on 32-bit value: leading zeros of 0x00010000 = 15
	if r.cpu.regs[1] != 15 {
		t.Fatalf("R1 = %d, want 15", r.cpu.regs[1])
	}
}

func TestARM64_R0_WriteDiscard(t *testing.T) {
	r := newJITTestRig(t)

	// ADD.Q R0, R0, #42 — should be no-op (R0 hardwired zero)
	r.compileAndRun(t, ie64Instr(OP_ADD, 0, IE64_SIZE_Q, 1, 0, 0, 42))

	if r.cpu.regs[0] != 0 {
		t.Fatalf("R0 = %d, want 0 (hardwired zero)", r.cpu.regs[0])
	}
}

func TestARM64_SpilledReg(t *testing.T) {
	r := newJITTestRig(t)

	// MOVE.Q R20, #12345 — R20 is spilled (not in ARM64 register)
	r.compileAndRun(t, ie64Instr(OP_MOVE, 20, IE64_SIZE_Q, 1, 0, 0, 12345))

	if r.cpu.regs[20] != 12345 {
		t.Fatalf("R20 = %d, want 12345", r.cpu.regs[20])
	}
}

// ===========================================================================
// Multi-instruction Tests
// ===========================================================================

func TestARM64_MultiALU(t *testing.T) {
	r := newJITTestRig(t)

	// R1 = 10, R2 = R1 + 20 = 30, R3 = R2 * 3 = 90
	r.compileAndRun(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 10),
		ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 1, 0, 20),
		ie64Instr(OP_MULU, 3, IE64_SIZE_Q, 1, 2, 0, 3),
	)

	if r.cpu.regs[1] != 10 {
		t.Errorf("R1 = %d, want 10", r.cpu.regs[1])
	}
	if r.cpu.regs[2] != 30 {
		t.Errorf("R2 = %d, want 30", r.cpu.regs[2])
	}
	if r.cpu.regs[3] != 90 {
		t.Errorf("R3 = %d, want 90", r.cpu.regs[3])
	}
}

// ===========================================================================
// Memory Access Tests
// ===========================================================================

func TestARM64_LOAD_Q_FastPath(t *testing.T) {
	r := newJITTestRig(t)

	addr := uint32(0x2000)
	val := uint64(0xDEADBEEFCAFEBABE)
	*(*uint64)(unsafe.Pointer(&r.cpu.memory[addr])) = val

	r.cpu.regs[2] = uint64(addr)

	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 1, 2, 0, 0))

	if r.cpu.regs[1] != val {
		t.Fatalf("R1 = 0x%X, want 0x%X", r.cpu.regs[1], val)
	}
}

func TestARM64_STORE_Q_FastPath(t *testing.T) {
	r := newJITTestRig(t)

	addr := uint32(0x3000)
	r.cpu.regs[1] = 0xCAFEBABE12345678
	r.cpu.regs[2] = uint64(addr)

	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 1, 2, 0, 0))

	got := *(*uint64)(unsafe.Pointer(&r.cpu.memory[addr]))
	if got != 0xCAFEBABE12345678 {
		t.Fatalf("mem[0x3000] = 0x%X, want 0xCAFEBABE12345678", got)
	}
}

func TestARM64_LOAD_AllSizes(t *testing.T) {
	r := newJITTestRig(t)

	addr := uint32(0x4000)
	*(*uint64)(unsafe.Pointer(&r.cpu.memory[addr])) = 0x0102030405060708

	tests := []struct {
		size byte
		want uint64
	}{
		{IE64_SIZE_B, 0x08},
		{IE64_SIZE_W, 0x0708},
		{IE64_SIZE_L, 0x05060708},
		{IE64_SIZE_Q, 0x0102030405060708},
	}

	for _, tt := range tests {
		r.cpu.regs[2] = uint64(addr)
		r.compileAndRun(t, ie64Instr(OP_LOAD, 1, tt.size, 1, 2, 0, 0))
		if r.cpu.regs[1] != tt.want {
			t.Errorf("LOAD.%d: R1 = 0x%X, want 0x%X", tt.size, r.cpu.regs[1], tt.want)
		}
	}
}

func TestARM64_STORE_AllSizes(t *testing.T) {
	r := newJITTestRig(t)

	tests := []struct {
		size byte
		val  uint64
		want uint64
	}{
		{IE64_SIZE_B, 0xAB, 0xAB},
		{IE64_SIZE_W, 0xABCD, 0xABCD},
		{IE64_SIZE_L, 0xABCDEF12, 0xABCDEF12},
		{IE64_SIZE_Q, 0xABCDEF1234567890, 0xABCDEF1234567890},
	}

	for _, tt := range tests {
		addr := uint32(0x5000)
		*(*uint64)(unsafe.Pointer(&r.cpu.memory[addr])) = 0

		r.cpu.regs[1] = tt.val
		r.cpu.regs[2] = uint64(addr)
		r.compileAndRun(t, ie64Instr(OP_STORE, 1, tt.size, 1, 2, 0, 0))

		var got uint64
		switch tt.size {
		case IE64_SIZE_B:
			got = uint64(r.cpu.memory[addr])
		case IE64_SIZE_W:
			got = uint64(*(*uint16)(unsafe.Pointer(&r.cpu.memory[addr])))
		case IE64_SIZE_L:
			got = uint64(*(*uint32)(unsafe.Pointer(&r.cpu.memory[addr])))
		case IE64_SIZE_Q:
			got = *(*uint64)(unsafe.Pointer(&r.cpu.memory[addr]))
		}
		if got != tt.want {
			t.Errorf("STORE.%d 0x%X: mem = 0x%X, want 0x%X", tt.size, tt.val, got, tt.want)
		}
	}
}

func TestARM64_LOAD_Displacement(t *testing.T) {
	r := newJITTestRig(t)

	base := uint32(0x6000)
	*(*uint64)(unsafe.Pointer(&r.cpu.memory[base+0x100])) = 0x42

	r.cpu.regs[2] = uint64(base)

	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 1, 2, 0, 0x100))

	if r.cpu.regs[1] != 0x42 {
		t.Fatalf("R1 = 0x%X, want 0x42", r.cpu.regs[1])
	}
}

func TestARM64_PUSH_POP(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xDEADBEEF

	initialSP := r.cpu.regs[31]

	r.compileAndRun(t,
		ie64Instr(OP_PUSH64, 0, 0, 0, 2, 0, 0),
		ie64Instr(OP_POP64, 3, 0, 0, 0, 0, 0),
	)

	if r.cpu.regs[3] != 0xDEADBEEF {
		t.Fatalf("R3 = 0x%X, want 0xDEADBEEF", r.cpu.regs[3])
	}
	if r.cpu.regs[31] != initialSP {
		t.Fatalf("SP = 0x%X, want 0x%X (should be restored)", r.cpu.regs[31], initialSP)
	}
}

func TestARM64_PUSH_POP_SPVerify(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42

	initialSP := r.cpu.regs[31]

	r.compileAndRun(t, ie64Instr(OP_PUSH64, 0, 0, 0, 2, 0, 0))

	if r.cpu.regs[31] != initialSP-8 {
		t.Fatalf("SP after push = 0x%X, want 0x%X", r.cpu.regs[31], initialSP-8)
	}
}

// ===========================================================================
// Control Flow Tests
// ===========================================================================

func TestARM64_BRA(t *testing.T) {
	r := newJITTestRig(t)

	r.compileAndRun(t, ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 16))

	want := uint64(PROG_START + 16)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, want)
	}
}

func TestARM64_BEQ_Taken(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 42

	r.compileAndRun(t, ie64Instr(OP_BEQ, 0, 0, 0, 2, 3, 24))

	want := uint64(PROG_START + 24)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X (branch should be taken)", r.cpu.PC, want)
	}
}

func TestARM64_BEQ_NotTaken(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 43

	r.compileAndRun(t, ie64Instr(OP_BEQ, 0, 0, 0, 2, 3, 24))

	want := uint64(PROG_START + IE64_INSTR_SIZE)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X (branch should NOT be taken)", r.cpu.PC, want)
	}
}

func TestARM64_BNE(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 43

	r.compileAndRun(t, ie64Instr(OP_BNE, 0, 0, 0, 2, 3, 16))

	want := uint64(PROG_START + 16)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, want)
	}
}

func TestARM64_BLT(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = negU64(-1)
	r.cpu.regs[3] = 1

	r.compileAndRun(t, ie64Instr(OP_BLT, 0, 0, 0, 2, 3, 16))

	want := uint64(PROG_START + 16)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X (signed -1 < 1)", r.cpu.PC, want)
	}
}

func TestARM64_BGE(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 5
	r.cpu.regs[3] = 5

	r.compileAndRun(t, ie64Instr(OP_BGE, 0, 0, 0, 2, 3, 16))

	want := uint64(PROG_START + 16)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X (5 >= 5)", r.cpu.PC, want)
	}
}

func TestARM64_BGT(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 6
	r.cpu.regs[3] = 5

	r.compileAndRun(t, ie64Instr(OP_BGT, 0, 0, 0, 2, 3, 16))

	want := uint64(PROG_START + 16)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X (6 > 5)", r.cpu.PC, want)
	}
}

func TestARM64_BLE(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 5
	r.cpu.regs[3] = 5

	r.compileAndRun(t, ie64Instr(OP_BLE, 0, 0, 0, 2, 3, 16))

	want := uint64(PROG_START + 16)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X (5 <= 5)", r.cpu.PC, want)
	}
}

func TestARM64_BHI(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xFFFFFFFFFFFFFFFF // unsigned max
	r.cpu.regs[3] = 0

	r.compileAndRun(t, ie64Instr(OP_BHI, 0, 0, 0, 2, 3, 16))

	want := uint64(PROG_START + 16)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X (unsigned max > 0)", r.cpu.PC, want)
	}
}

func TestARM64_BLS(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0
	r.cpu.regs[3] = 1

	r.compileAndRun(t, ie64Instr(OP_BLS, 0, 0, 0, 2, 3, 16))

	want := uint64(PROG_START + 16)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X (0 <= 1 unsigned)", r.cpu.PC, want)
	}
}

func TestARM64_JMP_Displacement(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0x2000

	r.compileAndRun(t, ie64Instr(OP_JMP, 0, 0, 1, 2, 0, 0x100))

	want := uint64(0x2100)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, want)
	}
}

func TestARM64_JSR_RTS(t *testing.T) {
	r := newJITTestRig(t)

	initialSP := r.cpu.regs[31]

	r.compileAndRun(t, ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 0x100))

	if r.cpu.regs[31] != initialSP-8 {
		t.Fatalf("SP after JSR = 0x%X, want 0x%X", r.cpu.regs[31], initialSP-8)
	}

	want := uint64(PROG_START + 0x100)
	if r.cpu.PC != want {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, want)
	}

	retAddr := *(*uint64)(unsafe.Pointer(&r.cpu.memory[r.cpu.regs[31]]))
	wantRet := uint64(PROG_START + IE64_INSTR_SIZE)
	if retAddr != wantRet {
		t.Fatalf("return addr = 0x%X, want 0x%X", retAddr, wantRet)
	}
}

func TestARM64_NOP(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[1] = 42

	r.compileAndRun(t, ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))

	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42 (NOP should not change state)", r.cpu.regs[1])
	}
}

// ===========================================================================
// RTI / WAIT Tests
// ===========================================================================

func TestARM64_RTI_InBlock(t *testing.T) {
	r := newJITTestRig(t)

	// Simulate being in an interrupt handler:
	// Set up stack with a return address, put a MOVE before RTI
	returnAddr := uint64(PROG_START + 0x200)
	r.cpu.regs[31] = STACK_START // initialize SP
	r.cpu.regs[31] -= 8
	sp := uint32(r.cpu.regs[31])
	binary.LittleEndian.PutUint64(r.cpu.memory[sp:], returnAddr)
	r.cpu.inInterrupt.Store(true)

	// Build block: MOVE R1, #42; RTI
	// RTI is a block terminator, so compileAndRun won't strip it or add HALT
	offset := uint32(PROG_START)
	move := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42)
	rti := ie64Instr(OP_RTI64, 0, 0, 0, 0, 0, 0)
	copy(r.cpu.memory[offset:], move)
	copy(r.cpu.memory[offset+8:], rti)

	// Scan and compile the full block [MOVE, RTI]
	instrs := scanBlock(r.cpu.memory, PROG_START)
	if len(instrs) != 2 {
		t.Fatalf("scanBlock returned %d instructions, want 2", len(instrs))
	}
	if instrs[1].opcode != OP_RTI64 {
		t.Fatalf("instrs[1].opcode = 0x%02X, want 0x%02X", instrs[1].opcode, OP_RTI64)
	}

	r.execMem.Reset()
	block, err := compileBlock(instrs, PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlock: %v", err)
	}

	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&r.cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	// Read packed PC+count
	r.cpu.PC = r.cpu.regs[0]
	r.cpu.regs[0] = 0

	// Verify MOVE executed
	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42 (MOVE should have executed before RTI bail)", r.cpu.regs[1])
	}

	// Verify bail was set
	if r.ctx.NeedIOFallback == 0 {
		t.Fatal("NeedIOFallback should be set after RTI bail")
	}

	// Verify PC points to RTI instruction
	rtiPC := uint64(PROG_START + 8)
	if uint32(r.cpu.PC) != uint32(rtiPC) {
		t.Fatalf("PC = 0x%X, want 0x%X (should point to RTI for interpreter re-execution)", r.cpu.PC, rtiPC)
	}

	// Now execute RTI via interpreter (simulating jitExecute's bail path)
	r.ctx.NeedIOFallback = 0
	r.cpu.PC = rtiPC
	r.cpu.StepOne()

	// Verify RTI effects
	if r.cpu.PC != returnAddr {
		t.Fatalf("PC = 0x%X, want 0x%X (RTI should have popped return address)", r.cpu.PC, returnAddr)
	}
	if r.cpu.regs[31] != STACK_START {
		t.Fatalf("SP = 0x%X, want 0x%X (RTI should have restored SP)", r.cpu.regs[31], STACK_START)
	}
	if r.cpu.inInterrupt.Load() {
		t.Fatal("inInterrupt should be false after RTI")
	}

	runtime.KeepAlive(r.ctx)
	runtime.KeepAlive(r.execMem)
}

func TestARM64_WAIT_InBlock(t *testing.T) {
	r := newJITTestRig(t)

	// Build block: MOVE R1, #99; WAIT #0
	offset := uint32(PROG_START)
	move := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 99)
	wait := ie64Instr(OP_WAIT64, 0, 0, 0, 0, 0, 0)
	copy(r.cpu.memory[offset:], move)
	copy(r.cpu.memory[offset+8:], wait)

	instrs := scanBlock(r.cpu.memory, PROG_START)
	if len(instrs) != 2 {
		t.Fatalf("scanBlock returned %d instructions, want 2", len(instrs))
	}
	if instrs[1].opcode != OP_WAIT64 {
		t.Fatalf("instrs[1].opcode = 0x%02X, want 0x%02X", instrs[1].opcode, OP_WAIT64)
	}

	r.execMem.Reset()
	block, err := compileBlock(instrs, PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlock: %v", err)
	}

	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&r.cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	r.cpu.PC = r.cpu.regs[0]
	r.cpu.regs[0] = 0

	// Verify MOVE executed
	if r.cpu.regs[1] != 99 {
		t.Fatalf("R1 = %d, want 99 (MOVE should have executed before WAIT bail)", r.cpu.regs[1])
	}

	// Verify bail was set
	if r.ctx.NeedIOFallback == 0 {
		t.Fatal("NeedIOFallback should be set after WAIT bail")
	}

	// Verify PC points to WAIT instruction
	waitPC := uint64(PROG_START + 8)
	if uint32(r.cpu.PC) != uint32(waitPC) {
		t.Fatalf("PC = 0x%X, want 0x%X (should point to WAIT for interpreter re-execution)", r.cpu.PC, waitPC)
	}

	// Execute WAIT via interpreter
	r.ctx.NeedIOFallback = 0
	r.cpu.PC = waitPC
	r.cpu.StepOne()

	// WAIT in step mode just advances PC
	expectedPC := waitPC + IE64_INSTR_SIZE
	if r.cpu.PC != expectedPC {
		t.Fatalf("PC = 0x%X, want 0x%X (WAIT should advance PC)", r.cpu.PC, expectedPC)
	}

	runtime.KeepAlive(r.ctx)
	runtime.KeepAlive(r.execMem)
}

func TestARM64_RTI_RegisterLiveness(t *testing.T) {
	// Test that RTI correctly includes R31 in register analysis
	instrs := []JITInstr{
		{opcode: OP_MOVE, rd: 1, size: IE64_SIZE_Q, xbit: 1, imm32: 42},
		{opcode: OP_RTI64, pcOffset: 8},
	}
	br := analyzeBlockRegs(instrs)

	// RTI reads R31 (pops from stack) and writes R31 (increments SP)
	if br.read&(1<<31) == 0 {
		t.Fatal("RTI should mark R31 as read")
	}
	if br.written&(1<<31) == 0 {
		t.Fatal("RTI should mark R31 as written")
	}

	// instrWrittenRegs for RTI should include R31
	w := instrWrittenRegs(&instrs[1])
	if w&(1<<31) == 0 {
		t.Fatal("instrWrittenRegs for RTI should include R31")
	}
}

// ===========================================================================
// JIT vs Interpreter Comparison Tests
// ===========================================================================

func TestJIT_vs_Interpreter_ALU(t *testing.T) {
	programs := []struct {
		name   string
		instrs [][]byte
		setup  func(cpu *CPU64)
	}{
		{
			"ADD chain",
			[][]byte{
				ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 100),
				ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 1, 0, 200),
				ie64Instr(OP_SUB, 3, IE64_SIZE_Q, 1, 2, 0, 50),
				ie64Instr(OP_MULU, 4, IE64_SIZE_Q, 1, 3, 0, 2),
			},
			nil,
		},
		{
			"Logic ops",
			[][]byte{
				ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0xFF),
				ie64Instr(OP_AND64, 2, IE64_SIZE_Q, 1, 1, 0, 0x0F),
				ie64Instr(OP_OR64, 3, IE64_SIZE_Q, 1, 1, 0, 0x100),
				ie64Instr(OP_EOR, 4, IE64_SIZE_Q, 0, 2, 3, 0),
			},
			nil,
		},
		{
			"Shift ops",
			[][]byte{
				ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 1),
				ie64Instr(OP_LSL, 2, IE64_SIZE_Q, 1, 1, 0, 8),
				ie64Instr(OP_LSR, 3, IE64_SIZE_Q, 1, 2, 0, 4),
				ie64Instr(OP_NEG, 4, IE64_SIZE_Q, 0, 3, 0, 0),
			},
			nil,
		},
	}

	for _, prog := range programs {
		t.Run(prog.name, func(t *testing.T) {
			// Run on interpreter
			interpRig := newIE64TestRig()
			if prog.setup != nil {
				prog.setup(interpRig.cpu)
			}
			interpRig.executeN(prog.instrs...)

			// Run on JIT
			jitRig := newJITTestRig(t)
			if prog.setup != nil {
				prog.setup(jitRig.cpu)
			}
			jitRig.compileAndRun(t, prog.instrs...)

			// Compare all 32 registers
			for i := range 32 {
				if interpRig.cpu.regs[i] != jitRig.cpu.regs[i] {
					t.Errorf("R%d: interpreter=0x%X, JIT=0x%X", i, interpRig.cpu.regs[i], jitRig.cpu.regs[i])
				}
			}
		})
	}
}

func TestJIT_vs_Interpreter_Memory(t *testing.T) {
	// Run on interpreter
	interpRig := newIE64TestRig()
	interpRig.cpu.regs[1] = 0xDEADBEEF
	interpRig.cpu.regs[2] = 0x2000
	interpRig.executeN(
		ie64Instr(OP_STORE, 1, IE64_SIZE_L, 1, 2, 0, 0),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 1, 2, 0, 0),
	)

	// Run on JIT
	jitRig := newJITTestRig(t)
	jitRig.cpu.regs[1] = 0xDEADBEEF
	jitRig.cpu.regs[2] = 0x2000
	jitRig.compileAndRun(t,
		ie64Instr(OP_STORE, 1, IE64_SIZE_L, 1, 2, 0, 0),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 1, 2, 0, 0),
	)

	if interpRig.cpu.regs[3] != jitRig.cpu.regs[3] {
		t.Fatalf("R3: interpreter=0x%X, JIT=0x%X", interpRig.cpu.regs[3], jitRig.cpu.regs[3])
	}
}

// ===========================================================================
// FPU Tests — Category A: Pure integer bitwise on FP registers
// ===========================================================================

func TestARM64_FMOV(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x40490FDB // pi

	r.compileAndRun(t, ie64Instr(OP_FMOV, 5, 0, 0, 2, 0, 0))

	if r.cpu.FPU.FPRegs[5] != 0x40490FDB {
		t.Fatalf("F5 = 0x%X, want 0x40490FDB", r.cpu.FPU.FPRegs[5])
	}
}

func TestARM64_FABS(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = 0xC0490FDB // -pi

	r.compileAndRun(t, ie64Instr(OP_FABS, 2, 0, 0, 1, 0, 0))

	if r.cpu.FPU.FPRegs[2] != 0x40490FDB { // +pi
		t.Fatalf("F2 = 0x%X, want 0x40490FDB (+pi)", r.cpu.FPU.FPRegs[2])
	}
	// Check CC: positive normal → no flags in CC bits
	cc := r.cpu.FPU.FPSR & 0x0F000000
	if cc != 0 {
		t.Fatalf("FPSR CC = 0x%X, want 0 (positive normal)", cc)
	}
}

func TestARM64_FABS_NegZero(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = 0x80000000 // -0.0

	r.compileAndRun(t, ie64Instr(OP_FABS, 2, 0, 0, 1, 0, 0))

	if r.cpu.FPU.FPRegs[2] != 0x00000000 { // +0.0
		t.Fatalf("F2 = 0x%X, want 0x00000000 (+0.0)", r.cpu.FPU.FPRegs[2])
	}
	cc := r.cpu.FPU.FPSR & 0x0F000000
	if cc != IE64_FPU_CC_Z {
		t.Fatalf("FPSR CC = 0x%X, want 0x%X (zero)", cc, IE64_FPU_CC_Z)
	}
}

func TestARM64_FNEG(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = 0x40490FDB // +pi

	r.compileAndRun(t, ie64Instr(OP_FNEG, 2, 0, 0, 1, 0, 0))

	if r.cpu.FPU.FPRegs[2] != 0xC0490FDB { // -pi
		t.Fatalf("F2 = 0x%X, want 0xC0490FDB (-pi)", r.cpu.FPU.FPRegs[2])
	}
	cc := r.cpu.FPU.FPSR & 0x0F000000
	if cc != IE64_FPU_CC_N {
		t.Fatalf("FPSR CC = 0x%X, want 0x%X (negative)", cc, IE64_FPU_CC_N)
	}
}

func TestARM64_FMOVI(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[3] = 0x42280000 // 42.0 as float32 bits

	r.compileAndRun(t, ie64Instr(OP_FMOVI, 1, 0, 0, 3, 0, 0))

	if r.cpu.FPU.FPRegs[1] != 0x42280000 {
		t.Fatalf("F1 = 0x%X, want 0x42280000 (42.0)", r.cpu.FPU.FPRegs[1])
	}
}

func TestARM64_FMOVO(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[3] = 0x42280000 // 42.0

	r.compileAndRun(t, ie64Instr(OP_FMOVO, 1, 0, 0, 3, 0, 0))

	if r.cpu.regs[1] != 0x42280000 {
		t.Fatalf("R1 = 0x%X, want 0x42280000", r.cpu.regs[1])
	}
}

func TestARM64_FMOVECR(t *testing.T) {
	r := newJITTestRig(t)

	// Load Pi (index 0)
	r.compileAndRun(t, ie64Instr(OP_FMOVECR, 1, 0, 0, 0, 0, 0))

	want := ie64FmovecrROMTable[0] // Pi
	if r.cpu.FPU.FPRegs[1] != want {
		t.Fatalf("F1 = 0x%X, want 0x%X (Pi)", r.cpu.FPU.FPRegs[1], want)
	}
}

func TestARM64_FMOVECR_OutOfRange(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = 0xDEADBEEF

	// Index > 15 → should store 0
	r.compileAndRun(t, ie64Instr(OP_FMOVECR, 1, 0, 0, 0, 0, 200))

	if r.cpu.FPU.FPRegs[1] != 0 {
		t.Fatalf("F1 = 0x%X, want 0 (out-of-range index)", r.cpu.FPU.FPRegs[1])
	}
}

func TestARM64_FMOVSR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPSR = 0x08000003 // CC_N | some exception bits

	r.compileAndRun(t, ie64Instr(OP_FMOVSR, 1, 0, 0, 0, 0, 0))

	if r.cpu.regs[1] != 0x08000003 {
		t.Fatalf("R1 = 0x%X, want 0x08000003", r.cpu.regs[1])
	}
}

func TestARM64_FMOVCR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPCR = 0x02 // Round toward floor

	r.compileAndRun(t, ie64Instr(OP_FMOVCR, 1, 0, 0, 0, 0, 0))

	if r.cpu.regs[1] != 0x02 {
		t.Fatalf("R1 = 0x%X, want 0x02", r.cpu.regs[1])
	}
}

func TestARM64_FMOVSC(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[3] = 0xFFFFFFFF // all bits set

	r.compileAndRun(t, ie64Instr(OP_FMOVSC, 0, 0, 0, 3, 0, 0))

	// Should be masked with IE64_FPU_FPSR_MASK (0x0F00000F)
	if r.cpu.FPU.FPSR != IE64_FPU_FPSR_MASK {
		t.Fatalf("FPSR = 0x%X, want 0x%X", r.cpu.FPU.FPSR, IE64_FPU_FPSR_MASK)
	}
}

func TestARM64_FMOVCC(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[3] = 0x03 // Round toward ceil

	r.compileAndRun(t, ie64Instr(OP_FMOVCC, 0, 0, 0, 3, 0, 0))

	if r.cpu.FPU.FPCR != 0x03 {
		t.Fatalf("FPCR = 0x%X, want 0x03", r.cpu.FPU.FPCR)
	}
}

// ===========================================================================
// FPU Tests — Category B: Native ARM64 FP arithmetic
// ===========================================================================

func TestARM64_FADD(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = math.Float32bits(3.0)
	r.cpu.FPU.FPRegs[2] = math.Float32bits(4.0)

	r.compileAndRun(t, ie64Instr(OP_FADD, 3, 0, 0, 1, 2, 0))

	got := math.Float32frombits(r.cpu.FPU.FPRegs[3])
	if got != 7.0 {
		t.Fatalf("F3 = %f, want 7.0", got)
	}
}

func TestARM64_FSUB(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = math.Float32bits(10.0)
	r.cpu.FPU.FPRegs[2] = math.Float32bits(3.0)

	r.compileAndRun(t, ie64Instr(OP_FSUB, 3, 0, 0, 1, 2, 0))

	got := math.Float32frombits(r.cpu.FPU.FPRegs[3])
	if got != 7.0 {
		t.Fatalf("F3 = %f, want 7.0", got)
	}
}

func TestARM64_FMUL(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = math.Float32bits(3.0)
	r.cpu.FPU.FPRegs[2] = math.Float32bits(4.0)

	r.compileAndRun(t, ie64Instr(OP_FMUL, 3, 0, 0, 1, 2, 0))

	got := math.Float32frombits(r.cpu.FPU.FPRegs[3])
	if got != 12.0 {
		t.Fatalf("F3 = %f, want 12.0", got)
	}
}

func TestARM64_FDIV(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = math.Float32bits(15.0)
	r.cpu.FPU.FPRegs[2] = math.Float32bits(3.0)

	r.compileAndRun(t, ie64Instr(OP_FDIV, 3, 0, 0, 1, 2, 0))

	got := math.Float32frombits(r.cpu.FPU.FPRegs[3])
	if got != 5.0 {
		t.Fatalf("F3 = %f, want 5.0", got)
	}
}

func TestARM64_FSQRT(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = math.Float32bits(25.0)

	r.compileAndRun(t, ie64Instr(OP_FSQRT, 2, 0, 0, 1, 0, 0))

	got := math.Float32frombits(r.cpu.FPU.FPRegs[2])
	if got != 5.0 {
		t.Fatalf("F2 = %f, want 5.0", got)
	}
}

func TestARM64_FINT_Nearest(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPCR = 0 // nearest
	r.cpu.FPU.FPRegs[1] = math.Float32bits(3.5)

	r.compileAndRun(t, ie64Instr(OP_FINT, 2, 0, 0, 1, 0, 0))

	got := math.Float32frombits(r.cpu.FPU.FPRegs[2])
	if got != 4.0 { // round to even
		t.Fatalf("F2 = %f, want 4.0 (nearest even)", got)
	}
}

func TestARM64_FINT_Truncate(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPCR = 1 // toward zero
	r.cpu.FPU.FPRegs[1] = math.Float32bits(3.9)

	r.compileAndRun(t, ie64Instr(OP_FINT, 2, 0, 0, 1, 0, 0))

	got := math.Float32frombits(r.cpu.FPU.FPRegs[2])
	if got != 3.0 {
		t.Fatalf("F2 = %f, want 3.0 (truncate)", got)
	}
}

func TestARM64_FINT_Floor(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPCR = 2 // floor
	r.cpu.FPU.FPRegs[1] = math.Float32bits(-3.1)

	r.compileAndRun(t, ie64Instr(OP_FINT, 2, 0, 0, 1, 0, 0))

	got := math.Float32frombits(r.cpu.FPU.FPRegs[2])
	if got != -4.0 {
		t.Fatalf("F2 = %f, want -4.0 (floor)", got)
	}
}

func TestARM64_FINT_Ceil(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPCR = 3 // ceil
	r.cpu.FPU.FPRegs[1] = math.Float32bits(3.1)

	r.compileAndRun(t, ie64Instr(OP_FINT, 2, 0, 0, 1, 0, 0))

	got := math.Float32frombits(r.cpu.FPU.FPRegs[2])
	if got != 4.0 {
		t.Fatalf("F2 = %f, want 4.0 (ceil)", got)
	}
}

// ===========================================================================
// FPU Tests — FCMP
// ===========================================================================

func TestARM64_FCMP_LessThan(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = math.Float32bits(1.0)
	r.cpu.FPU.FPRegs[2] = math.Float32bits(5.0)

	r.compileAndRun(t, ie64Instr(OP_FCMP, 3, 0, 0, 1, 2, 0))

	if r.cpu.regs[3] != negU64(-1) {
		t.Fatalf("R3 = 0x%X, want -1", r.cpu.regs[3])
	}
	cc := r.cpu.FPU.FPSR & 0x0F000000
	if cc != IE64_FPU_CC_N {
		t.Fatalf("FPSR CC = 0x%X, want 0x%X (N)", cc, IE64_FPU_CC_N)
	}
}

func TestARM64_FCMP_Equal(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = math.Float32bits(5.0)
	r.cpu.FPU.FPRegs[2] = math.Float32bits(5.0)

	r.compileAndRun(t, ie64Instr(OP_FCMP, 3, 0, 0, 1, 2, 0))

	if r.cpu.regs[3] != 0 {
		t.Fatalf("R3 = 0x%X, want 0", r.cpu.regs[3])
	}
	cc := r.cpu.FPU.FPSR & 0x0F000000
	if cc != IE64_FPU_CC_Z {
		t.Fatalf("FPSR CC = 0x%X, want 0x%X (Z)", cc, IE64_FPU_CC_Z)
	}
}

func TestARM64_FCMP_GreaterThan(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = math.Float32bits(10.0)
	r.cpu.FPU.FPRegs[2] = math.Float32bits(5.0)

	r.compileAndRun(t, ie64Instr(OP_FCMP, 3, 0, 0, 1, 2, 0))

	if r.cpu.regs[3] != 1 {
		t.Fatalf("R3 = 0x%X, want 1", r.cpu.regs[3])
	}
}

func TestARM64_FCMP_NaN(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = 0x7FC00000 // NaN
	r.cpu.FPU.FPRegs[2] = math.Float32bits(5.0)

	r.compileAndRun(t, ie64Instr(OP_FCMP, 3, 0, 0, 1, 2, 0))

	if r.cpu.regs[3] != 0 {
		t.Fatalf("R3 = 0x%X, want 0 (NaN → unordered)", r.cpu.regs[3])
	}
	cc := r.cpu.FPU.FPSR & 0x0F000000
	if cc != IE64_FPU_CC_NAN {
		t.Fatalf("FPSR CC = 0x%X, want 0x%X (NAN)", cc, IE64_FPU_CC_NAN)
	}
	ex := r.cpu.FPU.FPSR & 0x0F
	if ex&IE64_FPU_EX_IO == 0 {
		t.Fatalf("FPSR exceptions = 0x%X, want IO flag set", ex)
	}
}

// ===========================================================================
// FPU Tests — Conversions
// ===========================================================================

func TestARM64_FCVTIF(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[3] = 42

	r.compileAndRun(t, ie64Instr(OP_FCVTIF, 1, 0, 0, 3, 0, 0))

	got := math.Float32frombits(r.cpu.FPU.FPRegs[1])
	if got != 42.0 {
		t.Fatalf("F1 = %f, want 42.0", got)
	}
}

func TestARM64_FCVTIF_Negative(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[3] = negU64(-100)

	r.compileAndRun(t, ie64Instr(OP_FCVTIF, 1, 0, 0, 3, 0, 0))

	got := math.Float32frombits(r.cpu.FPU.FPRegs[1])
	if got != -100.0 {
		t.Fatalf("F1 = %f, want -100.0", got)
	}
}

func TestARM64_FCVTFI(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = math.Float32bits(42.7)

	r.compileAndRun(t, ie64Instr(OP_FCVTFI, 3, 0, 0, 1, 0, 0))

	// Truncation toward zero
	if r.cpu.regs[3] != 42 {
		t.Fatalf("R3 = %d, want 42", int64(r.cpu.regs[3]))
	}
}

func TestARM64_FCVTFI_Negative(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = math.Float32bits(-42.7)

	r.compileAndRun(t, ie64Instr(OP_FCVTFI, 3, 0, 0, 1, 0, 0))

	if r.cpu.regs[3] != negU64(-42) {
		t.Fatalf("R3 = 0x%X, want -42 (0x%X)", r.cpu.regs[3], negU64(-42))
	}
}

// ===========================================================================
// FPU Tests — Memory operations
// ===========================================================================

func TestARM64_FLOAD(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0x2000
	// Store a float value in memory at 0x2000
	val := math.Float32bits(123.456)
	r.cpu.memory[0x2000] = byte(val)
	r.cpu.memory[0x2001] = byte(val >> 8)
	r.cpu.memory[0x2002] = byte(val >> 16)
	r.cpu.memory[0x2003] = byte(val >> 24)

	r.compileAndRun(t, ie64Instr(OP_FLOAD, 1, 0, 0, 2, 0, 0))

	if r.cpu.FPU.FPRegs[1] != val {
		t.Fatalf("F1 = 0x%X, want 0x%X", r.cpu.FPU.FPRegs[1], val)
	}
}

func TestARM64_FSTORE(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0x2000
	r.cpu.FPU.FPRegs[1] = math.Float32bits(789.012)

	r.compileAndRun(t, ie64Instr(OP_FSTORE, 1, 0, 0, 2, 0, 0))

	stored := uint32(r.cpu.memory[0x2000]) |
		uint32(r.cpu.memory[0x2001])<<8 |
		uint32(r.cpu.memory[0x2002])<<16 |
		uint32(r.cpu.memory[0x2003])<<24
	want := math.Float32bits(789.012)
	if stored != want {
		t.Fatalf("mem[0x2000] = 0x%X, want 0x%X", stored, want)
	}
}

// ===========================================================================
// FPU Tests — Condition codes
// ===========================================================================

func TestARM64_FPU_CC_NaN(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = 0x7FC00000 // NaN

	// FABS of NaN is still NaN (but with sign bit cleared: 0x7FC00000)
	r.compileAndRun(t, ie64Instr(OP_FABS, 2, 0, 0, 1, 0, 0))

	cc := r.cpu.FPU.FPSR & 0x0F000000
	if cc != IE64_FPU_CC_NAN {
		t.Fatalf("FPSR CC = 0x%X, want 0x%X (NAN)", cc, IE64_FPU_CC_NAN)
	}
}

func TestARM64_FPU_CC_Inf(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = 0x7F800000 // +Inf

	r.compileAndRun(t, ie64Instr(OP_FABS, 2, 0, 0, 1, 0, 0))

	cc := r.cpu.FPU.FPSR & 0x0F000000
	if cc != IE64_FPU_CC_I {
		t.Fatalf("FPSR CC = 0x%X, want 0x%X (I)", cc, IE64_FPU_CC_I)
	}
}

func TestARM64_FPU_CC_NegInf(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[1] = 0xFF800000 // -Inf

	// FNEG(-Inf) = +Inf
	r.compileAndRun(t, ie64Instr(OP_FNEG, 2, 0, 0, 1, 0, 0))

	if r.cpu.FPU.FPRegs[2] != 0x7F800000 {
		t.Fatalf("F2 = 0x%X, want 0x7F800000 (+Inf)", r.cpu.FPU.FPRegs[2])
	}
	cc := r.cpu.FPU.FPSR & 0x0F000000
	if cc != IE64_FPU_CC_I {
		t.Fatalf("FPSR CC = 0x%X, want 0x%X (I)", cc, IE64_FPU_CC_I)
	}
}

func TestARM64_FPU_CC_ExceptionPreserved(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPSR = 0x00000005 // pre-existing exceptions (IO | OE)
	r.cpu.FPU.FPRegs[1] = math.Float32bits(-5.0)

	r.compileAndRun(t, ie64Instr(OP_FABS, 2, 0, 0, 1, 0, 0))

	// CC should be cleared, but exception bits preserved
	ex := r.cpu.FPU.FPSR & 0x0F
	if ex != 0x05 {
		t.Fatalf("FPSR exceptions = 0x%X, want 0x05 (preserved)", ex)
	}
}

// ===========================================================================
// FPU Tests — JIT vs Interpreter comparison
// ===========================================================================

func TestJIT_vs_Interpreter_FPU(t *testing.T) {
	tests := []struct {
		name  string
		instr []byte
		setup func(cpu *CPU64)
	}{
		{
			"FMOV",
			ie64Instr(OP_FMOV, 5, 0, 0, 2, 0, 0),
			func(cpu *CPU64) { cpu.FPU.FPRegs[2] = 0x40490FDB },
		},
		{
			"FABS negative",
			ie64Instr(OP_FABS, 2, 0, 0, 1, 0, 0),
			func(cpu *CPU64) { cpu.FPU.FPRegs[1] = 0xC0490FDB },
		},
		{
			"FNEG positive",
			ie64Instr(OP_FNEG, 2, 0, 0, 1, 0, 0),
			func(cpu *CPU64) { cpu.FPU.FPRegs[1] = 0x40490FDB },
		},
		{
			"FMOVI",
			ie64Instr(OP_FMOVI, 1, 0, 0, 3, 0, 0),
			func(cpu *CPU64) { cpu.regs[3] = 0x42280000 },
		},
		{
			"FMOVO",
			ie64Instr(OP_FMOVO, 5, 0, 0, 3, 0, 0),
			func(cpu *CPU64) { cpu.FPU.FPRegs[3] = 0x42280000 },
		},
		{
			"FMOVECR Pi",
			ie64Instr(OP_FMOVECR, 1, 0, 0, 0, 0, 0),
			nil,
		},
		{
			"FMOVECR 1.0",
			ie64Instr(OP_FMOVECR, 1, 0, 0, 0, 0, 8),
			nil,
		},
		{
			"FADD",
			ie64Instr(OP_FADD, 3, 0, 0, 1, 2, 0),
			func(cpu *CPU64) {
				cpu.FPU.FPRegs[1] = math.Float32bits(3.0)
				cpu.FPU.FPRegs[2] = math.Float32bits(4.0)
			},
		},
		{
			"FSUB",
			ie64Instr(OP_FSUB, 3, 0, 0, 1, 2, 0),
			func(cpu *CPU64) {
				cpu.FPU.FPRegs[1] = math.Float32bits(10.0)
				cpu.FPU.FPRegs[2] = math.Float32bits(3.0)
			},
		},
		{
			"FMUL",
			ie64Instr(OP_FMUL, 3, 0, 0, 1, 2, 0),
			func(cpu *CPU64) {
				cpu.FPU.FPRegs[1] = math.Float32bits(6.0)
				cpu.FPU.FPRegs[2] = math.Float32bits(7.0)
			},
		},
		{
			"FDIV",
			ie64Instr(OP_FDIV, 3, 0, 0, 1, 2, 0),
			func(cpu *CPU64) {
				cpu.FPU.FPRegs[1] = math.Float32bits(21.0)
				cpu.FPU.FPRegs[2] = math.Float32bits(3.0)
			},
		},
		{
			"FSQRT",
			ie64Instr(OP_FSQRT, 2, 0, 0, 1, 0, 0),
			func(cpu *CPU64) { cpu.FPU.FPRegs[1] = math.Float32bits(144.0) },
		},
		{
			"FINT nearest",
			ie64Instr(OP_FINT, 2, 0, 0, 1, 0, 0),
			func(cpu *CPU64) {
				cpu.FPU.FPCR = 0
				cpu.FPU.FPRegs[1] = math.Float32bits(2.5)
			},
		},
		{
			"FCMP less",
			ie64Instr(OP_FCMP, 5, 0, 0, 1, 2, 0),
			func(cpu *CPU64) {
				cpu.FPU.FPRegs[1] = math.Float32bits(1.0)
				cpu.FPU.FPRegs[2] = math.Float32bits(5.0)
			},
		},
		{
			"FCMP equal",
			ie64Instr(OP_FCMP, 5, 0, 0, 1, 2, 0),
			func(cpu *CPU64) {
				cpu.FPU.FPRegs[1] = math.Float32bits(5.0)
				cpu.FPU.FPRegs[2] = math.Float32bits(5.0)
			},
		},
		{
			"FCMP greater",
			ie64Instr(OP_FCMP, 5, 0, 0, 1, 2, 0),
			func(cpu *CPU64) {
				cpu.FPU.FPRegs[1] = math.Float32bits(10.0)
				cpu.FPU.FPRegs[2] = math.Float32bits(5.0)
			},
		},
		{
			"FCVTIF positive",
			ie64Instr(OP_FCVTIF, 1, 0, 0, 3, 0, 0),
			func(cpu *CPU64) { cpu.regs[3] = 42 },
		},
		{
			"FCVTFI positive",
			ie64Instr(OP_FCVTFI, 5, 0, 0, 1, 0, 0),
			func(cpu *CPU64) { cpu.FPU.FPRegs[1] = math.Float32bits(42.9) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Interpreter
			interpRig := newIE64TestRig()
			if tc.setup != nil {
				tc.setup(interpRig.cpu)
			}
			interpRig.executeN(tc.instr)

			// JIT
			jitRig := newJITTestRig(t)
			if tc.setup != nil {
				tc.setup(jitRig.cpu)
			}
			jitRig.compileAndRun(t, tc.instr)

			// Compare FPU state
			for i := range 16 {
				if interpRig.cpu.FPU.FPRegs[i] != jitRig.cpu.FPU.FPRegs[i] {
					t.Errorf("F%d: interpreter=0x%X, JIT=0x%X", i, interpRig.cpu.FPU.FPRegs[i], jitRig.cpu.FPU.FPRegs[i])
				}
			}

			// Compare FPSR (CC + exceptions)
			if interpRig.cpu.FPU.FPSR != jitRig.cpu.FPU.FPSR {
				t.Errorf("FPSR: interpreter=0x%X, JIT=0x%X", interpRig.cpu.FPU.FPSR, jitRig.cpu.FPU.FPSR)
			}

			// Compare integer registers (for FMOVO, FCMP, FCVTFI, FMOVSR, FMOVCR)
			for i := range 32 {
				if interpRig.cpu.regs[i] != jitRig.cpu.regs[i] {
					t.Errorf("R%d: interpreter=0x%X, JIT=0x%X", i, interpRig.cpu.regs[i], jitRig.cpu.regs[i])
				}
			}
		})
	}
}

// ===========================================================================
// Backward Branch Tests
// ===========================================================================

// compileAndRunBlock compiles a block including its terminator (BRA/BNE etc.)
// and executes it once via callNative. Returns the packed combined value from
// regs[0] so the caller can inspect both PC and instruction count.
func (r *jitTestRig) compileAndRunBlock(t *testing.T, instructions ...[]byte) uint64 {
	t.Helper()

	offset := uint32(PROG_START)
	for _, instr := range instructions {
		copy(r.cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}

	instrs := scanBlock(r.cpu.memory, PROG_START)
	if len(instrs) == 0 {
		t.Fatal("scanBlock returned 0 instructions")
	}

	r.execMem.Reset()
	block, err := compileBlock(instrs, PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlock: %v", err)
	}

	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&r.cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))

	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	combined := r.cpu.regs[0]
	r.cpu.PC = uint64(uint32(combined))
	r.cpu.regs[0] = 0

	runtime.KeepAlive(r.ctx)
	runtime.KeepAlive(r.execMem)
	return combined
}

func TestARM64_BackwardBranch_Simple(t *testing.T) {
	// MOVE R1, #100; SUB R1, R1, #1; BNE R1, R0, -8 (back to SUB)
	// 100 iterations; R1 = 0 at exit.
	// Budget = 4095, bodySize = 2 (SUB+BNE), so 100 iters < 4095/2 = ~2047.
	// Should complete in one native call.
	rig := newJITTestRig(t)
	neg8 := uint32(0xFFFFFFF8) // -8 as uint32

	rig.compileAndRun(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 100),
		ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 1, 0, 1),
		ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 1, 0, neg8),
	)

	if rig.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0", rig.cpu.regs[1])
	}
}

func TestARM64_BackwardBranch_InteriorLoop(t *testing.T) {
	// Block with preamble + inner loop targeting an interior instruction:
	// [0] MOVE R1, #100
	// [1] MOVE R2, #0
	// [2] ADD R2, R2, #1     <-- loop target
	// [3] SUB R1, R1, #1
	// [4] BNE R1, R0, -16    targets index 2 (ADD)
	// BNE exits after 100 iterations. R1=0, R2=100.
	rig := newJITTestRig(t)
	neg16 := uint32(0xFFFFFFF0) // -16 as uint32

	rig.compileAndRun(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 100),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0),
		ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 2, 0, 1),
		ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 1, 0, 1),
		ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 1, 0, neg16),
	)

	if rig.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0", rig.cpu.regs[1])
	}
	if rig.cpu.regs[2] != 100 {
		t.Fatalf("R2 = %d, want 100", rig.cpu.regs[2])
	}
}

func TestARM64_BackwardBranch_InstructionCount(t *testing.T) {
	// Simple 2-instruction loop (SUB+BNE), 100 iterations.
	// Expected instruction count:
	//   X7 accumulates bodySize=2 per backward branch taken (99 taken branches).
	//   X7 = 99 * 2 = 198.
	//   At BNE fall-through (not taken), staticCount = instrIdx+1 = 3.
	//   But BNE fall-through doesn't go through backward path, it falls through to
	//   the final epilogue with staticCount = len(instrs) = 3.
	//   Wait — actually with HALT stripped by compileAndRun, the block has 3 instrs
	//   and the last is BNE (not a terminator), so final epilogue runs.
	//   Total = X7(198) + staticCount(3) = 201.
	// Let's verify.
	rig := newJITTestRig(t)
	neg8 := uint32(0xFFFFFFF8)

	combined := rig.compileAndRunBlock(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 100),
		ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 1, 0, 1),
		ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 1, 0, neg8),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)

	if rig.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0", rig.cpu.regs[1])
	}

	count := uint32(combined >> 32)
	// MOVE(1) + [SUB(1) + BNE(1)] × 100 + HALT(1) = 1 + 200 + 1 = 202
	// Actually: 99 taken BNE (each adds bodySize=2 to X7), then
	// iteration 100: SUB makes R1=0, BNE not taken → falls through to HALT.
	// So: MOVE + 100×SUB + 100×BNE + HALT = 202
	// X7 = 99*2 = 198 (from backward branches)
	// HALT exits at instrIdx=3, staticCount=4
	// Total = 198 + 4 = 202
	expected := uint32(202)
	if count != expected {
		t.Fatalf("instruction count = %d, want %d", count, expected)
	}
}

func TestARM64_BackwardBranch_Budget(t *testing.T) {
	// Loop with 5000 iterations (bodySize=2, budget=4095).
	// Budget fires when X7+2 >= 4095, i.e. X7 >= 4093.
	// X7 increases by 2 each iter: fires at X7=4094 (2047 iters), rolls back to 4092.
	// Total from dispatcher perspective: multiple calls until R1=0.
	// Use the full dispatcher (runJITProgram) to handle budget exits.
	neg8 := uint32(0xFFFFFFF8)
	cpu := runJITProgram(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 5000),
		ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 1, 0, 1),
		ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 1, 0, neg8),
	)
	if cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0 after budget-limited loop", cpu.regs[1])
	}
}

func TestARM64_BackwardBranch_WithEarlyExit(t *testing.T) {
	// Loop with an early exit condition:
	// [0] MOVE R1, #100   ; counter
	// [1] MOVE R2, #50    ; early exit threshold
	// [2] SUB R1, R1, #1  ; decrement  <-- BNE target
	// [3] BEQ R1, R2, +16 ; early exit when R1 == R2 (forward to HALT)
	// [4] BNE R1, R0, -16 ; backward loop (to SUB at index 2)
	// [5] HALT
	// Should exit when R1=50 (after 50 iterations).
	rig := newJITTestRig(t)
	neg16 := uint32(0xFFFFFFF0)

	rig.compileAndRun(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 100),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 50),
		ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 1, 0, 1),
		ie64Instr(OP_BEQ, 0, IE64_SIZE_Q, 0, 1, 2, 16), // forward exit
		ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 1, 0, neg16),
	)

	if rig.cpu.regs[1] != 50 {
		t.Fatalf("R1 = %d, want 50 (early exit)", rig.cpu.regs[1])
	}
}

func TestARM64_BackwardBranch_BRA(t *testing.T) {
	// BRA as backward loop terminator:
	// [0] MOVE R1, #10
	// [1] MOVE R2, #0
	// [2] ADD R2, R2, #1     <-- loop top
	// [3] SUB R1, R1, #1
	// [4] BNE R1, R0, +8     ; if R1 != 0, skip over HALT (forward)
	// [5] HALT                ; R1 == 0, stop
	// [6] BRA -32             ; back to ADD (index 2)
	// Wait — BRA is a block terminator, so the block ends at BRA.
	// HALT is also a block terminator. Hmm — the scanner stops at the first
	// terminator: it will stop at HALT (index 5), so BRA is in a separate block.
	//
	// Better structure — use BEQ for exit + BRA for loop:
	// [0] MOVE R1, #10
	// [1] MOVE R2, #0
	// [2] ADD R2, R2, #1     <-- loop top
	// [3] SUB R1, R1, #1
	// [4] BEQ R1, R0, +8     ; if R1 == 0, skip BRA (exit block)
	// [5] BRA -24             ; back to ADD (index 2), terminates block
	//
	// Block: [MOVE, MOVE, ADD, SUB, BEQ, BRA] — BRA terminates.
	// 10 iterations; R1=0, R2=10.
	rig := newJITTestRig(t)
	neg24 := uint32(0xFFFFFFE8) // -24 as uint32

	rig.compileAndRunBlock(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 10),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0),
		ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 2, 0, 1),
		ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 1, 0, 1),
		ie64Instr(OP_BEQ, 0, IE64_SIZE_Q, 0, 1, 0, 8), // forward exit
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, neg24),       // backward to ADD
	)

	if rig.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0", rig.cpu.regs[1])
	}
	if rig.cpu.regs[2] != 10 {
		t.Fatalf("R2 = %d, want 10", rig.cpu.regs[2])
	}
}

func TestARM64_BackwardBranch_NestedLoops(t *testing.T) {
	// Outer loop: 5 iterations. Inner loop: 10 iterations per outer.
	// [0] MOVE R1, #5       ; outer counter
	// [1] MOVE R3, #0       ; accumulator
	// [2] MOVE R2, #10      ; inner counter  <-- outer loop target
	// [3] ADD R3, R3, #1    ; accumulate     <-- inner loop target
	// [4] SUB R2, R2, #1
	// [5] BNE R2, R0, -16   ; back to ADD (inner loop)
	// [6] SUB R1, R1, #1
	// [7] BNE R1, R0, -40   ; back to MOVE R2 (outer loop)
	// R3 should be 5 * 10 = 50.
	rig := newJITTestRig(t)
	neg16 := uint32(0xFFFFFFF0) // -16 as uint32
	neg40 := uint32(0xFFFFFFD8) // -40 as uint32

	rig.compileAndRun(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 5),
		ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 1, 0, 0, 0),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 10),
		ie64Instr(OP_ADD, 3, IE64_SIZE_Q, 1, 3, 0, 1),
		ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1),
		ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 2, 0, neg16),
		ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 1, 0, 1),
		ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 1, 0, neg40),
	)

	if rig.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0", rig.cpu.regs[1])
	}
	if rig.cpu.regs[2] != 0 {
		t.Fatalf("R2 = %d, want 0", rig.cpu.regs[2])
	}
	if rig.cpu.regs[3] != 50 {
		t.Fatalf("R3 = %d, want 50", rig.cpu.regs[3])
	}
}
