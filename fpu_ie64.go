package main

import (
	"math"
)

// =============================================================================
// IE64 FPU Constants
// =============================================================================

// Rounding modes (FPCR bits 1:0)
const (
	IE64_FPU_RND_NEAREST uint8 = 0 // Round to nearest (default)
	IE64_FPU_RND_ZERO    uint8 = 1 // Round toward zero (truncate)
	IE64_FPU_RND_FLOOR   uint8 = 2 // Round toward negative infinity
	IE64_FPU_RND_CEIL    uint8 = 3 // Round toward positive infinity
)

// FPSR condition code bits (bits 27:24)
const (
	IE64_FPU_CC_N   uint32 = 1 << 27 // Negative
	IE64_FPU_CC_Z   uint32 = 1 << 26 // Zero
	IE64_FPU_CC_I   uint32 = 1 << 25 // Infinity
	IE64_FPU_CC_NAN uint32 = 1 << 24 // Not a Number
)

// FPSR sticky exception flags (bits 3:0)
const (
	IE64_FPU_EX_IO uint32 = 1 << 0 // Invalid Operation
	IE64_FPU_EX_DZ uint32 = 1 << 1 // Divide by Zero
	IE64_FPU_EX_OE uint32 = 1 << 2 // Overflow
	IE64_FPU_EX_UE uint32 = 1 << 3 // Underflow
)

const IE64_FPU_FPSR_MASK uint32 = 0x0F00000F // CC bits (27:24) | Exception bits (3:0)

// Pre-computed constants for saturation
var fp32MaxInt32 = float32(math.MaxInt32)
var fp32MinInt32 = float32(math.MinInt32)

// =============================================================================
// IEEE-754 Bit Helpers
// =============================================================================

func isNaN32(bits uint32) bool  { return (bits&0x7F800000 == 0x7F800000) && (bits&0x007FFFFF != 0) }
func isInf32(bits uint32) bool  { return (bits & 0x7FFFFFFF) == 0x7F800000 }
func isZero32(bits uint32) bool { return (bits & 0x7FFFFFFF) == 0 }

// =============================================================================
// IE64FPU - FPU State and Registers
// =============================================================================

// IE64FPU represents the 64-bit RISC Floating Point Unit.
// It uses 32-bit (FP32) IEEE-754 registers.
type IE64FPU struct {
	FPRegs [16]uint32 // F0-F15 registers
	FPCR   uint32     // FPU Control Register
	FPSR   uint32     // FPU Status Register
}

// NewIE64FPU creates a new FPU with default state.
func NewIE64FPU() *IE64FPU {
	return &IE64FPU{
		FPCR: 0, // Default: RND_NEAREST
		FPSR: 0,
	}
}

// SetRoundingMode sets the rounding mode in FPCR (bits 1:0).
func (fpu *IE64FPU) SetRoundingMode(mode uint8) {
	fpu.FPCR = (fpu.FPCR & ^uint32(0x03)) | uint32(mode&0x03)
}

// GetRoundingMode returns the current rounding mode from FPCR.
func (fpu *IE64FPU) GetRoundingMode() uint8 {
	return uint8(fpu.FPCR & 0x03)
}

// setConditionCodes sets the condition codes based on a result.
// Condition codes (bits 27:24) are overwritten per instruction.
func (fpu *IE64FPU) setConditionCodes(val float32) {
	fpu.setConditionCodesBits(math.Float32bits(val))
}

func (fpu *IE64FPU) setConditionCodesBits(bits uint32) {
	cc := uint32(0)
	exp := bits & 0x7F800000
	frac := bits & 0x007FFFFF
	if exp == 0x7F800000 {
		if frac != 0 {
			cc = IE64_FPU_CC_NAN
		} else {
			cc = IE64_FPU_CC_I
			if bits>>31 != 0 {
				cc |= IE64_FPU_CC_N
			}
		}
	} else if exp|frac == 0 {
		cc = IE64_FPU_CC_Z
	} else if bits>>31 != 0 {
		cc = IE64_FPU_CC_N
	}
	// Clear CC bits (27:24) then set new ones. Preserve other bits.
	fpu.FPSR = (fpu.FPSR & ^uint32(0x0F000000)) | cc
}

// setExceptionFlag sets a sticky exception flag in FPSR.
func (fpu *IE64FPU) setExceptionFlag(flag uint32) {
	fpu.FPSR |= flag
}

// ------------------------------------------------------------------------------
// Register Access Helpers
// ------------------------------------------------------------------------------

func (fpu *IE64FPU) getFReg(idx byte) float32 {
	return math.Float32frombits(fpu.FPRegs[idx&0x0F])
}

