// iexec_test.go - IExec microkernel integration tests

package main

import (
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// ===========================================================================
// IExec Kernel Constants
// ===========================================================================

const (
	// Kernel memory layout (all in identity-mapped supervisor space)
	kernPageTableBase = 0x10000 // Kernel page table (64 KiB)
	kernDataBase      = 0x20000 // Kernel data (TCBs, state)
	kernStackTop      = 0x9F000 // Kernel stack top

	// User task page table bases (separate page tables per task)
	userPT0Base = 0x30000 // Task 0 page table
	userPT1Base = 0x40000 // Task 1 page table

	// User task physical pages (mapped into user virtual space)
	userTask0Code  = 0x600000 // Task 0 code (physical = virtual, identity for simplicity)
	userTask0Stack = 0x601000 // Task 0 stack (1 page)
	userTask0Data  = 0x602000 // Task 0 data/result page
	userTask1Code  = 0x610000 // Task 1 code
	userTask1Stack = 0x611000 // Task 1 stack
	userTask1Data  = 0x612000 // Task 1 data/result page

	// Syscall numbers (matching IExec contract)
	sysYield      = 26
	sysGetSysInfo = 27

	// Kernel data offsets
	kdCurrentTask = 0  // uint64: index of current task (0 or 1)
	kdTickCount   = 8  // uint64: tick counter
	kdNumTasks    = 16 // uint64: number of active tasks

	// TCB offsets within kernel data (after the globals)
	kdTCBBase   = 64  // start of TCB array
	tcbSize     = 288 // per-TCB size
	tcbRegsOff  = 0   // R0-R31 (256 bytes)
	tcbPCOff    = 256 // saved PC (8 bytes)
	tcbPTBROff  = 264 // PTBR (4 bytes)
	tcbStateOff = 268 // state byte
	tcbUSPOff   = 272 // saved userSP (8 bytes)
	tcbPadOff   = 280 // padding to 288
)

// ===========================================================================
// Kernel Builder
// ===========================================================================

type iexecKernel struct {
	code []byte
}

func newIExecKernel() *iexecKernel {
	return &iexecKernel{code: make([]byte, 0, 16384)}
}

func (k *iexecKernel) emit(instrs ...[]byte) uint32 {
	off := uint32(len(k.code))
	for _, instr := range instrs {
		k.code = append(k.code, instr...)
	}
	return off
}

func (k *iexecKernel) addr() uint32 {
	return PROG_START + uint32(len(k.code))
}

// padTo advances the code to a specific offset from PROG_START
func (k *iexecKernel) padTo(targetOff uint32) {
	current := uint32(len(k.code))
	if current < targetOff {
		k.code = append(k.code, make([]byte, targetOff-current)...)
	}
}

// setupPageTable writes identity-mapped kernel PTEs (0-383, supervisor-only)
// into the CPU memory at the given base address. Also maps specified user pages.
func setupKernelPTEs(mem []byte, ptBase uint32) {
	// Identity-map pages 0-383 (up to $180000 = kernel + IO + partial VRAM)
	// with P|R|W|X, no U (supervisor only)
	for page := uint16(0); page < 384; page++ {
		pte := makePTE(page, PTE_P|PTE_R|PTE_W|PTE_X)
		off := ptBase + uint32(page)*8
		binary.LittleEndian.PutUint64(mem[off:], pte)
	}
	// Also map VRAM region pages 384-1535 ($180000-$5FFFFF) supervisor-only
	for page := uint16(384); page < 1536; page++ {
		pte := makePTE(page, PTE_P|PTE_R|PTE_W)
		off := ptBase + uint32(page)*8
		binary.LittleEndian.PutUint64(mem[off:], pte)
	}
}

// mapUserPage adds a user-accessible PTE to a page table
func mapUserPage(mem []byte, ptBase uint32, vpn, ppn uint16, flags byte) {
	pte := makePTE(ppn, flags)
	off := ptBase + uint32(vpn)*8
	binary.LittleEndian.PutUint64(mem[off:], pte)
}

// ===========================================================================
// Kernel Binary Builders (for different test phases)
// ===========================================================================

// buildBootOnlyKernel: sets up vectors, page table, enables MMU, halts.
func buildBootOnlyKernel() *iexecKernel {
	k := newIExecKernel()

	// Set trap vector (0x3000 offset from PROG_START)
	trapAddr := uint32(PROG_START) + 0x3000
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, trapAddr))
	k.emit(ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0))

	// Set interrupt vector
	intrAddr := uint32(PROG_START) + 0x4000
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, intrAddr))
	k.emit(ie64Instr(OP_MTCR, CR_INTR_VEC, 0, 0, 1, 0, 0))

	// Set kernel stack pointer
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernStackTop))
	k.emit(ie64Instr(OP_MTCR, CR_KSP, 0, 0, 1, 0, 0))

	// Build page table inline
	k.emit(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, kernPageTableBase))
	k.emit(ie64Instr(OP_MOVE, 4, IE64_SIZE_L, 1, 0, 0, 0)) // counter

	loopStart := k.addr()
	k.emit(ie64Instr(OP_LSL, 3, IE64_SIZE_Q, 1, 4, 0, 13))    // R3 = R4 << 13
	k.emit(ie64Instr(OP_OR64, 3, IE64_SIZE_Q, 1, 3, 0, 0x0F)) // R3 |= P|R|W|X
	k.emit(ie64Instr(OP_LSL, 5, IE64_SIZE_Q, 1, 4, 0, 3))     // R5 = R4 * 8
	k.emit(ie64Instr(OP_ADD, 5, IE64_SIZE_Q, 0, 5, 2, 0))     // R5 += ptBase
	k.emit(ie64Instr(OP_STORE, 3, IE64_SIZE_Q, 0, 5, 0, 0))   // [R5] = R3
	k.emit(ie64Instr(OP_ADD, 4, IE64_SIZE_Q, 1, 4, 0, 1))     // R4++
	k.emit(ie64Instr(OP_MOVE, 6, IE64_SIZE_L, 1, 0, 0, 384))
	branchAddr := k.addr()
	k.emit(ie64Instr(OP_BLT, 0, 0, 0, 4, 6, uint32(int32(loopStart)-int32(branchAddr))))

	// Enable MMU
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernPageTableBase))
	k.emit(ie64Instr(OP_MTCR, CR_PTBR, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 1))
	k.emit(ie64Instr(OP_MTCR, CR_MMU_CTRL, 0, 0, 1, 0, 0))

	k.emit(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	return k
}

