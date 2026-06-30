//go:build amd64 && (linux || windows || darwin)

package main

// SSE2 scalar-double encoders for the native M68K FPU JIT path. The 68881 FP
// register file is stored as float64 (fpu_m68881.go); these instructions are
// exactly what Go's float64 arithmetic lowers to, so emitting them is
// bit-identical to the interpreter. All are baseline SSE2 (present on every
// x86-64), so no GOAMD64/CPUID gating is required. FMA is deliberately avoided:
// it fuses with a single rounding and would diverge from the two-step
// interpreter semantics.
//
// Encoding form: F2 (mandatory prefix), REX (if needed), 0F, opcode, ModRM
// [SIB] [disp]. xmm registers are 0-15; emitREX supplies REX.R/REX.B for 8-15.

const (
	sseOpMOVSDload  = 0x10 // MOVSD xmm, xmm/m64
	sseOpMOVSDstore = 0x11 // MOVSD xmm/m64, xmm
	sseOpSQRTSD     = 0x51
	sseOpADDSD      = 0x58
	sseOpMULSD      = 0x59
	sseOpSUBSD      = 0x5C
	sseOpDIVSD      = 0x5E
)

// amd64SSEsd_rr emits a register-to-register scalar-double op: <op> xmmDst, xmmSrc.
func amd64SSEsd_rr(cb *CodeBuffer, opcode, dst, src byte) {
	cb.EmitBytes(0xF2)
	emitREX(cb, false, dst, src)
	cb.EmitBytes(0x0F, opcode, modRM(3, dst, src))
}

// amd64SSEsd_rm emits a register/memory scalar-double op with reg field `reg`
// (an xmm) addressing [base+disp]. Used for both loads (reg=dst, opcode 0x10 /
// arithmetic) and stores (reg=src, opcode 0x11). Mirrors emitMemOp's addressing
// tail but injects the F2 prefix and 0F escape required by SSE.
func amd64SSEsd_rm(cb *CodeBuffer, opcode, reg, base byte, disp int32) {
	cb.EmitBytes(0xF2)
	emitREX(cb, false, reg, base)
	cb.EmitBytes(0x0F, opcode)
	emitModRMDisp(cb, reg, base, disp)
}

// emitModRMDisp emits the ModRM byte, optional SIB, and displacement for a
// register operand `reg` addressing memory at [base+disp]. This is the
// addressing tail shared by emitMemOp; REX and the opcode are emitted by the
// caller (SSE needs an F2 prefix and 0F escape between REX and opcode, which
// emitMemOp cannot express).
func emitModRMDisp(cb *CodeBuffer, reg, base byte, disp int32) {
	baseBits := regBits(base)
	needsSIB := baseBits == 4 // RSP/R12 low bits = 4 require a SIB byte
	switch {
	case disp == 0 && baseBits != 5: // RBP/R13 (low bits 5) always need a disp
		if needsSIB {
			cb.EmitBytes(modRM(0, reg, 4), sibByte(0, 4, base))
		} else {
			cb.EmitBytes(modRM(0, reg, base))
		}
	case disp >= -128 && disp <= 127:
		if needsSIB {
			cb.EmitBytes(modRM(1, reg, 4), sibByte(0, 4, base), byte(int8(disp)))
		} else {
			cb.EmitBytes(modRM(1, reg, base), byte(int8(disp)))
		}
	default:
		if needsSIB {
			cb.EmitBytes(modRM(2, reg, 4), sibByte(0, 4, base))
		} else {
			cb.EmitBytes(modRM(2, reg, base))
		}
		cb.Emit32(uint32(disp))
	}
}

// Register-to-register named wrappers.
func amd64ADDSD_rr(cb *CodeBuffer, dst, src byte)  { amd64SSEsd_rr(cb, sseOpADDSD, dst, src) }
func amd64SUBSD_rr(cb *CodeBuffer, dst, src byte)  { amd64SSEsd_rr(cb, sseOpSUBSD, dst, src) }
func amd64MULSD_rr(cb *CodeBuffer, dst, src byte)  { amd64SSEsd_rr(cb, sseOpMULSD, dst, src) }
func amd64DIVSD_rr(cb *CodeBuffer, dst, src byte)  { amd64SSEsd_rr(cb, sseOpDIVSD, dst, src) }
func amd64SQRTSD_rr(cb *CodeBuffer, dst, src byte) { amd64SSEsd_rr(cb, sseOpSQRTSD, dst, src) }
func amd64MOVSD_rr(cb *CodeBuffer, dst, src byte)  { amd64SSEsd_rr(cb, sseOpMOVSDload, dst, src) }

