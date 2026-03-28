// jit_m68k_emit_amd64.go - x86-64 native code emitter for M68020 JIT compiler

//go:build amd64 && linux

package main

// ===========================================================================
// x86-64 Register Mapping for M68K JIT
// ===========================================================================
//
// x86-64  M68K    Purpose
// ------  ------  -------
// RDI     --      &cpu.DataRegs[0] (data register file base)
// RSI     --      &cpu.memory[0] (memory base)
// R8      --      IOThreshold (0xA0000)
// R9      --      &cpu.AddrRegs[0] (address register file base)
// RAX     --      Scratch
// RCX     --      Scratch / shift count (CL)
// RDX     --      Scratch
// R10     --      Scratch
// R11     --      Scratch
// RBX     D0      Mapped (callee-saved)
// RBP     D1      Mapped (callee-saved)
// R12     A0      Mapped (callee-saved)
// R13     A7/SP   Mapped (callee-saved)
// R14     CCR     Condition code register (callee-saved, 5-bit XNZVC)
// R15     --      JITContext pointer (callee-saved)

// M68K register mapping: which M68K registers live in x86-64 registers
const (
	m68kAMD64RegD0       = amd64RBX // D0 -> RBX (callee-saved)
	m68kAMD64RegD1       = amd64RBP // D1 -> RBP (callee-saved)
	m68kAMD64RegA0       = amd64R12 // A0 -> R12 (callee-saved)
	m68kAMD64RegA7       = amd64R13 // A7 -> R13 (callee-saved)
	m68kAMD64RegCCR      = amd64R14 // CCR -> R14 (callee-saved)
	m68kAMD64RegCtx      = amd64R15 // JITContext -> R15 (callee-saved)
	m68kAMD64RegDataBase = amd64RDI // &DataRegs[0]
	m68kAMD64RegMemBase  = amd64RSI // &memory[0]
	m68kAMD64RegIOThresh = amd64R8  // IOThreshold
	m68kAMD64RegAddrBase = amd64R9  // &AddrRegs[0]

	// Stack frame: 6 callee-saved pushes (48 bytes) + SUB RSP,24 = 72 + 8 (ret addr) = 80 (16-aligned)
	m68kAMD64FrameSize    = 24
	m68kAMD64OffCtxPtr    = 0  // [RSP+0] = saved JITContext pointer (backup)
	m68kAMD64OffSRPtr     = 8  // [RSP+8] = SR pointer
	m68kAMD64OffLoopCount = 16 // [RSP+16] = backward branch loop counter
)

// M68K backward branch budget
const m68kJitBudget = 4095

// m68kDataRegToAMD64 maps an M68K data register (0-7) to an x86-64 register.
// Returns the x86-64 register and whether it's mapped (resident).
func m68kDataRegToAMD64(dreg uint16) (byte, bool) {
	switch dreg {
	case 0:
		return m68kAMD64RegD0, true
	case 1:
		return m68kAMD64RegD1, true
	}
	return 0, false
}

// m68kAddrRegToAMD64 maps an M68K address register (0-7) to an x86-64 register.
func m68kAddrRegToAMD64(areg uint16) (byte, bool) {
	switch areg {
	case 0:
		return m68kAMD64RegA0, true
	case 7:
		return m68kAMD64RegA7, true
	}
	return 0, false
}

// m68kResolveDataReg ensures a data register value is in an x86-64 register.
// For mapped: returns directly. For spilled: loads from [RDI + reg*4].
func m68kResolveDataReg(cb *CodeBuffer, dreg uint16, scratch byte) byte {
	r, mapped := m68kDataRegToAMD64(dreg)
	if mapped {
		return r
	}
	amd64MOV_reg_mem32(cb, scratch, m68kAMD64RegDataBase, int32(dreg)*4)
	return scratch
}

// m68kResolveAddrReg ensures an address register value is in an x86-64 register.
func m68kResolveAddrReg(cb *CodeBuffer, areg uint16, scratch byte) byte {
	r, mapped := m68kAddrRegToAMD64(areg)
	if mapped {
		return r
	}
	amd64MOV_reg_mem32(cb, scratch, m68kAMD64RegAddrBase, int32(areg)*4)
	return scratch
}

// m68kStoreDataReg stores a value from an x86-64 register to an M68K data register.
func m68kStoreDataReg(cb *CodeBuffer, dreg uint16, src byte) {
	r, mapped := m68kDataRegToAMD64(dreg)
	if mapped {
		if r != src {
			amd64MOV_reg_reg32(cb, r, src)
		}
		return
	}
	amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, int32(dreg)*4, src)
}

// m68kStoreAddrReg stores a value from an x86-64 register to an M68K address register.
func m68kStoreAddrReg(cb *CodeBuffer, areg uint16, src byte) {
	r, mapped := m68kAddrRegToAMD64(areg)
	if mapped {
		if r != src {
			amd64MOV_reg_reg32(cb, r, src)
		}
		return
	}
	amd64MOV_mem_reg32(cb, m68kAMD64RegAddrBase, int32(areg)*4, src)
}

// ===========================================================================
// CCR Helpers — Extract and Build XNZVC from x86-64 native flags
// ===========================================================================

// M68K CCR bit positions (matching SR lower 5 bits)
const (
	m68kCCR_C = 0x01 // bit 0: Carry
	m68kCCR_V = 0x02 // bit 1: Overflow
	m68kCCR_Z = 0x04 // bit 2: Zero
	m68kCCR_N = 0x08 // bit 3: Negative
	m68kCCR_X = 0x10 // bit 4: Extend
)

// amd64SETcc emits SETcc r/m8 (set byte based on condition).
func amd64SETcc(cb *CodeBuffer, cond byte, dst byte) {
	if isExtReg(dst) {
		cb.EmitBytes(rexByte(false, false, false, true))
	}
	cb.EmitBytes(0x0F, 0x90+cond, modRM(3, 0, dst))
}

// amd64TEST_reg_imm8 emits TEST r/m8, imm8 (for single register).
func amd64TEST_reg_imm8(cb *CodeBuffer, reg byte, imm8 byte) {
	if reg == amd64RAX {
		cb.EmitBytes(0xA8, imm8) // short form: TEST AL, imm8
	} else {
		if isExtReg(reg) {
			cb.EmitBytes(rexByte(false, false, false, true))
		}
		cb.EmitBytes(0xF6, modRM(3, 0, reg), imm8)
	}
}

// amd64TEST_reg_reg32 emits TEST r32, r32 (sets NZ flags from result, clears CV).
func amd64TEST_reg_reg32(cb *CodeBuffer, r1, r2 byte) {
	emitREX(cb, false, r1, r2)
	cb.EmitBytes(0x85, modRM(3, r1, r2))
}

// emitCCR_Arithmetic builds CCR (XNZVC) in R14 from x86-64 native flags after ADD/SUB/CMP/NEG.
// All 5 flags updated. X = C for arithmetic operations.
// CRITICAL: SETcc does NOT modify flags, but SHL/OR DO. So we must extract
// all 4 flags via SETcc FIRST, then combine them.
func emitCCR_Arithmetic(cb *CodeBuffer) {
	// Extract all flags via SETcc before any SHL/OR clobbers EFLAGS
	amd64SETcc(cb, amd64CondB, amd64RCX) // CL = C (carry)
	amd64SETcc(cb, amd64CondO, amd64RDX) // DL = V (overflow)
	amd64SETcc(cb, amd64CondE, amd64R10) // R10B = Z (zero)
	amd64SETcc(cb, 0x8, amd64R11)        // R11B = N (sign/negative)

	// Build R14 = X(4) | N(3) | Z(2) | V(1) | C(0)
	amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX) // R14 = C

	amd64MOVZX_B(cb, amd64RAX, amd64RDX) // EAX = V
	amd64SHL_imm(cb, amd64RAX, 1)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // R14 |= V<<1

	amd64MOVZX_B(cb, amd64RAX, amd64R10) // EAX = Z
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // R14 |= Z<<2

	amd64MOVZX_B(cb, amd64RAX, amd64R11) // EAX = N
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // R14 |= N<<3

	// X = C for arithmetic ops → copy bit 0 to bit 4
	amd64MOV_reg_reg(cb, amd64RAX, m68kAMD64RegCCR)
	amd64ALU_reg_imm32(cb, 4, amd64RAX, 0x01) // AND RAX, 1 (isolate C)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // R14 |= X<<4
}

// emitCCR_Logic builds CCR from x86-64 native flags after AND/OR/EOR/NOT/MOVE/CLR/TST.
// N and Z from result. V=0, C=0. X unchanged.
// Expects: native flags set by TEST or the logical operation itself.
// CRITICAL: Extract flags via SETcc BEFORE any AND/SHL/OR clobbers them.
func emitCCR_Logic(cb *CodeBuffer) {
	// Extract Z and N BEFORE any flag-clobbering operations
	amd64SETcc(cb, amd64CondE, amd64RCX) // CL = Z
	amd64SETcc(cb, 0x8, amd64RDX)        // DL = N (SETS)

	// Preserve X (bit 4) from current CCR, clear NZVC
	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X) // R14 &= 0x10 (keep only X)

	// Z → bit 2
	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // R14 |= Z<<2

	// N → bit 3
	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // R14 |= N<<3

	// V=0, C=0 already ensured by the AND with 0x10 above
}

// ===========================================================================
// Block Prologue / Epilogue
// ===========================================================================

func m68kEmitPrologue(cb *CodeBuffer, startPC uint32, br *m68kBlockRegs) {
	// Save callee-saved registers
	amd64PUSH(cb, amd64RBX)
	amd64PUSH(cb, amd64RBP)
	amd64PUSH(cb, amd64R12)
	amd64PUSH(cb, amd64R13)
	amd64PUSH(cb, amd64R14)
	amd64PUSH(cb, amd64R15)

	// Allocate stack frame
	amd64ALU_reg_imm32(cb, 5, amd64RSP, int32(m68kAMD64FrameSize)) // SUB RSP, 24

	// Save JITContext pointer to R15 (callee-saved)
	amd64MOV_reg_reg(cb, m68kAMD64RegCtx, amd64RDI) // R15 = RDI (context)

	// Load base pointers from M68KJITContext
	amd64MOV_reg_mem(cb, m68kAMD64RegDataBase, amd64RDI, int32(m68kCtxOffDataRegsPtr))          // RDI -> RAX temp
	amd64MOV_reg_mem(cb, m68kAMD64RegMemBase, m68kAMD64RegCtx, int32(m68kCtxOffMemPtr))         // RSI = MemPtr
	amd64MOV_reg_mem32(cb, m68kAMD64RegIOThresh, m68kAMD64RegCtx, int32(m68kCtxOffIOThreshold)) // R8d = IOThreshold
	amd64MOV_reg_mem(cb, m68kAMD64RegAddrBase, m68kAMD64RegCtx, int32(m68kCtxOffAddrRegsPtr))   // R9 = AddrRegsPtr

	// Save SR pointer to stack
	amd64MOV_reg_mem(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffSRPtr))
	amd64MOV_mem_reg(cb, amd64RSP, int32(m68kAMD64OffSRPtr), amd64RAX) // [RSP+8] = SRPtr

	// Load DataRegsPtr into RDI (was loaded above but then overwritten)
	amd64MOV_reg_mem(cb, m68kAMD64RegDataBase, m68kAMD64RegCtx, int32(m68kCtxOffDataRegsPtr))

	// Zero loop counter if needed
	if br.hasBackwardBranch {
		amd64MOV_mem_imm32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), 0)
	}

	// Load mapped data registers (32-bit values, zero-extended)
	if br.dataRead&(1<<0) != 0 || br.dataWritten&(1<<0) != 0 {
		amd64MOV_reg_mem32(cb, m68kAMD64RegD0, m68kAMD64RegDataBase, 0*4) // D0 -> RBX
	}
	if br.dataRead&(1<<1) != 0 || br.dataWritten&(1<<1) != 0 {
		amd64MOV_reg_mem32(cb, m68kAMD64RegD1, m68kAMD64RegDataBase, 1*4) // D1 -> RBP
	}

	// Load mapped address registers
	if br.addrRead&(1<<0) != 0 || br.addrWritten&(1<<0) != 0 {
		amd64MOV_reg_mem32(cb, m68kAMD64RegA0, m68kAMD64RegAddrBase, 0*4) // A0 -> R12
	}
	if br.addrRead&(1<<7) != 0 || br.addrWritten&(1<<7) != 0 {
		amd64MOV_reg_mem32(cb, m68kAMD64RegA7, m68kAMD64RegAddrBase, 7*4) // A7 -> R13
	}

	// Extract CCR from SR: R14 = *SRPtr & 0x1F
	amd64MOV_reg_mem(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffSRPtr)) // RAX = SRPtr
	amd64MOV_reg_mem32(cb, m68kAMD64RegCCR, amd64RAX, 0)               // R14d = *SRPtr (SR is uint16, load as 32-bit)
	amd64MOVZX_W(cb, m68kAMD64RegCCR, m68kAMD64RegCCR)                 // zero-extend from word
	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, 0x1F)                   // R14 &= 0x1F (extract XNZVC)
}

