// jit_helper_resume_test.go - IE64 helper continuation tests.

//go:build (amd64 || arm64) && linux

package main

import (
	"encoding/binary"
	"testing"
)

const helperResumeMMIOAddr = 0xF0700

func loadHelperResumeProgram(cpu *CPU64, handler uint64) {
	offset := uint32(PROG_START)
	instrs := [][]byte{
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 1, 0, 0, 0x42),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 1, 0, 0, helperResumeMMIOAddr),
		ie64Instr(OP_STORE, 1, IE64_SIZE_L, 1, 2, 0, 0),
		ie64Instr(OP_MOVE, 5, IE64_SIZE_Q, 1, 0, 0, 0xCAFE),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	}
	for _, instr := range instrs {
		copy(cpu.memory[offset:], instr)
		offset += uint32(len(instr))
	}
	if handler != 0 {
		copy(cpu.memory[handler:], ie64Instr(OP_MOVE, 10, IE64_SIZE_Q, 1, 0, 0, 0xBEEF))
		copy(cpu.memory[handler+IE64_INSTR_SIZE:], ie64Instr(OP_RTI64, 0, 0, 0, 0, 0, 0))
	}
	cpu.PC = PROG_START
}

func newHelperResumeCPU(t *testing.T) (*CPU64, *uint32, *int) {
	t.Helper()
	bus := NewMachineBus()
	var writeVal uint32
	var writes int
	bus.MapIO(helperResumeMMIOAddr, helperResumeMMIOAddr+3,
		func(addr uint32) uint32 { return 0 },
		func(addr uint32, value uint32) {
			writes++
			writeVal = value
		},
	)
	cpu := NewCPU64(bus)
	cpu.jitEnabled = true
	return cpu, &writeVal, &writes
}

func TestIE64JIT_MMIOStoreMidBlock_ResumesInBlock(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available")
	}
	t.Setenv("IE64_JIT_RESUME", "1")

	cpu, writeVal, writes := newHelperResumeCPU(t)
	loadHelperResumeProgram(cpu, 0)

	base := ie64JITStatsLoad()
	runConfiguredCPU(t, cpu, true)
	diff := ie64JITStatsLoad().Sub(base)

	if *writes != 1 || *writeVal != 0x42 {
		t.Fatalf("MMIO writes=%d value=0x%X, want one write of 0x42", *writes, *writeVal)
	}
	if cpu.regs[5] != 0xCAFE {
		t.Fatalf("R5 = 0x%X, want resumed MOVE to run", cpu.regs[5])
	}
	if diff.helperResumes != 1 {
		t.Fatalf("helperResumes = %d, want 1", diff.helperResumes)
	}
	if diff.tier1Blocks != 1 {
		t.Fatalf("tier1Blocks = %d, want 1 (no second block at post-helper PC)", diff.tier1Blocks)
	}
}

func TestIE64JIT_HelperResume_KillSwitchDisablesResume(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available")
	}
	t.Setenv("IE64_JIT_RESUME", "0")

	cpu, writeVal, writes := newHelperResumeCPU(t)
	loadHelperResumeProgram(cpu, 0)

	base := ie64JITStatsLoad()
	runConfiguredCPU(t, cpu, true)
	diff := ie64JITStatsLoad().Sub(base)

	if *writes != 1 || *writeVal != 0x42 {
		t.Fatalf("MMIO writes=%d value=0x%X, want one write of 0x42", *writes, *writeVal)
	}
	if cpu.regs[5] != 0xCAFE {
		t.Fatalf("R5 = 0x%X, want fallback block to run post-helper MOVE", cpu.regs[5])
	}
	if diff.helperResumes != 0 {
		t.Fatalf("helperResumes = %d, want 0 with IE64_JIT_RESUME=0", diff.helperResumes)
	}
	if diff.tier1Blocks < 2 {
		t.Fatalf("tier1Blocks = %d, want old path to compile a post-helper block", diff.tier1Blocks)
	}
}

func TestIE64JIT_HelperResume_InterruptCancelsResume(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available")
	}
	t.Setenv("IE64_JIT_RESUME", "1")

	cpu, writeVal, writes := newHelperResumeCPU(t)
	handler := uint64(PROG_START + 0x100)
	loadHelperResumeProgram(cpu, handler)
	cpu.interruptVector = handler
	cpu.regs[31] = STACK_START
	cpu.interruptEnabled.Store(true)

	fired := false
	cpu.preBlockHook = func() {
		if fired {
			return
		}
		fired = true
		NewIE64InterruptSink(cpu).Pulse(IntMaskBlitter)
	}

	base := ie64JITStatsLoad()
	runConfiguredCPU(t, cpu, true)
	diff := ie64JITStatsLoad().Sub(base)

	if cpu.regs[10] != 0xBEEF {
		t.Fatalf("R10 = 0x%X, want interrupt handler to run", cpu.regs[10])
	}
	if *writes != 1 || *writeVal != 0x42 {
		t.Fatalf("MMIO writes=%d value=0x%X, want store to run after RTI", *writes, *writeVal)
	}
	if cpu.regs[5] != 0xCAFE {
		t.Fatalf("R5 = 0x%X, want post-helper MOVE to run after RTI", cpu.regs[5])
	}
	if diff.helperResumeCancels != 1 {
		t.Fatalf("helperResumeCancels = %d, want first continuation cancelled by IRQ", diff.helperResumeCancels)
	}
	wantPushed := uint64(PROG_START + 2*IE64_INSTR_SIZE)
	if got := binary.LittleEndian.Uint64(cpu.memory[STACK_START-8:]); got != wantPushed {
		t.Fatalf("pushed PC = 0x%X, want helper PC 0x%X", got, wantPushed)
	}
}

func TestIE64JIT_HelperResume_SMCCancelsResume(t *testing.T) {
	t.Setenv("IE64_JIT_RESUME", "1")
	cpu := NewCPU64(NewMachineBus())
	cpu.jitCtx = newJITContext(cpu)
	cpu.running.Store(true)
	cpu.PC = PROG_START + IE64_INSTR_SIZE
	cpu.jitCtx.ResumeValid = 1
	cpu.jitCtx.ResumeAddr = 1
	cpu.jitCtx.ResumePC = cpu.PC
	cpu.jitCtx.NeedInval = 1

	if cpu.canResumeJITHelper(1) {
		t.Fatal("NeedInval helper exit must cancel resume")
	}
}

func TestIE64JIT_HelperResume_StopCancelsResume(t *testing.T) {
	t.Setenv("IE64_JIT_RESUME", "1")
	cpu := NewCPU64(NewMachineBus())
	cpu.jitCtx = newJITContext(cpu)
	cpu.running.Store(false)
	cpu.PC = PROG_START + IE64_INSTR_SIZE
	cpu.jitCtx.ResumeValid = 1
	cpu.jitCtx.ResumeAddr = 1
	cpu.jitCtx.ResumePC = cpu.PC

	if cpu.canResumeJITHelper(1) {
		t.Fatal("stopped CPU must not resume helper continuation")
	}
}
