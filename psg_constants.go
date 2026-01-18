package main

const (
	PSG_BASE      = 0xF0C00
	PSG_END       = 0xF0C0D
	PSG_PLUS_CTRL = 0xF0C0E
	PSG_REG_COUNT = 14

	PSG_PLAY_PTR    = 0xF0C10
	PSG_PLAY_LEN    = 0xF0C14
	PSG_PLAY_CTRL   = 0xF0C18
	PSG_PLAY_STATUS = 0xF0C1C

	PSG_CLOCK_ATARI_ST    = 2000000
	PSG_CLOCK_ZX_SPECTRUM = 1773400
	PSG_CLOCK_CPC         = 1000000
	PSG_CLOCK_MSX         = 1789773

	Z80_CLOCK_ZX_SPECTRUM = 3494400

	// Z80 PSG port I/O mapping for standalone Z80 programs
	// Use: OUT ($F0),A to select register, OUT ($F1),A to write data
	//      IN A,($F0) to read selected register number, IN A,($F1) to read data
	Z80_PSG_PORT_SELECT = 0xF0
	Z80_PSG_PORT_DATA   = 0xF1

	// 6502 PSG memory mapping (C64 SID-style address range)
	// Maps $D400-$D40D to PSG registers 0-13
	C6502_PSG_BASE = 0xD400
	C6502_PSG_END  = 0xD40D
)