func m68kEmitEpilogue(cb *CodeBuffer, br *m68kBlockRegs) {
	// Store mapped data registers back
	if br.dataWritten&(1<<0) != 0 {
		amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, 0*4, m68kAMD64RegD0)
	}
	if br.dataWritten&(1<<1) != 0 {
		amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, 1*4, m68kAMD64RegD1)
	}

	// Store mapped address registers back
	if br.addrWritten&(1<<0) != 0 {
		amd64MOV_mem_reg32(cb, m68kAMD64RegAddrBase, 0*4, m68kAMD64RegA0)
	}
	if br.addrWritten&(1<<7) != 0 {
		amd64MOV_mem_reg32(cb, m68kAMD64RegAddrBase, 7*4, m68kAMD64RegA7)
	}

	// Merge CCR back into SR: *SRPtr = (*SRPtr & 0xFFE0) | (R14 & 0x1F)
	amd64MOV_reg_mem(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffSRPtr)) // RAX = SRPtr
	amd64MOV_reg_mem32(cb, amd64RDX, amd64RAX, 0)                      // EDX = *SRPtr
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, int32(0xFFE0))           // EDX &= 0xFFE0
	amd64MOV_reg_reg32(cb, amd64RCX, m68kAMD64RegCCR)                  // ECX = R14
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 0x1F)                    // ECX &= 0x1F
	amd64ALU_reg_reg32(cb, 0x09, amd64RDX, amd64RCX)                   // EDX |= ECX
	// Store as 16-bit (SR is uint16) — use MOV [RAX], DX (16-bit prefix)
	cb.EmitBytes(0x66) // operand-size prefix for 16-bit
	emitREX(cb, false, amd64RDX, amd64RAX)
	cb.EmitBytes(0x89, modRM(0, amd64RDX, amd64RAX)) // MOV [RAX], DX

	// Write RetPC and RetCount to context
	// (set by instruction emitters before jumping to epilogue)

	// Deallocate stack frame
	amd64ALU_reg_imm32(cb, 0, amd64RSP, int32(m68kAMD64FrameSize)) // ADD RSP, 24

	// Restore callee-saved registers (reverse order)
	amd64POP(cb, amd64R15)
	amd64POP(cb, amd64R14)
	amd64POP(cb, amd64R13)
	amd64POP(cb, amd64R12)
	amd64POP(cb, amd64RBP)
	amd64POP(cb, amd64RBX)

	amd64RET(cb)
}

// m68kEmitRetPC writes RetPC and RetCount to the JITContext before epilogue.
func m68kEmitRetPC(cb *CodeBuffer, pc uint32, count uint32) {
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), pc)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), count)
}

// ===========================================================================
// Instruction Emitters — Stage 2: Register ALU
// ===========================================================================

// m68kEmitMOVEQ emits MOVEQ #data,Dn.
// Sign-extends 8-bit immediate to 32 bits, stores to data register.
// Flags: N and Z from result, V=0, C=0, X unchanged.
func m68kEmitMOVEQ(cb *CodeBuffer, opcode uint16) {
	reg := (opcode >> 9) & 7
	data := int32(int8(opcode & 0xFF))

	if data == 0 {
		// XOR reg, reg — also sets x86 ZF=1, SF=0
		r, mapped := m68kDataRegToAMD64(reg)
		if mapped {
			amd64XOR_reg_reg32(cb, r, r)
		} else {
			amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)
			m68kStoreDataReg(cb, reg, amd64RAX)
		}
		// Emit TEST to set flags (XOR already set them correctly)
	} else {
		r, mapped := m68kDataRegToAMD64(reg)
		if mapped {
			amd64MOV_reg_imm32(cb, r, uint32(data))
		} else {
			amd64MOV_reg_imm32(cb, amd64RAX, uint32(data))
			m68kStoreDataReg(cb, reg, amd64RAX)
		}
	}

	// Set flags: TEST result to set NZ, clear VC
	r := m68kResolveDataReg(cb, reg, amd64RAX)
	amd64TEST_reg_reg32(cb, r, r)
	emitCCR_Logic(cb)
}

// m68kEmitMOVE_Dn_Dn emits MOVE.x Ds,Dd (register-to-register).
// size: 0=byte, 1=word, 2=long.
func m68kEmitMOVE_Dn_Dn(cb *CodeBuffer, opcode uint16, size int) {
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7 // MOVE dest: reg at bits 11-9

	src := m68kResolveDataReg(cb, srcReg, amd64RAX)

	// Apply size masking to scratch
	amd64MOV_reg_reg32(cb, amd64RDX, src) // EDX = source value
	switch size {
	case M68K_SIZE_BYTE:
		amd64MOVZX_B(cb, amd64RDX, amd64RDX) // zero-extend byte
	case M68K_SIZE_WORD:
		amd64MOVZX_W(cb, amd64RDX, amd64RDX) // zero-extend word
	}
	// For LONG, the 32-bit MOV already zero-extends upper 32

	m68kStoreDataReg(cb, dstReg, amd64RDX)

	// Flags: N,Z from result, V=0, C=0, X unchanged
	amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
	emitCCR_Logic(cb)
}

// m68kEmitADD_Dn_Dn emits ADD.L Ds,Dd (register-to-register, long only for now).
// Flags: all 5 (XNZVC) updated.
func m68kEmitADD_Dn_Dn(cb *CodeBuffer, opcode uint16) {
	// ADD: opmode bits 8-6 determine direction and size
	// opmode 0-2: EA + Dn → Dn (size = opmode)
	// opmode 4-6: Dn + EA → EA (size = opmode - 4)
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	opmode := (opcode >> 6) & 7

	if srcMode == 0 && opmode <= 2 {
		// ADD.x Ds,Dd — source is data register
		size := int(opmode) // 0=byte, 1=word, 2=long
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

		// For sub-long sizes: operate on 32-bit values, then mask
		// ADD dst, src (32-bit)
		amd64ALU_reg_reg32(cb, 0x01, dst, src) // ADD dst32, src32

		// Extract CCR from native flags BEFORE masking
		emitCCR_Arithmetic(cb)

		// Apply size mask
		switch size {
		case M68K_SIZE_BYTE:
			// Mask to byte but keep full 32-bit register (only low byte meaningful)
		case M68K_SIZE_WORD:
			// Same — keep full value
		}

		// Store back
		m68kStoreDataReg(cb, dstReg, dst)
	}
}

// m68kEmitSUB_Dn_Dn emits SUB.L Ds,Dd (register-to-register).
func m68kEmitSUB_Dn_Dn(cb *CodeBuffer, opcode uint16) {
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	opmode := (opcode >> 6) & 7

	if srcMode == 0 && opmode <= 2 {
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

		amd64ALU_reg_reg32(cb, 0x29, dst, src) // SUB dst32, src32
		emitCCR_Arithmetic(cb)
		m68kStoreDataReg(cb, dstReg, dst)
	}
}

// m68kEmitCMP_Dn_Dn emits CMP.L Ds,Dd (compare, sets flags only).
// CMP sets NZVC but does NOT modify X.
func m68kEmitCMP_Dn_Dn(cb *CodeBuffer, opcode uint16) {
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7

	if srcMode == 0 {
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

		amd64ALU_reg_reg32(cb, 0x39, dst, src) // CMP dst32, src32

		// CMP sets NZVC but NOT X. Save old X, then use arithmetic CCR, then restore X.
		// Extract all flags first (before any clobbering)
		amd64SETcc(cb, amd64CondB, amd64RCX) // CL = C
		amd64SETcc(cb, amd64CondO, amd64RDX) // DL = V
		amd64SETcc(cb, amd64CondE, amd64R10) // R10B = Z
		amd64SETcc(cb, 0x8, amd64R11)        // R11B = N

		// Preserve old X (bit 4)
		amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X) // R14 &= 0x10 (keep only X)

		// Build NZVC into R14
		amd64MOVZX_B(cb, amd64RAX, amd64RCX) // C
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

		amd64MOVZX_B(cb, amd64RAX, amd64RDX) // V
		amd64SHL_imm(cb, amd64RAX, 1)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

		amd64MOVZX_B(cb, amd64RAX, amd64R10) // Z
		amd64SHL_imm(cb, amd64RAX, 2)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

		amd64MOVZX_B(cb, amd64RAX, amd64R11) // N
		amd64SHL_imm(cb, amd64RAX, 3)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
		// X unchanged — already preserved above
	}
}

// m68kEmitAND_Dn_Dn emits AND.L Ds,Dd.
func m68kEmitAND_Dn_Dn(cb *CodeBuffer, opcode uint16) {
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7

	if srcMode == 0 {
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

		amd64ALU_reg_reg32(cb, 0x21, dst, src) // AND dst32, src32
		// AND clears CF and OF on x86, sets SF and ZF — matches M68K exactly
		emitCCR_Logic(cb)
		m68kStoreDataReg(cb, dstReg, dst)
	}
}

// m68kEmitOR_Dn_Dn emits OR.L Ds,Dd.
func m68kEmitOR_Dn_Dn(cb *CodeBuffer, opcode uint16) {
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7

	if srcMode == 0 {
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

		amd64ALU_reg_reg32(cb, 0x09, dst, src) // OR dst32, src32
		emitCCR_Logic(cb)
		m68kStoreDataReg(cb, dstReg, dst)
	}
}

// m68kEmitEOR_Dn_Dn emits EOR.L Dn,<ea> (Group B, opmode 4-6).
func m68kEmitEOR_Dn_Dn(cb *CodeBuffer, opcode uint16) {
	srcReg := (opcode >> 9) & 7 // EOR: Dn is in bits 11-9
	dstMode := (opcode >> 3) & 7
	dstReg := opcode & 7

	if dstMode == 0 {
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

		amd64ALU_reg_reg32(cb, 0x31, dst, src) // XOR dst32, src32
		emitCCR_Logic(cb)
		m68kStoreDataReg(cb, dstReg, dst)
	}
}

// m68kEmitNOT emits NOT.L Dn.
func m68kEmitNOT(cb *CodeBuffer, opcode uint16) {
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if mode == 0 { // Data register direct
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		// NOT in x86 doesn't affect flags, so we need TEST after
		emitREX(cb, false, 0, r)
		cb.EmitBytes(0xF7, modRM(3, 2, r)) // NOT r32
		amd64TEST_reg_reg32(cb, r, r)
		emitCCR_Logic(cb)
		m68kStoreDataReg(cb, reg, r)
	}
}

// m68kEmitNEG emits NEG.L Dn.
func m68kEmitNEG(cb *CodeBuffer, opcode uint16) {
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if mode == 0 {
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		amd64NEG32(cb, r) // NEG r32 — sets all flags
		emitCCR_Arithmetic(cb)
		m68kStoreDataReg(cb, reg, r)
	}
}

