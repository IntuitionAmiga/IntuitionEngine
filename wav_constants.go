package main

// WAV Player registers (memory-mapped at 0xF0BD8-0xF0BF3)
// The WAV player provides PCM .wav file playback
const (
	WAV_PLAY_PTR     = 0xF0BD8 // Low 32 bits of WAV data pointer
	WAV_PLAY_LEN     = 0xF0BDC // 32-bit length of WAV data
	WAV_PLAY_CTRL    = 0xF0BE0 // Control: bit 0=start, bit 1=stop, bit 2=loop, bit 3=pause, bit 4=loop apply only
	WAV_PLAY_STATUS  = 0xF0BE4 // Status: bit 0=busy, bit 1=error, bit 2=paused, bit 3=stereo active
	WAV_POSITION     = 0xF0BE8 // Current source frame position (read-only)
	WAV_PLAY_PTR_HI  = 0xF0BEC // High 32 bits of WAV data pointer
	WAV_CHANNEL_BASE = 0xF0BF0 // DAC channel base for left; right uses base+1
	WAV_VOLUME_L     = 0xF0BF1 // Left volume, 0-255
	WAV_VOLUME_R     = 0xF0BF2 // Right volume, 0-255
	WAV_FLAGS        = 0xF0BF3 // bit 0=force mono
	WAV_END          = 0xF0BF3 // Inclusive end of WAV register block
)
