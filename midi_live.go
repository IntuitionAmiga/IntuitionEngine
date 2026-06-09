package main

import "sync"

// LiveMIDI is a generic, CPU-agnostic live-MIDI port. It accepts a raw MIDI
// byte stream (the same wire format a hardware MIDI UART would carry), decodes
// it with a running-status state machine, and drives the shared MIDIEngine. It
// is usable by every CPU core and by BASIC.
type LiveMIDI struct {
	engine *MIDIEngine

	mu      sync.Mutex
	status  byte // last channel-voice status byte (running status), 0 if none
	data    [2]byte
	dataIdx int // data bytes collected for the in-progress message
	need    int // data bytes the in-progress message needs
	inSysex bool
}

// NewLiveMIDI builds a live port over an existing engine. The engine is shared
// with the file player so there is one synth, one voice pool, one status owner.
func NewLiveMIDI(engine *MIDIEngine) *LiveMIDI {
	return &LiveMIDI{engine: engine}
}

// MapRegisters maps the live-MIDI MMIO block onto the bus. MapIOByte only
// updates existing regions, so a MapIO call creates the region first; the byte
// handlers then carry single-byte guest access (BASIC POKE, x86 OUT, etc).
func (l *LiveMIDI) MapRegisters(bus *MachineBus) {
	bus.MapIO(IE_MIDI_LIVE_DATA, IE_MIDI_LIVE_END, l.handleRead32, l.handleWrite32)
	bus.MapIOByteRead(IE_MIDI_LIVE_DATA, IE_MIDI_LIVE_END, l.handleRead8)
	bus.MapIOByte(IE_MIDI_LIVE_DATA, IE_MIDI_LIVE_END, l.handleWrite8)
	bus.OnReset(l.Reset)
}

func (l *LiveMIDI) handleRead8(addr uint32) uint8 {
	if addr == IE_MIDI_LIVE_STATUS {
		if l.engine != nil && l.engine.LiveActive() {
			return MIDI_LIVE_STATUS_ACTIVE
		}
	}
	return 0
}

func (l *LiveMIDI) handleWrite8(addr uint32, value uint8) {
	switch addr {
	case IE_MIDI_LIVE_DATA:
		l.Feed(value)
	case IE_MIDI_LIVE_CTRL:
		if value&MIDI_LIVE_CTRL_RESET != 0 {
			l.Reset()
		}
	}
}

func (l *LiveMIDI) handleRead32(addr uint32) uint32     { return uint32(l.handleRead8(addr)) }
func (l *LiveMIDI) handleWrite32(addr uint32, v uint32) { l.handleWrite8(addr, uint8(v)) }

// Reset clears the parser state and performs an all-notes-off on the engine.
func (l *LiveMIDI) Reset() {
	l.mu.Lock()
	l.status = 0
	l.dataIdx = 0
	l.need = 0
	l.inSysex = false
	l.mu.Unlock()
	if l.engine != nil {
		l.engine.ResetLive()
	}
}

// Feed feeds one raw MIDI byte through the running-status state machine.
// A complete channel-voice message is decoded to a MIDIEvent and applied to the
// engine. Realtime bytes (0xF8-0xFF) and SysEx (0xF0..0xF7) do not corrupt the
// running status and emit no events.
func (l *LiveMIDI) Feed(b byte) {
	l.mu.Lock()
	ev, ok := l.feedLocked(b)
	l.mu.Unlock()
	if ok && l.engine != nil {
		l.engine.ApplyLiveEvent(ev)
	}
}

func (l *LiveMIDI) feedLocked(b byte) (MIDIEvent, bool) {
	switch {
	case b >= 0xF8:
		// System realtime: does not affect running status, no parameters.
		return MIDIEvent{}, false
	case b == 0xF0:
		// SysEx start: clears running status, swallow until 0xF7.
		l.inSysex = true
		l.status = 0
		l.dataIdx = 0
		return MIDIEvent{}, false
	case b == 0xF7:
		// SysEx / EOX end.
		l.inSysex = false
		return MIDIEvent{}, false
	case l.inSysex:
		return MIDIEvent{}, false
	case b >= 0x80:
		// Status byte.
		if b < 0xF0 {
			l.status = b
			l.dataIdx = 0
			l.need = midiDataBytesFor(b)
		} else {
			// System common (0xF1..0xF6): clears running status; we ignore it.
			l.status = 0
			l.dataIdx = 0
		}
		return MIDIEvent{}, false
	default:
		// Data byte.
		if l.status == 0 {
			return MIDIEvent{}, false
		}
		l.data[l.dataIdx] = b
		l.dataIdx++
		if l.dataIdx < l.need {
			return MIDIEvent{}, false
		}
		l.dataIdx = 0
		return decodeChannelMessage(l.status, l.data[0], l.data[1])
	}
}

func midiDataBytesFor(status byte) int {
	switch status & 0xF0 {
	case 0xC0, 0xD0: // program change, channel pressure
		return 1
	default:
		return 2
	}
}

// decodeChannelMessage maps a complete channel-voice message to a MIDIEvent.
// Messages the engine does not model (polyphonic aftertouch, channel pressure)
// return ok=false.
func decodeChannelMessage(status, d0, d1 byte) (MIDIEvent, bool) {
	ch := status & 0x0F
	switch status & 0xF0 {
	case 0x80:
		return MIDIEvent{Kind: MIDIEventNoteOff, Channel: ch, Note: d0, Velocity: d1}, true
	case 0x90:
		kind := MIDIEventNoteOn
		if d1 == 0 {
			kind = MIDIEventNoteOff
		}
		return MIDIEvent{Kind: kind, Channel: ch, Note: d0, Velocity: d1}, true
	case 0xB0:
		return MIDIEvent{Kind: MIDIEventControlChange, Channel: ch, Controller: d0, Value: int(d1)}, true
	case 0xC0:
		return MIDIEvent{Kind: MIDIEventProgramChange, Channel: ch, Program: d0}, true
	case 0xE0:
		return MIDIEvent{Kind: MIDIEventPitchBend, Channel: ch, Value: int(d0) | int(d1)<<7}, true
	default:
		// 0xA0 poly aftertouch, 0xD0 channel pressure: not modelled.
		return MIDIEvent{}, false
	}
}

// NoteOn / NoteOff / ProgramChange / ControlChange are convenience entry points
// for callers that hold structured values (e.g. the BASIC MIDI keyword). They
// route through the same byte path so behaviour is identical.
func (l *LiveMIDI) NoteOn(ch, note, velocity byte) {
	l.Feed(0x90 | (ch & 0x0F))
	l.Feed(note & 0x7F)
	l.Feed(velocity & 0x7F)
}

func (l *LiveMIDI) NoteOff(ch, note byte) {
	l.Feed(0x80 | (ch & 0x0F))
	l.Feed(note & 0x7F)
	l.Feed(0)
}

func (l *LiveMIDI) ProgramChange(ch, program byte) {
	l.Feed(0xC0 | (ch & 0x0F))
	l.Feed(program & 0x7F)
}

func (l *LiveMIDI) ControlChange(ch, controller, value byte) {
	l.Feed(0xB0 | (ch & 0x0F))
	l.Feed(controller & 0x7F)
	l.Feed(value & 0x7F)
}
