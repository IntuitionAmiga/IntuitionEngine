//go:build amd64 && (linux || windows || darwin)

package main

import (
	"math"

	"golang.org/x/sys/cpu"
)

// m68kJITHasSSE41 reports whether the host supports SSE4.1 (needed for ROUNDSD,
// used by native FINT/FINTRZ). When false those ops stay on the FPU helper.
var m68kJITHasSSE41 = cpu.X86.HasSSE41

// amd64ROUNDSD_rr emits ROUNDSD xmm_dst, xmm_src, imm8 (66 0F 3A 0B /r ib), an
// SSE4.1 scalar-double round with an explicit rounding mode in imm8:
// bits[1:0] 00=nearest-even 01=floor 10=ceil 11=truncate; bit3 suppresses the
// precision (inexact) exception. Callers must have checked m68kJITHasSSE41.
func amd64ROUNDSD_rr(cb *CodeBuffer, dst, src, imm byte) {
	cb.EmitBytes(0x66)
	emitREX(cb, false, dst, src)
	cb.EmitBytes(0x0F, 0x3A, 0x0B, modRM(3, dst, src), imm)
}

// roundsd imm8 values (precision-exception suppressed) per rounding mode.
const (
	amd64RoundNearest  = 0x08 // round to nearest even
	amd64RoundFloor    = 0x09 // toward -inf
	amd64RoundCeil     = 0x0A // toward +inf
	amd64RoundTruncate = 0x0B // toward zero
)

// m68kEmitFPURoundToInt rounds fp src into dst as an integer-valued double.
// FINTRZ always truncates toward zero. FINT reads the live FPCR rounding mode
// (bits 5:4) and dispatches to the matching ROUNDSD — matching the
// interpreter's FINT, which honours FPCR while the emulator's arithmetic stays
// round-to-nearest, so MXCSR is deliberately left untouched. Uses RCX/R11 as
// scratch (NOT RAX, which the callers hold the &fp[0] base pointer in).
func m68kEmitFPURoundToInt(cb *CodeBuffer, dst, src byte, fintrz bool) {
	if fintrz {
		amd64ROUNDSD_rr(cb, dst, src, amd64RoundTruncate)
		return
	}
	// R11D = (FPCR >> 4) & 3
	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffFPCRPtr))
	amd64MOV_reg_mem32(cb, amd64R11, amd64RCX, 0)
	amd64SHR_imm32(cb, amd64R11, 4)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 3) // AND R11D, 3

	amd64ALU_reg_imm32_32bit(cb, 7, amd64R11, 1) // CMP R11D, 1 (FPU_RND_ZERO)
	jeZero := amd64Jcc_rel32(cb, amd64CondE)
	amd64ALU_reg_imm32_32bit(cb, 7, amd64R11, 2) // CMP R11D, 2 (FPU_RND_MINUS_INF)
	jeMinf := amd64Jcc_rel32(cb, amd64CondE)
	amd64ALU_reg_imm32_32bit(cb, 7, amd64R11, 3) // CMP R11D, 3 (FPU_RND_PLUS_INF)
	jePinf := amd64Jcc_rel32(cb, amd64CondE)

	// EAX == 0: round to nearest even.
	amd64ROUNDSD_rr(cb, dst, src, amd64RoundNearest)
	var done []int
	done = append(done, amd64JMP_rel32(cb))
	patchRel32(cb, jeZero, cb.Len())
	amd64ROUNDSD_rr(cb, dst, src, amd64RoundTruncate)
	done = append(done, amd64JMP_rel32(cb))
	patchRel32(cb, jeMinf, cb.Len())
	amd64ROUNDSD_rr(cb, dst, src, amd64RoundFloor)
	done = append(done, amd64JMP_rel32(cb))
	patchRel32(cb, jePinf, cb.Len())
	amd64ROUNDSD_rr(cb, dst, src, amd64RoundCeil)
	for _, off := range done {
		patchRel32(cb, off, cb.Len())
	}
}

// Native emission for 68881 EA-operand instructions: <ea> OP FPn → FPn and
// FMOVE FPn → <ea>. Before this path existed every FPU instruction with a
// memory, data-register, or immediate operand left the native block through
// the FPU helper — a full epilogue, a Go roundtrip, and a block re-entry per
// instruction. The forms below stay native; anything the decoder rejects
// (extended/packed formats, index/absolute/PC-relative EAs, FMOVECR,
// precision-qualified FCMP/FTST) still uses the helper.
//
// Semantics contract (mirrors execFPUEAToReg / execFPURegToMem):
//   - Address-register updates for (An)+/-(An) commit only after every guard
//     has passed: a guard bail re-executes the whole instruction in the
//     helper, which must observe the original An.
//   - Integer conversions use CVTTSD2SI (32-bit), which is exactly what Go
//     compiles int32(float64) to on amd64 — including the 0x80000000
//     "integer indefinite" result for NaN/overflow.
//   - Stores do not touch the FPSR condition codes (interpreter parity).

// fpuValXMM holds the converted EA operand as a float64. Distinct from
// fpuWorkXMM (the result register) so binary ops can use both.
const fpuValXMM = 1 // xmm1

// ---------------------------------------------------------------------------
// Encoders
// ---------------------------------------------------------------------------

// amd64CVTSI2SD_reg32 emits CVTSI2SD xmm, r32 (F2 0F 2A /r): signed 32-bit
// integer to scalar double.
func amd64CVTSI2SD_reg32(cb *CodeBuffer, xmm, gpr byte) {
	cb.EmitBytes(0xF2)
	emitREX(cb, false, xmm, gpr)
	cb.EmitBytes(0x0F, 0x2A, modRM(3, xmm, gpr))
}

// amd64CVTTSD2SI_reg32 emits CVTTSD2SI r32, xmm (F2 0F 2C /r): scalar double
// to signed 32-bit integer with truncation; NaN/overflow yields 0x80000000,
// matching Go's float64→int32 conversion on amd64.
func amd64CVTTSD2SI_reg32(cb *CodeBuffer, gpr, xmm byte) {
	cb.EmitBytes(0xF2)
	emitREX(cb, false, gpr, xmm)
	cb.EmitBytes(0x0F, 0x2C, modRM(3, gpr, xmm))
}

// amd64UCOMISD_rr emits UCOMISD xmm, xmm (66 0F 2E /r).
func amd64UCOMISD_rr(cb *CodeBuffer, dst, src byte) {
	cb.EmitBytes(0x66)
	emitREX(cb, false, dst, src)
	cb.EmitBytes(0x0F, 0x2E, modRM(3, dst, src))
}

// amd64MOVD_xmm_reg32 emits MOVD xmm, r32 (66 0F 6E /r, no REX.W).
func amd64MOVD_xmm_reg32(cb *CodeBuffer, xmm, gpr byte) {
	cb.EmitBytes(0x66)
	emitREX(cb, false, xmm, gpr)
	cb.EmitBytes(0x0F, 0x6E, modRM(3, xmm, gpr))
}

