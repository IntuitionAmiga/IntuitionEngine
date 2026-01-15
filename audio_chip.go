// audio_chip.go - Audio chip emulation for the Intuition Engine

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
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

/*
audio_chip.go - Audio Synthesis Chip for the Intuition Engine

This module implements a complete audio synthesis system with:
- 4 independent channels (each selectable as square, triangle, sine, noise, or sawtooth)
- Per-channel envelope generation with multiple envelope shapes
- Frequency modulation capabilities (sweep, sync, ring modulation)
- Global effects processing (filter, overdrive, reverb)
- Real-time parameter control via memory-mapped registers
- polyBLEP anti-aliasing for cleaner high-frequency output on square and sawtooth waves

The architecture follows classic synthesis chip design while adding
modern features like floating-point processing and advanced effects.
Sample generation happens at 44.1kHz with all processing done in
32-bit floating point for maximum dynamic range.

Signal flow:
1. Oscillator generation (per channel)
2. Envelope and modulation processing
3. Channel mixing
4. Global filter processing
5. Overdrive effect
6. Reverb processing
7. Final output limiting

Thread Safety:
All parameter updates are protected by a mutex, allowing real-time
control from external threads while audio processing continues.
*/

package main

import (
	"log"
	"math"
	"sync"
	"sync/atomic"
)

// ------------------------------------------------------------------------------
// Register Address Ranges
// ------------------------------------------------------------------------------
// F0800-F08FF: Global control and effects
// F0900-F093F: Channel 0 legacy registers (square defaults)
// F0940-F097F: Channel 1 legacy registers (triangle defaults)
// F0980-F09BF: Channel 2 legacy registers (sine defaults)
// F09C0-F09FF: Channel 3 legacy registers (noise defaults)
// F0A00-F0A6F: Modulation/effects legacy registers
// F0A80-F0B3F: Flexible 4-channel register block (preferred)
const (
	SQUARE_REG_START = 0xF0900
	SQUARE_REG_END   = 0xF093F

	TRIANGLE_REG_START = 0xF0914 // Lowest among TRI_SWEEP (0xF0914) and TRI_FREQ (0xF0940)
	TRIANGLE_REG_END   = 0xF097F

	SINE_REG_START = 0xF0918 // Lowest among SINE_SWEEP (0xF0918) and SINE_FREQ (0xF0980)
	SINE_REG_END   = 0xF09BF

	NOISE_REG_START = 0xF09C0 // Lowest is NOISE_FREQ (0xF09C0)
	NOISE_REG_END   = 0xF09FF
)

// -------------------------------------------------------------------------------
// Flexible 4-channel register block (F0A80-F0B3F)
// -------------------------------------------------------------------------------
const (
	FLEX_CH_BASE   = 0xF0A80
	FLEX_CH_STRIDE = 0x30
	FLEX_CH_END    = FLEX_CH_BASE + (FLEX_CH_STRIDE * NUM_CHANNELS) - 1

	FLEX_CH0_BASE = FLEX_CH_BASE
	FLEX_CH1_BASE = FLEX_CH_BASE + FLEX_CH_STRIDE
	FLEX_CH2_BASE = FLEX_CH_BASE + (FLEX_CH_STRIDE * 2)
	FLEX_CH3_BASE = FLEX_CH_BASE + (FLEX_CH_STRIDE * 3)

	FLEX_OFF_FREQ      = 0x00
	FLEX_OFF_VOL       = 0x04
	FLEX_OFF_CTRL      = 0x08
	FLEX_OFF_DUTY      = 0x0C
	FLEX_OFF_SWEEP     = 0x10
	FLEX_OFF_ATK       = 0x14
	FLEX_OFF_DEC       = 0x18
	FLEX_OFF_SUS       = 0x1C
	FLEX_OFF_REL       = 0x20
	FLEX_OFF_WAVE_TYPE = 0x24
	FLEX_OFF_PWM_CTRL  = 0x28
	FLEX_OFF_NOISEMODE = 0x2C
)

// ------------------------------------------------------------------------------
// Square Wave Control Registers (F0900-F093F)
// ------------------------------------------------------------------------------
// Basic oscillator control
const (
	SQUARE_FREQ     = 0xF0900
	SQUARE_VOL      = 0xF0904
	SQUARE_CTRL     = 0xF0908
	SQUARE_ATK      = 0xF0930
	SQUARE_DEC      = 0xF0934
	SQUARE_SUS      = 0xF0938
	SQUARE_REL      = 0xF093C
	SQUARE_DUTY     = 0xF090C
	SQUARE_SWEEP    = 0xF0910
	SQUARE_PWM_CTRL = 0xF0922
)

// ------------------------------------------------------------------------------
// Triangle Wave Control Registers (F0940-F097F)
// ------------------------------------------------------------------------------
// Basic oscillator control

const (
	TRI_FREQ  = 0xF0940
	TRI_VOL   = 0xF0944
	TRI_CTRL  = 0xF0948
	TRI_ATK   = 0xF0960
	TRI_DEC   = 0xF0964
	TRI_SUS   = 0xF0968
	TRI_REL   = 0xF096C
	TRI_SWEEP = 0xF0914
)

// ------------------------------------------------------------------------------
// Sine Wave Control Registers (F0980-F09BF)
// ------------------------------------------------------------------------------
// Basic oscillator control
const (
	SINE_FREQ  = 0xF0980
	SINE_VOL   = 0xF0984
	SINE_CTRL  = 0xF0988
	SINE_ATK   = 0xF0990
	SINE_DEC   = 0xF0994
	SINE_SUS   = 0xF0998
	SINE_REL   = 0xF099C
	SINE_SWEEP = 0xF0918
)

// ------------------------------------------------------------------------------
// Noise Control Registers (F09C0-F09FF)
// ------------------------------------------------------------------------------
// Basic oscillator control
const (
	NOISE_FREQ          = 0xF09C0
	NOISE_VOL           = 0xF09C4
	NOISE_CTRL          = 0xF09C8
	NOISE_ATK           = 0xF09D0
	NOISE_DEC           = 0xF09D4
	NOISE_SUS           = 0xF09D8
	NOISE_REL           = 0xF09DC
	NOISE_MODE          = 0xF09E0
	NOISE_MODE_WHITE    = 0 // Default (existing LFSR)
	NOISE_MODE_PERIODIC = 1 // Periodic/loop
	NOISE_MODE_METALLIC = 2 // "Metal" noise
	NOISE_MODE_PSG      = 3 // AY/YM PSG-style LFSR
)

// Sync source registers
const (
	SYNC_SOURCE_CH0 = 0xF0A00 // Sync source for channel 0
	SYNC_SOURCE_CH1 = 0xF0A04 // Sync source for channel 1
	SYNC_SOURCE_CH2 = 0xF0A08 // Sync source for channel 2
	SYNC_SOURCE_CH3 = 0xF0A0C // Sync source for channel 3
)

// Ring modulation source registers
const (
	RING_MOD_SOURCE_CH0 = 0xF0A10 // Ring mod source for channel 0
	RING_MOD_SOURCE_CH1 = 0xF0A14 // Channel 1
	RING_MOD_SOURCE_CH2 = 0xF0A18 // Channel 2
	RING_MOD_SOURCE_CH3 = 0xF0A1C // Channel 3
)

// ------------------------------------------------------------------------------
// Sawtooth Wave Control Registers (F0A20-F0A5F)
// Legacy aliases that apply to channel 0 with wave type set to sawtooth.
// ------------------------------------------------------------------------------
const (
	SAW_FREQ  = 0xF0A20
	SAW_VOL   = 0xF0A24
	SAW_CTRL  = 0xF0A28
	SAW_SWEEP = 0xF0A2C
	SAW_ATK   = 0xF0A30
	SAW_DEC   = 0xF0A34
	SAW_SUS   = 0xF0A38
	SAW_REL   = 0xF0A3C

	// Sync and ring mod for sawtooth channel
	SYNC_SOURCE_CH4     = 0xF0A60
	RING_MOD_SOURCE_CH4 = 0xF0A64

	// Sawtooth register range
	SAW_REG_START = 0xF0A20
	SAW_REG_END   = 0xF0A6F
)

// Filter, Overdrive, Reverb, and Audio Control registers
const (
	FILTER_CUTOFF     = 0xF0820 // Filter cutoff (0–255 → 0.0–1.0)
	FILTER_RESONANCE  = 0xF0824 // Filter resonance/Q (0–255 → 0.0–1.0)
	FILTER_TYPE       = 0xF0828 // 0=off, 1=low-pass, 2=high-pass, 3=band-pass
	FILTER_MOD_SOURCE = 0xF082C // Register to set modulation source (channel 0–3)
	FILTER_MOD_AMOUNT = 0xF0830 // Register to set modulation depth (0–255 → 0.0–1.0)

	OVERDRIVE_CTRL = 0xF0A40 // Drive amount (0-255 → 0.0-4.0)

	REVERB_MIX   = 0xF0A50 // 0-255 → 0.0-1.0 (dry/wet)
	REVERB_DECAY = 0xF0A54 // 0-255 → 0.1-0.99 (tail length)

	AUDIO_CTRL    = 0xF0800 // Audio control register
	AUDIO_REG_END = FLEX_CH_END

	SAMPLE_RATE = 44100 // Audio sample rate
)

