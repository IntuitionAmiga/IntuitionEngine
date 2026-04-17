// mmu_ie64.go - IE64 Memory Management Unit: page table walk, PTE helpers, TLB

package main

import (
	"encoding/binary"
	"unsafe"
)

// ===========================================================================
// PTE Helpers
// ===========================================================================

// makePTE builds a 64-bit page table entry from a physical page number and flags.
func makePTE(ppn uint16, flags byte) uint64 {
	return uint64(flags) | (uint64(ppn) << PTE_PPN_SHIFT)
}

// parsePTE extracts the physical page number and permission flags from a PTE.
func parsePTE(pte uint64) (ppn uint16, flags byte) {
	flags = byte(pte & 0x7F) // bits 6:0 (P|R|W|X|U|A|D)
	ppn = uint16((pte >> PTE_PPN_SHIFT) & PTE_PPN_MASK)
	return
}

// ===========================================================================
// Software TLB (64-entry, direct-mapped)
// ===========================================================================

// TLBEntry caches a single page table translation.
type TLBEntry struct {
	vpn   uint16
	ppn   uint16
	flags byte // PTE permission bits (P|R|W|X|U)
	valid bool
}

// tlbLookup checks the TLB for a cached translation.
func (cpu *CPU64) tlbLookup(vpn uint16) (entry *TLBEntry, hit bool) {
	idx := vpn & 63
	e := &cpu.tlb[idx]
	if e.valid && e.vpn == vpn {
		return e, true
	}
	return nil, false
}

// tlbInsert adds or replaces a TLB entry.
func (cpu *CPU64) tlbInsert(vpn, ppn uint16, flags byte) {
	idx := vpn & 63
	cpu.tlb[idx] = TLBEntry{vpn: vpn, ppn: ppn, flags: flags, valid: true}
}

// tlbFlush invalidates all TLB entries.
func (cpu *CPU64) tlbFlush() {
	for i := range cpu.tlb {
		cpu.tlb[i].valid = false
	}
}

// tlbInvalidate invalidates the TLB entry for a specific VPN.
func (cpu *CPU64) tlbInvalidate(vpn uint16) {
	idx := vpn & 63
	if cpu.tlb[idx].vpn == vpn {
		cpu.tlb[idx].valid = false
	}
}

// ===========================================================================
// Address Translation
// ===========================================================================

// translateAddr translates a virtual address to a physical address using the
// current page table. Returns the physical address and fault information.
// accessType is ACCESS_READ, ACCESS_WRITE, or ACCESS_EXEC.
func (cpu *CPU64) translateAddr(vaddr uint32, accessType byte) (physAddr uint32, fault bool, faultCause uint32) {
	vpn := uint16((vaddr >> MMU_PAGE_SHIFT) & PTE_PPN_MASK)
	offset := vaddr & MMU_PAGE_MASK

	var ppn uint16
	var flags byte

	// TLB lookup
	if entry, hit := cpu.tlbLookup(vpn); hit {
		ppn = entry.ppn
		flags = entry.flags
	} else {
		// TLB miss: walk the page table
		pteAddr := cpu.ptbr + uint32(vpn)*8
		if uint64(pteAddr)+8 > uint64(len(cpu.memory)) {
			return 0, true, FAULT_NOT_PRESENT
		}
		pte := binary.LittleEndian.Uint64(cpu.memory[pteAddr:])
		ppn, flags = parsePTE(pte)

		// Only insert into TLB if the page is present
		if flags&PTE_P != 0 {
			cpu.tlbInsert(vpn, ppn, flags)
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

	// Set A/D bits if not already set (page tables must be in normal RAM)
	{
		needA := flags&PTE_A == 0
		needD := accessType == ACCESS_WRITE && flags&PTE_D == 0
		if needA || needD {
			newFlags := flags | PTE_A
			if accessType == ACCESS_WRITE {
				newFlags |= PTE_D
			}
			// Write back updated PTE to page table in RAM (bounds-checked)
			pteAddr := cpu.ptbr + uint32(vpn)*8
			if uint64(pteAddr)+8 <= uint64(len(cpu.memory)) {
				pte := binary.LittleEndian.Uint64(cpu.memory[pteAddr:])
				pte = (pte &^ 0x7F) | uint64(newFlags)
				binary.LittleEndian.PutUint64(cpu.memory[pteAddr:], pte)
			}
			// Refresh TLB entry with updated flags
			cpu.tlbInsert(vpn, ppn, newFlags)
		}
	}

	physAddr = (uint32(ppn) << MMU_PAGE_SHIFT) | offset
	return physAddr, false, 0
}

// ===========================================================================
// MMU-Aware Memory Helpers
// ===========================================================================

// mmuStackWrite pushes a value to the stack with MMU translation.
// Returns false on MMU fault (cpu.trapped is set, trap handler activated).
func (cpu *CPU64) mmuStackWrite(sp uint32, val uint64, memBase unsafe.Pointer, memSize uint64) bool {
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
	if uint64(addr)+8 > memSize {
		return false
	}
	*(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(addr))) = val
	return true
}

// mmuStackRead pops a value from the stack with MMU translation.
// Returns the value and false on MMU fault.
func (cpu *CPU64) mmuStackRead(sp uint32, memBase unsafe.Pointer, memSize uint64) (uint64, bool) {
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
	if uint64(addr)+8 > memSize {
		return 0, false
	}
	return *(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(addr))), true
}
