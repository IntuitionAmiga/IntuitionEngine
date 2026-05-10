package main

import "testing"

// Final remaining branches.

// emitFUnary live shadow path.
func TestFUnary_Live_EmitsShadow(t *testing.T) {
	out := convertSrc(t, "\tfneg.x fp0,fp1\nL:\n\tfbeq target\n")
	mustContain(t, out, "arith result vs zero")
}

// emitFTst live shadow path.
func TestFTst_Live_EmitsShadow(t *testing.T) {
	out := convertSrc(t, "\tftst.x fp0\nL:\n\tfbeq target\n")
	mustContain(t, out, "; bit2 (Z)")
}

// emitFCmpForFuse default-size branch via fcmp with no .x.
func TestFCmpForFuse_DefaultSize(t *testing.T) {
	out := convertSrc(t, "\tfcmp fp0,fp1\n\tfbeq target\n")
	mustContain(t, out, "dcmp r17, f2, f0")
}

// fpRegList descending range valid syntax.
func TestFPRegList_DescendingRangeValidSyntax(t *testing.T) {
	if _, ok := fpRegList("fp3-fp0"); ok {
		t.Errorf("fp3-fp0 (descending) should fail")
	}
}

// emitFPMemLoad AMIndexPC branch.
func TestFPMemLoad_IndexPC(t *testing.T) {
	out := convertSrc(t, "\tfmove.d (LBL,pc,d0.l*4),fp0\n")
	mustContain(t, out, "la r16, LBL")
	mustContain(t, out, "dload f0, (r16)")
}

// emitFMoveLoadToFP int-load error path: pass AMRegList-shaped src.
func TestFMoveLoadToFP_IntLoadErr(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	if err := c.emitFMoveLoadToFP(e, "d0/d1", "f0", ".l"); err == nil {
		t.Errorf("expected error from loadValue on AMRegList")
	}
}

// ConvertLines fuse path: emitFusedFCmpFBcc returns err → "ERROR: FP fuse
// failed" diagnostic counted as conversion error. Reach via fcmp with src=Dn
// (materializeFPSrc rejects via emitFPMemLoad AMDataReg).
func TestConvertLines_FuseError_PathHit(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	out, errs := c.ConvertSource("\tfcmp.x d0,fp0\n\tfbeq target\n")
	if errs == 0 {
		t.Errorf("fcmp.x d0,fp0 should produce a fuse-path error:\n%s", out)
	}
}

// emitFMovemStore / Load emitEABase error — push synthetic mode through.
// emitEABase rejects modes not in its switch. emitFMovemStore lists which
// modes it forwards; AMRegList is not forwarded, hits final return. So
// reach emitEABase err: send AMIndirect with bogus reg — emitEABase
// accepts the indirect form and does move/lea. Hmm — emitEABase only
// returns err for unsupported-mode default. Sending unhandled mode means
// outer switch already rejects it. So this err branch is genuinely
// unreachable from emitFMovemStore/Load. Acceptable.
