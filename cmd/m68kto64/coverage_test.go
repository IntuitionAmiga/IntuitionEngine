package main

import (
	"strings"
	"testing"
)

// =====================================================================
// Per-condition shadow Bcc coverage
// =====================================================================

func TestShadowBcc_AllConditions(t *testing.T) {
	cases := map[string][]string{
		"beq":    {"beqz r25, L"},
		"bne":    {"bnez r25, L"},
		"bmi":    {"bltz r24, L"},
		"bpl":    {"bgez r24, L"},
		"bcs":    {"bnez r26, L"},
		"bcc":    {"beqz r26, L"},
		"bvs":    {"bnez r27, L"},
		"bvc":    {"beqz r27, L"},
		"blt":    {"eor.q", "bnez"},
		"bge":    {"eor.q", "beqz"},
		"bgt":    {"beqz r25", "eor.q"},
		"ble":    {"beqz r25", "eor.q"},
		"bhi":    {"bnez r26", "beqz r25"},
		"bls":    {"bnez r26", "beqz r25"},
	}
	for mnem, wants := range cases {
		out := convertSrc(t, "\ttst.l d0\n\tnop\n\t"+mnem+" L\nL:\n\trts\n")
		for _, w := range wants {
			if !strings.Contains(out, w) {
				t.Errorf("%s: missing %q\n%s", mnem, w, out)
			}
		}
	}
}

// =====================================================================
// Per-condition Scc coverage
// =====================================================================

func TestShadowScc_AllConditions(t *testing.T) {
	for _, mnem := range []string{
		"seq", "sne", "smi", "spl", "scs", "scc", "svs", "svc",
		"slt", "sge", "sgt", "sle", "shi", "sls", "st", "sf",
	} {
		out := convertSrc(t, "\ttst.l d0\n\t"+mnem+" d1\n")
		if strings.Contains(out, "ERROR") || strings.Contains(out, "FUSE-MISS") {
			t.Errorf("%s: unexpected diagnostic\n%s", mnem, out)
		}
	}
}

// =====================================================================
// Per-condition DBcc coverage
// =====================================================================

func TestShadowDBcc_AllConditions(t *testing.T) {
	for _, mnem := range []string{
		"dbeq", "dbne", "dbmi", "dbpl", "dbcs", "dbcc", "dbvs", "dbvc",
		"dblt", "dbge", "dbgt", "dble", "dbhi", "dbls", "dbt",
	} {
		out := convertSrc(t, "\ttst.l d0\n\t"+mnem+" d0,L\nL:\n\trts\n")
		if strings.Contains(out, "ERROR") || strings.Contains(out, "FUSE-MISS") {
			t.Errorf("%s: unexpected diagnostic\n%s", mnem, out)
		}
	}
}

// =====================================================================
// JMP modes
// =====================================================================

func TestJmp_DispAn(t *testing.T) {
	out := convertOneInstr(t, "\tjmp 8(a0)")
	mustContain(t, out, "jmp 8(r9)")
}

func TestJmp_Indexed(t *testing.T) {
	out := convertOneInstr(t, "\tjmp (8,a0,d0.l*4)")
	mustContain(t, out, "jmp (r16)")
}

func TestJmp_Abs(t *testing.T) {
	out := convertOneInstr(t, "\tjmp $F2000")
	mustContain(t, out, "bra $F2000")
}

// =====================================================================
// BTST forms
// =====================================================================

func TestBtst_ImmDn(t *testing.T) {
	out := convertOneInstr(t, "\tbtst #4,d0")
	// Bit value extracted into shadow Z: r25 = (d0 >> 4) & 1.
	mustContain(t, out, "lsr.l r25, r1, r17")
	mustContain(t, out, "and.l r25, r25, #1")
}

func TestBtst_DnDn(t *testing.T) {
	out := convertOneInstr(t, "\tbtst d2,d0")
	mustContain(t, out, "move.l r17, r3") // count from d2
	mustContain(t, out, "and.l r17, r17, #31") // mod 32 for Dn dst
}

func TestBtst_Mem(t *testing.T) {
	out := convertOneInstr(t, "\tbtst #3,(a0)")
	mustContain(t, out, "load.b r18, (r9)")
	mustContain(t, out, "and.l r17, r17, #7") // mod 8 for memory dst
}

// =====================================================================
// EAbase / index combine
// =====================================================================

func TestMovem_Indexed_BaseEA(t *testing.T) {
	out := convertOneInstr(t, "\tmovem.l d0-d1,(8,a0,d2.l*4)")
	mustContain(t, out, "store.l r1") // d0 saved
	mustContain(t, out, "store.l r2") // d1 saved
}
