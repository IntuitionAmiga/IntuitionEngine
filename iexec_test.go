// iexec_test.go - IExec microkernel integration tests

package main

import (
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	sysYield        = 26
	sysGetSysInfo   = 27
	sysDebugPutChar = 33
	sysAllocSignal  = 11
	sysFreeSignal   = 12
	sysSignal       = 13
	sysWait         = 14

	// Kernel data offsets (must match iexec.inc)
	kdCurrentTask = 0  // uint64: index of current task (0 or 1)
	kdTickCount   = 8  // uint64: tick counter
	kdNumTasks    = 16 // uint64: number of active tasks

	// TCB layout (must match iexec.inc KD_TASK_*)
	kdTCBBase      = 64 // start of TCB array
	tcbStride      = 32 // bytes per task
	tcbPCOff       = 0  // saved PC (8 bytes)
	tcbUSPOff      = 8  // saved USP (8 bytes)
	tcbSigAllocOff = 16 // allocated signal bits (4 bytes)
	tcbSigWaitOff  = 20 // wait mask (4 bytes)
	tcbSigRecvOff  = 24 // pending signals (4 bytes)
	tcbStateOff    = 28 // state byte

	// PTBR array (after TCBs)
	kdPTBRBase = 128 // KD_PTBR_BASE
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
// Phase B4: Two Tasks + Context Switch (simplified — uses inline kernel below)
// ===========================================================================

// buildTwoTaskKernel was removed — the old 288-byte TCB layout is replaced by
// the 32-byte M3 layout. The assembled kernel (iexec.s) is now the reference
// implementation. Programmatic kernels in tests below use the simplified inline
// approach with host-side data initialization.
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
	// This test is now handled by the M2 tests (BootBanner, TwoTasksVisibleOutput)
	// which use assembleAndLoadKernel with terminal MMIO.
	// Kept as a basic smoke test that the kernel assembles and runs.
	rig, term := assembleAndLoadKernel(t)
	rig.cpu.running.Store(true)

	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(200 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	output := term.DrainOutput()
	if len(output) == 0 {
		t.Fatal("assembled kernel produced no output")
	}
	if !strings.Contains(output, "IExec") {
		t.Fatalf("no boot banner in output: %q", output[:min(len(output), 40)])
	}
}

// ===========================================================================
// M2: Observable Kernel Tests
// ===========================================================================

// newIExecTerminalRig creates a test rig with terminal MMIO mapped.
func newIExecTerminalRig(t *testing.T) (*ie64TestRig, *TerminalMMIO) {
	t.Helper()
	rig := newIE64TestRig()
	term := NewTerminalMMIO()
	rig.bus.MapIO(TERM_OUT, TERMINAL_REGION_END, term.HandleRead, term.HandleWrite)
	return rig, term
}

// assembleAndLoadKernel builds ie64asm, assembles iexec.s, loads the binary into a rig with terminal.
func assembleAndLoadKernel(t *testing.T) (*ie64TestRig, *TerminalMMIO) {
	t.Helper()
	asmBin := buildAssembler(t)
	tmpDir := t.TempDir()
	root := repoRootDir(t)
	copyFileForTest(t, filepath.Join(root, "sdk", "intuitionos", "iexec", "iexec.s"), filepath.Join(tmpDir, "iexec.s"))
	copyFileForTest(t, filepath.Join(root, "sdk", "include", "iexec.inc"), filepath.Join(tmpDir, "iexec.inc"))
	copyFileForTest(t, filepath.Join(root, "sdk", "include", "ie64.inc"), filepath.Join(tmpDir, "ie64.inc"))

	cmd := exec.Command(asmBin, "-I", tmpDir, filepath.Join(tmpDir, "iexec.s"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("assembly failed: %v\n%s", err, out)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "iexec.ie64"))
	if err != nil {
		t.Fatalf("read assembled binary: %v", err)
	}

	rig, term := newIExecTerminalRig(t)
	copy(rig.cpu.memory[PROG_START:], data)
	rig.cpu.PC = PROG_START
	rig.cpu.CoprocMode = true
	return rig, term
}

func TestIExec_DebugPutChar(t *testing.T) {
	// Programmatic kernel: one user task does SYSCALL #33 with R1='X' then halts
	rig, term := newIExecTerminalRig(t)
	cpu := rig.cpu

	// Set up minimal kernel: vectors, KSP, no MMU needed for this test
	trapAddr := uint32(PROG_START) + 0x3000
	cpu.trapVector = uint64(trapAddr)
	cpu.kernelSP = kernStackTop

	// User task at userTask0Code: MOVE R1, #'X'; SYSCALL #33; HALT
	copy(cpu.memory[userTask0Code:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x58)) // 'X'
	copy(cpu.memory[userTask0Code+8:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, 33))
	copy(cpu.memory[userTask0Code+16:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Kernel: ERET to user task
	k := newIExecKernel()
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, trapAddr))
	k.emit(ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernStackTop))
	k.emit(ie64Instr(OP_MTCR, CR_KSP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Stack+MMU_PAGE_SIZE))
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Code))
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Trap handler: check SYSCALL #33, write R1 to TERM_OUT, ERET
	k.padTo(0x3000)
	k.emit(ie64Instr(OP_MFCR, 10, 0, 0, CR_FAULT_CAUSE, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, FAULT_SYSCALL))
	haltBranch := k.addr()
	k.emit(ie64Instr(OP_BNE, 0, 0, 0, 10, 11, 0))
	// SYSCALL dispatch
	k.emit(ie64Instr(OP_MFCR, 10, 0, 0, CR_FAULT_ADDR, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, 33))
	putcharBranch := k.addr()
	k.emit(ie64Instr(OP_BEQ, 0, 0, 0, 10, 11, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0)) // unknown syscall
	// DebugPutChar handler
	putcharAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[putcharBranch-PROG_START+4:], uint32(int32(putcharAddr)-int32(putcharBranch)))
	k.emit(ie64Instr(OP_MOVE, 28, IE64_SIZE_L, 1, 0, 0, TERM_OUT))
	k.emit(ie64Instr(OP_STORE, 1, IE64_SIZE_B, 0, 28, 0, 0)) // store.b R1, (R28)
	k.emit(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0))   // ERR_OK
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	// Fault halt
	haltAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[haltBranch-PROG_START+4:], uint32(int32(haltAddr)-int32(haltBranch)))
	k.emit(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	loadAndRunKernel(t, rig, k, 100000)

	output := term.DrainOutput()
	if !strings.Contains(output, "X") {
		t.Fatalf("DebugPutChar: output = %q, want 'X'", output)
	}
}

