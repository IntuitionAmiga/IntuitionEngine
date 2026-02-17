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
//   sjasmplus --raw=<name>.bin <name>.asm   (PT3, STC, SQT — community reference players)
//   vasmz80_std -Fbin -o <name>.bin <name>.asm  (generic player)

package main

import (
	_ "embed"
)

//go:embed sdk/players/pt3play.bin
var pt3PlayerBinary []byte

//go:embed sdk/players/stcplay.bin
var stcPlayerBinary []byte

//go:embed sdk/players/sqtplay.bin
var sqtPlayerBinary []byte

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
// Uses the PTx player which handles both PT2 and PT3 formats.
func pt2FormatConfig() trackerFormatConfig {
	return trackerFormatConfig{
		name:         "PT2",
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

// sqtFormatConfig returns the format configuration for SQ-Tracker modules.
func sqtFormatConfig() trackerFormatConfig {
	return trackerFormatConfig{
		name:         "SQT",
		playerBinary: sqtPlayerBinary,
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

// trackerFormatConfigByExt returns the appropriate Z80-based format config for a file extension.
// Note: PT1, ASC, FTC use native Go players and are NOT in this table.
func trackerFormatConfigByExt(ext string) (trackerFormatConfig, bool) {
	switch ext {
	case ".pt3":
		return pt3FormatConfig(), true
	case ".stc":
		return stcFormatConfig(), true
	case ".pt2":
		return pt2FormatConfig(), true
	case ".sqt":
		return sqtFormatConfig(), true
	default:
		return trackerFormatConfig{}, false
	}
}
