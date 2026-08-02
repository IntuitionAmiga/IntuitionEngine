// jit_x86_emit_arm64.go - initial Linux/arm64 x86 native emitter.
//
// x86 guest registers remain in the JIT register file for this first ARM64
// slice. Keeping the mapping memory-resident makes every native return an
// architecturally complete publication point and avoids an ABI split with
// interpreter helper exits.

//go:build arm64 && linux

package main

import (
	"encoding/binary"
	"fmt"
)

const (
	x86ARM64RegCtx   = 0  // X0, incoming X86JITContext
	x86ARM64RegRegs  = 9  // X9, cpu.jitRegs
	x86ARM64Scratch  = 10 // W10/W11 are caller-saved scratch
	x86ARM64Scratch2 = 11
)

const (
	x86ARM64FPUOffFCW = 64
	x86ARM64FPUOffFIP = 72
	x86ARM64FPUOffFCS = 76
	x86ARM64FPUOffFSW = 66
	x86ARM64FPUOffFTW = 68
	x86ARM64FPUOffFOP = 86
)

// x86ARM64DeferredBail is an interpreter re-entry branch emitted before a
// native guest-memory access. The guarded instruction has not mutated state,
// so RetPC names that instruction and RetCount excludes it.
type x86ARM64DeferredBail struct {
	branchOffset int
	retPC        uint32
	retCount     int
	mmio         bool
	inval        bool
	invalSize    uint32
}

// x86ARM64EmitMovImm32 materialises an arbitrary i386 immediate in Wd.
func x86ARM64EmitMovImm32(cb *CodeBuffer, rd byte, v uint32) {
	cb.Emit32(arm64MOVZ_W(rd, uint16(v), 0))
	if v>>16 != 0 {
		cb.Emit32(arm64MOVK_W(rd, uint16(v>>16), 16))
	}
}

// x86ARM64Grp2CountOne accepts the fixed-count Group 2 encodings only when
// their immediate is exactly one. The shared native lowering then has the
// same defined CF/OF semantics as D0/D1, without treating arbitrary counts as
// count-one instructions.
func x86ARM64Grp2CountOne(ji X86JITInstr, memory []byte) bool {
	op := byte(ji.opcode)
	if op == 0xD0 || op == 0xD1 {
		return true
	}
	if (op != 0xC0 && op != 0xC1) || ji.length == 0 {
		return false
	}
	immPC := int(ji.opcodePC) + int(ji.length) - 1
	return immPC >= 0 && immPC < len(memory) && memory[immPC]&0x1F == 1
}

func x86ARM64Grp2Width(op byte, prefixes byte) uint32 {
	if op == 0xD0 || op == 0xC0 {
		return 8
	}
	if prefixes&x86PrefOpSize != 0 {
		return 16
	}
	return 32
}

func x86ARM64EmitLoadReg(cb *CodeBuffer, dst, guest byte) {
	cb.Emit32(arm64LDR_W_imm(dst, x86ARM64RegRegs, uint32(guest)))
}

func x86ARM64EmitStoreReg(cb *CodeBuffer, guest, src byte) {
	cb.Emit32(arm64STR_W_imm(src, x86ARM64RegRegs, uint32(guest)))
}

func x86ARM64EmitAddrOffset(cb *CodeBuffer, dst, base byte, offset uint32) {
	if offset == 0 {
		cb.Emit32(arm64ORR_W(dst, base, 31))
		return
	}
	x86ARM64EmitMovImm32(cb, dst, offset)
	cb.Emit32(arm64ADD_W(dst, base, dst))
}

func x86ARM64EmitByteExtract(cb *CodeBuffer, dst, src byte, high bool) {
	shift := uint32(0)
	if high {
		shift = 8
	}
	// UBFX Wd, Wn, #shift, #8.  Low-byte extraction must mask too: a
	// register move can rely on BFI to discard upper bits, whereas MOVZX and
	// MOVSX need the standalone byte value.
	cb.Emit32(0x53000000 | shift<<16 | (shift+7)<<10 | uint32(src)<<5 | uint32(dst))
}

func x86ARM64EmitByteInsert(cb *CodeBuffer, dst, src byte, high bool) {
	if high {
		// BFI Wd, Wn, #8, #8: BFM immr=(32-8), imms=7.
		cb.Emit32(0x33000000 | 24<<16 | 7<<10 | uint32(src)<<5 | uint32(dst))
		return
	}
	// BFI Wd, Wn, #0, #8.
	cb.Emit32(0x33000000 | 7<<10 | uint32(src)<<5 | uint32(dst))
}

func x86ARM64EmitWordInsert(cb *CodeBuffer, dst, src byte) {
	// BFI Wd, Wn, #0, #16.
	cb.Emit32(0x33000000 | 15<<10 | uint32(src)<<5 | uint32(dst))
}

// x86ARM64EmitFlagBit changes one architected EFLAGS bit through FlagsPtr.
// This is deliberately memory-backed rather than NZCV-backed: x86 status
// flags remain valid across helper exits and no host flags leak to Go.
func x86ARM64EmitFlagBit(cb *CodeBuffer, bit uint32, set, toggle bool) {
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, x86ARM64Scratch2, 0))
	x86ARM64EmitMovImm32(cb, 12, bit)
	if toggle {
		cb.Emit32(arm64EOR_W(x86ARM64Scratch, x86ARM64Scratch, 12))
	} else if set {
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 12))
	} else {
		x86ARM64EmitMovImm32(cb, 12, ^bit)
		cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, 12))
	}
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64Scratch2, 0))
}

// x86ARM64EmitLogicFlags publishes the interpreter's logical-op flag subset:
// CF and OF clear, ZF/SF/PF derive from result, and AF is left unchanged.
// result must already be truncated to width bits.
func x86ARM64EmitLogicFlags(cb *CodeBuffer, result byte, width uint32) {
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, ^uint32(x86FlagCF|x86FlagOF|x86FlagZF|x86FlagSF|x86FlagPF))
	cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	cb.Emit32(arm64CMP_W(result, 31))
	cb.Emit32(arm64CSET_W(x86ARM64Scratch2, arm64CondEQ))
	cb.Emit32(arm64LSL_W_imm(x86ARM64Scratch2, x86ARM64Scratch2, 6))
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	cb.Emit32(arm64LSR_W_imm(x86ARM64Scratch2, result, width-1))
	x86ARM64EmitMovImm32(cb, 12, 1)
	cb.Emit32(arm64AND_W(x86ARM64Scratch2, x86ARM64Scratch2, 12))
	cb.Emit32(arm64LSL_W_imm(x86ARM64Scratch2, x86ARM64Scratch2, 7))
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	// Fold the low byte to an odd/even parity bit, then invert it because x86
	// PF is set for even parity.
	cb.Emit32(0x53000000 | 7<<10 | uint32(result)<<5 | 12) // UBFX W12,Wresult,#0,#8
	cb.Emit32(arm64LSR_W_imm(x86ARM64Scratch2, 12, 4))
	cb.Emit32(arm64EOR_W(12, 12, x86ARM64Scratch2))
	cb.Emit32(arm64LSR_W_imm(x86ARM64Scratch2, 12, 2))
	cb.Emit32(arm64EOR_W(12, 12, x86ARM64Scratch2))
	cb.Emit32(arm64LSR_W_imm(x86ARM64Scratch2, 12, 1))
	cb.Emit32(arm64EOR_W(12, 12, x86ARM64Scratch2))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 1)
	cb.Emit32(arm64AND_W(12, 12, x86ARM64Scratch2))
	cb.Emit32(arm64EOR_W(12, 12, x86ARM64Scratch2))
	cb.Emit32(arm64LSL_W_imm(12, 12, 2))
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 12))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
}

// x86ARM64EmitLogicalOp emits one of the three logical ALU operations shared
// by the one-byte ALU encodings and Group 1.  op is the low three-bit x86 ALU
// selector: 1 is OR, 4 is AND and 6 is XOR.
func x86ARM64EmitLogicalOp(cb *CodeBuffer, op byte, dst, src byte) bool {
	switch op {
	case 1:
		cb.Emit32(arm64ORR_W(dst, dst, src))
	case 4:
		cb.Emit32(arm64AND_W(dst, dst, src))
	case 6:
		cb.Emit32(arm64EOR_W(dst, dst, src))
	default:
		return false
	}
	return true
}

func x86ARM64EmitTruncate(cb *CodeBuffer, reg byte, width uint32) {
	switch width {
	case 8:
		x86ARM64EmitByteExtract(cb, reg, reg, false)
	case 16:
		cb.Emit32(0x53000000 | 15<<10 | uint32(reg)<<5 | uint32(reg)) // UBFX Wreg,Wreg,#0,#16
	}
}

func x86ARM64EmitPartialRegStore(cb *CodeBuffer, guest, value byte, width uint32) {
	switch width {
	case 8:
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, guest&3)
		x86ARM64EmitByteInsert(cb, x86ARM64Scratch, value, guest >= 4)
		x86ARM64EmitStoreReg(cb, guest&3, x86ARM64Scratch)
	case 16:
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, guest)
		x86ARM64EmitWordInsert(cb, x86ARM64Scratch, value)
		x86ARM64EmitStoreReg(cb, guest, x86ARM64Scratch)
	default:
		x86ARM64EmitStoreReg(cb, guest, value)
	}
}

func x86ARM64EmitPartialRegLoad(cb *CodeBuffer, dst, guest byte, width uint32) {
	x86ARM64EmitLoadReg(cb, dst, guest&3)
	if width == 8 {
		x86ARM64EmitByteExtract(cb, dst, dst, guest >= 4)
	} else {
		x86ARM64EmitTruncate(cb, dst, width)
	}
}

// csel Wd, Wn, Wm, cond
func arm64CSEL_W(rd, rn, rm, cond byte) uint32 {
	return 0x1A800000 | uint32(cond&0xF)<<12 | uint32(rm)<<16 | uint32(rn)<<5 | uint32(rd)
}

// x86ARM64EmitGrp2CL32Reg lowers the D3 register forms that do not carry CF
// through the operand. Count zero is a full architectural no-op; other counts
// retain OF except at count one, exactly matching shiftRotate32.
func x86ARM64EmitGrp2CL32Reg(cb *CodeBuffer, ji X86JITInstr) bool {
	if ji.opcode != 0xD3 || !ji.hasModRM || ji.modrm>>6 != 3 || ji.prefixes != 0 {
		return false
	}
	op, rm := ji.grpOp, ji.modrm&7
	if op != 0 && op != 1 && op != 4 && op != 5 && op != 6 && op != 7 {
		return false
	}
	x86ARM64EmitLoadReg(cb, 18, 1) // ECX
	x86ARM64EmitMovImm32(cb, 12, 31)
	cb.Emit32(arm64AND_W(18, 18, 12))
	zero := cb.Len()
	cb.Emit32(arm64CBZ(18, 0))
	x86ARM64EmitLoadReg(cb, 16, rm)
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(20, 14, 0))
	x86ARM64EmitMovImm32(cb, 12, x86FlagOF)
	cb.Emit32(arm64AND_W(20, 20, 12)) // old OF

	if op == 0 || op == 1 { // ROL/ROR
		if op == 0 {
			x86ARM64EmitMovImm32(cb, 17, 32)
			cb.Emit32(arm64SUB_W(17, 17, 18))
			cb.Emit32(arm64ROR_W(13, 16, 17))
			cb.Emit32(arm64ORR_W(19, 13, 31))
			cb.Emit32(arm64LSR_W_imm(17, 13, 31))
			cb.Emit32(arm64EOR_W(21, 17, 19))
		} else {
			cb.Emit32(arm64ROR_W(13, 16, 18))
			cb.Emit32(arm64LSR_W_imm(19, 13, 31))
			cb.Emit32(arm64LSR_W_imm(17, 13, 30))
			x86ARM64EmitMovImm32(cb, 12, 1)
			cb.Emit32(arm64AND_W(17, 17, 12))
			cb.Emit32(arm64EOR_W(21, 19, 17))
		}
		// UBFX W19,W19,#0,#1 keeps CF as a single guest flag bit without
		// relying on a mutable general scratch across the rotation sequence.
		cb.Emit32(0x53000000 | uint32(19)<<5 | 19)
		cb.Emit32(arm64LSL_W_imm(21, 21, 11))
		x86ARM64EmitStoreReg(cb, rm, 13)
		// Rotates define only CF and, for count one, OF.
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, ^uint32(x86FlagCF|x86FlagOF))
		cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitMovImm32(cb, 12, 1)
		cb.Emit32(arm64CMP_W(18, 12))
		notOne := cb.Len()
		cb.Emit32(arm64Bcond(arm64CondNE, 0))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 21))
		afterOF := cb.Len()
		cb.Emit32(arm64B(0))
		oldOF := cb.Len()
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 20))
		afterOFPatch := cb.Len()
		cb.PatchUint32(notOne, arm64Bcond(arm64CondNE, int32(oldOF-notOne)))
		cb.PatchUint32(afterOF, arm64B(int32(afterOFPatch-afterOF)))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 19))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
	} else {
		x86ARM64EmitMovImm32(cb, 17, 1)
		if op == 4 || op == 6 { // SHL/SAL
			x86ARM64EmitMovImm32(cb, 12, 32)
			cb.Emit32(arm64SUB_W(12, 12, 18))
			cb.Emit32(arm64LSR_W(19, 16, 12))
			cb.Emit32(arm64LSLV_W(13, 16, 18))
			cb.Emit32(arm64LSR_W_imm(17, 13, 31))
			cb.Emit32(arm64LSR_W_imm(21, 16, 31))
			cb.Emit32(arm64EOR_W(21, 17, 21))
		} else {
			cb.Emit32(arm64SUB_W(12, 18, 17))
			cb.Emit32(arm64LSR_W(19, 16, 12))
			if op == 5 { // SHR
				cb.Emit32(arm64LSR_W(13, 16, 18))
				cb.Emit32(arm64LSR_W_imm(21, 16, 31))
			} else { // SAR
				cb.Emit32(arm64ASR_W(13, 16, 18))
				x86ARM64EmitMovImm32(cb, 21, 0)
			}
		}
		x86ARM64EmitMovImm32(cb, 12, 1)
		cb.Emit32(arm64AND_W(19, 19, 12))
		cb.Emit32(arm64LSL_W_imm(21, 21, 11))
		x86ARM64EmitStoreReg(cb, rm, 13)
		x86ARM64EmitLogicFlags(cb, 13, 32)
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
		x86ARM64EmitMovImm32(cb, 12, 1)
		cb.Emit32(arm64CMP_W(18, 12))
		notOne := cb.Len()
		cb.Emit32(arm64Bcond(arm64CondNE, 0))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 21))
		afterOF := cb.Len()
		cb.Emit32(arm64B(0))
		oldOF := cb.Len()
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 20))
		afterOFPatch := cb.Len()
		cb.PatchUint32(notOne, arm64Bcond(arm64CondNE, int32(oldOF-notOne)))
		cb.PatchUint32(afterOF, arm64B(int32(afterOFPatch-afterOF)))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 19))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
	}
	end := cb.Len()
	cb.PatchUint32(zero, arm64CBZ(18, int32(end-zero)))
	return true
}

// x86ARM64EmitGrp2CL8ShiftReg lowers D2 register SHL/SHR/SAR forms.  A
// one-bit emitted loop intentionally mirrors the interpreter for counts above
// seven, where AArch64 variable shifts would otherwise wrap their count while
// the i386 byte operation has already shifted all operand bits away.
func x86ARM64EmitGrp2CL8ShiftReg(cb *CodeBuffer, ji X86JITInstr) bool {
	if ji.opcode != 0xD2 || !ji.hasModRM || ji.modrm>>6 != 3 || ji.prefixes != 0 {
		return false
	}
	op, rm := ji.grpOp, ji.modrm&7
	if op != 4 && op != 5 && op != 6 && op != 7 {
		return false
	}
	x86ARM64EmitLoadReg(cb, 18, 1) // ECX
	x86ARM64EmitMovImm32(cb, 12, 31)
	cb.Emit32(arm64AND_W(18, 18, 12))
	zero := cb.Len()
	cb.Emit32(arm64CBZ(18, 0))
	cb.Emit32(arm64ORR_W(17, 18, 31)) // original masked count
	x86ARM64EmitLoadReg(cb, 16, rm&3)
	x86ARM64EmitByteExtract(cb, 13, 16, rm >= 4)
	cb.Emit32(arm64ORR_W(21, 13, 31)) // original byte for count-one OF
	// Preserve OF because i386 leaves it unchanged for every count but one.
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(20, 14, 0))
	x86ARM64EmitMovImm32(cb, 12, x86FlagOF)
	cb.Emit32(arm64AND_W(20, 20, 12))

	loop := cb.Len()
	switch op {
	case 4, 6: // SHL/SAL
		cb.Emit32(arm64LSR_W_imm(19, 13, 7))
		cb.Emit32(arm64LSL_W_imm(13, 13, 1))
	case 5: // SHR
		x86ARM64EmitMovImm32(cb, 12, 1)
		cb.Emit32(arm64AND_W(19, 13, 12))
		cb.Emit32(arm64LSR_W_imm(13, 13, 1))
	case 7: // SAR
		x86ARM64EmitMovImm32(cb, 12, 1)
		cb.Emit32(arm64AND_W(19, 13, 12))
		cb.Emit32(arm64SXTB(13, 13))
		cb.Emit32(arm64ASR_W_imm(13, 13, 1))
	}
	x86ARM64EmitByteExtract(cb, 13, 13, false)
	cb.Emit32(arm64SUB_W_imm(18, 18, 1))
	back := cb.Len()
	cb.Emit32(arm64CBNZ(18, 0))
	cb.PatchUint32(back, arm64CBNZ(18, int32(loop-back)))
	if op == 7 {
		// The interpreter's byte SAR carry formula reads the original byte at
		// count-1. Once that exceeds bit seven it is clear, rather than the
		// sign-fill carry produced by another one-bit host iteration.
		x86ARM64EmitMovImm32(cb, 12, 8)
		cb.Emit32(arm64CMP_W(17, 12))
		keepCarry := cb.Len()
		cb.Emit32(arm64Bcond(arm64CondLS, 0))
		x86ARM64EmitMovImm32(cb, 19, 0)
		keepCarryPC := cb.Len()
		cb.PatchUint32(keepCarry, arm64Bcond(arm64CondLS, int32(keepCarryPC-keepCarry)))
	}

	x86ARM64EmitByteInsert(cb, 16, 13, rm >= 4)
	x86ARM64EmitStoreReg(cb, rm&3, 16)
	x86ARM64EmitLogicFlags(cb, 13, 8)
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(10, 14, 0))
	cb.Emit32(arm64ORR_W(10, 10, 19)) // final carry is bit zero
	cb.Emit32(arm64STR_W_imm(10, 14, 0))
	x86ARM64EmitMovImm32(cb, 12, 1)
	cb.Emit32(arm64CMP_W(17, 12))
	notOne := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondNE, 0))
	if op == 4 || op == 6 {
		cb.Emit32(arm64LSR_W_imm(11, 13, 7))
		cb.Emit32(arm64LSR_W_imm(12, 21, 7))
		cb.Emit32(arm64EOR_W(11, 11, 12))
	} else if op == 5 {
		cb.Emit32(arm64LSR_W_imm(11, 21, 7))
	} else { // SAR clears OF for count one.
		x86ARM64EmitMovImm32(cb, 11, 0)
	}
	noOF := cb.Len()
	cb.Emit32(arm64CBZ(11, 0))
	x86ARM64EmitFlagBit(cb, x86FlagOF, true, false)
	noOFPC := cb.Len()
	cb.PatchUint32(noOF, arm64CBZ(11, int32(noOFPC-noOF)))
	afterOF := cb.Len()
	cb.Emit32(arm64B(0))
	oldOF := cb.Len()
	cb.Emit32(arm64LDR_W_imm(10, 14, 0))
	cb.Emit32(arm64ORR_W(10, 10, 20))
	cb.Emit32(arm64STR_W_imm(10, 14, 0))
	end := cb.Len()
	cb.PatchUint32(notOne, arm64Bcond(arm64CondNE, int32(oldOF-notOne)))
	cb.PatchUint32(afterOF, arm64B(int32(end-afterOF)))
	cb.PatchUint32(zero, arm64CBZ(18, int32(end-zero)))
	return true
}

// x86ARM64EmitGrp2CL16ShiftReg lowers operand-size-prefixed D3 register
// SHL/SHR/SAR forms.  Keep the count as an emitted one-bit loop: after the
// i386 five-bit mask, counts 16..31 must not acquire AArch64's modulo-32
// variable-shift behaviour.
func x86ARM64EmitGrp2CL16ShiftReg(cb *CodeBuffer, ji X86JITInstr) bool {
	if ji.opcode != 0xD3 || !ji.hasModRM || ji.modrm>>6 != 3 || ji.prefixes != x86PrefOpSize {
		return false
	}
	op, rm := ji.grpOp, ji.modrm&7
	if op != 4 && op != 5 && op != 6 && op != 7 {
		return false
	}
	x86ARM64EmitLoadReg(cb, 18, 1) // ECX
	x86ARM64EmitMovImm32(cb, 12, 31)
	cb.Emit32(arm64AND_W(18, 18, 12))
	zero := cb.Len()
	cb.Emit32(arm64CBZ(18, 0))
	cb.Emit32(arm64ORR_W(17, 18, 31)) // original masked count
	x86ARM64EmitPartialRegLoad(cb, 16, rm, 16)
	cb.Emit32(arm64ORR_W(21, 16, 31)) // original word for count-one OF
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(20, 14, 0))
	x86ARM64EmitMovImm32(cb, 12, x86FlagOF)
	cb.Emit32(arm64AND_W(20, 20, 12))

	loop := cb.Len()
	switch op {
	case 4, 6: // SHL/SAL
		cb.Emit32(arm64LSR_W_imm(19, 16, 15))
		cb.Emit32(arm64LSL_W_imm(16, 16, 1))
	case 5: // SHR
		x86ARM64EmitMovImm32(cb, 12, 1)
		cb.Emit32(arm64AND_W(19, 16, 12))
		cb.Emit32(arm64LSR_W_imm(16, 16, 1))
	case 7: // SAR
		x86ARM64EmitMovImm32(cb, 12, 1)
		cb.Emit32(arm64AND_W(19, 16, 12))
		cb.Emit32(arm64SXTH(16, 16))
		cb.Emit32(arm64ASR_W_imm(16, 16, 1))
	}
	x86ARM64EmitTruncate(cb, 16, 16)
	cb.Emit32(arm64SUB_W_imm(18, 18, 1))
	back := cb.Len()
	cb.Emit32(arm64CBNZ(18, 0))
	cb.PatchUint32(back, arm64CBNZ(18, int32(loop-back)))
	// The interpreter defines word SHL/SHR with a count at or above the
	// operand width as zero with CF clear.  SAR instead retains the sign-fill
	// carry produced by the loop.
	if op != 7 {
		x86ARM64EmitMovImm32(cb, 12, 16)
		cb.Emit32(arm64CMP_W(17, 12))
		keepCarry := cb.Len()
		cb.Emit32(arm64Bcond(arm64CondLO, 0))
		x86ARM64EmitMovImm32(cb, 19, 0)
		carryDone := cb.Len()
		cb.PatchUint32(keepCarry, arm64Bcond(arm64CondLO, int32(carryDone-keepCarry)))
	}

	x86ARM64EmitLoadReg(cb, 13, rm)
	x86ARM64EmitWordInsert(cb, 13, 16)
	x86ARM64EmitStoreReg(cb, rm, 13)
	x86ARM64EmitLogicFlags(cb, 16, 16)
	cb.Emit32(arm64LDR_W_imm(10, 14, 0))
	cb.Emit32(arm64ORR_W(10, 10, 19))
	cb.Emit32(arm64STR_W_imm(10, 14, 0))
	x86ARM64EmitMovImm32(cb, 12, 1)
	cb.Emit32(arm64CMP_W(17, 12))
	notOne := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondNE, 0))
	if op == 4 || op == 6 {
		cb.Emit32(arm64LSR_W_imm(11, 16, 15))
		cb.Emit32(arm64LSR_W_imm(12, 21, 15))
		cb.Emit32(arm64EOR_W(11, 11, 12))
	} else if op == 5 {
		cb.Emit32(arm64LSR_W_imm(11, 21, 15))
	} else { // SAR clears OF for count one.
		x86ARM64EmitMovImm32(cb, 11, 0)
	}
	noOF := cb.Len()
	cb.Emit32(arm64CBZ(11, 0))
	x86ARM64EmitFlagBit(cb, x86FlagOF, true, false)
	noOFPC := cb.Len()
	cb.PatchUint32(noOF, arm64CBZ(11, int32(noOFPC-noOF)))
	afterOF := cb.Len()
	cb.Emit32(arm64B(0))
	oldOF := cb.Len()
	cb.Emit32(arm64LDR_W_imm(10, 14, 0))
	cb.Emit32(arm64ORR_W(10, 10, 20))
	cb.Emit32(arm64STR_W_imm(10, 14, 0))
	end := cb.Len()
	cb.PatchUint32(notOne, arm64Bcond(arm64CondNE, int32(oldOF-notOne)))
	cb.PatchUint32(afterOF, arm64B(int32(end-afterOF)))
	cb.PatchUint32(zero, arm64CBZ(18, int32(end-zero)))
	return true
}

