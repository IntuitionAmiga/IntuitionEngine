package main

import (
	"encoding/binary"
	"testing"
)

// ===========================================================================
// MMU Phase 2: Page Table Walk
// ===========================================================================

// writePTE installs a leaf PTE for VPN through the multi-level walk.
// PLAN_MAX_RAM.md slice 4 design: the page table is a 6-level sparse
// radix tree, so single-level pokes no longer reach the walk's leaf.
// Decode pte → (ppn, flags) and route through mmuMap so intermediate
// tables are auto-allocated from the per-PTBR pool.
func writePTE(cpu *CPU64, vpn uint64, pte uint64) {
	ppn, flags := parsePTE(pte)
	mmuMap(cpu, vpn<<MMU_PAGE_SHIFT, ppn, flags)
}

func TestMMU_PTEEncodeDecode(t *testing.T) {
	// Test round-trip of PTE encode/decode
	testCases := []struct {
		ppn   uint64
		flags byte
	}{
		{0, PTE_P | PTE_R},
		{1, PTE_P | PTE_R | PTE_W | PTE_X | PTE_U},
		{8191, PTE_P | PTE_R | PTE_X},
		{4096, PTE_P | PTE_W},
	}
	for _, tc := range testCases {
		pte := makePTE(tc.ppn, tc.flags)
		gotPPN, gotFlags := parsePTE(pte)
		if gotPPN != tc.ppn || gotFlags != tc.flags {
			t.Errorf("makePTE(%d, 0x%02X) round-trip: got ppn=%d flags=0x%02X",
				tc.ppn, tc.flags, gotPPN, gotFlags)
		}
	}
}

func TestMMU_WalkValid(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000 // page table at physical 0x20000

	// Map virtual page 1 -> physical page 5, with R+W+X+U
	writePTE(cpu, 1, makePTE(5, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	// Translate virtual address 0x1ABC (page 1, offset 0xABC)
	vaddr := uint64(0x1ABC)
	phys, fault, _ := cpu.translateAddr(vaddr, ACCESS_READ)
	if fault {
		t.Fatal("translateAddr returned fault for valid page")
	}
	// Expected: physical page 5, offset 0xABC = 0x5ABC
	if phys != 0x5ABC {
		t.Fatalf("translateAddr(0x%X) = 0x%X, want 0x5ABC", vaddr, phys)
	}
}

func TestMMU_WalkNotPresent(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000

	// PTE with P=0 (not present)
	writePTE(cpu, 1, makePTE(5, PTE_R|PTE_W)) // no PTE_P

	_, fault, cause := cpu.translateAddr(0x1000, ACCESS_READ)
	if !fault {
		t.Fatal("expected fault for not-present page")
	}
	if cause != FAULT_NOT_PRESENT {
		t.Fatalf("fault cause = %d, want %d", cause, FAULT_NOT_PRESENT)
	}
}

func TestMMU_WalkReadDenied(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000

	// Present but no R bit
	writePTE(cpu, 1, makePTE(5, PTE_P|PTE_W))

	_, fault, cause := cpu.translateAddr(0x1000, ACCESS_READ)
	if !fault {
		t.Fatal("expected fault for read-denied page")
	}
	if cause != FAULT_READ_DENIED {
		t.Fatalf("fault cause = %d, want %d", cause, FAULT_READ_DENIED)
	}
}

func TestMMU_WalkWriteDenied(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000

	// Present + R but no W
	writePTE(cpu, 1, makePTE(5, PTE_P|PTE_R))

	_, fault, cause := cpu.translateAddr(0x1000, ACCESS_WRITE)
	if !fault {
		t.Fatal("expected fault for write-denied page")
	}
	if cause != FAULT_WRITE_DENIED {
		t.Fatalf("fault cause = %d, want %d", cause, FAULT_WRITE_DENIED)
	}
}

func TestMMU_WalkExecDenied(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000

	// Present + R + W but no X
	writePTE(cpu, 1, makePTE(5, PTE_P|PTE_R|PTE_W))

	_, fault, cause := cpu.translateAddr(0x1000, ACCESS_EXEC)
	if !fault {
		t.Fatal("expected fault for exec-denied page")
	}
	if cause != FAULT_EXEC_DENIED {
		t.Fatalf("fault cause = %d, want %d", cause, FAULT_EXEC_DENIED)
	}
}

func TestMMU_WalkUserOnSuperPage(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.supervisorMode = false // user mode
	cpu.ptbr = 0x20000

	// Present + R + W + X but no U
	writePTE(cpu, 1, makePTE(5, PTE_P|PTE_R|PTE_W|PTE_X))

	_, fault, cause := cpu.translateAddr(0x1000, ACCESS_READ)
	if !fault {
		t.Fatal("expected fault for user accessing supervisor page")
	}
	if cause != FAULT_USER_SUPER {
		t.Fatalf("fault cause = %d, want %d", cause, FAULT_USER_SUPER)
	}
}

func TestMMU_WalkSupervisorBypass(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.supervisorMode = true // supervisor mode
	cpu.ptbr = 0x20000

	// Present + R + W + X but no U (supervisor-only page)
	writePTE(cpu, 1, makePTE(5, PTE_P|PTE_R|PTE_W|PTE_X))

	phys, fault, _ := cpu.translateAddr(0x1000, ACCESS_READ)
	if fault {
		t.Fatal("supervisor should be able to access U=0 page")
	}
	if phys != 0x5000 {
		t.Fatalf("translateAddr = 0x%X, want 0x5000", phys)
	}
}

func TestMMU_WalkAddressCalc(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000

	tests := []struct {
		vpn    uint64
		ppn    uint64
		offset uint32
	}{
		{0, 0, 0},
		{0, 0, 0xFFF},
		{1, 10, 0x123},
		{100, 200, 0x456},
		{8191, 8191, 0xFFF},
	}

	for _, tt := range tests {
		writePTE(cpu, tt.vpn, makePTE(tt.ppn, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))
		vaddr := (tt.vpn << MMU_PAGE_SHIFT) | uint64(tt.offset)
		expectedPhys := (tt.ppn << MMU_PAGE_SHIFT) | uint64(tt.offset)

		phys, fault, _ := cpu.translateAddr(vaddr, ACCESS_READ)
		if fault {
			t.Fatalf("unexpected fault for vpn=%d ppn=%d offset=0x%X", tt.vpn, tt.ppn, tt.offset)
		}
		if phys != expectedPhys {
			t.Fatalf("vpn=%d ppn=%d offset=0x%X: got phys=0x%X, want 0x%X",
				tt.vpn, tt.ppn, tt.offset, phys, expectedPhys)
		}
	}
}

// ===========================================================================
// MMU Phase 3: Interpreter Integration
// ===========================================================================

// setupIdentityMMU sets up an identity-mapped page table covering the first
// N pages. Page table is placed at physical 0x80000. Code/stack pages get
// R+W+X+U. PLAN_MAX_RAM.md slice 4 design: builds the multi-level walk
// structure via mmuMap; intermediate tables auto-allocate from the
// per-PTBR pool. After identity-mapping the data pages, the PT region
// itself (top table + all allocated intermediates) is identity-mapped so
// guest code that observes VA == PA inside the PT region still works.
func setupIdentityMMU(cpu *CPU64, numPages int) {
	cpu.ptbr = uint64(0x80000) // well above PROG_START, below IO_REGION_START
	cpu.mmuEnabled = true
	flags := byte(PTE_P | PTE_R | PTE_W | PTE_X | PTE_U)

	for i := 0; i < numPages; i++ {
		mmuMap(cpu, uint64(i)<<MMU_PAGE_SHIFT, uint64(i), flags)
	}

	// Identity-map the actual PT region (top table + every intermediate
	// allocated above). The pool cursor recorded by mmuTestNextTable is
	// the next free physical address; the in-use range is [ptbr, cursor).
	poolEnd := mmuTestState[cpu.ptbr]
	if poolEnd == 0 {
		poolEnd = cpu.ptbr + mmuTestPoolBaseOffset
	}
	ptStartPage := cpu.ptbr >> MMU_PAGE_SHIFT
	ptEndPage := (poolEnd - 1) >> MMU_PAGE_SHIFT
	for p := ptStartPage; p <= ptEndPage; p++ {
		mmuMap(cpu, p<<MMU_PAGE_SHIFT, p, flags)
	}
}

func TestMMU_DisabledByDefault(t *testing.T) {
	rig := newIE64TestRig()

	// Without MMU, LOAD/STORE works normally
	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x2000), // R1 = 0x2000
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xABCD), // R2 = 0xABCD
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0),     // [R1] = R2
		ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 0, 1, 0, 0),      // R3 = [R1]
	)

	if rig.cpu.regs[3] != 0xABCD {
		t.Fatalf("disabled MMU: R3 = 0x%X, want 0xABCD", rig.cpu.regs[3])
	}
}

func TestMMU_LoadWithTranslation(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu

	// Set up MMU: map virtual page 2 -> physical page 5
	cpu.ptbr = 0x80000
	cpu.mmuEnabled = true
	writePTE(cpu, 2, makePTE(5, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	// Also need to map the page containing PROG_START (page 0 at 0x1000 = page 0)
	// and the stack page, and the page table page
	setupIdentityMMU(cpu, 160) // covers up to 0xA0000

	// Write a known value at physical address 0x5100
	binary.LittleEndian.PutUint32(cpu.memory[0x5100:], 0xDEADBEEF)

	// Map virtual page 2 -> physical page 5 (override identity for this page)
	writePTE(cpu, 2, makePTE(5, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	// LOAD from virtual address 0x2100 should translate to physical 0x5100
	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x2100), // R1 = 0x2100 (virtual)
		ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0),      // R2 = [R1]
	)

	if rig.cpu.regs[2] != 0xDEADBEEF {
		t.Fatalf("MMU LOAD: R2 = 0x%X, want 0xDEADBEEF", rig.cpu.regs[2])
	}
}

func TestMMU_StoreWithTranslation(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	// Map virtual page 3 -> physical page 7
	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	// STORE to virtual 0x3200 should write to physical 0x7200
	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3200),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xCAFE),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0),
	)

	got := binary.LittleEndian.Uint32(cpu.memory[0x7200:])
	if got != 0xCAFE {
		t.Fatalf("MMU STORE: phys 0x7200 = 0x%X, want 0xCAFE", got)
	}
}