// m68kEmitCLR emits CLR.L Dn.
func m68kEmitCLR(cb *CodeBuffer, opcode uint16) {
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if mode == 0 {
		r, mapped := m68kDataRegToAMD64(reg)
		if mapped {
			amd64XOR_reg_reg32(cb, r, r)
		} else {
			amd64MOV_mem_imm32(cb, m68kAMD64RegDataBase, int32(reg)*4, 0)
		}
		// CLR sets: N=0, Z=1, V=0, C=0, X unchanged
		amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X) // keep X
		amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z) // set Z
	}
}

// m68kEmitTST emits TST.L Dn (test, sets flags only).
func m68kEmitTST(cb *CodeBuffer, opcode uint16) {
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if mode == 0 {
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		amd64TEST_reg_reg32(cb, r, r)
		emitCCR_Logic(cb)
	}
}

// m68kEmitSWAP emits SWAP Dn (swap upper and lower words).
func m68kEmitSWAP(cb *CodeBuffer, opcode uint16) {
	reg := opcode & 7
	r := m68kResolveDataReg(cb, reg, amd64RAX)

	// ROL r32, 16 — rotates upper and lower words
	emitREX(cb, false, 0, r)
	cb.EmitBytes(0xC1, modRM(3, 0, r), 16) // ROL r32, 16

	// Flags: N,Z from result, V=0, C=0, X unchanged
	amd64TEST_reg_reg32(cb, r, r)
	emitCCR_Logic(cb)
	m68kStoreDataReg(cb, reg, r)
}

// m68kEmitEXT emits EXT.W / EXT.L / EXTB.L Dn (sign-extend).
func m68kEmitEXT(cb *CodeBuffer, opcode uint16) {
	reg := opcode & 7
	opmode := (opcode >> 6) & 7

	r := m68kResolveDataReg(cb, reg, amd64RAX)

	switch opmode {
	case 2: // EXT.W — sign-extend byte to word
		// MOVSX r16, r8 then mask to 16-bit
		// Actually: MOVSX EAX, AL then MOVZX to word
		if r != amd64RAX {
			amd64MOV_reg_reg32(cb, amd64RAX, r)
		}
		// CBW equivalent: MOVSX EAX, AL
		cb.EmitBytes(0x0F, 0xBE, modRM(3, amd64RAX, amd64RAX)) // MOVSX EAX, AL
		amd64MOVZX_W(cb, amd64RAX, amd64RAX)                   // zero-extend to 32 keeping sign-extended word
		// Write back only the low word
		amd64MOV_reg_reg32(cb, amd64RDX, r)               // save original
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, -65536) // mask upper word (0xFFFF0000)
		amd64ALU_reg_reg32(cb, 0x09, amd64RDX, amd64RAX)  // OR in sign-extended word
		m68kStoreDataReg(cb, reg, amd64RDX)
		r = amd64RAX // for flag test

	case 3: // EXT.L — sign-extend word to long
		if r != amd64RAX {
			amd64MOV_reg_reg32(cb, amd64RAX, r)
		}
		// MOVSX EAX, AX
		cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RAX, amd64RAX)) // MOVSX EAX, AX
		m68kStoreDataReg(cb, reg, amd64RAX)
		r = amd64RAX

	case 7: // EXTB.L — sign-extend byte to long
		if r != amd64RAX {
			amd64MOV_reg_reg32(cb, amd64RAX, r)
		}
		cb.EmitBytes(0x0F, 0xBE, modRM(3, amd64RAX, amd64RAX)) // MOVSX EAX, AL
		m68kStoreDataReg(cb, reg, amd64RAX)
		r = amd64RAX
	}

	// Flags: N,Z from result, V=0, C=0, X unchanged
	amd64TEST_reg_reg32(cb, r, r)
	emitCCR_Logic(cb)
}

// ===========================================================================
// Memory Access Helpers — Big-Endian Read/Write with I/O Bail
// ===========================================================================

// m68kStepSize returns the address increment for post-increment/pre-decrement.
func m68kStepSize(size int, reg uint16) uint32 {
	switch size {
	case M68K_SIZE_BYTE:
		if reg == 7 {
			return 2 // A7 always steps by 2 for alignment
		}
		return 1
	case M68K_SIZE_WORD:
		return 2
	case M68K_SIZE_LONG:
		return 4
	}
	return 1
}

// m68kEmitMemRead emits code to read from memory at address in addrReg.
// Result goes into dstReg. Size determines byte-swap and width.
// On I/O bail: sets NeedIOFallback and jumps to bailLabel.
// Returns the offset of the bail Jcc for patching by the caller.
func m68kEmitMemRead(cb *CodeBuffer, addrReg, dstReg byte, size int, bailLabel *int) {
	// Alignment check for word/long
	if size == M68K_SIZE_WORD || size == M68K_SIZE_LONG {
		amd64TEST_reg_imm8(cb, addrReg, 1)
		*bailLabel = amd64Jcc_rel32(cb, amd64CondNE) // JNZ bail
	}

	// I/O threshold check
	amd64ALU_reg_reg32(cb, 0x39, addrReg, m68kAMD64RegIOThresh) // CMP addr, IOThresh
	ioBailOff := amd64Jcc_rel32(cb, amd64CondAE)                // JAE bail

	switch size {
	case M68K_SIZE_BYTE:
		// MOVZX dst, BYTE [RSI + addr*1] — two-byte opcode 0F B6 with SIB
		emitREX_SIB(cb, false, dstReg, addrReg, m68kAMD64RegMemBase)
		cb.EmitBytes(0x0F, 0xB6, modRM(0, dstReg, 4), sibByte(0, addrReg, m68kAMD64RegMemBase))
	case M68K_SIZE_WORD:
		// MOV dx, [RSI + addr]; byte-swap
		emitMemOpSIB(cb, false, 0x8B, dstReg, m68kAMD64RegMemBase, addrReg, 0)
		// Byte-swap 16-bit: ROL dx, 8
		if isExtReg(dstReg) {
			cb.EmitBytes(0x66, rexByte(false, false, false, true))
		} else {
			cb.EmitBytes(0x66)
		}
		cb.EmitBytes(0xC1, modRM(3, 0, dstReg), 8) // ROL r16, 8
		amd64MOVZX_W(cb, dstReg, dstReg)           // zero-extend to 32-bit
	case M68K_SIZE_LONG:
		// MOV dst, [RSI + addr]; BSWAP
		emitMemOpSIB(cb, false, 0x8B, dstReg, m68kAMD64RegMemBase, addrReg, 0)
		// BSWAP
		emitREX(cb, false, 0, dstReg)
		cb.EmitBytes(0x0F, 0xC8+regBits(dstReg))
	}

	// Join path for successful read
	doneOff := amd64JMP_rel32(cb)

	// I/O bail (and alignment bail shares this)
	if size == M68K_SIZE_WORD || size == M68K_SIZE_LONG {
		patchRel32(cb, *bailLabel, cb.Len())
	}
	patchRel32(cb, ioBailOff, cb.Len())
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	// Caller will handle setting RetPC and emitting epilogue after this

	patchRel32(cb, doneOff, cb.Len())
	_ = doneOff // suppress unused
}

// m68kEmitMemWrite emits code to write a value to memory at address.
// addrReg has the address, valReg has the value to write.
func m68kEmitMemWrite(cb *CodeBuffer, addrReg, valReg byte, size int, bailLabel *int) {
	// Alignment check
	if size == M68K_SIZE_WORD || size == M68K_SIZE_LONG {
		amd64TEST_reg_imm8(cb, addrReg, 1)
		*bailLabel = amd64Jcc_rel32(cb, amd64CondNE)
	}

	// I/O threshold check
	amd64ALU_reg_reg32(cb, 0x39, addrReg, m68kAMD64RegIOThresh)
	ioBailOff := amd64Jcc_rel32(cb, amd64CondAE)

	switch size {
	case M68K_SIZE_BYTE:
		// MOV [RSI + addr], val8
		emitMemOpSIB(cb, false, 0x88, valReg, m68kAMD64RegMemBase, addrReg, 0)
	case M68K_SIZE_WORD:
		// Byte-swap val into scratch, then store
		amd64MOV_reg_reg32(cb, amd64R11, valReg)
		// ROL r16, 8
		if isExtReg(amd64R11) {
			cb.EmitBytes(0x66, rexByte(false, false, false, true))
		} else {
			cb.EmitBytes(0x66)
		}
		cb.EmitBytes(0xC1, modRM(3, 0, amd64R11), 8)
		// MOV [RSI + addr], r16 (16-bit store)
		cb.EmitBytes(0x66) // operand-size prefix
		emitMemOpSIB(cb, false, 0x89, amd64R11, m68kAMD64RegMemBase, addrReg, 0)
	case M68K_SIZE_LONG:
		// BSWAP val into scratch, then store
		amd64MOV_reg_reg32(cb, amd64R11, valReg)
		emitREX(cb, false, 0, amd64R11)
		cb.EmitBytes(0x0F, 0xC8+regBits(amd64R11)) // BSWAP
		emitMemOpSIB(cb, false, 0x89, amd64R11, m68kAMD64RegMemBase, addrReg, 0)
	}

	// Self-modifying code detection: check code page bitmap
	// bitmap[addr>>12] != 0 → set NeedInval=1
	amd64MOV_reg_reg32(cb, amd64R11, addrReg)
	amd64SHR_imm(cb, amd64R11, 12) // R11 = page index
	// Load bitmap pointer from context
	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffCodePageBitmapPtr))
	// MOVZX R11, BYTE [RCX + R11] (check bitmap)
	emitREX_SIB(cb, false, amd64R11, amd64R11, amd64RCX)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, amd64R11, 4), sibByte(0, amd64R11, amd64RCX))
	amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
	skipInvalOff := amd64Jcc_rel32(cb, amd64CondE) // JZ skip (no code on this page)
	// Code page hit: set NeedInval
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedInval), 1)
	patchRel32(cb, skipInvalOff, cb.Len())

	doneOff := amd64JMP_rel32(cb)

	if size == M68K_SIZE_WORD || size == M68K_SIZE_LONG {
		patchRel32(cb, *bailLabel, cb.Len())
	}
	patchRel32(cb, ioBailOff, cb.Len())
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)

	patchRel32(cb, doneOff, cb.Len())
}

// ===========================================================================
// Effective Address Computation
// ===========================================================================

