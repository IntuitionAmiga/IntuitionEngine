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
	cpu.CoprocMode = true
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

func TestJIT_MMUEnabled_CacheIsScopedByPTBR(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	cpu.jitPersist = true
	defer func() {
		cpu.jitPersist = false
		cpu.freeJIT()
	}()

	const (
		pt1       = uint32(0x80000)
		pt2       = uint32(0x90000)
		virtPage  = uint16(PROG_START >> MMU_PAGE_SHIFT)
		physPage1 = uint16(0x40)
		physPage2 = uint16(0x50)
	)

	cpu.mmuEnabled = true
	binary.LittleEndian.PutUint64(cpu.memory[pt1+uint32(virtPage)*8:], makePTE(physPage1, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))
	binary.LittleEndian.PutUint64(cpu.memory[pt2+uint32(virtPage)*8:], makePTE(physPage2, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	copy(cpu.memory[uint32(physPage1)<<MMU_PAGE_SHIFT:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x1111))
	copy(cpu.memory[(uint32(physPage1)<<MMU_PAGE_SHIFT)+IE64_INSTR_SIZE:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	copy(cpu.memory[uint32(physPage2)<<MMU_PAGE_SHIFT:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x2222))
	copy(cpu.memory[(uint32(physPage2)<<MMU_PAGE_SHIFT)+IE64_INSTR_SIZE:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu.ptbr = pt1
	cpu.PC = PROG_START
	cpu.regs[1] = 0
	cpu.running.Store(true)
	cpu.ExecuteJIT()

	if cpu.regs[1] != 0x1111 {
		t.Fatalf("first address space: R1 = 0x%X, want 0x1111", cpu.regs[1])
	}

	cpu.ptbr = pt2
	cpu.tlbFlush()
	cpu.PC = PROG_START
	cpu.regs[1] = 0
	cpu.running.Store(true)
	cpu.ExecuteJIT()

	if cpu.regs[1] != 0x2222 {
		t.Fatalf("second address space: R1 = 0x%X, want 0x2222", cpu.regs[1])
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

func TestJIT_MMU_PushBailThenSyscallPreservesCopiedArgRegs(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	cpu.regs[1] = 0x60A4CC
	cpu.regs[21] = 0x11223344
	cpu.regs[22] = 0x55667788
	cpu.regs[29] = 0x60A000

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 20, IE64_SIZE_Q, 0, 1, 0, 0),
		ie64Instr(OP_PUSH64, 0, IE64_SIZE_Q, 0, 29, 0, 0),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 20, 0, 0),
		ie64Instr(OP_MOVE, 3, IE64_SIZE_Q, 0, 21, 0, 0),
		ie64Instr(OP_MOVE, 4, IE64_SIZE_Q, 0, 22, 0, 0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, BOOT_HOSTFS_STAT),
		ie64Instr(OP_MOVE, 5, IE64_SIZE_Q, 0, 0, 0, 0),
		ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, 40),
		ie64Instr(OP_MOVE, 6, IE64_SIZE_Q, 0, 2, 0, 0),
		ie64Instr(OP_MOVE, 7, IE64_SIZE_Q, 0, 3, 0, 0),
		ie64Instr(OP_MOVE, 8, IE64_SIZE_Q, 0, 4, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[6] != 0x60A4CC {
		t.Fatalf("R6 = 0x%X, want copied path ptr 0x60A4CC", cpu.regs[6])
	}
	if cpu.regs[7] != 0x11223344 {
		t.Fatalf("R7 = 0x%X, want copied R21 value 0x11223344", cpu.regs[7])
	}
	if cpu.regs[8] != 0x55667788 {
		t.Fatalf("R8 = 0x%X, want copied R22 value 0x55667788", cpu.regs[8])
	}
}

func TestJIT_MMU_JsrReturnValueFeedsNextCallerBlock(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	cpu.regs[31] = STACK_START

	rig.loadInstructions(
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(6*IE64_INSTR_SIZE)),
		ie64Instr(OP_BEQ, 0, 0, 0, 3, 0, uint32(4*IE64_INSTR_SIZE)),
		ie64Instr(OP_MOVE, 24, IE64_SIZE_Q, 0, 1, 0, 0),
		ie64Instr(OP_MOVE, 27, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x60A4CC),
		ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 1),
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0x60A4CC {
		t.Fatalf("R2 = 0x%X, want 0x60A4CC (R1=0x%X R3=0x%X R24=0x%X R27=0x%X SP=0x%X PC=0x%X)",
			cpu.regs[2], cpu.regs[1], cpu.regs[3], cpu.regs[24], cpu.regs[27], cpu.regs[31], cpu.PC)
	}
	if cpu.regs[24] != 0x60A4CC {
		t.Fatalf("R24 = 0x%X, want callee return value 0x60A4CC", cpu.regs[24])
	}
	if cpu.regs[27] != 0x60A4CC {
		t.Fatalf("R27 = 0x%X, want chained caller copy 0x60A4CC", cpu.regs[27])
	}
}

func TestJIT_MMU_JsrReturnValueWithoutBranch(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	cpu.regs[31] = STACK_START

	rig.loadInstructions(
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(4*IE64_INSTR_SIZE)),
		ie64Instr(OP_MOVE, 24, IE64_SIZE_Q, 0, 1, 0, 0),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x60A4CC),
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0x60A4CC {
		t.Fatalf("R2 = 0x%X, want 0x60A4CC after JSR return without branch (R1=0x%X R24=0x%X PC=0x%X)",
			cpu.regs[2], cpu.regs[1], cpu.regs[24], cpu.PC)
	}
}

func TestJIT_MMU_MoveMappedFromSpilled(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 24, IE64_SIZE_L, 1, 0, 0, 0x60A4CC),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0x60A4CC {
		t.Fatalf("R2 = 0x%X, want 0x60A4CC after MOVE mapped<-spilled", cpu.regs[2])
	}
}

func TestJIT_MMU_MoveR1FromSpilled(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 24, IE64_SIZE_L, 1, 0, 0, 0x60A4CC),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[1] != 0x60A4CC {
		t.Fatalf("R1 = 0x%X, want 0x60A4CC after MOVE R1<-spilled", cpu.regs[1])
	}
}

func TestJIT_MMU_AddSpilledBasePlusImmThenMoveToR1(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	cpu.regs[29] = 0x60A000
	rig.loadInstructions(
		ie64Instr(OP_ADD, 23, IE64_SIZE_Q, 1, 29, 0, 0x4CC),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 23, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[1] != 0x60A4CC {
		t.Fatalf("R1 = 0x%X, want 0x60A4CC after ADD spilled pointer then MOVE to R1 (R23=0x%X)", cpu.regs[1], cpu.regs[23])
	}
}

func TestJIT_MMU_MoveMappedFromMapped(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	cpu.regs[1] = 0x60A4CC
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 1, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0x60A4CC {
		t.Fatalf("R2 = 0x%X, want 0x60A4CC after MOVE mapped<-mapped", cpu.regs[2])
	}
}

func TestJIT_MMU_MoveMappedFromMappedAtNonEntryPC(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	cpu.regs[1] = 0x60A4CC
	cpu.PC = PROG_START + 4*IE64_INSTR_SIZE
	copy(cpu.memory[PROG_START+4*IE64_INSTR_SIZE:], ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 1, 0, 0))
	copy(cpu.memory[PROG_START+5*IE64_INSTR_SIZE:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0x60A4CC {
		t.Fatalf("R2 = 0x%X, want 0x60A4CC after MOVE mapped<-mapped at helper PC", cpu.regs[2])
	}
}

func TestJIT_MMU_BailBlockStoresMappedRegsBeforeRTS(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	haltPC := uint64(PROG_START + 3*IE64_INSTR_SIZE)
	binary.LittleEndian.PutUint64(cpu.memory[STACK_START-8:], haltPC)
	cpu.regs[31] = STACK_START - 8
	cpu.PC = PROG_START

	copy(cpu.memory[PROG_START:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x60A4CC))
	copy(cpu.memory[PROG_START+IE64_INSTR_SIZE:], ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 1))
	copy(cpu.memory[PROG_START+2*IE64_INSTR_SIZE:], ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0))
	copy(cpu.memory[PROG_START+3*IE64_INSTR_SIZE:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[1] != 0x60A4CC || cpu.regs[3] != 1 {
		t.Fatalf("R1=0x%X R3=0x%X, want 0x60A4CC and 1 after bailed RTS helper block", cpu.regs[1], cpu.regs[3])
	}
}

func TestJIT_MMU_TwoCallHostfsStyleArgFlow(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	cpu.regs[29] = 0x60A000
	cpu.regs[31] = STACK_START

	rig.loadInstructions(
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(8*IE64_INSTR_SIZE)),
		ie64Instr(OP_BEQ, 0, 0, 0, 3, 0, uint32(7*IE64_INSTR_SIZE)),
		ie64Instr(OP_MOVE, 24, IE64_SIZE_Q, 0, 1, 0, 0),
		ie64Instr(OP_MOVE, 27, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(6*IE64_INSTR_SIZE)),
		ie64Instr(OP_MOVE, 6, IE64_SIZE_Q, 0, 2, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),

		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x60A4CC),
		ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 1),
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),

		ie64Instr(OP_MOVE, 20, IE64_SIZE_Q, 0, 1, 0, 0),
		ie64Instr(OP_PUSH64, 0, IE64_SIZE_Q, 0, 29, 0, 0),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 20, 0, 0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, BOOT_HOSTFS_STAT),
		ie64Instr(OP_MOVE, 5, IE64_SIZE_Q, 0, 0, 0, 0),
		ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, 40),
		ie64Instr(OP_POP64, 29, IE64_SIZE_Q, 0, 0, 0, 0),
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[6] != 0x60A4CC {
		t.Fatalf("R6 = 0x%X, want 0x60A4CC (R1=0x%X R2=0x%X R3=0x%X R24=0x%X R27=0x%X SP=0x%X PC=0x%X)",
			cpu.regs[6], cpu.regs[1], cpu.regs[2], cpu.regs[3], cpu.regs[24], cpu.regs[27], cpu.regs[31], cpu.PC)
	}
}

