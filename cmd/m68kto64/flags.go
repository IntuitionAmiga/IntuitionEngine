package main

import (
	"fmt"
	"strings"
)

// Phase 3: flag-fuse peephole + control-flow lowering helpers.
//
// IE64 has no flags. m68k Bcc instructions consume CCR set by the most-recent
// flag-producing op. The transpiler fuses (CMP|TST)+Bcc pairs into IE64
// register-pair branches when they are immediately adjacent. Unfused Bcc
// emits "; ERROR" diagnostics in non-strict mode (Phase 5 will add full
// shadow-CCR maintenance).

// bccKind extracts the condition suffix of a m68k Bcc / DBcc / Scc.
// Returns "" if mnem is not a conditional branch / scc.
func bccKind(mnem string) string {
	switch mnem {
	case "beq", "dbeq", "seq":
		return "eq"
	case "bne", "dbne", "sne":
		return "ne"
	case "blt", "dblt", "slt":
		return "lt"
	case "bge", "dbge", "sge":
		return "ge"
	case "bgt", "dbgt", "sgt":
		return "gt"
	case "ble", "dble", "sle":
		return "le"
	case "bhi", "dbhi", "shi":
		return "hi"
	case "bls", "dbls", "sls":
		return "ls"
	case "bcc", "dbcc", "scc":
		return "cc"
	case "bcs", "dbcs", "scs":
		return "cs"
	case "bmi", "dbmi", "smi":
		return "mi"
	case "bpl", "dbpl", "spl":
		return "pl"
	case "bvs", "dbvs", "svs":
		return "vs"
	case "bvc", "dbvc", "svc":
		return "vc"
	case "bra", "dbra", "dbf", "dbt":
		// Not flag-consuming in the classical sense; callers handle.
		return ""
	case "bsr":
		return ""
	}
	return ""
}

// isSignedBcc reports whether this Bcc kind needs sign-extension before fuse.
func isSignedBcc(k string) bool {
	switch k {
	case "lt", "ge", "gt", "le", "mi", "pl":
		return true
	}
	return false
}

// fusableProducer reports whether the lexed line is a CMP/TST that can fuse
// with an immediately-following Bcc. Only CMP/TST (no destination side-effect)
// are fused in Phase 3; ADD/SUB/AND/OR + Bcc lower as the ALU op followed by
// a fused-against-result branch (handled separately).
func fusableProducer(l Line) bool {
	if l.Kind != LineInstruction {
		return false
	}
	switch l.Mnemonic {
	case "cmp", "cmpi", "cmpa":
		return len(l.Operands) == 2
	case "tst":
		return len(l.Operands) == 1
	}
	return false
}

// canFuseBcc reports whether this is a Bcc the transpiler is willing to fuse
// in Phase 3. (Excludes BCC/BCS/BVS/BVC which need C/V flags — Phase 5.)
func canFuseBcc(l Line) bool {
	if l.Kind != LineInstruction {
		return false
	}
	k := bccKind(l.Mnemonic)
	if !strings.HasPrefix(l.Mnemonic, "b") {
		return false
	}
	switch k {
	case "eq", "ne", "lt", "ge", "gt", "le", "hi", "ls", "mi", "pl":
		return true
	}
	return false
}

// =====================================================================
// Width normalization for fuse
// =====================================================================

