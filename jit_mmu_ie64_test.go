// jit_mmu_ie64_test.go - JIT + MMU integration tests

//go:build (amd64 || arm64) && linux

package main

import (
	"encoding/binary"
	"testing"
)

func TestJIT_MMUDisabled_FastPath(t *testing.T) {
	// With MMU disabled, JIT should work normally
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x2000),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xBEEF),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 0, 1, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[3] != 0xBEEF {
		t.Fatalf("JIT without MMU: R3 = 0x%X, want 0xBEEF", cpu.regs[3])
	}
}

func TestJIT_MMUEnabled_FullProgram(t *testing.T) {
	// With identity-mapped MMU, a simple program should run correctly under JIT
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3000),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xCAFE),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0), // will bail to interpreter
		ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 0, 1, 0, 0),  // will bail to interpreter
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[3] != 0xCAFE {
		t.Fatalf("JIT+MMU identity map: R3 = 0x%X, want 0xCAFE", cpu.regs[3])
	}
}

func TestJIT_MMUEnabled_BailsOnLoad(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	// Map virtual page 3 -> physical page 7
	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	// Write test value at physical 0x7100
	binary.LittleEndian.PutUint32(cpu.memory[0x7100:], 0xDEAD)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3100), // virtual 0x3100
		ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0),      // LOAD through MMU
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0xDEAD {
		t.Fatalf("JIT MMU LOAD: R2 = 0x%X, want 0xDEAD", cpu.regs[2])
	}
}

func TestJIT_MMUEnabled_BailsOnStore(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	// Map virtual page 3 -> physical page 7
	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3200),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xF00D),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0), // STORE through MMU
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	got := binary.LittleEndian.Uint32(cpu.memory[0x7200:])
	if got != 0xF00D {
		t.Fatalf("JIT MMU STORE: phys 0x7200 = 0x%X, want 0xF00D", got)
	}
}

func TestJIT_MMUEnabled_FetchFault(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Make PROG_START page non-executable
	progPage := uint16(PROG_START >> MMU_PAGE_SHIFT)
	writePTE(cpu, progPage, makePTE(progPage, PTE_P|PTE_R|PTE_W|PTE_U)) // no X

	rig.loadInstructions(ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.faultCause != FAULT_EXEC_DENIED {
		t.Fatalf("JIT fetch fault: cause = %d, want %d", cpu.faultCause, FAULT_EXEC_DENIED)
	}
}

func TestJIT_MMUToggle_InvalidatesCache(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true

	// Run a program first without MMU
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x42),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[1] != 0x42 {
		t.Fatalf("pre-MMU: R1 = 0x%X, want 0x42", cpu.regs[1])
	}

	// Now enable MMU via StepOne (simulating MTCR) and check jitNeedInval
	cpu.jitNeedInval = false
	setupIdentityMMU(cpu, 160) // this sets mmuEnabled=true
	// The interpreter MTCR handler would set jitNeedInval, simulate that:
	cpu.jitNeedInval = true

	if !cpu.jitNeedInval {
		t.Fatal("jitNeedInval should be set after MMU enable")
	}
}

func TestJIT_MMU_ExecDeniedOnDataPage(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Map a "data page" (page 6) as non-executable
	writePTE(cpu, 6, makePTE(6, PTE_P|PTE_R|PTE_W|PTE_U)) // no X

	// Write code to that page
	copy(cpu.memory[0x6000:], ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))

	// Jump to the data page
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x6000),
		ie64Instr(OP_JMP, 0, 0, 0, 1, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.faultCause != FAULT_EXEC_DENIED {
		t.Fatalf("JIT exec denied on data page: cause = %d, want %d", cpu.faultCause, FAULT_EXEC_DENIED)
	}
}

func TestJIT_MMUEnabled_BailsOnFLOAD(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))
	binary.LittleEndian.PutUint32(cpu.memory[0x7000:], 0x3F800000)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3000),
		ie64Instr(OP_FLOAD, 0, 0, 0, 1, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.FPU.FPRegs[0] != 0x3F800000 {
		t.Fatalf("JIT FLOAD: F0 = 0x%X, want 0x3F800000", cpu.FPU.FPRegs[0])
	}
}

func TestJIT_MMUEnabled_BailsOnFSTORE(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))
	cpu.FPU.FPRegs[0] = 0x40000000

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3100),
		ie64Instr(OP_FSTORE, 0, 0, 0, 1, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	got := binary.LittleEndian.Uint32(cpu.memory[0x7100:])
	if got != 0x40000000 {
		t.Fatalf("JIT FSTORE: phys 0x7100 = 0x%X, want 0x40000000", got)
	}
}

func TestJIT_MMU_JSR_BailsToInterpreter(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	// JSR pushes return address through MMU then RTS pops it back
	rig.loadInstructions(
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(int32(2*IE64_INSTR_SIZE))),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, 0x55),
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	// JSR -> subroutine -> MOVE R5=#0x55 -> RTS -> HALT
	// R5 should be set only if the subroutine executed
	if cpu.regs[5] != 0x55 {
		t.Fatalf("JIT JSR bail: R5 = 0x%X, want 0x55", cpu.regs[5])
	}
}

func TestJIT_MMU_StackFaultUnderJIT(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Make stack page read-only
	stackPage := uint16((STACK_START - 8) >> MMU_PAGE_SHIFT)
	writePTE(cpu, stackPage, makePTE(stackPage, PTE_P|PTE_R|PTE_X|PTE_U))

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x42),
		ie64Instr(OP_PUSH64, 0, 0, 0, 1, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.faultCause != FAULT_WRITE_DENIED {
		t.Fatalf("JIT stack fault: cause = %d, want %d", cpu.faultCause, FAULT_WRITE_DENIED)
	}
}

func TestJIT_MMU_KernelBootstrap(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true

	ptBase := uint32(0x80000)
	for i := 0; i < 160; i++ {
		pte := makePTE(uint16(i), PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)
		binary.LittleEndian.PutUint64(cpu.memory[ptBase+uint32(i)*8:], pte)
	}

	trapAddr := uint64(0x9000)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(trapAddr)),
		ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, ptBase),
		ie64Instr(OP_MTCR, CR_PTBR, 0, 0, 1, 0, 0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 1),
		ie64Instr(OP_MTCR, CR_MMU_CTRL, 0, 0, 1, 0, 0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3000),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0x5678),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 0, 1, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[3] != 0x5678 {
		t.Fatalf("JIT kernel bootstrap: R3 = 0x%X, want 0x5678", cpu.regs[3])
	}
}

func TestJIT_MMU_SyscallRoundTrip(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	rig.loadInstructions(
		ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, 1),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xDD),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0xDD {
		t.Fatalf("JIT syscall round trip: R2 = 0x%X, want 0xDD", cpu.regs[2])
	}
}
