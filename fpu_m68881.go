package main

import (
	"math"
)

// =============================================================================
// FPU Constants
// =============================================================================

// Rounding modes (FPCR bits 5-4)
const (
	FPU_RND_NEAREST   uint8 = 0 // Round to nearest (default)
	FPU_RND_ZERO      uint8 = 1 // Round toward zero (truncate)
	FPU_RND_MINUS_INF uint8 = 2 // Round toward negative infinity
	FPU_RND_PLUS_INF  uint8 = 3 // Round toward positive infinity
)

// Precision modes (FPCR bits 7-6)
const (
	FPU_PREC_EXTENDED uint8 = 0 // Extended precision (80-bit)
	FPU_PREC_SINGLE   uint8 = 1 // Single precision (32-bit)
	FPU_PREC_DOUBLE   uint8 = 2 // Double precision (64-bit)
)

// FPSR condition code bits (bits 27-24)
const (
	FPU_CC_N   uint32 = 1 << 27 // Negative
	FPU_CC_Z   uint32 = 1 << 26 // Zero
	FPU_CC_I   uint32 = 1 << 25 // Infinity
	FPU_CC_NAN uint32 = 1 << 24 // Not a Number
)

const fpuCCMask = FPU_CC_N | FPU_CC_Z | FPU_CC_I | FPU_CC_NAN

// Extended precision constants
const (
	extExpBias uint16 = 16383   // Exponent bias for 80-bit
	extExpMax  uint16 = 0x7FFF  // Maximum exponent (infinity/NaN)
	extMantMSB uint64 = 1 << 63 // Explicit integer bit
)

// fmovecrROMTable contains pre-computed FPU ROM constants for FMOVECR instruction.
// Indexed by ROM address (0x00-0x3F). Unknown addresses return zero.
var fmovecrROMTable = func() [64]float64 {
	var table [64]float64
	table[0x00] = math.Pi       // Pi
	table[0x0B] = math.Log10(2) // log10(2)
	table[0x0C] = math.E        // e
	table[0x0D] = math.Log2E    // log2(e)
	table[0x0E] = math.Log10E   // log10(e)
	table[0x0F] = 0.0           // 0.0
	table[0x30] = math.Ln2      // ln(2)
	table[0x31] = math.Ln10     // ln(10)
	table[0x32] = 1.0           // 10^0
	table[0x33] = 10.0          // 10^1
	table[0x34] = 100.0         // 10^2
	table[0x35] = 1000.0        // 10^3
	table[0x36] = 10000.0       // 10^4
	table[0x37] = 100000.0      // 10^5
	table[0x38] = 1000000.0     // 10^6
	table[0x39] = 10000000.0    // 10^7
	table[0x3A] = 100000000.0   // 10^8
	return table
}()

// =============================================================================
// ExtendedReal - 80-bit Extended Precision Floating Point
// =============================================================================

// ExtendedReal represents an 80-bit extended precision floating point number
// as used by the 68881/68882 FPU.
//
// Format: 1 sign bit + 15 exponent bits + 64 mantissa bits (with explicit integer bit)
type ExtendedReal struct {
	Sign uint8  // 0 = positive, 1 = negative
	Exp  uint16 // 15-bit biased exponent
	Mant uint64 // 64-bit mantissa with explicit integer bit
}

// NewExtendedReal creates a new ExtendedReal from components
func NewExtendedReal(sign uint8, exp uint16, mant uint64) ExtendedReal {
	return ExtendedReal{Sign: sign, Exp: exp, Mant: mant}
}

