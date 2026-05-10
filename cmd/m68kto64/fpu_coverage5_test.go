package main

import "testing"

func TestFMovem_Store_PostInc(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x fp0/fp1,(a0)+\n")
	mustContain(t, out, "dstore f0, (r9)")
	mustContain(t, out, "add.l r9, r9, #8")
}

func TestFMovem_Load_PreDec(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x -(a0),fp0/fp1\n")
	mustContain(t, out, "sub.l r9, r9, #8")
	mustContain(t, out, "dload f2, (r9)")
	mustContain(t, out, "dload f0, (r9)")
}

func TestFMovem_Load_PCRel(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x (LBL,pc),fp0/fp1\n")
	mustContain(t, out, "la r16, LBL")
	mustContain(t, out, "dload f0, (r16)")
}

// emitFTst on memory operand (not FPn).
func TestFTst_MemSrc(t *testing.T) {
	out := convertSrc(t, "\tftst.d (a0)\n")
	mustContain(t, out, "dload f10, (r9)")
	mustContain(t, out, "dcmp r17, f10, f12")
}

// emitFCmp size default.
func TestFCmp_NoSize(t *testing.T) {
	out := convertSrc(t, "\tfcmp fp0,fp1\n")
	mustContain(t, out, "dcmp r17, f2, f0")
}

// emitFMoveLoadToFP error: src parse fails. ParseOperand returns err for "#"
// (empty immediate).
func TestFMove_BadSrc_Parse(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmove.l #,fp0\n"); errs == 0 {
		t.Errorf("fmove with empty imm should error")
	}
}

func TestFMove_BadDst_Parse(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmove.l fp0,#\n"); errs == 0 {
		t.Errorf("fmove with empty imm dst should error")
	}
}

// emitFMoveDnToControl with non-Dn/An src.
func TestFMove_PCToFPSR_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmove.l #5,fpsr\n"); errs == 0 {
		t.Errorf("fmove imm,fpsr should error in strict")
	}
}

// emitFMovem first-list-only path with both as lists is rejected by parse —
// emitFMovem returns error if neither side is parseable as list. Two FPn-only
// operands look like both lists. Need to test the both-lists branch.
func TestFMovem_BothLists_FallsThrough(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmovem.x fp0,fp1\n"); errs == 0 {
		t.Errorf("fmovem with both lists should error")
	}
}

// emitFUnary with op fneg via 1-operand form.
func TestFUnary_Fneg_OneOp(t *testing.T) {
	out := convertSrc(t, "\tfneg fp0\n")
	mustContain(t, out, "dneg f0, f0")
}

// emitFTst src parse error.
func TestFTst_BadSrc_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tftst #\n"); errs == 0 {
		t.Errorf("ftst with empty imm should error")
	}
}

// fpRegList trailing-empty-token after slash.
func TestFPRegList_TrailingSlash(t *testing.T) {
	if _, ok := fpRegList("fp0/"); ok {
		t.Errorf("trailing slash should fail")
	}
}

// emitFScc bad dst parse error.
func TestFScc_BadDst_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfseq #\n"); errs == 0 {
		t.Errorf("fseq with empty imm dst should error")
	}
}

// emitFMovem .x degrade path with no size.
func TestFMovem_NoSize_DefaultsX(t *testing.T) {
	out := convertSrc(t, "\tfmovem fp0/fp1,(a0)\n")
	mustContain(t, out, ".X degraded to .D for fmovem")
}

// emitFPMemLoad with AMDispPC — covered indirectly via FMovem PCRel test.

// emitFTranscendental — fpRegFromToken error (covered) and materializeFPSrc
// error path. Force via fcvt-impossible src.
func TestFTranscendental_BadSrc(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfsin #,fp0\n"); errs == 0 {
		t.Errorf("fsin with empty imm should error")
	}
}

// emitFMovemStore unsupported mode (immediate dst).
func TestFMovem_Store_Imm_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmovem.x fp0,#5\n"); errs == 0 {
		t.Errorf("fmovem store to imm should error")
	}
}

// emitFMovemLoad unsupported mode (immediate src).
func TestFMovem_Load_Imm_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	// Imm parse will succeed; emitFMovemLoad rejects AMImmediate.
	if _, errs := c.ConvertSource("\tfmovem.x #5,fp0\n"); errs == 0 {
		t.Errorf("fmovem load from imm should error")
	}
}