// amd64MOVD_reg32_xmm emits MOVD r32, xmm (66 0F 7E /r, no REX.W).
func amd64MOVD_reg32_xmm(cb *CodeBuffer, gpr, xmm byte) {
	cb.EmitBytes(0x66)
	emitREX(cb, false, xmm, gpr)
	cb.EmitBytes(0x0F, 0x7E, modRM(3, xmm, gpr))
}

// m68kEmitLoadDirectRAM64 loads 8 big-endian guest bytes at [memBase+addrReg]
// into dstReg as a host-order uint64 (MOV + BSWAP64). Bounds/I-O guards are
// the caller's responsibility.
func m68kEmitLoadDirectRAM64(cb *CodeBuffer, addrReg, dstReg byte) {
	emitMemOpSIB(cb, true, 0x8B, dstReg, m68kAMD64RegMemBase, addrReg, 0)
	emitREX(cb, true, 0, dstReg)
	cb.EmitBytes(0x0F, 0xC8+regBits(dstReg))
}

// m68kEmitStoreDirectRAM64 stores valReg as 8 big-endian guest bytes at
// [memBase+addrReg]. Clobbers R11; valReg is preserved.
func m68kEmitStoreDirectRAM64(cb *CodeBuffer, addrReg, valReg byte) {
	amd64MOV_reg_reg(cb, amd64R11, valReg)
	emitREX(cb, true, 0, amd64R11)
	cb.EmitBytes(0x0F, 0xC8+regBits(amd64R11))
	emitMemOpSIB(cb, true, 0x89, amd64R11, m68kAMD64RegMemBase, addrReg, 0)
}

// ---------------------------------------------------------------------------
// Compile-time immediate extraction
// ---------------------------------------------------------------------------

// m68kFPUImmediateValue decodes a #<data> FPU operand from the instruction
// stream at extPC into its float64 value. Returns false when the format is
// unsupported or the bytes run past the scanned memory image.
func m68kFPUImmediateValue(memory []byte, extPC uint32, format int) (float64, bool) {
	be16 := func(off uint32) (uint16, bool) {
		if int(off)+2 > len(memory) {
			return 0, false
		}
		return uint16(memory[off])<<8 | uint16(memory[off+1]), true
	}
	be32 := func(off uint32) (uint32, bool) {
		if int(off)+4 > len(memory) {
			return 0, false
		}
		return uint32(memory[off])<<24 | uint32(memory[off+1])<<16 |
			uint32(memory[off+2])<<8 | uint32(memory[off+3]), true
	}
	switch format {
	case 0: // long integer
		v, ok := be32(extPC)
		return float64(int32(v)), ok
	case 1: // single precision
		v, ok := be32(extPC)
		return float64(math.Float32frombits(v)), ok
	case 4: // word integer
		v, ok := be16(extPC)
		return float64(int16(v)), ok
	case 6: // byte integer (low byte of one immediate word)
		v, ok := be16(extPC)
		return float64(int8(v)), ok
	case 5: // double precision
		hi, ok1 := be32(extPC)
		lo, ok2 := be32(extPC + 4)
		return math.Float64frombits(uint64(hi)<<32 | uint64(lo)), ok1 && ok2
	case 2: // extended precision (96-bit): convert exactly at compile time
		w0, ok0 := be32(extPC)
		w1, ok1 := be32(extPC + 4)
		w2, ok2 := be32(extPC + 8)
		if !ok0 || !ok1 || !ok2 {
			return 0, false
		}
		ext := ExtendedReal{
			Sign: uint8(w0 >> 31),
			Exp:  uint16(w0>>16) & 0x7FFF,
			Mant: (uint64(w1) << 32) | uint64(w2),
		}
		return ext.ToFloat64(), true
	default:
		return 0, false
	}
}

// ---------------------------------------------------------------------------
// Emission
// ---------------------------------------------------------------------------

// m68kEmitFPUEAAddr computes the operand address into R10 WITHOUT committing
// any address-register update. The caller commits (An)+/-(An) side effects
// after all guards have passed.
func m68kEmitFPUEAAddr(cb *CodeBuffer, form *m68kNativeFPUEAForm, memory []byte, instrPC uint32) {
	base := m68kResolveAddrReg(cb, uint16(form.reg), amd64R10)
	if base != amd64R10 {
		amd64MOV_reg_reg32(cb, amd64R10, base)
	}
	switch form.mode {
	case 4: // -(An): effective address is An - step
		amd64ALU_reg_imm32_32bit(cb, 5, amd64R10, int32(m68kFPUEAStepBytes(form.format, form.reg)))
	case 5: // d16(An)
		disp := int16(uint16(memory[instrPC+4])<<8 | uint16(memory[instrPC+5]))
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(disp))
	}
}

// m68kEmitFPUEACommitAn commits the address-register side effect after all
// guards passed: (An)+ steps forward; -(An) adopts the effective address
// still live in R10.
func m68kEmitFPUEACommitAn(cb *CodeBuffer, form *m68kNativeFPUEAForm) {
	switch form.mode {
	case 3: // (An)+
		step := int32(m68kFPUEAStepBytes(form.format, form.reg))
		if r, mapped := m68kAddrRegToAMD64(uint16(form.reg)); mapped {
			amd64ALU_reg_imm32_32bit(cb, 0, r, step)
		} else {
			amd64MOV_reg_mem32(cb, amd64R11, m68kAMD64RegDataBase, m68kAddrRegFileDisp(uint16(form.reg)))
			amd64ALU_reg_imm32_32bit(cb, 0, amd64R11, step)
			amd64MOV_mem_reg32(cb, m68kAMD64RegDataBase, m68kAddrRegFileDisp(uint16(form.reg)), amd64R11)
		}
	case 4: // -(An)
		m68kStoreAddrReg(cb, uint16(form.reg), amd64R10)
	}
}

// m68kEmitFPUEALoadConvert reads the operand at [memBase+R10] and leaves it in
// fpuValXMM as a float64. Guards must already have passed. Clobbers RAX.
func m68kEmitFPUEALoadConvert(cb *CodeBuffer, format int) {
	switch format {
	case 0: // long integer
		m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_LONG)
		amd64CVTSI2SD_reg32(cb, fpuValXMM, amd64RAX)
	case 1: // single precision
		m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_LONG)
		amd64MOVD_xmm_reg32(cb, fpuValXMM, amd64RAX)
		amd64CVTSS2SD_rr(cb, fpuValXMM, fpuValXMM)
	case 4: // word integer
		m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_WORD)
		amd64MOVSX_W(cb, amd64RAX, amd64RAX)
		amd64CVTSI2SD_reg32(cb, fpuValXMM, amd64RAX)
	case 6: // byte integer
		m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_BYTE)
		amd64MOVSX_B(cb, amd64RAX, amd64RAX)
		amd64CVTSI2SD_reg32(cb, fpuValXMM, amd64RAX)
	case 5: // double precision
		m68kEmitLoadDirectRAM64(cb, amd64R10, amd64RAX)
		amd64MOVQ_xmm_reg(cb, fpuValXMM, amd64RAX)
	}
}