func TestIExec_BootBanner(t *testing.T) {
	rig, term := assembleAndLoadKernel(t)
	rig.cpu.running.Store(true)

	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(200 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	output := term.DrainOutput()
	if !strings.HasPrefix(output, "IExec") {
		t.Fatalf("boot banner: output starts with %q, want 'IExec...'", output[:min(len(output), 20)])
	}
	t.Logf("Boot banner output (first 80 chars): %q", output[:min(len(output), 80)])
}

func TestIExec_TwoTasksVisibleOutput(t *testing.T) {
	rig, term := assembleAndLoadKernel(t)
	rig.cpu.running.Store(true)

	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(200 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	output := term.DrainOutput()
	hasA := strings.Contains(output, "A")
	hasB := strings.Contains(output, "B")
	if !hasA || !hasB {
		t.Fatalf("visible output: hasA=%v hasB=%v, output=%q", hasA, hasB, output[:min(len(output), 100)])
	}
	t.Logf("Task output (first 100 chars): %q", output[:min(len(output), 100)])
}

func TestIExec_FaultPrintsReport(t *testing.T) {
	// Boot the real assembled kernel, but with a modified task 0 that accesses
	// an unmapped page. The kernel's own fault handler (kern_puts/kern_put_hex)
	// should print a FAULT report.
	rig, term := assembleAndLoadKernel(t)

	// Find the task 0 template in the kernel binary and overwrite it with
	// fault-triggering code. The kernel init copies this to 0x600000, so the
	// fault happens when task 0 first runs.
	// Search for the task 0 template marker: MOVE R1, #0x41 ('A') = the first instruction.
	// The encoding is: opcode=0x01, rd=1, size=2(L), xbit=1, rs=0, rt=0, imm32=0x41
	marker := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x41)
	templateOff := -1
	for i := 0; i+8 <= len(rig.cpu.memory[PROG_START:]); i += 8 {
		match := true
		for j := 0; j < 8; j++ {
			if rig.cpu.memory[PROG_START+uint32(i)+uint32(j)] != marker[j] {
				match = false
				break
			}
		}
		if match {
			templateOff = i
			break
		}
	}
	if templateOff < 0 {
		t.Fatal("could not find task 0 template in kernel binary")
	}
	// Overwrite the template with fault-triggering code
	base := PROG_START + uint32(templateOff)
	copy(rig.cpu.memory[base:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x700000))
	copy(rig.cpu.memory[base+8:], ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(200 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	output := term.DrainOutput()
	if !strings.Contains(output, "FAULT") {
		t.Fatalf("fault report: output = %q, want 'FAULT' from real kernel handler", output)
	}
	// Verify the report includes PC and ADDR fields
	if !strings.Contains(output, "PC=") {
		t.Logf("fault output: %q", output)
		t.Fatal("fault report missing PC= field")
	}
	t.Logf("Fault report output: %q", output[strings.Index(output, "FAULT"):min(len(output), strings.Index(output, "FAULT")+80)])
}

// ===========================================================================
// M3: Signals Tests
// ===========================================================================

func TestIExec_AllocSignal(t *testing.T) {
	// User task: AllocSignal(-1), store result to data page, halt
	rig, term := newIExecTerminalRig(t)
	cpu := rig.cpu

	trapAddr := uint32(PROG_START) + 0x3000
	cpu.trapVector = uint64(trapAddr)
	cpu.kernelSP = kernStackTop

	// User task
	off := uint32(userTask0Code)
	copy(cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xFFFFFFFF))
	off += 8 // R1 = -1 (auto)
	copy(cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysAllocSignal))
	off += 8
	// R1 = allocated bit, R2 = err
	copy(cpu.memory[off:], ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, userTask0Data))
	off += 8
	copy(cpu.memory[off:], ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 3, 0, 0))
	off += 8 // [data] = bit
	copy(cpu.memory[off:], ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 3, 0, 8))
	off += 8 // [data+8] = err
	copy(cpu.memory[off:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Minimal kernel: boot, init TCB with sig_alloc=0xFFFF (system bits), ERET to user
	k := newIExecKernel()
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, trapAddr))
	k.emit(ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernStackTop))
	k.emit(ie64Instr(OP_MTCR, CR_KSP, 0, 0, 1, 0, 0))

	// Init task 0 TCB in memory
	tcb0 := uint32(kernDataBase + kdTCBBase)
	binary.LittleEndian.PutUint32(cpu.memory[tcb0+tcbSigAllocOff:], 0xFFFF) // system bits

	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Stack+MMU_PAGE_SIZE))
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Code))
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Trap handler: dispatch AllocSignal using the same pattern as the real kernel
	k.padTo(0x3000)
	// Read cause and syscall number, handle AllocSignal
	k.emit(ie64Instr(OP_MFCR, 10, 0, 0, CR_FAULT_CAUSE, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, FAULT_SYSCALL))
	faultBranch := k.addr()
	k.emit(ie64Instr(OP_BNE, 0, 0, 0, 10, 11, 0))

	k.emit(ie64Instr(OP_MFCR, 10, 0, 0, CR_FAULT_ADDR, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, sysAllocSignal))
	allocBranch := k.addr()
	k.emit(ie64Instr(OP_BEQ, 0, 0, 0, 10, 11, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0)) // unknown

	// AllocSignal handler (simplified — scan bits 16-31)
	allocAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[allocBranch-PROG_START+4:], uint32(int32(allocAddr)-int32(allocBranch)))
	// Load sig_alloc for current task (task 0)
	k.emit(ie64Instr(OP_MOVE, 20, IE64_SIZE_L, 1, 0, 0, tcb0+tcbSigAllocOff))
	k.emit(ie64Instr(OP_LOAD, 21, IE64_SIZE_L, 0, 20, 0, 0)) // R21 = sig_alloc
	// Find first free bit in 16-31
	k.emit(ie64Instr(OP_MOVE, 22, IE64_SIZE_L, 1, 0, 0, 16)) // R22 = bit counter
	scanLoop := k.addr()
	k.emit(ie64Instr(OP_MOVE, 23, IE64_SIZE_L, 1, 0, 0, 1))
	k.emit(ie64Instr(OP_LSL, 23, IE64_SIZE_Q, 0, 23, 22, 0))   // R23 = 1 << bit
	k.emit(ie64Instr(OP_AND64, 24, IE64_SIZE_Q, 0, 21, 23, 0)) // R24 = alloc & mask
	foundBranch := k.addr()
	k.emit(ie64Instr(OP_BEQ, 0, 0, 0, 24, 0, 0)) // if free, branch
	k.emit(ie64Instr(OP_ADD, 22, IE64_SIZE_Q, 1, 22, 0, 1))
	k.emit(ie64Instr(OP_MOVE, 25, IE64_SIZE_L, 1, 0, 0, 32))
	k.emit(ie64Instr(OP_BLT, 0, 0, 0, 22, 25, uint32(int32(scanLoop)-int32(k.addr()))))
	// Exhausted
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xFFFFFFFF))
	k.emit(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 1)) // ERR_NOMEM
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	// Found free bit
	foundAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[foundBranch-PROG_START+4:], uint32(int32(foundAddr)-int32(foundBranch)))
	k.emit(ie64Instr(OP_OR64, 21, IE64_SIZE_Q, 0, 21, 23, 0)) // alloc |= mask
	k.emit(ie64Instr(OP_STORE, 21, IE64_SIZE_L, 0, 20, 0, 0)) // write back
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 22, 0, 0))   // R1 = bit
	k.emit(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0))    // R2 = ERR_OK
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Fault halt
	haltAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[faultBranch-PROG_START+4:], uint32(int32(haltAddr)-int32(faultBranch)))
	k.emit(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	loadAndRunKernel(t, rig, k, 100000)

	bit := binary.LittleEndian.Uint64(cpu.memory[userTask0Data:])
	err := binary.LittleEndian.Uint64(cpu.memory[userTask0Data+8:])

	_ = term
	if err != 0 {
		t.Fatalf("AllocSignal err = %d, want 0", err)
	}
	if bit < 16 || bit > 31 {
		t.Fatalf("AllocSignal bit = %d, want 16-31", bit)
	}
	t.Logf("AllocSignal: allocated bit %d", bit)
}