func TestMMU_LoadPageFault(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Clear PTE for page 4 (make it not present)
	writePTE(cpu, 4, 0) // P=0

	// LOAD from virtual 0x4000 should page fault
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x4000),
		ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0),
	)
	rig.cpu.running.Store(true)
	rig.cpu.Execute()

	if cpu.faultCause != FAULT_NOT_PRESENT {
		t.Fatalf("page fault cause = %d, want %d", cpu.faultCause, FAULT_NOT_PRESENT)
	}
	if cpu.faultAddr != 0x4000 {
		t.Fatalf("faultAddr = 0x%X, want 0x4000", cpu.faultAddr)
	}
}

func TestMMU_StoreReadOnlyFault(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Make page 4 read-only (no W bit)
	writePTE(cpu, 4, makePTE(4, PTE_P|PTE_R|PTE_X|PTE_U))

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x4000),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xFF),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0),
	)
	rig.cpu.running.Store(true)
	rig.cpu.Execute()

	if cpu.faultCause != FAULT_WRITE_DENIED {
		t.Fatalf("store fault cause = %d, want %d", cpu.faultCause, FAULT_WRITE_DENIED)
	}
}

func TestMMU_FetchExecFault(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Map PROG_START page as non-executable (R+W but no X)
	progPage := uint64(PROG_START >> MMU_PAGE_SHIFT)
	writePTE(cpu, progPage, makePTE(progPage, PTE_P|PTE_R|PTE_W|PTE_U)) // no X

	rig.loadInstructions(ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))
	rig.cpu.running.Store(true)
	rig.cpu.Execute()

	if cpu.faultCause != FAULT_EXEC_DENIED {
		t.Fatalf("fetch fault cause = %d, want %d", cpu.faultCause, FAULT_EXEC_DENIED)
	}
}

func TestMMU_IdentityMap(t *testing.T) {
	rig := newIE64TestRig()
	setupIdentityMMU(rig.cpu, 160)

	// With identity mapping, behavior should be same as no MMU
	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3000),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0x1234),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 0, 1, 0, 0),
	)

	if rig.cpu.regs[3] != 0x1234 {
		t.Fatalf("identity-mapped LOAD/STORE: R3 = 0x%X, want 0x1234", rig.cpu.regs[3])
	}
}

func TestMMU_StackIsNonExecutable(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Map stack page as non-executable (R+W only)
	stackPage := uint64(0x9E000 >> MMU_PAGE_SHIFT)                        // near STACK_START
	writePTE(cpu, stackPage, makePTE(stackPage, PTE_P|PTE_R|PTE_W|PTE_U)) // no X

	// Jump to the stack page - should fault
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(stackPage)<<MMU_PAGE_SHIFT),
		ie64Instr(OP_JMP, 0, 0, 0, 1, 0, 0),
	)
	rig.cpu.running.Store(true)
	rig.cpu.Execute()

	if cpu.faultCause != FAULT_EXEC_DENIED {
		t.Fatalf("stack exec fault: cause = %d, want %d", cpu.faultCause, FAULT_EXEC_DENIED)
	}
}

func TestMMU_JSR_StackTranslation(t *testing.T) {
	rig := newIE64TestRig()
	setupIdentityMMU(rig.cpu, 160)

	// JSR should push return address through MMU
	// JSR at PROG_START jumps forward by 16 bytes to the RTS at PROG_START+16
	// Return address is PROG_START+8 (instruction after JSR)
	rig.loadInstructions(
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(int32(2*IE64_INSTR_SIZE))), // JSR -> PROG_START+16
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),                               // return lands here (PROG_START+8)
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0),                                // subroutine at PROG_START+16
	)
	rig.cpu.running.Store(true)
	rig.cpu.Execute()

	// Should have returned to PROG_START + 8 (instruction after JSR)
	expectedReturnPC := uint64(PROG_START) + IE64_INSTR_SIZE
	if rig.cpu.PC != expectedReturnPC {
		t.Fatalf("JSR/RTS with MMU: PC = 0x%X, want 0x%X", rig.cpu.PC, expectedReturnPC)
	}
}

// ===========================================================================
// MMU Phase 4: Software TLB
// ===========================================================================

func TestTLB_MissTriggersWalk(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000

	// Set up a valid PTE
	writePTE(cpu, 5, makePTE(10, PTE_P|PTE_R|PTE_U))

	// TLB should be empty initially
	if _, hit := cpu.tlbLookup(5); hit {
		t.Fatal("TLB should be empty initially")
	}

	// Translate should succeed (walks page table, fills TLB)
	phys, fault, _ := cpu.translateAddr(0x5000, ACCESS_READ)
	if fault {
		t.Fatal("unexpected fault")
	}
	if phys != 0xA000 {
		t.Fatalf("phys = 0x%X, want 0xA000", phys)
	}

	// TLB should now have the entry
	entry, hit := cpu.tlbLookup(5)
	if !hit {
		t.Fatal("TLB miss after walk should have filled entry")
	}
	if entry.ppn != 10 {
		t.Fatalf("TLB entry ppn = %d, want 10", entry.ppn)
	}
}

func TestTLB_HitReturnsCached(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000

	writePTE(cpu, 5, makePTE(10, PTE_P|PTE_R|PTE_U))

	// First access fills TLB
	cpu.translateAddr(0x5000, ACCESS_READ)

	// Now corrupt the PTE in memory (but TLB should still have cached value)
	writePTE(cpu, 5, 0) // clear PTE

	// Second access should hit TLB (not walk the now-cleared PTE)
	phys, fault, _ := cpu.translateAddr(0x5123, ACCESS_READ)
	if fault {
		t.Fatal("TLB hit should not fault even though PTE was cleared")
	}
	if phys != 0xA123 {
		t.Fatalf("TLB hit: phys = 0x%X, want 0xA123", phys)
	}
}

func TestTLB_FlushClearsAll(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000

	writePTE(cpu, 5, makePTE(10, PTE_P|PTE_R|PTE_U))
	cpu.translateAddr(0x5000, ACCESS_READ) // fill TLB

	cpu.tlbFlush()

	if _, hit := cpu.tlbLookup(5); hit {
		t.Fatal("TLB should be empty after flush")
	}

	// Now with PTE cleared, translation should fault
	writePTE(cpu, 5, 0)
	_, fault, _ := cpu.translateAddr(0x5000, ACCESS_READ)
	if !fault {
		t.Fatal("should fault after TLB flush with cleared PTE")
	}
}

func TestTLB_InvalSingleEntry(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000

	// Fill TLB entries for pages 5 and 69 (both map to TLB index 5: 5&63=5, 69&63=5)
	// Actually, use non-conflicting entries: page 5 (idx=5) and page 10 (idx=10)
	writePTE(cpu, 5, makePTE(10, PTE_P|PTE_R|PTE_U))
	writePTE(cpu, 10, makePTE(20, PTE_P|PTE_R|PTE_U))
	cpu.translateAddr(0x5000, ACCESS_READ) // fill for page 5
	cpu.translateAddr(0xA000, ACCESS_READ) // fill for page 10

	// Invalidate only page 5
	cpu.tlbInvalidate(5)

	if _, hit := cpu.tlbLookup(5); hit {
		t.Fatal("page 5 should be invalidated")
	}
	if _, hit := cpu.tlbLookup(10); !hit {
		t.Fatal("page 10 should still be cached")
	}
}

func TestTLB_PTBRChangeFlushes(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000

	writePTE(cpu, 5, makePTE(10, PTE_P|PTE_R|PTE_U))
	cpu.translateAddr(0x5000, ACCESS_READ) // fill TLB

	// Changing PTBR should flush TLB (done by MTCR handler)
	// Simulate what MTCR CR_PTBR does:
	cpu.ptbr = 0x30000
	cpu.tlbFlush()

	if _, hit := cpu.tlbLookup(5); hit {
		t.Fatal("TLB should be flushed after PTBR change")
	}
}

func TestTLB_PermissionsChecked(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.supervisorMode = false // user mode
	cpu.ptbr = 0x20000

	// Map page 5 with read-only, user access
	writePTE(cpu, 5, makePTE(10, PTE_P|PTE_R|PTE_U))

	// First access fills TLB (read succeeds)
	_, fault, _ := cpu.translateAddr(0x5000, ACCESS_READ)
	if fault {
		t.Fatal("read should succeed")
	}

	// Write should still fail (TLB has cached flags, W=0)
	_, fault, cause := cpu.translateAddr(0x5000, ACCESS_WRITE)
	if !fault {
		t.Fatal("write to read-only page should fault even on TLB hit")
	}
	if cause != FAULT_WRITE_DENIED {
		t.Fatalf("cause = %d, want %d", cause, FAULT_WRITE_DENIED)
	}
}

// ===========================================================================
// MMU Phase 8: Integration Tests
// ===========================================================================

func TestMMU_KernelBootstrap(t *testing.T) {
	// Simulate kernel bootstrap: set up page table, set trap vector, enable MMU
	rig := newIE64TestRig()
	cpu := rig.cpu

	// Place page table at 0x80000 and identity-map the first 160 pages
	// (covers code, stack, page table region) via the multi-level walk.
	ptBase := uint32(0x80000)
	cpu.ptbr = uint64(ptBase)
	for i := 0; i < 160; i++ {
		mmuMap(cpu, uint64(i)<<MMU_PAGE_SHIFT, uint64(i), PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)
	}

	// Trap handler at 0x9000
	trapAddr := uint64(0x9000)
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Bootstrap program:
	// 1. Set trap vector
	// 2. Set PTBR
	// 3. Enable MMU
	// 4. Do a LOAD through MMU to verify it works
	// 5. HALT
	rig.loadInstructions(
		// R1 = trap vector address
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(trapAddr)),
		ie64Instr(OP_MTCR, CR_TRAP_VEC, 0, 0, 1, 0, 0),
		// R1 = PTBR
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, ptBase),
		ie64Instr(OP_MTCR, CR_PTBR, 0, 0, 1, 0, 0),
		// Enable MMU (R1 = 1)
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 1),
		ie64Instr(OP_MTCR, CR_MMU_CTRL, 0, 0, 1, 0, 0),
		// Write a test value and read it back through MMU
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3000),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0x1234),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 0, 1, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.regs[3] != 0x1234 {
		t.Fatalf("kernel bootstrap: R3 = 0x%X, want 0x1234", cpu.regs[3])
	}
	if !cpu.mmuEnabled {
		t.Fatal("MMU should be enabled after bootstrap")
	}
}

