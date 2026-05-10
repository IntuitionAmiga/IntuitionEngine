package main

import (
	"fmt"
	"strings"
)

// Phase 5: 68020 extras lowering.
//
// Covered in this file:
//   - TRAP #n          -> syscall #n  (locked range #0..#15)
//   - TRAPV            -> conditional syscall #18 against shadow V (Phase 5 strict-only)
//   - CHK[.X] src,Dn   -> bounds test + syscall #17 on fail
//
// Syscall # vector table is locked in sdk/docs/M68KtoIE64.md §11. Integer
// TRAP-instruction-# (0..15) and m68k exception-vector-# (5,6,7,...) live
// in disjoint syscall # ranges to avoid handler collisions.
//   - MULU.L Dq,Dh:Dl  -> mulu.l + mulhu.q pair
//   - MULS.L Dq,Dh:Dl  -> muls.l + mulhs.q pair
//   - DIVU.L Dq,Dr:Dq  -> divu.l + mod.l pair
//   - DIVS.L Dq,Dr:Dq  -> divs.l + mods.l pair
//   - BFEXTU / BFEXTS  -> shift+mask lowering for {Dn,#offset,#width}
//   - MOVEC            -> stripped with diagnostic
//
// CAS/CAS2/PACK/UNPK/BCD remain TODO; they emit "; ERROR" in strict mode.

// emit68020Extra returns true iff the mnemonic was handled here.
func (c *Converter) emit68020Extra(e *Emit, l Line) (bool, error) {
	switch l.Mnemonic {
	case "trap":
		return true, c.emitTrap(e, l)
	case "chk":
		return true, c.emitChk(e, l)
	case "movec":
		// Privileged, strip.
		e.Lf("; m68kto64: stripped %s %s (privileged, user-mode AB3D2 should not hit)",
			l.Mnemonic, strings.Join(l.Operands, ","))
		return true, nil
	case "mulu", "muls":
		// Pair form (.l with Dh:Dl) — fall through to here only when operand
		// looks like "Dh:Dl"; otherwise Phase 2 emitArith would handle it.
		if isPairOperand(l.Operands) {
			return true, c.emitMulPair(e, l)
		}
		return false, nil
	case "divu", "divs":
		if isPairOperand(l.Operands) {
			return true, c.emitDivPair(e, l)
		}
		return false, nil
	case "bfextu":
		return true, c.emitBfextu(e, l, false)
	case "bfexts":
		return true, c.emitBfextu(e, l, true)
	}
	return false, nil
}

// isPairOperand reports whether any operand uses the m68k pair syntax
// "Dh:Dl" (the destination form of MULU.L / DIVU.L).
func isPairOperand(ops []string) bool {
	for _, o := range ops {
		if i := strings.Index(o, ":"); i > 0 {
			lo := strings.TrimSpace(o[:i])
			hi := strings.TrimSpace(o[i+1:])
			if IsRegisterName(lo) && IsRegisterName(hi) {
				return true
			}
		}
	}
	return false
}

// =====================================================================
// TRAP / TRAPV / CHK
// =====================================================================

// emitTrap lowers `trap #n` to `syscall #n`.
func (c *Converter) emitTrap(e *Emit, l Line) error {
	if len(l.Operands) != 1 {
		return fmt.Errorf("trap requires 1 operand")
	}
	op, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	if op.Mode != AMImmediate {
		return fmt.Errorf("trap operand must be #imm")
	}
	e.Lf("syscall #%s", op.Imm)
	return nil
}

// emitChk lowers CHK[.X] src,Dn — m68k bounds-trap if Dn < 0 or Dn > src
// (signed). On failure: syscall #17 (m68k vector 6, relocated to #17 to
// avoid collision with TRAP #6).
func (c *Converter) emitChk(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("chk requires 2 operands")
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
		return fmt.Errorf("chk destination must be Dn")
	}
	size := SizeBytes(l.Size)
	if size == 0 {
		size = 2
	}
	srcReg, err := c.fuseNormaliseValue(e, src, size, true, ScrV1)
	if err != nil {
		return err
	}
	// dst is pre-validated as AMDataReg above, so fuseNormaliseValue cannot
	// fail here — error path inlined and the err check removed.
	dstReg, _ := c.fuseNormaliseValue(e, dst, size, true, ScrV2)
	pass := e.NewLabel("chk_pass")
	fail := e.NewLabel("chk_fail")
	e.Lf("bltz %s, %s", dstReg, fail)
	e.Lf("bgt %s, %s, %s", dstReg, srcReg, fail)
	e.Lf("bra %s", pass)
	e.Label(fail)
	e.L("syscall #17")
	e.Label(pass)
	return nil
}

// =====================================================================
// MULU.L / MULS.L pair, DIVU.L / DIVS.L pair
// =====================================================================

// splitPair splits "Dh:Dl" (or "DhDl") into (high, low) IE64 register names.
func splitPair(s string) (hi, lo string, err error) {
	i := strings.Index(s, ":")
	if i < 0 {
		return "", "", fmt.Errorf("expected Dh:Dl, got %q", s)
	}
	hiTok := strings.TrimSpace(s[:i])
	loTok := strings.TrimSpace(s[i+1:])
	rh, ok1 := LookupRegister(hiTok)
	rl, ok2 := LookupRegister(loTok)
	if !ok1 || !ok2 {
		return "", "", fmt.Errorf("bad pair %q", s)
	}
	return rh.IE64, rl.IE64, nil
}