// m68kEmitExtendedEALoadConvert reads a 96-bit extended-real operand at
// [memBase+R10] and converts it into fpuValXMM as a float64, exactly mirroring
// readExtendedReal96 + ExtendedReal.ToFloat64 for finite normalised values.
// Zero, denormal, infinity and NaN inputs (extended exponent 0 or 0x7FFF, or a
// double exponent that would under/overflow) append a bail site to bailOffs so
// the FPU helper handles them with the full conversion. Assumes the 12-byte
// access at R10 has already been guarded. Clobbers RAX/RCX/RDX/R11; R10 (the
// operand address) is preserved so the caller can still commit an (An)+/-(An)
// side effect afterwards.
func m68kEmitExtendedEALoadConvert(cb *CodeBuffer, bailOffs *[]int) {
	// RAX = big-endian 16-bit sign+exponent at [R10]; R11 keeps a copy for sign.
	m68kEmitLoadDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_WORD)
	amd64MOV_reg_reg32(cb, amd64R11, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x7FFF)             // AND EAX, 0x7FFF → exponent (sets ZF)
	*bailOffs = append(*bailOffs, amd64Jcc_rel32(cb, amd64CondE)) // exp==0: zero/denormal
	amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, 0x7FFF)             // CMP exp, 0x7FFF
	*bailOffs = append(*bailOffs, amd64Jcc_rel32(cb, amd64CondE)) // exp==0x7FFF: inf/nan
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RAX, 15360)              // f64Exp = exp - 15360
	amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, 1)                  // CMP f64Exp, 1
	*bailOffs = append(*bailOffs, amd64Jcc_rel32(cb, amd64CondL)) // f64Exp <= 0: underflow
	amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, 0x7FE)              // CMP f64Exp, 0x7FE
	*bailOffs = append(*bailOffs, amd64Jcc_rel32(cb, amd64CondG)) // f64Exp >= 0x7FF: overflow

	// RCX = big-endian 64-bit mantissa at [R10+4] (via RDX so R10 is preserved).
	amd64MOV_reg_reg32(cb, amd64RDX, amd64R10)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 4)
	m68kEmitLoadDirectRAM64(cb, amd64RDX, amd64RCX)
	// f64Mant = (mant >> 11) & 0x000FFFFFFFFFFFFF (drop the explicit integer bit).
	amd64SHR_imm(cb, amd64RCX, 11)
	amd64SHL_imm(cb, amd64RCX, 12)
	amd64SHR_imm(cb, amd64RCX, 12)
	// bits = (sign<<63) | (f64Exp<<52) | f64Mant.
	amd64SHL_imm(cb, amd64RAX, 52)
	amd64ALU_reg_reg(cb, 0x09, amd64RCX, amd64RAX) // OR RCX, RAX
	amd64SHR_imm(cb, amd64R11, 15)                 // sign bit
	amd64SHL_imm(cb, amd64R11, 63)
	amd64ALU_reg_reg(cb, 0x09, amd64RCX, amd64R11) // OR RCX, R11
	amd64MOVQ_xmm_reg(cb, fpuValXMM, amd64RCX)
}

// m68kEmitExtendedEAStoreConvert writes the float64 in srcXMM as a 96-bit
// extended-real value at [memBase+R10], exactly mirroring
// ExtendedRealFromFloat64 + writeExtendedReal96 for finite normalised values.
// Zero, denormal, infinity and NaN (double exponent 0 or 0x7FF) append a bail
// site so the FPU helper performs the full conversion. Assumes the 12-byte
// access at R10 has been guarded; R10 (the destination address) is preserved.
// Clobbers RAX/RCX/RDX/R11.
func m68kEmitExtendedEAStoreConvert(cb *CodeBuffer, srcXMM byte, bailOffs *[]int) {
	amd64MOVQ_reg_xmm(cb, amd64RCX, srcXMM) // RCX = float64 bits
	amd64MOV_reg_reg(cb, amd64RAX, amd64RCX)
	amd64SHR_imm(cb, amd64RAX, 52)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, 0x7FF)              // AND EAX, 0x7FF → f64Exp (sets ZF)
	*bailOffs = append(*bailOffs, amd64Jcc_rel32(cb, amd64CondE)) // f64Exp==0: zero/denormal
	amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, 0x7FF)              // CMP f64Exp, 0x7FF
	*bailOffs = append(*bailOffs, amd64Jcc_rel32(cb, amd64CondE)) // f64Exp==0x7FF: inf/nan
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, 15360)              // extExp = f64Exp + 15360

	// sign = bits >> 63 (extract before RCX is overwritten with the mantissa).
	amd64MOV_reg_reg(cb, amd64RDX, amd64RCX)
	amd64SHR_imm(cb, amd64RDX, 63)
	// f64Mant = bits & 0x000FFFFFFFFFFFFF, then set the explicit integer bit and
	// shift into the extended mantissa position (integer bit at 63).
	amd64SHL_imm(cb, amd64RCX, 12)
	amd64SHR_imm(cb, amd64RCX, 12)
	amd64MOV_reg_imm64(cb, amd64R11, 1<<52)
	amd64ALU_reg_reg(cb, 0x09, amd64RCX, amd64R11) // OR RCX, 1<<52
	amd64SHL_imm(cb, amd64RCX, 11)                 // RCX = extMant
	// signExp = (sign << 15) | extExp.
	amd64SHL_imm(cb, amd64RDX, 15)
	amd64ALU_reg_reg(cb, 0x09, amd64RAX, amd64RDX) // RAX = signExp

	// Write signExp (BE16) at ea, a zero pad word at ea+2, extMant (BE64) at ea+4.
	m68kEmitStoreDirectRAM(cb, amd64R10, amd64RAX, M68K_SIZE_WORD)
	amd64MOV_reg_reg32(cb, amd64RDX, amd64R10)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 2)
	amd64XOR_reg_reg32(cb, amd64RAX, amd64RAX)
	m68kEmitStoreDirectRAM(cb, amd64RDX, amd64RAX, M68K_SIZE_WORD)
	amd64MOV_reg_reg32(cb, amd64RDX, amd64R10)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 4)
	m68kEmitStoreDirectRAM64(cb, amd64RDX, amd64RCX)
}

