// jit_emit_amd64_test.go - x86-64 emitter tests

//go:build amd64 && linux

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

	// Phase 2 return channel: epilogue writes full 64-bit PC to
	// ctx.RetPC and retired count to ctx.RetCount. Read PC here; reset
	// the legacy regs[0] mirror so subsequent runs see a clean slot.
	r.cpu.PC = r.ctx.RetPC
	r.ctx.RetPC = 0
	r.ctx.RetCount = 0
	r.cpu.regs[0] = 0

	runtime.KeepAlive(r.ctx)
	runtime.KeepAlive(r.execMem)
}

// ===========================================================================
// Data Movement Tests
// ===========================================================================

func TestAMD64_MOVE_Imm(t *testing.T) {
	r := newJITTestRig(t)

	// MOVE.Q R1, #42
	r.compileAndRun(t, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42))

	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
}

func TestAMD64_MOVE_Reg(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xDEADBEEF

	// MOVE.Q R1, R2
	r.compileAndRun(t, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] != 0xDEADBEEF {
		t.Fatalf("R1 = 0x%X, want 0xDEADBEEF", r.cpu.regs[1])
	}
}

func TestAMD64_MOVE_SizeB(t *testing.T) {
	r := newJITTestRig(t)

	// MOVE.B R1, #0x1FF (should mask to 0xFF)
	r.compileAndRun(t, ie64Instr(OP_MOVE, 1, IE64_SIZE_B, 1, 0, 0, 0x1FF))

	if r.cpu.regs[1] != 0xFF {
		t.Fatalf("R1 = 0x%X, want 0xFF", r.cpu.regs[1])
	}
}

func TestAMD64_MOVE_SizeW(t *testing.T) {
	r := newJITTestRig(t)

	// MOVE.W R1, #0x1FFFF (should mask to 0xFFFF)
	r.compileAndRun(t, ie64Instr(OP_MOVE, 1, IE64_SIZE_W, 1, 0, 0, 0x1FFFF))

	if r.cpu.regs[1] != 0xFFFF {
		t.Fatalf("R1 = 0x%X, want 0xFFFF", r.cpu.regs[1])
	}
}

func TestAMD64_MOVE_SizeL(t *testing.T) {
	r := newJITTestRig(t)

	// MOVE.L R1, #0xDEADBEEF
	r.compileAndRun(t, ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xDEADBEEF))

	if r.cpu.regs[1] != 0xDEADBEEF {
		t.Fatalf("R1 = 0x%X, want 0xDEADBEEF", r.cpu.regs[1])
	}
}

func TestAMD64_MOVE_RegSizeB(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xDEADBEEF

	// MOVE.B R1, R2 (should mask to 0xEF)
	r.compileAndRun(t, ie64Instr(OP_MOVE, 1, IE64_SIZE_B, 0, 2, 0, 0))

	if r.cpu.regs[1] != 0xEF {
		t.Fatalf("R1 = 0x%X, want 0xEF", r.cpu.regs[1])
	}
}

func TestAMD64_MOVE_R0_Discard(t *testing.T) {
	r := newJITTestRig(t)

	// MOVE.Q R0, #42 — writes to R0 should be discarded
	r.compileAndRun(t, ie64Instr(OP_MOVE, 0, IE64_SIZE_Q, 1, 0, 0, 42))

	// R0 should remain 0 (the compileAndRun extracts PC from regs[0] and zeros it)
	if r.cpu.regs[0] != 0 {
		t.Fatalf("R0 = %d, want 0 (writes to R0 should be discarded)", r.cpu.regs[0])
	}
}

func TestAMD64_MOVT(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[1] = 0x00000000DEADBEEF

	// MOVT R1, #0x12345678 — set upper 32 bits
	r.compileAndRun(t, ie64Instr(OP_MOVT, 1, 0, 0, 0, 0, 0x12345678))

	if r.cpu.regs[1] != 0x12345678DEADBEEF {
		t.Fatalf("R1 = 0x%016X, want 0x12345678DEADBEEF", r.cpu.regs[1])
	}
}

func TestAMD64_MOVEQ(t *testing.T) {
	r := newJITTestRig(t)

	// MOVEQ R1, #0xFFFFFFFF — sign-extend -1 to 64-bit
	r.compileAndRun(t, ie64Instr(OP_MOVEQ, 1, 0, 0, 0, 0, 0xFFFFFFFF))

	if r.cpu.regs[1] != negU64(-1) {
		t.Fatalf("R1 = 0x%016X, want 0xFFFFFFFFFFFFFFFF", r.cpu.regs[1])
	}
}

func TestAMD64_MOVEQ_Positive(t *testing.T) {
	r := newJITTestRig(t)

	// MOVEQ R1, #100 — sign-extend positive value
	r.compileAndRun(t, ie64Instr(OP_MOVEQ, 1, 0, 0, 0, 0, 100))

	if r.cpu.regs[1] != 100 {
		t.Fatalf("R1 = %d, want 100", r.cpu.regs[1])
	}
}

func TestAMD64_LEA(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 1000

	// LEA R1, 200(R2) -> R1 = R2 + 200 = 1200
	r.compileAndRun(t, ie64Instr(OP_LEA, 1, 0, 0, 2, 0, 200))

	if r.cpu.regs[1] != 1200 {
		t.Fatalf("R1 = %d, want 1200", r.cpu.regs[1])
	}
}

func TestAMD64_LEA_Negative(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 1000

	// LEA R1, -200(R2) -> R1 = R2 + (-200) = 800
	neg200 := uint32(0xFFFFFF38) // -200 as uint32
	r.compileAndRun(t, ie64Instr(OP_LEA, 1, 0, 0, 2, 0, neg200))

	if r.cpu.regs[1] != 800 {
		t.Fatalf("R1 = %d, want 800", r.cpu.regs[1])
	}
}

// ===========================================================================
// Arithmetic Tests
// ===========================================================================

func TestAMD64_ADD_Reg(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 42

	// ADD.Q R1, R2, R3
	r.compileAndRun(t, ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 142 {
		t.Fatalf("R1 = %d, want 142", r.cpu.regs[1])
	}
}

func TestAMD64_ADD_Imm(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 100

	// ADD.Q R1, R2, #50
	r.compileAndRun(t, ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 2, 0, 50))

	if r.cpu.regs[1] != 150 {
		t.Fatalf("R1 = %d, want 150", r.cpu.regs[1])
	}
}

