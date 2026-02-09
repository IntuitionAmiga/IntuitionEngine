// registers.go - Centralized I/O register address map for Intuition Engine

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

/*
registers.go - Master I/O Register Address Map

This file provides a centralized reference for all memory-mapped I/O regions
in the Intuition Engine. Individual chip implementations define their own
detailed register constants in separate *_constants.go files.

MEMORY MAP OVERVIEW
===================

Address Range       Size    Device              Constants File
---------------------------------------------------------------------------
0x00000-0x9EFFF     636KB   Main RAM            -
0x9F000             4KB     Stack               cpu_ie32.go (STACK_START)
0xA0000-0xAFFFF     64KB    VGA VRAM Window     vga_constants.go
0xB8000-0xBFFFF     32KB    VGA Text Buffer     vga_constants.go

0xF0000-0xF0054     84B     Video Chip          video_chip.go
0xF0700-0xF07FF     256B    Terminal/Serial     registers.go
0xF0800-0xF0B3F     832B    Audio Chip          audio_chip.go
0xF0C00-0xF0C1C     28B     PSG (AY-3-8910)     psg_constants.go
0xF0D00-0xF0D1D     29B     POKEY               pokey_constants.go
0xF0E00-0xF0E2D     45B     SID (6581/8580)     sid_constants.go
0xF0F00-0xF0F5F     96B     TED (audio+video)   ted_constants.go, ted_video_constants.go
0xF1000-0xF13FF     1KB     VGA Registers       vga_constants.go
0xF2000-0xF200B     12B     ULA (ZX Spectrum)   ula_constants.go

0x100000-0x4FFFFF   4MB     Video RAM           video_chip.go (VRAM_START)

I/O REGION DETAILS
==================

Video Chip (0xF0000-0xF0054) - video_chip.go
  VIDEO_CTRL, VIDEO_MODE, VIDEO_STATUS
  COPPER_CTRL, COPPER_PTR, COPPER_PC, COPPER_STATUS
  BLT_* (Blitter registers)
  VIDEO_RASTER_* (Raster effect registers)

Audio Chip (0xF0800-0xF0B3F) - audio_chip.go
  0xF0800-0xF08FF: Global control (AUDIO_CTRL, ENV_SHAPE, FILTER_*)
  0xF0900-0xF093F: Square wave (SQUARE_*)
  0xF0940-0xF097F: Triangle wave (TRI_*)
  0xF0980-0xF09BF: Sine wave (SINE_*)
  0xF09C0-0xF09FF: Noise (NOISE_*)
  0xF0A00-0xF0A1F: Sync/Ring mod sources
  0xF0A20-0xF0A6F: Sawtooth wave (SAW_*)
  0xF0A80-0xF0B3F: Flexible 4-channel block (FLEX_CH*)

PSG - AY-3-8910/YM2149 (0xF0C00-0xF0C1C) - psg_constants.go
  PSG_BASE through PSG_END
  PSG_PLAY_* (Player registers)

POKEY (0xF0D00-0xF0D1D) - pokey_constants.go
  POKEY_AUDF1-4, POKEY_AUDC1-4, POKEY_AUDCTL
  SAP_PLAY_* (SAP player registers)

SID - 6581/8580 (0xF0E00-0xF0E2D) - sid_constants.go
  SID_V1_*, SID_V2_*, SID_V3_* (Voice registers)
  SID_FC_*, SID_RES_FILT, SID_MODE_VOL (Filter/volume)
  SID_PLAY_* (Player registers)

TED (0xF0F00-0xF0F5F) - ted_constants.go, ted_video_constants.go
  Audio: TED_FREQ1_*, TED_FREQ2_*, TED_SND_CTRL (0xF0F00-0xF0F05)
  Player: TED_PLAY_* (0xF0F10-0xF0F1F)
  Video: TED_V_* (0xF0F20-0xF0F5F) - 40x25 text mode, 121 colors

VGA (0xF1000-0xF13FF) - vga_constants.go
  VGA_MODE, VGA_STATUS, VGA_CTRL
  VGA_SEQ_*, VGA_CRTC_*, VGA_GC_*, VGA_ATTR_*
  VGA_DAC_*, VGA_PALETTE

ULA - ZX Spectrum (0xF2000-0xF200B) - ula_constants.go
  ULA_BORDER (border color, bits 0-2)
  ULA_CTRL (enable/disable)
  ULA_STATUS (vblank)
  VRAM at 0x4000 (6144 bitmap + 768 attribute bytes)

CPU-SPECIFIC I/O MAPPINGS
=========================

6502 (16-bit address space, directly mapped)
  See cpu_six5go2.go - addresses 0xF000-0xF0FF map to 0xF0000-0xF00FF
  VGA at 0xD700-0xD70A (see vga_constants.go C6502_VGA_*)
  ULA at 0xD800-0xD80F, VRAM at 0x4000 (see ula_constants.go)

Z80 (16-bit address with bank windows)
  See cpu_z80_runner.go - addresses 0xF000-0xF0FF map to 0xF0000-0xF00FF
  VGA via port I/O 0xA0-0xAA (see vga_constants.go Z80_VGA_PORT_*)
  ULA via port I/O 0xFE (authentic ZX Spectrum), VRAM at 0x4000

M68K (32-bit address space, direct access)
  Full 32-bit addressing to all I/O regions

IE32 (32-bit address space, direct access)
  Full 32-bit addressing to all I/O regions
  Timer registers at IO_BASE+0x04, IO_BASE+0x08
*/

package main

// =============================================================================
// I/O Region Base Addresses and Boundaries
// =============================================================================

