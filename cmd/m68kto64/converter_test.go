package main

import (
	"strings"
	"testing"
)

// convertOneInstr is a test helper: feed exactly one m68k source line through
// the converter (with header suppressed) and return the trimmed body.
func convertOneInstr(t *testing.T, input string) string {
	t.Helper()
	c := NewConverter()
	c.noHeader = true
	out, errs := c.ConvertSource(input + "\n")
	if errs != 0 {
		t.Fatalf("conversion errors for %q:\n%s", input, out)
	}
	return strings.TrimSpace(out)
}

func mustContain(t *testing.T, src, want string) {
	t.Helper()
	if !strings.Contains(src, want) {
		t.Errorf("output missing %q\n--- got ---\n%s\n-----------", want, src)
	}
}

func mustNotContain(t *testing.T, src, want string) {
	t.Helper()
	if strings.Contains(src, want) {
		t.Errorf("output unexpectedly contained %q\n--- got ---\n%s\n-----------", want, src)
	}
}

// =====================================================================
// MOVE
// =====================================================================

func TestMove_DnDn_Long(t *testing.T) {
	out := convertOneInstr(t, "\tmove.l d0,d1")
	mustContain(t, out, "move.l r2, r1")
}

func TestMove_DnDn_SamePass(t *testing.T) {
	// move.l d0,d0 → no-op (no emit).
	out := convertOneInstr(t, "\tmove.l d0,d0")
	mustNotContain(t, out, "move.l r1, r1")
}

func TestMove_ImmDn_Long(t *testing.T) {
	out := convertOneInstr(t, "\tmove.l #$1234,d0")
	mustContain(t, out, "move.l r17, #$1234")
	mustContain(t, out, "move.l r1, r17")
}

func TestMove_Indirect_Load(t *testing.T) {
	out := convertOneInstr(t, "\tmove.l (a0),d1")
	mustContain(t, out, "load.l r17, (r9)")
	mustContain(t, out, "move.l r2, r17")
}

func TestMove_PostInc_Load(t *testing.T) {
	out := convertOneInstr(t, "\tmove.l (a0)+,d1")
	mustContain(t, out, "load.l r17, (r9)")
	mustContain(t, out, "add.l r9, r9, #4")
}

func TestMove_PreDec_Store(t *testing.T) {
	out := convertOneInstr(t, "\tmove.l d1,-(a0)")
	mustContain(t, out, "sub.l r9, r9, #4")
	mustContain(t, out, "store.l r2, (r9)")
}

func TestMove_DispAn(t *testing.T) {
	out := convertOneInstr(t, "\tmove.l 8(a0),d1")
	mustContain(t, out, "load.l r17, 8(r9)")
}

func TestMove_Abs(t *testing.T) {
	out := convertOneInstr(t, "\tmove.l ($F2000).l,d0")
	mustContain(t, out, "la r16, $F2000")
	mustContain(t, out, "load.l r17, (r16)")
}

func TestMove_Byte_PartialUpdate(t *testing.T) {
	// move.b #$AB,d0 must preserve upper bits of r1.
	out := convertOneInstr(t, "\tmove.b #$AB,d0")
	mustContain(t, out, "and.q r1, r1, #$FFFFFFFFFFFFFF00")
	mustContain(t, out, "or.q r1, r1, r19")
}

func TestMoveq(t *testing.T) {
	out := convertOneInstr(t, "\tmoveq #5,d0")
	mustContain(t, out, "moveq r1, #5")
}

func TestMovea_Word_SignExtend(t *testing.T) {
	out := convertOneInstr(t, "\tmovea.w d0,a1")
	mustContain(t, out, "sext.w r10, r1")
}

// =====================================================================
// ADD / SUB / AND / OR / EOR
// =====================================================================

func TestAdd_DnDn_Long(t *testing.T) {
	out := convertOneInstr(t, "\tadd.l d0,d1")
	mustContain(t, out, "add.l r2, r2, r1")
}

func TestAdd_ImmDn_Long(t *testing.T) {
	out := convertOneInstr(t, "\tadd.l #16,d0")
	// ADD wants shadow C/V; immediate is materialised first so a register
	// operand is available for the shadow-CV computation.
	mustContain(t, out, "move.l r17, #16")
	mustContain(t, out, "add.l r1, r1, r17")
}

func TestAdda_Word_SignExtendsImm(t *testing.T) {
	// adda.w d0,a0 must promote to .l.
	out := convertOneInstr(t, "\tadda.w d0,a0")
	mustContain(t, out, "sext.w r17, r1")
	mustContain(t, out, "add.l r9, r9, r17")
}

func TestSub_Mem_RMW(t *testing.T) {
	out := convertOneInstr(t, "\tsub.l #1,(a0)")
	mustContain(t, out, "load.l r18, (r9)")
	mustContain(t, out, "move.l r17, #1") // imm materialised for shadow C/V
	mustContain(t, out, "sub.l r18, r18, r17")
	mustContain(t, out, "store.l r18, (r9)")
}

func TestAnd_PartialByte(t *testing.T) {
	out := convertOneInstr(t, "\tand.b #$0F,d0")
	mustContain(t, out, "and.l r18, r1, #$FF")
	mustContain(t, out, "and.b r18, r18, #$0F")
	// Merge keeps upper bits.
	mustContain(t, out, "and.q r1, r1, #$FFFFFFFFFFFFFF00")
}

