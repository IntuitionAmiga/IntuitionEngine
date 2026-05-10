package main

import (
	"strings"
	"testing"
)

// =====================================================================
// Shift C exact (last-bit-shifted-out)
// =====================================================================

func TestShift_LSL_C_FromBit(t *testing.T) {
	out := convertOneInstr(t, "\tlsl.l #4,d0")
	// LSL: C = bit (width - count) of src.  width=32, count=4 → bit 28.
	mustContain(t, out, "lsr.q r26, r1,") // C extracted from r1 via lsr
	mustContain(t, out, "and.q r26, r26, #1")
}

func TestShift_LSR_C_FromBit(t *testing.T) {
	out := convertOneInstr(t, "\tlsr.l #1,d0")
	// LSR: C = bit (count-1) of src.  count=1 → bit 0.
	mustContain(t, out, "lsr.q r26, r1,")
}

func TestShift_CountZero_ClearsC(t *testing.T) {
	// At runtime when count=0 m68k clears C; we emit a `beqz` early-out.
	out := convertOneInstr(t, "\tlsl.l #0,d0")
	mustContain(t, out, "beqz r22") // count==0 branch
	mustContain(t, out, "move.l r26, #0")
}

// =====================================================================
// RTR
// =====================================================================

func TestRtr_PopsCcrAndPc(t *testing.T) {
	out := convertOneInstr(t, "\trtr")
	// 16-bit CCR load at sp.
	mustContain(t, out, "load.w r17, (r30)")
	// SP advance over CCR (+2) and PC (+4).
	if !strings.Contains(out, "add.l r30, r30, #2") || !strings.Contains(out, "add.l r30, r30, #4") {
		t.Errorf("missing CCR(+2) or PC(+4) sp advance:\n%s", out)
	}
	// PC pop + jmp.
	mustContain(t, out, "load.l r17, (r30)")
	mustContain(t, out, "jmp (r17)")
}

// =====================================================================
// Memory bit-field operands
// =====================================================================

func TestBfins_Mem(t *testing.T) {
	out := convertOneInstr(t, "\tbfins d0,(a0){#0:#8}")
	// Loads 32-bit window from (a0), modifies, stores.
	mustContain(t, out, "load.l r18, (r16)")
	mustContain(t, out, "store.l r18, (r16)")
}

func TestBfclr_Mem(t *testing.T) {
	out := convertOneInstr(t, "\tbfclr (a0){#0:#4}")
	mustContain(t, out, "load.l r18, (r16)")
	mustContain(t, out, "store.l r18, (r16)")
}

func TestBftst_Mem(t *testing.T) {
	out := convertOneInstr(t, "\tbftst (a0){#0:#8}")
	mustContain(t, out, "load.l r18, (r16)")
	mustContain(t, out, "sext.l r24, r19") // shadow N from extracted field
}

func TestBfffo_Mem(t *testing.T) {
	out := convertOneInstr(t, "\tbfffo (a0){#0:#8},d1")
	mustContain(t, out, "load.l r18, (r16)")
	mustContain(t, out, "clz r2, r19")
}
