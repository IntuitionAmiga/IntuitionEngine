package main

// MOD Player registers (memory-mapped at 0xF0BC0-0xF0BD7)
// The MOD player provides ProTracker .mod file playback
const (
	MOD_PLAY_PTR     = 0xF0BC0 // 32-bit pointer to MOD data in bus memory
	MOD_PLAY_LEN     = 0xF0BC4 // 32-bit length of MOD data
	MOD_PLAY_CTRL    = 0xF0BC8 // Control: bit 0=start, bit 1=stop, bit 2=loop
	MOD_PLAY_STATUS  = 0xF0BCC // Status: bit 0=playing, bit 1=error
	MOD_FILTER_MODEL = 0xF0BD0 // Filter: 0=none, 1=A500 (4.5kHz), 2=A1200 (28kHz)
	MOD_POSITION     = 0xF0BD4 // Current song position (read-only)
	MOD_END          = 0xF0BD7 // End of MOD register block
)
