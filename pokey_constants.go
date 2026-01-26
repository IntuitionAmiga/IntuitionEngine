// pokey_constants.go - POKEY sound chip register addresses and constants
// See registers.go for the complete I/O memory map reference.

package main

// POKEY register addresses (memory-mapped at 0xF0D00-0xF0D09)
const (
	POKEY_BASE      = 0xF0D00
	POKEY_AUDF1     = 0xF0D00 // Channel 1 frequency divider
	POKEY_AUDC1     = 0xF0D01 // Channel 1 control (distortion + volume)
	POKEY_AUDF2     = 0xF0D02 // Channel 2 frequency divider
	POKEY_AUDC2     = 0xF0D03 // Channel 2 control
	POKEY_AUDF3     = 0xF0D04 // Channel 3 frequency divider
	POKEY_AUDC3     = 0xF0D05 // Channel 3 control
	POKEY_AUDF4     = 0xF0D06 // Channel 4 frequency divider
	POKEY_AUDC4     = 0xF0D07 // Channel 4 control
	POKEY_AUDCTL    = 0xF0D08 // Master audio control
	POKEY_PLUS_CTRL = 0xF0D09 // POKEY+ mode enable (0=standard, 1=enhanced)
	POKEY_END       = 0xF0D09

	POKEY_REG_COUNT = 10
)

// POKEY clock frequencies
const (
	POKEY_CLOCK_NTSC = 1789773 // NTSC Atari 800/XL/XE clock (Hz)
	POKEY_CLOCK_PAL  = 1773447 // PAL Atari 800/XL/XE clock (Hz)
)

// POKEY base clock dividers
const (
	POKEY_DIV_64KHZ = 28  // Divider for ~64kHz base clock
	POKEY_DIV_15KHZ = 114 // Divider for ~15kHz base clock
)

// AUDCTL bit masks
const (
	AUDCTL_CLOCK_15KHZ = 0x01 // Bit 0: Use 15kHz base clock (else 64kHz)
	AUDCTL_HIPASS_CH1  = 0x02 // Bit 1: High-pass filter ch1 by ch3
	AUDCTL_HIPASS_CH2  = 0x04 // Bit 2: High-pass filter ch2 by ch4
	AUDCTL_CH4_BY_CH3  = 0x08 // Bit 3: Ch4 clocked by ch3 (16-bit mode)
	AUDCTL_CH2_BY_CH1  = 0x10 // Bit 4: Ch2 clocked by ch1 (16-bit mode)
	AUDCTL_CH3_179MHZ  = 0x20 // Bit 5: Ch3 uses 1.79MHz clock
	AUDCTL_CH1_179MHZ  = 0x40 // Bit 6: Ch1 uses 1.79MHz clock
	AUDCTL_POLY9       = 0x80 // Bit 7: Use 9-bit poly instead of 17-bit
)

// AUDC bit masks
const (
	AUDC_VOLUME_MASK      = 0x0F // Bits 0-3: Volume (0-15)
	AUDC_VOLUME_ONLY      = 0x10 // Bit 4: Volume-only mode (force DC output)
	AUDC_DISTORTION_MASK  = 0xE0 // Bits 5-7: Distortion select
	AUDC_DISTORTION_SHIFT = 5
)

// Distortion modes (AUDC bits 5-7)
const (
	POKEY_DIST_POLY17_POLY5 = 0 // 17-bit poly + 5-bit poly
	POKEY_DIST_POLY5        = 1 // 5-bit poly only
	POKEY_DIST_POLY17_POLY4 = 2 // 17-bit poly + 4-bit poly (most metallic)
	POKEY_DIST_POLY5_POLY4  = 3 // 5-bit poly + 4-bit poly
	POKEY_DIST_POLY17       = 4 // 17-bit poly only (white noise)
	POKEY_DIST_PURE_TONE    = 5 // No poly (pure square wave)
	POKEY_DIST_POLY4        = 6 // 4-bit poly only (buzzy)
	POKEY_DIST_POLY17_PULSE = 7 // 17-bit poly + pulse (50% duty)
)

// Z80 port mapping for POKEY access
// Use: OUT ($D0),A to select register, OUT ($D1),A to write data
const (
	Z80_POKEY_PORT_SELECT = 0xD0
	Z80_POKEY_PORT_DATA   = 0xD1
)

// 6502 memory mapping for POKEY (Atari-style address range)
// Maps $D200-$D209 to POKEY registers 0-9
const (
	C6502_POKEY_BASE = 0xD200
	C6502_POKEY_END  = 0xD209
)

// SAP Player registers (memory-mapped at 0xF0D10-0xF0D1C)
// Used to load and play .sap files with embedded 6502 code
const (
	SAP_PLAY_PTR    = 0xF0D10 // 32-bit pointer to SAP data (little-endian)
	SAP_PLAY_LEN    = 0xF0D14 // 32-bit length of SAP data (little-endian)
	SAP_PLAY_CTRL   = 0xF0D18 // Control: bit 0=start, bit 1=stop, bit 2=loop
	SAP_PLAY_STATUS = 0xF0D1C // Status: bit 0=busy, bit 1=error
	SAP_SUBSONG     = 0xF0D1D // Subsong selection (0-255)
)
