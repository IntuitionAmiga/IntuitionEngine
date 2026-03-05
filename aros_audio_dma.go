package main

import (
	"sync/atomic"
	"unsafe"
)

// ArosAudioDMA emulates Paula-style DMA audio for the AROS audio.device.
// It implements the SampleTicker interface and is called at 44100 Hz from
// the SoundChip's ReadSample path. For each active DMA channel it reads
// sample bytes from guest RAM, writes them to the corresponding flex
// channel DAC, and triggers an M68K level-3 interrupt when a buffer is
// exhausted.
type ArosAudioDMA struct {
	bus       *MachineBus
	soundChip *SoundChip
	cpu       *M68KCPU
	memBase   unsafe.Pointer // cached bus memory base for fast byte reads

	channels [4]arosAudDMACh

	dmacon uint32 // DMA enable bitmask (bits 0–3)
	status uint32 // completion status  (bits 0–3)
	intena uint32 // interrupt enable   (bits 0–3)

	enabled atomic.Bool // true when any channel has DMA enabled
}

type arosAudDMACh struct {
	ptr    uint32  // sample pointer in guest RAM
	len    uint32  // sample length in words (×2 = bytes)
	per    uint32  // Paula period
	vol    uint32  // Paula volume (0–64)
	pos    uint32  // current byte offset within buffer
	phase  float64 // fractional sample accumulator
	active bool    // DMA running
}

// NewArosAudioDMA creates a DMA engine wired to the given bus, SoundChip,
// and M68K CPU. Call MapIO afterwards to register the MMIO handlers.
func NewArosAudioDMA(bus *MachineBus, sc *SoundChip, cpu *M68KCPU) *ArosAudioDMA {
	return &ArosAudioDMA{
		bus:       bus,
		soundChip: sc,
		cpu:       cpu,
		memBase:   unsafe.Pointer(&bus.memory[0]),
	}
}

// TickSample is called at 44100 Hz from SoundChip.ReadSample().
// It advances each active DMA channel by one output sample, reads the
// next byte from guest RAM, and pushes it into the flex channel DAC.
func (dma *ArosAudioDMA) TickSample() {
	if !dma.enabled.Load() {
		return
	}

	for ch := range 4 {
		if dma.dmacon&(1<<ch) == 0 {
			continue
		}
		c := &dma.channels[ch]
		if !c.active || c.per == 0 || c.len == 0 {
			continue
		}

		// Phase increment: paulaFreq / sampleRate
		paulaFreq := paulaClockPAL / float64(c.per)
		phaseInc := paulaFreq / float64(SAMPLE_RATE)

		c.phase += phaseInc

		// Advance byte position by integer part of accumulated phase
		for c.phase >= 1.0 {
			c.pos++
			c.phase -= 1.0
		}

		bufBytes := c.len * 2 // len is in words
		if c.pos >= bufBytes {
			// Buffer exhausted — signal completion
			c.pos = 0
			c.active = false
			dma.status |= 1 << ch
			if dma.intena&(1<<ch) != 0 {
				dma.cpu.AssertInterrupt(arosAudioIRQLevel)
			}
			continue
		}

		// Read sample byte directly from bus memory (big-endian byte order).
		addr := c.ptr + c.pos
		if addr < DEFAULT_MEMORY_SIZE {
			sample := *(*byte)(unsafe.Pointer(uintptr(dma.memBase) + uintptr(addr)))

			// Write to flex channel DAC and volume.
			flexCh := dma.soundChip.channels[ch]
			flexCh.dacMode = true
			flexCh.dacValue = float32(int8(sample)) / 128.0

			// Paula volume 0–64 → IE volume 0–255.
			ieVol := c.vol * 4
			if ieVol > 255 {
				ieVol = 255
			}
			flexCh.volume = float32(ieVol) / 255.0
		}
	}
}

// HandleRead returns the value of the MMIO register at addr.
func (dma *ArosAudioDMA) HandleRead(addr uint32) uint32 {
	if addr >= AROS_AUD_REGION_BASE && addr < AROS_AUD_DMACON {
		chIdx := (addr - AROS_AUD_REGION_BASE) / AROS_AUD_CH_STRIDE
		off := (addr - AROS_AUD_REGION_BASE) % AROS_AUD_CH_STRIDE
		if chIdx >= 4 {
			return 0
		}
		c := &dma.channels[chIdx]
		switch off {
		case AROS_AUD_OFF_PTR:
			return c.ptr
		case AROS_AUD_OFF_LEN:
			return c.len
		case AROS_AUD_OFF_PER:
			return c.per
		case AROS_AUD_OFF_VOL:
			return c.vol
		}
		return 0
	}

	switch addr {
	case AROS_AUD_DMACON:
		return dma.dmacon
	case AROS_AUD_STATUS:
		return dma.status
	case AROS_AUD_INTENA:
		return dma.intena
	}
	return 0
}

// HandleWrite processes an MMIO write to addr.
func (dma *ArosAudioDMA) HandleWrite(addr, value uint32) {
	if addr >= AROS_AUD_REGION_BASE && addr < AROS_AUD_DMACON {
		chIdx := (addr - AROS_AUD_REGION_BASE) / AROS_AUD_CH_STRIDE
		off := (addr - AROS_AUD_REGION_BASE) % AROS_AUD_CH_STRIDE
		if chIdx >= 4 {
			return
		}
		c := &dma.channels[chIdx]
		switch off {
		case AROS_AUD_OFF_PTR:
			c.ptr = value
		case AROS_AUD_OFF_LEN:
			c.len = value
		case AROS_AUD_OFF_PER:
			c.per = value
		case AROS_AUD_OFF_VOL:
			c.vol = value & 0x7F
		}
		return
	}

	switch addr {
	case AROS_AUD_DMACON:
		if value&0x8000 != 0 {
			// Set mode — enable channels
			newBits := value & 0x0F
			for ch := range 4 {
				bit := uint32(1 << ch)
				if newBits&bit != 0 && dma.dmacon&bit == 0 {
					c := &dma.channels[ch]
					c.pos = 0
					c.phase = 0
					c.active = true

					// Initialise flex channel for DAC output
					flexBase := uint32(FLEX_CH_BASE) + uint32(ch)*uint32(FLEX_CH_STRIDE)
					ieVol := c.vol * 4
					if ieVol > 255 {
						ieVol = 255
					}
					dma.soundChip.HandleRegisterWrite(flexBase+FLEX_OFF_VOL, ieVol)
					dma.soundChip.HandleRegisterWrite(flexBase+FLEX_OFF_CTRL, 3)
					dma.soundChip.HandleRegisterWrite(flexBase+FLEX_OFF_DAC, 0)
				}
			}
			dma.dmacon |= newBits
		} else {
			// Clear mode — disable channels
			clearBits := value & 0x0F
			for ch := range 4 {
				if clearBits&(1<<ch) != 0 {
					dma.channels[ch].active = false
				}
			}
			dma.dmacon &= ^clearBits
		}
		dma.enabled.Store(dma.dmacon != 0)

	case AROS_AUD_STATUS:
		// Write-to-clear: bits written as 1 clear the corresponding status bits.
		dma.status &= ^(value & 0x0F)

	case AROS_AUD_INTENA:
		if value&0x8000 != 0 {
			dma.intena |= value & 0x0F
		} else {
			dma.intena &= ^(value & 0x0F)
		}
	}
}
