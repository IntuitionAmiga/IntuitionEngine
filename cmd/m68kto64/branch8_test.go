package main

import "testing"

// Final pass: target the remaining 47 uncovered blocks individually.

func TestBranch_EmitShadowScc_OnAn_DriveWriteByteConstError(t *testing.T) {
	// Scc on An — emitWriteByteConst returns error. Drives both val=0 and
	// val=0xFF error-return paths.
	c := NewConverter()
	e := &Emit{}
	c.emitShadowScc(e, Line{Mnemonic: "seq", Operands: []string{"a0"}})
}

func TestBranch_Mnem_BadSize_BranchTaken(t *testing.T) {
	// A mnemonic with an unrecognised size suffix → SizeBytes returns 0 →
	// `if size == 0 { size = 4 }` body fires inside emitInstruction.
	convertSrc(t, "\tmove.q d0,d1\n")
}

func TestBranch_Mnem_MulsLong_FallThrough(t *testing.T) {
	// muls.l falls through emitArith; size != 2 path of dispatcher.
	convertSrc(t, "\tmuls.l #5,d0\n")
}

func TestBranch_EmitMovem_ParseErrors_Indirect(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// emitMovem inner ParseOperand failures.
	c.emitMovem(e, Line{Mnemonic: "movem", Size: ".l", Operands: []string{"@@", "(a0)"}})
	c.emitMovem(e, Line{Mnemonic: "movem", Size: ".l", Operands: []string{"d0/d1", "@@"}})
}

func TestBranch_EmitMove_Imm_StoreOk(t *testing.T) {
	// Tests `move%s ScrV1, #imm` materialise + store path (srcImm != "").
	convertSrc(t, "\tmove.l #5,(a0)\n")
}

func TestBranch_FlagsCanFuseBcc_KindNotInSet(t *testing.T) {
	l := LexLine("\tbsr target")
	if canFuseBcc(l) {
		t.Errorf("bsr canFuseBcc = true")
	}
}

func TestBranch_FlagsCanFuseBcc_NonInstruction(t *testing.T) {
	l := LexLine("label:")
	if canFuseBcc(l) {
		t.Errorf("label-only canFuseBcc = true")
	}
}

func TestBranch_EmitFusedCmpBcc_NoSize(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	prod := Line{Mnemonic: "cmp", Size: "", Operands: []string{"d0", "d1"}}
	cons := Line{Mnemonic: "beq", Operands: []string{"L"}}
	c.emitFusedCmpBcc(e, prod, cons)
}

func TestBranch_EmitFusedTstBcc_NoSize(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	prod := Line{Mnemonic: "tst", Size: "", Operands: []string{"d0"}}
	cons := Line{Mnemonic: "beq", Operands: []string{"L"}}
	c.emitFusedTstBcc(e, prod, cons)
}

func TestBranch_EmitFusedCmpBcc_CmpaImmL(t *testing.T) {
	// cmpa.l + immediate src — exercises the `case srcOp.Mode == AMImmediate`
	// arm of the cmpa switch.
	convertSrc(t, "\tcmpa.l #100,a0\n\tbeq L\nL:\n\trts\n")
}

func TestBranch_EmitFusedCmpBcc_CmpaDstNotAnDn(t *testing.T) {
	// cmpa.l with dst memory — drives the `dstReg = r` fallback path.
	c := NewConverter()
	e := &Emit{}
	prod := Line{Mnemonic: "cmpa", Size: ".l", Operands: []string{"#5", "(a0)"}}
	cons := Line{Mnemonic: "beq", Operands: []string{"L"}}
	c.emitFusedCmpBcc(e, prod, cons)
}

func TestBranch_EmitFusedCmpBcc_CmpaDstLoadFail(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	// dst is ccr — loadValue fails inside cmpa branch.
	prod := Line{Mnemonic: "cmpa", Size: ".l", Operands: []string{"#5", "ccr"}}
	cons := Line{Mnemonic: "beq", Operands: []string{"L"}}
	c.emitFusedCmpBcc(e, prod, cons)
}

func TestBranch_EmitBccLine_Mi_Pl(t *testing.T) {
	// Adjacent CMP + BMI → emitBccLine "mi" arm.
	convertSrc(t, "\tcmp.l d0,d1\n\tbmi L\nL:\n\trts\n")
	convertSrc(t, "\tcmp.l d0,d1\n\tbpl L\nL:\n\trts\n")
}

func TestBranch_EmitBccLine_Default(t *testing.T) {
	// Try fused CMP + an unsupported Bcc kind directly.
	if err := emitBccLine(&Emit{}, "zzz", "r1", "r2", "L"); err == nil {
		t.Errorf("expected default-arm error")
	}
}

func TestBranch_EmitWriteByteConst_MemDst(t *testing.T) {
	// Scc on memory → emitWriteByteConst takes mem path.
	convertSrc(t, "\ttst.l d0\n\tseq (a0)\n")
	convertSrc(t, "\ttst.l d0\n\tsne (a0)\n")
}

