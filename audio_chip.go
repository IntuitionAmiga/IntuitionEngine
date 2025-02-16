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
License: GPLv3 or later
*/

package main

import (
	"math"
	"sync"
)

const (
	// Square wave: F900-F93F
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

	// Triangle wave: F940-F97F
	TRI_FREQ  = 0xF940
	TRI_VOL   = 0xF944
	TRI_CTRL  = 0xF948
	TRI_ATK   = 0xF960
	TRI_DEC   = 0xF964
	TRI_SUS   = 0xF968
	TRI_REL   = 0xF96C
	TRI_SWEEP = 0xF914

	// Sine wave: F980-F9BF
	SINE_FREQ  = 0xF980
	SINE_VOL   = 0xF984
	SINE_CTRL  = 0xF988
	SINE_ATK   = 0xF990
	SINE_DEC   = 0xF994
	SINE_SUS   = 0xF998
	SINE_REL   = 0xF99C
	SINE_SWEEP = 0xF918

	// Noise: F9C0-F9FF
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

	SYNC_SOURCE_CH0 = 0xFA00 // Sync source for channel 0
	SYNC_SOURCE_CH1 = 0xFA04 // Sync source for channel 1
	SYNC_SOURCE_CH2 = 0xFA08 // Sync source for channel 2
	SYNC_SOURCE_CH3 = 0xFA0C // Sync source for channel 3

	RING_MOD_SOURCE_CH0 = 0xFA10 // Ring mod source for channel 0
	RING_MOD_SOURCE_CH1 = 0xFA14 // Channel 1
	RING_MOD_SOURCE_CH2 = 0xFA18 // Channel 2
	RING_MOD_SOURCE_CH3 = 0xFA1C // Channel 3

	FILTER_CUTOFF     = 0xF820 // Filter cutoff (0–255 → 0.0–1.0)
	FILTER_RESONANCE  = 0xF824 // Filter resonance/Q (0–255 → 0.0–1.0)
	FILTER_TYPE       = 0xF828 // 0=off, 1=low-pass, 2=high-pass, 3=band-pass
	FILTER_MOD_SOURCE = 0xF82C // Register to set modulation source (channel 0–3)
	FILTER_MOD_AMOUNT = 0xF830 // Register to set modulation depth (0–255 → 0.0–1.0)

	OVERDRIVE_CTRL = 0xFA40 // Drive amount (0-255 → 0.0-4.0)

	REVERB_MIX   = 0xFA50 // 0-255 → 0.0-1.0 (dry/wet)
	REVERB_DECAY = 0xFA54 // 0-255 → 0.1-0.99 (tail length)

	AUDIO_CTRL = 0xF800

	SAMPLE_RATE = 44100
)
const (
	ENV_ATTACK = iota
	ENV_DECAY
	ENV_SUSTAIN
	ENV_RELEASE
	ENV_SHAPE          = 0xF804
	ENV_SHAPE_ADSR     = 0 // Default ADSR
	ENV_SHAPE_SAW_UP   = 1 // Linear rise to 1.0, then hold
	ENV_SHAPE_SAW_DOWN = 2 // Linear fall to 0.0, then hold
	ENV_SHAPE_LOOP     = 3 // ADSR but loops after release
)
const (
	NORMALIZE_8BIT = 255.0 // For 8-bit value normalization (0-255)
	PWM_RANGE      = 256.0 // Keep 256 for duty cycle since it's used as a power of 2
	FREQ_REF       = 256.0 // Keep 256 for frequency reference
)
const (
	MS_TO_SAMPLES = SAMPLE_RATE / 1000 // Convert milliseconds to samples
	MIN_ENV_TIME  = 1                  // Minimum envelope time
)
const (
	MAX_FILTER_CUTOFF = 0.95  // Maximum filter cutoff frequency
	MAX_RESONANCE     = 4.0   // Maximum filter resonance
	MAX_FREQ          = 20000 // Maximum frequency in Hz
)
const (
	MAX_SAMPLE = 1.0
	MIN_SAMPLE = -1.0
)
const (
	CHANNEL_MIX_LEVEL  = 0.25 // 1/4 for 4 channels
	REVERB_ATTENUATION = 0.3  // Reverb output scaling
)
const PWM_RATE_SCALE = 0.1 // Convert 7-bit value to Hz range 0-12.7

