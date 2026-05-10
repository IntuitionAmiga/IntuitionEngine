package main

import (
	"fmt"
	"strings"
)

// =====================================================================
// Phase 7.4 — comparison + ShadowFPCC + FBcc / FDBcc / FScc / FTRAPcc
//
// FCMP <ea>,FPn  → dcmp r17, fs, ft (writes -1/0/+1 into integer GPR)
// FTST <ea>      → dcmp r17, fs, f_zero
//
// ShadowFPCC (r29) layout matches m68k FPSR bits 27:24 for direct readability:
//   bit 3 = N    bit 2 = Z    bit 1 = I (infinity)    bit 0 = NaN
//
// Adjacent FCMP/FTST + FBcc fuse: dcmp + integer Bcc on r17.
// =====================================================================

// emitFCmp lowers FCMP. Always writes ShadowFPCC; liveness-elision is the
// caller's job in the fused path (see emitFusedFCmpFBcc).
func (c *Converter) emitFCmp(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("fcmp requires 2 operands")
	}
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
	if c.fpccLive() {
		c.emitShadowFPCCFromDcmp(e)
	}
	c.markFPInUse()
	return nil
}

// emitFTst lowers FTST. Compares FPn against +0.0 from the constant pool.
func (c *Converter) emitFTst(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("ftst requires 1 operand")
	}
	src := strings.TrimSpace(l.Operands[0])
	srcReg, err := c.materializeFPSrc(e, src, l.Size)
	if err != nil {
		return err
	}
	label := c.addFPConst("0.0", "ftst zero constant")
	e.Lf("la %s, %s", ScrEA, label)
	// FTST source may already live in f10 (ScrFP1) when materialised from
	// memory — load the zero constant into f12 (ScrFP2) to avoid clobber.
	e.Lf("dload f12, (%s)", ScrEA)
	e.Lf("dcmp %s, %s, f12", ScrV1, srcReg)
	if c.fpccLive() {
		c.emitShadowFPCCFromDcmp(e)
	}
	c.usesFPConstPool = true
	c.markFPInUse()
	return nil
}

// emitShadowFPCCFromResult emits the post-arithmetic ShadowFPCC update —
// compare the result against +0.0 then compose r29. Used by cc-affecting
// FPU arith ops when a downstream consumer is live.
func (c *Converter) emitShadowFPCCFromResult(e *Emit, resultFP string) {
	label := c.addFPConst("0.0", "arith result vs zero")
	e.Lf("la %s, %s", ScrEA, label)
	e.Lf("dload f12, (%s)", ScrEA)
	e.Lf("dcmp %s, %s, f12", ScrV1, resultFP)
	c.emitShadowFPCCFromDcmp(e)
	c.usesFPConstPool = true
}

// emitShadowFPCCFromDcmp composes ShadowFPCC (r29) from a freshly-issued
// `dcmp ScrV1, ...` integer result plus the hardware FPSR NaN bit.
//
//	r29 layout:  bit3=N  bit2=Z  bit1=I  bit0=NaN
//
// The standalone path always emits the full update; liveness-driven elision
// happens in the fused path.
func (c *Converter) emitShadowFPCCFromDcmp(e *Emit) {
	zLabel := e.NewLabel("fpcc_z")
	nLabel := e.NewLabel("fpcc_n")
	// Z = (r17 == 0) → 1 else 0; place at bit 2.
	e.Lf("move.l %s, #0", ShadowTmp1)
	e.Lf("bne %s, r0, %s", ScrV1, zLabel)
	e.Lf("move.l %s, #4 ; bit2 (Z)", ShadowTmp1)
	e.Label(zLabel)
	// N = (r17 < 0) → 1 else 0; place at bit 3.
	e.Lf("move.l %s, #0", ShadowTmp2)
	e.Lf("bge %s, r0, %s", ScrV1, nLabel)
	e.Lf("move.l %s, #8 ; bit3 (N)", ShadowTmp2)
	e.Label(nLabel)
	e.Lf("or.l %s, %s, %s", ShadowFPCC, ShadowTmp1, ShadowTmp2)
	// NaN bit: hardware FPSR bit 26 → r29 bit 0.
	e.Lf("fmovsr %s", ShadowTmp1)
	e.Lf("lsr.l %s, %s, #26", ShadowTmp1, ShadowTmp1)
	e.Lf("and.l %s, %s, #1", ShadowTmp1, ShadowTmp1)
	e.Lf("or.l %s, %s, %s", ShadowFPCC, ShadowFPCC, ShadowTmp1)
}

