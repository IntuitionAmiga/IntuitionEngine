// jit_emit_amd64.go - x86-64 native code emitter for IE64 JIT compiler

//go:build amd64 && linux

package main

// ===========================================================================
// x86-64 Register Mapping
// ===========================================================================
//
// x86-64  IE64  Purpose
// ------  ----  -------
// RDI     --    &cpu.regs[0] (register file base, loaded from JITContext in prologue)
// RSI     --    &cpu.memory[0] (memory base)
// R8      --    IO_REGION_START constant
// R9      --    &ioPageBitmap[0]
// RAX     --    Scratch
// RCX     --    Scratch / shift count (CL)
// RDX     --    Scratch
// R10     --    Scratch
// R11     --    Scratch
// RBX     R1    Mapped IE64 R1 (callee-saved)
// RBP     R2    Mapped IE64 R2 (callee-saved)
// R12     R3    Mapped IE64 R3 (callee-saved)
// R13     R4    Mapped IE64 R4 (callee-saved)
// R14     R31   IE64 SP (callee-saved, always resident)
// R15     --    IE64 PC / return channel (callee-saved)

const (
	amd64RAX = 0
	amd64RCX = 1
	amd64RDX = 2
	amd64RBX = 3
	amd64RSP = 4
	amd64RBP = 5
	amd64RSI = 6
	amd64RDI = 7
	amd64R8  = 8
	amd64R9  = 9
	amd64R10 = 10
	amd64R11 = 11
	amd64R12 = 12
	amd64R13 = 13
	amd64R14 = 14
	amd64R15 = 15
)

// Dedicated register assignments
const (
	amd64RegBase     = amd64RDI // &cpu.regs[0]
	amd64RegMemBase  = amd64RSI // &cpu.memory[0]
	amd64RegIOStart  = amd64R8  // IO_REGION_START
	amd64RegIOBitmap = amd64R9  // &ioPageBitmap[0]
	amd64RegScratch1 = amd64RAX // general scratch
	amd64RegScratch2 = amd64RDX // scratch (also DIV high)
	amd64RegScratch3 = amd64RCX // scratch (also shift count CL)
	amd64RegScratch4 = amd64R10 // scratch
	amd64RegScratch5 = amd64R11 // scratch
	amd64RegIE64R1   = amd64RBX // IE64 R1 (callee-saved)
	amd64RegIE64R2   = amd64RBP // IE64 R2 (callee-saved)
	amd64RegIE64R3   = amd64R12 // IE64 R3 (callee-saved)
	amd64RegIE64R4   = amd64R13 // IE64 R4 (callee-saved)
	amd64RegIE64SP   = amd64R14 // IE64 R31 (callee-saved)
	amd64RegIE64PC   = amd64R15 // IE64 PC / return channel (callee-saved)

	// IE64 mapped register range
	amd64IE64FirstMapped = 1
	amd64IE64LastMapped  = 4

	// Stack frame: 6 callee-saved pushes (48 bytes) + SUB RSP,24 = 72 bytes
	// + 8 bytes return address = 80 bytes (16-byte aligned)
	amd64FrameSize    = 24
	amd64OffCtxPtr    = 0  // [RSP+0] = saved JITContext pointer
	amd64OffFPUPtr    = 8  // [RSP+8] = FPU pointer (if hasFPU)
	amd64OffLoopCount = 16 // [RSP+16] = loop counter (if hasBackwardBranch)
)

// Backward branch budget (must fit in imm32)
const jitBudget = 4095

// ie64ToAMD64Reg maps an IE64 register index (0-31) to an x86-64 register.
// Returns the x86-64 register number and whether it's "mapped" (resident in
// a callee-saved register) vs "spilled" (in the register file in memory).
func ie64ToAMD64Reg(ie64Reg byte) (amd64Reg byte, mapped bool) {
	switch ie64Reg {
	case 1:
		return amd64RegIE64R1, true // RBX
	case 2:
		return amd64RegIE64R2, true // RBP
	case 3:
		return amd64RegIE64R3, true // R12
	case 4:
		return amd64RegIE64R4, true // R13
	case 31:
		return amd64RegIE64SP, true // R14
	}
	return 0, false // R0 (always zero) or spilled R5-R30
}

// ===========================================================================
// x86-64 Instruction Encoding Helpers
// ===========================================================================

// regBits returns the low 3 bits of a register number.
func regBits(reg byte) byte {
	return reg & 0x07
}

// isExtReg returns true for registers R8-R15 (need REX.R or REX.B).
func isExtReg(reg byte) bool {
	return reg >= 8
}

// rexByte builds a REX prefix. W=1 for 64-bit, R for reg extension,
// X for SIB index extension, B for rm/base/opcode extension.
func rexByte(w, r, x, b bool) byte {
	v := byte(0x40)
	if w {
		v |= 0x08
	}
	if r {
		v |= 0x04
	}
	if x {
		v |= 0x02
	}
	if b {
		v |= 0x01
	}
	return v
}

// modRM builds a ModR/M byte.
func modRM(mod, reg, rm byte) byte {
	return (mod << 6) | ((reg & 0x07) << 3) | (rm & 0x07)
}

// sibByte builds a SIB byte.
func sibByte(scale, index, base byte) byte {
	return (scale << 6) | ((index & 0x07) << 3) | (base & 0x07)
}

// ===========================================================================
// Emit Helpers — Register-Register Operations
// ===========================================================================

// emitREX emits a REX prefix for a reg,rm operation.
// w64=true for 64-bit operand size.
func emitREX(cb *CodeBuffer, w64 bool, reg, rm byte) {
	r := isExtReg(reg)
	b := isExtReg(rm)
	if w64 || r || b {
		cb.EmitBytes(rexByte(w64, r, false, b))
	}
}

// emitREX_SIB emits a REX prefix for an operation using SIB (reg, index, base).
func emitREX_SIB(cb *CodeBuffer, w64 bool, reg, index, base byte) {
	r := isExtReg(reg)
	x := isExtReg(index)
	b := isExtReg(base)
	if w64 || r || x || b {
		cb.EmitBytes(rexByte(w64, r, x, b))
	}
}

// amd64MOV_reg_reg emits MOV dst, src (64-bit register to register).
func amd64MOV_reg_reg(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, true, src, dst)
	cb.EmitBytes(0x89, modRM(3, src, dst))
}

// amd64MOV_reg_reg32 emits MOV dst32, src32 (32-bit, zero-extends upper 32).
func amd64MOV_reg_reg32(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, false, src, dst)
	cb.EmitBytes(0x89, modRM(3, src, dst))
}

// amd64MOV_reg_imm64 emits MOV dst, imm64 (64-bit immediate).
func amd64MOV_reg_imm64(cb *CodeBuffer, dst byte, val uint64) {
	cb.EmitBytes(rexByte(true, false, false, isExtReg(dst)))
	cb.EmitBytes(0xB8 + regBits(dst))
	cb.Emit64(val)
}

// amd64MOV_reg_imm32 emits MOV dst32, imm32 (32-bit, zero-extends).
func amd64MOV_reg_imm32(cb *CodeBuffer, dst byte, val uint32) {
	if isExtReg(dst) {
		cb.EmitBytes(rexByte(false, false, false, true))
	}
	cb.EmitBytes(0xB8 + regBits(dst))
	cb.Emit32(val)
}

// amd64XOR_reg_reg emits XOR dst, src (64-bit). Used to zero a register.
func amd64XOR_reg_reg(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, true, src, dst)
	cb.EmitBytes(0x31, modRM(3, src, dst))
}

// amd64XOR_reg_reg32 emits XOR dst32, src32 (32-bit). Used to zero a register.
func amd64XOR_reg_reg32(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, false, src, dst)
	cb.EmitBytes(0x31, modRM(3, src, dst))
}

// ===========================================================================
// Emit Helpers — Memory Operations
// ===========================================================================

// emitMemOp emits an instruction with [base + disp32] addressing.
// opcode is the main opcode byte, reg is the /r field.
// Handles RBP/R13 (always needs disp) and RSP/R12 (needs SIB) edge cases.
func emitMemOp(cb *CodeBuffer, w64 bool, opcode byte, reg, base byte, disp int32) {
	emitREX(cb, w64, reg, base)
	cb.EmitBytes(opcode)

	baseBits := regBits(base)

	// RSP/R12 as base requires SIB byte
	needsSIB := baseBits == 4 // RSP=4, R12=4 in low bits

	if disp == 0 && baseBits != 5 { // RBP/R13 (low bits = 5) always need disp
		if needsSIB {
			cb.EmitBytes(modRM(0, reg, 4), sibByte(0, 4, base)) // SIB with no index
		} else {
			cb.EmitBytes(modRM(0, reg, base))
		}
	} else if disp >= -128 && disp <= 127 {
		if needsSIB {
			cb.EmitBytes(modRM(1, reg, 4), sibByte(0, 4, base), byte(int8(disp)))
		} else {
			cb.EmitBytes(modRM(1, reg, base), byte(int8(disp)))
		}
	} else {
		if needsSIB {
			cb.EmitBytes(modRM(2, reg, 4), sibByte(0, 4, base))
		} else {
			cb.EmitBytes(modRM(2, reg, base))
		}
		cb.Emit32(uint32(disp))
	}
}

// amd64MOV_reg_mem emits MOV reg, [base + disp32] (64-bit load).
func amd64MOV_reg_mem(cb *CodeBuffer, dst, base byte, disp int32) {
	emitMemOp(cb, true, 0x8B, dst, base, disp)
}

// amd64MOV_mem_reg emits MOV [base + disp32], reg (64-bit store).
func amd64MOV_mem_reg(cb *CodeBuffer, base byte, disp int32, src byte) {
	emitMemOp(cb, true, 0x89, src, base, disp)
}

// amd64MOV_reg_mem32 emits MOV reg32, [base + disp32] (32-bit load, zero-extends).
func amd64MOV_reg_mem32(cb *CodeBuffer, dst, base byte, disp int32) {
	emitMemOp(cb, false, 0x8B, dst, base, disp)
}

// amd64MOV_mem_reg32 emits MOV [base + disp32], reg32 (32-bit store).
func amd64MOV_mem_reg32(cb *CodeBuffer, base byte, disp int32, src byte) {
	emitMemOp(cb, false, 0x89, src, base, disp)
}

// amd64MOV_mem_imm32 emits MOV DWORD [base + disp32], imm32.
func amd64MOV_mem_imm32(cb *CodeBuffer, base byte, disp int32, val uint32) {
	emitMemOp(cb, false, 0xC7, 0, base, disp) // /0 for MOV imm
	cb.Emit32(val)
}

// ===========================================================================
// Emit Helpers — Index-Scaled Memory Operations [base + index*1]
// ===========================================================================

// emitMemOpSIB emits an instruction with [base + index*scale] addressing.
func emitMemOpSIB(cb *CodeBuffer, w64 bool, opcode byte, reg, base, index byte, scale byte) {
	emitREX_SIB(cb, w64, reg, index, base)
	cb.EmitBytes(opcode, modRM(0, reg, 4), sibByte(scale, index, base))
}

// amd64MOV_reg_memSIB emits MOV reg, [base + index*1] (64-bit load).
func amd64MOV_reg_memSIB(cb *CodeBuffer, dst, base, index byte) {
	emitMemOpSIB(cb, true, 0x8B, dst, base, index, 0)
}

