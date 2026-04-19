// jit_6502_emit_amd64.go - x86-64 native code emitter for 6502 JIT compiler

//go:build amd64 && (linux || windows)

package main

// ===========================================================================
// x86-64 Register Mapping for 6502 JIT
// ===========================================================================
//
// x86-64  6502  Purpose
// ------  ----  -------
// RBX     A     Accumulator (callee-saved, low byte BL)
// RBP     X     X index register (callee-saved, low byte BPL)
// R12     Y     Y index register (callee-saved, low byte R12B)
// R13     SP    Stack pointer (callee-saved, low byte R13B)
// R14     PC    Program counter (callee-saved, low 16 bits R14W)
// R15     SR    Status register (callee-saved, low byte R15B)
// RSI     --    memDirect base pointer
// RDI     --    JIT6502Context pointer (saved to [RSP+0] in prologue)
// R8      --    ioPageBitmap pointer
// R9      --    (free — was FastPathLimit, now using directPageBitmap on stack)
// RAX     --    Scratch
// RCX     --    Scratch
// RDX     --    Scratch
// R10     --    Scratch
// R11     --    Scratch

// Dedicated register assignments for 6502 JIT
const (
	j65RegA        = amd64RBX // Accumulator (callee-saved)
	j65RegX        = amd64RBP // X index (callee-saved)
	j65RegY        = amd64R12 // Y index (callee-saved)
	j65RegSP       = amd64R13 // Stack pointer (callee-saved)
	j65RegPC       = amd64R14 // Program counter (callee-saved)
	j65RegSR       = amd64R15 // Status register (callee-saved)
	j65RegMem      = amd64RSI // memDirect base pointer
	j65RegCtx      = amd64RDI // JIT6502Context pointer (on entry; saved to stack)
	j65RegIO       = amd64R8  // ioPageBitmap pointer
	j65RegLimit    = amd64R9  // (free — was FastPathLimit, now unused)
	j65RegScratch  = amd64RAX // General scratch
	j65RegScratch2 = amd64RCX // Scratch (also shift count CL)
	j65RegScratch3 = amd64RDX // Scratch
	j65RegScratch4 = amd64R10 // Scratch
	j65RegScratch5 = amd64R11 // Scratch
)

// Stack frame layout:
// 6 callee-saved pushes (48 bytes) + return address (8) = 56 bytes.
// SUB RSP, 40 → total 96 = 16-byte aligned.
const (
	j65FrameSize     = 40 // SUB RSP, 40 (makes total stack 96 = 16-byte aligned)
	j65OffCtxPtr     = 0  // [RSP+0] = saved JIT6502Context pointer (8 bytes)
	j65OffCpuPtr     = 8  // [RSP+8] = CpuPtr (from context) (8 bytes)
	j65OffCycles     = 16 // [RSP+16] = cycle accumulator (uint32)
	j65OffLoopCount  = 20 // [RSP+20] = backward branch budget counter (uint32)
	j65OffCodePage   = 24 // [RSP+24] = CodePageBitmapPtr (from context) (8 bytes)
	j65OffDirectPage = 32 // [RSP+32] = DirectPageBitmapPtr (from context) (8 bytes)
)

// amd64MOVZX_W_mem emits MOVZX dst32, WORD [base + disp].
func amd64MOVZX_W_mem(cb *CodeBuffer, dst, base byte, disp int32) {
	emitREX(cb, false, dst, base)
	cb.EmitBytes(0x0F, 0xB7)
	baseBits := regBits(base)
	needsSIB := baseBits == 4
	if disp == 0 && baseBits != 5 {
		if needsSIB {
			cb.EmitBytes(modRM(0, dst, 4), sibByte(0, 4, base))
		} else {
			cb.EmitBytes(modRM(0, dst, base))
		}
	} else if disp >= -128 && disp <= 127 {
		if needsSIB {
			cb.EmitBytes(modRM(1, dst, 4), sibByte(0, 4, base), byte(int8(disp)))
		} else {
			cb.EmitBytes(modRM(1, dst, base), byte(int8(disp)))
		}
	} else {
		if needsSIB {
			cb.EmitBytes(modRM(2, dst, 4), sibByte(0, 4, base))
		} else {
			cb.EmitBytes(modRM(2, dst, base))
		}
		cb.Emit32(uint32(disp))
	}
}

// amd64MOVZX_B_memSIB emits MOVZX dst32, BYTE [base + index*1].
func amd64MOVZX_B_memSIB(cb *CodeBuffer, dst, base, index byte) {
	emitREX_SIB(cb, false, dst, index, base)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, dst, 4), sibByte(0, index, base))
}

// emitREXForByte emits a REX prefix for byte register operations.
// Unlike emitREX, this always emits REX when reg or rm is 4-7 to access
// SPL/BPL/SIL/DIL instead of AH/CH/DH/BH.
func emitREXForByte(cb *CodeBuffer, reg, rm byte) {
	r := isExtReg(reg)
	b := isExtReg(rm)
	needsREX := r || b || (reg >= 4 && reg <= 7) || (rm >= 4 && rm <= 7)
	if needsREX {
		cb.EmitBytes(rexByte(false, r, false, b))
	}
}

// emitREXForByteSIB emits a REX prefix for byte SIB operations.
func emitREXForByteSIB(cb *CodeBuffer, reg, index, base byte) {
	r := isExtReg(reg)
	x := isExtReg(index)
	b := isExtReg(base)
	needsREX := r || x || b || (reg >= 4 && reg <= 7)
	if needsREX {
		cb.EmitBytes(rexByte(false, r, x, b))
	}
}

// amd64MOV_memSIB_reg8 emits MOV BYTE [base + index*1], src8.
func amd64MOV_memSIB_reg8(cb *CodeBuffer, base, index, src byte) {
	emitREXForByteSIB(cb, src, index, base)
	cb.EmitBytes(0x88, modRM(0, src, 4), sibByte(0, index, base))
}

// amd64MOV_mem8 emits MOV BYTE [base + disp], src8.
func amd64MOV_mem8(cb *CodeBuffer, base byte, disp int32, src byte) {
	emitREXForByte(cb, src, base)
	cb.EmitBytes(0x88)
	baseBits := regBits(base)
	needsSIB := baseBits == 4
	if disp == 0 && baseBits != 5 {
		if needsSIB {
			cb.EmitBytes(modRM(0, src, 4), sibByte(0, 4, base))
		} else {
			cb.EmitBytes(modRM(0, src, base))
		}
	} else if disp >= -128 && disp <= 127 {
		if needsSIB {
			cb.EmitBytes(modRM(1, src, 4), sibByte(0, 4, base), byte(int8(disp)))
		} else {
			cb.EmitBytes(modRM(1, src, base), byte(int8(disp)))
		}
	} else {
		if needsSIB {
			cb.EmitBytes(modRM(2, src, 4), sibByte(0, 4, base))
		} else {
			cb.EmitBytes(modRM(2, src, base))
		}
		cb.Emit32(uint32(disp))
	}
}

// amd64CMP_reg_imm32 emits CMP reg32, imm32 (32-bit comparison).
func amd64CMP_reg_imm32(cb *CodeBuffer, dst byte, imm int32) {
	amd64ALU_reg_imm32_32bit(cb, 7, dst, imm) // /7 = CMP
}

// amd64TEST_reg_imm32 emits TEST reg32, imm32.
func amd64TEST_reg_imm32(cb *CodeBuffer, dst byte, imm uint32) {
	if dst == amd64RAX {
		// TEST EAX, imm32 has a short encoding: A9 imm32
		cb.EmitBytes(0xA9)
		cb.Emit32(imm)
		return
	}
	emitREX(cb, false, 0, dst)
	cb.EmitBytes(0xF7, modRM(3, 0, dst))
	cb.Emit32(imm)
}

// amd64ADD_mem32_imm32 emits ADD DWORD [base + disp], imm32.
func amd64ADD_mem32_imm32(cb *CodeBuffer, base byte, disp int32, imm int32) {
	emitMemOp(cb, false, 0x81, 0, base, disp) // /0 = ADD
	if imm >= -128 && imm <= 127 {
		// Re-encode with imm8 form
		// Actually, we already emitted 0x81... let me use the correct form
	}
	cb.Emit32(uint32(imm))
}

// amd64ADD_mem32_imm8 emits ADD DWORD [base + disp], imm8 (sign-extended).
func amd64ADD_mem32_imm8(cb *CodeBuffer, base byte, disp int32, imm int8) {
	emitMemOp(cb, false, 0x83, 0, base, disp) // /0 = ADD, imm8 form
	cb.EmitBytes(byte(imm))
}

// amd64INC_mem32 emits INC DWORD [base + disp].
func amd64INC_mem32(cb *CodeBuffer, base byte, disp int32) {
	emitMemOp(cb, false, 0xFF, 0, base, disp) // /0 = INC
}

// amd64MOV_mem32_imm32 is an alias for the existing amd64MOV_mem_imm32.
// (It emits MOV DWORD [base + disp], imm32.)

// amd64CMP_mem8_imm8 emits CMP BYTE [base + index*1], imm8.
func amd64CMP_memSIB8_imm8(cb *CodeBuffer, base, index byte, imm byte) {
	emitREX_SIB(cb, false, 0, index, base)
	cb.EmitBytes(0x80, modRM(0, 7, 4), sibByte(0, index, base), imm) // /7 = CMP
}

// ===========================================================================
// Prologue / Epilogue
// ===========================================================================

// emit6502Prologue emits the block entry sequence for a 6502 JIT block.
func emit6502Prologue(cb *CodeBuffer, hasBackwardBranch bool) {
	// Save callee-saved registers
	amd64PUSH(cb, amd64RBX)
	amd64PUSH(cb, amd64RBP)
	amd64PUSH(cb, amd64R12)
	amd64PUSH(cb, amd64R13)
	amd64PUSH(cb, amd64R14)
	amd64PUSH(cb, amd64R15)

	// Allocate stack frame
	amd64ALU_reg_imm32(cb, 5, amd64RSP, int32(j65FrameSize)) // SUB RSP, 40

	// Save JIT6502Context pointer
	amd64MOV_mem_reg(cb, amd64RSP, int32(j65OffCtxPtr), j65RegCtx) // [RSP+0] = RDI (ctx)

	// Load pointers from JIT6502Context
	amd64MOV_reg_mem(cb, j65RegMem, j65RegCtx, int32(j65CtxOffMemPtr))     // RSI = MemPtr
	amd64MOV_reg_mem(cb, j65RegIO, j65RegCtx, int32(j65CtxOffIOBitmapPtr)) // R8 = IOBitmapPtr

	// Save CpuPtr to stack
	amd64MOV_reg_mem(cb, amd64RAX, j65RegCtx, int32(j65CtxOffCpuPtr)) // RAX = CpuPtr
	amd64MOV_mem_reg(cb, amd64RSP, int32(j65OffCpuPtr), amd64RAX)     // [RSP+8] = CpuPtr

	// Save CodePageBitmapPtr to stack
	amd64MOV_reg_mem(cb, amd64RCX, j65RegCtx, int32(j65CtxOffCodePageBitmap))
	amd64MOV_mem_reg(cb, amd64RSP, int32(j65OffCodePage), amd64RCX) // [RSP+24] = CodePageBitmapPtr

	// Save DirectPageBitmapPtr to stack
	amd64MOV_reg_mem(cb, amd64RCX, j65RegCtx, int32(j65CtxOffDirectPageBitmapPtr))
	amd64MOV_mem_reg(cb, amd64RSP, int32(j65OffDirectPage), amd64RCX) // [RSP+32] = DirectPageBitmapPtr

	// Zero cycle accumulator
	amd64MOV_mem_imm32(cb, amd64RSP, int32(j65OffCycles), 0) // [RSP+16] = 0

	// Zero backward branch budget counter if needed
	if hasBackwardBranch {
		amd64MOV_mem_imm32(cb, amd64RSP, int32(j65OffLoopCount), 0)
	}

	// Load 6502 registers from CPU struct (via RAX = CpuPtr)
	amd64MOVZX_B_mem(cb, j65RegA, amd64RAX, int32(cpu6502OffA))   // RBX = A (zero-extended)
	amd64MOVZX_B_mem(cb, j65RegX, amd64RAX, int32(cpu6502OffX))   // RBP = X
	amd64MOVZX_B_mem(cb, j65RegY, amd64RAX, int32(cpu6502OffY))   // R12 = Y
	amd64MOVZX_B_mem(cb, j65RegSP, amd64RAX, int32(cpu6502OffSP)) // R13 = SP
	amd64MOVZX_W_mem(cb, j65RegPC, amd64RAX, int32(cpu6502OffPC)) // R14 = PC (16-bit)
	amd64MOVZX_B_mem(cb, j65RegSR, amd64RAX, int32(cpu6502OffSR)) // R15 = SR
}

// emit6502Epilogue emits the block normal-exit sequence.
// retPC is the PC value after the block, instrCount is instructions retired.
func emit6502Epilogue(cb *CodeBuffer, retPC uint32, instrCount uint32) {
	// Store 6502 registers back to CPU struct
	amd64MOV_reg_mem(cb, amd64RAX, amd64RSP, int32(j65OffCpuPtr)) // RAX = CpuPtr

	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffA), j65RegA)   // cpu.A = BL
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffX), j65RegX)   // cpu.X = BPL
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffY), j65RegY)   // cpu.Y = R12B
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffSP), j65RegSP) // cpu.SP = R13B
	// PC: store as 16-bit word
	emit6502StorePC(cb, amd64RAX)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffSR), j65RegSR) // cpu.SR = R15B

	// Write RetPC, RetCount, RetCycles to context
	amd64MOV_reg_mem(cb, amd64RCX, amd64RSP, int32(j65OffCtxPtr)) // RCX = ctx

	amd64MOV_mem_imm32(cb, amd64RCX, int32(j65CtxOffRetPC), retPC)
	amd64MOV_mem_imm32(cb, amd64RCX, int32(j65CtxOffRetCount), instrCount)

	// Flush cycle accumulator: ctx.RetCycles = [RSP+16] (zero-extended to 64-bit)
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(j65OffCycles))     // EAX = cycles (32-bit)
	amd64MOV_mem_reg(cb, amd64RCX, int32(j65CtxOffRetCycles), amd64RAX) // ctx.RetCycles = RAX (zero-ext)

	// Deallocate stack frame
	amd64ALU_reg_imm32(cb, 0, amd64RSP, int32(j65FrameSize)) // ADD RSP, 40

	// Restore callee-saved registers (reverse order)
	amd64POP(cb, amd64R15)
	amd64POP(cb, amd64R14)
	amd64POP(cb, amd64R13)
	amd64POP(cb, amd64R12)
	amd64POP(cb, amd64RBP)
	amd64POP(cb, amd64RBX)

	amd64RET(cb)
}

