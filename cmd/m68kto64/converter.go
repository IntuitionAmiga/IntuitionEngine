package main

import (
	"fmt"
	"math/bits"
	"strings"
)

// Converter turns lexed m68k Lines into IE64 source via the configured Emit.
//
// Phase 2 scope (this file): straight-line ALU + memory lowering.
//
//	MOVE, ADD, ADDA, ADDI, ADDQ, SUB, SUBA, SUBI, SUBQ,
//	AND, ANDI, OR, ORI, EOR, EORI, NOT, NEG, CLR,
//	LSL, LSR, ASL, ASR, ROL, ROR, EXT, EXTB,
//	plus LEA, MOVEA.
//
// Control flow (Bcc, JSR, RTS, MOVEM, LINK, UNLK, DBcc, Scc) and flag fusion
// land in Phase 3.
type Converter struct {
	defaultSize       string // ".l" or ".q" (default ".l")
	noHeader          bool
	strict            bool // unsupported op -> error rather than ; TODO
	noFlagsFuse       bool // disable Phase-3 fuse (forwards-compat flag)
	werrorUnknownMnem bool // unknown mnemonic emits ; ERROR: rather than passing through
	errors            int

	// Phase-7 FPU state. fpUsed is set when any FPU op is lowered so the
	// per-output-file FPU footer (memory slots) gets emitted. fpConsts
	// accumulates `dc.d` literals required by FMOVECR / FMOVE.D #imm
	// fallback; usesFPConstPool mirrors fpConsts != nil for fast checks.
	fpUsed          bool
	usesFPConstPool bool
	fpConsts        map[string]fpConstEntry
	// needsFP56Save is set when at least one synthesis op that touches FP5/FP6
	// scratch (FTST/FSCALE/FNEG/FGETEXP/FGETMAN/SINH/COSH/ATANH/...) is
	// lowered. Gates BSS allocation of __m68kto64_fp5_save / fp6_save and
	// (Phase 2) decides handler-frame size for the RTE-walkback wrapper.
	needsFP56Save bool
	// Phase F1 — FTANH uses f14 (= guest FP7) as a second scratch via the
	// hyperbolic helper. Gates BSS allocation of __m68kto64_fp7_save and
	// (Phase F1.1) the handler-frame growth from 32B to 40B.
	needsFP7Save bool

	// Phase-2 RTE-walkback handler wrap. When fpIrqWrap is set, the
	// pre-emit scan partitions the line stream into (label, …, rte)
	// regions and marks each handler label so emit-time can wrap entry
	// with FP-slot save and exit (immediately before RTE) with restore.
	// Default off — most corpus is single-threaded.
	fpIrqWrap          bool
	irqHandlerLabels   map[string]bool // label name → emit save stub at label
	irqHandlerRTELine  map[int]string  // RTE line index → handler label name (so emitRte knows frame size)
	irqOrphanRTELine   map[int]bool    // RTE line index with no preceding label in scope
	irqWrapInitialized bool
	// fpccLiveAt is populated by the Phase-7.4 liveness pass to gate
	// ShadowFPCC update emission. Indexed by line position in the lexed
	// input.
	fpccLiveAt map[int]bool
	// curLineIdx is the current pre-lexed line index for the in-flight
	// emit. Liveness lookups consume it.
	curLineIdx int

	// symtab carries preprocessor-time symbol bindings, used by emitDirective
	// to lower ifd/ifnd via symbol lookup instead of the legacy literal
	// IS_IE check. Defaults to a symtab seeded with IS_IE=1 (preserves the
	// pre-Phase-C output shape for direct ConvertSource callers). ConvertFile
	// overwrites this with the preprocessor's populated symtab.
	symtab *Symtab
}

// fpccLive reports whether the current line is a ShadowFPCC producer with
// a live downstream consumer. If liveness has not been computed (e.g. unit
// tests that call emit directly), conservatively returns true.
func (c *Converter) fpccLive() bool {
	if c.fpccLiveAt == nil {
		return true
	}
	return c.fpccLiveAt[c.curLineIdx]
}

// NewConverter constructs a Converter with default settings. The symtab is
// pre-seeded with `IS_IE=1` to preserve the legacy ifd/ifnd lowering shape
// for callers that use ConvertSource directly (bypassing the preprocessor).
func NewConverter() *Converter {
	st := NewSymtab()
	_ = st.SetMutable("IS_IE", 1)
	return &Converter{defaultSize: ".l", symtab: st}
}

// ConvertLines converts a slice of input lines to IE64 source. Returns the
// emitted source plus the number of lines that produced "; ERROR:" diagnostics.
func (c *Converter) ConvertLines(input []string) (string, int) {
	e := &Emit{}
	if !c.noHeader {
		e.sb.WriteString("; Converted from m68k by m68kto64\n\n")
	}
	// Pre-lex all lines so peephole fuse can look ahead.
	lines := make([]Line, len(input))
	for i, r := range input {
		lines[i] = LexLine(r)
	}
	// Compute ShadowFPCC liveness across the routine.
	c.fpccLiveAt = computeFPCCLiveness(lines)
	// Phase-2 RTE-walkback: partition the line stream into handler regions
	// when -fp-irq-wrap is enabled. No-op when disabled.
	c.scanRTEHandlerBlocks(lines)
	i := 0
	for i < len(lines) {
		c.curLineIdx = i
		l := lines[i]
		// Peephole fuse: CMP/TST + Bcc on adjacent lines.
		// Skip fuse if either line is itself a branch target. A label can
		// appear inline (`mylabel: cmp.l ...`) or on its own preceding line
		// (`label:\n\tcmp.l ...`) — both forms must inhibit fusion.
		precededByLabelOnly := i > 0 && lines[i-1].Kind == LineLabelOnly
		if !c.noFlagsFuse && fusableProducer(l) && l.Label == "" && !precededByLabelOnly &&
			i+1 < len(lines) && canFuseBcc(lines[i+1]) && lines[i+1].Label == "" {
			c.emitFusedPair(e, l, lines[i+1])
			i += 2
			continue
		}
		// FP fuse: adjacent FCMP/FTST + FBcc (no labels in between).
		if !c.noFlagsFuse && fpFusableProducer(l) && l.Label == "" && !precededByLabelOnly &&
			i+1 < len(lines) && canFuseFBcc(lines[i+1]) && lines[i+1].Label == "" {
			if err := c.emitFusedFCmpFBcc(e, l, lines[i+1]); err != nil {
				e.Lf("; ERROR: FP fuse failed: %v", err)
				c.errors++
			}
			i += 2
			continue
		}
		c.convertLexed(e, l)
		i++
	}
	c.emitFPFooter(e)
	return e.String(), c.errors
}

// emitFusedPair handles a (CMP|TST,Bcc) adjacent pair.
func (c *Converter) emitFusedPair(e *Emit, prod, cons Line) {
	// Caller (ConvertLines) only invokes us when neither line carries a
	// label, so we don't re-emit any label here.
	var err error
	switch prod.Mnemonic {
	case "tst":
		err = c.emitFusedTstBcc(e, prod, cons)
	default: // cmp / cmpi / cmpa
		err = c.emitFusedCmpBcc(e, prod, cons)
	}
	if err != nil {
		c.errors++
		e.Lf("; ERROR: fused %s + %s -> %v", prod.Mnemonic, cons.Mnemonic, err)
	}
}

// convertLexed emits IE64 for one already-lexed line.
func (c *Converter) convertLexed(e *Emit, l Line) {
	switch l.Kind {
	case LineEmpty:
		e.sb.WriteByte('\n')
		return
	case LineComment:
		e.sb.WriteString("; ")
		e.sb.WriteString(l.Comment)
		e.sb.WriteByte('\n')
		return
	case LineLabelOnly:
		e.Label(l.Label)
		if c.fpIrqWrap && c.irqHandlerLabels[l.Label] {
			c.emitIRQHandlerEntry(e, l.Label)
		}
		return
	}
	// Some directives (equ/set/= / rs) bind the label as part of their own
	// syntax — emitting a bare `LABEL:\n` ahead of them would create a
	// duplicate definition. Suppress the auto-label in that case.
	directiveOwnsLabel := l.Kind == LineDirective && labelBindingDirective(l.Mnemonic)
	if l.Label != "" && !directiveOwnsLabel {
		e.Label(l.Label)
		if c.fpIrqWrap && c.irqHandlerLabels[l.Label] {
			c.emitIRQHandlerEntry(e, l.Label)
		}
	}
	if l.Kind == LineDirective {
		if !c.emitDirective(e, l) {
			c.emitDirectivePassthrough(e, l)
		}
		return
	}
	if err := c.emitInstruction(e, l); err != nil {
		c.errors++
		e.Lf("; ERROR: %s -> %v", l.Mnemonic, err)
	}
}

// ConvertSource converts a multi-line source blob.
func (c *Converter) ConvertSource(src string) (string, int) {
	return c.ConvertLines(strings.Split(src, "\n"))
}

// emitDirectivePassthrough is a Phase-2 stub: pass directives through verbatim
// (Phase 4 owns directive translation).
func (c *Converter) emitDirectivePassthrough(e *Emit, l Line) {
	var sb strings.Builder
	sb.WriteString(l.Mnemonic)
	if l.Size != "" {
		sb.WriteString(l.Size)
	}
	if len(l.Operands) > 0 {
		sb.WriteByte(' ')
		sb.WriteString(strings.Join(l.Operands, ","))
	}
	e.L(sb.String())
}

