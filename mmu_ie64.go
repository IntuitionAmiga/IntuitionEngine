// mmu_ie64.go - IE64 Memory Management Unit: page table walk, PTE helpers, TLB

package main

import (
	"unsafe"
)

// ===========================================================================
// PTE Helpers
// ===========================================================================

// makePTE builds a 64-bit page table entry from a physical page number and flags.
//
// PLAN_MAX_RAM.md slice 4a: ppn is uint64 to cover the 51-bit PPN field. The
// caller is responsible for masking out bits beyond PTE_PPN_BITS; passing a
// PPN that exceeds the field is a kernel/loader bug, not silently truncated
// here, so callers see the corrupted PTE during testing rather than at boot.
func makePTE(ppn uint64, flags byte) uint64 {
	return uint64(flags) | (ppn << PTE_PPN_SHIFT)
}

// parsePTE extracts the physical page number and permission flags from a PTE.
func parsePTE(pte uint64) (ppn uint64, flags byte) {
	flags = byte(pte & 0x7F) // bits 6:0 (P|R|W|X|U|A|D)
	ppn = (pte >> PTE_PPN_SHIFT) & PTE_PPN_MASK
	return
}

// ===========================================================================
// Software TLB (64-entry, direct-mapped)
// ===========================================================================

// TLBEntry caches a single page table translation.
//
// PLAN_MAX_RAM.md slice 4a: vpn/ppn are uint64. The TLB is still a 64-entry
// direct-mapped cache indexed by the low VPN bits, but the stored key/value
// are full-width so two VPNs that share the same index but differ above the
// uint16 ceiling do not alias.
//
// PLAN_MAX_RAM.md slice 4 design: leafAddr caches the physical address of
// the leaf PTE in the multi-level walk so A/D writebacks on TLB hits do
// not have to redo the 6-level walk.
type TLBEntry struct {
	vpn      uint64
	ppn      uint64
	leafAddr uint64 // physical address of the leaf PTE for A/D writeback
	flags    byte   // PTE permission bits (P|R|W|X|U|A|D)
	valid    bool
}

// tlbLookup checks the TLB for a cached translation.
func (cpu *CPU64) tlbLookup(vpn uint64) (entry *TLBEntry, hit bool) {
	idx := vpn & 63
	e := &cpu.tlb[idx]
	if e.valid && e.vpn == vpn {
		return e, true
	}
	return nil, false
}

// tlbInsert adds or replaces a TLB entry. leafAddr is the physical address
// of the leaf PTE so subsequent A/D writebacks can update the page table
// without re-walking.
func (cpu *CPU64) tlbInsert(vpn, ppn, leafAddr uint64, flags byte) {
	idx := vpn & 63
	cpu.tlb[idx] = TLBEntry{vpn: vpn, ppn: ppn, leafAddr: leafAddr, flags: flags, valid: true}
}

// tlbFlush invalidates all TLB entries.
func (cpu *CPU64) tlbFlush() {
	for i := range cpu.tlb {
		cpu.tlb[i].valid = false
	}
}

// tlbInvalidate invalidates the TLB entry for a specific VPN.
func (cpu *CPU64) tlbInvalidate(vpn uint64) {
	idx := vpn & 63
	if cpu.tlb[idx].vpn == vpn {
		cpu.tlb[idx].valid = false
	}
}

// ===========================================================================
// Multi-Level Page Table Walk Helpers
// ===========================================================================

// ptLevelIndex extracts the index into the level-`level` table for the given
// VPN. Level 0 is the 7-bit top, levels 1..5 are 9 bits each.
func ptLevelIndex(vpn uint64, level int) uint64 {
	if level == 0 {
		return (vpn >> (PT_NODE_BITS * (PT_LEVELS - 1))) & PT_TOP_INDEX_MASK
	}
	shift := uint(PT_NODE_BITS * (PT_LEVELS - 1 - level))
	return (vpn >> shift) & PT_NODE_INDEX_MASK
}

// addrAddChecked adds an offset to a base physical address, returning false
// if the addition wraps around the uint64 space. Every level of the walk
// uses this so a high PTBR plus a nonzero VPN cannot wrap into low memory
// and silently read an unrelated PTE.
func addrAddChecked(base, offset uint64) (uint64, bool) {
	sum := base + offset
	if sum < base {
		return 0, false
	}
	return sum, true
}

// ppnToPhysChecked converts a PPN field to a base physical address (PPN <<
// MMU_PAGE_SHIFT). Returns false if the shift would overflow uint64. With a
// 52-bit PPN field the shift can leave the top 12 bits empty, so any PPN
// that does not fit in 52 bits indicates a corrupted PTE; reject it
// instead of producing an aliased address.
func ppnToPhysChecked(ppn uint64) (uint64, bool) {
	if ppn > PTE_PPN_MASK {
		return 0, false
	}
	return ppn << MMU_PAGE_SHIFT, true
}

