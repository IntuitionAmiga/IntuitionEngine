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
	"fmt"
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

	events         []SIDEvent
	eventIndex     int
	currentSample  uint64
	totalSamples   uint64
	loop           bool
	loopSample     uint64
	loopEventIndex int
	playing        bool
	debugEnabled   bool
	debugUntil     uint64
	debugNextTick  uint64
	lastCtrl       [3]uint8
	lastAD         [3]uint8
	lastSR         [3]uint8

	// Voice state (for gate tracking and envelope)
	voiceGate [3]bool
	voiceWave [3]bool

	enabled        bool
	sidPlusEnabled bool
	channelsInit   bool
	model          int  // SID_MODEL_6581 or SID_MODEL_8580
	forceLoop      bool // Force looping from start when track ends
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
		model:      SID_MODEL_6581, // Default to original SID
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

// SetModel sets the SID chip model (6581 or 8580)
func (e *SIDEngine) SetModel(model int) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if model == SID_MODEL_6581 || model == SID_MODEL_8580 {
		e.model = model
	}
}

// GetModel returns the current SID chip model
func (e *SIDEngine) GetModel() int {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.model
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

	// Handle read-only registers that return live voice 3 state
	switch reg {
	case 0x1B: // OSC3 - Oscillator 3 output (8-bit)
		if e.sound != nil {
			return uint32(e.sound.GetChannelOscillatorOutput(2))
		}
		return 0
	case 0x1C: // ENV3 - Envelope 3 output (8-bit)
		if e.sound != nil {
			return uint32(e.sound.GetChannelEnvelopeLevel(2))
		}
		return 0
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()
	return uint32(e.regs[reg])
}

// WriteRegister writes a value to a SID register
func (e *SIDEngine) WriteRegister(reg uint8, value uint8) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.writeRegisterLocked(reg, value)
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
		e.sound.SetChannelEnvelopeMode(ch, true)
		e.sound.SetChannelSIDFilterMode(ch, false) // Safe filter mode
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
		pw = 1
	} else if pw >= 4095 {
		pw = 4094
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
			e.writeChannel(voice, FLEX_OFF_FREQ, uint32(freq*256)) // 16.8 fixed-point
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
		mask := ctrl & (SID_CTRL_TRIANGLE | SID_CTRL_SAWTOOTH | SID_CTRL_PULSE | SID_CTRL_NOISE)

		// Determine waveform (multiple can be selected for combined waveforms,
		// but we'll prioritize: noise > pulse > sawtooth > triangle)
		var waveType int
		if ctrl&SID_CTRL_NOISE != 0 {
			waveType = WAVE_NOISE
		} else if ctrl&SID_CTRL_PULSE != 0 {
			waveType = WAVE_SQUARE
		} else if ctrl&SID_CTRL_SAWTOOTH != 0 {
			waveType = WAVE_SAWTOOTH
		} else if ctrl&SID_CTRL_TRIANGLE != 0 {
			waveType = WAVE_TRIANGLE
		} else {
			// No waveform selected - silence
			waveType = WAVE_SQUARE
		}

		if mask&SID_CTRL_PULSE != 0 {
			pw := e.calcPulseWidth(voice)
			e.writeChannel(voice, FLEX_OFF_DUTY, uint32(pw*255))
		}

		e.writeChannel(voice, FLEX_OFF_WAVE_TYPE, uint32(waveType))
		e.voiceWave[voice] = mask != 0
		e.sound.SetChannelSIDWaveMask(voice, mask)

		// Handle gate bit
		gate := (ctrl & SID_CTRL_GATE) != 0
		testBit := (ctrl & SID_CTRL_TEST) != 0
		effectiveGate := gate && e.voiceWave[voice] && !testBit
		prevGate := e.voiceGate[voice]

		if effectiveGate && !prevGate {
			// Gate just went high - trigger attack
			e.writeChannel(voice, FLEX_OFF_CTRL, 3) // Enable + gate
		} else if !effectiveGate && prevGate {
			// Gate just went low - trigger release
			e.writeChannel(voice, FLEX_OFF_CTRL, 1) // Enable, gate off
		} else if effectiveGate {
			e.writeChannel(voice, FLEX_OFF_CTRL, 3)
		} else {
			e.writeChannel(voice, FLEX_OFF_CTRL, 1)
		}

		e.voiceGate[voice] = effectiveGate

		// Handle test bit (resets oscillator, mutes output)
		if testBit {
			e.writeChannel(voice, FLEX_OFF_PHASE, 0)
			e.sound.SetChannelSIDTest(voice, true)
		} else {
			e.sound.SetChannelSIDTest(voice, false)
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

		sustainLevel := float32(sustain) / 15.0

		// Use authentic SID rate counter for ADSR timing
		e.sound.SetChannelSIDRateCounter(voice, false, e.sampleRate, e.clockHz, attack, decay, release)

		// Also set time-based ADSR as fallback and for sustain level
		attackMs := sidAttackMs[attack]
		decayMs := sidDecayReleaseMs[decay]
		releaseMs := sidDecayReleaseMs[release]
		e.sound.SetChannelADSR(voice, attackMs, decayMs, releaseMs, sustainLevel)
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
		if !e.voiceWave[voice] {
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
	routing := resFilt & 0x0F

	// Get filter mode
	modeVol := e.regs[0x18]
	lowPass := (modeVol & SID_MODE_LP) != 0
	bandPass := (modeVol & SID_MODE_BP) != 0
	highPass := (modeVol & SID_MODE_HP) != 0

	// Convert SID filter cutoff (0-2047) to frequency
	// Different curves for 6581 vs 8580 chip models
	var cutoffHz float64
	if cutoff == 0 {
		cutoffHz = 30
	} else if e.model == SID_MODEL_8580 {
		// 8580: More linear response, cleaner sound
		// Fc ≈ 30 + cutoff * 5.8 (approximately)
		cutoffHz = 30 + float64(cutoff)*5.8
	} else {
		// 6581 (default): Non-linear response, warmer sound
		// Characteristic curve: compressed at low values, expands at high
		// Approximation: Fc = 30 + cutoff^1.35 * 0.22
		cutoffHz = 30 + math.Pow(float64(cutoff), 1.35)*0.22
	}
	// 8580 can reach higher frequencies than 6581
	maxCutoff := 12000.0
	if e.model == SID_MODEL_8580 {
		maxCutoff = 18000.0
	}
	if cutoffHz > maxCutoff {
		cutoffHz = maxCutoff
	}

	// Map cutoff to normalized 0-1 range using log scale for musical response
	// 30Hz -> 0.0, maxCutoff -> 1.0
	cutoffNorm := float32(math.Log(cutoffHz/30) / math.Log(maxCutoff/30))
	if cutoffNorm < 0 {
		cutoffNorm = 0
	} else if cutoffNorm > 1 {
		cutoffNorm = 1
	}

	// SID resonance has exponential response - higher values cause self-oscillation
	// Use a smooth power curve (no discontinuity) that:
	// - Resonance 0-7: subtle effect
	// - Resonance 8-12: noticeable
	// - Resonance 13-15: very pronounced, approaching self-oscillation
	// Power of 2.2 gives good SID-like response: gentle at low, steep at high
	resFloat := float64(resonance) / 15.0
	resNorm := float32(math.Pow(resFloat, 2.2) * 0.95)
	modeMask := uint8(0)
	if lowPass {
		modeMask |= 0x01
	}
	if bandPass {
		modeMask |= 0x02
	}
	if highPass {
		modeMask |= 0x04
	}

	for voice := 0; voice < 3; voice++ {
		mask := uint8(1 << voice)
		if routing&mask != 0 && modeMask != 0 {
			e.sound.SetChannelFilter(voice, modeMask, cutoffNorm, resNorm)
		} else {
			e.sound.SetChannelFilter(voice, 0, 0, 0)
		}
	}
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
	e.playing = false
	e.events = nil
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = 0
	e.loop = false
	e.loopSample = 0
	e.loopEventIndex = 0
}

func (e *SIDEngine) SetEvents(events []SIDEvent, totalSamples uint64, loop bool, loopSample uint64) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.events = make([]SIDEvent, len(events))
	copy(e.events, events)
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = totalSamples
	e.loop = loop
	e.loopSample = loopSample
	e.loopEventIndex = 0
	for i, ev := range e.events {
		if ev.Sample >= loopSample {
			e.loopEventIndex = i
			break
		}
	}
	e.enabled = true
}

// EnableDebugLogging logs timing and ADSR/gate changes for the first N seconds.
func (e *SIDEngine) EnableDebugLogging(seconds int) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if seconds <= 0 || e.sampleRate <= 0 {
		e.debugEnabled = false
		return
	}
	e.debugEnabled = true
	e.debugUntil = uint64(seconds * e.sampleRate)
	e.debugNextTick = uint64(e.sampleRate)
	for i := range e.lastCtrl {
		e.lastCtrl[i] = e.regs[i*7+4]
		e.lastAD[i] = e.regs[i*7+5]
		e.lastSR[i] = e.regs[i*7+6]
	}
	fmt.Printf("SID debug: logging for %d seconds\n", seconds)
}

