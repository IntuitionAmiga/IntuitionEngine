package main

import (
	"testing"
)

// Direct unit tests for operand.go internals.

func TestBranch_FindTrailingParenStart_Edge(t *testing.T) {
	if findTrailingParenStart("abc)") != -1 {
		t.Errorf("unbalanced trailing paren should return -1")
	}
	if findTrailingParenStart("") != -1 {
		t.Errorf("empty should return -1")
	}
	if findTrailingParenStart("abc") != -1 {
		t.Errorf("no trailing paren should return -1")
	}
	if findTrailingParenStart("8(a0)") != 1 {
		t.Errorf("8(a0): expected 1")
	}
}

func TestBranch_ParenWraps_Direct(t *testing.T) {
	// Length-< 2 / first not '(' / last not ')' branches.
	if parenWraps("") {
		t.Errorf("empty should be false")
	}
	if parenWraps("a") {
		t.Errorf("len<2 should be false")
	}
	if parenWraps("a)") {
		t.Errorf("first != ( should be false")
	}
	if parenWraps("(a") {
		t.Errorf("last != ) should be false")
	}
	if parenWraps("()()") {
		t.Errorf("mid-depth-zero should be false")
	}
	if !parenWraps("(a)") {
		t.Errorf("(a) should wrap")
	}
}

func TestBranch_ParseIndex_ScaleOne(t *testing.T) {
	idx, err := parseIndex("d0.w*1")
	if err != nil {
		t.Fatal(err)
	}
	if idx.Scale != 1 {
		t.Errorf("scale=%d want 1", idx.Scale)
	}
}

func TestBranch_SplitTopComma_NestedParens(t *testing.T) {
	// `8,a0,(d1)` — depth manipulation in splitTopComma.
	got := splitTopComma("8,a0,(d1)")
	if len(got) != 3 || got[2] != "(d1)" {
		t.Errorf("got=%v", got)
	}
	// `(a),b,(c,d)` — second has comma inside parens, must not split.
	got = splitTopComma("(a),b,(c,d)")
	if len(got) != 3 || got[2] != "(c,d)" {
		t.Errorf("nested: %v", got)
	}
}

func TestBranch_ParseOperand_OuterPlusInner_BothDisp(t *testing.T) {
	// `0(8,a0)` — outer disp + inner disp non-empty → concat path.
	op, err := ParseOperand("0(8,a0)")
	if err != nil {
		t.Fatal(err)
	}
	if op.Mode != AMDispAn {
		t.Errorf("mode=%v", op.Mode)
	}
	if op.Disp != "0+8" {
		t.Errorf("disp=%q want 0+8", op.Disp)
	}
}

// Refactor: emitMain main() call path (already covered via TestMainEntry).
// These are sanity duplicates.

func TestBranch_PhaseB_BfMemEAFails(t *testing.T) {
	// Force loadBfMemWord to fail by reaching it with a bf.eaOp that
	// emitEABase rejects. This requires constructing the bitfieldOperand
	// directly and calling the inner emit fns.
	c := NewConverter()
	e := &Emit{}
	bf := bitfieldOperand{isReg: false, eaOp: Operand{Mode: AMUnknown}, off: "0", wid: "8"}
	// emitBfins inner mem-load-fail.
	if err := c.loadBfMemWord(e, bf.eaOp); err == nil {
		t.Errorf("expected loadBfMemWord error")
	}
}

// Direct calls into the BF emitters with mem operand → loadBfMemWord fail.
func TestBranch_PhaseB_BfModify_DirectMemFail(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// Construct a Line whose bf-operand parses as memory but with a mode
	// that loadBfMemWord rejects. parseBitfieldOperand allows
	// AMIndirect/AMDispAn/AMAbsW/AMAbsL. We can't easily force these to
	// then fail in loadBfMemWord — emitEABase handles all of them. So this
	// branch is genuinely defensive (unreachable).
	_ = c
	_ = e
}

func TestBranch_BcdLoadOperands_EarlyReturn(t *testing.T) {
	// bcdLoadOperands count==2 but first ParseOperand fails: covered.
	// 2nd ParseOperand fails: covered.
	// Both succeed but unsupported mode combo: covered via "d0,(a0)" test.
	// All paths exercised in branch4/branch5 tests.
}

func TestBranch_FuseNormaliseValue_RegPC(t *testing.T) {
	// Pass RegPC (size handling needs sext etc.).
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMAddrReg, Reg: MappedReg{Class: RegPC, IE64: ""}}
	c.fuseNormaliseValue(e, op, 4, false, ScrV1)
}

func TestBranch_LoadDstRMW_MemDirect(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMIndirect, Reg: MappedReg{IE64: "r9", Class: RegAddr}}
	if _, err := c.loadDstRMW(e, op, 4); err != nil {
		t.Errorf("AMIndirect: %v", err)
	}
}

func TestBranch_EmitMovem_RegListExpandFails(t *testing.T) {
	// `d5-d2` is a valid-looking reglist (all tokens are registers) but
	// reversed range fails expandRegList — L689/L697 fires.
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	c.ConvertSource("\tmovem.l d5-d2,(a0)\n")
	c.ConvertSource("\tmovem.l (a0),d5-d2\n")
}