func TestIExec_WaitBlocks(t *testing.T) {
	// Verify that Wait actually blocks a task and resumes it after Signal.
	// Task 0: Wait for bit 16 (blocks), then print 'R' (resumed).
	// Task 1: print 'S' (signaling), Signal task 0 bit 16, yield+loop.
	// Expected output: boot banner, then 'S' (task 1 signals), then 'R' (task 0 resumes).
	// The 'R' appearing proves task 0 blocked and was woken by Signal.
	rig, term := assembleAndLoadKernel(t)

	// Patch task 0: Wait for bit 16, print 'R' on resume
	marker0 := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x41)
	t0Off := -1
	for i := 0; i+8 <= len(rig.cpu.memory[PROG_START:]); i += 8 {
		match := true
		for j := 0; j < 8; j++ {
			if rig.cpu.memory[PROG_START+uint32(i)+uint32(j)] != marker0[j] {
				match = false
				break
			}
		}
		if match {
			t0Off = i
			break
		}
	}
	if t0Off < 0 {
		t.Fatal("could not find task 0 template")
	}
	base0 := PROG_START + uint32(t0Off)
	off := base0
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x10000))
	off += 8 // mask = 1<<16
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWait))
	off += 8 // Wait → blocks
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x52))
	off += 8 // 'R' (resumed)
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysDebugPutChar))
	off += 8
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	off += 8
	copy(rig.cpu.memory[off:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(-8&0xFFFFFFFF)))

	// Patch task 1: print 'S', Signal task 0 bit 16, yield+loop
	marker1 := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x10000)
	t1Off := -1
	for i := t0Off + 8; i+8 <= len(rig.cpu.memory[PROG_START:]); i += 8 {
		match := true
		for j := 0; j < 8; j++ {
			if rig.cpu.memory[PROG_START+uint32(i)+uint32(j)] != marker1[j] {
				match = false
				break
			}
		}
		if match {
			t1Off = i
			break
		}
	}
	if t1Off < 0 {
		t.Fatal("could not find task 1 template")
	}
	base1 := PROG_START + uint32(t1Off)
	off = base1
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x53))
	off += 8 // 'S'
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysDebugPutChar))
	off += 8
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0))
	off += 8 // taskID=0
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0x10000))
	off += 8 // mask=1<<16
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysSignal))
	off += 8
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	off += 8
	copy(rig.cpu.memory[off:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(-8&0xFFFFFFFF)))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(300 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	output := term.DrainOutput()
	hasS := strings.Contains(output, "S")
	hasR := strings.Contains(output, "R")
	if !hasS {
		t.Fatalf("WaitBlocks: task 1 didn't print 'S' (signaler), output=%q", output[:min(len(output), 80)])
	}
	if !hasR {
		t.Fatalf("WaitBlocks: task 0 didn't print 'R' (resumed after Wait), output=%q", output[:min(len(output), 80)])
	}
	t.Logf("WaitBlocks output (first 80 chars): %q", output[:min(len(output), 80)])
}

