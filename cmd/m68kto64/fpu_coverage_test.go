package main

import (
	"strings"
	"testing"
)

// =====================================================================
// FBcc — exercise every cc kind so emitShadowFBccTest hits all branches.
// =====================================================================

func TestFBcc_AllCCKinds(t *testing.T) {
	all := []string{
		"f", "eq", "ogt", "oge", "olt", "ole", "ogl", "or",
		"un", "ueq", "ugt", "uge", "ult", "ule", "ne", "t",
		"sf", "seq", "gt", "ge", "lt", "le", "gl", "gle",
		"ngle", "ngl", "nle", "nlt", "nge", "ngt", "sne", "st",
	}
	for _, cc := range all {
		src := "\tfb" + cc + " target\n"
		c := NewConverter()
		c.noHeader = true
		out, errs := c.ConvertSource(src)
		if errs != 0 {
			t.Errorf("fb%s: errors:\n%s", cc, out)
		}
		if !strings.Contains(out, "target") && cc != "f" && cc != "sf" {
			t.Errorf("fb%s: missing target reference", cc)
		}
	}
}

func TestFScc_AllCCKinds(t *testing.T) {
	for _, cc := range []string{"eq", "ne", "gt", "ge", "lt", "le", "gl", "gle", "or", "un", "ueq", "t", "f"} {
		src := "\tfs" + cc + " (a0)\n"
		c := NewConverter()
		c.noHeader = true
		_, errs := c.ConvertSource(src)
		if errs != 0 {
			t.Errorf("fs%s errored", cc)
		}
	}
}

func TestFDBcc_AllCCKinds(t *testing.T) {
	for _, cc := range []string{"eq", "ne", "f", "t", "gt", "ge", "lt", "le"} {
		src := "\tfdb" + cc + " d0,target\n"
		c := NewConverter()
		c.noHeader = true
		_, errs := c.ConvertSource(src)
		if errs != 0 {
			t.Errorf("fdb%s errored", cc)
		}
	}
}

func TestFTrapcc_AllCCKinds(t *testing.T) {
	for _, cc := range fpFTrapccOrder {
		src := "\tftrap" + cc + "\n"
		c := NewConverter()
		c.noHeader = true
		_, errs := c.ConvertSource(src)
		if errs != 0 {
			t.Errorf("ftrap%s errored", cc)
		}
	}
}

// =====================================================================
// materializeFPSrc — every size path
// =====================================================================

func TestMaterializeFPSrc_S_Mem(t *testing.T) {
	out := convertSrc(t, "\tfadd.s (a0),fp0\n")
	mustContain(t, out, "fload f10, (r9)")
	mustContain(t, out, "fcvtsd f10, f10")
}

func TestMaterializeFPSrc_S_Imm(t *testing.T) {
	out := convertSrc(t, "\tfadd.s #PI_LBL,fp0\n")
	mustContain(t, out, "la r16, PI_LBL")
	mustContain(t, out, "fload f10, (r16)")
}

func TestMaterializeFPSrc_D_Imm(t *testing.T) {
	out := convertSrc(t, "\tfadd.d #LBL,fp0\n")
	mustContain(t, out, "la r16, LBL")
	mustContain(t, out, "dload f10, (r16)")
}

func TestMaterializeFPSrc_B_SignExtended(t *testing.T) {
	out := convertSrc(t, "\tfadd.b d0,fp0\n")
	mustContain(t, out, "sext.b r17,")
	mustContain(t, out, "dcvtif f10, r17")
}

func TestMaterializeFPSrc_W_SignExtended(t *testing.T) {
	out := convertSrc(t, "\tfadd.w d0,fp0\n")
	mustContain(t, out, "sext.w r17,")
}

func TestMaterializeFPSrc_L_DirectCvt(t *testing.T) {
	out := convertSrc(t, "\tfadd.l d0,fp0\n")
	mustContain(t, out, "dcvtif f10,")
}

func TestMaterializeFPSrc_Imm_Int(t *testing.T) {
	out := convertSrc(t, "\tfadd.l #5,fp0\n")
	mustContain(t, out, "move.l r17, #5")
	mustContain(t, out, "dcvtif f10, r17")
}

// =====================================================================
// FMovem — abs / disp(An) EAs
// =====================================================================