// buildYieldKernel: boots, creates one user task, user does Yield then writes marker.
// Trap handler handles Yield (returns immediately via ERET) and faults (halts).
func buildYieldKernel(mem []byte) *iexecKernel {
	k := newIExecKernel()

	// --- Boot sequence (same as boot-only but continues instead of halting) ---

	trapAddr := uint32(PROG_START) + 0x3000
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, trapAddr))
	k.emit(ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0))

	intrAddr := uint32(PROG_START) + 0x4000
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, intrAddr))
	k.emit(ie64Instr(OP_MTCR, CR_INTR_VEC, 0, 0, 1, 0, 0))

	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernStackTop))
	k.emit(ie64Instr(OP_MTCR, CR_KSP, 0, 0, 1, 0, 0))

	// Set up page table in memory directly (faster than building in assembly)
	setupKernelPTEs(mem, kernPageTableBase)
	// Also set up in the user page table (copy kernel mappings + add user pages)
	setupKernelPTEs(mem, userPT0Base)
	userCodeVPN := uint16(userTask0Code >> MMU_PAGE_SHIFT)
	userStackVPN := uint16(userTask0Stack >> MMU_PAGE_SHIFT)
	userDataVPN := uint16(userTask0Data >> MMU_PAGE_SHIFT)
	mapUserPage(mem, userPT0Base, userCodeVPN, userCodeVPN, PTE_P|PTE_R|PTE_X|PTE_U)
	mapUserPage(mem, userPT0Base, userStackVPN, userStackVPN, PTE_P|PTE_R|PTE_W|PTE_U)
	mapUserPage(mem, userPT0Base, userDataVPN, userDataVPN, PTE_P|PTE_R|PTE_W|PTE_U)

	// Enable MMU with kernel page table first
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernPageTableBase))
	k.emit(ie64Instr(OP_MTCR, CR_PTBR, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 1))
	k.emit(ie64Instr(OP_MTCR, CR_MMU_CTRL, 0, 0, 1, 0, 0))

	// Switch to user task 0's page table
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userPT0Base))
	k.emit(ie64Instr(OP_MTCR, CR_PTBR, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_TLBFLUSH, 0, 0, 0, 0, 0, 0))

	// Set USP to user stack top and FAULT_PC to user code entry
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Stack+MMU_PAGE_SIZE)) // stack top
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Code))
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 1, 0, 0))

	// ERET to enter user mode at userTask0Code
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// --- Trap handler at offset 0x3000 ---
	k.padTo(0x3000)

	// Read fault cause
	k.emit(ie64Instr(OP_MFCR, 1, 0, 0, CR_FAULT_CAUSE, 0, 0))

	// Is it a SYSCALL?
	k.emit(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, FAULT_SYSCALL))
	syscallBranch := k.addr()
	k.emit(ie64Instr(OP_BEQ, 0, 0, 0, 1, 2, 0)) // patched below

	// Not a syscall → fault → halt
	k.emit(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Syscall handler
	syscallAddr := k.addr()
	// Patch the BEQ branch
	binary.LittleEndian.PutUint32(k.code[syscallBranch-PROG_START+4:], uint32(int32(syscallAddr)-int32(syscallBranch)))

	// Read syscall number from FAULT_ADDR
	k.emit(ie64Instr(OP_MFCR, 1, 0, 0, CR_FAULT_ADDR, 0, 0))

	// Is it Yield (#26)?
	k.emit(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, sysYield))
	yieldBranch := k.addr()
	k.emit(ie64Instr(OP_BEQ, 0, 0, 0, 1, 2, 0)) // patched below

	// Unknown syscall → return with error
	k.emit(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 3)) // ERR_BADARG
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Yield handler: just return via ERET (single task, no context switch yet)
	yieldAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[yieldBranch-PROG_START+4:], uint32(int32(yieldAddr)-int32(yieldBranch)))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// --- User task 0 code (written directly to memory) ---
	// At userTask0Code: SYSCALL #26 (Yield), then write 0xAAAA to data page, then HALT
	userCode := []byte{}
	userCode = append(userCode, ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield)...)
	userCode = append(userCode, ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Data)...)
	userCode = append(userCode, ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xAAAA)...)
	userCode = append(userCode, ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0)...)
	userCode = append(userCode, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)...)
	copy(mem[userTask0Code:], userCode)

	return k
}

// loadKernel loads kernel code and runs via Execute with bounded cycles
func loadAndRunKernel(t *testing.T, rig *ie64TestRig, k *iexecKernel, maxCycles int) {
	t.Helper()
	copy(rig.cpu.memory[PROG_START:], k.code)
	rig.cpu.PC = PROG_START
	rig.cpu.running.Store(true)
	rig.cpu.CoprocMode = true // allow PC in user space range

	for i := 0; i < maxCycles; i++ {
		if !rig.cpu.running.Load() {
			break
		}
		// Check for HALT opcode at current PC to handle StepOne not setting running=false
		pc := uint32(rig.cpu.PC)
		if pc+8 <= uint32(len(rig.cpu.memory)) && rig.cpu.memory[pc] == OP_HALT64 {
			break
		}
		rig.cpu.StepOne()
	}
}

// ===========================================================================
// Phase B1: Boot Constants
// ===========================================================================

func TestIExec_BootConstantsDefined(t *testing.T) {
	if EXEC_OP_IEXEC != 4 {
		t.Fatalf("EXEC_OP_IEXEC = %d, want 4", EXEC_OP_IEXEC)
	}
	if EXEC_TYPE_IEXEC != 10 {
		t.Fatalf("EXEC_TYPE_IEXEC = %d, want 10", EXEC_TYPE_IEXEC)
	}
}

// ===========================================================================
// Phase B2: Kernel Boot + MMU Enable
// ===========================================================================

func TestIExec_KernelBoots(t *testing.T) {
	k := buildBootOnlyKernel()
	rig := newIE64TestRig()
	loadAndRunKernel(t, rig, k, 100000)

	if !rig.cpu.mmuEnabled {
		t.Fatal("MMU should be enabled after kernel boot")
	}
	if rig.cpu.trapVector == 0 {
		t.Fatal("trap vector should be set")
	}
	if rig.cpu.intrVector == 0 {
		t.Fatal("interrupt vector should be set")
	}
	if rig.cpu.kernelSP == 0 {
		t.Fatal("kernel SP should be set")
	}
}

