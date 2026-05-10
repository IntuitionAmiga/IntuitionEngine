package main

import (
	"strings"
	"testing"
)

// =====================================================================
// Phase 7.4 FCMP+FBcc adjacent fuse
// =====================================================================

func TestFCmpFBeq_Fused(t *testing.T) {
	out := convertSrc(t, "\tfcmp.x fp1,fp0\n\tfbeq target\n")
	mustContain(t, out, "dcmp r17, f0, f2")
	mustContain(t, out, "beq r17, r0, target")
	// Fused path must not emit full ShadowFPCC composition.
	if strings.Contains(out, "; bit2 (Z)") {
		t.Errorf("fused fcmp+fbeq should skip full shadow update:\n%s", out)
	}
}

func TestFCmpFBne_Fused(t *testing.T) {
	out := convertSrc(t, "\tfcmp.x fp1,fp0\n\tfbne target\n")
	mustContain(t, out, "bne r17, r0, target")
}

func TestFCmpFBlt_Fused(t *testing.T) {
	out := convertSrc(t, "\tfcmp.x fp1,fp0\n\tfblt target\n")
	mustContain(t, out, "blt r17, r0, target")
}

func TestFCmpFBgt_Fused(t *testing.T) {
	out := convertSrc(t, "\tfcmp.x fp1,fp0\n\tfbgt target\n")
	mustContain(t, out, "bgt r17, r0, target")
}

func TestFTstFBeq_Fused(t *testing.T) {
	out := convertSrc(t, "\tftst.x fp0\n\tfbeq target\n")
	mustContain(t, out, "dload f12,")
	mustContain(t, out, "dcmp r17, f0, f12")
	mustContain(t, out, "beq r17, r0, target")
}

func TestFCmpFBor_NotFused_NaNAware(t *testing.T) {
	// FBOR is NaN-aware; not in the fused-cc set. Should take the
	// standalone-shadow path with full ShadowFPCC update.
	out := convertSrc(t, "\tfcmp.x fp1,fp0\n\tfbor target\n")
	mustContain(t, out, "; bit2 (Z)") // shadow update fired
}

func TestFCmp_LabelBetween_Suppresses_Fuse(t *testing.T) {
	out := convertSrc(t, "\tfcmp.x fp1,fp0\nL1:\n\tfbeq target\n")
	// Shadow update must fire (fuse suppressed by label).
	mustContain(t, out, "; bit2 (Z)")
}

func TestFCmp_LabelledProducer_Suppresses_Fuse(t *testing.T) {
	out := convertSrc(t, "L1:\tfcmp.x fp1,fp0\n\tfbeq target\n")
	mustContain(t, out, "; bit2 (Z)")
}

func TestFCmpFBst_Fused_AlwaysTaken(t *testing.T) {
	out := convertSrc(t, "\tfcmp.x fp1,fp0\n\tfbst target\n")
	mustContain(t, out, "bra target")
}

func TestFCmpFBsf_Fused_NeverTaken(t *testing.T) {
	out := convertSrc(t, "\tfcmp.x fp1,fp0\n\tfbsf target\n")
	mustContain(t, out, "never taken")
}

func TestFCmpFBseq_RaisesIOFlag(t *testing.T) {
	out := convertSrc(t, "\tfcmp.x fp1,fp0\n\tfbseq target\n")
	mustContain(t, out, "fmovsr r18")
	mustContain(t, out, "or.l r18, r18, #1")
	mustContain(t, out, "fmovsc r18")
	mustContain(t, out, "beq r17, r0, target")
}

func TestNoFuse_Flag_DisablesFPFuse(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	c.noFlagsFuse = true
	out, _ := c.ConvertSource("\tfcmp.x fp1,fp0\n\tfbeq target\n")
	// With fuse off, shadow update path emits.
	if !strings.Contains(out, "; bit2 (Z)") {
		t.Errorf("noFlagsFuse should disable FP fuse:\n%s", out)
	}
}
