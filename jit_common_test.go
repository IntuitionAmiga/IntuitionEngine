// jit_common_test.go - Tests for block scanner, CodeBuffer, and CodeCache

//go:build (amd64 || arm64) && linux

package main

import (
	"testing"
)

// ===========================================================================
// Block Scanner Tests
// ===========================================================================

func TestScanBlock_ALUThenHalt(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()

	// 3 ALU instructions + HALT = 4 instructions
	offset := uint32(PROG_START)
	copy(mem[offset:], ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 2, 0, 42))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_SUB, 3, IE64_SIZE_Q, 1, 1, 0, 10))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_AND64, 4, IE64_SIZE_Q, 0, 1, 3, 0))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	instrs := scanBlock(mem, PROG_START)
	if len(instrs) != 4 {
		t.Fatalf("expected 4 instructions, got %d", len(instrs))
	}
	if instrs[0].opcode != OP_ADD {
		t.Fatalf("instrs[0].opcode = 0x%02X, want 0x%02X", instrs[0].opcode, OP_ADD)
	}
	if instrs[3].opcode != OP_HALT64 {
		t.Fatalf("instrs[3].opcode = 0x%02X, want 0x%02X", instrs[3].opcode, OP_HALT64)
	}
	if instrs[2].pcOffset != 16 {
		t.Fatalf("instrs[2].pcOffset = %d, want 16", instrs[2].pcOffset)
	}
}

func TestScanBlock_BRATerminates(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()

	offset := uint32(PROG_START)
	copy(mem[offset:], ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 0, 0, 100))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, 16)) // unconditional branch terminates
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_ADD, 3, IE64_SIZE_Q, 1, 0, 0, 200)) // should NOT be in block

	instrs := scanBlock(mem, PROG_START)
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(instrs))
	}
	if instrs[1].opcode != OP_BRA {
		t.Fatalf("instrs[1].opcode = 0x%02X, want 0x%02X", instrs[1].opcode, OP_BRA)
	}
}

func TestScanBlock_ConditionalBranchNotTerminator(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()

	// Conditional branches are NOT terminators (they have fall-through paths)
	offset := uint32(PROG_START)
	copy(mem[offset:], ie64Instr(OP_ADD, 1, IE64_SIZE_Q, 1, 0, 0, 100))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_BEQ, 0, 0, 0, 1, 2, 16))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_ADD, 3, IE64_SIZE_Q, 1, 0, 0, 200))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	instrs := scanBlock(mem, PROG_START)
	if len(instrs) != 4 {
		t.Fatalf("expected 4 instructions (BEQ does not terminate), got %d", len(instrs))
	}
}

func TestScanBlock_FPUNotTerminator(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()

	// FPU ops are no longer terminators — they should be included in blocks
	offset := uint32(PROG_START)
	copy(mem[offset:], ie64Instr(OP_FADD, 1, 0, 0, 2, 3, 0))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_FSUB, 2, 0, 0, 1, 3, 0))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	instrs := scanBlock(mem, PROG_START)
	if len(instrs) != 3 {
		t.Fatalf("expected 3 instructions (FPU + HALT terminator), got %d", len(instrs))
	}
	if instrs[0].opcode != OP_FADD {
		t.Fatalf("instrs[0].opcode = 0x%02X, want 0x%02X", instrs[0].opcode, OP_FADD)
	}
	if instrs[1].opcode != OP_FSUB {
		t.Fatalf("instrs[1].opcode = 0x%02X, want 0x%02X", instrs[1].opcode, OP_FSUB)
	}
}

func TestScanBlock_MaxSize(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()

	// Fill memory with NOPs (non-terminating) — block should cap at 256
	for i := 0; i < 300; i++ {
		offset := uint32(PROG_START) + uint32(i)*IE64_INSTR_SIZE
		if offset+IE64_INSTR_SIZE > uint32(len(mem)) {
			break
		}
		copy(mem[offset:], ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))
	}

	instrs := scanBlock(mem, PROG_START)
	if len(instrs) != jitMaxBlockSize {
		t.Fatalf("expected %d instructions (max block size), got %d", jitMaxBlockSize, len(instrs))
	}
}