// walkPageTable walks the multi-level page table for `vpn` and returns the
// leaf PTE plus the physical address it was read from. The caller decodes
// permissions and applies A/D writes through leafAddr.
//
// PLAN_MAX_RAM.md slice 4 design: every level uses overflow-checked address
// arithmetic. A wrap or a corrupted next-level PPN fails the walk with
// FAULT_NOT_PRESENT rather than aliasing into low memory.
func (cpu *CPU64) walkPageTable(vpn uint64) (leafPTE uint64, leafAddr uint64, fault bool, cause uint32) {
	tableAddr := cpu.ptbr
	for level := 0; level < PT_LEVELS; level++ {
		idx := ptLevelIndex(vpn, level)
		pteAddr, ok := addrAddChecked(tableAddr, idx*8)
		if !ok {
			return 0, 0, true, FAULT_NOT_PRESENT
		}
		pte, ok := cpu.bus.ReadPhys64WithFault(pteAddr)
		if !ok {
			return 0, 0, true, FAULT_NOT_PRESENT
		}
		if level == PT_LEVELS-1 {
			return pte, pteAddr, false, 0
		}
		nextPPN, nextFlags := parsePTE(pte)
		if nextFlags&PTE_P == 0 {
			return 0, 0, true, FAULT_NOT_PRESENT
		}
		nextTable, ok := ppnToPhysChecked(nextPPN)
		if !ok {
			return 0, 0, true, FAULT_NOT_PRESENT
		}
		tableAddr = nextTable
	}
	// Unreachable: the loop returns at level == PT_LEVELS-1.
	return 0, 0, true, FAULT_NOT_PRESENT
}

// ===========================================================================
// Address Translation
// ===========================================================================

// translateAddr translates a virtual address to a physical address using the
// current page table. Returns the physical address and fault information.
// accessType is ACCESS_READ, ACCESS_WRITE, or ACCESS_EXEC.
//
// PLAN_MAX_RAM.md slice 4 design: vaddr and physAddr are uint64. The walk
// is a 6-level sparse radix tree (top=7 bits, levels 1..5=9 bits) read
// through bus.ReadPhys64WithFault, so PTBRs and intermediate tables that
// live above the legacy 32 MB bus.memory[] window are reachable via the
// bound Backing. Address arithmetic at every level is overflow-checked.
func (cpu *CPU64) translateAddr(vaddr uint64, accessType byte) (physAddr uint64, fault bool, faultCause uint32) {
	vpn := (vaddr >> MMU_PAGE_SHIFT) & PTE_PPN_MASK
	offset := vaddr & MMU_PAGE_MASK

	var ppn uint64
	var flags byte
	var leafAddr uint64

	// TLB lookup
	if entry, hit := cpu.tlbLookup(vpn); hit {
		ppn = entry.ppn
		flags = entry.flags
		leafAddr = entry.leafAddr
	} else {
		// TLB miss: walk the multi-level page table.
		leafPTE, walkLeaf, walkFault, walkCause := cpu.walkPageTable(vpn)
		if walkFault {
			return 0, true, walkCause
		}
		leafAddr = walkLeaf
		ppn, flags = parsePTE(leafPTE)

		// Only insert into TLB if the page is present
		if flags&PTE_P != 0 {
			cpu.tlbInsert(vpn, ppn, leafAddr, flags)
		}
	}

	// Check present bit
	if flags&PTE_P == 0 {
		return 0, true, FAULT_NOT_PRESENT
	}

	// Check access permissions
	switch accessType {
	case ACCESS_READ:
		if flags&PTE_R == 0 {
			return 0, true, FAULT_READ_DENIED
		}
	case ACCESS_WRITE:
		if flags&PTE_W == 0 {
			return 0, true, FAULT_WRITE_DENIED
		}
	case ACCESS_EXEC:
		if flags&PTE_X == 0 {
			return 0, true, FAULT_EXEC_DENIED
		}
	}

	// Check user/supervisor access
	if !cpu.supervisorMode && flags&PTE_U == 0 {
		return 0, true, FAULT_USER_SUPER
	}

	// M15.6 G2: SMEP/SMAP-equivalent supervisor guards on user pages.
	// Both only fire when the supervisor is touching a PTE_U=1 page;
	// user-mode accesses are governed by the check above.
	//
	//   - SKEF (supervisor-kernel-execute-fault): blocks supervisor
	//     instruction fetch from any user page. Eliminates the
	//     "redirect PC into user shellcode" class.
	//   - SKAC (supervisor-kernel-access-check): blocks supervisor
	//     read/write on user pages unless the per-CPU SUA latch is set.
	//     Kernel enters explicit copy regions with SUAEN and leaves
	//     them with SUADIS, so accidental dereferences of an attacker-
	//     controlled pointer fault cleanly. Trap entry saves and clears
	//     SUA; ERET restores it when returning to supervisor mode.
	if cpu.supervisorMode && flags&PTE_U != 0 {
		if accessType == ACCESS_EXEC && cpu.skef {
			return 0, true, FAULT_SKEF
		}
		if (accessType == ACCESS_READ || accessType == ACCESS_WRITE) && cpu.skac && !cpu.suaLatch {
			return 0, true, FAULT_SKAC
		}
	}

	// Set A/D bits if not already set. The leaf address was determined
	// during the walk (or restored from the TLB cache); writeback uses the
	// bus phys helpers and does not have to repeat the multi-level walk.
	{
		needA := flags&PTE_A == 0
		needD := accessType == ACCESS_WRITE && flags&PTE_D == 0
		if needA || needD {
			newFlags := flags | PTE_A
			if accessType == ACCESS_WRITE {
				newFlags |= PTE_D
			}
			if pte, ok := cpu.bus.ReadPhys64WithFault(leafAddr); ok {
				pte = (pte &^ 0x7F) | uint64(newFlags)
				cpu.bus.WritePhys64(leafAddr, pte)
			}
			// Refresh TLB entry with updated flags
			cpu.tlbInsert(vpn, ppn, leafAddr, newFlags)
		}
	}

	if shifted, ok := ppnToPhysChecked(ppn); ok {
		physAddr = shifted | offset
		return physAddr, false, 0
	}
	return 0, true, FAULT_NOT_PRESENT
}