type Channel struct {
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
	waveType       int  // Oscillator type (0=square, 1=triangle, etc)
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

	// Boolean state flags (packed together to minimize padding)
	enabled        bool    // Channel enabled flag
	gate           bool    // Gate/trigger state
	sweepEnabled   bool    // Frequency sweep enabled
	sweepDirection bool    // Sweep direction (up/down)
	pwmEnabled     bool    // PWM enabled flag
	phaseWrapped   bool    // Phase wrap indicator
	_pad           [2]byte // Padding for alignment
}
type CombFilter struct {
	buffer []float32 // Delay line buffer
	decay  float32   // Decay coefficient
	pos    int       // Current buffer position
	_pad   [4]byte   // Align to 8-byte boundary
}

type SoundChip struct {
	// Cache line 1 - Hot path DSP state (64 bytes)
	filterLP        float32 // Current low-pass filter state
	filterBP        float32 // Current band-pass filter state
	filterHP        float32 // Current high-pass filter state
	filterCutoff    float32 // Normalized filter cutoff frequency (0-1)
	filterResonance float32 // Filter resonance/Q factor (0-1)
	filterModAmount float32 // Filter modulation depth (0-1)
	overdriveLevel  float32 // Overdrive distortion amount (0-4)
	reverbMix       float32 // Reverb wet/dry mix ratio (0-1)
	filterType      int     // Filter mode (0=off, 1=LP, 2=HP, 3=BP)
	enabled         bool    // Global chip enable flag
	_pad1           [7]byte // Align to 64-byte cache line boundary

	// Cache line 2 - Channel references and thread safety (64 bytes)
	channels        [4]*Channel  // Array of 4 audio channel pointers
	filterModSource *Channel     // Channel modulating the filter cutoff
	mutex           sync.RWMutex // Concurrency control for parameter updates
	_pad2           [8]byte      // Align to 64-byte cache line boundary

	// Cache line 3+ - Reverb state (cold path)
	preDelayPos int           // Current position in pre-delay buffer
	allpassPos  [2]int        // Current positions in allpass buffers
	combFilters [4]CombFilter // Parallel comb filter bank for reverb
	allpassBuf  [2][]float32  // Allpass diffusion filters
	preDelayBuf []float32     // 8ms pre-delay buffer
	output      AudioOutput   // Audio backend interface
}

const PRE_DELAY_MS = 8 // 8ms pre-delay

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
const (
	ALLPASS_DELAY_1 = 389
	ALLPASS_DELAY_2 = 307
	ALLPASS_COEF_1  = 0.7
	ALLPASS_COEF_2  = 0.5
)

const ALLPASS_COEF = 0.5 // Standard allpass coefficient for optimal diffusion

const (
	NOISE_LFSR_SEED = 0x7FFFFF // 23-bit LFSR seed
	NOISE_LFSR_MASK = 0x7FFFFF // 23-bit mask
)