// =====================================================================
// FP cc → integer Bcc table (used by FBcc / FDBcc / FScc / FTRAPcc)
// =====================================================================

// fpCCKind canonicalises the suffix of an F<branch/set/dbcc/trap>cc
// mnemonic (e.g. "fbeq" → "eq").
func fpCCKind(m string) string {
	for _, prefix := range []string{"ftrap", "fdb", "fb", "fs"} {
		if strings.HasPrefix(m, prefix) {
			return m[len(prefix):]
		}
	}
	return ""
}

// fpFTrapccSyscall returns the locked syscall # (32–63) for a given FTRAPcc
// suffix. Order is fixed in plan §"Locked syscall # vector table" /
// `sdk/docs/M68KtoIE64.md` §11.FP.
var fpFTrapccOrder = []string{
	"f", "eq", "ogt", "oge", "olt", "ole", "ogl", "or",
	"un", "ueq", "ugt", "uge", "ult", "ule", "ne", "t",
	"sf", "seq", "gt", "ge", "lt", "le", "gl", "gle",
	"ngle", "ngl", "nle", "nlt", "nge", "ngt", "sne", "st",
}

func fpFTrapccSyscall(cc string) (int, bool) {
	for i, k := range fpFTrapccOrder {
		if k == cc {
			return 32 + i, true
		}
	}
	return 0, false
}