// Memory-form named wrappers.
func amd64MOVSD_load(cb *CodeBuffer, dst, base byte, disp int32) {
	amd64SSEsd_rm(cb, sseOpMOVSDload, dst, base, disp)
}
func amd64MOVSD_store(cb *CodeBuffer, base byte, disp int32, src byte) {
	amd64SSEsd_rm(cb, sseOpMOVSDstore, src, base, disp)
}
func amd64ADDSD_rm(cb *CodeBuffer, dst, base byte, disp int32) {
	amd64SSEsd_rm(cb, sseOpADDSD, dst, base, disp)
}
func amd64SUBSD_rm(cb *CodeBuffer, dst, base byte, disp int32) {
	amd64SSEsd_rm(cb, sseOpSUBSD, dst, base, disp)
}
func amd64MULSD_rm(cb *CodeBuffer, dst, base byte, disp int32) {
	amd64SSEsd_rm(cb, sseOpMULSD, dst, base, disp)
}
func amd64DIVSD_rm(cb *CodeBuffer, dst, base byte, disp int32) {
	amd64SSEsd_rm(cb, sseOpDIVSD, dst, base, disp)
}
func amd64SQRTSD_rm(cb *CodeBuffer, dst, base byte, disp int32) {
	amd64SSEsd_rm(cb, sseOpSQRTSD, dst, base, disp)
}

// Conversion encoders for single-precision rounding (FSxxx opmodes). Rounding a
// double through float32 is cvtsd2ss followed by cvtss2sd, matching
// applyFPUResultPrecision's float64(float32(x)). cvtsd2ss uses the F2 prefix,
// cvtss2sd the F3 prefix; both escape 0F 5A.
func amd64SSEcvt_rr(cb *CodeBuffer, prefix, opcode, dst, src byte) {
	cb.EmitBytes(prefix)
	emitREX(cb, false, dst, src)
	cb.EmitBytes(0x0F, opcode, modRM(3, dst, src))
}

func amd64CVTSD2SS_rr(cb *CodeBuffer, dst, src byte) { amd64SSEcvt_rr(cb, 0xF2, 0x5A, dst, src) }
func amd64CVTSS2SD_rr(cb *CodeBuffer, dst, src byte) { amd64SSEcvt_rr(cb, 0xF3, 0x5A, dst, src) }

// amd64MOVQ_reg_xmm moves the full 64 bits of an xmm register into a GPR
// (66 REX.W 0F 7E /r). Needed to inspect a float64 result's bit pattern when
// computing condition codes.
func amd64MOVQ_reg_xmm(cb *CodeBuffer, gpr, xmm byte) {
	cb.EmitBytes(0x66)
	emitREX(cb, true, xmm, gpr)
	cb.EmitBytes(0x0F, 0x7E, modRM(3, xmm, gpr))
}

// amd64OR_reg_reg32 emits OR dst, src on 32-bit registers (09 /r).
func amd64OR_reg_reg32(cb *CodeBuffer, dst, src byte) {
	emitREX(cb, false, src, dst)
	cb.EmitBytes(0x09, modRM(3, src, dst))
}

// amd64MOVQ_xmm_reg moves a GPR's 64 bits into the low quadword of an xmm
// register (66 REX.W 0F 6E /r). Used to materialize a sign-bit mask constant.
func amd64MOVQ_xmm_reg(cb *CodeBuffer, xmm, gpr byte) {
	cb.EmitBytes(0x66)
	emitREX(cb, true, xmm, gpr)
	cb.EmitBytes(0x0F, 0x6E, modRM(3, xmm, gpr))
}

