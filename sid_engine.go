// sid_engine.go - MOS 6581/8580 SID sound chip register mapping for the Intuition Engine

/*
SID (Sound Interface Device) was the legendary audio chip used in the Commodore 64.
This implementation provides pure register mapping to the SoundChip for audio synthesis,
accessible from all CPUs (M68K, IE32, Z80, 6502).

Features:
- 3 audio voices mapped to SoundChip channels 0-2
- 16-bit frequency registers with accurate conversion
- 12-bit pulse width for duty cycle control
- 4 waveform types: Triangle, Sawtooth, Pulse, Noise
- Hardware ADSR envelopes with authentic timing
- Ring modulation and hard sync between voices
- Programmable resonant filter (low/band/high pass)
- SID+ enhanced mode with improved audio quality

The engine translates SID register writes to SoundChip channel parameters.
Synthesis is performed by SoundChip - this module handles only the mapping.
*/

package main

import (
	"math"
	"sync"
)

// SIDEngine emulates the SID sound chip via register mapping to SoundChip
type SIDEngine struct {
	mutex      sync.Mutex
	sound      *SoundChip
	sampleRate int
	clockHz    uint32

	regs [SID_REG_COUNT]uint8

	// Voice state (for gate tracking and envelope)
	voiceGate [3]bool

	enabled        bool
	sidPlusEnabled bool
	channelsInit   bool
}

// SID+ logarithmic volume curve (2dB per step)
var sidPlusVolumeCurve = func() [16]float32 {
	var curve [16]float32
	curve[0] = 0
	for i := 1; i < 16; i++ {
		db := float64(i-15) * 2.0
		curve[i] = float32(math.Pow(10.0, db/20.0))
	}
	curve[15] = 1.0
	return curve
}()

// sidPlusMixGain is defined in audio_chip.go

// NewSIDEngine creates a new SID emulation engine
func NewSIDEngine(sound *SoundChip, sampleRate int) *SIDEngine {
	return &SIDEngine{
		sound:      sound,
		sampleRate: sampleRate,
		clockHz:    SID_CLOCK_PAL,
	}
}

// SetClockHz sets the SID master clock frequency
func (e *SIDEngine) SetClockHz(clock uint32) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if clock == 0 {
		return
	}
	e.clockHz = clock
}

// HandleWrite processes a write to a SID register via memory-mapped I/O
func (e *SIDEngine) HandleWrite(addr uint32, value uint32) {
	if addr < SID_BASE || addr > SID_END {
		return
	}
	reg := uint8(addr - SID_BASE)
	e.WriteRegister(reg, uint8(value))
}

// HandleRead processes a read from a SID register
func (e *SIDEngine) HandleRead(addr uint32) uint32 {
	if addr < SID_BASE || addr > SID_END {
		return 0
	}
	reg := uint8(addr - SID_BASE)
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return uint32(e.regs[reg])
}

// WriteRegister writes a value to a SID register
func (e *SIDEngine) WriteRegister(reg uint8, value uint8) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if reg >= SID_REG_COUNT {
		return
	}

	e.enabled = true
	e.regs[reg] = value

	// Handle SID+ control register
	if reg == 0x19 { // SID_PLUS_CTRL offset
		e.sidPlusEnabled = (value & 1) != 0
		if e.sound != nil {
			e.sound.SetSIDPlusEnabled(e.sidPlusEnabled)
		}
	}

	e.syncToChip()
}

// SetSIDPlusEnabled enables/disables SID+ enhanced mode
func (e *SIDEngine) SetSIDPlusEnabled(enabled bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.sidPlusEnabled = enabled
	if e.sound != nil {
		e.sound.SetSIDPlusEnabled(enabled)
	}
	e.syncToChip()
}

// SIDPlusEnabled returns whether SID+ mode is active
func (e *SIDEngine) SIDPlusEnabled() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.sidPlusEnabled
}

