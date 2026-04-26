// sysinfo_mmio_test.go - Tests for the low-MMIO RAM-size discovery block.
//
// PLAN_MAX_RAM.md slice 2 (RED phase).

package main

import (
	"testing"
)

func TestSysInfo_ConstantsAreDistinctAndAligned(t *testing.T) {
	if SYSINFO_REGION_END < SYSINFO_REGION_BASE {
		t.Fatalf("region end %#x < base %#x", SYSINFO_REGION_END, SYSINFO_REGION_BASE)
	}
	regs := []struct {
		name string
		addr uint32
	}{
		{"SYSINFO_TOTAL_RAM_LO", SYSINFO_TOTAL_RAM_LO},
		{"SYSINFO_TOTAL_RAM_HI", SYSINFO_TOTAL_RAM_HI},
		{"SYSINFO_ACTIVE_RAM_LO", SYSINFO_ACTIVE_RAM_LO},
		{"SYSINFO_ACTIVE_RAM_HI", SYSINFO_ACTIVE_RAM_HI},
	}
	for _, r := range regs {
		if r.addr%4 != 0 {
			t.Errorf("%s = %#x not 32-bit aligned", r.name, r.addr)
		}
		if r.addr < SYSINFO_REGION_BASE || r.addr+3 > SYSINFO_REGION_END {
			t.Errorf("%s = %#x outside region [%#x,%#x]", r.name, r.addr,
				SYSINFO_REGION_BASE, SYSINFO_REGION_END)
		}
	}
	// Total/active pairs must sit at consecutive 32-bit slots.
	if SYSINFO_TOTAL_RAM_HI != SYSINFO_TOTAL_RAM_LO+4 {
		t.Errorf("TOTAL_HI must follow TOTAL_LO by 4: lo=%#x hi=%#x",
			SYSINFO_TOTAL_RAM_LO, SYSINFO_TOTAL_RAM_HI)
	}
	if SYSINFO_ACTIVE_RAM_HI != SYSINFO_ACTIVE_RAM_LO+4 {
		t.Errorf("ACTIVE_HI must follow ACTIVE_LO by 4: lo=%#x hi=%#x",
			SYSINFO_ACTIVE_RAM_LO, SYSINFO_ACTIVE_RAM_HI)
	}
	// Active pair must immediately follow total pair (per plan).
	if SYSINFO_ACTIVE_RAM_LO != SYSINFO_TOTAL_RAM_HI+4 {
		t.Errorf("ACTIVE_LO must follow TOTAL_HI by 4: total_hi=%#x active_lo=%#x",
			SYSINFO_TOTAL_RAM_HI, SYSINFO_ACTIVE_RAM_LO)
	}
}

func TestSysInfo_RoundTrip_TotalAbove4GiB(t *testing.T) {
	bus := NewMachineBus()
	const total uint64 = 8 * bGiB
	const active uint64 = 4*bGiB - uint64(MMU_PAGE_SIZE)
	RegisterSysInfoMMIO(bus, total, active)

	gotLo := bus.Read32(SYSINFO_TOTAL_RAM_LO)
	gotHi := bus.Read32(SYSINFO_TOTAL_RAM_HI)
	got := uint64(gotHi)<<32 | uint64(gotLo)
	if got != total {
		t.Fatalf("total = %#x, want %#x (lo=%#x hi=%#x)", got, total, gotLo, gotHi)
	}
	if gotHi == 0 {
		t.Fatalf("TOTAL_HI = 0; high word truncated for %d-byte total", total)
	}
}

func TestSysInfo_RoundTrip_ActiveBelowCeiling(t *testing.T) {
	bus := NewMachineBus()
	const total uint64 = 8 * bGiB
	const active uint64 = 4*bGiB - uint64(MMU_PAGE_SIZE)
	RegisterSysInfoMMIO(bus, total, active)

	gotLo := bus.Read32(SYSINFO_ACTIVE_RAM_LO)
	gotHi := bus.Read32(SYSINFO_ACTIVE_RAM_HI)
	got := uint64(gotHi)<<32 | uint64(gotLo)
	if got != active {
		t.Fatalf("active = %#x, want %#x", got, active)
	}
}

func TestSysInfo_RoundTrip_ActiveAlsoAbove4GiB(t *testing.T) {
	bus := NewMachineBus()
	const total uint64 = 16 * bGiB
	const active uint64 = 12 * bGiB
	RegisterSysInfoMMIO(bus, total, active)

	gotALo := bus.Read32(SYSINFO_ACTIVE_RAM_LO)
	gotAHi := bus.Read32(SYSINFO_ACTIVE_RAM_HI)
	gotA := uint64(gotAHi)<<32 | uint64(gotALo)
	if gotA != active {
		t.Fatalf("active = %#x, want %#x", gotA, active)
	}
	if gotAHi == 0 {
		t.Fatalf("ACTIVE_HI = 0; high word truncated for %d-byte active", active)
	}
}

func TestSysInfo_StableAcrossRepeatedReads(t *testing.T) {
	bus := NewMachineBus()
	const total uint64 = 4 * bGiB
	const active uint64 = 2 * bGiB
	RegisterSysInfoMMIO(bus, total, active)

	for i := 0; i < 8; i++ {
		if got := bus.Read32(SYSINFO_TOTAL_RAM_LO); got != uint32(total&0xFFFFFFFF) {
			t.Fatalf("iter %d TOTAL_LO = %#x, want %#x", i, got, uint32(total&0xFFFFFFFF))
		}
		if got := bus.Read32(SYSINFO_TOTAL_RAM_HI); got != uint32(total>>32) {
			t.Fatalf("iter %d TOTAL_HI = %#x, want %#x", i, got, uint32(total>>32))
		}
	}
}