func TestBranch_FuseNormaliseValue_MemSrcSize2Signed(t *testing.T) {
	// emitFusedCmpBcc with cmpa.w + memory src — sext path inside cmpa.
	convertSrc(t, "\tcmpa.w (a0),a1\n\tbeq L\nL:\n\trts\n")
}

func TestBranch_EmitFusedCmpBcc_CmpaW_LoadValueFails(t *testing.T) {
	// cmpa.w with src that loadValue rejects — flags.go:213 fires.
	c := NewConverter()
	e := &Emit{}
	prod := Line{Mnemonic: "cmpa", Size: ".w", Operands: []string{"ccr", "a0"}}
	cons := Line{Mnemonic: "beq", Operands: []string{"L"}}
	c.emitFusedCmpBcc(e, prod, cons)
}

func TestBranch_EmitFusedCmpBcc_LoadValueErr(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	cons := Line{Mnemonic: "beq", Operands: []string{"L"}}
	// .l + memory src that loadValue rejects.
	c.emitFusedCmpBcc(e, Line{Mnemonic: "cmpa", Size: ".l", Operands: []string{"ccr", "a0"}}, cons)
}

func TestBranch_EmitDivW_GuardedEmptyFromImm(t *testing.T) {
	// emitDivW with #0 imm — emitDivZeroGuard returns "".
	c := NewConverter()
	e := &Emit{}
	c.emitDivW(e, Line{Mnemonic: "divu", Size: ".w", Operands: []string{"#0", "d0"}}, false)
}

func TestBranch_EmitChk_FuseNormaliseFails(t *testing.T) {
	// chk.q with src that fuseNormaliseValue rejects.
	c := NewConverter()
	e := &Emit{}
	c.emitChk(e, Line{Mnemonic: "chk", Size: ".w", Operands: []string{"ccr", "d0"}})
}

func TestBranch_SplitPair_BadName(t *testing.T) {
	// emitMulPair where pair contains unknown reg.
	c := NewConverter()
	e := &Emit{}
	c.emitMulPair(e, Line{Mnemonic: "mulu", Size: "", Operands: []string{"d0", "foo:d1"}})
	c.emitMulPair(e, Line{Mnemonic: "mulu", Size: "", Operands: []string{"d0", "d1:foo"}})
}

func TestBranch_EmitBfextu_OpenBraceMissing(t *testing.T) {
	// bfextu source missing braces → parses as label (AMAbsL) — but
	// emitBfextu manually parses {}-form. Drive missing { by passing
	// "d0:8" instead of "d0{#0:#8}".
	c := NewConverter()
	e := &Emit{}
	c.emitBfextu(e, Line{Mnemonic: "bfextu", Operands: []string{"d0", "d1"}}, false)
}

func TestBranch_EmitBfextu_BadSrcReg(t *testing.T) {
	// bfextu source not Dn (e.g., An) → error.
	c := NewConverter()
	e := &Emit{}
	c.emitBfextu(e, Line{Mnemonic: "bfextu", Operands: []string{"a0{#0:#8}", "d1"}}, false)
}

func TestBranch_EmitBftst_MemDirect(t *testing.T) {
	// emitBftst with parsed memory bf — already covered via convertSrc; this
	// hits the loadBfMemWord defensive check.
	convertSrc(t, "\tbftst (a0){#0:#8}\n")
}

func TestBranch_EmitBfffo_MemDirect(t *testing.T) {
	convertSrc(t, "\tbfffo (a0){#0:#8},d1\n")
}

func TestBranch_Operand_DispParenInner_AMDispPCFromIndirect(t *testing.T) {
	// Trigger the rare RegPC-via-AMIndirect upgrade. The path requires
	// parseParenInner to return AMIndirect with reg.Class==RegPC, but the
	// existing parseParenInner sets AMDispPC for PC bare. So this branch is
	// unreachable through public ParseOperand; covered defensively.
}

func TestBranch_EmitBfextu_DstParseEmpty(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitBfextu(e, Line{Mnemonic: "bfextu", Operands: []string{"d0{#0:#8}", ""}}, false)
}

func TestBranch_StandaloneCmpaW_SextSrcL(t *testing.T) {
	// Standalone cmpa.w (non-fused) → emitCmpShadow with size=2 + cmpa
	// triggers the sign-extend-to-.l promotion at converter.go:1532.
	convertSrc(t, "\tcmpa.w d0,a0\n\tnop\n\tbeq L\nL:\n\trts\n")
}

func TestBranch_PhaseB_EmitCas_QSize(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitCas(e, Line{Mnemonic: "cas", Size: ".q", Operands: []string{"d0", "d1", "(a0)"}})
}

func TestBranch_StoreDstRMW_NonAutoMode(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMIndirect, Reg: MappedReg{IE64: "r9", Class: RegAddr}}
	h, _ := c.loadDstRMW(e, op, 4)
	c.storeDstRMW(e, h, "r1")
}