func TestIExec_KernelPageTable(t *testing.T) {
	k := buildBootOnlyKernel()
	rig := newIE64TestRig()
	loadAndRunKernel(t, rig, k, 100000)

	// Kernel pages (0-383) should be mapped with P|R|W|X, no U
	for page := uint16(0); page < 384; page++ {
		pteAddr := uint32(kernPageTableBase) + uint32(page)*8
		pte := binary.LittleEndian.Uint64(rig.cpu.memory[pteAddr:])
		ppn, flags := parsePTE(pte)

		if ppn != page {
			t.Fatalf("page %d: PPN=%d, want identity map", page, ppn)
		}
		if flags&PTE_P == 0 {
			t.Fatalf("page %d: not present", page)
		}
		if flags&PTE_U != 0 {
			t.Fatalf("page %d: U bit set (should be supervisor-only)", page)
		}
	}

	// User pages (1536+) should be unmapped
	pteAddr := uint32(kernPageTableBase) + 1536*8
	pte := binary.LittleEndian.Uint64(rig.cpu.memory[pteAddr:])
	_, flags := parsePTE(pte)
	if flags&PTE_P != 0 {
		t.Fatal("first user page (1536) should not be present in kernel page table")
	}
}

// ===========================================================================
// Phase B3: Trap Handler + Yield
// ===========================================================================

func TestIExec_YieldReturns(t *testing.T) {
	rig := newIE64TestRig()
	k := buildYieldKernel(rig.cpu.memory)
	loadAndRunKernel(t, rig, k, 200000)

	// After Yield returns, user code writes 0xAAAA to data page
	marker := binary.LittleEndian.Uint32(rig.cpu.memory[userTask0Data:])
	if marker != 0xAAAA {
		t.Fatalf("user task marker = 0x%X, want 0xAAAA (Yield should have returned)", marker)
	}
}

func TestIExec_FaultKillsTask(t *testing.T) {
	rig := newIE64TestRig()

	// Build a kernel that enters user mode at an unmapped page
	k := newIExecKernel()

	trapAddr := uint32(PROG_START) + 0x3000
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, trapAddr))
	k.emit(ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernStackTop))
	k.emit(ie64Instr(OP_MTCR, CR_KSP, 0, 0, 1, 0, 0))

	// Set up page table with kernel pages only
	setupKernelPTEs(rig.cpu.memory, kernPageTableBase)

	// Enable MMU
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernPageTableBase))
	k.emit(ie64Instr(OP_MTCR, CR_PTBR, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 1))
	k.emit(ie64Instr(OP_MTCR, CR_MMU_CTRL, 0, 0, 1, 0, 0))

	// Try to ERET to unmapped user page (will fault on instruction fetch)
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x700000)) // unmapped address
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x700000))
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Trap handler: on fault, write cause to a known location and halt
	k.padTo(0x3000)
	k.emit(ie64Instr(OP_MFCR, 1, 0, 0, CR_FAULT_CAUSE, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, kernDataBase))
	k.emit(ie64Instr(OP_STORE, 1, IE64_SIZE_L, 0, 2, 0, 0)) // store cause at kernDataBase
	k.emit(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	loadAndRunKernel(t, rig, k, 200000)

	// Check that the fault cause was stored
	cause := binary.LittleEndian.Uint32(rig.cpu.memory[kernDataBase:])
	if cause != FAULT_NOT_PRESENT {
		t.Fatalf("fault cause = %d, want %d (NOT_PRESENT)", cause, FAULT_NOT_PRESENT)
	}
}

// ===========================================================================
// Phase B4: Two Tasks + Context Switch
// ===========================================================================

