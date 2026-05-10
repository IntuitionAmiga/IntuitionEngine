package main

import "testing"

// Final push: every remaining branch.

func TestFMove_S_Imm(t *testing.T) {
	out := convertSrc(t, "\tfmove.s #PI,fp0\n")
	mustContain(t, out, "la r16, PI")
	mustContain(t, out, "fload f0, (r16)")
}

func TestFMove_D_Imm(t *testing.T) {
	out := convertSrc(t, "\tfmove.d #LBL,fp0\n")
	mustContain(t, out, "la r16, LBL")
	mustContain(t, out, "dload f0, (r16)")
}

func TestFMove_B_FromMem(t *testing.T) {
	out := convertSrc(t, "\tfmove.b (a0),fp0\n")
	mustContain(t, out, "sext.b r17,")
	mustContain(t, out, "dcvtif f0, r17")
}

func TestFMove_B_FromImm(t *testing.T) {
	out := convertSrc(t, "\tfmove.l #42,fp0\n")
	mustContain(t, out, "move.l r17, #42")
	mustContain(t, out, "dcvtif f0, r17")
}

// fpFTrapccSyscall unknown branch — already tested. Re-run for confidence.

// addFPConst dedup branch: same const reused returns same label.
func TestAddFPConst_Dedup(t *testing.T) {
	c := NewConverter()
	a := c.addFPConst("1.0", "first")
	b := c.addFPConst("1.0", "second")
	if a != b {
		t.Errorf("addFPConst should dedup: %q vs %q", a, b)
	}
}

func TestFMoveCR_Empty_Operand_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmovecr fp0\n"); errs == 0 {
		t.Errorf("fmovecr 1-operand should error")
	}
}

// Cover materializeFPSrc bad-size error path. Force via direct Line.
func TestMaterializeFPSrc_BadSize(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	_, err := c.materializeFPSrc(e, "fp0", ".q")
	if err != nil {
		t.Errorf("materializeFPSrc(fp0, .q): expected pass-through to fpRegFromToken, got err=%v", err)
	}
	// Now with non-FP src, .q size — should error.
	_, err = c.materializeFPSrc(e, "(a0)", ".q")
	if err == nil {
		t.Errorf("expected error for unrecognised size")
	}
}

// emitFMoveLoadToFP unrecognised-size error path: directly invoke.
func TestFMoveLoadToFP_BadSize(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	if err := c.emitFMoveLoadToFP(e, "(a0)", "f0", ".q"); err == nil {
		t.Errorf("emitFMoveLoadToFP with .q size should error")
	}
}

// emitFMoveStoreFromFP unrecognised-size error.
func TestFMoveStoreFromFP_BadSize(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	if err := c.emitFMoveStoreFromFP(e, "f0", "(a0)", ".q"); err == nil {
		t.Errorf("emitFMoveStoreFromFP with .q size should error")
	}
}

// emitFPMemLoad unsupported-mode error.
func TestFPMemLoad_UnsupportedMode_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMDataReg}
	if err := c.emitFPMemLoad(e, op, "f0", ".d"); err == nil {
		t.Errorf("expected error for AMDataReg")
	}
}

// emitFPMemStore unsupported-mode error.
func TestFPMemStore_UnsupportedMode_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMDataReg}
	if err := c.emitFPMemStore(e, op, "f0", ".d"); err == nil {
		t.Errorf("expected error for AMDataReg")
	}
}

// emitFMovemStore unsupported EA.
func TestFMovemStore_UnsupportedEA_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMImmediate}
	if err := c.emitFMovemStore(e, []string{"f0"}, op, ".d", 8); err == nil {
		t.Errorf("expected error for AMImmediate")
	}
}

func TestFMovemLoad_UnsupportedEA_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMImmediate}
	if err := c.emitFMovemLoad(e, []string{"f0"}, op, ".d", 8); err == nil {
		t.Errorf("expected error for AMImmediate")
	}
}

func TestFArith_OpUnknown_Default(t *testing.T) {
	if got := fpArithOp("ffoo"); got != "; UNKNOWN-FARITH ffoo" {
		t.Errorf("fpArithOp default = %q", got)
	}
}

// emitShadowFBccTest unknown cc default.
func TestShadowFBccTest_UnknownCC(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitShadowFBccTest(e, "garbage")
	if !contains(e.String(), "UNKNOWN-FPCC") {
		t.Errorf("unknown cc should emit UNKNOWN-FPCC diagnostic; got:\n%s", e.String())
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// emitFCmp bad src parse error.
func TestFCmp_BadSrcParse(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfcmp #,fp0\n"); errs == 0 {
		t.Errorf("fcmp with empty immediate should error")
	}
}

// fpRegList — empty after parse (just slashes)
func TestFPRegList_OnlySlashes(t *testing.T) {
	if _, ok := fpRegList("/"); ok {
		t.Errorf("/ should fail to parse")
	}
}

// emitFMove src and dst both non-FP / non-control.
func TestFMove_NeitherSideFP_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmove.l d0,d1\n"); errs == 0 {
		t.Errorf("fmove between two integer regs should error")
	}
}

// emitFMoveControlToDn bad class default.
func TestFMoveControlToDn_BadClass(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	if err := c.emitFMoveControlToDn(e, FPRegUnknown, "d0"); err == nil {
		t.Errorf("expected error for FPRegUnknown class")
	}
}

func TestFMoveDnToControl_BadClass(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	if err := c.emitFMoveDnToControl(e, "d0", FPRegUnknown); err == nil {
		t.Errorf("expected error for FPRegUnknown class")
	}
}

// emitFUnary unsupported m default.
func TestFUnary_Default_Unknown(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	l := Line{Mnemonic: "fbogus", Operands: []string{"fp0", "fp1"}}
	if err := c.emitFUnary(e, l, "fbogus"); err == nil {
		t.Errorf("expected error for unsupported unary op")
	}
}

// emitFTranscendental default.
func TestFTranscendental_Default_Unknown(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	l := Line{Mnemonic: "fbogus", Operands: []string{"fp0", "fp1"}}
	if err := c.emitFTranscendental(e, l, "fbogus"); err == nil {
		t.Errorf("expected error for unsupported transcendental")
	}
}

// emitFTrapcc unknown cc.
func TestFTrapcc_UnknownCC_Errors(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	l := Line{Mnemonic: "ftrapbogus"}
	if err := c.emitFTrapcc(e, l, "ftrapbogus"); err == nil {
		t.Errorf("expected error for unknown ftrapcc")
	}
}
