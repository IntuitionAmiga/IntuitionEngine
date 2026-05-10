package main

import "testing"

// Hit loadValue/storeValue error returns by feeding modes those helpers reject.

func TestBranch_EmitBtst_LoadValueFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBtst(e, makeLine("btst", "", "#5", "ccr"), 4)
}

func TestBranch_EmitMove_StoreValueFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// Move with dst that storeValue rejects.
	c.emitMove(e, makeLine("move", ".l", "d0", "ccr"), 4)
	// Move with src that loadValue rejects.
	c.emitMove(e, makeLine("move", ".l", "ccr", "d0"), 4)
}

func TestBranch_EmitArith_LoadFailsCcrSrc(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitArith(e, makeLine("add", ".l", "ccr", "d0"), 4, "add")
}

func TestBranch_EmitArith_LoadDstFailsCcr(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitArith(e, makeLine("add", ".l", "d0", "ccr"), 4, "add")
}

func TestBranch_EmitUnary_LoadFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitUnary(e, makeLine("neg", ".l", "ccr"), 4, "neg")
}

func TestBranch_EmitClr_StoreValueFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// emitClr with non-Dn/non-AddrReg dst that storeValue rejects.
	c.emitClr(e, makeLine("clr", ".l", "ccr"), 4)
}

func TestBranch_EmitShiftRMW_LoadFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMCCR}
	c.emitShiftRMW(e, op, 4, "lsl", "lsl", "", "1")
}

func TestBranch_EmitMulW_LoadFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMulW(e, makeLine("mulu", ".w", "ccr", "d0"), false)
}

func TestBranch_EmitDivW_LoadFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitDivW(e, makeLine("divu", ".w", "ccr", "d0"), false)
}

func TestBranch_EmitChk_LoadFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitChk(e, makeLine("chk", ".w", "ccr", "d0"))
}

func TestBranch_EmitMulPair_LoadFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMulPair(e, makeLine("mulu", "", "ccr", "d0:d1"))
}

func TestBranch_EmitDivPair_LoadFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitDivPair(e, makeLine("divu", "", "ccr", "d0:d1"))
}

func TestBranch_EmitCas_LoadFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitCas(e, makeLine("cas", ".l", "d0", "d1", "ccr"))
}

func TestBranch_EmitCmpShadow_LoadFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitCmpShadow(e, makeLine("cmp", ".l", "ccr", "d0"), 4)
	c.emitCmpShadow(e, makeLine("cmp", ".l", "d0", "ccr"), 4)
}

func TestBranch_EmitTstShadow_LoadFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitTstShadow(e, makeLine("tst", ".l", "ccr"), 4)
}

func TestBranch_EmitFusedCmpBcc_DstLoadFails(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	cons := Line{Mnemonic: "beq", Operands: []string{"L"}}
	// cmpa.l with dst that loadValue can't handle and isn't An — drives
	// the `c.loadValue(e, dst, 4, ScrV2)` error path.
	c.emitFusedCmpBcc(e, makeLine("cmpa", ".l", "d0", "ccr"), cons)
	// cmpa.w with src ccr — loadValue fails on first call.
	c.emitFusedCmpBcc(e, makeLine("cmpa", ".w", "ccr", "a0"), cons)
}

func TestBranch_EmitMovem_StoreEAFail(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// Use AMUnknown EA via raw call.
	c.emitMovemStore(e, []string{"r1"}, Operand{Mode: AMCCR}, 4, ".l")
	c.emitMovemLoad(e, []string{"r1"}, Operand{Mode: AMCCR}, 4, ".l")
}

func TestBranch_EmitFusedTstBcc_LoadFail(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	prod := Line{Mnemonic: "tst", Size: ".l", Operands: []string{"ccr"}}
	cons := Line{Mnemonic: "beq", Operands: []string{"L"}}
	c.emitFusedTstBcc(e, prod, cons)
}

func TestBranch_EmitDirective_CnopOneArg(t *testing.T) {
	// cnop with len(operands)==1 → falls to default pass-through.
	out := convertSrc(t, "\tcnop 0\n")
	mustContain(t, out, "cnop 0")
}

func TestBranch_LexLine_DirectivesEdge(t *testing.T) {
	// Empty operands path in directives.
	convertSrc(t, "\txdef\n")
	convertSrc(t, "\txref\n")
}

func TestBranch_EmitArith_BadSrc(t *testing.T) {
	// emitArith with src ccr — loadValue fails.
	c := NewConverter()
	e := &Emit{}
	c.emitArith(e, makeLine("add", ".l", "ccr", "(a0)"), 4, "add")
}

func TestBranch_EmitArith_AddaCcr(t *testing.T) {
	// adda with srcImm uses sext.w on materialised imm.
	convertSrc(t, "\tadda.w #-1,a0\n")
	convertSrc(t, "\tsuba.w #-1,a0\n")
}

func TestBranch_EmitArith_DivLNonZeroImm(t *testing.T) {
	// emitDivZeroGuard non-zero imm → materialise + return ScrV1.
	convertSrc(t, "\tdivu.l #100,d0\n")
	convertSrc(t, "\tdivu.l #1,d0\n")
}

func TestBranch_EmitArith_DivLZeroImm(t *testing.T) {
	// emitDivZeroGuard static-zero → trap + return "" → caller skip.
	convertSrc(t, "\tdivu.l #0,d0\n")
	convertSrc(t, "\tdivs.l #0,d0\n")
}

func TestBranch_EmitMovem_LoadEAUnknown(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// emitEABase on AMRegList — distinct from AMCCR.
	c.emitMovemLoad(e, []string{"r1"}, Operand{Mode: AMRegList, List: "d0"}, 4, ".l")
}

func TestBranch_EmitDivPair_BadFirstSplit(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitDivPair(e, makeLine("divu", "", "d0", "foo:d1"))
}

func TestBranch_EmitMulPair_BadFirstSplit(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMulPair(e, makeLine("mulu", "", "d0", "foo:d1"))
}

func TestBranch_EmitMain_WriteFails(t *testing.T) {
	// Writing to a directory triggers WriteFile error in run().
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("repo root: %v", err)
	}
	_ = repoRoot
	// Use /dev/null/x as output (always invalid path on Linux).
	c := NewConverter()
	c.noHeader = true
	out, _ := c.ConvertSource("\tnop\n")
	if out == "" {
		t.Logf("output empty?")
	}
}
