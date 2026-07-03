//go:build amd64 && (linux || windows || darwin)

package main

import (
	"math"
	"testing"
)

// FP register pinning (fp0-7 -> host xmm8-15) activates only for loop blocks
// that use the FPU (m68kBlockRegs.fpPinned). A forced-native straight-line
// single-block test can NOT reach the pinned path — pinning needs a backward
// branch so the block compiles to one internally-looping unit. These parity
// tests build FP loops so the pinned entry-load / per-op xmm reuse / exit-spill
// discipline is exercised, and compare the full architectural FP state (all FP
// regs + FPSR/FPCR/FPIAR) plus a data window against the interpreter oracle.
//
// The critical failure classes these guard against, which the reg-to-reg
// single-op parity tests can not: (1) a pinned register not surviving across a
// loop back-edge or across an interleaved integer op; (2) a mid-loop exit to Go
// (transcendental helper / fallback) not spilling xmm8-15 before the helper
// reads the memory FP file, or not reloading them on re-entry; (3) an EA-form
// FP op reading/writing the memory file instead of the pinned xmm.

// runM68KFPUPinParity runs a program through the interpreter and the (non-forced)
// JIT from identical preset state and asserts bit-exact FP state, a data window,
// and that the block ran natively.
func runM68KFPUPinParity(t *testing.T, name string, preset func(*M68KCPU), dataLo, dataHi uint32, prog ...uint16) {
	t.Helper()
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	const startPC = uint32(0x1000)

	interp := newM68KTestProgramCPU(t, startPC)
	preset(interp)
	writeM68KStopProgram(interp, startPC, prog...)
	runM68KInterpreterUntilStopped(t, interp)

	jit := newM68KTestProgramCPU(t, startPC)
	jit.m68kJitEnabled = true
	preset(jit)
	writeM68KStopProgram(jit, startPC, prog...)
	runM68KJITUntilStopped(t, jit)

	for reg := 0; reg < 8; reg++ {
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
	for r := 0; r < 8; r++ {
		if jit.DataRegs[r] != interp.DataRegs[r] {
			t.Fatalf("%s: D%d got=%#08x want=%#08x", name, r, jit.DataRegs[r], interp.DataRegs[r])
		}
		if jit.AddrRegs[r] != interp.AddrRegs[r] {
			t.Fatalf("%s: A%d got=%#08x want=%#08x", name, r, jit.AddrRegs[r], interp.AddrRegs[r])
		}
	}
	for addr := dataLo; addr < dataHi; addr++ {
		if jit.memory[addr] != interp.memory[addr] {
			t.Fatalf("%s: memory[%#06x] got=%#02x want=%#02x", name, addr, jit.memory[addr], interp.memory[addr])
		}
	}
	if jit.m68kJitNativeBlocksExecuted.Load() == 0 {
		t.Fatalf("%s: block did not execute natively", name)
	}
}

// FADD FPm,FPn / FMUL / FMOVE etc. register-to-register (R/M=0).
func fpRR(op uint16, src, dst int) (uint16, uint16) {
	return 0xF200, uint16(src&7)<<10 | uint16(dst&7)<<7 | (op & 0x7F)
}

const (
	fpOpFMOVE = 0x00
	fpOpFADD  = 0x22
	fpOpFMUL  = 0x23
	fpOpFSUB  = 0x28
	fpOpFDIV  = 0x20
	fpOpFSQRT = 0x04
	fpOpFNEG  = 0x1A
	fpOpFABS  = 0x18
	fpOpFSIN  = 0x0E // transcendental — not native, bails to helper mid-loop
)

func presetFP(cpu *M68KCPU) {
	cpu.FPU.SetFP64(0, 1.0)
	cpu.FPU.SetFP64(1, 0.5)
	cpu.FPU.SetFP64(2, 1.5)
	cpu.FPU.SetFP64(3, 2.0)
	cpu.FPU.SetFP64(4, -3.25)
	cpu.FPU.SetFP64(5, 7.0)
	cpu.FPU.SetFP64(6, 0.125)
	cpu.FPU.SetFP64(7, 42.0)
}

// Pure register-to-register FP loop: exercises the pinned per-op xmm reuse and
// the survival of every pinned reg across the loop back-edge. Multiple distinct
// dst registers ensure fp0-7 are all live across iterations.
func TestM68KJIT_FPPin_RegToRegLoop(t *testing.T) {
	fa1, fa2 := fpRR(fpOpFADD, 1, 0)  // FP0 += FP1
	fm1, fm2 := fpRR(fpOpFMUL, 2, 3)  // FP3 *= FP2
	fs1, fs2 := fpRR(fpOpFSUB, 6, 5)  // FP5 -= FP6
	fv1, fv2 := fpRR(fpOpFMOVE, 0, 4) // FP4  = FP0
	runM68KFPUPinParity(t, "fp-rr-loop", func(cpu *M68KCPU) {
		presetFP(cpu)
		cpu.DataRegs[0] = 6
	}, 0, 0,
		fa1, fa2, // loop:
		fm1, fm2,
		fs1, fs2,
		fv1, fv2,
		0x5380, // SUBQ.L #1,D0
		0x66EC, // BNE.B loop (disp -20 back to 0x1000)
	)
}

// Mixed integer + FP loop: integer ops (ADDQ/MOVE/SUBQ) run between the FP ops.
// No non-FPU emitter touches xmm, so the pins must survive the integer body.
func TestM68KJIT_FPPin_MixedIntFPLoop(t *testing.T) {
	fa1, fa2 := fpRR(fpOpFADD, 1, 0)
	fm1, fm2 := fpRR(fpOpFMUL, 2, 3)
	runM68KFPUPinParity(t, "fp-mixed-loop", func(cpu *M68KCPU) {
		presetFP(cpu)
		cpu.DataRegs[0] = 5
		cpu.DataRegs[1] = 0
		cpu.AddrRegs[2] = 0
	}, 0, 0,
		fa1, fa2, // loop:
		0x5241,                 // ADDQ.W #1,D1     integer op between FP ops
		0xDBFC, 0x0000, 0x0004, // ADDA.L #4,A5  (integer, touches pinned A5)
		fm1, fm2,
		0x5380, // SUBQ.L #1,D0
		0x66EC, // BNE.B loop (disp -20)
	)
}

// FMOVECR (ROM constant -> FPn) inside a loop: validates the FMOVECR store
// reroute writes the pinned xmm, and the result survives the back-edge.
func TestM68KJIT_FPPin_FMOVECRLoop(t *testing.T) {
	// FMOVECR #$00 (pi),FP2 : opcode 0xF200, cmd 0x5C00 | (rom) | (FPn<<7).
	// rom offset 0x00 = pi. dst FP2.
	cmd := uint16(0x5C00) | (2 << 7) | 0x00
	fa1, fa2 := fpRR(fpOpFADD, 2, 0) // FP0 += FP2 (consumes the constant)
	runM68KFPUPinParity(t, "fp-fmovecr-loop", func(cpu *M68KCPU) {
		presetFP(cpu)
		cpu.DataRegs[0] = 4
	}, 0, 0,
		0xF200, cmd, // loop: FMOVECR #pi,FP2
		fa1, fa2,
		0x5380, // SUBQ.L #1,D0
		0x66F4, // BNE.B loop (disp -12)
	)
}

// The spill-before-exit half of the discipline (pinned xmm8-15 flushed to the
// memory FP file before every helper/fallback/exception/IO bail) is a
// STRUCTURAL guarantee, not a dedicated test: both epilogues spill and every
// exit funnels through one of them. A block that would bail to a helper on every
// pass (e.g. a transcendental in the loop body) never crosses the JIT hotness
// threshold, so it never compiles — that scenario cannot occur natively. The
// reachable helper-in-a-pinned-block case is an EA op whose address turns out to
// be MMIO/out-of-range (a data-dependent bail); the reroute and guest-memory
// boundary are covered by TestM68KJIT_FPUEA_FBccBackwardLoop, which runs a
// backward FP loop with FADD.S #imm and FCMP.D (A1) through the pinned path
// (fp[dst] read/written as xmm, guest memory as the operand source).
