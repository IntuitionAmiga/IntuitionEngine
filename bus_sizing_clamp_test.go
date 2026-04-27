// bus_sizing_clamp_test.go - PLAN_MAX_RAM.md slice 9.
//
// Asserts the production sizing path's plan invariant:
//
//   "production discovery must never report unbacked RAM as
//    total_guest_ram or usable guest RAM. If backing is allocated only
//    for the active CPU/profile, total_guest_ram is the backed guest
//    RAM size, not the larger host-derived candidate."
//
// SetSizing clamps published TotalGuestRAM (and the
// ActiveVisibleRAM <= TotalGuestRAM follow-on) to the actual backed
// guest RAM the bus can serve: max(len(bus.memory), backing.Size())
// when a Backing is bound, otherwise just len(bus.memory). Backing.Size()
// is the absolute upper bound of the backing-mapped range
// (addrInBacking checks end <= backing.Size()), so the legacy memory
// window and the backing share the same address space rather than
// stacking.

package main

import "testing"

func TestBusSetSizing_ClampsTotalToBackedMemoryWhenNoBacking(t *testing.T) {
	bus := NewMachineBus()
	hostScale := uint64(8) * 1024 * 1024 * 1024 // 8 GiB host-detected candidate

	// Production main.go publishes the autodetected sizing without
	// allocating a Backing for the high range. The bus must clamp the
	// reported total to its actual backed window so SYSINFO does not
	// advertise unbacked RAM.
	bus.SetSizing(MemorySizing{
		Platform:          PlatformX64PC,
		DetectedUsableRAM: hostScale,
		TotalGuestRAM:     hostScale,
		ActiveVisibleRAM:  uint64(DEFAULT_MEMORY_SIZE),
		VisibleCeiling:    uint64(DEFAULT_MEMORY_SIZE),
	})

	if got := bus.TotalGuestRAM(); got != uint64(len(bus.memory)) {
		t.Fatalf("TotalGuestRAM = 0x%X, want 0x%X (clamped to len(bus.memory))",
			got, uint64(len(bus.memory)))
	}
	if got := bus.ActiveVisibleRAM(); got > bus.TotalGuestRAM() {
		t.Fatalf("ActiveVisibleRAM = 0x%X > TotalGuestRAM = 0x%X", got, bus.TotalGuestRAM())
	}
}

func TestBusSetSizing_PreservesBackingScaledTotal(t *testing.T) {
	bus := NewMachineBus()
	const advertised uint64 = 4 * 1024 * 1024 * 1024 // 4 GiB
	bus.SetBacking(NewSparseBacking(advertised))

	bus.SetSizing(MemorySizing{
		Platform:         PlatformX64PC,
		TotalGuestRAM:    advertised,
		ActiveVisibleRAM: advertised,
		VisibleCeiling:   advertised,
	})

	if got := bus.TotalGuestRAM(); got != advertised {
		t.Fatalf("TotalGuestRAM = 0x%X, want 0x%X (backing-advertised)", got, advertised)
	}
}

func TestBusSetSizing_ClampsTotalToMaxOfMemoryAndBacking(t *testing.T) {
	// backing.Size() is the absolute upper bound of the backing-mapped
	// range (addrInBacking checks end <= backing.Size()). The backed
	// total is max(len(memory), backing.Size()), not their sum — the
	// legacy memory window and the backing share the same address space.
	bus := NewMachineBus()
	const advertised uint64 = uint64(DEFAULT_MEMORY_SIZE) * 2 // 64 MiB
	bus.SetBacking(NewSparseBacking(advertised))

	hostScale := uint64(8) * 1024 * 1024 * 1024
	bus.SetSizing(MemorySizing{
		Platform:         PlatformX64PC,
		TotalGuestRAM:    hostScale,
		ActiveVisibleRAM: uint64(DEFAULT_MEMORY_SIZE),
		VisibleCeiling:   uint64(DEFAULT_MEMORY_SIZE),
	})

	if got := bus.TotalGuestRAM(); got != advertised {
		t.Fatalf("TotalGuestRAM = 0x%X, want 0x%X (max(len(memory), backing.Size()))",
			got, advertised)
	}
}