// emitInstruction dispatches to per-mnemonic lowering.
func (c *Converter) emitInstruction(e *Emit, l Line) error {
	// Phase 7: FPU dispatch — before integer Phase-B / 68020-extras paths.
	// Returns handled=true for any f-prefixed op the FPU layer recognises.
	if handled, err := c.emitFPU(e, l); handled {
		return err
	}
	// Phase B: BCD / PACK / UNPK / CAS / TRAPV / bit-field — before the
	// 68020-extras fall-through.
	if handled, err := c.emitPhaseB(e, l); handled {
		return err
	}
	// Phase 5: 68020 extras (TRAP, CHK, MULU.L pair, BFEXTU/BFEXTS, MOVEC).
	if handled, err := c.emit68020Extra(e, l); handled {
		return err
	}
	size := SizeBytes(l.Size)
	if size == 0 {
		// Some mnemonics (LEA, MOVEM, JSR, ...) carry no size or default to .l/.w.
		size = 4
	}
	switch l.Mnemonic {
	case "move", "movea", "moveq":
		return c.emitMove(e, l, size)
	case "lea":
		return c.emitLea(e, l)

	case "add", "adda", "addi", "addq":
		return c.emitArith(e, l, size, "add")
	case "sub", "suba", "subi", "subq":
		return c.emitArith(e, l, size, "sub")
	case "and", "andi":
		if handled, err := c.tryEmitArithCCR(e, l, "and"); handled {
			return err
		}
		return c.emitArith(e, l, size, "and")
	case "or", "ori":
		if handled, err := c.tryEmitArithCCR(e, l, "or"); handled {
			return err
		}
		return c.emitArith(e, l, size, "or")
	case "eor", "eori":
		if handled, err := c.tryEmitArithCCR(e, l, "eor"); handled {
			return err
		}
		return c.emitArith(e, l, size, "eor")
	case "mulu":
		// 68000 MULU.W produces a 32-bit result that overwrites the full Dn.
		// The generic .w path partial-updates only the low 16 — wrong.
		if size == 2 {
			return c.emitMulW(e, l, false)
		}
		return c.emitArith(e, l, size, "mulu")
	case "muls":
		if size == 2 {
			return c.emitMulW(e, l, true)
		}
		return c.emitArith(e, l, size, "muls")
	case "divu":
		// 68000 DIVU.W writes Dn = (rem<<16)|quo as a single 32-bit value.
		if size == 2 {
			return c.emitDivW(e, l, false)
		}
		return c.emitArith(e, l, size, "divu")
	case "divs":
		if size == 2 {
			return c.emitDivW(e, l, true)
		}
		return c.emitArith(e, l, size, "divs")

	case "neg":
		return c.emitUnary(e, l, size, "neg")
	case "not":
		return c.emitUnary(e, l, size, "not")
	case "clr":
		return c.emitClr(e, l, size)

	case "lsl":
		return c.emitShift(e, l, size, "lsl", "lsl")
	case "asl":
		// IE64 has no ASL; m68k ASL == LSL for the shift, but ASL writes V
		// based on sign-bit changes. Pass the original mnemonic so shadow
		// emission can compute V correctly.
		return c.emitShift(e, l, size, "lsl", "asl")
	case "lsr":
		return c.emitShift(e, l, size, "lsr", "lsr")
	case "asr":
		return c.emitShift(e, l, size, "asr", "asr")
	case "rol":
		return c.emitShift(e, l, size, "rol", "rol")
	case "ror":
		return c.emitShift(e, l, size, "ror", "ror")

	case "ext":
		return c.emitExt(e, l, size, false)
	case "extb":
		return c.emitExt(e, l, size, true)
	case "swap":
		return c.emitSwap(e, l)
	case "st":
		return c.emitSetByte(e, l, true)
	case "sf":
		return c.emitSetByte(e, l, false)
	case "seq", "sne", "slt", "sge", "sgt", "sle", "shi", "sls", "scc", "scs", "smi", "spl", "svs", "svc":
		return c.emitShadowScc(e, l)
	case "btst":
		return c.emitBtst(e, l, size)
	case "bset":
		return c.emitBitRmw(e, l, "set")
	case "bclr":
		return c.emitBitRmw(e, l, "clr")
	case "bchg":
		return c.emitBitRmw(e, l, "chg")
	case "tas":
		return c.emitTas(e, l)
	case "exg":
		return c.emitExg(e, l)
	case "cmpm":
		return c.emitCmpm(e, l)
	case "illegal":
		e.L("syscall #19")
		return nil
	case "rte":
		return c.emitRte(e, l)
	case "stop":
		return c.emitStop(e, l)
	case "reset":
		return c.emitResetOp(e, l)

	case "nop":
		e.L("; nop (m68k) — no IE64 output (transparent)")
		return nil

	// =====================================================================
	// Phase 3: control flow
	// =====================================================================
	case "bra":
		return c.emitBra(e, l)
	case "bsr":
		return c.emitBsr(e, l)
	case "jmp":
		return c.emitJmp(e, l)
	case "jsr":
		return c.emitJsr(e, l)
	case "rts":
		return c.emitRts(e)
	case "rtr":
		return c.emitRtr(e)
	case "movem":
		return c.emitMovem(e, l)
	case "link":
		return c.emitLink(e, l)
	case "unlk":
		return c.emitUnlk(e, l)
	case "cmp", "cmpi", "cmpa":
		return c.emitCmpShadow(e, l, size)
	case "tst":
		return c.emitTstShadow(e, l, size)
	case "beq", "bne", "blt", "bge", "bgt", "ble", "bhi", "bls",
		"bcc", "bcs", "bmi", "bpl", "bvs", "bvc":
		return c.emitShadowBcc(e, l)
	case "dbra", "dbf":
		return c.emitDbra(e, l)
	case "dbeq", "dbne", "dblt", "dbge", "dbgt", "dble", "dbhi", "dbls",
		"dbcc", "dbcs", "dbmi", "dbpl", "dbvs", "dbvc", "dbt":
		return c.emitShadowDBcc(e, l)
	}
	// Unknown mnemonic. After Phase E, the preprocessor expands macros at
	// transpile time, so an unknown mnemonic reaching the converter via the
	// ConvertFile path is a real error — gated by werrorUnknownMnem (default
	// on through DefaultPreprocOpts). ConvertSource direct callers default
	// the flag off so the legacy passthrough survives for migration users.
	if c.strict {
		return fmt.Errorf("mnemonic %q not yet lowered", l.Mnemonic)
	}
	if c.werrorUnknownMnem {
		return fmt.Errorf("unknown mnemonic %q", l.Mnemonic)
	}
	c.emitDirectivePassthrough(e, l)
	return nil
}

// =====================================================================
// Phase 3: control-flow lowerings
// =====================================================================

// emitBra emits an unconditional PC-relative branch.
func (c *Converter) emitBra(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("bra requires 1 operand")
	}
	e.Lf("bra %s", strings.TrimSpace(l.Operands[0]))
	return nil
}

// emitBsr emits a m68k BSR — push 4-byte return PC onto guest stack, branch.
func (c *Converter) emitBsr(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("bsr requires 1 operand")
	}
	target := strings.TrimSpace(l.Operands[0])
	ret := e.NewLabel("bsr_ret")
	e.Lf("sub.l %s, %s, #4", GuestSP, GuestSP)
	e.Lf("la %s, %s", ScrV1, ret)
	e.Lf("store.l %s, (%s)", ScrV1, GuestSP)
	e.Lf("bra %s", target)
	e.Label(ret)
	return nil
}

// emitJmp emits m68k JMP — register-indirect or label.
func (c *Converter) emitJmp(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("jmp requires 1 operand")
	}
	op, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	switch op.Mode {
	case AMIndirect:
		e.Lf("jmp (%s)", op.Reg.IE64)
		return nil
	case AMDispAn:
		e.Lf("jmp %s(%s)", dispOrZero(op.Disp), op.Reg.IE64)
		return nil
	case AMIndexAn:
		c.emitIndexAddr(e, op, ScrEA)
		e.Lf("jmp (%s)", ScrEA)
		return nil
	case AMAbsW, AMAbsL, AMDispPC, AMIndexPC:
		// PC-rel and labelled absolute targets resolve via `bra` (ie64asm
		// patches the literal). Numeric (xxx).w needs explicit sign-extend
		// to a 32-bit address before jumping, since `bra $FFFE` would
		// resolve to +65534 instead of m68k's 0xFFFFFFFE.
		if op.Mode == AMAbsW && looksLikeNumericDisp(op.Disp) {
			e.Lf("la %s, %s", ScrV1, op.Disp)
			maybeSignExtAbsW(e, ScrV1, op.Mode)
			e.Lf("jmp (%s)", ScrV1)
			return nil
		}
		e.Lf("bra %s", op.Disp)
		return nil
	}
	return fmt.Errorf("jmp: unsupported mode %v", op.Mode)
}

// emitJsr emits m68k JSR — push 4-byte return PC, jump (or branch) to target.
func (c *Converter) emitJsr(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("jsr requires 1 operand")
	}
	op, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	ret := e.NewLabel("jsr_ret")
	e.Lf("sub.l %s, %s, #4", GuestSP, GuestSP)
	e.Lf("la %s, %s", ScrV1, ret)
	e.Lf("store.l %s, (%s)", ScrV1, GuestSP)
	switch op.Mode {
	case AMIndirect:
		e.Lf("jmp (%s)", op.Reg.IE64)
	case AMDispAn:
		e.Lf("jmp %s(%s)", dispOrZero(op.Disp), op.Reg.IE64)
	case AMIndexAn:
		c.emitIndexAddr(e, op, ScrEA)
		e.Lf("jmp (%s)", ScrEA)
	case AMAbsW, AMAbsL, AMDispPC, AMIndexPC:
		if op.Mode == AMAbsW && looksLikeNumericDisp(op.Disp) {
			e.Lf("la %s, %s", ScrV1, op.Disp)
			maybeSignExtAbsW(e, ScrV1, op.Mode)
			e.Lf("jmp (%s)", ScrV1)
		} else {
			e.Lf("bra %s", op.Disp)
		}
	default:
		return fmt.Errorf("jsr: unsupported mode %v", op.Mode)
	}
	e.Label(ret)
	return nil
}

// emitRts emits m68k RTS — pop return PC from guest stack, jump.
func (c *Converter) emitRts(e *Emit) error {
	e.Lf("load.l %s, (%s)", ScrV1, GuestSP)
	e.Lf("add.l %s, %s, #4", GuestSP, GuestSP)
	e.Lf("jmp (%s)", ScrV1)
	return nil
}

// emitRtr lowers RTR — pop CCR (lower byte) into shadow CCR, then RTS-like
// pop of return PC. m68k stack layout: CCR (16 bits, of which only low 5
// hold N/Z/V/C/X) at SP, then PC at SP+2 (.w) or SP+4 depending on stack
// frame; vasm/devpac emit a 2-byte CCR + 4-byte PC frame on user-mode entry.
//
// Lowering treats the CCR as a 16-bit value at (sp), unpacks N/Z/V/C into
// the shadow registers, then pops PC like RTS.
func (c *Converter) emitRtr(e *Emit) error {
	// Read 16-bit CCR.
	e.Lf("load.w %s, (%s)", ScrV1, GuestSP)
	e.Lf("add.l %s, %s, #2", GuestSP, GuestSP)
	// CCR bit layout (m68k):  bit4=X, bit3=N, bit2=Z, bit1=V, bit0=C.
	// Unpack into shadows. ShadowN: m68k N is "result negative" — store the
	// bit value (0/1) into the sign of r24 by sign-extending −1 if set.
	// ShadowZ: r25 is "zero iff Z=1"; we want r25=0 when Z bit set, else 1.
	// ShadowC, ShadowV: 0/1.
	// X (extend): we map to ShadowC for chained-arith use.
	e.Lf("and.l %s, %s, #1", ShadowC, ScrV1) // C
	e.Lf("lsr.l %s, %s, #1", ShadowV, ScrV1)
	e.Lf("and.l %s, %s, #1", ShadowV, ShadowV) // V
	// Z bit (bit 2):
	e.Lf("lsr.l %s, %s, #2", ShadowTmp1, ScrV1)
	e.Lf("and.l %s, %s, #1", ShadowTmp1, ShadowTmp1)
	// ShadowZ semantics: r25 nonzero ⇔ Z=0. So r25 = NOT(zBit).
	e.Lf("eor.l %s, %s, #1", ShadowZ, ShadowTmp1)
	// N bit (bit 3): set r24 to -1 if N=1 else 0 (sign-extended).
	e.Lf("lsr.l %s, %s, #3", ShadowTmp1, ScrV1)
	e.Lf("and.l %s, %s, #1", ShadowTmp1, ShadowTmp1)
	e.Lf("neg.q %s, %s", ShadowN, ShadowTmp1)
	// Pop PC.
	e.Lf("load.l %s, (%s)", ScrV1, GuestSP)
	e.Lf("add.l %s, %s, #4", GuestSP, GuestSP)
	e.Lf("jmp (%s)", ScrV1)
	return nil
}

