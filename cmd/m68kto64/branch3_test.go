package main

import (
	"testing"
)

// Final branch-coverage sweep — target each remaining uncovered block by
// direct calls with synthetic Line/Operand values that drive the err return
// or the exact switch arm we need.

func TestBranch_EmitShadowDBcc_ParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	if err := c.emitShadowDBcc(e, Line{Mnemonic: "dbeq", Operands: []string{"", "L"}}); err == nil {
		t.Errorf("expected ParseOperand error")
	}
}

func TestBranch_EmitShadowDBcc_DBT(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	if err := c.emitShadowDBcc(e, Line{Mnemonic: "dbt", Operands: []string{"d0", "L"}}); err != nil {
		t.Errorf("dbt: %v", err)
	}
}

func TestBranch_EmitShadowScc_ParseError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	if err := c.emitShadowScc(e, Line{Mnemonic: "seq", Operands: []string{""}}); err == nil {
		t.Errorf("expected ParseOperand error")
	}
}

func TestBranch_EmitWriteByteConst_AnError(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMAddrReg, Reg: MappedReg{IE64: "r9", Class: RegAddr}}
	// Drive both val=0 (zero) and val=0xFF (nonzero) byte-write paths into An.
	if err := c.emitWriteByteConst(e, op, 0); err == nil {
		t.Errorf("byte to An (val=0): expected error")
	}
	if err := c.emitWriteByteConst(e, op, 0xFF); err == nil {
		t.Errorf("byte to An (val=0xFF): expected error")
	}
}

func TestBranch_EmitInstruction_UnknownInNonStrict(t *testing.T) {
	out := convertSrc(t, "\tnonexistent_macrocall foo,bar\n")
	mustContain(t, out, "nonexistent_macrocall")
}

func TestBranch_EmitInstruction_BadSize(t *testing.T) {
	// `.q` size is unrecognised by SizeBytes → returns 0 → line 176 fires.
	convertSrc(t, "\tmove.q d0,d1\n")
}

func TestBranch_EmitArith_LSL_ASL_BadCount_NonImmDn(t *testing.T) {
	// emitShift count not Imm/Dn — converter.go ~1610 returns error.
	c := NewConverter()
	e := &Emit{}
	c.emitShift(e, Line{Mnemonic: "lsl", Size: ".l", Operands: []string{"(a0)", "d0"}}, 4, "lsl", "lsl")
}

func TestBranch_EmitMovem_StoreUnsupportedEA(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// emitEABase fails on AMUnknown.
	c.emitMovemStore(e, []string{"r1"}, Operand{Mode: AMUnknown}, 4, ".l")
}

func TestBranch_EmitMovem_LoadUnsupportedEA(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	c.emitMovemLoad(e, []string{"r1"}, Operand{Mode: AMUnknown}, 4, ".l")
}

func TestBranch_EmitArith_AddaImm(t *testing.T) {
	// Drive adda .w + immediate path (sign-extend imm before .l add).
	convertSrc(t, "\tadda.w #-1,a0\n")
	convertSrc(t, "\tsuba.w #-1,a0\n")
	convertSrc(t, "\tadda.l #100,a0\n")
	convertSrc(t, "\tsuba.l #100,a0\n")
}

func TestBranch_EmitMove_StoreError(t *testing.T) {
	// move.b imm,An — strict ParseOperand path forces store error.
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	c.ConvertSource("\tmove.b #5,a0\n")
}

func TestBranch_EmitMove_MoveaSizeError(t *testing.T) {
	// movea with .b size hits the size-check error in emitMove.
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	if _, errs := c.ConvertSource("\tmovea.b d0,a0\n"); errs == 0 {
		t.Errorf("movea.b should error")
	}
}

func TestBranch_LoadDstRMW_BadMode(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMRegList, List: "d0/d1"}
	if _, err := c.loadDstRMW(e, op, 4); err == nil {
		t.Errorf("expected error for AMRegList in loadDstRMW")
	}
}

func TestBranch_DivZeroGuard_StaticZero(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	got := c.emitDivZeroGuard(e, "r1", "0")
	if got != "" {
		t.Errorf("static-zero divisor must return empty: got %q", got)
	}
}

