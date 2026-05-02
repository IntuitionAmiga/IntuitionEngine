// psg_engine.go - AY/YM register translation and per-sample scheduling.

package main

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type PSGEngine struct {
	mutex      sync.Mutex
	sound      *SoundChip
	sampleRate int
	clockHz    uint32

	regs [PSG_REG_COUNT]uint8

	envPeriodSamples float64
	envSampleCounter float64
	envLevel         int
	envDirection     int
	envContinue      bool
	envAlternate     bool
	envAttack        bool
	envHoldRequest   bool
	envHoldActive    bool

	events          []PSGEvent
	eventIndex      int
	currentSample   uint64
	totalSamples    uint64
	loop            bool
	loopSample      uint64
	loopEventIndex  int
	playing         bool
	enabled         atomic.Bool
	psgPlusEnabled  bool
	useLegacyLinear bool

	channelsInit bool
	busMemory    []byte // mirror register writes for Machine Monitor visibility

	snEvents         []SNEvent
	snEventIndex     int
	snLoopEventIndex int
	snChip           *SN76489Chip
}

func NewPSGEngine(sound *SoundChip, sampleRate int) *PSGEngine {
	engine := &PSGEngine{
		sound:        sound,
		sampleRate:   sampleRate,
		clockHz:      PSG_CLOCK_ATARI_ST,
		envLevel:     15,
		envDirection: -1,
	}
	engine.updateEnvPeriodSamples()
	if sound != nil {
		sound.SetSampleTicker(engine)
	}
	return engine
}

func (e *PSGEngine) AttachBusMemory(mem []byte) {
	e.busMemory = mem
}

func (e *PSGEngine) SetPSGPlusEnabled(enabled bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.psgPlusEnabled = enabled
	if e.sound != nil {
		e.sound.SetPSGPlusEnabled(enabled)
		e.syncToChip()
	}
}

func (e *PSGEngine) SetLegacyLinearVolume(enabled bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.useLegacyLinear = enabled
	e.syncToChip()
}

func (e *PSGEngine) PSGPlusEnabled() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.psgPlusEnabled
}

func (e *PSGEngine) HandlePSGPlusWrite(addr uint32, value uint32) {
	if addr != PSG_PLUS_CTRL {
		return
	}
	e.SetPSGPlusEnabled(value&1 != 0)
}

func (e *PSGEngine) HandlePSGPlusRead(addr uint32) uint32 {
	if addr != PSG_PLUS_CTRL {
		return 0
	}
	if e.PSGPlusEnabled() {
		return 1
	}
	return 0
}

func (e *PSGEngine) SetClockHz(clock uint32) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if clock == 0 {
		return
	}
	e.clockHz = clock
	e.updateEnvPeriodSamples()
}

func (e *PSGEngine) HandleWrite(addr uint32, value uint32) {
	if addr < PSG_BASE || addr > PSG_END {
		return
	}
	reg := uint8(addr - PSG_BASE)
	e.WriteRegister(reg, uint8(value))
}

func (e *PSGEngine) HandleWrite8(addr uint32, value uint8) {
	if addr < PSG_BASE || addr > PSG_END {
		return
	}
	reg := uint8(addr - PSG_BASE)
	e.WriteRegister(reg, value)
}

func (e *PSGEngine) HandleRead(addr uint32) uint32 {
	if addr < PSG_BASE || addr > PSG_END {
		return 0
	}
	reg := uint8(addr - PSG_BASE)
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return uint32(e.regs[reg])
}

func (e *PSGEngine) WriteRegister(reg uint8, value uint8) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if reg >= PSG_REG_COUNT {
		return
	}

	e.enabled.Store(true)
	e.regs[reg] = value
	if mem := e.busMemory; mem != nil {
		mem[PSG_BASE+uint32(reg)] = value
	}
	if reg == 11 || reg == 12 {
		e.updateEnvPeriodSamples()
	}
	if reg == 13 {
		e.resetEnvelope()
	}

	e.syncToChip()
}

func (e *PSGEngine) ApplyFrame(frame []uint8) error {
	if len(frame) < PSG_REG_COUNT {
		return fmt.Errorf("psg frame too short: %d", len(frame))
	}
	e.mutex.Lock()
	copy(e.regs[:], frame[:PSG_REG_COUNT])
	e.enabled.Store(true)
	e.updateEnvPeriodSamples()
	e.resetEnvelope()
	e.syncToChip()
	e.mutex.Unlock()
	return nil
}

