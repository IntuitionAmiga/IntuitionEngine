// jit_emit_arm64.go - ARM64 native code emitter for IE64 JIT compiler

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"errors"
	"math"
)

// ===========================================================================
// ARM64 Register Mapping
// ===========================================================================
//
// ARM64    IE64 Purpose
// ------   ------------
// X0       *JITContext on entry; scratch after prologue
// X1-X7    Scratch
// X8       &cpu.regs[0] — register file base
// X9       &cpu.memory[0] — memory base
// X10      IO_REGION_START constant
// X11      Scratch for address computation
// X12-X17  IE64 R1-R6
// X18      NEVER USED — AAPCS64 platform register (see arm64PlatformReg)
// X19-X26  IE64 R7-R14 (callee-saved; prologue saves the pairs in use)
// X27      IE64 R31 (SP) — always resident
// X28      Current IE64 PC
// XZR      IE64 R0 (reads=0, writes=discard)
// X29/X30  Go FP/LR — saved/restored
//
// IE64 R15-R30 are spilled to the register file.

const (
	arm64RegCtx       = 0  // X0: JITContext pointer on entry
	arm64RegIOBitmap  = 5  // X5: &ioPageBitmap[0] (dedicated, avoids stack loads)
	arm64RegFPUBase   = 6  // X6: &cpu.FPU (FPU register file base, loaded when hasFPU)
	arm64RegLoopCount = 7  // X7: backward branch iteration counter (reserved when hasBackwardBranch)
	arm64RegBase      = 8  // X8: register file base
	arm64RegMemBase   = 9  // X9: memory base
	arm64RegIOStart   = 10 // X10: IO_REGION_START
	arm64RegScratch   = 11 // X11: scratch
	arm64RegIE64SP    = 27 // X27: IE64 R31 (SP)
	arm64RegIE64PC    = 28 // X28: IE64 PC
	arm64RegFP        = 29 // X29: Go frame pointer
	arm64RegLR        = 30 // X30: Go link register

	// arm64PlatformReg is the AAPCS64 platform register. It is reserved by the
	// ABI and must never be allocated: Darwin reserves it for the kernel and
	// Windows/ARM64 holds the thread environment block pointer in it. The Go
	// toolchain declines to allocate it on every platform, which is the only
	// reason Linux tolerated it being clobbered here historically. The mapping
	// skips it, which costs one resident slot.
	arm64PlatformReg = 18

	// IE64 R1-R14 mapped to ARM64 X12-X17 and X19-X26, skipping X18.
	arm64FirstMapped = 12
	arm64LastMapped  = 26
	ie64FirstMapped  = 1
	ie64LastMapped   = 14
)

// arm64CalleeSavedPairs enumerates the callee-saved host register pairs the
// prologue may save and the epilogue restore, together with the IE64 registers
// resident in each. The prologue saves a pair only when the block uses one of
// its IE64 registers, so this table must agree with ie64ToARM64Reg exactly;
// TestARM64RegMap_CalleeSavedPairsMatchMapping pins the two together. They were
// previously kept in sync by hand, which is how IE64 R7 came to live in X18
// unnoticed.
var arm64CalleeSavedPairs = [...]struct {
	loHost, hiHost byte // host pair saved by a single STP/LDP
	slot           int  // STP/LDP scaled offset from SP
	loIE64, hiIE64 byte // IE64 registers resident in loHost/hiHost
}{
	{19, 20, 0, 7, 8},
	{21, 22, 2, 9, 10},
	{23, 24, 4, 11, 12},
	{25, 26, 6, 13, 14},
}

// arm64CalleeSavedMask is the set of IE64 registers living in callee-saved host
// registers, as a bitmask over IE64 register numbers.
func arm64CalleeSavedMask(pairIdx int) uint32 {
	p := arm64CalleeSavedPairs[pairIdx]
	return (1 << p.loIE64) | (1 << p.hiIE64)
}

// ie64ToARM64Reg maps an IE64 register index (0-31) to an ARM64 register.
// Returns the ARM64 register number and whether it's a "mapped" register
// (resident in an ARM64 register) vs a "spilled" register (in memory).
func ie64ToARM64Reg(ie64Reg byte) (arm64Reg byte, mapped bool) {
	if ie64Reg == 0 {
		return 31, true // XZR
	}
	if ie64Reg >= ie64FirstMapped && ie64Reg <= ie64LastMapped {
		host := arm64FirstMapped + (ie64Reg - ie64FirstMapped)
		// Step over the platform register rather than allocating it: IE64 R7
		// onwards shift up by one, so R1-R6 occupy X12-X17 and R7-R14 occupy
		// X19-X26.
		if host >= arm64PlatformReg {
			host++
		}
		return host, true
	}
	if ie64Reg == 31 {
		return arm64RegIE64SP, true
	}
	return 0, false // spilled: R15-R30
}

// ===========================================================================
// ARM64 Instruction Encodings
// ===========================================================================

// ARM64 condition codes
const (
	arm64CondEQ = 0x0
	arm64CondNE = 0x1
	arm64CondHS = 0x2 // unsigned >=
	arm64CondLO = 0x3 // unsigned <
	arm64CondGE = 0xA // signed >=
	arm64CondLT = 0xB // signed <
	arm64CondGT = 0xC // signed >
	arm64CondLE = 0xD // signed <=
	arm64CondMI = 0x4 // negative (N set) — used for FCMP less-than
	arm64CondVS = 0x6 // overflow (V set) — used for FCMP unordered/NaN
	arm64CondHI = 0x8 // unsigned >
	arm64CondLS = 0x9 // unsigned <=
)

// stp Xt1, Xt2, [Xn, #imm7*8]! (pre-index)
func arm64STP_pre(rt1, rt2, rn byte, imm7 int) uint32 {
	return 0xA9800000 | uint32(imm7&0x7F)<<15 | uint32(rt2)<<10 | uint32(rn)<<5 | uint32(rt1)
}

// ldp Xt1, Xt2, [Xn], #imm7*8 (post-index)
func arm64LDP_post(rt1, rt2, rn byte, imm7 int) uint32 {
	return 0xA8C00000 | uint32(imm7&0x7F)<<15 | uint32(rt2)<<10 | uint32(rn)<<5 | uint32(rt1)
}

// stp Xt1, Xt2, [Xn, #imm7*8] (signed offset)
func arm64STP_offset(rt1, rt2, rn byte, imm7 int) uint32 {
	return 0xA9000000 | uint32(imm7&0x7F)<<15 | uint32(rt2)<<10 | uint32(rn)<<5 | uint32(rt1)
}

// ldp Xt1, Xt2, [Xn, #imm7*8] (signed offset)
func arm64LDP_offset(rt1, rt2, rn byte, imm7 int) uint32 {
	return 0xA9400000 | uint32(imm7&0x7F)<<15 | uint32(rt2)<<10 | uint32(rn)<<5 | uint32(rt1)
}

// ldr Xt, [Xn, #imm12*8] (unsigned offset, 64-bit)
func arm64LDR_imm(rt, rn byte, imm12 uint32) uint32 {
	return 0xF9400000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rt)
}

// str Xt, [Xn, #imm12*8] (unsigned offset, 64-bit)
func arm64STR_imm(rt, rn byte, imm12 uint32) uint32 {
	return 0xF9000000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rt)
}

// ldr Wt, [Xn, #imm12*4] (unsigned offset, 32-bit)
func arm64LDR_W_imm(rt, rn byte, imm12 uint32) uint32 {
	return 0xB9400000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rt)
}

// str Wt, [Xn, #imm12*4] (unsigned offset, 32-bit)
func arm64STR_W_imm(rt, rn byte, imm12 uint32) uint32 {
	return 0xB9000000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rt)
}

// ldrh Wt, [Xn, #imm12*2] (unsigned offset, 16-bit)
func arm64LDRH_imm(rt, rn byte, imm12 uint32) uint32 {
	return 0x79400000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rt)
}

// strh Wt, [Xn, #imm12*2] (unsigned offset, 16-bit)
func arm64STRH_imm(rt, rn byte, imm12 uint32) uint32 {
	return 0x79000000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rt)
}

// ldrb Wt, [Xn, #imm12] (unsigned offset, 8-bit)
func arm64LDRB_imm(rt, rn byte, imm12 uint32) uint32 {
	return 0x39400000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rt)
}

// strb Wt, [Xn, #imm12] (unsigned offset, 8-bit)
func arm64STRB_imm(rt, rn byte, imm12 uint32) uint32 {
	return 0x39000000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rt)
}

// ldr Xt, [Xn, Xm] (register offset, 64-bit, option=011/LSL)
func arm64LDR_reg(rt, rn, rm byte) uint32 {
	return 0xF8606800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rt)
}

// str Xt, [Xn, Xm] (register offset, 64-bit, option=011/LSL)
func arm64STR_reg(rt, rn, rm byte) uint32 {
	return 0xF8206800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rt)
}

// ldr Wt, [Xn, Xm] (register offset, 32-bit, option=011/LSL)
func arm64LDR_W_reg(rt, rn, rm byte) uint32 {
	return 0xB8606800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rt)
}

// str Wt, [Xn, Xm] (register offset, 32-bit, option=011/LSL)
func arm64STR_W_reg(rt, rn, rm byte) uint32 {
	return 0xB8206800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rt)
}

// ldrh Wt, [Xn, Xm] (register offset, 16-bit, option=011/LSL)
func arm64LDRH_reg(rt, rn, rm byte) uint32 {
	return 0x78606800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rt)
}

// strh Wt, [Xn, Xm] (register offset, 16-bit, option=011/LSL)
func arm64STRH_reg(rt, rn, rm byte) uint32 {
	return 0x78206800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rt)
}

// ldrb Wt, [Xn, Xm] (register offset, 8-bit, option=011/LSL)
func arm64LDRB_reg(rt, rn, rm byte) uint32 {
	return 0x38606800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rt)
}

// strb Wt, [Xn, Xm] (register offset, 8-bit, option=011/LSL)
func arm64STRB_reg(rt, rn, rm byte) uint32 {
	return 0x38206800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rt)
}

// movz Xd, #imm16, LSL #shift (shift = 0, 16, 32, 48)
func arm64MOVZ(rd byte, imm16 uint16, shift int) uint32 {
	hw := uint32(shift / 16)
	return 0xD2800000 | hw<<21 | uint32(imm16)<<5 | uint32(rd)
}

// movk Xd, #imm16, LSL #shift
func arm64MOVK(rd byte, imm16 uint16, shift int) uint32 {
	hw := uint32(shift / 16)
	return 0xF2800000 | hw<<21 | uint32(imm16)<<5 | uint32(rd)
}

// movz Wd, #imm16 (32-bit variant, shift=0)
func arm64MOVZ_W(rd byte, imm16 uint16, shift int) uint32 {
	hw := uint32(shift / 16)
	return 0x52800000 | hw<<21 | uint32(imm16)<<5 | uint32(rd)
}

// movk Wd, #imm16
func arm64MOVK_W(rd byte, imm16 uint16, shift int) uint32 {
	hw := uint32(shift / 16)
	return 0x72800000 | hw<<21 | uint32(imm16)<<5 | uint32(rd)
}

// mov Xd, Xm (alias for ORR Xd, XZR, Xm)
func arm64MOV(rd, rm byte) uint32 {
	return 0xAA0003E0 | uint32(rm)<<16 | uint32(rd)
}

// mov Wd, Wm (alias for ORR Wd, WZR, Wm)
func arm64MOV_W(rd, rm byte) uint32 {
	return 0x2A0003E0 | uint32(rm)<<16 | uint32(rd)
}

// adr Xd, label. The signed 21-bit immediate is a byte offset from
// the ADR instruction, so the final mmap base address does not matter.
func arm64ADR(rd byte, offset int32) uint32 {
	imm := uint32(offset) & 0x1FFFFF
	return 0x10000000 | (imm&0x3)<<29 | ((imm>>2)&0x7FFFF)<<5 | uint32(rd)
}