// amd64MOV_memSIB_reg emits MOV [base + index*1], reg (64-bit store).
func amd64MOV_memSIB_reg(cb *CodeBuffer, base, index, src byte) {
	emitMemOpSIB(cb, true, 0x89, src, base, index, 0)
}

// amd64MOV_reg_memSIB32 emits MOV reg32, [base + index*1] (32-bit load, zero-extends).
func amd64MOV_reg_memSIB32(cb *CodeBuffer, dst, base, index byte) {
	emitMemOpSIB(cb, false, 0x8B, dst, base, index, 0)
}

// amd64MOV_memSIB_reg32 emits MOV [base + index*1], reg32 (32-bit store).
func amd64MOV_memSIB_reg32(cb *CodeBuffer, base, index, src byte) {
	emitMemOpSIB(cb, false, 0x89, src, base, index, 0)
}

// ===========================================================================
// Emit Helpers — MOVZX (zero-extend byte/word)
// ===========================================================================

// amd64MOVZX_B emits MOVZX dst, src8 (zero-extend byte to 64-bit via 32-bit form).
func amd64MOVZX_B(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, false, dst, src)
	cb.EmitBytes(0x0F, 0xB6, modRM(3, dst, src))
}

// amd64MOVZX_W emits MOVZX dst, src16 (zero-extend word to 64-bit via 32-bit form).
func amd64MOVZX_W(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, false, dst, src)
	cb.EmitBytes(0x0F, 0xB7, modRM(3, dst, src))
}

// amd64MOVSXD emits MOVSXD dst64, src32 (sign-extend dword to qword).
func amd64MOVSXD(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, true, dst, src)
	cb.EmitBytes(0x63, modRM(3, dst, src))
}

// ===========================================================================
// Emit Helpers — ALU Operations (reg, reg)
// ===========================================================================

// amd64ALU_reg_reg emits an ALU op (opcode) dst, src (64-bit).
// opcode: ADD=0x01, OR=0x09, AND=0x21, SUB=0x29, XOR=0x31, CMP=0x39
func amd64ALU_reg_reg(cb *CodeBuffer, opcode byte, dst, src byte) {
	emitREX(cb, true, src, dst)
	cb.EmitBytes(opcode, modRM(3, src, dst))
}

// amd64ALU_reg_reg32 emits an ALU op (opcode) dst32, src32 (32-bit).
func amd64ALU_reg_reg32(cb *CodeBuffer, opcode byte, dst, src byte) {
	emitREX(cb, false, src, dst)
	cb.EmitBytes(opcode, modRM(3, src, dst))
}

// amd64ALU_reg_imm32 emits an ALU op dst, imm32 (64-bit, sign-extended).
// aluOp: 0=ADD, 1=OR, 4=AND, 5=SUB, 6=XOR, 7=CMP
func amd64ALU_reg_imm32(cb *CodeBuffer, aluOp byte, dst byte, imm32 int32) {
	emitREX(cb, true, 0, dst)
	if imm32 >= -128 && imm32 <= 127 {
		cb.EmitBytes(0x83, modRM(3, aluOp, dst), byte(int8(imm32)))
	} else {
		cb.EmitBytes(0x81, modRM(3, aluOp, dst))
		cb.Emit32(uint32(imm32))
	}
}

// amd64ALU_reg_imm32_32bit emits an ALU op dst32, imm32 (32-bit).
func amd64ALU_reg_imm32_32bit(cb *CodeBuffer, aluOp byte, dst byte, imm32 int32) {
	emitREX(cb, false, 0, dst)
	if imm32 >= -128 && imm32 <= 127 {
		cb.EmitBytes(0x83, modRM(3, aluOp, dst), byte(int8(imm32)))
	} else {
		cb.EmitBytes(0x81, modRM(3, aluOp, dst))
		cb.Emit32(uint32(imm32))
	}
}

// amd64NEG emits NEG dst (64-bit).
func amd64NEG(cb *CodeBuffer, dst byte) {
	emitREX(cb, true, 0, dst)
	cb.EmitBytes(0xF7, modRM(3, 3, dst)) // /3 = NEG
}

// amd64NEG32 emits NEG dst32 (32-bit).
func amd64NEG32(cb *CodeBuffer, dst byte) {
	emitREX(cb, false, 0, dst)
	cb.EmitBytes(0xF7, modRM(3, 3, dst))
}

// amd64NOT emits NOT dst (64-bit).
func amd64NOT(cb *CodeBuffer, dst byte) {
	emitREX(cb, true, 0, dst)
	cb.EmitBytes(0xF7, modRM(3, 2, dst)) // /2 = NOT
}

// ===========================================================================
// Emit Helpers — Shifts
// ===========================================================================

// amd64SHL_CL emits SHL dst, CL (64-bit).
func amd64SHL_CL(cb *CodeBuffer, dst byte) {
	emitREX(cb, true, 0, dst)
	cb.EmitBytes(0xD3, modRM(3, 4, dst)) // /4 = SHL
}

// amd64SHR_CL emits SHR dst, CL (64-bit).
func amd64SHR_CL(cb *CodeBuffer, dst byte) {
	emitREX(cb, true, 0, dst)
	cb.EmitBytes(0xD3, modRM(3, 5, dst)) // /5 = SHR
}

// amd64SAR_CL emits SAR dst, CL (64-bit).
func amd64SAR_CL(cb *CodeBuffer, dst byte) {
	emitREX(cb, true, 0, dst)
	cb.EmitBytes(0xD3, modRM(3, 7, dst)) // /7 = SAR
}

// amd64SHL_imm emits SHL dst, imm8 (64-bit).
func amd64SHL_imm(cb *CodeBuffer, dst byte, imm8 byte) {
	emitREX(cb, true, 0, dst)
	cb.EmitBytes(0xC1, modRM(3, 4, dst), imm8)
}

// amd64SHR_imm emits SHR dst, imm8 (64-bit).
func amd64SHR_imm(cb *CodeBuffer, dst byte, imm8 byte) {
	emitREX(cb, true, 0, dst)
	cb.EmitBytes(0xC1, modRM(3, 5, dst), imm8)
}

// amd64SHL_CL32 emits SHL dst32, CL (32-bit).
func amd64SHL_CL32(cb *CodeBuffer, dst byte) {
	emitREX(cb, false, 0, dst)
	cb.EmitBytes(0xD3, modRM(3, 4, dst))
}

// amd64SHR_CL32 emits SHR dst32, CL (32-bit).
func amd64SHR_CL32(cb *CodeBuffer, dst byte) {
	emitREX(cb, false, 0, dst)
	cb.EmitBytes(0xD3, modRM(3, 5, dst))
}

// amd64SAR_CL32 emits SAR dst32, CL (32-bit).
func amd64SAR_CL32(cb *CodeBuffer, dst byte) {
	emitREX(cb, false, 0, dst)
	cb.EmitBytes(0xD3, modRM(3, 7, dst))
}

// ===========================================================================
// Emit Helpers — PUSH / POP / RET / NOP
// ===========================================================================

// amd64PUSH emits PUSH reg (64-bit).
func amd64PUSH(cb *CodeBuffer, reg byte) {
	if isExtReg(reg) {
		cb.EmitBytes(rexByte(false, false, false, true))
	}
	cb.EmitBytes(0x50 + regBits(reg))
}

// amd64POP emits POP reg (64-bit).
func amd64POP(cb *CodeBuffer, reg byte) {
	if isExtReg(reg) {
		cb.EmitBytes(rexByte(false, false, false, true))
	}
	cb.EmitBytes(0x58 + regBits(reg))
}

// amd64RET emits RET.
func amd64RET(cb *CodeBuffer) {
	cb.EmitBytes(0xC3)
}

// amd64NOP emits NOP.
func amd64NOP(cb *CodeBuffer) {
	cb.EmitBytes(0x90)
}

// ===========================================================================
// Emit Helpers — Jumps
// ===========================================================================

// x86-64 condition codes for Jcc
const (
	amd64CondO  = 0x0 // overflow
	amd64CondNO = 0x1
	amd64CondB  = 0x2 // unsigned below (CF=1)
	amd64CondAE = 0x3 // unsigned above-or-equal
	amd64CondE  = 0x4 // equal (ZF=1)
	amd64CondNE = 0x5
	amd64CondBE = 0x6 // unsigned below-or-equal
	amd64CondA  = 0x7 // unsigned above
	amd64CondL  = 0xC // signed less
	amd64CondGE = 0xD // signed greater-or-equal
	amd64CondLE = 0xE // signed less-or-equal
	amd64CondG  = 0xF // signed greater
)

// amd64Jcc_rel32 emits Jcc near (rel32). Returns offset of the rel32 field for patching.
func amd64Jcc_rel32(cb *CodeBuffer, cond byte) int {
	cb.EmitBytes(0x0F, 0x80+cond)
	off := cb.Len()
	cb.Emit32(0) // placeholder
	return off
}

// amd64JMP_rel32 emits JMP near (rel32). Returns offset of the rel32 field.
func amd64JMP_rel32(cb *CodeBuffer) int {
	cb.EmitBytes(0xE9)
	off := cb.Len()
	cb.Emit32(0) // placeholder
	return off
}

// patchRel32 patches a rel32 at the given offset to jump to target.
// The rel32 is relative to the end of the instruction (offset + 4).
func patchRel32(cb *CodeBuffer, offset int, target int) {
	rel := int32(target - (offset + 4))
	cb.PatchUint32(offset, uint32(rel))
}

// ===========================================================================
// Emitter Helpers — IE64-Specific
// ===========================================================================

// emitLoadImm64AMD64 loads a 64-bit immediate into the given x86-64 register.
func emitLoadImm64AMD64(cb *CodeBuffer, dst byte, val uint64) {
	if val == 0 {
		amd64XOR_reg_reg32(cb, dst, dst)
	} else if val <= 0xFFFFFFFF {
		amd64MOV_reg_imm32(cb, dst, uint32(val))
	} else {
		amd64MOV_reg_imm64(cb, dst, val)
	}
}

// emitLoadImm32AMD64 loads a 32-bit value into a register (zero-extended).
func emitLoadImm32AMD64(cb *CodeBuffer, dst byte, val uint32) {
	if val == 0 {
		amd64XOR_reg_reg32(cb, dst, dst)
	} else {
		amd64MOV_reg_imm32(cb, dst, val)
	}
}

// emitLoadSpilledRegAMD64 loads an IE64 spilled register (R5-R30) from [RDI + ie64Reg*8].
func emitLoadSpilledRegAMD64(cb *CodeBuffer, amd64Dst, ie64Reg byte) {
	amd64MOV_reg_mem(cb, amd64Dst, amd64RegBase, int32(ie64Reg)*8)
}

// emitStoreSpilledRegAMD64 stores to an IE64 spilled register [RDI + ie64Reg*8].
func emitStoreSpilledRegAMD64(cb *CodeBuffer, amd64Src, ie64Reg byte) {
	amd64MOV_mem_reg(cb, amd64RegBase, int32(ie64Reg)*8, amd64Src)
}

