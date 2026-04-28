// main_boot_smoke_test.go - PLAN_MAX_RAM slice 10f TDD coverage.
//
// Pins resolveModeCaps + resolveActiveVisibleCeiling row by row, the
// per-mode boot ordering (ApplyProfileVisibleCeiling before SYSINFO
// registration), and the discovery-path "all paths agree" smokes for
// IE64 (8 GiB synthesised), EmuTOS (32 MiB), and EhBASIC.

package main

import "testing"

const eightGiB uint64 = 8 * 1024 * 1024 * 1024

// ===========================================================================
// resolveModeCaps row-by-row
// ===========================================================================

func TestResolveModeCaps_TableMatch(t *testing.T) {
	cases := []struct {
		name              string
		mode              runtimeMode
		autoTotal         uint64
		wantBusMem        uint64
		wantBackingMaxLen uint64
	}{
		{"ie64-8gib", modeIE64, eightGiB, lowMemWindowBytes, eightGiB},
		{"intuition-os-8gib", modeIntuitionOS, eightGiB, lowMemWindowBytes, eightGiB},
		{"basic-8gib", modeBasic, eightGiB, lowMemWindowBytes, eightGiB},
		{"ie32-8gib", modeIE32, eightGiB, busMemMaxBytes, busMemMaxBytes},
		{"x86-8gib", modeX86, eightGiB, busMemMaxBytes, busMemMaxBytes},
		{"bare-m68k-8gib", modeM68KBare, eightGiB, busMemMaxBytes, busMemMaxBytes},
		{"emutos-8gib", modeEmuTOS, eightGiB, EmuTOSProfileTopBytes, EmuTOSProfileTopBytes},
		{"aros-8gib", modeAros, eightGiB, arosProfileTopBytes, arosProfileTopBytes},
		{"6502-banked", mode6502, eightGiB, uint64(DEFAULT_MEMORY_SIZE), uint64(DEFAULT_MEMORY_SIZE)},
		{"z80-banked", modeZ80, eightGiB, uint64(DEFAULT_MEMORY_SIZE), uint64(DEFAULT_MEMORY_SIZE)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotBus, gotBacking := resolveModeCaps(tc.mode, tc.autoTotal)
			if gotBus != tc.wantBusMem {
				t.Errorf("busMemCap = %d, want %d", gotBus, tc.wantBusMem)
			}
			if gotBacking != tc.wantBackingMaxLen {
				t.Errorf("backingMaxSize = %d, want %d", gotBacking, tc.wantBackingMaxLen)
			}
		})
	}
}

func TestResolveActiveVisibleCeiling_TableMatch(t *testing.T) {
	cases := []struct {
		name     string
		mode     runtimeMode
		busTotal uint64
		wantCeil uint64
	}{
		{"ie64-host-scale", modeIE64, eightGiB, eightGiB},
		{"intuition-os-host-scale", modeIntuitionOS, eightGiB, eightGiB},
		{"basic-host-scale", modeBasic, eightGiB, eightGiB},
		{"ie32-clamps-to-4gib-page", modeIE32, eightGiB, busMemMaxBytes},
		{"x86-clamps-to-4gib-page", modeX86, eightGiB, busMemMaxBytes},
		{"bare-m68k-clamps", modeM68KBare, eightGiB, busMemMaxBytes},
		{"emutos-32mib", modeEmuTOS, eightGiB, EmuTOSProfileTopBytes},
		{"aros-profile-top", modeAros, eightGiB, arosProfileTopBytes},
		{"6502-banked", mode6502, eightGiB, uint64(DEFAULT_MEMORY_SIZE)},
		{"z80-banked", modeZ80, eightGiB, uint64(DEFAULT_MEMORY_SIZE)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveActiveVisibleCeiling(tc.mode, tc.busTotal)
			if got != tc.wantCeil {
				t.Errorf("ceiling = %d, want %d", got, tc.wantCeil)
			}
		})
	}
}

// ===========================================================================
// Per-mode boot ordering smokes (driven through bootGuestRAMFromComputed
// so we exercise the same code path main.go uses, but without launching
// the Ebiten game loop).
// ===========================================================================

func bootSimulate(t *testing.T, mode runtimeMode, autoTotal uint64, allocator func(size uint64) (Backing, error)) *MachineBus {
	t.Helper()
	busMemCap, backingMaxSize := resolveModeCaps(mode, autoTotal)
	memSize := autoTotal
	if busMemCap < memSize {
		memSize = busMemCap
	}
	if memSize > busMemMaxBytes {
		memSize = busMemMaxBytes
	}
	// PLAN_MAX_RAM slice 10 reviewer P3: mirror the production
	// busMemBootClamp so non-mmap platforms exercise the same clamp path
	// the appliance boot uses. Without this, the helper could ask for a
	// huge bus.memory on windows even though main.go would clamp it.
	if memSize > busMemBootClamp {
		memSize = busMemBootClamp
	}
	bus, err := NewMachineBusSized(memSize)
	if err != nil {
		t.Fatalf("NewMachineBusSized(%d): %v", memSize, err)
	}
	ms := MemorySizing{TotalGuestRAM: autoTotal}
	if _, err := bootGuestRAMFromComputed(bus, ms, backingMaxSize, allocator); err != nil {
		t.Fatalf("bootGuestRAMFromComputed: %v", err)
	}
	bus.ApplyProfileVisibleCeiling(resolveActiveVisibleCeiling(mode, bus.TotalGuestRAM()))
	RegisterSysInfoMMIOFromBus(bus)
	return bus
}