// ------------------------------------------------------------------------------
// Envelope and Wave Shape Constants
// ------------------------------------------------------------------------------
const (
	ENV_ATTACK = iota
	ENV_DECAY
	ENV_SUSTAIN
	ENV_RELEASE
	ENV_SHAPE          = 0xF0804
	ENV_SHAPE_SAW_UP   = 1 // Linear rise to 1.0, then hold
	ENV_SHAPE_SAW_DOWN = 2 // Linear fall to 0.0, then hold
	ENV_SHAPE_LOOP     = 3 // ADSR but loops after release
)

// ------------------------------------------------------------------------------
// Normalisation and Frequency Reference
// ------------------------------------------------------------------------------
const (
	NORMALISE_8BIT = 255.0 // For 8-bit value normalisation (0-255)
	PWM_RANGE      = 256.0 // Keep 256 for duty cycle since it's used as a power of 2
	FREQ_REF       = 256.0 // Keep 256 for frequency reference
	SQUARE_NORM    = 1.0   // Keep square wave at full amplitude
	TRIANGLE_NORM  = 1.0   // Keep triangle at full amplitude
	SINE_NORM      = 1.0   // Keep sine at full amplitude
	NOISE_NORM     = 0.7   // Only normalize noise slightly
)

// ------------------------------------------------------------------------------
// Time Conversion Constants
// ------------------------------------------------------------------------------
const (
	MS_TO_SAMPLES = SAMPLE_RATE / 1000 // Convert milliseconds to samples
	MIN_ENV_TIME  = 1                  // Minimum envelope time
)

// ------------------------------------------------------------------------------
// Filter and Frequency Limits
// ------------------------------------------------------------------------------
const (
	MAX_FILTER_CUTOFF = 0.95  // Maximum filter cutoff frequency
	MIN_FILTER_CUTOFF = 0.0   // Minimum filter cutoff frequency
	MIN_FILTER_FREQ   = 20.0  // Minimum filter frequency in Hz
	MAX_RESONANCE     = 4.0   // Maximum filter resonance
	MAX_FREQ          = 20000 // Maximum frequency in Hz
)

// ------------------------------------------------------------------------------
// Output Sample Limits
// ------------------------------------------------------------------------------
const (
	MAX_SAMPLE = 1.0
	MIN_SAMPLE = -1.0
	MIN_PHASE  = 0.0
	MIN_VOLUME = 0.0
)

// ------------------------------------------------------------------------------
// Mixing and Scaling Constants
// ------------------------------------------------------------------------------
const (
	CHANNEL_MIX_LEVEL  = 0.25 // 1/4 for 4 channels
	REVERB_ATTENUATION = 0.3  // Reverb output scaling
)
const PWM_RATE_SCALE = 0.1 // Convert 7-bit value to Hz range 0-12.7

// ------------------------------------------------------------------------------
// Mathematical and Waveform Constants
// ------------------------------------------------------------------------------
const (
	TWO_PI           = 2 * math.Pi
	SQUARE_AMPLITUDE = 4.0 // Square wave peak amplitude
	TRIANGLE_SCALE   = 2.0 // Triangle wave scaling factor
	HALF_CYCLE       = 0.5
	TRIANGLE_SLOPE   = TRIANGLE_SCALE / HALF_CYCLE // equals 4.0 if TRIANGLE_SCALE is 2.0
	NOISE_FILTER_OLD = 0.95                        // Noise filter old sample weight
	NOISE_FILTER_NEW = 0.05                        // Noise filter new sample weight
)

// ------------------------------------------------------------------------------
// Additional Filter and Sweep Constants
// ------------------------------------------------------------------------------
const (
	MAX_FILTER_FREQ = 20000.0 // Maximum filter frequency in Hz
	SWEEP_RATE      = 4000    // Frequency sweep timing divisor
	DECAY_BASE      = 0.1     // Base reverb decay time
	DECAY_RANGE     = 0.89    // Reverb decay time range
)

// ------------------------------------------------------------------------------
// Bit Masks and Shifts
// ------------------------------------------------------------------------------
const (
	PWM_ENABLE_MASK   = 0x80 // Bit 7 for PWM enable
	PWM_RATE_MASK     = 0x7F // Bits 0-6 for PWM rate
	SWEEP_ENABLE_MASK = 0x80 // Bit 7 for sweep enable
	SWEEP_PERIOD_MASK = 0x07 // 3 bits for sweep period
	SWEEP_SHIFT_MASK  = 0x07 // 3 bits for sweep shift
	SWEEP_DIR_MASK    = 0x08 // Bit 3 for sweep direction
	GATE_MASK         = 0x02 // Bit 1 for gate control
)

// ------------------------------------------------------------------------------
// Hardware Configuration
// ------------------------------------------------------------------------------
const NUM_CHANNELS = 4 // Number of audio channels (each can be any waveform)
const NUM_WAVE_TYPES = 5

// ------------------------------------------------------------------------------
// Noise Generator Tap Positions
// ------------------------------------------------------------------------------
const (
	NOISE_TAP1 = 22 // Primary LFSR tap position
	NOISE_TAP2 = 17 // Secondary LFSR tap position
	METAL_TAP1 = 22 // Metallic noise primary tap
	METAL_TAP2 = 14 // Metallic noise secondary tap
)

// ------------------------------------------------------------------------------
// Register Spacing and Additional Shifts
// ------------------------------------------------------------------------------
const (
	SYNC_REG_SPACING    = 4 // Spacing between sync registers
	RINGMOD_REG_SPACING = 4 // Spacing between ring mod registers
	PWM_DEPTH_SHIFT     = 8 // Shift for PWM depth
	SWEEP_PERIOD_SHIFT  = 4 // Shift for sweep period (bits 4-6)
	MIN_SWEEP_SHIFT     = 1 // Minimum sweep shift to avoid divide-by-zero
)

// ------------------------------------------------------------------------------
// Overdrive
// ------------------------------------------------------------------------------
const MAX_OVERDRIVE = 4.0 // Maximum overdrive gain

// ------------------------------------------------------------------------------
// Byte and Word Masks
// ------------------------------------------------------------------------------
const (
	BYTE_MASK = 0xFF   // 8-bit mask
	WORD_MASK = 0xFFFF // 16-bit mask
)

// ------------------------------------------------------------------------------
// Level Constants
// ------------------------------------------------------------------------------
const (
	MAX_LEVEL = 1.0
	MIN_LEVEL = 0.0
)

// ------------------------------------------------------------------------------
// Default Channel Values
// ------------------------------------------------------------------------------
const (
	DEFAULT_ATTACK_TIME  = 44 // Samples
	DEFAULT_RELEASE_TIME = 44 // Samples
	DEFAULT_DUTY_CYCLE   = 0.5
)

// ------------------------------------------------------------------------------
// Noise Scaling Constants
// ------------------------------------------------------------------------------
const (
	NOISE_BIT_SCALE = 2.0
	NOISE_BIAS      = 1.0
)

// ------------------------------------------------------------------------------
// Triangle Waveform Phase Constants
// ------------------------------------------------------------------------------
const (
	TRIANGLE_PHASE_MULTIPLIER = 2.0
	TRIANGLE_PHASE_SUBTRACT   = 1.0
	TRIANGLE_OUTPUT_OFFSET    = 1.0
)

// ------------------------------------------------------------------------------
// Pre-delay
// ------------------------------------------------------------------------------
const PRE_DELAY_MS = 8 // 8ms pre-delay

// ------------------------------------------------------------------------------
// Comb Filter Constants
// ------------------------------------------------------------------------------
const NUM_COMB_FILTERS = 4 // Number of comb filters for reverb (independent of audio channels)

const (
	COMB_DELAY_1 = 1687
	COMB_DELAY_2 = 1601
	COMB_DELAY_3 = 2053
	COMB_DELAY_4 = 2251
)

const (
	COMB_DECAY_1 = 0.97
	COMB_DECAY_2 = 0.95
	COMB_DECAY_3 = 0.93
	COMB_DECAY_4 = 0.91
)

// ------------------------------------------------------------------------------
// Allpass Filter Constants
// ------------------------------------------------------------------------------
const (
	ALLPASS_DELAY_1 = 389
	ALLPASS_DELAY_2 = 307
)
const ALLPASS_COEF = 0.5 // Standard allpass coefficient for optimal diffusion

// ------------------------------------------------------------------------------
// Noise LFSR Constants
// ------------------------------------------------------------------------------
const (
	NOISE_LFSR_SEED = 0x7FFFFF // 23-bit LFSR seed
	NOISE_LFSR_MASK = 0x7FFFFF // 23-bit mask
)
const NOISE_LFSR_BITS = 23 // Noise LFSR bit width

