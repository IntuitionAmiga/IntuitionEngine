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
	fpu.FPSR = 0
	if fpu.FPSR != 0 {
		t.Error("Failed to clear FPSR")
	}
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
