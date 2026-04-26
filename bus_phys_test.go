// bus_phys_test.go - Tests for the IE64 uint64-addressed physical bus path.
//
// PLAN_MAX_RAM.md slice 3 (RED phase): widen IE64 address plumbing to use
// uint64 addresses without truncation, MMU/TLB unchanged. Low addresses
// bridge to the legacy MMIO/memory dispatch; addresses at or above the
// legacy memory length use the bound Backing.

package main

import (
	"testing"
)

// --- Low-address bridge: legacy memory and MMIO remain reachable. ---

func TestBusPhys_LowAddrBridgesToLegacyMemory(t *testing.T) {
	bus := NewMachineBus()
	bus.WritePhys32(0x4000, 0xDEADBEEF)
	if got := bus.Read32(0x4000); got != 0xDEADBEEF {
		t.Fatalf("legacy Read32 = %#x, want 0xDEADBEEF", got)
	}
	if got := bus.ReadPhys32(0x4000); got != 0xDEADBEEF {
		t.Fatalf("ReadPhys32 = %#x, want 0xDEADBEEF", got)
	}
}

func TestBusPhys_LowAddrBridgesToLegacyMMIO(t *testing.T) {
	bus := NewMachineBus()
	var lastAddr uint32
	var lastVal uint32
	bus.MapIO(0xF1000, 0xF1003,
		func(addr uint32) uint32 { return 0xCAFEBABE },
		func(addr uint32, value uint32) {
			lastAddr = addr
			lastVal = value
		},
	)
	if got := bus.ReadPhys32(0xF1000); got != 0xCAFEBABE {
		t.Fatalf("ReadPhys32 = %#x, want 0xCAFEBABE", got)
	}
	bus.WritePhys32(0xF1000, 0x12345678)
	if lastAddr != 0xF1000 || lastVal != 0x12345678 {
		t.Fatalf("MMIO write not dispatched: addr=%#x val=%#x", lastAddr, lastVal)
	}
}

// --- Above legacy memory: backing must cover the address. ---

func TestBusPhys_AboveLowMemoryUsesBacking(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(8 * bGiB)
	bus.SetBacking(backing)

	addr := uint64(6) * bGiB
	bus.WritePhys32(addr, 0xA1B2C3D4)
	if got := bus.ReadPhys32(addr); got != 0xA1B2C3D4 {
		t.Fatalf("ReadPhys32 above 4GiB = %#x, want 0xA1B2C3D4", got)
	}
	// Round-trip through backing directly to confirm no truncation.
	if got := backing.Read32(addr); got != 0xA1B2C3D4 {
		t.Fatalf("backing.Read32 = %#x, want 0xA1B2C3D4", got)
	}
}

func TestBusPhys_AddressNotTruncatedToUint32(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(8 * bGiB)
	bus.SetBacking(backing)

	// Write at exactly the 4 GiB boundary.
	hi := uint64(1) << 32
	bus.WritePhys32(hi, 0x11223344)

	// Confirm legacy Read32(0) does NOT see the value.
	if got := bus.Read32(0); got != 0 {
		t.Fatalf("legacy Read32(0) = %#x, leaked from above-4GiB write", got)
	}
	// Confirm backing has it at the true uint64 address.
	if got := backing.Read32(hi); got != 0x11223344 {
		t.Fatalf("backing.Read32(%#x) = %#x, want 0x11223344", hi, got)
	}
}

func TestBusPhys_Read64Write64RoundTripsAbove4GiB(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(8 * bGiB)
	bus.SetBacking(backing)

	addr := (uint64(5) * bGiB) + 0x100
	val := uint64(0x0011223344556677)
	bus.WritePhys64(addr, val)
	if got := bus.ReadPhys64(addr); got != val {
		t.Fatalf("ReadPhys64 = %#x, want %#x", got, val)
	}
}

func TestBusPhys_Read8Write8RoundTripsAbove4GiB(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(8 * bGiB)
	bus.SetBacking(backing)

	addr := uint64(7) * bGiB
	bus.WritePhys8(addr, 0xAB)
	if got := bus.ReadPhys8(addr); got != 0xAB {
		t.Fatalf("ReadPhys8 = %#x, want 0xAB", got)
	}
}

func TestBusPhys_OutOfBackingReturnsZero(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(1 * bGiB)
	bus.SetBacking(backing)

	addr := uint64(2) * bGiB
	if got := bus.ReadPhys32(addr); got != 0 {
		t.Fatalf("ReadPhys32 OOB = %#x, want 0", got)
	}
	bus.WritePhys32(addr, 0xFFFFFFFF)
	if backing.AllocatedPages() != 0 {
		t.Fatalf("OOB write allocated %d pages", backing.AllocatedPages())
	}
}

func TestBusPhys_NoBackingAboveLowReturnsZero(t *testing.T) {
	bus := NewMachineBus()
	addr := uint64(2) * bGiB
	if got := bus.ReadPhys32(addr); got != 0 {
		t.Fatalf("ReadPhys32 with no backing = %#x, want 0", got)
	}
	bus.WritePhys32(addr, 0xFFFFFFFF) // must not panic
}

// --- Fault-reporting variants. ---

func TestBusPhys_Read64WithFault_OOB_ReturnsFalse(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(1 * bGiB)
	bus.SetBacking(backing)

	if _, ok := bus.ReadPhys64WithFault(uint64(2) * bGiB); ok {
		t.Fatalf("ReadPhys64WithFault OOB returned ok=true")
	}
	if ok := bus.WritePhys64WithFault(uint64(2)*bGiB, 0); ok {
		t.Fatalf("WritePhys64WithFault OOB returned ok=true")
	}
}

