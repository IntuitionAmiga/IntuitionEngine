// sid_constants.go - MOS 6581/8580 SID sound chip register addresses and constants

package main

// SID register addresses (memory-mapped at 0xF0E00-0xF0E1C)
const (
	SID_BASE = 0xF0E00
	SID_END  = 0xF0E1C

	// Voice 1 registers (0x00-0x06)
	SID_V1_FREQ_LO = 0xF0E00 // Voice 1 frequency low byte
	SID_V1_FREQ_HI = 0xF0E01 // Voice 1 frequency high byte
	SID_V1_PW_LO   = 0xF0E02 // Voice 1 pulse width low byte
	SID_V1_PW_HI   = 0xF0E03 // Voice 1 pulse width high byte (bits 0-3 only)
	SID_V1_CTRL    = 0xF0E04 // Voice 1 control register
	SID_V1_AD      = 0xF0E05 // Voice 1 attack/decay
	SID_V1_SR      = 0xF0E06 // Voice 1 sustain/release

	// Voice 2 registers (0x07-0x0D)
	SID_V2_FREQ_LO = 0xF0E07
	SID_V2_FREQ_HI = 0xF0E08
	SID_V2_PW_LO   = 0xF0E09
	SID_V2_PW_HI   = 0xF0E0A
	SID_V2_CTRL    = 0xF0E0B
	SID_V2_AD      = 0xF0E0C
	SID_V2_SR      = 0xF0E0D

	// Voice 3 registers (0x0E-0x14)
	SID_V3_FREQ_LO = 0xF0E0E
	SID_V3_FREQ_HI = 0xF0E0F
	SID_V3_PW_LO   = 0xF0E10
	SID_V3_PW_HI   = 0xF0E11
	SID_V3_CTRL    = 0xF0E12
	SID_V3_AD      = 0xF0E13
	SID_V3_SR      = 0xF0E14

	// Filter registers (0x15-0x17)
	SID_FC_LO    = 0xF0E15 // Filter cutoff low (bits 0-2 only)
	SID_FC_HI    = 0xF0E16 // Filter cutoff high byte
	SID_RES_FILT = 0xF0E17 // Filter resonance (bits 4-7) and routing (bits 0-3)

	// Volume and filter mode (0x18)
	SID_MODE_VOL = 0xF0E18 // Volume (bits 0-3), filter mode (bits 4-7)

	// SID+ control register
	SID_PLUS_CTRL = 0xF0E19 // SID+ mode enable (0=standard, 1=enhanced)

	// Read-only registers (on real SID, we can emulate these)
	SID_POT_X = 0xF0E19 // Potentiometer X (not implemented)
	SID_POT_Y = 0xF0E1A // Potentiometer Y (not implemented)
	SID_OSC3  = 0xF0E1B // Oscillator 3 output
	SID_ENV3  = 0xF0E1C // Envelope 3 output

	SID_REG_COUNT = 29
)

// SID clock frequencies
const (
	SID_CLOCK_PAL  = 985248  // PAL C64 clock (Hz)
	SID_CLOCK_NTSC = 1022727 // NTSC C64 clock (Hz)
)

// SID chip model types
const (
	SID_MODEL_6581 = 0 // Original SID (non-linear filter, warmer sound)
	SID_MODEL_8580 = 1 // Revised SID (linear filter, cleaner sound)
)

// Voice control register bits
const (
	SID_CTRL_GATE     = 0x01 // Bit 0: Gate (trigger envelope)
	SID_CTRL_SYNC     = 0x02 // Bit 1: Sync with previous voice
	SID_CTRL_RINGMOD  = 0x04 // Bit 2: Ring modulation with previous voice
	SID_CTRL_TEST     = 0x08 // Bit 3: Test bit (resets oscillator)
	SID_CTRL_TRIANGLE = 0x10 // Bit 4: Triangle waveform
	SID_CTRL_SAWTOOTH = 0x20 // Bit 5: Sawtooth waveform
	SID_CTRL_PULSE    = 0x40 // Bit 6: Pulse/square waveform
	SID_CTRL_NOISE    = 0x80 // Bit 7: Noise waveform
)

// Filter resonance/routing register bits
const (
	SID_FILT_V1  = 0x01 // Bit 0: Route voice 1 through filter
	SID_FILT_V2  = 0x02 // Bit 1: Route voice 2 through filter
	SID_FILT_V3  = 0x04 // Bit 2: Route voice 3 through filter
	SID_FILT_EXT = 0x08 // Bit 3: Route external input through filter
	SID_FILT_RES = 0xF0 // Bits 4-7: Filter resonance (0-15)
)

// Mode/volume register bits
const (
	SID_MODE_VOL_MASK = 0x0F // Bits 0-3: Master volume (0-15)
	SID_MODE_LP       = 0x10 // Bit 4: Low-pass filter
	SID_MODE_BP       = 0x20 // Bit 5: Band-pass filter
	SID_MODE_HP       = 0x40 // Bit 6: High-pass filter
	SID_MODE_3OFF     = 0x80 // Bit 7: Voice 3 off (disconnect from output)
)

// SID ADSR timing tables (values in milliseconds)
// These are approximations based on the SID's exponential decay
var sidAttackMs = [16]float32{
	2, 8, 16, 24, 38, 56, 68, 80,
	100, 250, 500, 800, 1000, 3000, 5000, 8000,
}

var sidDecayReleaseMs = [16]float32{
	6, 24, 48, 72, 114, 168, 204, 240,
	300, 750, 1500, 2400, 3000, 9000, 15000, 24000,
}

// SID ADSR rate counter periods (clock cycles at 985248 Hz PAL)
// These are the base periods for each ADSR value (0-15)
// NOTE: These are reference values for cycle-accurate emulation.
// Currently the envelope uses sidAttackMs/sidDecayReleaseMs time-based tables
// which provide a good approximation without the complexity of rate counters.
var sidADSRRatePeriods = [16]uint32{
	9, 32, 63, 95, 149, 220, 267, 313,
	392, 977, 1954, 3126, 3907, 11720, 19532, 31251,
}

// SID envelope exponential decay thresholds
// When envelope level crosses these thresholds, decay rate changes
// This creates the characteristic "bent" SID envelope curve
// NOTE: Reference values - the current implementation approximates this
// behavior using a simplified 3-region bent curve in audio_chip.go
var sidEnvExpThresholds = [6]uint8{93, 54, 26, 14, 6, 0}

// SID envelope exponential rate multipliers at each threshold
// Rate gets progressively slower as level decreases
// NOTE: Reference values - see sidEnvExpThresholds comment above
var sidEnvExpMultipliers = [6]uint8{1, 2, 4, 8, 16, 30}

// Z80 port mapping for SID access
const (
	Z80_SID_PORT_SELECT = 0xE0
	Z80_SID_PORT_DATA   = 0xE1
)

// 6502 memory mapping for SID
// Note: C64's original SID was at $D400, but that conflicts with PSG mapping.
// For Intuition Engine 6502 mode, SID is mapped at $D500-$D51C
const (
	C6502_SID_BASE = 0xD500
	C6502_SID_END  = 0xD51C
)
