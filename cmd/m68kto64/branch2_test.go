package main

import (
	"testing"
)

// Direct-call sweep: drive each `if err != nil { return err }` branch in
// the per-mnemonic emit functions by feeding malformed operands constructed
// directly as Line values. The transpiler is forgiving in non-strict mode,
// but the err-return paths only fire when ParseOperand or len-check fails.

// makeLine builds a Line with the given mnemonic + size + raw operands.
func makeLine(mnem string, sizeAndOps ...string) Line {
	size := ""
	var ops []string
	if len(sizeAndOps) > 0 {
		size = sizeAndOps[0]
		ops = sizeAndOps[1:]
	}
	return Line{Kind: LineInstruction, Mnemonic: mnem, Size: size, Operands: ops}
}

func TestBranch_EmitMove_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// Bad src.
	c.emitMove(e, makeLine("move", ".l", "", "d0"), 4)
	// Bad dst.
	c.emitMove(e, makeLine("move", ".l", "d0", ""), 4)
	// Moveq bad operands.
	c.emitMove(e, makeLine("moveq", "", "d0", "d1"), 4) // src not imm
	c.emitMove(e, makeLine("moveq", "", "#5", "(a0)"), 4)
	c.emitMove(e, makeLine("moveq", "", "", "d0"), 4)
	c.emitMove(e, makeLine("moveq", "", "#5", ""), 4)
	c.emitMove(e, makeLine("moveq", "", "#5"), 4)
	// movea bad src/dst.
	c.emitMove(e, makeLine("movea", ".l", "", "a0"), 4)
	c.emitMove(e, makeLine("movea", ".l", "d0", ""), 4)
	// movea bad size.
	c.emitMove(e, makeLine("movea", ".b", "d0", "a0"), 1)
}

func TestBranch_EmitLea_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitLea(e, makeLine("lea", "", "", "a0"))
	c.emitLea(e, makeLine("lea", "", "(a0)", ""))
	c.emitLea(e, makeLine("lea", "", "(a0)")) // count
	c.emitLea(e, makeLine("lea", "", "d0", "a0")) // unsupported src mode
}

func TestBranch_EmitArith_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitArith(e, makeLine("add", ".l", "", "d0"), 4, "add")
	c.emitArith(e, makeLine("add", ".l", "d0", ""), 4, "add")
	c.emitArith(e, makeLine("add", ".l"), 4, "add")
	// Drive divu/divs paths.
	c.emitArith(e, makeLine("divu", ".l", "", "d0"), 4, "divu")
}

func TestBranch_EmitUnary_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitUnary(e, makeLine("neg", ".l"), 4, "neg")
	c.emitUnary(e, makeLine("neg", ".l", ""), 4, "neg")
}

func TestBranch_EmitClr_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitClr(e, makeLine("clr", ".l"), 4)
	c.emitClr(e, makeLine("clr", ".l", ""), 4)
}

func TestBranch_EmitShift_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitShift(e, makeLine("lsl", ".l"), 4, "lsl", "lsl")
	c.emitShift(e, makeLine("lsl", ".l", "#1", ""), 4, "lsl", "lsl")
	c.emitShift(e, makeLine("lsl", ".l", "", "d0"), 4, "lsl", "lsl")
	// Bad count operand mode.
	c.emitShift(e, makeLine("lsl", ".l", "(a0)", "d0"), 4, "lsl", "lsl")
	// Single-operand bad.
	c.emitShift(e, makeLine("lsl", ".l", ""), 4, "lsl", "lsl")
}

func TestBranch_EmitExt_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitExt(e, makeLine("ext", ".w"), 2, false)
	c.emitExt(e, makeLine("ext", ".w", ""), 2, false)
	c.emitExt(e, makeLine("ext", ".b", "d0"), 1, false) // bad size for ext
	c.emitExt(e, makeLine("ext", ".w", "(a0)"), 2, false) // not Dn
}

func TestBranch_EmitSwap_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitSwap(e, makeLine("swap"))
	c.emitSwap(e, makeLine("swap", "", ""))
}

func TestBranch_EmitSetByte_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitSetByte(e, makeLine("st"), true)
	c.emitSetByte(e, makeLine("st", "", ""), true)
}

func TestBranch_EmitBtst_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBtst(e, makeLine("btst"), 4)
	c.emitBtst(e, makeLine("btst", "", ""), 4)
	c.emitBtst(e, makeLine("btst", "", "#5", ""), 4)
}