// add Xd, Xn, Xm
func arm64ADD(rd, rn, rm byte) uint32 {
	return 0x8B000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// add Wd, Wn, Wm
func arm64ADD_W(rd, rn, rm byte) uint32 {
	return 0x0B000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// add Xd, Xn, #imm12
func arm64ADD_imm(rd, rn byte, imm12 uint32) uint32 {
	return 0x91000000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rd)
}

// sub Xd, Xn, Xm
func arm64SUB(rd, rn, rm byte) uint32 {
	return 0xCB000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// sub Wd, Wn, Wm
func arm64SUB_W(rd, rn, rm byte) uint32 {
	return 0x4B000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// sub Xd, Xn, #imm12
func arm64SUB_imm(rd, rn byte, imm12 uint32) uint32 {
	return 0xD1000000 | (imm12&0xFFF)<<10 | uint32(rn)<<5 | uint32(rd)
}

// and Xd, Xn, Xm
func arm64AND(rd, rn, rm byte) uint32 {
	return 0x8A000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// and Wd, Wn, Wm
func arm64AND_W(rd, rn, rm byte) uint32 {
	return 0x0A000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// and Xd, Xn, #bitmask (logical immediate)
func arm64AND_imm(rd, rn byte, immr, imms byte, n byte) uint32 {
	return 0x92000000 | uint32(n)<<22 | uint32(immr)<<16 | uint32(imms)<<10 | uint32(rn)<<5 | uint32(rd)
}

// orr Xd, Xn, Xm
func arm64ORR(rd, rn, rm byte) uint32 {
	return 0xAA000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// orr Wd, Wn, Wm
func arm64ORR_W(rd, rn, rm byte) uint32 {
	return 0x2A000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// eor Xd, Xn, Xm
func arm64EOR(rd, rn, rm byte) uint32 {
	return 0xCA000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// eor Wd, Wn, Wm
func arm64EOR_W(rd, rn, rm byte) uint32 {
	return 0x4A000000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// orn Xd, XZR, Xm (bitwise NOT = MVN)
func arm64MVN(rd, rm byte) uint32 {
	return 0xAA2003E0 | uint32(rm)<<16 | uint32(rd)
}

// orn Wd, WZR, Wm
func arm64MVN_W(rd, rm byte) uint32 {
	return 0x2A2003E0 | uint32(rm)<<16 | uint32(rd)
}

// neg Xd, Xm (alias for SUB Xd, XZR, Xm)
func arm64NEG(rd, rm byte) uint32 {
	return 0xCB0003E0 | uint32(rm)<<16 | uint32(rd)
}

// neg Wd, Wm
func arm64NEG_W(rd, rm byte) uint32 {
	return 0x4B0003E0 | uint32(rm)<<16 | uint32(rd)
}

// lsl Xd, Xn, Xm (alias for LSLV)
func arm64LSL(rd, rn, rm byte) uint32 {
	return 0x9AC02000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// lsl Wd, Wn, Wm
func arm64LSL_W(rd, rn, rm byte) uint32 {
	return 0x1AC02000 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// lsr Xd, Xn, Xm (alias for LSRV)
func arm64LSR(rd, rn, rm byte) uint32 {
	return 0x9AC02400 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// lsr Wd, Wn, Wm
func arm64LSR_W(rd, rn, rm byte) uint32 {
	return 0x1AC02400 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// asr Xd, Xn, Xm (alias for ASRV)
func arm64ASR(rd, rn, rm byte) uint32 {
	return 0x9AC02800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// asr Wd, Wn, Wm
func arm64ASR_W(rd, rn, rm byte) uint32 {
	return 0x1AC02800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// mul Xd, Xn, Xm (alias for MADD Xd, Xn, Xm, XZR)
func arm64MUL(rd, rn, rm byte) uint32 {
	return 0x9B007C00 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// mul Wd, Wn, Wm
func arm64MUL_W(rd, rn, rm byte) uint32 {
	return 0x1B007C00 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// umulh Xd, Xn, Xm (unsigned high 64 bits of 64x64 multiply)
func arm64UMULH(rd, rn, rm byte) uint32 {
	return 0x9BC07C00 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// smulh Xd, Xn, Xm (signed high 64 bits of 64x64 multiply)
func arm64SMULH(rd, rn, rm byte) uint32 {
	return 0x9B407C00 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// smull Xd, Wn, Wm (signed multiply long — W×W→X)
func arm64SMULL(rd, rn, rm byte) uint32 {
	return 0x9B207C00 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// msub Xd, Xn, Xm, Xa: Xd = Xa - Xn*Xm
func arm64MSUB(rd, rn, rm, ra byte) uint32 {
	return 0x9B008000 | uint32(rm)<<16 | uint32(ra)<<10 | uint32(rn)<<5 | uint32(rd)
}

// udiv Xd, Xn, Xm
func arm64UDIV(rd, rn, rm byte) uint32 {
	return 0x9AC00800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// udiv Wd, Wn, Wm
func arm64UDIV_W(rd, rn, rm byte) uint32 {
	return 0x1AC00800 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// sdiv Xd, Xn, Xm
func arm64SDIV(rd, rn, rm byte) uint32 {
	return 0x9AC00C00 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// sdiv Wd, Wn, Wm
func arm64SDIV_W(rd, rn, rm byte) uint32 {
	return 0x1AC00C00 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// clz Xd, Xn
func arm64CLZ(rd, rn byte) uint32 {
	return 0xDAC01000 | uint32(rn)<<5 | uint32(rd)
}

// clz Wd, Wn
func arm64CLZ_W(rd, rn byte) uint32 {
	return 0x5AC01000 | uint32(rn)<<5 | uint32(rd)
}

// rbit Wd, Wn
func arm64RBIT_W(rd, rn byte) uint32 {
	return 0x5AC00000 | uint32(rn)<<5 | uint32(rd)
}

// rev Wd, Wn (reverse byte order in low 32 bits)
func arm64REV_W(rd, rn byte) uint32 {
	return 0x5AC00800 | uint32(rn)<<5 | uint32(rd)
}

// ror Xd, Xn, Xm (alias for RORV)
func arm64ROR(rd, rn, rm byte) uint32 {
	return 0x9AC02C00 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// ror Wd, Wn, Wm
func arm64ROR_W(rd, rn, rm byte) uint32 {
	return 0x1AC02C00 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// cmp Xn, Xm (alias for SUBS XZR, Xn, Xm)
func arm64CMP(rn, rm byte) uint32 {
	return 0xEB00001F | uint32(rm)<<16 | uint32(rn)<<5
}

// cmp Xn, #imm12
func arm64CMP_imm(rn byte, imm12 uint32) uint32 {
	return 0xF100001F | (imm12&0xFFF)<<10 | uint32(rn)<<5
}

// cmp Wn, Wm
func arm64CMP_W(rn, rm byte) uint32 {
	return 0x6B00001F | uint32(rm)<<16 | uint32(rn)<<5
}

// b.cond offset (19-bit signed word offset)
func arm64Bcond(cond byte, offset int32) uint32 {
	imm19 := uint32(offset>>2) & 0x7FFFF
	return 0x54000000 | imm19<<5 | uint32(cond)
}

// b offset (26-bit signed word offset)
func arm64B(offset int32) uint32 {
	imm26 := uint32(offset>>2) & 0x3FFFFFF
	return 0x14000000 | imm26
}

// cbz Xt, offset (compare and branch if zero)
func arm64CBZ(rt byte, offset int32) uint32 {
	imm19 := uint32(offset>>2) & 0x7FFFF
	return 0xB4000000 | imm19<<5 | uint32(rt)
}

// cbnz Xt, offset (compare and branch if not zero)
func arm64CBNZ(rt byte, offset int32) uint32 {
	imm19 := uint32(offset>>2) & 0x7FFFF
	return 0xB5000000 | imm19<<5 | uint32(rt)
}

// ret (X30)
func arm64RET() uint32 {
	return 0xD65F03C0
}

func arm64CLREX() uint32 {
	return 0xD503305F
}

func arm64LDAXR(rt, rn byte) uint32 {
	return 0xC85FFC00 | uint32(rn)<<5 | uint32(rt)
}

func arm64STLXR(rs, rt, rn byte) uint32 {
	return 0xC800FC00 | uint32(rs)<<16 | uint32(rn)<<5 | uint32(rt)
}

// lsr Xd, Xn, #shift (immediate, alias for UBFM Xd, Xn, #shift, #63)
func arm64LSR_imm(rd, rn byte, shift uint32) uint32 {
	return 0xD340FC00 | (shift&0x3F)<<16 | uint32(rn)<<5 | uint32(rd)
}

// nop
func arm64NOP() uint32 {
	return 0xD503201F
}

// sxtw Xd, Wn (sign-extend W to X)
func arm64SXTW(rd, rn byte) uint32 {
	return 0x93407C00 | uint32(rn)<<5 | uint32(rd)
}

// sxth Xd, Wn (sign-extend halfword to X)
func arm64SXTH(rd, rn byte) uint32 {
	return 0x93403C00 | uint32(rn)<<5 | uint32(rd)
}

// sxtb Xd, Wn (sign-extend byte to X)
func arm64SXTB(rd, rn byte) uint32 {
	return 0x93401C00 | uint32(rn)<<5 | uint32(rd)
}

// uxth Wd, Wn (zero-extend halfword, 32-bit)
func arm64UXTH(rd, rn byte) uint32 {
	// AND Wd, Wn, #0xFFFF  (immr=0, imms=15, N=0)
	return 0x12003C00 | uint32(rn)<<5 | uint32(rd)
}

// uxtb Wd, Wn (zero-extend byte, 32-bit)
func arm64UXTB(rd, rn byte) uint32 {
	// AND Wd, Wn, #0xFF  (immr=0, imms=7, N=0)
	return 0x12001C00 | uint32(rn)<<5 | uint32(rd)
}

// ===========================================================================
// ARM64 Floating-Point Instruction Encodings
// ===========================================================================

// fmov Sd, Wn (general→FP transfer, 32-bit)
func arm64FMOV_WtoS(sd, wn byte) uint32 {
	return 0x1E270000 | uint32(wn)<<5 | uint32(sd)
}

// fmov Wd, Sn (FP→general transfer, 32-bit)
func arm64FMOV_StoW(wd, sn byte) uint32 {
	return 0x1E260000 | uint32(sn)<<5 | uint32(wd)
}

// fadd Sd, Sn, Sm (single-precision add)
func arm64FADD_S(sd, sn, sm byte) uint32 {
	return 0x1E202800 | uint32(sm)<<16 | uint32(sn)<<5 | uint32(sd)
}

// fsub Sd, Sn, Sm (single-precision subtract)
func arm64FSUB_S(sd, sn, sm byte) uint32 {
	return 0x1E203800 | uint32(sm)<<16 | uint32(sn)<<5 | uint32(sd)
}

// fmul Sd, Sn, Sm (single-precision multiply)
func arm64FMUL_S(sd, sn, sm byte) uint32 {
	return 0x1E200800 | uint32(sm)<<16 | uint32(sn)<<5 | uint32(sd)
}

// fdiv Sd, Sn, Sm (single-precision divide)
func arm64FDIV_S(sd, sn, sm byte) uint32 {
	return 0x1E201800 | uint32(sm)<<16 | uint32(sn)<<5 | uint32(sd)
}

// fsqrt Sd, Sn (single-precision square root)
func arm64FSQRT_S(sd, sn byte) uint32 {
	return 0x1E21C000 | uint32(sn)<<5 | uint32(sd)
}

// fcmp Sn, Sm (single-precision compare, sets NZCV)
func arm64FCMP_S(sn, sm byte) uint32 {
	return 0x1E202000 | uint32(sm)<<16 | uint32(sn)<<5
}

// scvtf Sd, Wn (signed int32→float32)
func arm64SCVTF_WS(sd, wn byte) uint32 {
	return 0x1E220000 | uint32(wn)<<5 | uint32(sd)
}

// fcvtzs Wd, Sn (float32→signed int32, round toward zero)
func arm64FCVTZS_SW(wd, sn byte) uint32 {
	return 0x1E380000 | uint32(sn)<<5 | uint32(wd)
}

// frintn Sd, Sn (round to nearest even, single-precision)
func arm64FRINTN_S(sd, sn byte) uint32 {
	return 0x1E244000 | uint32(sn)<<5 | uint32(sd)
}

// frintz Sd, Sn (round toward zero, single-precision)
func arm64FRINTZ_S(sd, sn byte) uint32 {
	return 0x1E25C000 | uint32(sn)<<5 | uint32(sd)
}

// frintm Sd, Sn (round toward -infinity/floor, single-precision)
func arm64FRINTM_S(sd, sn byte) uint32 {
	return 0x1E254000 | uint32(sn)<<5 | uint32(sd)
}

// frintp Sd, Sn (round toward +infinity/ceil, single-precision)
func arm64FRINTP_S(sd, sn byte) uint32 {
	return 0x1E24C000 | uint32(sn)<<5 | uint32(sd)
}

// The double-precision forms below are the single-precision encodings with the
// ftype field (bits 23:22) set to 01 rather than 00, and, for the transfers and
// conversions that name a general register, sf (bit 31) set to select an X
// register. They are written out as whole constants rather than derived from the
// S forms so that a mis-typed bit shows up here against the manual, not as a
// wrong answer three layers up.

// fmov Dd, Xn (general→FP transfer, 64-bit)
func arm64FMOV_XtoD(dd, xn byte) uint32 {
	return 0x9E670000 | uint32(xn)<<5 | uint32(dd)
}

// fmov Xd, Dn (FP→general transfer, 64-bit)
func arm64FMOV_DtoX(xd, dn byte) uint32 {
	return 0x9E660000 | uint32(dn)<<5 | uint32(xd)
}

// fadd Dd, Dn, Dm (double-precision add)
func arm64FADD_D(dd, dn, dm byte) uint32 {
	return 0x1E602800 | uint32(dm)<<16 | uint32(dn)<<5 | uint32(dd)
}

// fsub Dd, Dn, Dm (double-precision subtract)
func arm64FSUB_D(dd, dn, dm byte) uint32 {
	return 0x1E603800 | uint32(dm)<<16 | uint32(dn)<<5 | uint32(dd)
}

// fmul Dd, Dn, Dm (double-precision multiply)
func arm64FMUL_D(dd, dn, dm byte) uint32 {
	return 0x1E600800 | uint32(dm)<<16 | uint32(dn)<<5 | uint32(dd)
}

// fdiv Dd, Dn, Dm (double-precision divide)
func arm64FDIV_D(dd, dn, dm byte) uint32 {
	return 0x1E601800 | uint32(dm)<<16 | uint32(dn)<<5 | uint32(dd)
}

// fcmp Dn, Dm (double-precision compare, sets NZCV)
func arm64FCMP_D(dn, dm byte) uint32 {
	return 0x1E602000 | uint32(dm)<<16 | uint32(dn)<<5
}

// scvtf Dd, Xn (signed int64→float64)
func arm64SCVTF_XD(dd, xn byte) uint32 {
	return 0x9E620000 | uint32(xn)<<5 | uint32(dd)
}

// fcvtzs Xd, Dn (float64→signed int64, round toward zero)
func arm64FCVTZS_DX(xd, dn byte) uint32 {
	return 0x9E780000 | uint32(dn)<<5 | uint32(xd)
}

// frintn Dd, Dn (round to nearest even, double-precision)
func arm64FRINTN_D(dd, dn byte) uint32 {
	return 0x1E644000 | uint32(dn)<<5 | uint32(dd)
}

// frintz Dd, Dn (round toward zero, double-precision)
func arm64FRINTZ_D(dd, dn byte) uint32 {
	return 0x1E65C000 | uint32(dn)<<5 | uint32(dd)
}

// frintm Dd, Dn (round toward -infinity/floor, double-precision)
func arm64FRINTM_D(dd, dn byte) uint32 {
	return 0x1E654000 | uint32(dn)<<5 | uint32(dd)
}

// frintp Dd, Dn (round toward +infinity/ceil, double-precision)
func arm64FRINTP_D(dd, dn byte) uint32 {
	return 0x1E64C000 | uint32(dn)<<5 | uint32(dd)
}

// lsr Wd, Wn, #shift (immediate, alias for UBFM Wd, Wn, #shift, #31)
func arm64LSR_W_imm(rd, rn byte, shift uint32) uint32 {
	return 0x53007C00 | (shift&0x1F)<<16 | uint32(rn)<<5 | uint32(rd)
}

// lsl Xd, Xn, #shift (immediate, alias for UBFM Xd, Xn, #(64-shift), #(63-shift))
func arm64LSL_imm(rd, rn byte, shift uint32) uint32 {
	immr := (64 - shift) & 0x3F
	imms := (63 - shift) & 0x3F
	return 0xD3400000 | immr<<16 | imms<<10 | uint32(rn)<<5 | uint32(rd)
}

// ubfx Wd, Wn, #lsb, #width (unsigned bit field extract, 32-bit)
func arm64UBFX_W(rd, rn byte, lsb, width uint32) uint32 {
	immr := lsb & 0x1F
	imms := (lsb + width - 1) & 0x1F
	return 0x53000000 | immr<<16 | imms<<10 | uint32(rn)<<5 | uint32(rd)
}

// ===========================================================================
// Emitter helpers
// ===========================================================================

// emitLoadImm64 loads a 64-bit immediate into the given ARM64 register.
func emitLoadImm64(cb *CodeBuffer, rd byte, val uint64) {
	cb.Emit32(arm64MOVZ(rd, uint16(val), 0))
	if val>>16 != 0 {
		cb.Emit32(arm64MOVK(rd, uint16(val>>16), 16))
	}
	if val>>32 != 0 {
		cb.Emit32(arm64MOVK(rd, uint16(val>>32), 32))
	}
	if val>>48 != 0 {
		cb.Emit32(arm64MOVK(rd, uint16(val>>48), 48))
	}
}

// emitLoadImm32 loads a 32-bit value into a W-register.
func emitLoadImm32(cb *CodeBuffer, rd byte, val uint32) {
	cb.Emit32(arm64MOVZ_W(rd, uint16(val), 0))
	if val>>16 != 0 {
		cb.Emit32(arm64MOVK_W(rd, uint16(val>>16), 16))
	}
}

// emitLoadSpilledReg loads an IE64 spilled register (R16-R30) into the given
// ARM64 scratch register from the register file in memory.
func emitLoadSpilledReg(cb *CodeBuffer, arm64Dst, ie64Reg byte) {
	// LDR Xdst, [X8, #ie64Reg*8]
	cb.Emit32(arm64LDR_imm(arm64Dst, arm64RegBase, uint32(ie64Reg)))
}

// emitStoreSpilledReg stores an ARM64 register value back to a spilled IE64
// register in the register file.
func emitStoreSpilledReg(cb *CodeBuffer, arm64Src, ie64Reg byte) {
	// STR Xsrc, [X8, #ie64Reg*8]
	cb.Emit32(arm64STR_imm(arm64Src, arm64RegBase, uint32(ie64Reg)))
}

// resolveReg ensures the IE64 register value is in an ARM64 register.
// For mapped registers, returns the ARM64 register directly.
// For spilled registers, loads into the scratch register and returns it.
func resolveReg(cb *CodeBuffer, ie64Reg byte, scratch byte) byte {
	if ie64Reg == 0 {
		return 31 // XZR
	}
	arm64Reg, mapped := ie64ToARM64Reg(ie64Reg)
	if mapped {
		return arm64Reg
	}
	emitLoadSpilledReg(cb, scratch, ie64Reg)
	return scratch
}

// emitSizeMask applies size masking to the result register.
// For .Q (64-bit): no-op. For .L: use W-register form (implicit zero-extend).
// For .W: AND with 0xFFFF. For .B: AND with 0xFF.
func emitSizeMask(cb *CodeBuffer, rd byte, size byte) {
	switch size {
	case IE64_SIZE_Q:
		// No masking needed for 64-bit
	case IE64_SIZE_L:
		// Zero-extend 32-bit: MOV Wd, Wd clears upper 32 bits
		cb.Emit32(arm64MOV_W(rd, rd))
	case IE64_SIZE_W:
		cb.Emit32(arm64UXTH(rd, rd))
	case IE64_SIZE_B:
		cb.Emit32(arm64UXTB(rd, rd))
	}
}

// ===========================================================================
// Backward Branch Helpers
// ===========================================================================

const jitBudget = 4095 // max ARM64 CMP imm12

// emitStoreRetCount writes the retired instruction count to ctx.RetCount.
// Phase 2: replaces the legacy upper-32-bit packing into X28 so that X28
// can carry a full 64-bit PC across the return channel.
//
// arm64RegCtx (X0) holds the JITContext pointer on block entry but may be
// clobbered by instructions in the block body, so we reload it from
// [SP, #96] (where the prologue stashed it) into a scratch register
// before writing RetCount.
//
// X1 is used as scratch for the count value; X2 holds the reloaded ctx
// pointer. Neither is a stable IE64 register file mapping, so clobbering
// them at block-exit time is safe.
func emitStoreRetCount(cb *CodeBuffer, staticCount uint32, br *blockRegs) {
	staticCount += cb.instrCountBase
	// X2 = ctx ptr (reloaded from [SP, #96]).
	cb.Emit32(arm64LDR_imm(2, 31, 96/8))
	if br.hasBackwardBranch {
		// W1 = X7 (loop counter) + staticCount.
		if staticCount > 0 {
			cb.Emit32(arm64ADD_imm(1, arm64RegLoopCount, staticCount))
		} else {
			cb.Emit32(arm64MOV_W(1, arm64RegLoopCount))
		}
	} else {
		// Static count: load into W1 then store.
		emitLoadImm32(cb, 1, staticCount)
	}
	cb.Emit32(arm64LDR_W_imm(3, 2, uint32(jitCtxOffResumeCountBase/4)))
	cb.Emit32(arm64SUB_W(1, 1, 3))
	cb.Emit32(arm64STR_W_imm(1, 2, uint32(jitCtxOffRetCount/4)))
}

// emitDynamicCount retained for ABI compatibility — rewritten to write
// to ctx.RetCount instead of packing into X28.
func emitDynamicCount(cb *CodeBuffer, staticCount uint32) {
	emitStoreRetCount(cb, staticCount, &blockRegs{hasBackwardBranch: true})
}

// emitPackedPCAndCount sets up the block-exit return channel.
// Phase 2 redesign: X28 carries the full 64-bit target PC (no packing);
// the retired instruction count is written to ctx.RetCount via a separate
// store so that PCs above 4 GiB can survive the channel intact.
func emitPackedPCAndCount(cb *CodeBuffer, targetPC uint64, staticCount uint32, br *blockRegs) {
	emitLoadImm64(cb, arm64RegIE64PC, targetPC)
	emitStoreRetCount(cb, staticCount, br)
}

// ===========================================================================
// Block Prologue / Epilogue
// ===========================================================================

// emitPrologue emits the block entry sequence, saving/loading only
// registers the block actually uses (determined by analyzeBlockRegs).
func emitPrologue(cb *CodeBuffer, blockPC uint64, br *blockRegs) {
	// Frame is always 112 bytes (fixed layout for I/O bail path compatibility)
	cb.Emit32(arm64SUB_imm(31, 31, 112))

	// Save callee-saved pairs only if the block uses the corresponding IE64 regs.
	for i, p := range arm64CalleeSavedPairs {
		if br.used&arm64CalleeSavedMask(i) != 0 {
			cb.Emit32(arm64STP_offset(p.loHost, p.hiHost, 31, p.slot))
		}
	}
	// X27/X28 (SP/PC) and X29/X30 (FP/LR) — always saved
	cb.Emit32(arm64STP_offset(27, 28, 31, 8))
	cb.Emit32(arm64STP_offset(29, 30, 31, 10))

	// Save JITContext pointer at [SP, #96] for I/O bail paths
	cb.Emit32(arm64STR_imm(arm64RegCtx, 31, 96/8))

	// Load base pointers from JITContext (X0 = *JITContext)
	cb.Emit32(arm64LDR_imm(arm64RegBase, arm64RegCtx, uint32(jitCtxOffRegsPtr/8)))
	cb.Emit32(arm64LDR_imm(arm64RegMemBase, arm64RegCtx, uint32(jitCtxOffMemPtr/8)))
	cb.Emit32(arm64LDR_W_imm(arm64RegIOStart, arm64RegCtx, uint32(jitCtxOffIOStart/4)))
	cb.Emit32(arm64LDR_imm(arm64RegIOBitmap, arm64RegCtx, uint32(jitCtxOffIOBitmapPtr/8)))

	// Load FPU base pointer if this block uses FPU instructions
	if br.hasFPU {
		cb.Emit32(arm64LDR_imm(arm64RegFPUBase, arm64RegCtx, uint32(jitCtxOffFPUPtr/8)))
		emitFPResidencyLoadARM64(cb)
	}

	// Zero X7 (loop iteration counter) for blocks with backward branches
	if br.hasBackwardBranch {
		cb.Emit32(arm64MOVZ(arm64RegLoopCount, 0, 0))
	}

	// Load every IE64 register the block reads *or* writes (br.used), not just
	// the ones it reads. A write-only register still has to arrive holding its
	// canonical value, because the epilogue stores it back unconditionally and
	// would otherwise publish whatever the host register happened to contain.
	//
	// Two ways that bites, both of which loading br.read alone gets wrong:
	//
	//   - Mixed JIT/interpreter handoff. A helper-exiting LOAD bails, the Go
	//     dispatcher writes the result into cpu.regs[rd], and the block resumes
	//     through emitResumeEntryARM64 -> emitPrologue. If rd is write-only the
	//     resume would not reload it, and the next epilogue would store the
	//     stale host register straight over the helper's result. (The Go runtime
	//     runs in between, so the stale value is typically one of its heap
	//     pointers.)
	//   - A loop that exits before reaching a register it writes later. A
	//     conditional-branch exit normally stores writtenSoFar, so a
	//     forward-only block is unaffected; but in a block with a backward edge
	//     the emitter widens that to br.written, because a prior iteration may
	//     have written registers appearing after the branch. On the first
	//     iteration those writes have not run yet.
	//
	// br.used is exactly the set whose callee-saved pairs are preserved above,
	// so this cannot load a register the frame has not saved. amd64 solves the
	// same problem by loading every mapped register unconditionally; its
	// resident set is 4 registers wide, where this backend maps 14, so it takes
	// the analysis rather than the blanket load.
	for ie64Reg := byte(ie64FirstMapped); ie64Reg <= ie64LastMapped; ie64Reg++ {
		if br.used&(1<<ie64Reg) != 0 {
			arm64Reg, _ := ie64ToARM64Reg(ie64Reg)
			cb.Emit32(arm64LDR_imm(arm64Reg, arm64RegBase, uint32(ie64Reg)))
		}
	}

	// Load IE64 R31 (SP) unconditionally. Phase 5 helper-exit paths
	// flush X27 into ctx.LiveSP for the Go dispatcher to copy back into
	// cpu.regs[31]; conditional loading would leave X27 holding the
	// host-saved callee value in blocks that only write SP later (e.g.
	// MOVEQ ..., R31) or whose first SP touch is a helper-exiting LOAD
	// before any SP-reading instruction. Cost: one LDR per block.
	cb.Emit32(arm64LDR_imm(arm64RegIE64SP, arm64RegBase, 31))

	// Load block start PC into X28
	emitLoadImm64(cb, arm64RegIE64PC, uint64(blockPC))
}

func emitResumeEntryARM64(cb *CodeBuffer, resumePC uint64, br *blockRegs) {
	emitPrologue(cb, resumePC, br)
}

func emitHelperResumeFieldsARM64(cb *CodeBuffer, ctxReg byte, resumePC uint64, countBase uint32, br *blockRegs) int {
	if br.hasBackwardBranch {
		emitLoadImm32(cb, 2, 0)
		cb.Emit32(arm64STR_W_imm(2, ctxReg, uint32(jitCtxOffResumeValid/4)))
		return -1
	}

	adrOff := cb.Len()
	cb.Emit32(0) // Patched to ADR X2, resume_label once the label is emitted.
	cb.Emit32(arm64STR_imm(2, ctxReg, uint32(jitCtxOffResumeAddr/8)))
	emitLoadImm64(cb, 2, resumePC)
	cb.Emit32(arm64STR_imm(2, ctxReg, uint32(jitCtxOffResumePC/8)))
	emitLoadImm32(cb, 2, countBase)
	cb.Emit32(arm64STR_W_imm(2, ctxReg, uint32(jitCtxOffResumeCountBase/4)))
	cb.Emit32(arm64LDR_W_imm(2, ctxReg, uint32(jitCtxOffMMUEnabled/4)))
	cb.Emit32(arm64STR_W_imm(2, ctxReg, uint32(jitCtxOffResumeMMUEnabled/4)))
	emitLoadImm32(cb, 2, 1)
	cb.Emit32(arm64STR_W_imm(2, ctxReg, uint32(jitCtxOffResumeValid/4)))
	return adrOff
}

func patchResumeADRARM64(cb *CodeBuffer, adrOff int, resumeOff int) {
	if adrOff < 0 {
		return
	}
	offset := int32(resumeOff - adrOff)
	cb.PatchUint32(adrOff, arm64ADR(2, offset))
}

// emitEpilogue emits the block exit sequence, storing/restoring only the
// registers indicated by the bitmasks.
//   - storeRegs: IE64 registers to store back (writtenRegs for normal, writtenSoFar for bail)
//   - calleeSaved: which callee-saved pairs to restore (must match what prologue saved)
func emitEpilogue(cb *CodeBuffer, storeRegs uint32, calleeSaved uint32) {
	// Materialise any sunk FPSR CC update. This is the block's only exit
	// funnel, so it has to happen here for every path that leaves. It must come
	// before the callee-saved restores below, which retire X6 (the FPU base).
	emitMaterializeFPCCARM64(cb)
	emitFPResidencySpillARM64(cb)

	// Store only the IE64 registers that were written
	for ie64Reg := byte(ie64FirstMapped); ie64Reg <= ie64LastMapped; ie64Reg++ {
		if storeRegs&(1<<ie64Reg) != 0 {
			arm64Reg, _ := ie64ToARM64Reg(ie64Reg)
			cb.Emit32(arm64STR_imm(arm64Reg, arm64RegBase, uint32(ie64Reg)))
		}
	}

	// Store IE64 R31 (SP) if written
	if storeRegs&(1<<31) != 0 {
		cb.Emit32(arm64STR_imm(arm64RegIE64SP, arm64RegBase, 31))
	}

	// Phase 2 return channel: write X28 (full 64-bit PC) to ctx.RetPC.
	// Count was already written to ctx.RetCount at the call site. The
	// legacy packed regs[0] channel is retained as belt-and-suspenders
	// but is no longer the source of truth.
	//
	// arm64RegCtx (X0) may have been clobbered during block execution, so
	// reload the ctx pointer from [SP, #96] into X2 before storing RetPC.
	cb.Emit32(arm64LDR_imm(2, 31, 96/8))
	cb.Emit32(arm64STR_imm(arm64RegIE64PC, 2, uint32(jitCtxOffRetPC/8)))
	cb.Emit32(arm64STR_imm(arm64RegIE64PC, arm64RegBase, 0))

	// Restore callee-saved pairs that were saved in prologue
	for i, p := range arm64CalleeSavedPairs {
		if calleeSaved&arm64CalleeSavedMask(i) != 0 {
			cb.Emit32(arm64LDP_offset(p.loHost, p.hiHost, 31, p.slot))
		}
	}
	cb.Emit32(arm64LDP_offset(27, 28, 31, 8))
	cb.Emit32(arm64LDP_offset(29, 30, 31, 10))
	cb.Emit32(arm64ADD_imm(31, 31, 112))

	cb.Emit32(arm64RET())
}

// ===========================================================================
// Instruction Compilation
// ===========================================================================

// compileBlock compiles a scanned block of IE64 instructions to ARM64 machine code.
func compileBlock(instrs []JITInstr, startPC uint64, execMem *ExecMem) (*JITBlock, error) {
	cb := NewCodeBuffer(len(instrs) * 256) // FPU ops can emit 30-60 ARM64 instructions with CC setting

	// Decide, for each FP condition-code write, whether no observer can reach it
	// (drop the classifier entirely) or only the block's exits can (defer it to
	// emitEpilogue). Per sub-block, so nothing is elided or deferred across a
	// block boundary.
	//
	// The sink slot needs no reset: it lives on the CodeBuffer just allocated,
	// so it starts empty and cannot outlive this compilation.
	ie64MarkFPSRCCDead(instrs)

	br := analyzeBlockRegs(instrs)
	br.hasBackwardBranch = detectBackwardBranches(instrs, startPC)
	prevLoopPlan := ie64ActiveLoopPlan
	ie64ActiveLoopPlan = ie64AnalyseLoop(instrs, startPC)
	defer func() { ie64ActiveLoopPlan = prevLoopPlan }()
	prevFoldPlan := ie64ActiveFoldPlan
	ie64ActiveFoldPlan = ie64AnalyseConstFold(instrs, startPC)
	defer func() { ie64ActiveFoldPlan = prevFoldPlan }()
	if br.hasFPU && ie64FPResidencyEnabled() {
		if plan, ok := ie64BuildBlockFPPlan(instrs); ok {
			cb.fpPlan = &plan
		}
	}
	emitPrologue(cb, startPC, &br)

	// Track ARM64 code offsets for each IE64 instruction (for backward branches)
	instrOffsets := make([]int, len(instrs))

	// Track which registers have been written by instructions executed so far.
	// Used for I/O bail epilogues: only store registers that were actually
	// written by instructions preceding the bail point.
	writtenSoFar := uint32(0)
	for i := range instrs {
		if ie64ActiveLoopPlan != nil && len(ie64ActiveLoopPlan.accesses) != 0 && i == ie64ActiveLoopPlan.head {
			emitIE64LoopPrecheckARM64(cb, &br, writtenSoFar, ie64ActiveLoopPlan)
		}
		if ie64ActiveLoopPlan != nil && len(ie64ActiveLoopPlan.hoists) != 0 && i == ie64ActiveLoopPlan.head {
			// Hoisted invariants: emitted once, in program order,
			// immediately before the loop-head label, so back-edge targets
			// land after them.
			for _, hi := range ie64ActiveLoopPlan.hoists {
				hj := &instrs[hi]
				emitInstruction(cb, hj, startPC, false, &br, writtenSoFar, hi, instrOffsets)
				writtenSoFar |= instrWrittenRegs(hj)
			}
			ie64LoopHoistEmits.Add(1)
		}
		instrOffsets[i] = cb.Len()
		ji := &instrs[i]
		ie64CurrentLoopInstr = i
		if ie64ActiveLoopPlan != nil && ie64ActiveLoopPlan.hoistSet[i] {
			// Suppressed inside the loop: the host instruction ran once
			// before the loop head. The guest instruction stays in every
			// index-based retired count.
			writtenSoFar |= instrWrittenRegs(ji)
			continue
		}
		if f := ie64FoldEntryAt(i); f.folded {
			emitFoldedConst(cb, ji.rd, f.value)
		} else {
			emitInstruction(cb, ji, startPC, i == len(instrs)-1, &br, writtenSoFar, i, instrOffsets)
		}
		writtenSoFar |= instrWrittenRegs(ji)
	}

	// Emit final epilogue (if the last instruction doesn't have its own)
	lastOp := instrs[len(instrs)-1].opcode
	if !isBlockTerminator(lastOp) {
		endPC := startPC + uint64(len(instrs))*IE64_INSTR_SIZE
		emitPackedPCAndCount(cb, endPC, uint32(len(instrs)), &br)
		emitEpilogue(cb, br.written, br.used)
	}

	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, err
	}

	return &JITBlock{
		startPC:    startPC,
		endPC:      startPC + uint64(len(instrs))*IE64_INSTR_SIZE,
		instrCount: len(instrs),
		execAddr:   addr,
		execSize:   len(code),
	}, nil
}

// emitInstruction emits ARM64 code for a single IE64 instruction.
// br contains register usage info; writtenSoFar tracks which registers
// have been written by instructions emitted before this one (for I/O bail).
// instrIdx is the 0-based index within the block; instrOffsets maps instruction
// indices to ARM64 code byte offsets (for backward branch targets).
func emitInstruction(cb *CodeBuffer, ji *JITInstr, blockStartPC uint64, isLast bool, br *blockRegs, writtenSoFar uint32, instrIdx int, instrOffsets []int) {
	instrPC := blockStartPC + uint64(ji.pcOffset)

	switch ji.opcode {
	// ======================================================================
	// Data Movement
	// ======================================================================
	case OP_MOVE:
		emitMOVE(cb, ji)
	case OP_MOVT:
		emitMOVT(cb, ji)
	case OP_MOVEQ:
		emitMOVEQ(cb, ji)
	case OP_LEA:
		emitLEA(cb, ji)

	// ======================================================================
	// Memory Access
	// ======================================================================
	case OP_LOAD:
		if ji.mmuBail {
			emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
			return
		}
		emitLOAD(cb, ji, instrPC, br, writtenSoFar)
	case OP_STORE:
		if ji.mmuBail {
			emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
			return
		}
		emitSTORE(cb, ji, instrPC, br, writtenSoFar)

	// ======================================================================
	// Arithmetic
	// ======================================================================
	case OP_ADD:
		emitALU(cb, ji, jitALUAdd)
	case OP_SUB:
		emitALU(cb, ji, jitALUSub)
	case OP_MULU:
		emitMULU(cb, ji)
	case OP_MULS:
		emitMULS(cb, ji)
	case OP_DIVU:
		emitDIVU(cb, ji)
	case OP_DIVS:
		emitDIVS(cb, ji)
	case OP_MOD64:
		emitMOD(cb, ji)
	case OP_NEG:
		emitNEG(cb, ji)
	case OP_MODS:
		emitMODS(cb, ji)
	case OP_MULHU:
		emitMULHU(cb, ji)
	case OP_MULHS:
		emitMULHS(cb, ji)

	// ======================================================================
	// Logic
	// ======================================================================
	case OP_AND64:
		emitALU(cb, ji, jitALUAnd)
	case OP_OR64:
		emitALU(cb, ji, jitALUOr)
	case OP_EOR:
		emitALU(cb, ji, jitALUEor)
	case OP_NOT64:
		emitNOT(cb, ji)
	case OP_LSL:
		emitShift(cb, ji, jitShiftLSL)
	case OP_LSR:
		emitShift(cb, ji, jitShiftLSR)
	case OP_ASR:
		emitASR(cb, ji)
	case OP_CLZ:
		emitCLZ(cb, ji)
	case OP_SEXT:
		emitSEXT(cb, ji)
	case OP_ROL:
		emitRotate(cb, ji, true)
	case OP_ROR:
		emitRotate(cb, ji, false)
	case OP_CTZ:
		emitCTZ(cb, ji)
	case OP_POPCNT:
		emitPOPCNT(cb, ji)
	case OP_BSWAP:
		emitBSWAP(cb, ji)

	// ======================================================================
	// Branches
	// ======================================================================
	case OP_BRA:
		emitBRA(cb, ji, instrPC, br, instrIdx, instrOffsets, blockStartPC)
	case OP_BEQ:
		emitBcc(cb, ji, instrPC, arm64CondEQ, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BNE:
		emitBcc(cb, ji, instrPC, arm64CondNE, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BLT:
		emitBcc(cb, ji, instrPC, arm64CondLT, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BGE:
		emitBcc(cb, ji, instrPC, arm64CondGE, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BGT:
		emitBcc(cb, ji, instrPC, arm64CondGT, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BLE:
		emitBcc(cb, ji, instrPC, arm64CondLE, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BHI:
		emitBcc(cb, ji, instrPC, arm64CondHI, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BLS:
		emitBcc(cb, ji, instrPC, arm64CondLS, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_JMP:
		emitJMP(cb, ji, br, ji.pcOffset/IE64_INSTR_SIZE+1)

	// ======================================================================
	// Subroutine / Stack
	// ======================================================================
	case OP_JSR64:
		if ji.mmuBail {
			emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
			return
		}
		emitJSR(cb, ji, instrPC, br, writtenSoFar)
	case OP_RTS64:
		if ji.mmuBail {
			emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
			return
		}
		emitRTS(cb, ji, instrPC, br, ji.pcOffset/IE64_INSTR_SIZE+1, writtenSoFar)
	case OP_PUSH64:
		if ji.mmuBail {
			emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
			return
		}
		emitPUSH(cb, ji, instrPC, br, writtenSoFar)
	case OP_POP64:
		if ji.mmuBail {
			emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
			return
		}
		emitPOP(cb, ji, instrPC, br, writtenSoFar)
	case OP_JSR_IND:
		if ji.mmuBail {
			emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
			return
		}
		emitJSR_IND(cb, ji, instrPC, br, ji.pcOffset/IE64_INSTR_SIZE+1, writtenSoFar)

	// ======================================================================
	// System
	// ======================================================================
	case OP_HALT64:
		emitPackedPCAndCount(cb, uint64(instrPC), uint32(instrIdx+1), br)
		emitEpilogue(cb, br.written, br.used)

	case OP_RTI64:
		emitRTI(cb, ji, instrPC, br, writtenSoFar)

	case OP_WAIT64:
		emitWAIT(cb, ji, instrPC, br, writtenSoFar)

	case OP_NOP64:
		cb.Emit32(arm64NOP())

	// SEI64/CLI64 mutate interruptEnabled, which lives in the CPU struct and is
	// read by the interrupt-delivery gate. Bail to the interpreter per instruction
	// so the architectural flag is updated; compiling them as NOPs would silently
	// drop the state change under JIT (timer-off native execution).
	case OP_SEI64, OP_CLI64:
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)

	// ======================================================================
	// FPU — Category A (pure integer bitwise on FP registers)
	// ======================================================================
	case OP_FMOV:
		emitFMOV(cb, ji)
	case OP_FABS:
		emitFABS(cb, ji)
	case OP_FNEG:
		emitFNEG(cb, ji)
	case OP_FMOVI:
		emitFMOVI(cb, ji)
	case OP_FMOVO:
		emitFMOVO(cb, ji)
	case OP_FMOVECR:
		emitFMOVECR(cb, ji)
	case OP_FMOVSR:
		emitFMOVSR(cb, ji)
	case OP_FMOVCR:
		emitFMOVCR(cb, ji)
	case OP_FMOVSC:
		emitFMOVSC(cb, ji)
	case OP_FMOVCC:
		emitFMOVCC(cb, ji)

	// ======================================================================
	// FPU — Category B (native ARM64 FP instructions)
	// ======================================================================
	case OP_FADD:
		emitFADD(cb, ji)
	case OP_FSUB:
		emitFSUB(cb, ji)
	case OP_FMUL:
		emitFMUL(cb, ji)
	case OP_FDIV:
		emitFDIV(cb, ji)
	case OP_FSQRT:
		emitFSQRT(cb, ji)
	case OP_FINT:
		emitFINT(cb, ji)
	case OP_FCMP:
		emitFCMP(cb, ji)
	case OP_FCVTIF:
		emitFCVTIF(cb, ji)
	case OP_FCVTFI:
		emitFCVTFI(cb, ji)

	// ======================================================================
	// FPU — Memory
	// ======================================================================
	case OP_FLOAD:
		if ji.mmuBail {
			emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
			return
		}
		emitFLOAD(cb, ji, instrPC, br, writtenSoFar)
	case OP_FSTORE:
		if ji.mmuBail {
			emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
			return
		}
		emitFSTORE(cb, ji, instrPC, br, writtenSoFar)

	// ======================================================================
	// FPU — Category C (transcendentals use canonical helper exits)
	// ======================================================================
	case OP_FMOD, OP_FSIN, OP_FCOS, OP_FTAN, OP_FATAN, OP_FLOG, OP_FEXP, OP_FPOW:
		emitFPTransHelperExitARM64(cb, ji, instrPC, HELPER_FTRANS, br, writtenSoFar)
	case OP_DLOAD:
		emitDLOAD(cb, ji, instrPC, br, writtenSoFar)
	case OP_DSTORE:
		emitDSTORE(cb, ji, instrPC, br, writtenSoFar)
	case OP_DSIN, OP_DCOS, OP_DTAN, OP_DATAN, OP_DLOG, OP_DEXP, OP_DPOW:
		emitDTransHelperExitARM64(cb, ji, instrPC, br, writtenSoFar)
	case OP_DMOV:
		emitDMOV_ARM64(cb, ji, instrPC, br, writtenSoFar)
	case OP_DADD:
		emitDPBinaryARM64(cb, ji, instrPC, br, writtenSoFar, arm64FADD_D)
	case OP_DSUB:
		emitDPBinaryARM64(cb, ji, instrPC, br, writtenSoFar, arm64FSUB_D)
	case OP_DMUL:
		emitDPBinaryARM64(cb, ji, instrPC, br, writtenSoFar, arm64FMUL_D)
	case OP_DDIV:
		emitDPBinaryARM64(cb, ji, instrPC, br, writtenSoFar, arm64FDIV_D)
	case OP_DINT:
		emitDINT_ARM64(cb, ji, instrPC, br, writtenSoFar)
	case OP_DCMP:
		emitDCMP_ARM64(cb, ji, instrPC, br, writtenSoFar)
	case OP_DCVTIF:
		emitDCVTIF_ARM64(cb, ji, instrPC, br, writtenSoFar)
	case OP_DCVTFI:
		emitDCVTFI_ARM64(cb, ji, instrPC, br, writtenSoFar)

	case OP_DMOD:
		emitFPTransHelperExitARM64(cb, ji, instrPC, HELPER_DTRANS, br, writtenSoFar)
	// Still interpreted on both backends. Keep this list and the amd64 one in
	// step: sdk/docs/IE64_JIT.md documents a single shared fallback table.
	case OP_DABS, OP_DNEG, OP_DSQRT, OP_FCVTSD, OP_FCVTDS:
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)

	// MMU/privilege opcodes: always bail to interpreter
	case OP_MTCR, OP_MFCR, OP_ERET, OP_TLBFLUSH, OP_TLBINVAL, OP_SYSCALL, OP_SMODE,
		OP_SUAEN, OP_SUADIS:
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
		return

	case OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
		emitAtomic(cb, ji, instrPC, br, writtenSoFar)
		return

	default:
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
	}
}

// ===========================================================================
// ALU emission helpers
// ===========================================================================

type jitALUOp int

const (
	jitALUAdd jitALUOp = iota
	jitALUSub
	jitALUAnd
	jitALUOr
	jitALUEor
)

type jitShiftOp int

const (
	jitShiftLSL jitShiftOp = iota
	jitShiftLSR
)

// emitALU handles ADD, SUB, AND, OR, EOR with register or immediate operand.
func emitALU(cb *CodeBuffer, ji *JITInstr, op jitALUOp) {
	if ji.rd == 0 {
		return // R0 is hardwired zero
	}

	rsReg := resolveReg(cb, ji.rs, 0) // scratch X0

	var opReg byte
	if ji.xbit == 1 {
		// Load immediate into X1
		emitLoadImm64(cb, 1, uint64(ji.imm32))
		opReg = 1
	} else {
		opReg = resolveReg(cb, ji.rt, 1) // scratch X1
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2 // scratch X2 for spilled destination
	}

	// Emit the operation
	switch op {
	case jitALUAdd:
		if ji.size == IE64_SIZE_L {
			cb.Emit32(arm64ADD_W(dstReg, rsReg, opReg))
		} else {
			cb.Emit32(arm64ADD(dstReg, rsReg, opReg))
		}
	case jitALUSub:
		if ji.size == IE64_SIZE_L {
			cb.Emit32(arm64SUB_W(dstReg, rsReg, opReg))
		} else {
			cb.Emit32(arm64SUB(dstReg, rsReg, opReg))
		}
	case jitALUAnd:
		if ji.size == IE64_SIZE_L {
			cb.Emit32(arm64AND_W(dstReg, rsReg, opReg))
		} else {
			cb.Emit32(arm64AND(dstReg, rsReg, opReg))
		}
	case jitALUOr:
		if ji.size == IE64_SIZE_L {
			cb.Emit32(arm64ORR_W(dstReg, rsReg, opReg))
		} else {
			cb.Emit32(arm64ORR(dstReg, rsReg, opReg))
		}
	case jitALUEor:
		if ji.size == IE64_SIZE_L {
			cb.Emit32(arm64EOR_W(dstReg, rsReg, opReg))
		} else {
			cb.Emit32(arm64EOR(dstReg, rsReg, opReg))
		}
	}

	// Apply size mask for .B and .W (the .L W-register forms already zero-extend)
	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMask(cb, dstReg, ji.size)
	}

	// Store back if spilled
	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitMOVE handles MOVE rd, rs/imm
// emitFoldedConst writes a precomputed constant to the destination register
// through the normal destination-write path (same shape as emitMOVE's
// immediate arm, widened to a full 64-bit value).
func emitFoldedConst(cb *CodeBuffer, rd byte, value uint64) {
	if rd == 0 {
		return
	}
	dstReg, mapped := ie64ToARM64Reg(rd)
	if !mapped {
		dstReg = 2
	}
	emitLoadImm64(cb, dstReg, value)
	if !mapped {
		emitStoreSpilledReg(cb, dstReg, rd)
	}
	ie64FoldedConstEmits.Add(1)
}

func emitMOVE(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2 // scratch
	}

	if ji.xbit == 1 {
		// MOVE rd, #imm32 — load immediate masked to size
		val := uint64(ji.imm32) & ie64SizeMask[ji.size]
		emitLoadImm64(cb, dstReg, val)
	} else {
		// MOVE rd, rs — register copy masked to size
		srcReg := resolveReg(cb, ji.rs, 0)
		if ji.size == IE64_SIZE_Q {
			cb.Emit32(arm64MOV(dstReg, srcReg))
		} else {
			cb.Emit32(arm64MOV(dstReg, srcReg))
			emitSizeMask(cb, dstReg, ji.size)
		}
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitMOVT handles MOVT rd, #imm32 (move to upper 32 bits)
func emitMOVT(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
		emitLoadSpilledReg(cb, dstReg, ji.rd)
	}

	// Clear upper 32 bits, keep lower 32
	cb.Emit32(arm64MOV_W(dstReg, dstReg)) // zero-extends

	// Set upper 32 bits from imm32
	emitLoadImm64(cb, 0, uint64(ji.imm32)<<32) // X0 = imm32 << 32
	cb.Emit32(arm64ORR(dstReg, dstReg, 0))     // dst |= X0

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitMOVEQ handles MOVEQ rd, #imm32 (sign-extend 32→64)
func emitMOVEQ(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	// Load imm32 into W-register, then sign-extend to X
	emitLoadImm32(cb, dstReg, ji.imm32)
	cb.Emit32(arm64SXTW(dstReg, dstReg))

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitLEA handles LEA rd, disp(rs)
func emitLEA(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	// LEA computes: int64(rs) + int64(int32(imm32))
	emitLoadImm32(cb, 1, ji.imm32) // X1 = imm32 (zero-extended)
	cb.Emit32(arm64SXTW(1, 1))     // X1 = sign-extend to 64-bit
	cb.Emit32(arm64ADD(dstReg, rsReg, 1))

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitMULU handles MULU rd, rs, rt/imm (unsigned multiply)
func emitMULU(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	var opReg byte
	if ji.xbit == 1 {
		emitLoadImm64(cb, 1, uint64(ji.imm32))
		opReg = 1
	} else {
		opReg = resolveReg(cb, ji.rt, 1)
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	if ji.size == IE64_SIZE_L {
		cb.Emit32(arm64MUL_W(dstReg, rsReg, opReg))
	} else {
		cb.Emit32(arm64MUL(dstReg, rsReg, opReg))
	}

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMask(cb, dstReg, ji.size)
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitMULS handles MULS rd, rs, rt/imm (signed multiply)
func emitMULS(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	var opReg byte
	if ji.xbit == 1 {
		emitLoadImm64(cb, 1, uint64(ji.imm32))
		opReg = 1
	} else {
		opReg = resolveReg(cb, ji.rt, 1)
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	// MULS: int64(rs) * int64(operand3), masked to size
	// For all sizes, the interpreter does: maskToSize(uint64(int64(rs)*int64(op3)), size)
	// ARM64 MUL is unsigned but produces same low bits for signed*signed
	if ji.size == IE64_SIZE_L {
		cb.Emit32(arm64MUL_W(dstReg, rsReg, opReg))
	} else {
		cb.Emit32(arm64MUL(dstReg, rsReg, opReg))
	}

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMask(cb, dstReg, ji.size)
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitMULHU handles MULHU rd, rs, rt/imm (unsigned high multiply).
func emitMULHU(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	var opReg byte
	if ji.xbit == 1 {
		emitLoadImm64(cb, 1, uint64(ji.imm32))
		opReg = 1
	} else {
		opReg = resolveReg(cb, ji.rt, 1)
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	cb.Emit32(arm64UMULH(dstReg, rsReg, opReg))

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitMULHS handles MULHS rd, rs, rt/imm (signed high multiply).
func emitMULHS(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	var opReg byte
	if ji.xbit == 1 {
		emitLoadImm64(cb, 1, uint64(ji.imm32))
		opReg = 1
	} else {
		opReg = resolveReg(cb, ji.rt, 1)
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	cb.Emit32(arm64SMULH(dstReg, rsReg, opReg))

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitDIVU handles DIVU rd, rs, rt/imm (unsigned divide)
func emitDIVU(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	var opReg byte
	if ji.xbit == 1 {
		emitLoadImm64(cb, 1, uint64(ji.imm32))
		opReg = 1
	} else {
		opReg = resolveReg(cb, ji.rt, 1)
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	// ARM64 UDIV returns 0 for divide by zero, which matches IE64 semantics
	if ji.size == IE64_SIZE_L {
		cb.Emit32(arm64UDIV_W(dstReg, rsReg, opReg))
	} else {
		cb.Emit32(arm64UDIV(dstReg, rsReg, opReg))
	}

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMask(cb, dstReg, ji.size)
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitDIVS handles DIVS rd, rs, rt/imm (signed divide)
func emitDIVS(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	var opReg byte
	if ji.xbit == 1 {
		emitLoadImm64(cb, 1, uint64(ji.imm32))
		opReg = 1
	} else {
		opReg = resolveReg(cb, ji.rt, 1)
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	// ARM64 SDIV returns 0 for divide by zero
	if ji.size == IE64_SIZE_L {
		cb.Emit32(arm64SDIV_W(dstReg, rsReg, opReg))
	} else {
		cb.Emit32(arm64SDIV(dstReg, rsReg, opReg))
	}

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMask(cb, dstReg, ji.size)
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitMOD handles MOD rd, rs, rt/imm (unsigned modulo)
func emitMOD(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	var opReg byte
	if ji.xbit == 1 {
		emitLoadImm64(cb, 1, uint64(ji.imm32))
		opReg = 1
	} else {
		opReg = resolveReg(cb, ji.rt, 1)
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	// MOD = rs - (rs/operand3)*operand3
	// Use MSUB: Xd = Xa - Xn*Xm → MSUB dstReg, quotient, opReg, rsReg
	// First compute quotient in X3
	if ji.size == IE64_SIZE_L {
		cb.Emit32(arm64UDIV_W(3, rsReg, opReg))
		// MSUB Wd = Wa - Wn*Wm: 0x1B008000 | rm<<16 | ra<<10 | rn<<5 | rd
		cb.Emit32(0x1B000000 | uint32(opReg)<<16 | uint32(rsReg)<<10 | uint32(3)<<5 | uint32(dstReg) | 0x00008000)
	} else {
		cb.Emit32(arm64UDIV(3, rsReg, opReg))
		// MSUB Xd = Xa - Xn*Xm: 0x9B008000 | rm<<16 | ra<<10 | rn<<5 | rd
		cb.Emit32(0x9B000000 | uint32(opReg)<<16 | uint32(rsReg)<<10 | uint32(3)<<5 | uint32(dstReg) | 0x00008000)
	}

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMask(cb, dstReg, ji.size)
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

func emitSignExtendForSize(cb *CodeBuffer, dst, src byte, size byte) {
	switch size {
	case IE64_SIZE_B:
		cb.Emit32(arm64SXTB(dst, src))
	case IE64_SIZE_W:
		cb.Emit32(arm64SXTH(dst, src))
	case IE64_SIZE_L:
		cb.Emit32(arm64SXTW(dst, src))
	case IE64_SIZE_Q:
		if dst != src {
			cb.Emit32(arm64MOV(dst, src))
		}
	}
}

// emitMODS handles signed modulo with IE64 size-based sign extension.
func emitMODS(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	emitSignExtendForSize(cb, 0, rsReg, ji.size) // X0 = dividend

	if ji.xbit == 1 {
		emitLoadImm64(cb, 1, uint64(ji.imm32))
		emitSignExtendForSize(cb, 1, 1, ji.size)
	} else {
		opReg := resolveReg(cb, ji.rt, 1)
		emitSignExtendForSize(cb, 1, opReg, ji.size) // X1 = divisor
	}

	zeroOff := cb.Len()
	cb.Emit32(0) // CBZ X1, zero

	cb.Emit32(arm64SDIV(3, 0, 1))    // X3 = X0 / X1
	cb.Emit32(arm64MSUB(2, 3, 1, 0)) // X2 = X0 - X3*X1
	doneOff1 := cb.Len()
	cb.Emit32(0) // B done

	zeroPC := cb.Len()
	cb.PatchUint32(zeroOff, arm64CBZ(1, int32(zeroPC-zeroOff)))
	cb.Emit32(arm64MOV(2, 31)) // X2 = 0

	donePC := cb.Len()
	cb.PatchUint32(doneOff1, arm64B(int32(donePC-doneOff1)))

	if ji.size != IE64_SIZE_Q {
		emitSizeMask(cb, 2, ji.size)
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if mapped {
		cb.Emit32(arm64MOV(dstReg, 2))
	} else {
		emitStoreSpilledReg(cb, 2, ji.rd)
	}
}

// emitNEG handles NEG rd, rs
func emitNEG(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	if ji.size == IE64_SIZE_L {
		cb.Emit32(arm64NEG_W(dstReg, rsReg))
	} else {
		cb.Emit32(arm64NEG(dstReg, rsReg))
	}

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMask(cb, dstReg, ji.size)
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitNOT handles NOT rd, rs
func emitNOT(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	if ji.size == IE64_SIZE_L {
		cb.Emit32(arm64MVN_W(dstReg, rsReg))
	} else {
		cb.Emit32(arm64MVN(dstReg, rsReg))
		// For .Q the full 64-bit NOT is correct
		// For .B/.W we need to mask
	}

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMask(cb, dstReg, ji.size)
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitShift handles LSL and LSR
func emitShift(cb *CodeBuffer, ji *JITInstr, op jitShiftOp) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	var opReg byte
	if ji.xbit == 1 {
		emitLoadImm64(cb, 1, uint64(ji.imm32)&63) // mask shift amount
		opReg = 1
	} else {
		opReg = resolveReg(cb, ji.rt, 1)
		// Mask shift amount to 63 bits: AND X1, opReg, #63
		cb.Emit32(arm64AND_imm(1, opReg, 0, 5, 1)) // N=1, immr=0, imms=5 encodes 0x3F for X
		opReg = 1
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	switch op {
	case jitShiftLSL:
		if ji.size == IE64_SIZE_L {
			cb.Emit32(arm64LSL_W(dstReg, rsReg, opReg))
		} else {
			cb.Emit32(arm64LSL(dstReg, rsReg, opReg))
		}
	case jitShiftLSR:
		if ji.size == IE64_SIZE_L {
			cb.Emit32(arm64LSR_W(dstReg, rsReg, opReg))
		} else {
			cb.Emit32(arm64LSR(dstReg, rsReg, opReg))
		}
	}

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMask(cb, dstReg, ji.size)
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitASR handles ASR (arithmetic shift right) with sign-extension per size
func emitASR(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	var opReg byte
	if ji.xbit == 1 {
		emitLoadImm64(cb, 1, uint64(ji.imm32)&63)
		opReg = 1
	} else {
		opReg = resolveReg(cb, ji.rt, 1)
		cb.Emit32(arm64AND_imm(1, opReg, 0, 5, 1)) // mask to 63
		opReg = 1
	}

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	// ASR needs sign-extension before shifting:
	// .B: sign-extend from 8 bits, .W: from 16, .L: from 32, .Q: native 64
	switch ji.size {
	case IE64_SIZE_B:
		cb.Emit32(arm64SXTB(dstReg, rsReg))
		cb.Emit32(arm64ASR(dstReg, dstReg, opReg))
		emitSizeMask(cb, dstReg, IE64_SIZE_B)
	case IE64_SIZE_W:
		cb.Emit32(arm64SXTH(dstReg, rsReg))
		cb.Emit32(arm64ASR(dstReg, dstReg, opReg))
		emitSizeMask(cb, dstReg, IE64_SIZE_W)
	case IE64_SIZE_L:
		cb.Emit32(arm64SXTW(dstReg, rsReg))
		cb.Emit32(arm64ASR(dstReg, dstReg, opReg))
		emitSizeMask(cb, dstReg, IE64_SIZE_L)
	case IE64_SIZE_Q:
		cb.Emit32(arm64ASR(dstReg, rsReg, opReg))
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitCLZ handles CLZ rd, rs (count leading zeros of 32-bit value)
func emitCLZ(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	// IE64 CLZ always operates on 32-bit value (LeadingZeros32)
	cb.Emit32(arm64CLZ_W(dstReg, rsReg))

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitSEXT handles SEXT rd, rs.
func emitSEXT(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	switch ji.size {
	case IE64_SIZE_B:
		cb.Emit32(arm64SXTB(dstReg, rsReg))
	case IE64_SIZE_W:
		cb.Emit32(arm64SXTH(dstReg, rsReg))
	case IE64_SIZE_L:
		cb.Emit32(arm64SXTW(dstReg, rsReg))
	case IE64_SIZE_Q:
		cb.Emit32(arm64MOV(dstReg, rsReg))
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

func arm64RotateMask(size byte) uint32 {
	switch size {
	case IE64_SIZE_B:
		return 7
	case IE64_SIZE_W:
		return 15
	case IE64_SIZE_L:
		return 31
	default:
		return 63
	}
}

func emitRotateCount(cb *CodeBuffer, ji *JITInstr, scratch byte, size byte) {
	if ji.xbit == 1 {
		emitLoadImm32(cb, scratch, ji.imm32&arm64RotateMask(size))
		return
	}

	opReg := resolveReg(cb, ji.rt, scratch)
	if opReg != scratch {
		cb.Emit32(arm64MOV_W(scratch, opReg))
	}
	mask := arm64RotateMask(size)
	if mask != 31 {
		emitLoadImm32(cb, 1, mask)
		cb.Emit32(arm64AND_W(scratch, scratch, 1))
	}
}

// emitRotate handles ROL/ROR. ROL is implemented as ROR by the negated count
// for 32/64-bit operands; byte/word use explicit shifts so the width is exact.
func emitRotate(cb *CodeBuffer, ji *JITInstr, left bool) {
	if ji.rd == 0 {
		return
	}

	emitRotateCount(cb, ji, 0, ji.size)

	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	rsReg := resolveReg(cb, ji.rs, 3)
	if ji.size == IE64_SIZE_Q {
		cb.Emit32(arm64MOV(dstReg, rsReg))
	} else {
		cb.Emit32(arm64MOV_W(dstReg, rsReg))
		if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
			emitSizeMask(cb, dstReg, ji.size)
		}
	}

	switch ji.size {
	case IE64_SIZE_B, IE64_SIZE_W:
		width := uint32(8)
		if ji.size == IE64_SIZE_W {
			width = 16
		}
		if left {
			cb.Emit32(arm64MOV_W(1, dstReg))
			cb.Emit32(arm64LSL_W(1, 1, 0))
			emitLoadImm32(cb, 3, width)
			cb.Emit32(arm64SUB_W(3, 3, 0))
			cb.Emit32(arm64LSR_W(dstReg, dstReg, 3))
		} else {
			cb.Emit32(arm64MOV_W(1, dstReg))
			cb.Emit32(arm64LSR_W(1, 1, 0))
			emitLoadImm32(cb, 3, width)
			cb.Emit32(arm64SUB_W(3, 3, 0))
			cb.Emit32(arm64LSL_W(dstReg, dstReg, 3))
		}
		cb.Emit32(arm64ORR_W(dstReg, dstReg, 1))
		emitSizeMask(cb, dstReg, ji.size)
	case IE64_SIZE_L:
		if left {
			cb.Emit32(arm64SUB_W(0, 31, 0))
		}
		cb.Emit32(arm64ROR_W(dstReg, dstReg, 0))
	case IE64_SIZE_Q:
		if left {
			cb.Emit32(arm64SUB(0, 31, 0))
		}
		cb.Emit32(arm64ROR(dstReg, dstReg, 0))
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitCTZ handles CTZ rd, rs over the low 32 bits.
func emitCTZ(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	cb.Emit32(arm64RBIT_W(dstReg, rsReg))
	cb.Emit32(arm64CLZ_W(dstReg, dstReg))

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitPOPCNT handles POPCNT rd, rs over the low 32 bits.
func emitPOPCNT(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	cb.Emit32(arm64MOV_W(dstReg, rsReg))

	cb.Emit32(arm64MOV_W(0, dstReg))
	cb.Emit32(arm64LSR_W_imm(0, 0, 1))
	emitLoadImm32(cb, 1, 0x55555555)
	cb.Emit32(arm64AND_W(0, 0, 1))
	cb.Emit32(arm64SUB_W(dstReg, dstReg, 0))

	cb.Emit32(arm64MOV_W(0, dstReg))
	emitLoadImm32(cb, 1, 0x33333333)
	cb.Emit32(arm64AND_W(dstReg, dstReg, 1))
	cb.Emit32(arm64LSR_W_imm(0, 0, 2))
	cb.Emit32(arm64AND_W(0, 0, 1))
	cb.Emit32(arm64ADD_W(dstReg, dstReg, 0))

	cb.Emit32(arm64MOV_W(0, dstReg))
	cb.Emit32(arm64LSR_W_imm(0, 0, 4))
	cb.Emit32(arm64ADD_W(dstReg, dstReg, 0))
	emitLoadImm32(cb, 1, 0x0F0F0F0F)
	cb.Emit32(arm64AND_W(dstReg, dstReg, 1))

	emitLoadImm32(cb, 1, 0x01010101)
	cb.Emit32(arm64MUL_W(dstReg, dstReg, 1))
	cb.Emit32(arm64LSR_W_imm(dstReg, dstReg, 24))

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitBSWAP handles BSWAP rd, rs over the low 32 bits.
func emitBSWAP(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveReg(cb, ji.rs, 0)
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	cb.Emit32(arm64REV_W(dstReg, rsReg))

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// ===========================================================================
// Memory Access
// ===========================================================================

func emitIE64LoopPrecheckARM64(cb *CodeBuffer, br *blockRegs, writtenSoFar uint32, plan *ie64LoopPlan) {
	var fail []int
	// A cached block compiled with the MMU off may be entered after the MMU
	// changes. Route that execution through the precheck fallback before any
	// direct, untranslated access.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuFail := cb.Len()
	cb.Emit32(0)
	for _, access := range plan.accesses {
		rs := resolveReg(cb, access.base, 0)
		emitLoadImm32(cb, 1, uint32(access.disp))
		cb.Emit32(arm64SXTW(1, 1))
		cb.Emit32(arm64ADD(0, rs, 1))
		cb.Emit32(arm64LDR_imm(1, 31, 96/8))
		cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4)))
		if access.width > 1 {
			cb.Emit32(arm64SUB_imm(1, 1, access.width-1))
		}
		cb.Emit32(arm64CMP(0, 1))
		fail = append(fail, cb.Len())
		cb.Emit32(0)
		cb.Emit32(arm64CMP(0, arm64RegIOStart))
		low := cb.Len()
		cb.Emit32(0)
		cb.Emit32(arm64LSR_imm(1, 0, 8))
		cb.Emit32(arm64LDRB_reg(1, arm64RegIOBitmap, 1))
		nonIO := cb.Len()
		cb.Emit32(0)
		fail = append(fail, cb.Len())
		cb.Emit32(0)
		next := cb.Len()
		cb.PatchUint32(low, arm64Bcond(arm64CondLO, int32(next-low)))
		cb.PatchUint32(nonIO, arm64CBZ(1, int32(next-nonIO)))
	}
	success := cb.Len()
	cb.Emit32(0)
	failPC := cb.Len()
	cb.PatchUint32(mmuFail, arm64CBNZ(1, int32(failPC-mmuFail)))
	for i, off := range fail {
		if i%2 == 0 {
			cb.PatchUint32(off, arm64Bcond(arm64CondHS, int32(failPC-off)))
		} else {
			cb.PatchUint32(off, arm64B(int32(failPC-off)))
		}
	}
	cb.Emit32(arm64LDR_imm(0, 31, 96/8))
	emitLoadImm32(cb, 1, jitFallbackLoopPrecheck)
	cb.Emit32(arm64STR_W_imm(1, 0, uint32(jitCtxOffNeedIOFallback/4)))
	emitPackedPCAndCount(cb, plan.headPC, plan.prefix, br)
	emitEpilogue(cb, writtenSoFar, br.used)
	cb.PatchUint32(success, arm64B(int32(cb.Len()-success)))
}

// emitLOAD handles LOAD rd, disp(rs)
func emitLOAD(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	if ji.rd == 0 {
		return
	}

	// Compute address: int64(rs) + int64(int32(imm32)). Full 64-bit
	// effective address — high bits preserved so the 64-bit CMP with
	// IO_REGION_START routes >4 GiB accesses to the slow path.
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32) // W1 = imm32
	cb.Emit32(arm64SXTW(1, 1))     // X1 = sign-extend
	cb.Emit32(arm64ADD(0, rsReg, 1))
	if ie64CurrentAccessHoisted() {
		dst, mapped := ie64ToARM64Reg(ji.rd)
		if !mapped {
			dst = 2
		}
		switch ji.size {
		case IE64_SIZE_B:
			cb.Emit32(arm64LDRB_reg(dst, arm64RegMemBase, 0))
		case IE64_SIZE_W:
			cb.Emit32(arm64LDRH_reg(dst, arm64RegMemBase, 0))
		case IE64_SIZE_L:
			cb.Emit32(arm64LDR_W_reg(dst, arm64RegMemBase, 0))
		default:
			cb.Emit32(arm64LDR_reg(dst, arm64RegMemBase, 0))
		}
		if !mapped {
			emitStoreSpilledReg(cb, dst, ji.rd)
		}
		return
	}

	// Phase 5 cycle 5.4: MMU-on check. ctx.MMUEnabled is refreshed by
	// the Go dispatcher before every callNative; any non-zero value
	// means virtual addresses must be translated by the interpreter
	// helper. Branch to helper exit.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))                           // X1 = ctx ptr (SP+96)
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4))) // W1 = MMUEnabled (zero-extends)
	mmuHelperOff := cb.Len()
	cb.Emit32(0) // placeholder CBNZ X1, helperLabel

	// R0 is hardwired zero, so a non-negative displacement whose complete
	// access lies below IO_REGION_START is immutable low RAM. ARM64 has no
	// native MMU micro-TLB path: MMU-on execution always takes the helper above.
	// The MMU-off fallthrough may therefore omit the I/O comparison, window
	// bound and I/O-page bitmap probe entirely. Keep the helper after the hot
	// direct load so the only hot-path branch skips a cold bailout and resume
	// entry.
	if _, ok := ie64ConstLowRAMAccess(ji.rs, ji.imm32, ji.size); ok {
		dstReg, mapped := ie64ToARM64Reg(ji.rd)
		if !mapped {
			dstReg = 2
		}
		switch ji.size {
		case IE64_SIZE_B:
			cb.Emit32(arm64LDRB_reg(dstReg, arm64RegMemBase, 0))
		case IE64_SIZE_W:
			cb.Emit32(arm64LDRH_reg(dstReg, arm64RegMemBase, 0))
		case IE64_SIZE_L:
			cb.Emit32(arm64LDR_W_reg(dstReg, arm64RegMemBase, 0))
		case IE64_SIZE_Q:
			cb.Emit32(arm64LDR_reg(dstReg, arm64RegMemBase, 0))
		}
		if !mapped {
			emitStoreSpilledReg(cb, dstReg, ji.rd)
		}
		doneOff := cb.Len()
		cb.Emit32(0)

		helperPC := cb.Len()
		cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
		resumePatch := emitLOADHelperExitARM64(cb, ji, instrPC, br, writtenSoFar)
		resumeOff := cb.Len()
		patchResumeADRARM64(cb, resumePatch, resumeOff)
		emitResumeEntryARM64(cb, instrPC+IE64_INSTR_SIZE, br)

		donePC := cb.Len()
		cb.PatchUint32(doneOff, arm64B(int32(donePC-doneOff)))
		return
	}

	// Compare with IO_REGION_START (64-bit CMP).
	cb.Emit32(arm64CMP(0, arm64RegIOStart))

	// Branch to slow path if addr >= IO_REGION_START.
	slowPathOffset := cb.Len()
	cb.Emit32(0) // placeholder for B.HS

	// Fast path: addr < IO_REGION_START → direct memory load.
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	switch ji.size {
	case IE64_SIZE_B:
		cb.Emit32(arm64LDRB_reg(dstReg, arm64RegMemBase, 0))
	case IE64_SIZE_W:
		cb.Emit32(arm64LDRH_reg(dstReg, arm64RegMemBase, 0))
	case IE64_SIZE_L:
		cb.Emit32(arm64LDR_W_reg(dstReg, arm64RegMemBase, 0))
	case IE64_SIZE_Q:
		cb.Emit32(arm64LDR_reg(dstReg, arm64RegMemBase, 0))
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}

	// Branch over slow path / helper to converged done.
	doneOff1 := cb.Len()
	cb.Emit32(0) // placeholder B done

	// Slow path: addr >= IO_REGION_START
	slowPathPC := cb.Len()
	cb.PatchUint32(slowPathOffset, arm64Bcond(arm64CondHS, int32(slowPathPC-slowPathOffset)))

	// Size-aware 64-bit high-addr check → helper exit (was NeedIOFallback).
	accessBytes := ie64AccessBytes(ji.size)
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))                        // X1 = ctx ptr
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4))) // W1 = MemSize (zero-extends)
	if accessBytes > 1 {
		cb.Emit32(arm64SUB_imm(1, 1, accessBytes-1)) // X1 = MemSize - (accessBytes-1)
	}
	cb.Emit32(arm64CMP(0, 1)) // CMP X0, X1
	inRangeOff := cb.Len()
	cb.Emit32(0) // placeholder B.LO in_range
	highHelperOff := cb.Len()
	cb.Emit32(0) // placeholder B helperLabel

	inRangePC := cb.Len()
	cb.PatchUint32(inRangeOff, arm64Bcond(arm64CondLO, int32(inRangePC-inRangeOff)))

	// Check ioPageBitmap[addr >> 8].
	cb.Emit32(arm64LSR_imm(1, 0, 8))                 // X1 = addr >> 8
	cb.Emit32(arm64LDRB_reg(1, arm64RegIOBitmap, 1)) // W1 = ioPageBitmap[page]
	cb.Emit32(arm64CBZ(1, 0))                        // placeholder CBZ → non-I/O direct load
	nonIOOffset := cb.Len() - 4

	// I/O page → helper exit.
	ioHelperOff := cb.Len()
	cb.Emit32(0) // placeholder B helperLabel

	// Non-I/O page (e.g. VRAM) → direct memory access.
	nonIOPC := cb.Len()
	cb.PatchUint32(nonIOOffset, arm64CBZ(1, int32(nonIOPC-nonIOOffset)))

	switch ji.size {
	case IE64_SIZE_B:
		cb.Emit32(arm64LDRB_reg(dstReg, arm64RegMemBase, 0))
	case IE64_SIZE_W:
		cb.Emit32(arm64LDRH_reg(dstReg, arm64RegMemBase, 0))
	case IE64_SIZE_L:
		cb.Emit32(arm64LDR_W_reg(dstReg, arm64RegMemBase, 0))
	case IE64_SIZE_Q:
		cb.Emit32(arm64LDR_reg(dstReg, arm64RegMemBase, 0))
	}

	if !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
	doneOff2 := cb.Len()
	cb.Emit32(0) // placeholder B done

	// Helper exit label. All three bail paths (MMU on, high addr,
	// I/O page) converge here. emitLOADHelperExitARM64 ends with
	// emitEpilogue → RET, so no fallthrough into done.
	helperPC := cb.Len()
	cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
	cb.PatchUint32(highHelperOff, arm64B(int32(helperPC-highHelperOff)))
	cb.PatchUint32(ioHelperOff, arm64B(int32(helperPC-ioHelperOff)))
	resumePatch := emitLOADHelperExitARM64(cb, ji, instrPC, br, writtenSoFar)
	resumeOff := cb.Len()
	patchResumeADRARM64(cb, resumePatch, resumeOff)
	emitResumeEntryARM64(cb, instrPC+IE64_INSTR_SIZE, br)

	// Converged done label for both successful direct loads.
	donePC := cb.Len()
	cb.PatchUint32(doneOff1, arm64B(int32(donePC-doneOff1)))
	cb.PatchUint32(doneOff2, arm64B(int32(donePC-doneOff2)))
}

// emitLOADHelperExitARM64 writes the JITContext HELPER_LOAD protocol
// fields and exits the block. The Go-side dispatcher (handleJITHelper)
// reads the request, calls cpu.loadMem(HelperAddr, HelperSize), writes
// the result into Rd, advances PC, and re-enters the JIT loop.
//
// X0 must hold the effective virtual address on entry.
func emitLOADHelperExitARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) int {
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))                                  // X1 = ctx ptr (SP+96)
	cb.Emit32(arm64STR_imm(0, 1, uint32(jitCtxOffHelperAddr/8)))          // HelperAddr = X0
	emitLoadImm32(cb, 2, uint32(ji.size))                                 //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffHelperSize/4)))        // HelperSize
	emitLoadImm32(cb, 2, uint32(ji.rd))                                   //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffHelperRd/4)))          // HelperRd
	cb.Emit32(arm64STR_imm(arm64RegIE64SP, 1, uint32(jitCtxOffLiveSP/8))) // LiveSP = X27
	emitLoadImm64(cb, 2, instrPC)                                         //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperPC/8)))            // HelperPC
	emitLoadImm32(cb, 2, HELPER_LOAD)                                     //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffNeedHelper/4)))        // NeedHelper

	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	resumePatch := emitHelperResumeFieldsARM64(cb, 1, instrPC+IE64_INSTR_SIZE, cb.instrCountBase+bailCount+1, br)
	emitEpilogue(cb, writtenSoFar, br.used)
	return resumePatch
}

// emitHighAddrBailCheckARM64 emits a size-aware 64-bit bail check at the
// top of an IE64 LOAD/STORE slow path: if addr > MemSize - accessBytes → bail.
// This catches both addresses with high bits set and accesses near MemSize
// whose end byte would escape the low window. accessBytes is 1/2/4/8.
//
// X0 holds the full 64-bit effective address; X1 is scratch.
func emitHighAddrBailCheckARM64(cb *CodeBuffer, instrPC uint64, pcOffset uint32, br *blockRegs, writtenSoFar uint32, accessBytes uint32) {
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))                        // X1 = JITContext ptr (from [SP, #96])
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4))) // W1 = ctx.MemSize (zero-extends to X1)
	if accessBytes > 1 {
		// X1 = MemSize - (accessBytes - 1) so that addr < X1 means
		// addr + accessBytes <= MemSize. MemSize >> accessBytes, no
		// underflow risk. accessBytes-1 ∈ {1,3,7}, all imm12-encodable.
		cb.Emit32(arm64SUB_imm(1, 1, accessBytes-1))
	}
	cb.Emit32(arm64CMP(0, 1)) // CMP X0, X1 (64-bit)
	inRangeOff := cb.Len()
	cb.Emit32(0) // placeholder for B.LO in_range (addr < X1 → in range)
	// bail body
	cb.Emit32(arm64LDR_imm(0, 31, 96/8))                               // X0 = JITContext ptr
	emitLoadImm32(cb, 1, 1)                                            // W1 = 1
	cb.Emit32(arm64STR_W_imm(1, 0, uint32(jitCtxOffNeedIOFallback/4))) // NeedIOFallback = 1
	bailCount := uint32(pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
	inRangePC := cb.Len()
	cb.PatchUint32(inRangeOff, arm64Bcond(arm64CondLO, int32(inRangePC-inRangeOff)))
}

// emitSTORE handles STORE rd, disp(rs)
func emitSTORE(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	// Compute address: int64(rs) + int64(int32(imm32)).
	// Full 64-bit effective address; downstream 64-bit CMPs route high
	// addresses to the slow path.
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(0, rsReg, 1))

	// Load source value (rd for STORE)
	srcReg := resolveReg(cb, ji.rd, 3) // X3 scratch

	// Apply size mask to value before store
	if ji.size != IE64_SIZE_Q {
		cb.Emit32(arm64MOV(4, srcReg)) // X4 = src
		emitSizeMask(cb, 4, ji.size)
		srcReg = 4
	}
	if ie64CurrentAccessHoisted() {
		switch ji.size {
		case IE64_SIZE_B:
			cb.Emit32(arm64STRB_reg(srcReg, arm64RegMemBase, 0))
		case IE64_SIZE_W:
			cb.Emit32(arm64STRH_reg(srcReg, arm64RegMemBase, 0))
		case IE64_SIZE_L:
			cb.Emit32(arm64STR_W_reg(srcReg, arm64RegMemBase, 0))
		default:
			cb.Emit32(arm64STR_reg(srcReg, arm64RegMemBase, 0))
		}
		return
	}

	// Phase 5 cycle 5.5: MMU-on check → helper exit.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuHelperOff := cb.Len()
	cb.Emit32(0) // CBNZ X1, helperLabel

	// The shared constant-address proof covers the complete access and low RAM
	// is always inside the direct window. With the MMU-on case already routed
	// to the helper, the MMU-off hot path can store directly without I/O,
	// bounds or bitmap checks. The helper and resume entry remain cold and the
	// direct store branches around them.
	if _, ok := ie64ConstLowRAMAccess(ji.rs, ji.imm32, ji.size); ok {
		switch ji.size {
		case IE64_SIZE_B:
			cb.Emit32(arm64STRB_reg(srcReg, arm64RegMemBase, 0))
		case IE64_SIZE_W:
			cb.Emit32(arm64STRH_reg(srcReg, arm64RegMemBase, 0))
		case IE64_SIZE_L:
			cb.Emit32(arm64STR_W_reg(srcReg, arm64RegMemBase, 0))
		case IE64_SIZE_Q:
			cb.Emit32(arm64STR_reg(srcReg, arm64RegMemBase, 0))
		}
		doneOff := cb.Len()
		cb.Emit32(0)

		helperPC := cb.Len()
		cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
		resumePatch := emitSTOREHelperExitARM64(cb, ji, instrPC, srcReg, br, writtenSoFar)
		resumeOff := cb.Len()
		patchResumeADRARM64(cb, resumePatch, resumeOff)
		emitResumeEntryARM64(cb, instrPC+IE64_INSTR_SIZE, br)

		donePC := cb.Len()
		cb.PatchUint32(doneOff, arm64B(int32(donePC-doneOff)))
		return
	}

	// Compare address with IO_REGION_START
	cb.Emit32(arm64CMP(0, arm64RegIOStart))

	// Branch to slow path if addr >= IO_REGION_START
	slowPathOffset := cb.Len()
	cb.Emit32(0) // placeholder for B.HS

	// Fast path: addr < IO_REGION_START → direct memory store
	switch ji.size {
	case IE64_SIZE_B:
		cb.Emit32(arm64STRB_reg(srcReg, arm64RegMemBase, 0))
	case IE64_SIZE_W:
		cb.Emit32(arm64STRH_reg(srcReg, arm64RegMemBase, 0))
	case IE64_SIZE_L:
		cb.Emit32(arm64STR_W_reg(srcReg, arm64RegMemBase, 0))
	case IE64_SIZE_Q:
		cb.Emit32(arm64STR_reg(srcReg, arm64RegMemBase, 0))
	}

	// Branch over slow path / helper to converged done.
	doneOff1 := cb.Len()
	cb.Emit32(0) // placeholder B done

	// Slow path: addr >= IO_REGION_START
	slowPathPC := cb.Len()
	cb.PatchUint32(slowPathOffset, arm64Bcond(arm64CondHS, int32(slowPathPC-slowPathOffset)))

	// Size-aware high-addr check → helper exit (was NeedIOFallback).
	accessBytes := ie64AccessBytes(ji.size)
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4)))
	if accessBytes > 1 {
		cb.Emit32(arm64SUB_imm(1, 1, accessBytes-1))
	}
	cb.Emit32(arm64CMP(0, 1))
	inRangeOff := cb.Len()
	cb.Emit32(0) // placeholder B.LO in_range
	highHelperOff := cb.Len()
	cb.Emit32(0) // placeholder B helperLabel

	inRangePC := cb.Len()
	cb.PatchUint32(inRangeOff, arm64Bcond(arm64CondLO, int32(inRangePC-inRangeOff)))

	// Check ioPageBitmap[addr >> 8].
	cb.Emit32(arm64LSR_imm(1, 0, 8))
	cb.Emit32(arm64LDRB_reg(1, arm64RegIOBitmap, 1))
	cb.Emit32(arm64CBZ(1, 0)) // placeholder CBZ → non-I/O direct store
	nonIOOffset := cb.Len() - 4

	// I/O page → helper exit.
	ioHelperOff := cb.Len()
	cb.Emit32(0) // placeholder B helperLabel

	// Non-I/O page (e.g. VRAM) → direct memory store
	nonIOPC := cb.Len()
	cb.PatchUint32(nonIOOffset, arm64CBZ(1, int32(nonIOPC-nonIOOffset)))

	switch ji.size {
	case IE64_SIZE_B:
		cb.Emit32(arm64STRB_reg(srcReg, arm64RegMemBase, 0))
	case IE64_SIZE_W:
		cb.Emit32(arm64STRH_reg(srcReg, arm64RegMemBase, 0))
	case IE64_SIZE_L:
		cb.Emit32(arm64STR_W_reg(srcReg, arm64RegMemBase, 0))
	case IE64_SIZE_Q:
		cb.Emit32(arm64STR_reg(srcReg, arm64RegMemBase, 0))
	}
	doneOff2 := cb.Len()
	cb.Emit32(0) // placeholder B done

	// Helper exit label.
	helperPC := cb.Len()
	cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
	cb.PatchUint32(highHelperOff, arm64B(int32(helperPC-highHelperOff)))
	cb.PatchUint32(ioHelperOff, arm64B(int32(helperPC-ioHelperOff)))
	resumePatch := emitSTOREHelperExitARM64(cb, ji, instrPC, srcReg, br, writtenSoFar)
	resumeOff := cb.Len()
	patchResumeADRARM64(cb, resumePatch, resumeOff)
	emitResumeEntryARM64(cb, instrPC+IE64_INSTR_SIZE, br)

	// Converged done label.
	donePC := cb.Len()
	cb.PatchUint32(doneOff1, arm64B(int32(donePC-doneOff1)))
	cb.PatchUint32(doneOff2, arm64B(int32(donePC-doneOff2)))
}

// emitSTOREHelperExitARM64 writes the JITContext HELPER_STORE protocol
// fields and exits the block. X0 = effective virtual address, srcReg =
// value to store (already size-masked).
func emitSTOREHelperExitARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, srcReg byte, br *blockRegs, writtenSoFar uint32) int {
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))                                  // X1 = ctx ptr
	cb.Emit32(arm64STR_imm(0, 1, uint32(jitCtxOffHelperAddr/8)))          // HelperAddr = X0
	cb.Emit32(arm64STR_imm(srcReg, 1, uint32(jitCtxOffHelperVal/8)))      // HelperVal
	emitLoadImm32(cb, 2, uint32(ji.size))                                 //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffHelperSize/4)))        // HelperSize
	emitLoadImm32(cb, 2, uint32(ji.rd))                                   //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffHelperRd/4)))          // HelperRd
	cb.Emit32(arm64STR_imm(arm64RegIE64SP, 1, uint32(jitCtxOffLiveSP/8))) // LiveSP = X27
	emitLoadImm64(cb, 2, instrPC)                                         //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperPC/8)))            // HelperPC
	emitLoadImm32(cb, 2, HELPER_STORE)                                    //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffNeedHelper/4)))        // NeedHelper

	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	resumePatch := emitHelperResumeFieldsARM64(cb, 1, instrPC+IE64_INSTR_SIZE, cb.instrCountBase+bailCount+1, br)
	emitEpilogue(cb, writtenSoFar, br.used)
	return resumePatch
}

func emitStoreAtomicOld(cb *CodeBuffer, oldReg byte, rd byte) {
	if rd == 0 {
		return
	}
	dstReg, mapped := ie64ToARM64Reg(rd)
	if mapped {
		cb.Emit32(arm64MOV(dstReg, oldReg))
	} else {
		emitStoreSpilledReg(cb, oldReg, rd)
	}
}

func emitAtomic(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(0, rsReg, 1)) // X0 = effective address

	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuOff := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64AND_imm(1, 0, 0, 2, 1)) // X1 = addr & 7
	alignOff := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64CMP(0, arm64RegIOStart))
	ioOff := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4)))
	cb.Emit32(arm64SUB_imm(1, 1, 7))
	cb.Emit32(arm64CMP(0, 1))
	highOff := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64ADD(0, arm64RegMemBase, 0)) // X0 = host pointer
	rtReg := resolveReg(cb, ji.rt, 3)
	if rtReg != 3 {
		cb.Emit32(arm64MOV(3, rtReg))
	}

	switch ji.opcode {
	case OP_CAS:
		rdReg := resolveReg(cb, ji.rd, 4)
		if rdReg != 4 {
			cb.Emit32(arm64MOV(4, rdReg))
		}
		loopPC := cb.Len()
		cb.Emit32(arm64LDAXR(2, 0))
		cb.Emit32(arm64CMP(2, 4))
		failOff := cb.Len()
		cb.Emit32(0)
		cb.Emit32(arm64STLXR(5, 3, 0))
		retryOff := cb.Len()
		cb.Emit32(0)
		doneCasOff := cb.Len()
		cb.Emit32(0)
		failPC := cb.Len()
		cb.PatchUint32(failOff, arm64Bcond(arm64CondNE, int32(failPC-failOff)))
		cb.Emit32(arm64CLREX())
		doneCasPC := cb.Len()
		cb.PatchUint32(doneCasOff, arm64B(int32(doneCasPC-doneCasOff)))
		cb.PatchUint32(retryOff, arm64CBNZ(5, int32(loopPC-retryOff)))
		emitStoreAtomicOld(cb, 2, ji.rd)
	case OP_XCHG:
		loopPC := cb.Len()
		cb.Emit32(arm64LDAXR(2, 0))
		cb.Emit32(arm64STLXR(5, 3, 0))
		retryOff := cb.Len()
		cb.Emit32(0)
		cb.PatchUint32(retryOff, arm64CBNZ(5, int32(loopPC-retryOff)))
		emitStoreAtomicOld(cb, 2, ji.rd)
	case OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
		loopPC := cb.Len()
		cb.Emit32(arm64LDAXR(2, 0))
		switch ji.opcode {
		case OP_FAA:
			cb.Emit32(arm64ADD(4, 2, 3))
		case OP_FAND:
			cb.Emit32(arm64AND(4, 2, 3))
		case OP_FOR:
			cb.Emit32(arm64ORR(4, 2, 3))
		case OP_FXOR:
			cb.Emit32(arm64EOR(4, 2, 3))
		}
		cb.Emit32(arm64STLXR(5, 4, 0))
		retryOff := cb.Len()
		cb.Emit32(0)
		cb.PatchUint32(retryOff, arm64CBNZ(5, int32(loopPC-retryOff)))
		emitStoreAtomicOld(cb, 2, ji.rd)
	}
	emitPackedPCAndCount(cb, instrPC+IE64_INSTR_SIZE, ji.pcOffset/IE64_INSTR_SIZE+1, br)
	emitEpilogue(cb, writtenSoFar|instrWrittenRegs(ji), br.used)

	bailPC := cb.Len()
	cb.PatchUint32(mmuOff, arm64CBNZ(1, int32(bailPC-mmuOff)))
	cb.PatchUint32(alignOff, arm64CBNZ(1, int32(bailPC-alignOff)))
	cb.PatchUint32(ioOff, arm64Bcond(arm64CondHS, int32(bailPC-ioOff)))
	cb.PatchUint32(highOff, arm64Bcond(arm64CondHS, int32(bailPC-highOff)))
	emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
}

// ===========================================================================
// Control Flow
// ===========================================================================

// emitBRA handles BRA (unconditional branch)
func emitBRA(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, instrIdx int, instrOffsets []int, blockStartPC uint64) {
	targetPC := uint64(int64(instrPC) + int64(int32(ji.imm32)))
	staticCount := uint32(instrIdx + 1)

	// Check for backward branch within block
	if br.hasBackwardBranch && targetPC >= blockStartPC && targetPC < instrPC &&
		(targetPC-blockStartPC)%IE64_INSTR_SIZE == 0 {
		targetIdx := int((targetPC - blockStartPC) / IE64_INSTR_SIZE)
		if targetIdx >= 0 && targetIdx < instrIdx && targetIdx < len(instrOffsets) {
			bodySize := uint32(instrIdx - targetIdx + 1)

			// ADD X7, X7, #bodySize (tentatively count re-execution)
			cb.Emit32(arm64ADD_imm(arm64RegLoopCount, arm64RegLoopCount, bodySize))
			// CMP X7, #jitBudget
			cb.Emit32(arm64CMP_imm(arm64RegLoopCount, jitBudget))
			// B.HS budget_exit
			budgetExitOffset := cb.Len()
			cb.Emit32(0) // placeholder
			// B backward to target
			targetARM64Offset := instrOffsets[targetIdx]
			cb.Emit32(arm64B(int32(targetARM64Offset - cb.Len())))

			// budget_exit:
			budgetExitPC := cb.Len()
			cb.PatchUint32(budgetExitOffset, arm64Bcond(arm64CondHS, int32(budgetExitPC-budgetExitOffset)))
			// SUB X7, X7, #bodySize (rollback)
			cb.Emit32(arm64SUB_imm(arm64RegLoopCount, arm64RegLoopCount, bodySize))
			emitPackedPCAndCount(cb, targetPC, staticCount, br)
			emitEpilogue(cb, br.written, br.used)
			return
		}
	}

	// Forward/external branch
	emitPackedPCAndCount(cb, targetPC, staticCount, br)
	emitEpilogue(cb, br.written, br.used)
}

// emitBcc handles conditional branches (BEQ, BNE, BLT, BGE, BGT, BLE, BHI, BLS).
// Conditional branches do NOT terminate blocks — the not-taken path falls through
// to the next instruction. Only the taken path exits the block.
//
// Three modes:
// 1. Backward branch in backward-branch block: native loop with budget
// 2. Forward exit in backward-branch block: dynamic count via X7
// 3. Non-backward-branch block: static packed immediate (original path)
func emitBcc(cb *CodeBuffer, ji *JITInstr, instrPC uint64, cond byte, br *blockRegs, writtenSoFar uint32, blockStartPC uint64, instrIdx int, instrOffsets []int) {
	targetPC := uint64(int64(instrPC) + int64(int32(ji.imm32)))
	staticCount := uint32(instrIdx + 1)

	rsReg := resolveReg(cb, ji.rs, 0)
	rtReg := resolveReg(cb, ji.rt, 1)
	cb.Emit32(arm64CMP(rsReg, rtReg))

	// Mode 1: Backward branch within block
	if br.hasBackwardBranch && targetPC >= blockStartPC && targetPC < instrPC &&
		(targetPC-blockStartPC)%IE64_INSTR_SIZE == 0 {
		targetIdx := int((targetPC - blockStartPC) / IE64_INSTR_SIZE)
		if targetIdx >= 0 && targetIdx < instrIdx && targetIdx < len(instrOffsets) {
			// B.!cond skip (not-taken → fall through)
			skipOffset := cb.Len()
			cb.Emit32(0) // placeholder

			bodySize := uint32(instrIdx - targetIdx + 1)
			if ie64ActiveLoopPlan != nil && ie64ActiveLoopPlan.bounded && ie64ActiveLoopPlan.back == instrIdx {
				cb.Emit32(arm64ADD_imm(arm64RegLoopCount, arm64RegLoopCount, bodySize))
				cb.Emit32(arm64B(int32(instrOffsets[targetIdx] - cb.Len())))
				cb.PatchUint32(skipOffset, arm64Bcond(cond^1, int32(cb.Len()-skipOffset)))
				return
			}
			// ADD X7, X7, #bodySize (tentatively count re-execution)
			cb.Emit32(arm64ADD_imm(arm64RegLoopCount, arm64RegLoopCount, bodySize))
			// CMP X7, #jitBudget
			cb.Emit32(arm64CMP_imm(arm64RegLoopCount, jitBudget))
			// B.HS budget_exit
			budgetExitOffset := cb.Len()
			cb.Emit32(0) // placeholder
			// B backward to target (native loop)
			targetARM64Offset := instrOffsets[targetIdx]
			cb.Emit32(arm64B(int32(targetARM64Offset - cb.Len())))

			// budget_exit:
			budgetExitPC := cb.Len()
			cb.PatchUint32(budgetExitOffset, arm64Bcond(arm64CondHS, int32(budgetExitPC-budgetExitOffset)))
			// SUB X7, X7, #bodySize (rollback — re-execution won't happen)
			cb.Emit32(arm64SUB_imm(arm64RegLoopCount, arm64RegLoopCount, bodySize))
			emitPackedPCAndCount(cb, targetPC, staticCount, br)
			// Use br.written (not writtenSoFar): in a backward branch loop,
			// instructions AFTER this branch may have executed in prior iterations,
			// modifying registers not yet in writtenSoFar at this instruction index.
			emitEpilogue(cb, br.written, br.used)

			// skip: (not-taken fall through)
			skipPC := cb.Len()
			cb.PatchUint32(skipOffset, arm64Bcond(cond^1, int32(skipPC-skipOffset)))
			return
		}
	}

	// Mode 2 & 3: Forward exit (or non-backward-branch block)
	skipOffset := cb.Len()
	cb.Emit32(0) // placeholder for B.NOT_cond

	emitPackedPCAndCount(cb, targetPC, staticCount, br)
	// In a backward-branch block, prior loop iterations may have written
	// registers that appear after this branch — use br.written to capture all.
	exitRegs := writtenSoFar
	if br.hasBackwardBranch {
		exitRegs = br.written
	}
	emitEpilogue(cb, exitRegs, br.used)

	skipPC := cb.Len()
	cb.PatchUint32(skipOffset, arm64Bcond(cond^1, int32(skipPC-skipOffset)))
}

// emitJMP handles JMP rs, disp
func emitJMP(cb *CodeBuffer, ji *JITInstr, br *blockRegs, instrCount uint32) {
	rsReg := resolveReg(cb, ji.rs, 0)

	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(arm64RegIE64PC, rsReg, 1))

	// PLAN_MAX_RAM.md slice 8 phase 8 retired the IE64_ADDR_MASK AND
	// here. The PC widened to 64-bit in slice 3; clamping to 25 bits
	// silently aliased high targets into low memory.

	// Phase 2: count to ctx.RetCount; X28 already carries full 64-bit PC.
	emitStoreRetCount(cb, instrCount, br)

	emitEpilogue(cb, br.written, br.used)
}

// emitJSR handles JSR (jump to subroutine, PC-relative). Phase 5: the
// return-address push routes through HELPER_JSR on MMU-on / high SP;
// HelperVal carries the return address, HelperAddr the call target.
func emitJSR(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	// Phase 3: keep the full 64-bit PC-relative target. The legacy
	// uint32(...) cast aliased a high (>4 GiB) call target down to its
	// low 32 bits, misdirecting the chain exit while AMD64 JSR and the
	// other ARM64 branch paths already preserve the full PC.
	targetPC := uint64(int64(instrPC) + int64(int32(ji.imm32)))
	staticCount := uint32(ji.pcOffset/IE64_INSTR_SIZE + 1)

	// MMU-on → helper.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuHelperOff := cb.Len()
	cb.Emit32(0) // CBNZ X1, helper

	// Underflow guard: SP >= 8 (the pre-decrement must not wrap).
	cb.Emit32(arm64CMP_imm(arm64RegIE64SP, 8))
	underflowOff := cb.Len()
	cb.Emit32(0) // B.LO helper

	// High-slot guard: SP-8 <= MemSize-8 ⇔ SP <= MemSize.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4)))
	cb.Emit32(arm64CMP(arm64RegIE64SP, 1))
	highHelperOff := cb.Len()
	cb.Emit32(0) // B.HI helper

	// Fast path: SP -= 8; mem[SP] = retAddr; exit toward target.
	cb.Emit32(arm64SUB_imm(arm64RegIE64SP, arm64RegIE64SP, 8))
	retAddr := uint64(instrPC + IE64_INSTR_SIZE)
	emitLoadImm64(cb, 0, retAddr)
	cb.Emit32(arm64STR_reg(0, arm64RegMemBase, arm64RegIE64SP))
	emitPackedPCAndCount(cb, targetPC, staticCount, br)
	emitEpilogue(cb, br.written, br.used)

	// Helper exit (block exit; reached only via the guard jumps above).
	helperPC := cb.Len()
	cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
	cb.PatchUint32(underflowOff, arm64Bcond(arm64CondLO, int32(helperPC-underflowOff)))
	cb.PatchUint32(highHelperOff, arm64Bcond(arm64CondHI, int32(helperPC-highHelperOff)))
	emitJSRHelperExitARM64(cb, ji, instrPC, targetPC, br, writtenSoFar)
}

// emitJSRHelperExitARM64 writes the JITContext HELPER_JSR protocol fields.
func emitJSRHelperExitARM64(cb *CodeBuffer, ji *JITInstr, instrPC, targetPC uint64, br *blockRegs, writtenSoFar uint32) {
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))                                  // X1 = ctx ptr
	emitLoadImm64(cb, 2, instrPC+IE64_INSTR_SIZE)                         //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperVal/8)))           // HelperVal = retAddr
	emitLoadImm64(cb, 2, targetPC)                                        //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperAddr/8)))          // HelperAddr = target
	cb.Emit32(arm64STR_imm(arm64RegIE64SP, 1, uint32(jitCtxOffLiveSP/8))) // LiveSP = X27
	emitLoadImm64(cb, 2, instrPC)                                         //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperPC/8)))            // HelperPC
	emitLoadImm32(cb, 2, HELPER_JSR)                                      //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffNeedHelper/4)))        // NeedHelper

	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
}

// emitRTS handles RTS (return from subroutine). Phase 5: routes the stack
// read through HELPER_RTS on MMU-on / high SP; otherwise pops directly.
func emitRTS(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, instrCount uint32, writtenSoFar uint32) {
	// MMU-on → helper.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuHelperOff := cb.Len()
	cb.Emit32(0) // CBNZ X1, helper

	// High-slot guard: read at SP needs SP <= MemSize-8.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4)))
	cb.Emit32(arm64SUB_imm(1, 1, 8)) // X1 = MemSize - 8
	cb.Emit32(arm64CMP(arm64RegIE64SP, 1))
	highHelperOff := cb.Len()
	cb.Emit32(0) // B.HI helper

	// Fast path: PC = mem[SP]; SP += 8.
	cb.Emit32(arm64LDR_reg(arm64RegIE64PC, arm64RegMemBase, arm64RegIE64SP))
	cb.Emit32(arm64ADD_imm(arm64RegIE64SP, arm64RegIE64SP, 8))
	emitStoreRetCount(cb, instrCount, br)
	emitEpilogue(cb, br.written, br.used)

	// Helper exit (block exit; reached only via the guard jumps above).
	helperPC := cb.Len()
	cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
	cb.PatchUint32(highHelperOff, arm64Bcond(arm64CondHI, int32(helperPC-highHelperOff)))
	emitRTSHelperExitARM64(cb, ji, instrPC, br, writtenSoFar)
}

// emitRTSHelperExitARM64 writes the JITContext HELPER_RTS protocol fields.
func emitRTSHelperExitARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))                                  // X1 = ctx ptr
	cb.Emit32(arm64STR_imm(arm64RegIE64SP, 1, uint32(jitCtxOffLiveSP/8))) // LiveSP = X27
	emitLoadImm64(cb, 2, instrPC)                                         //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperPC/8)))            // HelperPC
	emitLoadImm32(cb, 2, HELPER_RTS)                                      //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffNeedHelper/4)))        // NeedHelper

	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
}

// emitRTI handles RTI (return from interrupt) by bailing to the interpreter.
// RTI modifies PC (pops from stack) and clears the inInterrupt atomic flag,
// both of which require Go runtime interaction. We bail to the interpreter
// to handle it, storing all registers written by prior instructions.
func emitRTI(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	// Same bail pattern as emitFPUBail: set NeedIOFallback, store PC, epilogue
	cb.Emit32(arm64LDR_imm(0, 31, 96/8)) // X0 = JITContext from stack
	emitLoadImm32(cb, 1, 1)
	cb.Emit32(arm64STR_W_imm(1, 0, uint32(jitCtxOffNeedIOFallback/4)))
	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
}

// emitWAIT handles WAIT (sleep for imm32 microseconds) by bailing to the interpreter.
// WAIT requires time.Sleep from the Go runtime. In step mode the sleep is skipped,
// which is the expected JIT behavior. We bail so the interpreter can handle it.
func emitWAIT(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	cb.Emit32(arm64LDR_imm(0, 31, 96/8)) // X0 = JITContext from stack
	emitLoadImm32(cb, 1, 1)
	cb.Emit32(arm64STR_W_imm(1, 0, uint32(jitCtxOffNeedIOFallback/4)))
	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
}

// emitPUSH handles PUSH rs. Phase 5 cycle 5.8: the stack write goes
// through the HELPER_PUSH protocol when MMU is on or the target slot
// (SP-8) escapes the low window; otherwise it writes directly. The Go
// dispatcher decrements cpu.regs[31] itself, so LiveSP is flushed
// pre-decrement and HelperVal carries the value to push.
func emitPUSH(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	srcReg := resolveReg(cb, ji.rs, 0) // value to push (pre-decrement)

	// MMU-on → helper.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuHelperOff := cb.Len()
	cb.Emit32(0) // CBNZ X1, helper

	// Underflow guard: SP >= 8 (the pre-decrement must not wrap).
	cb.Emit32(arm64CMP_imm(arm64RegIE64SP, 8))
	underflowOff := cb.Len()
	cb.Emit32(0) // B.LO helper (SP < 8)

	// High-slot guard: SP-8 <= MemSize-8 ⇔ SP <= MemSize.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4))) // X1 = MemSize
	cb.Emit32(arm64CMP(arm64RegIE64SP, 1))
	highHelperOff := cb.Len()
	cb.Emit32(0) // B.HI helper (SP > MemSize)

	// Fast path: SP -= 8; mem[SP] = Rs.
	cb.Emit32(arm64SUB_imm(arm64RegIE64SP, arm64RegIE64SP, 8))
	cb.Emit32(arm64STR_reg(srcReg, arm64RegMemBase, arm64RegIE64SP))
	doneOff := cb.Len()
	cb.Emit32(0) // B done

	helperPC := cb.Len()
	cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
	cb.PatchUint32(underflowOff, arm64Bcond(arm64CondLO, int32(helperPC-underflowOff)))
	cb.PatchUint32(highHelperOff, arm64Bcond(arm64CondHI, int32(helperPC-highHelperOff)))
	resumePatch := emitPUSHHelperExitARM64(cb, ji, instrPC, srcReg, br, writtenSoFar)
	resumeOff := cb.Len()
	patchResumeADRARM64(cb, resumePatch, resumeOff)
	emitResumeEntryARM64(cb, instrPC+IE64_INSTR_SIZE, br)

	donePC := cb.Len()
	cb.PatchUint32(doneOff, arm64B(int32(donePC-doneOff)))
}

// emitPUSHHelperExitARM64 writes the JITContext HELPER_PUSH protocol
// fields. srcReg holds the value to push; X1 is the ctx pointer and
// never aliases a mapped IE64 register.
func emitPUSHHelperExitARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, srcReg byte, br *blockRegs, writtenSoFar uint32) int {
	cb.Emit32(arm64LDR_imm(1, 31, 96/8)) // X1 = ctx ptr
	valReg := srcReg
	if ji.rs == 31 {
		// PUSH R31: the interpreter decrements SP before reading R31, so
		// the pushed value is SP-8. The dispatcher decrements cpu.regs[31]
		// but writes HelperVal verbatim, so pre-compute SP-8. (The fast
		// path stores the already-decremented X27, so this only affects
		// the helper exit.) X2 is reused for instrPC below, after
		// HelperVal is written.
		cb.Emit32(arm64SUB_imm(2, arm64RegIE64SP, 8)) // X2 = SP-8
		valReg = 2
	}
	cb.Emit32(arm64STR_imm(valReg, 1, uint32(jitCtxOffHelperVal/8)))      // HelperVal
	cb.Emit32(arm64STR_imm(arm64RegIE64SP, 1, uint32(jitCtxOffLiveSP/8))) // LiveSP = X27
	emitLoadImm64(cb, 2, instrPC)                                         //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperPC/8)))            // HelperPC
	emitLoadImm32(cb, 2, HELPER_PUSH)                                     //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffNeedHelper/4)))        // NeedHelper

	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	resumePatch := emitHelperResumeFieldsARM64(cb, 1, instrPC+IE64_INSTR_SIZE, cb.instrCountBase+bailCount+1, br)
	emitEpilogue(cb, writtenSoFar, br.used)
	return resumePatch
}

// emitPOP handles POP rd. Phase 5 cycle 5.8: routes through HELPER_POP
// when MMU is on or SP escapes the low window; otherwise reads directly
// then SP += 8.
func emitPOP(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	// MMU-on → helper.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuHelperOff := cb.Len()
	cb.Emit32(0) // CBNZ X1, helper

	// High-slot guard: read at SP needs SP <= MemSize-8.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4)))
	cb.Emit32(arm64SUB_imm(1, 1, 8)) // X1 = MemSize - 8
	cb.Emit32(arm64CMP(arm64RegIE64SP, 1))
	highHelperOff := cb.Len()
	cb.Emit32(0) // B.HI helper (SP > MemSize-8)

	// Fast path: Rd = mem[SP]; SP += 8.
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}
	if ji.rd != 0 {
		cb.Emit32(arm64LDR_reg(dstReg, arm64RegMemBase, arm64RegIE64SP))
	}
	cb.Emit32(arm64ADD_imm(arm64RegIE64SP, arm64RegIE64SP, 8))
	if ji.rd != 0 && !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
	doneOff := cb.Len()
	cb.Emit32(0) // B done

	helperPC := cb.Len()
	cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
	cb.PatchUint32(highHelperOff, arm64Bcond(arm64CondHI, int32(helperPC-highHelperOff)))
	resumePatch := emitPOPHelperExitARM64(cb, ji, instrPC, br, writtenSoFar)
	resumeOff := cb.Len()
	patchResumeADRARM64(cb, resumePatch, resumeOff)
	emitResumeEntryARM64(cb, instrPC+IE64_INSTR_SIZE, br)

	donePC := cb.Len()
	cb.PatchUint32(doneOff, arm64B(int32(donePC-doneOff)))
}

// emitPOPHelperExitARM64 writes the JITContext HELPER_POP protocol
// fields. The dispatcher reads via mmuStackRead, writes into HelperRd,
// increments cpu.regs[31], and advances PC.
func emitPOPHelperExitARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) int {
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))                                  // X1 = ctx ptr
	emitLoadImm32(cb, 2, uint32(ji.rd))                                   //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffHelperRd/4)))          // HelperRd
	cb.Emit32(arm64STR_imm(arm64RegIE64SP, 1, uint32(jitCtxOffLiveSP/8))) // LiveSP = X27
	emitLoadImm64(cb, 2, instrPC)                                         //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperPC/8)))            // HelperPC
	emitLoadImm32(cb, 2, HELPER_POP)                                      //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffNeedHelper/4)))        // NeedHelper

	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	resumePatch := emitHelperResumeFieldsARM64(cb, 1, instrPC+IE64_INSTR_SIZE, cb.instrCountBase+bailCount+1, br)
	emitEpilogue(cb, writtenSoFar, br.used)
	return resumePatch
}

// emitJSR_IND handles JSR_IND (register-indirect subroutine call,
// target = rs + sext(imm32)). Phase 5: routes the return-address push
// through HELPER_JSR_IND on MMU-on / high SP; HelperVal carries the
// return address, HelperAddr the computed target.
func emitJSR_IND(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, instrCount uint32, writtenSoFar uint32) {
	// MMU-on → helper.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuHelperOff := cb.Len()
	cb.Emit32(0) // CBNZ X1, helper

	// Underflow guard: SP >= 8.
	cb.Emit32(arm64CMP_imm(arm64RegIE64SP, 8))
	underflowOff := cb.Len()
	cb.Emit32(0) // B.LO helper

	// High-slot guard: SP <= MemSize.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4)))
	cb.Emit32(arm64CMP(arm64RegIE64SP, 1))
	highHelperOff := cb.Len()
	cb.Emit32(0) // B.HI helper

	// Fast path.
	cb.Emit32(arm64SUB_imm(arm64RegIE64SP, arm64RegIE64SP, 8))
	retAddr := uint64(instrPC + IE64_INSTR_SIZE)
	emitLoadImm64(cb, 0, retAddr)
	cb.Emit32(arm64STR_reg(0, arm64RegMemBase, arm64RegIE64SP))

	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(arm64RegIE64PC, rsReg, 1))

	// PLAN_MAX_RAM.md slice 8 phase 8 retired the IE64_ADDR_MASK AND.

	// Phase 2: count to ctx.RetCount; X28 already carries full 64-bit PC.
	emitStoreRetCount(cb, instrCount, br)
	emitEpilogue(cb, br.written, br.used)

	// Helper exit (block exit; reached only via the guard jumps above).
	helperPC := cb.Len()
	cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
	cb.PatchUint32(underflowOff, arm64Bcond(arm64CondLO, int32(helperPC-underflowOff)))
	cb.PatchUint32(highHelperOff, arm64Bcond(arm64CondHI, int32(helperPC-highHelperOff)))
	emitJSR_INDHelperExitARM64(cb, ji, instrPC, br, writtenSoFar)
}

// emitJSR_INDHelperExitARM64 writes the JITContext HELPER_JSR_IND protocol
// fields. The target (rs + sext(imm32)) is computed into X2 and stored to
// HelperAddr; HelperVal carries the return address.
func emitJSR_INDHelperExitARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	// Compute target = base + sext(imm32) into X2 before loading ctx into X1.
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	if ji.rs == 31 {
		// JSR_IND R31: the interpreter decrements SP before resolving the
		// SP-relative target (cpu_ie64.go:1998-2010), as does the fast path.
		// rsReg (X27) is the live pre-decrement SP, so subtract 8 first.
		cb.Emit32(arm64SUB_imm(2, rsReg, 8)) // X2 = SP - 8
		cb.Emit32(arm64ADD(2, 2, 1))         // X2 = (SP-8) + imm32
	} else {
		cb.Emit32(arm64ADD(2, rsReg, 1)) // X2 = rs + imm32 (target)
	}

	cb.Emit32(arm64LDR_imm(1, 31, 96/8))                                  // X1 = ctx ptr
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperAddr/8)))          // HelperAddr = target
	emitLoadImm64(cb, 2, instrPC+IE64_INSTR_SIZE)                         //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperVal/8)))           // HelperVal = retAddr
	cb.Emit32(arm64STR_imm(arm64RegIE64SP, 1, uint32(jitCtxOffLiveSP/8))) // LiveSP = X27
	emitLoadImm64(cb, 2, instrPC)                                         //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperPC/8)))            // HelperPC
	emitLoadImm32(cb, 2, HELPER_JSR_IND)                                  //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffNeedHelper/4)))        // NeedHelper

	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
}

// ===========================================================================
// FPU emission helpers
// ===========================================================================

const (
	fpuOffFPCR = 64 / 4 // FPCR imm12 offset for LDR_W_imm/STR_W_imm
	fpuOffFPSR = 68 / 4 // FPSR imm12 offset for LDR_W_imm/STR_W_imm
)

func emitLoadFPReg(cb *CodeBuffer, armReg, fpIdx byte) {
	if v, ok := ie64FPResidentSingleARM64(cb, fpIdx); ok {
		cb.Emit32(arm64FMOV_StoW(armReg, v))
		return
	}
	cb.Emit32(arm64LDR_W_imm(armReg, arm64RegFPUBase, uint32(fpIdx&0x0F)))
}

func emitStoreFPReg(cb *CodeBuffer, armReg, fpIdx byte) {
	if v, ok := ie64FPResidentSingleARM64(cb, fpIdx); ok {
		cb.Emit32(arm64FMOV_WtoS(v, armReg))
		return
	}
	cb.Emit32(arm64STR_W_imm(armReg, arm64RegFPUBase, uint32(fpIdx&0x0F)))
}

func ie64FPResidentSingleARM64(cb *CodeBuffer, fpIdx byte) (byte, bool) {
	if cb.fpPlan == nil {
		return 0, false
	}
	b, ok := cb.fpPlan.resident(fpIdx & 0x0F)
	return b.xmm, ok && b.kind == ie64FPResSingle
}

func ie64FPResidentPairARM64(cb *CodeBuffer, fpIdx byte) (byte, bool) {
	if cb.fpPlan == nil {
		return 0, false
	}
	b, ok := cb.fpPlan.resident(fpIdx & 0x0E)
	return b.xmm, ok && b.kind == ie64FPResPair
}

func emitFPResidencyLoadARM64(cb *CodeBuffer) {
	if cb.fpPlan == nil {
		return
	}
	for _, b := range cb.fpPlan.bindings {
		if b.kind == ie64FPResSingle {
			cb.Emit32(arm64LDR_W_imm(0, arm64RegFPUBase, uint32(b.baseSlot)))
			cb.Emit32(arm64FMOV_WtoS(b.xmm, 0))
		} else {
			cb.Emit32(arm64LDR_imm(0, arm64RegFPUBase, uint32(b.baseSlot/2)))
			cb.Emit32(arm64FMOV_XtoD(b.xmm, 0))
		}
	}
}

func emitFPResidencySpillARM64(cb *CodeBuffer) {
	if cb.fpPlan == nil {
		return
	}
	for _, b := range cb.fpPlan.bindings {
		if b.kind == ie64FPResSingle {
			cb.Emit32(arm64FMOV_StoW(0, b.xmm))
			cb.Emit32(arm64STR_W_imm(0, arm64RegFPUBase, uint32(b.baseSlot)))
		} else {
			cb.Emit32(arm64FMOV_DtoX(0, b.xmm))
			cb.Emit32(arm64STR_imm(0, arm64RegFPUBase, uint32(b.baseSlot/2)))
		}
	}
}

// emitSetFPCondCodes classifies IEEE-754 bits in W0 and updates FPSR condition
// codes (bits 27:24). Preserves exception flags (bits 3:0). Uses W1-W3 scratch.
func emitSetFPCondCodes(cb *CodeBuffer) {
	// Extract exponent[7:0]
	cb.Emit32(arm64UBFX_W(1, 0, 23, 8))
	// W3 = default CC = 0
	cb.Emit32(arm64MOVZ_W(3, 0, 0))
	// Check special: exp == 0xFF
	cb.Emit32(arm64CMP_imm(1, 0xFF))
	notSpecialOff := cb.Len()
	cb.Emit32(0) // placeholder B.NE → not_special

	// exp == 0xFF: check NaN vs Inf
	cb.Emit32(arm64UBFX_W(2, 0, 0, 23)) // W2 = fraction
	isNanOff := cb.Len()
	cb.Emit32(0) // placeholder CBNZ → is_nan

	// Infinity: CC_I = 0x02000000
	cb.Emit32(arm64MOVZ_W(3, 0x0200, 16))
	// Check sign
	cb.Emit32(arm64LSR_W_imm(2, 0, 31))
	storeCCFromInfOff := cb.Len()
	cb.Emit32(0) // placeholder CBZ → store_cc (positive inf)
	// Negative infinity: add CC_N
	cb.Emit32(arm64MOVZ_W(1, 0x0800, 16))
	cb.Emit32(arm64ORR_W(3, 3, 1))
	storeCCFromNegInfOff := cb.Len()
	cb.Emit32(0) // B → store_cc

	// is_nan:
	isNanPC := cb.Len()
	cb.PatchUint32(isNanOff, arm64CBNZ(2, int32(isNanPC-isNanOff)))
	cb.Emit32(arm64MOVZ_W(3, 0x0100, 16)) // CC_NAN = 0x01000000
	storeCCFromNanOff := cb.Len()
	cb.Emit32(0) // B → store_cc

	// not_special:
	notSpecialPC := cb.Len()
	cb.PatchUint32(notSpecialOff, arm64Bcond(arm64CondNE, int32(notSpecialPC-notSpecialOff)))
	// Check zero: bits & 0x7FFFFFFF == 0
	cb.Emit32(arm64UBFX_W(2, 0, 0, 31))
	isZeroOff := cb.Len()
	cb.Emit32(0) // placeholder CBZ → is_zero

	// Normal: check sign
	cb.Emit32(arm64LSR_W_imm(3, 0, 31)) // W3 = 0 or 1
	storeCCFromPosOff := cb.Len()
	cb.Emit32(0) // placeholder CBZ → store_cc (positive)
	// Negative normal:
	cb.Emit32(arm64MOVZ_W(3, 0x0800, 16)) // CC_N = 0x08000000
	storeCCFromNegOff := cb.Len()
	cb.Emit32(0) // B → store_cc

	// is_zero:
	isZeroPC := cb.Len()
	cb.PatchUint32(isZeroOff, arm64CBZ(2, int32(isZeroPC-isZeroOff)))
	cb.Emit32(arm64MOVZ_W(3, 0x0400, 16)) // CC_Z = 0x04000000
	// fall through to store_cc

	// store_cc:
	storeCCPC := cb.Len()
	cb.PatchUint32(storeCCFromInfOff, arm64CBZ(2, int32(storeCCPC-storeCCFromInfOff)))
	cb.PatchUint32(storeCCFromNegInfOff, arm64B(int32(storeCCPC-storeCCFromNegInfOff)))
	cb.PatchUint32(storeCCFromNanOff, arm64B(int32(storeCCPC-storeCCFromNanOff)))
	cb.PatchUint32(storeCCFromPosOff, arm64CBZ(3, int32(storeCCPC-storeCCFromPosOff)))
	cb.PatchUint32(storeCCFromNegOff, arm64B(int32(storeCCPC-storeCCFromNegOff)))

	// Load FPSR, preserve exception bits, set new CC
	cb.Emit32(arm64LDR_W_imm(1, arm64RegFPUBase, fpuOffFPSR))
	cb.Emit32(arm64UBFX_W(1, 1, 0, 4)) // keep only exception flags
	cb.Emit32(arm64ORR_W(1, 1, 3))     // combine with new CC
	cb.Emit32(arm64STR_W_imm(1, arm64RegFPUBase, fpuOffFPSR))
}

// emitMaterializeFPCCARM64 emits a pending sunk CC update. It is called from
// emitEpilogue, which on this backend is the block's only exit funnel: every
// arm64 RET is emitted there, so block end, the terminators and every mid-block
// helper bail all pass through it. amd64 needs a second funnel
// (emitLightweightStoreRegs) for its chain exits; arm64 has no chaining tier.
//
// Only X0-X3 are touched. All four are scratch and dead at an exit: the return
// channel is memory (ctx.RetPC), none of them carries a mapped IE64 register
// (X12-X17, X19-X26), and X7 holds the loop counter only when
// hasBackwardBranch.
//
// X6 (the FPU base) has to be live for the re-read, and is, on two counts. The
// prologue loads it only when blockRegs.hasFPU, but hasFPU is set for every FPU
// opcode (0x60-0x7C), a superset of the sinkable writers in
// ie64FPSRCCSinkable — so an update can only be pending in a block that loaded
// it. And materialising happens at the top of emitEpilogue, before the
// callee-saved restores retire it.
//
// The pending slot is deliberately not cleared: the funnel sits on one exit
// path, and the fall-through may reach another.
//
// Reading the value back through emitLoadFPReg rather than trusting a register
// to have survived is what keeps this correct once FP residency lands on this
// backend, and mirrors the amd64 twin.
func emitMaterializeFPCCARM64(cb *CodeBuffer) {
	if !cb.pendingFPCC.valid {
		return
	}
	emitLoadFPReg(cb, 0, cb.pendingFPCC.reg)
	emitSetFPCondCodes(cb)
}

// emitFPCondCodesARM64 applies the liveness pass's decision for an FP32 CC
// writer whose classified value is in W0 and whose destination is ji.rd.
//
// Three outcomes, from jit_ie64_fpsr_liveness.go:
//   - fpsrCCDead: a later non-faulting FP32 killer overwrites the whole CC
//     field before anything can read it, so drop the update entirely.
//   - fpsrCCSink: nothing inside the block can read it, so defer it to the exit
//     funnel. This is what takes the classifier out of hot loop bodies: a loop
//     whose only CC observers are its exits pays once on the way out rather
//     than once per iteration.
//   - neither: emit in place.
//
// Only the CC field is elided or deferred. Sticky exception flags stay eager on
// every path, because a raised flag remains observable until software clears
// it.
func emitFPCondCodesARM64(cb *CodeBuffer, ji *JITInstr) {
	if ji.fpsrCCDead {
		return
	}
	if ji.fpsrCCSink {
		cb.pendingFPCC = ie64FPCCPending{valid: true, reg: ji.rd}
		return
	}
	cb.pendingFPCC.valid = false
	emitSetFPCondCodes(cb)
}

// emitFPCondCodes64ARM64 is emitFPCondCodesARM64 for a binary64 result already
// in X0.
//
// There is no sink case: ie64FPSRCCSinkable covers the FP32 writers only, whose
// value can be reconstructed at a funnel by re-reading the single FP32 slot
// ji.rd. It also needs no interaction with a pending FP32 update, because the
// pass treats every FP64 op as an inline observer (they can bail), so an
// earlier writer is never sunk across one and nothing can be pending here.
func emitFPCondCodes64ARM64(cb *CodeBuffer, ji *JITInstr) {
	if ji.fpsrCCDead {
		return
	}
	emitSetFPCondCodes64(cb)
}

// emitSetFPCondCodes64 classifies IEEE-754 binary64 bits in X0 and updates
// FPSR condition codes. It preserves exception flags.
func emitSetFPCondCodes64(cb *CodeBuffer) {
	cb.Emit32(arm64LSR_imm(1, 0, 52))       // exponent
	cb.Emit32(arm64AND_imm(1, 1, 0, 10, 1)) // X1 &= 0x7FF
	emitLoadImm32(cb, 3, 0)                 // W3 = CC
	cb.Emit32(arm64CMP_imm(1, 0x7FF))
	notSpecialOff := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64LSL_imm(2, 0, 12)) // fraction bits; exp/sign shifted out
	isNanOff := cb.Len()
	cb.Emit32(0)

	emitLoadImm32(cb, 3, IE64_FPU_CC_I)
	cb.Emit32(arm64LSR_imm(2, 0, 63))
	storeCCFromInfOff := cb.Len()
	cb.Emit32(0)
	emitLoadImm32(cb, 1, IE64_FPU_CC_N)
	cb.Emit32(arm64ORR_W(3, 3, 1))
	storeCCFromNegInfOff := cb.Len()
	cb.Emit32(0)

	isNanPC := cb.Len()
	cb.PatchUint32(isNanOff, arm64CBNZ(2, int32(isNanPC-isNanOff)))
	emitLoadImm32(cb, 3, IE64_FPU_CC_NAN)
	storeCCFromNanOff := cb.Len()
	cb.Emit32(0)

	notSpecialPC := cb.Len()
	cb.PatchUint32(notSpecialOff, arm64Bcond(arm64CondNE, int32(notSpecialPC-notSpecialOff)))
	cb.Emit32(arm64LSL_imm(2, 0, 1)) // abs-zero check
	isZeroOff := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64LSR_imm(3, 0, 63))
	storeCCFromPosOff := cb.Len()
	cb.Emit32(0)
	emitLoadImm32(cb, 3, IE64_FPU_CC_N)
	storeCCFromNegOff := cb.Len()
	cb.Emit32(0)

	isZeroPC := cb.Len()
	cb.PatchUint32(isZeroOff, arm64CBZ(2, int32(isZeroPC-isZeroOff)))
	emitLoadImm32(cb, 3, IE64_FPU_CC_Z)

	storeCCPC := cb.Len()
	cb.PatchUint32(storeCCFromInfOff, arm64CBZ(2, int32(storeCCPC-storeCCFromInfOff)))
	cb.PatchUint32(storeCCFromNegInfOff, arm64B(int32(storeCCPC-storeCCFromNegInfOff)))
	cb.PatchUint32(storeCCFromNanOff, arm64B(int32(storeCCPC-storeCCFromNanOff)))
	cb.PatchUint32(storeCCFromPosOff, arm64CBZ(3, int32(storeCCPC-storeCCFromPosOff)))
	cb.PatchUint32(storeCCFromNegOff, arm64B(int32(storeCCPC-storeCCFromNegOff)))

	cb.Emit32(arm64LDR_W_imm(1, arm64RegFPUBase, fpuOffFPSR))
	cb.Emit32(arm64UBFX_W(1, 1, 0, 4))
	cb.Emit32(arm64ORR_W(1, 1, 3))
	cb.Emit32(arm64STR_W_imm(1, arm64RegFPUBase, fpuOffFPSR))
}

// ===========================================================================
// FPU — Category A: Pure integer bitwise on FP registers
// ===========================================================================

func emitFMOV(cb *CodeBuffer, ji *JITInstr) {
	emitLoadFPReg(cb, 0, ji.rs)
	emitStoreFPReg(cb, 0, ji.rd)
}

func emitFABS(cb *CodeBuffer, ji *JITInstr) {
	emitLoadFPReg(cb, 0, ji.rs)
	emitLoadImm32(cb, 1, 0x7FFFFFFF)
	cb.Emit32(arm64AND_W(0, 0, 1)) // clear sign bit
	emitStoreFPReg(cb, 0, ji.rd)
	emitFPCondCodesARM64(cb, ji)
}

func emitFNEG(cb *CodeBuffer, ji *JITInstr) {
	emitLoadFPReg(cb, 0, ji.rs)
	emitLoadImm32(cb, 1, 0x80000000)
	cb.Emit32(arm64EOR_W(0, 0, 1)) // flip sign bit
	emitStoreFPReg(cb, 0, ji.rd)
	emitFPCondCodesARM64(cb, ji)
}

func emitFMOVI(cb *CodeBuffer, ji *JITInstr) {
	rsReg := resolveReg(cb, ji.rs, 0)
	cb.Emit32(arm64MOV_W(0, rsReg)) // W0 = uint32(rs)
	emitStoreFPReg(cb, 0, ji.rd)
	emitFPCondCodesARM64(cb, ji)
}

func emitFMOVO(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}
	emitLoadFPReg(cb, 0, ji.rs)
	cb.Emit32(arm64MOV_W(0, 0)) // zero-extend to 64-bit
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if mapped {
		cb.Emit32(arm64MOV(dstReg, 0))
	} else {
		emitStoreSpilledReg(cb, 0, ji.rd)
	}
}

func emitFMOVECR(cb *CodeBuffer, ji *JITInstr) {
	idx := byte(ji.imm32)
	var bits uint32
	if idx <= 15 {
		bits = ie64FmovecrROMTable[idx]
	}
	emitLoadImm32(cb, 0, bits)
	emitStoreFPReg(cb, 0, ji.rd)
	emitFPCondCodesARM64(cb, ji)
}

func emitFMOVSR(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}
	cb.Emit32(arm64LDR_W_imm(0, arm64RegFPUBase, fpuOffFPSR))
	cb.Emit32(arm64MOV_W(0, 0)) // zero-extend
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if mapped {
		cb.Emit32(arm64MOV(dstReg, 0))
	} else {
		emitStoreSpilledReg(cb, 0, ji.rd)
	}
}

func emitFMOVCR(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}
	cb.Emit32(arm64LDR_W_imm(0, arm64RegFPUBase, fpuOffFPCR))
	cb.Emit32(arm64MOV_W(0, 0))
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if mapped {
		cb.Emit32(arm64MOV(dstReg, 0))
	} else {
		emitStoreSpilledReg(cb, 0, ji.rd)
	}
}

func emitFMOVSC(cb *CodeBuffer, ji *JITInstr) {
	rsReg := resolveReg(cb, ji.rs, 0)
	cb.Emit32(arm64MOV_W(0, rsReg))
	emitLoadImm32(cb, 1, IE64_FPU_FPSR_MASK)
	cb.Emit32(arm64AND_W(0, 0, 1))
	cb.Emit32(arm64STR_W_imm(0, arm64RegFPUBase, fpuOffFPSR))
}

func emitFMOVCC(cb *CodeBuffer, ji *JITInstr) {
	rsReg := resolveReg(cb, ji.rs, 0)
	cb.Emit32(arm64MOV_W(0, rsReg))
	cb.Emit32(arm64STR_W_imm(0, arm64RegFPUBase, fpuOffFPCR))
}

// ===========================================================================
// FPU — Category B: Native ARM64 FP instructions
// ===========================================================================

// armSkip is a forward conditional branch awaiting its target.
type armSkip struct {
	off  int
	cond byte
}

// patchArmSkips points every queued forward branch at target.
func patchArmSkips(cb *CodeBuffer, skips []armSkip, target int) {
	for _, s := range skips {
		cb.PatchUint32(s.off, arm64Bcond(s.cond, int32(target-s.off)))
	}
}

// emitFPStickyFlags32ARM64 emits the IE64 sticky exception-flag updates for an
// FP32 binary op, mirroring IE64FPU.FADD/FSUB/FMUL/FDIV and the amd64 backend's
// emitFP32StickyFlagsAMD64 rule for rule. The two backends and the interpreter
// must agree exactly; the shared special-value matrix in
// jit_ie64_fp_parity_common_test.go is the gate.
//
// Sticky flags cannot be elided the way condition codes can: unlike the CC
// field, a raised exception flag stays observable until software clears it, so
// these updates are eager.
//
// Entry: W0 = result bits, W1 = ft bits, S0 = fs bits (left there by the
// arithmetic). W0 is preserved for the caller's register store and CC update.
// W1-W4 and W11 are clobbered.
func emitFPStickyFlags32ARM64(cb *CodeBuffer, op byte) {
	const (
		res   byte = 0
		ftReg byte = 1
		fsReg byte = 2
		acc   byte = 3
		tmp   byte = 4
	)
	// Recover fs from S0: the arithmetic overwrote W0 with the result, but the
	// source vector register still holds the original bits.
	cb.Emit32(arm64FMOV_StoW(fsReg, 0))
	cb.Emit32(arm64MOVZ_W(acc, 0, 0)) // W3 = flags accumulator = 0

	// skipIf emits "CMP |src|, imm" and a forward branch taken when the clause
	// fails, so the caller's OR runs only when every clause holds.
	skipIf := func(src byte, imm uint32, cond byte) armSkip {
		emitLoadImm32(cb, arm64RegScratch, 0x7FFFFFFF)
		cb.Emit32(arm64AND_W(tmp, src, arm64RegScratch)) // W4 = |src|
		emitLoadImm32(cb, arm64RegScratch, imm)
		cb.Emit32(arm64CMP_W(tmp, arm64RegScratch))
		off := cb.Len()
		cb.Emit32(0)
		return armSkip{off: off, cond: cond}
	}
	isInf := func(src byte) armSkip { return skipIf(src, fp32AbsInf, arm64CondNE) }
	notInf := func(src byte) armSkip { return skipIf(src, fp32AbsInf, arm64CondEQ) }
	isNaN := func(src byte) armSkip { return skipIf(src, fp32AbsInf, arm64CondLS) }
	notNaN := func(src byte) armSkip { return skipIf(src, fp32AbsInf, arm64CondHI) }
	isZero := func(src byte) armSkip { return skipIf(src, 0, arm64CondNE) }
	notZero := func(src byte) armSkip { return skipIf(src, 0, arm64CondEQ) }

	// raise emits the conjunction of checks; each check branches past the OR
	// when its clause fails.
	raise := func(flag uint32, checks ...func() armSkip) {
		skips := make([]armSkip, 0, len(checks))
		for _, c := range checks {
			skips = append(skips, c())
		}
		emitLoadImm32(cb, arm64RegScratch, flag)
		cb.Emit32(arm64ORR_W(acc, acc, arm64RegScratch))
		patchArmSkips(cb, skips, cb.Len())
	}

	if op == OP_FDIV {
		// DZ: isZero(t) && !isZero(s) && !isNaN(s)
		raise(IE64_FPU_EX_DZ,
			func() armSkip { return isZero(ftReg) },
			func() armSkip { return notZero(fsReg) },
			func() armSkip { return notNaN(fsReg) })
		// OE: isInf(res) && !isInf(s) && !isZero(t)
		raise(IE64_FPU_EX_OE,
			func() armSkip { return isInf(res) },
			func() armSkip { return notInf(fsReg) },
			func() armSkip { return notZero(ftReg) })
	} else {
		// OE: isInf(res) && !isInf(s) && !isInf(t)
		raise(IE64_FPU_EX_OE,
			func() armSkip { return isInf(res) },
			func() armSkip { return notInf(fsReg) },
			func() armSkip { return notInf(ftReg) })
	}

	// IO: isNaN(res) && !isNaN(s) && !isNaN(t). Identical for all four ops.
	raise(IE64_FPU_EX_IO,
		func() armSkip { return isNaN(res) },
		func() armSkip { return notNaN(fsReg) },
		func() armSkip { return notNaN(ftReg) })

	switch op {
	case OP_FMUL:
		// UE: isZero(res) && !isZero(s) && !isZero(t)
		raise(IE64_FPU_EX_UE,
			func() armSkip { return isZero(res) },
			func() armSkip { return notZero(fsReg) },
			func() armSkip { return notZero(ftReg) })
	case OP_FDIV:
		// UE: isZero(res) && !isZero(s) && !isZero(t) && !isInf(t)
		raise(IE64_FPU_EX_UE,
			func() armSkip { return isZero(res) },
			func() armSkip { return notZero(fsReg) },
			func() armSkip { return notZero(ftReg) },
			func() armSkip { return notInf(ftReg) })
	}

	// FPSR |= W3. Sticky: never clears an existing flag.
	cb.Emit32(arm64LDR_W_imm(arm64RegScratch, arm64RegFPUBase, fpuOffFPSR))
	cb.Emit32(arm64ORR_W(arm64RegScratch, arm64RegScratch, acc))
	cb.Emit32(arm64STR_W_imm(arm64RegScratch, arm64RegFPUBase, fpuOffFPSR))
}

// emitFPStickyGate32ARM64 wraps the classifier in a fast-path test. No flag can
// be raised unless the result is infinite, NaN, or (for the ops with an
// underflow rule) zero, so an ordinary finite non-zero result skips the whole
// classifier.
//
// The gate gets away with inspecting only the result because every rule that
// does not mention res implies a special res: a divide by zero always yields
// infinity or NaN.
func emitFPStickyGate32ARM64(cb *CodeBuffer, op byte) {
	emitLoadImm32(cb, arm64RegScratch, 0x7FFFFFFF)
	cb.Emit32(arm64AND_W(4, 0, arm64RegScratch)) // W4 = |res|
	emitLoadImm32(cb, arm64RegScratch, fp32AbsInf)
	cb.Emit32(arm64CMP_W(4, arm64RegScratch))
	runOff := cb.Len()
	cb.Emit32(0) // B.HS classifier (result is inf or NaN)

	zeroOff := -1
	if op == OP_FMUL || op == OP_FDIV {
		zeroOff = cb.Len()
		cb.Emit32(0) // CBZ W4, classifier (result is zero: underflow rule)
	}

	skipOff := cb.Len()
	cb.Emit32(0) // B past the classifier

	runPC := cb.Len()
	cb.PatchUint32(runOff, arm64Bcond(arm64CondHS, int32(runPC-runOff)))
	if zeroOff >= 0 {
		// AND_W zero-extends into X4, so the 64-bit CBZ tests the same value.
		cb.PatchUint32(zeroOff, arm64CBZ(4, int32(runPC-zeroOff)))
	}
	emitFPStickyFlags32ARM64(cb, op)
	cb.PatchUint32(skipOff, arm64B(int32(cb.Len()-skipOff)))
}

func emitFPBinaryArith(cb *CodeBuffer, ji *JITInstr, fpOp func(sd, sn, sm byte) uint32) {
	emitLoadFPReg(cb, 0, ji.rs)
	cb.Emit32(arm64FMOV_WtoS(0, 0))
	emitLoadFPReg(cb, 1, ji.rt)
	cb.Emit32(arm64FMOV_WtoS(1, 1))
	cb.Emit32(fpOp(2, 0, 1))
	cb.Emit32(arm64FMOV_StoW(0, 2))
	// Sticky flags before the store: the classifier needs W0 = result and
	// S0 = fs, and emitSetFPCondCodes preserves the sticky bits it finds.
	emitFPStickyGate32ARM64(cb, ji.opcode)
	emitStoreFPReg(cb, 0, ji.rd)
	emitFPCondCodesARM64(cb, ji)
}

func emitFADD(cb *CodeBuffer, ji *JITInstr) { emitFPBinaryArith(cb, ji, arm64FADD_S) }
func emitFSUB(cb *CodeBuffer, ji *JITInstr) { emitFPBinaryArith(cb, ji, arm64FSUB_S) }
func emitFMUL(cb *CodeBuffer, ji *JITInstr) { emitFPBinaryArith(cb, ji, arm64FMUL_S) }
func emitFDIV(cb *CodeBuffer, ji *JITInstr) { emitFPBinaryArith(cb, ji, arm64FDIV_S) }

// emitFPSqrtSticky32ARM64 emits the IE64 sticky exception update for FSQRT,
// mirroring IE64FPU.FSQRT and the amd64 backend's emitFPSqrtSticky32AMD64: IO is
// raised when the operand is negative, excluding -0.0 and NaN. FSQRT has no
// other exception rule.
//
// The clauses are ordered so the common case (a non-negative operand) leaves
// after one compare and one not-taken branch.
//
// On entry W0 = fs bits, and is preserved for the caller. W4 and W11 are
// clobbered.
func emitFPSqrtSticky32ARM64(cb *CodeBuffer) {
	var condSkips []armSkip

	// Sign clear -> no IO. Unsigned: the sign bit is set iff bits >= 0x80000000.
	emitLoadImm32(cb, arm64RegScratch, 0x80000000)
	cb.Emit32(arm64CMP_W(0, arm64RegScratch))
	signOff := cb.Len()
	cb.Emit32(0)
	condSkips = append(condSkips, armSkip{off: signOff, cond: arm64CondLO})

	// -0.0 -> no IO.
	emitLoadImm32(cb, arm64RegScratch, 0x7FFFFFFF)
	cb.Emit32(arm64AND_W(4, 0, arm64RegScratch)) // W4 = |fs|
	zeroOff := cb.Len()
	cb.Emit32(0) // CBZ W4, done

	// NaN -> no IO (a negative NaN still has the sign bit set).
	emitLoadImm32(cb, arm64RegScratch, fp32AbsInf)
	cb.Emit32(arm64CMP_W(4, arm64RegScratch))
	nanOff := cb.Len()
	cb.Emit32(0)
	condSkips = append(condSkips, armSkip{off: nanOff, cond: arm64CondHI})

	// FPSR |= IO. Sticky: never clears an existing flag.
	cb.Emit32(arm64LDR_W_imm(arm64RegScratch, arm64RegFPUBase, fpuOffFPSR))
	emitLoadImm32(cb, 4, IE64_FPU_EX_IO)
	cb.Emit32(arm64ORR_W(arm64RegScratch, arm64RegScratch, 4))
	cb.Emit32(arm64STR_W_imm(arm64RegScratch, arm64RegFPUBase, fpuOffFPSR))

	done := cb.Len()
	patchArmSkips(cb, condSkips, done)
	// AND_W zero-extends into X4, so the 64-bit CBZ tests the same value.
	cb.PatchUint32(zeroOff, arm64CBZ(4, int32(done-zeroOff)))
}

func emitFSQRT(cb *CodeBuffer, ji *JITInstr) {
	emitLoadFPReg(cb, 0, ji.rs)
	cb.Emit32(arm64FMOV_WtoS(0, 0))
	cb.Emit32(arm64FSQRT_S(1, 0))
	// Classify before W0 is overwritten: the rule is over the operand, not the
	// result.
	emitFPSqrtSticky32ARM64(cb)
	cb.Emit32(arm64FMOV_StoW(0, 1))
	emitStoreFPReg(cb, 0, ji.rd)
	emitFPCondCodesARM64(cb, ji)
}

func emitFINT(cb *CodeBuffer, ji *JITInstr) {
	// Load FPCR rounding mode
	cb.Emit32(arm64LDR_W_imm(1, arm64RegFPUBase, fpuOffFPCR))
	cb.Emit32(arm64UBFX_W(1, 1, 0, 2)) // W1 = rounding mode (0-3)

	// Load source FP reg
	emitLoadFPReg(cb, 0, ji.rs)
	cb.Emit32(arm64FMOV_WtoS(0, 0))

	// mode 0 (nearest) is most common — check first
	nearestOff := cb.Len()
	cb.Emit32(0) // placeholder CBZ → nearest

	cb.Emit32(arm64CMP_imm(1, 1))
	truncOff := cb.Len()
	cb.Emit32(0) // placeholder B.EQ → truncate

	cb.Emit32(arm64CMP_imm(1, 2))
	floorOff := cb.Len()
	cb.Emit32(0) // placeholder B.EQ → floor

	// mode 3: ceil
	cb.Emit32(arm64FRINTP_S(1, 0))
	doneOff1 := cb.Len()
	cb.Emit32(0)

	// floor:
	floorPC := cb.Len()
	cb.PatchUint32(floorOff, arm64Bcond(arm64CondEQ, int32(floorPC-floorOff)))
	cb.Emit32(arm64FRINTM_S(1, 0))
	doneOff2 := cb.Len()
	cb.Emit32(0)

	// truncate:
	truncPC := cb.Len()
	cb.PatchUint32(truncOff, arm64Bcond(arm64CondEQ, int32(truncPC-truncOff)))
	cb.Emit32(arm64FRINTZ_S(1, 0))
	doneOff3 := cb.Len()
	cb.Emit32(0)

	// nearest:
	nearestPC := cb.Len()
	cb.PatchUint32(nearestOff, arm64CBZ(1, int32(nearestPC-nearestOff)))
	cb.Emit32(arm64FRINTN_S(1, 0))
	// fall through

	// done:
	donePC := cb.Len()
	cb.PatchUint32(doneOff1, arm64B(int32(donePC-doneOff1)))
	cb.PatchUint32(doneOff2, arm64B(int32(donePC-doneOff2)))
	cb.PatchUint32(doneOff3, arm64B(int32(donePC-doneOff3)))

	cb.Emit32(arm64FMOV_StoW(0, 1))
	emitStoreFPReg(cb, 0, ji.rd)
	emitFPCondCodesARM64(cb, ji)
}

func emitFCMP(cb *CodeBuffer, ji *JITInstr) {
	// Load FP registers into S0, S1
	emitLoadFPReg(cb, 0, ji.rs)
	cb.Emit32(arm64FMOV_WtoS(0, 0))
	emitLoadFPReg(cb, 1, ji.rt)
	cb.Emit32(arm64FMOV_WtoS(1, 1))

	// Clear CC bits in FPSR, keep exception flags
	cb.Emit32(arm64LDR_W_imm(2, arm64RegFPUBase, fpuOffFPSR))
	cb.Emit32(arm64UBFX_W(2, 2, 0, 4))

	// ARM64 FCMP S0, S1
	cb.Emit32(arm64FCMP_S(0, 1))

	// Default result = 0
	cb.Emit32(arm64MOVZ_W(3, 0, 0))

	// VS (V set) → unordered (NaN)
	nanOff := cb.Len()
	cb.Emit32(0)

	// MI (N set) → less than
	ltOff := cb.Len()
	cb.Emit32(0)

	// EQ → equal
	eqOff := cb.Len()
	cb.Emit32(0)

	// W0 still holds the fs bits, and W1 the ft bits: the CC constants below go
	// through W4 so the greater-than and equal paths can inspect fs and
	// reproduce IE64FPU.FCMP's infinity rules.

	// Greater than (fallthrough)
	cb.Emit32(arm64MOVZ_W(3, 1, 0)) // result = 1
	// CC_I when fs is +Inf. IE64FPU.FCMP raises it here only for +Inf, which is
	// the only infinity that can be the greater operand.
	emitLoadImm32(cb, 4, fp32AbsInf)
	cb.Emit32(arm64CMP_W(0, 4))
	gtNotInfOff := cb.Len()
	cb.Emit32(0)
	emitLoadImm32(cb, 4, IE64_FPU_CC_I)
	cb.Emit32(arm64ORR_W(2, 2, 4))
	cb.PatchUint32(gtNotInfOff, arm64Bcond(arm64CondNE, int32(cb.Len()-gtNotInfOff)))
	doneOff1 := cb.Len()
	cb.Emit32(0)

	// nan:
	nanPC := cb.Len()
	cb.PatchUint32(nanOff, arm64Bcond(arm64CondVS, int32(nanPC-nanOff)))
	emitLoadImm32(cb, 4, IE64_FPU_CC_NAN|IE64_FPU_EX_IO)
	cb.Emit32(arm64ORR_W(2, 2, 4))
	doneOff2 := cb.Len()
	cb.Emit32(0)

	// lt:
	ltPC := cb.Len()
	cb.PatchUint32(ltOff, arm64Bcond(arm64CondMI, int32(ltPC-ltOff)))
	emitLoadImm32(cb, 3, 0xFFFFFFFF)
	cb.Emit32(arm64SXTW(3, 3)) // X3 = -1
	emitLoadImm32(cb, 4, IE64_FPU_CC_N)
	cb.Emit32(arm64ORR_W(2, 2, 4))
	doneOff3 := cb.Len()
	cb.Emit32(0)

	// eq:
	eqPC := cb.Len()
	cb.PatchUint32(eqOff, arm64Bcond(arm64CondEQ, int32(eqPC-eqOff)))
	emitLoadImm32(cb, 4, IE64_FPU_CC_Z)
	cb.Emit32(arm64ORR_W(2, 2, 4))
	// Equal infinities: IE64FPU.FCMP adds CC_I, and CC_N as well for -Inf.
	emitLoadImm32(cb, arm64RegScratch, 0x7FFFFFFF)
	cb.Emit32(arm64AND_W(4, 0, arm64RegScratch)) // W4 = |fs|
	emitLoadImm32(cb, arm64RegScratch, fp32AbsInf)
	cb.Emit32(arm64CMP_W(4, arm64RegScratch))
	eqNotInfOff := cb.Len()
	cb.Emit32(0)
	emitLoadImm32(cb, 4, IE64_FPU_CC_I)
	cb.Emit32(arm64ORR_W(2, 2, 4))
	emitLoadImm32(cb, arm64RegScratch, 0x80000000)
	cb.Emit32(arm64CMP_W(0, arm64RegScratch))
	eqNotNegOff := cb.Len()
	cb.Emit32(0)
	emitLoadImm32(cb, 4, IE64_FPU_CC_N)
	cb.Emit32(arm64ORR_W(2, 2, 4))
	eqCCDonePC := cb.Len()
	cb.PatchUint32(eqNotNegOff, arm64Bcond(arm64CondLO, int32(eqCCDonePC-eqNotNegOff)))
	cb.PatchUint32(eqNotInfOff, arm64Bcond(arm64CondNE, int32(eqCCDonePC-eqNotInfOff)))
	// fall through

	// done:
	donePC := cb.Len()
	cb.PatchUint32(doneOff1, arm64B(int32(donePC-doneOff1)))
	cb.PatchUint32(doneOff2, arm64B(int32(donePC-doneOff2)))
	cb.PatchUint32(doneOff3, arm64B(int32(donePC-doneOff3)))

	// Store FPSR
	cb.Emit32(arm64STR_W_imm(2, arm64RegFPUBase, fpuOffFPSR))

	// Store result to integer rd
	if ji.rd != 0 {
		dstReg, mapped := ie64ToARM64Reg(ji.rd)
		if mapped {
			cb.Emit32(arm64MOV(dstReg, 3))
		} else {
			emitStoreSpilledReg(cb, 3, ji.rd)
		}
	}
}

// ===========================================================================
// FPU — Conversions
// ===========================================================================

func emitFCVTIF(cb *CodeBuffer, ji *JITInstr) {
	rsReg := resolveReg(cb, ji.rs, 0)
	cb.Emit32(arm64SCVTF_WS(0, rsReg)) // S0 = float32(int32(rs))
	cb.Emit32(arm64FMOV_StoW(0, 0))
	emitStoreFPReg(cb, 0, ji.rd)
	emitFPCondCodesARM64(cb, ji)
}

func emitSetFPUInvalid(cb *CodeBuffer) {
	cb.Emit32(arm64LDR_W_imm(2, arm64RegFPUBase, fpuOffFPSR))
	emitLoadImm32(cb, 3, IE64_FPU_EX_IO)
	cb.Emit32(arm64ORR_W(2, 2, 3))
	cb.Emit32(arm64STR_W_imm(2, arm64RegFPUBase, fpuOffFPSR))
}

func emitFCVTFI(cb *CodeBuffer, ji *JITInstr) {
	emitLoadFPReg(cb, 0, ji.rs)
	cb.Emit32(arm64FMOV_WtoS(0, 0))

	cb.Emit32(arm64MOV_W(1, 0))
	emitLoadImm32(cb, 2, 0x7FFFFFFF)
	cb.Emit32(arm64AND_W(1, 1, 2))
	emitLoadImm32(cb, 2, 0x7F800000)
	cb.Emit32(arm64CMP_W(1, 2))
	nanOff := cb.Len()
	cb.Emit32(0)

	emitLoadImm32(cb, 1, 0x4F000000)
	cb.Emit32(arm64FMOV_WtoS(1, 1))
	cb.Emit32(arm64FCMP_S(0, 1))
	highOff := cb.Len()
	cb.Emit32(0)

	emitLoadImm32(cb, 1, 0xCF000000)
	cb.Emit32(arm64FMOV_WtoS(1, 1))
	cb.Emit32(arm64FCMP_S(0, 1))
	lowOff := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64FCVTZS_SW(0, 0))
	cb.Emit32(arm64SXTW(0, 0))
	storeOff1 := cb.Len()
	cb.Emit32(0)

	nanPC := cb.Len()
	cb.PatchUint32(nanOff, arm64Bcond(arm64CondHI, int32(nanPC-nanOff)))
	emitLoadImm32(cb, 0, 0)
	emitSetFPUInvalid(cb)
	storeOff2 := cb.Len()
	cb.Emit32(0)

	highPC := cb.Len()
	cb.PatchUint32(highOff, arm64Bcond(arm64CondGT, int32(highPC-highOff)))
	emitLoadImm32(cb, 0, 0x7FFFFFFF)
	cb.Emit32(arm64SXTW(0, 0))
	emitSetFPUInvalid(cb)
	storeOff3 := cb.Len()
	cb.Emit32(0)

	lowPC := cb.Len()
	cb.PatchUint32(lowOff, arm64Bcond(arm64CondMI, int32(lowPC-lowOff)))
	emitLoadImm32(cb, 0, 0x80000000)
	cb.Emit32(arm64SXTW(0, 0))
	emitSetFPUInvalid(cb)

	storePC := cb.Len()
	cb.PatchUint32(storeOff1, arm64B(int32(storePC-storeOff1)))
	cb.PatchUint32(storeOff2, arm64B(int32(storePC-storeOff2)))
	cb.PatchUint32(storeOff3, arm64B(int32(storePC-storeOff3)))
	if ji.rd != 0 {
		dstReg, mapped := ie64ToARM64Reg(ji.rd)
		if mapped {
			cb.Emit32(arm64MOV(dstReg, 0))
		} else {
			emitStoreSpilledReg(cb, 0, ji.rd)
		}
	}
}

// ===========================================================================
// FPU — Memory operations
// ===========================================================================

func emitFLOAD(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	// Compute address: int64(rs) + int64(int32(imm32)). Full 64-bit.
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(0, rsReg, 1))

	// Phase 5 cycle 5.6: MMU-on check → helper exit.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuHelperOff := cb.Len()
	cb.Emit32(0) // CBNZ X1, helperLabel

	cb.Emit32(arm64CMP(0, arm64RegIOStart))
	slowPathOffset := cb.Len()
	cb.Emit32(0) // B.HS → slow path

	// Fast path: direct 32-bit load
	cb.Emit32(arm64LDR_W_reg(2, arm64RegMemBase, 0))
	doneOff1 := cb.Len()
	cb.Emit32(0) // B → done

	// Slow path
	slowPathPC := cb.Len()
	cb.PatchUint32(slowPathOffset, arm64Bcond(arm64CondHS, int32(slowPathPC-slowPathOffset)))

	// High-addr (L=4 bytes) → helper exit.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4)))
	cb.Emit32(arm64SUB_imm(1, 1, 3))
	cb.Emit32(arm64CMP(0, 1))
	inRangeOff := cb.Len()
	cb.Emit32(0) // B.LO in_range
	highHelperOff := cb.Len()
	cb.Emit32(0) // B helperLabel
	inRangePC := cb.Len()
	cb.PatchUint32(inRangeOff, arm64Bcond(arm64CondLO, int32(inRangePC-inRangeOff)))

	cb.Emit32(arm64LSR_imm(1, 0, 8))
	cb.Emit32(arm64LDRB_reg(1, arm64RegIOBitmap, 1))
	cb.Emit32(arm64CBZ(1, 0))
	nonIOOffset := cb.Len() - 4

	// I/O page → helper exit.
	ioHelperOff := cb.Len()
	cb.Emit32(0) // B helperLabel

	// Non-I/O page
	nonIOPC := cb.Len()
	cb.PatchUint32(nonIOOffset, arm64CBZ(1, int32(nonIOPC-nonIOOffset)))
	cb.Emit32(arm64LDR_W_reg(2, arm64RegMemBase, 0))
	doneOff2 := cb.Len()
	cb.Emit32(0) // B done

	// Helper exit
	helperPC := cb.Len()
	cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
	cb.PatchUint32(highHelperOff, arm64B(int32(helperPC-highHelperOff)))
	cb.PatchUint32(ioHelperOff, arm64B(int32(helperPC-ioHelperOff)))
	emitFPMemHelperExitARM64(cb, ji, instrPC, HELPER_FLOAD, uint32(IE64_SIZE_L), br, writtenSoFar)

	// done:
	donePC := cb.Len()
	cb.PatchUint32(doneOff1, arm64B(int32(donePC-doneOff1)))
	cb.PatchUint32(doneOff2, arm64B(int32(donePC-doneOff2)))

	// Store to FP register and set CC (direct-load paths only).
	emitStoreFPReg(cb, 2, ji.rd)
	cb.Emit32(arm64MOV_W(0, 2))
	emitFPCondCodesARM64(cb, ji)
}

func emitFSTORE(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	// Compute address: int64(rs) + int64(int32(imm32)). Full 64-bit.
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(0, rsReg, 1))

	// Load FP source value
	emitLoadFPReg(cb, 3, ji.rd)

	// MMU-on check → helper exit.
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuHelperOff := cb.Len()
	cb.Emit32(0) // CBNZ X1, helperLabel

	cb.Emit32(arm64CMP(0, arm64RegIOStart))
	slowPathOffset := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64STR_W_reg(3, arm64RegMemBase, 0))
	doneOff1 := cb.Len()
	cb.Emit32(0)

	// Slow path
	slowPathPC := cb.Len()
	cb.PatchUint32(slowPathOffset, arm64Bcond(arm64CondHS, int32(slowPathPC-slowPathOffset)))

	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4)))
	cb.Emit32(arm64SUB_imm(1, 1, 3))
	cb.Emit32(arm64CMP(0, 1))
	inRangeOff := cb.Len()
	cb.Emit32(0)
	highHelperOff := cb.Len()
	cb.Emit32(0)
	inRangePC := cb.Len()
	cb.PatchUint32(inRangeOff, arm64Bcond(arm64CondLO, int32(inRangePC-inRangeOff)))

	cb.Emit32(arm64LSR_imm(1, 0, 8))
	cb.Emit32(arm64LDRB_reg(1, arm64RegIOBitmap, 1))
	cb.Emit32(arm64CBZ(1, 0))
	nonIOOffset := cb.Len() - 4

	ioHelperOff := cb.Len()
	cb.Emit32(0)

	nonIOPC := cb.Len()
	cb.PatchUint32(nonIOOffset, arm64CBZ(1, int32(nonIOPC-nonIOOffset)))
	cb.Emit32(arm64STR_W_reg(3, arm64RegMemBase, 0))
	doneOff2 := cb.Len()
	cb.Emit32(0)

	helperPC := cb.Len()
	cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
	cb.PatchUint32(highHelperOff, arm64B(int32(helperPC-highHelperOff)))
	cb.PatchUint32(ioHelperOff, arm64B(int32(helperPC-ioHelperOff)))
	emitFPMemHelperExitARM64(cb, ji, instrPC, HELPER_FSTORE, uint32(IE64_SIZE_L), br, writtenSoFar)

	donePC := cb.Len()
	cb.PatchUint32(doneOff1, arm64B(int32(donePC-doneOff1)))
	cb.PatchUint32(doneOff2, arm64B(int32(donePC-doneOff2)))
}

