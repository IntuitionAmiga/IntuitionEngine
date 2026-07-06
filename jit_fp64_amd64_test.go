//go:build amd64 && linux

package main

import (
	"math"
	"testing"
)

func assertNativeFP64NoIOFallback(t *testing.T, r *jitTestRig) {
	t.Helper()
	if r.ctx.NeedIOFallback != 0 {
		t.Fatalf("NeedIOFallback = %d, want 0 (FP64 op should run natively)", r.ctx.NeedIOFallback)
	}
	if r.ctx.NeedHelper != HELPER_NONE {
		t.Fatalf("NeedHelper = %d, want HELPER_NONE", r.ctx.NeedHelper)
	}
}

func TestJIT_AMD64_FP64ArithmeticRunsNatively(t *testing.T) {
	tests := []struct {
		name   string
		opcode byte
		a      float64
		b      float64
		want   float64
	}{
		{"dadd", OP_DADD, 10.25, 3.5, 13.75},
		{"dsub", OP_DSUB, 10.25, 3.5, 6.75},
		{"dmul", OP_DMUL, -6.0, 2.5, -15.0},
		{"ddiv", OP_DDIV, 22.5, 4.5, 5.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newJITTestRig(t)
			r.cpu.FPU.setDPair(2, tc.a)
			r.cpu.FPU.setDPair(4, tc.b)

			r.compileAndRun(t, ie64Instr(tc.opcode, 6, 0, 0, 2, 4, 0))

			assertNativeFP64NoIOFallback(t, r)
			if got := r.cpu.FPU.getDPair(6); got != tc.want {
				t.Fatalf("DPair(6) = %.17g, want %.17g", got, tc.want)
			}
		})
	}
}

func TestJIT_AMD64_FP64ArithmeticPreservesExceptionFlags(t *testing.T) {
	tests := []struct {
		name     string
		opcode   byte
		a        float64
		b        float64
		wantFlag uint32
	}{
		{"dadd_overflow", OP_DADD, math.MaxFloat64, math.MaxFloat64, IE64_FPU_EX_OE},
		{"dsub_overflow", OP_DSUB, -math.MaxFloat64, math.MaxFloat64, IE64_FPU_EX_OE},
		{"dmul_underflow", OP_DMUL, math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64, IE64_FPU_EX_UE},
		{"ddiv_divide_by_zero", OP_DDIV, 1, 0, IE64_FPU_EX_DZ},
		{"ddiv_invalid", OP_DDIV, 0, 0, IE64_FPU_EX_IO},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newJITTestRig(t)
			r.cpu.FPU.setDPair(2, tc.a)
			r.cpu.FPU.setDPair(4, tc.b)

			r.compileAndRun(t, ie64Instr(tc.opcode, 6, 0, 0, 2, 4, 0))

			assertNativeFP64NoIOFallback(t, r)
			if got := r.cpu.FPU.FPSR & tc.wantFlag; got == 0 {
				t.Fatalf("FPSR = 0x%08X, want flag 0x%08X", r.cpu.FPU.FPSR, tc.wantFlag)
			}
		})
	}
}

func TestJIT_AMD64_DINTRunsNatively(t *testing.T) {
	tests := []struct {
		name string
		mode uint8
		in   float64
		want float64
	}{
		{"nearest_even", IE64_FPU_RND_NEAREST, 2.5, 2.0},
		{"zero", IE64_FPU_RND_ZERO, -2.9, -2.0},
		{"floor", IE64_FPU_RND_FLOOR, -2.1, -3.0},
		{"ceil", IE64_FPU_RND_CEIL, -2.9, -2.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newJITTestRig(t)
			r.cpu.FPU.SetRoundingMode(tc.mode)
			r.cpu.FPU.setDPair(2, tc.in)

			r.compileAndRun(t, ie64Instr(OP_DINT, 4, 0, 0, 2, 0, 0))

			assertNativeFP64NoIOFallback(t, r)
			if got := r.cpu.FPU.getDPair(4); got != tc.want {
				t.Fatalf("DPair(4) = %.17g, want %.17g", got, tc.want)
			}
		})
	}
}