func TestBranch_EmitJmp_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitJmp(e, makeLine("jmp"))
	c.emitJmp(e, makeLine("jmp", "", ""))
	c.emitJmp(e, makeLine("jmp", "", "d0")) // unsupported mode
	c.emitJmp(e, makeLine("jmp", "", "(label,pc)")) // pc-rel goes through bra
}

func TestBranch_EmitJsr_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitJsr(e, makeLine("jsr"))
	c.emitJsr(e, makeLine("jsr", "", ""))
	c.emitJsr(e, makeLine("jsr", "", "d0")) // unsupported mode
}

func TestBranch_EmitLink_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitLink(e, makeLine("link"))
	c.emitLink(e, makeLine("link", "", "", "#0"))
	c.emitLink(e, makeLine("link", "", "a0", ""))
	c.emitLink(e, makeLine("link", "", "d0", "#0"))
	c.emitLink(e, makeLine("link", "", "a0", "d0")) // 2nd not imm
}

func TestBranch_EmitUnlk_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitUnlk(e, makeLine("unlk"))
	c.emitUnlk(e, makeLine("unlk", "", ""))
	c.emitUnlk(e, makeLine("unlk", "", "d0"))
}

func TestBranch_EmitDbra_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitDbra(e, makeLine("dbra"))
	c.emitDbra(e, makeLine("dbra", "", "", "L"))
	c.emitDbra(e, makeLine("dbra", "", "a0", "L"))
}

func TestBranch_EmitBra_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBra(e, makeLine("bra"))
}

func TestBranch_EmitBsr_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBsr(e, makeLine("bsr"))
}

func TestBranch_EmitMovem_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMovem(e, makeLine("movem"))
	c.emitMovem(e, makeLine("movem", ".b", "d0/d1", "(a0)"))
	c.emitMovem(e, makeLine("movem", ".l", "", "(a0)"))
	c.emitMovem(e, makeLine("movem", ".l", "d0/d1", ""))
	c.emitMovem(e, makeLine("movem", ".l", "d0", "d1")) // neither side reglist
	// movem with bad reg list.
	c.emitMovem(e, makeLine("movem", ".l", "foo/bar", "(a0)"))
	c.emitMovem(e, makeLine("movem", ".l", "(a0)", "foo/bar"))
	// emitMovemStore with unsupported EA.
	c.emitMovem(e, makeLine("movem", ".l", "d0/d1", "ccr"))
	// emitMovemLoad with unsupported EA.
	c.emitMovem(e, makeLine("movem", ".l", "ccr", "d0/d1"))
}

func TestBranch_EmitMulW_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMulW(e, makeLine("mulu", ".w"), false)
	c.emitMulW(e, makeLine("mulu", ".w", ""), false)
	c.emitMulW(e, makeLine("mulu", ".w", "d0", ""), false)
	c.emitMulW(e, makeLine("mulu", ".w", "d0", "(a0)"), false) // dst not Dn
}

func TestBranch_EmitDivW_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitDivW(e, makeLine("divu", ".w"), false)
	c.emitDivW(e, makeLine("divu", ".w", ""), false)
	c.emitDivW(e, makeLine("divu", ".w", "d0", ""), false)
	c.emitDivW(e, makeLine("divu", ".w", "d0", "(a0)"), false)
	// Static-zero divisor.
	c.emitDivW(e, makeLine("divu", ".w", "#0", "d1"), false)
}

func TestBranch_EmitCmpShadow_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitCmpShadow(e, makeLine("cmp", ".l"), 4)
	c.emitCmpShadow(e, makeLine("cmp", ".l", ""), 4)
	c.emitCmpShadow(e, makeLine("cmp", ".l", "", "d0"), 4)
	c.emitCmpShadow(e, makeLine("cmp", ".l", "d0", ""), 4)
}

func TestBranch_EmitTstShadow_AllErrors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitTstShadow(e, makeLine("tst", ".l"), 4)
	c.emitTstShadow(e, makeLine("tst", ".l", ""), 4)
}

func TestBranch_EmitTrap_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitTrap(e, makeLine("trap"))
	c.emitTrap(e, makeLine("trap", "", ""))
	c.emitTrap(e, makeLine("trap", "", "d0"))
}

