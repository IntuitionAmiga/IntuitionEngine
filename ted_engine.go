// ted_engine.go - MOS 7360/8360 TED sound chip register mapping for the Intuition Engine

/*
TED (Text Display) was the audio/video chip used in the Commodore Plus/4 and C16.
This implementation provides pure register mapping to the SoundChip for audio synthesis,
accessible from all CPUs (M68K, IE32, Z80, 6502).

Features:
- 2 audio voices mapped to SoundChip channels 0-1
- 10-bit frequency registers (split across two bytes per voice)
- Voice 2 can optionally produce white noise instead of square wave
- 4-bit master volume (0-8, values above 8 are clamped to max)
- TED+ enhanced mode with logarithmic volume curve and filtering

The engine translates TED register writes to SoundChip channel parameters.
Synthesis is performed by SoundChip - this module handles only the mapping.

TED frequency formula: freq_hz = clock/4 / (1024 - register_value)
Where register_value is the 10-bit combined frequency value.
The TED sound clock is main_clock/4 (221680 Hz for PAL), not main_clock/8.
*/

package main

import (
	"math"
	"sync"
)

// TEDEvent represents a single TED register write captured during playback.
type TEDEvent struct {
	Cycle  uint64 // CPU cycle when the write occurred
	Sample uint64 // Audio sample position for this event
	Reg    uint8  // Register offset (0-5)
	Value  uint8  // Value written
}

// TEDEngine emulates the TED sound chip via register mapping to SoundChip
type TEDEngine struct {
	mutex      sync.Mutex
	sound      *SoundChip
	sampleRate int
	clockHz    uint32

	regs [TED_REG_COUNT]uint8

	events         []TEDEvent
	eventIndex     int
	currentSample  uint64
	totalSamples   uint64
	loop           bool
	loopSample     uint64
	loopEventIndex int
	playing        bool

	enabled        bool
	tedPlusEnabled bool
	channelsInit   bool
}

// TED+ logarithmic volume curve (2dB per step)
var tedPlusVolumeCurve = func() [9]float32 {
	var curve [9]float32
	curve[0] = 0
	for i := 1; i <= 8; i++ {
		db := float64(i-8) * 3.0 // 3dB per step for more range
		curve[i] = float32(math.Pow(10.0, db/20.0))
	}
	curve[8] = 1.0
	return curve
}()

// NewTEDEngine creates a new TED emulation engine
func NewTEDEngine(sound *SoundChip, sampleRate int) *TEDEngine {
	return &TEDEngine{
		sound:      sound,
		sampleRate: sampleRate,
		clockHz:    TED_CLOCK_PAL,
	}
}

// SetClockHz sets the TED master clock frequency
func (e *TEDEngine) SetClockHz(clock uint32) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if clock == 0 {
		return
	}
	e.clockHz = clock
}

// HandleWrite processes a write to a TED register via memory-mapped I/O
func (e *TEDEngine) HandleWrite(addr uint32, value uint32) {
	if addr < TED_BASE || addr > TED_END {
		return
	}
	reg := uint8(addr - TED_BASE)
	e.WriteRegister(reg, uint8(value))
}

// HandleRead processes a read from a TED register
func (e *TEDEngine) HandleRead(addr uint32) uint32 {
	if addr < TED_BASE || addr > TED_END {
		return 0
	}
	reg := uint8(addr - TED_BASE)
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return uint32(e.regs[reg])
}

// WriteRegister writes a value to a TED register
func (e *TEDEngine) WriteRegister(reg uint8, value uint8) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if reg >= TED_REG_COUNT {
		return
	}

	e.enabled = true
	e.regs[reg] = value

	// Handle TED+ control register
	if reg == TED_REG_PLUS_CTRL {
		e.tedPlusEnabled = (value & 1) != 0
		if e.sound != nil {
			e.sound.SetTEDPlusEnabled(e.tedPlusEnabled)
		}
	}

	e.syncToChip()
}

// SetTEDPlusEnabled enables/disables TED+ enhanced mode
// When enabled, activates automatic audio enhancements:
// - Logarithmic volume curve for more musical response
// - Low-pass filtering to smooth harsh edges
func (e *TEDEngine) SetTEDPlusEnabled(enabled bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.tedPlusEnabled = enabled
	if e.sound != nil {
		e.sound.SetTEDPlusEnabled(enabled)
	}
	e.syncToChip()
}

// TEDPlusEnabled returns whether TED+ mode is active
func (e *TEDEngine) TEDPlusEnabled() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.tedPlusEnabled
}

