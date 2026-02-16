// tracker_z80_players.go - Embedded Z80 player routines for tracker formats.
//
// Each tracker format (PT3, PT2, PT1, STC, SQT, ASC, FTC) needs a Z80 player
// routine that understands its specific module data layout. The player is loaded
// at 0xC000 in Z80 memory, with module data at 0x4000.
//
// Entry points:
//   INIT (0xC000): HL = module address, initializes player state
//   PLAY (0xC003): Called 50 times/sec, processes one tick and writes AY registers
//
// Player binaries are assembled from source files in sdk/players/ using:
//   vasmz80_std -Fbin -o <name>.bin <name>.asm

package main

import (
	_ "embed"
)

//go:embed sdk/players/pt3play.bin
var pt3PlayerBinary []byte

//go:embed sdk/players/stcplay.bin
var stcPlayerBinary []byte

//go:embed sdk/players/generic_play.bin
var genericPlayerBinary []byte

const (
	trackerPlayerBase = 0xC000
	trackerModuleBase = 0x4000
	trackerInitEntry  = 0xC000
	trackerPlayEntry  = 0xC003
	zxSpectrumClock   = 1773400 // AY clock in ZX Spectrum (Hz)
	z80CPUClock       = 3500000 // Z80 CPU clock in ZX Spectrum (Hz)
	trackerFrameRate  = 50      // PAL frame rate
)

// pt3FormatConfig returns the format configuration for ProTracker 3 modules.
func pt3FormatConfig() trackerFormatConfig {
	return trackerFormatConfig{
		name:         "PT3",
		playerBinary: pt3PlayerBinary,
		playerBase:   trackerPlayerBase,
		moduleBase:   trackerModuleBase,
		initEntry:    trackerInitEntry,
		playEntry:    trackerPlayEntry,
		system:       ayZXSystemSpectrum,
		clockHz:      zxSpectrumClock,
		z80ClockHz:   z80CPUClock,
		frameRate:    trackerFrameRate,
	}
}

// stcFormatConfig returns the format configuration for Sound Tracker Compiled modules.
func stcFormatConfig() trackerFormatConfig {
	return trackerFormatConfig{
		name:         "STC",
		playerBinary: stcPlayerBinary,
		playerBase:   trackerPlayerBase,
		moduleBase:   trackerModuleBase,
		initEntry:    trackerInitEntry,
		playEntry:    trackerPlayEntry,
		system:       ayZXSystemSpectrum,
		clockHz:      zxSpectrumClock,
		z80ClockHz:   z80CPUClock,
		frameRate:    trackerFrameRate,
	}
}

// pt2FormatConfig returns the format configuration for ProTracker 2 modules.
func pt2FormatConfig() trackerFormatConfig {
	return trackerFormatConfig{
		name:         "PT2",
		playerBinary: genericPlayerBinary,
		playerBase:   trackerPlayerBase,
		moduleBase:   trackerModuleBase,
		initEntry:    trackerInitEntry,
		playEntry:    trackerPlayEntry,
		system:       ayZXSystemSpectrum,
		clockHz:      zxSpectrumClock,
		z80ClockHz:   z80CPUClock,
		frameRate:    trackerFrameRate,
	}
}

// pt1FormatConfig returns the format configuration for ProTracker 1 modules.
func pt1FormatConfig() trackerFormatConfig {
	return trackerFormatConfig{
		name:         "PT1",
		playerBinary: genericPlayerBinary,
		playerBase:   trackerPlayerBase,
		moduleBase:   trackerModuleBase,
		initEntry:    trackerInitEntry,
		playEntry:    trackerPlayEntry,
		system:       ayZXSystemSpectrum,
		clockHz:      zxSpectrumClock,
		z80ClockHz:   z80CPUClock,
		frameRate:    trackerFrameRate,
	}
}

// sqtFormatConfig returns the format configuration for SQ-Tracker modules.
func sqtFormatConfig() trackerFormatConfig {
	return trackerFormatConfig{
		name:         "SQT",
		playerBinary: genericPlayerBinary,
		playerBase:   trackerPlayerBase,
		moduleBase:   trackerModuleBase,
		initEntry:    trackerInitEntry,
		playEntry:    trackerPlayEntry,
		system:       ayZXSystemSpectrum,
		clockHz:      zxSpectrumClock,
		z80ClockHz:   z80CPUClock,
		frameRate:    trackerFrameRate,
	}
}

// ascFormatConfig returns the format configuration for ASC Sound Master modules.
func ascFormatConfig() trackerFormatConfig {
	return trackerFormatConfig{
		name:         "ASC",
		playerBinary: genericPlayerBinary,
		playerBase:   trackerPlayerBase,
		moduleBase:   trackerModuleBase,
		initEntry:    trackerInitEntry,
		playEntry:    trackerPlayEntry,
		system:       ayZXSystemSpectrum,
		clockHz:      zxSpectrumClock,
		z80ClockHz:   z80CPUClock,
		frameRate:    trackerFrameRate,
	}
}

// ftcFormatConfig returns the format configuration for Fast Tracker (ZX) modules.
func ftcFormatConfig() trackerFormatConfig {
	return trackerFormatConfig{
		name:         "FTC",
		playerBinary: genericPlayerBinary,
		playerBase:   trackerPlayerBase,
		moduleBase:   trackerModuleBase,
		initEntry:    trackerInitEntry,
		playEntry:    trackerPlayEntry,
		system:       ayZXSystemSpectrum,
		clockHz:      zxSpectrumClock,
		z80ClockHz:   z80CPUClock,
		frameRate:    trackerFrameRate,
	}
}

// trackerFormatConfigByExt returns the appropriate format config for a file extension.
func trackerFormatConfigByExt(ext string) (trackerFormatConfig, bool) {
	switch ext {
	case ".pt3":
		return pt3FormatConfig(), true
	case ".stc":
		return stcFormatConfig(), true
	case ".pt2":
		return pt2FormatConfig(), true
	case ".pt1":
		return pt1FormatConfig(), true
	case ".sqt":
		return sqtFormatConfig(), true
	case ".asc":
		return ascFormatConfig(), true
	case ".ftc":
		return ftcFormatConfig(), true
	default:
		return trackerFormatConfig{}, false
	}
}
