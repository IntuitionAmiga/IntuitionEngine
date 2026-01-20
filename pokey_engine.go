// pokey_engine.go - POKEY sound chip emulation for the Intuition Engine

/*
POKEY (Pot Keyboard Integrated Circuit) was the audio chip used in Atari 8-bit
computers and the Atari 5200. This implementation provides register-level
emulation accessible from all CPUs (M68K, IE32, Z80, 6502).

Features:
- 4 audio channels with independent frequency and volume control
- 8 distortion modes using polynomial counters (17/9/5/4-bit)
- 16-bit channel linking for extended frequency range
- High-pass filter emulation between channel pairs
- POKEY+ enhanced mode with logarithmic volume curve

The engine translates POKEY register writes to SoundChip channel parameters,
using the noise channel modes to approximate POKEY's polynomial distortion.
*/

package main

import (
	"math"
	"sync"
)

// POKEYEngine emulates the POKEY sound chip
type POKEYEngine struct {
	mutex      sync.Mutex
	sound      *SoundChip
	sampleRate int
	clockHz    uint32

	regs [POKEY_REG_COUNT]uint8

	// Polynomial counter state
	poly17 uint32
	poly9  uint16
	poly5  [4]uint8 // Per-channel 5-bit poly
	poly4  [4]uint8 // Per-channel 4-bit counter

	// Channel state
	chanPhase   [4]float64 // Phase accumulators
	chanOutput  [4]float32 // Current output values
	hipassState [2]float32 // High-pass filter state (ch1, ch2)

	enabled          bool
	pokeyPlusEnabled bool
	channelsInit     bool
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

// NewPOKEYEngine creates a new POKEY emulation engine
func NewPOKEYEngine(sound *SoundChip, sampleRate int) *POKEYEngine {
	engine := &POKEYEngine{
		sound:      sound,
		sampleRate: sampleRate,
		clockHz:    POKEY_CLOCK_NTSC,
		poly17:     0x1FFFF, // 17-bit LFSR seed (all 1s)
		poly9:      0x1FF,   // 9-bit LFSR seed
	}

	// Initialize per-channel poly counters
	for i := 0; i < 4; i++ {
		engine.poly5[i] = 0x1F // 5-bit seed
		engine.poly4[i] = 0x0F // 4-bit seed
	}

	return engine
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
	}

	e.syncToChip()
}

// SetPOKEYPlusEnabled enables/disables POKEY+ enhanced mode
func (e *POKEYEngine) SetPOKEYPlusEnabled(enabled bool) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.pokeyPlusEnabled = enabled
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
			e.writeChannel(ch, FLEX_OFF_FREQ, uint32(freq))
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
			e.writeChannel(ch, FLEX_OFF_FREQ, 1) // Minimal frequency
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

// Polynomial counter operations for accurate POKEY emulation

// tick17 advances the 17-bit polynomial counter
func (e *POKEYEngine) tick17() uint8 {
	// 17-bit LFSR with taps at bits 0 and 5
	bit := (e.poly17 ^ (e.poly17 >> 5)) & 1
	e.poly17 = (e.poly17 >> 1) | (bit << 16)
	return uint8(e.poly17 & 1)
}

// tick9 advances the 9-bit polynomial counter
func (e *POKEYEngine) tick9() uint8 {
	// 9-bit LFSR with taps at bits 0 and 5
	bit := (e.poly9 ^ (e.poly9 >> 5)) & 1
	e.poly9 = (e.poly9 >> 1) | (uint16(bit) << 8)
	return uint8(e.poly9 & 1)
}

// tick5 advances a 5-bit polynomial counter for a channel
func (e *POKEYEngine) tick5(ch int) uint8 {
	if ch < 0 || ch > 3 {
		return 0
	}
	// 5-bit LFSR with taps at bits 0 and 2
	bit := (e.poly5[ch] ^ (e.poly5[ch] >> 2)) & 1
	e.poly5[ch] = (e.poly5[ch] >> 1) | (bit << 4)
	return e.poly5[ch] & 1
}

// tick4 advances a 4-bit counter for a channel
func (e *POKEYEngine) tick4(ch int) uint8 {
	if ch < 0 || ch > 3 {
		return 0
	}
	e.poly4[ch] = (e.poly4[ch] + 1) & 0x0F
	return e.poly4[ch] & 1
}

// Reset resets all POKEY state
func (e *POKEYEngine) Reset() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for i := range e.regs {
		e.regs[i] = 0
	}

	e.poly17 = 0x1FFFF
	e.poly9 = 0x1FF
	for i := 0; i < 4; i++ {
		e.poly5[i] = 0x1F
		e.poly4[i] = 0x0F
		e.chanPhase[i] = 0
		e.chanOutput[i] = 0
	}
	e.hipassState[0] = 0
	e.hipassState[1] = 0

	e.enabled = false
	e.channelsInit = false
}