func TestMMU_SyscallDoesNotReexecute(t *testing.T) {
	// Verify that SYSCALL + ERET does NOT re-execute the SYSCALL.
	// This is the key correctness property of differentiated trap semantics.
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr

	// Trap handler: increment R10, then ERET
	copy(cpu.memory[trapAddr:], ie64Instr(OP_ADD, 10, IE64_SIZE_L, 1, 10, 0, 1))
	copy(cpu.memory[trapAddr+8:], ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Program: R10=0, SYSCALL, HALT
	// If SYSCALL re-executed on ERET, R10 would be > 1
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 10, IE64_SIZE_L, 1, 0, 0, 0),
		ie64Instr(OP_SYSCALL, 0, 0, 1, 0, 0, 1),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.regs[10] != 1 {
		t.Fatalf("SYSCALL re-execution: R10 = %d, want 1 (handler ran once)", cpu.regs[10])
	}
}

func TestMMU_PageFaultHandlerRestart(t *testing.T) {
	// Simulate: access unmapped page -> fault -> handler maps it -> ERET -> retry succeeds
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	// Unmap page 4 by clearing its leaf PTE. The intermediate tables
	// stay in place so the trap handler can walk to the same leaf
	// physical address and re-install the entry.
	mmuClearLeafPTE(cpu, 4<<MMU_PAGE_SHIFT)

	// Write a known value at physical page 4 address
	binary.LittleEndian.PutUint32(cpu.memory[0x4100:], 0xBEEFCAFE)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr

	// Trap handler: map page 4 with identity mapping, then ERET. The
	// PT format is multi-level, so the handler must store the PTE at
	// the leaf entry's physical address (computed Go-side from the
	// pre-built intermediate chain) rather than at ptbr + vpn*8.
	handlerOff := uint32(trapAddr)

	// R20 = page table entry value for page 4 (identity mapped, PRWXU)
	pte4 := makePTE(4, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)
	// R21 = leaf PTE address, walked from ptbr through the existing multi-level chain.
	ptEntryAddr := mmuLeafAddress(cpu, 4<<MMU_PAGE_SHIFT)
	if ptEntryAddr == 0 {
		t.Fatalf("leaf address for VPN 4 not reachable; intermediate chain missing")
	}

	copy(cpu.memory[handlerOff:], ie64Instr(OP_MOVE, 20, IE64_SIZE_L, 1, 0, 0, uint32(pte4)))
	handlerOff += 8
	copy(cpu.memory[handlerOff:], ie64Instr(OP_MOVT, 20, 0, 1, 0, 0, uint32(pte4>>32)))
	handlerOff += 8
	copy(cpu.memory[handlerOff:], ie64Instr(OP_MOVE, 21, IE64_SIZE_L, 1, 0, 0, uint32(ptEntryAddr)))
	handlerOff += 8
	copy(cpu.memory[handlerOff:], ie64Instr(OP_STORE, 20, IE64_SIZE_Q, 0, 21, 0, 0))
	handlerOff += 8
	// TLBINVAL for page 4 (R22 = 4 << 12 = 0x4000)
	copy(cpu.memory[handlerOff:], ie64Instr(OP_MOVE, 22, IE64_SIZE_L, 1, 0, 0, 0x4000))
	handlerOff += 8
	copy(cpu.memory[handlerOff:], ie64Instr(OP_TLBINVAL, 0, 0, 0, 22, 0, 0))
	handlerOff += 8
	copy(cpu.memory[handlerOff:], ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Main program: load from virtual 0x4100
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x4100),
		ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0), // faults, handler maps, ERET retries
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.regs[2] != 0xBEEFCAFE {
		t.Fatalf("page fault restart: R2 = 0x%X, want 0xBEEFCAFE", cpu.regs[2])
	}
}

func TestMMU_UserProcessIsolation(t *testing.T) {
	// Two "processes" with different page tables can't see each other's memory
	rig := newIE64TestRig()
	cpu := rig.cpu

	// Process 1 page table at 0x80000, process 2 at 0x90000. The pools that
	// hold each process's intermediate-level tables grow 64 KiB beyond the
	// top-level base, so the PTBRs must be at least 64 KiB apart.
	pt1 := uint64(0x80000)
	pt2 := uint64(0x90000)

	// Identity-map the first 160 pages in each process via the multi-level
	// walk; mmuMap allocates intermediate tables from the per-PTBR pool.
	flags := byte(PTE_P | PTE_R | PTE_W | PTE_X | PTE_U)
	cpu.ptbr = pt1
	for i := 0; i < 160; i++ {
		mmuMap(cpu, uint64(i)<<MMU_PAGE_SHIFT, uint64(i), flags)
	}
	cpu.ptbr = pt2
	for i := 0; i < 160; i++ {
		mmuMap(cpu, uint64(i)<<MMU_PAGE_SHIFT, uint64(i), flags)
	}

	// Override virtual page 10 so each process sees a different physical
	// page: process 1 → physical 20, process 2 → physical 30.
	cpu.ptbr = pt1
	mmuMap(cpu, 10<<MMU_PAGE_SHIFT, 20, flags)
	cpu.ptbr = pt2
	mmuMap(cpu, 10<<MMU_PAGE_SHIFT, 30, flags)

	// Write different values at physical pages 20 and 30
	binary.LittleEndian.PutUint32(cpu.memory[20*MMU_PAGE_SIZE:], 0xAAAA)
	binary.LittleEndian.PutUint32(cpu.memory[30*MMU_PAGE_SIZE:], 0xBBBB)

	// Test with process 1 page table
	cpu.mmuEnabled = true
	cpu.ptbr = pt1
	phys1, fault1, _ := cpu.translateAddr(10*MMU_PAGE_SIZE, ACCESS_READ)
	if fault1 {
		t.Fatal("process 1 translation faulted")
	}
	val1 := binary.LittleEndian.Uint32(cpu.memory[phys1:])

	// Switch to process 2 page table
	cpu.ptbr = pt2
	cpu.tlbFlush()
	phys2, fault2, _ := cpu.translateAddr(10*MMU_PAGE_SIZE, ACCESS_READ)
	if fault2 {
		t.Fatal("process 2 translation faulted")
	}
	val2 := binary.LittleEndian.Uint32(cpu.memory[phys2:])

	if val1 != 0xAAAA {
		t.Fatalf("process 1 saw 0x%X, want 0xAAAA", val1)
	}
	if val2 != 0xBBBB {
		t.Fatalf("process 2 saw 0x%X, want 0xBBBB", val2)
	}
	if phys1 == phys2 {
		t.Fatal("both processes mapped to same physical address")
	}
}

func TestMMU_BackwardCompat(t *testing.T) {
	// Verify existing programs still work with MMU code present but disabled
	rig := newIE64TestRig()

	// Fibonacci-like computation without MMU
	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 1), // R1 = 1
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 1), // R2 = 1
		ie64Instr(OP_ADD, 3, IE64_SIZE_L, 0, 1, 2, 0),  // R3 = R1 + R2
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 0, 2, 0, 0), // R1 = R2
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 0, 3, 0, 0), // R2 = R3
		ie64Instr(OP_ADD, 3, IE64_SIZE_L, 0, 1, 2, 0),  // R3 = R1 + R2
	)

	if rig.cpu.mmuEnabled {
		t.Fatal("MMU should be disabled by default")
	}
	if rig.cpu.regs[3] != 3 {
		t.Fatalf("backward compat: R3 = %d, want 3", rig.cpu.regs[3])
	}
}

// ===========================================================================
// Missing Phase 3 Tests: Individual Stack/FPU/NX Operations
// ===========================================================================

func TestMMU_StackPushTranslation(t *testing.T) {
	rig := newIE64TestRig()
	setupIdentityMMU(rig.cpu, 160)

	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0xABCD),
		ie64Instr(OP_PUSH64, 0, 0, 0, 1, 0, 0),
		ie64Instr(OP_POP64, 2, 0, 0, 0, 0, 0),
	)
	if rig.cpu.regs[2] != 0xABCD {
		t.Fatalf("PUSH/POP through MMU: R2 = 0x%X, want 0xABCD", rig.cpu.regs[2])
	}
}

func TestMMU_StackPopTranslation(t *testing.T) {
	rig := newIE64TestRig()
	setupIdentityMMU(rig.cpu, 160)

	// Push two values, pop them back in reverse
	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x1111),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0x2222),
		ie64Instr(OP_PUSH64, 0, 0, 0, 1, 0, 0),
		ie64Instr(OP_PUSH64, 0, 0, 0, 2, 0, 0),
		ie64Instr(OP_POP64, 3, 0, 0, 0, 0, 0),
		ie64Instr(OP_POP64, 4, 0, 0, 0, 0, 0),
	)
	if rig.cpu.regs[3] != 0x2222 {
		t.Fatalf("POP first: R3 = 0x%X, want 0x2222", rig.cpu.regs[3])
	}
	if rig.cpu.regs[4] != 0x1111 {
		t.Fatalf("POP second: R4 = 0x%X, want 0x1111", rig.cpu.regs[4])
	}
}

func TestMMU_RTS_StackTranslation(t *testing.T) {
	rig := newIE64TestRig()
	setupIdentityMMU(rig.cpu, 160)

	// JSR then RTS through MMU
	// JSR at 0x1000 jumps +24 to 0x1018 (RTS). Return addr = 0x1008 (MOVE).
	rig.loadInstructions(
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, uint32(int32(3*IE64_INSTR_SIZE))),
		ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, 0x99), // after RTS lands here
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0), // subroutine at 0x1018
	)
	rig.cpu.running.Store(true)
	rig.cpu.Execute()

	if rig.cpu.regs[5] != 0x99 {
		t.Fatalf("RTS through MMU: R5 = 0x%X, want 0x99", rig.cpu.regs[5])
	}
}

func TestMMU_JSR_IND_StackTranslation(t *testing.T) {
	rig := newIE64TestRig()
	setupIdentityMMU(rig.cpu, 160)

	// Place a RTS at 0x5000
	copy(rig.cpu.memory[0x5000:], ie64Instr(OP_RTS64, 0, 0, 0, 0, 0, 0))

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x5000),
		ie64Instr(OP_JSR_IND, 0, 0, 0, 1, 0, 0),           // JSR indirect to 0x5000
		ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, 0x77), // return address
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	rig.cpu.running.Store(true)
	rig.cpu.Execute()

	if rig.cpu.regs[5] != 0x77 {
		t.Fatalf("JSR_IND through MMU: R5 = 0x%X, want 0x77", rig.cpu.regs[5])
	}
}

