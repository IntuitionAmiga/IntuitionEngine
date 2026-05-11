package main

import (
	"fmt"
	"strings"
)

// Phase B — remaining 68020 specials.
//
// Covered here (with TDD):
//   - TRAPV       conditional syscall #18 against shadow V (m68k vector 7,
//                 relocated to keep TRAP-instruction range disjoint).
//   - ABCD/SBCD   binary BCD add/sub with low/high nybble adjust; reads shadow C
//                 as the m68k X (extend) flag.
//   - NBCD        zero-minus-dst with BCD adjust.
//   - PACK        2 BCD bytes → 1 byte (low nybbles of each, plus adjustment).
//   - UNPK        1 byte → 2 BCD bytes (split nybbles, add adjustment).
//   - CAS         non-atomic load → cmp → store fallback (per plan §"Risks #2").
//   - BFINS / BFCLR / BFSET / BFCHG / BFTST / BFFFO  — bit-field ops on Dn,
//                 inline shift+mask form. Memory bit-field operands fall back
//                 to "; ERROR" until needed.

// emitPhaseB returns true iff the mnemonic was handled.
func (c *Converter) emitPhaseB(e *Emit, l Line) (bool, error) {
	switch l.Mnemonic {
	case "trapv":
		return true, c.emitTrapVShadow(e, l)
	case "abcd":
		return true, c.emitBcdAdd(e, l, false)
	case "sbcd":
		return true, c.emitBcdSub(e, l, false)
	case "nbcd":
		return true, c.emitNbcd(e, l)
	case "pack":
		return true, c.emitPack(e, l)
	case "unpk":
		return true, c.emitUnpk(e, l)
	case "cas":
		return true, c.emitCas(e, l)
	case "cas2":
		return true, c.emitCas2(e, l)
	case "bfins":
		return true, c.emitBfins(e, l)
	case "bfclr":
		return true, c.emitBfModify(e, l, "clr")
	case "bfset":
		return true, c.emitBfModify(e, l, "set")
	case "bfchg":
		return true, c.emitBfModify(e, l, "chg")
	case "bftst":
		return true, c.emitBftst(e, l)
	case "bfffo":
		return true, c.emitBfffo(e, l)
	case "addx":
		return true, c.emitAddxSubx(e, l, true)
	case "subx":
		return true, c.emitAddxSubx(e, l, false)
	case "negx":
		return true, c.emitNegx(e, l)
	case "roxl":
		return true, c.emitRox(e, l, true)
	case "roxr":
		return true, c.emitRox(e, l, false)
	}
	return false, nil
}

// emitTrapVShadow lowers TRAPV — if shadow V set, syscall #18 (m68k vector 7).
func (c *Converter) emitTrapVShadow(e *Emit, l Line) error {
	skip := e.NewLabel("trapv_skip")
	e.Lf("beqz %s, %s", ShadowV, skip)
	e.L("syscall #18")
	e.Label(skip)
	return nil
}

// =====================================================================
// BCD ops (ABCD / SBCD / NBCD)
// =====================================================================
//
// Operand forms:
//   ABCD Dy,Dx
//   ABCD -(Ay),-(Ax)
//
// Lowering uses ScrV1 / ScrV2 / ScrAux for the BCD adjust dance.
// X (extend) flag taken from shadow C.

// bcdLoadOperands resolves the two operands of an ABCD/SBCD form. For
// register-direct: returns the IE64 reg names. For -(Ay),-(Ax): emits the
// predecrement loads into ScrV2 (src) and ScrV1 (dst) and returns sentinel
// names. Caller knows which path it asked for.
func (c *Converter) bcdLoadOperands(e *Emit, l Line) (srcReg, dstHolder, dstReg string, isMem bool, dstOp Operand, err error) {
	if len(l.Operands) != 2 {
		err = fmt.Errorf("%s requires 2 operands", l.Mnemonic)
		return
	}
	src, e1 := ParseOperand(l.Operands[0])
	if e1 != nil {
		err = e1
		return
	}
	dst, e2 := ParseOperand(l.Operands[1])
	if e2 != nil {
		err = e2
		return
	}
	if src.Mode == AMDataReg && dst.Mode == AMDataReg {
		return src.Reg.IE64, dst.Reg.IE64, dst.Reg.IE64, false, dst, nil
	}
	if src.Mode == AMPreDec && dst.Mode == AMPreDec {
		// -(Ay): load src byte
		e.Lf("sub.l %s, %s, #1", src.Reg.IE64, src.Reg.IE64)
		e.Lf("load.b %s, (%s)", ScrV2, src.Reg.IE64)
		// -(Ax): load dst byte
		e.Lf("sub.l %s, %s, #1", dst.Reg.IE64, dst.Reg.IE64)
		e.Lf("load.b %s, (%s)", ScrV1, dst.Reg.IE64)
		return ScrV2, ScrV1, ScrV1, true, dst, nil
	}
	err = fmt.Errorf("%s: unsupported operand combination", l.Mnemonic)
	return
}

// emitBcdAdd lowers ABCD: dst = (dst + src + X) BCD-adjusted.
func (c *Converter) emitBcdAdd(e *Emit, l Line, _ bool) error {
	srcReg, _, dstReg, isMem, dstOp, err := c.bcdLoadOperands(e, l)
	if err != nil {
		return err
	}
	// raw = (dst & 0xFF) + (src & 0xFF) + X
	e.Lf("and.l %s, %s, #$FF", ScrAux, dstReg)
	e.Lf("and.l %s, %s, #$FF", ScrV1, srcReg) // ScrV1 = src masked
	e.Lf("add.l %s, %s, %s", ScrAux, ScrAux, ScrV1)
	e.Lf("add.l %s, %s, %s", ScrAux, ScrAux, ShadowX) // + X (chain-in)
	// Low-nybble adjust: if (raw & 0xF) > 9 → raw += 6.
	e.Lf("and.l %s, %s, #$F", ScrV1, ScrAux)
	e.Lf("move.l %s, #9", ScrV2)
	skipLow := e.NewLabel("abcd_lo")
	e.Lf("ble %s, %s, %s", ScrV1, ScrV2, skipLow)
	e.Lf("add.l %s, %s, #6", ScrAux, ScrAux)
	e.Label(skipLow)
	// High-nybble adjust: if raw > 0x99 → raw += 0x60; carry out.
	e.Lf("move.l %s, #$99", ScrV2)
	e.Lf("move.l %s, #0", ShadowC)
	skipHi := e.NewLabel("abcd_hi")
	e.Lf("ble %s, %s, %s", ScrAux, ScrV2, skipHi)
	e.Lf("add.l %s, %s, #$60", ScrAux, ScrAux)
	e.Lf("move.l %s, #1", ShadowC)
	e.Label(skipHi)
	e.Lf("and.l %s, %s, #$FF", ScrAux, ScrAux)
	// Sticky Z: cleared (r25 nonzero) iff result nonzero; preserved otherwise.
	// Branchless: r25 := r25 OR result_byte.
	e.Lf("or.l %s, %s, %s", ShadowZ, ShadowZ, ScrAux)
	e.Lf("sext.b %s, %s", ShadowN, ScrAux)
	// X := C (m68k sets X = C for BCD ops).
	e.Lf("move.l %s, %s", ShadowX, ShadowC)
	// Write back.
	if isMem {
		e.Lf("store.b %s, (%s)", ScrAux, dstOp.Reg.IE64)
		return nil
	}
	// Dn dst — partial-update merge.
	e.Lf("and.q %s, %s, #%s", dstReg, dstReg, SizeInvMask(1))
	e.Lf("or.q %s, %s, %s", dstReg, dstReg, ScrAux)
	return nil
}

