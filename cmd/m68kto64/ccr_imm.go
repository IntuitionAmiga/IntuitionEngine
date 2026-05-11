package main

import (
	"fmt"
	"strings"
)

// Phase F3 — ANDI/ORI/EORI #imm,CCR/SR consumer.
//
// m68k spec: the only legal source for these forms is `#imm`. The
// transpiler now wires the shadow-CCR consumer so the previously-⚠
// status flips to ✅ (for CCR) / ⚠ (for SR — upper byte discarded).
//
// Lowering strategy: constant-immediate specialisation. Each shadow
// register's new value is computed at emit time from the known immediate
// bit. The result is the minimum amount of IE64 code needed for the
// requested boolean change.
//
// CCR low-byte layout:
//   bit 0 = C, bit 1 = V, bit 2 = Z, bit 3 = N, bit 4 = X
//
// Shadow contract recap (see ccr_shadow.go):
//   ShadowC / ShadowV / ShadowX : canonical 0/1
//   ShadowZ                     : inverted — nonzero ⇔ Z=0
//   ShadowN                     : sign-extended — −1 ⇔ N=1, 0 ⇔ N=0

// tryEmitArithCCR inspects `l.Operands[1]` for AMCCR / AMSR. When matched
// and `l.Operands[0]` is `#imm`, emits the per-shadow update and returns
// (true, nil). Returns (false, nil) when not applicable so the caller
// falls through to the generic arith path.
func (c *Converter) tryEmitArithCCR(e *Emit, l Line, op string) (bool, error) {
	if len(l.Operands) != 2 {
		return false, nil
	}
	dstOp, err := ParseOperand(l.Operands[1])
	if err != nil {
		return false, nil
	}
	if dstOp.Mode != AMCCR && dstOp.Mode != AMSR {
		return false, nil
	}
	srcOp, err := ParseOperand(l.Operands[0])
	if err != nil {
		return true, err
	}
	if srcOp.Mode != AMImmediate {
		return true, fmt.Errorf("%s to CCR/SR requires immediate source, got %v", op, srcOp.Mode)
	}
	imm, err := parseImmediateInt(srcOp.Imm)
	if err != nil {
		return true, fmt.Errorf("%s to CCR/SR: cannot parse immediate %q: %v", op, srcOp.Imm, err)
	}
	if dstOp.Mode == AMSR {
		// Upper byte (trace bits T1/T0, supervisor S, IPL mask I2/I1/I0)
		// has no IE64 representation in the user-mode target. Discard
		// silently with a diagnostic; the low byte (CCR portion) still
		// applies normally.
		if imm>>8 != 0 || op == "and" && imm&0xFF00 != 0xFF00 {
			e.Lf("; m68kto64: %sI to SR upper byte discarded (trace/IPL/S bits not modelled)", strings.ToUpper(op))
		}
	}
	low := imm & 0xFF
	switch op {
	case "and":
		emitANDIShadowCCR(e, low)
	case "or":
		emitORIShadowCCR(e, low)
	case "eor":
		emitEORIShadowCCR(e, low)
	}
	return true, nil
}

// emitANDIShadowCCR — for each CCR bit cleared by the immediate
// (imm bit == 0), force the corresponding shadow to the "flag clear" state.
// Bits set in imm leave the shadow unchanged.
func emitANDIShadowCCR(e *Emit, imm int) {
	if imm&0x01 == 0 {
		e.Lf("move.l %s, #0", ShadowC)
	}
	if imm&0x02 == 0 {
		e.Lf("move.l %s, #0", ShadowV)
	}
	if imm&0x04 == 0 {
		// Force Z=0: ShadowZ := nonzero (canonical 1).
		e.Lf("move.l %s, #1", ShadowZ)
	}
	if imm&0x08 == 0 {
		e.Lf("move.l %s, #0", ShadowN)
	}
	if imm&0x10 == 0 {
		e.Lf("move.l %s, #0", ShadowX)
	}
}

