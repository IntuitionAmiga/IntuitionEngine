//go:build amd64 && (linux || windows || darwin)

package main

import (
	"math"
	"testing"
)

// Slice 5c of native M68K FPU JIT: end-to-end parity. Each register-to-register
// FPU op runs through the interpreter and the JIT with identical preset FP
// registers; the full FPU state (all FP regs + FPCR/FPSR/FPIAR) must match. This
// is the oracle for both the SSE2 arithmetic and the inline condition-code
// emission, across every result class (positive/negative/zero/inf/nan).

var fpuParityValues = []struct{ a, b float64 }{
	{1.0, 2.0},     // → 3.0 positive
	{5.0, -5.0},    // → 0.0 zero
	{-1.0, -2.0},   // → -3.0 negative
	{2.5, 0.25},    // positive normal
	{-2.5, 0.25},   // negative
	{1e308, 1e308}, // → +Inf (overflow)
	{math.Inf(1), 1.0},
	{math.Inf(-1), 1.0},
	{math.NaN(), 1.0},
	{0.0, math.Copysign(0, -1)}, // +0 + -0 → +0
	{math.MaxFloat64, -math.MaxFloat64},
}

func runM68KFPUParity(t *testing.T, name string, opcodes []uint16, preset func(*M68KCPU)) {
	t.Helper()
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const startPC = uint32(0x1000)

	interp := newM68KTestProgramCPU(t, startPC)
	preset(interp)
	writeM68KStopProgram(interp, startPC, opcodes...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	preset(jit)
	writeM68KStopProgram(jit, startPC, opcodes...)
	runM68KJITUntilStopped(t, jit)

	// Bit-exact comparison: NaN != NaN under normal float compare, so the shared
	// asserter cannot validate NaN results. Compare raw bit patterns instead,
	// which also pins NaN payloads.
	for reg := range 8 {
		g := math.Float64bits(jit.FPU.GetFP64(reg))
		w := math.Float64bits(interp.FPU.GetFP64(reg))
		if g != w {
			t.Fatalf("%s: FP%d bits got=%#016x want=%#016x", name, reg, g, w)
		}
	}
	if jit.FPU.FPSR != interp.FPU.FPSR {
		t.Fatalf("%s: FPSR got=%#08x want=%#08x", name, jit.FPU.FPSR, interp.FPU.FPSR)
	}
	if jit.FPU.FPCR != interp.FPU.FPCR {
		t.Fatalf("%s: FPCR got=%#08x want=%#08x", name, jit.FPU.FPCR, interp.FPU.FPCR)
	}
	if jit.FPU.FPIAR != interp.FPU.FPIAR {
		t.Fatalf("%s: FPIAR got=%#08x want=%#08x", name, jit.FPU.FPIAR, interp.FPU.FPIAR)
	}

	if jit.m68kJitNativeBlocksExecuted.Load() == 0 {
		t.Fatalf("%s: block did not execute natively", name)
	}
	if got := jit.m68kJitFallbackOpcodeCounts[opcodes[0]].Load(); got != 0 {
		t.Fatalf("%s: FPU opcode 0x%04X fell back %d times (expected native)", name, opcodes[0], got)
	}
}

func fpuRegToRegProgram(op uint16, src, dst int) []uint16 {
	return []uint16{0xF200, uint16(src&7)<<10 | uint16(dst&7)<<7 | (op & 0x7F)}
}

func TestM68KJIT_NativeFPU_BinaryParity(t *testing.T) {
	ops := []struct {
		name   string
		opmode uint16
	}{
		{"FADD", FPU_OP_FADD},
		{"FSUB", FPU_OP_FSUB},
		{"FMUL", FPU_OP_FMUL},
		{"FDIV", FPU_OP_FDIV},
		{"FSGLDIV", FPU_OP_FSGLDIV},
		{"FSGLMUL", FPU_OP_FSGLMUL},
	}
	for _, o := range ops {
		for _, v := range fpuParityValues {
			a, b := v.a, v.b
			prog := fpuRegToRegProgram(o.opmode, 0, 1) // op FP0,FP1 → fp[1] OP= fp[0]
			runM68KFPUParity(t, o.name, prog, func(cpu *M68KCPU) {
				cpu.FPU.SetFP64(0, a)
				cpu.FPU.SetFP64(1, b)
			})
		}
	}
}

func TestM68KJIT_NativeFPU_MoveAndSqrtParity(t *testing.T) {
	for _, v := range fpuParityValues {
		a := v.a
		runM68KFPUParity(t, "FMOVE", fpuRegToRegProgram(FPU_OP_FMOVE, 2, 3), func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(2, a)
		})
		runM68KFPUParity(t, "FSQRT", fpuRegToRegProgram(FPU_OP_FSQRT, 2, 3), func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(2, math.Abs(a)) // sqrt domain; NaN/Inf still exercised via a
		})
		runM68KFPUParity(t, "FABS", fpuRegToRegProgram(FPU_OP_FABS, 2, 3), func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(2, a)
		})
		runM68KFPUParity(t, "FNEG", fpuRegToRegProgram(FPU_OP_FNEG, 2, 3), func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(2, a)
		})
		runM68KFPUParity(t, "FTST", fpuRegToRegProgram(FPU_OP_FTST, 2, 3), func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(2, a)
		})
	}
}

func TestM68KJIT_NativeFPU_CompareParity(t *testing.T) {
	for _, v := range fpuParityValues {
		a, b := v.a, v.b
		runM68KFPUParity(t, "FCMP", fpuRegToRegProgram(FPU_OP_FCMP, 0, 1), func(cpu *M68KCPU) {
			cpu.FPU.SetFP64(0, a)
			cpu.FPU.SetFP64(1, b)
		})
	}
}
