//go:build (amd64 || arm64) && (linux || windows || darwin)

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

// TestIE64MarkFPSRCCDead_FP64 checks the FP64 opcodes participate as elidable CC
// writers but never as killers or transparent ops (each can bail, and a bail lets
// the interpreter deliver an interrupt that observes the CC field).
func TestIE64MarkFPSRCCDead_FP64(t *testing.T) {
	// DADD writes CC; a later FP32 killer (FABS) overwrites it before the
	// FMOVSR observer, so DADD's CC is dead.
	instrs := []JITInstr{
		{opcode: OP_DADD, rd: 4, rs: 0, rt: 2}, // 0: CC write, overwritten -> dead
		{opcode: OP_FABS, rd: 6, rs: 6},        // 1: FP32 killer overwrites CC
		{opcode: OP_FMOVSR, rd: 3},             // 2: observer
	}
	ie64MarkFPSRCCDead(instrs)
	if !instrs[0].fpsrCCDead {
		t.Errorf("DADD overwritten by later FABS before observer should be CC-dead")
	}

	// An FP64 op must not kill an earlier CC write: a DADD between an elidable
	// writer and the observer leaves that writer live (DADD could bail).
	noKill := []JITInstr{
		{opcode: OP_FMOVI, rd: 6, rs: 1}, // 0: must stay live (DADD is not a killer)
		{opcode: OP_DADD, rd: 4, rs: 0, rt: 2},
		{opcode: OP_FMOVSR, rd: 3},
	}
	ie64MarkFPSRCCDead(noKill)
	if noKill[0].fpsrCCDead {
		t.Errorf("FMOVI before non-killer DADD must stay CC-live")
	}

	// DLOAD is elidable but is not a killer: an earlier writer survives it.
	load := []JITInstr{
		{opcode: OP_FMOVI, rd: 6, rs: 1}, // 0: DLOAD can fault -> stays live
		{opcode: OP_DLOAD, rd: 4, rs: 5},
		{opcode: OP_FMOVSR, rd: 3},
	}
	ie64MarkFPSRCCDead(load)
	if load[0].fpsrCCDead {
		t.Errorf("FMOVI before faulting DLOAD must stay CC-live")
	}
}

// TestIE64CompileBlockRunsFPSRCCLiveness pins that compileBlock actually runs the
// liveness pass, on whichever backend is being built.
//
// Every other test here would still pass if the pass were never called: eliding
// nothing is always correct, just slower. So dropping the ie64MarkFPSRCCDead call
// from a backend's compileBlock would silently forfeit the optimisation with a
// fully green suite. compileBlock marks in place, which is what makes this
// observable at all.
func TestIE64CompileBlockRunsFPSRCCLiveness(t *testing.T) {
	instrs := []JITInstr{
		{opcode: OP_FMOVI, rd: 1, rs: 1}, // 0: CC overwritten by #1 -> must be marked dead
		{opcode: OP_FMOVI, rd: 1, rs: 2}, // 1: observed by FMOVSR -> must stay live
		{opcode: OP_FMOVSR, rd: 3},       // 2: observer
	}
	execMem, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Skipf("AllocExecMem: %v", err)
	}
	defer execMem.Free()

	if _, err := compileBlock(instrs, PROG_START, execMem); err != nil {
		t.Fatalf("compileBlock: %v", err)
	}
	if !instrs[0].fpsrCCDead {
		t.Errorf("compileBlock did not mark the dead CC write: the liveness pass is not wired in")
	}
	if instrs[1].fpsrCCDead {
		t.Errorf("compileBlock marked an observed CC write dead")
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

// buildFPSRDeadCC64Program builds an FP64 block whose DADD CC update is dead
// (overwritten by a later FP32 FABS before the FMOVSR observer). Parity must hold
// whether or not the JIT elides the DADD CC write.
func buildFPSRDeadCC64Program(mem []byte) {
	base := uint64(PROG_START)
	put := func(off uint64, b []byte) { copy(mem[base+off:], b) }
	put(0x00, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 3)) // R1 = 3
	put(0x08, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, 5)) // R2 = 5
	put(0x10, ie64Instr(OP_DCVTIF, 0, 0, 0, 1, 0, 0))         // D0 = 3.0
	put(0x18, ie64Instr(OP_DCVTIF, 2, 0, 0, 2, 0, 0))         // D2 = 5.0
	put(0x20, ie64Instr(OP_DADD, 4, IE64_SIZE_L, 0, 0, 2, 0)) // D4 = 8.0 (CC -> dead)
	put(0x28, ie64Instr(OP_FABS, 6, 0, 0, 6, 0, 0))           // FP32 F6 = |F6| (kills CC)
	put(0x30, ie64Instr(OP_FMOVSR, 3, 0, 0, 0, 0, 0))         // R3 = FPSR (observer)
	put(0x38, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
}

// TestJIT_vs_Interpreter_FPSRDeadCC64 is the FP64 parity gate for CC liveness.
func TestJIT_vs_Interpreter_FPSRDeadCC64(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	jitCPU := runToHaltAt(t, true, buildFPSRDeadCC64Program)
	interpCPU := runToHaltAt(t, false, buildFPSRDeadCC64Program)

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