// resolveRegAMD64 ensures the IE64 register value is in an x86-64 register.
// For R0: zeros the scratch register (XOR). For mapped: returns directly.
// For spilled: loads into scratch and returns it.
func resolveRegAMD64(cb *CodeBuffer, ie64Reg byte, scratch byte) byte {
	if ie64Reg == 0 {
		amd64XOR_reg_reg32(cb, scratch, scratch)
		return scratch
	}
	amd64Reg, mapped := ie64ToAMD64Reg(ie64Reg)
	if mapped {
		return amd64Reg
	}
	emitLoadSpilledRegAMD64(cb, scratch, ie64Reg)
	return scratch
}

// emitSizeMaskAMD64 applies size masking to the result register.
func emitSizeMaskAMD64(cb *CodeBuffer, rd byte, size byte) {
	switch size {
	case IE64_SIZE_B:
		amd64MOVZX_B(cb, rd, rd)
	case IE64_SIZE_W:
		amd64MOVZX_W(cb, rd, rd)
	case IE64_SIZE_L:
		amd64MOV_reg_reg32(cb, rd, rd) // 32-bit write zero-extends
	case IE64_SIZE_Q:
		// no-op
	}
}

// ===========================================================================
// Packed PC + Count Helpers (Return Channel Contract)
// ===========================================================================

// emitPackedPCAndCount loads targetPC into R15 and packs instruction count.
// For backward-branch blocks: dynamic count via [RSP+16] loop counter.
// For normal blocks: static count packed into upper 32 bits of immediate.
func emitPackedPCAndCount(cb *CodeBuffer, targetPC uint64, staticCount uint32, br *blockRegs) {
	if br.hasBackwardBranch {
		emitLoadImm64AMD64(cb, amd64RegIE64PC, targetPC)
		emitDynamicCountAMD64(cb, staticCount)
	} else {
		emitLoadImm64AMD64(cb, amd64RegIE64PC, targetPC|(uint64(staticCount)<<32))
	}
}

// emitDynamicCountAMD64 packs [RSP+16] + staticCount into upper 32 bits of R15.
func emitDynamicCountAMD64(cb *CodeBuffer, staticCount uint32) {
	// MOV EAX, [RSP+16]
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(amd64OffLoopCount))
	// ADD EAX, staticCount
	if staticCount > 0 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(staticCount)) // 0 = ADD
	}
	// SHL RAX, 32
	amd64SHL_imm(cb, amd64RAX, 32)
	// OR R15, RAX
	amd64ALU_reg_reg(cb, 0x09, amd64RegIE64PC, amd64RAX) // 0x09 = OR
}

// ===========================================================================
// Block Prologue / Epilogue
// ===========================================================================

func emitPrologue(cb *CodeBuffer, blockPC uint32, br *blockRegs) {
	// Save callee-saved registers
	amd64PUSH(cb, amd64RBX)
	amd64PUSH(cb, amd64RBP)
	amd64PUSH(cb, amd64R12)
	amd64PUSH(cb, amd64R13)
	amd64PUSH(cb, amd64R14)
	amd64PUSH(cb, amd64R15)

	// Allocate stack frame
	amd64ALU_reg_imm32(cb, 5, amd64RSP, int32(amd64FrameSize)) // SUB RSP, 24

	// Save JITContext pointer
	amd64MOV_mem_reg(cb, amd64RSP, int32(amd64OffCtxPtr), amd64RDI) // [RSP+0] = RDI

	// Load base pointers from JITContext (RDI = *JITContext)
	amd64MOV_reg_mem(cb, amd64RegMemBase, amd64RDI, int32(jitCtxOffMemPtr))       // RSI = MemPtr
	amd64MOV_reg_mem32(cb, amd64RegIOStart, amd64RDI, int32(jitCtxOffIOStart))    // R8d = IOStart
	amd64MOV_reg_mem(cb, amd64RegIOBitmap, amd64RDI, int32(jitCtxOffIOBitmapPtr)) // R9 = IOBitmapPtr

	// Save RegsPtr temporarily in RAX
	amd64MOV_reg_mem(cb, amd64RAX, amd64RDI, int32(jitCtxOffRegsPtr)) // RAX = RegsPtr

	// Load FPU pointer if needed
	if br.hasFPU {
		amd64MOV_reg_mem(cb, amd64RCX, amd64RDI, int32(jitCtxOffFPUPtr))
		amd64MOV_mem_reg(cb, amd64RSP, int32(amd64OffFPUPtr), amd64RCX) // [RSP+8] = FPUPtr
	}

	// Zero loop counter if needed
	if br.hasBackwardBranch {
		amd64MOV_mem_imm32(cb, amd64RSP, int32(amd64OffLoopCount), 0) // [RSP+16] = 0
	}

	// Load mapped IE64 registers from register file (only those read by block)
	if br.read&(1<<1) != 0 {
		amd64MOV_reg_mem(cb, amd64RegIE64R1, amd64RAX, 1*8) // R1 -> RBX
	}
	if br.read&(1<<2) != 0 {
		amd64MOV_reg_mem(cb, amd64RegIE64R2, amd64RAX, 2*8) // R2 -> RBP
	}
	if br.read&(1<<3) != 0 {
		amd64MOV_reg_mem(cb, amd64RegIE64R3, amd64RAX, 3*8) // R3 -> R12
	}
	if br.read&(1<<4) != 0 {
		amd64MOV_reg_mem(cb, amd64RegIE64R4, amd64RAX, 4*8) // R4 -> R13
	}
	if br.read&(1<<31) != 0 {
		amd64MOV_reg_mem(cb, amd64RegIE64SP, amd64RAX, 31*8) // R31 -> R14
	}

	// Load block start PC into R15
	emitLoadImm64AMD64(cb, amd64RegIE64PC, uint64(blockPC))

	// Now overwrite RDI with RegsPtr (base for register file access)
	amd64MOV_reg_reg(cb, amd64RegBase, amd64RAX)
}

// emitEpilogue emits the block exit sequence.
//   - storeRegs: IE64 register bitmask — which registers to store back
//   - calleeSaved: IE64 register bitmask — which callee-saved pairs to restore (unused on amd64, we always restore all)
func emitEpilogue(cb *CodeBuffer, storeRegs uint32, _ uint32) {
	// Store mapped IE64 registers back to register file
	if storeRegs&(1<<1) != 0 {
		amd64MOV_mem_reg(cb, amd64RegBase, 1*8, amd64RegIE64R1)
	}
	if storeRegs&(1<<2) != 0 {
		amd64MOV_mem_reg(cb, amd64RegBase, 2*8, amd64RegIE64R2)
	}
	if storeRegs&(1<<3) != 0 {
		amd64MOV_mem_reg(cb, amd64RegBase, 3*8, amd64RegIE64R3)
	}
	if storeRegs&(1<<4) != 0 {
		amd64MOV_mem_reg(cb, amd64RegBase, 4*8, amd64RegIE64R4)
	}
	if storeRegs&(1<<31) != 0 {
		amd64MOV_mem_reg(cb, amd64RegBase, 31*8, amd64RegIE64SP)
	}

	// Store spilled registers that were written (R5-R30)
	// Spilled writes are already stored during instruction emission,
	// so nothing extra needed here.

	// Store packed PC+count to regs[0] (return channel)
	amd64MOV_mem_reg(cb, amd64RegBase, 0, amd64RegIE64PC)

	// Deallocate stack frame
	amd64ALU_reg_imm32(cb, 0, amd64RSP, int32(amd64FrameSize)) // ADD RSP, 24

	// Restore callee-saved registers (reverse order of push)
	amd64POP(cb, amd64R15)
	amd64POP(cb, amd64R14)
	amd64POP(cb, amd64R13)
	amd64POP(cb, amd64R12)
	amd64POP(cb, amd64RBP)
	amd64POP(cb, amd64RBX)

	amd64RET(cb)
}

// ===========================================================================
// Instruction Compilation
// ===========================================================================