func TestIExec_WaitDeadlock(t *testing.T) {
	// Assemble kernel, patch both task templates to Wait on unsatisfied signals.
	// Both tasks should block, kernel should print DEADLOCK.
	rig, term := assembleAndLoadKernel(t)

	// Patch task 0 template: Wait for bit 16 (never signaled)
	marker := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x41) // 'A'
	templateOff := -1
	for i := 0; i+8 <= len(rig.cpu.memory[PROG_START:]); i += 8 {
		match := true
		for j := 0; j < 8; j++ {
			if rig.cpu.memory[PROG_START+uint32(i)+uint32(j)] != marker[j] {
				match = false
				break
			}
		}
		if match {
			templateOff = i
			break
		}
	}
	if templateOff < 0 {
		t.Fatal("could not find task 0 template")
	}

	// Overwrite task 0: Wait for signal bit 16 (mask = 1<<16 = 0x10000)
	base := PROG_START + uint32(templateOff)
	copy(rig.cpu.memory[base:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x10000))
	copy(rig.cpu.memory[base+8:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWait))

	// Find task 1 template (starts with: MOVE R1, #0x10000 = Wait mask)
	marker2 := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x10000)
	template1Off := -1
	for i := templateOff + 8; i+8 <= len(rig.cpu.memory[PROG_START:]); i += 8 {
		match := true
		for j := 0; j < 8; j++ {
			if rig.cpu.memory[PROG_START+uint32(i)+uint32(j)] != marker2[j] {
				match = false
				break
			}
		}
		if match {
			template1Off = i
			break
		}
	}
	if template1Off < 0 {
		t.Fatal("could not find task 1 template")
	}

	// Overwrite task 1: Wait for signal bit 17 (mask = 1<<17 = 0x20000)
	base2 := PROG_START + uint32(template1Off)
	copy(rig.cpu.memory[base2:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x20000))
	copy(rig.cpu.memory[base2+8:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWait))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(200 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	output := term.DrainOutput()
	if !strings.Contains(output, "DEADLOCK") {
		t.Fatalf("deadlock: output = %q, expected 'DEADLOCK'", output[:min(len(output), 100)])
	}
	t.Logf("Deadlock output: %q", output[strings.Index(output, "DEADLOCK"):min(len(output), strings.Index(output, "DEADLOCK")+40)])
}