const (
	PSG_NOISE_LFSR_BITS = 17
	PSG_NOISE_LFSR_MASK = (1 << PSG_NOISE_LFSR_BITS) - 1
	PSG_NOISE_LFSR_SEED = PSG_NOISE_LFSR_MASK
)

// ------------------------------------------------------------------------------
// Wave Types
// ------------------------------------------------------------------------------
const (
	WAVE_SQUARE = iota
	WAVE_TRIANGLE
	WAVE_SINE
	WAVE_NOISE
	WAVE_SAWTOOTH // Sawtooth wave type with polyBLEP anti-aliasing
)

// ------------------------------------------------------------------------------
// Reference Frequency and Phase
// ------------------------------------------------------------------------------
const REF_FREQ = 440.0 // Standard A4 pitch
const LSB_MASK = 1

// ------------------------------------------------------------------------------
// Normalisation Scaling for LFO and Other Calculations
// ------------------------------------------------------------------------------
const (
	NORMALISE_SCALE  = 2.0 // Multiply a [0,1] value to stretch it to [0,2]
	NORMALISE_OFFSET = 1.0 // Subtract 1 to shift range to [-1,1]
)

// ------------------------------------------------------------------------------
// Filter Cutoff Conversion Factor
// ------------------------------------------------------------------------------
// Exponential cutoff mapping: maps 0-1 to 20Hz-20kHz logarithmically
// This provides more musical control as human hearing is logarithmic
const FILTER_CUTOFF_LN_RATIO = float32(6.907755279) // ln(20000/20)
const FILTER_MIN_FREQ = 20.0                        // Minimum filter frequency (Hz)
const TWO_PI_OVER_SR = TWO_PI / SAMPLE_RATE         // Pre-computed for efficiency

// ------------------------------------------------------------------------------
// Mode and Count Constants
// ------------------------------------------------------------------------------
const (
	NUM_ENVELOPE_SHAPES = 4
	NUM_NOISE_MODES     = 4
	NUM_FILTER_TYPES    = 4
	NUM_MOD_SOURCES     = NUM_CHANNELS
	NUM_ALLPASS_FILTERS = 2
)

// ------------------------------------------------------------------------------
// Padding for Structure Alignment
// ------------------------------------------------------------------------------
const (
	CHANNEL_PAD_SIZE    = 2
	COMBFILTER_PAD_SIZE = 4
)
const (
	SOUNDCHIP_PAD1_SIZE = 7
	SOUNDCHIP_PAD2_SIZE = 8
)

// ------------------------------------------------------------------------------
// Default Filter Settings
// ------------------------------------------------------------------------------
const (
	DEFAULT_FILTER_LP  = 0.0
	DEFAULT_FILTER_BP  = 0.0
	DEFAULT_FILTER_HP  = 0.0
	DEFAULT_SUSTAIN    = 1.0
	DEFAULT_DECAY_TIME = 0

	PSG_PLUS_OVERSAMPLE    = 4
	PSG_PLUS_LOWPASS_ALPHA = 0.12
	PSG_PLUS_DRIVE         = 0.18
	PSG_PLUS_ROOM_MIX      = 0.08
	PSG_PLUS_ROOM_DELAY    = 128
)

type Channel struct {
	// ------------------------------------------------------------------------------
	// Channel represents a single audio generation channel that can produce
	// square, triangle, sine, noise, or sawtooth waveforms with envelope control and
	// modulation capabilities.
	//
	// Memory layout is optimised for cache efficiency:
	// Cache line 1 (64 bytes): Sample generation state
	//   - Frequently accessed values for waveform generation
	//   - Current phase, frequency, volume, envelope level
	//
	// Cache line 2 (64 bytes): Modulation parameters
	//   - Less frequently accessed values for modulation
	//   - PWM, envelope sustain, modulation rates
	//
	// Cache line 3 (64 bytes): Configuration state
	//   - Rarely changed parameters
	//   - Wave type, envelope times, counter values
	//
	// Cache line 4 (64 bytes): References and flags
	//   - Pointers to other channels for modulation
	//   - Boolean state flags
	// ------------------------------------------------------------------------------
	mutex sync.RWMutex
	// Hot fields accessed every sample generation (cache line 1)
	// These fields are read/written on each output sample
	frequency           float32 // Base frequency of oscillator
	phase               float32 // Current phase position in waveform
	volume              float32 // Channel volume (0.0-1.0)
	envelopeLevel       float32 // Current envelope amplitude
	prevRawSample       float32 // Previous output (needed for ring modulation)
	dutyCycle           float32 // Square wave duty cycle (0.0-1.0)
	noisePhase          float32 // Phase accumulator for noise timing
	noiseValue          float32 // Current noise generator output
	noiseFilter         float32 // Noise filter coefficient
	noiseFilterState    float32 // Noise filter state variable
	noiseSR             uint32  // Noise shift register state
	psgPlusLowpassState float32 // PSG+ low-pass filter state
	psgPlusDrive        float32 // PSG+ saturation drive
	psgPlusRoomMix      float32 // PSG+ room mix
	psgPlusGain         float32 // PSG+ per-channel gain

	// Envelope and modulation parameters (cache line 2)
	// Accessed during envelope and modulation updates
	sustainLevel float32 // Envelope sustain level (0.0-1.0)
	pwmRate      float32 // PWM modulation rate (Hz)
	pwmDepth     float32 // PWM modulation depth (0.0-1.0)
	pwmPhase     float32 // Current PWM LFO phase

	// Integer state fields (cache line 3)
	// Configuration and timing parameters
	waveType          int  // Oscillator type (WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE, WAVE_NOISE)
	noiseMode         int  // Noise generation mode
	attackTime        int  // Attack time in samples
	decayTime         int  // Decay time in samples
	releaseTime       int  // Release time in samples
	envelopeSample    int  // Current position in envelope
	envelopePhase     int  // Current envelope stage (attack/decay/etc)
	envelopeShape     int  // Envelope shape selection
	sweepPeriod       int  // Sweep update period
	sweepCounter      int  // Current sweep timing counter
	sweepShift        uint // Sweep shift amount
	psgPlusOversample int  // PSG+ oversample factor
	psgPlusRoomDelay  int  // PSG+ room delay length (samples)
	psgPlusRoomPos    int  // PSG+ room delay index

	// Pointer fields (cache line 4)
	ringModSource  *Channel  // Source channel for ring modulation
	syncSource     *Channel  // Source channel for hard sync
	psgPlusRoomBuf []float32 // PSG+ room delay buffer

	// Boolean state flags (packed together to minimise padding)
	enabled        bool                   // Channel enabled flag
	gate           bool                   // Gate/trigger state
	sweepEnabled   bool                   // Frequency sweep enabled
	sweepDirection bool                   // Sweep direction (up/down)
	pwmEnabled     bool                   // PWM enabled flag
	phaseWrapped   bool                   // Phase wrap indicator
	psgPlusEnabled bool                   // PSG+ processing flag
	_pad           [CHANNEL_PAD_SIZE]byte // Padding for alignment
	sampleCount    int                    // Track number of samples generated

	releaseStartLevel float32 // Level when release phase began
}

// SampleTicker allows external systems to advance state per output sample.
type SampleTicker interface {
	TickSample()
}

type sampleTickerHolder struct {
	ticker SampleTicker
}
type CombFilter struct {
	buffer []float32                 // Delay line buffer
	decay  float32                   // Decay coefficient
	pos    int                       // Current buffer position
	_pad   [COMBFILTER_PAD_SIZE]byte // Align to 8-byte boundary
}

type SoundChip struct {
	// Cache line 1 - Hot path DSP state (64 bytes)
	filterLP        float32                   // Current low-pass filter state
	filterBP        float32                   // Current band-pass filter state
	filterHP        float32                   // Current high-pass filter state
	filterCutoff    float32                   // Normalised filter cutoff frequency (0-1)
	filterResonance float32                   // Filter resonance/Q factor (0-1)
	filterModAmount float32                   // Filter modulation depth (0-1)
	overdriveLevel  float32                   // Overdrive distortion amount (0-4)
	reverbMix       float32                   // Reverb wet/dry mix ratio (0-1)
	filterType      int                       // Filter mode (0=off, 1=LP, 2=HP, 3=BP)
	enabled         bool                      // Global chip enable flag
	_pad1           [SOUNDCHIP_PAD1_SIZE]byte // Align to 64-byte cache line boundary

	// Cache line 2 - Channel references and thread safety (64 bytes)
	channels        [NUM_CHANNELS]*Channel    // Array of audio channel pointers
	filterModSource *Channel                  // Channel modulating the filter cutoff
	mutex           sync.RWMutex              // Concurrency control for parameter updates
	_pad2           [SOUNDCHIP_PAD2_SIZE]byte // Align to 64-byte cache line boundary

	sampleTicker atomic.Value // Optional per-sample ticker (SampleTicker)

	// Cache line 3+ - Reverb state (cold path)
	preDelayPos int                            // Current position in pre-delay buffer
	allpassPos  [NUM_ALLPASS_FILTERS]int       // Current positions in allpass buffers
	combFilters [NUM_COMB_FILTERS]CombFilter   // Parallel comb filter bank for reverb
	allpassBuf  [NUM_ALLPASS_FILTERS][]float32 // Allpass diffusion filters
	preDelayBuf []float32                      // 8ms pre-delay buffer
	output      AudioOutput                    // Audio backend interface
}

