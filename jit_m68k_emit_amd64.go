// jit_m68k_emit_amd64.go - x86-64 native code emitter for M68020 JIT compiler

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"errors"
	"os"
	"strconv"
	"strings"
)

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

	// Stack frame: 6 callee-saved pushes (48 bytes) + SUB RSP,56 reserves
	// shared scratch slots through [RSP+52] and preserves 16-byte alignment.
	m68kAMD64FrameSize    = 56
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
//
// As of Phase 2 of the JIT unification, m68kFlagState is a type alias for the
// shared JITFlagState enum (see jit_flags_common.go). The legacy backend
// constants (flagsMaterialized, flagsLiveArith, flagsLiveLogi) remain valid
// names in this file as untyped aliases for the shared values, so existing
// emit-site references keep compiling unchanged. M68K's CMP-style state
// (flagsLiveArithNoX) is a backend-specific extension declared at
// JITFlagBackendBase + 0, and the interpreter-parity NZ-only logical state
// (flagsLiveLogiPreserveVC) is declared at JITFlagBackendBase + 1 — neither
// is part of the shared core enum.
type m68kFlagState = JITFlagState

const (
	flagsMaterialized       = JITFlagMaterialized    // R14 holds valid 5-bit CCR
	flagsLiveArith          = JITFlagLiveArith       // EFLAGS from ADD/SUB/NEG; X saved to [RSP+xFlagOff]
	flagsLiveArithNoX       = JITFlagLiveArithNoX    // EFLAGS from CMP (X not modified) — M68K backend extension
	flagsLiveLogi           = JITFlagLiveLogic       // EFLAGS from MOVE/TST/CLR-style logic (V=0,C=0; X in stack slot)
	flagsLiveLogiPreserveVC = JITFlagBackendBase + 1 // EFLAGS from AND/OR/EOR; preserve X,V,C like the interpreter's SetFlagsNZ.
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
	case 0x0:
		if opcode&0xF138 == 0x0108 { // MOVEP
			return true
		}
		if m68kIsBTSTDynamic(opcode) || m68kIsBTSTImmediate(opcode) {
			return true
		}
		if opcode&0xF1C0 == 0x0140 || opcode&0xF1C0 == 0x0180 || opcode&0xF1C0 == 0x01C0 {
			return true
		}
		if m68kIsImmediateLogicDn(opcode) {
			return true
		}
	case 0x1, 0x2, 0x3: // MOVE.B / MOVE.L / MOVE.W
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		// MOVE's memory access happens before MOVE's own CCR update. Any
		// MMIO/helper/debug callback on that access must therefore see the
		// previous instruction's architectural CCR, not stale lazy host flags
		// after EA/range-check code has clobbered EFLAGS. MOVEA also writes no
		// CCR, so it needs the same protection for ordinary EA computation.
		if m68kEAMayUseMemHelper(srcMode, srcReg, false) || m68kEAMayUseMemHelper(dstMode, dstReg, true) ||
			srcMode >= 2 || dstMode >= 2 {
			return true
		}
	case 0x4: // Misc group
		// RTE — clobbers EFLAGS via supervisor/range/frame checks.
		if opcode == 0x4E73 {
			return true
		}
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
		// MOVE USP
		if opcode&0xFFF0 == 0x4E60 {
			return true
		}
		// CHK updates only N in the current interpreter and uses host
		// comparisons internally, so the prior X/Z/V/C state must be real.
		if opcode&0xF1C0 == 0x4180 || opcode&0xF1C0 == 0x4100 {
			return true
		}
		// NOP — does NOT clobber EFLAGS
		if opcode == 0x4E71 {
			return false
		}
		// NOT preserves V/C in the interpreter, so materialize any
		// pending lazy V/C before it clobbers host flags.
		if opcode&0xFF00 == 0x4600 {
			return true
		}
		// CLR preserves X while clearing NZVC, so any pending lazy X must
		// be materialized before R14 is edited directly.
		if opcode&0xFF00 == 0x4200 {
			return true
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
	case 0x8: // OR preserves V/C in the interpreter.
		opmode := (opcode >> 6) & 7
		if opmode <= 2 || (opmode >= 4 && opmode <= 6) {
			return true
		}
	case 0xB: // EOR preserves V/C in the interpreter.
		opmode := (opcode >> 6) & 7
		if opmode >= 4 && opmode <= 6 {
			return true
		}
	case 0xC: // AND preserves V/C in the interpreter.
		opmode := (opcode >> 6) & 7
		if opmode <= 2 || (opmode >= 4 && opmode <= 6) {
			return true
		}
	case 0xF: // FPU. Native reg-to-reg ops clobber host EFLAGS computing FPSR
		// condition codes, and the helper path exits the block — both require
		// any lazily-live integer CCR to be materialized into R14 first so a
		// following Bcc reads the correct flags.
		return true
	}
	return false
}

// m68kCurrentCS is the compile state for the currently-compiling block.
// Set by m68kCompileBlockWithMem; read by emitCCR_Arithmetic/Logic and m68kCondToJcc.
// Safe: M68K JIT compilation is single-threaded.
var m68kCurrentCS *m68kCompileState

func m68kInstrMaySetGenericIOFallback(ji *M68KJITInstr) bool {
	opcode := ji.opcode
	group := opcode >> 12

	switch group {
	case 0x0: // CMPI
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		return opcode&0xFF00 == 0x0C00 && (opcode>>6)&3 != 3 &&
			m68kEAMayUseMemHelper(srcMode, srcReg, false)
	case 0x1, 0x2, 0x3: // MOVE
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		return m68kEAMayUseMemHelper(srcMode, srcReg, false) || m68kEAMayUseMemHelper(dstMode, dstReg, true)
	case 0x9, 0xB, 0xD: // SUB/CMP/ADD EA,Dn paths
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		opmode := (opcode >> 6) & 7
		if group == 0xB {
			return opmode <= 2 && m68kEAMayUseMemHelper(srcMode, srcReg, false)
		}
		if group == 0x9 || group == 0xD {
			return opmode <= 2 && m68kEAMayUseMemHelper(srcMode, srcReg, false)
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

	case flagsLiveLogiPreserveVC:
		// N and Z from EFLAGS; preserve X,V,C from materialized R14.
		amd64SETcc(cb, amd64CondE, amd64RCX) // CL = Z
		amd64SETcc(cb, 0x8, amd64RDX)        // DL = N

		amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X|m68kCCR_V|m68kCCR_C)
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
	emitREXForByte(cb, 0, dst)
	cb.EmitBytes(0x0F, 0x90+cond, modRM(3, 0, dst))
}

// amd64TEST_reg_imm8 emits TEST r/m8, imm8 (for single register).
func amd64TEST_reg_imm8(cb *CodeBuffer, reg byte, imm8 byte) {
	if reg == amd64RAX {
		cb.EmitBytes(0xA8, imm8) // short form: TEST AL, imm8
	} else {
		emitREXForByte(cb, 0, reg)
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
		// Phase 2c liveness: dead producer skips the lazy-flag dance.
		// Compile loop pre-materialises any live prior state before
		// this slot's arithmetic clobbers host EFLAGS, so cs.flagState
		// is already flagsMaterialized here.
		if m68kCCRDeadAtCurrent() {
			return
		}
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
		if m68kCCRDeadAtCurrent() {
			return
		}
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

// emitCCR_LogicPreserveVC matches the current interpreter's AND/OR/EOR
// behavior: N/Z are updated from the result, while X/V/C are preserved.
// The compile loop materializes any pending lazy CCR before these opcodes so
// R14 contains the V/C bits being preserved.
func emitCCR_LogicPreserveVC(cb *CodeBuffer) {
	if cs := m68kCurrentCS; cs != nil {
		if m68kCCRDeadAtCurrent() {
			return
		}
		cs.flagState = flagsLiveLogiPreserveVC
		return
	}
	amd64SETcc(cb, amd64CondE, amd64RCX)
	amd64SETcc(cb, 0x8, amd64RDX)

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X|m68kCCR_V|m68kCCR_C)
	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
}

func m68kEmitCCRArithmeticMaterialized(cb *CodeBuffer) {
	amd64SETcc(cb, amd64CondB, amd64RCX) // C
	amd64SETcc(cb, amd64CondO, amd64RDX) // V
	amd64SETcc(cb, amd64CondE, amd64R10) // Z
	amd64SETcc(cb, 0x8, amd64R11)        // N

	amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX)

	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 1)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOVZX_B(cb, amd64RAX, amd64R10)
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
}

func m68kEmitSizedLogicTest(cb *CodeBuffer, valueReg byte, size int, scratch byte) {
	switch size {
	case M68K_SIZE_BYTE:
		if scratch == valueReg {
			scratch = amd64RAX
			if scratch == valueReg {
				scratch = amd64RCX
			}
		}
		amd64MOV_reg_reg32(cb, scratch, valueReg)
		amd64MOVZX_B(cb, scratch, scratch)
		amd64SHL_imm32(cb, scratch, 24)
		amd64TEST_reg_reg32(cb, scratch, scratch)
	case M68K_SIZE_WORD:
		if scratch == valueReg {
			scratch = amd64RAX
			if scratch == valueReg {
				scratch = amd64RCX
			}
		}
		amd64MOV_reg_reg32(cb, scratch, valueReg)
		amd64MOVZX_W(cb, scratch, scratch)
		amd64SHL_imm32(cb, scratch, 16)
		amd64TEST_reg_reg32(cb, scratch, scratch)
	default:
		amd64TEST_reg_reg32(cb, valueReg, valueReg)
	}
}

func m68kEmitSizedALURegReg(cb *CodeBuffer, op8, opWide byte, dst, src byte, size int) {
	switch size {
	case M68K_SIZE_BYTE:
		emitREXForByte(cb, src, dst)
		cb.EmitBytes(op8, modRM(3, src, dst))
	case M68K_SIZE_WORD:
		cb.EmitBytes(0x66)
		emitREX(cb, false, src, dst)
		cb.EmitBytes(opWide, modRM(3, src, dst))
	default:
		amd64ALU_reg_reg32(cb, opWide, dst, src)
	}
}

func m68kEmitSizedALURegImm(cb *CodeBuffer, aluOp byte, dst byte, imm uint16, size int) {
	switch size {
	case M68K_SIZE_BYTE:
		emitREXForByte(cb, aluOp, dst)
		cb.EmitBytes(0x80, modRM(3, aluOp, dst), byte(imm))
	case M68K_SIZE_WORD:
		cb.EmitBytes(0x66)
		emitREX(cb, false, 0, dst)
		cb.EmitBytes(0x83, modRM(3, aluOp, dst), byte(int8(imm)))
	default:
		amd64ALU_reg_imm32_32bit(cb, aluOp, dst, int32(imm))
	}
}

func m68kEmitCCRLogicWithSavedCarry(cb *CodeBuffer, carryReg byte, valueReg byte, size int) {
	amd64SETcc(cb, amd64CondE, amd64RCX) // Z
	amd64SETcc(cb, 0x8, amd64RDX)        // N

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X) // preserve X only
	amd64MOVZX_B(cb, amd64RAX, carryReg)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
	_ = valueReg
	_ = size
}

func m68kEmitCCRShiftWithCarryOverflow(cb *CodeBuffer, carryReg byte, overflowReg byte, resultReg byte, size int) {
	amd64SETcc(cb, amd64CondE, amd64RCX) // Z
	amd64SETcc(cb, 0x8, amd64RDX)        // N

	amd64MOVZX_B(cb, m68kAMD64RegCCR, carryReg) // C

	amd64MOVZX_B(cb, amd64RAX, carryReg)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // X = C

	// Persist X into the lazy-CCR stack slot too. Shifts set X = C, but later
	// instructions (e.g. a following CMP) rebuild R14 and re-seed X from
	// [RSP+OffXFlag]; without this store they would read a stale X and drop the
	// shift's X across the block, diverging from the interpreter.
	amd64MOVZX_B(cb, amd64RAX, carryReg)
	emitMemOp(cb, false, 0x88, amd64RAX, amd64RSP, m68kAMD64OffXFlag) // MOV [RSP+OffXFlag], AL

	amd64MOVZX_B(cb, amd64RAX, overflowReg)
	amd64SHL_imm(cb, amd64RAX, 1)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
	_ = resultReg
	_ = size
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

	// Allocate stack frame for shared block scratch slots.
	amd64ALU_reg_imm32(cb, 5, amd64RSP, int32(m68kAMD64FrameSize)) // SUB RSP, frameSize

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

// m68kCurrentInstrCountBase is the per-region cumulative instruction
// count of all blocks emitted before the currently-active block.
// m68kEmitRetPC adds this base to the per-block `count` argument so
// every exit path inside a region (IO bail, RTS-cache miss, dynamic
// JMP/JSR exit) reports the cumulative instructions retired across
// the whole region — not just the current block. Default 0 leaves
// per-block compilation unchanged. m68kCompileRegion sets it before
// each block emit and clears after the region completes.
var m68kCurrentInstrCountBase uint32

// m68kEmitRetPC writes RetPC and RetCount to the JITContext before epilogue.
func m68kEmitRetPC(cb *CodeBuffer, pc uint32, count uint32) {
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), pc)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), m68kCurrentInstrCountBase+count)
}

func m68kEmitRetPCWithLoopCount(cb *CodeBuffer, pc uint32, count uint32) {
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), pc)
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(m68kCurrentInstrCountBase+count))
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), amd64RAX)
}

func m68kInstrCoveredRange(startPC uint32, instrs []M68KJITInstr) (uint32, uint32, bool) {
	endPC := startPC
	found := false
	for i := range instrs {
		if instrs[i].fusedFlag&(m68kFusedJSRLeafCall|m68kFusedRTSLeafReturn) != 0 {
			continue
		}
		instrEnd := startPC + instrs[i].pcOffset + uint32(instrs[i].length)
		if !found || instrEnd > endPC {
			endPC = instrEnd
		}
		found = true
	}
	if !found {
		return startPC, startPC, false
	}
	return startPC, endPC, true
}

func m68kEmitGuestByteStampGuard(cb *CodeBuffer, memory []byte, ranges [][2]uint32, retPC uint32, br *m68kBlockRegs) {
	if len(memory) == 0 || len(ranges) == 0 {
		return
	}

	var mismatchJumps []int
	for _, r := range ranges {
		start, end := r[0], r[1]
		if end <= start || uint64(end) > uint64(len(memory)) {
			continue
		}
		// The direct [MemBase+disp32] form sign-extends disp32. Ranges above
		// 2 GiB need a different address materialisation path, so leave those
		// to the dispatcher hash check instead of emitting an invalid guard.
		if end > uint32(1<<31-1) {
			continue
		}
		for addr := start; addr < end; {
			remaining := end - addr
			if remaining >= 4 {
				amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegMemBase, int32(addr))
				want := binary.LittleEndian.Uint32(memory[addr : addr+4])
				amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, int32(want))
				mismatchJumps = append(mismatchJumps, amd64Jcc_rel32(cb, amd64CondNE))
				addr += 4
				continue
			}
			amd64MOVZX_B_mem(cb, amd64RAX, m68kAMD64RegMemBase, int32(addr))
			amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, int32(memory[addr]))
			mismatchJumps = append(mismatchJumps, amd64Jcc_rel32(cb, amd64CondNE))
			addr++
		}
	}
	if len(mismatchJumps) == 0 {
		return
	}

	okJump := amd64JMP_rel32(cb)
	mismatchLabel := cb.Len()
	for _, off := range mismatchJumps {
		patchRel32(cb, off, mismatchLabel)
	}
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffInvalAddr), retPC)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffInvalSize), 0)
	if len(ranges) == 1 && ranges[0][1] >= ranges[0][0] {
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffInvalSize), ranges[0][1]-ranges[0][0])
	}
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedInval), 1)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), retPC)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), 0)
	m68kEmitEpilogue(cb, br)
	patchRel32(cb, okJump, cb.Len())
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
	// Prologue-path register loads. Chained jumps from another block bypass
	// these (their patched JMP lands at entryOff below) because the caller
	// block left D0/D1/A0/A7 live in the same global host registers and
	// already materialized R14 with the canonical CCR via its chain exit.
	// X-flag stack slot is in sync with R14 bit 4 by construction:
	// m68kMaterializeCCR derives R14 bit 4 from [RSP+m68kAMD64OffXFlag]
	// (or from CF in the live-arith case, which the corresponding
	// m68kSaveXToStack writes into the slot before any clobber), so the
	// slot already reflects R14 bit 4 across the chain edge.
	amd64MOV_reg_mem32(cb, m68kAMD64RegD0, m68kAMD64RegDataBase, 0*4) // D0 -> RBX
	amd64MOV_reg_mem32(cb, m68kAMD64RegD1, m68kAMD64RegDataBase, 1*4) // D1 -> RBP
	amd64MOV_reg_mem32(cb, m68kAMD64RegA0, m68kAMD64RegAddrBase, 0*4) // A0 -> R12
	amd64MOV_reg_mem32(cb, m68kAMD64RegA7, m68kAMD64RegAddrBase, 7*4) // A7 -> R13

	// Extract CCR from SR: R14 = *SRPtr & 0x1F
	amd64MOV_reg_mem(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffSRPtr))
	amd64MOV_reg_mem32(cb, m68kAMD64RegCCR, amd64RAX, 0)
	amd64MOVZX_W(cb, m68kAMD64RegCCR, m68kAMD64RegCCR)
	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, 0x1F) // AND R14, 0x1F

	// Seed X flag stack slot from R14 (bit 4) for lazy CCR.
	// Chain exits write R14 bit 4 back into [RSP+OffXFlag] before the
	// JMP rel32, so chained entries can skip this seed and inherit a
	// synced slot from the predecessor.
	amd64MOV_reg_reg32(cb, amd64RAX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RAX, 4)                                     // bit 4 → bit 0
	emitMemOp(cb, false, 0x88, amd64RAX, amd64RSP, m68kAMD64OffXFlag) // MOV [RSP+24], AL

	// Chained jumps from other blocks land here.
	entryOff := cb.Len()

	if br.hasBackwardBranch {
		amd64MOV_mem_imm32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), 0)
	}

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
	// Sync X stack slot with R14 bit 4. Non-lazy CCR emit paths can update
	// R14 bit 4 directly without writing the slot; downstream consumers
	// (lazy materialize, RTS-chained successors that skip the prologue X
	// seed) read from the slot. Keep it pinned to R14's authoritative bit 4.
	amd64MOV_reg_reg32(cb, amd64RAX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RAX, 4)
	emitMemOp(cb, false, 0x88, amd64RAX, amd64RSP, m68kAMD64OffXFlag) // MOV [RSP+24], AL
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

func m68kNativePrefixInstrCount(memory []byte, pc uint32) uint32 {
	if memory == nil || pc >= uint32(len(memory)) {
		return 1
	}
	instrs := m68kScanBlock(memory, pc)
	if len(instrs) == 0 {
		return 1
	}
	prefix := m68kProductionNativePrefix(memory, pc, instrs)
	if len(prefix) == 0 {
		return 1
	}
	return uint32(len(prefix))
}

// amd64ALU_mem_imm8 emits an ALU op (ADD/CMP/SUB) with [base+disp], imm8.
// Opcode 0x83, reg field = aluOp. Sign-extends imm8 to 32 bits.
func amd64ALU_mem_imm8(cb *CodeBuffer, aluOp byte, base byte, disp int32, imm8 int8) {
	emitMemOp(cb, false, 0x83, aluOp, base, disp)
	cb.EmitBytes(byte(imm8))
}

// amd64ALU_mem_imm32 emits an ALU op with [base+disp], imm32.
// Opcode 0x81, reg field = aluOp.
func amd64ALU_mem_imm32(cb *CodeBuffer, aluOp byte, base byte, disp int32, imm32 int32) {
	emitMemOp(cb, false, 0x81, aluOp, base, disp)
	cb.Emit32(uint32(imm32))
}

// amd64ALU_reg_mem32_cmp emits CMP reg32, [base + disp] (opcode 0x3B).
func amd64ALU_reg_mem32_cmp(cb *CodeBuffer, reg, base byte, disp int32) {
	emitMemOp(cb, false, 0x3B, reg, base, disp)
}

// amd64DEC_mem32 emits DEC DWORD [base+disp]. Opcode FF /1.
func amd64DEC_mem32(cb *CodeBuffer, base byte, disp int32) {
	emitMemOp(cb, false, 0xFF, 1, base, disp)
}

// amd64ALU_reg_mem_cmp emits CMP reg64, [base + disp] (opcode 0x3B with REX.W).
func amd64ALU_reg_mem_cmp(cb *CodeBuffer, reg, base byte, disp int32) {
	emitMemOp(cb, true, 0x3B, reg, base, disp)
}

func m68kEmitInvalGenerationChangedCheck(cb *CodeBuffer) int {
	amd64MOV_reg_mem(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffInvalGenPtr))
	amd64MOV_reg_mem(cb, amd64RAX, amd64RAX, 0)
	amd64ALU_reg_mem_cmp(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffInvalGenSnapshot))
	return amd64Jcc_rel32(cb, amd64CondNE)
}

// m68kEmitPendingAsyncExitChecks emits checks for async events that the Go
// dispatcher must deliver at a CPU instruction boundary. Masked interrupts do
// not force an exit; level 7 and levels above the current SR IPL do.
func m68kEmitPendingAsyncExitChecks(cb *CodeBuffer) []int {
	var exits []int

	amd64MOV_reg_mem(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffPendingExceptionPtr))
	amd64ALU_mem_imm8(cb, 7, amd64RAX, 0, 0)
	exits = append(exits, amd64Jcc_rel32(cb, amd64CondNE))

	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffPendingInterruptPtr))
	amd64MOV_reg_mem32(cb, amd64R10, amd64R10, 0)
	amd64TEST_reg_reg32(cb, amd64R10, amd64R10)
	noPendingOff := amd64Jcc_rel32(cb, amd64CondE)

	amd64TEST_reg_imm32(cb, amd64R10, 1<<7)
	exits = append(exits, amd64Jcc_rel32(cb, amd64CondNE))

	amd64MOV_reg_mem(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffSRPtr))
	amd64MOV_reg_mem32(cb, amd64R11, amd64R11, 0)
	amd64SHR_imm(cb, amd64R11, byte(M68K_SR_SHIFT))
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 7)

	for level := uint32(6); level >= 1; level-- {
		amd64TEST_reg_imm32(cb, amd64R10, 1<<level)
		nextLevelOff := amd64Jcc_rel32(cb, amd64CondE)
		amd64ALU_reg_imm32_32bit(cb, 7, amd64R11, int32(level))
		exits = append(exits, amd64Jcc_rel32(cb, amd64CondL))
		patchRel32(cb, nextLevelOff, cb.Len())
	}

	patchRel32(cb, noPendingOff, cb.Len())
	return exits
}

// m68kEmitChainExit emits a chain exit sequence for a block terminator with a
// statically known target PC. The sequence:
//  1. Lightweight epilogue (store regs, merge CCR)
//  2. Accumulate instruction count into ChainCount
//  3. Subtract instruction count from ChainBudget; if exhausted → unchained exit
//  4. Check NeedInval; if set → unchained exit
//  5. Patchable JMP rel32 (initially to unchained exit)
//  6. Unchained exit: set RetPC/RetCount, full pop/ret
//
// Returns the chain exit info for later patching.
func m68kEmitChainExit(cb *CodeBuffer, targetPC uint32, instrCount uint32, targetInstrCount uint32, br *m68kBlockRegs) m68kChainExitInfo {
	// Materialize lazy CCR so R14 holds the canonical 5-bit value the
	// chained target's body will consume (its m68kEmitChainEntry skips
	// the CCR re-extract on the chained-jump path). No register spill
	// here — the chained target reads D0/D1/A0/A7 from their mapped
	// host registers, not memory, so spilling on the success path is
	// pure waste. The .unchained bail still spills before returning to
	// the dispatcher.
	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
	}

	// Sync X stack slot with R14 bit 4 across the chain edge. Non-lazy
	// emitCCR_Arithmetic / emitCCR_Logic paths update R14 bit 4 directly
	// without writing the slot; if the chained successor consumes X
	// lazily (its first arithmetic op leaves flagsLiveLogi/Arith and a
	// later materialize reads from the slot), a stale slot would silently
	// preserve the wrong X. Persisting bit 4 → slot here keeps the chain
	// entry seed-free while making the contract explicit.
	amd64MOV_reg_reg32(cb, amd64RAX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RAX, 4)
	emitMemOp(cb, false, 0x88, amd64RAX, amd64RSP, m68kAMD64OffXFlag) // MOV [RSP+24], AL

	// ADD DWORD [R15 + ChainCount], instrCount
	// Use: load into scratch, add, store back
	amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffChainCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrCount)) // ADD EAX, instrCount
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffChainCount), amd64RAX)

	// SUB DWORD [R15 + ChainBudget], instrCount. The dispatcher seeds this
	// with instructions remaining until the next interpreter IRQ sample.
	amd64ALU_mem_imm32(cb, 5, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget), int32(instrCount))

	// JLE .unchained (budget exhausted — ChainBudget was signed, now <= 0)
	unchainedOff1 := amd64Jcc_rel32(cb, amd64CondLE)

	// CMP DWORD [R15 + NeedInval], 0
	amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedInval), 0)
	// JNE .unchained (self-mod detected)
	unchainedOff2 := amd64Jcc_rel32(cb, amd64CondNE)
	unchainedOffGen := m68kEmitInvalGenerationChangedCheck(cb)
	asyncExitOffs := m68kEmitPendingAsyncExitChecks(cb)

	// Do not chain into a target block that cannot fit before the next
	// interpreter-equivalent sampling boundary. The dispatcher seeds
	// ChainBudget with the instructions remaining to the next 256-instruction
	// poll; without this preflight, a patched chain can overshoot that boundary
	// by a whole block before returning to Go.
	if targetInstrCount == 0 {
		targetInstrCount = 1
	}
	amd64ALU_mem_imm32(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget), int32(targetInstrCount))
	unchainedOff3 := amd64Jcc_rel32(cb, amd64CondL)

	// Publish canonical CPU state before a native-to-native jump. Chained
	// targets keep D0/D1/A0/A7 and CCR live in host registers, but target
	// code may also explicitly read SR or the register arrays (for example
	// status-register moves and native exception exits). Match the unchained
	// exit contract before crossing the block boundary.
	m68kEmitLightweightEpilogue(cb, br)

	// Patchable JMP rel32 — initially jumps to .unchained
	jmpOff := cb.Len()
	cb.EmitBytes(0xE9, 0, 0, 0, 0) // JMP rel32 (placeholder)
	jmpDispOffset := jmpOff + 1    // displacement starts at byte after opcode

	// .unchained label
	unchainedLabel := cb.Len()
	patchRel32(cb, unchainedOff1, unchainedLabel)
	patchRel32(cb, unchainedOff2, unchainedLabel)
	patchRel32(cb, unchainedOffGen, unchainedLabel)
	patchRel32(cb, unchainedOff3, unchainedLabel)
	for _, off := range asyncExitOffs {
		patchRel32(cb, off, unchainedLabel)
	}
	// Patch the initial JMP to point to unchained (will be overwritten when target compiles)
	patchRel32(cb, jmpDispOffset, unchainedLabel)

	// Bail: spill registers + merge CCR into SR so the dispatcher sees a
	// consistent snapshot. m68kMaterializeCCR is idempotent so this call
	// only re-emits the spill and CCR merge.
	m68kEmitLightweightEpilogue(cb, br)

	// Set RetPC = targetPC
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), targetPC)
	// RetCount = 0. ChainCount already holds the full retired total for this
	// chain (every block, including this bailing one, added its own instrCount
	// at the top of m68kEmitChainExit). The dispatcher's
	// m68kJITRetiredInstructionCount returns ChainCount when RetCount==0.
	//
	// Do NOT mirror ChainCount into RetCount here: for a single-block bail
	// (no predecessor chained in), RetCount==ChainCount==blockInstrCount would
	// trip the `retCount <= blockInstrCount` branch and return
	// chainCount+retCount — double-counting the block's instructions. That
	// inflates the dispatcher's InstructionCount, which desynchronizes any
	// instruction-count-keyed interrupt cadence from the interpreter.
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), 0)

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

func m68kEmitImmediateToSRCCR(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, blockStartPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := blockStartPC + ji.pcOffset
	immPC := instrPC + 2
	if immPC+2 > uint32(len(memory)) {
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return
	}

	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
		cs.flagState = flagsMaterialized
	}

	opcode := ji.opcode
	imm := uint32(uint16(memory[immPC])<<8 | uint16(memory[immPC+1]))
	targetSR := opcode == 0x027C || opcode == 0x007C || opcode == 0x0A7C
	if !targetSR {
		imm &= 0xFF
	}

	amd64MOV_reg_mem(cb, amd64R10, amd64RSP, int32(m68kAMD64OffSRPtr))
	amd64MOV_reg_mem32(cb, amd64RAX, amd64R10, 0)
	amd64MOVZX_W(cb, amd64RAX, amd64RAX)
	// SR memory carries the system bits while R14 is the authoritative CCR
	// inside a native block. Merge them before applying SR/CCR immediates so
	// sequences like MOVE <ea>,CCR; EORI #imm,CCR see the updated CCR.
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, int32(0xFFE0))
	amd64MOV_reg_reg32(cb, amd64RCX, m68kAMD64RegCCR)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 0x1F)
	amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)

	if targetSR {
		amd64TEST_reg_imm32(cb, amd64RAX, M68K_SR_S)
		supervisorOff := amd64Jcc_rel32(cb, amd64CondNE)
		var savedFlagState m68kFlagState
		var haveCompileState bool
		if cs := m68kCurrentCS; cs != nil {
			savedFlagState = cs.flagState
			haveCompileState = true
		}
		m68kEmitNativeException(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), instrPC, opcode, M68K_VEC_PRIVILEGE, br)
		if haveCompileState {
			m68kCurrentCS.flagState = savedFlagState
		}
		patchRel32(cb, supervisorOff, cb.Len())
	} else {
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0xFF00)
	}

	switch opcode {
	case 0x023C, 0x027C: // ANDI to CCR/SR
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, int32(imm))
	case 0x003C, 0x007C: // ORI to CCR/SR
		amd64ALU_reg_imm32_32bit(cb, 1, amd64RAX, int32(imm))
	case 0x0A3C, 0x0A7C: // EORI to CCR/SR
		amd64ALU_reg_imm32_32bit(cb, 6, amd64RAX, int32(imm))
	}

	if !targetSR {
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x00FF)
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RDX)
	}

	cb.EmitBytes(0x66)
	emitREX(cb, false, amd64RAX, amd64R10)
	cb.EmitBytes(0x89, modRM(0, amd64RAX, amd64R10)) // MOV [R10], AX

	amd64MOV_reg_reg32(cb, m68kAMD64RegCCR, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, m68kAMD64RegCCR, 0x1F)
	amd64MOV_reg_reg32(cb, amd64RDX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RDX, 4)
	emitMemOp(cb, false, 0x88, amd64RDX, amd64RSP, m68kAMD64OffXFlag) // MOV [RSP+24], DL
}

func m68kEmitMOVEFromStatus(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, blockStartPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := blockStartPC + ji.pcOffset
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	fromSR := opcode&0xFFC0 == 0x40C0

	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedMOVEFromStatus(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	if fromSR {
		amd64MOV_reg_mem(cb, amd64R10, amd64RSP, int32(m68kAMD64OffSRPtr))
		amd64MOV_reg_mem32(cb, amd64RAX, amd64R10, 0)
		amd64TEST_reg_imm32(cb, amd64RAX, M68K_SR_S)
		supervisorOff := amd64Jcc_rel32(cb, amd64CondNE)
		var savedFlagState m68kFlagState
		var haveCompileState bool
		if cs := m68kCurrentCS; cs != nil {
			savedFlagState = cs.flagState
			haveCompileState = true
		}
		m68kEmitNativeException(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), instrPC, opcode, M68K_VEC_PRIVILEGE, br)
		if haveCompileState {
			m68kCurrentCS.flagState = savedFlagState
		}
		patchRel32(cb, supervisorOff, cb.Len())
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0xFFE0)
		amd64MOV_reg_reg32(cb, amd64RCX, m68kAMD64RegCCR)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 0x1F)
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)
	} else {
		amd64MOV_reg_reg32(cb, amd64RAX, m68kAMD64RegCCR)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x1F)
	}

	if mode == 0 {
		m68kStoreDataRegWord(cb, reg, amd64RAX, amd64R11)
		return
	}

	amd64MOV_mem_reg32(cb, amd64RSP, 0, amd64RAX)
	extPC := instrPC + 2
	bailOffs := []int{}
	switch {
	case mode == 3:
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case mode == 4:
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, 2)
	case mode == 6:
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
	}
	amd64MOV_reg_imm32(cb, amd64RDX, 2)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, 2)
	m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 0)
	m68kEmitStoreDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_WORD)
	switch mode {
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, 2)
		m68kStoreAddrReg(cb, reg, ar)
	case 4:
		m68kStoreAddrReg(cb, reg, amd64R10)
	}
	m68kEmitSMCInvalidateRangeCheck(cb, amd64R10, M68K_SIZE_WORD)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitMOVEToCCR(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, blockStartPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := blockStartPC + ji.pcOffset
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedMOVEToCCR(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}

	extPC := instrPC + 2
	bailOffs := []int{}
	switch mode {
	case 0:
		src := m68kResolveDataReg(cb, reg, amd64RAX)
		if src != amd64RAX {
			amd64MOV_reg_reg32(cb, amd64RAX, src)
		}
	case 7:
		if reg == 4 {
			imm := uint32(uint16(memory[extPC])<<8 | uint16(memory[extPC+1]))
			amd64MOV_reg_imm32(cb, amd64RAX, imm)
			break
		}
		if reg == 3 {
			if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
				m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
				return
			}
		} else {
			m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
		}
	case 3:
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case 4:
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, 2)
	case 6:
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
	}

	if !(mode == 0 || (mode == 7 && reg == 4)) {
		amd64MOV_reg_imm32(cb, amd64RDX, 2)
		m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
		m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_WORD)
		switch mode {
		case 3:
			ar := m68kResolveAddrReg(cb, reg, amd64RDX)
			amd64ALU_reg_imm32_32bit(cb, 0, ar, 2)
			m68kStoreAddrReg(cb, reg, ar)
		case 4:
			m68kStoreAddrReg(cb, reg, amd64R10)
		}
	}

	amd64MOV_reg_reg32(cb, m68kAMD64RegCCR, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, m68kAMD64RegCCR, 0x1F)
	// MOVE to CCR installs all five bits including X. The lazy CCR scheme keeps
	// X canonically in the [RSP+OffXFlag] stack slot — a later X-preserving op
	// (e.g. MOVEQ → flagsLiveLogi) rebuilds R14 from that slot, so writing only
	// R14 bit 4 here would be silently dropped at the next materialisation.
	// Persist the freshly-installed X into the slot too.
	amd64MOV_reg_reg32(cb, amd64RAX, m68kAMD64RegCCR) // RAX = src & 0x1F
	amd64SHR_imm(cb, amd64RAX, 4)                     // bit 4 → bit 0
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x1)    // AND RAX, 1 → AL = X
	emitMemOp(cb, false, 0x88, amd64RAX, amd64RSP, m68kAMD64OffXFlag)
	if len(bailOffs) > 0 {
		m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
	}
}

func m68kEmitMOVEToSR(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, blockStartPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := blockStartPC + ji.pcOffset
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
		cs.flagState = flagsMaterialized
	}

	amd64MOV_reg_mem(cb, amd64R10, amd64RSP, int32(m68kAMD64OffSRPtr))
	amd64MOV_reg_mem32(cb, amd64RDX, amd64R10, 0)
	amd64MOVZX_W(cb, amd64RDX, amd64RDX) // old SR
	amd64TEST_reg_imm32(cb, amd64RDX, M68K_SR_S)
	supervisorOff := amd64Jcc_rel32(cb, amd64CondNE)
	m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
	patchRel32(cb, supervisorOff, cb.Len())

	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedMOVEToSR(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	m68kEmitReadSourceEA(cb, mode, reg, M68K_SIZE_WORD, memory, instrPC+2, instrPC, amd64RAX)
	if m68kEAMayUseMemHelper(mode, reg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}

	amd64MOV_reg_mem(cb, amd64R10, amd64RSP, int32(m68kAMD64OffSRPtr))
	amd64MOV_reg_mem32(cb, amd64RDX, amd64R10, 0)
	amd64MOVZX_W(cb, amd64RDX, amd64RDX)             // old SR; source EA reader may clobber RDX/R10
	amd64MOVZX_W(cb, amd64RAX, amd64RAX)             // new SR
	amd64MOV_reg_reg32(cb, amd64RCX, amd64RDX)       // old SR
	amd64ALU_reg_reg32(cb, 0x31, amd64RCX, amd64RAX) // old SR ^ new SR
	amd64TEST_reg_imm32(cb, amd64RCX, M68K_SR_S)     // S bit changed?
	noSwapOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64TEST_reg_imm32(cb, amd64RAX, M68K_SR_S) // entering supervisor?
	toSupervisorOff := amd64Jcc_rel32(cb, amd64CondNE)

	// Supervisor -> user: SSP = active A7; active A7 = USP.
	amd64MOV_reg_mem(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffSSPPtr))
	amd64MOV_mem_reg32(cb, amd64R11, 0, m68kAMD64RegA7)
	amd64MOV_reg_mem(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffUSPPtr))
	amd64MOV_reg_mem32(cb, m68kAMD64RegA7, amd64R11, 0)
	afterSwapOff := amd64JMP_rel32(cb)

	// User -> supervisor: USP = active A7; active A7 = SSP.
	patchRel32(cb, toSupervisorOff, cb.Len())
	amd64MOV_reg_mem(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffUSPPtr))
	amd64MOV_mem_reg32(cb, amd64R11, 0, m68kAMD64RegA7)
	amd64MOV_reg_mem(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffSSPPtr))
	amd64MOV_reg_mem32(cb, m68kAMD64RegA7, amd64R11, 0)

	patchRel32(cb, afterSwapOff, cb.Len())
	patchRel32(cb, noSwapOff, cb.Len())

	cb.EmitBytes(0x66)
	emitREX(cb, false, amd64RAX, amd64R10)
	cb.EmitBytes(0x89, modRM(0, amd64RAX, amd64R10)) // MOV [R10], AX
	amd64MOV_reg_reg32(cb, m68kAMD64RegCCR, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, m68kAMD64RegCCR, 0x1F)
}

func m68kEmitMOVEUSP(cb *CodeBuffer, ji *M68KJITInstr, blockStartPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := blockStartPC + ji.pcOffset
	reg := opcode & 7
	fromUSP := ((opcode >> 3) & 1) == 1

	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
		cs.flagState = flagsMaterialized
	}

	amd64MOV_reg_mem(cb, amd64R10, amd64RSP, int32(m68kAMD64OffSRPtr))
	amd64MOV_reg_mem32(cb, amd64RAX, amd64R10, 0)
	amd64TEST_reg_imm32(cb, amd64RAX, M68K_SR_S)
	supervisorOff := amd64Jcc_rel32(cb, amd64CondNE)
	m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
	patchRel32(cb, supervisorOff, cb.Len())

	amd64MOV_reg_mem(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffUSPPtr))
	if fromUSP {
		amd64MOV_reg_mem32(cb, amd64RAX, amd64R11, 0)
		m68kStoreAddrReg(cb, reg, amd64RAX)
		return
	}

	src := m68kResolveAddrReg(cb, reg, amd64RAX)
	amd64MOV_mem_reg32(cb, amd64R11, 0, src)
}

func m68kMOVECControlPtrOffset(creg uint16) (int32, bool, bool) {
	switch creg {
	case M68K_CR_SFC:
		return int32(m68kCtxOffSFCPtr), true, true
	case M68K_CR_DFC:
		return int32(m68kCtxOffDFCPtr), true, true
	case M68K_CR_USP:
		return int32(m68kCtxOffUSPPtr), false, true
	case M68K_CR_VBR:
		return int32(m68kCtxOffVBRPtr), false, true
	case M68K_CR_CACR:
		return int32(m68kCtxOffCACRPtr), false, true
	case M68K_CR_CAAR:
		return int32(m68kCtxOffCAARPtr), false, true
	case M68K_CR_MSP:
		return int32(m68kCtxOffMSPPtr), false, true
	case M68K_CR_ISP:
		return int32(m68kCtxOffISPPtr), false, true
	default:
		return 0, false, false
	}
}

func m68kEmitMOVEC(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, blockStartPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := blockStartPC + ji.pcOffset
	if memory == nil || instrPC+4 > uint32(len(memory)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	extWord := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	regNum := (extWord >> 12) & 0xF
	regIndex := regNum & 7
	isDataReg := (regNum & 0x8) == 0
	creg := extWord & 0x0FFF
	ptrOff, byteControlReg, ok := m68kMOVECControlPtrOffset(creg)
	if !ok {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
		cs.flagState = flagsMaterialized
	}

	amd64MOV_reg_mem(cb, amd64R10, amd64RSP, int32(m68kAMD64OffSRPtr))
	amd64MOV_reg_mem32(cb, amd64RAX, amd64R10, 0)
	amd64TEST_reg_imm32(cb, amd64RAX, M68K_SR_S)
	supervisorOff := amd64Jcc_rel32(cb, amd64CondNE)
	m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
	patchRel32(cb, supervisorOff, cb.Len())

	amd64MOV_reg_mem(cb, amd64R11, m68kAMD64RegCtx, ptrOff)
	if opcode&1 != 0 {
		var src byte
		if isDataReg {
			src = m68kResolveDataReg(cb, regIndex, amd64RAX)
		} else {
			src = m68kResolveAddrReg(cb, regIndex, amd64RAX)
		}
		if src != amd64RAX {
			amd64MOV_reg_reg32(cb, amd64RAX, src)
		}
		if byteControlReg {
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x07)
			emitMemOp(cb, false, 0x88, amd64RAX, amd64R11, 0) // MOV [R11], AL
		} else {
			amd64MOV_mem_reg32(cb, amd64R11, 0, amd64RAX)
		}
		return
	}

	if byteControlReg {
		amd64MOVZX_B_mem(cb, amd64RAX, amd64R11, 0)
	} else {
		amd64MOV_reg_mem32(cb, amd64RAX, amd64R11, 0)
	}
	if isDataReg {
		m68kStoreDataReg(cb, regIndex, amd64RAX)
	} else {
		m68kStoreAddrReg(cb, regIndex, amd64RAX)
	}
}

func m68kEmitCHK(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, blockStartPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := blockStartPC + ji.pcOffset
	chkMasked := opcode & 0xF1C0
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedCHK(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	size := M68K_SIZE_WORD
	if chkMasked == 0x4100 {
		size = M68K_SIZE_LONG
	}

	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
		cs.flagState = flagsMaterialized
	}

	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7

	m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, instrPC+2, instrPC, amd64RAX)
	amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
	readOKOff := amd64Jcc_rel32(cb, amd64CondE)
	m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
	patchRel32(cb, readOKOff, cb.Len())

	dst := m68kResolveDataReg(cb, dstReg, amd64RDX)
	if dst != amd64RDX {
		amd64MOV_reg_reg32(cb, amd64RDX, dst)
	}

	if size == M68K_SIZE_WORD {
		amd64MOVSX_W(cb, amd64RAX, amd64RAX)
		amd64MOVSX_W(cb, amd64RDX, amd64RDX)
	}

	amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
	negativeOff := amd64Jcc_rel32(cb, 0x8) // JS: dest < 0
	amd64ALU_reg_reg32(cb, 0x39, amd64RDX, amd64RAX)
	tooHighOff := amd64Jcc_rel32(cb, amd64CondA) // unsigned dest > source, matching ExecChk

	amd64ALU_reg_imm32_32bit(cb, 4, m68kAMD64RegCCR, ^int32(m68kCCR_N))
	doneOff := amd64JMP_rel32(cb)

	patchRel32(cb, negativeOff, cb.Len())
	patchRel32(cb, tooHighOff, cb.Len())
	m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)

	patchRel32(cb, doneOff, cb.Len())
}

// m68kEmitMOVE_Dn_Dn emits MOVE.x Ds,Dd (register-to-register).
// size: 0=byte, 1=word, 2=long.
func m68kEmitMOVE_Dn_Dn(cb *CodeBuffer, opcode uint16, size int) {
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7 // MOVE dest: reg at bits 11-9

	src := m68kResolveDataReg(cb, srcReg, amd64RAX)

	amd64MOV_reg_reg32(cb, amd64R11, src)
	switch size {
	case M68K_SIZE_BYTE:
		m68kStoreDataRegByte(cb, dstReg, amd64R11)
	case M68K_SIZE_WORD:
		m68kStoreDataRegWord(cb, dstReg, amd64R11, amd64RDX)
	default:
		m68kStoreDataReg(cb, dstReg, amd64R11)
	}

	// Flags: N,Z from the operation width, V=0, C=0, X unchanged.
	m68kEmitSizedLogicTest(cb, amd64R11, size, amd64RAX)
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

		m68kEmitSizedALURegReg(cb, 0x00, 0x01, dst, src, size) // ADD dst,src
		emitCCR_Arithmetic(cb)
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
		size := int(opmode)
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

		m68kEmitSizedALURegReg(cb, 0x28, 0x29, dst, src, size) // SUB dst,src
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
		opmode := (opcode >> 6) & 7
		size := int(opmode)
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

		m68kEmitSizedALURegReg(cb, 0x38, 0x39, dst, src, size) // CMP dst,src

		// Lazy CCR: CMP sets NZVC but NOT X — use flagsLiveArithNoX
		// so materializer preserves old X from R14 instead of reading CF
		if cs := m68kCurrentCS; cs != nil {
			if m68kCCRDeadAtCurrent() {
				return
			}
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

// m68kEmitCMPA_Reg emits CMPA.W/L Dn/An,An for register-direct sources.
// CMPA compares the destination address register as a 32-bit value and
// updates NZVC only; X is preserved. CMPA.W sign-extends the source word.
func m68kEmitCMPA_Reg(cb *CodeBuffer, opcode uint16) {
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	opmode := (opcode >> 6) & 7
	if opmode != 3 && opmode != 7 {
		return
	}

	var src byte
	switch srcMode {
	case 0:
		src = m68kResolveDataReg(cb, srcReg, amd64RAX)
	case 1:
		src = m68kResolveAddrReg(cb, srcReg, amd64RAX)
	default:
		return
	}
	if opmode == 3 {
		amd64MOV_reg_reg32(cb, amd64RAX, src)
		cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RAX, amd64RAX)) // MOVSX EAX, AX
		src = amd64RAX
	}
	dst := m68kResolveAddrReg(cb, dstReg, amd64RDX)
	amd64ALU_reg_reg32(cb, 0x39, dst, src) // CMP dst,src

	if cs := m68kCurrentCS; cs != nil {
		if m68kCCRDeadAtCurrent() {
			return
		}
		cs.flagState = flagsLiveArithNoX
		return
	}

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

func m68kEmitCMPA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedCMPA(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	opmode := (opcode >> 6) & 7
	size := M68K_SIZE_WORD
	if opmode == 7 {
		size = M68K_SIZE_LONG
	}

	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
			m68kMaterializeCCR(cb, cs)
			cs.flagState = flagsMaterialized
		}
	}

	m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, instrPC+2, instrPC, amd64RAX)
	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}
	if size == M68K_SIZE_WORD {
		amd64MOVSX_W(cb, amd64RAX, amd64RAX)
	}

	dst := m68kResolveAddrReg(cb, dstReg, amd64RDX)
	amd64ALU_reg_reg32(cb, 0x39, dst, amd64RAX) // CMP dst,src

	if cs := m68kCurrentCS; cs != nil {
		if m68kCCRDeadAtCurrent() {
			return
		}
		cs.flagState = flagsLiveArithNoX
		return
	}

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

// m68kEmitAND_Dn_Dn emits AND.L Ds,Dd.
func m68kEmitAND_Dn_Dn(cb *CodeBuffer, opcode uint16) {
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7

	if srcMode == 0 {
		opmode := (opcode >> 6) & 7
		size := int(opmode)
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

		m68kEmitSizedALURegReg(cb, 0x20, 0x21, dst, src, size) // AND dst,src
		emitCCR_LogicPreserveVC(cb)
		m68kStoreDataReg(cb, dstReg, dst)
	}
}

// m68kEmitOR_Dn_Dn emits OR.L Ds,Dd.
func m68kEmitOR_Dn_Dn(cb *CodeBuffer, opcode uint16) {
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7

	if srcMode == 0 {
		opmode := (opcode >> 6) & 7
		size := int(opmode)
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

		m68kEmitSizedALURegReg(cb, 0x08, 0x09, dst, src, size) // OR dst,src
		emitCCR_LogicPreserveVC(cb)
		m68kStoreDataReg(cb, dstReg, dst)
	}
}

func m68kEmitLogic_EA_Dn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, op8, opWide byte) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedLogicEAToDn(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	size := int((opcode >> 6) & 3)
	if m68kModeIsIndexed(srcMode, srcReg) &&
		!m68kIndexedEAAllowed(memory, instrPC+2, srcMode, srcReg) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
			m68kMaterializeCCR(cb, cs)
			cs.flagState = flagsMaterialized
		}
	}

	mutatingSource := srcMode == 3 || srcMode == 4
	if mutatingSource {
		r := m68kResolveAddrReg(cb, srcReg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		if srcMode == 4 {
			amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, srcReg)))
		}
		bail := 0
		m68kEmitMemRead(cb, amd64R10, amd64RDX, size, &bail)
	} else {
		m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, instrPC+2, instrPC, amd64RDX)
	}
	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}
	if mutatingSource {
		if srcMode == 3 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(m68kStepSize(size, srcReg)))
		}
		m68kStoreAddrReg(cb, srcReg, amd64R10)
	}

	dst := m68kResolveDataReg(cb, dstReg, amd64RAX)
	m68kEmitSizedALURegReg(cb, op8, opWide, dst, amd64RDX, size)
	emitCCR_LogicPreserveVC(cb)
	m68kStoreDataReg(cb, dstReg, dst)
}

func m68kEmitLogic_Dn_EA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, op8, opWide byte) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedLogicDnToEA(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	srcReg := (opcode >> 9) & 7
	dstMode := (opcode >> 3) & 7
	dstReg := opcode & 7
	size := int((opcode >> 6) & 3)
	if m68kModeIsIndexed(dstMode, dstReg) && !m68kBriefIndexedEAAllowed(memory, instrPC+2, dstMode, dstReg) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	mutatingDest := dstMode == 3 || dstMode == 4
	if mutatingDest {
		r := m68kResolveAddrReg(cb, dstReg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		if dstMode == 4 {
			amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, dstReg)))
		}
		bail := 0
		m68kEmitMemRead(cb, amd64R10, amd64RAX, size, &bail)
	} else {
		m68kEmitComputeEAAddr(cb, dstMode, dstReg, memory, instrPC+2, instrPC, amd64R10)
		bail := 0
		m68kEmitMemRead(cb, amd64R10, amd64RAX, size, &bail)
	}
	if m68kEAMayUseMemHelper(dstMode, dstReg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}

	src := m68kResolveDataReg(cb, srcReg, amd64RDX)
	m68kEmitSizedALURegReg(cb, op8, opWide, amd64RAX, src, size)
	emitCCR_LogicPreserveVC(cb)
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RAX)
		amd64MOV_mem_reg32(cb, amd64RSP, 36, amd64R10)
		m68kMaterializeCCR(cb, cs)
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
		amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 36)
	}

	bail := 0
	m68kEmitMemWrite(cb, amd64R10, amd64RAX, size, &bail)
	if m68kEAMayUseMemHelper(dstMode, dstReg, true) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		writeOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, writeOKOff, cb.Len())
	}
	if mutatingDest {
		if dstMode == 3 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(m68kStepSize(size, dstReg)))
		}
		m68kStoreAddrReg(cb, dstReg, amd64R10)
	}
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
}

func m68kEmitArith_Dn_EA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, sub bool) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedArithDnToEA(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	srcReg := (opcode >> 9) & 7
	dstMode := (opcode >> 3) & 7
	dstReg := opcode & 7
	size := int((opcode >> 6) & 3)
	if m68kModeIsIndexed(dstMode, dstReg) && !m68kBriefIndexedEAAllowed(memory, instrPC+2, dstMode, dstReg) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	mutatingDest := dstMode == 3 || dstMode == 4
	if mutatingDest {
		r := m68kResolveAddrReg(cb, dstReg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		if dstMode == 4 {
			amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, dstReg)))
		}
		bail := 0
		m68kEmitMemRead(cb, amd64R10, amd64RAX, size, &bail)
	} else {
		m68kEmitComputeEAAddr(cb, dstMode, dstReg, memory, instrPC+2, instrPC, amd64R10)
		bail := 0
		m68kEmitMemRead(cb, amd64R10, amd64RAX, size, &bail)
	}
	if m68kEAMayUseMemHelper(dstMode, dstReg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}

	src := m68kResolveDataReg(cb, srcReg, amd64RDX)
	if sub {
		m68kEmitSizedALURegReg(cb, 0x28, 0x29, amd64RAX, src, size)
	} else {
		m68kEmitSizedALURegReg(cb, 0x00, 0x01, amd64RAX, src, size)
	}
	emitCCR_Arithmetic(cb)
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RAX)
		amd64MOV_mem_reg32(cb, amd64RSP, 36, amd64R10)
		m68kMaterializeCCR(cb, cs)
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
		amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 36)
	}

	bail := 0
	m68kEmitMemWrite(cb, amd64R10, amd64RAX, size, &bail)
	if m68kEAMayUseMemHelper(dstMode, dstReg, true) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		writeOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, writeOKOff, cb.Len())
	}
	if mutatingDest {
		if dstMode == 3 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(m68kStepSize(size, dstReg)))
		}
		m68kStoreAddrReg(cb, dstReg, amd64R10)
	}
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
}

// m68kEmitEXG emits EXG Dx,Dy / Ax,Ay / Dx,Ay. EXG does not affect CCR.
func m68kEmitEXG(cb *CodeBuffer, opcode uint16) {
	rx := (opcode >> 9) & 7
	ry := opcode & 7
	opmode := (opcode >> 3) & 0x1F

	switch opmode {
	case 0x08: // EXG Dx,Dy
		if rx == ry {
			return
		}
		x := m68kResolveDataReg(cb, rx, amd64RAX)
		y := m68kResolveDataReg(cb, ry, amd64RDX)
		amd64MOV_reg_reg32(cb, amd64R10, x)
		m68kStoreDataReg(cb, rx, y)
		m68kStoreDataReg(cb, ry, amd64R10)
	case 0x09: // EXG Ax,Ay
		if rx == ry {
			return
		}
		x := m68kResolveAddrReg(cb, rx, amd64RAX)
		y := m68kResolveAddrReg(cb, ry, amd64RDX)
		amd64MOV_reg_reg32(cb, amd64R10, x)
		m68kStoreAddrReg(cb, rx, y)
		m68kStoreAddrReg(cb, ry, amd64R10)
	case 0x11: // EXG Dx,Ay
		x := m68kResolveDataReg(cb, rx, amd64RAX)
		y := m68kResolveAddrReg(cb, ry, amd64RDX)
		amd64MOV_reg_reg32(cb, amd64R10, x)
		m68kStoreDataReg(cb, rx, y)
		m68kStoreAddrReg(cb, ry, amd64R10)
	}
}

// m68kEmitMULW_EA_Dn emits MULU/MULS.W <ea>,Dn for admitted source modes.
// The current interpreter updates
// N/Z only via SetFlagsNZ, so X/V/C are preserved here as well.
func m68kEmitMULW_EA_Dn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	srcMode := (opcode >> 3) & 7
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedMULW(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	signed := opcode&0x0100 != 0

	m68kEmitReadSourceEA(cb, srcMode, srcReg, M68K_SIZE_WORD, memory, instrPC+2, instrPC, amd64RAX)
	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}
	dst := m68kResolveDataReg(cb, dstReg, amd64RDX)
	if signed {
		amd64MOVSX_W(cb, amd64RAX, amd64RAX)
		amd64MOVSX_W(cb, amd64RDX, dst)
	} else {
		amd64MOVZX_W(cb, amd64RAX, amd64RAX)
		amd64MOVZX_W(cb, amd64RDX, dst)
	}
	amd64IMUL_reg_reg32(cb, amd64RDX, amd64RAX)
	m68kStoreDataReg(cb, dstReg, amd64RDX)
	m68kEmitSizedLogicTest(cb, amd64RDX, M68K_SIZE_LONG, amd64RAX)
	emitCCR_LogicPreserveVC(cb)
}

// m68kEmitEOR_Dn_Dn emits EOR.L Dn,<ea> (Group B, opmode 4-6).
func m68kEmitEOR_Dn_Dn(cb *CodeBuffer, opcode uint16) {
	srcReg := (opcode >> 9) & 7 // EOR: Dn is in bits 11-9
	dstMode := (opcode >> 3) & 7
	dstReg := opcode & 7

	if dstMode == 0 {
		opmode := (opcode >> 6) & 7
		size := int(opmode - 4)
		src := m68kResolveDataReg(cb, srcReg, amd64RAX)
		dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

		m68kEmitSizedALURegReg(cb, 0x30, 0x31, dst, src, size) // XOR dst,src
		emitCCR_LogicPreserveVC(cb)
		m68kStoreDataReg(cb, dstReg, dst)
	}
}

// m68kEmitNOT emits NOT.B/W/L for admitted register and memory EAs.
func m68kEmitNOT(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedNOT(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if mode == 0 { // Data register direct
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		switch size {
		case M68K_SIZE_BYTE:
			amd64ALU_reg_imm32_32bit(cb, 6, r, 0xFF) // XOR low byte bits only
			m68kStoreDataReg(cb, reg, r)
			scratch := byte(amd64RAX)
			if r == scratch {
				scratch = byte(amd64RCX)
			}
			amd64MOV_reg_reg32(cb, scratch, r)
			amd64MOVZX_B(cb, scratch, scratch)
			amd64SHL_imm32(cb, scratch, 24) // promote bit 7 to 32-bit sign flag
		case M68K_SIZE_WORD:
			amd64ALU_reg_imm32_32bit(cb, 6, r, 0xFFFF) // XOR low word bits only
			m68kStoreDataReg(cb, reg, r)
			scratch := byte(amd64RAX)
			if r == scratch {
				scratch = byte(amd64RCX)
			}
			amd64MOV_reg_reg32(cb, scratch, r)
			amd64MOVZX_W(cb, scratch, scratch)
			amd64SHL_imm32(cb, scratch, 16) // promote bit 15 to 32-bit sign flag
		case M68K_SIZE_LONG:
			// NOT in x86 doesn't affect flags, so we need TEST after.
			emitREX(cb, false, 0, r)
			cb.EmitBytes(0xF7, modRM(3, 2, r)) // NOT r32
			m68kStoreDataReg(cb, reg, r)
			amd64TEST_reg_reg32(cb, r, r)
		default:
			return
		}
		emitCCR_LogicPreserveVC(cb)
		return
	}

	if cs := m68kCurrentCS; cs != nil {
		if cs.flagState != flagsMaterialized {
			m68kMaterializeCCR(cb, cs)
		}
		cs.flagState = flagsMaterialized
	}

	extPC := instrPC + 2
	bailOffs := []int{}
	switch {
	case mode == 3: // (An)+
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case mode == 4: // -(An)
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, reg)))
	case mode == 6:
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
	}

	accessBytes := m68kAccessSizeBytes(size)
	amd64MOV_reg_imm32(cb, amd64RDX, accessBytes)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, accessBytes)
	m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, size)
	switch size {
	case M68K_SIZE_BYTE:
		amd64ALU_reg_imm32_32bit(cb, 6, amd64RAX, 0xFF)
	case M68K_SIZE_WORD:
		amd64ALU_reg_imm32_32bit(cb, 6, amd64RAX, 0xFFFF)
	case M68K_SIZE_LONG:
		emitREX(cb, false, 0, amd64RAX)
		cb.EmitBytes(0xF7, modRM(3, 2, amd64RAX))
	default:
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	m68kEmitStoreDirectRAM(cb, amd64R10, amd64RAX, size)

	switch mode {
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(size, reg)))
		m68kStoreAddrReg(cb, reg, ar)
	case 4:
		m68kStoreAddrReg(cb, reg, amd64R10)
	}

	m68kEmitSizedLogicTest(cb, amd64RAX, size, amd64RDX)
	emitCCR_LogicPreserveVC(cb)
	m68kEmitSMCInvalidateRangeCheck(cb, amd64R10, size)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
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
	m68kEmitExitIfIOFallback(cb, instrPC, uint32(instrIdx), br)
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
	m68kEmitExitIfIOFallback(cb, instrPC, uint32(instrIdx), br)
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

// m68kEmitDIVW_EA_Dn emits native DIVU.W/DIVS.W <ea>,Dn for admitted source modes.
// Zero-divide bails to the interpreter so the architected exception path stays exact.
func m68kEmitDIVW_EA_Dn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	signed := opcode&0x0100 != 0
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedDIVW(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	m68kEmitReadSourceEA(cb, srcMode, srcReg, M68K_SIZE_WORD, memory, instrPC+2, instrPC, amd64RCX)
	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}
	if signed {
		amd64MOVSX_W(cb, amd64RCX, amd64RCX)
	} else {
		amd64MOVZX_W(cb, amd64RCX, amd64RCX)
	}
	amd64TEST_reg_reg32(cb, amd64RCX, amd64RCX)
	divZeroOff := amd64Jcc_rel32(cb, amd64CondE)

	dst := m68kResolveDataReg(cb, dstReg, amd64R10)
	amd64MOV_reg_reg32(cb, amd64RAX, dst)

	var overflowOffs []int
	if signed {
		amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, int32(-2147483648))
		notMinOff := amd64Jcc_rel32(cb, amd64CondNE)
		amd64ALU_reg_imm32_32bit(cb, 7, amd64RCX, -1)
		overflowOffs = append(overflowOffs, amd64Jcc_rel32(cb, amd64CondE))
		patchRel32(cb, notMinOff, cb.Len())

		amd64CDQ(cb)
		amd64IDIV32(cb, amd64RCX)
		amd64MOVSX_W(cb, amd64R11, amd64RAX)
		amd64ALU_reg_reg32(cb, 0x39, amd64RAX, amd64R11) // quotient must fit int16
		overflowOffs = append(overflowOffs, amd64Jcc_rel32(cb, amd64CondNE))
	} else {
		amd64XOR_reg_reg32(cb, amd64RDX, amd64RDX)
		amd64DIV32(cb, amd64RCX)
		amd64MOV_reg_reg32(cb, amd64R11, amd64RAX)
		amd64SHR_imm(cb, amd64R11, 16)
		amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
		overflowOffs = append(overflowOffs, amd64Jcc_rel32(cb, amd64CondNE))
	}

	amd64MOV_reg_reg32(cb, amd64R10, amd64RDX)
	amd64SHL_imm(cb, amd64R10, 16)
	amd64MOVZX_W(cb, amd64RAX, amd64RAX)
	amd64ALU_reg_reg32(cb, 0x09, amd64R10, amd64RAX)
	m68kStoreDataReg(cb, dstReg, amd64R10)

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X) // preserve X only
	amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 0xFFFF)
	amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
	skipZOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	patchRel32(cb, skipZOff, cb.Len())
	amd64TEST_reg_imm32(cb, amd64R10, 0x8000)
	skipNOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
	patchRel32(cb, skipNOff, cb.Len())
	successDoneOff := amd64JMP_rel32(cb)

	overflowLabel := cb.Len()
	for _, off := range overflowOffs {
		patchRel32(cb, off, overflowLabel)
	}
	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X) // preserve X only
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_V)
	overflowDoneOff := amd64JMP_rel32(cb)

	patchRel32(cb, divZeroOff, cb.Len())
	m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)

	doneLabel := cb.Len()
	patchRel32(cb, successDoneOff, doneLabel)
	patchRel32(cb, overflowDoneOff, doneLabel)

	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
}

// m68kEmitNEG emits NEG.B/W/L for admitted register and memory EAs.
func m68kEmitNEG(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedNEG(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if mode == 0 {
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		switch size {
		case M68K_SIZE_BYTE:
			emitREXForByte(cb, 3, r)
			cb.EmitBytes(0xF6, modRM(3, 3, r))
		case M68K_SIZE_WORD:
			cb.EmitBytes(0x66)
			emitREX(cb, false, 0, r)
			cb.EmitBytes(0xF7, modRM(3, 3, r))
		case M68K_SIZE_LONG:
			amd64NEG32(cb, r)
		default:
			return
		}
		emitCCR_Arithmetic(cb)
		m68kStoreDataReg(cb, reg, r)
		return
	}

	if cs := m68kCurrentCS; cs != nil {
		if cs.flagState != flagsMaterialized {
			m68kMaterializeCCR(cb, cs)
		}
		cs.flagState = flagsMaterialized
	}

	extPC := instrPC + 2
	bailOffs := []int{}
	switch {
	case mode == 3: // (An)+
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case mode == 4: // -(An)
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, reg)))
	case mode == 6:
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
	}

	accessBytes := m68kAccessSizeBytes(size)
	amd64MOV_reg_imm32(cb, amd64RDX, accessBytes)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, accessBytes)
	m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, size)
	amd64XOR_reg_reg32(cb, amd64RDX, amd64RDX)
	m68kEmitSizedALURegReg(cb, 0x28, 0x29, amd64RDX, amd64RAX, size) // SUB 0,src
	amd64MOV_reg_reg32(cb, amd64R8, amd64RDX)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64R10)
	m68kEmitCCRArithmeticMaterialized(cb)
	amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 32)
	amd64MOV_reg_reg32(cb, amd64RAX, amd64R8)
	m68kEmitStoreDirectRAM(cb, amd64R10, amd64RAX, size)

	switch mode {
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(size, reg)))
		m68kStoreAddrReg(cb, reg, ar)
	case 4:
		m68kStoreAddrReg(cb, reg, amd64R10)
	}

	m68kEmitSMCInvalidateRangeCheck(cb, amd64R10, size)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

// m68kEmitNEGX emits NEGX.B/W/L for admitted register and memory EAs.
func m68kEmitNEGX(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedNEGX(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	if mode == 0 {
		dst := m68kResolveDataReg(cb, reg, amd64RDX)
		amd64MOV_reg_reg32(cb, amd64RAX, dst) // original operand

		amd64MOV_reg_reg32(cb, amd64R11, m68kAMD64RegCCR)
		amd64SHR_imm(cb, amd64R11, 2)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 1)
		amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64R11)

		switch size {
		case M68K_SIZE_BYTE:
			amd64ALU_reg_imm32_32bit(cb, 4, dst, ^int32(0xFF))
		case M68K_SIZE_WORD:
			amd64ALU_reg_imm32_32bit(cb, 4, dst, ^int32(0xFFFF))
		case M68K_SIZE_LONG:
			amd64XOR_reg_reg32(cb, dst, dst)
		default:
			return
		}

		amd64BTRegImm32(cb, m68kAMD64RegCCR, 4)
		m68kEmitSizedALURegReg(cb, 0x18, 0x19, dst, amd64RAX, size) // SBB dst,original
		m68kStoreDataReg(cb, reg, dst)

		amd64SETcc(cb, amd64CondB, amd64RCX) // C
		amd64SETcc(cb, amd64CondO, amd64RDX) // V
		amd64SETcc(cb, amd64CondE, amd64R10) // result zero
		amd64SETcc(cb, 0x8, amd64R11)        // N

		amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX)

		amd64MOVZX_B(cb, amd64RAX, amd64RCX)
		amd64SHL_imm(cb, amd64RAX, 4)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // X = C

		amd64MOVZX_B(cb, amd64RAX, amd64RDX)
		amd64SHL_imm(cb, amd64RAX, 1)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

		amd64MOVZX_B(cb, amd64RAX, amd64R10)
		amd64MOV_reg_mem32(cb, amd64RDX, amd64RSP, 32)
		amd64ALU_reg_reg32(cb, 0x21, amd64RAX, amd64RDX) // Z = resultZero && oldZ
		amd64SHL_imm(cb, amd64RAX, 2)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

		amd64MOVZX_B(cb, amd64RAX, amd64R11)
		amd64SHL_imm(cb, amd64RAX, 3)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
		return
	}

	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}

	extPC := instrPC + 2
	bailOffs := []int{}
	switch {
	case mode == 3: // (An)+
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case mode == 4: // -(An)
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, reg)))
	case mode == 6:
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
	}

	accessBytes := m68kAccessSizeBytes(size)
	amd64MOV_reg_imm32(cb, amd64RDX, accessBytes)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, accessBytes)
	m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, size)
	amd64MOV_reg_reg32(cb, amd64R11, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64R11, 2)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 1)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64R11)

	amd64XOR_reg_reg32(cb, amd64RDX, amd64RDX)
	amd64BTRegImm32(cb, m68kAMD64RegCCR, 4)
	m68kEmitSizedALURegReg(cb, 0x18, 0x19, amd64RDX, amd64RAX, size) // SBB 0,original
	amd64MOV_reg_reg32(cb, amd64R8, amd64RDX)
	amd64MOV_mem_reg32(cb, amd64RSP, 0, amd64R10)

	amd64SETcc(cb, amd64CondB, amd64RCX) // C
	amd64SETcc(cb, amd64CondO, amd64RDX) // V
	amd64SETcc(cb, amd64CondE, amd64R10) // result zero
	amd64SETcc(cb, 0x8, amd64R11)        // N

	amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX)

	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // X = C

	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 1)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOVZX_B(cb, amd64RAX, amd64R10)
	amd64MOV_reg_mem32(cb, amd64RDX, amd64RSP, 32)
	amd64ALU_reg_reg32(cb, 0x21, amd64RAX, amd64RDX) // Z = resultZero && oldZ
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 0)
	amd64MOV_reg_reg32(cb, amd64RAX, amd64R8)
	m68kEmitStoreDirectRAM(cb, amd64R10, amd64RAX, size)

	switch mode {
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(size, reg)))
		m68kStoreAddrReg(cb, reg, ar)
	case 4:
		m68kStoreAddrReg(cb, reg, amd64R10)
	}

	m68kEmitSMCInvalidateRangeCheck(cb, amd64R10, size)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

// m68kEmitCLR emits CLR.B/W/L for admitted register and memory EAs.
func m68kEmitCLR(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedCLR(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if cs := m68kCurrentCS; cs != nil {
		if cs.flagState != flagsMaterialized {
			m68kMaterializeCCR(cb, cs)
		}
		cs.flagState = flagsMaterialized
	}

	if mode == 0 {
		switch size {
		case M68K_SIZE_BYTE:
			amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)
			m68kStoreDataRegByte(cb, reg, amd64RAX)
		case M68K_SIZE_WORD:
			amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)
			m68kStoreDataRegWord(cb, reg, amd64RAX, amd64RDX)
		case M68K_SIZE_LONG:
			r, mapped := m68kDataRegToAMD64(reg)
			if mapped {
				amd64XOR_reg_reg32(cb, r, r)
			} else {
				amd64MOV_mem_imm32(cb, m68kAMD64RegDataBase, int32(reg)*4, 0)
			}
		default:
			return
		}
		// CLR sets: N=0, Z=1, V=0, C=0, X unchanged
		amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X) // keep X
		amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z) // set Z
		return
	}

	extPC := instrPC + 2
	bailOffs := []int{}
	switch {
	case mode == 3: // (An)+
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case mode == 4: // -(An)
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, reg)))
	case mode == 6:
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
	}

	accessBytes := m68kAccessSizeBytes(size)
	amd64MOV_reg_imm32(cb, amd64RDX, accessBytes)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	amd64XOR_reg_reg32(cb, amd64R11, amd64R11)
	switch size {
	case M68K_SIZE_BYTE:
		emitMemOpSIB(cb, false, 0x88, amd64R11, m68kAMD64RegMemBase, amd64R10, 0)
	case M68K_SIZE_WORD:
		cb.EmitBytes(0x66)
		emitMemOpSIB(cb, false, 0x89, amd64R11, m68kAMD64RegMemBase, amd64R10, 0)
	case M68K_SIZE_LONG:
		emitMemOpSIB(cb, false, 0x89, amd64R11, m68kAMD64RegMemBase, amd64R10, 0)
	default:
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	switch mode {
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(size, reg)))
		m68kStoreAddrReg(cb, reg, ar)
	case 4:
		m68kStoreAddrReg(cb, reg, amd64R10)
	}

	m68kEmitSMCInvalidateRangeCheck(cb, amd64R10, size)

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchCLRSlowBails(cb, bailOffs, opcode, instrPC, br, instrIdx)
}

func m68kEmitCLRLongStackPredec(cb *CodeBuffer, ji *M68KJITInstr, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset

	amd64MOV_reg_reg32(cb, amd64R10, m68kAMD64RegA7)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, 4)

	bailOffs := make([]int, 0, 8)
	amd64TEST_reg_imm8(cb, amd64R10, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_imm32(cb, amd64RDX, 4)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, 4)
	m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	amd64MOV_reg_imm32(cb, amd64R11, 0)
	emitMemOpSIB(cb, false, 0x89, amd64R11, m68kAMD64RegMemBase, amd64R10, 0)
	m68kStoreAddrReg(cb, 7, amd64R10)

	// CLR sets N=0, Z=1, V=0, C=0, and leaves X unchanged.
	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)

	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

// m68kEmitTST emits TST.{B,W,L} for admitted register and memory EAs.
func m68kEmitTST(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedTST(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if mode == 0 || mode == 1 {
		var r byte
		if mode == 0 {
			r = m68kResolveDataReg(cb, reg, amd64RAX)
		} else {
			r = m68kResolveAddrReg(cb, reg, amd64RAX)
		}
		m68kEmitSizedLogicTest(cb, r, size, amd64RDX)
		emitCCR_Logic(cb)
		return
	}

	if cs := m68kCurrentCS; cs != nil {
		if cs.flagState != flagsMaterialized {
			m68kMaterializeCCR(cb, cs)
		}
		cs.flagState = flagsMaterialized
	}

	extPC := instrPC + 2
	bailOffs := []int{}
	switch {
	case mode == 3: // (An)+
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case mode == 4: // -(An)
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, reg)))
	case mode == 6 || (mode == 7 && reg == 3):
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
	}

	amd64MOV_reg_imm32(cb, amd64RDX, m68kAccessSizeBytes(size))
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, size)
	switch mode {
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(size, reg)))
		m68kStoreAddrReg(cb, reg, ar)
	case 4:
		m68kStoreAddrReg(cb, reg, amd64R10)
	}
	m68kEmitSizedLogicTest(cb, amd64RAX, size, amd64RDX)
	emitCCR_Logic(cb)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitTAS(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedTAS(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	if mode == 0 {
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		amd64TEST_reg_imm8(cb, r, 0xFF)
		amd64SETcc(cb, amd64CondE, amd64R10) // Z from original byte
		amd64SETcc(cb, 0x8, amd64R11)        // N from original byte

		amd64ALU_reg_imm32_32bit(cb, 1, r, 0x80) // set low-byte bit 7
		m68kStoreDataReg(cb, reg, r)

		amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X) // keep X, clear NZVC
		amd64MOVZX_B(cb, amd64RAX, amd64R10)
		amd64SHL_imm(cb, amd64RAX, 2)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
		amd64MOVZX_B(cb, amd64RAX, amd64R11)
		amd64SHL_imm(cb, amd64RAX, 3)
		amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
		if cs := m68kCurrentCS; cs != nil {
			cs.flagState = flagsMaterialized
		}
		return
	}

	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}

	extPC := instrPC + 2
	bailOffs := []int{}
	switch {
	case mode == 3: // (An)+
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case mode == 4: // -(An)
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(M68K_SIZE_BYTE, reg)))
	case mode == 6:
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
	}

	amd64MOV_reg_imm32(cb, amd64RDX, 1)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, 1)
	m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_BYTE)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64R10)
	amd64TEST_reg_imm8(cb, amd64RAX, 0xFF)
	amd64SETcc(cb, amd64CondE, amd64R10) // Z from original byte
	amd64SETcc(cb, 0x8, amd64R11)        // N from original byte
	amd64MOV_reg_reg32(cb, amd64R8, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 1, amd64R8, 0x80)

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X) // keep X, clear NZVC
	amd64MOVZX_B(cb, amd64RAX, amd64R10)
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 32)
	amd64MOV_reg_reg32(cb, amd64RAX, amd64R8)
	m68kEmitStoreDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_BYTE)

	switch mode {
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(M68K_SIZE_BYTE, reg)))
		m68kStoreAddrReg(cb, reg, ar)
	case 4:
		m68kStoreAddrReg(cb, reg, amd64R10)
	}

	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitNBCD(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedNBCD(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}

	amd64MOV_reg_reg32(cb, amd64R11, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64R11, 2)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 1)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64R11)

	bailOffs := []int{}
	if mode == 0 {
		src := m68kResolveDataReg(cb, reg, amd64RAX)
		if src != amd64RAX {
			amd64MOV_reg_reg32(cb, amd64RAX, src)
		}
	} else {
		extPC := instrPC + 2
		switch {
		case mode == 3: // (An)+
			r := m68kResolveAddrReg(cb, reg, amd64R10)
			if r != amd64R10 {
				amd64MOV_reg_reg32(cb, amd64R10, r)
			}
		case mode == 4: // -(An)
			r := m68kResolveAddrReg(cb, reg, amd64R10)
			if r != amd64R10 {
				amd64MOV_reg_reg32(cb, amd64R10, r)
			}
			amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(M68K_SIZE_BYTE, reg)))
		case mode == 6:
			if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
				m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
				return
			}
		default:
			m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
		}

		amd64MOV_reg_imm32(cb, amd64RDX, 1)
		m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
		amd64MOV_reg_imm32(cb, amd64RDX, 1)
		m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

		m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_BYTE)
		amd64MOV_mem_reg32(cb, amd64RSP, 0, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R11, m68kAMD64RegCCR)
		amd64SHR_imm(cb, amd64R11, 2)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 1)
		amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64R11)
	}
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0xFF)

	amd64MOV_reg_reg32(cb, amd64RCX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RCX, 4)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 1)

	amd64MOV_reg_reg32(cb, amd64R10, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0x0F)
	amd64XOR_reg_reg32(cb, amd64RDX, amd64RDX)
	amd64ALU_reg_reg32(cb, 0x29, amd64RDX, amd64R10) // res = 0 - low
	amd64ALU_reg_reg32(cb, 0x29, amd64RDX, amd64RCX) // res -= X
	amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
	lowDoneOff := amd64Jcc_rel32(cb, amd64CondGE)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RDX, 6)
	patchRel32(cb, lowDoneOff, cb.Len())

	amd64MOV_reg_reg32(cb, amd64R11, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 0xF0)
	amd64ALU_reg_reg32(cb, 0x29, amd64RDX, amd64R11) // res -= high

	amd64XOR_reg_reg32(cb, amd64RCX, amd64RCX)
	amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
	adjustDoneOff := amd64Jcc_rel32(cb, amd64CondGE)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 0xA0)
	amd64MOV_reg_imm32(cb, amd64RCX, 1)
	patchRel32(cb, adjustDoneOff, cb.Len())

	amd64MOV_reg_reg32(cb, amd64R10, amd64RDX)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0xFF)

	amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX) // C
	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // X = C

	amd64TEST_reg_imm8(cb, amd64R10, 0xFF)
	amd64SETcc(cb, amd64CondE, amd64RDX)
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
	amd64ALU_reg_reg32(cb, 0x21, amd64RDX, amd64RAX) // Z = resultZero && oldZ
	amd64SHL_imm(cb, amd64RDX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RDX)

	amd64TEST_reg_imm8(cb, amd64R10, 0x80)
	amd64SETcc(cb, amd64CondNE, amd64R11)
	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	if mode == 0 {
		m68kStoreDataRegByte(cb, reg, amd64R10)
		return
	}

	amd64MOV_reg_mem32(cb, amd64R11, amd64RSP, 0)
	amd64MOV_reg_reg32(cb, amd64RAX, amd64R10)
	m68kEmitStoreDirectRAM(cb, amd64R11, amd64RAX, M68K_SIZE_BYTE)

	switch mode {
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(M68K_SIZE_BYTE, reg)))
		m68kStoreAddrReg(cb, reg, ar)
	case 4:
		m68kStoreAddrReg(cb, reg, amd64R11)
	}
	m68kEmitSMCInvalidateRangeCheck(cb, amd64R11, M68K_SIZE_BYTE)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitABCDSBCD_Dn_Dn(cb *CodeBuffer, opcode uint16, subtract bool) {
	regMode := (opcode >> 3) & 1
	if regMode != 0 {
		return
	}
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	amd64MOV_reg_reg32(cb, amd64R10, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64R10, 2)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 1)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64R10)

	src := m68kResolveDataReg(cb, srcReg, amd64RAX)
	if src != amd64RAX {
		amd64MOV_reg_reg32(cb, amd64RAX, src)
	}
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0xFF)

	dst := m68kResolveDataReg(cb, dstReg, amd64R11)
	if dst != amd64R11 {
		amd64MOV_reg_reg32(cb, amd64R11, dst)
	}
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 0xFF)

	amd64MOV_reg_reg32(cb, amd64RCX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RCX, 4)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 1)

	if subtract {
		amd64MOV_reg_reg32(cb, amd64RDX, amd64R11)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0x0F)
		amd64MOV_reg_reg32(cb, amd64R10, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0x0F)
		amd64ALU_reg_reg32(cb, 0x29, amd64RDX, amd64R10) // dst low - src low
		amd64ALU_reg_reg32(cb, 0x29, amd64RDX, amd64RCX) // res -= X
		amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
		lowDoneOff := amd64Jcc_rel32(cb, amd64CondGE)
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RDX, 6)
		patchRel32(cb, lowDoneOff, cb.Len())

		amd64MOV_reg_reg32(cb, amd64R10, amd64R11)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0xF0)
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R10, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0xF0)
		amd64ALU_reg_reg32(cb, 0x29, amd64RDX, amd64R10)

		amd64XOR_reg_reg32(cb, amd64RCX, amd64RCX)
		amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
		adjustDoneOff := amd64Jcc_rel32(cb, amd64CondGE)
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 0xA0)
		amd64MOV_reg_imm32(cb, amd64RCX, 1)
		patchRel32(cb, adjustDoneOff, cb.Len())
	} else {
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0x0F)
		amd64MOV_reg_reg32(cb, amd64R10, amd64R11)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0x0F)
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64R10)
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64RCX)
		amd64ALU_reg_imm32_32bit(cb, 7, amd64RDX, 9)
		lowDoneOff := amd64Jcc_rel32(cb, amd64CondLE)
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 6)
		patchRel32(cb, lowDoneOff, cb.Len())

		amd64MOV_reg_reg32(cb, amd64R10, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0xF0)
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R10, amd64R11)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0xF0)
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64R10)

		amd64XOR_reg_reg32(cb, amd64RCX, amd64RCX)
		amd64ALU_reg_imm32_32bit(cb, 7, amd64RDX, 0x99)
		adjustDoneOff := amd64Jcc_rel32(cb, amd64CondLE)
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 0x60)
		amd64MOV_reg_imm32(cb, amd64RCX, 1)
		patchRel32(cb, adjustDoneOff, cb.Len())
	}

	amd64MOV_reg_reg32(cb, amd64R10, amd64RDX)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0xFF)

	amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX) // C
	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // X = C

	amd64TEST_reg_imm8(cb, amd64R10, 0xFF)
	amd64SETcc(cb, amd64CondE, amd64RDX)
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
	amd64ALU_reg_reg32(cb, 0x21, amd64RDX, amd64RAX) // Z = resultZero && oldZ
	amd64SHL_imm(cb, amd64RDX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RDX)

	amd64TEST_reg_imm8(cb, amd64R10, 0x80)
	amd64SETcc(cb, amd64CondNE, amd64R11)
	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	m68kStoreDataRegByte(cb, dstReg, amd64R10)
}

func m68kEmitABCDSBCD_PredecMem(cb *CodeBuffer, ji *M68KJITInstr, startPC uint32, br *m68kBlockRegs, instrIdx int, subtract bool) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	regMode := (opcode >> 3) & 1
	if regMode != 1 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	rx := opcode & 7
	ry := (opcode >> 9) & 7

	srcAR := m68kResolveAddrReg(cb, rx, amd64R10)
	if srcAR != amd64R10 {
		amd64MOV_reg_reg32(cb, amd64R10, srcAR)
	}
	amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, 1)

	if rx == ry {
		amd64MOV_reg_reg32(cb, amd64RCX, amd64R10)
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
	} else {
		dstAR := m68kResolveAddrReg(cb, ry, amd64RCX)
		if dstAR != amd64RCX {
			amd64MOV_reg_reg32(cb, amd64RCX, dstAR)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
	}

	bailOffs := []int{}
	amd64MOV_mem_reg32(cb, amd64RSP, 48, amd64RCX)
	amd64MOV_reg_imm32(cb, amd64RDX, 1)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 48)
	amd64MOV_reg_imm32(cb, amd64RDX, 1)
	m68kEmitMemRangeBailChecks(cb, amd64RCX, amd64RDX, &bailOffs)
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 48)
	amd64MOV_reg_imm32(cb, amd64RDX, 1)
	m68kEmitSMCRangeBailChecks(cb, amd64RCX, amd64RDX, &bailOffs)

	m68kStoreAddrReg(cb, rx, amd64R10)
	m68kStoreAddrReg(cb, ry, amd64RCX)
	amd64MOV_mem_reg32(cb, amd64RSP, 36, amd64RCX)

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_BYTE)
	m68kEmitLoadDirectRAM(cb, amd64RCX, amd64R11, M68K_SIZE_BYTE)
	amd64MOVZX_B(cb, amd64RAX, amd64RAX)
	amd64MOVZX_B(cb, amd64R11, amd64R11)

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		amd64MOV_mem_reg32(cb, amd64RSP, 40, amd64RAX)
		amd64MOV_mem_reg32(cb, amd64RSP, 44, amd64R11)
		m68kMaterializeCCR(cb, cs)
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 40)
		amd64MOV_reg_mem32(cb, amd64R11, amd64RSP, 44)
	}

	amd64MOV_reg_reg32(cb, amd64R10, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64R10, 2)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 1)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64R10)

	amd64MOV_reg_reg32(cb, amd64RCX, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64RCX, 4)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 1)

	if subtract {
		amd64MOV_reg_reg32(cb, amd64RDX, amd64R11)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0x0F)
		amd64MOV_reg_reg32(cb, amd64R10, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0x0F)
		amd64ALU_reg_reg32(cb, 0x29, amd64RDX, amd64R10)
		amd64ALU_reg_reg32(cb, 0x29, amd64RDX, amd64RCX)
		amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
		lowDoneOff := amd64Jcc_rel32(cb, amd64CondGE)
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RDX, 6)
		patchRel32(cb, lowDoneOff, cb.Len())

		amd64MOV_reg_reg32(cb, amd64R10, amd64R11)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0xF0)
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R10, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0xF0)
		amd64ALU_reg_reg32(cb, 0x29, amd64RDX, amd64R10)

		amd64XOR_reg_reg32(cb, amd64RCX, amd64RCX)
		amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
		adjustDoneOff := amd64Jcc_rel32(cb, amd64CondGE)
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 0xA0)
		amd64MOV_reg_imm32(cb, amd64RCX, 1)
		patchRel32(cb, adjustDoneOff, cb.Len())
	} else {
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0x0F)
		amd64MOV_reg_reg32(cb, amd64R10, amd64R11)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0x0F)
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64R10)
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64RCX)
		amd64ALU_reg_imm32_32bit(cb, 7, amd64RDX, 9)
		lowDoneOff := amd64Jcc_rel32(cb, amd64CondLE)
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 6)
		patchRel32(cb, lowDoneOff, cb.Len())

		amd64MOV_reg_reg32(cb, amd64R10, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0xF0)
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64R10)
		amd64MOV_reg_reg32(cb, amd64R10, amd64R11)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0xF0)
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64R10)

		amd64XOR_reg_reg32(cb, amd64RCX, amd64RCX)
		amd64ALU_reg_imm32_32bit(cb, 7, amd64RDX, 0x99)
		adjustDoneOff := amd64Jcc_rel32(cb, amd64CondLE)
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 0x60)
		amd64MOV_reg_imm32(cb, amd64RCX, 1)
		patchRel32(cb, adjustDoneOff, cb.Len())
	}

	amd64MOV_reg_reg32(cb, amd64R10, amd64RDX)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0xFF)
	amd64MOV_mem_reg32(cb, amd64RSP, 40, amd64R10)
	amd64MOV_reg_mem32(cb, amd64RDX, amd64RSP, 36)
	m68kEmitStoreDirectRAM(cb, amd64RDX, amd64R10, M68K_SIZE_BYTE)
	amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 40)

	amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX)
	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64TEST_reg_imm8(cb, amd64R10, 0xFF)
	amd64SETcc(cb, amd64CondE, amd64RDX)
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
	amd64ALU_reg_reg32(cb, 0x21, amd64RDX, amd64RAX)
	amd64SHL_imm(cb, amd64RDX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RDX)

	amd64TEST_reg_imm8(cb, amd64R10, 0x80)
	amd64SETcc(cb, amd64CondNE, amd64R11)
	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
	amd64MOV_reg_mem32(cb, amd64RDX, amd64RSP, 36)
	m68kEmitSMCInvalidateRangeCheck(cb, amd64RDX, M68K_SIZE_BYTE)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitPACKUNPKRegister(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+4 > uint32(len(memory)) || !m68kIsNativeSupportedPACKUNPK(opcode) || ((opcode>>3)&1) != 0 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	rx := opcode & 7
	ry := (opcode >> 9) & 7
	adjustment := uint32(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3]))
	src := m68kResolveDataReg(cb, rx, amd64RAX)
	if src != amd64RAX {
		amd64MOV_reg_reg32(cb, amd64RAX, src)
	}

	if opcode&0xF1F0 == 0x8140 { // PACK Dx,Dy,#adj
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		amd64SHR_imm(cb, amd64RDX, 8)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0x0F)
		amd64SHL_imm(cb, amd64RDX, 4)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x0F)
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RDX)
		if adjustment != 0 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(adjustment))
		}
		m68kStoreDataRegByte(cb, ry, amd64RAX)
		return
	}

	// UNPK Dx,Dy,#adj
	amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0xF0)
	amd64SHL_imm(cb, amd64RDX, 4)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x0F)
	amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RDX)
	if adjustment != 0 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(adjustment))
	}
	m68kStoreDataRegWord(cb, ry, amd64RAX, amd64RDX)
}

func m68kEmitPACKUNPKPredecMem(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+4 > uint32(len(memory)) || !m68kIsNativeSupportedPACKUNPK(opcode) || ((opcode>>3)&1) != 1 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	rx := opcode & 7
	ry := (opcode >> 9) & 7
	isPack := opcode&0xF1F0 == 0x8140
	adjustment := uint32(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3]))

	srcAR := m68kResolveAddrReg(cb, rx, amd64R10)
	if srcAR != amd64R10 {
		amd64MOV_reg_reg32(cb, amd64R10, srcAR)
	}
	if isPack {
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, M68K_WORD_SIZE)
	} else {
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, M68K_BYTE_SIZE)
	}

	if rx == ry {
		amd64MOV_reg_reg32(cb, amd64RCX, amd64R10)
	} else {
		dstAR := m68kResolveAddrReg(cb, ry, amd64RCX)
		if dstAR != amd64RCX {
			amd64MOV_reg_reg32(cb, amd64RCX, dstAR)
		}
	}
	if isPack {
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, M68K_BYTE_SIZE)
	} else {
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, M68K_WORD_SIZE)
	}

	bailOffs := []int{}
	amd64MOV_mem_reg32(cb, amd64RSP, 48, amd64R10)
	amd64MOV_mem_reg32(cb, amd64RSP, 52, amd64RCX)
	amd64MOV_reg_imm32(cb, amd64RDX, M68K_WORD_SIZE)
	if !isPack {
		amd64MOV_reg_imm32(cb, amd64RDX, M68K_BYTE_SIZE)
	}
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 52)
	amd64MOV_reg_imm32(cb, amd64RDX, M68K_BYTE_SIZE)
	if !isPack {
		amd64MOV_reg_imm32(cb, amd64RDX, M68K_WORD_SIZE)
	}
	m68kEmitMemRangeBailChecks(cb, amd64RCX, amd64RDX, &bailOffs)
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 52)
	amd64MOV_reg_imm32(cb, amd64RDX, M68K_BYTE_SIZE)
	if !isPack {
		amd64MOV_reg_imm32(cb, amd64RDX, M68K_WORD_SIZE)
	}
	m68kEmitSMCRangeBailChecks(cb, amd64RCX, amd64RDX, &bailOffs)
	amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 48)
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 52)

	if rx == ry {
		m68kStoreAddrReg(cb, rx, amd64RCX)
	} else {
		m68kStoreAddrReg(cb, rx, amd64R10)
		amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 52)
		m68kStoreAddrReg(cb, ry, amd64RCX)
	}

	if isPack {
		m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_WORD)
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		amd64SHR_imm(cb, amd64RDX, 8)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0x0F)
		amd64SHL_imm(cb, amd64RDX, 4)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x0F)
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RDX)
		if adjustment != 0 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(adjustment))
		}
		amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 52)
		m68kEmitStoreDirectRAM(cb, amd64RCX, amd64RAX, M68K_SIZE_BYTE)
		m68kEmitSMCInvalidateRangeCheck(cb, amd64RCX, M68K_SIZE_BYTE)
		m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
		return
	}

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_BYTE)
	amd64MOVZX_B(cb, amd64RAX, amd64RAX)
	amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0xF0)
	amd64SHL_imm(cb, amd64RDX, 4)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x0F)
	amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RDX)
	if adjustment != 0 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(adjustment))
	}
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 52)
	m68kEmitStoreDirectRAM(cb, amd64RCX, amd64RAX, M68K_SIZE_WORD)
	m68kEmitSMCInvalidateRangeCheck(cb, amd64RCX, M68K_SIZE_WORD)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
}

func m68kEmitTSTAnDisp(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	if size == 3 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	bailOffs, ok := m68kEmitDispAnReadPrecheck(cb, ji, memory, startPC, size, 2, br, instrIdx)
	if !ok {
		return
	}

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, size)
	m68kEmitSizedLogicTest(cb, amd64RAX, size, amd64RDX)
	if cs := m68kCurrentCS; cs != nil {
		oldCS := m68kCurrentCS
		m68kCurrentCS = nil
		emitCCR_Logic(cb)
		m68kCurrentCS = oldCS
		cs.flagState = flagsMaterialized
	} else {
		emitCCR_Logic(cb)
	}
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitBTSTImmDn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	reg := opcode & 7
	if instrPC+4 > uint32(len(memory)) {
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return
	}
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}
	bit := (uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])) & 31
	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, ^int32(m68kCCR_Z))
	r := m68kResolveDataReg(cb, reg, amd64RAX)
	if r != amd64RAX {
		amd64MOV_reg_reg32(cb, amd64RAX, r)
	}
	if bit != 0 {
		amd64SHR_imm32(cb, amd64RAX, byte(bit))
	}
	amd64TEST_reg_imm8(cb, amd64RAX, 1)
	bitSetOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	patchRel32(cb, bitSetOff, cb.Len())
}

func m68kEmitBTSTImmAnDisp(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset
	if instrPC+6 > uint32(len(memory)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	bailOffs, ok := m68kEmitDispAnReadPrecheck(cb, ji, memory, startPC, M68K_SIZE_BYTE, 4, br, instrIdx)
	if !ok {
		return
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}
	bit := (uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])) & 7
	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_BYTE)
	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, ^int32(m68kCCR_Z))
	if bit != 0 {
		amd64SHR_imm32(cb, amd64RAX, byte(bit))
	}
	amd64TEST_reg_imm8(cb, amd64RAX, 1)
	bitSetOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	patchRel32(cb, bitSetOff, cb.Len())
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitBTST(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedBTST(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	mode := (opcode >> 3) & 7
	reg := opcode & 7
	isImmediate := m68kIsBTSTImmediate(opcode)

	valueSize := M68K_SIZE_BYTE
	bitMask := uint32(7)
	if mode == 0 {
		valueSize = M68K_SIZE_LONG
		bitMask = 31
	}

	if mode == 0 {
		if isImmediate {
			ext := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
			bit := byte(uint32(ext) & bitMask)
			m68kEmitReadSourceEA(cb, mode, reg, valueSize, memory, instrPC+4, instrPC, amd64RAX)
			if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
				m68kMaterializeCCR(cb, cs)
				cs.flagState = flagsMaterialized
			}
			amd64BTRegImm32(cb, amd64RAX, bit)
		} else {
			bitReg := (opcode >> 9) & 7
			m68kEmitReadSourceEA(cb, mode, reg, valueSize, memory, instrPC+2, instrPC, amd64RAX)
			src := m68kResolveDataReg(cb, bitReg, amd64RCX)
			if src != amd64RCX {
				amd64MOV_reg_reg32(cb, amd64RCX, src)
			}
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, int32(bitMask))
			if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
				m68kMaterializeCCR(cb, cs)
				cs.flagState = flagsMaterialized
			}
			amd64BitRegReg32(cb, 0xA3, amd64RAX, amd64RCX) // BT r/m32,r32
		}
		m68kEmitBitCCRFromCarry(cb)
		return
	}

	if cs := m68kCurrentCS; cs != nil {
		if cs.flagState != flagsMaterialized {
			m68kMaterializeCCR(cb, cs)
		}
		cs.flagState = flagsMaterialized
	}

	extPC := instrPC + 2
	if isImmediate {
		extPC = instrPC + 4
	}
	bailOffs := []int{}
	switch {
	case mode == 3: // (An)+
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case mode == 4: // -(An)
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(M68K_SIZE_BYTE, reg)))
	case mode == 6 || (mode == 7 && reg == 3):
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
	}

	amd64MOV_reg_imm32(cb, amd64RDX, 1)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_BYTE)

	if isImmediate {
		ext := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
		amd64BTRegImm32(cb, amd64RAX, byte(uint32(ext)&bitMask))
	} else {
		bitReg := (opcode >> 9) & 7
		src := m68kResolveDataReg(cb, bitReg, amd64RCX)
		if src != amd64RCX {
			amd64MOV_reg_reg32(cb, amd64RCX, src)
		}
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, int32(bitMask))
		amd64BitRegReg32(cb, 0xA3, amd64RAX, amd64RCX) // BT r/m32,r32
	}

	m68kEmitBitCCRFromCarry(cb)
	switch mode {
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(M68K_SIZE_BYTE, reg)))
		m68kStoreAddrReg(cb, reg, ar)
	case 4:
		m68kStoreAddrReg(cb, reg, amd64R10)
	}
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func amd64BitRegReg32(cb *CodeBuffer, opcode, base, bitReg byte) {
	emitREX(cb, false, bitReg, base)
	cb.EmitBytes(0x0F, opcode, modRM(3, bitReg, base))
}

func amd64BTRegImm32(cb *CodeBuffer, reg byte, bit byte) {
	emitREX(cb, false, 4, reg)
	cb.EmitBytes(0x0F, 0xBA, modRM(3, 4, reg), bit)
}

func m68kBitModifyRegRegOpcode(opcode uint16) byte {
	if m68kIsBitModifyImmediate(opcode) {
		switch opcode & 0x00C0 {
		case 0x0040: // BCHG #n,<ea>
			return 0xBB // BTC r/m32,r32
		case 0x0080: // BCLR #n,<ea>
			return 0xB3 // BTR r/m32,r32
		case 0x00C0: // BSET #n,<ea>
			return 0xAB // BTS r/m32,r32
		}
		return 0
	}
	switch opcode & 0xF1C0 {
	case 0x0140: // BCHG Dn,<ea>
		return 0xBB // BTC r/m32,r32
	case 0x0180: // BCLR Dn,<ea>
		return 0xB3 // BTR r/m32,r32
	case 0x01C0: // BSET Dn,<ea>
		return 0xAB // BTS r/m32,r32
	default:
		return 0
	}
}

func m68kEmitBitCCRFromCarry(cb *CodeBuffer) {
	amd64SETcc(cb, amd64CondB, amd64R11)
	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, ^int32(m68kCCR_Z))
	amd64TEST_reg_imm8(cb, amd64R11, 1)
	bitSetOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	patchRel32(cb, bitSetOff, cb.Len())
}

func m68kFullIndexDisp(memory []byte, pc uint32, long bool) (uint32, uint32, bool) {
	if long {
		if pc+4 > uint32(len(memory)) {
			return 0, pc, false
		}
		val := uint32(memory[pc])<<24 | uint32(memory[pc+1])<<16 |
			uint32(memory[pc+2])<<8 | uint32(memory[pc+3])
		return val, pc + 4, true
	}
	if pc+2 > uint32(len(memory)) {
		return 0, pc, false
	}
	val := uint32(int32(int16(uint16(memory[pc])<<8 | uint16(memory[pc+1]))))
	return val, pc + 2, true
}

func m68kEmitFullIndexAddIndex(cb *CodeBuffer, extWord uint16, dstReg byte) {
	if (extWord>>M68K_EXT_IS_BIT)&1 != 0 {
		return
	}
	idxReg := (extWord >> 12) & 0x07
	idxType := (extWord >> M68K_EXT_REG_TYPE_BIT) & 0x01
	idxSize := (extWord >> M68K_EXT_SIZE_BIT) & 0x01
	scale := (extWord >> M68K_EXT_SCALE_START_BIT) & ((1 << M68K_EXT_SCALE_SIZE) - 1)

	if idxType == M68K_EXT_DATA_REG_TYPE {
		idx := m68kResolveDataReg(cb, idxReg, amd64RCX)
		if idx != amd64RCX {
			amd64MOV_reg_reg32(cb, amd64RCX, idx)
		}
	} else {
		idx := m68kResolveAddrReg(cb, idxReg, amd64RCX)
		if idx != amd64RCX {
			amd64MOV_reg_reg32(cb, amd64RCX, idx)
		}
	}
	if idxSize == 0 {
		amd64MOVSX_W(cb, amd64RCX, amd64RCX)
	}
	if scale > 0 {
		amd64SHL_imm(cb, amd64RCX, byte(scale))
	}
	amd64ALU_reg_reg32(cb, 0x01, dstReg, amd64RCX)
}

func m68kEmitFullIndexIndirectRead32(cb *CodeBuffer, addrReg byte, bailOffs *[]int) {
	amd64MOV_reg_imm32(cb, amd64RDX, 4)
	m68kEmitMemRangeBailChecks(cb, addrReg, amd64RDX, bailOffs)
	m68kEmitLoadDirectRAM(cb, addrReg, addrReg, M68K_SIZE_LONG)
}

func m68kEmitComputeFullIndexEA(cb *CodeBuffer, mode, reg uint16, memory []byte, extPC uint32, dstReg byte, bailOffs *[]int) bool {
	if extPC+2 > uint32(len(memory)) {
		return false
	}
	extWord := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
	if extWord&M68K_EXT_FULL_FORMAT == 0 {
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, extPC, dstReg)
		return true
	}

	bs := (extWord >> M68K_EXT_BS_BIT) & 0x01
	bd := (extWord >> M68K_EXT_BD_START_BIT) & ((1 << M68K_EXT_BD_SIZE) - 1)
	iis := extWord & M68K_EXT_INDIRECTION_MASK
	cursor := extPC + 2

	if bs == 0 {
		if mode == 6 {
			base := m68kResolveAddrReg(cb, reg, dstReg)
			if base != dstReg {
				amd64MOV_reg_reg32(cb, dstReg, base)
			}
		} else if mode == 7 && reg == 3 {
			amd64MOV_reg_imm32(cb, dstReg, extPC)
		} else {
			return false
		}
	} else {
		amd64MOV_reg_imm32(cb, dstReg, 0)
	}

	switch bd {
	case 2:
		disp, next, ok := m68kFullIndexDisp(memory, cursor, false)
		if !ok {
			return false
		}
		cursor = next
		amd64ALU_reg_imm32_32bit(cb, 0, dstReg, int32(disp))
	case 3:
		disp, next, ok := m68kFullIndexDisp(memory, cursor, true)
		if !ok {
			return false
		}
		cursor = next
		amd64ALU_reg_imm32_32bit(cb, 0, dstReg, int32(disp))
	}

	switch {
	case iis == 0 || iis == 4:
		m68kEmitFullIndexAddIndex(cb, extWord, dstReg)
		return true

	case iis <= 3:
		m68kEmitFullIndexAddIndex(cb, extWord, dstReg)
		m68kEmitFullIndexIndirectRead32(cb, dstReg, bailOffs)
		switch iis {
		case 2:
			disp, next, ok := m68kFullIndexDisp(memory, cursor, false)
			if !ok {
				return false
			}
			cursor = next
			amd64ALU_reg_imm32_32bit(cb, 0, dstReg, int32(disp))
		case 3:
			disp, next, ok := m68kFullIndexDisp(memory, cursor, true)
			if !ok {
				return false
			}
			cursor = next
			amd64ALU_reg_imm32_32bit(cb, 0, dstReg, int32(disp))
		}
		return true

	case iis >= 5:
		m68kEmitFullIndexIndirectRead32(cb, dstReg, bailOffs)
		m68kEmitFullIndexAddIndex(cb, extWord, dstReg)
		switch iis {
		case 6:
			disp, next, ok := m68kFullIndexDisp(memory, cursor, false)
			if !ok {
				return false
			}
			cursor = next
			amd64ALU_reg_imm32_32bit(cb, 0, dstReg, int32(disp))
		case 7:
			disp, next, ok := m68kFullIndexDisp(memory, cursor, true)
			if !ok {
				return false
			}
			cursor = next
			amd64ALU_reg_imm32_32bit(cb, 0, dstReg, int32(disp))
		}
		return true
	}

	return false
}

func m68kEmitBitModify(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedBitModify(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	mode := (opcode >> 3) & 7
	reg := opcode & 7
	isImmediate := m68kIsBitModifyImmediate(opcode)
	op := m68kBitModifyRegRegOpcode(opcode)
	if op == 0 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if cs := m68kCurrentCS; cs != nil {
		if cs.flagState != flagsMaterialized {
			m68kMaterializeCCR(cb, cs)
		}
		cs.flagState = flagsMaterialized
	}

	loadBitReg := func(mask uint32) {
		if isImmediate {
			ext := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
			amd64MOV_reg_imm32(cb, amd64RCX, uint32(ext)&mask)
			return
		}
		bitReg := (opcode >> 9) & 7
		src := m68kResolveDataReg(cb, bitReg, amd64RCX)
		if src != amd64RCX {
			amd64MOV_reg_reg32(cb, amd64RCX, src)
		}
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, int32(mask))
	}

	if mode == 0 {
		dst := m68kResolveDataReg(cb, reg, amd64RDX)
		loadBitReg(31)
		amd64BitRegReg32(cb, op, dst, amd64RCX)
		m68kEmitBitCCRFromCarry(cb)
		m68kStoreDataReg(cb, reg, dst)
		return
	}

	extPC := instrPC + 2
	if isImmediate {
		extPC = instrPC + 4
	}
	bailOffs := []int{}
	switch mode {
	case 3: // (An)+
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case 4: // -(An)
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(M68K_SIZE_BYTE, reg)))
	case 6:
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, amd64R10)
	}

	amd64MOV_reg_imm32(cb, amd64RDX, 1)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, 1)
	m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	loadBitReg(7)
	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_BYTE)
	amd64BitRegReg32(cb, op, amd64RAX, amd64RCX)
	m68kEmitBitCCRFromCarry(cb)
	emitMemOpSIB(cb, false, 0x88, amd64RAX, m68kAMD64RegMemBase, amd64R10, 0)
	switch mode {
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(M68K_SIZE_BYTE, reg)))
		m68kStoreAddrReg(cb, reg, ar)
	case 4:
		m68kStoreAddrReg(cb, reg, amd64R10)
	}
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitADDXSUBX_Dn_Dn(cb *CodeBuffer, opcode uint16, subtract bool) {
	size := int((opcode >> 6) & 3)
	regMode := (opcode >> 3) & 1
	if size == 3 || regMode != 0 {
		return
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	src := m68kResolveDataReg(cb, srcReg, amd64RAX)
	dst := m68kResolveDataReg(cb, dstReg, amd64RDX)

	amd64MOV_reg_reg32(cb, amd64R11, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64R11, 2)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 1)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64R11)

	amd64BTRegImm32(cb, m68kAMD64RegCCR, 4)
	if subtract {
		m68kEmitSizedALURegReg(cb, 0x18, 0x19, dst, src, size) // SBB dst,src
	} else {
		m68kEmitSizedALURegReg(cb, 0x10, 0x11, dst, src, size) // ADC dst,src
	}
	m68kStoreDataReg(cb, dstReg, dst)

	amd64SETcc(cb, amd64CondB, amd64RCX) // C
	amd64SETcc(cb, amd64CondO, amd64RDX) // V
	amd64SETcc(cb, amd64CondE, amd64R10) // result zero
	amd64SETcc(cb, 0x8, amd64R11)        // N

	amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX)

	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // X = C

	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 1)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOVZX_B(cb, amd64RAX, amd64R10)
	amd64MOV_reg_mem32(cb, amd64RDX, amd64RSP, 32)
	amd64ALU_reg_reg32(cb, 0x21, amd64RAX, amd64RDX) // Z = resultZero && oldZ
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
}

func m68kEmitADDXSUBX_PredecMem(cb *CodeBuffer, ji *M68KJITInstr, startPC uint32, br *m68kBlockRegs, instrIdx int, subtract bool) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	regMode := (opcode >> 3) & 1
	if size == 3 || regMode != 1 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	rx := opcode & 7
	ry := (opcode >> 9) & 7
	stepX := int32(m68kStepSize(size, rx))
	stepY := int32(m68kStepSize(size, ry))

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	srcAR := m68kResolveAddrReg(cb, rx, amd64R10)
	if srcAR != amd64R10 {
		amd64MOV_reg_reg32(cb, amd64R10, srcAR)
	}
	amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, stepX)

	if rx == ry {
		amd64MOV_reg_reg32(cb, amd64RCX, amd64R10)
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, stepY)
	} else {
		dstAR := m68kResolveAddrReg(cb, ry, amd64RCX)
		if dstAR != amd64RCX {
			amd64MOV_reg_reg32(cb, amd64RCX, dstAR)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, stepY)
	}

	bailOffs := []int{}
	if size != M68K_SIZE_BYTE {
		amd64TEST_reg_imm8(cb, amd64R10, 1)
		bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
		amd64TEST_reg_imm8(cb, amd64RCX, 1)
		bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	}
	byteLen := uint32(1 << uint(size))
	amd64MOV_mem_reg32(cb, amd64RSP, 48, amd64RCX)
	amd64MOV_reg_imm32(cb, amd64RDX, byteLen)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 48)
	amd64MOV_reg_imm32(cb, amd64RDX, byteLen)
	m68kEmitMemRangeBailChecks(cb, amd64RCX, amd64RDX, &bailOffs)
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 48)
	amd64MOV_reg_imm32(cb, amd64RDX, byteLen)
	m68kEmitSMCRangeBailChecks(cb, amd64RCX, amd64RDX, &bailOffs)
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 48)

	m68kStoreAddrReg(cb, rx, amd64R10)
	m68kStoreAddrReg(cb, ry, amd64RCX)
	amd64MOV_mem_reg32(cb, amd64RSP, 36, amd64RCX)

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RDX, size)
	m68kEmitLoadDirectRAM(cb, amd64RCX, amd64RAX, size)
	if size == M68K_SIZE_BYTE {
		amd64MOVZX_B(cb, amd64RDX, amd64RDX)
		amd64MOVZX_B(cb, amd64RAX, amd64RAX)
	} else if size == M68K_SIZE_WORD {
		amd64MOVZX_W(cb, amd64RDX, amd64RDX)
		amd64MOVZX_W(cb, amd64RAX, amd64RAX)
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RAX)
		amd64MOV_mem_reg32(cb, amd64RSP, 40, amd64RDX)
		m68kMaterializeCCR(cb, cs)
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
		amd64MOV_reg_mem32(cb, amd64RDX, amd64RSP, 40)
	}

	amd64MOV_reg_reg32(cb, amd64R11, m68kAMD64RegCCR)
	amd64SHR_imm(cb, amd64R11, 2)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 1)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64R11)

	amd64BTRegImm32(cb, m68kAMD64RegCCR, 4)
	if subtract {
		m68kEmitSizedALURegReg(cb, 0x18, 0x19, amd64RAX, amd64RDX, size) // SBB dst,src
	} else {
		m68kEmitSizedALURegReg(cb, 0x10, 0x11, amd64RAX, amd64RDX, size) // ADC dst,src
	}

	amd64SETcc(cb, amd64CondB, amd64RCX) // C
	amd64SETcc(cb, amd64CondO, amd64RDX) // V
	amd64SETcc(cb, amd64CondE, amd64R10) // result zero
	amd64SETcc(cb, 0x8, amd64R11)        // N
	amd64MOV_mem_reg32(cb, amd64RSP, 40, amd64R11)
	amd64MOV_mem_reg32(cb, amd64RSP, 44, amd64RCX)
	amd64MOV_mem_reg32(cb, amd64RSP, 48, amd64RDX)
	amd64MOV_mem_reg32(cb, amd64RSP, 52, amd64R10)

	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 36)
	m68kEmitStoreDirectRAM(cb, amd64RCX, amd64RAX, size)

	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 44)
	amd64MOV_reg_mem32(cb, amd64RDX, amd64RSP, 48)
	amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 52)
	amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX)

	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // X = C

	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 1)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOVZX_B(cb, amd64RAX, amd64R10)
	amd64MOV_reg_mem32(cb, amd64RDX, amd64RSP, 32)
	amd64ALU_reg_reg32(cb, 0x21, amd64RAX, amd64RDX) // Z = resultZero && oldZ
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOV_reg_mem32(cb, amd64R11, amd64RSP, 40)
	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 36)
	m68kEmitSMCInvalidateRangeCheck(cb, amd64RCX, size)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitSizedALURegImmFull(cb *CodeBuffer, aluOp byte, dst byte, imm uint32, size int) {
	switch size {
	case M68K_SIZE_BYTE:
		emitREXForByte(cb, aluOp, dst)
		cb.EmitBytes(0x80, modRM(3, aluOp, dst), byte(imm))
	case M68K_SIZE_WORD:
		cb.EmitBytes(0x66)
		emitREX(cb, false, 0, dst)
		cb.EmitBytes(0x81, modRM(3, aluOp, dst))
		cb.EmitBytes(byte(imm), byte(imm>>8))
	default:
		emitREX(cb, false, 0, dst)
		cb.EmitBytes(0x81, modRM(3, aluOp, dst))
		cb.Emit32(imm)
	}
}

func m68kEmitImmediateLogicDn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	immBytes := m68kImmediateBytes(size)
	if size == 3 || instrPC+2+uint32(immBytes) > uint32(len(memory)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	immPC := instrPC + 2
	var imm uint32
	switch size {
	case M68K_SIZE_BYTE:
		imm = uint32(memory[immPC+1])
	case M68K_SIZE_WORD:
		imm = uint32(uint16(memory[immPC])<<8 | uint16(memory[immPC+1]))
	case M68K_SIZE_LONG:
		imm = uint32(memory[immPC])<<24 | uint32(memory[immPC+1])<<16 |
			uint32(memory[immPC+2])<<8 | uint32(memory[immPC+3])
	}
	reg := opcode & 7
	r := m68kResolveDataReg(cb, reg, amd64RAX)
	switch opcode & 0xFF00 {
	case 0x0000: // ORI #imm,Dn
		m68kEmitSizedALURegImmFull(cb, 1, r, imm, size)
	case 0x0200: // ANDI #imm,Dn
		m68kEmitSizedALURegImmFull(cb, 4, r, imm, size)
	case 0x0A00: // EORI #imm,Dn
		m68kEmitSizedALURegImmFull(cb, 6, r, imm, size)
	}
	emitCCR_LogicPreserveVC(cb)
	m68kStoreDataReg(cb, reg, r)
}

func m68kEmitImmediateLogicEA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	immBytes := m68kImmediateBytes(size)
	if size == 3 || instrPC+2+uint32(immBytes) > uint32(len(memory)) || !m68kIsNativeSupportedImmediateLogicEA(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	immPC := instrPC + 2
	var imm uint32
	switch size {
	case M68K_SIZE_BYTE:
		imm = uint32(memory[immPC+1])
	case M68K_SIZE_WORD:
		imm = uint32(uint16(memory[immPC])<<8 | uint16(memory[immPC+1]))
	case M68K_SIZE_LONG:
		imm = uint32(memory[immPC])<<24 | uint32(memory[immPC+1])<<16 |
			uint32(memory[immPC+2])<<8 | uint32(memory[immPC+3])
	}

	mode := (opcode >> 3) & 7
	reg := opcode & 7
	eaExtPC := immPC + uint32(immBytes)
	mutatingDest := mode == 3 || mode == 4
	if mutatingDest {
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		if mode == 4 {
			amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, reg)))
		}
		bail := 0
		m68kEmitMemRead(cb, amd64R10, amd64RAX, size, &bail)
	} else {
		m68kEmitReadSourceEA(cb, mode, reg, size, memory, eaExtPC, instrPC, amd64RAX)
	}
	if m68kEAMayUseMemHelper(mode, reg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}

	switch opcode & 0xFF00 {
	case 0x0000: // ORI #imm,<ea>
		m68kEmitSizedALURegImmFull(cb, 1, amd64RAX, imm, size)
	case 0x0200: // ANDI #imm,<ea>
		m68kEmitSizedALURegImmFull(cb, 4, amd64RAX, imm, size)
	case 0x0A00: // EORI #imm,<ea>
		m68kEmitSizedALURegImmFull(cb, 6, amd64RAX, imm, size)
	}
	emitCCR_LogicPreserveVC(cb)
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RAX)
		amd64MOV_mem_reg32(cb, amd64RSP, 36, amd64R10)
		m68kMaterializeCCR(cb, cs)
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
		amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 36)
	}

	bail := 0
	m68kEmitMemWrite(cb, amd64R10, amd64RAX, size, &bail)
	if m68kEAMayUseMemHelper(mode, reg, true) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		writeOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, writeOKOff, cb.Len())
	}
	if mutatingDest {
		if mode == 3 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(m68kStepSize(size, reg)))
		}
		m68kStoreAddrReg(cb, reg, amd64R10)
	}
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
}

func m68kEmitImmediateArithmeticDn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	immBytes := m68kImmediateBytes(size)
	if size == 3 || instrPC+2+uint32(immBytes) > uint32(len(memory)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	immPC := instrPC + 2
	var imm uint32
	switch size {
	case M68K_SIZE_BYTE:
		imm = uint32(memory[immPC+1])
	case M68K_SIZE_WORD:
		imm = uint32(uint16(memory[immPC])<<8 | uint16(memory[immPC+1]))
	case M68K_SIZE_LONG:
		imm = uint32(memory[immPC])<<24 | uint32(memory[immPC+1])<<16 |
			uint32(memory[immPC+2])<<8 | uint32(memory[immPC+3])
	}

	reg := opcode & 7
	r := m68kResolveDataReg(cb, reg, amd64RAX)
	switch opcode & 0xFF00 {
	case 0x0400: // SUBI #imm,Dn
		m68kEmitSizedALURegImmFull(cb, 5, r, imm, size)
	case 0x0600: // ADDI #imm,Dn
		m68kEmitSizedALURegImmFull(cb, 0, r, imm, size)
	}
	emitCCR_Arithmetic(cb)
	m68kStoreDataReg(cb, reg, r)
}

func m68kEmitImmediateArithmeticEA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	immBytes := m68kImmediateBytes(size)
	if size == 3 || instrPC+2+uint32(immBytes) > uint32(len(memory)) || !m68kIsNativeSupportedImmediateArithmeticEA(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	immPC := instrPC + 2
	var imm uint32
	switch size {
	case M68K_SIZE_BYTE:
		imm = uint32(memory[immPC+1])
	case M68K_SIZE_WORD:
		imm = uint32(uint16(memory[immPC])<<8 | uint16(memory[immPC+1]))
	case M68K_SIZE_LONG:
		imm = uint32(memory[immPC])<<24 | uint32(memory[immPC+1])<<16 |
			uint32(memory[immPC+2])<<8 | uint32(memory[immPC+3])
	}

	mode := (opcode >> 3) & 7
	reg := opcode & 7
	eaExtPC := immPC + uint32(immBytes)
	switch mode {
	case 5, 6:
		if eaExtPC+2 > uint32(len(memory)) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	case 7:
		switch reg {
		case 0:
			if eaExtPC+2 > uint32(len(memory)) {
				m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
				return
			}
		case 1:
			if eaExtPC+4 > uint32(len(memory)) {
				m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
				return
			}
		default:
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	}
	if mode == 6 && !m68kBriefIndexedEAAllowed(memory, eaExtPC, mode, reg) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	mutatingDest := mode == 3 || mode == 4
	if mutatingDest {
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		if mode == 4 {
			amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, reg)))
		}
		bail := 0
		m68kEmitMemRead(cb, amd64R10, amd64RAX, size, &bail)
	} else {
		m68kEmitReadSourceEA(cb, mode, reg, size, memory, eaExtPC, instrPC, amd64RAX)
	}
	if m68kEAMayUseMemHelper(mode, reg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}

	switch opcode & 0xFF00 {
	case 0x0400: // SUBI #imm,<ea>
		m68kEmitSizedALURegImmFull(cb, 5, amd64RAX, imm, size)
	case 0x0600: // ADDI #imm,<ea>
		m68kEmitSizedALURegImmFull(cb, 0, amd64RAX, imm, size)
	}
	emitCCR_Arithmetic(cb)
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RAX)
		amd64MOV_mem_reg32(cb, amd64RSP, 36, amd64R10)
		m68kMaterializeCCR(cb, cs)
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
		amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 36)
	}

	bail := 0
	m68kEmitMemWrite(cb, amd64R10, amd64RAX, size, &bail)
	if m68kEAMayUseMemHelper(mode, reg, true) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		writeOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, writeOKOff, cb.Len())
	}
	if mutatingDest {
		if mode == 3 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(m68kStepSize(size, reg)))
		}
		m68kStoreAddrReg(cb, reg, amd64R10)
	}
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
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
	flagSize := M68K_SIZE_LONG

	switch opmode {
	case 2: // EXT.W — sign-extend byte to word
		// MOVSX r16, r8 then mask to 16-bit
		// Actually: MOVSX EAX, AL then MOVZX to word
		if r != amd64RAX {
			amd64MOV_reg_reg32(cb, amd64RAX, r)
		}
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX) // save original upper word before sign-extension
		// CBW equivalent: MOVSX EAX, AL
		cb.EmitBytes(0x0F, 0xBE, modRM(3, amd64RAX, amd64RAX)) // MOVSX EAX, AL
		amd64MOVZX_W(cb, amd64RAX, amd64RAX)                   // zero-extend to 32 keeping sign-extended word
		// Write back only the low word
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, -65536) // mask upper word (0xFFFF0000)
		amd64ALU_reg_reg32(cb, 0x09, amd64RDX, amd64RAX)  // OR in sign-extended word
		m68kStoreDataReg(cb, reg, amd64RDX)
		r = amd64RAX // for flag test
		flagSize = M68K_SIZE_WORD

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
	m68kEmitSizedLogicTest(cb, r, flagSize, amd64RDX)
	emitCCR_Logic(cb)
}

func m68kEmitRawByteLoad(cb *CodeBuffer, dstReg, addrReg byte) {
	emitREX_SIB(cb, false, dstReg, addrReg, m68kAMD64RegMemBase)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, dstReg, 4), sibByte(0, addrReg, m68kAMD64RegMemBase))
}

func m68kEmitRawByteStore(cb *CodeBuffer, addrReg, valReg byte) {
	emitMemOpSIB(cb, false, 0x88, valReg, m68kAMD64RegMemBase, addrReg, 0)
}

func m68kEmitMOVEP(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+4 > uint32(len(memory)) || opcode&0xF138 != 0x0108 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
		cs.flagState = flagsMaterialized
	}

	opmode := (opcode >> 6) & 7
	if opmode < 4 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	dreg := (opcode >> 9) & 7
	areg := opcode & 7
	regToMem := opmode >= 6
	longSize := opmode&1 == 1
	span := uint32(3)
	if longSize {
		span = 7
	}

	disp := int32(int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])))
	ar := m68kResolveAddrReg(cb, areg, amd64R10)
	if ar != amd64R10 {
		amd64MOV_reg_reg32(cb, amd64R10, ar)
	}
	if disp != 0 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, disp)
	}

	bailOffs := make([]int, 0, 12)
	amd64MOV_reg_imm32(cb, amd64RDX, span)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	if regToMem {
		amd64MOV_reg_imm32(cb, amd64RDX, span)
		m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	}

	if regToMem {
		src := m68kResolveDataReg(cb, dreg, amd64RAX)
		if src != amd64RAX {
			amd64MOV_reg_reg32(cb, amd64RAX, src)
		}
		amd64MOV_reg_reg32(cb, amd64R11, amd64R10)

		shift := byte(8)
		if longSize {
			shift = 24
		}
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		amd64SHR_imm(cb, amd64RDX, shift)
		m68kEmitRawByteStore(cb, amd64R11, amd64RDX)

		amd64ALU_reg_imm32_32bit(cb, 0, amd64R11, 2)
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		if longSize {
			amd64SHR_imm(cb, amd64RDX, 16)
		}
		m68kEmitRawByteStore(cb, amd64R11, amd64RDX)

		if longSize {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R11, 2)
			amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
			amd64SHR_imm(cb, amd64RDX, 8)
			m68kEmitRawByteStore(cb, amd64R11, amd64RDX)

			amd64ALU_reg_imm32_32bit(cb, 0, amd64R11, 2)
			m68kEmitRawByteStore(cb, amd64R11, amd64RAX)
		}
	} else {
		m68kEmitRawByteLoad(cb, amd64RAX, amd64R10)
		if longSize {
			amd64SHL_imm(cb, amd64RAX, 24)
		} else {
			amd64SHL_imm(cb, amd64RAX, 8)
		}

		amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R11, 2)
		m68kEmitRawByteLoad(cb, amd64RCX, amd64R11)
		if longSize {
			amd64SHL_imm(cb, amd64RCX, 16)
		}
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)

		if longSize {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R11, 2)
			m68kEmitRawByteLoad(cb, amd64RCX, amd64R11)
			amd64SHL_imm(cb, amd64RCX, 8)
			amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)

			amd64ALU_reg_imm32_32bit(cb, 0, amd64R11, 2)
			m68kEmitRawByteLoad(cb, amd64RCX, amd64R11)
			amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)
			m68kStoreDataReg(cb, dreg, amd64RAX)
		} else {
			m68kStoreDataRegWord(cb, dreg, amd64RAX, amd64R11)
		}
	}

	doneOff := amd64JMP_rel32(cb)
	failLabel := cb.Len()
	for _, off := range bailOffs {
		patchRel32(cb, off, failLabel)
	}
	m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
	patchRel32(cb, doneOff, cb.Len())
}

// ===========================================================================
// Memory Access Helpers — Big-Endian Read/Write with I/O Bail
// ===========================================================================

func m68kAccessSizeBytes(size int) uint32 {
	switch size {
	case M68K_SIZE_BYTE:
		return 1
	case M68K_SIZE_WORD:
		return 2
	case M68K_SIZE_LONG:
		return 4
	default:
		return 1
	}
}

// m68kEmitMemRead emits code to read from memory at address in addrReg.
// Result goes into dstReg. Size determines byte-swap and width.
// On I/O bail: sets NeedIOFallback and jumps to bailLabel.
// Returns the offset of the bail Jcc for patching by the caller.
func m68kEmitMemAccessBailChecks(cb *CodeBuffer, addrReg byte, bailSites *[]int) {
	amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
	amd64ALU_reg_reg32(cb, 0x39, addrReg, amd64R11) // CMP addr, MemSize
	*bailSites = append(*bailSites, amd64Jcc_rel32(cb, amd64CondAE))

	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffIOPageBitmapPtr))
	amd64TEST_reg_reg32(cb, amd64RCX, amd64RCX)
	noBitmapOff := amd64Jcc_rel32(cb, amd64CondE)

	amd64MOV_reg_reg32(cb, amd64R11, addrReg)
	amd64SHR_imm32(cb, amd64R11, 8)
	amd64MOV_reg_mem32(cb, amd64RDX, m68kAMD64RegCtx, int32(m68kCtxOffIOPageBitmapLen))
	amd64ALU_reg_reg32(cb, 0x39, amd64R11, amd64RDX) // CMP page, bitmapLen
	noBitmapBoundsOff := amd64Jcc_rel32(cb, amd64CondAE)

	emitREX_SIB(cb, false, amd64R11, amd64R11, amd64RCX)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, amd64R11, 4), sibByte(0, amd64R11, amd64RCX))
	amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
	*bailSites = append(*bailSites, amd64Jcc_rel32(cb, amd64CondNE))
	patchRel32(cb, noBitmapOff, cb.Len())
	patchRel32(cb, noBitmapBoundsOff, cb.Len())
}

func m68kEmitMemRangeBailChecks(cb *CodeBuffer, startReg, countReg byte, bailSites *[]int) {
	amd64MOV_reg_reg32(cb, amd64RAX, startReg)
	amd64ALU_reg_reg32(cb, 0x01, amd64RAX, countReg) // end = start + count
	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, startReg) // CMP end, start
	*bailSites = append(*bailSites, amd64Jcc_rel32(cb, amd64CondB))

	amd64MOV_reg_mem32(cb, amd64RDX, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
	amd64ALU_reg_reg32(cb, 0x39, startReg, amd64RDX) // CMP start, MemSize
	*bailSites = append(*bailSites, amd64Jcc_rel32(cb, amd64CondAE))
	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, amd64RDX) // CMP end, MemSize
	*bailSites = append(*bailSites, amd64Jcc_rel32(cb, amd64CondA))

	// I/O page bitmap scan over [start,end). The old IOThreshold cutoff is
	// not valid for AROS high RAM; direct RAM above that cutoff must stay
	// native unless its page is actually marked as I/O.
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RAX)
	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffIOPageBitmapPtr))
	amd64TEST_reg_reg32(cb, amd64RCX, amd64RCX)
	noBitmapOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, 1)
	amd64SHR_imm32(cb, amd64RAX, 8)
	amd64MOV_reg_mem32(cb, amd64RDX, m68kAMD64RegCtx, int32(m68kCtxOffIOPageBitmapLen))
	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, amd64RDX) // CMP lastPage, bitmapLen
	noBitmapBoundsOff := amd64Jcc_rel32(cb, amd64CondAE)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RAX)
	amd64MOV_reg_reg32(cb, amd64RAX, startReg)
	amd64SHR_imm32(cb, amd64RAX, 8)
	ioLoopOff := cb.Len()
	cb.EmitBytes(0x3B, 0x44, 0x24, 0x20) // CMP EAX,[RSP+32]
	ioDoneOff := amd64Jcc_rel32(cb, amd64CondA)
	emitREX_SIB(cb, false, amd64RDX, amd64RAX, amd64RCX)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, amd64RDX, 4), sibByte(0, amd64RAX, amd64RCX))
	amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
	*bailSites = append(*bailSites, amd64Jcc_rel32(cb, amd64CondNE))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, 1)
	ioBackOff := amd64JMP_rel32(cb)
	patchRel32(cb, ioBackOff, ioLoopOff)
	patchRel32(cb, ioDoneOff, cb.Len())
	patchRel32(cb, noBitmapOff, cb.Len())
	patchRel32(cb, noBitmapBoundsOff, cb.Len())
}

func m68kEmitSMCRangeBailChecks(cb *CodeBuffer, startReg, countReg byte, bailSites *[]int) {
	amd64MOV_reg_reg32(cb, amd64RAX, startReg)
	amd64MOV_mem_reg32(cb, amd64RSP, 20, amd64RAX) // original write start for post-check store
	amd64ALU_reg_reg32(cb, 0x01, amd64RAX, countReg)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, 1)
	amd64SHR_imm(cb, amd64RAX, 12)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RAX) // inclusive end page
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 20)
	amd64SHR_imm(cb, amd64RAX, 12)
	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffCodePageBitmapPtr))
	amd64TEST_reg_reg(cb, amd64RCX, amd64RCX)
	noBitmapOff := amd64Jcc_rel32(cb, amd64CondE)
	smcLoopOff := cb.Len()
	cb.EmitBytes(0x3B, 0x44, 0x24, 0x20) // CMP EAX,[RSP+32]
	smcDoneOff := amd64Jcc_rel32(cb, amd64CondA)
	cb.EmitBytes(0x80, 0x3C, 0x01, 0x00) // CMP BYTE [RCX+RAX],0
	*bailSites = append(*bailSites, amd64Jcc_rel32(cb, amd64CondNE))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, 1)
	smcBackOff := amd64JMP_rel32(cb)
	patchRel32(cb, smcBackOff, smcLoopOff)
	patchRel32(cb, smcDoneOff, cb.Len())
	patchRel32(cb, noBitmapOff, cb.Len())
	amd64MOV_reg_mem32(cb, startReg, amd64RSP, 20)
}

func m68kEmitMemRead(cb *CodeBuffer, addrReg, dstReg byte, size int, bailLabel *int) {
	bailSites := make([]int, 0, 5)
	if size == M68K_SIZE_BYTE {
		m68kEmitMemAccessBailChecks(cb, addrReg, &bailSites)
	} else {
		amd64MOV_reg_imm32(cb, amd64RDX, m68kAccessSizeBytes(size))
		m68kEmitMemRangeBailChecks(cb, addrReg, amd64RDX, &bailSites)
	}

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
	for _, off := range bailSites {
		patchRel32(cb, off, cb.Len())
	}
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	// Caller will handle setting RetPC and emitting epilogue after this

	patchRel32(cb, doneOff, cb.Len())
	_ = doneOff // suppress unused
}

// m68kEmitMemWrite emits code to write a value to memory at address.
// addrReg has the address, valReg has the value to write.
func m68kEmitMemWrite(cb *CodeBuffer, addrReg, valReg byte, size int, bailLabel *int) {
	reloadValFromStack := false
	if size == M68K_SIZE_BYTE && (valReg == amd64RCX || valReg == amd64RDX || valReg == amd64R11) {
		amd64MOV_reg_reg32(cb, amd64RAX, valReg)
		valReg = amd64RAX
	}
	if valReg == amd64RAX || valReg == amd64RCX || valReg == amd64RDX || valReg == amd64R11 {
		amd64MOV_mem_reg32(cb, amd64RSP, 48, valReg)
		reloadValFromStack = true
	}

	bailSites := make([]int, 0, 5)
	if size == M68K_SIZE_BYTE {
		m68kEmitMemAccessBailChecks(cb, addrReg, &bailSites)
	} else {
		amd64MOV_reg_imm32(cb, amd64RDX, m68kAccessSizeBytes(size))
		m68kEmitMemRangeBailChecks(cb, addrReg, amd64RDX, &bailSites)
	}
	if reloadValFromStack {
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 48)
		valReg = amd64RAX
	}
	if watchAddr, ok := m68kJITNativeWatchWriteAddr(); ok {
		width := uint32(m68kAccessSizeBytes(size))
		low := watchAddr
		if width > 1 {
			delta := width - 1
			if watchAddr > delta {
				low = watchAddr - delta
			} else {
				low = 0
			}
		}
		amd64ALU_reg_imm32_32bit(cb, 7, addrReg, int32(low))
		noOverlapLowOff := amd64Jcc_rel32(cb, amd64CondB)
		amd64ALU_reg_imm32_32bit(cb, 7, addrReg, int32(watchAddr))
		noOverlapHighOff := amd64Jcc_rel32(cb, amd64CondA)
		bailSites = append(bailSites, amd64JMP_rel32(cb))
		patchRel32(cb, noOverlapLowOff, cb.Len())
		patchRel32(cb, noOverlapHighOff, cb.Len())
	}

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

	restoreAddrReg := false
	if addrReg == amd64R10 {
		amd64MOV_mem_reg32(cb, amd64RSP, 52, amd64R10)
		restoreAddrReg = true
	}
	m68kEmitSMCInvalidateRangeCheck(cb, addrReg, size)
	if restoreAddrReg {
		amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 52)
	}

	doneOff := amd64JMP_rel32(cb)

	for _, off := range bailSites {
		patchRel32(cb, off, cb.Len())
	}
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)

	patchRel32(cb, doneOff, cb.Len())
}

func m68kJITNativeWatchWriteAddr() (uint32, bool) {
	raw := strings.TrimSpace(os.Getenv("IE_M68K_JIT_WATCH_WRITE_ADDR"))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 0, 32)
	if err != nil {
		return 0, false
	}
	return uint32(value), true
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
		switch size {
		case M68K_SIZE_BYTE:
			amd64MOVZX_B(cb, dstReg, dstReg)
		case M68K_SIZE_WORD:
			amd64MOVZX_W(cb, dstReg, dstReg)
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
		undoOff := 0
		doneOff := 0
		if m68kEAMayUseMemHelper(4, reg, false) {
			amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
			undoOff = amd64Jcc_rel32(cb, amd64CondNE)
		}
		m68kStoreAddrReg(cb, reg, ar)
		if undoOff != 0 {
			doneOff = amd64JMP_rel32(cb)
			patchRel32(cb, undoOff, cb.Len())
			amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(step)) // undo predecrement before interpreter fallback
			patchRel32(cb, doneOff, cb.Len())
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

	case 6: // (d8,An,Xn) brief or 68020 full format
		indexBails := make([]int, 0, 4)
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &indexBails) {
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			return m68kIndexedExtBytes(memory, extPC)
		}
		bail := 0
		m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
		doneOff := amd64JMP_rel32(cb)
		for _, off := range indexBails {
			patchRel32(cb, off, cb.Len())
		}
		if len(indexBails) > 0 {
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		}
		patchRel32(cb, doneOff, cb.Len())
		return m68kIndexedExtBytes(memory, extPC)

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

		case 3: // (d8,PC,Xn) brief or 68020 full format
			indexBails := make([]int, 0, 4)
			if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, amd64R10, &indexBails) {
				amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
				return m68kIndexedExtBytes(memory, extPC)
			}
			bail := 0
			m68kEmitMemRead(cb, amd64R10, dstReg, size, &bail)
			doneOff := amd64JMP_rel32(cb)
			for _, off := range indexBails {
				patchRel32(cb, off, cb.Len())
			}
			if len(indexBails) > 0 {
				amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			}
			patchRel32(cb, doneOff, cb.Len())
			return m68kIndexedExtBytes(memory, extPC)

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

// m68kIsMemCopyLoopBlock returns true if the first two instructions match
// the canonical MemCopy loop shape:
//
//	MOVE.{B,W,L} (Ax)+,(Ay)+  ; instrs[0] at block start (pcOffset 0)
//	DBRA          Dn, instrs[0]  ; instrs[1], DBRA backward branch into MOVE
//
// On match, returns the source/destination address registers, the
// counter data register, the M68K size, and the chain-target PC (post-
// DBRA). Trailing instructions in the block (DBcc is not a block
// terminator on the 68K, so the scanner extends past DBRA) are
// intentionally ignored — the loop chain-exits to the post-DBRA PC where
// they form a fresh block compiled separately on demand.
//
// The MOVE must use distinct address registers so src and dst can occupy
// independent host registers in the unchecked body.
func m68kIsMemCopyLoopBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) (srcAn, dstAn, ctrDn uint16, size int, nextPC uint32, ok bool) {
	if len(instrs) < 2 {
		return
	}
	move := instrs[0]
	dbcc := instrs[1]
	if move.pcOffset != 0 {
		return
	}

	// MOVE.{B,W,L}: group is 1 (BYTE), 3 (WORD), or 2 (LONG); srcMode==3, dstMode==3.
	switch move.opcode >> 12 {
	case 0x1:
		size = M68K_SIZE_BYTE
	case 0x3:
		size = M68K_SIZE_WORD
	case 0x2:
		size = M68K_SIZE_LONG
	default:
		return
	}
	srcMode := (move.opcode >> 3) & 7
	dstMode := (move.opcode >> 6) & 7
	if srcMode != 3 || dstMode != 3 {
		return
	}
	sReg := move.opcode & 7
	dReg := (move.opcode >> 9) & 7
	if sReg == dReg {
		return
	}
	// A7 has special step rules (always +2 for byte ops). Skip — would
	// require alignment-aware scaling in the loop. Rare in MemCopy benches.
	if sReg == 7 || dReg == 7 {
		return
	}

	// DBRA opcode: 0x51C8 | reg (cond=1, mode=11001).
	if dbcc.opcode&0xFFF8 != 0x51C8 {
		return
	}
	// DBRA target = instrPC + 2 + dispWord. instrPC = startPC + dbcc.pcOffset.
	// Body loops back to instrs[0] when target == startPC.
	instrPC := startPC + dbcc.pcOffset
	if instrPC+4 > uint32(len(memory)) {
		return
	}
	dispWord := int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3]))
	target := uint32(int64(instrPC) + 2 + int64(dispWord))
	if target != startPC {
		return
	}

	srcAn = sReg
	dstAn = dReg
	ctrDn = dbcc.opcode & 7
	nextPC = instrPC + uint32(dbcc.length)
	ok = true
	return
}

func m68kIsAROSLongPostincFillLoopBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) (nextPC uint32, ok bool) {
	if len(instrs) < 7 {
		return
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x24C1 || // MOVE.L D1,(A2)+
		instrs[1].opcode != 0x2608 || // MOVE.L A0,D3
		instrs[2].opcode != 0x968A || // SUB.L A2,D3
		instrs[3].opcode != 0xD689 || // ADD.L A1,D3
		instrs[4].opcode != 0x7804 || // MOVEQ #4,D4
		instrs[5].opcode != 0xB883 { // CMP.L D3,D4
		return
	}
	bcs := instrs[6]
	if bcs.opcode>>8 != 0x65 { // BCS.S
		return
	}
	instrPC := startPC + bcs.pcOffset
	target := uint32(int64(instrPC+2) + int64(int8(bcs.opcode&0xFF)))
	if target != startPC {
		return
	}
	nextPC = instrPC + uint32(bcs.length)
	ok = true
	_ = memory
	return
}

func m68kIsBytePostincCountdownLoopBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) (nextPC uint32, ok bool) {
	if len(instrs) < 3 {
		return
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x10C0 || // MOVE.B D0,(A0)+
		instrs[1].opcode != 0x5381 { // SUBQ.L #1,D1
		return
	}
	bcc := instrs[2]
	if bcc.opcode>>8 != 0x64 { // BCC.S
		return
	}
	instrPC := startPC + bcc.pcOffset
	target := uint32(int64(instrPC+2) + int64(int8(bcc.opcode&0xFF)))
	if target != startPC {
		return
	}
	nextPC = instrPC + uint32(bcc.length)
	ok = true
	_ = memory
	return
}

func m68kIsAROSCMPIJSRBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) bool {
	if len(instrs) < 6 || memory == nil {
		return false
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x0C52 || // CMPI.W #$4AFC,(A2)
		instrs[1].opcode != 0x663C || // BNE.S skip
		instrs[2].opcode != 0xB5EA || // CMPA.L 2(A2),A2
		instrs[3].opcode != 0x6636 || // BNE.S skip
		instrs[4].opcode != 0x2C46 || // MOVEA.L D6,A6
		instrs[5].opcode != 0x4EAE { // JSR -132(A6)
		return false
	}

	readWord := func(pc uint32) (uint16, bool) {
		if pc+2 > uint32(len(memory)) {
			return 0, false
		}
		return uint16(memory[pc])<<8 | uint16(memory[pc+1]), true
	}
	imm, ok := readWord(startPC + instrs[0].pcOffset + 2)
	if !ok || imm != 0x4AFC {
		return false
	}
	cmpDisp, ok := readWord(startPC + instrs[2].pcOffset + 2)
	if !ok || cmpDisp != 0x0002 {
		return false
	}
	jsrDisp, ok := readWord(startPC + instrs[5].pcOffset + 2)
	return ok && jsrDisp == 0xFF7C
}

func m68kIsAROSStackLoadJSRBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) bool {
	if len(instrs) < 3 || memory == nil {
		return false
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x246F || // MOVEA.L 12(A7),A2
		instrs[1].opcode != 0x2C6F || // MOVEA.L 20(A7),A6
		instrs[2].opcode != 0x4EAE { // JSR -120(A6)
		return false
	}
	readWord := func(pc uint32) (uint16, bool) {
		if pc+2 > uint32(len(memory)) {
			return 0, false
		}
		return uint16(memory[pc])<<8 | uint16(memory[pc+1]), true
	}
	a2Disp, ok := readWord(startPC + instrs[0].pcOffset + 2)
	if !ok || a2Disp != 0x000C {
		return false
	}
	a6Disp, ok := readWord(startPC + instrs[1].pcOffset + 2)
	if !ok || a6Disp != 0x0014 {
		return false
	}
	jsrDisp, ok := readWord(startPC + instrs[2].pcOffset + 2)
	return ok && jsrDisp == 0xFF88
}

func m68kIsAROSStandaloneJSRBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) bool {
	if len(instrs) < 1 || memory == nil {
		return false
	}
	if instrs[0].pcOffset != 0 || instrs[0].opcode != 0x4EAE { // JSR d16(A6)
		return false
	}
	extPC := startPC + 2
	if extPC+2 > uint32(len(memory)) {
		return false
	}
	disp := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
	return int16(disp) < 0 && disp&1 == 0
}

func m68kIsStandaloneRTSBlock(instrs []M68KJITInstr) bool {
	return len(instrs) == 1 && instrs[0].pcOffset == 0 && instrs[0].opcode == 0x4E75
}

func m68kIsMOVEMPostincRTSBlock(instrs []M68KJITInstr) bool {
	return len(instrs) == 2 &&
		instrs[0].pcOffset == 0 &&
		instrs[0].opcode&0xFFFF == 0x4CDF &&
		instrs[1].opcode == 0x4E75
}

func m68kIsAROSMOVEMPrologueJSRBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) bool {
	if len(instrs) < 6 || memory == nil {
		return false
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x48E7 || // MOVEM.L reg-list,-(A7)
		instrs[1].opcode != 0x2449 || // MOVEA.L A1,A2
		instrs[2].opcode != 0x2400 || // MOVE.L D0,D2
		instrs[3].opcode != 0x264E || // MOVEA.L A6,A3
		instrs[4].opcode != 0x286E || // MOVEA.L d16(A6),A4
		instrs[5].opcode != 0x4EAE { // JSR d16(A6)
		return false
	}
	readWord := func(pc uint32) (uint16, bool) {
		if pc+2 > uint32(len(memory)) {
			return 0, false
		}
		return uint16(memory[pc])<<8 | uint16(memory[pc+1]), true
	}
	movemMask, ok := readWord(startPC + instrs[0].pcOffset + 2)
	if !ok || movemMask != 0x203A {
		return false
	}
	moveaDisp, ok := readWord(startPC + instrs[4].pcOffset + 2)
	if !ok || moveaDisp != 0x0114 {
		return false
	}
	jsrDisp, ok := readWord(startPC + instrs[5].pcOffset + 2)
	return ok && jsrDisp == 0xFF88
}

func m68kIsStackLoadAbsJSRBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) bool {
	if len(instrs) != 2 || memory == nil {
		return false
	}
	first := instrs[0].opcode
	if instrs[0].pcOffset != 0 || first>>12 != 0x2 { // MOVE.L
		return false
	}
	srcMode := (first >> 3) & 7
	srcReg := first & 7
	dstMode := (first >> 6) & 7
	if srcMode != 5 || srcReg != 7 || (dstMode != 0 && dstMode != 1) { // d16(A7) -> Dn/An
		return false
	}
	if instrs[1].opcode != 0x4EB9 { // JSR abs.L
		return false
	}
	extPC := startPC + instrs[1].pcOffset + 2
	return extPC+4 <= uint32(len(memory))
}

func m68kIsAROSStackCallWrapperBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) bool {
	if len(instrs) != 4 || memory == nil {
		return false
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x246F || // MOVEA.L 20(A7),A2
		instrs[1].opcode != 0x41EF || // LEA 8(A7),A0
		instrs[2].opcode != 0x2C6F || // MOVEA.L 24(A7),A6
		instrs[3].opcode != 0x4EAE { // JSR -66(A6)
		return false
	}
	readWord := func(pc uint32) (uint16, bool) {
		if pc+2 > uint32(len(memory)) {
			return 0, false
		}
		return uint16(memory[pc])<<8 | uint16(memory[pc+1]), true
	}
	a2Disp, ok := readWord(startPC + instrs[0].pcOffset + 2)
	if !ok || a2Disp != 0x0014 {
		return false
	}
	a0Disp, ok := readWord(startPC + instrs[1].pcOffset + 2)
	if !ok || a0Disp != 0x0008 {
		return false
	}
	a6Disp, ok := readWord(startPC + instrs[2].pcOffset + 2)
	if !ok || a6Disp != 0x0018 {
		return false
	}
	jsrDisp, ok := readWord(startPC + instrs[3].pcOffset + 2)
	return ok && jsrDisp == 0xFFBE
}

func m68kIsAROSAddStoreJSRBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) bool {
	if len(instrs) != 7 || memory == nil {
		return false
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x5480 || // ADDQ.L #2,D0
		instrs[1].opcode != 0x2540 || // MOVE.L D0,8(A2)
		instrs[2].opcode != 0x254B || // MOVE.L A3,12(A2)
		instrs[3].opcode != 0x91C8 || // SUBA.L A0,A0
		instrs[4].opcode != 0x43F9 || // LEA abs.L,A1
		instrs[5].opcode != 0x2C42 || // MOVEA.L D2,A6
		instrs[6].opcode != 0x4EAE { // JSR -30(A6)
		return false
	}
	readWord := func(pc uint32) (uint16, bool) {
		if pc+2 > uint32(len(memory)) {
			return 0, false
		}
		return uint16(memory[pc])<<8 | uint16(memory[pc+1]), true
	}
	d0Disp, ok := readWord(startPC + instrs[1].pcOffset + 2)
	if !ok || d0Disp != 0x0008 {
		return false
	}
	a3Disp, ok := readWord(startPC + instrs[2].pcOffset + 2)
	if !ok || a3Disp != 0x000C {
		return false
	}
	leaPC := startPC + instrs[4].pcOffset + 2
	if leaPC+4 > uint32(len(memory)) {
		return false
	}
	jsrDisp, ok := readWord(startPC + instrs[6].pcOffset + 2)
	return ok && jsrDisp == 0xFFE2
}

func m68kIsAROSIndexedLookupPrefix(instrs []M68KJITInstr, startPC uint32, memory []byte) (nextPC uint32, ok bool) {
	if len(instrs) < 6 || memory == nil {
		return
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x5381 || // SUBQ.L #1,D1
		instrs[1].opcode != 0xB081 || // CMP.L D1,D0
		instrs[2].opcode != 0x6302 || // BLS.S skip
		instrs[3].opcode != 0x2001 || // MOVE.L D1,D0
		instrs[4].opcode != 0x226D || // MOVEA.L 12(A5),A1
		instrs[5].opcode != 0x2231 { // MOVE.L 26(A1,D0.L*4),D1
		return
	}
	readWord := func(pc uint32) (uint16, bool) {
		if pc+2 > uint32(len(memory)) {
			return 0, false
		}
		return uint16(memory[pc])<<8 | uint16(memory[pc+1]), true
	}
	a1Disp, ok := readWord(startPC + instrs[4].pcOffset + 2)
	if !ok || a1Disp != 0x000C {
		return
	}
	idxExt, ok := readWord(startPC + instrs[5].pcOffset + 2)
	if !ok || idxExt != 0x0C1A {
		return
	}
	nextPC = startPC + instrs[5].pcOffset + uint32(instrs[5].length)
	ok = true
	return
}

func m68kIsSubqBCCMoveRTSBlock(instrs []M68KJITInstr, startPC uint32) bool {
	if len(instrs) != 4 {
		return false
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x5381 || // SUBQ.L #1,D1
		instrs[1].opcode != 0x64FA || // BCC.S back
		instrs[2].opcode != 0x2009 || // MOVE.L A1,D0
		instrs[3].opcode != 0x4E75 { // RTS
		return false
	}
	branchPC := startPC + instrs[1].pcOffset
	target := uint32(int64(branchPC+2) + int64(int8(instrs[1].opcode&0xFF)))
	return target == startPC-2
}

func m68kIsSubqLSRBNEStoreBRABlock(instrs []M68KJITInstr, startPC uint32) bool {
	if len(instrs) != 6 {
		return false
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x5381 || // SUBQ.L #1,D1
		instrs[1].opcode != 0xE289 || // LSR.L #1,D1
		instrs[2].opcode != 0x6606 || // BNE.S after store/BRA
		instrs[3].opcode != 0x72FF || // MOVEQ #-1,D1
		instrs[4].opcode != 0x2081 || // MOVE.L D1,(A0)
		instrs[5].opcode != 0x60C8 { // BRA.S back
		return false
	}
	bnePC := startPC + instrs[2].pcOffset
	bneTarget := uint32(int64(bnePC+2) + int64(int8(instrs[2].opcode&0xFF)))
	braPC := startPC + instrs[5].pcOffset
	braTarget := uint32(int64(braPC+2) + int64(int8(instrs[5].opcode&0xFF)))
	return bneTarget == startPC+12 && braTarget == startPC-44
}

func m68kIsSubqSubCmpBLSAddStoreBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) bool {
	if len(instrs) < 6 || memory == nil {
		return false
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x5381 || // SUBQ.L #1,D1
		instrs[1].opcode != 0x9283 || // SUB.L D3,D1
		instrs[2].opcode != 0xB282 || // CMP.L D2,D1
		instrs[3].opcode != 0x6314 || // BLS.S skip
		instrs[4].opcode != 0x5283 || // ADDQ.L #1,D3
		instrs[5].opcode != 0x2543 { // MOVE.L D3,44(A2)
		return false
	}
	extPC := startPC + instrs[5].pcOffset + 2
	if extPC+2 > uint32(len(memory)) {
		return false
	}
	disp := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
	return disp == 0x002C
}

func m68kIsStackCaseUpdateBlock(instrs []M68KJITInstr, startPC uint32, memory []byte) bool {
	if len(instrs) != 5 || memory == nil {
		return false
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x2F40 || // MOVE.L D0,96(A7)
		instrs[1].opcode != 0x0C6F || // CMPI.W #2,74(A7)
		instrs[2].opcode != 0x6200 || // BHI.W common target
		instrs[3].opcode != 0x3F7C || // MOVE.W #3,74(A7)
		instrs[4].opcode != 0x6000 { // BRA.W common target
		return false
	}
	readWord := func(pc uint32) (uint16, bool) {
		if pc+2 > uint32(len(memory)) {
			return 0, false
		}
		return uint16(memory[pc])<<8 | uint16(memory[pc+1]), true
	}
	moveLongDisp, ok := readWord(startPC + instrs[0].pcOffset + 2)
	if !ok || moveLongDisp != 0x0060 {
		return false
	}
	cmpiImm, ok := readWord(startPC + instrs[1].pcOffset + 2)
	if !ok || cmpiImm != 0x0002 {
		return false
	}
	cmpiDisp, ok := readWord(startPC + instrs[1].pcOffset + 4)
	if !ok || cmpiDisp != 0x004A {
		return false
	}
	bhiDisp, ok := readWord(startPC + instrs[2].pcOffset + 2)
	if !ok {
		return false
	}
	moveWordImm, ok := readWord(startPC + instrs[3].pcOffset + 2)
	if !ok || moveWordImm != 0x0003 {
		return false
	}
	moveWordDisp, ok := readWord(startPC + instrs[3].pcOffset + 4)
	if !ok || moveWordDisp != 0x004A {
		return false
	}
	braDisp, ok := readWord(startPC + instrs[4].pcOffset + 2)
	if !ok {
		return false
	}
	bhiPC := startPC + instrs[2].pcOffset
	braPC := startPC + instrs[4].pcOffset
	bhiTarget := uint32(int64(bhiPC+2) + int64(int16(bhiDisp)))
	braTarget := uint32(int64(braPC+2) + int64(int16(braDisp)))
	return bhiTarget == braTarget
}

func m68kIsBNEMoveMOVEMRTSBlock(instrs []M68KJITInstr, startPC uint32) bool {
	if len(instrs) != 4 {
		return false
	}
	if instrs[0].pcOffset != 0 ||
		instrs[0].opcode != 0x66FA || // BNE.S back
		instrs[1].opcode != 0x2004 || // MOVE.L D4,D0
		instrs[2].opcode != 0x4CDF || // MOVEM.L (A7)+,reg-list
		instrs[3].opcode != 0x4E75 { // RTS
		return false
	}
	branchTarget := uint32(int64(startPC+2) + int64(int8(instrs[0].opcode&0xFF)))
	return branchTarget == startPC-4
}

func m68kIsMoveA7PostincRTSBlock(instrs []M68KJITInstr) ([]uint16, bool) {
	if len(instrs) == 2 &&
		instrs[0].pcOffset == 0 &&
		instrs[0].opcode == 0x2C5F && // MOVEA.L (A7)+,A6
		instrs[1].opcode == 0x4E75 {
		return []uint16{6}, true
	}
	if len(instrs) == 3 &&
		instrs[0].pcOffset == 0 &&
		instrs[0].opcode == 0x245F && // MOVEA.L (A7)+,A2
		instrs[1].opcode == 0x2C5F && // MOVEA.L (A7)+,A6
		instrs[2].opcode == 0x4E75 {
		return []uint16{2, 6}, true
	}
	return nil, false
}

func m68kEmitArithmeticCCRNow(cb *CodeBuffer) {
	amd64SETcc(cb, amd64CondB, amd64RCX)
	amd64SETcc(cb, amd64CondO, amd64RDX)
	amd64SETcc(cb, amd64CondE, amd64R10)
	amd64SETcc(cb, 0x8, amd64R11)

	amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX)
	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 1)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64R10)
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
}

func m68kEmitPreserveXCMPCCR(cb *CodeBuffer) {
	amd64SETcc(cb, amd64CondB, amd64RCX)
	amd64SETcc(cb, amd64CondO, amd64RDX)
	amd64SETcc(cb, amd64CondE, amd64R10)
	amd64SETcc(cb, 0x8, amd64R11)

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
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

// m68kEmitMOVEM_PreInc emits MOVEM.{W,L} for the canonical prolog/epilog
// forms — register-list to -(An) (predecrement), (An)+ to register-list,
// d16(An) to register-list, and register-list to (An) (no-update bulk
// store). The register list is read at compile time from the extension word
// and unrolled into per-register transfers, then guarded by a single combined
// direct-RAM / SMC pre-check covering the full transfer extent.
// On any guard miss the block bails to the interpreter via NeedIOFallback.
//
// Handles eaMode == 2 (control store), eaMode == 3 (postincrement),
// eaMode == 4 (predecrement), and eaMode == 5 (displacement restore).
// Other addressing modes fall back to the interpreter via the unhandled
// path at the bottom of m68kEmitInstructionFull.
func m68kEmitMOVEM_PreInc(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if instrPC+4 > uint32(len(memory)) {
		// Cannot read register-list extension word — bail.
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return
	}
	// MOVEM clobbers EFLAGS (CMP for I/O, ADD/SUB for An). Materialize
	// any lazy CCR before this instruction's pre-check arithmetic.
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	regMask := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	dirMemToReg := (opcode>>10)&1 == 1 // 1 = (An)+ -> regs; 0 = regs -> -(An)
	sizeLong := (opcode>>6)&1 == 1
	eaMode := (opcode >> 3) & 7
	eaReg := opcode & 7
	disp := int16(0)
	if eaMode == 5 {
		if instrPC+6 > uint32(len(memory)) {
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
		disp = int16(uint16(memory[instrPC+4])<<8 | uint16(memory[instrPC+5]))
	}
	absWordAddr := uint32(0)
	if eaMode == 7 && eaReg == 0 {
		if instrPC+6 > uint32(len(memory)) {
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
		absWordAddr = uint32(int16(uint16(memory[instrPC+4])<<8 | uint16(memory[instrPC+5])))
	}
	pcDispAddr := uint32(0)
	if eaMode == 7 && eaReg == 2 {
		if instrPC+6 > uint32(len(memory)) {
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
		disp := int16(uint16(memory[instrPC+4])<<8 | uint16(memory[instrPC+5]))
		pcDispAddr = uint32(int64(instrPC+4) + int64(disp))
	}

	step := uint32(2)
	if sizeLong {
		step = 4
	}

	// Build the ordered list of regs to transfer.
	// Encoding: bit 0..15 = D0,D1,..,D7,A0,..,A7 normally, except for the
	// reg-to-memory predecrement form which reverses the bit order so the
	// physical memory order ends up D0..D7,A0..A7 from low to high address.
	var regs []uint16 // each entry: 0..7 = D0..D7, 8..15 = A0..A7
	if eaMode == 4 && !dirMemToReg {
		// Predecrement reg-to-memory: bit 0 = A7 .. bit 15 = D0.
		// Walk high-bit-first so transfers happen in D0..A7 order.
		for i := 15; i >= 0; i-- {
			if regMask&(1<<uint(i)) != 0 {
				regs = append(regs, uint16(15-i))
			}
		}
	} else {
		// Postincrement m-to-r (and any r-to-m mode != 4): bit 0 = D0..bit 15 = A7.
		for i := 0; i < 16; i++ {
			if regMask&(1<<uint(i)) != 0 {
				regs = append(regs, uint16(i))
			}
		}
	}
	regCount := uint32(len(regs))
	if regCount == 0 {
		// Empty mask: An update happens only on predecrement (no, actually
		// no transfer means no An change). Fall back conservatively.
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return
	}
	totalBytes := regCount * step
	failOff := []int{}

	// Compute starting address (R10 = base of transfer extent in memory).
	//   Postinc:    base = An;          new An = An + total.
	//   Predec:     base = An - total;  new An = base.
	//   Displ:      base = An + d16;    An unchanged.
	if eaMode == 6 || (eaMode == 7 && eaReg == 3) {
		if !m68kEmitComputeFullIndexEA(cb, eaMode, eaReg, memory, instrPC+4, amd64R10, &failOff) {
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
	} else if eaMode == 7 && eaReg == 0 {
		amd64MOV_reg_imm32(cb, amd64R10, absWordAddr)
	} else if eaMode == 7 && eaReg == 2 {
		amd64MOV_reg_imm32(cb, amd64R10, pcDispAddr)
	} else if asr, mapped := m68kAddrRegToAMD64(eaReg); mapped {
		amd64MOV_reg_reg32(cb, amd64R10, asr)
	} else {
		amd64MOV_reg_mem32(cb, amd64R10, m68kAMD64RegAddrBase, int32(eaReg)*4)
	}
	if eaMode == 4 {
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(totalBytes)) // SUB R10, total
	} else if eaMode == 5 && disp != 0 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(disp)) // ADD R10, d16
	}

	// Pre-check. The target CPU is 68020-class, so unaligned word/long RAM
	// MOVEM transfers are valid and can use the host's unaligned x86 loads.
	amd64MOV_reg_imm32(cb, amd64RDX, uint32(totalBytes))
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &failOff)

	// Unrolled transfers. R10 holds the base address; offset = i * step.
	// Loading/storing each register uses a SIB form that already supports
	// non-mapped data/address registers via the base file.
	for i, reg := range regs {
		offset := int32(uint32(i) * step)
		isAddr := reg >= 8
		regIdx := uint16(reg & 7)
		if dirMemToReg {
			// MOV[ZX] EAX, [RSI + R10 + offset]; store to register file.
			emitMOVEMRead(cb, sizeLong, amd64RAX, offset)
			if isAddr {
				// MOVEM.W m-to-r sign-extends to 32 (per 68K spec). For
				// .L the value is already 32-bit. Apply sign-extension
				// for word size before storing.
				if !sizeLong {
					cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RAX, amd64RAX)) // MOVSX EAX, AX
				}
				if mapped, ok := m68kAddrRegToAMD64(regIdx); ok {
					amd64MOV_reg_reg32(cb, mapped, amd64RAX)
				} else {
					amd64MOV_mem_reg32(cb, m68kAMD64RegAddrBase, int32(regIdx)*4, amd64RAX)
				}
			} else {
				if !sizeLong {
					// MOVEM.W m-to-r preserves the upper word for data
					// registers per 68000 semantics? Actually the 68K
					// spec sign-extends Dn the same as An. Sign-extend.
					cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RAX, amd64RAX)) // MOVSX EAX, AX
				}
				if mapped, ok := m68kDataRegToAMD64(regIdx); ok {
					amd64MOV_reg_reg32(cb, mapped, amd64RAX)
				} else {
					amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, int32(regIdx)*4, amd64RAX)
				}
			}
		} else {
			// Predecrement reg-to-memory with An itself in the mask:
			// the interpreter (cpu_m68k.go ExecMovem) decrements An
			// before each individual write, so the value stored for An's
			// own slot is the post-decrement An. By the iter ↔ slot
			// mapping (JIT slot j = interpreter iter n-1-j), the
			// post-decrement An at the An-slot equals R10 + offset, which
			// is exactly the address being written. We materialize that
			// directly so MOVEM.L A7,-(A7) and friends store the
			// decremented address rather than the original.
			isSelfBase := eaMode == 4 && isAddr && regIdx == eaReg
			if isSelfBase {
				amd64MOV_reg_reg32(cb, amd64RAX, amd64R10)
				if offset != 0 {
					amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, offset)
				}
			} else if isAddr {
				if mapped, ok := m68kAddrRegToAMD64(regIdx); ok {
					amd64MOV_reg_reg32(cb, amd64RAX, mapped)
				} else {
					amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegAddrBase, int32(regIdx)*4)
				}
			} else {
				if mapped, ok := m68kDataRegToAMD64(regIdx); ok {
					amd64MOV_reg_reg32(cb, amd64RAX, mapped)
				} else {
					amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegDataBase, int32(regIdx)*4)
				}
			}
			emitMOVEMWrite(cb, sizeLong, amd64RAX, offset)
		}
	}

	if !dirMemToReg {
		amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
	}

	// Update An where the addressing mode requires writeback.
	if eaMode == 3 {
		// Postincrement: An = R10 + total.
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(totalBytes))
	}
	// For predecrement, R10 already equals new An. Control and displacement
	// modes leave An unchanged.
	if eaMode == 3 || eaMode == 4 {
		if mapped, ok := m68kAddrRegToAMD64(eaReg); ok {
			amd64MOV_reg_reg32(cb, mapped, amd64R10)
		} else {
			amd64MOV_mem_reg32(cb, m68kAMD64RegAddrBase, int32(eaReg)*4, amd64R10)
		}
	}

	if !dirMemToReg {
		m68kEmitSMCInvalidateByteRangeCheck(cb, amd64R11, totalBytes)
		m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	}

	// Skip past the bail handler on the success path.
	doneOff := amd64JMP_rel32(cb)

	// Pre-check fail bail: NeedIOFallback + RetPC + full epilogue.
	failLabel := cb.Len()
	for _, off := range failOff {
		patchRel32(cb, off, failLabel)
	}
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)

	patchRel32(cb, doneOff, cb.Len())
}

// emitMOVEMRead emits a {word,long} read at [RSI + R10 + offset] with
// big-endian byte-swap into EAX.
func emitMOVEMRead(cb *CodeBuffer, sizeLong bool, dst byte, offset int32) {
	if sizeLong {
		// MOV EAX, [RSI + R10 + offset8/32]; BSWAP.
		emitMOVEMSIBLoad32(cb, dst, offset)
		emitREX(cb, false, 0, dst)
		cb.EmitBytes(0x0F, 0xC8+regBits(dst))
	} else {
		// MOV AX, [RSI + R10 + offset]; ROL AX, 8; MOVZX EAX, AX.
		emitMOVEMSIBLoad16(cb, dst, offset)
		cb.EmitBytes(0x66, 0xC1, 0xC0, 0x08) // ROL AX, 8
		amd64MOVZX_W(cb, dst, dst)
	}
}

// emitMOVEMWrite emits a {word,long} write at [RSI + R10 + offset] with
// big-endian byte-swap of the value in EAX (uses R11 as scratch).
func emitMOVEMWrite(cb *CodeBuffer, sizeLong bool, src byte, offset int32) {
	if sizeLong {
		amd64MOV_reg_reg32(cb, amd64R11, src)
		emitREX(cb, false, 0, amd64R11)
		cb.EmitBytes(0x0F, 0xC8+regBits(amd64R11))
		emitMOVEMSIBStore32(cb, amd64R11, offset)
	} else {
		amd64MOV_reg_reg32(cb, amd64R11, src)
		cb.EmitBytes(0x66, 0x41, 0xC1, 0xC3, 0x08) // ROL R11W, 8
		emitMOVEMSIBStore16(cb, amd64R11, offset)
	}
}

// emitMOVEMSIBLoad32 emits MOV reg, [RSI + R10 + disp]. reg is the
// destination GPR (must be in 0..7 unless caller passes REX-aware
// helpers — current usage stays in RAX).
func emitMOVEMSIBLoad32(cb *CodeBuffer, dst byte, disp int32) {
	// REX: extends index R10 (REX.X). dst RAX has reg field 0, so REX.R unset.
	cb.EmitBytes(0x42) // REX.X
	if disp == 0 {
		cb.EmitBytes(0x8B, modRM(0, dst, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase))
	} else if disp >= -128 && disp <= 127 {
		cb.EmitBytes(0x8B, modRM(1, dst, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase), byte(int8(disp)))
	} else {
		cb.EmitBytes(0x8B, modRM(2, dst, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase))
		cb.Emit32(uint32(disp))
	}
}

// emitMOVEMSIBStore32 emits MOV [RSI + R10 + disp], reg.
func emitMOVEMSIBStore32(cb *CodeBuffer, src byte, disp int32) {
	rex := byte(0x42) // REX.X
	if src >= 8 {
		rex |= 0x04 // REX.R
	}
	cb.EmitBytes(rex)
	if disp == 0 {
		cb.EmitBytes(0x89, modRM(0, src&7, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase))
	} else if disp >= -128 && disp <= 127 {
		cb.EmitBytes(0x89, modRM(1, src&7, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase), byte(int8(disp)))
	} else {
		cb.EmitBytes(0x89, modRM(2, src&7, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase))
		cb.Emit32(uint32(disp))
	}
}

// emitMOVEMSIBLoad16 emits MOV r16, [RSI + R10 + disp].
func emitMOVEMSIBLoad16(cb *CodeBuffer, dst byte, disp int32) {
	cb.EmitBytes(0x66, 0x42) // operand-size + REX.X
	if disp == 0 {
		cb.EmitBytes(0x8B, modRM(0, dst, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase))
	} else if disp >= -128 && disp <= 127 {
		cb.EmitBytes(0x8B, modRM(1, dst, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase), byte(int8(disp)))
	} else {
		cb.EmitBytes(0x8B, modRM(2, dst, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase))
		cb.Emit32(uint32(disp))
	}
}

// emitMOVEMSIBStore16 emits MOV [RSI + R10 + disp], r16.
func emitMOVEMSIBStore16(cb *CodeBuffer, src byte, disp int32) {
	rex := byte(0x42) // REX.X
	if src >= 8 {
		rex |= 0x04 // REX.R
	}
	cb.EmitBytes(0x66, rex)
	if disp == 0 {
		cb.EmitBytes(0x89, modRM(0, src&7, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase))
	} else if disp >= -128 && disp <= 127 {
		cb.EmitBytes(0x89, modRM(1, src&7, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase), byte(int8(disp)))
	} else {
		cb.EmitBytes(0x89, modRM(2, src&7, 4), sibByte(0, amd64R10&7, m68kAMD64RegMemBase))
		cb.Emit32(uint32(disp))
	}
}

// m68kEmitMOVE_Full emits MOVE.x <src_ea>,<dst_ea> (any addressing modes).
func m68kEmitMOVE_Full(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, size int, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset

	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7 // MOVE dest: mode at [8:6]
	dstReg := (opcode >> 9) & 7  // MOVE dest: reg at [11:9]

	// Read source into RDX
	extPC := instrPC + 2
	srcExtBytes := m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, extPC, instrPC, amd64RDX)
	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		m68kEmitExitIfIOFallback(cb, instrPC, uint32(instrIdx), br)
	}
	if dstMode == 1 && size == M68K_SIZE_WORD {
		cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64RDX, amd64RDX)) // MOVSX EDX, DX
	}
	if dstMode != 1 {
		amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RDX)
	}

	// Write to destination
	dstExtPC := extPC + uint32(srcExtBytes)
	m68kEmitWriteDestEA(cb, dstMode, dstReg, size, memory, dstExtPC, amd64RDX)
	if m68kEAMayUseMemHelper(dstMode, dstReg, true) {
		m68kEmitExitIfIOFallback(cb, instrPC, uint32(instrIdx), br)
	}

	// Flags: N,Z from result, V=0, C=0, X unchanged (unless MOVEA which doesn't set flags)
	if dstMode != 1 { // MOVEA doesn't affect flags
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
		m68kEmitSizedLogicTest(cb, amd64RAX, size, amd64RCX)
		emitCCR_Logic(cb)
	}
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
}

func m68kEmitMOVE_PostincPostinc(cb *CodeBuffer, ji *M68KJITInstr, startPC uint32, size int, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	stepSrc := m68kStepSize(size, srcReg)
	stepDst := m68kStepSize(size, dstReg)
	accessBytes := m68kAccessSizeBytes(size)

	srcAddr := m68kResolveAddrReg(cb, srcReg, amd64R10)
	if srcAddr != amd64R10 {
		amd64MOV_reg_reg32(cb, amd64R10, srcAddr)
	}
	dstAddr := m68kResolveAddrReg(cb, dstReg, amd64R11)
	if dstAddr != amd64R11 {
		amd64MOV_reg_reg32(cb, amd64R11, dstAddr)
	}

	bailOffs := make([]int, 0, 20)
	amd64MOV_reg_imm32(cb, amd64RDX, accessBytes)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, accessBytes)
	m68kEmitMemRangeBailChecks(cb, amd64R11, amd64RDX, &bailOffs)

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RDX, size)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RDX)
	amd64MOV_reg_reg32(cb, amd64R8, amd64R11)
	amd64MOV_mem_reg32(cb, amd64RSP, 40, amd64R8)
	switch size {
	case M68K_SIZE_BYTE:
		emitMemOpSIB(cb, false, 0x88, amd64RDX, m68kAMD64RegMemBase, amd64R11, 0)
	case M68K_SIZE_WORD:
		amd64MOV_reg_reg32(cb, amd64RAX, amd64RDX)
		cb.EmitBytes(0x66)
		emitREX(cb, false, 0, amd64RAX)
		cb.EmitBytes(0xC1, modRM(3, 0, amd64RAX), 8)
		cb.EmitBytes(0x66)
		emitMemOpSIB(cb, false, 0x89, amd64RAX, m68kAMD64RegMemBase, amd64R11, 0)
	case M68K_SIZE_LONG:
		amd64MOV_reg_reg32(cb, amd64RAX, amd64RDX)
		emitREX(cb, false, 0, amd64RAX)
		cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX))
		emitMemOpSIB(cb, false, 0x89, amd64RAX, m68kAMD64RegMemBase, amd64R11, 0)
	}

	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(stepSrc))
	m68kStoreAddrReg(cb, srcReg, amd64R10)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R8, int32(stepDst))
	m68kStoreAddrReg(cb, dstReg, amd64R8)
	amd64MOV_reg_mem32(cb, amd64RDX, amd64RSP, 32)
	m68kEmitSizedLogicTest(cb, amd64RDX, size, amd64RAX)
	emitCCR_Logic(cb)
	amd64MOV_reg_mem32(cb, amd64R8, amd64RSP, 40)
	m68kEmitSMCInvalidateRangeCheck(cb, amd64R8, size)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitLoadDirectRAM(cb *CodeBuffer, addrReg, dstReg byte, size int) {
	switch size {
	case M68K_SIZE_BYTE:
		emitREX_SIB(cb, false, dstReg, addrReg, m68kAMD64RegMemBase)
		cb.EmitBytes(0x0F, 0xB6, modRM(0, dstReg, 4), sibByte(0, addrReg, m68kAMD64RegMemBase))
	case M68K_SIZE_WORD:
		emitREX_SIB(cb, false, dstReg, addrReg, m68kAMD64RegMemBase)
		cb.EmitBytes(0x0F, 0xB7, modRM(0, dstReg, 4), sibByte(0, addrReg, m68kAMD64RegMemBase))
		if isExtReg(dstReg) {
			cb.EmitBytes(0x66, rexByte(false, false, false, true))
		} else {
			cb.EmitBytes(0x66)
		}
		cb.EmitBytes(0xC1, modRM(3, 0, dstReg), 8)
		amd64MOVZX_W(cb, dstReg, dstReg)
	case M68K_SIZE_LONG:
		emitMemOpSIB(cb, false, 0x8B, dstReg, m68kAMD64RegMemBase, addrReg, 0)
		emitREX(cb, false, 0, dstReg)
		cb.EmitBytes(0x0F, 0xC8+regBits(dstReg))
	}
}

func m68kEmitStoreDirectRAM(cb *CodeBuffer, addrReg, valReg byte, size int) {
	switch size {
	case M68K_SIZE_BYTE:
		emitMemOpSIB(cb, false, 0x88, valReg, m68kAMD64RegMemBase, addrReg, 0)
	case M68K_SIZE_WORD:
		amd64MOV_reg_reg32(cb, amd64R11, valReg)
		if isExtReg(amd64R11) {
			cb.EmitBytes(0x66, rexByte(false, false, false, true))
		} else {
			cb.EmitBytes(0x66)
		}
		cb.EmitBytes(0xC1, modRM(3, 0, amd64R11), 8)
		cb.EmitBytes(0x66)
		emitMemOpSIB(cb, false, 0x89, amd64R11, m68kAMD64RegMemBase, addrReg, 0)
	case M68K_SIZE_LONG:
		amd64MOV_reg_reg32(cb, amd64R11, valReg)
		emitREX(cb, false, 0, amd64R11)
		cb.EmitBytes(0x0F, 0xC8+regBits(amd64R11))
		emitMemOpSIB(cb, false, 0x89, amd64R11, m68kAMD64RegMemBase, addrReg, 0)
	}
}

func m68kEmitSMCInvalidateCheck(cb *CodeBuffer, addrReg byte) {
	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
	}
	amd64MOV_reg_reg32(cb, amd64RAX, addrReg)
	amd64MOV_reg_reg32(cb, amd64R11, amd64RAX)
	amd64SHR_imm(cb, amd64R11, 12)
	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffCodePageBitmapPtr))
	amd64TEST_reg_reg(cb, amd64RCX, amd64RCX)
	noBitmapOff := amd64Jcc_rel32(cb, amd64CondE)
	emitREX_SIB(cb, false, amd64R11, amd64R11, amd64RCX)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, amd64R11, 4), sibByte(0, amd64R11, amd64RCX))
	amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
	skipInvalOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffInvalAddr), amd64RAX)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffInvalSize), 1)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedInval), 1)
	patchRel32(cb, noBitmapOff, cb.Len())
	patchRel32(cb, skipInvalOff, cb.Len())
}

func m68kEmitSMCInvalidateRangeCheck(cb *CodeBuffer, addrReg byte, size int) {
	m68kEmitSMCInvalidateByteRangeCheck(cb, addrReg, m68kAccessSizeBytes(size))
}

func m68kEmitSMCInvalidateByteRangeCheck(cb *CodeBuffer, addrReg byte, accessBytes uint32) {
	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
	}
	amd64MOV_reg_reg32(cb, amd64RAX, addrReg)
	amd64MOV_mem_reg32(cb, amd64RSP, 20, amd64RAX) // write start
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(accessBytes))
	amd64MOV_mem_reg32(cb, amd64RSP, 36, amd64RAX) // exclusive write end
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, 1)
	amd64SHR_imm(cb, amd64RAX, 12)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RAX) // inclusive end page
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 20)
	amd64SHR_imm(cb, amd64RAX, 12)
	amd64MOV_mem_reg32(cb, amd64RSP, 28, amd64RAX) // start page

	doneOffs := []int{}
	hitOffs := []int{}

	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffCodePageBitmapPtr))
	amd64TEST_reg_reg(cb, amd64RCX, amd64RCX)
	doneOffs = append(doneOffs, amd64Jcc_rel32(cb, amd64CondE))

	loopOff := cb.Len()
	cb.EmitBytes(0x3B, 0x44, 0x24, 0x20) // CMP EAX,[RSP+32]
	doneOffs = append(doneOffs, amd64Jcc_rel32(cb, amd64CondA))
	cb.EmitBytes(0x80, 0x3C, 0x01, 0x00) // CMP BYTE [RCX+RAX],0
	noBitmapHitOff := amd64Jcc_rel32(cb, amd64CondE)

	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffCodePageMinPtr))
	amd64TEST_reg_reg(cb, amd64RCX, amd64RCX)
	hitOffs = append(hitOffs, amd64Jcc_rel32(cb, amd64CondE))
	amd64MOV_reg_mem(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffCodePageMaxPtr))
	amd64TEST_reg_reg(cb, amd64R11, amd64R11)
	hitOffs = append(hitOffs, amd64Jcc_rel32(cb, amd64CondE))
	amd64MOV_reg_mem32(cb, amd64RDX, m68kAMD64RegCtx, int32(m68kCtxOffCodePageBoundsLen))
	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, amd64RDX) // CMP page, bounds len
	hitOffs = append(hitOffs, amd64Jcc_rel32(cb, amd64CondAE))

	emitREX_SIB(cb, false, amd64RDX, amd64RAX, amd64RCX)
	cb.EmitBytes(0x0F, 0xB7, modRM(0, amd64RDX, 4), sibByte(1, amd64RAX, amd64RCX)) // MOVZX EDX, WORD [RCX+RAX*2]
	amd64ALU_reg_imm32_32bit(cb, 7, amd64RDX, 0xFFFF)
	noCompiledRangeOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64MOV_reg_reg(cb, amd64RCX, amd64R11)
	emitREX_SIB(cb, false, amd64R11, amd64RAX, amd64RCX)
	cb.EmitBytes(0x0F, 0xB7, modRM(0, amd64R11, 4), sibByte(1, amd64RAX, amd64RCX)) // MOVZX R11D, WORD [RCX+RAX*2]

	amd64MOV_reg_imm32(cb, amd64RCX, 0)  // write start offset for this page
	cb.EmitBytes(0x3B, 0x44, 0x24, 0x1C) // CMP EAX,[RSP+28]
	notStartPageOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, 20)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 0x0FFF)
	patchRel32(cb, notStartPageOff, cb.Len())

	amd64MOV_reg_imm32(cb, amd64R10, 0x1000) // exclusive write end offset for this page
	cb.EmitBytes(0x3B, 0x44, 0x24, 0x20)     // CMP EAX,[RSP+32]
	notEndPageOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 36)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, 1)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 0x0FFF)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)
	patchRel32(cb, notEndPageOff, cb.Len())

	amd64ALU_reg_reg32(cb, 0x39, amd64R10, amd64RDX) // CMP writeEnd, compiledMin
	noOverlapEndOff := amd64Jcc_rel32(cb, amd64CondBE)
	amd64ALU_reg_reg32(cb, 0x39, amd64RCX, amd64R11) // CMP writeStart, compiledMax
	hitOffs = append(hitOffs, amd64Jcc_rel32(cb, amd64CondB))

	patchRel32(cb, noBitmapHitOff, cb.Len())
	patchRel32(cb, noCompiledRangeOff, cb.Len())
	patchRel32(cb, noOverlapEndOff, cb.Len())
	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffCodePageBitmapPtr))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, 1)
	backOff := amd64JMP_rel32(cb)
	patchRel32(cb, backOff, loopOff)

	hitLabel := cb.Len()
	for _, off := range hitOffs {
		patchRel32(cb, off, hitLabel)
	}
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 20)
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffInvalAddr), amd64RAX)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffInvalSize), uint32(accessBytes))
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedInval), 1)
	doneLabel := cb.Len()
	for _, off := range doneOffs {
		patchRel32(cb, off, doneLabel)
	}
}

func m68kEmitExitIfInvalidated(cb *CodeBuffer, nextPC uint32, count uint32, br *m68kBlockRegs) {
	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
	}
	amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedInval), 0)
	doneOff := amd64Jcc_rel32(cb, amd64CondE)
	m68kEmitRetPC(cb, nextPC, count)
	m68kEmitEpilogue(cb, br)

	patchRel32(cb, doneOff, cb.Len())
}

func m68kEmitExitIfIOFallback(cb *CodeBuffer, instrPC uint32, count uint32, br *m68kBlockRegs) {
	if cs := m68kCurrentCS; cs != nil {
		m68kMaterializeCCR(cb, cs)
	}
	amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
	doneOff := amd64Jcc_rel32(cb, amd64CondE)
	m68kEmitRetPC(cb, instrPC, count)
	m68kEmitEpilogue(cb, br)

	patchRel32(cb, doneOff, cb.Len())
}

func m68kEmitFallbackAtInstr(cb *CodeBuffer, instrPC uint32, br *m68kBlockRegs, instrIdx int) {
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
}

func m68kEmitHelperAtInstr(cb *CodeBuffer, instrPC uint32, br *m68kBlockRegs, instrIdx int, helper uint32) {
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffHelperPC), instrPC)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedHelper), helper)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
}

func m68kEmitNativeException(cb *CodeBuffer, retPC uint32, count uint32, faultPC uint32, opcode uint16, vector uint8, br *m68kBlockRegs) {
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNativeException), uint32(vector))
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNativeExceptionPC), faultPC)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNativeExceptionIR), uint32(opcode))
	m68kEmitRetPC(cb, retPC, count)
	m68kEmitEpilogue(cb, br)
}

func m68kEmitTRAP(cb *CodeBuffer, ji *M68KJITInstr, blockStartPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := blockStartPC + ji.pcOffset
	vector := uint8(M68K_VEC_TRAP_BASE + uint8(opcode&0x000F))
	m68kEmitNativeException(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), instrPC, opcode, vector, br)
}

func m68kEmitDispAnReadPrecheck(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, size int, extOffset uint32, br *m68kBlockRegs, instrIdx int) ([]int, bool) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	baseReg := opcode & 7
	if instrPC+extOffset+2 > uint32(len(memory)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return nil, false
	}

	dispPC := instrPC + extOffset
	disp := int16(uint16(memory[dispPC])<<8 | uint16(memory[dispPC+1]))
	base := m68kResolveAddrReg(cb, baseReg, amd64R10)
	if base != amd64R10 {
		amd64MOV_reg_reg32(cb, amd64R10, base)
	}
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(disp))

	bailOffs := make([]int, 0, 8)
	amd64MOV_reg_imm32(cb, amd64RDX, m68kAccessSizeBytes(size))
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	return bailOffs, true
}

func m68kPatchFallbackBails(cb *CodeBuffer, bailOffs []int, instrPC uint32, br *m68kBlockRegs, instrIdx int) {
	doneOff := amd64JMP_rel32(cb)
	for _, off := range bailOffs {
		patchRel32(cb, off, cb.Len())
	}
	var savedFlagState m68kFlagState
	var haveCompileState bool
	if cs := m68kCurrentCS; cs != nil {
		savedFlagState = cs.flagState
		haveCompileState = true
	}
	m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
	if haveCompileState {
		m68kCurrentCS.flagState = savedFlagState
	}
	patchRel32(cb, doneOff, cb.Len())
}

func m68kPatchHelperBails(cb *CodeBuffer, bailOffs []int, instrPC uint32, br *m68kBlockRegs, instrIdx int, helper uint32) {
	doneOff := amd64JMP_rel32(cb)
	for _, off := range bailOffs {
		patchRel32(cb, off, cb.Len())
	}
	var savedFlagState m68kFlagState
	var haveCompileState bool
	if cs := m68kCurrentCS; cs != nil {
		savedFlagState = cs.flagState
		haveCompileState = true
	}
	m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, helper)
	if haveCompileState {
		m68kCurrentCS.flagState = savedFlagState
	}
	patchRel32(cb, doneOff, cb.Len())
}

func m68kPatchMOVESlowBails(cb *CodeBuffer, bailOffs []int, opcode uint16, memory []byte, instrPC uint32, br *m68kBlockRegs, instrIdx int) {
	if m68kMOVESlowHelperOK(opcode, memory, instrPC) {
		m68kPatchHelperBails(cb, bailOffs, instrPC, br, instrIdx, m68kJITHelperMMIOMOVE)
		return
	}
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kPatchCLRSlowBails(cb *CodeBuffer, bailOffs []int, opcode uint16, instrPC uint32, br *m68kBlockRegs, instrIdx int) {
	if m68kCLRSlowHelperOK(opcode) {
		m68kPatchHelperBails(cb, bailOffs, instrPC, br, instrIdx, m68kJITHelperMMIOCLR)
		return
	}
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kJITCodeLong(memory []byte, pc uint32) (uint32, bool) {
	if pc+4 > uint32(len(memory)) {
		return 0, false
	}
	return uint32(memory[pc])<<24 |
		uint32(memory[pc+1])<<16 |
		uint32(memory[pc+2])<<8 |
		uint32(memory[pc+3]), true
}

func m68kMOVESlowHelperOK(opcode uint16, memory []byte, instrPC uint32) bool {
	group := opcode >> 12
	if group != 0x1 && group != 0x2 && group != 0x3 {
		return false
	}
	size := M68K_SIZE_LONG
	if group == 0x1 {
		size = M68K_SIZE_BYTE
	} else if group == 0x3 {
		size = M68K_SIZE_WORD
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	extPC := instrPC + 2

	if m68kIsNativeSupportedMOVEA(opcode) {
		if srcMode != 7 || srcReg != 1 || dstMode != 1 {
			return false
		}
		addr, ok := m68kJITCodeLong(memory, extPC)
		return ok && isNativeNumericMMIOAddr(addr)
	}

	if m68kIsNativeSupportedMOVEMemToMemGuarded(opcode) && dstMode == 7 && dstReg == 1 {
		srcExtBytes := m68kEAExtBytes(srcMode, srcReg, size, memory, extPC)
		addr, ok := m68kJITCodeLong(memory, extPC+uint32(srcExtBytes))
		return ok && isNativeNumericMMIOAddr(addr)
	}

	if !m68kIsNativeSupportedMOVEGuarded(opcode) {
		return false
	}

	if srcMode == 7 && srcReg == 1 && dstMode == 0 {
		addr, ok := m68kJITCodeLong(memory, extPC)
		return ok && isNativeNumericMMIOAddr(addr)
	}

	srcRegOrImm := srcMode == 0 || (group != 0x1 && srcMode == 1) || (srcMode == 7 && srcReg == 4)
	if srcRegOrImm && m68kMoveMemToMemEASupported(dstMode, dstReg, false) {
		return true
	}
	return false
}

func m68kCLRSlowHelperOK(opcode uint16) bool {
	if !m68kIsNativeSupportedCLR(opcode) {
		return false
	}
	mode := (opcode >> 3) & 7
	return mode != 0
}

func m68kEmitMOVEA_Direct(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedMOVEA(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	group := opcode >> 12
	size := M68K_SIZE_LONG
	if group == 0x3 {
		size = M68K_SIZE_WORD
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	extPC := instrPC + 2
	bailOffs := []int{}

	switch srcMode {
	case 0:
		r := m68kResolveDataReg(cb, srcReg, amd64RAX)
		if r != amd64RAX {
			amd64MOV_reg_reg32(cb, amd64RAX, r)
		}
	case 1:
		r := m68kResolveAddrReg(cb, srcReg, amd64RAX)
		if r != amd64RAX {
			amd64MOV_reg_reg32(cb, amd64RAX, r)
		}
	case 3: // (An)+
		r := m68kResolveAddrReg(cb, srcReg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case 4: // -(An)
		r := m68kResolveAddrReg(cb, srcReg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, srcReg)))
	case 6, 7:
		if srcMode == 7 && srcReg != 3 {
			m68kEmitComputeEAAddr(cb, srcMode, srcReg, memory, extPC, instrPC, amd64R10)
			break
		}
		if !m68kEmitComputeFullIndexEA(cb, srcMode, srcReg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, srcMode, srcReg, memory, extPC, instrPC, amd64R10)
	}

	if srcMode == 7 && srcReg == 4 {
		if size == M68K_SIZE_WORD {
			val := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
			amd64MOV_reg_imm32(cb, amd64RAX, uint32(val))
		} else {
			val := uint32(memory[extPC])<<24 | uint32(memory[extPC+1])<<16 |
				uint32(memory[extPC+2])<<8 | uint32(memory[extPC+3])
			amd64MOV_reg_imm32(cb, amd64RAX, val)
		}
	} else if srcMode != 0 && srcMode != 1 {
		amd64MOV_reg_imm32(cb, amd64RDX, m68kAccessSizeBytes(size))
		m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
		m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, size)
		switch srcMode {
		case 3:
			ar := m68kResolveAddrReg(cb, srcReg, amd64RDX)
			amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(size, srcReg)))
			m68kStoreAddrReg(cb, srcReg, ar)
		case 4:
			m68kStoreAddrReg(cb, srcReg, amd64R10)
		}
	}

	if size == M68K_SIZE_WORD {
		amd64MOVSX_W(cb, amd64RAX, amd64RAX)
	}
	m68kStoreAddrReg(cb, dstReg, amd64RAX)
	if len(bailOffs) > 0 {
		m68kPatchMOVESlowBails(cb, bailOffs, opcode, memory, instrPC, br, instrIdx)
	}
}

func m68kEmitMOVE_Guarded(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedMOVEGuarded(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	group := opcode >> 12
	size := M68K_SIZE_LONG
	if group == 0x1 {
		size = M68K_SIZE_BYTE
	} else if group == 0x3 {
		size = M68K_SIZE_WORD
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	extPC := instrPC + 2
	srcExtBytes := m68kEAExtBytes(srcMode, srcReg, size, memory, extPC)
	dstExtPC := extPC + uint32(srcExtBytes)
	bailOffs := []int{}

	srcRegOrImm := srcMode == 0 || (group != 0x1 && srcMode == 1) || (srcMode == 7 && srcReg == 4)
	if srcRegOrImm {
		switch srcMode {
		case 0:
			r := m68kResolveDataReg(cb, srcReg, amd64RAX)
			if r != amd64RAX {
				amd64MOV_reg_reg32(cb, amd64RAX, r)
			}
			switch size {
			case M68K_SIZE_BYTE:
				amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0xFF)
			case M68K_SIZE_WORD:
				amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0xFFFF)
			}
		case 1:
			r := m68kResolveAddrReg(cb, srcReg, amd64RAX)
			if r != amd64RAX {
				amd64MOV_reg_reg32(cb, amd64RAX, r)
			}
			if size == M68K_SIZE_WORD {
				amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0xFFFF)
			}
		case 7:
			if size == M68K_SIZE_BYTE {
				amd64MOV_reg_imm32(cb, amd64RAX, uint32(memory[extPC+1]))
			} else if size == M68K_SIZE_WORD {
				val := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
				amd64MOV_reg_imm32(cb, amd64RAX, uint32(val))
			} else {
				val := uint32(memory[extPC])<<24 | uint32(memory[extPC+1])<<16 |
					uint32(memory[extPC+2])<<8 | uint32(memory[extPC+3])
				amd64MOV_reg_imm32(cb, amd64RAX, val)
			}
		}
		amd64MOV_reg_reg32(cb, amd64R8, amd64RAX)
		switch {
		case dstMode == 3:
			r := m68kResolveAddrReg(cb, dstReg, amd64R10)
			if r != amd64R10 {
				amd64MOV_reg_reg32(cb, amd64R10, r)
			}
		case dstMode == 4:
			r := m68kResolveAddrReg(cb, dstReg, amd64R10)
			if r != amd64R10 {
				amd64MOV_reg_reg32(cb, amd64R10, r)
			}
			amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, dstReg)))
		case dstMode == 6 || (dstMode == 7 && dstReg == 3):
			if !m68kEmitComputeFullIndexEA(cb, dstMode, dstReg, memory, dstExtPC, amd64R10, &bailOffs) {
				m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
				return
			}
		default:
			m68kEmitComputeEAAddr(cb, dstMode, dstReg, memory, dstExtPC, instrPC, amd64R10)
		}
		amd64MOV_reg_imm32(cb, amd64RDX, m68kAccessSizeBytes(size))
		m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
		amd64MOV_reg_reg32(cb, amd64RAX, amd64R8)
		m68kEmitStoreDirectRAM(cb, amd64R10, amd64RAX, size)
		switch dstMode {
		case 3:
			ar := m68kResolveAddrReg(cb, dstReg, amd64RDX)
			amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(size, dstReg)))
			m68kStoreAddrReg(cb, dstReg, ar)
		case 4:
			m68kStoreAddrReg(cb, dstReg, amd64R10)
		}
		m68kEmitSMCInvalidateRangeCheck(cb, amd64R10, size)
		m68kEmitSizedLogicTest(cb, amd64R8, size, amd64RAX)
		emitCCR_Logic(cb)
		m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
		m68kPatchMOVESlowBails(cb, bailOffs, opcode, memory, instrPC, br, instrIdx)
		return
	}

	switch {
	case srcMode == 3:
		r := m68kResolveAddrReg(cb, srcReg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
	case srcMode == 4:
		r := m68kResolveAddrReg(cb, srcReg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, srcReg)))
	case srcMode == 6 || (srcMode == 7 && srcReg == 3):
		if !m68kEmitComputeFullIndexEA(cb, srcMode, srcReg, memory, extPC, amd64R10, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	default:
		m68kEmitComputeEAAddr(cb, srcMode, srcReg, memory, extPC, instrPC, amd64R10)
	}
	amd64MOV_reg_imm32(cb, amd64RDX, m68kAccessSizeBytes(size))
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, size)
	switch srcMode {
	case 3:
		ar := m68kResolveAddrReg(cb, srcReg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(size, srcReg)))
		m68kStoreAddrReg(cb, srcReg, ar)
	case 4:
		m68kStoreAddrReg(cb, srcReg, amd64R10)
	}
	amd64MOV_reg_reg32(cb, amd64R8, amd64RAX)
	m68kEmitWriteDestEA(cb, dstMode, dstReg, size, memory, dstExtPC, amd64RAX)
	m68kEmitSizedLogicTest(cb, amd64R8, size, amd64RDX)
	emitCCR_Logic(cb)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchMOVESlowBails(cb, bailOffs, opcode, memory, instrPC, br, instrIdx)
}

func m68kEmitMOVE_MemToMemGuarded(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedMOVEMemToMemGuarded(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	bailOffs := []int{}
	if !m68kEmitMOVEMemToMemGuarded(cb, ji, memory, startPC, br, instrIdx, &bailOffs) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	m68kPatchMOVESlowBails(cb, bailOffs, opcode, memory, instrPC, br, instrIdx)
}

func m68kEmitMOVEMemToMemGuarded(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, bailOffs *[]int) bool {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := M68K_SIZE_LONG
	switch opcode >> 12 {
	case 0x1:
		size = M68K_SIZE_BYTE
	case 0x2:
		size = M68K_SIZE_LONG
	case 0x3:
		size = M68K_SIZE_WORD
	default:
		return false
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	if !m68kMoveMemToMemEASupported(srcMode, srcReg, true) ||
		!m68kMoveMemToMemEASupported(dstMode, dstReg, false) {
		return false
	}

	srcExtPC := instrPC + 2
	srcExtBytes := m68kEAExtBytes(srcMode, srcReg, size, memory, srcExtPC)
	dstExtPC := srcExtPC + uint32(srcExtBytes)
	dstExtBytes := m68kEAExtBytes(dstMode, dstReg, size, memory, dstExtPC)
	if dstExtPC+uint32(dstExtBytes) > uint32(len(memory)) {
		return false
	}

	if !m68kEmitMOVEMemEAAddr(cb, srcMode, srcReg, size, memory, srcExtPC, instrPC, amd64R10, 0, 0, false, bailOffs) {
		return false
	}
	if size == M68K_SIZE_WORD || size == M68K_SIZE_LONG {
		amd64TEST_reg_imm8(cb, amd64R10, 1)
		*bailOffs = append(*bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	}
	amd64MOV_reg_imm32(cb, amd64RDX, m68kAccessSizeBytes(size))
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, bailOffs)

	if !m68kEmitMOVEMemEAAddr(cb, dstMode, dstReg, size, memory, dstExtPC, instrPC, amd64R11, srcMode, srcReg, true, bailOffs) {
		return false
	}
	amd64MOV_reg_imm32(cb, amd64RDX, m68kAccessSizeBytes(size))
	m68kEmitMemRangeBailChecks(cb, amd64R11, amd64RDX, bailOffs)

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, size)
	amd64MOV_reg_reg32(cb, amd64R8, amd64RAX)
	storeAddrReg := byte(amd64R11)
	if size == M68K_SIZE_WORD || size == M68K_SIZE_LONG {
		amd64MOV_reg_reg32(cb, amd64RCX, amd64R11)
		storeAddrReg = amd64RCX
	}
	m68kEmitStoreDirectRAM(cb, storeAddrReg, amd64R8, size)
	m68kEmitMOVEMemEACommitFinal(cb, srcMode, srcReg, dstMode, dstReg, size)
	m68kEmitSMCInvalidateRangeCheck(cb, storeAddrReg, size)
	m68kEmitSizedLogicTest(cb, amd64R8, size, amd64RDX)
	emitCCR_Logic(cb)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	return true
}

func m68kEmitMOVEMemEAAddr(cb *CodeBuffer, mode, reg uint16, size int, memory []byte, extPC, instrPC uint32, dstReg byte, priorMode, priorReg uint16, havePrior bool, bailOffs *[]int) bool {
	switch mode {
	case 2, 3, 5:
		r := m68kResolveAddrReg(cb, reg, dstReg)
		if r != dstReg {
			amd64MOV_reg_reg32(cb, dstReg, r)
		}
		m68kEmitMOVEApplyPriorSourceAdjustment(cb, dstReg, reg, priorMode, priorReg, havePrior, size)
		if mode == 5 {
			if extPC+2 > uint32(len(memory)) {
				return false
			}
			disp := int16(uint16(memory[extPC])<<8 | uint16(memory[extPC+1]))
			amd64ALU_reg_imm32_32bit(cb, 0, dstReg, int32(disp))
		}
		return true
	case 7:
		if reg == 3 {
			if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, dstReg, bailOffs) {
				return false
			}
			return true
		}
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, dstReg)
		return true
	case 4:
		r := m68kResolveAddrReg(cb, reg, dstReg)
		if r != dstReg {
			amd64MOV_reg_reg32(cb, dstReg, r)
		}
		m68kEmitMOVEApplyPriorSourceAdjustment(cb, dstReg, reg, priorMode, priorReg, havePrior, size)
		amd64ALU_reg_imm32_32bit(cb, 5, dstReg, int32(m68kStepSize(size, reg)))
		return true
	case 6:
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, dstReg, bailOffs) {
			return false
		}
		m68kEmitMOVEApplyPriorSourceAdjustment(cb, dstReg, reg, priorMode, priorReg, havePrior, size)
		return true
	default:
		return false
	}
}

func m68kEmitMOVEApplyPriorSourceAdjustment(cb *CodeBuffer, dstReg byte, reg, priorMode, priorReg uint16, havePrior bool, size int) {
	if !havePrior || reg != priorReg {
		return
	}
	switch priorMode {
	case 3:
		amd64ALU_reg_imm32_32bit(cb, 0, dstReg, int32(m68kStepSize(size, priorReg)))
	case 4:
		amd64ALU_reg_imm32_32bit(cb, 5, dstReg, int32(m68kStepSize(size, priorReg)))
	}
}

func m68kEmitMOVEMemEACommitFinal(cb *CodeBuffer, srcMode, srcReg, dstMode, dstReg uint16, size int) {
	if (srcMode == 3 || srcMode == 4) && !((dstMode == 3 || dstMode == 4) && dstReg == srcReg) {
		ar := m68kResolveAddrReg(cb, srcReg, amd64RDX)
		if srcMode == 3 {
			amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(size, srcReg)))
		} else {
			amd64ALU_reg_imm32_32bit(cb, 5, ar, int32(m68kStepSize(size, srcReg)))
		}
		m68kStoreAddrReg(cb, srcReg, ar)
	}

	if dstMode != 3 && dstMode != 4 {
		return
	}
	ar := m68kResolveAddrReg(cb, dstReg, amd64RDX)
	m68kEmitMOVEApplyPriorSourceAdjustment(cb, ar, dstReg, srcMode, srcReg, true, size)
	if dstMode == 3 {
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(size, dstReg)))
	} else {
		amd64ALU_reg_imm32_32bit(cb, 5, ar, int32(m68kStepSize(size, dstReg)))
	}
	m68kStoreAddrReg(cb, dstReg, ar)
}

func m68kEmitMOVE_LongAnDispToReg(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	bailOffs, ok := m68kEmitDispAnReadPrecheck(cb, ji, memory, startPC, M68K_SIZE_LONG, 2, br, instrIdx)
	if !ok {
		return
	}

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_LONG)

	if dstMode == 0 {
		m68kStoreDataReg(cb, dstReg, amd64RAX)
		amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
		emitCCR_Logic(cb)
	} else {
		m68kStoreAddrReg(cb, dstReg, amd64RAX)
	}
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitMOVE_LongRegToStackDisp(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	if instrPC+4 > uint32(len(memory)) {
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return
	}

	disp := int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3]))
	amd64MOV_reg_reg32(cb, amd64R10, m68kAMD64RegA7)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(disp))

	bailOffs := make([]int, 0, 10)
	amd64TEST_reg_imm8(cb, amd64R10, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_imm32(cb, amd64RDX, 4)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, 4)
	m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	var src byte
	if srcMode == 0 {
		src = m68kResolveDataReg(cb, srcReg, amd64RAX)
	} else {
		src = m68kResolveAddrReg(cb, srcReg, amd64RAX)
	}
	if src != amd64RAX {
		amd64MOV_reg_reg32(cb, amd64RAX, src)
	}
	amd64MOV_reg_reg32(cb, amd64R11, amd64RAX)
	emitREX(cb, false, 0, amd64R11)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64R11)) // BSWAP R11D
	emitMemOpSIB(cb, false, 0x89, amd64R11, m68kAMD64RegMemBase, amd64R10, 0)

	amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
	emitCCR_Logic(cb)
	doneOff := amd64JMP_rel32(cb)

	for _, off := range bailOffs {
		patchRel32(cb, off, cb.Len())
	}
	var savedFlagState m68kFlagState
	var haveCompileState bool
	if cs := m68kCurrentCS; cs != nil {
		savedFlagState = cs.flagState
		haveCompileState = true
	}
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
	if haveCompileState {
		m68kCurrentCS.flagState = savedFlagState
	}

	patchRel32(cb, doneOff, cb.Len())
}

func m68kEmitMOVE_LongRegToStackPredec(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7

	amd64MOV_reg_reg32(cb, amd64R10, m68kAMD64RegA7)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, 4)

	bailOffs := make([]int, 0, 10)
	amd64TEST_reg_imm8(cb, amd64R10, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_imm32(cb, amd64RDX, 4)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, 4)
	m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	var src byte
	if srcMode == 0 {
		src = m68kResolveDataReg(cb, srcReg, amd64RAX)
	} else {
		src = m68kResolveAddrReg(cb, srcReg, amd64RAX)
	}
	if src != amd64RAX {
		amd64MOV_reg_reg32(cb, amd64RAX, src)
	}

	amd64MOV_reg_reg32(cb, amd64R11, amd64RAX)
	emitREX(cb, false, 0, amd64R11)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64R11))
	emitMemOpSIB(cb, false, 0x89, amd64R11, m68kAMD64RegMemBase, amd64R10, 0)
	m68kStoreAddrReg(cb, 7, amd64R10)

	amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
	emitCCR_Logic(cb)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
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

	info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, targetPC), br)
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

	bailOffs := make([]int, 0, 8)
	amd64MOV_reg_imm32(cb, amd64RDX, 4)
	m68kEmitMemRangeBailChecks(cb, m68kAMD64RegA7, amd64RDX, &bailOffs)

	// Write return address in big-endian: BSWAP + MOV [memBase+A7], val
	amd64MOV_reg_imm32(cb, amd64RAX, returnPC)
	// BSWAP EAX
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX))
	// MOV [RSI + R13], EAX (SIB addressing: base=RSI, index=R13, scale=1)
	emitMemOpSIB(cb, false, 0x89, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)
	m68kEmitSMCInvalidateRangeCheck(cb, m68kAMD64RegA7, M68K_SIZE_LONG)

	// Chain exit to target
	info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, targetPC), br)
	if chainSlots != nil {
		*chainSlots = append(*chainSlots, info)
	}

	// Bail path: restore A7 and return to interpreter
	for _, off := range bailOffs {
		patchRel32(cb, off, cb.Len())
	}
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

	// Bcc itself does not modify CCR, but the generated branch scaffolding
	// below may emit loop-budget arithmetic or chain-exit setup that clobbers
	// host EFLAGS. Materialize pending lazy flags first so both taken and
	// fall-through paths retain the interpreter-visible CCR.
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	// Evaluate M68K condition → returns Jcc condition code
	jccCond := m68kCondToJcc(cb, cond)

	if jccCond == 0xFF {
		// Always-taken condition: chain exit
		info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, targetPC), br)
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

			// Account the re-executed loop body in ChainCount so the
			// dispatcher's retired-instruction total includes every iteration,
			// not just the single linear pass the exit terminator counts. Only
			// committed back-jumps are counted (after the budget/cap checks
			// pass), so there is no off-by-one on the exiting iteration.
			amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffChainCount))
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffChainCount), amd64RAX)

			// Keep tight in-block loops on the same interrupt sampling cadence
			// as normal block chains. Without this, a hot native loop can spin
			// for m68kJitBudget guest instructions before Go observes pending
			// IRQ/exception state.
			amd64ALU_mem_imm32(cb, 5, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget), int32(bodySize))
			budgetExitOff := amd64Jcc_rel32(cb, amd64CondLE)

			// Safety budget check: if counter >= budget → exit.
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, int32(m68kJitBudget)) // CMP counter, budget
			safetyExitOff := amd64Jcc_rel32(cb, amd64CondAE)                // JAE budget_exit

			// Budget OK → JMP back to target native code
			targetNativeOffset := instrOffsets[targetIdx]
			backOff := amd64JMP_rel32(cb)
			patchRel32(cb, backOff, targetNativeOffset)

			// Budget exceeded → exit block with target PC.
			patchRel32(cb, budgetExitOff, cb.Len())
			patchRel32(cb, safetyExitOff, cb.Len())
			// Undo the last bodySize loop/budget addition before using the
			// normal chain exit. The exit itself accounts for the prefix plus
			// current loop body (instrIdx+1) that were actually executed.
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, int32(bodySize)) // SUB counter, bodySize
			amd64MOV_mem_reg32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), amd64RAX)
			amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffChainCount))
			amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffChainCount), amd64RAX)
			amd64ALU_mem_imm32(cb, 0, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget), int32(bodySize))
			bccInfo := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, targetPC), br)
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

	bccInfo := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, targetPC), br)
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

	bailOffs := make([]int, 0, 6)

	// Stack read checks: aligned, direct RAM, no 32-bit wrap across the longword.
	amd64TEST_reg_imm8(cb, m68kAMD64RegA7, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
	amd64ALU_reg_reg32(cb, 0x39, m68kAMD64RegA7, amd64R11) // CMP A7, MemSize
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondAE))
	amd64MOV_reg_reg32(cb, amd64RDX, m68kAMD64RegA7)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 4)
	amd64ALU_reg_reg32(cb, 0x39, amd64RDX, m68kAMD64RegA7) // CMP end, A7
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondB))
	amd64ALU_reg_reg32(cb, 0x39, amd64RDX, amd64R11) // CMP end, MemSize
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondA))

	m68kEmitMemAccessBailChecks(cb, m68kAMD64RegA7, &bailOffs)

	// Read32_BE: MOV EAX, [RSI + R13]; BSWAP EAX
	emitMemOpSIB(cb, false, 0x8B, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX)) // BSWAP EAX

	// Return target must be an even in-range guest PC before we enter the
	// cache-probe or unchained exit paths.
	amd64TEST_reg_imm8(cb, amd64RAX, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, amd64R11) // CMP poppedPC, MemSize
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondAE))

	// A7 += 4
	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4)

	// 8-entry MRU RTS inline cache. EAX holds the popped return PC.
	// Probe entries 0..7 sequentially; on hit, R10 = corresponding addr.
	// On miss after all eight, fall through to unchained exit (writes
	// RetPC, full epilogue). Slot 0 is the most-recently-warmed entry.
	amd64ALU_reg_mem32_cmp(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache0PC))
	miss0Off := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache0Addr))
	hit0Off := amd64JMP_rel32(cb)

	patchRel32(cb, miss0Off, cb.Len())
	amd64ALU_reg_mem32_cmp(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache1PC))
	miss1Off := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache1Addr))
	hit1Off := amd64JMP_rel32(cb)

	patchRel32(cb, miss1Off, cb.Len())
	amd64ALU_reg_mem32_cmp(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache2PC))
	miss2Off := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache2Addr))
	hit2Off := amd64JMP_rel32(cb)

	patchRel32(cb, miss2Off, cb.Len())
	amd64ALU_reg_mem32_cmp(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache3PC))
	miss3Off := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache3Addr))
	hit3Off := amd64JMP_rel32(cb)

	patchRel32(cb, miss3Off, cb.Len())
	amd64ALU_reg_mem32_cmp(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache4PC))
	miss4Off := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache4Addr))
	hit4Off := amd64JMP_rel32(cb)

	patchRel32(cb, miss4Off, cb.Len())
	amd64ALU_reg_mem32_cmp(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache5PC))
	miss5Off := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache5Addr))
	hit5Off := amd64JMP_rel32(cb)

	patchRel32(cb, miss5Off, cb.Len())
	amd64ALU_reg_mem32_cmp(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache6PC))
	miss6Off := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache6Addr))
	hit6Off := amd64JMP_rel32(cb)

	patchRel32(cb, miss6Off, cb.Len())
	amd64ALU_reg_mem32_cmp(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache7PC))
	missOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffRTSCache7Addr))

	// .chain_hit: R10 = chain entry. Stash it at [RSP+32] (free slot)
	// and the popped return PC at [RSP+40], because the lightweight epilogue
	// clobbers RAX and RDX.
	patchRel32(cb, hit0Off, cb.Len())
	patchRel32(cb, hit1Off, cb.Len())
	patchRel32(cb, hit2Off, cb.Len())
	patchRel32(cb, hit3Off, cb.Len())
	patchRel32(cb, hit4Off, cb.Len())
	patchRel32(cb, hit5Off, cb.Len())
	patchRel32(cb, hit6Off, cb.Len())
	amd64TEST_reg_reg(cb, amd64R10, amd64R10)
	emptySlotOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64MOV_mem_reg(cb, amd64RSP, 32, amd64R10)
	amd64MOV_mem_reg32(cb, amd64RSP, 40, amd64RAX)

	m68kEmitLightweightEpilogue(cb, br)

	// Reload R10 (chain target) from stack stash.
	amd64MOV_reg_mem(cb, amd64R10, amd64RSP, 32)

	// Add prefix instruction count (instrIdx — excludes the RTS itself,
	// which only counts on a successful chain). Charge the same amount to
	// ChainBudget so RTS-cache chaining observes the dispatcher IRQ cadence.
	if instrIdx > 0 {
		amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffChainCount))
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrIdx))
		amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffChainCount), amd64RAX)
		amd64ALU_mem_imm32(cb, 5, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget), int32(instrIdx))
	}

	amd64ALU_mem_imm32(cb, 5, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget), 1)
	budgetOff := amd64Jcc_rel32(cb, amd64CondLE)

	amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedInval), 0)
	invalOff := amd64Jcc_rel32(cb, amd64CondNE)
	invalGenOff := m68kEmitInvalGenerationChangedCheck(cb)
	asyncExitOffs := m68kEmitPendingAsyncExitChecks(cb)
	amd64MOV_reg_mem(cb, amd64R10, amd64RSP, 32)

	// Chain succeeded: count the RTS itself, then JMP R10.
	amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffChainCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, 1)
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffChainCount), amd64RAX)
	emitREX(cb, false, 0, amd64R10)
	cb.EmitBytes(0xFF, modRM(3, 4, amd64R10&7))

	// Budget exhausted: do not commit the RTS. The return PC was read for the
	// cache probe, but interrupts are sampled at the RTS boundary, so restore
	// A7 and ask the dispatcher to re-execute the RTS through the interpreter.
	patchRel32(cb, budgetOff, cb.Len())
	amd64ALU_reg_imm32_32bit(cb, 5, m68kAMD64RegA7, 4)
	amd64MOV_mem_reg32(cb, m68kAMD64RegAddrBase, 7*4, m68kAMD64RegA7)
	amd64ALU_mem_imm32(cb, 0, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget), 1)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), instrPC)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), 0)
	m68kEmitFullEpilogueEnd(cb)

	// Self-mod detected: stop chaining and return to the popped PC directly.
	// The RTS has already executed and A7 has already been stored as popped+4
	// by the lightweight epilogue.
	patchRel32(cb, invalOff, cb.Len())
	patchRel32(cb, invalGenOff, cb.Len())
	for _, off := range asyncExitOffs {
		patchRel32(cb, off, cb.Len())
	}
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 40)
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), amd64RAX)
	// RetCount = 1 (the RTS itself). The prefix instructions are already in
	// ChainCount (accumulated above), and the dispatcher sums ChainCount +
	// RetCount. Writing ChainCount+1 here would double-count the prefix.
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), 1)
	m68kEmitFullEpilogueEnd(cb)

	// .miss: cache miss — unchained exit with popped PC. CCR not yet
	// materialized at this point if flag tracker says lazy; ensure it.
	patchRel32(cb, missOff, cb.Len())
	patchRel32(cb, emptySlotOff, cb.Len())
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), amd64RAX)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
	m68kEmitEpilogue(cb, br)

	// Bail path: set NeedIOFallback + RetPC so dispatcher re-executes RTS via interpreter
	for _, off := range bailOffs {
		patchRel32(cb, off, cb.Len())
	}
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
}

func m68kEmitRTSNoChain(cb *CodeBuffer, ji *M68KJITInstr, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	bailOffs := make([]int, 0, 8)
	amd64TEST_reg_imm8(cb, m68kAMD64RegA7, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
	amd64ALU_reg_reg32(cb, 0x39, m68kAMD64RegA7, amd64R11) // CMP A7, MemSize
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondAE))
	amd64MOV_reg_reg32(cb, amd64RDX, m68kAMD64RegA7)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 4)
	amd64ALU_reg_reg32(cb, 0x39, amd64RDX, m68kAMD64RegA7) // CMP end, A7
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondB))
	amd64ALU_reg_reg32(cb, 0x39, amd64RDX, amd64R11) // CMP end, MemSize
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondA))
	m68kEmitMemAccessBailChecks(cb, m68kAMD64RegA7, &bailOffs)

	emitMemOpSIB(cb, false, 0x8B, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX)) // BSWAP EAX

	amd64TEST_reg_imm8(cb, amd64RAX, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, amd64R11) // CMP poppedPC, MemSize
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondAE))

	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4)
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), amd64RAX)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
	m68kEmitEpilogue(cb, br)

	for _, off := range bailOffs {
		patchRel32(cb, off, cb.Len())
	}
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)
}

func m68kEmitRTE(cb *CodeBuffer, ji *M68KJITInstr, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	instrPC := startPC + ji.pcOffset

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	bailOffs := make([]int, 0, 8)

	// The native path owns 68020+ format-0 frames. Malformed stacks, 68000
	// frames, non-supervisor RTE, and larger frame formats re-enter the
	// interpreter at the RTE.
	amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffUse68000Frame), 0)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))

	amd64MOV_reg_mem(cb, amd64R10, amd64RSP, int32(m68kAMD64OffSRPtr))
	amd64MOV_reg_mem32(cb, amd64RAX, amd64R10, 0)
	amd64MOVZX_W(cb, amd64RAX, amd64RAX)
	amd64TEST_reg_imm32(cb, amd64RAX, M68K_SR_S)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondE))

	amd64TEST_reg_imm8(cb, m68kAMD64RegA7, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_imm32(cb, amd64RDX, 8)
	m68kEmitMemRangeBailChecks(cb, m68kAMD64RegA7, amd64RDX, &bailOffs)

	// saved SR: big-endian word at [A7]. Load the first longword and keep
	// its high word after byte-swapping.
	emitMemOpSIB(cb, false, 0x8B, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX)) // BSWAP EAX
	amd64SHR_imm32(cb, amd64RAX, 16)
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RAX)

	// saved PC: big-endian longword at [A7+2].
	amd64MOV_reg_reg32(cb, amd64R10, m68kAMD64RegA7)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 2)
	emitMemOpSIB(cb, false, 0x8B, amd64RDX, m68kAMD64RegMemBase, amd64R10, 0)
	emitREX(cb, false, 0, amd64RDX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RDX)) // BSWAP EDX
	amd64MOV_mem_reg32(cb, amd64RSP, 36, amd64RDX)

	// format word: low word of big-endian longword at [A7+4]. Only format 0
	// is handled natively; other formats need interpreter frame sizing.
	amd64MOV_reg_reg32(cb, amd64R10, m68kAMD64RegA7)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 4)
	emitMemOpSIB(cb, false, 0x8B, amd64R11, m68kAMD64RegMemBase, amd64R10, 0)
	emitREX(cb, false, 0, amd64R11)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64R11)) // BSWAP R11D
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 0xF000)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))

	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
	amd64MOV_reg_mem(cb, amd64R10, amd64RSP, int32(m68kAMD64OffSRPtr))
	cb.EmitBytes(0x66)
	emitREX(cb, false, amd64RAX, amd64R10)
	cb.EmitBytes(0x89, modRM(0, amd64RAX, amd64R10&7)) // MOV [SRPtr], AX
	amd64MOV_reg_reg32(cb, m68kAMD64RegCCR, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, m68kAMD64RegCCR, 0x1F)

	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 8)
	amd64TEST_reg_imm32(cb, amd64RAX, M68K_SR_S)
	staySupervisorOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffSSPPtr))
	amd64MOV_mem_reg32(cb, amd64R10, 0, m68kAMD64RegA7)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffUSPPtr))
	amd64MOV_reg_mem32(cb, m68kAMD64RegA7, amd64R10, 0)
	patchRel32(cb, staySupervisorOff, cb.Len())
	amd64MOV_reg_mem32(cb, amd64RDX, amd64RSP, 36)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffInExceptionPtr))
	amd64MOV_mem_imm32(cb, amd64R10, 0, 0)
	amd64MOV_reg_mem(cb, amd64R10, m68kAMD64RegCtx, int32(m68kCtxOffRTECountPtr))
	amd64ALU_mem_imm32(cb, 0, amd64R10, 0, 1)
	amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), amd64RDX)
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
	m68kEmitEpilogue(cb, br)

	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

// m68kEmitJSR emits JSR <ea> (jump to subroutine, block terminator).
// Handles native-safe JSR addressing modes. Uses chain exits for static targets.
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
	case 5: // (d16,An) — dynamic
		if instrPC+4 > uint32(len(memory)) {
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
	case 6: // (d8,An,Xn) / full-format indexed — dynamic
		if !m68kIndexedEAAllowed(memory, instrPC+2, mode, reg) {
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
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
		case 2: // d16(PC)
			if instrPC+4 <= uint32(len(memory)) {
				w := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
				targetAddr = uint32(int64(instrPC+2) + int64(int16(w)))
				staticTarget = true
			}
		case 3: // d8(PC,Xn) / full-format indexed — dynamic
			if !m68kIndexedEAAllowed(memory, instrPC+2, mode, reg) {
				amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
				m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
				m68kEmitEpilogue(cb, br)
				return
			}
		default:
			// Unsupported addressing mode — bail
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
		if !staticTarget && reg != 3 {
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

	var targetBailOffs []int
	if !staticTarget {
		if m68kModeIsIndexed(mode, reg) {
			if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, instrPC+2, amd64R10, &targetBailOffs) {
				amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
				m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
				m68kEmitEpilogue(cb, br)
				return
			}
		} else {
			m68kEmitComputeEAAddr(cb, mode, reg, memory, instrPC+2, instrPC, amd64R10)
		}
		amd64TEST_reg_imm8(cb, amd64R10, 1)
		targetBailOffs = append(targetBailOffs, amd64Jcc_rel32(cb, amd64CondNE))
		amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
		amd64ALU_reg_reg32(cb, 0x39, amd64R10, amd64R11) // CMP target, MemSize
		targetBailOffs = append(targetBailOffs, amd64Jcc_rel32(cb, amd64CondAE))
	}

	// Push return address: A7 -= 4; Write32_BE
	amd64ALU_reg_imm32_32bit(cb, 5, m68kAMD64RegA7, 4) // SUB A7, 4

	bailOffs := make([]int, 0, 10)
	amd64TEST_reg_imm8(cb, m68kAMD64RegA7, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_imm32(cb, amd64RDX, 4)
	m68kEmitMemRangeBailChecks(cb, m68kAMD64RegA7, amd64RDX, &bailOffs)

	// Write return addr in big-endian
	amd64MOV_reg_imm32(cb, amd64RAX, returnPC)
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX)) // BSWAP EAX
	emitMemOpSIB(cb, false, 0x89, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)
	if !staticTarget {
		amd64MOV_mem_reg32(cb, amd64RSP, 48, amd64R10)
	}
	m68kEmitSMCInvalidateRangeCheck(cb, m68kAMD64RegA7, M68K_SIZE_LONG)
	if !staticTarget {
		amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 48)
	}

	if staticTarget {
		// Chain exit to known target
		info := m68kEmitChainExit(cb, targetAddr, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, targetAddr), br)
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
	for _, off := range bailOffs {
		patchRel32(cb, off, cb.Len())
	}
	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4) // undo SUB
	amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
	m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
	m68kEmitEpilogue(cb, br)

	for _, off := range targetBailOffs {
		patchRel32(cb, off, cb.Len())
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
	}
}

// m68kEmitJMP emits JMP <ea> (unconditional jump, block terminator).
// Uses chain exit for statically-known targets (abs.L, abs.W, d16(PC)).
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
	case 5: // d16(An) — dynamic target, cannot chain
		if instrPC+4 > uint32(len(memory)) {
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
		r := m68kResolveAddrReg(cb, reg, amd64RAX)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		disp := int32(int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])))
		if disp != 0 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, disp)
		}
		bailOffs := make([]int, 0, 2)
		amd64TEST_reg_imm8(cb, amd64R10, 1)
		bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
		amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
		amd64ALU_reg_reg32(cb, 0x39, amd64R10, amd64R11) // CMP target, MemSize
		bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondAE))
		amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), amd64R10)
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
		m68kEmitEpilogue(cb, br)
		for _, off := range bailOffs {
			patchRel32(cb, off, cb.Len())
		}
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return
	case 6: // indexed An — dynamic target, cannot chain
		if !m68kIndexedEAAllowed(memory, instrPC+2, mode, reg) {
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
		bailOffs := make([]int, 0, 4)
		if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, instrPC+2, amd64R10, &bailOffs) {
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
		}
		amd64TEST_reg_imm8(cb, amd64R10, 1)
		bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
		amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
		amd64ALU_reg_reg32(cb, 0x39, amd64R10, amd64R11) // CMP target, MemSize
		bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondAE))
		amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), amd64R10)
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
		m68kEmitEpilogue(cb, br)
		for _, off := range bailOffs {
			patchRel32(cb, off, cb.Len())
		}
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return
	case 7:
		switch reg {
		case 1: // abs.L — static target, chain
			if instrPC+6 <= uint32(len(memory)) {
				addr := uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
					uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5])
				info := m68kEmitChainExit(cb, addr, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, addr), br)
				if chainSlots != nil {
					*chainSlots = append(*chainSlots, info)
				}
				return
			}
		case 0: // abs.W — static target, chain
			if instrPC+4 <= uint32(len(memory)) {
				w := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
				addr := uint32(int32(int16(w)))
				info := m68kEmitChainExit(cb, addr, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, addr), br)
				if chainSlots != nil {
					*chainSlots = append(*chainSlots, info)
				}
				return
			}
		case 2: // d16(PC) — static target, chain
			if instrPC+4 <= uint32(len(memory)) {
				w := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
				addr := uint32(int64(instrPC+2) + int64(int16(w)))
				info := m68kEmitChainExit(cb, addr, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, addr), br)
				if chainSlots != nil {
					*chainSlots = append(*chainSlots, info)
				}
				return
			}
		case 3: // indexed PC — dynamic target, cannot chain
			if !m68kIndexedEAAllowed(memory, instrPC+2, mode, reg) {
				amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
				m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
				m68kEmitEpilogue(cb, br)
				return
			}
			bailOffs := make([]int, 0, 4)
			if !m68kEmitComputeFullIndexEA(cb, mode, reg, memory, instrPC+2, amd64R10, &bailOffs) {
				amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
				m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
				m68kEmitEpilogue(cb, br)
				return
			}
			amd64TEST_reg_imm8(cb, amd64R10, 1)
			bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
			amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
			amd64ALU_reg_reg32(cb, 0x39, amd64R10, amd64R11) // CMP target, MemSize
			bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondAE))
			amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetPC), amd64R10)
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffRetCount), uint32(instrIdx+1))
			m68kEmitEpilogue(cb, br)
			for _, off := range bailOffs {
				patchRel32(cb, off, cb.Len())
			}
			amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
			m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
			m68kEmitEpilogue(cb, br)
			return
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

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}
	jccCond := m68kCondToJcc(cb, cond)

	if jccCond == 0xFF { // always true
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

func m68kEmitSccValue(cb *CodeBuffer, cond uint16, dst byte) {
	jccCond := m68kCondToJcc(cb, cond)
	if jccCond == 0xFF {
		amd64MOV_reg_imm32(cb, dst, 0xFF)
		return
	}
	if jccCond == 0xFE {
		amd64XOR_reg_reg32(cb, dst, dst)
		return
	}
	amd64SETcc(cb, jccCond, dst)
	amd64NEG32(cb, dst)
}

func m68kEmitSccMemGuarded(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedScc(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	mode := (opcode >> 3) & 7
	reg := opcode & 7
	if mode == 0 {
		m68kEmitScc(cb, opcode)
		return
	}
	if m68kModeIsIndexed(mode, reg) && !m68kBriefIndexedEAAllowed(memory, instrPC+2, mode, reg) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	bailOffs := []int{}
	switch mode {
	case 2, 5, 6, 7:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, instrPC+2, instrPC, amd64R10)
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64R10)
		if ar != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, ar)
		}
	case 4:
		ar := m68kResolveAddrReg(cb, reg, amd64R10)
		if ar != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, ar)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(M68K_SIZE_BYTE, reg)))
	default:
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	amd64MOV_reg_imm32(cb, amd64RDX, 1)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	m68kEmitSccValue(cb, (opcode>>8)&0xF, amd64RAX)
	if mode == 3 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(m68kStepSize(M68K_SIZE_BYTE, reg)))
		m68kStoreAddrReg(cb, reg, amd64R10)
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(M68K_SIZE_BYTE, reg)))
	} else if mode == 4 {
		m68kStoreAddrReg(cb, reg, amd64R10)
	}
	m68kEmitStoreDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_BYTE)
	m68kEmitSMCInvalidateRangeCheck(cb, amd64R10, M68K_SIZE_BYTE)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
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

func m68kStoreDataRegWord(cb *CodeBuffer, dreg uint16, src byte, scratch byte) {
	r, mapped := m68kDataRegToAMD64(dreg)
	if mapped {
		amd64MOVZX_W(cb, scratch, src)
		amd64ALU_reg_imm32_32bit(cb, 4, r, -65536)
		amd64ALU_reg_reg32(cb, 0x09, r, scratch)
		return
	}
	if scratch == src {
		scratch = amd64RDX
		if scratch == src {
			scratch = amd64RCX
		}
	}
	amd64MOV_reg_mem32(cb, scratch, m68kAMD64RegDataBase, int32(dreg)*4)
	amd64ALU_reg_imm32_32bit(cb, 4, scratch, -65536)
	amd64MOVZX_W(cb, src, src)
	amd64ALU_reg_reg32(cb, 0x09, scratch, src)
	amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, int32(dreg)*4, scratch)
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
			// Account the re-executed loop body in ChainCount (see the Bcc
			// within-block loop for rationale). Only committed back-jumps count.
			amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffChainCount))
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffChainCount), amd64RAX)

			amd64ALU_mem_imm32(cb, 5, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget), int32(bodySize))
			budgetExitOff := amd64Jcc_rel32(cb, amd64CondLE)

			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, int32(m68kJitBudget))
			safetyExitOff := amd64Jcc_rel32(cb, amd64CondAE)

			// Budget OK → JMP back
			targetNativeOffset := instrOffsets[targetIdx]
			backOff := amd64JMP_rel32(cb)
			patchRel32(cb, backOff, targetNativeOffset)

			// Budget exceeded → chain exit to target.
			patchRel32(cb, budgetExitOff, cb.Len())
			patchRel32(cb, safetyExitOff, cb.Len())
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), amd64RAX)
			amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffChainCount))
			amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffChainCount), amd64RAX)
			amd64ALU_mem_imm32(cb, 0, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget), int32(bodySize))
			info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, targetPC), br)
			if chainSlots != nil {
				*chainSlots = append(*chainSlots, info)
			}

			// Exhausted → fall through
			patchRel32(cb, exhaustedOff, cb.Len())
			return
		}
	}

	// Fallback: chain exit to target PC (external branch)
	info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, targetPC), br)
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
func m68kEmitADD_EA_Dn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	opmode := (opcode >> 6) & 7
	size := int(opmode) // 0=byte, 1=word, 2=long
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) ||
		(m68kModeIsIndexed(srcMode, srcReg) && !m68kIndexedEAAllowed(memory, instrPC+2, srcMode, srcReg)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	// Read source EA value into RDX
	m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, instrPC+2, instrPC, amd64RDX)
	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}

	// ADD dst, src
	dst := m68kResolveDataReg(cb, dstReg, amd64RAX)
	m68kEmitSizedALURegReg(cb, 0x00, 0x01, dst, amd64RDX, size) // ADD dst,src
	emitCCR_Arithmetic(cb)
	m68kStoreDataReg(cb, dstReg, dst)
}

// m68kEmitSUB_EA_Dn emits SUB.x <ea>,Dn.
func m68kEmitSUB_EA_Dn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	opmode := (opcode >> 6) & 7
	size := int(opmode)
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) ||
		(m68kModeIsIndexed(srcMode, srcReg) && !m68kIndexedEAAllowed(memory, instrPC+2, srcMode, srcReg)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, instrPC+2, instrPC, amd64RDX)
	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}

	dst := m68kResolveDataReg(cb, dstReg, amd64RAX)
	m68kEmitSizedALURegReg(cb, 0x28, 0x29, dst, amd64RDX, size) // SUB dst,src
	emitCCR_Arithmetic(cb)
	m68kStoreDataReg(cb, dstReg, dst)
}

// m68kEmitCMP_EA_Dn emits CMP.x <ea>,Dn (compare, flags only, X unchanged).
func m68kEmitCMP_EA_Dn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedCMPToDn(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstReg := (opcode >> 9) & 7
	opmode := (opcode >> 6) & 7
	size := int(opmode)

	mutatingSource := srcMode == 3 || srcMode == 4
	if mutatingSource {
		r := m68kResolveAddrReg(cb, srcReg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		if srcMode == 4 {
			amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, srcReg)))
		}
		bail := 0
		m68kEmitMemRead(cb, amd64R10, amd64RDX, size, &bail)
	} else {
		m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, instrPC+2, instrPC, amd64RDX)
	}
	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}
	if mutatingSource {
		if srcMode == 3 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(m68kStepSize(size, srcReg)))
		}
		m68kStoreAddrReg(cb, srcReg, amd64R10)
	}

	dst := m68kResolveDataReg(cb, dstReg, amd64RAX)
	m68kEmitSizedALURegReg(cb, 0x38, 0x39, dst, amd64RDX, size) // CMP dst,src

	// Lazy CCR: CMP sets NZVC but NOT X — use flagsLiveArithNoX
	if cs := m68kCurrentCS; cs != nil {
		if m68kCCRDeadAtCurrent() {
			return
		}
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

func m68kEmitCMPM(cb *CodeBuffer, ji *M68KJITInstr, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if !m68kIsNativeSupportedCMPM(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	rx := opcode & 7
	ry := (opcode >> 9) & 7
	size := int((opcode >> 6) & 3)
	stepX := int32(m68kStepSize(size, rx))
	stepY := int32(m68kStepSize(size, ry))

	srcAddrReg := m68kResolveAddrReg(cb, rx, amd64R10)
	if srcAddrReg != amd64R10 {
		amd64MOV_reg_reg32(cb, amd64R10, srcAddrReg)
	}
	dstAddrReg := m68kResolveAddrReg(cb, ry, amd64R11)
	if dstAddrReg != amd64R11 {
		amd64MOV_reg_reg32(cb, amd64R11, dstAddrReg)
	}

	bailOffs := []int{}
	if size != M68K_SIZE_BYTE {
		amd64TEST_reg_imm8(cb, amd64R10, 1)
		bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
		amd64TEST_reg_imm8(cb, amd64R11, 1)
		bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	}
	byteLen := uint32(1 << uint(size))
	amd64MOV_reg_imm32(cb, amd64RDX, byteLen)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, byteLen)
	m68kEmitMemRangeBailChecks(cb, amd64R11, amd64RDX, &bailOffs)

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RDX, size)
	m68kEmitLoadDirectRAM(cb, amd64R11, amd64RAX, size)
	if size == M68K_SIZE_BYTE {
		amd64MOVZX_B(cb, amd64RDX, amd64RDX)
		amd64MOVZX_B(cb, amd64RAX, amd64RAX)
	} else if size == M68K_SIZE_WORD {
		amd64MOVZX_W(cb, amd64RDX, amd64RDX)
		amd64MOVZX_W(cb, amd64RAX, amd64RAX)
	}

	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, stepX)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R11, stepY)
	if rx == ry {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, stepY)
		m68kStoreAddrReg(cb, rx, amd64R10)
	} else {
		m68kStoreAddrReg(cb, rx, amd64R10)
		m68kStoreAddrReg(cb, ry, amd64R11)
	}

	m68kEmitSizedALURegReg(cb, 0x38, 0x39, amd64RAX, amd64RDX, size)
	if cs := m68kCurrentCS; cs != nil {
		if m68kCCRDeadAtCurrent() {
			m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
			return
		}
		cs.flagState = flagsLiveArithNoX
		m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
		return
	}
	m68kEmitPreserveXCMPCCR(cb)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

// m68kEmitCMPI emits CMPI.x #imm,<ea> (compare, flags only, X unchanged).
func m68kEmitCMPI(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if size == 3 {
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return
	}

	immBytes := m68kImmediateBytes(size)
	immPC := instrPC + 2
	if immPC+uint32(immBytes) > uint32(len(memory)) {
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		return
	}

	var imm uint32
	switch size {
	case M68K_SIZE_BYTE:
		imm = uint32(memory[immPC+1])
	case M68K_SIZE_WORD:
		imm = uint32(uint16(memory[immPC])<<8 | uint16(memory[immPC+1]))
	case M68K_SIZE_LONG:
		imm = uint32(memory[immPC])<<24 | uint32(memory[immPC+1])<<16 |
			uint32(memory[immPC+2])<<8 | uint32(memory[immPC+3])
	}

	eaExtPC := immPC + uint32(immBytes)
	mutatingSource := mode == 3 || mode == 4
	if mutatingSource {
		r := m68kResolveAddrReg(cb, reg, amd64R10)
		if r != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, r)
		}
		if mode == 4 {
			amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, reg)))
		}
		bail := 0
		m68kEmitMemRead(cb, amd64R10, amd64RDX, size, &bail)
	} else {
		m68kEmitReadSourceEA(cb, mode, reg, size, memory, eaExtPC, instrPC, amd64RDX)
	}
	if m68kEAMayUseMemHelper(mode, reg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}
	if mutatingSource {
		if mode == 3 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(m68kStepSize(size, reg)))
		}
		m68kStoreAddrReg(cb, reg, amd64R10)
	}
	amd64MOV_reg_imm32(cb, amd64RAX, imm)
	m68kEmitSizedALURegReg(cb, 0x38, 0x39, amd64RDX, amd64RAX, size) // CMP dest,imm

	if cs := m68kCurrentCS; cs != nil {
		if m68kCCRDeadAtCurrent() {
			return
		}
		cs.flagState = flagsLiveArithNoX
		return
	}

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
func m68kEmitLEA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	dstAReg := (opcode >> 9) & 7
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7

	if m68kModeIsIndexed(srcMode, srcReg) {
		bailOffs := []int{}
		if !m68kEmitComputeFullIndexEA(cb, srcMode, srcReg, memory, instrPC+2, amd64RAX, &bailOffs) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
		m68kStoreAddrReg(cb, dstAReg, amd64RAX)
		m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
		return
	}

	m68kEmitComputeEAAddr(cb, srcMode, srcReg, memory, instrPC+2, instrPC, amd64RAX)
	m68kStoreAddrReg(cb, dstAReg, amd64RAX)
}

// m68kEmitPEA emits PEA <ea> (compute EA, push to stack, no flags).
func m68kEmitPEA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7

	// Compute EA into R11. The range/page checks below use RAX/RCX/RDX as
	// scratch, so the pushed address must live elsewhere until the store.
	indexBails := []int{}
	if m68kModeIsIndexed(srcMode, srcReg) {
		if !m68kEmitComputeFullIndexEA(cb, srcMode, srcReg, memory, instrPC+2, amd64R11, &indexBails) {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
			return
		}
	} else {
		m68kEmitComputeEAAddr(cb, srcMode, srcReg, memory, instrPC+2, instrPC, amd64R11)
	}

	// Push to stack: A7 -= 4; Write32_BE([memBase + A7], EAX)
	amd64ALU_reg_imm32_32bit(cb, 5, m68kAMD64RegA7, 4)

	bailOffs := make([]int, 0, 10)
	amd64TEST_reg_imm8(cb, m68kAMD64RegA7, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_imm32(cb, amd64RDX, 4)
	m68kEmitMemRangeBailChecks(cb, m68kAMD64RegA7, amd64RDX, &bailOffs)

	// BSWAP and store
	emitREX(cb, false, 0, amd64R11)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64R11)) // BSWAP R11D
	emitMemOpSIB(cb, false, 0x89, amd64R11, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)
	m68kEmitSMCInvalidateRangeCheck(cb, m68kAMD64RegA7, M68K_SIZE_LONG)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)

	doneOff := amd64JMP_rel32(cb)

	// Bail
	for _, off := range indexBails {
		patchRel32(cb, off, cb.Len())
	}
	for _, off := range bailOffs {
		patchRel32(cb, off, cb.Len())
	}
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
	amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RDX)

	amd64ALU_reg_imm32_32bit(cb, 5, m68kAMD64RegA7, 4) // A7 -= 4

	bailOffs := make([]int, 0, 6)
	amd64TEST_reg_imm8(cb, m68kAMD64RegA7, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
	amd64ALU_reg_reg32(cb, 0x39, m68kAMD64RegA7, amd64R11) // CMP A7, MemSize
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondAE))
	amd64MOV_reg_reg32(cb, amd64RAX, m68kAMD64RegA7)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, 4)
	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, m68kAMD64RegA7) // CMP end, A7
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondB))
	amd64ALU_reg_reg32(cb, 0x39, amd64RAX, amd64R11) // CMP end, MemSize
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondA))
	m68kEmitMemAccessBailChecks(cb, m68kAMD64RegA7, &bailOffs)

	// Write An to stack (big-endian)
	amd64MOV_reg_mem32(cb, amd64RDX, amd64RSP, 32)
	emitREX(cb, false, 0, amd64RDX)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64RDX)) // BSWAP EDX
	emitMemOpSIB(cb, false, 0x89, amd64RDX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)

	// 2. An = A7 (frame pointer)
	m68kStoreAddrReg(cb, reg, m68kAMD64RegA7)

	// 3. A7 += displacement (negative = allocate)
	amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, disp) // ADD A7, disp

	doneOff := amd64JMP_rel32(cb)

	// Bail
	for _, off := range bailOffs {
		patchRel32(cb, off, cb.Len())
	}
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
	bailOffs := make([]int, 0, 6)
	amd64TEST_reg_imm8(cb, m68kAMD64RegA7, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegCtx, int32(m68kCtxOffMemSize))
	amd64ALU_reg_reg32(cb, 0x39, m68kAMD64RegA7, amd64R11) // CMP A7, MemSize
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondAE))
	amd64MOV_reg_reg32(cb, amd64RDX, m68kAMD64RegA7)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 4)
	amd64ALU_reg_reg32(cb, 0x39, amd64RDX, m68kAMD64RegA7) // CMP end, A7
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondB))
	amd64ALU_reg_reg32(cb, 0x39, amd64RDX, amd64R11) // CMP end, MemSize
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondA))
	m68kEmitMemAccessBailChecks(cb, m68kAMD64RegA7, &bailOffs)

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
	for _, off := range bailOffs {
		patchRel32(cb, off, cb.Len())
	}
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
	size := int((opcode >> 6) & 3)
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if mode == 0 { // Dn
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		m68kEmitSizedALURegImm(cb, 0, r, data, size)
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
	size := int((opcode >> 6) & 3)
	mode := (opcode >> 3) & 7
	reg := opcode & 7

	if mode == 0 { // Dn
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		m68kEmitSizedALURegImm(cb, 5, r, data, size)
		emitCCR_Arithmetic(cb)
		m68kStoreDataReg(cb, reg, r)
	} else if mode == 1 { // An — no flags affected
		r := m68kResolveAddrReg(cb, reg, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 5, r, int32(data))
		m68kStoreAddrReg(cb, reg, r)
	}
}

func m68kEmitADDQSUBQMemDispAn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, sub bool) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	size := int((opcode >> 6) & 3)
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	writableMemoryEA := mode == 2 || mode == 3 || mode == 4 || mode == 5 ||
		(mode == 6 && m68kBriefIndexedEAAllowed(memory, instrPC+2, mode, reg)) ||
		(mode == 7 && (reg == 0 || reg == 1))
	if size == 3 || !writableMemoryEA {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	if mode == 5 && instrPC+4 > uint32(len(memory)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	if (mode == 6 || mode == 7) && instrPC+2 > uint32(len(memory)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	if mode == 7 && reg == 1 && instrPC+6 > uint32(len(memory)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	data := (opcode >> 9) & 7
	if data == 0 {
		data = 8
	}
	if mode == 3 || mode == 4 {
		base := m68kResolveAddrReg(cb, reg, amd64R10)
		if base != amd64R10 {
			amd64MOV_reg_reg32(cb, amd64R10, base)
		}
		if mode == 4 {
			amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kStepSize(size, reg)))
		}
	} else {
		m68kEmitComputeEAAddr(cb, mode, reg, memory, instrPC+2, instrPC, amd64R10)
	}

	accessBytes := m68kAccessSizeBytes(size)
	bailOffs := []int{}
	amd64MOV_reg_imm32(cb, amd64RDX, accessBytes)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, size)

	if sub {
		m68kEmitSizedALURegImm(cb, 5, amd64RAX, data, size)
	} else {
		m68kEmitSizedALURegImm(cb, 0, amd64RAX, data, size)
	}
	emitCCR_Arithmetic(cb)
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		amd64MOV_mem_reg32(cb, amd64RSP, 32, amd64RAX)
		amd64MOV_mem_reg32(cb, amd64RSP, 36, amd64R10)
		m68kMaterializeCCR(cb, cs)
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, 32)
		amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 36)
	}
	amd64MOV_mem_reg32(cb, amd64RSP, 40, amd64R10)
	m68kEmitStoreDirectRAM(cb, amd64R10, amd64RAX, size)
	if mode == 4 {
		m68kStoreAddrReg(cb, reg, amd64R10)
	} else if mode == 3 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(m68kStepSize(size, reg)))
		m68kStoreAddrReg(cb, reg, amd64R10)
	}
	amd64MOV_reg_mem32(cb, amd64R10, amd64RSP, 40)
	m68kEmitSMCInvalidateRangeCheck(cb, amd64R10, size)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func amd64ShiftImm32(cb *CodeBuffer, op byte, dst byte, imm byte) {
	emitREX(cb, false, 0, dst)
	cb.EmitBytes(0xC1, modRM(3, op, dst), imm)
}

func amd64ShiftImmSized(cb *CodeBuffer, op byte, dst byte, size int, imm byte) {
	switch size {
	case M68K_SIZE_BYTE:
		emitREXForByte(cb, op, dst)
		cb.EmitBytes(0xC0, modRM(3, op, dst), imm)
	case M68K_SIZE_WORD:
		cb.EmitBytes(0x66)
		emitREX(cb, false, 0, dst)
		cb.EmitBytes(0xC1, modRM(3, op, dst), imm)
	default:
		amd64ShiftImm32(cb, op, dst, imm)
	}
}

func amd64ShiftCLSized(cb *CodeBuffer, op byte, dst byte, size int) {
	switch size {
	case M68K_SIZE_BYTE:
		emitREXForByte(cb, op, dst)
		cb.EmitBytes(0xD2, modRM(3, op, dst))
	case M68K_SIZE_WORD:
		cb.EmitBytes(0x66)
		emitREX(cb, false, op, dst)
		cb.EmitBytes(0xD3, modRM(3, op, dst))
	default:
		emitREX(cb, false, op, dst)
		cb.EmitBytes(0xD3, modRM(3, op, dst))
	}
}

func amd64RotateCarry1Sized(cb *CodeBuffer, op byte, dst byte, size int) {
	switch size {
	case M68K_SIZE_BYTE:
		emitREXForByte(cb, op, dst)
		cb.EmitBytes(0xD0, modRM(3, op, dst))
	case M68K_SIZE_WORD:
		cb.EmitBytes(0x66)
		emitREX(cb, false, op, dst)
		cb.EmitBytes(0xD1, modRM(3, op, dst))
	default:
		emitREX(cb, false, op, dst)
		cb.EmitBytes(0xD1, modRM(3, op, dst))
	}
}

func m68kEmitASImmByteWord(cb *CodeBuffer, opcode uint16) bool {
	size := int((opcode >> 6) & 3)
	isRegCount := (opcode >> 5) & 1
	shiftType := (opcode >> 3) & 3
	if isRegCount != 0 || shiftType != 0 || (size != M68K_SIZE_BYTE && size != M68K_SIZE_WORD) {
		return false
	}

	reg := opcode & 7
	count := (opcode >> 9) & 7
	if count == 0 {
		count = 8
	}
	direction := (opcode >> 8) & 1
	width := uint16(8)
	mask := int32(0xFF)
	signMask := uint32(0x80)
	if size == M68K_SIZE_WORD {
		width = 16
		mask = 0xFFFF
		signMask = 0x8000
	}

	r := m68kResolveDataReg(cb, reg, amd64RAX)
	amd64MOV_reg_reg32(cb, amd64R11, r)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, mask)
	amd64XOR_reg_reg32(cb, amd64R10, amd64R10) // V is only set by ASL below.

	if direction == 1 { // ASL
		if count >= width {
			amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
			amd64SETcc(cb, amd64CondNE, amd64R11)
			amd64MOVZX_B(cb, amd64R10, amd64R11)
			amd64ALU_reg_imm32_32bit(cb, 4, r, ^mask)
		} else {
			amd64SHR_imm32(cb, amd64R11, byte(width-count))
			amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
			amd64SETcc(cb, amd64CondNE, amd64R11)
			amd64MOVZX_B(cb, amd64R10, amd64R11)
			amd64ShiftImmSized(cb, 4, r, size, byte(count))
		}
	} else { // ASR
		if count >= width {
			amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
			amd64SETcc(cb, amd64CondNE, amd64R11)
			amd64TEST_reg_imm32(cb, r, signMask)
			signClearOff := amd64Jcc_rel32(cb, amd64CondE)
			amd64ALU_reg_imm32_32bit(cb, 4, r, ^mask)
			amd64ALU_reg_imm32_32bit(cb, 1, r, mask)
			signDoneOff := amd64JMP_rel32(cb)
			patchRel32(cb, signClearOff, cb.Len())
			amd64ALU_reg_imm32_32bit(cb, 4, r, ^mask)
			patchRel32(cb, signDoneOff, cb.Len())
		} else {
			if count > 1 {
				amd64SHR_imm32(cb, amd64R11, byte(count-1))
			}
			amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 1)
			amd64ShiftImmSized(cb, 7, r, size, byte(count))
		}
	}

	m68kEmitSizedLogicTest(cb, r, size, amd64RAX)
	m68kStoreDataReg(cb, reg, r)
	m68kEmitCCRShiftWithCarryOverflow(cb, amd64R11, amd64R10, r, size)
	return true
}

func m68kEmitLSRegDn(cb *CodeBuffer, opcode uint16) bool {
	size := int((opcode >> 6) & 3)
	isRegCount := (opcode >> 5) & 1
	shiftType := (opcode >> 3) & 3
	if isRegCount != 1 || shiftType != 1 || size == 3 {
		return false
	}

	reg := opcode & 7
	countReg := (opcode >> 9) & 7
	direction := (opcode >> 8) & 1
	width := uint32(32)
	mask := int32(-1)
	if size == M68K_SIZE_BYTE {
		width = 8
		mask = 0xFF
	} else if size == M68K_SIZE_WORD {
		width = 16
		mask = 0xFFFF
	}

	// Count 0 leaves CCR unchanged in the interpreter; materialize any pending
	// lazy CCR into R14 before the count manipulation clobbers host EFLAGS so
	// the count-0 path can preserve R14.
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	countSrc := m68kResolveDataReg(cb, countReg, amd64RCX)
	if countSrc != amd64RCX {
		amd64MOV_reg_reg32(cb, amd64RCX, countSrc)
	}
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 63)

	r := m68kResolveDataReg(cb, reg, amd64RAX)
	amd64TEST_reg_reg32(cb, amd64RCX, amd64RCX)
	zeroCountOff := amd64Jcc_rel32(cb, amd64CondE)

	amd64MOV_reg_reg32(cb, amd64R11, r)
	if size != M68K_SIZE_LONG {
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, mask)
	}

	amd64ALU_reg_imm32_32bit(cb, 7, amd64RCX, int32(width))
	saturateOff := amd64Jcc_rel32(cb, amd64CondAE)

	amd64MOV_reg_reg32(cb, amd64RDX, amd64RCX) // preserve original count
	amd64MOV_reg_reg32(cb, amd64R10, amd64R11)
	if direction == 1 { // LSL
		amd64MOV_reg_imm32(cb, amd64RCX, width)
		amd64ALU_reg_reg32(cb, 0x29, amd64RCX, amd64RDX) // width - count
		amd64ShiftCLSized(cb, 5, amd64R10, M68K_SIZE_LONG)
	} else { // LSR
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1) // count - 1
		amd64ShiftCLSized(cb, 5, amd64R10, M68K_SIZE_LONG)
	}
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 1)
	amd64MOV_reg_reg32(cb, amd64RCX, amd64RDX)
	if direction == 1 {
		amd64ShiftCLSized(cb, 4, r, size)
	} else {
		amd64ShiftCLSized(cb, 5, r, size)
	}
	normalDoneOff := amd64JMP_rel32(cb)

	patchRel32(cb, saturateOff, cb.Len())
	amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
	amd64SETcc(cb, amd64CondNE, amd64R10)
	if size == M68K_SIZE_LONG {
		amd64XOR_reg_reg32(cb, r, r)
	} else {
		amd64ALU_reg_imm32_32bit(cb, 4, r, ^mask)
	}

	patchRel32(cb, normalDoneOff, cb.Len())
	amd64XOR_reg_reg32(cb, amd64R11, amd64R11) // V=0
	m68kEmitSizedLogicTest(cb, r, size, amd64RAX)
	m68kStoreDataReg(cb, reg, r)
	m68kEmitCCRShiftWithCarryOverflow(cb, amd64R10, amd64R11, r, size)

	// Runtime count 0 lands here with R14 holding the pre-materialized incoming
	// CCR, which the M68K leaves unchanged.
	patchRel32(cb, zeroCountOff, cb.Len())
	return true
}

func m68kEmitASRegDn(cb *CodeBuffer, opcode uint16) bool {
	size := int((opcode >> 6) & 3)
	isRegCount := (opcode >> 5) & 1
	shiftType := (opcode >> 3) & 3
	if isRegCount != 1 || shiftType != 0 || size == 3 {
		return false
	}

	reg := opcode & 7
	countReg := (opcode >> 9) & 7
	direction := (opcode >> 8) & 1
	width := uint32(32)
	mask := int32(-1)
	signMask := uint32(0x80000000)
	if size == M68K_SIZE_BYTE {
		width = 8
		mask = 0xFF
		signMask = 0x80
	} else if size == M68K_SIZE_WORD {
		width = 16
		mask = 0xFFFF
		signMask = 0x8000
	}

	// The interpreter returns immediately for a runtime count of 0, leaving CCR
	// entirely unchanged (cpu_m68k.go ExecShiftRotate). Materialize any pending
	// lazy CCR into R14 now — before the count manipulation below clobbers host
	// EFLAGS — so the count-0 path can simply preserve R14 and match.
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	countSrc := m68kResolveDataReg(cb, countReg, amd64RCX)
	if countSrc != amd64RCX {
		amd64MOV_reg_reg32(cb, amd64RCX, countSrc)
	}
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 63)

	r := m68kResolveDataReg(cb, reg, amd64RAX)
	amd64TEST_reg_reg32(cb, amd64RCX, amd64RCX)
	zeroCountOff := amd64Jcc_rel32(cb, amd64CondE)

	amd64MOV_reg_reg32(cb, amd64R11, r)
	if size != M68K_SIZE_LONG {
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, mask)
	}

	amd64ALU_reg_imm32_32bit(cb, 7, amd64RCX, int32(width))
	saturateOff := amd64Jcc_rel32(cb, amd64CondAE)

	amd64MOV_reg_reg32(cb, amd64RDX, amd64RCX)
	amd64MOV_reg_reg32(cb, amd64R10, amd64R11)
	if direction == 1 { // ASL
		amd64MOV_reg_imm32(cb, amd64RCX, width)
		amd64ALU_reg_reg32(cb, 0x29, amd64RCX, amd64RDX) // width - count
		amd64ShiftCLSized(cb, 5, amd64R10, M68K_SIZE_LONG)
		amd64TEST_reg_reg32(cb, amd64R10, amd64R10)
		amd64SETcc(cb, amd64CondNE, amd64R10)
		amd64MOVZX_B(cb, amd64R11, amd64R10)
		amd64MOV_reg_reg32(cb, amd64RCX, amd64RDX)
		amd64ShiftCLSized(cb, 4, r, size)
	} else { // ASR
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
		amd64ShiftCLSized(cb, 5, amd64R10, M68K_SIZE_LONG)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, 1)
		amd64MOV_reg_reg32(cb, amd64RCX, amd64RDX)
		amd64ShiftCLSized(cb, 7, r, size)
		amd64XOR_reg_reg32(cb, amd64R11, amd64R11)
	}
	normalDoneOff := amd64JMP_rel32(cb)

	patchRel32(cb, saturateOff, cb.Len())
	if direction == 1 {
		amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
		amd64SETcc(cb, amd64CondNE, amd64R10)
		amd64MOVZX_B(cb, amd64R11, amd64R10)
		if size == M68K_SIZE_LONG {
			amd64XOR_reg_reg32(cb, r, r)
		} else {
			amd64ALU_reg_imm32_32bit(cb, 4, r, ^mask)
		}
	} else {
		amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
		amd64SETcc(cb, amd64CondNE, amd64R10)
		amd64XOR_reg_reg32(cb, amd64R11, amd64R11)
		amd64TEST_reg_imm32(cb, r, signMask)
		signClearOff := amd64Jcc_rel32(cb, amd64CondE)
		if size == M68K_SIZE_LONG {
			amd64MOV_reg_imm32(cb, r, 0xFFFFFFFF)
		} else {
			amd64ALU_reg_imm32_32bit(cb, 4, r, ^mask)
			amd64ALU_reg_imm32_32bit(cb, 1, r, mask)
		}
		signDoneOff := amd64JMP_rel32(cb)
		patchRel32(cb, signClearOff, cb.Len())
		if size == M68K_SIZE_LONG {
			amd64XOR_reg_reg32(cb, r, r)
		} else {
			amd64ALU_reg_imm32_32bit(cb, 4, r, ^mask)
		}
		patchRel32(cb, signDoneOff, cb.Len())
	}

	patchRel32(cb, normalDoneOff, cb.Len())
	m68kEmitSizedLogicTest(cb, r, size, amd64RAX)
	m68kStoreDataReg(cb, reg, r)
	m68kEmitCCRShiftWithCarryOverflow(cb, amd64R10, amd64R11, r, size)

	// Runtime count 0 lands here with R14 holding the pre-materialized incoming
	// CCR, which the M68K leaves unchanged. Nothing more to do.
	patchRel32(cb, zeroCountOff, cb.Len())
	return true
}

func m68kEmitRORegDn(cb *CodeBuffer, opcode uint16) bool {
	size := int((opcode >> 6) & 3)
	isRegCount := (opcode >> 5) & 1
	shiftType := (opcode >> 3) & 3
	if isRegCount != 1 || shiftType != 3 || size == 3 {
		return false
	}

	reg := opcode & 7
	countReg := (opcode >> 9) & 7
	direction := (opcode >> 8) & 1
	widthMask := int32(31)
	ieSize := byte(IE64_SIZE_L)
	signMask := uint32(0x80000000)
	if size == M68K_SIZE_BYTE {
		widthMask = 7
		ieSize = IE64_SIZE_B
		signMask = 0x80
	} else if size == M68K_SIZE_WORD {
		widthMask = 15
		ieSize = IE64_SIZE_W
		signMask = 0x8000
	}

	// Count 0 leaves CCR unchanged in the interpreter; materialize any pending
	// lazy CCR into R14 before the count manipulation clobbers host EFLAGS so
	// the count-0 path can preserve R14.
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	countSrc := m68kResolveDataReg(cb, countReg, amd64RCX)
	if countSrc != amd64RCX {
		amd64MOV_reg_reg32(cb, amd64RCX, countSrc)
	}
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 63)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, widthMask)

	r := m68kResolveDataReg(cb, reg, amd64RAX)
	amd64TEST_reg_reg32(cb, amd64RCX, amd64RCX)
	zeroCountOff := amd64Jcc_rel32(cb, amd64CondE)

	op := byte(0) // ROL
	if direction == 0 {
		op = 1 // ROR
	}
	amd64ROT_CL(cb, r, ieSize, op)

	if direction == 1 {
		amd64MOV_reg_reg32(cb, amd64R11, r)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 1)
	} else {
		amd64TEST_reg_imm32(cb, r, signMask)
		amd64SETcc(cb, amd64CondNE, amd64R11)
	}
	m68kEmitSizedLogicTest(cb, r, size, amd64RAX)
	m68kStoreDataReg(cb, reg, r)
	m68kEmitCCRLogicWithSavedCarry(cb, amd64R11, r, size)

	// Runtime count 0 lands here with R14 holding the pre-materialized incoming
	// CCR, which the M68K leaves unchanged.
	patchRel32(cb, zeroCountOff, cb.Len())
	return true
}

func amd64LOOPBack(cb *CodeBuffer, target int) {
	next := cb.Len() + 2
	rel := target - next
	if rel < -128 || rel > 127 {
		panic("amd64 LOOP target out of rel8 range")
	}
	cb.EmitBytes(0xE2, byte(int8(rel)))
}

func m68kEmitROXRegDn(cb *CodeBuffer, opcode uint16) bool {
	size := int((opcode >> 6) & 3)
	isRegCount := (opcode >> 5) & 1
	shiftType := (opcode >> 3) & 3
	if isRegCount != 1 || shiftType != 2 || size == 3 {
		return false
	}

	reg := opcode & 7
	countReg := (opcode >> 9) & 7
	modulus := int32(33)
	if size == M68K_SIZE_BYTE {
		modulus = 9
	} else if size == M68K_SIZE_WORD {
		modulus = 17
	}

	// Count 0 leaves CCR unchanged in the interpreter. Materialize any pending
	// lazy CCR into R14 BEFORE the count manipulation below clobbers host EFLAGS,
	// so the count-0 path (which jumps to the epilogue) exits with the correct,
	// unchanged CCR rather than the trashed EFLAGS from the AND/mod/TEST sequence.
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	countSrc := m68kResolveDataReg(cb, countReg, amd64RCX)
	if countSrc != amd64RCX {
		amd64MOV_reg_reg32(cb, amd64RCX, countSrc)
	}
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 63)

	modLoop := cb.Len()
	amd64ALU_reg_imm32_32bit(cb, 7, amd64RCX, modulus)
	modDoneOff := amd64Jcc_rel32(cb, amd64CondB)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, modulus)
	modBackOff := amd64JMP_rel32(cb)
	patchRel32(cb, modBackOff, modLoop)
	patchRel32(cb, modDoneOff, cb.Len())

	amd64TEST_reg_reg32(cb, amd64RCX, amd64RCX)
	doneOff := amd64Jcc_rel32(cb, amd64CondE)

	r := m68kResolveDataReg(cb, reg, amd64RAX)
	amd64BTRegImm32(cb, m68kAMD64RegCCR, 4)

	op := byte(2) // RCL
	if ((opcode >> 8) & 1) == 0 {
		op = 3 // RCR
	}
	loopStart := cb.Len()
	amd64RotateCarry1Sized(cb, op, r, size)
	amd64LOOPBack(cb, loopStart)
	m68kStoreDataReg(cb, reg, r)

	amd64SETcc(cb, amd64CondB, amd64RCX) // final C, also X

	mask := int32(-1)
	msb := int32(-2147483648)
	switch size {
	case M68K_SIZE_BYTE:
		mask = 0xFF
		msb = 0x80
	case M68K_SIZE_WORD:
		mask = 0xFFFF
		msb = 0x8000
	}
	amd64MOV_reg_reg32(cb, amd64R10, r)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, mask)
	amd64TEST_reg_reg32(cb, amd64R10, amd64R10)
	amd64SETcc(cb, amd64CondE, amd64RDX)

	amd64MOV_reg_reg32(cb, amd64R11, r)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, msb)
	amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
	amd64SETcc(cb, amd64CondNE, amd64R11)

	amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX) // C
	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // X = C
	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
	patchRel32(cb, doneOff, cb.Len())
	return true
}

func m68kEmitROXImmDn(cb *CodeBuffer, opcode uint16) {
	reg := opcode & 7
	count := (opcode >> 9) & 7
	if count == 0 {
		count = 8
	}
	size := int((opcode >> 6) & 3)
	if size == 3 || ((opcode>>5)&1) != 0 || ((opcode>>3)&3) != 2 {
		return
	}

	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	r := m68kResolveDataReg(cb, reg, amd64RAX)
	amd64BTRegImm32(cb, m68kAMD64RegCCR, 4)

	op := byte(2) // RCL
	if ((opcode >> 8) & 1) == 0 {
		op = 3 // RCR
	}
	for i := uint16(0); i < count; i++ {
		amd64RotateCarry1Sized(cb, op, r, size)
	}
	m68kStoreDataReg(cb, reg, r)

	amd64SETcc(cb, amd64CondB, amd64RCX) // final C, also X

	mask := int32(-1)
	msb := int32(-2147483648)
	switch size {
	case M68K_SIZE_BYTE:
		mask = 0xFF
		msb = 0x80
	case M68K_SIZE_WORD:
		mask = 0xFFFF
		msb = 0x8000
	}
	amd64MOV_reg_reg32(cb, amd64R10, r)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R10, mask)
	amd64TEST_reg_reg32(cb, amd64R10, amd64R10)
	amd64SETcc(cb, amd64CondE, amd64RDX)

	amd64MOV_reg_reg32(cb, amd64R11, r)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, msb)
	amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
	amd64SETcc(cb, amd64CondNE, amd64R11)

	amd64MOVZX_B(cb, m68kAMD64RegCCR, amd64RCX) // C
	amd64MOVZX_B(cb, amd64RAX, amd64RCX)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX) // X = C
	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	amd64MOVZX_B(cb, amd64RAX, amd64R11)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
}

func m68kEmitBFTSTRegister(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	reg := opcode & 7
	if memory == nil || instrPC+4 > uint32(len(memory)) || !m68kIsNativeSupportedBFTSTRegister(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	ext := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	if ext&(M68K_BF_OFFSET_REG|M68K_BF_WIDTH_REG) != 0 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	offset := uint32((ext & M68K_BF_OFFSET_MASK) >> 6)
	width := uint32(ext & M68K_BF_WIDTH_MASK)
	if width == 0 {
		width = 32
	}
	offset %= 32

	src := m68kResolveDataReg(cb, reg, amd64RAX)
	if src != amd64RAX {
		amd64MOV_reg_reg32(cb, amd64RAX, src)
	}
	if width < 32 {
		shiftAmount := uint32(0)
		if offset+width <= 32 {
			shiftAmount = 32 - offset - width
		}
		if shiftAmount != 0 {
			amd64SHR_imm(cb, amd64RAX, byte(shiftAmount))
		}
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, int32((uint32(1)<<width)-1))
	}

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
	amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	patchRel32(cb, notZeroOff, cb.Len())
	if width < 32 {
		amd64TEST_reg_imm32(cb, amd64RAX, uint32(1)<<(width-1))
	} else {
		amd64TEST_reg_imm32(cb, amd64RAX, 0x80000000)
	}
	notNegativeOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
	patchRel32(cb, notNegativeOff, cb.Len())
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
}

func m68kEmitBFEXTRegister(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, signed bool) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	reg := opcode & 7
	if memory == nil || instrPC+4 > uint32(len(memory)) ||
		(!signed && !m68kIsNativeSupportedBFEXTURegister(opcode)) ||
		(signed && !m68kIsNativeSupportedBFEXTSRegister(opcode)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	ext := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	if ext&(M68K_BF_OFFSET_REG|M68K_BF_WIDTH_REG) != 0 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	destReg := (ext >> 12) & 7
	offset := uint32((ext & M68K_BF_OFFSET_MASK) >> 6)
	width := uint32(ext & M68K_BF_WIDTH_MASK)
	if width == 0 {
		width = 32
	}
	offset %= 32

	src := m68kResolveDataReg(cb, reg, amd64RAX)
	if src != amd64RAX {
		amd64MOV_reg_reg32(cb, amd64RAX, src)
	}
	if width < 32 {
		shiftAmount := uint32(0)
		if offset+width <= 32 {
			shiftAmount = 32 - offset - width
		}
		if shiftAmount != 0 {
			amd64SHR_imm(cb, amd64RAX, byte(shiftAmount))
		}
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, int32((uint32(1)<<width)-1))
	}
	if signed && width < 32 {
		shift := byte(64 - width)
		amd64SHL_imm(cb, amd64RAX, shift)
		amd64SAR_imm(cb, amd64RAX, shift)
	}
	m68kStoreDataReg(cb, destReg, amd64RAX)

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
	amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	patchRel32(cb, notZeroOff, cb.Len())
	if width < 32 {
		amd64TEST_reg_imm32(cb, amd64RAX, uint32(1)<<(width-1))
	} else {
		amd64TEST_reg_imm32(cb, amd64RAX, 0x80000000)
	}
	notNegativeOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
	patchRel32(cb, notNegativeOff, cb.Len())
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
}

func m68kEmitBFEXTURegister(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	m68kEmitBFEXTRegister(cb, ji, memory, startPC, br, instrIdx, false)
}

func m68kEmitBFEXTSRegister(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	m68kEmitBFEXTRegister(cb, ji, memory, startPC, br, instrIdx, true)
}

func m68kEmitBFFFOResultAndFlags(cb *CodeBuffer, destReg uint16, offset, width uint32) {
	amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
	amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
	zeroOff := amd64Jcc_rel32(cb, amd64CondE)

	amd64BSR32(cb, amd64RCX, amd64RAX)
	amd64MOV_reg_imm32(cb, amd64RDX, offset+width-1)
	amd64ALU_reg_reg32(cb, 0x29, amd64RDX, amd64RCX) // SUB result, bitIndex
	doneOff := amd64JMP_rel32(cb)

	patchRel32(cb, zeroOff, cb.Len())
	amd64MOV_reg_imm32(cb, amd64RDX, offset+width)
	patchRel32(cb, doneOff, cb.Len())
	m68kStoreDataReg(cb, destReg, amd64RDX)

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
	amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	patchRel32(cb, notZeroOff, cb.Len())
	if width < 32 {
		amd64TEST_reg_imm32(cb, amd64RAX, uint32(1)<<(width-1))
	} else {
		amd64TEST_reg_imm32(cb, amd64RAX, 0x80000000)
	}
	notNegativeOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
	patchRel32(cb, notNegativeOff, cb.Len())
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
}

func m68kEmitBFFFORegister(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	reg := opcode & 7
	if memory == nil || instrPC+4 > uint32(len(memory)) || !m68kIsNativeSupportedBFFFORegister(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	ext := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	if ext&(M68K_BF_OFFSET_REG|M68K_BF_WIDTH_REG) != 0 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	destReg := (ext >> 12) & 7
	offset := uint32((ext & M68K_BF_OFFSET_MASK) >> 6)
	width := uint32(ext & M68K_BF_WIDTH_MASK)
	if width == 0 {
		width = 32
	}
	extractOffset := offset % 32

	src := m68kResolveDataReg(cb, reg, amd64RAX)
	if src != amd64RAX {
		amd64MOV_reg_reg32(cb, amd64RAX, src)
	}
	if width < 32 {
		shiftAmount := uint32(0)
		if extractOffset+width <= 32 {
			shiftAmount = 32 - extractOffset - width
		}
		if shiftAmount != 0 {
			amd64SHR_imm(cb, amd64RAX, byte(shiftAmount))
		}
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, int32((uint32(1)<<width)-1))
	}
	m68kEmitBFFFOResultAndFlags(cb, destReg, offset, width)
}

func m68kEmitBFWriteRegisterImmediate(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	reg := opcode & 7
	if memory == nil || instrPC+4 > uint32(len(memory)) || !m68kIsNativeSupportedBFWriteRegisterImmediate(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	ext := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	if ext&(M68K_BF_OFFSET_REG|M68K_BF_WIDTH_REG) != 0 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	destReg := (ext >> 12) & 7
	offset := uint32((ext & M68K_BF_OFFSET_MASK) >> 6)
	width := uint32(ext & M68K_BF_WIDTH_MASK)
	if width == 0 {
		width = 32
	}
	extractOffset := offset % 32
	shiftAmount := uint32(0)
	if width < 32 && extractOffset+width <= 32 {
		shiftAmount = 32 - extractOffset - width
	}
	mask := uint32(0xFFFFFFFF)
	if width < 32 {
		mask = (uint32(1) << width) - 1
	}

	src := m68kResolveDataReg(cb, reg, amd64RAX)
	if src != amd64RAX {
		amd64MOV_reg_reg32(cb, amd64RAX, src)
	}
	amd64MOV_reg_imm32(cb, amd64R11, mask)
	if shiftAmount != 0 {
		amd64SHL_imm(cb, amd64R11, byte(shiftAmount))
	}

	switch opcode & 0xFFC0 {
	case 0xEAC0: // BFCHG
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		if shiftAmount != 0 {
			amd64SHR_imm(cb, amd64RDX, byte(shiftAmount))
		}
		if width < 32 {
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, int32(mask))
		}
		amd64ALU_reg_reg32(cb, 0x31, amd64RAX, amd64R11)
	case 0xECC0: // BFCLR
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		if shiftAmount != 0 {
			amd64SHR_imm(cb, amd64RDX, byte(shiftAmount))
		}
		if width < 32 {
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, int32(mask))
		}
		amd64ALU_reg_imm32_32bit(cb, 6, amd64R11, -1)
		amd64ALU_reg_reg32(cb, 0x21, amd64RAX, amd64R11)
	case 0xEEC0: // BFSET
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		if shiftAmount != 0 {
			amd64SHR_imm(cb, amd64RDX, byte(shiftAmount))
		}
		if width < 32 {
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, int32(mask))
		}
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64R11)
	default: // BFINS
		valueReg := m68kResolveDataReg(cb, destReg, amd64RDX)
		if valueReg != amd64RDX {
			amd64MOV_reg_reg32(cb, amd64RDX, valueReg)
		}
		if width < 32 {
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, int32(mask))
		}
		amd64MOV_reg_reg32(cb, amd64RCX, amd64RDX)
		if shiftAmount != 0 {
			amd64SHL_imm(cb, amd64RCX, byte(shiftAmount))
		}
		amd64ALU_reg_imm32_32bit(cb, 6, amd64R11, -1)
		amd64ALU_reg_reg32(cb, 0x21, amd64RAX, amd64R11)
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)
	}
	m68kStoreDataReg(cb, reg, amd64RAX)

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
	amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	patchRel32(cb, notZeroOff, cb.Len())
	if width < 32 {
		amd64TEST_reg_imm32(cb, amd64RDX, uint32(1)<<(width-1))
	} else {
		amd64TEST_reg_imm32(cb, amd64RDX, 0x80000000)
	}
	notNegativeOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
	patchRel32(cb, notNegativeOff, cb.Len())
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
}

func m68kEmitLoadDirectRAMByte(cb *CodeBuffer, addrReg, dstReg byte) {
	emitREX_SIB(cb, false, dstReg, addrReg, m68kAMD64RegMemBase)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, dstReg, 4), sibByte(0, addrReg, m68kAMD64RegMemBase))
}

func m68kEmitBFTSTMemoryImmediate(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedBFTSTMemoryImmediate(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	ext := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	if ext&(M68K_BF_OFFSET_REG|M68K_BF_WIDTH_REG) != 0 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	offset := uint32((ext & M68K_BF_OFFSET_MASK) >> 6)
	width := uint32(ext & M68K_BF_WIDTH_MASK)
	if width == 0 {
		width = 32
	}
	bitOffset := offset & 7
	byteOffset := offset >> 3
	bytesToRead := (bitOffset + width + 7) >> 3
	if bytesToRead == 0 || bytesToRead > 4 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	eaExtPC := instrPC + 4
	m68kEmitComputeEAAddr(cb, mode, reg, memory, eaExtPC, instrPC, amd64R10)
	if byteOffset != 0 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(byteOffset))
	}

	bailOffs := make([]int, 0, 8)
	amd64MOV_reg_imm32(cb, amd64RDX, bytesToRead)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)
	for i := uint32(0); i < bytesToRead; i++ {
		if i != 0 {
			amd64SHL_imm(cb, amd64RAX, 8)
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)
		}
		m68kEmitLoadDirectRAMByte(cb, amd64R10, amd64RCX)
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)
	}

	shiftAmount := bytesToRead*8 - bitOffset - width
	if shiftAmount != 0 {
		amd64SHR_imm(cb, amd64RAX, byte(shiftAmount))
	}
	if width < 32 {
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, int32((uint32(1)<<width)-1))
	}

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
	amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	patchRel32(cb, notZeroOff, cb.Len())
	if width < 32 {
		amd64TEST_reg_imm32(cb, amd64RAX, uint32(1)<<(width-1))
	} else {
		amd64TEST_reg_imm32(cb, amd64RAX, 0x80000000)
	}
	notNegativeOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
	patchRel32(cb, notNegativeOff, cb.Len())
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}

	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitBFWriteMemoryImmediate(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedBFWriteMemoryImmediate(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	ext := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	if ext&(M68K_BF_OFFSET_REG|M68K_BF_WIDTH_REG) != 0 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	destReg := (ext >> 12) & 7
	offset := uint32((ext & M68K_BF_OFFSET_MASK) >> 6)
	width := uint32(ext & M68K_BF_WIDTH_MASK)
	if width == 0 {
		width = 32
	}
	bitOffset := offset & 7
	byteOffset := offset >> 3
	bytesToRead := (bitOffset + width + 7) >> 3
	if bytesToRead == 0 || bytesToRead > 4 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	eaExtPC := instrPC + 4
	m68kEmitComputeEAAddr(cb, mode, reg, memory, eaExtPC, instrPC, amd64R10)
	if byteOffset != 0 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(byteOffset))
	}

	bailOffs := make([]int, 0, 10)
	amd64MOV_reg_imm32(cb, amd64RDX, bytesToRead)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, bytesToRead)
	m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)
	for i := uint32(0); i < bytesToRead; i++ {
		if i != 0 {
			amd64SHL_imm(cb, amd64RAX, 8)
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)
		}
		m68kEmitLoadDirectRAMByte(cb, amd64R10, amd64RCX)
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)
	}
	if bytesToRead > 1 {
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(bytesToRead-1))
	}

	shiftAmount := bytesToRead*8 - bitOffset - width
	mask := uint32(0xFFFFFFFF)
	if width < 32 {
		mask = (uint32(1) << width) - 1
	}
	amd64MOV_reg_imm32(cb, amd64R11, mask)
	if shiftAmount != 0 {
		amd64SHL_imm(cb, amd64R11, byte(shiftAmount))
	}

	switch opcode & 0xFFC0 {
	case 0xEAC0: // BFCHG
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		if shiftAmount != 0 {
			amd64SHR_imm(cb, amd64RDX, byte(shiftAmount))
		}
		if width < 32 {
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, int32(mask))
		}
		amd64ALU_reg_reg32(cb, 0x31, amd64RAX, amd64R11) // XOR fieldData, mask
	case 0xECC0: // BFCLR
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		if shiftAmount != 0 {
			amd64SHR_imm(cb, amd64RDX, byte(shiftAmount))
		}
		if width < 32 {
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, int32(mask))
		}
		amd64ALU_reg_imm32_32bit(cb, 6, amd64R11, -1)
		amd64ALU_reg_reg32(cb, 0x21, amd64RAX, amd64R11) // AND fieldData, ^mask
	case 0xEEC0: // BFSET
		amd64MOV_reg_reg32(cb, amd64RDX, amd64RAX)
		if shiftAmount != 0 {
			amd64SHR_imm(cb, amd64RDX, byte(shiftAmount))
		}
		if width < 32 {
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, int32(mask))
		}
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64R11) // OR fieldData, mask
	default: // BFINS
		src := m68kResolveDataReg(cb, destReg, amd64RDX)
		if src != amd64RDX {
			amd64MOV_reg_reg32(cb, amd64RDX, src)
		}
		if width < 32 {
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, int32(mask))
		}
		amd64MOV_reg_reg32(cb, amd64RCX, amd64RDX)
		if shiftAmount != 0 {
			amd64SHL_imm(cb, amd64RCX, byte(shiftAmount))
		}
		amd64ALU_reg_imm32_32bit(cb, 6, amd64R11, -1)
		amd64ALU_reg_reg32(cb, 0x21, amd64RAX, amd64R11) // AND fieldData, ^mask
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX) // OR inserted field
	}

	for i := uint32(0); i < bytesToRead; i++ {
		amd64MOV_reg_reg32(cb, amd64R11, amd64RAX)
		shift := byte((bytesToRead - 1 - i) * 8)
		if shift != 0 {
			amd64SHR_imm(cb, amd64R11, shift)
		}
		if i != 0 {
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)
		}
		emitMemOpSIB(cb, false, 0x88, amd64R11, m68kAMD64RegMemBase, amd64R10, 0)
	}

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
	amd64TEST_reg_reg32(cb, amd64RDX, amd64RDX)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	patchRel32(cb, notZeroOff, cb.Len())
	if width < 32 {
		amd64TEST_reg_imm32(cb, amd64RDX, uint32(1)<<(width-1))
	} else {
		amd64TEST_reg_imm32(cb, amd64RDX, 0x80000000)
	}
	notNegativeOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
	patchRel32(cb, notNegativeOff, cb.Len())
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}

	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitBFEXTMemoryImmediate(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, signed bool) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) ||
		(!signed && !m68kIsNativeSupportedBFEXTUMemoryImmediate(opcode)) ||
		(signed && !m68kIsNativeSupportedBFEXTSMemoryImmediate(opcode)) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	ext := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	if ext&(M68K_BF_OFFSET_REG|M68K_BF_WIDTH_REG) != 0 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	destReg := (ext >> 12) & 7
	offset := uint32((ext & M68K_BF_OFFSET_MASK) >> 6)
	width := uint32(ext & M68K_BF_WIDTH_MASK)
	if width == 0 {
		width = 32
	}
	bitOffset := offset & 7
	byteOffset := offset >> 3
	bytesToRead := (bitOffset + width + 7) >> 3
	if bytesToRead == 0 || bytesToRead > 4 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	eaExtPC := instrPC + 4
	m68kEmitComputeEAAddr(cb, mode, reg, memory, eaExtPC, instrPC, amd64R10)
	if byteOffset != 0 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(byteOffset))
	}

	bailOffs := make([]int, 0, 8)
	amd64MOV_reg_imm32(cb, amd64RDX, bytesToRead)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)
	for i := uint32(0); i < bytesToRead; i++ {
		if i != 0 {
			amd64SHL_imm(cb, amd64RAX, 8)
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)
		}
		m68kEmitLoadDirectRAMByte(cb, amd64R10, amd64RCX)
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)
	}

	shiftAmount := bytesToRead*8 - bitOffset - width
	if shiftAmount != 0 {
		amd64SHR_imm(cb, amd64RAX, byte(shiftAmount))
	}
	if width < 32 {
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, int32((uint32(1)<<width)-1))
	}
	if signed && width < 32 {
		shift := byte(64 - width)
		amd64SHL_imm(cb, amd64RAX, shift)
		amd64SAR_imm(cb, amd64RAX, shift)
	}
	m68kStoreDataReg(cb, destReg, amd64RAX)

	amd64ALU_reg_imm32(cb, 4, m68kAMD64RegCCR, m68kCCR_X)
	amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
	notZeroOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_Z)
	patchRel32(cb, notZeroOff, cb.Len())
	if width < 32 {
		amd64TEST_reg_imm32(cb, amd64RAX, uint32(1)<<(width-1))
	} else {
		amd64TEST_reg_imm32(cb, amd64RAX, 0x80000000)
	}
	notNegativeOff := amd64Jcc_rel32(cb, amd64CondE)
	amd64ALU_reg_imm32(cb, 1, m68kAMD64RegCCR, m68kCCR_N)
	patchRel32(cb, notNegativeOff, cb.Len())
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}

	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitBFEXTUMemoryImmediate(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	m68kEmitBFEXTMemoryImmediate(cb, ji, memory, startPC, br, instrIdx, false)
}

func m68kEmitBFEXTSMemoryImmediate(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	m68kEmitBFEXTMemoryImmediate(cb, ji, memory, startPC, br, instrIdx, true)
}

func m68kEmitBFFFOMemoryImmediate(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedBFFFOMemoryImmediate(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	ext := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	if ext&(M68K_BF_OFFSET_REG|M68K_BF_WIDTH_REG) != 0 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	destReg := (ext >> 12) & 7
	offset := uint32((ext & M68K_BF_OFFSET_MASK) >> 6)
	width := uint32(ext & M68K_BF_WIDTH_MASK)
	if width == 0 {
		width = 32
	}
	bitOffset := offset & 7
	byteOffset := offset >> 3
	bytesToRead := (bitOffset + width + 7) >> 3
	if bytesToRead == 0 || bytesToRead > 4 {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	eaExtPC := instrPC + 4
	m68kEmitComputeEAAddr(cb, mode, reg, memory, eaExtPC, instrPC, amd64R10)
	if byteOffset != 0 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(byteOffset))
	}

	bailOffs := make([]int, 0, 8)
	amd64MOV_reg_imm32(cb, amd64RDX, bytesToRead)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)
	for i := uint32(0); i < bytesToRead; i++ {
		if i != 0 {
			amd64SHL_imm(cb, amd64RAX, 8)
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)
		}
		m68kEmitLoadDirectRAMByte(cb, amd64R10, amd64RCX)
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)
	}

	shiftAmount := bytesToRead*8 - bitOffset - width
	if shiftAmount != 0 {
		amd64SHR_imm(cb, amd64RAX, byte(shiftAmount))
	}
	if width < 32 {
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, int32((uint32(1)<<width)-1))
	}
	m68kEmitBFFFOResultAndFlags(cb, destReg, offset, width)

	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

// m68kEmitShift emits native shift/rotate forms that have been audited against the interpreter.
func m68kEmitShift(cb *CodeBuffer, opcode uint16) {
	reg := opcode & 7
	countField := (opcode >> 9) & 7
	isRegCount := (opcode >> 5) & 1 // 0=immediate, 1=register
	direction := (opcode >> 8) & 1  // 0=right, 1=left
	shiftType := (opcode >> 3) & 3  // 00=AS, 01=LS, 10=ROX, 11=RO
	size := int((opcode >> 6) & 3)

	if m68kEmitASImmByteWord(cb, opcode) {
		return
	}

	if m68kEmitASRegDn(cb, opcode) {
		return
	}

	if m68kEmitLSRegDn(cb, opcode) {
		return
	}

	if m68kEmitRORegDn(cb, opcode) {
		return
	}

	if m68kEmitROXRegDn(cb, opcode) {
		return
	}

	if shiftType == 2 && isRegCount == 0 && size != 3 {
		m68kEmitROXImmDn(cb, opcode)
		return
	}

	if shiftType == 3 && isRegCount == 0 && size != 3 {
		count := countField
		if count == 0 {
			count = 8
		}
		if size == M68K_SIZE_BYTE && count%8 == 0 {
			return
		}
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		op := byte(0) // ROL
		if direction == 0 {
			op = 1 // ROR
		}
		switch size {
		case M68K_SIZE_BYTE:
			amd64ROT_imm(cb, r, IE64_SIZE_B, op, byte(count))
		case M68K_SIZE_WORD:
			amd64ROT_imm(cb, r, IE64_SIZE_W, op, byte(count))
		default:
			amd64ROT_imm(cb, r, IE64_SIZE_L, op, byte(count))
		}
		amd64SETcc(cb, amd64CondB, amd64R11)
		m68kEmitSizedLogicTest(cb, r, size, amd64RAX)
		m68kStoreDataReg(cb, reg, r)
		m68kEmitCCRLogicWithSavedCarry(cb, amd64R11, r, size)
		return
	}

	if isRegCount == 0 && shiftType == 1 && size != M68K_SIZE_LONG {
		count := countField
		if count == 0 {
			count = 8
		}
		r := m68kResolveDataReg(cb, reg, amd64RAX)
		x86ShiftOp := byte(4) // SHL / LSL
		if direction == 0 {
			x86ShiftOp = 5 // SHR / LSR
		}
		width := byte(8)
		if size == M68K_SIZE_WORD {
			width = 16
		}
		amd64MOV_reg_reg32(cb, amd64R11, r)
		if size == M68K_SIZE_BYTE {
			amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 0xFF)
		} else {
			amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 0xFFFF)
		}
		if byte(count) >= width {
			amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
			amd64SETcc(cb, amd64CondNE, amd64R11)
		} else {
			if direction == 0 {
				if count > 1 {
					amd64ShiftImm32(cb, 5, amd64R11, byte(count-1))
				}
			} else {
				amd64ShiftImm32(cb, 5, amd64R11, width-byte(count))
			}
			amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 1)
		}
		amd64ShiftImmSized(cb, x86ShiftOp, r, size, byte(count))
		amd64XOR_reg_reg32(cb, amd64R10, amd64R10)
		m68kEmitSizedLogicTest(cb, r, size, amd64RAX)
		m68kStoreDataReg(cb, reg, r)
		m68kEmitCCRShiftWithCarryOverflow(cb, amd64R11, amd64R10, r, size)
		return
	}

	if isRegCount == 0 && size == M68K_SIZE_LONG && (shiftType == 0 || shiftType == 1) {
		count := countField
		if count == 0 {
			count = 8
		}
		r := m68kResolveDataReg(cb, reg, amd64RAX)

		amd64MOV_reg_reg32(cb, amd64R11, r)
		amd64XOR_reg_reg32(cb, amd64R10, amd64R10)

		if direction == 0 {
			if count > 1 {
				amd64ShiftImm32(cb, 5, amd64R11, byte(count-1))
			}
			amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 1)
			if shiftType == 0 {
				amd64ShiftImm32(cb, 7, r, byte(count))
			} else {
				amd64ShiftImm32(cb, 5, r, byte(count))
			}
		} else {
			amd64ShiftImm32(cb, 5, amd64R11, byte(32-count))
			if shiftType == 0 {
				amd64TEST_reg_reg32(cb, amd64R11, amd64R11)
				amd64SETcc(cb, amd64CondNE, amd64R11)
				amd64MOV_reg_reg32(cb, amd64R10, amd64R11)
			} else {
				amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 1)
			}
			amd64ShiftImm32(cb, 4, r, byte(count))
		}

		m68kEmitSizedLogicTest(cb, r, M68K_SIZE_LONG, amd64RAX)
		m68kStoreDataReg(cb, reg, r)
		m68kEmitCCRShiftWithCarryOverflow(cb, amd64R11, amd64R10, r, M68K_SIZE_LONG)
		return
	}

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

func m68kEmitMemShiftRotateWord(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedMemShiftRotateWord(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	mode := (opcode >> 3) & 7
	reg := opcode & 7
	bailOffs := []int{}
	if !m68kEmitMemShiftRotateEAAddr(cb, mode, reg, memory, instrPC+2, instrPC, amd64R10, &bailOffs) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}
	amd64TEST_reg_imm8(cb, amd64R10, 1)
	bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
	amd64MOV_reg_imm32(cb, amd64RDX, 2)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	amd64MOV_reg_imm32(cb, amd64RDX, 2)
	m68kEmitSMCRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_WORD)
	amd64MOVZX_W(cb, amd64RAX, amd64RAX)
	amd64MOV_reg_reg32(cb, amd64R11, amd64RAX)
	amd64XOR_reg_reg32(cb, amd64RCX, amd64RCX)

	switch (opcode >> 8) & 7 {
	case 0: // ASR
		amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 1)
		amd64ShiftImm32(cb, 5, amd64R11, 1)
		amd64TEST_reg_imm32(cb, amd64RAX, 0x8000)
		signClear := amd64Jcc_rel32(cb, amd64CondE)
		amd64ALU_reg_imm32_32bit(cb, 1, amd64R11, 0x8000)
		patchRel32(cb, signClear, cb.Len())
	case 1, 3: // ASL / LSL
		amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
		amd64SHR_imm32(cb, amd64RCX, 15)
		amd64ShiftImm32(cb, 4, amd64R11, 1)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 0xFFFF)
	case 2: // LSR
		amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 1)
		amd64ShiftImm32(cb, 5, amd64R11, 1)
	case 4: // ROXR
		amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 1)
		amd64ShiftImm32(cb, 5, amd64R11, 1)
		amd64TEST_reg_imm32(cb, m68kAMD64RegCCR, m68kCCR_X)
		xClear := amd64Jcc_rel32(cb, amd64CondE)
		amd64ALU_reg_imm32_32bit(cb, 1, amd64R11, 0x8000)
		patchRel32(cb, xClear, cb.Len())
	case 5: // ROXL
		amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
		amd64SHR_imm32(cb, amd64RCX, 15)
		amd64ShiftImm32(cb, 4, amd64R11, 1)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 0xFFFF)
		amd64TEST_reg_imm32(cb, m68kAMD64RegCCR, m68kCCR_X)
		xClear := amd64Jcc_rel32(cb, amd64CondE)
		amd64ALU_reg_imm32_32bit(cb, 1, amd64R11, 1)
		patchRel32(cb, xClear, cb.Len())
	case 6: // ROR
		amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 1)
		amd64ShiftImm32(cb, 5, amd64R11, 1)
		amd64TEST_reg_imm32(cb, amd64RAX, 1)
		bitClear := amd64Jcc_rel32(cb, amd64CondE)
		amd64ALU_reg_imm32_32bit(cb, 1, amd64R11, 0x8000)
		patchRel32(cb, bitClear, cb.Len())
	case 7: // ROL
		amd64MOV_reg_reg32(cb, amd64RCX, amd64RAX)
		amd64SHR_imm32(cb, amd64RCX, 15)
		amd64ShiftImm32(cb, 4, amd64R11, 1)
		amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 0xFFFF)
		amd64TEST_reg_imm32(cb, amd64RAX, 0x8000)
		bitClear := amd64Jcc_rel32(cb, amd64CondE)
		amd64ALU_reg_imm32_32bit(cb, 1, amd64R11, 1)
		patchRel32(cb, bitClear, cb.Len())
	}

	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 0xFFFF)
	m68kEmitCCRMemShiftRotateWord(cb, amd64R11, amd64RCX)
	m68kEmitStoreDirectRAM(cb, amd64R10, amd64R11, M68K_SIZE_WORD)
	m68kEmitMemShiftRotateCommitEA(cb, mode, reg, amd64R10)
	m68kPatchFallbackBails(cb, bailOffs, instrPC, br, instrIdx)
}

func m68kEmitMemShiftRotateEAAddr(cb *CodeBuffer, mode, reg uint16, memory []byte, extPC, instrPC uint32, dstReg byte, bailOffs *[]int) bool {
	switch mode {
	case 2, 3:
		r := m68kResolveAddrReg(cb, reg, dstReg)
		if r != dstReg {
			amd64MOV_reg_reg32(cb, dstReg, r)
		}
		return true
	case 4:
		r := m68kResolveAddrReg(cb, reg, dstReg)
		if r != dstReg {
			amd64MOV_reg_reg32(cb, dstReg, r)
		}
		amd64ALU_reg_imm32_32bit(cb, 5, dstReg, int32(m68kStepSize(M68K_SIZE_WORD, reg)))
		return true
	case 5, 7:
		m68kEmitComputeEAAddr(cb, mode, reg, memory, extPC, instrPC, dstReg)
		return true
	case 6:
		return m68kEmitComputeFullIndexEA(cb, mode, reg, memory, extPC, dstReg, bailOffs)
	default:
		return false
	}
}

func m68kEmitMemShiftRotateCommitEA(cb *CodeBuffer, mode, reg uint16, addrReg byte) {
	switch mode {
	case 3:
		ar := m68kResolveAddrReg(cb, reg, amd64RDX)
		amd64ALU_reg_imm32_32bit(cb, 0, ar, int32(m68kStepSize(M68K_SIZE_WORD, reg)))
		m68kStoreAddrReg(cb, reg, ar)
	case 4:
		m68kStoreAddrReg(cb, reg, addrReg)
	}
}

func m68kEmitCCRMemShiftRotateWord(cb *CodeBuffer, resultReg, carryReg byte) {
	if cs := m68kCurrentCS; cs != nil && m68kCCRDeadAtCurrent() {
		return
	}
	amd64MOVZX_B(cb, m68kAMD64RegCCR, carryReg)
	amd64MOVZX_B(cb, amd64RAX, carryReg)
	amd64SHL_imm(cb, amd64RAX, 4)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOV_reg_reg32(cb, amd64RAX, resultReg)
	amd64MOVZX_W(cb, amd64RAX, amd64RAX)
	amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
	amd64SETcc(cb, amd64CondE, amd64RDX)
	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 2)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)

	amd64MOV_reg_reg32(cb, amd64RAX, resultReg)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x8000)
	amd64TEST_reg_reg32(cb, amd64RAX, amd64RAX)
	amd64SETcc(cb, amd64CondNE, amd64RDX)
	amd64MOVZX_B(cb, amd64RAX, amd64RDX)
	amd64SHL_imm(cb, amd64RAX, 3)
	amd64ALU_reg_reg(cb, 0x09, m68kAMD64RegCCR, amd64RAX)
	if cs := m68kCurrentCS; cs != nil {
		cs.flagState = flagsMaterialized
	}
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

func m68kEmitAddrArithA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, isSub bool) {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil || instrPC+uint32(ji.length) > uint32(len(memory)) || !m68kIsNativeSupportedAddrArithA(opcode) {
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		return
	}

	dstReg := (opcode >> 9) & 7
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	opmode := (opcode >> 6) & 7
	size := M68K_SIZE_WORD
	if opmode == 7 {
		size = M68K_SIZE_LONG
	}

	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
			m68kMaterializeCCR(cb, cs)
			cs.flagState = flagsMaterialized
		}
	}

	m68kEmitReadSourceEA(cb, srcMode, srcReg, size, memory, instrPC+2, instrPC, amd64RAX)
	if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
		readOKOff := amd64Jcc_rel32(cb, amd64CondE)
		m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		patchRel32(cb, readOKOff, cb.Len())
	}
	if size == M68K_SIZE_WORD {
		amd64MOVSX_W(cb, amd64RAX, amd64RAX)
	}

	dst := m68kResolveAddrReg(cb, dstReg, amd64RDX)
	if isSub {
		amd64ALU_reg_reg32(cb, 0x29, dst, amd64RAX) // SUB dst, src
	} else {
		amd64ALU_reg_reg32(cb, 0x01, dst, amd64RAX) // ADD dst, src
	}
	m68kStoreAddrReg(cb, dstReg, dst)
}

// ===========================================================================
// Block Compilation
// ===========================================================================

// m68kCompileBlock compiles a scanned block of M68K instructions to x86-64.
func m68kCompileBlock(instrs []M68KJITInstr, startPC uint32, execMem *ExecMem) (*JITBlock, error) {
	return m68kCompileBlockWithMem(instrs, startPC, execMem, nil)
}

func m68kCodeBufferCapacity(instrCount int) int {
	if instrCount <= 0 {
		return 1024
	}
	return 1024 + instrCount*192
}

// m68kJitDiagDisableLoopOpt diagnostic: when IE_M68K_JIT_NO_LOOPOPT=1, backward-branch
// blocks are compiled without the in-block budget loop (each iteration exits and
// re-enters via the dispatcher). Used to bisect whether a divergence lives in the
// loop optimization vs the per-instruction codegen.
var m68kJitDiagDisableLoopOpt = os.Getenv("IE_M68K_JIT_NO_LOOPOPT") == "1"

func m68kJitDiagDisableLoopOptEnabled() bool { return m68kJitDiagDisableLoopOpt }

// m68kCompileBlockWithMem compiles with access to memory for reading branch displacements.
func m68kCompileBlockWithMem(instrs []M68KJITInstr, startPC uint32, execMem *ExecMem, memory []byte) (*JITBlock, error) {
	cb := NewCodeBuffer(m68kCodeBufferCapacity(len(instrs)))

	br := m68kAnalyzeBlockRegs(instrs)
	br.hasBackwardBranch = m68kDetectBackwardBranchesWithMem(instrs, startPC, memory)
	if m68kJitDiagDisableLoopOptEnabled() && br.hasBackwardBranch {
		// Diagnostic: treat backward-branch blocks as non-looping so each
		// committed back-jump exits the block and re-enters via the dispatcher
		// (one chained block per iteration) instead of the in-block budget loop.
		br.hasBackwardBranch = false
	}
	m68kEmitPrologue(cb, startPC, &br)

	// Emit chain entry point (lightweight entry for chained transitions)
	chainEntryOff := m68kEmitChainEntry(cb, &br)
	if guardStart, guardEnd, ok := m68kInstrCoveredRange(startPC, instrs); ok {
		m68kEmitGuestByteStampGuard(cb, memory, [][2]uint32{{guardStart, guardEnd}}, startPC, &br)
	}

	instrOffsets := make([]int, len(instrs))
	var chainExits []m68kChainExitInfo

	// Enable lazy CCR tracking for this block
	var cs m68kCompileState
	m68kCurrentCS = &cs

	// Phase 2c emit-side liveness: compute the per-instruction CCR
	// liveness bitmap up-front and publish it via the package-level
	// pointers the emitCCR_* helpers consult. nil-out at function end
	// so a subsequent block's emit cannot accidentally consult stale
	// state from this block.
	var live []bool
	if jit68KCCRLivenessEnabled {
		live = m68kCCRLiveness(instrs)
		if len(live) > 0 && !m68kIsBlockTerminator(instrs[len(instrs)-1].opcode) {
			live[len(live)-1] = true
		}
	}
	m68kCurrentLive = live
	defer func() { m68kCurrentLive = nil; m68kCurrentInstrIdx = 0 }()

	for i := range instrs {
		m68kCurrentInstrIdx = i

		// Materialize CCR before non-flag instructions that clobber EFLAGS
		if cs.flagState != flagsMaterialized {
			if m68kInstrNeedsCCRMaterialization(&instrs[i]) {
				m68kMaterializeCCR(cb, &cs)
			}
		}

		// Phase 2c dead-producer pre-materialise. If this slot's CCR
		// output is dead per the analyzer, force-materialise any
		// pending live state BEFORE the producer's arithmetic clobbers
		// host EFLAGS — without this, the dead producer's skip would
		// leave R14 stale relative to the prior live producer's
		// output.
		if live != nil && !live[i] && m68kIsCCRProducer(&instrs[i]) {
			if cs.flagState != flagsMaterialized {
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

	// Find the last non-fused instr — fused-leaf markers share the JSR
	// site's pcOffset (length = 0) and emit nothing, so they must be
	// excluded from terminator and end-PC computation.
	lastRealIdx := len(instrs) - 1
	for lastRealIdx >= 0 && instrs[lastRealIdx].fusedFlag&(m68kFusedJSRLeafCall|m68kFusedRTSLeafReturn) != 0 {
		lastRealIdx--
	}

	// If the last real instruction doesn't have its own epilogue, emit fallthrough.
	// Note: m68kCurrentCS must still be active so epilogue can materialize CCR.
	if lastRealIdx < 0 || !m68kIsBlockTerminator(instrs[lastRealIdx].opcode) {
		var endPC uint32
		if lastRealIdx >= 0 {
			lastInstr := &instrs[lastRealIdx]
			endPC = startPC + lastInstr.pcOffset + uint32(lastInstr.length)
		} else {
			endPC = startPC
		}
		// Loop iterations are accounted into ctx.ChainCount at each committed
		// back-jump (see the Bcc/DBcc within-block loop emitters), so the
		// fall-through terminator only reports this block's linear instruction
		// count; the dispatcher sums ChainCount + RetCount.
		m68kEmitRetPC(cb, endPC, uint32(len(instrs)))
		m68kEmitEpilogue(cb, &br)
	}

	m68kCurrentCS = nil

	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, err
	}

	var endPC uint32
	if lastRealIdx >= 0 {
		lastInstr := &instrs[lastRealIdx]
		endPC = startPC + lastInstr.pcOffset + uint32(lastInstr.length)
	} else {
		endPC = startPC
	}

	// Convert code buffer offsets to absolute ExecMem addresses
	chainEntry := addr + uintptr(chainEntryOff)
	var slots []chainSlot
	for _, ce := range chainExits {
		slots = append(slots, chainSlot{
			targetPC:  uint64(ce.targetPC),
			patchAddr: addr + uintptr(ce.jmpDispOffset),
		})
	}

	return &JITBlock{
		startPC:    uint64(startPC),
		endPC:      uint64(endPC),
		instrCount: len(instrs),
		execAddr:   addr,
		execSize:   len(code),
		chainEntry: chainEntry,
		chainSlots: slots,
	}, nil
}

// ===========================================================================
// Region Compilation (Phase 4 sub-phase B.1.b)
// ===========================================================================

// m68kRegion is the compiled-region descriptor produced by m68kFormRegion.
// blocks[i] is the pre-scanned instruction list for block i; blockPCs[i]
// is the guest start PC of that block. entryPC == blockPCs[0].
type m68kRegion struct {
	blocks   [][]M68KJITInstr
	blockPCs []uint32
	entryPC  uint32
}

// m68kFormRegion is the cache-aware region builder consumed by the M68K
// JIT exec loop. It walks the static control-flow graph from hotPC via
// ScanRegionM68K's per-backend rules, then refuses any region whose
// constituent blocks are not safe for region compile (fused-leaf
// markers, fallback-required first instruction, scan failure). Returns
// nil for single-block "regions" — caller falls back to per-block
// compile.
//
// Unlike x86FormRegion this implementation does not gate on cache
// presence: the region is built directly from memory. Cache-presence
// gating can be layered on later if region recompile thrash becomes a
// measured problem.
func m68kFormRegion(hotPC uint32, memory []byte) *m68kRegion {
	res := ScanRegionM68K(memory, hotPC)
	if len(res.BlockPCs) < 2 {
		return nil
	}
	region := &m68kRegion{entryPC: hotPC, blockPCs: res.BlockPCs}
	for _, pc := range res.BlockPCs {
		instrs := m68kScanBlock(memory, pc)
		if len(instrs) == 0 ||
			m68kNeedsConservativeFallback(memory, pc, instrs) ||
			!m68kCanUseProductionNativeBlock(memory, pc, instrs) {
			return nil
		}
		// Reject region if the block contains fused-leaf markers — the
		// region path does not handle the synthetic-RTS bookkeeping that
		// fused-leaf compile depends on.
		for _, ji := range instrs {
			if ji.fusedFlag != 0 {
				return nil
			}
		}
		region.blocks = append(region.blocks, instrs)
	}
	return region
}

// m68kCompileRegion compiles a multi-block region as a single native
// JITBlock. Mirrors x86CompileRegion's structure but without Tier-2 reg
// alloc (B.1.b is region-compile-only; reg-map promotion is B.1.c).
//
// Internal jumps (chain exits whose targetPC matches an in-region
// block start) are post-processed: the chain exit's patchable JMP is
// rewritten to jump to the in-region block's local label, eliminating
// the dispatcher round-trip. The chain exit's CCR-materialise +
// ChainCount-accum + ChainBudget-decrement still runs, so the harness's
// per-block instruction accounting and SMC budget remain correct.
//
// External chain exits (targetPC outside the region) are exposed via
// chainSlots as in the per-block compiler. Region SMC invalidation is
// inherited from the per-block path: when the code-page bitmap fires
// inside any region page, the entire JIT cache is invalidated by the
// exec loop's NeedInval handler — no region-specific bookkeeping
// required because the region is itself a single cache entry that
// gets dropped along with the rest.
//
// CCR liveness analysis is conservative across the region (every CCR
// producer materialises). A future B.1.c-level pass can wire a
// region-aware liveness once the cross-block analyzer lands.
func m68kCompileRegion(region *m68kRegion, execMem *ExecMem, memory []byte) (*JITBlock, error) {
	if region == nil || len(region.blocks) < 2 {
		return nil, errors.New("m68kCompileRegion: region has fewer than 2 blocks")
	}

	// Concatenate instructions for whole-region register analysis.
	var allInstrs []M68KJITInstr
	for _, blk := range region.blocks {
		allInstrs = append(allInstrs, blk...)
	}
	br := m68kAnalyzeBlockRegs(allInstrs)
	// Conservative: cross-block back-edge detection is not yet
	// region-aware (m68kDetectBackwardBranchesWithMem is per-block-PC).
	// Force-true so the prologue emits the full backward-branch
	// scaffolding; the small extra cost is bounded and correct.
	br.hasBackwardBranch = true

	cb := NewCodeBuffer(m68kCodeBufferCapacity(len(allInstrs)))
	m68kEmitPrologue(cb, region.entryPC, &br)
	chainEntryOff := m68kEmitChainEntry(cb, &br)

	var cs m68kCompileState
	m68kCurrentCS = &cs
	defer func() { m68kCurrentCS = nil }()

	// Disable per-instruction CCR liveness for the region path —
	// m68kCCRLiveness operates per-block and would mis-classify producers
	// whose consumers live in a later region block. Conservative
	// materialise on every consumer.
	prevLive := m68kCurrentLive
	m68kCurrentLive = nil
	defer func() {
		m68kCurrentLive = prevLive
		m68kCurrentInstrIdx = 0
	}()

	var allChainExits []m68kChainExitInfo
	blockLabels := make([]int, len(region.blocks))
	totalInstrCount := 0

	prevBase := m68kCurrentInstrCountBase
	defer func() { m68kCurrentInstrCountBase = prevBase }()

	for bi, blk := range region.blocks {
		blockLabels[bi] = cb.Len()
		if guardStart, guardEnd, ok := m68kInstrCoveredRange(region.blockPCs[bi], blk); ok {
			m68kEmitGuestByteStampGuard(cb, memory, [][2]uint32{{guardStart, guardEnd}}, region.blockPCs[bi], &br)
		}
		// Set the per-block instruction-count base so every emit-site
		// RetPC write (IO bail, RTS-cache miss, dynamic JMP/JSR exit,
		// etc.) reports cumulative-across-region instructions retired —
		// not just the current block's count. Without this, a late-block
		// exit would write a small RetCount and the dispatcher would
		// undercount executed instructions (it ignores ChainCount when
		// RetCount is nonzero).
		m68kCurrentInstrCountBase = uint32(totalInstrCount)

		instrOffsets := make([]int, len(blk))
		for i := range blk {
			m68kCurrentInstrIdx = i

			if cs.flagState != flagsMaterialized {
				if m68kInstrNeedsCCRMaterialization(&blk[i]) {
					m68kMaterializeCCR(cb, &cs)
				}
			}

			instrOffsets[i] = cb.Len()
			ji := &blk[i]
			m68kEmitInstructionFull(cb, ji, region.blockPCs[bi], &br, i, len(blk), memory, instrOffsets, blk, &allChainExits)

			if !m68kIsBlockTerminator(ji.opcode) && m68kInstrMaySetGenericIOFallback(ji) {
				instrPC := region.blockPCs[bi] + ji.pcOffset
				if cs.flagState != flagsMaterialized {
					m68kMaterializeCCR(cb, &cs)
				}
				amd64ALU_mem_imm8(cb, 7, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 0)
				noIOBailOff := amd64Jcc_rel32(cb, amd64CondE)
				// Per-block i passes through to the emit-base machinery
				// (m68kCurrentInstrCountBase + i = global retire count).
				m68kEmitRetPC(cb, instrPC, uint32(i))
				m68kEmitEpilogue(cb, &br)
				patchRel32(cb, noIOBailOff, cb.Len())
			}
		}
		totalInstrCount += len(blk)
	}

	// Patch in-region chain exits to internal block labels. Mark the
	// jmpDispOffset as -1 so the post-write chainSlots loop skips them.
	for i := range allChainExits {
		ce := &allChainExits[i]
		for bi, bpc := range region.blockPCs {
			if bpc == ce.targetPC {
				patchRel32(cb, ce.jmpDispOffset, blockLabels[bi])
				allChainExits[i].jmpDispOffset = -1
				break
			}
		}
	}

	// Defensive fall-through epilogue if the last real instruction is
	// not a terminator (region scanner stops at terminators, so this
	// should not normally fire — included as a safety net for any
	// future scanner change that admits fall-through tails).
	// Clear the per-block base before this emit so the explicit
	// totalInstrCount value lands as RetCount unmodified.
	m68kCurrentInstrCountBase = 0
	lastBlock := region.blocks[len(region.blocks)-1]
	lastInstr := &lastBlock[len(lastBlock)-1]
	if !m68kIsBlockTerminator(lastInstr.opcode) {
		endPC := region.blockPCs[len(region.blocks)-1] + lastInstr.pcOffset + uint32(lastInstr.length)
		m68kEmitRetPC(cb, endPC, uint32(totalInstrCount))
		m68kEmitEpilogue(cb, &br)
	}

	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, err
	}

	var slots []chainSlot
	for _, ce := range allChainExits {
		if ce.jmpDispOffset < 0 {
			continue
		}
		slots = append(slots, chainSlot{
			targetPC:  uint64(ce.targetPC),
			patchAddr: addr + uintptr(ce.jmpDispOffset),
		})
	}

	endPC := region.blockPCs[len(region.blocks)-1] + lastInstr.pcOffset + uint32(lastInstr.length)

	// coveredRanges enumerates every guest [start, end) span the
	// region's native code was compiled from. Required for SMC
	// invalidation correctness when the region follows non-monotonic
	// static targets — e.g. 0x100→0x5000→0x200 has a canonical
	// [startPC, endPC) of [0x100, 0x202) which would silently miss
	// guest writes to 0x5000. CodeCache.InvalidateRange and the
	// exec-loop code-page bitmap walk consult this list.
	covered := make([][2]uint64, 0, len(region.blocks))
	for bi, blk := range region.blocks {
		if len(blk) == 0 {
			continue
		}
		blockStart := region.blockPCs[bi]
		lastJI := &blk[len(blk)-1]
		blockEnd := blockStart + lastJI.pcOffset + uint32(lastJI.length)
		covered = append(covered, [2]uint64{uint64(blockStart), uint64(blockEnd)})
	}

	return &JITBlock{
		startPC:       uint64(region.entryPC),
		endPC:         uint64(endPC),
		instrCount:    totalInstrCount,
		execAddr:      addr,
		execSize:      len(code),
		chainEntry:    addr + uintptr(chainEntryOff),
		chainSlots:    slots,
		coveredRanges: covered,
	}, nil
}

// m68kEmitInstructionFull dispatches to the appropriate emitter, with full context.
func m68kEmitInstructionFull(cb *CodeBuffer, ji *M68KJITInstr, blockStartPC uint32, br *m68kBlockRegs, instrIdx int, blockLen int, memory []byte, instrOffsets []int, instrs []M68KJITInstr, chainSlots *[]m68kChainExitInfo) {
	// Fused-leaf markers preserve the architectural stack push/pop the
	// unfused JSR/RTS pair would have executed (A7 -= 4 + Write32_BE on
	// JSR, Read32_BE + A7 += 4 on RTS, both with I/O bounds bail). Only
	// the chain-dispatch overhead — block transition, RTS cache probe,
	// chain budget bookkeeping — is elided. Guest-visible stack memory
	// traffic and any I/O-bail semantics match the unfused interpreter
	// path.
	if ji.fusedFlag&m68kFusedJSRLeafCall != 0 {
		instrPC := blockStartPC + ji.pcOffset
		returnPC := instrPC + uint32(ji.length)
		// Materialize lazy CCR before clobbering EFLAGS via the I/O CMP
		// and A7 SUB/ADD path.
		if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
			m68kMaterializeCCR(cb, cs)
		}
		// Push return address: A7 -= 4; range/I/O check; Write32_BE.
		amd64ALU_reg_imm32_32bit(cb, 5, m68kAMD64RegA7, 4)
		bailOffs := make([]int, 0, 10)
		amd64TEST_reg_imm8(cb, m68kAMD64RegA7, 1)
		bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
		amd64MOV_reg_imm32(cb, amd64RDX, 4)
		m68kEmitMemRangeBailChecks(cb, m68kAMD64RegA7, amd64RDX, &bailOffs)
		m68kEmitSMCRangeBailChecks(cb, m68kAMD64RegA7, amd64RDX, &bailOffs)
		amd64MOV_reg_imm32(cb, amd64RAX, returnPC)
		emitREX(cb, false, 0, amd64RAX)
		cb.EmitBytes(0x0F, 0xC8+regBits(amd64RAX)) // BSWAP EAX
		emitMemOpSIB(cb, false, 0x89, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)
		// Continue into next instr in this block (the leaf body).
		successOff := amd64JMP_rel32(cb)
		// Bail path: undo SUB, set NeedIOFallback, exit so dispatcher
		// re-executes the JSR via interpreter.
		for _, off := range bailOffs {
			patchRel32(cb, off, cb.Len())
		}
		amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4) // undo SUB
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		// Success label — fall through to the next instr.
		patchRel32(cb, successOff, cb.Len())
		return
	}
	if ji.fusedFlag&m68kFusedRTSLeafReturn != 0 {
		instrPC := blockStartPC + ji.pcOffset
		// Materialize lazy CCR before clobbering EFLAGS via the I/O CMP
		// and A7 ADD path.
		if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
			m68kMaterializeCCR(cb, cs)
		}
		// Pop return address: range/I/O check; Read32_BE; A7 += 4. Value is
		// discarded — control flow naturally continues at the JSR's
		// returnPC (the next instr in the block).
		bailOffs := make([]int, 0, 10)
		amd64TEST_reg_imm8(cb, m68kAMD64RegA7, 1)
		bailOffs = append(bailOffs, amd64Jcc_rel32(cb, amd64CondNE))
		amd64MOV_reg_imm32(cb, amd64RDX, 4)
		m68kEmitMemRangeBailChecks(cb, m68kAMD64RegA7, amd64RDX, &bailOffs)
		emitMemOpSIB(cb, false, 0x8B, amd64RAX, m68kAMD64RegMemBase, m68kAMD64RegA7, 0)
		amd64ALU_reg_imm32_32bit(cb, 0, m68kAMD64RegA7, 4) // ADD A7, 4
		successOff := amd64JMP_rel32(cb)
		for _, off := range bailOffs {
			patchRel32(cb, off, cb.Len())
		}
		amd64MOV_mem_imm32(cb, m68kAMD64RegCtx, int32(m68kCtxOffNeedIOFallback), 1)
		m68kEmitRetPC(cb, instrPC, uint32(instrIdx))
		m68kEmitEpilogue(cb, br)
		patchRel32(cb, successOff, cb.Len())
		return
	}
	opcode := ji.opcode
	group := opcode >> 12

	switch group {
	case 0x0: // Immediate/bit manipulation group
		if opcode&0xFFF0 == 0x06C0 {
			instrPC := blockStartPC + ji.pcOffset
			m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperRTM)
			return
		}
		if opcode&0xFFC0 == 0x06C0 && ((opcode>>3)&7) >= M68K_AM_AR_IND {
			instrPC := blockStartPC + ji.pcOffset
			m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperCALLM)
			return
		}
		if opcode == 0x0CFC || opcode == 0x0EFC || (opcode&0xF9C0 == 0x08C0 && opcode&0x0600 != 0) {
			instrPC := blockStartPC + ji.pcOffset
			m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperCASCAS2)
			return
		}
		if opcode&0xFF00 == 0x0E00 {
			instrPC := blockStartPC + ji.pcOffset
			m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperMOVES)
			return
		}
		if opcode&0xF9C0 == 0x00C0 {
			instrPC := blockStartPC + ji.pcOffset
			m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperCHK2CMP2)
			return
		}
		if opcode&0xF138 == 0x0108 {
			m68kEmitMOVEP(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsBTSTDynamic(opcode) || m68kIsBTSTImmediate(opcode) {
			m68kEmitBTST(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsBitModifyDynamic(opcode) || m68kIsBitModifyImmediate(opcode) {
			m68kEmitBitModify(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if opcode == 0x023C || opcode == 0x027C || opcode == 0x003C || opcode == 0x007C || opcode == 0x0A3C || opcode == 0x0A7C {
			if memory != nil {
				m68kEmitImmediateToSRCCR(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		if m68kIsImmediateLogicDn(opcode) {
			if memory != nil {
				m68kEmitImmediateLogicDn(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		if m68kIsNativeSupportedImmediateLogicEA(opcode) {
			if memory != nil {
				m68kEmitImmediateLogicEA(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		if m68kIsNativeSupportedImmediateArithmeticEA(opcode) {
			if memory != nil {
				m68kEmitImmediateArithmeticEA(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		if m68kIsImmediateArithmeticDn(opcode) {
			if memory != nil {
				m68kEmitImmediateArithmeticDn(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		if m68kIsBTSTImmDn(opcode) {
			if memory != nil {
				m68kEmitBTSTImmDn(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		if m68kIsBTSTImmAnDisp(opcode) {
			if memory != nil {
				m68kEmitBTSTImmAnDisp(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		if opcode&0xFF00 == 0x0C00 { // CMPI.{B,W,L} #imm,<ea>
			if memory != nil {
				m68kEmitCMPI(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}

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
		if m68kIsNativeSupportedMOVEA(opcode) {
			m68kEmitMOVEA_Direct(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedMOVEGuarded(opcode) {
			m68kEmitMOVE_Guarded(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if group == 0x2 && m68kIsMoveLongStackDispToReg(opcode) {
			m68kEmitMOVE_LongAnDispToReg(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if group == 0x2 && m68kIsMoveLongRegToStackDisp(opcode) {
			m68kEmitMOVE_LongRegToStackDisp(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if group == 0x2 && m68kIsMoveLongRegToStackPredec(opcode) {
			m68kEmitMOVE_LongRegToStackPredec(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		srcMode := (opcode >> 3) & 7
		dstMode := (opcode >> 6) & 7
		if srcMode == 0 && dstMode == 0 { // Dn -> Dn
			m68kEmitMOVE_Dn_Dn(cb, opcode, size)
			return
		}
		if m68kIsMovePostincPostinc(opcode) {
			m68kEmitMOVE_PostincPostinc(cb, ji, blockStartPC, size, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedMOVEMemToMemGuarded(opcode) {
			m68kEmitMOVE_MemToMemGuarded(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// General MOVE with any EA (requires memory for extension words)
		if memory != nil {
			m68kEmitMOVE_Full(cb, ji, memory, blockStartPC, size, br, instrIdx)
			return
		}

	case 0xD: // ADD/ADDA/ADDX
		opmode := (opcode >> 6) & 7
		srcMode := (opcode >> 3) & 7
		if opcode&0xF130 == 0xD100 && opcode&0x00C0 != 0x00C0 {
			if srcMode&1 == 0 {
				m68kEmitADDXSUBX_Dn_Dn(cb, opcode, false)
			} else {
				m68kEmitADDXSUBX_PredecMem(cb, ji, blockStartPC, br, instrIdx, false)
			}
			return
		}
		if (opmode == 3 || opmode == 7) && m68kIsNativeSupportedAddrArithA(opcode) { // ADDA
			m68kEmitAddrArithA(cb, ji, memory, blockStartPC, br, instrIdx, false)
			return
		}
		if opmode <= 2 { // ADD.x <ea>,Dn
			if srcMode == 0 { // Dn,Dn fast path
				m68kEmitADD_Dn_Dn(cb, opcode)
			} else if memory != nil { // EA,Dn with memory access
				m68kEmitADD_EA_Dn(cb, ji, memory, blockStartPC, br, instrIdx)
			}
			return
		}
		if opmode >= 4 && opmode <= 6 && m68kIsNativeSupportedArithDnToEA(opcode) {
			m68kEmitArith_Dn_EA(cb, ji, memory, blockStartPC, br, instrIdx, false)
			return
		}

	case 0x9: // SUB/SUBA/SUBX
		opmode := (opcode >> 6) & 7
		srcMode := (opcode >> 3) & 7
		if opcode&0xF130 == 0x9100 && opcode&0x00C0 != 0x00C0 {
			if srcMode&1 == 0 {
				m68kEmitADDXSUBX_Dn_Dn(cb, opcode, true)
			} else {
				m68kEmitADDXSUBX_PredecMem(cb, ji, blockStartPC, br, instrIdx, true)
			}
			return
		}
		if (opmode == 3 || opmode == 7) && m68kIsNativeSupportedAddrArithA(opcode) { // SUBA
			m68kEmitAddrArithA(cb, ji, memory, blockStartPC, br, instrIdx, true)
			return
		}
		if opmode <= 2 { // SUB.x <ea>,Dn
			if srcMode == 0 {
				m68kEmitSUB_Dn_Dn(cb, opcode)
			} else if memory != nil {
				m68kEmitSUB_EA_Dn(cb, ji, memory, blockStartPC, br, instrIdx)
			}
			return
		}
		if opmode >= 4 && opmode <= 6 && m68kIsNativeSupportedArithDnToEA(opcode) {
			m68kEmitArith_Dn_EA(cb, ji, memory, blockStartPC, br, instrIdx, true)
			return
		}

	case 0xB: // CMP/EOR
		if m68kIsNativeSupportedCMPM(opcode) {
			m68kEmitCMPM(cb, ji, blockStartPC, br, instrIdx)
			return
		}
		opmode := (opcode >> 6) & 7
		srcMode := (opcode >> 3) & 7
		if (opmode == 3 || opmode == 7) && m68kIsNativeSupportedCMPA(opcode) {
			m68kEmitCMPA(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if opmode <= 2 { // CMP.x <ea>,Dn
			if srcMode == 0 {
				m68kEmitCMP_Dn_Dn(cb, opcode)
			} else if memory != nil {
				m68kEmitCMP_EA_Dn(cb, ji, memory, blockStartPC, br, instrIdx)
			}
			return
		}
		if opmode >= 4 && opmode <= 6 && (srcMode == 0 || m68kIsNativeSupportedLogicDnToEA(opcode)) { // EOR Dn,<ea>
			if srcMode == 0 {
				m68kEmitEOR_Dn_Dn(cb, opcode)
			} else {
				m68kEmitLogic_Dn_EA(cb, ji, memory, blockStartPC, br, instrIdx, 0x30, 0x31)
			}
			return
		}

	case 0xC: // AND
		if opcode&0xF1F0 == 0xC100 {
			if (opcode>>3)&1 == 0 {
				m68kEmitABCDSBCD_Dn_Dn(cb, opcode, false)
			} else {
				m68kEmitABCDSBCD_PredecMem(cb, ji, blockStartPC, br, instrIdx, false)
			}
			return
		}
		if m68kIsEXG(opcode) {
			m68kEmitEXG(cb, opcode)
			return
		}
		if opcode&0xF0C0 == 0xC0C0 {
			if m68kIsNativeSupportedMULW(opcode) {
				m68kEmitMULW_EA_Dn(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		srcMode := (opcode >> 3) & 7
		opmode := (opcode >> 6) & 7
		if opmode <= 2 && m68kIsNativeSupportedLogicEAToDn(opcode) {
			if srcMode == 0 {
				m68kEmitAND_Dn_Dn(cb, opcode)
			} else {
				m68kEmitLogic_EA_Dn(cb, ji, memory, blockStartPC, br, instrIdx, 0x20, 0x21)
			}
			return
		}
		if opmode >= 4 && opmode <= 6 && m68kIsNativeSupportedLogicDnToEA(opcode) {
			m68kEmitLogic_Dn_EA(cb, ji, memory, blockStartPC, br, instrIdx, 0x20, 0x21)
			return
		}

	case 0x8: // OR
		if opcode&0xF1F0 == 0x8100 {
			if (opcode>>3)&1 == 0 {
				m68kEmitABCDSBCD_Dn_Dn(cb, opcode, true)
			} else {
				m68kEmitABCDSBCD_PredecMem(cb, ji, blockStartPC, br, instrIdx, true)
			}
			return
		}
		if m68kIsNativeSupportedPACKUNPK(opcode) {
			if ((opcode >> 3) & 1) == 0 {
				m68kEmitPACKUNPKRegister(cb, ji, memory, blockStartPC, br, instrIdx)
			} else {
				m68kEmitPACKUNPKPredecMem(cb, ji, memory, blockStartPC, br, instrIdx)
			}
			return
		}
		if opcode&0xF0C0 == 0x80C0 {
			if m68kIsNativeSupportedDIVW(opcode) {
				m68kEmitDIVW_EA_Dn(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		srcMode := (opcode >> 3) & 7
		opmode := (opcode >> 6) & 7
		if opmode <= 2 && m68kIsNativeSupportedLogicEAToDn(opcode) {
			if srcMode == 0 {
				m68kEmitOR_Dn_Dn(cb, opcode)
			} else {
				m68kEmitLogic_EA_Dn(cb, ji, memory, blockStartPC, br, instrIdx, 0x08, 0x09)
			}
			return
		}
		if opmode >= 4 && opmode <= 6 && m68kIsNativeSupportedLogicDnToEA(opcode) {
			m68kEmitLogic_Dn_EA(cb, ji, memory, blockStartPC, br, instrIdx, 0x08, 0x09)
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
		if opcode&0xF1C0 == 0x4180 || opcode&0xF1C0 == 0x4100 {
			m68kEmitCHK(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if (opcode&0xFFC0 == 0x40C0 || opcode&0xFFC0 == 0x42C0) && m68kIsNativeSupportedMOVEFromStatus(opcode) {
			m68kEmitMOVEFromStatus(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if opcode&0xFFC0 == 0x44C0 {
			if m68kIsNativeSupportedMOVEToCCR(opcode) {
				m68kEmitMOVEToCCR(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		if opcode&0xFFC0 == 0x46C0 {
			if m68kIsNativeSupportedMOVEToSR(opcode) {
				m68kEmitMOVEToSR(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		if opcode&0xFFF0 == 0x4E60 {
			m68kEmitMOVEUSP(cb, ji, blockStartPC, br, instrIdx)
			return
		}
		if opcode&0xFFFE == 0x4E7A {
			m68kEmitMOVEC(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// NEGX
		if opcode&0xFF00 == 0x4000 {
			m68kEmitNEGX(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// CLR
		if opcode&0xFF00 == 0x4200 {
			m68kEmitCLR(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// NOT
		if opcode&0xFF00 == 0x4600 {
			m68kEmitNOT(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// NEG
		if opcode&0xFF00 == 0x4400 {
			m68kEmitNEG(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// NBCD
		if opcode&0xFFC0 == 0x4800 {
			m68kEmitNBCD(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// TAS
		if opcode&0xFFC0 == 0x4AC0 {
			m68kEmitTAS(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// TST
		if opcode&0xFF00 == 0x4A00 {
			if memory != nil && m68kIsNativeSupportedTST(opcode) {
				m68kEmitTST(cb, ji, memory, blockStartPC, br, instrIdx)
				return
			}
		}
		// SWAP
		if opcode&0xFFF8 == 0x4840 {
			m68kEmitSWAP(cb, opcode)
			return
		}
		// BKPT
		if opcode&0xFFF8 == 0x4848 {
			instrPC := blockStartPC + ji.pcOffset
			m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperBKPT)
			return
		}
		// EXT/EXTB
		if opcode&0xFFF8 == 0x4880 || opcode&0xFFF8 == 0x48C0 || opcode&0xFFF8 == 0x49C0 {
			m68kEmitEXT(cb, opcode)
			return
		}
		// MOVEM (memory <-> register list). EXT above already absorbed
		// the eaMode=0 (Dn) cases that share the 0x4880/0x48C0 base, so
		// the remaining 0xFB80 == 0x4880 patterns are genuine MOVEM.
		if memory != nil && m68kIsNativeSupportedMOVEM(opcode) {
			m68kEmitMOVEM_PreInc(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// TRAP #n
		if opcode&0xFFF0 == 0x4E40 {
			m68kEmitTRAP(cb, ji, blockStartPC, br, instrIdx)
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
		// RTE
		if opcode == 0x4E73 {
			m68kEmitRTE(cb, ji, blockStartPC, br, instrIdx)
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
				m68kEmitLEA(cb, ji, memory, blockStartPC, br, instrIdx)
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
		// Bail terminators: STOP, RTR, RESET, TRAPV
		// These are block terminators that can't be JIT-compiled.
		// Emit bail to interpreter so dispatcher re-executes via StepOne().
		if opcode == 0x4E72 || opcode == 0x4E73 || opcode == 0x4E77 ||
			opcode == 0x4E70 || opcode == 0x4E76 {
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
		// TRAPcc
		if opcode&0xF0F8 == 0x50F8 && opcode&7 >= 2 && opcode&7 <= 4 {
			instrPC := blockStartPC + ji.pcOffset
			m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperTRAPcc)
			return
		}
		// Scc
		if opcode&0x00C0 == 0x00C0 && m68kIsNativeSupportedScc(opcode) {
			if !m68kIsNativeSupportedScc(opcode) {
				m68kEmitFallbackAtInstr(cb, blockStartPC+ji.pcOffset, br, instrIdx)
				return
			}
			if (opcode>>3)&7 == 0 {
				m68kEmitScc(cb, opcode)
			} else {
				m68kEmitSccMemGuarded(cb, ji, memory, blockStartPC, br, instrIdx)
			}
			return
		}
		// ADDQ/SUBQ (size field != 3)
		if opcode&0x00C0 != 0x00C0 {
			mode := (opcode >> 3) & 0x7
			// JIT-safe today:
			// - ADDQ/SUBQ An (size encoding ignored by ISA, no flags)
			// - ADDQ/SUBQ Dn with byte/word/long width
			// - ADDQ/SUBQ writable memory EAs with guarded memory read/write
			if mode == 1 || mode == 0 {
				if opcode&0x0100 == 0 { // ADDQ
					m68kEmitADDQ(cb, opcode)
				} else { // SUBQ
					m68kEmitSUBQ(cb, opcode)
				}
				return
			}
			if mode == 2 || mode == 3 || mode == 4 || mode == 5 || mode == 6 || mode == 7 {
				m68kEmitADDQSUBQMemDispAn(cb, ji, memory, blockStartPC, br, instrIdx, opcode&0x0100 != 0)
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

	case 0xF: // FPU: native reg-to-reg arithmetic, else helper, else interpreter.
		instrPC := blockStartPC + ji.pcOffset
		// Native path: register-to-register arithmetic emitted inline in SSE2.
		// The command word is the second instruction word at instrPC+2.
		if cmdAddr := int(instrPC) + 2; cmdAddr+1 < len(memory) {
			cmdWord := uint16(memory[cmdAddr])<<8 | uint16(memory[cmdAddr+1])
			if op, src, dst, precision, ok := m68kDecodeNativeFPURegToReg(opcode, cmdWord); ok {
				if m68kEmitNativeFPUInstr(cb, op, src, dst, precision, instrPC, br, instrIdx) {
					return
				}
			}
		}
		if m68kIsJITHelperSupportedFPU(opcode) {
			m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperFPU)
		} else {
			m68kEmitFallbackAtInstr(cb, instrPC, br, instrIdx)
		}
		return

	case 0xE: // Shifts, Rotates, Bit Fields
		if m68kIsNativeSupportedBFTSTRegister(opcode) {
			m68kEmitBFTSTRegister(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedBFTSTMemoryImmediate(opcode) {
			m68kEmitBFTSTMemoryImmediate(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedBFEXTURegister(opcode) {
			m68kEmitBFEXTURegister(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedBFEXTUMemoryImmediate(opcode) {
			m68kEmitBFEXTUMemoryImmediate(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedBFEXTSRegister(opcode) {
			m68kEmitBFEXTSRegister(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedBFEXTSMemoryImmediate(opcode) {
			m68kEmitBFEXTSMemoryImmediate(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedBFFFORegister(opcode) {
			m68kEmitBFFFORegister(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedBFFFOMemoryImmediate(opcode) {
			m68kEmitBFFFOMemoryImmediate(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedBFWriteRegisterImmediate(opcode) {
			m68kEmitBFWriteRegisterImmediate(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedBFWriteMemoryImmediate(opcode) {
			m68kEmitBFWriteMemoryImmediate(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		if m68kIsNativeSupportedMemShiftRotateWord(opcode) {
			m68kEmitMemShiftRotateWord(cb, ji, memory, blockStartPC, br, instrIdx)
			return
		}
		// Only audited shift/rotate shapes may run natively. The generic
		// shift emitter uses host x86 flag/count behavior and is not exact
		// for the remaining ASx/LSx forms.
		if m68kInstrProductionNativeSafe(ji) {
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