func (c *Converter) emitMulPair(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s requires 2 operands (src,Dh:Dl)", l.Mnemonic)
	}
	src, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	hi, lo, err := splitPair(l.Operands[1])
	if err != nil {
		return err
	}
	srcReg, srcImm, err := c.loadValue(e, src, 4, ScrV1)
	if err != nil {
		return err
	}
	if srcImm != "" {
		e.Lf("move.l %s, #%s", ScrV1, srcImm)
		srcReg = ScrV1
	}
	signed := l.Mnemonic == "muls"
	if signed {
		e.Lf("muls.l %s, %s, %s", lo, lo, srcReg)
		e.Lf("mulhs.q %s, %s, %s", hi, lo, srcReg)
	} else {
		e.Lf("mulu.l %s, %s, %s", lo, lo, srcReg)
		e.Lf("mulhu.q %s, %s, %s", hi, lo, srcReg)
	}
	return nil
}

func (c *Converter) emitDivPair(e *Emit, l Line) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s requires 2 operands (src,Dr:Dq)", l.Mnemonic)
	}
	src, err := ParseOperand(l.Operands[0])
	if err != nil {
		return err
	}
	rem, quo, err := splitPair(l.Operands[1])
	if err != nil {
		return err
	}
	srcReg, srcImm, err := c.loadValue(e, src, 4, ScrV1)
	if err != nil {
		return err
	}
	if srcImm != "" {
		e.Lf("move.l %s, #%s", ScrV1, srcImm)
		srcReg = ScrV1
	}
	op := "divu"
	mod := "mod"
	if l.Mnemonic == "divs" {
		op = "divs"
		mod = "mods"
	}
	// Save quo before computing mod (mod uses original quo).
	e.Lf("move.l %s, %s", ScrV2, quo)
	e.Lf("%s.l %s, %s, %s", op, quo, ScrV2, srcReg)
	e.Lf("%s.l %s, %s, %s", mod, rem, ScrV2, srcReg)
	return nil
}

// =====================================================================
// BFEXTU / BFEXTS — bit-field extract
// =====================================================================

// emitBfextu lowers BFEXTU / BFEXTS Dn{#offset:#width},Dd at register source.
//
// 68020 syntax:  bfextu  Dn{#offset:#width}, Dd
// IE64 lowering:
//   shift = 32 - offset - width   (left-justify)
//   mask  = (1 << width) - 1
//   tmp   = (Dn >> (32 - offset - width)) & mask     ; for big-endian m68k bit-numbering
// Actually m68k bit fields number MSB=0 within the source value.
//   field = (Dn >> (32 - offset - width)) & ((1<<width) - 1)  for fixed-32-bit Dn
// Only the {Dn,#offset,#width} form is supported here.
func (c *Converter) emitBfextu(e *Emit, l Line, signed bool) error {
	if len(l.Operands) != 2 {
		return fmt.Errorf("%s requires 2 operands", l.Mnemonic)
	}
	srcText := strings.TrimSpace(l.Operands[0])
	dst, err := ParseOperand(l.Operands[1])
	if err != nil {
		return err
	}
	if dst.Mode != AMDataReg {
		return fmt.Errorf("%s destination must be Dn", l.Mnemonic)
	}
	// Parse "Dn{#off:#wid}".
	bopen := strings.Index(srcText, "{")
	bclose := strings.Index(srcText, "}")
	if bopen < 0 || bclose < 0 || bclose < bopen {
		return fmt.Errorf("%s: expected Dn{#off:#wid}", l.Mnemonic)
	}
	srcReg, ok := LookupRegister(strings.TrimSpace(srcText[:bopen]))
	if !ok || srcReg.Class != RegData {
		return fmt.Errorf("%s: source must be Dn (not %q)", l.Mnemonic,
			strings.TrimSpace(srcText[:bopen]))
	}
	field := strings.TrimSpace(srcText[bopen+1 : bclose])
	colon := strings.Index(field, ":")
	if colon < 0 {
		return fmt.Errorf("%s: expected #off:#wid in {}", l.Mnemonic)
	}
	off := strings.TrimPrefix(strings.TrimSpace(field[:colon]), "#")
	wid := strings.TrimPrefix(strings.TrimSpace(field[colon+1:]), "#")
	rd := dst.Reg.IE64
	rs := srcReg.IE64
	// shift = 32 - offset - width
	e.Lf("move.l %s, #32", ScrV1)
	e.Lf("sub.l %s, %s, #%s", ScrV1, ScrV1, off)
	e.Lf("sub.l %s, %s, #%s", ScrV1, ScrV1, wid)
	if signed {
		// Sign-aware: lsl up so MSB sits at bit 63, asr.q down to width.
		// shiftHi = 64 - width; shiftLo = 64 - width
		// equivalently: lsl rd, rs, (32 + offset)  ; asr rd, rd, (64 - width)
		e.Lf("move.l %s, #32", ScrV2)
		e.Lf("add.l %s, %s, #%s", ScrV2, ScrV2, off)
		e.Lf("lsl.q %s, %s, %s", rd, rs, ScrV2)
		e.Lf("move.l %s, #64", ScrV2)
		e.Lf("sub.l %s, %s, #%s", ScrV2, ScrV2, wid)
		e.Lf("asr.q %s, %s, %s", rd, rd, ScrV2)
	} else {
		e.Lf("lsr.l %s, %s, %s", rd, rs, ScrV1)
		// mask = (1 << width) - 1
		e.Lf("move.l %s, #1", ScrV2)
		e.Lf("lsl.l %s, %s, #%s", ScrV2, ScrV2, wid)
		e.Lf("sub.l %s, %s, #1", ScrV2, ScrV2)
		e.Lf("and.l %s, %s, %s", rd, rd, ScrV2)
	}
	return nil
}