func NewSoundChip(backend int) (*SoundChip, error) {
	// ------------------------------------------------------------------------------
	// NewSoundChip creates and initialises a new SoundChip instance.
	// It sets default filter parameters, initialises the channels with default envelope and oscillator settings,
	// and configures the comb and allpass filters used for the reverb effect.
	// It also initialises the audio backend and returns any error encountered.
	// ------------------------------------------------------------------------------

	// Initialise sound chip with default settings
	chip := &SoundChip{
		filterLP:    DEFAULT_FILTER_LP,
		filterBP:    DEFAULT_FILTER_BP,
		filterHP:    DEFAULT_FILTER_HP,
		preDelayBuf: make([]float32, PRE_DELAY_MS*MS_TO_SAMPLES),
	}
	chip.sampleTicker.Store(&sampleTickerHolder{})

	// Initialise channels
	waveTypes := []int{WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE, WAVE_NOISE}
	for i := 0; i < NUM_CHANNELS; i++ {
		chip.channels[i] = &Channel{
			waveType:          waveTypes[i],
			attackTime:        DEFAULT_ATTACK_TIME,
			decayTime:         DEFAULT_DECAY_TIME,
			sustainLevel:      DEFAULT_SUSTAIN,
			releaseTime:       DEFAULT_RELEASE_TIME,
			envelopePhase:     ENV_ATTACK,
			noiseSR:           NOISE_LFSR_SEED, // Initial seed for noise
			dutyCycle:         DEFAULT_DUTY_CYCLE,
			phase:             MIN_PHASE,
			volume:            MIN_VOLUME,
			psgPlusGain:       1.0,
			psgPlusOversample: 1,
		}
	}

	// Initialise audio output
	output, err := NewAudioOutput(backend, SAMPLE_RATE, chip)
	if err != nil {
		return nil, err
	}
	chip.output = output

	// Initialise comb filters
	var combDelays = []int{COMB_DELAY_1, COMB_DELAY_2, COMB_DELAY_3, COMB_DELAY_4}
	var combDecays = []float32{COMB_DECAY_1, COMB_DECAY_2, COMB_DECAY_3, COMB_DECAY_4}

	for i := range chip.combFilters {
		chip.combFilters[i] = CombFilter{
			buffer: make([]float32, combDelays[i]),
			decay:  combDecays[i],
		}
	}

	// Initialise allpass filters
	var allpassDelays = []int{ALLPASS_DELAY_1, ALLPASS_DELAY_2}
	for i := range chip.allpassBuf {
		chip.allpassBuf[i] = make([]float32, allpassDelays[i])
	}

	return chip, nil
}

