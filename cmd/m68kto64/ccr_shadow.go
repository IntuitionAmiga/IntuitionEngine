package main

import (
	"fmt"
	"strings"
)

// Phase A: shadow CCR maintenance.
//
// Shadow registers (m68k → IE64):
//
//   r24  N  shadow — sign-extended-to-64 last result. `bltz r24, L` ≡ BMI L,
//                    `bgez r24, L` ≡ BPL L.
//   r25  Z  shadow — width-masked last result. `beqz r25, L` ≡ BEQ L,
//                    `bnez r25, L` ≡ BNE L.
//   r26  C  shadow — 0/1 carry/borrow from last add/sub/cmp. `bnez r26, L`
//                    ≡ BCS L, `beqz r26, L` ≡ BCC L.
//   r27  V  shadow — 0/1 signed overflow from last add/sub/cmp.
//
// Each producer emits the shadow updates for the flags it writes (always-on,
// no liveness optimisation in Phase A). Unfused Bcc / DBcc / Scc consume
// the shadows. Adjacent CMP/TST + Bcc fuse still wins (Phase 3 path) and
// skips shadow updates entirely.

// ProducerFlags marks which CCR flags an instruction writes.
type ProducerFlags struct {
	N, Z, C, V bool
}

const (
	ShadowN = "r24"
	ShadowZ = "r25"
	ShadowC = "r26"
	ShadowV = "r27"
)

// =====================================================================
// Shadow update emitters
// =====================================================================

// emitShadowNZFromReg writes the N and Z shadows from a register that holds
// the operation's result at width `size` bytes.
//
//   r24 = sext.X result   (sign-extend to 64 bits)
//   r25 = result & mask   (width-masked, nonzero ⇔ Z=0)
func (c *Converter) emitShadowNZFromReg(e *Emit, resultReg string, size int) {
	if size <= 0 {
		size = 4
	}
	szIE := IE64Size(size)
	if size == 4 {
		// At .l, sext.l sign-extends bit 31 into the upper 32; the masked
		// form is just the .l value (upper 32 zeros) which `move.l` gives us.
		e.Lf("sext.l %s, %s", ShadowN, resultReg)
		e.Lf("move.l %s, %s", ShadowZ, resultReg)
		return
	}
	e.Lf("sext%s %s, %s", szIE, ShadowN, resultReg)
	e.Lf("and.l %s, %s, #%s", ShadowZ, resultReg, SizeMask(size))
}

// emitShadowClearC writes 0 into the C shadow.
func (c *Converter) emitShadowClearC(e *Emit) { e.Lf("move.l %s, #0", ShadowC) }

// emitShadowClearV writes 0 into the V shadow.
func (c *Converter) emitShadowClearV(e *Emit) { e.Lf("move.l %s, #0", ShadowV) }

// emitShadowCopyToX is invoked by add/sub/cmp/neg/shift after C is computed
// to mirror the value into the X (extend) shadow. m68k semantic: X := C for
// these ops. Logical ops (AND/OR/EOR/NOT/MOVE) skip this call.
func (c *Converter) emitShadowCopyToX(e *Emit) {
	e.Lf("move.l %s, %s", ShadowX, ShadowC)
}

