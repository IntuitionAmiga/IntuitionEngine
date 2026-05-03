// profile_bounds.go - Source-owned firmware/runtime profile contracts.
//
// PLAN_MAX_RAM.md slice 6 locks down explicit memory-map contracts for
// EmuTOS, AROS, and EhBASIC. The profiles are decoupled from the underlying
// CPU's architectural visible range: EmuTOS and AROS run on M68K (4 GiB
// addressable) but expose a much smaller profile-specific top-of-RAM by
// design, preserving the historical low-memory layout that GEMDOS, IOREC,
// the AROS audio DMA window, and direct VRAM placement all depend on.
//
// EhBASIC is an IE64 source/runtime profile and follows the bus-reported
// active visible RAM, capped at uint32 for low-memory accounting paths.
// Above-4-GiB IE64 visibility is queried separately via sysinfo MMIO and
// CR_RAM_SIZE_BYTES.

package main

import "fmt"

const (
	// EmuTOS_PROFILE_TOP is the explicit top-of-RAM the EmuTOS M68K profile
	// exposes. Independent of detected guest RAM and independent of the M68K
	// architectural 4 GiB visible range. Preserves the historical 32 MiB
	// low-memory layout that EmuTOS boot, GEMDOS bridge, IOREC, and ROM
	// validity checks all depend on. Any deliberate move requires source-
	// coordinated updates to emutos_loader.go, the EmuTOS source tree, and
	// the EmuTOS profile docs.
	EmuTOS_PROFILE_TOP uint32 = 32 * 1024 * 1024

	// AROS_PROFILE_TOP is the explicit top-of-RAM the AROS M68K profile
	// exposes. PLAN_MAX_RAM slice 10h raised this from 32 MiB to 2 GiB.
	// The direct VRAM window at 0x1E00000..0x2000000 (30 MiB) and the
	// AROS audio DMA fetch guard are well inside the new ceiling. 2 GiB
	// (= 0x80000000) still fits in uint32 — the value is the largest
	// page-aligned quantity that survives the M68K profile's uint32
	// representation. Any deliberate move requires source-coordinated
	// updates to aros_loader.go, aros_runtime.go, aros_audio_dma.go,
	// the AROS source tree, and the AROS profile docs.
	AROS_PROFILE_TOP uint32 = 2 * 1024 * 1024 * 1024

	// ehbasicMinRequiredRAM is the smallest active visible RAM that can
	// host the EhBASIC IE64 source layout. STACK_TOP sits at 0x9F000;
	// rounded up to the next page the layout requires 0xA0000 = 640 KiB.
	// Floor-clamped here so the profile error message is precise; in
	// practice MIN_GUEST_RAM (32 MiB) is far larger.
	ehbasicMinRequiredRAM uint32 = 0xA0000

	// ehbasicMaxTopOfRAM caps the uint32 TopOfRAM exposed for EhBASIC's
	// low-memory accounting paths. Above-4-GiB IE64 visibility is queried
	// via sysinfo MMIO; this constant keeps the low-memory profile bound
	// representable in a uint32 without truncating a larger active visible.
	ehbasicMaxTopOfRAM uint32 = 0xFFFFF000

	// arosMinRequiredRAM is the historical AROS runtime floor — 32 MiB.
	// AROS_PROFILE_TOP raised to 2 GiB in slice 10h is the maximum, not
	// the minimum: a 32 MiB test bus still satisfies the AROS profile
	// gate, and clampM68KProfileToBus pulls TopOfRAM down to the bus
	// size when smaller than the cap.
	arosMinRequiredRAM uint32 = 32 * 1024 * 1024
)

// ProfileBounds is the explicit memory-map contract a source-owned
// firmware/runtime profile imposes on the bus. Each profile constructor
// returns a populated ProfileBounds whose TopOfRAM is page-aligned, fits
// within the bus's reported active visible RAM, and whose Err is non-nil
// when the bus cannot satisfy the profile's minimum.
type ProfileBounds struct {
	Name        string
	TopOfRAM    uint32 // first byte past the highest profile-valid RAM address
	LowVecBase  uint32 // lowest RAM address the profile considers a valid vector
	ROMBase     uint32 // 0 if the profile has no ROM
	ROMEnd      uint32 // 0 if the profile has no ROM
	VRAMBase    uint32 // 0 if the profile has no direct VRAM contract
	VRAMEnd     uint32
	MinRequired uint32 // active visible RAM below this fails the profile
	Err         error  // non-nil if the bus sizing cannot satisfy the profile
}