// m68kEmitFPUDnConvert converts a data-register operand (mode 0) into
// fpuValXMM as a float64. Clobbers RAX/R10.
func m68kEmitFPUDnConvert(cb *CodeBuffer, reg uint16, format int) {
	r := m68kResolveDataReg(cb, reg, amd64R10)
	switch format {
	case 0: // long integer
		amd64CVTSI2SD_reg32(cb, fpuValXMM, r)
	case 1: // single precision
		amd64MOVD_xmm_reg32(cb, fpuValXMM, r)
		amd64CVTSS2SD_rr(cb, fpuValXMM, fpuValXMM)
	case 4: // word integer (low 16 bits)
		amd64MOVSX_W(cb, amd64RAX, r)
		amd64CVTSI2SD_reg32(cb, fpuValXMM, amd64RAX)
	case 6: // byte integer (low 8 bits)
		amd64MOVSX_B(cb, amd64RAX, r)
		amd64CVTSI2SD_reg32(cb, fpuValXMM, amd64RAX)
	}
}

// m68kEmitNativeFPUEAApply applies op to the operand in fpuValXMM, storing the
// result to fp[dst] and updating the FPSR condition codes exactly like the
// interpreter's applyFPUEAValue. Expects RAX free; reloads the FP base itself.
func m68kEmitNativeFPUEAApply(cb *CodeBuffer, op m68kFPUNativeOp, dst, precision int, elideCC bool) {
	dstDisp := int32(dst * 8)
	amd64MOV_reg_mem(cb, fpuBaseGPR, m68kAMD64RegCtx, int32(m68kCtxOffFPRegsPtr))
	switch op {
	case m68kFPUNativeFMOVE:
		amd64MOVSD_rr(cb, fpuWorkXMM, fpuValXMM)
	case m68kFPUNativeFSQRT:
		amd64SQRTSD_rr(cb, fpuWorkXMM, fpuValXMM)
	case m68kFPUNativeFABS:
		amd64MOVSD_load(cb, fpuWorkXMM, m68kAMD64RegCtx, int32(m68kCtxOffFPAbsMask))
		amd64ANDPD_rr(cb, fpuWorkXMM, fpuValXMM)
	case m68kFPUNativeFNEG:
		amd64MOVSD_load(cb, fpuWorkXMM, m68kAMD64RegCtx, int32(m68kCtxOffFPNegMask))
		amd64XORPD_rr(cb, fpuWorkXMM, fpuValXMM)
	case m68kFPUNativeFADD:
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64ADDSD_rr(cb, fpuWorkXMM, fpuValXMM)
	case m68kFPUNativeFSUB:
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64SUBSD_rr(cb, fpuWorkXMM, fpuValXMM)
	case m68kFPUNativeFMUL:
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64MULSD_rr(cb, fpuWorkXMM, fpuValXMM)
	case m68kFPUNativeFDIV:
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64DIVSD_rr(cb, fpuWorkXMM, fpuValXMM)
	case m68kFPUNativeFSGLDIV:
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64CVTSD2SS_rr(cb, fpuWorkXMM, fpuWorkXMM)
		amd64CVTSD2SS_rr(cb, fpuValXMM, fpuValXMM)
		amd64SSE_scalar(cb, 0x5E, fpuWorkXMM, fpuValXMM) // divss
		amd64CVTSS2SD_rr(cb, fpuWorkXMM, fpuWorkXMM)
	case m68kFPUNativeFSGLMUL:
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64CVTSD2SS_rr(cb, fpuWorkXMM, fpuWorkXMM)
		amd64CVTSD2SS_rr(cb, fpuValXMM, fpuValXMM)
		amd64SSE_scalar(cb, 0x59, fpuWorkXMM, fpuValXMM) // mulss
		amd64CVTSS2SD_rr(cb, fpuWorkXMM, fpuWorkXMM)
	case m68kFPUNativeFINT:
		m68kEmitFPURoundToInt(cb, fpuWorkXMM, fpuValXMM, false)
	case m68kFPUNativeFINTRZ:
		m68kEmitFPURoundToInt(cb, fpuWorkXMM, fpuValXMM, true)
	case m68kFPUNativeFTST:
		// CC from the operand; no result store, fp[dst] untouched.
		amd64MOVSD_rr(cb, fpuWorkXMM, fpuValXMM)
		m68kEmitNativeFPUSetCC(cb)
		return
	case m68kFPUNativeFCMP:
		// fp[dst] compared against the operand; CC only, no store.
		amd64XOR_reg_reg32(cb, amd64RDX, amd64RDX)
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, dstDisp)
		amd64UCOMISD_rr(cb, fpuWorkXMM, fpuValXMM)
		m68kEmitNativeFCMPFlagsTail(cb)
		return
	}
	if precision == m68kFPURoundSingle {
		amd64CVTSD2SS_rr(cb, fpuWorkXMM, fpuWorkXMM)
		amd64CVTSS2SD_rr(cb, fpuWorkXMM, fpuWorkXMM)
	}
	amd64MOVSD_store(cb, fpuBaseGPR, dstDisp, fpuWorkXMM)
	// Lazy FPSR: skip the CC update when the next in-block op overwrites the CC
	// before any observer or fault (FTST/FCMP above always set their CC).
	if !elideCC {
		m68kEmitNativeFPUSetCC(cb)
	}
}

// m68kEmitNativeFPUEA emits a complete native EA-operand FPU instruction:
// presence guard, FPIAR update, operand fetch/convert with guarded memory
// access, the operation, and (for stores) the SMC invalidation check. Guard
// bails re-execute the instruction through the FPU helper. Returns false —
// with nothing emitted — when the form is not natively supported.
func m68kEmitNativeFPUEA(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, blockStartPC uint32, br *m68kBlockRegs, instrIdx int, elideFPIAR, elideCC bool) bool {
	opcode := ji.opcode
	instrPC := blockStartPC + ji.pcOffset
	if memory == nil || int(instrPC)+4 > len(memory) {
		return false
	}
	cmdWord := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	form, ok := m68kDecodeNativeFPUEA(opcode, cmdWord)
	if !ok {
		return false
	}

	// Validate everything that could fail BEFORE emitting a single byte.
	var immVal float64
	switch {
	case form.mode == 5:
		if int(instrPC)+6 > len(memory) {
			return false
		}
	case form.mode == 7 && form.reg == 4:
		immVal, ok = m68kFPUImmediateValue(memory, instrPC+4, form.format)
		if !ok {
			return false
		}
	case m68kFPUEAModeIsComputed(form.mode, form.reg):
		// Index modes must be brief-format (the general path emits full-format
		// index via a helper bail); all computed modes' extension words must be
		// in bounds. The EA extension follows the command word, at instrPC+4.
		extPC := instrPC + 4
		if m68kModeIsIndexed(uint16(form.mode), uint16(form.reg)) &&
			!m68kBriefIndexedEAAllowed(memory, extPC, uint16(form.mode), uint16(form.reg)) {
			return false
		}
		extBytes := m68kEAExtBytes(uint16(form.mode), uint16(form.reg), M68K_SIZE_LONG, memory, extPC)
		if int(extPC)+extBytes > len(memory) {
			return false
		}
	}

	// FPU-presence guard: the FP register pointer is nil-zero when cpu.FPU is
	// absent; the helper raises Line-F in that case.
	amd64MOV_reg_mem(cb, fpuBaseGPR, m68kAMD64RegCtx, int32(m68kCtxOffFPRegsPtr))
	amd64TEST_reg_reg(cb, fpuBaseGPR, fpuBaseGPR)
	jnzNative := amd64Jcc_rel32(cb, amd64CondNE)
	m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperFPU)
	patchRel32(cb, jnzNative, cb.Len())

	// FPIAR = instruction address (data operation, cmdWord bit 15 == 0).
	m68kEmitFPIARStore(cb, instrPC, elideFPIAR)

	if form.store {
		m68kEmitNativeFPUEAStore(cb, ji, memory, instrPC, br, instrIdx, &form)
	} else {
		m68kEmitNativeFPUEALoad(cb, ji, memory, instrPC, br, instrIdx, &form, immVal, elideCC)
	}
	return true
}