// ExtendedRealFromFloat64 converts a float64 to ExtendedReal
func ExtendedRealFromFloat64(f float64) ExtendedReal {
	if math.IsNaN(f) {
		return ExtendedReal{Sign: 0, Exp: extExpMax, Mant: 0xC000000000000000}
	}

	if math.IsInf(f, 1) {
		return ExtendedReal{Sign: 0, Exp: extExpMax, Mant: extMantMSB}
	}

	if math.IsInf(f, -1) {
		return ExtendedReal{Sign: 1, Exp: extExpMax, Mant: extMantMSB}
	}

	if f == 0 {
		sign := uint8(0)
		if math.Signbit(f) {
			sign = 1
		}
		return ExtendedReal{Sign: sign, Exp: 0, Mant: 0}
	}

	bits := math.Float64bits(f)
	sign := uint8((bits >> 63) & 1)
	f64Exp := int((bits >> 52) & 0x7FF)
	f64Mant := bits & 0x000FFFFFFFFFFFFF

	if f64Exp == 0 {
		if f64Mant == 0 {
			return ExtendedReal{Sign: sign, Exp: 0, Mant: 0}
		}

		shift := 0
		for (f64Mant & (1 << 51)) == 0 {
			f64Mant <<= 1
			shift++
		}

		extExp := uint16(15360 - 1022 - shift)
		extMant := (f64Mant << 12) | extMantMSB
		return ExtendedReal{Sign: sign, Exp: extExp, Mant: extMant}
	}

	extExp := uint16(f64Exp + 15360)
	extMant := ((f64Mant | (1 << 52)) << 11)
	return ExtendedReal{Sign: sign, Exp: extExp, Mant: extMant}
}

// ToFloat64 converts an ExtendedReal to float64
func (e ExtendedReal) ToFloat64() float64 {
	if e.IsNaN() {
		return math.NaN()
	}

	if e.IsInf() {
		if e.Sign == 0 {
			return math.Inf(1)
		}
		return math.Inf(-1)
	}

	if e.IsZero() {
		if e.Sign == 0 {
			return 0.0
		}
		return math.Copysign(0.0, -1.0)
	}

	extExpUnbiased := int(e.Exp) - 16383
	f64Exp := extExpUnbiased + 1023

	if f64Exp <= 0 {
		if f64Exp < -52 {
			if e.Sign == 0 {
				return 0.0
			}
			return math.Copysign(0.0, -1.0)
		}

		shift := 1 - f64Exp
		f64Mant := e.Mant >> (11 + uint(shift))
		bits := uint64(e.Sign)<<63 | f64Mant
		return math.Float64frombits(bits)
	}

	if f64Exp >= 0x7FF {
		if e.Sign == 0 {
			return math.Inf(1)
		}
		return math.Inf(-1)
	}

	f64Mant := (e.Mant >> 11) & 0x000FFFFFFFFFFFFF
	bits := uint64(e.Sign)<<63 | uint64(f64Exp)<<52 | f64Mant
	return math.Float64frombits(bits)
}

// IsZero returns true if the value is zero (positive or negative)
func (e ExtendedReal) IsZero() bool {
	return e.Exp == 0 && e.Mant == 0
}

// IsInf returns true if the value is infinity
func (e ExtendedReal) IsInf() bool {
	return e.Exp == extExpMax && e.Mant == extMantMSB
}

// IsNaN returns true if the value is Not a Number
func (e ExtendedReal) IsNaN() bool {
	return e.Exp == extExpMax && e.Mant != extMantMSB && e.Mant != 0
}

// =============================================================================
// M68881FPU - FPU State and Registers
// =============================================================================

// M68881FPU represents the Motorola 68881/68882 Floating Point Unit
type M68881FPU struct {
	fp    [8]float64 // FP0-FP7 registers; cache-line sized hot storage
	FPCR  uint32     // FPU Control Register
	FPSR  uint32     // FPU Status Register
	FPIAR uint32     // FPU Instruction Address Register
	_     uint32     // Alignment/padding
}

// NewM68881FPU creates a new FPU with default state
func NewM68881FPU() *M68881FPU {
	return &M68881FPU{}
}

func (fpu *M68881FPU) GetFP64(reg int) float64 {
	return fpu.fp[reg]
}

func (fpu *M68881FPU) SetFP64(reg int, val float64) {
	fpu.fp[reg] = val
}

func (fpu *M68881FPU) GetExtendedReal(reg int) ExtendedReal {
	return ExtendedRealFromFloat64(fpu.fp[reg])
}

func (fpu *M68881FPU) SetFromExtendedReal(reg int, ext ExtendedReal) {
	fpu.fp[reg] = ext.ToFloat64()
}

// GetRoundingMode returns the current rounding mode from FPCR
func (fpu *M68881FPU) GetRoundingMode() uint8 {
	return uint8((fpu.FPCR >> 4) & 0x03)
}

// SetRoundingMode sets the rounding mode in FPCR
func (fpu *M68881FPU) SetRoundingMode(mode uint8) {
	fpu.FPCR = (fpu.FPCR & ^uint32(0x30)) | (uint32(mode&0x03) << 4)
}

