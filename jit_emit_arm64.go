// jit_emit_arm64.go - ARM64 native code emitter for IE64 JIT compiler

//go:build arm64 && linux

package main

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
// X12-X26  IE64 R1-R15 (15 most-used GPRs)
// X27      IE64 R31 (SP) — always resident
// X28      Current IE64 PC
// XZR      IE64 R0 (reads=0, writes=discard)
// X29/X30  Go FP/LR — saved/restored

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

	// IE64 R1-R15 mapped to ARM64 X12-X26
	arm64FirstMapped = 12
	arm64LastMapped  = 26
	ie64FirstMapped  = 1
	ie64LastMapped   = 15
)

// ie64ToARM64Reg maps an IE64 register index (0-31) to an ARM64 register.
// Returns the ARM64 register number and whether it's a "mapped" register
// (resident in an ARM64 register) vs a "spilled" register (in memory).
func ie64ToARM64Reg(ie64Reg byte) (arm64Reg byte, mapped bool) {
	if ie64Reg == 0 {
		return 31, true // XZR
	}
	if ie64Reg >= ie64FirstMapped && ie64Reg <= ie64LastMapped {
		return arm64FirstMapped + (ie64Reg - ie64FirstMapped), true
	}
	if ie64Reg == 31 {
		return arm64RegIE64SP, true
	}
	return 0, false // spilled: R16-R30
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

// smull Xd, Wn, Wm (signed multiply long — W×W→X)
func arm64SMULL(rd, rn, rm byte) uint32 {
	return 0x9B207C00 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
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

// emitDynamicCount packs X7 + staticCount into upper 32 bits of X28.
// Used at exit points in blocks with backward branches.
func emitDynamicCount(cb *CodeBuffer, staticCount uint32) {
	cb.Emit32(arm64ADD_imm(0, arm64RegLoopCount, staticCount))
	cb.Emit32(arm64LSL_imm(0, 0, 32))
	cb.Emit32(arm64ORR(arm64RegIE64PC, arm64RegIE64PC, 0))
}

// emitPackedPCAndCount loads targetPC into X28 and packs instruction count.
// For backward-branch blocks: dynamic count via X7.
// For normal blocks: static count packed into upper 32 bits of immediate.
func emitPackedPCAndCount(cb *CodeBuffer, targetPC uint64, staticCount uint32, br *blockRegs) {
	if br.hasBackwardBranch {
		emitLoadImm64(cb, arm64RegIE64PC, targetPC)
		emitDynamicCount(cb, staticCount)
	} else {
		emitLoadImm64(cb, arm64RegIE64PC, targetPC|(uint64(staticCount)<<32))
	}
}

// ===========================================================================
// Block Prologue / Epilogue
// ===========================================================================

// emitPrologue emits the block entry sequence, saving/loading only
// registers the block actually uses (determined by analyzeBlockRegs).
func emitPrologue(cb *CodeBuffer, blockPC uint32, br *blockRegs) {
	// Frame is always 112 bytes (fixed layout for I/O bail path compatibility)
	cb.Emit32(arm64SUB_imm(31, 31, 112))

	// Save callee-saved pairs only if the block uses the corresponding IE64 regs.
	// X19/X20 = R8/R9, X21/X22 = R10/R11, X23/X24 = R12/R13, X25/X26 = R14/R15
	if br.used&((1<<8)|(1<<9)) != 0 {
		cb.Emit32(arm64STP_offset(19, 20, 31, 0))
	}
	if br.used&((1<<10)|(1<<11)) != 0 {
		cb.Emit32(arm64STP_offset(21, 22, 31, 2))
	}
	if br.used&((1<<12)|(1<<13)) != 0 {
		cb.Emit32(arm64STP_offset(23, 24, 31, 4))
	}
	if br.used&((1<<14)|(1<<15)) != 0 {
		cb.Emit32(arm64STP_offset(25, 26, 31, 6))
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
	}

	// Zero X7 (loop iteration counter) for blocks with backward branches
	if br.hasBackwardBranch {
		cb.Emit32(arm64MOVZ(arm64RegLoopCount, 0, 0))
	}

	// Load only IE64 registers that are read by the block
	for ie64Reg := byte(ie64FirstMapped); ie64Reg <= ie64LastMapped; ie64Reg++ {
		if br.read&(1<<ie64Reg) != 0 {
			arm64Reg := arm64FirstMapped + (ie64Reg - ie64FirstMapped)
			cb.Emit32(arm64LDR_imm(arm64Reg, arm64RegBase, uint32(ie64Reg)))
		}
	}

	// Load IE64 R31 (SP) if used
	if br.read&(1<<31) != 0 {
		cb.Emit32(arm64LDR_imm(arm64RegIE64SP, arm64RegBase, 31))
	}

	// Load block start PC into X28
	emitLoadImm64(cb, arm64RegIE64PC, uint64(blockPC))
}

// emitEpilogue emits the block exit sequence, storing/restoring only the
// registers indicated by the bitmasks.
//   - storeRegs: IE64 registers to store back (writtenRegs for normal, writtenSoFar for bail)
//   - calleeSaved: which callee-saved pairs to restore (must match what prologue saved)
func emitEpilogue(cb *CodeBuffer, storeRegs uint32, calleeSaved uint32) {
	// Store only the IE64 registers that were written
	for ie64Reg := byte(ie64FirstMapped); ie64Reg <= ie64LastMapped; ie64Reg++ {
		if storeRegs&(1<<ie64Reg) != 0 {
			arm64Reg := arm64FirstMapped + (ie64Reg - ie64FirstMapped)
			cb.Emit32(arm64STR_imm(arm64Reg, arm64RegBase, uint32(ie64Reg)))
		}
	}

	// Store IE64 R31 (SP) if written
	if storeRegs&(1<<31) != 0 {
		cb.Emit32(arm64STR_imm(arm64RegIE64SP, arm64RegBase, 31))
	}

	// Always store PC to regs[0] (return channel to dispatcher)
	cb.Emit32(arm64STR_imm(arm64RegIE64PC, arm64RegBase, 0))

	// Restore callee-saved pairs that were saved in prologue
	if calleeSaved&((1<<8)|(1<<9)) != 0 {
		cb.Emit32(arm64LDP_offset(19, 20, 31, 0))
	}
	if calleeSaved&((1<<10)|(1<<11)) != 0 {
		cb.Emit32(arm64LDP_offset(21, 22, 31, 2))
	}
	if calleeSaved&((1<<12)|(1<<13)) != 0 {
		cb.Emit32(arm64LDP_offset(23, 24, 31, 4))
	}
	if calleeSaved&((1<<14)|(1<<15)) != 0 {
		cb.Emit32(arm64LDP_offset(25, 26, 31, 6))
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
func compileBlock(instrs []JITInstr, startPC uint32, execMem *ExecMem) (*JITBlock, error) {
	cb := NewCodeBuffer(len(instrs) * 256) // FPU ops can emit 30-60 ARM64 instructions with CC setting

	br := analyzeBlockRegs(instrs)
	br.hasBackwardBranch = detectBackwardBranches(instrs, startPC)
	emitPrologue(cb, startPC, &br)

	// Track ARM64 code offsets for each IE64 instruction (for backward branches)
	instrOffsets := make([]int, len(instrs))

	// Track which registers have been written by instructions executed so far.
	// Used for I/O bail epilogues: only store registers that were actually
	// written by instructions preceding the bail point.
	writtenSoFar := uint32(0)
	for i := range instrs {
		instrOffsets[i] = cb.Len()
		ji := &instrs[i]
		emitInstruction(cb, ji, startPC, i == len(instrs)-1, &br, writtenSoFar, i, instrOffsets)
		writtenSoFar |= instrWrittenRegs(ji)
	}

	// Emit final epilogue (if the last instruction doesn't have its own)
	lastOp := instrs[len(instrs)-1].opcode
	if !isBlockTerminator(lastOp) {
		endPC := startPC + uint32(len(instrs))*IE64_INSTR_SIZE
		emitPackedPCAndCount(cb, uint64(endPC), uint32(len(instrs)), &br)
		emitEpilogue(cb, br.written, br.used)
	}

	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, err
	}

	return &JITBlock{
		startPC:    startPC,
		endPC:      startPC + uint32(len(instrs))*IE64_INSTR_SIZE,
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
func emitInstruction(cb *CodeBuffer, ji *JITInstr, blockStartPC uint32, isLast bool, br *blockRegs, writtenSoFar uint32, instrIdx int, instrOffsets []int) {
	instrPC := blockStartPC + ji.pcOffset

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
		emitLOAD(cb, ji, instrPC, br, writtenSoFar)
	case OP_STORE:
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
		emitJSR(cb, ji, instrPC, br)
	case OP_RTS64:
		emitRTS(cb, br, ji.pcOffset/IE64_INSTR_SIZE+1)
	case OP_PUSH64:
		emitPUSH(cb, ji)
	case OP_POP64:
		emitPOP(cb, ji)
	case OP_JSR_IND:
		emitJSR_IND(cb, ji, instrPC, br, ji.pcOffset/IE64_INSTR_SIZE+1)

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

	case OP_SEI64:
		cb.Emit32(arm64NOP())

	case OP_CLI64:
		cb.Emit32(arm64NOP())

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
		emitFLOAD(cb, ji, instrPC, br, writtenSoFar)
	case OP_FSTORE:
		emitFSTORE(cb, ji, instrPC, br, writtenSoFar)

	// ======================================================================
	// FPU — Category C (transcendentals, bail to interpreter)
	// ======================================================================
	case OP_FMOD, OP_FSIN, OP_FCOS, OP_FTAN, OP_FATAN, OP_FLOG, OP_FEXP, OP_FPOW:
		emitFPUBail(cb, ji, instrPC, br, writtenSoFar)
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

// ===========================================================================
// Memory Access
// ===========================================================================

// emitLOAD handles LOAD rd, disp(rs)
func emitLOAD(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
	if ji.rd == 0 {
		return
	}

	// Compute address: uint32(int64(rs) + int64(int32(imm32)))
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32) // W1 = imm32
	cb.Emit32(arm64SXTW(1, 1))     // X1 = sign-extend
	cb.Emit32(arm64ADD(0, rsReg, 1))
	// Truncate to 32 bits (uint32 conversion)
	cb.Emit32(arm64MOV_W(0, 0)) // X0 = zero-extended W0

	// Compare with IO_REGION_START
	cb.Emit32(arm64CMP(0, arm64RegIOStart))

	// Branch to slow path if addr >= IO_REGION_START
	slowPathOffset := cb.Len()
	cb.Emit32(0) // placeholder for B.HS

	// Fast path: addr < IO_REGION_START → direct memory load
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

	// Branch over slow path
	doneOffset := cb.Len()
	cb.Emit32(0) // placeholder for B

	// Slow path: addr >= IO_REGION_START
	slowPathPC := cb.Len()
	cb.PatchUint32(slowPathOffset, arm64Bcond(arm64CondHS, int32(slowPathPC-slowPathOffset)))

	// Check ioPageBitmap[addr >> 8] — X5 holds IOBitmapPtr (loaded in prologue)
	cb.Emit32(arm64LSR_imm(1, 0, 8))                 // X1 = addr >> 8
	cb.Emit32(arm64LDRB_reg(1, arm64RegIOBitmap, 1)) // W1 = ioPageBitmap[page]
	cb.Emit32(arm64CBZ(1, 0))                        // placeholder: CBZ → non-I/O fast path
	nonIOOffset := cb.Len() - 4

	// I/O page → bail to interpreter (store only regs written so far)
	cb.Emit32(arm64LDR_imm(0, 31, 96/8))                               // X0 = JITContext ptr
	emitLoadImm32(cb, 1, 1)                                            // W1 = 1
	cb.Emit32(arm64STR_W_imm(1, 0, uint32(jitCtxOffNeedIOFallback/4))) // NeedIOFallback = 1
	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br) // PC + count
	emitEpilogue(cb, writtenSoFar, br.used)

	// Non-I/O page (e.g. VRAM) → direct memory access
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

	// Patch done branch
	donePC := cb.Len()
	cb.PatchUint32(doneOffset, arm64B(int32(donePC-doneOffset)))
}

// emitSTORE handles STORE rd, disp(rs)
func emitSTORE(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
	// Compute address: uint32(int64(rs) + int64(int32(imm32)))
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(0, rsReg, 1))
	cb.Emit32(arm64MOV_W(0, 0)) // truncate to uint32

	// Load source value (rd for STORE)
	srcReg := resolveReg(cb, ji.rd, 3) // X3 scratch

	// Apply size mask to value before store
	if ji.size != IE64_SIZE_Q {
		cb.Emit32(arm64MOV(4, srcReg)) // X4 = src
		emitSizeMask(cb, 4, ji.size)
		srcReg = 4
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

	// Branch over slow path
	doneOffset := cb.Len()
	cb.Emit32(0) // placeholder for B

	// Slow path: addr >= IO_REGION_START
	slowPathPC := cb.Len()
	cb.PatchUint32(slowPathOffset, arm64Bcond(arm64CondHS, int32(slowPathPC-slowPathOffset)))

	// Check ioPageBitmap[addr >> 8] — X5 holds IOBitmapPtr (loaded in prologue)
	cb.Emit32(arm64LSR_imm(1, 0, 8))                 // X1 = addr >> 8
	cb.Emit32(arm64LDRB_reg(1, arm64RegIOBitmap, 1)) // W1 = ioPageBitmap[page]
	cb.Emit32(arm64CBZ(1, 0))                        // placeholder: CBZ → non-I/O fast path
	nonIOOffset := cb.Len() - 4

	// I/O page → bail to interpreter (store only regs written so far)
	cb.Emit32(arm64LDR_imm(0, 31, 96/8))                               // X0 = JITContext ptr
	emitLoadImm32(cb, 1, 1)                                            // W1 = 1
	cb.Emit32(arm64STR_W_imm(1, 0, uint32(jitCtxOffNeedIOFallback/4))) // NeedIOFallback = 1
	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br) // PC + count
	emitEpilogue(cb, writtenSoFar, br.used)

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

	// Patch done branch
	donePC := cb.Len()
	cb.PatchUint32(doneOffset, arm64B(int32(donePC-doneOffset)))
}

// ===========================================================================
// Control Flow
// ===========================================================================

// emitBRA handles BRA (unconditional branch)
func emitBRA(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, instrIdx int, instrOffsets []int, blockStartPC uint32) {
	targetPC := uint32(int64(instrPC) + int64(int32(ji.imm32)))
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
			emitPackedPCAndCount(cb, uint64(targetPC), staticCount, br)
			emitEpilogue(cb, br.written, br.used)
			return
		}
	}

	// Forward/external branch
	emitPackedPCAndCount(cb, uint64(targetPC), staticCount, br)
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
func emitBcc(cb *CodeBuffer, ji *JITInstr, instrPC uint32, cond byte, br *blockRegs, writtenSoFar uint32, blockStartPC uint32, instrIdx int, instrOffsets []int) {
	targetPC := uint32(int64(instrPC) + int64(int32(ji.imm32)))
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
			emitPackedPCAndCount(cb, uint64(targetPC), staticCount, br)
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

	emitPackedPCAndCount(cb, uint64(targetPC), staticCount, br)
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

	emitLoadImm64(cb, 1, IE64_ADDR_MASK)
	cb.Emit32(arm64AND(arm64RegIE64PC, arm64RegIE64PC, 1))

	// Pack instruction count into upper 32 bits of X28
	if br.hasBackwardBranch {
		emitDynamicCount(cb, instrCount)
	} else {
		emitLoadImm64(cb, 0, uint64(instrCount)<<32)
		cb.Emit32(arm64ORR(arm64RegIE64PC, arm64RegIE64PC, 0))
	}

	emitEpilogue(cb, br.written, br.used)
}

// emitJSR handles JSR (jump to subroutine, PC-relative)
func emitJSR(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs) {
	cb.Emit32(arm64SUB_imm(arm64RegIE64SP, arm64RegIE64SP, 8))

	retAddr := uint64(instrPC + IE64_INSTR_SIZE)
	emitLoadImm64(cb, 0, retAddr)
	cb.Emit32(arm64STR_reg(0, arm64RegMemBase, arm64RegIE64SP))

	staticCount := uint32(ji.pcOffset/IE64_INSTR_SIZE + 1)
	targetPC := uint64(uint32(int64(instrPC) + int64(int32(ji.imm32))))
	emitPackedPCAndCount(cb, targetPC, staticCount, br)

	emitEpilogue(cb, br.written, br.used)
}

// emitRTS handles RTS (return from subroutine)
func emitRTS(cb *CodeBuffer, br *blockRegs, instrCount uint32) {
	cb.Emit32(arm64LDR_reg(arm64RegIE64PC, arm64RegMemBase, arm64RegIE64SP))
	cb.Emit32(arm64ADD_imm(arm64RegIE64SP, arm64RegIE64SP, 8))

	// Pack instruction count into upper 32 bits of X28
	if br.hasBackwardBranch {
		emitDynamicCount(cb, instrCount)
	} else {
		emitLoadImm64(cb, 0, uint64(instrCount)<<32)
		cb.Emit32(arm64ORR(arm64RegIE64PC, arm64RegIE64PC, 0))
	}

	emitEpilogue(cb, br.written, br.used)
}

// emitRTI handles RTI (return from interrupt) by bailing to the interpreter.
// RTI modifies PC (pops from stack) and clears the inInterrupt atomic flag,
// both of which require Go runtime interaction. We bail to the interpreter
// to handle it, storing all registers written by prior instructions.
func emitRTI(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
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
func emitWAIT(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
	cb.Emit32(arm64LDR_imm(0, 31, 96/8)) // X0 = JITContext from stack
	emitLoadImm32(cb, 1, 1)
	cb.Emit32(arm64STR_W_imm(1, 0, uint32(jitCtxOffNeedIOFallback/4)))
	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
}

// emitPUSH handles PUSH rs
func emitPUSH(cb *CodeBuffer, ji *JITInstr) {
	// SP -= 8; mem[SP] = Rs
	cb.Emit32(arm64SUB_imm(arm64RegIE64SP, arm64RegIE64SP, 8))

	srcReg := resolveReg(cb, ji.rs, 0)
	cb.Emit32(arm64STR_reg(srcReg, arm64RegMemBase, arm64RegIE64SP))
}

// emitPOP handles POP rd
func emitPOP(cb *CodeBuffer, ji *JITInstr) {
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if !mapped {
		dstReg = 2
	}

	// Rd = mem[SP]; SP += 8
	if ji.rd != 0 {
		cb.Emit32(arm64LDR_reg(dstReg, arm64RegMemBase, arm64RegIE64SP))
	}
	cb.Emit32(arm64ADD_imm(arm64RegIE64SP, arm64RegIE64SP, 8))

	if ji.rd != 0 && !mapped {
		emitStoreSpilledReg(cb, dstReg, ji.rd)
	}
}

// emitJSR_IND handles JSR_IND (register-indirect subroutine call)
func emitJSR_IND(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, instrCount uint32) {
	cb.Emit32(arm64SUB_imm(arm64RegIE64SP, arm64RegIE64SP, 8))

	retAddr := uint64(instrPC + IE64_INSTR_SIZE)
	emitLoadImm64(cb, 0, retAddr)
	cb.Emit32(arm64STR_reg(0, arm64RegMemBase, arm64RegIE64SP))

	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(arm64RegIE64PC, rsReg, 1))

	emitLoadImm64(cb, 1, IE64_ADDR_MASK)
	cb.Emit32(arm64AND(arm64RegIE64PC, arm64RegIE64PC, 1))

	// Pack instruction count into upper 32 bits of X28
	if br.hasBackwardBranch {
		emitDynamicCount(cb, instrCount)
	} else {
		emitLoadImm64(cb, 0, uint64(instrCount)<<32)
		cb.Emit32(arm64ORR(arm64RegIE64PC, arm64RegIE64PC, 0))
	}

	emitEpilogue(cb, br.written, br.used)
}

// ===========================================================================
// FPU emission helpers
// ===========================================================================

const (
	fpuOffFPCR = 64 / 4 // FPCR imm12 offset for LDR_W_imm/STR_W_imm
	fpuOffFPSR = 68 / 4 // FPSR imm12 offset for LDR_W_imm/STR_W_imm
)

func emitLoadFPReg(cb *CodeBuffer, armReg, fpIdx byte) {
	cb.Emit32(arm64LDR_W_imm(armReg, arm64RegFPUBase, uint32(fpIdx&0x0F)))
}

func emitStoreFPReg(cb *CodeBuffer, armReg, fpIdx byte) {
	cb.Emit32(arm64STR_W_imm(armReg, arm64RegFPUBase, uint32(fpIdx&0x0F)))
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
	emitSetFPCondCodes(cb)
}

func emitFNEG(cb *CodeBuffer, ji *JITInstr) {
	emitLoadFPReg(cb, 0, ji.rs)
	emitLoadImm32(cb, 1, 0x80000000)
	cb.Emit32(arm64EOR_W(0, 0, 1)) // flip sign bit
	emitStoreFPReg(cb, 0, ji.rd)
	emitSetFPCondCodes(cb)
}

func emitFMOVI(cb *CodeBuffer, ji *JITInstr) {
	rsReg := resolveReg(cb, ji.rs, 0)
	cb.Emit32(arm64MOV_W(0, rsReg)) // W0 = uint32(rs)
	emitStoreFPReg(cb, 0, ji.rd)
	emitSetFPCondCodes(cb)
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
	emitSetFPCondCodes(cb)
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

func emitFPBinaryArith(cb *CodeBuffer, ji *JITInstr, fpOp func(sd, sn, sm byte) uint32) {
	emitLoadFPReg(cb, 0, ji.rs)
	cb.Emit32(arm64FMOV_WtoS(0, 0))
	emitLoadFPReg(cb, 1, ji.rt)
	cb.Emit32(arm64FMOV_WtoS(1, 1))
	cb.Emit32(fpOp(2, 0, 1))
	cb.Emit32(arm64FMOV_StoW(0, 2))
	emitStoreFPReg(cb, 0, ji.rd)
	emitSetFPCondCodes(cb)
}

func emitFADD(cb *CodeBuffer, ji *JITInstr) { emitFPBinaryArith(cb, ji, arm64FADD_S) }
func emitFSUB(cb *CodeBuffer, ji *JITInstr) { emitFPBinaryArith(cb, ji, arm64FSUB_S) }
func emitFMUL(cb *CodeBuffer, ji *JITInstr) { emitFPBinaryArith(cb, ji, arm64FMUL_S) }
func emitFDIV(cb *CodeBuffer, ji *JITInstr) { emitFPBinaryArith(cb, ji, arm64FDIV_S) }

func emitFSQRT(cb *CodeBuffer, ji *JITInstr) {
	emitLoadFPReg(cb, 0, ji.rs)
	cb.Emit32(arm64FMOV_WtoS(0, 0))
	cb.Emit32(arm64FSQRT_S(1, 0))
	cb.Emit32(arm64FMOV_StoW(0, 1))
	emitStoreFPReg(cb, 0, ji.rd)
	emitSetFPCondCodes(cb)
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
	emitSetFPCondCodes(cb)
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

	// Greater than (fallthrough)
	cb.Emit32(arm64MOVZ_W(3, 1, 0)) // result = 1
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
	emitSetFPCondCodes(cb)
}

func emitFCVTFI(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}
	emitLoadFPReg(cb, 0, ji.rs)
	cb.Emit32(arm64FMOV_WtoS(0, 0))
	cb.Emit32(arm64FCVTZS_SW(0, 0)) // W0 = int32(S0), saturating
	cb.Emit32(arm64SXTW(0, 0))      // sign-extend to int64
	dstReg, mapped := ie64ToARM64Reg(ji.rd)
	if mapped {
		cb.Emit32(arm64MOV(dstReg, 0))
	} else {
		emitStoreSpilledReg(cb, 0, ji.rd)
	}
}

// ===========================================================================
// FPU — Memory operations
// ===========================================================================

func emitFLOAD(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
	// Compute address: uint32(int64(rs) + int64(int32(imm32)))
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(0, rsReg, 1))
	cb.Emit32(arm64MOV_W(0, 0)) // truncate to uint32

	cb.Emit32(arm64CMP(0, arm64RegIOStart))
	slowPathOffset := cb.Len()
	cb.Emit32(0) // B.HS → slow path

	// Fast path: direct 32-bit load
	cb.Emit32(arm64LDR_W_reg(2, arm64RegMemBase, 0))
	doneOffset := cb.Len()
	cb.Emit32(0) // B → done

	// Slow path
	slowPathPC := cb.Len()
	cb.PatchUint32(slowPathOffset, arm64Bcond(arm64CondHS, int32(slowPathPC-slowPathOffset)))

	cb.Emit32(arm64LSR_imm(1, 0, 8))
	cb.Emit32(arm64LDRB_reg(1, arm64RegIOBitmap, 1))
	cb.Emit32(arm64CBZ(1, 0))
	nonIOOffset := cb.Len() - 4

	// I/O page → bail
	cb.Emit32(arm64LDR_imm(0, 31, 96/8))
	emitLoadImm32(cb, 1, 1)
	cb.Emit32(arm64STR_W_imm(1, 0, uint32(jitCtxOffNeedIOFallback/4)))
	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)

	// Non-I/O page
	nonIOPC := cb.Len()
	cb.PatchUint32(nonIOOffset, arm64CBZ(1, int32(nonIOPC-nonIOOffset)))
	cb.Emit32(arm64LDR_W_reg(2, arm64RegMemBase, 0))

	// done:
	donePC := cb.Len()
	cb.PatchUint32(doneOffset, arm64B(int32(donePC-doneOffset)))

	// Store to FP register and set CC
	emitStoreFPReg(cb, 2, ji.rd)
	cb.Emit32(arm64MOV_W(0, 2))
	emitSetFPCondCodes(cb)
}