func TestMMU_StackFault(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Make the stack page read-only (no write)
	stackPage := uint64((STACK_START - 8) >> MMU_PAGE_SHIFT)
	writePTE(cpu, stackPage, makePTE(stackPage, PTE_P|PTE_R|PTE_X|PTE_U)) // no W

	// PUSH should fault on stack write
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x42),
		ie64Instr(OP_PUSH64, 0, 0, 0, 1, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.faultCause != FAULT_WRITE_DENIED {
		t.Fatalf("stack fault: cause = %d, want %d", cpu.faultCause, FAULT_WRITE_DENIED)
	}
}

func TestMMU_FLOADTranslation(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	// Map virtual page 3 -> physical page 7
	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	// Write a float32 at physical 0x7000
	binary.LittleEndian.PutUint32(cpu.memory[0x7000:], 0x3F800000) // 1.0f

	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3000), // virtual
		ie64Instr(OP_FLOAD, 0, 0, 0, 1, 0, 0),               // F0 = [0x3000]
	)

	if cpu.FPU.FPRegs[0] != 0x3F800000 {
		t.Fatalf("FLOAD through MMU: F0 = 0x%X, want 0x3F800000", cpu.FPU.FPRegs[0])
	}
}

func TestMMU_FSTORETranslation(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	// Set F0 to a known value, then FSTORE through MMU
	cpu.FPU.FPRegs[0] = 0x40000000 // 2.0f

	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3100),
		ie64Instr(OP_FSTORE, 0, 0, 0, 1, 0, 0), // [0x3100] = F0
	)

	got := binary.LittleEndian.Uint32(cpu.memory[0x7100:])
	if got != 0x40000000 {
		t.Fatalf("FSTORE through MMU: phys 0x7100 = 0x%X, want 0x40000000", got)
	}
}

func TestMMU_HeapIsNonExecutable(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Map page 5 as data (R+W, no X)
	writePTE(cpu, 5, makePTE(5, PTE_P|PTE_R|PTE_W|PTE_U))

	// Jump to page 5
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x5000),
		ie64Instr(OP_JMP, 0, 0, 0, 1, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.faultCause != FAULT_EXEC_DENIED {
		t.Fatalf("heap NX: cause = %d, want %d", cpu.faultCause, FAULT_EXEC_DENIED)
	}
}

func TestMMU_UserCannotExecuteDataPage(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Map page 6 as user data (R+W, no X, user-accessible)
	writePTE(cpu, 6, makePTE(6, PTE_P|PTE_R|PTE_W|PTE_U))

	// Write NOP at page 6
	copy(cpu.memory[0x6000:], ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))

	// Switch to user mode, then jump to page 6
	cpu.faultPC = PROG_START + IE64_INSTR_SIZE
	rig.loadInstructions(
		ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0), // switch to user
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x6000),
		ie64Instr(OP_JMP, 0, 0, 0, 1, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.faultCause != FAULT_EXEC_DENIED {
		t.Fatalf("user exec data page: cause = %d, want %d", cpu.faultCause, FAULT_EXEC_DENIED)
	}
}

func TestMMU_KernelTextExec_UserDataNX(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	// Kernel text page (PROG_START page): already has X from identity map
	// Data page 6: map as R+W only (no X)
	writePTE(cpu, 6, makePTE(6, PTE_P|PTE_R|PTE_W|PTE_U))

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Kernel code executes fine (X=1), then jumps to data page (X=0) -> faults
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, 0xAA), // kernel code works
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x6000),
		ie64Instr(OP_JMP, 0, 0, 0, 1, 0, 0), // jump to data page
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.regs[5] != 0xAA {
		t.Fatalf("kernel text didn't execute: R5 = 0x%X", cpu.regs[5])
	}
	if cpu.faultCause != FAULT_EXEC_DENIED {
		t.Fatalf("data page exec: cause = %d, want %d", cpu.faultCause, FAULT_EXEC_DENIED)
	}
}

func TestMMU_PrivViolationRestart(t *testing.T) {
	// Handler can emulate: MTCR faultPC=faultPC+8 then ERET skips the faulting instr
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr

	// Trap handler: read faultPC, add 8, write back, ERET
	copy(cpu.memory[trapAddr:], ie64Instr(OP_MFCR, 20, 0, 0, CR_FAULT_PC, 0, 0))
	copy(cpu.memory[trapAddr+8:], ie64Instr(OP_ADD, 20, IE64_SIZE_Q, 1, 20, 0, 8))
	copy(cpu.memory[trapAddr+16:], ie64Instr(OP_MTCR, CR_FAULT_PC, 0, 0, 20, 0, 0))
	copy(cpu.memory[trapAddr+24:], ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	// Program: ERET to user, MTCR (faults), next instruction sets R5
	cpu.faultPC = PROG_START + IE64_INSTR_SIZE
	rig.loadInstructions(
		ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0),              // switch to user
		ie64Instr(OP_MTCR, CR_PTBR, 0, 0, 1, 0, 0),        // priv violation
		ie64Instr(OP_MOVE, 5, IE64_SIZE_L, 1, 0, 0, 0xBB), // handler skips to here
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.regs[5] != 0xBB {
		t.Fatalf("priv violation skip: R5 = 0x%X, want 0xBB", cpu.regs[5])
	}
}

func TestMMU_StackProtection(t *testing.T) {
	// User-mode stack ops should respect page permissions
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Make stack page supervisor-only (no U bit)
	stackPage := uint64((STACK_START - 8) >> MMU_PAGE_SHIFT)
	writePTE(cpu, stackPage, makePTE(stackPage, PTE_P|PTE_R|PTE_W|PTE_X)) // no U

	// Switch to user mode with SP pointing at the supervisor-only stack page.
	// ERET will restore userSP into R31, so set userSP to the supervisor stack.
	cpu.faultPC = PROG_START + IE64_INSTR_SIZE
	cpu.userSP = STACK_START   // points to supervisor-only page
	cpu.kernelSP = STACK_START // need a valid KSP for the ERET
	rig.loadInstructions(
		ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_JSR64, 0, 0, 1, 0, 0, 8), // try to push to supervisor-only stack
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.faultCause != FAULT_USER_SUPER {
		t.Fatalf("stack protection: cause = %d, want %d", cpu.faultCause, FAULT_USER_SUPER)
	}
}

// ===========================================================================
// Atomic Operations + MMU
// ===========================================================================

func TestMMU_CAS_WithTranslation(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	// Map virtual page 3 -> physical page 7
	writePTE(cpu, 3, makePTE(7, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	// Write value at physical 0x7000
	binary.LittleEndian.PutUint64(cpu.memory[0x7000:], 100)

	// CAS through virtual 0x3000
	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3000),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 100),
		ie64Instr(OP_MOVE, 3, IE64_SIZE_L, 1, 0, 0, 200),
		ie64Instr(OP_CAS, 2, 0, 0, 1, 3, 0),
	)

	got := binary.LittleEndian.Uint64(cpu.memory[0x7000:])
	if got != 200 {
		t.Fatalf("MMU CAS: phys 0x7000 = %d, want 200", got)
	}
}

func TestMMU_Atomic_PageFault(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Unmap page 4
	writePTE(cpu, 4, 0)

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x4000),
		ie64Instr(OP_CAS, 2, 0, 0, 1, 3, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.faultCause != FAULT_NOT_PRESENT {
		t.Fatalf("atomic page fault: cause = %d, want %d", cpu.faultCause, FAULT_NOT_PRESENT)
	}
}

func TestMMU_Atomic_WritePermissionDenied(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Page 4: read-only (no W)
	writePTE(cpu, 4, makePTE(4, PTE_P|PTE_R|PTE_X|PTE_U))

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x4000),
		ie64Instr(OP_FAA, 2, 0, 0, 1, 3, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.faultCause != FAULT_WRITE_DENIED {
		t.Fatalf("atomic write denied: cause = %d, want %d", cpu.faultCause, FAULT_WRITE_DENIED)
	}
}

func TestMMU_Atomic_MisalignedWithMMU(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// Misalignment is checked before MMU translation
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3001),
		ie64Instr(OP_CAS, 2, 0, 0, 1, 3, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.faultCause != FAULT_MISALIGNED {
		t.Fatalf("MMU misaligned: cause = %d, want %d", cpu.faultCause, FAULT_MISALIGNED)
	}
}

// ===========================================================================
// TLS Register — User Mode Access
// ===========================================================================

func TestIE64_CR_TP_UserRead(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	cpu.threadPointer = 0x77777

	// ERET to user mode, then MFCR r5, cr6 should succeed
	cpu.faultPC = PROG_START + IE64_INSTR_SIZE
	rig.loadInstructions(
		ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_MFCR, 5, 0, 0, CR_TP, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.regs[5] != 0x77777 {
		t.Fatalf("user CR_TP read: R5 = 0x%X, want 0x77777", cpu.regs[5])
	}
}

func TestIE64_CR_TP_UserWriteTraps(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// ERET to user mode, then MTCR cr6, r1 should trap
	cpu.faultPC = PROG_START + IE64_INSTR_SIZE
	rig.loadInstructions(
		ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_MTCR, CR_TP, 0, 0, 1, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.faultCause != FAULT_PRIV {
		t.Fatalf("user CR_TP write: cause = %d, want %d", cpu.faultCause, FAULT_PRIV)
	}
}

func TestIE64_CR_TP_OtherCR_StillDenied(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr
	copy(cpu.memory[trapAddr:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))

	// ERET to user, MFCR r1, cr0 should still fault
	cpu.faultPC = PROG_START + IE64_INSTR_SIZE
	rig.loadInstructions(
		ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_MFCR, 1, 0, 0, CR_PTBR, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.faultCause != FAULT_PRIV {
		t.Fatalf("user MFCR PTBR: cause = %d, want %d", cpu.faultCause, FAULT_PRIV)
	}
}

// ===========================================================================
// A/D Bits in Page Table Entries
// ===========================================================================

// readPTEFlags reads the PTE flags byte for a given VPN by walking the
// multi-level page table. Returns 0 if any level is unmapped.
func readPTEFlags(cpu *CPU64, vpn uint64) byte {
	pte := mmuLeafPTE(cpu, vpn<<MMU_PAGE_SHIFT)
	_, flags := parsePTE(pte)
	return flags
}

func TestMMU_AD_PTE_RoundTrip(t *testing.T) {
	// makePTE with A|D, parsePTE extracts them
	pte := makePTE(42, PTE_P|PTE_R|PTE_W|PTE_A|PTE_D)
	ppn, flags := parsePTE(pte)
	if ppn != 42 {
		t.Fatalf("PPN = %d, want 42", ppn)
	}
	if flags&PTE_A == 0 {
		t.Fatal("A bit not set")
	}
	if flags&PTE_D == 0 {
		t.Fatal("D bit not set")
	}
}

func TestMMU_AD_AccessedOnRead(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	vpn := uint64(3)
	// Verify A is not set initially
	flags := readPTEFlags(cpu, vpn)
	if flags&PTE_A != 0 {
		t.Fatal("A should not be set before access")
	}

	// LOAD from page 3
	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(vpn)<<MMU_PAGE_SHIFT),
		ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0),
	)

	flags = readPTEFlags(cpu, vpn)
	if flags&PTE_A == 0 {
		t.Fatal("A bit should be set after read")
	}
}

