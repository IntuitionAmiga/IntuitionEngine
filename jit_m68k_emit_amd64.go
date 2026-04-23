// jit_m68k_emit_amd64.go - x86-64 native code emitter for M68020 JIT compiler

//go:build amd64 && (linux || windows || darwin)

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

	// Stack frame: 6 callee-saved pushes (48 bytes) + SUB RSP,40 = 88 + 8 (ret addr) = 96 (16-aligned)
	m68kAMD64FrameSize    = 40
	m68kAMD64OffCtxPtr    = 0  // [RSP+0] = saved JITContext pointer (backup)
	m68kAMD64OffSRPtr     = 8  // [RSP+8] = SR pointer
	m68kAMD64OffLoopCount = 16 // [RSP+16] = backward branch loop counter
	// [RSP+24] = X flag byte (lazy CCR) — defined as m68kAMD64OffXFlag above
	// [RSP+32] = reserved for Stage 6 (context pointer for register expansion)
)

// M68K backward branch budget
const m68kJitBudget = 4095

// ===========================================================================
// Lazy CCR — Flag State Tracking
// ===========================================================================

// m68kFlagState tracks whether M68K CCR is materialized in R14 or live in host EFLAGS.
type m68kFlagState int

const (
	flagsMaterialized m68kFlagState = iota // R14 holds valid 5-bit CCR
	flagsLiveArith                         // EFLAGS from ADD/SUB/NEG; X saved to [RSP+xFlagOff]
	flagsLiveArithNoX                      // EFLAGS from CMP (X not modified)
	flagsLiveLogi                          // EFLAGS from AND/OR/EOR/MOVE/TST/CLR (V=0,C=0; X in stack slot)
)

// m68kCompileState tracks flag liveness during block compilation.
type m68kCompileState struct {
	flagState m68kFlagState
}

// Stack frame X flag slot offset
const m68kAMD64OffXFlag = 24 // [RSP+24] = X flag byte (lazy CCR)

// m68kDirectJccTable maps M68K condition codes (0-15) to x86-64 Jcc condition codes.
// Used when EFLAGS is live from the previous flag-setting instruction.
// After arithmetic ops (ADD/SUB/CMP/NEG), x86 flags map perfectly to M68K conditions.
// After logical ops (AND/OR/EOR/TEST), OF=0 and CF=0 which matches M68K V=0, C=0.
// Values are x86 condition codes (0-15) for use with amd64Jcc_rel32.
var m68kDirectJccTable = [16]byte{
	0xFF,        // 0: T (always) — special
	0xFE,        // 1: F (never) — special
	amd64CondA,  // 2: HI (C=0 AND Z=0) → JA (0x7)
	amd64CondBE, // 3: LS (C=1 OR Z=1) → JBE (0x6)
	amd64CondAE, // 4: CC (C=0) → JAE (0x3)
	amd64CondB,  // 5: CS (C=1) → JB (0x2)
	amd64CondNE, // 6: NE (Z=0) → JNE (0x5)
	amd64CondE,  // 7: EQ (Z=1) → JE (0x4)
	0x1,         // 8: VC (V=0) → JNO
	amd64CondO,  // 9: VS (V=1) → JO (0x0)
	0x9,         // 10: PL (N=0) → JNS
	0x8,         // 11: MI (N=1) → JS
	amd64CondGE, // 12: GE (N⊕V=0) → JGE (0xD)
	amd64CondL,  // 13: LT (N⊕V=1) → JL (0xC)
	amd64CondG,  // 14: GT (Z=0 AND N⊕V=0) → JG (0xF)
	amd64CondLE, // 15: LE (Z=1 OR N⊕V=1) → JLE (0xE)
}

// m68kCondToDirectJcc returns the x86-64 Jcc opcode for a M68K condition when
// EFLAGS is live. Does NOT emit any code (unlike m68kCondToJcc which emits R14 bit tests).
func m68kCondToDirectJcc(m68kCond uint16) byte {
	if m68kCond < 16 {
		return m68kDirectJccTable[m68kCond]
	}
	return 0xFE
}