// emitShadowSubCV computes shadow C and V for a SUB/CMP at width `size`.
//
// Inputs:
//   dstReg — register holding the m68k destination (minuend) at full IE64 width.
//   srcReg — register holding the source (subtrahend).
//   size   — operation width in bytes (1, 2, or 4).
//
// Method:
//   ud = dst & mask
//   us = src & mask
//   diff = ud - us  (64-bit)
//   C = bit `size*8` of diff   (set ⇔ borrow at width `size`)
//
//   sd = sext dst, ss = sext src, sr = sext result_truncated
//   V = ((sd XOR ss) AND (sd XOR sr)) >> (size*8 - 1) & 1
func (c *Converter) emitShadowSubCV(e *Emit, dstReg, srcReg string, size int) {
	mask := SizeMask(size)
	bitN := size * 8
	signBit := bitN - 1
	// C: zero-extend operands, full 64-bit subtract, extract bit `size*8`.
	e.Lf("and.l %s, %s, #%s", ShadowTmp1, dstReg, mask)
	e.Lf("and.l %s, %s, #%s", ShadowTmp2, srcReg, mask)
	e.Lf("sub.q %s, %s, %s", ShadowC, ShadowTmp1, ShadowTmp2)
	e.Lf("lsr.q %s, %s, #%d", ShadowC, ShadowC, bitN)
	e.Lf("and.q %s, %s, #1", ShadowC, ShadowC)
	// V: sign-extend dst, src, and truncated result; compare sign-overflow.
	if size == 4 {
		e.Lf("sext.l %s, %s", ShadowTmp1, dstReg)
		e.Lf("sext.l %s, %s", ShadowTmp2, srcReg)
	} else {
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowTmp1, dstReg)
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowTmp2, srcReg)
	}
	// dst XOR src (preserved in ShadowTmp1)
	e.Lf("eor.q %s, %s, %s", ShadowTmp1, ShadowTmp1, ShadowTmp2)
	// Compute truncated, sign-extended result into ShadowTmp2.
	if size == 4 {
		e.Lf("sext.l %s, %s", ShadowTmp2, dstReg)
	} else {
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowTmp2, dstReg)
	}
	// dst_signed - src_signed (truncated to width via sign-extend) — recompute
	// using fresh sext of operands.
	if size == 4 {
		e.Lf("sext.l %s, %s", ShadowV, srcReg)
	} else {
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowV, srcReg)
	}
	e.Lf("sub.q %s, %s, %s", ShadowV, ShadowTmp2, ShadowV)
	// Truncate diff to width, sign-extend back.
	e.Lf("and.l %s, %s, #%s", ShadowV, ShadowV, mask)
	if size == 4 {
		e.Lf("sext.l %s, %s", ShadowV, ShadowV)
	} else {
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowV, ShadowV)
	}
	// dst XOR result (in ShadowTmp2 ^ ShadowV)
	e.Lf("eor.q %s, %s, %s", ShadowTmp2, ShadowTmp2, ShadowV)
	// V = (dst XOR src) AND (dst XOR result), sign bit.
	e.Lf("and.q %s, %s, %s", ShadowV, ShadowTmp1, ShadowTmp2)
	e.Lf("lsr.q %s, %s, #%d", ShadowV, ShadowV, signBit)
	e.Lf("and.q %s, %s, #1", ShadowV, ShadowV)
}

// emitShadowAddCV computes shadow C and V for an ADD at width `size`.
//
//   ud = dst & mask, us = src & mask
//   sum = ud + us  (64-bit)
//   C = bit `size*8` of sum
//
//   V = (NOT(dst XOR src) AND (dst XOR result)) sign-bit
func (c *Converter) emitShadowAddCV(e *Emit, dstReg, srcReg string, size int) {
	mask := SizeMask(size)
	bitN := size * 8
	signBit := bitN - 1
	e.Lf("and.l %s, %s, #%s", ShadowTmp1, dstReg, mask)
	e.Lf("and.l %s, %s, #%s", ShadowTmp2, srcReg, mask)
	e.Lf("add.q %s, %s, %s", ShadowC, ShadowTmp1, ShadowTmp2)
	e.Lf("lsr.q %s, %s, #%d", ShadowC, ShadowC, bitN)
	e.Lf("and.q %s, %s, #1", ShadowC, ShadowC)
	// V
	if size == 4 {
		e.Lf("sext.l %s, %s", ShadowTmp1, dstReg)
		e.Lf("sext.l %s, %s", ShadowTmp2, srcReg)
	} else {
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowTmp1, dstReg)
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowTmp2, srcReg)
	}
	// NOT(dst XOR src) → ShadowTmp1
	e.Lf("eor.q %s, %s, %s", ShadowTmp1, ShadowTmp1, ShadowTmp2)
	e.Lf("not.q %s, %s", ShadowTmp1, ShadowTmp1)
	// result = sign-extended (dst+src) truncated to width.
	if size == 4 {
		e.Lf("sext.l %s, %s", ShadowTmp2, dstReg)
		e.Lf("sext.l %s, %s", ShadowV, srcReg)
	} else {
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowTmp2, dstReg)
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowV, srcReg)
	}
	e.Lf("add.q %s, %s, %s", ShadowV, ShadowTmp2, ShadowV)
	e.Lf("and.l %s, %s, #%s", ShadowV, ShadowV, mask)
	if size == 4 {
		e.Lf("sext.l %s, %s", ShadowV, ShadowV)
	} else {
		e.Lf("sext%s %s, %s", IE64Size(size), ShadowV, ShadowV)
	}
	// dst XOR result
	e.Lf("eor.q %s, %s, %s", ShadowTmp2, ShadowTmp2, ShadowV)
	// V = NOT(dst XOR src) AND (dst XOR result) >> signBit
	e.Lf("and.q %s, %s, %s", ShadowV, ShadowTmp1, ShadowTmp2)
	e.Lf("lsr.q %s, %s, #%d", ShadowV, ShadowV, signBit)
	e.Lf("and.q %s, %s, #1", ShadowV, ShadowV)
}

