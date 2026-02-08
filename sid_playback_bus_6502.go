package main

const (
	c64VICBase  = 0xD000
	c64VICEnd   = 0xD3FF
	c64SIDBase  = 0xD400
	c64SIDEnd   = 0xD41C
	c64CIA1Base = 0xDC00
	c64CIA1End  = 0xDCFF
	c64CIA2Base = 0xDD00
	c64CIA2End  = 0xDDFF

	ciaTimerALo = 0xDC04
	ciaTimerAHi = 0xDC05
	ciaTimerBLo = 0xDC06
	ciaTimerBHi = 0xDC07
	ciaICR      = 0xDC0D
	ciaCRA      = 0xDC0E
	ciaCRB      = 0xDC0F
)

// SIDEvent represents a single SID register write captured during playback.
type SIDEvent struct {
	Cycle  uint64
	Sample uint64
	Reg    uint8
	Value  uint8
}

// SIDPlaybackBus6502 implements a minimal C64-like memory bus for PSID playback.
type SIDPlaybackBus6502 struct {
	ram     [0x10000]byte
	sidRegs [SID_REG_COUNT]uint8
	vicRegs [0x400]byte
	events  []SIDEvent

	cycles     uint64
	frameCycle uint64

	ntsc       bool
	irqPending bool

	ciaTimerA  uint16
	ciaTimerB  uint16
	ciaLatchA  uint16
	ciaLatchB  uint16
	ciaCtrlA   uint8
	ciaCtrlB   uint8
	ciaICR     uint8
	ciaIRQMask uint8

	raster uint16
}

func newSIDPlaybackBus6502(ntsc bool) *SIDPlaybackBus6502 {
	bus := &SIDPlaybackBus6502{
		ntsc: ntsc,
	}
	bus.installIRQStub()
	return bus
}

// Read reads a byte from the given address.
func (b *SIDPlaybackBus6502) Read(addr uint16) byte {
	switch {
	case addr >= c64VICBase && addr <= c64VICEnd:
		return b.readVIC(addr)
	case addr >= c64SIDBase && addr <= c64SIDEnd:
		return b.readSID(addr)
	case addr >= c64CIA1Base && addr <= c64CIA1End:
		return b.readCIA1(addr)
	case addr >= c64CIA2Base && addr <= c64CIA2End:
		return 0xFF
	default:
		return b.ram[addr]
	}
}

// Write writes a byte to the given address.
func (b *SIDPlaybackBus6502) Write(addr uint16, value byte) {
	switch {
	case addr >= c64VICBase && addr <= c64VICEnd:
		b.writeVIC(addr, value)
	case addr >= c64SIDBase && addr <= c64SIDEnd:
		b.writeSID(addr, value)
	case addr >= c64CIA1Base && addr <= c64CIA1End:
		b.writeCIA1(addr, value)
	case addr >= c64CIA2Base && addr <= c64CIA2End:
		return
	default:
		b.ram[addr] = value
	}
}

func (b *SIDPlaybackBus6502) readSID(addr uint16) byte {
	reg := uint8(addr - c64SIDBase)
	if reg >= SID_REG_COUNT {
		return 0xFF
	}
	if reg == 0x1B || reg == 0x1C {
		return 0x00
	}
	return b.sidRegs[reg]
}

func (b *SIDPlaybackBus6502) writeSID(addr uint16, value byte) {
	reg := uint8(addr - c64SIDBase)
	if reg >= SID_REG_COUNT {
		return
	}
	b.sidRegs[reg] = value
	b.events = append(b.events, SIDEvent{
		Cycle: b.cycles,
		Reg:   reg,
		Value: value,
	})
}

func (b *SIDPlaybackBus6502) readVIC(addr uint16) byte {
	reg := addr - c64VICBase
	if reg >= uint16(len(b.vicRegs)) {
		return 0xFF
	}
	if reg == 0x11 {
		value := b.vicRegs[reg] & 0x7F
		if (b.raster & 0x100) != 0 {
			value |= 0x80
		}
		return value
	}
	if reg == 0x12 {
		return byte(b.raster & 0xFF)
	}
	return b.vicRegs[reg]
}

func (b *SIDPlaybackBus6502) writeVIC(addr uint16, value byte) {
	reg := addr - c64VICBase
	if reg >= uint16(len(b.vicRegs)) {
		return
	}
	b.vicRegs[reg] = value
}

func (b *SIDPlaybackBus6502) readCIA1(addr uint16) byte {
	switch addr {
	case ciaTimerALo:
		return byte(b.ciaTimerA & 0xFF)
	case ciaTimerAHi:
		return byte((b.ciaTimerA >> 8) & 0xFF)
	case ciaTimerBLo:
		return byte(b.ciaTimerB & 0xFF)
	case ciaTimerBHi:
		return byte((b.ciaTimerB >> 8) & 0xFF)
	case ciaICR:
		value := b.ciaICR
		if (value & b.ciaIRQMask) != 0 {
			value |= 0x80
		}
		b.ciaICR = 0
		b.irqPending = false
		return value
	case ciaCRA:
		return b.ciaCtrlA
	case ciaCRB:
		return b.ciaCtrlB
	default:
		return 0xFF
	}
}