func TestFMovem_StoreList_DispAn(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x fp0/fp1,8(a0)\n")
	mustContain(t, out, "lea r16, 8(r9)")
	mustContain(t, out, "dstore f0, (r16)")
	mustContain(t, out, "dstore f2, 8(r16)")
}

func TestFMovem_LoadList_DispAn(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x 8(a0),fp0/fp1\n")
	mustContain(t, out, "dload f0, (r16)")
	mustContain(t, out, "dload f2, 8(r16)")
}

func TestFMovem_LoadList_PreDec_ReverseOrder(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x -(a0),fp0/fp1\n")
	mustContain(t, out, "sub.l r9, r9, #8")
	mustContain(t, out, "dload f2, (r9)")
}

func TestFMovem_RangeNotation(t *testing.T) {
	out := convertSrc(t, "\tfmovem.x fp0-fp3,(a0)\n")
	mustContain(t, out, "dstore f0,")
	mustContain(t, out, "dstore f2,")
	mustContain(t, out, "dstore f4,")
	mustContain(t, out, "dstore f6,")
}

// =====================================================================
// FMOVE error paths
// =====================================================================

func TestFMove_OperandCount_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmove fp0\n"); errs == 0 {
		t.Errorf("fmove with 1 operand should error")
	}
}

func TestFMove_BadDst(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmove.l fpcr,(a0)\n"); errs == 0 {
		t.Errorf("fmove ctrl→mem (non-Dn) should error in strict")
	}
}

func TestFMoveCR_BadOperands(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmovecr fp0,d0\n"); errs == 0 {
		t.Errorf("fmovecr without #imm should error")
	}
	if _, errs := c.ConvertSource("\tfmovecr d0,#0\n"); errs == 0 {
		t.Errorf("fmovecr non-FPn dst should error")
	}
}

func TestFMovem_OperandCount_Error(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmovem fp0\n"); errs == 0 {
		t.Errorf("fmovem with 1 operand should error")
	}
}

// =====================================================================
// FArith error paths
// =====================================================================

func TestFArith_DstNotFP_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfadd.x fp0,d0\n"); errs == 0 {
		t.Errorf("fadd into non-FPn should error")
	}
}

func TestFCmp_DstNotFP_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfcmp.x fp0,d0\n"); errs == 0 {
		t.Errorf("fcmp into non-FPn should error")
	}
}

func TestFScale_BadOperands(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfscale.x d0,fp0\n"); errs == 0 {
		t.Errorf("fscale with non-FPn src should error")
	}
}

// =====================================================================
// Transcendentals — every mnemonic
// =====================================================================

func TestFTranscendentals_AllSurvive(t *testing.T) {
	for _, m := range []string{
		"fsin", "fcos", "ftan", "fatan",
		"facos", "fasin",
		"fcosh", "fsinh", "ftanh", "fatanh",
		"fetox", "fetoxm1",
		"flogn", "flog10", "flog2", "flognp1",
		"ftentox", "ftwotox",
	} {
		src := "\t" + m + ".x fp0,fp1\n"
		c := NewConverter()
		c.noHeader = true
		_, errs := c.ConvertSource(src)
		if errs != 0 {
			t.Errorf("%s errored", m)
		}
	}
}

func TestFTranscendentals_OneOperand(t *testing.T) {
	out := convertSrc(t, "\tfsin.x fp0\n")
	mustContain(t, out, "fsin f0, f0")
}

// =====================================================================
// FUnary — error cases + remaining branches
// =====================================================================

func TestFUnary_BadOperandCount(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfneg fp0,fp1,fp2\n"); errs == 0 {
		t.Errorf("fneg with 3 operands should error")
	}
}

func TestFGetExp_LoweredApprox(t *testing.T) {
	out := convertSrc(t, "\tfgetexp.x fp0,fp1\n")
	mustContain(t, out, "1/ln(2) for fgetexp")
	mustContain(t, out, "dabs f2, f0")
	mustContain(t, out, "flog f2,")
	mustContain(t, out, "dint f2,")
}