// compileBlock compiles a scanned block of IE64 instructions to x86-64 machine code.
func compileBlock(instrs []JITInstr, startPC uint32, execMem *ExecMem) (*JITBlock, error) {
	cb := NewCodeBuffer(len(instrs) * 384) // x86-64 instructions are variable length

	br := analyzeBlockRegs(instrs)
	br.hasBackwardBranch = detectBackwardBranches(instrs, startPC)
	emitPrologue(cb, startPC, &br)

	instrOffsets := make([]int, len(instrs))
	writtenSoFar := uint32(0)

	for i := range instrs {
		instrOffsets[i] = cb.Len()
		ji := &instrs[i]
		emitInstruction(cb, ji, startPC, i == len(instrs)-1, &br, writtenSoFar, i, instrOffsets)
		writtenSoFar |= instrWrittenRegs(ji)
	}

	// Emit final epilogue if last instruction doesn't have its own
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

// emitInstruction emits x86-64 code for a single IE64 instruction.
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
	// Arithmetic
	// ======================================================================
	case OP_ADD:
		emitALU_AMD64(cb, ji, 0x01, 0) // ADD opcode=0x01, aluOp=0
	case OP_SUB:
		emitALU_AMD64(cb, ji, 0x29, 5) // SUB opcode=0x29, aluOp=5
	case OP_NEG:
		emitNEG_AMD64(cb, ji)

	// ======================================================================
	// Logic
	// ======================================================================
	case OP_MULU:
		emitMULU_AMD64(cb, ji)
	case OP_MULS:
		emitMULS_AMD64(cb, ji)
	case OP_DIVU:
		emitDIVU_AMD64(cb, ji)
	case OP_DIVS:
		emitDIVS_AMD64(cb, ji)
	case OP_MOD64:
		emitMOD_AMD64(cb, ji)

	// ======================================================================
	// Logic
	// ======================================================================
	case OP_AND64:
		emitALU_AMD64(cb, ji, 0x21, 4) // AND opcode=0x21, aluOp=4
	case OP_OR64:
		emitALU_AMD64(cb, ji, 0x09, 1) // OR opcode=0x09, aluOp=1
	case OP_EOR:
		emitALU_AMD64(cb, ji, 0x31, 6) // XOR opcode=0x31, aluOp=6
	case OP_NOT64:
		emitNOT_AMD64(cb, ji)

	// ======================================================================
	// Shifts
	// ======================================================================
	case OP_LSL:
		emitShift_AMD64(cb, ji, 4) // SHL /4
	case OP_LSR:
		emitShift_AMD64(cb, ji, 5) // SHR /5
	case OP_ASR:
		emitASR_AMD64(cb, ji)
	case OP_CLZ:
		emitCLZ_AMD64(cb, ji)

	// ======================================================================
	// Memory Access
	// ======================================================================
	case OP_LOAD:
		emitLOAD_AMD64(cb, ji, instrPC, br, writtenSoFar)
	case OP_STORE:
		emitSTORE_AMD64(cb, ji, instrPC, br, writtenSoFar)

	// ======================================================================
	// Branches
	// ======================================================================
	case OP_BRA:
		emitBRA_AMD64(cb, ji, instrPC, br, instrIdx, instrOffsets, blockStartPC)
	case OP_BEQ:
		emitBcc_AMD64(cb, ji, instrPC, amd64CondE, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BNE:
		emitBcc_AMD64(cb, ji, instrPC, amd64CondNE, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BLT:
		emitBcc_AMD64(cb, ji, instrPC, amd64CondL, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BGE:
		emitBcc_AMD64(cb, ji, instrPC, amd64CondGE, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BGT:
		emitBcc_AMD64(cb, ji, instrPC, amd64CondG, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BLE:
		emitBcc_AMD64(cb, ji, instrPC, amd64CondLE, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BHI:
		emitBcc_AMD64(cb, ji, instrPC, amd64CondA, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_BLS:
		emitBcc_AMD64(cb, ji, instrPC, amd64CondBE, br, writtenSoFar, blockStartPC, instrIdx, instrOffsets)
	case OP_JMP:
		emitJMP_AMD64(cb, ji, br, ji.pcOffset/IE64_INSTR_SIZE+1)

	// ======================================================================
	// Subroutine / Stack
	// ======================================================================
	case OP_JSR64:
		emitJSR_AMD64(cb, ji, instrPC, br)
	case OP_RTS64:
		emitRTS_AMD64(cb, br, ji.pcOffset/IE64_INSTR_SIZE+1)
	case OP_PUSH64:
		emitPUSH_AMD64(cb, ji)
	case OP_POP64:
		emitPOP_AMD64(cb, ji)
	case OP_JSR_IND:
		emitJSR_IND_AMD64(cb, ji, instrPC, br, ji.pcOffset/IE64_INSTR_SIZE+1)

	// ======================================================================
	// System
	// ======================================================================
	case OP_NOP64, OP_SEI64, OP_CLI64:
		amd64NOP(cb)

	case OP_HALT64:
		emitPackedPCAndCount(cb, uint64(instrPC), uint32(instrIdx+1), br)
		emitEpilogue(cb, br.written, br.used)

	case OP_RTI64:
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)

	case OP_WAIT64:
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)

	// ======================================================================
	// FPU — Category A (integer bitwise on FP registers)
	// ======================================================================
	case OP_FMOV:
		emitFMOV_AMD64(cb, ji)
	case OP_FABS:
		emitFABS_AMD64(cb, ji)
	case OP_FNEG:
		emitFNEG_AMD64(cb, ji)
	case OP_FMOVI:
		emitFMOVI_AMD64(cb, ji)
	case OP_FMOVO:
		emitFMOVO_AMD64(cb, ji)
	case OP_FMOVECR:
		emitFMOVECR_AMD64(cb, ji)
	case OP_FMOVSR:
		emitFMOVSR_AMD64(cb, ji)
	case OP_FMOVCR:
		emitFMOVCR_AMD64(cb, ji)
	case OP_FMOVSC:
		emitFMOVSC_AMD64(cb, ji)
	case OP_FMOVCC:
		emitFMOVCC_AMD64(cb, ji)

	// ======================================================================
	// FPU — Category B (native SSE instructions)
	// ======================================================================
	case OP_FADD:
		emitFPBinarySSE(cb, ji, 0x58) // ADDSS
	case OP_FSUB:
		emitFPBinarySSE(cb, ji, 0x5C) // SUBSS
	case OP_FMUL:
		emitFPBinarySSE(cb, ji, 0x59) // MULSS
	case OP_FDIV:
		emitFPBinarySSE(cb, ji, 0x5E) // DIVSS
	case OP_FSQRT:
		emitFSQRT_AMD64(cb, ji)
	case OP_FINT:
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)
	case OP_FCMP:
		emitFCMP_AMD64(cb, ji)
	case OP_FCVTIF:
		emitFCVTIF_AMD64(cb, ji)
	case OP_FCVTFI:
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)

	// ======================================================================
	// FPU — Memory
	// ======================================================================
	case OP_FLOAD:
		emitFLOAD_AMD64(cb, ji, instrPC, br, writtenSoFar)
	case OP_FSTORE:
		emitFSTORE_AMD64(cb, ji, instrPC, br, writtenSoFar)

	// ======================================================================
	// FPU — Category C (transcendentals, bail to interpreter)
	// ======================================================================
	case OP_FMOD, OP_FSIN, OP_FCOS, OP_FTAN, OP_FATAN, OP_FLOG, OP_FEXP, OP_FPOW:
		emitBailToInterpreter(cb, ji, instrPC, br, writtenSoFar)

	default:
		amd64NOP(cb)
	}

	_ = isLast
}

// ===========================================================================
// Data Movement Emitters
// ===========================================================================

func emitMOVE(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if !mapped {
		dstReg = amd64RAX // scratch for spilled destination
	}

	if ji.xbit == 1 {
		// MOVE rd, #imm32 — load immediate masked to size
		val := uint64(ji.imm32) & ie64SizeMask[ji.size]
		emitLoadImm64AMD64(cb, dstReg, val)
	} else {
		// MOVE rd, rs — register copy masked to size
		srcReg := resolveRegAMD64(cb, ji.rs, amd64RCX)
		if srcReg != dstReg {
			amd64MOV_reg_reg(cb, dstReg, srcReg)
		}
		if ji.size != IE64_SIZE_Q {
			emitSizeMaskAMD64(cb, dstReg, ji.size)
		}
	}

	if !mapped {
		emitStoreSpilledRegAMD64(cb, dstReg, ji.rd)
	}
}

func emitMOVT(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if !mapped {
		dstReg = amd64RAX
		emitLoadSpilledRegAMD64(cb, dstReg, ji.rd)
	}

	// Clear upper 32 bits, keep lower 32
	amd64MOV_reg_reg32(cb, dstReg, dstReg) // zero-extends

	// Load imm32 shifted left 32 into scratch, OR into dst
	amd64MOV_reg_imm64(cb, amd64RCX, uint64(ji.imm32)<<32)
	amd64ALU_reg_reg(cb, 0x09, dstReg, amd64RCX) // OR dst, RCX

	if !mapped {
		emitStoreSpilledRegAMD64(cb, dstReg, ji.rd)
	}
}

func emitMOVEQ(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if !mapped {
		dstReg = amd64RAX
	}

	// Load imm32 into 32-bit register, then sign-extend to 64
	amd64MOV_reg_imm32(cb, amd64RCX, ji.imm32)
	amd64MOVSXD(cb, dstReg, amd64RCX)

	if !mapped {
		emitStoreSpilledRegAMD64(cb, dstReg, ji.rd)
	}
}

func emitLEA(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RCX)
	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if !mapped {
		dstReg = amd64RAX
	}

	// LEA: int64(rs) + int64(int32(imm32))
	// Load sign-extended imm32 into scratch, add to rs
	amd64MOV_reg_imm32(cb, amd64RDX, ji.imm32)
	amd64MOVSXD(cb, amd64RDX, amd64RDX) // sign-extend to 64-bit
	if rsReg == dstReg {
		amd64ALU_reg_reg(cb, 0x01, dstReg, amd64RDX) // ADD dst, RDX
	} else {
		amd64MOV_reg_reg(cb, dstReg, rsReg)
		amd64ALU_reg_reg(cb, 0x01, dstReg, amd64RDX) // ADD dst, RDX
	}

	if !mapped {
		emitStoreSpilledRegAMD64(cb, dstReg, ji.rd)
	}
}

// ===========================================================================
// ALU Emitters
// ===========================================================================

// emitALU_AMD64 handles ADD, SUB, AND, OR, XOR.
// opcode64 is the reg,reg opcode (e.g., 0x01 for ADD).
// aluOp is the /r code for the imm32 form (e.g., 0 for ADD, 5 for SUB).
func emitALU_AMD64(cb *CodeBuffer, ji *JITInstr, opcode64 byte, aluOp byte) {
	if ji.rd == 0 {
		return
	}

	// Resolve source register (Rs)
	rsReg := resolveRegAMD64(cb, ji.rs, amd64RCX)

	// Copy Rs to scratch RAX (two-operand: dst = dst op src)
	amd64MOV_reg_reg(cb, amd64RAX, rsReg)

	if ji.xbit == 1 {
		// Immediate mode: RAX = RAX op imm32
		if ji.size == IE64_SIZE_L {
			amd64ALU_reg_imm32_32bit(cb, aluOp, amd64RAX, int32(ji.imm32))
		} else {
			amd64ALU_reg_imm32(cb, aluOp, amd64RAX, int32(ji.imm32))
		}
	} else {
		// Register mode: RAX = RAX op Rt
		opReg := resolveRegAMD64(cb, ji.rt, amd64RDX)
		if ji.size == IE64_SIZE_L {
			amd64ALU_reg_reg32(cb, opcode64, amd64RAX, opReg)
		} else {
			amd64ALU_reg_reg(cb, opcode64, amd64RAX, opReg)
		}
	}

	// Apply size mask for .B and .W
	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMaskAMD64(cb, amd64RAX, ji.size)
	}

	// Store result
	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

// ===========================================================================
// Multiply / Divide / Modulo Emitters
// ===========================================================================

// amd64IMUL_reg_reg emits IMUL dst, src (64-bit, two-operand form: dst = dst * src).
func amd64IMUL_reg_reg(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, true, dst, src)
	cb.EmitBytes(0x0F, 0xAF, modRM(3, dst, src))
}

// amd64IMUL_reg_reg32 emits IMUL dst32, src32 (32-bit).
func amd64IMUL_reg_reg32(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, false, dst, src)
	cb.EmitBytes(0x0F, 0xAF, modRM(3, dst, src))
}

// amd64DIV emits DIV src (unsigned: RDX:RAX / src → RAX=quotient, RDX=remainder).
func amd64DIV(cb *CodeBuffer, src byte) {
	emitREX(cb, true, 0, src)
	cb.EmitBytes(0xF7, modRM(3, 6, src)) // /6 = DIV
}

// amd64IDIV emits IDIV src (signed: RDX:RAX / src → RAX=quotient, RDX=remainder).
func amd64IDIV(cb *CodeBuffer, src byte) {
	emitREX(cb, true, 0, src)
	cb.EmitBytes(0xF7, modRM(3, 7, src)) // /7 = IDIV
}

// amd64DIV32 emits DIV src32 (unsigned 32-bit: EDX:EAX / src32).
func amd64DIV32(cb *CodeBuffer, src byte) {
	emitREX(cb, false, 0, src)
	cb.EmitBytes(0xF7, modRM(3, 6, src))
}

// amd64IDIV32 emits IDIV src32 (signed 32-bit: EDX:EAX / src32).
func amd64IDIV32(cb *CodeBuffer, src byte) {
	emitREX(cb, false, 0, src)
	cb.EmitBytes(0xF7, modRM(3, 7, src))
}

// amd64CQO emits CQO (sign-extend RAX into RDX:RAX).
func amd64CQO(cb *CodeBuffer) {
	cb.EmitBytes(0x48, 0x99) // REX.W CQO
}

// amd64CDQ emits CDQ (sign-extend EAX into EDX:EAX).
func amd64CDQ(cb *CodeBuffer) {
	cb.EmitBytes(0x99) // CDQ
}

// amd64TEST_reg_reg emits TEST dst, src (64-bit).
func amd64TEST_reg_reg(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, true, src, dst)
	cb.EmitBytes(0x85, modRM(3, src, dst))
}

func emitMULU_AMD64(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RCX)
	amd64MOV_reg_reg(cb, amd64RAX, rsReg)

	var opReg byte
	if ji.xbit == 1 {
		emitLoadImm64AMD64(cb, amd64RDX, uint64(ji.imm32))
		opReg = amd64RDX
	} else {
		opReg = resolveRegAMD64(cb, ji.rt, amd64RDX)
	}

	if ji.size == IE64_SIZE_L {
		amd64IMUL_reg_reg32(cb, amd64RAX, opReg)
	} else {
		amd64IMUL_reg_reg(cb, amd64RAX, opReg)
	}

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMaskAMD64(cb, amd64RAX, ji.size)
	}

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