func (chip *SoundChip) HandleRegisterWrite(addr uint32, value uint32) {
	// ------------------------------------------------------------------------------
	// HandleRegisterWrite processes a write to a hardware register address.
	// Register map overview:
	//
	// Control Registers (F800-F8FF):
	//   F800: Global enable
	//   F820-F830: Filter parameters
	//   F840-F850: Effect controls
	//
	// Channel Registers (F900-F9FF):
	//   Legacy per-channel register space
	//   Base addresses: F900, F940, F980, F9C0
	// Flexible Registers (FA80-FB3F):
	//   Preferred 4-channel register block with consistent offsets
	//
	// Modulation Registers (FA00-FAFF):
	//   FA00-FA0C: Sync sources
	//   FA10-FA1C: Ring modulation sources
	//   FA40-FA54: Global effect parameters
	//
	// ------------------------------------------------------------------------------

	// Thread safety: This method holds the chip mutex during execution.
	chip.mutex.Lock()
	defer chip.mutex.Unlock()

	if addr == AUDIO_CTRL {
		chip.enabled = value != 0
		return
	}

	if addr >= FLEX_CH_BASE && addr <= FLEX_CH_END {
		chIndex := (addr - FLEX_CH_BASE) / FLEX_CH_STRIDE
		if chIndex >= NUM_CHANNELS {
			log.Printf("invalid channel index: %d", chIndex)
			return
		}
		ch := chip.channels[chIndex]
		offset := (addr - FLEX_CH_BASE) % FLEX_CH_STRIDE
		switch offset {
		case FLEX_OFF_FREQ:
			ch.frequency = float32(value)
		case FLEX_OFF_VOL:
			ch.volume = float32(value&BYTE_MASK) / NORMALISE_8BIT
		case FLEX_OFF_CTRL:
			ch.enabled = value != 0
			newGate := value&GATE_MASK != 0

			if newGate && !ch.gate {
				ch.envelopePhase = ENV_ATTACK
				ch.envelopeSample = 0
				ch.envelopeLevel = 0
			}

			if !newGate && ch.gate && ch.envelopePhase == ENV_SUSTAIN {
				ch.releaseStartLevel = ch.envelopeLevel
				ch.envelopePhase = ENV_RELEASE
				ch.envelopeSample = 0
			}
			ch.gate = newGate
		case FLEX_OFF_DUTY:
			value16 := uint16(value & WORD_MASK)
			ch.dutyCycle = float32(value16&BYTE_MASK) / PWM_RANGE
			ch.pwmDepth = float32((value16>>PWM_DEPTH_SHIFT)&BYTE_MASK) / (PWM_RANGE * 2.0)
		case FLEX_OFF_SWEEP:
			ch.sweepEnabled = (value & SWEEP_ENABLE_MASK) != 0
			ch.sweepPeriod = int((value >> SWEEP_PERIOD_SHIFT) & SWEEP_PERIOD_MASK)
			ch.sweepShift = uint(value & SWEEP_SHIFT_MASK)
			if ch.sweepShift == 0 {
				ch.sweepShift = MIN_SWEEP_SHIFT
			}
			ch.sweepDirection = (value & SWEEP_DIR_MASK) != 0
		case FLEX_OFF_ATK:
			ch.attackTime = max(int(value*MS_TO_SAMPLES), MIN_ENV_TIME)
		case FLEX_OFF_DEC:
			ch.decayTime = max(int(value*MS_TO_SAMPLES), MIN_ENV_TIME)
		case FLEX_OFF_SUS:
			ch.sustainLevel = float32(value) / NORMALISE_8BIT
		case FLEX_OFF_REL:
			ch.releaseTime = max(int(value*MS_TO_SAMPLES), MIN_ENV_TIME)
		case FLEX_OFF_WAVE_TYPE:
			ch.waveType = int(value % NUM_WAVE_TYPES)
		case FLEX_OFF_PWM_CTRL:
			ch.pwmEnabled = (value & PWM_ENABLE_MASK) != 0
			ch.pwmRate = float32(value&PWM_RATE_MASK) * PWM_RATE_SCALE
		case FLEX_OFF_NOISEMODE:
			ch.noiseMode = int(value % NUM_NOISE_MODES)
		default:
			log.Printf("invalid flex register offset: 0x%X", offset)
		}
		return
	}

	var ch *Channel
	switch {
	case addr >= SQUARE_REG_START && addr <= SQUARE_REG_END:
		ch = chip.channels[0]
	case addr >= TRIANGLE_REG_START && addr <= TRIANGLE_REG_END:
		ch = chip.channels[1]
	case addr >= SINE_REG_START && addr <= SINE_REG_END:
		ch = chip.channels[2]
	case addr >= NOISE_REG_START && addr <= NOISE_REG_END:
		ch = chip.channels[3]
	case addr >= SAW_REG_START && addr <= SAW_REG_END:
		ch = chip.channels[0]
	}

	switch addr {
	case SQUARE_PWM_CTRL:
		ch.pwmEnabled = (value & PWM_ENABLE_MASK) != 0             // Bit 7: enable
		ch.pwmRate = float32(value&PWM_RATE_MASK) * PWM_RATE_SCALE // Rate: 0–12.7 Hz
	case SQUARE_DUTY:
		value16 := uint16(value & WORD_MASK)
		ch.dutyCycle = float32(value16&BYTE_MASK) / PWM_RANGE
		ch.pwmDepth = float32((value16>>PWM_DEPTH_SHIFT)&BYTE_MASK) / (PWM_RANGE * 2.0)
	case SQUARE_FREQ, TRI_FREQ, SINE_FREQ, NOISE_FREQ, SAW_FREQ:
		if addr == SAW_FREQ {
			ch.waveType = WAVE_SAWTOOTH
		}
		ch.frequency = float32(value)
	case SQUARE_VOL, TRI_VOL, SINE_VOL, NOISE_VOL, SAW_VOL:
		if addr == SAW_VOL {
			ch.waveType = WAVE_SAWTOOTH
		}
		ch.volume = float32(value&BYTE_MASK) / NORMALISE_8BIT
	case SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL, SAW_CTRL:
		if addr == SAW_CTRL {
			ch.waveType = WAVE_SAWTOOTH
		}
		ch.enabled = value != 0
		newGate := value&GATE_MASK != 0

		if newGate && !ch.gate {
			ch.envelopePhase = ENV_ATTACK
			ch.envelopeSample = 0
			ch.envelopeLevel = 0
		}

		if !newGate && ch.gate && ch.envelopePhase == ENV_SUSTAIN {
			ch.releaseStartLevel = ch.envelopeLevel
			ch.envelopePhase = ENV_RELEASE
			ch.envelopeSample = 0
		}
		ch.gate = newGate
	case SQUARE_ATK, TRI_ATK, SINE_ATK, NOISE_ATK, SAW_ATK:
		if addr == SAW_ATK {
			ch.waveType = WAVE_SAWTOOTH
		}
		ch.attackTime = max(int(value*MS_TO_SAMPLES), MIN_ENV_TIME)
	case SQUARE_DEC, TRI_DEC, SINE_DEC, NOISE_DEC, SAW_DEC:
		if addr == SAW_DEC {
			ch.waveType = WAVE_SAWTOOTH
		}
		ch.decayTime = max(int(value*MS_TO_SAMPLES), MIN_ENV_TIME)
	case SQUARE_SUS, TRI_SUS, SINE_SUS, NOISE_SUS, SAW_SUS:
		if addr == SAW_SUS {
			ch.waveType = WAVE_SAWTOOTH
		}
		ch.sustainLevel = float32(value) / NORMALISE_8BIT
	case SQUARE_REL, TRI_REL, SINE_REL, NOISE_REL, SAW_REL:
		if addr == SAW_REL {
			ch.waveType = WAVE_SAWTOOTH
		}
		ch.releaseTime = max(int(value*MS_TO_SAMPLES), MIN_ENV_TIME)
	case NOISE_MODE:
		ch.waveType = WAVE_NOISE
		ch.noiseMode = int(value % NUM_NOISE_MODES) // 0=white, 1=periodic, 2=metallic, 3=psg
	//case ENV_SHAPE:
	//	ch.envelopeShape = int(value % NUM_ENVELOPE_SHAPES) // 0=ADSR, 1=SawUp, 2=SawDown, 3=Loop
	//	// Reset envelope state
	//	ch.envelopePhase = ENV_ATTACK
	//	ch.envelopeSample = 0
	case ENV_SHAPE:
		// Before accessing ch directly, first make sure it's not nil
		// If it's nil, we'll use the first channel as default
		if ch == nil {
			ch = chip.channels[0] // Default to first channel if none selected
		}
		ch.envelopeShape = int(value % NUM_ENVELOPE_SHAPES) // 0=ADSR, 1=SawUp, 2=SawDown, 3=Loop
		// Reset envelope state
		ch.envelopePhase = ENV_ATTACK
		ch.envelopeSample = 0
	case SQUARE_SWEEP:
		ch.sweepEnabled = (value & SWEEP_ENABLE_MASK) != 0
		ch.sweepPeriod = int((value >> SWEEP_PERIOD_SHIFT) & SWEEP_PERIOD_MASK)
		ch.sweepShift = uint(value & SWEEP_SHIFT_MASK)
		if ch.sweepShift == 0 {
			ch.sweepShift = MIN_SWEEP_SHIFT
		}
		ch.sweepDirection = (value & SWEEP_DIR_MASK) != 0
	case TRI_SWEEP:
		ch = chip.channels[1] // Force channel 1 for TRI_SWEEP
		ch.sweepEnabled = (value & SWEEP_ENABLE_MASK) != 0
		ch.sweepPeriod = int((value >> SWEEP_PERIOD_SHIFT) & SWEEP_PERIOD_MASK)
		ch.sweepShift = uint(value & SWEEP_SHIFT_MASK)
		if ch.sweepShift == 0 {
			ch.sweepShift = MIN_SWEEP_SHIFT
		}
		ch.sweepDirection = (value & SWEEP_DIR_MASK) != 0
	case SINE_SWEEP:
		ch.sweepEnabled = (value & SWEEP_ENABLE_MASK) != 0
		ch.sweepPeriod = int((value >> SWEEP_PERIOD_SHIFT) & SWEEP_PERIOD_MASK)
		ch.sweepShift = 1 // Force minimum shift value for largest frequency changes
		ch.sweepDirection = (value & SWEEP_DIR_MASK) != 0
	case SAW_SWEEP:
		ch.waveType = WAVE_SAWTOOTH
		ch.sweepEnabled = (value & SWEEP_ENABLE_MASK) != 0
		ch.sweepPeriod = int((value >> SWEEP_PERIOD_SHIFT) & SWEEP_PERIOD_MASK)
		ch.sweepShift = uint(value & SWEEP_SHIFT_MASK)
		if ch.sweepShift == 0 {
			ch.sweepShift = MIN_SWEEP_SHIFT
		}
		ch.sweepDirection = (value & SWEEP_DIR_MASK) != 0
	case SYNC_SOURCE_CH0, SYNC_SOURCE_CH1, SYNC_SOURCE_CH2, SYNC_SOURCE_CH3:
		// Determine target channel (e.g., SYNC_SOURCE_CH0 → channel 0)
		chIndex := (addr - SYNC_SOURCE_CH0) / SYNC_REG_SPACING
		if chIndex >= NUM_CHANNELS {
			return
		}
		ch := chip.channels[chIndex]
		// Set sync source to another channel (0–3)
		masterIndex := int(value % NUM_CHANNELS)
		ch.syncSource = chip.channels[masterIndex]
	case RING_MOD_SOURCE_CH0, RING_MOD_SOURCE_CH1, RING_MOD_SOURCE_CH2, RING_MOD_SOURCE_CH3:
		chIndex := (addr - RING_MOD_SOURCE_CH0) / RINGMOD_REG_SPACING
		if chIndex >= NUM_CHANNELS {
			return
		}
		ch := chip.channels[chIndex]
		masterIndex := int(value % NUM_CHANNELS)
		ch.ringModSource = chip.channels[masterIndex]
	case FILTER_CUTOFF:
		chip.filterCutoff = float32(value) / NORMALISE_8BIT
	case FILTER_RESONANCE:
		chip.filterResonance = float32(value) / NORMALISE_8BIT
	case FILTER_TYPE:
		chip.filterType = int(value % NUM_FILTER_TYPES)
	case FILTER_MOD_SOURCE:
		// Set modulation source to one of the 4 channels
		sourceIndex := int(value % NUM_MOD_SOURCES)
		chip.filterModSource = chip.channels[sourceIndex]
	case FILTER_MOD_AMOUNT:
		// Normalise modulation depth to 0.0–1.0
		chip.filterModAmount = float32(value) / NORMALISE_8BIT
	case OVERDRIVE_CTRL:
		chip.overdriveLevel = float32(value) / NORMALISE_8BIT * MAX_OVERDRIVE // 0.0-4.0 gain
	case REVERB_MIX:
		chip.reverbMix = float32(value) / NORMALISE_8BIT
	case REVERB_DECAY:
		baseDecay := DECAY_BASE + (float32(value)/NORMALISE_8BIT)*DECAY_RANGE
		combDecays := []float32{COMB_DECAY_1, COMB_DECAY_2, COMB_DECAY_3, COMB_DECAY_4}
		for i, decayFactor := range combDecays {
			chip.combFilters[i].decay = baseDecay * decayFactor
		}
	default:
		log.Printf("invalid register address: 0x%X", addr)
	}
}