// fuseNormaliseValue puts a width-correct (masked or sign-extended) value of
// `op` into a scratch register and returns the register name that holds it.
//
// signed: whether the consumer Bcc is signed (needs sign-extension to 64 bits)
//
// For immediates: materialise into the scratch register.
func (c *Converter) fuseNormaliseValue(e *Emit, op Operand, size int, signed bool, scratch string) (string, error) {
	// Bare register: width-normalise into scratch.
	switch op.Mode {
	case AMDataReg, AMAddrReg:
		if size == 8 || size == 0 || size == 4 {
			// .l: IE64 reg already holds the m68k 32-bit value with upper
			// bits zero (ALU ops mask). For unsigned compare this is the
			// width-correct value. For signed .l compare on IE64's 64-bit
			// blt/bge, we'd need to sign-extend bit 31 to bit 63 — emit
			// sext.l in that case.
			if signed && size == 4 {
				e.Lf("sext.l %s, %s", scratch, op.Reg.IE64)
				return scratch, nil
			}
			return op.Reg.IE64, nil
		}
		if signed {
			e.Lf("sext%s %s, %s", IE64Size(size), scratch, op.Reg.IE64)
		} else {
			e.Lf("and.l %s, %s, #%s", scratch, op.Reg.IE64, SizeMask(size))
		}
		return scratch, nil
	case AMImmediate:
		// .l immediate doesn't need post-mask; IE64 move.l already truncates.
		if size == 4 {
			e.Lf("move.l %s, #%s", scratch, op.Imm)
			if signed {
				e.Lf("sext.l %s, %s", scratch, scratch)
			}
			return scratch, nil
		}
		e.Lf("move%s %s, #%s", IE64Size(size), scratch, op.Imm)
		if signed {
			e.Lf("sext%s %s, %s", IE64Size(size), scratch, scratch)
		}
		return scratch, nil
	}
	// Memory: load via existing helper, then normalise.
	r, _, err := c.loadValue(e, op, size, scratch)
	if err != nil {
		return "", err
	}
	if signed {
		e.Lf("sext%s %s, %s", IE64Size(size), scratch, r)
		return scratch, nil
	}
	// loadValue returns masked already (low bytes only).
	return r, nil
}

// =====================================================================
// Fused-pair emission
// =====================================================================

// emitFusedCmpBcc handles CMP/CMPI/CMPA + Bcc.
//
// m68k:   cmp.X src,dst   sets flags from (dst - src).
//         Bcc L   branches per cc on (dst CMP src).
// IE64:   bne dst, src, L  branches if dst != src.
//         For signed, sign-extend operands to 64 bits.
func (c *Converter) emitFusedCmpBcc(e *Emit, prod, cons Line) error {
	srcOp, err := ParseOperand(prod.Operands[0])
	if err != nil {
		return err
	}
	dstOp, err := ParseOperand(prod.Operands[1])
	if err != nil {
		return err
	}
	size := SizeBytes(prod.Size)
	if size == 0 {
		size = 2 // m68k default
	}
	kind := bccKind(cons.Mnemonic)
	if len(cons.Operands) != 1 {
		return fmt.Errorf("Bcc requires 1 operand")
	}
	label := strings.TrimSpace(cons.Operands[0])
	signed := isSignedBcc(kind)

	if prod.Mnemonic == "cmpa" {
		// CMPA semantics: source word is sign-extended to .l (when .w);
		// destination An participates at FULL 32-bit width — never masked
		// or sign-extended-at-word, since An is a 32-bit register and the
		// upper bits matter.
		var srcReg string
		switch {
		case size == 2 && srcOp.Mode == AMImmediate:
			e.Lf("move.w %s, #%s", ScrV1, srcOp.Imm)
			e.Lf("sext.w %s, %s", ScrV1, ScrV1)
			srcReg = ScrV1
		case size == 2:
			r, _, err := c.loadValue(e, srcOp, 2, ScrV1)
			if err != nil {
				return err
			}
			e.Lf("sext.w %s, %s", ScrV1, r)
			srcReg = ScrV1
		case srcOp.Mode == AMImmediate:
			e.Lf("move.l %s, #%s", ScrV1, srcOp.Imm)
			srcReg = ScrV1
		default:
			r, _, err := c.loadValue(e, srcOp, 4, ScrV1)
			if err != nil {
				return err
			}
			srcReg = r
		}
		var dstReg string
		if dstOp.Mode == AMAddrReg || dstOp.Mode == AMDataReg {
			dstReg = dstOp.Reg.IE64
		} else {
			// Defensive: real m68k CMPA dst must be An, but tolerate.
			r, _, err := c.loadValue(e, dstOp, 4, ScrV2)
			if err != nil {
				return err
			}
			dstReg = r
		}
		return emitBccLine(e, kind, dstReg, srcReg, label)
	}

	// fuseNormaliseValue only errors on AMUnknown; ParseOperand above
	// already produced concrete addressing modes, so both calls are
	// infallible in this flow.
	srcReg, _ := c.fuseNormaliseValue(e, srcOp, size, signed, ScrV1)
	dstReg, _ := c.fuseNormaliseValue(e, dstOp, size, signed, ScrV2)
	return emitBccLine(e, kind, dstReg, srcReg, label)
}

