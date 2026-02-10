package main

import (
	"math"
	"testing"
)

func TestIE64FPU_Init(t *testing.T) {
	fpu := NewIE64FPU()
	for i, reg := range fpu.FPRegs {
		if reg != 0 {
			t.Errorf("Expected F%d to be 0, got %v", i, reg)
		}
	}
	if fpu.FPCR != 0 {
		t.Errorf("Expected FPCR to be 0, got %v", fpu.FPCR)
	}
	if fpu.FPSR != 0 {
		t.Errorf("Expected FPSR to be 0, got %v", fpu.FPSR)
	}
}

func TestIE64FPU_RoundingMode(t *testing.T) {
	fpu := NewIE64FPU()
	modes := []uint8{
		IE64_FPU_RND_NEAREST,
		IE64_FPU_RND_ZERO,
		IE64_FPU_RND_FLOOR,
		IE64_FPU_RND_CEIL,
	}

	for _, mode := range modes {
		fpu.SetRoundingMode(mode)
		if fpu.GetRoundingMode() != mode {
			t.Errorf("Expected rounding mode %v, got %v", mode, fpu.GetRoundingMode())
		}
		// Verify other bits of FPCR are preserved (currently none, but let's test isolation)
		if fpu.FPCR != uint32(mode) {
			t.Errorf("FPCR mismatch: expected %v, got %v", mode, fpu.FPCR)
		}
	}
}

func TestIE64FPU_ConditionCodes(t *testing.T) {
	fpu := NewIE64FPU()

	tests := []struct {
		name string
		val  float32
		want uint32
	}{
		{"Positive", 1.0, 0},
		{"Negative", -1.0, IE64_FPU_CC_N},
		{"Zero", 0.0, IE64_FPU_CC_Z},
		{"NegativeZero", float32(math.Copysign(0, -1)), IE64_FPU_CC_Z},
		{"Infinity", float32(math.Inf(1)), IE64_FPU_CC_I},
		{"NegativeInfinity", float32(math.Inf(-1)), IE64_FPU_CC_I | IE64_FPU_CC_N},
		{"NaN", float32(math.NaN()), IE64_FPU_CC_NAN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu.setConditionCodes(tt.val)
			got := fpu.FPSR & 0x0F000000 // CC bits
			if got != tt.want {
				t.Errorf("setConditionCodes(%v) CC = %08x, want %08x", tt.val, got, tt.want)
			}
		})
	}
}

func TestIE64FPU_ExceptionFlagsSticky(t *testing.T) {
	fpu := NewIE64FPU()

	// Set IO flag
	fpu.setExceptionFlag(IE64_FPU_EX_IO)
	if (fpu.FPSR & IE64_FPU_EX_IO) == 0 {
		t.Fatal("IO flag not set")
	}

	// Perform "normal" CC update, verify IO still set
	fpu.setConditionCodes(1.0)
	if (fpu.FPSR & IE64_FPU_EX_IO) == 0 {
		t.Error("IO flag was cleared by setConditionCodes (should be sticky)")
	}

	// Set DZ flag
	fpu.setExceptionFlag(IE64_FPU_EX_DZ)
	if (fpu.FPSR & (IE64_FPU_EX_IO | IE64_FPU_EX_DZ)) != (IE64_FPU_EX_IO | IE64_FPU_EX_DZ) {
		t.Error("Flags not accumulating")
	}

	// Clear via direct write (model of fmovsc)
	fpu.FMOVSC(0)
	if fpu.FPSR != 0 {
		t.Error("Failed to clear FPSR")
	}

	// Test masking of reserved bits
	fpu.FMOVSC(0xFFFFFFFF)
	if fpu.FPSR != IE64_FPU_FPSR_MASK {
		t.Errorf("FMOVSC masking failed: got %08x, want %08x", fpu.FPSR, IE64_FPU_FPSR_MASK)
	}
}