func TestMMU_AD_AccessedOnExec(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	// The PROG_START page should get A set during instruction fetch
	progVPN := uint64(PROG_START >> MMU_PAGE_SHIFT)

	rig.executeOne(ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))

	flags := readPTEFlags(cpu, progVPN)
	if flags&PTE_A == 0 {
		t.Fatal("A bit should be set after exec fetch")
	}
}

func TestMMU_AD_DirtyOnWrite(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	vpn := uint64(3)

	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(vpn)<<MMU_PAGE_SHIFT),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xFF),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0),
	)

	flags := readPTEFlags(cpu, vpn)
	if flags&PTE_A == 0 {
		t.Fatal("A bit should be set after write")
	}
	if flags&PTE_D == 0 {
		t.Fatal("D bit should be set after write")
	}
}

func TestMMU_AD_ReadDoesNotSetDirty(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	vpn := uint64(3)

	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(vpn)<<MMU_PAGE_SHIFT),
		ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0),
	)

	flags := readPTEFlags(cpu, vpn)
	if flags&PTE_A == 0 {
		t.Fatal("A should be set after read")
	}
	if flags&PTE_D != 0 {
		t.Fatal("D should NOT be set after read-only access")
	}
}

func TestMMU_AD_AlreadySet(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	vpn := uint64(3)
	// Pre-set A|D in PTE
	writePTE(cpu, vpn, makePTE(vpn, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U|PTE_A|PTE_D))

	// Save original PTE
	pteAddr := cpu.ptbr + vpn*8
	origPTE := binary.LittleEndian.Uint64(cpu.memory[pteAddr:])

	// Access the page
	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(vpn)<<MMU_PAGE_SHIFT),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xFF),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0),
	)

	// PTE should be unchanged (A|D already set, no write-back needed)
	newPTE := binary.LittleEndian.Uint64(cpu.memory[pteAddr:])
	if origPTE != newPTE {
		t.Fatalf("PTE changed when A|D already set: 0x%X -> 0x%X", origPTE, newPTE)
	}
}

func TestMMU_AD_TLBHitUpdates(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	vpn := uint64(3)

	// First access: TLB miss, fills TLB, sets A
	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(vpn)<<MMU_PAGE_SHIFT),
		ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0),
	)

	flags := readPTEFlags(cpu, vpn)
	if flags&PTE_A == 0 {
		t.Fatal("A not set after first read")
	}

	// Second access: TLB hit, STORE should set D
	rig2 := newIE64TestRig()
	rig2.cpu = cpu // reuse same CPU with warm TLB
	rig2.bus = rig.bus
	rig2.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(vpn)<<MMU_PAGE_SHIFT),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xFF),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	flags = readPTEFlags(cpu, vpn)
	if flags&PTE_D == 0 {
		t.Fatal("D not set after TLB-hit write")
	}
}

func TestMMU_AD_KernelClearsAD(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	setupIdentityMMU(cpu, 160)

	vpn := uint64(3)

	// Access to set A|D
	rig.executeN(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(vpn)<<MMU_PAGE_SHIFT),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xFF),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 1, 0, 0),
	)

	flags := readPTEFlags(cpu, vpn)
	if flags&(PTE_A|PTE_D) == 0 {
		t.Fatal("A|D should be set")
	}

	// Kernel clears A|D
	writePTE(cpu, vpn, makePTE(vpn, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))
	cpu.tlbFlush()

	flags = readPTEFlags(cpu, vpn)
	if flags&(PTE_A|PTE_D) != 0 {
		t.Fatal("A|D should be cleared")
	}

	// Next access should re-set A
	rig2 := newIE64TestRig()
	rig2.cpu = cpu
	rig2.bus = rig.bus
	rig2.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(vpn)<<MMU_PAGE_SHIFT),
		ie64Instr(OP_LOAD, 2, IE64_SIZE_L, 0, 1, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	flags = readPTEFlags(cpu, vpn)
	if flags&PTE_A == 0 {
		t.Fatal("A should be re-set after kernel cleared it")
	}
}

// ===========================================================================
// M15.6 G2: SMEP/SMAP-equivalent supervisor guards (SKEF / SKAC / SUA)
// ===========================================================================

// setupG2MMU prepares a rig with MMU enabled, a page table at 0x20000, and
// a mapping for VPN 1 with the caller-supplied PTE flags. Returns the rig
// so the caller can toggle SKEF/SKAC/SUA/supervisor and probe translation.
func setupG2MMU(t *testing.T, pteFlags byte) *ie64TestRig {
	t.Helper()
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.ptbr = 0x20000
	writePTE(cpu, 1, makePTE(5, pteFlags))
	return rig
}

func TestMMU_SKEF_BlocksSupervisorFetchFromUserPage(t *testing.T) {
	rig := setupG2MMU(t, PTE_P|PTE_R|PTE_X|PTE_U)
	cpu := rig.cpu
	cpu.supervisorMode = true
	cpu.skef = true

	_, fault, cause := cpu.translateAddr(0x1000, ACCESS_EXEC)
	if !fault {
		t.Fatal("expected SKEF fault for supervisor fetch from user page")
	}
	if cause != FAULT_SKEF {
		t.Fatalf("fault cause = %d, want FAULT_SKEF (%d)", cause, FAULT_SKEF)
	}
}

func TestMMU_SKEF_OffByDefault(t *testing.T) {
	rig := setupG2MMU(t, PTE_P|PTE_R|PTE_X|PTE_U)
	cpu := rig.cpu
	cpu.supervisorMode = true
	// skef is false; supervisor fetch from user page must succeed.

	if cpu.skef {
		t.Fatal("SKEF should be off by default")
	}
	_, fault, cause := cpu.translateAddr(0x1000, ACCESS_EXEC)
	if fault {
		t.Fatalf("unexpected fault (cause=%d) when SKEF is disabled", cause)
	}
}

func TestMMU_SKEF_DoesNotBlockSupervisorFetchFromKernelPage(t *testing.T) {
	// Kernel page = PTE_U is clear.
	rig := setupG2MMU(t, PTE_P|PTE_R|PTE_X)
	cpu := rig.cpu
	cpu.supervisorMode = true
	cpu.skef = true

	_, fault, _ := cpu.translateAddr(0x1000, ACCESS_EXEC)
	if fault {
		t.Fatal("SKEF must not block supervisor fetch from a kernel page")
	}
}

func TestMMU_SKEF_DoesNotBlockUserFetch(t *testing.T) {
	rig := setupG2MMU(t, PTE_P|PTE_R|PTE_X|PTE_U)
	cpu := rig.cpu
	cpu.supervisorMode = false
	cpu.skef = true // should not affect user mode

	_, fault, cause := cpu.translateAddr(0x1000, ACCESS_EXEC)
	if fault {
		t.Fatalf("SKEF must not affect user-mode fetches (cause=%d)", cause)
	}
}

func TestMMU_SKEF_DoesNotBlockDataAccess(t *testing.T) {
	// SKEF is execute-only; data accesses on the same page should not trip it.
	rig := setupG2MMU(t, PTE_P|PTE_R|PTE_W|PTE_U)
	cpu := rig.cpu
	cpu.supervisorMode = true
	cpu.skef = true
	// SKAC off — we're only testing SKEF's scope.

	_, fault, _ := cpu.translateAddr(0x1000, ACCESS_READ)
	if fault {
		t.Fatal("SKEF must not fault on data reads")
	}
	_, fault, _ = cpu.translateAddr(0x1000, ACCESS_WRITE)
	if fault {
		t.Fatal("SKEF must not fault on data writes")
	}
}

func TestMMU_SKAC_BlocksSupervisorReadFromUserPage(t *testing.T) {
	rig := setupG2MMU(t, PTE_P|PTE_R|PTE_W|PTE_U)
	cpu := rig.cpu
	cpu.supervisorMode = true
	cpu.skac = true
	cpu.suaLatch = false

	_, fault, cause := cpu.translateAddr(0x1000, ACCESS_READ)
	if !fault {
		t.Fatal("expected SKAC fault for supervisor read with SUA=0")
	}
	if cause != FAULT_SKAC {
		t.Fatalf("fault cause = %d, want FAULT_SKAC (%d)", cause, FAULT_SKAC)
	}
}

func TestMMU_SKAC_BlocksSupervisorWriteFromUserPage(t *testing.T) {
	rig := setupG2MMU(t, PTE_P|PTE_R|PTE_W|PTE_U)
	cpu := rig.cpu
	cpu.supervisorMode = true
	cpu.skac = true
	cpu.suaLatch = false

	_, fault, cause := cpu.translateAddr(0x1000, ACCESS_WRITE)
	if !fault {
		t.Fatal("expected SKAC fault for supervisor write with SUA=0")
	}
	if cause != FAULT_SKAC {
		t.Fatalf("fault cause = %d, want FAULT_SKAC (%d)", cause, FAULT_SKAC)
	}
}

func TestMMU_SKAC_AllowsSupervisorAccessWhenSUAEnabled(t *testing.T) {
	rig := setupG2MMU(t, PTE_P|PTE_R|PTE_W|PTE_U)
	cpu := rig.cpu
	cpu.supervisorMode = true
	cpu.skac = true
	cpu.suaLatch = true // inside copy_from_user / copy_to_user region

	if _, fault, cause := cpu.translateAddr(0x1000, ACCESS_READ); fault {
		t.Fatalf("SKAC must allow supervisor read when SUA is set (cause=%d)", cause)
	}
	if _, fault, cause := cpu.translateAddr(0x1000, ACCESS_WRITE); fault {
		t.Fatalf("SKAC must allow supervisor write when SUA is set (cause=%d)", cause)
	}
}

func TestMMU_SKAC_DoesNotBlockExec(t *testing.T) {
	// SKAC is data-only; instruction fetch on a user page should
	// either succeed (SKEF off) or fault with FAULT_SKEF (SKEF on),
	// never with FAULT_SKAC.
	rig := setupG2MMU(t, PTE_P|PTE_R|PTE_X|PTE_U)
	cpu := rig.cpu
	cpu.supervisorMode = true
	cpu.skac = true
	cpu.skef = false
	cpu.suaLatch = false

	_, fault, cause := cpu.translateAddr(0x1000, ACCESS_EXEC)
	if fault {
		t.Fatalf("SKAC alone must not block execute (cause=%d)", cause)
	}
}