// emitFPMemHelperExitARM64 writes JITContext fields for an
// FLOAD/FSTORE/DLOAD/DSTORE helper exit. X0 = effective address; FP
// register is read/written by the Go dispatcher directly via cpu.FPU, no
// HelperVal needed. size is IE64_SIZE_L for FLOAD/FSTORE, IE64_SIZE_Q for
// DLOAD/DSTORE.
func emitFPMemHelperExitARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, op uint32, size uint32, br *blockRegs, writtenSoFar uint32) {
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))                                  // X1 = ctx ptr
	cb.Emit32(arm64STR_imm(0, 1, uint32(jitCtxOffHelperAddr/8)))          // HelperAddr = X0
	emitLoadImm32(cb, 2, size)                                            //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffHelperSize/4)))        // HelperSize
	emitLoadImm32(cb, 2, uint32(ji.rd))                                   //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffHelperRd/4)))          // HelperRd
	cb.Emit32(arm64STR_imm(arm64RegIE64SP, 1, uint32(jitCtxOffLiveSP/8))) // LiveSP
	emitLoadImm64(cb, 2, instrPC)                                         //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperPC/8)))            // HelperPC
	emitLoadImm32(cb, 2, op)                                              //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffNeedHelper/4)))        // NeedHelper

	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
}

func emitFPTransHelperExitARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, helper uint32, br *blockRegs, writtenSoFar uint32) {
	cb.Emit32(arm64LDR_imm(1, 31, 96/8))                                  // X1 = ctx ptr
	emitLoadImm32(cb, 2, uint32(ji.opcode))                               //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffHelperSize/4)))        // HelperSize = opcode
	emitLoadImm32(cb, 2, uint32(ji.rd))                                   //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffHelperRd/4)))          // HelperRd
	emitLoadImm64(cb, 2, uint64(ji.rs))                                   //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperAddr/8)))          // HelperAddr = rs
	emitLoadImm64(cb, 2, uint64(ji.rt))                                   //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperVal/8)))           // HelperVal = rt
	cb.Emit32(arm64STR_imm(arm64RegIE64SP, 1, uint32(jitCtxOffLiveSP/8))) // LiveSP
	emitLoadImm64(cb, 2, instrPC)                                         //
	cb.Emit32(arm64STR_imm(2, 1, uint32(jitCtxOffHelperPC/8)))            // HelperPC
	emitLoadImm32(cb, 2, helper)                                          //
	cb.Emit32(arm64STR_W_imm(2, 1, uint32(jitCtxOffNeedHelper/4)))        // NeedHelper

	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
}

func emitDTransHelperExitARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	emitFPTransHelperExitARM64(cb, ji, instrPC, HELPER_DTRANS, br, writtenSoFar)
}