// emitBcdSub lowers SBCD: dst = (dst - src - X) BCD-adjusted.
func (c *Converter) emitBcdSub(e *Emit, l Line, _ bool) error {
	srcReg, _, dstReg, isMem, dstOp, err := c.bcdLoadOperands(e, l)
	if err != nil {
		return err
	}
	e.Lf("and.l %s, %s, #$FF", ScrAux, dstReg)
	e.Lf("and.l %s, %s, #$FF", ScrV1, srcReg)
	e.Lf("sub.l %s, %s, %s", ScrAux, ScrAux, ScrV1)
	e.Lf("sub.l %s, %s, %s", ScrAux, ScrAux, ShadowX) // chain-in X
	// Borrow detection: if low nybble borrowed → adjust -6; if high nybble → -0x60.
	// Simplified approximation: check sign of raw.
	skipLow := e.NewLabel("sbcd_lo")
	e.Lf("and.l %s, %s, #$10", ScrV1, ScrAux)
	e.Lf("beqz %s, %s", ScrV1, skipLow)
	e.Lf("sub.l %s, %s, #6", ScrAux, ScrAux)
	e.Label(skipLow)
	skipHi := e.NewLabel("sbcd_hi")
	e.Lf("and.l %s, %s, #$100", ScrV1, ScrAux)
	e.Lf("move.l %s, #0", ShadowC)
	e.Lf("beqz %s, %s", ScrV1, skipHi)
	e.Lf("sub.l %s, %s, #$60", ScrAux, ScrAux)
	e.Lf("move.l %s, #1", ShadowC)
	e.Label(skipHi)
	e.Lf("and.l %s, %s, #$FF", ScrAux, ScrAux)
	e.Lf("or.l %s, %s, %s", ShadowZ, ShadowZ, ScrAux)
	e.Lf("move.l %s, %s", ShadowX, ShadowC)
	e.Lf("sext.b %s, %s", ShadowN, ScrAux)
	if isMem {
		e.Lf("store.b %s, (%s)", ScrAux, dstOp.Reg.IE64)
		return nil
	}
	e.Lf("and.q %s, %s, #%s", dstReg, dstReg, SizeInvMask(1))
	e.Lf("or.q %s, %s, %s", dstReg, dstReg, ScrAux)
	return nil
}

// emitNbcd lowers NBCD dst — dst = (0 - dst - X) BCD-adjusted.
func (c *Converter) emitNbcd(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("nbcd requires 1 operand")
	}
	dst, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if dst.Mode != AMDataReg {
		return fmt.Errorf("nbcd: only Dn supported in Phase B")
	}
	rd := dst.Reg.IE64
	e.Lf("and.l %s, %s, #$FF", ScrAux, rd)
	e.Lf("move.l %s, #0", ScrV1)
	e.Lf("sub.l %s, %s, %s", ScrAux, ScrV1, ScrAux)
	e.Lf("sub.l %s, %s, %s", ScrAux, ScrAux, ShadowX) // chain-in X
	// Adjust similar to SBCD.
	skipLow := e.NewLabel("nbcd_lo")
	e.Lf("and.l %s, %s, #$10", ScrV1, ScrAux)
	e.Lf("beqz %s, %s", ScrV1, skipLow)
	e.Lf("sub.l %s, %s, #6", ScrAux, ScrAux)
	e.Label(skipLow)
	skipHi := e.NewLabel("nbcd_hi")
	e.Lf("and.l %s, %s, #$100", ScrV1, ScrAux)
	e.Lf("move.l %s, #0", ShadowC)
	e.Lf("beqz %s, %s", ScrV1, skipHi)
	e.Lf("sub.l %s, %s, #$60", ScrAux, ScrAux)
	e.Lf("move.l %s, #1", ShadowC)
	e.Label(skipHi)
	e.Lf("and.l %s, %s, #$FF", ScrAux, ScrAux)
	e.Lf("or.l %s, %s, %s", ShadowZ, ShadowZ, ScrAux)
	e.Lf("move.l %s, %s", ShadowX, ShadowC)
	e.Lf("sext.b %s, %s", ShadowN, ScrAux)
	e.Lf("and.q %s, %s, #%s", rd, rd, SizeInvMask(1))
	e.Lf("or.q %s, %s, %s", rd, rd, ScrAux)
	return nil
}

// =====================================================================
// ADDX / SUBX / NEGX — X-chain-in arithmetic
// =====================================================================
//
// m68k ADDX/SUBX/NEGX consume the X (extend) flag as a chain-in carry/borrow.
// Producers update ShadowX at emit.go:69 / ccr_shadow.go:72; these emitters
// are the missing consumer side.
//
// Operand forms:
//   ADDX.<sz> Dy,Dx          SUBX.<sz> Dy,Dx          NEGX.<sz> Dn
//   ADDX.<sz> -(Ay),-(Ax)    SUBX.<sz> -(Ay),-(Ax)    NEGX.<sz> <mem>  (Dn-only here)
//
// Flag semantics differ from ABCD: V is **defined** (signed overflow), not
// undefined. Z is sticky (cleared only when result nonzero — preserve prior
// value otherwise). N reflects masked sign. X := C after the op.
//
// Lowering computes the full 64-bit (dst + src + X) or (dst - src - X) into
// ScrAux, then extracts C from bit `8*size` of the unmasked sum/diff and V
// from a sign-bit XOR mask using ShadowTmp1 / ShadowTmp2.

