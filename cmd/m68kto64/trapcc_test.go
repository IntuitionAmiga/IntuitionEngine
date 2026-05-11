package main

import (
	"strings"
	"testing"
)

// Phase 4 — integer TRAPcc.
//
// 68020 conditional trap. 16 cc kinds (T/F/EQ/NE/HI/LS/CC/CS/MI/PL/VS/VC/
// GE/LT/GT/LE). Forms:
//   TRAPcc                          (bare)
//   TRAPcc.W #data16                (optional data — dropped)
//   TRAPcc.L #data32                (optional data — dropped)
// Lowering: invert cc → conditional branch over `syscall #18` (shared TRAPV
// vector — m68k vector 7).

func TestTRAPcc_BareEQ(t *testing.T) {
	out := convertOneInstr(t, "\ttrapeq")
	mustContain(t, out, "syscall #18")
	mustContain(t, out, "beqz r25, ") // EQ → take when Z=1; inverted branch when Z=0 (r25 != 0) skips
}

func TestTRAPcc_BareNE(t *testing.T) {
	out := convertOneInstr(t, "\ttrapne")
	mustContain(t, out, "syscall #18")
	mustContain(t, out, "bnez r25, ")
}

func TestTRAPcc_BareT_AlwaysTraps(t *testing.T) {
	out := convertOneInstr(t, "\ttrapt")
	mustContain(t, out, "syscall #18")
	// No branch — unconditional trap.
}

func TestTRAPcc_BareF_NeverTraps(t *testing.T) {
	// TRAPF is "never trap" — emit only the syscall under an always-false guard
	// (or nothing at all). Accept either pattern but the syscall #18 must be
	// unreachable: an `; m68kto64: trapf` diag is acceptable too.
	out := convertOneInstr(t, "\ttrapf")
	if strings.Contains(out, "syscall #18") && !strings.Contains(out, "bra ") {
		t.Errorf("trapf must not unconditionally syscall:\n%s", out)
	}
}

func TestTRAPcc_W_DataDropped(t *testing.T) {
	out := convertOneInstr(t, "\ttrapeq.w #$1234")
	mustContain(t, out, "syscall #18")
	// Diagnostic about dropped data operand.
	mustContain(t, out, "data operand dropped")
}

func TestTRAPcc_L_DataDropped(t *testing.T) {
	out := convertOneInstr(t, "\ttrapne.l #$12345678")
	mustContain(t, out, "syscall #18")
	mustContain(t, out, "data operand dropped")
}

func TestTRAPcc_GT_ShadowDecoder(t *testing.T) {
	// GT is the most complex cc — uses ShadowZ + ShadowN/V XOR decode.
	out := convertOneInstr(t, "\ttrapgt")
	mustContain(t, out, "syscall #18")
	mustContain(t, out, "r25") // ShadowZ touched
	mustContain(t, out, "r24") // ShadowN touched
	mustContain(t, out, "r27") // ShadowV touched
}

func TestTRAPcc_CS_Carry(t *testing.T) {
	out := convertOneInstr(t, "\ttrapcs")
	mustContain(t, out, "syscall #18")
	mustContain(t, out, "r26") // ShadowC
}

func TestTRAPcc_VS_Overflow(t *testing.T) {
	out := convertOneInstr(t, "\ttrapvs")
	mustContain(t, out, "syscall #18")
	mustContain(t, out, "r27") // ShadowV
}