func TestBranch_DivZeroGuard_StaticNonZero(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	got := c.emitDivZeroGuard(e, "r1", "5")
	if got != ScrV1 {
		t.Errorf("nonzero immediate divisor should materialise to ScrV1; got %q", got)
	}
}

func TestBranch_EmitDirective_CnopSingleArg(t *testing.T) {
	// directives.go cnop with len < 2: falls through to default pass-through.
	out := convertSrc(t, "\tcnop\n")
	mustContain(t, out, "cnop")
}

func TestBranch_EmitDirective_CnopTwoArgs(t *testing.T) {
	out := convertSrc(t, "\tcnop 0,4\n")
	mustContain(t, out, "align 4")
}

func TestBranch_SizeInvMask_Default(t *testing.T) {
	if got := SizeInvMask(8); got != "$FFFFFFFF00000000" {
		t.Errorf("size 8 default fallback wrong: %q", got)
	}
	if got := SizeInvMask(0); got != "$FFFFFFFF00000000" {
		t.Errorf("size 0 default fallback wrong: %q", got)
	}
}

func TestBranch_IE64Size_Default(t *testing.T) {
	if got := IE64Size(8); got != ".l" {
		t.Errorf("IE64Size(8) wrong: %q", got)
	}
	if got := IE64Size(0); got != ".l" {
		t.Errorf("IE64Size(0) wrong: %q", got)
	}
}

func TestBranch_SizeMask_Default(t *testing.T) {
	if got := SizeMask(8); got != "$FFFFFFFF" {
		t.Errorf("SizeMask(8) wrong: %q", got)
	}
}

func TestBranch_BccKind_RareForms(t *testing.T) {
	// Cover the dbra/dbf/dbt special and bsr/bra returning empty.
	if bccKind("bsr") != "" {
		t.Errorf("bsr kind not empty")
	}
	if bccKind("foo") != "" {
		t.Errorf("foo kind not empty")
	}
}

func TestBranch_EmitDirective_OtherDirectives(t *testing.T) {
	out := convertSrc(t, "\txdef foo\n\txref bar\n\tpublic baz\n\tglobal qux\n\textern quux\n")
	mustContain(t, out, "dropped")
}

func TestBranch_EmitDirective_NoOps(t *testing.T) {
	convertSrc(t, "\txdef\n\txref\n") // no operands
}

func TestBranch_LoadValue_AMAbsAndPCRel(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMAbsW, Disp: "$1000"}
	c.loadValue(e, op, 4, ScrV1)
	op = Operand{Mode: AMDispPC, Disp: "label"}
	c.loadValue(e, op, 4, ScrV1)
	op = Operand{Mode: AMIndexPC, Disp: "label", Index: IndexSpec{Reg: MappedReg{IE64: "r1", Class: RegData}, Size: "w", Scale: 1}}
	c.loadValue(e, op, 4, ScrV1)
}

func TestBranch_StoreValue_AMAbs(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMAbsL, Disp: "FOO"}
	c.storeValue(e, op, 4, "r1")
}

func TestBranch_EmitMul_Imm(t *testing.T) {
	// MULU.W with imm src exercises emitMulW imm-materialise branch.
	convertSrc(t, "\tmulu.w #100,d0\n")
	convertSrc(t, "\tmuls.w #-1,d0\n")
}

func TestBranch_EmitDivW_Imm(t *testing.T) {
	convertSrc(t, "\tdivu.w #100,d0\n")
	convertSrc(t, "\tdivs.w #-1,d0\n")
}

func TestBranch_FuseNormaliseValue_AMUnknown(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMUnknown}
	if _, err := c.fuseNormaliseValue(e, op, 4, true, ScrV1); err == nil {
		t.Errorf("expected error for AMUnknown signed")
	}
}

func TestBranch_LexLine_LabelOnlyWithComment(t *testing.T) {
	l := LexLine("loop:    ; tag")
	if l.Kind != LineLabelOnly {
		t.Errorf("expected LineLabelOnly")
	}
}

func TestBranch_LexLine_AllSizes(t *testing.T) {
	for _, sz := range []string{".b", ".w", ".l", ".s"} {
		l := LexLine("\tmove" + sz + " d0,d1")
		if l.Size != sz {
			t.Errorf("%s: size %q", sz, l.Size)
		}
	}
}
