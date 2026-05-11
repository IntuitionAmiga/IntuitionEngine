package main

import (
	"testing"
)

// Phase 3 — CHK2 (bounds-test against EA bound-pair).
//
// CHK2.<sz> <ea>,Rn  reads two operands of size <sz> from contiguous memory
// at <ea>: lower bound at <ea>, upper bound at <ea>+size. Tests Rn against
// [lower, upper] inclusive. On fail → syscall #17 (CHK vector, relocated).
// Dn form: signed comparison (size-aware sign-extension of bounds).
// An form: always treated as 32-bit signed comparison.

func TestCHK2_B_Dn_Indirect(t *testing.T) {
	out := convertOneInstr(t, "\tchk2.b (a0),d1")
	// Two byte loads, the second offset by +1 from EA.
	mustContain(t, out, "load.b ")
	mustContain(t, out, "syscall #17") // shared CHK vector
	mustContain(t, out, "blt ")        // lower-bound fail (signed)
	mustContain(t, out, "bgt ")        // upper-bound fail (signed)
}

func TestCHK2_W_Dn_Indirect(t *testing.T) {
	out := convertOneInstr(t, "\tchk2.w (a0),d2")
	mustContain(t, out, "load.w ")
	mustContain(t, out, "syscall #17")
}

func TestCHK2_L_Dn_Indirect(t *testing.T) {
	out := convertOneInstr(t, "\tchk2.l (a0),d3")
	mustContain(t, out, "load.l ")
	mustContain(t, out, "syscall #17")
}

func TestCHK2_W_An_Indirect(t *testing.T) {
	// An form — 32-bit signed compare per 68020 spec; bounds sized to .w
	// still get sign-extended to long.
	out := convertOneInstr(t, "\tchk2.w (a0),a1")
	mustContain(t, out, "load.w ")
	mustContain(t, out, "syscall #17")
	// sext from .w → .l so An comparison is full-long.
	mustContain(t, out, "sext.w ")
}

func TestCHK2_L_Disp_Dn(t *testing.T) {
	out := convertOneInstr(t, "\tchk2.l 8(a0),d0")
	mustContain(t, out, "syscall #17")
	mustContain(t, out, "load.l ")
}

func TestCHK2_PassPath_NoTrap(t *testing.T) {
	// Both branches must skip past the syscall — verify a forward branch to a
	// label past `syscall #17` exists (the pass label).
	out := convertOneInstr(t, "\tchk2.l (a0),d0")
	mustContain(t, out, "syscall #17")
	mustContain(t, out, "bra ") // unconditional skip when in-bounds
}
