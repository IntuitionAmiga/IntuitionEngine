// ay_z80_bus.go - Z80 bus for ZXAYEMUL playback with AY port mapping.

package main

type ayPlaybackWriteZ80 struct {
	Reg   byte
	Value byte
	Cycle uint64
}

type ayPlaybackPSGWriterZ80 interface {
	WriteRegister(reg uint8, value uint8)
}

type ayPlaybackBusZ80 struct {
	ram       *[0x10000]byte
	system    byte
	engine    ayPlaybackPSGWriterZ80
	regSelect byte
	regs      [PSG_REG_COUNT]byte
	writes    []ayPlaybackWriteZ80
	cycles    uint64

	// Pre-computed port matching for fast dispatch
	selectPortMask uint16 // Mask for port matching
	selectPortVal  uint16 // Value after masking for select port
	dataPortMask   uint16 // Mask for port matching
	dataPortVal    uint16 // Value after masking for data port
	useByteMatch   bool   // True if matching on low byte only
}

func newAyPlaybackBusZ80(ram *[0x10000]byte, system byte, engine ayPlaybackPSGWriterZ80) *ayPlaybackBusZ80 {
	b := &ayPlaybackBusZ80{
		ram:    ram,
		system: system,
		engine: engine,
	}
	b.updatePortMatching()
	return b
}

// updatePortMatching pre-computes port matching values based on system type
func (b *ayPlaybackBusZ80) updatePortMatching() {
	switch b.system {
	case ayZXSystemCPC:
		// CPC: low byte match F4/F6
		b.useByteMatch = true
		b.selectPortVal = 0xF4
		b.dataPortVal = 0xF6
	case ayZXSystemMSX:
		// MSX: low byte match A0/A1
		b.useByteMatch = true
		b.selectPortVal = 0xA0
		b.dataPortVal = 0xA1
	default:
		// ZX128/Spectrum: mask-based matching
		b.useByteMatch = false
		b.selectPortMask = 0xC002
		b.selectPortVal = 0xC000
		b.dataPortMask = 0xC002
		b.dataPortVal = 0x8000
	}
}

func (b *ayPlaybackBusZ80) Read(addr uint16) byte {
	return b.ram[addr]
}

func (b *ayPlaybackBusZ80) Write(addr uint16, value byte) {
	b.ram[addr] = value
}

func (b *ayPlaybackBusZ80) In(port uint16) byte {
	if b.isAYDataPort(port) && b.regSelect < PSG_REG_COUNT {
		return b.regs[b.regSelect]
	}
	return 0
}

func (b *ayPlaybackBusZ80) Out(port uint16, value byte) {
	if b.isAYSelectPort(port) {
		b.regSelect = value & 0x0F
		return
	}
	if b.isAYDataPort(port) && b.regSelect < PSG_REG_COUNT {
		b.regs[b.regSelect] = value
		b.writes = append(b.writes, ayPlaybackWriteZ80{
			Reg:   b.regSelect,
			Value: value,
			Cycle: b.cycles,
		})
		if b.engine != nil {
			b.engine.WriteRegister(b.regSelect, value)
		}
	}
}

func (b *ayPlaybackBusZ80) Tick(cycles int) {
	b.cycles += uint64(cycles)
}

// isAYSelectPort checks if the port is the AY register select port
// Uses pre-computed values for fast matching without branching.
func (b *ayPlaybackBusZ80) isAYSelectPort(port uint16) bool {
	if b.useByteMatch {
		return byte(port) == byte(b.selectPortVal)
	}
	return port&b.selectPortMask == b.selectPortVal
}

// isAYDataPort checks if the port is the AY data port
// Uses pre-computed values for fast matching without branching.
func (b *ayPlaybackBusZ80) isAYDataPort(port uint16) bool {
	if b.useByteMatch {
		return byte(port) == byte(b.dataPortVal)
	}
	return port&b.dataPortMask == b.dataPortVal
}