func TestBootMode_EmuTOS_AllocatesOnly32MiBBus_NoBacking(t *testing.T) {
	bus := bootSimulate(t, modeEmuTOS, eightGiB, sparseAllocator)
	if got := uint64(len(bus.GetMemory())); got != EmuTOSProfileTopBytes {
		t.Fatalf("len(bus.memory) = %d, want %d (EmuTOS profile cap)", got, EmuTOSProfileTopBytes)
	}
	if bus.Backing() != nil {
		t.Fatal("EmuTOS mode should not allocate a high-range backing")
	}
	if got := bus.TotalGuestRAM(); got != EmuTOSProfileTopBytes {
		t.Fatalf("TotalGuestRAM = %d, want %d", got, EmuTOSProfileTopBytes)
	}
}

func TestBootMode_6502_AllocatesOnly32MiBBus_NoBacking(t *testing.T) {
	bus := bootSimulate(t, mode6502, eightGiB, sparseAllocator)
	if got := uint64(len(bus.GetMemory())); got != uint64(DEFAULT_MEMORY_SIZE) {
		t.Fatalf("len(bus.memory) = %d, want %d (6502 banked ABI)", got, DEFAULT_MEMORY_SIZE)
	}
	if bus.Backing() != nil {
		t.Fatal("6502 banked mode should not allocate a high-range backing")
	}
}

func TestBootMode_Z80_AllocatesOnly32MiBBus_NoBacking(t *testing.T) {
	bus := bootSimulate(t, modeZ80, eightGiB, sparseAllocator)
	if got := uint64(len(bus.GetMemory())); got != uint64(DEFAULT_MEMORY_SIZE) {
		t.Fatalf("len(bus.memory) = %d, want %d (Z80 banked ABI)", got, DEFAULT_MEMORY_SIZE)
	}
	if bus.Backing() != nil {
		t.Fatal("Z80 banked mode should not allocate a high-range backing")
	}
}

func TestBootMode_AROS_AllocatesUpToProfileTop_NoBacking(t *testing.T) {
	bus := bootSimulate(t, modeAros, eightGiB, sparseAllocator)
	if got := uint64(len(bus.GetMemory())); got > arosProfileTopBytes {
		t.Fatalf("len(bus.memory) = %d, want <= %d", got, arosProfileTopBytes)
	}
	if bus.Backing() != nil {
		t.Fatal("AROS should not allocate a high-range backing")
	}
}

func TestBootMode_IE32_AllocatesUpTo4GiBPage_NoBacking(t *testing.T) {
	// PLAN_MAX_RAM slice 10 reviewer P2: use the mmap-faithful allocator
	// and expect min(busMemMaxBytes, busMemBootClamp) so test coverage
	// matches both linux/darwin (busMemBootClamp == busMemMaxBytes) and
	// non-mmap platforms (busMemBootClamp == 256 MiB) without silently
	// installing a backing the production NewMmapBacking would reject.
	bus := bootSimulate(t, modeIE32, eightGiB, mmapFaithfulSparseAllocator)
	want := busMemMaxBytes
	if want > busMemBootClamp {
		want = busMemBootClamp
	}
	if got := uint64(len(bus.GetMemory())); got != want {
		t.Fatalf("len(bus.memory) = %d, want %d (IE32 cap = min(busMemMaxBytes, busMemBootClamp))", got, want)
	}
	if bus.Backing() != nil {
		t.Fatal("IE32 should not allocate a high-range backing (Bus32 has no high-range path)")
	}
}

// TestBootMode_IE64_Above4GiBTotal_SmallLowWindow_BackingForFullTotal pins
// the slice 10 reviewer P1 fix: bus.memory stays at the small low-mem
// compatibility window (lowMemWindowBytes) regardless of total advertised
// RAM; the backing covers the full host-scale total. With sparse
// allocation we exercise the plumbing without committing real RSS.
func TestBootMode_IE64_Above4GiBTotal_SmallLowWindow_BackingForFullTotal(t *testing.T) {
	bus := bootSimulate(t, modeIE64, eightGiB, sparseAllocator)
	if got := uint64(len(bus.GetMemory())); got != lowMemWindowBytes {
		t.Fatalf("len(bus.memory) = %d, want %d (small low window, not full guest RAM)", got, lowMemWindowBytes)
	}
	if bus.Backing() == nil {
		t.Fatal("IE64 mode with 8 GiB total expected a high-range backing")
	}
	if got := bus.Backing().Size(); got != eightGiB {
		t.Fatalf("backing.Size = %d, want %d", got, eightGiB)
	}
	if got := bus.TotalGuestRAM(); got != eightGiB {
		t.Fatalf("TotalGuestRAM = %d, want %d", got, eightGiB)
	}
}

