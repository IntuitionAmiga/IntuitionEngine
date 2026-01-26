// psg_engine.go - AY/YM register translation and per-sample scheduling.

package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
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

	events         []PSGEvent
	eventIndex     int
	currentSample  uint64
	totalSamples   uint64
	loop           bool
	loopSample     uint64
	loopEventIndex int
	playing        bool
	enabled        bool
	psgPlusEnabled bool

	channelsInit bool
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

func (e *PSGEngine) SetPSGPlusEnabled(enabled bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.psgPlusEnabled = enabled
	if e.sound != nil {
		e.sound.SetPSGPlusEnabled(enabled)
		e.syncToChip()
	}
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

	e.enabled = true
	e.regs[reg] = value
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
	e.enabled = true
	e.updateEnvPeriodSamples()
	e.resetEnvelope()
	e.syncToChip()
	e.mutex.Unlock()
	return nil
}

func (e *PSGEngine) SetEvents(events []PSGEvent, totalSamples uint64, loop bool, loopSample uint64) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.events = events
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = totalSamples
	e.loop = loop
	e.loopSample = loopSample
	e.loopEventIndex = 0
	e.playing = true
	e.enabled = true

	if loop {
		for i, ev := range events {
			if ev.Sample >= loopSample {
				e.loopEventIndex = i
				break
			}
		}
	}
}

func (e *PSGEngine) SetPlaying(playing bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = playing
	if playing {
		e.enabled = true
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
	}
}

func (e *PSGEngine) IsPlaying() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.playing
}

func (e *PSGEngine) StopPlayback() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = false
	e.events = nil
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = 0
}

func (e *PSGEngine) TickSample() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if !e.enabled {
		return
	}

	e.advanceEnvelope()

	if e.playing {
		for e.eventIndex < len(e.events) && e.events[e.eventIndex].Sample == e.currentSample {
			ev := e.events[e.eventIndex]
			if ev.Reg < PSG_REG_COUNT {
				e.regs[ev.Reg] = ev.Value
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

func (e *PSGEngine) silenceChannels() {
	if e.sound == nil {
		return
	}
	for ch := 0; ch < 4; ch++ {
		e.writeChannel(ch, FLEX_OFF_VOL, 0)
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
}

func (e *PSGEngine) advanceEnvelope() {
	e.envSampleCounter++
	if e.envSampleCounter < e.envPeriodSamples {
		return
	}

	steps := int(e.envSampleCounter / e.envPeriodSamples)
	e.envSampleCounter -= float64(steps) * e.envPeriodSamples

	for i := 0; i < steps; i++ {
		if e.envHoldActive {
			break
		}

		e.envLevel += e.envDirection
		if e.envLevel > 15 {
			e.envLevel = 15
		}
		if e.envLevel < 0 {
			e.envLevel = 0
		}

		if e.envLevel == 0 || e.envLevel == 15 {
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
			}
			if e.envDirection > 0 {
				e.envLevel = 0
			} else {
				e.envLevel = 15
			}
		}
	}

	e.applyVolumes()
}

func (e *PSGEngine) ensureChannelsInitialized() {
	if e.channelsInit || e.sound == nil {
		return
	}

	for ch := 0; ch < 3; ch++ {
		e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
		e.writeChannel(ch, FLEX_OFF_DUTY, 0x0080)
		e.writeChannel(ch, FLEX_OFF_PWM_CTRL, 0)
		e.writeChannel(ch, FLEX_OFF_ATK, 0)
		e.writeChannel(ch, FLEX_OFF_DEC, 0)
		e.writeChannel(ch, FLEX_OFF_SUS, 255)
		e.writeChannel(ch, FLEX_OFF_REL, 0)
		e.writeChannel(ch, FLEX_OFF_CTRL, 3)
	}

	e.writeChannel(3, FLEX_OFF_WAVE_TYPE, WAVE_NOISE)
	e.writeChannel(3, FLEX_OFF_NOISEMODE, NOISE_MODE_PSG)
	e.writeChannel(3, FLEX_OFF_ATK, 0)
	e.writeChannel(3, FLEX_OFF_DEC, 0)
	e.writeChannel(3, FLEX_OFF_SUS, 255)
	e.writeChannel(3, FLEX_OFF_REL, 0)
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

	for ch := 0; ch < 3; ch++ {
		low := uint16(e.regs[ch*2])
		high := uint16(e.regs[ch*2+1] & 0x0F)
		period := (high << 8) | low
		if period == 0 {
			e.writeChannel(ch, FLEX_OFF_FREQ, 0)
			continue
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

	var noiseSum float32
	for ch := 0; ch < 3; ch++ {
		vol := e.regs[8+ch]
		useEnv := (vol & 0x10) != 0
		level := vol & 0x0F
		if useEnv {
			// For SID emulation: upper nibble often contains the actual volume
			// when bit 4 is set. Check if upper nibble is non-zero before
			// falling back to envelope generator.
			upperNibble := vol >> 4
			if upperNibble > 0 {
				level = upperNibble
			} else {
				level = uint8(e.envLevel)
			}
		}
		toneLevel := level
		if !toneEnable[ch] {
			toneLevel = 0
		}
		toneGain := psgVolumeGain(toneLevel, e.psgPlusEnabled)
		e.writeChannel(ch, FLEX_OFF_VOL, uint32(psgGainToDAC(toneGain)))

		noiseLevel := level
		if !noiseEnable[ch] {
			noiseLevel = 0
		}
		if noiseLevel > 0 {
			noiseSum += psgVolumeGain(noiseLevel, e.psgPlusEnabled)
		}
	}

	if noiseSum <= 0 {
		e.writeChannel(3, FLEX_OFF_VOL, 0)
		return
	}
	if noiseSum > 1.0 {
		noiseSum = 1.0
	}
	e.writeChannel(3, FLEX_OFF_VOL, uint32(psgGainToDAC(noiseSum)))
}

func (e *PSGEngine) writeChannel(ch int, offset uint32, value uint32) {
	base := FLEX_CH_BASE + uint32(ch)*FLEX_CH_STRIDE
	e.sound.HandleRegisterWrite(base+offset, value)
}

var psgPlusMixGain = [3]float32{1.05, 1.0, 0.95}

var psgPlusVolumeCurve = func() [16]float32 {
	var curve [16]float32
	curve[0] = 0
	for i := 1; i < len(curve); i++ {
		db := float64(i-15) * 2.0
		curve[i] = float32(math.Pow(10.0, db/20.0))
	}
	curve[15] = 1.0
	return curve
}()

func psgVolumeGain(level uint8, psgPlus bool) float32 {
	if level > 15 {
		level = 15
	}
	if psgPlus {
		return psgPlusVolumeCurve[level]
	}
	return float32(level) / 15.0
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
	case ".ym", ".ay", ".vgm", ".vgz", ".snd", ".sndh":
		return true
	default:
		return false
	}
}