// emitShadowsForLogical writes shadows for AND/OR/EOR/NOT/MOVE/MOVEQ/EXT/SWAP/
// MULU/MULS/CLR style ops: N and Z from result; C and V cleared.
// Phase H: elide entirely when no downstream consumer reads N/Z/C/V.
func (c *Converter) emitShadowsForLogical(e *Emit, resultReg string, size int) {
	if !c.integerCCLive() {
		e.L("; m68kto64: logical shadow elided (no live consumer)")
		return
	}
	c.emitShadowNZFromReg(e, resultReg, size)
	c.emitShadowClearC(e)
	c.emitShadowClearV(e)
}

// shadowWantsCV reports whether a producer mnemonic computes meaningful C/V
// (and therefore needs an operand snapshot before the destructive write).
func shadowWantsCV(mnem string) bool {
	switch mnem {
	case "add", "addi", "addq", "sub", "subi", "subq", "cmp", "cmpi", "cmpa", "cmpm", "neg":
		return true
	}
	return false
}

// emitArithShadows emits the shadow-CCR updates for a producer at the end of
// emitArith. `resultReg` holds the operation's width-masked result; `preDst`
// the snapshot of the pre-op destination (only valid when shadowWantsCV);
// `srcReg` the post-load src register (or ScrV1 when an immediate was
// materialised). `size` is the operation width.
func (c *Converter) emitArithShadows(e *Emit, mnem string, resultReg, preDst, srcReg string, size int) {
	// Phase H: liveness-driven elision. When no downstream consumer reads
	// any of N/Z/C/V/X before the next producer overwrites them, skip the
	// full shadow update sequence (10-15 IE64 ops per arith op).
	if !c.integerCCLive() {
		e.L("; m68kto64: shadow elided (no live consumer)")
		return
	}
	switch mnem {
	case "add", "addi", "addq":
		c.emitShadowNZFromReg(e, resultReg, size)
		c.emitShadowAddCV(e, preDst, srcReg, size)
		c.emitShadowCopyToX(e)
	case "sub", "subi", "subq":
		c.emitShadowNZFromReg(e, resultReg, size)
		c.emitShadowSubCV(e, preDst, srcReg, size)
		c.emitShadowCopyToX(e)
	case "and", "andi", "or", "ori", "eor", "eori", "mulu", "muls", "divu", "divs":
		// Logical ops: N/Z from result, C/V cleared, X UNCHANGED.
		c.emitShadowsForLogical(e, resultReg, size)
	case "adda", "suba":
		// CCR not affected.
	}
}

// =====================================================================
// Standalone Bcc / DBcc / Scc lowering against shadows
// =====================================================================

