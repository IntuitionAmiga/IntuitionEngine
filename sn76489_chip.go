package main

import "sync"

type SN76489Chip struct {
	mu          sync.Mutex
	sound       *SoundChip
	clockHz     uint32
	mode        uint8
	latchCh     uint8
	latchVolume bool
	tone        [3]uint16
	atten       [4]uint8
	noiseReg    uint8
	lfsr        uint32
	lastWritten uint8
	writeCount  uint64
}

func NewSN76489Chip(sound *SoundChip) *SN76489Chip {
	chip := &SN76489Chip{
		sound:   sound,
		clockHz: SN_CLOCK_NTSC,
		mode:    SN76489_MODE_LFSR_15,
		atten:   [4]uint8{15, 15, 15, 15},
	}
	chip.lfsr = chip.lfsrSeed()
	chip.syncAllVoicesLocked()
	chip.resetAudibleNoiseLFSRLocked()
	if sound != nil {
		sound.RegisterSampleTicker("sn76489", chip)
	}
	return chip
}

func (c *SN76489Chip) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latchCh = 0
	c.latchVolume = false
	c.tone = [3]uint16{}
	c.atten = [4]uint8{15, 15, 15, 15}
	c.noiseReg = 0
	c.mode = SN76489_MODE_LFSR_15
	c.lfsr = c.lfsrSeed()
	c.lastWritten = 0
	c.writeCount = 0
	c.syncAllVoicesLocked()
	c.resetAudibleNoiseLFSRLocked()
}

func (c *SN76489Chip) SetClockHz(clock uint32) {
	if clock == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clockHz = clock
	c.syncAllVoicesLocked()
}

func (c *SN76489Chip) HandleRead(addr uint32) uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch addr {
	case SN_PORT_READY:
		return 1
	case SN_PORT_MODE:
		return uint32(c.mode)
	default:
		return 0
	}
}

func (c *SN76489Chip) HandleWrite(addr uint32, value uint32) {
	c.HandleWrite8(addr, byte(value))
}

func (c *SN76489Chip) HandleWrite8(addr uint32, value uint8) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch addr {
	case SN_PORT_WRITE:
		c.writeDataLocked(value)
	case SN_PORT_MODE:
		c.mode = value & 1
		c.lfsr = c.lfsrSeed()
		c.syncNoiseVoiceLocked()
		c.resetAudibleNoiseLFSRLocked()
	default:
		return
	}
}

func (c *SN76489Chip) TickSample() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clockNoise()
}

func (c *SN76489Chip) writeDataLocked(value uint8) {
	c.lastWritten = value
	c.writeCount++
	if value&0x80 != 0 {
		c.latchCh = (value >> 5) & 3
		c.latchVolume = value&0x10 != 0
		if c.latchVolume {
			c.setAttenLocked(c.latchCh, value&0x0F)
			return
		}
		if c.latchCh == 3 {
			c.setNoiseRegLocked(value & 0x07)
			return
		}
		c.tone[c.latchCh] = (c.tone[c.latchCh] & 0x3F0) | uint16(value&0x0F)
		c.syncToneVoiceLocked(c.latchCh)
		return
	}

	if c.latchVolume {
		c.setAttenLocked(c.latchCh, value&0x0F)
		return
	}
	if c.latchCh == 3 {
		// Data bytes after a noise latch are ignored by the SN76489.
		return
	}
	c.tone[c.latchCh] = (c.tone[c.latchCh] & 0x00F) | (uint16(value&0x3F) << 4)
	c.syncToneVoiceLocked(c.latchCh)
}

func (c *SN76489Chip) setAttenLocked(ch uint8, atten uint8) {
	c.atten[ch] = atten & 0x0F
	if ch < 3 {
		c.syncToneVoiceLocked(ch)
	} else {
		c.syncNoiseVoiceLocked()
	}
}

func (c *SN76489Chip) setNoiseRegLocked(value uint8) {
	c.noiseReg = value & 0x07
	c.lfsr = c.lfsrSeed()
	c.syncNoiseVoiceLocked()
	c.resetAudibleNoiseLFSRLocked()
}

func (c *SN76489Chip) lfsrSeed() uint32 {
	if c.mode == SN76489_MODE_LFSR_16 {
		return SN16_NOISE_LFSR_MASK
	}
	return SN15_NOISE_LFSR_MASK
}