// amd64MOVZX_B_mem emits MOVZX r32, BYTE [base + disp] (zero-extend byte from memory).
func amd64MOVZX_B_mem(cb *CodeBuffer, dst, base byte, disp int32) {
	emitREX(cb, false, dst, base)
	cb.EmitBytes(0x0F, 0xB6)
	baseBits := regBits(base)
	needsSIB := baseBits == 4
	if disp >= -128 && disp <= 127 {
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

// m68kInstrNeedsCCRMaterialization returns true if this instruction clobbers
// EFLAGS without setting M68K flags, requiring CCR materialization first.
func m68kInstrNeedsCCRMaterialization(ji *M68KJITInstr) bool {
	opcode := ji.opcode
	group := opcode >> 12

	switch group {
	case 0x4: // Misc group
		// RTS — clobbers EFLAGS via CMP (I/O check), ADD (A7), CMP (cache check)
		if opcode == 0x4E75 {
			return true
		}
		// JSR — clobbers EFLAGS via CMP (I/O check), ADD/SUB (A7)
		if opcode&0xFFC0 == 0x4E80 {
			return true
		}
		// LEA
		if opcode&0xF1C0 == 0x41C0 && (opcode>>3)&7 >= 2 {
			return true
		}
		// PEA
		if opcode&0xFFC0 == 0x4840 && (opcode>>3)&7 >= 2 {
			return true
		}
		// LINK.W / LINK.L
		if opcode&0xFFF8 == 0x4E50 || opcode&0xFFF8 == 0x4808 {
			return true
		}
		// UNLK
		if opcode&0xFFF8 == 0x4E58 {
			return true
		}
		// NOP — does NOT clobber EFLAGS
		if opcode == 0x4E71 {
			return false
		}
	case 0xD: // ADD/ADDA
		opmode := (opcode >> 6) & 7
		if opmode == 3 || opmode == 7 { // ADDA — no M68K flags
			return true
		}
	case 0x9: // SUB/SUBA
		opmode := (opcode >> 6) & 7
		if opmode == 3 || opmode == 7 { // SUBA — no M68K flags
			return true
		}
	case 0x5: // ADDQ/SUBQ with An mode
		mode := (opcode >> 3) & 7
		if opcode&0x00C0 != 0x00C0 && mode == 1 { // ADDQ/SUBQ An
			return true
		}
	}
	return false
}

// m68kCurrentCS is the compile state for the currently-compiling block.
// Set by m68kCompileBlockWithMem; read by emitCCR_Arithmetic/Logic and m68kCondToJcc.
// Safe: M68K JIT compilation is single-threaded.
var m68kCurrentCS *m68kCompileState

func m68kEAMayUseMemHelper(mode, reg uint16, isWrite bool) bool {
	switch mode {
	case 2, 3, 4, 5, 6:
		return true
	case 7:
		if isWrite {
			return reg <= 1 // abs.W / abs.L
		}
		return reg <= 3 // abs.W / abs.L / (d16,PC) / (d8,PC,Xn)
	}
	return false
}

func m68kInstrMaySetGenericIOFallback(ji *M68KJITInstr) bool {
	opcode := ji.opcode
	group := opcode >> 12

	switch group {
	case 0x1, 0x2, 0x3: // MOVE
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		return m68kEAMayUseMemHelper(srcMode, srcReg, false) || m68kEAMayUseMemHelper(dstMode, dstReg, true)
	case 0x8, 0x9, 0xB, 0xD: // OR/SUB/CMP/ADD EA,Dn paths
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		opmode := (opcode >> 6) & 7
		if group == 0xB {
			return opmode <= 2 && m68kEAMayUseMemHelper(srcMode, srcReg, false)
		}
		if group == 0x9 || group == 0xD {
			return opmode <= 2 && m68kEAMayUseMemHelper(srcMode, srcReg, false)
		}
		if group == 0x8 {
			return srcMode != 0 && opmode <= 2 && m68kEAMayUseMemHelper(srcMode, srcReg, false)
		}
	case 0x4: // Misc family
		if opcode&0xFF80 == 0x4C00 { // MULL/DIVL
			srcMode := (opcode >> 3) & 7
			srcReg := opcode & 7
			return m68kEAMayUseMemHelper(srcMode, srcReg, false)
		}
	}
	return false
}

// m68kSaveXToStack emits SETB [RSP+24] to save CF (= M68K X flag) to the stack slot.
func m68kSaveXToStack(cb *CodeBuffer) {
	// SETB [RSP + 24]: 0F 92 44 24 18
	cb.EmitBytes(0x0F, 0x92)                          // SETB
	cb.EmitBytes(0x44, 0x24, byte(m68kAMD64OffXFlag)) // ModRM=01 100 100 (disp8, SIB), SIB=00 100 100 (RSP base), disp8=24
}

// m68kMaterializeCCR extracts EFLAGS into R14 when flags are live.
// Must be called before any block exit or non-flag instruction that clobbers EFLAGS.
func m68kMaterializeCCR(cb *CodeBuffer, cs *m68kCompileState) {
	if cs == nil || cs.flagState == flagsMaterialized {
		return
	}

	switch cs.flagState {
	case flagsLiveArith:
		// Full extraction: same as emitCCR_Arithmetic but read X from stack slot
		amd64SETcc(cb, amd64CondB, amd64RCX) // CL = C
		amd64SETcc(cb, amd64CondO, amd64RDX) // DL = V
		amd64SETcc(cb, amd64CondE, amd64R10) // R10B = Z
		amd64SETcc(cb, 0x8, amd64R11)        // R11B = N

		amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX) // R14 = C
		amd64MOVZX_B(cb, amd64RAX, amd64RDX)
		amd64SHL_imm(cb, amd64RAX, 1)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // |= V<<1
		amd64MOVZX_B(cb, amd64RAX, amd64R10)
		amd64SHL_imm(cb, amd64RAX, 2)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // |= Z<<2
		amd64MOVZX_B(cb, amd64RAX, amd64R11)
		amd64SHL_imm(cb, amd64RAX, 3)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // |= N<<3

		// X from stack slot
		amd64MOVZX_B_mem(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffXFlag))
		amd64SHL_imm(cb, amd64RAX, 4)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // |= X<<4

	case flagsLiveArithNoX:
		// Same as flagsLiveArith but read X from stack slot (not CF).
		// CMP doesn't modify X, so the stack slot has the correct value
		// from the last arithmetic op's SETB or the prologue's X seeding.
		amd64SETcc(cb, amd64CondB, amd64RCX) // CL = C
		amd64SETcc(cb, amd64CondO, amd64RDX) // DL = V
		amd64SETcc(cb, amd64CondE, amd64R10) // R10B = Z
		amd64SETcc(cb, 0x8, amd64R11)        // R11B = N

		amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX) // R14 = C
		amd64MOVZX_B(cb, amd64RAX, amd64RDX)
		amd64SHL_imm(cb, amd64RAX, 1)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // |= V<<1
		amd64MOVZX_B(cb, amd64RAX, amd64R10)
		amd64SHL_imm(cb, amd64RAX, 2)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // |= Z<<2
		amd64MOVZX_B(cb, amd64RAX, amd64R11)
		amd64SHL_imm(cb, amd64RAX, 3)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // |= N<<3

		// X from stack slot (preserved from last arithmetic op or prologue seed)
		amd64MOVZX_B_mem(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffXFlag))
		amd64SHL_imm(cb, amd64RAX, 4)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // |= X<<4

	case flagsLiveLogi:
		// N and Z from EFLAGS; V=0, C=0; X from stack slot
		amd64SETcc(cb, amd64CondE, amd64RCX) // CL = Z
		amd64SETcc(cb, 0x8, amd64RDX)        // DL = N

		// X from stack slot → bit 4
		amd64MOVZX_B_mem(cb, m68kAMD64RegCCR, amd64RSP, int32(m68kAMD64OffXFlag))
		amd64SHL_imm(cb, m68kAMD64RegCCR, 4) // R14 = X << 4

		amd64MOVZX_B(cb, amd64RAX, amd64RCX)
		amd64SHL_imm(cb, amd64RAX, 2)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // |= Z<<2
		amd64MOVZX_B(cb, amd64RAX, amd64RDX)
		amd64SHL_imm(cb, amd64RAX, 3)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // |= N<<3
	}

	cs.flagState = flagsMaterialized
}

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
// In lazy mode (m68kCurrentCS != nil): saves X to stack slot and defers full extraction.
// CRITICAL: SETcc does NOT modify flags, but SHL/OR DO. So we must extract
// all 4 flags via SETcc FIRST, then combine them.
func emitCCR_Arithmetic(cb *CodeBuffer) {
	if cs := m68kCurrentCS; cs != nil {
		// Lazy mode: save X (CF) to stack slot, defer full extraction
		m68kSaveXToStack(cb) // SETB [RSP+24]
		cs.flagState = flagsLiveArith
		return
	}
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
// In lazy mode: defers extraction, sets flagsLiveLogi.
// Expects: native flags set by TEST or the logical operation itself.
// CRITICAL: Extract flags via SETcc BEFORE any AND/SHL/OR clobbers them.
func emitCCR_Logic(cb *CodeBuffer) {
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsLiveLogi
		return
	}
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

// m68kEmitPrologue emits the full block entry: push callee-saved, load base
// pointers from context. Does NOT load mapped registers or extract CCR — that
// is done by m68kEmitChainEntry which immediately follows and is shared with
// the chain entry path.
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
	amd64MOV_reg_mem(cb, m68kAMD64RegMemBase, m68kAMD64RegCtx, int32(m68kCtxOffMemPtr))         // RSI = MemPtr
	amd64MOV_reg_mem32(cb, m68kAMD64RegIOThresh, m68kAMD64RegCtx, int32(m68kCtxOffIOThreshold)) // R8d = IOThreshold
	amd64MOV_reg_mem(cb, m68kAMD64RegAddrBase, m68kAMD64RegCtx, int32(m68kCtxOffAddrRegsPtr))   // R9 = AddrRegsPtr

	// Save SR pointer to stack
	amd64MOV_reg_mem(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffSRPtr))
	amd64MOV_mem_reg(cb, amd64RSP, int32(m68kAMD64OffSRPtr), amd64RAX) // [RSP+8] = SRPtr

	// Load DataRegsPtr into RDI
	amd64MOV_reg_mem(cb, m68kAMD64RegDataBase, m68kAMD64RegCtx, int32(m68kCtxOffDataRegsPtr))

	// Falls through to m68kEmitChainEntry which loads mapped regs + CCR
}