func TestMMU_SKAC_DoesNotAffectUserMode(t *testing.T) {
	rig := setupG2MMU(t, PTE_P|PTE_R|PTE_W|PTE_U)
	cpu := rig.cpu
	cpu.supervisorMode = false
	cpu.skac = true
	cpu.suaLatch = false // irrelevant in user mode

	if _, fault, cause := cpu.translateAddr(0x1000, ACCESS_READ); fault {
		t.Fatalf("SKAC must not affect user-mode read (cause=%d)", cause)
	}
	if _, fault, cause := cpu.translateAddr(0x1000, ACCESS_WRITE); fault {
		t.Fatalf("SKAC must not affect user-mode write (cause=%d)", cause)
	}
}

func TestIE64_SUAEN_SetsSUALatch(t *testing.T) {
	rig := newIE64TestRig() // boots in supervisor mode
	rig.executeOne(ie64Instr(OP_SUAEN, 0, 0, 0, 0, 0, 0))
	if !rig.cpu.suaLatch {
		t.Fatal("SUAEN must set the SUA latch")
	}
}

func TestIE64_SUADIS_ClearsSUALatch(t *testing.T) {
	rig := newIE64TestRig()
	rig.cpu.suaLatch = true
	rig.executeOne(ie64Instr(OP_SUADIS, 0, 0, 0, 0, 0, 0))
	if rig.cpu.suaLatch {
		t.Fatal("SUADIS must clear the SUA latch")
	}
}

func TestIE64_SUAEN_PrivilegeFault(t *testing.T) {
	// In user mode, SUAEN must trap FAULT_PRIV before its latch side
	// effect runs. The latch after-state is dominated by trapEntry
	// (which clears SUA as part of the standard kernel-entry
	// discipline), so we verify the trap itself fired rather than the
	// latch value.
	rig := newIE64TestRig()
	rig.cpu.trapVector = 0x2000
	rig.cpu.supervisorMode = false

	rig.loadInstructions(ie64Instr(OP_SUAEN, 0, 0, 0, 0, 0, 0))
	rig.cpu.StepOne()

	if rig.cpu.faultCause != FAULT_PRIV {
		t.Fatalf("SUAEN in user mode must trap FAULT_PRIV, got %d", rig.cpu.faultCause)
	}
	if rig.cpu.PC != rig.cpu.trapVector {
		t.Fatalf("SUAEN in user mode must jump to trapVector; PC=0x%X", rig.cpu.PC)
	}
	if !rig.cpu.supervisorMode {
		t.Fatal("SUAEN trap should have switched to supervisor mode")
	}
}

func TestIE64_SUADIS_PrivilegeFault(t *testing.T) {
	rig := newIE64TestRig()
	rig.cpu.trapVector = 0x2000
	rig.cpu.supervisorMode = false

	rig.loadInstructions(ie64Instr(OP_SUADIS, 0, 0, 0, 0, 0, 0))
	rig.cpu.StepOne()

	if rig.cpu.faultCause != FAULT_PRIV {
		t.Fatalf("SUADIS in user mode must trap FAULT_PRIV, got %d", rig.cpu.faultCause)
	}
	if rig.cpu.PC != rig.cpu.trapVector {
		t.Fatalf("SUADIS in user mode must jump to trapVector; PC=0x%X", rig.cpu.PC)
	}
	if !rig.cpu.supervisorMode {
		t.Fatal("SUADIS trap should have switched to supervisor mode")
	}
}

func TestIE64_MMU_CTRL_SKEFWritable(t *testing.T) {
	rig := newIE64TestRig()
	// Put MMU_CTRL_SKEF into R1 and MTCR it into CR_MMU_CTRL.
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(MMU_CTRL_SKEF)),
		ie64Instr(OP_MTCR, CR_MMU_CTRL, 0, 0, 1, 0, 0),
	)
	rig.cpu.StepOne()
	rig.cpu.StepOne()
	if !rig.cpu.skef {
		t.Fatal("MTCR CR_MMU_CTRL with SKEF bit should enable skef")
	}
}

func TestIE64_MMU_CTRL_SKACWritable(t *testing.T) {
	rig := newIE64TestRig()
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(MMU_CTRL_SKAC)),
		ie64Instr(OP_MTCR, CR_MMU_CTRL, 0, 0, 1, 0, 0),
	)
	rig.cpu.StepOne()
	rig.cpu.StepOne()
	if !rig.cpu.skac {
		t.Fatal("MTCR CR_MMU_CTRL with SKAC bit should enable skac")
	}
}

func TestIE64_MMU_CTRL_SUANotWritableViaMTCR(t *testing.T) {
	rig := newIE64TestRig()
	if rig.cpu.suaLatch {
		t.Fatal("suaLatch should be false at boot")
	}
	// Try to set SUA via MTCR; it should be ignored.
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, uint32(MMU_CTRL_SUA)),
		ie64Instr(OP_MTCR, CR_MMU_CTRL, 0, 0, 1, 0, 0),
	)
	rig.cpu.StepOne()
	rig.cpu.StepOne()
	if rig.cpu.suaLatch {
		t.Fatal("MTCR CR_MMU_CTRL must NOT set SUA latch; only SUAEN/SUADIS do")
	}
}

func TestIE64_MMU_CTRL_ReadsAllBits(t *testing.T) {
	// Leave mmuEnabled off so StepOne can fetch without a page table;
	// the SKEF/SKAC/SUA bits and the SUPER bit (always set at boot in
	// supervisor mode) are enough to prove MFCR exposes every new
	// field. ENABLE readback is already covered by
	// TestIE64_MMU_CTRL_Bit1Readable and TestIE64_MMU_CTRL_Bit1ReadOnly.
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.skef = true
	cpu.skac = true
	cpu.suaLatch = true

	rig.loadInstructions(ie64Instr(OP_MFCR, 1, 0, 0, CR_MMU_CTRL, 0, 0))
	cpu.StepOne()

	want := uint64(MMU_CTRL_SUPER | MMU_CTRL_SKEF | MMU_CTRL_SKAC | MMU_CTRL_SUA)
	if cpu.regs[1] != want {
		t.Fatalf("MFCR CR_MMU_CTRL = 0x%X, want 0x%X", cpu.regs[1], want)
	}
}

func TestIE64_TrapEntry_SavesAndClearsSUA(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.suaLatch = true

	cpu.trapEntry()

	if cpu.suaLatch {
		t.Fatal("trapEntry must clear SUA latch")
	}
	if !cpu.savedSUA {
		t.Fatal("trapEntry must save prior SUA latch into savedSUA")
	}
}

func TestIE64_ERET_UserReturnClearsSUA(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu

	// Simulate: we trapped from user mode, now kernel handler sets
	// SUA=true mid-handling (bug scenario), then ERETs back to user.
	// Build prior state: previousMode=false (came from user),
	// suaLatch=true (simulated kernel error), savedSUA=false.
	cpu.supervisorMode = true
	cpu.previousMode = false
	cpu.suaLatch = true
	cpu.savedSUA = false
	cpu.kernelSP = cpu.regs[31]
	cpu.userSP = 0x10000
	cpu.faultPC = PROG_START + 16

	rig.executeOne(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	if cpu.suaLatch {
		t.Fatal("ERET returning to user mode must clear SUA latch")
	}
	if cpu.savedSUA {
		t.Fatal("ERET must also clear savedSUA after consuming it")
	}
	if cpu.supervisorMode {
		t.Fatal("ERET should have switched to user mode")
	}
}

func TestIE64_ERET_SupervisorReturnRestoresSUA(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu

	// Simulate: supervisor was executing a copy helper with SUA=1, a
	// nested trap fired, trapEntry saved SUA=1 and cleared it. Kernel
	// handler runs, ERETs back. The interrupted copy helper must
	// resume with SUA=1 restored.
	cpu.supervisorMode = true
	cpu.previousMode = true // came from supervisor (nested)
	cpu.suaLatch = false    // nested handler ran with SUA clear
	cpu.savedSUA = true     // earlier trapEntry saved SUA=1
	cpu.faultPC = PROG_START + 16

	rig.executeOne(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))

	if !cpu.suaLatch {
		t.Fatal("ERET to supervisor must restore SUA latch from savedSUA")
	}
	if cpu.savedSUA {
		t.Fatal("ERET must clear savedSUA after restoring SUA")
	}
	if !cpu.supervisorMode {
		t.Fatal("ERET should have stayed in supervisor mode")
	}
}

// TestIE64_UserCopyPattern_SUAENGatedLoadStore exercises the exact
// instruction sequence the kernel usercopy helpers emit (SUAEN, load
// byte from user page, store byte to kernel page, SUADIS). With SKAC
// enabled this validates that the helper pattern succeeds end-to-end.
func TestIE64_UserCopyPattern_SUAENGatedLoadStore(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.skac = true
	cpu.ptbr = 0x20000

	// Map user page (VPN 2 -> PPN 5) RW+U.
	writePTE(cpu, 2, makePTE(5, PTE_P|PTE_R|PTE_W|PTE_U))
	// Map kernel page (VPN 3 -> PPN 6) RW supervisor-only.
	writePTE(cpu, 3, makePTE(6, PTE_P|PTE_R|PTE_W))
	// Map VPN 1 (PROG_START / 4K) RX+U so instruction fetch works
	// while SKEF is off (SKEF not enabled for this test).
	writePTE(cpu, uint64(PROG_START>>MMU_PAGE_SHIFT), makePTE(1, PTE_P|PTE_R|PTE_X|PTE_U))

	// Seed user page byte.
	cpu.memory[(5<<MMU_PAGE_SHIFT)+0x10] = 0xA5

	const userAddr = uint32(2<<MMU_PAGE_SHIFT) | 0x10
	const kernAddr = uint32(3<<MMU_PAGE_SHIFT) | 0x20

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userAddr),
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, kernAddr),
		ie64Instr(OP_SUAEN, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_B, 0, 1, 0, 0),
		ie64Instr(OP_STORE, 3, IE64_SIZE_B, 0, 2, 0, 0),
		ie64Instr(OP_SUADIS, 0, 0, 0, 0, 0, 0),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.Execute()

	if cpu.memory[(6<<MMU_PAGE_SHIFT)+0x20] != 0xA5 {
		t.Fatalf("kernel store = 0x%02X, want 0xA5 (SUAEN-gated copy should succeed)",
			cpu.memory[(6<<MMU_PAGE_SHIFT)+0x20])
	}
	if cpu.suaLatch {
		t.Fatal("SUADIS should leave the latch cleared after the copy region")
	}
}

