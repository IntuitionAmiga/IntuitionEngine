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

(c) 2024 - 2025 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

/*
audio_chip.go - Audio Synthesis Engine for the Intuition Platform

This module implements a complete audio synthesis system with:
- 4 independent channels (Square, Triangle, Sine, and Noise)
- Per-channel envelope generation with multiple envelope shapes
- Frequency modulation capabilities (sweep, sync, ring modulation)
- Global effects processing (filter, overdrive, reverb)
- Real-time parameter control via memory-mapped registers

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
)

// ------------------------------------------------------------------------------
// Register Address Ranges
// ------------------------------------------------------------------------------
// F800-F8FF: Global control and effects
// F900-F93F: Square wave channel
// F940-F97F: Triangle wave channel
// F980-F9BF: Sine wave channel
// F9C0-F9FF: Noise channel
// FA00-FAFF: Modulation control

// ------------------------------------------------------------------------------
// Square Wave Control Registers (F900-F93F)
// ------------------------------------------------------------------------------
// Basic oscillator control
const (
	SQUARE_FREQ     = 0xF900
	SQUARE_VOL      = 0xF904
	SQUARE_CTRL     = 0xF908
	SQUARE_ATK      = 0xF930
	SQUARE_DEC      = 0xF934
	SQUARE_SUS      = 0xF938
	SQUARE_REL      = 0xF93C
	SQUARE_DUTY     = 0xF90C
	SQUARE_SWEEP    = 0xF910
	SQUARE_PWM_CTRL = 0xF922
)

// ------------------------------------------------------------------------------
// Triangle Wave Control Registers (F940-F97F)
// ------------------------------------------------------------------------------
// Basic oscillator control

const (
	TRI_FREQ  = 0xF940
	TRI_VOL   = 0xF944
	TRI_CTRL  = 0xF948
	TRI_ATK   = 0xF960
	TRI_DEC   = 0xF964
	TRI_SUS   = 0xF968
	TRI_REL   = 0xF96C
	TRI_SWEEP = 0xF914
)

// ------------------------------------------------------------------------------
// Sine Wave Control Registers (F980-F9BF)
// ------------------------------------------------------------------------------
// Basic oscillator control
const (
	SINE_FREQ  = 0xF980
	SINE_VOL   = 0xF984
	SINE_CTRL  = 0xF988
	SINE_ATK   = 0xF990
	SINE_DEC   = 0xF994
	SINE_SUS   = 0xF998
	SINE_REL   = 0xF99C
	SINE_SWEEP = 0xF918
)

// ------------------------------------------------------------------------------
// Noise Control Registers (F9C0-F9FF)
// ------------------------------------------------------------------------------
// Basic oscillator control
const (
	NOISE_FREQ          = 0xF9C0
	NOISE_VOL           = 0xF9C4
	NOISE_CTRL          = 0xF9C8
	NOISE_ATK           = 0xF9D0
	NOISE_DEC           = 0xF9D4
	NOISE_SUS           = 0xF9D8
	NOISE_REL           = 0xF9DC
	NOISE_MODE          = 0xF9E0
	NOISE_SWEEP         = 0xF91C
	NOISE_MODE_WHITE    = 0 // Default (existing LFSR)
	NOISE_MODE_PERIODIC = 1 // Periodic/loop
	NOISE_MODE_METALLIC = 2 // "Metal" noise
)

// Sync source registers
const (
	SYNC_SOURCE_CH0 = 0xFA00 // Sync source for channel 0
	SYNC_SOURCE_CH1 = 0xFA04 // Sync source for channel 1
	SYNC_SOURCE_CH2 = 0xFA08 // Sync source for channel 2
	SYNC_SOURCE_CH3 = 0xFA0C // Sync source for channel 3
)

// Ring modulation source registers
const (
	RING_MOD_SOURCE_CH0 = 0xFA10 // Ring mod source for channel 0
	RING_MOD_SOURCE_CH1 = 0xFA14 // Channel 1
	RING_MOD_SOURCE_CH2 = 0xFA18 // Channel 2
	RING_MOD_SOURCE_CH3 = 0xFA1C // Channel 3
)

// Filter, Overdrive, Reverb, and Audio Control registers
const (
	FILTER_CUTOFF     = 0xF820 // Filter cutoff (0–255 → 0.0–1.0)
	FILTER_RESONANCE  = 0xF824 // Filter resonance/Q (0–255 → 0.0–1.0)
	FILTER_TYPE       = 0xF828 // 0=off, 1=low-pass, 2=high-pass, 3=band-pass
	FILTER_MOD_SOURCE = 0xF82C // Register to set modulation source (channel 0–3)
	FILTER_MOD_AMOUNT = 0xF830 // Register to set modulation depth (0–255 → 0.0–1.0)

	OVERDRIVE_CTRL = 0xFA40 // Drive amount (0-255 → 0.0-4.0)

	REVERB_MIX   = 0xFA50 // 0-255 → 0.0-1.0 (dry/wet)
	REVERB_DECAY = 0xFA54 // 0-255 → 0.1-0.99 (tail length)

	AUDIO_CTRL = 0xF800 // Audio control register

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
	ENV_SHAPE          = 0xF804
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
	SQUARE_AMPLITUDE = 4.0  // Square wave peak amplitude
	TRIANGLE_SCALE   = 2.0  // Triangle wave scaling factor
	NOISE_FILTER_OLD = 0.95 // Noise filter old sample weight
	NOISE_FILTER_NEW = 0.05 // Noise filter new sample weight
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
const NUM_CHANNELS = 4 // Number of audio channels

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

// ------------------------------------------------------------------------------
// Wave Types
// ------------------------------------------------------------------------------
const (
	WAVE_SQUARE = iota
	WAVE_TRIANGLE
	WAVE_SINE
	WAVE_NOISE
)

// ------------------------------------------------------------------------------
// Reference Frequency and Phase
// ------------------------------------------------------------------------------
const REF_FREQ = 440.0 // Standard A4 pitch
const LSB_MASK = 1
const PHASE_INC_FACTOR = TWO_PI / SAMPLE_RATE

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
const CUTOFF_FACTOR = TWO_PI * MAX_FILTER_FREQ / SAMPLE_RATE

// ------------------------------------------------------------------------------
// Mode and Count Constants
// ------------------------------------------------------------------------------
const (
	NUM_ENVELOPE_SHAPES = 4
	NUM_NOISE_MODES     = 3
	NUM_FILTER_TYPES    = 4
	NUM_MOD_SOURCES     = 4
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
)

type Channel struct {
	// ------------------------------------------------------------------------------
	// Channel represents a single audio generation channel that can produce
	// square, triangle, sine or noise waveforms with envelope control and
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

	// Hot fields accessed every sample generation (cache line 1)
	// These fields are read/written on each output sample
	frequency        float32 // Base frequency of oscillator
	phase            float32 // Current phase position in waveform
	volume           float32 // Channel volume (0.0-1.0)
	envelopeLevel    float32 // Current envelope amplitude
	prevRawSample    float32 // Previous output (needed for ring modulation)
	dutyCycle        float32 // Square wave duty cycle (0.0-1.0)
	noisePhase       float32 // Phase accumulator for noise timing
	noiseValue       float32 // Current noise generator output
	noiseFilter      float32 // Noise filter coefficient
	noiseFilterState float32 // Noise filter state variable
	noiseSR          uint32  // Noise shift register state

	// Envelope and modulation parameters (cache line 2)
	// Accessed during envelope and modulation updates
	sustainLevel float32 // Envelope sustain level (0.0-1.0)
	pwmRate      float32 // PWM modulation rate (Hz)
	pwmDepth     float32 // PWM modulation depth (0.0-1.0)
	pwmPhase     float32 // Current PWM LFO phase

	// Integer state fields (cache line 3)
	// Configuration and timing parameters
	waveType       int  // Oscillator type (WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE, WAVE_NOISE)
	noiseMode      int  // Noise generation mode
	attackTime     int  // Attack time in samples
	decayTime      int  // Decay time in samples
	releaseTime    int  // Release time in samples
	envelopeSample int  // Current position in envelope
	envelopePhase  int  // Current envelope stage (attack/decay/etc)
	envelopeShape  int  // Envelope shape selection
	sweepPeriod    int  // Sweep update period
	sweepCounter   int  // Current sweep timing counter
	sweepShift     uint // Sweep shift amount

	// Pointer fields (cache line 4)
	ringModSource *Channel // Source channel for ring modulation
	syncSource    *Channel // Source channel for hard sync

	// Boolean state flags (packed together to minimise padding)
	enabled        bool                   // Channel enabled flag
	gate           bool                   // Gate/trigger state
	sweepEnabled   bool                   // Frequency sweep enabled
	sweepDirection bool                   // Sweep direction (up/down)
	pwmEnabled     bool                   // PWM enabled flag
	phaseWrapped   bool                   // Phase wrap indicator
	_pad           [CHANNEL_PAD_SIZE]byte // Padding for alignment
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
	channels        [NUM_CHANNELS]*Channel    // Array of 4 audio channel pointers
	filterModSource *Channel                  // Channel modulating the filter cutoff
	mutex           sync.RWMutex              // Concurrency control for parameter updates
	_pad2           [SOUNDCHIP_PAD2_SIZE]byte // Align to 64-byte cache line boundary

	// Cache line 3+ - Reverb state (cold path)
	preDelayPos int                            // Current position in pre-delay buffer
	allpassPos  [NUM_ALLPASS_FILTERS]int       // Current positions in allpass buffers
	combFilters [NUM_CHANNELS]CombFilter       // Parallel comb filter bank for reverb
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

	// Initialise channels
	waveTypes := []int{WAVE_SQUARE, WAVE_TRIANGLE, WAVE_SINE, WAVE_NOISE}
	for i := 0; i < NUM_CHANNELS; i++ {
		chip.channels[i] = &Channel{
			waveType:      waveTypes[i],
			attackTime:    DEFAULT_ATTACK_TIME,
			decayTime:     DEFAULT_DECAY_TIME,
			sustainLevel:  DEFAULT_SUSTAIN,
			releaseTime:   DEFAULT_RELEASE_TIME,
			envelopePhase: ENV_ATTACK,
			noiseSR:       NOISE_LFSR_SEED, // Initial seed for noise
			dutyCycle:     DEFAULT_DUTY_CYCLE,
			phase:         MIN_PHASE,
			volume:        MIN_VOLUME,
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
	//   Each channel has 64 bytes of register space
	//   Base addresses: F900, F940, F980, F9C0
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

	var ch *Channel
	switch {
	case addr >= SQUARE_FREQ && addr <= SQUARE_REL:
		ch = chip.channels[0]
	case addr >= TRI_FREQ && addr <= TRI_REL:
		ch = chip.channels[1]
	case addr >= SINE_FREQ && addr <= SINE_REL:
		ch = chip.channels[2]
	case addr >= NOISE_FREQ && addr <= NOISE_REL:
		ch = chip.channels[3]
	}

	switch addr {
	case SQUARE_PWM_CTRL:
		ch.pwmEnabled = (value & PWM_ENABLE_MASK) != 0             // Bit 7 = enable
		ch.pwmRate = float32(value&PWM_RATE_MASK) * PWM_RATE_SCALE // Rate: 0–12.7 Hz (7 bits)
	case SQUARE_DUTY:
		value16 := uint16(value & WORD_MASK) // Ensure 16-bit value
		ch.dutyCycle = float32(value16&BYTE_MASK) / PWM_RANGE
		ch.pwmDepth = float32((value16>>PWM_DEPTH_SHIFT)&BYTE_MASK) / PWM_RANGE
	case SQUARE_FREQ, TRI_FREQ, SINE_FREQ, NOISE_FREQ:
		ch.frequency = float32(value)
	case SQUARE_VOL, TRI_VOL, SINE_VOL, NOISE_VOL:
		ch.volume = float32(value&BYTE_MASK) / NORMALISE_8BIT
	case SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL:
		ch.enabled = value != 0
		newGate := value&GATE_MASK != 0

		if newGate && !ch.gate {
			ch.envelopePhase = ENV_ATTACK
			ch.envelopeSample = 0
		}
		if !newGate && ch.gate && ch.envelopePhase == ENV_SUSTAIN {
			ch.envelopePhase = ENV_RELEASE
			ch.envelopeSample = 0
		}
		ch.gate = newGate
	case SQUARE_ATK, TRI_ATK, SINE_ATK, NOISE_ATK:
		ch.attackTime = max(int(value*MS_TO_SAMPLES), MIN_ENV_TIME)
	case SQUARE_DEC, TRI_DEC, SINE_DEC, NOISE_DEC:
		ch.decayTime = max(int(value*MS_TO_SAMPLES), MIN_ENV_TIME)
	case SQUARE_SUS, TRI_SUS, SINE_SUS, NOISE_SUS:
		ch.sustainLevel = float32(value) / NORMALISE_8BIT
	case SQUARE_REL, TRI_REL, SINE_REL, NOISE_REL:
		ch.releaseTime = max(int(value*MS_TO_SAMPLES), MIN_ENV_TIME)
	case NOISE_MODE:
		ch.noiseMode = int(value % NUM_NOISE_MODES) // 0=white, 1=periodic, 2=metallic
	case ENV_SHAPE:
		ch.envelopeShape = int(value % NUM_ENVELOPE_SHAPES) // 0=ADSR, 1=SawUp, 2=SawDown, 3=Loop
		// Reset envelope state
		ch.envelopePhase = ENV_ATTACK
		ch.envelopeSample = 0
	case SQUARE_SWEEP, TRI_SWEEP, SINE_SWEEP, NOISE_SWEEP:
		ch.sweepEnabled = (value & SWEEP_ENABLE_MASK) != 0
		ch.sweepPeriod = int((value >> SWEEP_PERIOD_SHIFT) & SWEEP_PERIOD_MASK) // Extract bits 4-6
		ch.sweepShift = uint(value & SWEEP_SHIFT_MASK)                          // Extract bits 0-2 for shift
		if ch.sweepShift == 0 {
			ch.sweepShift = MIN_SWEEP_SHIFT // Prevent divide by zero
		}
		ch.sweepDirection = (value & SWEEP_DIR_MASK) != 0
	case SYNC_SOURCE_CH0, SYNC_SOURCE_CH1, SYNC_SOURCE_CH2, SYNC_SOURCE_CH3:
		// Determine target channel (e.g., SYNC_SOURCE_CH0 → channel 0)
		chIndex := (addr - SYNC_SOURCE_CH0) / SYNC_REG_SPACING
		ch := chip.channels[chIndex]
		// Set sync source to another channel (0–3)
		masterIndex := int(value % NUM_CHANNELS)
		ch.syncSource = chip.channels[masterIndex]
	case RING_MOD_SOURCE_CH0, RING_MOD_SOURCE_CH1, RING_MOD_SOURCE_CH2, RING_MOD_SOURCE_CH3:
		chIndex := (addr - RING_MOD_SOURCE_CH0) / RINGMOD_REG_SPACING
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
			} else {
				ch.envelopeLevel += MAX_LEVEL / float32(ch.attackTime)
				if ch.envelopeLevel >= MAX_LEVEL {
					ch.envelopeLevel = MAX_LEVEL
					ch.envelopePhase = ENV_DECAY
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
			ch.envelopeLevel *= MAX_LEVEL - float32(ch.envelopeSample)/float32(ch.releaseTime)
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
				ch.envelopeLevel *= MAX_LEVEL - float32(ch.envelopeSample)/float32(ch.releaseTime)
				ch.envelopeSample++
				if ch.envelopeSample >= ch.releaseTime {
					ch.envelopeLevel = 0
					ch.enabled = false
				}
			}
		}
	default:
		panic("unhandled default case")
	}
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

	// Frequency sweep logic
	if ch.sweepEnabled {
		ch.sweepCounter++
		if ch.sweepCounter >= ch.sweepPeriod {
			// Calculate delta per sample instead of per period
			delta := (ch.frequency / float32(int(1)<<ch.sweepShift)) / float32(ch.sweepPeriod*SWEEP_RATE)
			if ch.sweepDirection {
				ch.frequency += delta
			} else {
				if delta > ch.frequency {
					ch.frequency = 0
				} else {
					ch.frequency -= delta
				}
			}
			if ch.frequency < MIN_FILTER_FREQ {
				ch.frequency = MIN_FILTER_FREQ
			} else if ch.frequency > MAX_FREQ {
				ch.frequency = MAX_FREQ
			}
			ch.sweepCounter = 0
		}
	}

	var rawSample float32
	referenceFreq := float32(ch.frequency) * REF_FREQ / FREQ_REF // Map register value to Hz
	phaseInc := referenceFreq * PHASE_INC_FACTOR

	switch ch.waveType {
	case WAVE_SQUARE:
		currentDuty := ch.dutyCycle
		if ch.pwmEnabled {
			ch.pwmPhase += ch.pwmRate * (TWO_PI / SAMPLE_RATE)
			ch.pwmPhase = float32(math.Mod(float64(ch.pwmPhase), 2*math.Pi))
			normalisedPhase := ch.pwmPhase / TWO_PI // yields a [0,1] value
			lfo := float32(math.Abs(float64(normalisedPhase*NORMALISE_SCALE-NORMALISE_OFFSET)))*NORMALISE_SCALE - NORMALISE_OFFSET
			currentDuty = ch.dutyCycle + lfo*ch.pwmDepth
			if currentDuty < 0 {
				currentDuty = 0
			} else if currentDuty > 1 {
				currentDuty = 1
			}
		}
		threshold := TWO_PI * currentDuty
		if ch.phase < threshold {
			rawSample = SQUARE_AMPLITUDE
		} else {
			rawSample = -SQUARE_AMPLITUDE
		}
	case WAVE_TRIANGLE:
		rawSample = TRIANGLE_SCALE*float32(math.Abs(float64(TRIANGLE_PHASE_MULTIPLIER*(ch.phase/TWO_PI)-TRIANGLE_PHASE_SUBTRACT))) - TRIANGLE_OUTPUT_OFFSET
	case WAVE_SINE:
		rawSample = float32(math.Sin(float64(ch.phase)))
	case WAVE_NOISE:
		noisePhaseInc := ch.frequency / SAMPLE_RATE
		ch.noisePhase += noisePhaseInc
		steps := int(ch.noisePhase)
		ch.noisePhase -= float32(steps)

		// Process multiple LFSR steps if needed (for high frequencies)
		for i := 0; i < steps; i++ {
			switch ch.noiseMode {
			case NOISE_MODE_WHITE:
				// Using taps 23,18 for maximal-length sequence (period: 2^23-1)
				newBit := ((ch.noiseSR >> NOISE_TAP1) ^ (ch.noiseSR >> NOISE_TAP2)) & 1
				ch.noiseSR = ((ch.noiseSR << LSB_MASK) | newBit) & NOISE_LFSR_MASK
			case NOISE_MODE_PERIODIC:
				// Simple bit rotation for repeating patterns
				ch.noiseSR = ((ch.noiseSR >> LSB_MASK) | ((ch.noiseSR & 1) << (NOISE_LFSR_BITS - 1))) & NOISE_LFSR_MASK
			case NOISE_MODE_METALLIC:
				// XOR taps 23,15 for metallic tone with longer period
				newBit := ((ch.noiseSR >> METAL_TAP1) ^ (ch.noiseSR >> METAL_TAP2)) & 1
				ch.noiseSR = ((ch.noiseSR << LSB_MASK) | newBit) & NOISE_LFSR_MASK
			}
		}

		ch.noiseValue = float32(ch.noiseSR&LSB_MASK)*NOISE_BIT_SCALE - NOISE_BIAS
		ch.noiseFilterState = NOISE_FILTER_OLD*ch.noiseFilterState + NOISE_FILTER_NEW*ch.noiseValue
		rawSample = ch.noiseFilterState
	}

	// Ring modulation
	if ch.ringModSource != nil {
		rawSample *= ch.ringModSource.prevRawSample
	}
	ch.prevRawSample = rawSample

	// Phase update (skip for noise)
	if ch.waveType != 3 {
		ch.phase += phaseInc
		if ch.phase >= TWO_PI {
			ch.phase -= TWO_PI
			ch.phaseWrapped = true
		} else {
			ch.phaseWrapped = false
		}
	}

	// Oscillator sync
	if ch.syncSource != nil && ch.syncSource.phaseWrapped && ch.waveType != 3 {
		ch.phase = 0
	}

	return rawSample * ch.volume * ch.envelopeLevel
}

func (chip *SoundChip) GenerateSample() float32 {
	// ------------------------------------------------------------------------------
	// GenerateSample generates a single audio sample by processing all active channels
	// through the following signal chain:
	//
	// 1. Channel Generation and Mixing
	//    - Each enabled channel generates its raw waveform (square/triangle/sine/noise)
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
	var sample float32
	for i := 0; i < NUM_CHANNELS; i++ {
		ch := channels[i]
		if ch.enabled {
			sample += ch.generateSample() * CHANNEL_MIX_LEVEL
		}
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

		// Apply 2-pole state variable filter
		cutoff := modulatedCutoff * CUTOFF_FACTOR
		resonance := filterResonance * MAX_RESONANCE

		lp := filterLP + cutoff*filterBP
		hp := (sample - lp) - resonance*filterBP
		bp := filterBP + cutoff*hp

		// Clamp filter outputs
		lp = float32(math.Max(math.Min(float64(lp), MAX_SAMPLE), MIN_SAMPLE))
		bp = float32(math.Max(math.Min(float64(bp), MAX_SAMPLE), MIN_SAMPLE))
		hp = float32(math.Max(math.Min(float64(hp), MAX_SAMPLE), MIN_SAMPLE))

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

	// Apply overdrive effect
	if overdriveLevel > 0 {
		driven := sample * overdriveLevel
		sample = float32(math.Tanh(float64(driven)))
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
	return chip.GenerateSample()
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