// ===========================================================================
// MMU-Aware Memory Helpers
// ===========================================================================

// mmuStackWrite pushes a value to the stack with MMU translation.
// Returns false on MMU fault (cpu.trapped is set, trap handler activated).
//
// PLAN_MAX_RAM.md slice 4: sp is uint64 so above-4-GiB stacks reach the
// correct VPN instead of the truncated low-32-bit alias. The legacy
// unsafe.Pointer fast path is preserved for translated phys addresses
// inside the legacy bus.memory[] window; high-phys results route
// through bus.WritePhys64WithFault so unmapped or out-of-backing
// addresses fault loudly instead of silently no-op'ing.
func (cpu *CPU64) mmuStackWrite(sp uint64, val uint64, memBase unsafe.Pointer, memSize uint64) bool {
	addr := sp
	if cpu.mmuEnabled {
		phys, fault, cause := cpu.translateAddr(sp, ACCESS_WRITE)
		if fault {
			cpu.trapFault(cause, sp)
			cpu.trapped = true
			return false
		}
		addr = phys
	}
	if addr+8 <= memSize && memSize >= 8 && addr <= memSize-8 {
		*(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(addr))) = val
		return true
	}
	return cpu.bus.WritePhys64WithFault(addr, val)
}

// mmuStackRead pops a value from the stack with MMU translation.
// Returns the value and false on MMU fault.
func (cpu *CPU64) mmuStackRead(sp uint64, memBase unsafe.Pointer, memSize uint64) (uint64, bool) {
	addr := sp
	if cpu.mmuEnabled {
		phys, fault, cause := cpu.translateAddr(sp, ACCESS_READ)
		if fault {
			cpu.trapFault(cause, sp)
			cpu.trapped = true
			return 0, false
		}
		addr = phys
	}
	if addr+8 <= memSize && memSize >= 8 && addr <= memSize-8 {
		return *(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(addr))), true
	}
	return cpu.bus.ReadPhys64WithFault(addr)
}

// mmuStackWriteU64 is the uint64-addressed stack write path used when sp may
// exceed the legacy 32 MB bus.memory[] window. It routes through the bus
// phys helpers so high-memory backing pages are reachable. Returns false on
// MMU fault (cpu.trapped is set) or unmapped phys address.
func (cpu *CPU64) mmuStackWriteU64(sp uint64, val uint64) bool {
	addr := sp
	if cpu.mmuEnabled {
		phys, fault, cause := cpu.translateAddr(sp, ACCESS_WRITE)
		if fault {
			cpu.trapFault(cause, sp)
			cpu.trapped = true
			return false
		}
		addr = phys
	}
	return cpu.bus.WritePhys64WithFault(addr, val)
}

// mmuStackReadU64 is the uint64-addressed stack read path. Mirrors
// mmuStackWriteU64.
func (cpu *CPU64) mmuStackReadU64(sp uint64) (uint64, bool) {
	addr := sp
	if cpu.mmuEnabled {
		phys, fault, cause := cpu.translateAddr(sp, ACCESS_READ)
		if fault {
			cpu.trapFault(cause, sp)
			cpu.trapped = true
			return 0, false
		}
		addr = phys
	}
	return cpu.bus.ReadPhys64WithFault(addr)
}