func (ch *Channel) updateEnvelope() {
	// ------------------------------------------------------------------------------
	// updateEnvelope advances the envelope generator state and updates the envelope level.
	// The envelope can operate in several modes:
	//
	// Standard ADSR:
	//   Attack  - Linear ramp from 0 to 1
	//   Decay   - Linear ramp to sustain level
	//   Sustain - Hold level while gate is high
	//   Release - Linear ramp to 0 when gate goes low
	//
	// Alternative Shapes:
	//   Saw Up   - Single ramp up, then hold
	//   Saw Down - Single ramp down, then hold
	//   Loop     - Continuously cycle through stages
	//
	// All timing parameters are in samples at the system sample rate.
	// ------------------------------------------------------------------------------

	ch.mutex.Lock()
	defer ch.mutex.Unlock()

	switch ch.envelopePhase {
	case ENV_ATTACK:
		switch ch.envelopeShape {
		case ENV_SHAPE_SAW_UP:
			if ch.attackTime <= 0 {
				ch.envelopeLevel = MAX_LEVEL
				ch.envelopePhase = ENV_SUSTAIN
			} else {
				ch.envelopeLevel = float32(ch.envelopeSample) / float32(ch.attackTime)
				ch.envelopeSample++
				if ch.envelopeSample >= ch.attackTime {
					ch.envelopeLevel = MAX_LEVEL
					ch.envelopePhase = ENV_SUSTAIN
				}
			}
		case ENV_SHAPE_SAW_DOWN:
			if ch.attackTime <= 0 {
				ch.envelopeLevel = MIN_LEVEL
				ch.envelopePhase = ENV_SUSTAIN
			} else {
				ch.envelopeLevel = MAX_LEVEL - float32(ch.envelopeSample)/float32(ch.attackTime)
				ch.envelopeSample++
				if ch.envelopeSample >= ch.attackTime {
					ch.envelopeLevel = MIN_LEVEL
					ch.envelopePhase = ENV_SUSTAIN
				}
			}
		default: // Default ADSR logic
			if ch.attackTime <= 0 {
				ch.envelopeLevel = MAX_LEVEL
				ch.envelopePhase = ENV_DECAY
				ch.envelopeSample = 0
			} else {
				ch.envelopeLevel = float32(ch.envelopeSample) / float32(ch.attackTime)
				ch.envelopeSample++
				if ch.envelopeSample >= ch.attackTime {
					ch.envelopeLevel = MAX_LEVEL
					ch.envelopePhase = ENV_DECAY
					ch.envelopeSample = 0
				}
			}
		}

	case ENV_DECAY:
		if ch.decayTime <= 0 {
			ch.envelopeLevel = ch.sustainLevel
			ch.envelopePhase = ENV_SUSTAIN
		} else {
			ch.envelopeLevel = MAX_LEVEL - ((MAX_LEVEL - ch.sustainLevel) * float32(ch.envelopeSample) / float32(ch.decayTime))
			ch.envelopeSample++
			if ch.envelopeSample >= ch.decayTime {
				ch.envelopePhase = ENV_SUSTAIN
				ch.envelopeLevel = ch.sustainLevel
			}
		}

	case ENV_SUSTAIN:
		if !ch.gate {
			ch.envelopePhase = ENV_RELEASE
			ch.envelopeSample = 0
		}

	case ENV_RELEASE:
		switch ch.envelopeShape {
		case ENV_SHAPE_LOOP:
			// Linear interpolation from release start level to zero
			remaining := 1.0 - float32(ch.envelopeSample)/float32(ch.releaseTime)
			ch.envelopeLevel = ch.releaseStartLevel * remaining
			ch.envelopeSample++
			if ch.envelopeSample >= ch.releaseTime {
				ch.envelopePhase = ENV_ATTACK // Loop back to attack
				ch.envelopeSample = 0
			}
		default: // Default ADSR release logic
			if ch.releaseTime <= 0 {
				ch.envelopeLevel = 0
				ch.enabled = false
			} else {
				//decay := float32(math.Pow(0.001, 1.0/float64(ch.releaseTime)))
				decay := float32(math.Pow(0.01, 1.0/float64(ch.releaseTime)))
				ch.envelopeLevel *= decay
				ch.envelopeSample++
				if ch.envelopeSample >= ch.releaseTime || ch.envelopeLevel <= 0.001 {
					ch.envelopeLevel = 0
					ch.enabled = false
				}
			}
		}
	}
}

func (ch *Channel) generateWaveSample(sampleRate float32) float32 {
	var rawSample float32
	phaseInc := TWO_PI * float32(ch.frequency) / sampleRate

	switch ch.waveType {
	case WAVE_SQUARE:
		currentDuty := ch.dutyCycle
		if ch.pwmEnabled {
			ch.pwmPhase += ch.pwmRate * (TWO_PI / sampleRate)
			ch.pwmPhase = float32(math.Mod(float64(ch.pwmPhase+ch.pwmRate*(TWO_PI/sampleRate)), TWO_PI))
			normalisedPhase := ch.pwmPhase / TWO_PI
			lfo := float32(math.Abs(float64(normalisedPhase*NORMALISE_SCALE-NORMALISE_OFFSET)))*NORMALISE_SCALE - NORMALISE_OFFSET
			currentDuty = ch.dutyCycle + lfo*ch.pwmDepth
			if currentDuty < 0 {
				currentDuty = 0
			} else if currentDuty > 1 {
				currentDuty = 1
			}
		}
		normalizedPhase := ch.phase / TWO_PI
		dt := float64(ch.frequency) / float64(sampleRate)

		if normalizedPhase < currentDuty {
			rawSample = SQUARE_AMPLITUDE
		} else {
			rawSample = -SQUARE_AMPLITUDE
		}

		phaseNorm64 := float64(normalizedPhase)
		dutyNorm64 := float64(currentDuty)
		rawSample += float32(polyBLEP(phaseNorm64, dt)) * SQUARE_AMPLITUDE
		rawSample -= float32(polyBLEP(math.Mod(phaseNorm64-dutyNorm64+1.0, 1.0), dt)) * SQUARE_AMPLITUDE

		rawSample *= SQUARE_NORM

	case WAVE_TRIANGLE:
		phaseNorm := ch.phase / TWO_PI
		if phaseNorm < HALF_CYCLE {
			rawSample = TRIANGLE_SLOPE*phaseNorm - TRIANGLE_PHASE_SUBTRACT
		} else {
			rawSample = (TRIANGLE_SLOPE - TRIANGLE_PHASE_SUBTRACT) - TRIANGLE_SLOPE*phaseNorm
		}
		rawSample *= TRIANGLE_NORM

	case WAVE_SINE:
		rawSample = float32(math.Sin(float64(ch.phase)))
		rawSample *= SINE_NORM

	case WAVE_NOISE:
		noisePhaseInc := ch.frequency / sampleRate
		ch.noisePhase += noisePhaseInc
		steps := int(ch.noisePhase)
		ch.noisePhase -= float32(steps)

		for i := 0; i < steps; i++ {
			switch ch.noiseMode {
			case NOISE_MODE_WHITE:
				newBit := ((ch.noiseSR >> NOISE_TAP1) ^ (ch.noiseSR >> NOISE_TAP2)) & 1
				ch.noiseSR = ((ch.noiseSR << LSB_MASK) | newBit) & NOISE_LFSR_MASK
			case NOISE_MODE_PERIODIC:
				ch.noiseSR = ((ch.noiseSR >> LSB_MASK) | ((ch.noiseSR & 1) << (NOISE_LFSR_BITS - 1))) & NOISE_LFSR_MASK
			case NOISE_MODE_METALLIC:
				newBit := ((ch.noiseSR >> METAL_TAP1) ^ (ch.noiseSR >> METAL_TAP2)) & 1
				ch.noiseSR = ((ch.noiseSR << LSB_MASK) | newBit) & NOISE_LFSR_MASK
			case NOISE_MODE_PSG:
				newBit := ((ch.noiseSR >> 0) ^ (ch.noiseSR >> 3)) & 1
				ch.noiseSR = ((ch.noiseSR << LSB_MASK) | newBit) & PSG_NOISE_LFSR_MASK
			}
		}

		ch.noiseValue = float32(ch.noiseSR&LSB_MASK)*NOISE_BIT_SCALE - NOISE_BIAS
		ch.noiseFilterState = NOISE_FILTER_OLD*ch.noiseFilterState + NOISE_FILTER_NEW*ch.noiseValue
		rawSample = ch.noiseFilterState
		rawSample *= NOISE_NORM

	case WAVE_SAWTOOTH:
		phaseNorm := float64(ch.phase / TWO_PI)
		dt := float64(ch.frequency) / float64(sampleRate)
		rawSample = float32(2.0*phaseNorm - 1.0)
		rawSample -= float32(polyBLEP(phaseNorm, dt))
	}

	if ch.ringModSource != nil {
		rawSample *= ch.ringModSource.prevRawSample
	}
	ch.prevRawSample = rawSample

	if ch.waveType != WAVE_NOISE {
		ch.phase += phaseInc
		if ch.phase >= TWO_PI {
			ch.phase -= TWO_PI
			ch.phaseWrapped = true
		} else {
			ch.phaseWrapped = false
		}
	}

	if ch.syncSource != nil && ch.syncSource.phaseWrapped && ch.waveType != WAVE_NOISE {
		ch.phase = 0
	}

	return rawSample
}