// emitShadowFBccTest emits the standalone (non-fused) test sequence for an
// FBcc/FDBcc/FScc/FTRAPcc cc kind. Result lands in ScrV1: 1 if cc holds, 0
// otherwise. Reads ShadowFPCC (r29).
func (c *Converter) emitShadowFBccTest(e *Emit, cc string) {
	// Extract bits.
	e.Lf("and.l %s, %s, #1", ShadowTmp1, ShadowFPCC)            // NaN
	e.Lf("lsr.l %s, %s, #2", ShadowTmp2, ShadowFPCC)            // (r29 >> 2)
	e.Lf("and.l %s, %s, #1", ShadowTmp2, ShadowTmp2)            // Z
	e.Lf("lsr.l %s, %s, #3", ScrV1, ShadowFPCC)                 // (r29 >> 3)
	e.Lf("and.l %s, %s, #1 ; ScrV1=N, ShadowTmp1=NaN, ShadowTmp2=Z", ScrV1, ScrV1)
	switch cc {
	case "f", "sf":
		e.Lf("move.l %s, #0", ScrV1)
	case "t", "st":
		e.Lf("move.l %s, #1", ScrV1)
	case "eq", "seq":
		e.Lf("move.l %s, %s", ScrV1, ShadowTmp2)
	case "ne", "sne":
		e.Lf("xor.l %s, %s, #1", ScrV1, ShadowTmp2)
	case "or":
		e.Lf("xor.l %s, %s, #1", ScrV1, ShadowTmp1)
	case "un":
		e.Lf("move.l %s, %s", ScrV1, ShadowTmp1)
	case "gt", "ogt":
		// !NaN && !Z && !N
		e.Lf("or.l %s, %s, %s", ShadowTmp1, ShadowTmp1, ShadowTmp2) // NaN | Z
		e.Lf("or.l %s, %s, %s", ShadowTmp1, ShadowTmp1, ScrV1)      // | N
		e.Lf("xor.l %s, %s, #1", ScrV1, ShadowTmp1)
	case "ge", "oge":
		// !NaN && (Z || !N)
		e.Lf("xor.l %s, %s, #1", ScrV1, ScrV1)                       // !N
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp2)           // !N | Z
		e.Lf("xor.l %s, %s, #1", ShadowTmp1, ShadowTmp1)             // !NaN
		e.Lf("and.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp1)
	case "lt", "olt":
		// !NaN && N && !Z
		e.Lf("xor.l %s, %s, #1", ShadowTmp2, ShadowTmp2)             // !Z
		e.Lf("and.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp2)           // N & !Z
		e.Lf("xor.l %s, %s, #1", ShadowTmp1, ShadowTmp1)             // !NaN
		e.Lf("and.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp1)
	case "le", "ole":
		// !NaN && (Z || N)
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp2)           // N | Z
		e.Lf("xor.l %s, %s, #1", ShadowTmp1, ShadowTmp1)             // !NaN
		e.Lf("and.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp1)
	case "gl", "ogl":
		// !NaN && !Z
		e.Lf("xor.l %s, %s, #1", ShadowTmp2, ShadowTmp2)             // !Z
		e.Lf("xor.l %s, %s, #1", ShadowTmp1, ShadowTmp1)             // !NaN
		e.Lf("and.l %s, %s, %s", ScrV1, ShadowTmp1, ShadowTmp2)
	case "gle":
		// !NaN
		e.Lf("xor.l %s, %s, #1", ScrV1, ShadowTmp1)
	case "ngle":
		// NaN
		e.Lf("move.l %s, %s", ScrV1, ShadowTmp1)
	case "ngl":
		// NaN || Z
		e.Lf("or.l %s, %s, %s", ScrV1, ShadowTmp1, ShadowTmp2)
	case "nle":
		// NaN || (!Z && !N)
		e.Lf("xor.l %s, %s, #1", ShadowTmp2, ShadowTmp2)             // !Z
		e.Lf("xor.l %s, %s, #1", ScrV1, ScrV1)                        // !N
		e.Lf("and.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp2)           // !Z & !N
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp1)            // | NaN
	case "nlt":
		// NaN || (Z || !N)
		e.Lf("xor.l %s, %s, #1", ScrV1, ScrV1)                        // !N
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp2)           // | Z
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp1)            // | NaN
	case "nge":
		// NaN || (N && !Z)
		e.Lf("xor.l %s, %s, #1", ShadowTmp2, ShadowTmp2)             // !Z
		e.Lf("and.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp2)           // N & !Z
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp1)            // | NaN
	case "ngt":
		// NaN || Z || N
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp2)           // N | Z
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp1)            // | NaN
	case "ueq":
		// Z || NaN
		e.Lf("or.l %s, %s, %s", ScrV1, ShadowTmp2, ShadowTmp1)
	case "ugt":
		// NaN || (!Z && !N)  (same as nle without strict treatment)
		e.Lf("xor.l %s, %s, #1", ShadowTmp2, ShadowTmp2)
		e.Lf("xor.l %s, %s, #1", ScrV1, ScrV1)
		e.Lf("and.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp2)
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp1)
	case "uge":
		// NaN || !N
		e.Lf("xor.l %s, %s, #1", ScrV1, ScrV1)
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp1)
	case "ult":
		// NaN || (N && !Z)
		e.Lf("xor.l %s, %s, #1", ShadowTmp2, ShadowTmp2)
		e.Lf("and.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp2)
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp1)
	case "ule":
		// NaN || N || Z
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp2)
		e.Lf("or.l %s, %s, %s", ScrV1, ScrV1, ShadowTmp1)
	default:
		// Unknown cc — fall back to "true" with diagnostic.
		e.Lf("; UNKNOWN-FPCC %s", cc)
		e.Lf("move.l %s, #1", ScrV1)
	}
}

