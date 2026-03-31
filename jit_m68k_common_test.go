// jit_m68k_common_test.go - Tests for M68020 JIT infrastructure

//go:build amd64 && linux

package main

import (
	"testing"
	"unsafe"
)

// ===========================================================================
// Instruction Length Calculator Tests
// ===========================================================================

// Helper: write big-endian words into a byte slice at the given offset
func beWords(buf []byte, offset int, words ...uint16) {
	for _, w := range words {
		buf[offset] = byte(w >> 8)
		buf[offset+1] = byte(w)
		offset += 2
	}
}

// Helper: write a big-endian uint32 into a byte slice
func beLong(buf []byte, offset int, val uint32) {
	buf[offset] = byte(val >> 24)
	buf[offset+1] = byte(val >> 16)
	buf[offset+2] = byte(val >> 8)
	buf[offset+3] = byte(val)
}

func TestM68KJIT_InstrLength_MOVEQ(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x702A) // MOVEQ #42,D0
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("MOVEQ length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_MOVERegToReg(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x2200) // MOVE.L D0,D1
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("MOVE.L Dn,Dm length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_MOVEImm(t *testing.T) {
	mem := make([]byte, 64)
	// MOVE.L #$12345678,D0 — opcode + 32-bit immediate
	beWords(mem, 0, 0x203C) // MOVE.L #imm,D0
	beLong(mem, 2, 0x12345678)
	if got := m68kInstrLength(mem, 0); got != 6 {
		t.Errorf("MOVE.L #imm,Dn length: got %d, want 6", got)
	}
}

func TestM68KJIT_InstrLength_MOVEWordImm(t *testing.T) {
	mem := make([]byte, 64)
	// MOVE.W #$1234,D0
	beWords(mem, 0, 0x303C, 0x1234)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("MOVE.W #imm,Dn length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_MOVEByteImm(t *testing.T) {
	mem := make([]byte, 64)
	// MOVE.B #$42,D0
	beWords(mem, 0, 0x103C, 0x0042)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("MOVE.B #imm,Dn length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_MOVEIndirect(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x2010) // MOVE.L (A0),D0
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("MOVE.L (An),Dn length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_MOVEPostInc(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x2018) // MOVE.L (A0)+,D0
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("MOVE.L (An)+,Dn length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_MOVEDisp(t *testing.T) {
	mem := make([]byte, 64)
	// MOVE.L (d16,A0),D0
	beWords(mem, 0, 0x2028, 0x0010) // disp = 16
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("MOVE.L (d16,An),Dn length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_MOVEIndexed(t *testing.T) {
	mem := make([]byte, 64)
	// MOVE.L (d8,A0,D0.L),D1 — brief format extension word
	beWords(mem, 0, 0x2230, 0x0800) // brief: D0.L, disp=0
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("MOVE.L (d8,An,Xn),Dn length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_MOVEAbsShort(t *testing.T) {
	mem := make([]byte, 64)
	// MOVE.L (xxx).W,D0
	beWords(mem, 0, 0x2038, 0x1000)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("MOVE.L abs.W,Dn length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_MOVEAbsLong(t *testing.T) {
	mem := make([]byte, 64)
	// MOVE.L (xxx).L,D0
	beWords(mem, 0, 0x2039)
	beLong(mem, 2, 0x00001000)
	if got := m68kInstrLength(mem, 0); got != 6 {
		t.Errorf("MOVE.L abs.L,Dn length: got %d, want 6", got)
	}
}

func TestM68KJIT_InstrLength_MOVEPCRelative(t *testing.T) {
	mem := make([]byte, 64)
	// MOVE.L (d16,PC),D0
	beWords(mem, 0, 0x203A, 0x0010)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("MOVE.L (d16,PC),Dn length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_MOVESrcDstExt(t *testing.T) {
	mem := make([]byte, 64)
	// MOVE.L (d16,A0),(d16,A1) — both src and dst have extension words
	beWords(mem, 0, 0x2368, 0x0010, 0x0020) // src disp=16, dst disp=32
	if got := m68kInstrLength(mem, 0); got != 6 {
		t.Errorf("MOVE.L (d16,An),(d16,An) length: got %d, want 6", got)
	}
}

func TestM68KJIT_InstrLength_ADDLong(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0xD081) // ADD.L D1,D0
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("ADD.L Dn,Dm length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_ADDI(t *testing.T) {
	mem := make([]byte, 64)
	// ADDI.L #$100,D0
	beWords(mem, 0, 0x0680)
	beLong(mem, 2, 0x00000100)
	if got := m68kInstrLength(mem, 0); got != 6 {
		t.Errorf("ADDI.L #imm,Dn length: got %d, want 6", got)
	}
}

func TestM68KJIT_InstrLength_ADDIWord(t *testing.T) {
	mem := make([]byte, 64)
	// ADDI.W #$100,D0
	beWords(mem, 0, 0x0640, 0x0100)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("ADDI.W #imm,Dn length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_BRA_Byte(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x6008) // BRA.B +8
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("BRA.B length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_BRA_Word(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x6000, 0x0100) // BRA.W +256
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("BRA.W length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_BRA_Long(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x60FF) // BRA.L
	beLong(mem, 2, 0x00001000)
	if got := m68kInstrLength(mem, 0); got != 6 {
		t.Errorf("BRA.L length: got %d, want 6", got)
	}
}

func TestM68KJIT_InstrLength_BEQ_Byte(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x6708) // BEQ.B +8
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("BEQ.B length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_BEQ_Word(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x6700, 0x0100) // BEQ.W +256
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("BEQ.W length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_NOP(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x4E71)
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("NOP length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_RTS(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x4E75)
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("RTS length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_JSR_AbsLong(t *testing.T) {
	mem := make([]byte, 64)
	// JSR $12345678
	beWords(mem, 0, 0x4EB9)
	beLong(mem, 2, 0x12345678)
	if got := m68kInstrLength(mem, 0); got != 6 {
		t.Errorf("JSR abs.L length: got %d, want 6", got)
	}
}

func TestM68KJIT_InstrLength_JSR_Disp(t *testing.T) {
	mem := make([]byte, 64)
	// JSR (d16,A0)
	beWords(mem, 0, 0x4EA8, 0x0100)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("JSR (d16,An) length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_LEA(t *testing.T) {
	mem := make([]byte, 64)
	// LEA (d16,A0),A1
	beWords(mem, 0, 0x43E8, 0x0010)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("LEA (d16,An),Am length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_LINK(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x4E56, 0xFFF8) // LINK A6,#-8
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("LINK length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_UNLK(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x4E5E) // UNLK A6
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("UNLK length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_DBcc(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x51C8, 0xFFF8) // DBRA D0,$-8
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("DBRA length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_ADDQ(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x5280) // ADDQ.L #1,D0
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("ADDQ.L #n,Dn length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_LSL_Reg(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0xE1A8) // LSL.L D0,D0 (register shift)
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("LSL.L reg length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_MOVEM(t *testing.T) {
	mem := make([]byte, 64)
	// MOVEM.L D0-D7/A0-A6,-(A7)
	beWords(mem, 0, 0x48E7, 0xFFFE)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("MOVEM length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_MOVEM_Disp(t *testing.T) {
	mem := make([]byte, 64)
	// MOVEM.L (d16,A6),D0-D7/A0-A6
	beWords(mem, 0, 0x4CEE, 0x7FFF, 0xFFF8)
	if got := m68kInstrLength(mem, 0); got != 6 {
		t.Errorf("MOVEM (d16,An) length: got %d, want 6", got)
	}
}

func TestM68KJIT_InstrLength_SWAP(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x4840) // SWAP D0
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("SWAP length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_EXT(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x4880) // EXT.W D0
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("EXT.W length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_CLR(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x4280) // CLR.L D0
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("CLR.L Dn length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_MOVEP(t *testing.T) {
	mem := make([]byte, 64)
	// MOVEP.W D0,(d16,A0)
	beWords(mem, 0, 0x0188, 0x0010)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("MOVEP length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_MOVEC(t *testing.T) {
	mem := make([]byte, 64)
	// MOVEC VBR,D0
	beWords(mem, 0, 0x4E7A, 0x0801)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("MOVEC length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_TRAP(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x4E40) // TRAP #0
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("TRAP length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_LineA(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0xA000)
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("Line A length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_LineF(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0xF200)
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("Line F length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_STOP(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0x4E72, 0x2700) // STOP #$2700
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("STOP length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_PACK(t *testing.T) {
	mem := make([]byte, 64)
	// PACK D0,D1,#$1234
	beWords(mem, 0, 0x8341, 0x1234)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("PACK length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_BFEXTU(t *testing.T) {
	mem := make([]byte, 64)
	// BFEXTU D0{8:8},D1
	beWords(mem, 0, 0xE9C0, 0x2208)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("BFEXTU Dn length: got %d, want 4", got)
	}
}

func TestM68KJIT_InstrLength_LINKL(t *testing.T) {
	mem := make([]byte, 64)
	// LINK.L A6,#-100
	beWords(mem, 0, 0x480E)
	beLong(mem, 2, 0xFFFFFF9C)
	if got := m68kInstrLength(mem, 0); got != 6 {
		t.Errorf("LINK.L length: got %d, want 6", got)
	}
}

func TestM68KJIT_InstrLength_CMPM(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0xB308) // CMPM.B (A0)+,(A1)+
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("CMPM length: got %d, want 2", got)
	}
}

func TestM68KJIT_InstrLength_EXG(t *testing.T) {
	mem := make([]byte, 64)
	beWords(mem, 0, 0xC140) // EXG D0,D0
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("EXG length: got %d, want 2", got)
	}
}

// ===========================================================================
// Block Scanner Tests
// ===========================================================================

func TestM68KJIT_ScanBlock_SimpleSequence(t *testing.T) {
	mem := make([]byte, 1024)
	// MOVEQ #42,D0 (2)
	// ADD.L D0,D1 (2)
	// RTS (2)
	beWords(mem, 0x100, 0x702A) // MOVEQ #42,D0
	beWords(mem, 0x102, 0xD280) // ADD.L D0,D1
	beWords(mem, 0x104, 0x4E75) // RTS

	instrs := m68kScanBlock(mem, 0x100)
	if len(instrs) != 3 {
		t.Fatalf("expected 3 instructions, got %d", len(instrs))
	}
	if instrs[0].opcode != 0x702A {
		t.Errorf("instr[0] opcode: got 0x%04X, want 0x702A", instrs[0].opcode)
	}
	if instrs[0].pcOffset != 0 {
		t.Errorf("instr[0] pcOffset: got %d, want 0", instrs[0].pcOffset)
	}
	if instrs[1].pcOffset != 2 {
		t.Errorf("instr[1] pcOffset: got %d, want 2", instrs[1].pcOffset)
	}
	if instrs[2].pcOffset != 4 {
		t.Errorf("instr[2] pcOffset: got %d, want 4", instrs[2].pcOffset)
	}
	// RTS is a terminator, so scan should stop here
	if instrs[2].opcode != 0x4E75 {
		t.Errorf("last instr should be RTS: got 0x%04X", instrs[2].opcode)
	}
}

func TestM68KJIT_ScanBlock_WithExtWords(t *testing.T) {
	mem := make([]byte, 1024)
	// MOVE.L #$12345678,D0 (6 bytes)
	// MOVEQ #1,D1 (2 bytes)
	// BRA.B -10 (2 bytes, terminator)
	beWords(mem, 0x100, 0x203C)
	beLong(mem, 0x102, 0x12345678)
	beWords(mem, 0x106, 0x7201) // MOVEQ #1,D1
	beWords(mem, 0x108, 0x60F6) // BRA.B -10

	instrs := m68kScanBlock(mem, 0x100)
	if len(instrs) != 3 {
		t.Fatalf("expected 3 instructions, got %d", len(instrs))
	}
	if instrs[0].length != 6 {
		t.Errorf("MOVE.L #imm,D0 length: got %d, want 6", instrs[0].length)
	}
	if instrs[1].pcOffset != 6 {
		t.Errorf("MOVEQ pcOffset: got %d, want 6", instrs[1].pcOffset)
	}
	if instrs[2].pcOffset != 8 {
		t.Errorf("BRA pcOffset: got %d, want 8", instrs[2].pcOffset)
	}
}

func TestM68KJIT_ScanBlock_BccNotTerminator(t *testing.T) {
	mem := make([]byte, 1024)
	// MOVEQ #0,D0
	// BEQ.B +4 (Bcc — NOT a terminator)
	// MOVEQ #1,D0
	// RTS (terminator)
	beWords(mem, 0x100, 0x7000) // MOVEQ #0,D0
	beWords(mem, 0x102, 0x6702) // BEQ.B +2
	beWords(mem, 0x104, 0x7001) // MOVEQ #1,D0
	beWords(mem, 0x106, 0x4E75) // RTS

	instrs := m68kScanBlock(mem, 0x100)
	if len(instrs) != 4 {
		t.Fatalf("expected 4 instructions (Bcc is NOT a terminator), got %d", len(instrs))
	}
}

func TestM68KJIT_ScanBlock_LineATerminator(t *testing.T) {
	mem := make([]byte, 1024)
	beWords(mem, 0x100, 0x7000) // MOVEQ #0,D0
	beWords(mem, 0x102, 0xA000) // Line A trap

	instrs := m68kScanBlock(mem, 0x100)
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions (Line A is terminator), got %d", len(instrs))
	}
}

// ===========================================================================
// Block Terminator Tests
// ===========================================================================

func TestM68KJIT_BlockTerminators(t *testing.T) {
	tests := []struct {
		opcode       uint16
		isTerminator bool
		name         string
	}{
		{0x4E75, true, "RTS"},
		{0x4E73, true, "RTE"},
		{0x4E77, true, "RTR"},
		{0x4E72, true, "STOP"},
		{0x4E76, true, "TRAPV"},
		{0x4E70, true, "RESET"},
		{0x4E40, true, "TRAP #0"},
		{0x4E4F, true, "TRAP #15"},
		{0x4EB9, true, "JSR abs.L"},
		{0x4EC0, true, "JMP (A0)"},
		{0x6000, true, "BRA.W"},
		{0x6008, true, "BRA.B +8"},
		{0x60FF, true, "BRA.L"},
		{0x6100, true, "BSR.W"},
		{0x6108, true, "BSR.B +8"},
		{0xA000, true, "Line A"},
		{0xF200, true, "Line F"},
		// NOT terminators:
		{0x6700, false, "BEQ.W (conditional — NOT terminator)"},
		{0x6708, false, "BEQ.B (conditional — NOT terminator)"},
		{0x6600, false, "BNE.W (conditional — NOT terminator)"},
		{0x702A, false, "MOVEQ"},
		{0xD081, false, "ADD.L"},
		{0x51C8, false, "DBRA"},
		{0x4E71, false, "NOP"},
	}

	for _, tt := range tests {
		got := m68kIsBlockTerminator(tt.opcode)
		if got != tt.isTerminator {
			t.Errorf("%s (0x%04X): isTerminator = %v, want %v", tt.name, tt.opcode, got, tt.isTerminator)
		}
	}
}

// ===========================================================================
// Fallback Detection Tests
// ===========================================================================

func TestM68KJIT_NeedsFallback(t *testing.T) {
	tests := []struct {
		opcode        uint16
		needsFallback bool
		name          string
	}{
		{0xA000, true, "Line A"},
		{0xF200, true, "Line F / FPU"},
		{0x4E72, true, "STOP"},
		{0x4E73, true, "RTE"},
		{0x4E77, true, "RTR"},
		{0x4E70, true, "RESET"},
		{0x4E40, true, "TRAP #0"},
		{0x4E76, true, "TRAPV"},
		{0x4E7A, true, "MOVEC"},
		// NOT fallback:
		{0x702A, false, "MOVEQ"},
		{0xD081, false, "ADD.L"},
		{0x4E75, false, "RTS"},
		{0x6008, false, "BRA.B"},
	}

	for _, tt := range tests {
		instrs := []M68KJITInstr{{opcode: tt.opcode, group: uint8(tt.opcode >> 12)}}
		got := m68kNeedsFallback(instrs)
		if got != tt.needsFallback {
			t.Errorf("%s (0x%04X): needsFallback = %v, want %v", tt.name, tt.opcode, got, tt.needsFallback)
		}
	}
}

// ===========================================================================
// M68KJITContext Tests
// ===========================================================================

func TestM68KJIT_Context_Creation(t *testing.T) {
	bus := NewMachineBus()
	bus.Write32(0, 0x00010000) // SSP
	bus.Write32(4, 0x00001000) // PC
	cpu := NewM68KCPU(bus)

	bitmap := make([]byte, (uint32(len(cpu.memory))+4095)>>12)
	ctx := newM68KJITContext(cpu, bitmap)

	if ctx.DataRegsPtr == 0 {
		t.Error("DataRegsPtr should not be zero")
	}
	if ctx.AddrRegsPtr == 0 {
		t.Error("AddrRegsPtr should not be zero")
	}
	if ctx.MemPtr == 0 {
		t.Error("MemPtr should not be zero")
	}
	if ctx.MemSize == 0 {
		t.Error("MemSize should not be zero")
	}
	if ctx.IOThreshold != 0xA0000 {
		t.Errorf("IOThreshold: got 0x%X, want 0xA0000", ctx.IOThreshold)
	}
	if ctx.SRPtr == 0 {
		t.Error("SRPtr should not be zero")
	}
	if ctx.CodePageBitmapPtr == 0 {
		t.Error("CodePageBitmapPtr should not be zero")
	}
}

// ===========================================================================
// Backward Branch Detection Tests
// ===========================================================================

func TestM68KJIT_DetectBackwardBranch(t *testing.T) {
	// Block: MOVEQ + ADD + BEQ.B -4 (backward to ADD)
	instrs := []M68KJITInstr{
		{opcode: 0x702A, pcOffset: 0, length: 2, group: 7},   // MOVEQ #42,D0
		{opcode: 0xD081, pcOffset: 2, length: 2, group: 0xD}, // ADD.L D1,D0
		{opcode: 0x67FC, pcOffset: 4, length: 2, group: 6},   // BEQ.B -4 (back to offset 2)
	}

	if !m68kDetectBackwardBranches(instrs, 0x1000) {
		t.Error("should detect backward branch: BEQ.B -4 targets within block")
	}
}

func TestM68KJIT_DetectNoBackwardBranch(t *testing.T) {
	// Block: MOVEQ + BEQ.B +4 (forward, not backward)
	instrs := []M68KJITInstr{
		{opcode: 0x702A, pcOffset: 0, length: 2, group: 7}, // MOVEQ #42,D0
		{opcode: 0x6704, pcOffset: 2, length: 2, group: 6}, // BEQ.B +4 (forward)
		{opcode: 0x4E75, pcOffset: 4, length: 2, group: 4}, // RTS
	}

	if m68kDetectBackwardBranches(instrs, 0x1000) {
		t.Error("should NOT detect backward branch: BEQ.B +4 is forward")
	}
}

// ===========================================================================
// Block Chaining Infrastructure Tests (Stage 1)
// ===========================================================================

func TestM68KJIT_ContextChainOffsets(t *testing.T) {
	var ctx M68KJITContext
	base := uintptr(unsafe.Pointer(&ctx))

	tests := []struct {
		name   string
		field  uintptr
		expect uintptr
	}{
		{"ChainBudget", uintptr(unsafe.Pointer(&ctx.ChainBudget)) - base, m68kCtxOffChainBudget},
		{"ChainCount", uintptr(unsafe.Pointer(&ctx.ChainCount)) - base, m68kCtxOffChainCount},
		{"RTSCache0PC", uintptr(unsafe.Pointer(&ctx.RTSCache0PC)) - base, m68kCtxOffRTSCache0PC},
		{"RTSCache0Addr", uintptr(unsafe.Pointer(&ctx.RTSCache0Addr)) - base, m68kCtxOffRTSCache0Addr},
		{"RTSCache1PC", uintptr(unsafe.Pointer(&ctx.RTSCache1PC)) - base, m68kCtxOffRTSCache1PC},
		{"RTSCache1Addr", uintptr(unsafe.Pointer(&ctx.RTSCache1Addr)) - base, m68kCtxOffRTSCache1Addr},
	}
	for _, tc := range tests {
		if tc.field != tc.expect {
			t.Errorf("%s: offset %d, want %d", tc.name, tc.field, tc.expect)
		}
	}
}

func TestM68KJIT_ChainSlotTracking(t *testing.T) {
	block := &JITBlock{
		startPC:    0x1000,
		endPC:      0x100A,
		instrCount: 3,
		execAddr:   0xDEAD0000,
		execSize:   64,
		chainEntry: 0xDEAD0020,
		chainSlots: []chainSlot{
			{targetPC: 0x2000, patchAddr: 0xDEAD0010},
			{targetPC: 0x3000, patchAddr: 0xDEAD0030},
		},
	}

	if block.chainEntry != 0xDEAD0020 {
		t.Errorf("chainEntry: got %x, want %x", block.chainEntry, 0xDEAD0020)
	}
	if len(block.chainSlots) != 2 {
		t.Fatalf("chainSlots length: got %d, want 2", len(block.chainSlots))
	}
	if block.chainSlots[0].targetPC != 0x2000 {
		t.Errorf("slot[0].targetPC: got %x, want %x", block.chainSlots[0].targetPC, 0x2000)
	}
	if block.chainSlots[1].patchAddr != 0xDEAD0030 {
		t.Errorf("slot[1].patchAddr: got %x, want %x", block.chainSlots[1].patchAddr, 0xDEAD0030)
	}
}

func TestM68KJIT_CodeCachePatchChains(t *testing.T) {
	// Allocate real executable memory so PatchRel32At can write
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	defer em.Free()

	// Write a block A with a JMP rel32 (0xE9 + 4-byte displacement)
	// The displacement initially points to itself (displacement = 0)
	codeA := []byte{
		0x90,                         // NOP (padding to get interesting offsets)
		0xE9, 0x00, 0x00, 0x00, 0x00, // JMP rel32 (displacement at offset 1+1=2 from start)
	}
	addrA, err := em.Write(codeA)
	if err != nil {
		t.Fatal(err)
	}
	patchAddr := addrA + 2 // address of the 4-byte displacement

	// Write a block B (the chain target)
	codeB := []byte{0x90, 0xC3} // NOP + RET
	addrB, err := em.Write(codeB)
	if err != nil {
		t.Fatal(err)
	}

	blockA := &JITBlock{
		startPC:    0x1000,
		endPC:      0x1006,
		instrCount: 1,
		execAddr:   addrA,
		execSize:   len(codeA),
		chainSlots: []chainSlot{
			{targetPC: 0x2000, patchAddr: patchAddr},
		},
	}
	blockB := &JITBlock{
		startPC:    0x2000,
		endPC:      0x2004,
		instrCount: 1,
		execAddr:   addrB,
		execSize:   len(codeB),
		chainEntry: addrB, // chain entry = start of block B
	}

	cc := NewCodeCache()
	cc.Put(blockA)
	cc.Put(blockB)

	// Patch chains: block A's exit targeting 0x2000 should be patched to block B's chainEntry
	cc.PatchChainsTo(0x2000, blockB.chainEntry)

	// Verify the displacement was patched correctly
	// Expected: target - (patchAddr + 4)
	expectedDisp := int32(blockB.chainEntry) - int32(patchAddr+4)
	patchBytes := (*[4]byte)(unsafe.Pointer(patchAddr))
	gotDisp := int32(patchBytes[0]) | int32(patchBytes[1])<<8 | int32(patchBytes[2])<<16 | int32(patchBytes[3])<<24
	if gotDisp != expectedDisp {
		t.Errorf("patched displacement: got %d, want %d", gotDisp, expectedDisp)
	}
}

func TestM68KJIT_CodeCacheInvalidateChains(t *testing.T) {
	cc := NewCodeCache()
	cc.Put(&JITBlock{
		startPC:    0x1000,
		chainEntry: 0xDEAD0000,
		chainSlots: []chainSlot{{targetPC: 0x2000, patchAddr: 0xBEEF0000}},
	})
	cc.Put(&JITBlock{
		startPC:    0x2000,
		chainEntry: 0xDEAD1000,
	})

	cc.Invalidate()

	// After invalidation, cache should be empty — chain slots are unreachable
	if cc.Get(0x1000) != nil {
		t.Error("block 0x1000 should be gone after Invalidate")
	}
	if cc.Get(0x2000) != nil {
		t.Error("block 0x2000 should be gone after Invalidate")
	}
	if len(cc.blocks) != 0 {
		t.Errorf("blocks should be empty, got %d entries", len(cc.blocks))
	}
}