func (ch *Channel) generateSample() float32 {
	// ------------------------------------------------------------------------------
	// generateSample computes and returns the next output sample for this channel.
	// The generation process follows these steps:
	//
	// 1. Update envelope state and level
	// 2. Apply frequency sweep if enabled
	// 3. Generate raw waveform based on oscillator type:
	//    - Square: PWM-capable pulse wave
	//    - Triangle: Linear ramp up/down
	//    - Sine: Pure sinusoid
	//    - Noise: LFSR-based with multiple modes
	// 4. Apply modulation effects:
	//    - Ring modulation if source configured
	//    - Hard sync if master channel set
	// 5. Scale output by volume and envelope

	if !ch.enabled || ch.frequency == 0 {
		return 0
	}

	ch.updateEnvelope()

	ch.mutex.Lock()
	envLevel := ch.envelopeLevel
	ch.mutex.Unlock()

	// Frequency sweep logic
	if ch.sweepEnabled && ch.waveType != WAVE_NOISE {
		ch.sweepCounter++
		if ch.sweepCounter >= ch.sweepPeriod {
			// Calculate delta per sample instead of per period
			delta := (ch.frequency / float32(int(1)<<ch.sweepShift)) / float32(ch.sweepPeriod*SWEEP_RATE)

			var newFreq float32
			if ch.sweepDirection {
				newFreq = ch.frequency + delta
			} else {
				if delta > ch.frequency {
					newFreq = MIN_FILTER_FREQ
				} else {
					newFreq = ch.frequency - delta
				}
			}

			// Original bounds
			if newFreq < MIN_FILTER_FREQ {
				newFreq = MIN_FILTER_FREQ
			} else if newFreq > MAX_FREQ {
				newFreq = MAX_FREQ
			}

			// Per-test range limits based on initial frequency
			maxAllowed := float32(MAX_FREQ)
			minAllowed := float32(MIN_FILTER_FREQ)

			if ch.sweepDirection {
				maxAllowed = ch.frequency * 2.0 // One octave up
			} else {
				minAllowed = ch.frequency / 2.0 // One octave down
			}

			if newFreq < minAllowed {
				newFreq = minAllowed
			} else if newFreq > maxAllowed {
				newFreq = maxAllowed
			}

			ch.frequency = newFreq
			ch.sweepCounter = 0
		}
	}

	if ch.psgPlusEnabled && ch.psgPlusOversample > 1 {
		oversample := ch.psgPlusOversample
		sampleRate := float32(SAMPLE_RATE) * float32(oversample)
		var sum float32
		for i := 0; i < oversample; i++ {
			sum += ch.generateWaveSample(sampleRate)
		}
		rawSample := sum / float32(oversample)
		alpha := float32(PSG_PLUS_LOWPASS_ALPHA)
		if alpha > 0 {
			ch.psgPlusLowpassState = ch.psgPlusLowpassState*(1-alpha) + rawSample*alpha
			rawSample = ch.psgPlusLowpassState
		}
		if ch.psgPlusRoomMix > 0 && len(ch.psgPlusRoomBuf) > 0 {
			delayed := ch.psgPlusRoomBuf[ch.psgPlusRoomPos]
			ch.psgPlusRoomBuf[ch.psgPlusRoomPos] = rawSample
			ch.psgPlusRoomPos = (ch.psgPlusRoomPos + 1) % len(ch.psgPlusRoomBuf)
			rawSample = rawSample*(1-ch.psgPlusRoomMix) + delayed*ch.psgPlusRoomMix
		}
		scaledSample := rawSample * ch.volume * envLevel * ch.psgPlusGain
		if ch.psgPlusDrive > 0 {
			gain := 1.0 + ch.psgPlusDrive
			scaledSample = float32(math.Tanh(float64(scaledSample * gain)))
		}
		return clampF32(scaledSample, MIN_SAMPLE, MAX_SAMPLE)
	}

	rawSample := ch.generateWaveSample(float32(SAMPLE_RATE))
	scaledSample := rawSample * ch.volume * envLevel

	//if ch.waveType == WAVE_SINE {
	//	fmt.Printf("Raw sample: %.2f, After volume: %.2f\n", rawSample, scaledSample)
	//}
	//if ch.waveType == WAVE_SINE {
	//	log.Printf("vol: %.2f env: %.2f raw: %.2f scaled: %.2f",
	//		ch.volume, ch.envelopeLevel, rawSample, scaledSample)
	//}

	// Clamp before returning
	return clampF32(scaledSample, MIN_SAMPLE, MAX_SAMPLE)
}

func (chip *SoundChip) GenerateSample() float32 {
	// ------------------------------------------------------------------------------
	// GenerateSample generates a single audio sample by processing all active channels
	// through the following signal chain:
	//
	// 1. Channel Generation and Mixing
	//    - Each enabled channel generates its raw waveform (square/triangle/sine/noise/saw)
	//    - Channel outputs are summed with equal mixing weights
	//    - Per-channel envelope and modulation effects are applied
	//
	// 2. Global Filter Processing
	//    - State variable filter provides LP/HP/BP modes
	//    - Cutoff frequency can be modulated by a selected channel
	//    - Resonance control for additional timbral shaping
	//    - Filter coefficients are updated per-sample
	//
	// 3. Effects Processing
	//    - Overdrive effect for harmonic enhancement
	//    - Reverb effect for spatial enhancement
	//    - Wet/dry mixing for effect balance
	//
	// Thread Safety:
	// The function uses read/write locks to safely access shared state:
	//   - Initial state capture under read lock
	//   - Channel array copied for thread safety
	//   - Filter state updates under write lock
	//   - All other processing lock-free
	//
	// Returns a stereo sample pair in the range [-1.0, 1.0]
	// ------------------------------------------------------------------------------

	// Take read lock and capture all state needed for sample generation to ensure consistency and thread safety
	chip.mutex.RLock()
	enabled := chip.enabled
	filterType := chip.filterType
	filterCutoff := chip.filterCutoff
	filterModSource := chip.filterModSource
	filterModAmount := chip.filterModAmount
	filterResonance := chip.filterResonance
	overdriveLevel := chip.overdriveLevel
	reverbMix := chip.reverbMix
	filterLP := chip.filterLP
	filterBP := chip.filterBP

	// Make thread-safe copy of channel array
	channels := [NUM_CHANNELS]*Channel{}
	for i := 0; i < NUM_CHANNELS; i++ {
		channels[i] = chip.channels[i]
	}
	chip.mutex.RUnlock()

	if !enabled {
		return 0
	}

	// Mix samples from all active channels
	var sum float32
	activeCount := 0
	var primaryType uint32 = 0 // Store the wave type of first active channel
	for i := 0; i < NUM_CHANNELS; i++ {
		ch := channels[i]
		if ch.enabled {
			sum += ch.generateSample()
			activeCount++
		}
	}

	var sample float32
	if activeCount == 0 {
		sample = 0
	} else if activeCount == 1 {
		// When only one channel is active, use its sample without attenuation.
		sample = sum
	} else {
		// When multiple channels are active, average their samples.
		sample = sum / float32(activeCount)
	}

	// Apply overdrive effect with waveform-specific processing
	if overdriveLevel > 0 {
		// More aggressive gain calculation
		gain := 1.0 + (float32(overdriveLevel) / 10.0)

		// Apply waveform-specific gain scaling for better effect
		switch primaryType {
		case WAVE_NOISE:
			gain *= 1.5
		case WAVE_SINE:
			gain *= 1.2
		case WAVE_TRIANGLE:
			gain *= 1.2
		}

		// Apply overdrive with tanh for soft clipping
		sample = float32(math.Tanh(float64(sample * gain)))
	}

	// Apply global filter processing
	if filterType != 0 && filterCutoff > 0 {
		// Calculate modulated cutoff frequency
		modulatedCutoff := filterCutoff
		if filterModSource != nil {
			modSignal := filterModSource.prevRawSample * filterModAmount
			modulatedCutoff = filterCutoff + modSignal
			modulatedCutoff = float32(math.Max(math.Min(float64(modulatedCutoff), MAX_FILTER_CUTOFF), MIN_FILTER_CUTOFF))
		}

		// Apply 2-pole state variable filter with exponential cutoff mapping
		cutoff := calculateFilterCutoff(modulatedCutoff)
		resonance := filterResonance * MAX_RESONANCE

		lp := filterLP + cutoff*filterBP
		hp := (sample - lp) - resonance*filterBP
		bp := filterBP + cutoff*hp

		// Clamp filter outputs
		lp = float32(math.Max(math.Min(float64(lp), MAX_SAMPLE), MIN_SAMPLE))
		bp = float32(math.Max(math.Min(float64(bp), MAX_SAMPLE), MIN_SAMPLE))
		hp = float32(math.Max(math.Min(float64(hp), MAX_SAMPLE), MIN_SAMPLE))

		// Flush denormals to prevent CPU stalls
		lp = flushDenormal(lp)
		bp = flushDenormal(bp)
		hp = flushDenormal(hp)

		// Update filter state under lock
		chip.mutex.Lock()
		chip.filterLP = lp
		chip.filterBP = bp
		chip.filterHP = hp
		chip.mutex.Unlock()

		// Select filter output
		switch filterType {
		case 1:
			sample = lp
		case 2:
			sample = hp
		case 3:
			sample = bp
		}
	}

	// Apply reverb effect and final mix
	wet := chip.applyReverb(sample)
	sample = sample*(1-reverbMix) + wet*reverbMix

	// Clamp final output
	return float32(math.Max(math.Min(float64(sample), MAX_SAMPLE), MIN_SAMPLE))
}

