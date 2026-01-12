package main

import (
	"math"
	"testing"
)

// =============================================================================
// FPU Data Structure Tests
// =============================================================================

func TestFPURegisterInit(t *testing.T) {
	t.Run("FPU_registers_initialized_to_zero", func(t *testing.T) {
		fpu := NewM68881FPU()

		for i := 0; i < 8; i++ {
			if !fpu.FPRegs[i].IsZero() {
				t.Errorf("FP%d should be zero on init, got: %v", i, fpu.FPRegs[i])
			}
		}
	})

	t.Run("FPCR_default_values", func(t *testing.T) {
		fpu := NewM68881FPU()

		// Default rounding mode should be Round to Nearest (RN = 00)
		if fpu.GetRoundingMode() != FPU_RND_NEAREST {
			t.Errorf("Default rounding mode should be RN, got: %d", fpu.GetRoundingMode())
		}

		// Default precision should be Extended (00)
		if fpu.GetPrecision() != FPU_PREC_EXTENDED {
			t.Errorf("Default precision should be Extended, got: %d", fpu.GetPrecision())
		}
	})

	t.Run("FPSR_cleared_on_init", func(t *testing.T) {
		fpu := NewM68881FPU()

		if fpu.FPSR != 0 {
			t.Errorf("FPSR should be 0 on init, got: 0x%08X", fpu.FPSR)
		}
	})

	t.Run("FPIAR_cleared_on_init", func(t *testing.T) {
		fpu := NewM68881FPU()

		if fpu.FPIAR != 0 {
			t.Errorf("FPIAR should be 0 on init, got: 0x%08X", fpu.FPIAR)
		}
	})
}

// =============================================================================
// Extended Precision (80-bit) Tests
// =============================================================================

func TestExtendedRealFromFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		wantSign uint8
		wantZero bool
		wantInf  bool
		wantNaN  bool
	}{
		{"positive_zero", 0.0, 0, true, false, false},
		{"negative_zero", math.Copysign(0.0, -1.0), 1, true, false, false},
		{"positive_one", 1.0, 0, false, false, false},
		{"negative_one", -1.0, 1, false, false, false},
		{"positive_infinity", math.Inf(1), 0, false, true, false},
		{"negative_infinity", math.Inf(-1), 1, false, true, false},
		{"NaN", math.NaN(), 0, false, false, true},
		{"pi", math.Pi, 0, false, false, false},
		{"small_positive", 1e-300, 0, false, false, false},
		{"large_positive", 1e300, 0, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := ExtendedRealFromFloat64(tt.input)

			if ext.Sign != tt.wantSign {
				t.Errorf("Sign = %d, want %d", ext.Sign, tt.wantSign)
			}
			if ext.IsZero() != tt.wantZero {
				t.Errorf("IsZero() = %v, want %v", ext.IsZero(), tt.wantZero)
			}
			if ext.IsInf() != tt.wantInf {
				t.Errorf("IsInf() = %v, want %v", ext.IsInf(), tt.wantInf)
			}
			if ext.IsNaN() != tt.wantNaN {
				t.Errorf("IsNaN() = %v, want %v", ext.IsNaN(), tt.wantNaN)
			}
		})
	}
}

func TestExtendedRealToFloat64(t *testing.T) {
	tests := []struct {
		name    string
		input   float64
		epsilon float64
	}{
		{"zero", 0.0, 0.0},
		{"one", 1.0, 1e-15},
		{"negative_one", -1.0, 1e-15},
		{"pi", math.Pi, 1e-15},
		{"e", math.E, 1e-15},
		{"small", 1e-100, 1e-115},
		{"large", 1e100, 1e85},
		{"half", 0.5, 1e-15},
		{"third", 1.0 / 3.0, 1e-15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := ExtendedRealFromFloat64(tt.input)
			result := ext.ToFloat64()

			diff := math.Abs(result - tt.input)
			if diff > tt.epsilon {
				t.Errorf("ToFloat64() = %v, want %v (diff: %e)", result, tt.input, diff)
			}
		})
	}
}

