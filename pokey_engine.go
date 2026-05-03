// pokey_engine.go - POKEY sound chip register mapping for the Intuition Engine

/*
POKEY (Pot Keyboard Integrated Circuit) was the audio chip used in Atari 8-bit
computers and the Atari 5200. This implementation provides pure register mapping
to the SoundChip for audio synthesis, accessible from all CPUs (M68K, IE32, Z80, 6502).

Features:
- 4 audio channels mapped to SoundChip channels 0-3
- AUDF/AUDC register translation to frequency/volume/waveform
- 16-bit channel linking (Ch1+Ch2, Ch3+Ch4) for extended frequency range
- Distortion modes mapped to SoundChip waveform types (square/noise)
- POKEY+ enhanced mode with logarithmic volume curve
- Event-based playback for SAP file rendering

The engine translates POKEY register writes to SoundChip channel parameters.
Synthesis is performed by SoundChip - this module handles only the mapping.
*/

package main

import (
	"math"
	"sync"
	"sync/atomic"
)

// POKEYEngine emulates the POKEY sound chip via register mapping to SoundChip
type POKEYEngine struct {
	mutex       sync.Mutex
	sound       *SoundChip
	sampleRate  int
	clockHz     uint32
	baseChannel int

	regs [POKEY_REG_COUNT]uint8

	pokeyPlusEnabled bool
	channelsInit     bool
	right            *POKEYEngine
	randomSR         uint32

	// Pre-computed clock values for fast frequency calculation
	clock179MHz float64 // Full clock rate (1.79MHz)
	clock64KHz  float64 // Clock / DIV_64KHZ (~64kHz)
	clock15KHz  float64 // Clock / DIV_15KHZ (~15kHz)

	// Event-based playback (for SAP files)
	events        []SAPPOKEYEvent
	eventIndex    int
	currentSample uint64
	totalSamples  uint64
	playing       atomic.Bool
	loop          bool
	loopSample    uint64
	forceLoop     bool

	busMemory []byte // mirror register writes for Machine Monitor visibility
}

// pokeyLinearVolumeCurve provides pre-computed linear volume values for standard POKEY
// Index 0-15 corresponds to the 4-bit volume register value
var pokeyLinearVolumeCurve = [16]float32{
	0.0 / 15.0, 1.0 / 15.0, 2.0 / 15.0, 3.0 / 15.0,
	4.0 / 15.0, 5.0 / 15.0, 6.0 / 15.0, 7.0 / 15.0,
	8.0 / 15.0, 9.0 / 15.0, 10.0 / 15.0, 11.0 / 15.0,
	12.0 / 15.0, 13.0 / 15.0, 14.0 / 15.0, 15.0 / 15.0,
}

// POKEY+ logarithmic volume curve (2dB per step, more accurate to hardware DAC)
var pokeyPlusVolumeCurve = func() [16]float32 {
	var curve [16]float32
	curve[0] = 0
	for i := 1; i < 16; i++ {
		db := float64(i-15) * 2.0
		curve[i] = float32(math.Pow(10.0, db/20.0))
	}
	curve[15] = 1.0
	return curve
}()

// pokeyPlusMixGain provides per-channel gain adjustment for POKEY+ mode
// POKEY has 4 channels, slight variation adds stereo-like width
var pokeyPlusMixGain = [4]float32{1.03, 0.98, 1.02, 0.97}

// NewPOKEYEngine creates a new POKEY emulation engine
func NewPOKEYEngine(sound *SoundChip, sampleRate int) *POKEYEngine {
	return NewPOKEYEngineMulti(sound, sampleRate, 0)
}

func NewPOKEYEngineMulti(sound *SoundChip, sampleRate int, baseChannel int) *POKEYEngine {
	e := &POKEYEngine{
		sound:       sound,
		sampleRate:  sampleRate,
		clockHz:     POKEY_CLOCK_NTSC,
		baseChannel: baseChannel,
		randomSR:    0x1FFFF,
	}
	// Pre-compute clock divisors for fast frequency calculation
	e.updateClockDivisors()
	return e
}