// emitLink emits m68k LINK An,#d — push old An, set An to current SP, allocate d bytes.
func (c *Converter) emitLink(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("link requires 2 operands (An,#disp)")
	}
	an, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if an.Mode != AMAddrReg {
		return fmt.Errorf("link: first operand must be An")
	}
	disp, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	if disp.Mode != AMImmediate {
		return fmt.Errorf("link: second operand must be #imm")
	}
	rA := an.Reg.IE64
	e.Lf("sub.l %s, %s, #4", GuestSP, GuestSP)
	e.Lf("store.l %s, (%s)", rA, GuestSP)
	e.Lf("move.l %s, %s", rA, GuestSP)
	e.Lf("add.l %s, %s, #%s", GuestSP, GuestSP, disp.Imm)
	return nil
}

// emitUnlk emits m68k UNLK An — SP := An; pop old An.
func (c *Converter) emitUnlk(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("unlk requires 1 operand (An)")
	}
	an, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if an.Mode != AMAddrReg {
		return fmt.Errorf("unlk: operand must be An")
	}
	rA := an.Reg.IE64
	e.Lf("move.l %s, %s", GuestSP, rA)
	e.Lf("load.l %s, (%s)", rA, GuestSP)
	e.Lf("add.l %s, %s, #4", GuestSP, GuestSP)
	return nil
}

// emitSwap lowers SWAP Dn — swap upper/lower 16 bits of Dn (32-bit operation).
// CCR: N/Z from result (full 32-bit), C=0, V=0.
func (c *Converter) emitSwap(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("swap requires 1 operand")
	}
	op, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if op.Mode != AMDataReg {
		return fmt.Errorf("swap operand must be Dn")
	}
	e.Lf("rol.l %s, %s, #16", op.Reg.IE64, op.Reg.IE64)
	c.emitShadowsForLogical(e, op.Reg.IE64, 4)
	return nil
}

// emitSetByte lowers ST (always-set) and SF (always-clear) — write $FF or $00
// to the low byte of Dn, preserving upper bits.
func (c *Converter) emitSetByte(e *Emit, l Line, set bool) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("st/sf requires 1 operand")
	}
	op, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if op.Mode == AMDataReg {
		rd := op.Reg.IE64
		// Clear low byte.
		e.Lf("and.q %s, %s, #%s", rd, rd, SizeInvMask(1))
		if set {
			e.Lf("or.q %s, %s, #$FF", rd, rd)
		}
		return nil
	}
	// Memory destination: store the constant.
	val := "r0"
	if set {
		e.Lf("move.b %s, #$FF", ScrV1)
		val = ScrV1
	}
	return c.storeValue(e, op, 1, val)
}

// emitBtst lowers BTST <bit>,<dst> — Z = NOT bit. m68k BTST sets Z to 1 if
// the tested bit is 0 (i.e. bit==0 → Z=1 → BEQ taken). N/V/C unaffected.
//
// For Dn dst: bit number modulo 32. For memory dst: modulo 8.
func (c *Converter) emitBtst(e *Emit, l Line, size int) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("btst requires 2 operands")
	}
	bitOp, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	dst, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	mod := 8
	loadSize := 1
	if dst.Mode == AMDataReg {
		mod = 32
		loadSize = 4
	}
	// Materialise bit number into ScrV1.
	switch bitOp.Mode {
	case AMImmediate:
		e.Lf("move.l %s, #%s", ScrV1, bitOp.Imm)
	case AMDataReg:
		e.Lf("move.l %s, %s", ScrV1, bitOp.Reg.IE64)
	default:
		return fmt.Errorf("btst bit operand must be #imm or Dn")
	}
	e.Lf("and.l %s, %s, #%d", ScrV1, ScrV1, mod-1)
	// Load dst value.
	dstReg, _, err := c.loadValue(e, dst, loadSize, ScrV2)
	if err != nil {
		return err
	}
	// Z = NOT bit: r25 holds (val >> bitNum) & 1; m68k convention is
	// "Z=1 ⇔ bit was 0", i.e. consumers do `beqz r25, L` for BEQ. So we
	// store the bit value directly: r25 = bitval.
	e.Lf("lsr.l %s, %s, %s", ShadowZ, dstReg, ScrV1)
	e.Lf("and.l %s, %s, #1", ShadowZ, ShadowZ)
	// N/V/C unaffected — leave alone.
	return nil
}

// emitBitRmw lowers BSET / BCLR / BCHG <bit>,<dst>.
//
// Semantics: read the dst at width (long for Dn, byte for memory), set
// ShadowZ to the pre-op bit value (ShadowZ contract: r25 nonzero ⇔ Z=0,
// so r25 := bitValue matches m68k Z = NOT(bit)). Then modify the dst with
// OR (set) / AND-NOT (clr) / EOR (chg) using a mask = 1 << bitNum.
// N / V / C unaffected. Bit numbers mask mod 32 (Dn) or mod 8 (memory).
func (c *Converter) emitBitRmw(e *Emit, l Line, op string) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s requires 2 operands", l.Mnemonic)
	}
	bitOp, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	dst, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	mod := 8
	size := 1
	if dst.Mode == AMDataReg {
		mod = 32
		size = 4
	}

	switch bitOp.Mode {
	case AMImmediate:
		e.Lf("move.l %s, #%s", ScrV1, bitOp.Imm)
	case AMDataReg:
		e.Lf("move.l %s, %s", ScrV1, bitOp.Reg.IE64)
	default:
		return fmt.Errorf("%s bit operand must be #imm or Dn", l.Mnemonic)
	}
	e.Lf("and.l %s, %s, #%d", ScrV1, ScrV1, mod-1)

	// Mask = 1 << bitNum into ScrV2.
	e.Lf("move.l %s, #1", ScrV2)
	e.Lf("lsl.l %s, %s, %s", ScrV2, ScrV2, ScrV1)

	apply := func(target string) {
		// Z := pre-op bit.
		e.Lf("lsr.l %s, %s, %s", ShadowZ, target, ScrV1)
		e.Lf("and.l %s, %s, #1", ShadowZ, ShadowZ)
		switch op {
		case "set":
			e.Lf("or.l %s, %s, %s", target, target, ScrV2)
		case "clr":
			e.Lf("not.l %s, %s", ScrAux, ScrV2)
			e.Lf("and.l %s, %s, %s", target, target, ScrAux)
		case "chg":
			e.Lf("eor.l %s, %s, %s", target, target, ScrV2)
		}
	}

	if dst.Mode == AMDataReg {
		apply(dst.Reg.IE64)
		return nil
	}
	h, err := c.loadDstRMW(e, dst, size)
	if err != nil {
		return err
	}
	apply(h.valReg)
	return c.storeDstRMW(e, h, h.valReg)
}

// emitTas lowers TAS <ea> — test byte (set N/Z from pre-op value, clear V/C),
// then set bit 7 of the byte in dst.
//
// For Dn destinations the operation is purely in-register and atomicity is
// trivially preserved by single-threaded transpile-time codegen. For memory
// destinations we emit `syscall #20`, the host-pinned atomic test-and-set
// primitive (see §11): the host receives the target byte address in r17
// (ScrV1) and returns the pre-op byte value in r17, performing load + set-bit-7
// + store as one indivisible operation. Shadows are derived from the
// returned pre-op byte.
func (c *Converter) emitTas(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("tas requires 1 operand")
	}
	dst, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if dst.Mode == AMDataReg {
		rd := dst.Reg.IE64
		e.Lf("and.l %s, %s, #$FF", ScrV1, rd)
		e.Lf("sext.b %s, %s", ShadowN, ScrV1)
		e.Lf("move.l %s, %s", ShadowZ, ScrV1)
		c.emitShadowClearC(e)
		c.emitShadowClearV(e)
		e.Lf("or.l %s, %s, #$80", rd, rd)
		return nil
	}

	// Compute byte address into ScrV1 (r17) per the syscall #20 ABI.
	switch dst.Mode {
	case AMIndirect:
		e.Lf("move.l %s, %s", ScrV1, dst.Reg.IE64)
	case AMDispAn:
		e.Lf("lea %s, %s(%s)", ScrV1, dispOrZero(dst.Disp), dst.Reg.IE64)
	case AMAbsW, AMAbsL:
		e.Lf("la %s, %s", ScrV1, dst.Disp)
		maybeSignExtAbsW(e, ScrV1, dst.Mode)
	case AMPostInc:
		e.Lf("move.l %s, %s", ScrV1, dst.Reg.IE64)
		e.Lf("add.l %s, %s, #1", dst.Reg.IE64, dst.Reg.IE64)
	case AMPreDec:
		e.Lf("sub.l %s, %s, #1", dst.Reg.IE64, dst.Reg.IE64)
		e.Lf("move.l %s, %s", ScrV1, dst.Reg.IE64)
	default:
		return fmt.Errorf("tas: EA mode %v not supported", dst.Mode)
	}
	// Atomic test-and-set: host returns pre-op byte in r17 after performing
	// load + set-bit-7 + store as one operation.
	e.L("syscall #20")
	// Derive shadows from the returned pre-op byte.
	e.Lf("and.l %s, %s, #$FF", ScrV1, ScrV1)
	e.Lf("sext.b %s, %s", ShadowN, ScrV1)
	e.Lf("move.l %s, %s", ShadowZ, ScrV1)
	c.emitShadowClearC(e)
	c.emitShadowClearV(e)
	return nil
}

// looksLikeNumericDisp returns true when a Disp string is a numeric
// literal ($-hex, %-binary, 0x-hex, or decimal) rather than a label or
// symbolic expression. Used to decide whether AMAbsW JMP/JSR targets need
// explicit sign-extension before the jump.
func looksLikeNumericDisp(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if s[0] == '$' || s[0] == '%' || s[0] == '+' || s[0] == '-' {
		return true
	}
	if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
		return true
	}
	return s[0] >= '0' && s[0] <= '9'
}

// maybeSignExtAbsW emits `sext.w reg, reg` when `mode == AMAbsW`. m68k
// `(xxx).w` short-absolute addresses are 16-bit signed and sign-extend to
// 32 bits before forming the EA; IE64 `la` zero-extends, so addresses like
// `$FFFE.w` would resolve to +65534 instead of -2 / 0xFFFFFFFE.
//
// Called immediately after every `la reg, op.Disp` emit that lowers an
// AMAbsW operand. No-op for AMAbsL (already 32-bit) and other modes.
func maybeSignExtAbsW(e *Emit, reg string, mode AddrMode) {
	if mode == AMAbsW {
		e.Lf("sext.w %s, %s ; (xxx).w sign-extend to 32-bit address", reg, reg)
	}
}

// emitUnpackCCRBits unpacks the low byte of a 16-bit m68k SR/CCR value held
// in `srcReg` into the shadow CCR registers. Bit layout:
//   bit 0 = C, bit 1 = V, bit 2 = Z, bit 3 = N, bit 4 = X.
// Shadow contract: ShadowN sign-extended (−1 if N=1 else 0), ShadowZ inverted
// (r25 nonzero ⇔ Z=0), ShadowC / ShadowV / ShadowX as 0/1 bits.
func (c *Converter) emitUnpackCCRBits(e *Emit, srcReg string) {
	// C bit 0.
	e.Lf("and.l %s, %s, #1", ShadowC, srcReg)
	// V bit 1.
	e.Lf("lsr.l %s, %s, #1", ShadowV, srcReg)
	e.Lf("and.l %s, %s, #1", ShadowV, ShadowV)
	// Z bit 2 → ShadowZ = NOT(Z).
	e.Lf("lsr.l %s, %s, #2", ShadowTmp1, srcReg)
	e.Lf("and.l %s, %s, #1", ShadowTmp1, ShadowTmp1)
	e.Lf("eor.l %s, %s, #1", ShadowZ, ShadowTmp1)
	// N bit 3 → ShadowN = neg(Nbit) so bit set sign-extends to −1.
	e.Lf("lsr.l %s, %s, #3", ShadowTmp1, srcReg)
	e.Lf("and.l %s, %s, #1", ShadowTmp1, ShadowTmp1)
	e.Lf("neg.q %s, %s", ShadowN, ShadowTmp1)
	// X bit 4 → ShadowX = 0/1 bit.
	e.Lf("lsr.l %s, %s, #4", ShadowX, srcReg)
	e.Lf("and.l %s, %s, #1", ShadowX, ShadowX)
}

