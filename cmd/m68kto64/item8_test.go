package main

import (
	"strings"
	"testing"
)

// =====================================================================
// Item 8a: BCD Z-stickiness (m68k Z is "cleared if non-zero, unchanged
// otherwise" for ABCD/SBCD/NBCD).
// =====================================================================

func TestAbcd_StickyZ(t *testing.T) {
	out := convertOneInstr(t, "\tabcd d0,d1")
	// Sticky-Z lowering: r25 := r25 OR result_byte. Branchless.
	mustContain(t, out, "or.l r25, r25, r19")
	// The buggy form was `move.l r25, r19` (overwrites old Z).
	if strings.Contains(out, "move.l r25, r19") {
		t.Errorf("ABCD overwrites Z instead of preserving sticky:\n%s", out)
	}
}

func TestSbcd_StickyZ(t *testing.T) {
	out := convertOneInstr(t, "\tsbcd d0,d1")
	mustContain(t, out, "or.l r25, r25, r19")
}

func TestNbcd_StickyZ(t *testing.T) {
	out := convertOneInstr(t, "\tnbcd d0")
	mustContain(t, out, "or.l r25, r25, r19")
}

// =====================================================================
// Item 8b: separate ShadowX (r28) for chain-flag, distinct from ShadowC.
//
// AND/OR/EOR/NOT/MOVE clear C but leave X unchanged. ABCD/SBCD/NBCD read X
// as the chain-in carry. With C and X unified, an intervening AND would
// destroy X.
// =====================================================================

func TestShadowX_PreservedByAnd(t *testing.T) {
	out := convertSrc(t, "\tabcd d0,d1\n\tand.l #$F,d2\n\tabcd d3,d4\n")
	// Find the second ABCD's chain-add. It must read r28 (ShadowX), not
	// r26 (ShadowC), since AND clears C but must leave X.
	if !strings.Contains(out, "r28") {
		t.Errorf("ShadowX (r28) absent — X not separated from C:\n%s", out)
	}
}

func TestShadowX_AddSetsBoth(t *testing.T) {
	// ADD updates both C and X to the same value. After an add, X==C.
	out := convertOneInstr(t, "\tadd.l d0,d1")
	mustContain(t, out, "r26") // C
	mustContain(t, out, "r28") // X
}

func TestShadowX_AndDoesNotTouch(t *testing.T) {
	// AND clears C but does NOT update X.
	out := convertOneInstr(t, "\tand.l d0,d1")
	// C cleared explicitly.
	mustContain(t, out, "move.l r26, #0")
	// No store to r28 — X unchanged.
	if strings.Contains(out, "move.l r28") || strings.Contains(out, "and.l r28") || strings.Contains(out, "or.l r28") {
		t.Errorf("AND should not touch ShadowX (r28):\n%s", out)
	}
}

// =====================================================================
// Item 8c: ASL V flag — V=1 if MSB changed during shift.
// =====================================================================

func TestAsl_V_SetWhenSignChanges(t *testing.T) {
	// asl.l #1,d0: V=1 iff bit 30 of pre-shift differs from bit 31.
	out := convertOneInstr(t, "\tasl.l #1,d0")
	// V computation must touch r27 with non-zero logic (eor or branch).
	// Existing impl emits `move.l r27, #0` — flag the buggy form.
	idx := strings.LastIndex(out, "move.l r27")
	if idx < 0 {
		// Possibly emitted as branch-set; that's fine.
		return
	}
	// If there's only a "move.l r27, #0", then ASL V is approximated as 0.
	tail := out[idx:]
	if strings.HasPrefix(tail, "move.l r27, #0") && !strings.Contains(out, "lsr.q r27") &&
		!strings.Contains(out, "eor.l r27") && !strings.Contains(out, "and.l r27") {
		t.Errorf("ASL V left as constant 0 — m68k semantic requires sign-change detection:\n%s", out)
	}
}

// =====================================================================
// Item 8d: ifeq / ifne real expression eval.
// =====================================================================

func TestDirective_Ifeq_LowersToExprComparison(t *testing.T) {
	out := convertSrc(t, "\tifeq FOO\n\tdc.l 1\n\tendc\n")
	// Real: ie64asm supports `==` in if-expr. Lower to `if (FOO) == 0`.
	mustContain(t, out, "if (FOO) == 0")
}

func TestDirective_Ifne_LowersToExprComparison(t *testing.T) {
	out := convertSrc(t, "\tifne FOO\n\tdc.l 1\n\tendc\n")
	mustContain(t, out, "if (FOO) != 0")
}
