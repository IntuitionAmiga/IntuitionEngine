//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// A reserved/unsupported reg-reg FPU opmode raises Line-F in execFPURegToReg
// WITHOUT writing the FPSR condition codes. The lazy-FPSR elision must NOT treat
// such an op as a no-fault CC overwriter, or the preceding op would elide its
// CC update and the Line-F exception path would observe stale flags.

// firstReservedFPUOpmode returns a base opmode (0..0x3F) with no fpuOpTable
// entry — a reserved encoding that raises Line-F without touching the CC.
func firstReservedFPUOpmode(t *testing.T) uint16 {
	t.Helper()
	for op := uint16(0); op < 0x40; op++ {
		if fpuOpTable[op] == nil {
			return op
		}
	}
	t.Fatal("no reserved FPU opmode found")
	return 0
}

func TestM68KFPU_OverwritesCCNoFault_ExcludesReservedOpmodes(t *testing.T) {
	regReg := func(op uint16) uint16 { return uint16(1)<<10 | uint16(0)<<7 | (op & 0x7F) }

	// Real reg-reg data ops all write the CC with no memory fault → overwriter.
	for _, op := range []uint16{FPU_OP_FADD, FPU_OP_FSUB, FPU_OP_FMUL, FPU_OP_FDIV,
		FPU_OP_FSIN, FPU_OP_FCOS, FPU_OP_FMOD, FPU_OP_FCMP, FPU_OP_FTST, FPU_OP_FMOVE} {
		if !m68kFPUInstrOverwritesCCNoFault(0xF200, regReg(op)) {
			t.Errorf("opmode %#02x: real op should be a no-fault CC overwriter", op)
		}
	}

	// Reserved base opmodes raise Line-F without writing the CC → NOT an
	// overwriter. (Note: the precision bits 0x40/0x04 are part of the opmode
	// encoding — ORing 0x40 onto a base opmode can decode to a different, valid
	// op, so only the bare reserved base opmode is tested here.)
	reserved := firstReservedFPUOpmode(t)
	if m68kFPUInstrOverwritesCCNoFault(0xF200, regReg(reserved)) {
		t.Errorf("reserved opmode %#02x wrongly reported as a no-fault CC overwriter", reserved)
	}

	// FMOVECR still qualifies (constant load, writes CC, no fault).
	if !m68kFPUInstrOverwritesCCNoFault(0xF200, 0x5C00) {
		t.Error("FMOVECR should be a no-fault CC overwriter")
	}
	// A reg→EA store writes no CC.
	if m68kFPUInstrOverwritesCCNoFault(0xF200, uint16(3)<<13|uint16(1)<<10) {
		t.Error("FMOVE store should not be a CC overwriter")
	}
}

// End-to-end: a native FPU op followed by a reserved reg-reg opmode that raises
// Line-F. Under forced native compilation (which bypasses the transcendental
// interpreter-admission that would otherwise steer this block away), the JIT
// must leave the same FPSR as the interpreter on the Line-F exception path —
// i.e. the first op's CC must NOT have been elided.
func TestM68KJIT_LazyFPSR_ReservedOpmodePreservesCC(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const startPC = uint32(0x1000)
	const handlerPC = uint32(0x1400)

	reserved := firstReservedFPUOpmode(t)
	// FADD FP1,FP0 ; <reserved> FP1,FP0
	prog := []uint16{
		0xF200, uint16(1)<<10 | uint16(0)<<7 | FPU_OP_FADD,
		0xF200, uint16(1)<<10 | uint16(0)<<7 | (reserved & 0x7F),
	}
	preset := func(cpu *M68KCPU) {
		// Line-F vector (11) → STOP handler, so the CPU halts after the fault.
		cpu.Write32(11*4, handlerPC)
		cpu.Write16(handlerPC, 0x4E72)
		cpu.Write16(handlerPC+2, 0x2700)
		cpu.AddrRegs[7] = 0x2000 // supervisor stack for the exception frame
		cpu.FPU.SetFP64(0, -3.0)
		cpu.FPU.SetFP64(1, 1.0) // FADD → -2.0 → N flag set
	}

	interp := newM68KTestProgramCPU(t, startPC)
	preset(interp)
	writeM68KStopProgram(interp, startPC, prog...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	jit.m68kJitForceNative = true // bypass transcendental admission; force native compile
	preset(jit)
	writeM68KStopProgram(jit, startPC, prog...)
	runM68KJITUntilStopped(t, jit)

	if jit.FPU.FPSR != interp.FPU.FPSR {
		t.Fatalf("FPSR after Line-F: JIT=%#08x interp=%#08x (first op's CC wrongly elided?)",
			jit.FPU.FPSR, interp.FPU.FPSR)
	}
	// The surviving CC must be FADD's result (-2.0 → Negative), proving it was written.
	if jit.FPU.FPSR&FPU_CC_N == 0 {
		t.Fatalf("expected N flag from FADD result -2.0, FPSR=%#08x", jit.FPU.FPSR)
	}
}