func TestAMD64_ADD_SizeL(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xFFFFFFFF
	r.cpu.regs[3] = 1

	// ADD.L R1, R2, R3 — should wrap at 32-bit
	r.compileAndRun(t, ie64Instr(OP_ADD, 1, IE64_SIZE_L, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (32-bit wrap)", r.cpu.regs[1])
	}
}

func TestAMD64_SUB(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 42

	// SUB.Q R1, R2, R3
	r.compileAndRun(t, ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 58 {
		t.Fatalf("R1 = %d, want 58", r.cpu.regs[1])
	}
}

func TestAMD64_NEG(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42

	// NEG.Q R1, R2
	r.compileAndRun(t, ie64Instr(OP_NEG, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] != negU64(-42) {
		t.Fatalf("R1 = 0x%X, want -42", r.cpu.regs[1])
	}
}

func TestAMD64_R0_WriteDiscard(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 42

	// ADD.Q R0, R2, R3 — writes to R0 should be discarded
	r.compileAndRun(t, ie64Instr(OP_ADD, 0, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[0] != 0 {
		t.Fatalf("R0 = %d, want 0", r.cpu.regs[0])
	}
}

// ===========================================================================
// Multiply / Divide / Modulo Tests
// ===========================================================================

func TestAMD64_MULU(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 7
	r.cpu.regs[3] = 6

	// MULU.Q R1, R2, R3
	r.compileAndRun(t, ie64Instr(OP_MULU, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
}

func TestAMD64_MULU_Imm(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 100

	// MULU.Q R1, R2, #10
	r.compileAndRun(t, ie64Instr(OP_MULU, 1, IE64_SIZE_Q, 1, 2, 0, 10))

	if r.cpu.regs[1] != 1000 {
		t.Fatalf("R1 = %d, want 1000", r.cpu.regs[1])
	}
}

func TestAMD64_MULS(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = negU64(-7)
	r.cpu.regs[3] = 6

	// MULS.Q R1, R2, R3
	r.compileAndRun(t, ie64Instr(OP_MULS, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != negU64(-42) {
		t.Fatalf("R1 = 0x%X, want -42", r.cpu.regs[1])
	}
}

func TestAMD64_MULHU_MULHS(t *testing.T) {
	r := newJITTestRig(t)

	r.cpu.regs[2] = ^uint64(0)
	r.cpu.regs[3] = 2
	r.compileAndRun(t, ie64Instr(OP_MULHU, 1, IE64_SIZE_Q, 0, 2, 3, 0))
	if r.cpu.regs[1] != 1 {
		t.Fatalf("MULHU R1 = 0x%X, want 1", r.cpu.regs[1])
	}

	r.cpu.regs[1] = 0
	r.cpu.regs[2] = negU64(-2)
	r.cpu.regs[3] = 3
	r.compileAndRun(t, ie64Instr(OP_MULHS, 1, IE64_SIZE_Q, 0, 2, 3, 0))
	if r.cpu.regs[1] != ^uint64(0) {
		t.Fatalf("MULHS R1 = 0x%X, want 0xFFFFFFFFFFFFFFFF", r.cpu.regs[1])
	}
}

func TestAMD64_MODS_NativeSizes(t *testing.T) {
	tests := []struct {
		name string
		size byte
		a    uint64
		b    uint64
		want uint64
	}{
		{"byte negative", IE64_SIZE_B, 0xFB, 0x03, 0xFE},                               // -5 % 3 = -2
		{"word negative", IE64_SIZE_W, 0xFED4, 0x0007, 0xFFFA},                         // -300 % 7 = -6
		{"long negative", IE64_SIZE_L, 0xFFFE7960, 0x0000012C, 0xFFFFFF9C},             // -100000 % 300 = -100
		{"quad negative", IE64_SIZE_Q, negU64(-10), 3, negU64(-1)},                     // -10 % 3 = -1
		{"zero divisor", IE64_SIZE_Q, 1234, 0, 0},                                      // defined IE64 behavior
		{"min overflow pair", IE64_SIZE_Q, uint64(1) << 63, negU64(-1), 0},             // avoid host IDIV trap
		{"immediate divisor", IE64_SIZE_L, uint64(uint32(negU64(-25))), 7, 0xFFFFFFFC}, // -25 % 7 = -4
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newJITTestRig(t)
			r.cpu.regs[2] = tt.a
			r.cpu.regs[3] = tt.b
			if tt.name == "immediate divisor" {
				r.compileAndRun(t, ie64Instr(OP_MODS, 1, tt.size, 1, 2, 0, uint32(tt.b)))
			} else {
				r.compileAndRun(t, ie64Instr(OP_MODS, 1, tt.size, 0, 2, 3, 0))
			}

			if r.ctx.NeedIOFallback != 0 {
				t.Fatalf("NeedIOFallback = %d, want 0 (MODS should be native)", r.ctx.NeedIOFallback)
			}
			if r.cpu.regs[1] != tt.want {
				t.Fatalf("R1 = 0x%016X, want 0x%016X", r.cpu.regs[1], tt.want)
			}
		})
	}
}

func TestAMD64_DIVU(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 7

	// DIVU.Q R1, R2, R3
	r.compileAndRun(t, ie64Instr(OP_DIVU, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 6 {
		t.Fatalf("R1 = %d, want 6", r.cpu.regs[1])
	}
}

func TestAMD64_DIVU_ByZero(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 0

	// DIVU.Q R1, R2, R3 — divide by zero should return 0
	r.compileAndRun(t, ie64Instr(OP_DIVU, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0 (div by zero)", r.cpu.regs[1])
	}
}

func TestAMD64_DIVS(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = negU64(-42)
	r.cpu.regs[3] = 7

	// DIVS.Q R1, R2, R3
	r.compileAndRun(t, ie64Instr(OP_DIVS, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != negU64(-6) {
		t.Fatalf("R1 = 0x%X, want -6", r.cpu.regs[1])
	}
}

func TestAMD64_MOD(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 47
	r.cpu.regs[3] = 7

	// MOD.Q R1, R2, R3
	r.compileAndRun(t, ie64Instr(OP_MOD64, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 5 {
		t.Fatalf("R1 = %d, want 5", r.cpu.regs[1])
	}
}

// ===========================================================================
// Logic Tests
// ===========================================================================

func TestAMD64_AND(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xFF00FF00
	r.cpu.regs[3] = 0x0F0F0F0F

	// AND.Q R1, R2, R3
	r.compileAndRun(t, ie64Instr(OP_AND64, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0x0F000F00 {
		t.Fatalf("R1 = 0x%X, want 0x0F000F00", r.cpu.regs[1])
	}
}

func TestAMD64_OR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xFF00
	r.cpu.regs[3] = 0x00FF

	r.compileAndRun(t, ie64Instr(OP_OR64, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0xFFFF {
		t.Fatalf("R1 = 0x%X, want 0xFFFF", r.cpu.regs[1])
	}
}

func TestAMD64_EOR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xFFFF
	r.cpu.regs[3] = 0x0F0F

	r.compileAndRun(t, ie64Instr(OP_EOR, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0xF0F0 {
		t.Fatalf("R1 = 0x%X, want 0xF0F0", r.cpu.regs[1])
	}
}

func TestAMD64_NOT(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0

	// NOT.Q R1, R2
	r.compileAndRun(t, ie64Instr(OP_NOT64, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] != 0xFFFFFFFFFFFFFFFF {
		t.Fatalf("R1 = 0x%X, want all ones", r.cpu.regs[1])
	}
}

// ===========================================================================
// Shift Tests
// ===========================================================================

func TestAMD64_LSL(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 1
	r.cpu.regs[3] = 8

	// LSL.Q R1, R2, R3
	r.compileAndRun(t, ie64Instr(OP_LSL, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 256 {
		t.Fatalf("R1 = %d, want 256", r.cpu.regs[1])
	}
}

func TestAMD64_LSL_Imm(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 1

	// LSL.Q R1, R2, #16
	r.compileAndRun(t, ie64Instr(OP_LSL, 1, IE64_SIZE_Q, 1, 2, 0, 16))

	if r.cpu.regs[1] != 65536 {
		t.Fatalf("R1 = %d, want 65536", r.cpu.regs[1])
	}
}

func TestAMD64_LSR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 256
	r.cpu.regs[3] = 4

	// LSR.Q R1, R2, R3
	r.compileAndRun(t, ie64Instr(OP_LSR, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != 16 {
		t.Fatalf("R1 = %d, want 16", r.cpu.regs[1])
	}
}

func TestAMD64_ASR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = negU64(-256)
	r.cpu.regs[3] = 4

	// ASR.Q R1, R2, R3
	r.compileAndRun(t, ie64Instr(OP_ASR, 1, IE64_SIZE_Q, 0, 2, 3, 0))

	if r.cpu.regs[1] != negU64(-16) {
		t.Fatalf("R1 = 0x%X, want -16", r.cpu.regs[1])
	}
}

func TestAMD64_CLZ(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0x00010000 // bit 16 set → 15 leading zeros in 32-bit

	// CLZ R1, R2
	r.compileAndRun(t, ie64Instr(OP_CLZ, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] != 15 {
		t.Fatalf("R1 = %d, want 15", r.cpu.regs[1])
	}
}

func TestAMD64_CLZ_Zero(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0

	// CLZ R1, R2 — CLZ(0) = 32
	r.compileAndRun(t, ie64Instr(OP_CLZ, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] != 32 {
		t.Fatalf("R1 = %d, want 32", r.cpu.regs[1])
	}
}

func TestAMD64_CLZ_One(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0x80000000 // bit 31 set → 0 leading zeros

	r.compileAndRun(t, ie64Instr(OP_CLZ, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0", r.cpu.regs[1])
	}
}

func TestAMD64_SEXT(t *testing.T) {
	r := newJITTestRig(t)

	tests := []struct {
		name string
		size byte
		in   uint64
		want uint64
	}{
		{"byte", IE64_SIZE_B, 0x80, 0xFFFFFFFFFFFFFF80},
		{"word", IE64_SIZE_W, 0x8000, 0xFFFFFFFFFFFF8000},
		{"long", IE64_SIZE_L, 0x80000000, 0xFFFFFFFF80000000},
		{"quad", IE64_SIZE_Q, 0x8000000000000000, 0x8000000000000000},
	}

	for _, tt := range tests {
		r.cpu.regs[1] = 0
		r.cpu.regs[2] = tt.in
		r.compileAndRun(t, ie64Instr(OP_SEXT, 1, tt.size, 0, 2, 0, 0))
		if r.cpu.regs[1] != tt.want {
			t.Fatalf("SEXT.%s R1 = 0x%X, want 0x%X", tt.name, r.cpu.regs[1], tt.want)
		}
	}
}

func TestAMD64_Rotate(t *testing.T) {
	r := newJITTestRig(t)

	tests := []struct {
		name   string
		opcode byte
		size   byte
		xbit   byte
		in     uint64
		count  uint64
		want   uint64
	}{
		{"rol_byte_wrap_imm", OP_ROL, IE64_SIZE_B, 1, 0x81, 9, 0x03},
		{"ror_word_imm", OP_ROR, IE64_SIZE_W, 1, 0x1234, 4, 0x4123},
		{"rol_long_reg", OP_ROL, IE64_SIZE_L, 0, 0x80000001, 1, 0x00000003},
		{"ror_quad_reg", OP_ROR, IE64_SIZE_Q, 0, 1, 1, 0x8000000000000000},
	}

	for _, tt := range tests {
		r.cpu.regs[1] = 0
		r.cpu.regs[2] = tt.in
		r.cpu.regs[3] = tt.count
		imm := uint32(0)
		rt := byte(3)
		if tt.xbit == 1 {
			imm = uint32(tt.count)
			rt = 0
		}
		r.compileAndRun(t, ie64Instr(tt.opcode, 1, tt.size, tt.xbit, 2, rt, imm))
		if r.cpu.regs[1] != tt.want {
			t.Fatalf("%s R1 = 0x%X, want 0x%X", tt.name, r.cpu.regs[1], tt.want)
		}
	}
}

func TestAMD64_CTZ_POPCNT_BSWAP(t *testing.T) {
	r := newJITTestRig(t)

	r.cpu.regs[2] = 0
	r.compileAndRun(t, ie64Instr(OP_CTZ, 1, IE64_SIZE_Q, 0, 2, 0, 0))
	if r.cpu.regs[1] != 32 {
		t.Fatalf("CTZ zero R1 = %d, want 32", r.cpu.regs[1])
	}

	r.cpu.regs[1] = 0
	r.cpu.regs[2] = 0x00100000
	r.compileAndRun(t, ie64Instr(OP_CTZ, 1, IE64_SIZE_Q, 0, 2, 0, 0))
	if r.cpu.regs[1] != 20 {
		t.Fatalf("CTZ R1 = %d, want 20", r.cpu.regs[1])
	}

	r.cpu.regs[1] = 0
	r.cpu.regs[2] = 0xF0F0F00F
	r.compileAndRun(t, ie64Instr(OP_POPCNT, 1, IE64_SIZE_Q, 0, 2, 0, 0))
	if r.cpu.regs[1] != 16 {
		t.Fatalf("POPCNT R1 = %d, want 16", r.cpu.regs[1])
	}

	r.cpu.regs[1] = 0
	r.cpu.regs[2] = 0xAABBCCDD
	r.compileAndRun(t, ie64Instr(OP_BSWAP, 1, IE64_SIZE_Q, 0, 2, 0, 0))
	if r.cpu.regs[1] != 0xDDCCBBAA {
		t.Fatalf("BSWAP R1 = 0x%X, want 0xDDCCBBAA", r.cpu.regs[1])
	}
}

// ===========================================================================
// Spilled Register Tests
// ===========================================================================

func TestAMD64_SpilledReg(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[16] = 100
	r.cpu.regs[17] = 42

	// ADD.Q R18, R16, R17 — all three are spilled (R16-R30)
	r.compileAndRun(t, ie64Instr(OP_ADD, 18, IE64_SIZE_Q, 0, 16, 17, 0))

	if r.cpu.regs[18] != 142 {
		t.Fatalf("R18 = %d, want 142", r.cpu.regs[18])
	}
}

func TestAMD64_SpilledDst_MappedSrc(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[1] = 100 // mapped (RBX)
	r.cpu.regs[2] = 42  // mapped (RBP)

	// ADD.Q R20, R1, R2 — spilled destination, mapped sources
	r.compileAndRun(t, ie64Instr(OP_ADD, 20, IE64_SIZE_Q, 0, 1, 2, 0))

	if r.cpu.regs[20] != 142 {
		t.Fatalf("R20 = %d, want 142", r.cpu.regs[20])
	}
}

// ===========================================================================
// Multi-Instruction Block Tests
// ===========================================================================

func TestAMD64_MultiALU(t *testing.T) {
	r := newJITTestRig(t)

	// R1 = 10, R1 = R1 + 5, R2 = R1 + 20 → R1=15, R2=35
	r.compileAndRun(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 10),
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 5),
		ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 1, 0, 20),
	)

	if r.cpu.regs[1] != 15 {
		t.Fatalf("R1 = %d, want 15", r.cpu.regs[1])
	}
	if r.cpu.regs[2] != 35 {
		t.Fatalf("R2 = %d, want 35", r.cpu.regs[2])
	}
}

// ===========================================================================
// NOP Test
// ===========================================================================

func TestAMD64_NOP(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[1] = 42

	r.compileAndRun(t, ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))

	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42 (NOP should not change state)", r.cpu.regs[1])
	}
}

// ===========================================================================
// Memory Access Tests
// ===========================================================================

func TestAMD64_LOAD_Q_FastPath(t *testing.T) {
	r := newJITTestRig(t)

	// Store a known value in memory
	addr := uint32(PROG_START + 0x100)
	binary.LittleEndian.PutUint64(r.cpu.memory[addr:], 0xDEADBEEFCAFEBABE)
	r.cpu.regs[2] = uint64(addr)

	// LOAD.Q R1, 0(R2)
	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.cpu.regs[1] != 0xDEADBEEFCAFEBABE {
		t.Fatalf("R1 = 0x%X, want 0xDEADBEEFCAFEBABE", r.cpu.regs[1])
	}
}

func TestAMD64_LOAD_B(t *testing.T) {
	r := newJITTestRig(t)

	addr := uint32(PROG_START + 0x100)
	r.cpu.memory[addr] = 0xAB
	r.cpu.regs[2] = uint64(addr)

	// LOAD.B R1, 0(R2)
	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_B, 0, 2, 0, 0))

	if r.cpu.regs[1] != 0xAB {
		t.Fatalf("R1 = 0x%X, want 0xAB", r.cpu.regs[1])
	}
}

func TestAMD64_LOAD_W(t *testing.T) {
	r := newJITTestRig(t)

	addr := uint32(PROG_START + 0x100)
	binary.LittleEndian.PutUint16(r.cpu.memory[addr:], 0xBEEF)
	r.cpu.regs[2] = uint64(addr)

	// LOAD.W R1, 0(R2)
	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_W, 0, 2, 0, 0))

	if r.cpu.regs[1] != 0xBEEF {
		t.Fatalf("R1 = 0x%X, want 0xBEEF", r.cpu.regs[1])
	}
}

func TestAMD64_LOAD_L(t *testing.T) {
	r := newJITTestRig(t)

	addr := uint32(PROG_START + 0x100)
	binary.LittleEndian.PutUint32(r.cpu.memory[addr:], 0xDEADBEEF)
	r.cpu.regs[2] = uint64(addr)

	// LOAD.L R1, 0(R2)
	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_L, 0, 2, 0, 0))

	if r.cpu.regs[1] != 0xDEADBEEF {
		t.Fatalf("R1 = 0x%X, want 0xDEADBEEF", r.cpu.regs[1])
	}
}

func TestAMD64_LOAD_Displacement(t *testing.T) {
	r := newJITTestRig(t)

	addr := uint32(PROG_START + 0x100)
	binary.LittleEndian.PutUint64(r.cpu.memory[addr+16:], 0x1234567890ABCDEF)
	r.cpu.regs[2] = uint64(addr)

	// LOAD.Q R1, 16(R2)
	r.compileAndRun(t, ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 2, 0, 16))

	if r.cpu.regs[1] != 0x1234567890ABCDEF {
		t.Fatalf("R1 = 0x%X, want 0x1234567890ABCDEF", r.cpu.regs[1])
	}
}

func TestAMD64_STORE_Q(t *testing.T) {
	r := newJITTestRig(t)

	addr := uint32(PROG_START + 0x100)
	r.cpu.regs[1] = 0xDEADBEEFCAFEBABE
	r.cpu.regs[2] = uint64(addr)

	// STORE.Q R1, 0(R2)
	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	got := binary.LittleEndian.Uint64(r.cpu.memory[addr:])
	if got != 0xDEADBEEFCAFEBABE {
		t.Fatalf("mem = 0x%X, want 0xDEADBEEFCAFEBABE", got)
	}
}

func TestAMD64_STORE_B(t *testing.T) {
	r := newJITTestRig(t)

	addr := uint32(PROG_START + 0x100)
	r.cpu.regs[1] = 0xAB
	r.cpu.regs[2] = uint64(addr)

	// STORE.B R1, 0(R2)
	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_B, 0, 2, 0, 0))

	if r.cpu.memory[addr] != 0xAB {
		t.Fatalf("mem = 0x%X, want 0xAB", r.cpu.memory[addr])
	}
}

// ===========================================================================
// Branch Tests
// ===========================================================================

func TestAMD64_BRA(t *testing.T) {
	r := newJITTestRig(t)

	// MOVE R1, #42; BRA +16 (skip over the next 2 instructions)
	r.compileAndRun(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42),
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 16), // BRA target = instrPC + 16
	)

	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}
	// PC should be at BRA target
	expected := uint64(PROG_START + 8 + 16) // BRA is at PROG_START+8, target = +16
	if r.cpu.PC != expected {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, expected)
	}
}

func TestAMD64_BEQ_Taken(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 42

	// BEQ R2, R3, +16 (should be taken since R2 == R3)
	r.compileAndRun(t, ie64Instr(OP_BEQ, 0, 0, 0, 2, 3, 16))

	expected := uint64(PROG_START + 16)
	if r.cpu.PC != expected {
		t.Fatalf("PC = 0x%X, want 0x%X (branch taken)", r.cpu.PC, expected)
	}
}

func TestAMD64_BEQ_NotTaken(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 99

	// MOVE R1, #10; BEQ R2, R3, +16; MOVE R1, #20
	r.compileAndRun(t,
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 10),
		ie64Instr(OP_BEQ, 0, 0, 0, 2, 3, 16),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 20),
	)

	// Not taken, so the MOVE R1, #20 should execute
	if r.cpu.regs[1] != 20 {
		t.Fatalf("R1 = %d, want 20 (branch not taken)", r.cpu.regs[1])
	}
}

func TestAMD64_BNE(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42
	r.cpu.regs[3] = 99

	// BNE R2, R3, +16 (should be taken since R2 != R3)
	r.compileAndRun(t, ie64Instr(OP_BNE, 0, 0, 0, 2, 3, 16))

	expected := uint64(PROG_START + 16)
	if r.cpu.PC != expected {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, expected)
	}
}

func TestAMD64_BLT(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = negU64(-10) // -10 (signed)
	r.cpu.regs[3] = 5

	// BLT R2, R3, +16 (should be taken since -10 < 5)
	r.compileAndRun(t, ie64Instr(OP_BLT, 0, 0, 0, 2, 3, 16))

	expected := uint64(PROG_START + 16)
	if r.cpu.PC != expected {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, expected)
	}
}

func TestAMD64_BGE(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 10
	r.cpu.regs[3] = 10

	// BGE R2, R3, +16 (should be taken since 10 >= 10)
	r.compileAndRun(t, ie64Instr(OP_BGE, 0, 0, 0, 2, 3, 16))

	expected := uint64(PROG_START + 16)
	if r.cpu.PC != expected {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, expected)
	}
}

func TestAMD64_BHI(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 100
	r.cpu.regs[3] = 50

	// BHI R2, R3, +16 (unsigned above: 100 > 50)
	r.compileAndRun(t, ie64Instr(OP_BHI, 0, 0, 0, 2, 3, 16))

	expected := uint64(PROG_START + 16)
	if r.cpu.PC != expected {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, expected)
	}
}