type pokeySyncState struct {
	sound            *SoundChip
	regs             [POKEY_REG_COUNT]uint8
	pokeyPlusEnabled bool
	baseChannel      int
	initChannels     bool
	clock179MHz      float64
	clock64KHz       float64
	clock15KHz       float64
}

func (e *POKEYEngine) setRight(right *POKEYEngine) {
	if e.right != nil || right == nil || right == e || right.right != nil {
		panic("invalid POKEY stereo right-engine assignment")
	}
	e.right = right
}

func (e *POKEYEngine) AttachBusMemory(mem []byte) {
	e.busMemory = mem
}

// SetClockHz sets the POKEY master clock frequency
func (e *POKEYEngine) SetClockHz(clock uint32) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if clock == 0 {
		return
	}
	e.clockHz = clock
	e.updateClockDivisors()
}

// updateClockDivisors pre-computes clock divisors for fast frequency calculation
func (e *POKEYEngine) updateClockDivisors() {
	e.clock179MHz = float64(e.clockHz)
	e.clock64KHz = float64(e.clockHz) / float64(POKEY_DIV_64KHZ)
	e.clock15KHz = float64(e.clockHz) / float64(POKEY_DIV_15KHZ)
}

// HandleWrite processes a write to a POKEY register via memory-mapped I/O
func (e *POKEYEngine) HandleWrite(addr uint32, value uint32) {
	if addr < POKEY_BASE || addr > POKEY_END {
		return
	}
	reg := uint8(addr - POKEY_BASE)
	e.WriteRegister(reg, uint8(value))
}

func (e *POKEYEngine) HandleWrite8(addr uint32, value uint8) {
	if addr < POKEY_BASE || addr > POKEY_END {
		return
	}
	e.WriteRegister(uint8(addr-POKEY_BASE), value)
}

// HandleRead processes a read from a POKEY register
func (e *POKEYEngine) HandleRead(addr uint32) uint32 {
	if addr < POKEY_BASE || addr > POKEY_END {
		return 0
	}
	reg := uint8(addr - POKEY_BASE)
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if reg == POKEY_RANDOM-POKEY_BASE {
		return uint32(e.nextRandomLocked())
	}
	return uint32(e.regs[reg])
}

func (e *POKEYEngine) nextRandomLocked() uint8 {
	if e.randomSR == 0 {
		e.randomSR = 0x1FFFF
	}
	newBit := ((e.randomSR >> 0) ^ (e.randomSR >> 5)) & 1
	e.randomSR = ((e.randomSR >> 1) | (newBit << 16)) & 0x1FFFF
	return uint8(e.randomSR ^ (e.randomSR >> 8))
}

// WriteRegister writes a value to a POKEY register
func (e *POKEYEngine) WriteRegister(reg uint8, value uint8) {
	e.mutex.Lock()

	if reg >= POKEY_REG_COUNT {
		e.mutex.Unlock()
		return
	}
	if reg == POKEY_RANDOM-POKEY_BASE {
		e.mutex.Unlock()
		return
	}

	oldValue := e.regs[reg]
	e.regs[reg] = value
	if mem := e.busMemory; mem != nil {
		mem[POKEY_BASE+uint32(reg)] = value
	}

	var right *POKEYEngine
	pokeyPlusChanged := false
	if reg == POKEY_PLUS_CTRL-POKEY_BASE {
		e.pokeyPlusEnabled = (value & 1) != 0
		pokeyPlusChanged = true
		right = e.right
	}
	state := e.snapshotSyncStateLocked()
	e.mutex.Unlock()

	if pokeyPlusChanged && state.sound != nil {
		state.sound.SetPOKEYPlusEnabledForRange(state.baseChannel, 4, state.pokeyPlusEnabled)
	}
	if pokeyPlusChanged && right != nil {
		right.SetPOKEYPlusEnabled(state.pokeyPlusEnabled)
	}
	applyPOKEYSyncState(state)
	if reg < 8 && reg%2 == 0 && oldValue != value && state.sound != nil {
		state.sound.RetriggerChannel(state.baseChannel + int(reg)/2)
	}
}