// profileBoundsBus is the subset of *MachineBus that profile constructors
// consume. Stated as an interface so tests can wire fake sizing without
// constructing a full bus. ProfileMemoryCap returns the active visible RAM
// once sizing has been published, falling back to len(memory) so early-boot
// and lightweight test paths still get a usable bound.
type profileBoundsBus interface {
	ProfileMemoryCap() uint64
}

// EmuTOSProfileBounds returns the explicit M68K EmuTOS memory-map contract.
func EmuTOSProfileBounds(bus profileBoundsBus) ProfileBounds {
	pb := ProfileBounds{
		Name:        "EmuTOS",
		TopOfRAM:    EmuTOS_PROFILE_TOP,
		LowVecBase:  0x00001000,
		ROMBase:     emutosBaseStd,
		ROMEnd:      emutosBaseStd + emutosROM256K,
		MinRequired: EmuTOS_PROFILE_TOP,
	}
	return clampM68KProfileToBus(pb, bus)
}

// AROSProfileBounds returns the explicit M68K AROS memory-map contract.
//
// PLAN_MAX_RAM slice 10h: TopOfRAM was raised from 32 MiB to 2 GiB so AROS
// guests can address more RAM. MinRequired stays at the historical 32 MiB
// runtime minimum so legacy 32 MiB test rigs and embedded boots still pass
// the gate; clampM68KProfileToBus reduces TopOfRAM to the bus's active
// visible RAM when the bus is smaller than the profile cap.
func AROSProfileBounds(bus profileBoundsBus) ProfileBounds {
	pb := ProfileBounds{
		Name:        "AROS",
		TopOfRAM:    AROS_PROFILE_TOP,
		LowVecBase:  0x00001000,
		ROMBase:     arosROMBase,
		VRAMBase:    arosDirectVRAMBase,
		VRAMEnd:     arosDirectVRAMBase + arosDirectVRAMSize,
		MinRequired: arosMinRequiredRAM,
	}
	return clampM68KProfileToBus(pb, bus)
}

// EhBASICProfileBounds returns the IE64 EhBASIC profile contract. EhBASIC
// is IE64-aware so its TopOfRAM follows the bus active visible RAM. The
// uint32 cap ehbasicMaxTopOfRAM keeps the low-memory profile bound
// representable when active visible exceeds 4 GiB; full 64-bit RAM
// addressing remains available through sysinfo MMIO and CR_RAM_SIZE_BYTES.
func EhBASICProfileBounds(bus profileBoundsBus) ProfileBounds {
	pb := ProfileBounds{
		Name:        "EhBASIC",
		LowVecBase:  0x00001000,
		MinRequired: ehbasicMinRequiredRAM,
	}
	avr := bus.ProfileMemoryCap()
	if avr < uint64(pb.MinRequired) {
		pb.Err = fmt.Errorf("EhBASIC profile requires at least %d bytes of active visible RAM, got %d",
			pb.MinRequired, avr)
		return pb
	}
	if avr > uint64(ehbasicMaxTopOfRAM) {
		pb.TopOfRAM = ehbasicMaxTopOfRAM
	} else {
		pb.TopOfRAM = uint32(avr) &^ uint32(MMU_PAGE_SIZE-1)
	}
	return pb
}

// EnforceEhBASICProfile is the boot-time gate that the EhBASIC entry path
// in main.go consults before loading the embedded image. Returns nil when
// the active visible RAM satisfies the EhBASIC source layout, otherwise
// returns the profile error verbatim so the boot caller can surface a
// precise message.
func EnforceEhBASICProfile(bus profileBoundsBus) error {
	pb := EhBASICProfileBounds(bus)
	return pb.Err
}

// EhBASICLayoutFitsTopOfRAM reports whether the IE64 EhBASIC source layout
// (BASIC_LINE_BUF..STACK_TOP) fits inside top. Sanity-check helper for
// tests and assertions; the layout constants live in sdk/include/ie64.inc.
const ehbasicLayoutBase uint32 = 0x21000 // BASIC_LINE_BUF
const ehbasicLayoutTop uint32 = 0x9F000  // STACK_TOP (initial R31)

func EhBASICLayoutFitsTopOfRAM(top uint32) bool {
	return top >= ehbasicLayoutTop && ehbasicLayoutBase < top
}

func clampM68KProfileToBus(pb ProfileBounds, bus profileBoundsBus) ProfileBounds {
	avr := bus.ProfileMemoryCap()
	if avr < uint64(pb.MinRequired) {
		pb.Err = fmt.Errorf("%s profile requires at least %d bytes of active visible RAM, got %d",
			pb.Name, pb.MinRequired, avr)
		return pb
	}
	if uint64(pb.TopOfRAM) > avr {
		pb.TopOfRAM = uint32(avr) &^ uint32(MMU_PAGE_SIZE-1)
	}
	return pb
}