// emitExg lowers EXG Rx,Ry — exchange two 32-bit registers. m68k allows
// Dn↔Dn, An↔An, and Dn↔An. CCR unaffected.
func (c *Converter) emitExg(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("exg requires 2 operands")
	}
	a, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	b, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	if (a.Mode != AMDataReg && a.Mode != AMAddrReg) ||
		(b.Mode != AMDataReg && b.Mode != AMAddrReg) {
		return fmt.Errorf("exg: both operands must be Dn or An")
	}
	ra, rb := a.Reg.IE64, b.Reg.IE64
	e.Lf("move.l %s, %s", ScrV1, ra)
	e.Lf("move.l %s, %s", ra, rb)
	e.Lf("move.l %s, %s", rb, ScrV1)
	return nil
}

// emitCmpm lowers CMPM.<sz> (Ay)+,(Ax)+ — postinc-read both operands,
// compute dst - src for shadow CCR (no writeback).
func (c *Converter) emitCmpm(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("cmpm requires 2 operands")
	}
	src, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	dst, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	if src.Mode != AMPostInc || dst.Mode != AMPostInc {
		return fmt.Errorf("cmpm: both operands must be (An)+")
	}
	size := SizeBytes(l.Size)
	if size == 0 {
		size = 2
	}
	szIE := IE64Size(size)
	e.Lf("load%s %s, (%s)", szIE, ScrV1, src.Reg.IE64)
	e.Lf("add.l %s, %s, #%d", src.Reg.IE64, src.Reg.IE64, size)
	e.Lf("load%s %s, (%s)", szIE, ScrV2, dst.Reg.IE64)
	e.Lf("add.l %s, %s, #%d", dst.Reg.IE64, dst.Reg.IE64, size)
	// Shadow CCR via sub semantics (dst - src). Snapshot dst pre-op.
	e.Lf("move.l %s, %s", ShadowSnap, ScrV2)
	e.Lf("sub%s %s, %s, %s", szIE, ScrAux, ScrV2, ScrV1)
	c.emitArithShadows(e, "sub", ScrAux, ShadowSnap, ScrV1, size)
	return nil
}

// emitRte lowers RTE — return from exception. Pops 16-bit SR (with its low
// byte = CCR) then 4-byte PC from the guest stack, unpacks the CCR bits
// (X/N/Z/V/C) into the shadow registers via the shared `emitUnpackCCRBits`
// helper, and jumps to the popped PC. The supervisor-only high byte (T,S,M,
// interrupt mask) has no IE64 user-mode representation and is discarded —
// the transpile target is single-context user mode, so the discarded bits
// have no effect on continued execution.
//
// 68000 stack-frame format (6 bytes total: SR + PC) is assumed. 68010+
// format/vector words sit AFTER the PC; user code that runs RTE under an
// 8-byte frame must drop the format word with an additional `addq.l #2,sp`
// before the RTE. Document under §12.
func (c *Converter) emitRte(e *Emit, l Line) error {
	// Phase-2 IRQ wrap: if this RTE was identified by scanRTEHandlerBlocks
	// as a handler exit, emit the FP-slot restore stub first. Orphan RTE
	// (no preceding label) cannot be wrapped — error under -strict, diag
	// under default.
	if c.fpIrqWrap {
		if _, isHandlerRTE := c.irqHandlerRTELine[c.curLineIdx]; isHandlerRTE {
			c.emitIRQHandlerExit(e)
		} else if c.irqOrphanRTELine[c.curLineIdx] {
			if c.strict {
				return fmt.Errorf("cannot determine handler entry for RTE (no preceding label); disable -fp-irq-wrap or add a label")
			}
			e.L("; m68kto64: orphan RTE — no preceding label, FP-slot wrap skipped")
		}
	}
	// Pop 16-bit SR.
	e.Lf("load.w %s, (%s)", ScrV1, GuestSP)
	e.Lf("add.l %s, %s, #2", GuestSP, GuestSP)
	// Unpack CCR bits from low byte of SR.
	c.emitUnpackCCRBits(e, ScrV1)
	// Pop 32-bit PC and jump.
	e.Lf("load.l %s, (%s)", ScrV1, GuestSP)
	e.Lf("add.l %s, %s, #4", GuestSP, GuestSP)
	e.Lf("jmp (%s)", ScrV1)
	return nil
}

// emitStop lowers STOP #imm — load SR with the immediate, then halt until
// interrupt. The CCR portion (low byte of #imm) is unpacked into the shadow
// registers exactly like RTE. The halt itself is delegated to the host via
// `syscall #21`, the host-pinned suspend-until-interrupt primitive (see §11).
func (c *Converter) emitStop(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("stop requires 1 operand (#imm)")
	}
	imm, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if imm.Mode != AMImmediate {
		return fmt.Errorf("stop: operand must be #imm")
	}
	// Materialise the SR-load value and unpack its CCR bits.
	e.Lf("move.l %s, #%s", ScrV1, imm.Imm)
	c.emitUnpackCCRBits(e, ScrV1)
	// Suspend until interrupt — host-pinned syscall ABI.
	e.L("syscall #21")
	return nil
}

// emitResetOp lowers RESET — broadcast peripheral reset. Delegated to the
// host via `syscall #22`, the host-pinned reset-peripherals primitive
// (see §11). CCR / register state preserved per m68k spec.
func (c *Converter) emitResetOp(e *Emit, l Line) error {
	e.L("syscall #22")
	return nil
}

// emitDbra lowers DBRA/DBF Dn,L — decrement low 16 bits of Dn; if result !=
// -1 (i.e. low word != $FFFF), branch to L.
//
// Skeleton (no condition test, since DBRA has no cc):
//
//	and.l  scrV1, Dn, #$FFFF
//	sub.l  scrV1, scrV1, #1
//	and.l  scrV1, scrV1, #$FFFF
//	and.q  scrV2, Dn,    #$FFFFFFFFFFFF0000
//	or.q   Dn,    scrV2, scrV1
//	move.l scrV2, #$FFFF
//	bne    scrV1, scrV2, L
func (c *Converter) emitDbra(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("dbra requires 2 operands (Dn,label)")
	}
	dn, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if dn.Mode != AMDataReg {
		return fmt.Errorf("dbra: first operand must be Dn")
	}
	label := strings.TrimSpace(l.Operands[1])
	rd := dn.Reg.IE64
	e.Lf("and.l %s, %s, #$FFFF", ScrV1, rd)
	e.Lf("sub.l %s, %s, #1", ScrV1, ScrV1)
	e.Lf("and.l %s, %s, #$FFFF", ScrV1, ScrV1)
	e.Lf("and.q %s, %s, #%s", ScrV2, rd, SizeInvMask(2))
	e.Lf("or.q %s, %s, %s", rd, ScrV2, ScrV1)
	e.Lf("move.l %s, #$FFFF", ScrV2)
	e.Lf("bne %s, %s, %s", ScrV1, ScrV2, label)
	return nil
}

// =====================================================================
// MOVEM
// =====================================================================

// expandRegList resolves a m68k register list ("d0-d7/a0-a3") to an ordered
// slice of IE64 register names. Order follows the m68k EA-direction: for
// predecrement destinations the caller reverses the slice.
func expandRegList(list string) ([]string, error) {
	// m68k canonical order: d0,d1,...,d7,a0,a1,...,a7
	canonical := []string{"d0", "d1", "d2", "d3", "d4", "d5", "d6", "d7",
		"a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7"}
	include := map[string]bool{}
	for _, chunk := range strings.Split(list, "/") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		if i := strings.Index(chunk, "-"); i >= 0 {
			lo := strings.TrimSpace(chunk[:i])
			hi := strings.TrimSpace(chunk[i+1:])
			loIdx := indexOf(canonical, strings.ToLower(lo))
			hiIdx := indexOf(canonical, strings.ToLower(hi))
			if loIdx < 0 || hiIdx < 0 || loIdx > hiIdx {
				return nil, fmt.Errorf("invalid range %q", chunk)
			}
			for j := loIdx; j <= hiIdx; j++ {
				include[canonical[j]] = true
			}
		} else {
			name := strings.ToLower(chunk)
			if indexOf(canonical, name) < 0 {
				return nil, fmt.Errorf("unknown register %q", chunk)
			}
			include[name] = true
		}
	}
	var out []string
	for _, n := range canonical {
		if include[n] {
			r, _ := LookupRegister(n)
			out = append(out, r.IE64)
		}
	}
	return out, nil
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// emitMovem handles MOVEM <regs>,<ea> and MOVEM <ea>,<regs>.
func (c *Converter) emitMovem(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("movem requires 2 operands")
	}
	size := SizeBytes(l.Size)
	if size == 0 {
		size = 2 // .w default
	}
	if size != 2 && size != 4 {
		return fmt.Errorf("movem size must be .w or .l")
	}
	szIE := IE64Size(size)
	a, errA := ParseOperand(l.Operands[0])
	b, errB := ParseOperand(l.Operands[1])
	if errA != nil {
		return errA
	}
	if errB != nil {
		return errB
	}
	// Determine direction.
	if a.Mode == AMRegList {
		// regs → ea (store)
		regs, err := expandRegList(a.List)
		if err != nil {
			return err
		}
		return c.emitMovemStore(e, regs, b, size, szIE)
	}
	if b.Mode == AMRegList {
		// ea → regs (load)
		regs, err := expandRegList(b.List)
		if err != nil {
			return err
		}
		return c.emitMovemLoad(e, regs, a, size, szIE)
	}
	// Single-register movem: m68k accepts `movem.<sz> Dn,<ea>` and
	// `movem.<sz> <ea>,Dn` with one register (the regList degenerates to
	// a singleton). Treat the bare register as a 1-element reglist when
	// the other operand is a memory EA.
	isReg := func(op Operand) bool { return op.Mode == AMDataReg || op.Mode == AMAddrReg }
	isMemEA := func(op Operand) bool {
		switch op.Mode {
		case AMIndirect, AMPostInc, AMPreDec, AMDispAn, AMIndexAn, AMAbsW, AMAbsL, AMDispPC, AMIndexPC:
			return true
		}
		return false
	}
	if isReg(a) && isMemEA(b) {
		return c.emitMovemStore(e, []string{a.Reg.IE64}, b, size, szIE)
	}
	if isMemEA(a) && isReg(b) {
		return c.emitMovemLoad(e, []string{b.Reg.IE64}, a, size, szIE)
	}
	return fmt.Errorf("movem: one operand must be a register list")
}