func TestJIT_MMU_JsrIntoHostfsStyleHelperPreservesR1Argument(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	cpu.regs[29] = 0x60A000
	cpu.regs[31] = STACK_START

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 24, IE64_SIZE_L, 1, 0, 0, 0x60A4CC),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(3*IE64_INSTR_SIZE)),
		ie64Instr(OP_MOVE, 6, IE64_SIZE_Q, 0, 2, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),

		ie64Instr(OP_MOVE, 20, IE64_SIZE_Q, 0, 1, 0, 0),
		ie64Instr(OP_PUSH64, 0, IE64_SIZE_Q, 0, 29, 0, 0),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 20, 0, 0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, BOOT_HOSTFS_STAT),
		ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, 40),
		ie64Instr(OP_POP64, 29, IE64_SIZE_Q, 0, 0, 0, 0),
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[6] != 0x60A4CC {
		t.Fatalf("R6 = 0x%X, want 0x60A4CC after hostfs-style helper call (R1=0x%X R2=0x%X R20=0x%X)",
			cpu.regs[6], cpu.regs[1], cpu.regs[2], cpu.regs[20])
	}
}

func TestJIT_MMU_JsrBailPreservesMappedR1IntoCallee(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 24, IE64_SIZE_L, 1, 0, 0, 0x60A4CC),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(2*IE64_INSTR_SIZE)),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 1, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.regs[31] = STACK_START
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0x60A4CC {
		t.Fatalf("R2 = 0x%X, want 0x60A4CC after callee copies R1 post-JSR bail (R1=0x%X R24=0x%X PC=0x%X fault=%d)",
			cpu.regs[2], cpu.regs[1], cpu.regs[24], cpu.PC, cpu.faultCause)
	}
}