func m68kEmitEpilogue(cb *CodeBuffer, br *m68kBlockRegs) {
	// Materialize lazy CCR before merging into SR
	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
	}
	// Always spill mapped registers at block exit. The extra stores are cheap
	// compared with interpreter fallback and they avoid stale host-register
	// state when the block-level write analysis misses a path edge.
	amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, 0*4, m68kAMD64RegD0)
	amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, 1*4, m68kAMD64RegD1)
	amd64MOV_mem_reg32(cb, m68kAMD64RegAddrBase, 0*4, m68kAMD64RegA0)
	amd64MOV_mem_reg32(cb, m68kAMD64RegAddrBase, 7*4, m68kAMD64RegA7)

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
// Block Chaining — Chain Entry / Chain Exit
// ===========================================================================

// m68kEmitChainEntry emits register loads and CCR extraction. This code is
// shared between the full entry path (after prologue falls through) and the
// chain entry path (jumped to from another block's chain exit).
//
// For the full entry, the prologue has already set up R15/RDI/RSI/R8/R9 and
// the stack frame. For chain entry, those are still valid from the original
// callNative.
//
// Returns the code buffer offset of the chain entry label.
func m68kEmitChainEntry(cb *CodeBuffer, br *m68kBlockRegs) int {
	entryOff := cb.Len()

	// Reset backward branch loop counter
	if br.hasBackwardBranch {
		amd64MOV_mem_imm32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), 0)
	}

	// Load ALL mapped data registers unconditionally.
	// Chain entry doesn't know which block is chaining to us, so we must reload all.
	// For the full entry path (prologue falls through), the extra loads are negligible.
	amd64MOV_reg_mem32(cb, m68kAMD64RegD0, m68kAMD64RegDataBase, 0*4) // D0 -> RBX
	amd64MOV_reg_mem32(cb, m68kAMD64RegD1, m68kAMD64RegDataBase, 1*4) // D1 -> RBP

	// Load ALL mapped address registers unconditionally
	amd64MOV_reg_mem32(cb, m68kAMD64RegA0, m68kAMD64RegAddrBase, 0*4) // A0 -> R12
	amd64MOV_reg_mem32(cb, m68kAMD64RegA7, m68kAMD64RegAddrBase, 7*4) // A7 -> R13

	// Extract CCR from SR: R14 = *SRPtr & 0x1F
	amd64MOV_reg_mem(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffSRPtr))
	amd64MOV_reg_mem32(cb, m68kAMD64RegCCR, amd64RAX, 0)
	amd64MOVZX_W(cb, m68kAMD64RegCCR, m68kAMD64RegCCR)
	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, 0x1F) // AND R14, 0x1F

	// Seed X flag stack slot from R14 (bit 4) for lazy CCR
	amd64MOV_reg_reg32(cb, amd64RAX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RAX, 4)                                     // bit 4 → bit 0
	emitMemOp(cb, false, 0x88, amd64RAX, amd64RSP, m68kAMD64OffXFlag) // MOV [RSP+24], AL

	return entryOff
}

// m68kEmitLightweightEpilogue stores mapped registers back and merges CCR into SR.
// Does NOT pop callee-saved registers or RET — used before chain exits.
// Materializes lazy CCR into R14 before merging if needed.
func m68kEmitLightweightEpilogue(cb *CodeBuffer, br *m68kBlockRegs) {
	// Materialize lazy CCR before merging into SR
	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
	}
	amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, 0*4, m68kAMD64RegD0)
	amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, 1*4, m68kAMD64RegD1)
	amd64MOV_mem_reg32(cb, m68kAMD64RegAddrBase, 0*4, m68kAMD64RegA0)
	amd64MOV_mem_reg32(cb, m68kAMD64RegAddrBase, 7*4, m68kAMD64RegA7)

	// Merge CCR back into SR
	amd64MOV_reg_mem(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffSRPtr))
	amd64MOV_reg_mem32(cb, amd64RDX, amd64RAX, 0)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, int32(0xFFE0)) // EDX &= 0xFFE0
	amd64MOV_reg_reg32(cb, amd64RCX, m68kAMD64RegCCR)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 0x1F)
	amd64ALU_reg_reg32(cb, 0x09, amd64RDX, amd64RCX) // EDX |= ECX
	cb.EmitBytes(0x66)                               // 16-bit prefix
	emitREX(cb, false, amd64RDX, amd64RAX)
	cb.EmitBytes(0x89, modRM(0, amd64RDX, amd64RAX)) // MOV [RAX], DX
}

// m68kEmitFullEpilogueEnd emits the pop callee-saved + RET sequence.
// Used at the end of an unchained exit path.
func m68kEmitFullEpilogueEnd(cb *CodeBuffer) {
	amd64ALU_reg_imm32(cb, 0, amd64RSP, int32(m68kAMD64FrameSize)) // ADD RSP, frameSize
	amd64POP(cb, amd64R15)
	amd64POP(cb, amd64R14)
	amd64POP(cb, amd64R13)
	amd64POP(cb, amd64R12)
	amd64POP(cb, amd64RBP)
	amd64POP(cb, amd64RBX)
	amd64RET(cb)
}

// m68kChainExitInfo is returned by m68kEmitChainExit to track the patchable JMP.
type m68kChainExitInfo struct {
	targetPC      uint32 // M68K target PC for this exit
	jmpDispOffset int    // offset within CodeBuffer of the JMP rel32 displacement
}

// amd64ALU_mem_imm8 emits an ALU op (ADD/CMP/SUB) with [base+disp], imm8.
// Opcode 0x83, reg field = aluOp. Sign-extends imm8 to 32 bits.
func amd64ALU_mem_imm8(cb *CodeBuffer, aluOp byte, base byte, disp int32, imm8 int8) {
	emitMemOp(cb, false, 0x83, aluOp, base, disp)
	cb.EmitBytes(byte(imm8))
}

// amd64ALU_reg_mem32_cmp emits CMP reg32, [base + disp] (opcode 0x3B).
func amd64ALU_reg_mem32_cmp(cb *CodeBuffer, reg, base byte, disp int32) {
	emitMemOp(cb, false, 0x3B, reg, base, disp)
}

// amd64DEC_mem32 emits DEC DWORD [base+disp]. Opcode FF /1.
func amd64DEC_mem32(cb *CodeBuffer, base byte, disp int32) {
	emitMemOp(cb, false, 0xFF, 1, base, disp)
}

