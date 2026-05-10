package main

import (
	"strings"
	"testing"
)

// =====================================================================
// Bug 1: fused CMPA.W must compare full An, not An masked at .w.
// =====================================================================

func TestFuse_CmpaW_PreservesFullAn(t *testing.T) {
	out := convertSrc(t, "\tcmpa.w d0,a0\n\tbeq L\nL:\n\trts\n")
	// Source word sign-extended to .l; An NOT sign-extended-at-word.
	mustContain(t, out, "sext.w r17, r1")
	// Destination must be the FULL An reg (r9), not a masked sext.w r9.
	mustNotContain(t, out, "sext.w r18, r9")
	// Final compare uses An at full width.
	if !strings.Contains(out, "beq r9, r17, L") {
		t.Errorf("expected fused beq with full An:\n%s", out)
	}
}

func TestFuse_CmpaL_FullCompare(t *testing.T) {
	out := convertSrc(t, "\tcmpa.l d0,a0\n\tbeq L\nL:\n\trts\n")
	mustContain(t, out, "beq r9, r1, L")
}

// =====================================================================
// Bug 2: BLE/BLS polarity (Z-test side).
// =====================================================================

func TestShadowBcc_BLE_TakesOnZ1(t *testing.T) {
	// BLE = Z=1 OR (N XOR V)=1. Z=1 means r25==0, so the Z-leg must use
	// `beqz r25, take`, not `bnez r25, take`.
	out := convertSrc(t, "\tcmp.l d0,d1\n\tnop\n\tble L\nL:\n\trts\n")
	// Find the take/skip label and check that the Z-side branch is beqz.
	if strings.Contains(out, "bnez r25, ") && !strings.Contains(out, "beqz r25, ") {
		t.Errorf("BLE Z-leg uses wrong polarity (bnez instead of beqz):\n%s", out)
	}
}

func TestShadowBcc_BLS_TakesOnZ1(t *testing.T) {
	out := convertSrc(t, "\tcmp.l d0,d1\n\tnop\n\tbls L\nL:\n\trts\n")
	if strings.Contains(out, "bnez r25, ") && !strings.Contains(out, "beqz r25, ") {
		t.Errorf("BLS Z-leg polarity wrong:\n%s", out)
	}
}

// =====================================================================
// Bug 3: DBEQ/DBNE polarity in skip-on-cc-true.
// =====================================================================

func TestDBcc_DBEQ_SkipOnZ1(t *testing.T) {
	// DBEQ skips decrement when cc=true=Z=1=(r25==0). Skip line must be
	// `beqz ShadowZ, skip`, not `bnez`.
	out := convertSrc(t, "\ttst.l d0\n\tdbeq d0,L\nL:\n\trts\n")
	mustContain(t, out, "beqz r25,")
}

func TestDBcc_DBNE_SkipOnZ0(t *testing.T) {
	out := convertSrc(t, "\ttst.l d0\n\tdbne d0,L\nL:\n\trts\n")
	// Skip when r25!=0 (Z=0).
	mustContain(t, out, "bnez r25,")
}

// =====================================================================
// Bug 4: ADDA.W/SUBA.W with sign-bit-set immediate must sign-extend.
// =====================================================================

func TestAddaW_NegativeImm_SignExtends(t *testing.T) {
	out := convertOneInstr(t, "\tadda.w #$FFFF,a0")
	// Materialise word imm and sign-extend before adding to An.
	mustContain(t, out, "move.w r17, #$FFFF")
	mustContain(t, out, "sext.w r17, r17")
	mustContain(t, out, "add.l r9, r9, r17")
	// Direct `add.l r9, r9, #$FFFF` is the broken form.
	if strings.Contains(out, "add.l r9, r9, #$FFFF") {
		t.Errorf("adda.w emitted unsigned immediate add — wrong:\n%s", out)
	}
}

func TestSubaW_NegativeImm_SignExtends(t *testing.T) {
	out := convertOneInstr(t, "\tsuba.w #$FFFF,a0")
	mustContain(t, out, "sext.w r17")
	mustContain(t, out, "sub.l r9, r9, r17")
	if strings.Contains(out, "sub.l r9, r9, #$FFFF") {
		t.Errorf("suba.w emitted unsigned immediate sub — wrong:\n%s", out)
	}
}

func TestAddaW_PositiveImm_StillCorrect(t *testing.T) {
	// $1234 has bit 15 clear; sign-extend leaves it positive — no
	// regression on the existing positive-imm path.
	out := convertOneInstr(t, "\tadda.w #$1234,a0")
	mustContain(t, out, "sext.w r17")
	mustContain(t, out, "add.l r9, r9, r17")
}
