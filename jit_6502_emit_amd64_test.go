// jit_6502_emit_amd64_test.go - x86-64 emitter unit tests for 6502 JIT

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"unsafe"
)

// ===========================================================================
// Test Rig
// ===========================================================================

// jit6502TestRig creates a CPU + JIT context for emitter tests.
type jit6502TestRig struct {
	bus     *MachineBus
	cpu     *CPU_6502
	execMem *ExecMem
	ctx     *JIT6502Context
}

func newJIT6502TestRig(t *testing.T) *jit6502TestRig {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewCPU_6502(bus)
	cpu.SetRDYLine(true)

	execMem, err := AllocExecMem(1024 * 1024) // 1MB for tests
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}

	ctx := newJIT6502Context(cpu)
	if ctx == nil {
		t.Fatal("newJIT6502Context returned nil")
	}

	return &jit6502TestRig{
		bus:     bus,
		cpu:     cpu,
		execMem: execMem,
		ctx:     ctx,
	}
}

func (r *jit6502TestRig) cleanup() {
	if r.execMem != nil {
		r.execMem.Free()
	}
}

// compileAndRun compiles a 6502 program placed at startPC and executes it via JIT.
func (r *jit6502TestRig) compileAndRun(t *testing.T, program []byte, startPC uint16) {
	t.Helper()

	// Place program in memory
	for i, b := range program {
		r.bus.Write8(uint32(startPC)+uint32(i), b)
	}

	// Refresh context pointers (memDirect may have changed)
	r.ctx = newJIT6502Context(r.cpu)

	// Get memDirect for scanner
	mem := r.cpu.fastAdapter.memDirect

	// Scan block
	instrs := jit6502ScanBlock(mem, startPC, len(mem))
	if len(instrs) == 0 {
		t.Fatal("scanner returned empty block")
	}

	// Compile block
	block, err := compileBlock6502(instrs, startPC, r.execMem, &r.cpu.codePageBitmap)
	if err != nil {
		t.Fatalf("compileBlock6502: %v", err)
	}

	// Execute via callNative
	callNative(block.execAddr, uintptr(unsafe.Pointer(r.ctx)))

	// Read results back from context
	r.cpu.PC = uint16(r.ctx.RetPC)
}

// ===========================================================================
// Prologue/Epilogue Tests
// ===========================================================================

func TestJIT6502_AMD64_NOP_RegisterRoundTrip(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	// Set known register values
	rig.cpu.A = 0x42
	rig.cpu.X = 0x33
	rig.cpu.Y = 0x77
	rig.cpu.SP = 0xFD
	rig.cpu.PC = 0x0600
	rig.cpu.SR = 0xA5

	// Program: NOP + BRK (BRK terminates the block)
	rig.compileAndRun(t, []byte{0xEA, 0x00}, 0x0600)

	// Verify registers are preserved through prologue/epilogue
	if rig.cpu.A != 0x42 {
		t.Errorf("A = 0x%02X, want 0x42", rig.cpu.A)
	}
	if rig.cpu.X != 0x33 {
		t.Errorf("X = 0x%02X, want 0x33", rig.cpu.X)
	}
	if rig.cpu.Y != 0x77 {
		t.Errorf("Y = 0x%02X, want 0x77", rig.cpu.Y)
	}
	if rig.cpu.SP != 0xFD {
		t.Errorf("SP = 0x%02X, want 0xFD", rig.cpu.SP)
	}
	if rig.cpu.SR != 0xA5 {
		t.Errorf("SR = 0x%02X, want 0xA5", rig.cpu.SR)
	}
}

func TestJIT6502_AMD64_NOP_RetPC(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600

	// NOP (1 byte) — block scanner will also include BRK as terminator
	// But NOP alone won't terminate the block. Let's just use a single NOP
	// followed by a BRK to end the block.
	rig.compileAndRun(t, []byte{0xEA, 0x00}, 0x0600)

	// NOP is at 0x0600 (1 byte), BRK at 0x0601 terminates block.
	// The scanner includes BRK in the block. Since BRK bails to interpreter
	// (needsFallback), the block is only [NOP]. Wait, the scanner includes
	// BRK because it's a block terminator.
	// Actually: BRK IS compilable=false and IS a block terminator.
	// The scanner includes block terminators. BRK is included.
	// But in compileBlock6502, BRK will hit the default case and bail.
	// For this test, the block is [NOP, BRK]. The NOP emits nothing,
	// then BRK hits the default and bails.
	//
	// Hmm, that means the bail sets RetPC = 0x0601 (BRK's PC).
	// Let me verify.
	if rig.ctx.RetPC != 0x0601 {
		t.Errorf("RetPC = 0x%04X, want 0x0601 (BRK bail)", rig.ctx.RetPC)
	}
	if rig.ctx.NeedBail != 1 {
		t.Errorf("NeedBail = %d, want 1 (BRK should bail)", rig.ctx.NeedBail)
	}
}

func TestJIT6502_AMD64_NOP_Cycles(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600

	// 3 NOPs + BRK. The 3 NOPs should accumulate 6 cycles (3 x 2).
	// BRK will bail, so we only get cycles for the NOPs.
	rig.compileAndRun(t, []byte{0xEA, 0xEA, 0xEA, 0x00}, 0x0600)

	if rig.ctx.RetCycles != 6 {
		t.Errorf("RetCycles = %d, want 6 (3 NOPs x 2 cycles)", rig.ctx.RetCycles)
	}
	if rig.ctx.RetCount != 3 {
		t.Errorf("RetCount = %d, want 3 (3 NOPs before BRK bail)", rig.ctx.RetCount)
	}
}

