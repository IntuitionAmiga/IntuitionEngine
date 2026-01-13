// psg_engine.go - AY/YM register translation and per-sample scheduling.

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

const (
	PSG_BASE      = 0xFC00
	PSG_END       = 0xFC0D
	PSG_REG_COUNT = 14

	PSG_CLOCK_ATARI_ST    = 2000000
	PSG_CLOCK_ZX_SPECTRUM = 1773400
	PSG_CLOCK_CPC         = 1000000
	PSG_CLOCK_MSX         = 1789773
)

type PSGEvent struct {
	Sample uint64
	Reg    uint8
	Value  uint8
}

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
	envHold          bool

	events         []PSGEvent
	eventIndex     int
	currentSample  uint64
	totalSamples   uint64
	loop           bool
	loopSample     uint64
	loopEventIndex int
	playing        bool
	enabled        bool

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
	attack := (shape & 0x04) != 0
	if attack {
		e.envLevel = 0
		e.envDirection = 1
	} else {
		e.envLevel = 15
		e.envDirection = -1
	}
	e.envHold = false
}

func (e *PSGEngine) advanceEnvelope() {
	e.envSampleCounter++
	if e.envSampleCounter < e.envPeriodSamples {
		return
	}

	steps := int(e.envSampleCounter / e.envPeriodSamples)
	e.envSampleCounter -= float64(steps) * e.envPeriodSamples

	for i := 0; i < steps; i++ {
		if e.envHold {
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
			shape := e.regs[13] & 0x0F
			cont := (shape & 0x08) != 0
			hold := (shape & 0x02) != 0
			alt := (shape & 0x01) != 0

			if !cont {
				e.envLevel = 0
				e.envHold = true
				break
			}
			if hold {
				e.envHold = true
				break
			}
			if alt {
				e.envDirection = -e.envDirection
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
	e.writeChannel(3, FLEX_OFF_NOISEMODE, NOISE_MODE_WHITE)
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
		e.writeChannel(ch, FLEX_OFF_FREQ, uint32(freq))
	}

	noisePeriod := uint16(e.regs[6] & 0x1F)
	if noisePeriod == 0 {
		noisePeriod = 1
	}
	noiseFreq := float64(e.clockHz) / (16.0 * float64(noisePeriod))
	e.writeChannel(3, FLEX_OFF_FREQ, uint32(noiseFreq))
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

	noiseVolume := uint8(0)
	for ch := 0; ch < 3; ch++ {
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
		e.writeChannel(ch, FLEX_OFF_VOL, uint32(toneLevel)*17)

		noiseLevel := level
		if !noiseEnable[ch] {
			noiseLevel = 0
		}
		if noiseLevel > noiseVolume {
			noiseVolume = noiseLevel
		}
	}

	if noiseVolume == 0 {
		e.writeChannel(3, FLEX_OFF_VOL, 0)
		return
	}
	e.writeChannel(3, FLEX_OFF_VOL, uint32(noiseVolume)*17)
}

func (e *PSGEngine) writeChannel(ch int, offset uint32, value uint32) {
	base := FLEX_CH_BASE + uint32(ch)*FLEX_CH_STRIDE
	e.sound.HandleRegisterWrite(base+offset, value)
}

type PSGMetadata struct {
	Title  string
	Author string
	System string
}

func isPSGExtension(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ym", ".ay", ".vgm", ".vgz":
		return true
	default:
		return false
	}
}
