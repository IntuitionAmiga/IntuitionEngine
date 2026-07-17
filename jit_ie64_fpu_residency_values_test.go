//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
)

// The FP32 special-value constants, the program builder and the shared case
// matrix live in jit_ie64_fp_parity_common_test.go: the plain JIT-versus-
// interpreter gate over them applies to every backend, so it must compile for
// every backend. This file keeps only the residency-specific tests, which
// depend on amd64-only helpers.

func TestIE64FPResidencyEnabled_DefaultAndKillSwitch(t *testing.T) {
	if !ie64FPResidencySysV() {
		t.Skip("FP residency unavailable on this host ABI")
	}
	t.Setenv("IE64_JIT_FP_RESIDENCY", "")
	if !ie64FPResidencyEnabled() {
		t.Fatal("IE64_JIT_FP_RESIDENCY should default on")
	}
	t.Setenv("IE64_JIT_FP_RESIDENCY", "0")
	if ie64FPResidencyEnabled() {
		t.Fatal("IE64_JIT_FP_RESIDENCY=0 should disable FP residency")
	}
}

// TestJIT_vs_Interpreter_FPResidencyValueMatrix is the special-value gate for FP
// residency (Technique 3): every pair in the shared matrix is run under the JIT
// with residency enabled and under the interpreter, and the full architectural
// FP state (all sixteen FPRegs slots, FPSR sticky flags, GPRs) must match
// exactly.
func TestJIT_vs_Interpreter_FPResidencyValueMatrix(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	t.Setenv("IE64_JIT_FP_RESIDENCY", "1")
	if !ie64FPResidencyEnabled() {
		t.Skip("FP residency unavailable on this host ABI")
	}

	for _, tc := range fp32SpecialValueCases {
		t.Run(tc.name, func(t *testing.T) {
			assertFPParity(t, tc.name, buildFP32ResidencyValueProgram(tc.a, tc.b))
		})
	}
}

// TestFPResidencyValueMatrix_MatchesMemoryPath cross-checks the same special
// values against the default memory-backed FP path: enabling residency must not
// change any result. This isolates residency from any pre-existing JIT/interpreter
// FP divergence, which would show up in the test above but not here.
func TestFPResidencyValueMatrix_MatchesMemoryPath(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	if !ie64FPResidencySysV() {
		t.Skip("FP residency unavailable on this host ABI")
	}
	cases := []struct {
		name string
		a, b uint32
	}{
		{"qnan/one", fp32QNaN, fp32One},
		{"snan/one", fp32SNaN, fp32One},
		{"posinf/neginf", fp32PosInf, fp32NegInf},
		{"one/negzero", fp32One, fp32NegZero},
		{"zero/zero", fp32PosZero, fp32PosZero},
		{"max/two", fp32Max, fp32Two},
		{"min/min", fp32Min, fp32Min},
		{"denormal/two", fp32Denormal, fp32Two},
		{"frac/negone", fp32Frac, fp32NegOne},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			build := buildFP32ResidencyValueProgram(tc.a, tc.b)
			t.Setenv("IE64_JIT_FP_RESIDENCY", "0")
			off := runToHaltAt(t, true, build)
			t.Setenv("IE64_JIT_FP_RESIDENCY", "1")
			on := runToHaltAt(t, true, build)
			for i := range off.FPU.FPRegs {
				if off.FPU.FPRegs[i] != on.FPU.FPRegs[i] {
					t.Fatalf("F%d differs with residency: off 0x%08X, on 0x%08X", i, off.FPU.FPRegs[i], on.FPU.FPRegs[i])
				}
			}
			if off.FPU.FPSR != on.FPU.FPSR {
				t.Fatalf("FPSR differs with residency: off 0x%08X, on 0x%08X", off.FPU.FPSR, on.FPU.FPSR)
			}
			for i := range off.regs {
				if off.regs[i] != on.regs[i] {
					t.Fatalf("R%d differs with residency: off 0x%X, on 0x%X", i, off.regs[i], on.regs[i])
				}
			}
		})
	}
}