// ensureChannelsInitialized sets up SoundChip channels for TED output
func (e *TEDEngine) ensureChannelsInitialized() {
	if e.channelsInit || e.sound == nil {
		return
	}

	// TED uses channels 0-1 of the SoundChip
	// Configure them as square waves with instant envelope
	for ch := 0; ch < 2; ch++ {
		e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
		e.writeChannel(ch, FLEX_OFF_DUTY, 0x0080) // 50% duty cycle
		e.writeChannel(ch, FLEX_OFF_PWM_CTRL, 0)
		e.writeChannel(ch, FLEX_OFF_ATK, 0)
		e.writeChannel(ch, FLEX_OFF_DEC, 0)
		e.writeChannel(ch, FLEX_OFF_SUS, 255)
		e.writeChannel(ch, FLEX_OFF_REL, 0)
		e.writeChannel(ch, FLEX_OFF_CTRL, 3) // Enable + gate
	}

	e.channelsInit = true
}

// syncToChip updates the SoundChip based on current TED register state
func (e *TEDEngine) syncToChip() {
	if e.sound == nil {
		return
	}

	e.ensureChannelsInitialized()
	e.applyFrequencies()
	e.applyVolumes()
	e.applyWaveforms()
}

// calcFrequency calculates the output frequency for a voice
func (e *TEDEngine) calcFrequency(voice int) float64 {
	if voice < 0 || voice > 1 {
		return 0
	}

	var freqReg uint16
	if voice == 0 {
		// Voice 1: low byte at reg 0, high bits at reg 4
		freqReg = uint16(e.regs[TED_REG_FREQ1_LO]) | (uint16(e.regs[TED_REG_FREQ1_HI]&0x03) << 8)
	} else {
		// Voice 2: low byte at reg 1, high bits at reg 2
		freqReg = uint16(e.regs[TED_REG_FREQ2_LO]) | (uint16(e.regs[TED_REG_FREQ2_HI]&0x03) << 8)
	}

	return tedFrequencyHz(freqReg, e.clockHz)
}

// tedFrequencyHz calculates the output frequency from a 10-bit register value
// Formula: freq_hz = clock/4 / (1024 - register_value)
// The TED sound clock is main_clock/4 = 221680 Hz (PAL)
// Reference: tedplay uses TED_SOUND_CLOCK = 221680
func tedFrequencyHz(regValue uint16, clockHz uint32) float64 {
	if regValue >= 1024 {
		regValue = 1023
	}
	divisor := 1024 - int(regValue)
	if divisor <= 0 {
		divisor = 1
	}
	return float64(clockHz) / 4.0 / float64(divisor)
}

// applyFrequencies updates SoundChip frequencies from TED registers
func (e *TEDEngine) applyFrequencies() {
	if e.sound == nil {
		return
	}

	for voice := 0; voice < 2; voice++ {
		freq := e.calcFrequency(voice)
		if freq > 0 && freq <= 20000 {
			e.writeChannel(voice, FLEX_OFF_FREQ, uint32(freq))
		} else {
			e.writeChannel(voice, FLEX_OFF_FREQ, 0)
		}
	}
}

// applyVolumes updates SoundChip volumes from TED registers
func (e *TEDEngine) applyVolumes() {
	if e.sound == nil {
		return
	}

	ctrl := e.regs[TED_REG_SND_CTRL]
	volume := ctrl & TED_CTRL_VOLUME
	voice1On := (ctrl & TED_CTRL_SND1ON) != 0
	voice2On := (ctrl & TED_CTRL_SND2ON) != 0

	gain := tedVolumeGain(volume, e.tedPlusEnabled)

	// Voice 1
	if voice1On {
		v1Gain := gain
		if e.tedPlusEnabled {
			v1Gain *= tedPlusMixGain[0]
		}
		e.writeChannel(0, FLEX_OFF_VOL, uint32(tedGainToDAC(v1Gain)))
	} else {
		e.writeChannel(0, FLEX_OFF_VOL, 0)
	}

	// Voice 2
	if voice2On {
		v2Gain := gain
		if e.tedPlusEnabled {
			v2Gain *= tedPlusMixGain[1]
		}
		e.writeChannel(1, FLEX_OFF_VOL, uint32(tedGainToDAC(v2Gain)))
	} else {
		e.writeChannel(1, FLEX_OFF_VOL, 0)
	}
}

