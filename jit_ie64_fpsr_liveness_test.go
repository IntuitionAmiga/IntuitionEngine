//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// TestIE64MarkFPSRCCDead_ElidesOverwritten checks the backward pass marks a CC
// write dead when a later non-faulting CC writer overwrites the field before any
// observer, and keeps the final write (observed by FMOVSR) live.
func TestIE64MarkFPSRCCDead_ElidesOverwritten(t *testing.T) {
	instrs := []JITInstr{
		{opcode: OP_FMOVI, rd: 1, rs: 1}, // 0: CC write, overwritten by #2 -> dead
		{opcode: OP_MOVE, rd: 2, rs: 0},  // 1: transparent, does not break the chain
		{opcode: OP_FMOVI, rd: 1, rs: 2}, // 2: CC write, observed by FMOVSR -> live
		{opcode: OP_FMOVSR, rd: 3},       // 3: observer (reads FPSR)
		{opcode: OP_HALT64},              // 4
	}
	ie64MarkFPSRCCDead(instrs)
	if !instrs[0].fpsrCCDead {
		t.Errorf("instr 0 (FMOVI overwritten before observer) should be CC-dead")
	}
	if instrs[2].fpsrCCDead {
		t.Errorf("instr 2 (FMOVI observed by FMOVSR) must stay CC-live")
	}
}

// TestIE64MarkFPSRCCDead_ObserverBarrier verifies a faulting/observing op between
// two CC writers keeps the earlier write live, and the block end is an observer.
func TestIE64MarkFPSRCCDead_ObserverBarrier(t *testing.T) {
	// FLOAD can fault -> observer barrier for the earlier write.
	barrier := []JITInstr{
		{opcode: OP_FABS, rd: 1, rs: 1},  // 0: followed by faulting FLOAD -> live
		{opcode: OP_FLOAD, rd: 2, rs: 3}, // 1: faulting; its own CC dead? followed by killer -> see below
		{opcode: OP_FABS, rd: 4, rs: 4},  // 2: killer, but block end observes -> live
	}
	ie64MarkFPSRCCDead(barrier)
	if barrier[0].fpsrCCDead {
		t.Errorf("FABS before faulting FLOAD must stay CC-live (trap could observe)")
	}
	if !barrier[1].fpsrCCDead {
		t.Errorf("FLOAD CC overwritten by later FABS before any observer should be dead")
	}
	if barrier[2].fpsrCCDead {
		t.Errorf("final FABS is observed by block end and must stay CC-live")
	}

	// Lone CC writer at block end: always live.
	lone := []JITInstr{{opcode: OP_FMOVI, rd: 1, rs: 1}}
	ie64MarkFPSRCCDead(lone)
	if lone[0].fpsrCCDead {
		t.Errorf("sole CC writer must stay live (block end is an observer)")
	}
}

// buildFPSRDeadCCProgram writes a straight-line FP block with a dead CC write
// (first FMOVI, overwritten before observation) followed by a live CC write whose
// result is read back into a GPR via FMOVSR, then HALT. Parity must hold whether
// or not the JIT elides the first CC update.
func buildFPSRDeadCCProgram(mem []byte) {
	base := uint64(PROG_START)
	put := func(off uint64, b []byte) { copy(mem[base+off:], b) }
	put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x00000000)) // R1 = +0.0 bits
	put(0x08, ie64Instr(OP_FMOVI, 1, 0, 0, 1, 0, 0))                   // F1 = R1 (CC Z) -> dead
	put(0x10, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 0xC0000000)) // R2 = -2.0 bits
	put(0x18, ie64Instr(OP_FMOVI, 1, 0, 0, 2, 0, 0))                   // F1 = R2 (CC N) -> live
	put(0x20, ie64Instr(OP_FMOVSR, 3, 0, 0, 0, 0, 0))                  // R3 = FPSR (observer)
	put(0x28, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

// TestJIT_vs_Interpreter_FPSRDeadCC runs the dead-CC FP block under the JIT (which
// elides the first CC update) and the interpreter, asserting identical GPRs, FP
// registers, FPSR and PC. This is the parity gate for FPSR CC liveness elision.
func TestJIT_vs_Interpreter_FPSRDeadCC(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	jitCPU := runToHaltAt(t, true, buildFPSRDeadCCProgram)
	interpCPU := runToHaltAt(t, false, buildFPSRDeadCCProgram)

	if jitCPU.PC != interpCPU.PC {
		t.Fatalf("PC mismatch: JIT 0x%X, interp 0x%X", jitCPU.PC, interpCPU.PC)
	}
	for i := range jitCPU.regs {
		if jitCPU.regs[i] != interpCPU.regs[i] {
			t.Fatalf("R%d mismatch: JIT 0x%X, interp 0x%X", i, jitCPU.regs[i], interpCPU.regs[i])
		}
	}
	if jitCPU.FPU.FPSR != interpCPU.FPU.FPSR {
		t.Fatalf("FPSR mismatch: JIT 0x%08X, interp 0x%08X", jitCPU.FPU.FPSR, interpCPU.FPU.FPSR)
	}
	for i := range jitCPU.FPU.FPRegs {
		if jitCPU.FPU.FPRegs[i] != interpCPU.FPU.FPRegs[i] {
			t.Fatalf("F%d mismatch: JIT 0x%08X, interp 0x%08X", i, jitCPU.FPU.FPRegs[i], interpCPU.FPU.FPRegs[i])
		}
	}
	// The observer must have captured the live (negative) CC, proving the second
	// write survived and the read is correct.
	if uint32(jitCPU.regs[3])&IE64_FPU_CC_N == 0 {
		t.Fatalf("observed FPSR in R3=0x%X lacks CC_N; live CC write was lost", jitCPU.regs[3])
	}
}