// GetPrecision returns the current precision from FPCR
func (fpu *M68881FPU) GetPrecision() uint8 {
	return uint8((fpu.FPCR >> 6) & 0x03)
}

// SetPrecision sets the precision in FPCR
func (fpu *M68881FPU) SetPrecision(prec uint8) {
	fpu.FPCR = (fpu.FPCR & ^uint32(0xC0)) | (uint32(prec&0x03) << 6)
}

// GetConditionN returns the N (negative) condition code
func (fpu *M68881FPU) GetConditionN() bool {
	return (fpu.FPSR & FPU_CC_N) != 0
}

// GetConditionZ returns the Z (zero) condition code
func (fpu *M68881FPU) GetConditionZ() bool {
	return (fpu.FPSR & FPU_CC_Z) != 0
}

// GetConditionI returns the I (infinity) condition code
func (fpu *M68881FPU) GetConditionI() bool {
	return (fpu.FPSR & FPU_CC_I) != 0
}

// GetConditionNAN returns the NAN condition code
func (fpu *M68881FPU) GetConditionNAN() bool {
	return (fpu.FPSR & FPU_CC_NAN) != 0
}

func (fpu *M68881FPU) setCC64(val float64) {
	bits := math.Float64bits(val)
	exp := bits & 0x7FF0000000000000
	frac := bits & 0x000FFFFFFFFFFFFF
	sign := bits >> 63

	var cc uint32
	if exp == 0x7FF0000000000000 {
		if frac != 0 {
			cc = FPU_CC_NAN
		} else {
			cc = FPU_CC_I
			if sign != 0 {
				cc |= FPU_CC_N
			}
		}
	} else if exp|frac == 0 {
		cc = FPU_CC_Z
	} else if sign != 0 {
		cc = FPU_CC_N
	}

	fpu.FPSR = (fpu.FPSR & ^fpuCCMask) | cc
}

// =============================================================================
// FPU Instructions - Data Movement
// =============================================================================

// FMOVE_RegToReg copies a value from one FP register to another
func (fpu *M68881FPU) FMOVE_RegToReg(src, dst int) {
	fpu.fp[dst] = fpu.fp[src]
	fpu.setCC64(fpu.fp[dst])
}

// FMOVE_ImmToReg loads an immediate value into an FP register
func (fpu *M68881FPU) FMOVE_ImmToReg(value float64, dst int) {
	fpu.fp[dst] = value
	fpu.setCC64(value)
}

// =============================================================================
// FPU Instructions - Arithmetic
// =============================================================================

// FADD adds FPsrc to FPdst and stores result in FPdst
func (fpu *M68881FPU) FADD(src, dst int) {
	fpu.fp[dst] += fpu.fp[src]
	fpu.setCC64(fpu.fp[dst])
}

// FSUB subtracts FPsrc from FPdst and stores result in FPdst
func (fpu *M68881FPU) FSUB(src, dst int) {
	fpu.fp[dst] -= fpu.fp[src]
	fpu.setCC64(fpu.fp[dst])
}

// FMUL multiplies FPdst by FPsrc and stores result in FPdst
func (fpu *M68881FPU) FMUL(src, dst int) {
	fpu.fp[dst] *= fpu.fp[src]
	fpu.setCC64(fpu.fp[dst])
}

// FDIV divides FPdst by FPsrc and stores result in FPdst
func (fpu *M68881FPU) FDIV(src, dst int) {
	fpu.fp[dst] /= fpu.fp[src]
	fpu.setCC64(fpu.fp[dst])
}

// FNEG negates FPsrc and stores result in FPdst
func (fpu *M68881FPU) FNEG(src, dst int) {
	fpu.fp[dst] = -fpu.fp[src]
	fpu.setCC64(fpu.fp[dst])
}