func TestSysInfo_WritesAreIgnored(t *testing.T) {
	bus := NewMachineBus()
	const total uint64 = 8 * bGiB
	const active uint64 = 2 * bGiB
	RegisterSysInfoMMIO(bus, total, active)

	bus.Write32(SYSINFO_TOTAL_RAM_LO, 0xDEADBEEF)
	bus.Write32(SYSINFO_TOTAL_RAM_HI, 0xDEADBEEF)
	bus.Write32(SYSINFO_ACTIVE_RAM_LO, 0xDEADBEEF)
	bus.Write32(SYSINFO_ACTIVE_RAM_HI, 0xDEADBEEF)

	gotLo := bus.Read32(SYSINFO_TOTAL_RAM_LO)
	gotHi := bus.Read32(SYSINFO_TOTAL_RAM_HI)
	got := uint64(gotHi)<<32 | uint64(gotLo)
	if got != total {
		t.Fatalf("after write: total=%#x, want %#x", got, total)
	}

	gotLo = bus.Read32(SYSINFO_ACTIVE_RAM_LO)
	gotHi = bus.Read32(SYSINFO_ACTIVE_RAM_HI)
	got = uint64(gotHi)<<32 | uint64(gotLo)
	if got != active {
		t.Fatalf("after write: active=%#x, want %#x", got, active)
	}
}

func TestSysInfo_GetIORegionLabelsBlock(t *testing.T) {
	if got := GetIORegion(SYSINFO_TOTAL_RAM_LO); got != "SysInfo" {
		t.Fatalf("GetIORegion(TOTAL_LO) = %q, want \"SysInfo\"", got)
	}
	if got := GetIORegion(SYSINFO_ACTIVE_RAM_HI); got != "SysInfo" {
		t.Fatalf("GetIORegion(ACTIVE_HI) = %q, want \"SysInfo\"", got)
	}
}

// Conflict registry: SysInfo MMIO must not overlap any documented production
// MMIO region. The list mirrors the registers.go memory map.
func TestSysInfo_NoOverlapWithDocumentedMMIO(t *testing.T) {
	regions := []struct {
		name       string
		start, end uint32
	}{
		{"VideoChip", VIDEO_REGION_BASE, VIDEO_REGION_END},
		{"Terminal", TERMINAL_REGION_BASE, TERMINAL_REGION_END},
		{"AudioChip", AUDIO_REGION_BASE, AUDIO_REGION_END},
		{"PSG", PSG_REGION_BASE, PSG_REGION_END},
		{"POKEY", POKEY_REGION_BASE, POKEY_REGION_END},
		{"SID", SID_REGION_BASE, SID_REGION_END},
		{"TED", TED_REGION_BASE, TED_REGION_END},
		{"VGA", VGA_REGION_BASE, VGA_REGION_END},
		{"ULA", ULA_REGION_BASE, ULA_REGION_END},
		{"FileIO", FILE_IO_REGION_BASE, FILE_IO_REGION_END},
		{"AROSDOSHandler", AROS_DOS_REGION_BASE, AROS_DOS_REGION_END},
		{"AROSAudioDMA", AROS_AUD_REGION_BASE_REG, AROS_AUD_REGION_END_REG},
		{"MediaLoader", MEDIA_LOADER_REGION_BASE, MEDIA_LOADER_REGION_END},
		{"ProgramExecutor", EXEC_REGION_BASE, EXEC_REGION_END},
		{"ANTIC", ANTIC_REGION_BASE, ANTIC_REGION_END},
		{"GTIA", GTIA_REGION_BASE, GTIA_REGION_END},
		{"Coprocessor", COPROC_REGION_BASE, COPROC_REGION_END},
		{"ClipboardBridge", CLIP_BRIDGE_REGION_BASE, CLIP_BRIDGE_REGION_END},
		{"CoprocessorExt", COPROC_EXT_REGION_BASE, COPROC_EXT_REGION_END},
		{"IRQDiag", IRQ_DIAG_REGION_BASE, IRQ_DIAG_REGION_END},
		{"BootstrapHostFS", BOOT_HOSTFS_BASE, BOOT_HOSTFS_BASE + 0x1F},
		{"Voodoo", VOODOO_REGION_BASE, VOODOO_REGION_END},
		{"VGA_VRAM", VGA_VRAM_BASE, VGA_VRAM_END},
		{"VGA_TEXT", VGA_TEXT_BASE, VGA_TEXT_END},
	}
	overlap := func(a0, a1, b0, b1 uint32) bool {
		return a0 <= b1 && b0 <= a1
	}
	for _, r := range regions {
		if overlap(SYSINFO_REGION_BASE, SYSINFO_REGION_END, r.start, r.end) {
			t.Errorf("SysInfo region [%#x,%#x] overlaps %s [%#x,%#x]",
				SYSINFO_REGION_BASE, SYSINFO_REGION_END, r.name, r.start, r.end)
		}
	}
	if SYSINFO_REGION_BASE < IO_REGION_BASE || SYSINFO_REGION_END > IO_REGION_END {
		t.Errorf("SysInfo region [%#x,%#x] outside IO region [%#x,%#x]",
			SYSINFO_REGION_BASE, SYSINFO_REGION_END, IO_REGION_BASE, IO_REGION_END)
	}
}
