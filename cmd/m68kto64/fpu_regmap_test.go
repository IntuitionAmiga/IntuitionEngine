package main

import (
	"fmt"
	"testing"
)

func TestFPRegMap_FP0ThroughFP7_EvenPaired(t *testing.T) {
	want := map[string]string{
		"fp0": "f0", "fp1": "f2", "fp2": "f4", "fp3": "f6",
		"fp4": "f8", "fp5": "f10", "fp6": "f12", "fp7": "f14",
	}
	for tok, ie := range want {
		got, ok := LookupFPRegister(tok)
		if !ok {
			t.Fatalf("LookupFPRegister(%q) returned ok=false", tok)
		}
		if got.Class != FPRegData {
			t.Errorf("%s: class=%v, want FPRegData", tok, got.Class)
		}
		if got.IE64 != ie {
			t.Errorf("%s: ie64=%q, want %q", tok, got.IE64, ie)
		}
	}
}

func TestFPRegMap_CaseInsensitive_AndStripsWhitespace(t *testing.T) {
	for _, tok := range []string{"FP0", "  fp0  ", "Fp0"} {
		got, ok := LookupFPRegister(tok)
		if !ok || got.IE64 != "f0" {
			t.Errorf("%q → %+v ok=%v, want f0", tok, got, ok)
		}
	}
}

func TestFPRegMap_ControlRegs(t *testing.T) {
	cases := []struct {
		tok   string
		class FPRegClass
	}{
		{"fpcr", FPRegFPCR}, {"fpsr", FPRegFPSR}, {"fpiar", FPRegFPIAR},
	}
	for _, c := range cases {
		got, ok := LookupFPRegister(c.tok)
		if !ok || got.Class != c.class {
			t.Errorf("%s: class=%v ok=%v, want %v", c.tok, got.Class, ok, c.class)
		}
	}
}

func TestFPRegMap_RejectsUnknown(t *testing.T) {
	for _, tok := range []string{"fp8", "fp", "f0", "d0", "a0", ""} {
		if _, ok := LookupFPRegister(tok); ok {
			t.Errorf("%q: expected ok=false", tok)
		}
	}
}

func TestFPRegMap_FPGuestRegToHost(t *testing.T) {
	for n := 0; n <= 7; n++ {
		want := fmt.Sprintf("f%d", 2*n)
		if got := FPGuestRegToHost(n); got != want {
			t.Errorf("FPGuestRegToHost(%d)=%q, want %q", n, got, want)
		}
	}
}

func TestFPSize_AllSuffixesAccepted(t *testing.T) {
	for _, s := range []string{".b", ".w", ".l", ".s", ".d", ".x", ".p"} {
		if !IsFPSize(s) {
			t.Errorf("IsFPSize(%q)=false, want true", s)
		}
	}
}

func TestFPSize_FPOnlyClassification(t *testing.T) {
	fpOnly := []string{".d", ".x", ".p"}
	intCompat := []string{".b", ".w", ".l", ".s"}
	for _, s := range fpOnly {
		if !IsFPOnlySize(s) {
			t.Errorf("IsFPOnlySize(%q)=false, want true", s)
		}
	}
	for _, s := range intCompat {
		if IsFPOnlySize(s) {
			t.Errorf("IsFPOnlySize(%q)=true, want false", s)
		}
	}
}

func TestFPSize_RejectsUnknown(t *testing.T) {
	for _, s := range []string{"", ".q", ".bb", "b", "."} {
		if IsFPSize(s) {
			t.Errorf("IsFPSize(%q)=true, want false", s)
		}
	}
}

func TestLexer_FPMnemonic_FlushLeft(t *testing.T) {
	// FPU mnemonics flush-left should be classified as instructions, not
	// labels (parallel to the integer-side col-0 disambiguation rule).
	for _, tok := range []string{"fmove", "fadd", "fcmp", "fbeq", "fdbgt", "fseq", "ftrapeq"} {
		ln := LexLine(tok + ".d fp0,fp1")
		if ln.Kind != LineInstruction {
			t.Errorf("%q: kind=%v, want LineInstruction", tok, ln.Kind)
		}
		if ln.Mnemonic != tok {
			t.Errorf("%q: mnemonic=%q", tok, ln.Mnemonic)
		}
	}
}

func TestLexer_FPSizeSuffixes(t *testing.T) {
	cases := map[string]string{
		"\tfmove.d fp0,fp1":  ".d",
		"\tfmove.x fp0,fp1":  ".x",
		"\tfmove.s fp0,fp1":  ".s",
		"\tfmove.p fp0,(a0)": ".p",
		"\tfadd.d fp0,fp1":   ".d",
	}
	for src, want := range cases {
		ln := LexLine(src)
		if ln.Size != want {
			t.Errorf("%q: size=%q, want %q", src, ln.Size, want)
		}
	}
}

func TestEmit_ShadowFPCCReserved_IsR29(t *testing.T) {
	if ShadowFPCC != "r29" {
		t.Errorf("ShadowFPCC = %q, want r29", ShadowFPCC)
	}
}

func TestEmit_FPMemorySlotsNamed(t *testing.T) {
	cases := map[string]string{
		FPSlotFPCRSave:  "__m68kto64_fpcr_save",
		FPSlotScratchQ:  "__m68kto64_fp_scratch_q",
		FPSlotConstPool: "__m68kto64_fp_const_pool",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("memory slot name mismatch: %q != %q", got, want)
		}
	}
}