// addxLoadOperands resolves ADDX/SUBX operands at the requested size.
//
// For Dn,Dn: returns the IE64 guest-mapped register names and isMem=false.
// For -(Ay),-(Ax): emits size-aware predec + load.<sz> into ScrV1 (src) and
// ScrAux (dst), returns ScrV1 / ScrAux as the value carriers and isMem=true
// (caller must store.<sz> the result back at dst.Reg.IE64).
//
// dstOp is returned so the caller can reach dst.Reg.IE64 for the writeback.
func (c *Converter) addxLoadOperands(e *Emit, l Line, size int) (srcReg, dstReg string, isMem bool, dstOp Operand, err error) {
	if len(l.Operands) != 2 {
		err = fmt.Errorf("%s requires 2 operands", l.Mnemonic)
		return
	}
	src, e1 := ParseOperand(l.Operands[0])
	if e1 != nil {
		err = e1
		return
	}
	dst, e2 := ParseOperand(l.Operands[1])
	if e2 != nil {
		err = e2
		return
	}
	if src.Mode == AMDataReg && dst.Mode == AMDataReg {
		return src.Reg.IE64, dst.Reg.IE64, false, dst, nil
	}
	if src.Mode == AMPreDec && dst.Mode == AMPreDec {
		szIE := IE64Size(size)
		e.Lf("sub.l %s, %s, #%d", src.Reg.IE64, src.Reg.IE64, size)
		e.Lf("load%s %s, (%s)", szIE, ScrV1, src.Reg.IE64)
		e.Lf("sub.l %s, %s, #%d", dst.Reg.IE64, dst.Reg.IE64, size)
		e.Lf("load%s %s, (%s)", szIE, ScrAux, dst.Reg.IE64)
		return ScrV1, ScrAux, true, dst, nil
	}
	err = fmt.Errorf("%s: unsupported operand combination", l.Mnemonic)
	return
}

// emitAddxSubx lowers ADDX (isAdd=true) or SUBX (isAdd=false) at the size
// in l.Size (defaults to .w if absent).
func (c *Converter) emitAddxSubx(e *Emit, l Line, isAdd bool) error {
	size := SizeBytes(l.Size)
	if size == 0 {
		size = 2
	}
	srcCarrier, dstCarrier, isMem, dstOp, err := c.addxLoadOperands(e, l, size)
	if err != nil {
		return err
	}
	mask := SizeMask(size)
	bitN := size * 8
	signBit := bitN - 1

	// Stage src and pre-op dst at width into ShadowTmp scratches we own for
	// the full shadow-update window. ScrV1/ScrAux may collide with the
	// memory-form holders above, so route through ShadowTmp1 (src masked)
	// and ShadowSnap (dst masked, pre-op snapshot for V).
	e.Lf("and.l %s, %s, #%s", ShadowTmp1, srcCarrier, mask)
	e.Lf("and.l %s, %s, #%s", ShadowSnap, dstCarrier, mask)

	// Full-width result = dst ± src ± X. Use ScrAux to hold the unmasked
	// 64-bit value (carry/borrow lives in bit `bitN`).
	if isAdd {
		e.Lf("add.q %s, %s, %s", ScrAux, ShadowSnap, ShadowTmp1)
		e.Lf("add.q %s, %s, %s", ScrAux, ScrAux, ShadowX)
	} else {
		e.Lf("sub.q %s, %s, %s", ScrAux, ShadowSnap, ShadowTmp1)
		e.Lf("sub.q %s, %s, %s", ScrAux, ScrAux, ShadowX)
	}

	// C = bit `bitN` of unmasked result.
	e.Lf("lsr.q %s, %s, #%d", ShadowC, ScrAux, bitN)
	e.Lf("and.q %s, %s, #1", ShadowC, ShadowC)

	// V = signed-overflow at the width sign bit.
	//   ADD: V = NOT(d XOR s) AND (d XOR r), sign bit.
	//   SUB: V = (d XOR s) AND (d XOR r), sign bit.
	// d = ShadowSnap (already width-masked), s = ShadowTmp1, r = ScrAux masked.
	// Use ShadowTmp2 to hold (d XOR r), ShadowV as workspace.
	e.Lf("and.l %s, %s, #%s", ShadowTmp2, ScrAux, mask) // masked result
	e.Lf("eor.q %s, %s, %s", ShadowTmp2, ShadowTmp2, ShadowSnap) // d XOR r
	e.Lf("eor.q %s, %s, %s", ShadowV, ShadowSnap, ShadowTmp1)    // d XOR s
	if isAdd {
		e.Lf("not.q %s, %s", ShadowV, ShadowV) // NOT(d XOR s)
	}
	e.Lf("and.q %s, %s, %s", ShadowV, ShadowV, ShadowTmp2)
	e.Lf("lsr.q %s, %s, #%d", ShadowV, ShadowV, signBit)
	e.Lf("and.q %s, %s, #1", ShadowV, ShadowV)

	// Mask the result down to width for writeback + N/Z shadows.
	e.Lf("and.l %s, %s, #%s", ScrAux, ScrAux, mask)

	// Sticky Z: r25 stays nonzero if any prior op in the chain produced
	// nonzero, or if this op did. (r25 == 0 ⇔ m68k Z=1.)
	e.Lf("or.l %s, %s, %s", ShadowZ, ShadowZ, ScrAux)

	// N: sign-extend masked result.
	if size == 4 {
		e.Lf("sext.l %s, %s", ShadowN, ScrAux)
	} else {
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowN, ScrAux)
	}

	// X := C.
	e.Lf("move.l %s, %s", ShadowX, ShadowC)

	// Writeback.
	if isMem {
		e.Lf("store%s %s, (%s)", IE64Size(size), ScrAux, dstOp.Reg.IE64)
		return nil
	}
	// Dn dst — partial-update merge.
	e.Lf("and.q %s, %s, #%s", dstCarrier, dstCarrier, SizeInvMask(size))
	e.Lf("or.q %s, %s, %s", dstCarrier, dstCarrier, ScrAux)
	return nil
}