// buildTwoTaskKernel creates a kernel with two user tasks that yield to each other.
// Task 0 writes 0xAAAA to userTask0Data, yields, writes 0xBBBB, halts.
// Task 1 writes 0xCCCC to userTask1Data, yields, writes 0xDDDD, halts.
// The scheduler alternates: task0 → yield → task1 → yield → task0 → halt.
func buildTwoTaskKernel(mem []byte) *iexecKernel {
	k := newIExecKernel()

	// --- Boot: set vectors, KSP ---
	trapAddr := uint32(PROG_START) + 0x3000
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, trapAddr))
	k.emit(ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernStackTop))
	k.emit(ie64Instr(OP_MTCR, CR_KSP, 0, 0, 1, 0, 0))

	// Set up both page tables in memory
	setupKernelPTEs(mem, kernPageTableBase) // kernel PT
	setupKernelPTEs(mem, userPT0Base)       // task 0 PT
	setupKernelPTEs(mem, userPT1Base)       // task 1 PT

	// Task 0: map code, stack, data pages
	mapUserPage(mem, userPT0Base, uint16(userTask0Code>>MMU_PAGE_SHIFT), uint16(userTask0Code>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_X|PTE_U)
	mapUserPage(mem, userPT0Base, uint16(userTask0Stack>>MMU_PAGE_SHIFT), uint16(userTask0Stack>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_U)
	mapUserPage(mem, userPT0Base, uint16(userTask0Data>>MMU_PAGE_SHIFT), uint16(userTask0Data>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_U)

	// Task 1: map code, stack, data pages
	mapUserPage(mem, userPT1Base, uint16(userTask1Code>>MMU_PAGE_SHIFT), uint16(userTask1Code>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_X|PTE_U)
	mapUserPage(mem, userPT1Base, uint16(userTask1Stack>>MMU_PAGE_SHIFT), uint16(userTask1Stack>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_U)
	mapUserPage(mem, userPT1Base, uint16(userTask1Data>>MMU_PAGE_SHIFT), uint16(userTask1Data>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_U)

	// Initialize kernel data structures directly in memory (before MMU)
	kdBase := uint32(kernDataBase)
	binary.LittleEndian.PutUint64(mem[kdBase+kdCurrentTask:], 0)
	binary.LittleEndian.PutUint64(mem[kdBase+kdNumTasks:], 2)

	// TCB 0
	tcb0 := kdBase + kdTCBBase
	binary.LittleEndian.PutUint64(mem[tcb0+tcbPCOff:], userTask0Code)
	binary.LittleEndian.PutUint32(mem[tcb0+tcbPTBROff:], userPT0Base)
	binary.LittleEndian.PutUint64(mem[tcb0+tcbUSPOff:], userTask0Stack+MMU_PAGE_SIZE)
	mem[tcb0+tcbStateOff] = 0 // READY

	// TCB 1
	tcb1 := kdBase + kdTCBBase + tcbSize
	binary.LittleEndian.PutUint64(mem[tcb1+tcbPCOff:], userTask1Code)
	binary.LittleEndian.PutUint32(mem[tcb1+tcbPTBROff:], userPT1Base)
	binary.LittleEndian.PutUint64(mem[tcb1+tcbUSPOff:], userTask1Stack+MMU_PAGE_SIZE)
	mem[tcb1+tcbStateOff] = 0 // READY

	// --- Enter first task via ERET (no MMU for now — test scheduler logic first) ---
	// Set USP for task 0
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Stack+MMU_PAGE_SIZE))
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Code))
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// --- Trap handler at offset 0x3000 ---
	k.padTo(0x3000)

	// Read fault cause
	k.emit(ie64Instr(OP_MFCR, 10, 0, 0, CR_FAULT_CAUSE, 0, 0))

	// Is it SYSCALL?
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, FAULT_SYSCALL))
	trapSyscallBranch := k.addr()
	k.emit(ie64Instr(OP_BEQ, 0, 0, 0, 10, 11, 0)) // patched

	// Not syscall → halt (fault)
	k.emit(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// --- Syscall dispatch ---
	syscallEntry := k.addr()
	binary.LittleEndian.PutUint32(k.code[trapSyscallBranch-PROG_START+4:], uint32(int32(syscallEntry)-int32(trapSyscallBranch)))

	k.emit(ie64Instr(OP_MFCR, 10, 0, 0, CR_FAULT_ADDR, 0, 0)) // syscall number

	// Is it Yield?
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, sysYield))
	yieldBranch := k.addr()
	k.emit(ie64Instr(OP_BEQ, 0, 0, 0, 10, 11, 0)) // patched

	// Unknown syscall → ERET with error
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// --- Yield handler: context switch ---
	yieldEntry := k.addr()
	binary.LittleEndian.PutUint32(k.code[yieldBranch-PROG_START+4:], uint32(int32(yieldEntry)-int32(yieldBranch)))

	// Save current task's state:
	// 1. Read current_task index
	k.emit(ie64Instr(OP_MOVE, 20, IE64_SIZE_L, 1, 0, 0, kdBase+kdCurrentTask))
	k.emit(ie64Instr(OP_LOAD, 20, IE64_SIZE_Q, 0, 20, 0, 0)) // R20 = current task index

	// 2. Compute TCB address: R21 = kdBase + kdTCBBase + R20 * tcbSize
	k.emit(ie64Instr(OP_MULU, 21, IE64_SIZE_Q, 1, 20, 0, tcbSize))
	k.emit(ie64Instr(OP_ADD, 21, IE64_SIZE_Q, 1, 21, 0, kdBase+kdTCBBase))

	// 3. Save FAULT_PC (user's return address) to TCB
	k.emit(ie64Instr(OP_MFCR, 22, 0, 0, CR_FAULT_PC, 0, 0))
	k.emit(ie64Instr(OP_STORE, 22, IE64_SIZE_Q, 1, 21, 0, tcbPCOff))

	// 4. Save USP to TCB
	k.emit(ie64Instr(OP_MFCR, 22, 0, 0, CR_USP, 0, 0))
	k.emit(ie64Instr(OP_STORE, 22, IE64_SIZE_Q, 1, 21, 0, tcbUSPOff))

	// 5. Switch to next task: next = (current + 1) % 2
	k.emit(ie64Instr(OP_ADD, 20, IE64_SIZE_Q, 1, 20, 0, 1))   // R20 = current + 1
	k.emit(ie64Instr(OP_AND64, 20, IE64_SIZE_Q, 1, 20, 0, 1)) // R20 = R20 & 1 (mod 2)

	// 6. Store new current_task
	k.emit(ie64Instr(OP_MOVE, 22, IE64_SIZE_L, 1, 0, 0, kdBase+kdCurrentTask))
	k.emit(ie64Instr(OP_STORE, 20, IE64_SIZE_Q, 0, 22, 0, 0))

	// 7. Compute next TCB address
	k.emit(ie64Instr(OP_MULU, 21, IE64_SIZE_Q, 1, 20, 0, tcbSize))
	k.emit(ie64Instr(OP_ADD, 21, IE64_SIZE_Q, 1, 21, 0, kdBase+kdTCBBase))

	// 8. Load next task's PTBR and switch
	k.emit(ie64Instr(OP_LOAD, 22, IE64_SIZE_L, 1, 21, 0, tcbPTBROff))
	k.emit(ie64Instr(OP_MTCR, CR_PTBR, 0, 0, 22, 0, 0))
	k.emit(ie64Instr(OP_TLBFLUSH, 0, 0, 0, 0, 0, 0))

	// 9. Load next task's USP
	k.emit(ie64Instr(OP_LOAD, 22, IE64_SIZE_Q, 1, 21, 0, tcbUSPOff))
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 22, 0, 0))

	// 10. Load next task's PC and ERET
	k.emit(ie64Instr(OP_LOAD, 22, IE64_SIZE_Q, 1, 21, 0, tcbPCOff))
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 22, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// --- User task 0 code ---
	// Write 0xAAAA, yield, write 0xBBBB, halt
	userCode0 := []byte{}
	userCode0 = append(userCode0, ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Data)...)
	userCode0 = append(userCode0, ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xAAAA)...)
	userCode0 = append(userCode0, ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0)...)
	userCode0 = append(userCode0, ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield)...)
	userCode0 = append(userCode0, ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xBBBB)...)
	userCode0 = append(userCode0, ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 4)...) // offset +4
	userCode0 = append(userCode0, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)...)
	copy(mem[userTask0Code:], userCode0)

	// --- User task 1 code ---
	// Write 0xCCCC, yield, write 0xDDDD, halt
	userCode1 := []byte{}
	userCode1 = append(userCode1, ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask1Data)...)
	userCode1 = append(userCode1, ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xCCCC)...)
	userCode1 = append(userCode1, ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0)...)
	userCode1 = append(userCode1, ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield)...)
	userCode1 = append(userCode1, ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xDDDD)...)
	userCode1 = append(userCode1, ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 4)...)
	userCode1 = append(userCode1, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)...)
	copy(mem[userTask1Code:], userCode1)

	return k
}

