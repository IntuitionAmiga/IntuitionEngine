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

type Channel struct {
	waveType   int
	frequency  float32
	volume     float32
	enabled    bool
	phase      float32
	noiseValue float32

	// Noise generation
	noiseSR     uint32
	noisePhase  float32
	noiseFilter float32

	// Envelope
	envelopePhase  int
	attackTime     int
	decayTime      int
	sustainLevel   float32
	releaseTime    int
	envelopeSample int
	envelopeLevel  float32
	gate           bool
	envelopeShape  int

	// PWM control (square wave only)
	dutyCycle  float32
	pwmEnabled bool    // PWM modulation on/off
	pwmRate    float32 // LFO frequency (Hz)
	pwmDepth   float32 // Modulation depth (0.0–1.0)
	pwmPhase   float32 // LFO phase accumulator

	// Filter states
	noiseFilterState float32

	// Noise type
	noiseMode int

	// Sweep control
	sweepEnabled   bool
	sweepPeriod    int
	sweepShift     uint
	sweepDirection bool // True = up, false = down
	sweepCounter   int  // Tracks sweep timing

	// Modulation
	ringModulate  bool
	ringModSource *Channel // Pointer to the modulating channel
	prevRawSample float32  // Stores last raw waveform for modulation

	syncSource   *Channel // Master oscillator to sync to
	phaseWrapped bool     // Flag for phase reset tracking
}

type CombFilter struct {
	buffer []float32
	pos    int
	decay  float32
}

type SoundChip struct {
	channels [4]*Channel
	enabled  bool
	mutex    sync.RWMutex
	output   AudioOutput

	// Global filter settings
	filterCutoff    float32 // 0–1 (normalized)
	filterResonance float32 // 0–1 (normalized)
	filterType      int     // 0=off, 1=low-pass, 2=high-pass, 3=band-pass
	filterLP        float32 // Low-pass state
	filterBP        float32 // Band-pass state
	filterHP        float32 // High-pass state

	// Filter modulation
	filterModSource *Channel // Channel modulating the filter cutoff
	filterModAmount float32  // Modulation depth (0.0–1.0)

	// TB-303 style Overdrive control
	overdriveLevel float32

	// Reverb effect
	reverbMix   float32
	combFilters [4]CombFilter
	allpassBuf  [2][]float32
	allpassPos  [2]int
}

const (
	COMB_DELAY_1  = 1687
	COMB_DELAY_2  = 1601
	COMB_DELAY_3  = 2053
	COMB_DELAY_4  = 2251
	ALLPASS_DELAY = 389
)