// emitShadowBcc lowers an unfused Bcc (no immediately-preceding CMP/TST) by
// reading the shadow CCR registers.
func (c *Converter) emitShadowBcc(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("%s requires 1 operand", l.Mnemonic)
	}
	label := strings.TrimSpace(l.Operands[0])
	kind := bccKind(l.Mnemonic)
	switch kind {
	case "eq":
		e.Lf("beqz %s, %s", ShadowZ, label)
	case "ne":
		e.Lf("bnez %s, %s", ShadowZ, label)
	case "mi":
		e.Lf("bltz %s, %s", ShadowN, label)
	case "pl":
		e.Lf("bgez %s, %s", ShadowN, label)
	case "cs":
		e.Lf("bnez %s, %s", ShadowC, label)
	case "cc":
		e.Lf("beqz %s, %s", ShadowC, label)
	case "vs":
		e.Lf("bnez %s, %s", ShadowV, label)
	case "vc":
		e.Lf("beqz %s, %s", ShadowV, label)
	case "lt":
		// signed lt = N XOR V == 1 (after a SUB/CMP). r24 sign-bit XOR r27.
		// Easiest: extract sign of r24 (bit 63) into 0/1 in ScrV1, XOR with
		// r27, branch if nonzero.
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("bnez %s, %s", ScrV1, label)
	case "ge":
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("beqz %s, %s", ScrV1, label)
	case "gt":
		// gt = (Z=0) AND (N XOR V == 0)  i.e. !Z AND !lt
		// Compute: !Z = bnez r25; lt cond = NXORV
		// Combined: skipBranch if Z OR (N XOR V); else branch.
		skip := e.NewLabel("bgt_skip")
		e.Lf("beqz %s, %s", ShadowZ, skip) // Z=1 → skip
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("bnez %s, %s", ScrV1, skip)   // N XOR V = 1 (lt) → skip
		e.Lf("bra %s", label)              // Z=0 AND lt=0 → take branch
		e.Label(skip)
	case "le":
		// le = Z=1 OR (N XOR V == 1).
		// ShadowZ contract: r25 == 0 ⇔ m68k Z=1, so Z=1 → beqz r25.
		take := e.NewLabel("ble_take")
		e.Lf("beqz %s, %s", ShadowZ, take) // Z=1 → take
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("bnez %s, %s", ScrV1, take)
		skip := e.NewLabel("ble_skip")
		e.Lf("bra %s", skip)
		e.Label(take)
		e.Lf("bra %s", label)
		e.Label(skip)
	case "hi":
		// hi unsigned = (C=0) AND (Z=0). i.e. neither bit set.
		skip := e.NewLabel("bhi_skip")
		e.Lf("bnez %s, %s", ShadowC, skip)
		e.Lf("beqz %s, %s", ShadowZ, skip)
		e.Lf("bra %s", label)
		e.Label(skip)
	case "ls":
		// ls unsigned = C=1 OR Z=1. ShadowZ: r25 == 0 ⇔ Z=1 → beqz r25.
		take := e.NewLabel("bls_take")
		e.Lf("bnez %s, %s", ShadowC, take)
		e.Lf("beqz %s, %s", ShadowZ, take)
		skip := e.NewLabel("bls_skip")
		e.Lf("bra %s", skip)
		e.Label(take)
		e.Lf("bra %s", label)
		e.Label(skip)
	default:
		return fmt.Errorf("unsupported Bcc kind %q", kind)
	}
	return nil
}

// emitShadowDBcc lowers a DBcc with shadow-CCR condition test. m68k semantic
// for DBcc:
//   if cc TRUE  → fall through with Dn unchanged
//   else        → low word of Dn -= 1; if low word == -1 fall through; else
//                 branch to label.
func (c *Converter) emitShadowDBcc(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("dbcc requires 2 operands (Dn,L)")
	}
	dn, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if dn.Mode != AMDataReg {
		return fmt.Errorf("dbcc: first operand must be Dn")
	}
	label := strings.TrimSpace(l.Operands[1])
	rd := dn.Reg.IE64
	skip := e.NewLabel("dbcc_skip")
	// Emit the cc test → branch to `skip` (means "cc TRUE, skip decrement").
	if err := c.emitDBccConditionSkip(e, l.Mnemonic, skip); err != nil {
		return err
	}
	// Decrement low word, branch to label unless low word == $FFFF.
	e.Lf("and.l %s, %s, #$FFFF", ScrV1, rd)
	e.Lf("sub.l %s, %s, #1", ScrV1, ScrV1)
	e.Lf("and.l %s, %s, #$FFFF", ScrV1, ScrV1)
	e.Lf("and.q %s, %s, #%s", ScrV2, rd, SizeInvMask(2))
	e.Lf("or.q %s, %s, %s", rd, ScrV2, ScrV1)
	e.Lf("move.l %s, #$FFFF", ScrV2)
	e.Lf("bne %s, %s, %s", ScrV1, ScrV2, label)
	e.Label(skip)
	return nil
}