// m68kEmitReadSourceEA emits code to read a source EA value into dstReg.
// memory and extPC point to where extension words start (instrPC + 2 for simple instructions).
// Returns the number of extension bytes consumed.
// On I/O bail, sets NeedIOFallback=1 (caller handles RetPC + epilogue).
func m68kEmitReadSourceEA(cb *CodeBuffer, mode, reg uint16, size int,
	memory []byte, extPC uint32, instrPC uint32, dstReg byte) int {

	switch mode {
	case 0: // Dn
		r := m68kResolveDataReg(cb, reg, dstReg)
		if r != dstReg {
			amd64MOV_reg_reg32(cb, dstReg, r)
		}
		// Apply size masking
		switch size {
		case M68K_SIZE_BYTE:
			amd64MOVZX_B(cb, dstReg, dstReg)
		case M68K_SIZE_WORD:
			amd64MOVZX_W(cb, dstReg, dstReg)
		}
		return 0

	case 1: // An
		r := m68kResolveAddrReg(cb, reg, dstReg)
		if r != dstReg {
			amd64MOV_reg_reg32(cb, dstReg, r)
		}
		return 0

	case 2: // (An) — indirect
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R10, r) // address in R10
		bail := 0
		m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
		return 0

	case 3: // (An)+ — post-increment
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R10, r) // save address before increment
		bail := 0
		m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
		// Post-increment: An += step
		step := m68kStepSize(size, reg)
		ar := m68kResolveAddrReg(cb, reg, amd64RCX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(step)) // ADD An, step
		m68kStoreAddrReg(cb, reg, ar)
		return 0

	case 4: // -(An) — pre-decrement
		step := m68kStepSize(size, reg)
		ar := m68kResolveAddrReg(cb, reg, amd64R10)
		amd64ALU_reg_imm32_32bit(cb, 5, ar, int32(step)) // SUB An, step
		m68kStoreAddrReg(cb, reg, ar)
		amd64MOV_reg_reg32(cb, amd64R10, ar) // address in R10
		bail := 0
		m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
		return 0

	case 5: // (d16,An)
		if extPC+2 > uint32(len(memory)) {
			return 2
		}
		disp := int16(uint16(memory[extPC])<<8 | uint16(memory[extPC+1]))
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R10, r)
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(disp)) // ADD R10, disp16
		bail := 0
		m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
		return 2

	case 6: // (d8,An,Xn) brief format
		if extPC+2 > uint32(len(memory)) {
			return 2
		}
		extWord := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
		if extWord&0x0100 != 0 {
			// Full 68020 format — bail to interpreter
			return m68kIndexedExtBytes(memory, extPC)
		}
		// Brief format: base + d8 + Xn.size * scale
		disp8 := int8(extWord & 0xFF)
		idxReg := (extWord >> 12) & 0x0F
		idxIsAddr := (extWord >> 15) & 1
		idxSize := (extWord >> 11) & 1 // 0=word, 1=long
		scale := (extWord >> 9) & 3    // 0-3

		// Base address: An + d8
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R10, r)
		if disp8 != 0 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(disp8))
		}

		// Index register
		if idxIsAddr == 1 {
			m68kResolveAddrReg(cb, idxReg&7, amd64RCX)
			amd64MOV_reg_mem32(cb, amd64RCX, m68kAMD64RegAddrBase, int32(idxReg&7)*4)
		} else {
			amd64MOV_reg_mem32(cb, amd64RCX, m68kAMD64RegDataBase, int32(idxReg&7)*4)
		}

		// Sign-extend if word index
		if idxSize == 0 {
			// MOVSX ECX, CX
			cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RCX, amd64RCX))
		}

		// Apply scale
		if scale > 0 {
			amd64SHL_imm(cb, amd64RCX, byte(scale))
		}

		// Add index to base
		amd64ALU_reg_reg32(cb, 0x01, amd64R10, amd64RCX) // ADD R10, RCX

		bail := 0
		m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
		return 2

	case 7: // Special modes
		switch reg {
		case 0: // (xxx).W — absolute short
			if extPC+2 > uint32(len(memory)) {
				return 2
			}
			w := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
			addr := uint32(int32(int16(w))) // sign-extend
			amd64MOV_reg_imm32(cb, amd64R10, addr)
			bail := 0
			m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
			return 2

		case 1: // (xxx).L — absolute long
			if extPC+4 > uint32(len(memory)) {
				return 4
			}
			addr := uint32(memory[extPC])<<24 | uint32(memory[extPC+1])<<16 |
				uint32(memory[extPC+2])<<8 | uint32(memory[extPC+3])
			amd64MOV_reg_imm32(cb, amd64R10, addr)
			bail := 0
			m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
			return 4

		case 2: // (d16,PC)
			if extPC+2 > uint32(len(memory)) {
				return 2
			}
			disp := int16(uint16(memory[extPC])<<8 | uint16(memory[extPC+1]))
			// PC for displacement is instrPC + 2 (after opcode), which is extPC for the first EA
			// Actually, for PC-relative, the base PC is the address of the extension word
			pcBase := extPC
			addr := uint32(int64(pcBase) + int64(disp))
			amd64MOV_reg_imm32(cb, amd64R10, addr)
			bail := 0
			m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
			return 2

		case 3: // (d8,PC,Xn) brief format
			if extPC+2 > uint32(len(memory)) {
				return 2
			}
			extWord := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
			if extWord&0x0100 != 0 {
				return m68kIndexedExtBytes(memory, extPC)
			}
			disp8 := int8(extWord & 0xFF)
			idxReg := (extWord >> 12) & 0x0F
			idxIsAddr := (extWord >> 15) & 1
			idxSize := (extWord >> 11) & 1
			scale := (extWord >> 9) & 3

			pcBase := extPC
			addr := uint32(int64(pcBase) + int64(disp8))
			amd64MOV_reg_imm32(cb, amd64R10, addr)

			if idxIsAddr == 1 {
				amd64MOV_reg_mem32(cb, amd64RCX, m68kAMD64RegAddrBase, int32(idxReg&7)*4)
			} else {
				amd64MOV_reg_mem32(cb, amd64RCX, m68kAMD64RegDataBase, int32(idxReg&7)*4)
			}
			if idxSize == 0 {
				cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RCX, amd64RCX))
			}
			if scale > 0 {
				amd64SHL_imm(cb, amd64RCX, byte(scale))
			}
			amd64ALU_reg_reg32(cb, 0x01, amd64R10, amd64RCX)

			bail := 0
			m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
			return 2

		case 4: // #imm
			immBytes := m68kImmediateBytes(size)
			if extPC+uint32(immBytes) > uint32(len(memory)) {
				return immBytes
			}
			switch size {
			case M68K_SIZE_BYTE:
				val := memory[extPC+1] // byte immediate is in low byte of word
				amd64MOV_reg_imm32(cb, dstReg, uint32(val))
			case M68K_SIZE_WORD:
				val := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
				amd64MOV_reg_imm32(cb, dstReg, uint32(val))
			case M68K_SIZE_LONG:
				val := uint32(memory[extPC])<<24 | uint32(memory[extPC+1])<<16 |
					uint32(memory[extPC+2])<<8 | uint32(memory[extPC+3])
				amd64MOV_reg_imm32(cb, dstReg, val)
			}
			return immBytes
		}
	}
	return 0
}

// m68kEmitWriteDestEA emits code to write valReg to a destination EA.
// For MOVE destination encoding: mode=bits[8:6], reg=bits[11:9] (reversed!).
func m68kEmitWriteDestEA(cb *CodeBuffer, mode, reg uint16, size int,
	memory []byte, extPC uint32, valReg byte) int {

	switch mode {
	case 0: // Dn
		switch size {
		case M68K_SIZE_BYTE:
			m68kStoreDataRegByte(cb, reg, valReg)
		case M68K_SIZE_WORD:
			// Store low word, preserve upper word
			r, mapped := m68kDataRegToAMD64(reg)
			if mapped {
				amd64MOVZX_W(cb, amd64R11, valReg)
				amd64ALU_reg_imm32_32bit(cb, 4, r, -65536) // AND r, 0xFFFF0000
				amd64ALU_reg_reg32(cb, 0x09, r, amd64R11)  // OR r, val16
			} else {
				amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegDataBase, int32(reg)*4)
				amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, -65536)
				amd64MOVZX_W(cb, amd64RCX, valReg)
				amd64ALU_reg_reg32(cb, 0x09, amd64R11, amd64RCX)
				amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, int32(reg)*4, amd64R11)
			}
		case M68K_SIZE_LONG:
			m68kStoreDataReg(cb, reg, valReg)
		}
		return 0

	case 1: // An (MOVEA)
		m68kStoreAddrReg(cb, reg, valReg)
		return 0

	case 2: // (An)
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R10, r)
		bail := 0
		m68kEmitMemWrite(cb, amd64R10, valReg, size, &bail)
		return 0

	case 3: // (An)+
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R10, r)
		bail := 0
		m68kEmitMemWrite(cb, amd64R10, valReg, size, &bail)
		step := m68kStepSize(size, reg)
		ar := m68kResolveAddrReg(cb, reg, amd64RCX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(step))
		m68kStoreAddrReg(cb, reg, ar)
		return 0

	case 4: // -(An)
		step := m68kStepSize(size, reg)
		ar := m68kResolveAddrReg(cb, reg, amd64R10)
		amd64ALU_reg_imm32_32bit(cb, 5, ar, int32(step))
		m68kStoreAddrReg(cb, reg, ar)
		amd64MOV_reg_reg32(cb, amd64R10, ar)
		bail := 0
		m68kEmitMemWrite(cb, amd64R10, valReg, size, &bail)
		return 0

	case 5: // (d16,An)
		if extPC+2 > uint32(len(memory)) {
			return 2
		}
		disp := int16(uint16(memory[extPC])<<8 | uint16(memory[extPC+1]))
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R10, r)
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(disp))
		bail := 0
		m68kEmitMemWrite(cb, amd64R10, valReg, size, &bail)
		return 2
	}
	return 0
}

// ===========================================================================
// MOVE with Memory Operands
// ===========================================================================

// m68kEmitMOVE_Full emits MOVE.x <src_ea>,<dst_ea> (any addressing modes).
func m68kEmitMOVE_Full(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, size int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset

	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7 // MOVE dest: mode at [8:6]
	dstReg := (opcode >> 9) & 7  // MOVE dest: reg at [11:9]

	// Read source into RDX
	extPC := instrPC + 2
	srcExtBytes := m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, extPC, instrPC, amd64RDX)

	// Write to destination
	dstExtPC := extPC + uint32(srcExtBytes)
	m68kEmitWriteDestEA(cb, dstMode, dstReg, size, memory, dstExtPC, amd64RDX)

	// Flags: N,Z from result, V=0, C=0, X unchanged (unless MOVEA which doesn't set flags)
	if dstMode != 1 { // MOVEA doesn't affect flags
		amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
		emitCCR_Logic(cb)
	}
}

// ===========================================================================
// Control Flow Helpers — M68K Condition Evaluation
// ===========================================================================

// m68kEmitCondTest emits native code that evaluates an M68K condition code
// (0-15) by testing bits in the CCR register (R14). After this sequence,
// the x86-64 ZF flag indicates: ZF=0 means condition TRUE, ZF=1 means FALSE.
// (Uses TEST R14, mask → JNZ for "bit set" conditions.)
//
// Returns the x86-64 Jcc condition code to use for "branch if condition TRUE".
func m68kCondToJcc(cb *CodeBuffer, m68kCond uint16) byte {
	switch m68kCond {
	case 0: // T (always true) — use JMP
		return 0xFF // special: unconditional
	case 1: // F (always false) — never branch
		return 0xFE // special: never
	case 2: // HI: C=0 AND Z=0 → TEST R14, 5; JZ
		amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_C|m68kCCR_Z)
		return amd64CondE // JZ = both bits clear
	case 3: // LS: C=1 OR Z=1 → TEST R14, 5; JNZ
		amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_C|m68kCCR_Z)
		return amd64CondNE // JNZ = at least one bit set
	case 4: // CC: C=0 → TEST R14, 1; JZ
		amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_C)
		return amd64CondE
	case 5: // CS: C=1 → TEST R14, 1; JNZ
		amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_C)
		return amd64CondNE
	case 6: // NE: Z=0 → TEST R14, 4; JZ
		amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_Z)
		return amd64CondE
	case 7: // EQ: Z=1 → TEST R14, 4; JNZ
		amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_Z)
		return amd64CondNE
	case 8: // VC: V=0 → TEST R14, 2; JZ
		amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_V)
		return amd64CondE
	case 9: // VS: V=1 → TEST R14, 2; JNZ
		amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_V)
		return amd64CondNE
	case 10: // PL: N=0 → TEST R14, 8; JZ
		amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_N)
		return amd64CondE
	case 11: // MI: N=1 → TEST R14, 8; JNZ
		amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_N)
		return amd64CondNE
	case 12: // GE: N⊕V=0
		return m68kEmitNxorV(cb, false)
	case 13: // LT: N⊕V=1
		return m68kEmitNxorV(cb, true)
	case 14: // GT: Z=0 AND N⊕V=0
		return m68kEmitGT(cb)
	case 15: // LE: Z=1 OR N⊕V=1
		return m68kEmitLE(cb)
	}
	return 0xFE // never
}

