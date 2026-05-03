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
	"sort"
	"sync"
	"sync/atomic"
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

	enabled        atomic.Bool
	sidPlusEnabled bool
	channelsInit   bool
	model          int    // SID_MODEL_6581 or SID_MODEL_8580
	forceLoop      bool   // Force looping from start when track ends
	baseChannel    int    // Base channel offset in SoundChip (0 for primary, 4 for SID2, 7 for SID3)
	regBase        uint32 // MMIO register base address (SID_BASE for primary, SID2_BASE/SID3_BASE for secondary)
	regEnd         uint32 // MMIO register end address

	// Multi-SID: secondary engines for Chip 1/2 dispatch
	sid2 *SIDEngine
	sid3 *SIDEngine

	busMemory []byte // mirror register writes for Machine Monitor visibility
}

type sidChannelWrite struct {
	ch     int
	offset uint32
	value  uint32
}

type sidBoolSetting struct {
	ch      int
	enabled bool
}

type sidWaveMaskSetting struct {
	ch   int
	mask uint8
}

type sidRateCounterSetting struct {
	ch                     int
	enabled                bool
	sampleRate             int
	clockHz                uint32
	attack, decay, release uint8
}

type sidADSRSetting struct {
	ch           int
	attackMs     float32
	decayMs      float32
	releaseMs    float32
	sustainLevel float32
}

type sidFilterSetting struct {
	ch                int
	modeMask          uint8
	cutoff, resonance float32
}