func emitDLOAD(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(0, rsReg, 1)) // X0 = rs + sext(imm32)

	if !isValidDPairReg(ji.rd) {
		emitFPMemHelperExitARM64(cb, ji, instrPC, HELPER_DLOAD, uint32(IE64_SIZE_Q), br, writtenSoFar)
		return
	}
	if ie64CurrentAccessHoisted() {
		cb.Emit32(arm64LDR_reg(2, arm64RegMemBase, 0))
		emitStoreDPairBits(cb, 2, ji.rd)
		cb.Emit32(arm64MOV(0, 2))
		emitFPCondCodes64ARM64(cb, ji)
		return
	}

	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuHelperOff := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64CMP(0, arm64RegIOStart))
	slowPathOffset := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64LDR_reg(2, arm64RegMemBase, 0))
	doneOff1 := cb.Len()
	cb.Emit32(0)

	slowPathPC := cb.Len()
	cb.PatchUint32(slowPathOffset, arm64Bcond(arm64CondHS, int32(slowPathPC-slowPathOffset)))

	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4)))
	cb.Emit32(arm64SUB_imm(1, 1, 7))
	cb.Emit32(arm64CMP(0, 1))
	inRangeOff := cb.Len()
	cb.Emit32(0)
	highHelperOff := cb.Len()
	cb.Emit32(0)
	inRangePC := cb.Len()
	cb.PatchUint32(inRangeOff, arm64Bcond(arm64CondLO, int32(inRangePC-inRangeOff)))

	cb.Emit32(arm64LSR_imm(1, 0, 8))
	cb.Emit32(arm64LDRB_reg(1, arm64RegIOBitmap, 1))
	cb.Emit32(arm64CBZ(1, 0))
	nonIOOffset := cb.Len() - 4

	ioHelperOff := cb.Len()
	cb.Emit32(0)

	nonIOPC := cb.Len()
	cb.PatchUint32(nonIOOffset, arm64CBZ(1, int32(nonIOPC-nonIOOffset)))
	cb.Emit32(arm64LDR_reg(2, arm64RegMemBase, 0))
	doneOff2 := cb.Len()
	cb.Emit32(0)

	helperPC := cb.Len()
	cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
	cb.PatchUint32(highHelperOff, arm64B(int32(helperPC-highHelperOff)))
	cb.PatchUint32(ioHelperOff, arm64B(int32(helperPC-ioHelperOff)))
	emitFPMemHelperExitARM64(cb, ji, instrPC, HELPER_DLOAD, uint32(IE64_SIZE_Q), br, writtenSoFar)

	donePC := cb.Len()
	cb.PatchUint32(doneOff1, arm64B(int32(donePC-doneOff1)))
	cb.PatchUint32(doneOff2, arm64B(int32(donePC-doneOff2)))

	emitStoreDPairBits(cb, 2, ji.rd)
	cb.Emit32(arm64MOV(0, 2))
	emitFPCondCodes64ARM64(cb, ji)
}

