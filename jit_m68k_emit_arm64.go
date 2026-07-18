// jit_m68k_emit_arm64.go - ARM64 native code emitter for the M68020 JIT
// (parity plan milestone 3, slices 1 and 2).
//
// Correctness-first minimal backend: straight-line prefixes of integer
// instructions, full CCR materialisation on every flag-setting instruction,
// no liveness elision, no chaining, no register pinning.
//
// Slice 2 adds memory effective addresses with big-endian access, byte and
// word operation sizes, and mid-block I/O bails: every guest memory access
// is guarded by a bounds check and an I/O page bitmap probe, and a guarded
// access that fails exits the block BEFORE any of the faulting instruction's
// side effects, publishing the faulting PC, the partial retired count and
// NeedIOFallback so the dispatcher can interpret that one instruction.
//
// Register discipline: the emitted block is a leaf function that touches
// only caller-saved registers, so it needs no stack frame and never spills
// callee-saved state.
//
//	X0  M68KJITContext pointer (C ABI argument, live for the whole block)
//	X1  &cpu.DataRegs[0]
//	X2  &cpu.AddrRegs[0]
//	X3  &cpu.SR
//	W4  live CCR (X N Z V C in M68K bit order, bits 4..0)
//	X5  &cpu.memory[0]
//	W6  len(cpu.memory)
//	X7  I/O page bitmap base (0 when absent)
//	W8  I/O page bitmap length in pages
//	W9-W15  scratch
//
// The pinned callee-saved mapping in jit_m68k_abi_arm64.go is the milestone 4
// residency plan; these slices deliberately do not use it. X18 is never
// touched (AAPCS64 platform register).

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"unsafe"
)

// ARM64 condition codes used for flag extraction and guards.
const (
	m68kA64CondEQ = 0x0
	m68kA64CondNE = 0x1
	m68kA64CondCS = 0x2 // also HS
	m68kA64CondCC = 0x3 // also LO
	m68kA64CondMI = 0x4
	m68kA64CondVS = 0x6
	m68kA64CondHI = 0x8
)

// M68K CCR bit positions (SR bits 4..0) as untyped shift amounts.
// (m68kCCRBit* in jit_m68k_ccr_liveness.go are the typed liveness-mask
// constants; these are the raw encoder shift values.)
const (
	m68kA64BitC = 0
	m68kA64BitV = 1
	m68kA64BitZ = 2
	m68kA64BitN = 3
	m68kA64BitX = 4
)

// Encoders missing from the IE64 arm64 helper set.

// adds Wd, Wn, Wm
func arm64ADDS_W(rd, rn, rm byte) uint32 {
	return 0x2B000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// subs Wd, Wn, Wm
func arm64SUBS_W(rd, rn, rm byte) uint32 {
	return 0x6B000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// adds Wd, Wn, #imm12
func arm64ADDS_W_imm(rd, rn byte, imm12 uint32) uint32 {
	return 0x31000000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rd)
}

// subs Wd, Wn, #imm12
func arm64SUBS_W_imm(rd, rn byte, imm12 uint32) uint32 {
	return 0x71000000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rd)
}

// add Wd, Wn, #imm12 (no flags)
func arm64ADD_W_imm(rd, rn byte, imm12 uint32) uint32 {
	return 0x11000000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rd)
}

// sub Wd, Wn, #imm12 (no flags)
func arm64SUB_W_imm(rd, rn byte, imm12 uint32) uint32 {
	return 0x51000000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rd)
}

// cmp Wn, #imm12  (SUBS WZR, Wn, #imm12 — 32-bit form; the IE64 helper
// arm64CMP_imm is the 64-bit form, whose N flag would come from bit 63)
func arm64CMP_W_imm(rn byte, imm12 uint32) uint32 {
	return 0x7100001F | (imm12&0xFFF)<<10 | uint32(rn)<<5
}

// cset Wd, cond  (CSINC Wd, WZR, WZR, invert(cond))
func arm64CSET_W(rd byte, cond byte) uint32 {
	return 0x1A800400 | uint32(31)<<16 | uint32(cond^1)<<12 | uint32(31)<<5 | uint32(rd)
}

// orr Wd, Wn, Wm, lsl #shift
func arm64ORR_W_lsl(rd, rn, rm byte, shift uint32) uint32 {
	return 0x2A000000 | uint32(rm)<<16 | (shift&0x3F)<<10 | uint32(rn)<<5 | uint32(rd)
}

// lslv Wd, Wn, Wm (variable logical shift left, shift amount mod 32)
func arm64LSLV_W(rd, rn, rm byte) uint32 {
	return 0x1AC02000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// rev16 Wd, Wn (byte swap within each halfword)
func arm64REV16_W(rd, rn byte) uint32 {
	return 0x5AC00400 | uint32(rn)<<5 | uint32(rd)
}

// lsl Wd, Wn, #shift (UBFM alias, 32-bit)
func arm64LSL_W_imm(rd, rn byte, shift uint32) uint32 {
	immr := (32 - shift) & 31
	imms := 31 - shift
	return 0x53000000 | immr<<16 | imms<<10 | uint32(rn)<<5 | uint32(rd)
}

// sxth Wd, Wn (SBFM alias, 32-bit)
func arm64SXTH_W(rd, rn byte) uint32 {
	return 0x13003C00 | uint32(rn)<<5 | uint32(rd)
}

// sxtb Wd, Wn (SBFM alias, 32-bit)
func arm64SXTB_W(rd, rn byte) uint32 {
	return 0x13001C00 | uint32(rn)<<5 | uint32(rd)
}

// asr Wd, Wn, #shift (SBFM alias, 32-bit)
func arm64ASR_W_imm(rd, rn byte, shift uint32) uint32 {
	return 0x13007C00 | (shift&0x1F)<<16 | uint32(rn)<<5 | uint32(rd)
}

// Fixed emitter register assignments (see file header).
const (
	m68kA64Ctx      = 0
	m68kA64DataBase = 1
	m68kA64AddrBase = 2
	m68kA64SRAddr   = 3
	m68kA64CCR      = 4
	m68kA64MemBase  = 5
	m68kA64MemSize  = 6
	m68kA64IOBmp    = 7
	m68kA64IOBmpLen = 8
	m68kA64Tmp0     = 9
	m68kA64Tmp1     = 10
	m68kA64Tmp2     = 11
	m68kA64Tmp3     = 12
	m68kA64Tmp4     = 13
	m68kA64Tmp5     = 14
	m68kA64Tmp6     = 15
)

func m68kA64MovImm32(cb *CodeBuffer, rd byte, val uint32) {
	cb.Emit32(arm64MOVZ_W(rd, uint16(val), 0))
	if val>>16 != 0 {
		cb.Emit32(arm64MOVK_W(rd, uint16(val>>16), 16))
	}
}

func m68kA64LoadD(cb *CodeBuffer, rt byte, dn uint16) {
	cb.Emit32(arm64LDR_W_imm(rt, m68kA64DataBase, uint32(dn)))
}

func m68kA64StoreD(cb *CodeBuffer, rt byte, dn uint16) {
	cb.Emit32(arm64STR_W_imm(rt, m68kA64DataBase, uint32(dn)))
}

func m68kA64LoadA(cb *CodeBuffer, rt byte, an uint16) {
	cb.Emit32(arm64LDR_W_imm(rt, m68kA64AddrBase, uint32(an)))
}

func m68kA64StoreA(cb *CodeBuffer, rt byte, an uint16) {
	cb.Emit32(arm64STR_W_imm(rt, m68kA64AddrBase, uint32(an)))
}

// m68kA64AssembleCCR combines single-bit registers into W4.
// Each argument is a register holding 0/1, or 0xFF meaning "bit is zero".
// x may equal c (the ADD/SUB X=C rule).
func m68kA64AssembleCCR(cb *CodeBuffer, c, v, z, n, x byte) {
	// Start from C in bit 0 (or zero).
	if c != 0xFF {
		cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, 31, c, m68kA64BitC)) // mov W4, Wc
	} else {
		cb.Emit32(arm64MOVZ_W(m68kA64CCR, 0, 0))
	}
	if v != 0xFF {
		cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, v, m68kA64BitV))
	}
	if z != 0xFF {
		cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, z, m68kA64BitZ))
	}
	if n != 0xFF {
		cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, n, m68kA64BitN))
	}
	if x != 0xFF {
		cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, x, m68kA64BitX))
	}
}

// m68kA64FlagsArith extracts full XNZVC after an ADDS/SUBS.
// carryCond selects the M68K carry: CS for add, CC (borrow) for sub/cmp.
// If keepX is true (CMP), X is preserved from the old CCR instead of C.
func m68kA64FlagsArith(cb *CodeBuffer, carryCond byte, keepX bool) {
	if keepX {
		cb.Emit32(arm64UBFX_W(m68kA64Tmp4, m68kA64CCR, m68kA64BitX, 1))
	}
	cb.Emit32(arm64CSET_W(m68kA64Tmp0, carryCond))     // C
	cb.Emit32(arm64CSET_W(m68kA64Tmp1, m68kA64CondVS)) // V
	cb.Emit32(arm64CSET_W(m68kA64Tmp2, m68kA64CondEQ)) // Z
	cb.Emit32(arm64CSET_W(m68kA64Tmp3, m68kA64CondMI)) // N
	if keepX {
		m68kA64AssembleCCR(cb, m68kA64Tmp0, m68kA64Tmp1, m68kA64Tmp2, m68kA64Tmp3, m68kA64Tmp4)
	} else {
		m68kA64AssembleCCR(cb, m68kA64Tmp0, m68kA64Tmp1, m68kA64Tmp2, m68kA64Tmp3, m68kA64Tmp0)
	}
}

// m68kA64ShiftForSize returns the left shift that places a sized value's
// sign bit at bit 31.
func m68kA64ShiftForSize(size uint32) uint32 {
	return (4 - size) * 8
}

// m68kA64FlagsLogicPreserveVC updates only N and Z from the SIZED value in
// rv, preserving X, V and C. This matches the interpreter's SetFlagsNZ,
// which the amd64 backend also replicates (emitCCR_LogicPreserveVC): IE's
// AND/OR/EOR deliberately keep V and C, unlike the M68000PRM. Parity is
// with the interpreter, not the manual. rv is not modified.
func m68kA64FlagsLogicPreserveVC(cb *CodeBuffer, rv byte, size uint32) {
	rt := rv
	if sh := m68kA64ShiftForSize(size); sh != 0 {
		cb.Emit32(arm64LSL_W_imm(m68kA64Tmp5, rv, sh))
		rt = m68kA64Tmp5
	}
	cb.Emit32(arm64CMP_W_imm(rt, 0))
	// Keep X, V, C; refresh N and Z.
	cb.Emit32(arm64CSET_W(m68kA64Tmp2, m68kA64CondEQ)) // Z
	cb.Emit32(arm64CSET_W(m68kA64Tmp3, m68kA64CondMI)) // N
	m68kA64MovImm32(cb, m68kA64Tmp0, 0x13)             // X|V|C mask
	cb.Emit32(arm64AND_W(m68kA64CCR, m68kA64CCR, m68kA64Tmp0))
	cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, m68kA64Tmp2, m68kA64BitZ))
	cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, m68kA64Tmp3, m68kA64BitN))
}

// m68kA64FlagsMove materialises the MOVE/TST CCR (N/Z from the sized value,
// V=C=0, X preserved). rv is not modified.
func m68kA64FlagsMove(cb *CodeBuffer, rv byte, size uint32) {
	rt := rv
	if sh := m68kA64ShiftForSize(size); sh != 0 {
		cb.Emit32(arm64LSL_W_imm(m68kA64Tmp5, rv, sh))
		rt = m68kA64Tmp5
	}
	cb.Emit32(arm64CMP_W_imm(rt, 0))
	cb.Emit32(arm64UBFX_W(m68kA64Tmp4, m68kA64CCR, m68kA64BitX, 1))
	cb.Emit32(arm64CSET_W(m68kA64Tmp2, m68kA64CondEQ)) // Z
	cb.Emit32(arm64CSET_W(m68kA64Tmp3, m68kA64CondMI)) // N
	m68kA64AssembleCCR(cb, 0xFF, 0xFF, m68kA64Tmp2, m68kA64Tmp3, m68kA64Tmp4)
}

// m68kA64FlagsStatic sets CCR to a compile-time N/Z pair with V=C=0 and X
// preserved (MOVEQ, MOVE #imm, CLR).
func m68kA64FlagsStatic(cb *CodeBuffer, negative, zero bool) {
	cb.Emit32(arm64UBFX_W(m68kA64Tmp4, m68kA64CCR, m68kA64BitX, 1))
	bits := uint16(0)
	if negative {
		bits |= 1 << m68kA64BitN
	}
	if zero {
		bits |= 1 << m68kA64BitZ
	}
	cb.Emit32(arm64MOVZ_W(m68kA64CCR, bits, 0))
	cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, m68kA64Tmp4, m68kA64BitX))
}

// ===========================================================================
// Effective addresses (slice 2)
// ===========================================================================

// EA modes the arm64 emitter lowers. Absolute short and PC-relative modes
// resolve to a compile-time constant address and reuse m68kA64EAAbsL; the
// brief-format index modes ((d8,An,Xn) and (d8,PC,Xn)) use m68kA64EAIndex.
// The 68020 full extension-word format (memory indirect, base/outer
// displacements) stays interpreter fallback.
const (
	m68kA64EADn    = iota // data register direct
	m68kA64EAAn           // address register direct
	m68kA64EAInd          // (An)
	m68kA64EAPost         // (An)+
	m68kA64EAPre          // -(An)
	m68kA64EADisp         // (d16,An)
	m68kA64EAAbsL         // (xxx).L, (xxx).W and (d16,PC) (constant address)
	m68kA64EAImm          // #imm
	m68kA64EAIndex        // (d8,An,Xn) and (d8,PC,Xn), brief format
)

type m68kA64EA struct {
	kind int
	reg  uint16 // Dn/An number (base An for m68kA64EAIndex)
	disp int32  // (d16,An); disp8 for m68kA64EAIndex
	abs  uint32 // (xxx).L/.W and (d16,PC) constant; PC base for PC-index
	imm  uint32 // #imm (already sized)
	ext  int    // extension words consumed

	// Brief-format index fields (m68kA64EAIndex).
	idxReg    uint16 // index register number
	idxIsAddr bool   // index is An (else Dn)
	idxLong   bool   // index used as long (else sign-extended word)
	scale     uint32 // index scale shift (0..3)
	pcRel     bool   // base is the PC constant in abs, not An
}

// m68kA64ParseBriefIndex decodes a brief-format index extension word into an
// m68kA64EAIndex. The 68020 full extension-word format (bit 8 set) stays
// interpreter fallback. base is the An number (ignored when pcRel), pcBase is
// the PC constant used as the base for (d8,PC,Xn).
func m68kA64ParseBriefIndex(ext uint16, pcRel bool, base uint16, pcBase uint32) (m68kA64EA, bool) {
	if ext&M68K_EXT_FULL_FORMAT != 0 {
		return m68kA64EA{}, false
	}
	return m68kA64EA{
		kind:      m68kA64EAIndex,
		reg:       base,
		disp:      int32(int8(ext & 0xFF)),
		abs:       pcBase,
		idxReg:    (ext >> 12) & 0x07,
		idxIsAddr: (ext>>M68K_EXT_REG_TYPE_BIT)&1 != 0,
		idxLong:   (ext>>M68K_EXT_SIZE_BIT)&1 != 0,
		scale:     uint32((ext >> M68K_EXT_SCALE_START_BIT) & 3),
		pcRel:     pcRel,
		ext:       1,
	}, true
}