// TestIE64_UserCopyPattern_WithoutSUAENFaultsSKAC verifies that the
// same sequence WITHOUT the SUAEN bracket faults cleanly on the first
// user-page load when SKAC is enabled. This is the invariant that
// justifies migrating every kernel user-access site onto the helpers.
func TestIE64_UserCopyPattern_WithoutSUAENFaultsSKAC(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.skac = true
	cpu.ptbr = 0x20000
	cpu.trapVector = PROG_START + 0x200

	writePTE(cpu, 2, makePTE(5, PTE_P|PTE_R|PTE_W|PTE_U))
	writePTE(cpu, uint64(PROG_START>>MMU_PAGE_SHIFT), makePTE(1, PTE_P|PTE_R|PTE_X|PTE_U))

	const userAddr = uint32(2<<MMU_PAGE_SHIFT) | 0x10

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, userAddr),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_B, 0, 1, 0, 0), // should fault FAULT_SKAC
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	// Trap handler at PROG_START+0x200 just halts to end Execute.
	haltInstr := ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0)
	copy(cpu.memory[PROG_START+0x200:], haltInstr)
	// Also map the trap-handler page RX supervisor-only (same VPN 1).

	cpu.running.Store(true)
	cpu.Execute()

	if cpu.faultCause != FAULT_SKAC {
		t.Fatalf("unguarded kernel read from user page: faultCause = %d, want FAULT_SKAC (%d)",
			cpu.faultCause, FAULT_SKAC)
	}
}

func TestIE64_SUALatch_SurvivesNestedTrap(t *testing.T) {
	// End-to-end: SUAEN, simulate a trap+ERET cycle, verify SUA is
	// still set for the resumed code. Validates save/restore as a
	// whole mechanism (not just the two ends separately).
	rig := newIE64TestRig()
	cpu := rig.cpu

	// Kernel context, SUA set as if mid copy_from_user.
	cpu.supervisorMode = true
	cpu.suaLatch = true

	// Nested trap fires. trapEntry saves + clears SUA.
	cpu.trapEntry()
	if cpu.suaLatch {
		t.Fatal("nested trap must clear SUA")
	}
	if !cpu.savedSUA {
		t.Fatal("nested trap must stash SUA in savedSUA")
	}

	// Nested handler runs and then ERETs. Must land back in supervisor
	// with SUA restored so copy helper resumes safely.
	cpu.faultPC = PROG_START + 16
	rig.executeOne(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	if !cpu.suaLatch {
		t.Fatal("SUA latch must be restored after nested-trap ERET")
	}
	if cpu.savedSUA {
		t.Fatal("savedSUA must be consumed by ERET")
	}
}

// TestIE64_CR_SAVED_SUA_IsReadableByKernel verifies MFCR CR_SAVED_SUA
// returns 0/1 reflecting the latched-on-entry value. The kernel's
// nested-trap prologue reads this to stash on the kernel stack.
func TestIE64_CR_SAVED_SUA_IsReadableByKernel(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.savedSUA = true

	rig.loadInstructions(ie64Instr(OP_MFCR, 1, 0, 0, CR_SAVED_SUA, 0, 0))
	cpu.StepOne()
	if cpu.regs[1] != 1 {
		t.Fatalf("MFCR CR_SAVED_SUA = %d, want 1", cpu.regs[1])
	}

	cpu.savedSUA = false
	cpu.regs[1] = 0xDEADBEEF // scribble so we see the write
	rig.loadInstructions(ie64Instr(OP_MFCR, 1, 0, 0, CR_SAVED_SUA, 0, 0))
	cpu.PC = PROG_START
	cpu.StepOne()
	if cpu.regs[1] != 0 {
		t.Fatalf("MFCR CR_SAVED_SUA = 0x%X, want 0", cpu.regs[1])
	}
}

// TestIE64_CR_SAVED_SUA_IsWritableByKernel verifies MTCR CR_SAVED_SUA
// stores a value the kernel can later consume via ERET. This is the
// restore half of the nested-trap prologue.
func TestIE64_CR_SAVED_SUA_IsWritableByKernel(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.savedSUA = false

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 1),
		ie64Instr(OP_MTCR, CR_SAVED_SUA, 0, 0, 1, 0, 0),
	)
	cpu.StepOne()
	cpu.StepOne()
	if !cpu.savedSUA {
		t.Fatal("MTCR CR_SAVED_SUA with 1 should set savedSUA")
	}

	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0),
		ie64Instr(OP_MTCR, CR_SAVED_SUA, 0, 0, 1, 0, 0),
	)
	cpu.PC = PROG_START
	cpu.StepOne()
	cpu.StepOne()
	if cpu.savedSUA {
		t.Fatal("MTCR CR_SAVED_SUA with 0 should clear savedSUA")
	}
}

// TestIE64_CR_SAVED_SUA_UserModeFaults verifies MTCR/MFCR on
// CR_SAVED_SUA trap in user mode (supervisor-only CR).
func TestIE64_CR_SAVED_SUA_UserModeFaults(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.trapVector = 0x2000
	cpu.supervisorMode = false

	rig.loadInstructions(ie64Instr(OP_MFCR, 1, 0, 0, CR_SAVED_SUA, 0, 0))
	cpu.StepOne()
	if cpu.faultCause != FAULT_PRIV {
		t.Fatalf("user-mode MFCR CR_SAVED_SUA must FAULT_PRIV; got %d", cpu.faultCause)
	}
}