// m68kEmitChainExit emits a chain exit sequence for a block terminator with a
// statically known target PC. The sequence:
//  1. Lightweight epilogue (store regs, merge CCR)
//  2. Accumulate instruction count into ChainCount
//  3. Decrement ChainBudget; if exhausted → unchained exit
//  4. Check NeedInval; if set → unchained exit
//  5. Patchable JMP rel32 (initially to unchained exit)
//  6. Unchained exit: set RetPC/RetCount, full pop/ret
//
// Returns the chain exit info for later patching.
func m68kEmitChainExit(cb *CodeBuffer, targetPC uint32, instrCount uint32, br *m68kBlockRegs) m68kChainExitInfo {
	m68kEmitLightweightEpilogue(cb, br)

	// ADD DWORD [R15 + ChainCount], instrCount
	// Use: load into scratch, add, store back
	amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffChainCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrCount)) // ADD EAX, instrCount
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffChainCount), amd64RAX)

	// DEC DWORD [R15 + ChainBudget]
	amd64DEC_mem32(cb, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget))

	// JLE .unchained (budget exhausted — ChainBudget was signed, now <= 0)
	unchainedOff1 := amd64Jcc_rel32(cb, amd64CondLE)

	// CMP DWORD [R15 + NeedInval], 0
	amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedInval), 0)
	// JNE .unchained (self-mod detected)
	unchainedOff2 := amd64Jcc_rel32(cb, amd64CondNE)

	// Patchable JMP rel32 — initially jumps to .unchained
	jmpOff := cb.Len()
	cb.EmitBytes(0xE9, 0, 0, 0, 0) // JMP rel32 (placeholder)
	jmpDispOffset := jmpOff + 1    // displacement starts at byte after opcode

	// .unchained label
	unchainedLabel := cb.Len()
	patchRel32(cb, unchainedOff1, unchainedLabel)
	patchRel32(cb, unchainedOff2, unchainedLabel)
	// Patch the initial JMP to point to unchained (will be overwritten when target compiles)
	patchRel32(cb, jmpDispOffset, unchainedLabel)

	// Set RetPC = targetPC
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), targetPC)
	// RetCount = ChainCount (already accumulated)
	amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffChainCount))
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), amd64RAX)

	// Full pop/ret
	m68kEmitFullEpilogueEnd(cb)

	return m68kChainExitInfo{
		targetPC:      targetPC,
		jmpDispOffset: jmpDispOffset,
	}
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

		// Lazy CCR: CMP sets NZVC but NOT X — use flagsLiveArithNoX
		// so materializer preserves old X from R14 instead of reading CF
		if cs := m68kCurrentCS; cs != nil {
			cs.flagState = flagsLiveArithNoX
			return
		}

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

// m68kEmitNOT emits NOT.B/W/L Dn.
func m68kEmitNOT(cb *CodeBuffer, opcode uint16) {
	size := (opcode >> 6) & 3
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if mode == 0 { // Data register direct
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		switch size {
		case 0: // NOT.B
			amd64ALU_reg_imm32_32bit(cb, 6, r, 0xFF) // XOR low byte bits only
			m68kStoreDataReg(cb, reg, r)
			scratch := byte(amd64RAX)
			if r == scratch {
				scratch = byte(amd64RCX)
			}
			amd64MOV_reg_reg32(cb, scratch, r)
			amd64MOVZX_B(cb, scratch, scratch)
			amd64SHL_imm32(cb, scratch, 24) // promote bit 7 to 32-bit sign flag
		case 1: // NOT.W
			amd64ALU_reg_imm32_32bit(cb, 6, r, 0xFFFF) // XOR low word bits only
			m68kStoreDataReg(cb, reg, r)
			scratch := byte(amd64RAX)
			if r == scratch {
				scratch = byte(amd64RCX)
			}
			amd64MOV_reg_reg32(cb, scratch, r)
			amd64MOVZX_W(cb, scratch, scratch)
			amd64SHL_imm32(cb, scratch, 16) // promote bit 15 to 32-bit sign flag
		case 2: // NOT.L
			// NOT in x86 doesn't affect flags, so we need TEST after.
			emitREX(cb, false, 0, r)
			cb.EmitBytes(0xF7, modRM(3, 2, r)) // NOT r32
			m68kStoreDataReg(cb, reg, r)
			amd64TEST_reg_reg32(cb, r, r)
		default:
			return
		}
		emitCCR_Logic(cb)
	}
}

// m68kEmitMULL emits native MULL.L Dn,Dl / Dn,Dh:Dl.
// Only direct-register source forms are owned here; other shapes stay on the interpreter path.
func m68kEmitMULL(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset
	extPC := instrPC + 2
	extWord := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
	srcMode := (ji.opcode >> 3) & 7
	srcReg := ji.opcode & 0x7
	signed := (extWord & 0x0800) != 0
	resultHigh := (extWord & 0x0400) != 0
	dlReg := (extWord >> 12) & 0x7
	dhReg := dlReg
	if resultHigh {
		dhReg = extWord & 0x7
	}

	// Full 68020 indexed source forms still belong to the interpreter path.
	if srcMode == 6 || (srcMode == 7 && srcReg == 3) {
		eaExtPC := instrPC + 4
		if eaExtPC+2 <= uint32(len(memory)) {
			eaExtWord := uint16(memory[eaExtPC])<<8 | uint16(memory[eaExtPC+1])
			if eaExtWord&0x0100 != 0 {
				amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
				m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
				m68kEmitEpilogue(cb, br)
				return
			}
		}
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	m68kEmitReadSourceEA(cb, srcMode, srcReg, M68K_SIZE_LONG, memory, instrPC+4, instrPC, amd64R11)
	mul := m68kResolveDataReg(cb, dlReg, amd64RCX)

	if signed {
		amd64MOVSXD(cb, amd64RAX, mul)
		amd64MOVSXD(cb, amd64RCX, amd64R11)
	} else {
		amd64MOV_reg_reg32(cb, amd64RAX, mul)
		amd64MOV_reg_reg32(cb, amd64RCX, amd64R11)
	}
	amd64IMUL_reg_reg(cb, amd64RAX, amd64RCX)

	amd64MOV_reg_reg32(cb, amd64R10, amd64RAX) // low 32 bits
	amd64MOV_reg_reg(cb, amd64R11, amd64RAX)   // full 64-bit product copy
	if signed {
		amd64SAR_imm(cb, amd64R11, 32)
	} else {
		amd64SHR_imm(cb, amd64R11, 32)
	}

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X) // preserve X only

	if resultHigh {
		m68kStoreDataReg(cb, dhReg, amd64R11)
		m68kStoreDataReg(cb, dlReg, amd64R10)

		amd64MOV_reg_reg32(cb, amd64RDX, amd64R10)
		amd64ALU_reg_reg32(cb, 0x09, amd64RDX, amd64R11) // OR for Z test
		amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
		skipZOff := amd64Jcc_rel32(cb, amd64CondNE)
		amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
		patchRel32(cb, skipZOff, cb.Len())

		amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
		skipNOff := amd64Jcc_rel32(cb, 0x9) // JNS
		amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
		patchRel32(cb, skipNOff, cb.Len())
	} else {
		m68kStoreDataReg(cb, dlReg, amd64R10)

		amd64TEST_reg_reg32(cb, amd64R10, amd64R10)
		skipZOff := amd64Jcc_rel32(cb, amd64CondNE)
		amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
		patchRel32(cb, skipZOff, cb.Len())

		skipNOff := amd64Jcc_rel32(cb, 0x9) // JNS
		amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
		patchRel32(cb, skipNOff, cb.Len())

		if signed {
			amd64MOVSXD(cb, amd64RDX, amd64R10)
			amd64SAR_imm(cb, amd64RDX, 32)
			amd64ALU_reg_reg32(cb, 0x39, amd64R11, amd64RDX) // CMP hi, signext(low)
			skipVOff := amd64Jcc_rel32(cb, amd64CondE)
			amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_V)
			patchRel32(cb, skipVOff, cb.Len())
		} else {
			amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
			skipVOff := amd64Jcc_rel32(cb, amd64CondE)
			amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_V)
			patchRel32(cb, skipVOff, cb.Len())
		}
	}

	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
}

