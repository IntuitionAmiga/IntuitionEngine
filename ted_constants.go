// ted_constants.go - MOS 7360/8360 TED sound chip register addresses and constants
// See registers.go for the complete I/O memory map reference.

package main

// TED register addresses (memory-mapped at 0xF0F00-0xF0F05)
const (
	TED_BASE = 0xF0F00
	TED_END  = 0xF0F05

	// Sound registers (matching Plus/4 layout)
	TED_FREQ1_LO  = 0xF0F00 // Voice 1 frequency low byte ($FF0E on Plus/4)
	TED_FREQ2_LO  = 0xF0F01 // Voice 2 frequency low byte ($FF0F on Plus/4)
	TED_FREQ2_HI  = 0xF0F02 // Voice 2 frequency high (bits 0-1) ($FF10 on Plus/4)
	TED_SND_CTRL  = 0xF0F03 // Sound control register ($FF11 on Plus/4)
	TED_FREQ1_HI  = 0xF0F04 // Voice 1 frequency high (bits 0-1) ($FF12 on Plus/4)
	TED_PLUS_CTRL = 0xF0F05 // TED+ mode enable (0=standard, 1=enhanced)

	TED_REG_COUNT = 6
)

// TED register offsets (for array indexing)
const (
	TED_REG_FREQ1_LO  = 0 // Voice 1 frequency low byte
	TED_REG_FREQ2_LO  = 1 // Voice 2 frequency low byte
	TED_REG_FREQ2_HI  = 2 // Voice 2 frequency high (bits 0-1)
	TED_REG_SND_CTRL  = 3 // Sound control register
	TED_REG_FREQ1_HI  = 4 // Voice 1 frequency high (bits 0-1)
	TED_REG_PLUS_CTRL = 5 // TED+ mode control
)

// TED clock frequencies
const (
	TED_CLOCK_PAL        = 886724                               // PAL Plus/4 clock (Hz)
	TED_CLOCK_NTSC       = 894886                               // NTSC Plus/4 clock (Hz)
	TED_SOUND_CLOCK_DIV  = 4                                    // Sound clock = main clock / 4
	TED_SOUND_CLOCK_PAL  = TED_CLOCK_PAL / TED_SOUND_CLOCK_DIV  // 221681 Hz
	TED_SOUND_CLOCK_NTSC = TED_CLOCK_NTSC / TED_SOUND_CLOCK_DIV // 223721 Hz
)

// TED control register bits ($FF11)
const (
	TED_CTRL_SNDDC     = 0x80 // Bit 7: D/A mode (direct DAC output)
	TED_CTRL_SND2NOISE = 0x40 // Bit 6: Voice 2 noise enable
	TED_CTRL_SND2ON    = 0x20 // Bit 5: Voice 2 enable
	TED_CTRL_SND1ON    = 0x10 // Bit 4: Voice 1 enable
	TED_CTRL_VOLUME    = 0x0F // Bits 0-3: Master volume (0-8, values above 8 = max)
)

// TED Player registers (memory-mapped at 0xF0F10-0xF0F1F)
// Used to load and play .ted files with embedded 6502 code
const (
	TED_PLAY_PTR    = 0xF0F10 // 32-bit pointer to TED data (little-endian)
	TED_PLAY_LEN    = 0xF0F14 // 32-bit length of TED data (little-endian)
	TED_PLAY_CTRL   = 0xF0F18 // Control: bit 0=start, bit 1=stop, bit 2=loop
	TED_PLAY_STATUS = 0xF0F1C // Status: bit 0=busy, bit 1=error
)

// Z80 port mapping for TED access
const (
	Z80_TED_PORT_SELECT = 0xF2
	Z80_TED_PORT_DATA   = 0xF3
)

// 6502 memory mapping for TED
// Plus/4 TED sound registers are at $FF0E-$FF12, but we map to $D600 for consistency
const (
	C6502_TED_BASE = 0xD600
	C6502_TED_END  = 0xD605
)

// Plus/4 original addresses (for reference and .ted file playback)
const (
	PLUS4_TED_FREQ1_LO = 0xFF0E // Voice 1 frequency low
	PLUS4_TED_FREQ2_LO = 0xFF0F // Voice 2 frequency low
	PLUS4_TED_FREQ2_HI = 0xFF10 // Voice 2 frequency high (bits 0-1)
	PLUS4_TED_SND_CTRL = 0xFF11 // Sound control
	PLUS4_TED_FREQ1_HI = 0xFF12 // Voice 1 frequency high (bits 0-1)
)

// TED max volume (0-8, with 8 being maximum)
const TED_MAX_VOLUME = 8

// TMF format detection constants
const (
	TMF_SIGNATURE_OFFSET = 17   // TEDMUSIC signature at file offset 17 for TMF format
	TMF_MAX_BASIC_LINE   = 4096 // BASIC line number < 4096 indicates TMF format
)

// TEDMUSIC header offsets (relative to signature start)
const (
	TED_HDR_INIT_LO  = 9  // Init offset low byte
	TED_HDR_INIT_HI  = 10 // Init offset high byte
	TED_HDR_PLAY_LO  = 11 // Play address low byte
	TED_HDR_PLAY_HI  = 12 // Play address high byte
	TED_HDR_END_LO   = 13 // End address low byte
	TED_HDR_END_HI   = 14 // End address high byte
	TED_HDR_RESERVED = 15 // Reserved bytes (2 bytes)
	TED_HDR_SUBTUNES = 17 // Subtune count (2 bytes, little-endian)
	TED_HDR_FLAGS    = 19 // FileFlags byte (at offset $27 from header start)
	TED_HDR_STRINGS  = 48 // Metadata strings start offset
)

// TEDMUSIC header string sizes
const (
	TED_STRING_SIZE = 32 // Size of each metadata string field
)