func TestBusPhys_Read64WithFault_AboveLow_OK(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(8 * bGiB)
	bus.SetBacking(backing)

	addr := uint64(5) * bGiB
	if ok := bus.WritePhys64WithFault(addr, 0xFEEDFACECAFEBEEF); !ok {
		t.Fatalf("WritePhys64WithFault returned ok=false")
	}
	got, ok := bus.ReadPhys64WithFault(addr)
	if !ok {
		t.Fatalf("ReadPhys64WithFault returned ok=false")
	}
	if got != 0xFEEDFACECAFEBEEF {
		t.Fatalf("ReadPhys64WithFault = %#x, want 0xFEEDFACECAFEBEEF", got)
	}
}

// --- Legacy Bus32 path remains unchanged. ---

func TestBusPhys_LegacyBus32Unchanged(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(8 * bGiB)
	bus.SetBacking(backing)

	// Legacy 32-bit accessors only see the low 32 MB memory window.
	bus.Write32(0x10000, 0xAABBCCDD)
	if got := bus.Read32(0x10000); got != 0xAABBCCDD {
		t.Fatalf("legacy Read32 = %#x, want 0xAABBCCDD", got)
	}
	// And do NOT leak the above-4GiB store.
	bus.WritePhys32(uint64(5)*bGiB, 0x99999999)
	if got := bus.Read32(0x10000); got != 0xAABBCCDD {
		t.Fatalf("legacy Read32 corrupted by phys write = %#x", got)
	}
}

func TestBusPhys_BusBindsBackingViaAllocateGuestRAM(t *testing.T) {
	bus := NewMachineBus()
	requested := MemorySizing{
		Platform:         PlatformX64PC,
		VisibleCeiling:   8 * bGiB,
		TotalGuestRAM:    256 * bMiB,
		ActiveVisibleRAM: 256 * bMiB,
	}
	alloc := func(size uint64) (Backing, error) {
		return NewContiguousBacking(size)
	}
	_, _, err := AllocateGuestRAM(bus, requested, alloc)
	if err != nil {
		t.Fatalf("AllocateGuestRAM: %v", err)
	}
	if bus.Backing() == nil {
		t.Fatalf("AllocateGuestRAM did not bind backing on bus")
	}
	if bus.Backing().Size() != 256*bMiB {
		t.Fatalf("bus.Backing().Size() = %d, want %d", bus.Backing().Size(), 256*bMiB)
	}
}

// Bus64Phys interface compliance check (compile-time guard).
var _ Bus64Phys = (*MachineBus)(nil)

// --- Boundary-straddling: legacy/backing seam is unmapped. ---

func TestBusPhys_StraddlingLegacyBackingBoundaryIsUnmapped(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(2 * bGiB)
	bus.SetBacking(backing)

	// Pre-seed legacy memory with a known sentinel pattern at the top of
	// the low-memory window so we can detect any accidental clobber.
	low := uint64(DEFAULT_MEMORY_SIZE) - 8
	for i := uint64(0); i < 8; i++ {
		bus.Write8(uint32(low+i), 0xAA)
	}

	// Straddle: addr starts inside legacy memory, span extends past
	// len(bus.memory). The new contract treats the seam as unmapped.
	straddle := uint64(DEFAULT_MEMORY_SIZE) - 4
	bus.WritePhys64(straddle, 0xDEADBEEFCAFEBABE)

	// Legacy memory must remain untouched.
	for i := uint64(0); i < 8; i++ {
		if got := bus.Read8(uint32(low + i)); got != 0xAA {
			t.Fatalf("legacy clobbered at +%d: got %#x want 0xAA", i, got)
		}
	}

	// Backing must remain pristine (no allocated pages).
	if backing.AllocatedPages() != 0 {
		t.Fatalf("backing got %d pages from straddling write, want 0", backing.AllocatedPages())
	}

	// Read back through phys path: also unmapped, returns zero.
	if got := bus.ReadPhys64(straddle); got != 0 {
		t.Fatalf("ReadPhys64 straddling = %#x, want 0 (unmapped)", got)
	}

	// Fault variants must report the unmapped access.
	if _, ok := bus.ReadPhys64WithFault(straddle); ok {
		t.Fatalf("ReadPhys64WithFault straddling returned ok=true")
	}
	if ok := bus.WritePhys64WithFault(straddle, 0); ok {
		t.Fatalf("WritePhys64WithFault straddling returned ok=true")
	}
}

func TestBusPhys_StraddlingBoundary32BitIsUnmapped(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(2 * bGiB)
	bus.SetBacking(backing)

	// 32-bit straddle: starts at len-2, length 4 → bytes at [len-2..len+1].
	straddle := uint64(DEFAULT_MEMORY_SIZE) - 2
	bus.WritePhys32(straddle, 0x11223344)
	if backing.AllocatedPages() != 0 {
		t.Fatalf("32-bit straddling write touched backing")
	}
	if got := bus.ReadPhys32(straddle); got != 0 {
		t.Fatalf("32-bit straddling read = %#x, want 0", got)
	}
}

// --- Reset clears bound backing. ---

func TestBusPhys_ResetClearsBoundBacking(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(8 * bGiB)
	bus.SetBacking(backing)

	addr := uint64(5) * bGiB
	bus.WritePhys64(addr, 0x0123456789ABCDEF)
	if got := bus.ReadPhys64(addr); got != 0x0123456789ABCDEF {
		t.Fatalf("pre-reset ReadPhys64 = %#x", got)
	}

	bus.Reset()

	if got := bus.ReadPhys64(addr); got != 0 {
		t.Fatalf("post-reset ReadPhys64 = %#x, want 0", got)
	}
	if backing.AllocatedPages() != 0 {
		t.Fatalf("post-reset backing has %d pages, want 0", backing.AllocatedPages())
	}
}