// m68kA64ParseEA decodes a mode/reg pair with its extension words at extPC.
// Returns ok=false for any shape outside the supported set.
func m68kA64ParseEA(mode, reg uint16, size uint32, memory []byte, extPC uint32) (m68kA64EA, bool) {
	rd16 := func(off uint32) (uint16, bool) {
		if int(extPC)+int(off)+2 > len(memory) {
			return 0, false
		}
		return uint16(memory[extPC+off])<<8 | uint16(memory[extPC+off+1]), true
	}
	switch mode {
	case 0:
		return m68kA64EA{kind: m68kA64EADn, reg: reg}, true
	case 1:
		if size == 1 {
			return m68kA64EA{}, false // byte An access is illegal
		}
		return m68kA64EA{kind: m68kA64EAAn, reg: reg}, true
	case 2:
		return m68kA64EA{kind: m68kA64EAInd, reg: reg}, true
	case 3:
		return m68kA64EA{kind: m68kA64EAPost, reg: reg}, true
	case 4:
		return m68kA64EA{kind: m68kA64EAPre, reg: reg}, true
	case 5:
		w, ok := rd16(0)
		if !ok {
			return m68kA64EA{}, false
		}
		return m68kA64EA{kind: m68kA64EADisp, reg: reg, disp: int32(int16(w)), ext: 1}, true
	case 6: // (d8,An,Xn) brief format
		w, ok := rd16(0)
		if !ok {
			return m68kA64EA{}, false
		}
		return m68kA64ParseBriefIndex(w, false, reg, 0)
	case 7:
		switch reg {
		case 0: // (xxx).W (sign-extended constant address)
			w, ok := rd16(0)
			if !ok {
				return m68kA64EA{}, false
			}
			return m68kA64EA{kind: m68kA64EAAbsL, abs: uint32(int32(int16(w))), ext: 1}, true
		case 1: // (xxx).L
			hi, ok1 := rd16(0)
			lo, ok2 := rd16(2)
			if !ok1 || !ok2 {
				return m68kA64EA{}, false
			}
			return m68kA64EA{kind: m68kA64EAAbsL, abs: uint32(hi)<<16 | uint32(lo), ext: 2}, true
		case 2: // (d16,PC): base is the address of the extension word
			w, ok := rd16(0)
			if !ok {
				return m68kA64EA{}, false
			}
			return m68kA64EA{kind: m68kA64EAAbsL, abs: extPC + uint32(int32(int16(w))), ext: 1, pcRel: true}, true
		case 3: // (d8,PC,Xn) brief format
			w, ok := rd16(0)
			if !ok {
				return m68kA64EA{}, false
			}
			return m68kA64ParseBriefIndex(w, true, 0, extPC)
		case 4: // #imm
			switch size {
			case 1, 2:
				w, ok := rd16(0)
				if !ok {
					return m68kA64EA{}, false
				}
				v := uint32(w)
				if size == 1 {
					v &= 0xFF
				}
				return m68kA64EA{kind: m68kA64EAImm, imm: v, ext: 1}, true
			case 4:
				hi, ok1 := rd16(0)
				lo, ok2 := rd16(2)
				if !ok1 || !ok2 {
					return m68kA64EA{}, false
				}
				return m68kA64EA{kind: m68kA64EAImm, imm: uint32(hi)<<16 | uint32(lo), ext: 2}, true
			}
		}
	}
	return m68kA64EA{}, false
}

func (ea *m68kA64EA) isMem() bool {
	switch ea.kind {
	case m68kA64EAInd, m68kA64EAPost, m68kA64EAPre, m68kA64EADisp, m68kA64EAAbsL, m68kA64EAIndex:
		return true
	}
	return false
}

// sideEffectDelta returns the signed change (An)+/-(An) applies to An,
// honouring the A7 byte rule (stack stays word-aligned: byte moves by 2).
func (ea *m68kA64EA) sideEffectDelta(size uint32) int32 {
	step := int32(size)
	if size == 1 && ea.reg == 7 {
		step = 2
	}
	switch ea.kind {
	case m68kA64EAPost:
		return step
	case m68kA64EAPre:
		return -step
	}
	return 0
}

// ===========================================================================
// Block emitter with per-instruction bail stubs
// ===========================================================================

type m68kA64BranchSite struct {
	off   int
	base  uint32 // instruction word without its imm19/imm26 field
	imm26 bool
}

type m68kA64BailStub struct {
	sites   []m68kA64BranchSite
	pc      uint32
	retired uint32
}

type m68kA64Emitter struct {
	cb    *CodeBuffer
	bails []m68kA64BranchSite // guard branches of the CURRENT instruction
	stubs []m68kA64BailStub

	// Block exit shape, set by a final branch instruction. dynExit means the
	// resume PC was computed at run time into W14 (Tmp5); staticExit
	// overrides the fallthrough resume PC with a compile-time target (BRA).
	dynExit       bool
	staticExit    uint32
	hasStaticExit bool
}

func (e *m68kA64Emitter) patchSite(s m68kA64BranchSite, target int) {
	delta := int32(target - s.off)
	if s.imm26 {
		e.cb.PatchUint32(s.off, s.base|uint32(delta>>2)&0x3FFFFFF)
	} else {
		e.cb.PatchUint32(s.off, s.base|(uint32(delta>>2)&0x7FFFF)<<5)
	}
}

// localFwdBcond emits a forward conditional branch inside the current
// instruction and returns its unpatched site; bindLocal patches it to the
// current position. Used for in-block control (division overflow, etc.) that
// is not a bail to the interpreter.
func (e *m68kA64Emitter) localFwdBcond(cond byte) m68kA64BranchSite {
	s := m68kA64BranchSite{off: e.cb.Len(), base: 0x54000000 | uint32(cond)}
	e.cb.Emit32(arm64Bcond(cond, 0))
	return s
}

func (e *m68kA64Emitter) localFwdCBZ(rt byte) m68kA64BranchSite {
	s := m68kA64BranchSite{off: e.cb.Len(), base: 0xB4000000 | uint32(rt)}
	e.cb.Emit32(arm64CBZ(rt, 0))
	return s
}

func (e *m68kA64Emitter) localFwdB() m68kA64BranchSite {
	s := m68kA64BranchSite{off: e.cb.Len(), base: 0x14000000, imm26: true}
	e.cb.Emit32(arm64B(0))
	return s
}

func (e *m68kA64Emitter) bindLocal(s m68kA64BranchSite) {
	e.patchSite(s, e.cb.Len())
}

func (e *m68kA64Emitter) branchToBail(cond byte) {
	e.bails = append(e.bails, m68kA64BranchSite{off: e.cb.Len(), base: 0x54000000 | uint32(cond)})
	e.cb.Emit32(arm64Bcond(cond, 0))
}

func (e *m68kA64Emitter) cbzToBail(rt byte) {
	e.bails = append(e.bails, m68kA64BranchSite{off: e.cb.Len(), base: 0xB4000000 | uint32(rt)})
	e.cb.Emit32(arm64CBZ(rt, 0))
}

func (e *m68kA64Emitter) cbnzToBail(rt byte) {
	e.bails = append(e.bails, m68kA64BranchSite{off: e.cb.Len(), base: 0xB5000000 | uint32(rt)})
	e.cb.Emit32(arm64CBNZ(rt, 0))
}

// emitGuard validates a guest access of the given size at the address in
// addrReg: both bounds against MemSize and an I/O bitmap probe of the first
// byte's page plus, for multi-byte accesses, the last byte's page (a 32-bit
// stack push starting 2 bytes below an I/O page boundary must still bail;
// the amd64 single-access guard probes only the start page and keeps that
// known gap for now). On failure it branches to the current instruction's
// bail stub. addrReg is preserved; W15 (Tmp6) is clobbered.
func (e *m68kA64Emitter) emitGuard(addrReg byte, size uint32) {
	cb := e.cb
	// Start bound (also protects the end-bound ADD against 32-bit wrap).
	cb.Emit32(arm64CMP_W(addrReg, m68kA64MemSize))
	e.branchToBail(m68kA64CondCS) // addr >= MemSize
	// End bound.
	cb.Emit32(arm64ADD_W_imm(m68kA64Tmp6, addrReg, size))
	cb.Emit32(arm64CMP_W(m68kA64Tmp6, m68kA64MemSize))
	e.branchToBail(m68kA64CondHI) // addr+size > MemSize
	// I/O page bitmap probe (one byte per 256-byte page).
	skip1 := m68kA64BranchSite{off: cb.Len(), base: 0xB4000000 | uint32(m68kA64IOBmp)}
	cb.Emit32(arm64CBZ(m68kA64IOBmp, 0)) // no bitmap: plain RAM
	cb.Emit32(arm64LSR_W_imm(m68kA64Tmp6, addrReg, 8))
	cb.Emit32(arm64CMP_W(m68kA64Tmp6, m68kA64IOBmpLen))
	skip2 := m68kA64BranchSite{off: cb.Len(), base: 0x54000000 | uint32(m68kA64CondCS)}
	cb.Emit32(arm64Bcond(m68kA64CondCS, 0)) // beyond bitmap: plain RAM
	cb.Emit32(arm64LDRB_reg(m68kA64Tmp6, m68kA64IOBmp, m68kA64Tmp6))
	e.cbnzToBail(m68kA64Tmp6)
	var skip3 m68kA64BranchSite
	haveSkip3 := false
	if size > 1 {
		// Last byte's page: catches accesses that cross a 256-byte page
		// boundary into an I/O page. Usually the same page (the redundant
		// probe is cheap); when the end page is past the bitmap it is plain
		// RAM, like the start-page case above.
		cb.Emit32(arm64ADD_W_imm(m68kA64Tmp6, addrReg, size-1))
		cb.Emit32(arm64LSR_W_imm(m68kA64Tmp6, m68kA64Tmp6, 8))
		cb.Emit32(arm64CMP_W(m68kA64Tmp6, m68kA64IOBmpLen))
		skip3 = m68kA64BranchSite{off: cb.Len(), base: 0x54000000 | uint32(m68kA64CondCS)}
		haveSkip3 = true
		cb.Emit32(arm64Bcond(m68kA64CondCS, 0))
		cb.Emit32(arm64LDRB_reg(m68kA64Tmp6, m68kA64IOBmp, m68kA64Tmp6))
		e.cbnzToBail(m68kA64Tmp6)
	}
	ok := cb.Len()
	e.patchSite(skip1, ok)
	e.patchSite(skip2, ok)
	if haveSkip3 {
		e.patchSite(skip3, ok)
	}
}

// emitLoadBE loads a sized big-endian guest value at [MemBase + addrReg]
// into dst (zero-extended). Clobbers only dst.
func m68kA64EmitLoadBE(cb *CodeBuffer, dst, addrReg byte, size uint32) {
	switch size {
	case 1:
		cb.Emit32(arm64LDRB_reg(dst, m68kA64MemBase, addrReg))
	case 2:
		cb.Emit32(arm64LDRH_reg(dst, m68kA64MemBase, addrReg))
		cb.Emit32(arm64REV16_W(dst, dst))
	default:
		cb.Emit32(arm64LDR_W_reg(dst, m68kA64MemBase, addrReg))
		cb.Emit32(arm64REV_W(dst, dst))
	}
}

// emitStoreBE stores the sized low bits of src big-endian to
// [MemBase + addrReg]. src is preserved; scratch is clobbered.
func m68kA64EmitStoreBE(cb *CodeBuffer, src, addrReg, scratch byte, size uint32) {
	switch size {
	case 1:
		cb.Emit32(arm64STRB_reg(src, m68kA64MemBase, addrReg))
	case 2:
		cb.Emit32(arm64REV16_W(scratch, src))
		cb.Emit32(arm64STRH_reg(scratch, m68kA64MemBase, addrReg))
	default:
		cb.Emit32(arm64REV_W(scratch, src))
		cb.Emit32(arm64STR_W_reg(scratch, m68kA64MemBase, addrReg))
	}
}

// emitEAAddr materialises the access address of a memory EA into dst
// WITHOUT committing any side effect. extraDelta pre-adjusts the base
// register value (used when an earlier operand's uncommitted side effect
// targets the same An).
func m68kA64EmitEAAddr(cb *CodeBuffer, ea *m68kA64EA, size uint32, dst byte, extraDelta int32) {
	switch ea.kind {
	case m68kA64EAAbsL:
		m68kA64MovImm32(cb, dst, ea.abs)
		if extraDelta != 0 {
			m68kA64MovImm32(cb, m68kA64Tmp6, uint32(extraDelta))
			cb.Emit32(arm64ADD_W(dst, dst, m68kA64Tmp6))
		}
		return
	case m68kA64EAIndex:
		// base + disp8 + (index sign-extended/long, scaled). No side effects,
		// so extraDelta only ever pre-adjusts the base (rare) and folds in.
		if ea.pcRel {
			m68kA64MovImm32(cb, dst, ea.abs)
		} else {
			m68kA64LoadA(cb, dst, ea.reg)
		}
		if ea.idxIsAddr {
			m68kA64LoadA(cb, m68kA64Tmp6, ea.idxReg)
		} else {
			m68kA64LoadD(cb, m68kA64Tmp6, ea.idxReg)
		}
		if !ea.idxLong {
			cb.Emit32(arm64SXTH_W(m68kA64Tmp6, m68kA64Tmp6))
		}
		if ea.scale != 0 {
			cb.Emit32(arm64LSL_W_imm(m68kA64Tmp6, m68kA64Tmp6, ea.scale))
		}
		cb.Emit32(arm64ADD_W(dst, dst, m68kA64Tmp6))
		if d := ea.disp + extraDelta; d != 0 {
			m68kA64MovImm32(cb, m68kA64Tmp6, uint32(d))
			cb.Emit32(arm64ADD_W(dst, dst, m68kA64Tmp6))
		}
		return
	}
	m68kA64LoadA(cb, dst, ea.reg)
	adj := extraDelta
	if ea.kind == m68kA64EAPre {
		adj += ea.sideEffectDelta(size) // negative
	}
	if ea.kind == m68kA64EADisp {
		adj += ea.disp
	}
	switch {
	case adj > 0 && adj <= 0xFFF:
		cb.Emit32(arm64ADD_W_imm(dst, dst, uint32(adj)))
	case adj < 0 && -adj <= 0xFFF:
		cb.Emit32(arm64SUB_W_imm(dst, dst, uint32(-adj)))
	case adj != 0:
		m68kA64MovImm32(cb, m68kA64Tmp6, uint32(adj))
		cb.Emit32(arm64ADD_W(dst, dst, m68kA64Tmp6))
	}
}

// emitEACommit writes back the (An)+/-(An) side effect, given the access
// address still in addrReg. Clobbers scratch.
func m68kA64EmitEACommit(cb *CodeBuffer, ea *m68kA64EA, size uint32, addrReg, scratch byte) {
	switch ea.kind {
	case m68kA64EAPost:
		step := uint32(ea.sideEffectDelta(size))
		cb.Emit32(arm64ADD_W_imm(scratch, addrReg, step))
		m68kA64StoreA(cb, scratch, ea.reg)
	case m68kA64EAPre:
		m68kA64StoreA(cb, addrReg, ea.reg)
	}
}