// emit6502StorePC stores R14W (PC) as a 16-bit word at [RAX + cpu6502OffPC].
func emit6502StorePC(cb *CodeBuffer, baseReg byte) {
	// MOV WORD [base + 0], R14W  — need 66h prefix for 16-bit operand
	cb.EmitBytes(0x66) // operand size override
	emitREX(cb, false, j65RegPC, baseReg)
	cb.EmitBytes(0x89)
	baseBits := regBits(baseReg)
	if cpu6502OffPC == 0 && baseBits != 5 {
		if baseBits == 4 {
			cb.EmitBytes(modRM(0, j65RegPC, 4), sibByte(0, 4, baseReg))
		} else {
			cb.EmitBytes(modRM(0, j65RegPC, baseReg))
		}
	} else {
		if baseBits == 4 {
			cb.EmitBytes(modRM(1, j65RegPC, 4), sibByte(0, 4, baseReg), byte(cpu6502OffPC))
		} else {
			cb.EmitBytes(modRM(1, j65RegPC, baseReg), byte(cpu6502OffPC))
		}
	}
}

// emit6502BailEpilogue emits the NeedBail exit path. This stores all registers back,
// sets RetPC to instrPC (re-execute this instruction), RetCount to instrsSoFar,
// flushes cycles, sets NeedBail=1, and returns.
func emit6502BailEpilogue(cb *CodeBuffer, instrPC uint32, instrsSoFar uint32, pendingCycles uint32, nzPending bool, nzReg byte) {
	// Flush any pending cycles to the accumulator
	if pendingCycles > 0 {
		if pendingCycles <= 127 {
			amd64ADD_mem32_imm8(cb, amd64RSP, int32(j65OffCycles), int8(pendingCycles))
		} else {
			amd64ADD_mem32_imm32(cb, amd64RSP, int32(j65OffCycles), int32(pendingCycles))
		}
	}

	// Store registers back
	amd64MOV_reg_mem(cb, amd64RAX, amd64RSP, int32(j65OffCpuPtr))
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffA), j65RegA)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffX), j65RegX)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffY), j65RegY)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffSP), j65RegSP)
	emit6502StorePC(cb, amd64RAX)
	// Materialize deferred N/Z before storing SR
	if nzPending {
		emit6502UpdateNZ(cb, nzReg)
	}
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffSR), j65RegSR)

	// Set NeedBail, RetPC, RetCount (merged with ChainCount), RetCycles in context
	amd64MOV_reg_mem(cb, amd64RCX, amd64RSP, int32(j65OffCtxPtr))
	amd64MOV_mem_imm32(cb, amd64RCX, int32(j65CtxOffNeedBail), 1)
	amd64MOV_mem_imm32(cb, amd64RCX, int32(j65CtxOffRetPC), instrPC)
	// RetCount = ChainCount + instrsSoFar (merge prior chained count)
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RCX, int32(j65CtxOffChainCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrsSoFar)) // ADD EAX, instrsSoFar
	amd64MOV_mem_reg32(cb, amd64RCX, int32(j65CtxOffRetCount), amd64RAX)

	// Flush cycle accumulator
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(j65OffCycles))
	amd64MOV_mem_reg(cb, amd64RCX, int32(j65CtxOffRetCycles), amd64RAX)

	// Stack cleanup and return
	amd64ALU_reg_imm32(cb, 0, amd64RSP, int32(j65FrameSize))
	amd64POP(cb, amd64R15)
	amd64POP(cb, amd64R14)
	amd64POP(cb, amd64R13)
	amd64POP(cb, amd64R12)
	amd64POP(cb, amd64RBP)
	amd64POP(cb, amd64RBX)
	amd64RET(cb)
}

// emit6502InvalEpilogue emits the NeedInval exit path. The store has already
// completed, so RetPC is the NEXT instruction's PC, RetCount includes this instruction,
// and cycles include this instruction's cost.
func emit6502InvalEpilogue(cb *CodeBuffer, nextPC uint32, instrsSoFar uint32, pendingCycles uint32, nzPending bool, nzReg byte) {
	// Flush pending cycles
	if pendingCycles > 0 {
		if pendingCycles <= 127 {
			amd64ADD_mem32_imm8(cb, amd64RSP, int32(j65OffCycles), int8(pendingCycles))
		} else {
			amd64ADD_mem32_imm32(cb, amd64RSP, int32(j65OffCycles), int32(pendingCycles))
		}
	}

	// Store registers back
	amd64MOV_reg_mem(cb, amd64RAX, amd64RSP, int32(j65OffCpuPtr))
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffA), j65RegA)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffX), j65RegX)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffY), j65RegY)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffSP), j65RegSP)
	emit6502StorePC(cb, amd64RAX)
	// Materialize deferred N/Z before storing SR
	if nzPending {
		emit6502UpdateNZ(cb, nzReg)
	}
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffSR), j65RegSR)

	// Set NeedInval, RetPC, RetCount (merged with ChainCount), RetCycles in context
	amd64MOV_reg_mem(cb, amd64RCX, amd64RSP, int32(j65OffCtxPtr))
	amd64MOV_mem_imm32(cb, amd64RCX, int32(j65CtxOffNeedInval), 1)
	amd64MOV_mem_imm32(cb, amd64RCX, int32(j65CtxOffRetPC), nextPC)
	// RetCount = ChainCount + instrsSoFar (merge prior chained count)
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RCX, int32(j65CtxOffChainCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrsSoFar)) // ADD EAX, instrsSoFar
	amd64MOV_mem_reg32(cb, amd64RCX, int32(j65CtxOffRetCount), amd64RAX)

	// Flush cycle accumulator
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(j65OffCycles))
	amd64MOV_mem_reg(cb, amd64RCX, int32(j65CtxOffRetCycles), amd64RAX)

	// Stack cleanup and return
	amd64ALU_reg_imm32(cb, 0, amd64RSP, int32(j65FrameSize))
	amd64POP(cb, amd64R15)
	amd64POP(cb, amd64R14)
	amd64POP(cb, amd64R13)
	amd64POP(cb, amd64R12)
	amd64POP(cb, amd64RBP)
	amd64POP(cb, amd64RBX)
	amd64RET(cb)
}

// ===========================================================================
// Block Chaining: Chain Entry / Chain Exit / Unchained Exit
// ===========================================================================

// j65ChainExitInfo records a chain exit point for later patching.
type j65ChainExitInfo struct {
	targetPC      uint32 // 6502 PC this exit targets
	jmpDispOffset int    // offset within CodeBuffer of the JMP rel32 displacement
}

// emit6502ChainEntry emits the lightweight chain entry point. Chained blocks
// JMP directly here, skipping the full prologue. Since all 6502 state lives in
// callee-saved registers, no register loads are needed.
func emit6502ChainEntry(cb *CodeBuffer, hasBackwardBranch bool) int {
	entryOff := cb.Len()
	if hasBackwardBranch {
		amd64MOV_mem_imm32(cb, amd64RSP, int32(j65OffLoopCount), 0)
	}
	return entryOff
}

// emit6502ChainExit emits a chain exit sequence for a block terminator with a
// statically known target PC. The sequence:
//  1. Flush pending cycles
//  2. Accumulate instruction count into ChainCount
//  3. Decrement ChainBudget; if exhausted → unchained exit
//  4. Check NeedInval; if set → unchained exit
//  5. Patchable JMP rel32 (initially to unchained exit)
//  6. Unchained exit: store regs, set RetPC/RetCount, full pop/ret
func emit6502ChainExit(cb *CodeBuffer, targetPC uint32, instrCount uint32, pendingCycles *uint32, nzPending bool, nzReg byte) j65ChainExitInfo {
	flushPendingCycles(cb, pendingCycles)

	// Load ctx pointer
	amd64MOV_reg_mem(cb, amd64RCX, amd64RSP, int32(j65OffCtxPtr))

	// Accumulate instruction count: ChainCount += instrCount
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RCX, int32(j65CtxOffChainCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrCount)) // ADD EAX, instrCount
	amd64MOV_mem_reg32(cb, amd64RCX, int32(j65CtxOffChainCount), amd64RAX)

	// Materialize deferred N/Z before budget/inval checks.
	// Covers both the chained path (next block reads R15) and unchained path (stores R15).
	if nzPending {
		emit6502UpdateNZ(cb, nzReg)
	}

	// DEC DWORD [RCX + ChainBudget]
	amd64DEC_mem32(cb, amd64RCX, int32(j65CtxOffChainBudget))

	// JLE .unchained (budget exhausted — signed, now <= 0)
	unchainedOff1 := amd64Jcc_rel32(cb, amd64CondLE)

	// CMP DWORD [RCX + NeedInval], 0
	amd64ALU_mem_imm8(cb, 7, amd64RCX, int32(j65CtxOffNeedInval), 0) // /7 = CMP
	// JNE .unchained (self-mod detected)
	unchainedOff2 := amd64Jcc_rel32(cb, amd64CondNE)

	// Patchable JMP rel32 — initially jumps to .unchained
	jmpOff := cb.Len()
	cb.EmitBytes(0xE9, 0, 0, 0, 0) // JMP rel32 (placeholder)
	jmpDispOffset := jmpOff + 1    // displacement starts after opcode byte

	// .unchained label
	unchainedLabel := cb.Len()
	patchRel32(cb, unchainedOff1, unchainedLabel)
	patchRel32(cb, unchainedOff2, unchainedLabel)
	patchRel32(cb, jmpDispOffset, unchainedLabel) // initial target = unchained

	// Emit unchained exit
	emit6502UnchainedExitImm(cb, targetPC, false, 0) // NZ already materialized above

	return j65ChainExitInfo{
		targetPC:      targetPC,
		jmpDispOffset: jmpDispOffset,
	}
}

// emit6502UnchainedExitImm emits a full exit sequence with a static RetPC.
// Stores all 6502 registers to CPU struct, writes RetPC/RetCount/RetCycles, returns.
func emit6502UnchainedExitImm(cb *CodeBuffer, retPC uint32, nzPending bool, nzReg byte) {
	// Materialize deferred N/Z before storing SR
	if nzPending {
		emit6502UpdateNZ(cb, nzReg)
	}
	// Store registers back to CPU struct
	amd64MOV_reg_mem(cb, amd64RAX, amd64RSP, int32(j65OffCpuPtr))
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffA), j65RegA)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffX), j65RegX)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffY), j65RegY)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffSP), j65RegSP)
	emit6502StorePC(cb, amd64RAX)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffSR), j65RegSR)

	// Write context fields: RetPC, RetCount from ChainCount, RetCycles
	amd64MOV_reg_mem(cb, amd64RCX, amd64RSP, int32(j65OffCtxPtr))
	amd64MOV_mem_imm32(cb, amd64RCX, int32(j65CtxOffRetPC), retPC)
	// RetCount = ChainCount (already accumulated)
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RCX, int32(j65CtxOffChainCount))
	amd64MOV_mem_reg32(cb, amd64RCX, int32(j65CtxOffRetCount), amd64RAX)
	// RetCycles from accumulator
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(j65OffCycles))
	amd64MOV_mem_reg(cb, amd64RCX, int32(j65CtxOffRetCycles), amd64RAX)

	// Full stack cleanup and return
	amd64ALU_reg_imm32(cb, 0, amd64RSP, int32(j65FrameSize))
	amd64POP(cb, amd64R15)
	amd64POP(cb, amd64R14)
	amd64POP(cb, amd64R13)
	amd64POP(cb, amd64R12)
	amd64POP(cb, amd64RBP)
	amd64POP(cb, amd64RBX)
	amd64RET(cb)
}

// emit6502UnchainedExitReg emits a full exit with RetPC from a host register (e.g. R10D).
// Used by RTS and JMP indirect where the target PC is computed at runtime.
func emit6502UnchainedExitReg(cb *CodeBuffer, pcReg byte, nzPending bool, nzReg byte) {
	// Materialize deferred N/Z before storing SR
	if nzPending {
		emit6502UpdateNZ(cb, nzReg)
	}
	// Store registers back to CPU struct
	amd64MOV_reg_mem(cb, amd64RAX, amd64RSP, int32(j65OffCpuPtr))
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffA), j65RegA)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffX), j65RegX)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffY), j65RegY)
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffSP), j65RegSP)
	// Store PC from pcReg as 16-bit word
	cb.EmitBytes(0x66) // operand size prefix
	emitREX(cb, false, pcReg, amd64RAX)
	cb.EmitBytes(0x89, modRM(0, pcReg, amd64RAX)) // MOV WORD [RAX], pcRegW
	amd64MOV_mem8(cb, amd64RAX, int32(cpu6502OffSR), j65RegSR)

	// Write context fields
	amd64MOV_reg_mem(cb, amd64RCX, amd64RSP, int32(j65OffCtxPtr))
	// RetPC from pcReg
	amd64MOV_mem_reg32(cb, amd64RCX, int32(j65CtxOffRetPC), pcReg)
	// RetCount = ChainCount
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RCX, int32(j65CtxOffChainCount))
	amd64MOV_mem_reg32(cb, amd64RCX, int32(j65CtxOffRetCount), amd64RAX)
	// RetCycles
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(j65OffCycles))
	amd64MOV_mem_reg(cb, amd64RCX, int32(j65CtxOffRetCycles), amd64RAX)

	// Full stack cleanup and return
	amd64ALU_reg_imm32(cb, 0, amd64RSP, int32(j65FrameSize))
	amd64POP(cb, amd64R15)
	amd64POP(cb, amd64R14)
	amd64POP(cb, amd64R13)
	amd64POP(cb, amd64R12)
	amd64POP(cb, amd64RBP)
	amd64POP(cb, amd64RBX)
	amd64RET(cb)
}

// ===========================================================================
// Cycle Batching
// ===========================================================================

// flushPendingCycles emits an ADD to the cycle accumulator if there are pending cycles.
func flushPendingCycles(cb *CodeBuffer, pending *uint32) {
	if *pending == 0 {
		return
	}
	if *pending <= 127 {
		amd64ADD_mem32_imm8(cb, amd64RSP, int32(j65OffCycles), int8(*pending))
	} else {
		amd64ADD_mem32_imm32(cb, amd64RSP, int32(j65OffCycles), int32(*pending))
	}
	*pending = 0
}

// ===========================================================================
// Instruction Emission Helpers
// ===========================================================================

// emit6502NOP emits native code for 6502 NOP ($EA).
func emit6502NOP(_ *CodeBuffer) {
	// NOP produces no native code — PC advancement and cycles are handled externally
}

// amd64SHR_imm32 emits SHR dst32, imm8 (32-bit shift right).
func amd64SHR_imm32(cb *CodeBuffer, dst byte, imm8 byte) {
	emitREX(cb, false, 0, dst)
	cb.EmitBytes(0xC1, modRM(3, 5, dst), imm8)
}