// emitFusedTstBcc handles TST + Bcc.
//
// m68k: tst.X dst sets N/Z from dst (sign-extended at .X for N).
func (c *Converter) emitFusedTstBcc(e *Emit, prod, cons Line) error {
	dstOp, err := ParseOperand(prod.Operands[0])
	if err != nil {
		return err
	}
	size := SizeBytes(prod.Size)
	if size == 0 {
		size = 2
	}
	kind := bccKind(cons.Mnemonic)
	if len(cons.Operands) != 1 {
		return fmt.Errorf("Bcc requires 1 operand")
	}
	label := strings.TrimSpace(cons.Operands[0])

	signed := isSignedBcc(kind)
	reg, err := c.fuseNormaliseValue(e, dstOp, size, signed, ScrV1)
	if err != nil {
		return err
	}
	// Compare against zero: BMI/BPL/BLT/BGE etc. all reduce to bccz forms.
	switch kind {
	case "eq":
		e.Lf("beqz %s, %s", reg, label)
	case "ne":
		e.Lf("bnez %s, %s", reg, label)
	case "lt", "mi":
		e.Lf("bltz %s, %s", reg, label)
	case "ge", "pl":
		e.Lf("bgez %s, %s", reg, label)
	case "gt":
		e.Lf("bgtz %s, %s", reg, label)
	case "le":
		e.Lf("blez %s, %s", reg, label)
	case "hi":
		// dst > 0 unsigned == dst != 0 (since width-masked).
		e.Lf("bnez %s, %s", reg, label)
	case "ls":
		// dst <= 0 unsigned == dst == 0.
		e.Lf("beqz %s, %s", reg, label)
	default:
		return fmt.Errorf("TST + b%s: unsupported fuse kind", kind)
	}
	return nil
}

// emitBccLine emits a fused two-register IE64 Bcc.
func emitBccLine(e *Emit, kind, ra, rb, label string) error {
	switch kind {
	case "eq":
		e.Lf("beq %s, %s, %s", ra, rb, label)
	case "ne":
		e.Lf("bne %s, %s, %s", ra, rb, label)
	case "lt":
		e.Lf("blt %s, %s, %s", ra, rb, label)
	case "ge":
		e.Lf("bge %s, %s, %s", ra, rb, label)
	case "gt":
		e.Lf("bgt %s, %s, %s", ra, rb, label)
	case "le":
		e.Lf("ble %s, %s, %s", ra, rb, label)
	case "hi":
		e.Lf("bhi %s, %s, %s", ra, rb, label)
	case "ls":
		e.Lf("bls %s, %s, %s", ra, rb, label)
	case "mi":
		// dst < 0 signed: comparing against 0 only meaningful for TST.
		// For CMP src,dst + BMI: branches if (dst-src) < 0, i.e. dst < src signed.
		e.Lf("blt %s, %s, %s", ra, rb, label)
	case "pl":
		e.Lf("bge %s, %s, %s", ra, rb, label)
	default:
		return fmt.Errorf("emitBccLine: unsupported kind %q", kind)
	}
	return nil
}
