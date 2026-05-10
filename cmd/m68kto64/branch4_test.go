package main

import (
	"testing"
)

func TestBranch_EmitBtst_BitOpParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBtst(e, makeLine("btst", "", "", "d0"), 4)
}

func TestBranch_EmitSetByte_ParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitSetByte(e, makeLine("st", "", ""), true)
	c.emitSetByte(e, makeLine("st", "", ""), false)
}

func TestBranch_EmitMovem_SizeZero(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMovem(e, Line{Mnemonic: "movem", Size: ".q", Operands: []string{"d0/d1", "(a0)"}})
}

func TestBranch_EmitMovem_OperandParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMovem(e, Line{Mnemonic: "movem", Size: ".l", Operands: []string{"", "(a0)"}})
	c.emitMovem(e, Line{Mnemonic: "movem", Size: ".l", Operands: []string{"d0/d1", ""}})
}

func TestBranch_EmitMovemLoad_WordPredec(t *testing.T) {
	// movem.w postinc — sign-extend branch.
	convertSrc(t, "\tmovem.w (sp)+,d0/d2/d4\n")
}

func TestBranch_EmitMovemLoad_WordForward(t *testing.T) {
	// movem.w with non-postinc EA — exercises the forward-mode .w sign-ext.
	convertSrc(t, "\tmovem.w 8(a0),d0/d1\n")
	convertSrc(t, "\tmovem.w (a0),d0/d1\n")
}

func TestBranch_EmitMovemStore_Word(t *testing.T) {
	convertSrc(t, "\tmovem.w d0/d1,8(a0)\n")
	convertSrc(t, "\tmovem.w d0/d1,-(sp)\n")
}

func TestBranch_EmitEABase_PCIndexCombine(t *testing.T) {
	out := convertSrc(t, "\tmovem.l (label,pc,d0.l*4),d0/d1\n")
	mustContain(t, out, "la r16, label")
}

func TestBranch_EmitEABase_AMUnknown(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	if err := c.emitEABase(e, Operand{Mode: AMUnknown}, ScrEA); err == nil {
		t.Errorf("expected error for AMUnknown")
	}
}

func TestBranch_EmitMovemStore_EAFailure(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// regs to forward-mode dst with bad EA (drives emitEABase failure).
	c.emitMovemStore(e, []string{"r1"}, Operand{Mode: AMUnknown}, 4, ".l")
}

func TestBranch_EmitMovemLoad_EAFailure(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMovemLoad(e, []string{"r1"}, Operand{Mode: AMUnknown}, 4, ".l")
}

func TestBranch_EmitArith_AddiSubiOiEoriAndi(t *testing.T) {
	// Drive aliases.
	for _, src := range []string{
		"\taddi.l #5,d0\n",
		"\tsubi.l #5,d0\n",
		"\taddq.l #1,d0\n",
		"\tsubq.l #1,d0\n",
		"\tandi.l #$F,d0\n",
		"\tori.l #$F,d0\n",
		"\teori.l #$F,d0\n",
	} {
		convertSrc(t, src)
	}
}

func TestBranch_EmitMul_OperandParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMulW(e, makeLine("mulu", ".w", "", "d0"), false)
	c.emitMulW(e, makeLine("mulu", ".w", "d0", ""), false)
}

func TestBranch_EmitDivW_DstParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitDivW(e, makeLine("divu", ".w", "d0", ""), false)
	c.emitDivW(e, makeLine("divu", ".w", "", "d0"), false)
}

func TestBranch_EmitMulPair_SrcImm(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMulPair(e, makeLine("mulu", "", "#100", "d0:d1"))
}

func TestBranch_EmitDivPair_SrcImm(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitDivPair(e, makeLine("divu", "", "#100", "d0:d1"))
}

func TestBranch_EmitDivPair_SrcParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitDivPair(e, makeLine("divu", "", "", "d0:d1"))
}

func TestBranch_EmitDivPair_PairParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitDivPair(e, makeLine("divu", "", "d0", "d0:foo"))
}

func TestBranch_EmitMulPair_PairParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMulPair(e, makeLine("mulu", "", "d0", "d0:foo"))
}

func TestBranch_EmitMulPair_BadSplitColon(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// splitPair with no colon error path.
	c.emitMulPair(e, makeLine("mulu", "", "d0", "d0d1"))
}

func TestBranch_EmitClrMem_StoreError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// emitClr storeValue path: AMImmediate dst -> store error.
	c.emitClr(e, Line{Mnemonic: "clr", Size: ".l", Operands: []string{"#5"}}, 4)
}

func TestBranch_EmitArith_AddaWidthDispatch(t *testing.T) {
	// adda .l reg src.
	convertSrc(t, "\tadda.l d0,a0\n")
}

func TestBranch_EmitMove_ImmToMemSize2(t *testing.T) {
	convertSrc(t, "\tmove.w #$1234,(a0)\n")
}

func TestBranch_EmitArith_SrcImmRegSwap(t *testing.T) {
	// Drive `move%s ScrV1, #imm` materialise path.
	convertSrc(t, "\tand.l #$F,d0\n")  // no shadow C/V wanted (logical), so srcImm stays imm.
}

func TestBranch_EmitFusedCmpBcc_OperandParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	cons := Line{Mnemonic: "beq", Operands: []string{"L"}}
	c.emitFusedCmpBcc(e, makeLine("cmpa", ".w", "", "a0"), cons)
}

func TestBranch_EmitFusedCmpBcc_DstParseFailure(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// Drive dst-load failure inside cmpa branch.
	cons := Line{Mnemonic: "beq", Operands: []string{"L"}}
	c.emitFusedCmpBcc(e, makeLine("cmpa", ".l", "(a0)", "(badexpr)"), cons)
}

func TestBranch_EmitMove_AnAn_FastPath_EmitsShadows(t *testing.T) {
	// move.l a0,a1 — fast path exit, emits shadows.
	out := convertSrc(t, "\tmove.l a0,a1\n")
	mustContain(t, out, "sext.l r24")
}

func TestBranch_EmitMove_LongMemSrcDn(t *testing.T) {
	convertSrc(t, "\tmove.l (a0),d0\n")
}

func TestBranch_EmitArith_AdditionalForms(t *testing.T) {
	convertSrc(t, "\tadd.l 8(a0,d0.l*4),d1\n")
	convertSrc(t, "\tadd.l (8,a0),d1\n")
	convertSrc(t, "\tadd.l ($F2000).l,d1\n")
}

func TestBranch_EmitFusedTstBcc_HiLs(t *testing.T) {
	// hi/ls cases of emitFusedTstBcc.
	convertSrc(t, "\ttst.l d0\n\tbhi L\nL:\n\trts\n")
	convertSrc(t, "\ttst.l d0\n\tbls L\nL:\n\trts\n")
}