// amd64OR_reg_imm32 emits OR r32, imm32.
func amd64OR_reg_imm32_32bit(cb *CodeBuffer, dst byte, imm int32) {
	amd64ALU_reg_imm32_32bit(cb, 1, dst, imm) // /1 = OR
}

// amd64AND_reg_imm32 emits AND r32, imm32.
func amd64AND_reg_imm32_32bit(cb *CodeBuffer, dst byte, imm int32) {
	amd64ALU_reg_imm32_32bit(cb, 4, dst, imm) // /4 = AND
}

// amd64ADD_reg_reg32 emits ADD dst32, src32.
func amd64ADD_reg_reg32(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, false, src, dst)
	cb.EmitBytes(0x01, modRM(3, src, dst))
}

// amd64LEA_reg_memDisp32 emits LEA dst32, [base + disp32] (32-bit).
func amd64LEA_reg_memDisp32(cb *CodeBuffer, dst, base byte, disp int32) {
	emitMemOp(cb, false, 0x8D, dst, base, disp)
}

// amd64SHL_imm32 emits SHL dst32, imm8.
func amd64SHL_imm32(cb *CodeBuffer, dst byte, imm8 byte) {
	emitREX(cb, false, 0, dst)
	cb.EmitBytes(0xC1, modRM(3, 4, dst), imm8)
}

// amd64XOR_reg_reg32_local emits XOR r32, r32. (Used to compute page cross.)
func amd64XOR_reg_reg32_local(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, false, src, dst)
	cb.EmitBytes(0x31, modRM(3, src, dst))
}

// ===========================================================================
// N/Z Flag Update Helper
// ===========================================================================

// emit6502UpdateNZ clears N,Z in SR (R15) and sets them from the result byte
// in resultReg. resultReg must hold a zero-extended byte value (upper bits = 0).
// Clobbers: none (uses TEST which doesn't modify operands).
func emit6502UpdateNZ(cb *CodeBuffer, resultReg byte) {
	// AND R15D, 0x7D  — clear N (0x80) and Z (0x02)
	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0x7D)

	// TEST result32, result32 (upper bits are zero, so this tests the byte)
	amd64TEST_reg_reg32(cb, resultReg, resultReg)

	// JNZ .not_zero
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)

	// OR R15D, 0x02  — set Z flag
	amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x02)

	// .not_zero:
	patchRel32(cb, notZeroOff, cb.Len())

	// TEST result32, 0x80  — check bit 7 for N flag
	amd64TEST_reg_imm32(cb, resultReg, 0x80)

	// JZ .not_neg
	notNegOff := amd64Jcc_rel32(cb, amd64CondE)

	// OR R15D, 0x80  — set N flag
	amd64OR_reg_imm32_32bit(cb, j65RegSR, int32(NEGATIVE_FLAG))

	// .not_neg:
	patchRel32(cb, notNegOff, cb.Len())
}

// ===========================================================================
// Lazy N/Z Flag Tracking
// ===========================================================================

// j65NZState tracks deferred N/Z flag computation at compile time.
// When nzPending is true, the N and Z bits in R15 (j65RegSR) are stale;
// the authoritative byte result lives in nzReg. Materialization writes
// N/Z into R15 from nzReg. C and V in R15 are always current.
type j65NZState struct {
	nzPending bool // true if N/Z in R15 are stale
	nzReg     byte // callee-saved host register holding the byte result
}

// j65SetNZPending marks N/Z as deferred from the given callee-saved register.
func j65SetNZPending(nz *j65NZState, reg byte) {
	nz.nzPending = true
	nz.nzReg = reg
}

// j65MaterializeNZ emits code to flush deferred N/Z into R15.
// If no flags are pending, emits nothing.
func j65MaterializeNZ(cb *CodeBuffer, nz *j65NZState) {
	if !nz.nzPending {
		return
	}
	emit6502UpdateNZ(cb, nz.nzReg)
	nz.nzPending = false
	nz.nzReg = 0
}

// ===========================================================================
// Fast-Path Memory Check
// ===========================================================================

// emit6502FullFastPathCheck emits the fast-path check for addr in EAX using
// the directPageBitmap. If directPageBitmap[addr>>8] != 0, jumps to bail.
// Returns a single offset that needs to be patched to the bail target.
// Clobbers: ECX, R9. Does NOT clobber RDX (preserved for page-cross checks).
func emit6502FullFastPathCheck(cb *CodeBuffer) int {
	// ECX = addr >> 8 (page number)
	amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
	amd64SHR_imm32(cb, amd64RCX, 8)

	// R9 = directPageBitmap pointer (from stack). R9 is free since FastPathLimit was removed.
	amd64MOV_reg_mem(cb, amd64R9, amd64RSP, int32(j65OffDirectPage))

	// MOVZX ECX, BYTE [R9 + RCX]
	amd64MOVZX_B_memSIB(cb, amd64RCX, amd64R9, amd64RCX)

	// TEST ECX, ECX
	amd64TEST_reg_reg32(cb, amd64RCX, amd64RCX)

	// JNZ bail
	return amd64Jcc_rel32(cb, amd64CondNE)
}

// emit6502ZPPageCheck emits the ioPageBitmap check for a specific ZP page (0).
// Returns the offset to be patched to the bail target.
func emit6502ZPPageCheck(cb *CodeBuffer, page byte) int {
	amd64MOVZX_B_mem(cb, amd64RCX, j65RegIO, int32(page))
	amd64TEST_reg_reg32(cb, amd64RCX, amd64RCX)
	return amd64Jcc_rel32(cb, amd64CondNE) // JNZ bail
}

// ===========================================================================
// Addressing Mode Helpers
// ===========================================================================
// Each helper computes the effective address in EAX.
// For ZP modes, the result is always < 0x100 (fast path guaranteed if page 0 ok).
// For absolute/indexed modes, the caller must do a fast-path check.

// emit6502AddrZP computes zero page address: EAX = operand (0-255).
func emit6502AddrZP(cb *CodeBuffer, operand byte) {
	amd64MOV_reg_imm32(cb, amd64RAX, uint32(operand))
}

// emit6502AddrZPX computes zero page + X: EAX = (operand + X) & 0xFF.
func emit6502AddrZPX(cb *CodeBuffer, operand byte) {
	amd64LEA_reg_memDisp32(cb, amd64RAX, j65RegX, int32(operand))
	amd64AND_reg_imm32_32bit(cb, amd64RAX, 0xFF)
}

// emit6502AddrZPY computes zero page + Y: EAX = (operand + Y) & 0xFF.
func emit6502AddrZPY(cb *CodeBuffer, operand byte) {
	amd64LEA_reg_memDisp32(cb, amd64RAX, j65RegY, int32(operand))
	amd64AND_reg_imm32_32bit(cb, amd64RAX, 0xFF)
}

// emit6502AddrAbs computes absolute address: EAX = operand16.
func emit6502AddrAbs(cb *CodeBuffer, operand uint16) {
	amd64MOV_reg_imm32(cb, amd64RAX, uint32(operand))
}

// emit6502AddrAbsX computes absolute + X: EAX = (operand + X) & 0xFFFF.
// Also saves base in EDX for page-cross detection.
func emit6502AddrAbsX(cb *CodeBuffer, operand uint16) {
	amd64MOV_reg_imm32(cb, amd64RAX, uint32(operand))
	amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)     // EDX = base (for page-cross check)
	amd64ADD_reg_reg32(cb, amd64RAX, j65RegX)      // EAX = base + X
	amd64AND_reg_imm32_32bit(cb, amd64RAX, 0xFFFF) // mask to 16-bit
}

// emit6502AddrAbsY computes absolute + Y: EAX = (operand + Y) & 0xFFFF.
// Also saves base in EDX for page-cross detection.
func emit6502AddrAbsY(cb *CodeBuffer, operand uint16) {
	amd64MOV_reg_imm32(cb, amd64RAX, uint32(operand))
	amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX) // EDX = base
	amd64ADD_reg_reg32(cb, amd64RAX, j65RegY)  // EAX = base + Y
	amd64AND_reg_imm32_32bit(cb, amd64RAX, 0xFFFF)
}

// emit6502AddrIndX computes (indirect,X): read 16-bit pointer from ZP[(operand+X)&FF].
// Result in EAX. Clobbers ECX, EDX.
func emit6502AddrIndX(cb *CodeBuffer, operand byte) {
	// Compute ZP pointer address: (operand + X) & 0xFF
	amd64LEA_reg_memDisp32(cb, amd64RCX, j65RegX, int32(operand))
	amd64AND_reg_imm32_32bit(cb, amd64RCX, 0xFF)

	// Read low byte of pointer from ZP
	amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RCX) // EAX = mem[zpAddr]

	// Read high byte: (zpAddr + 1) & 0xFF (ZP wrap)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RCX, 1) // ADD ECX, 1
	amd64AND_reg_imm32_32bit(cb, amd64RCX, 0xFF)
	amd64MOVZX_B_memSIB(cb, amd64RDX, j65RegMem, amd64RCX) // EDX = mem[(zpAddr+1)&FF]

	// Combine: EAX = low | (high << 8)
	amd64SHL_imm32(cb, amd64RDX, 8)
	amd64OR_reg_imm32_32bit(cb, amd64RAX, 0) // dummy — need OR EAX, EDX
	// Actually, OR r32, r32:
	emitREX(cb, false, amd64RDX, amd64RAX)
	cb.EmitBytes(0x09, modRM(3, amd64RDX, amd64RAX)) // OR EAX, EDX
}

// emit6502AddrIndY computes (indirect),Y: read 16-bit pointer from ZP[operand], add Y.
// Result in EAX. Base pointer (before adding Y) saved in EDX for page-cross check.
// Clobbers ECX, EDX.
func emit6502AddrIndY(cb *CodeBuffer, operand byte) {
	// Read low byte of pointer from ZP
	amd64MOVZX_B_mem(cb, amd64RAX, j65RegMem, int32(operand)) // EAX = mem[operand]

	// Read high byte: (operand + 1) & 0xFF
	highAddr := (uint32(operand) + 1) & 0xFF
	amd64MOVZX_B_mem(cb, amd64RDX, j65RegMem, int32(highAddr)) // EDX = mem[(operand+1)&FF]

	// Combine: EAX = low | (high << 8)
	amd64SHL_imm32(cb, amd64RDX, 8)
	emitREX(cb, false, amd64RDX, amd64RAX)
	cb.EmitBytes(0x09, modRM(3, amd64RDX, amd64RAX)) // OR EAX, EDX

	// Save base pointer for page-cross check
	amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX) // EDX = base pointer

	// Add Y
	amd64ADD_reg_reg32(cb, amd64RAX, j65RegY)
	amd64AND_reg_imm32_32bit(cb, amd64RAX, 0xFFFF)
}

// emit6502PageCrossCheck emits a conditional +1 cycle penalty if EAX and EDX
// differ in the high byte (page crossing). EDX must contain the base address.
func emit6502PageCrossCheck(cb *CodeBuffer) {
	// XOR EDX, EAX  — find differing bits
	amd64XOR_reg_reg32_local(cb, amd64RDX, amd64RAX)
	// TEST EDX, 0xFF00  — check if pages differ
	amd64TEST_reg_imm32(cb, amd64RDX, 0xFF00)
	// JZ .no_cross
	noCrossOff := amd64Jcc_rel32(cb, amd64CondE)
	// INC DWORD [RSP+16]  — +1 cycle
	amd64INC_mem32(cb, amd64RSP, int32(j65OffCycles))
	// .no_cross:
	patchRel32(cb, noCrossOff, cb.Len())
}

// ===========================================================================
// Self-Modification Detection (for stores)
// ===========================================================================

// emit6502SelfModCheck emits code page bitmap check after a store.
// Address must be in EAX. Returns offset to patch to the inval bail target.
// Clobbers: ECX, RDX.
func emit6502SelfModCheck(cb *CodeBuffer) int {
	amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX) // ECX = addr
	amd64SHR_imm32(cb, amd64RCX, 8)            // ECX = page number

	// Store page number to ctx.InvalPage (unconditional, cheap — needed for
	// page-granular invalidation if we actually trigger NeedInval)
	amd64MOV_reg_mem(cb, amd64RDX, amd64RSP, int32(j65OffCtxPtr))         // RDX = ctx
	amd64MOV_mem_reg32(cb, amd64RDX, int32(j65CtxOffInvalPage), amd64RCX) // ctx.InvalPage = page

	// Load CodePageBitmapPtr from stack
	amd64MOV_reg_mem(cb, amd64RDX, amd64RSP, int32(j65OffCodePage)) // RDX = bitmap ptr

	// CMP BYTE [RDX + RCX], 0
	amd64CMP_memSIB8_imm8(cb, amd64RDX, amd64RCX, 0)

	// JNE .inval  — if page has code, invalidate
	return amd64Jcc_rel32(cb, amd64CondNE)
}

// ===========================================================================
// Generic Load/Store Emitters
// ===========================================================================

// bailInfo records a deferred bail epilogue to be emitted at the end of the block.
type bailInfo struct {
	offsets       []int // code offsets that need patching to this bail
	instrPC       uint32
	instrIdx      int
	pendingCycles uint32
	nzPending     bool // lazy N/Z state at this bail point
	nzReg         byte // register holding deferred N/Z result
}

// invalInfo records a deferred NeedInval epilogue.
type invalInfo struct {
	offsets       []int
	nextPC        uint32 // next instruction's PC (store already completed)
	instrIdx      int    // including this instruction
	pendingCycles uint32 // including this instruction's cycles
	nzPending     bool   // lazy N/Z state at this inval point
	nzReg         byte   // register holding deferred N/Z result
}

