package main

import (
	"strings"
	"testing"
)

// FP7 (f14) auto-spill — Phase F1.
//
// FTANH's hyperbolic helper writes f14 (= guest FP7) as a second scratch
// target. Without spill, live FP7 silently corrupts. Wrapper now spills
// via __m68kto64_fp7_save alongside FP5/FP6.

const (
	spillStoreFP7 = "dstore f14, (r16)  ; spill FP7"
	spillLoadFP7  = "dload f14, (r16)  ; restore FP7"
)

func TestFP7Spill_FTANH_PreservesFP7(t *testing.T) {
	out := convertOneInstr(t, "\tftanh.x fp1")
	mustContain(t, out, "la r16, __m68kto64_fp7_save")
	mustContain(t, out, spillStoreFP7)
	mustContain(t, out, spillLoadFP7)
}

// FTANH dst=FP7: prologue spills (in case src reads FP7), epilogue must NOT
// restore (would overwrite result).
func TestFP7Spill_FTANH_DstFP7_NoRestore(t *testing.T) {
	out := convertOneInstr(t, "\tftanh.x fp1,fp7")
	mustContain(t, out, spillStoreFP7)
	mustNotContain(t, out, spillLoadFP7)
}

// FSINH uses only f10/f12, not f14 — no FP7 slot allocated.
func TestFP7Spill_NoOpForNonFTANH(t *testing.T) {
	out := convertOneInstr(t, "\tfsinh.x fp1")
	if strings.Contains(out, "__m68kto64_fp7_save:") {
		t.Errorf("fp7_save BSS label allocated for op that does not need it:\n%s", out)
	}
	if strings.Contains(out, spillStoreFP7) {
		t.Errorf("FP7 spill emitted for fsinh (not ftanh):\n%s", out)
	}
}

// When FTANH runs, BSS slot IS allocated in the footer.
func TestFP7Spill_FTANH_BSSAllocated(t *testing.T) {
	out := convertOneInstr(t, "\tftanh.x fp1")
	mustContain(t, out, "__m68kto64_fp7_save:")
}