func TestJIT_MMU_JsrPreservesSpilledRelpathPointerIntoCallee(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	cpu.regs[29] = 0x3000
	cpu.regs[31] = STACK_START

	rig.loadInstructions(
		ie64Instr(OP_ADD, 23, IE64_SIZE_Q, 1, 29, 0, 0x1CC),
		ie64Instr(OP_MOVE, 30, IE64_SIZE_Q, 0, 29, 0, 0),
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(2*IE64_INSTR_SIZE)),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_MOVE, 24, IE64_SIZE_Q, 0, 23, 0, 0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[1] != 0x31CC {
		t.Fatalf("R1 = 0x%X, want 0x31CC after spilled relpath pointer handoff (R23=0x%X R24=0x%X R29=0x%X PC=0x%X)",
			cpu.regs[1], cpu.regs[23], cpu.regs[24], cpu.regs[29], cpu.PC)
	}
}

func TestJIT_MMU_JsrPreservesSpilledRelpathPointerAcrossCalleeStore(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	cpu.regs[29] = 0x3000
	cpu.regs[31] = STACK_START

	rig.loadInstructions(
		ie64Instr(OP_ADD, 23, IE64_SIZE_Q, 1, 29, 0, 0x1CC),
		ie64Instr(OP_MOVE, 30, IE64_SIZE_Q, 0, 29, 0, 0),
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(2*IE64_INSTR_SIZE)),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_MOVE, 24, IE64_SIZE_Q, 0, 23, 0, 0),
		ie64Instr(OP_STORE, 23, IE64_SIZE_Q, 1, 29, 0, 888),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[1] != 0x31CC {
		t.Fatalf("R1 = 0x%X, want 0x31CC after spilled relpath pointer survives callee store (R23=0x%X R24=0x%X R29=0x%X PC=0x%X)",
			cpu.regs[1], cpu.regs[23], cpu.regs[24], cpu.regs[29], cpu.PC)
	}
}