func TestIE64FPU_FCMP_SpecialValues(t *testing.T) {
	fpu := NewIE64FPU()
	inf := float32(math.Inf(1))
	negInf := float32(math.Inf(-1))

	t.Run("Inf_Inf", func(t *testing.T) {
		fpu.setFReg(1, inf)
		fpu.setFReg(2, inf)
		res := fpu.FCMP(1, 2)
		if res != 0 {
			t.Errorf("FCMP(Inf, Inf) = %v, want 0", res)
		}
		if (fpu.FPSR & IE64_FPU_CC_Z) == 0 {
			t.Error("Zero bit not set")
		}
		if (fpu.FPSR & IE64_FPU_CC_I) == 0 {
			t.Error("Infinity bit not set")
		}
		if (fpu.FPSR & IE64_FPU_CC_NAN) != 0 {
			t.Error("NaN bit set")
		}
	})

	t.Run("Inf_NegInf", func(t *testing.T) {
		fpu.setFReg(1, inf)
		fpu.setFReg(2, negInf)
		res := fpu.FCMP(1, 2)
		if res != 1 {
			t.Errorf("FCMP(Inf, -Inf) = %v, want 1", res)
		}
		if (fpu.FPSR & IE64_FPU_CC_I) == 0 {
			t.Error("Infinity bit not set")
		}
	})
}

func TestIE64FPU_Arithmetic(t *testing.T) {
	fpu := NewIE64FPU()

	t.Run("FADD", func(t *testing.T) {
		fpu.FPSR = 0
		fpu.setFReg(1, 1.5)
		fpu.setFReg(2, 2.5)
		fpu.FADD(0, 1, 2)
		if fpu.getFReg(0) != 4.0 {
			t.Errorf("1.5 + 2.5 = %v, want 4.0", fpu.getFReg(0))
		}
		if (fpu.FPSR&IE64_FPU_CC_Z) != 0 || (fpu.FPSR&IE64_FPU_CC_N) != 0 {
			t.Errorf("Unexpected CC: %08x", fpu.FPSR)
		}
	})

	t.Run("FSUB", func(t *testing.T) {
		fpu.FPSR = 0
		fpu.setFReg(1, 10.0)
		fpu.setFReg(2, 3.0)
		fpu.FSUB(0, 1, 2)
		if fpu.getFReg(0) != 7.0 {
			t.Errorf("10.0 - 3.0 = %v, want 7.0", fpu.getFReg(0))
		}
	})

	t.Run("FMUL", func(t *testing.T) {
		fpu.FPSR = 0
		fpu.setFReg(1, 3.0)
		fpu.setFReg(2, 4.0)
		fpu.FMUL(0, 1, 2)
		if fpu.getFReg(0) != 12.0 {
			t.Errorf("3.0 * 4.0 = %v, want 12.0", fpu.getFReg(0))
		}
	})

	t.Run("FDIV", func(t *testing.T) {
		fpu.FPSR = 0
		fpu.setFReg(1, 12.0)
		fpu.setFReg(2, 4.0)
		fpu.FDIV(0, 1, 2)
		if fpu.getFReg(0) != 3.0 {
			t.Errorf("12.0 / 4.0 = %v, want 3.0", fpu.getFReg(0))
		}
	})

	t.Run("FMOD", func(t *testing.T) {
		fpu.FPSR = 0
		fpu.setFReg(1, 10.0)
		fpu.setFReg(2, 3.0)
		fpu.FMOD(0, 1, 2)
		if fpu.getFReg(0) != 1.0 {
			t.Errorf("10.0 mod 3.0 = %v, want 1.0", fpu.getFReg(0))
		}
	})
}

