// jit_m68k_common_test.go - Tests for M68020 JIT infrastructure

//go:build amd64 && (linux || windows || darwin)

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

func TestM68KJIT_ScanBlockDoesNotFuseJSRLeaf(t *testing.T) {
	mem := make([]byte, 0x2000)
	const start = 0x100
	const target = 0x1000
	beWords(mem, start, 0x4EB9)
	beLong(mem, start+2, target)
	beWords(mem, start+6, 0x4E71)
	beWords(mem, target,
		0x4000,                 // NEGX.B D0
		0x0280, 0x0000, 0x2000, // ANDI.L #$00002000,D0
		0x4E75, // RTS
	)

	instrs := m68kScanBlock(mem, start)
	if got, want := len(instrs), 1; got != want {
		t.Fatalf("scanned instruction count = %d, want %d", got, want)
	}
	if instrs[0].opcode != 0x4EB9 || instrs[0].fusedFlag != 0 {
		t.Fatalf("scan fused or changed JSR: opcode=%04X fused=%02X", instrs[0].opcode, instrs[0].fusedFlag)
	}
	if got, want := instrs[0].length, uint16(6); got != want {
		t.Fatalf("JSR length = %d, want %d", got, want)
	}
}

func TestM68KJIT_AROSAssignLibraryCallBlockIsProductionNative(t *testing.T) {
	const pc = uint32(0x1000)
	mem := make([]byte, 0x2000)
	words := []uint16{
		0x220B,         // MOVE.L A3,D1
		0x242D, 0xFF6C, // MOVE.L -148(A5),D2
		0x2C6D, 0xFF98, // MOVEA.L -104(A5),A6
		0x4EAE, 0xFD9C, // JSR -612(A6)
		0x4A80, // TST.L D0
		0x661C, // BNE.S
	}
	for i, word := range words {
		addr := pc + uint32(i*2)
		mem[addr] = byte(word >> 8)
		mem[addr+1] = byte(word)
	}

	instrs := m68kScanBlock(mem, pc)
	if got, want := len(instrs), 4; got != want {
		t.Fatalf("scan length=%d, want %d; instrs=%+v", got, want, instrs)
	}
	if got := instrs[len(instrs)-1].opcode; got != 0x4EAE {
		t.Fatalf("last scanned opcode=0x%04X, want JSR 0x4EAE", got)
	}
	// JSR d16(A6) blocks are now production-native (control flow is handled
	// natively), so the full block is admitted.
	if !m68kCanUseProductionNativeBlock(mem, pc, instrs) {
		t.Fatalf("AROS Assign library-call block ending in JSR was not admitted as production native: instrs=%+v", instrs)
	}
}

func TestM68KJIT_AROSSetPatchJSRBlockIsProductionNative(t *testing.T) {
	const pc = uint32(0x1000)
	mem := make([]byte, 0x2000)
	words := []uint16{
		0x7025,         // MOVEQ #37,D0
		0x2C45,         // MOVEA.L D5,A6
		0x4EAE, 0xFDD8, // JSR -552(A6)
	}
	for i, word := range words {
		addr := pc + uint32(i*2)
		mem[addr] = byte(word >> 8)
		mem[addr+1] = byte(word)
	}

	instrs := m68kScanBlock(mem, pc)
	if len(instrs) != 3 || instrs[2].opcode != 0x4EAE {
		t.Fatalf("unexpected SetPatch JSR block scan: %+v", instrs)
	}
	// JSR d16(A6) blocks are now admitted to the production-native path.
	if !m68kCanUseProductionNativeBlock(mem, pc, instrs) {
		t.Fatalf("SetPatch JSR block was not admitted to production native path: %+v", instrs)
	}
}

func TestM68KJIT_WriteGuestBytesInvalidatesCachedBlock(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}

	const pc = uint32(0x1000)
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	if err := cpu.initM68KJIT(); err != nil {
		t.Fatalf("initM68KJIT: %v", err)
	}
	defer cpu.freeM68KJIT()

	writeM68KWords(cpu, pc, 0x7001, 0x7202, 0xD280)
	instrs := m68kScanBlock(cpu.memory, pc)
	block, err := m68kCompileBlockWithMem(instrs, pc, cpu.m68kGetJITExecMem(), cpu.memory)
	if err != nil {
		t.Fatalf("m68kCompileBlockWithMem: %v", err)
	}
	cpu.m68kJitCache.Put(block)
	cpu.m68kMarkJITCodeRanges(block)
	if got := cpu.m68kJitCache.Get(uint64(pc)); got == nil {
		t.Fatal("compiled block was not cached")
	}

	if err := WriteGuestBytes(bus, pc+2, 0, []byte{0x4E, 0x71}); err != nil {
		t.Fatalf("WriteGuestBytes: %v", err)
	}
	if got := cpu.m68kJitCache.Get(uint64(pc)); got != nil {
		t.Fatal("bulk guest write did not invalidate overlapping cached M68K JIT block")
	}
}