func emitDSTORE(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(0, rsReg, 1)) // X0 = rs + sext(imm32)

	if !isValidDPairReg(ji.rd) {
		emitFPMemHelperExitARM64(cb, ji, instrPC, HELPER_DSTORE, uint32(IE64_SIZE_Q), br, writtenSoFar)
		return
	}

	emitLoadDPairBits(cb, 3, ji.rd)
	if ie64CurrentAccessHoisted() {
		cb.Emit32(arm64STR_reg(3, arm64RegMemBase, 0))
		return
	}

	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMMUEnabled/4)))
	mmuHelperOff := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64CMP(0, arm64RegIOStart))
	slowPathOffset := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64STR_reg(3, arm64RegMemBase, 0))
	doneOff1 := cb.Len()
	cb.Emit32(0)

	slowPathPC := cb.Len()
	cb.PatchUint32(slowPathOffset, arm64Bcond(arm64CondHS, int32(slowPathPC-slowPathOffset)))

	cb.Emit32(arm64LDR_imm(1, 31, 96/8))
	cb.Emit32(arm64LDR_W_imm(1, 1, uint32(jitCtxOffMemSize/4)))
	cb.Emit32(arm64SUB_imm(1, 1, 7))
	cb.Emit32(arm64CMP(0, 1))
	inRangeOff := cb.Len()
	cb.Emit32(0)
	highHelperOff := cb.Len()
	cb.Emit32(0)
	inRangePC := cb.Len()
	cb.PatchUint32(inRangeOff, arm64Bcond(arm64CondLO, int32(inRangePC-inRangeOff)))

	cb.Emit32(arm64LSR_imm(1, 0, 8))
	cb.Emit32(arm64LDRB_reg(1, arm64RegIOBitmap, 1))
	cb.Emit32(arm64CBZ(1, 0))
	nonIOOffset := cb.Len() - 4

	ioHelperOff := cb.Len()
	cb.Emit32(0)

	nonIOPC := cb.Len()
	cb.PatchUint32(nonIOOffset, arm64CBZ(1, int32(nonIOPC-nonIOOffset)))
	cb.Emit32(arm64STR_reg(3, arm64RegMemBase, 0))
	doneOff2 := cb.Len()
	cb.Emit32(0)

	helperPC := cb.Len()
	cb.PatchUint32(mmuHelperOff, arm64CBNZ(1, int32(helperPC-mmuHelperOff)))
	cb.PatchUint32(highHelperOff, arm64B(int32(helperPC-highHelperOff)))
	cb.PatchUint32(ioHelperOff, arm64B(int32(helperPC-ioHelperOff)))
	emitFPMemHelperExitARM64(cb, ji, instrPC, HELPER_DSTORE, uint32(IE64_SIZE_Q), br, writtenSoFar)

	donePC := cb.Len()
	cb.PatchUint32(doneOff1, arm64B(int32(donePC-doneOff1)))
	cb.PatchUint32(doneOff2, arm64B(int32(donePC-doneOff2)))
}