func TestScanBlock_DecodesFields(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()

	// ADD.L R5, R3, #0xDEAD
	copy(mem[PROG_START:], ie64Instr(OP_ADD, 5, IE64_SIZE_L, 1, 3, 0, 0xDEAD))
	copy(mem[PROG_START+IE64_INSTR_SIZE:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	instrs := scanBlock(mem, PROG_START)
	if len(instrs) != 2 {
		t.Fatalf("expected 2 instructions, got %d", len(instrs))
	}

	ji := instrs[0]
	if ji.opcode != OP_ADD {
		t.Errorf("opcode = 0x%02X, want 0x%02X", ji.opcode, OP_ADD)
	}
	if ji.rd != 5 {
		t.Errorf("rd = %d, want 5", ji.rd)
	}
	if ji.size != IE64_SIZE_L {
		t.Errorf("size = %d, want %d", ji.size, IE64_SIZE_L)
	}
	if ji.xbit != 1 {
		t.Errorf("xbit = %d, want 1", ji.xbit)
	}
	if ji.rs != 3 {
		t.Errorf("rs = %d, want 3", ji.rs)
	}
	if ji.imm32 != 0xDEAD {
		t.Errorf("imm32 = 0x%X, want 0xDEAD", ji.imm32)
	}
}

// ===========================================================================
// needsFallback Tests
// ===========================================================================

func TestNeedsFallback_FPU_Compilable(t *testing.T) {
	// FADD is now JIT-compiled, should NOT need fallback
	instrs := []JITInstr{{opcode: OP_FADD}}
	if needsFallback(instrs) {
		t.Fatal("FADD should NOT need fallback (now JIT-compiled)")
	}
}

func TestNeedsFallback_FPU_Transcendental(t *testing.T) {
	// Transcendentals still need fallback when they're the sole instruction
	for _, op := range []byte{OP_FMOD, OP_FSIN, OP_FCOS, OP_FTAN, OP_FATAN, OP_FLOG, OP_FEXP, OP_FPOW} {
		instrs := []JITInstr{{opcode: op}}
		if !needsFallback(instrs) {
			t.Fatalf("opcode 0x%02X should need fallback (transcendental)", op)
		}
	}
}

func TestNeedsFallback_HALT(t *testing.T) {
	instrs := []JITInstr{{opcode: OP_HALT64}}
	if !needsFallback(instrs) {
		t.Fatal("HALT should need fallback")
	}
}

func TestNeedsFallback_WAIT(t *testing.T) {
	instrs := []JITInstr{{opcode: OP_WAIT64}}
	if !needsFallback(instrs) {
		t.Fatal("WAIT should need fallback")
	}
}

func TestNeedsFallback_ALU(t *testing.T) {
	instrs := []JITInstr{{opcode: OP_ADD}}
	if needsFallback(instrs) {
		t.Fatal("ALU instruction should NOT need fallback")
	}
}

func TestNeedsFallback_Empty(t *testing.T) {
	if !needsFallback(nil) {
		t.Fatal("empty block should need fallback")
	}
}

func TestNeedsFallback_BranchBlock(t *testing.T) {
	instrs := []JITInstr{
		{opcode: OP_ADD},
		{opcode: OP_BEQ},
	}
	if needsFallback(instrs) {
		t.Fatal("block starting with ALU should NOT need fallback even if branch follows")
	}
}

// ===========================================================================
// CodeBuffer Tests
// ===========================================================================

func TestCodeBuffer_Emit32(t *testing.T) {
	cb := NewCodeBuffer(64)
	cb.Emit32(0x12345678)

	bytes := cb.Bytes()
	if len(bytes) != 4 {
		t.Fatalf("expected 4 bytes, got %d", len(bytes))
	}
	// Little-endian
	if bytes[0] != 0x78 || bytes[1] != 0x56 || bytes[2] != 0x34 || bytes[3] != 0x12 {
		t.Fatalf("bytes = %X, want 78563412", bytes)
	}
}

func TestCodeBuffer_EmitBytes(t *testing.T) {
	cb := NewCodeBuffer(64)
	cb.EmitBytes(0x48, 0x89, 0xC3) // MOV RBX, RAX

	bytes := cb.Bytes()
	if len(bytes) != 3 {
		t.Fatalf("expected 3 bytes, got %d", len(bytes))
	}
	if bytes[0] != 0x48 || bytes[1] != 0x89 || bytes[2] != 0xC3 {
		t.Fatalf("bytes = %X, want 4889C3", bytes)
	}
}

func TestCodeBuffer_Emit64(t *testing.T) {
	cb := NewCodeBuffer(64)
	cb.Emit64(0x0102030405060708)

	bytes := cb.Bytes()
	if len(bytes) != 8 {
		t.Fatalf("expected 8 bytes, got %d", len(bytes))
	}
	if bytes[0] != 0x08 || bytes[7] != 0x01 {
		t.Fatalf("bytes = %X, want 0807060504030201", bytes)
	}
}

func TestCodeBuffer_ForwardLabel(t *testing.T) {
	cb := NewCodeBuffer(64)

	// Emit a fixup referencing a forward label
	cb.EmitBytes(0x90) // NOP placeholder (1 byte)
	pcBase := cb.Len()
	cb.FixupRel32("target", pcBase)

	// Emit some padding
	cb.EmitBytes(0x90, 0x90, 0x90, 0x90) // 4 more NOPs

	// Place the label
	cb.Label("target")

	// Resolve
	cb.Resolve()

	// The fixup at offset 1 should contain the relative offset from pcBase to target
	// pcBase = 1, target = 1 + 4(fixup placeholder) + 4(NOPs) = 9
	// rel = 9 - 1 = 8
	bytes := cb.Bytes()
	rel := int32(uint32(bytes[1]) | uint32(bytes[2])<<8 | uint32(bytes[3])<<16 | uint32(bytes[4])<<24)
	if rel != 8 {
		t.Fatalf("fixup relative offset = %d, want 8", rel)
	}
}

func TestCodeBuffer_Len(t *testing.T) {
	cb := NewCodeBuffer(64)
	if cb.Len() != 0 {
		t.Fatalf("empty buffer Len = %d, want 0", cb.Len())
	}
	cb.Emit32(0)
	if cb.Len() != 4 {
		t.Fatalf("after Emit32 Len = %d, want 4", cb.Len())
	}
	cb.EmitBytes(0x90, 0x90)
	if cb.Len() != 6 {
		t.Fatalf("after EmitBytes Len = %d, want 6", cb.Len())
	}
}

func TestCodeBuffer_PatchUint32(t *testing.T) {
	cb := NewCodeBuffer(64)
	cb.Emit32(0x00000000) // placeholder
	cb.PatchUint32(0, 0xAABBCCDD)

	bytes := cb.Bytes()
	if bytes[0] != 0xDD || bytes[1] != 0xCC || bytes[2] != 0xBB || bytes[3] != 0xAA {
		t.Fatalf("patched bytes = %X, want DDCCBBAA", bytes)
	}
}

// ===========================================================================
// Code Cache Tests
// ===========================================================================

func TestCodeCache_PutGet(t *testing.T) {
	cc := NewCodeCache()
	block := &JITBlock{startPC: 0x1000, endPC: 0x1020, instrCount: 4}
	cc.Put(block)

	got := cc.Get(0x1000)
	if got != block {
		t.Fatal("expected to retrieve stored block")
	}
	if got.instrCount != 4 {
		t.Fatalf("instrCount = %d, want 4", got.instrCount)
	}
}

func TestCodeCache_GetMiss(t *testing.T) {
	cc := NewCodeCache()
	got := cc.Get(0x2000)
	if got != nil {
		t.Fatal("expected nil for cache miss")
	}
}

func TestCodeCache_Invalidate(t *testing.T) {
	cc := NewCodeCache()
	cc.Put(&JITBlock{startPC: 0x1000, endPC: 0x1020})
	cc.Put(&JITBlock{startPC: 0x2000, endPC: 0x2020})
	cc.Invalidate()

	if cc.Get(0x1000) != nil || cc.Get(0x2000) != nil {
		t.Fatal("cache should be empty after Invalidate")
	}
}

func TestCodeCache_InvalidateRange(t *testing.T) {
	cc := NewCodeCache()
	cc.Put(&JITBlock{startPC: 0x1000, endPC: 0x1020})
	cc.Put(&JITBlock{startPC: 0x2000, endPC: 0x2020})
	cc.Put(&JITBlock{startPC: 0x3000, endPC: 0x3020})

	// Invalidate range that overlaps with 0x2000-0x2020
	cc.InvalidateRange(0x2010, 0x2030)

	if cc.Get(0x1000) == nil {
		t.Fatal("block at 0x1000 should survive")
	}
	if cc.Get(0x2000) != nil {
		t.Fatal("block at 0x2000 should be invalidated")
	}
	if cc.Get(0x3000) == nil {
		t.Fatal("block at 0x3000 should survive")
	}
}

func TestCodeCache_InvalidateRange_NoOverlap(t *testing.T) {
	cc := NewCodeCache()
	cc.Put(&JITBlock{startPC: 0x1000, endPC: 0x1020})

	cc.InvalidateRange(0x2000, 0x3000) // doesn't overlap
	if cc.Get(0x1000) == nil {
		t.Fatal("block should survive non-overlapping invalidation")
	}
}

func TestCodeCache_Replace(t *testing.T) {
	cc := NewCodeCache()
	b1 := &JITBlock{startPC: 0x1000, endPC: 0x1020, instrCount: 4}
	b2 := &JITBlock{startPC: 0x1000, endPC: 0x1040, instrCount: 8}
	cc.Put(b1)
	cc.Put(b2)

	got := cc.Get(0x1000)
	if got.instrCount != 8 {
		t.Fatalf("expected replaced block with instrCount=8, got %d", got.instrCount)
	}
}

// ===========================================================================
// detectBackwardBranches Tests
// ===========================================================================

func TestDetectBackwardBranches_Present(t *testing.T) {
	// Block: MOVE R1, #5000; SUB R1, R1, #1; BNE R1, R0, -8 (targets SUB)
	bus := NewMachineBus()
	mem := bus.GetMemory()
	neg8 := uint32(0xFFFFFFF8) // -8 as uint32

	offset := uint32(PROG_START)
	copy(mem[offset:], ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 5000))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 1, 0, 1))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 1, 0, neg8))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	instrs := scanBlock(mem, PROG_START)
	if !detectBackwardBranches(instrs, PROG_START) {
		t.Fatal("expected detectBackwardBranches to return true for BNE backward branch")
	}
}