func TestJIT6502_AMD64_MultipleNOPs_PC(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600

	// 5 NOPs + BRK
	rig.compileAndRun(t, []byte{0xEA, 0xEA, 0xEA, 0xEA, 0xEA, 0x00}, 0x0600)

	// BRK is at 0x0605, bail sets RetPC to BRK's address
	if rig.ctx.RetPC != 0x0605 {
		t.Errorf("RetPC = 0x%04X, want 0x0605", rig.ctx.RetPC)
	}
	if rig.ctx.RetCount != 5 {
		t.Errorf("RetCount = %d, want 5", rig.ctx.RetCount)
	}
	if rig.ctx.RetCycles != 10 {
		t.Errorf("RetCycles = %d, want 10 (5 NOPs x 2)", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_NOP_OnlyBlock(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600

	// Two NOPs + JMP $0700 — JMP is a block terminator with proper epilogue
	program := []byte{
		0xEA,             // NOP (2 cycles)
		0xEA,             // NOP (2 cycles)
		0x4C, 0x00, 0x07, // JMP $0700 (3 cycles)
	}
	rig.compileAndRun(t, program, 0x0600)

	if rig.ctx.NeedBail != 0 {
		t.Errorf("NeedBail = %d, want 0 (JMP compiled)", rig.ctx.NeedBail)
	}
	if rig.ctx.RetPC != 0x0700 {
		t.Errorf("RetPC = 0x%04X, want 0x0700 (JMP target)", rig.ctx.RetPC)
	}
	if rig.ctx.RetCycles != 7 {
		t.Errorf("RetCycles = %d, want 7 (2 NOPs + JMP = 4+3)", rig.ctx.RetCycles)
	}
}

// ===========================================================================
// Stage 3: Load/Store/Transfer Tests
// ===========================================================================

func TestJIT6502_AMD64_LDA_Immediate(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.A = 0x00
	rig.cpu.PC = 0x0600
	rig.cpu.SR = 0x00

	// LDA #$42 + BRK
	rig.compileAndRun(t, []byte{0xA9, 0x42, 0x00}, 0x0600)

	if rig.cpu.A != 0x42 {
		t.Errorf("A = 0x%02X, want 0x42", rig.cpu.A)
	}
	// N=0 (bit 7 clear), Z=0 (non-zero)
	if rig.cpu.SR&0x82 != 0 {
		t.Errorf("SR N/Z = 0x%02X, want 0x00", rig.cpu.SR&0x82)
	}
	if rig.ctx.RetCycles != 2 {
		t.Errorf("RetCycles = %d, want 2", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_LDA_Immediate_Zero(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.A = 0xFF
	rig.cpu.PC = 0x0600
	rig.cpu.SR = 0x00

	// LDA #$00 + BRK
	rig.compileAndRun(t, []byte{0xA9, 0x00, 0x00}, 0x0600)

	if rig.cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00", rig.cpu.A)
	}
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z flag should be set")
	}
	if rig.cpu.SR&NEGATIVE_FLAG != 0 {
		t.Error("N flag should not be set")
	}
}

func TestJIT6502_AMD64_LDA_Immediate_Negative(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.A = 0x00
	rig.cpu.PC = 0x0600
	rig.cpu.SR = 0x00

	// LDA #$80 + BRK
	rig.compileAndRun(t, []byte{0xA9, 0x80, 0x00}, 0x0600)

	if rig.cpu.A != 0x80 {
		t.Errorf("A = 0x%02X, want 0x80", rig.cpu.A)
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N flag should be set")
	}
	if rig.cpu.SR&ZERO_FLAG != 0 {
		t.Error("Z flag should not be set")
	}
}

func TestJIT6502_AMD64_LDA_ZeroPage(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.bus.Write8(0x0042, 0x55) // ZP address $42 = 0x55

	// LDA $42 + BRK
	rig.compileAndRun(t, []byte{0xA5, 0x42, 0x00}, 0x0600)

	if rig.cpu.A != 0x55 {
		t.Errorf("A = 0x%02X, want 0x55", rig.cpu.A)
	}
	if rig.ctx.RetCycles != 3 {
		t.Errorf("RetCycles = %d, want 3", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_LDA_ZeroPageX(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0x04
	rig.bus.Write8(0x0046, 0xAB) // ZP address $42 + $04 = $46

	// LDA $42,X + BRK
	rig.compileAndRun(t, []byte{0xB5, 0x42, 0x00}, 0x0600)

	if rig.cpu.A != 0xAB {
		t.Errorf("A = 0x%02X, want 0xAB", rig.cpu.A)
	}
	if rig.ctx.RetCycles != 4 {
		t.Errorf("RetCycles = %d, want 4", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_LDA_ZeroPageX_Wrap(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0x10
	rig.bus.Write8(0x0005, 0xCC) // ZP $F5 + $10 = $105 wraps to $05

	// LDA $F5,X + BRK
	rig.compileAndRun(t, []byte{0xB5, 0xF5, 0x00}, 0x0600)

	if rig.cpu.A != 0xCC {
		t.Errorf("A = 0x%02X, want 0xCC (ZP wrap)", rig.cpu.A)
	}
}

func TestJIT6502_AMD64_LDA_Absolute(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.bus.Write8(0x1000, 0x77) // Address $1000 (within fast path)

	// LDA $1000 + BRK
	rig.compileAndRun(t, []byte{0xAD, 0x00, 0x10, 0x00}, 0x0600)

	if rig.cpu.A != 0x77 {
		t.Errorf("A = 0x%02X, want 0x77", rig.cpu.A)
	}
	if rig.ctx.RetCycles != 4 {
		t.Errorf("RetCycles = %d, want 4", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_LDA_AbsoluteX(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0x05
	rig.bus.Write8(0x1005, 0x33) // $1000 + $05

	// LDA $1000,X + BRK — no page cross ($10xx → $10xx)
	rig.compileAndRun(t, []byte{0xBD, 0x00, 0x10, 0x00}, 0x0600)

	if rig.cpu.A != 0x33 {
		t.Errorf("A = 0x%02X, want 0x33", rig.cpu.A)
	}
	if rig.ctx.RetCycles != 4 {
		t.Errorf("RetCycles = %d, want 4 (no page cross)", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_LDA_AbsoluteX_PageCross(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0xFF
	rig.bus.Write8(0x10FF, 0x44) // $1000 + $FF = $10FF — page cross from $10 to $10

	// Wait, $1000 + $FF = $10FF. Page of $1000 = $10, page of $10FF = $10. No cross!
	// Let me use $10FE + $05 = $1103. Page $10 → $11. Cross!
	rig.bus.Write8(0x1103, 0x44)
	rig.cpu.X = 0x05

	// LDA $10FE,X + BRK  — page cross ($10FE + $05 = $1103, page $10→$11)
	rig.compileAndRun(t, []byte{0xBD, 0xFE, 0x10, 0x00}, 0x0600)

	if rig.cpu.A != 0x44 {
		t.Errorf("A = 0x%02X, want 0x44", rig.cpu.A)
	}
	if rig.ctx.RetCycles != 5 {
		t.Errorf("RetCycles = %d, want 5 (4 base + 1 page cross)", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_LDA_AbsoluteY(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.Y = 0x03
	rig.bus.Write8(0x1003, 0x88)

	// LDA $1000,Y + BRK
	rig.compileAndRun(t, []byte{0xB9, 0x00, 0x10, 0x00}, 0x0600)

	if rig.cpu.A != 0x88 {
		t.Errorf("A = 0x%02X, want 0x88", rig.cpu.A)
	}
}

func TestJIT6502_AMD64_LDA_IndirectX(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0x04
	// Pointer at ZP $44 (=$40+$04): points to $1000
	rig.bus.Write8(0x0044, 0x00) // low byte
	rig.bus.Write8(0x0045, 0x10) // high byte
	rig.bus.Write8(0x1000, 0xEE) // data at $1000

	// LDA ($40,X) + BRK
	rig.compileAndRun(t, []byte{0xA1, 0x40, 0x00}, 0x0600)

	if rig.cpu.A != 0xEE {
		t.Errorf("A = 0x%02X, want 0xEE", rig.cpu.A)
	}
	if rig.ctx.RetCycles != 6 {
		t.Errorf("RetCycles = %d, want 6", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_LDA_IndirectY(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.Y = 0x01
	// Pointer at ZP $42: points to $1000
	rig.bus.Write8(0x0042, 0x00) // low byte
	rig.bus.Write8(0x0043, 0x10) // high byte
	rig.bus.Write8(0x1001, 0xDD) // data at $1000 + $01 = $1001

	// LDA ($42),Y + BRK
	rig.compileAndRun(t, []byte{0xB1, 0x42, 0x00}, 0x0600)

	if rig.cpu.A != 0xDD {
		t.Errorf("A = 0x%02X, want 0xDD", rig.cpu.A)
	}
	if rig.ctx.RetCycles != 5 {
		t.Errorf("RetCycles = %d, want 5 (no page cross)", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_LDA_IOBail(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600

	// LDA $D200 — I/O page, should bail
	rig.compileAndRun(t, []byte{0xAD, 0x00, 0xD2, 0x00}, 0x0600)

	if rig.ctx.NeedBail != 1 {
		t.Errorf("NeedBail = %d, want 1 (I/O address)", rig.ctx.NeedBail)
	}
	if rig.ctx.RetPC != 0x0600 {
		t.Errorf("RetPC = 0x%04X, want 0x0600 (bail re-executes LDA)", rig.ctx.RetPC)
	}
}

func TestJIT6502_AMD64_LDA_HighAddrBail(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600

	// LDA $2000 — above fast-path limit, should bail
	rig.compileAndRun(t, []byte{0xAD, 0x00, 0x20, 0x00}, 0x0600)

	if rig.ctx.NeedBail != 1 {
		t.Errorf("NeedBail = %d, want 1 (above fast-path limit)", rig.ctx.NeedBail)
	}
}

func TestJIT6502_AMD64_STA_ZeroPage(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x42

	// STA $10 + BRK
	rig.compileAndRun(t, []byte{0x85, 0x10, 0x00}, 0x0600)

	val := rig.bus.Read8(0x0010)
	if val != 0x42 {
		t.Errorf("mem[$10] = 0x%02X, want 0x42", val)
	}
	if rig.ctx.RetCycles != 3 {
		t.Errorf("RetCycles = %d, want 3", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_STA_Absolute(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x99

	// STA $1000 + BRK
	rig.compileAndRun(t, []byte{0x8D, 0x00, 0x10, 0x00}, 0x0600)

	val := rig.bus.Read8(0x1000)
	if val != 0x99 {
		t.Errorf("mem[$1000] = 0x%02X, want 0x99", val)
	}
}

func TestJIT6502_AMD64_STA_SelfMod(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0xEA // NOP opcode

	// STA $0600 + BRK — store to the code page itself!
	// The code page bitmap should be set for page 6 (0x0600>>8=6)
	rig.compileAndRun(t, []byte{0x8D, 0x00, 0x06, 0x00}, 0x0600)

	// The store should succeed (data written), but NeedInval should be set
	if rig.ctx.NeedInval != 1 {
		t.Errorf("NeedInval = %d, want 1 (self-modification detected)", rig.ctx.NeedInval)
	}
	// RetPC should be the next instruction (store completed)
	if rig.ctx.RetPC != 0x0603 {
		t.Errorf("RetPC = 0x%04X, want 0x0603 (next instruction)", rig.ctx.RetPC)
	}
}

func TestJIT6502_AMD64_LDX_Immediate(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600

	// LDX #$33 + BRK
	rig.compileAndRun(t, []byte{0xA2, 0x33, 0x00}, 0x0600)

	if rig.cpu.X != 0x33 {
		t.Errorf("X = 0x%02X, want 0x33", rig.cpu.X)
	}
}

func TestJIT6502_AMD64_LDY_Immediate(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600

	// LDY #$77 + BRK
	rig.compileAndRun(t, []byte{0xA0, 0x77, 0x00}, 0x0600)

	if rig.cpu.Y != 0x77 {
		t.Errorf("Y = 0x%02X, want 0x77", rig.cpu.Y)
	}
}

func TestJIT6502_AMD64_TAX(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x55

	// TAX + BRK
	rig.compileAndRun(t, []byte{0xAA, 0x00}, 0x0600)

	if rig.cpu.X != 0x55 {
		t.Errorf("X = 0x%02X, want 0x55", rig.cpu.X)
	}
}

func TestJIT6502_AMD64_TXA(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0xAA

	// TXA + BRK
	rig.compileAndRun(t, []byte{0x8A, 0x00}, 0x0600)

	if rig.cpu.A != 0xAA {
		t.Errorf("A = 0x%02X, want 0xAA", rig.cpu.A)
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N flag should be set (0xAA has bit 7)")
	}
}

func TestJIT6502_AMD64_TAY(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x00

	// TAY + BRK
	rig.compileAndRun(t, []byte{0xA8, 0x00}, 0x0600)

	if rig.cpu.Y != 0x00 {
		t.Errorf("Y = 0x%02X, want 0x00", rig.cpu.Y)
	}
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z flag should be set (A=0)")
	}
}

func TestJIT6502_AMD64_TYA(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.Y = 0x42

	// TYA + BRK
	rig.compileAndRun(t, []byte{0x98, 0x00}, 0x0600)

	if rig.cpu.A != 0x42 {
		t.Errorf("A = 0x%02X, want 0x42", rig.cpu.A)
	}
}

func TestJIT6502_AMD64_TSX(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SP = 0xFD

	// TSX + BRK
	rig.compileAndRun(t, []byte{0xBA, 0x00}, 0x0600)

	if rig.cpu.X != 0xFD {
		t.Errorf("X = 0x%02X, want 0xFD", rig.cpu.X)
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N flag should be set (0xFD has bit 7)")
	}
}

func TestJIT6502_AMD64_TXS(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0xFF

	// TXS + BRK (no flag update)
	rig.compileAndRun(t, []byte{0x9A, 0x00}, 0x0600)

	if rig.cpu.SP != 0xFF {
		t.Errorf("SP = 0x%02X, want 0xFF", rig.cpu.SP)
	}
}

func TestJIT6502_AMD64_LDA_STA_RoundTrip(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.bus.Write8(0x0010, 0xAB)

	// LDA $10; STA $20; BRK
	rig.compileAndRun(t, []byte{0xA5, 0x10, 0x85, 0x20, 0x00}, 0x0600)

	val := rig.bus.Read8(0x0020)
	if val != 0xAB {
		t.Errorf("mem[$20] = 0x%02X, want 0xAB", val)
	}
	if rig.cpu.A != 0xAB {
		t.Errorf("A = 0x%02X, want 0xAB", rig.cpu.A)
	}
	// 3 cycles (LDA zp) + 3 cycles (STA zp) = 6
	if rig.ctx.RetCycles != 6 {
		t.Errorf("RetCycles = %d, want 6", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_STX_ZeroPage(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0x42

	// STX $10 + BRK
	rig.compileAndRun(t, []byte{0x86, 0x10, 0x00}, 0x0600)

	val := rig.bus.Read8(0x0010)
	if val != 0x42 {
		t.Errorf("mem[$10] = 0x%02X, want 0x42", val)
	}
}

func TestJIT6502_AMD64_STY_ZeroPage(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.Y = 0x77

	// STY $10 + BRK
	rig.compileAndRun(t, []byte{0x84, 0x10, 0x00}, 0x0600)

	val := rig.bus.Read8(0x0010)
	if val != 0x77 {
		t.Errorf("mem[$10] = 0x%02X, want 0x77", val)
	}
}

// ===========================================================================
// Stage 4: ALU Tests
// ===========================================================================

func TestJIT6502_AMD64_ADC_Binary_NoCarry(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x10
	rig.cpu.SR = 0x00 // C=0, D=0

	// ADC #$20 + BRK (SR already has C=0)
	rig.compileAndRun(t, []byte{0x69, 0x20, 0x00}, 0x0600)

	if rig.cpu.A != 0x30 {
		t.Errorf("A = 0x%02X, want 0x30", rig.cpu.A)
	}
	if rig.cpu.SR&CARRY_FLAG != 0 {
		t.Error("C should be clear")
	}
	if rig.cpu.SR&ZERO_FLAG != 0 {
		t.Error("Z should be clear")
	}
	if rig.cpu.SR&NEGATIVE_FLAG != 0 {
		t.Error("N should be clear")
	}
	if rig.cpu.SR&OVERFLOW_FLAG != 0 {
		t.Error("V should be clear")
	}
}

func TestJIT6502_AMD64_ADC_Binary_WithCarryIn(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x10
	rig.cpu.SR = CARRY_FLAG // C=1

	// ADC #$20 + BRK (with carry in)
	rig.compileAndRun(t, []byte{0x69, 0x20, 0x00}, 0x0600)

	if rig.cpu.A != 0x31 { // 0x10 + 0x20 + 1 = 0x31
		t.Errorf("A = 0x%02X, want 0x31", rig.cpu.A)
	}
}

func TestJIT6502_AMD64_ADC_Binary_CarryOut(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0xFF
	rig.cpu.SR = 0x00

	// ADC #$01 + BRK — 0xFF + 0x01 = 0x100 → A=0x00, C=1
	rig.compileAndRun(t, []byte{0x69, 0x01, 0x00}, 0x0600)

	if rig.cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00", rig.cpu.A)
	}
	if rig.cpu.SR&CARRY_FLAG == 0 {
		t.Error("C should be set (carry out)")
	}
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set (result is 0)")
	}
}

func TestJIT6502_AMD64_ADC_Binary_Overflow(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x50
	rig.cpu.SR = 0x00

	// ADC #$50 → 0x50 + 0x50 = 0xA0 — signed overflow (80+80 > 127)
	rig.compileAndRun(t, []byte{0x69, 0x50, 0x00}, 0x0600)

	if rig.cpu.A != 0xA0 {
		t.Errorf("A = 0x%02X, want 0xA0", rig.cpu.A)
	}
	if rig.cpu.SR&OVERFLOW_FLAG == 0 {
		t.Error("V should be set (signed overflow: 80+80=160)")
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N should be set (0xA0 has bit 7)")
	}
}

func TestJIT6502_AMD64_ADC_DecimalBail(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x10
	rig.cpu.SR = DECIMAL_FLAG // D=1

	// ADC #$20 — decimal mode, should bail
	rig.compileAndRun(t, []byte{0x69, 0x20, 0x00}, 0x0600)

	if rig.ctx.NeedBail != 1 {
		t.Errorf("NeedBail = %d, want 1 (decimal mode)", rig.ctx.NeedBail)
	}
	if rig.ctx.RetPC != 0x0600 {
		t.Errorf("RetPC = 0x%04X, want 0x0600", rig.ctx.RetPC)
	}
}

func TestJIT6502_AMD64_SBC_Binary(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x50
	rig.cpu.SR = CARRY_FLAG // C=1 (no borrow)

	// SBC #$10 → 0x50 - 0x10 - 0 = 0x40
	rig.compileAndRun(t, []byte{0xE9, 0x10, 0x00}, 0x0600)

	if rig.cpu.A != 0x40 {
		t.Errorf("A = 0x%02X, want 0x40", rig.cpu.A)
	}
	if rig.cpu.SR&CARRY_FLAG == 0 {
		t.Error("C should be set (no borrow)")
	}
	if rig.cpu.SR&ZERO_FLAG != 0 {
		t.Error("Z should be clear")
	}
	if rig.cpu.SR&NEGATIVE_FLAG != 0 {
		t.Error("N should be clear")
	}
}

func TestJIT6502_AMD64_SBC_Binary_Borrow(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x10
	rig.cpu.SR = CARRY_FLAG // C=1

	// SBC #$20 → 0x10 - 0x20 - 0 = -0x10 = 0xF0
	rig.compileAndRun(t, []byte{0xE9, 0x20, 0x00}, 0x0600)

	if rig.cpu.A != 0xF0 {
		t.Errorf("A = 0x%02X, want 0xF0", rig.cpu.A)
	}
	if rig.cpu.SR&CARRY_FLAG != 0 {
		t.Error("C should be clear (borrow occurred)")
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N should be set (0xF0 has bit 7)")
	}
}

func TestJIT6502_AMD64_SBC_Binary_WithBorrow(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x50
	rig.cpu.SR = 0x00 // C=0 (borrow in)

	// SBC #$10 → 0x50 - 0x10 - 1 = 0x3F
	rig.compileAndRun(t, []byte{0xE9, 0x10, 0x00}, 0x0600)

	if rig.cpu.A != 0x3F {
		t.Errorf("A = 0x%02X, want 0x3F", rig.cpu.A)
	}
}

func TestJIT6502_AMD64_AND_Immediate(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0xFF

	// AND #$0F + BRK
	rig.compileAndRun(t, []byte{0x29, 0x0F, 0x00}, 0x0600)

	if rig.cpu.A != 0x0F {
		t.Errorf("A = 0x%02X, want 0x0F", rig.cpu.A)
	}
	if rig.cpu.SR&ZERO_FLAG != 0 {
		t.Error("Z should be clear")
	}
	if rig.cpu.SR&NEGATIVE_FLAG != 0 {
		t.Error("N should be clear")
	}
}

func TestJIT6502_AMD64_AND_Zero(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0xAA

	// AND #$55 → 0xAA & 0x55 = 0x00
	rig.compileAndRun(t, []byte{0x29, 0x55, 0x00}, 0x0600)

	if rig.cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00", rig.cpu.A)
	}
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set")
	}
}

func TestJIT6502_AMD64_ORA_Immediate(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0xA0

	// ORA #$0F → 0xA0 | 0x0F = 0xAF
	rig.compileAndRun(t, []byte{0x09, 0x0F, 0x00}, 0x0600)

	if rig.cpu.A != 0xAF {
		t.Errorf("A = 0x%02X, want 0xAF", rig.cpu.A)
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N should be set (0xAF has bit 7)")
	}
}

func TestJIT6502_AMD64_EOR_Immediate(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0xFF

	// EOR #$FF → 0xFF ^ 0xFF = 0x00
	rig.compileAndRun(t, []byte{0x49, 0xFF, 0x00}, 0x0600)

	if rig.cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00", rig.cpu.A)
	}
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set")
	}
}

func TestJIT6502_AMD64_CMP_Immediate_Equal(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x42

	// CMP #$42 + BRK — A == operand
	rig.compileAndRun(t, []byte{0xC9, 0x42, 0x00}, 0x0600)

	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set (A == operand)")
	}
	if rig.cpu.SR&CARRY_FLAG == 0 {
		t.Error("C should be set (A >= operand)")
	}
	if rig.cpu.SR&NEGATIVE_FLAG != 0 {
		t.Error("N should be clear (result is 0)")
	}
}

func TestJIT6502_AMD64_CMP_Immediate_Greater(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x50

	// CMP #$42 → A > operand
	rig.compileAndRun(t, []byte{0xC9, 0x42, 0x00}, 0x0600)

	if rig.cpu.SR&ZERO_FLAG != 0 {
		t.Error("Z should be clear")
	}
	if rig.cpu.SR&CARRY_FLAG == 0 {
		t.Error("C should be set (A >= operand)")
	}
}

func TestJIT6502_AMD64_CMP_Immediate_Less(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x30

	// CMP #$42 → A < operand
	rig.compileAndRun(t, []byte{0xC9, 0x42, 0x00}, 0x0600)

	if rig.cpu.SR&CARRY_FLAG != 0 {
		t.Error("C should be clear (A < operand)")
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N should be set (result 0x30-0x42 = 0xEE, bit 7 set)")
	}
}

func TestJIT6502_AMD64_CPX_Immediate(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0x10

	// CPX #$10 — equal
	rig.compileAndRun(t, []byte{0xE0, 0x10, 0x00}, 0x0600)

	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set")
	}
	if rig.cpu.SR&CARRY_FLAG == 0 {
		t.Error("C should be set")
	}
}

func TestJIT6502_AMD64_CPY_Immediate(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.Y = 0x20

	// CPY #$10 — Y > operand
	rig.compileAndRun(t, []byte{0xC0, 0x10, 0x00}, 0x0600)

	if rig.cpu.SR&CARRY_FLAG == 0 {
		t.Error("C should be set (Y >= operand)")
	}
	if rig.cpu.SR&ZERO_FLAG != 0 {
		t.Error("Z should be clear")
	}
}

func TestJIT6502_AMD64_INX(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0x41

	// INX + BRK
	rig.compileAndRun(t, []byte{0xE8, 0x00}, 0x0600)

	if rig.cpu.X != 0x42 {
		t.Errorf("X = 0x%02X, want 0x42", rig.cpu.X)
	}
}

func TestJIT6502_AMD64_INX_Wrap(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0xFF

	// INX + BRK — 0xFF + 1 = 0x00 (wrap)
	rig.compileAndRun(t, []byte{0xE8, 0x00}, 0x0600)

	if rig.cpu.X != 0x00 {
		t.Errorf("X = 0x%02X, want 0x00 (wrap)", rig.cpu.X)
	}
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set")
	}
}

func TestJIT6502_AMD64_DEX(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0x01

	// DEX + BRK — 0x01 - 1 = 0x00
	rig.compileAndRun(t, []byte{0xCA, 0x00}, 0x0600)

	if rig.cpu.X != 0x00 {
		t.Errorf("X = 0x%02X, want 0x00", rig.cpu.X)
	}
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set")
	}
}

func TestJIT6502_AMD64_DEX_Wrap(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0x00

	// DEX + BRK — 0x00 - 1 = 0xFF (wrap)
	rig.compileAndRun(t, []byte{0xCA, 0x00}, 0x0600)

	if rig.cpu.X != 0xFF {
		t.Errorf("X = 0x%02X, want 0xFF (wrap)", rig.cpu.X)
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N should be set (0xFF)")
	}
}

func TestJIT6502_AMD64_INY_DEY(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.Y = 0x10

	// INY; DEY; BRK — should end up at 0x10
	rig.compileAndRun(t, []byte{0xC8, 0x88, 0x00}, 0x0600)

	if rig.cpu.Y != 0x10 {
		t.Errorf("Y = 0x%02X, want 0x10", rig.cpu.Y)
	}
}

func TestJIT6502_AMD64_INC_ZeroPage(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.bus.Write8(0x0010, 0x41)

	// INC $10 + BRK
	rig.compileAndRun(t, []byte{0xE6, 0x10, 0x00}, 0x0600)

	val := rig.bus.Read8(0x0010)
	if val != 0x42 {
		t.Errorf("mem[$10] = 0x%02X, want 0x42", val)
	}
	if rig.ctx.RetCycles != 5 {
		t.Errorf("RetCycles = %d, want 5", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_DEC_ZeroPage(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.bus.Write8(0x0010, 0x01)

	// DEC $10 + BRK
	rig.compileAndRun(t, []byte{0xC6, 0x10, 0x00}, 0x0600)

	val := rig.bus.Read8(0x0010)
	if val != 0x00 {
		t.Errorf("mem[$10] = 0x%02X, want 0x00", val)
	}
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set")
	}
}

func TestJIT6502_AMD64_INC_Wrap(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.bus.Write8(0x0010, 0xFF)

	// INC $10 — 0xFF + 1 = 0x00 (wrap)
	rig.compileAndRun(t, []byte{0xE6, 0x10, 0x00}, 0x0600)

	val := rig.bus.Read8(0x0010)
	if val != 0x00 {
		t.Errorf("mem[$10] = 0x%02X, want 0x00", val)
	}
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set")
	}
}

func TestJIT6502_AMD64_BIT_ZeroPage(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x00
	rig.bus.Write8(0x0010, 0xC0) // bits 7 and 6 set

	// BIT $10 + BRK
	rig.compileAndRun(t, []byte{0x24, 0x10, 0x00}, 0x0600)

	// Z = (A AND mem) == 0 → 0x00 & 0xC0 = 0x00, Z=1
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set (A AND mem = 0)")
	}
	// N = bit 7 of mem (0xC0 has bit 7)
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N should be set (bit 7 of operand)")
	}
	// V = bit 6 of mem (0xC0 has bit 6)
	if rig.cpu.SR&OVERFLOW_FLAG == 0 {
		t.Error("V should be set (bit 6 of operand)")
	}
}

func TestJIT6502_AMD64_BIT_NotZero(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x01
	rig.bus.Write8(0x0010, 0x01) // bit 0 only

	// BIT $10
	rig.compileAndRun(t, []byte{0x24, 0x10, 0x00}, 0x0600)

	// Z = (A AND mem) = 0x01, not zero
	if rig.cpu.SR&ZERO_FLAG != 0 {
		t.Error("Z should be clear")
	}
	if rig.cpu.SR&NEGATIVE_FLAG != 0 {
		t.Error("N should be clear")
	}
	if rig.cpu.SR&OVERFLOW_FLAG != 0 {
		t.Error("V should be clear")
	}
}

func TestJIT6502_AMD64_AND_ZeroPage(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0xFF
	rig.bus.Write8(0x0042, 0xF0)

	// AND $42 + BRK
	rig.compileAndRun(t, []byte{0x25, 0x42, 0x00}, 0x0600)

	if rig.cpu.A != 0xF0 {
		t.Errorf("A = 0x%02X, want 0xF0", rig.cpu.A)
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N should be set (0xF0)")
	}
}

func TestJIT6502_AMD64_ADC_ZeroPage(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x10
	rig.cpu.SR = 0x00
	rig.bus.Write8(0x0042, 0x20)

	// ADC $42 + BRK
	rig.compileAndRun(t, []byte{0x65, 0x42, 0x00}, 0x0600)

	if rig.cpu.A != 0x30 {
		t.Errorf("A = 0x%02X, want 0x30", rig.cpu.A)
	}
}

// ===========================================================================
// Stage 5: Shift/Stack/Flag Tests
// ===========================================================================

func TestJIT6502_AMD64_ASL_A(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x41
	rig.cpu.SR = 0x00

	// ASL A + BRK — 0x41 << 1 = 0x82
	rig.compileAndRun(t, []byte{0x0A, 0x00}, 0x0600)

	if rig.cpu.A != 0x82 {
		t.Errorf("A = 0x%02X, want 0x82", rig.cpu.A)
	}
	if rig.cpu.SR&CARRY_FLAG != 0 {
		t.Error("C should be clear (bit 7 of 0x41 is 0)")
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N should be set (0x82 has bit 7)")
	}
}

func TestJIT6502_AMD64_ASL_A_CarryOut(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x80
	rig.cpu.SR = 0x00

	// ASL A — 0x80 << 1 = 0x00, C=1
	rig.compileAndRun(t, []byte{0x0A, 0x00}, 0x0600)

	if rig.cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00", rig.cpu.A)
	}
	if rig.cpu.SR&CARRY_FLAG == 0 {
		t.Error("C should be set (bit 7 shifted out)")
	}
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set")
	}
}

func TestJIT6502_AMD64_LSR_A(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x42
	rig.cpu.SR = 0x00

	// LSR A — 0x42 >> 1 = 0x21, C=0
	rig.compileAndRun(t, []byte{0x4A, 0x00}, 0x0600)

	if rig.cpu.A != 0x21 {
		t.Errorf("A = 0x%02X, want 0x21", rig.cpu.A)
	}
	if rig.cpu.SR&CARRY_FLAG != 0 {
		t.Error("C should be clear")
	}
}

func TestJIT6502_AMD64_LSR_A_CarryOut(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x01
	rig.cpu.SR = 0x00

	// LSR A — 0x01 >> 1 = 0x00, C=1
	rig.compileAndRun(t, []byte{0x4A, 0x00}, 0x0600)

	if rig.cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00", rig.cpu.A)
	}
	if rig.cpu.SR&CARRY_FLAG == 0 {
		t.Error("C should be set (bit 0 shifted out)")
	}
	if rig.cpu.SR&ZERO_FLAG == 0 {
		t.Error("Z should be set")
	}
}

func TestJIT6502_AMD64_ROL_A(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x41
	rig.cpu.SR = CARRY_FLAG // C=1

	// ROL A — 0x41 rotated left through carry: old C→bit0, bit7→new C
	// 0x41 = 0100_0001, C=1 → result = 1000_0011 = 0x83, new C = 0
	rig.compileAndRun(t, []byte{0x2A, 0x00}, 0x0600)

	if rig.cpu.A != 0x83 {
		t.Errorf("A = 0x%02X, want 0x83", rig.cpu.A)
	}
	if rig.cpu.SR&CARRY_FLAG != 0 {
		t.Error("C should be clear (old bit 7 was 0)")
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N should be set (0x83 has bit 7)")
	}
}

func TestJIT6502_AMD64_ROR_A(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x02
	rig.cpu.SR = CARRY_FLAG // C=1

	// ROR A — C→bit7, bit0→new C
	// 0x02 = 0000_0010, C=1 → result = 1000_0001 = 0x81, new C = 0
	rig.compileAndRun(t, []byte{0x6A, 0x00}, 0x0600)

	if rig.cpu.A != 0x81 {
		t.Errorf("A = 0x%02X, want 0x81", rig.cpu.A)
	}
	if rig.cpu.SR&CARRY_FLAG != 0 {
		t.Error("C should be clear (old bit 0 was 0)")
	}
	if rig.cpu.SR&NEGATIVE_FLAG == 0 {
		t.Error("N should be set")
	}
}

func TestJIT6502_AMD64_ASL_ZeroPage(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.bus.Write8(0x0010, 0x42)

	// ASL $10 — 0x42 << 1 = 0x84
	rig.compileAndRun(t, []byte{0x06, 0x10, 0x00}, 0x0600)

	val := rig.bus.Read8(0x0010)
	if val != 0x84 {
		t.Errorf("mem[$10] = 0x%02X, want 0x84", val)
	}
	if rig.ctx.RetCycles != 5 {
		t.Errorf("RetCycles = %d, want 5", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_PHA_PLA(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x42
	rig.cpu.SP = 0xFF

	// PHA; LDA #$00; PLA; BRK — push 0x42, load 0x00, pull back 0x42
	rig.compileAndRun(t, []byte{0x48, 0xA9, 0x00, 0x68, 0x00}, 0x0600)

	if rig.cpu.A != 0x42 {
		t.Errorf("A = 0x%02X, want 0x42 (pulled from stack)", rig.cpu.A)
	}
	if rig.cpu.SP != 0xFF {
		t.Errorf("SP = 0x%02X, want 0xFF (back to original)", rig.cpu.SP)
	}
	// PHA(3) + LDA(2) + PLA(4) = 9 cycles
	if rig.ctx.RetCycles != 9 {
		t.Errorf("RetCycles = %d, want 9", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_PHP_PLP(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SR = CARRY_FLAG | NEGATIVE_FLAG // C=1, N=1
	rig.cpu.SP = 0xFF

	// PHP; PLP; BRK — push and pull status
	rig.compileAndRun(t, []byte{0x08, 0x28, 0x00}, 0x0600)

	// SR should be restored with B cleared and U set
	expectedSR := (CARRY_FLAG|NEGATIVE_FLAG) & ^byte(BREAK_FLAG) | UNUSED_FLAG
	if rig.cpu.SR != expectedSR {
		t.Errorf("SR = 0x%02X, want 0x%02X", rig.cpu.SR, expectedSR)
	}
	if rig.cpu.SP != 0xFF {
		t.Errorf("SP = 0x%02X, want 0xFF", rig.cpu.SP)
	}
}

func TestJIT6502_AMD64_CLC_SEC(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SR = 0x00

	// SEC; BRK — set carry
	rig.compileAndRun(t, []byte{0x38, 0x00}, 0x0600)

	if rig.cpu.SR&CARRY_FLAG == 0 {
		t.Error("C should be set after SEC")
	}

	// Now CLC
	rig.cpu.PC = 0x0600
	rig.cpu.SR = CARRY_FLAG
	rig.ctx.NeedBail = 0
	rig.compileAndRun(t, []byte{0x18, 0x00}, 0x0600)

	if rig.cpu.SR&CARRY_FLAG != 0 {
		t.Error("C should be clear after CLC")
	}
}

func TestJIT6502_AMD64_SEI_CLI(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SR = 0x00

	// SEI; BRK
	rig.compileAndRun(t, []byte{0x78, 0x00}, 0x0600)

	if rig.cpu.SR&INTERRUPT_FLAG == 0 {
		t.Error("I should be set after SEI")
	}

	// CLI
	rig.cpu.PC = 0x0600
	rig.cpu.SR = INTERRUPT_FLAG
	rig.ctx.NeedBail = 0
	rig.compileAndRun(t, []byte{0x58, 0x00}, 0x0600)

	if rig.cpu.SR&INTERRUPT_FLAG != 0 {
		t.Error("I should be clear after CLI")
	}
}

func TestJIT6502_AMD64_SED_CLD(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SR = 0x00

	// SED; BRK
	rig.compileAndRun(t, []byte{0xF8, 0x00}, 0x0600)

	if rig.cpu.SR&DECIMAL_FLAG == 0 {
		t.Error("D should be set after SED")
	}

	// CLD
	rig.cpu.PC = 0x0600
	rig.cpu.SR = DECIMAL_FLAG
	rig.ctx.NeedBail = 0
	rig.compileAndRun(t, []byte{0xD8, 0x00}, 0x0600)

	if rig.cpu.SR&DECIMAL_FLAG != 0 {
		t.Error("D should be clear after CLD")
	}
}

func TestJIT6502_AMD64_CLV(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SR = OVERFLOW_FLAG

	// CLV; BRK
	rig.compileAndRun(t, []byte{0xB8, 0x00}, 0x0600)

	if rig.cpu.SR&OVERFLOW_FLAG != 0 {
		t.Error("V should be clear after CLV")
	}
}

func TestJIT6502_AMD64_ADC_WithCLC(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x10
	rig.cpu.SR = CARRY_FLAG // C=1

	// CLC; ADC #$20; BRK — clear carry, then add
	rig.compileAndRun(t, []byte{0x18, 0x69, 0x20, 0x00}, 0x0600)

	if rig.cpu.A != 0x30 { // 0x10 + 0x20 + 0 (carry cleared by CLC)
		t.Errorf("A = 0x%02X, want 0x30", rig.cpu.A)
	}
}

// ===========================================================================
// Stage 6: Control Flow Tests
// ===========================================================================

func TestJIT6502_AMD64_JMP_Absolute(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600

	// JMP $1234
	rig.compileAndRun(t, []byte{0x4C, 0x34, 0x12}, 0x0600)

	if rig.ctx.RetPC != 0x1234 {
		t.Errorf("RetPC = 0x%04X, want 0x1234", rig.ctx.RetPC)
	}
	if rig.ctx.RetCycles != 3 {
		t.Errorf("RetCycles = %d, want 3", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_JSR_RTS_RoundTrip(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SP = 0xFF

	// JSR $0700 — pushes $0602 (last byte addr) to stack, sets PC to $0700
	rig.compileAndRun(t, []byte{0x20, 0x00, 0x07}, 0x0600)

	if rig.ctx.RetPC != 0x0700 {
		t.Errorf("RetPC = 0x%04X, want 0x0700", rig.ctx.RetPC)
	}
	if rig.cpu.SP != 0xFD {
		t.Errorf("SP = 0x%02X, want 0xFD (pushed 2 bytes)", rig.cpu.SP)
	}
	// Verify stack contents: $01FF = $06 (high), $01FE = $02 (low)
	if rig.bus.Read8(0x01FF) != 0x06 {
		t.Errorf("stack[$01FF] = 0x%02X, want 0x06", rig.bus.Read8(0x01FF))
	}
	if rig.bus.Read8(0x01FE) != 0x02 {
		t.Errorf("stack[$01FE] = 0x%02X, want 0x02", rig.bus.Read8(0x01FE))
	}
	if rig.ctx.RetCycles != 6 {
		t.Errorf("RetCycles = %d, want 6", rig.ctx.RetCycles)
	}

	// Now place RTS at $0700 and run it
	rig.cpu.PC = 0x0700
	rig.ctx.NeedBail = 0
	rig.ctx.RetCycles = 0
	rig.compileAndRun(t, []byte{0x60}, 0x0700) // RTS

	// RTS pulls $0602 from stack, adds 1 → $0603
	if rig.ctx.RetPC != 0x0603 {
		t.Errorf("RetPC after RTS = 0x%04X, want 0x0603", rig.ctx.RetPC)
	}
	if rig.cpu.SP != 0xFF {
		t.Errorf("SP after RTS = 0x%02X, want 0xFF", rig.cpu.SP)
	}
	if rig.ctx.RetCycles != 6 {
		t.Errorf("RetCycles = %d, want 6", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_BNE_NotTaken(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.A = 0x00
	rig.cpu.SR = ZERO_FLAG // Z=1, so BNE not taken

	// BNE +2; NOP; BRK — branch not taken, falls through to NOP
	rig.compileAndRun(t, []byte{0xD0, 0x02, 0xEA, 0x00}, 0x0600)

	// BNE (0 cycles) + NOP (2 cycles) = 2 cycles, then BRK bails
	if rig.ctx.RetCycles != 2 {
		t.Errorf("RetCycles = %d, want 2 (BNE not taken + NOP)", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_BNE_Taken_Forward(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SR = 0x00 // Z=0, so BNE taken

	// BNE +1; NOP_a; NOP_b; JMP $0700
	// Block: [BNE, NOP_a, NOP_b, JMP]. BNE +1 targets NOP_b (internal forward).
	// Taken: BNE(+1) + NOP_b(2) + JMP(3) = 6
	// Not taken: NOP_a(2) + NOP_b(2) + JMP(3) = 7
	rig.compileAndRun(t, []byte{0xD0, 0x01, 0xEA, 0xEA, 0x4C, 0x00, 0x07}, 0x0600)

	if rig.ctx.RetPC != 0x0700 {
		t.Errorf("RetPC = 0x%04X, want 0x0700", rig.ctx.RetPC)
	}
	if rig.ctx.RetCycles != 6 {
		t.Errorf("RetCycles = %d, want 6 (BNE taken +1 + NOP 2 + JMP 3)", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_BEQ_Taken(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SR = ZERO_FLAG // Z=1, BEQ taken

	// BEQ +1; NOP_a; NOP_b; JMP $0700
	// BEQ taken: skip NOP_a, land on NOP_b
	rig.compileAndRun(t, []byte{0xF0, 0x01, 0xEA, 0xEA, 0x4C, 0x00, 0x07}, 0x0600)

	if rig.ctx.RetCycles != 6 { // BEQ taken (+1) + NOP_b (2) + JMP (3)
		t.Errorf("RetCycles = %d, want 6", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_BNE_External(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SR = 0x00 // Z=0, BNE taken

	// BNE to $0700 (external — outside this block)
	// offset = $0700 - ($0600 + 2) = $FE (but that's > 127, can't fit in signed byte)
	// Let me use a closer target: BNE $0610
	// offset = $0610 - ($0600 + 2) = $0E
	// But wait, our block is [BNE, BRK] — the scanner would put BNE inside and BRK at end.
	// The target $0610 is outside the block. So the branch exits.

	rig.compileAndRun(t, []byte{0xD0, 0x0E, 0x00}, 0x0600)

	// BNE taken, target = $0610 (external exit)
	if rig.ctx.RetPC != 0x0610 {
		t.Errorf("RetPC = 0x%04X, want 0x0610", rig.ctx.RetPC)
	}
	// +1 cycle for taken (same page: $06xx → $06xx)
	if rig.ctx.RetCycles != 1 {
		t.Errorf("RetCycles = %d, want 1 (taken penalty only)", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_BCC_SEC(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SR = CARRY_FLAG // C=1, BCC not taken

	// BCC +1; NOP; BRK — BCC not taken
	rig.compileAndRun(t, []byte{0x90, 0x01, 0xEA, 0x00}, 0x0600)

	// Not taken: 0 cycles for BCC + 2 for NOP = 2
	if rig.ctx.RetCycles != 2 {
		t.Errorf("RetCycles = %d, want 2", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_DEX_BNE_Loop(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 3
	rig.cpu.SR = 0x00

	// DEX; BNE -3 (back to DEX); BRK
	// Offset -3: target = $0602 + (-3) = $05FF? No, $0600+0+2 + (-3) = wait...
	// DEX is at offset 0 (1 byte), BNE at offset 1 (2 bytes), BRK at offset 3
	// BNE's PC after fetch = $0600 + 1 + 2 = $0603
	// Offset = -3: target = $0603 + (-3) = $0600 = DEX instruction ✓
	rig.compileAndRun(t, []byte{0xCA, 0xD0, 0xFD, 0x00}, 0x0600)

	// Loop: X=3→2→1→0. DEX runs 3 times, BNE runs 3 times (2 taken, 1 not taken)
	if rig.cpu.X != 0x00 {
		t.Errorf("X = 0x%02X, want 0x00", rig.cpu.X)
	}
	// Cycles: 3*DEX(2) + 2*BNE(taken=+1) + 1*BNE(not=0) = 6 + 2 + 0 = 8
	if rig.ctx.RetCycles != 8 {
		t.Errorf("RetCycles = %d, want 8 (3*DEX + 2*BNE_taken)", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_LDA_CMP_BEQ_Branch(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600

	// LDA #$42; CMP #$42; BEQ +1; NOP_a; NOP_b; JMP $0700
	// CMP sets Z=1, BEQ taken → skip NOP_a, land on NOP_b
	rig.compileAndRun(t, []byte{
		0xA9, 0x42, // LDA #$42 (2 cycles)
		0xC9, 0x42, // CMP #$42 (2 cycles)
		0xF0, 0x01, // BEQ +1 (taken: +1 cycle)
		0xEA,             // NOP_a (skipped)
		0xEA,             // NOP_b (2 cycles, branch target)
		0x4C, 0x00, 0x07, // JMP $0700 (3 cycles)
	}, 0x0600)

	if rig.cpu.A != 0x42 {
		t.Errorf("A = 0x%02X, want 0x42", rig.cpu.A)
	}
	// LDA(2) + CMP(2) + BEQ_taken(+1) + NOP_b(2) + JMP(3) = 10
	if rig.ctx.RetCycles != 10 {
		t.Errorf("RetCycles = %d, want 10", rig.ctx.RetCycles)
	}
}

func TestJIT6502_AMD64_RegisterEdgeCases(t *testing.T) {
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	// Test with extreme register values
	rig.cpu.A = 0x00  // zero
	rig.cpu.X = 0xFF  // max byte
	rig.cpu.Y = 0x80  // high bit set
	rig.cpu.SP = 0x00 // stack empty
	rig.cpu.PC = 0x0600
	rig.cpu.SR = 0x00 // all flags clear

	rig.compileAndRun(t, []byte{0xEA, 0x00}, 0x0600) // NOP + BRK

	if rig.cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00", rig.cpu.A)
	}
	if rig.cpu.X != 0xFF {
		t.Errorf("X = 0x%02X, want 0xFF", rig.cpu.X)
	}
	if rig.cpu.Y != 0x80 {
		t.Errorf("Y = 0x%02X, want 0x80", rig.cpu.Y)
	}
	if rig.cpu.SP != 0x00 {
		t.Errorf("SP = 0x%02X, want 0x00", rig.cpu.SP)
	}
	if rig.cpu.SR != 0x00 {
		t.Errorf("SR = 0x%02X, want 0x00", rig.cpu.SR)
	}
}

// ===========================================================================
// Block Chaining Tests
// ===========================================================================

func TestJIT6502_AMD64_ChainExit_JMPAbs(t *testing.T) {
	// Compile a block ending in JMP abs and verify chainSlots are populated.
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	// NOP; JMP $1234
	mem := rig.cpu.fastAdapter.memDirect
	mem[0x0600] = 0xEA // NOP
	mem[0x0601] = 0x4C // JMP $1234
	mem[0x0602] = 0x34
	mem[0x0603] = 0x12

	instrs := jit6502ScanBlock(mem, 0x0600, len(mem))
	block, err := compileBlock6502(instrs, 0x0600, rig.execMem, &rig.cpu.codePageBitmap)
	if err != nil {
		t.Fatalf("compileBlock6502: %v", err)
	}

	if block.chainEntry == 0 {
		t.Error("chainEntry should be non-zero")
	}

	// Should have at least one chain slot targeting $1234
	found := false
	for _, slot := range block.chainSlots {
		if slot.targetPC == 0x1234 {
			found = true
			if slot.patchAddr == 0 {
				t.Error("chain slot patchAddr should be non-zero")
			}
		}
	}
	if !found {
		t.Error("expected chain slot targeting $1234 from JMP abs")
	}
}

func TestJIT6502_AMD64_ChainExit_DefaultFallthrough(t *testing.T) {
	// Compile a block that ends by size limit (no terminator) and verify
	// the default fallthrough creates a chain slot.
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	// LDA #$42 followed by an uncompilable opcode (block ends before it)
	mem := rig.cpu.fastAdapter.memDirect
	mem[0x0600] = 0xA9 // LDA #$42
	mem[0x0601] = 0x42
	mem[0x0602] = 0xA7 // LAX zp (undocumented, not compilable)
	mem[0x0603] = 0x10

	instrs := jit6502ScanBlock(mem, 0x0600, len(mem))
	if len(instrs) != 1 {
		t.Fatalf("expected 1 instruction, got %d", len(instrs))
	}

	block, err := compileBlock6502(instrs, 0x0600, rig.execMem, &rig.cpu.codePageBitmap)
	if err != nil {
		t.Fatalf("compileBlock6502: %v", err)
	}

	// Default fallthrough should create a chain slot targeting endPC = $0602
	found := false
	for _, slot := range block.chainSlots {
		if slot.targetPC == 0x0602 {
			found = true
		}
	}
	if !found {
		t.Error("expected chain slot targeting $0602 from default fallthrough")
	}
}

func TestJIT6502_AMD64_BidirectionalPatching(t *testing.T) {
	// Compile block A (JMP $0610) then block B at $0610.
	// After B is compiled, A's chain slot should be patched to B's chainEntry.
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	mem := rig.cpu.fastAdapter.memDirect

	// Block A at $0600: NOP; JMP $0610
	mem[0x0600] = 0xEA // NOP
	mem[0x0601] = 0x4C // JMP $0610
	mem[0x0602] = 0x10
	mem[0x0603] = 0x06

	// Block B at $0610: NOP; BRK (block is just NOP, BRK excluded)
	mem[0x0610] = 0xEA // NOP
	mem[0x0611] = 0x00 // BRK

	cache := NewCodeCache()

	// Compile and cache block A
	instrsA := jit6502ScanBlock(mem, 0x0600, len(mem))
	blockA, err := compileBlock6502(instrsA, 0x0600, rig.execMem, &rig.cpu.codePageBitmap)
	if err != nil {
		t.Fatalf("compile block A: %v", err)
	}
	cache.Put(blockA)

	// Compile and cache block B
	instrsB := jit6502ScanBlock(mem, 0x0610, len(mem))
	blockB, err := compileBlock6502(instrsB, 0x0610, rig.execMem, &rig.cpu.codePageBitmap)
	if err != nil {
		t.Fatalf("compile block B: %v", err)
	}
	cache.Put(blockB)

	// Bidirectional patching (as done in the execution loop)
	if blockB.chainEntry != 0 {
		cache.PatchChainsTo(blockB.startPC, blockB.chainEntry)
	}

	// Verify: block A should have a chain slot targeting $0610 that is now patched
	foundPatched := false
	for _, slot := range blockA.chainSlots {
		if slot.targetPC == 0x0610 && slot.patchAddr != 0 {
			// Read the JMP rel32 displacement at patchAddr
			disp := mustExecRel32(t, slot.patchAddr)
			target := uintptr(int64(slot.patchAddr) + 4 + int64(disp))
			if target == blockB.chainEntry {
				foundPatched = true
			}
		}
	}
	if !foundPatched {
		t.Error("block A's chain slot targeting $0610 should be patched to block B's chainEntry")
	}
}

func TestJIT6502_AMD64_ChainExit_JSR(t *testing.T) {
	// Compile a block with JSR and verify chain slot is created.
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	mem := rig.cpu.fastAdapter.memDirect
	mem[0x0600] = 0x20 // JSR $0700
	mem[0x0601] = 0x00
	mem[0x0602] = 0x07

	instrs := jit6502ScanBlock(mem, 0x0600, len(mem))
	block, err := compileBlock6502(instrs, 0x0600, rig.execMem, &rig.cpu.codePageBitmap)
	if err != nil {
		t.Fatalf("compileBlock6502: %v", err)
	}

	found := false
	for _, slot := range block.chainSlots {
		if slot.targetPC == 0x0700 {
			found = true
		}
	}
	if !found {
		t.Error("expected chain slot targeting $0700 from JSR")
	}
}

// ===========================================================================
// Lazy N/Z Flag Tests
// ===========================================================================

func TestJIT6502_AMD64_LazyNZ_DEX_BNE(t *testing.T) {
	// DEX; BNE tight loop — the primary beneficiary of lazy flags.
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.X = 0x05
	// LDX #$05; DEX; BNE -3; BRK
	rig.compileAndRun(t, []byte{0xA2, 0x05, 0xCA, 0xD0, 0xFD, 0x00}, 0x0600)
	if rig.cpu.X != 0x00 {
		t.Errorf("X = 0x%02X, want 0x00", rig.cpu.X)
	}
}

func TestJIT6502_AMD64_LazyNZ_LDA_STA_BNE(t *testing.T) {
	// LDA; STA (flag-neutral); BNE reads deferred Z from LDA
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	// LDA #$00; STA $10; BNE +2; LDA #$42; BRK
	rig.compileAndRun(t, []byte{
		0xA9, 0x00, // LDA #$00 — Z=1
		0x85, 0x10, // STA $10 (no flag change)
		0xD0, 0x02, // BNE +2 (should NOT branch, Z=1)
		0xA9, 0x42, // LDA #$42
		0x00, // BRK
	}, 0x0600)
	if rig.cpu.A != 0x42 {
		t.Errorf("A = 0x%02X, want 0x42 (BNE should not branch when Z=1)", rig.cpu.A)
	}
}

func TestJIT6502_AMD64_LazyNZ_PHP_Materialization(t *testing.T) {
	// LDA #0; PHP — verify Z=1 in pushed SR
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SP = 0xFF
	// LDA #$00; PHP; BRK
	rig.compileAndRun(t, []byte{0xA9, 0x00, 0x08, 0x00}, 0x0600)

	// PHP pushes SR with B and U set. Z should be set.
	pushedSR := rig.bus.Read8(0x01FF) // SP was FF, PHP decrements to FE, stores at 01FF
	if pushedSR&0x02 == 0 {
		t.Errorf("pushed SR = 0x%02X, Z bit should be set (LDA #0)", pushedSR)
	}
}

func TestJIT6502_AMD64_LazyNZ_BMI_Direct(t *testing.T) {
	// LDA #$80; BMI should branch (N=1 from pending)
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	// LDA #$80; BMI +2; LDA #$00; BRK
	rig.compileAndRun(t, []byte{
		0xA9, 0x80, // LDA #$80 — N=1
		0x30, 0x02, // BMI +2 (should branch)
		0xA9, 0x00, // LDA #$00 (skipped)
		0x00, // BRK
	}, 0x0600)
	if rig.cpu.A != 0x80 {
		t.Errorf("A = 0x%02X, want 0x80 (BMI should have branched over LDA #$00)", rig.cpu.A)
	}
}

func TestJIT6502_AMD64_LazyNZ_ADC_BEQ(t *testing.T) {
	// ADC producing zero; BEQ should branch
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SR = 0x01 // C=1
	// LDA #$FF; ADC #$00 → $FF + $00 + C(1) = $100 = $00 with carry. Z=1
	// BEQ +2; LDA #$42; BRK
	rig.compileAndRun(t, []byte{
		0xA9, 0xFF, // LDA #$FF
		0x69, 0x00, // ADC #$00 (+C=1 → A=$00, C=1, Z=1)
		0xF0, 0x02, // BEQ +2 (should branch)
		0xA9, 0x42, // LDA #$42 (skipped)
		0x00, // BRK
	}, 0x0600)
	if rig.cpu.A != 0x00 {
		t.Errorf("A = 0x%02X, want 0x00 (BEQ should have branched)", rig.cpu.A)
	}
}

func TestJIT6502_AMD64_LazyNZ_PendingThenBail(t *testing.T) {
	// LDA #$00 sets Z=1 pending, then LDA $D200 bails (I/O page).
	// The bail epilogue must materialize the pending Z=1 into SR before
	// returning to Go, so the interpreter sees correct flags.
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	// LDA #$00; LDA $D200; BRK
	rig.compileAndRun(t, []byte{
		0xA9, 0x00, // LDA #$00 → Z=1 pending
		0xAD, 0x00, 0xD2, // LDA $D200 → bail (I/O page)
		0x00, // BRK
	}, 0x0600)

	if rig.ctx.NeedBail != 1 {
		t.Fatalf("NeedBail = %d, want 1", rig.ctx.NeedBail)
	}
	// After bail, SR must have Z=1 from the LDA #$00 that preceded the bail
	if rig.cpu.SR&0x02 == 0 {
		t.Errorf("SR = 0x%02X, Z bit should be set (pending from LDA #$00 before bail)", rig.cpu.SR)
	}
}

func TestJIT6502_AMD64_LazyNZ_PLP_ClearsPending(t *testing.T) {
	// LDA #0 (Z=1 pending); PLP clears pending; BEQ uses PLP'd flags
	rig := newJIT6502TestRig(t)
	defer rig.cleanup()

	rig.cpu.PC = 0x0600
	rig.cpu.SP = 0xFD
	// Pre-push SR with Z=0 onto stack at $01FE
	rig.bus.Write8(0x01FE, 0x20) // U bit set, Z=0
	// LDA #$00; PLP; BEQ +2; LDA #$42; BRK
	rig.compileAndRun(t, []byte{
		0xA9, 0x00, // LDA #$00 — Z=1 pending
		0x28,       // PLP — pulls SR=$20 (Z=0), clears pending
		0xF0, 0x02, // BEQ +2 (should NOT branch — PLP set Z=0)
		0xA9, 0x42, // LDA #$42
		0x00, // BRK
	}, 0x0600)
	if rig.cpu.A != 0x42 {
		t.Errorf("A = 0x%02X, want 0x42 (BEQ should not branch after PLP with Z=0)", rig.cpu.A)
	}
}