type sidSyncPlan struct {
	baseChannel  int
	writes       []sidChannelWrite
	envelopeMode []sidBoolSetting
	filterMode   []sidBoolSetting
	dac          []sidBoolSetting
	waveMasks    []sidWaveMaskSetting
	testBits     []sidBoolSetting
	rateCounters []sidRateCounterSetting
	adsr         []sidADSRSetting
	filters      []sidFilterSetting
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

// SID linear volume curve - pre-computed lookup table (0-15 range)
var sidLinearVolumeCurve = [16]float32{
	0.0 / 15.0,  // 0
	1.0 / 15.0,  // 1
	2.0 / 15.0,  // 2
	3.0 / 15.0,  // 3
	4.0 / 15.0,  // 4
	5.0 / 15.0,  // 5
	6.0 / 15.0,  // 6
	7.0 / 15.0,  // 7
	8.0 / 15.0,  // 8
	9.0 / 15.0,  // 9
	10.0 / 15.0, // 10
	11.0 / 15.0, // 11
	12.0 / 15.0, // 12
	13.0 / 15.0, // 13
	14.0 / 15.0, // 14
	15.0 / 15.0, // 15 (max)
}

// sidPlusMixGain is defined in audio_chip.go

// NewSIDEngine creates a new SID emulation engine
func NewSIDEngine(sound *SoundChip, sampleRate int) *SIDEngine {
	return &SIDEngine{
		sound:      sound,
		sampleRate: sampleRate,
		clockHz:    SID_CLOCK_PAL,
		model:      SID_MODEL_6581, // Default to original SID
		regBase:    SID_BASE,
		regEnd:     SID_END,
	}
}

// NewSIDEngineMulti creates a SID engine targeting a specific channel range in the SoundChip.
// baseChannel offsets all channel writes (0 for primary, 4 for SID2, 7 for SID3).
// regBase/regEnd define the MMIO address range for HandleRead/HandleWrite.
func NewSIDEngineMulti(sound *SoundChip, sampleRate int, baseChannel int, regBase, regEnd uint32) *SIDEngine {
	return &SIDEngine{
		sound:       sound,
		sampleRate:  sampleRate,
		clockHz:     SID_CLOCK_PAL,
		model:       SID_MODEL_6581,
		baseChannel: baseChannel,
		regBase:     regBase,
		regEnd:      regEnd,
	}
}

func (e *SIDEngine) AttachBusMemory(mem []byte) {
	e.busMemory = mem
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
	if model != SID_MODEL_6581 && model != SID_MODEL_8580 {
		e.mutex.Unlock()
		return
	}
	e.model = model
	sound := e.sound
	baseChannel := e.baseChannel
	e.mutex.Unlock()

	if sound == nil {
		return
	}
	for ch := range 3 {
		idx := baseChannel + ch
		if model == SID_MODEL_6581 {
			sound.SetChannelSIDADSRBugs(idx, true)
			sound.SetChannelSID6581FilterDistort(idx, true)
			sound.SetChannelSIDNoisePhaseLocked(idx, true)
		} else {
			sound.SetChannelSIDADSRBugs(idx, false)
			sound.SetChannelSID6581FilterDistort(idx, false)
			sound.SetChannelSIDNoisePhaseLocked(idx, true)
		}
	}
	if model == SID_MODEL_6581 {
		sound.SetSIDMixerMode(true, SID_6581_DC_OFFSET, true)
	} else {
		sound.SetSIDMixerMode(true, SID_8580_DC_OFFSET, false)
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
	if addr < e.regBase || addr > e.regEnd {
		return
	}
	reg := uint8(addr - e.regBase)
	e.WriteRegister(reg, uint8(value))
}

func (e *SIDEngine) HandleWrite8(addr uint32, value uint8) {
	if addr < e.regBase || addr > e.regEnd {
		return
	}
	reg := uint8(addr - e.regBase)
	e.WriteRegister(reg, value)
}

// HandleRead processes a read from a SID register
func (e *SIDEngine) HandleRead(addr uint32) uint32 {
	if addr < e.regBase || addr > e.regEnd {
		return 0
	}
	reg := uint8(addr - e.regBase)

	// Handle read-only registers that return live voice 3 state
	switch reg {
	case 0x1B: // OSC3 - Oscillator 3 output (8-bit)
		if e.sound != nil {
			return uint32(e.sound.GetChannelOscillatorOutput(e.baseChannel + 2))
		}
		return 0
	case 0x1C: // ENV3 - Envelope 3 output (8-bit)
		if e.sound != nil {
			return uint32(e.sound.GetChannelEnvelopeLevel(e.baseChannel + 2))
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
	plusChanged, sidPlusEnabled, sound, baseChannel := e.writeRegisterStateLocked(reg, value)
	e.mutex.Unlock()

	if plusChanged && sound != nil {
		sound.SetSIDPlusEnabledForRange(sidPlusEnabled, baseChannel, 3)
	}
	e.syncToChip()
}

// SetSIDPlusEnabled enables/disables SID+ enhanced mode
func (e *SIDEngine) SetSIDPlusEnabled(enabled bool) {
	e.mutex.Lock()
	e.sidPlusEnabled = enabled
	sound := e.sound
	e.mutex.Unlock()

	if sound != nil {
		sound.SetSIDPlusEnabled(enabled)
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

	// SID uses 3 voices mapped to baseChannel..baseChannel+2
	for ch := range 3 {
		e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_TRIANGLE)
		e.writeChannel(ch, FLEX_OFF_DUTY, 0x0080) // 50% duty cycle
		e.writeChannel(ch, FLEX_OFF_PWM_CTRL, 0)
		e.writeChannel(ch, FLEX_OFF_ATK, 0)
		e.writeChannel(ch, FLEX_OFF_DEC, 0)
		e.writeChannel(ch, FLEX_OFF_SUS, 255)
		e.writeChannel(ch, FLEX_OFF_REL, 0)
		e.writeChannel(ch, FLEX_OFF_VOL, 0)
		e.writeChannel(ch, FLEX_OFF_CTRL, 0) // Start with gate off
		e.sound.SetChannelEnvelopeMode(e.baseChannel+ch, true)
		e.sound.SetChannelSIDFilterMode(e.baseChannel+ch, false) // Safe filter mode
		e.sound.SetChannelSIDDAC(e.baseChannel+ch, true)         // Enable 12-bit DAC quantization
	}

	e.channelsInit = true
}

// syncToChip updates the SoundChip based on current SID register state
func (e *SIDEngine) syncToChip() {
	e.mutex.Lock()
	sound := e.sound
	if sound == nil {
		e.mutex.Unlock()
		return
	}
	plan := e.buildSyncPlanLocked()
	e.mutex.Unlock()

	for _, w := range plan.writes {
		if addr, ok := flexAddrForChannel(w.ch, w.offset); ok {
			sound.HandleRegisterWrite(addr, w.value)
		}
	}
	for _, s := range plan.envelopeMode {
		sound.SetChannelEnvelopeMode(s.ch, s.enabled)
	}
	for _, s := range plan.filterMode {
		sound.SetChannelSIDFilterMode(s.ch, s.enabled)
	}
	for _, s := range plan.dac {
		sound.SetChannelSIDDAC(s.ch, s.enabled)
	}
	for _, s := range plan.waveMasks {
		sound.SetChannelSIDWaveMask(s.ch, s.mask)
	}
	for _, s := range plan.testBits {
		sound.SetChannelSIDTest(s.ch, s.enabled)
	}
	for _, s := range plan.rateCounters {
		sound.SetChannelSIDRateCounter(s.ch, s.enabled, s.sampleRate, s.clockHz, s.attack, s.decay, s.release)
	}
	for _, s := range plan.adsr {
		sound.SetChannelADSR(s.ch, s.attackMs, s.decayMs, s.releaseMs, s.sustainLevel)
	}
	for _, s := range plan.filters {
		sound.SetChannelFilter(s.ch, s.modeMask, s.cutoff, s.resonance)
	}
}

func (e *SIDEngine) buildSyncPlanLocked() sidSyncPlan {
	plan := sidSyncPlan{baseChannel: e.baseChannel}
	if !e.channelsInit {
		for ch := range 3 {
			idx := e.baseChannel + ch
			plan.write(ch, FLEX_OFF_WAVE_TYPE, WAVE_TRIANGLE)
			plan.write(ch, FLEX_OFF_DUTY, 0x0080)
			plan.write(ch, FLEX_OFF_PWM_CTRL, 0)
			plan.write(ch, FLEX_OFF_ATK, 0)
			plan.write(ch, FLEX_OFF_DEC, 0)
			plan.write(ch, FLEX_OFF_SUS, 255)
			plan.write(ch, FLEX_OFF_REL, 0)
			plan.write(ch, FLEX_OFF_VOL, 0)
			plan.write(ch, FLEX_OFF_CTRL, 0)
			plan.envelopeMode = append(plan.envelopeMode, sidBoolSetting{ch: idx, enabled: true})
			plan.filterMode = append(plan.filterMode, sidBoolSetting{ch: idx, enabled: false})
			plan.dac = append(plan.dac, sidBoolSetting{ch: idx, enabled: true})
		}
		e.channelsInit = true
	}

	e.planFrequenciesLocked(&plan)
	e.planWaveformsLocked(&plan)
	e.planEnvelopesLocked(&plan)
	e.planVolumesLocked(&plan)
	e.planModulationLocked(&plan)
	e.planFilterLocked(&plan)
	return plan
}

func (p *sidSyncPlan) write(ch int, offset uint32, value uint32) {
	p.writes = append(p.writes, sidChannelWrite{ch: p.baseChannel + ch, offset: offset, value: value})
}

func (e *SIDEngine) planFrequenciesLocked(plan *sidSyncPlan) {
	for voice := range 3 {
		freq := e.calcFrequency(voice)
		if freq > 0 && freq <= 20000 {
			plan.write(voice, FLEX_OFF_FREQ, uint32(freq*256))
		} else {
			plan.write(voice, FLEX_OFF_FREQ, 0)
		}
	}
}

func (e *SIDEngine) planWaveformsLocked(plan *sidSyncPlan) {
	for voice := range 3 {
		base := voice * 7
		ctrl := e.regs[base+4]
		mask := ctrl & (SID_CTRL_TRIANGLE | SID_CTRL_SAWTOOTH | SID_CTRL_PULSE | SID_CTRL_NOISE)

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
			waveType = WAVE_SQUARE
		}

		if mask&SID_CTRL_PULSE != 0 {
			pw := e.calcPulseWidth(voice)
			plan.write(voice, FLEX_OFF_DUTY, uint32(pw*255))
		}
		plan.write(voice, FLEX_OFF_WAVE_TYPE, uint32(waveType))
		e.voiceWave[voice] = mask != 0
		plan.waveMasks = append(plan.waveMasks, sidWaveMaskSetting{ch: e.baseChannel + voice, mask: mask})

		gate := (ctrl & SID_CTRL_GATE) != 0
		testBit := (ctrl & SID_CTRL_TEST) != 0
		effectiveGate := gate && e.voiceWave[voice] && !testBit
		prevGate := e.voiceGate[voice]

		switch {
		case effectiveGate && !prevGate:
			plan.write(voice, FLEX_OFF_CTRL, 3)
		case !effectiveGate && prevGate:
			plan.write(voice, FLEX_OFF_CTRL, 1)
		case effectiveGate:
			plan.write(voice, FLEX_OFF_CTRL, 3)
		default:
			plan.write(voice, FLEX_OFF_CTRL, 1)
		}
		e.voiceGate[voice] = effectiveGate

		if testBit {
			plan.write(voice, FLEX_OFF_PHASE, 0)
		}
		plan.testBits = append(plan.testBits, sidBoolSetting{ch: e.baseChannel + voice, enabled: testBit})
	}
}

func (e *SIDEngine) planEnvelopesLocked(plan *sidSyncPlan) {
	for voice := range 3 {
		base := voice * 7
		ad := e.regs[base+5]
		sr := e.regs[base+6]
		attack := (ad >> 4) & 0x0F
		decay := ad & 0x0F
		sustain := (sr >> 4) & 0x0F
		release := sr & 0x0F

		plan.rateCounters = append(plan.rateCounters, sidRateCounterSetting{
			ch:         e.baseChannel + voice,
			enabled:    false,
			sampleRate: e.sampleRate,
			clockHz:    e.clockHz,
			attack:     attack,
			decay:      decay,
			release:    release,
		})
		plan.adsr = append(plan.adsr, sidADSRSetting{
			ch:           e.baseChannel + voice,
			attackMs:     sidAttackMs[attack],
			decayMs:      sidDecayReleaseMs[decay],
			releaseMs:    sidDecayReleaseMs[release],
			sustainLevel: float32(sustain) / 15.0,
		})
	}
}

func (e *SIDEngine) planVolumesLocked(plan *sidSyncPlan) {
	modeVol := e.regs[0x18]
	masterVol := modeVol & SID_MODE_VOL_MASK
	voice3Off := (modeVol & SID_MODE_3OFF) != 0
	filterRoutedV3 := (e.regs[0x17] & SID_FILT_V3) != 0
	masterGain := sidVolumeGain(masterVol, e.sidPlusEnabled)

	for voice := range 3 {
		if voice == 2 && voice3Off && !filterRoutedV3 {
			plan.write(voice, FLEX_OFF_VOL, 0)
			continue
		}
		if !e.voiceWave[voice] {
			plan.write(voice, FLEX_OFF_VOL, 0)
			continue
		}
		plan.write(voice, FLEX_OFF_VOL, uint32(sidGainToDAC(masterGain)))
	}
}

func (e *SIDEngine) planModulationLocked(plan *sidSyncPlan) {
	for voice := range 3 {
		base := voice * 7
		ctrl := e.regs[base+4]
		ringMod := (ctrl&SID_CTRL_RINGMOD) != 0 && (ctrl&SID_CTRL_TRIANGLE) != 0
		sync := (ctrl & SID_CTRL_SYNC) != 0
		srcVoice := e.baseChannel + ((voice + 2) % 3)
		if ringMod {
			plan.write(voice, FLEX_OFF_RINGMOD, uint32(0x80|srcVoice))
		} else {
			plan.write(voice, FLEX_OFF_RINGMOD, 0)
		}
		if sync {
			plan.write(voice, FLEX_OFF_SYNC, uint32(0x80|srcVoice))
		} else {
			plan.write(voice, FLEX_OFF_SYNC, 0)
		}
	}
}

func (e *SIDEngine) planFilterLocked(plan *sidSyncPlan) {
	fcLo := e.regs[0x15] & 0x07
	fcHi := e.regs[0x16]
	cutoff := uint16(fcLo) | (uint16(fcHi) << 3)
	resFilt := e.regs[0x17]
	resonance := (resFilt & SID_FILT_RES) >> 4
	routing := resFilt & 0x0F
	modeVol := e.regs[0x18]

	var cutoffNorm float32
	if e.model == SID_MODEL_8580 {
		cutoffNorm = sidFilterNorm8580Table[cutoff]
	} else {
		cutoffNorm = sidFilterNorm6581Table[cutoff]
	}
	var resNorm float32
	if e.model == SID_MODEL_8580 {
		resNorm = sid8580ResonanceTable[resonance] / 12.0
	} else {
		resNorm = sid6581ResonanceTable[resonance] / 12.0
	}

	modeMask := uint8(0)
	if modeVol&SID_MODE_LP != 0 {
		modeMask |= 0x01
	}
	if modeVol&SID_MODE_BP != 0 {
		modeMask |= 0x02
	}
	if modeVol&SID_MODE_HP != 0 {
		modeMask |= 0x04
	}

	for voice := range 3 {
		mask := uint8(1 << voice)
		if routing&mask != 0 && modeMask != 0 {
			plan.filters = append(plan.filters, sidFilterSetting{ch: e.baseChannel + voice, modeMask: modeMask, cutoff: cutoffNorm, resonance: resNorm})
		} else {
			plan.filters = append(plan.filters, sidFilterSetting{ch: e.baseChannel + voice})
		}
	}
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
	if pw >= 4095 {
		pw = 4094
	}
	return float32(pw) / 4095.0
}

// applyFrequencies updates SoundChip frequencies from SID registers
func (e *SIDEngine) applyFrequencies() {
	if e.sound == nil {
		return
	}

	for voice := range 3 {
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

	for voice := range 3 {
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
		e.sound.SetChannelSIDWaveMask(e.baseChannel+voice, mask)

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
			e.sound.SetChannelSIDTest(e.baseChannel+voice, true)
		} else {
			e.sound.SetChannelSIDTest(e.baseChannel+voice, false)
		}
	}
}

// applyEnvelopes configures ADSR envelopes for each voice
func (e *SIDEngine) applyEnvelopes() {
	if e.sound == nil {
		return
	}

	for voice := range 3 {
		base := voice * 7
		ad := e.regs[base+5]
		sr := e.regs[base+6]

		attack := (ad >> 4) & 0x0F
		decay := ad & 0x0F
		sustain := (sr >> 4) & 0x0F
		release := sr & 0x0F

		sustainLevel := float32(sustain) / 15.0

		// Use authentic SID rate counter for ADSR timing
		e.sound.SetChannelSIDRateCounter(e.baseChannel+voice, false, e.sampleRate, e.clockHz, attack, decay, release)

		// Also set time-based ADSR as fallback and for sustain level
		attackMs := sidAttackMs[attack]
		decayMs := sidDecayReleaseMs[decay]
		releaseMs := sidDecayReleaseMs[release]
		e.sound.SetChannelADSR(e.baseChannel+voice, attackMs, decayMs, releaseMs, sustainLevel)
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
	filterRoutedV3 := (e.regs[0x17] & SID_FILT_V3) != 0

	masterGain := sidVolumeGain(masterVol, e.sidPlusEnabled)

	for voice := range 3 {
		if voice == 2 && voice3Off && !filterRoutedV3 {
			e.writeChannel(voice, FLEX_OFF_VOL, 0)
			continue
		}
		if !e.voiceWave[voice] {
			e.writeChannel(voice, FLEX_OFF_VOL, 0)
			continue
		}

		// Each voice gets master volume (individual voice volume is handled by envelope)
		// Note: SID+ per-voice mix gain is applied in processEnhancedSample via ch.sidPlusGain,
		// so we only apply masterGain here to avoid double application.
		e.writeChannel(voice, FLEX_OFF_VOL, uint32(sidGainToDAC(masterGain)))
	}
}

// applyModulation sets up ring modulation and hard sync between voices
func (e *SIDEngine) applyModulation() {
	if e.sound == nil {
		return
	}

	for voice := range 3 {
		base := voice * 7
		ctrl := e.regs[base+4]

		// Ring modulation: voice N is modulated by voice N-1 (wraps: voice 0 by voice 2)
		ringMod := (ctrl&SID_CTRL_RINGMOD) != 0 && (ctrl&SID_CTRL_TRIANGLE) != 0
		sync := (ctrl & SID_CTRL_SYNC) != 0

		// Source voice for modulation (voice 0 uses voice 2, others use voice-1)
		srcVoice := e.baseChannel + ((voice + 2) % 3)

		// Ring modulation - enable bit 7, source in bits 0-1
		if ringMod {
			e.writeChannel(voice, FLEX_OFF_RINGMOD, uint32(0x80|srcVoice))
		} else {
			e.writeChannel(voice, FLEX_OFF_RINGMOD, 0)
		}

		// Hard sync - enable bit 7, source in bits 0-1
		if sync {
			e.writeChannel(voice, FLEX_OFF_SYNC, uint32(0x80|srcVoice))
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

	// Convert SID filter cutoff (0-2047) to normalized frequency
	// Use pre-computed lookup tables for optimal performance
	// Tables are initialized in sid_constants.go init()
	var cutoffNorm float32
	if e.model == SID_MODEL_8580 {
		cutoffNorm = sidFilterNorm8580Table[cutoff]
	} else {
		cutoffNorm = sidFilterNorm6581Table[cutoff]
	}

	// SID resonance uses model-specific lookup tables for authentic response
	// 6581: Non-linear, "wilder" resonance with earlier self-oscillation
	// 8580: More linear, cleaner and more controlled resonance
	var resNorm float32
	if e.model == SID_MODEL_8580 {
		resNorm = sid8580ResonanceTable[resonance] / 12.0 // Normalize to ~0-0.4 range
	} else {
		resNorm = sid6581ResonanceTable[resonance] / 12.0 // Normalize to ~0-1.0 range
	}
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

	for voice := range 3 {
		mask := uint8(1 << voice)
		if routing&mask != 0 && modeMask != 0 {
			e.sound.SetChannelFilter(e.baseChannel+voice, modeMask, cutoffNorm, resNorm)
		} else {
			e.sound.SetChannelFilter(e.baseChannel+voice, 0, 0, 0)
		}
	}
}

// writeChannel writes a value to a SoundChip channel register
func (e *SIDEngine) writeChannel(ch int, offset uint32, value uint32) {
	if e.sound == nil {
		return
	}
	if addr, ok := flexAddrForChannel(e.baseChannel+ch, offset); ok {
		e.sound.HandleRegisterWrite(addr, value)
	}
}

// sidVolumeGain converts a 4-bit SID volume level to a gain value
// Uses pre-computed lookup tables for optimal performance.
func sidVolumeGain(level uint8, sidPlus bool) float32 {
	if level > 15 {
		level = 15
	}
	if sidPlus {
		return sidPlusVolumeCurve[level]
	}
	// Linear volume curve from lookup table
	return sidLinearVolumeCurve[level]
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

	e.enabled.Store(false)
	e.channelsInit = false
	e.playing = false
	e.events = nil
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = 0
	e.loop = false
	e.loopSample = 0
	e.loopEventIndex = 0

	// Reset secondary engines if present
	if e.sid2 != nil {
		e.sid2.Reset()
	}
	if e.sid3 != nil {
		e.sid3.Reset()
	}
}

func (e *SIDEngine) SetEvents(events []SIDEvent, totalSamples uint64, loop bool, loopSample uint64) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if cap(e.events) < len(events) {
		e.events = make([]SIDEvent, len(events))
	} else {
		e.events = e.events[:len(events)]
	}
	copy(e.events, events)
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = totalSamples
	e.loop = loop
	e.loopSample = loopSample
	// Binary search for loop event index - O(log n) instead of O(n)
	e.loopEventIndex = sort.Search(len(e.events), func(i int) bool {
		return e.events[i].Sample >= loopSample
	})
	e.enabled.Store(true)
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
	e.playing = playing
	e.mutex.Unlock()
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
	e.playing = false
	e.events = nil
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = 0
	e.loop = false
	e.loopSample = 0
	e.loopEventIndex = 0
	e.mutex.Unlock()
	e.silenceChannels()
}

func (e *SIDEngine) TickSample() {
	if !e.enabled.Load() {
		return
	}

	e.mutex.Lock()

	if !e.playing {
		e.mutex.Unlock()
		return
	}

	needsSync := false
	var plusChanged bool
	var sidPlusEnabled bool
	var plusSound *SoundChip
	var plusBase int
	var secondaryEvents []SIDEvent
	for e.eventIndex < len(e.events) && e.events[e.eventIndex].Sample == e.currentSample {
		ev := e.events[e.eventIndex]
		switch {
		case ev.Chip == 0:
			changed, enabled, sound, base := e.writeRegisterStateLocked(ev.Reg, ev.Value)
			if changed {
				plusChanged = true
				sidPlusEnabled = enabled
				plusSound = sound
				plusBase = base
			}
			needsSync = true
		case ev.Chip == 1 && e.sid2 != nil:
			secondaryEvents = append(secondaryEvents, ev)
		case ev.Chip == 2 && e.sid3 != nil:
			secondaryEvents = append(secondaryEvents, ev)
		}
		e.eventIndex++
	}

	if e.debugEnabled && e.currentSample < e.debugUntil && e.currentSample >= e.debugNextTick {
		seconds := float64(e.currentSample) / float64(e.sampleRate)
		fmt.Printf("SID t=%.1fs\n", seconds)
		e.debugNextTick += uint64(e.sampleRate)
	}

	e.currentSample++

	if e.totalSamples > 0 && e.currentSample >= e.totalSamples {
		if e.loop {
			e.currentSample = e.loopSample
			e.eventIndex = e.loopEventIndex
		} else {
			e.playing = false
			needsSync = false
			e.mutex.Unlock()
			if plusChanged && plusSound != nil {
				plusSound.SetSIDPlusEnabledForRange(sidPlusEnabled, plusBase, 3)
			}
			for _, ev := range secondaryEvents {
				switch {
				case ev.Chip == 1 && e.sid2 != nil:
					e.sid2.WriteRegister(ev.Reg, ev.Value)
				case ev.Chip == 2 && e.sid3 != nil:
					e.sid3.WriteRegister(ev.Reg, ev.Value)
				}
			}
			e.silenceChannels()
			return
		}
	}
	e.mutex.Unlock()

	if plusChanged && plusSound != nil {
		plusSound.SetSIDPlusEnabledForRange(sidPlusEnabled, plusBase, 3)
	}
	if needsSync {
		e.syncToChip()
	}
	for _, ev := range secondaryEvents {
		switch {
		case ev.Chip == 1 && e.sid2 != nil:
			e.sid2.WriteRegister(ev.Reg, ev.Value)
		case ev.Chip == 2 && e.sid3 != nil:
			e.sid3.WriteRegister(ev.Reg, ev.Value)
		}
	}
}

func (e *SIDEngine) silenceChannels() {
	if e.sound == nil {
		return
	}
	for ch := range 3 {
		e.writeChannel(ch, FLEX_OFF_VOL, 0)
	}
}

func (e *SIDEngine) writeRegisterStateLocked(reg uint8, value uint8) (bool, bool, *SoundChip, int) {
	if reg >= SID_REG_COUNT {
		return false, false, nil, 0
	}

	if e.debugEnabled && e.currentSample < e.debugUntil {
		e.debugRegisterWrite(reg, value)
	}

	e.enabled.Store(true)
	e.regs[reg] = value
	if mem := e.busMemory; mem != nil {
		if idx := e.regBase + uint32(reg); idx < uint32(len(mem)) {
			mem[idx] = value
		}
	}

	// Handle SID+ control register (scoped to this engine's channels only)
	if reg == 0x19 { // SID_PLUS_CTRL offset
		e.sidPlusEnabled = (value & 1) != 0
		return true, e.sidPlusEnabled, e.sound, e.baseChannel
	}

	return false, false, nil, 0
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