// amd64SSEpd_rr emits a packed-double SSE op (66 0F xx /r). Used for ANDPD/XORPD
// against a sign-bit mask to implement FABS/FNEG.
func amd64SSEpd_rr(cb *CodeBuffer, opcode, dst, src byte) {
	cb.EmitBytes(0x66)
	emitREX(cb, false, dst, src)
	cb.EmitBytes(0x0F, opcode, modRM(3, dst, src))
}

func amd64ANDPD_rr(cb *CodeBuffer, dst, src byte) { amd64SSEpd_rr(cb, 0x54, dst, src) }
func amd64XORPD_rr(cb *CodeBuffer, dst, src byte) { amd64SSEpd_rr(cb, 0x57, dst, src) }

// amd64UCOMISD_rm emits UCOMISD xmm, [base+disp] (66 0F 2E /r): an unordered
// scalar-double compare that sets ZF/PF/CF (PF on unordered/NaN). Used by FCMP.
func amd64UCOMISD_rm(cb *CodeBuffer, reg, base byte, disp int32) {
	cb.EmitBytes(0x66)
	emitREX(cb, false, reg, base)
	cb.EmitBytes(0x0F, 0x2E)
	emitModRMDisp(cb, reg, base, disp)
}

// fpuWorkXMM is the scalar-double work register for inline native FPU ops, and
// fpuBaseGPR holds the &fp[0] pointer. Both are outside the M68K integer
// register map (RDI/RSI/R8/R9/R14/R15), so they are free to clobber within a
// block without disturbing guest register state.
const (
	fpuWorkXMM = 0        // xmm0 — result work register
	fpuMaskXMM = 1        // xmm1 — sign-bit mask for FABS/FNEG
	fpuBaseGPR = amd64RAX // scratch pointer to the FP register file
)

// m68kEmitNativeFPURegToReg emits an inline SSE2 implementation of a 68881
// register-to-register arithmetic op: fp[dst] = fp[dst] OP fp[src] for the
// binary ops, fp[dst] = fp[src] for FMOVE, fp[dst] = sqrt(fp[src]) for FSQRT.
// Single-precision opmodes round the result through float32. The register file
// is float64, so these instructions are bit-identical to the interpreter's
// float64 arithmetic. Returns false for ops not yet natively emittable (the
// caller must fall back to the FPU helper); condition-code emission is layered
// on separately (slice 5b). Caller must have verified cpu.FPU != nil.
func m68kEmitNativeFPURegToReg(cb *CodeBuffer, op m68kFPUNativeOp, src, dst, precision int) bool {
	srcDisp := int32(src * 8)
	dstDisp := int32(dst * 8)

	// fpuBaseGPR = &fp[0]
	amd64MOV_reg_mem(cb, fpuBaseGPR, m68kAMD64RegCtx, int32(m68kCtxOffFPRegsPtr))

	switch op {
	case m68kFPUNativeFMOVE:
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, srcDisp)
	case m68kFPUNativeFSQRT:
		amd64SQRTSD_rm(cb, fpuWorkXMM, fpuBaseGPR, srcDisp)
	case m68kFPUNativeFABS:
		// fp[dst] = |fp[src]|: clear the sign bit via ANDPD with 0x7FFF...FFFF.
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, srcDisp)
		amd64MOV_reg_imm64(cb, amd64RCX, 0x7FFFFFFFFFFFFFFF)
		amd64MOVQ_xmm_reg(cb, fpuMaskXMM, amd64RCX)
		amd64ANDPD_rr(cb, fpuWorkXMM, fpuMaskXMM)
	case m68kFPUNativeFNEG:
		// fp[dst] = -fp[src]: flip the sign bit via XORPD with 0x8000...0000.
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, srcDisp)
		amd64MOV_reg_imm64(cb, amd64RCX, 0x8000000000000000)
		amd64MOVQ_xmm_reg(cb, fpuMaskXMM, amd64RCX)
		amd64XORPD_rr(cb, fpuWorkXMM, fpuMaskXMM)
	case m68kFPUNativeFADD:
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64ADDSD_rm(cb, fpuWorkXMM, fpuBaseGPR, srcDisp)
	case m68kFPUNativeFSUB:
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64SUBSD_rm(cb, fpuWorkXMM, fpuBaseGPR, srcDisp)
	case m68kFPUNativeFMUL:
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64MULSD_rm(cb, fpuWorkXMM, fpuBaseGPR, srcDisp)
	case m68kFPUNativeFDIV:
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64DIVSD_rm(cb, fpuWorkXMM, fpuBaseGPR, srcDisp)
	case m68kFPUNativeFSGLDIV:
		// Single-precision divide: round BOTH operands to float32 first, divide
		// in single, widen back. Matches float64(float32(dst)/float32(src)).
		amd64SSEsd_rm(cb, 0x5A, fpuWorkXMM, fpuBaseGPR, dstDisp) // cvtsd2ss xmm0,[dst]
		amd64SSEsd_rm(cb, 0x5A, fpuMaskXMM, fpuBaseGPR, srcDisp) // cvtsd2ss xmm1,[src]
		amd64SSE_scalar(cb, 0x5E, fpuWorkXMM, fpuMaskXMM)        // divss xmm0,xmm1
		amd64CVTSS2SD_rr(cb, fpuWorkXMM, fpuWorkXMM)             // → double
	case m68kFPUNativeFSGLMUL:
		amd64SSEsd_rm(cb, 0x5A, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64SSEsd_rm(cb, 0x5A, fpuMaskXMM, fpuBaseGPR, srcDisp)
		amd64SSE_scalar(cb, 0x59, fpuWorkXMM, fpuMaskXMM) // mulss xmm0,xmm1
		amd64CVTSS2SD_rr(cb, fpuWorkXMM, fpuWorkXMM)
	default:
		// FCMP/FTST are handled by the no-store path; anything else falls back.
		return false
	}

	if precision == m68kFPURoundSingle {
		amd64CVTSD2SS_rr(cb, fpuWorkXMM, fpuWorkXMM)
		amd64CVTSS2SD_rr(cb, fpuWorkXMM, fpuWorkXMM)
	}

	amd64MOVSD_store(cb, fpuBaseGPR, dstDisp, fpuWorkXMM)
	return true
}