// ensureChannelsInitialized sets up SoundChip channels for SID output
func (e *SIDEngine) ensureChannelsInitialized() {
	if e.channelsInit || e.sound == nil {
		return
	}

	// SID uses channels 0-2 of the SoundChip
	for ch := 0; ch < 3; ch++ {
		e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_TRIANGLE)
		e.writeChannel(ch, FLEX_OFF_DUTY, 0x0080) // 50% duty cycle
		e.writeChannel(ch, FLEX_OFF_PWM_CTRL, 0)
		e.writeChannel(ch, FLEX_OFF_ATK, 0)
		e.writeChannel(ch, FLEX_OFF_DEC, 0)
		e.writeChannel(ch, FLEX_OFF_SUS, 255)
		e.writeChannel(ch, FLEX_OFF_REL, 0)
		e.writeChannel(ch, FLEX_OFF_VOL, 0)
		e.writeChannel(ch, FLEX_OFF_CTRL, 0) // Start with gate off
	}

	e.channelsInit = true
}

// syncToChip updates the SoundChip based on current SID register state
func (e *SIDEngine) syncToChip() {
	if e.sound == nil {
		return
	}

	e.ensureChannelsInitialized()
	e.applyFrequencies()
	e.applyWaveforms()
	e.applyEnvelopes()
	e.applyVolumes()
	e.applyModulation()
	e.applyFilter()
}

// calcFrequency calculates the output frequency for a voice
// SID frequency = Fout * 16777216 / clockHz
// So: Fout = freq * clockHz / 16777216
func (e *SIDEngine) calcFrequency(voice int) float64 {
	if voice < 0 || voice > 2 {
		return 0
	}

	base := voice * 7
	freq := uint16(e.regs[base]) | (uint16(e.regs[base+1]) << 8)
	if freq == 0 {
		return 0
	}

	return float64(freq) * float64(e.clockHz) / 16777216.0
}

// calcPulseWidth calculates the duty cycle from pulse width registers (0.0-1.0)
func (e *SIDEngine) calcPulseWidth(voice int) float32 {
	if voice < 0 || voice > 2 {
		return 0.5
	}

	base := voice * 7
	pw := uint16(e.regs[base+2]) | (uint16(e.regs[base+3]&0x0F) << 8)
	// PW ranges from 0-4095, convert to duty cycle
	// 0 = 0% duty (silent), 2048 = 50% duty, 4095 = ~100% duty
	if pw == 0 {
		return 0
	}
	return float32(pw) / 4095.0
}

// applyFrequencies updates SoundChip frequencies from SID registers
func (e *SIDEngine) applyFrequencies() {
	if e.sound == nil {
		return
	}

	for voice := 0; voice < 3; voice++ {
		freq := e.calcFrequency(voice)
		if freq > 0 && freq <= 20000 {
			e.writeChannel(voice, FLEX_OFF_FREQ, uint32(freq))
		} else {
			e.writeChannel(voice, FLEX_OFF_FREQ, 0)
		}
	}
}

// applyWaveforms sets the waveform type for each voice based on control register
func (e *SIDEngine) applyWaveforms() {
	if e.sound == nil {
		return
	}

	for voice := 0; voice < 3; voice++ {
		base := voice * 7
		ctrl := e.regs[base+4]

		// Determine waveform (multiple can be selected for combined waveforms,
		// but we'll prioritize: noise > pulse > sawtooth > triangle)
		var waveType int
		if ctrl&SID_CTRL_NOISE != 0 {
			waveType = WAVE_NOISE
		} else if ctrl&SID_CTRL_PULSE != 0 {
			waveType = WAVE_SQUARE
			// Apply pulse width as duty cycle
			pw := e.calcPulseWidth(voice)
			e.writeChannel(voice, FLEX_OFF_DUTY, uint32(pw*255))
		} else if ctrl&SID_CTRL_SAWTOOTH != 0 {
			waveType = WAVE_SAWTOOTH
		} else if ctrl&SID_CTRL_TRIANGLE != 0 {
			waveType = WAVE_TRIANGLE
		} else {
			// No waveform selected - silence
			waveType = WAVE_SQUARE
			e.writeChannel(voice, FLEX_OFF_VOL, 0)
		}

		e.writeChannel(voice, FLEX_OFF_WAVE_TYPE, uint32(waveType))

		// Handle gate bit
		gate := (ctrl & SID_CTRL_GATE) != 0
		prevGate := e.voiceGate[voice]

		if gate && !prevGate {
			// Gate just went high - trigger attack
			e.writeChannel(voice, FLEX_OFF_CTRL, 3) // Enable + gate
		} else if !gate && prevGate {
			// Gate just went low - trigger release
			e.writeChannel(voice, FLEX_OFF_CTRL, 1) // Enable, gate off
		} else if gate {
			e.writeChannel(voice, FLEX_OFF_CTRL, 3)
		} else {
			e.writeChannel(voice, FLEX_OFF_CTRL, 1)
		}

		e.voiceGate[voice] = gate

		// Handle test bit (resets oscillator)
		if ctrl&SID_CTRL_TEST != 0 {
			e.writeChannel(voice, FLEX_OFF_PHASE, 0)
		}
	}
}

