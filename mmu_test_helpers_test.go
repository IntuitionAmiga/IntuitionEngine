// mmu_test_helpers_test.go - Multi-level page-table fixture helper for
// IE64 MMU tests. PLAN_MAX_RAM.md slice 4 design: tests build the same
// 6-level sparse radix structure that the production walk reads, with
// intermediate tables auto-allocated from a per-PTBR pool that lives just
// above the top-level table.

package main

import "testing"

// mmuTestPoolBaseOffset is the starting offset (relative to PTBR) for the
// pool that mmuMap allocates intermediate-level tables from. The top-level
// table consumes 1 KiB at PTBR; the pool starts one full 4 KiB page later
// so allocations stay page-aligned and never overlap the top.
const mmuTestPoolBaseOffset uint64 = MMU_PAGE_SIZE

// mmuTestState tracks per-PTBR allocation cursors so multiple tasks (each
// with their own ptbr) share the same map without colliding on cursor
// state. Keys are PTBR physical addresses; values are the next free
// 4 KiB page-aligned physical address for an intermediate/leaf table.
var mmuTestState = make(map[uint64]uint64)

// mmuTestSlotEnd tracks the per-PTBR slot-end address (one past the last
// usable byte) configured via mmuTestSetSlotSize. Mirrors the asm-side
// PT_SLOT_END_OFFSET metadata stored at PTBR+0x408 by kern_pt_zero_and_seed.
// PTBRs without a registered slot end fall back to "no bound" so
// pre-existing tests keep their unbounded behavior.
var mmuTestSlotEnd = make(map[uint64]uint64)

// mmuTestNextTable returns the next free intermediate-table physical
// address for `ptbr` and advances the cursor. Allocation is sequential
// inside each ptbr's region; tests that need a different layout can call
// mmuTestSeedPool(ptbr, base) before any mmuMap calls.
func mmuTestNextTable(ptbr uint64) uint64 {
	cursor, ok := mmuTestState[ptbr]
	if !ok {
		cursor = ptbr + mmuTestPoolBaseOffset
	}
	allocated := cursor
	mmuTestState[ptbr] = cursor + PT_NODE_SIZE_BYTES
	return allocated
}

// mmuTestNextTableChecked allocates like mmuTestNextTable but consults the
// per-PTBR slot end (mmuTestSetSlotSize). Returns (base, true) on success.
// If the bumped cursor would exceed slot end, returns (cursor, false) and
// leaves the allocator state untouched. Mirrors the bounds check that
// production install_leaf performs against PTBR+PT_SLOT_END_OFFSET.
func mmuTestNextTableChecked(ptbr uint64) (uint64, bool) {
	cursor, ok := mmuTestState[ptbr]
	if !ok {
		cursor = ptbr + mmuTestPoolBaseOffset
	}
	bumped := cursor + PT_NODE_SIZE_BYTES
	if end, hasEnd := mmuTestSlotEnd[ptbr]; hasEnd && bumped > end {
		return cursor, false
	}
	mmuTestState[ptbr] = bumped
	return cursor, true
}

// mmuTestSetSlotSize records the per-PTBR slot size in bytes so subsequent
// mmuTestNextTableChecked calls fault when allocation would exceed it.
func mmuTestSetSlotSize(ptbr, slotSize uint64) {
	mmuTestSlotEnd[ptbr] = ptbr + slotSize
}

// mmuTestSeedPool overrides the default pool start for the given PTBR.
// Used by tests that pack many task page tables into a tight region and
// need to control where each task's intermediate tables land.
func mmuTestSeedPool(ptbr, base uint64) {
	mmuTestState[ptbr] = base
}

// mmuTestResetPools forgets all per-PTBR allocator state. Tests that share
// a fixed PTBR across subtests should call this between subtests so a
// stale cursor from an earlier run does not bleed into the next.
func mmuTestResetPools() {
	mmuTestState = make(map[uint64]uint64)
	mmuTestSlotEnd = make(map[uint64]uint64)
}

// mmuMap installs a leaf PTE for `vaddr` mapping to physical page `ppn`
// with `flags`. Intermediate tables are allocated from the per-PTBR pool
// on first use. Re-mapping the same vaddr just overwrites the leaf entry.
func mmuMap(cpu *CPU64, vaddr uint64, ppn uint64, flags byte) {
	vpn := (vaddr >> MMU_PAGE_SHIFT) & PTE_PPN_MASK
	tableAddr := cpu.ptbr
	for level := 0; level < PT_LEVELS-1; level++ {
		idx := ptLevelIndex(vpn, level)
		pteAddr := tableAddr + idx*8
		pte := cpu.bus.ReadPhys64(pteAddr)
		nextPPN, pteFlags := parsePTE(pte)
		if pteFlags&PTE_P == 0 {
			newPage := mmuTestNextTable(cpu.ptbr)
			zeroIntermediateTable(cpu, newPage)
			newPPN := newPage >> MMU_PAGE_SHIFT
			cpu.bus.WritePhys64(pteAddr, makePTE(newPPN, PTE_P))
			tableAddr = newPage
		} else {
			tableAddr = nextPPN << MMU_PAGE_SHIFT
		}
	}
	leafIdx := ptLevelIndex(vpn, PT_LEVELS-1)
	leafAddr := tableAddr + leafIdx*8
	cpu.bus.WritePhys64(leafAddr, makePTE(ppn, flags))
}