// m68kFPUNativeOpEmittable reports whether the native FPU emitter implements op.
// Covers all SSE2-cleanly-mappable register-to-register ops; transcendentals,
// FINT/FINTRZ (need SSE4.1 roundsd / rounding-mode state), FMOD/FREM,
// FGETEXP/FGETMAN/FSCALE and EA-operand forms remain on the FPU helper.
func m68kFPUNativeOpEmittable(op m68kFPUNativeOp) bool {
	switch op {
	case m68kFPUNativeFMOVE, m68kFPUNativeFADD, m68kFPUNativeFSUB,
		m68kFPUNativeFMUL, m68kFPUNativeFDIV, m68kFPUNativeFSQRT,
		m68kFPUNativeFABS, m68kFPUNativeFNEG, m68kFPUNativeFSGLDIV,
		m68kFPUNativeFSGLMUL, m68kFPUNativeFCMP, m68kFPUNativeFTST:
		return true
	default:
		return false
	}
}

// m68kEmitNativeFPUSetCC emits an inline branch tree that derives the FPSR
// condition-code bits (N/Z/I/NAN) from the float64 result in fpuWorkXMM and
// merges them into FPSR, exactly mirroring (*M68881FPU).setCC64 /
// m68kFPUConditionBits. Clobbers RAX/RCX/RDX/R10/R11 (all scratch in the M68K
// register map). Validated end-to-end by the FPU JIT parity tests.
func m68kEmitNativeFPUSetCC(cb *CodeBuffer) {
	const (
		rResult = amd64RAX
		rFPSR   = amd64RCX
		rCC     = amd64RDX
		rTmp    = amd64R10
		rTmp2   = amd64R11
		clearCC = int32(-251658241) // ^fpuCCMask (0xF0FFFFFF as int32)
	)
	var doneJumps []int

	amd64MOVQ_reg_xmm(cb, rResult, fpuWorkXMM) // rax = result bits
	amd64XOR_reg_reg32(cb, rCC, rCC)           // cc = 0

	// rTmp = exponent field = (bits >> 52) & 0x7FF
	amd64MOV_reg_reg(cb, rTmp, rResult)
	amd64SHR_imm(cb, rTmp, 52)
	amd64ALU_reg_imm32_32bit(cb, 4, rTmp, 0x7FF) // and r10d, 0x7FF
	amd64ALU_reg_imm32_32bit(cb, 7, rTmp, 0x7FF) // cmp r10d, 0x7FF
	jneNotSpecial := amd64Jcc_rel32(cb, amd64CondNE)

	// --- exponent all ones: infinity or NaN ---
	amd64MOV_reg_reg(cb, rTmp2, rResult)
	amd64SHL_imm(cb, rTmp2, 12) // frac<<12 (zero iff fraction zero)
	amd64TEST_reg_reg(cb, rTmp2, rTmp2)
	jeInf := amd64Jcc_rel32(cb, amd64CondE)
	amd64MOV_reg_imm32(cb, rCC, uint32(FPU_CC_NAN))
	doneJumps = append(doneJumps, amd64JMP_rel32(cb))
	patchRel32(cb, jeInf, cb.Len())
	// infinity: N if sign set
	amd64MOV_reg_reg(cb, rTmp, rResult)
	amd64SHR_imm(cb, rTmp, 63)
	amd64TEST_reg_reg(cb, rTmp, rTmp)
	jeInfPos := amd64Jcc_rel32(cb, amd64CondE)
	amd64MOV_reg_imm32(cb, rCC, uint32(FPU_CC_I|FPU_CC_N))
	doneJumps = append(doneJumps, amd64JMP_rel32(cb))
	patchRel32(cb, jeInfPos, cb.Len())
	amd64MOV_reg_imm32(cb, rCC, uint32(FPU_CC_I))
	doneJumps = append(doneJumps, amd64JMP_rel32(cb))

	// --- finite: zero, negative, or positive ---
	patchRel32(cb, jneNotSpecial, cb.Len())
	amd64MOV_reg_reg(cb, rTmp2, rResult)
	amd64SHL_imm(cb, rTmp2, 1) // drops sign; ZF set iff value is +/-0
	jneNotZero := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_imm32(cb, rCC, uint32(FPU_CC_Z))
	doneJumps = append(doneJumps, amd64JMP_rel32(cb))
	patchRel32(cb, jneNotZero, cb.Len())
	amd64MOV_reg_reg(cb, rTmp, rResult)
	amd64SHR_imm(cb, rTmp, 63)
	amd64TEST_reg_reg(cb, rTmp, rTmp)
	jePositive := amd64Jcc_rel32(cb, amd64CondE) // positive → cc stays 0
	amd64MOV_reg_imm32(cb, rCC, uint32(FPU_CC_N))
	patchRel32(cb, jePositive, cb.Len())

	// merge point
	for _, j := range doneJumps {
		patchRel32(cb, j, cb.Len())
	}
	amd64MOV_reg_mem(cb, rFPSR, m68kAMD64RegCtx, int32(m68kCtxOffFPSRPtr)) // rcx = &FPSR
	amd64MOV_reg_mem32(cb, rTmp, rFPSR, 0)                                 // r10d = FPSR
	amd64ALU_reg_imm32_32bit(cb, 4, rTmp, clearCC)                         // clear cc field
	amd64OR_reg_reg32(cb, rTmp, rCC)                                       // | new cc
	amd64MOV_mem_reg32(cb, rFPSR, 0, rTmp)                                 // FPSR = r10d
}