func (b *SIDPlaybackBus6502) writeCIA1(addr uint16, value byte) {
	switch addr {
	case ciaTimerALo:
		b.ciaLatchA = (b.ciaLatchA & 0xFF00) | uint16(value)
	case ciaTimerAHi:
		b.ciaLatchA = (b.ciaLatchA & 0x00FF) | (uint16(value) << 8)
	case ciaTimerBLo:
		b.ciaLatchB = (b.ciaLatchB & 0xFF00) | uint16(value)
	case ciaTimerBHi:
		b.ciaLatchB = (b.ciaLatchB & 0x00FF) | (uint16(value) << 8)
	case ciaICR:
		mask := value & 0x1F
		if (value & 0x80) != 0 {
			b.ciaIRQMask |= mask
		} else {
			b.ciaIRQMask &^= mask
		}
	case ciaCRA:
		b.ciaCtrlA = value
		if (value & 0x10) != 0 {
			b.ciaTimerA = b.ciaLatchA
		}
	case ciaCRB:
		b.ciaCtrlB = value
		if (value & 0x10) != 0 {
			b.ciaTimerB = b.ciaLatchB
		}
	}
}

func (b *SIDPlaybackBus6502) installIRQStub() {
	b.ram[0xFF00] = 0x6C // JMP ($0314)
	b.ram[0xFF01] = 0x14
	b.ram[0xFF02] = 0x03
	b.ram[0xFFFE] = 0x00
	b.ram[0xFFFF] = 0xFF
}

// AddCycles advances the bus clock and updates CIA timers.
func (b *SIDPlaybackBus6502) AddCycles(cycles int) {
	if cycles <= 0 {
		return
	}
	b.cycles += uint64(cycles)

	if (b.ciaCtrlA & 0x01) != 0 {
		b.advanceTimer(&b.ciaTimerA, b.ciaLatchA, cycles, 0x01)
	}
	if (b.ciaCtrlB & 0x01) != 0 {
		b.advanceTimer(&b.ciaTimerB, b.ciaLatchB, cycles, 0x02)
	}
}

func (b *SIDPlaybackBus6502) advanceTimer(timer *uint16, latch uint16, cycles int, flag uint8) {
	if latch == 0 {
		return
	}

	remaining := int(*timer)
	if remaining == 0 {
		remaining = int(latch)
	}

	for cycles > 0 {
		if remaining <= cycles {
			cycles -= remaining
			b.setCIAFlag(flag)
			remaining = int(latch)
		} else {
			remaining -= cycles
			cycles = 0
		}
	}

	*timer = uint16(remaining)
}

func (b *SIDPlaybackBus6502) setCIAFlag(flag uint8) {
	b.ciaICR |= flag
	if (b.ciaICR & b.ciaIRQMask) != 0 {
		b.irqPending = true
	}
}

func (b *SIDPlaybackBus6502) StartFrame() {
	b.frameCycle = b.cycles
	b.events = b.events[:0]
}

// CollectEvents returns the events captured this frame.
// DEPRECATED: Use GetEvents() for zero-allocation access.
func (b *SIDPlaybackBus6502) CollectEvents() []SIDEvent {
	if len(b.events) == 0 {
		return nil
	}
	events := make([]SIDEvent, len(b.events))
	copy(events, b.events)
	b.events = b.events[:0]
	return events
}

// GetEvents returns a direct reference to the internal events slice.
// The caller must not retain this slice after the next StartFrame call.
// This is the zero-allocation path for performance-critical code.
func (b *SIDPlaybackBus6502) GetEvents() []SIDEvent {
	return b.events
}

// ClearEvents clears the internal events buffer without allocation.
// Call this after processing events from GetEvents().
func (b *SIDPlaybackBus6502) ClearEvents() {
	b.events = b.events[:0]
}

// GetFrameCycleStart returns the cycle count at the start of the current frame.
func (b *SIDPlaybackBus6502) GetFrameCycleStart() uint64 {
	return b.frameCycle
}

func (b *SIDPlaybackBus6502) LoadBinary(addr uint16, data []byte) {
	for i, v := range data {
		target := addr + uint16(i)
		b.ram[target] = v
	}
}

func (b *SIDPlaybackBus6502) Reset() {
	b.cycles = 0
	b.frameCycle = 0
	b.events = nil
	b.ciaTimerA = 0
	b.ciaTimerB = 0
	b.ciaLatchA = 0
	b.ciaLatchB = 0
	b.ciaCtrlA = 0
	b.ciaCtrlB = 0
	b.ciaICR = 0
	b.ciaIRQMask = 0
	b.irqPending = false
	b.installIRQStub()
	for i := range b.sidRegs {
		b.sidRegs[i] = 0
	}
	for i := range b.vicRegs {
		b.vicRegs[i] = 0
	}
	b.raster = 0
}

func (b *SIDPlaybackBus6502) GetCycles() uint64 {
	return b.cycles
}

func (b *SIDPlaybackBus6502) GetFrameCycles() uint64 {
	return b.cycles - b.frameCycle
}

func (b *SIDPlaybackBus6502) SetRaster(raster uint16) {
	b.raster = raster & 0x1FF
}
