package main

import "testing"

// EmuTOS Atari MIDI ACIA (MC6850) bridge tests.
//
// The ACIA is an output-only shim over the shared *LiveMIDI. EmuTOS sees the
// Atari hardware contract at $FFFC04/$FFFC06; in IE this reaches the bus as
// either the sign-extended low-16 alias (0xFFFFFC04/06, normalized to
// 0x0000FC04/06 before handler lookup) or the 24-bit Atari I/O alias
// (0x00FFFC04/06). All three forms must drive the same LiveMIDI/MIDIEngine.

// newACIATestRig builds a bus + shared engine + mapped ACIA over a LiveMIDI.
func newACIATestRig() (*MachineBus, *MIDIEngine, *AtariMIDIACIA) {
	bus := NewMachineBus()
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)
	live.MapRegisters(bus)
	acia := NewAtariMIDIACIA(live)
	acia.MapRegisters(bus)
	return bus, eng, acia
}

// (1) status read returns TDRE (ready to TX) at every address alias.
func TestAtariMIDIACIA_StatusReady(t *testing.T) {
	bus, _, _ := newACIATestRig()
	for _, addr := range []uint32{
		ATARI_MIDI_ACIA_CTRL,         // bus-canonical low-16
		ATARI_MIDI_ACIA_CTRL_SIGNEXT, // guest-visible sign-extended
		ATARI_MIDI_ACIA_CTRL_24BIT,   // 24-bit Atari hardware alias
	} {
		if got := bus.Read8(addr); got != ACIA_SR_TDRE {
			t.Fatalf("status read at 0x%08X = 0x%02X, want TDRE 0x%02X", addr, got, ACIA_SR_TDRE)
		}
	}
}

// (2) writing a Note-On 3-byte sequence to each data alias drives the engine.
func TestAtariMIDIACIA_DataWriteNoteOn(t *testing.T) {
	for _, dataAddr := range []uint32{
		ATARI_MIDI_ACIA_DATA,
		ATARI_MIDI_ACIA_DATA_SIGNEXT,
		ATARI_MIDI_ACIA_DATA_24BIT,
	} {
		bus, eng, _ := newACIATestRig()
		for _, b := range []byte{0x90, 0x3C, 0x64} { // note-on ch0 note60 vel100
			bus.Write8(dataAddr, b)
		}
		if !eng.HasActiveNote(0x3C) {
			t.Fatalf("data alias 0x%08X: note 0x3C not active", dataAddr)
		}
		if !eng.LiveActive() {
			t.Fatalf("data alias 0x%08X: LiveActive should be true", dataAddr)
		}
	}
}

// (3) master reset control write clears a partial parser message and live voices.
func TestAtariMIDIACIA_MasterReset(t *testing.T) {
	bus, eng, _ := newACIATestRig()
	// Two full note-ons plus a dangling status byte starting a partial message.
	for _, b := range []byte{0x90, 0x3C, 0x64, 0x40, 0x64, 0x90, 0x43} {
		bus.Write8(ATARI_MIDI_ACIA_DATA, b)
	}
	if eng.ActiveVoiceCount() != 2 {
		t.Fatalf("setup: want 2 active voices, got %d", eng.ActiveVoiceCount())
	}

	bus.Write8(ATARI_MIDI_ACIA_CTRL, ACIA_CTRL_MASTER_RESET)

	if eng.ActiveVoiceCount() != 0 {
		t.Fatalf("master reset did not clear voices, got %d", eng.ActiveVoiceCount())
	}
	if eng.LiveActive() {
		t.Fatalf("master reset should deactivate the live port")
	}
	// Reset must also discard the partial message: completing it must not sound.
	bus.Write8(ATARI_MIDI_ACIA_DATA, 0x64)
	if eng.HasActiveNote(0x43) {
		t.Fatalf("master reset leaked the partial parser message")
	}
}