func TestAMD64_BLS(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 50
	r.cpu.regs[3] = 100

	// BLS R2, R3, +16 (unsigned below-or-same: 50 <= 100)
	r.compileAndRun(t, ie64Instr(OP_BLS, 0, 0, 0, 2, 3, 16))

	expected := uint64(PROG_START + 16)
	if r.cpu.PC != expected {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, expected)
	}
}

// ===========================================================================
// Subroutine / Stack Tests
// ===========================================================================

func TestAMD64_PUSH_POP(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[31] = STACK_START // initialize SP
	r.cpu.regs[2] = 0xDEADBEEFCAFEBABE

	// PUSH R2; POP R1
	r.compileAndRun(t,
		ie64Instr(OP_PUSH64, 0, 0, 0, 2, 0, 0),
		ie64Instr(OP_POP64, 1, 0, 0, 0, 0, 0),
	)

	if r.cpu.regs[1] != 0xDEADBEEFCAFEBABE {
		t.Fatalf("R1 = 0x%X, want 0xDEADBEEFCAFEBABE", r.cpu.regs[1])
	}
	// SP should be restored
	if r.cpu.regs[31] != STACK_START {
		t.Fatalf("SP = 0x%X, want 0x%X", r.cpu.regs[31], uint64(STACK_START))
	}
}