func m68kEmitNativeFPUEALoad(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, instrPC uint32, br *m68kBlockRegs, instrIdx int, form *m68kNativeFPUEAForm, immVal float64, elideCC bool) {
	var bailOffs []int
	switch {
	case form.mode == 0: // Dn direct
		m68kEmitFPUDnConvert(cb, uint16(form.reg), form.format)
	case form.mode == 7 && form.reg == 4: // immediate — baked in at compile time
		amd64MOV_reg_imm64(cb, amd64R10, math.Float64bits(immVal))
		amd64MOVQ_xmm_reg(cb, fpuValXMM, amd64R10)
	case m68kFPUEAModeIsComputed(form.mode, form.reg):
		// Index / absolute / PC-relative: general address computation into R10,
		// no address-register side effect.
		m68kEmitComputeEAAddr(cb, uint16(form.mode), uint16(form.reg), memory, instrPC+4, instrPC, amd64R10)
		amd64MOV_reg_imm32(cb, amd64RDX, m68kFPUEAFormatAccessBytes(form.format))
		m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
		if form.format == 2 {
			m68kEmitExtendedEALoadConvert(cb, &bailOffs)
		} else {
			m68kEmitFPUEALoadConvert(cb, form.format)
		}
	default: // (An), (An)+, -(An), d16(An)
		m68kEmitFPUEAAddr(cb, form, memory, instrPC)
		amd64MOV_reg_imm32(cb, amd64RDX, m68kFPUEAFormatAccessBytes(form.format))
		m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
		if form.format == 2 {
			m68kEmitExtendedEALoadConvert(cb, &bailOffs)
		} else {
			m68kEmitFPUEALoadConvert(cb, form.format)
		}
		m68kEmitFPUEACommitAn(cb, form)
	}
	m68kEmitNativeFPUEAApply(cb, form.op, form.fpReg, form.precision, elideCC)
	if len(bailOffs) > 0 {
		m68kPatchHelperBails(cb, bailOffs, instrPC, br, instrIdx, m68kJITHelperFPU)
	}
}

func m68kEmitNativeFPUEAStore(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, instrPC uint32, br *m68kBlockRegs, instrIdx int, form *m68kNativeFPUEAForm) {
	srcDisp := int32(form.fpReg * 8)

	if form.mode == 0 {
		// Dn destination: convert and merge with move-to-Dn semantics
		// (byte/word writes preserve the upper bits). No memory access, no
		// FPSR change.
		amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, srcDisp)
		switch form.format {
		case 1: // single precision bits
			amd64CVTSD2SS_rr(cb, fpuWorkXMM, fpuWorkXMM)
			amd64MOVD_reg32_xmm(cb, amd64RDX, fpuWorkXMM)
			m68kStoreDataReg(cb, uint16(form.reg), amd64RDX)
		case 0: // long integer
			amd64CVTTSD2SI_reg32(cb, amd64RDX, fpuWorkXMM)
			m68kStoreDataReg(cb, uint16(form.reg), amd64RDX)
		case 4: // word integer, low 16 bits merged
			amd64CVTTSD2SI_reg32(cb, amd64RDX, fpuWorkXMM)
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0xFFFF)
			r := m68kResolveDataReg(cb, uint16(form.reg), amd64R10)
			amd64ALU_reg_imm32_32bit(cb, 4, r, int32(-65536)) // AND 0xFFFF0000
			amd64OR_reg_reg32(cb, r, amd64RDX)
			m68kStoreDataReg(cb, uint16(form.reg), r)
		case 6: // byte integer, low 8 bits merged
			amd64CVTTSD2SI_reg32(cb, amd64RDX, fpuWorkXMM)
			amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 0xFF)
			r := m68kResolveDataReg(cb, uint16(form.reg), amd64R10)
			amd64ALU_reg_imm32_32bit(cb, 4, r, int32(-256)) // AND 0xFFFFFF00
			amd64OR_reg_reg32(cb, r, amd64RDX)
			m68kStoreDataReg(cb, uint16(form.reg), r)
		}
		return
	}

	// Memory destination: EA + guards first (An uncommitted), then convert
	// and store, then commit An and run the SMC invalidation check.
	var bailOffs []int
	computed := m68kFPUEAModeIsComputed(form.mode, form.reg)
	if computed {
		// Index / absolute destination (PC-relative and immediate stores are
		// rejected at decode time); no address-register side effect.
		m68kEmitComputeEAAddr(cb, uint16(form.mode), uint16(form.reg), memory, instrPC+4, instrPC, amd64R10)
	} else {
		m68kEmitFPUEAAddr(cb, form, memory, instrPC)
	}
	accessBytes := m68kFPUEAFormatAccessBytes(form.format)
	amd64MOV_reg_imm32(cb, amd64RDX, accessBytes)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)

	amd64MOV_reg_mem(cb, fpuBaseGPR, m68kAMD64RegCtx, int32(m68kCtxOffFPRegsPtr))
	amd64MOVSD_load(cb, fpuWorkXMM, fpuBaseGPR, srcDisp)
	switch form.format {
	case 1: // single precision
		amd64CVTSD2SS_rr(cb, fpuWorkXMM, fpuWorkXMM)
		amd64MOVD_reg32_xmm(cb, amd64RDX, fpuWorkXMM)
		m68kEmitStoreDirectRAM(cb, amd64R10, amd64RDX, M68K_SIZE_LONG)
	case 0: // long integer
		amd64CVTTSD2SI_reg32(cb, amd64RDX, fpuWorkXMM)
		m68kEmitStoreDirectRAM(cb, amd64R10, amd64RDX, M68K_SIZE_LONG)
	case 4: // word integer
		amd64CVTTSD2SI_reg32(cb, amd64RDX, fpuWorkXMM)
		m68kEmitStoreDirectRAM(cb, amd64R10, amd64RDX, M68K_SIZE_WORD)
	case 6: // byte integer
		amd64CVTTSD2SI_reg32(cb, amd64RDX, fpuWorkXMM)
		m68kEmitStoreDirectRAM(cb, amd64R10, amd64RDX, M68K_SIZE_BYTE)
	case 5: // double precision
		amd64MOVQ_reg_xmm(cb, amd64RDX, fpuWorkXMM)
		m68kEmitStoreDirectRAM64(cb, amd64R10, amd64RDX)
	case 2: // extended precision (96-bit)
		m68kEmitExtendedEAStoreConvert(cb, fpuWorkXMM, &bailOffs)
	}
	if !computed {
		m68kEmitFPUEACommitAn(cb, form)
	}
	m68kEmitSMCInvalidateByteRangeCheck(cb, amd64R10, accessBytes)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchHelperBails(cb, bailOffs, instrPC, br, instrIdx, m68kJITHelperFPU)
}