func (fpu *IE64FPU) setFReg(idx byte, val float32) {
	fpu.FPRegs[idx&0x0F] = math.Float32bits(val)
}

// ------------------------------------------------------------------------------
// Arithmetic Operations
// ------------------------------------------------------------------------------

// FADD: fd = fs + ft
func (fpu *IE64FPU) FADD(fd, fs, ft byte) {
	sBits := fpu.FPRegs[fs&0x0F]
	tBits := fpu.FPRegs[ft&0x0F]
	res := math.Float32frombits(sBits) + math.Float32frombits(tBits)
	resBits := math.Float32bits(res)

	// Check for overflow/NaN creation
	if isInf32(resBits) && !isInf32(sBits) && !isInf32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_OE)
	}
	if isNaN32(resBits) && !isNaN32(sBits) && !isNaN32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_IO)
	}

	fpu.FPRegs[fd&0x0F] = resBits
	fpu.setConditionCodesBits(resBits)
}

// FSUB: fd = fs - ft
func (fpu *IE64FPU) FSUB(fd, fs, ft byte) {
	sBits := fpu.FPRegs[fs&0x0F]
	tBits := fpu.FPRegs[ft&0x0F]
	res := math.Float32frombits(sBits) - math.Float32frombits(tBits)
	resBits := math.Float32bits(res)

	if isInf32(resBits) && !isInf32(sBits) && !isInf32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_OE)
	}
	if isNaN32(resBits) && !isNaN32(sBits) && !isNaN32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_IO)
	}

	fpu.FPRegs[fd&0x0F] = resBits
	fpu.setConditionCodesBits(resBits)
}

// FMUL: fd = fs * ft
func (fpu *IE64FPU) FMUL(fd, fs, ft byte) {
	sBits := fpu.FPRegs[fs&0x0F]
	tBits := fpu.FPRegs[ft&0x0F]
	res := math.Float32frombits(sBits) * math.Float32frombits(tBits)
	resBits := math.Float32bits(res)

	if isInf32(resBits) && !isInf32(sBits) && !isInf32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_OE)
	}
	if isNaN32(resBits) && !isNaN32(sBits) && !isNaN32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_IO)
	}
	if isZero32(resBits) && !isZero32(sBits) && !isZero32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_UE)
	}

	fpu.FPRegs[fd&0x0F] = resBits
	fpu.setConditionCodesBits(resBits)
}

// FDIV: fd = fs / ft
func (fpu *IE64FPU) FDIV(fd, fs, ft byte) {
	sBits := fpu.FPRegs[fs&0x0F]
	tBits := fpu.FPRegs[ft&0x0F]

	if isZero32(tBits) && !isZero32(sBits) && !isNaN32(sBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_DZ)
	}

	res := math.Float32frombits(sBits) / math.Float32frombits(tBits)
	resBits := math.Float32bits(res)

	if isInf32(resBits) && !isInf32(sBits) && !isZero32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_OE)
	}
	if isNaN32(resBits) && !isNaN32(sBits) && !isNaN32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_IO)
	}
	if isZero32(resBits) && !isZero32(sBits) && !isZero32(tBits) && !isInf32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_UE)
	}

	fpu.FPRegs[fd&0x0F] = resBits
	fpu.setConditionCodesBits(resBits)
}

// FMOD: fd = fmod(fs, ft)
func (fpu *IE64FPU) FMOD(fd, fs, ft byte) {
	sBits := fpu.FPRegs[fs&0x0F]
	tBits := fpu.FPRegs[ft&0x0F]
	res := float32(math.Mod(float64(math.Float32frombits(sBits)), float64(math.Float32frombits(tBits))))
	resBits := math.Float32bits(res)

	if isNaN32(resBits) && !isNaN32(sBits) && !isNaN32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_IO)
	}

	fpu.FPRegs[fd&0x0F] = resBits
	fpu.setConditionCodesBits(resBits)
}

// ------------------------------------------------------------------------------
// Unary Operations
// ------------------------------------------------------------------------------

// FABS: fd = |fs|
func (fpu *IE64FPU) FABS(fd, fs byte) {
	bits := fpu.FPRegs[fs&0x0F] & 0x7FFFFFFF
	fpu.FPRegs[fd&0x0F] = bits
	fpu.setConditionCodesBits(bits)
}

// FNEG: fd = -fs
func (fpu *IE64FPU) FNEG(fd, fs byte) {
	bits := fpu.FPRegs[fs&0x0F] ^ 0x80000000
	fpu.FPRegs[fd&0x0F] = bits
	fpu.setConditionCodesBits(bits)
}

// FSQRT: fd = sqrt(fs)