func TestAMD64_JSR_RTS(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[31] = STACK_START

	// JSR #16 — pushes return addr, jumps forward
	offset := uint32(PROG_START)
	jsr := ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 16)
	copy(r.cpu.memory[offset:], jsr)

	instrs := scanBlock(r.cpu.memory, PROG_START)
	if len(instrs) == 0 {
		t.Fatal("no instructions")
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

	// PC should be at JSR target
	expected := uint64(PROG_START + 16)
	if r.cpu.PC != expected {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, expected)
	}

	// SP should have been decremented by 8
	if r.cpu.regs[31] != STACK_START-8 {
		t.Fatalf("SP = 0x%X, want 0x%X", r.cpu.regs[31], uint64(STACK_START-8))
	}

	// Return address should be on the stack
	sp := uint32(r.cpu.regs[31])
	retAddr := binary.LittleEndian.Uint64(r.cpu.memory[sp:])
	expectedRet := uint64(PROG_START + IE64_INSTR_SIZE) // address after JSR
	if retAddr != expectedRet {
		t.Fatalf("return addr = 0x%X, want 0x%X", retAddr, expectedRet)
	}

	runtime.KeepAlive(r.ctx)
	runtime.KeepAlive(r.execMem)
}

func TestAMD64_JSR_PreservesSpilledRegsWrittenEarlierInBlock(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[31] = STACK_START
	// Seed POP.Q R29 from the stack so the block mirrors the DOS shell-launch
	// handoff: restore r29, derive spilled arguments, then JSR into a helper.
	binary.LittleEndian.PutUint64(r.cpu.memory[STACK_START:], 0x60A000)

	instrs := [][]byte{
		ie64Instr(OP_POP64, 29, IE64_SIZE_Q, 0, 0, 0, 0),    // R29 = 0x60A000
		ie64Instr(OP_ADD, 23, IE64_SIZE_Q, 1, 29, 0, 0x4CC), // R23 = R29 + 0x4CC
		ie64Instr(OP_ADD, 16, IE64_SIZE_Q, 1, 29, 0, 0x4DF), // R16 = R29 + 0x4DF
		ie64Instr(OP_MOVE, 18, IE64_SIZE_Q, 1, 0, 0, 0),     // R18 = 0
		ie64Instr(OP_MOVE, 30, IE64_SIZE_Q, 0, 29, 0, 0),    // R30 = R29
		ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 16),              // jump to helper
	}

	r.compileAndRun(t, instrs...)

	if r.cpu.PC != PROG_START+0x38 {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, uint64(PROG_START+0x38))
	}
	if r.cpu.regs[23] != 0x60A4CC {
		t.Fatalf("R23 = 0x%X, want 0x60A4CC", r.cpu.regs[23])
	}
	if r.cpu.regs[29] != 0x60A000 {
		t.Fatalf("R29 = 0x%X, want 0x60A000", r.cpu.regs[29])
	}
	if r.cpu.regs[16] != 0x60A4DF {
		t.Fatalf("R16 = 0x%X, want 0x60A4DF", r.cpu.regs[16])
	}
	if r.cpu.regs[30] != 0x60A000 {
		t.Fatalf("R30 = 0x%X, want 0x60A000", r.cpu.regs[30])
	}
}

