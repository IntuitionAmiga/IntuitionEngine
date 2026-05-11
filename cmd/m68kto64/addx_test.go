package main

import (
	"strings"
	"testing"
)

// Phase 1 — ADDX / SUBX / NEGX consumer wiring.
//
// Verifies:
//   - X (ShadowX = r28) is chain-added/subtracted into the result.
//   - Sticky-Z semantics: m68k ADDX/SUBX/NEGX clear Z only when the result is
//     nonzero. Lowering must OR the masked result into ShadowZ rather than
//     overwriting it (matches ABCD pattern at phase_b.go:140).
//   - C/V shadows are computed with X factored into the sum/difference (cannot
//     reuse emitShadowAddCV / emitShadowSubCV verbatim — those omit X).
//   - X := C after the op (matches emitShadowCopyToX at ccr_shadow.go:71).
//   - .B/.W/.L sizes; Dn,Dn and -(Ay),-(Ax) operand forms.
//   - NEGX: 0 - dst - X with the same shadow rules as SUBX(src=0).

func TestADDX_B_DnDn_BasicChainIn(t *testing.T) {
	out := convertOneInstr(t, "\taddx.b d1,d2")
	// Width-mask both operands then add with X chain-in.
	mustContain(t, out, "#$FF")
	mustContain(t, out, "r28") // ShadowX consumed
	// Sticky Z, X := C, partial-update merge into d2 (r3).
	mustContain(t, out, "or.l r25, r25, ")           // sticky-Z merge
	mustContain(t, out, "move.l r28, r26")           // X := C
	mustContain(t, out, "and.q r3, r3, #$FFFFFFFFFFFFFF00")
}

func TestADDX_W_DnDn_PartialUpdate(t *testing.T) {
	out := convertOneInstr(t, "\taddx.w d3,d4")
	mustContain(t, out, "#$FFFF")
	mustContain(t, out, "r28")
	mustContain(t, out, "and.q r5, r5, #$FFFFFFFFFFFF0000") // .w invmask
	mustContain(t, out, "or.q r5, r5, ")
}

func TestADDX_L_DnDn_FullWidth(t *testing.T) {
	out := convertOneInstr(t, "\taddx.l d0,d1")
	mustContain(t, out, "r28")
	mustContain(t, out, "or.l r25, r25, ") // sticky-Z
	mustContain(t, out, "move.l r28, r26") // X := C
}

func TestADDX_VOverflow_SignBitPath(t *testing.T) {
	// V must be computed (defined for ADDX). Look for the standard
	// (NOT(d^s) AND (d^r)) >> signBit pattern via ShadowTmp1/2.
	out := convertOneInstr(t, "\taddx.l d0,d1")
	mustContain(t, out, "r22") // ShadowTmp1 (src masked)
	mustContain(t, out, "r23") // ShadowTmp2 (d XOR r)
	mustContain(t, out, "not.q r27, r27") // NOT(d XOR s) for ADD form
	mustContain(t, out, "lsr.q r27, r27, #31") // .l sign bit
}

func TestSUBX_B_DnDn_BorrowChain(t *testing.T) {
	out := convertOneInstr(t, "\tsubx.b d2,d3")
	mustContain(t, out, "r28")           // X read
	mustContain(t, out, "or.l r25, r25, ")
	mustContain(t, out, "move.l r28, r26") // X := C
}

func TestSUBX_L_DnDn_FullWidth(t *testing.T) {
	out := convertOneInstr(t, "\tsubx.l d4,d5")
	mustContain(t, out, "r28")
	mustContain(t, out, "lsr.q r27, r27, #31")
}

func TestADDX_B_PredecPair(t *testing.T) {
	out := convertOneInstr(t, "\taddx.b -(a1),-(a0)")
	mustContain(t, out, "sub.l r10, r10, #1") // -(a1) predec
	mustContain(t, out, "sub.l r9, r9, #1")   // -(a0) predec
	mustContain(t, out, "load.b r")
	mustContain(t, out, "store.b r")
	mustContain(t, out, "r28") // ShadowX still consumed
}

func TestADDX_W_PredecPair_SizeAware(t *testing.T) {
	// .w must decrement by 2, not 1.
	out := convertOneInstr(t, "\taddx.w -(a1),-(a0)")
	mustContain(t, out, "sub.l r10, r10, #2")
	mustContain(t, out, "sub.l r9, r9, #2")
	mustContain(t, out, "load.w r")
	mustContain(t, out, "store.w r")
	// No byte-only artefacts.
	if strings.Contains(out, "sub.l r10, r10, #1") {
		t.Errorf("addx.w must decrement by 2, found #1 predec:\n%s", out)
	}
}

func TestADDX_L_PredecPair_SizeAware(t *testing.T) {
	out := convertOneInstr(t, "\taddx.l -(a1),-(a0)")
	mustContain(t, out, "sub.l r10, r10, #4")
	mustContain(t, out, "sub.l r9, r9, #4")
	mustContain(t, out, "load.l r")
	mustContain(t, out, "store.l r")
}

func TestSUBX_L_PredecPair_SizeAware(t *testing.T) {
	out := convertOneInstr(t, "\tsubx.l -(a1),-(a0)")
	mustContain(t, out, "sub.l r10, r10, #4")
	mustContain(t, out, "sub.l r9, r9, #4")
}

func TestNEGX_B_Dn(t *testing.T) {
	out := convertOneInstr(t, "\tnegx.b d2")
	mustContain(t, out, "r28") // X chain-in
	mustContain(t, out, "or.l r25, r25, ")
	mustContain(t, out, "move.l r28, r26")
	mustContain(t, out, "and.q r3, r3, #$FFFFFFFFFFFFFF00")
}

func TestNEGX_W_Dn(t *testing.T) {
	out := convertOneInstr(t, "\tnegx.w d1")
	mustContain(t, out, "r28")
	mustContain(t, out, "and.q r2, r2, #$FFFFFFFFFFFF0000")
}

func TestNEGX_L_Dn(t *testing.T) {
	out := convertOneInstr(t, "\tnegx.l d0")
	mustContain(t, out, "r28")
	mustContain(t, out, "or.l r25, r25, ")
	mustContain(t, out, "move.l r28, r26")
}

// Sticky-Z guard: a known-non-zero ADDX result must not clobber a pre-set
// ShadowZ to "zero" — verify by absence of unconditional `move.l r25, #0`.
func TestADDX_DoesNotOverwriteShadowZ(t *testing.T) {
	out := convertOneInstr(t, "\taddx.l d0,d1")
	if strings.Contains(out, "move.l r25, #0") {
		t.Errorf("ADDX clobbered ShadowZ instead of sticky-merge:\n%s", out)
	}
}
