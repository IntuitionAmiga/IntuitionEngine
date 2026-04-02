package main

import (
	"encoding/binary"
	"testing"
)

// ===========================================================================
// MMU Phase 2: Page Table Walk
// ===========================================================================

// Helper: write a PTE into the page table at the given VPN
func writePTE(cpu *CPU64, vpn uint16, pte uint64) {
	off := uint32(cpu.ptbr) + uint32(vpn)*8
	cpu.memory[off+0] = byte(pte)
	cpu.memory[off+1] = byte(pte >> 8)
	cpu.memory[off+2] = byte(pte >> 16)
	cpu.memory[off+3] = byte(pte >> 24)
	cpu.memory[off+4] = byte(pte >> 32)
	cpu.memory[off+5] = byte(pte >> 40)
	cpu.memory[off+6] = byte(pte >> 48)
	cpu.memory[off+7] = byte(pte >> 56)
}

func TestMMU_PTEEncodeDecode(t *testing.T) {
	// Test round-trip of PTE encode/decode
	testCases := []struct {
		ppn   uint16
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
	vaddr := uint32(0x1ABC)
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
		vpn    uint16
		ppn    uint16
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
		vaddr := (uint32(tt.vpn) << MMU_PAGE_SHIFT) | tt.offset
		expectedPhys := (uint32(tt.ppn) << MMU_PAGE_SHIFT) | tt.offset

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

// setupIdentityMMU sets up an identity-mapped page table covering the first N pages.
// Page table is placed at physical 0x80000. Code/stack pages get R+W+X+U.
func setupIdentityMMU(cpu *CPU64, numPages int) {
	ptBase := uint32(0x80000) // well above PROG_START, below IO_REGION_START
	cpu.ptbr = ptBase
	cpu.mmuEnabled = true

	for i := 0; i < numPages; i++ {
		pte := makePTE(uint16(i), PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)
		off := ptBase + uint32(i)*8
		binary.LittleEndian.PutUint64(cpu.memory[off:], pte)
	}

	// Also map the page table itself (it lives at 0x80000, page 128)
	ptPage := ptBase >> MMU_PAGE_SHIFT
	ptEndPage := (ptBase + uint32(numPages)*8 + MMU_PAGE_SIZE - 1) >> MMU_PAGE_SHIFT
	for p := ptPage; p <= ptEndPage; p++ {
		pte := makePTE(uint16(p), PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)
		off := ptBase + p*8
		binary.LittleEndian.PutUint64(cpu.memory[off:], pte)
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
	progPage := uint16(PROG_START >> MMU_PAGE_SHIFT)
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
	stackPage := uint16(0x9E000 >> MMU_PAGE_SHIFT)                        // near STACK_START
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

	// Place page table at 0x80000
	ptBase := uint32(0x80000)

	// Identity-map the first 160 pages (covers code, stack, page table)
	for i := 0; i < 160; i++ {
		pte := makePTE(uint16(i), PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)
		binary.LittleEndian.PutUint64(cpu.memory[ptBase+uint32(i)*8:], pte)
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

	// Unmap page 4
	writePTE(cpu, 4, 0)

	// Write a known value at physical page 4 address
	binary.LittleEndian.PutUint32(cpu.memory[0x4100:], 0xBEEFCAFE)

	trapAddr := uint64(0x9000)
	cpu.trapVector = trapAddr

	// Trap handler: map page 4 with identity mapping, then ERET
	// Handler needs to:
	// 1. Read FAULT_ADDR (MFCR)
	// 2. Calculate VPN
	// 3. Create PTE and write to page table
	// 4. TLBINVAL
	// 5. ERET
	// This is complex in IE64 assembly, so we'll cheat: pre-program the handler
	// to just map page 4 directly.
	handlerOff := uint32(trapAddr)

	// R20 = page table entry value for page 4 (identity mapped, PRWXU)
	pte4 := makePTE(4, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)
	// R21 = address in page table for VPN 4 = ptbr + 4*8 = 0x80000 + 32 = 0x80020
	ptEntryAddr := cpu.ptbr + 4*8

	copy(cpu.memory[handlerOff:], ie64Instr(OP_MOVE, 20, IE64_SIZE_L, 1, 0, 0, uint32(pte4)))
	handlerOff += 8
	copy(cpu.memory[handlerOff:], ie64Instr(OP_MOVT, 20, 0, 1, 0, 0, uint32(pte4>>32)))
	handlerOff += 8
	copy(cpu.memory[handlerOff:], ie64Instr(OP_MOVE, 21, IE64_SIZE_L, 1, 0, 0, ptEntryAddr))
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

	// Process 1 page table at 0x80000
	pt1 := uint32(0x80000)
	// Process 2 page table at 0x84000
	pt2 := uint32(0x84000)

	// Identity map pages for code and stack in both page tables
	for i := 0; i < 160; i++ {
		pte := makePTE(uint16(i), PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)
		binary.LittleEndian.PutUint64(cpu.memory[pt1+uint32(i)*8:], pte)
		binary.LittleEndian.PutUint64(cpu.memory[pt2+uint32(i)*8:], pte)
	}

	// But map virtual page 10 differently:
	// Process 1: virtual page 10 -> physical page 20
	// Process 2: virtual page 10 -> physical page 30
	pte1 := makePTE(20, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)
	pte2 := makePTE(30, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U)
	binary.LittleEndian.PutUint64(cpu.memory[pt1+10*8:], pte1)
	binary.LittleEndian.PutUint64(cpu.memory[pt2+10*8:], pte2)

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
	stackPage := uint16((STACK_START - 8) >> MMU_PAGE_SHIFT)
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
	stackPage := uint16((STACK_START - 8) >> MMU_PAGE_SHIFT)
	writePTE(cpu, stackPage, makePTE(stackPage, PTE_P|PTE_R|PTE_W|PTE_X)) // no U

	// Switch to user mode, then try JSR (which pushes to stack)
	cpu.faultPC = PROG_START + IE64_INSTR_SIZE
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

// readPTEFlags reads the PTE flags byte for a given VPN from the page table.
func readPTEFlags(cpu *CPU64, vpn uint16) byte {
	pteAddr := cpu.ptbr + uint32(vpn)*8
	pte := binary.LittleEndian.Uint64(cpu.memory[pteAddr:])
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

	vpn := uint16(3)
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
	progVPN := uint16(PROG_START >> MMU_PAGE_SHIFT)

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

	vpn := uint16(3)

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

	vpn := uint16(3)

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

	vpn := uint16(3)
	// Pre-set A|D in PTE
	writePTE(cpu, vpn, makePTE(vpn, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U|PTE_A|PTE_D))

	// Save original PTE
	pteAddr := cpu.ptbr + uint32(vpn)*8
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

	vpn := uint16(3)

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

	vpn := uint16(3)

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
