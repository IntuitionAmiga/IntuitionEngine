package main

import (
	"strings"
	"testing"
)

// ILLEGAL — m68k vector 4. Maps to syscall #19 (newly registered).
func TestIllegal(t *testing.T) {
	out := convertOneInstr(t, "\tillegal")
	mustContain(t, out, "syscall #19")
}

// RTE — full implementation. Pops 16-bit SR, unpacks CCR bits (X/N/Z/V/C)
// into shadow regs, pops 32-bit PC, jumps.
func TestRTE_FullUnpack(t *testing.T) {
	out := convertOneInstr(t, "\trte")
	mustContain(t, out, "load.w r17, (r30)") // pop SR
	mustContain(t, out, "add.l r30, r30, #2")
	// CCR unpack (helper-emitted): C (and.l r26,...,#1), V (lsr #1), Z (lsr #2 + eor #1), N (lsr #3 + neg), X (lsr #4).
	mustContain(t, out, "and.l r26, r17, #1") // C
	mustContain(t, out, "lsr.l r28, r17, #4") // X bit
	mustContain(t, out, "neg.q r24, ")        // N sign-extend
	// Pop PC.
	mustContain(t, out, "load.l r17, (r30)")
	mustContain(t, out, "add.l r30, r30, #4")
	mustContain(t, out, "jmp (r17)")
	if strings.Contains(out, "SR pop dropped") {
		t.Errorf("RTE should not strip SR pop in full implementation:\n%s", out)
	}
}

// STOP — full implementation. Materialises #imm SR value, unpacks CCR bits,
// then halts via syscall #21.
func TestSTOP_FullImpl(t *testing.T) {
	out := convertOneInstr(t, "\tstop #$2700")
	mustContain(t, out, "move.l r17, #$2700") // SR value
	mustContain(t, out, "and.l r26, r17, #1") // C bit unpack from low byte
	mustContain(t, out, "lsr.l r28, r17, #4") // X bit
	mustContain(t, out, "syscall #21")        // halt-until-interrupt
	if strings.Contains(out, "stripped STOP") {
		t.Errorf("STOP should not be stripped in full implementation:\n%s", out)
	}
}

// RESET — full implementation. Delegated to host via syscall #22.
func TestRESET_FullImpl(t *testing.T) {
	out := convertOneInstr(t, "\treset")
	mustContain(t, out, "syscall #22")
	if strings.Contains(out, "stripped RESET") {
		t.Errorf("RESET should not be stripped in full implementation:\n%s", out)
	}
}