func emitMULS_AMD64(cb *CodeBuffer, ji *JITInstr) {
	// IMUL gives same low-half result for signed and unsigned
	emitMULU_AMD64(cb, ji)
}

func emitDIVU_AMD64(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RCX)
	amd64MOV_reg_reg(cb, amd64RAX, rsReg)

	var divisorReg byte
	if ji.xbit == 1 {
		emitLoadImm64AMD64(cb, amd64R10, uint64(ji.imm32))
		divisorReg = amd64R10
	} else {
		divisorReg = resolveRegAMD64(cb, ji.rt, amd64R10)
		// If divisor resolved to RDX, move it out of the way
		if divisorReg == amd64RDX {
			amd64MOV_reg_reg(cb, amd64R10, amd64RDX)
			divisorReg = amd64R10
		}
	}

	// Zero-check: if divisor == 0, result = 0
	amd64TEST_reg_reg(cb, divisorReg, divisorReg)
	divZeroOff := amd64Jcc_rel32(cb, amd64CondE) // JE div_zero

	// XOR RDX, RDX (clear upper dividend)
	amd64XOR_reg_reg(cb, amd64RDX, amd64RDX)
	// Always 64-bit DIV: interpreter does 64-bit divide, then masks result to size
	amd64DIV(cb, divisorReg)
	doneOff := amd64JMP_rel32(cb) // JMP done

	// div_zero: XOR RAX, RAX
	divZeroPC := cb.Len()
	patchRel32(cb, divZeroOff, divZeroPC)
	amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)

	// done:
	donePC := cb.Len()
	patchRel32(cb, doneOff, donePC)

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMaskAMD64(cb, amd64RAX, ji.size)
	}

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

func emitDIVS_AMD64(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RCX)
	amd64MOV_reg_reg(cb, amd64RAX, rsReg)

	var divisorReg byte
	if ji.xbit == 1 {
		emitLoadImm64AMD64(cb, amd64R10, uint64(ji.imm32))
		divisorReg = amd64R10
	} else {
		divisorReg = resolveRegAMD64(cb, ji.rt, amd64R10)
		if divisorReg == amd64RDX {
			amd64MOV_reg_reg(cb, amd64R10, amd64RDX)
			divisorReg = amd64R10
		}
	}

	// Zero-check
	amd64TEST_reg_reg(cb, divisorReg, divisorReg)
	divZeroOff := amd64Jcc_rel32(cb, amd64CondE)

	// Sign-extend RAX into RDX:RAX — always 64-bit
	amd64CQO(cb)
	amd64IDIV(cb, divisorReg)
	doneOff := amd64JMP_rel32(cb)

	divZeroPC := cb.Len()
	patchRel32(cb, divZeroOff, divZeroPC)
	amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)

	donePC := cb.Len()
	patchRel32(cb, doneOff, donePC)

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMaskAMD64(cb, amd64RAX, ji.size)
	}

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

func emitMOD_AMD64(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RCX)
	amd64MOV_reg_reg(cb, amd64RAX, rsReg)

	var divisorReg byte
	if ji.xbit == 1 {
		emitLoadImm64AMD64(cb, amd64R10, uint64(ji.imm32))
		divisorReg = amd64R10
	} else {
		divisorReg = resolveRegAMD64(cb, ji.rt, amd64R10)
		if divisorReg == amd64RDX {
			amd64MOV_reg_reg(cb, amd64R10, amd64RDX)
			divisorReg = amd64R10
		}
	}

	// Zero-check
	amd64TEST_reg_reg(cb, divisorReg, divisorReg)
	divZeroOff := amd64Jcc_rel32(cb, amd64CondE)

	// XOR RDX, RDX (unsigned modulo) — always 64-bit
	amd64XOR_reg_reg(cb, amd64RDX, amd64RDX)
	amd64DIV(cb, divisorReg)
	// Result is in RDX (remainder)
	amd64MOV_reg_reg(cb, amd64RAX, amd64RDX)
	doneOff := amd64JMP_rel32(cb)

	divZeroPC := cb.Len()
	patchRel32(cb, divZeroOff, divZeroPC)
	amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)

	donePC := cb.Len()
	patchRel32(cb, doneOff, donePC)

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMaskAMD64(cb, amd64RAX, ji.size)
	}

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

// ===========================================================================
// Logic Emitters
// ===========================================================================

func emitNOT_AMD64(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RCX)
	amd64MOV_reg_reg(cb, amd64RAX, rsReg)
	amd64NOT(cb, amd64RAX)

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W || ji.size == IE64_SIZE_L {
		emitSizeMaskAMD64(cb, amd64RAX, ji.size)
	}

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

// ===========================================================================
// Shift Emitters
// ===========================================================================

// emitShift_AMD64 handles LSL (shiftOp=4/SHL) and LSR (shiftOp=5/SHR).
func emitShift_AMD64(cb *CodeBuffer, ji *JITInstr, shiftOp byte) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RAX)
	amd64MOV_reg_reg(cb, amd64RAX, rsReg)

	if ji.xbit == 1 {
		// Immediate shift — always 64-bit (IE64 masks count & 63, x86-64 does this natively for 64-bit shifts)
		imm := byte(ji.imm32 & 0x3F)
		emitREX(cb, true, 0, amd64RAX)
		cb.EmitBytes(0xC1, modRM(3, shiftOp, amd64RAX), imm)
	} else {
		// Variable shift (count in CL) — always 64-bit
		countReg := resolveRegAMD64(cb, ji.rt, amd64RDX)
		amd64MOV_reg_reg(cb, amd64RCX, countReg) // move count to RCX
		emitREX(cb, true, 0, amd64RAX)
		cb.EmitBytes(0xD3, modRM(3, shiftOp, amd64RAX))
	}

	// Apply size mask (interpreter does 64-bit shift then maskToSize)
	if ji.size != IE64_SIZE_Q {
		emitSizeMaskAMD64(cb, amd64RAX, ji.size)
	}

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

// emitASR_AMD64 handles ASR (arithmetic shift right).
// IE64 ASR has size-dependent sign-extension before shifting.
func emitASR_AMD64(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RAX)
	amd64MOV_reg_reg(cb, amd64RAX, rsReg)

	// Sign-extend based on size before shifting
	switch ji.size {
	case IE64_SIZE_B:
		// MOVSX EAX, AL → sign-extend byte to 64-bit
		emitREX(cb, true, amd64RAX, amd64RAX)
		cb.EmitBytes(0x0F, 0xBE, modRM(3, amd64RAX, amd64RAX))
	case IE64_SIZE_W:
		// MOVSX EAX, AX → sign-extend word to 64-bit
		emitREX(cb, true, amd64RAX, amd64RAX)
		cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RAX, amd64RAX))
	case IE64_SIZE_L:
		// MOVSXD RAX, EAX → sign-extend dword to 64-bit
		amd64MOVSXD(cb, amd64RAX, amd64RAX)
	case IE64_SIZE_Q:
		// Already 64-bit, no sign extension needed
	}

	if ji.xbit == 1 {
		imm := byte(ji.imm32 & 0x3F)
		emitREX(cb, true, 0, amd64RAX)
		cb.EmitBytes(0xC1, modRM(3, 7, amd64RAX), imm) // SAR RAX, imm8
	} else {
		countReg := resolveRegAMD64(cb, ji.rt, amd64RDX)
		amd64MOV_reg_reg(cb, amd64RCX, countReg)
		amd64SAR_CL(cb, amd64RAX)
	}

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W || ji.size == IE64_SIZE_L {
		emitSizeMaskAMD64(cb, amd64RAX, ji.size)
	}

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

// emitCLZ_AMD64 handles CLZ (count leading zeros, 32-bit operation).
// Uses BSR-based sequence for universal amd64 compatibility.
func emitCLZ_AMD64(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RCX)

	// MOV EAX, src32 (truncate to uint32)
	amd64MOV_reg_reg32(cb, amd64RAX, rsReg)

	// TEST EAX, EAX
	emitREX(cb, false, amd64RAX, amd64RAX)
	cb.EmitBytes(0x85, modRM(3, amd64RAX, amd64RAX))

	// JZ .clz_zero
	clzZeroOff := amd64Jcc_rel32(cb, amd64CondE)

	// BSR ECX, EAX (find highest set bit)
	emitREX(cb, false, amd64RCX, amd64RAX)
	cb.EmitBytes(0x0F, 0xBD, modRM(3, amd64RCX, amd64RAX))

	// XOR ECX, 31 (convert bit index to leading zero count)
	amd64ALU_reg_imm32_32bit(cb, 6, amd64RCX, 31) // XOR ECX, 31

	// MOV RAX, RCX (result)
	amd64MOV_reg_reg(cb, amd64RAX, amd64RCX)

	clzDoneOff := amd64JMP_rel32(cb)

	// .clz_zero: MOV EAX, 32
	clzZeroPC := cb.Len()
	patchRel32(cb, clzZeroOff, clzZeroPC)
	amd64MOV_reg_imm32(cb, amd64RAX, 32)

	// .clz_done:
	clzDonePC := cb.Len()
	patchRel32(cb, clzDoneOff, clzDonePC)

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

// ===========================================================================
// NEG Emitter
// ===========================================================================

// emitNEG_AMD64 handles NEG rd, rs.
func emitNEG_AMD64(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RCX)
	amd64MOV_reg_reg(cb, amd64RAX, rsReg)

	if ji.size == IE64_SIZE_L {
		amd64NEG32(cb, amd64RAX)
	} else {
		amd64NEG(cb, amd64RAX)
	}

	if ji.size == IE64_SIZE_B || ji.size == IE64_SIZE_W {
		emitSizeMaskAMD64(cb, amd64RAX, ji.size)
	}

	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

// ===========================================================================
// Memory Access Emitters (LOAD / STORE)
// ===========================================================================

// emitMemAddr computes address into RAX: uint32(int64(rs) + int64(int32(imm32)))
func emitMemAddr(cb *CodeBuffer, ji *JITInstr) {
	rsReg := resolveRegAMD64(cb, ji.rs, amd64RCX)
	amd64MOV_reg_imm32(cb, amd64RDX, ji.imm32)
	amd64MOVSXD(cb, amd64RDX, amd64RDX)
	amd64MOV_reg_reg(cb, amd64RAX, rsReg)
	amd64ALU_reg_reg(cb, 0x01, amd64RAX, amd64RDX) // ADD RAX, RDX
	amd64MOV_reg_reg32(cb, amd64RAX, amd64RAX)     // truncate to uint32
}

// emitMemLoad emits a sized load from [RSI + RAX] into dstReg.
func emitMemLoad(cb *CodeBuffer, dstReg byte, size byte) {
	switch size {
	case IE64_SIZE_B:
		emitREX_SIB(cb, false, dstReg, amd64RAX, amd64RegMemBase)
		cb.EmitBytes(0x0F, 0xB6, modRM(0, dstReg, 4), sibByte(0, amd64RAX, amd64RegMemBase))
	case IE64_SIZE_W:
		emitREX_SIB(cb, false, dstReg, amd64RAX, amd64RegMemBase)
		cb.EmitBytes(0x0F, 0xB7, modRM(0, dstReg, 4), sibByte(0, amd64RAX, amd64RegMemBase))
	case IE64_SIZE_L:
		emitMemOpSIB(cb, false, 0x8B, dstReg, amd64RegMemBase, amd64RAX, 0)
	case IE64_SIZE_Q:
		emitMemOpSIB(cb, true, 0x8B, dstReg, amd64RegMemBase, amd64RAX, 0)
	}
}

