package main

import (
	"strings"
	"testing"
)

// =====================================================================
// Phase A: shadow CCR maintenance + standalone Bcc consumption.
//
// Shadow registers:
//   r24 — sign-extended-to-64 last result (N)
//   r25 — width-masked last result (Z: zero iff masked == 0)
//   r26 — carry/borrow from last add/sub/cmp (0/1)
//   r27 — signed overflow from last add/sub/cmp (0/1)
// =====================================================================

func TestShadowCCR_TstSetsNZ(t *testing.T) {
	out := convertSrc(t, "\ttst.l d0\n\tnop\n\tbeq target\n")
	mustContain(t, out, "sext.l r24")
	mustContain(t, out, "r25")
	mustNotContain(t, out, "FUSE-MISS")
}

func TestShadowCCR_TstFarBeq_UsesShadow(t *testing.T) {
	out := convertSrc(t, "\ttst.l d0\n\tnop\n\tbeq target\n")
	// BEQ consumes shadow Z (r25).
	mustContain(t, out, "beqz r25, target")
}

func TestShadowCCR_TstFarBmi_UsesShadowN(t *testing.T) {
	out := convertSrc(t, "\ttst.l d0\n\tnop\n\tbmi target\n")
	mustContain(t, out, "bltz r24, target")
}

func TestShadowCCR_TstFarBpl_UsesShadowN(t *testing.T) {
	out := convertSrc(t, "\ttst.l d0\n\tnop\n\tbpl target\n")
	mustContain(t, out, "bgez r24, target")
}

func TestShadowCCR_AddFarBne(t *testing.T) {
	out := convertSrc(t, "\tadd.l d0,d1\n\tnop\n\tbne target\n")
	mustContain(t, out, "bnez r25, target")
	mustNotContain(t, out, "FUSE-MISS")
}

func TestShadowCCR_SubSetsCarry(t *testing.T) {
	// SUB at .l: shadow C must be 1 iff dst (uint) < src (uint).
	out := convertSrc(t, "\tsub.l d0,d1\n\tnop\n\tbcs target\n")
	mustContain(t, out, "r26") // C shadow updated
	mustContain(t, out, "bnez r26, target")
}

func TestShadowCCR_AddSetsCarry(t *testing.T) {
	out := convertSrc(t, "\tadd.l d0,d1\n\tnop\n\tbcc target\n")
	mustContain(t, out, "r26")
	mustContain(t, out, "beqz r26, target")
}

func TestShadowCCR_CmpFarBlt_UsesShadows(t *testing.T) {
	out := convertSrc(t, "\tcmp.l d0,d1\n\tnop\n\tblt target\n")
	// BLT signed = N XOR V. Without fusing, must use shadow N XOR shadow V.
	mustContain(t, out, "r24")
	mustContain(t, out, "r27")
	mustContain(t, out, "bne") // branch on XOR-not-equal
}

func TestShadowCCR_ClrSetsZ1(t *testing.T) {
	out := convertSrc(t, "\tclr.l d0\n\tbeq target\n")
	// CLR sets Z=1 (always taken). Shadow Z value should be 0 (the cleared
	// destination), so beqz r25 always branches.
	mustContain(t, out, "beqz r25, target")
}

func TestShadowCCR_AndClearsCV(t *testing.T) {
	out := convertSrc(t, "\tand.l d0,d1\n\tnop\n\tbcs target\n")
	// AND clears C → shadow C := 0 → BCS never taken.
	// Output must contain explicit r26 := 0.
	mustContain(t, out, "move.l r26, #0")
}

// =====================================================================
// DBcc against shadows
// =====================================================================

func TestDBcc_DBNE_AgainstShadow(t *testing.T) {
	// dbne dn,L: if Z=0, skip decrement (condition true); else decrement+test.
	out := convertSrc(t, "\ttst.l d1\n\tdbne d0,loop\n")
	// DBNE evaluates against shadow Z (r25). Skip-decrement means branch
	// past decrement when Z is 0 (i.e. r25 nonzero).
	mustContain(t, out, "r25")
}

// =====================================================================
// Scc against shadows
// =====================================================================

func TestScc_SEQ_AgainstShadow(t *testing.T) {
	out := convertSrc(t, "\ttst.l d0\n\tseq d1\n")
	// SEQ d1 = $FF if Z=1 else $00. Reads shadow Z.
	mustContain(t, out, "r25")
	mustNotContain(t, out, "FUSE-MISS")
}

func TestScc_SNE_AgainstShadow(t *testing.T) {
	out := convertSrc(t, "\ttst.l d0\n\tsne d1\n")
	mustContain(t, out, "r25")
	mustNotContain(t, out, "FUSE-MISS")
}

// =====================================================================
// Liveness — adjacent fuse still wins; shadow only emitted otherwise.
// (Optional; skip if test is too brittle for current emit shape.)
// =====================================================================

func TestShadowCCR_AdjacentFuseStillWins(t *testing.T) {
	out := convertSrc(t, "\tcmp.l d0,d1\n\tbeq target\n")
	// Fused adjacent: emits "beq r2, r1, target" — two-reg form.
	mustContain(t, out, "beq r2, r1, target")
	// And does NOT emit unfused shadow consumption "beqz r25, target".
	if strings.Contains(out, "beqz r25, target") {
		t.Errorf("adjacent fuse should win; output:\n%s", out)
	}
}