// x86ARM64EmitGrp2CLCarryRotateReg lowers register RCL/RCR forms.  The
// interpreter defines byte and word counts modulo nine and seventeen after
// the normal five-bit mask, while dword counts retain the masked value.  An
// emitted one-bit loop is compact, exact and avoids coupling guest CF to
// AArch64 NZCV.
func x86ARM64EmitGrp2CLCarryRotateReg(cb *CodeBuffer, ji X86JITInstr) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 || (ji.grpOp != 2 && ji.grpOp != 3) {
		return false
	}
	var width uint32
	var guest byte
	switch ji.opcode {
	case 0xD2:
		if ji.prefixes != 0 {
			return false
		}
		width, guest = 8, ji.modrm&7 // retain AH/CH/DH/BH selector
	case 0xD3:
		if ji.prefixes == x86PrefOpSize {
			width = 16
		} else if ji.prefixes == 0 {
			width = 32
		} else {
			return false
		}
		guest = ji.modrm & 7
	default:
		return false
	}

	x86ARM64EmitLoadReg(cb, 18, 1) // ECX
	x86ARM64EmitMovImm32(cb, 12, 31)
	cb.Emit32(arm64AND_W(18, 18, 12))
	if width != 32 {
		modulus := width + 1
		reduce := cb.Len()
		x86ARM64EmitMovImm32(cb, 12, modulus)
		cb.Emit32(arm64CMP_W(18, 12))
		doneReduce := cb.Len()
		cb.Emit32(arm64Bcond(arm64CondLO, 0))
		cb.Emit32(arm64SUB_W(18, 18, 12))
		back := cb.Len()
		cb.Emit32(arm64B(0))
		endReduce := cb.Len()
		cb.PatchUint32(doneReduce, arm64Bcond(arm64CondLO, int32(endReduce-doneReduce)))
		cb.PatchUint32(back, arm64B(int32(reduce-back)))
	}
	zero := cb.Len()
	cb.Emit32(arm64CBZ(18, 0))
	cb.Emit32(arm64ORR_W(17, 18, 31)) // effective count for OF
	x86ARM64EmitPartialRegLoad(cb, 16, guest, width)
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(20, 14, 0))
	x86ARM64EmitMovImm32(cb, 12, x86FlagCF)
	cb.Emit32(arm64AND_W(19, 20, 12)) // carry-in as bit zero

	loop := cb.Len()
	if ji.grpOp == 2 { // RCL
		cb.Emit32(arm64LSR_W_imm(13, 16, width-1))
		cb.Emit32(arm64LSL_W_imm(16, 16, 1))
		cb.Emit32(arm64ORR_W(16, 16, 19))
	} else { // RCR
		x86ARM64EmitMovImm32(cb, 12, 1)
		cb.Emit32(arm64AND_W(13, 16, 12))
		cb.Emit32(arm64LSR_W_imm(16, 16, 1))
		cb.Emit32(arm64LSL_W_imm(12, 19, width-1))
		cb.Emit32(arm64ORR_W(16, 16, 12))
	}
	x86ARM64EmitTruncate(cb, 16, width)
	cb.Emit32(arm64ORR_W(19, 13, 31))
	cb.Emit32(arm64SUB_W_imm(18, 18, 1))
	back := cb.Len()
	cb.Emit32(arm64CBNZ(18, 0))
	cb.PatchUint32(back, arm64CBNZ(18, int32(loop-back)))

	if width == 8 {
		x86ARM64EmitLoadReg(cb, 13, guest&3)
		x86ARM64EmitByteInsert(cb, 13, 16, ji.modrm&7 >= 4)
		x86ARM64EmitStoreReg(cb, guest&3, 13)
	} else if width == 16 {
		x86ARM64EmitLoadReg(cb, 13, guest)
		x86ARM64EmitWordInsert(cb, 13, 16)
		x86ARM64EmitStoreReg(cb, guest, 13)
	} else {
		x86ARM64EmitStoreReg(cb, guest, 16)
	}

	// Rotates preserve every flag except CF and (only when count is one) OF.
	x86ARM64EmitMovImm32(cb, 10, ^uint32(x86FlagCF|x86FlagOF))
	cb.Emit32(arm64AND_W(10, 20, 10))
	x86ARM64EmitMovImm32(cb, 12, 1)
	cb.Emit32(arm64CMP_W(17, 12))
	notOne := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondNE, 0))
	if ji.grpOp == 2 {
		cb.Emit32(arm64LSR_W_imm(21, 16, width-1))
		cb.Emit32(arm64EOR_W(21, 21, 19))
	} else {
		cb.Emit32(arm64LSR_W_imm(21, 16, width-1))
		cb.Emit32(arm64LSR_W_imm(12, 16, width-2))
		x86ARM64EmitMovImm32(cb, 13, 1)
		cb.Emit32(arm64AND_W(12, 12, 13))
		cb.Emit32(arm64EOR_W(21, 21, 12))
	}
	cb.Emit32(arm64LSL_W_imm(21, 21, 11))
	cb.Emit32(arm64ORR_W(10, 10, 21))
	afterOF := cb.Len()
	cb.Emit32(arm64B(0))
	oldOF := cb.Len()
	x86ARM64EmitMovImm32(cb, 12, x86FlagOF)
	cb.Emit32(arm64AND_W(21, 20, 12))
	cb.Emit32(arm64ORR_W(10, 10, 21))
	end := cb.Len()
	cb.PatchUint32(notOne, arm64Bcond(arm64CondNE, int32(oldOF-notOne)))
	cb.PatchUint32(afterOF, arm64B(int32(end-afterOF)))
	cb.Emit32(arm64ORR_W(10, 10, 19))
	cb.Emit32(arm64STR_W_imm(10, 14, 0))
	endNoop := cb.Len()
	cb.PatchUint32(zero, arm64CBZ(18, int32(endNoop-zero)))
	return true
}

// x86ARM64EmitGrp2CLRotateNarrowReg lowers byte and word ROL/ROR register
// forms.  Dword ROL/ROR already use the variable-rotate lowering above; the
// narrow forms need their own modulo-width count before execution.
func x86ARM64EmitGrp2CLRotateNarrowReg(cb *CodeBuffer, ji X86JITInstr) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 || (ji.grpOp != 0 && ji.grpOp != 1) {
		return false
	}
	var width uint32
	var guest byte
	switch ji.opcode {
	case 0xD2:
		if ji.prefixes != 0 {
			return false
		}
		width, guest = 8, ji.modrm&7
	case 0xD3:
		if ji.prefixes != x86PrefOpSize {
			return false
		}
		width, guest = 16, ji.modrm&7
	default:
		return false
	}
	x86ARM64EmitLoadReg(cb, 18, 1)
	// The interpreter first applies the architectural five-bit mask.  Do not
	// reduce this to the operand width before deciding whether the operation is
	// a no-op: CL=8 for byte and CL=16 for word ROL/ROR still recompute CF from
	// the unchanged result, while only a zero five-bit count preserves flags.
	x86ARM64EmitMovImm32(cb, 12, 31)
	cb.Emit32(arm64AND_W(18, 18, 12))
	zero := cb.Len()
	cb.Emit32(arm64CBZ(18, 0))
	// Keep W18 as the raw five-bit count so full-width rotations update CF;
	// W17 is the interpreter's width-reduced count, used only for OF.
	cb.Emit32(arm64ORR_W(17, 18, 31))
	x86ARM64EmitMovImm32(cb, 12, width-1)
	cb.Emit32(arm64AND_W(17, 17, 12))
	x86ARM64EmitPartialRegLoad(cb, 16, guest, width)
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(20, 14, 0))
	loop := cb.Len()
	if ji.grpOp == 0 { // ROL
		cb.Emit32(arm64LSR_W_imm(19, 16, width-1))
		cb.Emit32(arm64LSL_W_imm(16, 16, 1))
		cb.Emit32(arm64ORR_W(16, 16, 19))
	} else { // ROR
		x86ARM64EmitMovImm32(cb, 12, 1)
		cb.Emit32(arm64AND_W(19, 16, 12))
		cb.Emit32(arm64LSR_W_imm(16, 16, 1))
		cb.Emit32(arm64LSL_W_imm(12, 19, width-1))
		cb.Emit32(arm64ORR_W(16, 16, 12))
	}
	x86ARM64EmitTruncate(cb, 16, width)
	cb.Emit32(arm64SUB_W_imm(18, 18, 1))
	back := cb.Len()
	cb.Emit32(arm64CBNZ(18, 0))
	cb.PatchUint32(back, arm64CBNZ(18, int32(loop-back)))
	if width == 8 {
		x86ARM64EmitLoadReg(cb, 13, guest&3)
		x86ARM64EmitByteInsert(cb, 13, 16, guest >= 4)
		x86ARM64EmitStoreReg(cb, guest&3, 13)
	} else {
		x86ARM64EmitLoadReg(cb, 13, guest)
		x86ARM64EmitWordInsert(cb, 13, 16)
		x86ARM64EmitStoreReg(cb, guest, 13)
	}
	x86ARM64EmitMovImm32(cb, 10, ^uint32(x86FlagCF|x86FlagOF))
	cb.Emit32(arm64AND_W(10, 20, 10))
	x86ARM64EmitMovImm32(cb, 12, 1)
	cb.Emit32(arm64CMP_W(17, 12))
	notOne := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondNE, 0))
	if ji.grpOp == 0 {
		cb.Emit32(arm64LSR_W_imm(21, 16, width-1))
		cb.Emit32(arm64EOR_W(21, 21, 19))
	} else {
		cb.Emit32(arm64LSR_W_imm(21, 16, width-1))
		cb.Emit32(arm64LSR_W_imm(12, 16, width-2))
		x86ARM64EmitMovImm32(cb, 13, 1)
		cb.Emit32(arm64AND_W(12, 12, 13))
		cb.Emit32(arm64EOR_W(21, 21, 12))
	}
	cb.Emit32(arm64LSL_W_imm(21, 21, 11))
	cb.Emit32(arm64ORR_W(10, 10, 21))
	afterOF := cb.Len()
	cb.Emit32(arm64B(0))
	oldOF := cb.Len()
	x86ARM64EmitMovImm32(cb, 12, x86FlagOF)
	cb.Emit32(arm64AND_W(21, 20, 12))
	cb.Emit32(arm64ORR_W(10, 10, 21))
	end := cb.Len()
	cb.PatchUint32(notOne, arm64Bcond(arm64CondNE, int32(oldOF-notOne)))
	cb.PatchUint32(afterOF, arm64B(int32(end-afterOF)))
	cb.Emit32(arm64ORR_W(10, 10, 19))
	cb.Emit32(arm64STR_W_imm(10, 14, 0))
	endNoop := cb.Len()
	cb.PatchUint32(zero, arm64CBZ(18, int32(endNoop-zero)))
	return true
}

// x86ARM64EmitArithFlags publishes the i386 arithmetic flag subset using
// already-truncated operands.  The host NZCV register is only used for the
// unsigned comparisons; AF and parity follow the interpreter's explicit
// formulas so byte and word operations do not inherit 32-bit host semantics.
// It clobbers W10-W12 and W14-W15, but preserves result, a and b.
func x86ARM64EmitArithFlags(cb *CodeBuffer, result, a, b byte, width uint32, sub bool) {
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, ^uint32(x86FlagCF|x86FlagPF|x86FlagAF|x86FlagZF|x86FlagSF|x86FlagOF))
	cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))

	// CF is a carry for add and a borrow for subtract.
	if sub {
		cb.Emit32(arm64CMP_W(a, b))
	} else {
		cb.Emit32(arm64CMP_W(result, a))
	}
	cb.Emit32(arm64CSET_W(x86ARM64Scratch2, arm64CondLO))
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))

	// AF is bit four of a xor b xor result for both addition and subtraction.
	cb.Emit32(arm64EOR_W(15, a, b))
	cb.Emit32(arm64EOR_W(15, 15, result))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, x86FlagAF)
	cb.Emit32(arm64AND_W(15, 15, x86ARM64Scratch2))
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 15))

	cb.Emit32(arm64CMP_W(result, 31))
	cb.Emit32(arm64CSET_W(x86ARM64Scratch2, arm64CondEQ))
	cb.Emit32(arm64LSL_W_imm(x86ARM64Scratch2, x86ARM64Scratch2, 6))
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	cb.Emit32(arm64LSR_W_imm(x86ARM64Scratch2, result, width-1))
	x86ARM64EmitMovImm32(cb, 12, 1)
	cb.Emit32(arm64AND_W(x86ARM64Scratch2, x86ARM64Scratch2, 12))
	cb.Emit32(arm64LSL_W_imm(x86ARM64Scratch2, x86ARM64Scratch2, 7))
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))

	// OF is ((a^b) xor all-ones for add) & (a^result) at the sign bit.
	cb.Emit32(arm64EOR_W(15, a, b))
	if !sub {
		cb.Emit32(0x2A2003EF | uint32(15)<<16) // MVN W15,W15
	}
	cb.Emit32(arm64EOR_W(12, a, result))
	cb.Emit32(arm64AND_W(15, 15, 12))
	cb.Emit32(arm64LSR_W_imm(15, 15, width-1))
	x86ARM64EmitMovImm32(cb, 12, 1)
	cb.Emit32(arm64AND_W(15, 15, 12))
	cb.Emit32(arm64LSL_W_imm(15, 15, 11))
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 15))

	// Fold low-byte parity, as in x86ARM64EmitLogicFlags.
	cb.Emit32(0x53000000 | 7<<10 | uint32(result)<<5 | 12) // UBFX W12,Wresult,#0,#8
	cb.Emit32(arm64LSR_W_imm(x86ARM64Scratch2, 12, 4))
	cb.Emit32(arm64EOR_W(12, 12, x86ARM64Scratch2))
	cb.Emit32(arm64LSR_W_imm(x86ARM64Scratch2, 12, 2))
	cb.Emit32(arm64EOR_W(12, 12, x86ARM64Scratch2))
	cb.Emit32(arm64LSR_W_imm(x86ARM64Scratch2, 12, 1))
	cb.Emit32(arm64EOR_W(12, 12, x86ARM64Scratch2))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 1)
	cb.Emit32(arm64AND_W(12, 12, x86ARM64Scratch2))
	cb.Emit32(arm64EOR_W(12, 12, x86ARM64Scratch2))
	cb.Emit32(arm64LSL_W_imm(12, 12, 2))
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 12))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
}

// x86ARM64EmitMulOverflowFlags updates only CF and OF, as the interpreter's
// IMUL handlers do. overflow is a 0/1 register value.
func x86ARM64EmitMulOverflowFlags(cb *CodeBuffer, overflow byte) {
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, ^uint32(x86FlagCF|x86FlagOF))
	cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, overflow))
	cb.Emit32(arm64LSL_W_imm(x86ARM64Scratch2, overflow, 11))
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
}

func x86ARM64EmitStoreByte(cb *CodeBuffer, off uint32, value byte) {
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch, uint32(value))
	cb.Emit32(arm64STRB_imm(x86ARM64Scratch, x86ARM64RegCtx, off))
}

func x86ARM64FindModRMPC(ji X86JITInstr, memory []byte) int {
	pc := int(ji.opcodePC)
	for pc < len(memory) {
		switch memory[pc] {
		case 0x26, 0x2E, 0x36, 0x3E, 0x64, 0x65, 0x66, 0x67, 0xF0, 0xF2, 0xF3:
			pc++
			continue
		}
		break
	}
	if pc >= len(memory) {
		return -1
	}
	if memory[pc] == 0x0F {
		pc += 2
	} else {
		pc++
	}
	if pc >= len(memory) {
		return -1
	}
	return pc
}

// x86ARM64EmitEA32 emits the project interpreter's flat 32-bit addressing
// calculation. It deliberately performs no memory access, so LEA can reuse it
// without span/MMIO/SMC machinery and x87 helper exits can retain their own
// guarded interpreter replay model.
func x86ARM64EmitEA32(cb *CodeBuffer, ji X86JITInstr, memory []byte, dst byte) bool {
	if !ji.hasModRM || ji.prefixes&x86PrefAddrSize != 0 {
		return false
	}
	mod, rm := ji.modrm>>6, ji.modrm&7
	if mod == 3 {
		return false
	}
	modrmPC := x86ARM64FindModRMPC(ji, memory)
	if modrmPC < 0 || modrmPC >= len(memory) {
		return false
	}
	pos := modrmPC + 1
	if rm == 4 {
		if pos >= len(memory) {
			return false
		}
		sib := memory[pos]
		pos++
		base, index, scale := sib&7, (sib>>3)&7, sib>>6
		if mod == 0 && base == 5 {
			x86ARM64EmitMovImm32(cb, dst, 0)
		} else {
			x86ARM64EmitLoadReg(cb, dst, base)
		}
		if index != 4 {
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, index)
			if scale != 0 {
				x86ARM64EmitMovImm32(cb, 12, uint32(scale))
				cb.Emit32(arm64LSL_W(x86ARM64Scratch2, x86ARM64Scratch2, 12))
			}
			cb.Emit32(arm64ADD_W(dst, dst, x86ARM64Scratch2))
		}
	} else if mod == 0 && rm == 5 {
		x86ARM64EmitMovImm32(cb, dst, 0)
	} else {
		x86ARM64EmitLoadReg(cb, dst, rm)
	}
	dispBytes := 0
	if mod == 1 {
		dispBytes = 1
	} else if mod == 2 || (mod == 0 && (rm == 5 || (rm == 4 && memory[modrmPC+1]&7 == 5))) {
		dispBytes = 4
	}
	if pos+dispBytes > len(memory) {
		return false
	}
	var disp uint32
	if dispBytes == 1 {
		disp = uint32(int32(int8(memory[pos])))
	} else if dispBytes == 4 {
		disp = binary.LittleEndian.Uint32(memory[pos : pos+4])
	}
	if disp != 0 {
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, disp)
		cb.Emit32(arm64ADD_W(dst, dst, x86ARM64Scratch2))
	}
	return true
}

// x86ARM64EADependsOnESP identifies the POP r/m forms whose destination EA
// the interpreter computes after incrementing ESP. They remain at the helper
// boundary until that post-pop addressing case has a dedicated cold exit.
func x86ARM64EADependsOnESP(ji X86JITInstr, memory []byte) bool {
	if !ji.hasModRM || ji.modrm>>6 == 3 || ji.modrm&7 != 4 {
		return false
	}
	modrmPC := x86ARM64FindModRMPC(ji, memory)
	if modrmPC < 0 || modrmPC+1 >= len(memory) {
		return true
	}
	sib := memory[modrmPC+1]
	return sib&7 == 4
}

func x86ARM64EmitDeferredBail(cb *CodeBuffer, bails *[]x86ARM64DeferredBail, retPC uint32, retCount int) {
	off := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondHS, 0))
	*bails = append(*bails, x86ARM64DeferredBail{branchOffset: off, retPC: retPC, retCount: retCount})
}

// x86ARM64EmitSpanGuard validates a non-mutating guest-memory access before
// it can touch the host backing. As on amd64, a cross-page, out-of-range or
// MMIO access returns to the interpreter before the guest instruction runs.
// X10 retains EA; X11 and X12 are scratch.
func x86ARM64EmitSpanGuard(cb *CodeBuffer, size uint32, retPC uint32, retCount int, bails *[]x86ARM64DeferredBail) {
	if size == 0 || size > 256 {
		panic("invalid ARM64 x86 memory span")
	}
	cb.Emit32(arm64ORR_W(x86ARM64Scratch2, x86ARM64Scratch, 31))
	x86ARM64EmitMovImm32(cb, 12, 0xFF)
	cb.Emit32(arm64AND_W(x86ARM64Scratch2, x86ARM64Scratch2, 12))
	x86ARM64EmitMovImm32(cb, 12, 0x101-size)
	cb.Emit32(arm64CMP_W(x86ARM64Scratch2, 12))
	x86ARM64EmitDeferredBail(cb, bails, retPC, retCount)

	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemSize/4))
	x86ARM64EmitMovImm32(cb, 12, size-1)
	cb.Emit32(arm64SUB_W(x86ARM64Scratch2, x86ARM64Scratch2, 12))
	cb.Emit32(arm64CMP_W(x86ARM64Scratch, x86ARM64Scratch2))
	x86ARM64EmitDeferredBail(cb, bails, retPC, retCount)

	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffIOBitmapPtr/8))
	cb.Emit32(arm64LSR_W_imm(12, x86ARM64Scratch, 8))
	cb.Emit32(arm64LDRB_reg(x86ARM64Scratch2, x86ARM64Scratch2, 12))
	off := cb.Len()
	cb.Emit32(arm64CBNZ(x86ARM64Scratch2, 0))
	*bails = append(*bails, x86ARM64DeferredBail{branchOffset: off, retPC: retPC, retCount: retCount, mmio: true})
}

func x86ARM64EmitDeferredBails(cb *CodeBuffer, bails []x86ARM64DeferredBail) {
	for _, bail := range bails {
		stub := cb.Len()
		if bail.mmio || bail.inval {
			cb.PatchUint32(bail.branchOffset, arm64CBNZ(x86ARM64Scratch2, int32(stub-bail.branchOffset)))
		} else {
			cb.PatchUint32(bail.branchOffset, arm64Bcond(arm64CondHS, int32(stub-bail.branchOffset)))
		}
		if bail.inval {
			// W10 still contains the store EA on this taken edge.
			cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffInvalAddr/4))
			x86ARM64EmitMovImm32(cb, x86ARM64Scratch, bail.invalSize)
			cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffInvalSize/4))
		}
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch, bail.retPC)
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffRetPC/4))
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch, uint32(bail.retCount))
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffChainCount/4))
		cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffRetCount/4))
		if bail.inval {
			x86ARM64EmitMovImm32(cb, x86ARM64Scratch, 1)
			cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffNeedInval/4))
		} else {
			x86ARM64EmitMovImm32(cb, x86ARM64Scratch, 1)
			cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffNeedIOFallback/4))
		}
		cb.Emit32(arm64RET())
	}
}

// x86ARM64EmitSMCStoreCheck records a completed store to a compiled 256-byte
// code page and exits before the next guest instruction can execute stale
// native code. A nil bitmap means no native blocks are currently published.
func x86ARM64EmitSMCStoreCheck(cb *CodeBuffer, size uint32, nextPC uint32, retired int, bails *[]x86ARM64DeferredBail) {
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffCodePageBitmapPtr/8))
	noBitmap := cb.Len()
	cb.Emit32(arm64CBZ(x86ARM64Scratch2, 0))
	cb.Emit32(arm64LSR_W_imm(12, x86ARM64Scratch, 8))
	cb.Emit32(arm64LDRB_reg(x86ARM64Scratch2, x86ARM64Scratch2, 12))
	hit := cb.Len()
	cb.Emit32(arm64CBNZ(x86ARM64Scratch2, 0))
	end := cb.Len()
	cb.PatchUint32(noBitmap, arm64CBZ(x86ARM64Scratch2, int32(end-noBitmap)))
	*bails = append(*bails, x86ARM64DeferredBail{branchOffset: hit, retPC: nextPC, retCount: retired, inval: true, invalSize: size})
}

// x86ARM64FPUHelperSegment derives the interpreter's default data segment
// from a decoded 32-bit ModR/M address. Segment bases are flat in this model,
// but FDS and the helper payload retain the selected register value.
func x86ARM64FPUHelperSegment(p x86FPUHelperPayload) (byte, bool) {
	// Address-size affects only memory EA decoding. For a register form it is
	// architecturally inert, so retain canonical helper admission and the
	// interpreter's conventional DS payload segment.
	if p.Prefixes&x86PrefAddrSize != 0 && p.ModRM>>6 == 3 {
		return x86SegDS, true
	}
	if p.Prefixes&x86PrefAddrSize != 0 {
		return 0, false
	}
	seg := byte(x86SegDS)
	for i := 0; i < int(p.Length); i++ {
		switch p.Bytes[i] {
		case 0x26:
			seg = x86SegES
		case 0x2E:
			seg = x86SegCS
		case 0x36:
			seg = x86SegSS
		case 0x3E:
			seg = x86SegDS
		case 0x64:
			seg = x86SegFS
		case 0x65:
			seg = x86SegGS
		case 0xD8, 0xD9, 0xDA, 0xDB, 0xDC, 0xDD, 0xDE, 0xDF:
			goto decoded
		}
	}
	return 0, false
decoded:
	mod, rm := p.ModRM>>6, p.ModRM&7
	if mod == 3 {
		return x86SegDS, true
	}
	if p.Prefixes&x86PrefSeg != 0 {
		return seg, true
	}
	if rm == 4 {
		modrmPC := x86FindModRMPCFromPayload(p)
		if modrmPC+1 >= int(p.Length) {
			return 0, false
		}
		base := p.Bytes[modrmPC+1] & 7
		if !(mod == 0 && base == 5) && (base == 4 || base == 5) {
			return x86SegSS, true
		}
	} else if mod != 0 && (rm == 4 || rm == 5) {
		return x86SegSS, true
	}
	return seg, true
}

func x86FindModRMPCFromPayload(p x86FPUHelperPayload) int {
	for i := 0; i < int(p.Length); i++ {
		if p.Bytes[i] >= 0xD8 && p.Bytes[i] <= 0xDF {
			return i + 1
		}
	}
	return -1
}

// x86ARM64EmitFPUHelperEA computes the interpreter's flat 32-bit EA without
// touching guest memory. The helper owns bounds, MMIO and fault semantics.
func x86ARM64EmitFPUHelperEA(cb *CodeBuffer, p x86FPUHelperPayload) bool {
	if p.ModRM>>6 == 3 || p.Prefixes&x86PrefAddrSize != 0 {
		return false
	}
	modrmPC := x86FindModRMPCFromPayload(p)
	if modrmPC < 1 || modrmPC >= int(p.Length) {
		return false
	}
	mod, rm := p.ModRM>>6, p.ModRM&7
	pos := modrmPC + 1
	if rm == 4 {
		if pos >= int(p.Length) {
			return false
		}
		sib := p.Bytes[pos]
		pos++
		base, index, scale := sib&7, (sib>>3)&7, sib>>6
		if mod == 0 && base == 5 {
			x86ARM64EmitMovImm32(cb, x86ARM64Scratch, 0)
		} else {
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, base)
		}
		if index != 4 {
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, index)
			if scale != 0 {
				x86ARM64EmitMovImm32(cb, 12, uint32(scale))
				cb.Emit32(arm64LSL_W(x86ARM64Scratch2, x86ARM64Scratch2, 12))
			}
			cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		}
	} else if mod == 0 && rm == 5 {
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch, 0)
	} else {
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, rm)
	}
	dispBytes := 0
	if mod == 1 {
		dispBytes = 1
	} else if mod == 2 || (mod == 0 && ((rm == 5) || (rm == 4 && p.Bytes[modrmPC+1]&7 == 5))) {
		dispBytes = 4
	}
	if pos+dispBytes > int(p.Length) {
		return false
	}
	var disp uint32
	if dispBytes == 1 {
		disp = uint32(int32(int8(p.Bytes[pos])))
	} else if dispBytes == 4 {
		disp = binary.LittleEndian.Uint32(p.Bytes[pos : pos+4])
	}
	if disp != 0 {
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, disp)
		cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	}
	return true
}

// x86ARM64EmitFPUHelperExit publishes the immutable decoder-owned payload and
// returns before it mutates x87 state. It admits register forms and absolute
// disp32 memory forms. The latter has an architecturally complete live EA
// without dereferencing native guest memory; the interpreter still owns the
// eventual bounds, MMIO and exception behaviour.
func x86ARM64EmitFPUHelperExit(cb *CodeBuffer, p x86FPUHelperPayload, retired int) bool {
	memForm := p.ModRM>>6 != 3
	segment, ok := x86ARM64FPUHelperSegment(p)
	if !ok {
		return false
	}
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch, p.InstrPC)
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUHelperInstrPC/4))
	// A preceding native MOV Sreg can update the live selector in the same
	// block. Capture CS from jitSegRegs at the helper exit, not the compile-time
	// CPU snapshot used for immutable instruction bytes.
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
	cb.Emit32(arm64LDRH_imm(x86ARM64Scratch, x86ARM64Scratch2, uint32(x86SegCS)))
	cb.Emit32(arm64STRH_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUHelperCS/2))
	x86ARM64EmitStoreByte(cb, x86CtxOffFPUHelperEscape, p.Escape)
	x86ARM64EmitStoreByte(cb, x86CtxOffFPUHelperModRM, p.ModRM)
	x86ARM64EmitStoreByte(cb, x86CtxOffFPUHelperPrefixes, p.Prefixes)
	x86ARM64EmitStoreByte(cb, x86CtxOffFPUHelperLength, p.Length)
	x86ARM64EmitStoreByte(cb, x86CtxOffFPUHelperSegment, segment)
	for off, b := range p.Bytes {
		x86ARM64EmitStoreByte(cb, x86CtxOffFPUHelperBytes+uint32(off), b)
	}
	// Register forms intentionally have no memory provenance. Memory forms
	// publish their live EA before returning to the interpreter helper.
	width := uint32(0)
	if memForm {
		if !x86ARM64EmitFPUHelperEA(cb, p) {
			return false
		}
		width = x86FPUHelperAccessWidthFromOpcode(p.Escape, p.ModRM)
	} else {
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch, 0)
	}
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUHelperEA/4))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch, width)
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUHelperWidth/4))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch, x86JITExitFPUHelper)
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffExitReason/4))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch, 1)
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffNeedIOFallback/4))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch, p.InstrPC)
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffRetPC/4))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch, uint32(retired))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffChainCount/4))
	cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffRetCount/4))
	cb.Emit32(arm64RET())
	return true
}