// SetPOKEYPlusEnabled enables/disables POKEY+ enhanced mode
// When enabled, activates automatic audio enhancements:
// - Oversampling (4x) for cleaner waveforms
// - Low-pass filtering to smooth harsh edges
// - Soft saturation for warm analog-style sound
// - Subtle room ambience
// - Logarithmic volume curve
func (e *POKEYEngine) SetPOKEYPlusEnabled(enabled bool) {
	e.mutex.Lock()
	e.pokeyPlusEnabled = enabled
	right := e.right
	state := e.snapshotSyncStateLocked()
	e.mutex.Unlock()

	if state.sound != nil {
		state.sound.SetPOKEYPlusEnabledForRange(state.baseChannel, 4, enabled)
	}
	if right != nil {
		right.SetPOKEYPlusEnabled(enabled)
	}
	applyPOKEYSyncState(state)
}

// POKEYPlusEnabled returns whether POKEY+ mode is active
func (e *POKEYEngine) POKEYPlusEnabled() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.pokeyPlusEnabled
}

// ensureChannelsInitialized sets up SoundChip channels for POKEY output
func (e *POKEYEngine) ensureChannelsInitialized() {
	if e.channelsInit || e.sound == nil {
		return
	}

	// POKEY uses channels 0-3 of the SoundChip
	// Configure them as square waves with instant envelope
	for ch := range 4 {
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

// syncToChip updates the SoundChip based on current POKEY register state
func (e *POKEYEngine) syncToChip() {
	applyPOKEYSyncState(e.snapshotSyncStateLocked())
}

func (e *POKEYEngine) snapshotSyncStateLocked() pokeySyncState {
	initChannels := false
	if !e.channelsInit && e.sound != nil {
		e.channelsInit = true
		initChannels = true
	}
	return pokeySyncState{
		sound:            e.sound,
		regs:             e.regs,
		pokeyPlusEnabled: e.pokeyPlusEnabled,
		baseChannel:      e.baseChannel,
		initChannels:     initChannels,
		clock179MHz:      e.clock179MHz,
		clock64KHz:       e.clock64KHz,
		clock15KHz:       e.clock15KHz,
	}
}

func applyPOKEYSyncState(s pokeySyncState) {
	if s.sound == nil {
		return
	}
	if s.initChannels {
		for ch := range 4 {
			writePOKEYChannel(s, ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
			writePOKEYChannel(s, ch, FLEX_OFF_DUTY, 0x0080)
			writePOKEYChannel(s, ch, FLEX_OFF_PWM_CTRL, 0)
			writePOKEYChannel(s, ch, FLEX_OFF_ATK, 0)
			writePOKEYChannel(s, ch, FLEX_OFF_DEC, 0)
			writePOKEYChannel(s, ch, FLEX_OFF_SUS, 255)
			writePOKEYChannel(s, ch, FLEX_OFF_REL, 0)
			writePOKEYChannel(s, ch, FLEX_OFF_CTRL, 3)
		}
	}
	applyPOKEYFrequencies(s)
	applyPOKEYVolumes(s)
	applyPOKEYDistortion(s)
	applyPOKEYHighPassFilters(s)
}

func writePOKEYChannel(s pokeySyncState, ch int, offset uint32, value uint32) {
	if addr, ok := flexAddrForChannel(s.baseChannel+ch, offset); ok {
		s.sound.HandleRegisterWrite(addr, value)
	}
}

func calcPOKEYFrequency(s pokeySyncState, channel int) float64 {
	if channel < 0 || channel > 3 {
		return 0
	}

	audf := s.regs[channel*2]
	audctl := s.regs[8]
	var baseClock float64

	switch channel {
	case 0:
		if (audctl & AUDCTL_CH2_BY_CH1) != 0 {
			period := uint16(s.regs[0]) | (uint16(s.regs[2]) << 8)
			if (audctl & AUDCTL_CH1_179MHZ) != 0 {
				baseClock = s.clock179MHz
			} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
				baseClock = s.clock15KHz
			} else {
				baseClock = s.clock64KHz
			}
			return baseClock * 0.5 / float64(period+1)
		}
		if (audctl & AUDCTL_CH1_179MHZ) != 0 {
			baseClock = s.clock179MHz
		} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = s.clock15KHz
		} else {
			baseClock = s.clock64KHz
		}
	case 1:
		if (audctl & AUDCTL_CH2_BY_CH1) != 0 {
			period := uint16(s.regs[0]) | (uint16(audf) << 8)
			if (audctl & AUDCTL_CH1_179MHZ) != 0 {
				baseClock = s.clock179MHz
			} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
				baseClock = s.clock15KHz
			} else {
				baseClock = s.clock64KHz
			}
			return baseClock * 0.5 / float64(period+1)
		}
		if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = s.clock15KHz
		} else {
			baseClock = s.clock64KHz
		}
	case 2:
		if (audctl & AUDCTL_CH4_BY_CH3) != 0 {
			period := uint16(s.regs[4]) | (uint16(s.regs[6]) << 8)
			if (audctl & AUDCTL_CH3_179MHZ) != 0 {
				baseClock = s.clock179MHz
			} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
				baseClock = s.clock15KHz
			} else {
				baseClock = s.clock64KHz
			}
			return baseClock * 0.5 / float64(period+1)
		}
		if (audctl & AUDCTL_CH3_179MHZ) != 0 {
			baseClock = s.clock179MHz
		} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = s.clock15KHz
		} else {
			baseClock = s.clock64KHz
		}
	case 3:
		if (audctl & AUDCTL_CH4_BY_CH3) != 0 {
			period := uint16(s.regs[4]) | (uint16(audf) << 8)
			if (audctl & AUDCTL_CH3_179MHZ) != 0 {
				baseClock = s.clock179MHz
			} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
				baseClock = s.clock15KHz
			} else {
				baseClock = s.clock64KHz
			}
			return baseClock * 0.5 / float64(period+1)
		}
		if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = s.clock15KHz
		} else {
			baseClock = s.clock64KHz
		}
	}

	return baseClock * 0.5 / float64(audf+1)
}