func (c *SN76489Chip) effectiveDivider(ch int) uint16 {
	if ch < 0 || ch >= 3 {
		return 1
	}
	if c.tone[ch] == 0 {
		return 1024
	}
	return c.tone[ch]
}

func (c *SN76489Chip) toneFrequency(ch int) float64 {
	return float64(c.clockHz) / (32.0 * float64(c.effectiveDivider(ch)))
}

func (c *SN76489Chip) noiseFrequency() float64 {
	switch c.noiseReg & 3 {
	case 0:
		return float64(c.clockHz) / 512.0
	case 1:
		return float64(c.clockHz) / 1024.0
	case 2:
		return float64(c.clockHz) / 2048.0
	default:
		return c.toneFrequency(2)
	}
}

func (c *SN76489Chip) clockNoise() {
	if c.noiseReg&0x04 != 0 {
		if c.mode == SN76489_MODE_LFSR_16 {
			c.lfsr = stepNoiseLFSR(NOISE_MODE_SN16_WHITE, c.lfsr)
		} else {
			c.lfsr = stepNoiseLFSR(NOISE_MODE_SN15_WHITE, c.lfsr)
		}
		return
	}
	if c.mode == SN76489_MODE_LFSR_16 {
		c.lfsr = stepNoiseLFSR(NOISE_MODE_SN16_PERIODIC, c.lfsr)
	} else {
		c.lfsr = stepNoiseLFSR(NOISE_MODE_SN15_PERIODIC, c.lfsr)
	}
}

func (c *SN76489Chip) syncAllVoicesLocked() {
	for ch := uint8(0); ch < 3; ch++ {
		c.syncToneVoiceLocked(ch)
	}
	c.syncNoiseVoiceLocked()
}

func (c *SN76489Chip) syncToneVoiceLocked(ch uint8) {
	if c.sound == nil || ch >= 3 {
		return
	}
	c.sound.mu.Lock()
	defer c.sound.mu.Unlock()
	voice := &c.sound.snVoices[ch]
	voice.waveType = WAVE_SQUARE
	voice.frequency = float32(c.toneFrequency(int(ch)))
	voice.volume = snAttenuationGain(c.atten[ch])
	voice.enabled = c.atten[ch] < 15
	voice.gate = voice.enabled
	voice.attackTime = 0
	voice.decayTime = 0
	voice.sustainLevel = MAX_LEVEL
	voice.releaseTime = 0
	voice.attackRecip = 0
	voice.decayRecip = 0
	voice.releaseRecip = 0
	voice.envelopePhase = ENV_SUSTAIN
	voice.envelopeLevel = MAX_LEVEL
}

func (c *SN76489Chip) syncNoiseVoiceLocked() {
	if c.sound == nil {
		return
	}
	c.sound.mu.Lock()
	defer c.sound.mu.Unlock()
	voice := &c.sound.snVoices[3]
	voice.waveType = WAVE_NOISE
	voice.frequency = float32(c.noiseFrequency())
	voice.noiseFrequency = float32(c.noiseFrequency())
	if c.noiseReg&0x04 != 0 {
		if c.mode == SN76489_MODE_LFSR_16 {
			voice.noiseMode = NOISE_MODE_SN16_WHITE
		} else {
			voice.noiseMode = NOISE_MODE_SN15_WHITE
		}
	} else if c.mode == SN76489_MODE_LFSR_16 {
		voice.noiseMode = NOISE_MODE_SN16_PERIODIC
	} else {
		voice.noiseMode = NOISE_MODE_SN15_PERIODIC
	}
	voice.volume = snAttenuationGain(c.atten[3])
	voice.enabled = c.atten[3] < 15
	voice.gate = voice.enabled
	voice.attackTime = 0
	voice.decayTime = 0
	voice.sustainLevel = MAX_LEVEL
	voice.releaseTime = 0
	voice.attackRecip = 0
	voice.decayRecip = 0
	voice.releaseRecip = 0
	voice.envelopePhase = ENV_SUSTAIN
	voice.envelopeLevel = MAX_LEVEL
}

func (c *SN76489Chip) resetAudibleNoiseLFSRLocked() {
	if c.sound == nil {
		return
	}
	c.sound.mu.Lock()
	defer c.sound.mu.Unlock()
	c.sound.snVoices[3].noiseSR = c.lfsr
}

func snAttenuationGain(atten uint8) float32 {
	if atten >= 15 {
		return 0
	}
	return psgLogVolumeCurve[15-(atten&0x0F)]
}