func (e *PSGEngine) SetEvents(events []PSGEvent, totalSamples uint64, loop bool, loopSample uint64) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	// Reset register state to prevent stale values from previous playback
	// (e.g. mixer reg 7 from SNDH disabling tone channels in a subsequent VGM).
	e.regs = [PSG_REG_COUNT]uint8{}
	e.envLevel = 15
	e.envDirection = -1
	e.envSampleCounter = 0
	e.envContinue = false
	e.envAlternate = false
	e.envAttack = false
	e.envHoldRequest = false
	e.envHoldActive = false
	e.updateEnvPeriodSamples()
	e.events = events
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = totalSamples
	e.loop = loop
	e.loopSample = loopSample
	e.loopEventIndex = 0
	e.playing = true
	e.enabled.Store(true)

	if loop {
		// Binary search for loop event index - O(log n) instead of O(n)
		e.loopEventIndex = sort.Search(len(events), func(i int) bool {
			return events[i].Sample >= loopSample
		})
	}
}

func (e *PSGEngine) SetSNStream(snEvents []SNEvent, chip *SN76489Chip, clockHz uint32) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.snEvents = snEvents
	e.snEventIndex = 0
	e.snLoopEventIndex = 0
	e.snChip = chip
	if chip != nil && clockHz != 0 {
		chip.SetClockHz(clockHz)
	}
	if e.loop {
		e.snLoopEventIndex = sort.Search(len(snEvents), func(i int) bool {
			return snEvents[i].Sample >= e.loopSample
		})
	}
}

func (e *PSGEngine) SetPlaying(playing bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = playing
	if playing {
		e.enabled.Store(true)
	}
}

// SetForceLoop enables looping from the start of the track, even if
// the file has no native loop point. This overrides the file's loop setting.
func (e *PSGEngine) SetForceLoop(enable bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if enable {
		e.loop = true
		e.loopSample = 0
		e.loopEventIndex = 0
		e.snLoopEventIndex = 0
	}
}

func (e *PSGEngine) IsPlaying() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.playing
}

func (e *PSGEngine) PlaybackComplete() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return !e.playing && e.totalSamples > 0 && e.currentSample >= e.totalSamples
}

func (e *PSGEngine) StopPlayback() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = false
	e.events = nil
	e.eventIndex = 0
	e.snEvents = nil
	e.snEventIndex = 0
	e.snLoopEventIndex = 0
	e.currentSample = 0
	e.totalSamples = 0
	e.silenceSNLocked()
	e.silenceChannels()
	// Reset channelsInit so the next song triggers fresh channel initialization.
	// Without this, stale SoundChip channel state (gate, envelope, sidEnvelope)
	// from the current song persists into the next one, causing silence.
	e.channelsInit = false
}

func (e *PSGEngine) TickSample() {
	if !e.enabled.Load() {
		return
	}

	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.advanceEnvelope()

	if !e.playing {
		return
	}

	for e.eventIndex < len(e.events) && e.events[e.eventIndex].Sample == e.currentSample {
		ev := e.events[e.eventIndex]
		if ev.Reg < PSG_REG_COUNT {
			e.regs[ev.Reg] = ev.Value
			if mem := e.busMemory; mem != nil {
				mem[PSG_BASE+uint32(ev.Reg)] = ev.Value
			}
			if ev.Reg == 11 || ev.Reg == 12 {
				e.updateEnvPeriodSamples()
			}
			if ev.Reg == 13 {
				e.resetEnvelope()
			}
			e.syncToChip()
		}
		e.eventIndex++
	}
	for e.snEventIndex < len(e.snEvents) && e.snEvents[e.snEventIndex].Sample <= e.currentSample {
		ev := e.snEvents[e.snEventIndex]
		if e.snChip != nil {
			e.snChip.HandleWrite8(SN_PORT_WRITE, ev.Byte)
		}
		e.snEventIndex++
	}

	e.currentSample++

	if e.totalSamples > 0 && e.currentSample >= e.totalSamples {
		if e.loop {
			e.currentSample = e.loopSample
			e.eventIndex = e.loopEventIndex
			e.snEventIndex = e.snLoopEventIndex
		} else {
			e.playing = false
			e.silenceSNLocked()
			e.silenceChannels()
		}
	}
}

func (e *PSGEngine) silenceSNLocked() {
	if e.snChip == nil {
		return
	}
	e.snChip.HandleWrite8(SN_PORT_WRITE, 0x9F)
	e.snChip.HandleWrite8(SN_PORT_WRITE, 0xBF)
	e.snChip.HandleWrite8(SN_PORT_WRITE, 0xDF)
	e.snChip.HandleWrite8(SN_PORT_WRITE, 0xFF)
}