// emitDBccConditionSkip emits a branch to `skip` when the DBcc's condition
// is TRUE (per m68k: cc TRUE → fall through without decrement). Caller
// (emitShadowDBcc) only forwards conditional DBcc forms — DBRA/DBF go
// through emitDbra directly.
func (c *Converter) emitDBccConditionSkip(e *Emit, mnem, skip string) error {
	if mnem == "dbt" {
		// DBT: cc always TRUE → always skip.
		e.Lf("bra %s", skip)
		return nil
	}
	kind := bccKind(mnem)
	switch kind {
	case "eq":
		// DBEQ skip-when-cc-true: cc=true ⇔ Z=1 ⇔ r25==0 → beqz.
		e.Lf("beqz %s, %s", ShadowZ, skip)
	case "ne":
		// DBNE: cc=true ⇔ Z=0 ⇔ r25!=0 → bnez.
		e.Lf("bnez %s, %s", ShadowZ, skip)
	case "mi":
		e.Lf("bltz %s, %s", ShadowN, skip)
	case "pl":
		e.Lf("bgez %s, %s", ShadowN, skip)
	case "cs":
		e.Lf("bnez %s, %s", ShadowC, skip)
	case "cc":
		e.Lf("beqz %s, %s", ShadowC, skip)
	case "vs":
		e.Lf("bnez %s, %s", ShadowV, skip)
	case "vc":
		e.Lf("beqz %s, %s", ShadowV, skip)
	case "lt":
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("bnez %s, %s", ScrV1, skip)
	case "ge":
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("beqz %s, %s", ScrV1, skip)
	case "gt":
		// gt: Z=0 AND (N XOR V = 0). ShadowZ contract: r25!=0 ⇔ Z=0.
		early := e.NewLabel("dbgt_early")
		e.Lf("beqz %s, %s", ShadowZ, early) // r25==0 → Z=1 → cc false → fall through (no skip)
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("bnez %s, %s", ScrV1, early) // lt → cc false
		e.Lf("bra %s", skip)              // Z=0 AND not-lt → cc true → skip
		e.Label(early)
	case "le":
		// le: Z=1 OR (N XOR V = 1). Z=1 ⇔ r25==0 → beqz r25.
		take := e.NewLabel("dble_take")
		e.Lf("beqz %s, %s", ShadowZ, take) // Z=1 → cc true → skip
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("bnez %s, %s", ScrV1, take)
		early := e.NewLabel("dble_early")
		e.Lf("bra %s", early)
		e.Label(take)
		e.Lf("bra %s", skip)
		e.Label(early)
	case "hi":
		// hi: C=0 AND Z=0. Z=0 ⇔ r25!=0.
		early := e.NewLabel("dbhi_early")
		e.Lf("bnez %s, %s", ShadowC, early)
		e.Lf("beqz %s, %s", ShadowZ, early) // r25==0 → Z=1 → cc false
		e.Lf("bra %s", skip)
		e.Label(early)
	case "ls":
		// ls: C=1 OR Z=1. Z=1 ⇔ r25==0 → beqz r25 = skip.
		e.Lf("bnez %s, %s", ShadowC, skip)
		e.Lf("beqz %s, %s", ShadowZ, skip)
	default:
		return fmt.Errorf("unsupported DBcc kind %q", mnem)
	}
	return nil
}

