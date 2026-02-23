package main

// WAV Player registers (memory-mapped at 0xF0BD8-0xF0BF3)
// The WAV player provides PCM .wav file playback
const (
	WAV_PLAY_PTR    = 0xF0BD8 // 32-bit pointer to WAV data in bus memory
	WAV_PLAY_LEN    = 0xF0BDC // 32-bit length of WAV data
	WAV_PLAY_CTRL   = 0xF0BE0 // Control: bit 0=start, bit 1=stop, bit 2=loop
	WAV_PLAY_STATUS = 0xF0BE4 // Status: bit 0=busy, bit 1=error
	WAV_POSITION    = 0xF0BE8 // Current playback position (read-only)
	WAV_END         = 0xF0BEB // End of WAV register block
)