// TestIExec_YieldHandlerStore tests that the yield handler can write to kernel data
func TestIExec_YieldHandlerStore(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu

	// Pre-store current_task = 0 at kernDataBase
	binary.LittleEndian.PutUint64(cpu.memory[kernDataBase:], 0)

	// Trap handler: on SYSCALL, write 42 to kernDataBase, then HALT
	trapAddr := uint32(PROG_START) + 0x3000
	cpu.trapVector = uint64(trapAddr)
	cpu.kernelSP = kernStackTop

	// Simple user task: just SYSCALL
	userPC := uint32(userTask0Code)
	copy(cpu.memory[userPC:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))

	// Kernel code: set vectors, ERET to user
	k := newIExecKernel()
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, trapAddr))
	k.emit(ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernStackTop))
	k.emit(ie64Instr(OP_MTCR, CR_KSP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Stack+MMU_PAGE_SIZE))
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userPC))
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Trap handler: write 42 to kernDataBase then HALT
	k.padTo(0x3000)
	k.emit(ie64Instr(OP_MOVE, 20, IE64_SIZE_L, 1, 0, 0, 42))
	k.emit(ie64Instr(OP_MOVE, 21, IE64_SIZE_L, 1, 0, 0, kernDataBase))
	k.emit(ie64Instr(OP_STORE, 20, IE64_SIZE_Q, 0, 21, 0, 0))
	k.emit(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	loadAndRunKernel(t, rig, k, 100000)

	val := binary.LittleEndian.Uint64(cpu.memory[kernDataBase:])
	if val != 42 {
		t.Fatalf("trap handler store: got %d, want 42 (supervisorMode=%v, PC=0x%X)", val, cpu.supervisorMode, cpu.PC)
	}
}

func TestIExec_TwoTasksRun(t *testing.T) {
	rig := newIE64TestRig()
	mem := rig.cpu.memory
	k := newIExecKernel()

	// --- Set up data in memory (host-side) ---
	// current_task = 0
	binary.LittleEndian.PutUint64(mem[kernDataBase:], 0)

	// User task code
	// Task 0: write 0xAAAA, yield, reload regs, write 0xBBBB at +4, HALT
	off := uint32(userTask0Code)
	copy(mem[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Data))
	off += 8
	copy(mem[off:], ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xAAAA))
	off += 8
	copy(mem[off:], ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0))
	off += 8
	copy(mem[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	off += 8
	// After yield, reload R1 (GPRs not preserved across context switch yet)
	copy(mem[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Data))
	off += 8
	copy(mem[off:], ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xBBBB))
	off += 8
	copy(mem[off:], ie64Instr(OP_STORE, 2, IE64_SIZE_L, 1, 1, 0, 4))
	off += 8
	copy(mem[off:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Task 1: write 0xCCCC, yield, reload regs, write 0xDDDD at +4, HALT
	off = uint32(userTask1Code)
	copy(mem[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask1Data))
	off += 8
	copy(mem[off:], ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xCCCC))
	off += 8
	copy(mem[off:], ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0))
	off += 8
	copy(mem[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	off += 8
	copy(mem[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask1Data))
	off += 8
	copy(mem[off:], ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xDDDD))
	off += 8
	copy(mem[off:], ie64Instr(OP_STORE, 2, IE64_SIZE_L, 1, 1, 0, 4))
	off += 8
	copy(mem[off:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// --- Kernel boot code ---
	trapAddr := uint32(PROG_START) + 0x3000
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, trapAddr))
	k.emit(ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernStackTop))
	k.emit(ie64Instr(OP_MTCR, CR_KSP, 0, 0, 1, 0, 0))

	// ERET to task 0
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Stack+MMU_PAGE_SIZE))
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Code))
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// --- Trap handler at 0x3000 ---
	// Simple hardcoded scheduler: toggle between task 0 and task 1
	k.padTo(0x3000)

	// Check cause = SYSCALL
	k.emit(ie64Instr(OP_MFCR, 10, 0, 0, CR_FAULT_CAUSE, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, FAULT_SYSCALL))
	faultBranch := k.addr()
	k.emit(ie64Instr(OP_BNE, 0, 0, 0, 10, 11, 0)) // if not SYSCALL, jump to halt
	// *** SYSCALL handler ***

	// Save current task's return PC (FAULT_PC) and USP
	// R12 = &current_task
	k.emit(ie64Instr(OP_MOVE, 12, IE64_SIZE_L, 1, 0, 0, kernDataBase))
	k.emit(ie64Instr(OP_LOAD, 13, IE64_SIZE_Q, 0, 12, 0, 0)) // R13 = current_task (0 or 1)

	// Save FAULT_PC to task's PC slot in data area
	// task 0 PC at kernDataBase+16, task 1 PC at kernDataBase+32
	k.emit(ie64Instr(OP_MFCR, 14, 0, 0, CR_FAULT_PC, 0, 0))
	k.emit(ie64Instr(OP_LSL, 15, IE64_SIZE_Q, 1, 13, 0, 4))               // R15 = task * 16
	k.emit(ie64Instr(OP_ADD, 15, IE64_SIZE_Q, 1, 15, 0, kernDataBase+16)) // R15 = &tcb[task].pc
	k.emit(ie64Instr(OP_STORE, 14, IE64_SIZE_Q, 0, 15, 0, 0))             // save PC

	// Save USP
	k.emit(ie64Instr(OP_MFCR, 14, 0, 0, CR_USP, 0, 0))
	k.emit(ie64Instr(OP_STORE, 14, IE64_SIZE_Q, 1, 15, 0, 8)) // save USP at +8

	// Toggle task: next = 1 - current
	k.emit(ie64Instr(OP_MOVE, 16, IE64_SIZE_L, 1, 0, 0, 1))
	k.emit(ie64Instr(OP_SUB, 13, IE64_SIZE_Q, 0, 16, 13, 0)) // R13 = 1 - current

	// Store new current_task
	k.emit(ie64Instr(OP_STORE, 13, IE64_SIZE_Q, 0, 12, 0, 0))

	// Load next task's PC and USP
	k.emit(ie64Instr(OP_LSL, 15, IE64_SIZE_Q, 1, 13, 0, 4)) // R15 = next * 16
	k.emit(ie64Instr(OP_ADD, 15, IE64_SIZE_Q, 1, 15, 0, kernDataBase+16))
	k.emit(ie64Instr(OP_LOAD, 14, IE64_SIZE_Q, 0, 15, 0, 0)) // load PC
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 14, 0, 0))
	k.emit(ie64Instr(OP_LOAD, 14, IE64_SIZE_Q, 1, 15, 0, 8)) // load USP
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 14, 0, 0))

	// ERET to next task
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Fault handler: HALT
	faultAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[faultBranch-PROG_START+4:], uint32(int32(faultAddr)-int32(faultBranch)))
	k.emit(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Initialize task state in memory (simplified: just PC and USP per task)
	// Task 0: PC=userTask0Code, USP=stack top (at kernDataBase+16)
	binary.LittleEndian.PutUint64(mem[kernDataBase+16:], userTask0Code)
	binary.LittleEndian.PutUint64(mem[kernDataBase+24:], userTask0Stack+MMU_PAGE_SIZE)
	// Task 1: PC=userTask1Code, USP=stack top (at kernDataBase+32)
	binary.LittleEndian.PutUint64(mem[kernDataBase+32:], userTask1Code)
	binary.LittleEndian.PutUint64(mem[kernDataBase+40:], userTask1Stack+MMU_PAGE_SIZE)

	loadAndRunKernel(t, rig, k, 2000000)

	// Task 0 should have written 0xAAAA
	marker0 := binary.LittleEndian.Uint32(rig.cpu.memory[userTask0Data:])
	if marker0 != 0xAAAA {
		t.Fatalf("task 0 marker = 0x%X, want 0xAAAA", marker0)
	}

	// Task 1 should have written 0xCCCC
	marker1 := binary.LittleEndian.Uint32(rig.cpu.memory[userTask1Data:])
	if marker1 != 0xCCCC {
		t.Fatalf("task 1 marker = 0x%X, want 0xCCCC", marker1)
	}

	// Flow: task0 writes 0xAAAA → yield → task1 writes 0xCCCC → yield
	//     → task0 writes 0xBBBB → halt (CPU stops, task1 doesn't get 2nd turn)

	marker0b := binary.LittleEndian.Uint32(rig.cpu.memory[userTask0Data+4:])
	if marker0b != 0xBBBB {
		t.Fatalf("task 0 second marker = 0x%X, want 0xBBBB", marker0b)
	}
	// Task 1 only ran once (task 0 halted before task 1's second turn)
	marker1a := binary.LittleEndian.Uint32(rig.cpu.memory[userTask1Data:])
	if marker1a != 0xCCCC {
		t.Fatalf("task 1 first marker = 0x%X, want 0xCCCC", marker1a)
	}
}

