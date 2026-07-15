//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// The FPSR CC sinking pass (Technique 3) defers a condition-code update to the
// block's exit funnels, where it is rebuilt by re-reading the writer's
// destination register. These tests cover the two ways that can go wrong.
//
// The first reproduces a real divergence: without the ie64CCSinkSafe guard it
// fails with JIT FPSR 0x08000000 (CC_N) against interpreter 0x04000000 (CC_Z).
//
// The second does not currently diverge with the guard removed — block
// formation appears not to hand the analysis a loop shaped this way — so it
// stands as a parity check on a case the guard rejects on principle rather than
// as a demonstrated bug. It is kept because the reasoning that makes it
// dangerous is sound, and a change to block formation could make it reachable
// without anything else noticing.

// TestFPSRSink_ClobberedDestination covers a CC-transparent instruction
// overwriting the sunk writer's destination before the exit.
//
// FMOV neither reads nor writes FPSR, so it does not stop the FADD's CC update
// being sunk, but it does rewrite F1. An exit that classified FPRegs[F1] would
// report the condition codes of the FMOV's source instead of the FADD's result.
func TestFPSRSink_ClobberedDestination(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	build := func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		// F2 = -1.0, F3 = 1.0, F4 = +0.0. FADD F1, F2, F3 -> +0.0 (CC_Z),
		// then FMOV F1, F5 with F5 = -1.0 (CC_N). The two disagree, so a sunk
		// update reading the clobbered F1 lands on CC_N instead of CC_Z.
		put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, fp32NegOne))
		put(0x08, ie64Instr(OP_FMOVI, 2, 0, 0, 1, 0, 0)) // F2 = -1.0
		put(0x10, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, fp32One))
		put(0x18, ie64Instr(OP_FMOVI, 3, 0, 0, 1, 0, 0)) // F3 = 1.0
		put(0x20, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, fp32NegOne))
		put(0x28, ie64Instr(OP_FMOVI, 5, 0, 0, 1, 0, 0)) // F5 = -1.0

		put(0x30, ie64Instr(OP_FADD, 1, 0, 0, 2, 3, 0)) // F1 = -1.0 + 1.0 = +0.0
		put(0x38, ie64Instr(OP_FMOV, 1, 0, 0, 5, 0, 0)) // F1 = F5, clobbers it
		put(0x40, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	assertFPParity(t, "sink/clobbered-destination", build)
}

// TestFPSRSink_ExitBeforeWriterViaBackEdge covers an exit that precedes the
// sunk writer in the instruction stream but is reachable after it through a
// back-edge.
//
// The pending update is a compile-time, linear notion: at the BEQ nothing is
// pending yet, so no materialisation is emitted there. At run time the second
// iteration reaches that same BEQ with the FADD's CC outstanding, and taking it
// leaves the block with FPSR never updated.
func TestFPSRSink_ExitBeforeWriterViaBackEdge(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	build := func(mem []byte) {
		base := uint64(PROG_START)
		put := func(off uint64, ins []byte) { copy(mem[base+off:], ins) }
		put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, fp32NegOne))
		put(0x08, ie64Instr(OP_FMOVI, 2, 0, 0, 1, 0, 0))           // F2 = -1.0
		put(0x10, ie64Instr(OP_FMOVI, 3, 0, 0, 0, 0, 0))           // F3 = +0.0 (R0)
		put(0x18, ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, 2)) // R10 = 2

		// loop (starts at 0x20):
		//   BEQ R10, R0, out   ; exits on the second iteration, after the FADD
		//   FADD F1, F2, F3    ; F1 = -1.0 -> CC_N
		//   SUB  R10, R10, #1
		//   BNE  R10, R11, loop
		put(0x20, ie64Instr(OP_BEQ, 0, 0, 0, 10, 0, 0x18))         // -> 0x38
		put(0x28, ie64Instr(OP_FADD, 1, 0, 0, 2, 3, 0))            // F1 = -1.0
		put(0x30, ie64Instr(OP_SUB, 10, IE64_SIZE_Q, 1, 10, 0, 1)) // R10 -= 1
		put(0x38, ie64Instr(OP_BNE, 0, 0, 0, 10, 11, 0xFFFFFFE8))  // -> 0x20

		put(0x40, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	}
	assertFPParity(t, "sink/exit-before-writer", build)
}