// m68kEmitDIVL emits native DIVL.L Dn,Dq / Dn,Dr:Dq.
// Only direct-register source forms are owned here; other shapes stay on the interpreter path.
// Zero-divide still bails back to the interpreter so exception delivery remains exact.
func m68kEmitDIVL(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset
	extPC := instrPC + 2
	extWord := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
	srcMode := (ji.opcode >> 3) & 7
	srcReg := ji.opcode & 0x7
	signed := (extWord & 0x0800) != 0
	longDiv := (extWord & 0x0400) != 0
	dqReg := (extWord >> 12) & 0x7
	drReg := extWord & 0x7

	// Full 68020 indexed source forms still belong to the interpreter path.
	if srcMode == 6 || (srcMode == 7 && srcReg == 3) {
		eaExtPC := instrPC + 4
		if eaExtPC+2 <= uint32(len(memory)) {
			eaExtWord := uint16(memory[eaExtPC])<<8 | uint16(memory[eaExtPC+1])
			if eaExtWord&0x0100 != 0 {
				amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
				m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
				m68kEmitEpilogue(cb, br)
				return
			}
		}
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	m68kEmitReadSourceEA(cb, srcMode, srcReg, M68K_SIZE_LONG, memory, instrPC+4, instrPC, amd64R11)
	amd64MOV_reg_reg32(cb, amd64RCX, amd64R11)
	amd64TEST_reg_reg32(cb, amd64RCX, amd64RCX)
	divZeroOff := amd64Jcc_rel32(cb, amd64CondE)

	if longDiv {
		dividendHi := m68kResolveDataReg(cb, drReg, amd64R10)
		dividendLo := m68kResolveDataReg(cb, dqReg, amd64R11)

		if signed {
			amd64MOV_reg_reg32(cb, amd64RAX, dividendHi)
			amd64SHL_imm(cb, amd64RAX, 32)
			amd64MOV_reg_reg32(cb, amd64RDX, dividendLo)
			amd64ALU_reg_reg(cb, 0x09, amd64RAX, amd64RDX)
			amd64CQO(cb)
			amd64MOVSXD(cb, amd64RCX, amd64RCX)
			amd64IDIV(cb, amd64RCX)
			amd64MOVSXD(cb, amd64R10, amd64RAX)
			amd64ALU_reg_reg(cb, 0x39, amd64RAX, amd64R10)
			overflowOff := amd64Jcc_rel32(cb, amd64CondNE)

			m68kStoreDataReg(cb, dqReg, amd64RAX)
			m68kStoreDataReg(cb, drReg, amd64RDX)

			amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
			amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
			skipZOff := amd64Jcc_rel32(cb, amd64CondNE)
			amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
			patchRel32(cb, skipZOff, cb.Len())
			skipNOff := amd64Jcc_rel32(cb, 0x9) // JNS
			amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
			patchRel32(cb, skipNOff, cb.Len())
			doneOff := amd64JMP_rel32(cb)

			patchRel32(cb, overflowOff, cb.Len())
			amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
			amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_V)
			patchRel32(cb, doneOff, cb.Len())
		} else {
			amd64MOV_reg_reg32(cb, amd64RAX, dividendHi)
			amd64SHL_imm(cb, amd64RAX, 32)
			amd64MOV_reg_reg32(cb, amd64RDX, dividendLo)
			amd64ALU_reg_reg(cb, 0x09, amd64RAX, amd64RDX)
			amd64XOR_reg_reg(cb, amd64RDX, amd64RDX)
			amd64DIV(cb, amd64RCX)
			amd64MOV_reg_reg(cb, amd64R10, amd64RAX)
			amd64SHR_imm(cb, amd64R10, 32)
			overflowOff := amd64Jcc_rel32(cb, amd64CondNE)

			m68kStoreDataReg(cb, dqReg, amd64RAX)
			m68kStoreDataReg(cb, drReg, amd64RDX)

			amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
			amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
			skipZOff := amd64Jcc_rel32(cb, amd64CondNE)
			amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
			patchRel32(cb, skipZOff, cb.Len())
			doneOff := amd64JMP_rel32(cb)

			patchRel32(cb, overflowOff, cb.Len())
			amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
			amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_V)
			patchRel32(cb, doneOff, cb.Len())
		}
	} else {
		dividend := m68kResolveDataReg(cb, dqReg, amd64R10)

		if signed {
			amd64MOVSXD(cb, amd64RAX, dividend)
			amd64CQO(cb)
			amd64MOVSXD(cb, amd64RCX, amd64RCX)
			amd64IDIV(cb, amd64RCX)
			amd64MOVSXD(cb, amd64R10, amd64RAX)
			amd64ALU_reg_reg(cb, 0x39, amd64RAX, amd64R10)
			overflowOff := amd64Jcc_rel32(cb, amd64CondNE)

			m68kStoreDataReg(cb, dqReg, amd64RAX)
			if drReg != dqReg {
				m68kStoreDataReg(cb, drReg, amd64RDX)
			}

			amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
			amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
			skipZOff := amd64Jcc_rel32(cb, amd64CondNE)
			amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
			patchRel32(cb, skipZOff, cb.Len())
			skipNOff := amd64Jcc_rel32(cb, 0x9) // JNS
			amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
			patchRel32(cb, skipNOff, cb.Len())
			doneOff := amd64JMP_rel32(cb)

			patchRel32(cb, overflowOff, cb.Len())
			amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
			amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_V)
			patchRel32(cb, doneOff, cb.Len())
		} else {
			amd64MOV_reg_reg32(cb, amd64RAX, dividend)
			amd64XOR_reg_reg(cb, amd64RDX, amd64RDX)
			amd64DIV(cb, amd64RCX)

			m68kStoreDataReg(cb, dqReg, amd64RAX)
			if drReg != dqReg {
				m68kStoreDataReg(cb, drReg, amd64RDX)
			}

			amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
			amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
			skipZOff := amd64Jcc_rel32(cb, amd64CondNE)
			amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
			patchRel32(cb, skipZOff, cb.Len())
		}
	}

	finalDoneOff := amd64JMP_rel32(cb)

	// Zero-divide re-executes through the interpreter for exact exception handling.
	patchRel32(cb, divZeroOff, cb.Len())
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
	patchRel32(cb, finalDoneOff, cb.Len())

	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
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
		skipIncOff := 0
		if m68kEAMayUseMemHelper(3, reg, false) {
			amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
			skipIncOff = amd64Jcc_rel32(cb, amd64CondNE)
		}
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(step)) // ADD An, step
		m68kStoreAddrReg(cb, reg, ar)
		if skipIncOff != 0 {
			patchRel32(cb, skipIncOff, cb.Len())
		}
		return 0

	case 4: // -(An) — pre-decrement
		step := m68kStepSize(size, reg)
		ar := m68kResolveAddrReg(cb, reg, amd64R10)
		amd64ALU_reg_imm32_32bit(cb, 5, ar, int32(step)) // SUB An, step
		amd64MOV_reg_reg32(cb, amd64R10, ar)             // address in R10
		bail := 0
		m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
		skipStoreOff := 0
		if m68kEAMayUseMemHelper(4, reg, false) {
			amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
			skipStoreOff = amd64Jcc_rel32(cb, amd64CondNE)
		}
		m68kStoreAddrReg(cb, reg, ar)
		if skipStoreOff != 0 {
			patchRel32(cb, skipStoreOff, cb.Len())
		}
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

		// Index register must come from the current live register mapping when resident.
		if idxIsAddr == 1 {
			idx := m68kResolveAddrReg(cb, idxReg&7, amd64RCX)
			if idx != amd64RCX {
				amd64MOV_reg_reg32(cb, amd64RCX, idx)
			}
		} else {
			idx := m68kResolveDataReg(cb, idxReg&7, amd64RCX)
			if idx != amd64RCX {
				amd64MOV_reg_reg32(cb, amd64RCX, idx)
			}
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
				idx := m68kResolveAddrReg(cb, idxReg&7, amd64RCX)
				if idx != amd64RCX {
					amd64MOV_reg_reg32(cb, amd64RCX, idx)
				}
			} else {
				idx := m68kResolveDataReg(cb, idxReg&7, amd64RCX)
				if idx != amd64RCX {
					amd64MOV_reg_reg32(cb, amd64RCX, idx)
				}
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
		skipIncOff := 0
		if m68kEAMayUseMemHelper(3, reg, true) {
			amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
			skipIncOff = amd64Jcc_rel32(cb, amd64CondNE)
		}
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(step))
		m68kStoreAddrReg(cb, reg, ar)
		if skipIncOff != 0 {
			patchRel32(cb, skipIncOff, cb.Len())
		}
		return 0

	case 4: // -(An)
		step := m68kStepSize(size, reg)
		ar := m68kResolveAddrReg(cb, reg, amd64R10)
		amd64ALU_reg_imm32_32bit(cb, 5, ar, int32(step))
		amd64MOV_reg_reg32(cb, amd64R10, ar)
		bail := 0
		m68kEmitMemWrite(cb, amd64R10, valReg, size, &bail)
		skipStoreOff := 0
		if m68kEAMayUseMemHelper(4, reg, true) {
			amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
			skipStoreOff = amd64Jcc_rel32(cb, amd64CondNE)
		}
		m68kStoreAddrReg(cb, reg, ar)
		if skipStoreOff != 0 {
			patchRel32(cb, skipStoreOff, cb.Len())
		}
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

	case 6: // (d8,An,Xn) brief/full format
		extBytes := m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, 0, amd64R10)
		bail := 0
		m68kEmitMemWrite(cb, amd64R10, valReg, size, &bail)
		return extBytes

	case 7:
		// Absolute destinations use mode 7/reg 0 (.W) and 1 (.L).
		// Other mode-7 forms are invalid as write destinations for MOVE.
		if reg <= 1 {
			extBytes := m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, 0, amd64R10)
			bail := 0
			m68kEmitMemWrite(cb, amd64R10, valReg, size, &bail)
			return extBytes
		}
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
	// Lazy CCR: if EFLAGS is live, use direct Jcc mapping (no R14 bit tests needed)
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		jcc := m68kCondToDirectJcc(m68kCond)
		// Consuming flags: after the branch, flags are still live (Jcc doesn't modify EFLAGS)
		// Don't change flagState here — the caller may need it for the fall-through path
		return jcc
	}
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