func TestBranch_EmitMovem_LoadValueErrors(t *testing.T) {
	// emitMovemStore/Load with reglist tied to non-EA (load fails).
	c := NewConverter()
	e := &Emit{}
	c.emitMovemStore(e, []string{"r1"}, Operand{Mode: AMRegList, List: "d0"}, 4, ".l")
}

func TestBranch_Operand_BareSpecialCases(t *testing.T) {
	// Bare CCR/SR/USP — exercises switch arms in ParseOperand.
	for _, n := range []string{"ccr", "CCR", "sr", "SR", "usp", "USP"} {
		op, err := ParseOperand(n)
		if err != nil {
			t.Errorf("%q: %v", n, err)
		}
		_ = op
	}
}

func TestBranch_Operand_ParenInner_OneTokenPC(t *testing.T) {
	// (pc) → AMDispPC.
	op, _ := ParseOperand("(pc)")
	if op.Mode != AMDispPC {
		t.Errorf("(pc) mode=%v", op.Mode)
	}
}

func TestBranch_Operand_TwoTokenAnXn(t *testing.T) {
	// (a0,d1) — 2-token form where first is base.
	op, _ := ParseOperand("(a0,d1)")
	if op.Mode != AMIndexAn {
		t.Errorf("(a0,d1) mode=%v", op.Mode)
	}
}

func TestBranch_Operand_TwoTokenPCXn(t *testing.T) {
	op, _ := ParseOperand("(pc,d1)")
	if op.Mode != AMIndexPC {
		t.Errorf("(pc,d1) mode=%v", op.Mode)
	}
}

func TestBranch_Operand_DSecondNotAnPc(t *testing.T) {
	// (d,X) where X is not An or PC.
	if _, err := ParseOperand("(8,ccr)"); err == nil {
		t.Errorf("(8,ccr) should error")
	}
}

func TestBranch_Operand_ThreeTokenBaseNotAnPc(t *testing.T) {
	// (d,X,Xn) where X not An/PC.
	if _, err := ParseOperand("(8,ccr,d0)"); err == nil {
		t.Errorf("(8,ccr,d0) should error")
	}
}

func TestBranch_Operand_FindTrailingParen_FallsThroughToAbs(t *testing.T) {
	// `LABEL+(EXPR)` — has trailing ), but inner is not An/PC → fall through
	// to AMAbsL.
	op, _ := ParseOperand("LABEL+(15*8)")
	if op.Mode != AMAbsL {
		t.Errorf("LABEL+(15*8) mode=%v", op.Mode)
	}
}

func TestBranch_Operand_ImmediateEmpty(t *testing.T) {
	if _, err := ParseOperand("#"); err == nil {
		t.Errorf("# alone should error")
	}
}

func TestBranch_Operand_PreDecBadInner(t *testing.T) {
	if _, err := ParseOperand("-(@@)"); err == nil {
		t.Errorf("-(@@) should error")
	}
}

func TestBranch_Operand_PostIncBadInner(t *testing.T) {
	if _, err := ParseOperand("(@@)+"); err == nil {
		t.Errorf("(@@)+ should error")
	}
}

func TestBranch_PhaseB_BfTst_DispatchPath(t *testing.T) {
	// Mem bf source with single 32-bit window.
	convertSrc(t, "\tbftst (a0){#0:#16}\n")
}

func TestBranch_PhaseB_BfFFO_MemSrc(t *testing.T) {
	convertSrc(t, "\tbfffo (a0){#0:#16},d1\n")
}

func TestBranch_PhaseB_BfModify_MemForms(t *testing.T) {
	convertSrc(t, "\tbfclr (a0){#0:#8}\n")
	convertSrc(t, "\tbfset (a0){#0:#8}\n")
	convertSrc(t, "\tbfchg (a0){#0:#8}\n")
}

func TestBranch_PhaseB_BfMemEABaseFail(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	if err := c.loadBfMemWord(e, Operand{Mode: AMUnknown}); err == nil {
		t.Errorf("loadBfMemWord with AMUnknown should error")
	}
}

func TestBranch_PhaseB_BfModify_MemEAFail(t *testing.T) {
	// emitBfins/emitBfModify/emitBftst/emitBfffo with bf operand whose
	// memory form parses but fails on emitEABase. Use unsupported memory
	// like postinc which parseBitfieldOperand rejects → drives "supports …
	// only" error.
	c := NewConverter()
	c.noHeader = true
	c.strict = true
	for _, src := range []string{
		"\tbfins d0,(a0)+{#0:#8}\n",
		"\tbfclr (a0)+{#0:#8}\n",
		"\tbftst (a0)+{#0:#8}\n",
		"\tbfffo (a0)+{#0:#8},d0\n",
	} {
		c.ConvertSource(src)
	}
}

func TestBranch_FuseNormaliseValue_AddrRegSize2Signed(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMAddrReg, Reg: MappedReg{IE64: "r9"}}
	c.fuseNormaliseValue(e, op, 2, true, ScrV1)
}

func TestBranch_FuseNormaliseValue_AddrRegSize2Unsigned(t *testing.T) {
	c := NewConverter()
	e := &Emit{}
	op := Operand{Mode: AMAddrReg, Reg: MappedReg{IE64: "r9"}}
	c.fuseNormaliseValue(e, op, 2, false, ScrV1)
}