func TestDetectBackwardBranches_Absent(t *testing.T) {
	// Block: MOVE R1, #10; BEQ R1, R0, +16 (forward); ADD R3, R1, #1; HALT
	bus := NewMachineBus()
	mem := bus.GetMemory()

	offset := uint32(PROG_START)
	copy(mem[offset:], ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 10))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_BEQ, 0, IE64_SIZE_Q, 0, 1, 0, 16))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_ADD, 3, IE64_SIZE_Q, 1, 1, 0, 1))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	instrs := scanBlock(mem, PROG_START)
	if detectBackwardBranches(instrs, PROG_START) {
		t.Fatal("expected detectBackwardBranches to return false for forward-only branches")
	}
}

func TestDetectBackwardBranches_BRA(t *testing.T) {
	// Block: ADD R2, R2, #1; SUB R1, R1, #1; BNE forward; BRA -24 (targets ADD at startPC)
	bus := NewMachineBus()
	mem := bus.GetMemory()
	neg24 := uint32(0xFFFFFFE8) // -24 as uint32

	offset := uint32(PROG_START)
	copy(mem[offset:], ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 2, 0, 1))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_SUB, 1, IE64_SIZE_Q, 1, 1, 0, 1))
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_BNE, 0, IE64_SIZE_Q, 0, 1, 0, 16)) // forward
	offset += IE64_INSTR_SIZE
	copy(mem[offset:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, neg24)) // backward to startPC

	instrs := scanBlock(mem, PROG_START)
	if !detectBackwardBranches(instrs, PROG_START) {
		t.Fatal("expected detectBackwardBranches to return true for BRA backward branch")
	}
}