func TestIE64FPU_Arithmetic_Exceptions(t *testing.T) {
	fpu := NewIE64FPU()

	t.Run("DivideByZero", func(t *testing.T) {
		fpu.FPSR = 0
		fpu.setFReg(1, 1.0)
		fpu.setFReg(2, 0.0)
		fpu.FDIV(0, 1, 2)
		if !math.IsInf(float64(fpu.getFReg(0)), 1) {
			t.Errorf("1.0 / 0.0 = %v, want +Inf", fpu.getFReg(0))
		}
		if (fpu.FPSR & IE64_FPU_EX_DZ) == 0 {
			t.Error("DZ flag not set")
		}
	})

	t.Run("InvalidOp_InfMinusInf", func(t *testing.T) {
		fpu.FPSR = 0
		fpu.setFReg(1, float32(math.Inf(1)))
		fpu.setFReg(2, float32(math.Inf(1)))
		fpu.FSUB(0, 1, 2)
		if !math.IsNaN(float64(fpu.getFReg(0))) {
			t.Errorf("Inf - Inf = %v, want NaN", fpu.getFReg(0))
		}
		if (fpu.FPSR & IE64_FPU_EX_IO) == 0 {
			t.Error("IO flag not set")
		}
	})

	t.Run("Overflow", func(t *testing.T) {
		fpu.FPSR = 0
		fpu.setFReg(1, math.MaxFloat32)
		fpu.setFReg(2, 2.0)
		fpu.FMUL(0, 1, 2)
		if !math.IsInf(float64(fpu.getFReg(0)), 1) {
			t.Errorf("Max * 2 = %v, want +Inf", fpu.getFReg(0))
		}
		if (fpu.FPSR & IE64_FPU_EX_OE) == 0 {
			t.Error("OE flag not set")
		}
	})

	t.Run("Underflow", func(t *testing.T) {
		fpu.FPSR = 0
		fpu.setFReg(1, math.SmallestNonzeroFloat32)
		fpu.setFReg(2, 0.5)
		fpu.FMUL(0, 1, 2)
		if fpu.getFReg(0) != 0 {
			t.Errorf("Smallest * 0.5 = %v, want 0.0", fpu.getFReg(0))
		}
		if (fpu.FPSR & IE64_FPU_EX_UE) == 0 {
			t.Error("UE flag not set")
		}
	})
}

func TestIE64FPU_UnaryAndTranscendentals(t *testing.T) {
	fpu := NewIE64FPU()

	t.Run("FABS", func(t *testing.T) {
		fpu.setFReg(1, -5.5)
		fpu.FABS(0, 1)
		if fpu.getFReg(0) != 5.5 {
			t.Errorf("abs(-5.5) = %v", fpu.getFReg(0))
		}
	})

	t.Run("FSQRT", func(t *testing.T) {
		fpu.setFReg(1, 16.0)
		fpu.FSQRT(0, 1)
		if fpu.getFReg(0) != 4.0 {
			t.Errorf("sqrt(16) = %v", fpu.getFReg(0))
		}
	})

	t.Run("FSIN", func(t *testing.T) {
		fpu.setFReg(1, 0)
		fpu.FSIN(0, 1)
		if fpu.getFReg(0) != 0 {
			t.Errorf("sin(0) = %v", fpu.getFReg(0))
		}
	})
}

func TestIE64FPU_Conversions(t *testing.T) {
	fpu := NewIE64FPU()

	t.Run("FCVTIF", func(t *testing.T) {
		val := int32(-42)
		fpu.FCVTIF(0, uint64(val))
		if fpu.getFReg(0) != -42.0 {
			t.Errorf("cvtif(-42) = %v", fpu.getFReg(0))
		}
	})

	t.Run("FCVTFI_Saturate", func(t *testing.T) {
		fpu.FPSR = 0
		fpu.setFReg(1, 1e20)
		res := fpu.FCVTFI(1)
		if res != math.MaxInt32 {
			t.Errorf("cvtfi(1e20) = %v, want MaxInt32", res)
		}
		if (fpu.FPSR & IE64_FPU_EX_IO) == 0 {
			t.Error("IO flag not set for saturation")
		}
	})
}