func emitStoreDPairBits(cb *CodeBuffer, srcReg byte, fpIdx byte) {
	if v, ok := ie64FPResidentPairARM64(cb, fpIdx); ok {
		cb.Emit32(arm64FMOV_XtoD(v, srcReg))
		return
	}
	base := fpIdx & 0x0E
	cb.Emit32(arm64STR_W_imm(srcReg, arm64RegFPUBase, uint32(base)))
	cb.Emit32(arm64LSR_imm(1, srcReg, 32))
	cb.Emit32(arm64STR_W_imm(1, arm64RegFPUBase, uint32(base+1)))
}

func emitLoadDPairBits(cb *CodeBuffer, dstReg byte, fpIdx byte) {
	if v, ok := ie64FPResidentPairARM64(cb, fpIdx); ok {
		cb.Emit32(arm64FMOV_DtoX(dstReg, v))
		return
	}
	base := fpIdx & 0x0E
	cb.Emit32(arm64LDR_W_imm(dstReg, arm64RegFPUBase, uint32(base)))
	cb.Emit32(arm64LDR_W_imm(1, arm64RegFPUBase, uint32(base+1)))
	cb.Emit32(arm64LSL_imm(1, 1, 32))
	cb.Emit32(arm64ORR(dstReg, dstReg, 1))
}

// ===========================================================================
// FPU — Category D: Double precision
// ===========================================================================
//
// The native set here is exactly the amd64 backend's: DMOV, DADD, DSUB, DMUL,
// DDIV, DINT, DCMP, DCVTIF, DCVTFI. DMOD, DABS, DNEG, DSQRT, FCVTSD and FCVTDS
// still bail on both backends, so the two fallback tables stay identical and
// sdk/docs/IE64_JIT.md can describe one rule rather than two.
//
// Every emitter below is deliberately shaped like its amd64 twin — same clause
// order, same helper split — so the pair can be diffed by eye. The FP64
// exception rules live in IE64FPU (fpu_ie64.go) and are the only spec; six of
// the bugs fixed on this branch came from a backend re-deriving them.
//
// Operands that are NaN or infinite bail to the interpreter (see
// emitDPairNonFiniteBailARM64), which buys the awkward half of the semantics for
// free and, as a side effect, discharges every "!isInf(operand)" and
// "!isNaN(operand)" clause in the rules: they are statically true on the paths
// reached below. That is why the classifiers here only ever inspect the result.

// emitDPairToDReg loads the binary64 bits of dpair fpIdx into dReg. Clobbers X0
// and X1.
func emitDPairToDReg(cb *CodeBuffer, dReg, fpIdx byte) {
	emitLoadDPairBits(cb, 0, fpIdx)
	cb.Emit32(arm64FMOV_XtoD(dReg, 0))
}

// emitDRegToDPair stores dReg's bits to dpair fpIdx and updates the FPSR
// condition codes from the result, unless the block proved that write dead.
// Clobbers X0-X3.
func emitDRegToDPair(cb *CodeBuffer, dReg, fpIdx byte, ccDead bool) {
	cb.Emit32(arm64FMOV_DtoX(0, dReg))
	emitStoreDPairBits(cb, 0, fpIdx)
	if !ccDead {
		// Re-read from dReg rather than trusting X0 to have survived the store.
		// It does today, but the amd64 twin had exactly this bug: its store uses
		// RAX as a shift scratch, so classifying the leftover register read the
		// high word alone and lost the sign and exponent of every result.
		cb.Emit32(arm64FMOV_DtoX(0, dReg))
		emitSetFPCondCodes64(cb)
	}
}

// emitDPairNonFiniteBailARM64 emits, for each named dpair, a test for an
// all-ones exponent and a forward branch taken when the operand is NaN or
// infinite. The caller passes the returned skips to
// patchFP64BailToInterpreterARM64. Clobbers X0-X2.
func emitDPairNonFiniteBailARM64(cb *CodeBuffer, regs ...byte) []armSkip {
	var skips []armSkip
	for _, fpIdx := range regs {
		emitLoadDPairBits(cb, 0, fpIdx)
		cb.Emit32(arm64LSR_imm(2, 0, 52))
		cb.Emit32(arm64AND_imm(2, 2, 0, 10, 1)) // X2 &= 0x7FF
		cb.Emit32(arm64CMP_imm(2, 0x7FF))
		off := cb.Len()
		cb.Emit32(0)
		skips = append(skips, armSkip{off: off, cond: arm64CondEQ})
	}
	return skips
}

// patchFP64BailToInterpreterARM64 lands the bail branches after the fast path,
// so the common case falls straight through and never jumps.
func patchFP64BailToInterpreterARM64(cb *CodeBuffer, skips []armSkip, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	if len(skips) == 0 {
		return
	}
	doneOff := cb.Len()
	cb.Emit32(0)
	patchArmSkips(cb, skips, cb.Len())
	emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
	cb.PatchUint32(doneOff, arm64B(int32(cb.Len()-doneOff)))
}

// emitSetFPUExceptionARM64 ORs flag into FPSR. Sticky: never clears a flag that
// is already set. Clobbers X3 and X4.
func emitSetFPUExceptionARM64(cb *CodeBuffer, flag uint32) {
	cb.Emit32(arm64LDR_W_imm(3, arm64RegFPUBase, fpuOffFPSR))
	emitLoadImm32(cb, 4, flag)
	cb.Emit32(arm64ORR_W(3, 3, 4))
	cb.Emit32(arm64STR_W_imm(3, arm64RegFPUBase, fpuOffFPSR))
}

// emitDPairZeroSkipARM64 tests whether dpair fpIdx is +/-0.0 and returns a
// forward branch taken on cond: pass arm64CondEQ for "is zero", arm64CondNE for
// "is not zero". Clobbers X1 and X2.
func emitDPairZeroSkipARM64(cb *CodeBuffer, fpIdx, cond byte) armSkip {
	emitLoadDPairBits(cb, 2, fpIdx)
	cb.Emit32(arm64LSL_imm(2, 2, 1)) // drop the sign bit: 0 iff +/-0.0
	cb.Emit32(arm64CMP_imm(2, 0))
	off := cb.Len()
	cb.Emit32(0)
	return armSkip{off: off, cond: cond}
}

// emitSetDPResultInfOrNaNFlagsARM64 raises IO for a NaN result and, when
// overflowAllowed, OE for an infinite one. Both operands are finite on every
// path that reaches this, so a special result can only have been produced by the
// operation itself, which is precisely what the OE and IO rules ask about.
//
// overflowAllowed is false only for a division by zero, where an infinite
// quotient is the divide-by-zero case and raises DZ alone.
func emitSetDPResultInfOrNaNFlagsARM64(cb *CodeBuffer, dReg byte, overflowAllowed bool) {
	cb.Emit32(arm64FMOV_DtoX(0, dReg))
	cb.Emit32(arm64LSR_imm(2, 0, 52))
	cb.Emit32(arm64AND_imm(2, 2, 0, 10, 1)) // X2 &= 0x7FF
	cb.Emit32(arm64CMP_imm(2, 0x7FF))
	notSpecialOff := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64LSL_imm(2, 0, 12)) // fraction; sign and exponent shifted out
	nanOff := cb.Len()
	cb.Emit32(0)

	if overflowAllowed {
		emitSetFPUExceptionARM64(cb, IE64_FPU_EX_OE)
	}
	doneOff := cb.Len()
	cb.Emit32(0)

	nanPC := cb.Len()
	cb.PatchUint32(nanOff, arm64CBNZ(2, int32(nanPC-nanOff)))
	emitSetFPUExceptionARM64(cb, IE64_FPU_EX_IO)

	donePC := cb.Len()
	cb.PatchUint32(notSpecialOff, arm64Bcond(arm64CondNE, int32(donePC-notSpecialOff)))
	cb.PatchUint32(doneOff, arm64B(int32(donePC-doneOff)))
}

// emitSetDPResultUnderflowIfZeroARM64 raises UE when a zero result came from two
// non-zero operands. DDIV's rule adds "&& !isInf(t)", which is already true
// here: an infinite divisor would have bailed.
func emitSetDPResultUnderflowIfZeroARM64(cb *CodeBuffer, dReg byte, ji *JITInstr) {
	cb.Emit32(arm64FMOV_DtoX(0, dReg))
	cb.Emit32(arm64LSL_imm(2, 0, 1))
	notZeroOff := cb.Len()
	cb.Emit32(0)

	skips := []armSkip{
		emitDPairZeroSkipARM64(cb, ji.rs, arm64CondEQ),
		emitDPairZeroSkipARM64(cb, ji.rt, arm64CondEQ),
	}
	emitSetFPUExceptionARM64(cb, IE64_FPU_EX_UE)

	donePC := cb.Len()
	cb.PatchUint32(notZeroOff, arm64CBNZ(2, int32(donePC-notZeroOff)))
	patchArmSkips(cb, skips, donePC)
}

// emitSetDPDivideByZeroIfNeededARM64 raises DZ for a non-zero numerator over a
// zero divisor. IE64FPU.DDIV also requires !isNaN(s), which a NaN numerator
// would have bailed on.
func emitSetDPDivideByZeroIfNeededARM64(cb *CodeBuffer, ji *JITInstr) {
	skips := []armSkip{
		emitDPairZeroSkipARM64(cb, ji.rt, arm64CondNE),
		emitDPairZeroSkipARM64(cb, ji.rs, arm64CondEQ),
	}
	emitSetFPUExceptionARM64(cb, IE64_FPU_EX_DZ)
	patchArmSkips(cb, skips, cb.Len())
}

func emitDMOV_ARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	if !isValidDPairReg(ji.rd) || !isValidDPairReg(ji.rs) {
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
		return
	}
	// A pure bit copy: the interpreter's DMOV sets neither condition codes nor
	// exception flags, so there is nothing to classify and NaN payloads pass
	// through untouched.
	emitLoadDPairBits(cb, 0, ji.rs)
	emitStoreDPairBits(cb, 0, ji.rd)
}

func emitDPBinaryARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32, fpOp func(dd, dn, dm byte) uint32) {
	if !isValidDPairReg(ji.rd) || !isValidDPairReg(ji.rs) || !isValidDPairReg(ji.rt) {
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
		return
	}
	skips := emitDPairNonFiniteBailARM64(cb, ji.rs, ji.rt)

	emitDPairToDReg(cb, 0, ji.rs)
	emitDPairToDReg(cb, 1, ji.rt)
	if ji.opcode == OP_DDIV {
		// Before the divide: IE64FPU.DDIV tests the operands, not the quotient.
		emitSetDPDivideByZeroIfNeededARM64(cb, ji)
	}
	cb.Emit32(fpOp(2, 0, 1))
	if ji.opcode == OP_DDIV {
		tZeroOff := emitDPairZeroSkipARM64(cb, ji.rt, arm64CondNE)
		emitSetDPResultInfOrNaNFlagsARM64(cb, 2, false)
		doneOff := cb.Len()
		cb.Emit32(0)
		patchArmSkips(cb, []armSkip{tZeroOff}, cb.Len())
		emitSetDPResultInfOrNaNFlagsARM64(cb, 2, true)
		cb.PatchUint32(doneOff, arm64B(int32(cb.Len()-doneOff)))
	} else {
		emitSetDPResultInfOrNaNFlagsARM64(cb, 2, true)
	}
	if ji.opcode == OP_DMUL || ji.opcode == OP_DDIV {
		emitSetDPResultUnderflowIfZeroARM64(cb, 2, ji)
	}
	emitDRegToDPair(cb, 2, ji.rd, ji.fpsrCCDead)
	patchFP64BailToInterpreterARM64(cb, skips, ji, instrPC, br, writtenSoFar)
}

func emitDINT_ARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	if !isValidDPairReg(ji.rd) || !isValidDPairReg(ji.rs) {
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
		return
	}
	emitDPairToDReg(cb, 0, ji.rs)

	// Dispatch on the guest rounding mode held in FPCR, not the host's: the
	// interpreter picks its rounding function from these two bits explicitly, so
	// FRINTI (round per host FPCR) would answer a different question. DINT raises
	// no exception flags and needs no operand check.
	cb.Emit32(arm64LDR_W_imm(2, arm64RegFPUBase, fpuOffFPCR))
	emitLoadImm32(cb, 3, 0x03)
	cb.Emit32(arm64AND_W(2, 2, 3))

	modeIs := func(mode uint8) armSkip {
		emitLoadImm32(cb, 3, uint32(mode))
		cb.Emit32(arm64CMP_W(2, 3))
		off := cb.Len()
		cb.Emit32(0)
		return armSkip{off: off, cond: arm64CondEQ}
	}
	nearestSkip := modeIs(IE64_FPU_RND_NEAREST)
	truncSkip := modeIs(IE64_FPU_RND_ZERO)
	floorSkip := modeIs(IE64_FPU_RND_FLOOR)

	// Fallthrough is ceil, matching the interpreter's remaining mode.
	cb.Emit32(arm64FRINTP_D(1, 0))
	done1Off := cb.Len()
	cb.Emit32(0)

	patchArmSkips(cb, []armSkip{floorSkip}, cb.Len())
	cb.Emit32(arm64FRINTM_D(1, 0))
	done2Off := cb.Len()
	cb.Emit32(0)

	patchArmSkips(cb, []armSkip{truncSkip}, cb.Len())
	cb.Emit32(arm64FRINTZ_D(1, 0))
	done3Off := cb.Len()
	cb.Emit32(0)

	patchArmSkips(cb, []armSkip{nearestSkip}, cb.Len())
	cb.Emit32(arm64FRINTN_D(1, 0))

	donePC := cb.Len()
	cb.PatchUint32(done1Off, arm64B(int32(donePC-done1Off)))
	cb.PatchUint32(done2Off, arm64B(int32(donePC-done2Off)))
	cb.PatchUint32(done3Off, arm64B(int32(donePC-done3Off)))

	emitDRegToDPair(cb, 1, ji.rd, ji.fpsrCCDead)
}

func emitDCMP_ARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	if !isValidDPairReg(ji.rs) || !isValidDPairReg(ji.rt) {
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
		return
	}
	skips := emitDPairNonFiniteBailARM64(cb, ji.rs, ji.rt)

	emitDPairToDReg(cb, 0, ji.rs)
	emitDPairToDReg(cb, 1, ji.rt)

	// Seed the accumulator with the surviving sticky flags before the compare:
	// IE64FPU.DCMP clears the CC field and keeps the exception flags.
	cb.Emit32(arm64LDR_W_imm(2, arm64RegFPUBase, fpuOffFPSR))
	cb.Emit32(arm64UBFX_W(2, 2, 0, 4))

	cb.Emit32(arm64FCMP_D(0, 1))
	ltOff := cb.Len()
	cb.Emit32(0)
	eqOff := cb.Len()
	cb.Emit32(0)

	// Greater than (fallthrough). Non-finite operands bail above, so neither
	// IE64FPU.DCMP's +Inf CC_I rule nor its unordered path can be reached: the
	// only outcomes left are less-than, equal and greater-than between finites.
	cb.Emit32(arm64MOVZ_W(3, 1, 0)) // result = 1
	done1Off := cb.Len()
	cb.Emit32(0)

	ltPC := cb.Len()
	cb.PatchUint32(ltOff, arm64Bcond(arm64CondMI, int32(ltPC-ltOff)))
	emitLoadImm32(cb, 3, 0xFFFFFFFF)
	cb.Emit32(arm64SXTW(3, 3)) // X3 = -1
	emitLoadImm32(cb, 4, IE64_FPU_CC_N)
	cb.Emit32(arm64ORR_W(2, 2, 4))
	done2Off := cb.Len()
	cb.Emit32(0)

	eqPC := cb.Len()
	cb.PatchUint32(eqOff, arm64Bcond(arm64CondEQ, int32(eqPC-eqOff)))
	cb.Emit32(arm64MOVZ_W(3, 0, 0)) // result = 0
	emitLoadImm32(cb, 4, IE64_FPU_CC_Z)
	cb.Emit32(arm64ORR_W(2, 2, 4))

	donePC := cb.Len()
	cb.PatchUint32(done1Off, arm64B(int32(donePC-done1Off)))
	cb.PatchUint32(done2Off, arm64B(int32(donePC-done2Off)))

	cb.Emit32(arm64STR_W_imm(2, arm64RegFPUBase, fpuOffFPSR))
	if ji.rd != 0 {
		dstReg, mapped := ie64ToARM64Reg(ji.rd)
		if mapped {
			cb.Emit32(arm64MOV(dstReg, 3))
		} else {
			emitStoreSpilledReg(cb, 3, ji.rd)
		}
	}
	patchFP64BailToInterpreterARM64(cb, skips, ji, instrPC, br, writtenSoFar)
}

func emitDCVTIF_ARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	if !isValidDPairReg(ji.rd) {
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
		return
	}
	// int64 -> float64 cannot raise anything: every int64 has a float64 image,
	// exactly or rounded. The interpreter agrees, and only sets condition codes.
	rsReg := resolveReg(cb, ji.rs, 0)
	cb.Emit32(arm64SCVTF_XD(0, rsReg))
	emitDRegToDPair(cb, 0, ji.rd, ji.fpsrCCDead)
}

func emitDCVTFI_ARM64(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	if !isValidDPairReg(ji.rs) {
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
		return
	}
	skips := emitDPairNonFiniteBailARM64(cb, ji.rs)
	emitDPairToDReg(cb, 0, ji.rs)

	// Saturate where IE64FPU.DCVTFI saturates. FCVTZS clamps on its own, but
	// silently, and the IO flag on an out-of-range operand is ours to raise. The
	// NaN arm of the interpreter's rule is unreachable: a NaN operand bails.
	emitLoadImm64(cb, 2, math.Float64bits(fp64MaxInt64))
	cb.Emit32(arm64FMOV_XtoD(1, 2))
	cb.Emit32(arm64FCMP_D(0, 1))
	highOff := cb.Len()
	cb.Emit32(0)

	emitLoadImm64(cb, 2, math.Float64bits(fp64MinInt64))
	cb.Emit32(arm64FMOV_XtoD(1, 2))
	cb.Emit32(arm64FCMP_D(0, 1))
	lowOff := cb.Len()
	cb.Emit32(0)

	cb.Emit32(arm64FCVTZS_DX(0, 0))
	storeOff := cb.Len()
	cb.Emit32(0)

	highPC := cb.Len()
	cb.PatchUint32(highOff, arm64Bcond(arm64CondGT, int32(highPC-highOff)))
	emitLoadImm64(cb, 0, uint64(math.MaxInt64))
	emitSetFPUInvalid(cb)
	highDoneOff := cb.Len()
	cb.Emit32(0)

	lowPC := cb.Len()
	cb.PatchUint32(lowOff, arm64Bcond(arm64CondMI, int32(lowPC-lowOff)))
	emitLoadImm64(cb, 0, uint64(1)<<63)
	emitSetFPUInvalid(cb)

	storePC := cb.Len()
	cb.PatchUint32(storeOff, arm64B(int32(storePC-storeOff)))
	cb.PatchUint32(highDoneOff, arm64B(int32(storePC-highDoneOff)))

	if ji.rd != 0 {
		dstReg, mapped := ie64ToARM64Reg(ji.rd)
		if mapped {
			cb.Emit32(arm64MOV(dstReg, 0))
		} else {
			emitStoreSpilledReg(cb, 0, ji.rd)
		}
	}
	patchFP64BailToInterpreterARM64(cb, skips, ji, instrPC, br, writtenSoFar)
}

// ===========================================================================
// FPU — Category C: Transcendentals (bail to interpreter)
// ===========================================================================

func emitFPUBail(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	cb.Emit32(arm64LDR_imm(0, 31, 96/8))
	emitLoadImm32(cb, 1, 1)
	cb.Emit32(arm64STR_W_imm(1, 0, uint32(jitCtxOffNeedIOFallback/4)))
	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
}

func emitBailToInterpreter(cb *CodeBuffer, ji *JITInstr, instrPC uint64, br *blockRegs, writtenSoFar uint32) {
	emitFPUBail(cb, ji, instrPC, br, writtenSoFar)
}

// ===========================================================================
// IE64 Region Compilation
// ===========================================================================

func ie64NativeObservedRegion(observed *ie64ObservedRegion) *ie64Region {
	if observed == nil {
		return nil
	}
	r := &ie64Region{entryPC: observed.entryPC, observed: observed.blocks}
	for i := range observed.blocks {
		r.blockPCs = append(r.blockPCs, observed.blocks[i].pc)
		r.blocks = append(r.blocks, observed.blocks[i].instrs)
	}
	return r
}

const (
	ie64ARM64RegionMaxBlocks       = 8
	ie64ARM64RegionMaxInstructions = 512
)

func ie64FormRegion(hotPC uint64, memory []byte) *ie64Region {
	pc := hotPC
	totalInstrs := 0
	visited := make(map[uint64]struct{})
	region := &ie64Region{entryPC: hotPC}
	for len(region.blockPCs) < ie64ARM64RegionMaxBlocks && totalInstrs < ie64ARM64RegionMaxInstructions {
		if _, seen := visited[pc]; seen || pc >= uint64(len(memory)) {
			break
		}
		instrs := scanBlock(memory, pc)
		if len(instrs) == 0 || needsFallback(instrs) {
			break
		}
		if len(region.blocks) > 0 && totalInstrs+len(instrs) > ie64ARM64RegionMaxInstructions {
			break
		}
		for _, ji := range instrs {
			if ji.fusedFlag != 0 {
				return nil
			}
		}
		visited[pc] = struct{}{}
		region.blockPCs = append(region.blockPCs, pc)
		region.blocks = append(region.blocks, instrs)
		totalInstrs += len(instrs)

		last := &instrs[len(instrs)-1]
		if !isBlockTerminator(last.opcode) || last.fusedFlag&ie64FusedRTSLeafReturn != 0 {
			break
		}
		instrPC := pc + uint64(last.pcOffset)
		target, ok := ie64ResolveTerminatorTarget(last.opcode, last.rs, last.imm32, instrPC)
		if !ok {
			break
		}
		pc = target
	}
	if len(region.blocks) < 2 {
		return nil
	}
	return region
}

func ie64FormRegionMMU(cpu *CPU64, hotPC uint64) *ie64Region {
	if cpu == nil || cpu.bus == nil || !cpu.mmuEnabled {
		return nil
	}
	pc := hotPC
	totalInstrs := 0
	visited := make(map[uint64]struct{})
	region := &ie64Region{entryPC: hotPC}
	memLen := uint64(len(cpu.memory))
	for len(region.blockPCs) < ie64ARM64RegionMaxBlocks && totalInstrs < ie64ARM64RegionMaxInstructions {
		if _, seen := visited[pc]; seen {
			break
		}
		pcPhys, fault, _ := cpu.translateAddr(pc, ACCESS_EXEC)
		if fault {
			break
		}
		pageEnd := (pcPhys &^ uint64(MMU_PAGE_MASK)) + MMU_PAGE_SIZE
		highPhys := memLen < IE64_INSTR_SIZE || pcPhys > memLen-IE64_INSTR_SIZE
		var instrs []JITInstr
		if !highPhys && pageEnd <= memLen {
			instrs = scanBlockWithLimit(cpu.memory, pcPhys, pageEnd)
		} else {
			instrs = scanBlockBusWithLimit(cpu.bus, pcPhys, pageEnd)
		}
		if len(instrs) == 0 || needsFallback(instrs) {
			break
		}
		for _, ji := range instrs {
			if ji.fusedFlag != 0 {
				return nil
			}
		}
		if len(region.blocks) > 0 && totalInstrs+len(instrs) > ie64ARM64RegionMaxInstructions {
			break
		}
		markIE64MMUBails(instrs)
		visited[pc] = struct{}{}
		region.blockPCs = append(region.blockPCs, pc)
		region.blocks = append(region.blocks, instrs)
		totalInstrs += len(instrs)

		last := &instrs[len(instrs)-1]
		if !isBlockTerminator(last.opcode) || last.fusedFlag&ie64FusedRTSLeafReturn != 0 {
			break
		}
		instrPC := pc + uint64(last.pcOffset)
		target, ok := ie64ResolveTerminatorTarget(last.opcode, last.rs, last.imm32, instrPC)
		if !ok {
			break
		}
		pc = target
	}
	if len(region.blocks) < 2 {
		return nil
	}
	return region
}

func ie64CompileRegion(region *ie64Region, execMem *ExecMem, memory []byte) (*JITBlock, error) {
	if region == nil || len(region.blocks) < 2 {
		return nil, errIE64RegionTooSmall
	}

	var allInstrs []JITInstr
	for _, block := range region.blocks {
		allInstrs = append(allInstrs, block...)
	}
	br := analyzeBlockRegs(allInstrs)
	br.hasBackwardBranch = true
	regionLoopPlan, _ := ie64AnalyseRegionLoop(region)
	prevLoopPlan := ie64ActiveLoopPlan
	ie64ActiveLoopPlan = regionLoopPlan
	defer func() { ie64ActiveLoopPlan = prevLoopPlan }()
	prevFoldPlan := ie64ActiveFoldPlan
	ie64ActiveFoldPlan = ie64AnalyseRegionConstFold(region.blocks, region.blockPCs)
	defer func() { ie64ActiveFoldPlan = prevFoldPlan }()

	cb := NewCodeBuffer(len(allInstrs) * 256)
	if br.hasFPU && ie64FPResidencyEnabled() {
		if plan, ok := ie64BuildBlockFPPlan(allInstrs); ok {
			cb.fpPlan = &plan
		}
	}
	emitPrologue(cb, region.entryPC, &br)

	pcToBlock := make(map[uint64]int, len(region.blockPCs))
	for i, pc := range region.blockPCs {
		pcToBlock[pc] = i
	}
	blockLabels := make([]int, len(region.blocks))
	loopBodyLabels := make([]int, len(region.blocks))
	instrCountAtBlock := make([]int, len(region.blocks))
	type forwardFixup struct {
		branchOffset int
		targetBlock  int
	}
	var fixups []forwardFixup
	var coldStubs []ie64ColdExitStubARM64
	totalInstrCount := 0
	regionWrittenSoFar := uint32(0)
	for blockIndex, block := range region.blocks {
		blockLabels[blockIndex] = cb.Len()
		instrCountAtBlock[blockIndex] = totalInstrCount
		cb.instrCountBase = uint32(totalInstrCount)
		cb.pendingFPCC = ie64FPCCPending{}
		ie64MarkFPSRCCDead(block)
		instrOffsets := make([]int, len(block))
		writtenSoFar := regionWrittenSoFar
		if regionLoopPlan != nil && region.blockPCs[blockIndex] == regionLoopPlan.headPC && len(regionLoopPlan.accesses) != 0 {
			saved := cb.instrCountBase
			cb.instrCountBase = 0
			emitIE64LoopPrecheckARM64(cb, &br, writtenSoFar, regionLoopPlan)
			cb.instrCountBase = saved
		}
		loopBodyLabels[blockIndex] = cb.Len()
		for i := range block {
			ji := &block[i]
			ie64CurrentLoopInstr = totalInstrCount + i
			isLast := i == len(block)-1
			if isLast && blockIndex < len(region.observed) && region.observed[blockIndex].kind == ie64ObservedIndirectJMP {
				targetBlock, internal := pcToBlock[region.observed[blockIndex].predictedTarget]
				if !internal || ji.opcode != OP_JMP || ji.rs == 0 {
					return nil, errIE64ObservedInvalid
				}
				instrOffsets[i] = cb.Len()
				rsReg := resolveReg(cb, ji.rs, 0)
				emitLoadImm32(cb, 1, ji.imm32)
				cb.Emit32(arm64SXTW(1, 1))
				cb.Emit32(arm64ADD(arm64RegIE64PC, rsReg, 1))
				emitLoadImm64(cb, 0, region.observed[blockIndex].predictedTarget)
				cb.Emit32(arm64CMP(arm64RegIE64PC, 0))
				matchOffset := cb.Len()
				cb.Emit32(0)
				emitStoreRetCount(cb, uint32(i+1), &br)
				emitEpilogue(cb, br.written, br.used)
				cb.PatchUint32(matchOffset, arm64Bcond(arm64CondEQ, int32(cb.Len()-matchOffset)))
				if targetBlock <= blockIndex {
					bodySize := uint32(totalInstrCount + i + 1 - instrCountAtBlock[targetBlock])
					cb.Emit32(arm64ADD_imm(arm64RegLoopCount, arm64RegLoopCount, bodySize))
					cb.Emit32(arm64CMP_imm(arm64RegLoopCount, jitBudget))
					budgetExitOffset := cb.Len()
					cb.Emit32(0)
					cb.Emit32(arm64B(int32(loopBodyLabels[targetBlock] - cb.Len())))
					cb.PatchUint32(budgetExitOffset, arm64Bcond(arm64CondHS, int32(cb.Len()-budgetExitOffset)))
					cb.Emit32(arm64SUB_imm(arm64RegLoopCount, arm64RegLoopCount, bodySize))
					emitPackedPCAndCount(cb, region.observed[blockIndex].predictedTarget, uint32(i+1), &br)
					emitEpilogue(cb, br.written, br.used)
				} else {
					branchOffset := cb.Len()
					cb.Emit32(0)
					fixups = append(fixups, forwardFixup{branchOffset: branchOffset, targetBlock: targetBlock})
				}
				writtenSoFar |= instrWrittenRegs(ji)
				continue
			}
			if isLast && blockIndex < len(region.observed) && region.observed[blockIndex].kind == ie64ObservedConditional {
				targetBlock, internal := pcToBlock[region.observed[blockIndex].hotTarget]
				cond, ok := ie64ARM64Cond(ji.opcode)
				if !internal || !ok {
					return nil, errIE64ObservedInvalid
				}
				instrOffsets[i] = cb.Len()
				rsReg := resolveReg(cb, ji.rs, 0)
				rtReg := resolveReg(cb, ji.rt, 1)
				cb.Emit32(arm64CMP(rsReg, rtReg))
				if targetBlock == blockIndex+1 && ie64ColdExitOutlineEligible(region.observed) {
					// Outlined cold exit: invert the host condition so the
					// adjacent forward hot successor is reached by
					// fall-through; the cold-exit sequence is emitted after
					// all normal region bodies and exits, reached only
					// through this conditional fixup. AArch64 condition
					// codes pair by xor-1.
					coldStubs = append(coldStubs, ie64ColdExitStubARM64{
						bcondOff:     cb.Len(),
						cond:         cond ^ 1,
						coldTarget:   region.observed[blockIndex].coldTarget,
						count:        uint32(i + 1),
						countBase:    cb.instrCountBase,
						writtenSoFar: writtenSoFar,
						pendingFPCC:  cb.pendingFPCC,
					})
					cb.Emit32(0)
					// Fall through into the next emitted block. The
					// block-end flush below settles any sunk CC on this
					// edge, exactly like the previous explicit hot edge.
					writtenSoFar |= instrWrittenRegs(ji)
					continue
				}
				hotOffset := cb.Len()
				cb.Emit32(0)
				emitPackedPCAndCount(cb, region.observed[blockIndex].coldTarget, uint32(i+1), &br)
				emitEpilogue(cb, writtenSoFar, br.used)
				cb.PatchUint32(hotOffset, arm64Bcond(cond, int32(cb.Len()-hotOffset)))
				emitMaterializeFPCCARM64(cb)
				if targetBlock <= blockIndex {
					bodySize := uint32(totalInstrCount + i + 1 - instrCountAtBlock[targetBlock])
					cb.Emit32(arm64ADD_imm(arm64RegLoopCount, arm64RegLoopCount, bodySize))
					cb.Emit32(arm64CMP_imm(arm64RegLoopCount, jitBudget))
					budgetExitOffset := cb.Len()
					cb.Emit32(0)
					cb.Emit32(arm64B(int32(loopBodyLabels[targetBlock] - cb.Len())))
					cb.PatchUint32(budgetExitOffset, arm64Bcond(arm64CondHS, int32(cb.Len()-budgetExitOffset)))
					cb.Emit32(arm64SUB_imm(arm64RegLoopCount, arm64RegLoopCount, bodySize))
					emitPackedPCAndCount(cb, region.observed[blockIndex].hotTarget, uint32(i+1), &br)
					emitEpilogue(cb, br.written, br.used)
				} else {
					branchOffset := cb.Len()
					cb.Emit32(0)
					fixups = append(fixups, forwardFixup{branchOffset: branchOffset, targetBlock: targetBlock})
				}
				writtenSoFar |= instrWrittenRegs(ji)
				continue
			}
			if isLast && (ji.opcode == OP_BRA || ji.opcode == OP_JMP) {
				instrPC := region.blockPCs[blockIndex] + uint64(ji.pcOffset)
				if target, ok := ie64ResolveTerminatorTarget(ji.opcode, ji.rs, ji.imm32, instrPC); ok {
					if targetBlock, internal := pcToBlock[target]; internal {
						instrOffsets[i] = cb.Len()
						emitMaterializeFPCCARM64(cb)
						if targetBlock <= blockIndex {
							bodySize := uint32(totalInstrCount + i + 1 - instrCountAtBlock[targetBlock])
							cb.Emit32(arm64ADD_imm(arm64RegLoopCount, arm64RegLoopCount, bodySize))
							cb.Emit32(arm64CMP_imm(arm64RegLoopCount, jitBudget))
							budgetExitOffset := cb.Len()
							cb.Emit32(0)
							cb.Emit32(arm64B(int32(loopBodyLabels[targetBlock] - cb.Len())))
							budgetExitPC := cb.Len()
							cb.PatchUint32(budgetExitOffset, arm64Bcond(arm64CondHS, int32(budgetExitPC-budgetExitOffset)))
							cb.Emit32(arm64SUB_imm(arm64RegLoopCount, arm64RegLoopCount, bodySize))
							emitPackedPCAndCount(cb, target, uint32(i+1), &br)
							emitEpilogue(cb, br.written, br.used)
						} else {
							branchOffset := cb.Len()
							cb.Emit32(0)
							fixups = append(fixups, forwardFixup{branchOffset: branchOffset, targetBlock: targetBlock})
						}
						writtenSoFar |= instrWrittenRegs(ji)
						continue
					}
				}
			}
			instrOffsets[i] = cb.Len()
			if f := ie64FoldEntryAt(totalInstrCount + i); f.folded {
				emitFoldedConst(cb, ji.rd, f.value)
			} else {
				emitInstruction(cb, ji, region.blockPCs[blockIndex], isLast, &br, writtenSoFar, i, instrOffsets)
			}
			writtenSoFar |= instrWrittenRegs(ji)
		}
		if !isBlockTerminator(block[len(block)-1].opcode) {
			emitMaterializeFPCCARM64(cb)
		}
		cb.pendingFPCC = ie64FPCCPending{}
		regionWrittenSoFar = writtenSoFar
		totalInstrCount += len(block)
	}
	for _, fixup := range fixups {
		cb.PatchUint32(fixup.branchOffset, arm64B(int32(blockLabels[fixup.targetBlock]-fixup.branchOffset)))
	}

	cb.instrCountBase = 0
	lastBlock := region.blocks[len(region.blocks)-1]
	lastInstr := &lastBlock[len(lastBlock)-1]
	if !isBlockTerminator(lastInstr.opcode) {
		endPC := region.blockPCs[len(region.blocks)-1] + uint64(lastInstr.pcOffset) + IE64_INSTR_SIZE
		emitPackedPCAndCount(cb, endPC, uint32(totalInstrCount), &br)
		emitEpilogue(cb, br.written, br.used)
	}

	// Outlined cold exit stubs: each reached only through its conditional
	// fixup. Every normal path above has already terminated or branched.
	// Each stub replays the exact cold-exit sequence with the state captured
	// at its conditional: count base, written-so-far spill mask and pending
	// FPCC.
	for s := range coldStubs {
		coldStub := &coldStubs[s]
		cb.PatchUint32(coldStub.bcondOff, arm64Bcond(coldStub.cond, int32(cb.Len()-coldStub.bcondOff)))
		cb.instrCountBase = coldStub.countBase
		cb.pendingFPCC = coldStub.pendingFPCC
		emitPackedPCAndCount(cb, coldStub.coldTarget, coldStub.count, &br)
		emitEpilogue(cb, coldStub.writtenSoFar, br.used)
		cb.instrCountBase = 0
		cb.pendingFPCC = ie64FPCCPending{}
		ie64ColdExitOutlines.Add(1)
	}

	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, err
	}
	covered := make([][2]uint64, 0, len(region.blocks))
	for i, block := range region.blocks {
		last := &block[len(block)-1]
		covered = append(covered, [2]uint64{
			region.blockPCs[i],
			region.blockPCs[i] + uint64(last.pcOffset) + IE64_INSTR_SIZE,
		})
	}
	endPC := covered[len(covered)-1][1]
	return &JITBlock{
		startPC:       region.entryPC,
		endPC:         endPC,
		instrCount:    totalInstrCount,
		execAddr:      addr,
		execSize:      len(code),
		coveredRanges: covered,
	}, nil
}

var errIE64RegionTooSmall = errors.New("ie64CompileRegion: region has fewer than 2 blocks")

// ie64ColdExitStubARM64 captures everything the outlined cold-exit stub
// needs to replay the existing cold-exit sequence after all normal region
// bodies and exits.
type ie64ColdExitStubARM64 struct {
	bcondOff     int
	cond         byte
	coldTarget   uint64
	count        uint32
	countBase    uint32
	writtenSoFar uint32
	pendingFPCC  ie64FPCCPending
}

func ie64ARM64Cond(op byte) (byte, bool) {
	switch op {
	case OP_BEQ:
		return arm64CondEQ, true
	case OP_BNE:
		return arm64CondNE, true
	case OP_BLT:
		return arm64CondLT, true
	case OP_BGE:
		return arm64CondGE, true
	case OP_BGT:
		return arm64CondGT, true
	case OP_BLE:
		return arm64CondLE, true
	case OP_BHI:
		return arm64CondHI, true
	case OP_BLS:
		return arm64CondLS, true
	}
	return 0, false
}