func TestExtendedRealRoundTrip(t *testing.T) {
	// Test that float64 -> ExtendedReal -> float64 preserves value
	values := []float64{
		0.0, 1.0, -1.0, 0.5, -0.5,
		math.Pi, math.E, math.Sqrt2,
		1e-10, 1e10, -1e-10, -1e10,
		math.MaxFloat64 / 2, 1e-300, // Use 1e-300 instead of SmallestNonzeroFloat64
	}

	for _, v := range values {
		ext := ExtendedRealFromFloat64(v)
		result := ext.ToFloat64()

		if v == 0.0 {
			if result != 0.0 {
				t.Errorf("Round trip of %v gave %v", v, result)
			}
		} else {
			relErr := math.Abs((result - v) / v)
			if relErr > 1e-15 {
				t.Errorf("Round trip of %v gave %v (rel error: %e)", v, result, relErr)
			}
		}
	}
}

// =============================================================================
// FMOVE Tests - Data Movement
// =============================================================================

func TestFMOVE_RegisterToRegister(t *testing.T) {
	tests := []struct {
		name   string
		value  float64
		srcReg int
		dstReg int
	}{
		{"move_FP0_to_FP1", math.Pi, 0, 1},
		{"move_FP7_to_FP0", math.E, 7, 0},
		{"move_negative", -123.456, 2, 5},
		{"move_zero", 0.0, 3, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[tt.srcReg] = ExtendedRealFromFloat64(tt.value)

			fpu.FMOVE_RegToReg(tt.srcReg, tt.dstReg)

			result := fpu.FPRegs[tt.dstReg].ToFloat64()
			if result != tt.value {
				t.Errorf("FMOVE FP%d,FP%d = %v, want %v",
					tt.srcReg, tt.dstReg, result, tt.value)
			}
		})
	}
}

func TestFMOVE_ImmediateToRegister(t *testing.T) {
	tests := []struct {
		name   string
		value  float64
		dstReg int
	}{
		{"load_pi", math.Pi, 0},
		{"load_negative", -42.5, 3},
		{"load_zero", 0.0, 7},
		{"load_one", 1.0, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()

			fpu.FMOVE_ImmToReg(tt.value, tt.dstReg)

			result := fpu.FPRegs[tt.dstReg].ToFloat64()
			if math.Abs(result-tt.value) > 1e-15 {
				t.Errorf("FMOVE #%v,FP%d = %v", tt.value, tt.dstReg, result)
			}
		})
	}
}

// =============================================================================
// FADD Tests - Addition
// =============================================================================

func TestFADD_Basic(t *testing.T) {
	tests := []struct {
		name   string
		a      float64
		b      float64
		expect float64
	}{
		{"positive_plus_positive", 1.0, 2.0, 3.0},
		{"positive_plus_negative", 5.0, -3.0, 2.0},
		{"negative_plus_negative", -2.0, -3.0, -5.0},
		{"zero_plus_number", 0.0, 42.0, 42.0},
		{"number_plus_zero", 42.0, 0.0, 42.0},
		{"pi_plus_e", math.Pi, math.E, math.Pi + math.E},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.a)
			fpu.FPRegs[1] = ExtendedRealFromFloat64(tt.b)

			fpu.FADD(1, 0) // FP0 = FP0 + FP1

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > 1e-14 {
				t.Errorf("FADD: %v + %v = %v, want %v", tt.a, tt.b, result, tt.expect)
			}
		})
	}
}

func TestFADD_SpecialValues(t *testing.T) {
	t.Run("infinity_plus_finite", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(math.Inf(1))
		fpu.FPRegs[1] = ExtendedRealFromFloat64(42.0)

		fpu.FADD(1, 0)

		if !fpu.FPRegs[0].IsInf() || fpu.FPRegs[0].Sign != 0 {
			t.Error("Inf + finite should be +Inf")
		}
	})

	t.Run("infinity_plus_neg_infinity", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(math.Inf(1))
		fpu.FPRegs[1] = ExtendedRealFromFloat64(math.Inf(-1))

		fpu.FADD(1, 0)

		if !fpu.FPRegs[0].IsNaN() {
			t.Error("Inf + (-Inf) should be NaN")
		}
	})
}