func TestBusSetSizing_BackingSmallerThanLegacyMemoryStillAdvertisesLegacy(t *testing.T) {
	// Pathological case: backing.Size() < len(bus.memory). The legacy
	// window already covers more than the backing claims; the backed
	// total must remain len(bus.memory) so SYSINFO does not under-report.
	bus := NewMachineBus()
	bus.SetBacking(NewSparseBacking(uint64(DEFAULT_MEMORY_SIZE) / 2)) // 16 MiB
	hostScale := uint64(8) * 1024 * 1024 * 1024
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    hostScale,
		ActiveVisibleRAM: uint64(DEFAULT_MEMORY_SIZE),
	})
	if got := bus.TotalGuestRAM(); got != uint64(len(bus.memory)) {
		t.Fatalf("TotalGuestRAM = 0x%X, want 0x%X (legacy memory wins when backing is smaller)",
			got, uint64(len(bus.memory)))
	}
}

func TestBusSetSizing_ActiveOnlyInputIsPreserved(t *testing.T) {
	// Legacy callers hand-build a MemorySizing setting only
	// ActiveVisibleRAM (TotalGuestRAM left zero). The clamp must NOT
	// zero out the active value via the active <= total comparison;
	// it must preserve the active size as long as it fits the backed
	// window. ActiveVisiblePages() depends on this.
	bus := NewMachineBus()
	const active uint64 = 16 * 1024 * 1024
	bus.SetSizing(MemorySizing{
		ActiveVisibleRAM: active,
	})
	if got := bus.ActiveVisibleRAM(); got != active {
		t.Fatalf("ActiveVisibleRAM = 0x%X, want 0x%X (active-only input must survive clamp)",
			got, active)
	}
	if got := bus.TotalGuestRAM(); got != 0 {
		t.Fatalf("TotalGuestRAM = 0x%X, want 0 (caller did not set it)", got)
	}
}

func TestBusSetSizing_ActiveOnlyInputAboveBackedIsCallerResponsibility(t *testing.T) {
	// Active-only callers (TotalGuestRAM=0) keep their ActiveVisibleRAM
	// verbatim. SetSizing does not silently clamp active against the
	// backed window — that would zero-out hand-built test sizings such
	// as TestPhase4c_ActiveVisiblePages_RoundsDownToWholePages and
	// surprise legacy callers. Honesty against backed RAM is enforced
	// via the TotalGuestRAM clamp + the active<=total chain when both
	// are set, which is the production path through ComputeMemorySizing.
	bus := NewMachineBus()
	const active uint64 = 64 * 1024 * 1024 // larger than legacy bus.memory
	bus.SetSizing(MemorySizing{
		ActiveVisibleRAM: active,
	})
	if got := bus.ActiveVisibleRAM(); got != active {
		t.Fatalf("ActiveVisibleRAM = 0x%X, want 0x%X (active-only must not be clamped against backed)",
			got, active)
	}
}

func TestBusSetSizing_ActiveCannotExceedBackedTotal(t *testing.T) {
	bus := NewMachineBus()
	hostScale := uint64(8) * 1024 * 1024 * 1024
	// Caller hands in active = total = host-scale; both must clamp.
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    hostScale,
		ActiveVisibleRAM: hostScale,
		VisibleCeiling:   hostScale,
	})

	backed := uint64(len(bus.memory))
	if bus.TotalGuestRAM() != backed {
		t.Fatalf("TotalGuestRAM = 0x%X, want 0x%X", bus.TotalGuestRAM(), backed)
	}
	if bus.ActiveVisibleRAM() > bus.TotalGuestRAM() {
		t.Fatalf("ActiveVisibleRAM = 0x%X > TotalGuestRAM = 0x%X",
			bus.ActiveVisibleRAM(), bus.TotalGuestRAM())
	}
}

func TestBusSetSizing_SysInfoMatchesClampedValues(t *testing.T) {
	bus := NewMachineBus()
	hostScale := uint64(8) * 1024 * 1024 * 1024
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    hostScale,
		ActiveVisibleRAM: uint64(DEFAULT_MEMORY_SIZE),
		VisibleCeiling:   uint64(DEFAULT_MEMORY_SIZE),
	})
	RegisterSysInfoMMIOFromBus(bus)

	loT := bus.Read32(SYSINFO_TOTAL_RAM_LO)
	hiT := bus.Read32(SYSINFO_TOTAL_RAM_HI)
	publishedTotal := uint64(hiT)<<32 | uint64(loT)
	if publishedTotal != bus.TotalGuestRAM() {
		t.Fatalf("SYSINFO total = 0x%X, bus.TotalGuestRAM = 0x%X (must agree post-clamp)",
			publishedTotal, bus.TotalGuestRAM())
	}
	if publishedTotal > uint64(len(bus.memory)) {
		t.Fatalf("SYSINFO total = 0x%X > len(bus.memory) = 0x%X (advertised unbacked RAM)",
			publishedTotal, uint64(len(bus.memory)))
	}
}