func TestIE64FPU_ROM(t *testing.T) {
	fpu := NewIE64FPU()

	t.Run("Pi", func(t *testing.T) {
		fpu.FMOVECR(0, 0)
		if fpu.getFReg(0) != float32(math.Pi) {
			t.Errorf("ROM Pi = %v, want %v", fpu.getFReg(0), float32(math.Pi))
		}
	})

	t.Run("OutOfRange", func(t *testing.T) {
		fpu.FMOVECR(0, 99)
		if fpu.getFReg(0) != 0.0 {
			t.Errorf("ROM[99] = %v, want 0.0", fpu.getFReg(0))
		}
		if (fpu.FPSR & IE64_FPU_CC_Z) == 0 {
			t.Error("Z flag not set for out-of-range ROM")
		}
	})
}

// ===========================================================================
// Helpers
// ===========================================================================

const fpEpsilon float32 = 1e-6

func approxEq32(a, b, epsilon float32) bool {
	if math.IsNaN(float64(a)) && math.IsNaN(float64(b)) {
		return true
	}
	if math.IsInf(float64(a), 0) || math.IsInf(float64(b), 0) {
		return a == b
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= epsilon
}

// ===========================================================================
// Extended Tests
// ===========================================================================

func TestIE64FPU_FNEG(t *testing.T) {
	fpu := NewIE64FPU()

	tests := []struct {
		input float32
		want  float32
		cc    uint32 // Expected CC bits set
	}{
		{5.5, -5.5, IE64_FPU_CC_N},
		{-5.5, 5.5, 0},
		{0.0, float32(math.Copysign(0, -1)), IE64_FPU_CC_Z}, // +0 -> -0
		{float32(math.Copysign(0, -1)), 0.0, IE64_FPU_CC_Z}, // -0 -> +0
		{float32(math.Inf(1)), float32(math.Inf(-1)), IE64_FPU_CC_N | IE64_FPU_CC_I},
	}

	for _, tt := range tests {
		fpu.setFReg(1, tt.input)
		fpu.FNEG(0, 1)
		got := fpu.getFReg(0)

		// Check value
		if tt.input == 0 {
			// Special check for signed zero
			if math.Float32bits(got) != math.Float32bits(tt.want) {
				t.Errorf("FNEG(%v) = %v (bits %08x), want %v (bits %08x)",
					tt.input, got, math.Float32bits(got), tt.want, math.Float32bits(tt.want))
			}
		} else {
			if got != tt.want {
				t.Errorf("FNEG(%v) = %v, want %v", tt.input, got, tt.want)
			}
		}

		// Check CC
		gotCC := fpu.FPSR & IE64_FPU_FPSR_MASK & 0x0F000000
		if gotCC != tt.cc {
			t.Errorf("FNEG(%v) CC = %08x, want %08x", tt.input, gotCC, tt.cc)
		}
	}

	// NaN case separately
	fpu.setFReg(1, float32(math.NaN()))
	fpu.FNEG(0, 1)
	if !math.IsNaN(float64(fpu.getFReg(0))) {
		t.Error("FNEG(NaN) should be NaN")
	}
	if (fpu.FPSR & IE64_FPU_CC_NAN) == 0 {
		t.Error("FNEG(NaN) should set NAN flag")
	}
}

func TestIE64FPU_Transcendentals(t *testing.T) {
	fpu := NewIE64FPU()

	tests := []struct {
		op   string
		in1  float32
		in2  float32
		want float32
	}{
		{"FCOS", 0, 0, 1.0},
		{"FCOS", float32(math.Pi), 0, -1.0},
		{"FTAN", 0, 0, 0.0},
		{"FTAN", float32(math.Pi) / 4, 0, 1.0},
		{"FATAN", 0, 0, 0.0},
		{"FATAN", 1, 0, float32(math.Pi) / 4},
		{"FLOG", 1, 0, 0.0},
		{"FLOG", float32(math.E), 0, 1.0},
		{"FEXP", 0, 0, 1.0},
		{"FEXP", 1, 0, float32(math.E)},
		{"FPOW", 2, 3, 8.0},
		{"FPOW", 4, 0.5, 2.0},
		{"FPOW", 5, 0, 1.0},
	}

	for _, tt := range tests {
		fpu.setFReg(1, tt.in1)
		fpu.setFReg(2, tt.in2) // For FPOW

		switch tt.op {
		case "FCOS":
			fpu.FCOS(0, 1)
		case "FTAN":
			fpu.FTAN(0, 1)
		case "FATAN":
			fpu.FATAN(0, 1)
		case "FLOG":
			fpu.FLOG(0, 1)
		case "FEXP":
			fpu.FEXP(0, 1)
		case "FPOW":
			fpu.FPOW(0, 1, 2)
		}

		got := fpu.getFReg(0)
		if !approxEq32(got, tt.want, fpEpsilon) {
			t.Errorf("%s(%v, %v) = %v, want %v", tt.op, tt.in1, tt.in2, got, tt.want)
		}
	}
}

func TestIE64FPU_Transcendentals_Exceptions(t *testing.T) {
	fpu := NewIE64FPU()

	// FLOG(0) -> -Inf, DZ
	fpu.FPSR = 0
	fpu.setFReg(1, 0)
	fpu.FLOG(0, 1)
	if !math.IsInf(float64(fpu.getFReg(0)), -1) {
		t.Errorf("FLOG(0) = %v, want -Inf", fpu.getFReg(0))
	}
	if (fpu.FPSR & IE64_FPU_EX_DZ) == 0 {
		t.Error("FLOG(0) should set DZ")
	}

	// FLOG(-1) -> NaN, IO
	fpu.FPSR = 0
	fpu.setFReg(1, -1)
	fpu.FLOG(0, 1)
	if !math.IsNaN(float64(fpu.getFReg(0))) {
		t.Errorf("FLOG(-1) = %v, want NaN", fpu.getFReg(0))
	}
	if (fpu.FPSR & IE64_FPU_EX_IO) == 0 {
		t.Error("FLOG(-1) should set IO")
	}

	// FEXP(89) -> +Inf, OE
	fpu.FPSR = 0
	fpu.setFReg(1, 89)
	fpu.FEXP(0, 1)
	if !math.IsInf(float64(fpu.getFReg(0)), 1) {
		t.Errorf("FEXP(89) = %v, want +Inf", fpu.getFReg(0))
	}
	if (fpu.FPSR & IE64_FPU_EX_OE) == 0 {
		t.Error("FEXP(89) should set OE")
	}
}

func TestIE64FPU_FINT(t *testing.T) {
	fpu := NewIE64FPU()

	tests := []struct {
		mode uint8
		in   float32
		want float32
	}{
		{IE64_FPU_RND_NEAREST, 2.5, 2.0}, // banker's
		{IE64_FPU_RND_NEAREST, 3.5, 4.0}, // banker's
		{IE64_FPU_RND_NEAREST, 2.7, 3.0},
		{IE64_FPU_RND_ZERO, 2.9, 2.0},
		{IE64_FPU_RND_ZERO, -2.9, -2.0},
		{IE64_FPU_RND_ZERO, 0.5, 0.0},
		{IE64_FPU_RND_FLOOR, 2.1, 2.0},
		{IE64_FPU_RND_FLOOR, -2.1, -3.0},
		{IE64_FPU_RND_FLOOR, -0.5, -1.0},
		{IE64_FPU_RND_CEIL, 2.1, 3.0},
		{IE64_FPU_RND_CEIL, -2.1, -2.0},
		{IE64_FPU_RND_CEIL, 0.1, 1.0},
	}

	for _, tt := range tests {
		fpu.SetRoundingMode(tt.mode)
		fpu.setFReg(1, tt.in)
		fpu.FINT(0, 1)
		if fpu.getFReg(0) != tt.want {
			t.Errorf("FINT(%v) mode=%d = %v, want %v", tt.in, tt.mode, fpu.getFReg(0), tt.want)
		}
	}
}

func TestIE64FPU_NaNPropagation(t *testing.T) {
	fpu := NewIE64FPU()
	nan := float32(math.NaN())

	ops := []struct {
		name string
		fn   func()
	}{
		{"FADD", func() { fpu.FADD(0, 1, 2) }},
		{"FSUB", func() { fpu.FSUB(0, 1, 2) }},
		{"FMUL", func() { fpu.FMUL(0, 1, 2) }},
		{"FDIV", func() { fpu.FDIV(0, 1, 2) }},
		{"FMOD", func() { fpu.FMOD(0, 1, 2) }},
		{"FPOW", func() { fpu.FPOW(0, 1, 2) }},
		{"FABS", func() { fpu.FABS(0, 1) }},
		{"FNEG", func() { fpu.FNEG(0, 1) }},
		{"FSQRT", func() { fpu.FSQRT(0, 1) }},
	}

	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			fpu.FPSR = 0
			fpu.setFReg(1, nan) // Input NaN
			fpu.setFReg(2, 1.0)
			op.fn()

			if !math.IsNaN(float64(fpu.getFReg(0))) {
				t.Error("Result should be NaN")
			}
			if (fpu.FPSR & IE64_FPU_CC_NAN) == 0 {
				t.Error("NAN flag should be set")
			}
			// IO exception should NOT be set for propagation
			if (fpu.FPSR & IE64_FPU_EX_IO) != 0 {
				t.Error("IO exception should NOT be set for NaN propagation")
			}
		})
	}
}

