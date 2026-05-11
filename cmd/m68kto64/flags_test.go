package main

import (
	"strings"
	"testing"
)

// convertSrc converts a multi-line m68k snippet (with header suppressed).
func convertSrc(t *testing.T, src string) string {
	t.Helper()
	c := NewConverter()
	c.noHeader = true
	out, errs := c.ConvertSource(src)
	if errs != 0 {
		t.Fatalf("conversion errors:\n%s", out)
	}
	return out
}

// preprocSrc routes a snippet through the full Preprocess + ConvertLines
// pipeline so tests can assert post-expansion shape (macros expanded,
// conditionals lowered, includes inlined, etc).
func preprocSrc(t *testing.T, src string, opts PreprocOpts) string {
	t.Helper()
	var stderr strings.Builder
	r, errs := Preprocess([]byte(src), "test.s", opts, &stderr)
	if errs != 0 {
		t.Fatalf("preproc errors=%d:\n%s", errs, stderr.String())
	}
	c := NewConverter()
	c.noHeader = true
	c.symtab = r.symtab
	c.werrorUnknownMnem = opts.WerrorUnknownMnem
	out, cerrs := c.ConvertLines(r.lines)
	if cerrs != 0 {
		t.Fatalf("convert errors:\n%s", out)
	}
	return out
}

// =====================================================================
// CMP + Bcc fuse
// =====================================================================

func TestFuse_CmpBeq_Long(t *testing.T) {
	out := convertSrc(t, "\tcmp.l d0,d1\n\tbeq target\n")
	mustContain(t, out, "beq r2, r1, target")
	mustNotContain(t, out, "FUSE-MISS")
}

func TestFuse_CmpBne_Long(t *testing.T) {
	out := convertSrc(t, "\tcmp.l d0,d1\n\tbne loop\n")
	mustContain(t, out, "bne r2, r1, loop")
}

func TestFuse_CmpBlt_Word_SignExtend(t *testing.T) {
	out := convertSrc(t, "\tcmp.w d0,d1\n\tblt loop\n")
	// Both operands sign-extended at .w.
	mustContain(t, out, "sext.w r17, r1")
	mustContain(t, out, "sext.w r18, r2")
	mustContain(t, out, "blt r18, r17, loop")
}

func TestFuse_CmpBhi_Unsigned(t *testing.T) {
	out := convertSrc(t, "\tcmp.w d0,d1\n\tbhi loop\n")
	// Unsigned: low-byte/word mask, no sext.
	mustContain(t, out, "and.l r17, r1, #$FFFF")
	mustContain(t, out, "and.l r18, r2, #$FFFF")
	mustContain(t, out, "bhi r18, r17, loop")
}

func TestFuse_CmpiImm(t *testing.T) {
	out := convertSrc(t, "\tcmpi.l #5,d0\n\tbeq target\n")
	mustContain(t, out, "move.l r17, #5")
	mustContain(t, out, "beq r1, r17, target")
}

// =====================================================================
// TST + Bcc fuse
// =====================================================================

func TestFuse_TstBeq(t *testing.T) {
	out := convertSrc(t, "\ttst.l d0\n\tbeq target\n")
	mustContain(t, out, "beqz r1, target")
}

func TestFuse_TstBmi(t *testing.T) {
	// BMI: signed N — sign-extend then bltz.
	out := convertSrc(t, "\ttst.w d0\n\tbmi target\n")
	mustContain(t, out, "sext.w r17, r1")
	mustContain(t, out, "bltz r17, target")
}

func TestFuse_TstBpl_Long(t *testing.T) {
	out := convertSrc(t, "\ttst.l d0\n\tbpl target\n")
	mustContain(t, out, "bgez")
}

// =====================================================================
// Unfused fall-through (non-strict)
// =====================================================================

func TestUnfusedCmp_NonStrict_UsesShadows(t *testing.T) {
	out := convertSrc(t, "\tcmp.l d0,d1\n\tnop\n\tbeq target\n")
	// CMP not adjacent to Bcc: emit shadow CCR; BEQ reads shadow Z.
	mustContain(t, out, "beqz r25, target")
	mustNotContain(t, out, "FUSE-MISS")
}

// =====================================================================
// BRA / BSR / JSR / RTS
// =====================================================================

func TestBra(t *testing.T) {
	out := convertSrc(t, "\tbra target\n")
	mustContain(t, out, "bra target")
}