// =====================================================================
// FBcc — FP conditional branch
// =====================================================================

func (c *Converter) emitFBcc(e *Emit, l Line, m string) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("%s requires 1 operand", m)
	}
	target := strings.TrimSpace(l.Operands[0])
	cc := fpCCKind(m)
	switch cc {
	case "f", "sf":
		// always-false → no emit.
		e.Lf("; %s never taken (cc=F)", m)
		return nil
	case "t", "st":
		e.Lf("bra %s", target)
		return nil
	}
	// Signaling variants (FBSEQ / FBSNE) raise the IO flag without clobbering
	// other FPSR bits (RMW: fmovsr; or; fmovsc).
	if strings.HasPrefix(cc, "s") && (cc == "seq" || cc == "sne") {
		e.Lf("fmovsr %s", ScrV2)
		e.Lf("or.l %s, %s, #1", ScrV2, ScrV2)
		e.Lf("fmovsc %s", ScrV2)
	}
	c.emitShadowFBccTest(e, cc)
	e.Lf("bnez %s, %s", ScrV1, target)
	return nil
}

// =====================================================================
// FDBcc — FP decrement-and-branch on condition false
// =====================================================================

func (c *Converter) emitFDBcc(e *Emit, l Line, m string) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s requires 2 operands", m)
	}
	dn := strings.TrimSpace(l.Operands[0])
	target := strings.TrimSpace(l.Operands[1])
	r, ok := LookupRegister(dn)
	if !ok || r.Class != RegData {
		return fmt.Errorf("%s first operand must be Dn, got %q", m, dn)
	}
	cc := fpCCKind(m)
	// If cc holds, fall through (no decrement). Otherwise decrement Dn (low
	// 16 bits) and branch to target if Dn != -1.
	skip := e.NewLabel("fdb_skip")
	switch cc {
	case "f", "sf":
		// cc=F never holds; always decrement.
	case "t", "st":
		// cc=T always holds; never decrement, never branch.
		e.Lf("; %s never iterates (cc=T)", m)
		return nil
	default:
		c.emitShadowFBccTest(e, cc)
		e.Lf("bnez %s, %s", ScrV1, skip)
	}
	e.Lf("sub.l %s, %s, #1", r.IE64, r.IE64)
	e.Lf("sext.w %s, %s", ShadowTmp1, r.IE64)
	e.Lf("move.l %s, #-1", ShadowTmp2)
	e.Lf("bne %s, %s, %s", ShadowTmp1, ShadowTmp2, target)
	e.Label(skip)
	return nil
}

// =====================================================================
// FScc — FP conditional set-byte
// =====================================================================

func (c *Converter) emitFScc(e *Emit, l Line, m string) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("%s requires 1 operand", m)
	}
	dst := strings.TrimSpace(l.Operands[0])
	cc := fpCCKind(m)
	c.emitShadowFBccTest(e, cc)
	// Result is 0 or 1 in ScrV1; m68k Scc writes 0xFF/0x00.
	e.Lf("neg.l %s, %s ; expand 0/1 → 0x00/0xFFFFFFFF", ScrV1, ScrV1)
	op, err := ParseOperand(dst)
	if err != nil {
		return fmt.Errorf("fscc dst: %v", err)
	}
	return c.storeValue(e, op, 1, ScrV1)
}

// =====================================================================
// FTRAPcc — conditional syscall against ShadowFPCC
// =====================================================================

func (c *Converter) emitFTrapcc(e *Emit, l Line, m string) error {
	cc := fpCCKind(m)
	num, ok := fpFTrapccSyscall(cc)
	if !ok {
		return fmt.Errorf("%s: unknown FTRAPcc cc %q", m, cc)
	}
	c.emitShadowFBccTest(e, cc)
	skip := e.NewLabel("ftrap_skip")
	e.Lf("beqz %s, %s", ScrV1, skip)
	e.Lf("syscall #%d", num)
	e.Label(skip)
	return nil
}