// x86ARM64EmitFNOP performs the no-stack-effect x87 form directly while
// retaining captureOp provenance. Unlike arithmetic or stack forms it cannot
// fault on tags or require an x87 exception-state cold exit.
func x86ARM64EmitFNOP(cb *CodeBuffer, ji X86JITInstr, memory []byte) bool {
	if ji.opcode != 0xD9 || ji.modrm != 0xD0 {
		return false
	}
	modrmPC := x86ARM64FindModRMPC(ji, memory)
	if modrmPC < 1 {
		return false
	}
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUPtr/8))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(modrmPC-1))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFIP/4))
	cb.Emit32(arm64LDR_imm(12, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
	cb.Emit32(arm64LDRH_imm(13, 12, uint32(x86SegCS)))
	cb.Emit32(arm64STRH_imm(13, x86ARM64Scratch, x86ARM64FPUOffFCS/2))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32((uint16(1)<<8)|uint16(ji.modrm)))
	cb.Emit32(arm64STRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFOP/2))
	return true
}

// x86ARM64EmitFFREE lowers DD C0+i. FFREE only changes the selected physical
// tag to empty, so it has no operand fault or floating-point rounding path.
func x86ARM64EmitFFREE(cb *CodeBuffer, ji X86JITInstr, memory []byte) bool {
	if ji.opcode != 0xDD || ji.modrm>>6 != 3 || (ji.modrm>>3)&7 != 0 {
		return false
	}
	modrmPC := x86ARM64FindModRMPC(ji, memory)
	if modrmPC < 1 {
		return false
	}
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUPtr/8))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(modrmPC-1))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFIP/4))
	cb.Emit32(arm64LDR_imm(12, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
	cb.Emit32(arm64LDRH_imm(13, 12, uint32(x86SegCS)))
	cb.Emit32(arm64STRH_imm(13, x86ARM64Scratch, x86ARM64FPUOffFCS/2))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32((uint16(5)<<8)|uint16(ji.modrm)))
	cb.Emit32(arm64STRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFOP/2))

	// physical = (TOP + i) & 7; FTW holds two tag bits per physical slot.
	cb.Emit32(arm64LDRH_imm(11, x86ARM64Scratch, x86ARM64FPUOffFSW/2))
	cb.Emit32(0x53000000 | 11<<16 | 13<<10 | 11<<5 | 11) // UBFX W11,W11,#11,#3
	if i := ji.modrm & 7; i != 0 {
		x86ARM64EmitMovImm32(cb, 12, uint32(i))
		cb.Emit32(arm64ADD_W(11, 11, 12))
		x86ARM64EmitMovImm32(cb, 12, 7)
		cb.Emit32(arm64AND_W(11, 11, 12))
	}
	cb.Emit32(arm64LSL_W_imm(11, 11, 1))
	cb.Emit32(arm64LDRH_imm(12, x86ARM64Scratch, x86ARM64FPUOffFTW/2))
	x86ARM64EmitMovImm32(cb, 13, 3)
	cb.Emit32(arm64LSLV_W(13, 13, 11))
	cb.Emit32(arm64ORR_W(12, 12, 13))
	cb.Emit32(arm64STRH_imm(12, x86ARM64Scratch, x86ARM64FPUOffFTW/2))
	return true
}

// x86ARM64EmitFXCH lowers D9 C8+i after proving both logical operands are
// non-empty. Empty-stack fault state remains in the canonical helper.
func x86ARM64EmitFXCH(cb *CodeBuffer, ji X86JITInstr, memory []byte, retired int) bool {
	if ji.opcode != 0xD9 || ji.modrm < 0xC8 || ji.modrm > 0xCF {
		return false
	}
	payload, ok := x86FPUHelperPayloadFor(ji, memory, 0)
	if !ok {
		return false
	}
	modrmPC := x86ARM64FindModRMPC(ji, memory)
	if modrmPC < 1 {
		return false
	}
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUPtr/8))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(modrmPC-1))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFIP/4))
	cb.Emit32(arm64LDR_imm(12, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
	cb.Emit32(arm64LDRH_imm(13, 12, uint32(x86SegCS)))
	cb.Emit32(arm64STRH_imm(13, x86ARM64Scratch, x86ARM64FPUOffFCS/2))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32((uint16(1)<<8)|uint16(ji.modrm)))
	cb.Emit32(arm64STRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFOP/2))

	cb.Emit32(arm64LDRH_imm(11, x86ARM64Scratch, x86ARM64FPUOffFSW/2))
	cb.Emit32(0x53000000 | 11<<16 | 13<<10 | 11<<5 | 11) // TOP
	cb.Emit32(arm64ORR_W(14, 11, 31))
	if i := ji.modrm & 7; i != 0 {
		x86ARM64EmitMovImm32(cb, 12, uint32(i))
		cb.Emit32(arm64ADD_W(14, 14, 12))
		x86ARM64EmitMovImm32(cb, 12, 7)
		cb.Emit32(arm64AND_W(14, 14, 12))
	}
	cb.Emit32(arm64LDRH_imm(15, x86ARM64Scratch, x86ARM64FPUOffFTW/2))
	check := func(phys byte) int {
		cb.Emit32(arm64LSL_W_imm(12, phys, 1))
		cb.Emit32(arm64LSR_W(13, 15, 12))
		x86ARM64EmitMovImm32(cb, 12, 3)
		cb.Emit32(arm64AND_W(13, 13, 12))
		cb.Emit32(arm64CMP_W(13, 12))
		off := cb.Len()
		cb.Emit32(arm64Bcond(arm64CondEQ, 0))
		return off
	}
	branches := []int{check(11)}
	if ji.modrm&7 != 0 {
		branches = append(branches, check(14))
	}
	cb.Emit32(arm64LSL_W_imm(11, 11, 3))
	cb.Emit32(arm64LSL_W_imm(14, 14, 3))
	cb.Emit32(arm64LDR_reg(12, x86ARM64Scratch, 11))
	cb.Emit32(arm64LDR_reg(13, x86ARM64Scratch, 14))
	cb.Emit32(arm64STR_reg(13, x86ARM64Scratch, 11))
	cb.Emit32(arm64STR_reg(12, x86ARM64Scratch, 14))
	done := cb.Len()
	cb.Emit32(arm64B(0))
	cold := cb.Len()
	if !x86ARM64EmitFPUHelperExit(cb, payload, retired) {
		return false
	}
	end := cb.Len()
	for _, branch := range branches {
		cb.PatchUint32(branch, arm64Bcond(arm64CondEQ, int32(cold-branch)))
	}
	cb.PatchUint32(done, arm64B(int32(end-done)))
	return true
}

// x86ARM64EmitFSTPSTi lowers DD D8+i. It copies ST(0)'s value and tag to
// ST(i), marks the old top empty and advances TOP. Empty operands retain the
// interpreter's exception path through the canonical helper.
func x86ARM64EmitFSTPSTi(cb *CodeBuffer, ji X86JITInstr, memory []byte, retired int) bool {
	if ji.opcode != 0xDD || ji.modrm < 0xD8 || ji.modrm > 0xDF {
		return false
	}
	p, ok := x86FPUHelperPayloadFor(ji, memory, 0)
	if !ok {
		return false
	}
	m := x86ARM64FindModRMPC(ji, memory)
	if m < 1 {
		return false
	}
	cb.Emit32(arm64LDR_imm(10, 0, x86CtxOffFPUPtr/8))
	x86ARM64EmitMovImm32(cb, 11, uint32(m-1))
	cb.Emit32(arm64STR_W_imm(11, 10, x86ARM64FPUOffFIP/4))
	cb.Emit32(arm64LDR_imm(12, 0, x86CtxOffSegRegsPtr/8))
	cb.Emit32(arm64LDRH_imm(13, 12, uint32(x86SegCS)))
	cb.Emit32(arm64STRH_imm(13, 10, x86ARM64FPUOffFCS/2))
	x86ARM64EmitMovImm32(cb, 11, uint32((uint16(5)<<8)|uint16(ji.modrm)))
	cb.Emit32(arm64STRH_imm(11, 10, x86ARM64FPUOffFOP/2))
	cb.Emit32(arm64LDRH_imm(11, 10, x86ARM64FPUOffFSW/2))
	cb.Emit32(0x53000000 | 11<<16 | 13<<10 | 11<<5 | 11)
	cb.Emit32(arm64ORR_W(14, 11, 31))
	if i := ji.modrm & 7; i != 0 {
		x86ARM64EmitMovImm32(cb, 12, uint32(i))
		cb.Emit32(arm64ADD_W(14, 14, 12))
		x86ARM64EmitMovImm32(cb, 12, 7)
		cb.Emit32(arm64AND_W(14, 14, 12))
	}
	cb.Emit32(arm64LDRH_imm(15, 10, x86ARM64FPUOffFTW/2))
	check := func(phys byte) int {
		cb.Emit32(arm64LSL_W_imm(12, phys, 1))
		cb.Emit32(arm64LSR_W(13, 15, 12))
		x86ARM64EmitMovImm32(cb, 12, 3)
		cb.Emit32(arm64AND_W(13, 13, 12))
		cb.Emit32(arm64CMP_W(13, 12))
		off := cb.Len()
		cb.Emit32(arm64Bcond(arm64CondEQ, 0))
		return off
	}
	branches := []int{check(11)}
	if ji.modrm&7 != 0 {
		branches = append(branches, check(14))
	}
	// Copy the value and tag class before advancing TOP.  A direct successor
	// can observe the new ST(0) without returning through the dispatcher, so
	// normalising tags only at the block boundary would expose stale state.
	cb.Emit32(arm64LSL_W_imm(12, 11, 1))
	cb.Emit32(arm64LSR_W(13, 15, 12))
	x86ARM64EmitMovImm32(cb, 16, 3)
	cb.Emit32(arm64AND_W(13, 13, 16)) // source tag

	// Replace the destination's two FTW bits with the source tag.
	cb.Emit32(arm64LSL_W_imm(12, 14, 1))
	cb.Emit32(arm64LSLV_W(16, 16, 12))
	cb.Emit32(arm64MVN_W(16, 16))
	cb.Emit32(arm64AND_W(15, 15, 16))
	cb.Emit32(arm64LSLV_W(13, 13, 12))
	cb.Emit32(arm64ORR_W(15, 15, 13))

	// Empty the old physical ST(0) slot, including the i == 0 case.
	cb.Emit32(arm64LSL_W_imm(12, 11, 1))
	x86ARM64EmitMovImm32(cb, 16, 3)
	cb.Emit32(arm64LSLV_W(16, 16, 12))
	cb.Emit32(arm64ORR_W(15, 15, 16))
	cb.Emit32(arm64STRH_imm(15, 10, x86ARM64FPUOffFTW/2))

	cb.Emit32(arm64LSL_W_imm(12, 11, 3))
	cb.Emit32(arm64LDR_reg(13, 10, 12))
	cb.Emit32(arm64LSL_W_imm(14, 14, 3))
	cb.Emit32(arm64STR_reg(13, 10, 14))
	cb.Emit32(arm64LDRH_imm(13, 10, x86ARM64FPUOffFSW/2))
	x86ARM64EmitMovImm32(cb, 14, ^uint32(7<<11))
	cb.Emit32(arm64AND_W(13, 13, 14))
	cb.Emit32(arm64ADD_W_imm(11, 11, 1))
	x86ARM64EmitMovImm32(cb, 14, 7)
	cb.Emit32(arm64AND_W(11, 11, 14))
	cb.Emit32(arm64LSL_W_imm(11, 11, 11))
	cb.Emit32(arm64ORR_W(13, 13, 11))
	cb.Emit32(arm64STRH_imm(13, 10, x86ARM64FPUOffFSW/2))
	done := cb.Len()
	cb.Emit32(arm64B(0))
	cold := cb.Len()
	if !x86ARM64EmitFPUHelperExit(cb, p, retired) {
		return false
	}
	end := cb.Len()
	for _, b := range branches {
		cb.PatchUint32(b, arm64Bcond(arm64CondEQ, int32(cold-b)))
	}
	cb.PatchUint32(done, arm64B(int32(end-done)))
	return true
}

// x86ARM64EmitFCHSFABS lowers the two sign-only x87 unary operations. Their
// tag class is invariant under a sign change, including signed zero, NaN and
// infinity. An empty ST(0) remains an interpreter-owned stack-fault exit so
// the guest FSW/FCW exception contract is never reconstructed from host state.
func x86ARM64EmitFCHSFABS(cb *CodeBuffer, ji X86JITInstr, memory []byte, retired int) bool {
	if ji.opcode != 0xD9 || (ji.modrm != 0xE0 && ji.modrm != 0xE1) {
		return false
	}
	modrmPC := x86ARM64FindModRMPC(ji, memory)
	if modrmPC < 1 {
		return false
	}
	payload, ok := x86FPUHelperPayloadFor(ji, memory, 0)
	if !ok {
		return false
	}
	// captureOp provenance: FIP identifies the escape opcode after prefixes,
	// FCS is the live selector and FOP includes the D9 escape and ModR/M.
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUPtr/8))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(modrmPC-1))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFIP/4))
	cb.Emit32(arm64LDR_imm(12, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
	cb.Emit32(arm64LDRH_imm(13, 12, uint32(x86SegCS)))
	cb.Emit32(arm64STRH_imm(13, x86ARM64Scratch, x86ARM64FPUOffFCS/2))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32((uint16(1)<<8)|uint16(ji.modrm)))
	cb.Emit32(arm64STRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFOP/2))

	// W11 = physical ST(0) tag shift, namely TOP*2.
	cb.Emit32(arm64LDRH_imm(11, x86ARM64Scratch, x86ARM64FPUOffFSW/2))
	cb.Emit32(0x53000000 | 11<<16 | 13<<10 | 11<<5 | 11) // UBFX W11,W11,#11,#3
	cb.Emit32(arm64LSL_W_imm(11, 11, 1))
	cb.Emit32(arm64LDRH_imm(12, x86ARM64Scratch, x86ARM64FPUOffFTW/2))
	cb.Emit32(arm64LSR_W(12, 12, 11))
	x86ARM64EmitMovImm32(cb, 13, 3)
	cb.Emit32(arm64AND_W(12, 12, 13))
	cb.Emit32(arm64CMP_W(12, 13))
	empty := cb.Len()
	cb.Emit32(arm64Bcond(arm64CondEQ, 0))

	// Convert TOP*2 to the byte offset of its float64 backing slot and update
	// only its IEEE-754 sign bit. The FPU register array is at offset zero.
	cb.Emit32(arm64LSL_W_imm(11, 11, 2))
	cb.Emit32(arm64LDR_reg(12, x86ARM64Scratch, 11))
	if ji.modrm == 0xE0 { // FCHS
		cb.Emit32(arm64MOVZ(13, 0x8000, 48))
		cb.Emit32(arm64EOR(12, 12, 13))
	} else { // FABS
		cb.Emit32(arm64MOVZ(13, 0xFFFF, 0))
		cb.Emit32(arm64MOVK(13, 0xFFFF, 16))
		cb.Emit32(arm64MOVK(13, 0xFFFF, 32))
		cb.Emit32(arm64MOVK(13, 0x7FFF, 48))
		cb.Emit32(arm64AND(12, 12, 13))
	}
	cb.Emit32(arm64STR_reg(12, x86ARM64Scratch, 11))
	done := cb.Len()
	cb.Emit32(arm64B(0))
	cold := cb.Len()
	if !x86ARM64EmitFPUHelperExit(cb, payload, retired) {
		return false
	}
	end := cb.Len()
	cb.PatchUint32(empty, arm64Bcond(arm64CondEQ, int32(cold-empty)))
	cb.PatchUint32(done, arm64B(int32(end-done)))
	return true
}

// x86ARM64EmitBinaryST0STi lowers the finite normal register D8 arithmetic
// forms FADD, FMUL, FSUB, FSUBR, FDIV and FDIVR. Every tag transition and exceptional result
// stays on the canonical helper path before native code can publish a value.
func x86ARM64EmitBinaryST0STi(cb *CodeBuffer, ji X86JITInstr, memory []byte, retired int) bool {
	if ji.opcode != 0xD8 || ji.modrm>>6 != 3 {
		return false
	}
	op := (ji.modrm >> 3) & 7
	if op != 0 && op != 1 && op != 4 && op != 5 && op != 6 && op != 7 {
		return false
	}
	payload, ok := x86FPUHelperPayloadFor(ji, memory, 0)
	if !ok {
		return false
	}
	modrmPC := x86ARM64FindModRMPC(ji, memory)
	if modrmPC < 1 {
		return false
	}
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUPtr/8))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(modrmPC-1))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFIP/4))
	cb.Emit32(arm64LDR_imm(12, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
	cb.Emit32(arm64LDRH_imm(13, 12, uint32(x86SegCS)))
	cb.Emit32(arm64STRH_imm(13, x86ARM64Scratch, x86ARM64FPUOffFCS/2))
	// FOP records the escape opcode class (D8 maps to zero in the project's
	// captureOp encoding), not the arithmetic group in ModR/M.reg.
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(ji.modrm))
	cb.Emit32(arm64STRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFOP/2))

	// W11 = physical ST(0), W14 = physical ST(i). W15 retains the FTW word.
	cb.Emit32(arm64LDRH_imm(11, x86ARM64Scratch, x86ARM64FPUOffFSW/2))
	cb.Emit32(0x53000000 | 11<<16 | 13<<10 | 11<<5 | 11) // UBFX W11,W11,#11,#3
	cb.Emit32(arm64ORR_W(14, 11, 31))
	if i := ji.modrm & 7; i != 0 {
		x86ARM64EmitMovImm32(cb, 12, uint32(i))
		cb.Emit32(arm64ADD_W(14, 14, 12))
		x86ARM64EmitMovImm32(cb, 12, 7)
		cb.Emit32(arm64AND_W(14, 14, 12))
	}
	cb.Emit32(arm64LDRH_imm(15, x86ARM64Scratch, x86ARM64FPUOffFTW/2))
	checkTag := func(phys byte) []int {
		cb.Emit32(arm64LSL_W_imm(12, phys, 1))
		cb.Emit32(arm64LSR_W(13, 15, 12))
		x86ARM64EmitMovImm32(cb, 12, 3)
		cb.Emit32(arm64AND_W(13, 13, 12))
		var branches []int
		cb.Emit32(arm64CMP_W(13, 12))
		branches = append(branches, cb.Len())
		cb.Emit32(arm64Bcond(arm64CondEQ, 0)) // empty
		x86ARM64EmitMovImm32(cb, 12, 1)
		cb.Emit32(arm64CMP_W(13, 12))
		branches = append(branches, cb.Len())
		cb.Emit32(arm64Bcond(arm64CondEQ, 0)) // zero, needs tag transition
		x86ARM64EmitMovImm32(cb, 12, 2)
		cb.Emit32(arm64CMP_W(13, 12))
		branches = append(branches, cb.Len())
		cb.Emit32(arm64Bcond(arm64CondEQ, 0)) // special
		return branches
	}
	branches := checkTag(11)
	if ji.modrm&7 != 0 {
		branches = append(branches, checkTag(14)...)
	}
	// Slot byte offsets: physical index * sizeof(float64).
	cb.Emit32(arm64LSL_W_imm(11, 11, 3))
	cb.Emit32(arm64LSL_W_imm(14, 14, 3))
	cb.Emit32(arm64LDR_reg(12, x86ARM64Scratch, 11))
	cb.Emit32(arm64FMOV_XtoD(0, 12))
	cb.Emit32(arm64LDR_reg(12, x86ARM64Scratch, 14))
	cb.Emit32(arm64FMOV_XtoD(1, 12))
	switch op {
	case 0:
		cb.Emit32(arm64FADD_D(0, 0, 1))
	case 1:
		cb.Emit32(arm64FMUL_D(0, 0, 1))
	case 4:
		cb.Emit32(arm64FSUB_D(0, 0, 1))
	case 5:
		cb.Emit32(arm64FSUB_D(0, 1, 0))
	case 6:
		cb.Emit32(arm64FDIV_D(0, 0, 1))
	case 7:
		cb.Emit32(arm64FDIV_D(0, 1, 0))
	}
	cb.Emit32(arm64FMOV_DtoX(12, 0))
	// The direct path retains the current normal/zero input tags. A zero or
	// subnormal result needs the interpreter to reclassify the destination tag;
	// an all-one exponent likewise owns exception state. Exit before publishing
	// any of those results.
	cb.Emit32(arm64MOVZ(13, 0x7FF0, 48))
	cb.Emit32(arm64AND(13, 12, 13))
	cb.Emit32(arm64CMP(13, 31))
	branches = append(branches, cb.Len())
	cb.Emit32(arm64Bcond(arm64CondEQ, 0))
	cb.Emit32(arm64MOVZ(15, 0x7FF0, 48))
	cb.Emit32(arm64CMP(13, 15))
	branches = append(branches, cb.Len())
	cb.Emit32(arm64Bcond(arm64CondEQ, 0))
	cb.Emit32(arm64STR_reg(12, x86ARM64Scratch, 11))
	done := cb.Len()
	cb.Emit32(arm64B(0))
	cold := cb.Len()
	if !x86ARM64EmitFPUHelperExit(cb, payload, retired) {
		return false
	}
	end := cb.Len()
	for _, branch := range branches {
		cb.PatchUint32(branch, arm64Bcond(arm64CondEQ, int32(cold-branch)))
	}
	cb.PatchUint32(done, arm64B(int32(end-done)))
	return true
}

func x86ARM64EmitFNCLEX(cb *CodeBuffer, ji X86JITInstr, memory []byte) bool {
	if ji.opcode != 0xDB || ji.modrm != 0xE2 {
		return false
	}
	modrmPC := x86ARM64FindModRMPC(ji, memory)
	if modrmPC < 1 {
		return false
	}
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUPtr/8))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(modrmPC-1))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFIP/4))
	cb.Emit32(arm64LDR_imm(12, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
	cb.Emit32(arm64LDRH_imm(13, 12, uint32(x86SegCS)))
	cb.Emit32(arm64STRH_imm(13, x86ARM64Scratch, x86ARM64FPUOffFCS/2))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32((uint16(3)<<8)|uint16(ji.modrm)))
	cb.Emit32(arm64STRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFOP/2))
	cb.Emit32(arm64LDRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFSW/2))
	x86ARM64EmitMovImm32(cb, 12, ^uint32(0x80FF))
	cb.Emit32(arm64AND_W(x86ARM64Scratch2, x86ARM64Scratch2, 12))
	cb.Emit32(arm64STRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFSW/2))
	return true
}

// x86ARM64EmitFNINIT resets the complete project x87 state. The interpreter
// captures the operation before Reset, whose reset semantics deliberately
// clear every provenance field, so this direct form stores only reset values.
func x86ARM64EmitFNINIT(cb *CodeBuffer, ji X86JITInstr, memory []byte) bool {
	if ji.opcode != 0xDB || ji.modrm != 0xE3 || x86ARM64FindModRMPC(ji, memory) < 1 {
		return false
	}
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUPtr/8))
	// FPU_X87.regs is an eight-element float64 array at offset zero. XZR is
	// the architectural zero register for a 64-bit store on AArch64.
	for off := uint32(0); off < 8; off++ {
		cb.Emit32(arm64STR_imm(31, x86ARM64Scratch, off))
	}
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 0x037F)
	cb.Emit32(arm64STRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFCW/2))
	cb.Emit32(arm64STRH_imm(31, x86ARM64Scratch, x86ARM64FPUOffFSW/2))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 0xFFFF)
	cb.Emit32(arm64STRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFTW/2))
	cb.Emit32(arm64STR_W_imm(31, x86ARM64Scratch, x86ARM64FPUOffFIP/4))
	cb.Emit32(arm64STRH_imm(31, x86ARM64Scratch, x86ARM64FPUOffFCS/2))
	cb.Emit32(arm64STR_W_imm(31, x86ARM64Scratch, (x86ARM64FPUOffFIP+8)/4))
	cb.Emit32(arm64STRH_imm(31, x86ARM64Scratch, (x86ARM64FPUOffFCS+8)/2))
	cb.Emit32(arm64STRH_imm(31, x86ARM64Scratch, x86ARM64FPUOffFOP/2))
	return true
}

func x86ARM64EmitFNSTSWAX(cb *CodeBuffer, ji X86JITInstr, memory []byte) bool {
	if ji.opcode != 0xDF || ji.modrm != 0xE0 {
		return false
	}
	modrmPC := x86ARM64FindModRMPC(ji, memory)
	if modrmPC < 1 {
		return false
	}
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffFPUPtr/8))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(modrmPC-1))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFIP/4))
	cb.Emit32(arm64LDR_imm(12, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
	cb.Emit32(arm64LDRH_imm(13, 12, uint32(x86SegCS)))
	cb.Emit32(arm64STRH_imm(13, x86ARM64Scratch, x86ARM64FPUOffFCS/2))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32((uint16(7)<<8)|uint16(ji.modrm)))
	cb.Emit32(arm64STRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFOP/2))
	cb.Emit32(arm64LDRH_imm(x86ARM64Scratch2, x86ARM64Scratch, x86ARM64FPUOffFSW/2))
	x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 0)
	x86ARM64EmitWordInsert(cb, x86ARM64Scratch, x86ARM64Scratch2)
	x86ARM64EmitStoreReg(cb, 0, x86ARM64Scratch)
	return true
}

// x86ARM64EmitMOVS lowers MOVS and REP MOVS. Every completed REP element
// publishes ESI, EDI and ECX before the SMC guard, so a later guard exit
// restarts the same x86 instruction with exactly the interpreter's partial
// progress state.
func x86ARM64EmitMOVS(cb *CodeBuffer, ji X86JITInstr, retired int, bails *[]x86ARM64DeferredBail) bool {
	if ji.opcode != 0xA4 && ji.opcode != 0xA5 {
		return false
	}
	width := uint32(1)
	rep := ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0
	allowed := byte(0)
	if rep {
		allowed = x86PrefRep | x86PrefRepNE
	}
	if ji.opcode == 0xA5 {
		width = 4
		allowed |= x86PrefOpSize
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
	}
	if ji.prefixes&^allowed != 0 {
		return false
	}
	zero := -1
	if rep {
		x86ARM64EmitLoadReg(cb, 18, 1) // ECX
		zero = cb.Len()
		cb.Emit32(arm64CBZ(18, 0))
	}
	loop := cb.Len()
	x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 6) // ESI
	x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
	cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
	x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 7) // EDI
	x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
	cb.Emit32(arm64ORR_W(20, x86ARM64Scratch, 31))
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
	if width == 1 {
		cb.Emit32(arm64LDRB_reg(13, x86ARM64Scratch2, 19))
		cb.Emit32(arm64STRB_reg(13, x86ARM64Scratch2, 20))
	} else if width == 2 {
		cb.Emit32(arm64LDRH_reg(13, x86ARM64Scratch2, 19))
		cb.Emit32(arm64STRH_reg(13, x86ARM64Scratch2, 20))
	} else {
		cb.Emit32(arm64LDR_W_reg(13, x86ARM64Scratch2, 19))
		cb.Emit32(arm64STR_W_reg(13, x86ARM64Scratch2, 20))
	}
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(12, 14, 0))
	x86ARM64EmitMovImm32(cb, 13, x86FlagDF)
	cb.Emit32(arm64AND_W(12, 12, 13))
	forward := cb.Len()
	cb.Emit32(arm64CBZ(12, 0))
	cb.Emit32(arm64SUB_W_imm(19, 19, width))
	cb.Emit32(arm64SUB_W_imm(20, 20, width))
	done := cb.Len()
	cb.Emit32(arm64B(0))
	forwardPC := cb.Len()
	cb.Emit32(arm64ADD_W_imm(19, 19, width))
	cb.Emit32(arm64ADD_W_imm(20, 20, width))
	end := cb.Len()
	cb.PatchUint32(forward, arm64CBZ(12, int32(forwardPC-forward)))
	cb.PatchUint32(done, arm64B(int32(end-done)))
	if rep {
		cb.Emit32(arm64SUB_W_imm(18, 18, 1))
		x86ARM64EmitStoreReg(cb, 1, 18)
	}
	x86ARM64EmitStoreReg(cb, 6, 19)
	x86ARM64EmitStoreReg(cb, 7, 20)
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, 20, 31))
	x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
	if rep {
		back := cb.Len()
		cb.Emit32(arm64CBNZ(18, 0))
		cb.PatchUint32(back, arm64CBNZ(18, int32(loop-back)))
		end = cb.Len()
		cb.PatchUint32(zero, arm64CBZ(18, int32(end-zero)))
	}
	return true
}