// ===========================================================================
// Phase B5: Timer Preemption
// ===========================================================================

func TestIExec_TimerPreemption(t *testing.T) {
	rig := newIE64TestRig()
	mem := rig.cpu.memory

	// Set up page tables (host-side for simplicity)
	setupKernelPTEs(mem, kernPageTableBase)
	setupKernelPTEs(mem, userPT0Base)
	setupKernelPTEs(mem, userPT1Base)

	// Map user pages for both tasks
	mapUserPage(mem, userPT0Base, uint16(userTask0Code>>MMU_PAGE_SHIFT), uint16(userTask0Code>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_X|PTE_U)
	mapUserPage(mem, userPT0Base, uint16(userTask0Stack>>MMU_PAGE_SHIFT), uint16(userTask0Stack>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_U)
	mapUserPage(mem, userPT0Base, uint16(userTask0Data>>MMU_PAGE_SHIFT), uint16(userTask0Data>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_U)
	mapUserPage(mem, userPT1Base, uint16(userTask1Code>>MMU_PAGE_SHIFT), uint16(userTask1Code>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_X|PTE_U)
	mapUserPage(mem, userPT1Base, uint16(userTask1Stack>>MMU_PAGE_SHIFT), uint16(userTask1Stack>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_U)
	mapUserPage(mem, userPT1Base, uint16(userTask1Data>>MMU_PAGE_SHIFT), uint16(userTask1Data>>MMU_PAGE_SHIFT), PTE_P|PTE_R|PTE_W|PTE_U)

	// Initialize scheduler state
	binary.LittleEndian.PutUint64(mem[kernDataBase:], 0) // current_task = 0
	// Task state: PC and USP at kernDataBase+16 (task0) and kernDataBase+32 (task1)
	binary.LittleEndian.PutUint64(mem[kernDataBase+16:], userTask0Code)
	binary.LittleEndian.PutUint64(mem[kernDataBase+24:], userTask0Stack+MMU_PAGE_SIZE)
	binary.LittleEndian.PutUint64(mem[kernDataBase+32:], userTask1Code)
	binary.LittleEndian.PutUint64(mem[kernDataBase+40:], userTask1Stack+MMU_PAGE_SIZE)
	// PTBR per task at kernDataBase+48 (task0) and kernDataBase+56 (task1)
	binary.LittleEndian.PutUint64(mem[kernDataBase+48:], userPT0Base)
	binary.LittleEndian.PutUint64(mem[kernDataBase+56:], userPT1Base)

	// User tasks: busy-loop incrementing a counter at their data page (no HALT, no Yield)
	// Task 0: loop { mem[userTask0Data] += 1 }
	off := uint32(userTask0Code)
	copy(mem[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Data))
	off += 8
	copy(mem[off:], ie64Instr(OP_LOAD, 2, IE64_SIZE_Q, 0, 1, 0, 0))
	off += 8 // R2 = [data]
	copy(mem[off:], ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 2, 0, 1))
	off += 8 // R2++
	copy(mem[off:], ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 0, 1, 0, 0))
	off += 8 // [data] = R2
	copy(mem[off:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(-24&0xFFFFFFFF)))
	off += 8 // loop back to LOAD

	// Task 1: same pattern with its own data page
	off = uint32(userTask1Code)
	copy(mem[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask1Data))
	off += 8
	copy(mem[off:], ie64Instr(OP_LOAD, 2, IE64_SIZE_Q, 0, 1, 0, 0))
	off += 8
	copy(mem[off:], ie64Instr(OP_ADD, 2, IE64_SIZE_Q, 1, 2, 0, 1))
	off += 8
	copy(mem[off:], ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 0, 1, 0, 0))
	off += 8
	copy(mem[off:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(-24&0xFFFFFFFF)))
	off += 8

	// Build kernel code
	k := newIExecKernel()

	trapAddr := uint32(PROG_START) + 0x3000
	intrAddr := uint32(PROG_START) + 0x3800

	// Set trap vector
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, trapAddr))
	k.emit(ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0))

	// Set interrupt vector
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, intrAddr))
	k.emit(ie64Instr(OP_MTCR, CR_INTR_VEC, 0, 0, 1, 0, 0))

	// Set kernel stack pointer
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernStackTop))
	k.emit(ie64Instr(OP_MTCR, CR_KSP, 0, 0, 1, 0, 0))

	// Enable MMU
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernPageTableBase))
	k.emit(ie64Instr(OP_MTCR, CR_PTBR, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 1))
	k.emit(ie64Instr(OP_MTCR, CR_MMU_CTRL, 0, 0, 1, 0, 0))

	// Program timer: period=10000 instructions, count=10000, ctrl=3 (enable + interrupts)
	// The count is set large enough that the remaining boot instructions
	// (PTBR switch, TLBFLUSH, USP, FAULT_PC, ERET) complete before the first tick.
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 10000))
	k.emit(ie64Instr(OP_MTCR, CR_TIMER_PERIOD, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 10000))
	k.emit(ie64Instr(OP_MTCR, CR_TIMER_COUNT, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 3))
	k.emit(ie64Instr(OP_MTCR, CR_TIMER_CTRL, 0, 0, 1, 0, 0))

	// Switch to task 0's page table and ERET to user mode
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userPT0Base))
	k.emit(ie64Instr(OP_MTCR, CR_PTBR, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_TLBFLUSH, 0, 0, 0, 0, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Stack+MMU_PAGE_SIZE))
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Code))
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// --- Trap handler at 0x3000 (syscalls + faults) ---
	k.padTo(0x3000)
	k.emit(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)) // unexpected trap → halt

	// --- Interrupt handler at 0x3800 (timer preemption) ---
	k.padTo(0x3800)

	// Context switch on timer interrupt
	// The CPU has already: saved previousMode, switched to KSP, set FAULT_PC/FAULT_CAUSE=FAULT_TIMER

	// Disable interrupts while in handler (kernel invariant)
	k.emit(ie64Instr(OP_CLI64, 0, 0, 0, 0, 0, 0))

	// Save current task's return PC and USP
	k.emit(ie64Instr(OP_MOVE, 12, IE64_SIZE_L, 1, 0, 0, kernDataBase))
	k.emit(ie64Instr(OP_LOAD, 13, IE64_SIZE_Q, 0, 12, 0, 0)) // R13 = current_task

	// Save FAULT_PC to task slot
	k.emit(ie64Instr(OP_MFCR, 14, 0, 0, CR_FAULT_PC, 0, 0))
	k.emit(ie64Instr(OP_LSL, 15, IE64_SIZE_Q, 1, 13, 0, 4)) // R15 = task * 16
	k.emit(ie64Instr(OP_ADD, 15, IE64_SIZE_Q, 1, 15, 0, kernDataBase+16))
	k.emit(ie64Instr(OP_STORE, 14, IE64_SIZE_Q, 0, 15, 0, 0)) // save PC

	// Save USP to task slot
	k.emit(ie64Instr(OP_MFCR, 14, 0, 0, CR_USP, 0, 0))
	k.emit(ie64Instr(OP_STORE, 14, IE64_SIZE_Q, 1, 15, 0, 8)) // save USP at +8

	// Toggle task: next = 1 - current
	k.emit(ie64Instr(OP_MOVE, 16, IE64_SIZE_L, 1, 0, 0, 1))
	k.emit(ie64Instr(OP_SUB, 13, IE64_SIZE_Q, 0, 16, 13, 0)) // R13 = 1 - current

	// Store new current_task
	k.emit(ie64Instr(OP_STORE, 13, IE64_SIZE_Q, 0, 12, 0, 0))

	// Load next task's state
	k.emit(ie64Instr(OP_LSL, 15, IE64_SIZE_Q, 1, 13, 0, 4))
	k.emit(ie64Instr(OP_ADD, 15, IE64_SIZE_Q, 1, 15, 0, kernDataBase+16))
	k.emit(ie64Instr(OP_LOAD, 14, IE64_SIZE_Q, 0, 15, 0, 0)) // load PC
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 14, 0, 0))
	k.emit(ie64Instr(OP_LOAD, 14, IE64_SIZE_Q, 1, 15, 0, 8)) // load USP
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 14, 0, 0))

	// Load next task's PTBR
	k.emit(ie64Instr(OP_LSL, 15, IE64_SIZE_Q, 1, 13, 0, 3)) // R15 = task * 8
	k.emit(ie64Instr(OP_ADD, 15, IE64_SIZE_Q, 1, 15, 0, kernDataBase+48))
	k.emit(ie64Instr(OP_LOAD, 14, IE64_SIZE_Q, 0, 15, 0, 0))
	k.emit(ie64Instr(OP_MTCR, CR_PTBR, 0, 0, 14, 0, 0))
	k.emit(ie64Instr(OP_TLBFLUSH, 0, 0, 0, 0, 0, 0))

	// Re-enable interrupts and ERET
	k.emit(ie64Instr(OP_SEI64, 0, 0, 0, 0, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Must use Execute() (not StepOne) because the timer only fires in Execute()
	copy(rig.cpu.memory[PROG_START:], k.code)
	rig.cpu.PC = PROG_START
	rig.cpu.CoprocMode = true // allow PC in user space range
	rig.cpu.running.Store(true)

	// Run in a goroutine, stop after brief execution
	done := make(chan struct{})
	go func() {
		rig.cpu.Execute()
		close(done)
	}()

	// Let it run for a short time then force stop
	time.Sleep(50 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	// Both tasks should have incremented their counters
	counter0 := binary.LittleEndian.Uint64(rig.cpu.memory[userTask0Data:])
	counter1 := binary.LittleEndian.Uint64(rig.cpu.memory[userTask1Data:])

	if counter0 == 0 {
		t.Fatalf("task 0 counter = 0 (timer preemption didn't work)")
	}
	if counter1 == 0 {
		t.Fatalf("task 1 counter = 0 (timer preemption didn't switch to task 1)")
	}
	t.Logf("Timer preemption: task0 count=%d, task1 count=%d", counter0, counter1)
}

// ===========================================================================
// Phase B6: GetSysInfo
// ===========================================================================

func TestIExec_GetSysInfo(t *testing.T) {
	rig := newIE64TestRig()
	mem := rig.cpu.memory

	// Kernel data: tick count at kernDataBase+8
	binary.LittleEndian.PutUint64(mem[kernDataBase+kdTickCount:], 42)

	// User task: SYSCALL #27 (GetSysInfo), infoID=2 (tick count), then store result, HALT
	userPC := uint32(userTask0Code)
	copy(mem[userPC:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 2))        // R1 = infoID 2
	copy(mem[userPC+8:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysGetSysInfo)) // syscall
	copy(mem[userPC+16:], ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, userTask0Data))
	copy(mem[userPC+24:], ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 3, 0, 0)) // [data] = R1 (result)
	copy(mem[userPC+32:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Build kernel
	k := newIExecKernel()
	trapAddr := uint32(PROG_START) + 0x3000
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, trapAddr))
	k.emit(ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernStackTop))
	k.emit(ie64Instr(OP_MTCR, CR_KSP, 0, 0, 1, 0, 0))

	// ERET to user task
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Stack+MMU_PAGE_SIZE))
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userPC))
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Trap handler at 0x3000
	k.padTo(0x3000)
	k.emit(ie64Instr(OP_MFCR, 10, 0, 0, CR_FAULT_CAUSE, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, FAULT_SYSCALL))
	faultBranch := k.addr()
	k.emit(ie64Instr(OP_BNE, 0, 0, 0, 10, 11, 0)) // not syscall → halt

	// Syscall dispatch
	k.emit(ie64Instr(OP_MFCR, 10, 0, 0, CR_FAULT_ADDR, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, sysGetSysInfo))
	getSysInfoBranch := k.addr()
	k.emit(ie64Instr(OP_BEQ, 0, 0, 0, 10, 11, 0)) // patched

	// Unknown syscall → ERET
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// GetSysInfo handler:
	// Read the user's R1 (infoID) — but R1 was clobbered by trap entry.
	// The user put infoID=2 in R1 before SYSCALL. After trap entry, R1 was used
	// by the kernel. But the user's R1 is preserved in the user context (the kernel
	// trap handler clobbered R10-R16, not R1-R6).
	// Actually, trapEntry doesn't save user registers — it only swaps SP.
	// The user's R1 is still in cpu.regs[1] when the handler runs!
	// But the handler reads CR_FAULT_CAUSE into R10 and CR_FAULT_ADDR into R10,
	// clobbering R10-R11. R1 should still hold the user's infoID.
	// Wait — actually R1 is used by the boot code before ERET. After ERET, the
	// user sets R1=2. Then SYSCALL fires. trapEntry doesn't save R1.
	// In the handler, R1 should still be 2 (the user's value).

	getSysInfoAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[getSysInfoBranch-PROG_START+4:], uint32(int32(getSysInfoAddr)-int32(getSysInfoBranch)))

	// R1 = user's infoID (still in R1 since handler didn't clobber it)
	// infoID 2 = tick count at kernDataBase+8
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, 2))
	infoTickBranch := k.addr()
	k.emit(ie64Instr(OP_BEQ, 0, 0, 0, 1, 11, 0)) // if infoID == 2

	// Unknown infoID → return 0
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// infoID 2: return tick count
	infoTickAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[infoTickBranch-PROG_START+4:], uint32(int32(infoTickAddr)-int32(infoTickBranch)))
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, kernDataBase+kdTickCount))
	k.emit(ie64Instr(OP_LOAD, 1, IE64_SIZE_Q, 0, 11, 0, 0)) // R1 = tick count
	k.emit(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0))  // R2 = ERR_OK
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Fault → halt
	faultAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[faultBranch-PROG_START+4:], uint32(int32(faultAddr)-int32(faultBranch)))
	k.emit(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	loadAndRunKernel(t, rig, k, 200000)

	result := binary.LittleEndian.Uint64(rig.cpu.memory[userTask0Data:])
	if result != 42 {
		t.Fatalf("GetSysInfo(tick_count) = %d, want 42", result)
	}
}

