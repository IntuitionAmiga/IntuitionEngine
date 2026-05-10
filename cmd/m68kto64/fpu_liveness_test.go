package main

import (
	"strings"
	"testing"
)

// =====================================================================
// Phase 7.4 — ShadowFPCC liveness pass
// =====================================================================

func TestLiveness_DeadFCmp_NoShadow(t *testing.T) {
	out := convertSrc(t, "\tfcmp.x fp0,fp1\n\trts\n")
	if strings.Contains(out, "; bit2 (Z)") {
		t.Errorf("fcmp with no downstream FP cc consumer should elide shadow:\n%s", out)
	}
}

func TestLiveness_FCmp_FBcc_ViaLabel_Live(t *testing.T) {
	// Label in between blocks fuse; liveness still sees fbeq downstream.
	out := convertSrc(t, "\tfcmp.x fp0,fp1\nL1:\n\tfbeq target\n")
	mustContain(t, out, "; bit2 (Z)")
}

func TestLiveness_FCmp_Then_FCmp_FirstDead(t *testing.T) {
	// Two fcmp ops with FBeq only after the second — first is dead.
	out := convertSrc(t, "\tfcmp.x fp0,fp1\n\tfcmp.x fp2,fp3\nL:\n\tfbeq target\n")
	// First fcmp emits dcmp but no shadow. Second emits shadow.
	count := strings.Count(out, "; bit2 (Z)")
	if count != 1 {
		t.Errorf("want 1 shadow emit (second fcmp), got %d:\n%s", count, out)
	}
}

func TestLiveness_FAdd_Live_EmitsShadow(t *testing.T) {
	out := convertSrc(t, "\tfadd.x fp0,fp1\nL:\n\tfbeq target\n")
	mustContain(t, out, "arith result vs zero")
	mustContain(t, out, "; bit2 (Z)")
}

func TestLiveness_FAdd_Dead_NoShadow(t *testing.T) {
	out := convertSrc(t, "\tfadd.x fp0,fp1\n\trts\n")
	if strings.Contains(out, "arith result vs zero") {
		t.Errorf("fadd with no consumer should elide shadow:\n%s", out)
	}
}

func TestLiveness_FSin_Live_EmitsShadow(t *testing.T) {
	out := convertSrc(t, "\tfsin.x fp0,fp1\nL:\n\tfbeq target\n")
	mustContain(t, out, "arith result vs zero")
}

func TestLiveness_FSin_Dead_NoShadow(t *testing.T) {
	out := convertSrc(t, "\tfsin.x fp0,fp1\n\trts\n")
	if strings.Contains(out, "arith result vs zero") {
		t.Errorf("fsin with no consumer should elide shadow:\n%s", out)
	}
}

func TestLiveness_FScale_Live(t *testing.T) {
	out := convertSrc(t, "\tfscale.x fp0,fp1\nL:\n\tfbeq target\n")
	mustContain(t, out, "arith result vs zero")
}

func TestLiveness_FScale_Dead(t *testing.T) {
	out := convertSrc(t, "\tfscale.x fp0,fp1\n\trts\n")
	if strings.Contains(out, "arith result vs zero") {
		t.Errorf("dead fscale should elide shadow:\n%s", out)
	}
}

func TestLiveness_FMOVE_FPSR_Dn_IsConsumer(t *testing.T) {
	// FMOVE.L FPSR,Dn reads ShadowFPCC. Live upstream producer must emit shadow.
	out := convertSrc(t, "\tfadd.x fp0,fp1\n\tfmove.l fpsr,d0\n")
	mustContain(t, out, "arith result vs zero")
}

func TestLiveness_FMOVE_Dn_FPSR_IsProducer(t *testing.T) {
	// FMOVE.L Dn,FPSR is a producer (split fold writes ShadowFPCC).
	// Liveness pass should treat it as a producer; downstream FBcc reads
	// what it wrote, so it doesn't gate shadow elision of *upstream* ops.
	l := LexLine("\tfmove.l d0,fpsr")
	if !isFPCCProducer(l) {
		t.Errorf("fmove.l Dn,fpsr should be a producer")
	}
}

func TestLiveness_FSave_NotConsumer(t *testing.T) {
	l := LexLine("\tfsave (a0)")
	if isFPCCConsumer(l) {
		t.Errorf("fsave should not be a cc consumer")
	}
}

func TestLiveness_FSin_NotConsumer(t *testing.T) {
	l := LexLine("\tfsin.x fp0,fp1")
	if isFPCCConsumer(l) {
		t.Errorf("fsin should be producer not consumer")
	}
}

func TestLiveness_LabelForcesLive(t *testing.T) {
	// A label between producer and rts forces "all live" — fadd before
	// label must emit shadow because control may re-enter at the label.
	out := convertSrc(t, "\tfadd.x fp0,fp1\nL:\n\trts\n")
	// rts is not a consumer; but label sets live = true at its position.
	// Since fadd appears before the label in backward walk, live is true
	// at fadd's index.
	mustContain(t, out, "arith result vs zero")
}

func TestLiveness_FBcc_IsConsumer(t *testing.T) {
	if !isFPCCConsumer(LexLine("\tfbeq target")) {
		t.Errorf("fbeq should be consumer")
	}
	if !isFPCCConsumer(LexLine("\tfdbeq d0,target")) {
		t.Errorf("fdbeq should be consumer")
	}
	if !isFPCCConsumer(LexLine("\tftrapeq")) {
		t.Errorf("ftrapeq should be consumer")
	}
	if !isFPCCConsumer(LexLine("\tfseq d0")) {
		t.Errorf("fseq should be consumer")
	}
}

func TestLiveness_NonFP_NotProducer(t *testing.T) {
	if isFPCCProducer(LexLine("\tadd.l d0,d1")) {
		t.Errorf("add.l should not be FP producer")
	}
}

func TestComputeFPCCLiveness_NoOps_EmptyMap(t *testing.T) {
	m := computeFPCCLiveness([]Line{LexLine("\trts")})
	if len(m) != 0 {
		t.Errorf("non-FP routine should produce empty liveness map, got %v", m)
	}
}