// x86ARM64EmitSTOS lowers STOS and REP STOS with the same per-element state
// publication and exact invalidation rules as MOVS.
func x86ARM64EmitSTOS(cb *CodeBuffer, ji X86JITInstr, retired int, bails *[]x86ARM64DeferredBail) bool {
	if ji.opcode != 0xAA && ji.opcode != 0xAB {
		return false
	}
	width := uint32(1)
	rep := ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0
	allowed := byte(0)
	if rep {
		allowed = x86PrefRep | x86PrefRepNE
	}
	if ji.opcode == 0xAB {
		width = 4
		allowed |= x86PrefOpSize
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
	}
	if ji.prefixes&^allowed != 0 {
		return false
	}
	zero := -1
	if rep {
		x86ARM64EmitLoadReg(cb, 18, 1)
		zero = cb.Len()
		cb.Emit32(arm64CBZ(18, 0))
	}
	loop := cb.Len()
	x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 7)
	x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
	cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
	x86ARM64EmitLoadReg(cb, 13, 0)
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
	if width == 1 {
		cb.Emit32(arm64STRB_reg(13, x86ARM64Scratch2, 19))
	} else if width == 2 {
		cb.Emit32(arm64STRH_reg(13, x86ARM64Scratch2, 19))
	} else {
		cb.Emit32(arm64STR_W_reg(13, x86ARM64Scratch2, 19))
	}
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(12, 14, 0))
	x86ARM64EmitMovImm32(cb, 13, x86FlagDF)
	cb.Emit32(arm64AND_W(12, 12, 13))
	forward := cb.Len()
	cb.Emit32(arm64CBZ(12, 0))
	cb.Emit32(arm64SUB_W_imm(19, 19, width))
	done := cb.Len()
	cb.Emit32(arm64B(0))
	forwardPC := cb.Len()
	cb.Emit32(arm64ADD_W_imm(19, 19, width))
	end := cb.Len()
	cb.PatchUint32(forward, arm64CBZ(12, int32(forwardPC-forward)))
	cb.PatchUint32(done, arm64B(int32(end-done)))
	if rep {
		cb.Emit32(arm64SUB_W_imm(18, 18, 1))
		x86ARM64EmitStoreReg(cb, 1, 18)
	}
	x86ARM64EmitStoreReg(cb, 7, 19)
	cb.Emit32(arm64ORR_W(x86ARM64Scratch, 19, 31))
	x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
	if rep {
		back := cb.Len()
		cb.Emit32(arm64CBNZ(18, 0))
		cb.PatchUint32(back, arm64CBNZ(18, int32(loop-back)))
		end = cb.Len()
		cb.PatchUint32(zero, arm64CBZ(18, int32(end-zero)))
	}
	return true
}

// x86ARM64EmitLODS lowers LODS and REP LODS using the live saved DF.
func x86ARM64EmitLODS(cb *CodeBuffer, ji X86JITInstr, retired int, bails *[]x86ARM64DeferredBail) bool {
	if ji.opcode != 0xAC && ji.opcode != 0xAD {
		return false
	}
	width := uint32(1)
	rep := ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0
	allowed := byte(0)
	if rep {
		allowed = x86PrefRep | x86PrefRepNE
	}
	if ji.opcode == 0xAD {
		width = 4
		allowed |= x86PrefOpSize
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
	}
	if ji.prefixes&^allowed != 0 {
		return false
	}
	zero := -1
	if rep {
		x86ARM64EmitLoadReg(cb, 18, 1)
		zero = cb.Len()
		cb.Emit32(arm64CBZ(18, 0))
	}
	loop := cb.Len()
	x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 6)
	x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
	cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
	if width == 1 {
		cb.Emit32(arm64LDRB_reg(13, x86ARM64Scratch2, 19))
	} else if width == 2 {
		cb.Emit32(arm64LDRH_reg(13, x86ARM64Scratch2, 19))
	} else {
		cb.Emit32(arm64LDR_W_reg(13, x86ARM64Scratch2, 19))
	}
	x86ARM64EmitPartialRegStore(cb, 0, 13, width*8)
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(12, 14, 0))
	x86ARM64EmitMovImm32(cb, 13, x86FlagDF)
	cb.Emit32(arm64AND_W(12, 12, 13))
	forward := cb.Len()
	cb.Emit32(arm64CBZ(12, 0))
	cb.Emit32(arm64SUB_W_imm(19, 19, width))
	done := cb.Len()
	cb.Emit32(arm64B(0))
	forwardPC := cb.Len()
	cb.Emit32(arm64ADD_W_imm(19, 19, width))
	end := cb.Len()
	cb.PatchUint32(forward, arm64CBZ(12, int32(forwardPC-forward)))
	cb.PatchUint32(done, arm64B(int32(end-done)))
	if rep {
		cb.Emit32(arm64SUB_W_imm(18, 18, 1))
		x86ARM64EmitStoreReg(cb, 1, 18)
	}
	x86ARM64EmitStoreReg(cb, 6, 19)
	if rep {
		back := cb.Len()
		cb.Emit32(arm64CBNZ(18, 0))
		cb.PatchUint32(back, arm64CBNZ(18, int32(loop-back)))
		end = cb.Len()
		cb.PatchUint32(zero, arm64CBZ(18, int32(end-zero)))
	}
	return true
}

func x86ARM64EmitCMPS(cb *CodeBuffer, ji X86JITInstr, retired int, bails *[]x86ARM64DeferredBail) bool {
	if ji.opcode != 0xA6 && ji.opcode != 0xA7 {
		return false
	}
	width := uint32(1)
	rep := ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0
	repNE := ji.prefixes&x86PrefRepNE != 0
	allowed := byte(0)
	if rep {
		allowed = x86PrefRep | x86PrefRepNE
	}
	if ji.opcode == 0xA7 {
		width = 4
		allowed |= x86PrefOpSize
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
	}
	if ji.prefixes&^allowed != 0 {
		return false
	}
	zero := -1
	if rep {
		x86ARM64EmitLoadReg(cb, 18, 1)
		zero = cb.Len()
		cb.Emit32(arm64CBZ(18, 0))
	}
	loop := cb.Len()
	x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 6)
	x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
	cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
	x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 7)
	x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
	cb.Emit32(arm64ORR_W(20, x86ARM64Scratch, 31))
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
	if width == 1 {
		cb.Emit32(arm64LDRB_reg(16, x86ARM64Scratch2, 19))
		cb.Emit32(arm64LDRB_reg(17, x86ARM64Scratch2, 20))
	} else if width == 2 {
		cb.Emit32(arm64LDRH_reg(16, x86ARM64Scratch2, 19))
		cb.Emit32(arm64LDRH_reg(17, x86ARM64Scratch2, 20))
	} else {
		cb.Emit32(arm64LDR_W_reg(16, x86ARM64Scratch2, 19))
		cb.Emit32(arm64LDR_W_reg(17, x86ARM64Scratch2, 20))
	}
	cb.Emit32(arm64SUB_W(13, 16, 17))
	x86ARM64EmitTruncate(cb, 13, width*8)
	x86ARM64EmitArithFlags(cb, 13, 16, 17, width*8, true)
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(12, 14, 0))
	x86ARM64EmitMovImm32(cb, 13, x86FlagDF)
	cb.Emit32(arm64AND_W(12, 12, 13))
	forward := cb.Len()
	cb.Emit32(arm64CBZ(12, 0))
	cb.Emit32(arm64SUB_W_imm(19, 19, width))
	cb.Emit32(arm64SUB_W_imm(20, 20, width))
	done := cb.Len()
	cb.Emit32(arm64B(0))
	forwardPC := cb.Len()
	cb.Emit32(arm64ADD_W_imm(19, 19, width))
	cb.Emit32(arm64ADD_W_imm(20, 20, width))
	end := cb.Len()
	cb.PatchUint32(forward, arm64CBZ(12, int32(forwardPC-forward)))
	cb.PatchUint32(done, arm64B(int32(end-done)))
	if rep {
		cb.Emit32(arm64SUB_W_imm(18, 18, 1))
		x86ARM64EmitStoreReg(cb, 1, 18)
	}
	x86ARM64EmitStoreReg(cb, 6, 19)
	x86ARM64EmitStoreReg(cb, 7, 20)
	if rep {
		countDone := cb.Len()
		cb.Emit32(arm64CBZ(18, 0))
		cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(12, 14, 0))
		x86ARM64EmitMovImm32(cb, 13, x86FlagZF)
		cb.Emit32(arm64AND_W(12, 12, 13))
		back := cb.Len()
		if repNE {
			cb.Emit32(arm64CBZ(12, 0))
		} else {
			cb.Emit32(arm64CBNZ(12, 0))
		}
		end = cb.Len()
		if repNE {
			cb.PatchUint32(back, arm64CBZ(12, int32(loop-back)))
		} else {
			cb.PatchUint32(back, arm64CBNZ(12, int32(loop-back)))
		}
		cb.PatchUint32(countDone, arm64CBZ(18, int32(end-countDone)))
		cb.PatchUint32(zero, arm64CBZ(18, int32(end-zero)))
	}
	return true
}

func x86ARM64EmitSCAS(cb *CodeBuffer, ji X86JITInstr, retired int, bails *[]x86ARM64DeferredBail) bool {
	if ji.opcode != 0xAE && ji.opcode != 0xAF {
		return false
	}
	width := uint32(1)
	rep := ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0
	repNE := ji.prefixes&x86PrefRepNE != 0
	allowed := byte(0)
	if rep {
		allowed = x86PrefRep | x86PrefRepNE
	}
	if ji.opcode == 0xAF {
		width = 4
		allowed |= x86PrefOpSize
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
	}
	if ji.prefixes&^allowed != 0 {
		return false
	}
	zero := -1
	if rep {
		x86ARM64EmitLoadReg(cb, 18, 1)
		zero = cb.Len()
		cb.Emit32(arm64CBZ(18, 0))
	}
	loop := cb.Len()
	x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 7)
	x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
	cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
	x86ARM64EmitPartialRegLoad(cb, 16, 0, width*8)
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
	if width == 1 {
		cb.Emit32(arm64LDRB_reg(17, x86ARM64Scratch2, 19))
	} else if width == 2 {
		cb.Emit32(arm64LDRH_reg(17, x86ARM64Scratch2, 19))
	} else {
		cb.Emit32(arm64LDR_W_reg(17, x86ARM64Scratch2, 19))
	}
	cb.Emit32(arm64SUB_W(13, 16, 17))
	x86ARM64EmitTruncate(cb, 13, width*8)
	x86ARM64EmitArithFlags(cb, 13, 16, 17, width*8, true)
	cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(12, 14, 0))
	x86ARM64EmitMovImm32(cb, 13, x86FlagDF)
	cb.Emit32(arm64AND_W(12, 12, 13))
	forward := cb.Len()
	cb.Emit32(arm64CBZ(12, 0))
	cb.Emit32(arm64SUB_W_imm(19, 19, width))
	done := cb.Len()
	cb.Emit32(arm64B(0))
	forwardPC := cb.Len()
	cb.Emit32(arm64ADD_W_imm(19, 19, width))
	end := cb.Len()
	cb.PatchUint32(forward, arm64CBZ(12, int32(forwardPC-forward)))
	cb.PatchUint32(done, arm64B(int32(end-done)))
	if rep {
		cb.Emit32(arm64SUB_W_imm(18, 18, 1))
		x86ARM64EmitStoreReg(cb, 1, 18)
	}
	x86ARM64EmitStoreReg(cb, 7, 19)
	if rep {
		countDone := cb.Len()
		cb.Emit32(arm64CBZ(18, 0))
		cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(12, 14, 0))
		x86ARM64EmitMovImm32(cb, 13, x86FlagZF)
		cb.Emit32(arm64AND_W(12, 12, 13))
		back := cb.Len()
		if repNE {
			cb.Emit32(arm64CBZ(12, 0))
		} else {
			cb.Emit32(arm64CBNZ(12, 0))
		}
		end = cb.Len()
		if repNE {
			cb.PatchUint32(back, arm64CBZ(12, int32(loop-back)))
		} else {
			cb.PatchUint32(back, arm64CBNZ(12, int32(loop-back)))
		}
		cb.PatchUint32(countDone, arm64CBZ(18, int32(end-countDone)))
		cb.PatchUint32(zero, arm64CBZ(18, int32(end-zero)))
	}
	return true
}

// x86ARM64ChainExit is resolved after ExecMem.Write gives the code buffer its
// executable address.  ARM64 patches whole B instructions rather than an
// amd64 rel32 displacement, so the cold return address is retained explicitly
// for SMC invalidation to restore.
type x86ARM64ChainExit struct {
	targetPC    uint32
	branchOff   int
	fallbackOff int
}

// x86ARM64EmitChainOrReturn emits the shared terminal path for a statically
// known successor.  A chain always checks its bounded-execution budget and a
// completed native SMC store before entering the patched target.  Either check
// takes the cold path, publishes the accumulated retired count and returns to
// Go so interrupt, debugger and invalidation observation remain intact.
func x86ARM64EmitChainOrReturn(cb *CodeBuffer, targetPC uint32, retired int, cycles, ticks uint32, exits *[]x86ARM64ChainExit) {
	if !x86BlockChainingEnabled {
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch, targetPC)
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffRetPC/4))
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch, uint32(retired))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffRetCount/4))
		cb.Emit32(arm64RET())
		return
	}

	// ChainCount += retired; ChainBudget--.
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffChainCount/4))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(retired))
	cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffChainCount/4))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffChainCycles/4))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, cycles)
	cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffChainCycles/4))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffChainTicks/4))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, ticks)
	cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffChainTicks/4))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffChainBudget/4))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 1)
	cb.Emit32(arm64SUB_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffChainBudget/4))
	budgetBail := cb.Len()
	cb.Emit32(arm64CBZ(x86ARM64Scratch, 0))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffNeedInval/4))
	invalBail := cb.Len()
	cb.Emit32(arm64CBNZ(x86ARM64Scratch, 0))

	branchOff := cb.Len()
	cb.Emit32(arm64B(0)) // initially patched to cold return below
	fallbackOff := cb.Len()
	cb.PatchUint32(budgetBail, arm64CBZ(x86ARM64Scratch, int32(fallbackOff-budgetBail)))
	cb.PatchUint32(invalBail, arm64CBNZ(x86ARM64Scratch, int32(fallbackOff-invalBail)))
	cb.PatchUint32(branchOff, arm64B(int32(fallbackOff-branchOff)))

	x86ARM64EmitMovImm32(cb, x86ARM64Scratch, targetPC)
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffRetPC/4))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffChainCount/4))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffRetCount/4))
	cb.Emit32(arm64RET())
	*exits = append(*exits, x86ARM64ChainExit{targetPC: targetPC, branchOff: branchOff, fallbackOff: fallbackOff})
}

func x86ARM64InstallChainSlots(block *JITBlock, exits []x86ARM64ChainExit, chainable bool) {
	if block == nil || !x86BlockChainingEnabled || !chainable || len(exits) == 0 {
		return
	}
	// An ARM64 chained transfer keeps X0 and LR live, so the normal native
	// entry is also the lightweight chain entry.
	block.chainEntry = block.execAddr
	for _, exit := range exits {
		patchAddr := block.execAddr + uintptr(exit.branchOff)
		fallbackAddr := block.execAddr + uintptr(exit.fallbackOff)
		block.chainSlots = append(block.chainSlots, chainSlot{
			targetPC: uint64(exit.targetPC), patchAddr: patchAddr, fallbackAddr: fallbackAddr,
			patch: func(target uintptr) { x86ARM64PatchBranchAt(patchAddr, target) },
		})
	}
}

// x86ARM64EmitDirectJMP terminates the generated prefix at an unconditional
// relative branch. The block scanner's fall-through PC remains useful for
// cache ownership, but the native result must publish the architectural
// branch target before returning to the dispatcher or transfer through a
// generation-safe patched chain.
func x86ARM64EmitDirectJMP(cb *CodeBuffer, ji X86JITInstr, memory []byte, retired int, cycles, ticks uint32, exits *[]x86ARM64ChainExit) bool {
	if ji.prefixes&^x86PrefOpSize != 0 {
		return false
	}
	var disp int32
	switch byte(ji.opcode) {
	case 0xEB:
		if ji.length < 2 {
			return false
		}
		immPC := int(ji.opcodePC) + int(ji.length) - 1
		if immPC < 0 || immPC >= len(memory) {
			return false
		}
		disp = int32(int8(memory[immPC]))
	case 0xE9:
		width := 4
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if ji.length < uint16(width+1) {
			return false
		}
		immPC := int(ji.opcodePC) + int(ji.length) - width
		if immPC < 0 || immPC+width > len(memory) {
			return false
		}
		if width == 2 {
			disp = int32(int16(binary.LittleEndian.Uint16(memory[immPC:])))
		} else {
			disp = int32(binary.LittleEndian.Uint32(memory[immPC:]))
		}
	default:
		return false
	}
	target := uint32(int32(ji.opcodePC+uint32(ji.length)) + disp)
	x86ARM64EmitChainOrReturn(cb, target, retired+1, cycles, ticks, exits)
	return true
}

// x86ARM64EmitDirectCALL implements the near relative form as a terminal
// block. Its pushed continuation and resolved target both follow the
// interpreter's operand-size rules. A code-page stack write exits through the
// normal exact-range invalidation path before the target is dispatched.
func x86ARM64EmitDirectCALL(cb *CodeBuffer, ji X86JITInstr, memory []byte, retired int, cycles, ticks uint32, bails *[]x86ARM64DeferredBail, exits *[]x86ARM64ChainExit) bool {
	if ji.opcode != 0xE8 || ji.prefixes&^x86PrefOpSize != 0 {
		return false
	}
	width := 4
	if ji.prefixes&x86PrefOpSize != 0 {
		width = 2
	}
	if ji.length < uint16(width+1) {
		return false
	}
	immPC := int(ji.opcodePC) + int(ji.length) - width
	if immPC < 0 || immPC+width > len(memory) {
		return false
	}
	var disp int32
	if width == 2 {
		disp = int32(int16(binary.LittleEndian.Uint16(memory[immPC:])))
	} else {
		disp = int32(binary.LittleEndian.Uint32(memory[immPC:]))
	}
	returnPC := ji.opcodePC + uint32(ji.length)
	target := uint32(int32(returnPC) + disp)

	x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(width))
	cb.Emit32(arm64SUB_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	x86ARM64EmitSpanGuard(cb, uint32(width), ji.opcodePC, retired, bails)
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
	x86ARM64EmitMovImm32(cb, 12, returnPC)
	if width == 2 {
		cb.Emit32(arm64STRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
	} else {
		cb.Emit32(arm64STR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
	}
	x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
	x86ARM64EmitSMCStoreCheck(cb, uint32(width), target, retired+1, bails)
	x86ARM64EmitChainOrReturn(cb, target, retired+1, cycles, ticks, exits)
	return true
}

// x86ARM64EmitDirectRET implements near RET and RET imm16 for the native
// 32-bit operand-size path. Operand-size RET preserves EIP's high half in the
// interpreter and therefore remains at the canonical boundary for now.
func x86ARM64EmitDirectRET(cb *CodeBuffer, ji X86JITInstr, memory []byte, retired int, bails *[]x86ARM64DeferredBail) bool {
	if (ji.opcode != 0xC3 && ji.opcode != 0xC2) || ji.prefixes != 0 {
		return false
	}
	stackAdjust := uint32(4)
	if ji.opcode == 0xC2 {
		if ji.length < 3 {
			return false
		}
		immPC := int(ji.opcodePC) + int(ji.length) - 2
		if immPC < 0 || immPC+2 > len(memory) {
			return false
		}
		stackAdjust += uint32(binary.LittleEndian.Uint16(memory[immPC:]))
	}
	x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
	x86ARM64EmitSpanGuard(cb, 4, ji.opcodePC, retired, bails)
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
	cb.Emit32(arm64LDR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, stackAdjust)
	cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
	cb.Emit32(arm64STR_W_imm(12, x86ARM64RegCtx, x86CtxOffRetPC/4))
	x86ARM64EmitMovImm32(cb, x86ARM64Scratch, uint32(retired+1))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffChainCount/4))
	cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
	cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64RegCtx, x86CtxOffRetCount/4))
	cb.Emit32(arm64RET())
	return true
}

// x86ARM64EmitJccCondition materialises the x86 Jcc predicate in W10. Host
// NZCV is used only for the local CSET operations, never as guest flag state.
func x86ARM64EmitJccCondition(cb *CodeBuffer, condition byte) bool {
	if condition > 0xF {
		return false
	}
	cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
	cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, x86ARM64Scratch2, 0))
	bit := func(mask uint32, invert bool) {
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, mask)
		cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		cb.Emit32(arm64CMP_W(x86ARM64Scratch, 31))
		cb.Emit32(arm64CSET_W(x86ARM64Scratch, arm64CondNE))
		if invert {
			x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 1)
			cb.Emit32(arm64EOR_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		}
	}
	switch condition {
	case 0x0:
		bit(x86FlagOF, false)
	case 0x1:
		bit(x86FlagOF, true)
	case 0x2:
		bit(x86FlagCF, false)
	case 0x3:
		bit(x86FlagCF, true)
	case 0x4:
		bit(x86FlagZF, false)
	case 0x5:
		bit(x86FlagZF, true)
	case 0x6, 0x7:
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, x86FlagCF|x86FlagZF)
		cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		cb.Emit32(arm64CMP_W(x86ARM64Scratch, 31))
		cb.Emit32(arm64CSET_W(x86ARM64Scratch, arm64CondNE))
		if condition == 0x7 {
			x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 1)
			cb.Emit32(arm64EOR_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		}
	case 0x8:
		bit(x86FlagSF, false)
	case 0x9:
		bit(x86FlagSF, true)
	case 0xA:
		bit(x86FlagPF, false)
	case 0xB:
		bit(x86FlagPF, true)
	case 0xC, 0xD, 0xE, 0xF:
		// SF != OF. Retain the original flags in W11 so LE can OR in ZF.
		cb.Emit32(arm64ORR_W(x86ARM64Scratch2, x86ARM64Scratch, 31))
		cb.Emit32(arm64LSR_W_imm(x86ARM64Scratch, x86ARM64Scratch, 7))
		cb.Emit32(arm64LSR_W_imm(12, x86ARM64Scratch2, 11))
		cb.Emit32(arm64EOR_W(x86ARM64Scratch, x86ARM64Scratch, 12))
		x86ARM64EmitMovImm32(cb, 12, 1)
		cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, 12))
		if condition == 0xE || condition == 0xF {
			cb.Emit32(arm64LSR_W_imm(x86ARM64Scratch2, x86ARM64Scratch2, 6))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
			cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, 12))
		}
		if condition == 0xD || condition == 0xF {
			cb.Emit32(arm64EOR_W(x86ARM64Scratch, x86ARM64Scratch, 12))
		}
	}
	return true
}

// x86ARM64EmitDirectJcc is a terminal direct branch. Both local paths publish
// a complete block result, ensuring the dispatcher sees the selected x86 PC.
func x86ARM64EmitDirectJcc(cb *CodeBuffer, ji X86JITInstr, memory []byte, retired int, cycles, ticks uint32, exits *[]x86ARM64ChainExit) bool {
	if ji.prefixes&^x86PrefOpSize != 0 {
		return false
	}
	condition := byte(0)
	width := 0
	switch {
	case ji.opcode >= 0x70 && ji.opcode <= 0x7F:
		condition, width = byte(ji.opcode)&0xF, 1
	case ji.opcode >= 0x0F80 && ji.opcode <= 0x0F8F:
		condition = byte(ji.opcode) & 0xF
		width = 4
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
	default:
		return false
	}
	if ji.length < uint16(width+1) {
		return false
	}
	immPC := int(ji.opcodePC) + int(ji.length) - width
	if immPC < 0 || immPC+width > len(memory) {
		return false
	}
	var disp int32
	if width == 1 {
		disp = int32(int8(memory[immPC]))
	} else if width == 2 {
		disp = int32(int16(binary.LittleEndian.Uint16(memory[immPC:])))
	} else {
		disp = int32(binary.LittleEndian.Uint32(memory[immPC:]))
	}
	fallthroughPC := ji.opcodePC + uint32(ji.length)
	target := uint32(int32(fallthroughPC) + disp)
	if !x86ARM64EmitJccCondition(cb, condition) {
		return false
	}
	notTaken := cb.Len()
	cb.Emit32(arm64CBZ(x86ARM64Scratch, 0))
	x86ARM64EmitChainOrReturn(cb, target, retired+1, cycles, ticks, exits)
	notTakenPC := cb.Len()
	cb.PatchUint32(notTaken, arm64CBZ(x86ARM64Scratch, int32(notTakenPC-notTaken)))
	x86ARM64EmitChainOrReturn(cb, fallthroughPC, retired+1, cycles, ticks, exits)
	return true
}

// x86ARM64EmitDirectLoop covers LOOP, LOOPE, LOOPNE and JCXZ. Address-size
// LOOP forms update only CX, preserving the upper half of ECX exactly as the
// interpreter's SetCX path does.
func x86ARM64EmitDirectLoop(cb *CodeBuffer, ji X86JITInstr, memory []byte, retired int, cycles, ticks uint32, exits *[]x86ARM64ChainExit) bool {
	op := byte(ji.opcode)
	if op < 0xE0 || op > 0xE3 || ji.prefixes&^(x86PrefAddrSize|x86PrefOpSize) != 0 || ji.length < 2 {
		return false
	}
	immPC := int(ji.opcodePC) + int(ji.length) - 1
	if immPC < 0 || immPC >= len(memory) {
		return false
	}
	fallthroughPC := ji.opcodePC + uint32(ji.length)
	target := uint32(int32(fallthroughPC) + int32(int8(memory[immPC])))

	x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 1) // ECX
	if ji.prefixes&x86PrefAddrSize != 0 {
		// W12 holds the low CX value used for the test.
		cb.Emit32(0x53000000 | 15<<10 | uint32(x86ARM64Scratch)<<5 | 12) // UBFX W12,W10,#0,#16
		if op != 0xE3 {
			cb.Emit32(arm64SUB_W_imm(12, 12, 1))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch2, x86ARM64Scratch, 31))
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch2, 12)
			x86ARM64EmitStoreReg(cb, 1, x86ARM64Scratch2)
		}
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, 12, 31))
	} else if op != 0xE3 {
		cb.Emit32(arm64SUB_W_imm(x86ARM64Scratch, x86ARM64Scratch, 1))
		x86ARM64EmitStoreReg(cb, 1, x86ARM64Scratch)
	}
	cb.Emit32(arm64CMP_W(x86ARM64Scratch, 31))
	if op == 0xE3 {
		cb.Emit32(arm64CSET_W(x86ARM64Scratch, arm64CondEQ))
	} else {
		cb.Emit32(arm64CSET_W(x86ARM64Scratch, arm64CondNE))
		if op == 0xE0 || op == 0xE1 {
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch2, x86ARM64Scratch2, 0))
			x86ARM64EmitMovImm32(cb, 12, x86FlagZF)
			cb.Emit32(arm64AND_W(x86ARM64Scratch2, x86ARM64Scratch2, 12))
			cb.Emit32(arm64CMP_W(x86ARM64Scratch2, 31))
			cb.Emit32(arm64CSET_W(x86ARM64Scratch2, arm64CondNE))
			if op == 0xE0 { // LOOPNE
				x86ARM64EmitMovImm32(cb, 12, 1)
				cb.Emit32(arm64EOR_W(x86ARM64Scratch2, x86ARM64Scratch2, 12))
			}
			cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		}
	}
	notTaken := cb.Len()
	cb.Emit32(arm64CBZ(x86ARM64Scratch, 0))
	x86ARM64EmitChainOrReturn(cb, target, retired+1, cycles, ticks, exits)
	notTakenPC := cb.Len()
	cb.PatchUint32(notTaken, arm64CBZ(x86ARM64Scratch, int32(notTakenPC-notTaken)))
	x86ARM64EmitChainOrReturn(cb, fallthroughPC, retired+1, cycles, ticks, exits)
	return true
}