func TestOr_DnDn_Long(t *testing.T) {
	out := convertOneInstr(t, "\tor.l d2,d3")
	mustContain(t, out, "or.l r4, r4, r3")
}

func TestEor_DnDn(t *testing.T) {
	out := convertOneInstr(t, "\teor.l d2,d3")
	mustContain(t, out, "eor.l r4, r4, r3")
}

// =====================================================================
// NEG / NOT / CLR / EXT
// =====================================================================

func TestNeg_DnLong(t *testing.T) {
	out := convertOneInstr(t, "\tneg.l d0")
	mustContain(t, out, "neg.l r1, r1")
}

func TestNot_Mem(t *testing.T) {
	out := convertOneInstr(t, "\tnot.l (a0)")
	mustContain(t, out, "load.l r18, (r9)")
	mustContain(t, out, "not.l r18, r18")
	mustContain(t, out, "store.l r18, (r9)")
}

func TestClr_DnLong(t *testing.T) {
	out := convertOneInstr(t, "\tclr.l d0")
	mustContain(t, out, "move.l r1, #0")
}

func TestClr_DnByte_PartialUpdate(t *testing.T) {
	out := convertOneInstr(t, "\tclr.b d0")
	mustContain(t, out, "and.q r1, r1, #$FFFFFFFFFFFFFF00")
}

func TestClr_Mem(t *testing.T) {
	out := convertOneInstr(t, "\tclr.l (a0)")
	mustContain(t, out, "store.l r0, (r9)")
}

func TestExt_W(t *testing.T) {
	out := convertOneInstr(t, "\text.w d0")
	mustContain(t, out, "sext.b r17, r1")
}

func TestExt_L(t *testing.T) {
	out := convertOneInstr(t, "\text.l d0")
	mustContain(t, out, "sext.w r1, r1")
}

func TestExtb_L(t *testing.T) {
	out := convertOneInstr(t, "\textb.l d0")
	mustContain(t, out, "sext.b r1, r1")
}

// =====================================================================
// SHIFTS / ROTATES
// =====================================================================

func TestLsl_ImmDn_Long(t *testing.T) {
	out := convertOneInstr(t, "\tlsl.l #2,d0")
	mustContain(t, out, "lsl.l r1, r1, #2")
}

func TestAsl_AliasesLsl(t *testing.T) {
	out := convertOneInstr(t, "\tasl.l #2,d0")
	mustContain(t, out, "lsl.l r1, r1, #2")
}

func TestAsr_DnDn(t *testing.T) {
	// asr.l d0,d1 — count from d0 low 6 bits.
	out := convertOneInstr(t, "\tasr.l d0,d1")
	mustContain(t, out, "and.l r17, r1, #63")
	mustContain(t, out, "asr.l r2, r2, r17")
}

func TestRol_ImmDn(t *testing.T) {
	out := convertOneInstr(t, "\trol.l #1,d0")
	mustContain(t, out, "rol.l r1, r1, #1")
}

// =====================================================================
// LEA
// =====================================================================

func TestLea_DispAn(t *testing.T) {
	out := convertOneInstr(t, "\tlea 8(a0),a1")
	mustContain(t, out, "lea r10, 8(r9)")
}

func TestLea_Abs(t *testing.T) {
	out := convertOneInstr(t, "\tlea $F2000,a1")
	mustContain(t, out, "la r10, $F2000")
}

func TestLea_Indexed(t *testing.T) {
	out := convertOneInstr(t, "\tlea (8,a0,d0.l*4),a1")
	mustContain(t, out, "lsl.l r10, r10, #2")
	mustContain(t, out, "add.l r10, r10, r9")
	mustContain(t, out, "add.l r10, r10, #8")
}

// =====================================================================
// IndexAn lowering for general load
// =====================================================================

func TestMove_IndexAn_Load(t *testing.T) {
	out := convertOneInstr(t, "\tmove.l (8,a0,d0.w*4),d1")
	mustContain(t, out, "load.l r17, (r16)")
}

// =====================================================================
// Label preservation + comment passthrough
// =====================================================================

func TestLabelPreservation(t *testing.T) {
	out := convertOneInstr(t, "loop: move.l d0,d1")
	mustContain(t, out, "loop:")
	mustContain(t, out, "move.l r2, r1")
}

func TestSwap(t *testing.T) {
	out := convertOneInstr(t, "\tswap d0")
	mustContain(t, out, "rol.l r1, r1, #16")
}

func TestSt_Dn(t *testing.T) {
	out := convertOneInstr(t, "\tst d0")
	mustContain(t, out, "and.q r1, r1, #$FFFFFFFFFFFFFF00")
	mustContain(t, out, "or.q r1, r1, #$FF")
}

func TestSf_Dn(t *testing.T) {
	out := convertOneInstr(t, "\tsf d0")
	mustContain(t, out, "and.q r1, r1, #$FFFFFFFFFFFFFF00")
	mustNotContain(t, out, "or.q r1, r1, #$FF")
}

func TestComment_Passthrough(t *testing.T) {
	out := convertOneInstr(t, "* whole-line comment")
	mustContain(t, out, "; whole-line comment")
}
