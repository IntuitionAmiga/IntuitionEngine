package main

import (
	"strings"
	"testing"
)

// =====================================================================
// Bug 1: lexer col-0 instructions
// =====================================================================

func TestLexer_Col0Instruction(t *testing.T) {
	out := convertSrc(t, "move.l d0,d1\n")
	mustContain(t, out, "move.l r2, r1")
	mustNotContain(t, out, "; ERROR")
}

func TestLexer_Col0Instruction_NoSize(t *testing.T) {
	out := convertSrc(t, "rts\n")
	mustContain(t, out, "load.l r17, (r30)")
}

func TestLexer_Col0Label_StillWorks(t *testing.T) {
	// A bare identifier with no operands stays a label.
	out := convertSrc(t, "loop\n\tmove.l d0,d1\n")
	mustContain(t, out, "loop:")
	mustContain(t, out, "move.l r2, r1")
}

func TestLexer_Col0LabelWithColon(t *testing.T) {
	// "label:" classification unchanged.
	out := convertSrc(t, "loop:\n\tmove.l d0,d1\n")
	mustContain(t, out, "loop:")
}

// =====================================================================
// Bug 2: fuse must skip when Bcc has its own label
// =====================================================================

func TestFuse_SkipsWhenBccHasLabel(t *testing.T) {
	src := "\tcmp.l d0,d1\nmylabel:\tbne target\n"
	out := convertSrc(t, src)
	mustContain(t, out, "mylabel:")
	// Label must come BEFORE any synthesised fused branch operands.
	// Easiest invariant: no fused two-reg `bne r2, r1, target` should appear.
	mustNotContain(t, out, "bne r2, r1, target")
}

// =====================================================================
// Bug 3: RMW with (An)+ must increment a0 exactly ONCE
// =====================================================================

func TestRMW_PostInc_SingleIncrement(t *testing.T) {
	out := convertOneInstr(t, "\tadd.l d0,(a0)+")
	cnt := strings.Count(out, "add.l r9, r9, #4")
	if cnt != 1 {
		t.Errorf("expected exactly 1 a0 increment for add.l d0,(a0)+, got %d\n%s", cnt, out)
	}
}

func TestRMW_PreDec_SingleDecrement(t *testing.T) {
	out := convertOneInstr(t, "\tadd.l d0,-(a0)")
	cnt := strings.Count(out, "sub.l r9, r9, #4")
	if cnt != 1 {
		t.Errorf("expected exactly 1 a0 decrement for add.l d0,-(a0), got %d\n%s", cnt, out)
	}
}

// =====================================================================
// Bug 4: MULU.W / MULS.W produce 32-bit result; DIVU.W / DIVS.W pack
// (rem<<16)|quo into full 32-bit Dn.
// =====================================================================

func TestMuluW_FullDnWrite(t *testing.T) {
	out := convertOneInstr(t, "\tmulu.w d0,d1")
	// 32-bit product into low 32 of Dn (full Dn write, not partial).
	mustContain(t, out, "mulu.l")
	mustNotContain(t, out, "$FFFFFFFFFFFF0000") // upper-preserve mask is wrong here
}

func TestMulsW_FullDnWrite_SignExtend(t *testing.T) {
	out := convertOneInstr(t, "\tmuls.w d0,d1")
	mustContain(t, out, "muls.l")
	mustContain(t, out, "sext.w") // operands sign-extended before signed multiply
}

func TestDivuW_PacksRemQuoIntoDn(t *testing.T) {
	out := convertOneInstr(t, "\tdivu.w d0,d1")
	mustContain(t, out, "divu.l")
	mustContain(t, out, "mod.l")
	mustContain(t, out, "lsl.l") // remainder shifted to high word
	mustContain(t, out, "or.l")  // pack
}

func TestDivsW_PacksRemQuoIntoDn(t *testing.T) {
	out := convertOneInstr(t, "\tdivs.w d0,d1")
	mustContain(t, out, "divs.l")
	mustContain(t, out, "mods.l")
	mustContain(t, out, "lsl.l")
	mustContain(t, out, "or.l")
}
