// boot_guest_ram.go - PLAN_MAX_RAM slice 10e production-side boot helper.
//
// `bootGuestRAMFromComputed` is the post-sizing wiring step that main.go
// (and tests) call after `ComputeMemorySizing` and `NewMachineBusSized`.
// It allocates a high-range Backing iff the autodetected total exceeds
// the legacy bus.memory window; capped modes (EmuTOS/AROS/6502/Z80/IE32/
// x86/bare-M68K) pass `backingMaxSize == busMemCap` so no backing is
// attempted for them.
//
// `resolveModeCaps` and `resolveActiveVisibleCeiling` encode the per-
// mode three-cap table (see PLAN_MAX_RAM slice 10 §A6).
//
// SYSINFO MMIO registration is intentionally NOT done here — it must be
// called by main.go AFTER `ApplyProfileVisibleCeiling` so the snapshot
// picks up the final active value, not the zero placeholder this helper
// publishes.

package main

import (
	"errors"
	"fmt"
)

// runtimeMode enumerates the per-mode boot families used by the cap
// resolvers. The numeric values are not part of any ABI; only the
// symbolic constants are referenced from main.go.
type runtimeMode int

const (
	modeIE64 runtimeMode = iota
	modeIntuitionOS
	modeBasic
	modeIE32
	modeX86
	modeM68KBare
	modeEmuTOS
	modeAros
	mode6502
	modeZ80
)

// EmuTOSProfileTopBytes is the upper bound EmuTOS guests see. EmuTOS is a
// source-owned firmware profile that the appliance pins at 32 MiB.
const EmuTOSProfileTopBytes uint64 = 32 * 1024 * 1024

// lowMemWindowBytes is the upper bound on len(bus.memory) for IE64-family
// modes. PLAN_MAX_RAM slice 10 reviewer P1: bus.memory is the legacy
// direct-slice low compatibility window only; it is NOT the guest RAM
// size authority. Advertised guest RAM above this window is served via
// the high-range Backing through Bus64Phys. Sized at 256 MiB so AROS-
// style profiles, VRAM, MMIO, and reasonable program/heap staging all
// fit, while keeping mmap-backed boot RSS small.
const lowMemWindowBytes uint64 = 256 * 1024 * 1024

// arosProfileTopBytes is set by profile_bounds.go's AROS_PROFILE_TOP at
// boot. The cap resolver below reads from this var so slice 10h's AROS
// 2 GiB raise can flip a single source of truth.
var arosProfileTopBytes uint64 = uint64(AROS_PROFILE_TOP)

// resolveModeCaps returns (busMemCap, backingMaxSize) for the given mode
// per the slice-10 §A6 three-cap table. busMemCap is the upper bound on
// len(bus.memory); backingMaxSize is the upper bound on a high-range
// Backing allocation (only IE64 family modes pass autodetectedTotal).
func resolveModeCaps(mode runtimeMode, autodetectedTotal uint64) (busMemCap, backingMaxSize uint64) {
	switch mode {
	case modeIE64, modeIntuitionOS, modeBasic:
		return lowMemWindowBytes, autodetectedTotal
	case modeIE32, modeX86, modeM68KBare:
		// 32-bit modes do not route through Bus64Phys, so bus.memory is
		// the only path to advertised RAM. Cap is the full 32-bit
		// addressable window (just below the sign-extension alias zone)
		// — mmap-backed allocators keep this lazy on Linux/darwin.
		return busMemMaxBytes, busMemMaxBytes
	case modeEmuTOS:
		return EmuTOSProfileTopBytes, EmuTOSProfileTopBytes
	case modeAros:
		return arosProfileTopBytes, arosProfileTopBytes
	case mode6502, modeZ80:
		return uint64(DEFAULT_MEMORY_SIZE), uint64(DEFAULT_MEMORY_SIZE)
	default:
		// Defensive default: behave as if IE64 family (small low window
		// + full backing).
		return lowMemWindowBytes, autodetectedTotal
	}
}

// resolveActiveVisibleCeiling returns the per-mode active-visible-RAM cap
// per the slice-10 §A6 three-cap table. For IE64-family modes, callers
// should pass `bus.TotalGuestRAM()` AFTER bootGuestRAMFromComputed has
// run so the ceiling reflects whatever total survived backing
// allocation.
func resolveActiveVisibleCeiling(mode runtimeMode, busTotalGuestRAM uint64) uint64 {
	switch mode {
	case modeIE64, modeIntuitionOS, modeBasic:
		return busTotalGuestRAM
	case modeIE32, modeX86, modeM68KBare:
		if busTotalGuestRAM > busMemMaxBytes {
			return busMemMaxBytes
		}
		return busTotalGuestRAM
	case modeEmuTOS:
		return EmuTOSProfileTopBytes
	case modeAros:
		return arosProfileTopBytes
	case mode6502, modeZ80:
		return uint64(DEFAULT_MEMORY_SIZE)
	default:
		return busTotalGuestRAM
	}
}

// bootGuestRAMFromComputed binds a high-range Backing onto bus when the
// requested total exceeds bus.memory and publishes the post-allocation
// MemorySizing on bus. Active and ceiling are intentionally zeroed at
// this point — the caller MUST call bus.ApplyProfileVisibleCeiling
// before SYSINFO registration so guest discovery paths see the final
// values.
//
// The allocator is injected so tests can substitute SparseBacking (cheap)
// or a stub that always errors. Production callers pass NewMmapBacking.
//
// Returns the published MemorySizing on success, or an error if backing
// allocation failed for a reason other than ErrHighRangeBacking-
// Unsupported (which is treated as a soft fallback).
func bootGuestRAMFromComputed(
	bus *MachineBus,
	ms MemorySizing,
	backingMaxSize uint64,
	allocator func(size uint64) (Backing, error),
) (MemorySizing, error) {
	published := ms

	if backingMaxSize > uint64(len(bus.memory)) {
		backing, finalSize, err := AllocateBacking(backingMaxSize, allocator)
		switch {
		case errors.Is(err, ErrHighRangeBackingUnsupported):
			// Soft fallback: platform does not support mmap-backed high-
			// range RAM. Clamp the published total to bus.memory and
			// proceed without a backing.
			published.TotalGuestRAM = uint64(len(bus.memory))
		case err != nil:
			return MemorySizing{}, fmt.Errorf("backing allocation failed: %w", err)
		case finalSize <= uint64(len(bus.memory)):
			// Retry-shrink coherence: the halve-and-retry policy landed
			// on a size that no longer exceeds the bus.memory window.
			// Drop the (now redundant) backing to avoid leaking the
			// mmap region and clamp to the bus window.
			_ = backing.Close()
			published.TotalGuestRAM = uint64(len(bus.memory))
		default:
			bus.SetBacking(backing)
			published.TotalGuestRAM = finalSize
		}
	} else {
		// No backing required. Capped modes (EmuTOS/AROS/banked) advertise
		// the smaller of backingMaxSize and len(bus.memory).
		busMem := uint64(len(bus.memory))
		if backingMaxSize < busMem {
			published.TotalGuestRAM = backingMaxSize
		} else {
			published.TotalGuestRAM = busMem
		}
	}

	published.ActiveVisibleRAM = 0
	published.VisibleCeiling = 0
	bus.SetSizing(published)
	return published, nil
}