// emitMemStore emits a sized store of srcReg to [RSI + RAX].
func emitMemStore(cb *CodeBuffer, srcReg byte, size byte) {
	switch size {
	case IE64_SIZE_B:
		emitREX_SIB(cb, false, srcReg, amd64RAX, amd64RegMemBase)
		cb.EmitBytes(0x88, modRM(0, srcReg, 4), sibByte(0, amd64RAX, amd64RegMemBase))
	case IE64_SIZE_W:
		cb.EmitBytes(0x66)
		emitREX_SIB(cb, false, srcReg, amd64RAX, amd64RegMemBase)
		cb.EmitBytes(0x89, modRM(0, srcReg, 4), sibByte(0, amd64RAX, amd64RegMemBase))
	case IE64_SIZE_L:
		emitMemOpSIB(cb, false, 0x89, srcReg, amd64RegMemBase, amd64RAX, 0)
	case IE64_SIZE_Q:
		emitMemOpSIB(cb, true, 0x89, srcReg, amd64RegMemBase, amd64RAX, 0)
	}
}

func emitLOAD_AMD64(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
	if ji.rd == 0 {
		return
	}

	emitMemAddr(cb, ji)

	// Compare with IO_REGION_START (R8)
	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, amd64RegIOStart) // CMP EAX, R8d
	slowPathOff := amd64Jcc_rel32(cb, amd64CondAE)

	// Fast path
	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if !mapped {
		dstReg = amd64R10
	}
	emitMemLoad(cb, dstReg, ji.size)
	if !mapped {
		emitStoreSpilledRegAMD64(cb, dstReg, ji.rd)
	}
	doneOff := amd64JMP_rel32(cb)

	// Slow path
	slowPathPC := cb.Len()
	patchRel32(cb, slowPathOff, slowPathPC)

	amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
	amd64SHR_imm(cb, amd64RCX, 8)
	emitREX_SIB(cb, false, amd64RCX, amd64RCX, amd64RegIOBitmap)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, amd64RCX, 4), sibByte(0, amd64RCX, amd64RegIOBitmap))
	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	nonIOOff := amd64Jcc_rel32(cb, amd64CondE)

	emitIOBail(cb, instrPC, ji.pcOffset, br, writtenSoFar)

	nonIOPC := cb.Len()
	patchRel32(cb, nonIOOff, nonIOPC)
	emitMemLoad(cb, dstReg, ji.size)
	if !mapped {
		emitStoreSpilledRegAMD64(cb, dstReg, ji.rd)
	}

	donePC := cb.Len()
	patchRel32(cb, doneOff, donePC)
}

func emitSTORE_AMD64(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
	emitMemAddr(cb, ji)

	srcReg := resolveRegAMD64(cb, ji.rd, amd64R10)
	if ji.size != IE64_SIZE_Q {
		amd64MOV_reg_reg(cb, amd64R11, srcReg)
		emitSizeMaskAMD64(cb, amd64R11, ji.size)
		srcReg = amd64R11
	}

	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, amd64RegIOStart)
	slowPathOff := amd64Jcc_rel32(cb, amd64CondAE)

	emitMemStore(cb, srcReg, ji.size)
	doneOff := amd64JMP_rel32(cb)

	slowPathPC := cb.Len()
	patchRel32(cb, slowPathOff, slowPathPC)

	amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
	amd64SHR_imm(cb, amd64RCX, 8)
	emitREX_SIB(cb, false, amd64RCX, amd64RCX, amd64RegIOBitmap)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, amd64RCX, 4), sibByte(0, amd64RCX, amd64RegIOBitmap))
	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	nonIOOff := amd64Jcc_rel32(cb, amd64CondE)

	emitIOBail(cb, instrPC, ji.pcOffset, br, writtenSoFar)

	nonIOPC := cb.Len()
	patchRel32(cb, nonIOOff, nonIOPC)
	emitMemStore(cb, srcReg, ji.size)

	donePC := cb.Len()
	patchRel32(cb, doneOff, donePC)
}

// emitIOBail emits the I/O bail path.
func emitIOBail(cb *CodeBuffer, instrPC uint32, pcOffset uint32, br *blockRegs, writtenSoFar uint32) {
	amd64MOV_reg_mem(cb, amd64RCX, amd64RSP, int32(amd64OffCtxPtr))
	amd64MOV_mem_imm32(cb, amd64RCX, int32(jitCtxOffNeedIOFallback), 1)
	bailCount := pcOffset / IE64_INSTR_SIZE
	emitPackedPCAndCount(cb, uint64(instrPC), bailCount, br)
	emitEpilogue(cb, writtenSoFar, br.used)
}

// emitBailToInterpreter is used for RTI, WAIT, and FPU transcendentals.
func emitBailToInterpreter(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
	emitIOBail(cb, instrPC, ji.pcOffset, br, writtenSoFar)
}

// ===========================================================================
// Branch Emitters
// ===========================================================================

func emitBRA_AMD64(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, instrIdx int, instrOffsets []int, blockStartPC uint32) {
	targetPC := uint32(int64(instrPC) + int64(int32(ji.imm32)))
	staticCount := uint32(instrIdx + 1)

	if br.hasBackwardBranch && targetPC >= blockStartPC && targetPC < instrPC &&
		(targetPC-blockStartPC)%IE64_INSTR_SIZE == 0 {
		targetIdx := int((targetPC - blockStartPC) / IE64_INSTR_SIZE)
		if targetIdx >= 0 && targetIdx < instrIdx && targetIdx < len(instrOffsets) {
			bodySize := uint32(instrIdx - targetIdx + 1)

			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(amd64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, amd64RSP, int32(amd64OffLoopCount), amd64RAX)
			amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, int32(jitBudget))
			budgetExitOff := amd64Jcc_rel32(cb, amd64CondAE)

			targetOffset := instrOffsets[targetIdx]
			backOff := amd64JMP_rel32(cb)
			patchRel32(cb, backOff, targetOffset)

			budgetExitPC := cb.Len()
			patchRel32(cb, budgetExitOff, budgetExitPC)
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(amd64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, amd64RSP, int32(amd64OffLoopCount), amd64RAX)
			emitPackedPCAndCount(cb, uint64(targetPC), staticCount, br)
			emitEpilogue(cb, br.written, br.used)
			return
		}
	}

	emitPackedPCAndCount(cb, uint64(targetPC), staticCount, br)
	emitEpilogue(cb, br.written, br.used)
}

func invertCond(cond byte) byte {
	return cond ^ 1
}

func emitBcc_AMD64(cb *CodeBuffer, ji *JITInstr, instrPC uint32, cond byte, br *blockRegs, writtenSoFar uint32, blockStartPC uint32, instrIdx int, instrOffsets []int) {
	targetPC := uint32(int64(instrPC) + int64(int32(ji.imm32)))
	staticCount := uint32(instrIdx + 1)

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RAX)
	rtReg := resolveRegAMD64(cb, ji.rt, amd64RDX)
	amd64ALU_reg_reg(cb, 0x39, rsReg, rtReg) // CMP rs, rt

	if br.hasBackwardBranch && targetPC >= blockStartPC && targetPC < instrPC &&
		(targetPC-blockStartPC)%IE64_INSTR_SIZE == 0 {
		targetIdx := int((targetPC - blockStartPC) / IE64_INSTR_SIZE)
		if targetIdx >= 0 && targetIdx < instrIdx && targetIdx < len(instrOffsets) {
			skipOff := amd64Jcc_rel32(cb, invertCond(cond))

			bodySize := uint32(instrIdx - targetIdx + 1)
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(amd64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, amd64RSP, int32(amd64OffLoopCount), amd64RAX)
			amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, int32(jitBudget))
			budgetExitOff := amd64Jcc_rel32(cb, amd64CondAE)

			targetOffset := instrOffsets[targetIdx]
			backOff := amd64JMP_rel32(cb)
			patchRel32(cb, backOff, targetOffset)

			budgetExitPC := cb.Len()
			patchRel32(cb, budgetExitOff, budgetExitPC)
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(amd64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, amd64RSP, int32(amd64OffLoopCount), amd64RAX)
			emitPackedPCAndCount(cb, uint64(targetPC), staticCount, br)
			emitEpilogue(cb, br.written, br.used)

			skipPC := cb.Len()
			patchRel32(cb, skipOff, skipPC)
			return
		}
	}

	skipOff := amd64Jcc_rel32(cb, invertCond(cond))

	if br.hasBackwardBranch {
		emitLoadImm64AMD64(cb, amd64RegIE64PC, uint64(targetPC))
		emitDynamicCountAMD64(cb, staticCount)
	} else {
		emitPackedPCAndCount(cb, uint64(targetPC), staticCount, br)
	}
	emitEpilogue(cb, br.written, br.used)

	skipPC := cb.Len()
	patchRel32(cb, skipOff, skipPC)
}

func emitJMP_AMD64(cb *CodeBuffer, ji *JITInstr, br *blockRegs, instrCount uint32) {
	rsReg := resolveRegAMD64(cb, ji.rs, amd64RAX)
	amd64MOV_reg_imm32(cb, amd64RCX, ji.imm32)
	amd64MOVSXD(cb, amd64RCX, amd64RCX)
	amd64MOV_reg_reg(cb, amd64RegIE64PC, rsReg)
	amd64ALU_reg_reg(cb, 0x01, amd64RegIE64PC, amd64RCX)

	emitLoadImm64AMD64(cb, amd64RAX, IE64_ADDR_MASK)
	amd64ALU_reg_reg(cb, 0x21, amd64RegIE64PC, amd64RAX)

	if br.hasBackwardBranch {
		emitDynamicCountAMD64(cb, instrCount)
	} else {
		emitLoadImm64AMD64(cb, amd64RAX, uint64(instrCount)<<32)
		amd64ALU_reg_reg(cb, 0x09, amd64RegIE64PC, amd64RAX)
	}
	emitEpilogue(cb, br.written, br.used)
}

// ===========================================================================
// Subroutine / Stack Emitters
// ===========================================================================

func emitPUSH_AMD64(cb *CodeBuffer, ji *JITInstr) {
	amd64ALU_reg_imm32(cb, 5, amd64RegIE64SP, 8) // SUB R14, 8
	srcReg := resolveRegAMD64(cb, ji.rs, amd64RAX)
	emitMemOpSIB(cb, true, 0x89, srcReg, amd64RegMemBase, amd64RegIE64SP, 0) // MOV [RSI+R14], src
}

func emitPOP_AMD64(cb *CodeBuffer, ji *JITInstr) {
	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if !mapped {
		dstReg = amd64RAX
	}

	if ji.rd != 0 {
		emitMemOpSIB(cb, true, 0x8B, dstReg, amd64RegMemBase, amd64RegIE64SP, 0)
	}
	amd64ALU_reg_imm32(cb, 0, amd64RegIE64SP, 8) // ADD R14, 8

	if ji.rd != 0 && !mapped {
		emitStoreSpilledRegAMD64(cb, dstReg, ji.rd)
	}
}

