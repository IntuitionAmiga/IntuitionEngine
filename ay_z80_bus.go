// ay_z80_bus.go - Z80 bus for ZXAYEMUL playback with AY port mapping.

package main

type ayZ80Write struct {
	Reg   byte
	Value byte
	Cycle uint64
}

type ayZ80PSGWriter interface {
	WriteRegister(reg uint8, value uint8)
}

type ayZ80Bus struct {
	ram       *[0x10000]byte
	system    byte
	engine    ayZ80PSGWriter
	regSelect byte
	regs      [PSG_REG_COUNT]byte
	writes    []ayZ80Write
	cycles    uint64
}

func newAYZ80Bus(ram *[0x10000]byte, system byte, engine ayZ80PSGWriter) *ayZ80Bus {
	return &ayZ80Bus{
		ram:    ram,
		system: system,
		engine: engine,
	}
}

func (b *ayZ80Bus) Read(addr uint16) byte {
	return b.ram[addr]
}

func (b *ayZ80Bus) Write(addr uint16, value byte) {
	b.ram[addr] = value
}

func (b *ayZ80Bus) In(port uint16) byte {
	if b.isAYDataPort(port) && b.regSelect < PSG_REG_COUNT {
		return b.regs[b.regSelect]
	}
	return 0
}

func (b *ayZ80Bus) Out(port uint16, value byte) {
	if b.isAYSelectPort(port) {
		b.regSelect = value & 0x0F
		return
	}
	if b.isAYDataPort(port) && b.regSelect < PSG_REG_COUNT {
		b.regs[b.regSelect] = value
		b.writes = append(b.writes, ayZ80Write{
			Reg:   b.regSelect,
			Value: value,
			Cycle: b.cycles,
		})
		if b.engine != nil {
			b.engine.WriteRegister(b.regSelect, value)
		}
	}
}

func (b *ayZ80Bus) Tick(cycles int) {
	b.cycles += uint64(cycles)
}

func (b *ayZ80Bus) isAYSelectPort(port uint16) bool {
	switch b.system {
	case ayZXSystemCPC:
		return byte(port) == 0xF4
	case ayZXSystemMSX:
		return byte(port) == 0xA0
	default:
		return port&0xC002 == 0xC000
	}
}

func (b *ayZ80Bus) isAYDataPort(port uint16) bool {
	switch b.system {
	case ayZXSystemCPC:
		return byte(port) == 0xF6
	case ayZXSystemMSX:
		return byte(port) == 0xA1
	default:
		return port&0xC002 == 0x8000
	}
}