func TestBranch_EmitChk_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitChk(e, makeLine("chk"))
	c.emitChk(e, makeLine("chk", ".w", "", "d0"))
	c.emitChk(e, makeLine("chk", ".w", "d0", ""))
	c.emitChk(e, makeLine("chk", ".w", "d0", "(a0)")) // dst not Dn
}

func TestBranch_EmitMulPair_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMulPair(e, makeLine("mulu"))
	c.emitMulPair(e, makeLine("mulu", "", "", "d0:d1"))
	c.emitMulPair(e, makeLine("mulu", "", "d0", "d0d1"))
}

func TestBranch_EmitDivPair_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitDivPair(e, makeLine("divu"))
	c.emitDivPair(e, makeLine("divu", "", "", "d0:d1"))
	c.emitDivPair(e, makeLine("divu", "", "d0", "d0d1"))
}

func TestBranch_EmitBfextu_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBfextu(e, makeLine("bfextu", "", "d0"), false)
	c.emitBfextu(e, makeLine("bfextu", "", "d0", "(a0)"), false)
	c.emitBfextu(e, makeLine("bfextu", "", "d0{0}", "d1"), false) // missing colon
	c.emitBfextu(e, makeLine("bfextu", "", "(a0)", "d1"), false)  // not Dn
	c.emitBfextu(e, makeLine("bfextu", "", "d0", "d1"), false)    // missing braces
}

func TestBranch_EmitPack_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitPack(e, makeLine("pack"))
	c.emitPack(e, makeLine("pack", "", "d0", "d1", "d2")) // adj not imm
	c.emitPack(e, makeLine("pack", "", "(a0)", "(a1)", "#0"))
	c.emitPack(e, makeLine("pack", "", "d0", "(a0)", "#0")) // mismatched modes
	c.emitPack(e, makeLine("pack", "", "", "d0", "#0"))     // bad src
}

func TestBranch_EmitUnpk_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitUnpk(e, makeLine("unpk"))
	c.emitUnpk(e, makeLine("unpk", "", "d0", "d1", "d2"))
	c.emitUnpk(e, makeLine("unpk", "", "(a0)", "(a1)", "#0"))
	c.emitUnpk(e, makeLine("unpk", "", "d0", "(a0)", "#0"))
	c.emitUnpk(e, makeLine("unpk", "", "", "d0", "#0"))
}

func TestBranch_EmitCas_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitCas(e, makeLine("cas"))
	c.emitCas(e, makeLine("cas", ".l", "(a0)", "d0", "(a1)"))
	c.emitCas(e, makeLine("cas", ".l", "d0", "(a0)", "(a1)"))
	c.emitCas(e, makeLine("cas", ".l", "d0", "d1", "d2")) // bad EA — not (An)/disp/abs
}

func TestBranch_EmitBfins_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBfins(e, makeLine("bfins"))
	c.emitBfins(e, makeLine("bfins", "", "(a0)", "d0{#0:#8}"))   // src not Dn
	c.emitBfins(e, makeLine("bfins", "", "d0", "")) // bad bf operand
	c.emitBfins(e, makeLine("bfins", "", "", "d0{#0:#8}"))
}

func TestBranch_EmitBfModify_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBfModify(e, makeLine("bfclr"), "clr")
	c.emitBfModify(e, makeLine("bfclr", "", ""), "clr")
}

func TestBranch_EmitBftst_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBftst(e, makeLine("bftst"))
	c.emitBftst(e, makeLine("bftst", "", ""))
}

func TestBranch_EmitBfffo_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBfffo(e, makeLine("bfffo"))
	c.emitBfffo(e, makeLine("bfffo", "", "", "d0"))
	c.emitBfffo(e, makeLine("bfffo", "", "d0{#0:#8}", ""))
	c.emitBfffo(e, makeLine("bfffo", "", "d0{#0:#8}", "(a0)"))
}

func TestBranch_EmitNbcd_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitNbcd(e, makeLine("nbcd"))
	c.emitNbcd(e, makeLine("nbcd", "", ""))
	c.emitNbcd(e, makeLine("nbcd", "", "(a0)"))
}

func TestBranch_EmitBcdAddSub_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBcdAdd(e, makeLine("abcd"), false)
	c.emitBcdAdd(e, makeLine("abcd", "", "d0"), false)
	c.emitBcdAdd(e, makeLine("abcd", "", "d0", "(a0)"), false)
	c.emitBcdAdd(e, makeLine("abcd", "", "", "d1"), false)
	c.emitBcdSub(e, makeLine("sbcd"), false)
	c.emitBcdSub(e, makeLine("sbcd", "", "d0", "(a0)"), false)
}