// m68kEmitNxorV emits code to compute N⊕V and sets flags for branch.
// If wantSet=true, returns Jcc for "N⊕V=1" (LT). If false, "N⊕V=0" (GE).
func m68kEmitNxorV(cb *CodeBuffer, wantSet bool) byte {
	// Extract N (bit 3) and V (bit 1) from CCR
	amd64MOV_reg_reg32(cb, amd64RAX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RAX, 3) // RAX bit 0 = N
	amd64MOV_reg_reg32(cb, amd64RCX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RCX, 1)                    // RCX bit 0 = V
	amd64ALU_reg_reg32(cb, 0x31, amd64RAX, amd64RCX) // XOR EAX, ECX → bit 0 = N⊕V
	amd64TEST_reg_imm8(cb, amd64RAX, 1)
	if wantSet {
		return amd64CondNE // JNZ: N⊕V=1 → LT
	}
	return amd64CondE // JZ: N⊕V=0 → GE
}

// m68kEmitGT emits code for GT condition: Z=0 AND N⊕V=0.
func m68kEmitGT(cb *CodeBuffer) byte {
	// First check Z=0
	amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_Z)
	skipOff := amd64Jcc_rel32(cb, amd64CondNE) // if Z=1, skip (condition false)

	// Z=0, now check N⊕V=0
	amd64MOV_reg_reg32(cb, amd64RAX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RAX, 3)
	amd64MOV_reg_reg32(cb, amd64RCX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RCX, 1)
	amd64ALU_reg_reg32(cb, 0x31, amd64RAX, amd64RCX)
	amd64TEST_reg_imm8(cb, amd64RAX, 1)
	// If N⊕V=0 (JZ), condition is TRUE → we want to fall through to "taken"
	// We'll return JZ and let the caller handle it
	takenOff := amd64Jcc_rel32(cb, amd64CondE) // JZ → GT is true

	// Neither path: condition false
	patchRel32(cb, skipOff, cb.Len()) // Z=1 lands here
	// Set ZF=1 to indicate "false" (using XOR EAX, EAX which sets ZF)
	amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)
	amd64TEST_reg_imm8(cb, amd64RAX, 1) // ZF=1 → false
	doneOff := amd64JMP_rel32(cb)

	// Taken path
	patchRel32(cb, takenOff, cb.Len())
	amd64MOV_reg_imm32(cb, amd64RAX, 1)
	amd64TEST_reg_imm8(cb, amd64RAX, 1) // ZF=0 → true

	patchRel32(cb, doneOff, cb.Len())
	return amd64CondNE // JNZ: condition true
}

// m68kEmitLE emits code for LE condition: Z=1 OR N⊕V=1.
func m68kEmitLE(cb *CodeBuffer) byte {
	// Check Z=1 first (short-circuit OR)
	amd64TEST_reg_imm8(cb, m68kAMD64RegCCR, m68kCCR_Z)
	takenOff := amd64Jcc_rel32(cb, amd64CondNE) // if Z=1, condition true

	// Z=0, check N⊕V=1
	amd64MOV_reg_reg32(cb, amd64RAX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RAX, 3)
	amd64MOV_reg_reg32(cb, amd64RCX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RCX, 1)
	amd64ALU_reg_reg32(cb, 0x31, amd64RAX, amd64RCX)
	amd64TEST_reg_imm8(cb, amd64RAX, 1)          // N⊕V in bit 0
	taken2Off := amd64Jcc_rel32(cb, amd64CondNE) // JNZ → N⊕V=1 → LE true

	// Neither Z=1 nor N⊕V=1: false
	amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)
	amd64TEST_reg_imm8(cb, amd64RAX, 1) // ZF=1 → false
	doneOff := amd64JMP_rel32(cb)

	patchRel32(cb, takenOff, cb.Len())
	patchRel32(cb, taken2Off, cb.Len())
	amd64MOV_reg_imm32(cb, amd64RAX, 1)
	amd64TEST_reg_imm8(cb, amd64RAX, 1) // ZF=0 → true

	patchRel32(cb, doneOff, cb.Len())
	return amd64CondNE // JNZ: condition true
}

// invertM68KCond returns the opposite M68K condition code.
func invertM68KCond(cond uint16) uint16 {
	return cond ^ 1
}

// ===========================================================================
// Control Flow Emitters
// ===========================================================================

// m68kReadBranchDisp reads the branch displacement for a Bcc/BRA/BSR instruction.
// Returns the signed displacement (relative to instrPC + 2).
func m68kReadBranchDisp(memory []byte, instrPC uint32, opcode uint16) int32 {
	disp8 := opcode & 0xFF
	switch disp8 {
	case 0x00: // word displacement
		if instrPC+4 <= uint32(len(memory)) {
			w := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
			return int32(int16(w))
		}
		return 0
	case 0xFF: // long displacement (68020)
		if instrPC+6 <= uint32(len(memory)) {
			l := uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
				uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5])
			return int32(l)
		}
		return 0
	default: // byte displacement in opcode
		return int32(int8(disp8))
	}
}

// m68kEmitBRA emits BRA (unconditional branch, block terminator).
func m68kEmitBRA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset
	disp := m68kReadBranchDisp(memory, instrPC, ji.opcode)
	targetPC := uint32(int64(instrPC) + 2 + int64(disp))

	m68kEmitRetPC(cb, targetPC, uint32(instrIdx+1))
	m68kEmitEpilogue(cb, br)
}

// m68kEmitBSR emits BSR (branch to subroutine, block terminator).
// Pushes return address to stack, then branches.
func m68kEmitBSR(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset
	disp := m68kReadBranchDisp(memory, instrPC, ji.opcode)
	targetPC := uint32(int64(instrPC) + 2 + int64(disp))
	returnPC := startPC + ji.pcOffset + uint32(ji.length) // PC after BSR instruction

	// Push return address to stack: A7 -= 4; Write32_BE([memBase+A7], returnPC)
	// SUB R13d, 4  (R13 = A7)
	amd64ALU_reg_imm32_32bit(cb, 5, m68kAMD64RegA7, 4)

	// I/O check: if A7 >= IOThreshold, bail
	amd64ALU_reg_reg32(cb, 0x39, m68kAMD64RegA7, m68kAMD64RegIOThresh) // CMP A7, IOThresh
	bailOff := amd64Jcc_rel32(cb, amd64CondAE)                         // JAE bail

	// Write return address in big-endian: BSWAP + MOV [memBase+A7], val
	amd64MOV_reg_imm32(cb, amd64RAX, returnPC)
	// BSWAP EAX
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX))
	// MOV [RSI + R13], EAX (SIB addressing: base=RSI, index=R13, scale=1)
	emitMemOpSIB(cb, false, 0x89, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)

	m68kEmitRetPC(cb, targetPC, uint32(instrIdx+1))
	m68kEmitEpilogue(cb, br)

	// Bail path: restore A7 and return to interpreter
	patchRel32(cb, bailOff, cb.Len())
	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4) // ADD R13d, 4 (undo)
	// Set NeedIOFallback and return to dispatcher with this instruction's PC
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
}

// m68kEmitBcc emits Bcc (conditional branch, NOT a terminator).
// Pattern: if condition NOT met → skip over taken path (fall through).
//
//	if condition met → exit block with targetPC.
//
// m68kFindInstrByPC finds the instruction index with the given M68K pcOffset.
// Returns -1 if not found.
func m68kFindInstrByPC(instrs []M68KJITInstr, targetPCOffset uint32, maxIdx int) int {
	for k := 0; k < maxIdx; k++ {
		if instrs[k].pcOffset == targetPCOffset {
			return k
		}
	}
	return -1
}

func m68kEmitBcc(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32,
	br *m68kBlockRegs, instrIdx int, instrOffsets []int, blockLen int,
	instrs []M68KJITInstr) {

	instrPC := startPC + ji.pcOffset
	cond := (ji.opcode >> 8) & 0xF

	if memory == nil {
		return
	}

	disp := m68kReadBranchDisp(memory, instrPC, ji.opcode)
	targetPC := uint32(int64(instrPC) + 2 + int64(disp))

	// Evaluate M68K condition → returns Jcc condition code
	jccCond := m68kCondToJcc(cb, cond)

	if jccCond == 0xFF {
		m68kEmitRetPC(cb, targetPC, uint32(instrIdx+1))
		m68kEmitEpilogue(cb, br)
		return
	}
	if jccCond == 0xFE {
		return
	}

	// Within-block backward branch with budget (loop optimization)
	if br.hasBackwardBranch && targetPC >= startPC && targetPC < instrPC &&
		instrOffsets != nil && instrs != nil {
		targetPCOffset := targetPC - startPC
		targetIdx := m68kFindInstrByPC(instrs, targetPCOffset, instrIdx)

		if targetIdx >= 0 && targetIdx < len(instrOffsets) {
			// Pattern: if NOT taken → skip; if taken → budget check → JMP back
			invertedCond := jccCond ^ 1
			skipOff := amd64Jcc_rel32(cb, invertedCond) // skip if NOT taken

			// Increment loop counter by body size (instructions in loop body)
			bodySize := uint32(instrIdx - targetIdx + 1)
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(bodySize)) // ADD counter, bodySize
			amd64MOV_mem_reg32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), amd64RAX)

			// Budget check: if counter >= budget → exit
			amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, int32(m68kJitBudget)) // CMP counter, budget
			budgetExitOff := amd64Jcc_rel32(cb, amd64CondAE)                // JAE budget_exit

			// Budget OK → JMP back to target native code
			targetNativeOffset := instrOffsets[targetIdx]
			backOff := amd64JMP_rel32(cb)
			patchRel32(cb, backOff, targetNativeOffset)

			// Budget exceeded → exit block with target PC
			patchRel32(cb, budgetExitOff, cb.Len())
			// Undo the last bodySize addition (we didn't execute it)
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, int32(bodySize)) // SUB counter, bodySize
			amd64MOV_mem_reg32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), amd64RAX)
			m68kEmitRetPC(cb, targetPC, uint32(instrIdx+1))
			m68kEmitEpilogue(cb, br)

			// Not-taken path
			patchRel32(cb, skipOff, cb.Len())
			return
		}
	}

	// Exit-block path (forward branch or can't resolve target)
	invertedCond := jccCond ^ 1
	skipOff := amd64Jcc_rel32(cb, invertedCond)

	m68kEmitRetPC(cb, targetPC, uint32(instrIdx+1))
	m68kEmitEpilogue(cb, br)

	patchRel32(cb, skipOff, cb.Len())
}

// m68kEmitRTS emits RTS (return from subroutine, block terminator).
func m68kEmitRTS(cb *CodeBuffer, br *m68kBlockRegs, instrIdx int) {
	// Read return address from stack: Read32_BE([memBase + A7]); A7 += 4

	// I/O check: if A7 >= IOThreshold, bail
	amd64ALU_reg_reg32(cb, 0x39, m68kAMD64RegA7, m68kAMD64RegIOThresh)
	bailOff := amd64Jcc_rel32(cb, amd64CondAE)

	// Read32_BE: MOV EAX, [RSI + R13]; BSWAP EAX
	emitMemOpSIB(cb, false, 0x8B, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX)) // BSWAP EAX

	// A7 += 4
	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4)

	// RetPC = popped value (EAX), RetCount = instrIdx + 1
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), amd64RAX)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
	m68kEmitEpilogue(cb, br)

	// Bail path: set NeedIOFallback, bail with RetCount=instrIdx (not +1, so dispatcher re-executes RTS)
	patchRel32(cb, bailOff, cb.Len())
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
}

