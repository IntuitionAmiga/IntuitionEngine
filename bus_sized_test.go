// bus_sized_test.go - PLAN_MAX_RAM slice 10c TDD coverage.
//
// Pins NewMachineBusSized's input validation and the proportional ioPage-
// Bitmap sizing. Banked CPU tests pin 32 MiB ABI rejection even when the
// underlying bus.memory has been grown (the banked-widening slice is a
// future deliverable; until then the banked CPUs must respect the
// published ActiveVisibleRAM/BankedVisibleCeiling = DEFAULT_MEMORY_SIZE).

package main

import (
	"errors"
	"testing"
)

// ===========================================================================
// Validation tests — internal allocator avoids real heap commit
// ===========================================================================

func TestNewMachineBusSized_RejectsZeroSize(t *testing.T) {
	_, err := NewMachineBusSized(0)
	if !errors.Is(err, ErrInvalidSizeArg) {
		t.Fatalf("NewMachineBusSized(0) err=%v, want ErrInvalidSizeArg", err)
	}
}

func TestNewMachineBusSized_RejectsUnalignedSize(t *testing.T) {
	_, err := NewMachineBusSized(uint64(MMU_PAGE_SIZE) + 1)
	if !errors.Is(err, ErrInvalidSizeArg) {
		t.Fatalf("NewMachineBusSized(unaligned) err=%v, want ErrInvalidSizeArg", err)
	}
}

func TestNewMachineBusSized_RejectsAbove4GiBMinusPage(t *testing.T) {
	// 0x100000000 (4 GiB) is far above the cap.
	_, err := NewMachineBusSized(0x100000000)
	if !errors.Is(err, ErrInvalidSizeArg) {
		t.Fatalf("NewMachineBusSized(4GiB) err=%v, want ErrInvalidSizeArg", err)
	}
}

// TestNewMachineBusSized_RejectsInsideSignExtAliasZone pins the reviewer
// P2 fix: the M68K sign-extended low-16-bit alias kicks in at 0xFFFF0000
// for every bus access, so any advertised RAM in [0xFFFF0000, 0x100000000)
// would be silently aliased to low memory. The cap must reject any
// memSize that lands inside the alias zone.
func TestNewMachineBusSized_RejectsInsideSignExtAliasZone(t *testing.T) {
	// 0xFFFFF000 (the old cap) lands inside the alias zone.
	_, err := NewMachineBusSized(0xFFFFF000)
	if !errors.Is(err, ErrInvalidSizeArg) {
		t.Fatalf("NewMachineBusSized(0xFFFFF000) err=%v, want ErrInvalidSizeArg", err)
	}
}

// TestBusMemMaxBytes_BelowSignExtAliasZone is a compile-time guard pinning
// that the cap stays below the alias zone start. Any future cap raise
// must not reintroduce the silent-aliasing regression reviewer P2 fixed.
func TestBusMemMaxBytes_BelowSignExtAliasZone(t *testing.T) {
	const aliasZoneStart uint64 = 0xFFFF0000
	if busMemMaxBytes > aliasZoneStart {
		t.Fatalf("busMemMaxBytes=%#x exceeds sign-ext alias zone start %#x", busMemMaxBytes, aliasZoneStart)
	}
}

func TestNewMachineBusSized_ValidationAcceptsAtBusMemMaxBytes(t *testing.T) {
	// Use the internal allocator hook to exercise validation at the cap
	// without committing ~4 GiB of heap. Allocator stub records that it
	// was invoked with the requested size and returns a sentinel slice.
	var requested uint64
	stub := func(size uint64) []byte {
		requested = size
		return make([]byte, MMU_PAGE_SIZE)
	}
	_, err := newMachineBusSizedWithAllocator(busMemMaxBytes, stub)
	if err != nil {
		t.Fatalf("validation rejected size at cap: %v", err)
	}
	if requested != busMemMaxBytes {
		t.Fatalf("allocator requested=%d, want %d", requested, busMemMaxBytes)
	}
}

// ===========================================================================
// Allocation tests — small real sizes only
// ===========================================================================

func TestNewMachineBusSized_AllocatesRequestedMemory_256MiB(t *testing.T) {
	const want = 256 * 1024 * 1024
	bus, err := NewMachineBusSized(want)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	if got := len(bus.GetMemory()); got != want {
		t.Fatalf("len(memory)=%d, want %d", got, want)
	}
	if got := len(bus.ioPageBitmap); got != want/int(PAGE_SIZE) {
		t.Fatalf("len(ioPageBitmap)=%d, want %d (proportional)", got, want/int(PAGE_SIZE))
	}
}

func TestNewMachineBus_DefaultsTo32MiB(t *testing.T) {
	bus := NewMachineBus()
	if got := len(bus.GetMemory()); got != DEFAULT_MEMORY_SIZE {
		t.Fatalf("legacy NewMachineBus len(memory)=%d, want %d", got, DEFAULT_MEMORY_SIZE)
	}
}

