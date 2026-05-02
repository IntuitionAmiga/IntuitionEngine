// ahx_constants.go - AHX engine register constants

package main

// AHX Engine registers (memory-mapped at 0xF0B80-0xF0B94)
// The AHX engine provides Amiga AHX module playback
const (
	AHX_BASE      = 0xF0B80
	AHX_PLUS_CTRL = 0xF0B80 // AHX+ mode (0=standard, 1=enhanced)
	AHX_END       = 0xF0B80
)

// AHX Player registers (memory-mapped at 0xF0B84-0xF0B94)
// Used to load and play .ahx files
const (
	AHX_PLAY_PTR    = 0xF0B84 // 32-bit pointer to AHX data (little-endian)
	AHX_PLAY_LEN    = 0xF0B88 // 32-bit length of AHX data (little-endian)
	AHX_PLAY_CTRL   = 0xF0B8C // Control: bit 0=start, bit 1=stop, bit 2=loop
	AHX_PLAY_STATUS = 0xF0B90 // Status: bit 0=busy, bit 1=error
	AHX_SUBSONG     = 0xF0B91 // Subsong selection (0-255)
)