// ===========================================================================
// Instruction admission
// ===========================================================================

// m68kA64DecodedOp is the slice-2 decode of one supported instruction.
type m68kA64DecodedOp struct {
	class int
	size  uint32
	src   m68kA64EA
	dst   m68kA64EA
	q     uint32 // ADDQ/SUBQ/shift count, or ALU op selector for the ALU classes
	kind  int    // shift/rotate kind, EXT opmode
	sub   bool   // SUBQ/SUB vs ADD variants; shift direction (true = right)
}

const (
	m68kA64ClassNOP = iota
	m68kA64ClassMOVEQ
	m68kA64ClassMOVE  // includes all EA combinations
	m68kA64ClassMOVEA // dst An
	m68kA64ClassTST
	m68kA64ClassCLR
	m68kA64ClassALUToD   // ADD/SUB/CMP/AND/OR <ea>,Dn
	m68kA64ClassALUToMem // ADD/SUB/AND/OR Dn,<ea> and EOR Dn,<ea>
	m68kA64ClassQuickD   // ADDQ/SUBQ #q,Dn
	m68kA64ClassQuickA   // ADDQ/SUBQ #q,An (no flags)
	m68kA64ClassQuickMem // ADDQ/SUBQ #q,<ea>
	m68kA64ClassLEA
	m68kA64ClassNEG      // NEG <ea>
	m68kA64ClassNOT      // NOT <ea>
	m68kA64ClassSWAP     // SWAP Dn
	m68kA64ClassEXT      // EXT.W/EXT.L/EXTB.L Dn (q = opmode)
	m68kA64ClassPEA      // PEA <ea>
	m68kA64ClassShift    // immediate-count shift/rotate on Dn
	m68kA64ClassShiftMem // memory shift/rotate by one (kind = operation 0..7)
	m68kA64ClassMulW     // MULU/MULS.W <ea>,Dn (sub = signed)
	m68kA64ClassDivW     // DIVU/DIVS.W <ea>,Dn (sub = signed)
	m68kA64ClassBitOp    // BTST/BCHG/BCLR/BSET (kind = op, sub = dynamic bit, q = bit source)
	m68kA64ClassScc      // Scc <ea> byte (kind = condition)
	m68kA64ClassTAS      // TAS <ea> byte
	m68kA64ClassMOVEM    // MOVEM (sub = mem-to-reg, q = mask, dst = EA)
	m68kA64ClassBRA      // unconditional branch (block terminator; q = target)
	m68kA64ClassBcc      // conditional branch (block-ending; kind = cond, q = target)
	m68kA64ClassDBcc     // decrement and branch (block-ending; kind = cond, q = target)
	m68kA64ClassBSR      // branch to subroutine (block terminator; q = target)
	m68kA64ClassJSR      // jump to subroutine (block terminator; src = EA)
	m68kA64ClassJMP      // jump (block terminator; src = EA)
	m68kA64ClassRTS      // return from subroutine (block terminator)
)

// Shift/rotate kinds (bits 4-3 of the opcode), carried in q; sub carries
// direction (true = right).
const (
	m68kA64ShiftAS = iota
	m68kA64ShiftLS
	m68kA64ShiftROX
	m68kA64ShiftRO
)

const (
	m68kA64ALUAdd = iota
	m68kA64ALUSub
	m68kA64ALUCmp
	m68kA64ALUAnd
	m68kA64ALUOr
	m68kA64ALUEor
)

// aluOp is carried in q for the ALU classes.

func m68kA64SizeFromBits(bits uint16) uint32 {
	switch bits {
	case 0:
		return 1
	case 1:
		return 2
	case 2:
		return 4
	}
	return 0
}

// m68kA64Decode classifies one instruction for the slice-2 emitter.
// Returns ok=false when the shape is not supported natively. The decoded
// extension-word count must agree with the scanner's instruction length;
// any disagreement rejects the instruction rather than risk a wrong PC.
func m68kA64Decode(ji *M68KJITInstr, memory []byte, instrPC uint32) (m68kA64DecodedOp, bool) {
	dec, ok := m68kA64DecodeInner(ji, memory, instrPC)
	if !ok {
		return dec, false
	}
	if 2+uint32(dec.src.ext+dec.dst.ext)*2 != uint32(ji.length) {
		return m68kA64DecodedOp{}, false
	}
	return dec, true
}

func m68kA64DecodeInner(ji *M68KJITInstr, memory []byte, instrPC uint32) (m68kA64DecodedOp, bool) {
	op := ji.opcode
	extPC := instrPC + 2
	bad := m68kA64DecodedOp{}

	switch {
	case op == 0x4E71:
		return m68kA64DecodedOp{class: m68kA64ClassNOP}, true

	case op&0xF100 == 0x7000: // MOVEQ
		return m68kA64DecodedOp{class: m68kA64ClassMOVEQ}, true

	case op&0xC000 == 0x0000 && op&0x3000 != 0x0000: // MOVE/MOVEA
		var size uint32
		switch op & 0x3000 {
		case 0x1000:
			size = 1
		case 0x3000:
			size = 2
		case 0x2000:
			size = 4
		}
		srcMode := (op >> 3) & 7
		srcReg := op & 7
		dstReg := (op >> 9) & 7
		dstMode := (op >> 6) & 7
		src, ok := m68kA64ParseEA(srcMode, srcReg, size, memory, extPC)
		if !ok {
			return bad, false
		}
		extPC += uint32(src.ext) * 2
		if dstMode == 1 { // MOVEA
			if size == 1 {
				return bad, false
			}
			return m68kA64DecodedOp{class: m68kA64ClassMOVEA, size: size, src: src,
				dst: m68kA64EA{kind: m68kA64EAAn, reg: dstReg}}, true
		}
		dst, ok := m68kA64ParseEA(dstMode, dstReg, size, memory, extPC)
		if !ok || dst.kind == m68kA64EAImm || dst.kind == m68kA64EAAn || dst.pcRel {
			return bad, false
		}
		d := m68kA64DecodedOp{class: m68kA64ClassMOVE, size: size, src: src, dst: dst}
		if 2+uint32(src.ext+dst.ext)*2 != uint32(ji.length) {
			return bad, false // decode length disagrees with the scanner
		}
		return d, true

	case op&0xFFC0 == 0x4AC0: // TAS <ea> (checked before TST; TST's mask catches TAS)
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, 1, memory, extPC)
		if !ok || ea.kind == m68kA64EAAn || ea.kind == m68kA64EAImm || ea.pcRel {
			return bad, false
		}
		return m68kA64DecodedOp{class: m68kA64ClassTAS, size: 1, dst: ea}, true

	case op&0xFF00 == 0x4A00: // TST
		size := m68kA64SizeFromBits((op >> 6) & 3)
		if size == 0 {
			return bad, false
		}
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, size, memory, extPC)
		if !ok || ea.kind == m68kA64EAImm || ea.kind == m68kA64EAAn || ea.pcRel {
			return bad, false
		}
		return m68kA64DecodedOp{class: m68kA64ClassTST, size: size, src: ea}, true

	case op&0xFF00 == 0x4200: // CLR
		size := m68kA64SizeFromBits((op >> 6) & 3)
		if size == 0 {
			return bad, false
		}
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, size, memory, extPC)
		if !ok || ea.kind == m68kA64EAImm || ea.kind == m68kA64EAAn || ea.pcRel {
			return bad, false
		}
		return m68kA64DecodedOp{class: m68kA64ClassCLR, size: size, dst: ea}, true

	case op&0xFFF8 == 0x4880 || op&0xFFF8 == 0x48C0 || op&0xFFF8 == 0x49C0: // EXT/EXTB
		// Checked before LEA: 0x49C0 also matches the LEA opcode mask.
		return m68kA64DecodedOp{class: m68kA64ClassEXT, kind: int((op >> 6) & 7),
			dst: m68kA64EA{kind: m68kA64EADn, reg: op & 7}}, true

	case op&0xF1C0 == 0x41C0: // LEA
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, 4, memory, extPC)
		if !ok || !ea.isMem() || ea.kind == m68kA64EAPost || ea.kind == m68kA64EAPre {
			return bad, false
		}
		return m68kA64DecodedOp{class: m68kA64ClassLEA, size: 4, src: ea,
			dst: m68kA64EA{kind: m68kA64EAAn, reg: (op >> 9) & 7}}, true

	case op&0xFB80 == 0x4880: // MOVEM (checked after EXT, which owns the mode-0 forms)
		if int(extPC)+2 > len(memory) {
			return bad, false
		}
		mask := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
		memToReg := op&0x0400 != 0
		size := uint32(2)
		if op&0x0040 != 0 {
			size = 4
		}
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, size, memory, extPC+2)
		if !ok {
			return bad, false
		}
		switch ea.kind {
		case m68kA64EAInd, m68kA64EADisp, m68kA64EAAbsL, m68kA64EAIndex:
			if ea.pcRel && !memToReg {
				return bad, false // PC-relative is a mem-to-reg source only
			}
		case m68kA64EAPost:
			if !memToReg {
				return bad, false // (An)+ is mem-to-reg only
			}
		case m68kA64EAPre:
			if memToReg {
				return bad, false // -(An) is reg-to-mem only
			}
		default:
			return bad, false // Dn, An, immediate invalid
		}
		ea.ext++ // the register-mask extension word
		return m68kA64DecodedOp{class: m68kA64ClassMOVEM, size: size, sub: memToReg,
			q: uint32(mask), dst: ea}, true

	case op&0xF0C0 == 0x50C0 && (op>>3)&7 != 1: // Scc <ea> (mode An is DBcc)
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, 1, memory, extPC)
		if !ok || ea.kind == m68kA64EAAn || ea.kind == m68kA64EAImm || ea.pcRel {
			return bad, false
		}
		return m68kA64DecodedOp{class: m68kA64ClassScc, size: 1,
			kind: int((op >> 8) & 0xF), dst: ea}, true

	case op&0xF000 == 0x5000: // ADDQ/SUBQ
		size := m68kA64SizeFromBits((op >> 6) & 3)
		if size == 0 {
			return bad, false
		}
		q := uint32((op >> 9) & 7)
		if q == 0 {
			q = 8
		}
		sub := op&0x0100 != 0
		mode := (op >> 3) & 7
		reg := op & 7
		if mode == 1 { // An: no flags, full 32-bit regardless of size; byte illegal
			if size == 1 {
				return bad, false
			}
			return m68kA64DecodedOp{class: m68kA64ClassQuickA, size: size, q: q, sub: sub,
				dst: m68kA64EA{kind: m68kA64EAAn, reg: reg}}, true
		}
		ea, ok := m68kA64ParseEA(mode, reg, size, memory, extPC)
		if !ok || ea.kind == m68kA64EAImm || ea.pcRel {
			return bad, false
		}
		if ea.kind == m68kA64EADn {
			return m68kA64DecodedOp{class: m68kA64ClassQuickD, size: size, q: q, sub: sub, dst: ea}, true
		}
		return m68kA64DecodedOp{class: m68kA64ClassQuickMem, size: size, q: q, sub: sub, dst: ea}, true

	case op&0xFFC0 == 0x4840: // SWAP (mode 0) / PEA (memory modes)
		if op&0x0038 == 0 {
			return m68kA64DecodedOp{class: m68kA64ClassSWAP,
				dst: m68kA64EA{kind: m68kA64EADn, reg: op & 7}}, true
		}
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, 4, memory, extPC)
		if !ok || !ea.isMem() || ea.kind == m68kA64EAPost || ea.kind == m68kA64EAPre {
			return bad, false
		}
		return m68kA64DecodedOp{class: m68kA64ClassPEA, size: 4, src: ea}, true

	case op&0xFF00 == 0x4400 || op&0xFF00 == 0x4600: // NEG / NOT
		size := m68kA64SizeFromBits((op >> 6) & 3)
		if size == 0 {
			return bad, false
		}
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, size, memory, extPC)
		if !ok || ea.kind == m68kA64EAImm || ea.kind == m68kA64EAAn || ea.pcRel {
			return bad, false
		}
		class := m68kA64ClassNEG
		if op&0xFF00 == 0x4600 {
			class = m68kA64ClassNOT
		}
		return m68kA64DecodedOp{class: class, size: size, dst: ea}, true

	case op&0xF000 == 0x0000 && (op&0xFF00 == 0x0000 || op&0xFF00 == 0x0200 ||
		op&0xFF00 == 0x0400 || op&0xFF00 == 0x0600 ||
		op&0xFF00 == 0x0A00 || op&0xFF00 == 0x0C00): // ORI/ANDI/SUBI/ADDI/EORI/CMPI
		size := m68kA64SizeFromBits((op >> 6) & 3)
		if size == 0 {
			return bad, false
		}
		imm, okImm := m68kA64ParseEA(7, 4, size, memory, extPC)
		if !okImm {
			return bad, false
		}
		extPC += uint32(imm.ext) * 2
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, size, memory, extPC)
		if !ok || ea.kind == m68kA64EAImm || ea.kind == m68kA64EAAn || ea.pcRel {
			// Rejects the CCR/SR forms (EA = immediate) along with
			// unsupported destinations.
			return bad, false
		}
		var alu int
		switch op & 0xFF00 {
		case 0x0000:
			alu = m68kA64ALUOr
		case 0x0200:
			alu = m68kA64ALUAnd
		case 0x0400:
			alu = m68kA64ALUSub
		case 0x0600:
			alu = m68kA64ALUAdd
		case 0x0A00:
			alu = m68kA64ALUEor
		case 0x0C00:
			alu = m68kA64ALUCmp
		}
		if ea.kind == m68kA64EADn {
			return m68kA64DecodedOp{class: m68kA64ClassALUToD, size: size, q: uint32(alu),
				src: imm, dst: ea}, true
		}
		// Memory destination, including CMPI (which reads, compares and
		// commits EA side effects without writing back).
		return m68kA64DecodedOp{class: m68kA64ClassALUToMem, size: size, q: uint32(alu),
			src: imm, dst: ea}, true

	case op&0xF000 == 0xE000 && (op>>6)&3 == 3: // memory shift/rotate by one
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, 2, memory, extPC)
		if !ok || !ea.isMem() || ea.pcRel {
			return bad, false
		}
		return m68kA64DecodedOp{class: m68kA64ClassShiftMem, size: 2,
			kind: int((op >> 8) & 7), dst: ea}, true

	case op&0xF100 == 0x0100: // BTST/BCHG/BCLR/BSET Dn,<ea> (dynamic bit)
		operation := int((op >> 6) & 3)
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, 1, memory, extPC)
		if !ok || ea.kind == m68kA64EAAn || ea.kind == m68kA64EAImm {
			return bad, false // rejects MOVEP (mode An) and illegal forms
		}
		if operation != 0 && ea.pcRel {
			return bad, false // only BTST may read a PC-relative operand
		}
		return m68kA64DecodedOp{class: m68kA64ClassBitOp, kind: operation, sub: true,
			q: uint32((op >> 9) & 7), dst: ea}, true

	case op&0xFF00 == 0x0800: // BTST/BCHG/BCLR/BSET #n,<ea> (immediate bit)
		operation := int((op >> 6) & 3)
		if int(extPC)+2 > len(memory) {
			return bad, false
		}
		bitNum := uint32(memory[extPC])<<8 | uint32(memory[extPC+1])
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, 1, memory, extPC+2)
		if !ok || ea.kind == m68kA64EAAn || ea.kind == m68kA64EAImm {
			return bad, false
		}
		if operation != 0 && ea.pcRel {
			return bad, false
		}
		ea.ext++ // the bit-number extension word
		return m68kA64DecodedOp{class: m68kA64ClassBitOp, kind: operation, sub: false,
			q: bitNum & 0xFF, dst: ea}, true

	case op&0xF1C0 == 0xC0C0 || op&0xF1C0 == 0xC1C0: // MULU/MULS.W <ea>,Dn
		src, ok := m68kA64ParseEA((op>>3)&7, op&7, 2, memory, extPC)
		if !ok || src.kind == m68kA64EAAn {
			return bad, false
		}
		return m68kA64DecodedOp{class: m68kA64ClassMulW, size: 2, sub: op&0x0100 != 0,
			src: src, dst: m68kA64EA{kind: m68kA64EADn, reg: (op >> 9) & 7}}, true

	case op&0xF1C0 == 0x80C0 || op&0xF1C0 == 0x81C0: // DIVU/DIVS.W <ea>,Dn
		src, ok := m68kA64ParseEA((op>>3)&7, op&7, 2, memory, extPC)
		// Reject the pre/post modes: ExecDivu/ExecDivs resolve the memory
		// source with GetEffectiveAddress, which does not commit the (An)+
		// or -(An) side effect the MUL path does, so only side-effect-free
		// sources are safe to lower without diverging.
		if !ok || src.kind == m68kA64EAAn || src.kind == m68kA64EAPost || src.kind == m68kA64EAPre {
			return bad, false
		}
		return m68kA64DecodedOp{class: m68kA64ClassDivW, size: 2, sub: op&0x0100 != 0,
			src: src, dst: m68kA64EA{kind: m68kA64EADn, reg: (op >> 9) & 7}}, true

	// Register-count shift/rotate (op&0x0020 set) stays interpreter fallback:
	// the count is a runtime Dn value with clamp/modulo and count==0 CCR
	// preservation, so lowering it is an optimisation, not a correctness
	// capability. Fallback reproduces ExecShiftRotate exactly. Deferred to a
	// milestone 4 optimisation slice.
	case op&0xF000 == 0xE000 && (op>>6)&3 != 3 && op&0x0020 == 0: // shift/rotate, imm count, Dn
		size := m68kA64SizeFromBits((op >> 6) & 3)
		c := uint32((op >> 9) & 7)
		if c == 0 {
			c = 8
		}
		return m68kA64DecodedOp{class: m68kA64ClassShift, size: size, q: c,
			kind: int((op >> 3) & 3), sub: op&0x0100 == 0,
			dst: m68kA64EA{kind: m68kA64EADn, reg: op & 7}}, true

	case op&0xF000 == 0xD000 || op&0xF000 == 0x9000 || // ADD/SUB
		op&0xF000 == 0xC000 || op&0xF000 == 0x8000 || // AND/OR
		op&0xF000 == 0xB000: // CMP/EOR
		sizeBits := (op >> 6) & 3
		size := m68kA64SizeFromBits(sizeBits)
		if size == 0 {
			return bad, false // opmode 011/111 (ADDA/CMPA/etc) unsupported
		}
		dn := (op >> 9) & 7
		toEA := op&0x0100 != 0
		var alu int
		switch op & 0xF000 {
		case 0xD000:
			alu = m68kA64ALUAdd
		case 0x9000:
			alu = m68kA64ALUSub
		case 0xC000:
			alu = m68kA64ALUAnd
		case 0x8000:
			alu = m68kA64ALUOr
		case 0xB000:
			if toEA {
				alu = m68kA64ALUEor
			} else {
				alu = m68kA64ALUCmp
			}
		}
		if !toEA || alu == m68kA64ALUEor {
			// <ea>,Dn (or EOR Dn,<ea> where the EA may also be Dn)
			ea, ok := m68kA64ParseEA((op>>3)&7, op&7, size, memory, extPC)
			if !ok {
				return bad, false
			}
			if ea.kind == m68kA64EAAn && !(alu == m68kA64ALUCmp && size >= 2) {
				return bad, false // An source only legal for CMP.W/.L here
			}
			if alu == m68kA64ALUEor {
				if ea.kind == m68kA64EAImm || ea.kind == m68kA64EAAn {
					return bad, false
				}
				if ea.kind == m68kA64EADn {
					return m68kA64DecodedOp{class: m68kA64ClassALUToD, size: size, q: uint32(alu),
						src: m68kA64EA{kind: m68kA64EADn, reg: dn}, dst: ea}, true
				}
				return m68kA64DecodedOp{class: m68kA64ClassALUToMem, size: size, q: uint32(alu),
					src: m68kA64EA{kind: m68kA64EADn, reg: dn}, dst: ea}, true
			}
			return m68kA64DecodedOp{class: m68kA64ClassALUToD, size: size, q: uint32(alu),
				src: ea, dst: m68kA64EA{kind: m68kA64EADn, reg: dn}}, true
		}
		// Dn,<ea>: memory destination read-modify-write.
		if alu == m68kA64ALUCmp {
			return bad, false
		}
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, size, memory, extPC)
		if !ok || !ea.isMem() || ea.pcRel {
			return bad, false
		}
		return m68kA64DecodedOp{class: m68kA64ClassALUToMem, size: size, q: uint32(alu),
			src: m68kA64EA{kind: m68kA64EADn, reg: dn}, dst: ea}, true
	}
	return bad, false
}

