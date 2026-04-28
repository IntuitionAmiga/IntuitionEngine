// boot_driver.go - PLAN_MAX_RAM slice 10f mode-resolver helpers.
//
// determineRuntimeMode encodes the per-flag → mode mapping main.go uses
// to drive the slice-10 cap resolvers and the per-mode CPU constructor.
// It is split into a stand-alone helper (rather than left inline in main)
// so the boot-ordering integration tests can witness the per-mode
// dispatch without launching the full Ebiten game loop.

package main

// bootModeFlags is the subset of main.go's mode-flag variables that
// determineRuntimeMode consumes. Tests construct one directly; main.go
// builds it from the parsed flag locals.
type bootModeFlags struct {
	IE32   bool
	IE64   bool
	M68K   bool
	EmuTOS bool
	AROS   bool
	Basic  bool
	Z80    bool
	X86    bool
	M6502  bool
}

// determineRuntimeMode returns the runtimeMode driven by the boot flags.
// EhBASIC (-basic) is an IE64-family runtime that wants the IE64 cap
// table but with EhBASIC-specific profile bounds applied later via
// EnforceEhBASICProfile; for cap resolution it shares the IE64-family
// row (full backed total). Source-owned firmware profiles (EmuTOS, AROS)
// take precedence over generic CPU flags so callers like main's
// -emutos-image / -aros-image fast paths land in the right cap table
// row even when the corresponding CPU flag was set first.
func determineRuntimeMode(f bootModeFlags) runtimeMode {
	switch {
	case f.EmuTOS:
		return modeEmuTOS
	case f.AROS:
		return modeAros
	case f.Basic:
		return modeBasic
	case f.IE64:
		return modeIE64
	case f.IE32:
		return modeIE32
	case f.X86:
		return modeX86
	case f.M6502:
		return mode6502
	case f.Z80:
		return modeZ80
	case f.M68K:
		return modeM68KBare
	default:
		return modeIE64
	}
}