func TestFGetMan_Lowered(t *testing.T) {
	out := convertSrc(t, "\tfgetman.x fp0,fp1\n")
	mustContain(t, out, "1/ln(2) for fgetman")
	mustContain(t, out, "dabs f10,")
	mustContain(t, out, "flog f10, f10")
	mustContain(t, out, "dcvtfi r17, f10")
	mustContain(t, out, "add.l r18, r17, #1023")
	mustContain(t, out, "lsl.q r18, r18, #52")
	mustContain(t, out, "__m68kto64_fp_scratch_q")
	mustContain(t, out, "ddiv f2, f0, f10")
}

// =====================================================================
// Helpers — IsFPRegisterName, evenRegIndex, parseInt
// =====================================================================

func TestIsFPRegisterName_PositiveAndNegative(t *testing.T) {
	for _, n := range []string{"fp0", "fpcr", "FPSR", "fpiar"} {
		if !IsFPRegisterName(n) {
			t.Errorf("IsFPRegisterName(%q)=false", n)
		}
	}
	for _, n := range []string{"d0", "a0", "fp", "f0"} {
		if IsFPRegisterName(n) {
			t.Errorf("IsFPRegisterName(%q)=true", n)
		}
	}
}

func TestEvenRegIndex(t *testing.T) {
	cases := map[string]int{
		"f0": 0, "f2": 1, "f14": 7,
		"f1": -1, "f15": -1, "f16": -1, "r0": -1, "fX": -1,
	}
	for in, want := range cases {
		if got := evenRegIndex(in); got != want {
			t.Errorf("evenRegIndex(%q)=%d, want %d", in, got, want)
		}
	}
}

func TestParseInt_Forms(t *testing.T) {
	cases := map[string]int{
		"5": 5, "-5": -5, "+5": 5, "$10": 16, "0x10": 16, "%101": 5,
	}
	for in, want := range cases {
		got, err := parseInt(in)
		if err != nil || got != want {
			t.Errorf("parseInt(%q)=(%d,%v), want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "abc", "$gg"} {
		if _, err := parseInt(bad); err == nil {
			t.Errorf("parseInt(%q) should error", bad)
		}
	}
}

// =====================================================================
// FP step / size helpers
// =====================================================================

func TestFPStepBytes(t *testing.T) {
	if fpStepBytes(".s") != 4 {
		t.Errorf(".s step should be 4")
	}
	if fpStepBytes(".d") != 8 {
		t.Errorf(".d step should be 8")
	}
	if fpStepBytes(".x") != 8 {
		t.Errorf(".x step should be 8")
	}
	if fpStepBytes("") != 8 {
		t.Errorf("empty step should default to 8")
	}
}

// =====================================================================
// FMovem error path: list cannot be parsed
// =====================================================================

func TestFMovem_NeitherListSide_Errors(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfmovem.x (a0),(a1)\n"); errs == 0 {
		t.Errorf("fmovem mem,mem should error")
	}
}

// =====================================================================
// FCmp / FTst error paths
// =====================================================================

func TestFCmp_Wrong_Arity(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfcmp fp0\n"); errs == 0 {
		t.Errorf("fcmp with 1 operand should error")
	}
}

func TestFTst_Wrong_Arity(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tftst fp0,fp1\n"); errs == 0 {
		t.Errorf("ftst with 2 operands should error")
	}
}

// =====================================================================
// FBcc / FDBcc / FScc / FTRAPcc — bad operand counts.
// =====================================================================

func TestFBcc_Wrong_Arity(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfbeq\n"); errs == 0 {
		t.Errorf("fbeq with 0 operands should error")
	}
}

func TestFDBcc_Wrong_Arity(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfdbeq d0\n"); errs == 0 {
		t.Errorf("fdbeq with 1 operand should error")
	}
}

func TestFDBcc_FirstNotDn(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfdbeq a0,target\n"); errs == 0 {
		t.Errorf("fdbeq first must be Dn")
	}
}

func TestFScc_Wrong_Arity(t *testing.T) {
	c := NewConverter()
	c.strict = true
	if _, errs := c.ConvertSource("\tfseq\n"); errs == 0 {
		t.Errorf("fseq with 0 operands should error")
	}
}

func TestFDBcc_TrueAlwaysSkips(t *testing.T) {
	out := convertSrc(t, "\tfdbt d0,target\n")
	mustContain(t, out, "never iterates")
}
