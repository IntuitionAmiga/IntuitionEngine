package main

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

// ArosAudioDMA emulates the AROS Paula-style DMA shim for audio.device.
// It implements the SampleTicker interface and is called at 44100 Hz from
// the SoundChip's ReadSample path. For each active DMA channel it reads
// sample bytes from guest RAM, writes them to the corresponding flex
// channel DAC, and triggers an M68K level-3 interrupt when a buffer is
// exhausted.
//
// Lock order: ArosAudioDMA.mu may be held while briefly taking SoundChip.mu
// for direct DAC handoff. SoundChip code must not call back into this DMA
// while holding SoundChip.mu.
type ArosAudioDMA struct {
	mu        sync.Mutex
	bus       *MachineBus
	soundChip *SoundChip
	cpu       *M68KCPU
	memBase   unsafe.Pointer // cached bus memory base for fast byte reads

	profileTop uint32 // AROS profile top-of-RAM; cached at construction so the
	// per-sample fetch hot path skips a bus accessor call. The AROS audio DMA
	// fetch is bounded by this rather than the underlying CPU's full
	// architectural visible range.

	channels [4]arosAudDMACh

	dmacon uint32 // DMA enable bitmask (bits 0–3)
	status uint32 // completion status  (bits 0–3)
	intena uint32 // interrupt enable   (bits 0–3)

	enabled atomic.Bool // true when any channel has DMA enabled
}

type arosAudDMACh struct {
	ptr     uint32 // sample pointer in guest RAM
	len     uint32 // sample length in words (×2 = bytes)
	per     uint32 // Paula period
	vol     uint32 // Paula volume (0–64)
	lptr    uint32 // arm-time latched sample pointer
	llen    uint32 // arm-time latched sample length
	lper    uint32 // arm-time latched period
	lvol    uint32 // arm-time latched volume
	nptr    uint32 // next-buffer latched pointer
	nlen    uint32 // next-buffer latched length
	nper    uint32 // next-buffer latched period
	nvol    uint32 // next-buffer latched volume
	hasNext bool
	pos     uint32  // current byte offset within buffer
	phase   float64 // fractional sample accumulator
	active  bool    // DMA running
}

// NewArosAudioDMA creates a DMA engine wired to the given bus, SoundChip,
// and M68K CPU. Call MapIO afterwards to register the MMIO handlers.
func NewArosAudioDMA(bus *MachineBus, sc *SoundChip, cpu *M68KCPU) (*ArosAudioDMA, error) {
	pb := AROSProfileBounds(bus)
	if pb.Err != nil {
		return nil, pb.Err
	}
	top := pb.TopOfRAM
	if top == 0 {
		top = uint32(len(bus.memory))
	}
	return &ArosAudioDMA{
		bus:        bus,
		soundChip:  sc,
		cpu:        cpu,
		memBase:    unsafe.Pointer(&bus.memory[0]),
		profileTop: top,
	}, nil
}

func (dma *ArosAudioDMA) Reset() {
	dma.mu.Lock()
	defer dma.mu.Unlock()
	dma.dmacon = 0
	dma.status = 0
	dma.intena = 0
	for i := range dma.channels {
		dma.channels[i] = arosAudDMACh{}
	}
	dma.enabled.Store(false)
}

func arosAudioTeardown(dma *ArosAudioDMA, sysBus *MachineBus, chip *SoundChip) {
	if dma == nil || sysBus == nil || chip == nil {
		return
	}
	dma.Reset()
	sysBus.UnmapIO(AROS_AUD_REGION_BASE, AROS_AUD_REGION_END)
	runtimeStatus.setPaulaDMA(nil)
	chip.UnregisterSampleTickerIf("default", dma)
}

func arosTeardownAll(snap runtimeStatusSnapshot, sysBus *MachineBus, chip *SoundChip) {
	if sysBus == nil {
		return
	}
	if snap.paulaDMA != nil {
		arosAudioTeardown(snap.paulaDMA, sysBus, chip)
	}
	if snap.arosDOS != nil {
		snap.arosDOS.Close()
		sysBus.UnmapIO(AROS_DOS_REGION_BASE, AROS_DOS_REGION_END)
		runtimeStatus.setAROSDOS(nil)
	}
	if snap.arosClip != nil {
		snap.arosClip.Close()
		sysBus.UnmapIO(CLIP_REGION_BASE, CLIP_REGION_END)
		runtimeStatus.setAROSClipboard(nil)
	}
	sysBus.UnmapIO(IRQ_DIAG_REGION_BASE, IRQ_DIAG_REGION_END)
}