func TestAMD64_MMUJsrBail_PreservesSpilledRegsWrittenEarlierInBlock(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[29] = 0x60A000

	offset := uint32(PROG_START)
	prog := [][]byte{
		ie64Instr(OP_ADD, 23, IE64_SIZE_Q, 1, 29, 0, 0x4CC), // R23 = R29 + 0x4CC
		ie64Instr(OP_ADD, 16, IE64_SIZE_Q, 1, 29, 0, 0x4DF), // R16 = R29 + 0x4DF
		ie64Instr(OP_MOVE, 18, IE64_SIZE_Q, 1, 0, 0, 0),     // R18 = 0
		ie64Instr(OP_MOVE, 30, IE64_SIZE_Q, 0, 29, 0, 0),    // R30 = R29
		ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 16),              // must bail under MMU
	}
	for _, instr := range prog {
		copy(r.cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}

	instrs := scanBlock(r.cpu.memory, PROG_START)
	if len(instrs) == 0 {
		t.Fatal("scanBlock returned 0 instructions")
	}

	r.execMem.Reset()
	block, err := compileBlockMMU(instrs, PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlockMMU: %v", err)
	}

	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&r.cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	// Phase 5: JSR no longer carries a compile-time mmuBail; it helper-exits
	// at runtime when MMUEnabled is set. The helper exit flushes the same
	// earlier-written registers via emitEpilogue and reports the JSR's own PC
	// (RetPC) so the dispatcher re-executes it through the MMU-aware helper.
	r.ctx.MMUEnabled = 1
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	r.cpu.PC = r.ctx.RetPC
	r.ctx.RetPC = 0

	// Exit PC should be the JSR itself so the helper can service it.
	if r.cpu.PC != PROG_START+32 {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, uint64(PROG_START+32))
	}
	if r.cpu.regs[23] != 0x60A4CC {
		t.Fatalf("R23 = 0x%X, want 0x60A4CC", r.cpu.regs[23])
	}
	if r.cpu.regs[16] != 0x60A4DF {
		t.Fatalf("R16 = 0x%X, want 0x60A4DF", r.cpu.regs[16])
	}
	if r.cpu.regs[18] != 0 {
		t.Fatalf("R18 = 0x%X, want 0", r.cpu.regs[18])
	}
	if r.cpu.regs[30] != 0x60A000 {
		t.Fatalf("R30 = 0x%X, want 0x60A000", r.cpu.regs[30])
	}
}

func TestAMD64_MMUPushBail_PreservesSpilledRegsWrittenEarlierInBlock(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[1] = 0x60A4CC
	r.cpu.regs[29] = 0x60A000
	r.cpu.regs[31] = STACK_START

	offset := uint32(PROG_START)
	prog := [][]byte{
		ie64Instr(OP_MOVE, 20, IE64_SIZE_Q, 0, 1, 0, 0),   // R20 = R1
		ie64Instr(OP_PUSH64, 0, IE64_SIZE_Q, 0, 29, 0, 0), // bail under MMU
	}
	for _, instr := range prog {
		copy(r.cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}

	instrs := scanBlock(r.cpu.memory, PROG_START)
	if len(instrs) == 0 {
		t.Fatal("scanBlock returned 0 instructions")
	}

	r.execMem.Reset()
	block, err := compileBlockMMU(instrs, PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlockMMU: %v", err)
	}

	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&r.cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	// Phase 5: PUSH helper-exits at runtime under MMU; emitEpilogue flushes
	// earlier-written regs and RetPC reports the PUSH's own PC.
	r.ctx.MMUEnabled = 1
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	r.cpu.PC = r.ctx.RetPC
	r.ctx.RetPC = 0

	if r.cpu.PC != PROG_START+8 {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, uint64(PROG_START+8))
	}
	if r.cpu.regs[20] != 0x60A4CC {
		t.Fatalf("R20 = 0x%X, want 0x60A4CC", r.cpu.regs[20])
	}
}

func TestAMD64_MMUJsrBail_PreservesMappedArgWrittenFromSpilledSource(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[23] = 0x60A4CC

	offset := uint32(PROG_START)
	prog := [][]byte{
		ie64Instr(OP_MOVE, 24, IE64_SIZE_Q, 0, 23, 0, 0), // R24 = R23
		ie64Instr(OP_MOVE, 27, IE64_SIZE_Q, 0, 24, 0, 0), // R27 = R24
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 24, 0, 0),  // R1 = R24
		ie64Instr(OP_JSR64, 0, 0, 0, 0, 0, 16),           // bail under MMU
	}
	for _, instr := range prog {
		copy(r.cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}

	instrs := scanBlock(r.cpu.memory, PROG_START)
	if len(instrs) == 0 {
		t.Fatal("scanBlock returned 0 instructions")
	}

	r.execMem.Reset()
	block, err := compileBlockMMU(instrs, PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlockMMU: %v", err)
	}

	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&r.cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	// Phase 5: JSR helper-exits at runtime under MMU; emitEpilogue flushes
	// the mapped arg written from a spilled source, RetPC reports the JSR PC.
	r.ctx.MMUEnabled = 1
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	r.cpu.PC = r.ctx.RetPC
	r.ctx.RetPC = 0

	if r.cpu.PC != PROG_START+24 {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, uint64(PROG_START+24))
	}
	if r.cpu.regs[1] != 0x60A4CC {
		t.Fatalf("R1 = 0x%X, want 0x60A4CC", r.cpu.regs[1])
	}
}

func TestAMD64_MMUJsrBail_FlushesMappedR1WrittenEarlierInBlock(t *testing.T) {
	r := newJITTestRig(t)
	setupIdentityMMU(r.cpu, 160)

	offset := uint32(PROG_START)
	prog := [][]byte{
		ie64Instr(OP_MOVE, 24, IE64_SIZE_L, 1, 0, 0, 0x60A4CC),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(3*IE64_INSTR_SIZE)),
	}
	for _, instr := range prog {
		copy(r.cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}

	instrs := scanBlock(r.cpu.memory, PROG_START)
	if len(instrs) == 0 {
		t.Fatal("scanBlock returned 0 instructions")
	}

	r.execMem.Reset()
	block, err := compileBlockMMU(instrs, PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlockMMU: %v", err)
	}

	// setupIdentityMMU already enabled the MMU on the CPU; mirror it into the
	// JITContext so the JSR takes the Phase 5 helper-exit path at runtime.
	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&r.cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	r.ctx.MMUEnabled = 1
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	r.cpu.PC = r.ctx.RetPC
	r.ctx.RetPC = 0

	if r.cpu.PC != PROG_START+16 {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, uint64(PROG_START+16))
	}
	if r.cpu.regs[1] != 0x60A4CC {
		t.Fatalf("R1 = 0x%X, want 0x60A4CC after JSR bail epilogue flush", r.cpu.regs[1])
	}
}

// ===========================================================================
// RTI / WAIT Mid-Block Tests
// ===========================================================================

func TestAMD64_RTI_InBlock(t *testing.T) {
	r := newJITTestRig(t)

	// Set up stack with a return address
	returnAddr := uint64(PROG_START + 0x200)
	r.cpu.regs[31] = STACK_START
	r.cpu.regs[31] -= 8
	sp := uint32(r.cpu.regs[31])
	binary.LittleEndian.PutUint64(r.cpu.memory[sp:], returnAddr)

	// Build block: MOVE R1, #42; RTI
	offset := uint32(PROG_START)
	move := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 42)
	rti := ie64Instr(OP_RTI64, 0, 0, 0, 0, 0, 0)
	copy(r.cpu.memory[offset:], move)
	copy(r.cpu.memory[offset+8:], rti)

	instrs := scanBlock(r.cpu.memory, PROG_START)
	if len(instrs) != 2 {
		t.Fatalf("scanBlock returned %d instructions, want 2", len(instrs))
	}

	r.execMem.Reset()
	block, err := compileBlock(instrs, PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlock: %v", err)
	}

	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&r.cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	// R1 should be 42 (MOVE executed before bail)
	if r.cpu.regs[1] != 42 {
		t.Fatalf("R1 = %d, want 42", r.cpu.regs[1])
	}

	// NeedIOFallback should be set (RTI bails)
	if r.ctx.NeedIOFallback == 0 {
		t.Fatal("NeedIOFallback should be set after RTI")
	}

	// PC should point at RTI instruction
	combined := r.cpu.regs[0]
	pc := uint32(combined)
	expectedPC := uint32(PROG_START + 8) // RTI is second instruction
	if pc != expectedPC {
		t.Fatalf("PC = 0x%X, want 0x%X", pc, expectedPC)
	}
	r.cpu.regs[0] = 0

	runtime.KeepAlive(r.ctx)
	runtime.KeepAlive(r.execMem)
}