// m68kARM64InstrSupported reports whether the arm64 emitter can lower the
// instruction natively. Must stay in exact lockstep with m68kA64EmitInstr.
func m68kARM64InstrSupported(ji *M68KJITInstr, memory []byte, instrPC uint32) bool {
	_, ok := m68kA64Decode(ji, memory, instrPC)
	return ok
}

// m68kA64DecodeBranch classifies a block-ending control transfer: BRA/BSR
// (byte, word or long displacement), Bcc (conditions 2-15), DBcc, JSR and
// JMP with a lowered EA ((An), (d16,An) or (xxx).L), and RTS.
//
// The interpreter halts the machine when a taken BRA/BSR/Bcc target reaches
// ProfileTopOfRAM-2 (decodeGroup6/ExecBRA); such branches are rejected here
// so the interpreter keeps that behaviour (for BSR the interpreter pushes
// the return address before halting, which the fallback reproduces). DBcc,
// JSR and JMP apply their targets unchecked in the interpreter, so no
// target check is made for them.
func m68kA64DecodeBranch(ji *M68KJITInstr, memory []byte, instrPC uint32, topOfRAM uint32) (m68kA64DecodedOp, bool) {
	op := ji.opcode
	bad := m68kA64DecodedOp{}
	rd16 := func(off uint32) (uint16, bool) {
		p := instrPC + 2 + off
		if int(p)+2 > len(memory) {
			return 0, false
		}
		return uint16(memory[p])<<8 | uint16(memory[p+1]), true
	}
	switch {
	case op&0xF000 == 0x6000: // BRA/BSR/Bcc
		cond := (op >> 8) & 0xF
		disp8 := int8(op)
		var target uint32
		ext := 0
		switch disp8 {
		case 0: // word displacement
			w, ok := rd16(0)
			if !ok {
				return bad, false
			}
			target = instrPC + 2 + uint32(int32(int16(w)))
			ext = 1
		case -1: // long displacement (68020)
			hi, ok1 := rd16(0)
			lo, ok2 := rd16(2)
			if !ok1 || !ok2 {
				return bad, false
			}
			target = instrPC + 2 + (uint32(hi)<<16 | uint32(lo))
			ext = 2
		default:
			target = instrPC + 2 + uint32(int32(disp8))
		}
		if 2+uint32(ext)*2 != uint32(ji.length) {
			return bad, false
		}
		if target >= topOfRAM-M68K_WORD_SIZE {
			return bad, false // interpreter halts on this taken target
		}
		class := m68kA64ClassBcc
		switch cond {
		case 0:
			class = m68kA64ClassBRA
		case 1:
			class = m68kA64ClassBSR
		}
		return m68kA64DecodedOp{class: class, kind: int(cond), q: target}, true

	case op == 0x4E75: // RTS
		if ji.length != 2 {
			return bad, false
		}
		return m68kA64DecodedOp{class: m68kA64ClassRTS}, true

	case op&0xFFC0 == 0x4E80 || op&0xFFC0 == 0x4EC0: // JSR/JMP
		ea, ok := m68kA64ParseEA((op>>3)&7, op&7, 4, memory, instrPC+2)
		if !ok {
			return bad, false
		}
		switch ea.kind {
		case m68kA64EAInd, m68kA64EADisp, m68kA64EAAbsL:
		default:
			return bad, false // control EA forms outside the lowered set
		}
		if 2+uint32(ea.ext)*2 != uint32(ji.length) {
			return bad, false
		}
		class := m68kA64ClassJMP
		if op&0xFFC0 == 0x4E80 {
			class = m68kA64ClassJSR
		}
		return m68kA64DecodedOp{class: class, src: ea}, true

	case op&0xF0F8 == 0x50C8: // DBcc
		if ji.length != 4 {
			return bad, false
		}
		w, ok := rd16(0)
		if !ok {
			return bad, false
		}
		target := instrPC + 2 + uint32(int32(int16(w)))
		return m68kA64DecodedOp{class: m68kA64ClassDBcc, kind: int((op >> 8) & 0xF), q: target,
			dst: m68kA64EA{kind: m68kA64EADn, reg: op & 7}}, true
	}
	return bad, false
}

// m68kARM64SupportedPrefix returns the number of leading instructions the
// arm64 backend can execute natively. A supported control transfer (BRA,
// Bcc, DBcc, BSR, JSR, JMP, RTS) ends the prefix and is included as its
// final instruction; any other block terminator or unsupported instruction
// ends the prefix without being included.
func m68kARM64SupportedPrefix(instrs []M68KJITInstr, memory []byte, startPC uint32, topOfRAM uint32) int {
	n := 0
	for i := range instrs {
		ji := &instrs[i]
		instrPC := startPC + ji.pcOffset
		if _, ok := m68kA64DecodeBranch(ji, memory, instrPC, topOfRAM); ok {
			return n + 1
		}
		if m68kIsBlockTerminator(ji.opcode) || !m68kARM64InstrSupported(ji, memory, instrPC) {
			break
		}
		n++
	}
	return n
}

// ===========================================================================
// Instruction lowering
// ===========================================================================

// loadOperand materialises a source operand's sized value into dst.
// Memory operands are guarded; their side effects are committed only after
// the guard passes (commit=true). addrOut receives the access address
// register when the operand was memory (for callers that must not commit
// yet), else 0xFF.
func (e *m68kA64Emitter) loadOperand(ea *m68kA64EA, size uint32, dst byte, commit bool) {
	cb := e.cb
	switch ea.kind {
	case m68kA64EADn:
		m68kA64LoadD(cb, dst, ea.reg)
	case m68kA64EAAn:
		m68kA64LoadA(cb, dst, ea.reg)
	case m68kA64EAImm:
		m68kA64MovImm32(cb, dst, ea.imm)
	default:
		m68kA64EmitEAAddr(cb, ea, size, m68kA64Tmp0, 0)
		e.emitGuard(m68kA64Tmp0, size)
		m68kA64EmitLoadBE(cb, dst, m68kA64Tmp0, size)
		if commit {
			m68kA64EmitEACommit(cb, ea, size, m68kA64Tmp0, m68kA64Tmp6)
		}
	}
}

// mergeSized writes the sized low bits of val into data register dn,
// preserving the untouched high bits. val must survive; scratch regs a/b
// are clobbered.
func m68kA64MergeSizedToD(cb *CodeBuffer, dn uint16, val, a, b byte, size uint32) {
	if size == 4 {
		m68kA64StoreD(cb, val, dn)
		return
	}
	m68kA64LoadD(cb, a, dn)
	if size == 1 {
		cb.Emit32(arm64UXTB(b, val))
		cb.Emit32(arm64LSR_W_imm(a, a, 8))
		cb.Emit32(arm64ORR_W_lsl(a, b, a, 8))
	} else {
		cb.Emit32(arm64UXTH(b, val))
		cb.Emit32(arm64LSR_W_imm(a, a, 16))
		cb.Emit32(arm64ORR_W_lsl(a, b, a, 16))
	}
	m68kA64StoreD(cb, a, dn)
}

// emitSizedArith performs dst = dst op src at the given size with correct
// NZCV, using the shift-to-top trick for byte/word. Result (sized, in the
// low bits, zero-extended) lands in rOut. NZCV is live on return.
// rSrc/rDst are clobbered.
func m68kA64EmitSizedArith(cb *CodeBuffer, sub bool, rDst, rSrc, rOut byte, size uint32) {
	sh := m68kA64ShiftForSize(size)
	if sh != 0 {
		cb.Emit32(arm64LSL_W_imm(rDst, rDst, sh))
		cb.Emit32(arm64LSL_W_imm(rSrc, rSrc, sh))
	}
	if sub {
		cb.Emit32(arm64SUBS_W(rOut, rDst, rSrc))
	} else {
		cb.Emit32(arm64ADDS_W(rOut, rDst, rSrc))
	}
	if sh != 0 {
		cb.Emit32(arm64LSR_W_imm(rOut, rOut, sh))
	}
}