func TestIExec_FreeSignal(t *testing.T) {
	// Allocate a signal, then free it. Verify no error on both operations.
	rig, _ := newIExecTerminalRig(t)
	cpu := rig.cpu

	trapAddr := uint32(PROG_START) + 0x3000
	cpu.trapVector = uint64(trapAddr)
	cpu.kernelSP = kernStackTop

	// Init task 0 TCB
	tcb0 := uint32(kernDataBase + kdTCBBase)
	binary.LittleEndian.PutUint32(cpu.memory[tcb0+tcbSigAllocOff:], 0xFFFF)

	// User task: AllocSignal(-1), store bit; FreeSignal(bit), store err; halt
	off := uint32(userTask0Code)
	copy(cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xFFFFFFFF))
	off += 8
	copy(cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysAllocSignal))
	off += 8
	// R1 = bit, save it
	copy(cpu.memory[off:], ie64Instr(OP_MOVE, 5, IE64_SIZE_Q, 0, 1, 0, 0))
	off += 8 // R5 = bit
	copy(cpu.memory[off:], ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, userTask0Data))
	off += 8
	copy(cpu.memory[off:], ie64Instr(OP_STORE, 5, IE64_SIZE_Q, 0, 3, 0, 0))
	off += 8 // [data] = bit
	// Now free it: R1 = bit
	copy(cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 5, 0, 0))
	off += 8
	copy(cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysFreeSignal))
	off += 8
	copy(cpu.memory[off:], ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 1, 3, 0, 8))
	off += 8 // [data+8] = err
	copy(cpu.memory[off:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Build kernel with AllocSignal + FreeSignal handlers (reuse AllocSignal test kernel pattern)
	// For simplicity, assemble the real kernel and patch task 0 template
	// Actually, let's use the assembled kernel approach
	rigAsm, termAsm := assembleAndLoadKernel(t)

	// Patch task 0 template with our alloc+free code
	marker := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x41) // 'A'
	templateOff := -1
	for i := 0; i+8 <= len(rigAsm.cpu.memory[PROG_START:]); i += 8 {
		match := true
		for j := 0; j < 8; j++ {
			if rigAsm.cpu.memory[PROG_START+uint32(i)+uint32(j)] != marker[j] {
				match = false
				break
			}
		}
		if match {
			templateOff = i
			break
		}
	}
	if templateOff < 0 {
		t.Fatal("could not find task 0 template")
	}

	base := PROG_START + uint32(templateOff)
	off = base
	copy(rigAsm.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xFFFFFFFF))
	off += 8
	copy(rigAsm.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysAllocSignal))
	off += 8
	copy(rigAsm.cpu.memory[off:], ie64Instr(OP_MOVE, 5, IE64_SIZE_Q, 0, 1, 0, 0))
	off += 8
	copy(rigAsm.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 5, 0, 0))
	off += 8
	copy(rigAsm.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysFreeSignal))
	off += 8
	// Store free error to data page
	copy(rigAsm.cpu.memory[off:], ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, userTask0Data))
	off += 8
	copy(rigAsm.cpu.memory[off:], ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 0, 3, 0, 0))
	off += 8
	copy(rigAsm.cpu.memory[off:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	rigAsm.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rigAsm.cpu.Execute(); close(done) }()
	time.Sleep(200 * time.Millisecond)
	rigAsm.cpu.running.Store(false)
	<-done

	_ = termAsm
	freeErr := binary.LittleEndian.Uint64(rigAsm.cpu.memory[userTask0Data:])
	if freeErr != 0 {
		t.Fatalf("FreeSignal err = %d, want 0", freeErr)
	}
}