func TestAMD64_WAIT_InBlock(t *testing.T) {
	r := newJITTestRig(t)

	offset := uint32(PROG_START)
	move := ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 99)
	wait := ie64Instr(OP_WAIT64, 0, 0, 0, 0, 0, 0)
	copy(r.cpu.memory[offset:], move)
	copy(r.cpu.memory[offset+8:], wait)

	instrs := scanBlock(r.cpu.memory, PROG_START)
	if len(instrs) != 2 {
		t.Fatalf("scanBlock returned %d instructions, want 2", len(instrs))
	}

	r.execMem.Reset()
	block, err := compileBlock(instrs, PROG_START, r.execMem)
	if err != nil {
		t.Fatalf("compileBlock: %v", err)
	}

	r.ctx.RegsPtr = uintptr(unsafe.Pointer(&r.cpu.regs[0]))
	r.ctx.MemPtr = uintptr(unsafe.Pointer(&r.cpu.memory[0]))
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	if r.cpu.regs[1] != 99 {
		t.Fatalf("R1 = %d, want 99", r.cpu.regs[1])
	}

	if r.ctx.NeedIOFallback == 0 {
		t.Fatal("NeedIOFallback should be set after WAIT")
	}

	combined := r.cpu.regs[0]
	pc := uint32(combined)
	expectedPC := uint32(PROG_START + 8)
	if pc != expectedPC {
		t.Fatalf("PC = 0x%X, want 0x%X", pc, expectedPC)
	}
	r.cpu.regs[0] = 0

	runtime.KeepAlive(r.ctx)
	runtime.KeepAlive(r.execMem)
}

// ===========================================================================
// FPU Tests — Category A (integer bitwise)
// ===========================================================================