// applyEnvelopes configures ADSR envelopes for each voice
func (e *SIDEngine) applyEnvelopes() {
	if e.sound == nil {
		return
	}

	for voice := 0; voice < 3; voice++ {
		base := voice * 7
		ad := e.regs[base+5]
		sr := e.regs[base+6]

		attack := (ad >> 4) & 0x0F
		decay := ad & 0x0F
		sustain := (sr >> 4) & 0x0F
		release := sr & 0x0F

		// Convert SID ADSR values to SoundChip format
		// SoundChip uses 8-bit values for time (0-255 maps to ms range)
		// and 8-bit for sustain level

		attackMs := sidAttackMs[attack]
		decayMs := sidDecayReleaseMs[decay]
		releaseMs := sidDecayReleaseMs[release]
		sustainLevel := float32(sustain) / 15.0

		// Scale to SoundChip's 8-bit range
		// Assume SoundChip maps 0-255 to approximately 0-2000ms for A/D/R
		attackVal := uint32(math.Min(255, float64(attackMs)/8))
		decayVal := uint32(math.Min(255, float64(decayMs)/8))
		releaseVal := uint32(math.Min(255, float64(releaseMs)/8))
		sustainVal := uint32(sustainLevel * 255)

		e.writeChannel(voice, FLEX_OFF_ATK, attackVal)
		e.writeChannel(voice, FLEX_OFF_DEC, decayVal)
		e.writeChannel(voice, FLEX_OFF_SUS, sustainVal)
		e.writeChannel(voice, FLEX_OFF_REL, releaseVal)
	}
}

// applyVolumes updates SoundChip volumes from SID registers
func (e *SIDEngine) applyVolumes() {
	if e.sound == nil {
		return
	}

	// Get master volume from MODE_VOL register
	modeVol := e.regs[0x18]
	masterVol := modeVol & SID_MODE_VOL_MASK

	// Check if voice 3 is disconnected
	voice3Off := (modeVol & SID_MODE_3OFF) != 0

	masterGain := sidVolumeGain(masterVol, e.sidPlusEnabled)

	for voice := 0; voice < 3; voice++ {
		if voice == 2 && voice3Off {
			e.writeChannel(voice, FLEX_OFF_VOL, 0)
			continue
		}

		// Each voice gets master volume (individual voice volume is handled by envelope)
		gain := masterGain
		if e.sidPlusEnabled {
			gain *= sidPlusMixGain[voice]
		}

		e.writeChannel(voice, FLEX_OFF_VOL, uint32(sidGainToDAC(gain)))
	}
}

// applyModulation sets up ring modulation and hard sync between voices
func (e *SIDEngine) applyModulation() {
	if e.sound == nil {
		return
	}

	for voice := 0; voice < 3; voice++ {
		base := voice * 7
		ctrl := e.regs[base+4]

		// Ring modulation: voice N is modulated by voice N-1 (wraps: voice 0 by voice 2)
		ringMod := (ctrl & SID_CTRL_RINGMOD) != 0
		sync := (ctrl & SID_CTRL_SYNC) != 0

		// Source voice for modulation (voice 0 uses voice 2, others use voice-1)
		srcVoice := (voice + 2) % 3

		if ringMod {
			e.writeChannel(voice, FLEX_OFF_RINGMOD, uint32(srcVoice)|0x80) // Enable + source
		} else {
			e.writeChannel(voice, FLEX_OFF_RINGMOD, 0)
		}

		if sync {
			e.writeChannel(voice, FLEX_OFF_SYNC, uint32(srcVoice)|0x80) // Enable + source
		} else {
			e.writeChannel(voice, FLEX_OFF_SYNC, 0)
		}
	}
}