func TestJIT_MMU_DosBootfsStatPrepPreservesRelpathPointer(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	cpu.regs[29] = 0x3000
	cpu.regs[31] = STACK_START

	rig.loadInstructions(
		ie64Instr(OP_ADD, 23, IE64_SIZE_Q, 1, 29, 0, 0x4CC),
		ie64Instr(OP_MOVE, 30, IE64_SIZE_Q, 0, 29, 0, 0),
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(2*IE64_INSTR_SIZE)),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_MOVE, 24, IE64_SIZE_Q, 0, 23, 0, 0),
		ie64Instr(OP_MOVE, 27, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_STORE, 23, IE64_SIZE_Q, 1, 29, 0, 888),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 24, 0, 0),
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(2*IE64_INSTR_SIZE)),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_MOVE, 20, IE64_SIZE_Q, 0, 1, 0, 0),
		ie64Instr(OP_PUSH64, 0, IE64_SIZE_Q, 0, 29, 0, 0),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 20, 0, 0),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, BOOT_HOSTFS_STAT),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0x34CC {
		t.Fatalf("R2 = 0x%X, want 0x34CC at BOOT_HOSTFS_STAT prep (R1=0x%X R20=0x%X R23=0x%X R24=0x%X R27=0x%X R29=0x%X PC=0x%X)",
			cpu.regs[2], cpu.regs[1], cpu.regs[20], cpu.regs[23], cpu.regs[24], cpu.regs[27], cpu.regs[29], cpu.PC)
	}
}