// emit6502Load emits a load from the given addressing mode into dstReg.
// For modes that can bail (absolute, indexed), adds bail targets.
// For modes with page-crossing penalty (abs+X, abs+Y, (ind),Y on loads),
// emits the conditional +1 cycle check.
func emit6502Load(cb *CodeBuffer, dstReg byte, opcode byte, operand uint16,
	instrPC uint32, instrIdx int, pendingCycles uint32,
	bails *[]bailInfo, isLoadWithPageCross bool,
	nzPending bool, nzReg byte) {

	switch opcode {
	// === Immediate ===
	case 0xA9, 0xA2, 0xA0: // LDA/LDX/LDY imm
		amd64MOV_reg_imm32(cb, dstReg, uint32(operand&0xFF))

	// === Zero Page ===
	case 0xA5, 0xA6, 0xA4: // LDA/LDX/LDY zp
		emit6502AddrZP(cb, byte(operand))
		bailOff := emit6502ZPPageCheck(cb, 0)
		*bails = append(*bails, bailInfo{
			offsets: []int{bailOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOVZX_B_memSIB(cb, dstReg, j65RegMem, amd64RAX)

	// === Zero Page,X ===
	case 0xB5, 0xB4: // LDA/LDY zp,X
		emit6502AddrZPX(cb, byte(operand))
		bailOff := emit6502ZPPageCheck(cb, 0)
		*bails = append(*bails, bailInfo{
			offsets: []int{bailOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOVZX_B_memSIB(cb, dstReg, j65RegMem, amd64RAX)

	// === Zero Page,Y ===
	case 0xB6: // LDX zp,Y
		emit6502AddrZPY(cb, byte(operand))
		bailOff := emit6502ZPPageCheck(cb, 0)
		*bails = append(*bails, bailInfo{
			offsets: []int{bailOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOVZX_B_memSIB(cb, dstReg, j65RegMem, amd64RAX)

	// === Absolute ===
	case 0xAD, 0xAE, 0xAC: // LDA/LDX/LDY abs
		emit6502AddrAbs(cb, operand)
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{
			offsets: []int{fpOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOVZX_B_memSIB(cb, dstReg, j65RegMem, amd64RAX)

	// === Absolute,X ===
	case 0xBD, 0xBC: // LDA/LDY abs,X
		emit6502AddrAbsX(cb, operand)
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{
			offsets: []int{fpOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOVZX_B_memSIB(cb, dstReg, j65RegMem, amd64RAX)
		if isLoadWithPageCross {
			emit6502PageCrossCheck(cb)
		}

	// === Absolute,Y ===
	case 0xB9, 0xBE: // LDA/LDX abs,Y
		emit6502AddrAbsY(cb, operand)
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{
			offsets: []int{fpOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOVZX_B_memSIB(cb, dstReg, j65RegMem, amd64RAX)
		if isLoadWithPageCross {
			emit6502PageCrossCheck(cb)
		}

	// === (Indirect,X) ===
	case 0xA1: // LDA (ind,X)
		emit6502AddrIndX(cb, byte(operand))
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{
			offsets: []int{fpOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOVZX_B_memSIB(cb, dstReg, j65RegMem, amd64RAX)

	// === (Indirect),Y ===
	case 0xB1: // LDA (ind),Y
		emit6502AddrIndY(cb, byte(operand))
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{
			offsets: []int{fpOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOVZX_B_memSIB(cb, dstReg, j65RegMem, amd64RAX)
		if isLoadWithPageCross {
			emit6502PageCrossCheck(cb)
		}
	}
}

// emit6502Store emits a store of srcReg to the given addressing mode.
// Adds bail targets for fast-path check failures and inval targets for self-mod detection.
func emit6502Store(cb *CodeBuffer, srcReg byte, opcode byte, operand uint16,
	instrPC uint32, instrIdx int, pendingCycles uint32, instrCycles uint32,
	bails *[]bailInfo, invals *[]invalInfo, nextPC uint32,
	nzPending bool, nzReg byte) {

	switch opcode {
	// === Zero Page ===
	case 0x85, 0x86, 0x84: // STA/STX/STY zp
		emit6502AddrZP(cb, byte(operand))
		bailOff := emit6502ZPPageCheck(cb, 0)
		*bails = append(*bails, bailInfo{
			offsets: []int{bailOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RAX, srcReg)
		invalOff := emit6502SelfModCheck(cb)
		*invals = append(*invals, invalInfo{
			offsets: []int{invalOff}, nextPC: nextPC, instrIdx: instrIdx + 1, pendingCycles: pendingCycles + instrCycles, nzPending: nzPending, nzReg: nzReg,
		})

	// === Zero Page,X ===
	case 0x95, 0x94: // STA/STY zp,X
		emit6502AddrZPX(cb, byte(operand))
		bailOff := emit6502ZPPageCheck(cb, 0)
		*bails = append(*bails, bailInfo{
			offsets: []int{bailOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RAX, srcReg)
		invalOff := emit6502SelfModCheck(cb)
		*invals = append(*invals, invalInfo{
			offsets: []int{invalOff}, nextPC: nextPC, instrIdx: instrIdx + 1, pendingCycles: pendingCycles + instrCycles, nzPending: nzPending, nzReg: nzReg,
		})

	// === Zero Page,Y ===
	case 0x96: // STX zp,Y
		emit6502AddrZPY(cb, byte(operand))
		bailOff := emit6502ZPPageCheck(cb, 0)
		*bails = append(*bails, bailInfo{
			offsets: []int{bailOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RAX, srcReg)
		invalOff := emit6502SelfModCheck(cb)
		*invals = append(*invals, invalInfo{
			offsets: []int{invalOff}, nextPC: nextPC, instrIdx: instrIdx + 1, pendingCycles: pendingCycles + instrCycles, nzPending: nzPending, nzReg: nzReg,
		})

	// === Absolute ===
	case 0x8D, 0x8E, 0x8C: // STA/STX/STY abs
		emit6502AddrAbs(cb, operand)
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{
			offsets: []int{fpOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RAX, srcReg)
		invalOff := emit6502SelfModCheck(cb)
		*invals = append(*invals, invalInfo{
			offsets: []int{invalOff}, nextPC: nextPC, instrIdx: instrIdx + 1, pendingCycles: pendingCycles + instrCycles, nzPending: nzPending, nzReg: nzReg,
		})

	// === Absolute,X ===
	case 0x9D: // STA abs,X
		emit6502AddrAbsX(cb, operand)
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{
			offsets: []int{fpOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RAX, srcReg)
		invalOff := emit6502SelfModCheck(cb)
		*invals = append(*invals, invalInfo{
			offsets: []int{invalOff}, nextPC: nextPC, instrIdx: instrIdx + 1, pendingCycles: pendingCycles + instrCycles, nzPending: nzPending, nzReg: nzReg,
		})

	// === Absolute,Y ===
	case 0x99: // STA abs,Y
		emit6502AddrAbsY(cb, operand)
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{
			offsets: []int{fpOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RAX, srcReg)
		invalOff := emit6502SelfModCheck(cb)
		*invals = append(*invals, invalInfo{
			offsets: []int{invalOff}, nextPC: nextPC, instrIdx: instrIdx + 1, pendingCycles: pendingCycles + instrCycles, nzPending: nzPending, nzReg: nzReg,
		})

	// === (Indirect,X) ===
	case 0x81: // STA (ind,X)
		emit6502AddrIndX(cb, byte(operand))
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{
			offsets: []int{fpOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RAX, srcReg)
		invalOff := emit6502SelfModCheck(cb)
		*invals = append(*invals, invalInfo{
			offsets: []int{invalOff}, nextPC: nextPC, instrIdx: instrIdx + 1, pendingCycles: pendingCycles + instrCycles, nzPending: nzPending, nzReg: nzReg,
		})

	// === (Indirect),Y ===
	case 0x91: // STA (ind),Y
		emit6502AddrIndY(cb, byte(operand))
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{
			offsets: []int{fpOff}, instrPC: instrPC, instrIdx: instrIdx, pendingCycles: pendingCycles, nzPending: nzPending, nzReg: nzReg,
		})
		amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RAX, srcReg)
		invalOff := emit6502SelfModCheck(cb)
		*invals = append(*invals, invalInfo{
			offsets: []int{invalOff}, nextPC: nextPC, instrIdx: instrIdx + 1, pendingCycles: pendingCycles + instrCycles, nzPending: nzPending, nzReg: nzReg,
		})
	}
}

// ===========================================================================
// Transfer Instructions
// ===========================================================================

func emit6502Transfer(cb *CodeBuffer, dst, src byte, updateNZ bool) {
	amd64MOV_reg_reg32(cb, dst, src)
	amd64AND_reg_imm32_32bit(cb, dst, 0xFF)
	if updateNZ {
		emit6502UpdateNZ(cb, dst)
	}
}

// ===========================================================================
// Control Flow — Conditional Branches
// ===========================================================================

// branchFixup records a forward branch JMP that needs patching to a target instruction.
type branchFixup struct {
	jmpOffset      int // offset of the JMP rel32 placeholder
	targetInstrIdx int // index of the target instruction in the block
}

// branchExit records a branch-taken exit from the block (external target).
type branchExit struct {
	jmpOffset     int    // offset of JMP rel32 to this exit
	targetPC      uint32 // 6502 target PC
	instrCount    uint32 // instructions retired including this branch
	pendingCycles uint32 // cycles including branch penalties
	nzPending     bool   // lazy N/Z state at this branch exit
	nzReg         byte   // register holding deferred N/Z result
}

// j65BranchInfo describes a 6502 conditional branch's flag test.
type j65BranchInfo struct {
	flagBit    uint32 // SR flag bit to test (0x01, 0x02, 0x40, 0x80)
	takenIfSet bool   // true if branch taken when flag is SET
}

var j65BranchTable = map[byte]j65BranchInfo{
	0x90: {0x01, false}, // BCC: branch if C=0
	0xB0: {0x01, true},  // BCS: branch if C=1
	0xF0: {0x02, true},  // BEQ: branch if Z=1
	0xD0: {0x02, false}, // BNE: branch if Z=0
	0x30: {0x80, true},  // BMI: branch if N=1
	0x10: {0x80, false}, // BPL: branch if N=0
	0x70: {0x40, true},  // BVS: branch if V=1
	0x50: {0x40, false}, // BVC: branch if V=0
}

// findInstrByPC finds the instruction index whose pcOffset matches targetOffset.
// Returns -1 if not found (external branch).
func findInstrByPC(instrs []JIT6502Instr, targetOffset uint16) int {
	for i, ji := range instrs {
		if ji.pcOffset == targetOffset {
			return i
		}
	}
	return -1
}

// emit6502ConditionalBranch emits a conditional branch instruction.
// Returns branchFixup for internal forward, branchExit for external, or handles backward inline.
func emit6502ConditionalBranch(cb *CodeBuffer, ji *JIT6502Instr, startPC uint16,
	instrIdx int, pendingCycles uint32, instrs []JIT6502Instr, instrOffsets []int,
	hasBackward bool, fwdFixups *[]branchFixup, extExits *[]branchExit,
	nz *j65NZState) {

	bi := j65BranchTable[ji.opcode]
	instrPC := startPC + ji.pcOffset
	branchPC := instrPC + 2 // PC after fetching the branch instruction
	offset := int8(ji.operand & 0xFF)
	targetPC := uint16(int(branchPC) + int(offset))

	// Check if target is within the block
	targetOffset := int(targetPC) - int(startPC)
	targetIdx := -1
	if targetOffset >= 0 {
		targetIdx = findInstrByPC(instrs, uint16(targetOffset))
	}

	// Emit conditional test. For N/Z flags with pending state, test the
	// pending register directly instead of materializing into R15.
	isNZFlag := bi.flagBit == 0x02 || bi.flagBit == 0x80
	useDirect := isNZFlag && nz.nzPending

	var notTakenCond byte
	if useDirect {
		if bi.flagBit == 0x02 { // Z flag (BEQ/BNE)
			amd64TEST_reg_reg32(cb, nz.nzReg, nz.nzReg) // ZF = (result == 0)
			if bi.takenIfSet {
				notTakenCond = amd64CondNE // skip taken if NOT zero (Z=0)
			} else {
				notTakenCond = amd64CondE // skip taken if zero (Z=1)
			}
		} else { // N flag (BMI/BPL), flagBit == 0x80
			amd64TEST_reg_imm32(cb, nz.nzReg, 0x80) // check bit 7
			if bi.takenIfSet {
				notTakenCond = amd64CondE // skip taken if bit 7 NOT set (N=0)
			} else {
				notTakenCond = amd64CondNE // skip taken if bit 7 IS set (N=1)
			}
		}
	} else {
		// C/V flags or N/Z not pending — materialize if needed, test R15
		if isNZFlag {
			j65MaterializeNZ(cb, nz)
		}
		amd64TEST_reg_imm32(cb, j65RegSR, bi.flagBit)
		if bi.takenIfSet {
			notTakenCond = amd64CondE // JZ = skip if flag NOT set
		} else {
			notTakenCond = amd64CondNE // JNZ = skip if flag IS set
		}
	}
	notTakenOff := amd64Jcc_rel32(cb, notTakenCond)

	// === TAKEN PATH ===

	// +1 cycle for taken
	amd64INC_mem32(cb, amd64RSP, int32(j65OffCycles))

	// Page cross check (compile-time): branchPC and targetPC
	if (branchPC & 0xFF00) != (targetPC & 0xFF00) {
		amd64INC_mem32(cb, amd64RSP, int32(j65OffCycles)) // +1 more for page cross
	}

	if targetIdx >= 0 {
		// INTERNAL branch
		isBackward := targetIdx <= instrIdx

		if isBackward && hasBackward {
			// Budget check
			amd64INC_mem32(cb, amd64RSP, int32(j65OffLoopCount))
			// CMP DWORD [RSP+loopCount], budget
			amd64CMP_mem32_imm32(cb, amd64RSP, int32(j65OffLoopCount), jitBudget)
			// JAE budget_bail — exit if budget exhausted
			budgetBailOff := amd64Jcc_rel32(cb, amd64CondAE)
			// Budget bail exits the block normally (not a bail, just an exit)
			*extExits = append(*extExits, branchExit{
				jmpOffset:     budgetBailOff,
				targetPC:      uint32(targetPC),
				instrCount:    uint32(instrIdx + 1),
				pendingCycles: pendingCycles,
				nzPending:     nz.nzPending,
				nzReg:         nz.nzReg,
			})
		}

		if isBackward {
			// Target offset is known, emit direct backward JMP
			jmpOff := amd64JMP_rel32(cb)
			patchRel32(cb, jmpOff, instrOffsets[targetIdx])
		} else {
			// Forward: record fixup
			jmpOff := amd64JMP_rel32(cb)
			*fwdFixups = append(*fwdFixups, branchFixup{jmpOff, targetIdx})
		}
	} else {
		// EXTERNAL branch — exit the block
		jmpOff := amd64JMP_rel32(cb)
		exitCycles := pendingCycles // cycles already flushed to accumulator by earlier code
		*extExits = append(*extExits, branchExit{
			jmpOffset:     jmpOff,
			targetPC:      uint32(targetPC),
			instrCount:    uint32(instrIdx + 1),
			pendingCycles: exitCycles,
			nzPending:     nz.nzPending,
			nzReg:         nz.nzReg,
		})
	}

	// === NOT TAKEN PATH ===
	patchRel32(cb, notTakenOff, cb.Len())
}

// amd64CMP_mem32_imm32 emits CMP DWORD [base+disp], imm32.
func amd64CMP_mem32_imm32(cb *CodeBuffer, base byte, disp int32, imm int32) {
	emitMemOp(cb, false, 0x81, 7, base, disp) // /7 = CMP
	cb.Emit32(uint32(imm))
}

// ===========================================================================
// Control Flow — JMP, JSR, RTS
// ===========================================================================

// emit6502JMP_Abs emits JMP absolute ($4C). Block terminator.
// Returns chain exit info for patching.
func emit6502JMP_Abs(cb *CodeBuffer, targetPC uint16, instrCount uint32, pendingCycles *uint32, nzPending bool, nzReg byte) j65ChainExitInfo {
	return emit6502ChainExit(cb, uint32(targetPC), instrCount, pendingCycles, nzPending, nzReg)
}

// emit6502JMP_Ind emits JMP indirect ($6C). Block terminator.
// Handles the 6502 page-crossing bug: if low byte of pointer is $FF,
// the high byte is read from $xx00 (same page) instead of $xx00+$100.
func emit6502JMP_Ind(cb *CodeBuffer, operand uint16, instrCount uint32,
	pendingCycles *uint32, bails *[]bailInfo, instrPC uint32, instrIdx int, curPending uint32,
	nzPending bool, nzReg byte) {

	// Load the pointer address
	amd64MOV_reg_imm32(cb, amd64RAX, uint32(operand))

	// Fast-path check for the pointer read
	fpOff := emit6502FullFastPathCheck(cb)
	*bails = append(*bails, bailInfo{[]int{fpOff}, instrPC, instrIdx, curPending, nzPending, nzReg})

	// Read low byte of target
	amd64MOVZX_B_memSIB(cb, amd64R10, j65RegMem, amd64RAX)

	// Read high byte with page-crossing bug:
	// highAddr = (operand & 0xFF00) | ((operand + 1) & 0x00FF)
	highAddr := (operand & 0xFF00) | ((operand + 1) & 0x00FF)
	amd64MOV_reg_imm32(cb, amd64RAX, uint32(highAddr))
	// Check fast-path for high byte too
	fpOff2 := emit6502FullFastPathCheck(cb)
	*bails = append(*bails, bailInfo{[]int{fpOff2}, instrPC, instrIdx, curPending, nzPending, nzReg})

	amd64MOVZX_B_memSIB(cb, amd64R11, j65RegMem, amd64RAX)

	// Combine: R10D = low | (high << 8)
	amd64SHL_imm32(cb, amd64R11, 8)
	emitREX(cb, false, amd64R11, amd64R10)
	cb.EmitBytes(0x09, modRM(3, amd64R11, amd64R10)) // OR R10D, R11D

	// targetPC in R10D — dynamic target, cannot chain
	flushPendingCycles(cb, pendingCycles)

	// Accumulate this block's instruction count into ChainCount before unchained exit
	amd64MOV_reg_mem(cb, amd64RCX, amd64RSP, int32(j65OffCtxPtr))
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RCX, int32(j65CtxOffChainCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrCount)) // ADD EAX, instrCount
	amd64MOV_mem_reg32(cb, amd64RCX, int32(j65CtxOffChainCount), amd64RAX)

	emit6502UnchainedExitReg(cb, amd64R10, nzPending, nzReg)
}

// emit6502JSR emits JSR ($20). Block terminator.
// Pushes PC+2 (address of last byte of JSR) to stack, sets PC to target.
// Returns chain exit info for patching.
func emit6502JSR(cb *CodeBuffer, targetPC uint16, returnAddr uint16, instrCount uint32, pendingCycles *uint32, nzPending bool, nzReg byte) j65ChainExitInfo {
	// Push return address high byte
	amd64MOVZX_B(cb, amd64RAX, j65RegSP)
	amd64OR_reg_imm32_32bit(cb, amd64RAX, 0x0100)
	// MOV BYTE [RSI + RAX], high(returnAddr)
	amd64MOV_reg_imm32(cb, amd64RCX, uint32(returnAddr>>8))
	amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RAX, amd64RCX)
	amd64DEC_reg8(cb, j65RegSP)

	// Push return address low byte
	amd64MOVZX_B(cb, amd64RAX, j65RegSP)
	amd64OR_reg_imm32_32bit(cb, amd64RAX, 0x0100)
	amd64MOV_reg_imm32(cb, amd64RCX, uint32(returnAddr&0xFF))
	amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RAX, amd64RCX)
	amd64DEC_reg8(cb, j65RegSP)

	return emit6502ChainExit(cb, uint32(targetPC), instrCount, pendingCycles, nzPending, nzReg)
}

// emit6502RTS emits RTS ($60). Block terminator.
// Pulls PC from stack, adds 1. Checks 2-entry MRU RTS cache for chain hit.
func emit6502RTS(cb *CodeBuffer, instrCount uint32, pendingCycles *uint32, nzPending bool, nzReg byte) {
	// Pull low byte
	amd64INC_reg8(cb, j65RegSP)
	amd64MOVZX_B(cb, amd64RAX, j65RegSP)
	amd64OR_reg_imm32_32bit(cb, amd64RAX, 0x0100)
	amd64MOVZX_B_memSIB(cb, amd64R10, j65RegMem, amd64RAX) // R10 = low byte

	// Pull high byte
	amd64INC_reg8(cb, j65RegSP)
	amd64MOVZX_B(cb, amd64RAX, j65RegSP)
	amd64OR_reg_imm32_32bit(cb, amd64RAX, 0x0100)
	amd64MOVZX_B_memSIB(cb, amd64R11, j65RegMem, amd64RAX) // R11 = high byte

	// Combine + 1: R10D = (high << 8 | low) + 1
	amd64SHL_imm32(cb, amd64R11, 8)
	emitREX(cb, false, amd64R11, amd64R10)
	cb.EmitBytes(0x09, modRM(3, amd64R11, amd64R10)) // OR R10D, R11D
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)     // ADD R10D, 1
	amd64AND_reg_imm32_32bit(cb, amd64R10, 0xFFFF)   // mask 16-bit

	flushPendingCycles(cb, pendingCycles)

	// Load ctx for cache check and chain count
	amd64MOV_reg_mem(cb, amd64RCX, amd64RSP, int32(j65OffCtxPtr))

	// Accumulate instruction count before potential chain
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RCX, int32(j65CtxOffChainCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrCount)) // ADD EAX, instrCount
	amd64MOV_mem_reg32(cb, amd64RCX, int32(j65CtxOffChainCount), amd64RAX)

	// Materialize deferred N/Z before cache check / chain / unchained exit
	if nzPending {
		emit6502UpdateNZ(cb, nzReg)
	}

	// === 2-entry MRU RTS cache check ===
	// Check entry 0: CMP R10D, [RCX + RTSCache0PC]
	amd64ALU_reg_mem32_cmp(cb, amd64R10, amd64RCX, int32(j65CtxOffRTSCache0PC))
	miss0Off := amd64Jcc_rel32(cb, amd64CondNE) // JNE .check1

	// Entry 0 hit: load chain addr
	amd64MOV_reg_mem(cb, amd64R11, amd64RCX, int32(j65CtxOffRTSCache0Addr))
	hitOff := amd64JMP_rel32(cb) // JMP .hit

	// .check1: Check entry 1
	patchRel32(cb, miss0Off, cb.Len())
	amd64ALU_reg_mem32_cmp(cb, amd64R10, amd64RCX, int32(j65CtxOffRTSCache1PC))
	miss1Off := amd64Jcc_rel32(cb, amd64CondNE) // JNE .miss

	// Entry 1 hit: load chain addr
	amd64MOV_reg_mem(cb, amd64R11, amd64RCX, int32(j65CtxOffRTSCache1Addr))

	// .hit: chain budget/inval check, then JMP R11
	patchRel32(cb, hitOff, cb.Len())

	// DEC ChainBudget
	amd64DEC_mem32(cb, amd64RCX, int32(j65CtxOffChainBudget))
	budgetOff := amd64Jcc_rel32(cb, amd64CondLE) // JLE .miss (reuse miss path)

	// Check NeedInval
	amd64ALU_mem_imm8(cb, 7, amd64RCX, int32(j65CtxOffNeedInval), 0) // CMP [RCX+NeedInval], 0
	invalOff := amd64Jcc_rel32(cb, amd64CondNE)                      // JNE .miss

	// JMP R11 (chain to target)
	emitREX(cb, true, 0, amd64R11)
	cb.EmitBytes(0xFF, modRM(3, 4, amd64R11)) // JMP R11

	// .miss: unchained exit with R10D
	patchRel32(cb, miss1Off, cb.Len())
	patchRel32(cb, budgetOff, cb.Len())
	patchRel32(cb, invalOff, cb.Len())
	emit6502UnchainedExitReg(cb, amd64R10, false, 0) // NZ already materialized above
}

// ===========================================================================
// Byte Register Inc/Dec/Shift Helpers
// ===========================================================================

// amd64INC_reg8 emits INC r8 (byte increment).
func amd64INC_reg8(cb *CodeBuffer, reg byte) {
	if isExtReg(reg) || (reg >= 4 && reg <= 7) {
		cb.EmitBytes(rexByte(false, false, false, isExtReg(reg)))
	}
	cb.EmitBytes(0xFE, modRM(3, 0, reg)) // FE /0
}

// amd64DEC_reg8 emits DEC r8 (byte decrement).
func amd64DEC_reg8(cb *CodeBuffer, reg byte) {
	if isExtReg(reg) || (reg >= 4 && reg <= 7) {
		cb.EmitBytes(rexByte(false, false, false, isExtReg(reg)))
	}
	cb.EmitBytes(0xFE, modRM(3, 1, reg)) // FE /1
}

// emit6502ShiftByte emits a shift/rotate by 1 on a byte register.
// shiftOp: 4=SHL, 5=SHR, 2=RCL, 3=RCR
func emit6502ShiftByte(cb *CodeBuffer, reg byte, shiftOp byte) {
	if isExtReg(reg) || (reg >= 4 && reg <= 7) {
		cb.EmitBytes(rexByte(false, false, false, isExtReg(reg)))
	}
	cb.EmitBytes(0xD0, modRM(3, shiftOp, reg)) // D0 /op r/m8, 1
}

// ===========================================================================
// Shift/Rotate Flag Extraction
// ===========================================================================

// emit6502ShiftFlags extracts C, N, Z after a shift/rotate operation.
// SETC must be callable right after the shift (before any flag-modifying instr).
// resultReg contains the shifted byte result.
func emit6502ShiftFlags(cb *CodeBuffer, resultReg byte) {
	// Capture carry from shift
	cb.EmitBytes(0x0F, 0x92, modRM(3, 0, amd64RCX)) // SETC CL

	// Zero-extend result
	amd64MOVZX_B(cb, resultReg, resultReg)

	// Clear C, N, Z in SR
	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0x7C) // ~(0x01|0x02|0x80)

	// Set C from CL
	amd64MOVZX_B(cb, amd64RCX, amd64RCX)
	emitREX(cb, false, amd64RCX, j65RegSR)
	cb.EmitBytes(0x09, modRM(3, amd64RCX, j65RegSR)) // OR R15D, ECX

	// N, Z from result
	amd64TEST_reg_reg32(cb, resultReg, resultReg)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x02)
	patchRel32(cb, notZeroOff, cb.Len())

	amd64TEST_reg_imm32(cb, resultReg, 0x80)
	notNegOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, int32(NEGATIVE_FLAG))
	patchRel32(cb, notNegOff, cb.Len())
}

// emit6502ShiftFlagsCOnly extracts only the Carry flag after a shift/rotate.
// N/Z are deferred — the caller must call j65SetNZPending. Used for accumulator shifts.
func emit6502ShiftFlagsCOnly(cb *CodeBuffer, resultReg byte) {
	// Capture carry from shift
	cb.EmitBytes(0x0F, 0x92, modRM(3, 0, amd64RCX)) // SETC CL

	// Zero-extend result
	amd64MOVZX_B(cb, resultReg, resultReg)

	// Clear only C in SR (preserve N, Z for lazy deferral)
	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0xFE) // ~0x01

	// Set C from CL
	amd64MOVZX_B(cb, amd64RCX, amd64RCX)
	emitREX(cb, false, amd64RCX, j65RegSR)
	cb.EmitBytes(0x09, modRM(3, amd64RCX, j65RegSR)) // OR R15D, ECX
	// N/Z deferred — caller sets j65SetNZPending
}

// ===========================================================================
// ASL — Arithmetic Shift Left
// ===========================================================================

func emit6502ASL_A(cb *CodeBuffer) {
	emit6502ShiftByte(cb, j65RegA, 4)    // SHL BL, 1
	emit6502ShiftFlagsCOnly(cb, j65RegA) // N/Z deferred by caller
}

func emit6502ASL_Mem(cb *CodeBuffer) {
	// Address already in EAX, byte loaded into R10
	amd64MOVZX_B_memSIB(cb, amd64R10, j65RegMem, amd64RAX)
	emit6502ShiftByte(cb, amd64R10, 4) // SHL R10B, 1
	// Store before extracting flags (SETC is the first thing in ShiftFlags)
	// Wait — SETC must be right after the shift. Let me capture carry first, then store.
	cb.EmitBytes(0x0F, 0x92, modRM(3, 0, amd64RCX)) // SETC CL (capture carry)

	// Zero-extend for NZ check
	amd64MOVZX_B(cb, amd64R10, amd64R10)

	// Store back
	emitREXForByteSIB(cb, amd64R10, amd64RAX, j65RegMem)
	cb.EmitBytes(0x88, modRM(0, amd64R10, 4), sibByte(0, amd64RAX, j65RegMem))

	// Now set flags from saved CL and result in R10D
	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0x7C)
	amd64MOVZX_B(cb, amd64RCX, amd64RCX)
	emitREX(cb, false, amd64RCX, j65RegSR)
	cb.EmitBytes(0x09, modRM(3, amd64RCX, j65RegSR))

	amd64TEST_reg_reg32(cb, amd64R10, amd64R10)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x02)
	patchRel32(cb, notZeroOff, cb.Len())

	amd64TEST_reg_imm32(cb, amd64R10, 0x80)
	notNegOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, int32(NEGATIVE_FLAG))
	patchRel32(cb, notNegOff, cb.Len())
}

// ===========================================================================
// LSR — Logical Shift Right
// ===========================================================================

func emit6502LSR_A(cb *CodeBuffer) {
	emit6502ShiftByte(cb, j65RegA, 5)    // SHR BL, 1
	emit6502ShiftFlagsCOnly(cb, j65RegA) // N/Z deferred by caller
}

func emit6502LSR_Mem(cb *CodeBuffer) {
	amd64MOVZX_B_memSIB(cb, amd64R10, j65RegMem, amd64RAX)
	emit6502ShiftByte(cb, amd64R10, 5)              // SHR R10B, 1
	cb.EmitBytes(0x0F, 0x92, modRM(3, 0, amd64RCX)) // SETC CL

	amd64MOVZX_B(cb, amd64R10, amd64R10)

	emitREXForByteSIB(cb, amd64R10, amd64RAX, j65RegMem)
	cb.EmitBytes(0x88, modRM(0, amd64R10, 4), sibByte(0, amd64RAX, j65RegMem))

	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0x7C)
	amd64MOVZX_B(cb, amd64RCX, amd64RCX)
	emitREX(cb, false, amd64RCX, j65RegSR)
	cb.EmitBytes(0x09, modRM(3, amd64RCX, j65RegSR))

	amd64TEST_reg_reg32(cb, amd64R10, amd64R10)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x02)
	patchRel32(cb, notZeroOff, cb.Len())

	amd64TEST_reg_imm32(cb, amd64R10, 0x80)
	notNegOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, int32(NEGATIVE_FLAG))
	patchRel32(cb, notNegOff, cb.Len())
}

// ===========================================================================
// ROL — Rotate Left through Carry
// ===========================================================================

func emit6502ROL_A(cb *CodeBuffer) {
	// Load old carry into x86 CF
	emitREX(cb, false, 0, j65RegSR)
	cb.EmitBytes(0x0F, 0xBA, modRM(3, 4, j65RegSR), 0x00) // BT R15D, 0

	emit6502ShiftByte(cb, j65RegA, 2)    // RCL BL, 1
	emit6502ShiftFlagsCOnly(cb, j65RegA) // N/Z deferred by caller
}

func emit6502ROL_Mem(cb *CodeBuffer) {
	amd64MOVZX_B_memSIB(cb, amd64R10, j65RegMem, amd64RAX)

	emitREX(cb, false, 0, j65RegSR)
	cb.EmitBytes(0x0F, 0xBA, modRM(3, 4, j65RegSR), 0x00) // BT R15D, 0

	emit6502ShiftByte(cb, amd64R10, 2)              // RCL R10B, 1
	cb.EmitBytes(0x0F, 0x92, modRM(3, 0, amd64RCX)) // SETC CL

	amd64MOVZX_B(cb, amd64R10, amd64R10)

	emitREXForByteSIB(cb, amd64R10, amd64RAX, j65RegMem)
	cb.EmitBytes(0x88, modRM(0, amd64R10, 4), sibByte(0, amd64RAX, j65RegMem))

	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0x7C)
	amd64MOVZX_B(cb, amd64RCX, amd64RCX)
	emitREX(cb, false, amd64RCX, j65RegSR)
	cb.EmitBytes(0x09, modRM(3, amd64RCX, j65RegSR))

	amd64TEST_reg_reg32(cb, amd64R10, amd64R10)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x02)
	patchRel32(cb, notZeroOff, cb.Len())

	amd64TEST_reg_imm32(cb, amd64R10, 0x80)
	notNegOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, int32(NEGATIVE_FLAG))
	patchRel32(cb, notNegOff, cb.Len())
}

// ===========================================================================
// ROR — Rotate Right through Carry
// ===========================================================================

func emit6502ROR_A(cb *CodeBuffer) {
	emitREX(cb, false, 0, j65RegSR)
	cb.EmitBytes(0x0F, 0xBA, modRM(3, 4, j65RegSR), 0x00) // BT R15D, 0

	emit6502ShiftByte(cb, j65RegA, 3)    // RCR BL, 1
	emit6502ShiftFlagsCOnly(cb, j65RegA) // N/Z deferred by caller
}

func emit6502ROR_Mem(cb *CodeBuffer) {
	amd64MOVZX_B_memSIB(cb, amd64R10, j65RegMem, amd64RAX)

	emitREX(cb, false, 0, j65RegSR)
	cb.EmitBytes(0x0F, 0xBA, modRM(3, 4, j65RegSR), 0x00) // BT R15D, 0

	emit6502ShiftByte(cb, amd64R10, 3)              // RCR R10B, 1
	cb.EmitBytes(0x0F, 0x92, modRM(3, 0, amd64RCX)) // SETC CL

	amd64MOVZX_B(cb, amd64R10, amd64R10)

	emitREXForByteSIB(cb, amd64R10, amd64RAX, j65RegMem)
	cb.EmitBytes(0x88, modRM(0, amd64R10, 4), sibByte(0, amd64RAX, j65RegMem))

	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0x7C)
	amd64MOVZX_B(cb, amd64RCX, amd64RCX)
	emitREX(cb, false, amd64RCX, j65RegSR)
	cb.EmitBytes(0x09, modRM(3, amd64RCX, j65RegSR))

	amd64TEST_reg_reg32(cb, amd64R10, amd64R10)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x02)
	patchRel32(cb, notZeroOff, cb.Len())

	amd64TEST_reg_imm32(cb, amd64R10, 0x80)
	notNegOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, int32(NEGATIVE_FLAG))
	patchRel32(cb, notNegOff, cb.Len())
}

// ===========================================================================
// Stack Operations
// ===========================================================================

// emit6502PHA emits PHA (push A to stack).
func emit6502PHA(cb *CodeBuffer) {
	// addr = 0x0100 | SP
	amd64MOVZX_B(cb, amd64RAX, j65RegSP)          // EAX = SP (zero-extended)
	amd64OR_reg_imm32_32bit(cb, amd64RAX, 0x0100) // EAX = 0x100 | SP
	// MOV BYTE [RSI + RAX], BL
	amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RAX, j65RegA)
	// DEC SP (byte wrap)
	amd64DEC_reg8(cb, j65RegSP)
}

// emit6502PLA emits PLA (pull A from stack). N/Z deferred by caller.
func emit6502PLA(cb *CodeBuffer) {
	// SP++
	amd64INC_reg8(cb, j65RegSP)
	// addr = 0x0100 | SP
	amd64MOVZX_B(cb, amd64RAX, j65RegSP)
	amd64OR_reg_imm32_32bit(cb, amd64RAX, 0x0100)
	// Load A = BYTE [RSI + RAX]
	amd64MOVZX_B_memSIB(cb, j65RegA, j65RegMem, amd64RAX)
	// N/Z deferred — caller sets j65SetNZPending(&nz, j65RegA)
}

// emit6502PHP emits PHP (push processor status with B and U set).
func emit6502PHP(cb *CodeBuffer) {
	// value = SR | 0x30 (set B and U flags)
	amd64MOV_reg_reg32(cb, amd64RAX, j65RegSR)  // EAX = SR
	amd64OR_reg_imm32_32bit(cb, amd64RAX, 0x30) // set B and U

	// addr = 0x0100 | SP
	amd64MOVZX_B(cb, amd64RCX, j65RegSP)
	amd64OR_reg_imm32_32bit(cb, amd64RCX, 0x0100)

	// MOV BYTE [RSI + RCX], AL
	amd64MOV_memSIB_reg8(cb, j65RegMem, amd64RCX, amd64RAX)

	// DEC SP
	amd64DEC_reg8(cb, j65RegSP)
}

// emit6502PLP emits PLP (pull processor status, clear B, set U).
func emit6502PLP(cb *CodeBuffer) {
	// SP++
	amd64INC_reg8(cb, j65RegSP)
	// addr = 0x0100 | SP
	amd64MOVZX_B(cb, amd64RAX, j65RegSP)
	amd64OR_reg_imm32_32bit(cb, amd64RAX, 0x0100)
	// Load SR
	amd64MOVZX_B_memSIB(cb, j65RegSR, j65RegMem, amd64RAX)
	// Clear B flag (bit 4), set U flag (bit 5)
	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0xEF) // clear bit 4 (B)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x20)  // set bit 5 (U)
}

// ===========================================================================
// Generic Operand Loader (for group 01 instructions: ADC/SBC/AND/ORA/EOR/CMP)
// ===========================================================================

// emit6502LoadOperandToEAX loads the operand byte into EAX for any group 01
// addressing mode. Group 01 opcodes have addressing mode in bits 4:2.
// Returns true if page-crossing check was emitted (for loads with variable cycles).
func emit6502LoadOperandToEAX(cb *CodeBuffer, opcode byte, operand uint16,
	instrPC uint32, instrIdx int, pendingCycles uint32,
	bails *[]bailInfo, emitPageCross bool,
	nzPending bool, nzReg byte) {

	mode := (opcode >> 2) & 7
	switch mode {
	case 2: // Immediate
		amd64MOV_reg_imm32(cb, amd64RAX, uint32(operand&0xFF))
	case 1: // Zero Page
		emit6502AddrZP(cb, byte(operand))
		bailOff := emit6502ZPPageCheck(cb, 0)
		*bails = append(*bails, bailInfo{[]int{bailOff}, instrPC, instrIdx, pendingCycles, nzPending, nzReg})
		amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
	case 5: // Zero Page,X
		emit6502AddrZPX(cb, byte(operand))
		bailOff := emit6502ZPPageCheck(cb, 0)
		*bails = append(*bails, bailInfo{[]int{bailOff}, instrPC, instrIdx, pendingCycles, nzPending, nzReg})
		amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
	case 3: // Absolute
		emit6502AddrAbs(cb, operand)
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{[]int{fpOff}, instrPC, instrIdx, pendingCycles, nzPending, nzReg})
		amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
	case 7: // Absolute,X
		emit6502AddrAbsX(cb, operand)
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{[]int{fpOff}, instrPC, instrIdx, pendingCycles, nzPending, nzReg})
		amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
		if emitPageCross {
			emit6502PageCrossCheck(cb)
		}
	case 6: // Absolute,Y
		emit6502AddrAbsY(cb, operand)
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{[]int{fpOff}, instrPC, instrIdx, pendingCycles, nzPending, nzReg})
		amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
		if emitPageCross {
			emit6502PageCrossCheck(cb)
		}
	case 0: // (Indirect,X)
		emit6502AddrIndX(cb, byte(operand))
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{[]int{fpOff}, instrPC, instrIdx, pendingCycles, nzPending, nzReg})
		amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
	case 4: // (Indirect),Y
		emit6502AddrIndY(cb, byte(operand))
		fpOff := emit6502FullFastPathCheck(cb)
		*bails = append(*bails, bailInfo{[]int{fpOff}, instrPC, instrIdx, pendingCycles, nzPending, nzReg})
		amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
		if emitPageCross {
			emit6502PageCrossCheck(cb)
		}
	}
}