// emitShadowScc lowers a Scc dst — set low byte of dst to $FF if cc, else $00.
// Uses shadow CCR.
func (c *Converter) emitShadowScc(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("Scc requires 1 operand")
	}
	dst, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	// Pre-validate that dst can hold a byte. m68k Scc on An is illegal; we
	// reject it once here so the byte writes below can't fail.
	if dst.Mode == AMAddrReg {
		return fmt.Errorf("Scc on An is illegal m68k")
	}
	setLbl := e.NewLabel("scc_set")
	doneLbl := e.NewLabel("scc_done")
	if err := c.emitSccConditionSet(e, l.Mnemonic, setLbl); err != nil {
		return err
	}
	_ = c.emitWriteByteConst(e, dst, 0)
	e.Lf("bra %s", doneLbl)
	e.Label(setLbl)
	_ = c.emitWriteByteConst(e, dst, 0xFF)
	e.Label(doneLbl)
	return nil
}

// emitSccConditionSet emits a branch to `setLbl` when cc is TRUE. ST and SF
// are handled at dispatch time (emitSetByte) and never reach this helper.
func (c *Converter) emitSccConditionSet(e *Emit, mnem, setLbl string) error {
	kind := bccKind(mnem)
	switch kind {
	case "eq":
		e.Lf("beqz %s, %s", ShadowZ, setLbl)
	case "ne":
		e.Lf("bnez %s, %s", ShadowZ, setLbl)
	case "mi":
		e.Lf("bltz %s, %s", ShadowN, setLbl)
	case "pl":
		e.Lf("bgez %s, %s", ShadowN, setLbl)
	case "cs":
		e.Lf("bnez %s, %s", ShadowC, setLbl)
	case "cc":
		e.Lf("beqz %s, %s", ShadowC, setLbl)
	case "vs":
		e.Lf("bnez %s, %s", ShadowV, setLbl)
	case "vc":
		e.Lf("beqz %s, %s", ShadowV, setLbl)
	case "lt":
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("bnez %s, %s", ScrV1, setLbl)
	case "ge":
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("beqz %s, %s", ScrV1, setLbl)
	case "gt":
		// gt: Z=0 AND (N XOR V = 0). Z=1 ⇔ r25==0.
		early := e.NewLabel("sgt_skip")
		e.Lf("beqz %s, %s", ShadowZ, early) // Z=1 → cc false
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("bnez %s, %s", ScrV1, early)
		e.Lf("bra %s", setLbl)
		e.Label(early)
	case "le":
		// le: Z=1 OR (N XOR V = 1). Z=1 ⇔ r25==0 → beqz r25 = set.
		e.Lf("beqz %s, %s", ShadowZ, setLbl)
		e.Lf("lsr.q %s, %s, #63", ScrV1, ShadowN)
		e.Lf("and.q %s, %s, #1", ScrV1, ScrV1)
		e.Lf("eor.q %s, %s, %s", ScrV1, ScrV1, ShadowV)
		e.Lf("bnez %s, %s", ScrV1, setLbl)
	case "hi":
		// hi: C=0 AND Z=0. Z=0 ⇔ r25!=0.
		early := e.NewLabel("shi_skip")
		e.Lf("bnez %s, %s", ShadowC, early)
		e.Lf("beqz %s, %s", ShadowZ, early) // r25==0 → Z=1 → cc false
		e.Lf("bra %s", setLbl)
		e.Label(early)
	case "ls":
		// ls: C=1 OR Z=1. Z=1 ⇔ r25==0.
		e.Lf("bnez %s, %s", ShadowC, setLbl)
		e.Lf("beqz %s, %s", ShadowZ, setLbl)
	default:
		return fmt.Errorf("unsupported Scc kind %q", mnem)
	}
	return nil
}

// emitWriteByteConst writes a byte-sized constant (val) into dst.
func (c *Converter) emitWriteByteConst(e *Emit, dst Operand, val byte) error {
	if dst.Mode == AMDataReg {
		// Clear low byte then OR in val.
		e.Lf("and.q %s, %s, #%s", dst.Reg.IE64, dst.Reg.IE64, SizeInvMask(1))
		if val != 0 {
			e.Lf("or.q %s, %s, #$%X", dst.Reg.IE64, dst.Reg.IE64, val)
		}
		return nil
	}
	if val == 0 {
		return c.storeValue(e, dst, 1, "r0")
	}
	e.Lf("move.b %s, #$%X", ScrV1, val)
	return c.storeValue(e, dst, 1, ScrV1)
}
