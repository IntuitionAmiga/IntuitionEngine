// jit_m68k_emit_arm64.go - ARM64 native code emitter for the M68020 JIT
// (parity plan milestone 3, slice 1).
//
// Correctness-first minimal backend: straight-line prefixes of data-register
// integer instructions, full CCR materialisation on every flag-setting
// instruction, no liveness elision, no chaining, no register pinning.
//
// Register discipline for this slice: the emitted block is a leaf function
// that touches only caller-saved registers, so it needs no stack frame and
// never spills callee-saved state.
//
//	X0  M68KJITContext pointer (C ABI argument, live for the whole block)
//	X1  &cpu.DataRegs[0]
//	X3  &cpu.SR
//	W4  live CCR (X N Z V C in M68K bit order, bits 4..0)
//	W9-W15  scratch
//
// The pinned callee-saved mapping in jit_m68k_abi_arm64.go is the milestone 4
// residency plan; this slice deliberately does not use it. X18 is never
// touched (AAPCS64 platform register).

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"unsafe"
)

// ARM64 condition codes used for flag extraction.
const (
	m68kA64CondEQ = 0x0
	m68kA64CondCS = 0x2
	m68kA64CondCC = 0x3
	m68kA64CondMI = 0x4
	m68kA64CondVS = 0x6
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

// Fixed emitter register assignments (see file header).
const (
	m68kA64Ctx      = 0
	m68kA64DataBase = 1
	m68kA64SRAddr   = 3
	m68kA64CCR      = 4
	m68kA64Tmp0     = 9
	m68kA64Tmp1     = 10
	m68kA64Tmp2     = 11
	m68kA64Tmp3     = 12
	m68kA64Tmp4     = 13
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

// m68kA64FlagsLogicPreserveVC updates only N and Z from the value in rv,
// preserving X, V and C. This matches the interpreter's SetFlagsNZ, which the
// amd64 backend also replicates (emitCCR_LogicPreserveVC): IE's AND/OR/EOR
// deliberately keep V and C, unlike the M68000PRM. Parity is with the
// interpreter, not the manual.
func m68kA64FlagsLogicPreserveVC(cb *CodeBuffer, rv byte) {
	cb.Emit32(arm64CMP_W_imm(rv, 0))
	// tmp4 = CCR & ~(N|Z) — keep X, V, C.
	cb.Emit32(arm64CSET_W(m68kA64Tmp2, m68kA64CondEQ)) // Z
	cb.Emit32(arm64CSET_W(m68kA64Tmp3, m68kA64CondMI)) // N
	m68kA64MovImm32(cb, m68kA64Tmp0, 0x13)             // X|V|C mask
	cb.Emit32(arm64AND_W(m68kA64CCR, m68kA64CCR, m68kA64Tmp0))
	cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, m68kA64Tmp2, m68kA64BitZ))
	cb.Emit32(arm64ORR_W_lsl(m68kA64CCR, m68kA64CCR, m68kA64Tmp3, m68kA64BitN))
}