// ===========================================================================
// ADC — Add with Carry (binary mode only, decimal mode bails)
// ===========================================================================

// emit6502ADCFlags emits the ADC operation and flag extraction.
// Operand must be in EAX, A is in j65RegA (EBX). Modifies A and SR.
// Uses native 8-bit ADC instruction for correct C/V/N/Z flag extraction.
func emit6502ADCFlags(cb *CodeBuffer) {
	// Load carry from SR bit 0 into x86 CF
	// BT R15D, 0 → CF = bit 0 of SR
	emitREX(cb, false, 0, j65RegSR)
	cb.EmitBytes(0x0F, 0xBA, modRM(3, 4, j65RegSR), 0x00) // BT R15D, 0

	// ADC BL, AL — 8-bit add with carry. BL=A, AL=operand.
	// No REX needed (BL=reg3, AL=reg0, both < 4)
	cb.EmitBytes(0x12, modRM(3, regBits(j65RegA), regBits(amd64RAX))) // ADC BL, AL

	// Extract flags immediately (before any flag-modifying instruction)
	// SETC CL (carry out)
	cb.EmitBytes(0x0F, 0x92, modRM(3, 0, amd64RCX)) // SETC CL
	// SETO DL (overflow)
	cb.EmitBytes(0x0F, 0x90, modRM(3, 0, amd64RDX)) // SETO DL

	// Zero-extend result
	amd64MOVZX_B(cb, j65RegA, j65RegA) // EBX = zero-extended BL

	// Clear C, V in SR (preserve N, Z for lazy deferral; also preserve I, D, B, U)
	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0xBE) // ~(0x01|0x40) = 0xBE

	// Set C from CL
	amd64MOVZX_B(cb, amd64RCX, amd64RCX)
	emitREX(cb, false, amd64RCX, j65RegSR)
	cb.EmitBytes(0x09, modRM(3, amd64RCX, j65RegSR)) // OR R15D, ECX

	// Set V from DL (shift to bit 6)
	amd64MOVZX_B(cb, amd64RDX, amd64RDX)
	amd64SHL_imm32(cb, amd64RDX, 6)
	emitREX(cb, false, amd64RDX, j65RegSR)
	cb.EmitBytes(0x09, modRM(3, amd64RDX, j65RegSR)) // OR R15D, EDX

	// N/Z deferred — caller sets j65SetNZPending(&nz, j65RegA)
}

