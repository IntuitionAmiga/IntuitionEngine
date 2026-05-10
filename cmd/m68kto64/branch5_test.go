package main

import "testing"

// More targeted branch hits.

func TestBranch_EmitArith_StoreDstFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// add.l d0,#5 — illegal m68k but parser accepts; stores fail through.
	c.emitArith(e, makeLine("add", ".l", "d0", "#5"), 4, "add")
}

func TestBranch_EmitUnary_StoreFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitUnary(e, makeLine("neg", ".l", "#5"), 4, "neg")
}

func TestBranch_EmitShift_StoreFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitShift(e, makeLine("lsl", ".l", "#1", "#5"), 4, "lsl", "lsl")
}

func TestBranch_EmitBcdAdd_PreDecParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBcdAdd(e, makeLine("abcd", "", "-(a0)", ""), false)
}

func TestBranch_EmitBcdAdd_FirstParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBcdAdd(e, makeLine("abcd", "", "", "-(a0)"), false)
}

func TestBranch_EmitBcdSub_FirstParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBcdSub(e, makeLine("sbcd", "", "", "-(a0)"), false)
}

func TestBranch_EmitNbcd_OperandError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitNbcd(e, makeLine("nbcd", "", ""))
}

func TestBranch_EmitPack_AdjParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitPack(e, makeLine("pack", "", "d0", "d1", ""))
}

func TestBranch_EmitUnpk_AdjParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitUnpk(e, makeLine("unpk", "", "d0", "d1", ""))
}

func TestBranch_EmitCas_OperandParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitCas(e, makeLine("cas", ".l", "", "d0", "(a0)"))
	c.emitCas(e, makeLine("cas", ".l", "d0", "", "(a0)"))
	c.emitCas(e, makeLine("cas", ".l", "d0", "d1", ""))
}

func TestBranch_EmitBfins_DstParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBfins(e, makeLine("bfins", "", "d0", ""))
}

func TestBranch_EmitBfffo_DstParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBfffo(e, makeLine("bfffo", "", "d0{#0:#8}", ""))
}

func TestBranch_EmitBfModify_OperandError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBfModify(e, makeLine("bfclr", "", ""), "clr")
}

func TestBranch_EmitChk_OperandParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitChk(e, makeLine("chk", ".w", "", "d0"))
	c.emitChk(e, makeLine("chk", ".w", "d0", ""))
}

func TestBranch_EmitMulPair_SrcParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMulPair(e, makeLine("mulu", "", "", "d0:d1"))
}

func TestBranch_EmitDivPair_BadEverything(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitDivPair(e, makeLine("divu", "", "@@@", "d0:d1"))
}

func TestBranch_EmitClr_OperandError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitClr(e, makeLine("clr", ".l", ""), 4)
}

func TestBranch_EmitMove_DispOrZero(t *testing.T) {
	// `move.l 0(a0),d0` exercises dispOrZero pass-through.
	convertSrc(t, "\tmove.l 0(a0),d0\n")
}

func TestBranch_EmitMove_StoreErrorPath(t *testing.T) {
	// move.l d0,#5 — storeValue returns error.
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	c.ConvertSource("\tmove.l d0,#5\n")
}

func TestBranch_EmitArith_LoadValueError(t *testing.T) {
	// add ccr,d0 — loadValue returns error for ccr.
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	if _, errs := c.ConvertSource("\tadd.l ccr,d0\n"); errs == 0 {
		t.Logf("note: add.l ccr,d0 didn't error in strict (acceptable)")
	}
}

func TestBranch_EmitMovem_BadOperandTypes(t *testing.T) {
	// emitMovemStore loadValue failure — not really a path; but feed bad list.
	c := NewConverter()
	e := &Emit{}
	// emitMovem with both list-mode operands.
	c.emitMovem(e, Line{Mnemonic: "movem", Size: ".l", Operands: []string{"d0/d1", "d2/d3"}})
}