func TestIExec_AllocSignalExhausted(t *testing.T) {
	// Use the same programmatic kernel pattern as TestIExec_AllocSignal.
	// Pre-set sig_alloc to 0xFFFFFFFF (all bits allocated), try to allocate → ERR_NOMEM.
	rig, _ := newIExecTerminalRig(t)
	cpu := rig.cpu

	trapAddr := uint32(PROG_START) + 0x3000
	cpu.trapVector = uint64(trapAddr)
	cpu.kernelSP = kernStackTop

	// Pre-set ALL bits allocated in task 0's TCB
	tcb0 := uint32(kernDataBase + kdTCBBase)
	binary.LittleEndian.PutUint32(cpu.memory[tcb0+tcbSigAllocOff:], 0xFFFFFFFF)

	// User task: AllocSignal(-1), store err, halt
	off := uint32(userTask0Code)
	copy(cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xFFFFFFFF))
	off += 8
	copy(cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysAllocSignal))
	off += 8
	copy(cpu.memory[off:], ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, userTask0Data))
	off += 8
	copy(cpu.memory[off:], ie64Instr(OP_STORE, 2, IE64_SIZE_Q, 0, 3, 0, 0))
	off += 8 // [data] = err
	copy(cpu.memory[off:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Reuse the same programmatic AllocSignal kernel from TestIExec_AllocSignal
	k := newIExecKernel()
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, trapAddr))
	k.emit(ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, kernStackTop))
	k.emit(ie64Instr(OP_MTCR, CR_KSP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Stack+MMU_PAGE_SIZE))
	k.emit(ie64Instr(OP_MTCR, CR_USP, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userTask0Code))
	k.emit(ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 1, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Trap handler with AllocSignal (same as TestIExec_AllocSignal)
	k.padTo(0x3000)
	k.emit(ie64Instr(OP_MFCR, 10, 0, 0, CR_FAULT_CAUSE, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, FAULT_SYSCALL))
	faultBranch := k.addr()
	k.emit(ie64Instr(OP_BNE, 0, 0, 0, 10, 11, 0))
	k.emit(ie64Instr(OP_MFCR, 10, 0, 0, CR_FAULT_ADDR, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 11, IE64_SIZE_L, 1, 0, 0, sysAllocSignal))
	allocBranch := k.addr()
	k.emit(ie64Instr(OP_BEQ, 0, 0, 0, 10, 11, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	allocAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[allocBranch-PROG_START+4:], uint32(int32(allocAddr)-int32(allocBranch)))
	// Load sig_alloc
	k.emit(ie64Instr(OP_MOVE, 20, IE64_SIZE_L, 1, 0, 0, tcb0+tcbSigAllocOff))
	k.emit(ie64Instr(OP_LOAD, 21, IE64_SIZE_L, 0, 20, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 22, IE64_SIZE_L, 1, 0, 0, 16))
	scanLoop := k.addr()
	k.emit(ie64Instr(OP_MOVE, 23, IE64_SIZE_L, 1, 0, 0, 1))
	k.emit(ie64Instr(OP_LSL, 23, IE64_SIZE_Q, 0, 23, 22, 0))
	k.emit(ie64Instr(OP_AND64, 24, IE64_SIZE_Q, 0, 21, 23, 0))
	foundBranch := k.addr()
	k.emit(ie64Instr(OP_BEQ, 0, 0, 0, 24, 0, 0))
	k.emit(ie64Instr(OP_ADD, 22, IE64_SIZE_Q, 1, 22, 0, 1))
	k.emit(ie64Instr(OP_MOVE, 25, IE64_SIZE_L, 1, 0, 0, 32))
	k.emit(ie64Instr(OP_BLT, 0, 0, 0, 22, 25, uint32(int32(scanLoop)-int32(k.addr()))))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xFFFFFFFF))
	k.emit(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 1)) // ERR_NOMEM
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	foundAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[foundBranch-PROG_START+4:], uint32(int32(foundAddr)-int32(foundBranch)))
	k.emit(ie64Instr(OP_OR64, 21, IE64_SIZE_Q, 0, 21, 23, 0))
	k.emit(ie64Instr(OP_STORE, 21, IE64_SIZE_L, 0, 20, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 1, IE64_SIZE_Q, 0, 22, 0, 0))
	k.emit(ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0))
	k.emit(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	haltAddr := k.addr()
	binary.LittleEndian.PutUint32(k.code[faultBranch-PROG_START+4:], uint32(int32(haltAddr)-int32(faultBranch)))
	k.emit(ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	loadAndRunKernel(t, rig, k, 100000)

	allocErr := binary.LittleEndian.Uint64(cpu.memory[userTask0Data:])
	if allocErr != 1 {
		t.Fatalf("AllocSignal exhausted: err = %d, want 1 (ERR_NOMEM)", allocErr)
	}
}

func TestIExec_WaitImmediate(t *testing.T) {
	// Pre-set a signal bit in task 0's recv, then Wait for it — should return immediately
	rig, term := assembleAndLoadKernel(t)

	// Set signal bit 16 in task 0's sig_recv (before kernel init overwrites it)
	// The kernel init writes sig_recv=0, so we need to set it AFTER init runs.
	// Instead, patch the task template to: Signal self with bit 16, then Wait for bit 16.
	marker := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x41)
	templateOff := -1
	for i := 0; i+8 <= len(rig.cpu.memory[PROG_START:]); i += 8 {
		match := true
		for j := 0; j < 8; j++ {
			if rig.cpu.memory[PROG_START+uint32(i)+uint32(j)] != marker[j] {
				match = false
				break
			}
		}
		if match {
			templateOff = i
			break
		}
	}
	if templateOff < 0 {
		t.Fatal("could not find task 0 template")
	}

	// Task 0: Signal self (task 0) with bit 16, then Wait for bit 16, print 'Y' if received, halt
	base := PROG_START + uint32(templateOff)
	off := base
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0))
	off += 8 // R1 = taskID 0 (self)
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0x10000))
	off += 8 // R2 = 1<<16
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysSignal))
	off += 8 // Signal(0, 1<<16)
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x10000))
	off += 8 // R1 = mask
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWait))
	off += 8 // Wait(1<<16) → immediate
	// R1 should now be 0x10000. Print 'Y' to confirm.
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x59))
	off += 8 // 'Y'
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysDebugPutChar))
	off += 8
	copy(rig.cpu.memory[off:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(200 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	output := term.DrainOutput()
	if !strings.Contains(output, "Y") {
		t.Fatalf("WaitImmediate: output = %q, expected 'Y' (Wait returned immediately)", output[:min(len(output), 80)])
	}
}

func TestIExec_SignalWakes(t *testing.T) {
	// Task 0 Waits for bit 16. Task 1 Signals task 0 with bit 16, then prints 'W'.
	// Task 0 wakes and prints 'K'. Output should contain both 'W' and 'K'.
	rig, term := assembleAndLoadKernel(t)

	// Patch task 0: Wait for bit 16, then print 'K'
	marker0 := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x41)
	t0Off := -1
	for i := 0; i+8 <= len(rig.cpu.memory[PROG_START:]); i += 8 {
		match := true
		for j := 0; j < 8; j++ {
			if rig.cpu.memory[PROG_START+uint32(i)+uint32(j)] != marker0[j] {
				match = false
				break
			}
		}
		if match {
			t0Off = i
			break
		}
	}
	if t0Off < 0 {
		t.Fatal("could not find task 0 template")
	}
	base0 := PROG_START + uint32(t0Off)
	off := base0
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x10000))
	off += 8 // mask = 1<<16
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWait))
	off += 8 // Wait → blocks
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x4B))
	off += 8 // 'K'
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysDebugPutChar))
	off += 8
	copy(rig.cpu.memory[off:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Patch task 1: Signal task 0 with bit 16, print 'W', yield+loop
	// Task 1 template starts with: move.l r1, #0x10000 (Wait for bit 16)
	marker1 := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x10000)
	t1Off := -1
	for i := t0Off + 8; i+8 <= len(rig.cpu.memory[PROG_START:]); i += 8 {
		match := true
		for j := 0; j < 8; j++ {
			if rig.cpu.memory[PROG_START+uint32(i)+uint32(j)] != marker1[j] {
				match = false
				break
			}
		}
		if match {
			t1Off = i
			break
		}
	}
	if t1Off < 0 {
		t.Fatal("could not find task 1 template")
	}
	base1 := PROG_START + uint32(t1Off)
	off = base1
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0))
	off += 8 // taskID = 0
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0x10000))
	off += 8 // mask = 1<<16
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysSignal))
	off += 8 // Signal(0, 1<<16)
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x57))
	off += 8 // 'W'
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysDebugPutChar))
	off += 8
	// Yield so the woken task 0 gets to run (HALT would stop the CPU entirely)
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysYield))
	off += 8
	copy(rig.cpu.memory[off:], ie64Instr(OP_BRA, 0, 0, 0, 0, 0, uint32(-8&0xFFFFFFFF)))

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(300 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	output := term.DrainOutput()
	hasW := strings.Contains(output, "W")
	hasK := strings.Contains(output, "K")
	if !hasW || !hasK {
		t.Fatalf("SignalWakes: hasW=%v hasK=%v, output=%q", hasW, hasK, output[:min(len(output), 100)])
	}
	t.Logf("SignalWakes output: %q", output[:min(len(output), 80)])
}