func applyPOKEYFrequencies(s pokeySyncState) {
	audctl := s.regs[8]
	for ch := range 4 {
		if ch == 1 && (audctl&AUDCTL_CH2_BY_CH1) != 0 {
			writePOKEYChannel(s, ch, FLEX_OFF_FREQ, 0)
			writePOKEYChannel(s, ch, FLEX_OFF_VOL, 0)
			continue
		}
		if ch == 3 && (audctl&AUDCTL_CH4_BY_CH3) != 0 {
			writePOKEYChannel(s, ch, FLEX_OFF_FREQ, 0)
			writePOKEYChannel(s, ch, FLEX_OFF_VOL, 0)
			continue
		}
		freq := calcPOKEYFrequency(s, ch)
		if freq > 0 && freq <= 20000 {
			writePOKEYChannel(s, ch, FLEX_OFF_FREQ, uint32(freq*256))
		} else {
			writePOKEYChannel(s, ch, FLEX_OFF_FREQ, 0)
		}
	}
}

func applyPOKEYVolumes(s pokeySyncState) {
	audctl := s.regs[8]
	for ch := range 4 {
		if ch == 1 && (audctl&AUDCTL_CH2_BY_CH1) != 0 {
			writePOKEYChannel(s, ch, FLEX_OFF_VOL, 0)
			continue
		}
		if ch == 3 && (audctl&AUDCTL_CH4_BY_CH3) != 0 {
			writePOKEYChannel(s, ch, FLEX_OFF_VOL, 0)
			continue
		}
		audc := s.regs[ch*2+1]
		level := audc & AUDC_VOLUME_MASK
		gain := pokeyVolumeGain(level, s.pokeyPlusEnabled)
		writePOKEYChannel(s, ch, FLEX_OFF_VOL, uint32(pokeyGainToDAC(gain)))
	}
}