// emit6502DecimalBailCheck emits a check for decimal mode (D flag).
// If D flag is set, jumps to a bail path. Returns bail offset to patch.
func emit6502DecimalBailCheck(cb *CodeBuffer) int {
	amd64TEST_reg_imm32(cb, j65RegSR, uint32(DECIMAL_FLAG))
	return amd64Jcc_rel32(cb, amd64CondNE) // JNZ bail (D flag set)
}

// ===========================================================================
// SBC — Subtract with Carry (binary mode only)
// ===========================================================================

// emit6502SBCFlags emits the SBC operation and flag extraction.
// Operand in EAX, A in j65RegA. 6502 SBC: A = A - operand - (1-C).
func emit6502SBCFlags(cb *CodeBuffer) {
	// Load carry and invert: 6502 C=1 means no borrow → x86 CF should be 0
	// BT R15D, 0 → CF = bit 0 of SR (6502 carry)
	emitREX(cb, false, 0, j65RegSR)
	cb.EmitBytes(0x0F, 0xBA, modRM(3, 4, j65RegSR), 0x00)
	// CMC — complement carry flag
	cb.EmitBytes(0xF5) // CMC

	// SBB BL, AL — 8-bit subtract with borrow
	cb.EmitBytes(0x1A, modRM(3, regBits(j65RegA), regBits(amd64RAX))) // SBB BL, AL

	// Extract flags
	// SETC CL (x86 carry = borrow occurred)
	cb.EmitBytes(0x0F, 0x92, modRM(3, 0, amd64RCX))
	// SETO DL (overflow)
	cb.EmitBytes(0x0F, 0x90, modRM(3, 0, amd64RDX))

	// Zero-extend result
	amd64MOVZX_B(cb, j65RegA, j65RegA)

	// Clear C, V in SR (preserve N, Z for lazy deferral; also preserve I, D, B, U)
	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0xBE) // ~(0x01|0x40) = 0xBE

	// C = !borrow = !CL (6502: C=1 if no borrow)
	// XOR CL, 1
	cb.EmitBytes(0x80, modRM(3, 6, amd64RCX), 0x01) // XOR CL, 1
	amd64MOVZX_B(cb, amd64RCX, amd64RCX)
	emitREX(cb, false, amd64RCX, j65RegSR)
	cb.EmitBytes(0x09, modRM(3, amd64RCX, j65RegSR))

	// V from DL (shift to bit 6)
	amd64MOVZX_B(cb, amd64RDX, amd64RDX)
	amd64SHL_imm32(cb, amd64RDX, 6)
	emitREX(cb, false, amd64RDX, j65RegSR)
	cb.EmitBytes(0x09, modRM(3, amd64RDX, j65RegSR))

	// N/Z deferred — caller sets j65SetNZPending(&nz, j65RegA)
}

