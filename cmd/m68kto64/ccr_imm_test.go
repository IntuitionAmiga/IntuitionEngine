package main

import (
	"strings"
	"testing"
)

// ANDI/ORI/EORI #imm,CCR/SR — Phase F3.
//
// Shadow contract recap:
//   ShadowC / ShadowV / ShadowX : canonical 0/1
//   ShadowZ                     : inverted (nonzero ⇔ Z=0)
//   ShadowN                     : sign-extended (−1 ⇔ N=1)

func TestANDI_CCR_ClearC(t *testing.T) {
	out := convertOneInstr(t, "\tandi #$FE,ccr")
	mustContain(t, out, "move.l r26, #0") // ShadowC=r26
	// Other shadows unchanged: bits 1..4 set in $FE, no move emitted for them.
	mustNotContain(t, out, "move.l r27, #0") // ShadowV (would emit if cleared)
}

func TestANDI_CCR_ClearZ_SetsShadowZNonzero(t *testing.T) {
	out := convertOneInstr(t, "\tandi #$FB,ccr") // $FB = bits 0,1,3,4 set; bit 2 (Z) cleared
	mustContain(t, out, "move.l r25, #1")        // ShadowZ=r25, force nonzero so Z=0
}

func TestANDI_CCR_ClearN(t *testing.T) {
	out := convertOneInstr(t, "\tandi #$F7,ccr") // bit 3 (N) cleared
	mustContain(t, out, "move.l r24, #0")        // ShadowN=r24, force 0
}

func TestANDI_CCR_ClearX(t *testing.T) {
	out := convertOneInstr(t, "\tandi #$EF,ccr") // bit 4 (X) cleared
	mustContain(t, out, "move.l r28, #0")        // ShadowX=r28
}

func TestORI_CCR_SetX(t *testing.T) {
	out := convertOneInstr(t, "\tori #$10,ccr")
	mustContain(t, out, "move.l r28, #1")
}

func TestORI_CCR_SetZ(t *testing.T) {
	out := convertOneInstr(t, "\tori #$04,ccr") // Z=1 → ShadowZ:=0
	mustContain(t, out, "move.l r25, #0")
}

func TestORI_CCR_SetN(t *testing.T) {
	out := convertOneInstr(t, "\tori #$08,ccr") // N=1 → ShadowN:=−1
	mustContain(t, out, "move.l r24, #-1")
}

func TestEORI_CCR_ToggleC(t *testing.T) {
	out := convertOneInstr(t, "\teori #$01,ccr")
	mustContain(t, out, "eor.l r26, r26, #1")
}

func TestEORI_CCR_ToggleZ(t *testing.T) {
	out := convertOneInstr(t, "\teori #$04,ccr")
	// Branched re-encode sequence:
	mustContain(t, out, "beqz r25,")
	mustContain(t, out, "move.l r25, #0")
	mustContain(t, out, "move.l r25, #1")
}

func TestEORI_CCR_ToggleN(t *testing.T) {
	out := convertOneInstr(t, "\teori #$08,ccr")
	mustContain(t, out, "and.l r22, r24, #1")
	mustContain(t, out, "eor.l r22, r22, #1")
	mustContain(t, out, "neg.q r24, r22")
}

// ,SR variant: low byte applies to shadows; upper byte discarded with diag.
func TestANDI_SR_UpperByteDiscarded(t *testing.T) {
	out := convertOneInstr(t, "\tandi #$F0FB,sr")
	mustContain(t, out, "; m68kto64: ANDI to SR upper byte discarded")
	// Low byte $FB still clears Z.
	mustContain(t, out, "move.l r25, #1")
}

// ,SR under -strict: diag, no error promotion.
func TestANDI_SR_StrictNoError(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	out, errs := c.ConvertSource("\tandi #$F0FB,sr\n")
	if errs != 0 {
		t.Errorf("-strict should not error on ANDI #imm,SR; got %d errs:\n%s", errs, out)
	}
}

// Non-immediate source is illegal per m68k spec for these forms.
func TestANDI_CCR_NonImmediateRejected(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	out, errs := c.ConvertSource("\tand.b d0,ccr\n")
	if errs == 0 || !strings.Contains(out, "ERROR") {
		t.Errorf("AND.B Dn,CCR should error (illegal m68k form); got:\n%s", out)
	}
}

// Identity case: ANDI #$FF clears nothing.
func TestANDI_CCR_AllBitsSet_Identity(t *testing.T) {
	out := convertOneInstr(t, "\tandi #$FF,ccr")
	mustNotContain(t, out, "move.l r26, #0")
	mustNotContain(t, out, "move.l r27, #0")
	mustNotContain(t, out, "move.l r25, #1")
	mustNotContain(t, out, "move.l r24, #0")
	mustNotContain(t, out, "move.l r28, #0")
}
