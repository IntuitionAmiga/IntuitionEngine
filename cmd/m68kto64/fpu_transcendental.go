package main

import (
	"fmt"
	"strings"
)

// =====================================================================
// Phase 7.5 — Transcendentals
//
// IE64 exposes single-precision fsin/fcos/ftan/fatan/flog/fexp/fpow. m68k
// FPU ops are extended-precision; we degrade to double, narrow to single
// for the IE64 native call, then widen back. Identities synthesise the
// remainder of the m68k transcendental set.
//
// Approximations are documented per-mnemonic in `sdk/docs/M68KtoIE64.md`
// §15.FP.
// =====================================================================

func (c *Converter) emitFTranscendental(e *Emit, l Line, m string) error {
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
	srcReg, err := c.materializeFPSrc(e, src, l.Size)
	if err != nil {
		return err
	}
	c.markFPInUse()
	defer func() {
		if c.fpccLive() {
			c.emitShadowFPCCFromResult(e, dstFP)
		}
	}()

	// Direct single-precision native ops.
	switch m {
	case "fsin", "fcos", "ftan", "fatan":
		e.Lf("fcvtds %s, %s", dstFP, srcReg)
		e.Lf("%s %s, %s", m, dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		return nil
	case "fetox":
		e.Lf("fcvtds %s, %s", dstFP, srcReg)
		e.Lf("fexp %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		return nil
	case "flogn":
		e.Lf("fcvtds %s, %s", dstFP, srcReg)
		e.Lf("flog %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		return nil
	}

	// Identity-synthesised ops.
	switch m {
	case "fasin":
		// fatan(x / sqrt(1 - x^2))
		e.Lf("dmov f10, %s", srcReg)
		e.Lf("dmul %s, f10, f10", dstFP)
		oneLbl := c.addFPConst("1.0", "1.0 const for fasin")
		e.Lf("la %s, %s", ScrEA, oneLbl)
		e.Lf("dload f12, (%s)", ScrEA)
		e.Lf("dsub %s, f12, %s", dstFP, dstFP)
		e.Lf("dsqrt %s, %s", dstFP, dstFP)
		e.Lf("ddiv %s, f10, %s", dstFP, dstFP)
		e.Lf("fcvtds %s, %s", dstFP, dstFP)
		e.Lf("fatan %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		c.usesFPConstPool = true
		return nil
	case "facos":
		// pi/2 - fasin(x). Lower fasin inline then subtract.
		piHalf := c.addFPConst("1.5707963267948966", "pi/2 for facos")
		e.Lf("dmov f10, %s", srcReg)
		e.Lf("dmul %s, f10, f10", dstFP)
		oneLbl := c.addFPConst("1.0", "1.0 const for facos")
		e.Lf("la %s, %s", ScrEA, oneLbl)
		e.Lf("dload f12, (%s)", ScrEA)
		e.Lf("dsub %s, f12, %s", dstFP, dstFP)
		e.Lf("dsqrt %s, %s", dstFP, dstFP)
		e.Lf("ddiv %s, f10, %s", dstFP, dstFP)
		e.Lf("fcvtds %s, %s", dstFP, dstFP)
		e.Lf("fatan %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		e.Lf("la %s, %s", ScrEA, piHalf)
		e.Lf("dload f12, (%s)", ScrEA)
		e.Lf("dsub %s, f12, %s", dstFP, dstFP)
		c.usesFPConstPool = true
		return nil
	case "fcosh":
		// (fexp(x) + fexp(-x)) / 2
		c.emitHyperbolicHelper(e, srcReg, dstFP, true)
		halfLbl := c.addFPConst("0.5", "0.5 const for fcosh/fsinh")
		e.Lf("la %s, %s", ScrEA, halfLbl)
		e.Lf("dload f12, (%s)", ScrEA)
		e.Lf("dmul %s, %s, f12", dstFP, dstFP)
		c.usesFPConstPool = true
		return nil
	case "fsinh":
		c.emitHyperbolicHelper(e, srcReg, dstFP, false)
		halfLbl := c.addFPConst("0.5", "0.5 const for fcosh/fsinh")
		e.Lf("la %s, %s", ScrEA, halfLbl)
		e.Lf("dload f12, (%s)", ScrEA)
		e.Lf("dmul %s, %s, f12", dstFP, dstFP)
		c.usesFPConstPool = true
		return nil
	case "ftanh":
		// fsinh(x) / fcosh(x)
		c.emitHyperbolicHelper(e, srcReg, dstFP, false) // dstFP = sinh-pre-half (e^x - e^-x)
		c.emitHyperbolicHelper(e, srcReg, "f14", true)  // f14 = cosh-pre-half (e^x + e^-x)
		e.Lf("ddiv %s, %s, f14", dstFP, dstFP)
		return nil
	case "fatanh":
		// 0.5 * flog((1 + x) / (1 - x))
		oneLbl := c.addFPConst("1.0", "1.0 const for fatanh")
		halfLbl := c.addFPConst("0.5", "0.5 const for fatanh")
		e.Lf("la %s, %s", ScrEA, oneLbl)
		e.Lf("dload f10, (%s)", ScrEA)
		e.Lf("dadd f12, f10, %s", srcReg) // 1+x
		e.Lf("dsub f10, f10, %s", srcReg) // 1-x
		e.Lf("ddiv %s, f12, f10", dstFP)
		e.Lf("fcvtds %s, %s", dstFP, dstFP)
		e.Lf("flog %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		e.Lf("la %s, %s", ScrEA, halfLbl)
		e.Lf("dload f10, (%s)", ScrEA)
		e.Lf("dmul %s, %s, f10", dstFP, dstFP)
		c.usesFPConstPool = true
		return nil
	case "fetoxm1":
		// fexp(x) - 1. Loses precision near 0; documented.
		e.Lf("fcvtds %s, %s", dstFP, srcReg)
		e.Lf("fexp %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		oneLbl := c.addFPConst("1.0", "1.0 for fetoxm1")
		e.Lf("la %s, %s", ScrEA, oneLbl)
		e.Lf("dload f10, (%s)", ScrEA)
		e.Lf("dsub %s, %s, f10", dstFP, dstFP)
		c.usesFPConstPool = true
		return nil
	case "flog10":
		// flog(x) / flog(10) — flog(10) precomputed at output-pool emit.
		const ln10 = "2.302585092994046"
		lbl := c.addFPConst(ln10, "ln(10) for flog10")
		e.Lf("fcvtds %s, %s", dstFP, srcReg)
		e.Lf("flog %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		e.Lf("la %s, %s", ScrEA, lbl)
		e.Lf("dload f10, (%s)", ScrEA)
		e.Lf("ddiv %s, %s, f10", dstFP, dstFP)
		c.usesFPConstPool = true
		return nil
	case "flog2":
		const ln2 = "0.6931471805599453"
		lbl := c.addFPConst(ln2, "ln(2) for flog2")
		e.Lf("fcvtds %s, %s", dstFP, srcReg)
		e.Lf("flog %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		e.Lf("la %s, %s", ScrEA, lbl)
		e.Lf("dload f10, (%s)", ScrEA)
		e.Lf("ddiv %s, %s, f10", dstFP, dstFP)
		c.usesFPConstPool = true
		return nil
	case "flognp1":
		// flog(1 + x); precision loss near 0.
		oneLbl := c.addFPConst("1.0", "1.0 for flognp1")
		e.Lf("la %s, %s", ScrEA, oneLbl)
		e.Lf("dload f10, (%s)", ScrEA)
		e.Lf("dadd %s, %s, f10", dstFP, srcReg)
		e.Lf("fcvtds %s, %s", dstFP, dstFP)
		e.Lf("flog %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		c.usesFPConstPool = true
		return nil
	case "ftentox":
		// 10^x = fexp(x * ln(10))
		const ln10 = "2.302585092994046"
		lbl := c.addFPConst(ln10, "ln(10) for ftentox")
		e.Lf("la %s, %s", ScrEA, lbl)
		e.Lf("dload f10, (%s)", ScrEA)
		e.Lf("dmul %s, %s, f10", dstFP, srcReg)
		e.Lf("fcvtds %s, %s", dstFP, dstFP)
		e.Lf("fexp %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		c.usesFPConstPool = true
		return nil
	case "ftwotox":
		const ln2 = "0.6931471805599453"
		lbl := c.addFPConst(ln2, "ln(2) for ftwotox")
		e.Lf("la %s, %s", ScrEA, lbl)
		e.Lf("dload f10, (%s)", ScrEA)
		e.Lf("dmul %s, %s, f10", dstFP, srcReg)
		e.Lf("fcvtds %s, %s", dstFP, dstFP)
		e.Lf("fexp %s, %s", dstFP, dstFP)
		e.Lf("fcvtsd %s, %s", dstFP, dstFP)
		c.usesFPConstPool = true
		return nil
	}
	return fmt.Errorf("unsupported transcendental %s", m)
}

// emitHyperbolicHelper emits e^x and e^-x and either adds (cosh) or subtracts
// (sinh) into dstFP. f10/f14 used as scratch. Caller scales by 0.5 if needed.
func (c *Converter) emitHyperbolicHelper(e *Emit, srcReg, dstFP string, add bool) {
	// f10 = e^x
	e.Lf("fcvtds f10, %s", srcReg)
	e.Lf("fexp f10, f10")
	e.Lf("fcvtsd f10, f10")
	// f12 = e^-x
	e.Lf("dneg f12, %s", srcReg)
	e.Lf("fcvtds f12, f12")
	e.Lf("fexp f12, f12")
	e.Lf("fcvtsd f12, f12")
	if add {
		e.Lf("dadd %s, f10, f12", dstFP)
	} else {
		e.Lf("dsub %s, f10, f12", dstFP)
	}
}
