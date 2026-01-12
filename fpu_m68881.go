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

// Extended precision constants
const (
	extExpBias uint16 = 16383   // Exponent bias for 80-bit
	extExpMax  uint16 = 0x7FFF  // Maximum exponent (infinity/NaN)
	extMantMSB uint64 = 1 << 63 // Explicit integer bit
)

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

	// Convert from float64 (bias 1023) to extended (bias 16383)
	// float64 exponent: biased by 1023
	// extended exponent: biased by 16383
	// Difference: 16383 - 1023 = 15360

	if f64Exp == 0 {
		// Denormalized float64 - normalize it
		// Find the leading 1 bit
		if f64Mant == 0 {
			return ExtendedReal{Sign: sign, Exp: 0, Mant: 0}
		}

		// Shift mantissa left until we find the leading 1
		shift := 0
		for (f64Mant & (1 << 51)) == 0 {
			f64Mant <<= 1
			shift++
		}

		// Now f64Mant has the leading 1 in bit 51
		extExp := uint16(15360 - 1022 - shift)
		extMant := (f64Mant << 12) | extMantMSB

		return ExtendedReal{Sign: sign, Exp: extExp, Mant: extMant}
	}

	// Normal number
	extExp := uint16(f64Exp + 15360)

	// Extended format has explicit integer bit
	// float64 mantissa is 52 bits (implied leading 1)
	// extended mantissa is 64 bits (explicit leading 1)
	// Shift left by 11 bits and add explicit 1
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

	// Convert exponent from extended (bias 16383) to float64 (bias 1023)
	extExpUnbiased := int(e.Exp) - 16383
	f64Exp := extExpUnbiased + 1023

	if f64Exp <= 0 {
		// Denormalized or too small for float64
		if f64Exp < -52 {
			// Too small - return signed zero
			if e.Sign == 0 {
				return 0.0
			}
			return math.Copysign(0.0, -1.0)
		}

		// Denormalized - shift right
		shift := 1 - f64Exp
		f64Mant := e.Mant >> (11 + uint(shift))
		bits := uint64(e.Sign)<<63 | f64Mant
		return math.Float64frombits(bits)
	}

	if f64Exp >= 0x7FF {
		// Too large - return infinity
		if e.Sign == 0 {
			return math.Inf(1)
		}
		return math.Inf(-1)
	}

	// Normal number
	// Remove explicit leading 1 and shift right by 11
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

// Negate returns the negation of the value
func (e ExtendedReal) Negate() ExtendedReal {
	return ExtendedReal{
		Sign: 1 - e.Sign,
		Exp:  e.Exp,
		Mant: e.Mant,
	}
}

// Abs returns the absolute value
func (e ExtendedReal) Abs() ExtendedReal {
	return ExtendedReal{
		Sign: 0,
		Exp:  e.Exp,
		Mant: e.Mant,
	}
}

// =============================================================================
// M68881FPU - FPU State and Registers
// =============================================================================

// M68881FPU represents the Motorola 68881/68882 Floating Point Unit
type M68881FPU struct {
	FPRegs [8]ExtendedReal // FP0-FP7 registers
	FPCR   uint32          // FPU Control Register
	FPSR   uint32          // FPU Status Register
	FPIAR  uint32          // FPU Instruction Address Register
}