func (e *SIDEngine) SetPlaying(playing bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = playing
	if !playing {
		e.silenceChannels()
	}
}

func (e *SIDEngine) IsPlaying() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.playing
}

// SetForceLoop enables looping from the start of the track even if the
// track doesn't have a built-in loop point.
func (e *SIDEngine) SetForceLoop(enable bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.forceLoop = enable
	if enable {
		e.loop = true
		e.loopSample = 0
		e.loopEventIndex = 0
	}
}

func (e *SIDEngine) StopPlayback() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = false
	e.events = nil
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = 0
	e.loop = false
	e.loopSample = 0
	e.loopEventIndex = 0
	e.silenceChannels()
}

func (e *SIDEngine) TickSample() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if !e.enabled {
		return
	}

	if e.playing {
		for e.eventIndex < len(e.events) && e.events[e.eventIndex].Sample == e.currentSample {
			ev := e.events[e.eventIndex]
			e.writeRegisterLocked(ev.Reg, ev.Value)
			e.eventIndex++
		}
	}

	if e.debugEnabled && e.currentSample < e.debugUntil && e.currentSample >= e.debugNextTick {
		seconds := float64(e.currentSample) / float64(e.sampleRate)
		fmt.Printf("SID t=%.1fs\n", seconds)
		e.debugNextTick += uint64(e.sampleRate)
	}

	e.currentSample++

	if e.playing && e.totalSamples > 0 && e.currentSample >= e.totalSamples {
		if e.loop {
			e.currentSample = e.loopSample
			e.eventIndex = e.loopEventIndex
		} else {
			e.playing = false
			e.silenceChannels()
		}
	}
}

