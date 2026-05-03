package main

import "testing"

func TestMachineBusMemoryHandleStableAcrossSealSizingReset(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()
	if len(mem) == 0 {
		t.Fatal("empty bus memory")
	}
	before := &mem[0]

	bus.SealMappings()
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    uint64(len(mem)),
		ActiveVisibleRAM: uint64(len(mem)),
		VisibleCeiling:   uint64(len(mem)),
	})
	bus.Reset()

	afterMem := bus.GetMemory()
	if &afterMem[0] != before {
		t.Fatal("bus.memory backing pointer changed after SealMappings/SetSizing/Reset")
	}
}

func TestLowMemoryRemainsBusMemoryAuthoritativeWithBacking(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(64 * bMiB)
	bus.SetBacking(backing)

	bus.Write32(0x1000, 0x11223344)
	backing.Write32(0x1000, 0xAABBCCDD)

	if got := bus.Read32(0x1000); got != 0x11223344 {
		t.Fatalf("low memory read = 0x%08X, want bus.memory value", got)
	}
	if got := backing.Read32(0x1000); got != 0xAABBCCDD {
		t.Fatalf("backing low-address test value = 0x%08X, want independent backing value", got)
	}
}

func TestSetSizingUsesMaxBackedWindowNotSum(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(64 * bMiB)
	bus.SetBacking(backing)

	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    uint64(len(bus.GetMemory())) + backing.Size(),
		ActiveVisibleRAM: uint64(len(bus.GetMemory())) + backing.Size(),
		VisibleCeiling:   uint64(len(bus.GetMemory())) + backing.Size(),
	})

	if got := bus.TotalGuestRAM(); got != backing.Size() {
		t.Fatalf("TotalGuestRAM=%d, want max backing size %d", got, backing.Size())
	}
}

func TestMachineBusResetRunsHooksAfterMemoryReset(t *testing.T) {
	bus := NewMachineBus()
	bus.Write32(0x1000, 0x11223344)

	called := false
	bus.OnReset(func() {
		called = true
		if got := bus.Read32(0x1000); got != 0 {
			t.Fatalf("reset hook observed stale RAM value 0x%08X", got)
		}
	})

	bus.Reset()
	if !called {
		t.Fatal("reset hook was not called")
	}
}

func TestMachineBusResetIgnoresNilHook(t *testing.T) {
	bus := NewMachineBus()
	bus.OnReset(nil)
	bus.Reset()
}
