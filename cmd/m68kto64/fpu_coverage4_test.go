package main

import "testing"

// Drill remaining FMovem / FMove / FArith / FTst paths.

func TestFMovem_Store_Abs(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x fp0/fp1,(LBL).l\n")
	mustContain(t, out, "la r16, LBL")
	mustContain(t, out, "dstore f0, (r16)")
	mustContain(t, out, "dstore f2, 8(r16)")
}

func TestFMovem_Store_IndexAn(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x fp0/fp1,(8,a0,d0.l*4)\n")
	mustContain(t, out, "dstore f0, (r16)")
}

func TestFMovem_Load_Abs(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x (LBL).l,fp0/fp1\n")
	mustContain(t, out, "la r16, LBL")
	mustContain(t, out, "dload f0, (r16)")
}

func TestFMovem_Load_IndexAn(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x (8,a0,d0.l*4),fp0/fp1\n")
	mustContain(t, out, "dload f0, (r16)")
}

func TestFMovem_Single_S(t *testing.T) {
	out := convertSrc(t, "\tfmovem.s fp0,(a0)\n")
	mustContain(t, out, "fstore f0, (r16)")
}

// FMOVE.X comment emit on degraded.
func TestFMove_X_FPmFPn_Degrade(t *testing.T) {
	out := convertSrc(t, "\tfmove.x fp0,fp1\n")
	mustContain(t, out, ".X degraded to .D")
}

// emitFMove with no size: defaults to .x.
func TestFMove_NoSize_DefaultsX(t *testing.T) {
	out := convertSrc(t, "\tfmove fp0,fp1\n")
	mustContain(t, out, "dmov f2, f0")
}

// emitFArith strict-error on bad src parse.
func TestFArith_BadSrc(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfadd #,fp0\n"); errs == 0 {
		t.Errorf("fadd with empty immediate should error")
	}
}

// fpArithOp: fsglmul / fsgldiv branches.
func TestFArithOp_Sgl(t *testing.T) {
	if got := fpArithOp("fsgldiv"); got != "fdiv" {
		t.Errorf("fsgldiv → %q, want fdiv", got)
	}
}

// emitFTst — value-from-FPn path (no memory load).
func TestFTst_FPnSrc_LoadsZeroIntoF12(t *testing.T) {
	out := convertSrc(t, "\tftst fp0\n")
	mustContain(t, out, "dload f12,")
	mustContain(t, out, "dcmp r17, f0, f12")
}

// emitFScc on Dn dest.
func TestFScc_OnDn_StoresByte(t *testing.T) {
	out := convertSrc(t, "\tfseq d0\n")
	mustContain(t, out, "neg.l r17,")
	// Dn store.b path partial-update merges into low byte of d0=r1.
	mustContain(t, out, "or.q r1, r1,")
}

// emitFMoveCR with odd dst — strict only checks fpRegFromToken.
func TestFMoveCR_DataReg_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmovecr d0,#0\n"); errs == 0 {
		t.Errorf("fmovecr non-FPn dst should error")
	}
}

// emitFMoveStoreFromFP with PreDec / PostInc int dst.
func TestFMove_FP_To_PostInc_L(t *testing.T) {
	out := convertSrc(t, "\tfmove.l fp0,(a0)+\n")
	mustContain(t, out, "dcvtfi r17, f0")
	mustContain(t, out, "store.l r17, (r9)")
	mustContain(t, out, "add.l r9, r9, #4")
}

// fpRegList: empty operand → false.
func TestFPRegList_EmptyString(t *testing.T) {
	if _, ok := fpRegList(""); ok {
		t.Errorf("empty list should fail")
	}
}

// FMovem.x list,(An) explicit indirect (not predec/postinc/abs).
func TestFMovem_Store_Indirect(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x fp0/fp1,(a0)\n")
	mustContain(t, out, "dstore f0, (r16)")
	mustContain(t, out, "dstore f2, 8(r16)")
}

func TestFMovem_Load_Indirect(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x (a0),fp0/fp1\n")
	mustContain(t, out, "dload f0, (r16)")
	mustContain(t, out, "dload f2, 8(r16)")
}
