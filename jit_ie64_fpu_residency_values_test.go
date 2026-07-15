//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
)

// FP32 bit patterns for the residency value matrix. Named rather than inlined so
// failures report which special value diverged.
const (
	fp32PosZero  uint32 = 0x00000000
	fp32NegZero  uint32 = 0x80000000
	fp32One      uint32 = 0x3F800000
	fp32NegOne   uint32 = 0xBF800000
	fp32Two      uint32 = 0x40000000
	fp32PosInf   uint32 = 0x7F800000
	fp32NegInf   uint32 = 0xFF800000
	fp32QNaN     uint32 = 0x7FC00000
	fp32SNaN     uint32 = 0x7F800001
	fp32Max      uint32 = 0x7F7FFFFF // FLT_MAX: MUL overflows to +Inf
	fp32Min      uint32 = 0x00800000 // smallest normal: MUL underflows to denormal
	fp32Denormal uint32 = 0x00000001 // smallest denormal
	fp32Frac     uint32 = 0x3FC00000 // 1.5: exercises FINT rounding
)

// buildFP32ResidencyValueProgram builds a barrier-free FP32 block seeded with two
// arbitrary bit patterns and running every natively-emitted FP32 operation over
// them. Because it contains no residency barrier the block qualifies for XMM8..15
// residency, so JIT/interpreter parity here proves residency preserves special
// values (NaN payloads, signed zero, infinities, denormals) and their FPSR side
// effects across the prologue seed, in-XMM arithmetic and exit spill.
//
// F1 = a, F2 = b are seeded via FMOVI (raw bit move from a GPR), which keeps the
// exact pattern including signalling NaNs that a decimal literal could not carry.
func buildFP32ResidencyValueProgram(a, b uint32) func(mem []byte) {
	return func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, a))
		put(0x08, ie64Instr(OP_FMOVI, 1, 0, 0, 1, 0, 0)) // F1 = a
		put(0x10, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, b))
		put(0x18, ie64Instr(OP_FMOVI, 2, 0, 0, 2, 0, 0)) // F2 = b

		put(0x20, ie64Instr(OP_FADD, 3, 0, 0, 1, 2, 0))     // F3 = a + b
		put(0x28, ie64Instr(OP_FSUB, 4, 0, 0, 1, 2, 0))     // F4 = a - b
		put(0x30, ie64Instr(OP_FMUL, 5, 0, 0, 1, 2, 0))     // F5 = a * b (overflow/underflow)
		put(0x38, ie64Instr(OP_FDIV, 6, 0, 0, 1, 2, 0))     // F6 = a / b (divide by zero)
		put(0x40, ie64Instr(OP_FABS, 7, 0, 0, 1, 0, 0))     // F7 = |a|
		put(0x48, ie64Instr(OP_FNEG, 8, 0, 0, 2, 0, 0))     // F8 = -b (signed zero)
		put(0x50, ie64Instr(OP_FINT, 9, 0, 0, 1, 0, 0))     // F9 = int(a)
		put(0x58, ie64Instr(OP_FCVTFI, 11, 0, 0, 1, 0, 0))  // R11 = int64(a)
		put(0x60, ie64Instr(OP_FCVTIF, 10, 0, 0, 11, 0, 0)) // F10 = float(R11)
		put(0x68, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
}

// TestJIT_vs_Interpreter_FPResidencyValueMatrix is the special-value gate for FP
// residency (Technique 3): every pair below is run under the JIT with residency
// enabled and under the interpreter, and the full architectural FP state (all
// sixteen FPRegs slots, FPSR sticky flags, GPRs) must match exactly.
func TestJIT_vs_Interpreter_FPResidencyValueMatrix(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	t.Setenv("IE64_JIT_FP_RESIDENCY", "1")
	if !ie64FPResidencyEnabled() {
		t.Skip("FP residency unavailable on this host ABI")
	}

	cases := []struct {
		name string
		a, b uint32
	}{
		{"qnan/one", fp32QNaN, fp32One},
		{"one/qnan", fp32One, fp32QNaN},
		{"snan/one", fp32SNaN, fp32One},
		{"posinf/neginf", fp32PosInf, fp32NegInf},
		{"posinf/posinf", fp32PosInf, fp32PosInf},
		{"posinf/zero", fp32PosInf, fp32PosZero},
		{"one/poszero", fp32One, fp32PosZero}, // divide by zero -> +Inf
		{"one/negzero", fp32One, fp32NegZero}, // divide by zero -> -Inf
		{"poszero/negzero", fp32PosZero, fp32NegZero},
		{"negzero/negzero", fp32NegZero, fp32NegZero},
		{"zero/zero", fp32PosZero, fp32PosZero}, // 0/0 -> NaN, invalid
		{"max/two", fp32Max, fp32Two},           // multiply overflow -> +Inf
		{"min/min", fp32Min, fp32Min},           // multiply underflow -> denormal/zero
		{"denormal/two", fp32Denormal, fp32Two},
		{"denormal/max", fp32Denormal, fp32Max},
		{"frac/negone", fp32Frac, fp32NegOne}, // FINT rounding, negative operand
		{"negone/frac", fp32NegOne, fp32Frac},
		{"posinf/qnan", fp32PosInf, fp32QNaN},
		{"neginf/two", fp32NegInf, fp32Two},
	}
	for _, tc := range cases {
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
