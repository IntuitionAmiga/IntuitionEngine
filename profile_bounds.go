// profile_bounds.go - Source-owned firmware/runtime profile contracts.
//
// PLAN_MAX_RAM.md slice 6 locks down explicit memory-map contracts for
// EmuTOS, AROS, and EhBASIC. The profiles are decoupled from the underlying
// CPU's architectural visible range: EmuTOS and AROS run on M68K (4 GiB
// addressable) but expose profile-specific top-of-RAM values by design,
// preserving the stable low-memory layout that GEMDOS, IOREC, the AROS
// audio DMA window, and direct VRAM placement all depend on.
//
// EhBASIC is an IE64 source/runtime profile and follows the bus-reported
// active visible RAM, capped at uint32 for low-memory accounting paths.
// Above-4-GiB IE64 visibility is queried separately via sysinfo MMIO and
// CR_RAM_SIZE_BYTES.

package main

import "fmt"

const (
	// EmuTOS_PROFILE_TOP is the explicit top-of-RAM the EmuTOS M68K profile
	// exposes. Keep this at the historical 32 MiB profile cap: EmuTOS sizes
	// and initializes RAM during boot, so advertising the architectural 2 GiB
	// M68K ceiling makes a normal desktop boot spend a long time walking RAM
	// before it reaches GEM/VDI. Any deliberate move requires source-coordinated
	// updates to emutos_loader.go, the EmuTOS source tree, and the EmuTOS
	// profile docs.
	EmuTOS_PROFILE_TOP uint32 = 32 * 1024 * 1024

	// AROS_PROFILE_TOP is the explicit top-of-RAM the AROS M68K profile
	// exposes. PLAN_MAX_RAM slice 10h raised this from 32 MiB to 2 GiB.
	// The direct VRAM window at 0x1E00000..0x5E00000 and the
	// AROS audio DMA fetch guard are well inside the new ceiling. 2 GiB
	// (= 0x80000000) still fits in uint32 — the value is the largest
	// page-aligned quantity that survives the M68K profile's uint32
	// representation. Any deliberate move requires source-coordinated
	// updates to aros_loader.go, aros_runtime.go, aros_audio_dma.go,
	// the AROS source tree, and the AROS profile docs.
	AROS_PROFILE_TOP uint32 = uint32(m68kProfileTop2GiB)

	// ehbasicMinRequiredRAM is the IE64 BASIC dynamic-layout minimum. The
	// BASIC runtime derives stack/control reservations and AOT compiler arenas
	// from CR_RAM_SIZE_BYTES; the native RUN AOT, TRANSPILE and COMPILE paths
	// reserve multi-megabyte source and code buffers, so the historical 32 MiB
	// floor is no longer a valid profile.
	ehbasicMinRequiredRAM uint32 = 256 * 1024 * 1024

	// ehbasicMaxTopOfRAM caps the uint32 TopOfRAM exposed for EhBASIC's
	// low-memory accounting paths. Above-4-GiB IE64 visibility is queried
	// via sysinfo MMIO; this constant keeps the low-memory profile bound
	// representable in a uint32 without truncating a larger active visible.
	ehbasicMaxTopOfRAM uint32 = 0xFFFFF000

	// arosMinRequiredRAM backs the whole AROS direct VRAM contract:
	// 0x1E00000..0x5E00000. 1920x1080 RGBA32 requires 8,294,400 bytes
	// per screen-sized bitmap, so the old 2 MiB/32 MiB-floor contract
	// and single-frame 16 MiB pool are not sufficient for Workbench.
	arosMinRequiredRAM uint32 = arosDirectVRAMBase + arosDirectVRAMSize

	// emutosMinRequiredRAM is the historical EmuTOS runtime floor — 32 MiB.
	// EmuTOS_PROFILE_TOP is the maximum, not the minimum: a 32 MiB test bus
	// still satisfies the profile gate, and clampM68KProfileToBus pulls
	// TopOfRAM down to the bus size when smaller than the 2 GiB cap.
	emutosMinRequiredRAM uint32 = 32 * 1024 * 1024
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
		MinRequired: emutosMinRequiredRAM,
	}
	return clampM68KProfileToBus(pb, bus)
}

// AROSProfileBounds returns the explicit M68K AROS memory-map contract.
//
// PLAN_MAX_RAM slice 10h: TopOfRAM was raised from 32 MiB to 2 GiB so AROS
// guests can address more RAM. MinRequired is the end of the AROS direct
// VRAM window; clampM68KProfileToBus reduces TopOfRAM to the bus's active
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
		Name:        "IE64 BASIC",
		LowVecBase:  0x00001000,
		MinRequired: ehbasicMinRequiredRAM,
	}
	avr := bus.ProfileMemoryCap()
	if avr < uint64(pb.MinRequired) {
		pb.Err = fmt.Errorf("IE64 BASIC profile requires at least %d bytes of active visible RAM, got %d",
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

// EhBASICLayoutFitsTopOfRAM reports whether the fixed low BASIC state anchors
// fit inside top. General BASIC storage and stacks are dynamic.
const ehbasicLayoutBase uint32 = 0x42000 // BASIC_STATE
const ehbasicLayoutTop uint32 = 0x43000  // BASIC_STATE_END (half-open)

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
