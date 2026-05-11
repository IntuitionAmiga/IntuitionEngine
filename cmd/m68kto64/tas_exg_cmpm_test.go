package main

import (
	"testing"
)

// TAS — test-and-set byte; N/Z from pre-op, V/C := 0, bit 7 set in dst.
// Non-atomic (no IE64 atomic byte primitive).

func TestTAS_Dn(t *testing.T) {
	out := convertOneInstr(t, "\ttas d1")
	mustContain(t, out, "#$FF")     // byte mask
	mustContain(t, out, "sext.b r24") // N := sign of pre-op byte
	mustContain(t, out, "or.l r2, r2, #$80") // set bit 7 in d1 (r2)
	mustContain(t, out, "move.l r26, #0")    // C := 0
	mustContain(t, out, "move.l r27, #0")    // V := 0
}

func TestTAS_Memory_NonAtomicDiag(t *testing.T) {
	out := convertOneInstr(t, "\ttas (a0)")
	mustContain(t, out, "non-atomic")
	mustContain(t, out, "load.b ")
	mustContain(t, out, "store.b ")
	mustContain(t, out, "or.l ")
}

// EXG — 32-bit register exchange.

func TestEXG_DnDn(t *testing.T) {
	out := convertOneInstr(t, "\texg d0,d1")
	mustContain(t, out, "move.l r17, r1") // save d0
	mustContain(t, out, "move.l r1, r2")  // d0 := d1
	mustContain(t, out, "move.l r2, r17") // d1 := saved
}

func TestEXG_AnAn(t *testing.T) {
	out := convertOneInstr(t, "\texg a0,a1")
	mustContain(t, out, "move.l r17, r9")
	mustContain(t, out, "move.l r9, r10")
	mustContain(t, out, "move.l r10, r17")
}

func TestEXG_DnAn(t *testing.T) {
	out := convertOneInstr(t, "\texg d2,a3")
	mustContain(t, out, "move.l ")
}

// CMPM — postinc-postinc memory compare.

func TestCMPM_L(t *testing.T) {
	out := convertOneInstr(t, "\tcmpm.l (a0)+,(a1)+")
	mustContain(t, out, "load.l r17, (r9)")
	mustContain(t, out, "add.l r9, r9, #4")
	mustContain(t, out, "load.l r18, (r10)")
	mustContain(t, out, "add.l r10, r10, #4")
}

func TestCMPM_W(t *testing.T) {
	out := convertOneInstr(t, "\tcmpm.w (a0)+,(a1)+")
	mustContain(t, out, "load.w ")
	mustContain(t, out, "add.l r9, r9, #2")
	mustContain(t, out, "add.l r10, r10, #2")
}

func TestCMPM_B(t *testing.T) {
	out := convertOneInstr(t, "\tcmpm.b (a0)+,(a1)+")
	mustContain(t, out, "load.b ")
	mustContain(t, out, "add.l r9, r9, #1")
	mustContain(t, out, "add.l r10, r10, #1")
}