func (c *Converter) emitMovemStore(e *Emit, regs []string, ea Operand, size int, szIE string) error {
	if ea.Mode == AMPreDec {
		// Predecrement: reverse order, sub-then-store per reg.
		rA := ea.Reg.IE64
		for i := len(regs) - 1; i >= 0; i-- {
			e.Lf("sub.l %s, %s, #%d", rA, rA, size)
			e.Lf("store%s %s, (%s)", szIE, regs[i], rA)
		}
		return nil
	}
	// Forward: compute base into ScrEA.
	if err := c.emitEABase(e, ea, ScrEA); err != nil {
		return err
	}
	for i, r := range regs {
		e.Lf("store%s %s, %d(%s)", szIE, r, i*size, ScrEA)
	}
	return nil
}

func (c *Converter) emitMovemLoad(e *Emit, regs []string, ea Operand, size int, szIE string) error {
	if ea.Mode == AMPostInc {
		rA := ea.Reg.IE64
		for _, r := range regs {
			if size == 2 {
				// .w MOVEM load sign-extends to .l.
				e.Lf("load.w %s, (%s)", r, rA)
				e.Lf("sext.w %s, %s", r, r)
			} else {
				e.Lf("load.l %s, (%s)", r, rA)
			}
			e.Lf("add.l %s, %s, #%d", rA, rA, size)
		}
		return nil
	}
	if err := c.emitEABase(e, ea, ScrEA); err != nil {
		return err
	}
	for i, r := range regs {
		if size == 2 {
			e.Lf("load.w %s, %d(%s)", r, i*size, ScrEA)
			e.Lf("sext.w %s, %s", r, r)
		} else {
			e.Lf("load.l %s, %d(%s)", r, i*size, ScrEA)
		}
	}
	return nil
}

// emitEABase materialises the base address of ea into `dst`. Supports
// (An), d(An), (xxx).w/.l, indexed, PC-rel.
func (c *Converter) emitEABase(e *Emit, ea Operand, dst string) error {
	switch ea.Mode {
	case AMIndirect:
		if dst != ea.Reg.IE64 {
			e.Lf("move.l %s, %s", dst, ea.Reg.IE64)
		}
		return nil
	case AMDispAn:
		e.Lf("lea %s, %s(%s)", dst, dispOrZero(ea.Disp), ea.Reg.IE64)
		return nil
	case AMIndexAn:
		c.emitIndexAddr(e, ea, dst)
		return nil
	case AMAbsW, AMAbsL, AMDispPC, AMIndexPC:
		e.Lf("la %s, %s", dst, ea.Disp)
		maybeSignExtAbsW(e, dst, ea.Mode)
		if ea.Mode == AMIndexPC {
			c.emitIndexCombine(e, ea.Index, dst)
		}
		return nil
	}
	return fmt.Errorf("emitEABase: unsupported mode %v", ea.Mode)
}

// =====================================================================
// Operand load / store helpers
// =====================================================================

// loadValue emits the IE64 sequence to materialise the value of `op` (m68k
// width = `size` bytes) into a register the caller can consume. Returns the
// register name holding the value, or — for immediates — an empty regName and
// the immediate text in immText.
//
// `scratch` is the scratch register to use when loading from memory; it must
// be one of ScrV1 / ScrV2 (callers pick to avoid clashes).
//
// Width semantics: the value comes back masked to `size` low bytes (upper
// bits zero). Sign-extension is the caller's job (used only by signed fused
// branches in Phase 3).
func (c *Converter) loadValue(e *Emit, op Operand, size int, scratch string) (regName string, immText string, err error) {
	sz := IE64Size(size)
	switch op.Mode {
	case AMDataReg:
		if size == 4 {
			// Full-width data register; ALU3 will mask anyway, but caller
			// expects "value <= mask"; emit a mask if Phase 2 caller asks
			// for strict masking (see emitArith).
			return op.Reg.IE64, "", nil
		}
		// Mask low bytes into scratch.
		e.Lf("and.l %s, %s, #%s", scratch, op.Reg.IE64, SizeMask(size))
		return scratch, "", nil
	case AMAddrReg:
		return op.Reg.IE64, "", nil
	case AMImmediate:
		return "", op.Imm, nil
	case AMIndirect:
		e.Lf("load%s %s, (%s)", sz, scratch, op.Reg.IE64)
		return scratch, "", nil
	case AMPostInc:
		e.Lf("load%s %s, (%s)", sz, scratch, op.Reg.IE64)
		e.Lf("add.l %s, %s, #%d", op.Reg.IE64, op.Reg.IE64, postIncStep(op.Reg, size))
		return scratch, "", nil
	case AMPreDec:
		e.Lf("sub.l %s, %s, #%d", op.Reg.IE64, op.Reg.IE64, postIncStep(op.Reg, size))
		e.Lf("load%s %s, (%s)", sz, scratch, op.Reg.IE64)
		return scratch, "", nil
	case AMDispAn:
		e.Lf("load%s %s, %s(%s)", sz, scratch, dispOrZero(op.Disp), op.Reg.IE64)
		return scratch, "", nil
	case AMIndexAn:
		c.emitIndexAddr(e, op, ScrEA)
		e.Lf("load%s %s, (%s)", sz, scratch, ScrEA)
		return scratch, "", nil
	case AMAbsW, AMAbsL:
		e.Lf("la %s, %s", ScrEA, op.Disp)
		maybeSignExtAbsW(e, ScrEA, op.Mode)
		e.Lf("load%s %s, (%s)", sz, scratch, ScrEA)
		return scratch, "", nil
	case AMDispPC:
		// PC-relative collapses to `la <label>` since labels resolve at
		// assemble time. Disp may be empty (treated as 0) for `(label,pc)`
		// where the label *is* the disp.
		if op.Disp == "" {
			return "", "", fmt.Errorf("PC-relative without disp not yet lowered")
		}
		e.Lf("la %s, %s", ScrEA, op.Disp)
		e.Lf("load%s %s, (%s)", sz, scratch, ScrEA)
		return scratch, "", nil
	case AMIndexPC:
		if op.Disp == "" {
			return "", "", fmt.Errorf("indexed PC-rel without disp not yet lowered")
		}
		e.Lf("la %s, %s", ScrEA, op.Disp)
		c.emitIndexCombine(e, op.Index, ScrEA)
		e.Lf("load%s %s, (%s)", sz, scratch, ScrEA)
		return scratch, "", nil
	}
	return "", "", fmt.Errorf("loadValue: unsupported mode %v", op.Mode)
}

// storeValue writes the value held in `srcReg` (assumed already masked or
// truncated by the producing op at width `size`) into `op`. For Dn at .b/.w
// width, emits the partial-update merge so upper bits of the host IE64 reg
// are preserved (m68k semantics).
func (c *Converter) storeValue(e *Emit, op Operand, size int, srcReg string) error {
	sz := IE64Size(size)
	switch op.Mode {
	case AMDataReg:
		if size == 4 {
			// Full 32-bit write — IE64 ALU/move with .l masks to 32 bits, so
			// this is correct for the m68k contract (Dn is 32-bit; upper
			// IE64 bits stay zero).
			if srcReg != op.Reg.IE64 {
				e.Lf("move.l %s, %s", op.Reg.IE64, srcReg)
			}
			return nil
		}
		// Partial update: keep upper bits of dst, replace low `size` bytes.
		// dst = (dst & ~mask) | (src & mask)
		e.Lf("and.l %s, %s, #%s", ScrAux, srcReg, SizeMask(size))
		e.Lf("and.q %s, %s, #%s", op.Reg.IE64, op.Reg.IE64, SizeInvMask(size))
		e.Lf("or.q %s, %s, %s", op.Reg.IE64, op.Reg.IE64, ScrAux)
		return nil
	case AMAddrReg:
		// MOVEA / ADDA / SUBA semantics: writes are sign-extended .w → .l.
		// .b form does NOT exist on m68k (MOVEA, ADDA, SUBA are .w/.l only);
		// callers normalize size to 1/2/4 so we only branch on those.
		if size == 1 {
			return fmt.Errorf("byte-sized write to An is illegal m68k")
		}
		if size == 2 {
			e.Lf("sext.w %s, %s", op.Reg.IE64, srcReg)
			return nil
		}
		if srcReg != op.Reg.IE64 {
			e.Lf("move.l %s, %s", op.Reg.IE64, srcReg)
		}
		return nil
	case AMIndirect:
		e.Lf("store%s %s, (%s)", sz, srcReg, op.Reg.IE64)
		return nil
	case AMPostInc:
		e.Lf("store%s %s, (%s)", sz, srcReg, op.Reg.IE64)
		e.Lf("add.l %s, %s, #%d", op.Reg.IE64, op.Reg.IE64, postIncStep(op.Reg, size))
		return nil
	case AMPreDec:
		e.Lf("sub.l %s, %s, #%d", op.Reg.IE64, op.Reg.IE64, postIncStep(op.Reg, size))
		e.Lf("store%s %s, (%s)", sz, srcReg, op.Reg.IE64)
		return nil
	case AMDispAn:
		e.Lf("store%s %s, %s(%s)", sz, srcReg, dispOrZero(op.Disp), op.Reg.IE64)
		return nil
	case AMIndexAn:
		c.emitIndexAddr(e, op, ScrEA)
		e.Lf("store%s %s, (%s)", sz, srcReg, ScrEA)
		return nil
	case AMAbsW, AMAbsL:
		e.Lf("la %s, %s", ScrEA, op.Disp)
		maybeSignExtAbsW(e, ScrEA, op.Mode)
		e.Lf("store%s %s, (%s)", sz, srcReg, ScrEA)
		return nil
	}
	return fmt.Errorf("storeValue: unsupported mode %v", op.Mode)
}

// emitIndexAddr puts the effective address of an AMIndexAn into `dst`.
// (d8,An,Xn.size*scale) -> dst := An + (Xn[.size] << log2(scale)) + d8
func (c *Converter) emitIndexAddr(e *Emit, op Operand, dst string) {
	idx := op.Index
	idxReg := idx.Reg.IE64
	// Width-normalize Xn.
	if idx.Size == "w" {
		e.Lf("sext.w %s, %s", dst, idxReg)
	} else {
		e.Lf("move.l %s, %s", dst, idxReg)
	}
	if idx.Scale > 1 {
		log2 := bits.TrailingZeros(uint(idx.Scale))
		e.Lf("lsl.l %s, %s, #%d", dst, dst, log2)
	}
	e.Lf("add.l %s, %s, %s", dst, dst, op.Reg.IE64)
	if op.Disp != "" {
		e.Lf("add.l %s, %s, #%s", dst, dst, op.Disp)
	}
}

// emitIndexCombine adds (Xn[.size] << log2(scale)) to an already-loaded base
// address register (used for PC-rel indexed where `la` provides the base).
func (c *Converter) emitIndexCombine(e *Emit, idx IndexSpec, baseReg string) {
	if idx.Size == "w" {
		e.Lf("sext.w %s, %s", ScrAux, idx.Reg.IE64)
	} else {
		e.Lf("move.l %s, %s", ScrAux, idx.Reg.IE64)
	}
	if idx.Scale > 1 {
		log2 := bits.TrailingZeros(uint(idx.Scale))
		e.Lf("lsl.l %s, %s, #%d", ScrAux, ScrAux, log2)
	}
	e.Lf("add.l %s, %s, %s", baseReg, baseReg, ScrAux)
}