func NewSoundChip(backend int) (*SoundChip, error) {
	// Initialize sound chip with default settings
	chip := &SoundChip{
		filterLP:    0.0,
		filterBP:    0.0,
		filterHP:    0.0,
		preDelayBuf: make([]float32, PRE_DELAY_MS*SAMPLE_RATE/1000),
	}

	// Initialize channels
	for i := 0; i < 4; i++ {
		chip.channels[i] = &Channel{
			waveType:      i,
			attackTime:    44,
			decayTime:     0,
			sustainLevel:  1.0,
			releaseTime:   44,
			envelopePhase: ENV_ATTACK,
			noiseSR:       NOISE_LFSR_SEED, // Initial seed for noise
			dutyCycle:     0.5,
		}
	}

	// Initialize audio output
	output, err := NewAudioOutput(backend, SAMPLE_RATE, chip)
	if err != nil {
		return nil, err
	}
	chip.output = output

	// Initialize comb filters
	combLengths := []int{COMB_DELAY_1, COMB_DELAY_2, COMB_DELAY_3, COMB_DELAY_4}
	for i := range chip.combFilters {
		chip.combFilters[i] = CombFilter{
			buffer: make([]float32, combLengths[i]),
			decay:  []float32{COMB_DECAY_1, COMB_DECAY_2, COMB_DECAY_3, COMB_DECAY_4}[i],
		}
	}

	// Initialize allpass filters
	for i := range chip.allpassBuf {
		chip.allpassBuf[i] = make([]float32, []int{ALLPASS_DELAY_1, ALLPASS_DELAY_2}[i])
	}

	return chip, nil
}

func (chip *SoundChip) HandleRegisterWrite(addr uint32, value uint32) {
	chip.mutex.Lock()
	defer chip.mutex.Unlock()

	if addr == AUDIO_CTRL {
		chip.enabled = value != 0
		return
	}

	var ch *Channel
	switch {
	case addr >= 0xF900 && addr <= 0xF93F:
		ch = chip.channels[0]
	case addr >= 0xF940 && addr <= 0xF97F:
		ch = chip.channels[1]
	case addr >= 0xF980 && addr <= 0xF9BF:
		ch = chip.channels[2]
	case addr >= 0xF9C0 && addr <= 0xF9FF:
		ch = chip.channels[3]
	}

	switch addr {
	case SQUARE_PWM_CTRL:
		ch.pwmEnabled = (value & 0x80) != 0               // Bit 7 = enable
		ch.pwmRate = float32(value&0x7F) * PWM_RATE_SCALE // Rate: 0–12.7 Hz (7 bits)
	case SQUARE_DUTY:
		value16 := uint16(value & 0xFFFF) // Ensure 16-bit value
		ch.dutyCycle = float32(value16&0xFF) / PWM_RANGE
		ch.pwmDepth = float32((value16>>8)&0xFF) / PWM_RANGE
	case SQUARE_FREQ, TRI_FREQ, SINE_FREQ, NOISE_FREQ:
		ch.frequency = float32(value)
	case SQUARE_VOL, TRI_VOL, SINE_VOL, NOISE_VOL:
		ch.volume = float32(value&0xFF) / NORMALIZE_8BIT
	case SQUARE_CTRL, TRI_CTRL, SINE_CTRL, NOISE_CTRL:
		ch.enabled = value != 0
		newGate := value&0x02 != 0

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
		ch.sustainLevel = float32(value) / NORMALIZE_8BIT
	case SQUARE_REL, TRI_REL, SINE_REL, NOISE_REL:
		ch.releaseTime = max(int(value*MS_TO_SAMPLES), MIN_ENV_TIME)
	case NOISE_MODE:
		ch.noiseMode = int(value % 3) // 0=white, 1=periodic, 2=metallic
	case ENV_SHAPE:
		ch.envelopeShape = int(value % 4) // 0=ADSR, 1=SawUp, 2=SawDown, 3=Loop
		// Reset envelope state
		ch.envelopePhase = ENV_ATTACK
		ch.envelopeSample = 0
	case SQUARE_SWEEP, TRI_SWEEP, SINE_SWEEP, NOISE_SWEEP:
		ch.sweepEnabled = (value & 0x80) != 0
		ch.sweepPeriod = int((value >> 4) & 0x07) // Extract bits 4-6 properly
		ch.sweepShift = uint((value & 0x07))      // Extract bits 0-2 for shift
		if ch.sweepShift == 0 {
			ch.sweepShift = 1 // Prevent divide by zero
		}
		ch.sweepDirection = (value & 0x08) != 0
	case SYNC_SOURCE_CH0, SYNC_SOURCE_CH1, SYNC_SOURCE_CH2, SYNC_SOURCE_CH3:
		// Determine target channel (e.g., SYNC_SOURCE_CH0 → channel 0)
		chIndex := (addr - SYNC_SOURCE_CH0) / 4
		ch := chip.channels[chIndex]
		// Set sync source to another channel (0–3)
		masterIndex := int(value % 4)
		ch.syncSource = chip.channels[masterIndex]
	case RING_MOD_SOURCE_CH0, RING_MOD_SOURCE_CH1, RING_MOD_SOURCE_CH2, RING_MOD_SOURCE_CH3:
		chIndex := (addr - RING_MOD_SOURCE_CH0) / 4
		ch := chip.channels[chIndex]
		masterIndex := int(value % 4)
		ch.ringModSource = chip.channels[masterIndex]
	case FILTER_CUTOFF:
		chip.filterCutoff = float32(value) / NORMALIZE_8BIT
	case FILTER_RESONANCE:
		chip.filterResonance = float32(value) / 256.0
	case FILTER_TYPE:
		chip.filterType = int(value % 4)
	case FILTER_MOD_SOURCE:
		// Set modulation source to one of the 4 channels
		sourceIndex := int(value % 4)
		chip.filterModSource = chip.channels[sourceIndex]
	case FILTER_MOD_AMOUNT:
		// Normalize modulation depth to 0.0–1.0
		chip.filterModAmount = float32(value) / NORMALIZE_8BIT
	case OVERDRIVE_CTRL:
		chip.overdriveLevel = float32(value) / 256.0 * 4.0 // 0.0-4.0 gain
	case REVERB_MIX:
		chip.reverbMix = float32(value) / NORMALIZE_8BIT
	case REVERB_DECAY:
		baseDecay := 0.1 + (float32(value)/NORMALIZE_8BIT)*0.89
		chip.combFilters[0].decay = baseDecay * 0.97
		chip.combFilters[1].decay = baseDecay * 0.95
		chip.combFilters[2].decay = baseDecay * 0.93
		chip.combFilters[3].decay = baseDecay * 0.91
	}
}