func emitFSTORE(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
	// Compute address
	rsReg := resolveReg(cb, ji.rs, 0)
	emitLoadImm32(cb, 1, ji.imm32)
	cb.Emit32(arm64SXTW(1, 1))
	cb.Emit32(arm64ADD(0, rsReg, 1))
	cb.Emit32(arm64MOV_W(0, 0))

	// Load FP source value
	emitLoadFPReg(cb, 3, ji.rd)

	cb.Emit32(arm64CMP(0, arm64RegIOStart))
	slowPathOffset := cb.Len()
	cb.Emit32(0)

	// Fast path: direct 32-bit store
	cb.Emit32(arm64STR_W_reg(3, arm64RegMemBase, 0))
	doneOffset := cb.Len()
	cb.Emit32(0)

	// Slow path
	slowPathPC := cb.Len()
	cb.PatchUint32(slowPathOffset, arm64Bcond(arm64CondHS, int32(slowPathPC-slowPathOffset)))

	cb.Emit32(arm64LSR_imm(1, 0, 8))
	cb.Emit32(arm64LDRB_reg(1, arm64RegIOBitmap, 1))
	cb.Emit32(arm64CBZ(1, 0))
	nonIOOffset := cb.Len() - 4

	// I/O page → bail
	cb.Emit32(arm64LDR_imm(0, 31, 96/8))
	emitLoadImm32(cb, 1, 1)
	cb.Emit32(arm64STR_W_imm(1, 0, uint32(jitCtxOffNeedIOFallback/4)))
	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)

	// Non-I/O page
	nonIOPC := cb.Len()
	cb.PatchUint32(nonIOOffset, arm64CBZ(1, int32(nonIOPC-nonIOOffset)))
	cb.Emit32(arm64STR_W_reg(3, arm64RegMemBase, 0))

	donePC := cb.Len()
	cb.PatchUint32(doneOffset, arm64B(int32(donePC-doneOffset)))
}

// ===========================================================================
// FPU — Category C: Transcendentals (bail to interpreter)
// ===========================================================================

func emitFPUBail(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
	cb.Emit32(arm64LDR_imm(0, 31, 96/8))
	emitLoadImm32(cb, 1, 1)
	cb.Emit32(arm64STR_W_imm(1, 0, uint32(jitCtxOffNeedIOFallback/4)))
	bailCount := uint32(ji.pcOffset / IE64_INSTR_SIZE)
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
}