func TestIE64FPU_NegativeZero(t *testing.T) {
	fpu := NewIE64FPU()

	// FSUB(1.0, 1.0) -> +0.0 (default for RND_NEAREST)
	fpu.SetRoundingMode(IE64_FPU_RND_NEAREST)
	fpu.setFReg(1, 1.0)
	fpu.setFReg(2, 1.0)
	fpu.FSUB(0, 1, 2)
	if fpu.getFReg(0) != 0.0 || math.Signbit(float64(fpu.getFReg(0))) {
		t.Errorf("1.0 - 1.0 = %v, want +0.0", fpu.getFReg(0))
	}
	if (fpu.FPSR & IE64_FPU_CC_Z) == 0 {
		t.Error("Z flag not set for +0.0")
	}

	// FNEG(+0.0) -> -0.0
	fpu.setFReg(1, 0.0)
	fpu.FNEG(0, 1)
	if math.Float32bits(fpu.getFReg(0)) != 0x80000000 { // Check sign bit directly
		// Go's float32 comparison thinks -0 == +0, so use bits or copysign check
		if !math.Signbit(float64(fpu.getFReg(0))) {
			t.Error("FNEG(+0.0) should be -0.0")
		}
	}
	if (fpu.FPSR & IE64_FPU_CC_Z) == 0 {
		t.Error("Z flag not set for -0.0")
	}

	// FABS(-0.0) -> +0.0
	fpu.setFReg(1, float32(math.Copysign(0, -1)))
	fpu.FABS(0, 1)
	if math.Signbit(float64(fpu.getFReg(0))) {
		t.Error("FABS(-0.0) should be +0.0")
	}
	if (fpu.FPSR & IE64_FPU_CC_Z) == 0 {
		t.Error("Z flag not set for +0.0")
	}
}