func TestBus_ReadAt64MiB_HitsGrownMemory(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	const addr uint32 = 64 * 1024 * 1024
	bus.Write32(addr, 0xCAFEBABE)
	if got := bus.Read32(addr); got != 0xCAFEBABE {
		t.Fatalf("Read32(64MiB)=0x%X, want 0xCAFEBABE", got)
	}
}

func TestIoPageBitmap_LenMatchesMemoryLen(t *testing.T) {
	bus, err := NewMachineBusSized(128 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	wantLen := len(bus.GetMemory()) / int(PAGE_SIZE)
	if got := len(bus.ioPageBitmap); got != wantLen {
		t.Fatalf("len(ioPageBitmap)=%d, want %d (proportional invariant after growth)",
			got, wantLen)
	}
}

// TestBus_GrownMemory_TEDVRAMMMIOStillRoutesToHandler pins that low MMIO
// windows (TED VRAM at 0x100000) still hit the I/O dispatch path even after
// bus.memory is grown — the bitmap-proportional sizing must not silently
// shadow MMIO with normal RAM.
func TestBus_GrownMemory_TEDVRAMMMIOStillRoutesToHandler(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	ted := NewTEDVideoEngine(bus)
	bus.MapIO(TED_V_VRAM_BASE, TED_V_VRAM_BASE+TED_V_VRAM_SIZE-1, ted.HandleBusVRAMRead, ted.HandleBusVRAMWrite)

	const offset uint16 = 0x10
	bus.Write32(TED_V_VRAM_BASE+uint32(offset), 0x5A)
	if got := ted.HandleVRAMRead(offset); got != 0x5A {
		t.Fatalf("ted.HandleVRAMRead(0x10)=0x%X, want 0x5A (MMIO routed)", got)
	}
}

// ===========================================================================
// Banked CPU tests — pin 32 MiB rejection after bus.memory growth
// ===========================================================================

func TestBanked_6502_RemainsAt32MiBAfterBusGrowth(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	// Publish a TotalGuestRAM so ApplyProfileVisibleCeiling has something
	// to clamp against; otherwise BankedVisibleCeiling falls through to
	// len(bus.memory) (= 256 MiB), the regression we are pinning against.
	bus.SetSizing(MemorySizing{TotalGuestRAM: 256 * 1024 * 1024})
	bus.ApplyProfileVisibleCeiling(uint64(DEFAULT_MEMORY_SIZE))
	if got := bus.BankedVisibleCeiling(); got != uint64(DEFAULT_MEMORY_SIZE) {
		t.Fatalf("BankedVisibleCeiling=%d, want %d (32 MiB banked ABI pin)", got, DEFAULT_MEMORY_SIZE)
	}
}

func TestBanked_Z80_RemainsAt32MiBAfterBusGrowth(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	bus.SetSizing(MemorySizing{TotalGuestRAM: 256 * 1024 * 1024})
	bus.ApplyProfileVisibleCeiling(uint64(DEFAULT_MEMORY_SIZE))
	if got := bus.BankedVisibleCeiling(); got != uint64(DEFAULT_MEMORY_SIZE) {
		t.Fatalf("BankedVisibleCeiling=%d, want %d (Z80 32 MiB banked ABI pin)", got, DEFAULT_MEMORY_SIZE)
	}
}

// ===========================================================================
// Banked translation tests — pin actual translation, not just the getter
// ===========================================================================

func TestBanked_6502_BankTranslation_RejectsAbove32MiB_AfterBusGrowth(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	bus.SetSizing(MemorySizing{TotalGuestRAM: 256 * 1024 * 1024})
	bus.ApplyProfileVisibleCeiling(uint64(DEFAULT_MEMORY_SIZE))

	adapter := NewBus6502Adapter(bus)
	// Configure Bank 1 to map to absolute base = 64 MiB. With BANK_WINDOW_SIZE
	// = 0x2000 (8 KiB), bank index = 64 MiB / 8 KiB = 0x2000.
	const bank1Idx = (64 * 1024 * 1024) / BANK_WINDOW_SIZE
	adapter.bank1 = bank1Idx
	adapter.bank1Enable = true
	if _, ok := adapter.translateExtendedBank(BANK1_WINDOW_BASE); ok {
		t.Fatalf("translateExtendedBank accepted address > 32 MiB ceiling (bus grown but ABI pinned)")
	}
}

func TestBanked_Z80_BankTranslation_RejectsAbove32MiB_AfterBusGrowth(t *testing.T) {
	bus, err := NewMachineBusSized(256 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	bus.SetSizing(MemorySizing{TotalGuestRAM: 256 * 1024 * 1024})
	bus.ApplyProfileVisibleCeiling(uint64(DEFAULT_MEMORY_SIZE))

	adapter := NewZ80BusAdapter(bus)
	const bank1Idx = (64 * 1024 * 1024) / Z80_BANK_WINDOW_SIZE
	adapter.bank1 = bank1Idx
	adapter.bank1Enable = true
	if _, ok := adapter.translateExtendedBank(Z80_BANK1_WINDOW_BASE); ok {
		t.Fatalf("Z80 translateExtendedBank accepted address > 32 MiB ceiling")
	}
}