func TestFADD_ConditionCodes(t *testing.T) {
	t.Run("result_negative_sets_N", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(-5.0)
		fpu.FPRegs[1] = ExtendedRealFromFloat64(2.0)

		fpu.FADD(1, 0)

		if !fpu.GetConditionN() {
			t.Error("N flag should be set for negative result")
		}
	})

	t.Run("result_zero_sets_Z", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(5.0)
		fpu.FPRegs[1] = ExtendedRealFromFloat64(-5.0)

		fpu.FADD(1, 0)

		if !fpu.GetConditionZ() {
			t.Error("Z flag should be set for zero result")
		}
	})
}

// =============================================================================
// FSUB Tests - Subtraction
// =============================================================================

func TestFSUB_Basic(t *testing.T) {
	tests := []struct {
		name   string
		a      float64
		b      float64
		expect float64
	}{
		{"positive_minus_positive", 5.0, 3.0, 2.0},
		{"positive_minus_negative", 5.0, -3.0, 8.0},
		{"negative_minus_negative", -2.0, -3.0, 1.0},
		{"zero_minus_number", 0.0, 42.0, -42.0},
		{"number_minus_zero", 42.0, 0.0, 42.0},
		{"same_values", 3.14159, 3.14159, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.a)
			fpu.FPRegs[1] = ExtendedRealFromFloat64(tt.b)

			fpu.FSUB(1, 0) // FP0 = FP0 - FP1

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > 1e-14 {
				t.Errorf("FSUB: %v - %v = %v, want %v", tt.a, tt.b, result, tt.expect)
			}
		})
	}
}

// =============================================================================
// FMUL Tests - Multiplication
// =============================================================================

func TestFMUL_Basic(t *testing.T) {
	tests := []struct {
		name   string
		a      float64
		b      float64
		expect float64
	}{
		{"positive_times_positive", 3.0, 4.0, 12.0},
		{"positive_times_negative", 3.0, -4.0, -12.0},
		{"negative_times_negative", -3.0, -4.0, 12.0},
		{"multiply_by_zero", 42.0, 0.0, 0.0},
		{"multiply_by_one", 42.0, 1.0, 42.0},
		{"multiply_by_neg_one", 42.0, -1.0, -42.0},
		{"pi_times_two", math.Pi, 2.0, math.Pi * 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.a)
			fpu.FPRegs[1] = ExtendedRealFromFloat64(tt.b)

			fpu.FMUL(1, 0) // FP0 = FP0 * FP1

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > 1e-14 {
				t.Errorf("FMUL: %v * %v = %v, want %v", tt.a, tt.b, result, tt.expect)
			}
		})
	}
}

func TestFMUL_SpecialValues(t *testing.T) {
	t.Run("infinity_times_finite", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(math.Inf(1))
		fpu.FPRegs[1] = ExtendedRealFromFloat64(2.0)

		fpu.FMUL(1, 0)

		if !fpu.FPRegs[0].IsInf() {
			t.Error("Inf * positive should be Inf")
		}
	})

	t.Run("infinity_times_zero", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(math.Inf(1))
		fpu.FPRegs[1] = ExtendedRealFromFloat64(0.0)

		fpu.FMUL(1, 0)

		if !fpu.FPRegs[0].IsNaN() {
			t.Error("Inf * 0 should be NaN")
		}
	})
}

// =============================================================================
// FDIV Tests - Division
// =============================================================================

