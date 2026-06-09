package main

// AtariMIDIACIA is a minimal, output-only MC6850 ACIA shim that bridges the
// Atari ST/Falcon MIDI port ($FFFC04/$FFFC06) into the shared *LiveMIDI. It
// holds no synth or voice state of its own: control writes forward to the
// LiveMIDI parser, status reads always report ready-to-transmit, and there is
// no MIDI-in. It exists only for EmuTOS, which talks MIDI through Atari
// hardware registers rather than IE's native live-MIDI port.
type AtariMIDIACIA struct {
	live *LiveMIDI // shared live port (one synth, one voice pool)
	ctrl byte      // last control value (stored, otherwise ignored)
}

// NewAtariMIDIACIA builds an ACIA shim over an existing LiveMIDI.
func NewAtariMIDIACIA(live *LiveMIDI) *AtariMIDIACIA {
	return &AtariMIDIACIA{live: live}
}

// MapRegisters maps both the bus-canonical low-16 addresses and the 24-bit
// Atari aliases. MachineBus routes guest sign-extended accesses at
// 0xFFFFFC04/06 through the low-16 handlers, while 0x00FFFC04/06 must be
// mapped directly.
func (a *AtariMIDIACIA) MapRegisters(bus *MachineBus) {
	a.mapRegister(bus, ATARI_MIDI_ACIA_CTRL)
	a.mapRegister(bus, ATARI_MIDI_ACIA_DATA)
	a.mapRegister(bus, ATARI_MIDI_ACIA_CTRL_24BIT)
	a.mapRegister(bus, ATARI_MIDI_ACIA_DATA_24BIT)
}

func (a *AtariMIDIACIA) mapRegister(bus *MachineBus, addr uint32) {
	// NoShadow is required: the low-16 alias and the 24-bit alias both fall
	// inside guest RAM, and MMIO status polls / data writes must not mirror
	// handler values into guest memory.
	bus.MapIONoShadow(addr, addr, a.read32, a.write32)
	bus.MapIOByteRead(addr, addr, a.read8)
	bus.MapIOByte(addr, addr, a.write8)
}

func (a *AtariMIDIACIA) read8(addr uint32) uint8 {
	switch addr {
	case ATARI_MIDI_ACIA_CTRL, ATARI_MIDI_ACIA_CTRL_24BIT:
		// Status: always ready to TX, never RX-full (output-only).
		return ACIA_SR_TDRE
	case ATARI_MIDI_ACIA_DATA, ATARI_MIDI_ACIA_DATA_24BIT:
		// No MIDI-in.
		return 0
	}
	return 0
}

func (a *AtariMIDIACIA) write8(addr uint32, v uint8) {
	switch addr {
	case ATARI_MIDI_ACIA_CTRL, ATARI_MIDI_ACIA_CTRL_24BIT:
		a.ctrl = v
		if a.live != nil && v&ACIA_CTRL_MASTER_RESET == ACIA_CTRL_MASTER_RESET {
			a.live.Reset()
		}
	case ATARI_MIDI_ACIA_DATA, ATARI_MIDI_ACIA_DATA_24BIT:
		if a.live != nil {
			a.live.Feed(v) // raw MIDI byte into the running-status parser
		}
	}
}

func (a *AtariMIDIACIA) read32(addr uint32) uint32     { return uint32(a.read8(addr)) }
func (a *AtariMIDIACIA) write32(addr uint32, v uint32) { a.write8(addr, uint8(v)) }