func applyPOKEYDistortion(s pokeySyncState) {
	audctl := s.regs[8]
	for ch := range 4 {
		audc := s.regs[ch*2+1]
		distortion := (audc & AUDC_DISTORTION_MASK) >> AUDC_DISTORTION_SHIFT
		volumeOnly := (audc & AUDC_VOLUME_ONLY) != 0

		if volumeOnly {
			writePOKEYChannel(s, ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
			writePOKEYChannel(s, ch, FLEX_OFF_FREQ, 0)
			continue
		}

		switch distortion {
		case POKEY_DIST_PURE_TONE:
			writePOKEYChannel(s, ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
			writePOKEYChannel(s, ch, FLEX_OFF_NOISEMODE, NOISE_MODE_WHITE)
		case POKEY_DIST_POLY17_PULSE:
			writePOKEYChannel(s, ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
			if (audctl & AUDCTL_POLY9) != 0 {
				writePOKEYChannel(s, ch, FLEX_OFF_NOISEMODE, NOISE_MODE_PERIODIC)
			} else {
				writePOKEYChannel(s, ch, FLEX_OFF_NOISEMODE, NOISE_MODE_WHITE)
			}
		case POKEY_DIST_POLY17, POKEY_DIST_POLY17_POLY5, POKEY_DIST_POLY17_POLY4:
			writePOKEYChannel(s, ch, FLEX_OFF_WAVE_TYPE, WAVE_NOISE)
			if (audctl & AUDCTL_POLY9) != 0 {
				writePOKEYChannel(s, ch, FLEX_OFF_NOISEMODE, NOISE_MODE_PERIODIC)
			} else {
				writePOKEYChannel(s, ch, FLEX_OFF_NOISEMODE, NOISE_MODE_WHITE)
			}
		case POKEY_DIST_POLY5, POKEY_DIST_POLY5_POLY4:
			writePOKEYChannel(s, ch, FLEX_OFF_WAVE_TYPE, WAVE_NOISE)
			writePOKEYChannel(s, ch, FLEX_OFF_NOISEMODE, NOISE_MODE_PERIODIC)
		case POKEY_DIST_POLY4:
			writePOKEYChannel(s, ch, FLEX_OFF_WAVE_TYPE, WAVE_NOISE)
			writePOKEYChannel(s, ch, FLEX_OFF_NOISEMODE, NOISE_MODE_METALLIC)
		default:
			writePOKEYChannel(s, ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
		}
	}
}

func applyPOKEYHighPassFilters(s pokeySyncState) {
	audctl := s.regs[8]
	applyPOKEYHighPassFilter(s, 0, 2, (audctl&AUDCTL_HIPASS_CH1) != 0)
	applyPOKEYHighPassFilter(s, 1, 3, (audctl&AUDCTL_HIPASS_CH2) != 0)
}

func applyPOKEYHighPassFilter(s pokeySyncState, targetCh, cutoffCh int, enabled bool) {
	soundCh := s.baseChannel + targetCh
	if !enabled {
		s.sound.SetChannelFilter(soundCh, 0, 0, 0)
		return
	}
	cutoff := float32(calcPOKEYFrequency(s, cutoffCh) / MAX_FILTER_FREQ)
	if cutoff < 0 {
		cutoff = 0
	} else if cutoff > 1 {
		cutoff = 1
	}
	s.sound.SetChannelFilter(soundCh, 0x04, cutoff, 0)
}

// calcFrequency calculates the output frequency for a channel
// Uses pre-computed clock divisors for optimal performance.
func (e *POKEYEngine) calcFrequency(channel int) float64 {
	if channel < 0 || channel > 3 {
		return 0
	}

	audf := e.regs[channel*2] // AUDFn register
	audctl := e.regs[8]       // AUDCTL register

	var baseClock float64

	// Determine base clock for this channel using pre-computed divisors
	switch channel {
	case 0: // Channel 1
		if (audctl & AUDCTL_CH2_BY_CH1) != 0 {
			period := uint16(e.regs[0]) | (uint16(e.regs[2]) << 8)
			if (audctl & AUDCTL_CH1_179MHZ) != 0 {
				baseClock = e.clock179MHz
			} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
				baseClock = e.clock15KHz
			} else {
				baseClock = e.clock64KHz
			}
			return baseClock * 0.5 / float64(period+1)
		}
		if (audctl & AUDCTL_CH1_179MHZ) != 0 {
			baseClock = e.clock179MHz
		} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = e.clock15KHz
		} else {
			baseClock = e.clock64KHz
		}
	case 1: // Channel 2
		if (audctl & AUDCTL_CH2_BY_CH1) != 0 {
			// 16-bit mode: ch2 clocked by ch1
			audf1 := e.regs[0]
			period := uint16(audf1) | (uint16(audf) << 8)
			if (audctl & AUDCTL_CH1_179MHZ) != 0 {
				baseClock = e.clock179MHz
			} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
				baseClock = e.clock15KHz
			} else {
				baseClock = e.clock64KHz
			}
			return baseClock * 0.5 / float64(period+1)
		}
		if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = e.clock15KHz
		} else {
			baseClock = e.clock64KHz
		}
	case 2: // Channel 3
		if (audctl & AUDCTL_CH4_BY_CH3) != 0 {
			period := uint16(e.regs[4]) | (uint16(e.regs[6]) << 8)
			if (audctl & AUDCTL_CH3_179MHZ) != 0 {
				baseClock = e.clock179MHz
			} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
				baseClock = e.clock15KHz
			} else {
				baseClock = e.clock64KHz
			}
			return baseClock * 0.5 / float64(period+1)
		}
		if (audctl & AUDCTL_CH3_179MHZ) != 0 {
			baseClock = e.clock179MHz
		} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = e.clock15KHz
		} else {
			baseClock = e.clock64KHz
		}
	case 3: // Channel 4
		if (audctl & AUDCTL_CH4_BY_CH3) != 0 {
			// 16-bit mode: ch4 clocked by ch3
			audf3 := e.regs[4]
			period := uint16(audf3) | (uint16(audf) << 8)
			if (audctl & AUDCTL_CH3_179MHZ) != 0 {
				baseClock = e.clock179MHz
			} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
				baseClock = e.clock15KHz
			} else {
				baseClock = e.clock64KHz
			}
			return baseClock * 0.5 / float64(period+1)
		}
		if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = e.clock15KHz
		} else {
			baseClock = e.clock64KHz
		}
	}

	// Standard frequency calculation: baseClock / (2 * (AUDF + 1))
	// Note: baseClock * 0.5 is faster than baseClock / 2.0
	return baseClock * 0.5 / float64(audf+1)
}