func TestBsr_PushReturn(t *testing.T) {
	out := convertSrc(t, "\tbsr target\n")
	mustContain(t, out, "sub.l r30, r30, #4")
	mustContain(t, out, "store.l r17, (r30)")
	mustContain(t, out, "bra target")
}

func TestJsr_Indirect(t *testing.T) {
	out := convertSrc(t, "\tjsr (a0)\n")
	mustContain(t, out, "sub.l r30, r30, #4")
	mustContain(t, out, "jmp (r9)")
}

func TestJsr_Label_UsesBra(t *testing.T) {
	out := convertSrc(t, "\tjsr target\n")
	mustContain(t, out, "bra target")
}

func TestRts(t *testing.T) {
	out := convertSrc(t, "\trts\n")
	mustContain(t, out, "load.l r17, (r30)")
	mustContain(t, out, "add.l r30, r30, #4")
	mustContain(t, out, "jmp (r17)")
}

func TestJmp_Indirect(t *testing.T) {
	out := convertSrc(t, "\tjmp (a0)\n")
	// Single line, no return push.
	mustContain(t, out, "jmp (r9)")
	mustNotContain(t, out, "store.l")
}

// =====================================================================
// LINK / UNLK
// =====================================================================

func TestLink(t *testing.T) {
	out := convertSrc(t, "\tlink a6,#-12\n")
	mustContain(t, out, "sub.l r30, r30, #4")
	mustContain(t, out, "store.l r15, (r30)")
	mustContain(t, out, "move.l r15, r30")
	mustContain(t, out, "add.l r30, r30, #-12")
}

func TestUnlk(t *testing.T) {
	out := convertSrc(t, "\tunlk a6\n")
	mustContain(t, out, "move.l r30, r15")
	mustContain(t, out, "load.l r15, (r30)")
	mustContain(t, out, "add.l r30, r30, #4")
}

// =====================================================================
// MOVEM
// =====================================================================

func TestMovem_StorePreDec_Reversed(t *testing.T) {
	// movem.l d0-d2,-(sp): predec stores in reverse canonical order, so
	// d2 first, then d1, then d0 (each preceded by sub r30,r30,#4).
	out := convertSrc(t, "\tmovem.l d0-d2,-(sp)\n")
	// d2 stored first.
	d2Idx := strings.Index(out, "store.l r3,")
	d0Idx := strings.Index(out, "store.l r1,")
	if d2Idx < 0 || d0Idx < 0 {
		t.Fatalf("missing d0 or d2 store:\n%s", out)
	}
	if d2Idx >= d0Idx {
		t.Errorf("expected d2 store before d0 store (predec reversal)\n%s", out)
	}
	mustContain(t, out, "sub.l r30, r30, #4")
}

func TestMovem_LoadPostInc(t *testing.T) {
	out := convertSrc(t, "\tmovem.l (sp)+,d0-d1\n")
	mustContain(t, out, "load.l r1, (r30)")
	mustContain(t, out, "add.l r30, r30, #4")
	mustContain(t, out, "load.l r2, (r30)")
}

// =====================================================================
// DBRA
// =====================================================================

func TestDbra_LowWordDecrement(t *testing.T) {
	out := convertSrc(t, "\tdbra d0,loop\n")
	mustContain(t, out, "and.l r17, r1, #$FFFF")
	mustContain(t, out, "sub.l r17, r17, #1")
	mustContain(t, out, "and.l r17, r17, #$FFFF")
	mustContain(t, out, "move.l r18, #$FFFF")
	mustContain(t, out, "bne r17, r18, loop")
}

// =====================================================================
// expandRegList unit tests
// =====================================================================

func TestExpandRegList(t *testing.T) {
	cases := map[string][]string{
		"d0":          {"r1"},
		"d0/d2":       {"r1", "r3"},
		"d0-d2":       {"r1", "r2", "r3"},
		"d0-d1/a0":    {"r1", "r2", "r9"},
		"d0-d7/a0-a6": {"r1", "r2", "r3", "r4", "r5", "r6", "r7", "r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15"},
	}
	for in, want := range cases {
		got, err := expandRegList(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("%q: len=%d want %d  (%v)", in, len(got), len(want), got)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%q [%d]: %q want %q", in, i, got[i], want[i])
			}
		}
	}
}