func (e *m68kA64Emitter) emitInstr(dec *m68kA64DecodedOp, ji *M68KJITInstr, instrPC uint32) error {
	cb := e.cb
	op := ji.opcode
	switch dec.class {
	case m68kA64ClassNOP:
		return nil

	case m68kA64ClassMOVEQ:
		dn := (op >> 9) & 7
		val := uint32(int32(int8(op)))
		m68kA64MovImm32(cb, m68kA64Tmp0, val)
		m68kA64StoreD(cb, m68kA64Tmp0, dn)
		m68kA64FlagsStatic(cb, int8(op) < 0, int8(op) == 0)
		return nil

	case m68kA64ClassMOVE:
		src, dst, size := &dec.src, &dec.dst, dec.size
		if !dst.isMem() {
			// Destination Dn.
			e.loadOperand(src, size, m68kA64Tmp1, true)
			m68kA64MergeSizedToD(cb, dst.reg, m68kA64Tmp1, m68kA64Tmp2, m68kA64Tmp3, size)
			if src.kind == m68kA64EAImm {
				neg := size == 4 && int32(src.imm) < 0 ||
					size == 2 && src.imm&0x8000 != 0 ||
					size == 1 && src.imm&0x80 != 0
				m68kA64FlagsStatic(cb, neg, src.imm == 0)
			} else {
				m68kA64FlagsMove(cb, m68kA64Tmp1, size)
			}
			return nil
		}
		// Destination memory: guard BOTH addresses before committing any
		// side effect, so a bail leaves the instruction fully unexecuted.
		srcMem := src.isMem()
		if srcMem {
			m68kA64EmitEAAddr(cb, src, size, m68kA64Tmp0, 0)
			e.emitGuard(m68kA64Tmp0, size)
		}
		extra := int32(0)
		if srcMem && (dst.kind == m68kA64EAPost || dst.kind == m68kA64EAPre || dst.kind == m68kA64EAInd || dst.kind == m68kA64EADisp) &&
			dst.reg == src.reg && (src.kind == m68kA64EAPost || src.kind == m68kA64EAPre) {
			// The uncommitted source side effect targets the same An the
			// destination EA reads: fold it in at compile time.
			extra = src.sideEffectDelta(size)
		}
		m68kA64EmitEAAddr(cb, dst, size, m68kA64Tmp2, extra)
		e.emitGuard(m68kA64Tmp2, size)
		// Both guards passed: load, commit source, store, commit dest.
		if srcMem {
			m68kA64EmitLoadBE(cb, m68kA64Tmp1, m68kA64Tmp0, size)
			m68kA64EmitEACommit(cb, src, size, m68kA64Tmp0, m68kA64Tmp6)
		} else {
			e.loadOperand(src, size, m68kA64Tmp1, true)
		}
		m68kA64EmitStoreBE(cb, m68kA64Tmp1, m68kA64Tmp2, m68kA64Tmp3, size)
		m68kA64EmitEACommit(cb, dst, size, m68kA64Tmp2, m68kA64Tmp6)
		if src.kind == m68kA64EAImm {
			neg := size == 4 && int32(src.imm) < 0 ||
				size == 2 && src.imm&0x8000 != 0 ||
				size == 1 && src.imm&0x80 != 0
			m68kA64FlagsStatic(cb, neg, src.imm == 0)
		} else {
			m68kA64FlagsMove(cb, m68kA64Tmp1, size)
		}
		return nil

	case m68kA64ClassMOVEA:
		e.loadOperand(&dec.src, dec.size, m68kA64Tmp1, true)
		if dec.size == 2 {
			cb.Emit32(arm64SXTH_W(m68kA64Tmp1, m68kA64Tmp1))
		}
		m68kA64StoreA(cb, m68kA64Tmp1, dec.dst.reg)
		return nil

	case m68kA64ClassTST:
		e.loadOperand(&dec.src, dec.size, m68kA64Tmp1, true)
		m68kA64FlagsMove(cb, m68kA64Tmp1, dec.size)
		return nil

	case m68kA64ClassCLR:
		dst, size := &dec.dst, dec.size
		if dst.kind == m68kA64EADn {
			if size == 4 {
				m68kA64StoreD(cb, 31, dst.reg)
			} else {
				m68kA64LoadD(cb, m68kA64Tmp0, dst.reg)
				sh := uint32(8)
				if size == 2 {
					sh = 16
				}
				cb.Emit32(arm64LSR_W_imm(m68kA64Tmp0, m68kA64Tmp0, sh))
				cb.Emit32(arm64LSL_W_imm(m68kA64Tmp0, m68kA64Tmp0, sh))
				m68kA64StoreD(cb, m68kA64Tmp0, dst.reg)
			}
		} else {
			m68kA64EmitEAAddr(cb, dst, size, m68kA64Tmp0, 0)
			e.emitGuard(m68kA64Tmp0, size)
			m68kA64EmitStoreBE(cb, 31, m68kA64Tmp0, m68kA64Tmp3, size)
			m68kA64EmitEACommit(cb, dst, size, m68kA64Tmp0, m68kA64Tmp6)
		}
		m68kA64FlagsStatic(cb, false, true)
		return nil

	case m68kA64ClassLEA:
		m68kA64EmitEAAddr(cb, &dec.src, 4, m68kA64Tmp0, 0)
		m68kA64StoreA(cb, m68kA64Tmp0, dec.dst.reg)
		return nil

	case m68kA64ClassALUToD:
		alu := int(dec.q)
		size := dec.size
		e.loadOperand(&dec.src, size, m68kA64Tmp1, true)
		m68kA64LoadD(cb, m68kA64Tmp2, dec.dst.reg)
		switch alu {
		case m68kA64ALUAdd, m68kA64ALUSub, m68kA64ALUCmp:
			m68kA64EmitSizedArith(cb, alu != m68kA64ALUAdd, m68kA64Tmp2, m68kA64Tmp1, m68kA64Tmp3, size)
			if alu != m68kA64ALUCmp {
				m68kA64MergeSizedToD(cb, dec.dst.reg, m68kA64Tmp3, m68kA64Tmp1, m68kA64Tmp2, size)
			}
			carry := byte(m68kA64CondCS)
			if alu != m68kA64ALUAdd {
				carry = m68kA64CondCC
			}
			m68kA64FlagsArith(cb, carry, alu == m68kA64ALUCmp)
		default:
			switch alu {
			case m68kA64ALUAnd:
				cb.Emit32(arm64AND_W(m68kA64Tmp3, m68kA64Tmp2, m68kA64Tmp1))
			case m68kA64ALUOr:
				cb.Emit32(arm64ORR_W(m68kA64Tmp3, m68kA64Tmp2, m68kA64Tmp1))
			case m68kA64ALUEor:
				cb.Emit32(arm64EOR_W(m68kA64Tmp3, m68kA64Tmp2, m68kA64Tmp1))
			}
			m68kA64MergeSizedToD(cb, dec.dst.reg, m68kA64Tmp3, m68kA64Tmp1, m68kA64Tmp2, size)
			m68kA64FlagsLogicPreserveVC(cb, m68kA64Tmp3, size)
		}
		return nil

	case m68kA64ClassALUToMem:
		alu := int(dec.q)
		size := dec.size
		dst := &dec.dst
		m68kA64EmitEAAddr(cb, dst, size, m68kA64Tmp0, 0)
		e.emitGuard(m68kA64Tmp0, size)
		m68kA64EmitLoadBE(cb, m68kA64Tmp1, m68kA64Tmp0, size) // old value
		if dec.src.kind == m68kA64EAImm {
			m68kA64MovImm32(cb, m68kA64Tmp2, dec.src.imm)
		} else {
			m68kA64LoadD(cb, m68kA64Tmp2, dec.src.reg) // Dn source
		}
		switch alu {
		case m68kA64ALUAdd, m68kA64ALUSub, m68kA64ALUCmp:
			m68kA64EmitSizedArith(cb, alu != m68kA64ALUAdd, m68kA64Tmp1, m68kA64Tmp2, m68kA64Tmp3, size)
			if alu != m68kA64ALUCmp {
				m68kA64EmitStoreBE(cb, m68kA64Tmp3, m68kA64Tmp0, m68kA64Tmp5, size)
			}
			m68kA64EmitEACommit(cb, dst, size, m68kA64Tmp0, m68kA64Tmp6)
			carry := byte(m68kA64CondCS)
			if alu != m68kA64ALUAdd {
				carry = m68kA64CondCC
			}
			m68kA64FlagsArith(cb, carry, alu == m68kA64ALUCmp)
		default:
			switch alu {
			case m68kA64ALUAnd:
				cb.Emit32(arm64AND_W(m68kA64Tmp3, m68kA64Tmp1, m68kA64Tmp2))
			case m68kA64ALUOr:
				cb.Emit32(arm64ORR_W(m68kA64Tmp3, m68kA64Tmp1, m68kA64Tmp2))
			case m68kA64ALUEor:
				cb.Emit32(arm64EOR_W(m68kA64Tmp3, m68kA64Tmp1, m68kA64Tmp2))
			}
			m68kA64EmitStoreBE(cb, m68kA64Tmp3, m68kA64Tmp0, m68kA64Tmp5, size)
			m68kA64EmitEACommit(cb, dst, size, m68kA64Tmp0, m68kA64Tmp6)
			m68kA64FlagsLogicPreserveVC(cb, m68kA64Tmp3, size)
		}
		return nil

	case m68kA64ClassQuickD:
		size := dec.size
		m68kA64LoadD(cb, m68kA64Tmp1, dec.dst.reg)
		m68kA64MovImm32(cb, m68kA64Tmp2, dec.q)
		m68kA64EmitSizedArith(cb, dec.sub, m68kA64Tmp1, m68kA64Tmp2, m68kA64Tmp3, size)
		m68kA64MergeSizedToD(cb, dec.dst.reg, m68kA64Tmp3, m68kA64Tmp1, m68kA64Tmp2, size)
		carry := byte(m68kA64CondCS)
		if dec.sub {
			carry = m68kA64CondCC
		}
		m68kA64FlagsArith(cb, carry, false)
		return nil

	case m68kA64ClassQuickA:
		// ADDQ/SUBQ to An: full 32-bit, CCR untouched.
		m68kA64LoadA(cb, m68kA64Tmp1, dec.dst.reg)
		if dec.sub {
			cb.Emit32(arm64SUB_W_imm(m68kA64Tmp1, m68kA64Tmp1, dec.q))
		} else {
			cb.Emit32(arm64ADD_W_imm(m68kA64Tmp1, m68kA64Tmp1, dec.q))
		}
		m68kA64StoreA(cb, m68kA64Tmp1, dec.dst.reg)
		return nil

	case m68kA64ClassNEG, m68kA64ClassNOT:
		size := dec.size
		dst := &dec.dst
		isNeg := dec.class == m68kA64ClassNEG
		mem := dst.isMem()
		if mem {
			m68kA64EmitEAAddr(cb, dst, size, m68kA64Tmp0, 0)
			e.emitGuard(m68kA64Tmp0, size)
			m68kA64EmitLoadBE(cb, m68kA64Tmp1, m68kA64Tmp0, size)
		} else {
			m68kA64LoadD(cb, m68kA64Tmp1, dst.reg)
		}
		if isNeg {
			// 0 - value, sized, with the interpreter's NEG flag rule
			// (X=C=value!=0, V=value==sign minimum) — exactly the ARM
			// SUBS-from-zero NZCV mapped through the SUB carry inversion.
			cb.Emit32(arm64MOVZ_W(m68kA64Tmp2, 0, 0))
			m68kA64EmitSizedArith(cb, true, m68kA64Tmp2, m68kA64Tmp1, m68kA64Tmp3, size)
		} else {
			cb.Emit32(arm64MVN_W(m68kA64Tmp3, m68kA64Tmp1))
		}
		if mem {
			m68kA64EmitStoreBE(cb, m68kA64Tmp3, m68kA64Tmp0, m68kA64Tmp5, size)
			m68kA64EmitEACommit(cb, dst, size, m68kA64Tmp0, m68kA64Tmp6)
		} else {
			m68kA64MergeSizedToD(cb, dst.reg, m68kA64Tmp3, m68kA64Tmp1, m68kA64Tmp2, size)
		}
		if isNeg {
			m68kA64FlagsArith(cb, m68kA64CondCC, false)
		} else {
			// NOT uses the interpreter's SetFlagsNZ: X, V and C preserved.
			m68kA64FlagsLogicPreserveVC(cb, m68kA64Tmp3, size)
		}
		return nil

	case m68kA64ClassSWAP:
		dn := dec.dst.reg
		m68kA64LoadD(cb, m68kA64Tmp1, dn)
		cb.Emit32(arm64LSR_W_imm(m68kA64Tmp2, m68kA64Tmp1, 16))
		cb.Emit32(arm64ORR_W_lsl(m68kA64Tmp2, m68kA64Tmp2, m68kA64Tmp1, 16))
		m68kA64StoreD(cb, m68kA64Tmp2, dn)
		// SWAP: N/Z from the 32-bit result, V and C cleared, X preserved.
		m68kA64FlagsMove(cb, m68kA64Tmp2, 4)
		return nil

	case m68kA64ClassEXT:
		dn := dec.dst.reg
		m68kA64LoadD(cb, m68kA64Tmp1, dn)
		switch dec.kind {
		case 2: // EXT.W: sign-extend low byte into the low word
			cb.Emit32(arm64SXTB_W(m68kA64Tmp2, m68kA64Tmp1))
			cb.Emit32(arm64UXTH(m68kA64Tmp2, m68kA64Tmp2))
			cb.Emit32(arm64LSR_W_imm(m68kA64Tmp1, m68kA64Tmp1, 16))
			cb.Emit32(arm64ORR_W_lsl(m68kA64Tmp2, m68kA64Tmp2, m68kA64Tmp1, 16))
			m68kA64StoreD(cb, m68kA64Tmp2, dn)
			m68kA64FlagsMove(cb, m68kA64Tmp2, 2)
		case 3: // EXT.L: sign-extend low word
			cb.Emit32(arm64SXTH_W(m68kA64Tmp2, m68kA64Tmp1))
			m68kA64StoreD(cb, m68kA64Tmp2, dn)
			m68kA64FlagsMove(cb, m68kA64Tmp2, 4)
		case 7: // EXTB.L: sign-extend low byte to long (68020)
			cb.Emit32(arm64SXTB_W(m68kA64Tmp2, m68kA64Tmp1))
			m68kA64StoreD(cb, m68kA64Tmp2, dn)
			m68kA64FlagsMove(cb, m68kA64Tmp2, 4)
		default:
			return fmt.Errorf("m68k arm64 emitter: EXT opmode %d at %08X", dec.kind, instrPC)
		}
		return nil

	case m68kA64ClassPEA:
		// Compute the effective address, then push it: A7 -= 4 with the
		// stack write guarded BEFORE A7 commits, so an I/O bail leaves the
		// instruction fully unexecuted. CCR is untouched.
		m68kA64EmitEAAddr(cb, &dec.src, 4, m68kA64Tmp1, 0)
		m68kA64LoadA(cb, m68kA64Tmp0, 7)
		cb.Emit32(arm64SUB_W_imm(m68kA64Tmp0, m68kA64Tmp0, 4))
		// Push32's stack-floor bus error applies to PEA as well.
		cb.Emit32(arm64LDR_imm(m68kA64Tmp2, m68kA64Ctx, m68kCtxOffStackLowerBoundPtr/8))
		cb.Emit32(arm64LDR_W_imm(m68kA64Tmp2, m68kA64Tmp2, 0))
		cb.Emit32(arm64CMP_W(m68kA64Tmp0, m68kA64Tmp2))
		e.branchToBail(m68kA64CondCC) // newSP < stackLowerBound
		e.emitGuard(m68kA64Tmp0, 4)
		m68kA64EmitStoreBE(cb, m68kA64Tmp1, m68kA64Tmp0, m68kA64Tmp3, 4)
		m68kA64StoreA(cb, m68kA64Tmp0, 7)
		return nil

	case m68kA64ClassMulW:
		// MULU/MULS.W: 16x16 -> 32. N/Z from the 32-bit product; V, C and X
		// preserved (SetFlagsNZ, an IE deviation from the M68000PRM).
		signed := dec.sub
		e.loadOperand(&dec.src, 2, m68kA64Tmp1, true)
		m68kA64LoadD(cb, m68kA64Tmp2, dec.dst.reg)
		if signed {
			cb.Emit32(arm64SXTH_W(m68kA64Tmp1, m68kA64Tmp1))
			cb.Emit32(arm64SXTH_W(m68kA64Tmp2, m68kA64Tmp2))
		} else {
			cb.Emit32(arm64UXTH(m68kA64Tmp1, m68kA64Tmp1))
			cb.Emit32(arm64UXTH(m68kA64Tmp2, m68kA64Tmp2))
		}
		cb.Emit32(arm64MUL_W(m68kA64Tmp3, m68kA64Tmp2, m68kA64Tmp1))
		m68kA64StoreD(cb, m68kA64Tmp3, dec.dst.reg)
		m68kA64FlagsLogicPreserveVC(cb, m68kA64Tmp3, 4)
		return nil

	case m68kA64ClassDivW:
		return e.emitDivW(dec)

	case m68kA64ClassBitOp:
		return e.emitBitOp(dec)

	case m68kA64ClassMOVEM:
		return e.emitMovem(dec)

	case m68kA64ClassScc:
		// Set a byte to 0xFF when the condition holds, else 0x00; CCR is not
		// affected (ExecScc).
		dst := &dec.dst
		e.emitCondTest(dec.kind, m68kA64Tmp0)                // 0/1
		cb.Emit32(arm64SUBS_W(m68kA64Tmp1, 31, m68kA64Tmp0)) // 0x00 or 0xFFFFFFFF
		if dst.kind == m68kA64EADn {
			m68kA64MergeSizedToD(cb, dst.reg, m68kA64Tmp1, m68kA64Tmp2, m68kA64Tmp3, 1)
		} else {
			m68kA64EmitEAAddr(cb, dst, 1, m68kA64Tmp2, 0)
			e.emitGuard(m68kA64Tmp2, 1)
			m68kA64EmitStoreBE(cb, m68kA64Tmp1, m68kA64Tmp2, m68kA64Tmp3, 1)
			m68kA64EmitEACommit(cb, dst, 1, m68kA64Tmp2, m68kA64Tmp6)
		}
		return nil

	case m68kA64ClassTAS:
		// Atomic test-and-set on a byte: N/Z from the original byte, V and C
		// cleared, X preserved, then bit 7 is set and written back (ExecTas).
		dst := &dec.dst
		if dst.kind == m68kA64EADn {
			m68kA64LoadD(cb, m68kA64Tmp1, dst.reg)
			m68kA64FlagsMove(cb, m68kA64Tmp1, 1)
			cb.Emit32(arm64MOVZ_W(m68kA64Tmp3, 0x80, 0))
			cb.Emit32(arm64ORR_W(m68kA64Tmp2, m68kA64Tmp1, m68kA64Tmp3))
			m68kA64MergeSizedToD(cb, dst.reg, m68kA64Tmp2, m68kA64Tmp3, m68kA64Tmp4, 1)
		} else {
			m68kA64EmitEAAddr(cb, dst, 1, m68kA64Tmp0, 0)
			e.emitGuard(m68kA64Tmp0, 1)
			m68kA64EmitLoadBE(cb, m68kA64Tmp1, m68kA64Tmp0, 1)
			m68kA64FlagsMove(cb, m68kA64Tmp1, 1)
			cb.Emit32(arm64MOVZ_W(m68kA64Tmp3, 0x80, 0))
			cb.Emit32(arm64ORR_W(m68kA64Tmp2, m68kA64Tmp1, m68kA64Tmp3))
			m68kA64EmitStoreBE(cb, m68kA64Tmp2, m68kA64Tmp0, m68kA64Tmp3, 1)
			m68kA64EmitEACommit(cb, dst, 1, m68kA64Tmp0, m68kA64Tmp6)
		}
		return nil

	case m68kA64ClassShift:
		return e.emitShift(dec, instrPC)

	case m68kA64ClassShiftMem:
		// Memory shift/rotate by one on a word. Parity with
		// ExecShiftRotateMemory: V cleared, C and X both take the shifted-out
		// bit (memory ROL/ROR set X too, unlike the register forms).
		dst := &dec.dst
		m68kA64EmitEAAddr(cb, dst, 2, m68kA64Tmp0, 0)
		e.emitGuard(m68kA64Tmp0, 2)
		m68kA64EmitLoadBE(cb, m68kA64Tmp1, m68kA64Tmp0, 2) // zero-extended word
		switch dec.kind {
		case 0: // ASR
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, 0, 1))
			cb.Emit32(arm64SXTH_W(m68kA64Tmp3, m68kA64Tmp1))
			cb.Emit32(arm64ASR_W_imm(m68kA64Tmp3, m68kA64Tmp3, 1))
		case 1, 3: // ASL / LSL
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, 15, 1))
			cb.Emit32(arm64LSL_W_imm(m68kA64Tmp3, m68kA64Tmp1, 1))
		case 2: // LSR
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, 0, 1))
			cb.Emit32(arm64LSR_W_imm(m68kA64Tmp3, m68kA64Tmp1, 1))
		case 4: // ROXR
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, 0, 1))
			cb.Emit32(arm64LSR_W_imm(m68kA64Tmp3, m68kA64Tmp1, 1))
			cb.Emit32(arm64UBFX_W(m68kA64Tmp6, m68kA64CCR, m68kA64BitX, 1))
			cb.Emit32(arm64ORR_W_lsl(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp6, 15))
		case 5: // ROXL
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, 15, 1))
			cb.Emit32(arm64LSL_W_imm(m68kA64Tmp3, m68kA64Tmp1, 1))
			cb.Emit32(arm64UBFX_W(m68kA64Tmp6, m68kA64CCR, m68kA64BitX, 1))
			cb.Emit32(arm64ORR_W(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp6))
		case 6: // ROR
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, 0, 1))
			cb.Emit32(arm64LSR_W_imm(m68kA64Tmp3, m68kA64Tmp1, 1))
			cb.Emit32(arm64ORR_W_lsl(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp2, 15))
		case 7: // ROL
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, 15, 1))
			cb.Emit32(arm64LSL_W_imm(m68kA64Tmp3, m68kA64Tmp1, 1))
			cb.Emit32(arm64ORR_W(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp2))
		default:
			return fmt.Errorf("m68k arm64 emitter: mem shift op %d at %08X", dec.kind, instrPC)
		}
		m68kA64EmitStoreBE(cb, m68kA64Tmp3, m68kA64Tmp0, m68kA64Tmp5, 2)
		m68kA64EmitEACommit(cb, dst, 2, m68kA64Tmp0, m68kA64Tmp6)
		m68kA64FlagsShift(cb, m68kA64Tmp3, 2, m68kA64Tmp2, 0xFF, false)
		return nil

	case m68kA64ClassQuickMem:
		size := dec.size
		dst := &dec.dst
		m68kA64EmitEAAddr(cb, dst, size, m68kA64Tmp0, 0)
		e.emitGuard(m68kA64Tmp0, size)
		m68kA64EmitLoadBE(cb, m68kA64Tmp1, m68kA64Tmp0, size)
		m68kA64MovImm32(cb, m68kA64Tmp2, dec.q)
		m68kA64EmitSizedArith(cb, dec.sub, m68kA64Tmp1, m68kA64Tmp2, m68kA64Tmp3, size)
		m68kA64EmitStoreBE(cb, m68kA64Tmp3, m68kA64Tmp0, m68kA64Tmp5, size)
		m68kA64EmitEACommit(cb, dst, size, m68kA64Tmp0, m68kA64Tmp6)
		carry := byte(m68kA64CondCS)
		if dec.sub {
			carry = m68kA64CondCC
		}
		m68kA64FlagsArith(cb, carry, false)
		return nil
	}
	return fmt.Errorf("m68k arm64 emitter: unsupported opcode %04X at %08X", op, instrPC)
}