// applyFilter configures the SoundChip global filter based on SID filter settings
func (e *SIDEngine) applyFilter() {
	if e.sound == nil {
		return
	}

	// Get filter cutoff (11-bit value: 3 bits from FC_LO, 8 bits from FC_HI)
	fcLo := e.regs[0x15] & 0x07
	fcHi := e.regs[0x16]
	cutoff := uint16(fcLo) | (uint16(fcHi) << 3)

	// Get resonance and filter routing
	resFilt := e.regs[0x17]
	resonance := (resFilt & SID_FILT_RES) >> 4

	// Get filter mode
	modeVol := e.regs[0x18]
	lowPass := (modeVol & SID_MODE_LP) != 0
	bandPass := (modeVol & SID_MODE_BP) != 0
	highPass := (modeVol & SID_MODE_HP) != 0

	// Convert SID filter cutoff (0-2047) to frequency
	// SID cutoff is roughly exponential, ranging from ~30Hz to ~12kHz
	// Approximation: freq = 30 * 2^(cutoff/200)
	var cutoffHz float64
	if cutoff == 0 {
		cutoffHz = 30
	} else {
		cutoffHz = 30 * math.Pow(2, float64(cutoff)/200)
	}
	if cutoffHz > 12000 {
		cutoffHz = 12000
	}

	// Map to SoundChip filter (0-255 range)
	cutoffVal := uint32(math.Min(255, cutoffHz/50))
	resVal := uint32(resonance * 16) // Scale 0-15 to 0-240

	// Determine filter type for SoundChip
	var filterType uint32
	if lowPass && !bandPass && !highPass {
		filterType = 0 // Low-pass only
	} else if bandPass && !lowPass && !highPass {
		filterType = 1 // Band-pass only
	} else if highPass && !lowPass && !bandPass {
		filterType = 2 // High-pass only
	} else if lowPass && highPass && !bandPass {
		filterType = 3 // Notch (LP + HP)
	} else {
		filterType = 0 // Default to low-pass for combinations
	}

	// Write to SoundChip global filter registers
	e.sound.HandleRegisterWrite(FILTER_CUTOFF, cutoffVal)
	e.sound.HandleRegisterWrite(FILTER_RESONANCE, resVal)
	e.sound.HandleRegisterWrite(FILTER_TYPE, filterType)
}

// writeChannel writes a value to a SoundChip channel register
func (e *SIDEngine) writeChannel(ch int, offset uint32, value uint32) {
	if e.sound == nil {
		return
	}
	base := FLEX_CH_BASE + uint32(ch)*FLEX_CH_STRIDE
	e.sound.HandleRegisterWrite(base+offset, value)
}

// sidVolumeGain converts a 4-bit SID volume level to a gain value
func sidVolumeGain(level uint8, sidPlus bool) float32 {
	if level > 15 {
		level = 15
	}
	if sidPlus {
		return sidPlusVolumeCurve[level]
	}
	// Linear volume curve for standard SID
	return float32(level) / 15.0
}

// sidGainToDAC converts a gain value to an 8-bit DAC value
func sidGainToDAC(gain float32) uint8 {
	if gain <= 0 {
		return 0
	}
	if gain >= 1.0 {
		return 255
	}
	return uint8(math.Round(float64(gain * 255.0)))
}

// Reset resets all SID state
func (e *SIDEngine) Reset() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for i := range e.regs {
		e.regs[i] = 0
	}
	for i := range e.voiceGate {
		e.voiceGate[i] = false
	}

	e.enabled = false
	e.channelsInit = false
}