func TestJIT_AMD64_DCMPAndConversionsRunNatively(t *testing.T) {
	t.Run("dcmp_less", func(t *testing.T) {
		r := newJITTestRig(t)
		r.cpu.FPU.setDPair(2, -1.25)
		r.cpu.FPU.setDPair(4, 3.0)

		r.compileAndRun(t, ie64Instr(OP_DCMP, 5, 0, 0, 2, 4, 0))

		assertNativeFP64NoIOFallback(t, r)
		if got := int64(r.cpu.regs[5]); got != -1 {
			t.Fatalf("R5 = %d, want -1", got)
		}
		if got := r.cpu.FPU.FPSR & 0x0F000000; got != IE64_FPU_CC_N {
			t.Fatalf("FPSR CC = %#x, want %#x", got, IE64_FPU_CC_N)
		}
	})

	t.Run("dcvtif", func(t *testing.T) {
		r := newJITTestRig(t)
		r.cpu.regs[3] = negU64(-42)

		r.compileAndRun(t, ie64Instr(OP_DCVTIF, 2, 0, 0, 3, 0, 0))

		assertNativeFP64NoIOFallback(t, r)
		if got := r.cpu.FPU.getDPair(2); got != -42.0 {
			t.Fatalf("DPair(2) = %.17g, want -42", got)
		}
	})

	t.Run("dcvtfi", func(t *testing.T) {
		r := newJITTestRig(t)
		r.cpu.FPU.setDPair(2, 123.75)

		r.compileAndRun(t, ie64Instr(OP_DCVTFI, 6, 0, 0, 2, 0, 0))

		assertNativeFP64NoIOFallback(t, r)
		if got := int64(r.cpu.regs[6]); got != 123 {
			t.Fatalf("R6 = %d, want 123", got)
		}
	})

	t.Run("dcvtfi_saturates_high", func(t *testing.T) {
		r := newJITTestRig(t)
		r.cpu.FPU.setDPair(2, 1e300)

		r.compileAndRun(t, ie64Instr(OP_DCVTFI, 6, 0, 0, 2, 0, 0))

		assertNativeFP64NoIOFallback(t, r)
		if got := int64(r.cpu.regs[6]); got != math.MaxInt64 {
			t.Fatalf("R6 = %d, want MaxInt64", got)
		}
		if got := r.cpu.FPU.FPSR & IE64_FPU_EX_IO; got == 0 {
			t.Fatalf("FPSR = 0x%08X, want invalid-operation flag", r.cpu.FPU.FPSR)
		}
	})

	t.Run("dcvtfi_saturates_low", func(t *testing.T) {
		r := newJITTestRig(t)
		r.cpu.FPU.setDPair(2, -1e300)

		r.compileAndRun(t, ie64Instr(OP_DCVTFI, 6, 0, 0, 2, 0, 0))

		assertNativeFP64NoIOFallback(t, r)
		if got := int64(r.cpu.regs[6]); got != math.MinInt64 {
			t.Fatalf("R6 = %d, want MinInt64", got)
		}
		if got := r.cpu.FPU.FPSR & IE64_FPU_EX_IO; got == 0 {
			t.Fatalf("FPSR = 0x%08X, want invalid-operation flag", r.cpu.FPU.FPSR)
		}
	})

	t.Run("dcmp_nan_stays_helper", func(t *testing.T) {
		r := newJITTestRig(t)
		r.cpu.FPU.setDPair(2, math.NaN())
		r.cpu.FPU.setDPair(4, 1)

		r.compileAndRun(t, ie64Instr(OP_DCMP, 5, 0, 0, 2, 4, 0))

		if r.ctx.NeedIOFallback == 0 {
			t.Fatal("NeedIOFallback = 0, want conservative interpreter fallback for NaN compare")
		}
	})
}