func TestFDIV_Basic(t *testing.T) {
	tests := []struct {
		name   string
		a      float64
		b      float64
		expect float64
	}{
		{"divide_12_by_4", 12.0, 4.0, 3.0},
		{"divide_positive_by_negative", 12.0, -4.0, -3.0},
		{"divide_negative_by_negative", -12.0, -4.0, 3.0},
		{"divide_by_one", 42.0, 1.0, 42.0},
		{"divide_zero_by_number", 0.0, 42.0, 0.0},
		{"divide_pi_by_two", math.Pi, 2.0, math.Pi / 2.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.a)
			fpu.FPRegs[1] = ExtendedRealFromFloat64(tt.b)

			fpu.FDIV(1, 0) // FP0 = FP0 / FP1

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > 1e-14 {
				t.Errorf("FDIV: %v / %v = %v, want %v", tt.a, tt.b, result, tt.expect)
			}
		})
	}
}

func TestFDIV_SpecialValues(t *testing.T) {
	t.Run("divide_by_zero_positive", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(1.0)
		fpu.FPRegs[1] = ExtendedRealFromFloat64(0.0)

		fpu.FDIV(1, 0)

		if !fpu.FPRegs[0].IsInf() || fpu.FPRegs[0].Sign != 0 {
			t.Error("1 / 0 should be +Inf")
		}
	})

	t.Run("divide_by_zero_negative", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(-1.0)
		fpu.FPRegs[1] = ExtendedRealFromFloat64(0.0)

		fpu.FDIV(1, 0)

		if !fpu.FPRegs[0].IsInf() || fpu.FPRegs[0].Sign != 1 {
			t.Error("-1 / 0 should be -Inf")
		}
	})

	t.Run("zero_divide_by_zero", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(0.0)
		fpu.FPRegs[1] = ExtendedRealFromFloat64(0.0)

		fpu.FDIV(1, 0)

		if !fpu.FPRegs[0].IsNaN() {
			t.Error("0 / 0 should be NaN")
		}
	})
}

// =============================================================================
// FNEG Tests - Negate
// =============================================================================