// (4) no-shadow: ACIA reads/writes must not mirror status/data into guest RAM.
func TestAtariMIDIACIA_NoShadow(t *testing.T) {
	bus, _, _ := newACIATestRig()

	// Drive both a status read and a data write on every alias.
	for _, ctrl := range []uint32{ATARI_MIDI_ACIA_CTRL, ATARI_MIDI_ACIA_CTRL_SIGNEXT, ATARI_MIDI_ACIA_CTRL_24BIT} {
		_ = bus.Read8(ctrl)
	}
	for _, data := range []uint32{ATARI_MIDI_ACIA_DATA, ATARI_MIDI_ACIA_DATA_SIGNEXT, ATARI_MIDI_ACIA_DATA_24BIT} {
		bus.Write8(data, 0x90)
	}

	// Check the low-16 and 24-bit backing bytes are untouched (still zero).
	for _, addr := range []uint32{
		ATARI_MIDI_ACIA_CTRL, ATARI_MIDI_ACIA_DATA,
		ATARI_MIDI_ACIA_CTRL_24BIT, ATARI_MIDI_ACIA_DATA_24BIT,
	} {
		if bus.memory[addr] != 0 {
			t.Fatalf("no-shadow violated: bus.memory[0x%08X] = 0x%02X, want 0", addr, bus.memory[addr])
		}
	}
}

// (5) odd-address / wide-access guard.
func TestAtariMIDIACIA_OddAddressGuard(t *testing.T) {
	// Byte writes to the odd ...FC05 / ...FC07 addresses must not feed MIDI.
	bus, eng, _ := newACIATestRig()
	for _, b := range []byte{0x90, 0x3C, 0x64} {
		bus.Write8(ATARI_MIDI_ACIA_CTRL+1, b) // 0x0000FC05
		bus.Write8(ATARI_MIDI_ACIA_DATA+1, b) // 0x0000FC07
	}
	if eng.LiveActive() || eng.ActiveVoiceCount() != 0 {
		t.Fatalf("odd-address byte writes fed MIDI: active=%v count=%d", eng.LiveActive(), eng.ActiveVoiceCount())
	}

	// A 16-bit write to the data register fans out to onWrite8(addr) and
	// onWrite8(addr+1). The high byte lands on the odd ...FC07 address and must
	// be ignored; only the addressed low byte (0x90 here) is fed.
	bus2, eng2, _ := newACIATestRig()
	bus2.Write16(ATARI_MIDI_ACIA_DATA, 0x3C90) // LE: 0x90 at FC06, 0x3C at FC07
	bus2.Write8(ATARI_MIDI_ACIA_DATA, 0x3C)    // supply the real data byte
	bus2.Write8(ATARI_MIDI_ACIA_DATA, 0x64)
	if !eng2.HasActiveNote(0x3C) {
		t.Fatalf("16-bit fanout: low byte 0x90 should have been fed as status")
	}
	if eng2.HasActiveNote(0x3C + 1) {
		t.Fatalf("16-bit fanout: high byte from odd FC07 must not have been fed")
	}
}

// (6) narrow M68K-level smoke test through the CPU's bus path. CPU accesses at
// 0xFFFFFC04/06 and 0x00FFFC04/06 are >= 0xA0000 so they route through the bus
// MMIO handlers (the < 0xA0000 fast path is bypassed).
func TestM68KAtariMIDIACIASmoke(t *testing.T) {
	bus := NewMachineBus()
	eng := newLiveMIDITestEngine()
	live := NewLiveMIDI(eng)
	live.MapRegisters(bus)
	NewAtariMIDIACIA(live).MapRegisters(bus)

	cpu := NewM68KCPU(bus)

	for _, ctrl := range []uint32{ATARI_MIDI_ACIA_CTRL_SIGNEXT, ATARI_MIDI_ACIA_CTRL_24BIT} {
		if got := cpu.Read8(ctrl); got != ACIA_SR_TDRE {
			t.Fatalf("M68K status read at 0x%08X = 0x%02X, want TDRE", ctrl, got)
		}
	}

	for _, dataAddr := range []uint32{ATARI_MIDI_ACIA_DATA_SIGNEXT, ATARI_MIDI_ACIA_DATA_24BIT} {
		// Reset between aliases so each proves its own path.
		cpu.Write8(ATARI_MIDI_ACIA_CTRL_SIGNEXT, ACIA_CTRL_MASTER_RESET)
		for _, b := range []byte{0x90, 0x3C, 0x64} {
			cpu.Write8(dataAddr, b)
		}
		if !eng.HasActiveNote(0x3C) {
			t.Fatalf("M68K data write at 0x%08X did not drive engine", dataAddr)
		}
	}
}