// applyFrequencies updates SoundChip frequencies from POKEY registers
func (e *POKEYEngine) applyFrequencies() {
	if e.sound == nil {
		return
	}

	audctl := e.regs[8]

	for ch := range 4 {
		// Skip slave channels in 16-bit mode (they're driven by master)
		if ch == 1 && (audctl&AUDCTL_CH2_BY_CH1) != 0 {
			e.writeChannel(ch, FLEX_OFF_FREQ, 0)
			e.writeChannel(ch, FLEX_OFF_VOL, 0) // Silence slave
			continue
		}
		if ch == 3 && (audctl&AUDCTL_CH4_BY_CH3) != 0 {
			e.writeChannel(ch, FLEX_OFF_FREQ, 0)
			e.writeChannel(ch, FLEX_OFF_VOL, 0) // Silence slave
			continue
		}

		freq := e.calcFrequency(ch)
		if freq > 0 && freq <= 20000 {
			e.writeChannel(ch, FLEX_OFF_FREQ, uint32(freq*256)) // 16.8 fixed-point
		} else {
			e.writeChannel(ch, FLEX_OFF_FREQ, 0)
		}
	}
}

// applyVolumes updates SoundChip volumes from POKEY registers
func (e *POKEYEngine) applyVolumes() {
	if e.sound == nil {
		return
	}

	audctl := e.regs[8]
	for ch := range 4 {
		if ch == 1 && (audctl&AUDCTL_CH2_BY_CH1) != 0 {
			e.writeChannel(ch, FLEX_OFF_VOL, 0)
			continue
		}
		if ch == 3 && (audctl&AUDCTL_CH4_BY_CH3) != 0 {
			e.writeChannel(ch, FLEX_OFF_VOL, 0)
			continue
		}
		audc := e.regs[ch*2+1] // AUDCn register
		level := audc & AUDC_VOLUME_MASK

		gain := pokeyVolumeGain(level, e.pokeyPlusEnabled)
		e.writeChannel(ch, FLEX_OFF_VOL, uint32(pokeyGainToDAC(gain)))
	}
}