// ---------------------------------------------------------------------------
// FMOVECR — native FPU ROM constant load
// ---------------------------------------------------------------------------

// m68kEmitNativeFMOVECR emits FMOVECR #rom,FPn: the ROM value is a
// compile-time constant (fmovecrROMTable), so this is an immediate store plus
// the condition-code update. Returns false when the encoding is not FMOVECR.
func m68kEmitNativeFMOVECR(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, blockStartPC uint32, br *m68kBlockRegs, instrIdx int, elideFPIAR bool) bool {
	opcode := ji.opcode
	instrPC := blockStartPC + ji.pcOffset
	if (opcode>>6)&0x7 != 0 || memory == nil || int(instrPC)+4 > len(memory) {
		return false
	}
	cmdWord := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
	if cmdWord&0xFC00 != 0x5C00 {
		return false
	}
	dst := int((cmdWord >> 7) & 0x7)
	value := fmovecrROMTable[cmdWord&0x3F]

	// FPU-presence guard + FPIAR update (FMOVECR is a data operation).
	amd64MOV_reg_mem(cb, fpuBaseGPR, m68kAMD64RegCtx, int32(m68kCtxOffFPRegsPtr))
	amd64TEST_reg_reg(cb, fpuBaseGPR, fpuBaseGPR)
	jnzNative := amd64Jcc_rel32(cb, amd64CondNE)
	m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperFPU)
	patchRel32(cb, jnzNative, cb.Len())
	m68kEmitFPIARStore(cb, instrPC, elideFPIAR)

	amd64MOV_reg_imm64(cb, amd64R10, math.Float64bits(value))
	amd64MOVQ_xmm_reg(cb, fpuWorkXMM, amd64R10)
	amd64MOVSD_store(cb, fpuBaseGPR, int32(dst*8), fpuWorkXMM)
	m68kEmitNativeFPUSetCC(cb)
	return true
}

// ---------------------------------------------------------------------------
// FScc Dn — native FPU condition-to-boolean
// ---------------------------------------------------------------------------

// m68kEmitFSccBoolR11 leaves the FScc boolean (0xFF true / 0x00 false) in R11.
// The FPSR pointer must already be in RCX; the condition byte is read from the
// FPU condition table via m68kEmitFBccSkipJumps. Clobbers RAX and host EFLAGS.
func m68kEmitFSccBoolR11(cb *CodeBuffer, cond uint16) {
	switch cond & 0xF {
	case 0x0: // F: always 0
		amd64XOR_reg_reg32(cb, amd64R11, amd64R11)
	case 0xF: // T: always 0xFF
		amd64MOV_reg_imm32(cb, amd64R11, 0xFF)
	default:
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RCX, 0) // EAX = FPSR
		skipOffs, takenOffs := m68kEmitFBccSkipJumps(cb, cond)
		for _, off := range takenOffs {
			patchRel32(cb, off, cb.Len())
		}
		amd64MOV_reg_imm32(cb, amd64R11, 0xFF)
		joinOff := amd64JMP_rel32(cb)
		for _, off := range skipOffs {
			patchRel32(cb, off, cb.Len())
		}
		amd64XOR_reg_reg32(cb, amd64R11, amd64R11)
		patchRel32(cb, joinOff, cb.Len())
	}
}

// m68kEmitNativeFScc emits FScc <ea> (typeField 1): set an 8-bit boolean from
// the FPU condition. Data-register-direct (mode 0), (An) (mode 2) and d16(An)
// (mode 5) are native; (An)+/-(An) are left to the helper because the
// interpreter's FScc uses GetEffectiveAddress, which does NOT adjust An for
// those modes — a quirk this fast path deliberately does not replicate — and
// index/absolute/PC-relative EAs, FDBcc, and FTRAPcc also stay on the helper.
// FScc does not update FPIAR.
func m68kEmitNativeFSccDn(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, blockStartPC uint32, br *m68kBlockRegs, instrIdx int) bool {
	opcode := ji.opcode
	instrPC := blockStartPC + ji.pcOffset
	if (opcode>>6)&0x7 != 1 {
		return false
	}
	mode := (opcode >> 3) & 0x7
	switch mode {
	case 0, 2, 5:
	default:
		return false
	}
	if memory == nil || int(instrPC)+4 > len(memory) {
		return false
	}
	if mode == 5 && int(instrPC)+6 > len(memory) {
		return false
	}
	cond := (uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])) & 0x3F
	reg := opcode & 0x7

	// Condition tests clobber EFLAGS; materialize any lazy integer CCR.
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	// FPU-presence guard (no FPIAR update for conditionals).
	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffFPSRPtr))
	amd64TEST_reg_reg(cb, amd64RCX, amd64RCX)
	jnzNative := amd64Jcc_rel32(cb, amd64CondNE)
	m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperFPU)
	patchRel32(cb, jnzNative, cb.Len())

	if mode == 0 {
		m68kEmitFSccBoolR11(cb, cond)
		r := m68kResolveDataReg(cb, reg, amd64R10)
		amd64ALU_reg_imm32_32bit(cb, 4, r, int32(-256)) // AND 0xFFFFFF00
		amd64OR_reg_reg32(cb, r, amd64R11)
		m68kStoreDataReg(cb, reg, r)
		return true
	}

	// Memory byte destination (An)/d16(An). Compute EA into R10, then guard.
	base := m68kResolveAddrReg(cb, reg, amd64R10)
	if base != amd64R10 {
		amd64MOV_reg_reg32(cb, amd64R10, base)
	}
	if mode == 5 {
		disp := int16(uint16(memory[instrPC+4])<<8 | uint16(memory[instrPC+5]))
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, int32(disp))
	}
	var bailOffs []int
	amd64MOV_reg_imm32(cb, amd64RDX, 1)
	m68kEmitMemRangeBailChecks(cb, amd64R10, amd64RDX, &bailOffs)
	// The bail checks clobbered RCX; reload the FPSR pointer (presence already
	// verified non-null above).
	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffFPSRPtr))
	m68kEmitFSccBoolR11(cb, cond)
	m68kEmitStoreDirectRAM(cb, amd64R10, amd64R11, M68K_SIZE_BYTE)
	m68kEmitSMCInvalidateRangeCheck(cb, amd64R10, M68K_SIZE_BYTE)
	m68kEmitExitIfInvalidated(cb, instrPC+uint32(ji.length), uint32(instrIdx+1), br)
	m68kPatchHelperBails(cb, bailOffs, instrPC, br, instrIdx, m68kJITHelperFPU)
	return true
}

