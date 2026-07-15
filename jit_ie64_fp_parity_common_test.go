//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

// jit_ie64_fp_parity_common_test.go - FP parity helpers and special-value
// programs shared by every JIT backend.
//
// These live in an architecture-neutral file on purpose. The FP32 special-value
// matrix originally sat in an amd64-only file, where it found that the amd64
// backend never updated FPSR for FP32 binary arithmetic. The same class of bug
// can exist in any backend, so the gate that catches it must compile for all of
// them.

package main

import "testing"

// FP32 bit patterns for the special-value matrix. Named rather than inlined so
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

// assertFPParity runs a builder under JIT and interpreter and asserts identical
// architectural state.
func assertFPParity(t *testing.T, name string, build func(mem []byte)) {
	t.Helper()
	jitCPU := runToHaltAt(t, true, build)
	interpCPU := runToHaltAt(t, false, build)
	if jitCPU.PC != interpCPU.PC {
		t.Fatalf("%s: PC mismatch: JIT 0x%X, interp 0x%X", name, jitCPU.PC, interpCPU.PC)
	}
	for i := range jitCPU.regs {
		if jitCPU.regs[i] != interpCPU.regs[i] {
			t.Fatalf("%s: R%d mismatch: JIT 0x%X, interp 0x%X", name, i, jitCPU.regs[i], interpCPU.regs[i])
		}
	}
	if jitCPU.FPU.FPSR != interpCPU.FPU.FPSR {
		t.Fatalf("%s: FPSR mismatch: JIT 0x%08X, interp 0x%08X", name, jitCPU.FPU.FPSR, interpCPU.FPU.FPSR)
	}
	for i := range jitCPU.FPU.FPRegs {
		if jitCPU.FPU.FPRegs[i] != interpCPU.FPU.FPRegs[i] {
			t.Fatalf("%s: F%d mismatch: JIT 0x%08X, interp 0x%08X", name, i, jitCPU.FPU.FPRegs[i], interpCPU.FPU.FPRegs[i])
		}
	}
}

// buildFP32ResidencyValueProgram builds a barrier-free FP32 block seeded with two
// arbitrary bit patterns and running every natively-emitted FP32 operation over
// them. Because it contains no residency barrier the block qualifies for FP
// register residency on backends that implement it, so JIT/interpreter parity
// here proves residency preserves special values (NaN payloads, signed zero,
// infinities, denormals) and their FPSR side effects across the prologue seed,
// in-register arithmetic and exit spill.
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

// fp32SpecialValueCases is the shared special-value matrix. Each pair drives the
// FP32 program above through a different IEEE-754 corner.
var fp32SpecialValueCases = []struct {
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
	// Equal infinities: FCMP reports CC_Z|CC_I, and CC_N as well for -Inf.
	{"neginf/neginf", fp32NegInf, fp32NegInf},
}

// buildFP32SqrtProgram runs FSQRT over a single seeded bit pattern. FSQRT is not
// covered by the binary-op matrix above (that program has no FSQRT), and it has
// its own exception rule: IE64FPU.FSQRT raises IO when the operand is negative,
// excluding -0.0 and NaN.
func buildFP32SqrtProgram(a uint32) func(mem []byte) {
	return func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, a))
		put(0x08, ie64Instr(OP_FMOVI, 1, 0, 0, 1, 0, 0)) // F1 = a
		put(0x10, ie64Instr(OP_FSQRT, 2, 0, 0, 1, 0, 0)) // F2 = sqrt(a)
		put(0x18, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
}

// TestJIT_vs_Interpreter_FP32Sqrt is the backend-neutral FSQRT gate. It asserts
// both the result bits and the full FPSR, so it catches a missing IO flag on a
// negative operand and any divergence in the NaN produced for one.
func TestJIT_vs_Interpreter_FP32Sqrt(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	cases := []struct {
		name string
		a    uint32
	}{
		{"negone", fp32NegOne},   // negative: IO, result NaN
		{"negtwo", 0xC0000000},   // negative: IO
		{"neginf", fp32NegInf},   // negative: IO, result NaN
		{"negzero", fp32NegZero}, // sign set but zero: no IO, result -0.0
		{"negdenormal", 0x80000001},
		{"poszero", fp32PosZero},
		{"one", fp32One},
		{"two", fp32Two},
		{"frac", fp32Frac},
		{"max", fp32Max},
		{"min", fp32Min},
		{"denormal", fp32Denormal},
		{"posinf", fp32PosInf},
		{"qnan", fp32QNaN}, // NaN: no IO
		{"snan", fp32SNaN}, // NaN: no IO
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertFPParity(t, tc.name, buildFP32SqrtProgram(tc.a))
		})
	}
}

// TestJIT_vs_Interpreter_FP32SpecialValues is the backend-neutral FP32
// correctness gate. It makes no reference to register residency: it asserts only
// that the JIT and the interpreter agree on results and on the full FPSR,
// including the sticky exception flags (invalid, divide-by-zero, overflow,
// underflow), for every IEEE-754 corner in the matrix.
//
// Any backend that emits FP32 arithmetic without updating FPSR fails here.
func TestJIT_vs_Interpreter_FP32SpecialValues(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	for _, tc := range fp32SpecialValueCases {
		t.Run(tc.name, func(t *testing.T) {
			assertFPParity(t, tc.name, buildFP32ResidencyValueProgram(tc.a, tc.b))
		})
	}
}