func (e *PSGEngine) silenceChannels() {
	if e.sound == nil {
		return
	}
	for ch := range 4 {
		e.writeChannel(ch, FLEX_OFF_VOL, 0)
	}
	for ch := range 3 {
		if sndCh := e.sound.channels[ch]; sndCh != nil {
			sndCh.noiseMix = 0
		}
	}
}

func (e *PSGEngine) updateEnvPeriodSamples() {
	period := uint16(e.regs[11]) | uint16(e.regs[12])<<8
	if period == 0 {
		period = 1
	}
	e.envPeriodSamples = float64(e.sampleRate) * 256.0 * float64(period) / float64(e.clockHz)
	if e.envPeriodSamples <= 0 {
		e.envPeriodSamples = 1
	}
}

func (e *PSGEngine) resetEnvelope() {
	shape := e.regs[13] & 0x0F
	e.envContinue = (shape & 0x08) != 0
	e.envAttack = (shape & 0x04) != 0
	e.envAlternate = (shape & 0x02) != 0
	e.envHoldRequest = (shape & 0x01) != 0
	e.envHoldActive = false
	if e.envAttack {
		e.envLevel = 0
		e.envDirection = 1
	} else {
		e.envLevel = 15
		e.envDirection = -1
	}
	e.envSampleCounter = 0
}

func (e *PSGEngine) advanceEnvelope() {
	e.envSampleCounter++
	if e.envSampleCounter < e.envPeriodSamples {
		return
	}

	steps := int(e.envSampleCounter / e.envPeriodSamples)
	e.envSampleCounter -= float64(steps) * e.envPeriodSamples

	for range steps {
		if e.envHoldActive {
			break
		}

		e.envLevel += e.envDirection

		if e.envLevel > 15 || e.envLevel < 0 {
			// Clamp to valid range
			if e.envLevel > 15 {
				e.envLevel = 15
			}
			if e.envLevel < 0 {
				e.envLevel = 0
			}

			if !e.envContinue {
				e.envLevel = 0
				e.envHoldActive = true
				break
			}
			if e.envHoldRequest {
				e.envHoldActive = true
				if e.envAlternate {
					if e.envDirection > 0 {
						e.envLevel = 0
					} else {
						e.envLevel = 15
					}
				}
				break
			}
			if e.envAlternate {
				e.envDirection = -e.envDirection
			} else {
				// Non-alternating: wrap to start of ramp
				if e.envDirection > 0 {
					e.envLevel = 0
				} else {
					e.envLevel = 15
				}
			}
		}
	}

	e.applyVolumes()
}

func (e *PSGEngine) ensureChannelsInitialized() {
	if e.channelsInit || e.sound == nil {
		return
	}

	for ch := range 3 {
		// Gate off first to clear stale envelope state from other engines (e.g. SID).
		// This ensures the subsequent gate-on triggers a fresh attack phase.
		e.writeChannel(ch, FLEX_OFF_CTRL, 0)
		e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
		e.writeChannel(ch, FLEX_OFF_DUTY, 0x0080)
		e.writeChannel(ch, FLEX_OFF_PWM_CTRL, 0)
		e.writeChannel(ch, FLEX_OFF_ATK, 0)
		e.writeChannel(ch, FLEX_OFF_DEC, 0)
		e.writeChannel(ch, FLEX_OFF_SUS, 255)
		e.writeChannel(ch, FLEX_OFF_REL, 0)
		// Clear SID envelope mode so standard ADSR is used
		if sndCh := e.sound.channels[ch]; sndCh != nil {
			sndCh.sidEnvelope = false
			sndCh.noiseMix = 0
			sndCh.noiseMode = NOISE_MODE_PSG
			sndCh.noiseFrequency = 0
		}
		e.writeChannel(ch, FLEX_OFF_CTRL, 3)
	}

	// Gate off noise channel first too
	e.writeChannel(3, FLEX_OFF_CTRL, 0)
	e.writeChannel(3, FLEX_OFF_WAVE_TYPE, WAVE_NOISE)
	e.writeChannel(3, FLEX_OFF_NOISEMODE, NOISE_MODE_PSG)
	e.writeChannel(3, FLEX_OFF_ATK, 0)
	e.writeChannel(3, FLEX_OFF_DEC, 0)
	e.writeChannel(3, FLEX_OFF_SUS, 255)
	e.writeChannel(3, FLEX_OFF_REL, 0)
	if sndCh := e.sound.channels[3]; sndCh != nil {
		sndCh.sidEnvelope = false
	}
	e.writeChannel(3, FLEX_OFF_CTRL, 3)

	e.channelsInit = true
}

func (e *PSGEngine) syncToChip() {
	e.ensureChannelsInitialized()
	e.applyFrequencies()
	e.applyVolumes()
}

