//go:build arm64 && linux

package main

import "testing"

// The arm64 prologue used to load only br.read, the registers the block reads.
// That is wrong: the epilogue stores br.written back unconditionally, so a
// register the block writes but never reads arrived holding whatever the host
// register happened to contain and could be published over its canonical value.
//
// These tests pin the two ways it went wrong. Both are named TestARM64_ so the
// allowlist in scripts/test-cross-compile.sh runs them: the MMU and MMIO tests
// that first exposed the bug are not matched by that regex, which is why it sat
// undetected.

// TestARM64_PrologueLoadsWriteOnlyReg covers a loop that exits before reaching
// a register it writes later. No MMU, no helper: plain guest code.
//
// A conditional-branch exit normally stores writtenSoFar, the registers written
// before the branch, which is why a forward-only block cannot show this. But in
// a block with a backward edge the emitter widens that to br.written, because a
// prior iteration may have written registers appearing after the branch. On the
// first iteration those writes have not run, so the host register still holds
// whatever it was given at entry, and the prologue has to have made that the
// canonical value.
func TestARM64_PrologueLoadsWriteOnlyReg(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true

	// R2 is written only after the exit branch, so it is in br.written but not
	// br.read. R1 = 0 makes the exit branch taken on the first iteration, so the
	// write never runs and R2 must come out untouched.
	const sentinel = 0x1234
	cpu.regs[1] = 0
	cpu.regs[2] = sentinel

	rig.loadInstructions(
		ie64Instr(OP_BEQ, 0, 0, 0, 1, 0, 0x18),              // R1 == R0 -> taken -> out
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xBEEF), // never runs
		ie64Instr(OP_BNE, 0, 0, 0, 1, 1, 0xFFFFFFF0),        // backward edge -> hasBackwardBranch
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),              // out:
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != sentinel {
		t.Fatalf("R2 clobbered by a loop that exited before writing it: R2 = 0x%X, want 0x%X "+
			"(prologue did not load a register the epilogue stores back)",
			cpu.regs[2], sentinel)
	}
}

// TestARM64_ResumeReloadsHelperResult covers the mixed JIT/interpreter handoff.
//
// A helper-exiting LOAD bails, the Go dispatcher writes the result into
// cpu.regs[rd], and the block resumes through emitResumeEntryARM64 ->
// emitPrologue. rd is write-only here, so a prologue that loaded br.read alone
// would not reload it and the next epilogue would store the stale host register
// straight over the helper's result. The Go runtime runs in between, so the
// stale value is typically one of its heap pointers rather than anything that
// looks like guest data.
func TestARM64_ResumeReloadsHelperResult(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)
	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))
	cpu.memory[0x7100] = 0xAD
	cpu.memory[0x7101] = 0xDE

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3100),
		ie64Instr(OP_LOAD, 2, IE64_SIZE_W, 0, 1, 0, 0), // MMU on -> helper exit
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0xDEAD {
		t.Fatalf("helper result lost across resume: R2 = 0x%X, want 0xDEAD", cpu.regs[2])
	}
}
