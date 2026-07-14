//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import (
	"testing"
)

// TestJIT_ConstAddr_MMUMappedToIO_UsesHelper is the regression test for the
// micro-TLB-hit hazard in constant-address elision. The constant virtual
// address 0x3100 satisfies ie64ConstLowRAMAccess (base R0, below IO_REGION_START),
// but under the MMU it is mapped to a PHYSICAL I/O page. The elision must not
// fire on the MMU path: after translation the register holds a physical I/O
// address, so the access must route through the helper (cpu.storeMem/loadMem)
// exactly as the interpreter does, reaching the device rather than raw RAM.
func TestJIT_ConstAddr_MMUMappedToIO_UsesHelper(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	// Device at physical page 0xB0 (0xB0000), inside the I/O region.
	const ioPhysBase = uint32(0xB0000)
	var gotWriteAddr uint32
	var gotWriteVal uint32
	var writeCount int
	cpu.bus.MapIO(ioPhysBase, ioPhysBase+0xFFF,
		func(addr uint32) uint32 { return 0xCAFEBABE },
		func(addr uint32, value uint32) { gotWriteAddr = addr; gotWriteVal = value; writeCount++ })

	// Map virtual page 3 -> physical page 0xB0 (the I/O page).
	writePTE(cpu, 3, makePTE(0xB0, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	// MOVE R2, #0xF00D ; STORE.L R2, 0x3100(R0) ; then four const-address LOADs
	// of the same page so the micro-TLB is warm and a hit is exercised on the
	// later loads (the exact condition the pre-fix elision mishandled). Each
	// LOAD must read the device (0xCAFEBABE), never raw/shadow RAM (0xF00D).
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 2, IE64_SIZE_L, 1, 0, 0, 0xF00D),
		ie64Instr(OP_STORE, 2, IE64_SIZE_L, 0, 0, 0, 0x3100),
		ie64Instr(OP_LOAD, 3, IE64_SIZE_L, 0, 0, 0, 0x3100),
		ie64Instr(OP_LOAD, 4, IE64_SIZE_L, 0, 0, 0, 0x3100),
		ie64Instr(OP_LOAD, 5, IE64_SIZE_L, 0, 0, 0, 0x3100),
		ie64Instr(OP_LOAD, 6, IE64_SIZE_L, 0, 0, 0, 0x3100),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	// Store must have reached the device, not been elided into raw memory.
	if writeCount == 0 {
		t.Fatalf("STORE did not reach the I/O device: elision bypassed the MMU helper")
	}
	if gotWriteAddr != 0xB0100 || gotWriteVal != 0xF00D {
		t.Fatalf("device write = addr 0x%X val 0x%X, want addr 0xB0100 val 0xF00D", gotWriteAddr, gotWriteVal)
	}
	// Every LOAD (including micro-TLB hits) must read the device value.
	for r, reg := range []int{3, 4, 5, 6} {
		if cpu.regs[reg] != 0xCAFEBABE {
			t.Fatalf("LOAD #%d (R%d) = 0x%X, want 0xCAFEBABE (device read via helper, not elided raw RAM)",
				r, reg, cpu.regs[reg])
		}
	}
}

// TestJIT_ConstAddr_MMUMappedToIO_StoreWarmTLB strengthens the store side of
// the micro-TLB-hit hazard: it issues several constant-address STOREs to the
// same MMU-mapped I/O page so the write micro-TLB is warm and later stores hit.
// Every store must route through the helper to the device; a store that wrongly
// elided on a write-TLB hit would write raw RAM and skip the device callback,
// so the device write count must equal the number of stores.
func TestJIT_ConstAddr_MMUMappedToIO_StoreWarmTLB(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.jitEnabled = true
	setupIdentityMMU(cpu, 160)

	const ioPhysBase = uint32(0xB0000)
	var writeCount int
	var lastVal uint32
	cpu.bus.MapIO(ioPhysBase, ioPhysBase+0xFFF,
		func(addr uint32) uint32 { return 0 },
		func(addr uint32, value uint32) { writeCount++; lastVal = value })

	writePTE(cpu, 3, makePTE(0xB0, PTE_P|PTE_R|PTE_W|PTE_X|PTE_U))

	// Four distinct const-address STOREs to the same page: the write micro-TLB
	// warms after the first, so the later ones exercise the hit path.
	rig.loadInstructions(
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x1111),
		ie64Instr(OP_STORE, 1, IE64_SIZE_L, 0, 0, 0, 0x3100),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x2222),
		ie64Instr(OP_STORE, 1, IE64_SIZE_L, 0, 0, 0, 0x3100),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x3333),
		ie64Instr(OP_STORE, 1, IE64_SIZE_L, 0, 0, 0, 0x3100),
		ie64Instr(OP_MOVE, 1, IE64_SIZE_L, 1, 0, 0, 0x4444),
		ie64Instr(OP_STORE, 1, IE64_SIZE_L, 0, 0, 0, 0x3100),
		ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0),
	)
	cpu.running.Store(true)
	cpu.jitExecute()

	if writeCount != 4 {
		t.Fatalf("device write count = %d, want 4 (a write-TLB-hit store bypassed the helper into raw RAM)", writeCount)
	}
	if lastVal != 0x4444 {
		t.Fatalf("last device write = 0x%X, want 0x4444", lastVal)
	}
}
