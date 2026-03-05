package main

// AROS Audio DMA — MMIO register addresses for Paula-compatible DMA emulation.
//
// The audio DMA engine sits between the AROS audio.device (M68K guest) and
// the IE SoundChip flex channels. The M68K side writes sample pointer, length,
// period, and volume to per-channel registers, then enables DMA via DMACON.
// The Go side reads sample bytes from guest memory at the correct rate and
// writes them to the flex channel DAC registers at 44100 Hz.
//
// Memory layout (0xF2260 – 0xF22AF):
//
//	Per-channel (4 channels × 16 bytes):
//	  CH0: 0xF2260 – 0xF226F
//	  CH1: 0xF2270 – 0xF227F
//	  CH2: 0xF2280 – 0xF228F
//	  CH3: 0xF2290 – 0xF229F
//
//	Global:
//	  0xF22A0: DMACON  (DMA control)
//	  0xF22A4: STATUS  (completion flags)
//	  0xF22A8: INTENA  (interrupt enable)
//	  0xF22AC: reserved

const (
	// MMIO region bounds
	AROS_AUD_REGION_BASE = 0xF2260
	AROS_AUD_REGION_END  = 0xF22AF

	// Per-channel register stride (16 bytes per channel)
	AROS_AUD_CH_STRIDE = 16

	// Per-channel register offsets (relative to channel base)
	AROS_AUD_OFF_PTR = 0x00 // Sample pointer in guest RAM (uint32)
	AROS_AUD_OFF_LEN = 0x04 // Length in words (1 word = 2 bytes, Paula-style)
	AROS_AUD_OFF_PER = 0x08 // Period (Paula-compatible: PAULA_CLOCK / freq)
	AROS_AUD_OFF_VOL = 0x0C // Volume (0–64, Paula-compatible)

	// Global register addresses
	AROS_AUD_DMACON = 0xF22A0 // DMA control: bit 15 = set/clear, bits 0–3 = channels
	AROS_AUD_STATUS = 0xF22A4 // Completion status: bits 0–3 (set by Go, write-to-clear)
	AROS_AUD_INTENA = 0xF22A8 // Interrupt enable: bit 15 = set/clear, bits 0–3 = channels

	// Paula clock (PAL) for period → frequency conversion.
	// frequency = PAULA_CLOCK / period
	paulaClockPAL = 3546895.0

	// Audio interrupt level (M68K autovector level 3, vector 27).
	arosAudioIRQLevel = 3
)