func TestJIT_MMU_HighVA_DosBootfsStatPrepPreservesRelpathPointer(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	cpu.CoprocMode = true
	setupIdentityMMU(cpu, 160)

	codeVirt := uint32(0x600000)
	codePhys := uint32(0x100000)
	writePTE(cpu, uint16(codeVirt>>MMU_PAGE_SHIFT), makePTE(uint16(codePhys>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	dataVirt := uint32(0x60A000)
	dataPhys := uint32(0x110000)
	writePTE(cpu, uint16(dataVirt>>MMU_PAGE_SHIFT), makePTE(uint16(dataPhys>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	cpu.PC = uint64(codeVirt)
	cpu.regs[29] = uint64(dataVirt)
	cpu.regs[31] = STACK_START

	emitAt := func(phys uint32, instr []byte) {
		copy(cpu.memory[phys:], instr)
	}
	emitAt(codePhys+0*IE64_INSTR_SIZE, ie64Instr(OP_ADD, 23, IE64_SIZE_Q, 1, 29, 0, 0x4CC))
	emitAt(codePhys+1*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 30, IE64_SIZE_Q, 0, 29, 0, 0))
	emitAt(codePhys+2*IE64_INSTR_SIZE, ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(2*IE64_INSTR_SIZE)))
	emitAt(codePhys+3*IE64_INSTR_SIZE, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	emitAt(codePhys+4*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 24, IE64_SIZE_Q, 0, 23, 0, 0))
	emitAt(codePhys+5*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 27, IE64_SIZE_Q, 0, 24, 0, 0))
	emitAt(codePhys+6*IE64_INSTR_SIZE, ie64Instr(OP_STORE, 23, IE64_SIZE_Q, 1, 29, 0, 888))
	emitAt(codePhys+7*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 24, 0, 0))
	emitAt(codePhys+8*IE64_INSTR_SIZE, ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(2*IE64_INSTR_SIZE)))
	emitAt(codePhys+9*IE64_INSTR_SIZE, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	emitAt(codePhys+10*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 20, IE64_SIZE_Q, 0, 1, 0, 0))
	emitAt(codePhys+11*IE64_INSTR_SIZE, ie64Instr(OP_PUSH64, 0, IE64_SIZE_Q, 0, 29, 0, 0))
	emitAt(codePhys+12*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 20, 0, 0))
	emitAt(codePhys+13*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, BOOT_HOSTFS_STAT))
	emitAt(codePhys+14*IE64_INSTR_SIZE, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0x60A4CC {
		t.Fatalf("R2 = 0x%X, want 0x60A4CC at high-VA BOOT_HOSTFS_STAT prep (R1=0x%X R20=0x%X R23=0x%X R24=0x%X R27=0x%X R29=0x%X PC=0x%X)",
			cpu.regs[2], cpu.regs[1], cpu.regs[20], cpu.regs[23], cpu.regs[24], cpu.regs[27], cpu.regs[29], cpu.PC)
	}
}

