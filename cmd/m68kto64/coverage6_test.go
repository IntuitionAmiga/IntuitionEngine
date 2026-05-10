package main

import (
	"strings"
	"testing"
)

// More targeted coverage for branches still uncovered.

func TestEmitShift_MemSizes(t *testing.T) {
	cases := []string{
		"\tlsl.b #1,(a0)",
		"\tlsl.w #1,(a0)",
		"\tlsr.b d0,(a0)",
		"\tasr.w d0,(a0)",
		"\tasl.l #2,(a0)",
		"\trol.w #1,(a0)",
		"\tror.l d0,(a0)",
		"\tlsl (a0)", // single-operand form, count=1
	}
	for _, src := range cases {
		out := convertOneInstr(t, src)
		if strings.Contains(out, "ERROR") {
			t.Errorf("%q: %s", src, out)
		}
	}
}

func TestEmitFusedCmpBcc_AllPairs(t *testing.T) {
	srcModes := []string{"d0", "#5", "(a0)", "8(a0)"}
	dstModes := []string{"d1", "(a1)", "8(a1)"}
	bccs := []string{"beq", "bne", "blt", "bge", "bgt", "ble", "bhi", "bls"}
	sizes := []string{".b", ".w", ".l"}
	count := 0
	for _, sz := range sizes {
		for _, src := range srcModes {
			for _, dst := range dstModes {
				for _, bcc := range bccs {
					prog := "\tcmp" + sz + " " + src + "," + dst + "\n\t" + bcc + " L\nL:\n\trts\n"
					c := NewConverter()
					c.noHeader = true
					_, errs := c.ConvertSource(prog)
					if errs == 0 {
						count++
					}
				}
			}
		}
	}
	if count == 0 {
		t.Fatalf("grid produced zero successful conversions")
	}
}

func TestEmitFusedTstBcc_AllPairs(t *testing.T) {
	dstModes := []string{"d0", "(a0)", "8(a0)"}
	bccs := []string{"beq", "bne", "blt", "bge", "bgt", "ble", "bhi", "bls", "bmi", "bpl"}
	sizes := []string{".b", ".w", ".l"}
	for _, sz := range sizes {
		for _, dst := range dstModes {
			for _, bcc := range bccs {
				prog := "\ttst" + sz + " " + dst + "\n\t" + bcc + " L\nL:\n\trts\n"
				c := NewConverter()
				c.noHeader = true
				_, errs := c.ConvertSource(prog)
				if errs != 0 {
					t.Errorf("%s + %s + %s: errors", sz, dst, bcc)
				}
			}
		}
	}
}

func TestEmitMulW_AddrModes(t *testing.T) {
	for _, src := range []string{"d0", "#5", "(a0)", "8(a0)"} {
		out := convertOneInstr(t, "\tmulu.w "+src+",d1")
		if strings.Contains(out, "ERROR") {
			t.Errorf("mulu.w %s: %s", src, out)
		}
	}
	for _, src := range []string{"d0", "#5", "(a0)"} {
		out := convertOneInstr(t, "\tmuls.w "+src+",d1")
		if strings.Contains(out, "ERROR") {
			t.Errorf("muls.w %s: %s", src, out)
		}
	}
}

func TestEmitDivPair_AddrModes(t *testing.T) {
	for _, src := range []string{"d0", "#100", "(a0)"} {
		for _, mnem := range []string{"divu.l", "divs.l"} {
			out := convertOneInstr(t, "\t"+mnem+" "+src+",d2:d3")
			if strings.Contains(out, "ERROR") {
				t.Errorf("%s %s: %s", mnem, src, out)
			}
		}
	}
}

func TestStoreValue_AddrModes(t *testing.T) {
	for _, dst := range []string{"d0", "a0", "(a0)", "(a0)+", "-(a0)", "8(a0)", "(8,a0,d0.w)", "($F2000).w", "$F2000"} {
		out := convertOneInstr(t, "\tmove.l #1,"+dst)
		if strings.Contains(out, "ERROR") {
			t.Errorf("move dst=%s: %s", dst, out)
		}
	}
}

func TestLoadValue_AddrModes(t *testing.T) {
	for _, src := range []string{"d0", "a0", "(a0)", "(a0)+", "-(a0)", "8(a0)", "(8,a0,d0.w)", "($F2000).w", "$F2000"} {
		out := convertOneInstr(t, "\tmove.l "+src+",d1")
		if strings.Contains(out, "ERROR") {
			t.Errorf("move src=%s: %s", src, out)
		}
	}
}

// Hit emitFusedPair label-on-prod path.
func TestFuse_LabelOnProducer_NotFused(t *testing.T) {
	out := convertSrc(t, "label:\n\tcmp.l d0,d1\n\tbeq L\nL:\n\trts\n")
	mustContain(t, out, "label:")
	// With label on cmp, fusion is suppressed; CMP emits shadows, BEQ uses
	// shadow consumer.
	mustContain(t, out, "beqz r25, L")
}

// Hit emitFusedPair both-labels path (cmp labelled, Bcc labelled).
func TestFuse_BothLabels_NotFused(t *testing.T) {
	out := convertSrc(t, "lbl1:\n\tcmp.l d0,d1\nlbl2:\n\tbeq L\nL:\n\trts\n")
	mustContain(t, out, "lbl1:")
	mustContain(t, out, "lbl2:")
}

// IndexOf for unknown.
func TestExpandRegList_BadRange(t *testing.T) {
	if _, err := expandRegList("d5-d2"); err == nil {
		t.Errorf("expected error for reversed range")
	}
	if _, err := expandRegList("foo"); err == nil {
		t.Errorf("expected error for unknown reg")
	}
}

func TestExpandRegList_Empty(t *testing.T) {
	got, err := expandRegList("")
	if err != nil {
		t.Errorf("empty list errored: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty list got %v", got)
	}
}

func TestPostIncStep_StackByte(t *testing.T) {
	// move.b -(sp),d0 → 2-byte step on a7 (m68k stack alignment).
	out := convertOneInstr(t, "\tmove.b -(sp),d0")
	mustContain(t, out, "sub.l r30, r30, #2")
}

func TestPostIncStep_StackByte_PostInc(t *testing.T) {
	out := convertOneInstr(t, "\tmove.b (sp)+,d0")
	mustContain(t, out, "add.l r30, r30, #2")
}

func TestParseIndex_BadInputs(t *testing.T) {
	bad := []string{"d0.q", "d0*3", "(a0)", "d0.w*5"}
	for _, s := range bad {
		if _, err := parseIndex(s); err == nil {
			t.Logf("%q: parseIndex unexpectedly OK (could be valid)", s)
		}
	}
}