// x86ARM64EmitInstruction emits forms whose complete i386 semantics are
// represented by the supplied guarded-memory and flag paths. Returning false
// is an admission miss, never a partial lowering.
func x86ARM64EmitInstruction(cb *CodeBuffer, ji X86JITInstr, memory []byte, retired int, bails *[]x86ARM64DeferredBail) bool {
	// Operand-size has native MOV forms below. REP is additionally accepted by
	// the string lowerers, which consume the live ECX and saved DF state.
	allowedPrefixes := byte(x86PrefOpSize)
	switch byte(ji.opcode) {
	case 0xA4, 0xA5, 0xAA, 0xAB, 0xAC, 0xAD, 0xA6, 0xA7, 0xAE, 0xAF:
		allowedPrefixes |= x86PrefRep | x86PrefRepNE
	}
	if ji.prefixes&^allowedPrefixes != 0 {
		return false
	}
	// Keep the two-byte opcode map separate from the one-byte switch below.
	// Using byte(ji.opcode) there would alias, for example, MOVZX 0F B6 with
	// MOV r8, imm8 (B6), and make the valid register form an admission miss.
	if ji.opcode >= 0x0F00 {
		switch {
		case (ji.opcode == 0x0FA4 || ji.opcode == 0x0FAC) && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == x86PrefOpSize:
			immPC := int(ji.opcodePC) + int(ji.length) - 1
			if immPC < 0 || immPC >= len(memory) {
				return false
			}
			count := uint32(memory[immPC] & 0x1F)
			// The interpreter's uint32 shifts have defined results through a
			// count of 16. ARM's immediate shifts mask their count, so preserve
			// the remaining architectural cases at the interpreter boundary.
			if count > 16 {
				return false
			}
			if count == 0 {
				return true
			}
			dst, src := ji.modrm&7, (ji.modrm>>3)&7
			x86ARM64EmitPartialRegLoad(cb, 16, dst, 16)
			x86ARM64EmitPartialRegLoad(cb, 17, src, 16)
			if ji.opcode == 0x0FA4 { // SHLD
				cb.Emit32(arm64LSR_W_imm(12, 16, 16-count))
				cb.Emit32(arm64LSL_W_imm(13, 16, count))
				cb.Emit32(arm64LSR_W_imm(17, 17, 16-count))
			} else { // SHRD
				cb.Emit32(arm64LSR_W_imm(12, 16, count-1))
				cb.Emit32(arm64LSR_W_imm(13, 16, count))
				cb.Emit32(arm64LSL_W_imm(17, 17, 16-count))
			}
			cb.Emit32(arm64ORR_W(13, 13, 17))
			x86ARM64EmitTruncate(cb, 13, 16)
			x86ARM64EmitMovImm32(cb, 17, 1)
			cb.Emit32(arm64AND_W(12, 12, 17))
			cb.Emit32(arm64ORR_W(19, 12, 31))
			cb.Emit32(arm64LDR_imm(18, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(18, 18, 0))
			x86ARM64EmitMovImm32(cb, 17, x86FlagOF)
			cb.Emit32(arm64AND_W(18, 18, 17))
			x86ARM64EmitPartialRegStore(cb, dst, 13, 16)
			x86ARM64EmitLogicFlags(cb, 13, 16)
			cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 19))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 18))
			cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
			return true
		case (ji.opcode == 0x0FA4 || ji.opcode == 0x0FAC) && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == x86PrefOpSize:
			immPC := int(ji.opcodePC) + int(ji.length) - 1
			if immPC < 0 || immPC >= len(memory) {
				return false
			}
			count := uint32(memory[immPC] & 0x1F)
			if count > 16 {
				return false
			}
			if count == 0 {
				return true
			}
			if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
				return false
			}
			x86ARM64EmitSpanGuard(cb, 2, ji.opcodePC, retired, bails)
			cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			cb.Emit32(arm64LDRH_reg(16, x86ARM64Scratch2, 19))
			x86ARM64EmitPartialRegLoad(cb, 17, (ji.modrm>>3)&7, 16)
			if ji.opcode == 0x0FA4 { // SHLD
				cb.Emit32(arm64LSR_W_imm(12, 16, 16-count))
				cb.Emit32(arm64LSL_W_imm(13, 16, count))
				cb.Emit32(arm64LSR_W_imm(17, 17, 16-count))
			} else { // SHRD
				cb.Emit32(arm64LSR_W_imm(12, 16, count-1))
				cb.Emit32(arm64LSR_W_imm(13, 16, count))
				cb.Emit32(arm64LSL_W_imm(17, 17, 16-count))
			}
			cb.Emit32(arm64ORR_W(13, 13, 17))
			x86ARM64EmitTruncate(cb, 13, 16)
			x86ARM64EmitMovImm32(cb, 17, 1)
			cb.Emit32(arm64AND_W(12, 12, 17))
			cb.Emit32(arm64ORR_W(18, 12, 31))
			cb.Emit32(arm64LDR_imm(17, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(17, 17, 0))
			x86ARM64EmitMovImm32(cb, 12, x86FlagOF)
			cb.Emit32(arm64AND_W(17, 17, 12))
			x86ARM64EmitLogicFlags(cb, 13, 16)
			cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 18))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 17))
			cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			cb.Emit32(arm64STRH_reg(13, x86ARM64Scratch2, 19))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, 19, 31))
			x86ARM64EmitSMCStoreCheck(cb, 2, ji.opcodePC+uint32(ji.length), retired+1, bails)
			return true
		case ji.opcode == 0x0FAF && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == x86PrefOpSize:
			dst, src := (ji.modrm>>3)&7, ji.modrm&7
			x86ARM64EmitPartialRegLoad(cb, 16, dst, 16)
			x86ARM64EmitPartialRegLoad(cb, 17, src, 16)
			cb.Emit32(arm64SXTH(16, 16))
			cb.Emit32(arm64SXTH(17, 17))
			cb.Emit32(arm64SMULL(13, 16, 17))
			cb.Emit32(arm64SXTH(18, 13))
			cb.Emit32(arm64SXTW(18, 18))
			cb.Emit32(arm64CMP(13, 18))
			cb.Emit32(arm64CSET_W(17, arm64CondNE))
			x86ARM64EmitPartialRegStore(cb, dst, 13, 16)
			x86ARM64EmitMulOverflowFlags(cb, 17)
			return true
		case ji.opcode == 0x0FAF && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == x86PrefOpSize:
			if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
				return false
			}
			x86ARM64EmitSpanGuard(cb, 2, ji.opcodePC, retired, bails)
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			cb.Emit32(arm64LDRH_reg(17, x86ARM64Scratch2, x86ARM64Scratch))
			dst := (ji.modrm >> 3) & 7
			x86ARM64EmitPartialRegLoad(cb, 16, dst, 16)
			cb.Emit32(arm64SXTH(16, 16))
			cb.Emit32(arm64SXTH(17, 17))
			cb.Emit32(arm64SMULL(13, 16, 17))
			cb.Emit32(arm64SXTH(18, 13))
			cb.Emit32(arm64SXTW(18, 18))
			cb.Emit32(arm64CMP(13, 18))
			cb.Emit32(arm64CSET_W(17, arm64CondNE))
			x86ARM64EmitPartialRegStore(cb, dst, 13, 16)
			x86ARM64EmitMulOverflowFlags(cb, 17)
			return true
		case ji.opcode == 0x0FAF && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0:
			dst, src := (ji.modrm>>3)&7, ji.modrm&7
			x86ARM64EmitLoadReg(cb, 16, dst)
			x86ARM64EmitLoadReg(cb, 17, src)
			cb.Emit32(arm64SMULL(13, 16, 17))
			cb.Emit32(arm64SXTW(18, 13))
			cb.Emit32(arm64CMP(13, 18))
			cb.Emit32(arm64CSET_W(17, arm64CondNE))
			x86ARM64EmitStoreReg(cb, dst, 13)
			x86ARM64EmitMulOverflowFlags(cb, 17)
			return true
		case ji.opcode == 0x0FAF && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0:
			if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
				return false
			}
			x86ARM64EmitSpanGuard(cb, 4, ji.opcodePC, retired, bails)
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			cb.Emit32(arm64LDR_W_reg(17, x86ARM64Scratch2, x86ARM64Scratch))
			dst := (ji.modrm >> 3) & 7
			x86ARM64EmitLoadReg(cb, 16, dst)
			cb.Emit32(arm64SMULL(13, 16, 17))
			cb.Emit32(arm64SXTW(18, 13))
			cb.Emit32(arm64CMP(13, 18))
			cb.Emit32(arm64CSET_W(17, arm64CondNE))
			x86ARM64EmitStoreReg(cb, dst, 13)
			x86ARM64EmitMulOverflowFlags(cb, 17)
			return true
		case (ji.opcode == 0x0FA4 || ji.opcode == 0x0FAC) && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0:
			immPC := int(ji.opcodePC) + int(ji.length) - 1
			if immPC < 0 || immPC >= len(memory) {
				return false
			}
			count := uint32(memory[immPC] & 0x1F)
			if count == 0 {
				return true
			}
			dst, src := ji.modrm&7, (ji.modrm>>3)&7
			x86ARM64EmitLoadReg(cb, 16, dst)
			x86ARM64EmitLoadReg(cb, 17, src)
			if ji.opcode == 0x0FA4 {
				cb.Emit32(arm64LSR_W_imm(12, 16, 32-count))
				cb.Emit32(arm64LSL_W_imm(13, 16, count))
				cb.Emit32(arm64LSR_W_imm(17, 17, 32-count))
			} else {
				cb.Emit32(arm64LSR_W_imm(12, 16, count-1))
				cb.Emit32(arm64LSR_W_imm(13, 16, count))
				cb.Emit32(arm64LSL_W_imm(17, 17, 32-count))
			}
			cb.Emit32(arm64ORR_W(13, 13, 17))
			x86ARM64EmitMovImm32(cb, 17, 1)
			cb.Emit32(arm64AND_W(12, 12, 17))
			cb.Emit32(arm64ORR_W(19, 12, 31))
			cb.Emit32(arm64LDR_imm(18, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(18, 18, 0))
			x86ARM64EmitMovImm32(cb, 17, x86FlagOF)
			cb.Emit32(arm64AND_W(18, 18, 17))
			x86ARM64EmitStoreReg(cb, dst, 13)
			x86ARM64EmitLogicFlags(cb, 13, 32)
			cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 19))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 18))
			cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
			return true
		case (ji.opcode == 0x0FA5 || ji.opcode == 0x0FAD) && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0:
			// SHLD/SHRD r32,r32,CL. The interpreter masks CL to five bits;
			// a zero effective count leaves both the operand and flags intact.
			dst, src := ji.modrm&7, (ji.modrm>>3)&7
			x86ARM64EmitLoadReg(cb, 18, 1) // ECX
			x86ARM64EmitMovImm32(cb, 12, 31)
			cb.Emit32(arm64AND_W(18, 18, 12))
			zero := cb.Len()
			cb.Emit32(arm64CBZ(18, 0))
			x86ARM64EmitLoadReg(cb, 16, dst)
			x86ARM64EmitLoadReg(cb, 17, src)
			cb.Emit32(arm64LDR_imm(20, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(20, 20, 0))
			x86ARM64EmitMovImm32(cb, 12, x86FlagOF)
			cb.Emit32(arm64AND_W(20, 20, 12))
			x86ARM64EmitMovImm32(cb, 13, 32)
			cb.Emit32(arm64SUB_W(13, 13, 18))
			if ji.opcode == 0x0FA5 { // SHLD
				cb.Emit32(arm64LSR_W(12, 16, 13))
				cb.Emit32(arm64LSLV_W(16, 16, 18))
				cb.Emit32(arm64LSR_W(17, 17, 13))
			} else { // SHRD
				x86ARM64EmitMovImm32(cb, 12, 1)
				cb.Emit32(arm64SUB_W(12, 18, 12))
				cb.Emit32(arm64LSR_W(12, 16, 12))
				cb.Emit32(arm64LSR_W(16, 16, 18))
				cb.Emit32(arm64LSLV_W(17, 17, 13))
			}
			cb.Emit32(arm64ORR_W(16, 16, 17))
			x86ARM64EmitMovImm32(cb, 17, 1)
			cb.Emit32(arm64AND_W(12, 12, 17))
			cb.Emit32(arm64ORR_W(19, 12, 31))
			x86ARM64EmitStoreReg(cb, dst, 16)
			x86ARM64EmitLogicFlags(cb, 16, 32)
			cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 19))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 20))
			cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
			end := cb.Len()
			cb.PatchUint32(zero, arm64CBZ(18, int32(end-zero)))
			return true
		case (ji.opcode == 0x0FA4 || ji.opcode == 0x0FAC) && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0:
			immPC := int(ji.opcodePC) + int(ji.length) - 1
			if immPC < 0 || immPC >= len(memory) {
				return false
			}
			count := uint32(memory[immPC] & 0x1F)
			if count == 0 {
				return true
			}
			if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
				return false
			}
			x86ARM64EmitSpanGuard(cb, 4, ji.opcodePC, retired, bails)
			// Retain the resolved destination across flag publication so the
			// native store and its exact SMC check use the same guest address.
			cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			cb.Emit32(arm64LDR_W_reg(16, x86ARM64Scratch2, 19))
			x86ARM64EmitLoadReg(cb, 17, (ji.modrm>>3)&7)
			if ji.opcode == 0x0FA4 { // SHLD
				cb.Emit32(arm64LSR_W_imm(12, 16, 32-count))
				cb.Emit32(arm64LSL_W_imm(13, 16, count))
				cb.Emit32(arm64LSR_W_imm(17, 17, 32-count))
			} else { // SHRD
				cb.Emit32(arm64LSR_W_imm(12, 16, count-1))
				cb.Emit32(arm64LSR_W_imm(13, 16, count))
				cb.Emit32(arm64LSL_W_imm(17, 17, 32-count))
			}
			cb.Emit32(arm64ORR_W(13, 13, 17))
			x86ARM64EmitMovImm32(cb, 17, 1)
			cb.Emit32(arm64AND_W(12, 12, 17))
			cb.Emit32(arm64ORR_W(18, 12, 31))
			cb.Emit32(arm64LDR_imm(17, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(17, 17, 0))
			x86ARM64EmitMovImm32(cb, 12, x86FlagOF)
			cb.Emit32(arm64AND_W(17, 17, 12))
			x86ARM64EmitLogicFlags(cb, 13, 32)
			cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 18))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 17))
			cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			cb.Emit32(arm64STR_W_reg(13, x86ARM64Scratch2, 19))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, 19, 31))
			x86ARM64EmitSMCStoreCheck(cb, 4, ji.opcodePC+uint32(ji.length), retired+1, bails)
			return true
		case (ji.opcode == 0x0FA3 || ji.opcode == 0x0FAB || ji.opcode == 0x0FB3 || ji.opcode == 0x0FBB || (ji.opcode == 0x0FBA && ji.grpOp == 4)) && ji.hasModRM && ji.modrm>>6 == 3: // BT/BTS/BTR/BTC r/m16/32,r16/32; BT r/m,imm8
			if ji.prefixes&^x86PrefOpSize != 0 {
				return false
			}
			rm, reg := ji.modrm&7, (ji.modrm>>3)&7
			x86ARM64EmitLoadReg(cb, 13, rm)
			if ji.opcode == 0x0FBA {
				immPC := int(ji.opcodePC) + int(ji.length) - 1
				if immPC < 0 || immPC >= len(memory) {
					return false
				}
				x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(memory[immPC]))
			} else {
				x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, reg)
			}
			mask := uint32(31)
			if ji.prefixes&x86PrefOpSize != 0 {
				mask = 15
				cb.Emit32(arm64ORR_W(14, 13, 31))
				x86ARM64EmitMovImm32(cb, 12, 0xFFFF)
				cb.Emit32(arm64AND_W(13, 13, 12))
			}
			x86ARM64EmitMovImm32(cb, 12, mask)
			cb.Emit32(arm64AND_W(x86ARM64Scratch2, x86ARM64Scratch2, 12))
			cb.Emit32(arm64LSR_W(x86ARM64Scratch, 13, x86ARM64Scratch2))
			x86ARM64EmitMovImm32(cb, 12, 1)
			cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, 12)) // CF bit value
			cb.Emit32(arm64ORR_W(16, x86ARM64Scratch2, 31))             // retain bit index across CF publication
			// BT defines only CF. Clear the old bit then merge the extracted
			// 0/1 value, preserving every other architected EFLAGS bit.
			cb.Emit32(arm64LDR_imm(15, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
			cb.Emit32(arm64LDR_W_imm(12, 15, 0))
			x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, ^uint32(x86FlagCF))
			cb.Emit32(arm64AND_W(12, 12, x86ARM64Scratch2))
			cb.Emit32(arm64ORR_W(12, 12, x86ARM64Scratch))
			cb.Emit32(arm64STR_W_imm(12, 15, 0))
			if ji.opcode != 0x0FA3 && ji.opcode != 0x0FBA {
				x86ARM64EmitMovImm32(cb, 12, 1)
				cb.Emit32(arm64LSLV_W(12, 12, 16))
				switch ji.opcode {
				case 0x0FAB: // BTS
					cb.Emit32(arm64ORR_W(13, 13, 12))
				case 0x0FB3: // BTR
					// MVN W12,W12 then AND clears the selected bit.
					cb.Emit32(0x2A2003E0 | uint32(12)<<16 | 12)
					cb.Emit32(arm64AND_W(13, 13, 12))
				case 0x0FBB: // BTC
					cb.Emit32(arm64EOR_W(13, 13, 12))
				}
				if ji.prefixes&x86PrefOpSize != 0 {
					x86ARM64EmitWordInsert(cb, 14, 13)
					x86ARM64EmitStoreReg(cb, rm, 14)
				} else {
					x86ARM64EmitStoreReg(cb, rm, 13)
				}
			}
			return true
		case (ji.opcode == 0x0FBC || ji.opcode == 0x0FBD) && ji.hasModRM: // BSF/BSR r32,r/m32
			if ji.prefixes&^x86PrefOpSize != 0 {
				return false
			}
			width := uint32(4)
			if ji.prefixes&x86PrefOpSize != 0 {
				width = 2
			}
			if ji.modrm>>6 == 3 {
				x86ARM64EmitLoadReg(cb, 13, ji.modrm&7)
				if width == 2 {
					cb.Emit32(0x53000000 | 15<<10 | 13<<5 | 13) // UBFX W13,W13,#0,#16
				}
			} else {
				if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
					return false
				}
				x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
				cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
				if width == 2 {
					cb.Emit32(arm64LDRH_reg(13, x86ARM64Scratch2, x86ARM64Scratch))
				} else {
					cb.Emit32(arm64LDR_W_reg(13, x86ARM64Scratch2, x86ARM64Scratch))
				}
			}
			cb.Emit32(arm64CMP_W(13, 31))
			zero := cb.Len()
			cb.Emit32(arm64Bcond(arm64CondEQ, 0))
			x86ARM64EmitFlagBit(cb, x86FlagZF, false, false)
			if ji.opcode == 0x0FBC {
				cb.Emit32(arm64RBIT_W(x86ARM64Scratch, 13))
				cb.Emit32(arm64CLZ_W(x86ARM64Scratch, x86ARM64Scratch))
			} else {
				cb.Emit32(arm64CLZ_W(x86ARM64Scratch, 13))
				x86ARM64EmitMovImm32(cb, 12, 31)
				cb.Emit32(arm64EOR_W(x86ARM64Scratch, x86ARM64Scratch, 12))
			}
			dst := (ji.modrm >> 3) & 7
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, dst)
			if width == 2 {
				x86ARM64EmitWordInsert(cb, x86ARM64Scratch2, x86ARM64Scratch)
			} else {
				cb.Emit32(arm64ORR_W(x86ARM64Scratch2, x86ARM64Scratch, 31))
			}
			x86ARM64EmitStoreReg(cb, dst, x86ARM64Scratch2)
			done := cb.Len()
			cb.Emit32(arm64B(0))
			zeroPC := cb.Len()
			cb.PatchUint32(zero, arm64Bcond(arm64CondEQ, int32(zeroPC-zero)))
			x86ARM64EmitFlagBit(cb, x86FlagZF, true, false)
			end := cb.Len()
			cb.PatchUint32(done, arm64B(int32(end-done)))
			return true
		case ji.opcode >= 0x0F40 && ji.opcode <= 0x0F4F && ji.hasModRM: // CMOVcc r32,r/m32
			if ji.prefixes&^x86PrefOpSize != 0 {
				return false
			}
			width := uint32(4)
			if ji.prefixes&x86PrefOpSize != 0 {
				width = 2
			}
			if ji.modrm>>6 == 3 {
				x86ARM64EmitLoadReg(cb, 13, ji.modrm&7)
			} else {
				if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
					return false
				}
				x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
				cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
				if width == 2 {
					cb.Emit32(arm64LDRH_reg(13, x86ARM64Scratch2, x86ARM64Scratch))
				} else {
					cb.Emit32(arm64LDR_W_reg(13, x86ARM64Scratch2, x86ARM64Scratch))
				}
			}
			if !x86ARM64EmitJccCondition(cb, byte(ji.opcode)&0xF) {
				return false
			}
			skip := cb.Len()
			cb.Emit32(arm64CBZ(x86ARM64Scratch, 0))
			dst := (ji.modrm >> 3) & 7
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, dst)
			if width == 2 {
				x86ARM64EmitWordInsert(cb, x86ARM64Scratch2, 13)
			} else {
				cb.Emit32(arm64ORR_W(x86ARM64Scratch2, 13, 31))
			}
			x86ARM64EmitStoreReg(cb, dst, x86ARM64Scratch2)
			end := cb.Len()
			cb.PatchUint32(skip, arm64CBZ(x86ARM64Scratch, int32(end-skip)))
			return true
		case ji.opcode >= 0x0F90 && ji.opcode <= 0x0F9F && ji.hasModRM: // SETcc r/m8
			if ji.modrm>>6 == 3 {
				if !x86ARM64EmitJccCondition(cb, byte(ji.opcode)&0xF) {
					return false
				}
				rm := ji.modrm & 7
				x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, rm&3)
				x86ARM64EmitByteInsert(cb, x86ARM64Scratch2, x86ARM64Scratch, rm >= 4)
				x86ARM64EmitStoreReg(cb, rm&3, x86ARM64Scratch2)
				return true
			}
			if ji.prefixes != 0 || !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
				return false
			}
			x86ARM64EmitSpanGuard(cb, 1, ji.opcodePC, retired, bails)
			// Preserve the guarded EA across predicate evaluation, whose scratch
			// contract intentionally owns W10, W11 and W12.
			cb.Emit32(arm64ORR_W(13, x86ARM64Scratch, 31))
			if !x86ARM64EmitJccCondition(cb, byte(ji.opcode)&0xF) {
				return false
			}
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			cb.Emit32(arm64STRB_reg(x86ARM64Scratch, x86ARM64Scratch2, 13))
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, 13, 31))
			x86ARM64EmitSMCStoreCheck(cb, 1, ji.opcodePC+uint32(ji.length), retired+1, bails)
			return true
		case ji.opcode == 0x0FA0 || ji.opcode == 0x0FA8: // PUSH FS/GS
			seg := byte(x86SegFS)
			if ji.opcode == 0x0FA8 {
				seg = x86SegGS
			}
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
			x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 2)
			cb.Emit32(arm64SUB_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
			x86ARM64EmitSpanGuard(cb, 2, ji.opcodePC, retired, bails)
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
			cb.Emit32(arm64LDRH_imm(12, x86ARM64Scratch2, uint32(seg)))
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			cb.Emit32(arm64STRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
			x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
			x86ARM64EmitSMCStoreCheck(cb, 2, ji.opcodePC+uint32(ji.length), retired+1, bails)
			return true
		case ji.opcode == 0x0FA1 || ji.opcode == 0x0FA9: // POP FS/GS
			seg := byte(x86SegFS)
			if ji.opcode == 0x0FA9 {
				seg = x86SegGS
			}
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
			x86ARM64EmitSpanGuard(cb, 2, ji.opcodePC, retired, bails)
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			cb.Emit32(arm64LDRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
			x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 2)
			cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
			x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
			cb.Emit32(arm64STRH_imm(12, x86ARM64Scratch2, uint32(seg)))
			return true
		case ji.opcode >= 0x0FC8 && ji.opcode <= 0x0FCF: // BSWAP r32
			reg := byte(ji.opcode) & 7
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, reg)
			cb.Emit32(arm64REV_W(x86ARM64Scratch, x86ARM64Scratch))
			x86ARM64EmitStoreReg(cb, reg, x86ARM64Scratch)
			return true
		case (ji.opcode == 0x0FB6 || ji.opcode == 0x0FB7) && ji.hasModRM && ji.modrm>>6 != 3: // MOVZX r32,m8/m16
			if ji.prefixes != 0 || !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
				return false
			}
			width := uint32(1)
			if ji.opcode == 0x0FB7 {
				width = 2
			}
			x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			if width == 1 {
				cb.Emit32(arm64LDRB_reg(x86ARM64Scratch, x86ARM64Scratch2, x86ARM64Scratch))
			} else {
				cb.Emit32(arm64LDRH_reg(x86ARM64Scratch, x86ARM64Scratch2, x86ARM64Scratch))
			}
			x86ARM64EmitStoreReg(cb, (ji.modrm>>3)&7, x86ARM64Scratch)
			return true
		case (ji.opcode == 0x0FBE || ji.opcode == 0x0FBF) && ji.hasModRM && ji.modrm>>6 != 3: // MOVSX r32,m8/m16
			if ji.prefixes != 0 || !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
				return false
			}
			width := uint32(1)
			if ji.opcode == 0x0FBF {
				width = 2
			}
			x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			if width == 1 {
				cb.Emit32(arm64LDRB_reg(x86ARM64Scratch, x86ARM64Scratch2, x86ARM64Scratch))
				cb.Emit32(arm64SXTB(x86ARM64Scratch, x86ARM64Scratch))
			} else {
				cb.Emit32(arm64LDRH_reg(x86ARM64Scratch, x86ARM64Scratch2, x86ARM64Scratch))
				cb.Emit32(arm64SXTH(x86ARM64Scratch, x86ARM64Scratch))
			}
			x86ARM64EmitStoreReg(cb, (ji.modrm>>3)&7, x86ARM64Scratch)
			return true
		case (ji.opcode == 0x0FB6 || ji.opcode == 0x0FB7) && ji.hasModRM && ji.modrm>>6 == 3: // MOVZX r32,r8/r16
			reg, rm := (ji.modrm>>3)&7, ji.modrm&7
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, rm&3)
			if ji.opcode == 0x0FB6 {
				x86ARM64EmitByteExtract(cb, x86ARM64Scratch, x86ARM64Scratch, rm >= 4)
			} else {
				// UBFX Wd, Wn, #0, #16.
				cb.Emit32(0x53000000 | 15<<10 | uint32(x86ARM64Scratch)<<5 | uint32(x86ARM64Scratch))
			}
			x86ARM64EmitStoreReg(cb, reg, x86ARM64Scratch)
			return true
		case (ji.opcode == 0x0FBE || ji.opcode == 0x0FBF) && ji.hasModRM && ji.modrm>>6 == 3: // MOVSX r32,r8/r16
			reg, rm := (ji.modrm>>3)&7, ji.modrm&7
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, rm&3)
			if ji.opcode == 0x0FBE {
				x86ARM64EmitByteExtract(cb, x86ARM64Scratch, x86ARM64Scratch, rm >= 4)
				cb.Emit32(arm64SXTB(x86ARM64Scratch, x86ARM64Scratch))
			} else {
				cb.Emit32(arm64SXTH(x86ARM64Scratch, x86ARM64Scratch))
			}
			x86ARM64EmitStoreReg(cb, reg, x86ARM64Scratch)
			return true
		}
		return false
	}
	op := byte(ji.opcode)
	switch {
	case (op == 0xA4 || op == 0xA5) && x86ARM64EmitMOVS(cb, ji, retired, bails):
		return true
	case (op == 0xAA || op == 0xAB) && x86ARM64EmitSTOS(cb, ji, retired, bails):
		return true
	case (op == 0xAC || op == 0xAD) && x86ARM64EmitLODS(cb, ji, retired, bails):
		return true
	case (op == 0xA6 || op == 0xA7) && x86ARM64EmitCMPS(cb, ji, retired, bails):
		return true
	case (op == 0xAE || op == 0xAF) && x86ARM64EmitSCAS(cb, ji, retired, bails):
		return true
	case (ji.opcode == 0xD2 || ji.opcode == 0xD3) && x86ARM64EmitGrp2CLRotateNarrowReg(cb, ji):
		return true
	case (ji.opcode == 0xD2 || ji.opcode == 0xD3) && x86ARM64EmitGrp2CLCarryRotateReg(cb, ji):
		return true
	case ji.opcode == 0xD3 && ji.grpOp >= 4 && x86ARM64EmitGrp2CL16ShiftReg(cb, ji):
		return true
	case ji.opcode == 0xD3 && ji.grpOp >= 4 && x86ARM64EmitGrp2CL32Reg(cb, ji):
		return true
	case ji.opcode == 0xD2 && ji.grpOp >= 4 && x86ARM64EmitGrp2CL8ShiftReg(cb, ji):
		return true
	case (op == 0x69 || op == 0x6B) && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == x86PrefOpSize:
		immSize := 2
		if op == 0x6B {
			immSize = 1
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) {
			return false
		}
		imm := int32(int16(binary.LittleEndian.Uint16(memory[immPC:])))
		if immSize == 1 {
			imm = int32(int8(memory[immPC]))
		}
		dst, src := (ji.modrm>>3)&7, ji.modrm&7
		x86ARM64EmitPartialRegLoad(cb, 16, src, 16)
		cb.Emit32(arm64SXTH(16, 16))
		x86ARM64EmitMovImm32(cb, 17, uint32(imm))
		cb.Emit32(arm64SMULL(13, 16, 17))
		cb.Emit32(arm64SXTH(18, 13))
		cb.Emit32(arm64SXTW(18, 18))
		cb.Emit32(arm64CMP(13, 18))
		cb.Emit32(arm64CSET_W(17, arm64CondNE))
		x86ARM64EmitPartialRegStore(cb, dst, 13, 16)
		x86ARM64EmitMulOverflowFlags(cb, 17)
		return true
	case (op == 0x69 || op == 0x6B) && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == x86PrefOpSize:
		immSize := 2
		if op == 0x6B {
			immSize = 1
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) || !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		imm := int32(int16(binary.LittleEndian.Uint16(memory[immPC:])))
		if immSize == 1 {
			imm = int32(int8(memory[immPC]))
		}
		x86ARM64EmitSpanGuard(cb, 2, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		cb.Emit32(arm64LDRH_reg(16, x86ARM64Scratch2, x86ARM64Scratch))
		cb.Emit32(arm64SXTH(16, 16))
		x86ARM64EmitMovImm32(cb, 17, uint32(imm))
		cb.Emit32(arm64SMULL(13, 16, 17))
		cb.Emit32(arm64SXTH(18, 13))
		cb.Emit32(arm64SXTW(18, 18))
		cb.Emit32(arm64CMP(13, 18))
		cb.Emit32(arm64CSET_W(17, arm64CondNE))
		x86ARM64EmitPartialRegStore(cb, (ji.modrm>>3)&7, 13, 16)
		x86ARM64EmitMulOverflowFlags(cb, 17)
		return true
	case (op == 0x69 || op == 0x6B) && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0:
		immSize := 4
		if op == 0x6B {
			immSize = 1
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) {
			return false
		}
		imm := uint32(binary.LittleEndian.Uint32(memory[immPC:]))
		if immSize == 1 {
			imm = uint32(int32(int8(memory[immPC])))
		}
		dst, src := (ji.modrm>>3)&7, ji.modrm&7
		x86ARM64EmitLoadReg(cb, 16, src)
		x86ARM64EmitMovImm32(cb, 17, imm)
		cb.Emit32(arm64SMULL(13, 16, 17))
		cb.Emit32(arm64SXTW(18, 13))
		cb.Emit32(arm64CMP(13, 18))
		cb.Emit32(arm64CSET_W(17, arm64CondNE))
		x86ARM64EmitStoreReg(cb, dst, 13)
		x86ARM64EmitMulOverflowFlags(cb, 17)
		return true
	case (op == 0x69 || op == 0x6B) && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0:
		immSize := 4
		if op == 0x6B {
			immSize = 1
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) || !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		imm := uint32(binary.LittleEndian.Uint32(memory[immPC:]))
		if immSize == 1 {
			imm = uint32(int32(int8(memory[immPC])))
		}
		x86ARM64EmitSpanGuard(cb, 4, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		cb.Emit32(arm64LDR_W_reg(16, x86ARM64Scratch2, x86ARM64Scratch))
		x86ARM64EmitMovImm32(cb, 17, imm)
		cb.Emit32(arm64SMULL(13, 16, 17))
		cb.Emit32(arm64SXTW(18, 13))
		cb.Emit32(arm64CMP(13, 18))
		cb.Emit32(arm64CSET_W(17, arm64CondNE))
		x86ARM64EmitStoreReg(cb, (ji.modrm>>3)&7, 13)
		x86ARM64EmitMulOverflowFlags(cb, 17)
		return true
	case op == 0xC1 && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0 && ji.length != 0 && memory[int(ji.opcodePC)+int(ji.length)-1]&0x1F == 0: // Group 2 count masks to zero
		return true
	case op == 0xC1 && ji.hasModRM && ji.modrm>>6 == 3 && (ji.grpOp == 0 || ji.grpOp == 1) && ji.prefixes == 0 && ji.length != 0 && memory[int(ji.opcodePC)+int(ji.length)-1]&0x1F > 1: // ROL/ROR r32,imm8
		count := uint32(memory[int(ji.opcodePC)+int(ji.length)-1] & 0x1F)
		rm := ji.modrm & 7
		x86ARM64EmitPartialRegLoad(cb, 16, rm, 32)
		if ji.grpOp == 0 { // ROL x,c is ROR x,32-c.
			x86ARM64EmitMovImm32(cb, 17, 32-count)
			cb.Emit32(arm64ROR_W(13, 16, 17))
			x86ARM64EmitMovImm32(cb, 17, 1)
			cb.Emit32(arm64AND_W(12, 13, 17))
		} else {
			x86ARM64EmitMovImm32(cb, 17, count)
			cb.Emit32(arm64ROR_W(13, 16, 17))
			cb.Emit32(arm64LSR_W_imm(12, 13, 31))
		}
		x86ARM64EmitPartialRegStore(cb, rm, 13, 32)
		cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, ^uint32(x86FlagCF))
		cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 12))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
		return true
	case op == 0xC1 && ji.hasModRM && ji.modrm>>6 == 3 && (ji.grpOp == 4 || ji.grpOp == 5 || ji.grpOp == 6 || ji.grpOp == 7) && ji.prefixes == 0 && ji.length != 0 && memory[int(ji.opcodePC)+int(ji.length)-1]&0x1F > 1: // SHL/SAL/SHR/SAR r32,imm8
		count := uint32(memory[int(ji.opcodePC)+int(ji.length)-1] & 0x1F)
		rm := ji.modrm & 7
		x86ARM64EmitPartialRegLoad(cb, 16, rm, 32)
		// The interpreter leaves OF unchanged for counts other than one.
		cb.Emit32(arm64LDR_imm(18, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(18, 18, 0))
		x86ARM64EmitMovImm32(cb, 17, x86FlagOF)
		cb.Emit32(arm64AND_W(18, 18, 17))
		if ji.grpOp == 4 || ji.grpOp == 6 {
			cb.Emit32(arm64LSR_W_imm(12, 16, 32-count))
			cb.Emit32(arm64LSL_W_imm(13, 16, count))
		} else if ji.grpOp == 5 {
			cb.Emit32(arm64LSR_W_imm(12, 16, count-1))
			cb.Emit32(arm64LSR_W_imm(13, 16, count))
		} else {
			cb.Emit32(arm64LSR_W_imm(12, 16, count-1))
			cb.Emit32(arm64ASR_W_imm(13, 16, count))
		}
		x86ARM64EmitMovImm32(cb, 17, 1)
		cb.Emit32(arm64AND_W(12, 12, 17))
		x86ARM64EmitPartialRegStore(cb, rm, 13, 32)
		cb.Emit32(arm64ORR_W(17, 12, 31))
		x86ARM64EmitLogicFlags(cb, 13, 32)
		cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 17))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 18))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
		return true
	case x86ARM64Grp2CountOne(ji, memory) && ji.hasModRM && ji.modrm>>6 == 3 && (ji.grpOp == 0 || ji.grpOp == 1) && ji.prefixes&^x86PrefOpSize == 0: // ROL/ROR r/m,1 register
		rm := ji.modrm & 7
		width := x86ARM64Grp2Width(op, ji.prefixes)
		x86ARM64EmitPartialRegLoad(cb, 16, rm, width)
		if ji.grpOp == 0 { // ROL
			cb.Emit32(arm64LSL_W_imm(13, 16, 1))
			cb.Emit32(arm64LSR_W_imm(12, 16, width-1))
			cb.Emit32(arm64ORR_W(13, 13, 12))
			x86ARM64EmitTruncate(cb, 13, width)
			cb.Emit32(arm64AND_W(12, 13, 12))
			cb.Emit32(arm64LSR_W_imm(17, 13, width-1))
			cb.Emit32(arm64EOR_W(17, 17, 12))
		} else { // ROR
			cb.Emit32(arm64LSR_W_imm(13, 16, 1))
			cb.Emit32(arm64LSL_W_imm(12, 16, width-1))
			cb.Emit32(arm64ORR_W(13, 13, 12))
			x86ARM64EmitTruncate(cb, 13, width)
			cb.Emit32(arm64LSR_W_imm(12, 13, width-1))
			cb.Emit32(arm64LSR_W_imm(17, 13, width-2))
			x86ARM64EmitMovImm32(cb, 16, 1)
			cb.Emit32(arm64AND_W(17, 17, 16))
			cb.Emit32(arm64EOR_W(17, 17, 12))
		}
		x86ARM64EmitPartialRegStore(cb, rm, 13, width)
		cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, ^uint32(x86FlagCF|x86FlagOF))
		cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 12))
		cb.Emit32(arm64LSL_W_imm(17, 17, 11))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 17))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
		return true
	case x86ARM64Grp2CountOne(ji, memory) && ji.hasModRM && ji.modrm>>6 == 3 && (ji.grpOp == 4 || ji.grpOp == 5 || ji.grpOp == 6 || ji.grpOp == 7) && ji.prefixes&^x86PrefOpSize == 0: // SHL/SAL/SHR/SAR r/m,1 register
		rm := ji.modrm & 7
		width := x86ARM64Grp2Width(op, ji.prefixes)
		x86ARM64EmitPartialRegLoad(cb, 16, rm, width)
		if ji.grpOp == 4 || ji.grpOp == 6 { // SHL/SAL
			cb.Emit32(arm64LSR_W_imm(12, 16, width-1))
			cb.Emit32(arm64LSL_W_imm(13, 16, 1))
			x86ARM64EmitTruncate(cb, 13, width)
			cb.Emit32(arm64LSR_W_imm(17, 13, width-1))
			cb.Emit32(arm64LSR_W_imm(18, 16, width-1))
			cb.Emit32(arm64EOR_W(17, 17, 18))
		} else if ji.grpOp == 5 { // SHR
			x86ARM64EmitMovImm32(cb, 17, 1)
			cb.Emit32(arm64AND_W(12, 16, 17))
			cb.Emit32(arm64LSR_W_imm(13, 16, 1))
			cb.Emit32(arm64LSR_W_imm(17, 16, width-1))
		} else { // SAR
			x86ARM64EmitMovImm32(cb, 17, 1)
			cb.Emit32(arm64AND_W(12, 16, 17))
			if width == 8 {
				cb.Emit32(arm64SXTB(16, 16))
			} else if width == 16 {
				cb.Emit32(arm64SXTH(16, 16))
			}
			cb.Emit32(arm64ASR_W_imm(13, 16, 1))
			x86ARM64EmitTruncate(cb, 13, width)
			x86ARM64EmitMovImm32(cb, 17, 0)
		}
		x86ARM64EmitPartialRegStore(cb, rm, 13, width)
		// x86ARM64EmitLogicFlags uses W12 while calculating parity. Preserve the
		// carry bit separately for the subsequent merge.
		cb.Emit32(arm64ORR_W(18, 12, 31))
		x86ARM64EmitLogicFlags(cb, 13, width)
		// Logical flag publication cleared CF/OF. Merge the computed bit values
		// directly so this remains a single straight-line native path.
		cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 18))
		cb.Emit32(arm64LSL_W_imm(17, 17, 11))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 17))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
		return true
	case x86ARM64Grp2CountOne(ji, memory) && ji.hasModRM && ji.modrm>>6 != 3 && (ji.grpOp == 4 || ji.grpOp == 5 || ji.grpOp == 6 || ji.grpOp == 7) && ji.prefixes&^x86PrefOpSize == 0: // SHL/SAL/SHR/SAR r/m,1 guarded memory
		width := x86ARM64Grp2Width(op, ji.prefixes) / 8
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 1 {
			cb.Emit32(arm64LDRB_reg(16, x86ARM64Scratch2, 19))
		} else if width == 2 {
			cb.Emit32(arm64LDRH_reg(16, x86ARM64Scratch2, 19))
		} else {
			cb.Emit32(arm64LDR_W_reg(16, x86ARM64Scratch2, 19))
		}
		bits := width * 8
		if ji.grpOp == 4 || ji.grpOp == 6 {
			cb.Emit32(arm64LSR_W_imm(12, 16, bits-1))
			cb.Emit32(arm64LSL_W_imm(13, 16, 1))
			x86ARM64EmitTruncate(cb, 13, bits)
			cb.Emit32(arm64LSR_W_imm(17, 13, bits-1))
			cb.Emit32(arm64LSR_W_imm(18, 16, bits-1))
			cb.Emit32(arm64EOR_W(17, 17, 18))
		} else if ji.grpOp == 5 {
			x86ARM64EmitMovImm32(cb, 17, 1)
			cb.Emit32(arm64AND_W(12, 16, 17))
			cb.Emit32(arm64LSR_W_imm(13, 16, 1))
			cb.Emit32(arm64LSR_W_imm(17, 16, bits-1))
		} else {
			x86ARM64EmitMovImm32(cb, 17, 1)
			cb.Emit32(arm64AND_W(12, 16, 17))
			if width == 1 {
				cb.Emit32(arm64SXTB(16, 16))
			} else if width == 2 {
				cb.Emit32(arm64SXTH(16, 16))
			}
			cb.Emit32(arm64ASR_W_imm(13, 16, 1))
			x86ARM64EmitTruncate(cb, 13, bits)
			x86ARM64EmitMovImm32(cb, 17, 0)
		}
		cb.Emit32(arm64ORR_W(18, 12, 31))
		x86ARM64EmitLogicFlags(cb, 13, bits)
		cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 18))
		cb.Emit32(arm64LSL_W_imm(17, 17, 11))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 17))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 1 {
			cb.Emit32(arm64STRB_reg(13, x86ARM64Scratch2, 19))
		} else if width == 2 {
			cb.Emit32(arm64STRH_reg(13, x86ARM64Scratch2, 19))
		} else {
			cb.Emit32(arm64STR_W_reg(13, x86ARM64Scratch2, 19))
		}
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, 19, 31))
		x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op == 0xFE && ji.hasModRM && ji.modrm>>6 == 3 && (ji.grpOp == 0 || ji.grpOp == 1): // Group 4 INC/DEC r/m8 register
		rm := ji.modrm & 7
		x86ARM64EmitPartialRegLoad(cb, 16, rm, 8)
		x86ARM64EmitMovImm32(cb, 17, 1)
		cb.Emit32(arm64LDR_imm(18, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(18, 18, 0))
		x86ARM64EmitMovImm32(cb, 15, x86FlagCF)
		cb.Emit32(arm64AND_W(18, 18, 15))
		isSub := ji.grpOp == 1
		if isSub {
			cb.Emit32(arm64SUB_W(13, 16, 17))
		} else {
			cb.Emit32(arm64ADD_W(13, 16, 17))
		}
		x86ARM64EmitTruncate(cb, 13, 8)
		x86ARM64EmitArithFlags(cb, 13, 16, 17, 8, isSub)
		cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
		x86ARM64EmitMovImm32(cb, 15, ^uint32(x86FlagCF))
		cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, 15))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 18))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
		x86ARM64EmitPartialRegStore(cb, rm, 13, 8)
		return true
	case op == 0xFE && ji.hasModRM && ji.modrm>>6 != 3 && (ji.grpOp == 0 || ji.grpOp == 1): // Group 4 guarded-memory INC/DEC r/m8
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, 1, ji.opcodePC, retired, bails)
		cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		cb.Emit32(arm64LDRB_reg(16, x86ARM64Scratch2, 19))
		x86ARM64EmitMovImm32(cb, 17, 1)
		cb.Emit32(arm64LDR_imm(18, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(18, 18, 0))
		x86ARM64EmitMovImm32(cb, 15, x86FlagCF)
		cb.Emit32(arm64AND_W(18, 18, 15))
		isSub := ji.grpOp == 1
		if isSub {
			cb.Emit32(arm64SUB_W(13, 16, 17))
		} else {
			cb.Emit32(arm64ADD_W(13, 16, 17))
		}
		x86ARM64EmitTruncate(cb, 13, 8)
		x86ARM64EmitArithFlags(cb, 13, 16, 17, 8, isSub)
		cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
		x86ARM64EmitMovImm32(cb, 15, ^uint32(x86FlagCF))
		cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, 15))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 18))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		cb.Emit32(arm64STRB_reg(13, x86ARM64Scratch2, 19))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, 19, 31))
		x86ARM64EmitSMCStoreCheck(cb, 1, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op >= 0x40 && op <= 0x4F && ji.prefixes&^x86PrefOpSize == 0: // INC/DEC r16/r32
		width := uint32(32)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 16
		}
		reg := op & 7
		x86ARM64EmitPartialRegLoad(cb, 16, reg, width)
		x86ARM64EmitMovImm32(cb, 17, 1)
		// INC/DEC preserve CF. Keep it outside the generic arithmetic publisher.
		cb.Emit32(arm64LDR_imm(18, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(18, 18, 0))
		x86ARM64EmitMovImm32(cb, 15, x86FlagCF)
		cb.Emit32(arm64AND_W(18, 18, 15))
		isSub := op >= 0x48
		if isSub {
			cb.Emit32(arm64SUB_W(13, 16, 17))
		} else {
			cb.Emit32(arm64ADD_W(13, 16, 17))
		}
		x86ARM64EmitTruncate(cb, 13, width)
		x86ARM64EmitArithFlags(cb, 13, 16, 17, width, isSub)
		cb.Emit32(arm64LDR_imm(14, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, 14, 0))
		x86ARM64EmitMovImm32(cb, 15, ^uint32(x86FlagCF))
		cb.Emit32(arm64AND_W(x86ARM64Scratch, x86ARM64Scratch, 15))
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, x86ARM64Scratch, 18))
		cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, 14, 0))
		x86ARM64EmitPartialRegStore(cb, reg, 13, width)
		return true
	case ((op >= 0x00 && op <= 0x03) || (op >= 0x28 && op <= 0x2B) || (op >= 0x38 && op <= 0x3B)) && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0: // ADD/SUB/CMP r/m,r and r,r/m
		width := uint32(32)
		if op&1 == 0 {
			width = 8
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width = 16
		}
		rm, reg := ji.modrm&7, (ji.modrm>>3)&7
		dst, src := rm, reg
		if op&2 != 0 {
			dst, src = reg, rm
		}
		x86ARM64EmitPartialRegLoad(cb, 16, dst, width)
		x86ARM64EmitPartialRegLoad(cb, 17, src, width)
		isSub := op >= 0x28
		if isSub {
			cb.Emit32(arm64SUB_W(13, 16, 17))
		} else {
			cb.Emit32(arm64ADD_W(13, 16, 17))
		}
		x86ARM64EmitTruncate(cb, 13, width)
		x86ARM64EmitArithFlags(cb, 13, 16, 17, width, isSub)
		if op < 0x38 { // CMP has no destination write.
			x86ARM64EmitPartialRegStore(cb, dst, 13, width)
		}
		return true
	case ((op >= 0x00 && op <= 0x03) || (op >= 0x28 && op <= 0x2B) || (op >= 0x38 && op <= 0x3B)) && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes&^x86PrefOpSize == 0: // ADD/SUB/CMP guarded memory form
		width := uint32(4)
		if op&1 == 0 {
			width = 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		// Flag publication owns the normal scratch registers, so retain the
		// guarded EA independently for a possible store and its SMC check.
		cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 1 {
			cb.Emit32(arm64LDRB_reg(17, x86ARM64Scratch2, 19))
		} else if width == 2 {
			cb.Emit32(arm64LDRH_reg(17, x86ARM64Scratch2, 19))
		} else {
			cb.Emit32(arm64LDR_W_reg(17, x86ARM64Scratch2, 19))
		}
		reg := (ji.modrm >> 3) & 7
		dstIsMem := op&2 == 0
		if dstIsMem {
			cb.Emit32(arm64ORR_W(16, 17, 31))
			x86ARM64EmitLoadReg(cb, 17, reg&3)
			x86ARM64EmitTruncate(cb, 17, width*8)
		} else {
			x86ARM64EmitPartialRegLoad(cb, 16, reg, width*8)
			// W17 already holds the guarded memory source.
		}
		isSub := op >= 0x28
		if isSub {
			cb.Emit32(arm64SUB_W(13, 16, 17))
		} else {
			cb.Emit32(arm64ADD_W(13, 16, 17))
		}
		x86ARM64EmitTruncate(cb, 13, width*8)
		x86ARM64EmitArithFlags(cb, 13, 16, 17, width*8, isSub)
		if op < 0x38 {
			if dstIsMem {
				cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
				if width == 1 {
					cb.Emit32(arm64STRB_reg(13, x86ARM64Scratch2, 19))
				} else if width == 2 {
					cb.Emit32(arm64STRH_reg(13, x86ARM64Scratch2, 19))
				} else {
					cb.Emit32(arm64STR_W_reg(13, x86ARM64Scratch2, 19))
				}
				cb.Emit32(arm64ORR_W(x86ARM64Scratch, 19, 31))
				x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
			} else {
				x86ARM64EmitPartialRegStore(cb, reg, 13, width*8)
			}
		}
		return true
	case op == 0x04 || op == 0x05 || op == 0x2C || op == 0x2D || op == 0x3C || op == 0x3D: // ADD/SUB/CMP accumulator,imm
		width, immSize := uint32(32), 4
		if op&1 == 0 {
			width, immSize = 8, 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width, immSize = 16, 2
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) {
			return false
		}
		var imm uint32
		if immSize == 1 {
			imm = uint32(memory[immPC])
		} else if immSize == 2 {
			imm = uint32(binary.LittleEndian.Uint16(memory[immPC:]))
		} else {
			imm = binary.LittleEndian.Uint32(memory[immPC:])
		}
		x86ARM64EmitPartialRegLoad(cb, 16, 0, width)
		x86ARM64EmitMovImm32(cb, 17, imm)
		x86ARM64EmitTruncate(cb, 17, width)
		isSub := op >= 0x2C
		if isSub {
			cb.Emit32(arm64SUB_W(13, 16, 17))
		} else {
			cb.Emit32(arm64ADD_W(13, 16, 17))
		}
		x86ARM64EmitTruncate(cb, 13, width)
		x86ARM64EmitArithFlags(cb, 13, 16, 17, width, isSub)
		if op < 0x3C {
			x86ARM64EmitPartialRegStore(cb, 0, 13, width)
		}
		return true
	case (op == 0x80 || op == 0x81 || op == 0x83) && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0: // Group 1 ADD/SUB/CMP r/m,imm
		aluOp := (ji.modrm >> 3) & 7
		if aluOp != 0 && aluOp != 5 && aluOp != 7 {
			return false
		}
		width, immSize := uint32(32), 4
		if op == 0x80 {
			width, immSize = 8, 1
		} else if op == 0x83 {
			immSize = 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width, immSize = 16, 2
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) {
			return false
		}
		var imm uint32
		if immSize == 1 {
			imm = uint32(int32(int8(memory[immPC])))
		} else if immSize == 2 {
			imm = uint32(binary.LittleEndian.Uint16(memory[immPC:]))
		} else {
			imm = binary.LittleEndian.Uint32(memory[immPC:])
		}
		rm := ji.modrm & 7
		x86ARM64EmitPartialRegLoad(cb, 16, rm, width)
		x86ARM64EmitMovImm32(cb, 17, imm)
		x86ARM64EmitTruncate(cb, 17, width)
		isSub := aluOp != 0
		if isSub {
			cb.Emit32(arm64SUB_W(13, 16, 17))
		} else {
			cb.Emit32(arm64ADD_W(13, 16, 17))
		}
		x86ARM64EmitTruncate(cb, 13, width)
		x86ARM64EmitArithFlags(cb, 13, 16, 17, width, isSub)
		if aluOp != 7 {
			x86ARM64EmitPartialRegStore(cb, rm, 13, width)
		}
		return true
	case (op == 0x80 || op == 0x81 || op == 0x83) && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes&^x86PrefOpSize == 0: // Group 1 guarded-memory arithmetic/logical r/m,imm
		aluOp := (ji.modrm >> 3) & 7
		if aluOp != 0 && aluOp != 1 && aluOp != 4 && aluOp != 5 && aluOp != 6 && aluOp != 7 {
			return false
		}
		width, immSize := uint32(4), 4
		if op == 0x80 {
			width, immSize = 1, 1
		} else if op == 0x83 {
			immSize = 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width, immSize = 2, 2
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) || !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		var imm uint32
		if immSize == 1 {
			imm = uint32(int32(int8(memory[immPC])))
		} else if immSize == 2 {
			imm = uint32(binary.LittleEndian.Uint16(memory[immPC:]))
		} else {
			imm = binary.LittleEndian.Uint32(memory[immPC:])
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 1 {
			cb.Emit32(arm64LDRB_reg(16, x86ARM64Scratch2, 19))
		} else if width == 2 {
			cb.Emit32(arm64LDRH_reg(16, x86ARM64Scratch2, 19))
		} else {
			cb.Emit32(arm64LDR_W_reg(16, x86ARM64Scratch2, 19))
		}
		x86ARM64EmitMovImm32(cb, 17, imm)
		x86ARM64EmitTruncate(cb, 17, width*8)
		if aluOp == 0 || aluOp == 5 || aluOp == 7 {
			isSub := aluOp != 0
			if isSub {
				cb.Emit32(arm64SUB_W(13, 16, 17))
			} else {
				cb.Emit32(arm64ADD_W(13, 16, 17))
			}
			x86ARM64EmitTruncate(cb, 13, width*8)
			x86ARM64EmitArithFlags(cb, 13, 16, 17, width*8, isSub)
		} else {
			if !x86ARM64EmitLogicalOp(cb, aluOp, 16, 17) {
				return false
			}
			cb.Emit32(arm64ORR_W(13, 16, 31))
			x86ARM64EmitLogicFlags(cb, 13, width*8)
		}
		if aluOp != 7 {
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			if width == 1 {
				cb.Emit32(arm64STRB_reg(13, x86ARM64Scratch2, 19))
			} else if width == 2 {
				cb.Emit32(arm64STRH_reg(13, x86ARM64Scratch2, 19))
			} else {
				cb.Emit32(arm64STR_W_reg(13, x86ARM64Scratch2, 19))
			}
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, 19, 31))
			x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		}
		return true
	case (op == 0xF6 || op == 0xF7) && ji.hasModRM && ji.modrm>>6 == 3 && (ji.grpOp == 2 || ji.grpOp == 3) && ji.prefixes&^x86PrefOpSize == 0: // Group 3 NOT/NEG register
		width := uint32(32)
		if op == 0xF6 {
			width = 8
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width = 16
		}
		rm := ji.modrm & 7
		x86ARM64EmitPartialRegLoad(cb, 16, rm, width)
		if ji.grpOp == 2 {
			cb.Emit32(0x2A2003ED | uint32(16)<<16) // MVN W13,W16
		} else {
			cb.Emit32(arm64SUB_W(13, 31, 16))
			x86ARM64EmitMovImm32(cb, 17, 0)
			x86ARM64EmitArithFlags(cb, 13, 17, 16, width, true)
		}
		x86ARM64EmitTruncate(cb, 13, width)
		x86ARM64EmitPartialRegStore(cb, rm, 13, width)
		return true
	case (op == 0xF6 || op == 0xF7) && ji.hasModRM && ji.modrm>>6 != 3 && (ji.grpOp == 2 || ji.grpOp == 3) && ji.prefixes&^x86PrefOpSize == 0: // Group 3 guarded-memory NOT/NEG
		width := uint32(4)
		if op == 0xF6 {
			width = 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 1 {
			cb.Emit32(arm64LDRB_reg(16, x86ARM64Scratch2, 19))
		} else if width == 2 {
			cb.Emit32(arm64LDRH_reg(16, x86ARM64Scratch2, 19))
		} else {
			cb.Emit32(arm64LDR_W_reg(16, x86ARM64Scratch2, 19))
		}
		if ji.grpOp == 2 {
			cb.Emit32(0x2A2003ED | uint32(16)<<16) // MVN W13,W16
		} else {
			cb.Emit32(arm64SUB_W(13, 31, 16))
			x86ARM64EmitMovImm32(cb, 17, 0)
			x86ARM64EmitArithFlags(cb, 13, 17, 16, width*8, true)
		}
		x86ARM64EmitTruncate(cb, 13, width*8)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 1 {
			cb.Emit32(arm64STRB_reg(13, x86ARM64Scratch2, 19))
		} else if width == 2 {
			cb.Emit32(arm64STRH_reg(13, x86ARM64Scratch2, 19))
		} else {
			cb.Emit32(arm64STR_W_reg(13, x86ARM64Scratch2, 19))
		}
		cb.Emit32(arm64ORR_W(x86ARM64Scratch, 19, 31))
		x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op == 0xF7 && ji.hasModRM && ji.modrm>>6 == 3 && (ji.grpOp == 0 || ji.grpOp == 1) && ji.prefixes&^x86PrefOpSize == 0: // Group 3 TEST r16/r32,imm
		width, immSize := uint32(32), 4
		if ji.prefixes&x86PrefOpSize != 0 {
			width, immSize = 16, 2
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) {
			return false
		}
		var imm uint32
		if immSize == 2 {
			imm = uint32(binary.LittleEndian.Uint16(memory[immPC:]))
		} else {
			imm = binary.LittleEndian.Uint32(memory[immPC:])
		}
		x86ARM64EmitPartialRegLoad(cb, 16, ji.modrm&7, width)
		x86ARM64EmitMovImm32(cb, 17, imm)
		cb.Emit32(arm64AND_W(13, 16, 17))
		x86ARM64EmitLogicFlags(cb, 13, width)
		return true
	case op == 0xF6 && ji.hasModRM && ji.modrm>>6 == 3 && (ji.grpOp == 0 || ji.grpOp == 1) && ji.prefixes == 0: // Group 3 TEST r8,imm8
		immPC := int(ji.opcodePC) + int(ji.length) - 1
		if immPC < 0 || immPC >= len(memory) {
			return false
		}
		x86ARM64EmitPartialRegLoad(cb, 16, ji.modrm&7, 8)
		x86ARM64EmitMovImm32(cb, 17, uint32(memory[immPC]))
		cb.Emit32(arm64AND_W(13, 16, 17))
		x86ARM64EmitLogicFlags(cb, 13, 8)
		return true
	case (op == 0xF6 || op == 0xF7) && ji.hasModRM && ji.modrm>>6 != 3 && (ji.grpOp == 0 || ji.grpOp == 1) && ji.prefixes&^x86PrefOpSize == 0: // Group 3 guarded-memory TEST r/m,imm
		width, immSize := uint32(4), 4
		if op == 0xF6 {
			width, immSize = 1, 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width, immSize = 2, 2
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) || !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		var imm uint32
		if immSize == 1 {
			imm = uint32(memory[immPC])
		} else if immSize == 2 {
			imm = uint32(binary.LittleEndian.Uint16(memory[immPC:]))
		} else {
			imm = binary.LittleEndian.Uint32(memory[immPC:])
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 1 {
			cb.Emit32(arm64LDRB_reg(16, x86ARM64Scratch2, x86ARM64Scratch))
		} else if width == 2 {
			cb.Emit32(arm64LDRH_reg(16, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64LDR_W_reg(16, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitMovImm32(cb, 17, imm)
		cb.Emit32(arm64AND_W(13, 16, 17))
		x86ARM64EmitLogicFlags(cb, 13, width*8)
		return true
	case (op == 0x08 || op == 0x09 || op == 0x20 || op == 0x21 || op == 0x30 || op == 0x31) && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0: // OR/AND/XOR r/m,r
		rm, reg := ji.modrm&7, (ji.modrm>>3)&7
		width := uint32(32)
		if op&1 == 0 {
			width = 8
			x86ARM64EmitLoadReg(cb, 13, rm&3)
			x86ARM64EmitByteExtract(cb, 13, 13, rm >= 4)
			x86ARM64EmitLoadReg(cb, 12, reg&3)
			x86ARM64EmitByteExtract(cb, 12, 12, reg >= 4)
		} else {
			if ji.prefixes&x86PrefOpSize != 0 {
				width = 16
			}
			x86ARM64EmitLoadReg(cb, 13, rm)
			x86ARM64EmitLoadReg(cb, 12, reg)
			x86ARM64EmitTruncate(cb, 13, width)
			x86ARM64EmitTruncate(cb, 12, width)
		}
		if !x86ARM64EmitLogicalOp(cb, (op>>3)&7, 13, 12) {
			return false
		}
		x86ARM64EmitPartialRegStore(cb, rm, 13, width)
		x86ARM64EmitLogicFlags(cb, 13, width)
		return true
	case ((op >= 0x08 && op <= 0x0B) || (op >= 0x20 && op <= 0x23) || (op >= 0x30 && op <= 0x33)) && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes&^x86PrefOpSize == 0:
		width := uint32(4)
		if op&1 == 0 {
			width = 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64ORR_W(19, x86ARM64Scratch, 31))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 1 {
			cb.Emit32(arm64LDRB_reg(17, x86ARM64Scratch2, 19))
		} else if width == 2 {
			cb.Emit32(arm64LDRH_reg(17, x86ARM64Scratch2, 19))
		} else {
			cb.Emit32(arm64LDR_W_reg(17, x86ARM64Scratch2, 19))
		}
		reg, dstIsMem := (ji.modrm>>3)&7, op&2 == 0
		if dstIsMem {
			cb.Emit32(arm64ORR_W(16, 17, 31))
			x86ARM64EmitPartialRegLoad(cb, 17, reg, width*8)
		} else {
			x86ARM64EmitPartialRegLoad(cb, 16, reg, width*8)
		}
		if !x86ARM64EmitLogicalOp(cb, (op>>3)&7, 16, 17) {
			return false
		}
		cb.Emit32(arm64ORR_W(13, 16, 31))
		x86ARM64EmitLogicFlags(cb, 13, width*8)
		if dstIsMem {
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
			if width == 1 {
				cb.Emit32(arm64STRB_reg(13, x86ARM64Scratch2, 19))
			} else if width == 2 {
				cb.Emit32(arm64STRH_reg(13, x86ARM64Scratch2, 19))
			} else {
				cb.Emit32(arm64STR_W_reg(13, x86ARM64Scratch2, 19))
			}
			cb.Emit32(arm64ORR_W(x86ARM64Scratch, 19, 31))
			x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		} else {
			x86ARM64EmitPartialRegStore(cb, reg, 13, width*8)
		}
		return true
	case (op == 0x0C || op == 0x0D || op == 0x24 || op == 0x25 || op == 0x34 || op == 0x35) && ji.prefixes&^x86PrefOpSize == 0: // OR/AND/XOR accumulator,imm
		width, immSize := uint32(32), 4
		if op&1 == 0 {
			width, immSize = 8, 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width, immSize = 16, 2
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) {
			return false
		}
		var imm uint32
		switch immSize {
		case 1:
			imm = uint32(memory[immPC])
		case 2:
			imm = uint32(binary.LittleEndian.Uint16(memory[immPC:]))
		default:
			imm = binary.LittleEndian.Uint32(memory[immPC:])
		}
		x86ARM64EmitLoadReg(cb, 13, 0)
		if width == 8 {
			x86ARM64EmitByteExtract(cb, 13, 13, false)
		} else {
			x86ARM64EmitTruncate(cb, 13, width)
		}
		x86ARM64EmitMovImm32(cb, 12, imm)
		if !x86ARM64EmitLogicalOp(cb, (op>>3)&7, 13, 12) {
			return false
		}
		x86ARM64EmitPartialRegStore(cb, 0, 13, width)
		x86ARM64EmitLogicFlags(cb, 13, width)
		return true
	case (op == 0x80 || op == 0x81 || op == 0x83) && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0: // Group 1 logical r/m,imm
		aluOp := (ji.modrm >> 3) & 7
		if aluOp != 1 && aluOp != 4 && aluOp != 6 {
			return false
		}
		width, immSize := uint32(32), 4
		if op == 0x80 {
			width, immSize = 8, 1
		} else if op == 0x83 {
			immSize = 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width, immSize = 16, 2
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) {
			return false
		}
		var imm uint32
		if immSize == 1 {
			imm = uint32(int32(int8(memory[immPC])))
		} else if immSize == 2 {
			imm = uint32(binary.LittleEndian.Uint16(memory[immPC:]))
		} else {
			imm = binary.LittleEndian.Uint32(memory[immPC:])
		}
		rm := ji.modrm & 7
		x86ARM64EmitLoadReg(cb, 13, rm&3)
		if width == 8 {
			x86ARM64EmitByteExtract(cb, 13, 13, rm >= 4)
		} else {
			x86ARM64EmitTruncate(cb, 13, width)
		}
		x86ARM64EmitMovImm32(cb, 12, imm)
		x86ARM64EmitTruncate(cb, 12, width)
		if !x86ARM64EmitLogicalOp(cb, aluOp, 13, 12) {
			return false
		}
		x86ARM64EmitPartialRegStore(cb, rm, 13, width)
		x86ARM64EmitLogicFlags(cb, 13, width)
		return true
	case (op == 0x84 || op == 0x85) && ji.hasModRM && ji.modrm>>6 == 3: // TEST r/m,r
		width := uint32(32)
		if op == 0x84 {
			width = 8
			x86ARM64EmitLoadReg(cb, 13, (ji.modrm&7)&3)
			x86ARM64EmitByteExtract(cb, 13, 13, ji.modrm&7 >= 4)
			x86ARM64EmitLoadReg(cb, 12, ((ji.modrm>>3)&7)&3)
			x86ARM64EmitByteExtract(cb, 12, 12, (ji.modrm>>3)&7 >= 4)
		} else {
			if ji.prefixes&x86PrefOpSize != 0 {
				width = 16
			}
			x86ARM64EmitLoadReg(cb, 13, ji.modrm&7)
			x86ARM64EmitLoadReg(cb, 12, (ji.modrm>>3)&7)
			x86ARM64EmitTruncate(cb, 13, width)
			x86ARM64EmitTruncate(cb, 12, width)
		}
		cb.Emit32(arm64AND_W(13, 13, 12))
		x86ARM64EmitLogicFlags(cb, 13, width)
		return true
	case (op == 0x84 || op == 0x85) && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes&^x86PrefOpSize == 0: // TEST guarded r/m,r
		width := uint32(4)
		if op == 0x84 {
			width = 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 1 {
			cb.Emit32(arm64LDRB_reg(13, x86ARM64Scratch2, x86ARM64Scratch))
		} else if width == 2 {
			cb.Emit32(arm64LDRH_reg(13, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64LDR_W_reg(13, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitPartialRegLoad(cb, 12, (ji.modrm>>3)&7, width*8)
		cb.Emit32(arm64AND_W(13, 13, 12))
		x86ARM64EmitLogicFlags(cb, 13, width*8)
		return true
	case (op == 0xA8 || op == 0xA9) && ji.prefixes&^x86PrefOpSize == 0: // TEST accumulator,imm
		width, immSize := uint32(32), 4
		if op == 0xA8 {
			width, immSize = 8, 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width, immSize = 16, 2
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immSize
		if immPC < 0 || immPC+immSize > len(memory) {
			return false
		}
		var imm uint32
		if immSize == 1 {
			imm = uint32(memory[immPC])
		} else if immSize == 2 {
			imm = uint32(binary.LittleEndian.Uint16(memory[immPC:]))
		} else {
			imm = binary.LittleEndian.Uint32(memory[immPC:])
		}
		x86ARM64EmitLoadReg(cb, 13, 0)
		if width == 8 {
			x86ARM64EmitByteExtract(cb, 13, 13, false)
		} else {
			x86ARM64EmitTruncate(cb, 13, width)
		}
		x86ARM64EmitMovImm32(cb, 12, imm)
		cb.Emit32(arm64AND_W(13, 13, 12))
		x86ARM64EmitLogicFlags(cb, 13, width)
		return true
	case op == 0xC8 && ji.prefixes == 0 && ji.length == 4: // ENTER imm16,0
		immPC := int(ji.opcodePC) + int(ji.length) - 3
		if immPC < 0 || immPC+3 > len(memory) || memory[immPC+2]&0x1F != 0 {
			return false
		}
		frameSize := uint32(binary.LittleEndian.Uint16(memory[immPC:]))
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 4)
		cb.Emit32(arm64SUB_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitSpanGuard(cb, 4, ji.opcodePC, retired, bails)
		x86ARM64EmitLoadReg(cb, 12, 5)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		cb.Emit32(arm64STR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		x86ARM64EmitStoreReg(cb, 5, x86ARM64Scratch)
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, frameSize)
		cb.Emit32(arm64SUB_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
		x86ARM64EmitSMCStoreCheck(cb, 4, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op == 0xC9: // LEAVE
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if width == 2 {
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, 5)
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, x86ARM64Scratch2)
		} else {
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 5)
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64LDRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64LDR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, width)
		cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
		if width == 2 {
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 5)
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, 12)
			x86ARM64EmitStoreReg(cb, 5, x86ARM64Scratch)
		} else {
			x86ARM64EmitStoreReg(cb, 5, 12)
		}
		return true
	case op == 0xD6: // SALC
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(12, x86ARM64Scratch2, 0))
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, x86FlagCF)
		cb.Emit32(arm64AND_W(12, 12, x86ARM64Scratch2))
		cb.Emit32(arm64SUB_W(12, 31, 12))
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 0)
		x86ARM64EmitByteInsert(cb, x86ARM64Scratch, 12, false)
		x86ARM64EmitStoreReg(cb, 0, x86ARM64Scratch)
		return true
	case op == 0xD7: // XLAT
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 3)
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, 0)
		x86ARM64EmitByteExtract(cb, x86ARM64Scratch2, x86ARM64Scratch2, false)
		cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitSpanGuard(cb, 1, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		cb.Emit32(arm64LDRB_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 0)
		x86ARM64EmitByteInsert(cb, x86ARM64Scratch, 12, false)
		x86ARM64EmitStoreReg(cb, 0, x86ARM64Scratch)
		return true
	case op >= 0xA0 && op <= 0xA3: // MOV AL/AX/EAX, moffs or reverse
		if ji.prefixes&x86PrefAddrSize != 0 || ji.length < 5 {
			return false
		}
		addrPC := int(ji.opcodePC) + int(ji.length) - 4
		if addrPC < 0 || addrPC+4 > len(memory) {
			return false
		}
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch, binary.LittleEndian.Uint32(memory[addrPC:]))
		width := uint32(4)
		if op == 0xA0 || op == 0xA2 {
			width = 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if op == 0xA0 || op == 0xA1 {
			if width == 1 {
				cb.Emit32(arm64LDRB_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
				x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, 0)
				x86ARM64EmitByteInsert(cb, x86ARM64Scratch2, 12, false)
				x86ARM64EmitStoreReg(cb, 0, x86ARM64Scratch2)
			} else if width == 2 {
				cb.Emit32(arm64LDRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
				x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, 0)
				x86ARM64EmitWordInsert(cb, x86ARM64Scratch2, 12)
				x86ARM64EmitStoreReg(cb, 0, x86ARM64Scratch2)
			} else {
				cb.Emit32(arm64LDR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
				x86ARM64EmitStoreReg(cb, 0, 12)
			}
			return true
		}
		x86ARM64EmitLoadReg(cb, 12, 0)
		if width == 1 {
			x86ARM64EmitByteExtract(cb, 12, 12, false)
			cb.Emit32(arm64STRB_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else if width == 2 {
			cb.Emit32(arm64STRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64STR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op == 0x8F && ji.hasModRM && ji.grpOp == 0 && ji.modrm>>6 != 3: // POP Ev
		if x86ARM64EADependsOnESP(ji, memory) {
			return false
		}
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		// Read the stack operand first. Its guard must precede every state
		// mutation, just as pop32/pop16 does in the interpreter.
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64LDRH_reg(13, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64LDR_W_reg(13, x86ARM64Scratch2, x86ARM64Scratch))
		}
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, 4)
		x86ARM64EmitMovImm32(cb, 12, width)
		cb.Emit32(arm64ADD_W(x86ARM64Scratch2, x86ARM64Scratch2, 12))
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch2)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64STRH_reg(13, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64STR_W_reg(13, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case (op == 0xC4 || op == 0xC5) && ji.hasModRM: // LES/LDS Ev,Mp
		// CPU_X86 uses a flat real-mode model: fetch an operand-sized offset
		// followed by a 16-bit selector, leaving all EFLAGS unchanged.  Its
		// ModR/M=3 behaviour treats r/m as the pointer address, matching the
		// amd64 direct path.
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if ji.modrm>>6 == 3 {
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, ji.modrm&7)
		} else if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, width+2, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		dst := (ji.modrm >> 3) & 7
		if width == 2 {
			cb.Emit32(arm64LDRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
			x86ARM64EmitLoadReg(cb, 13, dst)
			x86ARM64EmitWordInsert(cb, 13, 12)
			x86ARM64EmitStoreReg(cb, dst, 13)
		} else {
			cb.Emit32(arm64LDR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
			x86ARM64EmitStoreReg(cb, dst, 12)
		}
		x86ARM64EmitMovImm32(cb, 12, width)
		cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, 12))
		cb.Emit32(arm64LDRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
		seg := byte(x86SegES)
		if op == 0xC5 {
			seg = x86SegDS
		}
		cb.Emit32(arm64STRH_imm(12, x86ARM64Scratch2, uint32(seg)))
		return true
	case op == 0x60: // PUSHA / PUSHAD
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		total := width * 8
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, total)
		cb.Emit32(arm64SUB_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitSpanGuard(cb, total, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		store := func(off uint32, reg byte, originalESP bool) {
			if originalESP {
				x86ARM64EmitAddrOffset(cb, 12, x86ARM64Scratch, total)
			} else {
				x86ARM64EmitLoadReg(cb, 12, reg)
			}
			x86ARM64EmitAddrOffset(cb, 13, x86ARM64Scratch, off)
			if width == 2 {
				cb.Emit32(arm64STRH_reg(12, x86ARM64Scratch2, 13))
			} else {
				cb.Emit32(arm64STR_W_reg(12, x86ARM64Scratch2, 13))
			}
		}
		// The lowest address receives DI, matching the interpreter's sequential
		// pushes of AX, CX, DX, BX, original SP, BP, SI and DI.
		store(0*width, 7, false)
		store(1*width, 6, false)
		store(2*width, 5, false)
		store(3*width, 4, true)
		store(4*width, 3, false)
		store(5*width, 2, false)
		store(6*width, 1, false)
		store(7*width, 0, false)
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
		x86ARM64EmitSMCStoreCheck(cb, total, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op == 0x61: // POPA / POPAD
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		total := width * 8
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
		x86ARM64EmitSpanGuard(cb, total, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		load := func(off uint32, reg byte) {
			x86ARM64EmitAddrOffset(cb, 13, x86ARM64Scratch, off)
			if width == 2 {
				cb.Emit32(arm64LDRH_reg(12, x86ARM64Scratch2, 13))
				x86ARM64EmitLoadReg(cb, 13, reg)
				x86ARM64EmitWordInsert(cb, 13, 12)
				x86ARM64EmitStoreReg(cb, reg, 13)
			} else {
				cb.Emit32(arm64LDR_W_reg(12, x86ARM64Scratch2, 13))
				x86ARM64EmitStoreReg(cb, reg, 12)
			}
		}
		load(0*width, 7)
		load(1*width, 6)
		load(2*width, 5)
		// Skip the saved SP/ESP slot.
		load(4*width, 3)
		load(5*width, 2)
		load(6*width, 1)
		load(7*width, 0)
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, total)
		cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
		return true
	case op == 0x9C: // PUSHF / PUSHFD
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, width)
		cb.Emit32(arm64SUB_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(12, x86ARM64Scratch2, 0))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64STRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64STR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
		x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op == 0x9D: // POPF / POPFD
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64LDRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64LDR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, width)
		cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		if width == 2 {
			cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, x86ARM64Scratch2, 0))
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, 12)
			cb.Emit32(arm64STR_W_imm(x86ARM64Scratch, x86ARM64Scratch2, 0))
		} else {
			cb.Emit32(arm64STR_W_imm(12, x86ARM64Scratch2, 0))
		}
		return true
	case op == 0x06 || op == 0x0E || op == 0x16 || op == 0x1E: // PUSH ES/CS/SS/DS
		seg := byte(x86SegES)
		switch op {
		case 0x0E:
			seg = x86SegCS
		case 0x16:
			seg = x86SegSS
		case 0x1E:
			seg = x86SegDS
		}
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 2)
		cb.Emit32(arm64SUB_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitSpanGuard(cb, 2, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
		cb.Emit32(arm64LDRH_imm(12, x86ARM64Scratch2, uint32(seg)))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		cb.Emit32(arm64STRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
		x86ARM64EmitSMCStoreCheck(cb, 2, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op == 0x07 || op == 0x17 || op == 0x1F: // POP ES/SS/DS
		seg := byte(x86SegES)
		switch op {
		case 0x17:
			seg = x86SegSS
		case 0x1F:
			seg = x86SegDS
		}
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
		x86ARM64EmitSpanGuard(cb, 2, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		cb.Emit32(arm64LDRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, 2)
		cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
		cb.Emit32(arm64STRH_imm(12, x86ARM64Scratch2, uint32(seg)))
		return true
	case op == 0x68 || op == 0x6A: // PUSH imm16/imm32 or sign-extended imm8
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		immBytes := int(width)
		if op == 0x6A {
			immBytes = 1
		}
		immPC := int(ji.opcodePC) + int(ji.length) - immBytes
		if immPC < 0 || immPC+immBytes > len(memory) {
			return false
		}
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, width)
		cb.Emit32(arm64SUB_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		if op == 0x6A {
			x86ARM64EmitMovImm32(cb, 12, uint32(int32(int8(memory[immPC]))))
		} else if width == 2 {
			x86ARM64EmitMovImm32(cb, 12, uint32(binary.LittleEndian.Uint16(memory[immPC:])))
		} else {
			x86ARM64EmitMovImm32(cb, 12, binary.LittleEndian.Uint32(memory[immPC:]))
		}
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64STRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64STR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
		x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op >= 0x50 && op <= 0x57: // PUSH r32 / r16
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, width)
		cb.Emit32(arm64SUB_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		x86ARM64EmitLoadReg(cb, 12, op&7)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64STRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64STR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		}
		// Publish the decremented ESP before an SMC exit. PUSH ESP uses the
		// pre-decrement register value already captured in W12 above.
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
		x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op >= 0x58 && op <= 0x5F: // POP r32 / r16
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 4)
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64LDRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64LDR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, width)
		cb.Emit32(arm64ADD_W(x86ARM64Scratch, x86ARM64Scratch, x86ARM64Scratch2))
		x86ARM64EmitStoreReg(cb, 4, x86ARM64Scratch)
		dst := op & 7
		if width == 2 {
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, dst)
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, 12)
			x86ARM64EmitStoreReg(cb, dst, x86ARM64Scratch)
		} else {
			x86ARM64EmitStoreReg(cb, dst, 12)
		}
		return true
	case op == 0x90 || op == 0x9B: // NOP / WAIT, including ignored 0x66
		cb.Emit32(arm64NOP())
		return true
	case op == 0xF8: // CLC
		x86ARM64EmitFlagBit(cb, x86FlagCF, false, false)
		return true
	case op == 0xF9: // STC
		x86ARM64EmitFlagBit(cb, x86FlagCF, true, false)
		return true
	case op == 0xF5: // CMC
		x86ARM64EmitFlagBit(cb, x86FlagCF, false, true)
		return true
	case op == 0xFC: // CLD
		x86ARM64EmitFlagBit(cb, x86FlagDF, false, false)
		return true
	case op == 0xFD: // STD
		x86ARM64EmitFlagBit(cb, x86FlagDF, true, false)
		return true
	case op == 0xFA: // CLI
		x86ARM64EmitFlagBit(cb, x86FlagIF, false, false)
		return true
	case op == 0xFB: // STI
		x86ARM64EmitFlagBit(cb, x86FlagIF, true, false)
		return true
	case op == 0x9E: // SAHF
		// Flags[7:0] = AH, preserving every higher architected bit.
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 0)
		x86ARM64EmitByteExtract(cb, x86ARM64Scratch, x86ARM64Scratch, true)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(12, x86ARM64Scratch2, 0))
		x86ARM64EmitMovImm32(cb, 13, 0xFFFFFF00)
		cb.Emit32(arm64AND_W(12, 12, 13))
		cb.Emit32(arm64ORR_W(12, 12, x86ARM64Scratch))
		cb.Emit32(arm64STR_W_imm(12, x86ARM64Scratch2, 0))
		return true
	case op == 0x9F: // LAHF
		// AH = Flags[7:0], preserving the rest of EAX.
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffFlagsPtr/8))
		cb.Emit32(arm64LDR_W_imm(x86ARM64Scratch, x86ARM64Scratch2, 0))
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, 0)
		x86ARM64EmitByteInsert(cb, x86ARM64Scratch2, x86ARM64Scratch, true)
		x86ARM64EmitStoreReg(cb, 0, x86ARM64Scratch2)
		return true
	case op == 0x98: // CBW / CWDE
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 0)
		if ji.prefixes&x86PrefOpSize != 0 {
			x86ARM64EmitByteExtract(cb, x86ARM64Scratch2, x86ARM64Scratch, false)
			cb.Emit32(arm64SXTB(x86ARM64Scratch2, x86ARM64Scratch2))
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, x86ARM64Scratch2)
		} else {
			cb.Emit32(arm64SXTH(x86ARM64Scratch, x86ARM64Scratch))
		}
		x86ARM64EmitStoreReg(cb, 0, x86ARM64Scratch)
		return true
	case op == 0x99: // CWD / CDQ
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 0)
		if ji.prefixes&x86PrefOpSize != 0 {
			// DX receives the all-zero or all-one sign word of AX.
			cb.Emit32(arm64UBFX_W(x86ARM64Scratch2, x86ARM64Scratch, 15, 1))
			cb.Emit32(arm64SUB_W(x86ARM64Scratch2, 31, x86ARM64Scratch2))
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 2)
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, x86ARM64Scratch2)
		} else {
			x86ARM64EmitMovImm32(cb, 12, 31)
			cb.Emit32(arm64ASR_W(x86ARM64Scratch, x86ARM64Scratch, 12))
		}
		x86ARM64EmitStoreReg(cb, 2, x86ARM64Scratch)
		return true
	case op >= 0xB8 && op <= 0xBF: // MOV r32, imm32 / MOV r16,imm16
		if int(ji.opcodePC)+int(ji.length) > len(memory) {
			return false
		}
		if ji.prefixes&x86PrefOpSize != 0 {
			if ji.length != 4 {
				return false
			}
			guest := op - 0xB8
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, guest)
			x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(binary.LittleEndian.Uint16(memory[int(ji.opcodePC)+2:])))
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, x86ARM64Scratch2)
			x86ARM64EmitStoreReg(cb, guest, x86ARM64Scratch)
			return true
		}
		if ji.length != 5 {
			return false
		}
		v := binary.LittleEndian.Uint32(memory[int(ji.opcodePC)+1:])
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch, v)
		x86ARM64EmitStoreReg(cb, op-0xB8, x86ARM64Scratch)
		return true
	case op >= 0xB0 && op <= 0xB7: // MOV r8, imm8
		if ji.length != 2 || int(ji.opcodePC)+1 >= len(memory) {
			return false
		}
		guest, high := op&3, op >= 0xB4
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, guest)
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(memory[int(ji.opcodePC)+1]))
		x86ARM64EmitByteInsert(cb, x86ARM64Scratch, x86ARM64Scratch2, high)
		x86ARM64EmitStoreReg(cb, guest, x86ARM64Scratch)
		return true
	case op == 0xC6 && ji.hasModRM && ji.modrm>>6 != 3 && ji.grpOp == 0: // MOV m8,imm8
		if ji.prefixes != 0 || ji.length < 3 || !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		immPC := int(ji.opcodePC) + int(ji.length) - 1
		if immPC < 0 || immPC >= len(memory) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, 1, ji.opcodePC, retired, bails)
		x86ARM64EmitMovImm32(cb, 12, uint32(memory[immPC]))
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		cb.Emit32(arm64STRB_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		x86ARM64EmitSMCStoreCheck(cb, 1, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op == 0xC6 && ji.hasModRM && ji.modrm>>6 == 3 && ji.grpOp == 0: // MOV r/m8,imm8
		if ji.length != 3 || int(ji.opcodePC)+2 >= len(memory) {
			return false
		}
		dst := ji.modrm & 7
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, dst&3)
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(memory[int(ji.opcodePC)+2]))
		x86ARM64EmitByteInsert(cb, x86ARM64Scratch, x86ARM64Scratch2, dst >= 4)
		x86ARM64EmitStoreReg(cb, dst&3, x86ARM64Scratch)
		return true
	case op == 0xC7 && ji.hasModRM && ji.modrm>>6 != 3 && ji.grpOp == 0: // MOV m32,imm32 / m16,imm16
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		immPC := int(ji.opcodePC) + int(ji.length) - int(width)
		if immPC < 0 || immPC+int(width) > len(memory) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		if width == 2 {
			x86ARM64EmitMovImm32(cb, 12, uint32(binary.LittleEndian.Uint16(memory[immPC:])))
		} else {
			x86ARM64EmitMovImm32(cb, 12, binary.LittleEndian.Uint32(memory[immPC:]))
		}
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64STRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64STR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op == 0xC7 && ji.hasModRM && ji.modrm>>6 == 3 && ji.grpOp == 0: // MOV r/m32,imm32 / r/m16,imm16
		dst := ji.modrm & 7
		if ji.prefixes&x86PrefOpSize != 0 {
			if ji.length != 5 || int(ji.opcodePC)+4 >= len(memory) {
				return false
			}
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, dst)
			x86ARM64EmitMovImm32(cb, x86ARM64Scratch2, uint32(binary.LittleEndian.Uint16(memory[int(ji.opcodePC)+3:])))
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, x86ARM64Scratch2)
			x86ARM64EmitStoreReg(cb, dst, x86ARM64Scratch)
			return true
		}
		if ji.length != 6 || int(ji.opcodePC)+5 >= len(memory) {
			return false
		}
		x86ARM64EmitMovImm32(cb, x86ARM64Scratch, binary.LittleEndian.Uint32(memory[int(ji.opcodePC)+2:]))
		x86ARM64EmitStoreReg(cb, dst, x86ARM64Scratch)
		return true
	case op == 0x88 && ji.hasModRM && ji.modrm>>6 != 3: // MOV m8,r8
		if ji.prefixes != 0 || !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, 1, ji.opcodePC, retired, bails)
		src := (ji.modrm >> 3) & 7
		x86ARM64EmitLoadReg(cb, 12, src&3)
		x86ARM64EmitByteExtract(cb, 12, 12, src >= 4)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		cb.Emit32(arm64STRB_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		x86ARM64EmitSMCStoreCheck(cb, 1, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case (op == 0x88 || op == 0x8A) && ji.hasModRM && ji.modrm>>6 == 3: // MOV r8,r8
		reg, rm := (ji.modrm>>3)&7, ji.modrm&7
		src, dst := reg, rm
		if op == 0x8A {
			src, dst = rm, reg
		}
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, dst&3)
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, src&3)
		x86ARM64EmitByteExtract(cb, x86ARM64Scratch2, x86ARM64Scratch2, src >= 4)
		x86ARM64EmitByteInsert(cb, x86ARM64Scratch, x86ARM64Scratch2, dst >= 4)
		x86ARM64EmitStoreReg(cb, dst&3, x86ARM64Scratch)
		return true
	case op == 0x89 && ji.hasModRM && ji.modrm>>6 != 3: // MOV m32,r32 / m16,r16
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		x86ARM64EmitLoadReg(cb, 12, (ji.modrm>>3)&7)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64STRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64STR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case (op == 0x89 || op == 0x8B) && ji.hasModRM && ji.modrm>>6 == 3: // MOV r32,r32
		reg, rm := (ji.modrm>>3)&7, ji.modrm&7
		if ji.prefixes&x86PrefOpSize != 0 {
			src, dst := reg, rm
			if op == 0x8B {
				src, dst = rm, reg
			}
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, dst)
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, src)
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, x86ARM64Scratch2)
			x86ARM64EmitStoreReg(cb, dst, x86ARM64Scratch)
			return true
		}
		if op == 0x89 {
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, reg)
			x86ARM64EmitStoreReg(cb, rm, x86ARM64Scratch)
		} else {
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, rm)
			x86ARM64EmitStoreReg(cb, reg, x86ARM64Scratch)
		}
		return true
	case op == 0x8A && ji.hasModRM && ji.modrm>>6 != 3: // MOV r8,m8
		if ji.prefixes != 0 || !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		x86ARM64EmitSpanGuard(cb, 1, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		cb.Emit32(arm64LDRB_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		dst := (ji.modrm >> 3) & 7
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, dst&3)
		x86ARM64EmitByteInsert(cb, x86ARM64Scratch, 12, dst >= 4)
		x86ARM64EmitStoreReg(cb, dst&3, x86ARM64Scratch)
		return true
	case op == 0x8B && ji.hasModRM && ji.modrm>>6 != 3: // MOV r32,m32 / r16,m16
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		dst := (ji.modrm >> 3) & 7
		if width == 2 {
			cb.Emit32(arm64LDRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
			x86ARM64EmitLoadReg(cb, x86ARM64Scratch, dst)
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, 12)
		} else {
			cb.Emit32(arm64LDR_W_reg(x86ARM64Scratch, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitStoreReg(cb, dst, x86ARM64Scratch)
		return true
	case op == 0x8C && ji.hasModRM && ji.modrm>>6 != 3: // MOV Ev,Sreg
		seg := (ji.modrm >> 3) & 7
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		if seg <= x86SegGS {
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
			cb.Emit32(arm64LDRH_imm(12, x86ARM64Scratch2, uint32(seg)))
		} else {
			x86ARM64EmitMovImm32(cb, 12, 0)
		}
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64STRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64STR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		}
		x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op == 0x8C && ji.hasModRM && ji.modrm>>6 == 3: // MOV r/m16,Sreg
		seg, dst := (ji.modrm>>3)&7, ji.modrm&7
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, dst)
		if seg <= x86SegGS {
			cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
			cb.Emit32(arm64LDRH_imm(12, x86ARM64Scratch2, uint32(seg)))
		} else {
			x86ARM64EmitMovImm32(cb, 12, 0)
		}
		x86ARM64EmitWordInsert(cb, x86ARM64Scratch, 12)
		x86ARM64EmitStoreReg(cb, dst, x86ARM64Scratch)
		return true
	case op == 0x8E && ji.hasModRM && ji.modrm>>6 != 3: // MOV Sreg,Ev
		seg := (ji.modrm >> 3) & 7
		if seg > x86SegGS || !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64LDRH_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64LDR_W_reg(12, x86ARM64Scratch2, x86ARM64Scratch))
		}
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
		cb.Emit32(arm64STRH_imm(12, x86ARM64Scratch2, uint32(seg)))
		return true
	case op == 0x8E && ji.hasModRM && ji.modrm>>6 == 3: // MOV Sreg,r/m16
		seg, src := (ji.modrm>>3)&7, ji.modrm&7
		if seg > x86SegGS {
			return true // CPU_X86.setSeg ignores invalid selectors.
		}
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, src)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffSegRegsPtr/8))
		cb.Emit32(arm64STRH_imm(x86ARM64Scratch, x86ARM64Scratch2, uint32(seg)))
		return true
	case op == 0x8D && ji.hasModRM: // LEA r32, [base+index*scale+disp]
		if ji.prefixes != 0 {
			return false
		}
		reg := (ji.modrm >> 3) & 7
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		x86ARM64EmitStoreReg(cb, reg, x86ARM64Scratch)
		return true
	case op >= 0x91 && op <= 0x97: // XCHG EAX,r32
		other := op - 0x90
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, 0)
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, other)
		if ji.prefixes&x86PrefOpSize != 0 {
			cb.Emit32(arm64ORR_W(12, x86ARM64Scratch, 31))
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, x86ARM64Scratch2)
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch2, 12)
			x86ARM64EmitStoreReg(cb, 0, x86ARM64Scratch)
			x86ARM64EmitStoreReg(cb, other, x86ARM64Scratch2)
			return true
		}
		x86ARM64EmitStoreReg(cb, 0, x86ARM64Scratch2)
		x86ARM64EmitStoreReg(cb, other, x86ARM64Scratch)
		return true
	case op == 0x87 && ji.hasModRM && ji.modrm>>6 != 3: // XCHG m32,r32 / m16,r16
		if !x86ARM64EmitEA32(cb, ji, memory, x86ARM64Scratch) {
			return false
		}
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86ARM64EmitSpanGuard(cb, width, ji.opcodePC, retired, bails)
		reg := (ji.modrm >> 3) & 7
		x86ARM64EmitLoadReg(cb, 12, reg)
		cb.Emit32(arm64LDR_imm(x86ARM64Scratch2, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64LDRH_reg(x86ARM64Scratch2, x86ARM64Scratch2, x86ARM64Scratch))
		} else {
			cb.Emit32(arm64LDR_W_reg(x86ARM64Scratch2, x86ARM64Scratch2, x86ARM64Scratch))
		}
		// Reload the backing base because W11 now holds the old memory value.
		cb.Emit32(arm64LDR_imm(13, x86ARM64RegCtx, x86CtxOffMemPtr/8))
		if width == 2 {
			cb.Emit32(arm64STRH_reg(12, 13, x86ARM64Scratch))
			x86ARM64EmitLoadReg(cb, 12, reg)
			x86ARM64EmitWordInsert(cb, 12, x86ARM64Scratch2)
			x86ARM64EmitStoreReg(cb, reg, 12)
		} else {
			cb.Emit32(arm64STR_W_reg(12, 13, x86ARM64Scratch))
			x86ARM64EmitStoreReg(cb, reg, x86ARM64Scratch2)
		}
		x86ARM64EmitSMCStoreCheck(cb, width, ji.opcodePC+uint32(ji.length), retired+1, bails)
		return true
	case op == 0x87 && ji.hasModRM && ji.modrm>>6 == 3: // XCHG r/m32,r32
		reg, rm := (ji.modrm>>3)&7, ji.modrm&7
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch, reg)
		x86ARM64EmitLoadReg(cb, x86ARM64Scratch2, rm)
		if ji.prefixes&x86PrefOpSize != 0 {
			cb.Emit32(arm64ORR_W(12, x86ARM64Scratch, 31))
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch, x86ARM64Scratch2)
			x86ARM64EmitWordInsert(cb, x86ARM64Scratch2, 12)
			x86ARM64EmitStoreReg(cb, reg, x86ARM64Scratch)
			x86ARM64EmitStoreReg(cb, rm, x86ARM64Scratch2)
			return true
		}
		x86ARM64EmitStoreReg(cb, reg, x86ARM64Scratch2)
		x86ARM64EmitStoreReg(cb, rm, x86ARM64Scratch)
		return true
	}
	return false
}

