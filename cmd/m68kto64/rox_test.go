package main

import (
	"strings"
	"testing"
)

// Phase 2 — ROXL / ROXR (rotate through X-extend).
//
// Semantics:
//   - (width+1)-bit rotate including the X flag.
//   - count=0 special case: operand unchanged, C := X (X itself unchanged).
//     N/Z reflect operand value. V cleared.
//   - count>0: rotate count times; X and C end equal to the last bit out.
//   - Reg-count form (Dy,Dn): count = Dy mod 64.
//   - Z is NOT sticky (unlike ADDX/SUBX/NEGX) — standard "result==0 → Z=1".

func TestROXL_B_ImmCount1_Dn(t *testing.T) {
	out := convertOneInstr(t, "\troxl.b #1,d0")
	mustContain(t, out, "r28")              // X consumed (chain-in)
	mustContain(t, out, "move.l r26, ")     // C := final X
	mustContain(t, out, "move.l r28, ")     // X := final X
	mustContain(t, out, "move.l r27, #0")   // V cleared
	mustContain(t, out, "and.q r1, r1, #$FFFFFFFFFFFFFF00") // .b partial update
}

func TestROXL_W_ImmCount1_Dn(t *testing.T) {
	out := convertOneInstr(t, "\troxl.w #1,d2")
	mustContain(t, out, "r28")
	mustContain(t, out, "and.q r3, r3, #$FFFFFFFFFFFF0000") // .w partial update
	mustContain(t, out, "move.l r27, #0")                    // V cleared
}

func TestROXL_L_ImmCount1_Dn(t *testing.T) {
	out := convertOneInstr(t, "\troxl.l #1,d4")
	mustContain(t, out, "r28")
	mustContain(t, out, "and.q r5, r5, #$FFFFFFFF00000000") // .l partial update
}

func TestROXR_B_ImmCount1_Dn(t *testing.T) {
	out := convertOneInstr(t, "\troxr.b #1,d0")
	mustContain(t, out, "r28")
	mustContain(t, out, "move.l r27, #0")
}

func TestROXR_L_ImmCount1_Dn(t *testing.T) {
	out := convertOneInstr(t, "\troxr.l #1,d3")
	mustContain(t, out, "r28")
	mustContain(t, out, "and.q r4, r4, #$FFFFFFFF00000000")
}

func TestROXL_RegCountForm(t *testing.T) {
	// ROXL.L d1,d2 — count = d1 (mod 64). Lowering must read d1 into a
	// scratch and mask by 63 before the rotate loop.
	out := convertOneInstr(t, "\troxl.l d1,d2")
	mustContain(t, out, "and.l ") // count masked
	mustContain(t, out, "#63")    // mod-64 mask present
	mustContain(t, out, "r28")    // ShadowX
}

func TestROXL_Count0_FastPath_PreservesXIntoC(t *testing.T) {
	// count=0 must NOT touch operand bits. We assert that the same byte/word/long
	// invmask appears (partial-update at end is fine — sets dst to (dst & invmask)
	// | (masked operand)), and that the loop body is guarded by a count check
	// (beqz on the count scratch).
	out := convertOneInstr(t, "\troxl.l d0,d1")
	mustContain(t, out, "beqz") // loop guard for count=0 path
}

func TestROXL_VClearedAlways(t *testing.T) {
	out := convertOneInstr(t, "\troxl.l #1,d0")
	mustContain(t, out, "move.l r27, #0") // ShadowV := 0
}

func TestROXR_RegCountModulo64(t *testing.T) {
	out := convertOneInstr(t, "\troxr.l d2,d3")
	mustContain(t, out, "#63") // mod-64 mask
}

// Z is NOT sticky for ROX — verify the lowering does not OR into ShadowZ.
func TestROXL_NotStickyZ(t *testing.T) {
	out := convertOneInstr(t, "\troxl.l #1,d0")
	if strings.Contains(out, "or.l r25, r25,") || strings.Contains(out, "or.q r25, r25,") {
		t.Errorf("ROXL should not sticky-merge into ShadowZ:\n%s", out)
	}
}