// dispOrZero returns "0" for empty disp text so we always emit a syntactically
// valid `disp(reg)` in IE64.
//
//nolint:unused
func dispOrZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// rmwHandle threads a memory destination through a read-modify-write so the
// autoincrement/predecrement side-effect on the address register fires
// exactly once. Without this, naive `loadValue(dst) + storeValue(dst)` on
// `(An)+` or `-(An)` increments/decrements An twice and stores at the wrong
// address.
type rmwHandle struct {
	op      Operand // original dst operand
	size    int
	szIE    string
	mode    AddrMode
	rA      string // address register for autoinc/predec
	eaReg   string // ScrEA holds snapshot of EA
	valReg  string // register that holds the loaded dst value
	autoinc bool
	predec  bool
}

// loadDstRMW emits the dst-load portion of a read-modify-write. For
// autoincrement/predecrement modes it snapshots the EA into ScrEA and does
// the load through it, leaving the address register untouched (autoinc) or
// already decremented (predec). The companion storeDstRMW finishes the
// pattern with the (single) side-effect.
func (c *Converter) loadDstRMW(e *Emit, dst Operand, size int) (rmwHandle, error) {
	h := rmwHandle{op: dst, size: size, szIE: IE64Size(size), mode: dst.Mode}
	switch dst.Mode {
	case AMPostInc:
		h.autoinc = true
		h.rA = dst.Reg.IE64
		e.Lf("move.l %s, %s", ScrEA, h.rA)
		e.Lf("load%s %s, (%s)", h.szIE, ScrV2, ScrEA)
		h.eaReg = ScrEA
		h.valReg = ScrV2
		return h, nil
	case AMPreDec:
		h.predec = true
		h.rA = dst.Reg.IE64
		e.Lf("sub.l %s, %s, #%d", h.rA, h.rA, postIncStep(dst.Reg, size))
		e.Lf("move.l %s, %s", ScrEA, h.rA)
		e.Lf("load%s %s, (%s)", h.szIE, ScrV2, ScrEA)
		h.eaReg = ScrEA
		h.valReg = ScrV2
		return h, nil
	}
	// All other modes: defer to plain loadValue / storeValue (no
	// double-side-effect risk).
	r, _, err := c.loadValue(e, dst, size, ScrV2)
	if err != nil {
		return h, err
	}
	h.valReg = r
	return h, nil
}

// storeDstRMW emits the dst-store portion of a read-modify-write, using the
// snapshot in h.eaReg for autoinc/predec modes and applying the autoinc
// side-effect exactly once.
func (c *Converter) storeDstRMW(e *Emit, h rmwHandle, srcReg string) error {
	if h.autoinc {
		e.Lf("store%s %s, (%s)", h.szIE, srcReg, h.eaReg)
		e.Lf("add.l %s, %s, #%d", h.rA, h.rA, postIncStep(h.op.Reg, h.size))
		return nil
	}
	if h.predec {
		e.Lf("store%s %s, (%s)", h.szIE, srcReg, h.eaReg)
		return nil
	}
	return c.storeValue(e, h.op, h.size, srcReg)
}

// postIncStep returns the size-in-bytes step for (An)+ / -(An). For SP (a7),
// .b uses 2 bytes (m68k stack alignment), all others use the literal width.
func postIncStep(reg MappedReg, size int) int {
	if reg.IsStack && size == 1 {
		return 2
	}
	return size
}

// =====================================================================
// Per-mnemonic lowerings
// =====================================================================

// emitMove handles MOVE / MOVEA / MOVEQ.
//
// MOVE.X src,dst                : load src@X, store dst@X (with partial-update for Dn .b/.w)
// MOVEA.X src,An (X in .w/.l)   : load src, sign-extend if .w, store full-width to An
// MOVEQ #imm,Dn                 : sign-extended 8-bit imm into low 32 bits of Dn (full .l write)
func (c *Converter) emitMove(e *Emit, l Line, size int) error {
	if l.Mnemonic == "moveq" {
		if len(l.Operands) != 2 {
			return fmt.Errorf("moveq requires 2 operands")
		}
		src, err := ParseOperand(l.Operands[0])
		if err != nil {
			return err
		}
		dst, err := ParseOperand(l.Operands[1])
		if err != nil {
			return err
		}
		if src.Mode != AMImmediate || dst.Mode != AMDataReg {
			return fmt.Errorf("moveq operands must be #imm,Dn")
		}
		e.Lf("moveq %s, #%s", dst.Reg.IE64, src.Imm)
		// CCR: N/Z from sign-extended value; C=V=0.
		c.emitShadowsForLogical(e, dst.Reg.IE64, 4)
		return nil
	}
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s requires 2 operands", l.Mnemonic)
	}
	src, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	dst, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	if l.Mnemonic == "movea" {
		// MOVEA.w sign-extends; MOVEA.l preserves; dst must be An.
		if dst.Mode != AMAddrReg {
			return fmt.Errorf("movea destination must be An, got %v", dst.Mode)
		}
		if size != 2 && size != 4 {
			return fmt.Errorf("movea size must be .w or .l")
		}
	}
	// Fast path: src reg → dst reg, both Dn or An, size .l → single move.
	if size == 4 && (src.Mode == AMDataReg || src.Mode == AMAddrReg) &&
		(dst.Mode == AMDataReg || dst.Mode == AMAddrReg) {
		if src.Reg.IE64 != dst.Reg.IE64 {
			e.Lf("move.l %s, %s", dst.Reg.IE64, src.Reg.IE64)
		}
		// MOVEA does not affect CCR; MOVE does.
		if l.Mnemonic == "move" {
			c.emitShadowsForLogical(e, dst.Reg.IE64, 4)
		}
		return nil
	}
	// General path: load src into ScrV1 (or take immediate), then store.
	srcReg, srcImm, err := c.loadValue(e, src, size, ScrV1)
	if err != nil {
		return err
	}
	if srcImm != "" {
		e.Lf("move%s %s, #%s", IE64Size(size), ScrV1, srcImm)
		srcReg = ScrV1
	}
	if err := c.storeValue(e, dst, size, srcReg); err != nil {
		return err
	}
	if l.Mnemonic == "move" {
		c.emitShadowsForLogical(e, srcReg, size)
	}
	return nil
}

// emitLea handles LEA src,An — compute effective address of src, write to An.
func (c *Converter) emitLea(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("lea requires 2 operands")
	}
	src, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	dst, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	if dst.Mode != AMAddrReg {
		return fmt.Errorf("lea destination must be An")
	}
	switch src.Mode {
	case AMIndirect:
		e.Lf("move.l %s, %s", dst.Reg.IE64, src.Reg.IE64)
		return nil
	case AMDispAn:
		e.Lf("lea %s, %s(%s)", dst.Reg.IE64, dispOrZero(src.Disp), src.Reg.IE64)
		return nil
	case AMIndexAn:
		c.emitIndexAddr(e, src, dst.Reg.IE64)
		return nil
	case AMAbsW, AMAbsL:
		e.Lf("la %s, %s", dst.Reg.IE64, src.Disp)
		maybeSignExtAbsW(e, dst.Reg.IE64, src.Mode)
		return nil
	case AMDispPC, AMIndexPC:
		// la resolves PC-rel labels.
		e.Lf("la %s, %s", dst.Reg.IE64, src.Disp)
		if src.Mode == AMIndexPC {
			c.emitIndexCombine(e, src.Index, dst.Reg.IE64)
		}
		return nil
	}
	return fmt.Errorf("lea: unsupported source mode %v", src.Mode)
}

// emitArith handles binary ALU (ADD/SUB/AND/OR/EOR) at width `size`.
//
//	op.X src, dst
//
// dst is the m68k destination (read & written). src may be reg / imm / mem.
// For dst==Dn at .l: emit `op.l rd, rd, src` directly. For dst==Dn at .b/.w:
// emit width-masked op into ScrV2, then partial-update merge into Dn. For
// dst==An: only ADDA/SUBA make sense; size=.w sign-extends src.
//
// For dst in memory (indirect / displacement / absolute): load → op → store.
func (c *Converter) emitArith(e *Emit, l Line, size int, ie64op string) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s requires 2 operands", l.Mnemonic)
	}
	src, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	dst, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}

	// Resolve src to a (reg, imm) pair.
	srcReg, srcImm, err := c.loadValue(e, src, size, ScrV1)
	if err != nil {
		return err
	}
	// Divide-by-zero guard: m68k DIVU/DIVS trap on zero divisor (vector 5).
	if l.Mnemonic == "divu" || l.Mnemonic == "divs" {
		guarded := c.emitDivZeroGuard(e, srcReg, srcImm)
		if guarded == "" {
			// Static-zero divisor: trap emitted, skip divide.
			return nil
		}
		srcReg = guarded
		srcImm = ""
	}

	// ADDA / SUBA — destination is An. .w forms must sign-extend the source
	// word (regardless of register or immediate origin) before the .l add;
	// ie64asm `add.l rd, rd, #imm` treats `#$FFFF` as +65535, but m68k
	// ADDA.W #$FFFF must add -1.  ADDA/SUBA do not affect CCR, so no shadow
	// updates are emitted here.
	if dst.Mode == AMAddrReg {
		if size == 2 {
			// Materialise + sign-extend the word source into ScrV1.
			if srcImm != "" {
				e.Lf("move.w %s, #%s", ScrV1, srcImm)
				e.Lf("sext.w %s, %s", ScrV1, ScrV1)
			} else {
				e.Lf("sext.w %s, %s", ScrV1, srcReg)
			}
			e.Lf("%s.l %s, %s, %s", ie64op, dst.Reg.IE64, dst.Reg.IE64, ScrV1)
			return nil
		}
		if srcImm != "" {
			e.Lf("%s.l %s, %s, #%s", ie64op, dst.Reg.IE64, dst.Reg.IE64, srcImm)
			return nil
		}
		e.Lf("%s.l %s, %s, %s", ie64op, dst.Reg.IE64, dst.Reg.IE64, srcReg)
		return nil
	}

	// Materialise an immediate src into ScrV1 once if we'll need it for
	// shadow C/V (which require a register operand).
	needRegSrc := shadowWantsCV(l.Mnemonic) && srcImm != ""
	if needRegSrc {
		e.Lf("move%s %s, #%s", IE64Size(size), ScrV1, srcImm)
		srcReg = ScrV1
		srcImm = ""
	}

	// dst == Dn
	if dst.Mode == AMDataReg {
		if size == 4 {
			// Snapshot pre-op dst for shadow C/V.
			if shadowWantsCV(l.Mnemonic) {
				e.Lf("move.l %s, %s", ShadowSnap, dst.Reg.IE64)
			}
			if srcImm != "" {
				e.Lf("%s.l %s, %s, #%s", ie64op, dst.Reg.IE64, dst.Reg.IE64, srcImm)
			} else {
				e.Lf("%s.l %s, %s, %s", ie64op, dst.Reg.IE64, dst.Reg.IE64, srcReg)
			}
			c.emitArithShadows(e, l.Mnemonic, dst.Reg.IE64, ShadowSnap, srcReg, size)
			return nil
		}
		// .b/.w into Dn — partial update.
		szIE := IE64Size(size)
		// Snapshot pre-op dst masked to width for shadow C/V.
		if shadowWantsCV(l.Mnemonic) {
			e.Lf("and.l %s, %s, #%s", ShadowSnap, dst.Reg.IE64, SizeMask(size))
		}
		e.Lf("and.l %s, %s, #%s", ScrV2, dst.Reg.IE64, SizeMask(size))
		if srcImm != "" {
			e.Lf("%s%s %s, %s, #%s", ie64op, szIE, ScrV2, ScrV2, srcImm)
		} else {
			e.Lf("%s%s %s, %s, %s", ie64op, szIE, ScrV2, ScrV2, srcReg)
		}
		e.Lf("and.l %s, %s, #%s", ScrV2, ScrV2, SizeMask(size))
		// Shadows BEFORE the merge so the result is still in ScrV2 narrow.
		c.emitArithShadows(e, l.Mnemonic, ScrV2, ShadowSnap, srcReg, size)
		// Merge into the data register.
		e.Lf("and.q %s, %s, #%s", dst.Reg.IE64, dst.Reg.IE64, SizeInvMask(size))
		e.Lf("or.q %s, %s, %s", dst.Reg.IE64, dst.Reg.IE64, ScrV2)
		return nil
	}

	// dst is memory. Read-modify-write via rmwHandle so autoinc/predec on
	// `(An)+` / `-(An)` fires exactly once.
	h, err := c.loadDstRMW(e, dst, size)
	if err != nil {
		return err
	}
	szIE := IE64Size(size)
	// Snapshot pre-op dst for shadow C/V.
	if shadowWantsCV(l.Mnemonic) {
		e.Lf("move.l %s, %s", ShadowSnap, h.valReg)
	}
	if srcImm != "" {
		e.Lf("%s%s %s, %s, #%s", ie64op, szIE, ScrV2, h.valReg, srcImm)
	} else {
		e.Lf("%s%s %s, %s, %s", ie64op, szIE, ScrV2, h.valReg, srcReg)
	}
	c.emitArithShadows(e, l.Mnemonic, ScrV2, ShadowSnap, srcReg, size)
	return c.storeDstRMW(e, h, ScrV2)
}