func NewSoundChip(backend int) (*SoundChip, error) {
	// Initialize sound chip with default settings
	chip := &SoundChip{
		filterLP: 0.0,
		filterBP: 0.0,
		filterHP: 0.0,
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
			noiseSR:       0xACE1, // Initial seed for noise
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
			decay:  0.95,
		}
	}

	// Initialize allpass filters
	for i := range chip.allpassBuf {
		chip.allpassBuf[i] = make([]float32, ALLPASS_DELAY)
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
		ch.pwmEnabled = (value & 0x80) != 0    // Bit 7 = enable
		ch.pwmRate = float32(value&0x7F) * 0.1 // Rate: 0–12.7 Hz (7 bits)
	case SQUARE_DUTY:
		value16 := uint16(value & 0xFFFF) // Ensure 16-bit value
		ch.dutyCycle = float32(value16&0xFF) / 256.0
		ch.pwmDepth = float32((value16>>8)&0xFF) / 256.0
	case SQUARE_FREQ, TRI_FREQ, SINE_FREQ, NOISE_FREQ:
		ch.frequency = float32(value)
	case SQUARE_VOL, TRI_VOL, SINE_VOL, NOISE_VOL:
		ch.volume = float32(value&0xFF) / 256.0 // Mask to 8 bits
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
		ch.attackTime = max(int(value*SAMPLE_RATE/1000), 1)
	case SQUARE_DEC, TRI_DEC, SINE_DEC, NOISE_DEC:
		ch.decayTime = max(int(value*SAMPLE_RATE/1000), 1)
	case SQUARE_SUS, TRI_SUS, SINE_SUS, NOISE_SUS:
		ch.sustainLevel = float32(value) / 256.0
	case SQUARE_REL, TRI_REL, SINE_REL, NOISE_REL:
		ch.releaseTime = max(int(value*SAMPLE_RATE/1000), 1)
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
		chip.filterCutoff = float32(value) / 256.0
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
		chip.filterModAmount = float32(value) / 256.0
	case OVERDRIVE_CTRL:
		chip.overdriveLevel = float32(value) / 256.0 * 4.0 // 0.0-4.0 gain
	case REVERB_MIX:
		chip.reverbMix = float32(value) / 256.0
	case REVERB_DECAY:
		baseDecay := 0.1 + (float32(value)/256.0)*0.89
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
			if ch.frequency > 20000 {
				ch.frequency = 20000
			}
			ch.sweepCounter = 0
		}
	}

	var rawSample float32
	phaseInc := ch.frequency * (2 * math.Pi / SAMPLE_RATE)

	switch ch.waveType {
	case 0: // Square wave
		currentDuty := ch.dutyCycle
		if ch.pwmEnabled {
			ch.pwmPhase += ch.pwmRate * (2 * math.Pi / SAMPLE_RATE)
			if ch.pwmPhase >= 2*math.Pi {
				ch.pwmPhase -= 2 * math.Pi
			}
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
			rawSample = 1.0
		} else {
			rawSample = -1.0
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
				// XOR bits 0 and 3 (Galois LFSR)
				newBit := ((ch.noiseSR & 1) ^ ((ch.noiseSR >> 3) & 1)) & 1
				ch.noiseSR = (ch.noiseSR >> 1) | (newBit << 16)
			case NOISE_MODE_PERIODIC:
				// Rotate bits (periodic noise)
				ch.noiseSR = (ch.noiseSR >> 1) | ((ch.noiseSR & 1) << 16)
			case NOISE_MODE_METALLIC:
				// XOR bits 0 and 2 (metallic noise)
				newBit := ((ch.noiseSR & 1) ^ ((ch.noiseSR >> 2) & 1)) & 1
				ch.noiseSR = (ch.noiseSR >> 1) | (newBit << 16)
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
			sample += ch.generateSample() * 0.25
		}
	}

	// Apply global filter first
	if chip.filterType != 0 && chip.filterCutoff > 0 {
		// Default to no modulation
		modulatedCutoff := chip.filterCutoff
		// Apply modulation if enabled
		if chip.filterModSource != nil {
			modSignal := chip.filterModSource.prevRawSample * chip.filterModAmount * 2.0
			const MAX_CUTOFF = 0.95 // Stay below Nyquist
			modulatedCutoff = chip.filterCutoff + modSignal
			modulatedCutoff = float32(math.Max(math.Min(float64(modulatedCutoff), MAX_CUTOFF), 0.0))
		}

		// Convert cutoff to Hz and apply resonance
		cutoff := float32(2.0*math.Pi) * modulatedCutoff * 20000.0 / SAMPLE_RATE
		const MAX_RESONANCE = 4.0
		resonance := chip.filterResonance * MAX_RESONANCE

		// 2-pole resonant filter (state variable)
		lp := chip.filterLP + cutoff*chip.filterBP
		hp := (sample - lp) - resonance*chip.filterBP
		bp := chip.filterBP + cutoff*hp

		// Clamp to prevent overflow
		lp = float32(math.Max(float64(lp), -1.0))
		lp = float32(math.Min(float64(lp), 1.0))
		bp = float32(math.Max(float64(bp), -1.0))
		bp = float32(math.Min(float64(bp), 1.0))
		hp = float32(math.Max(float64(hp), -1.0))
		hp = float32(math.Min(float64(hp), 1.0))

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
		driven := sample * chip.overdriveLevel * 2.0
		sample = float32(math.Tanh(float64(driven)))
	}

	// Apply reverb
	wet := chip.applyReverb(sample)
	sample = sample*(1-chip.reverbMix) + wet*chip.reverbMix

	return sample
}
func (chip *SoundChip) applyReverb(input float32) float32 {
	var out float32
	for i := 0; i < 4; i++ {
		comb := &chip.combFilters[i]
		delayed := comb.buffer[comb.pos]
		comb.buffer[comb.pos] = input + delayed*comb.decay
		out += delayed
		comb.pos = (comb.pos + 1) % len(comb.buffer)
	}

	// Allpass filters remain unchanged
	for i := 0; i < 2; i++ {
		pos := chip.allpassPos[i]
		buf := chip.allpassBuf[i]
		delayed := buf[pos]
		buf[pos] = out + delayed*0.5
		out = delayed - out
		chip.allpassPos[i] = (pos + 1) % len(buf)
	}

	return out * 0.3
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