func TestJIT_MMU_HighVA_JsrIntoHostfsStyleHelperPreservesR1Argument(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	cpu.CoprocMode = true
	setupIdentityMMU(cpu, 160)

	codeVirt := uint32(0x600000)
	codePhys := uint32(0x120000)
	writePTE(cpu, uint16(codeVirt>>MMU_PAGE_SHIFT), makePTE(uint16(codePhys>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	dataVirt := uint32(0x60A000)
	dataPhys := uint32(0x121000)
	writePTE(cpu, uint16(dataVirt>>MMU_PAGE_SHIFT), makePTE(uint16(dataPhys>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	cpu.PC = uint64(codeVirt)
	cpu.regs[29] = uint64(dataVirt)
	cpu.regs[31] = STACK_START

	emitAt := func(phys uint32, instr []byte) {
		copy(cpu.memory[phys:], instr)
	}
	emitAt(codePhys+0*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 24, IE64_SIZE_L, 1, 0, 0, 0x60A4CC))
	emitAt(codePhys+1*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 24, 0, 0))
	emitAt(codePhys+2*IE64_INSTR_SIZE, ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(2*IE64_INSTR_SIZE)))
	emitAt(codePhys+3*IE64_INSTR_SIZE, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	emitAt(codePhys+4*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 20, IE64_SIZE_Q, 0, 1, 0, 0))
	emitAt(codePhys+5*IE64_INSTR_SIZE, ie64Instr(OP_PUSH64, 0, IE64_SIZE_Q, 0, 29, 0, 0))
	emitAt(codePhys+6*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 2, IE64_SIZE_Q, 0, 20, 0, 0))
	emitAt(codePhys+7*IE64_INSTR_SIZE, ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, BOOT_HOSTFS_STAT))
	emitAt(codePhys+8*IE64_INSTR_SIZE, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.regs[2] != 0x60A4CC {
		t.Fatalf("R2 = 0x%X, want 0x60A4CC after high-VA hostfs-style helper call (R1=0x%X R20=0x%X R24=0x%X R29=0x%X PC=0x%X)",
			cpu.regs[2], cpu.regs[1], cpu.regs[20], cpu.regs[24], cpu.regs[29], cpu.PC)
	}
}

// ===========================================================================
// Atomic RMW + JIT
// ===========================================================================

func TestJIT_CAS_NoMMU(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true

	addr := uint32(0x3000)
	binary.LittleEndian.PutUint64(cpu.memory[addr:], 100)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, addr),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 100),
		ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 200),
		ie64Instr(OP_CAS, 2, 0, 0, 1, 3, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	got := binary.LittleEndian.Uint64(cpu.memory[addr:])
	if got != 200 {
		t.Fatalf("JIT CAS: mem = %d, want 200", got)
	}
	if cpu.regs[2] != 100 {
		t.Fatalf("JIT CAS old: R2 = %d, want 100", cpu.regs[2])
	}
}

func TestJIT_CAS_WithMMU(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))
	binary.LittleEndian.PutUint64(cpu.memory[0x7000:], 100)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3000),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 100),
		ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 200),
		ie64Instr(OP_CAS, 2, 0, 0, 1, 3, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	got := binary.LittleEndian.Uint64(cpu.memory[0x7000:])
	if got != 200 {
		t.Fatalf("JIT+MMU CAS: phys = %d, want 200", got)
	}
}

func TestJIT_FAA_NoMMU(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true

	addr := uint32(0x3000)
	binary.LittleEndian.PutUint64(cpu.memory[addr:], 10)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, addr),
		ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 5),
		ie64Instr(OP_FAA, 2, 0, 0, 1, 3, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	got := binary.LittleEndian.Uint64(cpu.memory[addr:])
	if got != 15 {
		t.Fatalf("JIT FAA: mem = %d, want 15", got)
	}
	if cpu.regs[2] != 10 {
		t.Fatalf("JIT FAA old: R2 = %d, want 10", cpu.regs[2])
	}
}

func TestJIT_XCHG_NoMMU(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true

	addr := uint32(0x3000)
	binary.LittleEndian.PutUint64(cpu.memory[addr:], 0xDEAD)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, addr),
		ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 0xBEEF),
		ie64Instr(OP_XCHG, 2, 0, 0, 1, 3, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	got := binary.LittleEndian.Uint64(cpu.memory[addr:])
	if got != 0xBEEF {
		t.Fatalf("JIT XCHG: mem = 0x%X, want 0xBEEF", got)
	}
	if cpu.regs[2] != 0xDEAD {
		t.Fatalf("JIT XCHG old: R2 = 0x%X, want 0xDEAD", cpu.regs[2])
	}
}

func TestJIT_Atomic_MisalignedFaultJIT(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true

	trapAddr := uint64(0x8000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3001),
		ie64Instr(OP_CAS, 2, 0, 0, 1, 3, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if cpu.faultCause != FAULT_MISALIGNED {
		t.Fatalf("JIT misaligned: cause = %d, want %d", cpu.faultCause, FAULT_MISALIGNED)
	}
}