func TestBranch_FuseNormaliseValue_Variants(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// Memory operand path with signed=true.
	op, _ := ParseOperand("(a0)")
	c.fuseNormaliseValue(e, op, 4, true, ScrV1)
	// Memory operand unsigned.
	c.fuseNormaliseValue(e, op, 2, false, ScrV1)
}

func TestBranch_FuseNormaliseValue_BadOperand(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMUnknown}
	if _, err := c.fuseNormaliseValue(e, op, 4, false, ScrV1); err == nil {
		t.Errorf("expected error for AMUnknown")
	}
}

func TestBranch_LoadValue_Unsupported(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMRegList, List: "d0/d1"}
	if _, _, err := c.loadValue(e, op, 4, ScrV1); err == nil {
		t.Errorf("expected error for AMRegList")
	}
	op = Operand{Mode: AMCCR}
	if _, _, err := c.loadValue(e, op, 4, ScrV1); err == nil {
		t.Errorf("expected error for AMCCR")
	}
	// AMDispPC with empty disp triggers error path.
	op = Operand{Mode: AMDispPC, Disp: ""}
	if _, _, err := c.loadValue(e, op, 4, ScrV1); err == nil {
		t.Errorf("expected error for AMDispPC empty disp")
	}
	op = Operand{Mode: AMIndexPC, Disp: ""}
	if _, _, err := c.loadValue(e, op, 4, ScrV1); err == nil {
		t.Errorf("expected error for AMIndexPC empty disp")
	}
}

func TestBranch_StoreValue_Unsupported(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMImmediate, Imm: "5"}
	if err := c.storeValue(e, op, 4, "r1"); err == nil {
		t.Errorf("expected error storing to imm")
	}
}

func TestBranch_EmitEABase_Unsupported(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMUnknown}
	if err := c.emitEABase(e, op, ScrEA); err == nil {
		t.Errorf("expected error for AMUnknown")
	}
}

func TestBranch_EmitFusedCmpBcc_Cmpa_AllPaths(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	cons := Line{Mnemonic: "beq", Operands: []string{"L"}}
	// .w + Imm src.
	c.emitFusedCmpBcc(e, makeLine("cmpa", ".w", "#5", "a0"), cons)
	// .w + reg src.
	c.emitFusedCmpBcc(e, makeLine("cmpa", ".w", "d0", "a0"), cons)
	// .l + Imm src.
	c.emitFusedCmpBcc(e, makeLine("cmpa", ".l", "#5", "a0"), cons)
	// .l + Reg src.
	c.emitFusedCmpBcc(e, makeLine("cmpa", ".l", "d0", "a0"), cons)
	// Memory src.
	c.emitFusedCmpBcc(e, makeLine("cmpa", ".l", "(a0)", "a1"), cons)
	c.emitFusedCmpBcc(e, makeLine("cmpa", ".w", "(a0)", "a1"), cons)
	// Bcc count error.
	consBad := Line{Mnemonic: "beq"}
	c.emitFusedCmpBcc(e, makeLine("cmpa", ".w", "#5", "a0"), consBad)
}

func TestBranch_EmitArithShadows_AddaSuba(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// adda branch (skipped).
	c.emitArithShadows(e, "adda", "r9", "r9", "r1", 4)
	c.emitArithShadows(e, "suba", "r9", "r9", "r1", 4)
}

func TestBranch_EmitDBcc_DBT(t *testing.T) {
	out := convertSrc(t, "\tdbt d0,L\nL:\n\trts\n")
	// DBT always-true cc path.
	mustContain(t, out, "bra")
}

func TestBranch_EmitInstruction_UnknownInStrict(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	if _, errs := c.ConvertSource("\tnonexistent_op d0\n"); errs == 0 {
		t.Errorf("strict mode: unknown mnemonic should error")
	}
}

func TestBranch_DispOrZero_EmptyForm(t *testing.T) {
	if dispOrZero("") != "0" {
		t.Errorf("dispOrZero(empty) wrong")
	}
	if dispOrZero("8") != "8" {
		t.Errorf("dispOrZero(8) wrong")
	}
}