// emitNegx lowers NEGX.<sz> Dn — dst = 0 - dst - X with full N/Z/C/V shadows
// per the SUBX rule with src treated as 0. Memory destinations fall back to
// "unsupported" until needed (matches the NBCD pattern at phase_b.go:202).
func (c *Converter) emitNegx(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("negx requires 1 operand")
	}
	dst, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if dst.Mode != AMDataReg {
		return fmt.Errorf("negx: only Dn supported in Phase B")
	}
	size := SizeBytes(l.Size)
	if size == 0 {
		size = 2
	}
	mask := SizeMask(size)
	bitN := size * 8
	signBit := bitN - 1
	rd := dst.Reg.IE64

	// Snapshot pre-op dst at width.
	e.Lf("and.l %s, %s, #%s", ShadowSnap, rd, mask)
	// src is 0 for NEGX.
	e.Lf("move.l %s, #0", ShadowTmp1)

	// Full-width result = 0 - dst - X.
	e.Lf("sub.q %s, %s, %s", ScrAux, ShadowTmp1, ShadowSnap)
	e.Lf("sub.q %s, %s, %s", ScrAux, ScrAux, ShadowX)

	// C — borrow at bit `bitN`. For 0 - dst - X this is 1 iff (dst | X) nonzero.
	// Use the standard "lsr.q bitN, and #1" extraction.
	e.Lf("lsr.q %s, %s, #%d", ShadowC, ScrAux, bitN)
	e.Lf("and.q %s, %s, #1", ShadowC, ShadowC)

	// V: SUB form with src=0 — (d XOR 0) AND (d XOR r) = d AND (d XOR r).
	e.Lf("and.l %s, %s, #%s", ShadowTmp2, ScrAux, mask)
	e.Lf("eor.q %s, %s, %s", ShadowTmp2, ShadowTmp2, ShadowSnap) // d XOR r
	e.Lf("and.q %s, %s, %s", ShadowV, ShadowSnap, ShadowTmp2)
	e.Lf("lsr.q %s, %s, #%d", ShadowV, ShadowV, signBit)
	e.Lf("and.q %s, %s, #1", ShadowV, ShadowV)

	// Mask, sticky Z, N, X := C.
	e.Lf("and.l %s, %s, #%s", ScrAux, ScrAux, mask)
	e.Lf("or.l %s, %s, %s", ShadowZ, ShadowZ, ScrAux)
	if size == 4 {
		e.Lf("sext.l %s, %s", ShadowN, ScrAux)
	} else {
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowN, ScrAux)
	}
	e.Lf("move.l %s, %s", ShadowX, ShadowC)

	// Partial-update merge.
	e.Lf("and.q %s, %s, #%s", rd, rd, SizeInvMask(size))
	e.Lf("or.q %s, %s, %s", rd, rd, ScrAux)
	return nil
}

// =====================================================================
// ROXL / ROXR — rotate through X-extend ((width+1)-bit rotate)
// =====================================================================
//
// Operand forms:
//   ROXL.<sz> #data,Dn       data ∈ 1..8
//   ROXL.<sz> Dx,Dn          count = Dx mod 64
//   ROXL.W <ea>              memory single-bit rotate — DEFERRED, returns error
// (and the ROXR mirror image of each.)
//
// Semantics:
//   - X participates as the (width+1)th bit. X := last bit shifted out.
//   - C := X (after the rotate completes).
//   - count=0 → operand unchanged, but C := X (X stays). Z is NOT sticky for
//     ROX: standard result-based "result==0 → Z=1" semantic.
//   - V := 0 always.

func (c *Converter) emitRox(e *Emit, l Line, isLeft bool) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s: memory single-bit form not yet supported", l.Mnemonic)
	}
	cnt, e1 := ParseOperand(l.Operands[0])
	if e1 != nil {
		return e1
	}
	dst, e2 := ParseOperand(l.Operands[1])
	if e2 != nil {
		return e2
	}
	if dst.Mode != AMDataReg {
		return fmt.Errorf("%s: destination must be Dn (Phase B)", l.Mnemonic)
	}
	size := SizeBytes(l.Size)
	if size == 0 {
		size = 2
	}
	mask := SizeMask(size)
	width := size * 8
	rd := dst.Reg.IE64

	// Materialise count into ScrAux. Reg form masks mod 64; #imm is constant.
	switch cnt.Mode {
	case AMImmediate:
		e.Lf("move.l %s, #%s", ScrAux, cnt.Imm)
	case AMDataReg:
		e.Lf("and.l %s, %s, #63", ScrAux, cnt.Reg.IE64)
	default:
		return fmt.Errorf("%s: count must be Dn or #imm", l.Mnemonic)
	}

	// ShadowTmp1 = working X (chain), ShadowTmp2 = working operand (masked).
	e.Lf("move.l %s, %s", ShadowTmp1, ShadowX)
	e.Lf("and.l %s, %s, #%s", ShadowTmp2, rd, mask)

	head := e.NewLabel("rox_head")
	end := e.NewLabel("rox_end")
	e.Label(head)
	e.Lf("beqz %s, %s", ScrAux, end)
	if isLeft {
		// newX = bit (width-1) of operand
		e.Lf("lsr.l %s, %s, #%d", ScrV1, ShadowTmp2, width-1)
		e.Lf("and.l %s, %s, #1", ScrV1, ScrV1)
		// op = (op << 1) | oldX
		e.Lf("lsl.l %s, %s, #1", ShadowTmp2, ShadowTmp2)
		e.Lf("or.l %s, %s, %s", ShadowTmp2, ShadowTmp2, ShadowTmp1)
	} else {
		// newX = bit 0 of operand
		e.Lf("and.l %s, %s, #1", ScrV1, ShadowTmp2)
		// op = (op >> 1) | (oldX << (width-1))
		e.Lf("lsr.l %s, %s, #1", ShadowTmp2, ShadowTmp2)
		e.Lf("lsl.l %s, %s, #%d", ScrV2, ShadowTmp1, width-1)
		e.Lf("or.l %s, %s, %s", ShadowTmp2, ShadowTmp2, ScrV2)
	}
	e.Lf("and.l %s, %s, #%s", ShadowTmp2, ShadowTmp2, mask)
	e.Lf("move.l %s, %s", ShadowTmp1, ScrV1)
	e.Lf("sub.l %s, %s, #1", ScrAux, ScrAux)
	e.Lf("bra %s", head)
	e.Label(end)

	// C := final X, X := final X (chain forward).
	e.Lf("move.l %s, %s", ShadowC, ShadowTmp1)
	e.Lf("move.l %s, %s", ShadowX, ShadowTmp1)
	// V := 0.
	e.Lf("move.l %s, #0", ShadowV)
	// N := sign of result.
	if size == 4 {
		e.Lf("sext.l %s, %s", ShadowN, ShadowTmp2)
	} else {
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowN, ShadowTmp2)
	}
	// Z := result (r25 nonzero ⇔ m68k Z=0). Non-sticky.
	e.Lf("move.l %s, %s", ShadowZ, ShadowTmp2)
	// Partial-update merge.
	e.Lf("and.q %s, %s, #%s", rd, rd, SizeInvMask(size))
	e.Lf("or.q %s, %s, %s", rd, rd, ShadowTmp2)
	return nil
}