func emitJSR_AMD64(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs) {
	amd64ALU_reg_imm32(cb, 5, amd64RegIE64SP, 8)
	retAddr := uint64(instrPC + IE64_INSTR_SIZE)
	emitLoadImm64AMD64(cb, amd64RAX, retAddr)
	emitMemOpSIB(cb, true, 0x89, amd64RAX, amd64RegMemBase, amd64RegIE64SP, 0)

	targetPC := uint64(int64(instrPC) + int64(int32(ji.imm32)))
	instrCount := uint32(ji.pcOffset/IE64_INSTR_SIZE + 1)
	emitPackedPCAndCount(cb, targetPC, instrCount, br)
	emitEpilogue(cb, br.written, br.used)
}

func emitRTS_AMD64(cb *CodeBuffer, br *blockRegs, instrCount uint32) {
	emitMemOpSIB(cb, true, 0x8B, amd64RegIE64PC, amd64RegMemBase, amd64RegIE64SP, 0)
	amd64ALU_reg_imm32(cb, 0, amd64RegIE64SP, 8)

	emitLoadImm64AMD64(cb, amd64RAX, IE64_ADDR_MASK)
	amd64ALU_reg_reg(cb, 0x21, amd64RegIE64PC, amd64RAX)

	if br.hasBackwardBranch {
		emitDynamicCountAMD64(cb, instrCount)
	} else {
		emitLoadImm64AMD64(cb, amd64RAX, uint64(instrCount)<<32)
		amd64ALU_reg_reg(cb, 0x09, amd64RegIE64PC, amd64RAX)
	}
	emitEpilogue(cb, br.written, br.used)
}

func emitJSR_IND_AMD64(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, instrCount uint32) {
	amd64ALU_reg_imm32(cb, 5, amd64RegIE64SP, 8)
	retAddr := uint64(instrPC + IE64_INSTR_SIZE)
	emitLoadImm64AMD64(cb, amd64RAX, retAddr)
	emitMemOpSIB(cb, true, 0x89, amd64RAX, amd64RegMemBase, amd64RegIE64SP, 0)

	rsReg := resolveRegAMD64(cb, ji.rs, amd64RAX)
	amd64MOV_reg_imm32(cb, amd64RCX, ji.imm32)
	amd64MOVSXD(cb, amd64RCX, amd64RCX)
	amd64MOV_reg_reg(cb, amd64RegIE64PC, rsReg)
	amd64ALU_reg_reg(cb, 0x01, amd64RegIE64PC, amd64RCX)

	emitLoadImm64AMD64(cb, amd64RAX, IE64_ADDR_MASK)
	amd64ALU_reg_reg(cb, 0x21, amd64RegIE64PC, amd64RAX)

	if br.hasBackwardBranch {
		emitDynamicCountAMD64(cb, instrCount)
	} else {
		emitLoadImm64AMD64(cb, amd64RAX, uint64(instrCount)<<32)
		amd64ALU_reg_reg(cb, 0x09, amd64RegIE64PC, amd64RAX)
	}
	emitEpilogue(cb, br.written, br.used)
}

// ===========================================================================
// FPU Helpers
// ===========================================================================

func emitLoadFPRegAMD64(cb *CodeBuffer, amd64Dst, fpIdx byte) {
	amd64MOV_reg_mem(cb, amd64R11, amd64RSP, int32(amd64OffFPUPtr))
	amd64MOV_reg_mem32(cb, amd64Dst, amd64R11, int32(fpIdx&0x0F)*4)
}

func emitStoreFPRegAMD64(cb *CodeBuffer, amd64Src, fpIdx byte) {
	amd64MOV_reg_mem(cb, amd64R11, amd64RSP, int32(amd64OffFPUPtr))
	amd64MOV_mem_reg32(cb, amd64R11, int32(fpIdx&0x0F)*4, amd64Src)
}

// ===========================================================================
// FPU Condition Code Setter
// ===========================================================================

// emitSetFPCondCodesAMD64 classifies IEEE-754 bits in EAX and updates FPSR
// condition codes (bits 27:24). Preserves exception flags (bits 3:0).
// Uses RCX, RDX as scratch. EAX must contain the float32 bit pattern.
func emitSetFPCondCodesAMD64(cb *CodeBuffer) {
	// ECX = exponent = (EAX >> 23) & 0xFF
	amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
	amd64SHR_imm(cb, amd64RCX, 23)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 0xFF) // AND ECX, 0xFF

	// EDX = default CC = 0
	amd64XOR_reg_reg32(cb, amd64RDX, amd64RDX)

	// Check special: exp == 0xFF
	amd64ALU_reg_imm32_32bit(cb, 7, amd64RCX, 0xFF)  // CMP ECX, 0xFF
	notSpecialOff := amd64Jcc_rel32(cb, amd64CondNE) // JNE not_special

	// exp == 0xFF: check NaN vs Inf
	// ECX = fraction = EAX & 0x7FFFFF
	amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 0x7FFFFF) // AND ECX, 0x7FFFFF
	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX)) // TEST ECX, ECX
	isNanOff := amd64Jcc_rel32(cb, amd64CondNE)      // JNZ is_nan

	// Infinity: CC_I = 0x02000000
	emitLoadImm32AMD64(cb, amd64RDX, IE64_FPU_CC_I)
	// Check sign: EAX >> 31
	amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
	amd64SHR_imm(cb, amd64RCX, 31)
	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	storeCCFromInfOff := amd64Jcc_rel32(cb, amd64CondE) // JZ store_cc (positive inf)
	// Negative infinity: add CC_N
	amd64ALU_reg_imm32_32bit(cb, 1, amd64RDX, int32(IE64_FPU_CC_N)) // OR EDX, CC_N
	storeCCFromNegInfOff := amd64JMP_rel32(cb)                      // JMP store_cc

	// is_nan:
	isNanPC := cb.Len()
	patchRel32(cb, isNanOff, isNanPC)
	emitLoadImm32AMD64(cb, amd64RDX, IE64_FPU_CC_NAN)
	storeCCFromNanOff := amd64JMP_rel32(cb)

	// not_special:
	notSpecialPC := cb.Len()
	patchRel32(cb, notSpecialOff, notSpecialPC)
	// Check zero: bits & 0x7FFFFFFF == 0
	amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 0x7FFFFFFF) // AND ECX, 0x7FFFFFFF
	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	isZeroOff := amd64Jcc_rel32(cb, amd64CondE) // JZ is_zero

	// Normal: check sign
	amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
	amd64SHR_imm(cb, amd64RDX, 31)
	storeCCFromPosOff := amd64Jcc_rel32(cb, amd64CondE) // JZ store_cc (positive, CC=0)
	// Negative normal: CC_N
	emitLoadImm32AMD64(cb, amd64RDX, IE64_FPU_CC_N)
	storeCCFromNegOff := amd64JMP_rel32(cb)

	// is_zero:
	isZeroPC := cb.Len()
	patchRel32(cb, isZeroOff, isZeroPC)
	emitLoadImm32AMD64(cb, amd64RDX, IE64_FPU_CC_Z)
	// fall through to store_cc

	// store_cc:
	storeCCPC := cb.Len()
	patchRel32(cb, storeCCFromInfOff, storeCCPC)
	patchRel32(cb, storeCCFromNegInfOff, storeCCPC)
	patchRel32(cb, storeCCFromNanOff, storeCCPC)
	patchRel32(cb, storeCCFromPosOff, storeCCPC)
	patchRel32(cb, storeCCFromNegOff, storeCCPC)

	// Load FPSR, preserve exception bits (3:0), set new CC
	amd64MOV_reg_mem(cb, amd64R11, amd64RSP, int32(amd64OffFPUPtr))
	amd64MOV_reg_mem32(cb, amd64RCX, amd64R11, 68)   // ECX = FPSR
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 0x0F)  // AND ECX, 0x0F (keep exception flags)
	amd64ALU_reg_reg32(cb, 0x09, amd64RCX, amd64RDX) // OR ECX, EDX (merge CC)
	amd64MOV_mem_reg32(cb, amd64R11, 68, amd64RCX)   // FPSR = ECX
}

// ===========================================================================
// FPU — Category A: Integer bitwise on FP registers
// ===========================================================================

func emitFMOV_AMD64(cb *CodeBuffer, ji *JITInstr) {
	emitLoadFPRegAMD64(cb, amd64RAX, ji.rs)
	emitStoreFPRegAMD64(cb, amd64RAX, ji.rd)
}

func emitFABS_AMD64(cb *CodeBuffer, ji *JITInstr) {
	emitLoadFPRegAMD64(cb, amd64RAX, ji.rs)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x7FFFFFFF) // AND EAX, 0x7FFFFFFF
	emitStoreFPRegAMD64(cb, amd64RAX, ji.rd)
	emitSetFPCondCodesAMD64(cb)
}

func emitFNEG_AMD64(cb *CodeBuffer, ji *JITInstr) {
	emitLoadFPRegAMD64(cb, amd64RAX, ji.rs)
	emitLoadImm32AMD64(cb, amd64RCX, 0x80000000)
	amd64ALU_reg_reg32(cb, 0x31, amd64RAX, amd64RCX) // XOR EAX, ECX
	emitStoreFPRegAMD64(cb, amd64RAX, ji.rd)
	emitSetFPCondCodesAMD64(cb)
}

func emitFMOVI_AMD64(cb *CodeBuffer, ji *JITInstr) {
	rsReg := resolveRegAMD64(cb, ji.rs, amd64RAX)
	amd64MOV_reg_reg32(cb, amd64RAX, rsReg)
	emitStoreFPRegAMD64(cb, amd64RAX, ji.rd)
	emitSetFPCondCodesAMD64(cb)
}

func emitFMOVO_AMD64(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}
	emitLoadFPRegAMD64(cb, amd64RAX, ji.rs)
	amd64MOV_reg_reg32(cb, amd64RAX, amd64RAX)
	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

func emitFMOVECR_AMD64(cb *CodeBuffer, ji *JITInstr) {
	idx := byte(ji.imm32)
	var bits uint32
	if idx <= 15 {
		bits = ie64FmovecrROMTable[idx]
	}
	emitLoadImm32AMD64(cb, amd64RAX, bits)
	emitStoreFPRegAMD64(cb, amd64RAX, ji.rd)
	emitSetFPCondCodesAMD64(cb)
}

func emitFMOVSR_AMD64(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}
	amd64MOV_reg_mem(cb, amd64R11, amd64RSP, int32(amd64OffFPUPtr))
	amd64MOV_reg_mem32(cb, amd64RAX, amd64R11, 68)
	amd64MOV_reg_reg32(cb, amd64RAX, amd64RAX)
	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

func emitFMOVCR_AMD64(cb *CodeBuffer, ji *JITInstr) {
	if ji.rd == 0 {
		return
	}
	amd64MOV_reg_mem(cb, amd64R11, amd64RSP, int32(amd64OffFPUPtr))
	amd64MOV_reg_mem32(cb, amd64RAX, amd64R11, 64)
	amd64MOV_reg_reg32(cb, amd64RAX, amd64RAX)
	dstReg, mapped := ie64ToAMD64Reg(ji.rd)
	if mapped {
		amd64MOV_reg_reg(cb, dstReg, amd64RAX)
	} else {
		emitStoreSpilledRegAMD64(cb, amd64RAX, ji.rd)
	}
}