// emitDivZeroGuard inserts a zero-check on `divisorReg` (or imm value when
// `divisorImm` is non-empty). On zero divisor, m68k DIVU/DIVS traps via
// vector 5; we emit `syscall #16` (relocated from #5 to keep TRAP #0..#15
// disjoint from m68k exception vectors per the locked syscall # table in
// sdk/docs/m68Kto64.md §11) and skip the divide. Returns the divisor
// register name to use in the actual divide (always a register; immediates
// get materialised into ScrV1).
func (c *Converter) emitDivZeroGuard(e *Emit, divisorReg, divisorImm string) string {
	if divisorImm != "" {
		// Immediate divisors are constant-foldable. Static-zero → unconditional
		// trap; static-nonzero → no guard needed.
		if strings.TrimSpace(divisorImm) == "0" {
			e.L("syscall #16 ; div-by-zero (m68k vector 5, relocated)")
			return ""
		}
		e.Lf("move.l %s, #%s", ScrV1, divisorImm)
		return ScrV1
	}
	skip := e.NewLabel("div_ok")
	e.Lf("bnez %s, %s", divisorReg, skip)
	e.L("syscall #16") // m68k vector 5 (zero-divide), relocated
	e.Label(skip)
	return divisorReg
}

// emitMulW handles MULU.W / MULS.W src,Dn — 16×16→32, full 32-bit Dn write.
//
// MULU.W: zero-extend src and Dn low 16 to 32 bits, then 32-bit unsigned
// multiply, store full 32-bit result into Dn (upper 32 of IE64 reg cleared
// by .l ALU semantics).
//
// MULS.W: sign-extend instead.
func (c *Converter) emitMulW(e *Emit, l Line, signed bool) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s.w requires 2 operands", l.Mnemonic)
	}
	src, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	dst, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	if dst.Mode != AMDataReg {
		return fmt.Errorf("%s.w destination must be Dn", l.Mnemonic)
	}
	rd := dst.Reg.IE64
	srcReg, srcImm, err := c.loadValue(e, src, 2, ScrV1)
	if err != nil {
		return err
	}
	if srcImm != "" {
		e.Lf("move.w %s, #%s", ScrV1, srcImm)
		srcReg = ScrV1
	}
	if signed {
		e.Lf("sext.w %s, %s", ScrV1, srcReg)
		e.Lf("sext.w %s, %s", ScrV2, rd)
		e.Lf("muls.l %s, %s, %s", rd, ScrV2, ScrV1)
	} else {
		e.Lf("and.l %s, %s, #$FFFF", ScrV1, srcReg)
		e.Lf("and.l %s, %s, #$FFFF", ScrV2, rd)
		e.Lf("mulu.l %s, %s, %s", rd, ScrV2, ScrV1)
	}
	c.emitShadowsForLogical(e, rd, 4)
	return nil
}

// emitDivW handles DIVU.W / DIVS.W src,Dn — 32÷16, packs Dn = (rem<<16)|quo.
//
// m68k semantics:
//
//	DIVU.W: Dn (32-bit unsigned) / src (16-bit unsigned) -> 16-bit quotient
//	        in low word, 16-bit remainder in high word.
//	DIVS.W: signed equivalent.
//
// IE64 lowering: divu.l and mod.l on full 32-bit Dn against a width-correct
// src, then mask both results to 16 and pack.
func (c *Converter) emitDivW(e *Emit, l Line, signed bool) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s.w requires 2 operands", l.Mnemonic)
	}
	src, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	dst, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	if dst.Mode != AMDataReg {
		return fmt.Errorf("%s.w destination must be Dn", l.Mnemonic)
	}
	rd := dst.Reg.IE64
	srcReg, srcImm, err := c.loadValue(e, src, 2, ScrV1)
	if err != nil {
		return err
	}
	if srcImm != "" {
		e.Lf("move.w %s, #%s", ScrV1, srcImm)
		srcReg = ScrV1
	}
	// Divide-by-zero guard. emitDivZeroGuard with empty divisorImm always
	// returns the divisor register (the static-zero branch fires only for
	// constant-zero immediates, which we materialise above).
	srcReg = c.emitDivZeroGuard(e, srcReg, "")
	// Width-correct src into ScrV1.
	if signed {
		e.Lf("sext.w %s, %s", ScrV1, srcReg)
	} else {
		e.Lf("and.l %s, %s, #$FFFF", ScrV1, srcReg)
	}
	// Quotient → ScrAux, remainder → ScrV2.
	if signed {
		// Sign-aware: divs/mods on 32-bit Dn (interpreted signed).
		e.Lf("divs.l %s, %s, %s", ScrAux, rd, ScrV1)
		e.Lf("mods.l %s, %s, %s", ScrV2, rd, ScrV1)
	} else {
		e.Lf("divu.l %s, %s, %s", ScrAux, rd, ScrV1)
		e.Lf("mod.l %s, %s, %s", ScrV2, rd, ScrV1)
	}
	// Mask each to 16 bits.
	e.Lf("and.l %s, %s, #$FFFF", ScrAux, ScrAux)
	e.Lf("and.l %s, %s, #$FFFF", ScrV2, ScrV2)
	// Pack: rd = (rem << 16) | quo.
	e.Lf("lsl.l %s, %s, #16", ScrV2, ScrV2)
	e.Lf("or.l %s, %s, %s", rd, ScrAux, ScrV2)
	c.emitShadowsForLogical(e, rd, 4)
	return nil
}

// emitUnary handles NEG / NOT (one operand: dst).
func (c *Converter) emitUnary(e *Emit, l Line, size int, ie64op string) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("%s requires 1 operand", l.Mnemonic)
	}
	dst, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if dst.Mode == AMDataReg && size == 4 {
		if ie64op == "neg" {
			e.Lf("move.l %s, %s", ShadowSnap, dst.Reg.IE64)
		}
		e.Lf("%s.l %s, %s", ie64op, dst.Reg.IE64, dst.Reg.IE64)
		c.emitUnaryShadows(e, ie64op, dst.Reg.IE64, ShadowSnap, size)
		return nil
	}
	h, err := c.loadDstRMW(e, dst, size)
	if err != nil {
		return err
	}
	szIE := IE64Size(size)
	if ie64op == "neg" {
		e.Lf("move.l %s, %s", ShadowSnap, h.valReg)
	}
	e.Lf("%s%s %s, %s", ie64op, szIE, ScrV2, h.valReg)
	c.emitUnaryShadows(e, ie64op, ScrV2, ShadowSnap, size)
	return c.storeDstRMW(e, h, ScrV2)
}

// emitUnaryShadows handles shadows for NEG/NOT.
func (c *Converter) emitUnaryShadows(e *Emit, ie64op, resultReg, preDst string, size int) {
	switch ie64op {
	case "neg":
		c.emitShadowNZFromReg(e, resultReg, size)
		c.emitShadowSubCV(e, "r0", preDst, size)
		c.emitShadowCopyToX(e) // NEG sets X := C.
	case "not":
		c.emitShadowsForLogical(e, resultReg, size) // X unchanged.
	}
}

// emitClr writes 0 to dst at width `size`.
// CCR semantics: N=0, Z=1, C=0, V=0.
func (c *Converter) emitClr(e *Emit, l Line, size int) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("clr requires 1 operand")
	}
	dst, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if dst.Mode == AMAddrReg {
		// CLR.X An is not legal m68k. Reject in strict mode; non-strict emits
		// a diagnostic comment and does NOT touch An (a `move r9, r0` would
		// silently zero the register and break code that reaches here).
		if c.strict {
			return fmt.Errorf("clr: An destination not allowed by m68k")
		}
		e.Lf("; m68kto64: clr.%s %s skipped (CLR on An is illegal m68k)",
			IE64Size(size), dst.Reg.Name)
		return nil
	}
	if dst.Mode == AMDataReg {
		if size == 4 {
			e.Lf("move.l %s, #0", dst.Reg.IE64)
		} else {
			e.Lf("and.q %s, %s, #%s", dst.Reg.IE64, dst.Reg.IE64, SizeInvMask(size))
		}
	} else {
		if err := c.storeValue(e, dst, size, "r0"); err != nil {
			return err
		}
	}
	// Shadows: result is 0. N=0, Z=1, C=0, V=0.
	e.Lf("move.l %s, #0", ShadowN)
	e.Lf("move.l %s, #0", ShadowZ)
	c.emitShadowClearC(e)
	c.emitShadowClearV(e)
	return nil
}

// emitCmpShadow lowers a standalone CMP/CMPI/CMPA (no immediately-following
// Bcc fuse), writing all four shadow flags from result = dst - src.
func (c *Converter) emitCmpShadow(e *Emit, l Line, size int) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s requires 2 operands", l.Mnemonic)
	}
	src, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	dst, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	srcReg, srcImm, err := c.loadValue(e, src, size, ScrV1)
	if err != nil {
		return err
	}
	if srcImm != "" {
		e.Lf("move%s %s, #%s", IE64Size(size), ScrV1, srcImm)
		srcReg = ScrV1
	}
	// Load dst into ScrV2 (or use direct reg). For An with CMPA at .w,
	// promote: m68k CMPA sign-extends src to .l, dst is full An.
	dstReg, _, err := c.loadValue(e, dst, size, ScrV2)
	if err != nil {
		return err
	}
	if l.Mnemonic == "cmpa" && size == 2 {
		// Sign-extend src to .l, compare at .l.
		e.Lf("sext.w %s, %s", ScrV1, srcReg)
		srcReg = ScrV1
		size = 4
	}
	// Compute result = dst - src into ShadowSnap (used as result holder here).
	szIE := IE64Size(size)
	e.Lf("sub%s %s, %s, %s", szIE, ShadowSnap, dstReg, srcReg)
	c.emitShadowNZFromReg(e, ShadowSnap, size)
	c.emitShadowSubCV(e, dstReg, srcReg, size)
	return nil
}