// ===========================================================================
// Assembled Kernel Boot Test
// ===========================================================================

// copyFile copies a single file for the assembler test.
func copyFileForTest(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open %s: %v", src, err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create %s: %v", dst, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}

func TestIExec_AssembledKernelBoots(t *testing.T) {
	// Build ie64asm from source (reuse existing helper)
	asmBin := buildAssembler(t)

	// Copy kernel source + include to temp dir (ie64asm outputs alongside source)
	tmpDir := t.TempDir()
	root := repoRootDir(t)
	copyFileForTest(t, filepath.Join(root, "sdk", "intuitionos", "iexec", "iexec.s"),
		filepath.Join(tmpDir, "iexec.s"))
	copyFileForTest(t, filepath.Join(root, "sdk", "include", "iexec.inc"),
		filepath.Join(tmpDir, "iexec.inc"))

	// Assemble — output goes to tmpDir/iexec.ie64
	cmd := exec.Command(asmBin, "-I", tmpDir, filepath.Join(tmpDir, "iexec.s"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("assembly failed: %v\n%s", err, out)
	}

	// Load the assembled binary
	data, err := os.ReadFile(filepath.Join(tmpDir, "iexec.ie64"))
	if err != nil {
		t.Fatalf("read assembled binary: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("assembled binary is empty")
	}

	// Boot the kernel
	rig := newIE64TestRig()
	copy(rig.cpu.memory[PROG_START:], data)
	rig.cpu.PC = PROG_START
	rig.cpu.CoprocMode = true
	rig.cpu.running.Store(true)

	done := make(chan struct{})
	go func() {
		rig.cpu.Execute()
		close(done)
	}()

	// Let it run for 200ms (millions of instruction cycles)
	time.Sleep(200 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	// Both tasks should have incremented their counters (timer preemption)
	counter0 := binary.LittleEndian.Uint64(rig.cpu.memory[userTask0Data:])
	counter1 := binary.LittleEndian.Uint64(rig.cpu.memory[userTask1Data:])

	t.Logf("Assembled kernel: task0 count=%d, task1 count=%d", counter0, counter1)

	if counter0 == 0 {
		t.Fatalf("task 0 counter = 0 (standalone kernel didn't boot or preempt)")
	}
	if counter1 == 0 {
		t.Fatalf("task 1 counter = 0 (standalone kernel didn't switch to task 1)")
	}
}
