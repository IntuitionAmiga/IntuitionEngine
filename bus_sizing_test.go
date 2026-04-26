// bus_sizing_test.go - Tests for the bus single-source-of-truth wiring of
// total guest RAM, active visible RAM, and the visible ceiling, plus the
// AllocateGuestRAM retry-then-publish helper.
//
// PLAN_MAX_RAM.md slice 2 (RED phase).

package main

import (
	"errors"
	"testing"
)

func TestBus_SizingDefaultsAreZeroBeforeSet(t *testing.T) {
	bus := NewMachineBus()
	if bus.TotalGuestRAM() != 0 || bus.ActiveVisibleRAM() != 0 || bus.VisibleCeiling() != 0 {
		t.Fatalf("default sizing nonzero: total=%d active=%d ceiling=%d",
			bus.TotalGuestRAM(), bus.ActiveVisibleRAM(), bus.VisibleCeiling())
	}
}

func TestBus_SetSizingExposesValues(t *testing.T) {
	bus := NewMachineBus()
	ms := MemorySizing{
		Platform:         PlatformX64PC,
		VisibleCeiling:   4 * bGiB,
		TotalGuestRAM:    8 * bGiB,
		ActiveVisibleRAM: 4 * bGiB,
	}
	bus.SetSizing(ms)
	if bus.TotalGuestRAM() != 8*bGiB {
		t.Errorf("TotalGuestRAM = %d, want %d", bus.TotalGuestRAM(), uint64(8*bGiB))
	}
	if bus.ActiveVisibleRAM() != 4*bGiB {
		t.Errorf("ActiveVisibleRAM = %d, want %d", bus.ActiveVisibleRAM(), uint64(4*bGiB))
	}
	if bus.VisibleCeiling() != 4*bGiB {
		t.Errorf("VisibleCeiling = %d, want %d", bus.VisibleCeiling(), uint64(4*bGiB))
	}
	if bus.Sizing().Platform != PlatformX64PC {
		t.Errorf("Sizing().Platform = %v, want PlatformX64PC", bus.Sizing().Platform)
	}
}

func TestSysInfo_FromBusReportsBusSizing(t *testing.T) {
	bus := NewMachineBus()
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    16 * bGiB,
		ActiveVisibleRAM: 4 * bGiB,
		VisibleCeiling:   4 * bGiB,
	})
	RegisterSysInfoMMIOFromBus(bus)

	totalLo := bus.Read32(SYSINFO_TOTAL_RAM_LO)
	totalHi := bus.Read32(SYSINFO_TOTAL_RAM_HI)
	total := uint64(totalHi)<<32 | uint64(totalLo)
	if total != 16*bGiB {
		t.Errorf("sysinfo total = %#x, want %#x", total, uint64(16*bGiB))
	}
	activeLo := bus.Read32(SYSINFO_ACTIVE_RAM_LO)
	activeHi := bus.Read32(SYSINFO_ACTIVE_RAM_HI)
	active := uint64(activeHi)<<32 | uint64(activeLo)
	if active != 4*bGiB {
		t.Errorf("sysinfo active = %#x, want %#x", active, uint64(4*bGiB))
	}
}

func TestAllocateGuestRAM_RetrySync_BusSysinfoBackingAgree(t *testing.T) {
	bus := NewMachineBus()
	requested := MemorySizing{
		Platform:         PlatformX64PC,
		VisibleCeiling:   4 * bGiB,
		TotalGuestRAM:    1 * bGiB,
		ActiveVisibleRAM: 1 * bGiB,
	}
	failBeyond := uint64(256 * bMiB)
	alloc := func(size uint64) (Backing, error) {
		if size > failBeyond {
			return nil, errors.New("simulated alloc failure")
		}
		return NewContiguousBacking(size)
	}
	backing, finalMS, err := AllocateGuestRAM(bus, requested, alloc)
	if err != nil {
		t.Fatalf("AllocateGuestRAM: %v", err)
	}
	if backing.Size() > failBeyond {
		t.Fatalf("backing.Size = %d, expected <= %d", backing.Size(), failBeyond)
	}
	if backing.Size() != finalMS.TotalGuestRAM {
		t.Fatalf("backing %d != finalMS.TotalGuestRAM %d",
			backing.Size(), finalMS.TotalGuestRAM)
	}
	// active must be re-clamped to the new total.
	if finalMS.ActiveVisibleRAM > finalMS.TotalGuestRAM {
		t.Fatalf("active %d > total %d after retry",
			finalMS.ActiveVisibleRAM, finalMS.TotalGuestRAM)
	}
	// bus must reflect the same final values.
	if bus.TotalGuestRAM() != finalMS.TotalGuestRAM {
		t.Fatalf("bus.TotalGuestRAM = %d, finalMS = %d",
			bus.TotalGuestRAM(), finalMS.TotalGuestRAM)
	}
	if bus.ActiveVisibleRAM() != finalMS.ActiveVisibleRAM {
		t.Fatalf("bus.ActiveVisibleRAM = %d, finalMS = %d",
			bus.ActiveVisibleRAM(), finalMS.ActiveVisibleRAM)
	}
	// sysinfo must report the same final values.
	totalLo := bus.Read32(SYSINFO_TOTAL_RAM_LO)
	totalHi := bus.Read32(SYSINFO_TOTAL_RAM_HI)
	got := uint64(totalHi)<<32 | uint64(totalLo)
	if got != finalMS.TotalGuestRAM {
		t.Fatalf("sysinfo total = %d, finalMS = %d", got, finalMS.TotalGuestRAM)
	}
}

func TestAllocateGuestRAM_FirstAttemptDoesNotMutateOnFailure(t *testing.T) {
	bus := NewMachineBus()
	requested := MemorySizing{
		Platform:         PlatformX64PC,
		VisibleCeiling:   4 * bGiB,
		TotalGuestRAM:    64 * bMiB,
		ActiveVisibleRAM: 64 * bMiB,
	}
	alloc := func(size uint64) (Backing, error) {
		return nil, errors.New("always fails")
	}
	_, _, err := AllocateGuestRAM(bus, requested, alloc)
	if err == nil {
		t.Fatalf("expected error on persistent allocation failure")
	}
	if bus.TotalGuestRAM() != 0 || bus.ActiveVisibleRAM() != 0 {
		t.Fatalf("bus mutated after failed allocation: total=%d active=%d",
			bus.TotalGuestRAM(), bus.ActiveVisibleRAM())
	}
}