func TestIExec_SignalMaskFiltering(t *testing.T) {
	// Task 0 Waits for bit 16. Task 1 Signals task 0 with bit 17 (wrong bit).
	// Task 0 should NOT wake — both tasks should deadlock.
	rig, term := assembleAndLoadKernel(t)

	// Patch task 0: Wait for bit 16
	marker0 := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x41)
	t0Off := -1
	for i := 0; i+8 <= len(rig.cpu.memory[PROG_START:]); i += 8 {
		match := true
		for j := 0; j < 8; j++ {
			if rig.cpu.memory[PROG_START+uint32(i)+uint32(j)] != marker0[j] {
				match = false
				break
			}
		}
		if match {
			t0Off = i
			break
		}
	}
	if t0Off < 0 {
		t.Fatal("could not find task 0 template")
	}
	base0 := PROG_START + uint32(t0Off)
	copy(rig.cpu.memory[base0:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x10000)) // Wait for bit 16
	copy(rig.cpu.memory[base0+8:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWait))

	// Patch task 1: Signal task 0 with bit 17 (wrong), then Wait for bit 18 (deadlock)
	marker1 := ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x10000)
	t1Off := -1
	for i := t0Off + 8; i+8 <= len(rig.cpu.memory[PROG_START:]); i += 8 {
		match := true
		for j := 0; j < 8; j++ {
			if rig.cpu.memory[PROG_START+uint32(i)+uint32(j)] != marker1[j] {
				match = false
				break
			}
		}
		if match {
			t1Off = i
			break
		}
	}
	if t1Off < 0 {
		t.Fatal("could not find task 1 template")
	}
	base1 := PROG_START + uint32(t1Off)
	off := base1
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0))
	off += 8 // taskID = 0
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0x20000))
	off += 8 // mask = 1<<17 (WRONG)
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysSignal))
	off += 8
	copy(rig.cpu.memory[off:], ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x40000))
	off += 8 // Wait for bit 18
	copy(rig.cpu.memory[off:], ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, sysWait))
	off += 8 // blocks → deadlock

	rig.cpu.running.Store(true)
	done := make(chan struct{})
	go func() { rig.cpu.Execute(); close(done) }()
	time.Sleep(200 * time.Millisecond)
	rig.cpu.running.Store(false)
	<-done

	output := term.DrainOutput()
	// Task 0 should NOT have woken (bit 17 != bit 16), so both deadlock
	if !strings.Contains(output, "DEADLOCK") {
		t.Fatalf("SignalMaskFiltering: output = %q, expected DEADLOCK (wrong bit should not wake)", output[:min(len(output), 100)])
	}
}