// TickSample is called at 44100 Hz from SoundChip.ReadSample().
// It advances each active DMA channel by one output sample, reads the
// next byte from guest RAM, and pushes it into the flex channel DAC.
func (dma *ArosAudioDMA) TickSample() {
	if !dma.enabled.Load() {
		return
	}

	var assertIRQ bool
	dma.mu.Lock()
	for ch := range 4 {
		if dma.dmacon&(1<<ch) == 0 {
			continue
		}
		c := &dma.channels[ch]
		if !c.active || c.lper == 0 || c.llen == 0 {
			continue
		}

		// Phase increment: paulaFreq / sampleRate
		paulaFreq := paulaClockPAL / float64(c.lper)
		phaseInc := paulaFreq / float64(SAMPLE_RATE)

		c.phase += phaseInc

		// Advance byte position by integer part of accumulated phase
		for c.phase >= 1.0 {
			c.pos++
			c.phase -= 1.0
		}

		bufBytes := c.llen * 2 // len is in words
		if c.pos >= bufBytes {
			// Buffer exhausted — signal completion
			dma.status |= 1 << ch
			if dma.intena&(1<<ch) != 0 {
				assertIRQ = true
			}
			c.pos = 0
			if c.hasNext && c.nper != 0 && c.nlen != 0 {
				c.lptr = c.nptr
				c.llen = c.nlen
				c.lper = c.nper
				c.lvol = c.nvol
				c.ptr = c.nptr
				c.len = c.nlen
				c.per = c.nper
				c.vol = c.nvol
				c.hasNext = false
			} else {
				c.active = false
				dma.dmacon &^= 1 << ch
			}
			continue
		}

		// Read sample byte directly from bus memory (big-endian byte order).
		// Bounded by the AROS profile top, not DEFAULT_MEMORY_SIZE, so future
		// profile changes do not silently widen AROS audio DMA.
		addr := c.lptr + c.pos
		if addr < dma.profileTop {
			var sample byte
			if uint64(addr) < uint64(len(dma.bus.memory)) {
				// Fast path: address within the legacy bus.memory slice.
				sample = *(*byte)(unsafe.Pointer(uintptr(dma.memBase) + uintptr(addr)))
			} else {
				// Slow path: high backing (e.g. SparseBacking for AROS 2 GiB).
				// ReadPhys8 routes through the bound Backing for addresses
				// above the legacy slice; bus.Read8 would zero-fill them.
				sample = dma.bus.ReadPhys8(uint64(addr))
			}
			dma.setFlexDACLocked(ch, float32(int8(sample))/128.0, c.lvol)
		} else {
			c.active = false
			dma.dmacon &^= 1 << ch
			dma.status |= 1 << ch
			dma.setFlexDACLocked(ch, 0, c.lvol)
			if dma.intena&(1<<ch) != 0 {
				assertIRQ = true
			}
		}
	}
	dma.enabled.Store(dma.dmacon != 0)
	cpu := dma.cpu
	dma.mu.Unlock()
	if assertIRQ && cpu != nil {
		cpu.AssertInterrupt(arosAudioIRQLevel)
	}
}

// HandleRead returns the value of the MMIO register at addr.
func (dma *ArosAudioDMA) HandleRead(addr uint32) uint32 {
	dma.mu.Lock()
	defer dma.mu.Unlock()
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
	var assertIRQ bool
	dma.mu.Lock()
	if addr >= AROS_AUD_REGION_BASE && addr < AROS_AUD_DMACON {
		chIdx := (addr - AROS_AUD_REGION_BASE) / AROS_AUD_CH_STRIDE
		off := (addr - AROS_AUD_REGION_BASE) % AROS_AUD_CH_STRIDE
		if chIdx >= 4 {
			dma.mu.Unlock()
			return
		}
		c := &dma.channels[chIdx]
		switch off {
		case AROS_AUD_OFF_PTR:
			c.ptr = value &^ 1
			if c.active {
				c.nptr = c.ptr
				c.hasNext = true
			}
		case AROS_AUD_OFF_LEN:
			c.len = value
			if c.active {
				c.nlen = value
				c.hasNext = true
			}
		case AROS_AUD_OFF_PER:
			if value == 0 {
				dma.mu.Unlock()
				return
			}
			c.per = value
			if c.active {
				c.nper = value
				c.hasNext = true
			}
		case AROS_AUD_OFF_VOL:
			if value > 64 {
				value = 64
			}
			c.vol = value
			if c.active {
				c.nvol = value
				c.hasNext = true
			}
		}
		dma.mu.Unlock()
		return
	}

	switch addr {
	case AROS_AUD_DMACON:
		if value&0x8000 != 0 {
			// Set mode — enable channels
			newBits := value & 0x0F
			acceptedBits := uint32(0)
			for ch := range 4 {
				bit := uint32(1 << ch)
				if newBits&bit != 0 && dma.dmacon&bit == 0 {
					c := &dma.channels[ch]
					if c.per == 0 || c.len == 0 {
						c.active = false
						dma.status |= bit
						if dma.intena&bit != 0 {
							assertIRQ = true
						}
						continue
					}
					c.lptr = c.ptr
					c.llen = c.len
					c.lper = c.per
					c.lvol = c.vol
					c.nptr = c.ptr
					c.nlen = c.len
					c.nper = c.per
					c.nvol = c.vol
					c.hasNext = false
					c.pos = 0
					c.phase = 0
					c.active = true
					acceptedBits |= bit
					dma.setFlexDACLocked(ch, 0, c.lvol)
				}
			}
			dma.dmacon |= acceptedBits
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
	cpu := dma.cpu
	dma.mu.Unlock()
	if assertIRQ && cpu != nil {
		cpu.AssertInterrupt(arosAudioIRQLevel)
	}
}

func (dma *ArosAudioDMA) setFlexDACLocked(ch int, sample float32, vol uint32) {
	if dma.soundChip == nil || ch < 0 || ch >= len(dma.soundChip.channels) {
		return
	}
	dma.soundChip.mu.Lock()
	defer dma.soundChip.mu.Unlock()
	flexCh := dma.soundChip.channels[ch]
	if flexCh == nil {
		return
	}
	if !flexCh.gate {
		flexCh.envelopePhase = ENV_ATTACK
		flexCh.envelopeSample = 0
		if !flexCh.sidEnvelope {
			flexCh.envelopeLevel = 0
		}
	}
	flexCh.enabled = true
	flexCh.gate = true
	flexCh.dacMode = true
	flexCh.dacValue = sample
	ieVol := vol * 4
	if ieVol > 255 {
		ieVol = 255
	}
	flexCh.volume = float32(ieVol) / 255.0
}
