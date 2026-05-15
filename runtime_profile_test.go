package main

import "testing"

func TestApplyRuntimeVisibleRAMForMode_ArosFromBasicBusClampsToBusMemory(t *testing.T) {
	bus, err := NewMachineBusSized(lowMemWindowBytes)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	bus.SetBacking(NewSparseBacking(8 * bGiB))
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    8 * bGiB,
		ActiveVisibleRAM: 8 * bGiB,
	})

	applyRuntimeVisibleRAMForMode(bus, "aros")

	if got := bus.ActiveVisibleRAM(); got != lowMemWindowBytes {
		t.Fatalf("AROS dynamic active visible RAM = 0x%X, want bus.memory window 0x%X", got, lowMemWindowBytes)
	}
	pb := AROSProfileBounds(bus)
	if pb.Err != nil {
		t.Fatalf("AROS profile should fit dynamic BASIC bus after clamp: %v", pb.Err)
	}
	if pb.TopOfRAM != uint32(lowMemWindowBytes) {
		t.Fatalf("AROS TopOfRAM = 0x%X, want 0x%X", pb.TopOfRAM, lowMemWindowBytes)
	}
}

func TestApplyRuntimeVisibleRAMForMode_RestoresIE64Total(t *testing.T) {
	bus, err := NewMachineBusSized(lowMemWindowBytes)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	bus.SetBacking(NewSparseBacking(8 * bGiB))
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    8 * bGiB,
		ActiveVisibleRAM: lowMemWindowBytes,
	})

	applyRuntimeVisibleRAMForMode(bus, "ie64")

	if got := bus.ActiveVisibleRAM(); got != 8*bGiB {
		t.Fatalf("IE64 active visible RAM = 0x%X, want total guest RAM 0x%X", got, uint64(8*bGiB))
	}
}