// TestIE64_SUALatch_SurvivesTwoLevelNestedTrap is the multi-level
// scenario the first M15.6 G2 review flagged. Outer copy_from_user
// (SUA=1) is interrupted by trap A; trap A's handler is itself
// interrupted by trap B before it completes; when both handlers
// unwind, the original SUA=1 must be restored for the resumed copy.
//
// This test was originally written against the single-slot savedSUA
// model and required the kernel to save/restore CR_SAVED_SUA manually
// between nested levels. With the M15.6 Phase 2c-trap trap stack the
// preservation is architectural: trapEntry pushes the outer frame and
// ERET pops, so the test exercises two trapEntry calls followed by two
// ERETs with no CR_SAVED_SUA cooperation and the outer copy helper's
// SUA=1 comes back automatically.
func TestIE64_SUALatch_SurvivesTwoLevelNestedTrap(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.supervisorMode = true

	// State 0: executing copy_from_user with SUA=1.
	cpu.suaLatch = true

	// Trap A fires. trapEntry pushes the outer (empty) frame and moves
	// suaLatch into the active savedSUA.
	cpu.trapEntry()
	if !cpu.savedSUA {
		t.Fatal("trap A trapEntry should stash SUA=1 into savedSUA")
	}
	if cpu.suaLatch {
		t.Fatal("trap A trapEntry should clear the active SUA latch")
	}

	// Handler A runs and takes trap B. The active frame is pushed as-is
	// (savedSUA=1 goes onto the stack), so B's savedSUA starts fresh
	// from handler-A's suaLatch (0).
	cpu.trapEntry()
	if cpu.savedSUA {
		t.Fatal("trap B trapEntry should stash handler-A SUA=0, not clobber the outer")
	}

	// Handler B ERETs with no CR_SAVED_SUA dance; the pop restores A's
	// frame (savedSUA=1, ready to be consumed by A's own ERET).
	cpu.faultPC = PROG_START + 16
	rig.executeOne(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	if cpu.suaLatch {
		t.Fatal("ERET from trap B should leave SUA=0 (handler A body)")
	}
	if !cpu.savedSUA {
		t.Fatal("ERET from trap B should pop A's savedSUA=1 back into the active frame")
	}

	// Handler A ERETs; consuming the popped frame restores the outer
	// copy-helper's SUA=1 without any kernel cooperation.
	cpu.faultPC = PROG_START + 32
	rig.executeOne(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	if !cpu.suaLatch {
		t.Fatal("ERET from trap A should restore outer SUA=1 (the copy_from_user window)")
	}
}

// TestIE64_Disassembler_DecodesSUAENAndSUADIS guards against silent
// decoder regressions: adding the SUAEN/SUADIS opcodes without updating
// the in-VM monitor's disassembler (debug_disasm_ie64.go) would render
// any binary using them as "dc.b $F3 / dc.b $F4", breaking monitor
// listings of the shipped iexec kernel.
func TestIE64_Disassembler_DecodesSUAENAndSUADIS(t *testing.T) {
	suaen := ie64Instr(OP_SUAEN, 0, 0, 0, 0, 0, 0)
	suadis := ie64Instr(OP_SUADIS, 0, 0, 0, 0, 0, 0)

	dSUAEN := ie64Decode(suaen, 0)
	_, mnemSUAEN := ie64FormatInstruction(dSUAEN)
	if mnemSUAEN != "suaen" {
		t.Errorf("monitor disasm of SUAEN = %q, want \"suaen\"", mnemSUAEN)
	}

	dSUADIS := ie64Decode(suadis, 0)
	_, mnemSUADIS := ie64FormatInstruction(dSUADIS)
	if mnemSUADIS != "suadis" {
		t.Errorf("monitor disasm of SUADIS = %q, want \"suadis\"", mnemSUADIS)
	}
}

// TestIE64_Reset_ClearsG2State verifies Reset() wipes the M15.6 G2
// SMEP/SMAP-equivalent state (skef, skac, suaLatch, savedSUA). Without
// this, a reused CPU instance inherits a stale privilege configuration
// from the previous run and spuriously faults or runs with an open SUA
// window.
func TestIE64_Reset_ClearsG2State(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu

	cpu.skef = true
	cpu.skac = true
	cpu.suaLatch = true
	cpu.savedSUA = true

	cpu.Reset()

	if cpu.skef {
		t.Error("Reset() must clear skef")
	}
	if cpu.skac {
		t.Error("Reset() must clear skac")
	}
	if cpu.suaLatch {
		t.Error("Reset() must clear suaLatch (no open SUA window after reset)")
	}
	if cpu.savedSUA {
		t.Error("Reset() must clear savedSUA")
	}
}

// TestIE64_JIT_SUAEN_NotSilentlyCompiledAsNop guards against the
// original review finding: if SUAEN/SUADIS are reachable inside a JIT
// block without a dedicated handler, the JIT backends' default case
// emits a NOP and the latch is never toggled. Both opcodes must now
// be isBlockTerminator + needsInterpreter + emitBailToInterpreter.
func TestIE64_JIT_SUAEN_NotSilentlyCompiledAsNop(t *testing.T) {
	if !isBlockTerminator(OP_SUAEN) {
		t.Error("SUAEN must be a block terminator so the JIT dispatches it through the interpreter")
	}
	if !isBlockTerminator(OP_SUADIS) {
		t.Error("SUADIS must be a block terminator so the JIT dispatches it through the interpreter")
	}

	// Single-opcode blocks must force interpreter dispatch via
	// needsFallback.
	blockSUAEN := []JITInstr{{opcode: OP_SUAEN}}
	if !needsFallback(blockSUAEN) {
		t.Error("single-opcode SUAEN block must flag needsFallback")
	}
	blockSUADIS := []JITInstr{{opcode: OP_SUADIS}}
	if !needsFallback(blockSUADIS) {
		t.Error("single-opcode SUADIS block must flag needsFallback")
	}
}

// TestIE64_TrapStack_AutomaticNestedSUARestore is the proof that nested
// SUA preservation is architectural, not kernel-managed. No MFCR/MTCR on
// CR_SAVED_SUA and no kernel-stack save: trapEntry pushes a frame and
// ERET pops, so the outer copy-helper window is restored without any
// cooperation from the kernel handler.
func TestIE64_TrapStack_AutomaticNestedSUARestore(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.supervisorMode = true

	// Outer copy_from_user has opened the supervisor-access window.
	cpu.suaLatch = true

	// Trap A fires from inside the copy helper.
	cpu.trapEntry()
	if cpu.suaLatch {
		t.Fatal("trap A entry must clear the active SUA latch")
	}

	// Handler A, mid-work, takes a nested trap B. No manual save of
	// CR_SAVED_SUA; the architecture must preserve A's frame.
	cpu.trapEntry()
	if cpu.suaLatch {
		t.Fatal("trap B entry must clear the active SUA latch")
	}

	// Handler B runs and ERETs. The pop must restore the frame that
	// belonged to A, including A's savedSUA from the outer copy helper.
	cpu.faultPC = PROG_START + 16
	rig.executeOne(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	if !cpu.savedSUA {
		t.Fatal("inner ERET pop must restore A's savedSUA (outer SUA=1) with no kernel cooperation")
	}
	if cpu.suaLatch {
		t.Fatal("after B returns, A's handler body has SUA clear until its own ERET")
	}

	// Handler A ERETs. The outer copy-helper window (SUA=1) must
	// come back purely from the popped frame.
	cpu.faultPC = PROG_START + 32
	rig.executeOne(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	if !cpu.suaLatch {
		t.Fatal("outer SUA=1 must be restored by trap-stack pop, not kernel save/restore")
	}
}

// TestIE64_TrapStack_PreservesOuterFaultPC pins that the nested-trap
// push captures the outer trap's full frame, not just the SUA latch.
// CR_FAULT_PC / CR_FAULT_ADDR / CR_FAULT_CAUSE must all come back on
// inner ERET without any MTCR restore by the kernel.
func TestIE64_TrapStack_PreservesOuterFaultPC(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.supervisorMode = true

	// Outer trap A: handler has set up its idea of fault state.
	cpu.trapEntry()
	cpu.faultPC = 0xAAA0
	cpu.faultAddr = 0xAAA4
	cpu.faultCause = FAULT_READ_DENIED

	// Nested trap B overwrites the active frame.
	cpu.trapEntry()
	cpu.faultPC = 0xBBB0
	cpu.faultAddr = 0xBBB4
	cpu.faultCause = FAULT_WRITE_DENIED

	// Inner ERET: pop must restore A's entire frame.
	rig.executeOne(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	if cpu.faultPC != 0xAAA0 {
		t.Fatalf("outer faultPC lost after nested ERET: got 0x%X, want 0xAAA0", cpu.faultPC)
	}
	if cpu.faultAddr != 0xAAA4 {
		t.Fatalf("outer faultAddr lost after nested ERET: got 0x%X, want 0xAAA4", cpu.faultAddr)
	}
	if cpu.faultCause != FAULT_READ_DENIED {
		t.Fatalf("outer faultCause lost after nested ERET: got %d, want %d", cpu.faultCause, FAULT_READ_DENIED)
	}
}

// TestIE64_TrapStack_PreservesOuterPrevMode pins that previousMode
// (CR_PREV_MODE) is part of the pushed frame. Without this, an outer
// trap from user mode followed by a nested supervisor-mode trap would
// lose the "came from user" bit and the outer ERET would take the
// wrong branch.
func TestIE64_TrapStack_PreservesOuterPrevMode(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu

	// Outer trap entered from user mode.
	cpu.supervisorMode = false
	cpu.kernelSP = 0x20000
	cpu.regs[31] = 0x10000
	cpu.trapEntry() // pushes fresh frame; sets previousMode=false
	if cpu.previousMode {
		t.Fatal("outer trapEntry from user should set previousMode=false")
	}

	// Nested trap from supervisor clobbers previousMode in the active frame.
	cpu.trapEntry()
	if !cpu.previousMode {
		t.Fatal("inner trapEntry from supervisor should set previousMode=true")
	}

	// Inner ERET: pop restores the outer frame (previousMode=false).
	cpu.faultPC = PROG_START + 16
	rig.executeOne(ie64Instr(OP_ERET, 0, 0, 0, 0, 0, 0))
	if cpu.previousMode {
		t.Fatal("outer previousMode=false must be restored by pop, not overwritten by nested trap")
	}
}

// TestIE64_TrapStack_OverflowHaltsCleanly verifies that trap-stack
// overflow is a defined failure mode, not silent data loss. A runaway
// nested trap is a kernel bug; halting the CPU is the safe response.
func TestIE64_TrapStack_OverflowHaltsCleanly(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.supervisorMode = true
	cpu.running.Store(true)

	for i := 0; i < TrapStackDepth; i++ {
		if !cpu.trapEntry() {
			t.Fatalf("trapEntry unexpectedly failed at depth %d", i)
		}
	}

	if ok := cpu.trapEntry(); ok {
		t.Fatal("trap stack overflow must make trapEntry return false")
	}
	if cpu.running.Load() {
		t.Fatalf("trap stack overflow (depth=%d) must halt the CPU", TrapStackDepth+1)
	}
	if !cpu.trapHalted {
		t.Fatal("trap stack overflow must set trapHalted so the execution loop can bail")
	}
}

// TestIE64_TrapStack_OverflowDoesNotRedirectPC proves that the main
// loop's "halt cleanly" contract is actually honoured: after overflow,
// the CPU must not continue to redirect PC to the trap vector and
// execute the handler — the trap is aborted mid-flight. Without the
// trapHalted check in Execute, the loop would run thousands of extra
// guest instructions before noticing the running flag dropped.
func TestIE64_TrapStack_OverflowDoesNotRedirectPC(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.supervisorMode = true
	cpu.trapVector = 0xDEADBE00

	// Fill the trap stack.
	for i := 0; i < TrapStackDepth; i++ {
		cpu.trapEntry()
	}

	// Drive an overflow through trapFault. Under the old behaviour,
	// trapFault would still redirect PC to trapVector and overwrite
	// faultPC/faultAddr/faultCause — leaving the CPU "halted" but
	// primed to vector into the trap handler on the next instruction
	// fetch. The fix makes trapFault a no-op on overflow so the loop
	// stops without touching PC.
	cpu.PC = 0x1234
	cpu.faultPC = 0xAAAA
	cpu.faultAddr = 0xBBBB
	cpu.faultCause = 0xCC
	cpu.trapFault(FAULT_READ_DENIED, 0x5678)

	if cpu.PC != 0x1234 {
		t.Fatalf("trapFault on overflow must not redirect PC; got 0x%X, want 0x1234", cpu.PC)
	}
	if cpu.faultPC != 0xAAAA || cpu.faultAddr != 0xBBBB || cpu.faultCause != 0xCC {
		t.Fatalf("trapFault on overflow must not mutate active fault fields; got pc=0x%X addr=0x%X cause=0x%X",
			cpu.faultPC, cpu.faultAddr, cpu.faultCause)
	}
	if !cpu.trapHalted {
		t.Fatal("trapFault on overflow must set trapHalted")
	}
}

// TestIE64_TrapStack_OverflowBreaksExecuteImmediately pins that the
// Execute hot loop actually observes trapHalted on the iteration it
// was set. Prior to the fix, cpu.running.Load() was polled only every
// 4096 instructions, so overflow would let the interpreter run
// thousands of extra guest instructions before noticing it was halted.
func TestIE64_TrapStack_OverflowBreaksExecuteImmediately(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.supervisorMode = true

	// Fill the stack so the next trap entry overflows.
	for i := 0; i < TrapStackDepth; i++ {
		cpu.trapEntry()
	}

	// Load a benign program: a big run of NOPs then HALT. If the loop
	// failed to honour trapHalted, it would execute many of these
	// NOPs before the next cpu.running poll.
	instrs := make([][]byte, 0, 200)
	for i := 0; i < 100; i++ {
		instrs = append(instrs, ie64Instr(OP_NOP64, 0, 0, 0, 0, 0, 0))
	}
	instrs = append(instrs, ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	rig.loadInstructions(instrs...)

	// Seed an overflow trap before Execute is called; the very first
	// iteration of Execute must see trapHalted and bail.
	cpu.trapFault(FAULT_READ_DENIED, 0)

	startInstrCount := cpu.InstructionCount
	cpu.PerfEnabled = true
	cpu.running.Store(true) // simulate a bus re-arm: does not clear trapHalted
	cpu.Execute()
	cpu.PerfEnabled = false

	if cpu.InstructionCount-startInstrCount > 1 {
		t.Fatalf("Execute must bail immediately on trapHalted; ran %d instructions",
			cpu.InstructionCount-startInstrCount)
	}
}

// TestIE64_TrapStack_ResetClears wipes the trap stack on Reset() so a
// reused CPU does not inherit a half-built frame from a previous run.
func TestIE64_TrapStack_ResetClears(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu

	cpu.supervisorMode = true
	cpu.trapEntry()
	cpu.trapEntry()
	if cpu.trapDepth == 0 {
		t.Fatal("precondition: two trapEntry calls should leave trapDepth > 0")
	}

	cpu.Reset()

	if cpu.trapDepth != 0 {
		t.Fatalf("Reset() must clear trapDepth; got %d", cpu.trapDepth)
	}
}