func TestM68KJIT_JSRLeafFusionRejectsMemoryNEGX(t *testing.T) {
	mem := make([]byte, 0x2000)
	const target = 0x1000
	beWords(mem, target,
		0x4010, // NEGX.B (A0)
		0x4E75, // RTS
	)

	if _, ok := m68kAnalyzeJSRLeafFusion(mem, target); ok {
		t.Fatalf("memory NEGX leaf was accepted for fusion")
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

	// AROS uses negative library-vector displacements heavily. The
	// displacement word is still part of the instruction, so RTS must return
	// to PC+4, not to the extension word at PC+2.
	beWords(mem, 8, 0x4EAE, 0xFF1C)
	if got := m68kInstrLength(mem, 8); got != 4 {
		t.Errorf("JSR (-228,A6) length: got %d, want 4", got)
	}

	beWords(mem, 16, 0x4EAE, 0xFD9C)
	if got := m68kInstrLength(mem, 16); got != 4 {
		t.Errorf("JSR (-612,A6) length: got %d, want 4", got)
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
	beWords(mem, 0, 0xF180)
	if got := m68kInstrLength(mem, 0); got != 2 {
		t.Errorf("Line F trap-class length: got %d, want 2", got)
	}
	beWords(mem, 0, 0xF200, 0x0000)
	if got := m68kInstrLength(mem, 0); got != 4 {
		t.Errorf("FPU general-op length: got %d, want 4", got)
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

func TestM68KJIT_InstrLength_MOVELImmediateToAnDisplacement(t *testing.T) {
	mem := make([]byte, 64)
	// MOVE.L #$07514483,60(A3)
	beWords(mem, 0, 0x277C)
	beLong(mem, 2, 0x07514483)
	beWords(mem, 6, 0x003C)
	if got := m68kInstrLength(mem, 0); got != 8 {
		t.Fatalf("MOVE.L #imm,d16(An) length: got %d, want 8", got)
	}

	beWords(mem, 8, 0x42AB, 0x005C) // CLR.L 92(A3)
	instrs := m68kScanBlock(mem, 0)
	if len(instrs) < 2 {
		t.Fatalf("scan returned %d instructions, want at least 2", len(instrs))
	}
	if got := instrs[0].length; got != 8 {
		t.Fatalf("first instruction length = %d, want 8", got)
	}
	if got := instrs[1].pcOffset; got != 8 {
		t.Fatalf("second instruction pcOffset = %d, want 8", got)
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
		{0xF180, true, "Line F trap-class"},
		{0xF200, false, "FPU general-op"},
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
		{0xF180, true, "Line F trap-class"},
		{0xF200, false, "FPU general-op"},
		{0x4E72, true, "STOP"},
		{0x4E73, false, "RTE"},
		{0x4E77, true, "RTR"},
		{0x4E70, true, "RESET"},
		{0x4E40, false, "TRAP #0"},
		{0x4E76, true, "TRAPV"},
		{0x4E7A, false, "MOVEC"},
		{0x4C03, false, "MULL.L D3"},
		{0x4C1A, false, "MULL.L (A2)+"},
		{0x4C40, false, "DIVL.L D0"},
		{0x4C5A, false, "DIVL.L (A2)+"},
		{0x4605, false, "NOT.B D5"},
		{0x4641, false, "NOT.W D1"},
		{0x4610, false, "NOT.B (A0)"},
		{0x702A, false, "MOVEQ"},
		{0xD081, false, "ADD.L"},
		{0x4680, false, "NOT.L D0"},
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
	pageMin := make([]uint16, len(bitmap))
	pageMax := make([]uint16, len(bitmap))
	ctx := newM68KJITContext(cpu, bitmap, pageMin, pageMax)

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
		{"RTSCache2PC", uintptr(unsafe.Pointer(&ctx.RTSCache2PC)) - base, m68kCtxOffRTSCache2PC},
		{"RTSCache2Addr", uintptr(unsafe.Pointer(&ctx.RTSCache2Addr)) - base, m68kCtxOffRTSCache2Addr},
		{"RTSCache3PC", uintptr(unsafe.Pointer(&ctx.RTSCache3PC)) - base, m68kCtxOffRTSCache3PC},
		{"RTSCache3Addr", uintptr(unsafe.Pointer(&ctx.RTSCache3Addr)) - base, m68kCtxOffRTSCache3Addr},
		{"RTSCache4PC", uintptr(unsafe.Pointer(&ctx.RTSCache4PC)) - base, m68kCtxOffRTSCache4PC},
		{"RTSCache4Addr", uintptr(unsafe.Pointer(&ctx.RTSCache4Addr)) - base, m68kCtxOffRTSCache4Addr},
		{"RTSCache5PC", uintptr(unsafe.Pointer(&ctx.RTSCache5PC)) - base, m68kCtxOffRTSCache5PC},
		{"RTSCache5Addr", uintptr(unsafe.Pointer(&ctx.RTSCache5Addr)) - base, m68kCtxOffRTSCache5Addr},
		{"RTSCache6PC", uintptr(unsafe.Pointer(&ctx.RTSCache6PC)) - base, m68kCtxOffRTSCache6PC},
		{"RTSCache6Addr", uintptr(unsafe.Pointer(&ctx.RTSCache6Addr)) - base, m68kCtxOffRTSCache6Addr},
		{"RTSCache7PC", uintptr(unsafe.Pointer(&ctx.RTSCache7PC)) - base, m68kCtxOffRTSCache7PC},
		{"RTSCache7Addr", uintptr(unsafe.Pointer(&ctx.RTSCache7Addr)) - base, m68kCtxOffRTSCache7Addr},
		{"IOPageBitmapPtr", uintptr(unsafe.Pointer(&ctx.IOPageBitmapPtr)) - base, m68kCtxOffIOPageBitmapPtr},
		{"IOPageBitmapLen", uintptr(unsafe.Pointer(&ctx.IOPageBitmapLen)) - base, m68kCtxOffIOPageBitmapLen},
		{"USPPtr", uintptr(unsafe.Pointer(&ctx.USPPtr)) - base, m68kCtxOffUSPPtr},
		{"SSPPtr", uintptr(unsafe.Pointer(&ctx.SSPPtr)) - base, m68kCtxOffSSPPtr},
		{"NativeException", uintptr(unsafe.Pointer(&ctx.NativeException)) - base, m68kCtxOffNativeException},
		{"NativeExceptionPC", uintptr(unsafe.Pointer(&ctx.NativeExceptionPC)) - base, m68kCtxOffNativeExceptionPC},
		{"NativeExceptionIR", uintptr(unsafe.Pointer(&ctx.NativeExceptionIR)) - base, m68kCtxOffNativeExceptionIR},
		{"NeedHelper", uintptr(unsafe.Pointer(&ctx.NeedHelper)) - base, m68kCtxOffNeedHelper},
		{"HelperPC", uintptr(unsafe.Pointer(&ctx.HelperPC)) - base, m68kCtxOffHelperPC},
		{"CodePageMinPtr", uintptr(unsafe.Pointer(&ctx.CodePageMinPtr)) - base, m68kCtxOffCodePageMinPtr},
		{"CodePageMaxPtr", uintptr(unsafe.Pointer(&ctx.CodePageMaxPtr)) - base, m68kCtxOffCodePageMaxPtr},
		{"CodePageBoundsLen", uintptr(unsafe.Pointer(&ctx.CodePageBoundsLen)) - base, m68kCtxOffCodePageBoundsLen},
		{"InExceptionPtr", uintptr(unsafe.Pointer(&ctx.InExceptionPtr)) - base, m68kCtxOffInExceptionPtr},
		{"RTECountPtr", uintptr(unsafe.Pointer(&ctx.RTECountPtr)) - base, m68kCtxOffRTECountPtr},
		{"PendingExceptionPtr", uintptr(unsafe.Pointer(&ctx.PendingExceptionPtr)) - base, m68kCtxOffPendingExceptionPtr},
		{"PendingInterruptPtr", uintptr(unsafe.Pointer(&ctx.PendingInterruptPtr)) - base, m68kCtxOffPendingInterruptPtr},
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
	gotDisp := mustExecRel32(t, patchAddr)
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

func TestM68KJIT_CodeCacheUnpatchesCoveredRangeMidBlockTarget(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatal(err)
	}
	defer em.Free()

	codeA := []byte{
		0x90,
		0xE9, 0x00, 0x00, 0x00, 0x00,
	}
	addrA, err := em.Write(codeA)
	if err != nil {
		t.Fatal(err)
	}
	patchAddr := addrA + 2

	codeB := []byte{0x90, 0xC3}
	addrB, err := em.Write(codeB)
	if err != nil {
		t.Fatal(err)
	}

	cc := NewCodeCache()
	cc.Put(&JITBlock{
		startPC:    0x1000,
		endPC:      0x1006,
		execAddr:   addrA,
		execSize:   len(codeA),
		chainSlots: []chainSlot{{targetPC: 0x2004, patchAddr: patchAddr}},
	})
	cc.Put(&JITBlock{
		startPC:       0x2000,
		endPC:         0x2008,
		execAddr:      addrB,
		execSize:      len(codeB),
		chainEntry:    addrB,
		coveredRanges: [][2]uint64{{0x2000, 0x2008}},
	})

	PatchRel32At(patchAddr, addrB)
	if got := mustExecRel32(t, patchAddr); got == 0 {
		t.Fatalf("test setup did not patch chain slot")
	}

	cc.UnpatchChainsInRange(0x2002, 0x2006)
	if got := mustExecRel32(t, patchAddr); got != 0 {
		t.Fatalf("chain slot targeting covered range mid-block displacement=%d, want 0", got)
	}
}
