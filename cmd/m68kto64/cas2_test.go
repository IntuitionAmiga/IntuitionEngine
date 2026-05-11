package main

import (
	"testing"
)

// Phase 5 — CAS2 (non-atomic dual-address compare-and-swap fallback).
//
// CAS2.<sz> Dc1:Dc2,Du1:Du2,(Rn1):(Rn2)
//   if (Rn1)==Dc1 && (Rn2)==Dc2:
//     (Rn1) := Du1; (Rn2) := Du2; Z := 1
//   else:
//     Dc1 := (Rn1); Dc2 := (Rn2); Z := 0
//
// Lowering: two sequential CAS-style load-cmp-store pairs. No atomicity.
// Multi-context guests racing on either address will observe lost updates.

func TestCAS2_L_BasicShape(t *testing.T) {
	out := convertOneInstr(t, "\tcas2.l d0:d1,d2:d3,(a0):(a1)")
	mustContain(t, out, "load.l ")  // load first EA
	mustContain(t, out, "store.l ") // store on equal path
	mustContain(t, out, "move.l r25, #0") // Z := 1 on equal (ShadowZ contract: 0 ⇔ Z=1)
	mustContain(t, out, "move.l r25, #1") // Z := 0 on not-equal
}

func TestCAS2_W_Size(t *testing.T) {
	out := convertOneInstr(t, "\tcas2.w d0:d1,d2:d3,(a0):(a1)")
	mustContain(t, out, "load.w ")
	mustContain(t, out, "store.w ")
}

func TestCAS2_ComparesBoth(t *testing.T) {
	// Both compares must appear — verify via "bne " count ≥ 2.
	out := convertOneInstr(t, "\tcas2.l d0:d1,d2:d3,(a0):(a1)")
	if countSubstring(out, "bne ") < 2 {
		t.Errorf("CAS2 must compare both EAs against Dc1/Dc2; got fewer than 2 'bne' branches:\n%s", out)
	}
}

func countSubstring(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}