func emitFMOVSC_AMD64(cb *CodeBuffer, ji *JITInstr) {
	rsReg := resolveRegAMD64(cb, ji.rs, amd64RAX)
	amd64MOV_reg_reg32(cb, amd64RAX, rsReg)
	emitLoadImm32AMD64(cb, amd64RCX, IE64_FPU_FPSR_MASK)
	amd64ALU_reg_reg32(cb, 0x21, amd64RAX, amd64RCX)
	amd64MOV_reg_mem(cb, amd64R11, amd64RSP, int32(amd64OffFPUPtr))
	amd64MOV_mem_reg32(cb, amd64R11, 68, amd64RAX)
}

func emitFMOVCC_AMD64(cb *CodeBuffer, ji *JITInstr) {
	rsReg := resolveRegAMD64(cb, ji.rs, amd64RAX)
	amd64MOV_reg_reg32(cb, amd64RAX, rsReg)
	amd64MOV_reg_mem(cb, amd64R11, amd64RSP, int32(amd64OffFPUPtr))
	amd64MOV_mem_reg32(cb, amd64R11, 64, amd64RAX)
}

// ===========================================================================
// FPU — Category B: Native SSE instructions
// ===========================================================================

func amd64MOVD_xmm_reg(cb *CodeBuffer, xmm, gpr byte) {
	cb.EmitBytes(0x66)
	emitREX(cb, false, xmm, gpr)
	cb.EmitBytes(0x0F, 0x6E, modRM(3, xmm, gpr))
}

func amd64MOVD_reg_xmm(cb *CodeBuffer, gpr, xmm byte) {
	cb.EmitBytes(0x66)
	emitREX(cb, false, xmm, gpr)
	cb.EmitBytes(0x0F, 0x7E, modRM(3, xmm, gpr))
}

func amd64SSE_scalar(cb *CodeBuffer, opcode byte, dst, src byte) {
	cb.EmitBytes(0xF3)
	emitREX(cb, false, dst, src)
	cb.EmitBytes(0x0F, opcode, modRM(3, dst, src))
}

func emitFPBinarySSE(cb *CodeBuffer, ji *JITInstr, sseOpcode byte) {
	emitLoadFPRegAMD64(cb, amd64RAX, ji.rs)
	emitLoadFPRegAMD64(cb, amd64RCX, ji.rt)
	amd64MOVD_xmm_reg(cb, 0, amd64RAX)
	amd64MOVD_xmm_reg(cb, 1, amd64RCX)
	amd64SSE_scalar(cb, sseOpcode, 0, 1)
	amd64MOVD_reg_xmm(cb, amd64RAX, 0)
	emitStoreFPRegAMD64(cb, amd64RAX, ji.rd)
}

func emitFSQRT_AMD64(cb *CodeBuffer, ji *JITInstr) {
	emitLoadFPRegAMD64(cb, amd64RAX, ji.rs)
	amd64MOVD_xmm_reg(cb, 0, amd64RAX)
	amd64SSE_scalar(cb, 0x51, 1, 0) // SQRTSS XMM1, XMM0
	amd64MOVD_reg_xmm(cb, amd64RAX, 1)
	emitStoreFPRegAMD64(cb, amd64RAX, ji.rd)
	emitSetFPCondCodesAMD64(cb)
}

// emitFCMP_AMD64 handles FCMP using UCOMISS.
func emitFCMP_AMD64(cb *CodeBuffer, ji *JITInstr) {
	// Load FP regs
	emitLoadFPRegAMD64(cb, amd64RAX, ji.rs)
	amd64MOVD_xmm_reg(cb, 0, amd64RAX)
	emitLoadFPRegAMD64(cb, amd64RCX, ji.rt)
	amd64MOVD_xmm_reg(cb, 1, amd64RCX)

	// Clear CC bits in FPSR, keep exception flags
	amd64MOV_reg_mem(cb, amd64R11, amd64RSP, int32(amd64OffFPUPtr))
	amd64MOV_reg_mem32(cb, amd64RDX, amd64R11, 68)  // EDX = FPSR
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0x0F) // AND EDX, 0x0F (keep exceptions)

	// UCOMISS XMM0, XMM1 (sets EFLAGS: PF=unordered, ZF=equal, CF=less)
	cb.EmitBytes(0x0F, 0x2E, modRM(3, 0, 1))

	// Branch BEFORE any flag-clobbering instructions
	// Check PF (parity) → unordered (NaN)
	nanOff := amd64Jcc_rel32(cb, 0x0A) // JP (parity set) = 0x0A

	// Check CF → less than
	ltOff := amd64Jcc_rel32(cb, amd64CondB) // JB (CF=1) = below

	// Check ZF → equal
	eqOff := amd64Jcc_rel32(cb, amd64CondE) // JE

	// Greater than (fallthrough): result = 1
	emitLoadImm32AMD64(cb, amd64RCX, 1)
	done1Off := amd64JMP_rel32(cb)

	// nan: result = 0
	nanPC := cb.Len()
	patchRel32(cb, nanOff, nanPC)
	amd64XOR_reg_reg32(cb, amd64RCX, amd64RCX) // result = 0 for NaN
	emitLoadImm32AMD64(cb, amd64RAX, IE64_FPU_CC_NAN|IE64_FPU_EX_IO)
	amd64ALU_reg_reg32(cb, 0x09, amd64RDX, amd64RAX) // OR EDX, CC_NAN|EX_IO
	done2Off := amd64JMP_rel32(cb)

	// lt:
	ltPC := cb.Len()
	patchRel32(cb, ltOff, ltPC)
	// result = -1 (sign-extended)
	amd64MOV_reg_imm32(cb, amd64RCX, 0xFFFFFFFF)
	amd64MOVSXD(cb, amd64RCX, amd64RCX) // RCX = -1 (64-bit)
	emitLoadImm32AMD64(cb, amd64RAX, IE64_FPU_CC_N)
	amd64ALU_reg_reg32(cb, 0x09, amd64RDX, amd64RAX)
	done3Off := amd64JMP_rel32(cb)

	// eq: result = 0
	eqPC := cb.Len()
	patchRel32(cb, eqOff, eqPC)
	amd64XOR_reg_reg32(cb, amd64RCX, amd64RCX) // result = 0
	emitLoadImm32AMD64(cb, amd64RAX, IE64_FPU_CC_Z)
	amd64ALU_reg_reg32(cb, 0x09, amd64RDX, amd64RAX)
	// fall through

	// done:
	donePC := cb.Len()
	patchRel32(cb, done1Off, donePC)
	patchRel32(cb, done2Off, donePC)
	patchRel32(cb, done3Off, donePC)

	// Store FPSR
	amd64MOV_reg_mem(cb, amd64R11, amd64RSP, int32(amd64OffFPUPtr))
	amd64MOV_mem_reg32(cb, amd64R11, 68, amd64RDX)

	// Store result to integer rd
	if ji.rd != 0 {
		dstReg, mapped := ie64ToAMD64Reg(ji.rd)
		if mapped {
			amd64MOV_reg_reg(cb, dstReg, amd64RCX)
		} else {
			emitStoreSpilledRegAMD64(cb, amd64RCX, ji.rd)
		}
	}
}

// emitFCVTIF_AMD64 handles int -> float conversion using CVTSI2SS.
func emitFCVTIF_AMD64(cb *CodeBuffer, ji *JITInstr) {
	rsReg := resolveRegAMD64(cb, ji.rs, amd64RAX)
	amd64MOV_reg_reg32(cb, amd64RAX, rsReg) // EAX = int32(rs)
	// CVTSI2SS XMM0, EAX (F3 0F 2A /r)
	cb.EmitBytes(0xF3)
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x0F, 0x2A, modRM(3, 0, amd64RAX))
	amd64MOVD_reg_xmm(cb, amd64RAX, 0)
	emitStoreFPRegAMD64(cb, amd64RAX, ji.rd)
	emitSetFPCondCodesAMD64(cb)
}

// ===========================================================================
// FPU — Memory (FLOAD / FSTORE)
// ===========================================================================

func emitFLOAD_AMD64(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
	emitMemAddr(cb, ji) // address in RAX

	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, amd64RegIOStart)
	slowPathOff := amd64Jcc_rel32(cb, amd64CondAE)

	// Fast path: 32-bit load
	emitMemOpSIB(cb, false, 0x8B, amd64RDX, amd64RegMemBase, amd64RAX, 0) // MOV EDX, [RSI+RAX]
	doneOff := amd64JMP_rel32(cb)

	// Slow path
	slowPathPC := cb.Len()
	patchRel32(cb, slowPathOff, slowPathPC)

	amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
	amd64SHR_imm(cb, amd64RCX, 8)
	emitREX_SIB(cb, false, amd64RCX, amd64RCX, amd64RegIOBitmap)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, amd64RCX, 4), sibByte(0, amd64RCX, amd64RegIOBitmap))
	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	nonIOOff := amd64Jcc_rel32(cb, amd64CondE)

	emitIOBail(cb, instrPC, ji.pcOffset, br, writtenSoFar)

	nonIOPC := cb.Len()
	patchRel32(cb, nonIOOff, nonIOPC)
	emitMemOpSIB(cb, false, 0x8B, amd64RDX, amd64RegMemBase, amd64RAX, 0)

	donePC := cb.Len()
	patchRel32(cb, doneOff, donePC)

	// Store to FP register
	emitStoreFPRegAMD64(cb, amd64RDX, ji.rd)
	// Set condition codes from loaded value
	amd64MOV_reg_reg32(cb, amd64RAX, amd64RDX)
	emitSetFPCondCodesAMD64(cb)
}

func emitFSTORE_AMD64(cb *CodeBuffer, ji *JITInstr, instrPC uint32, br *blockRegs, writtenSoFar uint32) {
	emitMemAddr(cb, ji) // address in RAX

	// Load FP source value
	emitLoadFPRegAMD64(cb, amd64R10, ji.rd)

	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, amd64RegIOStart)
	slowPathOff := amd64Jcc_rel32(cb, amd64CondAE)

	// Fast path
	emitMemOpSIB(cb, false, 0x89, amd64R10, amd64RegMemBase, amd64RAX, 0)
	doneOff := amd64JMP_rel32(cb)

	// Slow path
	slowPathPC := cb.Len()
	patchRel32(cb, slowPathOff, slowPathPC)

	amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
	amd64SHR_imm(cb, amd64RCX, 8)
	emitREX_SIB(cb, false, amd64RCX, amd64RCX, amd64RegIOBitmap)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, amd64RCX, 4), sibByte(0, amd64RCX, amd64RegIOBitmap))
	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	nonIOOff := amd64Jcc_rel32(cb, amd64CondE)

	emitIOBail(cb, instrPC, ji.pcOffset, br, writtenSoFar)

	nonIOPC := cb.Len()
	patchRel32(cb, nonIOOff, nonIOPC)
	emitMemOpSIB(cb, false, 0x89, amd64R10, amd64RegMemBase, amd64RAX, 0)

	donePC := cb.Len()
	patchRel32(cb, doneOff, donePC)
}