// ===========================================================================
// CMP/CPX/CPY — Compare
// ===========================================================================

// emit6502CompareFlags emits comparison: reg - EAX, sets N, Z, C.
// Does NOT modify the register. Clobbers R10, R11.
func emit6502CompareFlags(cb *CodeBuffer, reg byte) {
	// R10D = reg value
	amd64MOV_reg_reg32(cb, amd64R10, reg)
	// SUB R10D, EAX (32-bit subtraction on zero-extended bytes)
	emitREX(cb, false, amd64RAX, amd64R10)
	cb.EmitBytes(0x29, modRM(3, amd64RAX, amd64R10)) // SUB R10D, EAX

	// SETAE R11B = 1 if no borrow (6502 C = 1 if reg >= operand)
	emitREX(cb, false, 0, amd64R11)
	cb.EmitBytes(0x0F, 0x93, modRM(3, 0, amd64R11)) // SETAE R11B

	// Clear C, Z, N in SR (keep V, I, D, B, U)
	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0x7C) // ~(0x01|0x02|0x80) = 0x7C

	// Set C
	amd64MOVZX_B(cb, amd64R11, amd64R11)
	emitREX(cb, false, amd64R11, j65RegSR)
	cb.EmitBytes(0x09, modRM(3, amd64R11, j65RegSR)) // OR R15D, R11D

	// Set N, Z from 8-bit result in R10D
	amd64AND_reg_imm32_32bit(cb, amd64R10, 0xFF)

	amd64TEST_reg_reg32(cb, amd64R10, amd64R10)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x02)
	patchRel32(cb, notZeroOff, cb.Len())

	amd64TEST_reg_imm32(cb, amd64R10, 0x80)
	notNegOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, int32(NEGATIVE_FLAG))
	patchRel32(cb, notNegOff, cb.Len())
}

// ===========================================================================
// AND/ORA/EOR — Logical Operations
// ===========================================================================

// emit6502LogicOp performs a logical operation on A with operand in EAX.
// aluOpcode: 0x21=AND, 0x09=OR, 0x31=XOR (for r32,r32 encoding).
func emit6502LogicOp(cb *CodeBuffer, aluOpcode byte) {
	// Perform 32-bit operation (both zero-extended, result stays zero-extended)
	emitREX(cb, false, amd64RAX, j65RegA)
	cb.EmitBytes(aluOpcode, modRM(3, amd64RAX, j65RegA)) // OP EBX, EAX
	// N/Z deferred — caller sets j65SetNZPending(&nz, j65RegA)
}

// ===========================================================================
// INC/DEC Memory — Read-Modify-Write
// ===========================================================================

// emit6502IncDecMem emits an INC or DEC on a memory byte.
// isInc: true for INC, false for DEC.
// Address mode and operand loading must be done before calling this.
// EAX must contain the effective address. Memory is at [RSI + RAX].
// Clobbers: R10.
func emit6502IncDecMem(cb *CodeBuffer, isInc bool) {
	// Load byte into R10D
	amd64MOVZX_B_memSIB(cb, amd64R10, j65RegMem, amd64RAX)

	// INC or DEC
	if isInc {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1) // ADD R10D, 1
	} else {
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, 1) // SUB R10D, 1
	}
	amd64AND_reg_imm32_32bit(cb, amd64R10, 0xFF) // mask to byte

	// Store back — MOV BYTE [RSI + RAX], R10B
	emitREXForByteSIB(cb, amd64R10, amd64RAX, j65RegMem)
	cb.EmitBytes(0x88, modRM(0, amd64R10, 4), sibByte(0, amd64RAX, j65RegMem))

	// Update N, Z from R10D
	emit6502UpdateNZ(cb, amd64R10)
}

// ===========================================================================
// BIT — Bit Test
// ===========================================================================

// emit6502BITFlags emits the BIT flag computation.
// Operand in EAX, A in j65RegA. Sets N, V, Z.
func emit6502BITFlags(cb *CodeBuffer) {
	// Clear N, V, Z in SR (keep C, I, D, B, U)
	amd64AND_reg_imm32_32bit(cb, j65RegSR, 0x3D) // ~(0x80|0x40|0x02) = 0x3D

	// Z = (A AND operand) == 0
	amd64MOV_reg_reg32(cb, amd64RCX, j65RegA)
	emitREX(cb, false, amd64RAX, amd64RCX)
	cb.EmitBytes(0x21, modRM(3, amd64RAX, amd64RCX)) // AND ECX, EAX
	amd64TEST_reg_reg32(cb, amd64RCX, amd64RCX)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x02) // set Z
	patchRel32(cb, notZeroOff, cb.Len())

	// N = bit 7 of operand
	amd64TEST_reg_imm32(cb, amd64RAX, 0x80)
	notNegOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, int32(NEGATIVE_FLAG))
	patchRel32(cb, notNegOff, cb.Len())

	// V = bit 6 of operand
	amd64TEST_reg_imm32(cb, amd64RAX, 0x40)
	notOverflowOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64OR_reg_imm32_32bit(cb, j65RegSR, int32(OVERFLOW_FLAG))
	patchRel32(cb, notOverflowOff, cb.Len())
}

// ===========================================================================
// Block Compilation
// ===========================================================================