// emitRangeGuard validates a contiguous guest access [startReg, startReg+len)
// against the RAM bounds and the I/O page bitmap, scanning every page the
// range touches (unlike emitGuard, which probes only the first and last
// bytes). A failure branches to the current instruction's bail stub, leaving
// the instruction unexecuted. startReg is preserved; Tmp2, Tmp3 and Tmp6 are
// clobbered, so startReg must not be one of them.
func (e *m68kA64Emitter) emitRangeGuard(startReg byte, length uint32) {
	cb := e.cb
	// Start in bounds (also guards the end-bound add against 32-bit wrap).
	cb.Emit32(arm64CMP_W(startReg, m68kA64MemSize))
	e.branchToBail(m68kA64CondCS)
	// End in bounds.
	cb.Emit32(arm64ADD_W_imm(m68kA64Tmp6, startReg, length))
	cb.Emit32(arm64CMP_W(m68kA64Tmp6, m68kA64MemSize))
	e.branchToBail(m68kA64CondHI)
	// I/O bitmap scan over every touched page.
	noBmp := e.localFwdCBZ(m68kA64IOBmp)
	cb.Emit32(arm64LSR_W_imm(m68kA64Tmp2, startReg, 8)) // current page
	cb.Emit32(arm64ADD_W_imm(m68kA64Tmp3, startReg, length-1))
	cb.Emit32(arm64LSR_W_imm(m68kA64Tmp3, m68kA64Tmp3, 8)) // last page
	loop := cb.Len()
	cb.Emit32(arm64CMP_W(m68kA64Tmp2, m68kA64IOBmpLen))
	pastBmp := e.localFwdBcond(m68kA64CondCS) // page beyond bitmap: plain RAM
	cb.Emit32(arm64LDRB_reg(m68kA64Tmp6, m68kA64IOBmp, m68kA64Tmp2))
	e.cbnzToBail(m68kA64Tmp6)
	e.bindLocal(pastBmp)
	cb.Emit32(arm64CMP_W(m68kA64Tmp2, m68kA64Tmp3))
	done := e.localFwdBcond(m68kA64CondCS) // current >= last page: finished
	cb.Emit32(arm64ADD_W_imm(m68kA64Tmp2, m68kA64Tmp2, 1))
	e.cb.Emit32(arm64B(0))
	// Patch the unconditional back-branch to the loop head.
	e.patchSite(m68kA64BranchSite{off: cb.Len() - 4, base: 0x14000000, imm26: true}, loop)
	e.bindLocal(done)
	e.bindLocal(noBmp)
}

// emitMovem lowers MOVEM in both directions. The register mask is a
// compile-time constant, so the transfers are unrolled. Every access lies in
// one contiguous span, guarded as a whole before any transfer so a bail
// leaves the instruction unexecuted (the interpreter then replays it). Parity
// with ExecMovem: predecrement writes registers A7..A0, D7..D0 decrementing
// the base before each write (committing it each step so a base register in
// the list stores its decremented value); every other form transfers D0..D7,
// A0..A7 ascending; word transfers to registers sign-extend to long; (An)+
// leaves the base at the final address; the CCR is untouched.
func (e *m68kA64Emitter) emitMovem(dec *m68kA64DecodedOp) error {
	cb := e.cb
	mask := uint16(dec.q)
	size := dec.size
	memToReg := dec.sub
	dst := &dec.dst
	predec := dst.kind == m68kA64EAPre
	postinc := dst.kind == m68kA64EAPost
	n := 0
	for i := 0; i < 16; i++ {
		if mask&(1<<uint(i)) != 0 {
			n++
		}
	}
	byteLen := uint32(n) * size
	if n == 0 {
		// Empty mask: no memory access. (An)+ still writes back the base
		// unchanged, which is a no-op; nothing else happens.
		return nil
	}

	// Base/running address into Tmp0 and span start into Tmp5 for the guard.
	if predec {
		m68kA64LoadA(cb, m68kA64Tmp0, dst.reg) // running base = An
		m68kA64MovImm32(cb, m68kA64Tmp5, byteLen)
		cb.Emit32(arm64SUBS_W(m68kA64Tmp5, m68kA64Tmp0, m68kA64Tmp5)) // span start = An - len
	} else {
		m68kA64EmitEAAddr(cb, dst, size, m68kA64Tmp0, 0)
		cb.Emit32(arm64MOV_W(m68kA64Tmp5, m68kA64Tmp0))
	}
	e.emitRangeGuard(m68kA64Tmp5, byteLen)

	switch {
	case predec:
		for i := 0; i < 16; i++ {
			if mask&(1<<uint(i)) == 0 {
				continue
			}
			cb.Emit32(arm64SUB_W_imm(m68kA64Tmp0, m68kA64Tmp0, size))
			m68kA64StoreA(cb, m68kA64Tmp0, dst.reg) // commit running base
			if i < 8 {
				m68kA64LoadA(cb, m68kA64Tmp1, uint16(7-i))
			} else {
				m68kA64LoadD(cb, m68kA64Tmp1, uint16(15-i))
			}
			m68kA64EmitStoreBE(cb, m68kA64Tmp1, m68kA64Tmp0, m68kA64Tmp3, size)
		}
	case !memToReg:
		for i := 0; i < 16; i++ {
			if mask&(1<<uint(i)) == 0 {
				continue
			}
			if i < 8 {
				m68kA64LoadD(cb, m68kA64Tmp1, uint16(i))
			} else {
				m68kA64LoadA(cb, m68kA64Tmp1, uint16(i-8))
			}
			m68kA64EmitStoreBE(cb, m68kA64Tmp1, m68kA64Tmp0, m68kA64Tmp3, size)
			cb.Emit32(arm64ADD_W_imm(m68kA64Tmp0, m68kA64Tmp0, size))
		}
	default: // mem-to-reg
		for i := 0; i < 16; i++ {
			if mask&(1<<uint(i)) == 0 {
				continue
			}
			m68kA64EmitLoadBE(cb, m68kA64Tmp1, m68kA64Tmp0, size)
			if size == 2 {
				cb.Emit32(arm64SXTH_W(m68kA64Tmp1, m68kA64Tmp1))
			}
			if i < 8 {
				m68kA64StoreD(cb, m68kA64Tmp1, uint16(i))
			} else {
				m68kA64StoreA(cb, m68kA64Tmp1, uint16(i-8))
			}
			cb.Emit32(arm64ADD_W_imm(m68kA64Tmp0, m68kA64Tmp0, size))
		}
		if postinc {
			m68kA64StoreA(cb, m68kA64Tmp0, dst.reg) // final address
		}
	}
	return nil
}

// emitBitOp lowers BTST/BCHG/BCLR/BSET. Parity with ExecBitManip: the tested
// bit sets Z (Z = tested bit was zero) and no other flag changes; a register
// destination is a long operand with the bit taken modulo 32, a memory
// destination is a byte with the bit modulo 8. BTST only tests; the others
// write the modified operand back.
func (e *m68kA64Emitter) emitBitOp(dec *m68kA64DecodedOp) error {
	cb := e.cb
	dst := &dec.dst
	memDst := dst.isMem()
	op := dec.kind

	// Bit number (mod 8 for memory bytes, mod 32 for long registers).
	if dec.sub {
		m68kA64LoadD(cb, m68kA64Tmp2, uint16(dec.q))
	} else {
		m68kA64MovImm32(cb, m68kA64Tmp2, dec.q)
	}
	modMask := uint32(31)
	if memDst {
		modMask = 7
	}
	m68kA64MovImm32(cb, m68kA64Tmp4, modMask)
	cb.Emit32(arm64AND_W(m68kA64Tmp2, m68kA64Tmp2, m68kA64Tmp4))
	// mask = 1 << bitNum
	cb.Emit32(arm64MOVZ_W(m68kA64Tmp3, 1, 0))
	cb.Emit32(arm64LSLV_W(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp2))

	// Load the operand.
	if memDst {
		m68kA64EmitEAAddr(cb, dst, 1, m68kA64Tmp0, 0)
		e.emitGuard(m68kA64Tmp0, 1)
		m68kA64EmitLoadBE(cb, m68kA64Tmp1, m68kA64Tmp0, 1)
	} else {
		m68kA64LoadD(cb, m68kA64Tmp1, dst.reg)
	}

	// Z = (operand & mask) == 0; all other CCR bits preserved.
	cb.Emit32(arm64AND_W(m68kA64Tmp4, m68kA64Tmp1, m68kA64Tmp3))
	cb.Emit32(arm64CMP_W_imm(m68kA64Tmp4, 0))
	cb.Emit32(arm64CSET_W(m68kA64Tmp4, m68kA64CondEQ))
	m68kA64MovImm32(cb, m68kA64Tmp5, 0x1F&^(1<<m68kA64BitZ))
	cb.Emit32(arm64AND_W(m68kA64CCR, m68kA64CCR, m68kA64Tmp5))
	cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, m68kA64Tmp4, m68kA64BitZ))

	if op != 0 { // BCHG/BCLR/BSET modify and write back
		switch op {
		case 1: // BCHG
			cb.Emit32(arm64EOR_W(m68kA64Tmp1, m68kA64Tmp1, m68kA64Tmp3))
		case 2: // BCLR
			cb.Emit32(arm64MVN_W(m68kA64Tmp5, m68kA64Tmp3))
			cb.Emit32(arm64AND_W(m68kA64Tmp1, m68kA64Tmp1, m68kA64Tmp5))
		case 3: // BSET
			cb.Emit32(arm64ORR_W(m68kA64Tmp1, m68kA64Tmp1, m68kA64Tmp3))
		}
		if memDst {
			m68kA64EmitStoreBE(cb, m68kA64Tmp1, m68kA64Tmp0, m68kA64Tmp5, 1)
		} else {
			m68kA64StoreD(cb, m68kA64Tmp1, dst.reg) // long register write-back
		}
	}
	if memDst {
		m68kA64EmitEACommit(cb, dst, 1, m68kA64Tmp0, m68kA64Tmp6)
	}
	return nil
}