func TestBootMode_ActiveCeiling_IE64FullBackedTotal(t *testing.T) {
	bus := bootSimulate(t, modeIE64, eightGiB, sparseAllocator)
	if got, total := bus.ActiveVisibleRAM(), bus.TotalGuestRAM(); got != total {
		t.Fatalf("ActiveVisibleRAM = %d, want full backed total %d (IE64 family)", got, total)
	}
}

// ===========================================================================
// Discovery: all paths agree
// ===========================================================================

func sysinfoActiveRAM(bus *MachineBus) uint64 {
	lo := bus.Read32(SYSINFO_ACTIVE_RAM_LO)
	hi := bus.Read32(SYSINFO_ACTIVE_RAM_HI)
	return uint64(lo) | (uint64(hi) << 32)
}

func sysinfoTotalRAM(bus *MachineBus) uint64 {
	lo := bus.Read32(SYSINFO_TOTAL_RAM_LO)
	hi := bus.Read32(SYSINFO_TOTAL_RAM_HI)
	return uint64(lo) | (uint64(hi) << 32)
}

func TestDiscovery_IE64_AllPathsAgreeOnFullBackedTotal(t *testing.T) {
	bus := bootSimulate(t, modeIE64, eightGiB, sparseAllocator)

	if got := bus.TotalGuestRAM(); got != eightGiB {
		t.Errorf("bus.TotalGuestRAM = %d, want %d", got, eightGiB)
	}
	if got := bus.ActiveVisibleRAM(); got != eightGiB {
		t.Errorf("bus.ActiveVisibleRAM = %d, want %d", got, eightGiB)
	}
	if got := sysinfoTotalRAM(bus); got != eightGiB {
		t.Errorf("SYSINFO_TOTAL_RAM = %d, want %d", got, eightGiB)
	}
	if got := sysinfoActiveRAM(bus); got != eightGiB {
		t.Errorf("SYSINFO_ACTIVE_RAM = %d, want %d", got, eightGiB)
	}
	// MFCR CR_RAM_SIZE_BYTES via a tiny IE64 program
	cpu := NewCPU64(bus)
	copy(cpu.memory[PROG_START:], ie64Instr(OP_MFCR, 1, 0, 0, CR_RAM_SIZE_BYTES, 0, 0))
	copy(cpu.memory[PROG_START+8:], ie64Instr(OP_HALT64, 0, 0, 0, 0, 0, 0))
	cpu.PC = PROG_START
	cpu.running.Store(true)
	cpu.Execute()
	if got := cpu.regs[1]; got != eightGiB {
		t.Errorf("MFCR CR_RAM_SIZE_BYTES = %d, want %d", got, eightGiB)
	}
}

func TestDiscovery_EmuTOS_AllPathsAgreeAt32MiB(t *testing.T) {
	bus := bootSimulate(t, modeEmuTOS, eightGiB, sparseAllocator)
	if got := bus.TotalGuestRAM(); got != EmuTOSProfileTopBytes {
		t.Errorf("TotalGuestRAM = %d, want %d", got, EmuTOSProfileTopBytes)
	}
	if got := bus.ActiveVisibleRAM(); got != EmuTOSProfileTopBytes {
		t.Errorf("ActiveVisibleRAM = %d, want %d", got, EmuTOSProfileTopBytes)
	}
	if got := sysinfoTotalRAM(bus); got != EmuTOSProfileTopBytes {
		t.Errorf("SYSINFO_TOTAL_RAM = %d, want %d", got, EmuTOSProfileTopBytes)
	}
	if got := sysinfoActiveRAM(bus); got != EmuTOSProfileTopBytes {
		t.Errorf("SYSINFO_ACTIVE_RAM = %d, want %d", got, EmuTOSProfileTopBytes)
	}
}

func TestDiscovery_EhBASIC_AVAILMatchesActiveVisible(t *testing.T) {
	const oneGiB uint64 = 1024 * 1024 * 1024
	bus := bootSimulate(t, modeBasic, oneGiB, sparseAllocator)
	pb := EhBASICProfileBounds(bus)
	if pb.Err != nil {
		t.Fatalf("EhBASICProfileBounds: %v", pb.Err)
	}
	// EhBASIC's TopOfRAM is uint32, capped at ehbasicMaxTopOfRAM (4 GiB-page).
	// For a 1 GiB active visible the cap does not bite; TopOfRAM should
	// equal the active visible (page-aligned).
	want := uint32(bus.ActiveVisibleRAM()) &^ uint32(MMU_PAGE_SIZE-1)
	if pb.TopOfRAM != want {
		t.Errorf("EhBASIC TopOfRAM = 0x%X, want 0x%X (= active visible page-aligned)", pb.TopOfRAM, want)
	}
}