// emitORIShadowCCR — for each CCR bit set by the immediate (imm bit == 1),
// force the shadow to the "flag set" state. Bits clear in imm leave the
// shadow unchanged.
func emitORIShadowCCR(e *Emit, imm int) {
	if imm&0x01 != 0 {
		e.Lf("move.l %s, #1", ShadowC)
	}
	if imm&0x02 != 0 {
		e.Lf("move.l %s, #1", ShadowV)
	}
	if imm&0x04 != 0 {
		// Force Z=1: ShadowZ := 0.
		e.Lf("move.l %s, #0", ShadowZ)
	}
	if imm&0x08 != 0 {
		// Force N=1: ShadowN := −1 (sign-extended).
		e.Lf("move.l %s, #-1", ShadowN)
	}
	if imm&0x10 != 0 {
		e.Lf("move.l %s, #1", ShadowX)
	}
}

// emitEORIShadowCCR — toggle each shadow whose imm bit is set. Per-shadow
// specialisation avoids a pack/unpack round-trip.
func emitEORIShadowCCR(e *Emit, imm int) {
	if imm&0x01 != 0 {
		e.Lf("eor.l %s, %s, #1", ShadowC, ShadowC)
	}
	if imm&0x02 != 0 {
		e.Lf("eor.l %s, %s, #1", ShadowV, ShadowV)
	}
	if imm&0x04 != 0 {
		emitToggleShadowZ(e)
	}
	if imm&0x08 != 0 {
		emitToggleShadowN(e)
	}
	if imm&0x10 != 0 {
		e.Lf("eor.l %s, %s, #1", ShadowX, ShadowX)
	}
}

// emitToggleShadowZ flips the m68k Z flag held in inverted form in
// ShadowZ. Encoding: ShadowZ nonzero ⇔ Z=0, ShadowZ==0 ⇔ Z=1.
// Toggle = re-encode the negation.
func emitToggleShadowZ(e *Emit) {
	// new ShadowZ = (ShadowZ == 0) ? 1 : 0
	// Implemented via: tmp = (ShadowZ == 0); ShadowZ = tmp
	// IE64 fallback (no branchless setcc available in this transpiler's
	// vocabulary): explicit beqz branch + sub-from-1 re-encode.
	swap := e.NewLabel("ccrz_was1")
	done := e.NewLabel("ccrz_done")
	e.Lf("beqz %s, %s", ShadowZ, swap)
	// ShadowZ was nonzero → Z was 0 → after flip Z is 1 → ShadowZ := 0
	e.Lf("move.l %s, #0", ShadowZ)
	e.Lf("bra %s", done)
	e.Label(swap)
	// ShadowZ was 0 → Z was 1 → after flip Z is 0 → ShadowZ := 1
	e.Lf("move.l %s, #1", ShadowZ)
	e.Label(done)
}

// emitToggleShadowN flips the m68k N flag held in sign-extended form in
// ShadowN. Encoding: ShadowN==−1 ⇔ N=1, ShadowN==0 ⇔ N=0.
// Toggle via low-bit XOR then neg.q to re-sign-extend.
func emitToggleShadowN(e *Emit) {
	e.Lf("and.l %s, %s, #1", ShadowTmp1, ShadowN)
	e.Lf("eor.l %s, %s, #1", ShadowTmp1, ShadowTmp1)
	e.Lf("neg.q %s, %s", ShadowN, ShadowTmp1)
}

// parseImmediateInt extracts an integer value from an Operand.Imm string.
// Supports m68k-style hex ($XX), binary (%01), decimal, and bare-label
// fallback (rejected — F3 requires a numeric constant).
func parseImmediateInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	var n int
	var err error
	switch {
	case strings.HasPrefix(s, "$"):
		_, err = fmt.Sscanf(s[1:], "%x", &n)
	case strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X"):
		_, err = fmt.Sscanf(s[2:], "%x", &n)
	case strings.HasPrefix(s, "%"):
		_, err = fmt.Sscanf(s[1:], "%b", &n)
	default:
		_, err = fmt.Sscanf(s, "%d", &n)
	}
	if err != nil {
		return 0, err
	}
	return n, nil
}