func TestIE64FPU_FCMP_NaN(t *testing.T) {
	fpu := NewIE64FPU()
	nan := float32(math.NaN())

	cases := []struct {
		fs, ft float32
	}{
		{nan, 1.0},
		{1.0, nan},
		{nan, nan},
	}

	for _, c := range cases {
		fpu.FPSR = 0
		fpu.setFReg(1, c.fs)
		fpu.setFReg(2, c.ft)
		res := fpu.FCMP(1, 2)

		if res != 0 {
			t.Errorf("FCMP with NaN should return 0, got %d", res)
		}
		if (fpu.FPSR & IE64_FPU_CC_NAN) == 0 {
			t.Error("NAN flag should be set")
		}
		if (fpu.FPSR & IE64_FPU_EX_IO) == 0 {
			t.Error("IO exception should be set for FCMP with NaN")
		}
	}
}

func TestIE64FPU_FCVTFI_Extended(t *testing.T) {
	fpu := NewIE64FPU()

	// NaN -> 0, IO set
	fpu.FPSR = 0
	fpu.setFReg(1, float32(math.NaN()))
	res := fpu.FCVTFI(1)
	if res != 0 {
		t.Errorf("FCVTFI(NaN) = %d, want 0", res)
	}
	if (fpu.FPSR & IE64_FPU_EX_IO) == 0 {
		t.Error("IO not set for NaN")
	}
	if (fpu.FPSR & 0x0F000000) != 0 {
		t.Error("CC should not change") // FCVTFI does NOT set CC
	}

	// -1e20 -> MinInt32, IO set
	fpu.FPSR = 0
	fpu.setFReg(1, -1e20)
	res = fpu.FCVTFI(1)
	if res != math.MinInt32 {
		t.Errorf("FCVTFI(-Large) = %d, want MinInt32", res)
	}
	if (fpu.FPSR & IE64_FPU_EX_IO) == 0 {
		t.Error("IO not set for saturation")
	}

	// 42.7 -> 42 (truncate is default for cast in Go, but FINT uses rounding mode)
	// FCVTFI implementation uses int32(s) which is truncation.
	// Wait, Go spec says float to int conversion truncates fraction.
	// The implementation matches.
	fpu.FPSR = 0
	fpu.setFReg(1, 42.7)
	res = fpu.FCVTFI(1)
	if res != 42 {
		t.Errorf("FCVTFI(42.7) = %d, want 42", res)
	}
	if fpu.FPSR != 0 {
		t.Error("FPSR should remain 0")
	}
}