// m68kA64FlagsMove materialises the logic-result CCR for the value in rv.
func m68kA64FlagsMove(cb *CodeBuffer, rv byte) {
	cb.Emit32(arm64CMP_W_imm(rv, 0))
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

// m68kARM64InstrSupported reports whether the slice-1 arm64 emitter can lower
// the instruction natively. Must stay in exact lockstep with
// m68kA64EmitInstr — every admitted opcode shape must be emitted, and only
// those shapes.
func m68kARM64InstrSupported(ji *M68KJITInstr) bool {
	op := ji.opcode
	switch {
	case op == 0x4E71: // NOP
		return true
	case op&0xF100 == 0x7000: // MOVEQ
		return true
	case op&0xF1FF == 0x203C: // MOVE.L #imm,Dn
		return ji.length == 6
	case op&0xF1F8 == 0x2000: // MOVE.L Dn,Dm
		return true
	case op&0xFFF8 == 0x4A80: // TST.L Dn
		return true
	case op&0xFFF8 == 0x4280: // CLR.L Dn
		return true
	case op&0xF1F8 == 0xD080: // ADD.L Dn,Dm
		return true
	case op&0xF1F8 == 0x9080: // SUB.L Dn,Dm
		return true
	case op&0xF1F8 == 0xB080: // CMP.L Dn,Dm
		return true
	case op&0xF1F8 == 0xC080: // AND.L Dn,Dm
		return true
	case op&0xF1F8 == 0x8080: // OR.L Dn,Dm
		return true
	case op&0xF1F8 == 0xB180: // EOR.L Dn,Dm
		return true
	case op&0xF1F8 == 0x5080: // ADDQ.L #q,Dn
		return true
	case op&0xF1F8 == 0x5180: // SUBQ.L #q,Dn
		return true
	}
	return false
}

// m68kARM64SupportedPrefix returns the number of leading instructions the
// arm64 backend can execute natively. The prefix never includes a block
// terminator or an unsupported instruction.
func m68kARM64SupportedPrefix(instrs []M68KJITInstr, memory []byte, startPC uint32) int {
	n := 0
	for i := range instrs {
		ji := &instrs[i]
		if m68kIsBlockTerminator(ji.opcode) || !m68kARM64InstrSupported(ji) {
			break
		}
		n++
	}
	_ = memory
	_ = startPC
	return n
}

// m68kA64EmitInstr lowers one supported instruction.
func m68kA64EmitInstr(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, instrPC uint32) error {
	op := ji.opcode
	switch {
	case op == 0x4E71: // NOP
		return nil

	case op&0xF100 == 0x7000: // MOVEQ #imm8,Dn
		dn := (op >> 9) & 7
		val := uint32(int32(int8(op)))
		m68kA64MovImm32(cb, m68kA64Tmp0, val)
		m68kA64StoreD(cb, m68kA64Tmp0, dn)
		m68kA64FlagsStatic(cb, int8(op) < 0, int8(op) == 0)
		return nil

	case op&0xF1FF == 0x203C: // MOVE.L #imm,Dn
		dn := (op >> 9) & 7
		if int(instrPC)+6 > len(memory) {
			return fmt.Errorf("MOVE.L #imm at %08X: immediate out of range", instrPC)
		}
		imm := uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
			uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5])
		m68kA64MovImm32(cb, m68kA64Tmp0, imm)
		m68kA64StoreD(cb, m68kA64Tmp0, dn)
		m68kA64FlagsStatic(cb, int32(imm) < 0, imm == 0)
		return nil

	case op&0xF1F8 == 0x2000: // MOVE.L Dn,Dm
		src := op & 7
		dst := (op >> 9) & 7
		m68kA64LoadD(cb, m68kA64Tmp0, src)
		m68kA64StoreD(cb, m68kA64Tmp0, dst)
		m68kA64FlagsMove(cb, m68kA64Tmp0)
		return nil

	case op&0xFFF8 == 0x4A80: // TST.L Dn
		m68kA64LoadD(cb, m68kA64Tmp0, op&7)
		m68kA64FlagsMove(cb, m68kA64Tmp0)
		return nil

	case op&0xFFF8 == 0x4280: // CLR.L Dn
		m68kA64StoreD(cb, 31, op&7) // str wzr
		m68kA64FlagsStatic(cb, false, true)
		return nil

	case op&0xF1F8 == 0xD080: // ADD.L Dsrc,Ddst
		src := op & 7
		dst := (op >> 9) & 7
		m68kA64LoadD(cb, m68kA64Tmp0, src)
		m68kA64LoadD(cb, m68kA64Tmp1, dst)
		cb.Emit32(arm64ADDS_W(m68kA64Tmp1, m68kA64Tmp1, m68kA64Tmp0))
		m68kA64StoreD(cb, m68kA64Tmp1, dst)
		m68kA64FlagsArith(cb, m68kA64CondCS, false)
		return nil

	case op&0xF1F8 == 0x9080: // SUB.L Dsrc,Ddst
		src := op & 7
		dst := (op >> 9) & 7
		m68kA64LoadD(cb, m68kA64Tmp0, src)
		m68kA64LoadD(cb, m68kA64Tmp1, dst)
		cb.Emit32(arm64SUBS_W(m68kA64Tmp1, m68kA64Tmp1, m68kA64Tmp0))
		m68kA64StoreD(cb, m68kA64Tmp1, dst)
		m68kA64FlagsArith(cb, m68kA64CondCC, false)
		return nil

	case op&0xF1F8 == 0xB080: // CMP.L Dsrc,Ddst
		src := op & 7
		dst := (op >> 9) & 7
		m68kA64LoadD(cb, m68kA64Tmp0, src)
		m68kA64LoadD(cb, m68kA64Tmp1, dst)
		cb.Emit32(arm64SUBS_W(m68kA64Tmp1, m68kA64Tmp1, m68kA64Tmp0))
		m68kA64FlagsArith(cb, m68kA64CondCC, true)
		return nil

	case op&0xF1F8 == 0xC080, op&0xF1F8 == 0x8080: // AND.L / OR.L Dsrc,Ddst
		src := op & 7
		dst := (op >> 9) & 7
		m68kA64LoadD(cb, m68kA64Tmp0, src)
		m68kA64LoadD(cb, m68kA64Tmp1, dst)
		if op&0xF000 == 0xC000 {
			cb.Emit32(arm64AND_W(m68kA64Tmp1, m68kA64Tmp1, m68kA64Tmp0))
		} else {
			cb.Emit32(arm64ORR_W(m68kA64Tmp1, m68kA64Tmp1, m68kA64Tmp0))
		}
		m68kA64StoreD(cb, m68kA64Tmp1, dst)
		m68kA64FlagsLogicPreserveVC(cb, m68kA64Tmp1)
		return nil

	case op&0xF1F8 == 0xB180: // EOR.L Dn,Dea — dest is the EA register
		src := (op >> 9) & 7
		dst := op & 7
		m68kA64LoadD(cb, m68kA64Tmp0, src)
		m68kA64LoadD(cb, m68kA64Tmp1, dst)
		cb.Emit32(arm64EOR_W(m68kA64Tmp1, m68kA64Tmp1, m68kA64Tmp0))
		m68kA64StoreD(cb, m68kA64Tmp1, dst)
		m68kA64FlagsLogicPreserveVC(cb, m68kA64Tmp1)
		return nil

	case op&0xF1F8 == 0x5080: // ADDQ.L #q,Dn
		q := uint32((op >> 9) & 7)
		if q == 0 {
			q = 8
		}
		dn := op & 7
		m68kA64LoadD(cb, m68kA64Tmp1, dn)
		cb.Emit32(arm64ADDS_W_imm(m68kA64Tmp1, m68kA64Tmp1, q))
		m68kA64StoreD(cb, m68kA64Tmp1, dn)
		m68kA64FlagsArith(cb, m68kA64CondCS, false)
		return nil

	case op&0xF1F8 == 0x5180: // SUBQ.L #q,Dn
		q := uint32((op >> 9) & 7)
		if q == 0 {
			q = 8
		}
		dn := op & 7
		m68kA64LoadD(cb, m68kA64Tmp1, dn)
		cb.Emit32(arm64SUBS_W_imm(m68kA64Tmp1, m68kA64Tmp1, q))
		m68kA64StoreD(cb, m68kA64Tmp1, dn)
		m68kA64FlagsArith(cb, m68kA64CondCC, false)
		return nil
	}
	return fmt.Errorf("m68k arm64 emitter: unsupported opcode %04X at %08X", op, instrPC)
}