// applyWaveforms sets the waveform type for each voice
func (e *TEDEngine) applyWaveforms() {
	if e.sound == nil {
		return
	}

	ctrl := e.regs[TED_REG_SND_CTRL]
	noise := (ctrl & TED_CTRL_SND2NOISE) != 0

	// Voice 1 is always square wave
	e.writeChannel(0, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)

	// Voice 2 can be square or noise
	if noise {
		e.writeChannel(1, FLEX_OFF_WAVE_TYPE, WAVE_NOISE)
		e.writeChannel(1, FLEX_OFF_NOISEMODE, NOISE_MODE_WHITE)
	} else {
		e.writeChannel(1, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
	}
}

// writeChannel writes a value to a SoundChip channel register
func (e *TEDEngine) writeChannel(ch int, offset uint32, value uint32) {
	if e.sound == nil {
		return
	}
	base := FLEX_CH_BASE + uint32(ch)*FLEX_CH_STRIDE
	e.sound.HandleRegisterWrite(base+offset, value)
}

// tedVolumeGain converts a 4-bit TED volume level to a gain value
// TED volume is 0-8 (values above 8 are clamped to max)
func tedVolumeGain(level uint8, tedPlus bool) float32 {
	if level > TED_MAX_VOLUME {
		level = TED_MAX_VOLUME
	}
	if tedPlus {
		return tedPlusVolumeCurve[level]
	}
	// Linear volume curve for standard TED
	return float32(level) / float32(TED_MAX_VOLUME)
}

// tedGainToDAC converts a gain value to an 8-bit DAC value
func tedGainToDAC(gain float32) uint8 {
	if gain <= 0 {
		return 0
	}
	if gain >= 1.0 {
		return 255
	}
	return uint8(math.Round(float64(gain * 255.0)))
}

// Reset resets all TED state
func (e *TEDEngine) Reset() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for i := range e.regs {
		e.regs[i] = 0
	}

	e.enabled = false
	e.channelsInit = false

	// Reset playback state
	e.playing = false
	e.events = nil
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = 0
	e.loop = false
	e.loopSample = 0
	e.loopEventIndex = 0
}

// SetEvents sets the TED events for playback
func (e *TEDEngine) SetEvents(events []TEDEvent, totalSamples uint64, loop bool, loopSample uint64) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.events = make([]TEDEvent, len(events))
	copy(e.events, events)
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = totalSamples
	e.loop = loop
	e.loopSample = loopSample
	e.loopEventIndex = 0

	// Find loop event index
	for i, ev := range e.events {
		if ev.Sample >= loopSample {
			e.loopEventIndex = i
			break
		}
	}

	e.playing = true
	e.enabled = true
}

// SetPlaying starts or stops event-based playback
func (e *TEDEngine) SetPlaying(playing bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = playing
	if playing {
		e.enabled = true
		e.ensureChannelsInitialized()
	}
}

// IsPlaying returns true if event-based playback is active
func (e *TEDEngine) IsPlaying() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.playing
}

// SetForceLoop enables looping from the start of the track
func (e *TEDEngine) SetForceLoop(enable bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if enable {
		e.loop = true
		e.loopSample = 0
		e.loopEventIndex = 0
	}
}

// StopPlayback stops playback and clears events
func (e *TEDEngine) StopPlayback() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = false
	e.events = nil
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = 0
	e.silenceChannels()
}

// TickSample processes one sample of event-based playback
// Implements SampleTicker interface for SoundChip integration
func (e *TEDEngine) TickSample() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if !e.enabled {
		return
	}

	if e.playing {
		// Process all events at current sample position
		for e.eventIndex < len(e.events) && e.events[e.eventIndex].Sample == e.currentSample {
			ev := e.events[e.eventIndex]
			e.writeRegisterLocked(ev.Reg, ev.Value)
			e.eventIndex++
		}
	}

	e.currentSample++

	// Check for end of playback
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

// silenceChannels sets all channel volumes to 0
func (e *TEDEngine) silenceChannels() {
	if e.sound == nil {
		return
	}
	for ch := 0; ch < 2; ch++ {
		e.writeChannel(ch, FLEX_OFF_VOL, 0)
	}
}

// writeRegisterLocked writes a register without acquiring the lock (caller must hold lock)
func (e *TEDEngine) writeRegisterLocked(reg uint8, value uint8) {
	if reg >= TED_REG_COUNT {
		return
	}
	e.regs[reg] = value

	// Handle TED+ control register
	if reg == TED_REG_PLUS_CTRL {
		e.tedPlusEnabled = (value & 1) != 0
		if e.sound != nil {
			e.sound.SetTEDPlusEnabled(e.tedPlusEnabled)
		}
	}

	e.syncToChip()
}
