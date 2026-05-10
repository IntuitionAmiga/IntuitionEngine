package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestM68KFPU_NumericDifferential_RuntimeVsHostMath closes the Phase 7.7
// runtime-differential gate: transpile a m68k FPU program, assemble via
// sdk/bin/ie64asm, execute on the IE64 CPU64 core, read FP results back
// from guest memory, and compare against `math.Sin` / `math.Cos` / etc.
// within doubled-precision ε.
//
// The harness lives in the root package so it can instantiate CPU64 +
// MachineBus directly. It transpiles via shell-out to sdk/bin/m68kto64
// so the integration also exercises the CLI surface.
//
// Skips if any toolchain artefact is missing.
func TestM68KFPU_NumericDifferential_RuntimeVsHostMath(t *testing.T) {
	repoRoot, _ := os.Getwd()

	transpiler := filepath.Join(repoRoot, "sdk", "bin", "m68kto64")
	asm := filepath.Join(repoRoot, "sdk", "bin", "ie64asm")
	for _, p := range []string{transpiler, asm} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("%s not built: %v", p, err)
		}
	}

	src := filepath.Join(repoRoot, "cmd", "m68kto64", "golden", "fpu_numeric.s")
	if _, err := os.Stat(src); err != nil {
		t.Skipf("golden fpu_numeric.s missing: %v", err)
	}

	tmp := t.TempDir()
	ie64s := filepath.Join(tmp, "fpu_numeric.ie64.s")
	out, err := exec.Command(transpiler, "-no-header", "-o", ie64s, src).CombinedOutput()
	if err != nil {
		t.Fatalf("transpile failed: %v\n%s", err, out)
	}
	bin := filepath.Join(tmp, "fpu_numeric.bin")
	out, err = exec.Command(asm, ie64s, "-o", bin).CombinedOutput()
	if err != nil {
		t.Fatalf("assemble failed: %v\n%s", err, out)
	}
	prog, err := os.ReadFile(bin)
	if err != nil {
		t.Fatalf("read binary: %v", err)
	}

	// Load into a CPU64. The transpiled program halts on an RTS that pops a
	// return address from an empty guest stack; CPU64 traps that as a fault.
	// We step manually and tear down on fault / step budget exhaustion.
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.LoadProgramBytes(prog)

	const stepBudget = 100_000
	for i := 0; i < stepBudget; i++ {
		cpu.StepOne()
		if cpu.trapHalted {
			break
		}
	}

	// Read back the 5 double-precision results from guest memory.
	// Layout matches the m68k source's store addresses.
	addrs := []uint32{0x1000, 0x1008, 0x1010, 0x1018, 0x1020}
	refs := []float64{
		math.Sin(1.0),
		math.Cos(0.5),
		math.Exp(1.0),
		math.Log(2.0),
		math.Sqrt(2.0),
	}
	names := []string{"sin(1.0)", "cos(0.5)", "exp(1.0)", "log(2.0)", "sqrt(2.0)"}

	const eps = 1e-6 // doubled-precision ε with transcendental-approximation slack
	for i, addr := range addrs {
		var buf [8]byte
		for j := 0; j < 8; j++ {
			b, _ := bus.Read8WithFault(addr + uint32(j))
			buf[j] = b
		}
		got := math.Float64frombits(binary.LittleEndian.Uint64(buf[:]))
		want := refs[i]
		if math.IsNaN(got) || math.IsInf(got, 0) {
			t.Errorf("%s @ 0x%x: NaN/Inf result; transpile or runtime broke",
				names[i], addr)
			continue
		}
		t.Logf("%s @ 0x%x: got %.17g, want %.17g (Δ=%g)",
			names[i], addr, got, want, math.Abs(got-want))
		if math.Abs(got-want) > eps {
			t.Errorf("%s @ 0x%x: got %.17g, want %.17g (Δ=%g, ε=%g)",
				names[i], addr, got, want, math.Abs(got-want), eps)
		}
	}
}

// ensure bytes import is used even on the skip path
var _ = bytes.Compare
