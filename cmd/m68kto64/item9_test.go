package main

import (
	"strings"
	"testing"
)

// Item 9: additional bugs found in self-review.

// =====================================================================
// 9a: DIVU/DIVS must trap on divide-by-zero (m68k vector 5).
// =====================================================================

func TestDivuL_TrapsOnZero(t *testing.T) {
	out := convertOneInstr(t, "\tdivu.l d0,d1")
	// Pre-divide zero check + syscall #5. Implementation uses
	// `bnez divisor, ok ; syscall #5 ; ok:` (branch around trap on nonzero).
	mustContain(t, out, "syscall #16")
	mustContain(t, out, "bnez r1")
}

func TestDivsL_TrapsOnZero(t *testing.T) {
	out := convertOneInstr(t, "\tdivs.l d0,d1")
	mustContain(t, out, "syscall #16")
}

func TestDivuW_TrapsOnZero(t *testing.T) {
	out := convertOneInstr(t, "\tdivu.w d0,d1")
	mustContain(t, out, "syscall #16")
}

func TestDivsW_TrapsOnZero(t *testing.T) {
	out := convertOneInstr(t, "\tdivs.w d0,d1")
	mustContain(t, out, "syscall #16")
}

// =====================================================================
// 9b: CLR on An is illegal m68k; transpiler should error in strict, comment
// out otherwise.
// =====================================================================

func TestClr_OnAn_Errors(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	out, errs := c.ConvertSource("\tclr.l a0\n")
	if errs == 0 {
		t.Errorf("strict mode should reject clr.l a0:\n%s", out)
	}
}

func TestClr_OnAn_NonStrict_DoesNotZeroMemory(t *testing.T) {
	// Non-strict: emit a diagnostic comment, do NOT generate `store.l r0,(r9)`
	// (which would zero memory at the address held by a0).
	out := convertOneInstr(t, "\tclr.l a0")
	if strings.Contains(out, "store.l r0, (r9)") {
		t.Errorf("clr.l a0 must not emit memory store at An:\n%s", out)
	}
}

// =====================================================================
// 9c: MOVEA.B does not exist; treat as error or coerce. MOVE byte to An via
// the byte-of-An sext path is also wrong (m68k allows neither).
// (Smoke test: at least don't emit `sext.w` for size=1 store to An.)
// =====================================================================

func TestMove_ByteToAn_Errors(t *testing.T) {
	// MOVEA.b is invalid m68k. Plain `move.b d0,a0` is also disallowed.
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	_, errs := c.ConvertSource("\tmove.b d0,a0\n")
	if errs == 0 {
		t.Errorf("strict mode should reject move.b to An")
	}
}

// =====================================================================
// 9d: ASL V works for variable count.
// =====================================================================

func TestAsl_V_VariableCount(t *testing.T) {
	out := convertOneInstr(t, "\tasl.l d0,d1")
	// Same V machinery — non-trivial r27 logic must appear.
	if !strings.Contains(out, "lsr.q r27") && !strings.Contains(out, "eor.q") {
		t.Errorf("variable-count ASL must compute V, not constant 0:\n%s", out)
	}
}