func TestIE64FPU_BitwiseTransfers(t *testing.T) {
	fpu := NewIE64FPU()

	// FMOVI: write bits of Pi
	piBits := uint32(0x40490FDB)
	fpu.FMOVI(0, uint64(piBits))
	if fpu.FPRegs[0] != piBits {
		t.Errorf("FMOVI failed: got %08x, want %08x", fpu.FPRegs[0], piBits)
	}
	if !approxEq32(fpu.getFReg(0), float32(math.Pi), fpEpsilon) {
		t.Error("FMOVI value mismatch")
	}

	// FMOVO: read bits of Pi
	fpu.setFReg(1, float32(math.Pi))
	bits := fpu.FMOVO(1)
	if uint32(bits) != piBits {
		t.Errorf("FMOVO failed: got %08x, want %08x", bits, piBits)
	}

	// FMOVCC: write CC bits
	fpu.FMOVCC(0x03) // Set rounding mode
	if fpu.FPCR != 0x03 {
		t.Error("FMOVCC failed")
	}
}

func TestIE64FPU_Arithmetic_Exceptions_Extended(t *testing.T) {
	fpu := NewIE64FPU()

	// FADD Overflow
	fpu.FPSR = 0
	fpu.setFReg(1, math.MaxFloat32)
	fpu.setFReg(2, math.MaxFloat32)
	fpu.FADD(0, 1, 2)
	if !math.IsInf(float64(fpu.getFReg(0)), 1) {
		t.Error("Overflow should result in +Inf")
	}
	if (fpu.FPSR & IE64_FPU_EX_OE) == 0 {
		t.Error("OE not set")
	}

	// FSUB Overflow
	fpu.FPSR = 0
	fpu.setFReg(1, math.MaxFloat32)
	fpu.setFReg(2, -math.MaxFloat32)
	fpu.FSUB(0, 1, 2)
	if !math.IsInf(float64(fpu.getFReg(0)), 1) {
		t.Error("Overflow should result in +Inf")
	}
	if (fpu.FPSR & IE64_FPU_EX_OE) == 0 {
		t.Error("OE not set")
	}

	// FDIV Underflow
	fpu.FPSR = 0
	fpu.setFReg(1, math.SmallestNonzeroFloat32)
	fpu.setFReg(2, 2.0)
	fpu.FDIV(0, 1, 2)
	if fpu.getFReg(0) != 0.0 {
		t.Error("Underflow should result in 0.0")
	}
	if (fpu.FPSR & IE64_FPU_EX_UE) == 0 {
		t.Error("UE not set")
	}

	// FMOD Invalid (x % 0)
	fpu.FPSR = 0
	fpu.setFReg(1, 10.0)
	fpu.setFReg(2, 0.0)
	fpu.FMOD(0, 1, 2)
	if !math.IsNaN(float64(fpu.getFReg(0))) {
		t.Error("FMOD by zero should result in NaN")
	}
	if (fpu.FPSR & IE64_FPU_EX_IO) == 0 {
		t.Error("IO not set for FMOD by zero")
	}
}

