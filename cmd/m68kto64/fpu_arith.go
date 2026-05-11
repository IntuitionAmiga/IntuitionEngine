package main

import (
	"fmt"
	"strings"
)

// =====================================================================
// Phase 7.3 — FPU arithmetic
//
// All ops degrade m68k extended (.X) → IE64 double precision. Single (.S)
// participates by widening at load time (`.S` source → `fcvtsd` → double
// pipeline; `.S` destination store → `fcvtds` narrow).
//
// Single-precision-native variants (FSGLMUL / FSGLDIV) emit the f-prefix
// (32-bit) IE64 ops directly.
// =====================================================================

// emitFArith handles binary ops (FADD/FSUB/FMUL/FDIV/FMOD/FREM/FSGLMUL/FSGLDIV).
//
// m68k forms:
//
//	F<op>.<size> FPm,FPn         — FPn := FPn op FPm
//	F<op>.<size> <ea>,FPn        — FPn := FPn op <ea>
func (c *Converter) emitFArith(e *Emit, l Line, m string) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s requires 2 operands", m)
	}
	src := strings.TrimSpace(l.Operands[0])
	dst := strings.TrimSpace(l.Operands[1])
	dstFP, ok := fpRegFromToken(dst)
	if !ok {
		return fmt.Errorf("%s dst must be FPn, got %q", m, dst)
	}
	size := l.Size
	if size == "" {
		size = ".x"
	}
	if strings.ToLower(size) == ".x" {
		// degraded silently; per-op no need to spam the comment
	}
	srcReg, err := c.materializeFPSrc(e, src, size)
	if err != nil {
		return err
	}
	op := fpArithOp(m)
	e.Lf("%s %s, %s, %s", op, dstFP, dstFP, srcReg)
	if c.fpccLive() {
		c.emitShadowFPCCFromResult(e, dstFP)
	}
	c.markFPInUse()
	return nil
}

func fpArithOp(m string) string {
	switch m {
	case "fadd":
		return "dadd"
	case "fsub":
		return "dsub"
	case "fmul":
		return "dmul"
	case "fdiv":
		return "ddiv"
	case "fmod":
		return "dmod"
	case "frem":
		// IEEE remainder differs from fmod for negative dividends; IE64 has
		// no native frem so we approximate with dmod and document the
		// compromise. Sign correction is left to the caller via FCMP at
		// validation time.
		return "dmod"
	case "fsglmul":
		return "fmul"
	case "fsgldiv":
		return "fdiv"
	}
	return "; UNKNOWN-FARITH " + m
}

