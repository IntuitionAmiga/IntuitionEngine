//go:build amd64 && (linux || windows || darwin)

package main

import (
	"math"
	"testing"
)

// Interpreter-admission for transcendental FPU blocks: a block containing a
// helper-only op (FSIN/FCOS/FMOD/…) is executed by an interpreter burst rather
// than compiled to per-instruction helper exits. These tests pin both the
// correctness (JIT result == interpreter) and the admission (the block took the
// transcendental burst path and never emitted a native FPU helper exit).

func TestM68KJIT_TranscendentalPredicate(t *testing.T) {
	transc := []uint16{
		FPU_OP_FSIN, FPU_OP_FCOS, FPU_OP_FTAN, FPU_OP_FETOX, FPU_OP_FLOGN,
		FPU_OP_FGETEXP, FPU_OP_FMOD, FPU_OP_FSCALE,
	}
	for _, op := range transc {
		cmd := uint16(1)<<10 | uint16(0)<<7 | (op & 0x7F) // reg-reg FP1,FP0
		if !m68kFPUInstrIsTranscendental(0xF200, cmd) {
			t.Errorf("op %#02x: expected transcendental", op)
		}
		// EA-load form is also helper-only.
		cmdEA := uint16(1)<<14 | uint16(5)<<10 | uint16(0)<<7 | (op & 0x7F)
		if !m68kFPUInstrIsTranscendental(0xF210, cmdEA) {
			t.Errorf("op %#02x EA: expected transcendental", op)
		}
	}

	native := []uint16{FPU_OP_FADD, FPU_OP_FSUB, FPU_OP_FMUL, FPU_OP_FDIV, FPU_OP_FMOVE, FPU_OP_FSQRT, FPU_OP_FABS, FPU_OP_FCMP}
	for _, op := range native {
		cmd := uint16(1)<<10 | uint16(0)<<7 | (op & 0x7F)
		if m68kFPUInstrIsTranscendental(0xF200, cmd) {
			t.Errorf("op %#02x: native op wrongly flagged transcendental", op)
		}
	}
	// FMOVECR and reg→EA store are native, not transcendental.
	if m68kFPUInstrIsTranscendental(0xF200, 0x5C00) {
		t.Error("FMOVECR flagged transcendental")
	}
	if m68kFPUInstrIsTranscendental(0xF200, uint16(3)<<13|uint16(1)<<10) {
		t.Error("FMOVE store flagged transcendental")
	}
	// Non-FPU and conditional forms are not transcendental.
	if m68kFPUInstrIsTranscendental(0x4E71, 0) { // NOP
		t.Error("NOP flagged transcendental")
	}
}

func TestM68KJIT_TranscendentalLoopInterpretsCorrectly(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const startPC = uint32(0x1000)

	// MOVE.W #2,D7 ; loop: FSIN FP1,FP0 ; FADD FP0,FP2 ; DBF D7,loop
	prog := []uint16{
		0x3E3C, 0x0002, // MOVE.W #2,D7
		0xF200, uint16(1)<<10 | uint16(0)<<7 | FPU_OP_FSIN, // FSIN FP1,FP0
		0xF200, uint16(0)<<10 | uint16(2)<<7 | FPU_OP_FADD, // FADD FP0,FP2
		0x51CF, 0xFFF6, // DBF D7,loop (-10)
	}
	preset := func(cpu *M68KCPU) {
		cpu.FPU.SetFP64(1, 0.5)
		cpu.FPU.SetFP64(2, 0)
	}

	interp := newM68KTestProgramCPU(t, startPC)
	preset(interp)
	writeM68KStopProgram(interp, startPC, prog...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	preset(jit)
	writeM68KStopProgram(jit, startPC, prog...)
	runM68KJITUntilStopped(t, jit)

	if g, w := math.Float64bits(jit.FPU.GetFP64(2)), math.Float64bits(interp.FPU.GetFP64(2)); g != w {
		t.Fatalf("FP2 bits got=%#016x want=%#016x", g, w)
	}
	if jit.FPU.FPSR != interp.FPU.FPSR {
		t.Fatalf("FPSR got=%#08x want=%#08x", jit.FPU.FPSR, interp.FPU.FPSR)
	}
	if jit.m68kJitTranscendentalBursts.Load() == 0 {
		t.Fatal("transcendental block was not admitted to an interpreter burst")
	}
	if got := jit.m68kJitNativeHelperExits.Load(); got != 0 {
		t.Fatalf("transcendental loop took %d native FPU helper exits (expected 0 — should burst)", got)
	}
}