// emitDivW lowers DIVU.W/DIVS.W <ea>,Dn. Parity with ExecDivu/ExecDivs:
// division by zero bails (leaving the instruction unexecuted so the
// interpreter raises the zero-divide exception); a quotient that does not fit
// in a signed/unsigned 16-bit word sets V and leaves Dn unchanged; otherwise
// Dn becomes (remainder<<16)|(quotient&0xFFFF) with N/Z from the quotient and
// V, C cleared, X preserved.
func (e *m68kA64Emitter) emitDivW(dec *m68kA64DecodedOp) error {
	cb := e.cb
	signed := dec.sub
	// Divisor (word) into Tmp1; no side effect commits (pre/post rejected).
	e.loadOperand(&dec.src, 2, m68kA64Tmp1, false)
	if signed {
		cb.Emit32(arm64SXTH_W(m68kA64Tmp1, m68kA64Tmp1))
	} else {
		cb.Emit32(arm64UXTH(m68kA64Tmp1, m68kA64Tmp1))
	}
	e.cbzToBail(m68kA64Tmp1) // divisor == 0: reproduce the trap via fallback

	m68kA64LoadD(cb, m68kA64Tmp2, dec.dst.reg) // full 32-bit dividend
	// Quotient into Tmp3, overflow test into the carry flag.
	if signed {
		cb.Emit32(arm64SDIV_W(m68kA64Tmp3, m68kA64Tmp2, m68kA64Tmp1))
		// Overflow when quotient is outside [-32768, 32767]. Biasing by
		// 0x8000 maps that to the unsigned range [0, 0xFFFF]; anything above
		// (including the wrapped INT_MIN/-1 case) fails the unsigned compare.
		m68kA64MovImm32(cb, m68kA64Tmp5, 0x8000)
		cb.Emit32(arm64ADDS_W(m68kA64Tmp4, m68kA64Tmp3, m68kA64Tmp5))
		m68kA64MovImm32(cb, m68kA64Tmp5, 0x10000)
		cb.Emit32(arm64CMP_W(m68kA64Tmp4, m68kA64Tmp5))
	} else {
		cb.Emit32(arm64UDIV_W(m68kA64Tmp3, m68kA64Tmp2, m68kA64Tmp1))
		m68kA64MovImm32(cb, m68kA64Tmp5, 0x10000)
		cb.Emit32(arm64CMP_W(m68kA64Tmp3, m68kA64Tmp5))
	}
	overflow := e.localFwdBcond(m68kA64CondCS) // quotient out of range

	// No overflow: remainder = dividend - quotient*divisor.
	cb.Emit32(arm64MSUB(m68kA64Tmp4, m68kA64Tmp3, m68kA64Tmp1, m68kA64Tmp2))
	cb.Emit32(arm64UXTH(m68kA64Tmp4, m68kA64Tmp4)) // remainder low word
	cb.Emit32(arm64UXTH(m68kA64Tmp5, m68kA64Tmp3)) // quotient low word
	cb.Emit32(arm64ORR_W_lsl(m68kA64Tmp5, m68kA64Tmp5, m68kA64Tmp4, 16))
	m68kA64StoreD(cb, m68kA64Tmp5, dec.dst.reg)
	// CCR: keep X, clear V and C, set N/Z from the quotient word.
	m68kA64MovImm32(cb, m68kA64Tmp6, 0x10)
	cb.Emit32(arm64AND_W(m68kA64CCR, m68kA64CCR, m68kA64Tmp6))
	cb.Emit32(arm64UXTH(m68kA64Tmp4, m68kA64Tmp3))
	cb.Emit32(arm64CMP_W_imm(m68kA64Tmp4, 0))
	cb.Emit32(arm64CSET_W(m68kA64Tmp4, m68kA64CondEQ)) // Z
	cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, m68kA64Tmp4, m68kA64BitZ))
	cb.Emit32(arm64UBFX_W(m68kA64Tmp4, m68kA64Tmp3, 15, 1)) // N = quotient bit 15
	cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, m68kA64Tmp4, m68kA64BitN))
	toEnd := e.localFwdB()

	// Overflow: Dn unchanged, keep X, clear N/Z/C, set V.
	e.bindLocal(overflow)
	m68kA64MovImm32(cb, m68kA64Tmp6, 0x10)
	cb.Emit32(arm64AND_W(m68kA64CCR, m68kA64CCR, m68kA64Tmp6))
	m68kA64MovImm32(cb, m68kA64Tmp6, 1<<m68kA64BitV)
	cb.Emit32(arm64ORR_W(m68kA64CCR, m68kA64CCR, m68kA64Tmp6))

	e.bindLocal(toEnd)
	return nil
}

// m68kA64FlagsShift assembles the CCR after a shift or rotate: N/Z from the
// sized result in rRes (preserved), C from the 0/1 bit in cReg, V from vReg
// (0xFF = cleared), X = C for shifts and ROXd, X preserved for ROL/ROR
// (keepX). Clobbers Tmp4, Tmp5, Tmp6.
func m68kA64FlagsShift(cb *CodeBuffer, rRes byte, size uint32, cReg, vReg byte, keepX bool) {
	if keepX {
		cb.Emit32(arm64UBFX_W(m68kA64Tmp4, m68kA64CCR, m68kA64BitX, 1))
	}
	rt := rRes
	if sh := m68kA64ShiftForSize(size); sh != 0 {
		cb.Emit32(arm64LSL_W_imm(m68kA64Tmp5, rRes, sh))
		rt = m68kA64Tmp5
	}
	cb.Emit32(arm64CMP_W_imm(rt, 0))
	cb.Emit32(arm64CSET_W(m68kA64Tmp6, m68kA64CondEQ)) // Z
	cb.Emit32(arm64CSET_W(m68kA64Tmp5, m68kA64CondMI)) // N
	x := cReg
	if keepX {
		x = m68kA64Tmp4
	}
	m68kA64AssembleCCR(cb, cReg, vReg, m68kA64Tmp6, m68kA64Tmp5, x)
}

// emitShift lowers an immediate-count shift or rotate on Dn. The count is
// compile-time (1..8), so every interpreter case — including the
// count-at-least-width forms and the interpreter's ASL carry rule (C and V
// are the OR of every shifted-out bit, not the last one) — reduces to
// straight-line bit extraction. Parity is with ExecShiftRotate, not the
// M68000PRM.
func (e *m68kA64Emitter) emitShift(dec *m68kA64DecodedOp, instrPC uint32) error {
	cb := e.cb
	size := dec.size
	c := dec.q
	w := size * 8
	dn := dec.dst.reg
	right := dec.sub

	// The interpreter applies the rotate modulo AFTER the immediate 0->8
	// mapping (GetShiftCount), so ROL.B #8 / ROR.B #8 have an effective
	// count of zero: a complete no-op that preserves the whole CCR.
	if dec.kind == m68kA64ShiftRO {
		c %= w
		if c == 0 {
			return nil
		}
	}

	// Sized, zero-extended value in Tmp1.
	m68kA64LoadD(cb, m68kA64Tmp1, dn)
	switch size {
	case 1:
		cb.Emit32(arm64UXTB(m68kA64Tmp1, m68kA64Tmp1))
	case 2:
		cb.Emit32(arm64UXTH(m68kA64Tmp1, m68kA64Tmp1))
	}
	maskRes := func() {
		switch size {
		case 1:
			cb.Emit32(arm64UXTB(m68kA64Tmp3, m68kA64Tmp3))
		case 2:
			cb.Emit32(arm64UXTH(m68kA64Tmp3, m68kA64Tmp3))
		}
	}
	csetNonZero := func(rd byte) { // rd = (value != 0)
		cb.Emit32(arm64CMP_W_imm(m68kA64Tmp1, 0))
		cb.Emit32(arm64CSET_W(rd, m68kA64CondEQ^1)) // NE
	}

	vReg := byte(0xFF)
	keepX := false
	switch dec.kind {
	case m68kA64ShiftLS:
		if c >= w {
			csetNonZero(m68kA64Tmp2)
			cb.Emit32(arm64MOVZ_W(m68kA64Tmp3, 0, 0))
		} else if right {
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, c-1, 1))
			cb.Emit32(arm64LSR_W_imm(m68kA64Tmp3, m68kA64Tmp1, c))
		} else {
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, w-c, 1))
			cb.Emit32(arm64LSL_W_imm(m68kA64Tmp3, m68kA64Tmp1, c))
			maskRes()
		}

	case m68kA64ShiftAS:
		if right {
			// Sign-extended source in Tmp5.
			switch size {
			case 1:
				cb.Emit32(arm64SXTB_W(m68kA64Tmp5, m68kA64Tmp1))
			case 2:
				cb.Emit32(arm64SXTH_W(m68kA64Tmp5, m68kA64Tmp1))
			default:
				cb.Emit32(arm64MOV_W(m68kA64Tmp5, m68kA64Tmp1))
			}
			if c >= w {
				csetNonZero(m68kA64Tmp2)
				cb.Emit32(arm64ASR_W_imm(m68kA64Tmp3, m68kA64Tmp5, 31))
			} else {
				cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, c-1, 1))
				cb.Emit32(arm64ASR_W_imm(m68kA64Tmp3, m68kA64Tmp5, c))
			}
			maskRes()
		} else { // ASL: C = V = OR of the shifted-out bits
			if c >= w {
				csetNonZero(m68kA64Tmp2)
				cb.Emit32(arm64MOVZ_W(m68kA64Tmp3, 0, 0))
			} else {
				cb.Emit32(arm64UBFX_W(m68kA64Tmp6, m68kA64Tmp1, w-c, c))
				cb.Emit32(arm64CMP_W_imm(m68kA64Tmp6, 0))
				cb.Emit32(arm64CSET_W(m68kA64Tmp2, m68kA64CondEQ^1)) // NE
				cb.Emit32(arm64LSL_W_imm(m68kA64Tmp3, m68kA64Tmp1, c))
				maskRes()
			}
			vReg = m68kA64Tmp2
		}

	case m68kA64ShiftRO:
		keepX = true
		if right {
			cb.Emit32(arm64LSR_W_imm(m68kA64Tmp3, m68kA64Tmp1, c))
			if w != c {
				cb.Emit32(arm64ORR_W_lsl(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp1, w-c))
			} else {
				cb.Emit32(arm64ORR_W(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp1))
			}
			maskRes()
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp3, w-1, 1)) // C = MSB of result
		} else {
			cb.Emit32(arm64LSL_W_imm(m68kA64Tmp3, m68kA64Tmp1, c))
			if w != c {
				cb.Emit32(arm64LSR_W_imm(m68kA64Tmp6, m68kA64Tmp1, w-c))
				cb.Emit32(arm64ORR_W(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp6))
			} else {
				cb.Emit32(arm64ORR_W(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp1))
			}
			maskRes()
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp3, 0, 1)) // C = LSB of result
		}

	case m68kA64ShiftROX:
		// Rotate through X: a width+1-bit rotate expressed with compile
		// time shifts. New X = C = the last bit rotated out of the value.
		cb.Emit32(arm64UBFX_W(m68kA64Tmp6, m68kA64CCR, m68kA64BitX, 1))
		if right {
			cb.Emit32(arm64LSR_W_imm(m68kA64Tmp3, m68kA64Tmp1, c))
			cb.Emit32(arm64ORR_W_lsl(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp6, w-c))
			if w+1-c < 32 {
				cb.Emit32(arm64LSL_W_imm(m68kA64Tmp5, m68kA64Tmp1, w+1-c))
				cb.Emit32(arm64ORR_W(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp5))
			}
			maskRes()
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, c-1, 1))
		} else {
			cb.Emit32(arm64LSL_W_imm(m68kA64Tmp3, m68kA64Tmp1, c))
			cb.Emit32(arm64ORR_W_lsl(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp6, c-1))
			if w+1-c < 32 {
				cb.Emit32(arm64LSR_W_imm(m68kA64Tmp5, m68kA64Tmp1, w+1-c))
				cb.Emit32(arm64ORR_W(m68kA64Tmp3, m68kA64Tmp3, m68kA64Tmp5))
			}
			maskRes()
			cb.Emit32(arm64UBFX_W(m68kA64Tmp2, m68kA64Tmp1, w-c, 1))
		}

	default:
		return fmt.Errorf("m68k arm64 emitter: shift kind %d at %08X", dec.kind, instrPC)
	}

	m68kA64MergeSizedToD(cb, dn, m68kA64Tmp3, m68kA64Tmp1, m68kA64Tmp6, size)
	m68kA64FlagsShift(cb, m68kA64Tmp3, size, m68kA64Tmp2, vReg, keepX)
	return nil
}

// ===========================================================================
// Block compilation
// ===========================================================================