func TestFNEG(t *testing.T) {
	tests := []struct {
		name   string
		value  float64
		expect float64
	}{
		{"negate_positive", 42.0, -42.0},
		{"negate_negative", -42.0, 42.0},
		{"negate_zero", 0.0, 0.0}, // Note: -0.0 == 0.0 in float comparison
		{"negate_pi", math.Pi, -math.Pi},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FNEG(0, 0) // FP0 = -FP0

			result := fpu.FPRegs[0].ToFloat64()
			if result != tt.expect {
				t.Errorf("FNEG: -%v = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

// =============================================================================
// FABS Tests - Absolute Value
// =============================================================================

func TestFABS(t *testing.T) {
	tests := []struct {
		name   string
		value  float64
		expect float64
	}{
		{"abs_positive", 42.0, 42.0},
		{"abs_negative", -42.0, 42.0},
		{"abs_zero", 0.0, 0.0},
		{"abs_neg_pi", -math.Pi, math.Pi},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FABS(0, 0) // FP0 = |FP0|

			result := fpu.FPRegs[0].ToFloat64()
			if result != tt.expect {
				t.Errorf("FABS: |%v| = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

// =============================================================================
// FCMP Tests - Compare
// =============================================================================

func TestFCMP(t *testing.T) {
	t.Run("equal_values", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(42.0)
		fpu.FPRegs[1] = ExtendedRealFromFloat64(42.0)

		fpu.FCMP(1, 0) // Compare FP0 with FP1

		if !fpu.GetConditionZ() {
			t.Error("Z flag should be set for equal values")
		}
		if fpu.GetConditionN() {
			t.Error("N flag should not be set for equal values")
		}
	})

	t.Run("first_greater", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(100.0)
		fpu.FPRegs[1] = ExtendedRealFromFloat64(42.0)

		fpu.FCMP(1, 0)

		if fpu.GetConditionZ() {
			t.Error("Z flag should not be set when a > b")
		}
		if fpu.GetConditionN() {
			t.Error("N flag should not be set when a > b")
		}
	})

	t.Run("first_less", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(10.0)
		fpu.FPRegs[1] = ExtendedRealFromFloat64(42.0)

		fpu.FCMP(1, 0)

		if fpu.GetConditionZ() {
			t.Error("Z flag should not be set when a < b")
		}
		if !fpu.GetConditionN() {
			t.Error("N flag should be set when a < b")
		}
	})
}

// =============================================================================
// FTST Tests - Test
// =============================================================================

func TestFTST(t *testing.T) {
	t.Run("test_positive", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(42.0)

		fpu.FTST(0)

		if fpu.GetConditionN() {
			t.Error("N should not be set for positive value")
		}
		if fpu.GetConditionZ() {
			t.Error("Z should not be set for non-zero value")
		}
	})

	t.Run("test_negative", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(-42.0)

		fpu.FTST(0)

		if !fpu.GetConditionN() {
			t.Error("N should be set for negative value")
		}
		if fpu.GetConditionZ() {
			t.Error("Z should not be set for non-zero value")
		}
	})

	t.Run("test_zero", func(t *testing.T) {
		fpu := NewM68881FPU()
		fpu.FPRegs[0] = ExtendedRealFromFloat64(0.0)

		fpu.FTST(0)

		if fpu.GetConditionN() {
			t.Error("N should not be set for zero")
		}
		if !fpu.GetConditionZ() {
			t.Error("Z should be set for zero")
		}
	})
}

// =============================================================================
// FSQRT Tests - Square Root
// =============================================================================

func TestFSQRT(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		expect  float64
		epsilon float64
	}{
		{"sqrt_4", 4.0, 2.0, 1e-15},
		{"sqrt_2", 2.0, math.Sqrt2, 1e-15},
		{"sqrt_9", 9.0, 3.0, 1e-15},
		{"sqrt_100", 100.0, 10.0, 1e-14},
		{"sqrt_0", 0.0, 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FSQRT(0, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > tt.epsilon {
				t.Errorf("FSQRT(%v) = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

func TestFSQRT_Negative(t *testing.T) {
	fpu := NewM68881FPU()
	fpu.FPRegs[0] = ExtendedRealFromFloat64(-1.0)

	fpu.FSQRT(0, 0)

	if !fpu.FPRegs[0].IsNaN() {
		t.Error("FSQRT of negative should be NaN")
	}
}

// =============================================================================
// Transcendental Function Tests
// =============================================================================

func TestFSIN(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		expect  float64
		epsilon float64
	}{
		{"sin_0", 0.0, 0.0, 1e-15},
		{"sin_pi/2", math.Pi / 2, 1.0, 1e-15},
		{"sin_pi", math.Pi, 0.0, 1e-15},
		{"sin_3pi/2", 3 * math.Pi / 2, -1.0, 1e-15},
		{"sin_pi/6", math.Pi / 6, 0.5, 1e-15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FSIN(0, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > tt.epsilon {
				t.Errorf("FSIN(%v) = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

func TestFCOS(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		expect  float64
		epsilon float64
	}{
		{"cos_0", 0.0, 1.0, 1e-15},
		{"cos_pi/2", math.Pi / 2, 0.0, 1e-15},
		{"cos_pi", math.Pi, -1.0, 1e-15},
		{"cos_2pi", 2 * math.Pi, 1.0, 1e-14},
		{"cos_pi/3", math.Pi / 3, 0.5, 1e-15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FCOS(0, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > tt.epsilon {
				t.Errorf("FCOS(%v) = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

func TestFTAN(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		expect  float64
		epsilon float64
	}{
		{"tan_0", 0.0, 0.0, 1e-15},
		{"tan_pi/4", math.Pi / 4, 1.0, 1e-14},
		{"tan_pi", math.Pi, 0.0, 1e-14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FTAN(0, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > tt.epsilon {
				t.Errorf("FTAN(%v) = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

func TestFLOG10(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		expect  float64
		epsilon float64
	}{
		{"log10_1", 1.0, 0.0, 1e-15},
		{"log10_10", 10.0, 1.0, 1e-15},
		{"log10_100", 100.0, 2.0, 1e-14},
		{"log10_0.1", 0.1, -1.0, 1e-14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FLOG10(0, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > tt.epsilon {
				t.Errorf("FLOG10(%v) = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

func TestFLOGN(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		expect  float64
		epsilon float64
	}{
		{"ln_1", 1.0, 0.0, 1e-15},
		{"ln_e", math.E, 1.0, 1e-15},
		{"ln_e^2", math.E * math.E, 2.0, 1e-14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FLOGN(0, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > tt.epsilon {
				t.Errorf("FLOGN(%v) = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

func TestFETOX(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		expect  float64
		epsilon float64
	}{
		{"e^0", 0.0, 1.0, 1e-15},
		{"e^1", 1.0, math.E, 1e-15},
		{"e^2", 2.0, math.E * math.E, 1e-14},
		{"e^-1", -1.0, 1.0 / math.E, 1e-15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FETOX(0, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > tt.epsilon {
				t.Errorf("FETOX(%v) = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

func TestFTWOTOX(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		expect  float64
		epsilon float64
	}{
		{"2^0", 0.0, 1.0, 1e-15},
		{"2^1", 1.0, 2.0, 1e-15},
		{"2^10", 10.0, 1024.0, 1e-12},
		{"2^-1", -1.0, 0.5, 1e-15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FTWOTOX(0, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > tt.epsilon {
				t.Errorf("FTWOTOX(%v) = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

func TestFTENTOX(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		expect  float64
		epsilon float64
	}{
		{"10^0", 0.0, 1.0, 1e-15},
		{"10^1", 1.0, 10.0, 1e-14},
		{"10^2", 2.0, 100.0, 1e-13},
		{"10^-1", -1.0, 0.1, 1e-15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FTENTOX(0, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > tt.epsilon {
				t.Errorf("FTENTOX(%v) = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

// =============================================================================
// FINT / FINTRZ Tests - Integer Part
// =============================================================================

func TestFINT(t *testing.T) {
	tests := []struct {
		name   string
		value  float64
		expect float64
	}{
		{"int_positive", 3.7, 4.0}, // Round to nearest
		{"int_negative", -3.7, -4.0},
		{"int_exact", 5.0, 5.0},
		{"int_half_up", 2.5, 2.0}, // Round to even (banker's rounding)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FINT(0, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if result != tt.expect {
				t.Errorf("FINT(%v) = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

func TestFINTRZ(t *testing.T) {
	tests := []struct {
		name   string
		value  float64
		expect float64
	}{
		{"intrz_positive", 3.7, 3.0}, // Truncate toward zero
		{"intrz_negative", -3.7, -3.0},
		{"intrz_exact", 5.0, 5.0},
		{"intrz_small", 0.9, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()
			fpu.FPRegs[0] = ExtendedRealFromFloat64(tt.value)

			fpu.FINTRZ(0, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if result != tt.expect {
				t.Errorf("FINTRZ(%v) = %v, want %v", tt.value, result, tt.expect)
			}
		})
	}
}

// =============================================================================
// FPCR/FPSR Manipulation Tests
// =============================================================================

func TestFPCR_RoundingModes(t *testing.T) {
	fpu := NewM68881FPU()

	// Test setting different rounding modes
	roundingModes := []struct {
		mode uint8
		name string
	}{
		{FPU_RND_NEAREST, "Round to Nearest"},
		{FPU_RND_ZERO, "Round toward Zero"},
		{FPU_RND_MINUS_INF, "Round toward -Inf"},
		{FPU_RND_PLUS_INF, "Round toward +Inf"},
	}

	for _, rm := range roundingModes {
		t.Run(rm.name, func(t *testing.T) {
			fpu.SetRoundingMode(rm.mode)
			if fpu.GetRoundingMode() != rm.mode {
				t.Errorf("Rounding mode = %d, want %d", fpu.GetRoundingMode(), rm.mode)
			}
		})
	}
}

func TestFPCR_Precision(t *testing.T) {
	fpu := NewM68881FPU()

	precisions := []struct {
		prec uint8
		name string
	}{
		{FPU_PREC_EXTENDED, "Extended (80-bit)"},
		{FPU_PREC_SINGLE, "Single (32-bit)"},
		{FPU_PREC_DOUBLE, "Double (64-bit)"},
	}

	for _, p := range precisions {
		t.Run(p.name, func(t *testing.T) {
			fpu.SetPrecision(p.prec)
			if fpu.GetPrecision() != p.prec {
				t.Errorf("Precision = %d, want %d", fpu.GetPrecision(), p.prec)
			}
		})
	}
}

// =============================================================================
// FPU Constants Tests
// =============================================================================

func TestFMOVECR(t *testing.T) {
	tests := []struct {
		name    string
		romAddr uint8
		expect  float64
		epsilon float64
	}{
		{"pi", 0x00, math.Pi, 1e-15},
		{"log10(2)", 0x0B, math.Log10(2), 1e-15},
		{"e", 0x0C, math.E, 1e-15},
		{"log2(e)", 0x0D, math.Log2E, 1e-15},
		{"log10(e)", 0x0E, math.Log10E, 1e-15},
		{"zero", 0x0F, 0.0, 0.0},
		{"ln(2)", 0x30, math.Ln2, 1e-15},
		{"ln(10)", 0x31, math.Ln10, 1e-15},
		{"10^0 (1.0)", 0x32, 1.0, 1e-15},
		{"10^1", 0x33, 10.0, 1e-14},
		{"10^2", 0x34, 100.0, 1e-13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fpu := NewM68881FPU()

			fpu.FMOVECR(tt.romAddr, 0)

			result := fpu.FPRegs[0].ToFloat64()
			if math.Abs(result-tt.expect) > tt.epsilon {
				t.Errorf("FMOVECR(0x%02X) = %v, want %v", tt.romAddr, result, tt.expect)
			}
		})
	}
}

// =============================================================================
// FPU Benchmarks
// =============================================================================

func BenchmarkFADD(b *testing.B) {
	fpu := NewM68881FPU()
	fpu.FPRegs[0] = ExtendedRealFromFloat64(math.Pi)
	fpu.FPRegs[1] = ExtendedRealFromFloat64(math.E)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fpu.FADD(1, 0)
	}
}

func BenchmarkFMUL(b *testing.B) {
	fpu := NewM68881FPU()
	fpu.FPRegs[0] = ExtendedRealFromFloat64(math.Pi)
	fpu.FPRegs[1] = ExtendedRealFromFloat64(math.E)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fpu.FMUL(1, 0)
	}
}

func BenchmarkFDIV(b *testing.B) {
	fpu := NewM68881FPU()
	fpu.FPRegs[0] = ExtendedRealFromFloat64(math.Pi)
	fpu.FPRegs[1] = ExtendedRealFromFloat64(math.E)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fpu.FDIV(1, 0)
	}
}

func BenchmarkFSQRT(b *testing.B) {
	fpu := NewM68881FPU()
	fpu.FPRegs[0] = ExtendedRealFromFloat64(2.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fpu.FSQRT(0, 0)
	}
}

func BenchmarkFSIN(b *testing.B) {
	fpu := NewM68881FPU()
	fpu.FPRegs[0] = ExtendedRealFromFloat64(math.Pi / 4)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fpu.FSIN(0, 0)
	}
}

func BenchmarkExtendedRealConversion(b *testing.B) {
	b.Run("Float64ToExtended", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ExtendedRealFromFloat64(math.Pi)
		}
	})

	b.Run("ExtendedToFloat64", func(b *testing.B) {
		ext := ExtendedRealFromFloat64(math.Pi)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = ext.ToFloat64()
		}
	})
}