// NewM68881FPU creates a new FPU with default state
func NewM68881FPU() *M68881FPU {
	return &M68881FPU{
		FPCR:  0, // Default: RN rounding, extended precision, no exceptions enabled
		FPSR:  0,
		FPIAR: 0,
	}
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

// SetConditionCodes sets the condition codes based on a result
func (fpu *M68881FPU) SetConditionCodes(result ExtendedReal) {
	fpu.FPSR &= ^(FPU_CC_N | FPU_CC_Z | FPU_CC_I | FPU_CC_NAN)

	if result.IsNaN() {
		fpu.FPSR |= FPU_CC_NAN
		return
	}

	if result.IsInf() {
		fpu.FPSR |= FPU_CC_I
		if result.Sign != 0 {
			fpu.FPSR |= FPU_CC_N
		}
		return
	}

	if result.IsZero() {
		fpu.FPSR |= FPU_CC_Z
		return
	}

	if result.Sign != 0 {
		fpu.FPSR |= FPU_CC_N
	}
}

// =============================================================================
// FPU Instructions - Data Movement
// =============================================================================

// FMOVE_RegToReg copies a value from one FP register to another
func (fpu *M68881FPU) FMOVE_RegToReg(src, dst int) {
	fpu.FPRegs[dst] = fpu.FPRegs[src]
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FMOVE_ImmToReg loads an immediate value into an FP register
func (fpu *M68881FPU) FMOVE_ImmToReg(value float64, dst int) {
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(value)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// =============================================================================
// FPU Instructions - Arithmetic
// =============================================================================

// FADD adds FPsrc to FPdst and stores result in FPdst
func (fpu *M68881FPU) FADD(src, dst int) {
	a := fpu.FPRegs[dst].ToFloat64()
	b := fpu.FPRegs[src].ToFloat64()
	result := a + b
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FSUB subtracts FPsrc from FPdst and stores result in FPdst
func (fpu *M68881FPU) FSUB(src, dst int) {
	a := fpu.FPRegs[dst].ToFloat64()
	b := fpu.FPRegs[src].ToFloat64()
	result := a - b
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FMUL multiplies FPdst by FPsrc and stores result in FPdst
func (fpu *M68881FPU) FMUL(src, dst int) {
	a := fpu.FPRegs[dst].ToFloat64()
	b := fpu.FPRegs[src].ToFloat64()
	result := a * b
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FDIV divides FPdst by FPsrc and stores result in FPdst
func (fpu *M68881FPU) FDIV(src, dst int) {
	a := fpu.FPRegs[dst].ToFloat64()
	b := fpu.FPRegs[src].ToFloat64()
	result := a / b
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FNEG negates FPsrc and stores result in FPdst
func (fpu *M68881FPU) FNEG(src, dst int) {
	fpu.FPRegs[dst] = fpu.FPRegs[src].Negate()
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FABS takes absolute value of FPsrc and stores result in FPdst
func (fpu *M68881FPU) FABS(src, dst int) {
	fpu.FPRegs[dst] = fpu.FPRegs[src].Abs()
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// =============================================================================
// FPU Instructions - Comparison
// =============================================================================

// FCMP compares FPdst with FPsrc and sets condition codes
func (fpu *M68881FPU) FCMP(src, dst int) {
	a := fpu.FPRegs[dst].ToFloat64()
	b := fpu.FPRegs[src].ToFloat64()

	fpu.FPSR &= ^(FPU_CC_N | FPU_CC_Z | FPU_CC_I | FPU_CC_NAN)

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
	fpu.SetConditionCodes(fpu.FPRegs[reg])
}

// =============================================================================
// FPU Instructions - Transcendentals
// =============================================================================

// FSQRT calculates square root of FPsrc and stores in FPdst
func (fpu *M68881FPU) FSQRT(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Sqrt(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FSIN calculates sine of FPsrc and stores in FPdst
func (fpu *M68881FPU) FSIN(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Sin(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FCOS calculates cosine of FPsrc and stores in FPdst
func (fpu *M68881FPU) FCOS(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Cos(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FTAN calculates tangent of FPsrc and stores in FPdst
func (fpu *M68881FPU) FTAN(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Tan(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FLOG10 calculates base-10 logarithm of FPsrc and stores in FPdst
func (fpu *M68881FPU) FLOG10(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Log10(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FLOGN calculates natural logarithm of FPsrc and stores in FPdst
func (fpu *M68881FPU) FLOGN(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Log(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FLOG2 calculates base-2 logarithm of FPsrc and stores in FPdst
func (fpu *M68881FPU) FLOG2(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Log2(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FETOX calculates e^x for FPsrc and stores in FPdst
func (fpu *M68881FPU) FETOX(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Exp(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FTWOTOX calculates 2^x for FPsrc and stores in FPdst
func (fpu *M68881FPU) FTWOTOX(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Pow(2, a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FTENTOX calculates 10^x for FPsrc and stores in FPdst
func (fpu *M68881FPU) FTENTOX(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Pow(10, a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FASIN calculates arcsine of FPsrc and stores in FPdst
func (fpu *M68881FPU) FASIN(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Asin(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FACOS calculates arccosine of FPsrc and stores in FPdst
func (fpu *M68881FPU) FACOS(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Acos(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FATAN calculates arctangent of FPsrc and stores in FPdst
func (fpu *M68881FPU) FATAN(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Atan(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// =============================================================================
// FPU Instructions - Rounding
// =============================================================================

// FINT rounds FPsrc to integer using current rounding mode
func (fpu *M68881FPU) FINT(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
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

	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FINTRZ rounds FPsrc toward zero (truncate)
func (fpu *M68881FPU) FINTRZ(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Trunc(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// =============================================================================
// FPU ROM Constants
// =============================================================================

// FMOVECR loads a constant from the FPU ROM into FPdst
func (fpu *M68881FPU) FMOVECR(romAddr uint8, dst int) {
	var value float64

	switch romAddr {
	case 0x00:
		value = math.Pi
	case 0x0B:
		value = math.Log10(2) // log10(2)
	case 0x0C:
		value = math.E
	case 0x0D:
		value = math.Log2E // log2(e)
	case 0x0E:
		value = math.Log10E // log10(e)
	case 0x0F:
		value = 0.0
	case 0x30:
		value = math.Ln2 // ln(2)
	case 0x31:
		value = math.Ln10 // ln(10)
	case 0x32:
		value = 1.0 // 10^0
	case 0x33:
		value = 10.0 // 10^1
	case 0x34:
		value = 100.0 // 10^2
	case 0x35:
		value = 1000.0 // 10^3
	case 0x36:
		value = 10000.0 // 10^4
	case 0x37:
		value = 100000.0 // 10^5
	case 0x38:
		value = 1000000.0 // 10^6
	case 0x39:
		value = 10000000.0 // 10^7
	case 0x3A:
		value = 100000000.0 // 10^8
	default:
		// Unknown constant - return zero
		value = 0.0
	}

	fpu.FPRegs[dst] = ExtendedRealFromFloat64(value)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// =============================================================================
// Additional FPU Instructions
// =============================================================================

// FSINH calculates hyperbolic sine
func (fpu *M68881FPU) FSINH(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Sinh(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FCOSH calculates hyperbolic cosine
func (fpu *M68881FPU) FCOSH(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Cosh(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FTANH calculates hyperbolic tangent
func (fpu *M68881FPU) FTANH(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Tanh(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FATANH calculates inverse hyperbolic tangent
func (fpu *M68881FPU) FATANH(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	result := math.Atanh(a)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FMOD calculates modulo: FPdst = FPdst mod FPsrc
func (fpu *M68881FPU) FMOD(src, dst int) {
	a := fpu.FPRegs[dst].ToFloat64()
	b := fpu.FPRegs[src].ToFloat64()
	result := math.Mod(a, b)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FREM calculates IEEE remainder: FPdst = IEEE_remainder(FPdst, FPsrc)
func (fpu *M68881FPU) FREM(src, dst int) {
	a := fpu.FPRegs[dst].ToFloat64()
	b := fpu.FPRegs[src].ToFloat64()
	result := math.Remainder(a, b)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FGETEXP extracts exponent of FPsrc and stores in FPdst
func (fpu *M68881FPU) FGETEXP(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	if a == 0 {
		fpu.FPRegs[dst] = ExtendedRealFromFloat64(0)
	} else {
		_, exp := math.Frexp(a)
		fpu.FPRegs[dst] = ExtendedRealFromFloat64(float64(exp - 1))
	}
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FGETMAN extracts mantissa of FPsrc and stores in FPdst
func (fpu *M68881FPU) FGETMAN(src, dst int) {
	a := fpu.FPRegs[src].ToFloat64()
	if a == 0 {
		fpu.FPRegs[dst] = ExtendedRealFromFloat64(0)
	} else {
		mant, _ := math.Frexp(a)
		// Frexp returns mantissa in [0.5, 1), 68881 wants [1, 2)
		fpu.FPRegs[dst] = ExtendedRealFromFloat64(mant * 2)
	}
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FSCALE scales FPdst by 2^FPsrc
func (fpu *M68881FPU) FSCALE(src, dst int) {
	a := fpu.FPRegs[dst].ToFloat64()
	b := fpu.FPRegs[src].ToFloat64()
	result := math.Ldexp(a, int(b))
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FSGLDIV performs single-precision division
func (fpu *M68881FPU) FSGLDIV(src, dst int) {
	a := float32(fpu.FPRegs[dst].ToFloat64())
	b := float32(fpu.FPRegs[src].ToFloat64())
	result := float64(a / b)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}

// FSGLMUL performs single-precision multiplication
func (fpu *M68881FPU) FSGLMUL(src, dst int) {
	a := float32(fpu.FPRegs[dst].ToFloat64())
	b := float32(fpu.FPRegs[src].ToFloat64())
	result := float64(a * b)
	fpu.FPRegs[dst] = ExtendedRealFromFloat64(result)
	fpu.SetConditionCodes(fpu.FPRegs[dst])
}