// m68kEmitNativeFPUInstr emits a complete native register-to-register FPU
// instruction: a runtime FPU-presence guard (bail to the helper, which raises
// Line-F, when cpu.FPU is absent), the SSE2 arithmetic, and the condition-code
// update. Returns false if op has no native implementation, leaving the caller
// to emit the helper path.
func m68kEmitNativeFPUInstr(cb *CodeBuffer, op m68kFPUNativeOp, src, dst, precision int, instrPC uint32, br *m68kBlockRegs, instrIdx int) bool {
	if !m68kFPUNativeOpEmittable(op) {
		return false
	}
	// The compile loop materializes any lazily-live integer CCR into R14 before
	// this instruction (m68kInstrNeedsCCRMaterialization returns true for group
	// 0xF), so the host EFLAGS are free to clobber here: a following Bcc reads
	// the materialized CCR from R14, not the flags the code below leaves behind.

	// FPU-presence guard. The FP register pointer is zero when cpu.FPU == nil;
	// in that case bail to the helper (which raises Line-F) instead of
	// dereferencing a null pointer.
	amd64MOV_reg_mem(cb, fpuBaseGPR, m68kAMD64RegCtx, int32(m68kCtxOffFPRegsPtr))
	amd64TEST_reg_reg(cb, fpuBaseGPR, fpuBaseGPR)
	jnzNative := amd64Jcc_rel32(cb, amd64CondNE)
	m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperFPU)
	patchRel32(cb, jnzNative, cb.Len())

	// FPIAR = instruction address (data operations only; all native ops have
	// cmdWord bit 15 == 0). Mirrors ExecFPUInstruction.
	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffFPIARPtr))
	amd64MOV_mem_imm32(cb, amd64RCX, 0, instrPC)

	switch op {
	case m68kFPUNativeFCMP:
		m68kEmitNativeFCMP(cb, src, dst) // compare-only, custom CC, no store
	case m68kFPUNativeFTST:
		// FTST sets CC from the source operand; no result is stored.
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, int32(src*8))
		m68kEmitNativeFPUSetCC(cb)
	default:
		m68kEmitNativeFPURegToReg(cb, op, src, dst, precision)
		m68kEmitNativeFPUSetCC(cb)
	}
	return true
}