// =====================================================================
// PACK / UNPK
// =====================================================================
//
// PACK src,dst,#adj: src and dst must both be Dn or both be -(An).
//   raw = (src + adj) & $FFFF
//   packed = ((raw >> 4) & $F0) | (raw & $0F)
//   write low byte of packed to dst (byte-wide write)
//
// UNPK src,dst,#adj: opposite.
//   raw_byte = (src) & $FF
//   spread = ((raw_byte & $F0) << 4) | (raw_byte & $0F)
//   word = spread + adj (16-bit)
//   write 2 bytes of word to dst (high then low for -(An))

func (c *Converter) emitPack(e *Emit, l Line) error {
	if len(l.Operands) != 3 {
		return fmt.Errorf("pack requires 3 operands")
	}
	src, _ := ParseOperand(l.Operands[0])
	dst, _ := ParseOperand(l.Operands[1])
	adj, err := ParseOperand(l.Operands[2])
	if err != nil {
		return err
	}
	if adj.Mode != AMImmediate {
		return fmt.Errorf("pack: adjustment must be #imm")
	}
	if src.Mode == AMDataReg && dst.Mode == AMDataReg {
		// Read 16-bit src, add adj, repack.
		e.Lf("and.l %s, %s, #$FFFF", ScrAux, src.Reg.IE64)
		e.Lf("add.l %s, %s, #%s", ScrAux, ScrAux, adj.Imm)
		e.Lf("and.l %s, %s, #$FFFF", ScrAux, ScrAux)
		// packed = ((raw >> 4) & $F0) | (raw & $F)
		e.Lf("lsr.l %s, %s, #4", ScrV1, ScrAux)
		e.Lf("and.l %s, %s, #$F0", ScrV1, ScrV1)
		e.Lf("and.l %s, %s, #$F", ScrAux, ScrAux)
		e.Lf("or.l %s, %s, %s", ScrAux, ScrAux, ScrV1)
		// Write low byte of packed into dst.
		e.Lf("and.q %s, %s, #%s", dst.Reg.IE64, dst.Reg.IE64, SizeInvMask(1))
		e.Lf("or.q %s, %s, %s", dst.Reg.IE64, dst.Reg.IE64, ScrAux)
		return nil
	}
	if src.Mode == AMPreDec && dst.Mode == AMPreDec {
		// Two-byte read from src (predec).
		e.Lf("sub.l %s, %s, #1", src.Reg.IE64, src.Reg.IE64)
		e.Lf("load.b %s, (%s)", ScrAux, src.Reg.IE64) // low byte
		e.Lf("sub.l %s, %s, #1", src.Reg.IE64, src.Reg.IE64)
		e.Lf("load.b %s, (%s)", ScrV1, src.Reg.IE64) // high byte
		e.Lf("lsl.l %s, %s, #8", ScrV1, ScrV1)
		e.Lf("or.l %s, %s, %s", ScrAux, ScrAux, ScrV1)
		e.Lf("add.l %s, %s, #%s", ScrAux, ScrAux, adj.Imm)
		e.Lf("and.l %s, %s, #$FFFF", ScrAux, ScrAux)
		e.Lf("lsr.l %s, %s, #4", ScrV1, ScrAux)
		e.Lf("and.l %s, %s, #$F0", ScrV1, ScrV1)
		e.Lf("and.l %s, %s, #$F", ScrAux, ScrAux)
		e.Lf("or.l %s, %s, %s", ScrAux, ScrAux, ScrV1)
		// Predec dst, store byte.
		e.Lf("sub.l %s, %s, #1", dst.Reg.IE64, dst.Reg.IE64)
		e.Lf("store.b %s, (%s)", ScrAux, dst.Reg.IE64)
		return nil
	}
	return fmt.Errorf("pack: unsupported operand combination")
}

func (c *Converter) emitUnpk(e *Emit, l Line) error {
	if len(l.Operands) != 3 {
		return fmt.Errorf("unpk requires 3 operands")
	}
	src, _ := ParseOperand(l.Operands[0])
	dst, _ := ParseOperand(l.Operands[1])
	adj, err := ParseOperand(l.Operands[2])
	if err != nil {
		return err
	}
	if adj.Mode != AMImmediate {
		return fmt.Errorf("unpk: adjustment must be #imm")
	}
	if src.Mode == AMDataReg && dst.Mode == AMDataReg {
		e.Lf("and.l %s, %s, #$FF", ScrAux, src.Reg.IE64)
		// spread = ((b & $F0) << 4) | (b & $F)
		e.Lf("and.l %s, %s, #$F0", ScrV1, ScrAux)
		e.Lf("lsl.l %s, %s, #4", ScrV1, ScrV1)
		e.Lf("and.l %s, %s, #$F", ScrAux, ScrAux)
		e.Lf("or.l %s, %s, %s", ScrAux, ScrAux, ScrV1)
		e.Lf("add.l %s, %s, #%s", ScrAux, ScrAux, adj.Imm)
		e.Lf("and.l %s, %s, #$FFFF", ScrAux, ScrAux)
		// Write 16-bit result to dst.w preserving upper bits.
		e.Lf("and.q %s, %s, #%s", dst.Reg.IE64, dst.Reg.IE64, SizeInvMask(2))
		e.Lf("or.q %s, %s, %s", dst.Reg.IE64, dst.Reg.IE64, ScrAux)
		return nil
	}
	if src.Mode == AMPreDec && dst.Mode == AMPreDec {
		e.Lf("sub.l %s, %s, #1", src.Reg.IE64, src.Reg.IE64)
		e.Lf("load.b %s, (%s)", ScrAux, src.Reg.IE64)
		e.Lf("and.l %s, %s, #$F0", ScrV1, ScrAux)
		e.Lf("lsl.l %s, %s, #4", ScrV1, ScrV1)
		e.Lf("and.l %s, %s, #$F", ScrAux, ScrAux)
		e.Lf("or.l %s, %s, %s", ScrAux, ScrAux, ScrV1)
		e.Lf("add.l %s, %s, #%s", ScrAux, ScrAux, adj.Imm)
		e.Lf("and.l %s, %s, #$FFFF", ScrAux, ScrAux)
		// Write low byte then high byte (predec).
		e.Lf("sub.l %s, %s, #1", dst.Reg.IE64, dst.Reg.IE64)
		e.Lf("store.b %s, (%s)", ScrAux, dst.Reg.IE64)
		e.Lf("lsr.l %s, %s, #8", ScrAux, ScrAux)
		e.Lf("sub.l %s, %s, #1", dst.Reg.IE64, dst.Reg.IE64)
		e.Lf("store.b %s, (%s)", ScrAux, dst.Reg.IE64)
		return nil
	}
	return fmt.Errorf("unpk: unsupported operand combination")
}