// applyDistortion configures channel wave types based on POKEY distortion settings
func (e *POKEYEngine) applyDistortion() {
	if e.sound == nil {
		return
	}

	audctl := e.regs[8]
	for ch := range 4 {
		audc := e.regs[ch*2+1]
		distortion := (audc & AUDC_DISTORTION_MASK) >> AUDC_DISTORTION_SHIFT
		volumeOnly := (audc & AUDC_VOLUME_ONLY) != 0

		if volumeOnly {
			// Volume-only mode: DC output at volume level
			e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
			e.writeChannel(ch, FLEX_OFF_FREQ, 0)
			continue
		}

		// Map POKEY distortion modes to SoundChip wave types
		switch distortion {
		case POKEY_DIST_PURE_TONE:
			// Pure square wave
			e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
			e.writeChannel(ch, FLEX_OFF_NOISEMODE, NOISE_MODE_WHITE)
		case POKEY_DIST_POLY17_PULSE:
			e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
			if (audctl & AUDCTL_POLY9) != 0 {
				e.writeChannel(ch, FLEX_OFF_NOISEMODE, NOISE_MODE_PERIODIC)
			} else {
				e.writeChannel(ch, FLEX_OFF_NOISEMODE, NOISE_MODE_WHITE)
			}
		case POKEY_DIST_POLY17, POKEY_DIST_POLY17_POLY5, POKEY_DIST_POLY17_POLY4:
			e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_NOISE)
			if (audctl & AUDCTL_POLY9) != 0 {
				e.writeChannel(ch, FLEX_OFF_NOISEMODE, NOISE_MODE_PERIODIC)
			} else {
				e.writeChannel(ch, FLEX_OFF_NOISEMODE, NOISE_MODE_WHITE)
			}
		case POKEY_DIST_POLY5, POKEY_DIST_POLY5_POLY4:
			// Periodic/buzzy noise
			e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_NOISE)
			e.writeChannel(ch, FLEX_OFF_NOISEMODE, NOISE_MODE_PERIODIC)
		case POKEY_DIST_POLY4:
			// Metallic/buzzy noise
			e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_NOISE)
			e.writeChannel(ch, FLEX_OFF_NOISEMODE, NOISE_MODE_METALLIC)
		default:
			e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
		}
	}
}

func (e *POKEYEngine) applyHighPassFilters() {
	if e.sound == nil {
		return
	}
	audctl := e.regs[8]
	e.applyHighPassFilter(0, 2, (audctl&AUDCTL_HIPASS_CH1) != 0)
	e.applyHighPassFilter(1, 3, (audctl&AUDCTL_HIPASS_CH2) != 0)
}

func (e *POKEYEngine) applyHighPassFilter(targetCh, cutoffCh int, enabled bool) {
	soundCh := e.baseChannel + targetCh
	if !enabled {
		e.sound.SetChannelFilter(soundCh, 0, 0, 0)
		return
	}
	cutoff := float32(e.calcFrequency(cutoffCh) / MAX_FILTER_FREQ)
	if cutoff < 0 {
		cutoff = 0
	} else if cutoff > 1 {
		cutoff = 1
	}
	e.sound.SetChannelFilter(soundCh, 0x04, cutoff, 0)
}

// writeChannel writes a value to a SoundChip channel register
func (e *POKEYEngine) writeChannel(ch int, offset uint32, value uint32) {
	if e.sound == nil {
		return
	}
	if addr, ok := flexAddrForChannel(e.baseChannel+ch, offset); ok {
		e.sound.HandleRegisterWrite(addr, value)
	}
}

// pokeyVolumeGain converts a 4-bit POKEY volume level to a gain value
// Uses pre-computed lookup tables for optimal performance.
func pokeyVolumeGain(level uint8, pokeyPlus bool) float32 {
	if level > 15 {
		level = 15
	}
	if pokeyPlus {
		return pokeyPlusVolumeCurve[level]
	}
	// Use pre-computed linear volume lookup table (no division)
	return pokeyLinearVolumeCurve[level]
}