// FABS takes absolute value of FPsrc and stores result in FPdst
func (fpu *M68881FPU) FABS(src, dst int) {
	fpu.fp[dst] = math.Abs(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// =============================================================================
// FPU Instructions - Comparison
// =============================================================================

// FCMP compares FPdst with FPsrc and sets condition codes
func (fpu *M68881FPU) FCMP(src, dst int) {
	a := fpu.fp[dst]
	b := fpu.fp[src]

	fpu.FPSR &= ^fpuCCMask

	if math.IsNaN(a) || math.IsNaN(b) {
		fpu.FPSR |= FPU_CC_NAN
		return
	}

	diff := a - b
	if diff == 0 {
		fpu.FPSR |= FPU_CC_Z
	} else if diff < 0 {
		fpu.FPSR |= FPU_CC_N
	}
}

// FTST tests FP register and sets condition codes
func (fpu *M68881FPU) FTST(reg int) {
	fpu.setCC64(fpu.fp[reg])
}

// =============================================================================
// FPU Instructions - Transcendentals
// =============================================================================

// FSQRT calculates square root of FPsrc and stores in FPdst
func (fpu *M68881FPU) FSQRT(src, dst int) {
	fpu.fp[dst] = math.Sqrt(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FSIN calculates sine of FPsrc and stores in FPdst
func (fpu *M68881FPU) FSIN(src, dst int) {
	fpu.fp[dst] = math.Sin(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FCOS calculates cosine of FPsrc and stores in FPdst
func (fpu *M68881FPU) FCOS(src, dst int) {
	fpu.fp[dst] = math.Cos(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FTAN calculates tangent of FPsrc and stores in FPdst
func (fpu *M68881FPU) FTAN(src, dst int) {
	fpu.fp[dst] = math.Tan(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FLOG10 calculates base-10 logarithm of FPsrc and stores in FPdst
func (fpu *M68881FPU) FLOG10(src, dst int) {
	fpu.fp[dst] = math.Log10(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FLOGN calculates natural logarithm of FPsrc and stores in FPdst
func (fpu *M68881FPU) FLOGN(src, dst int) {
	fpu.fp[dst] = math.Log(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FLOG2 calculates base-2 logarithm of FPsrc and stores in FPdst
func (fpu *M68881FPU) FLOG2(src, dst int) {
	fpu.fp[dst] = math.Log2(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FETOX calculates e^x for FPsrc and stores in FPdst
func (fpu *M68881FPU) FETOX(src, dst int) {
	fpu.fp[dst] = math.Exp(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FTWOTOX calculates 2^x for FPsrc and stores in FPdst
func (fpu *M68881FPU) FTWOTOX(src, dst int) {
	fpu.fp[dst] = math.Exp2(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FTENTOX calculates 10^x for FPsrc and stores in FPdst
func (fpu *M68881FPU) FTENTOX(src, dst int) {
	fpu.fp[dst] = math.Pow(10, fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FASIN calculates arcsine of FPsrc and stores in FPdst
func (fpu *M68881FPU) FASIN(src, dst int) {
	fpu.fp[dst] = math.Asin(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FACOS calculates arccosine of FPsrc and stores in FPdst
func (fpu *M68881FPU) FACOS(src, dst int) {
	fpu.fp[dst] = math.Acos(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FATAN calculates arctangent of FPsrc and stores in FPdst
func (fpu *M68881FPU) FATAN(src, dst int) {
	fpu.fp[dst] = math.Atan(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// =============================================================================
// FPU Instructions - Rounding
// =============================================================================

// FINT rounds FPsrc to integer using current rounding mode
func (fpu *M68881FPU) FINT(src, dst int) {
	a := fpu.fp[src]
	var result float64

	switch fpu.GetRoundingMode() {
	case FPU_RND_NEAREST:
		result = math.RoundToEven(a)
	case FPU_RND_ZERO:
		result = math.Trunc(a)
	case FPU_RND_MINUS_INF:
		result = math.Floor(a)
	case FPU_RND_PLUS_INF:
		result = math.Ceil(a)
	default:
		result = math.RoundToEven(a)
	}

	fpu.fp[dst] = result
	fpu.setCC64(result)
}

// FINTRZ rounds FPsrc toward zero (truncate)
func (fpu *M68881FPU) FINTRZ(src, dst int) {
	fpu.fp[dst] = math.Trunc(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// =============================================================================
// FPU ROM Constants
// =============================================================================

// FMOVECR loads a constant from the FPU ROM into FPdst
func (fpu *M68881FPU) FMOVECR(romAddr uint8, dst int) {
	idx := romAddr & 0x3F
	fpu.fp[dst] = fmovecrROMTable[idx]
	fpu.setCC64(fpu.fp[dst])
}

// =============================================================================
// Additional FPU Instructions
// =============================================================================

// FSINH calculates hyperbolic sine
func (fpu *M68881FPU) FSINH(src, dst int) {
	fpu.fp[dst] = math.Sinh(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FCOSH calculates hyperbolic cosine
func (fpu *M68881FPU) FCOSH(src, dst int) {
	fpu.fp[dst] = math.Cosh(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FTANH calculates hyperbolic tangent
func (fpu *M68881FPU) FTANH(src, dst int) {
	fpu.fp[dst] = math.Tanh(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FATANH calculates inverse hyperbolic tangent
func (fpu *M68881FPU) FATANH(src, dst int) {
	fpu.fp[dst] = math.Atanh(fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FMOD calculates modulo: FPdst = FPdst mod FPsrc
func (fpu *M68881FPU) FMOD(src, dst int) {
	fpu.fp[dst] = math.Mod(fpu.fp[dst], fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FREM calculates IEEE remainder: FPdst = IEEE_remainder(FPdst, FPsrc)
func (fpu *M68881FPU) FREM(src, dst int) {
	fpu.fp[dst] = math.Remainder(fpu.fp[dst], fpu.fp[src])
	fpu.setCC64(fpu.fp[dst])
}

// FGETEXP extracts exponent of FPsrc and stores in FPdst
func (fpu *M68881FPU) FGETEXP(src, dst int) {
	a := fpu.fp[src]
	if a == 0 {
		fpu.fp[dst] = 0
	} else {
		_, exp := math.Frexp(a)
		fpu.fp[dst] = float64(exp - 1)
	}
	fpu.setCC64(fpu.fp[dst])
}

// FGETMAN extracts mantissa of FPsrc and stores in FPdst
func (fpu *M68881FPU) FGETMAN(src, dst int) {
	a := fpu.fp[src]
	if a == 0 {
		fpu.fp[dst] = 0
	} else {
		mant, _ := math.Frexp(a)
		fpu.fp[dst] = mant * 2
	}
	fpu.setCC64(fpu.fp[dst])
}

// FSCALE scales FPdst by 2^FPsrc
func (fpu *M68881FPU) FSCALE(src, dst int) {
	fpu.fp[dst] = math.Ldexp(fpu.fp[dst], int(fpu.fp[src]))
	fpu.setCC64(fpu.fp[dst])
}

// FSGLDIV performs single-precision division
func (fpu *M68881FPU) FSGLDIV(src, dst int) {
	a := float32(fpu.fp[dst])
	b := float32(fpu.fp[src])
	fpu.fp[dst] = float64(a / b)
	fpu.setCC64(fpu.fp[dst])
}

// FSGLMUL performs single-precision multiplication
func (fpu *M68881FPU) FSGLMUL(src, dst int) {
	a := float32(fpu.fp[dst])
	b := float32(fpu.fp[src])
	fpu.fp[dst] = float64(a * b)
	fpu.setCC64(fpu.fp[dst])
}

// AddImm applies FPdst += val and updates condition codes.
func (fpu *M68881FPU) AddImm(dst int, val float64) {
	fpu.fp[dst] += val
	fpu.setCC64(fpu.fp[dst])
}

// SubImm applies FPdst -= val and updates condition codes.
func (fpu *M68881FPU) SubImm(dst int, val float64) {
	fpu.fp[dst] -= val
	fpu.setCC64(fpu.fp[dst])
}

// MulImm applies FPdst *= val and updates condition codes.
func (fpu *M68881FPU) MulImm(dst int, val float64) {
	fpu.fp[dst] *= val
	fpu.setCC64(fpu.fp[dst])
}

// DivImm applies FPdst /= val and updates condition codes.
func (fpu *M68881FPU) DivImm(dst int, val float64) {
	fpu.fp[dst] /= val
	fpu.setCC64(fpu.fp[dst])
}

// MoveImm applies FPdst = val and updates condition codes.
func (fpu *M68881FPU) MoveImm(dst int, val float64) {
	fpu.fp[dst] = val
	fpu.setCC64(val)
}

// CmpImm compares FPdst against val and updates compare condition codes.
func (fpu *M68881FPU) CmpImm(dst int, val float64) {
	a := fpu.fp[dst]
	fpu.FPSR &= ^fpuCCMask
	if math.IsNaN(a) || math.IsNaN(val) {
		fpu.FPSR |= FPU_CC_NAN
		return
	}
	diff := a - val
	if diff == 0 {
		fpu.FPSR |= FPU_CC_Z
	} else if diff < 0 {
		fpu.FPSR |= FPU_CC_N
	}
}