// x86CompileBlockForCPU emits a complete ARM64 function. It intentionally
// accepts a maximal contiguous direct prefix: the dispatcher executes the
// next instruction through the normal helper boundary rather than risking a
// mixed native/interpreter instruction.
func x86CompileBlockForCPU(cpu *CPU_X86, instrs []X86JITInstr, startPC uint32, execMem *ExecMem) (*JITBlock, error) {
	if cpu == nil || execMem == nil || len(instrs) == 0 {
		return nil, fmt.Errorf("no instructions compiled")
	}
	cb := NewCodeBuffer(len(instrs)*96 + 128)
	// X9 = ctx->JITRegsPtr.
	cb.Emit32(arm64LDR_imm(x86ARM64RegRegs, x86ARM64RegCtx, x86CtxOffJITRegsPtr/8))
	count := 0
	pc := startPC
	helperExit := false
	terminalExit := false
	var bails []x86ARM64DeferredBail
	var chainExits []x86ARM64ChainExit
	cyclePrefix := x86JITCyclePrefix(instrs)
	tickPrefix := x86JITTickPrefix(instrs)
	for _, ji := range instrs {
		cycles, ticks := uint32(cyclePrefix[count]), uint32(tickPrefix[count])
		if ji.opcode >= 0xD8 && ji.opcode <= 0xDF {
			if x86ARM64EmitFNOP(cb, ji, cpu.memory) {
				count++
				pc = ji.opcodePC + uint32(ji.length)
				continue
			}
			if x86ARM64EmitFFREE(cb, ji, cpu.memory) {
				count++
				pc = ji.opcodePC + uint32(ji.length)
				continue
			}
			if x86ARM64EmitFXCH(cb, ji, cpu.memory, count) {
				count++
				pc = ji.opcodePC + uint32(ji.length)
				continue
			}
			if x86ARM64EmitFSTPSTi(cb, ji, cpu.memory, count) {
				count++
				pc = ji.opcodePC + uint32(ji.length)
				continue
			}
			if x86ARM64EmitFNCLEX(cb, ji, cpu.memory) {
				count++
				pc = ji.opcodePC + uint32(ji.length)
				continue
			}
			if x86ARM64EmitFNINIT(cb, ji, cpu.memory) {
				count++
				pc = ji.opcodePC + uint32(ji.length)
				continue
			}
			if x86ARM64EmitFNSTSWAX(cb, ji, cpu.memory) {
				count++
				pc = ji.opcodePC + uint32(ji.length)
				continue
			}
			if x86ARM64EmitFCHSFABS(cb, ji, cpu.memory, count) {
				count++
				pc = ji.opcodePC + uint32(ji.length)
				continue
			}
			if x86ARM64EmitBinaryST0STi(cb, ji, cpu.memory, count) {
				count++
				pc = ji.opcodePC + uint32(ji.length)
				continue
			}
			p, ok := x86FPUHelperPayloadFor(ji, cpu.memory, cpu.CS)
			if !ok || !x86ARM64EmitFPUHelperExit(cb, p, count) {
				break
			}
			helperExit = true
			break
		}
		if x86ARM64EmitDirectJMP(cb, ji, cpu.memory, count, cycles, ticks, &chainExits) {
			count++
			pc = ji.opcodePC + uint32(ji.length)
			terminalExit = true
			break
		}
		if x86ARM64EmitDirectCALL(cb, ji, cpu.memory, count, cycles, ticks, &bails, &chainExits) {
			count++
			pc = ji.opcodePC + uint32(ji.length)
			terminalExit = true
			break
		}
		if x86ARM64EmitDirectRET(cb, ji, cpu.memory, count, &bails) {
			count++
			pc = ji.opcodePC + uint32(ji.length)
			terminalExit = true
			break
		}
		if x86ARM64EmitDirectJcc(cb, ji, cpu.memory, count, cycles, ticks, &chainExits) {
			count++
			pc = ji.opcodePC + uint32(ji.length)
			terminalExit = true
			break
		}
		if x86ARM64EmitDirectLoop(cb, ji, cpu.memory, count, cycles, ticks, &chainExits) {
			count++
			pc = ji.opcodePC + uint32(ji.length)
			terminalExit = true
			break
		}
		if !x86ARM64EmitInstruction(cb, ji, cpu.memory, count, &bails) {
			break
		}
		count++
		pc = ji.opcodePC + uint32(ji.length)
		// Dynamic string cycles are derived from the architectural pointer delta
		// at native return. Keep every REP form at its own ARM64 block boundary
		// so a following string instruction cannot overwrite that observation.
		if ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0 {
			break
		}
	}
	if count == 0 && !helperExit {
		return nil, fmt.Errorf("no instructions compiled")
	}
	if helperExit || terminalExit {
		x86ARM64EmitDeferredBails(cb, bails)
		compiled := append([]X86JITInstr(nil), instrs[:count]...)
		code := cb.Bytes()
		addr, err := execMem.Write(code)
		if err != nil {
			return nil, err
		}
		block := &JITBlock{startPC: uint64(startPC), endPC: uint64(pc), instrCount: count,
			x86CyclePrefix: x86JITCyclePrefix(compiled), x86TickPrefix: x86JITTickPrefix(compiled), x86DynamicCycles: x86JITDynamicCycles(compiled),
			execAddr: addr, execSize: len(code)}
		x86ARM64InstallChainSlots(block, chainExits, len(bails) == 0 && !helperExit && len(block.x86DynamicCycles) == 0)
		return block, nil
	}
	// A non-terminating scanner prefix has a statically known fall-through,
	// just like a direct JMP.  Folding it through the chain exit is essential:
	// otherwise a chained predecessor's retired count would be lost when this
	// block returns normally to the dispatcher.
	x86ARM64EmitChainOrReturn(cb, pc, count, uint32(cyclePrefix[count-1]), uint32(tickPrefix[count-1]), &chainExits)
	x86ARM64EmitDeferredBails(cb, bails)
	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, err
	}
	compiled := append([]X86JITInstr(nil), instrs[:count]...)
	block := &JITBlock{startPC: uint64(startPC), endPC: uint64(pc), instrCount: count,
		x86CyclePrefix: x86JITCyclePrefix(compiled), x86TickPrefix: x86JITTickPrefix(compiled), x86DynamicCycles: x86JITDynamicCycles(compiled),
		execAddr: addr, execSize: len(code)}
	x86ARM64InstallChainSlots(block, chainExits, len(bails) == 0 && len(block.x86DynamicCycles) == 0)
	return block, nil
}