// compileBlock6502 compiles a scanned block of 6502 instructions to x86-64 machine code.
func compileBlock6502(instrs []JIT6502Instr, startPC uint16, execMem *ExecMem, codePageBitmap *[256]byte) (*JITBlock, error) {
	cb := NewCodeBuffer(len(instrs) * 64) // 6502 emits ~10-60 bytes per instruction

	hasBackward := jit6502DetectBackwardBranches(instrs, startPC)
	emit6502Prologue(cb, hasBackward)

	// Chain entry point — chained blocks JMP here, skipping the prologue
	chainEntryOff := emit6502ChainEntry(cb, hasBackward)

	var pendingCycles uint32
	var bails []bailInfo
	var invals []invalInfo
	var fwdFixups []branchFixup
	var extExits []branchExit
	var chainExits []j65ChainExitInfo
	var nz j65NZState

	instrOffsets := make([]int, len(instrs))

	// Pre-compute internal branch targets so we can flush pending cycles
	// at the start of any instruction that a branch might jump to.
	branchTargets := make([]bool, len(instrs))
	for _, ji := range instrs {
		switch ji.opcode {
		case 0x10, 0x30, 0x50, 0x70, 0x90, 0xB0, 0xD0, 0xF0:
			branchPC := startPC + ji.pcOffset + 2
			offset := int8(ji.operand & 0xFF)
			targetPC := uint16(int(branchPC) + int(offset))
			targetOff := int(targetPC) - int(startPC)
			if targetOff >= 0 {
				idx := findInstrByPC(instrs, uint16(targetOff))
				if idx >= 0 {
					branchTargets[idx] = true
				}
			}
		}
	}

	for i := range instrs {
		// Flush pending cycles and materialize deferred N/Z at branch targets
		// so forward/backward branches land with accurate state
		if branchTargets[i] {
			j65MaterializeNZ(cb, &nz)
			flushPendingCycles(cb, &pendingCycles)
		}

		instrOffsets[i] = cb.Len() // record native code offset for this instruction

		ji := &instrs[i]
		instrPC := startPC + ji.pcOffset
		baseCycles := uint32(jit6502BaseCycles[ji.opcode])
		nextPC := uint32(instrPC) + uint32(ji.length)

		switch ji.opcode {

		// ================================================================
		// NOP
		// ================================================================
		case 0xEA:
			emit6502NOP(cb)
			pendingCycles += baseCycles

		// ================================================================
		// LDA — Load Accumulator (all addressing modes)
		// ================================================================
		case 0xA9, 0xA5, 0xB5, 0xAD, 0xBD, 0xB9, 0xA1, 0xB1:
			isPageCross := ji.opcode == 0xBD || ji.opcode == 0xB9 || ji.opcode == 0xB1
			emit6502Load(cb, j65RegA, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, &bails, isPageCross, nz.nzPending, nz.nzReg)
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles

		// ================================================================
		// LDX — Load X Register
		// ================================================================
		case 0xA2, 0xA6, 0xB6, 0xAE, 0xBE:
			isPageCross := ji.opcode == 0xBE
			emit6502Load(cb, j65RegX, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, &bails, isPageCross, nz.nzPending, nz.nzReg)
			j65SetNZPending(&nz, j65RegX)
			pendingCycles += baseCycles

		// ================================================================
		// LDY — Load Y Register
		// ================================================================
		case 0xA0, 0xA4, 0xB4, 0xAC, 0xBC:
			isPageCross := ji.opcode == 0xBC
			emit6502Load(cb, j65RegY, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, &bails, isPageCross, nz.nzPending, nz.nzReg)
			j65SetNZPending(&nz, j65RegY)
			pendingCycles += baseCycles

		// ================================================================
		// STA — Store Accumulator
		// ================================================================
		case 0x85, 0x95, 0x8D, 0x9D, 0x99, 0x81, 0x91:
			emit6502Store(cb, j65RegA, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, baseCycles, &bails, &invals, nextPC, nz.nzPending, nz.nzReg)
			pendingCycles += baseCycles

		// ================================================================
		// STX — Store X Register
		// ================================================================
		case 0x86, 0x96, 0x8E:
			emit6502Store(cb, j65RegX, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, baseCycles, &bails, &invals, nextPC, nz.nzPending, nz.nzReg)
			pendingCycles += baseCycles

		// ================================================================
		// STY — Store Y Register
		// ================================================================
		case 0x84, 0x94, 0x8C:
			emit6502Store(cb, j65RegY, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, baseCycles, &bails, &invals, nextPC, nz.nzPending, nz.nzReg)
			pendingCycles += baseCycles

		// ================================================================
		// ADC — Add with Carry (binary mode; decimal bails)
		// ================================================================
		case 0x69, 0x65, 0x75, 0x6D, 0x7D, 0x79, 0x61, 0x71:
			isPageCross := ji.opcode == 0x7D || ji.opcode == 0x79 || ji.opcode == 0x71
			decBailOff := emit6502DecimalBailCheck(cb)
			bails = append(bails, bailInfo{[]int{decBailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			emit6502LoadOperandToEAX(cb, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, &bails, isPageCross, nz.nzPending, nz.nzReg)
			emit6502ADCFlags(cb)
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles

		// ================================================================
		// SBC — Subtract with Carry (binary mode; decimal bails)
		// ================================================================
		case 0xE9, 0xE5, 0xF5, 0xED, 0xFD, 0xF9, 0xE1, 0xF1:
			isPageCross := ji.opcode == 0xFD || ji.opcode == 0xF9 || ji.opcode == 0xF1
			decBailOff := emit6502DecimalBailCheck(cb)
			bails = append(bails, bailInfo{[]int{decBailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			emit6502LoadOperandToEAX(cb, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, &bails, isPageCross, nz.nzPending, nz.nzReg)
			emit6502SBCFlags(cb)
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles

		// ================================================================
		// AND — Logical AND
		// ================================================================
		case 0x29, 0x25, 0x35, 0x2D, 0x3D, 0x39, 0x21, 0x31:
			isPageCross := ji.opcode == 0x3D || ji.opcode == 0x39 || ji.opcode == 0x31
			emit6502LoadOperandToEAX(cb, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, &bails, isPageCross, nz.nzPending, nz.nzReg)
			emit6502LogicOp(cb, 0x21) // AND
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles

		// ================================================================
		// ORA — Logical OR
		// ================================================================
		case 0x09, 0x05, 0x15, 0x0D, 0x1D, 0x19, 0x01, 0x11:
			isPageCross := ji.opcode == 0x1D || ji.opcode == 0x19 || ji.opcode == 0x11
			emit6502LoadOperandToEAX(cb, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, &bails, isPageCross, nz.nzPending, nz.nzReg)
			emit6502LogicOp(cb, 0x09) // OR
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles

		// ================================================================
		// EOR — Exclusive OR
		// ================================================================
		case 0x49, 0x45, 0x55, 0x4D, 0x5D, 0x59, 0x41, 0x51:
			isPageCross := ji.opcode == 0x5D || ji.opcode == 0x59 || ji.opcode == 0x51
			emit6502LoadOperandToEAX(cb, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, &bails, isPageCross, nz.nzPending, nz.nzReg)
			emit6502LogicOp(cb, 0x31) // XOR
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles

		// ================================================================
		// CMP — Compare Accumulator
		// ================================================================
		case 0xC9, 0xC5, 0xD5, 0xCD, 0xDD, 0xD9, 0xC1, 0xD1:
			isPageCross := ji.opcode == 0xDD || ji.opcode == 0xD9 || ji.opcode == 0xD1
			emit6502LoadOperandToEAX(cb, ji.opcode, ji.operand,
				uint32(instrPC), i, pendingCycles, &bails, isPageCross, nz.nzPending, nz.nzReg)
			emit6502CompareFlags(cb, j65RegA)
			nz.nzPending = false // CMP eagerly sets N/Z in R15
			pendingCycles += baseCycles

		// ================================================================
		// CPX — Compare X Register
		// ================================================================
		case 0xE0, 0xE4, 0xEC:
			// CPX uses group 00 encoding, not group 01. Handle manually.
			switch ji.opcode {
			case 0xE0: // immediate
				amd64MOV_reg_imm32(cb, amd64RAX, uint32(ji.operand&0xFF))
			case 0xE4: // zero page
				emit6502AddrZP(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
				amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
			case 0xEC: // absolute
				emit6502AddrAbs(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
				amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
			}
			emit6502CompareFlags(cb, j65RegX)
			nz.nzPending = false // CPX eagerly sets N/Z
			pendingCycles += baseCycles

		// ================================================================
		// CPY — Compare Y Register
		// ================================================================
		case 0xC0, 0xC4, 0xCC:
			switch ji.opcode {
			case 0xC0: // immediate
				amd64MOV_reg_imm32(cb, amd64RAX, uint32(ji.operand&0xFF))
			case 0xC4: // zero page
				emit6502AddrZP(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
				amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
			case 0xCC: // absolute
				emit6502AddrAbs(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
				amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
			}
			emit6502CompareFlags(cb, j65RegY)
			nz.nzPending = false // CPY eagerly sets N/Z
			pendingCycles += baseCycles

		// ================================================================
		// INC/DEC Memory
		// ================================================================
		case 0xE6, 0xF6, 0xEE, 0xFE: // INC zp, zp+X, abs, abs+X
			switch ji.opcode {
			case 0xE6: // INC zp
				emit6502AddrZP(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0xF6: // INC zp,X
				emit6502AddrZPX(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0xEE: // INC abs
				emit6502AddrAbs(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0xFE: // INC abs,X
				emit6502AddrAbsX(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			}
			emit6502IncDecMem(cb, true)
			nz.nzPending = false // INC mem eagerly sets N/Z
			invalOff := emit6502SelfModCheck(cb)
			invals = append(invals, invalInfo{[]int{invalOff}, nextPC, i + 1, pendingCycles + baseCycles, nz.nzPending, nz.nzReg})
			pendingCycles += baseCycles

		case 0xC6, 0xD6, 0xCE, 0xDE: // DEC zp, zp+X, abs, abs+X
			switch ji.opcode {
			case 0xC6:
				emit6502AddrZP(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0xD6:
				emit6502AddrZPX(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0xCE:
				emit6502AddrAbs(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0xDE:
				emit6502AddrAbsX(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			}
			emit6502IncDecMem(cb, false)
			nz.nzPending = false // DEC mem eagerly sets N/Z
			invalOff := emit6502SelfModCheck(cb)
			invals = append(invals, invalInfo{[]int{invalOff}, nextPC, i + 1, pendingCycles + baseCycles, nz.nzPending, nz.nzReg})
			pendingCycles += baseCycles

		// ================================================================
		// INX/INY/DEX/DEY — Register Increment/Decrement
		// ================================================================
		case 0xE8: // INX
			amd64ALU_reg_imm32_32bit(cb, 0, j65RegX, 1) // ADD EBP, 1
			amd64AND_reg_imm32_32bit(cb, j65RegX, 0xFF)
			j65SetNZPending(&nz, j65RegX)
			pendingCycles += baseCycles
		case 0xC8: // INY
			amd64ALU_reg_imm32_32bit(cb, 0, j65RegY, 1)
			amd64AND_reg_imm32_32bit(cb, j65RegY, 0xFF)
			j65SetNZPending(&nz, j65RegY)
			pendingCycles += baseCycles
		case 0xCA: // DEX
			amd64ALU_reg_imm32_32bit(cb, 5, j65RegX, 1) // SUB EBP, 1
			amd64AND_reg_imm32_32bit(cb, j65RegX, 0xFF)
			j65SetNZPending(&nz, j65RegX)
			pendingCycles += baseCycles
		case 0x88: // DEY
			amd64ALU_reg_imm32_32bit(cb, 5, j65RegY, 1)
			amd64AND_reg_imm32_32bit(cb, j65RegY, 0xFF)
			j65SetNZPending(&nz, j65RegY)
			pendingCycles += baseCycles

		// ================================================================
		// BIT — Bit Test
		// ================================================================
		case 0x24: // BIT zp
			emit6502AddrZP(cb, byte(ji.operand))
			bailOff := emit6502ZPPageCheck(cb, 0)
			bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
			emit6502BITFlags(cb)
			nz.nzPending = false // BIT eagerly sets N/Z/V
			pendingCycles += baseCycles
		case 0x2C: // BIT abs
			emit6502AddrAbs(cb, ji.operand)
			fpOff := emit6502FullFastPathCheck(cb)
			bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			amd64MOVZX_B_memSIB(cb, amd64RAX, j65RegMem, amd64RAX)
			emit6502BITFlags(cb)
			nz.nzPending = false // BIT eagerly sets N/Z/V
			pendingCycles += baseCycles

		// ================================================================
		// ASL — Arithmetic Shift Left
		// ================================================================
		case 0x0A: // ASL A
			emit6502ASL_A(cb)
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles
		case 0x06, 0x16, 0x0E, 0x1E: // ASL memory
			switch ji.opcode {
			case 0x06:
				emit6502AddrZP(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x16:
				emit6502AddrZPX(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x0E:
				emit6502AddrAbs(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x1E:
				emit6502AddrAbsX(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			}
			emit6502ASL_Mem(cb)
			nz.nzPending = false // memory shift eagerly sets N/Z
			invalOff := emit6502SelfModCheck(cb)
			invals = append(invals, invalInfo{[]int{invalOff}, nextPC, i + 1, pendingCycles + baseCycles, nz.nzPending, nz.nzReg})
			pendingCycles += baseCycles

		// ================================================================
		// LSR — Logical Shift Right
		// ================================================================
		case 0x4A: // LSR A
			emit6502LSR_A(cb)
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles
		case 0x46, 0x56, 0x4E, 0x5E: // LSR memory
			switch ji.opcode {
			case 0x46:
				emit6502AddrZP(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x56:
				emit6502AddrZPX(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x4E:
				emit6502AddrAbs(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x5E:
				emit6502AddrAbsX(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			}
			emit6502LSR_Mem(cb)
			nz.nzPending = false
			invalOff := emit6502SelfModCheck(cb)
			invals = append(invals, invalInfo{[]int{invalOff}, nextPC, i + 1, pendingCycles + baseCycles, nz.nzPending, nz.nzReg})
			pendingCycles += baseCycles

		// ================================================================
		// ROL — Rotate Left through Carry
		// ================================================================
		case 0x2A: // ROL A
			emit6502ROL_A(cb)
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles
		case 0x26, 0x36, 0x2E, 0x3E: // ROL memory
			switch ji.opcode {
			case 0x26:
				emit6502AddrZP(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x36:
				emit6502AddrZPX(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x2E:
				emit6502AddrAbs(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x3E:
				emit6502AddrAbsX(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			}
			emit6502ROL_Mem(cb)
			nz.nzPending = false
			invalOff := emit6502SelfModCheck(cb)
			invals = append(invals, invalInfo{[]int{invalOff}, nextPC, i + 1, pendingCycles + baseCycles, nz.nzPending, nz.nzReg})
			pendingCycles += baseCycles

		// ================================================================
		// ROR — Rotate Right through Carry
		// ================================================================
		case 0x6A: // ROR A
			emit6502ROR_A(cb)
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles
		case 0x66, 0x76, 0x6E, 0x7E: // ROR memory
			switch ji.opcode {
			case 0x66:
				emit6502AddrZP(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x76:
				emit6502AddrZPX(cb, byte(ji.operand))
				bailOff := emit6502ZPPageCheck(cb, 0)
				bails = append(bails, bailInfo{[]int{bailOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x6E:
				emit6502AddrAbs(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			case 0x7E:
				emit6502AddrAbsX(cb, ji.operand)
				fpOff := emit6502FullFastPathCheck(cb)
				bails = append(bails, bailInfo{[]int{fpOff}, uint32(instrPC), i, pendingCycles, nz.nzPending, nz.nzReg})
			}
			emit6502ROR_Mem(cb)
			nz.nzPending = false
			invalOff := emit6502SelfModCheck(cb)
			invals = append(invals, invalInfo{[]int{invalOff}, nextPC, i + 1, pendingCycles + baseCycles, nz.nzPending, nz.nzReg})
			pendingCycles += baseCycles

		// ================================================================
		// Stack Operations
		// ================================================================
		case 0x48: // PHA
			emit6502PHA(cb)
			pendingCycles += baseCycles
		case 0x68: // PLA
			emit6502PLA(cb)
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles
		case 0x08: // PHP
			j65MaterializeNZ(cb, &nz) // PHP reads full SR
			emit6502PHP(cb)
			pendingCycles += baseCycles
		case 0x28: // PLP
			emit6502PLP(cb)
			nz.nzPending = false // PLP overwrites entire SR
			pendingCycles += baseCycles

		// ================================================================
		// Flag Instructions
		// ================================================================
		case 0x18: // CLC
			amd64AND_reg_imm32_32bit(cb, j65RegSR, 0xFE) // clear C
			pendingCycles += baseCycles
		case 0x38: // SEC
			amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x01) // set C
			pendingCycles += baseCycles
		case 0x58: // CLI
			amd64AND_reg_imm32_32bit(cb, j65RegSR, 0xFB) // clear I
			pendingCycles += baseCycles
		case 0x78: // SEI
			amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x04) // set I
			pendingCycles += baseCycles
		case 0xD8: // CLD
			amd64AND_reg_imm32_32bit(cb, j65RegSR, 0xF7) // clear D
			pendingCycles += baseCycles
		case 0xF8: // SED
			amd64OR_reg_imm32_32bit(cb, j65RegSR, 0x08) // set D
			pendingCycles += baseCycles
		case 0xB8: // CLV
			amd64AND_reg_imm32_32bit(cb, j65RegSR, 0xBF) // clear V
			pendingCycles += baseCycles

		// ================================================================
		// Conditional Branches
		// ================================================================
		case 0x90, 0xB0, 0xF0, 0xD0, 0x30, 0x10, 0x70, 0x50:
			// Flush pending cycles before any branch. This is critical for
			// backward branches (loops): each iteration must commit its
			// instruction cycles to the runtime accumulator [RSP+16] before
			// the branch adds its taken penalty and jumps back.
			flushPendingCycles(cb, &pendingCycles)
			emit6502ConditionalBranch(cb, ji, startPC, i, pendingCycles,
				instrs, instrOffsets, hasBackward, &fwdFixups, &extExits, &nz)
			// No pendingCycles += baseCycles (baseCycles is 0 for branches)

		// ================================================================
		// JMP Absolute ($4C) — Block Terminator
		// ================================================================
		case 0x4C:
			pendingCycles += baseCycles
			ce := emit6502JMP_Abs(cb, ji.operand, uint32(i+1), &pendingCycles, nz.nzPending, nz.nzReg)
			chainExits = append(chainExits, ce)
			goto done

		// ================================================================
		// JMP Indirect ($6C) — Block Terminator
		// ================================================================
		case 0x6C:
			pendingCycles += baseCycles
			emit6502JMP_Ind(cb, ji.operand, uint32(i+1), &pendingCycles,
				&bails, uint32(instrPC), i, pendingCycles,
				nz.nzPending, nz.nzReg)
			goto done

		// ================================================================
		// JSR ($20) — Block Terminator
		// ================================================================
		case 0x20:
			pendingCycles += baseCycles
			returnAddr := instrPC + 2 // address of last byte of JSR instruction
			ce := emit6502JSR(cb, ji.operand, returnAddr, uint32(i+1), &pendingCycles, nz.nzPending, nz.nzReg)
			chainExits = append(chainExits, ce)
			goto done

		// ================================================================
		// RTS ($60) — Block Terminator
		// ================================================================
		case 0x60:
			pendingCycles += baseCycles
			emit6502RTS(cb, uint32(i+1), &pendingCycles, nz.nzPending, nz.nzReg)
			goto done

		// ================================================================
		// Transfer Instructions
		// ================================================================
		case 0xAA: // TAX
			emit6502Transfer(cb, j65RegX, j65RegA, false)
			j65SetNZPending(&nz, j65RegX)
			pendingCycles += baseCycles
		case 0x8A: // TXA
			emit6502Transfer(cb, j65RegA, j65RegX, false)
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles
		case 0xA8: // TAY
			emit6502Transfer(cb, j65RegY, j65RegA, false)
			j65SetNZPending(&nz, j65RegY)
			pendingCycles += baseCycles
		case 0x98: // TYA
			emit6502Transfer(cb, j65RegA, j65RegY, false)
			j65SetNZPending(&nz, j65RegA)
			pendingCycles += baseCycles
		case 0xBA: // TSX
			emit6502Transfer(cb, j65RegX, j65RegSP, false)
			j65SetNZPending(&nz, j65RegX)
			pendingCycles += baseCycles
		case 0x9A: // TXS (no flag update)
			emit6502Transfer(cb, j65RegSP, j65RegX, false)
			pendingCycles += baseCycles

		default:
			// Unimplemented opcode — bail to interpreter.
			flushPendingCycles(cb, &pendingCycles)
			emit6502BailEpilogue(cb, uint32(instrPC), uint32(i), 0, nz.nzPending, nz.nzReg)
			goto done
		}
	}

	// Default fallthrough: block ended without a terminator (size limit, next opcode not compilable).
	// Emit a chain exit to the next sequential PC so straight-line code chains block to block.
	{
		lastInstr := &instrs[len(instrs)-1]
		endPC := uint32(startPC) + uint32(lastInstr.pcOffset) + uint32(lastInstr.length)
		ce := emit6502ChainExit(cb, endPC, uint32(len(instrs)), &pendingCycles, nz.nzPending, nz.nzReg)
		chainExits = append(chainExits, ce)
	}

done:
	// Patch forward branch fixups
	for _, bf := range fwdFixups {
		if bf.targetInstrIdx < len(instrOffsets) {
			patchRel32(cb, bf.jmpOffset, instrOffsets[bf.targetInstrIdx])
		}
	}

	// Emit deferred branch exit epilogues (external branch targets) as chain exits
	for _, be := range extExits {
		target := cb.Len()
		patchRel32(cb, be.jmpOffset, target)
		ce := emit6502ChainExit(cb, be.targetPC, be.instrCount, &be.pendingCycles, be.nzPending, be.nzReg)
		chainExits = append(chainExits, ce)
	}

	// Emit deferred bail epilogues (cold code at end of block)
	for _, bi := range bails {
		target := cb.Len()
		for _, off := range bi.offsets {
			patchRel32(cb, off, target)
		}
		emit6502BailEpilogue(cb, bi.instrPC, uint32(bi.instrIdx), bi.pendingCycles, bi.nzPending, bi.nzReg)
	}

	// Emit deferred NeedInval epilogues
	for _, ii := range invals {
		target := cb.Len()
		for _, off := range ii.offsets {
			patchRel32(cb, off, target)
		}
		emit6502InvalEpilogue(cb, ii.nextPC, uint32(ii.instrIdx), ii.pendingCycles, ii.nzPending, ii.nzReg)
	}
	cb.Resolve()
	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, err
	}

	// Mark code pages in bitmap. Use lastByte (inclusive) to avoid marking
	// the next page when the block ends exactly on a page boundary.
	lastByte := startPC + instrs[len(instrs)-1].pcOffset + uint16(instrs[len(instrs)-1].length) - 1
	for page := startPC >> 8; page <= lastByte>>8; page++ {
		codePageBitmap[page&0xFF] = 1
	}

	// Build chain slots from chain exit info (convert CodeBuffer offsets to absolute addresses)
	var slots []chainSlot
	for _, ce := range chainExits {
		slots = append(slots, chainSlot{
			targetPC:  ce.targetPC,
			patchAddr: addr + uintptr(ce.jmpDispOffset),
		})
	}

	return &JITBlock{
		startPC:    uint32(startPC),
		endPC:      uint32(lastByte) + 1, // first byte after block (for cache invalidation)
		instrCount: len(instrs),
		execAddr:   addr,
		execSize:   len(code),
		chainEntry: addr + uintptr(chainEntryOff),
		chainSlots: slots,
	}, nil
}