func (fpu *IE64FPU) FSQRT(fd, fs byte) {

	sBits := fpu.FPRegs[fs&0x0F]

	// s < 0 check: sign bit (31) set, not -0.0, and not NaN

	if (sBits&0x80000000) != 0 && !isZero32(sBits) && !isNaN32(sBits) {

		fpu.setExceptionFlag(IE64_FPU_EX_IO)

	}

	res := float32(math.Sqrt(float64(math.Float32frombits(sBits))))

	resBits := math.Float32bits(res)

	fpu.FPRegs[fd&0x0F] = resBits
	fpu.setConditionCodesBits(resBits)
}

// FINT: fd = round(fs)
func (fpu *IE64FPU) FINT(fd, fs byte) {
	s := fpu.getFReg(fs)
	var res float32
	switch fpu.GetRoundingMode() {
	case IE64_FPU_RND_NEAREST:
		res = float32(math.RoundToEven(float64(s)))
	case IE64_FPU_RND_ZERO:
		res = float32(math.Trunc(float64(s)))
	case IE64_FPU_RND_FLOOR:
		res = float32(math.Floor(float64(s)))
	case IE64_FPU_RND_CEIL:
		res = float32(math.Ceil(float64(s)))
	}
	fpu.setFReg(fd, res)
	fpu.setConditionCodes(res)
}

// ------------------------------------------------------------------------------
// Transcendentals
// ------------------------------------------------------------------------------

func (fpu *IE64FPU) FSIN(fd, fs byte) {
	s := fpu.getFReg(fs)
	res := float32(math.Sin(float64(s)))
	fpu.setFReg(fd, res)
	fpu.setConditionCodes(res)
}

func (fpu *IE64FPU) FCOS(fd, fs byte) {
	s := fpu.getFReg(fs)
	res := float32(math.Cos(float64(s)))
	fpu.setFReg(fd, res)
	fpu.setConditionCodes(res)
}

func (fpu *IE64FPU) FTAN(fd, fs byte) {
	s := fpu.getFReg(fs)
	res := float32(math.Tan(float64(s)))
	fpu.setFReg(fd, res)
	fpu.setConditionCodes(res)
}

func (fpu *IE64FPU) FATAN(fd, fs byte) {
	s := fpu.getFReg(fs)
	res := float32(math.Atan(float64(s)))
	fpu.setFReg(fd, res)
	fpu.setConditionCodes(res)
}

func (fpu *IE64FPU) FLOG(fd, fs byte) {
	s := fpu.getFReg(fs)
	if s == 0 {
		fpu.setExceptionFlag(IE64_FPU_EX_DZ)
	} else if s < 0 {
		fpu.setExceptionFlag(IE64_FPU_EX_IO)
	}
	res := float32(math.Log(float64(s)))
	fpu.setFReg(fd, res)
	fpu.setConditionCodes(res)
}

func (fpu *IE64FPU) FEXP(fd, fs byte) {
	sBits := fpu.FPRegs[fs&0x0F]
	res := float32(math.Exp(float64(math.Float32frombits(sBits))))
	resBits := math.Float32bits(res)

	if isInf32(resBits) && !isInf32(sBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_OE)
	}
	fpu.FPRegs[fd&0x0F] = resBits
	fpu.setConditionCodesBits(resBits)
}

func (fpu *IE64FPU) FPOW(fd, fs, ft byte) {
	sBits := fpu.FPRegs[fs&0x0F]
	tBits := fpu.FPRegs[ft&0x0F]
	res := float32(math.Pow(float64(math.Float32frombits(sBits)), float64(math.Float32frombits(tBits))))
	resBits := math.Float32bits(res)

	// Simple OE/IO check for Pow
	if isInf32(resBits) && !isInf32(sBits) && !isInf32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_OE)
	}
	if isNaN32(resBits) && !isNaN32(sBits) && !isNaN32(tBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_IO)
	}
	fpu.FPRegs[fd&0x0F] = resBits
	fpu.setConditionCodesBits(resBits)
}

// ------------------------------------------------------------------------------
// Comparison and Conversion
// ------------------------------------------------------------------------------