// m68kCompileBlockARM64 compiles a fully supported straight-line prefix into
// native arm64 code. The emitted block loads live state on entry, executes
// the instructions, publishes CCR/RetPC/RetCount on exit and returns.
func m68kCompileBlockARM64(instrs []M68KJITInstr, startPC uint32, execMem *ExecMem, memory []byte) (*JITBlock, error) {
	if len(instrs) == 0 {
		return nil, fmt.Errorf("m68k arm64 emitter: empty block at %08X", startPC)
	}
	cb := NewCodeBuffer(len(instrs)*64 + 64)

	// Prologue: base pointers and live CCR.
	cb.Emit32(arm64LDR_imm(m68kA64DataBase, m68kA64Ctx, m68kCtxOffDataRegsPtr/8))
	cb.Emit32(arm64LDR_imm(m68kA64SRAddr, m68kA64Ctx, m68kCtxOffSRPtr/8))
	cb.Emit32(arm64LDRH_imm(m68kA64Tmp0, m68kA64SRAddr, 0))
	cb.Emit32(arm64UBFX_W(m68kA64CCR, m68kA64Tmp0, 0, 5))

	endPC := startPC
	for i := range instrs {
		ji := &instrs[i]
		instrPC := startPC + ji.pcOffset
		if err := m68kA64EmitInstr(cb, ji, memory, instrPC); err != nil {
			return nil, err
		}
		endPC = instrPC + uint32(ji.length)
	}

	// Epilogue: merge CCR into SR, publish RetPC and RetCount, return.
	cb.Emit32(arm64LDRH_imm(m68kA64Tmp0, m68kA64SRAddr, 0))
	cb.Emit32(arm64LSR_W_imm(m68kA64Tmp0, m68kA64Tmp0, 5))
	cb.Emit32(arm64ORR_W_lsl(m68kA64Tmp0, m68kA64CCR, m68kA64Tmp0, 5))
	cb.Emit32(arm64STRH_imm(m68kA64Tmp0, m68kA64SRAddr, 0))
	m68kA64MovImm32(cb, m68kA64Tmp0, endPC)
	cb.Emit32(arm64STR_W_imm(m68kA64Tmp0, m68kA64Ctx, m68kCtxOffRetPC/4))
	m68kA64MovImm32(cb, m68kA64Tmp0, uint32(len(instrs)))
	cb.Emit32(arm64STR_W_imm(m68kA64Tmp0, m68kA64Ctx, m68kCtxOffRetCount/4))
	cb.Emit32(arm64MOVZ(0, 0, 0)) // mov x0, #0 — clean return value
	cb.Emit32(arm64RET())

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