// =====================================================================
// CAS (non-atomic fallback, per plan §"Risks #X")
// =====================================================================
//
// CAS Dc,Du,<ea>:  if (ea) == Dc:  (ea) := Du  (Z=1)
//                  else:           Dc := (ea)  (Z=0)
//
// IE64 has no atomic primitive yet; fallback as plain load-cmp-store-or-update.
func (c *Converter) emitCas(e *Emit, l Line) error {
	if len(l.Operands) != 3 {
		return fmt.Errorf("cas requires 3 operands")
	}
	dc, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	du, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	ea, err := ParseOperand(l.Operands[2])
	if err != nil {
		return err
	}
	if dc.Mode != AMDataReg || du.Mode != AMDataReg {
		return fmt.Errorf("cas: Dc and Du must be Dn")
	}
	size := SizeBytes(l.Size)
	if size == 0 {
		size = 4
	}
	szIE := IE64Size(size)
	// Load (ea) → ScrV2.
	if err := c.emitEABase(e, ea, ScrEA); err != nil {
		return err
	}
	e.Lf("load%s %s, (%s)", szIE, ScrV2, ScrEA)
	notEq := e.NewLabel("cas_neq")
	done := e.NewLabel("cas_done")
	// Compare against Dc (masked at width).
	if size == 4 {
		e.Lf("bne %s, %s, %s", ScrV2, dc.Reg.IE64, notEq)
	} else {
		e.Lf("and.l %s, %s, #%s", ScrV1, dc.Reg.IE64, SizeMask(size))
		e.Lf("bne %s, %s, %s", ScrV2, ScrV1, notEq)
	}
	// Equal: store Du → (ea); set Z.
	e.Lf("store%s %s, (%s)", szIE, du.Reg.IE64, ScrEA)
	e.Lf("move.l %s, #0", ShadowZ)
	e.Lf("bra %s", done)
	e.Label(notEq)
	// Not equal: Dc := (ea); clear Z.
	if size == 4 {
		e.Lf("move.l %s, %s", dc.Reg.IE64, ScrV2)
	} else {
		e.Lf("and.q %s, %s, #%s", dc.Reg.IE64, dc.Reg.IE64, SizeInvMask(size))
		e.Lf("or.q %s, %s, %s", dc.Reg.IE64, dc.Reg.IE64, ScrV2)
	}
	e.Lf("move.l %s, #1", ShadowZ)
	e.Label(done)
	return nil
}

// =====================================================================
// CAS2 — non-atomic dual-address compare-and-swap fallback
// =====================================================================
//
// CAS2.<sz> Dc1:Dc2,Du1:Du2,(Rn1):(Rn2)
//
// Sequential load-cmp-store-or-update on each of the two addresses. No
// atomicity — multi-context guests will observe lost updates. Mirrors the
// existing single-CAS fallback (`emitCas` above).

// splitColonPair splits "lhs:rhs" into two trimmed parts.
func splitColonPair(s string) (lhs, rhs string, err error) {
	i := strings.Index(s, ":")
	if i < 0 {
		return "", "", fmt.Errorf("expected Lhs:Rhs, got %q", s)
	}
	return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), nil
}