// mmuLeafPTE returns the leaf PTE installed for `vaddr`, or 0 if any level
// of the walk is unmapped. Used by tests that previously read PTEs by
// indexing the flat single-level table directly.
func mmuLeafPTE(cpu *CPU64, vaddr uint64) uint64 {
	vpn := (vaddr >> MMU_PAGE_SHIFT) & PTE_PPN_MASK
	tableAddr := cpu.ptbr
	for level := 0; level < PT_LEVELS-1; level++ {
		idx := ptLevelIndex(vpn, level)
		pteAddr := tableAddr + idx*8
		pte := cpu.bus.ReadPhys64(pteAddr)
		nextPPN, pteFlags := parsePTE(pte)
		if pteFlags&PTE_P == 0 {
			return 0
		}
		tableAddr = nextPPN << MMU_PAGE_SHIFT
	}
	leafIdx := ptLevelIndex(vpn, PT_LEVELS-1)
	return cpu.bus.ReadPhys64(tableAddr + leafIdx*8)
}

// mmuLeafAddress returns the physical address of the leaf PTE for `vaddr`
// walking the existing multi-level structure without allocating new
// tables. Returns 0 if any intermediate level is unmapped. Used by tests
// that simulate fault-handler PTE updates and need the exact physical
// leaf location to poke into.
func mmuLeafAddress(cpu *CPU64, vaddr uint64) uint64 {
	vpn := (vaddr >> MMU_PAGE_SHIFT) & PTE_PPN_MASK
	tableAddr := cpu.ptbr
	for level := 0; level < PT_LEVELS-1; level++ {
		idx := ptLevelIndex(vpn, level)
		pteAddr := tableAddr + idx*8
		pte := cpu.bus.ReadPhys64(pteAddr)
		nextPPN, pteFlags := parsePTE(pte)
		if pteFlags&PTE_P == 0 {
			return 0
		}
		tableAddr = nextPPN << MMU_PAGE_SHIFT
	}
	leafIdx := ptLevelIndex(vpn, PT_LEVELS-1)
	return tableAddr + leafIdx*8
}

// mmuClearLeafPTE wipes the leaf entry for `vaddr` without dropping the
// intermediate tables. Used by tests that need to leave a guard hole in
// an otherwise-mapped region.
func mmuClearLeafPTE(cpu *CPU64, vaddr uint64) {
	vpn := (vaddr >> MMU_PAGE_SHIFT) & PTE_PPN_MASK
	tableAddr := cpu.ptbr
	for level := 0; level < PT_LEVELS-1; level++ {
		idx := ptLevelIndex(vpn, level)
		pteAddr := tableAddr + idx*8
		pte := cpu.bus.ReadPhys64(pteAddr)
		nextPPN, pteFlags := parsePTE(pte)
		if pteFlags&PTE_P == 0 {
			return
		}
		tableAddr = nextPPN << MMU_PAGE_SHIFT
	}
	leafIdx := ptLevelIndex(vpn, PT_LEVELS-1)
	cpu.bus.WritePhys64(tableAddr+leafIdx*8, 0)
}

// zeroIntermediateTable wipes a freshly-allocated 4 KiB intermediate-level
// table so previously-stored bytes do not leak into the walk as ghost
// PTEs (a uniformly-zero PTE has P=0 and is treated as unmapped).
func zeroIntermediateTable(cpu *CPU64, base uint64) {
	for i := uint64(0); i < PT_NODE_SIZE_BYTES; i += 8 {
		cpu.bus.WritePhys64(base+i, 0)
	}
}

// mmuTestRequireMap installs the mapping and t.Fatals if `vaddr` would
// be rejected by the walk's overflow guard. Useful for tests that build
// up a fixture and want a clear failure if their VA falls in the high
// reserved range.
func mmuTestRequireMap(t *testing.T, cpu *CPU64, vaddr uint64, ppn uint64, flags byte) {
	t.Helper()
	if ppn > PTE_PPN_MASK {
		t.Fatalf("mmuTestRequireMap: ppn 0x%X exceeds PTE_PPN_MASK 0x%X", ppn, PTE_PPN_MASK)
	}
	mmuMap(cpu, vaddr, ppn, flags)
}