// m68kBranchBasePC returns the PC used as the branch base for BRA/BSR/Bcc.
// Branch displacements are relative to instrPC+2. Word/long forms still use
// the same base; the extension words only carry the displacement itself.
func m68kBranchBasePC(instrPC uint32, opcode uint16) uint32 {
	disp8 := opcode & 0xFF
	switch disp8 {
	case 0x00:
		return instrPC + 2
	case 0xFF:
		return instrPC + 2
	default:
		return instrPC + 2
	}
}

// m68kEmitBRA emits BRA (unconditional branch, block terminator).
// Uses chain exit for direct block-to-block chaining.
func m68kEmitBRA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, chainSlots *[]m68kChainExitInfo) {
	instrPC := startPC + ji.pcOffset
	disp := m68kReadBranchDisp(memory, instrPC, ji.opcode)
	targetPC := uint32(int64(m68kBranchBasePC(instrPC, ji.opcode)) + int64(disp))

	info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), br)
	if chainSlots != nil {
		*chainSlots = append(*chainSlots, info)
	}
}

// m68kEmitBSR emits BSR (branch to subroutine, block terminator).
// Pushes return address to stack, then uses chain exit to target.
func m68kEmitBSR(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, chainSlots *[]m68kChainExitInfo) {
	// BSR clobbers EFLAGS (CMP for I/O check, SUB for A7).
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}
	instrPC := startPC + ji.pcOffset
	disp := m68kReadBranchDisp(memory, instrPC, ji.opcode)
	targetPC := uint32(int64(m68kBranchBasePC(instrPC, ji.opcode)) + int64(disp))
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

	// Chain exit to target
	info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), br)
	if chainSlots != nil {
		*chainSlots = append(*chainSlots, info)
	}

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
	instrs []M68KJITInstr, chainSlots *[]m68kChainExitInfo) {

	instrPC := startPC + ji.pcOffset
	cond := (ji.opcode >> 8) & 0xF

	if memory == nil {
		return
	}

	disp := m68kReadBranchDisp(memory, instrPC, ji.opcode)
	targetPC := uint32(int64(m68kBranchBasePC(instrPC, ji.opcode)) + int64(disp))

	// Evaluate M68K condition → returns Jcc condition code
	jccCond := m68kCondToJcc(cb, cond)

	if jccCond == 0xFF {
		// Always-taken condition: chain exit
		info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), br)
		if chainSlots != nil {
			*chainSlots = append(*chainSlots, info)
		}
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
			bccInfo := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), br)
			if chainSlots != nil {
				*chainSlots = append(*chainSlots, bccInfo)
			}

			// Not-taken path
			patchRel32(cb, skipOff, cb.Len())
			return
		}
	}

	// Exit-block path (forward branch or can't resolve target)
	invertedCond := jccCond ^ 1
	skipOff := amd64Jcc_rel32(cb, invertedCond)

	bccInfo := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), br)
	if chainSlots != nil {
		*chainSlots = append(*chainSlots, bccInfo)
	}

	patchRel32(cb, skipOff, cb.Len())
}