// emitTstShadow lowers a standalone TST, writing N/Z from operand and
// clearing C/V.
func (c *Converter) emitTstShadow(e *Emit, l Line, size int) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("tst requires 1 operand")
	}
	dst, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	dstReg, _, err := c.loadValue(e, dst, size, ScrV2)
	if err != nil {
		return err
	}
	c.emitShadowsForLogical(e, dstReg, size)
	return nil
}

// emitShift handles LSL/LSR/ASL/ASR/ROL/ROR.
//
// Forms accepted:
//
//	shift.X #count, Dn
//	shift.X Dm, Dn      (count in low 6 bits of Dm)
//	shift.X dst         (single-operand mem form, count = 1 — m68k legacy)
//
// IE64 has lsl/lsr/asr/rol/ror at .b/.w/.l/.q widths and supports both
// register and immediate shift counts.
func (c *Converter) emitShift(e *Emit, l Line, size int, ie64op, m68kMnem string) error {
	switch len(l.Operands) {
	case 1:
		// shift.X dst — count = 1.
		dst, err := ParseOperand(l.Operands[0])
		if err != nil {
			return err
		}
		return c.emitShiftRMW(e, dst, size, ie64op, m68kMnem, "", "1")
	case 2:
		count, err := ParseOperand(l.Operands[0])
		if err != nil {
			return err
		}
		dst, err := ParseOperand(l.Operands[1])
		if err != nil {
			return err
		}
		var countReg, countImm string
		switch count.Mode {
		case AMImmediate:
			countImm = count.Imm
		case AMDataReg:
			// m68k uses low 6 bits; emit AND mask before shift.
			e.Lf("and.l %s, %s, #63", ScrV1, count.Reg.IE64)
			countReg = ScrV1
		default:
			return fmt.Errorf("shift count must be #imm or Dn")
		}
		return c.emitShiftRMW(e, dst, size, ie64op, m68kMnem, countReg, countImm)
	}
	return fmt.Errorf("%s requires 1 or 2 operands", l.Mnemonic)
}

func (c *Converter) emitShiftRMW(e *Emit, dst Operand, size int, ie64op, m68kMnem, countReg, countImm string) error {
	szIE := IE64Size(size)
	if dst.Mode == AMDataReg && size == 4 {
		// Capture pre-op for shift-C computation.
		c.emitShiftCPre(e, dst.Reg.IE64, size, ie64op, m68kMnem, countReg, countImm)
		if countImm != "" {
			e.Lf("%s.l %s, %s, #%s", ie64op, dst.Reg.IE64, dst.Reg.IE64, countImm)
		} else {
			e.Lf("%s.l %s, %s, %s", ie64op, dst.Reg.IE64, dst.Reg.IE64, countReg)
		}
		// Shadow N/Z from result. V was set by emitShiftCPre for ASL; for
		// every other shift m68k clears V.
		c.emitShadowNZFromReg(e, dst.Reg.IE64, size)
		if m68kMnem != "asl" {
			c.emitShadowClearV(e)
		}
		return nil
	}
	h, err := c.loadDstRMW(e, dst, size)
	if err != nil {
		return err
	}
	c.emitShiftCPre(e, h.valReg, size, ie64op, m68kMnem, countReg, countImm)
	if countImm != "" {
		e.Lf("%s%s %s, %s, #%s", ie64op, szIE, ScrV2, h.valReg, countImm)
	} else {
		e.Lf("%s%s %s, %s, %s", ie64op, szIE, ScrV2, h.valReg, countReg)
	}
	c.emitShadowNZFromReg(e, ScrV2, size)
	if m68kMnem != "asl" {
		c.emitShadowClearV(e)
	}
	return c.storeDstRMW(e, h, ScrV2)
}

// emitShiftCPre computes shadow C for a shift/rotate at width `size`. The
// m68k semantic is C = last bit shifted out (or for ROL/ROR the bit rotated
// into the carry-equivalent position). Captures C BEFORE the destructive
// shift writes.
//
// LSL/ASL: C = bit (W - count) of pre-shift value.
// LSR/ASR: C = bit (count - 1) of pre-shift value.
// ROL:     C = bit (W - count) of pre-shift value.
// ROR:     C = bit (count - 1) of pre-shift value.
//
// For count = 0, m68k clears C. For count > W, behaviour is implementation
// defined; we clamp count modulo W (low 6 bits like the shifter).
func (c *Converter) emitShiftCPre(e *Emit, srcReg string, size int, ie64op, m68kMnem, countReg, countImm string) {
	width := size * 8
	// Materialise count into ShadowTmp1 (clamped).
	if countImm != "" {
		e.Lf("move.l %s, #%s", ShadowTmp1, countImm)
	} else {
		e.Lf("move.l %s, %s", ShadowTmp1, countReg)
	}
	e.Lf("and.l %s, %s, #63", ShadowTmp1, ShadowTmp1)
	// If count == 0 → C := 0 (handled implicitly because shift below
	// won't execute meaningfully — but m68k explicitly zeroes C). Use a
	// branch to skip the C computation if count == 0.
	zero := e.NewLabel("shft_c_zero")
	done := e.NewLabel("shft_c_done")
	e.Lf("beqz %s, %s", ShadowTmp1, zero)
	if ie64op == "lsl" || ie64op == "rol" {
		// C = bit (width - count) of src.
		e.Lf("move.l %s, #%d", ShadowTmp2, width)
		e.Lf("sub.l %s, %s, %s", ShadowTmp2, ShadowTmp2, ShadowTmp1)
		e.Lf("lsr.q %s, %s, %s", ShadowC, srcReg, ShadowTmp2)
		e.Lf("and.q %s, %s, #1", ShadowC, ShadowC)
	} else {
		// LSR / ASR / ROR: C = bit (count - 1) of src.
		e.Lf("sub.l %s, %s, #1", ShadowTmp1, ShadowTmp1)
		e.Lf("lsr.q %s, %s, %s", ShadowC, srcReg, ShadowTmp1)
		e.Lf("and.q %s, %s, #1", ShadowC, ShadowC)
	}
	// Mirror C into X (m68k: shifts with count > 0 update X := C; ROL/ROR
	// leave X unchanged, but skipping that guarantee here is benign for the
	// typical guest programs the transpiler targets).
	if ie64op != "rol" && ie64op != "ror" {
		e.Lf("move.l %s, %s", ShadowX, ShadowC)
	}
	// ASL V: m68k sets V if any bit of the destination changes value during
	// the shift. Equivalent: top (count+1) bits of pre-shift src must all be
	// the same (all 0 or all 1). top := (src >> (width-1-count)) masked to
	// (count+1) bits. V=0 iff top==0 OR top==mask.
	if m68kMnem == "asl" {
		vDone := e.NewLabel("asl_v_done")
		vSet := e.NewLabel("asl_v_set")
		// bitOffset = (width - 1) - count → ShadowTmp2.
		e.Lf("move.l %s, #%d", ShadowTmp2, width-1)
		e.Lf("sub.l %s, %s, %s", ShadowTmp2, ShadowTmp2, ShadowTmp1)
		e.Lf("lsr.q %s, %s, %s", ShadowV, srcReg, ShadowTmp2)
		// mask = (1 << (count+1)) - 1 → ShadowSnap.
		e.Lf("move.l %s, #1", ShadowSnap)
		e.Lf("move.l %s, %s", ShadowTmp2, ShadowTmp1)
		e.Lf("add.l %s, %s, #1", ShadowTmp2, ShadowTmp2)
		e.Lf("lsl.q %s, %s, %s", ShadowSnap, ShadowSnap, ShadowTmp2)
		e.Lf("sub.l %s, %s, #1", ShadowSnap, ShadowSnap)
		e.Lf("and.q %s, %s, %s", ShadowV, ShadowV, ShadowSnap)
		// V=0 if ShadowV == 0 OR ShadowV == mask.
		e.Lf("beqz %s, %s", ShadowV, vSet) // top==0 → all-same → V=0; via skip path
		e.Lf("eor.q %s, %s, %s", ShadowSnap, ShadowV, ShadowSnap)
		e.Lf("beqz %s, %s", ShadowSnap, vSet) // top == mask → all-1 → V=0
		// Otherwise V := 1.
		e.Lf("move.l %s, #1", ShadowV)
		e.Lf("bra %s", vDone)
		e.Label(vSet)
		e.Lf("move.l %s, #0", ShadowV)
		e.Label(vDone)
	}
	e.Lf("bra %s", done)
	e.Label(zero)
	// count == 0:
	//   LSL/LSR/ASL/ASR: m68k says C := 0, X UNCHANGED, V := 0 (ASL).
	//   ROL/ROR:         m68k says C UNCHANGED, X UNCHANGED, V := 0.
	//   ROXL/ROXR:       m68k says C := X, X UNCHANGED; handled by emitRox,
	//                    not reached here.
	// We unconditionally write C := 0 for all shift/rotate mnemonics that
	// route through emitShiftCPre. Correct for LSL/LSR/ASL/ASR; deviation
	// for ROL/ROR (documented in sdk/docs/m68Kto64.md §12 "Rotate count=0
	// C-flag deviation").
	e.Lf("move.l %s, #0", ShadowC)
	if m68kMnem == "asl" {
		e.Lf("move.l %s, #0", ShadowV)
	}
	e.Label(done)
}

// emitExt handles EXT.W (sign-extend byte→word) / EXT.L (word→long) / EXTB.L
// (68020+ byte→long).
func (c *Converter) emitExt(e *Emit, l Line, size int, byteToLong bool) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("ext requires 1 operand")
	}
	dst, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if dst.Mode != AMDataReg {
		return fmt.Errorf("ext destination must be Dn")
	}
	rd := dst.Reg.IE64
	if byteToLong {
		// EXTB.L Dn — sign-extend low byte to 32 bits.
		e.Lf("sext.b %s, %s", rd, rd)
		e.Lf("and.l %s, %s, #%s", rd, rd, SizeMask(4))
		c.emitShadowsForLogical(e, rd, 4)
		return nil
	}
	switch size {
	case 2: // EXT.W : sign-extend low byte to 16 bits, preserve upper 16.
		e.Lf("sext.b %s, %s", ScrV1, rd)
		e.Lf("and.l %s, %s, #$FFFF", ScrV1, ScrV1)
		e.Lf("and.q %s, %s, #%s", rd, rd, SizeInvMask(2))
		e.Lf("or.q %s, %s, %s", rd, rd, ScrV1)
		c.emitShadowsForLogical(e, ScrV1, 2)
	case 4: // EXT.L : sign-extend low word to 32 bits.
		e.Lf("sext.w %s, %s", rd, rd)
		e.Lf("and.l %s, %s, #%s", rd, rd, SizeMask(4))
		c.emitShadowsForLogical(e, rd, 4)
	default:
		return fmt.Errorf("ext.%s unsupported", IE64Size(size))
	}
	return nil
}
