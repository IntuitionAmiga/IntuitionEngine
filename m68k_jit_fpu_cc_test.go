package main

import (
	"math"
	"testing"
)

// Slice 4 of native M68K FPU JIT: the condition-code computation that native
// code must replicate after each arithmetic op. m68kFPUConditionBits is the
// pure spec — it must produce exactly the FPSR cc bits that setCC64 sets, for
// every class of float64 result. Slice 5 emits an inline equivalent; this test
// pins the reference so any divergence is caught here, not in execution.
func TestM68KFPUConditionBits_MatchesSetCC64(t *testing.T) {
	fpu := NewM68881FPU()
	vals := []float64{
		0,
		math.Copysign(0, -1), // -0.0 → Zero, NOT Negative (zero check precedes sign)
		1, -1, 2.5, -2.5,
		math.Inf(1), math.Inf(-1),
		math.NaN(),
		math.SmallestNonzeroFloat64,  // positive subnormal → not zero
		-math.SmallestNonzeroFloat64, // negative subnormal → Negative
		math.MaxFloat64, -math.MaxFloat64,
		math.Float64frombits(0x7FF0000000000001), // signalling-ish NaN
		math.Float64frombits(0xFFF8000000000000), // negative NaN → still just NAN
	}
	for _, v := range vals {
		fpu.FPSR = 0xFFFFFFFF // dirty all bits; setCC64 must clear the cc field
		fpu.setCC64(v)
		want := fpu.FPSR & fpuCCMask
		got := m68kFPUConditionBits(math.Float64bits(v))
		if got != want {
			t.Errorf("v=%v bits=%#016x: got cc %#x, want %#x", v, math.Float64bits(v), got, want)
		}
	}
}