func (ch *Channel) updateEnvelope() {
	switch ch.envelopePhase {
	case ENV_ATTACK:
		switch ch.envelopeShape {
		case ENV_SHAPE_SAW_UP:
			if ch.attackTime <= 0 {
				ch.envelopeLevel = 1.0
				ch.envelopePhase = ENV_SUSTAIN
			} else {
				ch.envelopeLevel = float32(ch.envelopeSample) / float32(ch.attackTime)
				ch.envelopeSample++
				if ch.envelopeSample >= ch.attackTime {
					ch.envelopeLevel = 1.0
					ch.envelopePhase = ENV_SUSTAIN
				}
			}
		case ENV_SHAPE_SAW_DOWN:
			if ch.attackTime <= 0 {
				ch.envelopeLevel = 0.0
				ch.envelopePhase = ENV_SUSTAIN
			} else {
				ch.envelopeLevel = 1.0 - float32(ch.envelopeSample)/float32(ch.attackTime)
				ch.envelopeSample++
				if ch.envelopeSample >= ch.attackTime {
					ch.envelopeLevel = 0.0
					ch.envelopePhase = ENV_SUSTAIN
				}
			}
		default: // Default ADSR logic
			if ch.attackTime <= 0 {
				ch.envelopeLevel = 1.0
				ch.envelopePhase = ENV_DECAY
			} else {
				ch.envelopeLevel += 1.0 / float32(ch.attackTime)
				if ch.envelopeLevel >= 1.0 {
					ch.envelopeLevel = 1.0
					ch.envelopePhase = ENV_DECAY
				}
			}
		}

	case ENV_DECAY:
		if ch.decayTime <= 0 {
			ch.envelopeLevel = ch.sustainLevel
			ch.envelopePhase = ENV_SUSTAIN
		} else {
			ch.envelopeLevel = 1.0 - ((1.0 - ch.sustainLevel) * float32(ch.envelopeSample) / float32(ch.decayTime))
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
			ch.envelopeLevel *= (1.0 - float32(ch.envelopeSample)/float32(ch.releaseTime))
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
				ch.envelopeLevel *= (1.0 - float32(ch.envelopeSample)/float32(ch.releaseTime))
				ch.envelopeSample++
				if ch.envelopeSample >= ch.releaseTime {
					ch.envelopeLevel = 0
					ch.enabled = false
				}
			}
		}
	}
}
func (ch *Channel) generateSample() float32 {
	if !ch.enabled || ch.frequency == 0 {
		return 0
	}

	ch.updateEnvelope()

	// Frequency sweep logic
	if ch.sweepEnabled {
		ch.sweepCounter++
		if ch.sweepCounter >= ch.sweepPeriod {
			// Calculate delta per sample instead of per period
			delta := (ch.frequency / float32(int(1)<<ch.sweepShift)) / float32(ch.sweepPeriod*4000)
			if ch.sweepDirection {
				ch.frequency += delta
			} else {
				if delta > ch.frequency {
					ch.frequency = 0
				} else {
					ch.frequency -= delta
				}
			}
			if ch.frequency > MAX_FREQ {
				ch.frequency = MAX_FREQ
			}
			ch.sweepCounter = 0
		}
	}

	var rawSample float32
	referenceFreq := float32(ch.frequency) * 440.0 / FREQ_REF // Map register value to Hz
	phaseInc := referenceFreq * (2 * math.Pi / SAMPLE_RATE)

	switch ch.waveType {
	case 0: // Square wave
		currentDuty := ch.dutyCycle
		if ch.pwmEnabled {
			ch.pwmPhase += ch.pwmRate * (2 * math.Pi / SAMPLE_RATE)
			ch.pwmPhase = float32(math.Mod(float64(ch.pwmPhase), 2*math.Pi))
			lfo := float32(math.Abs(float64(2*(ch.pwmPhase/(2*math.Pi))-1)))*2 - 1
			currentDuty = ch.dutyCycle + lfo*ch.pwmDepth
			if currentDuty < 0 {
				currentDuty = 0
			} else if currentDuty > 1 {
				currentDuty = 1
			}
		}
		threshold := 2 * math.Pi * currentDuty
		if ch.phase < threshold {
			rawSample = 4.0
		} else {
			rawSample = -4.0
		}

	case 1: // Triangle
		rawSample = 2.0*float32(math.Abs(float64(2.0*(ch.phase/(2*math.Pi))-1.0))) - 1.0
	case 2: // Sine
		rawSample = float32(math.Sin(float64(ch.phase)))

	case 3: // Noise
		noisePhaseInc := ch.frequency / SAMPLE_RATE
		ch.noisePhase += noisePhaseInc
		steps := int(ch.noisePhase)
		ch.noisePhase -= float32(steps)

		// Process multiple LFSR steps if needed (for high frequencies)
		for i := 0; i < steps; i++ {
			switch ch.noiseMode {
			case NOISE_MODE_WHITE:
				// Using taps 23,18 for maximal-length sequence (period: 2^23-1)
				newBit := ((ch.noiseSR >> 22) ^ (ch.noiseSR >> 17)) & 1
				ch.noiseSR = ((ch.noiseSR << 1) | newBit) & NOISE_LFSR_MASK
			case NOISE_MODE_PERIODIC:
				// Simple bit rotation for repeating patterns
				ch.noiseSR = ((ch.noiseSR >> 1) | ((ch.noiseSR & 1) << 22)) & NOISE_LFSR_MASK
			case NOISE_MODE_METALLIC:
				// XOR taps 23,15 for metallic tone with longer period
				newBit := ((ch.noiseSR >> 22) ^ (ch.noiseSR >> 14)) & 1
				ch.noiseSR = ((ch.noiseSR << 1) | newBit) & NOISE_LFSR_MASK
			}
		}

		ch.noiseValue = float32(ch.noiseSR&1)*2 - 1
		ch.noiseFilterState = 0.95*ch.noiseFilterState + 0.05*ch.noiseValue
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
		if ch.phase >= 2*math.Pi {
			ch.phase -= 2 * math.Pi
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
	chip.mutex.RLock()
	defer chip.mutex.RUnlock()

	if !chip.enabled {
		return 0
	}

	var sample float32

	// Generate raw samples from each channel
	for i := 0; i < 4; i++ { // Process channels 0, 1, 2, 3
		ch := chip.channels[i]
		if ch.enabled {
			sample += ch.generateSample() * CHANNEL_MIX_LEVEL
		}
	}

	// Apply global filter first
	if chip.filterType != 0 && chip.filterCutoff > 0 {
		// Default to no modulation
		modulatedCutoff := chip.filterCutoff
		// Apply modulation if enabled
		if chip.filterModSource != nil {
			modSignal := chip.filterModSource.prevRawSample * chip.filterModAmount
			modulatedCutoff = chip.filterCutoff + modSignal
			modulatedCutoff = float32(math.Max(math.Min(float64(modulatedCutoff), MAX_FILTER_CUTOFF), 0.0))
		}

		// Convert cutoff to Hz and apply resonance
		cutoff := float32(2.0*math.Pi) * modulatedCutoff * 20000.0 / SAMPLE_RATE
		resonance := chip.filterResonance * MAX_RESONANCE

		// 2-pole resonant filter (state variable)
		lp := chip.filterLP + cutoff*chip.filterBP
		hp := (sample - lp) - resonance*chip.filterBP
		bp := chip.filterBP + cutoff*hp

		// Clamp to prevent overflow
		lp = float32(math.Max(math.Min(float64(lp), MAX_SAMPLE), MIN_SAMPLE))
		bp = float32(math.Max(math.Min(float64(bp), MAX_SAMPLE), MIN_SAMPLE))
		hp = float32(math.Max(math.Min(float64(hp), MAX_SAMPLE), MIN_SAMPLE))

		// Update filter states
		chip.filterLP = lp
		chip.filterBP = bp
		chip.filterHP = hp

		// Output based on filter type
		switch chip.filterType {
		case 1:
			sample = chip.filterLP
		case 2:
			sample = chip.filterHP
		case 3:
			sample = chip.filterBP
		}
	}

	// Apply overdrive after filter
	if chip.overdriveLevel > 0 {
		driven := sample * chip.overdriveLevel
		sample = float32(math.Tanh(float64(driven)))
	}

	// Apply reverb
	wet := chip.applyReverb(sample)
	sample = sample*(1-chip.reverbMix) + wet*chip.reverbMix

	// Clamp final output
	return float32(math.Max(math.Min(float64(sample), 1.0), -1.0))
}
func (chip *SoundChip) applyReverb(input float32) float32 {
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

	// Apply pre-delay
	delayed := chip.preDelayBuf[chip.preDelayPos]
	chip.preDelayBuf[chip.preDelayPos] = input
	chip.preDelayPos = (chip.preDelayPos + 1) % len(chip.preDelayBuf)

	// Process comb filters
	var out float32
	for i := 0; i < 4; i++ {
		comb := &chip.combFilters[i]
		cDelay := comb.buffer[comb.pos]
		comb.buffer[comb.pos] = delayed + cDelay*comb.decay
		out += cDelay
		comb.pos = (comb.pos + 1) % len(comb.buffer)
	}

	// Process allpass filters
	for i := 0; i < 2; i++ {
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
	chip.enabled = false
	chip.output.Stop()
	chip.output.Close()
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