func (e *SIDEngine) silenceChannels() {
	if e.sound == nil {
		return
	}
	for ch := 0; ch < 3; ch++ {
		e.writeChannel(ch, FLEX_OFF_VOL, 0)
	}
}

func (e *SIDEngine) writeRegisterLocked(reg uint8, value uint8) {
	if reg >= SID_REG_COUNT {
		return
	}

	if e.debugEnabled && e.currentSample < e.debugUntil {
		e.debugRegisterWrite(reg, value)
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

func (e *SIDEngine) debugRegisterWrite(reg uint8, value uint8) {
	voice := -1
	switch reg {
	case 0x04:
		voice = 0
	case 0x0B:
		voice = 1
	case 0x12:
		voice = 2
	}

	if voice >= 0 {
		prev := e.lastCtrl[voice]
		if (prev^value)&SID_CTRL_GATE != 0 {
			state := "off"
			if value&SID_CTRL_GATE != 0 {
				state = "on"
			}
			fmt.Printf("SID t=%.3fs V%d gate=%s ctrl=0x%02X\n", e.debugTime(), voice+1, state, value)
		}
		if (prev^value)&0xF0 != 0 {
			fmt.Printf("SID t=%.3fs V%d wave=0x%02X\n", e.debugTime(), voice+1, value&0xF0)
		}
		e.lastCtrl[voice] = value
		return
	}

	switch reg {
	case 0x05, 0x0C, 0x13:
		voice = int((reg - 0x05) / 7)
		if voice >= 0 && voice < 3 && value != e.lastAD[voice] {
			fmt.Printf("SID t=%.3fs V%d AD=0x%02X\n", e.debugTime(), voice+1, value)
			e.lastAD[voice] = value
		}
	case 0x06, 0x0D, 0x14:
		voice = int((reg - 0x06) / 7)
		if voice >= 0 && voice < 3 && value != e.lastSR[voice] {
			fmt.Printf("SID t=%.3fs V%d SR=0x%02X\n", e.debugTime(), voice+1, value)
			e.lastSR[voice] = value
		}
	}

	switch reg {
	case 0x15, 0x16, 0x17, 0x18:
		fcLo := e.regs[0x15] & 0x07
		fcHi := e.regs[0x16]
		cutoff := uint16(fcLo) | (uint16(fcHi) << 3)
		resFilt := e.regs[0x17]
		modeVol := e.regs[0x18]
		fmt.Printf("SID t=%.3fs FILTER cutoff=%d res=0x%02X mode=0x%02X\n", e.debugTime(), cutoff, resFilt, modeVol)
	}
}

func (e *SIDEngine) debugTime() float64 {
	if e.sampleRate <= 0 {
		return 0
	}
	return float64(e.currentSample) / float64(e.sampleRate)
}