// m68kEmitJSR emits JSR <ea> (jump to subroutine, block terminator).
// Currently handles JSR (An) and JSR (abs).
func m68kEmitJSR(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset
	returnPC := instrPC + uint32(ji.length) // PC after JSR
	mode := (ji.opcode >> 3) & 7
	reg := ji.opcode & 7

	// Compute target address into RAX
	switch mode {
	case 2: // (An)
		amd64MOV_reg_reg32(cb, amd64RAX, m68kResolveAddrReg(cb, reg, amd64RDX))
	case 7:
		switch reg {
		case 1: // abs.L
			if instrPC+6 <= uint32(len(memory)) {
				addr := uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
					uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5])
				amd64MOV_reg_imm32(cb, amd64RAX, addr)
			}
		case 0: // abs.W
			if instrPC+4 <= uint32(len(memory)) {
				w := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
				addr := uint32(int32(int16(w))) // sign-extend
				amd64MOV_reg_imm32(cb, amd64RAX, addr)
			}
		default:
			// Unsupported addressing mode — bail
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
	default:
		// Unsupported — bail
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return
	}

	// Save target in R10 (scratch)
	amd64MOV_reg_reg32(cb, amd64R10, amd64RAX)

	// Push return address: A7 -= 4; Write32_BE
	amd64ALU_reg_imm32_32bit(cb, 5, m68kAMD64RegA7, 4) // SUB A7, 4

	// I/O check
	amd64ALU_reg_reg32(cb, 0x39, m68kAMD64RegA7, m68kAMD64RegIOThresh)
	bailOff := amd64Jcc_rel32(cb, amd64CondAE)

	// Write return addr in big-endian
	amd64MOV_reg_imm32(cb, amd64RAX, returnPC)
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX)) // BSWAP EAX
	emitMemOpSIB(cb, false, 0x89, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)

	// RetPC = target (R10), RetCount
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), amd64R10)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
	m68kEmitEpilogue(cb, br)

	// Bail path
	patchRel32(cb, bailOff, cb.Len())
	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4) // undo SUB
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
}

// m68kEmitJMP emits JMP <ea> (unconditional jump, block terminator).
func m68kEmitJMP(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset
	mode := (ji.opcode >> 3) & 7
	reg := ji.opcode & 7

	switch mode {
	case 2: // (An)
		r := m68kResolveAddrReg(cb, reg, amd64RAX)
		amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), r)
	case 7:
		switch reg {
		case 1: // abs.L
			if instrPC+6 <= uint32(len(memory)) {
				addr := uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
					uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5])
				amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), addr)
			}
		case 0: // abs.W
			if instrPC+4 <= uint32(len(memory)) {
				w := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
				addr := uint32(int32(int16(w)))
				amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), addr)
			}
		default:
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
	default:
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return
	}

	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
	m68kEmitEpilogue(cb, br)
}

// m68kEmitScc emits Scc Dn (set byte to 0xFF or 0x00 based on condition).
func m68kEmitScc(cb *CodeBuffer, opcode uint16) {
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	cond := (opcode >> 8) & 0xF

	if mode != 0 {
		return // only Dn for now
	}

	jccCond := m68kCondToJcc(cb, cond)

	if jccCond == 0xFF { // always true
		r, mapped := m68kDataRegToAMD64(reg)
		if mapped {
			amd64ALU_reg_imm32_32bit(cb, 1, r, -1) // OR r, -1 → sets low byte to 0xFF... no
			// Actually set low byte: MOV r8, 0xFF
			// Simpler: OR r, 0xFF for byte
		}
		amd64MOV_reg_imm32(cb, amd64RAX, 0xFF)
		m68kStoreDataRegByte(cb, reg, amd64RAX)
		return
	}
	if jccCond == 0xFE { // always false
		amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)
		m68kStoreDataRegByte(cb, reg, amd64RAX)
		return
	}

	// Condition was already evaluated by m68kCondToJcc
	// Use SETcc to get 0 or 1, then NEG to get 0 or 0xFF
	amd64SETcc(cb, jccCond, amd64RAX) // AL = 0 or 1
	amd64NEG32(cb, amd64RAX)          // -1 → 0xFF or 0 → 0
	m68kStoreDataRegByte(cb, reg, amd64RAX)
}

// m68kStoreDataRegByte stores the low byte of src to the low byte of a data register.
func m68kStoreDataRegByte(cb *CodeBuffer, dreg uint16, src byte) {
	r, mapped := m68kDataRegToAMD64(dreg)
	if mapped {
		// AND out old low byte, OR in new
		amd64MOVZX_B(cb, src, src)               // zero-extend to get clean byte
		amd64ALU_reg_imm32_32bit(cb, 4, r, -256) // AND r, 0xFFFFFF00
		amd64ALU_reg_reg32(cb, 0x09, r, src)     // OR r, src (low byte)
		return
	}
	// Spilled: load, modify byte, store back
	amd64MOV_reg_mem32(cb, amd64RDX, m68kAMD64RegDataBase, int32(dreg)*4)
	amd64MOVZX_B(cb, src, src)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, -256) // AND r, 0xFFFFFF00
	amd64ALU_reg_reg32(cb, 0x09, amd64RDX, src)
	amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, int32(dreg)*4, amd64RDX)
}

// ===========================================================================
// DBcc Emitter
// ===========================================================================

// m68kEmitDBcc emits DBcc Dn,displacement.
// Semantics: if condition TRUE → fall through (exit loop).
//
//	if condition FALSE → Dn.W--; if Dn.W != -1 → branch to target; else fall through.
func m68kEmitDBcc(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32,
	br *m68kBlockRegs, instrIdx int, instrOffsets []int, instrs []M68KJITInstr) {

	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	cond := (opcode >> 8) & 0xF
	reg := opcode & 7

	if memory == nil || instrPC+4 > uint32(len(memory)) {
		return
	}

	// Read word displacement (relative to instrPC + 2)
	dispWord := int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3]))
	targetPC := uint32(int64(instrPC) + 2 + int64(dispWord))

	// Step 1: Evaluate condition
	if cond == 0 { // DBT — always true → always fall through (never loops)
		return // fall through to next instruction
	}
	if cond == 1 { // DBF / DBRA — always false → always decrement and branch
		// Skip condition check, go straight to decrement
	} else {
		// Evaluate condition: if TRUE → skip to fall-through (exit loop)
		jccCond := m68kCondToJcc(cb, cond)
		if jccCond != 0xFF && jccCond != 0xFE {
			exitOff := amd64Jcc_rel32(cb, jccCond) // if condition TRUE → skip
			defer func(off int) {
				patchRel32(cb, off, cb.Len())
			}(exitOff)
		}
	}

	// Step 2: Decrement Dn.W (only low word, upper word preserved)
	r := m68kResolveDataReg(cb, reg, amd64RAX)
	// Extract low word, decrement, store back
	amd64MOV_reg_reg32(cb, amd64RDX, r)          // save full value
	amd64MOVZX_W(cb, amd64RCX, r)                // ECX = low word
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1) // ECX -= 1 (SUB)
	// Merge decremented word back: (upper word) | (new low word)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, -65536) // AND EDX, 0xFFFF0000
	amd64MOVZX_W(cb, amd64RCX, amd64RCX)              // zero-extend new word
	amd64ALU_reg_reg32(cb, 0x09, amd64RDX, amd64RCX)  // OR EDX, ECX
	m68kStoreDataReg(cb, reg, amd64RDX)

	// Step 3: If Dn.W == -1 (0xFFFF) → fall through (loop exhausted)
	amd64ALU_reg_imm32_32bit(cb, 7, amd64RCX, 0xFFFF) // CMP ECX, 0xFFFF
	exhaustedOff := amd64Jcc_rel32(cb, amd64CondE)    // JE exhausted

	// Not exhausted → branch to target
	// Try within-block backward branch with budget
	if br.hasBackwardBranch && targetPC >= startPC && targetPC < instrPC &&
		instrOffsets != nil && instrs != nil {
		targetPCOffset := targetPC - startPC
		targetIdx := m68kFindInstrByPC(instrs, targetPCOffset, instrIdx)

		if targetIdx >= 0 && targetIdx < len(instrOffsets) {
			// Budget check
			bodySize := uint32(instrIdx - targetIdx + 1)
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), amd64RAX)
			amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, int32(m68kJitBudget))
			budgetExitOff := amd64Jcc_rel32(cb, amd64CondAE)

			// Budget OK → JMP back
			targetNativeOffset := instrOffsets[targetIdx]
			backOff := amd64JMP_rel32(cb)
			patchRel32(cb, backOff, targetNativeOffset)

			// Budget exceeded → exit
			patchRel32(cb, budgetExitOff, cb.Len())
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), amd64RAX)
			m68kEmitRetPC(cb, targetPC, uint32(instrIdx+1))
			m68kEmitEpilogue(cb, br)

			// Exhausted → fall through
			patchRel32(cb, exhaustedOff, cb.Len())
			return
		}
	}

	// Fallback: exit block with target PC
	m68kEmitRetPC(cb, targetPC, uint32(instrIdx+1))
	m68kEmitEpilogue(cb, br)

	// Exhausted → fall through
	patchRel32(cb, exhaustedOff, cb.Len())
}

// ===========================================================================
// ADD/SUB/CMP with Memory Operands
// ===========================================================================

// m68kEmitADD_EA_Dn emits ADD.x <ea>,Dn (EA source, register destination).
func m68kEmitADD_EA_Dn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	opmode := (opcode >> 6) & 7
	size := int(opmode) // 0=byte, 1=word, 2=long

	// Read source EA value into RDX
	m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, instrPC+2, instrPC, amd64RDX)

	// ADD dst, src
	dst := m68kResolveDataReg(cb, dstReg, amd64RAX)
	amd64ALU_reg_reg32(cb, 0x01, dst, amd64RDX) // ADD dst, EDX
	emitCCR_Arithmetic(cb)
	m68kStoreDataReg(cb, dstReg, dst)
}

// m68kEmitSUB_EA_Dn emits SUB.x <ea>,Dn.
func m68kEmitSUB_EA_Dn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	opmode := (opcode >> 6) & 7
	size := int(opmode)

	m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, instrPC+2, instrPC, amd64RDX)

	dst := m68kResolveDataReg(cb, dstReg, amd64RAX)
	amd64ALU_reg_reg32(cb, 0x29, dst, amd64RDX) // SUB dst, EDX
	emitCCR_Arithmetic(cb)
	m68kStoreDataReg(cb, dstReg, dst)
}

// m68kEmitCMP_EA_Dn emits CMP.x <ea>,Dn (compare, flags only, X unchanged).
func m68kEmitCMP_EA_Dn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	opmode := (opcode >> 6) & 7
	size := int(opmode)

	m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, instrPC+2, instrPC, amd64RDX)

	dst := m68kResolveDataReg(cb, dstReg, amd64RAX)
	amd64ALU_reg_reg32(cb, 0x39, dst, amd64RDX) // CMP dst, EDX

	// CMP sets NZVC but NOT X — preserve old X
	amd64SETcc(cb, amd64CondB, amd64RCX)
	amd64SETcc(cb, amd64CondO, amd64RDX)
	amd64SETcc(cb, amd64CondE, amd64R10)
	amd64SETcc(cb, 0x8, amd64R11)

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X) // keep X
	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 1)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64R10)
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
}

// ===========================================================================
// Stage 5: Extended Instructions — LEA, PEA, LINK, UNLK, ADDQ, SUBQ, Shifts, ADDA, SUBA
// ===========================================================================

