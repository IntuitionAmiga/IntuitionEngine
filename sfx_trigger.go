package main

import (
	"encoding/binary"
	"math"
	"sync"
)

// SFXTrigger implements the IE_SFX trigger-and-forget sample MMIO block.
// MMIO writes and audio ticks share per-channel state through a small mutex per
// channel. TickSample locks each channel only long enough to snapshot, decode,
// and advance that one channel, then publishes a stable mixed contribution for
// SoundChip.GenerateSample to add after the regular FLEX/MOD average.
type SFXTrigger struct {
	bus      *MachineBus
	channels [IE_SFX_CHANNELS]sfxChannel
	mixMu    sync.Mutex
	mix      float32
}

type sfxChannel struct {
	mu          sync.Mutex
	shadow      [IE_SFX_CH_STRIDE]byte
	ptr         uint32
	length      uint32
	loopPtr     uint32
	loopLength  uint32
	frequency   uint32
	volume      uint16
	panReserved uint16
	format      uint8
	loopEnabled bool
	loopActive  bool
	playing     bool
	ptrOOBError bool
	cursor      float64
}

func NewSFXTrigger() *SFXTrigger {
	return &SFXTrigger{}
}

func (s *SFXTrigger) AttachBus(bus *MachineBus) {
	s.bus = bus
}

func (s *SFXTrigger) HandleRead(addr uint32) uint32 {
	ch, offset, ok := s.channelFor(addr)
	if !ok {
		return 0
	}
	c := &s.channels[ch]
	c.mu.Lock()
	defer c.mu.Unlock()

	if offset == SFX_CTRL {
		var status uint32
		if c.playing {
			status |= SFX_STATUS_PLAYING
		}
		if c.ptrOOBError {
			status |= SFX_STATUS_ERROR
		}
		return status
	}
	if offset+3 < IE_SFX_CH_STRIDE {
		return binary.LittleEndian.Uint32(c.shadow[offset : offset+4])
	}
	return uint32(c.shadow[offset])
}

func (s *SFXTrigger) HandleWrite(addr uint32, value uint32) {
	ch, offset, ok := s.channelFor(addr)
	if !ok {
		return
	}
	c := &s.channels[ch]
	c.mu.Lock()
	defer c.mu.Unlock()

	if offset+3 < IE_SFX_CH_STRIDE {
		binary.LittleEndian.PutUint32(c.shadow[offset:offset+4], value)
	}
	s.applyLocked(c, offset, value)
}

func (s *SFXTrigger) HandleWrite8(addr uint32, value uint8) {
	ch, offset, ok := s.channelFor(addr)
	if !ok {
		return
	}
	c := &s.channels[ch]
	c.mu.Lock()
	defer c.mu.Unlock()

	c.shadow[offset] = value
	regBase := offset &^ 3
	fullValue := binary.LittleEndian.Uint32(c.shadow[regBase : regBase+4])
	s.applyLocked(c, regBase, fullValue)
}

func (s *SFXTrigger) TickSample() {
	var mixed float32
	for i := range s.channels {
		mixed += s.tickChannel(&s.channels[i])
	}
	s.mixMu.Lock()
	s.mix = clampF32(mixed, MIN_SAMPLE, MAX_SAMPLE)
	s.mixMu.Unlock()
}

func (s *SFXTrigger) MixSample() float32 {
	s.mixMu.Lock()
	defer s.mixMu.Unlock()
	return s.mix
}

func (s *SFXTrigger) Reset() {
	for i := range s.channels {
		c := &s.channels[i]
		c.mu.Lock()
		c.shadow = [IE_SFX_CH_STRIDE]byte{}
		c.ptr = 0
		c.length = 0
		c.loopPtr = 0
		c.loopLength = 0
		c.frequency = 0
		c.volume = 0
		c.panReserved = 0
		c.format = 0
		c.loopEnabled = false
		c.loopActive = false
		c.playing = false
		c.ptrOOBError = false
		c.cursor = 0
		c.mu.Unlock()
	}
	s.mixMu.Lock()
	s.mix = 0
	s.mixMu.Unlock()
}

func (s *SFXTrigger) channelFor(addr uint32) (int, uint32, bool) {
	if addr < IE_SFX_REGION_BASE || addr > IE_SFX_REGION_END {
		return 0, 0, false
	}
	rel := addr - IE_SFX_REGION_BASE
	ch := int(rel / IE_SFX_CH_STRIDE)
	if ch < 0 || ch >= IE_SFX_CHANNELS {
		return 0, 0, false
	}
	return ch, rel % IE_SFX_CH_STRIDE, true
}

