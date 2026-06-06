package main

import "testing"

func newMMUSharedWalkRig(t *testing.T) *CPU64 {
	t.Helper()
	mmuTestResetPools()
	t.Cleanup(mmuTestResetPools)

	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.mmuEnabled = true
	cpu.supervisorMode = true
	cpu.ptbr = 0x20000
	return cpu
}

func TestMMUSharedWalk_CPUReadSetsAccessedOnly(t *testing.T) {
	cpu := newMMUSharedWalkRig(t)
	const va uint64 = 0x3000
	const ppn uint64 = 0x09
	mmuMap(cpu, va, ppn, PTE_P|PTE_R|PTE_W|PTE_U)

	phys, fault, cause := cpu.translateAddr(va, ACCESS_READ)
	if fault {
		t.Fatalf("translateAddr read fault cause=%d", cause)
	}
	if phys != ppn<<MMU_PAGE_SHIFT {
		t.Fatalf("translateAddr read phys=0x%X, want 0x%X", phys, ppn<<MMU_PAGE_SHIFT)
	}
	_, flags := parsePTE(mmuLeafPTE(cpu, va))
	if flags&PTE_A == 0 {
		t.Fatalf("CPU read did not set PTE_A: flags=%#x", flags)
	}
	if flags&PTE_D != 0 {
		t.Fatalf("CPU read unexpectedly set PTE_D: flags=%#x", flags)
	}
}

func TestMMUSharedWalk_CPUWriteSetsAccessedAndDirty(t *testing.T) {
	cpu := newMMUSharedWalkRig(t)
	const va uint64 = 0x4000
	const ppn uint64 = 0x0A
	mmuMap(cpu, va, ppn, PTE_P|PTE_R|PTE_W|PTE_U)

	phys, fault, cause := cpu.translateAddr(va, ACCESS_WRITE)
	if fault {
		t.Fatalf("translateAddr write fault cause=%d", cause)
	}
	if phys != ppn<<MMU_PAGE_SHIFT {
		t.Fatalf("translateAddr write phys=0x%X, want 0x%X", phys, ppn<<MMU_PAGE_SHIFT)
	}
	_, flags := parsePTE(mmuLeafPTE(cpu, va))
	if flags&(PTE_A|PTE_D) != PTE_A|PTE_D {
		t.Fatalf("CPU write did not set PTE_A|PTE_D: flags=%#x", flags)
	}
}

func TestMMUSharedWalk_HostReadWriteDoNotMutateAccessedOrDirty(t *testing.T) {
	cpu := newMMUSharedWalkRig(t)
	const va uint64 = 0x5000
	const ppn uint64 = 0x0B
	mmuMap(cpu, va, ppn, PTE_P|PTE_R|PTE_W|PTE_U)

	dev := NewBootstrapHostFSDevice(cpu.bus, "")
	dev.arg4 = uint32(cpu.ptbr)
	ptr := uint32(va)

	readPhys, readOK := dev.translateGuestVA(ptr, false)
	writePhys, writeOK := dev.translateGuestVA(ptr, true)
	wantPhys := ppn << MMU_PAGE_SHIFT
	if !readOK || readPhys != wantPhys {
		t.Fatalf("host read translate=(0x%X,%t), want (0x%X,true)", readPhys, readOK, wantPhys)
	}
	if !writeOK || writePhys != wantPhys {
		t.Fatalf("host write translate=(0x%X,%t), want (0x%X,true)", writePhys, writeOK, wantPhys)
	}
	_, flags := parsePTE(mmuLeafPTE(cpu, va))
	if flags&(PTE_A|PTE_D) != 0 {
		t.Fatalf("host translation mutated A/D bits: flags=%#x", flags)
	}
}

func TestMMUSharedWalk_HostRequiresUserMapping(t *testing.T) {
	cpu := newMMUSharedWalkRig(t)
	const va uint64 = 0x6000
	mmuMap(cpu, va, 0x0C, PTE_P|PTE_R|PTE_W)

	dev := NewBootstrapHostFSDevice(cpu.bus, "")
	dev.arg4 = uint32(cpu.ptbr)
	if phys, ok := dev.translateGuestVA(uint32(va), false); ok {
		t.Fatalf("host read accepted supervisor-only mapping phys=0x%X", phys)
	}
	if phys, ok := dev.translateGuestVA(uint32(va), true); ok {
		t.Fatalf("host write accepted supervisor-only mapping phys=0x%X", phys)
	}
}