// m68kEmitComputeEAAddr emits code to compute an effective address into dstReg.
// Unlike m68kEmitReadSourceEA, this does NOT read from memory — just computes the address.
// Used by LEA and PEA. Returns extension bytes consumed.
func m68kEmitComputeEAAddr(cb *CodeBuffer, mode, reg uint16,
	memory []byte, extPC uint32, instrPC uint32, dstReg byte) int {

	switch mode {
	case 2: // (An)
		r := m68kResolveAddrReg(cb, reg, dstReg)
		if r != dstReg {
			amd64MOV_reg_reg32(cb, dstReg, r)
		}
		return 0

	case 5: // (d16,An)
		if extPC+2 > uint32(len(memory)) {
			return 2
		}
		disp := int16(uint16(memory[extPC])<<8 | uint16(memory[extPC+1]))
		r := m68kResolveAddrReg(cb, reg, dstReg)
		if r != dstReg {
			amd64MOV_reg_reg32(cb, dstReg, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 0, dstReg, int32(disp))
		return 2

	case 6: // (d8,An,Xn) brief format
		if extPC+2 > uint32(len(memory)) {
			return 2
		}
		extWord := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
		if extWord&0x0100 != 0 {
			return m68kIndexedExtBytes(memory, extPC) // full format — bail
		}
		disp8 := int8(extWord & 0xFF)
		idxReg := (extWord >> 12) & 0x0F
		idxIsAddr := (extWord >> 15) & 1
		idxSize := (extWord >> 11) & 1
		scale := (extWord >> 9) & 3

		r := m68kResolveAddrReg(cb, reg, dstReg)
		if r != dstReg {
			amd64MOV_reg_reg32(cb, dstReg, r)
		}
		if disp8 != 0 {
			amd64ALU_reg_imm32_32bit(cb, 0, dstReg, int32(disp8))
		}
		if idxIsAddr == 1 {
			amd64MOV_reg_mem32(cb, amd64RCX, m68kAMD64RegAddrBase, int32(idxReg&7)*4)
		} else {
			amd64MOV_reg_mem32(cb, amd64RCX, m68kAMD64RegDataBase, int32(idxReg&7)*4)
		}
		if idxSize == 0 {
			cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RCX, amd64RCX)) // MOVSX ECX, CX
		}
		if scale > 0 {
			amd64SHL_imm(cb, amd64RCX, byte(scale))
		}
		amd64ALU_reg_reg32(cb, 0x01, dstReg, amd64RCX)
		return 2

	case 7:
		switch reg {
		case 0: // (xxx).W
			if extPC+2 > uint32(len(memory)) {
				return 2
			}
			w := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
			amd64MOV_reg_imm32(cb, dstReg, uint32(int32(int16(w))))
			return 2

		case 1: // (xxx).L
			if extPC+4 > uint32(len(memory)) {
				return 4
			}
			addr := uint32(memory[extPC])<<24 | uint32(memory[extPC+1])<<16 |
				uint32(memory[extPC+2])<<8 | uint32(memory[extPC+3])
			amd64MOV_reg_imm32(cb, dstReg, addr)
			return 4

		case 2: // (d16,PC)
			if extPC+2 > uint32(len(memory)) {
				return 2
			}
			disp := int16(uint16(memory[extPC])<<8 | uint16(memory[extPC+1]))
			addr := uint32(int64(extPC) + int64(disp))
			amd64MOV_reg_imm32(cb, dstReg, addr)
			return 2

		case 3: // (d8,PC,Xn) brief
			if extPC+2 > uint32(len(memory)) {
				return 2
			}
			extWord := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
			if extWord&0x0100 != 0 {
				return m68kIndexedExtBytes(memory, extPC)
			}
			disp8 := int8(extWord & 0xFF)
			idxReg := (extWord >> 12) & 0x0F
			idxIsAddr := (extWord >> 15) & 1
			idxSize := (extWord >> 11) & 1
			scale := (extWord >> 9) & 3

			addr := uint32(int64(extPC) + int64(disp8))
			amd64MOV_reg_imm32(cb, dstReg, addr)
			if idxIsAddr == 1 {
				amd64MOV_reg_mem32(cb, amd64RCX, m68kAMD64RegAddrBase, int32(idxReg&7)*4)
			} else {
				amd64MOV_reg_mem32(cb, amd64RCX, m68kAMD64RegDataBase, int32(idxReg&7)*4)
			}
			if idxSize == 0 {
				cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RCX, amd64RCX))
			}
			if scale > 0 {
				amd64SHL_imm(cb, amd64RCX, byte(scale))
			}
			amd64ALU_reg_reg32(cb, 0x01, dstReg, amd64RCX)
			return 2
		}
	}
	return 0
}

// m68kEmitLEA emits LEA <ea>,An (compute EA, store in address register, no flags).
func m68kEmitLEA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	dstAReg := (opcode >> 9) & 7
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7

	m68kEmitComputeEAAddr(cb, srcMode, srcReg, memory, instrPC+2, instrPC, amd64RAX)
	m68kStoreAddrReg(cb, dstAReg, amd64RAX)
}

// m68kEmitPEA emits PEA <ea> (compute EA, push to stack, no flags).
func m68kEmitPEA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7

	// Compute EA into RAX
	m68kEmitComputeEAAddr(cb, srcMode, srcReg, memory, instrPC+2, instrPC, amd64RAX)

	// Push to stack: A7 -= 4; Write32_BE([memBase + A7], EAX)
	amd64ALU_reg_imm32_32bit(cb, 5, m68kAMD64RegA7, 4)

	// I/O check
	amd64ALU_reg_reg32(cb, 0x39, m68kAMD64RegA7, m68kAMD64RegIOThresh)
	bailOff := amd64Jcc_rel32(cb, amd64CondAE)

	// BSWAP and store
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX)) // BSWAP EAX
	emitMemOpSIB(cb, false, 0x89, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)

	doneOff := amd64JMP_rel32(cb)

	// Bail
	patchRel32(cb, bailOff, cb.Len())
	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4) // undo SUB
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)

	patchRel32(cb, doneOff, cb.Len())
}

// m68kEmitLINK emits LINK An,#displacement.
func m68kEmitLINK(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset
	reg := ji.opcode & 7

	// Read displacement (word or long)
	var disp int32
	if ji.opcode&0xFFF8 == 0x4808 { // LINK.L
		if instrPC+6 <= uint32(len(memory)) {
			disp = int32(uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
				uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5]))
		}
	} else { // LINK.W
		if instrPC+4 <= uint32(len(memory)) {
			disp = int32(int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])))
		}
	}

	// 1. Push An: A7 -= 4; Write32_BE([A7], An)
	ar := m68kResolveAddrReg(cb, reg, amd64RAX)
	amd64MOV_reg_reg32(cb, amd64RDX, ar) // save An value

	amd64ALU_reg_imm32_32bit(cb, 5, m68kAMD64RegA7, 4) // A7 -= 4

	// I/O check
	amd64ALU_reg_reg32(cb, 0x39, m68kAMD64RegA7, m68kAMD64RegIOThresh)
	bailOff := amd64Jcc_rel32(cb, amd64CondAE)

	// Write An to stack (big-endian)
	emitREX(cb, false, 0, amd64RDX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RDX)) // BSWAP EDX
	emitMemOpSIB(cb, false, 0x89, amd64RDX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)

	// 2. An = A7 (frame pointer)
	m68kStoreAddrReg(cb, reg, m68kAMD64RegA7)

	// 3. A7 += displacement (negative = allocate)
	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, disp) // ADD A7, disp

	doneOff := amd64JMP_rel32(cb)

	// Bail
	patchRel32(cb, bailOff, cb.Len())
	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4) // undo SUB
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)

	patchRel32(cb, doneOff, cb.Len())
}

// m68kEmitUNLK emits UNLK An.
func m68kEmitUNLK(cb *CodeBuffer, ji *M68KJITInstr, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset
	reg := ji.opcode & 7

	// 1. A7 = An
	ar := m68kResolveAddrReg(cb, reg, amd64RAX)
	amd64MOV_reg_reg32(cb, m68kAMD64RegA7, ar)

	// 2. Pop An: An = Read32_BE([A7]); A7 += 4
	amd64ALU_reg_reg32(cb, 0x39, m68kAMD64RegA7, m68kAMD64RegIOThresh)
	bailOff := amd64Jcc_rel32(cb, amd64CondAE)

	// Read from stack (big-endian)
	emitMemOpSIB(cb, false, 0x8B, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX)) // BSWAP EAX

	// A7 += 4
	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4)

	// Store popped value to An
	m68kStoreAddrReg(cb, reg, amd64RAX)

	doneOff := amd64JMP_rel32(cb)

	// Bail
	patchRel32(cb, bailOff, cb.Len())
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)

	patchRel32(cb, doneOff, cb.Len())
}

// m68kEmitADDQ emits ADDQ #data,<ea>.
func m68kEmitADDQ(cb *CodeBuffer, opcode uint16) {
	data := (opcode >> 9) & 7
	if data == 0 {
		data = 8
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if mode == 0 { // Dn
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 0, r, int32(data)) // ADD r32, imm8
		emitCCR_Arithmetic(cb)
		m68kStoreDataReg(cb, reg, r)
	} else if mode == 1 { // An — no flags affected
		r := m68kResolveAddrReg(cb, reg, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 0, r, int32(data))
		m68kStoreAddrReg(cb, reg, r)
	}
	// Memory modes not yet implemented
}

// m68kEmitSUBQ emits SUBQ #data,<ea>.
func m68kEmitSUBQ(cb *CodeBuffer, opcode uint16) {
	data := (opcode >> 9) & 7
	if data == 0 {
		data = 8
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if mode == 0 { // Dn
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 5, r, int32(data)) // SUB r32, imm8
		emitCCR_Arithmetic(cb)
		m68kStoreDataReg(cb, reg, r)
	} else if mode == 1 { // An — no flags affected
		r := m68kResolveAddrReg(cb, reg, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 5, r, int32(data))
		m68kStoreAddrReg(cb, reg, r)
	}
}

// m68kEmitShift emits LSL/LSR/ASL/ASR (register form).
// shiftOp: 4=SHL, 5=SHR, 7=SAR (x86 encoding). aslFlag for ASL V-flag special case.
func m68kEmitShift(cb *CodeBuffer, opcode uint16) {
	reg := opcode & 7
	countField := (opcode >> 9) & 7
	isRegCount := (opcode >> 5) & 1 // 0=immediate, 1=register
	direction := (opcode >> 8) & 1  // 0=right, 1=left
	shiftType := (opcode >> 3) & 3  // 00=AS, 01=LS, 10=ROX, 11=RO

	// Determine x86 shift operation
	var x86ShiftOp byte
	switch {
	case direction == 1 && (shiftType == 0 || shiftType == 1): // ASL or LSL
		x86ShiftOp = 4 // SHL
	case direction == 0 && shiftType == 1: // LSR
		x86ShiftOp = 5 // SHR
	case direction == 0 && shiftType == 0: // ASR
		x86ShiftOp = 7 // SAR
	default:
		return // ROL/ROR/ROXL/ROXR not yet implemented
	}

	r := m68kResolveDataReg(cb, reg, amd64RAX)

	if isRegCount == 1 {
		// Count from register (modulo 64 on 68020)
		countReg := m68kResolveDataReg(cb, countField, amd64RCX)
		if countReg != amd64RCX {
			amd64MOV_reg_reg32(cb, amd64RCX, countReg)
		}
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 63) // AND ECX, 63

		// Emit shift: SHx r32, CL
		emitREX(cb, false, 0, r)
		cb.EmitBytes(0xD3, modRM(3, x86ShiftOp, r))
	} else {
		// Immediate count (0 encodes 8)
		count := countField
		if count == 0 {
			count = 8
		}
		// Emit shift: SHx r32, imm8
		emitREX(cb, false, 0, r)
		cb.EmitBytes(0xC1, modRM(3, x86ShiftOp, r), byte(count))
	}

	// Flags: C = last bit shifted out, N/Z from result, V=0 (for LSx), X=C
	// x86 shift sets CF to last bit shifted, SF/ZF from result, OF for 1-bit shifts
	// This is close enough for the common case. Extract flags.
	emitCCR_Arithmetic(cb) // Gets C, N, Z, V from native flags; X=C

	m68kStoreDataReg(cb, reg, r)
}