// FCMP: integer rd = -1/0/1; FPSR NaN set if unordered
func (fpu *IE64FPU) FCMP(fs, ft byte) int32 {
	sBits := fpu.FPRegs[fs&0x0F]
	tBits := fpu.FPRegs[ft&0x0F]
	s := math.Float32frombits(sBits)
	t := math.Float32frombits(tBits)

	// Clear CC
	fpu.FPSR &= ^(IE64_FPU_CC_N | IE64_FPU_CC_Z | IE64_FPU_CC_I | IE64_FPU_CC_NAN)

	if isNaN32(sBits) || isNaN32(tBits) {
		fpu.FPSR |= IE64_FPU_CC_NAN
		fpu.setExceptionFlag(IE64_FPU_EX_IO)
		return 0
	}

	if s < t {
		fpu.FPSR |= IE64_FPU_CC_N
		return -1
	}
	if s > t {
		if isInf32(sBits) && (sBits>>31) == 0 { // +Inf
			fpu.FPSR |= IE64_FPU_CC_I
		}
		return 1
	}

	// Equal
	fpu.FPSR |= IE64_FPU_CC_Z
	if isInf32(sBits) {
		fpu.FPSR |= IE64_FPU_CC_I
		if (sBits >> 31) != 0 {
			fpu.FPSR |= IE64_FPU_CC_N
		}
	}
	return 0
}

// FCVTIF: int32(rs) -> float32(fd)
func (fpu *IE64FPU) FCVTIF(fd byte, rs uint64) {
	val := float32(int32(rs))
	resBits := math.Float32bits(val)
	fpu.FPRegs[fd&0x0F] = resBits
	fpu.setConditionCodesBits(resBits)
}

// FCVTFI: float32(fs) -> int32(rd); saturating
func (fpu *IE64FPU) FCVTFI(fs byte) int32 {
	sBits := fpu.FPRegs[fs&0x0F]
	s := math.Float32frombits(sBits)

	if isNaN32(sBits) {
		fpu.setExceptionFlag(IE64_FPU_EX_IO)
		return 0
	}

	if s > fp32MaxInt32 {
		fpu.setExceptionFlag(IE64_FPU_EX_IO)
		return math.MaxInt32
	}
	if s < fp32MinInt32 {
		fpu.setExceptionFlag(IE64_FPU_EX_IO)
		return math.MinInt32
	}

	return int32(s)
}

// FMOVI: bitwise int reg -> FP reg
func (fpu *IE64FPU) FMOVI(fd byte, rs uint64) {
	bits := uint32(rs)
	fpu.FPRegs[fd&0x0F] = bits
	fpu.setConditionCodesBits(bits)
}

// FMOVO: bitwise FP reg -> int reg
func (fpu *IE64FPU) FMOVO(fs byte) uint64 {
	return uint64(fpu.FPRegs[fs&0x0F])
}

// ------------------------------------------------------------------------------
// ROM and Status
// ------------------------------------------------------------------------------

func (fpu *IE64FPU) FMOVECR(fd, idx byte) {
	if idx > 15 {
		fpu.FPRegs[fd&0x0F] = 0 // 0.0 bits
		fpu.setConditionCodesBits(0)
		return
	}
	bits := ie64FmovecrROMTable[idx]
	fpu.FPRegs[fd&0x0F] = bits
	fpu.setConditionCodesBits(bits)
}

func (fpu *IE64FPU) FMOVSR() uint32 {
	return fpu.FPSR
}

func (fpu *IE64FPU) FMOVCR() uint32 {
	return fpu.FPCR
}

func (fpu *IE64FPU) FMOVSC(val uint32) {
	fpu.FPSR = val & IE64_FPU_FPSR_MASK
}

func (fpu *IE64FPU) FMOVCC(val uint32) {
	fpu.FPCR = val
}

// =============================================================================
// FMOVECR ROM Table
// =============================================================================

// ie64FmovecrROMTable contains FP32 bit patterns for FMOVECR instruction.
var ie64FmovecrROMTable = [16]uint32{
	math.Float32bits(float32(math.Pi)),            // 0: Pi
	math.Float32bits(float32(math.E)),             // 1: e
	math.Float32bits(float32(math.Log2E)),         // 2: log2(e)
	math.Float32bits(float32(math.Log10E)),        // 3: log10(e)
	math.Float32bits(float32(math.Ln2)),           // 4: ln(2)
	math.Float32bits(float32(math.Ln10)),          // 5: ln(10)
	math.Float32bits(float32(math.Log10(2))),      // 6: log10(2)
	math.Float32bits(0.0),                         // 7: 0.0
	math.Float32bits(1.0),                         // 8: 1.0
	math.Float32bits(2.0),                         // 9: 2.0
	math.Float32bits(10.0),                        // 10: 10.0
	math.Float32bits(100.0),                       // 11: 100.0
	math.Float32bits(1000.0),                      // 12: 1000.0
	math.Float32bits(0.5),                         // 13: 0.5
	math.Float32bits(math.SmallestNonzeroFloat32), // 14: FLT_MIN (denormal)
	math.Float32bits(math.MaxFloat32),             // 15: FLT_MAX
}