// ===========================================================================
// Benchmarks
// ===========================================================================

func BenchmarkIE64FPU_FADD(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, 1.5)
	fpu.setFReg(2, 2.5)
	for i := 0; i < b.N; i++ {
		fpu.FADD(0, 1, 2)
	}
}

func BenchmarkIE64FPU_FSUB(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, 1.5)
	fpu.setFReg(2, 2.5)
	for i := 0; i < b.N; i++ {
		fpu.FSUB(0, 1, 2)
	}
}

func BenchmarkIE64FPU_FMUL(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, 1.5)
	fpu.setFReg(2, 2.5)
	for i := 0; i < b.N; i++ {
		fpu.FMUL(0, 1, 2)
	}
}

func BenchmarkIE64FPU_FDIV(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, 1.5)
	fpu.setFReg(2, 2.5)
	for i := 0; i < b.N; i++ {
		fpu.FDIV(0, 1, 2)
	}
}

func BenchmarkIE64FPU_FMOD(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, 1.5)
	fpu.setFReg(2, 2.5)
	for i := 0; i < b.N; i++ {
		fpu.FMOD(0, 1, 2)
	}
}

func BenchmarkIE64FPU_FABS(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, -1.5)
	for i := 0; i < b.N; i++ {
		fpu.FABS(0, 1)
	}
}

func BenchmarkIE64FPU_FNEG(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, 1.5)
	for i := 0; i < b.N; i++ {
		fpu.FNEG(0, 1)
	}
}

func BenchmarkIE64FPU_FSQRT(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, 16.0)
	for i := 0; i < b.N; i++ {
		fpu.FSQRT(0, 1)
	}
}

func BenchmarkIE64FPU_SetConditionCodes(b *testing.B) {
	fpu := NewIE64FPU()
	for i := 0; i < b.N; i++ {
		fpu.setConditionCodes(1.0)
	}
}

func BenchmarkIE64FPU_SetConditionCodes_NaN(b *testing.B) {
	fpu := NewIE64FPU()
	val := float32(math.NaN())
	for i := 0; i < b.N; i++ {
		fpu.setConditionCodes(val)
	}
}

func BenchmarkIE64FPU_SetConditionCodes_Inf(b *testing.B) {
	fpu := NewIE64FPU()
	val := float32(math.Inf(1))
	for i := 0; i < b.N; i++ {
		fpu.setConditionCodes(val)
	}
}

func BenchmarkIE64FPU_FSIN(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, 1.0)
	for i := 0; i < b.N; i++ {
		fpu.FSIN(0, 1)
	}
}

func BenchmarkIE64FPU_FCOS(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, 1.0)
	for i := 0; i < b.N; i++ {
		fpu.FCOS(0, 1)
	}
}

func BenchmarkIE64FPU_FCMP(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, 1.5)
	fpu.setFReg(2, 2.5)
	for i := 0; i < b.N; i++ {
		fpu.FCMP(1, 2)
	}
}

func BenchmarkIE64FPU_FCVTIF(b *testing.B) {
	fpu := NewIE64FPU()
	for i := 0; i < b.N; i++ {
		fpu.FCVTIF(0, 42)
	}
}

func BenchmarkIE64FPU_FCVTFI(b *testing.B) {
	fpu := NewIE64FPU()
	fpu.setFReg(1, 42.5)
	for i := 0; i < b.N; i++ {
		fpu.FCVTFI(1)
	}
}