// m68kEmitADDA emits ADDA.W/L <ea>,An (add to address register, no flags).
func m68kEmitADDA(cb *CodeBuffer, opcode uint16) {
	dstReg := (opcode >> 9) & 7
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	opmode := (opcode >> 6) & 7

	if srcMode == 0 { // Dn source
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		if opmode == 3 { // ADDA.W — sign-extend word to long
			amd64MOV_reg_reg32(cb, amd64RAX, src)
			cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RAX, amd64RAX)) // MOVSX EAX, AX
			src = amd64RAX
		}
		dst := m68kResolveAddrReg(cb, dstReg, amd64RDX)
		amd64ALU_reg_reg32(cb, 0x01, dst, src) // ADD dst, src
		m68kStoreAddrReg(cb, dstReg, dst)
	} else if srcMode == 1 { // An source
		src := m68kResolveAddrReg(cb, srcReg, amd64RAX)
		if opmode == 3 {
			amd64MOV_reg_reg32(cb, amd64RAX, src)
			cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RAX, amd64RAX))
			src = amd64RAX
		}
		dst := m68kResolveAddrReg(cb, dstReg, amd64RDX)
		amd64ALU_reg_reg32(cb, 0x01, dst, src)
		m68kStoreAddrReg(cb, dstReg, dst)
	}
	// No flags affected
}

// m68kEmitSUBA emits SUBA.W/L <ea>,An (subtract from address register, no flags).
func m68kEmitSUBA(cb *CodeBuffer, opcode uint16) {
	dstReg := (opcode >> 9) & 7
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	opmode := (opcode >> 6) & 7

	if srcMode == 0 { // Dn source
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		if opmode == 3 { // SUBA.W
			amd64MOV_reg_reg32(cb, amd64RAX, src)
			cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RAX, amd64RAX))
			src = amd64RAX
		}
		dst := m68kResolveAddrReg(cb, dstReg, amd64RDX)
		amd64ALU_reg_reg32(cb, 0x29, dst, src) // SUB dst, src
		m68kStoreAddrReg(cb, dstReg, dst)
	} else if srcMode == 1 { // An source
		src := m68kResolveAddrReg(cb, srcReg, amd64RAX)
		if opmode == 3 {
			amd64MOV_reg_reg32(cb, amd64RAX, src)
			cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RAX, amd64RAX))
			src = amd64RAX
		}
		dst := m68kResolveAddrReg(cb, dstReg, amd64RDX)
		amd64ALU_reg_reg32(cb, 0x29, dst, src)
		m68kStoreAddrReg(cb, dstReg, dst)
	}
}

// ===========================================================================
// Block Compilation
// ===========================================================================

// m68kCompileBlock compiles a scanned block of M68K instructions to x86-64.
func m68kCompileBlock(instrs []M68KJITInstr, startPC uint32, execMem *ExecMem) (*JITBlock, error) {
	return m68kCompileBlockWithMem(instrs, startPC, execMem, nil)
}

// m68kCompileBlockWithMem compiles with access to memory for reading branch displacements.
func m68kCompileBlockWithMem(instrs []M68KJITInstr, startPC uint32, execMem *ExecMem, memory []byte) (*JITBlock, error) {
	cb := NewCodeBuffer(len(instrs) * 512)

	br := m68kAnalyzeBlockRegs(instrs)
	br.hasBackwardBranch = m68kDetectBackwardBranchesWithMem(instrs, startPC, memory)
	m68kEmitPrologue(cb, startPC, &br)

	instrOffsets := make([]int, len(instrs))

	for i := range instrs {
		instrOffsets[i] = cb.Len()
		ji := &instrs[i]
		m68kEmitInstructionFull(cb, ji, startPC, &br, i, len(instrs), memory, instrOffsets, instrs)
	}

	// If the last instruction doesn't have its own epilogue, emit fallthrough
	lastOp := instrs[len(instrs)-1].opcode
	if !m68kIsBlockTerminator(lastOp) {
		lastInstr := &instrs[len(instrs)-1]
		endPC := startPC + lastInstr.pcOffset + uint32(lastInstr.length)
		m68kEmitRetPC(cb, endPC, uint32(len(instrs)))
		m68kEmitEpilogue(cb, &br)
	}

	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, err
	}

	lastInstr := &instrs[len(instrs)-1]
	endPC := startPC + lastInstr.pcOffset + uint32(lastInstr.length)

	return &JITBlock{
		startPC:    startPC,
		endPC:      endPC,
		instrCount: len(instrs),
		execAddr:   addr,
		execSize:   len(code),
	}, nil
}

// m68kEmitInstructionFull dispatches to the appropriate emitter, with full context.
func m68kEmitInstructionFull(cb *CodeBuffer, ji *M68KJITInstr, blockStartPC uint32, br *m68kBlockRegs, instrIdx int, blockLen int, memory []byte, instrOffsets []int, instrs []M68KJITInstr) {
	opcode := ji.opcode
	group := opcode >> 12

	switch group {
	case 0x7: // MOVEQ
		m68kEmitMOVEQ(cb, opcode)
		return

	case 0x1, 0x2, 0x3: // MOVE
		size := M68K_SIZE_LONG
		if group == 0x1 {
			size = M68K_SIZE_BYTE
		} else if group == 0x3 {
			size = M68K_SIZE_WORD
		}
		srcMode := (opcode >> 3) & 7
		dstMode := (opcode >> 6) & 7
		if srcMode == 0 && dstMode == 0 { // Dn → Dn (fast path)
			m68kEmitMOVE_Dn_Dn(cb, opcode, size)
			return
		}
		// General MOVE with any EA (requires memory for extension words)
		if memory != nil {
			m68kEmitMOVE_Full(cb, ji, memory, blockStartPC, size)
			return
		}

	case 0xD: // ADD/ADDA/ADDX
		opmode := (opcode >> 6) & 7
		srcMode := (opcode >> 3) & 7
		if opmode == 3 || opmode == 7 { // ADDA
			m68kEmitADDA(cb, opcode)
			return
		}
		if opmode <= 2 { // ADD.x <ea>,Dn
			if srcMode == 0 { // Dn,Dn fast path
				m68kEmitADD_Dn_Dn(cb, opcode)
			} else if memory != nil { // EA,Dn with memory access
				m68kEmitADD_EA_Dn(cb, ji, memory, blockStartPC)
			}
			return
		}

	case 0x9: // SUB/SUBA/SUBX
		opmode := (opcode >> 6) & 7
		srcMode := (opcode >> 3) & 7
		if opmode == 3 || opmode == 7 { // SUBA
			m68kEmitSUBA(cb, opcode)
			return
		}
		if opmode <= 2 { // SUB.x <ea>,Dn
			if srcMode == 0 {
				m68kEmitSUB_Dn_Dn(cb, opcode)
			} else if memory != nil {
				m68kEmitSUB_EA_Dn(cb, ji, memory, blockStartPC)
			}
			return
		}

	case 0xB: // CMP/EOR
		opmode := (opcode >> 6) & 7
		srcMode := (opcode >> 3) & 7
		if opmode <= 2 { // CMP.x <ea>,Dn
			if srcMode == 0 {
				m68kEmitCMP_Dn_Dn(cb, opcode)
			} else if memory != nil {
				m68kEmitCMP_EA_Dn(cb, ji, memory, blockStartPC)
			}
			return
		}
		if opmode >= 4 && opmode <= 6 && srcMode == 0 { // EOR Dn,Dn
			m68kEmitEOR_Dn_Dn(cb, opcode)
			return
		}

	case 0xC: // AND
		srcMode := (opcode >> 3) & 7
		opmode := (opcode >> 6) & 7
		if srcMode == 0 && opmode <= 2 {
			m68kEmitAND_Dn_Dn(cb, opcode)
			return
		}

	case 0x8: // OR
		srcMode := (opcode >> 3) & 7
		opmode := (opcode >> 6) & 7
		if srcMode == 0 && opmode <= 2 {
			m68kEmitOR_Dn_Dn(cb, opcode)
			return
		}

	case 0x4: // Misc
		// CLR
		if opcode&0xFF00 == 0x4200 {
			m68kEmitCLR(cb, opcode)
			return
		}
		// NOT
		if opcode&0xFF00 == 0x4600 {
			m68kEmitNOT(cb, opcode)
			return
		}
		// NEG
		if opcode&0xFF00 == 0x4400 {
			m68kEmitNEG(cb, opcode)
			return
		}
		// TST
		if opcode&0xFF00 == 0x4A00 {
			m68kEmitTST(cb, opcode)
			return
		}
		// SWAP
		if opcode&0xFFF8 == 0x4840 {
			m68kEmitSWAP(cb, opcode)
			return
		}
		// EXT/EXTB
		if opcode&0xFFF8 == 0x4880 || opcode&0xFFF8 == 0x48C0 || opcode&0xFFF8 == 0x49C0 {
			m68kEmitEXT(cb, opcode)
			return
		}
		// NOP
		if opcode == 0x4E71 {
			return // nothing to emit
		}
		// RTS
		if opcode == 0x4E75 {
			m68kEmitRTS(cb, br, instrIdx)
			return
		}
		// JSR
		if opcode&0xFFC0 == 0x4E80 {
			m68kEmitJSR(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// JMP
		if opcode&0xFFC0 == 0x4EC0 {
			m68kEmitJMP(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// LEA
		if opcode&0xF1C0 == 0x41C0 && (opcode>>3)&7 >= 2 {
			if memory != nil {
				m68kEmitLEA(cb, ji, memory, blockStartPC)
				return
			}
		}
		// PEA
		if opcode&0xFFC0 == 0x4840 && (opcode>>3)&7 >= 2 {
			if memory != nil {
				m68kEmitPEA(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		// LINK.W
		if opcode&0xFFF8 == 0x4E50 {
			if memory != nil {
				m68kEmitLINK(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		// LINK.L
		if opcode&0xFFF8 == 0x4808 {
			if memory != nil {
				m68kEmitLINK(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		// UNLK
		if opcode&0xFFF8 == 0x4E58 {
			m68kEmitUNLK(cb, ji, blockStartPC, br, instrIdx)
			return
		}
		// Bail terminators: STOP, TRAP, RTE, RTR, RESET, TRAPV
		// These are block terminators that can't be JIT-compiled.
		// Emit bail to interpreter so dispatcher re-executes via StepOne().
		if opcode == 0x4E72 || opcode == 0x4E73 || opcode == 0x4E77 ||
			opcode == 0x4E70 || opcode == 0x4E76 || opcode&0xFFF0 == 0x4E40 {
			instrPC := blockStartPC + ji.pcOffset
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}

	case 0x6: // Bcc/BRA/BSR
		cond := (opcode >> 8) & 0xF
		if cond == 0 { // BRA
			m68kEmitBRA(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if cond == 1 { // BSR
			m68kEmitBSR(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// Bcc (cond >= 2) — conditional branch, NOT a terminator
		m68kEmitBcc(cb, ji, memory, blockStartPC, br, instrIdx, instrOffsets, blockLen, instrs)
		return

	case 0x5: // ADDQ/SUBQ/Scc/DBcc
		// DBcc
		if opcode&0xF0F8 == 0x50C8 {
			m68kEmitDBcc(cb, ji, memory, blockStartPC, br, instrIdx, instrOffsets, instrs)
			return
		}
		// Scc
		if opcode&0x00C0 == 0x00C0 && opcode&0xF0F8 != 0x50F8 {
			m68kEmitScc(cb, opcode)
			return
		}
		// ADDQ/SUBQ (size field != 3)
		if opcode&0x00C0 != 0x00C0 {
			if opcode&0x0100 == 0 { // ADDQ
				m68kEmitADDQ(cb, opcode)
			} else { // SUBQ
				m68kEmitSUBQ(cb, opcode)
			}
			return
		}

	case 0xA: // Line A trap — bail to interpreter
		instrPC := blockStartPC + ji.pcOffset
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return

	case 0xF: // Line F / FPU — bail to interpreter
		instrPC := blockStartPC + ji.pcOffset
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return

	case 0xE: // Shifts, Rotates, Bit Fields
		// Register/immediate shifts (size != 3)
		if (opcode>>6)&3 != 3 {
			m68kEmitShift(cb, opcode)
			return
		}
	}

	// Unhandled instruction: bail to interpreter.
	// Set RetPC to this instruction's PC so dispatcher re-executes via interpreter.
	instrPC := blockStartPC + ji.pcOffset
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
}