// pokeyGainToDAC converts a gain value to an 8-bit DAC value
func pokeyGainToDAC(gain float32) uint8 {
	if gain <= 0 {
		return 0
	}
	if gain >= 1.0 {
		return 255
	}
	return uint8(math.Round(float64(gain * 255.0)))
}

// Reset resets all POKEY state
func (e *POKEYEngine) Reset() {
	e.mutex.Lock()

	for i := range e.regs {
		e.regs[i] = 0
	}

	e.channelsInit = false
	e.pokeyPlusEnabled = false
	e.randomSR = 0x1FFFF
	sound := e.sound
	baseChannel := e.baseChannel

	// Reset playback state
	e.events = nil
	e.eventIndex = 0
	e.currentSample = 0
	e.totalSamples = 0
	e.loop = false
	e.loopSample = 0
	e.forceLoop = false
	e.playing.Store(false)
	e.mutex.Unlock()

	if sound != nil {
		sound.SetPOKEYPlusEnabledForRange(baseChannel, 4, false)
		for ch := range 4 {
			if addr, ok := flexAddrForChannel(baseChannel+ch, FLEX_OFF_VOL); ok {
				sound.HandleRegisterWrite(addr, 0)
			}
			if addr, ok := flexAddrForChannel(baseChannel+ch, FLEX_OFF_FREQ); ok {
				sound.HandleRegisterWrite(addr, 0)
			}
			if addr, ok := flexAddrForChannel(baseChannel+ch, FLEX_OFF_CTRL); ok {
				sound.HandleRegisterWrite(addr, 0)
			}
		}
	}
}

// SetEvents sets the POKEY events for playback (from SAP rendering)
func (e *POKEYEngine) SetEvents(events []SAPPOKEYEvent, totalSamples uint64, loop bool, loopSample uint64) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.events = events
	e.eventIndex = 0
	e.totalSamples = totalSamples
	e.currentSample = 0
	e.loop = loop
	e.loopSample = loopSample
	e.playing.Store(false)
}

// SetPlaying starts or stops event-based playback
func (e *POKEYEngine) SetPlaying(playing bool) {
	e.mutex.Lock()
	e.playing.Store(playing)
	var state pokeySyncState
	if playing {
		state = e.snapshotSyncStateLocked()
	}
	e.mutex.Unlock()
	if playing {
		applyPOKEYSyncState(state)
	}
}

// IsPlaying returns true if event-based playback is active
func (e *POKEYEngine) IsPlaying() bool {
	return e.playing.Load()
}

// SetForceLoop enables looping from the start of the track
func (e *POKEYEngine) SetForceLoop(enable bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if enable {
		e.loop = true
		e.loopSample = 0
	}
	e.forceLoop = enable
}

// StopPlayback stops playback and clears events
func (e *POKEYEngine) StopPlayback() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing.Store(false)
	e.events = nil
	e.eventIndex = 0
	e.currentSample = 0
}

// TickSample processes one sample of event-based playback
// Implements SampleTicker interface for SoundChip integration
func (e *POKEYEngine) TickSample() {
	if !e.playing.Load() {
		return
	}

	e.mutex.Lock()

	if len(e.events) == 0 {
		e.mutex.Unlock()
		return
	}

	drained := make([]SAPPOKEYEvent, 0, 4)
	// Process all events at current sample position
	for e.eventIndex < len(e.events) && e.events[e.eventIndex].Sample <= e.currentSample {
		drained = append(drained, e.events[e.eventIndex])
		e.eventIndex++
	}

	// Advance sample counter
	e.currentSample++

	// Check for end of playback
	if e.totalSamples > 0 && e.currentSample >= e.totalSamples {
		if e.loop {
			e.currentSample = e.loopSample
			// Find event index for loop position
			e.eventIndex = 0
			for e.eventIndex < len(e.events) && e.events[e.eventIndex].Sample < e.loopSample {
				e.eventIndex++
			}
		} else {
			e.playing.Store(false)
		}
	}
	e.mutex.Unlock()

	for _, ev := range drained {
		if ev.Chip == 1 && e.right != nil {
			e.right.WriteRegister(ev.Reg, ev.Value)
			continue
		}
		e.WriteRegister(ev.Reg, ev.Value)
	}
}