// x86CompileBlock is the CPU-independent test entry point used to prove the
// ARM64 emitted ABI. Production compilation always takes the CPU-owned input
// path above so memory and context pointers come from one CPU snapshot.
func x86CompileBlock(instrs []X86JITInstr, startPC uint32, execMem *ExecMem, memory []byte) (*JITBlock, error) {
	if len(memory) == 0 {
		return nil, fmt.Errorf("no instructions compiled")
	}
	stub := &CPU_X86{memory: memory}
	return x86CompileBlockForCPU(stub, instrs, startPC, execMem)
}

// x86CompileRegionForCPU lowers an acyclic, direct-only region into one ARM64
// entry point.  Back-edges deliberately remain basic-block dispatches: their
// variable retirement and cycle counts need a separate loop-accounting ABI,
// whereas an acyclic region retires each flattened instruction exactly once.
// This preserves the outer interrupt and bounded-execution observation points
// while still removing the Go round trips from hot straight-line traces.
func x86CompileRegionForCPU(cpu *CPU_X86, region *x86Region, execMem *ExecMem) (*JITBlock, error) {
	if cpu == nil || execMem == nil || region == nil || len(region.blocks) < 2 || len(region.backEdges) != 0 {
		return nil, fmt.Errorf("region not ARM64-admissible")
	}
	cb := NewCodeBuffer(1024)
	cb.Emit32(arm64LDR_imm(x86ARM64RegRegs, x86ARM64RegCtx, x86CtxOffJITRegsPtr/8))
	labels := make([]int, len(region.blocks))
	blockIndex := make(map[uint32]int, len(region.blocks))
	for i, pc := range region.blockPCs {
		blockIndex[pc] = i
	}
	type forwardBranch struct{ off, target int }
	var forward []forwardBranch
	var bails []x86ARM64DeferredBail
	var chainExits []x86ARM64ChainExit
	var all []X86JITInstr
	total := 0
	var staticCycles, staticTicks uint32

	for bi, instrs := range region.blocks {
		if len(instrs) == 0 {
			return nil, fmt.Errorf("empty region block")
		}
		labels[bi] = cb.Len()
		for ii, ji := range instrs {
			nextCycles := staticCycles + uint32(x86JITCycleCost(ji))
			nextTicks := staticTicks + uint32(max(x86JITCycleCost(ji), 1))
			last := ii == len(instrs)-1
			if last && x86IsBlockTerminator(ji.opcode) {
				if target, known := x86ResolveTerminatorTarget(&ji, cpu.memory, region.blockPCs[bi]); known {
					if targetIdx, internal := blockIndex[target]; internal {
						if targetIdx <= bi { // variable loop accounting stays at the dispatch boundary.
							return nil, fmt.Errorf("region back-edge")
						}
						off := cb.Len()
						cb.Emit32(arm64B(0))
						forward = append(forward, forwardBranch{off: off, target: targetIdx})
						total++
						staticCycles, staticTicks = nextCycles, nextTicks
						all = append(all, ji)
						continue
					}
				}
				if !x86ARM64EmitDirectJMP(cb, ji, cpu.memory, total, nextCycles, nextTicks, &chainExits) {
					return nil, fmt.Errorf("unsupported ARM64 region terminator")
				}
				total++
				staticCycles, staticTicks = nextCycles, nextTicks
				all = append(all, ji)
				// A terminal external branch returns from the native function.
				if bi != len(region.blocks)-1 {
					return nil, fmt.Errorf("external region branch before final block")
				}
				continue
			}
			// The current region path is deliberately direct-only: helper exits
			// replay one instruction and would split a flattened retirement count.
			if ji.opcode >= 0xD8 && ji.opcode <= 0xDF || !x86ARM64EmitInstruction(cb, ji, cpu.memory, total, &bails) {
				return nil, fmt.Errorf("unsupported ARM64 region instruction")
			}
			total++
			staticCycles, staticTicks = nextCycles, nextTicks
			all = append(all, ji)
		}
	}
	for _, fix := range forward {
		cb.PatchUint32(fix.off, arm64B(int32(labels[fix.target]-fix.off)))
	}
	last := region.blocks[len(region.blocks)-1][len(region.blocks[len(region.blocks)-1])-1]
	if !x86IsBlockTerminator(last.opcode) {
		x86ARM64EmitChainOrReturn(cb, last.opcodePC+uint32(last.length), total, staticCycles, staticTicks, &chainExits)
	}
	x86ARM64EmitDeferredBails(cb, bails)
	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, err
	}
	covered := make([][2]uint64, 0, len(region.blocks))
	for i, block := range region.blocks {
		end := block[len(block)-1].opcodePC + uint32(block[len(block)-1].length)
		covered = append(covered, [2]uint64{uint64(region.blockPCs[i]), uint64(end)})
	}
	block := &JITBlock{
		startPC: uint64(region.entryPC), endPC: uint64(last.opcodePC + uint32(last.length)), instrCount: total,
		x86CyclePrefix: x86JITCyclePrefix(all), x86TickPrefix: x86JITTickPrefix(all), x86DynamicCycles: x86JITDynamicCycles(all),
		execAddr: addr, execSize: len(code), tier: 2, coveredRanges: covered,
	}
	x86ARM64InstallChainSlots(block, chainExits, len(bails) == 0 && len(block.x86DynamicCycles) == 0)
	return block, nil
}

func x86CompileRegion(region *x86Region, execMem *ExecMem, memory []byte) (*JITBlock, error) {
	if len(memory) == 0 {
		return nil, fmt.Errorf("region has no memory")
	}
	return x86CompileRegionForCPU(&CPU_X86{memory: memory}, region, execMem)
}
