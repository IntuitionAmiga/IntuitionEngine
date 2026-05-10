package main

import (
	"fmt"
	"strings"
)

// =====================================================================
// FCMP / FTST + FBcc adjacent fuse (Phase 7.4)
//
// Mirrors the integer-side CMP/TST + Bcc peephole. When an `FCMP` (or `FTST`)
// is immediately followed by an `FBcc` with no intervening label and no
// label on either line, the producer is lowered to a single `dcmp r17,
// fs, ft` and the consumer to an integer Bcc against r17 — skipping the
// full ShadowFPCC composition.
//
// Fuse-skip rule: any non-FCMP/FTST producer (incl. arithmetic op,
// `fmove.l Dn,fpsr`, `fmovsc`, or any label/labelled line in between)
// suppresses fuse. Fuse only fires when prod and cons are both adjacent
// **and** prod is fcmp/ftst. See plan §"Fuse-skip rule for FPSR writes".
//
// NaN-aware cc kinds (FBOR/FBUN/FBOGT/FBOGE/...) cannot be expressed by a
// single integer Bcc against r17 alone — they need the NaN bit too. For
// those, the producer still emits `dcmp` (cheap) but the consumer falls
// back to the standalone-shadow path (`emitShadowFBccTest`), so we
// effectively skip fuse and let Phase 7.4's regular emit handle it.
// =====================================================================

// fpFusableProducer reports whether `l` is an FCMP/FTST that may fuse with
// an immediately-following FBcc.
func fpFusableProducer(l Line) bool {
	if l.Kind != LineInstruction {
		return false
	}
	switch l.Mnemonic {
	case "fcmp":
		return len(l.Operands) == 2
	case "ftst":
		return len(l.Operands) == 1
	}
	return false
}

// canFuseFBcc reports whether the consumer's cc kind admits the
// single-integer-Bcc-on-r17 fused form. Excludes NaN-aware variants.
func canFuseFBcc(l Line) bool {
	if l.Kind != LineInstruction {
		return false
	}
	if !strings.HasPrefix(l.Mnemonic, "fb") {
		return false
	}
	cc := strings.TrimPrefix(l.Mnemonic, "fb")
	switch cc {
	case "eq", "ne", "lt", "le", "gt", "ge", "f", "t",
		"sf", "st", "seq", "sne":
		return true
	}
	return false
}

// emitFusedFCmpFBcc lowers an adjacent (fcmp|ftst, fbcc) pair without
// emitting the full ShadowFPCC update sequence.
func (c *Converter) emitFusedFCmpFBcc(e *Emit, prod, cons Line) error {
	// Emit the dcmp into r17.
	if err := c.emitFCmpForFuse(e, prod); err != nil {
		return err
	}
	cc := strings.TrimPrefix(cons.Mnemonic, "fb")
	if len(cons.Operands) != 1 {
		return fmt.Errorf("fbcc requires 1 operand")
	}
	target := strings.TrimSpace(cons.Operands[0])

	// Signaling variants (FBSEQ / FBSNE) raise the IO flag without
	// clobbering other FPSR bits (RMW: fmovsr; or; fmovsc).
	if cc == "seq" || cc == "sne" {
		e.Lf("fmovsr %s", ScrV2)
		e.Lf("or.l %s, %s, #1", ScrV2, ScrV2)
		e.Lf("fmovsc %s", ScrV2)
	}

	switch cc {
	case "f", "sf":
		e.Lf("; fbcc never taken (cc=F)")
	case "t", "st":
		e.Lf("bra %s", target)
	case "eq", "seq":
		e.Lf("beq %s, r0, %s", ScrV1, target)
	case "ne", "sne":
		e.Lf("bne %s, r0, %s", ScrV1, target)
	case "lt":
		e.Lf("blt %s, r0, %s", ScrV1, target)
	case "le":
		e.Lf("ble %s, r0, %s", ScrV1, target)
	case "gt":
		e.Lf("bgt %s, r0, %s", ScrV1, target)
	case "ge":
		e.Lf("bge %s, r0, %s", ScrV1, target)
	default:
		return fmt.Errorf("fused fbcc cc %q not handled", cc)
	}
	c.markFPInUse()
	return nil
}

// emitFCmpForFuse emits just the `dcmp r17, ...` portion of FCMP/FTST,
// without ShadowFPCC composition. Caller emits the integer Bcc next.
func (c *Converter) emitFCmpForFuse(e *Emit, l Line) error {
	if l.Mnemonic == "ftst" {
		src := strings.TrimSpace(l.Operands[0])
		srcReg, err := c.materializeFPSrc(e, src, l.Size)
		if err != nil {
			return err
		}
		label := c.addFPConst("0.0", "ftst zero constant (fused)")
		e.Lf("la %s, %s", ScrEA, label)
		e.Lf("dload f12, (%s)", ScrEA)
		e.Lf("dcmp %s, %s, f12", ScrV1, srcReg)
		c.markFPInUse()
		c.usesFPConstPool = true
		return nil
	}
	// fcmp
	src := strings.TrimSpace(l.Operands[0])
	dst := strings.TrimSpace(l.Operands[1])
	dstFP, ok := fpRegFromToken(dst)
	if !ok {
		return fmt.Errorf("fcmp dst must be FPn, got %q", dst)
	}
	size := l.Size
	if size == "" {
		size = ".x"
	}
	srcReg, err := c.materializeFPSrc(e, src, size)
	if err != nil {
		return err
	}
	e.Lf("dcmp %s, %s, %s", ScrV1, dstFP, srcReg)
	c.markFPInUse()
	return nil
}