// ---------------------------------------------------------------------------
// FBcc — native FPU conditional branch
// ---------------------------------------------------------------------------

// m68kEmitFBccSkipJumps emits condition tests on the FPSR value in EAX.
// Control falls through when the condition is TRUE; skipOffs are Jcc rel32
// placeholders taken when the condition is FALSE (the caller patches them to
// the not-taken label). takenOffs shortcut directly to the fall-through
// (taken) label for mixed-polarity conditions. Mirrors evalFPUCondition:
// I (infinity) never participates; cond is evaluated modulo 16.
func m68kEmitFBccSkipJumps(cb *CodeBuffer, cond uint16) (skipOffs, takenOffs []int) {
	const (
		bitN   = FPU_CC_N
		bitZ   = FPU_CC_Z
		bitNAN = FPU_CC_NAN
	)
	test := func(mask uint32) {
		amd64TEST_reg_imm32(cb, amd64RAX, mask)
	}
	skip := func(cc byte) { skipOffs = append(skipOffs, amd64Jcc_rel32(cb, cc)) }
	taken := func(cc byte) { takenOffs = append(takenOffs, amd64Jcc_rel32(cb, cc)) }

	switch cond & 0xF {
	case 0x1: // EQ: z
		test(bitZ)
		skip(amd64CondE)
	case 0x2: // OGT: !nan && !z && !n
		test(bitNAN | bitZ | bitN)
		skip(amd64CondNE)
	case 0x3: // OGE: !nan && (z || !n)
		test(bitNAN)
		skip(amd64CondNE)
		test(bitZ)
		taken(amd64CondNE)
		test(bitN)
		skip(amd64CondNE)
	case 0x4: // OLT: !nan && !z && n
		test(bitNAN | bitZ)
		skip(amd64CondNE)
		test(bitN)
		skip(amd64CondE)
	case 0x5: // OLE: !nan && (z || n)
		test(bitNAN)
		skip(amd64CondNE)
		test(bitZ | bitN)
		skip(amd64CondE)
	case 0x6: // OGL: !nan && !z
		test(bitNAN | bitZ)
		skip(amd64CondNE)
	case 0x7: // OR: !nan
		test(bitNAN)
		skip(amd64CondNE)
	case 0x8: // UN: nan
		test(bitNAN)
		skip(amd64CondE)
	case 0x9: // UEQ: nan || z
		test(bitNAN | bitZ)
		skip(amd64CondE)
	case 0xA: // UGT: nan || (!z && !n)
		test(bitNAN)
		taken(amd64CondNE)
		test(bitZ | bitN)
		skip(amd64CondNE)
	case 0xB: // UGE: nan || z || !n
		test(bitNAN | bitZ)
		taken(amd64CondNE)
		test(bitN)
		skip(amd64CondNE)
	case 0xC: // ULT: nan || (!z && n)
		test(bitNAN)
		taken(amd64CondNE)
		test(bitZ)
		skip(amd64CondNE)
		test(bitN)
		skip(amd64CondE)
	case 0xD: // ULE: nan || z || n
		test(bitNAN | bitZ | bitN)
		skip(amd64CondE)
	case 0xE: // NE: !z
		test(bitZ)
		skip(amd64CondNE)
	case 0xF: // T: always taken — no tests
	}
	return skipOffs, takenOffs
}

// m68kEmitFBcc emits a native FBcc (typeField 2/3): evaluate the FPU condition
// from the FPSR and branch via the block-chain machinery — an in-block
// backward jump with loop budget when the target is inside the block, a
// patchable chain exit otherwise. Before this, every FBcc left the block
// through the FPU helper. FBF (condition F, never taken) compiles to nothing
// but the presence guard. Returns false — with nothing emitted — for forms it
// cannot handle (the caller falls back to the helper).
func m68kEmitFBcc(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, instrOffsets []int, instrs []M68KJITInstr, chainSlots *[]m68kChainExitInfo) bool {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if memory == nil {
		return false
	}
	var disp int32
	switch (opcode >> 6) & 0x7 {
	case 2: // word displacement
		if int(instrPC)+4 > len(memory) {
			return false
		}
		disp = int32(int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])))
	case 3: // long displacement
		if int(instrPC)+6 > len(memory) {
			return false
		}
		disp = int32(uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
			uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5]))
	default:
		return false
	}
	targetPC := uint32(int64(instrPC) + 2 + int64(disp))
	cond := opcode & 0x3F

	// The tests below clobber EFLAGS on both paths; materialize any lazy
	// integer CCR first.
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	// FPU-presence guard: without an FPU the helper raises Line-F.
	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffFPSRPtr))
	amd64TEST_reg_reg(cb, amd64RCX, amd64RCX)
	jnzNative := amd64Jcc_rel32(cb, amd64CondNE)
	m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperFPU)
	patchRel32(cb, jnzNative, cb.Len())

	if cond&0xF == 0 {
		return true // FBF: never taken
	}

	amd64MOV_reg_mem32(cb, amd64RAX, amd64RCX, 0) // EAX = FPSR

	skipOffs, takenOffs := m68kEmitFBccSkipJumps(cb, cond)
	for _, off := range takenOffs {
		patchRel32(cb, off, cb.Len())
	}

	// Taken path. In-block backward branch with loop budget (mirrors
	// m68kEmitBcc's loop optimization) when the target is resolvable.
	if br.hasBackwardBranch && targetPC >= startPC && targetPC < instrPC &&
		instrOffsets != nil && instrs != nil {
		targetIdx := m68kFindInstrByPC(instrs, targetPC-startPC, instrIdx)
		if targetIdx >= 0 && targetIdx < len(instrOffsets) {
			bodySize := uint32(instrIdx - targetIdx + 1)
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), amd64RAX)

			amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffChainCount))
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffChainCount), amd64RAX)

			amd64ALU_mem_imm32(cb, 5, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget), int32(bodySize))
			budgetExitOff := amd64Jcc_rel32(cb, amd64CondLE)

			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, int32(m68kJitBudget))
			safetyExitOff := amd64Jcc_rel32(cb, amd64CondAE)

			backOff := amd64JMP_rel32(cb)
			patchRel32(cb, backOff, instrOffsets[targetIdx])

			// Budget exceeded: undo this iteration's accounting and exit via
			// the normal chain, which counts the linear prefix itself.
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

			for _, off := range skipOffs {
				patchRel32(cb, off, cb.Len())
			}
			return true
		}
	}

	info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, targetPC), br)
	if chainSlots != nil {
		*chainSlots = append(*chainSlots, info)
	}
	for _, off := range skipOffs {
		patchRel32(cb, off, cb.Len())
	}
	return true
}