const (
	// Main I/O region boundaries
	IO_REGION_BASE = 0xF0000 // Start of I/O mapped region
	IO_REGION_END  = 0xFFFFF // End of I/O mapped region

	// Video chip region
	VIDEO_REGION_BASE = 0xF0000
	VIDEO_REGION_END  = 0xF0057

	// Terminal/Serial region
	TERMINAL_REGION_BASE = 0xF0700
	// TERMINAL_REGION_END defined above as 0xF07FF

	// Audio chip region
	AUDIO_REGION_BASE = 0xF0800
	AUDIO_REGION_END  = 0xF0B3F

	// PSG region (AY-3-8910/YM2149)
	PSG_REGION_BASE = 0xF0C00
	PSG_REGION_END  = 0xF0C1C

	// POKEY region
	POKEY_REGION_BASE = 0xF0D00
	POKEY_REGION_END  = 0xF0D1D

	// SID region (6581/8580)
	SID_REGION_BASE = 0xF0E00
	SID_REGION_END  = 0xF0E2D

	// TED region (audio 0xF0F00-0xF0F1F, video 0xF0F20-0xF0F5F)
	TED_REGION_BASE = 0xF0F00
	TED_REGION_END  = 0xF0F5F

	// VGA region
	VGA_REGION_BASE = 0xF1000
	VGA_REGION_END  = 0xF13FF

	// ULA region (ZX Spectrum video)
	ULA_REGION_BASE = 0xF2000
	ULA_REGION_END  = 0xF200B
)

// =============================================================================
// VRAM and Special Memory Regions
// =============================================================================

const (
	// VGA legacy memory windows (PC-compatible addresses)
	VGA_VRAM_BASE = 0xA0000 // Mode 13h/12h framebuffer
	VGA_VRAM_END  = 0xAFFFF
	VGA_TEXT_BASE = 0xB8000 // Text mode buffer
	VGA_TEXT_END  = 0xBFFFF

	// Main VRAM (VideoChip high-resolution modes)
	MAIN_VRAM_BASE = 0x100000 // 1MB
	MAIN_VRAM_END  = 0x4FFFFF // 5MB - 1
	MAIN_VRAM_SIZE = 0x400000 // 4MB
)

// =============================================================================
// IE32 CPU-Specific I/O (defined in cpu_ie32.go, referenced here for clarity)
// =============================================================================

const (
	// Timer registers (relative to IO_BASE 0xF0800)
	IE32_TIMER_COUNT  = 0xF0804 // Timer current count
	IE32_TIMER_PERIOD = 0xF0808 // Timer period value
)

// =============================================================================
// Terminal Output (debug serial interface)
// =============================================================================

const (
	// Terminal output at 0xF0700 (in the gap between Video and Audio regions)
	TERMINAL_OUT        = 0xF0700    // 32-bit address
	TERM_OUT            = 0xF0700    // Alias for backwards compatibility
	TERM_OUT_16BIT      = 0xF700     // 16-bit form for Z80/6502 access
	TERM_OUT_SIGNEXT    = 0xFFFFF700 // Sign-extended form (M68K .W addressing)
	TERM_STATUS         = 0xF0704    // Bit 0: input available, Bit 1: output ready
	TERM_IN             = 0xF0708    // Read next input character (dequeues)
	TERM_LINE_STATUS    = 0xF070C    // Bit 0: complete line available
	TERM_ECHO           = 0xF0710    // Bit 0: local echo enable (default 1)
	TERM_SENTINEL       = 0xF07F0    // Write 0xDEAD to stop CPU (via OnSentinel callback)
	TERMINAL_REGION_END = 0xF07FF    // Reserve 256 bytes for future expansion
)

// =============================================================================
// Helper Functions
// =============================================================================

// IsIOAddress returns true if the address is in the I/O region
func IsIOAddress(addr uint32) bool {
	return addr >= IO_REGION_BASE && addr <= IO_REGION_END
}

// IsVRAMAddress returns true if the address is in main VRAM
func IsVRAMAddress(addr uint32) bool {
	return addr >= MAIN_VRAM_BASE && addr <= MAIN_VRAM_END
}

// IsVGAWindowAddress returns true if the address is in VGA legacy windows
func IsVGAWindowAddress(addr uint32) bool {
	return (addr >= VGA_VRAM_BASE && addr <= VGA_VRAM_END) ||
		(addr >= VGA_TEXT_BASE && addr <= VGA_TEXT_END)
}

// GetIORegion returns the device name for an I/O address
func GetIORegion(addr uint32) string {
	switch {
	case addr >= VIDEO_REGION_BASE && addr <= VIDEO_REGION_END:
		return "VideoChip"
	case addr >= TERMINAL_REGION_BASE && addr <= TERMINAL_REGION_END:
		return "Terminal"
	case addr >= AUDIO_REGION_BASE && addr <= AUDIO_REGION_END:
		return "AudioChip"
	case addr >= PSG_REGION_BASE && addr <= PSG_REGION_END:
		return "PSG"
	case addr >= POKEY_REGION_BASE && addr <= POKEY_REGION_END:
		return "POKEY"
	case addr >= SID_REGION_BASE && addr <= SID_REGION_END:
		return "SID"
	case addr >= TED_REGION_BASE && addr <= TED_REGION_END:
		return "TED"
	case addr >= VGA_REGION_BASE && addr <= VGA_REGION_END:
		return "VGA"
	case addr >= ULA_REGION_BASE && addr <= ULA_REGION_END:
		return "ULA"
	default:
		return "Unknown"
	}
}