// m68kEmitNativeFCMP emits FCMP FPsrc,FPdst: compare fp[dst] against fp[src] and
// set the FPSR condition codes — NAN if either operand is NaN, Z if equal, N if
// dst < src; nothing is stored. This mirrors (*M68881FPU).FCMP, which differs
// from setCC64: no infinity bit, and NaN is keyed off the operands (via the
// unordered compare) rather than a difference value.
func m68kEmitNativeFCMP(cb *CodeBuffer, src, dst int) {
	const (
		rFPSR      = amd64RCX
		rCC        = amd64RDX
		rTmp       = amd64R10
		clearCC    = int32(-251658241) // ^fpuCCMask
		condParity = 0xA               // JP — unordered (either operand NaN)
	)
	// Zero cc BEFORE the compare: XOR sets host flags (result 0 → PF=1), which
	// would corrupt the ucomisd result the branches below test.
	amd64XOR_reg_reg32(cb, rCC, rCC)
	amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, int32(dst*8))
	amd64UCOMISD_rm(cb, fpuWorkXMM, fpuBaseGPR, int32(src*8)) // sets ZF/PF/CF

	jp := amd64Jcc_rel32(cb, condParity) // unordered → NaN
	je := amd64Jcc_rel32(cb, amd64CondE) // equal → Z
	jb := amd64Jcc_rel32(cb, amd64CondB) // dst < src (CF, already not unordered) → N
	var done []int
	done = append(done, amd64JMP_rel32(cb)) // dst > src → cc stays 0

	patchRel32(cb, jp, cb.Len())
	amd64MOV_reg_imm32(cb, rCC, uint32(FPU_CC_NAN))
	done = append(done, amd64JMP_rel32(cb))
	patchRel32(cb, je, cb.Len())
	amd64MOV_reg_imm32(cb, rCC, uint32(FPU_CC_Z))
	done = append(done, amd64JMP_rel32(cb))
	patchRel32(cb, jb, cb.Len())
	amd64MOV_reg_imm32(cb, rCC, uint32(FPU_CC_N))

	for _, j := range done {
		patchRel32(cb, j, cb.Len())
	}
	amd64MOV_reg_mem(cb, rFPSR, m68kAMD64RegCtx, int32(m68kCtxOffFPSRPtr))
	amd64MOV_reg_mem32(cb, rTmp, rFPSR, 0)
	amd64ALU_reg_imm32_32bit(cb, 4, rTmp, clearCC)
	amd64OR_reg_reg32(cb, rTmp, rCC)
	amd64MOV_mem_reg32(cb, rFPSR, 0, rTmp)
}
