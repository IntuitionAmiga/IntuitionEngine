package main

import (
	"testing"
)

// BSET / BCLR / BCHG — RMW bit ops with Z = NOT(pre-op bit).
//
// Bit index modulo: 32 for Dn destination, 8 for memory.
// BSET: set bit to 1   BCLR: clear to 0   BCHG: toggle
// N/V/C unaffected (same convention as BTST).

func TestBSET_ImmDn(t *testing.T) {
	out := convertOneInstr(t, "\tbset #5,d1")
	mustContain(t, out, "or.l ")          // set via OR with mask
	mustContain(t, out, "r25")            // Z shadow
	mustContain(t, out, "r2, r2, ")       // d1 (r2) RMW
}

func TestBCLR_ImmDn(t *testing.T) {
	out := convertOneInstr(t, "\tbclr #3,d2")
	mustContain(t, out, "and.l r3, r3, ") // clear via AND-NOT
	mustContain(t, out, "r25")
}

func TestBCHG_ImmDn(t *testing.T) {
	out := convertOneInstr(t, "\tbchg #7,d0")
	mustContain(t, out, "eor.l r1, r1, ") // toggle via XOR
	mustContain(t, out, "r25")
}

func TestBSET_DnDn_VariableBit(t *testing.T) {
	out := convertOneInstr(t, "\tbset d0,d1")
	mustContain(t, out, "and.l ")  // mask bit number mod 32
	mustContain(t, out, "#31")
	mustContain(t, out, "r25")
}

func TestBSET_ImmMem_ByteMod(t *testing.T) {
	out := convertOneInstr(t, "\tbset #2,(a0)")
	mustContain(t, out, "load.b ")
	mustContain(t, out, "store.b ")
	mustContain(t, out, "r25")
}

func TestBCLR_ImmMem(t *testing.T) {
	out := convertOneInstr(t, "\tbclr #4,(a1)")
	mustContain(t, out, "load.b ")
	mustContain(t, out, "store.b ")
	mustContain(t, out, "and.l ")
}