func (c *Converter) emitCas2(e *Emit, l Line) error {
	if len(l.Operands) != 3 {
		return fmt.Errorf("cas2 requires 3 colon-pair operands")
	}
	dc1Tok, dc2Tok, err := splitColonPair(l.Operands[0])
	if err != nil {
		return err
	}
	du1Tok, du2Tok, err := splitColonPair(l.Operands[1])
	if err != nil {
		return err
	}
	ea1Tok, ea2Tok, err := splitColonPair(l.Operands[2])
	if err != nil {
		return err
	}
	dc1, err := ParseOperand(dc1Tok)
	if err != nil {
		return err
	}
	dc2, err := ParseOperand(dc2Tok)
	if err != nil {
		return err
	}
	du1, err := ParseOperand(du1Tok)
	if err != nil {
		return err
	}
	du2, err := ParseOperand(du2Tok)
	if err != nil {
		return err
	}
	ea1, err := ParseOperand(ea1Tok)
	if err != nil {
		return err
	}
	ea2, err := ParseOperand(ea2Tok)
	if err != nil {
		return err
	}
	if dc1.Mode != AMDataReg || dc2.Mode != AMDataReg ||
		du1.Mode != AMDataReg || du2.Mode != AMDataReg {
		return fmt.Errorf("cas2: Dc/Du operands must be Dn")
	}
	if ea1.Mode != AMIndirect || ea2.Mode != AMIndirect {
		return fmt.Errorf("cas2: address operands must be (Rn)")
	}
	size := SizeBytes(l.Size)
	if size == 0 {
		size = 4
	}
	szIE := IE64Size(size)
	mask := SizeMask(size)
	invMask := SizeInvMask(size)

	e.L("; m68kto64: CAS2 non-atomic fallback (no IE64 dual-address atomic primitive)")

	// Load both (Rn) values: first into ScrV1, second into ScrV2.
	e.Lf("load%s %s, (%s)", szIE, ScrV1, ea1.Reg.IE64)
	e.Lf("load%s %s, (%s)", szIE, ScrV2, ea2.Reg.IE64)

	notEq := e.NewLabel("cas2_neq")
	done := e.NewLabel("cas2_done")

	// Compare both EAs against Dc1/Dc2 (size-masked).
	if size == 4 {
		e.Lf("bne %s, %s, %s", ScrV1, dc1.Reg.IE64, notEq)
		e.Lf("bne %s, %s, %s", ScrV2, dc2.Reg.IE64, notEq)
	} else {
		e.Lf("and.l %s, %s, #%s", ScrAux, dc1.Reg.IE64, mask)
		e.Lf("bne %s, %s, %s", ScrV1, ScrAux, notEq)
		e.Lf("and.l %s, %s, #%s", ScrAux, dc2.Reg.IE64, mask)
		e.Lf("bne %s, %s, %s", ScrV2, ScrAux, notEq)
	}
	// Both equal: store Du1 → (Rn1), Du2 → (Rn2); Z := 1.
	e.Lf("store%s %s, (%s)", szIE, du1.Reg.IE64, ea1.Reg.IE64)
	e.Lf("store%s %s, (%s)", szIE, du2.Reg.IE64, ea2.Reg.IE64)
	e.Lf("move.l %s, #0", ShadowZ) // r25==0 ⇔ Z=1
	e.Lf("bra %s", done)

	e.Label(notEq)
	// Mismatch: Dc1 := (Rn1), Dc2 := (Rn2); Z := 0.
	if size == 4 {
		e.Lf("move.l %s, %s", dc1.Reg.IE64, ScrV1)
		e.Lf("move.l %s, %s", dc2.Reg.IE64, ScrV2)
	} else {
		e.Lf("and.q %s, %s, #%s", dc1.Reg.IE64, dc1.Reg.IE64, invMask)
		e.Lf("or.q %s, %s, %s", dc1.Reg.IE64, dc1.Reg.IE64, ScrV1)
		e.Lf("and.q %s, %s, #%s", dc2.Reg.IE64, dc2.Reg.IE64, invMask)
		e.Lf("or.q %s, %s, %s", dc2.Reg.IE64, dc2.Reg.IE64, ScrV2)
	}
	e.Lf("move.l %s, #1", ShadowZ) // r25!=0 ⇔ Z=0
	e.Label(done)
	return nil
}

// =====================================================================
// Bit-field ops (Dn-only forms in Phase B)
// =====================================================================
//
// Form: BFxxx Dn{#off:#wid}[, Dd]
// Memory bit-fields are deferred — emit ; ERROR.

// bitfieldOperand abstracts a Dn or memory bit-field destination/source. For
// Dn forms the regName is set; for memory forms the eaOp is set instead.
type bitfieldOperand struct {
	isReg   bool
	regName string  // when isReg
	eaOp    Operand // when !isReg — pre-parsed memory operand
	off     string
	wid     string
}

// parseBitfieldOperand parses "Dn{#off:#wid}" or "<ea>{#off:#wid}".
//
// Limitation: memory forms support only single-32-bit-word access, i.e.
// off+wid must fit in 32 bits.
func parseBitfieldOperand(s string) (bitfieldOperand, error) {
	bopen := strings.Index(s, "{")
	bclose := strings.Index(s, "}")
	if bopen < 0 || bclose < 0 || bclose < bopen {
		return bitfieldOperand{}, fmt.Errorf("expected Dn{#off:#wid} or <ea>{#off:#wid}")
	}
	head := strings.TrimSpace(s[:bopen])
	field := strings.TrimSpace(s[bopen+1 : bclose])
	colon := strings.Index(field, ":")
	if colon < 0 {
		return bitfieldOperand{}, fmt.Errorf("bit-field expects {#off:#wid}")
	}
	off := strings.TrimPrefix(strings.TrimSpace(field[:colon]), "#")
	wid := strings.TrimPrefix(strings.TrimSpace(field[colon+1:]), "#")
	out := bitfieldOperand{off: off, wid: wid}
	if r, ok := LookupRegister(head); ok && r.Class == RegData {
		out.isReg = true
		out.regName = r.IE64
		return out, nil
	}
	// Memory operand.
	op, err := ParseOperand(head)
	if err != nil {
		return out, fmt.Errorf("bf: bad addressing mode: %v", err)
	}
	if op.Mode != AMIndirect && op.Mode != AMDispAn && op.Mode != AMAbsW && op.Mode != AMAbsL {
		return out, fmt.Errorf("bf: memory bit-field supports (An), d(An), abs only (got %v)", op.Mode)
	}
	out.eaOp = op
	return out, nil
}

// loadBfMemWord materialises the EA of a memory bit-field into ScrEA and
// loads a 32-bit window at that address into ScrV2. Returns the EA reg
// (always ScrEA) and the value reg (always ScrV2).
func (c *Converter) loadBfMemWord(e *Emit, ea Operand) error {
	if err := c.emitEABase(e, ea, ScrEA); err != nil {
		return err
	}
	e.Lf("load.l %s, (%s)", ScrV2, ScrEA)
	return nil
}

// storeBfMemWord stores the 32-bit ScrV2 back to the address in ScrEA.
func (c *Converter) storeBfMemWord(e *Emit) {
	e.Lf("store.l %s, (%s)", ScrV2, ScrEA)
}

// emitBfins lowers BFINS Dn,<dst>{#off:#wid} for both Dn and memory dst.
func (c *Converter) emitBfins(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("bfins requires 2 operands")
	}
	src, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if src.Mode != AMDataReg {
		return fmt.Errorf("bfins: source must be Dn")
	}
	bf, err := parseBitfieldOperand(l.Operands[1])
	if err != nil {
		return err
	}
	if bf.isReg {
		c.emitBfShiftMask(e, bf.regName, src.Reg.IE64, bf.off, bf.wid, "ins")
		return nil
	}
	// parseBitfieldOperand restricts memory bf modes to ones emitEABase
	// always accepts, so loadBfMemWord cannot fail here.
	_ = c.loadBfMemWord(e, bf.eaOp)
	c.emitBfShiftMask(e, ScrV2, src.Reg.IE64, bf.off, bf.wid, "ins")
	c.storeBfMemWord(e)
	return nil
}