// m68kCompileBlockARM64 compiles a fully supported straight-line prefix into
// native arm64 code. The emitted block loads live state on entry, executes
// the instructions, publishes CCR/RetPC/RetCount on exit and returns. A
// guarded memory access that fails takes the per-instruction bail stub:
// CCR/SR are flushed, RetPC is the faulting instruction, RetCount is the
// number of fully retired instructions, and NeedIOFallback is set.
// emitCondTest materialises an M68K condition (CheckCondition semantics) as
// 0/1 in rc, reading the live CCR in W4. Clobbers Tmp5 and Tmp6; callers
// needing Tmp5 afterwards must load it after this call.
func (e *m68kA64Emitter) emitCondTest(cond int, rc byte) {
	cb := e.cb
	// maskTest sets rc from a CCR bit mask: whenSet selects "any masked bit
	// set" versus "all masked bits clear".
	maskTest := func(mask uint32, whenSet bool) {
		m68kA64MovImm32(cb, m68kA64Tmp6, mask)
		cb.Emit32(arm64AND_W(rc, m68kA64CCR, m68kA64Tmp6))
		cb.Emit32(arm64CMP_W_imm(rc, 0))
		if whenSet {
			cb.Emit32(arm64CSET_W(rc, m68kA64CondNE))
		} else {
			cb.Emit32(arm64CSET_W(rc, m68kA64CondEQ))
		}
	}
	// nxv leaves N^V in bit 1 of Tmp6 (N is CCR bit 3, V is CCR bit 1).
	nxv := func() {
		cb.Emit32(arm64LSR_W_imm(m68kA64Tmp6, m68kA64CCR, 2))
		cb.Emit32(arm64EOR_W(m68kA64Tmp6, m68kA64Tmp6, m68kA64CCR))
	}
	switch cond {
	case M68K_CC_T:
		cb.Emit32(arm64MOVZ_W(rc, 1, 0))
	case M68K_CC_F:
		cb.Emit32(arm64MOVZ_W(rc, 0, 0))
	case M68K_CC_HI:
		maskTest(0x5, false) // C=0 and Z=0
	case M68K_CC_LS:
		maskTest(0x5, true) // C=1 or Z=1
	case M68K_CC_CC:
		maskTest(0x1, false)
	case M68K_CC_CS:
		maskTest(0x1, true)
	case M68K_CC_NE:
		maskTest(0x4, false)
	case M68K_CC_EQ:
		maskTest(0x4, true)
	case M68K_CC_VC:
		maskTest(0x2, false)
	case M68K_CC_VS:
		maskTest(0x2, true)
	case M68K_CC_PL:
		maskTest(0x8, false)
	case M68K_CC_MI:
		maskTest(0x8, true)
	case M68K_CC_GE, M68K_CC_LT:
		nxv()
		m68kA64MovImm32(cb, m68kA64Tmp5, 2)
		cb.Emit32(arm64AND_W(rc, m68kA64Tmp6, m68kA64Tmp5))
		cb.Emit32(arm64CMP_W_imm(rc, 0))
		if cond == M68K_CC_GE {
			cb.Emit32(arm64CSET_W(rc, m68kA64CondEQ)) // N==V
		} else {
			cb.Emit32(arm64CSET_W(rc, m68kA64CondNE)) // N!=V
		}
	case M68K_CC_GT, M68K_CC_LE:
		nxv()
		m68kA64MovImm32(cb, m68kA64Tmp5, 2)
		cb.Emit32(arm64AND_W(m68kA64Tmp6, m68kA64Tmp6, m68kA64Tmp5)) // (N^V)<<1
		m68kA64MovImm32(cb, m68kA64Tmp5, 4)
		cb.Emit32(arm64AND_W(rc, m68kA64CCR, m68kA64Tmp5)) // Z<<2
		cb.Emit32(arm64ORR_W(rc, rc, m68kA64Tmp6))
		cb.Emit32(arm64CMP_W_imm(rc, 0))
		if cond == M68K_CC_GT {
			cb.Emit32(arm64CSET_W(rc, m68kA64CondEQ)) // Z=0 and N==V
		} else {
			cb.Emit32(arm64CSET_W(rc, m68kA64CondNE)) // Z=1 or N!=V
		}
	}
}

// emitBranch lowers a block-ending control transfer. BRA resolves to a
// static exit PC; every other shape leaves the resume PC in Tmp5 at run
// time. DBcc parity with ExecDBcc: condition true takes the fallthrough with
// no decrement; otherwise the low word of Dn decrements (high word
// preserved) and the branch is taken unless the counter expires to -1.
// BSR/JSR push the return address through the guarded stack path and RTS
// pops through it; their guard failures bail with the terminator unexecuted
// so the interpreter reproduces Push32/Pop32 exception behaviour.
func (e *m68kA64Emitter) emitBranch(dec *m68kA64DecodedOp, ji *M68KJITInstr, instrPC uint32) {
	cb := e.cb
	fall := instrPC + uint32(ji.length)
	switch dec.class {
	case m68kA64ClassBRA:
		e.staticExit = dec.q
		e.hasStaticExit = true

	case m68kA64ClassBcc:
		e.emitCondTest(dec.kind, m68kA64Tmp0)
		m68kA64MovImm32(cb, m68kA64Tmp5, fall)
		skip := m68kA64BranchSite{off: cb.Len(), base: 0xB4000000 | uint32(m68kA64Tmp0)}
		cb.Emit32(arm64CBZ(m68kA64Tmp0, 0))
		m68kA64MovImm32(cb, m68kA64Tmp5, dec.q)
		e.patchSite(skip, cb.Len())
		e.dynExit = true

	case m68kA64ClassDBcc:
		e.emitCondTest(dec.kind, m68kA64Tmp0)
		m68kA64MovImm32(cb, m68kA64Tmp5, fall)
		condTrue := m68kA64BranchSite{off: cb.Len(), base: 0xB5000000 | uint32(m68kA64Tmp0)}
		cb.Emit32(arm64CBNZ(m68kA64Tmp0, 0))
		dn := dec.dst.reg
		m68kA64LoadD(cb, m68kA64Tmp1, dn)
		cb.Emit32(arm64UXTH(m68kA64Tmp2, m68kA64Tmp1)) // counter before decrement
		cb.Emit32(arm64SUB_W_imm(m68kA64Tmp3, m68kA64Tmp2, 1))
		m68kA64MergeSizedToD(cb, dn, m68kA64Tmp3, m68kA64Tmp4, m68kA64Tmp6, 2)
		// Counter was zero: it expired to -1, fall through.
		expired := m68kA64BranchSite{off: cb.Len(), base: 0xB4000000 | uint32(m68kA64Tmp2)}
		cb.Emit32(arm64CBZ(m68kA64Tmp2, 0))
		m68kA64MovImm32(cb, m68kA64Tmp5, dec.q)
		done := cb.Len()
		e.patchSite(condTrue, done)
		e.patchSite(expired, done)
		e.dynExit = true

	case m68kA64ClassBSR:
		// Target is static and range-checked at decode. The push is guarded
		// before A7 commits: a bail leaves the BSR fully unexecuted and the
		// interpreter reproduces Push32's stack exceptions.
		m68kA64MovImm32(cb, m68kA64Tmp5, dec.q)
		e.emitPushRet(fall)
		e.dynExit = true

	case m68kA64ClassJSR:
		// ExecJsr computes the EA before pushing (visible when the EA base
		// is A7), so the target lands in Tmp5 first. emitGuard and the
		// store scratch leave Tmp5 intact.
		m68kA64EmitEAAddr(cb, &dec.src, 4, m68kA64Tmp5, 0)
		e.emitPushRet(fall)
		e.dynExit = true

	case m68kA64ClassJMP:
		m68kA64EmitEAAddr(cb, &dec.src, 4, m68kA64Tmp5, 0)
		e.dynExit = true

	case m68kA64ClassRTS:
		// Pop the return address: guarded read at A7, then A7 += 4. A bail
		// leaves RTS unexecuted; the interpreter reproduces Pop32's checks.
		// Pop32 bus-errors when A7 >= stackUpperBound even for valid RAM,
		// so the ceiling (read through the context; loaders retune it at
		// runtime) is enforced before the access guard.
		m68kA64LoadA(cb, m68kA64Tmp0, 7)
		cb.Emit32(arm64LDR_imm(m68kA64Tmp1, m68kA64Ctx, m68kCtxOffStackUpperBoundPtr/8))
		cb.Emit32(arm64LDR_W_imm(m68kA64Tmp1, m68kA64Tmp1, 0))
		cb.Emit32(arm64CMP_W(m68kA64Tmp0, m68kA64Tmp1))
		e.branchToBail(m68kA64CondCS) // A7 >= stackUpperBound
		e.emitGuard(m68kA64Tmp0, 4)
		m68kA64EmitLoadBE(cb, m68kA64Tmp5, m68kA64Tmp0, 4)
		cb.Emit32(arm64ADD_W_imm(m68kA64Tmp0, m68kA64Tmp0, 4))
		m68kA64StoreA(cb, m68kA64Tmp0, 7)
		e.dynExit = true
	}
}

// emitPushRet pushes the 32-bit return address retAddr onto the guest stack:
// A7 -= 4 with the write guarded BEFORE A7 commits, so an I/O or bounds bail
// leaves the calling instruction fully unexecuted. Tmp5 is preserved (it
// carries the jump target); Tmp0-Tmp3 and Tmp6 are clobbered.
func (e *m68kA64Emitter) emitPushRet(retAddr uint32) {
	cb := e.cb
	m68kA64LoadA(cb, m68kA64Tmp0, 7)
	cb.Emit32(arm64SUB_W_imm(m68kA64Tmp0, m68kA64Tmp0, 4))
	// Push32 bus-errors when the decremented SP falls below stackLowerBound
	// even for valid RAM; the floor is read through the context because
	// loaders retune the bounds at runtime.
	cb.Emit32(arm64LDR_imm(m68kA64Tmp1, m68kA64Ctx, m68kCtxOffStackLowerBoundPtr/8))
	cb.Emit32(arm64LDR_W_imm(m68kA64Tmp1, m68kA64Tmp1, 0))
	cb.Emit32(arm64CMP_W(m68kA64Tmp0, m68kA64Tmp1))
	e.branchToBail(m68kA64CondCC) // newSP < stackLowerBound
	e.emitGuard(m68kA64Tmp0, 4)
	m68kA64MovImm32(cb, m68kA64Tmp1, retAddr)
	m68kA64EmitStoreBE(cb, m68kA64Tmp1, m68kA64Tmp0, m68kA64Tmp2, 4)
	m68kA64StoreA(cb, m68kA64Tmp0, 7)
}

func m68kCompileBlockARM64(instrs []M68KJITInstr, startPC uint32, execMem *ExecMem, memory []byte, topOfRAM uint32) (*JITBlock, error) {
	if len(instrs) == 0 {
		return nil, fmt.Errorf("m68k arm64 emitter: empty block at %08X", startPC)
	}
	cb := NewCodeBuffer(len(instrs)*160 + 128)
	e := &m68kA64Emitter{cb: cb}

	// Prologue: base pointers, memory window, I/O bitmap, live CCR.
	cb.Emit32(arm64LDR_imm(m68kA64DataBase, m68kA64Ctx, m68kCtxOffDataRegsPtr/8))
	cb.Emit32(arm64LDR_imm(m68kA64AddrBase, m68kA64Ctx, m68kCtxOffAddrRegsPtr/8))
	cb.Emit32(arm64LDR_imm(m68kA64SRAddr, m68kA64Ctx, m68kCtxOffSRPtr/8))
	cb.Emit32(arm64LDR_imm(m68kA64MemBase, m68kA64Ctx, m68kCtxOffMemPtr/8))
	cb.Emit32(arm64LDR_W_imm(m68kA64MemSize, m68kA64Ctx, m68kCtxOffMemSize/4))
	cb.Emit32(arm64LDR_imm(m68kA64IOBmp, m68kA64Ctx, m68kCtxOffIOPageBitmapPtr/8))
	cb.Emit32(arm64LDR_W_imm(m68kA64IOBmpLen, m68kA64Ctx, m68kCtxOffIOPageBitmapLen/4))
	cb.Emit32(arm64LDRH_imm(m68kA64Tmp0, m68kA64SRAddr, 0))
	cb.Emit32(arm64UBFX_W(m68kA64CCR, m68kA64Tmp0, 0, 5))

	endPC := startPC
	for i := range instrs {
		ji := &instrs[i]
		instrPC := startPC + ji.pcOffset
		e.bails = nil
		if i == len(instrs)-1 {
			// A block-ending branch is only ever admitted as the final
			// instruction (m68kARM64SupportedPrefix stops on it).
			if bdec, ok := m68kA64DecodeBranch(ji, memory, instrPC, topOfRAM); ok {
				e.emitBranch(&bdec, ji, instrPC)
				// BSR/JSR/RTS carry stack-access guards; their bail exits
				// leave the terminator unexecuted.
				if len(e.bails) > 0 {
					e.stubs = append(e.stubs, m68kA64BailStub{sites: e.bails, pc: instrPC, retired: uint32(i)})
				}
				endPC = instrPC + uint32(ji.length)
				break
			}
		}
		dec, ok := m68kA64Decode(ji, memory, instrPC)
		if !ok {
			return nil, fmt.Errorf("m68k arm64 emitter: unsupported opcode %04X at %08X", ji.opcode, instrPC)
		}
		if err := e.emitInstr(&dec, ji, instrPC); err != nil {
			return nil, err
		}
		if len(e.bails) > 0 {
			e.stubs = append(e.stubs, m68kA64BailStub{sites: e.bails, pc: instrPC, retired: uint32(i)})
		}
		endPC = instrPC + uint32(ji.length)
	}

	// Success epilogue: merge CCR into SR, publish RetPC and RetCount. The
	// resume PC is the block fallthrough, a static branch target (BRA) or
	// the run-time value a Bcc/DBcc left in Tmp5 (flushSR only touches Tmp0).
	flushSR := func() {
		cb.Emit32(arm64LDRH_imm(m68kA64Tmp0, m68kA64SRAddr, 0))
		cb.Emit32(arm64LSR_W_imm(m68kA64Tmp0, m68kA64Tmp0, 5))
		cb.Emit32(arm64ORR_W_lsl(m68kA64Tmp0, m68kA64CCR, m68kA64Tmp0, 5))
		cb.Emit32(arm64STRH_imm(m68kA64Tmp0, m68kA64SRAddr, 0))
	}
	flushSR()
	if e.dynExit {
		cb.Emit32(arm64STR_W_imm(m68kA64Tmp5, m68kA64Ctx, m68kCtxOffRetPC/4))
	} else {
		exitPC := endPC
		if e.hasStaticExit {
			exitPC = e.staticExit
		}
		m68kA64MovImm32(cb, m68kA64Tmp0, exitPC)
		cb.Emit32(arm64STR_W_imm(m68kA64Tmp0, m68kA64Ctx, m68kCtxOffRetPC/4))
	}
	m68kA64MovImm32(cb, m68kA64Tmp0, uint32(len(instrs)))
	cb.Emit32(arm64STR_W_imm(m68kA64Tmp0, m68kA64Ctx, m68kCtxOffRetCount/4))
	cb.Emit32(arm64MOVZ(0, 0, 0)) // mov x0, #0 — clean return value
	cb.Emit32(arm64RET())

	// Per-instruction bail stubs: RetPC = faulting instruction, RetCount =
	// fully retired predecessors, then the shared I/O-fallback tail.
	if len(e.stubs) > 0 {
		var tailSites []m68kA64BranchSite
		for _, stub := range e.stubs {
			target := cb.Len()
			for _, s := range stub.sites {
				e.patchSite(s, target)
			}
			m68kA64MovImm32(cb, m68kA64Tmp5, stub.pc)
			m68kA64MovImm32(cb, m68kA64Tmp6, stub.retired)
			tailSites = append(tailSites, m68kA64BranchSite{off: cb.Len(), base: 0x14000000, imm26: true})
			cb.Emit32(arm64B(0))
		}
		tail := cb.Len()
		for _, s := range tailSites {
			e.patchSite(s, tail)
		}
		flushSR()
		cb.Emit32(arm64STR_W_imm(m68kA64Tmp5, m68kA64Ctx, m68kCtxOffRetPC/4))
		cb.Emit32(arm64STR_W_imm(m68kA64Tmp6, m68kA64Ctx, m68kCtxOffRetCount/4))
		cb.Emit32(arm64MOVZ_W(m68kA64Tmp0, 1, 0))
		cb.Emit32(arm64STR_W_imm(m68kA64Tmp0, m68kA64Ctx, m68kCtxOffNeedIOFallback/4))
		cb.Emit32(arm64MOVZ(0, 0, 0))
		cb.Emit32(arm64RET())
	}

	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, err
	}
	return &JITBlock{
		startPC:    uint64(startPC),
		endPC:      uint64(endPC),
		instrCount: len(instrs),
		execAddr:   addr,
		execSize:   len(code),
	}, nil
}

// m68kJITContextPtr returns the context pointer for callNative.
func m68kJITContextPtr(ctx *M68KJITContext) uintptr {
	return uintptr(unsafe.Pointer(ctx))
}