func (e *PSGEngine) applyFrequencies() {
	if e.sound == nil {
		return
	}

	for ch := range 3 {
		low := uint16(e.regs[ch*2])
		high := uint16(e.regs[ch*2+1] & 0x0F)
		period := (high << 8) | low
		if period == 0 {
			period = 1
		}
		freq := float64(e.clockHz) / (16.0 * float64(period))
		e.writeChannel(ch, FLEX_OFF_FREQ, uint32(freq*256)) // 16.8 fixed-point
	}

	noisePeriod := uint16(e.regs[6] & 0x1F)
	if noisePeriod == 0 {
		noisePeriod = 1
	}
	noiseFreq := float64(e.clockHz) / (16.0 * float64(noisePeriod))
	e.writeChannel(3, FLEX_OFF_FREQ, uint32(noiseFreq*256)) // 16.8 fixed-point
	for ch := range 3 {
		if sndCh := e.sound.channels[ch]; sndCh != nil {
			sndCh.noiseFrequency = float32(noiseFreq)
		}
	}
}

func (e *PSGEngine) applyVolumes() {
	if e.sound == nil {
		return
	}

	mixer := e.regs[7]
	toneEnable := [3]bool{
		(mixer & 0x01) == 0,
		(mixer & 0x02) == 0,
		(mixer & 0x04) == 0,
	}
	noiseEnable := [3]bool{
		(mixer & 0x08) == 0,
		(mixer & 0x10) == 0,
		(mixer & 0x20) == 0,
	}

	for ch := range 3 {
		vol := e.regs[8+ch]
		useEnv := (vol & 0x10) != 0
		level := vol & 0x0F
		if useEnv {
			level = uint8(e.envLevel)
		}
		toneLevel := level
		if !toneEnable[ch] {
			toneLevel = 0
		}
		toneGain := e.volumeGain(toneLevel)
		if e.psgPlusEnabled {
			toneGain *= psgPlusMixGain[ch]
			if toneGain > 1.0 {
				toneGain = 1.0
			}
		}
		e.writeChannel(ch, FLEX_OFF_VOL, uint32(psgGainToDAC(toneGain)))

		noiseLevel := level
		if !noiseEnable[ch] {
			noiseLevel = 0
		}
		if sndCh := e.sound.channels[ch]; sndCh != nil {
			sndCh.noiseMix = e.volumeGain(noiseLevel)
			if e.psgPlusEnabled {
				sndCh.noiseMix *= psgPlusMixGain[ch]
				if sndCh.noiseMix > 1.0 {
					sndCh.noiseMix = 1.0
				}
			}
		}
	}

	e.writeChannel(3, FLEX_OFF_VOL, 0)
}

func (e *PSGEngine) volumeGain(level uint8) float32 {
	if e.useLegacyLinear {
		return psgLegacyLinearVolumeCurve[level&0x0F]
	}
	return psgVolumeGain(level, e.psgPlusEnabled)
}

func (e *PSGEngine) writeChannel(ch int, offset uint32, value uint32) {
	base := FLEX_CH_BASE + uint32(ch)*FLEX_CH_STRIDE
	e.sound.HandleRegisterWrite(base+offset, value)
}

var psgPlusMixGain = [3]float32{1.05, 1.0, 0.95}

var psgLogVolumeCurve = [16]float32{
	0.000000,
	0.004654,
	0.007721,
	0.010955,
	0.016998,
	0.025085,
	0.036926,
	0.051636,
	0.077637,
	0.112671,
	0.163055,
	0.230682,
	0.330017,
	0.468689,
	0.664581,
	1.000000,
}

var psgLegacyLinearVolumeCurve = [16]float32{
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

// psgVolumeGain converts a 4-bit PSG volume level to a gain value
// Uses pre-computed lookup tables for optimal performance.
func psgVolumeGain(level uint8, psgPlus bool) float32 {
	if level > 15 {
		level = 15
	}
	if psgPlus {
		return psgLogVolumeCurve[level]
	}
	return psgLogVolumeCurve[level]
}

func psgGainToDAC(gain float32) uint8 {
	if gain <= 0 {
		return 0
	}
	if gain >= 1.0 {
		return 255
	}
	return uint8(math.Round(float64(gain * 255.0)))
}

func psgVolumeToDAC(level uint8, psgPlus bool) uint8 {
	return psgGainToDAC(psgVolumeGain(level, psgPlus))
}

func isPSGExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ym", ".ay", ".vgm", ".vgz", ".snd", ".sndh",
		".vtx", ".pt3", ".pt2", ".pt1", ".stc", ".sqt", ".asc", ".ftc":
		return true
	default:
		return false
	}
}