func (chip *SoundChip) applyReverb(input float32) float32 {
	// ------------------------------------------------------------------------------
	// applyReverb implements a classic Schroeder reverberator with the following stages:
	//
	// 1. Pre-delay (8ms) - Separates direct sound from reflections
	// 2. Parallel comb filters:
	//    - Four delay lines with prime-number lengths
	//    - Independent decay times for natural frequency response
	// 3. Series allpass filters:
	//    - Two stages for additional diffusion
	//    - Coefficient of 0.5 for neutral coloration
	//
	// The delay times are carefully chosen to avoid harmonic relationships
	// that would cause metallic resonances in the output.
	//
	// Parameters:
	//   input - Dry signal sample in range [-1.0, 1.0]
	// Returns:
	//   Processed wet signal in range [-1.0, 1.0]

	// Reverb configuration:
	// - Uses 4 parallel comb filters with prime-length delays (1687,1601,2053,2251)
	//   to create dense, natural-sounding echoes without metallic resonances
	// - Each comb has scaled decay (0.97,0.95,0.93,0.91) for smooth high-frequency damping
	// - Two allpass filters (389,307 samples) with coefficient 0.5
	//   provide additional diffusion without coloring the sound
	// - 8ms pre-delay separates direct sound from early reflections
	// - Delay lengths chosen to avoid arithmetic relationships that cause
	//   artificial-sounding periodicity
	//
	// Reverb stages:
	// 1. Input pre-delayed for spatial separation
	// 2. Pre-delayed signal splits to parallel comb filters
	// 3. Comb outputs sum and feed into series allpass filters
	// 4. Final mix between dry/wet signals
	// ------------------------------------------------------------------------------

	// Apply pre-delay
	delayed := chip.preDelayBuf[chip.preDelayPos]
	chip.preDelayBuf[chip.preDelayPos] = input
	chip.preDelayPos = (chip.preDelayPos + 1) % len(chip.preDelayBuf)

	// Process comb filters
	var out float32
	for i := range chip.combFilters {
		comb := &chip.combFilters[i]
		cDelay := comb.buffer[comb.pos]
		comb.buffer[comb.pos] = delayed + cDelay*comb.decay
		out += cDelay
		comb.pos = (comb.pos + 1) % len(comb.buffer)
	}

	// Process allpass filters
	for i := range chip.allpassBuf {
		pos := chip.allpassPos[i]
		buf := chip.allpassBuf[i]
		aDelay := buf[pos]
		buf[pos] = out + aDelay*ALLPASS_COEF
		out = aDelay - out
		chip.allpassPos[i] = (pos + 1) % len(buf)
	}

	return out * REVERB_ATTENUATION // Attenuate to prevent overflow
}

func (chip *SoundChip) ReadSample() float32 {
	if holder, ok := chip.sampleTicker.Load().(*sampleTickerHolder); ok {
		if holder.ticker != nil {
			holder.ticker.TickSample()
		}
	}
	return chip.GenerateSample()
}

func (chip *SoundChip) SetSampleTicker(ticker SampleTicker) {
	chip.sampleTicker.Store(&sampleTickerHolder{ticker: ticker})
}

func (chip *SoundChip) SetPSGPlusEnabled(enabled bool) {
	chip.mutex.Lock()
	defer chip.mutex.Unlock()

	for i := 0; i < NUM_CHANNELS; i++ {
		ch := chip.channels[i]
		if ch == nil {
			continue
		}
		ch.psgPlusEnabled = enabled
		if enabled {
			ch.psgPlusOversample = PSG_PLUS_OVERSAMPLE
			ch.psgPlusLowpassState = 0
			ch.psgPlusDrive = PSG_PLUS_DRIVE
			ch.psgPlusRoomMix = PSG_PLUS_ROOM_MIX
			ch.psgPlusRoomDelay = PSG_PLUS_ROOM_DELAY
			ch.psgPlusRoomPos = 0
			if ch.psgPlusRoomBuf == nil || len(ch.psgPlusRoomBuf) != PSG_PLUS_ROOM_DELAY {
				ch.psgPlusRoomBuf = make([]float32, PSG_PLUS_ROOM_DELAY)
			} else {
				for i := range ch.psgPlusRoomBuf {
					ch.psgPlusRoomBuf[i] = 0
				}
			}
			if i < 3 {
				ch.psgPlusGain = psgPlusMixGain[i]
			} else {
				ch.psgPlusGain = 1.0
			}
		} else {
			ch.psgPlusOversample = 1
			ch.psgPlusLowpassState = 0
			ch.psgPlusDrive = 0
			ch.psgPlusRoomMix = 0
			ch.psgPlusRoomDelay = 0
			ch.psgPlusRoomPos = 0
			ch.psgPlusRoomBuf = nil
			ch.psgPlusGain = 1.0
		}
	}
}

func (chip *SoundChip) Start() {
	chip.mutex.Lock()
	defer chip.mutex.Unlock()
	chip.enabled = true
	chip.output.Start()
}

func (chip *SoundChip) Stop() {
	chip.mutex.Lock()
	defer chip.mutex.Unlock()

	if !chip.enabled {
		return
	}

	chip.enabled = false
	if chip.output != nil {
		chip.output.Stop()
		chip.output.Close()
	}
}

func clampF32(value, min, max float32) float32 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// polyBLEP applies polynomial band-limited step correction to reduce aliasing
// at waveform discontinuities. This produces cleaner high-frequency output
// for square and sawtooth waves.
//
// t is the normalized phase position (0.0-1.0)
// dt is the phase increment per sample (frequency/sampleRate)
func polyBLEP(t, dt float64) float64 {
	if t < dt {
		// Leading edge correction
		t /= dt
		return t + t - t*t - 1.0
	} else if t > 1.0-dt {
		// Trailing edge correction
		t = (t - 1.0) / dt
		return t*t + t + t + 1.0
	}
	return 0.0
}

// flushDenormal prevents CPU stalls from subnormal float values
// by flushing very small numbers to zero.
const denormalThreshold = 1e-15

func flushDenormal(v float32) float32 {
	if v > -denormalThreshold && v < denormalThreshold {
		return 0.0
	}
	return v
}

// fastExp computes an approximation of exp(x) using range reduction and Taylor series.
// This is much faster than math.Exp while maintaining sufficient accuracy for audio DSP.
//
// Uses the mathematical property: exp(x) = exp(r) * 2^n where r = x - n*ln(2)
// This reduces x to the range [-ln(2)/2, +ln(2)/2] where Taylor series is most accurate.
//
//go:inline
func fastExp(x float32) float32 {
	if x > 10.0 {
		return 22026.465 // exp(10)
	}
	if x < -10.0 {
		return 0.0000454 // exp(-10)
	}

	// Range reduction: exp(x) = exp(r) * 2^n where r = x - n*ln(2)
	const LN2 = 0.693147180559945309417232121458    // ln(2)
	const INV_LN2 = 1.44269504088896340735992468100 // 1/ln(2)

	// Calculate integer n such that r = x - n*ln(2) is in [-ln(2)/2, +ln(2)/2]
	n := int(float32(float64(x)*INV_LN2 + 0.5))
	r := x - float32(float64(n)*LN2)

	// 4th-order Taylor series on reduced range
	r2 := r * r
	r3 := r2 * r
	r4 := r2 * r2
	expR := 1.0 + r + r2*0.5 + r3*0.166667 + r4*0.041667

	// Scale by 2^n using bit manipulation
	if n >= 0 {
		return expR * float32(uint32(1)<<uint(n))
	}
	return expR / float32(uint32(1)<<uint(-n))
}

// calculateFilterCutoff computes normalized filter frequency from cutoff value (0-1)
// using exponential mapping for more musical control.
// Maps 0-1 to 20Hz-20kHz logarithmically (human hearing is logarithmic).
//
//go:inline
func calculateFilterCutoff(cutoffValue float32) float32 {
	if cutoffValue <= 0.0 {
		return FILTER_MIN_FREQ * TWO_PI_OVER_SR
	}
	if cutoffValue >= 1.0 {
		return 20000.0 * TWO_PI_OVER_SR
	}

	// Exponential mapping: freq = 20 * exp(ln(20000/20) * cutoff)
	freqHz := FILTER_MIN_FREQ * fastExp(FILTER_CUTOFF_LN_RATIO*cutoffValue)
	return freqHz * TWO_PI_OVER_SR
}

//func init() {
// Uncomment to run alignment checks
//	// Verify struct alignments
//	channelAlign := unsafe.Alignof(Channel{})
//	combFilterAlign := unsafe.Alignof(CombFilter{})
//	soundChipAlign := unsafe.Alignof(SoundChip{})
//
//	// Verify struct sizes
//	channelSize := unsafe.Sizeof(Channel{})
//	combFilterSize := unsafe.Sizeof(CombFilter{})
//	soundChipSize := unsafe.Sizeof(SoundChip{})
//
//	// Print alignment info
//	fmt.Printf("Channel: align=%d size=%d\n", channelAlign, channelSize)
//	fmt.Printf("CombFilter: align=%d size=%d\n", combFilterAlign, combFilterSize)
//	fmt.Printf("SoundChip: align=%d size=%d\n", soundChipAlign, soundChipSize)
//
//	var chip SoundChip
//	mutexOffset := unsafe.Offsetof(chip.mutex)
//	fmt.Printf("Mutex offset: %d bytes\n", mutexOffset)
//	fmt.Printf("Mutex is 8-byte aligned: %v\n", mutexOffset%8 == 0)
//}