func (s *SFXTrigger) applyLocked(c *sfxChannel, offset uint32, value uint32) {
	switch offset {
	case SFX_PTR:
		c.ptr = value
	case SFX_LEN:
		c.length = value
	case SFX_LOOP_PTR:
		c.loopPtr = value
	case SFX_LOOP_LEN:
		c.loopLength = value
	case SFX_FREQ:
		c.frequency = value
	case SFX_VOL:
		c.volume = uint16(value & 0xFFFF)
	case SFX_PAN_RESERVED:
		c.panReserved = uint16(value & 0xFFFF)
	case SFX_FORMAT:
		c.format = uint8(value)
	case SFX_CTRL:
		if value&SFX_CTRL_STOP != 0 {
			c.playing = false
			c.cursor = 0
		}
		c.loopEnabled = value&SFX_CTRL_LOOP_EN != 0
		if value&SFX_CTRL_TRIGGER != 0 {
			s.triggerLocked(c)
		}
	}
}

func (s *SFXTrigger) triggerLocked(c *sfxChannel) {
	c.ptrOOBError = false
	c.playing = false
	c.loopActive = false
	c.cursor = 0

	if c.length == 0 || !s.sampleRangeValid(c.ptr, c.length) {
		c.ptrOOBError = true
		return
	}
	if c.loopEnabled && c.loopPtr != 0 && c.loopLength != 0 && !s.sampleRangeValid(c.loopPtr, c.loopLength) {
		c.ptrOOBError = true
		return
	}
	c.playing = true
}

func (s *SFXTrigger) sampleRangeValid(ptr, length uint32) bool {
	if s.bus == nil || length == 0 {
		return false
	}
	end := uint64(ptr) + uint64(length)
	if end > math.MaxUint32+1 {
		return false
	}
	limit := s.bus.ActiveVisibleRAM()
	if limit == 0 {
		limit = uint64(len(s.bus.GetMemory()))
	}
	return end <= limit
}

func (s *SFXTrigger) tickChannel(c *sfxChannel) float32 {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.playing {
		return 0
	}
	bytesPerSample := c.bytesPerSampleLocked()
	endSample := c.playbackEndSampleLocked(bytesPerSample)
	if endSample <= 0 {
		c.playing = false
		return 0
	}

	idx := int(c.cursor)
	if idx >= endSample {
		if !s.wrapLoopLocked(c, bytesPerSample) {
			return 0
		}
		idx = int(c.cursor)
		endSample = c.playbackEndSampleLocked(bytesPerSample)
	}

	raw := s.decodeSampleLocked(c, idx, bytesPerSample)
	step := float64(c.frequency)
	if step == 0 {
		step = SAMPLE_RATE
	}
	c.cursor += step / SAMPLE_RATE
	if int(c.cursor) >= endSample {
		s.wrapLoopLocked(c, bytesPerSample)
	}

	vol := float32(c.volume)
	if vol > 255 {
		vol = 255
	}
	return raw * (vol / 255.0)
}

func (s *SFXTrigger) wrapLoopLocked(c *sfxChannel, bytesPerSample int) bool {
	if !c.loopEnabled || c.loopPtr == 0 || c.loopLength == 0 {
		c.playing = false
		c.loopActive = false
		c.cursor = 0
		return false
	}
	loopSamples := int(c.loopLength) / bytesPerSample
	if loopSamples <= 0 {
		c.playing = false
		c.loopActive = false
		c.cursor = 0
		return false
	}
	c.loopActive = true
	c.cursor = 0
	return true
}

func (c *sfxChannel) playbackEndSampleLocked(bytesPerSample int) int {
	if c.loopActive {
		return int(c.loopLength) / bytesPerSample
	}
	return int(c.length) / bytesPerSample
}

func (s *SFXTrigger) decodeSampleLocked(c *sfxChannel, idx int, bytesPerSample int) float32 {
	base := c.ptr
	if c.loopActive {
		base = c.loopPtr
	}
	byteAddr := base + uint32(idx*bytesPerSample)
	switch c.format {
	case SFX_FORMAT_UNSIGNED8:
		return (float32(s.readSampleByte(byteAddr)) - 128.0) / 128.0
	case SFX_FORMAT_SIGNED16:
		lo := s.readSampleByte(byteAddr)
		hi := s.readSampleByte(byteAddr + 1)
		v := int16(binary.LittleEndian.Uint16([]byte{lo, hi}))
		if v == math.MinInt16 {
			return -1
		}
		return float32(v) / 32767.0
	default:
		v := int8(s.readSampleByte(byteAddr))
		if v == math.MinInt8 {
			return -1
		}
		return float32(v) / 127.0
	}
}

func (s *SFXTrigger) readSampleByte(addr uint32) byte {
	mem := s.bus.GetMemory()
	if uint64(addr) < uint64(len(mem)) {
		return mem[addr]
	}
	return s.bus.Read8(addr)
}

func (c *sfxChannel) bytesPerSampleLocked() int {
	if c.format == SFX_FORMAT_SIGNED16 {
		return 2
	}
	return 1
}

var _ SampleTicker = (*SFXTrigger)(nil)