// ---------------------------------------------------------------------------
// FDBcc — native FPU decrement-and-branch
// ---------------------------------------------------------------------------

// m68kEmitFDBcc emits FDBcc Dn,<disp> (typeField 1, EA mode 1): a decrement-
// and-branch loop keyed off the FPU condition. It mirrors execFPUConditional's
// FDBcc arm exactly — condition TRUE exits the loop (no decrement); condition
// FALSE decrements Dn.W and branches to instrPC+2+disp unless the counter
// wrapped from 0 to -1. FDBcc is the FPU analogue of DBcc but with the FPU
// condition polarity inverted (FPU cond 0 = F never-true → always loops like
// DBF; cond 0xF = T always-true → never loops like DBT). Returns false — with
// nothing emitted — for forms it does not handle (the caller falls back).
func m68kEmitFDBcc(cb *CodeBuffer, ji *M68KJITInstr, memory []byte, startPC uint32, br *m68kBlockRegs, instrIdx int, instrOffsets []int, instrs []M68KJITInstr, chainSlots *[]m68kChainExitInfo) bool {
	opcode := ji.opcode
	instrPC := startPC + ji.pcOffset
	if (opcode>>6)&0x7 != 1 || (opcode>>3)&0x7 != 1 {
		return false
	}
	if memory == nil || int(instrPC)+6 > len(memory) {
		return false
	}
	reg := opcode & 7
	cond := (uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])) & 0x3F
	dispWord := int16(uint16(memory[instrPC+4])<<8 | uint16(memory[instrPC+5]))
	targetPC := uint32(int64(instrPC) + 2 + int64(dispWord))

	// Condition tests and the decrement clobber EFLAGS; materialize any lazy
	// integer CCR into R14 first.
	if cs := m68kCurrentCS; cs != nil && cs.flagState != flagsMaterialized {
		m68kMaterializeCCR(cb, cs)
	}

	// FPU-presence guard (no FPIAR update for conditionals).
	amd64MOV_reg_mem(cb, amd64RCX, m68kAMD64RegCtx, int32(m68kCtxOffFPSRPtr))
	amd64TEST_reg_reg(cb, amd64RCX, amd64RCX)
	jnzNative := amd64Jcc_rel32(cb, amd64CondNE)
	m68kEmitHelperAtInstr(cb, instrPC, br, instrIdx, m68kJITHelperFPU)
	patchRel32(cb, jnzNative, cb.Len())

	// FDBT: condition always true → never loops, falls through.
	if cond&0xF == 0xF {
		return true
	}

	// Condition TRUE arms all converge on the loop-exit (fall-through) label.
	var exitJumps []int
	if cond&0xF != 0 { // not FDBF: evaluate the FPU condition
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RCX, 0) // EAX = FPSR
		skipOffs, takenOffs := m68kEmitFBccSkipJumps(cb, cond)
		// Fall-through here = condition TRUE → exit loop.
		exitJumps = append(exitJumps, amd64JMP_rel32(cb))
		exitJumps = append(exitJumps, takenOffs...)
		// Condition FALSE → decrement.
		for _, off := range skipOffs {
			patchRel32(cb, off, cb.Len())
		}
	}

	// Decrement Dn.W (low word only, upper word preserved). Identical to DBcc.
	r := m68kResolveDataReg(cb, reg, amd64RAX)
	amd64MOV_reg_reg32(cb, amd64RDX, r)
	amd64MOVZX_W(cb, amd64RCX, r)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, -65536)
	amd64MOVZX_W(cb, amd64RCX, amd64RCX)
	amd64ALU_reg_reg32(cb, 0x09, amd64RDX, amd64RCX)
	m68kStoreDataReg(cb, reg, amd64RDX)

	// Exhausted (Dn.W wrapped to 0xFFFF) → exit loop.
	amd64ALU_reg_imm32_32bit(cb, 7, amd64RCX, 0xFFFF)
	exhaustedOff := amd64Jcc_rel32(cb, amd64CondE)

	branched := false
	if br.hasBackwardBranch && targetPC >= startPC && targetPC < instrPC &&
		instrOffsets != nil && instrs != nil {
		targetIdx := m68kFindInstrByPC(instrs, targetPC-startPC, instrIdx)
		if targetIdx >= 0 && targetIdx < len(instrOffsets) {
			bodySize := uint32(instrIdx - targetIdx + 1)
			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, amd64RSP, int32(m68kAMD64OffLoopCount), amd64RAX)
			amd64MOV_reg_mem32(cb, amd64RAX, m68kAMD64RegCtx, int32(m68kCtxOffChainCount))
			amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(bodySize))
			amd64MOV_mem_reg32(cb, m68kAMD64RegCtx, int32(m68kCtxOffChainCount), amd64RAX)

			amd64ALU_mem_imm32(cb, 5, m68kAMD64RegCtx, int32(m68kCtxOffChainBudget), int32(bodySize))
			budgetExitOff := amd64Jcc_rel32(cb, amd64CondLE)

			amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(m68kAMD64OffLoopCount))
			amd64ALU_reg_imm32_32bit(cb, 7, amd64RAX, int32(m68kJitBudget))
			safetyExitOff := amd64Jcc_rel32(cb, amd64CondAE)

			backOff := amd64JMP_rel32(cb)
			patchRel32(cb, backOff, instrOffsets[targetIdx])

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
			branched = true
		}
	}
	if !branched {
		info := m68kEmitChainExit(cb, targetPC, uint32(instrIdx+1), m68kNativePrefixInstrCount(memory, targetPC), br)
		if chainSlots != nil {
			*chainSlots = append(*chainSlots, info)
		}
	}

	// Loop-exit convergence: exhausted counter and every condition-TRUE arm.
	patchRel32(cb, exhaustedOff, cb.Len())
	for _, off := range exitJumps {
		patchRel32(cb, off, cb.Len())
	}
	return true
}
