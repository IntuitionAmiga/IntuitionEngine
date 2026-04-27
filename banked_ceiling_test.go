// banked_ceiling_test.go - PLAN_MAX_RAM.md slice 8 phase 1.
//
// Asserts that 6502 and Z80 bank translation rejects addresses above the
// configured banked-CPU visible ceiling, not above the legacy fixed
// DEFAULT_MEMORY_SIZE constant. The plan's Banking ABI Policy:
//
// > Bank translation must reject addresses above that CPU's configured
// > visible ceiling, not above IE64's full RAM ceiling.

package main

import "testing"

// installBankedCeiling configures the bus with a forced banked-visible
// ceiling so the test can verify that bank translation honours it.
func installBankedCeiling(t *testing.T, bus *MachineBus, ceiling uint64) {
	t.Helper()
	bus.SetSizing(MemorySizing{
		Platform:         PlatformX64PC,
		HostReserve:      0,
		VisibleCeiling:   ceiling,
		TotalGuestRAM:    ceiling,
		ActiveVisibleRAM: ceiling,
	})
}

func Test6502_BankTranslation_RejectsAboveBankedCeiling(t *testing.T) {
	bus := NewMachineBus()
	const ceiling uint64 = 16 * 1024 * 1024
	installBankedCeiling(t, bus, ceiling)
	adapter := NewBus6502Adapter(bus)

	// Bank 1 window is 8 KiB at 0x2000-0x3FFF. A bank index of 0x800 maps
	// to physical 0x800 * 0x2000 = 0x1000000 (16 MiB) — exactly on the new
	// ceiling, so should be rejected. The legacy DEFAULT_MEMORY_SIZE check
	// would have accepted because 0x1000000 < 32 MiB.
	adapter.bank1 = 0x800
	adapter.bank1Enable = true
	if _, ok := adapter.translateExtendedBank(BANK1_WINDOW_BASE); ok {
		t.Fatalf("bank1 translate at ceiling=0x%X must be rejected", ceiling)
	}

	// Sanity: translation below ceiling still works.
	adapter.bank1 = 0x100 // 0x100 * 0x2000 = 0x200000 (2 MiB), well below 16 MiB
	if _, ok := adapter.translateExtendedBank(BANK1_WINDOW_BASE); !ok {
		t.Fatalf("bank1 translate below ceiling must be accepted")
	}
}

func Test6502_BankTranslation_AcceptsLegacyCeilingWhenBusUnsized(t *testing.T) {
	// When SetSizing has not been called, the banked ceiling falls back to
	// len(bus.memory) which equals the historical DEFAULT_MEMORY_SIZE. The
	// pre-slice-8 behaviour must be preserved for that path.
	bus := NewMachineBus()
	adapter := NewBus6502Adapter(bus)
	adapter.bank1 = 0x100 // 2 MiB
	adapter.bank1Enable = true
	if _, ok := adapter.translateExtendedBank(BANK1_WINDOW_BASE); !ok {
		t.Fatalf("bank1 translate to legacy 2 MiB must succeed under default sizing")
	}
	// 32 MiB exactly should still be rejected (boundary preserved).
	adapter.bank1 = 0x1000 // 0x1000 * 0x2000 = 32 MiB
	if _, ok := adapter.translateExtendedBank(BANK1_WINDOW_BASE); ok {
		t.Fatalf("bank1 translate at legacy ceiling must be rejected")
	}
}

func TestZ80_BankTranslation_RejectsAboveBankedCeiling(t *testing.T) {
	bus := NewMachineBus()
	const ceiling uint64 = 16 * 1024 * 1024
	installBankedCeiling(t, bus, ceiling)
	adapter := NewZ80BusAdapter(bus)

	adapter.bank1 = 0x800 // 16 MiB
	adapter.bank1Enable = true
	if _, ok := adapter.translateExtendedBank(Z80_BANK1_WINDOW_BASE); ok {
		t.Fatalf("z80 bank1 translate at ceiling=0x%X must be rejected", ceiling)
	}

	adapter.bank1 = 0x100 // 2 MiB
	if _, ok := adapter.translateExtendedBank(Z80_BANK1_WINDOW_BASE); !ok {
		t.Fatalf("z80 bank1 translate below ceiling must be accepted")
	}
}

func TestZ80_BankTranslation_AcceptsLegacyCeilingWhenBusUnsized(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	adapter.bank1 = 0x100
	adapter.bank1Enable = true
	if _, ok := adapter.translateExtendedBank(Z80_BANK1_WINDOW_BASE); !ok {
		t.Fatalf("z80 bank1 translate to legacy 2 MiB must succeed under default sizing")
	}
	adapter.bank1 = 0x1000 // 32 MiB
	if _, ok := adapter.translateExtendedBank(Z80_BANK1_WINDOW_BASE); ok {
		t.Fatalf("z80 bank1 translate at legacy ceiling must be rejected")
	}
}

func TestMachineBus_BankedVisibleCeiling_FollowsSizingThenLenMemory(t *testing.T) {
	bus := NewMachineBus()
	want := uint64(len(bus.memory))
	if got := bus.BankedVisibleCeiling(); got != want {
		t.Fatalf("default banked ceiling = 0x%X, want len(memory)=0x%X", got, want)
	}
	installBankedCeiling(t, bus, 8*1024*1024)
	if got := bus.BankedVisibleCeiling(); got != 8*1024*1024 {
		t.Fatalf("after SetSizing banked ceiling = 0x%X, want 0x%X", got, 8*1024*1024)
	}
}