// m68kEmitRTS emits RTS (return from subroutine, block terminator).
// Includes RTS inline cache check for fast chained returns.
func m68kEmitRTS(cb *CodeBuffer, ji *M68KJITInstr, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset

	// RTS clobbers EFLAGS internally (CMP for I/O check, ADD for A7, CMP for cache check).
	// Materialize lazy CCR into R14 before we clobber EFLAGS.
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

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

	// Correctness-first path: always perform an unchained RTS exit.
	// The inline RTS cache can be re-enabled once nested subroutine paths
	// are covered by focused regressions.
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), amd64RAX)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
	m68kEmitEpilogue(cb, br)

	// Bail path: set NeedIOFallback + RetPC so dispatcher re-executes RTS via interpreter
	patchRel32(cb, bailOff, cb.Len())
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
}

// m68kEmitJSR emits JSR <ea> (jump to subroutine, block terminator).
// Handles JSR (An) and JSR (abs). Uses chain exit for static targets.
func m68kEmitJSR(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, chainSlots *[]m68kChainExitInfo) {
	// JSR clobbers EFLAGS (CMP for I/O check, SUB/ADD for A7).
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}
	instrPC := startPC + ji.pcOffset
	returnPC := instrPC + uint32(ji.length) // PC after JSR
	mode := (ji.opcode >> 3) & 7
	reg := ji.opcode & 7

	// Determine if target is statically known (for chain exit)
	var targetAddr uint32
	staticTarget := false

	switch mode {
	case 2: // (An) — dynamic
	case 7:
		switch reg {
		case 1: // abs.L
			if instrPC+6 <= uint32(len(memory)) {
				targetAddr = uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
					uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5])
				staticTarget = true
			}
		case 0: // abs.W
			if instrPC+4 <= uint32(len(memory)) {
				w := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
				targetAddr = uint32(int32(int16(w)))
				staticTarget = true
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

	// For dynamic targets, compute into R10
	if !staticTarget {
		amd64MOV_reg_reg32(cb, amd64R10, m68kResolveAddrReg(cb, reg, amd64RDX))
	}

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

	if staticTarget {
		// Chain exit to known target
		info := m68kEmitChainExit(cb, targetAddr, uint32(instrIdx+1), br)
		if chainSlots != nil {
			*chainSlots = append(*chainSlots, info)
		}
	} else {
		// Dynamic target: normal unchained exit
		amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), amd64R10)
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
		m68kEmitEpilogue(cb, br)
	}

	// Bail path
	patchRel32(cb, bailOff, cb.Len())
	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4) // undo SUB
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
}

// m68kEmitJMP emits JMP <ea> (unconditional jump, block terminator).
// Uses chain exit for statically-known targets (abs.L, abs.W).
func m68kEmitJMP(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, chainSlots *[]m68kChainExitInfo) {
	instrPC := startPC + ji.pcOffset
	mode := (ji.opcode >> 3) & 7
	reg := ji.opcode & 7

	switch mode {
	case 2: // (An) — dynamic target, cannot chain
		r := m68kResolveAddrReg(cb, reg, amd64RAX)
		amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), r)
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
		m68kEmitEpilogue(cb, br)
		return
	case 7:
		switch reg {
		case 1: // abs.L — static target, chain
			if instrPC+6 <= uint32(len(memory)) {
				addr := uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
					uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5])
				info := m68kEmitChainExit(cb, addr, uint32(instrIdx+1), br)
				if chainSlots != nil {
					*chainSlots = append(*chainSlots, info)
				}
				return
			}
		case 0: // abs.W — static target, chain
			if instrPC+4 <= uint32(len(memory)) {
				w := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
				addr := uint32(int32(int16(w)))
				info := m68kEmitChainExit(cb, addr, uint32(instrIdx+1), br)
				if chainSlots != nil {
					*chainSlots = append(*chainSlots, info)
				}
				return
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

	// Fallback for cases where memory bounds check failed
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
	tmp := src
	if tmp == amd64RDX {
		amd64MOV_reg_reg32(cb, amd64RCX, tmp)
		tmp = amd64RCX
	}
	amd64MOV_reg_mem32(cb, amd64RDX, m68kAMD64RegDataBase, int32(dreg)*4)
	amd64MOVZX_B(cb, tmp, tmp)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, -256) // AND r, 0xFFFFFF00
	amd64ALU_reg_reg32(cb, 0x09, amd64RDX, tmp)
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
	br *m68kBlockRegs, instrIdx int, instrOffsets []int, instrs []M68KJITInstr, chainSlots *[]m68kChainExitInfo) {

	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	cond := (opcode >> 8) & 0xF
	reg := opcode & 7

	if memory == nil || instrPC+4 > uint32(len(memory)) {
		return
	}

	// DBcc clobbers EFLAGS internally (SUB for decrement, CMP for exhaustion check).
	// Materialize lazy CCR into R14 before we clobber EFLAGS.
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
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

			// Budget exceeded → chain exit to target
			patchRel32(cb, budgetExitOff, cb.Len())
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), amd64RAX)
			info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), br)
			if chainSlots != nil {
				*chainSlots = append(*chainSlots, info)
			}

			// Exhausted → fall through
			patchRel32(cb, exhaustedOff, cb.Len())
			return
		}
	}

	// Fallback: chain exit to target PC (external branch)
	info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), br)
	if chainSlots != nil {
		*chainSlots = append(*chainSlots, info)
	}

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

	// Lazy CCR: CMP sets NZVC but NOT X — use flagsLiveArithNoX
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsLiveArithNoX
		return
	}

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
			idx := m68kResolveAddrReg(cb, idxReg&7, amd64RCX)
			if idx != amd64RCX {
				amd64MOV_reg_reg32(cb, amd64RCX, idx)
			}
		} else {
			idx := m68kResolveDataReg(cb, idxReg&7, amd64RCX)
			if idx != amd64RCX {
				amd64MOV_reg_reg32(cb, amd64RCX, idx)
			}
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
				idx := m68kResolveAddrReg(cb, idxReg&7, amd64RCX)
				if idx != amd64RCX {
					amd64MOV_reg_reg32(cb, amd64RCX, idx)
				}
			} else {
				idx := m68kResolveDataReg(cb, idxReg&7, amd64RCX)
				if idx != amd64RCX {
					amd64MOV_reg_reg32(cb, amd64RCX, idx)
				}
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

	// Emit chain entry point (lightweight entry for chained transitions)
	chainEntryOff := m68kEmitChainEntry(cb, &br)

	instrOffsets := make([]int, len(instrs))
	var chainExits []m68kChainExitInfo

	// Enable lazy CCR tracking for this block
	var cs m68kCompileState
	m68kCurrentCS = &cs

	for i := range instrs {
		// Materialize CCR before non-flag instructions that clobber EFLAGS
		if cs.flagState != flagsMaterialized {
			if m68kInstrNeedsCCRMaterialization(&instrs[i]) {
				m68kMaterializeCCR(cb, &cs)
			}
		}
		instrOffsets[i] = cb.Len()
		ji := &instrs[i]
		m68kEmitInstructionFull(cb, ji, startPC, &br, i, len(instrs), memory, instrOffsets, instrs, &chainExits)

		// Memory helpers raise NeedIOFallback on MMIO/alignment bails. Exit the
		// block immediately after instructions that use the generic helper path
		// so the dispatcher can re-execute them via the interpreter instead of
		// continuing natively.
		if !m68kIsBlockTerminator(ji.opcode) && m68kInstrMaySetGenericIOFallback(ji) {
			instrPC := startPC + ji.pcOffset
			if cs.flagState != flagsMaterialized {
				m68kMaterializeCCR(cb, &cs)
			}
			amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
			noIOBailOff := amd64Jcc_rel32(cb, amd64CondE)
			m68kEmitRetPC(cb, instrPC, uint32(i))
			m68kEmitEpilogue(cb, &br)
			patchRel32(cb, noIOBailOff, cb.Len())
		}
	}

	// If the last instruction doesn't have its own epilogue, emit fallthrough
	// Note: m68kCurrentCS must still be active so epilogue can materialize CCR
	lastOp := instrs[len(instrs)-1].opcode
	if !m68kIsBlockTerminator(lastOp) {
		lastInstr := &instrs[len(instrs)-1]
		endPC := startPC + lastInstr.pcOffset + uint32(lastInstr.length)
		m68kEmitRetPC(cb, endPC, uint32(len(instrs)))
		m68kEmitEpilogue(cb, &br)
	}

	m68kCurrentCS = nil

	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, err
	}

	lastInstr := &instrs[len(instrs)-1]
	endPC := startPC + lastInstr.pcOffset + uint32(lastInstr.length)

	// Convert code buffer offsets to absolute ExecMem addresses
	chainEntry := addr + uintptr(chainEntryOff)
	var slots []chainSlot
	for _, ce := range chainExits {
		slots = append(slots, chainSlot{
			targetPC:  ce.targetPC,
			patchAddr: addr + uintptr(ce.jmpDispOffset),
		})
	}

	return &JITBlock{
		startPC:    startPC,
		endPC:      endPC,
		instrCount: len(instrs),
		execAddr:   addr,
		execSize:   len(code),
		chainEntry: chainEntry,
		chainSlots: slots,
	}, nil
}

