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
)

// POKEYEngine emulates the POKEY sound chip via register mapping to SoundChip
type POKEYEngine struct {
	mutex      sync.Mutex
	sound      *SoundChip
	sampleRate int
	clockHz    uint32

	regs [POKEY_REG_COUNT]uint8

	enabled          bool
	pokeyPlusEnabled bool
	channelsInit     bool

	// Event-based playback (for SAP files)
	events        []SAPPOKEYEvent
	eventIndex    int
	currentSample uint64
	totalSamples  uint64
	playing       bool
	loop          bool
	loopSample    uint64
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
	return &POKEYEngine{
		sound:      sound,
		sampleRate: sampleRate,
		clockHz:    POKEY_CLOCK_NTSC,
	}
}

// SetClockHz sets the POKEY master clock frequency
func (e *POKEYEngine) SetClockHz(clock uint32) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if clock == 0 {
		return
	}
	e.clockHz = clock
}

// HandleWrite processes a write to a POKEY register via memory-mapped I/O
func (e *POKEYEngine) HandleWrite(addr uint32, value uint32) {
	if addr < POKEY_BASE || addr > POKEY_END {
		return
	}
	reg := uint8(addr - POKEY_BASE)
	e.WriteRegister(reg, uint8(value))
}

// HandleRead processes a read from a POKEY register
func (e *POKEYEngine) HandleRead(addr uint32) uint32 {
	if addr < POKEY_BASE || addr > POKEY_END {
		return 0
	}
	reg := uint8(addr - POKEY_BASE)
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return uint32(e.regs[reg])
}

// WriteRegister writes a value to a POKEY register
func (e *POKEYEngine) WriteRegister(reg uint8, value uint8) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if reg >= POKEY_REG_COUNT {
		return
	}

	e.enabled = true
	e.regs[reg] = value

	// Handle POKEY+ control register
	if reg == 9 { // POKEY_PLUS_CTRL offset
		e.pokeyPlusEnabled = (value & 1) != 0
		if e.sound != nil {
			e.sound.SetPOKEYPlusEnabled(e.pokeyPlusEnabled)
		}
	}

	e.syncToChip()
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
	defer e.mutex.Unlock()
	e.pokeyPlusEnabled = enabled
	if e.sound != nil {
		e.sound.SetPOKEYPlusEnabled(enabled)
	}
	e.syncToChip()
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
	for ch := 0; ch < 4; ch++ {
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
	if e.sound == nil {
		return
	}

	e.ensureChannelsInitialized()
	e.applyFrequencies()
	e.applyVolumes()
	e.applyDistortion()
}

// calcFrequency calculates the output frequency for a channel
func (e *POKEYEngine) calcFrequency(channel int) float64 {
	if channel < 0 || channel > 3 {
		return 0
	}

	audf := e.regs[channel*2] // AUDFn register
	audctl := e.regs[8]       // AUDCTL register

	var baseClock float64

	// Determine base clock for this channel
	switch channel {
	case 0: // Channel 1
		if (audctl & AUDCTL_CH1_179MHZ) != 0 {
			baseClock = float64(e.clockHz)
		} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = float64(e.clockHz) / float64(POKEY_DIV_15KHZ)
		} else {
			baseClock = float64(e.clockHz) / float64(POKEY_DIV_64KHZ)
		}
	case 1: // Channel 2
		if (audctl & AUDCTL_CH2_BY_CH1) != 0 {
			// 16-bit mode: ch2 clocked by ch1
			audf1 := e.regs[0]
			period := uint16(audf1) | (uint16(audf) << 8)
			if (audctl & AUDCTL_CH1_179MHZ) != 0 {
				baseClock = float64(e.clockHz)
			} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
				baseClock = float64(e.clockHz) / float64(POKEY_DIV_15KHZ)
			} else {
				baseClock = float64(e.clockHz) / float64(POKEY_DIV_64KHZ)
			}
			if period == 0 {
				return 0
			}
			return baseClock / (2.0 * float64(period+1))
		}
		if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = float64(e.clockHz) / float64(POKEY_DIV_15KHZ)
		} else {
			baseClock = float64(e.clockHz) / float64(POKEY_DIV_64KHZ)
		}
	case 2: // Channel 3
		if (audctl & AUDCTL_CH3_179MHZ) != 0 {
			baseClock = float64(e.clockHz)
		} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = float64(e.clockHz) / float64(POKEY_DIV_15KHZ)
		} else {
			baseClock = float64(e.clockHz) / float64(POKEY_DIV_64KHZ)
		}
	case 3: // Channel 4
		if (audctl & AUDCTL_CH4_BY_CH3) != 0 {
			// 16-bit mode: ch4 clocked by ch3
			audf3 := e.regs[4]
			period := uint16(audf3) | (uint16(audf) << 8)
			if (audctl & AUDCTL_CH3_179MHZ) != 0 {
				baseClock = float64(e.clockHz)
			} else if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
				baseClock = float64(e.clockHz) / float64(POKEY_DIV_15KHZ)
			} else {
				baseClock = float64(e.clockHz) / float64(POKEY_DIV_64KHZ)
			}
			if period == 0 {
				return 0
			}
			return baseClock / (2.0 * float64(period+1))
		}
		if (audctl & AUDCTL_CLOCK_15KHZ) != 0 {
			baseClock = float64(e.clockHz) / float64(POKEY_DIV_15KHZ)
		} else {
			baseClock = float64(e.clockHz) / float64(POKEY_DIV_64KHZ)
		}
	}

	if audf == 0 {
		return 0
	}

	// Standard frequency calculation: baseClock / (2 * (AUDF + 1))
	return baseClock / (2.0 * float64(audf+1))
}