// emitFUnary handles single-operand ops: FNEG, FABS, FSQRT, FINT, FINTRZ,
// FGETEXP, FGETMAN.
//
// m68k forms:
//
//	F<op>.<size> FPm,FPn         — FPn := op(FPm)
//	F<op>.<size> FPn             — FPn := op(FPn)
//	F<op>.<size> <ea>,FPn        — FPn := op(<ea>)
func (c *Converter) emitFUnary(e *Emit, l Line, m string) error {
	if len(l.Operands) < 1 || len(l.Operands) > 2 {
		return fmt.Errorf("%s requires 1 or 2 operands", m)
	}
	var src, dst string
	if len(l.Operands) == 1 {
		src = strings.TrimSpace(l.Operands[0])
		dst = src
	} else {
		src = strings.TrimSpace(l.Operands[0])
		dst = strings.TrimSpace(l.Operands[1])
	}
	dstFP, ok := fpRegFromToken(dst)
	if !ok {
		return fmt.Errorf("%s dst must be FPn, got %q", m, dst)
	}
	size := l.Size
	if size == "" {
		size = ".x"
	}
	srcReg, err := c.materializeFPSrc(e, src, size)
	if err != nil {
		return err
	}
	switch m {
	case "fneg":
		e.Lf("dneg %s, %s", dstFP, srcReg)
	case "fabs":
		e.Lf("dabs %s, %s", dstFP, srcReg)
	case "fsqrt":
		e.Lf("dsqrt %s, %s", dstFP, srcReg)
	case "fint":
		e.Lf("dint %s, %s", dstFP, srcReg)
	case "fintrz":
		// Save FPCR → set RZ → dint → restore.
		e.Lf("fmovcr %s", ScrV1)
		e.Lf("la %s, %s", ScrEA, FPSlotFPCRSave)
		e.Lf("store.l %s, (%s)", ScrV1, ScrEA)
		e.Lf("move.l %s, #2 ; round-toward-zero", ScrV2)
		e.Lf("fmovcc %s", ScrV2)
		e.Lf("dint %s, %s", dstFP, srcReg)
		e.Lf("la %s, %s", ScrEA, FPSlotFPCRSave)
		e.Lf("load.l %s, (%s)", ScrV1, ScrEA)
		e.Lf("fmovcc %s", ScrV1)
	case "fgetexp":
		// floor(log2(|x|)). Approximate via dabs → flog → multiply by 1/ln(2)
		// → dint. flog is single-precision in IE64; round-trip narrow/widen.
		label := c.addFPConst("1.4426950408889634", "1/ln(2) for fgetexp")
		e.Lf("dabs %s, %s", dstFP, srcReg)
		e.Lf("fcvtds %s, %s ; widen→narrow for single-precision flog", dstFP, dstFP)
		e.Lf("flog %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		e.Lf("la %s, %s", ScrEA, label)
		e.Lf("dload f10, (%s)", ScrEA)
		e.Lf("dmul %s, %s, f10", dstFP, dstFP)
		e.Lf("dint %s, %s", dstFP, dstFP)
		c.usesFPConstPool = true
	case "fgetman":
		// FGETMAN(x) = x / 2^fgetexp(x).
		// Lower in three stages:
		//   1. fgetexp into ScrV1 (integer exponent k).
		//   2. Build 2^k via IEEE-754 exponent bit pattern in f10
		//      (same trick as FSCALE; round-trip through scratch slot).
		//   3. ddiv dstFP, srcReg, f10.
		invLn2 := c.addFPConst("1.4426950408889634", "1/ln(2) for fgetman")
		// Stage 1: k = floor(log2(|x|))
		e.Lf("dabs f10, %s ; |x|", srcReg)
		e.Lf("fcvtds f10, f10")
		e.Lf("flog f10, f10")
		e.Lf("fcvtsd f10, f10")
		e.Lf("la %s, %s", ScrEA, invLn2)
		e.Lf("dload f12, (%s)", ScrEA)
		e.Lf("dmul f10, f10, f12")
		e.Lf("dint f10, f10")
		e.Lf("dcvtfi %s, f10", ScrV1)
		// Stage 2: build 2^k double in f10.
		e.Lf("add.l %s, %s, #1023", ScrV2, ScrV1)
		e.Lf("lsl.q %s, %s, #52", ScrV2, ScrV2)
		e.Lf("la %s, %s", ScrEA, FPSlotScratchQ)
		e.Lf("store.q %s, (%s)", ScrV2, ScrEA)
		e.Lf("dload f10, (%s)", ScrEA)
		// Stage 3: dstFP = srcReg / 2^k
		e.Lf("ddiv %s, %s, f10", dstFP, srcReg)
		c.usesFPConstPool = true
	default:
		return fmt.Errorf("unsupported unary op %s", m)
	}
	if c.fpccLive() {
		c.emitShadowFPCCFromResult(e, dstFP)
	}
	c.markFPInUse()
	return nil
}

// emitFScale: FPn := FPn * 2^FPm. Lowered via IEEE-754 exponent bit-pattern
// constructed in a GPR, round-tripped through __m68kto64_fp_scratch_q.
func (c *Converter) emitFScale(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("fscale requires 2 operands")
	}
	src := strings.TrimSpace(l.Operands[0])
	dst := strings.TrimSpace(l.Operands[1])
	srcFP, ok1 := fpRegFromToken(src)
	dstFP, ok2 := fpRegFromToken(dst)
	if !ok1 || !ok2 {
		return fmt.Errorf("fscale operands must be FPn,FPn")
	}
	// Extract integer scale factor: dcvtfi r17, srcFP.
	e.Lf("dcvtfi %s, %s", ScrV1, srcFP)
	// Build double-precision exponent: r18 = (1023 + r17) << 52.
	e.Lf("add.l %s, %s, #1023", ScrV2, ScrV1)
	e.Lf("lsl.q %s, %s, #52", ScrV2, ScrV2)
	// Round-trip via memory slot (no dmovi in IE64).
	e.Lf("la %s, %s", ScrEA, FPSlotScratchQ)
	e.Lf("store.q %s, (%s)", ScrV2, ScrEA)
	// Load into ScrFP1 (f10) — reserved for transpiler scratch per plan
	// §"Register-file mapping". MUST NOT target the guest stack-shadow
	// (r30 is integer; FP scratch is in the f-file).
	e.Lf("dload f10, (%s)", ScrEA)
	e.Lf("dmul %s, %s, f10", dstFP, dstFP)
	if c.fpccLive() {
		c.emitShadowFPCCFromResult(e, dstFP)
	}
	c.markFPInUse()
	return nil
}

// =====================================================================
// FP source materialisation helper
// =====================================================================

// materializeFPSrc returns an IE64 FP register name holding the value of `src`
// at double precision (after widening if .S, identity if .D / .X / FPn).
//
// Side effects: emits the load sequence to e.
func (c *Converter) materializeFPSrc(e *Emit, src string, size string) (string, error) {
	if r, ok := fpRegFromToken(src); ok {
		return r, nil
	}
	op, err := ParseOperand(src)
	if err != nil {
		return "", err
	}
	scratch := ScrFP1 // reserved scratch — see emit.go
	sz := strings.ToLower(size)
	switch sz {
	case ".s":
		if op.Mode == AMImmediate {
			e.Lf("la %s, %s", ScrEA, op.Imm)
			e.Lf("fload %s, (%s)", scratch, ScrEA)
		} else if err := c.emitFPMemLoad(e, op, scratch, ".s"); err != nil {
			return "", err
		}
		// Widen single → double.
		e.Lf("fcvtsd %s, %s", scratch, scratch)
		return scratch, nil
	case ".d", ".x", "":
		if op.Mode == AMImmediate {
			e.Lf("la %s, %s", ScrEA, op.Imm)
			e.Lf("dload %s, (%s)", scratch, ScrEA)
		} else if err := c.emitFPMemLoad(e, op, scratch, ".d"); err != nil {
			return "", err
		}
		return scratch, nil
	case ".b", ".w", ".l":
		szBytes := SizeBytes(sz)
		reg, imm, err := c.loadValue(e, op, szBytes, ScrV1)
		if err != nil {
			return "", err
		}
		if reg != "" {
			if szBytes == 1 {
				e.Lf("sext.b %s, %s", ScrV1, reg)
				reg = ScrV1
			} else if szBytes == 2 {
				e.Lf("sext.w %s, %s", ScrV1, reg)
				reg = ScrV1
			}
			e.Lf("dcvtif %s, %s", scratch, reg)
		} else {
			e.Lf("move.l %s, #%s", ScrV1, imm)
			e.Lf("dcvtif %s, %s", scratch, ScrV1)
		}
		return scratch, nil
	}
	return "", fmt.Errorf("materializeFPSrc: unrecognised size %q", size)
}