// emitBfModify lowers BFCLR / BFSET / BFCHG <dst>{#off:#wid}.
func (c *Converter) emitBfModify(e *Emit, l Line, kind string) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("bf%s requires 1 operand", kind)
	}
	bf, err := parseBitfieldOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if bf.isReg {
		c.emitBfShiftMask(e, bf.regName, "", bf.off, bf.wid, kind)
		return nil
	}
	// parseBitfieldOperand restricts memory bf modes to ones emitEABase
	// always accepts, so loadBfMemWord cannot fail here.
	_ = c.loadBfMemWord(e, bf.eaOp)
	c.emitBfShiftMask(e, ScrV2, "", bf.off, bf.wid, kind)
	c.storeBfMemWord(e)
	return nil
}

// emitBftst lowers BFTST <src>{#off:#wid} — sets shadow N/Z from extracted field.
func (c *Converter) emitBftst(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("bftst requires 1 operand")
	}
	bf, err := parseBitfieldOperand(l.Operands[0])
	if err != nil {
		return err
	}
	srcReg := bf.regName
	if !bf.isReg {
		_ = c.loadBfMemWord(e, bf.eaOp) // pre-validated; cannot fail
		srcReg = ScrV2
	}
	e.Lf("move.l %s, #32", ScrV1)
	e.Lf("sub.l %s, %s, #%s", ScrV1, ScrV1, bf.off)
	e.Lf("sub.l %s, %s, #%s", ScrV1, ScrV1, bf.wid)
	e.Lf("lsr.l %s, %s, %s", ScrAux, srcReg, ScrV1)
	e.Lf("move.l %s, #1", ScrV2)
	e.Lf("lsl.l %s, %s, #%s", ScrV2, ScrV2, bf.wid)
	e.Lf("sub.l %s, %s, #1", ScrV2, ScrV2)
	e.Lf("and.l %s, %s, %s", ScrAux, ScrAux, ScrV2)
	c.emitShadowsForLogical(e, ScrAux, 4)
	return nil
}

// emitBfffo lowers BFFFO <src>{#off:#wid},Dd.
func (c *Converter) emitBfffo(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("bfffo requires 2 operands")
	}
	bf, err := parseBitfieldOperand(l.Operands[0])
	if err != nil {
		return err
	}
	dst, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	if dst.Mode != AMDataReg {
		return fmt.Errorf("bfffo: destination must be Dn")
	}
	srcReg := bf.regName
	if !bf.isReg {
		_ = c.loadBfMemWord(e, bf.eaOp) // pre-validated; cannot fail
		srcReg = ScrV2
	}
	e.Lf("move.l %s, #32", ScrV1)
	e.Lf("sub.l %s, %s, #%s", ScrV1, ScrV1, bf.off)
	e.Lf("sub.l %s, %s, #%s", ScrV1, ScrV1, bf.wid)
	e.Lf("lsr.l %s, %s, %s", ScrAux, srcReg, ScrV1)
	e.Lf("move.l %s, #1", ScrV2)
	e.Lf("lsl.l %s, %s, #%s", ScrV2, ScrV2, bf.wid)
	e.Lf("sub.l %s, %s, #1", ScrV2, ScrV2)
	e.Lf("and.l %s, %s, %s", ScrAux, ScrAux, ScrV2)
	e.Lf("clz %s, %s", dst.Reg.IE64, ScrAux)
	e.Lf("move.l %s, #32", ScrV1)
	e.Lf("sub.l %s, %s, #%s", ScrV1, ScrV1, bf.wid)
	e.Lf("sub.l %s, %s, %s", dst.Reg.IE64, dst.Reg.IE64, ScrV1)
	e.Lf("add.l %s, %s, #%s", dst.Reg.IE64, dst.Reg.IE64, bf.off)
	c.emitShadowsForLogical(e, ScrAux, 4)
	return nil
}

// emitBfShiftMask is the shared shift+mask kernel for BFINS/BFCLR/BFSET/BFCHG
// with Dn destination.
//
//   shift = 32 - off - wid
//   mask  = ((1 << wid) - 1) << shift
//   ins  : dst = (dst & ~mask) | ((src & ((1<<wid)-1)) << shift)
//   clr  : dst = dst & ~mask
//   set  : dst = dst | mask
//   chg  : dst = dst ^ mask
func (c *Converter) emitBfShiftMask(e *Emit, dstReg, srcReg, off, wid, kind string) {
	// shift → ScrV1
	e.Lf("move.l %s, #32", ScrV1)
	e.Lf("sub.l %s, %s, #%s", ScrV1, ScrV1, off)
	e.Lf("sub.l %s, %s, #%s", ScrV1, ScrV1, wid)
	// mask → ScrV2 = ((1<<wid)-1) << shift
	e.Lf("move.l %s, #1", ScrV2)
	e.Lf("lsl.l %s, %s, #%s", ScrV2, ScrV2, wid)
	e.Lf("sub.l %s, %s, #1", ScrV2, ScrV2)
	e.Lf("lsl.l %s, %s, %s", ScrV2, ScrV2, ScrV1)
	switch kind {
	case "ins":
		// field = (src & ((1<<wid)-1)) << shift
		e.Lf("move.l %s, #1", ScrAux)
		e.Lf("lsl.l %s, %s, #%s", ScrAux, ScrAux, wid)
		e.Lf("sub.l %s, %s, #1", ScrAux, ScrAux)
		e.Lf("and.l %s, %s, %s", ScrAux, srcReg, ScrAux)
		e.Lf("lsl.l %s, %s, %s", ScrAux, ScrAux, ScrV1)
		e.Lf("not.l %s, %s", ScrV2, ScrV2)
		e.Lf("and.l %s, %s, %s", dstReg, dstReg, ScrV2)
		e.Lf("or.l %s, %s, %s", dstReg, dstReg, ScrAux)
	case "clr":
		e.Lf("not.l %s, %s", ScrV2, ScrV2)
		e.Lf("and.l %s, %s, %s", dstReg, dstReg, ScrV2)
	case "set":
		e.Lf("or.l %s, %s, %s", dstReg, dstReg, ScrV2)
	case "chg":
		e.Lf("eor.l %s, %s, %s", dstReg, dstReg, ScrV2)
	}
	c.emitShadowsForLogical(e, dstReg, 4)
}