func TestAMD64_FMOV(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x3F800000 // 1.0

	// FMOV F1, F2
	r.compileAndRun(t, ie64Instr(OP_FMOV, 1, 0, 0, 2, 0, 0))

	if r.cpu.FPU.FPRegs[1] != 0x3F800000 {
		t.Fatalf("F1 = 0x%08X, want 0x3F800000", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FABS(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0xBF800000 // -1.0

	// FABS F1, F2
	r.compileAndRun(t, ie64Instr(OP_FABS, 1, 0, 0, 2, 0, 0))

	if r.cpu.FPU.FPRegs[1] != 0x3F800000 { // 1.0
		t.Fatalf("F1 = 0x%08X, want 0x3F800000", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FNEG(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x3F800000 // 1.0

	// FNEG F1, F2
	r.compileAndRun(t, ie64Instr(OP_FNEG, 1, 0, 0, 2, 0, 0))

	if r.cpu.FPU.FPRegs[1] != 0xBF800000 { // -1.0
		t.Fatalf("F1 = 0x%08X, want 0xBF800000", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FMOVI(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0x3F800000 // 1.0 bit pattern

	// FMOVI F1, R2
	r.compileAndRun(t, ie64Instr(OP_FMOVI, 1, 0, 0, 2, 0, 0))

	if r.cpu.FPU.FPRegs[1] != 0x3F800000 {
		t.Fatalf("F1 = 0x%08X, want 0x3F800000", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FMOVO(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x3F800000

	// FMOVO R1, F2
	r.compileAndRun(t, ie64Instr(OP_FMOVO, 1, 0, 0, 2, 0, 0))

	if r.cpu.regs[1] != 0x3F800000 {
		t.Fatalf("R1 = 0x%X, want 0x3F800000", r.cpu.regs[1])
	}
}

func TestAMD64_FMOVECR(t *testing.T) {
	r := newJITTestRig(t)

	// FMOVECR F1, #8 (1.0)
	r.compileAndRun(t, ie64Instr(OP_FMOVECR, 1, 0, 0, 0, 0, 8))

	if r.cpu.FPU.FPRegs[1] != 0x3F800000 { // 1.0
		t.Fatalf("F1 = 0x%08X, want 0x3F800000 (1.0)", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FMOVSR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPSR = 0x0A000005 // some CC + exception bits

	// FMOVSR R1 (read FPSR into R1)
	r.compileAndRun(t, ie64Instr(OP_FMOVSR, 1, 0, 0, 0, 0, 0))

	if r.cpu.regs[1] != uint64(r.cpu.FPU.FPSR) {
		t.Fatalf("R1 = 0x%X, want 0x%X", r.cpu.regs[1], r.cpu.FPU.FPSR)
	}
}

func TestAMD64_FMOVCR(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPCR = 0x02 // floor rounding

	// FMOVCR R1 (read FPCR into R1)
	r.compileAndRun(t, ie64Instr(OP_FMOVCR, 1, 0, 0, 0, 0, 0))

	if r.cpu.regs[1] != 0x02 {
		t.Fatalf("R1 = 0x%X, want 0x02", r.cpu.regs[1])
	}
}

func TestAMD64_FMOVSC(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0x0F00000F // try to set all CC and exception bits

	// FMOVSC (write R2 to FPSR, masked)
	r.compileAndRun(t, ie64Instr(OP_FMOVSC, 0, 0, 0, 2, 0, 0))

	expected := uint32(0x0F00000F) & IE64_FPU_FPSR_MASK
	if r.cpu.FPU.FPSR != expected {
		t.Fatalf("FPSR = 0x%08X, want 0x%08X", r.cpu.FPU.FPSR, expected)
	}
}

func TestAMD64_FMOVCC(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0x03 // ceil rounding

	// FMOVCC (write R2 to FPCR)
	r.compileAndRun(t, ie64Instr(OP_FMOVCC, 0, 0, 0, 2, 0, 0))

	if r.cpu.FPU.FPCR != 0x03 {
		t.Fatalf("FPCR = 0x%X, want 0x03", r.cpu.FPU.FPCR)
	}
}

// ===========================================================================
// FPU Tests — Category B (native SSE)
// ===========================================================================

func TestAMD64_FADD(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x3F800000 // 1.0
	r.cpu.FPU.FPRegs[3] = 0x40000000 // 2.0

	// FADD F1, F2, F3 → 1.0 + 2.0 = 3.0
	r.compileAndRun(t, ie64Instr(OP_FADD, 1, 0, 0, 2, 3, 0))

	if r.cpu.FPU.FPRegs[1] != 0x40400000 { // 3.0
		t.Fatalf("F1 = 0x%08X, want 0x40400000 (3.0)", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FSUB(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x40400000 // 3.0
	r.cpu.FPU.FPRegs[3] = 0x3F800000 // 1.0

	// FSUB F1, F2, F3 → 3.0 - 1.0 = 2.0
	r.compileAndRun(t, ie64Instr(OP_FSUB, 1, 0, 0, 2, 3, 0))

	if r.cpu.FPU.FPRegs[1] != 0x40000000 { // 2.0
		t.Fatalf("F1 = 0x%08X, want 0x40000000 (2.0)", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FMUL(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x40400000 // 3.0
	r.cpu.FPU.FPRegs[3] = 0x40000000 // 2.0

	// FMUL F1, F2, F3 → 3.0 * 2.0 = 6.0
	r.compileAndRun(t, ie64Instr(OP_FMUL, 1, 0, 0, 2, 3, 0))

	if r.cpu.FPU.FPRegs[1] != 0x40C00000 { // 6.0
		t.Fatalf("F1 = 0x%08X, want 0x40C00000 (6.0)", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FDIV(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x40C00000 // 6.0
	r.cpu.FPU.FPRegs[3] = 0x40000000 // 2.0

	// FDIV F1, F2, F3 → 6.0 / 2.0 = 3.0
	r.compileAndRun(t, ie64Instr(OP_FDIV, 1, 0, 0, 2, 3, 0))

	if r.cpu.FPU.FPRegs[1] != 0x40400000 { // 3.0
		t.Fatalf("F1 = 0x%08X, want 0x40400000 (3.0)", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FSQRT(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x41100000 // 9.0

	// FSQRT F1, F2 → sqrt(9.0) = 3.0
	r.compileAndRun(t, ie64Instr(OP_FSQRT, 1, 0, 0, 2, 0, 0))

	if r.cpu.FPU.FPRegs[1] != 0x40400000 { // 3.0
		t.Fatalf("F1 = 0x%08X, want 0x40400000 (3.0)", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FCMP_GreaterThan(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x40000000 // 2.0
	r.cpu.FPU.FPRegs[3] = 0x3F800000 // 1.0

	// FCMP R1, F2, F3 → 2.0 > 1.0 → result = 1
	r.compileAndRun(t, ie64Instr(OP_FCMP, 1, 0, 0, 2, 3, 0))

	if r.cpu.regs[1] != 1 {
		t.Fatalf("R1 = %d, want 1 (greater than)", r.cpu.regs[1])
	}
}

func TestAMD64_FCMP_LessThan(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x3F800000 // 1.0
	r.cpu.FPU.FPRegs[3] = 0x40000000 // 2.0

	r.compileAndRun(t, ie64Instr(OP_FCMP, 1, 0, 0, 2, 3, 0))

	if r.cpu.regs[1] != negU64(-1) {
		t.Fatalf("R1 = 0x%X, want -1 (less than)", r.cpu.regs[1])
	}
}

func TestAMD64_FCMP_Equal(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.FPU.FPRegs[2] = 0x3F800000 // 1.0
	r.cpu.FPU.FPRegs[3] = 0x3F800000 // 1.0

	r.compileAndRun(t, ie64Instr(OP_FCMP, 1, 0, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = %d, want 0 (equal)", r.cpu.regs[1])
	}
}

func TestAMD64_FCVTIF(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 42

	// FCVTIF F1, R2 → F1 = float32(42) = 0x42280000
	r.compileAndRun(t, ie64Instr(OP_FCVTIF, 1, 0, 0, 2, 0, 0))

	if r.cpu.FPU.FPRegs[1] != 0x42280000 {
		t.Fatalf("F1 = 0x%08X, want 0x42280000 (42.0)", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FINT_RoundingModes(t *testing.T) {
	tests := []struct {
		name string
		fpcr uint32
		in   float32
		want float32
	}{
		{"nearest even", uint32(IE64_FPU_RND_NEAREST), 2.5, 2.0},
		{"truncate", uint32(IE64_FPU_RND_ZERO), -3.9, -3.0},
		{"floor", uint32(IE64_FPU_RND_FLOOR), -3.1, -4.0},
		{"ceil", uint32(IE64_FPU_RND_CEIL), 3.1, 4.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newJITTestRig(t)
			r.cpu.FPU.FPCR = tt.fpcr
			r.cpu.FPU.FPRegs[2] = math.Float32bits(tt.in)

			r.compileAndRun(t, ie64Instr(OP_FINT, 1, 0, 0, 2, 0, 0))

			if r.ctx.NeedIOFallback != 0 {
				t.Fatalf("NeedIOFallback = %d, want 0 (FINT should be native)", r.ctx.NeedIOFallback)
			}
			if got := math.Float32frombits(r.cpu.FPU.FPRegs[1]); got != tt.want {
				t.Fatalf("F1 = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAMD64_FCVTFI_NativeExceptionParity(t *testing.T) {
	tests := []struct {
		name     string
		bits     uint32
		rd       byte
		want     uint64
		wantIO   bool
		wantKeep uint64
	}{
		{"truncate positive", math.Float32bits(42.7), 1, 42, false, 0},
		{"truncate negative", math.Float32bits(-42.7), 1, negU64(-42), false, 0},
		{"nan to zero with invalid", 0x7FC00000, 1, 0, true, 0},
		{"large positive saturates", math.Float32bits(1e20), 1, uint64(math.MaxInt32), true, 0},
		{"large negative saturates", math.Float32bits(-1e20), 1, negU64(int64(math.MinInt32)), true, 0},
		{"rd zero still sets invalid", 0x7FC00000, 0, 0, true, 0xCAFE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newJITTestRig(t)
			r.cpu.FPU.FPRegs[2] = tt.bits
			r.cpu.regs[1] = tt.wantKeep

			r.compileAndRun(t, ie64Instr(OP_FCVTFI, tt.rd, 0, 0, 2, 0, 0))

			if r.ctx.NeedIOFallback != 0 {
				t.Fatalf("NeedIOFallback = %d, want 0 (FCVTFI should be native)", r.ctx.NeedIOFallback)
			}
			if tt.rd != 0 && r.cpu.regs[tt.rd] != tt.want {
				t.Fatalf("R%d = 0x%016X, want 0x%016X", tt.rd, r.cpu.regs[tt.rd], tt.want)
			}
			if tt.rd == 0 && r.cpu.regs[1] != tt.wantKeep {
				t.Fatalf("R1 = 0x%016X, want preserved 0x%016X", r.cpu.regs[1], tt.wantKeep)
			}
			gotIO := r.cpu.FPU.FPSR&IE64_FPU_EX_IO != 0
			if gotIO != tt.wantIO {
				t.Fatalf("FPSR IO flag = %v, want %v (FPSR=0x%08X)", gotIO, tt.wantIO, r.cpu.FPU.FPSR)
			}
		})
	}
}

func TestAMD64_FLOAD(t *testing.T) {
	r := newJITTestRig(t)
	addr := uint32(PROG_START + 0x100)
	binary.LittleEndian.PutUint32(r.cpu.memory[addr:], 0x3F800000) // 1.0
	r.cpu.regs[2] = uint64(addr)

	// FLOAD F1, 0(R2)
	r.compileAndRun(t, ie64Instr(OP_FLOAD, 1, 0, 0, 2, 0, 0))

	if r.cpu.FPU.FPRegs[1] != 0x3F800000 {
		t.Fatalf("F1 = 0x%08X, want 0x3F800000", r.cpu.FPU.FPRegs[1])
	}
}

func TestAMD64_FSTORE(t *testing.T) {
	r := newJITTestRig(t)
	addr := uint32(PROG_START + 0x100)
	r.cpu.FPU.FPRegs[1] = 0x40400000 // 3.0
	r.cpu.regs[2] = uint64(addr)

	// FSTORE F1, 0(R2)
	r.compileAndRun(t, ie64Instr(OP_FSTORE, 1, 0, 0, 2, 0, 0))

	got := binary.LittleEndian.Uint32(r.cpu.memory[addr:])
	if got != 0x40400000 {
		t.Fatalf("mem = 0x%08X, want 0x40400000", got)
	}
}

func TestAMD64_AtomicLowRAMNative(t *testing.T) {
	tests := []struct {
		name    string
		op      byte
		initial uint64
		rd      uint64
		rt      uint64
		wantMem uint64
		wantRd  uint64
	}{
		{"cas success", OP_CAS, 0x11, 0x11, 0x22, 0x22, 0x11},
		{"cas fail", OP_CAS, 0x33, 0x11, 0x22, 0x33, 0x33},
		{"xchg", OP_XCHG, 0x11, 0, 0x44, 0x44, 0x11},
		{"faa", OP_FAA, 5, 0, 7, 12, 5},
		{"fand", OP_FAND, 0xF0F0, 0, 0x0FF0, 0x00F0, 0xF0F0},
		{"for", OP_FOR, 0x1000, 0, 0x0001, 0x1001, 0x1000},
		{"fxor", OP_FXOR, 0xAAAA, 0, 0x0F0F, 0xA5A5, 0xAAAA},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newJITTestRig(t)
			addr := uint32(PROG_START + 0x800)
			binary.LittleEndian.PutUint64(r.cpu.memory[addr:], tt.initial)
			r.cpu.regs[1] = tt.rd
			r.cpu.regs[2] = uint64(addr)
			r.cpu.regs[3] = tt.rt

			r.compileAndRun(t, ie64Instr(tt.op, 1, IE64_SIZE_Q, 0, 2, 3, 0))

			if r.ctx.NeedIOFallback != 0 {
				t.Fatalf("NeedIOFallback = %d, want 0 (atomic should be native)", r.ctx.NeedIOFallback)
			}
			if got := binary.LittleEndian.Uint64(r.cpu.memory[addr:]); got != tt.wantMem {
				t.Fatalf("mem = 0x%016X, want 0x%016X", got, tt.wantMem)
			}
			if r.cpu.regs[1] != tt.wantRd {
				t.Fatalf("R1 = 0x%016X, want old value 0x%016X", r.cpu.regs[1], tt.wantRd)
			}
		})
	}
}

// ===========================================================================
// Missing Branch Tests
// ===========================================================================

// ===========================================================================
// Edge-case tests for .L size correctness
// ===========================================================================

func TestAMD64_DIVU_SizeL_HighBits(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0x100000000 // 4GB (high 32 bits set)
	r.cpu.regs[3] = 2

	// DIVU.L R1, R2, R3 → 0x100000000 / 2 = 0x80000000, masked to 32-bit = 0x80000000
	r.compileAndRun(t, ie64Instr(OP_DIVU, 1, IE64_SIZE_L, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0x80000000 {
		t.Fatalf("R1 = 0x%X, want 0x80000000", r.cpu.regs[1])
	}
}

func TestAMD64_LSR_SizeL_LargeCount(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 0xFF0000000000 // 0xFF << 40
	r.cpu.regs[3] = 40             // shift count > 31

	// LSR.L R1, R2, R3 → 0xFF0000000000 >> 40 = 0xFF, masked to .L = 0xFF
	r.compileAndRun(t, ie64Instr(OP_LSR, 1, IE64_SIZE_L, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0xFF {
		t.Fatalf("R1 = 0x%X, want 0xFF", r.cpu.regs[1])
	}
}

func TestAMD64_LSL_SizeL_LargeCount(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 1
	r.cpu.regs[3] = 33 // shift count > 31

	// LSL.L R1, R2, R3 → 1 << 33 = 0x200000000, masked to .L = 0
	r.compileAndRun(t, ie64Instr(OP_LSL, 1, IE64_SIZE_L, 0, 2, 3, 0))

	if r.cpu.regs[1] != 0 {
		t.Fatalf("R1 = 0x%X, want 0 (1<<33 masked to 32-bit)", r.cpu.regs[1])
	}
}

func TestAMD64_BGT(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 10
	r.cpu.regs[3] = 5

	// BGT R2, R3, +16 (10 > 5 signed)
	r.compileAndRun(t, ie64Instr(OP_BGT, 0, 0, 0, 2, 3, 16))

	expected := uint64(PROG_START + 16)
	if r.cpu.PC != expected {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, expected)
	}
}

func TestAMD64_BLE(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[2] = 5
	r.cpu.regs[3] = 10

	// BLE R2, R3, +16 (5 <= 10 signed)
	r.compileAndRun(t, ie64Instr(OP_BLE, 0, 0, 0, 2, 3, 16))

	expected := uint64(PROG_START + 16)
	if r.cpu.PC != expected {
		t.Fatalf("PC = 0x%X, want 0x%X", r.cpu.PC, expected)
	}
}

// ===========================================================================
// Backward Branch Tests
// ===========================================================================

func TestAMD64_BackwardBranch_Simple(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[1] = 0
	r.cpu.regs[2] = 5

	// Simple loop: R1 += 1; R2 -= 1; BNE R2, R0, -16 (back to start)
	// BNE is at PROG_START+16, target = PROG_START+16 + (-16) = PROG_START
	// Loop runs 5 times: R1 = 5, R2 = 0
	r.compileAndRun(t,
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 1), // R1 += 1
		ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1), // R2 -= 1
		ie64Instr(OP_BNE, 0, 0, 0, 2, 0, 0xFFFFFFF0),  // BNE R2, R0, -16 (back to instr 0)
	)

	// BNE is a block terminator, so the block ends there.
	// The backward branch runs natively until R2==0 (not taken) or budget exhausted.
	if r.cpu.regs[1] != 5 {
		t.Fatalf("R1 = %d, want 5", r.cpu.regs[1])
	}
	if r.cpu.regs[2] != 0 {
		t.Fatalf("R2 = %d, want 0", r.cpu.regs[2])
	}
}

func TestAMD64_BackwardBranch_BRA(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[1] = 0
	r.cpu.regs[2] = 3

	// R1 += 1; R2 -= 1; BEQ R2, R0, +8; BRA -24 (back to start)
	// BEQ at offset 16 exits when R2==0 (+8 skips BRA)
	// BRA at offset 24: target = 24 + (-24) = 0 (PROG_START relative)
	r.compileAndRun(t,
		ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 1, 0, 1),
		ie64Instr(OP_SUB, 2, IE64_SIZE_Q, 1, 2, 0, 1),
		ie64Instr(OP_BEQ, 0, 0, 0, 2, 0, 8),          // BEQ R2, R0, +8 (skip BRA)
		ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 0xFFFFFFE8), // BRA -24 (back to instr 0)
	)

	if r.cpu.regs[1] != 3 {
		t.Fatalf("R1 = %d, want 3", r.cpu.regs[1])
	}
}

func TestAMD64_RTI_RegisterLiveness(t *testing.T) {
	// Pure analyzeBlockRegs test — no code emission needed
	instrs := []JITInstr{
		{opcode: OP_MOVE, rd: 1, size: IE64_SIZE_Q, xbit: 1, imm32: 42},
		{opcode: OP_RTI64, pcOffset: 8},
	}
	br := analyzeBlockRegs(instrs)

	if br.read&(1<<31) == 0 {
		t.Fatal("RTI should mark R31 as read")
	}
	if br.written&(1<<31) == 0 {
		t.Fatal("RTI should mark R31 as written")
	}
	w := instrWrittenRegs(&instrs[1])
	if w&(1<<31) == 0 {
		t.Fatal("instrWrittenRegs for RTI should include R31")
	}
}

// ===========================================================================
// JIT vs Interpreter Parity Tests
// ===========================================================================

func TestJIT_vs_Interpreter_ALU(t *testing.T) {
	programs := []struct {
		name   string
		instrs [][]byte
		setup  func(cpu *CPU64)
	}{
		{
			name: "ADD_imm",
			instrs: [][]byte{
				ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 100),
				ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 1, 0, 50),
			},
		},
		{
			name: "SUB_reg",
			instrs: [][]byte{
				ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 100),
				ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 42),
				ie64Instr(OP_SUB, 3, IE64_SIZE_Q, 0, 1, 2, 0),
			},
		},
		{
			name: "MULU_DIVU",
			instrs: [][]byte{
				ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 7),
				ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 6),
				ie64Instr(OP_MULU, 3, IE64_SIZE_Q, 0, 1, 2, 0),
				ie64Instr(OP_DIVU, 4, IE64_SIZE_Q, 0, 3, 1, 0),
			},
		},
	}

	for _, prog := range programs {
		t.Run(prog.name, func(t *testing.T) {
			// Run via JIT
			jitRig := newJITTestRig(t)
			if prog.setup != nil {
				prog.setup(jitRig.cpu)
			}
			jitRig.compileAndRun(t, prog.instrs...)

			// Run via interpreter
			interpBus := NewMachineBus()
			interpCPU := NewCPU64(interpBus)
			if prog.setup != nil {
				prog.setup(interpCPU)
			}
			offset := uint32(PROG_START)
			for _, instr := range prog.instrs {
				copy(interpCPU.memory[offset:], instr)
				offset += uint32(len(instr))
			}
			copy(interpCPU.memory[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
			interpCPU.PC = PROG_START
			interpCPU.running.Store(true)
			for i := 0; i < 1000; i++ {
				if interpCPU.memory[uint32(interpCPU.PC)] == OP_HALT64 {
					break
				}
				interpCPU.StepOne()
			}

			// Compare registers R1-R31
			for i := 1; i <= 31; i++ {
				if jitRig.cpu.regs[i] != interpCPU.regs[i] {
					t.Errorf("R%d: JIT=0x%X, interp=0x%X", i, jitRig.cpu.regs[i], interpCPU.regs[i])
				}
			}
		})
	}
}

// Ensure ie64Instr is available (defined in cpu_ie64_test.go)
var _ = ie64Instr
var _ = binary.LittleEndian
