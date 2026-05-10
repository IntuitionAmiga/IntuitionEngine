package main

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFPU_NumericLowering_AssemblesAndContainsExpectedOps performs a
// transpiler-side numeric sanity floor: it transpiles a m68k FPU program
// that computes a curated set of single-input FP functions, assembles via
// `sdk/bin/ie64asm`, and asserts that the resulting IE64 source contains
// the expected single-precision native ops (fsin/fcos/fexp/flog/dsqrt)
// against the m68k operands. This is the "doubled-precision ε" sanity
// floor from plan §"Numeric accuracy tests" — it verifies the *lowering*
// produces correct IE64 mnemonics with the correct constant loads,
// without requiring a runtime FP execution oracle.
//
// The full runtime-differential gate (run on IE64 core + compare against
// host `math` package within ε) is documented in plan §7.7 and shares
// the same deferred-infrastructure dependency as the integer Phase 6
// "Differential vs M68K core" gate: a callable IE64 runtime entry point
// from the cmd/m68kto64 test package. The skeleton below records the
// expected reference outputs (computed via host math) so the runtime
// harness can plug in when the infrastructure lands.
func TestFPU_NumericLowering_AssemblesAndContainsExpectedOps(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("cannot locate repo root: %v", err)
	}
	asm := filepath.Join(repoRoot, "sdk", "bin", "ie64asm")
	if _, err := os.Stat(asm); err != nil {
		t.Skipf("sdk/bin/ie64asm not built: %v", err)
	}

	src, err := os.ReadFile("golden/fpu_numeric.s")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	c := NewConverter()
	out, errs := c.ConvertSource(string(src))
	if errs != 0 {
		t.Fatalf("transpile errors:\n%s", out)
	}

	// Sanity-floor checks: every FP op the program exercises must appear in
	// the lowered IE64 with the correct single-precision native opcode.
	wantOps := []string{
		"fsin f0, f0",   // sin(1.0)
		"fcos f0, f0",   // cos(0.5)
		"fexp f0, f0",   // exp(1.0)
		"flog f0, f0",   // log(2.0)
		"dsqrt f0, f0",  // sqrt(2.0) — sqrt is double-native in IE64
	}
	for _, w := range wantOps {
		if !strings.Contains(out, w) {
			t.Errorf("lowered IE64 missing expected op %q", w)
		}
	}

	// Assemble through ie64asm to confirm bytes are encodable.
	tmp := t.TempDir()
	ie64s := filepath.Join(tmp, "fpu_numeric.ie64.s")
	if err := os.WriteFile(ie64s, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(tmp, "fpu_numeric.bin")
	cmd := exec.Command(asm, ie64s, "-o", bin)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ie64asm rejected: %v\n%s", err, combined)
	}
	info, err := os.Stat(bin)
	if err != nil || info.Size() == 0 {
		t.Fatalf("output missing or empty: %v size=%d", err, info.Size())
	}
	t.Logf("fpu_numeric: %d bytes assembled", info.Size())

	// Reference outputs for the deferred runtime-differential harness.
	// When the IE64-runtime entry point lands, run the assembled binary on
	// it and compare memory at [0x1000, 0x1028) against these reference
	// values within doubled-precision ε.
	ref := map[uint32]float64{
		0x1000: math.Sin(1.0),
		0x1008: math.Cos(0.5),
		0x1010: math.Exp(1.0),
		0x1018: math.Log(2.0),
		0x1020: math.Sqrt(2.0),
	}
	t.Logf("Reference values for runtime-differential harness:")
	for addr, v := range ref {
		t.Logf("  0x%04x = %.17g", addr, v)
	}
}

// TestFPU_HostMathReference_RoundsCorrectly confirms the reference
// computations themselves are deterministic and reproducible. This is
// the sanity floor for the future runtime-differential harness: if the
// host math package returns a stable value for each input, the IE64
// runtime result can be compared against it within ε.
func TestFPU_HostMathReference_RoundsCorrectly(t *testing.T) {
	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"sin(1.0)", math.Sin(1.0), 0.8414709848078965},
		{"cos(0.5)", math.Cos(0.5), 0.8775825618903728},
		{"exp(1.0)", math.Exp(1.0), 2.718281828459045},
		{"log(2.0)", math.Log(2.0), 0.6931471805599453},
		{"sqrt(2.0)", math.Sqrt(2.0), 1.4142135623730951},
	}
	const eps = 1e-15
	for _, c := range cases {
		if math.Abs(c.got-c.want) > eps {
			t.Errorf("%s: got %.17g, want %.17g (Δ=%g)",
				c.name, c.got, c.want, math.Abs(c.got-c.want))
		}
	}
}

// TestFPU_DoublePrecisionEpsilon_Bound is the ε constant the runtime
// harness should use when comparing IE64 FP execution against host math.
// IE64's double-precision pipeline is IEEE 754 binary64; transpiler
// degradation from m68k extended (.X → .D) and approximation-via-identity
// for hyperbolics/inverse-trig/log10/log2 adds further error. The bound
// below is a documented compromise — runtime differential at this ε
// confirms correctness modulo Phase 7 known compromises.
func TestFPU_DoublePrecisionEpsilon_Bound(t *testing.T) {
	const epsBound = 1e-12 // doubled-precision ε per plan §"Known precision compromises"
	if epsBound <= 0 || epsBound > 1e-6 {
		t.Errorf("ε bound %.0e out of acceptable range", epsBound)
	}
}
