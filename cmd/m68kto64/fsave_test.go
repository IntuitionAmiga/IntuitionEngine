package main

import (
	"strings"
	"testing"
)

// FSAVE / FRESTORE — opaque transpiler-private state frame.
// Frame layout: 64B for FP0..FP7 + 4B FPCR + 4B FPSR + 4B FPIAR-slot + 4B magic.

func TestFSAVE_Indirect_WritesAllRegs(t *testing.T) {
	out := convertOneInstr(t, "\tfsave (a0)")
	mustContain(t, out, "move.l r16, r9") // EA base into ScrEA
	// All 8 FP regs stored at offsets 0..56.
	for _, off := range []string{"0(r16)", "8(r16)", "16(r16)", "24(r16)", "32(r16)", "40(r16)", "48(r16)", "56(r16)"} {
		if !strings.Contains(out, "dstore.d") || !strings.Contains(out, off) {
			t.Errorf("FSAVE missing dstore.d at %s:\n%s", off, out)
		}
	}
	mustContain(t, out, "fmovcr ")
	mustContain(t, out, "store.l ")
	mustContain(t, out, "64(r16)")
	mustContain(t, out, "68(r16)")
	mustContain(t, out, "72(r16)")
	mustContain(t, out, "$1E64FE7E") // magic
	mustContain(t, out, "76(r16)")
}

func TestFSAVE_PreDec_DecrementsFirst(t *testing.T) {
	out := convertOneInstr(t, "\tfsave -(a7)")
	mustContain(t, out, "sub.l r30, r30, #80") // decrement An by full frame size
	mustContain(t, out, "$1E64FE7E")
}

func TestFSAVE_Absolute(t *testing.T) {
	out := convertOneInstr(t, "\tfsave $1000")
	mustContain(t, out, "la r16,")
	mustContain(t, out, "$1E64FE7E")
}

func TestFSAVE_FPSRCompose(t *testing.T) {
	// FPSR composed: hardware sticky | (ShadowFPCC << 24).
	out := convertOneInstr(t, "\tfsave (a0)")
	mustContain(t, out, "fmovsr ")
	mustContain(t, out, "lsl.l r18, r29, #24") // ShadowFPCC << 24
	mustContain(t, out, "or.l ")
}

func TestFRESTORE_Indirect_VerifiesMagic(t *testing.T) {
	out := convertOneInstr(t, "\tfrestore (a0)")
	// Load magic from +76 and compare.
	mustContain(t, out, "load.l r17, 76(r16)")
	mustContain(t, out, "move.l r18, #$1E64FE7E")
	mustContain(t, out, "bne r17, r18, ")
	// Match path: dload 8 fp regs.
	mustContain(t, out, "dload.d ")
	// FPCR + FPSR restore.
	mustContain(t, out, "load.l r17, 64(r16)")
	mustContain(t, out, "fmovcc r17")
	mustContain(t, out, "load.l r17, 68(r16)")
	mustContain(t, out, "fmovsc r17")
	// Null-path label present.
	mustContain(t, out, "frestore_null")
}

func TestFRESTORE_PostInc_IncrementsAfter(t *testing.T) {
	out := convertOneInstr(t, "\tfrestore (a7)+")
	mustContain(t, out, "add.l r30, r30, #80")
}

func TestFRESTORE_NullPath_ClearsFPU(t *testing.T) {
	// Null/foreign frame path clears all FP regs to 0.0 and resets controls.
	out := convertOneInstr(t, "\tfrestore (a0)")
	mustContain(t, out, "dcvtif ")             // FP regs cleared via 0 → fp
	mustContain(t, out, "move.l r29, #0")      // ShadowFPCC := 0
}

func TestFSAVE_FRESTORE_Pair_RoundTrip(t *testing.T) {
	// Symmetric pair must use the same magic so the FRESTORE match path runs.
	src := "\tfsave -(a7)\n\tfrestore (a7)+\n"
	out := convertSrc(t, src)
	// Same magic literal appears in both directions.
	if strings.Count(out, "$1E64FE7E") < 2 {
		t.Errorf("FSAVE/FRESTORE pair must reference the same magic in both ends:\n%s", out)
	}
}

func TestFSAVE_StrictMode_NoLongerErrors(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	out, errs := c.ConvertSource("\tfsave (a0)\n")
	if errs != 0 {
		t.Errorf("FSAVE under -strict should now succeed (full lowering):\n%s", out)
	}
}

func TestFRESTORE_StrictMode_NoLongerErrors(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	out, errs := c.ConvertSource("\tfrestore (a0)\n")
	if errs != 0 {
		t.Errorf("FRESTORE under -strict should now succeed (full lowering):\n%s", out)
	}
}