// applyFrequencies updates SoundChip frequencies from POKEY registers
func (e *POKEYEngine) applyFrequencies() {
	if e.sound == nil {
		return
	}

	audctl := e.regs[8]

	for ch := 0; ch < 4; ch++ {
		// Skip slave channels in 16-bit mode (they're driven by master)
		if ch == 1 && (audctl&AUDCTL_CH2_BY_CH1) != 0 {
			e.writeChannel(ch, FLEX_OFF_VOL, 0) // Silence slave
			continue
		}
		if ch == 3 && (audctl&AUDCTL_CH4_BY_CH3) != 0 {
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

	for ch := 0; ch < 4; ch++ {
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

	for ch := 0; ch < 4; ch++ {
		audc := e.regs[ch*2+1]
		distortion := (audc & AUDC_DISTORTION_MASK) >> AUDC_DISTORTION_SHIFT
		volumeOnly := (audc & AUDC_VOLUME_ONLY) != 0

		if volumeOnly {
			// Volume-only mode: DC output at volume level
			// Use a very low frequency square wave to approximate DC
			e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
			e.writeChannel(ch, FLEX_OFF_FREQ, 256) // 1 Hz in 16.8 fixed-point
			continue
		}

		// Map POKEY distortion modes to SoundChip wave types
		switch distortion {
		case POKEY_DIST_PURE_TONE:
			// Pure square wave
			e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_SQUARE)
			e.writeChannel(ch, FLEX_OFF_NOISEMODE, NOISE_MODE_WHITE)
		case POKEY_DIST_POLY17, POKEY_DIST_POLY17_POLY5, POKEY_DIST_POLY17_POLY4, POKEY_DIST_POLY17_PULSE:
			// White noise variants
			e.writeChannel(ch, FLEX_OFF_WAVE_TYPE, WAVE_NOISE)
			e.writeChannel(ch, FLEX_OFF_NOISEMODE, NOISE_MODE_WHITE)
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

// writeChannel writes a value to a SoundChip channel register
func (e *POKEYEngine) writeChannel(ch int, offset uint32, value uint32) {
	if e.sound == nil {
		return
	}
	base := FLEX_CH_BASE + uint32(ch)*FLEX_CH_STRIDE
	e.sound.HandleRegisterWrite(base+offset, value)
}

// pokeyVolumeGain converts a 4-bit POKEY volume level to a gain value
func pokeyVolumeGain(level uint8, pokeyPlus bool) float32 {
	if level > 15 {
		level = 15
	}
	if pokeyPlus {
		return pokeyPlusVolumeCurve[level]
	}
	// Linear volume curve for standard POKEY
	return float32(level) / 15.0
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
	defer e.mutex.Unlock()

	for i := range e.regs {
		e.regs[i] = 0
	}

	e.enabled = false
	e.channelsInit = false

	// Reset playback state
	e.eventIndex = 0
	e.currentSample = 0
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
	e.playing = false
}

// SetPlaying starts or stops event-based playback
func (e *POKEYEngine) SetPlaying(playing bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = playing
	if playing {
		e.ensureChannelsInitialized()
	}
}

// IsPlaying returns true if event-based playback is active
func (e *POKEYEngine) IsPlaying() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return e.playing
}

// StopPlayback stops playback and clears events
func (e *POKEYEngine) StopPlayback() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.playing = false
	e.events = nil
	e.eventIndex = 0
	e.currentSample = 0
}

// TickSample processes one sample of event-based playback
// Implements SampleTicker interface for SoundChip integration
func (e *POKEYEngine) TickSample() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if !e.playing || len(e.events) == 0 {
		return
	}

	// Process all events at current sample position
	for e.eventIndex < len(e.events) && e.events[e.eventIndex].Sample <= e.currentSample {
		ev := e.events[e.eventIndex]
		// Apply POKEY register write (without locking - we already hold the lock)
		e.writeRegisterLocked(ev.Reg, ev.Value)
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
			e.playing = false
		}
	}
}

// writeRegisterLocked writes a register without acquiring the lock (caller must hold lock)
func (e *POKEYEngine) writeRegisterLocked(reg uint8, value uint8) {
	if int(reg) >= len(e.regs) {
		return
	}
	e.regs[reg] = value
	e.syncToChip() // Apply changes to SoundChip
}