// m68kEmitInstructionFull dispatches to the appropriate emitter, with full context.
func m68kEmitInstructionFull(cb *CodeBuffer, ji *M68KJITInstr, blockStartPC uint32, br *m68kBlockRegs, instrIdx int, blockLen int, memory []byte, instrOffsets []int, instrs []M68KJITInstr, chainSlots *[]m68kChainExitInfo) {
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
		if opmode == 2 { // ADD.L <ea>,Dn
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
		if opmode == 2 { // SUB.L <ea>,Dn
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
		if opmode == 2 { // CMP.L <ea>,Dn
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
		// MULL/DIVL (68020)
		if opcode&0xFF80 == 0x4C00 {
			// Native coverage currently owns direct-register MULL.L and DIVL.L only.
			// Memory/immediate forms still bail to the interpreter.
			if memory != nil {
				if opcode&0x0040 == 0 {
					m68kEmitMULL(cb, ji, memory, blockStartPC, br, instrIdx)
				} else {
					m68kEmitDIVL(cb, ji, memory, blockStartPC, br, instrIdx)
				}
				return
			}
		}
		// CLR
		if opcode&0xFF00 == 0x4200 {
			m68kEmitCLR(cb, opcode)
			return
		}
		// NOT
		if opcode&0xFF00 == 0x4600 {
			// Native coverage currently owns direct-register byte/word/long NOT.
			// Memory forms still bail to the interpreter.
			if ((opcode>>3)&7) == 0 && ((opcode>>6)&3) != 3 {
				m68kEmitNOT(cb, opcode)
				return
			}
		}
		// NEG
		if opcode&0xFF00 == 0x4400 {
			m68kEmitNEG(cb, opcode)
			return
		}
		// TST
		if opcode&0xFF00 == 0x4A00 {
			if (opcode>>6)&3 == 2 { // TST.L only; byte/word fall back for correct narrow semantics
				m68kEmitTST(cb, opcode)
				return
			}
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
			m68kEmitRTS(cb, ji, blockStartPC, br, instrIdx)
			return
		}
		// JSR
		if opcode&0xFFC0 == 0x4E80 {
			m68kEmitJSR(cb, ji, memory, blockStartPC, br, instrIdx, chainSlots)
			return
		}
		// JMP
		if opcode&0xFFC0 == 0x4EC0 {
			m68kEmitJMP(cb, ji, memory, blockStartPC, br, instrIdx, chainSlots)
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
			m68kEmitBRA(cb, ji, memory, blockStartPC, br, instrIdx, chainSlots)
			return
		}
		if cond == 1 { // BSR
			m68kEmitBSR(cb, ji, memory, blockStartPC, br, instrIdx, chainSlots)
			return
		}
		// Bcc (cond >= 2) — conditional branch, NOT a terminator
		m68kEmitBcc(cb, ji, memory, blockStartPC, br, instrIdx, instrOffsets, blockLen, instrs, chainSlots)
		return

	case 0x5: // ADDQ/SUBQ/Scc/DBcc
		// DBcc
		if opcode&0xF0F8 == 0x50C8 {
			m68kEmitDBcc(cb, ji, memory, blockStartPC, br, instrIdx, instrOffsets, instrs, chainSlots)
			return
		}
		// Scc
		if opcode&0x00C0 == 0x00C0 && opcode&0xF0F8 != 0x50F8 {
			m68kEmitScc(cb, opcode)
			return
		}
		// ADDQ/SUBQ (size field != 3)
		if opcode&0x00C0 != 0x00C0 {
			size := (opcode >> 6) & 0x3
			mode := (opcode >> 3) & 0x7
			// JIT-safe today:
			// - ADDQ/SUBQ An (size encoding ignored by ISA, no flags)
			// - ADDQ.L/SUBQ.L Dn
			if mode == 1 || (mode == 0 && size == 2) {
				if opcode&0x0100 == 0 { // ADDQ
					m68kEmitADDQ(cb, opcode)
				} else { // SUBQ
					m68kEmitSUBQ(cb, opcode)
				}
				return
			}

			// Unsupported width or EA form: re-execute via interpreter.
			instrPC := blockStartPC + ji.pcOffset
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
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
		// Register/immediate shifts.
		// Only the long-sized register form is JIT-safe today; byte/word forms
		// have different width semantics and fall back to the interpreter.
		if (opcode>>6)&3 == 2 {
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
